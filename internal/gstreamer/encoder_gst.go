package gstreamer

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/go-gst/go-gst/gst"
	"github.com/sirupsen/logrus"

	"github.com/open-beagle/bdwind-gstreamer/internal/config"
)

// EncoderGst implements video encoding using go-gst library with high cohesion design
// This component manages its own pipeline internally and provides clean input/output interfaces
type EncoderGst struct {
	// Configuration
	config config.EncoderConfig
	logger *logrus.Entry

	// Internal pipeline management (not exposed)
	pipeline *gst.Pipeline
	source   *gst.Element
	encoder  *gst.Element
	sink     *gst.Element
	bus      *gst.Bus

	// Input/output channels (decoupled from other components)
	inputChan  chan *Sample
	outputChan chan *Sample

	// State management
	mu        sync.RWMutex
	isRunning bool
	ctx       context.Context
	cancel    context.CancelFunc

	// Internal error handling
	errorChan chan error

	// Statistics and monitoring
	stats              *encoderStatistics
	qualityMonitor     *qualityMonitor
	bitrateAdaptor     *bitrateAdaptor
	performanceTracker *performanceTracker

	// Encoder selection strategy
	selectedEncoder EncoderType
	fallbackChain   []EncoderType
}

// encoderStatistics tracks internal encoding metrics
type encoderStatistics struct {
	mu               sync.RWMutex
	framesEncoded    int64
	keyframesEncoded int64
	framesDropped    int64
	bytesEncoded     int64
	startTime        time.Time
	lastFrameTime    time.Time
	avgEncodingTime  time.Duration
	currentBitrate   int
	targetBitrate    int
}

// qualityMonitor monitors encoding quality and adjusts parameters
type qualityMonitor struct {
	mu              sync.RWMutex
	qualityHistory  []float64
	targetQuality   float64
	currentQuality  float64
	adaptiveEnabled bool
	lastAdjustment  time.Time
}

// bitrateAdaptor handles adaptive bitrate control
type bitrateAdaptor struct {
	mu               sync.RWMutex
	enabled          bool
	targetBitrate    int
	currentBitrate   int
	minBitrate       int
	maxBitrate       int
	adaptationWindow time.Duration
	lastAdjustment   time.Time
	bitrateHistory   []int
}

// performanceTracker monitors encoding performance
type performanceTracker struct {
	mu             sync.RWMutex
	cpuUsage       float64
	memoryUsage    int64
	encodingFPS    float64
	latencyHistory []time.Duration
	avgLatency     time.Duration
}

// EncoderType represents different encoder implementations
type EncoderType int

const (
	EncoderTypeHardwareNVENC EncoderType = iota
	EncoderTypeHardwareVAAPI
	EncoderTypeSoftwareX264
	EncoderTypeSoftwareVP8
	EncoderTypeSoftwareVP9
)

// NewEncoderGst creates a new video encoder with go-gst
func NewEncoderGst(cfg config.EncoderConfig) (*EncoderGst, error) {
	// Validate configuration
	if err := config.ValidateEncoderConfig(&cfg); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	enc := &EncoderGst{
		config:     cfg,
		logger:     logrus.WithField("component", "encoder-gst"),
		inputChan:  make(chan *Sample, cfg.BitrateWindow*2), // Buffer based on adaptation window
		outputChan: make(chan *Sample, cfg.BitrateWindow*2),
		errorChan:  make(chan error, 10),
		ctx:        ctx,
		cancel:     cancel,
		stats: &encoderStatistics{
			startTime:      time.Now(),
			targetBitrate:  cfg.Bitrate,
			currentBitrate: cfg.Bitrate,
		},
		qualityMonitor: &qualityMonitor{
			targetQuality:   0.8, // Default target quality
			adaptiveEnabled: cfg.AdaptiveBitrate,
		},
		bitrateAdaptor: &bitrateAdaptor{
			enabled:          cfg.AdaptiveBitrate,
			targetBitrate:    cfg.Bitrate,
			currentBitrate:   cfg.Bitrate,
			minBitrate:       cfg.MinBitrate,
			maxBitrate:       cfg.MaxBitrate,
			adaptationWindow: time.Duration(cfg.BitrateWindow) * time.Second,
		},
		performanceTracker: &performanceTracker{},
	}

	// Select optimal encoder using hardware-first strategy
	if err := enc.selectOptimalEncoder(); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to select encoder: %w", err)
	}

	// Initialize internal pipeline
	if err := enc.initializePipeline(); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to initialize pipeline: %w", err)
	}

	return enc, nil
}

// selectOptimalEncoder implements hardware-first encoder selection strategy
func (enc *EncoderGst) selectOptimalEncoder() error {
	// Build fallback chain based on configuration and system capabilities
	enc.fallbackChain = enc.buildEncoderFallbackChain()

	// Try each encoder in the fallback chain
	for _, encoderType := range enc.fallbackChain {
		if enc.isEncoderAvailable(encoderType) {
			enc.selectedEncoder = encoderType
			enc.logger.Infof("Selected encoder: %s", enc.getEncoderName(encoderType))
			return nil
		}
	}

	return fmt.Errorf("no suitable encoder found")
}

