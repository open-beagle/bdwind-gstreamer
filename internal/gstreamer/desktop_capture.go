package gstreamer

/*
#cgo pkg-config: gstreamer-1.0 gstreamer-app-1.0
#include "desktop_capture.h"
*/
import "C"
import (
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/open-beagle/bdwind-gstreamer/internal/config"
)

// DesktopCapture is a global instance of the desktop capture
var desktopCapturePtr unsafe.Pointer

//export goHandleAppsinkSample
func goHandleAppsinkSample(sample *C.GstSample) {
	if desktopCapturePtr == nil {
		return
	}
	dc := (*desktopCapture)(atomic.LoadPointer(&desktopCapturePtr))
	if dc == nil || dc.sampleCallback == nil {
		return
	}

	buffer := C.gst_sample_get_buffer(sample)
	if buffer == nil {
		return
	}

	var mapInfo C.GstMapInfo
	if C.gst_buffer_map(buffer, &mapInfo, C.GST_MAP_READ) == 0 {
		return
	}

	// Create a Go slice that shares memory with the C buffer
	data := C.GoBytes(unsafe.Pointer(mapInfo.data), C.int(mapInfo.size))

	// Get caps from sample to extract width and height
	caps := C.gst_sample_get_caps(sample)
	var width, height int
	if caps != nil {
		s := C.gst_caps_get_structure(caps, 0)
		if s != nil {
			var w, h C.gint
			C.gst_structure_get_int(s, C.CString("width"), &w)
			C.gst_structure_get_int(s, C.CString("height"), &h)
			width = int(w)
			height = int(h)
		}
	}

	// Create a new Sample
	videoSample := NewVideoSample(data, width, height, "raw")

	if err := dc.sampleCallback(videoSample); err != nil {
		fmt.Printf("Error in sample callback: %v\n", err)
	}

	dc.statsCollector.recordFrame()

	C.gst_buffer_unmap(buffer, &mapInfo)
}


// CaptureStats contains statistics about the capture
type CaptureStats struct {
	FramesCapture  int64   // Total frames captured
	FramesDropped  int64   // Total frames dropped
	CurrentFPS     float64 // Current frames per second
	AverageFPS     float64 // Average frames per second since start
	CaptureLatency int64   // Current capture latency in milliseconds
	MinLatency     int64   // Minimum latency recorded in milliseconds
	MaxLatency     int64   // Maximum latency recorded in milliseconds
	AvgLatency     int64   // Average latency in milliseconds
	LastFrameTime  int64   // Timestamp of last frame in milliseconds
	StartTime      int64   // Start time in milliseconds since epoch
}

// statsCollector handles statistics collection for desktop capture
type statsCollector struct {
	mu                  sync.RWMutex
	frameCount          int64
	droppedCount        int64
	startTime           time.Time
	lastFrameTime       time.Time
	lastStatsTime       time.Time
	frameTimings        []int64 // Ring buffer for latency measurements
	timingIndex         int
	currentFPS          float64
	fpsWindow           []time.Time // Sliding window for FPS calculation
	fpsWindowSize       int
	latencySum          int64
	minLatency          int64
	maxLatency          int64
	latencyMeasurements int64
}

// newStatsCollector creates a new statistics collector
func newStatsCollector() *statsCollector {
	const (
		latencyBufferSize = 100 // Keep last 100 latency measurements
		fpsWindowSize     = 60  // Calculate FPS over last 60 frames
	)

	return &statsCollector{
		frameTimings:  make([]int64, latencyBufferSize),
		fpsWindow:     make([]time.Time, 0, fpsWindowSize),
		fpsWindowSize: fpsWindowSize,
		startTime:     time.Now(),
		lastStatsTime: time.Now(),
		minLatency:    math.MaxInt64,
		maxLatency:    0,
	}
}

// recordFrame records a new frame capture event
func (sc *statsCollector) recordFrame() {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	now := time.Now()
	sc.frameCount++
	sc.lastFrameTime = now

	// Update FPS sliding window
	sc.fpsWindow = append(sc.fpsWindow, now)
	if len(sc.fpsWindow) > sc.fpsWindowSize {
		sc.fpsWindow = sc.fpsWindow[1:]
	}

	// Calculate current FPS based on sliding window
	if len(sc.fpsWindow) > 1 {
		windowDuration := sc.fpsWindow[len(sc.fpsWindow)-1].Sub(sc.fpsWindow[0]).Seconds()
		if windowDuration > 0 {
			sc.currentFPS = float64(len(sc.fpsWindow)-1) / windowDuration
		}
	}
}

// recordDroppedFrame records a dropped frame event
func (sc *statsCollector) recordDroppedFrame() {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	sc.droppedCount++
}

// recordLatency records a latency measurement
func (sc *statsCollector) recordLatency(latencyMs int64) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	// Add to ring buffer
	sc.frameTimings[sc.timingIndex] = latencyMs
	sc.timingIndex = (sc.timingIndex + 1) % len(sc.frameTimings)

	// Update min/max
	if latencyMs < sc.minLatency {
		sc.minLatency = latencyMs
	}
	if latencyMs > sc.maxLatency {
		sc.maxLatency = latencyMs
	}

	// Update running average
	sc.latencySum += latencyMs
	sc.latencyMeasurements++
}

// getStats returns current statistics
func (sc *statsCollector) getStats() CaptureStats {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	now := time.Now()
	elapsedSeconds := now.Sub(sc.startTime).Seconds()

	stats := CaptureStats{
		FramesCapture: sc.frameCount,
		FramesDropped: sc.droppedCount,
		CurrentFPS:    sc.currentFPS,
		StartTime:     sc.startTime.UnixMilli(),
		LastFrameTime: sc.lastFrameTime.UnixMilli(),
	}

	// Calculate average FPS
	if elapsedSeconds > 0 {
		stats.AverageFPS = float64(sc.frameCount) / elapsedSeconds
	}

	// Calculate latency statistics
	if sc.latencyMeasurements > 0 {
		stats.AvgLatency = sc.latencySum / sc.latencyMeasurements
		stats.MinLatency = sc.minLatency
		stats.MaxLatency = sc.maxLatency

		// Calculate current latency from recent measurements
		var recentLatencySum int64
		var recentCount int64
		for i := 0; i < len(sc.frameTimings) && i < 10; i++ { // Last 10 measurements
			idx := (sc.timingIndex - 1 - i + len(sc.frameTimings)) % len(sc.frameTimings)
			if sc.frameTimings[idx] > 0 {
				recentLatencySum += sc.frameTimings[idx]
				recentCount++
			}
		}
		if recentCount > 0 {
			stats.CaptureLatency = recentLatencySum / recentCount
		}
	}

	// Handle case where no latency measurements yet
	if stats.MinLatency == math.MaxInt64 {
		stats.MinLatency = 0
	}

	return stats
}

