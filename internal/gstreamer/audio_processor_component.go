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

// AudioProcessorComponent implements high-cohesion audio processing using go-gst
// Handles audio capture and encoding with PulseAudio/ALSA support
// Supports Opus/PCM audio encoding and publishes encoded audio data
type AudioProcessorComponent struct {
	// Configuration
	config config.AudioConfig
	logger *logrus.Entry

	// go-gst audio pipeline elements
	source     *gst.Element // pulsesrc/alsasrc
	converter  *gst.Element // audioconvert
	resampler  *gst.Element // audioresample
	encoder    *gst.Element // opusenc/identity
	capsfilter *gst.Element // audio format filter
	queue      *gst.Element // output queue

	// Input/Output pads
	outputPad *gst.Pad

	// Audio source type
	sourceType AudioSourceType

	// Output management
	publisher *StreamPublisher

	// Configuration and monitoring
	stats      *AudioStatistics
	memManager *MemoryManager

	// Lifecycle management
	mu        sync.RWMutex
	isRunning bool
	ctx       context.Context
	cancel    context.CancelFunc
}

// AudioSourceType represents the type of audio source
type AudioSourceType int

const (
	AudioSourceTypeAuto AudioSourceType = iota
	AudioSourceTypePulseAudio
	AudioSourceTypeALSA
	AudioSourceTypeTest
)

// AudioStatistics tracks audio processing metrics
type AudioStatistics struct {
	mu                sync.RWMutex
	FramesProcessed   int64     `json:"frames_processed"`
	FramesDropped     int64     `json:"frames_dropped"`
	BytesProcessed    int64     `json:"bytes_processed"`
	StartTime         time.Time `json:"start_time"`
	LastFrameTime     time.Time `json:"last_frame_time"`
	ProcessingLatency int64     `json:"processing_latency_ms"`
	ErrorCount        int64     `json:"error_count"`
	CurrentSampleRate int       `json:"current_sample_rate"`
	CurrentChannels   int       `json:"current_channels"`
	CurrentBitrate    int       `json:"current_bitrate"`
	AudioLevel        float64   `json:"audio_level"`
}

// NewAudioProcessorComponent creates a new audio processor component
func NewAudioProcessorComponent(cfg config.AudioConfig, memManager *MemoryManager, logger *logrus.Entry) (*AudioProcessorComponent, error) {
	if logger == nil {
		logger = logrus.WithField("component", "audio-processor")
	}

	ctx, cancel := context.WithCancel(context.Background())

	ap := &AudioProcessorComponent{
		config:     cfg,
		logger:     logger,
		memManager: memManager,
		ctx:        ctx,
		cancel:     cancel,
		stats: &AudioStatistics{
			StartTime:         time.Now(),
			CurrentSampleRate: cfg.SampleRate,
			CurrentChannels:   cfg.Channels,
			CurrentBitrate:    cfg.Bitrate,
		},
	}

	// Determine audio source type
	ap.determineAudioSourceType()

	return ap, nil
}

// Initialize initializes the audio processor component within a pipeline
func (ap *AudioProcessorComponent) Initialize(pipeline *gst.Pipeline) error {
	ap.mu.Lock()
	defer ap.mu.Unlock()

	if pipeline == nil {
		return fmt.Errorf("pipeline cannot be nil")
	}

	// Create audio source
	if err := ap.createAudioSource(); err != nil {
		return fmt.Errorf("failed to create audio source: %w", err)
	}

	// Create audio processing elements
	if err := ap.createProcessingElements(); err != nil {
		return fmt.Errorf("failed to create processing elements: %w", err)
	}

	// Create encoder element
	if err := ap.createEncoderElement(); err != nil {
		return fmt.Errorf("failed to create encoder element: %w", err)
	}

	// Create caps filter
	if err := ap.createCapsFilter(); err != nil {
		return fmt.Errorf("failed to create caps filter: %w", err)
	}

	// Create output queue
	if err := ap.createOutputQueue(); err != nil {
		return fmt.Errorf("failed to create output queue: %w", err)
	}

	// Add all elements to pipeline
	elements := []*gst.Element{ap.source, ap.converter, ap.resampler, ap.encoder, ap.capsfilter, ap.queue}
	for _, element := range elements {
		if err := pipeline.Add(element); err != nil {
			return fmt.Errorf("failed to add element to pipeline: %w", err)
		}
	}

	// Link elements
	if err := ap.linkElements(); err != nil {
		return fmt.Errorf("failed to link elements: %w", err)
	}

	// Get output pad
	ap.outputPad = ap.queue.GetStaticPad("src")
	if ap.outputPad == nil {
		return fmt.Errorf("failed to get output pad from queue")
	}

	ap.logger.Info("Audio processor component initialized successfully")
	return nil
}

