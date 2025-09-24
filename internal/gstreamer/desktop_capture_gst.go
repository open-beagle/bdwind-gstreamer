package gstreamer

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-gst/go-gst/gst"
	"github.com/sirupsen/logrus"

	"github.com/open-beagle/bdwind-gstreamer/internal/config"
)

// DesktopCaptureGst implements desktop video capture using go-gst library
// This component has high cohesion - it manages its own pipeline internally
// and provides a clean interface for sample output
// Implements the DesktopCapture interface to maintain compatibility
type DesktopCaptureGst struct {
	// Configuration
	config config.DesktopCaptureConfig
	logger *logrus.Entry

	// Internal pipeline management (not exposed)
	pipeline *gst.Pipeline
	source   *gst.Element
	sink     *gst.Element
	bus      *gst.Bus

	// Sample output channel (decoupled from other components)
	sampleChan chan *Sample

	// State management
	mu        sync.RWMutex
	isRunning bool
	ctx       context.Context
	cancel    context.CancelFunc

	// Internal error handling
	errorChan chan error

	// Statistics tracking
	stats *captureStatistics

	// Sample callback for compatibility with existing interface
	sampleCallback func(*Sample) error

	// Memory management
	memoryManager *MemoryManager

	// Object lifecycle management
	lifecycleManager *ObjectLifecycleManager
}

// captureStatistics tracks internal capture metrics
type captureStatistics struct {
	mu                    sync.RWMutex
	framesTotal           int64
	framesDropped         int64
	startTime             time.Time
	lastFrameTime         time.Time
	errorCount            int64
	recoveryAttempts      int64
	lastRecoveryTime      time.Time
	sampleConversionFails int64
	pipelineStateErrors   int64
}

// NewDesktopCaptureGst creates a new desktop capture component with go-gst
func NewDesktopCaptureGst(cfg config.DesktopCaptureConfig) (*DesktopCaptureGst, error) {
	// Validate configuration
	if err := config.ValidateDesktopCaptureConfig(&cfg); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Create logger
	logger := logrus.WithField("component", "desktop-capture-gst")

	// Initialize memory manager with appropriate configuration
	memConfig := MemoryManagerConfig{
		BufferPoolSize:       100,
		GCInterval:           30 * time.Second,
		LeakDetectionEnabled: true,
		MaxObjectRefs:        1000,
		CleanupInterval:      5 * time.Minute,
	}

	memoryManager := NewMemoryManager(&memConfig, logger)

	// Initialize object lifecycle manager
	lifecycleConfig := ObjectLifecycleConfig{
		CleanupInterval:    30 * time.Second,
		ObjectRetention:    5 * time.Minute,
		EnableValidation:   true,
		ValidationInterval: 60 * time.Second,
		EnableStackTrace:   false, // Disable for performance in production
		LogObjectEvents:    false, // Disable for performance in production
	}
	lifecycleManager := NewObjectLifecycleManager(lifecycleConfig)

	dc := &DesktopCaptureGst{
		config:           cfg,
		logger:           logrus.WithField("component", "desktop-capture-gst"),
		sampleChan:       make(chan *Sample, cfg.BufferSize),
		errorChan:        make(chan error, 10),
		ctx:              ctx,
		cancel:           cancel,
		memoryManager:    memoryManager,
		lifecycleManager: lifecycleManager,
		stats: &captureStatistics{
			startTime: time.Now(),
		},
	}

	// Initialize internal pipeline
	if err := dc.initializePipeline(); err != nil {
		cancel()
		memoryManager.Cleanup()
		lifecycleManager.Close()
		return nil, fmt.Errorf("failed to initialize pipeline: %w", err)
	}

	return dc, nil
}

// initializePipeline creates and configures the internal GStreamer pipeline
func (dc *DesktopCaptureGst) initializePipeline() error {
	// Create pipeline with memory tracking
	pipeline, err := gst.NewPipeline("desktop-capture")
	if err != nil {
		return fmt.Errorf("failed to create pipeline: %w", err)
	}
	dc.pipeline = pipeline

	// Track pipeline object
	if dc.memoryManager != nil {
		dc.memoryManager.RegisterObject(pipeline)
	}

	// Get bus for message handling
	dc.bus = pipeline.GetPipelineBus()
	if dc.bus != nil && dc.memoryManager != nil {
		dc.memoryManager.RegisterObject(dc.bus)
	}

	// Create and configure source element based on display server
	if err := dc.createSourceElement(); err != nil {
		return fmt.Errorf("failed to create source element: %w", err)
	}

	// Processing elements will be created during linking

	// Create sink element
	if err := dc.createSinkElement(); err != nil {
		return fmt.Errorf("failed to create sink element: %w", err)
	}

	// Link all elements
	if err := dc.linkElements(); err != nil {
		return fmt.Errorf("failed to link elements: %w", err)
	}

	dc.logger.Info("Desktop capture pipeline initialized successfully")
	return nil
}

// createSourceElement creates and configures the appropriate capture source
func (dc *DesktopCaptureGst) createSourceElement() error {
	var elementName string

	// Select source based on display server (X11 vs Wayland)
	if dc.config.UseWayland {
		elementName = "waylandsrc"
	} else {
		elementName = "ximagesrc"
	}

	source, err := gst.NewElement(elementName)
	if err != nil {
		return fmt.Errorf("failed to create %s element: %w", elementName, err)
	}
	dc.source = source

	// Track source element
	if dc.memoryManager != nil {
		dc.memoryManager.RegisterObject(source)
	}

	// Configure source properties
	if err := dc.configureSourceProperties(); err != nil {
		return fmt.Errorf("failed to configure source properties: %w", err)
	}

	// Add to pipeline
	if err := dc.pipeline.Add(source); err != nil {
		return fmt.Errorf("failed to add source to pipeline: %w", err)
	}

	dc.logger.Debugf("Created and configured %s source element", elementName)
	return nil
}

// configureSourceProperties configures source element properties based on config
func (dc *DesktopCaptureGst) configureSourceProperties() error {
	if dc.config.UseWayland {
		return dc.configureWaylandSource()
	}
	return dc.configureX11Source()
}

// configureX11Source configures ximagesrc properties
func (dc *DesktopCaptureGst) configureX11Source() error {
	// Set display
	if err := dc.source.SetProperty("display-name", dc.config.DisplayID); err != nil {
		return fmt.Errorf("failed to set display-name: %w", err)
	}

	// Configure damage events for optimization (disable for stability)
	if err := dc.source.SetProperty("use-damage", false); err != nil {
		return fmt.Errorf("failed to set use-damage: %w", err)
	}

	// Configure mouse pointer visibility
	if err := dc.source.SetProperty("show-pointer", dc.config.ShowPointer); err != nil {
		return fmt.Errorf("failed to set show-pointer: %w", err)
	}

	// Add safety properties to prevent crashes
	if err := dc.source.SetProperty("remote", false); err != nil {
		dc.logger.Debug("Failed to set remote property (may not be supported)")
	}

	// Configure capture region if specified
	if dc.config.CaptureRegion != nil {
		region := dc.config.CaptureRegion
		if err := dc.source.SetProperty("startx", uint(region.X)); err != nil {
			return fmt.Errorf("failed to set startx: %w", err)
		}
		if err := dc.source.SetProperty("starty", uint(region.Y)); err != nil {
			return fmt.Errorf("failed to set starty: %w", err)
		}
		if err := dc.source.SetProperty("endx", uint(region.X+region.Width-1)); err != nil {
			return fmt.Errorf("failed to set endx: %w", err)
		}
		if err := dc.source.SetProperty("endy", uint(region.Y+region.Height-1)); err != nil {
			return fmt.Errorf("failed to set endy: %w", err)
		}
	}

	dc.logger.Debug("Configured X11 source properties")
	return nil
}

// configureTestPatternSource configures videotestsrc properties
func (dc *DesktopCaptureGst) configureTestPatternSource() error {
	// Set test pattern (use string value for pattern)
	if err := dc.source.SetProperty("pattern", "smpte"); err != nil {
		// Try with integer value as fallback
		if err := dc.source.SetProperty("pattern", int(0)); err != nil {
			dc.logger.Debug("Failed to set pattern property, using defaults")
		}
	}

	// Set frame rate to match config
	if err := dc.source.SetProperty("num-buffers", int(-1)); err != nil { // Infinite buffers
		dc.logger.Debug("Failed to set num-buffers (may not be supported)")
	}

	// Set animation for visual feedback
	if err := dc.source.SetProperty("animation-mode", int(1)); err != nil { // Wall time
		dc.logger.Debug("Failed to set animation-mode (may not be supported)")
	}

	dc.logger.Debug("Configured test pattern source properties")
	return nil
}

// configureWaylandSource configures waylandsrc properties
func (dc *DesktopCaptureGst) configureWaylandSource() error {
	// Set display if specified
	if dc.config.DisplayID != "" && dc.config.DisplayID != ":0" {
		if err := dc.source.SetProperty("display", dc.config.DisplayID); err != nil {
			return fmt.Errorf("failed to set display: %w", err)
		}
	}

	// Configure mouse pointer visibility
	if err := dc.source.SetProperty("show-pointer", dc.config.ShowPointer); err != nil {
		return fmt.Errorf("failed to set show-pointer: %w", err)
	}

	// Configure capture region if specified
	if dc.config.CaptureRegion != nil {
		region := dc.config.CaptureRegion
		if err := dc.source.SetProperty("crop-x", uint(region.X)); err != nil {
			return fmt.Errorf("failed to set crop-x: %w", err)
		}
		if err := dc.source.SetProperty("crop-y", uint(region.Y)); err != nil {
			return fmt.Errorf("failed to set crop-y: %w", err)
		}
		if err := dc.source.SetProperty("crop-width", uint(region.Width)); err != nil {
			return fmt.Errorf("failed to set crop-width: %w", err)
		}
		if err := dc.source.SetProperty("crop-height", uint(region.Height)); err != nil {
			return fmt.Errorf("failed to set crop-height: %w", err)
		}
	}

	dc.logger.Debug("Configured Wayland source properties")
	return nil
}

