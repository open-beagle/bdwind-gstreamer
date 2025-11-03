package gstreamer

import (
	"context"
	"fmt"
	"sync"

	"github.com/go-gst/go-gst/gst"
	"github.com/sirupsen/logrus"

	"github.com/open-beagle/bdwind-gstreamer/internal/config"
)

// SimpleGStreamerManager provides a simplified GStreamer management interface
// focused on core functionality without complex abstractions
type SimpleGStreamerManager struct {
	// Basic configuration
	config *config.GStreamerConfig
	logger *logrus.Entry

	// Direct pipeline management
	pipeline *gst.Pipeline
	source   *gst.Element
	encoder  *gst.Element
	sink     *gst.Element

	// Simple callback mechanism
	videoCallback func([]byte) error

	// Basic state management
	running bool
	mutex   sync.RWMutex

	// Context for lifecycle management
	ctx    context.Context
	cancel context.CancelFunc
}

// NewSimpleGStreamerManager creates a new simplified GStreamer manager
func NewSimpleGStreamerManager(cfg *config.GStreamerConfig) (*SimpleGStreamerManager, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	// Initialize GStreamer library
	gst.Init(nil)

	// Create context for lifecycle management
	ctx, cancel := context.WithCancel(context.Background())

	// Create logger
	logger := logrus.WithField("component", "simple-gstreamer")

	manager := &SimpleGStreamerManager{
		config: cfg,
		logger: logger,
		ctx:    ctx,
		cancel: cancel,
	}

	logger.Debug("Simple GStreamer manager created successfully")
	logger.Debugf("Configuration: display=%s, codec=%s, resolution=%dx%d@%dfps",
		cfg.Capture.DisplayID, cfg.Encoding.Codec,
		cfg.Capture.Width, cfg.Capture.Height, cfg.Capture.FrameRate)

	return manager, nil
}

// NewSimpleGStreamerManagerFromSimpleConfig creates a new simplified GStreamer manager from SimpleConfig
func NewSimpleGStreamerManagerFromSimpleConfig(cfg *config.SimpleConfig) (*SimpleGStreamerManager, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	// Get GStreamer config with direct access (no validation)
	gstreamerConfig := cfg.GetGStreamerConfig()

	// Initialize GStreamer library
	gst.Init(nil)

	// Create context for lifecycle management
	ctx, cancel := context.WithCancel(context.Background())

	// Create logger using simple config
	logger := cfg.GetLoggerWithPrefix("simple-gstreamer")

	manager := &SimpleGStreamerManager{
		config: gstreamerConfig,
		logger: logger,
		ctx:    ctx,
		cancel: cancel,
	}

	logger.Debug("Simple GStreamer manager created from SimpleConfig")
	logger.Debugf("Configuration: display=%s, codec=%s, resolution=%dx%d@%dfps",
		gstreamerConfig.Capture.DisplayID, gstreamerConfig.Encoding.Codec,
		gstreamerConfig.Capture.Width, gstreamerConfig.Capture.Height, gstreamerConfig.Capture.FrameRate)

	return manager, nil
}

// Start starts the GStreamer pipeline
func (m *SimpleGStreamerManager) Start() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.running {
		return fmt.Errorf("manager is already running")
	}

	m.logger.Info("Starting simple GStreamer manager...")

	// Create and configure pipeline
	if err := m.createPipeline(); err != nil {
		return fmt.Errorf("failed to create pipeline: %w", err)
	}

	// Set up sample callback mechanism
	if err := m.setupSampleCallback(); err != nil {
		return fmt.Errorf("failed to setup sample callback: %w", err)
	}

	// Start the pipeline
	if err := m.pipeline.SetState(gst.StatePlaying); err != nil {
		return fmt.Errorf("failed to start pipeline: %w", err)
	}

	m.running = true
	m.logger.Info("Simple GStreamer manager started successfully")

	return nil
}

