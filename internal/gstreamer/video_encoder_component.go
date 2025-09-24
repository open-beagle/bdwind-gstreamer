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

// VideoEncoderComponent implements high-cohesion video encoding using go-gst
// Supports hardware (NVENC/VAAPI) and software (x264) encoders with automatic selection
// Outputs WebRTC-compatible H.264/VP8/VP9 formats
type VideoEncoderComponent struct {
	// Configuration
	config config.EncoderConfig
	logger *logrus.Entry

	// go-gst encoding pipeline elements
	queue      *gst.Element // input queue
	converter  *gst.Element // videoconvert
	scaler     *gst.Element // videoscale
	encoder    *gst.Element // x264enc/nvh264enc/vaapih264enc
	parser     *gst.Element // h264parse/vp8parse/vp9parse
	capsfilter *gst.Element // output format filter

	// Input/Output pads
	inputPad  *gst.Pad
	outputPad *gst.Pad

	// Encoder selection strategy
	encoderStrategy EncoderStrategy // Auto/Hardware/Software
	selectedEncoder EncoderType     // Actual selected encoder
	codecType       CodecType       // H264/VP8/VP9

	// Quality control
	bitrateController *BitrateController
	qualityAdaptor    *QualityAdaptor

	// Output management
	publisher *StreamPublisher

	// Configuration and monitoring
	stats      *EncoderStatistics
	memManager *MemoryManager

	// Lifecycle management
	mu        sync.RWMutex
	isRunning bool
	ctx       context.Context
	cancel    context.CancelFunc
}

// EncoderStrategy represents the encoder selection strategy
type EncoderStrategy int

const (
	EncoderStrategyAuto EncoderStrategy = iota
	EncoderStrategyHardware
	EncoderStrategySoftware
)

// CodecType represents the video codec type
type CodecType int

const (
	CodecTypeH264 CodecType = iota
	CodecTypeVP8
	CodecTypeVP9
)

// EncoderStatistics tracks video encoding metrics
type EncoderStatistics struct {
	mu               sync.RWMutex
	FramesEncoded    int64     `json:"frames_encoded"`
	KeyFramesEncoded int64     `json:"keyframes_encoded"`
	FramesDropped    int64     `json:"frames_dropped"`
	BytesEncoded     int64     `json:"bytes_encoded"`
	StartTime        time.Time `json:"start_time"`
	LastFrameTime    time.Time `json:"last_frame_time"`
	AvgEncodingTime  int64     `json:"avg_encoding_time_ms"`
	CurrentBitrate   int       `json:"current_bitrate"`
	TargetBitrate    int       `json:"target_bitrate"`
	EncodingLatency  int64     `json:"encoding_latency_ms"`
	ErrorCount       int64     `json:"error_count"`
	QualityScore     float64   `json:"quality_score"`
}

// NewVideoEncoderComponent creates a new video encoder component
func NewVideoEncoderComponent(cfg config.EncoderConfig, memManager *MemoryManager, logger *logrus.Entry) (*VideoEncoderComponent, error) {
	if logger == nil {
		logger = logrus.WithField("component", "video-encoder")
	}

	ctx, cancel := context.WithCancel(context.Background())

	ve := &VideoEncoderComponent{
		config:     cfg,
		logger:     logger,
		memManager: memManager,
		ctx:        ctx,
		cancel:     cancel,
		stats: &EncoderStatistics{
			StartTime:      time.Now(),
			TargetBitrate:  cfg.Bitrate,
			CurrentBitrate: cfg.Bitrate,
		},
	}

	// Determine encoder and codec types
	ve.determineEncoderStrategy()

	return ve, nil
}