// reset resets all statistics
func (sc *statsCollector) reset() {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	sc.frameCount = 0
	sc.droppedCount = 0
	sc.startTime = time.Now()
	sc.lastFrameTime = time.Time{}
	sc.lastStatsTime = time.Now()
	sc.currentFPS = 0
	sc.fpsWindow = sc.fpsWindow[:0]
	sc.timingIndex = 0
	sc.latencySum = 0
	sc.minLatency = math.MaxInt64
	sc.maxLatency = 0
	sc.latencyMeasurements = 0

	// Clear latency buffer
	for i := range sc.frameTimings {
		sc.frameTimings[i] = 0
	}
}

// DesktopCapture interface for desktop capture
type DesktopCapture interface {
	// Start starts the capture
	Start() error

	// Stop stops the capture
	Stop() error

	// GetSource gets the GStreamer source element
	GetSource() (string, map[string]interface{})

	// GetStats gets capture statistics
	GetStats() CaptureStats

	// SetSampleCallback sets the callback function for processing video samples
	SetSampleCallback(callback func(*Sample) error) error

	// GetAppsink returns the appsink element for external processing
	GetAppsink() Element
}

// Implementation of the DesktopCapture interface
type desktopCapture struct {
	config         config.DesktopCaptureConfig
	pipeline       Pipeline
	sourceElem     Element
	appsinkElem    Element
	stats          CaptureStats
	startTime      time.Time
	frameCount     int64
	droppedCount   int64
	lastFrameTime  time.Time
	frameTimings   []int64 // Ring buffer for latency calculations
	timingIndex    int
	mu             sync.Mutex
	isRunning      bool
	statsCollector *statsCollector
	sampleCallback func(*Sample) error
}

// NewDesktopCapture creates a new desktop capture with validated configuration
func NewDesktopCapture(cfg config.DesktopCaptureConfig) (DesktopCapture, error) {
	// Validate configuration
	if err := config.ValidateDesktopCaptureConfig(&cfg); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &desktopCapture{
		config:         cfg,
		startTime:      time.Now(),
		frameCount:     0,
		frameTimings:   make([]int64, 100), // Buffer for latency measurements
		statsCollector: newStatsCollector(),
	}, nil
}

// NewDesktopCaptureFromEnv creates a new desktop capture with configuration loaded from environment variables
func NewDesktopCaptureFromEnv() (DesktopCapture, error) {
	cfg := config.LoadDesktopCaptureConfigFromEnv()
	return NewDesktopCapture(cfg)
}

// NewDesktopCaptureFromFile creates a new desktop capture with configuration loaded from a file
func NewDesktopCaptureFromFile(filename string) (DesktopCapture, error) {
	cfg, err := config.LoadDesktopCaptureConfigFromFile(filename)
	if err != nil {
		return nil, err
	}
	return NewDesktopCapture(cfg)
}

// Start starts the capture
func (dc *desktopCapture) Start() error {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	if dc.isRunning {
		return errors.New("desktop capture already running")
	}

	// Create pipeline
	dc.pipeline = NewPipeline("desktop-capture")
	err := dc.pipeline.Create()
	if err != nil {
		return fmt.Errorf("failed to create pipeline: %w", err)
	}

	// Configure X11 desktop capture
	if !dc.config.UseWayland {
		err = dc.configureX11Capture()
		if err != nil {
			return fmt.Errorf("failed to configure X11 capture: %w", err)
		}
	} else {
		// Wayland configuration
		err = dc.configureWaylandCapture()
		if err != nil {
			return fmt.Errorf("failed to configure Wayland capture: %w", err)
		}
	}

	// Start pipeline
	fmt.Printf("Starting GStreamer pipeline for display: %s\n", dc.config.DisplayID)
	err = dc.pipeline.Start()
	if err != nil {
		return fmt.Errorf("failed to start pipeline: %w", err)
	}

	// Verify pipeline state
	state, err := dc.pipeline.GetState()
	if err != nil {
		fmt.Printf("Warning: failed to get pipeline state: %v\n", err)
	} else {
		fmt.Printf("Pipeline state after start: %v\n", state)
	}

	dc.isRunning = true
	dc.startTime = time.Now()
	dc.frameCount = 0
	dc.droppedCount = 0
	dc.lastFrameTime = time.Time{}

	// Reset statistics collector
	dc.statsCollector.reset()

	fmt.Printf("Desktop capture pipeline started successfully for display: %s\n", dc.config.DisplayID)
	fmt.Printf("Pipeline configuration: %dx%d@%dfps, Quality: %s\n",
		dc.config.Width, dc.config.Height, dc.config.FrameRate, dc.config.Quality)

	atomic.StorePointer(&desktopCapturePtr, unsafe.Pointer(dc))

	// Start a goroutine to monitor pipeline data flow
	go dc.monitorDataFlow()

	// After starting the pipeline, set up the appsink callback if it's been configured
	if dc.sampleCallback != nil {
		if err := dc.setupAppsinkCallback(); err != nil {
			return fmt.Errorf("failed to setup appsink callback post-start: %w", err)
		}
	}

	return nil
}

