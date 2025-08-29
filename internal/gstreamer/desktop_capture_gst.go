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
}

// captureStatistics tracks internal capture metrics
type captureStatistics struct {
	mu            sync.RWMutex
	framesTotal   int64
	framesDropped int64
	startTime     time.Time
	lastFrameTime time.Time
}

// NewDesktopCaptureGst creates a new desktop capture component with go-gst
func NewDesktopCaptureGst(cfg config.DesktopCaptureConfig) (*DesktopCaptureGst, error) {
	// Validate configuration
	if err := config.ValidateDesktopCaptureConfig(&cfg); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	dc := &DesktopCaptureGst{
		config:     cfg,
		logger:     logrus.WithField("component", "desktop-capture-gst"),
		sampleChan: make(chan *Sample, cfg.BufferSize),
		errorChan:  make(chan error, 10),
		ctx:        ctx,
		cancel:     cancel,
		stats: &captureStatistics{
			startTime: time.Now(),
		},
	}

	// Initialize internal pipeline
	if err := dc.initializePipeline(); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to initialize pipeline: %w", err)
	}

	return dc, nil
}

// initializePipeline creates and configures the internal GStreamer pipeline
func (dc *DesktopCaptureGst) initializePipeline() error {
	// Create pipeline
	pipeline, err := gst.NewPipeline("desktop-capture")
	if err != nil {
		return fmt.Errorf("failed to create pipeline: %w", err)
	}
	dc.pipeline = pipeline

	// Get bus for message handling
	dc.bus = pipeline.GetPipelineBus()

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

	// Configure source properties
	if err := dc.configureSourceProperties(); err != nil {
		return fmt.Errorf("failed to configure source properties: %w", err)
	}

	// Add to pipeline
	dc.pipeline.Add(source)

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

	// Configure damage events for optimization
	if err := dc.source.SetProperty("use-damage", dc.config.UseDamage); err != nil {
		return fmt.Errorf("failed to set use-damage: %w", err)
	}

	// Configure mouse pointer visibility
	if err := dc.source.SetProperty("show-pointer", dc.config.ShowPointer); err != nil {
		return fmt.Errorf("failed to set show-pointer: %w", err)
	}

	// Configure capture region if specified
	if dc.config.CaptureRegion != nil {
		region := dc.config.CaptureRegion
		if err := dc.source.SetProperty("startx", region.X); err != nil {
			return fmt.Errorf("failed to set startx: %w", err)
		}
		if err := dc.source.SetProperty("starty", region.Y); err != nil {
			return fmt.Errorf("failed to set starty: %w", err)
		}
		if err := dc.source.SetProperty("endx", region.X+region.Width-1); err != nil {
			return fmt.Errorf("failed to set endx: %w", err)
		}
		if err := dc.source.SetProperty("endy", region.Y+region.Height-1); err != nil {
			return fmt.Errorf("failed to set endy: %w", err)
		}
	}

	dc.logger.Debug("Configured X11 source properties")
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
		if err := dc.source.SetProperty("crop-x", region.X); err != nil {
			return fmt.Errorf("failed to set crop-x: %w", err)
		}
		if err := dc.source.SetProperty("crop-y", region.Y); err != nil {
			return fmt.Errorf("failed to set crop-y: %w", err)
		}
		if err := dc.source.SetProperty("crop-width", region.Width); err != nil {
			return fmt.Errorf("failed to set crop-width: %w", err)
		}
		if err := dc.source.SetProperty("crop-height", region.Height); err != nil {
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

	// Create video scale element
	videoScale, err := gst.NewElement("videoscale")
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to create videoscale: %w", err)
	}

	// Configure scaling method
	if err := videoScale.SetProperty("method", 1); err != nil { // Bilinear
		return nil, nil, nil, nil, fmt.Errorf("failed to set scale method: %w", err)
	}

	// Create video rate element for frame rate control
	videoRate, err := gst.NewElement("videorate")
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to create videorate: %w", err)
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

	// Configure caps
	capsStr := dc.buildCapsString()
	caps := gst.NewCapsFromString(capsStr)
	if caps == nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to create caps from string '%s'", capsStr)
	}

	if err := capsFilter.SetProperty("caps", caps); err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to set caps: %w", err)
	}

	// Add all elements to pipeline
	dc.pipeline.AddMany(videoConvert, videoScale, videoRate, capsFilter)

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

	// Configure appsink properties
	if err := sink.SetProperty("emit-signals", true); err != nil {
		return fmt.Errorf("failed to set emit-signals: %w", err)
	}
	if err := sink.SetProperty("sync", false); err != nil {
		return fmt.Errorf("failed to set sync: %w", err)
	}
	if err := sink.SetProperty("max-buffers", 1); err != nil {
		return fmt.Errorf("failed to set max-buffers: %w", err)
	}
	if err := sink.SetProperty("drop", true); err != nil {
		return fmt.Errorf("failed to set drop: %w", err)
	}

	// Add to pipeline
	dc.pipeline.Add(sink)

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

	// Cancel context to stop goroutines
	dc.cancel()

	// Set pipeline to null state
	if err := dc.pipeline.SetState(gst.StateNull); err != nil {
		dc.logger.Warnf("Failed to set pipeline to null state: %v", err)
	}

	dc.isRunning = false

	// Close channels
	close(dc.sampleChan)
	close(dc.errorChan)

	dc.logger.Info("Desktop capture stopped")
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
	}
}