// Initialize initializes the video encoder component within a pipeline
func (ve *VideoEncoderComponent) Initialize(pipeline *gst.Pipeline) error {
	ve.mu.Lock()
	defer ve.mu.Unlock()

	if pipeline == nil {
		return fmt.Errorf("pipeline cannot be nil")
	}

	// Create input queue
	if err := ve.createInputQueue(); err != nil {
		return fmt.Errorf("failed to create input queue: %w", err)
	}

	// Create video processing elements
	if err := ve.createProcessingElements(); err != nil {
		return fmt.Errorf("failed to create processing elements: %w", err)
	}

	// Create encoder element
	if err := ve.createEncoderElement(); err != nil {
		return fmt.Errorf("failed to create encoder element: %w", err)
	}

	// Create parser element
	if err := ve.createParserElement(); err != nil {
		return fmt.Errorf("failed to create parser element: %w", err)
	}

	// Create output caps filter
	if err := ve.createOutputCapsFilter(); err != nil {
		return fmt.Errorf("failed to create output caps filter: %w", err)
	}

	// Add all elements to pipeline
	elements := []*gst.Element{ve.queue, ve.converter, ve.scaler, ve.encoder, ve.parser, ve.capsfilter}
	for _, element := range elements {
		if err := pipeline.Add(element); err != nil {
			return fmt.Errorf("failed to add element to pipeline: %w", err)
		}
	}

	// Link elements
	if err := ve.linkElements(); err != nil {
		return fmt.Errorf("failed to link elements: %w", err)
	}

	// Get input and output pads
	ve.inputPad = ve.queue.GetStaticPad("sink")
	ve.outputPad = ve.capsfilter.GetStaticPad("src")

	if ve.inputPad == nil || ve.outputPad == nil {
		return fmt.Errorf("failed to get input or output pads")
	}

	ve.logger.Info("Video encoder component initialized successfully")
	return nil
}

// ConnectToSource connects this encoder to a source component's output pad
func (ve *VideoEncoderComponent) ConnectToSource(sourcePad *gst.Pad) error {
	if sourcePad == nil {
		return fmt.Errorf("source pad cannot be nil")
	}

	if ve.inputPad == nil {
		return fmt.Errorf("input pad not initialized")
	}

	if linkResult := sourcePad.Link(ve.inputPad); linkResult != gst.PadLinkOK {
		return fmt.Errorf("failed to link source to encoder: %s", linkResult.String())
	}

	ve.logger.Debug("Connected video encoder to source")
	return nil
}

// Start starts the video encoder component
func (ve *VideoEncoderComponent) Start() error {
	ve.mu.Lock()
	defer ve.mu.Unlock()

	if ve.isRunning {
		return fmt.Errorf("video encoder component already running")
	}

	ve.isRunning = true
	ve.stats.StartTime = time.Now()

	ve.logger.Info("Video encoder component started")
	return nil
}

// Stop stops the video encoder component
func (ve *VideoEncoderComponent) Stop() error {
	ve.mu.Lock()
	defer ve.mu.Unlock()

	if !ve.isRunning {
		return nil
	}

	ve.cancel()
	ve.isRunning = false

	ve.logger.Info("Video encoder component stopped")
	return nil
}

// GetOutputPad returns the output pad for connecting to next component
func (ve *VideoEncoderComponent) GetOutputPad() *gst.Pad {
	return ve.outputPad
}

// GetStatistics returns current encoder statistics
func (ve *VideoEncoderComponent) GetStatistics() *EncoderStatistics {
	ve.stats.mu.RLock()
	defer ve.stats.mu.RUnlock()

	// Create a copy to return
	statsCopy := *ve.stats
	return &statsCopy
}

// SetPublisher sets the stream publisher for publishing encoded video data
func (ve *VideoEncoderComponent) SetPublisher(publisher *StreamPublisher) {
	ve.publisher = publisher
}

// AdaptQuality adapts encoding quality based on network conditions
func (ve *VideoEncoderComponent) AdaptQuality(networkCondition NetworkCondition) error {
	// Calculate target bitrate based on network conditions
	targetBitrate := ve.calculateTargetBitrate(networkCondition)

	// Apply bitrate changes to encoder
	if err := ve.applyBitrateChange(targetBitrate); err != nil {
		return fmt.Errorf("failed to apply bitrate change: %w", err)
	}

	ve.logger.Debugf("Adapted quality: bitrate=%d, rtt=%dms, loss=%.2f%%",
		targetBitrate, networkCondition.RTT, networkCondition.PacketLoss)

	return nil
}

