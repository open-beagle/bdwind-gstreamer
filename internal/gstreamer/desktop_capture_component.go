package gstreamer

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/go-gst/go-gst/gst"
	"github.com/sirupsen/logrus"

	"github.com/open-beagle/bdwind-gstreamer/internal/config"
)

// DesktopCaptureComponent implements high-cohesion desktop capture using go-gst
// Based on go-gst best practices with ximagesrc/waylandsrc support
// Supports automatic X11/Wayland detection and format conversion
type DesktopCaptureComponent struct {
	// Configuration
	config config.DesktopCaptureConfig
	logger *logrus.Entry

	// go-gst elements
	source     *gst.Element // ximagesrc or waylandsrc
	capsfilter *gst.Element // format filter
	queue      *gst.Element // buffer queue

	// Output management
	outputPad *gst.Pad
	publisher *StreamPublisher

	// Configuration and state
	isRunning bool
	stats     *CaptureStatistics

	// Memory management
	memManager *MemoryManager

	// Lifecycle management
	mu     sync.RWMutex
	ctx    context.Context
	cancel context.CancelFunc
}

// CaptureStatistics tracks desktop capture metrics
type CaptureStatistics struct {
	mu               sync.RWMutex
	FramesTotal      int64     `json:"frames_total"`
	FramesDropped    int64     `json:"frames_dropped"`
	StartTime        time.Time `json:"start_time"`
	LastFrameTime    time.Time `json:"last_frame_time"`
	ErrorCount       int64     `json:"error_count"`
	RecoveryAttempts int64     `json:"recovery_attempts"`
	LastRecoveryTime time.Time `json:"last_recovery_time"`
	CurrentFPS       float64   `json:"current_fps"`
	AverageFPS       float64   `json:"average_fps"`
	CaptureLatency   int64     `json:"capture_latency_ms"`
	BytesProcessed   int64     `json:"bytes_processed"`
}

// NewDesktopCaptureComponent creates a new desktop capture component
func NewDesktopCaptureComponent(cfg config.DesktopCaptureConfig, memManager *MemoryManager, logger *logrus.Entry) (*DesktopCaptureComponent, error) {
	if logger == nil {
		logger = logrus.WithField("component", "desktop-capture")
	}

	ctx, cancel := context.WithCancel(context.Background())

	dc := &DesktopCaptureComponent{
		config:     cfg,
		logger:     logger,
		memManager: memManager,
		ctx:        ctx,
		cancel:     cancel,
		stats: &CaptureStatistics{
			StartTime: time.Now(),
		},
	}

	return dc, nil
}

// Initialize initializes the desktop capture component within a pipeline
func (dc *DesktopCaptureComponent) Initialize(pipeline *gst.Pipeline) error {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	if pipeline == nil {
		return fmt.Errorf("pipeline cannot be nil")
	}

	// Detect desktop environment and create appropriate source
	if err := dc.createSourceElement(); err != nil {
		return fmt.Errorf("failed to create source element: %w", err)
	}

	// Create format filter
	if err := dc.createCapsFilter(); err != nil {
		return fmt.Errorf("failed to create caps filter: %w", err)
	}

	// Create buffer queue
	if err := dc.createQueue(); err != nil {
		return fmt.Errorf("failed to create queue: %w", err)
	}

	// Add elements to pipeline
	elements := []*gst.Element{dc.source, dc.capsfilter, dc.queue}
	for _, element := range elements {
		if err := pipeline.Add(element); err != nil {
			return fmt.Errorf("failed to add element to pipeline: %w", err)
		}
	}

	// Link elements
	if err := dc.linkElements(); err != nil {
		return fmt.Errorf("failed to link elements: %w", err)
	}

	// Get output pad for connection to next component
	dc.outputPad = dc.queue.GetStaticPad("src")
	if dc.outputPad == nil {
		return fmt.Errorf("failed to get output pad from queue")
	}

	dc.logger.Info("Desktop capture component initialized successfully")
	return nil
}

// Start starts the desktop capture component
func (dc *DesktopCaptureComponent) Start() error {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	if dc.isRunning {
		return fmt.Errorf("desktop capture component already running")
	}

	dc.isRunning = true
	dc.stats.StartTime = time.Now()

	dc.logger.Info("Desktop capture component started")
	return nil
}

// Stop stops the desktop capture component
func (dc *DesktopCaptureComponent) Stop() error {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	if !dc.isRunning {
		return nil
	}

	dc.cancel()
	dc.isRunning = false

	dc.logger.Info("Desktop capture component stopped")
	return nil
}