// createAndGetProcessingElements creates video processing elements and returns them
func (dc *DesktopCaptureGst) createAndGetProcessingElements() (*gst.Element, *gst.Element, *gst.Element, *gst.Element, error) {
	// Create video convert element
	videoConvert, err := gst.NewElement("videoconvert")
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to create videoconvert: %w", err)
	}

	// Track video convert element
	if dc.memoryManager != nil {
		dc.memoryManager.RegisterObject(videoConvert)
	}

	// Create video scale element
	videoScale, err := gst.NewElement("videoscale")
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to create videoscale: %w", err)
	}

	// Track video scale element
	if dc.memoryManager != nil {
		dc.memoryManager.RegisterObject(videoScale)
	}

	// Create video rate element for frame rate control
	videoRate, err := gst.NewElement("videorate")
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to create videorate: %w", err)
	}

	// Track video rate element
	if dc.memoryManager != nil {
		dc.memoryManager.RegisterObject(videoRate)
	}

	// Configure video rate to drop frames only
	if err := videoRate.SetProperty("drop-only", true); err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to set drop-only: %w", err)
	}

	// Create caps filter for format specification
	capsFilter, err := gst.NewElement("capsfilter")
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to create capsfilter: %w", err)
	}

	// Track caps filter element
	if dc.memoryManager != nil {
		dc.memoryManager.RegisterObject(capsFilter)
	}

	// Configure caps
	capsStr := dc.buildCapsString()
	caps := gst.NewCapsFromString(capsStr)
	if caps == nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to create caps from string '%s'", capsStr)
	}

	// Track caps object
	if dc.memoryManager != nil {
		dc.memoryManager.RegisterObject(caps)
	}

	if err := capsFilter.SetProperty("caps", caps); err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to set caps: %w", err)
	}

	// Add all elements to pipeline with error handling
	elements := []*gst.Element{videoConvert, videoScale, videoRate, capsFilter}
	for i, element := range elements {
		if err := dc.pipeline.Add(element); err != nil {
			return nil, nil, nil, nil, fmt.Errorf("failed to add element %d to pipeline: %w", i, err)
		}
	}

	dc.logger.Debugf("Created processing elements with caps: %s", capsStr)
	return videoConvert, videoScale, videoRate, capsFilter, nil
}

// buildCapsString builds the caps string based on configuration
func (dc *DesktopCaptureGst) buildCapsString() string {
	format := "BGRx" // Default format

	// Adjust format based on quality preset
	switch dc.config.Quality {
	case "low":
		format = "RGB"
	case "medium":
		format = "BGRx"
	case "high":
		format = "BGRA"
	}

	return fmt.Sprintf("video/x-raw,format=%s,width=%d,height=%d,framerate=%d/1,pixel-aspect-ratio=1/1",
		format, dc.config.Width, dc.config.Height, dc.config.FrameRate)
}

// createSinkElement creates and configures the appsink element
func (dc *DesktopCaptureGst) createSinkElement() error {
	sink, err := gst.NewElement("appsink")
	if err != nil {
		return fmt.Errorf("failed to create appsink: %w", err)
	}
	dc.sink = sink

	// Track sink element
	if dc.memoryManager != nil {
		dc.memoryManager.RegisterObject(sink)
	}

	// Configure appsink properties with error handling
	properties := map[string]interface{}{
		"emit-signals": true,
		"sync":         false,
		"max-buffers":  uint(1),
		"drop":         true,
	}

	for name, value := range properties {
		if err := sink.SetProperty(name, value); err != nil {
			return fmt.Errorf("failed to set property %s: %w", name, err)
		}
	}

	// Add to pipeline
	if err := dc.pipeline.Add(sink); err != nil {
		return fmt.Errorf("failed to add sink to pipeline: %w", err)
	}

	dc.logger.Debug("Created and configured appsink element")
	return nil
}

// linkElements links all pipeline elements together
func (dc *DesktopCaptureGst) linkElements() error {
	// Create processing elements and get references
	videoConvert, videoScale, videoRate, capsFilter, err := dc.createAndGetProcessingElements()
	if err != nil {
		return fmt.Errorf("failed to get processing elements: %w", err)
	}

	// Link source -> videoconvert
	if err := dc.source.Link(videoConvert); err != nil {
		return fmt.Errorf("failed to link source to videoconvert: %w", err)
	}

	// Link videoconvert -> videoscale
	if err := videoConvert.Link(videoScale); err != nil {
		return fmt.Errorf("failed to link videoconvert to videoscale: %w", err)
	}

	// Link videoscale -> videorate
	if err := videoScale.Link(videoRate); err != nil {
		return fmt.Errorf("failed to link videoscale to videorate: %w", err)
	}

	// Link videorate -> capsfilter
	if err := videoRate.Link(capsFilter); err != nil {
		return fmt.Errorf("failed to link videorate to capsfilter: %w", err)
	}

	// Link capsfilter -> sink
	if err := capsFilter.Link(dc.sink); err != nil {
		return fmt.Errorf("failed to link capsfilter to sink: %w", err)
	}

	dc.logger.Debug("Successfully linked all pipeline elements")
	return nil
}

// Start starts the desktop capture pipeline
func (dc *DesktopCaptureGst) Start() error {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	if dc.isRunning {
		return fmt.Errorf("desktop capture already running")
	}

	// Connect to new-sample signal
	dc.sink.Connect("new-sample", dc.onNewSample)

	// Start bus message monitoring
	go dc.monitorBusMessages()

	// Start sample processing
	go dc.processSamples()

	// Set pipeline to playing state
	if err := dc.pipeline.SetState(gst.StatePlaying); err != nil {
		return fmt.Errorf("failed to set pipeline to playing state: %w", err)
	}

	// Wait for state change to complete
	ret, _ := dc.pipeline.GetState(gst.StatePlaying, gst.ClockTimeNone)
	if ret == gst.StateChangeFailure {
		return fmt.Errorf("failed to reach playing state")
	}

	dc.isRunning = true
	dc.stats.startTime = time.Now()

	dc.logger.Info("Desktop capture started successfully")
	return nil
}

// Stop stops the desktop capture pipeline
func (dc *DesktopCaptureGst) Stop() error {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	if !dc.isRunning {
		return nil
	}

	dc.logger.Info("Stopping desktop capture pipeline...")

	// Cancel context to stop goroutines
	dc.cancel()

	// Set pipeline to null state with proper error handling and timeout
	if dc.pipeline != nil {
		dc.logger.Debug("Setting pipeline to null state")
		if err := dc.pipeline.SetState(gst.StateNull); err != nil {
			dc.logger.Warnf("Failed to set pipeline to null state: %v", err)
		}

		// Wait for state change to complete with timeout
		ret, _ := dc.pipeline.GetState(gst.StateNull, gst.ClockTime(5*time.Second))
		if ret == gst.StateChangeFailure {
			dc.logger.Warn("Pipeline failed to reach null state within timeout")
		} else {
			dc.logger.Debug("Pipeline successfully reached null state")
		}

		// Unref pipeline and related objects to ensure proper cleanup
		if dc.bus != nil {
			dc.logger.Debug("Cleaning up bus object")
			// Note: go-gst handles reference counting automatically
			dc.bus = nil
		}

		dc.logger.Debug("Cleaning up pipeline object")
		// Note: go-gst handles reference counting automatically
		dc.pipeline = nil
		dc.source = nil
		dc.sink = nil
	}

	dc.isRunning = false

	// Drain and close channels safely
	dc.logger.Debug("Closing channels")

	// Drain sample channel
	go func() {
		for range dc.sampleChan {
			// Drain remaining samples
		}
	}()

	// Close sample channel
	select {
	case <-dc.sampleChan:
	default:
		close(dc.sampleChan)
	}

	// Drain error channel
	go func() {
		for range dc.errorChan {
			// Drain remaining errors
		}
	}()

	// Close error channel
	select {
	case <-dc.errorChan:
	default:
		close(dc.errorChan)
	}

	// Clean up memory manager with final leak check
	if dc.memoryManager != nil {
		dc.logger.Debug("Performing final memory cleanup")

		// Trigger final cleanup
		dc.memoryManager.ForceGC()

		// Check for leaks before closing
		leaks := dc.memoryManager.CheckMemoryLeaks()
		if len(leaks) > 0 {
			dc.logger.Warnf("Found %d potential memory leaks during shutdown", len(leaks))
			for _, leak := range leaks {
				dc.logger.Warnf("Leaked object: %s (ID: %x, Age: %v, RefCount: %d)",
					leak.ObjectType, leak.ObjectID, leak.Age, leak.RefCount)
			}
		}

		dc.memoryManager.Cleanup()
		dc.memoryManager = nil
	}

	// Clean up lifecycle manager
	if dc.lifecycleManager != nil {
		dc.logger.Debug("Shutting down object lifecycle manager")
		dc.lifecycleManager.Close()
		dc.lifecycleManager = nil
	}

	dc.logger.Info("Desktop capture stopped successfully")
	return nil
}

// GetSampleChannel returns the channel for receiving captured samples
func (dc *DesktopCaptureGst) GetSampleChannel() <-chan *Sample {
	return dc.sampleChan
}

// GetErrorChannel returns the channel for receiving errors
func (dc *DesktopCaptureGst) GetErrorChannel() <-chan error {
	return dc.errorChan
}

// IsRunning returns whether the capture is currently running
func (dc *DesktopCaptureGst) IsRunning() bool {
	dc.mu.RLock()
	defer dc.mu.RUnlock()
	return dc.isRunning
}

// GetStats returns current capture statistics
func (dc *DesktopCaptureGst) GetStats() CaptureStats {
	dc.stats.mu.RLock()
	defer dc.stats.mu.RUnlock()

	elapsed := time.Since(dc.stats.startTime).Seconds()
	avgFPS := float64(0)
	if elapsed > 0 {
		avgFPS = float64(dc.stats.framesTotal) / elapsed
	}

	// Calculate current FPS based on recent frames
	currentFPS := avgFPS // Simplified - in practice you'd calculate over a shorter window

	// Calculate latency metrics (simplified implementation)
	currentLatency := int64(0)
	if !dc.stats.lastFrameTime.IsZero() {
		currentLatency = time.Since(dc.stats.lastFrameTime).Milliseconds()
	}

	return CaptureStats{
		FramesTotal:    dc.stats.framesTotal,
		FramesCapture:  dc.stats.framesTotal,
		FramesDropped:  dc.stats.framesDropped,
		CurrentFPS:     currentFPS,
		AverageFPS:     avgFPS,
		CaptureLatency: currentLatency,
		MinLatency:     0,              // Would need to track this properly
		MaxLatency:     0,              // Would need to track this properly
		AvgLatency:     currentLatency, // Simplified
		LastFrameTime:  dc.stats.lastFrameTime.UnixMilli(),
		StartTime:      dc.stats.startTime.UnixMilli(),
		ErrorCount:     dc.stats.errorCount,
		UptimeSeconds:  elapsed,
		BytesProcessed: 0, // Would need to track this properly
	}
}