// configureX11Capture configures the pipeline for X11 desktop capture
func (dc *desktopCapture) configureX11Capture() error {
	// Create ximagesrc element
	sourceElem, err := dc.pipeline.GetElement("ximagesrc")
	if err != nil {
		return fmt.Errorf("failed to create ximagesrc element: %w", err)
	}
	dc.sourceElem = sourceElem

	// Configure display
	err = dc.sourceElem.SetProperty("display-name", dc.config.DisplayID)
	if err != nil {
		return fmt.Errorf("failed to set display-name property: %w", err)
	}

	// Configure damage events for optimization
	err = dc.sourceElem.SetProperty("use-damage", dc.config.UseDamage)
	if err != nil {
		return fmt.Errorf("failed to set use-damage property: %w", err)
	}

	// Configure mouse pointer visibility
	err = dc.sourceElem.SetProperty("show-pointer", dc.config.ShowPointer)
	if err != nil {
		return fmt.Errorf("failed to set show-pointer property: %w", err)
	}

	// Configure capture region if specified
	if dc.config.CaptureRegion != nil {
		err = dc.configureX11CaptureRegion()
		if err != nil {
			return fmt.Errorf("failed to configure capture region: %w", err)
		}
	}

	// Configure multi-monitor support
	err = dc.configureX11MultiMonitor()
	if err != nil {
		return fmt.Errorf("failed to configure multi-monitor support: %w", err)
	}

	// Create video rate element for frame rate control
	videoRate, err := dc.pipeline.GetElement("videorate")
	if err != nil {
		return fmt.Errorf("failed to create videorate element: %w", err)
	}

	// Configure video rate properties
	err = videoRate.SetProperty("drop-only", true)
	if err != nil {
		return fmt.Errorf("failed to set drop-only property: %w", err)
	}

	// Create caps filter for resolution and frame rate
	capsFilter, err := dc.pipeline.GetElement("capsfilter")
	if err != nil {
		return fmt.Errorf("failed to create capsfilter element: %w", err)
	}

	// Configure caps based on quality preset
	capsStr, err := dc.buildX11CapsString()
	if err != nil {
		return fmt.Errorf("failed to build caps string: %w", err)
	}

	caps, err := NewCapsFromString(capsStr)
	if err != nil {
		return fmt.Errorf("failed to create caps from string: %w", err)
	}

	err = capsFilter.SetProperty("caps", caps)
	if err != nil {
		return fmt.Errorf("failed to set caps property: %w", err)
	}

	// Create video convert element for format conversion
	videoConvert, err := dc.pipeline.GetElement("videoconvert")
	if err != nil {
		return fmt.Errorf("failed to create videoconvert element: %w", err)
	}

	// Create video scale element for resolution scaling
	videoScale, err := dc.pipeline.GetElement("videoscale")
	if err != nil {
		return fmt.Errorf("failed to create videoscale element: %w", err)
	}

	// Configure video scale properties
	err = videoScale.SetProperty("method", 1) // Bilinear scaling
	if err != nil {
		return fmt.Errorf("failed to set videoscale method: %w", err)
	}

	// Link elements in the pipeline
	err = dc.sourceElem.Link(videoConvert)
	if err != nil {
		return fmt.Errorf("failed to link source to videoconvert: %w", err)
	}

	err = videoConvert.Link(videoScale)
	if err != nil {
		return fmt.Errorf("failed to link videoconvert to videoscale: %w", err)
	}

	err = videoScale.Link(videoRate)
	if err != nil {
		return fmt.Errorf("failed to link videoscale to videorate: %w", err)
	}

	err = videoRate.Link(capsFilter)
	if err != nil {
		return fmt.Errorf("failed to link videorate to capsfilter: %w", err)
	}

	// Create appsink element for data output
	appsink, err := dc.pipeline.GetElement("appsink")
	if err != nil {
		return fmt.Errorf("failed to create appsink element: %w", err)
	}

	// Configure appsink properties
	err = appsink.SetProperty("name", "sink")
	if err != nil {
		return fmt.Errorf("failed to set appsink name: %w", err)
	}

	err = appsink.SetProperty("emit-signals", true)
	if err != nil {
		return fmt.Errorf("failed to set emit-signals property: %w", err)
	}

	err = appsink.SetProperty("sync", false)
	if err != nil {
		return fmt.Errorf("failed to set sync property: %w", err)
	}

	err = appsink.SetProperty("max-buffers", 1)
	if err != nil {
		return fmt.Errorf("failed to set max-buffers property: %w", err)
	}

	err = appsink.SetProperty("drop", true)
	if err != nil {
		return fmt.Errorf("failed to set drop property: %w", err)
	}

	err = capsFilter.Link(appsink)
	if err != nil {
		return fmt.Errorf("failed to link capsfilter to appsink: %w", err)
	}

	// Store appsink reference for later use
	dc.appsinkElem = appsink

	return nil
}

// configureX11CaptureRegion configures region capture for X11
func (dc *desktopCapture) configureX11CaptureRegion() error {
	region := dc.config.CaptureRegion

	// Set capture region coordinates
	err := dc.sourceElem.SetProperty("startx", region.X)
	if err != nil {
		return fmt.Errorf("failed to set startx property: %w", err)
	}

	err = dc.sourceElem.SetProperty("starty", region.Y)
	if err != nil {
		return fmt.Errorf("failed to set starty property: %w", err)
	}

	err = dc.sourceElem.SetProperty("endx", region.X+region.Width-1)
	if err != nil {
		return fmt.Errorf("failed to set endx property: %w", err)
	}

	err = dc.sourceElem.SetProperty("endy", region.Y+region.Height-1)
	if err != nil {
		return fmt.Errorf("failed to set endy property: %w", err)
	}

	return nil
}

// configureX11MultiMonitor configures multi-monitor support for X11
func (dc *desktopCapture) configureX11MultiMonitor() error {
	// Set screen number if specified in display ID (e.g., ":0.1" for screen 1)
	if strings.Contains(dc.config.DisplayID, ".") {
		parts := strings.Split(dc.config.DisplayID, ".")
		if len(parts) == 2 {
			screenNum := parts[1]
			if screenNum != "0" {
				err := dc.sourceElem.SetProperty("screen-num", screenNum)
				if err != nil {
					return fmt.Errorf("failed to set screen-num property: %w", err)
				}
			}
		}
	}

	// Configure XID if capturing a specific window
	if dc.config.CaptureRegion != nil && dc.config.CaptureRegion.X == 0 && dc.config.CaptureRegion.Y == 0 {
		// This could be enhanced to support window ID capture
		// For now, we'll use the full screen approach
	}

	return nil
}

// buildX11CapsString builds the caps string for X11 capture based on configuration
func (dc *desktopCapture) buildX11CapsString() (string, error) {
	var capsStr strings.Builder

	// Base video format
	capsStr.WriteString("video/x-raw")

	// Add format based on quality preset
	switch dc.config.Quality {
	case "low":
		capsStr.WriteString(",format=RGB")
	case "medium":
		capsStr.WriteString(",format=BGRx")
	case "high":
		capsStr.WriteString(",format=BGRA")
	default:
		capsStr.WriteString(",format=BGRx")
	}

	// Add resolution
	capsStr.WriteString(fmt.Sprintf(",width=%d,height=%d", dc.config.Width, dc.config.Height))

	// Add frame rate
	capsStr.WriteString(fmt.Sprintf(",framerate=%d/1", dc.config.FrameRate))

	// Add pixel aspect ratio
	capsStr.WriteString(",pixel-aspect-ratio=1/1")

	return capsStr.String(), nil
}