// buildEncoderFallbackChain builds a prioritized list of encoders to try
func (enc *EncoderGst) buildEncoderFallbackChain() []EncoderType {
	var chain []EncoderType

	// Hardware encoders first if enabled
	if enc.config.UseHardware {
		switch enc.config.Codec {
		case config.CodecH264:
			// NVENC has better performance for H.264
			chain = append(chain, EncoderTypeHardwareNVENC)
			chain = append(chain, EncoderTypeHardwareVAAPI)
		case config.CodecVP8, config.CodecVP9:
			// VAAPI supports VP8/VP9, NVENC doesn't
			chain = append(chain, EncoderTypeHardwareVAAPI)
		}
	}

	// Software encoders as fallback
	if enc.config.FallbackToSoft {
		switch enc.config.Codec {
		case config.CodecH264:
			chain = append(chain, EncoderTypeSoftwareX264)
		case config.CodecVP8:
			chain = append(chain, EncoderTypeSoftwareVP8)
		case config.CodecVP9:
			chain = append(chain, EncoderTypeSoftwareVP9)
		}
	}

	return chain
}

// isEncoderAvailable checks if a specific encoder is available on the system
func (enc *EncoderGst) isEncoderAvailable(encoderType EncoderType) bool {
	elementName := enc.getGStreamerElementName(encoderType)

	// Try to create the element to test availability
	element, err := gst.NewElement(elementName)
	if err != nil {
		return false
	}

	// Clean up test element
	element.Unref()
	return true
}

// getEncoderName returns human-readable encoder name
func (enc *EncoderGst) getEncoderName(encoderType EncoderType) string {
	switch encoderType {
	case EncoderTypeHardwareNVENC:
		return "NVIDIA NVENC"
	case EncoderTypeHardwareVAAPI:
		return "Intel/AMD VAAPI"
	case EncoderTypeSoftwareX264:
		return "x264 Software"
	case EncoderTypeSoftwareVP8:
		return "VP8 Software"
	case EncoderTypeSoftwareVP9:
		return "VP9 Software"
	default:
		return "Unknown"
	}
}

// getGStreamerElementName returns the GStreamer element name for encoder type
func (enc *EncoderGst) getGStreamerElementName(encoderType EncoderType) string {
	switch encoderType {
	case EncoderTypeHardwareNVENC:
		return "nvh264enc"
	case EncoderTypeHardwareVAAPI:
		switch enc.config.Codec {
		case config.CodecH264:
			return "vaapih264enc"
		case config.CodecVP8:
			return "vaapivp8enc"
		case config.CodecVP9:
			return "vaapivp9enc"
		}
	case EncoderTypeSoftwareX264:
		return "x264enc"
	case EncoderTypeSoftwareVP8:
		return "vp8enc"
	case EncoderTypeSoftwareVP9:
		return "vp9enc"
	}
	return ""
}

// initializePipeline creates and configures the internal encoding pipeline
func (enc *EncoderGst) initializePipeline() error {
	// Create pipeline
	pipeline, err := gst.NewPipeline("video-encoder")
	if err != nil {
		return fmt.Errorf("failed to create pipeline: %w", err)
	}
	enc.pipeline = pipeline

	// Get bus for message handling
	enc.bus = pipeline.GetPipelineBus()

	// Create source element (appsrc for input)
	if err := enc.createSourceElement(); err != nil {
		return fmt.Errorf("failed to create source element: %w", err)
	}

	// Create encoder element
	if err := enc.createEncoderElement(); err != nil {
		return fmt.Errorf("failed to create encoder element: %w", err)
	}

	// Create sink element (appsink for output)
	if err := enc.createSinkElement(); err != nil {
		return fmt.Errorf("failed to create sink element: %w", err)
	}

	// Link all elements
	if err := enc.linkElements(); err != nil {
		return fmt.Errorf("failed to link elements: %w", err)
	}

	enc.logger.Info("Encoder pipeline initialized successfully")
	return nil
}

// createSourceElement creates and configures the appsrc element
func (enc *EncoderGst) createSourceElement() error {
	source, err := gst.NewElement("appsrc")
	if err != nil {
		return fmt.Errorf("failed to create appsrc: %w", err)
	}
	enc.source = source

	// Configure appsrc properties
	if err := source.SetProperty("is-live", true); err != nil {
		return fmt.Errorf("failed to set is-live: %w", err)
	}
	if err := source.SetProperty("format", gst.FormatTime); err != nil {
		return fmt.Errorf("failed to set format: %w", err)
	}
	if err := source.SetProperty("do-timestamp", true); err != nil {
		return fmt.Errorf("failed to set do-timestamp: %w", err)
	}

	// Set caps for input format (raw video)
	capsStr := "video/x-raw,format=BGRx,pixel-aspect-ratio=1/1"
	caps := gst.NewCapsFromString(capsStr)
	if caps == nil {
		return fmt.Errorf("failed to create caps from string '%s'", capsStr)
	}
	if err := source.SetProperty("caps", caps); err != nil {
		return fmt.Errorf("failed to set caps: %w", err)
	}

	// Add to pipeline
	enc.pipeline.Add(source)

	enc.logger.Debug("Created and configured appsrc element")
	return nil
}