// onNewSample handles new samples from the appsink
func (dc *DesktopCaptureGst) onNewSample(sink *gst.Element) gst.FlowReturn {
	// Pull sample from appsink
	sample, err := sink.Emit("pull-sample")
	if err != nil || sample == nil {
		return gst.FlowError
	}

	gstSample := sample.(*gst.Sample)

	// Convert to internal sample format
	internalSample, err := dc.convertGstSample(gstSample)
	if err != nil {
		dc.logger.Errorf("Failed to convert sample: %v", err)
		return gst.FlowError
	}

	// Send to sample channel (non-blocking)
	select {
	case dc.sampleChan <- internalSample:
		// Update statistics
		dc.stats.mu.Lock()
		dc.stats.framesTotal++
		dc.stats.lastFrameTime = time.Now()
		dc.stats.mu.Unlock()
	default:
		// Channel full, drop sample
		dc.stats.mu.Lock()
		dc.stats.framesDropped++
		dc.stats.mu.Unlock()
		dc.logger.Debug("Dropped sample due to full channel")
	}

	return gst.FlowOK
}

// convertGstSample converts a go-gst sample to internal Sample format
func (dc *DesktopCaptureGst) convertGstSample(gstSample *gst.Sample) (*Sample, error) {
	// Get buffer from sample
	buffer := gstSample.GetBuffer()
	if buffer == nil {
		return nil, fmt.Errorf("no buffer in sample")
	}

	// Map buffer for reading
	mapInfo := buffer.Map(gst.MapRead)
	if mapInfo == nil {
		return nil, fmt.Errorf("failed to map buffer")
	}
	defer buffer.Unmap()

	// Copy data
	data := make([]byte, len(mapInfo.Bytes()))
	copy(data, mapInfo.Bytes())

	// Get caps for format information
	caps := gstSample.GetCaps()
	if caps == nil {
		return nil, fmt.Errorf("no caps in sample")
	}

	// Parse format information
	structure := caps.GetStructureAt(0)
	width, _ := structure.GetValue("width")
	height, _ := structure.GetValue("height")

	// Create internal sample
	sample := &Sample{
		Data:      data,
		Timestamp: time.Now(),
		Format: SampleFormat{
			MediaType: MediaTypeVideo,
			Codec:     "raw",
			Width:     width.(int),
			Height:    height.(int),
		},
		Metadata: make(map[string]interface{}),
	}

	return sample, nil
}

// monitorBusMessages monitors pipeline bus messages for error handling
func (dc *DesktopCaptureGst) monitorBusMessages() {
	for {
		select {
		case <-dc.ctx.Done():
			return
		default:
			msg := dc.bus.TimedPop(gst.ClockTime(100 * time.Millisecond))
			if msg == nil {
				continue
			}

			switch msg.Type() {
			case gst.MessageError:
				err := msg.ParseError()
				dc.logger.Errorf("Pipeline error: %s", err.Error())
				select {
				case dc.errorChan <- fmt.Errorf("pipeline error: %s", err.Error()):
				default:
				}

			case gst.MessageWarning:
				err := msg.ParseWarning()
				dc.logger.Warnf("Pipeline warning: %s", err.Error())

			case gst.MessageEOS:
				dc.logger.Info("End of stream received")
				select {
				case dc.errorChan <- fmt.Errorf("end of stream"):
				default:
				}

			case gst.MessageStateChanged:
				if msg.Source() == "desktop-capture" {
					oldState, newState := msg.ParseStateChanged()
					dc.logger.Debugf("Pipeline state changed: %s -> %s",
						oldState.String(), newState.String())
				}
			}
		}
	}
}

// processSamples handles any additional sample processing if needed
func (dc *DesktopCaptureGst) processSamples() {
	// This goroutine can be used for additional sample processing
	// Currently, samples are processed directly in onNewSample
	// This provides a hook for future enhancements

	for {
		select {
		case <-dc.ctx.Done():
			return
		case <-time.After(time.Second):
			// Periodic maintenance tasks can go here
			// For example, statistics cleanup, health checks, etc.
		}
	}
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