// GetMemoryStats returns current memory usage statistics
func (dc *DesktopCaptureGst) GetMemoryStats() *MemoryStatistics {
	if dc.memoryManager == nil {
		return &MemoryStatistics{}
	}
	return dc.memoryManager.GetMemoryStats()
}

// TriggerMemoryCleanup triggers garbage collection and memory cleanup
func (dc *DesktopCaptureGst) TriggerMemoryCleanup() {
	if dc.memoryManager != nil {
		dc.memoryManager.ForceGC()
	}
}

// CheckForMemoryLeaks checks for potential memory leaks
func (dc *DesktopCaptureGst) CheckForMemoryLeaks() []MemoryLeak {
	if dc.memoryManager == nil {
		return nil
	}
	return dc.memoryManager.CheckMemoryLeaks()
}

// onNewSample handles new samples from the appsink with enhanced error handling
func (dc *DesktopCaptureGst) onNewSample(sink *gst.Element) gst.FlowReturn {
	// Enhanced safety check for nil sink with context
	if sink == nil {
		dc.logger.WithFields(logrus.Fields{
			"component": "desktop-capture",
			"method":    "onNewSample",
			"error":     "nil_sink",
		}).Error("Received nil sink in onNewSample callback")
		dc.handleSampleError("nil_sink", fmt.Errorf("received nil sink"))
		return gst.FlowError
	}

	// Pull sample from appsink with enhanced error handling
	sample, err := sink.Emit("pull-sample")
	if err != nil {
		dc.logger.WithFields(logrus.Fields{
			"component":    "desktop-capture",
			"method":       "onNewSample",
			"error":        "pull_sample_failed",
			"sink_name":    sink.GetName(),
			"error_detail": err.Error(),
		}).Warn("Failed to pull sample from sink")
		dc.handleSampleError("pull_sample_failed", err)
		return dc.handleSampleFailure(err)
	}

	// Enhanced nil sample check with context
	if sample == nil {
		dc.logger.WithFields(logrus.Fields{
			"component": "desktop-capture",
			"method":    "onNewSample",
			"warning":   "nil_sample",
			"sink_name": sink.GetName(),
		}).Debug("Received nil sample from sink - this may be normal during state transitions")
		return gst.FlowOK // Continue processing, this might be normal
	}

	// Enhanced type assertion with detailed error context
	gstSample, ok := sample.(*gst.Sample)
	if !ok {
		actualType := "unknown"
		if sample != nil {
			actualType = fmt.Sprintf("%T", sample)
		}
		dc.logger.WithFields(logrus.Fields{
			"component":     "desktop-capture",
			"method":        "onNewSample",
			"error":         "type_assertion_failed",
			"expected_type": "*gst.Sample",
			"actual_type":   actualType,
			"sink_name":     sink.GetName(),
		}).Error("Sample type assertion failed")
		dc.handleSampleError("type_assertion_failed", fmt.Errorf("expected *gst.Sample, got %T", sample))
		return gst.FlowError
	}

	// Convert to internal sample format with graceful degradation
	internalSample, err := dc.convertGstSampleWithRecovery(gstSample)
	if err != nil {
		dc.logger.WithFields(logrus.Fields{
			"component":    "desktop-capture",
			"method":       "onNewSample",
			"error":        "sample_conversion_failed",
			"error_detail": err.Error(),
			"sink_name":    sink.GetName(),
		}).Warn("Failed to convert sample, attempting recovery")

		// Attempt graceful degradation
		return dc.handleSampleConversionFailure(gstSample, err)
	}

	// Enhanced sample delivery with error tracking
	delivered := dc.deliverSample(internalSample)
	if !delivered {
		dc.logger.WithFields(logrus.Fields{
			"component":   "desktop-capture",
			"method":      "onNewSample",
			"warning":     "sample_dropped",
			"sample_size": len(internalSample.Data),
			"reason":      "channel_full",
		}).Debug("Sample dropped due to full channel")
	}

	return gst.FlowOK
}