// configureWaylandCapture configures the pipeline for Wayland desktop capture
func (dc *desktopCapture) configureWaylandCapture() error {
	// Create waylandsrc element
	sourceElem, err := dc.pipeline.GetElement("waylandsrc")
	if err != nil {
		return fmt.Errorf("failed to create waylandsrc element: %w", err)
	}
	dc.sourceElem = sourceElem

	// Configure Wayland display
	if dc.config.DisplayID != "" && dc.config.DisplayID != ":0" {
		err = dc.sourceElem.SetProperty("display", dc.config.DisplayID)
		if err != nil {
			return fmt.Errorf("failed to set display property: %w", err)
		}
	}

	// Configure mouse pointer visibility
	err = dc.sourceElem.SetProperty("show-pointer", dc.config.ShowPointer)
	if err != nil {
		return fmt.Errorf("failed to set show-pointer property: %w", err)
	}

	// Configure capture region if specified
	if dc.config.CaptureRegion != nil {
		err = dc.configureWaylandCaptureRegion()
		if err != nil {
			return fmt.Errorf("failed to configure capture region: %w", err)
		}
	}

	// Configure multi-monitor support
	err = dc.configureWaylandMultiMonitor()
	if err != nil {
		return fmt.Errorf("failed to configure multi-monitor support: %w", err)
	}

	// Create video rate element for frame rate control
	videoRate, err := dc.pipeline.GetElement("videorate")
	if err != nil {
		return fmt.Errorf("failed to create videorate element: %w", err)
	}

	// Configure video rate properties
	err = videoRate.SetProperty("drop-only", true)
	if err != nil {
		return fmt.Errorf("failed to set drop-only property: %w", err)
	}

	// Create caps filter for resolution and frame rate
	capsFilter, err := dc.pipeline.GetElement("capsfilter")
	if err != nil {
		return fmt.Errorf("failed to create capsfilter element: %w", err)
	}

	// Configure caps based on quality preset
	capsStr, err := dc.buildWaylandCapsString()
	if err != nil {
		return fmt.Errorf("failed to build caps string: %w", err)
	}

	caps, err := NewCapsFromString(capsStr)
	if err != nil {
		return fmt.Errorf("failed to create caps from string: %w", err)
	}

	err = capsFilter.SetProperty("caps", caps)
	if err != nil {
		return fmt.Errorf("failed to set caps property: %w", err)
	}

	// Create video convert element for format conversion
	videoConvert, err := dc.pipeline.GetElement("videoconvert")
	if err != nil {
		return fmt.Errorf("failed to create videoconvert element: %w", err)
	}

	// Create video scale element for resolution scaling
	videoScale, err := dc.pipeline.GetElement("videoscale")
	if err != nil {
		return fmt.Errorf("failed to create videoscale element: %w", err)
	}

	// Configure video scale properties
	err = videoScale.SetProperty("method", 1) // Bilinear scaling
	if err != nil {
		return fmt.Errorf("failed to set videoscale method: %w", err)
	}

	// Link elements in the pipeline
	err = dc.sourceElem.Link(videoConvert)
	if err != nil {
		return fmt.Errorf("failed to link source to videoconvert: %w", err)
	}

	err = videoConvert.Link(videoScale)
	if err != nil {
		return fmt.Errorf("failed to link videoconvert to videoscale: %w", err)
	}

	err = videoScale.Link(videoRate)
	if err != nil {
		return fmt.Errorf("failed to link videoscale to videorate: %w", err)
	}

	err = videoRate.Link(capsFilter)
	if err != nil {
		return fmt.Errorf("failed to link videorate to capsfilter: %w", err)
	}

	// Create appsink element for data output
	appsink, err := dc.pipeline.GetElement("appsink")
	if err != nil {
		return fmt.Errorf("failed to create appsink element: %w", err)
	}

	// Configure appsink properties
	err = appsink.SetProperty("name", "sink")
	if err != nil {
		return fmt.Errorf("failed to set appsink name: %w", err)
	}

	err = appsink.SetProperty("emit-signals", true)
	if err != nil {
		return fmt.Errorf("failed to set emit-signals property: %w", err)
	}

	err = appsink.SetProperty("sync", false)
	if err != nil {
		return fmt.Errorf("failed to set sync property: %w", err)
	}

	err = appsink.SetProperty("max-buffers", 1)
	if err != nil {
		return fmt.Errorf("failed to set max-buffers property: %w", err)
	}

	err = appsink.SetProperty("drop", true)
	if err != nil {
		return fmt.Errorf("failed to set drop property: %w", err)
	}

	err = capsFilter.Link(appsink)
	if err != nil {
		return fmt.Errorf("failed to link capsfilter to appsink: %w", err)
	}

	// Store appsink reference for later use
	dc.appsinkElem = appsink

	return nil
}

// configureWaylandCaptureRegion configures region capture for Wayland
func (dc *desktopCapture) configureWaylandCaptureRegion() error {
	region := dc.config.CaptureRegion

	// Set capture region coordinates
	err := dc.sourceElem.SetProperty("crop-x", region.X)
	if err != nil {
		return fmt.Errorf("failed to set crop-x property: %w", err)
	}

	err = dc.sourceElem.SetProperty("crop-y", region.Y)
	if err != nil {
		return fmt.Errorf("failed to set crop-y property: %w", err)
	}

	err = dc.sourceElem.SetProperty("crop-width", region.Width)
	if err != nil {
		return fmt.Errorf("failed to set crop-width property: %w", err)
	}

	err = dc.sourceElem.SetProperty("crop-height", region.Height)
	if err != nil {
		return fmt.Errorf("failed to set crop-height property: %w", err)
	}

	return nil
}

// configureWaylandMultiMonitor configures multi-monitor support for Wayland
func (dc *desktopCapture) configureWaylandMultiMonitor() error {
	// For Wayland, monitor selection is typically handled through the display property
	// or through output selection if supported by the compositor

	// Parse display ID for output selection (e.g., "wayland-0", "wayland-1")
	if strings.Contains(dc.config.DisplayID, "-") {
		parts := strings.Split(dc.config.DisplayID, "-")
		if len(parts) == 2 {
			outputNum := parts[1]
			if outputNum != "0" {
				// Try to set output property if supported
				err := dc.sourceElem.SetProperty("output", outputNum)
				if err != nil {
					// If output property is not supported, log but don't fail
					// This is because different Wayland compositors may have different capabilities
					fmt.Printf("Warning: failed to set output property (may not be supported): %v\n", err)
				}
			}
		}
	}

	return nil
}

// buildWaylandCapsString builds the caps string for Wayland capture based on configuration
func (dc *desktopCapture) buildWaylandCapsString() (string, error) {
	var capsStr strings.Builder

	// Base video format
	capsStr.WriteString("video/x-raw")

	// Add format based on quality preset
	switch dc.config.Quality {
	case "low":
		capsStr.WriteString(",format=RGB")
	case "medium":
		capsStr.WriteString(",format=BGRx")
	case "high":
		capsStr.WriteString(",format=BGRA")
	default:
		capsStr.WriteString(",format=BGRx")
	}

	// Add resolution
	capsStr.WriteString(fmt.Sprintf(",width=%d,height=%d", dc.config.Width, dc.config.Height))

	// Add frame rate
	capsStr.WriteString(fmt.Sprintf(",framerate=%d/1", dc.config.FrameRate))

	// Add pixel aspect ratio
	capsStr.WriteString(",pixel-aspect-ratio=1/1")

	return capsStr.String(), nil
}

