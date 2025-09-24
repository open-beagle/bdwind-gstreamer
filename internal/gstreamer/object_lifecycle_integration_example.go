package gstreamer

import (
	"fmt"
	"time"

	"github.com/go-gst/go-gst/gst"
	"github.com/sirupsen/logrus"
)

// SafeDesktopCapture demonstrates how to integrate ObjectLifecycleManager
// with existing GStreamer components to prevent segfaults
type SafeDesktopCapture struct {
	// Object tracking
	objectTracker *GstObjectTracker
	logger        *logrus.Entry

	// GStreamer objects with tracked IDs
	pipelineID uintptr
	sourceID   uintptr
	sinkID     uintptr
	busID      uintptr

	// Original objects (kept for reference, but accessed through tracker)
	pipeline *gst.Pipeline
	source   *gst.Element
	sink     *gst.Element
	bus      *gst.Bus
}

// NewSafeDesktopCapture creates a new safe desktop capture with object lifecycle management
func NewSafeDesktopCapture() (*SafeDesktopCapture, error) {
	gst.Init(nil)

	sdc := &SafeDesktopCapture{
		objectTracker: NewGstObjectTracker(),
		logger:        logrus.WithField("component", "safe-desktop-capture"),
	}

	// Enable debug logging for demonstration
	sdc.objectTracker.EnableDebugLogging(true)

	if err := sdc.initializePipeline(); err != nil {
		sdc.Close()
		return nil, fmt.Errorf("failed to initialize pipeline: %w", err)
	}

	return sdc, nil
}

// initializePipeline creates and tracks all GStreamer objects
func (sdc *SafeDesktopCapture) initializePipeline() error {
	var err error

	// Create pipeline
	sdc.pipeline, err = gst.NewPipeline("safe-desktop-capture")
	if err != nil {
		return fmt.Errorf("failed to create pipeline: %w", err)
	}

	// Track pipeline
	sdc.pipelineID, err = sdc.objectTracker.TrackPipeline(sdc.pipeline, "safe-desktop-capture")
	if err != nil {
		return fmt.Errorf("failed to track pipeline: %w", err)
	}

	// Create source element
	sdc.source, err = gst.NewElement("ximagesrc")
	if err != nil {
		return fmt.Errorf("failed to create source element: %w", err)
	}

	// Track source element
	sdc.sourceID, err = sdc.objectTracker.TrackElement(sdc.source, "ximagesrc")
	if err != nil {
		return fmt.Errorf("failed to track source element: %w", err)
	}

	// Create sink element
	sdc.sink, err = gst.NewElement("fakesink")
	if err != nil {
		return fmt.Errorf("failed to create sink element: %w", err)
	}

	// Track sink element
	sdc.sinkID, err = sdc.objectTracker.TrackElement(sdc.sink, "fakesink")
	if err != nil {
		return fmt.Errorf("failed to track sink element: %w", err)
	}

	// Get and track bus
	sdc.bus = sdc.pipeline.GetBus()
	sdc.busID, err = sdc.objectTracker.TrackBus(sdc.bus, "pipeline-bus")
	if err != nil {
		return fmt.Errorf("failed to track bus: %w", err)
	}

	// Add elements to pipeline using safe operations
	err = sdc.safeAddElementsToPipeline()
	if err != nil {
		return fmt.Errorf("failed to add elements to pipeline: %w", err)
	}

	sdc.logger.Info("Pipeline initialized successfully with object lifecycle management")
	return nil
}

// safeAddElementsToPipeline demonstrates safe pipeline operations
func (sdc *SafeDesktopCapture) safeAddElementsToPipeline() error {
	// Use safe pipeline operations
	return sdc.objectTracker.SafeUsePipeline(sdc.pipelineID, func(pipeline *gst.Pipeline) error {
		// Add source element
		if err := pipeline.Add(sdc.source); err != nil {
			return fmt.Errorf("failed to add source to pipeline: %w", err)
		}

		// Add sink element
		if err := pipeline.Add(sdc.sink); err != nil {
			return fmt.Errorf("failed to add sink to pipeline: %w", err)
		}

		// Link elements
		if err := sdc.source.Link(sdc.sink); err != nil {
			return fmt.Errorf("failed to link elements: %w", err)
		}

		return nil
	})
}

// Start starts the desktop capture pipeline safely
func (sdc *SafeDesktopCapture) Start() error {
	if !sdc.objectTracker.IsObjectValid(sdc.pipelineID) {
		return fmt.Errorf("pipeline is not valid")
	}

	return sdc.objectTracker.SafeUsePipeline(sdc.pipelineID, func(pipeline *gst.Pipeline) error {
		err := pipeline.SetState(gst.StatePlaying)
		if err != nil {
			return fmt.Errorf("failed to start pipeline: %w", err)
		}

		sdc.logger.Info("Desktop capture started successfully")
		return nil
	})
}

// Stop stops the desktop capture pipeline safely
func (sdc *SafeDesktopCapture) Stop() error {
	if !sdc.objectTracker.IsObjectValid(sdc.pipelineID) {
		sdc.logger.Warn("Pipeline is not valid, cannot stop")
		return nil
	}

	return sdc.objectTracker.SafeUsePipeline(sdc.pipelineID, func(pipeline *gst.Pipeline) error {
		err := pipeline.SetState(gst.StateNull)
		if err != nil {
			return fmt.Errorf("failed to stop pipeline: %w", err)
		}

		sdc.logger.Info("Desktop capture stopped successfully")
		return nil
	})
}