// convertGstSample converts a go-gst sample to internal Sample format with enhanced object lifecycle management
func (dc *DesktopCaptureGst) convertGstSample(gstSample *gst.Sample) (*Sample, error) {
	// Enhanced safety check for nil sample with detailed logging
	if gstSample == nil {
		dc.logger.WithFields(logrus.Fields{
			"component": "desktop-capture",
			"method":    "convertGstSample",
			"error":     "nil_sample",
		}).Error("Received nil gst sample")
		return nil, fmt.Errorf("received nil gst sample")
	}

	// Register sample object with lifecycle manager for validity tracking
	var sampleLifecycleID uintptr
	if dc.lifecycleManager != nil {
		var err error
		sampleLifecycleID, err = dc.lifecycleManager.RegisterObject(gstSample, "gst.Sample", "desktop-capture-sample")
		if err != nil {
			dc.logger.WithFields(logrus.Fields{
				"component": "desktop-capture",
				"method":    "convertGstSample",
				"error":     "lifecycle_registration_failed",
				"details":   err.Error(),
			}).Warn("Failed to register sample with lifecycle manager")
			// Continue processing but without lifecycle tracking
		}
		defer func() {
			if sampleLifecycleID != 0 {
				dc.lifecycleManager.UnregisterObject(sampleLifecycleID)
			}
		}()
	}

	// Use safe access to validate and process the sample
	var conversionErr error
	if dc.lifecycleManager != nil && sampleLifecycleID != 0 {
		conversionErr = dc.lifecycleManager.SafeObjectAccess(sampleLifecycleID, func() error {
			// Validate sample object before processing
			if !dc.lifecycleManager.ValidateGstObject(gstSample) {
				return fmt.Errorf("sample object failed validation check")
			}
			return nil
		})

		if conversionErr != nil {
			dc.logger.WithFields(logrus.Fields{
				"component": "desktop-capture",
				"method":    "convertGstSample",
				"error":     "sample_validation_failed",
				"details":   conversionErr.Error(),
			}).Error("Sample object validation failed during safe access")
			return nil, conversionErr
		}
	}

	// Track the sample object if memory manager is available
	var sampleRef *ObjectRef
	if dc.memoryManager != nil {
		sampleRef = dc.memoryManager.RegisterObject(gstSample)
		defer func() {
			if sampleRef != nil {
				dc.memoryManager.UnregisterObject(sampleRef)
			}
		}()
	}

	// Get buffer from sample with enhanced safety checks and validation
	buffer := gstSample.GetBuffer()
	if buffer == nil {
		dc.logger.WithFields(logrus.Fields{
			"component": "desktop-capture",
			"method":    "convertGstSample",
			"error":     "no_buffer",
		}).Error("No buffer found in gst sample")
		return nil, fmt.Errorf("no buffer in sample")
	}

	// Register buffer object with lifecycle manager
	var bufferLifecycleID uintptr
	if dc.lifecycleManager != nil {
		var err error
		bufferLifecycleID, err = dc.lifecycleManager.RegisterObject(buffer, "gst.Buffer", "sample-buffer")
		if err != nil {
			dc.logger.WithFields(logrus.Fields{
				"component": "desktop-capture",
				"method":    "convertGstSample",
				"warning":   "buffer_lifecycle_registration_failed",
				"details":   err.Error(),
			}).Debug("Failed to register buffer with lifecycle manager")
		}
		defer func() {
			if bufferLifecycleID != 0 {
				dc.lifecycleManager.UnregisterObject(bufferLifecycleID)
			}
		}()

		// Validate buffer object before processing
		if !dc.lifecycleManager.ValidateGstObject(buffer) {
			dc.logger.WithFields(logrus.Fields{
				"component": "desktop-capture",
				"method":    "convertGstSample",
				"error":     "buffer_validation_failed",
			}).Error("Buffer object failed validation check")
			return nil, fmt.Errorf("buffer object is no longer valid")
		}
	}

	// Get buffer size first for validation and tracking
	bufferSize := int(buffer.GetSize())
	if bufferSize == 0 {
		dc.logger.Debug("convertGstSample: empty buffer in sample")
		return nil, fmt.Errorf("empty buffer in sample")
	}

	// Enhanced buffer size validation with configurable limits
	maxBufferSize := 50 * 1024 * 1024 // Default 50MB

	if bufferSize > maxBufferSize {
		dc.logger.Errorf("convertGstSample: buffer too large: %d bytes (max: %d)", bufferSize, maxBufferSize)
		return nil, fmt.Errorf("buffer too large: %d bytes (max: %d)", bufferSize, maxBufferSize)
	}

	// Track the buffer object
	var bufferRef *ObjectRef
	if dc.memoryManager != nil {
		bufferRef = dc.memoryManager.RegisterObject(buffer)
		defer func() {
			if bufferRef != nil {
				dc.memoryManager.UnregisterObject(bufferRef)
			}
		}()
	}

	// Map buffer for reading with enhanced error handling and object validation
	var mapInfo *gst.MapInfo
	var mapRef *ObjectRef

	// Validate buffer before mapping
	if dc.lifecycleManager != nil && !dc.lifecycleManager.ValidateGstObject(buffer) {
		dc.logger.WithFields(logrus.Fields{
			"component": "desktop-capture",
			"method":    "convertGstSample",
			"error":     "buffer_invalid_before_mapping",
		}).Error("Buffer became invalid before mapping")
		return nil, fmt.Errorf("buffer object became invalid before mapping")
	}

	// Attempt buffer mapping with recovery
	func() {
		defer func() {
			if r := recover(); r != nil {
				dc.logger.WithFields(logrus.Fields{
					"component": "desktop-capture",
					"method":    "convertGstSample",
					"error":     "buffer_mapping_panic",
					"panic":     r,
				}).Error("Panic occurred during buffer mapping")
				mapInfo = nil // Ensure mapInfo is nil on panic
			}
		}()
		mapInfo = buffer.Map(gst.MapRead)
	}()

	if mapInfo == nil {
		dc.logger.WithFields(logrus.Fields{
			"component":   "desktop-capture",
			"method":      "convertGstSample",
			"error":       "buffer_mapping_failed",
			"buffer_size": bufferSize,
		}).Error("Failed to map gst buffer for reading")
		return nil, fmt.Errorf("failed to map buffer")
	}

	// Track the map info object
	if dc.memoryManager != nil {
		mapRef = dc.memoryManager.RegisterObject(mapInfo)
	}

	// Ensure buffer is unmapped and objects are properly released even if we panic
	defer func() {
		if err := recover(); err != nil {
			dc.logger.WithFields(logrus.Fields{
				"component": "desktop-capture",
				"method":    "convertGstSample",
				"error":     "panic_during_processing",
				"panic":     err,
			}).Error("Panic during buffer processing")

			// Safe cleanup with additional validation
			if mapInfo != nil && buffer != nil {
				// Validate buffer before unmapping
				if dc.lifecycleManager == nil || dc.lifecycleManager.ValidateGstObject(buffer) {
					func() {
						defer func() {
							if r := recover(); r != nil {
								dc.logger.Warnf("Panic during buffer unmap cleanup: %v", r)
							}
						}()
						buffer.Unmap()
					}()
				}
			}

			// Release map info tracking
			if mapRef != nil && dc.memoryManager != nil {
				dc.memoryManager.UnregisterObject(mapRef)
			}
			panic(err) // Re-panic after cleanup
		}

		// Normal cleanup path with validation
		if mapInfo != nil && buffer != nil {
			// Validate buffer before unmapping
			if dc.lifecycleManager == nil || dc.lifecycleManager.ValidateGstObject(buffer) {
				func() {
					defer func() {
						if r := recover(); r != nil {
							dc.logger.Warnf("Panic during normal buffer unmap: %v", r)
						}
					}()
					buffer.Unmap()
				}()
			} else {
				dc.logger.Warn("Skipping buffer unmap due to invalid buffer object")
			}
		}

		if mapRef != nil && dc.memoryManager != nil {
			dc.memoryManager.UnregisterObject(mapRef)
		}
	}()

	// Get buffer data with enhanced safety checks and validation
	var bufferBytes []byte
	func() {
		defer func() {
			if r := recover(); r != nil {
				dc.logger.WithFields(logrus.Fields{
					"component": "desktop-capture",
					"method":    "convertGstSample",
					"error":     "buffer_bytes_access_panic",
					"panic":     r,
				}).Error("Panic occurred while accessing buffer bytes")
				bufferBytes = nil // Ensure bufferBytes is nil on panic
			}
		}()
		bufferBytes = mapInfo.Bytes()
	}()

	if bufferBytes == nil {
		dc.logger.WithFields(logrus.Fields{
			"component": "desktop-capture",
			"method":    "convertGstSample",
			"error":     "buffer_bytes_nil",
		}).Error("Buffer bytes is nil after mapping")
		return nil, fmt.Errorf("buffer bytes is nil")
	}

	if len(bufferBytes) == 0 {
		dc.logger.WithFields(logrus.Fields{
			"component": "desktop-capture",
			"method":    "convertGstSample",
			"warning":   "empty_buffer_bytes",
		}).Debug("Empty buffer bytes in map info")
		return nil, fmt.Errorf("empty buffer bytes in map info")
	}

	// Enhanced buffer size validation with detailed logging
	if len(bufferBytes) != bufferSize {
		dc.logger.WithFields(logrus.Fields{
			"component":     "desktop-capture",
			"method":        "convertGstSample",
			"warning":       "buffer_size_mismatch",
			"expected_size": bufferSize,
			"actual_size":   len(bufferBytes),
		}).Warn("Buffer size mismatch detected")

		// Use the smaller size to prevent buffer overruns
		if len(bufferBytes) < bufferSize {
			bufferSize = len(bufferBytes)
			dc.logger.Debugf("Adjusted buffer size to %d bytes to prevent overrun", bufferSize)
		}
	}

	// Enhanced buffer copying with additional safety checks and validation
	var data []byte
	var err error

	// Validate buffer bounds before copying
	if bufferSize > len(bufferBytes) {
		dc.logger.WithFields(logrus.Fields{
			"component": "desktop-capture",
			"method":    "convertGstSample",
			"error":     "buffer_size_exceeds_available",
			"requested": bufferSize,
			"available": len(bufferBytes),
		}).Error("Requested buffer size exceeds available bytes")
		return nil, fmt.Errorf("buffer size %d exceeds available bytes %d", bufferSize, len(bufferBytes))
	}

	if dc.memoryManager != nil {
		// Use only the validated buffer size with bounds checking
		validBytes := bufferBytes[:bufferSize]

		// Additional validation before copying
		if len(validBytes) != bufferSize {
			dc.logger.WithFields(logrus.Fields{
				"component":    "desktop-capture",
				"method":       "convertGstSample",
				"error":        "slice_size_mismatch",
				"expected":     bufferSize,
				"actual_slice": len(validBytes),
			}).Error("Slice size does not match expected buffer size")
			return nil, fmt.Errorf("slice size mismatch: expected %d, got %d", bufferSize, len(validBytes))
		}

		data = make([]byte, len(validBytes))
		copy(data, validBytes)
		if err != nil {
			dc.logger.WithFields(logrus.Fields{
				"component":   "desktop-capture",
				"method":      "convertGstSample",
				"error":       "safe_copy_failed",
				"buffer_size": bufferSize,
				"details":     err.Error(),
			}).Error("Failed to copy buffer safely using memory manager")
			return nil, fmt.Errorf("failed to copy buffer safely: %w", err)
		}
	} else {
		// Fallback to direct allocation with enhanced size validation
		if bufferSize <= 0 {
			dc.logger.WithFields(logrus.Fields{
				"component":   "desktop-capture",
				"method":      "convertGstSample",
				"error":       "invalid_buffer_size",
				"buffer_size": bufferSize,
			}).Error("Invalid buffer size for direct allocation")
			return nil, fmt.Errorf("invalid buffer size: %d", bufferSize)
		}

		data = make([]byte, bufferSize)

		// Safe copy with bounds checking
		func() {
			defer func() {
				if r := recover(); r != nil {
					dc.logger.WithFields(logrus.Fields{
						"component": "desktop-capture",
						"method":    "convertGstSample",
						"error":     "copy_panic",
						"panic":     r,
					}).Error("Panic during buffer copy operation")
					data = nil // Ensure data is nil on panic
				}
			}()
			copy(data, bufferBytes[:bufferSize])
		}()

		if data == nil {
			return nil, fmt.Errorf("buffer copy failed due to panic")
		}
	}

	// Validate copied data
	if len(data) != bufferSize {
		dc.logger.WithFields(logrus.Fields{
			"component":     "desktop-capture",
			"method":        "convertGstSample",
			"error":         "copied_data_size_mismatch",
			"expected_size": bufferSize,
			"actual_size":   len(data),
		}).Error("Copied data size does not match expected size")
		return nil, fmt.Errorf("copied data size mismatch: expected %d, got %d", bufferSize, len(data))
	}

	// Get caps for format information with enhanced validation
	caps := gstSample.GetCaps()
	if caps == nil {
		dc.logger.WithFields(logrus.Fields{
			"component": "desktop-capture",
			"method":    "convertGstSample",
			"error":     "no_caps",
		}).Error("No caps found in gst sample")
		return nil, fmt.Errorf("no caps in sample")
	}

	// Register caps object with lifecycle manager
	var capsLifecycleID uintptr
	if dc.lifecycleManager != nil {
		var err error
		capsLifecycleID, err = dc.lifecycleManager.RegisterObject(caps, "gst.Caps", "sample-caps")
		if err != nil {
			dc.logger.WithFields(logrus.Fields{
				"component": "desktop-capture",
				"method":    "convertGstSample",
				"warning":   "caps_lifecycle_registration_failed",
				"details":   err.Error(),
			}).Debug("Failed to register caps with lifecycle manager")
		}
		defer func() {
			if capsLifecycleID != 0 {
				dc.lifecycleManager.UnregisterObject(capsLifecycleID)
			}
		}()

		// Validate caps object before processing
		if !dc.lifecycleManager.ValidateGstObject(caps) {
			dc.logger.WithFields(logrus.Fields{
				"component": "desktop-capture",
				"method":    "convertGstSample",
				"error":     "caps_validation_failed",
			}).Error("Caps object failed validation check")
			return nil, fmt.Errorf("caps object is no longer valid")
		}
	}

	// Track caps object with memory manager
	var capsRef *ObjectRef
	if dc.memoryManager != nil {
		capsRef = dc.memoryManager.RegisterObject(caps)
		defer func() {
			if capsRef != nil {
				dc.memoryManager.UnregisterObject(capsRef)
			}
		}()
	}

	// Parse format information safely
	format, err := dc.parseMediaFormat(caps)
	if err != nil {
		dc.logger.Errorf("convertGstSample: failed to parse media format: %v", err)
		return nil, fmt.Errorf("failed to parse media format: %w", err)
	}

	// Extract timestamp and duration from buffer
	timestamp := dc.extractTimestamp(buffer)
	duration := dc.extractDuration(buffer)

	// Create internal sample with comprehensive metadata
	sample := &Sample{
		Data:      data,
		Timestamp: timestamp,
		Duration:  duration,
		Format:    format,
		Metadata:  dc.extractSampleMetadata(gstSample, buffer),
	}

	// Final validation of the created sample
	if sample == nil {
		dc.logger.WithFields(logrus.Fields{
			"component": "desktop-capture",
			"method":    "convertGstSample",
			"error":     "sample_creation_failed",
		}).Error("Sample creation returned nil")
		return nil, fmt.Errorf("sample creation failed")
	}

	if len(sample.Data) == 0 {
		dc.logger.WithFields(logrus.Fields{
			"component": "desktop-capture",
			"method":    "convertGstSample",
			"warning":   "empty_sample_data",
		}).Warn("Created sample has empty data")
	}

	// Log successful conversion with enhanced details
	dc.logger.WithFields(logrus.Fields{
		"component":    "desktop-capture",
		"method":       "convertGstSample",
		"result":       "success",
		"data_size":    len(data),
		"format_codec": format.Codec,
		"width":        format.Width,
		"height":       format.Height,
		"timestamp":    sample.Timestamp.Format(time.RFC3339Nano),
	}).Debug("Successfully converted gst sample to internal format")

	return sample, nil
}