// determineEncoderStrategy determines the best encoder and codec strategy
func (ve *VideoEncoderComponent) determineEncoderStrategy() {
	// Default to H.264 codec
	ve.codecType = CodecTypeH264

	// Check codec preference from config
	switch ve.config.Codec {
	case "h264":
		ve.codecType = CodecTypeH264
	case "vp8":
		ve.codecType = CodecTypeVP8
	case "vp9":
		ve.codecType = CodecTypeVP9
	default:
		ve.codecType = CodecTypeH264
	}

	// Determine encoder strategy based on config
	if ve.config.UseHardware {
		ve.encoderStrategy = EncoderStrategyHardware
	} else if ve.config.Type == config.EncoderTypeX264 {
		ve.encoderStrategy = EncoderStrategySoftware
	} else {
		ve.encoderStrategy = EncoderStrategyAuto
	}

	ve.logger.Debugf("Encoder strategy: strategy=%v, codec=%v", ve.encoderStrategy, ve.codecType)
}

// createInputQueue creates the input queue element
func (ve *VideoEncoderComponent) createInputQueue() error {
	queue, err := gst.NewElement("queue")
	if err != nil {
		return fmt.Errorf("failed to create input queue: %w", err)
	}

	ve.queue = queue

	// Register with memory manager
	if ve.memManager != nil {
		ve.memManager.RegisterObject(queue)
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
			ve.logger.Warnf("Failed to set queue property %s: %v", name, err)
		}
	}

	ve.logger.Debug("Created input queue element")
	return nil
}

// createProcessingElements creates video processing elements (convert and scale)
func (ve *VideoEncoderComponent) createProcessingElements() error {
	// Create videoconvert
	converter, err := gst.NewElement("videoconvert")
	if err != nil {
		return fmt.Errorf("failed to create videoconvert: %w", err)
	}
	ve.converter = converter

	// Create videoscale
	scaler, err := gst.NewElement("videoscale")
	if err != nil {
		return fmt.Errorf("failed to create videoscale: %w", err)
	}
	ve.scaler = scaler

	// Register with memory manager
	if ve.memManager != nil {
		ve.memManager.RegisterObject(converter)
		ve.memManager.RegisterObject(scaler)
	}

	// Configure videoscale properties
	if err := scaler.SetProperty("method", 1); err != nil { // bilinear
		ve.logger.Debug("Failed to set videoscale method")
	}

	ve.logger.Debug("Created video processing elements")
	return nil
}

// createEncoderElement creates the appropriate encoder element
func (ve *VideoEncoderComponent) createEncoderElement() error {
	var encoderName string
	var err error

	// Try to create encoder based on strategy
	if ve.encoderStrategy == EncoderStrategyAuto || ve.encoderStrategy == EncoderStrategyHardware {
		ve.selectedEncoder, encoderName, err = ve.selectHardwareEncoder()
		if err != nil {
			ve.logger.Warnf("Hardware encoder not available: %v, falling back to software", err)
			ve.selectedEncoder, encoderName = ve.selectSoftwareEncoder()
		}
	} else {
		ve.selectedEncoder, encoderName = ve.selectSoftwareEncoder()
	}

	encoder, err := gst.NewElement(encoderName)
	if err != nil {
		return fmt.Errorf("failed to create encoder %s: %w", encoderName, err)
	}

	ve.encoder = encoder

	// Register with memory manager
	if ve.memManager != nil {
		ve.memManager.RegisterObject(encoder)
	}

	// Configure encoder properties
	if err := ve.configureEncoder(); err != nil {
		return fmt.Errorf("failed to configure encoder: %w", err)
	}

	ve.logger.Infof("Created %s encoder", encoderName)
	return nil
}

// selectHardwareEncoder selects the best available hardware encoder
func (ve *VideoEncoderComponent) selectHardwareEncoder() (EncoderType, string, error) {
	switch ve.codecType {
	case CodecTypeH264:
		// Try NVENC first, then VAAPI
		if ve.isElementAvailable("nvh264enc") {
			return EncoderTypeHardwareNVENC, "nvh264enc", nil
		}
		if ve.isElementAvailable("vaapih264enc") {
			return EncoderTypeHardwareVAAPI, "vaapih264enc", nil
		}
		return 0, "", fmt.Errorf("no hardware H.264 encoder available")
	case CodecTypeVP8:
		if ve.isElementAvailable("vaapivp8enc") {
			return EncoderTypeHardwareVAAPI, "vaapivp8enc", nil
		}
		return 0, "", fmt.Errorf("no hardware VP8 encoder available")
	case CodecTypeVP9:
		if ve.isElementAvailable("vaapivp9enc") {
			return EncoderTypeHardwareVAAPI, "vaapivp9enc", nil
		}
		return 0, "", fmt.Errorf("no hardware VP9 encoder available")
	default:
		return 0, "", fmt.Errorf("unsupported codec type")
	}
}