// ProcessSample demonstrates safe sample processing
func (sdc *SafeDesktopCapture) ProcessSample(sample *gst.Sample) error {
	if sample == nil {
		return fmt.Errorf("sample is nil")
	}

	// Track the sample for safe processing
	sampleID, err := sdc.objectTracker.TrackSample(sample, "desktop-sample")
	if err != nil {
		return fmt.Errorf("failed to track sample: %w", err)
	}

	// Ensure we untrack the sample when done
	defer func() {
		if err := sdc.objectTracker.UntrackObject(sampleID); err != nil {
			sdc.logger.Errorf("Failed to untrack sample: %v", err)
		}
	}()

	// Process sample safely
	return sdc.objectTracker.SafeUseSample(sampleID, func(s *gst.Sample) error {
		// Safe sample processing logic here
		buffer := s.GetBuffer()
		if buffer == nil {
			return fmt.Errorf("sample has no buffer")
		}

		// Process the buffer data...
		sdc.logger.Debugf("Processed sample with buffer size: %d", buffer.GetSize())
		return nil
	})
}

// GetStats returns object lifecycle statistics
func (sdc *SafeDesktopCapture) GetStats() map[string]interface{} {
	stats := sdc.objectTracker.GetStats()
	health := sdc.objectTracker.GetHealthStatus()

	return map[string]interface{}{
		"lifecycle_stats": stats,
		"health_status":   health,
		"active_objects":  len(sdc.objectTracker.ListActiveObjects()),
	}
}

// IsHealthy checks if the desktop capture is in a healthy state
func (sdc *SafeDesktopCapture) IsHealthy() bool {
	// Check if all critical objects are valid
	if !sdc.objectTracker.IsObjectValid(sdc.pipelineID) {
		return false
	}
	if !sdc.objectTracker.IsObjectValid(sdc.sourceID) {
		return false
	}
	if !sdc.objectTracker.IsObjectValid(sdc.sinkID) {
		return false
	}
	if !sdc.objectTracker.IsObjectValid(sdc.busID) {
		return false
	}

	// Check overall health
	health := sdc.objectTracker.GetHealthStatus()
	return health["healthy"].(bool)
}

// RecoverFromError attempts to recover from object lifecycle errors
func (sdc *SafeDesktopCapture) RecoverFromError() error {
	sdc.logger.Warn("Attempting to recover from object lifecycle error")

	// Check which objects are invalid
	invalidObjects := []string{}
	if !sdc.objectTracker.IsObjectValid(sdc.pipelineID) {
		invalidObjects = append(invalidObjects, "pipeline")
	}
	if !sdc.objectTracker.IsObjectValid(sdc.sourceID) {
		invalidObjects = append(invalidObjects, "source")
	}
	if !sdc.objectTracker.IsObjectValid(sdc.sinkID) {
		invalidObjects = append(invalidObjects, "sink")
	}
	if !sdc.objectTracker.IsObjectValid(sdc.busID) {
		invalidObjects = append(invalidObjects, "bus")
	}

	if len(invalidObjects) > 0 {
		sdc.logger.Errorf("Invalid objects detected: %v", invalidObjects)

		// For this example, we'll just log the error
		// In a real implementation, you might recreate the invalid objects
		return fmt.Errorf("cannot recover: invalid objects: %v", invalidObjects)
	}

	sdc.logger.Info("No invalid objects found, system appears healthy")
	return nil
}

// Close safely closes the desktop capture and cleans up all tracked objects
func (sdc *SafeDesktopCapture) Close() {
	sdc.logger.Info("Closing safe desktop capture")

	// Stop pipeline if it's still valid
	if sdc.objectTracker.IsObjectValid(sdc.pipelineID) {
		if err := sdc.Stop(); err != nil {
			sdc.logger.Errorf("Error stopping pipeline: %v", err)
		}
	}

	// Untrack all objects
	if sdc.pipelineID != 0 {
		sdc.objectTracker.UntrackObject(sdc.pipelineID)
	}
	if sdc.sourceID != 0 {
		sdc.objectTracker.UntrackObject(sdc.sourceID)
	}
	if sdc.sinkID != 0 {
		sdc.objectTracker.UntrackObject(sdc.sinkID)
	}
	if sdc.busID != 0 {
		sdc.objectTracker.UntrackObject(sdc.busID)
	}

	// Unref GStreamer objects
	if sdc.pipeline != nil {
		sdc.pipeline.Unref()
	}
	if sdc.source != nil {
		sdc.source.Unref()
	}
	if sdc.sink != nil {
		sdc.sink.Unref()
	}

	// Close object tracker
	sdc.objectTracker.Close()

	sdc.logger.Info("Safe desktop capture closed successfully")
}

// DemoUsage demonstrates how to use the SafeDesktopCapture
func DemoSafeDesktopCaptureUsage() {
	// Create safe desktop capture
	capture, err := NewSafeDesktopCapture()
	if err != nil {
		logrus.Errorf("Failed to create safe desktop capture: %v", err)
		return
	}
	defer capture.Close()

	// Check initial health
	if !capture.IsHealthy() {
		logrus.Error("Desktop capture is not healthy")
		return
	}

	// Start capture
	if err := capture.Start(); err != nil {
		logrus.Errorf("Failed to start capture: %v", err)
		return
	}

	// Run for a short time
	time.Sleep(2 * time.Second)

	// Check stats
	stats := capture.GetStats()
	logrus.Infof("Capture stats: %+v", stats)

	// Stop capture
	if err := capture.Stop(); err != nil {
		logrus.Errorf("Failed to stop capture: %v", err)
		return
	}

	logrus.Info("Demo completed successfully")
}