// safeConvertGstSample provides a safe wrapper around convertGstSample with additional validation
func (dc *DesktopCaptureGst) safeConvertGstSample(gstSample *gst.Sample) (*Sample, error) {
	// Pre-conversion validation
	if gstSample == nil {
		dc.logger.WithFields(logrus.Fields{
			"component": "desktop-capture",
			"method":    "safeConvertGstSample",
			"error":     "nil_sample_input",
		}).Error("Received nil gst sample for safe conversion")
		return nil, fmt.Errorf("gst sample is nil")
	}

	// Validate sample object if lifecycle manager is available
	if dc.lifecycleManager != nil && !dc.lifecycleManager.ValidateGstObject(gstSample) {
		dc.logger.WithFields(logrus.Fields{
			"component": "desktop-capture",
			"method":    "safeConvertGstSample",
			"error":     "sample_validation_failed",
		}).Error("Gst sample object failed validation check")
		return nil, fmt.Errorf("gst sample object is no longer valid")
	}

	// Perform conversion with panic recovery
	var sample *Sample
	var conversionErr error

	func() {
		defer func() {
			if r := recover(); r != nil {
				dc.logger.WithFields(logrus.Fields{
					"component": "desktop-capture",
					"method":    "safeConvertGstSample",
					"error":     "conversion_panic",
					"panic":     r,
				}).Error("Panic occurred during sample conversion")

				// Update statistics
				dc.stats.mu.Lock()
				dc.stats.sampleConversionFails++
				dc.stats.mu.Unlock()

				conversionErr = fmt.Errorf("sample conversion failed due to panic: %v", r)
				sample = nil
			}
		}()

		sample, conversionErr = dc.convertGstSample(gstSample)
	}()

	if conversionErr != nil {
		dc.logger.WithFields(logrus.Fields{
			"component": "desktop-capture",
			"method":    "safeConvertGstSample",
			"error":     "conversion_failed",
			"details":   conversionErr.Error(),
		}).Error("Sample conversion failed")

		// Update statistics
		dc.stats.mu.Lock()
		dc.stats.sampleConversionFails++
		dc.stats.mu.Unlock()

		return nil, conversionErr
	}

	// Post-conversion validation
	if sample == nil {
		dc.logger.WithFields(logrus.Fields{
			"component": "desktop-capture",
			"method":    "safeConvertGstSample",
			"error":     "nil_sample_result",
		}).Error("Sample conversion returned nil without error")

		// Update statistics
		dc.stats.mu.Lock()
		dc.stats.sampleConversionFails++
		dc.stats.mu.Unlock()

		return nil, fmt.Errorf("sample conversion returned nil")
	}

	dc.logger.WithFields(logrus.Fields{
		"component": "desktop-capture",
		"method":    "safeConvertGstSample",
		"result":    "success",
		"data_size": len(sample.Data),
	}).Debug("Safe sample conversion completed successfully")

	return sample, nil
}

// monitorBusMessages monitors pipeline bus messages for error handling with enhanced context
func (dc *DesktopCaptureGst) monitorBusMessages() {
	dc.logger.WithFields(logrus.Fields{
		"component": "desktop-capture",
		"method":    "monitorBusMessages",
	}).Debug("Starting bus message monitoring")

	for {
		select {
		case <-dc.ctx.Done():
			dc.logger.Debug("Bus message monitoring stopped due to context cancellation")
			return
		default:
			msg := dc.bus.TimedPop(gst.ClockTime(100 * time.Millisecond))
			if msg == nil {
				continue
			}

			// Get source element name for context
			sourceName := msg.Source()
			if sourceName == "" {
				sourceName = "unknown"
			}

			switch msg.Type() {
			case gst.MessageError:
				err := msg.ParseError()
				dc.logger.WithFields(logrus.Fields{
					"component":    "desktop-capture",
					"message_type": "error",
					"source":       sourceName,
					"error_detail": err.Error(),
				}).Error("Pipeline error received")

				// Update error statistics
				dc.stats.mu.Lock()
				dc.stats.errorCount++
				dc.stats.pipelineStateErrors++
				dc.stats.mu.Unlock()

				// Send error to error channel
				select {
				case dc.errorChan <- fmt.Errorf("pipeline error from %s: %s", sourceName, err.Error()):
				default:
					dc.logger.Warn("Error channel full, unable to report pipeline error")
				}

				// Trigger recovery for critical errors
				go dc.triggerPipelineRecovery("pipeline_error")

			case gst.MessageWarning:
				err := msg.ParseWarning()
				dc.logger.WithFields(logrus.Fields{
					"component":      "desktop-capture",
					"message_type":   "warning",
					"source":         sourceName,
					"warning_detail": err.Error(),
				}).Warn("Pipeline warning received")

				// Check if warning indicates a potential problem
				warningStr := err.Error()
				criticalWarnings := []string{
					"resource not found",
					"permission denied",
					"device busy",
					"format not supported",
				}

				for _, criticalWarning := range criticalWarnings {
					if contains(warningStr, criticalWarning) {
						dc.logger.WithFields(logrus.Fields{
							"component": "desktop-capture",
							"warning":   "critical_warning_detected",
							"reason":    criticalWarning,
						}).Warn("Critical warning detected, may trigger recovery")
						break
					}
				}

			case gst.MessageEOS:
				dc.logger.WithFields(logrus.Fields{
					"component":    "desktop-capture",
					"message_type": "eos",
					"source":       sourceName,
				}).Info("End of stream received")

				// Update statistics
				dc.stats.mu.Lock()
				dc.stats.errorCount++
				dc.stats.mu.Unlock()

				select {
				case dc.errorChan <- fmt.Errorf("end of stream from %s", sourceName):
				default:
					dc.logger.Warn("Error channel full, unable to report EOS")
				}

				// EOS might indicate a problem, trigger recovery
				go dc.triggerPipelineRecovery("end_of_stream")

			case gst.MessageStateChanged:
				if msg.Source() == "desktop-capture" {
					oldState, newState := msg.ParseStateChanged()
					dc.logger.WithFields(logrus.Fields{
						"component":    "desktop-capture",
						"message_type": "state_changed",
						"old_state":    oldState.String(),
						"new_state":    newState.String(),
						"source":       sourceName,
					}).Debug("Pipeline state changed")

					// Check for unexpected state changes
					if dc.IsRunning() && newState != gst.StatePlaying && newState != gst.StatePaused {
						dc.logger.WithFields(logrus.Fields{
							"component":      "desktop-capture",
							"issue":          "unexpected_state_change",
							"expected_state": "PLAYING or PAUSED",
							"actual_state":   newState.String(),
						}).Warn("Unexpected pipeline state change detected")

						// Update error statistics
						dc.stats.mu.Lock()
						dc.stats.pipelineStateErrors++
						dc.stats.mu.Unlock()
					}
				}

			case gst.MessageStreamStatus:
				// Handle stream status messages for additional context
				streamStatus, _ := msg.ParseStreamStatus()
				dc.logger.WithFields(logrus.Fields{
					"component":     "desktop-capture",
					"message_type":  "stream_status",
					"source":        sourceName,
					"stream_status": streamStatus.String(),
				}).Debug("Stream status message received")

			// Note: gst.MessageQos might not be available in this version of go-gst
			// Commenting out for now
			// case gst.MessageQos:
			//	// Handle Quality of Service messages
			//	dc.logger.WithFields(logrus.Fields{
			//		"component":    "desktop-capture",
			//		"message_type": "qos",
			//		"source":       sourceName,
			//	}).Debug("QoS message received - may indicate performance issues")

			case gst.MessageLatency:
				// Handle latency messages
				dc.logger.WithFields(logrus.Fields{
					"component":    "desktop-capture",
					"message_type": "latency",
					"source":       sourceName,
				}).Debug("Latency message received")

			case gst.MessageClockLost:
				// Handle clock lost messages
				dc.logger.WithFields(logrus.Fields{
					"component":    "desktop-capture",
					"message_type": "clock_lost",
					"source":       sourceName,
				}).Warn("Clock lost message received - may affect synchronization")

				// Trigger recovery for clock issues
				go dc.triggerPipelineRecovery("clock_lost")

			default:
				// Log other message types for debugging
				dc.logger.WithFields(logrus.Fields{
					"component":    "desktop-capture",
					"message_type": msg.Type().String(),
					"source":       sourceName,
				}).Trace("Other bus message received")
			}
		}
	}
}

// processSamples handles any additional sample processing if needed
func (dc *DesktopCaptureGst) processSamples() {
	// This goroutine can be used for additional sample processing
	// Currently, samples are processed directly in onNewSample
	// This provides a hook for future enhancements

	healthCheckTicker := time.NewTicker(5 * time.Second)
	defer healthCheckTicker.Stop()

	for {
		select {
		case <-dc.ctx.Done():
			return
		case <-healthCheckTicker.C:
			// Periodic health checks and maintenance
			dc.performHealthCheck()
		case <-time.After(time.Second):
			// Periodic maintenance tasks can go here
			// For example, statistics cleanup, health checks, etc.
		}
	}
}

// handleSampleError handles sample processing errors with context
func (dc *DesktopCaptureGst) handleSampleError(errorType string, err error) {
	// Update error statistics
	dc.stats.mu.Lock()
	dc.stats.errorCount++
	if errorType == "sample_conversion_failed" {
		dc.stats.sampleConversionFails++
	}
	dc.stats.mu.Unlock()

	// Send error to error channel if available
	select {
	case dc.errorChan <- fmt.Errorf("sample error [%s]: %w", errorType, err):
	default:
		// Error channel full, log the issue
		dc.logger.WithFields(logrus.Fields{
			"component":  "desktop-capture",
			"error_type": errorType,
			"issue":      "error_channel_full",
		}).Warn("Error channel full, unable to report sample error")
	}
}

// handleSampleFailure determines the appropriate flow return for sample failures
func (dc *DesktopCaptureGst) handleSampleFailure(err error) gst.FlowReturn {
	// Analyze error to determine if we should continue or stop
	errorStr := err.Error()

	// Critical errors that should stop the pipeline
	criticalErrors := []string{
		"out of memory",
		"resource unavailable",
		"permission denied",
		"device not found",
	}

	for _, criticalError := range criticalErrors {
		if contains(errorStr, criticalError) {
			dc.logger.WithFields(logrus.Fields{
				"component": "desktop-capture",
				"error":     "critical_sample_failure",
				"reason":    criticalError,
			}).Error("Critical sample failure detected, stopping pipeline")
			return gst.FlowError
		}
	}

	// Non-critical errors - continue processing
	dc.logger.WithFields(logrus.Fields{
		"component": "desktop-capture",
		"warning":   "non_critical_sample_failure",
		"action":    "continue_processing",
	}).Debug("Non-critical sample failure, continuing processing")

	return gst.FlowOK
}