// Stop stops the capture
func (dc *desktopCapture) Stop() error {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	if !dc.isRunning {
		return nil
	}

	err := dc.pipeline.Stop()
	if err != nil {
		return fmt.Errorf("failed to stop pipeline: %w", err)
	}

	dc.isRunning = false

	return nil
}

// GetSource gets the GStreamer source element configuration
func (dc *desktopCapture) GetSource() (string, map[string]interface{}) {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	var sourceType string
	if dc.config.UseWayland {
		sourceType = "waylandsrc"
	} else {
		sourceType = "ximagesrc"
	}

	props := make(map[string]interface{})

	if !dc.config.UseWayland {
		// X11-specific properties
		props["display"] = dc.config.DisplayID
		props["use-damage"] = dc.config.UseDamage
		props["show-pointer"] = dc.config.ShowPointer

		// Add screen number if specified
		if strings.Contains(dc.config.DisplayID, ".") {
			parts := strings.Split(dc.config.DisplayID, ".")
			if len(parts) == 2 {
				screenNum := parts[1]
				if screenNum != "0" {
					props["screen-num"] = screenNum
				}
			}
		}

		// Add capture region if specified
		if dc.config.CaptureRegion != nil {
			props["startx"] = dc.config.CaptureRegion.X
			props["starty"] = dc.config.CaptureRegion.Y
			props["endx"] = dc.config.CaptureRegion.X + dc.config.CaptureRegion.Width - 1
			props["endy"] = dc.config.CaptureRegion.Y + dc.config.CaptureRegion.Height - 1
		}
	} else {
		// Wayland-specific properties
		if dc.config.DisplayID != "" && dc.config.DisplayID != ":0" {
			props["display"] = dc.config.DisplayID
		}
		props["show-pointer"] = dc.config.ShowPointer

		// Add output selection if specified
		if strings.Contains(dc.config.DisplayID, "-") {
			parts := strings.Split(dc.config.DisplayID, "-")
			if len(parts) == 2 {
				outputNum := parts[1]
				if outputNum != "0" {
					props["output"] = outputNum
				}
			}
		}

		// Add capture region if specified
		if dc.config.CaptureRegion != nil {
			props["crop-x"] = dc.config.CaptureRegion.X
			props["crop-y"] = dc.config.CaptureRegion.Y
			props["crop-width"] = dc.config.CaptureRegion.Width
			props["crop-height"] = dc.config.CaptureRegion.Height
		}
	}

	return sourceType, props
}

// GetStats gets capture statistics
func (dc *desktopCapture) GetStats() CaptureStats {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	// Get statistics from the collector
	return dc.statsCollector.getStats()
}

// SetSampleCallback sets the callback function for receiving video samples
func (dc *desktopCapture) SetSampleCallback(callback func(*Sample) error) error {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	dc.sampleCallback = callback

	// If pipeline is running and appsink is available, set up the callback
	if dc.isRunning && dc.appsinkElem != nil {
		return dc.setupAppsinkCallback()
	}

	return nil
}

// setupAppsinkCallback sets up the appsink callback to receive samples
func (dc *desktopCapture) setupAppsinkCallback() error {
	if dc.appsinkElem == nil {
		return fmt.Errorf("appsink element not available")
	}
	C.connect_appsink_callback(dc.appsinkElem.GetInternal())
	return nil
}

// simulateFrameGeneration simulates frame generation for testing purposes
// In a real implementation, this would be replaced by actual GStreamer callbacks
func (dc *desktopCapture) simulateFrameGeneration() {
	if !dc.isRunning || dc.sampleCallback == nil {
		return
	}

	fmt.Printf("Starting simulated frame generation for testing\n")

	ticker := time.NewTicker(time.Duration(1000/dc.config.FrameRate) * time.Millisecond)
	defer ticker.Stop()

	frameCount := 0
	for dc.isRunning {
		select {
		case <-ticker.C:
			frameCount++

			// Create a test pattern video sample
			dummyData := make([]byte, dc.config.Width*dc.config.Height*3) // RGB data

			// Generate a more visible test pattern
			for y := 0; y < dc.config.Height; y++ {
				for x := 0; x < dc.config.Width; x++ {
					idx := (y*dc.config.Width + x) * 3

					// Create a colorful test pattern that changes over time
					r := byte((x + frameCount) % 256)
					g := byte((y + frameCount*2) % 256)
					b := byte((x + y + frameCount*3) % 256)

					// Add some geometric patterns
					if (x/50+y/50)%2 == 0 {
						r = byte(255 - r)
					}
					if (x/100)%2 == (y/100)%2 {
						g = byte(255 - g)
					}

					dummyData[idx] = r   // Red
					dummyData[idx+1] = g // Green
					dummyData[idx+2] = b // Blue
				}
			}

			sample := NewVideoSample(dummyData, dc.config.Width, dc.config.Height, "raw")
			sample.Metadata["frame_number"] = frameCount

			// Call the callback
			if err := dc.sampleCallback(sample); err != nil {
				fmt.Printf("Error in sample callback: %v\n", err)
			} else {
				// Update statistics
				dc.statsCollector.recordFrame()
				if frameCount%30 == 0 { // Log every 30 frames (1 second at 30fps)
					fmt.Printf("Generated frame %d, size: %d bytes\n", frameCount, len(dummyData))
				}
			}

		default:
			if !dc.isRunning {
				break
			}
			time.Sleep(time.Millisecond * 10)
		}
	}

	fmt.Printf("Stopped simulated frame generation\n")
}

// GetAppsink returns the appsink element for external processing
func (dc *desktopCapture) GetAppsink() Element {
	dc.mu.Lock()
	defer dc.mu.Unlock()
	return dc.appsinkElem
}

// monitorDataFlow monitors the pipeline data flow and logs statistics
func (dc *desktopCapture) monitorDataFlow() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if !dc.isRunning {
				return
			}

			// Get current pipeline state
			state, err := dc.pipeline.GetState()
			if err != nil {
				fmt.Printf("Warning: failed to get pipeline state: %v\n", err)
				continue
			}

			// Get capture statistics
			stats := dc.GetStats()

			fmt.Printf("Pipeline Status - State: %v, Frames: %d, FPS: %.2f, Dropped: %d\n",
				state, stats.FramesCapture, stats.CurrentFPS, stats.FramesDropped)

			// Check if we're receiving data
			if stats.FramesCapture == 0 && time.Since(dc.startTime) > 10*time.Second {
				fmt.Printf("Warning: No frames captured after 10 seconds - pipeline may not be producing data\n")
				fmt.Printf("Display: %s, Pipeline State: %v\n", dc.config.DisplayID, state)
			}

		default:
			if !dc.isRunning {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
}