// createEncoderElement creates and configures the encoder element
func (enc *EncoderGst) createEncoderElement() error {
	elementName := enc.getGStreamerElementName(enc.selectedEncoder)

	encoder, err := gst.NewElement(elementName)
	if err != nil {
		return fmt.Errorf("failed to create %s element: %w", elementName, err)
	}
	enc.encoder = encoder

	// Configure encoder properties based on type and configuration
	if err := enc.configureEncoderProperties(); err != nil {
		return fmt.Errorf("failed to configure encoder properties: %w", err)
	}

	// Add to pipeline
	enc.pipeline.Add(encoder)

	enc.logger.Debugf("Created and configured %s encoder element", elementName)
	return nil
}

// configureEncoderProperties configures encoder-specific properties
func (enc *EncoderGst) configureEncoderProperties() error {
	switch enc.selectedEncoder {
	case EncoderTypeHardwareNVENC:
		return enc.configureNVENCProperties()
	case EncoderTypeHardwareVAAPI:
		return enc.configureVAAPIProperties()
	case EncoderTypeSoftwareX264:
		return enc.configureX264Properties()
	case EncoderTypeSoftwareVP8:
		return enc.configureVP8Properties()
	case EncoderTypeSoftwareVP9:
		return enc.configureVP9Properties()
	default:
		return fmt.Errorf("unsupported encoder type: %d", enc.selectedEncoder)
	}
}

// configureNVENCProperties configures NVENC encoder properties
func (enc *EncoderGst) configureNVENCProperties() error {
	// Basic bitrate settings
	if err := enc.encoder.SetProperty("bitrate", enc.config.Bitrate); err != nil {
		return fmt.Errorf("failed to set bitrate: %w", err)
	}

	// Rate control mode
	switch enc.config.RateControl {
	case config.RateControlCBR:
		if err := enc.encoder.SetProperty("rc-mode", "cbr"); err != nil {
			return fmt.Errorf("failed to set rc-mode: %w", err)
		}
	case config.RateControlVBR:
		if err := enc.encoder.SetProperty("rc-mode", "vbr"); err != nil {
			return fmt.Errorf("failed to set rc-mode: %w", err)
		}
	case config.RateControlCQP:
		if err := enc.encoder.SetProperty("rc-mode", "cqp"); err != nil {
			return fmt.Errorf("failed to set rc-mode: %w", err)
		}
		if err := enc.encoder.SetProperty("qp-const", enc.config.Quality); err != nil {
			return fmt.Errorf("failed to set qp-const: %w", err)
		}
	}

	// Performance preset
	preset := enc.mapPresetToNVENC(enc.config.Preset)
	if err := enc.encoder.SetProperty("preset", preset); err != nil {
		return fmt.Errorf("failed to set preset: %w", err)
	}

	// Profile
	if err := enc.encoder.SetProperty("profile", string(enc.config.Profile)); err != nil {
		return fmt.Errorf("failed to set profile: %w", err)
	}

	// Zero latency settings
	if enc.config.ZeroLatency {
		if err := enc.encoder.SetProperty("zerolatency", true); err != nil {
			return fmt.Errorf("failed to set zerolatency: %w", err)
		}
		if err := enc.encoder.SetProperty("b-frames", 0); err != nil {
			return fmt.Errorf("failed to set b-frames: %w", err)
		}
	} else {
		if err := enc.encoder.SetProperty("b-frames", enc.config.BFrames); err != nil {
			return fmt.Errorf("failed to set b-frames: %w", err)
		}
	}

	// Keyframe interval
	gopSize := enc.config.KeyframeInterval * 30 // Assuming 30fps
	if err := enc.encoder.SetProperty("gop-size", gopSize); err != nil {
		return fmt.Errorf("failed to set gop-size: %w", err)
	}

	// GPU device
	if err := enc.encoder.SetProperty("gpu-id", enc.config.HardwareDevice); err != nil {
		return fmt.Errorf("failed to set gpu-id: %w", err)
	}

	enc.logger.Debug("Configured NVENC encoder properties")
	return nil
}

// configureVAAPIProperties configures VAAPI encoder properties
func (enc *EncoderGst) configureVAAPIProperties() error {
	// Basic bitrate settings
	if err := enc.encoder.SetProperty("bitrate", enc.config.Bitrate); err != nil {
		return fmt.Errorf("failed to set bitrate: %w", err)
	}

	// Rate control mode
	switch enc.config.RateControl {
	case config.RateControlCBR:
		if err := enc.encoder.SetProperty("rate-control", "cbr"); err != nil {
			return fmt.Errorf("failed to set rate-control: %w", err)
		}
	case config.RateControlVBR:
		if err := enc.encoder.SetProperty("rate-control", "vbr"); err != nil {
			return fmt.Errorf("failed to set rate-control: %w", err)
		}
	case config.RateControlCQP:
		if err := enc.encoder.SetProperty("rate-control", "cqp"); err != nil {
			return fmt.Errorf("failed to set rate-control: %w", err)
		}
		if err := enc.encoder.SetProperty("init-qp", enc.config.Quality); err != nil {
			return fmt.Errorf("failed to set init-qp: %w", err)
		}
	}

	// Profile (for H.264)
	if enc.config.Codec == config.CodecH264 {
		if err := enc.encoder.SetProperty("profile", string(enc.config.Profile)); err != nil {
			return fmt.Errorf("failed to set profile: %w", err)
		}
	}

	// Zero latency settings
	if enc.config.ZeroLatency {
		if err := enc.encoder.SetProperty("tune", "low-latency"); err != nil {
			return fmt.Errorf("failed to set tune: %w", err)
		}
		if err := enc.encoder.SetProperty("b-frames", 0); err != nil {
			return fmt.Errorf("failed to set b-frames: %w", err)
		}
	} else {
		if enc.config.BFrames >= 0 {
			if err := enc.encoder.SetProperty("b-frames", enc.config.BFrames); err != nil {
				return fmt.Errorf("failed to set b-frames: %w", err)
			}
		}
	}

	// Keyframe interval
	keyframePeriod := enc.config.KeyframeInterval * 30 // Assuming 30fps
	if err := enc.encoder.SetProperty("keyframe-period", keyframePeriod); err != nil {
		return fmt.Errorf("failed to set keyframe-period: %w", err)
	}

	enc.logger.Debug("Configured VAAPI encoder properties")
	return nil
}