// selectSoftwareEncoder selects the appropriate software encoder
func (ve *VideoEncoderComponent) selectSoftwareEncoder() (EncoderType, string) {
	switch ve.codecType {
	case CodecTypeH264:
		return EncoderTypeSoftwareX264, "x264enc"
	case CodecTypeVP8:
		return EncoderTypeSoftwareVP8, "vp8enc"
	case CodecTypeVP9:
		return EncoderTypeSoftwareVP9, "vp9enc"
	default:
		return EncoderTypeSoftwareX264, "x264enc"
	}
}

// isElementAvailable checks if a GStreamer element is available
func (ve *VideoEncoderComponent) isElementAvailable(elementName string) bool {
	element, err := gst.NewElement(elementName)
	if err != nil {
		return false
	}
	element.Unref()
	return true
}

// configureEncoder configures encoder properties based on config and type
func (ve *VideoEncoderComponent) configureEncoder() error {
	encoderName := ve.encoder.GetName()

	// Common properties for all encoders
	commonProps := map[string]interface{}{
		"bitrate": uint(ve.config.Bitrate / 1000), // Convert to kbps
	}

	// Encoder-specific properties
	switch {
	case encoderName == "x264enc":
		return ve.configureX264Encoder(commonProps)
	case encoderName == "nvh264enc":
		return ve.configureNVENCEncoder(commonProps)
	case encoderName == "vaapih264enc":
		return ve.configureVAAPIEncoder(commonProps)
	case encoderName == "vp8enc":
		return ve.configureVP8Encoder(commonProps)
	case encoderName == "vp9enc":
		return ve.configureVP9Encoder(commonProps)
	default:
		return ve.configureGenericEncoder(commonProps)
	}
}

// configureX264Encoder configures x264enc properties
func (ve *VideoEncoderComponent) configureX264Encoder(commonProps map[string]interface{}) error {
	properties := map[string]interface{}{
		"bitrate":      commonProps["bitrate"],
		"speed-preset": 6, // medium
		"tune":         0, // none
		"key-int-max":  uint(ve.config.KeyframeInterval),
		"pass":         0, // cbr
		"quantizer":    23,
		"threads":      uint(0), // auto
	}

	for name, value := range properties {
		if err := ve.encoder.SetProperty(name, value); err != nil {
			ve.logger.Warnf("Failed to set x264enc property %s: %v", name, err)
		}
	}

	ve.logger.Debug("Configured x264enc encoder")
	return nil
}

// configureNVENCEncoder configures nvh264enc properties
func (ve *VideoEncoderComponent) configureNVENCEncoder(commonProps map[string]interface{}) error {
	properties := map[string]interface{}{
		"bitrate":     commonProps["bitrate"],
		"gop-size":    int(ve.config.KeyframeInterval),
		"preset":      2, // medium
		"rc-mode":     1, // cbr
		"zerolatency": true,
	}

	for name, value := range properties {
		if err := ve.encoder.SetProperty(name, value); err != nil {
			ve.logger.Warnf("Failed to set nvh264enc property %s: %v", name, err)
		}
	}

	ve.logger.Debug("Configured nvh264enc encoder")
	return nil
}

// configureVAAPIEncoder configures vaapih264enc properties
func (ve *VideoEncoderComponent) configureVAAPIEncoder(commonProps map[string]interface{}) error {
	properties := map[string]interface{}{
		"bitrate":         commonProps["bitrate"],
		"keyframe-period": uint(ve.config.KeyframeInterval),
		"rate-control":    2,       // cbr
		"quality-level":   uint(4), // medium
	}

	for name, value := range properties {
		if err := ve.encoder.SetProperty(name, value); err != nil {
			ve.logger.Warnf("Failed to set vaapih264enc property %s: %v", name, err)
		}
	}

	ve.logger.Debug("Configured vaapih264enc encoder")
	return nil
}