// simulateFrameCapture simulates a frame capture event for testing/demo purposes
// In a real implementation, this would be called by GStreamer callbacks
func (dc *desktopCapture) simulateFrameCapture(latencyMs int64) {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	// Allow simulation even if not running for testing purposes
	dc.frameCount++
	dc.lastFrameTime = time.Now()

	// Record frame and latency in statistics collector
	dc.statsCollector.recordFrame()
	if latencyMs > 0 {
		dc.statsCollector.recordLatency(latencyMs)
	}
}

// simulateDroppedFrame simulates a dropped frame event for testing/demo purposes
// In a real implementation, this would be called by GStreamer callbacks
func (dc *desktopCapture) simulateDroppedFrame() {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	// Allow simulation even if not running for testing purposes
	dc.droppedCount++

	// Record dropped frame in statistics collector
	dc.statsCollector.recordDroppedFrame()
}

// UpdateStats updates statistics based on GStreamer pipeline events
// This method would be called by GStreamer event handlers in a real implementation
func (dc *desktopCapture) UpdateStats(framesCaptured, framesDropped int64, avgLatencyMs int64) {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	// Allow stats update even if not running for testing purposes

	// Update frame counts
	newFrames := framesCaptured - dc.frameCount
	newDropped := framesDropped - dc.droppedCount

	dc.frameCount = framesCaptured
	dc.droppedCount = framesDropped

	// Record new frames and dropped frames
	for i := int64(0); i < newFrames; i++ {
		dc.statsCollector.recordFrame()
		if avgLatencyMs > 0 {
			dc.statsCollector.recordLatency(avgLatencyMs)
		}
	}

	for i := int64(0); i < newDropped; i++ {
		dc.statsCollector.recordDroppedFrame()
	}
}

// X11DisplayInfo contains information about an X11 display
type X11DisplayInfo struct {
	DisplayName string
	ScreenNum   int
	Width       int
	Height      int
	RefreshRate float64
}

// GetX11Displays returns information about available X11 displays
func GetX11Displays() ([]X11DisplayInfo, error) {
	// In a real implementation, this would query X11 for display information
	// For now, we'll return a simplified list based on environment variables

	displays := []X11DisplayInfo{}

	// Get primary display from DISPLAY environment variable
	displayEnv := os.Getenv("DISPLAY")
	if displayEnv == "" {
		displayEnv = ":0"
	}

	// Parse display name to extract screen information
	parts := strings.Split(displayEnv, ".")
	displayName := parts[0]
	screenNum := 0

	if len(parts) > 1 {
		if num, err := strconv.Atoi(parts[1]); err == nil {
			screenNum = num
		}
	}

	// Add primary display (this would be queried from X11 in a real implementation)
	displays = append(displays, X11DisplayInfo{
		DisplayName: displayName,
		ScreenNum:   screenNum,
		Width:       1920, // Default values - would be queried from X11
		Height:      1080,
		RefreshRate: 60.0,
	})

	return displays, nil
}

// ValidateX11Display validates that the specified display is available
func ValidateX11Display(displayID string) error {
	displays, err := GetX11Displays()
	if err != nil {
		return fmt.Errorf("failed to get X11 displays: %w", err)
	}

	for _, display := range displays {
		if display.DisplayName == displayID ||
			fmt.Sprintf("%s.%d", display.DisplayName, display.ScreenNum) == displayID {
			return nil
		}
	}

	return fmt.Errorf("display %s not found", displayID)
}

// GetOptimalX11CaptureSettings returns optimal capture settings for X11
func GetOptimalX11CaptureSettings(displayID string) (config.DesktopCaptureConfig, error) {
	cfg := config.DefaultDesktopCaptureConfig()
	cfg.UseWayland = false
	cfg.DisplayID = displayID

	// Validate display exists
	err := ValidateX11Display(displayID)
	if err != nil {
		return cfg, err
	}

	// Get display information
	displays, err := GetX11Displays()
	if err != nil {
		return cfg, fmt.Errorf("failed to get display info: %w", err)
	}

	// Find matching display and set optimal settings
	for _, display := range displays {
		if display.DisplayName == displayID ||
			fmt.Sprintf("%s.%d", display.DisplayName, display.ScreenNum) == displayID {
			cfg.Width = display.Width
			cfg.Height = display.Height
			cfg.FrameRate = int(display.RefreshRate)
			break
		}
	}

	return cfg, nil
}

// CreateX11CaptureRegion creates a capture region for a specific area
func CreateX11CaptureRegion(x, y, width, height int) *config.Rect {
	return &config.Rect{
		X:      x,
		Y:      y,
		Width:  width,
		Height: height,
	}
}

// CreateX11WindowCaptureRegion creates a capture region for a specific window
// This is a placeholder - in a real implementation, this would query X11 for window geometry
func CreateX11WindowCaptureRegion(windowID string) (*config.Rect, error) {
	// In a real implementation, this would use X11 APIs to get window geometry
	// For now, return a default region
	return &config.Rect{
		X:      100,
		Y:      100,
		Width:  800,
		Height: 600,
	}, nil
}

// WaylandDisplayInfo contains information about a Wayland display
type WaylandDisplayInfo struct {
	DisplayName string
	OutputName  string
	Width       int
	Height      int
	RefreshRate float64
	Scale       float64
}

// GetWaylandDisplays returns information about available Wayland displays
func GetWaylandDisplays() ([]WaylandDisplayInfo, error) {
	// In a real implementation, this would query Wayland compositor for display information
	// For now, we'll return a simplified list based on environment variables

	displays := []WaylandDisplayInfo{}

	// Get primary display from WAYLAND_DISPLAY environment variable
	waylandDisplay := os.Getenv("WAYLAND_DISPLAY")
	if waylandDisplay == "" {
		waylandDisplay = "wayland-0"
	}

	// Add primary display (this would be queried from Wayland compositor in a real implementation)
	displays = append(displays, WaylandDisplayInfo{
		DisplayName: waylandDisplay,
		OutputName:  "output-0",
		Width:       1920, // Default values - would be queried from Wayland
		Height:      1080,
		RefreshRate: 60.0,
		Scale:       1.0,
	})

	// Check for additional outputs (simplified approach)
	// In a real implementation, this would use Wayland protocols to enumerate outputs
	for i := 1; i <= 3; i++ {
		outputName := fmt.Sprintf("output-%d", i)
		displays = append(displays, WaylandDisplayInfo{
			DisplayName: fmt.Sprintf("wayland-%d", i),
			OutputName:  outputName,
			Width:       1920, // Default values
			Height:      1080,
			RefreshRate: 60.0,
			Scale:       1.0,
		})
	}

	return displays, nil
}