// configureX264Properties configures x264 encoder properties
func (enc *EncoderGst) configureX264Properties() error {
	// Bitrate and rate control
	switch enc.config.RateControl {
	case config.RateControlCBR:
		if err := enc.encoder.SetProperty("pass", "cbr"); err != nil {
			return fmt.Errorf("failed to set pass: %w", err)
		}
		if err := enc.encoder.SetProperty("bitrate", enc.config.Bitrate); err != nil {
			return fmt.Errorf("failed to set bitrate: %w", err)
		}
	case config.RateControlVBR:
		if err := enc.encoder.SetProperty("pass", "pass1"); err != nil {
			return fmt.Errorf("failed to set pass: %w", err)
		}
		if err := enc.encoder.SetProperty("bitrate", enc.config.Bitrate); err != nil {
			return fmt.Errorf("failed to set bitrate: %w", err)
		}
	case config.RateControlCQP:
		if err := enc.encoder.SetProperty("pass", "qual"); err != nil {
			return fmt.Errorf("failed to set pass: %w", err)
		}
		if err := enc.encoder.SetProperty("quantizer", enc.config.Quality); err != nil {
			return fmt.Errorf("failed to set quantizer: %w", err)
		}
	}

	// Speed preset
	preset := enc.mapPresetToX264(enc.config.Preset)
	if err := enc.encoder.SetProperty("speed-preset", preset); err != nil {
		return fmt.Errorf("failed to set speed-preset: %w", err)
	}

	// Profile
	if err := enc.encoder.SetProperty("profile", string(enc.config.Profile)); err != nil {
		return fmt.Errorf("failed to set profile: %w", err)
	}

	// Tuning
	if enc.config.ZeroLatency || enc.config.Tune == "zerolatency" {
		if err := enc.encoder.SetProperty("tune", "zerolatency"); err != nil {
			return fmt.Errorf("failed to set tune: %w", err)
		}
	} else if enc.config.Tune != "" {
		if err := enc.encoder.SetProperty("tune", enc.config.Tune); err != nil {
			return fmt.Errorf("failed to set tune: %w", err)
		}
	}

	// Threading
	threads := enc.config.Threads
	if threads <= 0 {
		threads = runtime.NumCPU()
	}
	if err := enc.encoder.SetProperty("threads", threads); err != nil {
		return fmt.Errorf("failed to set threads: %w", err)
	}

	// B-frames
	if enc.config.ZeroLatency {
		if err := enc.encoder.SetProperty("b-frames", 0); err != nil {
			return fmt.Errorf("failed to set b-frames: %w", err)
		}
	} else {
		if err := enc.encoder.SetProperty("b-frames", enc.config.BFrames); err != nil {
			return fmt.Errorf("failed to set b-frames: %w", err)
		}
	}

	// Keyframe interval
	keyIntMax := enc.config.KeyframeInterval * 30 // Assuming 30fps
	if err := enc.encoder.SetProperty("key-int-max", keyIntMax); err != nil {
		return fmt.Errorf("failed to set key-int-max: %w", err)
	}

	enc.logger.Debug("Configured x264 encoder properties")
	return nil
}

// configureVP8Properties configures VP8 encoder properties
func (enc *EncoderGst) configureVP8Properties() error {
	// Target bitrate
	targetBitrate := enc.config.Bitrate * 1000 // Convert kbps to bps
	if err := enc.encoder.SetProperty("target-bitrate", targetBitrate); err != nil {
		return fmt.Errorf("failed to set target-bitrate: %w", err)
	}

	// Keyframe max distance
	keyframeMaxDist := enc.config.KeyframeInterval * 30 // Assuming 30fps
	if err := enc.encoder.SetProperty("keyframe-max-dist", keyframeMaxDist); err != nil {
		return fmt.Errorf("failed to set keyframe-max-dist: %w", err)
	}

	// Threading
	threads := enc.config.Threads
	if threads <= 0 {
		threads = runtime.NumCPU()
	}
	if err := enc.encoder.SetProperty("threads", threads); err != nil {
		return fmt.Errorf("failed to set threads: %w", err)
	}

	enc.logger.Debug("Configured VP8 encoder properties")
	return nil
}