// Start starts the audio processor component
func (ap *AudioProcessorComponent) Start() error {
	ap.mu.Lock()
	defer ap.mu.Unlock()

	if ap.isRunning {
		return fmt.Errorf("audio processor component already running")
	}

	ap.isRunning = true
	ap.stats.StartTime = time.Now()

	ap.logger.Info("Audio processor component started")
	return nil
}

// Stop stops the audio processor component
func (ap *AudioProcessorComponent) Stop() error {
	ap.mu.Lock()
	defer ap.mu.Unlock()

	if !ap.isRunning {
		return nil
	}

	ap.cancel()
	ap.isRunning = false

	ap.logger.Info("Audio processor component stopped")
	return nil
}

// GetOutputPad returns the output pad for connecting to next component
func (ap *AudioProcessorComponent) GetOutputPad() *gst.Pad {
	return ap.outputPad
}

// GetStatistics returns current audio processing statistics
func (ap *AudioProcessorComponent) GetStatistics() *AudioStatistics {
	ap.stats.mu.RLock()
	defer ap.stats.mu.RUnlock()

	// Create a copy to return
	statsCopy := *ap.stats
	return &statsCopy
}

// SetPublisher sets the stream publisher for publishing encoded audio data
func (ap *AudioProcessorComponent) SetPublisher(publisher *StreamPublisher) {
	ap.publisher = publisher
}

// determineAudioSourceType determines the best audio source type
func (ap *AudioProcessorComponent) determineAudioSourceType() {
	// Check if audio is enabled
	if !ap.config.Enabled {
		ap.sourceType = AudioSourceTypeTest
		return
	}

	// Check for explicit source type in config
	switch ap.config.Source {
	case "pulseaudio":
		ap.sourceType = AudioSourceTypePulseAudio
	case "alsa":
		ap.sourceType = AudioSourceTypeALSA
	case "test":
		ap.sourceType = AudioSourceTypeTest
	default:
		// Auto-detect
		ap.sourceType = ap.detectAudioSource()
	}

	ap.logger.Debugf("Audio source type determined: %v", ap.sourceType)
}

// detectAudioSource detects the best available audio source
func (ap *AudioProcessorComponent) detectAudioSource() AudioSourceType {
	// Check for PulseAudio first
	if ap.isPulseAudioAvailable() {
		ap.logger.Debug("Detected PulseAudio")
		return AudioSourceTypePulseAudio
	}

	// Check for ALSA
	if ap.isALSAAvailable() {
		ap.logger.Debug("Detected ALSA")
		return AudioSourceTypeALSA
	}

	// Default to test source
	ap.logger.Warn("No audio system detected, using test source")
	return AudioSourceTypeTest
}

// isPulseAudioAvailable checks if PulseAudio is available
func (ap *AudioProcessorComponent) isPulseAudioAvailable() bool {
	// Check for PulseAudio server
	if os.Getenv("PULSE_RUNTIME_PATH") != "" || os.Getenv("PULSE_SERVER") != "" {
		return true
	}

	// Check if pulsesrc element is available
	return ap.isElementAvailable("pulsesrc")
}