// ValidateWaylandDisplay validates that the specified display is available
func ValidateWaylandDisplay(displayID string) error {
	displays, err := GetWaylandDisplays()
	if err != nil {
		return fmt.Errorf("failed to get Wayland displays: %w", err)
	}

	for _, display := range displays {
		if display.DisplayName == displayID {
			return nil
		}
	}

	return fmt.Errorf("Wayland display %s not found", displayID)
}

// GetOptimalWaylandCaptureSettings returns optimal capture settings for Wayland
func GetOptimalWaylandCaptureSettings(displayID string) (config.DesktopCaptureConfig, error) {
	cfg := config.DefaultDesktopCaptureConfig()
	cfg.UseWayland = true
	cfg.DisplayID = displayID

	// Validate display exists
	err := ValidateWaylandDisplay(displayID)
	if err != nil {
		return cfg, err
	}

	// Get display information
	displays, err := GetWaylandDisplays()
	if err != nil {
		return cfg, fmt.Errorf("failed to get display info: %w", err)
	}

	// Find matching display and set optimal settings
	for _, display := range displays {
		if display.DisplayName == displayID {
			cfg.Width = int(float64(display.Width) / display.Scale)
			cfg.Height = int(float64(display.Height) / display.Scale)
			cfg.FrameRate = int(display.RefreshRate)
			break
		}
	}

	return cfg, nil
}

// CreateWaylandCaptureRegion creates a capture region for a specific area
func CreateWaylandCaptureRegion(x, y, width, height int) *config.Rect {
	return &config.Rect{
		X:      x,
		Y:      y,
		Width:  width,
		Height: height,
	}
}

// CreateWaylandWindowCaptureRegion creates a capture region for a specific window
// This is a placeholder - in a real implementation, this would query Wayland compositor for window geometry
func CreateWaylandWindowCaptureRegion(windowID string) (*config.Rect, error) {
	// In a real implementation, this would use Wayland protocols to get window geometry
	// For now, return a default region
	return &config.Rect{
		X:      100,
		Y:      100,
		Width:  800,
		Height: 600,
	}, nil
}

// DetectWaylandCompositor detects the current Wayland compositor
func DetectWaylandCompositor() (string, error) {
	// Check common environment variables that indicate the compositor
	compositorEnvVars := map[string]string{
		"GNOME_DESKTOP_SESSION_ID": "gnome-shell",
		"KDE_SESSION_VERSION":      "kwin",
		"DESKTOP_SESSION":          "",
		"XDG_CURRENT_DESKTOP":      "",
		"XDG_SESSION_DESKTOP":      "",
	}

	// Check for specific compositor environment variables
	if os.Getenv("WAYLAND_DISPLAY") == "" {
		return "", fmt.Errorf("not running under Wayland")
	}

	// Try to detect compositor from environment variables
	for envVar, compositor := range compositorEnvVars {
		if value := os.Getenv(envVar); value != "" {
			if compositor != "" {
				return compositor, nil
			}
			// For generic variables, try to parse the value
			switch strings.ToLower(value) {
			case "gnome", "gnome-shell":
				return "gnome-shell", nil
			case "kde", "plasma", "kwin":
				return "kwin", nil
			case "sway":
				return "sway", nil
			case "weston":
				return "weston", nil
			case "mutter":
				return "mutter", nil
			}
		}
	}

	// Check for compositor-specific processes
	compositorProcesses := []string{
		"gnome-shell",
		"kwin_wayland",
		"sway",
		"weston",
		"mutter",
		"wayfire",
		"river",
		"hyprland",
	}

	for _, process := range compositorProcesses {
		if isProcessRunning(process) {
			return process, nil
		}
	}

	// If we can't detect the specific compositor, return generic Wayland
	return "unknown", nil
}

// isProcessRunning checks if a process with the given name is running
func isProcessRunning(processName string) bool {
	// This is a simplified check - in a real implementation, you might use
	// more sophisticated process detection
	cmd := fmt.Sprintf("pgrep -x %s", processName)
	if err := executeBashCommand(cmd); err == nil {
		return true
	}
	return false
}

// executeBashCommand executes a bash command and returns error if it fails
func executeBashCommand(cmd string) error {
	// This is a placeholder - in a real implementation, you would use
	// exec.Command or similar to execute the command
	return fmt.Errorf("command execution not implemented: %s", cmd)
}

// GetWaylandCompositorCapabilities returns the capabilities of the current Wayland compositor
func GetWaylandCompositorCapabilities() (map[string]bool, error) {
	compositor, err := DetectWaylandCompositor()
	if err != nil {
		return nil, fmt.Errorf("failed to detect compositor: %w", err)
	}

	capabilities := make(map[string]bool)

	// Set default capabilities that most compositors support
	capabilities["screen-capture"] = true
	capabilities["pointer-capture"] = true
	capabilities["keyboard-capture"] = false // Usually requires special permissions
	capabilities["clipboard-access"] = false // Usually requires special permissions

	// Set compositor-specific capabilities
	switch compositor {
	case "gnome-shell", "mutter":
		capabilities["screen-capture"] = true
		capabilities["pointer-capture"] = true
		capabilities["window-capture"] = true
		capabilities["output-selection"] = true
		capabilities["region-capture"] = true
		capabilities["damage-tracking"] = true

	case "kwin":
		capabilities["screen-capture"] = true
		capabilities["pointer-capture"] = true
		capabilities["window-capture"] = true
		capabilities["output-selection"] = true
		capabilities["region-capture"] = true
		capabilities["damage-tracking"] = true

	case "sway":
		capabilities["screen-capture"] = true
		capabilities["pointer-capture"] = true
		capabilities["window-capture"] = true
		capabilities["output-selection"] = true
		capabilities["region-capture"] = true
		capabilities["damage-tracking"] = false

	case "weston":
		capabilities["screen-capture"] = true
		capabilities["pointer-capture"] = true
		capabilities["window-capture"] = false
		capabilities["output-selection"] = true
		capabilities["region-capture"] = false
		capabilities["damage-tracking"] = false

	case "wayfire":
		capabilities["screen-capture"] = true
		capabilities["pointer-capture"] = true
		capabilities["window-capture"] = true
		capabilities["output-selection"] = true
		capabilities["region-capture"] = true
		capabilities["damage-tracking"] = false

	case "hyprland":
		capabilities["screen-capture"] = true
		capabilities["pointer-capture"] = true
		capabilities["window-capture"] = true
		capabilities["output-selection"] = true
		capabilities["region-capture"] = true
		capabilities["damage-tracking"] = true

	default:
		// Unknown compositor - assume basic capabilities
		capabilities["screen-capture"] = true
		capabilities["pointer-capture"] = true
		capabilities["window-capture"] = false
		capabilities["output-selection"] = false
		capabilities["region-capture"] = false
		capabilities["damage-tracking"] = false
	}

	return capabilities, nil
}