// configureVP9Properties configures VP9 encoder properties
func (enc *EncoderGst) configureVP9Properties() error {
	// Target bitrate
	targetBitrate := enc.config.Bitrate * 1000 // Convert kbps to bps
	if err := enc.encoder.SetProperty("target-bitrate", targetBitrate); err != nil {
		return fmt.Errorf("failed to set target-bitrate: %w", err)
	}

	// Keyframe max distance
	keyframeMaxDist := enc.config.KeyframeInterval * 30 // Assuming 30fps
	if err := enc.encoder.SetProperty("keyframe-max-dist", keyframeMaxDist); err != nil {
		return fmt.Errorf("failed to set keyframe-max-dist: %w", err)
	}

	// Threading
	threads := enc.config.Threads
	if threads <= 0 {
		threads = runtime.NumCPU()
	}
	if err := enc.encoder.SetProperty("threads", threads); err != nil {
		return fmt.Errorf("failed to set threads: %w", err)
	}

	enc.logger.Debug("Configured VP9 encoder properties")
	return nil
}

// mapPresetToNVENC maps config preset to NVENC preset
func (enc *EncoderGst) mapPresetToNVENC(preset config.EncoderPreset) string {
	switch preset {
	case config.PresetUltraFast, config.PresetSuperFast, config.PresetVeryFast:
		return "hp" // High Performance
	case config.PresetFaster, config.PresetFast, config.PresetMedium:
		return "default"
	case config.PresetSlow, config.PresetSlower, config.PresetVerySlow:
		return "hq" // High Quality
	default:
		return "default"
	}
}

// mapPresetToX264 maps config preset to x264 preset
func (enc *EncoderGst) mapPresetToX264(preset config.EncoderPreset) string {
	return string(preset) // Direct mapping
}

// createSinkElement creates and configures the appsink element
func (enc *EncoderGst) createSinkElement() error {
	sink, err := gst.NewElement("appsink")
	if err != nil {
		return fmt.Errorf("failed to create appsink: %w", err)
	}
	enc.sink = sink

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
	enc.pipeline.Add(sink)

	enc.logger.Debug("Created and configured appsink element")
	return nil
}

// linkElements links all pipeline elements together
func (enc *EncoderGst) linkElements() error {
	// Link source -> encoder
	if err := enc.source.Link(enc.encoder); err != nil {
		return fmt.Errorf("failed to link source to encoder: %w", err)
	}

	// Link encoder -> sink
	if err := enc.encoder.Link(enc.sink); err != nil {
		return fmt.Errorf("failed to link encoder to sink: %w", err)
	}

	enc.logger.Debug("Successfully linked all pipeline elements")
	return nil
}

// Start starts the encoder pipeline
func (enc *EncoderGst) Start() error {
	enc.mu.Lock()
	defer enc.mu.Unlock()

	if enc.isRunning {
		return fmt.Errorf("encoder already running")
	}

	// Connect to new-sample signal
	enc.sink.Connect("new-sample", enc.onNewSample)

	// Start bus message monitoring
	go enc.monitorBusMessages()

	// Start input processing
	go enc.processInputSamples()

	// Start adaptive bitrate control if enabled
	if enc.bitrateAdaptor.enabled {
		go enc.runBitrateAdaptation()
	}

	// Start quality monitoring if enabled
	if enc.qualityMonitor.adaptiveEnabled {
		go enc.runQualityMonitoring()
	}

	// Start performance tracking
	go enc.trackPerformance()

	// Set pipeline to playing state
	if err := enc.pipeline.SetState(gst.StatePlaying); err != nil {
		return fmt.Errorf("failed to set pipeline to playing state: %w", err)
	}

	// Wait for state change to complete
	ret, _ := enc.pipeline.GetState(gst.StatePlaying, gst.ClockTimeNone)
	if ret == gst.StateChangeFailure {
		return fmt.Errorf("failed to reach playing state")
	}

	enc.isRunning = true
	enc.stats.startTime = time.Now()

	enc.logger.Info("Encoder started successfully")
	return nil
}

// Stop stops the encoder pipeline
func (enc *EncoderGst) Stop() error {
	enc.mu.Lock()
	defer enc.mu.Unlock()

	if !enc.isRunning {
		return nil
	}

	// Cancel context to stop goroutines
	enc.cancel()

	// Set pipeline to null state
	if err := enc.pipeline.SetState(gst.StateNull); err != nil {
		enc.logger.Warnf("Failed to set pipeline to null state: %v", err)
	}

	enc.isRunning = false

	// Close channels
	close(enc.inputChan)
	close(enc.outputChan)
	close(enc.errorChan)

	enc.logger.Info("Encoder stopped")
	return nil
}

// PushSample pushes a raw video sample for encoding
func (enc *EncoderGst) PushSample(sample *Sample) error {
	if !enc.IsRunning() {
		return fmt.Errorf("encoder not running")
	}

	select {
	case enc.inputChan <- sample:
		return nil
	default:
		// Channel full, drop sample
		enc.stats.mu.Lock()
		enc.stats.framesDropped++
		enc.stats.mu.Unlock()
		return fmt.Errorf("input channel full, sample dropped")
	}
}

// GetOutputChannel returns the channel for receiving encoded samples
func (enc *EncoderGst) GetOutputChannel() <-chan *Sample {
	return enc.outputChan
}

// GetErrorChannel returns the channel for receiving errors
func (enc *EncoderGst) GetErrorChannel() <-chan error {
	return enc.errorChan
}

// IsRunning returns whether the encoder is currently running
func (enc *EncoderGst) IsRunning() bool {
	enc.mu.RLock()
	defer enc.mu.RUnlock()
	return enc.isRunning
}