// configureVP8Encoder configures vp8enc properties
func (ve *VideoEncoderComponent) configureVP8Encoder(commonProps map[string]interface{}) error {
	properties := map[string]interface{}{
		"target-bitrate":    int(ve.config.Bitrate),
		"keyframe-max-dist": uint(ve.config.KeyframeInterval),
		"end-usage":         1, // cbr
		"deadline":          1, // realtime
		"cpu-used":          4, // medium speed
	}

	for name, value := range properties {
		if err := ve.encoder.SetProperty(name, value); err != nil {
			ve.logger.Warnf("Failed to set vp8enc property %s: %v", name, err)
		}
	}

	ve.logger.Debug("Configured vp8enc encoder")
	return nil
}

// configureVP9Encoder configures vp9enc properties
func (ve *VideoEncoderComponent) configureVP9Encoder(commonProps map[string]interface{}) error {
	properties := map[string]interface{}{
		"target-bitrate":    int(ve.config.Bitrate),
		"keyframe-max-dist": uint(ve.config.KeyframeInterval),
		"end-usage":         1, // cbr
		"deadline":          1, // realtime
		"cpu-used":          4, // medium speed
	}

	for name, value := range properties {
		if err := ve.encoder.SetProperty(name, value); err != nil {
			ve.logger.Warnf("Failed to set vp9enc property %s: %v", name, err)
		}
	}

	ve.logger.Debug("Configured vp9enc encoder")
	return nil
}

// configureGenericEncoder configures generic encoder properties
func (ve *VideoEncoderComponent) configureGenericEncoder(commonProps map[string]interface{}) error {
	for name, value := range commonProps {
		if err := ve.encoder.SetProperty(name, value); err != nil {
			ve.logger.Warnf("Failed to set generic encoder property %s: %v", name, err)
		}
	}

	ve.logger.Debug("Configured generic encoder")
	return nil
}

// createParserElement creates the appropriate parser element
func (ve *VideoEncoderComponent) createParserElement() error {
	var parserName string

	switch ve.codecType {
	case CodecTypeH264:
		parserName = "h264parse"
	case CodecTypeVP8:
		parserName = "vp8parse"
	case CodecTypeVP9:
		parserName = "vp9parse"
	default:
		parserName = "h264parse"
	}

	parser, err := gst.NewElement(parserName)
	if err != nil {
		return fmt.Errorf("failed to create parser %s: %w", parserName, err)
	}

	ve.parser = parser

	// Register with memory manager
	if ve.memManager != nil {
		ve.memManager.RegisterObject(parser)
	}

	ve.logger.Debugf("Created %s parser", parserName)
	return nil
}

// createOutputCapsFilter creates the output caps filter
func (ve *VideoEncoderComponent) createOutputCapsFilter() error {
	capsfilter, err := gst.NewElement("capsfilter")
	if err != nil {
		return fmt.Errorf("failed to create output capsfilter: %w", err)
	}

	ve.capsfilter = capsfilter

	// Register with memory manager
	if ve.memManager != nil {
		ve.memManager.RegisterObject(capsfilter)
	}

	// Build output caps string
	capsStr := ve.buildOutputCapsString()
	caps := gst.NewCapsFromString(capsStr)
	if caps == nil {
		return fmt.Errorf("failed to create output caps from string: %s", capsStr)
	}

	// Register caps with memory manager
	if ve.memManager != nil {
		ve.memManager.RegisterObject(caps)
	}

	if err := capsfilter.SetProperty("caps", caps); err != nil {
		return fmt.Errorf("failed to set output caps: %w", err)
	}

	ve.logger.Debugf("Created output caps filter with caps: %s", capsStr)
	return nil
}

// buildOutputCapsString builds the output caps string for WebRTC compatibility
func (ve *VideoEncoderComponent) buildOutputCapsString() string {
	switch ve.codecType {
	case CodecTypeH264:
		return "video/x-h264,stream-format=byte-stream,alignment=au,profile=baseline"
	case CodecTypeVP8:
		return "video/x-vp8"
	case CodecTypeVP9:
		return "video/x-vp9"
	default:
		return "video/x-h264,stream-format=byte-stream,alignment=au,profile=baseline"
	}
}