// Stop stops the GStreamer pipeline
func (m *SimpleGStreamerManager) Stop() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if !m.running {
		return nil
	}

	m.logger.Info("Stopping simple GStreamer manager...")

	// Stop the pipeline
	if m.pipeline != nil {
		if err := m.pipeline.SetState(gst.StateNull); err != nil {
			m.logger.Warnf("Failed to stop pipeline: %v", err)
		}
		m.pipeline = nil
	}

	// Cancel context
	m.cancel()

	m.running = false
	m.logger.Info("Simple GStreamer manager stopped successfully")

	return nil
}

// IsRunning returns whether the manager is currently running
func (m *SimpleGStreamerManager) IsRunning() bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.running
}

// SetVideoCallback sets the callback function for processed video data
func (m *SimpleGStreamerManager) SetVideoCallback(callback func([]byte) error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.videoCallback = callback
	m.logger.Debug("Video callback set successfully")
}

// createPipeline creates and configures the GStreamer pipeline
func (m *SimpleGStreamerManager) createPipeline() error {
	m.logger.Debug("Creating GStreamer pipeline...")

	// Create pipeline
	pipeline, err := gst.NewPipeline("simple-pipeline")
	if err != nil {
		return fmt.Errorf("failed to create pipeline: %w", err)
	}
	m.pipeline = pipeline

	// Create video source based on configuration
	if err := m.createVideoSource(); err != nil {
		return fmt.Errorf("failed to create video source: %w", err)
	}

	// Create video encoder
	if err := m.createVideoEncoder(); err != nil {
		return fmt.Errorf("failed to create video encoder: %w", err)
	}

	// Create app sink for receiving encoded data
	if err := m.createAppSink(); err != nil {
		return fmt.Errorf("failed to create app sink: %w", err)
	}

	// Link pipeline elements
	if err := m.linkPipelineElements(); err != nil {
		return fmt.Errorf("failed to link pipeline elements: %w", err)
	}

	m.logger.Debug("GStreamer pipeline created successfully")
	return nil
}

// createVideoSource creates the video capture source element
func (m *SimpleGStreamerManager) createVideoSource() error {
	var sourceName string

	// Choose source based on configuration
	if m.config.Capture.UseWayland {
		sourceName = "waylandsink"
		m.logger.Debug("Using Wayland capture source")
	} else {
		sourceName = "ximagesrc"
		m.logger.Debug("Using X11 capture source")
	}

	source, err := gst.NewElement(sourceName)
	if err != nil {
		return fmt.Errorf("failed to create %s element: %w", sourceName, err)
	}
	m.source = source

	// Configure source properties
	if !m.config.Capture.UseWayland {
		// Configure X11 source
		if m.config.Capture.DisplayID != "" {
			source.SetProperty("display-name", m.config.Capture.DisplayID)
		}
		source.SetProperty("show-pointer", m.config.Capture.ShowPointer)
		source.SetProperty("use-damage", m.config.Capture.UseDamage)

		// Set capture region if specified
		if m.config.Capture.Width > 0 && m.config.Capture.Height > 0 {
			source.SetProperty("endx", m.config.Capture.Width)
			source.SetProperty("endy", m.config.Capture.Height)
		}
	}

	// Add source to pipeline
	if err := m.pipeline.Add(source); err != nil {
		return fmt.Errorf("failed to add source to pipeline: %w", err)
	}

	m.logger.Debugf("Video source created: %s", sourceName)
	return nil
}