// isALSAAvailable checks if ALSA is available
func (ap *AudioProcessorComponent) isALSAAvailable() bool {
	// Check for ALSA devices
	if _, err := os.Stat("/proc/asound"); err == nil {
		return true
	}

	// Check if alsasrc element is available
	return ap.isElementAvailable("alsasrc")
}

// isElementAvailable checks if a GStreamer element is available
func (ap *AudioProcessorComponent) isElementAvailable(elementName string) bool {
	element, err := gst.NewElement(elementName)
	if err != nil {
		return false
	}
	element.Unref()
	return true
}

// createAudioSource creates the appropriate audio source element
func (ap *AudioProcessorComponent) createAudioSource() error {
	var elementName string
	var err error

	switch ap.sourceType {
	case AudioSourceTypePulseAudio:
		elementName = "pulsesrc"
	case AudioSourceTypeALSA:
		elementName = "alsasrc"
	case AudioSourceTypeTest:
		elementName = "audiotestsrc"
	default:
		elementName = "audiotestsrc"
	}

	source, err := gst.NewElement(elementName)
	if err != nil {
		// Fallback to test source if preferred source fails
		if elementName != "audiotestsrc" {
			ap.logger.Warnf("%s not available, falling back to audiotestsrc", elementName)
			source, err = gst.NewElement("audiotestsrc")
			if err != nil {
				return fmt.Errorf("failed to create audio source: %w", err)
			}
			ap.sourceType = AudioSourceTypeTest
		} else {
			return fmt.Errorf("failed to create audio source %s: %w", elementName, err)
		}
	}

	ap.source = source

	// Register with memory manager
	if ap.memManager != nil {
		ap.memManager.RegisterObject(source)
	}

	// Configure source properties
	if err := ap.configureAudioSource(); err != nil {
		return fmt.Errorf("failed to configure audio source: %w", err)
	}

	ap.logger.Infof("Created %s audio source", elementName)
	return nil
}

// configureAudioSource configures audio source properties
func (ap *AudioProcessorComponent) configureAudioSource() error {
	switch ap.sourceType {
	case AudioSourceTypePulseAudio:
		return ap.configurePulseAudioSource()
	case AudioSourceTypeALSA:
		return ap.configureALSASource()
	case AudioSourceTypeTest:
		return ap.configureTestAudioSource()
	default:
		return nil
	}
}

// configurePulseAudioSource configures PulseAudio source properties
func (ap *AudioProcessorComponent) configurePulseAudioSource() error {
	properties := map[string]interface{}{
		"provide-clock": false,
		"slave-method":  1, // re-timestamp
	}

	// Set device if specified
	if ap.config.Device != "" {
		properties["device"] = ap.config.Device
	}

	// Set client name
	properties["client-name"] = "bdwind-gstreamer"

	for name, value := range properties {
		if err := ap.source.SetProperty(name, value); err != nil {
			ap.logger.Warnf("Failed to set PulseAudio property %s: %v", name, err)
		}
	}

	ap.logger.Debug("Configured PulseAudio source")
	return nil
}

// configureALSASource configures ALSA source properties
func (ap *AudioProcessorComponent) configureALSASource() error {
	properties := map[string]interface{}{
		"provide-clock": false,
	}

	// Set device if specified
	if ap.config.Device != "" {
		properties["device"] = ap.config.Device
	} else {
		properties["device"] = "default"
	}

	for name, value := range properties {
		if err := ap.source.SetProperty(name, value); err != nil {
			ap.logger.Warnf("Failed to set ALSA property %s: %v", name, err)
		}
	}

	ap.logger.Debug("Configured ALSA source")
	return nil
}

// configureTestAudioSource configures test audio source properties
func (ap *AudioProcessorComponent) configureTestAudioSource() error {
	properties := map[string]interface{}{
		"wave":          4, // sine wave
		"freq":          440.0,
		"volume":        0.1,
		"is-live":       true,
		"provide-clock": false,
	}

	for name, value := range properties {
		if err := ap.source.SetProperty(name, value); err != nil {
			ap.logger.Warnf("Failed to set test audio property %s: %v", name, err)
		}
	}

	ap.logger.Debug("Configured test audio source")
	return nil
}