// linkElements links all encoder elements together
func (ve *VideoEncoderComponent) linkElements() error {
	// Link queue -> converter
	if err := ve.queue.Link(ve.converter); err != nil {
		return fmt.Errorf("failed to link queue to converter: %w", err)
	}

	// Link converter -> scaler
	if err := ve.converter.Link(ve.scaler); err != nil {
		return fmt.Errorf("failed to link converter to scaler: %w", err)
	}

	// Link scaler -> encoder
	if err := ve.scaler.Link(ve.encoder); err != nil {
		return fmt.Errorf("failed to link scaler to encoder: %w", err)
	}

	// Link encoder -> parser
	if err := ve.encoder.Link(ve.parser); err != nil {
		return fmt.Errorf("failed to link encoder to parser: %w", err)
	}

	// Link parser -> capsfilter
	if err := ve.parser.Link(ve.capsfilter); err != nil {
		return fmt.Errorf("failed to link parser to capsfilter: %w", err)
	}

	ve.logger.Debug("Successfully linked video encoder elements")
	return nil
}

// calculateTargetBitrate calculates target bitrate based on network conditions
func (ve *VideoEncoderComponent) calculateTargetBitrate(networkCondition NetworkCondition) int {
	baseBitrate := ve.config.Bitrate

	// Adjust based on available bandwidth
	if networkCondition.Bandwidth > 0 {
		// Use 80% of available bandwidth
		maxBitrate := int(float64(networkCondition.Bandwidth) * 0.8)
		if maxBitrate < baseBitrate {
			baseBitrate = maxBitrate
		}
	}

	// Adjust based on packet loss
	if networkCondition.PacketLoss > 0 {
		lossReduction := 1.0 - (networkCondition.PacketLoss / 100.0 * 0.5)
		baseBitrate = int(float64(baseBitrate) * lossReduction)
	}

	// Adjust based on RTT
	if networkCondition.RTT > 100 {
		rttReduction := 1.0 - (float64(networkCondition.RTT-100) / 1000.0 * 0.3)
		if rttReduction < 0.5 {
			rttReduction = 0.5
		}
		baseBitrate = int(float64(baseBitrate) * rttReduction)
	}

	// Ensure bitrate is within bounds
	minBitrate := ve.config.Bitrate / 4
	maxBitrate := ve.config.Bitrate * 2
	if baseBitrate < minBitrate {
		baseBitrate = minBitrate
	}
	if baseBitrate > maxBitrate {
		baseBitrate = maxBitrate
	}

	return baseBitrate
}

// applyBitrateChange applies bitrate changes to the encoder
func (ve *VideoEncoderComponent) applyBitrateChange(targetBitrate int) error {
	if ve.encoder == nil {
		return fmt.Errorf("encoder not initialized")
	}

	// Convert to kbps for most encoders
	bitrateKbps := uint(targetBitrate / 1000)

	// Apply bitrate change based on encoder type
	encoderName := ve.encoder.GetName()
	switch encoderName {
	case "x264enc", "nvh264enc", "vaapih264enc":
		if err := ve.encoder.SetProperty("bitrate", bitrateKbps); err != nil {
			return fmt.Errorf("failed to set bitrate: %w", err)
		}
	case "vp8enc", "vp9enc":
		if err := ve.encoder.SetProperty("target-bitrate", targetBitrate); err != nil {
			return fmt.Errorf("failed to set target-bitrate: %w", err)
		}
	default:
		// Try both properties
		if err := ve.encoder.SetProperty("bitrate", bitrateKbps); err != nil {
			if err := ve.encoder.SetProperty("target-bitrate", targetBitrate); err != nil {
				return fmt.Errorf("failed to set bitrate on encoder: %w", err)
			}
		}
	}

	// Update statistics
	ve.stats.mu.Lock()
	ve.stats.TargetBitrate = targetBitrate
	ve.stats.CurrentBitrate = targetBitrate
	ve.stats.mu.Unlock()

	return nil
}

// IsRunning returns whether the component is currently running
func (ve *VideoEncoderComponent) IsRunning() bool {
	ve.mu.RLock()
	defer ve.mu.RUnlock()
	return ve.isRunning
}

// GetState returns the current component state
func (ve *VideoEncoderComponent) GetState() ComponentState {
	if ve.IsRunning() {
		return ComponentStateStarted
	}
	return ComponentStateStopped
}