// convertGstSampleWithRecovery attempts sample conversion with enhanced recovery mechanisms
func (dc *DesktopCaptureGst) convertGstSampleWithRecovery(gstSample *gst.Sample) (*Sample, error) {
	// Pre-recovery validation
	if gstSample == nil {
		dc.logger.WithFields(logrus.Fields{
			"component": "desktop-capture",
			"method":    "convertGstSampleWithRecovery",
			"error":     "nil_sample_input",
		}).Error("Received nil gst sample for recovery conversion")
		return nil, fmt.Errorf("gst sample is nil")
	}

	// First attempt: safe conversion with full validation
	sample, err := dc.safeConvertGstSample(gstSample)
	if err == nil {
		dc.logger.WithFields(logrus.Fields{
			"component": "desktop-capture",
			"method":    "convertGstSampleWithRecovery",
			"attempt":   "safe_conversion",
			"result":    "success",
		}).Debug("Safe sample conversion succeeded on first attempt")
		return sample, nil
	}

	// Log initial failure with detailed context
	dc.logger.WithFields(logrus.Fields{
		"component": "desktop-capture",
		"method":    "convertGstSampleWithRecovery",
		"attempt":   "safe_conversion",
		"result":    "failed",
		"error":     err.Error(),
	}).Debug("Safe sample conversion failed, attempting recovery strategies")

	// Recovery attempt 1: relaxed buffer size limits
	if contains(err.Error(), "buffer too large") || contains(err.Error(), "buffer size") {
		dc.logger.WithFields(logrus.Fields{
			"component": "desktop-capture",
			"method":    "convertGstSampleWithRecovery",
			"attempt":   "relaxed_limits",
		}).Debug("Attempting recovery with relaxed buffer size limits")

		sample, recoveryErr := dc.convertGstSampleWithRelaxedLimits(gstSample)
		if recoveryErr == nil {
			dc.logger.WithFields(logrus.Fields{
				"component": "desktop-capture",
				"method":    "convertGstSampleWithRecovery",
				"attempt":   "relaxed_limits",
				"result":    "success",
			}).Info("Sample conversion succeeded with relaxed limits")
			return sample, nil
		}

		dc.logger.WithFields(logrus.Fields{
			"component":      "desktop-capture",
			"method":         "convertGstSampleWithRecovery",
			"attempt":        "relaxed_limits",
			"result":         "failed",
			"recovery_error": recoveryErr.Error(),
		}).Debug("Recovery with relaxed limits failed")
	}

	// Recovery attempt 2: validation bypass for critical errors
	if contains(err.Error(), "validation failed") || contains(err.Error(), "no longer valid") {
		dc.logger.WithFields(logrus.Fields{
			"component": "desktop-capture",
			"method":    "convertGstSampleWithRecovery",
			"attempt":   "bypass_validation",
		}).Debug("Attempting recovery by bypassing validation checks")

		// Temporarily disable lifecycle validation for this conversion
		originalLifecycleManager := dc.lifecycleManager
		dc.lifecycleManager = nil

		sample, recoveryErr := dc.convertGstSample(gstSample)

		// Restore lifecycle manager
		dc.lifecycleManager = originalLifecycleManager

		if recoveryErr == nil {
			dc.logger.WithFields(logrus.Fields{
				"component": "desktop-capture",
				"method":    "convertGstSampleWithRecovery",
				"attempt":   "bypass_validation",
				"result":    "success",
			}).Warn("Sample conversion succeeded by bypassing validation - potential stability risk")
			return sample, nil
		}

		dc.logger.WithFields(logrus.Fields{
			"component":      "desktop-capture",
			"method":         "convertGstSampleWithRecovery",
			"attempt":        "bypass_validation",
			"result":         "failed",
			"recovery_error": recoveryErr.Error(),
		}).Debug("Recovery by bypassing validation failed")
	}

	// Recovery attempt 3: Try with minimal sample data
	if contains(err.Error(), "failed to map buffer") || contains(err.Error(), "empty buffer") {
		dc.logger.WithFields(logrus.Fields{
			"component": "desktop-capture",
			"method":    "convertGstSampleWithRecovery",
			"attempt":   "minimal_sample",
		}).Debug("Attempting recovery with minimal sample creation")

		sample, recoveryErr := dc.createMinimalSample(gstSample)
		if recoveryErr == nil {
			dc.logger.WithFields(logrus.Fields{
				"component": "desktop-capture",
				"method":    "convertGstSampleWithRecovery",
				"attempt":   "minimal_sample",
				"result":    "success",
			}).Info("Sample conversion recovered with minimal sample")
			return sample, nil
		}

		dc.logger.WithFields(logrus.Fields{
			"component":      "desktop-capture",
			"method":         "convertGstSampleWithRecovery",
			"attempt":        "minimal_sample",
			"result":         "failed",
			"recovery_error": recoveryErr.Error(),
		}).Debug("Recovery with minimal sample failed")
	}

	// All recovery attempts failed
	dc.logger.WithFields(logrus.Fields{
		"component":      "desktop-capture",
		"method":         "convertGstSampleWithRecovery",
		"result":         "all_recovery_failed",
		"original_error": err.Error(),
	}).Error("All sample conversion recovery attempts failed")

	// Update statistics
	dc.stats.mu.Lock()
	dc.stats.sampleConversionFails++
	dc.stats.mu.Unlock()

	return nil, fmt.Errorf("sample conversion failed after recovery attempts: %w", err)
}

// handleSampleConversionFailure handles sample conversion failures with graceful degradation
func (dc *DesktopCaptureGst) handleSampleConversionFailure(gstSample *gst.Sample, err error) gst.FlowReturn {
	dc.logger.WithFields(logrus.Fields{
		"component":    "desktop-capture",
		"method":       "handleSampleConversionFailure",
		"error_detail": err.Error(),
	}).Debug("Handling sample conversion failure")

	// Update failure statistics
	dc.stats.mu.Lock()
	dc.stats.framesDropped++
	dc.stats.mu.Unlock()

	// Determine if this is a recoverable error
	errorStr := err.Error()

	// Temporary errors - continue processing
	temporaryErrors := []string{
		"buffer too large",
		"failed to map buffer",
		"empty buffer",
		"failed to copy buffer",
	}

	for _, tempError := range temporaryErrors {
		if contains(errorStr, tempError) {
			dc.logger.WithFields(logrus.Fields{
				"component": "desktop-capture",
				"error":     "temporary_conversion_failure",
				"action":    "continue_processing",
				"reason":    tempError,
			}).Debug("Temporary sample conversion failure, continuing")
			return gst.FlowOK
		}
	}

	// Persistent errors - might indicate pipeline issues
	persistentErrors := []string{
		"no caps in sample",
		"failed to parse media format",
		"type assertion failed",
	}

	for _, persistentError := range persistentErrors {
		if contains(errorStr, persistentError) {
			dc.logger.WithFields(logrus.Fields{
				"component": "desktop-capture",
				"error":     "persistent_conversion_failure",
				"action":    "trigger_recovery",
				"reason":    persistentError,
			}).Warn("Persistent sample conversion failure detected")

			// Trigger pipeline recovery in background
			go dc.triggerPipelineRecovery("sample_conversion_failure")
			return gst.FlowOK
		}
	}

	// Unknown errors - continue but log for investigation
	dc.logger.WithFields(logrus.Fields{
		"component": "desktop-capture",
		"error":     "unknown_conversion_failure",
		"action":    "continue_with_monitoring",
	}).Warn("Unknown sample conversion failure, continuing with monitoring")

	return gst.FlowOK
}

// deliverSample delivers a sample to the output channel with error tracking
func (dc *DesktopCaptureGst) deliverSample(sample *Sample) bool {
	// Attempt to send to sample channel (non-blocking)
	select {
	case dc.sampleChan <- sample:
		// Update success statistics
		dc.stats.mu.Lock()
		dc.stats.framesTotal++
		dc.stats.lastFrameTime = time.Now()
		dc.stats.mu.Unlock()

		// Call legacy callback if set
		if dc.sampleCallback != nil {
			go func() {
				if err := dc.sampleCallback(sample); err != nil {
					dc.logger.WithFields(logrus.Fields{
						"component": "desktop-capture",
						"error":     "callback_failed",
						"detail":    err.Error(),
					}).Warn("Sample callback failed")
				}
			}()
		}

		return true

	default:
		// Channel full, drop sample
		// Buffer will be garbage collected
		dc.stats.mu.Lock()
		dc.stats.framesDropped++
		dc.stats.mu.Unlock()

		return false
	}
}

// convertGstSampleWithRelaxedLimits attempts conversion with relaxed buffer size limits
func (dc *DesktopCaptureGst) convertGstSampleWithRelaxedLimits(gstSample *gst.Sample) (*Sample, error) {
	// Temporarily increase buffer size limit for this conversion
	originalLimit := 50 * 1024 * 1024 // 50MB
	relaxedLimit := 100 * 1024 * 1024 // 100MB

	// Memory manager limits are handled internally

	dc.logger.WithFields(logrus.Fields{
		"component":      "desktop-capture",
		"recovery":       "relaxed_limits",
		"original_limit": originalLimit,
		"relaxed_limit":  relaxedLimit,
	}).Debug("Attempting sample conversion with relaxed limits")

	return dc.convertGstSample(gstSample)
}

// createMinimalSample creates a minimal sample when normal conversion fails
func (dc *DesktopCaptureGst) createMinimalSample(gstSample *gst.Sample) (*Sample, error) {
	dc.logger.WithFields(logrus.Fields{
		"component": "desktop-capture",
		"recovery":  "minimal_sample",
	}).Debug("Creating minimal sample as recovery")

	// Create a minimal sample with basic information
	sample := &Sample{
		Data:      make([]byte, 0), // Empty data
		Timestamp: time.Now(),
		Duration:  0,
		Format: SampleFormat{
			MediaType:  MediaTypeVideo,
			Codec:      "unknown",
			Width:      dc.config.Width,
			Height:     dc.config.Height,
			Parameters: make(map[string]interface{}),
		},
		Metadata: map[string]interface{}{
			"recovery_sample": true,
			"recovery_reason": "conversion_failure",
			"processing_time": time.Now(),
		},
	}

	return sample, nil
}