// createProcessingElements creates audio processing elements
func (ap *AudioProcessorComponent) createProcessingElements() error {
	// Create audioconvert
	converter, err := gst.NewElement("audioconvert")
	if err != nil {
		return fmt.Errorf("failed to create audioconvert: %w", err)
	}
	ap.converter = converter

	// Create audioresample
	resampler, err := gst.NewElement("audioresample")
	if err != nil {
		return fmt.Errorf("failed to create audioresample: %w", err)
	}
	ap.resampler = resampler

	// Register with memory manager
	if ap.memManager != nil {
		ap.memManager.RegisterObject(converter)
		ap.memManager.RegisterObject(resampler)
	}

	// Configure audioresample properties
	if err := resampler.SetProperty("quality", 4); err != nil { // medium quality
		ap.logger.Debug("Failed to set audioresample quality")
	}

	ap.logger.Debug("Created audio processing elements")
	return nil
}

// createEncoderElement creates the appropriate audio encoder element
func (ap *AudioProcessorComponent) createEncoderElement() error {
	var encoderName string

	switch ap.config.Codec {
	case "opus":
		encoderName = "opusenc"
	case "pcm":
		encoderName = "identity" // Pass-through for PCM
	default:
		encoderName = "opusenc" // Default to Opus
	}

	encoder, err := gst.NewElement(encoderName)
	if err != nil {
		// Fallback to identity if encoder not available
		if encoderName != "identity" {
			ap.logger.Warnf("%s not available, falling back to PCM", encoderName)
			encoder, err = gst.NewElement("identity")
			if err != nil {
				return fmt.Errorf("failed to create audio encoder: %w", err)
			}
		} else {
			return fmt.Errorf("failed to create audio encoder %s: %w", encoderName, err)
		}
	}

	ap.encoder = encoder

	// Register with memory manager
	if ap.memManager != nil {
		ap.memManager.RegisterObject(encoder)
	}

	// Configure encoder properties
	if err := ap.configureEncoder(encoderName); err != nil {
		return fmt.Errorf("failed to configure encoder: %w", err)
	}

	ap.logger.Infof("Created %s audio encoder", encoderName)
	return nil
}

// configureEncoder configures audio encoder properties
func (ap *AudioProcessorComponent) configureEncoder(encoderName string) error {
	switch encoderName {
	case "opusenc":
		return ap.configureOpusEncoder()
	case "identity":
		return ap.configureIdentityEncoder()
	default:
		return nil
	}
}

// configureOpusEncoder configures Opus encoder properties
func (ap *AudioProcessorComponent) configureOpusEncoder() error {
	properties := map[string]interface{}{
		"bitrate":                ap.config.Bitrate,
		"complexity":             5, // medium complexity
		"inband-fec":             true,
		"packet-loss-percentage": 1,
		"dtx":                    false,
		"audio-type":             2048, // generic audio
	}

	for name, value := range properties {
		if err := ap.encoder.SetProperty(name, value); err != nil {
			ap.logger.Warnf("Failed to set Opus encoder property %s: %v", name, err)
		}
	}

	ap.logger.Debug("Configured Opus encoder")
	return nil
}

// configureIdentityEncoder configures identity element (pass-through)
func (ap *AudioProcessorComponent) configureIdentityEncoder() error {
	// Identity element doesn't need special configuration for PCM pass-through
	ap.logger.Debug("Configured identity encoder for PCM")
	return nil
}