// createVideoEncoder creates the video encoder element
func (m *SimpleGStreamerManager) createVideoEncoder() error {
	var encoderName string

	// Choose encoder based on codec configuration
	switch m.config.Encoding.Codec {
	case "h264":
		if m.config.Encoding.UseHardware {
			encoderName = "nvh264enc" // Try hardware encoder first
		} else {
			encoderName = "x264enc"
		}
	case "vp8":
		encoderName = "vp8enc"
	case "vp9":
		encoderName = "vp9enc"
	default:
		encoderName = "x264enc" // Default to H.264 software encoder
	}

	encoder, err := gst.NewElement(encoderName)
	if err != nil {
		// Fallback to software encoder if hardware encoder fails
		if m.config.Encoding.UseHardware && m.config.Encoding.Codec == "h264" {
			m.logger.Warnf("Hardware encoder %s failed, falling back to x264enc", encoderName)
			encoder, err = gst.NewElement("x264enc")
			if err != nil {
				return fmt.Errorf("failed to create fallback encoder: %w", err)
			}
		} else {
			return fmt.Errorf("failed to create encoder %s: %w", encoderName, err)
		}
	}
	m.encoder = encoder

	// Configure encoder properties
	if err := m.configureEncoder(encoder, encoderName); err != nil {
		return fmt.Errorf("failed to configure encoder: %w", err)
	}

	// Add encoder to pipeline
	if err := m.pipeline.Add(encoder); err != nil {
		return fmt.Errorf("failed to add encoder to pipeline: %w", err)
	}

	m.logger.Debugf("Video encoder created: %s", encoderName)
	return nil
}

// configureEncoder configures encoder properties based on configuration
func (m *SimpleGStreamerManager) configureEncoder(encoder *gst.Element, encoderName string) error {
	switch encoderName {
	case "x264enc":
		encoder.SetProperty("bitrate", m.config.Encoding.Bitrate)
		encoder.SetProperty("speed-preset", m.config.Encoding.Preset)
		if m.config.Encoding.ZeroLatency {
			encoder.SetProperty("tune", "zerolatency")
		}
		if m.config.Encoding.Profile != "" {
			encoder.SetProperty("profile", m.config.Encoding.Profile)
		}

	case "nvh264enc":
		encoder.SetProperty("bitrate", m.config.Encoding.Bitrate*1000) // nvh264enc expects bps
		encoder.SetProperty("preset", m.config.Encoding.Preset)
		if m.config.Encoding.ZeroLatency {
			encoder.SetProperty("zerolatency", true)
		}

	case "vp8enc":
		encoder.SetProperty("target-bitrate", m.config.Encoding.Bitrate*1000)
		encoder.SetProperty("deadline", 1) // Real-time encoding

	case "vp9enc":
		encoder.SetProperty("target-bitrate", m.config.Encoding.Bitrate*1000)
		encoder.SetProperty("deadline", 1) // Real-time encoding
	}

	return nil
}

// createAppSink creates the application sink for receiving encoded data
func (m *SimpleGStreamerManager) createAppSink() error {
	sink, err := gst.NewElement("appsink")
	if err != nil {
		return fmt.Errorf("failed to create appsink: %w", err)
	}
	m.sink = sink

	// Configure appsink properties
	sink.SetProperty("emit-signals", true)
	sink.SetProperty("sync", false)
	sink.SetProperty("max-buffers", 10)
	sink.SetProperty("drop", true)

	// Add sink to pipeline
	if err := m.pipeline.Add(sink); err != nil {
		return fmt.Errorf("failed to add sink to pipeline: %w", err)
	}

	m.logger.Debug("App sink created successfully")
	return nil
}