// GetOutputPad returns the output pad for connecting to next component
func (dc *DesktopCaptureComponent) GetOutputPad() *gst.Pad {
	return dc.outputPad
}

// GetStatistics returns current capture statistics
func (dc *DesktopCaptureComponent) GetStatistics() *CaptureStatistics {
	dc.stats.mu.RLock()
	defer dc.stats.mu.RUnlock()

	// Calculate current metrics
	elapsed := time.Since(dc.stats.StartTime).Seconds()
	if elapsed > 0 {
		dc.stats.AverageFPS = float64(dc.stats.FramesTotal) / elapsed
	}

	// Create a copy to return
	statsCopy := *dc.stats
	return &statsCopy
}

// SetPublisher sets the stream publisher for publishing raw video data
func (dc *DesktopCaptureComponent) SetPublisher(publisher *StreamPublisher) {
	dc.publisher = publisher
}

// detectDesktopEnvironment detects whether we're running on X11 or Wayland
func (dc *DesktopCaptureComponent) detectDesktopEnvironment() (bool, error) {
	// Check for Wayland first
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		dc.logger.Debug("Detected Wayland desktop environment")
		return true, nil
	}

	// Check for X11
	if os.Getenv("DISPLAY") != "" {
		dc.logger.Debug("Detected X11 desktop environment")
		return false, nil
	}

	// Default to X11 if neither is clearly detected
	dc.logger.Warn("Could not detect desktop environment, defaulting to X11")
	return false, nil
}

// createSourceElement creates the appropriate capture source (ximagesrc or waylandsrc)
func (dc *DesktopCaptureComponent) createSourceElement() error {
	var elementName string
	var useWayland bool
	var err error

	// Auto-detect desktop environment if not explicitly configured
	if dc.config.UseWayland {
		useWayland = true
		elementName = "waylandsrc"
	} else {
		// Auto-detect
		useWayland, err = dc.detectDesktopEnvironment()
		if err != nil {
			return fmt.Errorf("failed to detect desktop environment: %w", err)
		}

		if useWayland {
			elementName = "waylandsrc"
		} else {
			elementName = "ximagesrc"
		}
	}

	// Create source element
	source, err := gst.NewElement(elementName)
	if err != nil {
		// Fallback to the other source if the preferred one fails
		if useWayland {
			dc.logger.Warn("waylandsrc not available, falling back to ximagesrc")
			source, err = gst.NewElement("ximagesrc")
		} else {
			dc.logger.Warn("ximagesrc not available, falling back to waylandsrc")
			source, err = gst.NewElement("waylandsrc")
		}

		if err != nil {
			return fmt.Errorf("failed to create capture source element: %w", err)
		}
	}

	dc.source = source

	// Register with memory manager
	if dc.memManager != nil {
		dc.memManager.RegisterObject(source)
	}

	// Configure source properties
	if err := dc.configureSourceProperties(useWayland); err != nil {
		return fmt.Errorf("failed to configure source properties: %w", err)
	}

	dc.logger.Infof("Created %s source element", elementName)
	return nil
}

// configureSourceProperties configures source element properties
func (dc *DesktopCaptureComponent) configureSourceProperties(useWayland bool) error {
	if useWayland {
		return dc.configureWaylandSource()
	}
	return dc.configureX11Source()
}

// configureX11Source configures ximagesrc properties
func (dc *DesktopCaptureComponent) configureX11Source() error {
	// Set display
	if dc.config.DisplayID != "" {
		if err := dc.source.SetProperty("display-name", dc.config.DisplayID); err != nil {
			dc.logger.Warnf("Failed to set display-name: %v", err)
		}
	}

	// Configure damage events (disable for stability)
	if err := dc.source.SetProperty("use-damage", false); err != nil {
		dc.logger.Debug("Failed to set use-damage property")
	}

	// Configure mouse pointer visibility
	if err := dc.source.SetProperty("show-pointer", dc.config.ShowPointer); err != nil {
		dc.logger.Warnf("Failed to set show-pointer: %v", err)
	}

	// Configure capture region if specified
	if dc.config.CaptureRegion != nil {
		region := dc.config.CaptureRegion
		properties := map[string]interface{}{
			"startx": uint(region.X),
			"starty": uint(region.Y),
			"endx":   uint(region.X + region.Width - 1),
			"endy":   uint(region.Y + region.Height - 1),
		}

		for name, value := range properties {
			if err := dc.source.SetProperty(name, value); err != nil {
				dc.logger.Warnf("Failed to set %s: %v", name, err)
			}
		}
	}

	dc.logger.Debug("Configured X11 source properties")
	return nil
}