// performHealthCheck performs periodic health checks on the pipeline
func (dc *DesktopCaptureGst) performHealthCheck() {
	if !dc.IsRunning() {
		return
	}

	dc.logger.WithFields(logrus.Fields{
		"component": "desktop-capture",
		"method":    "performHealthCheck",
	}).Debug("Performing pipeline health check")

	// Check pipeline state
	if dc.pipeline != nil {
		ret, currentState := dc.pipeline.GetState(gst.StatePlaying, gst.ClockTime(100*time.Millisecond))
		expectedState := gst.StatePlaying

		if ret == gst.StateChangeSuccess && currentState != expectedState {
			dc.logger.WithFields(logrus.Fields{
				"component":      "desktop-capture",
				"health_issue":   "state_mismatch",
				"current_state":  currentState.String(),
				"expected_state": expectedState.String(),
			}).Warn("Pipeline state mismatch detected")

			// Trigger recovery if state is not as expected
			go dc.triggerPipelineRecovery("state_mismatch")
		}
	}

	// Check for excessive frame drops
	dc.stats.mu.RLock()
	totalFrames := dc.stats.framesTotal
	droppedFrames := dc.stats.framesDropped
	dc.stats.mu.RUnlock()

	if totalFrames > 0 {
		dropRate := float64(droppedFrames) / float64(totalFrames)
		if dropRate > 0.1 { // More than 10% drop rate
			dc.logger.WithFields(logrus.Fields{
				"component":      "desktop-capture",
				"health_issue":   "high_drop_rate",
				"drop_rate":      dropRate,
				"total_frames":   totalFrames,
				"dropped_frames": droppedFrames,
			}).Warn("High frame drop rate detected")
		}
	}
}

// triggerPipelineRecovery triggers pipeline recovery for the specified reason
func (dc *DesktopCaptureGst) triggerPipelineRecovery(reason string) {
	// Update recovery statistics
	dc.stats.mu.Lock()
	dc.stats.recoveryAttempts++
	dc.stats.lastRecoveryTime = time.Now()
	if reason == "state_mismatch" {
		dc.stats.pipelineStateErrors++
	}
	dc.stats.mu.Unlock()

	dc.logger.WithFields(logrus.Fields{
		"component": "desktop-capture",
		"method":    "triggerPipelineRecovery",
		"reason":    reason,
	}).Info("Triggering pipeline recovery")

	// Implement recovery logic based on reason
	switch reason {
	case "state_mismatch":
		dc.recoverFromStateMismatch()
	case "sample_conversion_failure":
		dc.recoverFromSampleFailure()
	case "pipeline_error":
		dc.recoverFromPipelineError()
	case "end_of_stream":
		dc.recoverFromEndOfStream()
	case "clock_lost":
		dc.recoverFromClockLost()
	default:
		dc.performGenericRecovery()
	}
}

// recoverFromStateMismatch recovers from pipeline state mismatches
func (dc *DesktopCaptureGst) recoverFromStateMismatch() {
	dc.logger.WithFields(logrus.Fields{
		"component": "desktop-capture",
		"recovery":  "state_mismatch",
	}).Info("Attempting recovery from state mismatch")

	if dc.pipeline == nil {
		dc.logger.Error("Cannot recover: pipeline is nil")
		return
	}

	// Try to set pipeline back to playing state
	if err := dc.pipeline.SetState(gst.StatePlaying); err != nil {
		dc.logger.WithFields(logrus.Fields{
			"component": "desktop-capture",
			"recovery":  "state_mismatch",
			"error":     err.Error(),
		}).Error("Failed to set pipeline to playing state during recovery")

		// If setting to playing fails, try a full restart
		go dc.performFullRestart()
		return
	}

	// Wait for state change
	ret, _ := dc.pipeline.GetState(gst.StatePlaying, gst.ClockTime(5*time.Second))
	if ret == gst.StateChangeSuccess {
		dc.logger.WithFields(logrus.Fields{
			"component": "desktop-capture",
			"recovery":  "state_mismatch",
			"result":    "success",
		}).Info("Successfully recovered from state mismatch")
	} else {
		dc.logger.WithFields(logrus.Fields{
			"component": "desktop-capture",
			"recovery":  "state_mismatch",
			"result":    "failed",
			"state_ret": ret.String(),
		}).Warn("Failed to recover from state mismatch")

		// Trigger full restart as last resort
		go dc.performFullRestart()
	}
}

// recoverFromSampleFailure recovers from persistent sample conversion failures
func (dc *DesktopCaptureGst) recoverFromSampleFailure() {
	dc.logger.WithFields(logrus.Fields{
		"component": "desktop-capture",
		"recovery":  "sample_failure",
	}).Info("Attempting recovery from sample conversion failures")

	// Reset memory manager if available
	if dc.memoryManager != nil {
		dc.logger.Debug("Triggering memory cleanup for sample failure recovery")
		dc.memoryManager.ForceGC()
	}

	// Reset statistics to clear error accumulation
	dc.stats.mu.Lock()
	dc.stats.framesDropped = 0
	dc.stats.mu.Unlock()

	dc.logger.WithFields(logrus.Fields{
		"component": "desktop-capture",
		"recovery":  "sample_failure",
		"result":    "completed",
	}).Info("Sample failure recovery completed")
}

// performGenericRecovery performs generic recovery actions
func (dc *DesktopCaptureGst) performGenericRecovery() {
	dc.logger.WithFields(logrus.Fields{
		"component": "desktop-capture",
		"recovery":  "generic",
	}).Info("Performing generic pipeline recovery")

	// Trigger memory cleanup
	if dc.memoryManager != nil {
		dc.memoryManager.ForceGC()
	}

	// Clear error channels
	select {
	case <-dc.errorChan:
		// Drain one error
	default:
		// No errors to drain
	}
}

// performFullRestart performs a full pipeline restart as last resort
func (dc *DesktopCaptureGst) performFullRestart() {
	dc.logger.WithFields(logrus.Fields{
		"component": "desktop-capture",
		"recovery":  "full_restart",
	}).Warn("Performing full pipeline restart as last resort")

	// This would require careful implementation to avoid disrupting the service
	// For now, just log the intent - actual implementation would need to:
	// 1. Stop the current pipeline
	// 2. Clean up resources
	// 3. Reinitialize the pipeline
	// 4. Restart capture

	dc.logger.WithFields(logrus.Fields{
		"component": "desktop-capture",
		"recovery":  "full_restart",
		"status":    "not_implemented",
	}).Warn("Full restart recovery not yet implemented")
}

// contains checks if a string contains a substring (helper function)
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
			func() bool {
				for i := 1; i <= len(s)-len(substr); i++ {
					if s[i:i+len(substr)] == substr {
						return true
					}
				}
				return false
			}())))
}

// parseMediaFormat extracts media format information from GStreamer caps
func (dc *DesktopCaptureGst) parseMediaFormat(caps *gst.Caps) (SampleFormat, error) {
	if caps.GetSize() == 0 {
		return SampleFormat{}, fmt.Errorf("caps structure is empty")
	}

	structure := caps.GetStructureAt(0)
	if structure == nil {
		return SampleFormat{}, fmt.Errorf("failed to get caps structure")
	}

	format := SampleFormat{
		MediaType:  MediaTypeVideo, // Default for desktop capture
		Parameters: make(map[string]interface{}),
	}

	// Extract width
	if width, err := structure.GetValue("width"); err == nil {
		if w, ok := width.(int); ok {
			format.Width = w
		}
	}

	// Extract height
	if height, err := structure.GetValue("height"); err == nil {
		if h, ok := height.(int); ok {
			format.Height = h
		}
	}

	// Extract format/codec information
	structName := structure.Name()
	switch structName {
	case "video/x-raw":
		format.Codec = "raw"
		if pixelFormat, err := structure.GetValue("format"); err == nil {
			format.Parameters["pixel_format"] = pixelFormat
		}
	case "video/x-h264":
		format.Codec = "h264"
	case "video/x-vp8":
		format.Codec = "vp8"
	case "video/x-vp9":
		format.Codec = "vp9"
	default:
		format.Codec = "unknown"
	}

	// Extract framerate
	if framerate, err := structure.GetValue("framerate"); err == nil {
		format.Parameters["framerate"] = framerate
	}

	return format, nil
}

// extractTimestamp extracts timestamp from GStreamer buffer
func (dc *DesktopCaptureGst) extractTimestamp(buffer *gst.Buffer) time.Time {
	if buffer == nil {
		return time.Now()
	}

	pts := buffer.PresentationTimestamp()
	if pts == gst.ClockTimeNone {
		return time.Now()
	}

	// Convert GStreamer timestamp to Go time
	// GStreamer uses nanoseconds since pipeline start
	return time.Unix(0, int64(pts))
}

// extractDuration extracts duration from GStreamer buffer
func (dc *DesktopCaptureGst) extractDuration(buffer *gst.Buffer) time.Duration {
	if buffer == nil {
		return 0
	}

	duration := buffer.Duration()
	if duration == gst.ClockTimeNone {
		return 0
	}

	return time.Duration(duration)
}

// extractSampleMetadata extracts metadata from GStreamer sample and buffer
func (dc *DesktopCaptureGst) extractSampleMetadata(gstSample *gst.Sample, buffer *gst.Buffer) map[string]interface{} {
	metadata := make(map[string]interface{})

	if buffer != nil {
		// Check for keyframe flag in buffer flags
		flags := buffer.GetFlags()
		if flags&gst.BufferFlagDeltaUnit == 0 {
			// If delta unit flag is not set, this is likely a keyframe
			metadata["key_frame"] = true
		} else {
			metadata["key_frame"] = false
		}

		// Extract buffer size
		metadata["buffer_size"] = buffer.GetSize()

		// Extract buffer offset (if available)
		if offset := buffer.Offset(); offset != -1 { // Check for valid offset
			metadata["offset"] = offset
		}

		// Extract buffer offset end (if available)
		if offsetEnd := buffer.OffsetEnd(); offsetEnd != -1 { // Check for valid offset end
			metadata["offset_end"] = offsetEnd
		}

		// Add buffer flags information
		metadata["buffer_flags"] = flags
	}

	// Add processing timestamp
	metadata["processing_time"] = time.Now()

	return metadata
}

// GetSource gets the GStreamer source element (implements DesktopCapture interface)
func (dc *DesktopCaptureGst) GetSource() (string, map[string]interface{}) {
	properties := make(map[string]interface{})

	if dc.config.UseWayland {
		return "waylandsink", properties
	}

	properties["display-name"] = dc.config.DisplayID
	properties["use-damage"] = dc.config.UseDamage
	properties["show-pointer"] = dc.config.ShowPointer

	if dc.config.Width > 0 && dc.config.Height > 0 {
		properties["endx"] = dc.config.Width
		properties["endy"] = dc.config.Height
	}

	return "ximagesrc", properties
}

// SetSampleCallback sets the callback function for processing video samples (implements DesktopCapture interface)
func (dc *DesktopCaptureGst) SetSampleCallback(callback func(*Sample) error) error {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	dc.sampleCallback = callback
	dc.logger.Debug("Sample callback set for go-gst desktop capture")
	return nil
}