// linkPipelineElements links all pipeline elements together
func (m *SimpleGStreamerManager) linkPipelineElements() error {
	// Create video conversion and scaling elements for compatibility
	videoconvert, err := gst.NewElement("videoconvert")
	if err != nil {
		return fmt.Errorf("failed to create videoconvert: %w", err)
	}

	videoscale, err := gst.NewElement("videoscale")
	if err != nil {
		return fmt.Errorf("failed to create videoscale: %w", err)
	}

	// Add conversion elements to pipeline
	if err := m.pipeline.Add(videoconvert); err != nil {
		return fmt.Errorf("failed to add videoconvert: %w", err)
	}
	if err := m.pipeline.Add(videoscale); err != nil {
		return fmt.Errorf("failed to add videoscale: %w", err)
	}

	// Create caps filter for video format
	capsFilter, err := gst.NewElement("capsfilter")
	if err != nil {
		return fmt.Errorf("failed to create caps filter: %w", err)
	}

	// Set video caps - use NV12 format for NVIDIA encoder compatibility
	format := "NV12" // Default format for NVIDIA encoders
	if m.config.Encoding.Codec != "h264" || !m.config.Encoding.UseHardware {
		format = "I420" // Use I420 for software encoders
	}

	caps := gst.NewCapsFromString(fmt.Sprintf(
		"video/x-raw,format=%s,width=%d,height=%d,framerate=%d/1",
		format,
		m.config.Capture.Width,
		m.config.Capture.Height,
		m.config.Capture.FrameRate,
	))
	capsFilter.SetProperty("caps", caps)

	if err := m.pipeline.Add(capsFilter); err != nil {
		return fmt.Errorf("failed to add caps filter: %w", err)
	}

	// Link elements: source -> videoconvert -> videoscale -> capsfilter -> encoder -> sink
	if err := gst.ElementLinkMany(m.source, videoconvert, videoscale, capsFilter, m.encoder, m.sink); err != nil {
		return fmt.Errorf("failed to link pipeline elements: %w", err)
	}

	m.logger.Debug("Pipeline elements linked successfully")
	return nil
}

// setupSampleCallback sets up the callback mechanism for receiving encoded video data
func (m *SimpleGStreamerManager) setupSampleCallback() error {
	if m.sink == nil {
		return fmt.Errorf("app sink not created")
	}

	// Connect to the new-sample signal
	m.sink.Connect("new-sample", m.onNewSample)

	m.logger.Debug("Sample callback mechanism setup successfully")
	return nil
}

// onNewSample handles new samples from the GStreamer pipeline
func (m *SimpleGStreamerManager) onNewSample(sink *gst.Element) gst.FlowReturn {
	// Pull sample from appsink
	sample, err := sink.GetProperty("last-sample")
	if err != nil || sample == nil {
		m.logger.Warn("Failed to pull sample from appsink")
		return gst.FlowError
	}

	// Convert to gst.Sample
	gstSample, ok := sample.(*gst.Sample)
	if !ok {
		m.logger.Warn("Invalid sample type received")
		return gst.FlowError
	}

	// Extract buffer data
	buffer := gstSample.GetBuffer()
	if buffer == nil {
		m.logger.Warn("No buffer in sample")
		return gst.FlowError
	}

	// Get buffer data
	data := buffer.Map(gst.MapRead)
	if data == nil {
		m.logger.Warn("Failed to map buffer data")
		return gst.FlowReturn(1) // GST_FLOW_ERROR
	}
	defer buffer.Unmap()

	// Copy data to avoid memory issues
	videoData := make([]byte, len(data.Bytes()))
	copy(videoData, data.Bytes())

	// Call the registered callback if available
	m.mutex.RLock()
	callback := m.videoCallback
	m.mutex.RUnlock()

	if callback != nil {
		if err := callback(videoData); err != nil {
			m.logger.Warnf("Video callback failed: %v", err)
			return gst.FlowReturn(1) // GST_FLOW_ERROR
		}
	}

	return gst.FlowReturn(0) // GST_FLOW_OK
}

// processVideoData processes encoded video data and calls the callback
func (m *SimpleGStreamerManager) processVideoData(data []byte) error {
	if len(data) == 0 {
		return fmt.Errorf("empty video data")
	}

	m.mutex.RLock()
	callback := m.videoCallback
	m.mutex.RUnlock()

	if callback == nil {
		// No callback registered, just log and continue
		m.logger.Trace("No video callback registered, discarding data")
		return nil
	}

	// Call the callback with the video data
	if err := callback(data); err != nil {
		return fmt.Errorf("callback failed: %w", err)
	}

	return nil
}