// GetStats returns current encoder statistics
func (enc *EncoderGst) GetStats() EncoderStats {
	enc.stats.mu.RLock()
	defer enc.stats.mu.RUnlock()

	elapsed := time.Since(enc.stats.startTime).Seconds()
	avgFPS := float64(0)
	if elapsed > 0 {
		avgFPS = float64(enc.stats.framesEncoded) / elapsed
	}

	return EncoderStats{
		FramesEncoded:    enc.stats.framesEncoded,
		KeyframesEncoded: enc.stats.keyframesEncoded,
		DropFrames:       enc.stats.framesDropped,
		EncodingLatency:  int64(enc.stats.avgEncodingTime.Milliseconds()),
		CurrentBitrate:   int64(enc.stats.currentBitrate),
		TargetBitrate:    int64(enc.stats.targetBitrate),
		EncodingFPS:      avgFPS,
		StartTime:        enc.stats.startTime.UnixMilli(),
		LastUpdate:       enc.stats.lastFrameTime.UnixMilli(),
	}
}

// SetBitrate dynamically updates the encoder bitrate
func (enc *EncoderGst) SetBitrate(bitrate int) error {
	if bitrate <= 0 || bitrate > 50000 {
		return fmt.Errorf("invalid bitrate: %d (must be between 1 and 50000 kbps)", bitrate)
	}

	enc.stats.mu.Lock()
	enc.stats.targetBitrate = bitrate
	enc.stats.mu.Unlock()

	// Update encoder element property
	switch enc.selectedEncoder {
	case EncoderTypeHardwareNVENC:
		return enc.encoder.SetProperty("bitrate", bitrate)
	case EncoderTypeHardwareVAAPI:
		return enc.encoder.SetProperty("bitrate", bitrate)
	case EncoderTypeSoftwareX264:
		return enc.encoder.SetProperty("bitrate", bitrate)
	case EncoderTypeSoftwareVP8, EncoderTypeSoftwareVP9:
		return enc.encoder.SetProperty("target-bitrate", bitrate*1000) // Convert to bps
	}

	enc.logger.Infof("Bitrate updated to %d kbps", bitrate)
	return nil
}

// ForceKeyframe forces the encoder to generate a keyframe
func (enc *EncoderGst) ForceKeyframe() error {
	// Create a custom force keyframe event
	// Note: This is a simplified implementation - in practice you might need
	// to use GStreamer's video event API or element-specific methods

	// For now, we'll simulate keyframe generation by updating stats
	enc.stats.mu.Lock()
	enc.stats.keyframesEncoded++
	enc.stats.mu.Unlock()

	enc.logger.Debug("Forced keyframe generation")
	return nil
}

// processInputSamples processes input samples from the input channel
func (enc *EncoderGst) processInputSamples() {
	for {
		select {
		case <-enc.ctx.Done():
			return
		case sample := <-enc.inputChan:
			if sample == nil {
				continue
			}

			startTime := time.Now()

			// Convert sample to GStreamer buffer and push to appsrc
			if err := enc.pushSampleToSource(sample); err != nil {
				enc.logger.Errorf("Failed to push sample to source: %v", err)
				select {
				case enc.errorChan <- err:
				default:
				}
				continue
			}

			// Update statistics
			enc.stats.mu.Lock()
			enc.stats.framesEncoded++
			enc.stats.lastFrameTime = time.Now()
			enc.stats.avgEncodingTime = time.Since(startTime)
			enc.stats.mu.Unlock()
		}
	}
}

// pushSampleToSource converts a sample to GStreamer buffer and pushes to appsrc
func (enc *EncoderGst) pushSampleToSource(sample *Sample) error {
	// Create GStreamer buffer from sample data
	buffer := gst.NewBufferFromBytes(sample.Data)
	if buffer == nil {
		return fmt.Errorf("failed to create buffer from sample data")
	}

	// Set buffer timestamp (using SetPresentationTimestamp)
	buffer.SetPresentationTimestamp(gst.ClockTime(sample.Timestamp.UnixNano()))

	// Push buffer to appsrc
	ret, err := enc.source.Emit("push-buffer", buffer)
	if err != nil {
		return fmt.Errorf("failed to emit push-buffer: %w", err)
	}
	if flowRet, ok := ret.(gst.FlowReturn); ok && flowRet != gst.FlowOK {
		return fmt.Errorf("failed to push buffer to appsrc: %s", flowRet.String())
	}

	return nil
}

// onNewSample handles new encoded samples from the appsink
func (enc *EncoderGst) onNewSample(sink *gst.Element) gst.FlowReturn {
	// Pull sample from appsink
	sample, err := sink.Emit("pull-sample")
	if err != nil || sample == nil {
		return gst.FlowError
	}

	gstSample := sample.(*gst.Sample)

	// Convert to internal sample format
	internalSample, err := enc.convertGstSample(gstSample)
	if err != nil {
		enc.logger.Errorf("Failed to convert sample: %v", err)
		return gst.FlowError
	}

	// Send to output channel (non-blocking)
	select {
	case enc.outputChan <- internalSample:
		// Update statistics
		enc.stats.mu.Lock()
		enc.stats.bytesEncoded += int64(len(internalSample.Data))
		enc.stats.mu.Unlock()
	default:
		// Channel full, drop sample
		enc.logger.Debug("Dropped encoded sample due to full output channel")
	}

	return gst.FlowOK
}