// configureWaylandSource configures waylandsrc properties
func (dc *DesktopCaptureComponent) configureWaylandSource() error {
	// Set display if specified
	if dc.config.DisplayID != "" && dc.config.DisplayID != ":0" {
		if err := dc.source.SetProperty("display", dc.config.DisplayID); err != nil {
			dc.logger.Warnf("Failed to set display: %v", err)
		}
	}

	// Configure mouse pointer visibility
	if err := dc.source.SetProperty("show-pointer", dc.config.ShowPointer); err != nil {
		dc.logger.Warnf("Failed to set show-pointer: %v", err)
	}

	// Configure capture region if specified
	if dc.config.CaptureRegion != nil {
		region := dc.config.CaptureRegion
		properties := map[string]interface{}{
			"crop-x":      uint(region.X),
			"crop-y":      uint(region.Y),
			"crop-width":  uint(region.Width),
			"crop-height": uint(region.Height),
		}

		for name, value := range properties {
			if err := dc.source.SetProperty(name, value); err != nil {
				dc.logger.Warnf("Failed to set %s: %v", name, err)
			}
		}
	}

	dc.logger.Debug("Configured Wayland source properties")
	return nil
}

// createCapsFilter creates and configures the caps filter for format specification
func (dc *DesktopCaptureComponent) createCapsFilter() error {
	capsfilter, err := gst.NewElement("capsfilter")
	if err != nil {
		return fmt.Errorf("failed to create capsfilter: %w", err)
	}

	dc.capsfilter = capsfilter

	// Register with memory manager
	if dc.memManager != nil {
		dc.memManager.RegisterObject(capsfilter)
	}

	// Build caps string based on configuration
	capsStr := dc.buildCapsString()
	caps := gst.NewCapsFromString(capsStr)
	if caps == nil {
		return fmt.Errorf("failed to create caps from string: %s", capsStr)
	}

	// Register caps with memory manager
	if dc.memManager != nil {
		dc.memManager.RegisterObject(caps)
	}

	if err := capsfilter.SetProperty("caps", caps); err != nil {
		return fmt.Errorf("failed to set caps: %w", err)
	}

	dc.logger.Debugf("Created caps filter with caps: %s", capsStr)
	return nil
}

// buildCapsString builds the caps string based on configuration
func (dc *DesktopCaptureComponent) buildCapsString() string {
	format := "BGRx" // Default format for good performance

	// Adjust format based on quality preset
	switch dc.config.Quality {
	case "low":
		format = "RGB"
	case "medium":
		format = "BGRx"
	case "high":
		format = "BGRA"
	default:
		format = "BGRx"
	}

	return fmt.Sprintf("video/x-raw,format=%s,width=%d,height=%d,framerate=%d/1,pixel-aspect-ratio=1/1",
		format, dc.config.Width, dc.config.Height, dc.config.FrameRate)
}

// createQueue creates and configures the buffer queue
func (dc *DesktopCaptureComponent) createQueue() error {
	queue, err := gst.NewElement("queue")
	if err != nil {
		return fmt.Errorf("failed to create queue: %w", err)
	}

	dc.queue = queue

	// Register with memory manager
	if dc.memManager != nil {
		dc.memManager.RegisterObject(queue)
	}

	// Configure queue properties
	properties := map[string]interface{}{
		"max-size-buffers": uint(10),
		"max-size-bytes":   uint(0),
		"max-size-time":    uint64(0),
		"leaky":            2, // downstream leaky
	}

	for name, value := range properties {
		if err := queue.SetProperty(name, value); err != nil {
			dc.logger.Warnf("Failed to set queue property %s: %v", name, err)
		}
	}

	dc.logger.Debug("Created and configured queue element")
	return nil
}

// linkElements links all internal elements together
func (dc *DesktopCaptureComponent) linkElements() error {
	// Link source -> capsfilter
	if err := dc.source.Link(dc.capsfilter); err != nil {
		return fmt.Errorf("failed to link source to capsfilter: %w", err)
	}

	// Link capsfilter -> queue
	if err := dc.capsfilter.Link(dc.queue); err != nil {
		return fmt.Errorf("failed to link capsfilter to queue: %w", err)
	}

	dc.logger.Debug("Successfully linked desktop capture elements")
	return nil
}

// IsRunning returns whether the component is currently running
func (dc *DesktopCaptureComponent) IsRunning() bool {
	dc.mu.RLock()
	defer dc.mu.RUnlock()
	return dc.isRunning
}

// GetState returns the current component state
func (dc *DesktopCaptureComponent) GetState() ComponentState {
	if dc.IsRunning() {
		return ComponentStateStarted
	}
	return ComponentStateStopped
}