// IsWaylandSupported checks if Wayland desktop capture is supported on the current system
func IsWaylandSupported() bool {
	// Check if we're running under Wayland
	if os.Getenv("WAYLAND_DISPLAY") == "" {
		return false
	}

	// Check if waylandsrc GStreamer plugin is available
	// In a real implementation, this would check GStreamer plugin registry
	return true
}

// GetWaylandDisplayInfo gets detailed information about a specific Wayland display
func GetWaylandDisplayInfo(displayID string) (*WaylandDisplayInfo, error) {
	displays, err := GetWaylandDisplays()
	if err != nil {
		return nil, fmt.Errorf("failed to get displays: %w", err)
	}

	for _, display := range displays {
		if display.DisplayName == displayID {
			return &display, nil
		}
	}

	return nil, fmt.Errorf("display %s not found", displayID)
}

// ConfigureWaylandSecurity configures security settings for Wayland capture
func ConfigureWaylandSecurity(cfg *config.DesktopCaptureConfig) error {
	// Check if we have necessary permissions for screen capture
	compositor, err := DetectWaylandCompositor()
	if err != nil {
		return fmt.Errorf("failed to detect compositor: %w", err)
	}

	capabilities, err := GetWaylandCompositorCapabilities()
	if err != nil {
		return fmt.Errorf("failed to get capabilities: %w", err)
	}

	// Check if screen capture is supported
	if !capabilities["screen-capture"] {
		return fmt.Errorf("screen capture not supported by compositor %s", compositor)
	}

	// For some compositors, we might need to request permissions
	switch compositor {
	case "gnome-shell", "mutter":
		// GNOME requires permission through portal
		if err := requestGNOMEScreenCapturePermission(); err != nil {
			return fmt.Errorf("failed to request GNOME screen capture permission: %w", err)
		}

	case "kwin":
		// KDE might require specific configuration
		if err := checkKDEScreenCaptureConfig(); err != nil {
			return fmt.Errorf("KDE screen capture not properly configured: %w", err)
		}
	}

	return nil
}

// requestGNOMEScreenCapturePermission requests screen capture permission from GNOME
func requestGNOMEScreenCapturePermission() error {
	// In a real implementation, this would use D-Bus to request permission
	// through the desktop portal
	return nil
}

// checkKDEScreenCaptureConfig checks if KDE screen capture is properly configured
func checkKDEScreenCaptureConfig() error {
	// In a real implementation, this would check KDE configuration files
	return nil
}

// OptimizeWaylandCapture optimizes capture settings for the detected Wayland compositor
func OptimizeWaylandCapture(cfg *config.DesktopCaptureConfig) error {
	compositor, err := DetectWaylandCompositor()
	if err != nil {
		return fmt.Errorf("failed to detect compositor: %w", err)
	}

	capabilities, err := GetWaylandCompositorCapabilities()
	if err != nil {
		return fmt.Errorf("failed to get capabilities: %w", err)
	}

	// Optimize based on compositor capabilities
	switch compositor {
	case "gnome-shell", "mutter":
		// GNOME/Mutter supports damage tracking for better performance
		if capabilities["damage-tracking"] {
			// Enable damage tracking optimizations
			cfg.Quality = "high" // Can afford higher quality with damage tracking
		}

	case "kwin":
		// KWin has good hardware acceleration support
		cfg.Quality = "high"

	case "sway":
		// Sway is lightweight, optimize for performance
		if cfg.FrameRate > 30 {
			cfg.FrameRate = 30 // Limit frame rate for better performance
		}

	case "weston":
		// Weston is basic, use conservative settings
		cfg.Quality = "medium"
		if cfg.FrameRate > 24 {
			cfg.FrameRate = 24
		}
	}

	return nil
}

// GetWaylandOutputs returns information about available Wayland outputs
func GetWaylandOutputs() ([]WaylandOutputInfo, error) {
	// In a real implementation, this would use Wayland protocols to enumerate outputs
	outputs := []WaylandOutputInfo{}

	// Get display information and convert to output information
	displays, err := GetWaylandDisplays()
	if err != nil {
		return nil, fmt.Errorf("failed to get displays: %w", err)
	}

	for i, display := range displays {
		output := WaylandOutputInfo{
			Name:        display.OutputName,
			Description: fmt.Sprintf("Output %d", i),
			Width:       display.Width,
			Height:      display.Height,
			RefreshRate: display.RefreshRate,
			Scale:       display.Scale,
			Transform:   0,                 // Normal orientation
			X:           i * display.Width, // Assume horizontal layout
			Y:           0,
		}
		outputs = append(outputs, output)
	}

	return outputs, nil
}

// WaylandOutputInfo contains information about a Wayland output
type WaylandOutputInfo struct {
	Name        string  // Output name (e.g., "DP-1", "HDMI-A-1")
	Description string  // Human-readable description
	Width       int     // Width in pixels
	Height      int     // Height in pixels
	RefreshRate float64 // Refresh rate in Hz
	Scale       float64 // Scale factor
	Transform   int     // Transform (rotation/reflection)
	X           int     // X position in global coordinate space
	Y           int     // Y position in global coordinate space
}

// SelectWaylandOutput selects a specific Wayland output for capture
func SelectWaylandOutput(cfg *config.DesktopCaptureConfig, outputName string) error {
	outputs, err := GetWaylandOutputs()
	if err != nil {
		return fmt.Errorf("failed to get outputs: %w", err)
	}

	// Find the requested output
	var selectedOutput *WaylandOutputInfo
	for _, output := range outputs {
		if output.Name == outputName {
			selectedOutput = &output
			break
		}
	}

	if selectedOutput == nil {
		return fmt.Errorf("output %s not found", outputName)
	}

	// Update configuration for the selected output
	cfg.Width = int(float64(selectedOutput.Width) / selectedOutput.Scale)
	cfg.Height = int(float64(selectedOutput.Height) / selectedOutput.Scale)
	cfg.FrameRate = int(selectedOutput.RefreshRate)

	// Set display ID to include output selection
	cfg.DisplayID = fmt.Sprintf("wayland-%s", outputName)

	return nil
}