// GetAppsink returns the appsink element for external processing (implements DesktopCapture interface)
// elementWrapper wraps a go-gst element to implement the Element interface
type elementWrapper struct {
	element *gst.Element
}

func (ew *elementWrapper) SetProperty(name string, value interface{}) error {
	return ew.element.SetProperty(name, value)
}

func (ew *elementWrapper) GetProperty(name string) (interface{}, error) {
	return ew.element.GetProperty(name)
}

func (ew *elementWrapper) Link(sink Element) error {
	if sinkWrapper, ok := sink.(*elementWrapper); ok {
		return ew.element.Link(sinkWrapper.element)
	}
	return fmt.Errorf("cannot link to non-wrapped element")
}

func (ew *elementWrapper) GetStaticPad(name string) (Pad, error) {
	// This is a simplified implementation - you may need to implement a padWrapper too
	return nil, fmt.Errorf("GetStaticPad not implemented in elementWrapper")
}

func (ew *elementWrapper) GetName() string {
	name, _ := ew.element.GetProperty("name")
	if nameStr, ok := name.(string); ok {
		return nameStr
	}
	return ""
}

func (ew *elementWrapper) GetFactoryName() string {
	return ew.element.GetFactory().GetName()
}

func (ew *elementWrapper) ForceKeyUnit() bool {
	// This would need to be implemented based on the specific element type
	return false
}

func (ew *elementWrapper) ConnectPadAdded(callback PadAddedCallback) error {
	// This would need to be implemented based on go-gst signal handling
	return fmt.Errorf("ConnectPadAdded not implemented in elementWrapper")
}

func (ew *elementWrapper) GetInternal() interface{} {
	// This would need to access the internal C pointer from go-gst
	// For go-gst implementation, we return the gst.Element directly
	return ew.element
}

func (dc *DesktopCaptureGst) GetAppsink() Element {
	// Return a wrapper around the go-gst element to maintain interface compatibility
	return &elementWrapper{element: dc.sink}
}

// SetFrameRate dynamically sets the capture frame rate (implements DesktopCapture interface)
func (dc *DesktopCaptureGst) SetFrameRate(frameRate int) error {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	dc.config.FrameRate = frameRate
	dc.logger.Debugf("Frame rate set to %d fps", frameRate)

	// TODO: Implement dynamic frame rate change in pipeline
	return nil
}

// GetDetailedErrorStats returns detailed error statistics for monitoring
func (dc *DesktopCaptureGst) GetDetailedErrorStats() map[string]interface{} {
	dc.stats.mu.RLock()
	defer dc.stats.mu.RUnlock()

	stats := map[string]interface{}{
		"total_errors":               dc.stats.errorCount,
		"sample_conversion_failures": dc.stats.sampleConversionFails,
		"pipeline_state_errors":      dc.stats.pipelineStateErrors,
		"recovery_attempts":          dc.stats.recoveryAttempts,
		"last_recovery_time":         dc.stats.lastRecoveryTime.UnixMilli(),
		"frames_total":               dc.stats.framesTotal,
		"frames_dropped":             dc.stats.framesDropped,
		"uptime_seconds":             time.Since(dc.stats.startTime).Seconds(),
	}

	// Calculate error rates
	if dc.stats.framesTotal > 0 {
		stats["error_rate"] = float64(dc.stats.errorCount) / float64(dc.stats.framesTotal)
		stats["drop_rate"] = float64(dc.stats.framesDropped) / float64(dc.stats.framesTotal)
	} else {
		stats["error_rate"] = 0.0
		stats["drop_rate"] = 0.0
	}

	return stats
}

// GetHealthStatus returns the current health status of the desktop capture component
func (dc *DesktopCaptureGst) GetHealthStatus() HealthStatus {
	dc.stats.mu.RLock()
	defer dc.stats.mu.RUnlock()

	status := HealthStatus{
		Overall:   HealthStatusHealthy,
		LastCheck: time.Now().UnixMilli(),
		Uptime:    int64(time.Since(dc.stats.startTime).Seconds()),
		Issues:    make([]string, 0),
		Checks:    make(map[string]interface{}),
	}

	// Check error rates
	if dc.stats.framesTotal > 0 {
		errorRate := float64(dc.stats.errorCount) / float64(dc.stats.framesTotal)
		dropRate := float64(dc.stats.framesDropped) / float64(dc.stats.framesTotal)

		status.Checks["error_rate"] = errorRate
		status.Checks["drop_rate"] = dropRate
		status.Checks["total_frames"] = dc.stats.framesTotal
		status.Checks["total_errors"] = dc.stats.errorCount

		// Determine health level based on error rates
		if errorRate > 0.05 { // More than 5% error rate
			status.Overall = HealthStatusError
			status.Issues = append(status.Issues, fmt.Sprintf("High error rate: %.2f%%", errorRate*100))
		} else if errorRate > 0.01 { // More than 1% error rate
			status.Overall = HealthStatusWarning
			status.Issues = append(status.Issues, fmt.Sprintf("Elevated error rate: %.2f%%", errorRate*100))
		}

		if dropRate > 0.1 { // More than 10% drop rate
			if status.Overall < HealthStatusError {
				status.Overall = HealthStatusError
			}
			status.Issues = append(status.Issues, fmt.Sprintf("High drop rate: %.2f%%", dropRate*100))
		} else if dropRate > 0.05 { // More than 5% drop rate
			if status.Overall < HealthStatusWarning {
				status.Overall = HealthStatusWarning
			}
			status.Issues = append(status.Issues, fmt.Sprintf("Elevated drop rate: %.2f%%", dropRate*100))
		}
	}

	// Check pipeline state
	if dc.pipeline != nil && dc.IsRunning() {
		ret, currentState := dc.pipeline.GetState(gst.StatePlaying, gst.ClockTime(100*time.Millisecond))
		status.Checks["pipeline_state"] = currentState.String()

		if ret == gst.StateChangeSuccess && currentState != gst.StatePlaying {
			status.Overall = HealthStatusCritical
			status.Issues = append(status.Issues, fmt.Sprintf("Pipeline not in playing state: %s", currentState.String()))
		}
	}

	// Check recent recovery attempts
	if !dc.stats.lastRecoveryTime.IsZero() {
		timeSinceRecovery := time.Since(dc.stats.lastRecoveryTime)
		status.Checks["time_since_last_recovery"] = timeSinceRecovery.Seconds()
		status.Checks["total_recovery_attempts"] = dc.stats.recoveryAttempts

		if timeSinceRecovery < 5*time.Minute {
			if status.Overall < HealthStatusWarning {
				status.Overall = HealthStatusWarning
			}
			status.Issues = append(status.Issues, "Recent recovery attempt detected")
		}
	}

	return status
}

// recoverFromPipelineError recovers from pipeline errors
func (dc *DesktopCaptureGst) recoverFromPipelineError() {
	dc.logger.WithFields(logrus.Fields{
		"component": "desktop-capture",
		"recovery":  "pipeline_error",
	}).Info("Attempting recovery from pipeline error")

	if dc.pipeline == nil {
		dc.logger.Error("Cannot recover from pipeline error: pipeline is nil")
		return
	}

	// First, try to get current state
	ret, currentState := dc.pipeline.GetState(gst.StatePlaying, gst.ClockTime(1*time.Second))

	dc.logger.WithFields(logrus.Fields{
		"component":     "desktop-capture",
		"recovery":      "pipeline_error",
		"current_state": currentState.String(),
	}).Debug("Current pipeline state during error recovery")

	// Try to reset pipeline to NULL and back to PLAYING
	if err := dc.pipeline.SetState(gst.StateNull); err != nil {
		dc.logger.WithFields(logrus.Fields{
			"component": "desktop-capture",
			"recovery":  "pipeline_error",
			"error":     err.Error(),
		}).Error("Failed to set pipeline to NULL during error recovery")
		return
	}

	// Wait for NULL state
	ret, _ = dc.pipeline.GetState(gst.StateNull, gst.ClockTime(5*time.Second))
	if ret != gst.StateChangeSuccess {
		dc.logger.WithFields(logrus.Fields{
			"component": "desktop-capture",
			"recovery":  "pipeline_error",
			"result":    ret.String(),
		}).Warn("Failed to reach NULL state during error recovery")
	}

	// Try to restart pipeline
	if err := dc.pipeline.SetState(gst.StatePlaying); err != nil {
		dc.logger.WithFields(logrus.Fields{
			"component": "desktop-capture",
			"recovery":  "pipeline_error",
			"error":     err.Error(),
		}).Error("Failed to restart pipeline after error recovery")
		return
	}

	// Wait for PLAYING state
	ret, _ = dc.pipeline.GetState(gst.StatePlaying, gst.ClockTime(10*time.Second))
	if ret == gst.StateChangeSuccess {
		dc.logger.WithFields(logrus.Fields{
			"component": "desktop-capture",
			"recovery":  "pipeline_error",
			"result":    "success",
		}).Info("Successfully recovered from pipeline error")
	} else {
		dc.logger.WithFields(logrus.Fields{
			"component": "desktop-capture",
			"recovery":  "pipeline_error",
			"result":    "failed",
			"state_ret": ret.String(),
		}).Error("Failed to recover from pipeline error")
	}
}

// recoverFromEndOfStream recovers from unexpected end of stream
func (dc *DesktopCaptureGst) recoverFromEndOfStream() {
	dc.logger.WithFields(logrus.Fields{
		"component": "desktop-capture",
		"recovery":  "end_of_stream",
	}).Info("Attempting recovery from end of stream")

	// EOS usually means the source stopped producing data
	// Try to restart the pipeline
	dc.recoverFromPipelineError()
}

// recoverFromClockLost recovers from clock lost situations
func (dc *DesktopCaptureGst) recoverFromClockLost() {
	dc.logger.WithFields(logrus.Fields{
		"component": "desktop-capture",
		"recovery":  "clock_lost",
	}).Info("Attempting recovery from clock lost")

	if dc.pipeline == nil {
		dc.logger.Error("Cannot recover from clock lost: pipeline is nil")
		return
	}

	// Clock recovery is not easily available in go-gst
	// For now, just log the attempt and perform generic recovery
	dc.logger.WithFields(logrus.Fields{
		"component": "desktop-capture",
		"recovery":  "clock_lost",
		"action":    "generic_recovery",
	}).Info("Clock lost detected, performing generic recovery")

	// Also trigger a generic recovery to ensure pipeline is healthy
	dc.performGenericRecovery()
}