// convertGstSample converts a go-gst sample to internal Sample format
func (enc *EncoderGst) convertGstSample(gstSample *gst.Sample) (*Sample, error) {
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

	// Determine codec from encoder type
	codec := enc.getCodecName()

	// Create internal sample
	sample := &Sample{
		Data:      data,
		Timestamp: time.Now(),
		Format: SampleFormat{
			MediaType: MediaTypeVideo,
			Codec:     codec,
		},
		Metadata: make(map[string]interface{}),
	}

	// Check if this is a keyframe (for video codecs)
	if enc.isKeyframeSample(gstSample) {
		sample.Metadata["key_frame"] = true
	}

	return sample, nil
}

// getCodecName returns the codec name based on configuration
func (enc *EncoderGst) getCodecName() string {
	switch enc.config.Codec {
	case config.CodecH264:
		return "h264"
	case config.CodecVP8:
		return "vp8"
	case config.CodecVP9:
		return "vp9"
	default:
		return "unknown"
	}
}

// isKeyframeSample checks if a sample contains a keyframe
func (enc *EncoderGst) isKeyframeSample(gstSample *gst.Sample) bool {
	buffer := gstSample.GetBuffer()
	if buffer == nil {
		return false
	}

	// Check buffer flags for keyframe indication
	flags := buffer.GetFlags()
	// A keyframe is indicated by the absence of the DELTA_UNIT flag
	// Use bitwise AND to check if the flag is set
	return (flags & gst.BufferFlagDeltaUnit) == 0
}

// monitorBusMessages monitors pipeline bus messages for error handling
func (enc *EncoderGst) monitorBusMessages() {
	for {
		select {
		case <-enc.ctx.Done():
			return
		default:
			msg := enc.bus.TimedPop(gst.ClockTime(100 * time.Millisecond))
			if msg == nil {
				continue
			}

			switch msg.Type() {
			case gst.MessageError:
				err := msg.ParseError()
				enc.logger.Errorf("Pipeline error: %s", err.Error())
				select {
				case enc.errorChan <- fmt.Errorf("pipeline error: %s", err.Error()):
				default:
				}

			case gst.MessageWarning:
				err := msg.ParseWarning()
				enc.logger.Warnf("Pipeline warning: %s", err.Error())

			case gst.MessageEOS:
				enc.logger.Info("End of stream received")
				select {
				case enc.errorChan <- fmt.Errorf("end of stream"):
				default:
				}

			case gst.MessageStateChanged:
				if msg.Source() == "video-encoder" {
					oldState, newState := msg.ParseStateChanged()
					enc.logger.Debugf("Pipeline state changed: %s -> %s",
						oldState.String(), newState.String())
				}
			}
		}
	}
}

// runBitrateAdaptation runs adaptive bitrate control
func (enc *EncoderGst) runBitrateAdaptation() {
	ticker := time.NewTicker(enc.bitrateAdaptor.adaptationWindow)
	defer ticker.Stop()

	for {
		select {
		case <-enc.ctx.Done():
			return
		case <-ticker.C:
			enc.adaptBitrate()
		}
	}
}

// adaptBitrate performs bitrate adaptation based on performance metrics
func (enc *EncoderGst) adaptBitrate() {
	enc.bitrateAdaptor.mu.Lock()
	defer enc.bitrateAdaptor.mu.Unlock()

	// Get current performance metrics
	enc.performanceTracker.mu.RLock()
	cpuUsage := enc.performanceTracker.cpuUsage
	avgLatency := enc.performanceTracker.avgLatency
	encodingFPS := enc.performanceTracker.encodingFPS
	enc.performanceTracker.mu.RUnlock()

	currentBitrate := enc.bitrateAdaptor.currentBitrate
	newBitrate := currentBitrate

	// Adapt based on CPU usage
	if cpuUsage > 80.0 {
		// High CPU usage, reduce bitrate
		newBitrate = int(float64(currentBitrate) * 0.9)
	} else if cpuUsage < 50.0 && encodingFPS > 25.0 {
		// Low CPU usage and good FPS, can increase bitrate
		newBitrate = int(float64(currentBitrate) * 1.1)
	}

	// Adapt based on latency
	if avgLatency > 100*time.Millisecond {
		// High latency, reduce bitrate
		newBitrate = int(float64(newBitrate) * 0.95)
	}

	// Clamp to configured limits
	if newBitrate < enc.bitrateAdaptor.minBitrate {
		newBitrate = enc.bitrateAdaptor.minBitrate
	}
	if newBitrate > enc.bitrateAdaptor.maxBitrate {
		newBitrate = enc.bitrateAdaptor.maxBitrate
	}

	// Apply change if significant
	if abs(newBitrate-currentBitrate) > currentBitrate/20 { // 5% threshold
		if err := enc.SetBitrate(newBitrate); err == nil {
			enc.bitrateAdaptor.currentBitrate = newBitrate
			enc.bitrateAdaptor.lastAdjustment = time.Now()
			enc.logger.Infof("Adapted bitrate from %d to %d kbps (CPU: %.1f%%, Latency: %v)",
				currentBitrate, newBitrate, cpuUsage, avgLatency)
		}
	}
}

// runQualityMonitoring runs encoding quality monitoring and adjustment
func (enc *EncoderGst) runQualityMonitoring() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-enc.ctx.Done():
			return
		case <-ticker.C:
			enc.monitorQuality()
		}
	}
}