// createCapsFilter creates the audio caps filter
func (ap *AudioProcessorComponent) createCapsFilter() error {
	capsfilter, err := gst.NewElement("capsfilter")
	if err != nil {
		return fmt.Errorf("failed to create audio capsfilter: %w", err)
	}

	ap.capsfilter = capsfilter

	// Register with memory manager
	if ap.memManager != nil {
		ap.memManager.RegisterObject(capsfilter)
	}

	// Build caps string based on configuration
	capsStr := ap.buildCapsString()
	caps := gst.NewCapsFromString(capsStr)
	if caps == nil {
		return fmt.Errorf("failed to create audio caps from string: %s", capsStr)
	}

	// Register caps with memory manager
	if ap.memManager != nil {
		ap.memManager.RegisterObject(caps)
	}

	if err := capsfilter.SetProperty("caps", caps); err != nil {
		return fmt.Errorf("failed to set audio caps: %w", err)
	}

	ap.logger.Debugf("Created audio caps filter with caps: %s", capsStr)
	return nil
}

// buildCapsString builds the audio caps string based on configuration
func (ap *AudioProcessorComponent) buildCapsString() string {
	switch ap.config.Codec {
	case "opus":
		return fmt.Sprintf("audio/x-opus,channels=%d,rate=%d", ap.config.Channels, ap.config.SampleRate)
	case "pcm":
		return fmt.Sprintf("audio/x-raw,format=S16LE,channels=%d,rate=%d,layout=interleaved",
			ap.config.Channels, ap.config.SampleRate)
	default:
		return fmt.Sprintf("audio/x-opus,channels=%d,rate=%d", ap.config.Channels, ap.config.SampleRate)
	}
}

// createOutputQueue creates the output queue element
func (ap *AudioProcessorComponent) createOutputQueue() error {
	queue, err := gst.NewElement("queue")
	if err != nil {
		return fmt.Errorf("failed to create output queue: %w", err)
	}

	ap.queue = queue

	// Register with memory manager
	if ap.memManager != nil {
		ap.memManager.RegisterObject(queue)
	}

	// Configure queue properties
	properties := map[string]interface{}{
		"max-size-buffers": uint(100),
		"max-size-bytes":   uint(0),
		"max-size-time":    uint64(0),
		"leaky":            2, // downstream leaky
	}

	for name, value := range properties {
		if err := queue.SetProperty(name, value); err != nil {
			ap.logger.Warnf("Failed to set audio queue property %s: %v", name, err)
		}
	}

	ap.logger.Debug("Created audio output queue")
	return nil
}

// linkElements links all audio processing elements together
func (ap *AudioProcessorComponent) linkElements() error {
	// Link source -> converter
	if err := ap.source.Link(ap.converter); err != nil {
		return fmt.Errorf("failed to link source to converter: %w", err)
	}

	// Link converter -> resampler
	if err := ap.converter.Link(ap.resampler); err != nil {
		return fmt.Errorf("failed to link converter to resampler: %w", err)
	}

	// Link resampler -> encoder
	if err := ap.resampler.Link(ap.encoder); err != nil {
		return fmt.Errorf("failed to link resampler to encoder: %w", err)
	}

	// Link encoder -> capsfilter
	if err := ap.encoder.Link(ap.capsfilter); err != nil {
		return fmt.Errorf("failed to link encoder to capsfilter: %w", err)
	}

	// Link capsfilter -> queue
	if err := ap.capsfilter.Link(ap.queue); err != nil {
		return fmt.Errorf("failed to link capsfilter to queue: %w", err)
	}

	ap.logger.Debug("Successfully linked audio processing elements")
	return nil
}

// IsRunning returns whether the component is currently running
func (ap *AudioProcessorComponent) IsRunning() bool {
	ap.mu.RLock()
	defer ap.mu.RUnlock()
	return ap.isRunning
}

// GetState returns the current component state
func (ap *AudioProcessorComponent) GetState() ComponentState {
	if ap.IsRunning() {
		return ComponentStateStarted
	}
	return ComponentStateStopped
}

// String returns string representation of audio source type
func (ast AudioSourceType) String() string {
	switch ast {
	case AudioSourceTypeAuto:
		return "auto"
	case AudioSourceTypePulseAudio:
		return "pulseaudio"
	case AudioSourceTypeALSA:
		return "alsa"
	case AudioSourceTypeTest:
		return "test"
	default:
		return "unknown"
	}
}