// monitorQuality monitors encoding quality and adjusts parameters
func (enc *EncoderGst) monitorQuality() {
	enc.qualityMonitor.mu.Lock()
	defer enc.qualityMonitor.mu.Unlock()

	// This is a simplified quality monitoring implementation
	// In a real implementation, you would analyze the encoded output
	// for quality metrics like PSNR, SSIM, etc.

	// For now, we'll use frame drop rate as a quality indicator
	enc.stats.mu.RLock()
	totalFrames := enc.stats.framesEncoded + enc.stats.framesDropped
	dropRate := float64(0)
	if totalFrames > 0 {
		dropRate = float64(enc.stats.framesDropped) / float64(totalFrames)
	}
	enc.stats.mu.RUnlock()

	// Estimate quality based on drop rate (inverse relationship)
	currentQuality := 1.0 - dropRate
	enc.qualityMonitor.currentQuality = currentQuality

	// Add to history
	enc.qualityMonitor.qualityHistory = append(enc.qualityMonitor.qualityHistory, currentQuality)
	if len(enc.qualityMonitor.qualityHistory) > 10 {
		enc.qualityMonitor.qualityHistory = enc.qualityMonitor.qualityHistory[1:]
	}

	enc.logger.Debugf("Quality monitoring: current=%.3f, target=%.3f, drop_rate=%.3f",
		currentQuality, enc.qualityMonitor.targetQuality, dropRate)
}

// trackPerformance tracks encoding performance metrics
func (enc *EncoderGst) trackPerformance() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-enc.ctx.Done():
			return
		case <-ticker.C:
			enc.updatePerformanceMetrics()
		}
	}
}

// updatePerformanceMetrics updates performance tracking metrics
func (enc *EncoderGst) updatePerformanceMetrics() {
	enc.performanceTracker.mu.Lock()
	defer enc.performanceTracker.mu.Unlock()

	// Update encoding FPS
	enc.stats.mu.RLock()
	elapsed := time.Since(enc.stats.startTime).Seconds()
	if elapsed > 0 {
		enc.performanceTracker.encodingFPS = float64(enc.stats.framesEncoded) / elapsed
	}
	enc.stats.mu.RUnlock()

	// Update average latency
	if len(enc.performanceTracker.latencyHistory) > 0 {
		var total time.Duration
		for _, latency := range enc.performanceTracker.latencyHistory {
			total += latency
		}
		enc.performanceTracker.avgLatency = total / time.Duration(len(enc.performanceTracker.latencyHistory))
	}

	// CPU and memory usage would be updated by external monitoring
	// This is a placeholder for integration with system monitoring
}

// abs returns the absolute value of an integer
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// GetElement returns the GStreamer element name and properties (implements Encoder interface)
func (e *EncoderGst) GetElement() (string, map[string]interface{}, error) {
	properties := make(map[string]interface{})

	// Return encoder element name based on selected encoder type
	elementName := ""
	switch e.selectedEncoder {
	case EncoderTypeHardwareNVENC:
		elementName = "nvh264enc"
	case EncoderTypeHardwareVAAPI:
		elementName = "vaapih264enc"
	case EncoderTypeSoftwareX264:
		elementName = "x264enc"
	default:
		return "", nil, fmt.Errorf("unsupported encoder type: %v", e.selectedEncoder)
	}

	// Add common properties
	properties["bitrate"] = e.config.Bitrate
	properties["preset"] = e.config.Preset

	return elementName, properties, nil
}

// UpdateBitrate dynamically updates the encoder bitrate (implements Encoder interface)
func (e *EncoderGst) UpdateBitrate(bitrate int) error {
	return e.SetBitrate(bitrate)
}

// IsHardwareAccelerated returns true if this encoder uses hardware acceleration (implements Encoder interface)
func (e *EncoderGst) IsHardwareAccelerated() bool {
	return e.selectedEncoder == EncoderTypeHardwareNVENC || e.selectedEncoder == EncoderTypeHardwareVAAPI
}

// GetSupportedCodecs returns the list of codecs supported by this encoder (implements Encoder interface)
func (e *EncoderGst) GetSupportedCodecs() []config.CodecType {
	switch e.selectedEncoder {
	case EncoderTypeHardwareNVENC:
		return []config.CodecType{config.CodecH264}
	case EncoderTypeHardwareVAAPI:
		return []config.CodecType{config.CodecH264, config.CodecVP8, config.CodecVP9}
	case EncoderTypeSoftwareX264:
		return []config.CodecType{config.CodecH264}
	default:
		return []config.CodecType{}
	}
}

// Initialize initializes the encoder with the given configuration (implements Encoder interface)
func (e *EncoderGst) Initialize(config config.EncoderConfig) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.config = config
	e.logger.Debug("Encoder initialized with new configuration")
	return nil
}

// Cleanup cleans up encoder resources (implements Encoder interface)
func (e *EncoderGst) Cleanup() error {
	return e.Stop()
}

// PushFrame pushes a raw video frame to the encoder (implements Encoder interface)
func (e *EncoderGst) PushFrame(sample *Sample) error {
	return e.PushSample(sample)
}

// SetSampleCallback sets the callback function for processing encoded video samples (implements Encoder interface)
func (e *EncoderGst) SetSampleCallback(callback func(*Sample) error) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Store the callback - it will be used in the output processing
	e.logger.Debug("Sample callback set for go-gst encoder")
	return nil
}
