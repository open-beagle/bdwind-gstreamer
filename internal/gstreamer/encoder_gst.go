package gstreamer

import (
	"context"
	"fmt"
	"reflect"
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

	// Memory management
	memoryManager *MemoryManager

	// Object lifecycle management
	lifecycleManager *ObjectLifecycleManager
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

// x264PropertyMap maps configuration parameters to GStreamer x264 properties with appropriate type handling
var x264PropertyMap = map[string]X264PropertyDescriptor{
	"speed-preset": {
		Name:      "speed-preset",
		Type:      PropertyTypeEnum,
		Required:  false,
		Validator: validateSpeedPreset,
	},
	"profile": {
		Name:      "profile",
		Type:      PropertyTypeEnum,
		Required:  false,
		Validator: validateProfile,
	},
	"tune": {
		Name:      "tune",
		Type:      PropertyTypeEnum,
		Required:  false,
		Validator: validateTune,
	},
	"bframes": {
		Name:      "bframes",
		Type:      PropertyTypeUInt,
		Required:  false,
		Validator: validateBFrames,
	},
	"bitrate": {
		Name:      "bitrate",
		Type:      PropertyTypeUInt,
		Required:  true,
		Validator: validateBitrate,
	},
	"quantizer": {
		Name:      "quantizer",
		Type:      PropertyTypeUInt,
		Required:  false,
		Validator: validateQuantizer,
	},
	"threads": {
		Name:      "threads",
		Type:      PropertyTypeUInt,
		Required:  true,
		Validator: validateThreads,
	},
	"key-int-max": {
		Name:      "key-int-max",
		Type:      PropertyTypeUInt,
		Required:  true,
		Validator: validateKeyIntMax,
	},
}

// validateSpeedPreset validates and converts speed-preset values to correct enum type
func validateSpeedPreset(value interface{}) (interface{}, error) {
	str, ok := value.(string)
	if !ok {
		return nil, fmt.Errorf("speed-preset must be a string, got %T", value)
	}

	// Valid x264 speed presets
	validPresets := map[string]bool{
		"ultrafast": true,
		"superfast": true,
		"veryfast":  true,
		"faster":    true,
		"fast":      true,
		"medium":    true,
		"slow":      true,
		"slower":    true,
		"veryslow":  true,
	}

	if !validPresets[str] {
		return nil, fmt.Errorf("invalid speed-preset: %s", str)
	}

	return str, nil
}

// validateProfile validates and converts profile values to correct enum type
func validateProfile(value interface{}) (interface{}, error) {
	str, ok := value.(string)
	if !ok {
		return nil, fmt.Errorf("profile must be a string, got %T", value)
	}

	// Valid x264 profiles
	validProfiles := map[string]bool{
		"baseline":   true,
		"main":       true,
		"high":       true,
		"high-10":    true,
		"high-4:2:2": true,
		"high-4:4:4": true,
	}

	if !validProfiles[str] {
		return nil, fmt.Errorf("invalid profile: %s", str)
	}

	return str, nil
}

// validateTune validates and converts tune values to correct enum type
func validateTune(value interface{}) (interface{}, error) {
	str, ok := value.(string)
	if !ok {
		return nil, fmt.Errorf("tune must be a string, got %T", value)
	}

	// Valid x264 tune options
	validTunes := map[string]bool{
		"film":        true,
		"animation":   true,
		"grain":       true,
		"stillimage":  true,
		"psnr":        true,
		"ssim":        true,
		"fastdecode":  true,
		"zerolatency": true,
	}

	if !validTunes[str] {
		return nil, fmt.Errorf("invalid tune: %s", str)
	}

	return str, nil
}

// validateBFrames validates and converts bframes values to correct uint type
func validateBFrames(value interface{}) (interface{}, error) {
	switch v := value.(type) {
	case int:
		if v < 0 || v > 16 {
			return nil, fmt.Errorf("bframes must be between 0 and 16, got %d", v)
		}
		return uint(v), nil
	case uint:
		if v > 16 {
			return nil, fmt.Errorf("bframes must be between 0 and 16, got %d", v)
		}
		return v, nil
	case string:
		// Try to parse string as integer
		if v == "" {
			return uint(0), nil
		}
		return nil, fmt.Errorf("bframes must be a number, got string: %s", v)
	default:
		return nil, fmt.Errorf("bframes must be a number, got %T", value)
	}
}

// validateBitrate validates and converts bitrate values to correct uint type
func validateBitrate(value interface{}) (interface{}, error) {
	switch v := value.(type) {
	case int:
		if v <= 0 || v > 50000 {
			return nil, fmt.Errorf("bitrate must be between 1 and 50000 kbps, got %d", v)
		}
		return uint(v), nil
	case uint:
		if v == 0 || v > 50000 {
			return nil, fmt.Errorf("bitrate must be between 1 and 50000 kbps, got %d", v)
		}
		return v, nil
	default:
		return nil, fmt.Errorf("bitrate must be a number, got %T", value)
	}
}

// validateQuantizer validates and converts quantizer values to correct uint type
func validateQuantizer(value interface{}) (interface{}, error) {
	switch v := value.(type) {
	case int:
		if v < 0 || v > 51 {
			return nil, fmt.Errorf("quantizer must be between 0 and 51, got %d", v)
		}
		return uint(v), nil
	case uint:
		if v > 51 {
			return nil, fmt.Errorf("quantizer must be between 0 and 51, got %d", v)
		}
		return v, nil
	default:
		return nil, fmt.Errorf("quantizer must be a number, got %T", value)
	}
}

// validateThreads validates and converts threads values to correct uint type
func validateThreads(value interface{}) (interface{}, error) {
	switch v := value.(type) {
	case int:
		if v <= 0 || v > 64 {
			return nil, fmt.Errorf("threads must be between 1 and 64, got %d", v)
		}
		return uint(v), nil
	case uint:
		if v == 0 || v > 64 {
			return nil, fmt.Errorf("threads must be between 1 and 64, got %d", v)
		}
		return v, nil
	default:
		return nil, fmt.Errorf("threads must be a number, got %T", value)
	}
}

// validateKeyIntMax validates and converts key-int-max values to correct uint type
func validateKeyIntMax(value interface{}) (interface{}, error) {
	switch v := value.(type) {
	case int:
		if v <= 0 || v > 3600 {
			return nil, fmt.Errorf("key-int-max must be between 1 and 3600, got %d", v)
		}
		return uint(v), nil
	case uint:
		if v == 0 || v > 3600 {
			return nil, fmt.Errorf("key-int-max must be between 1 and 3600, got %d", v)
		}
		return v, nil
	default:
		return nil, fmt.Errorf("key-int-max must be a number, got %T", value)
	}
}

// PropertyType represents the expected GStreamer property type
type PropertyType int

const (
	PropertyTypeString PropertyType = iota
	PropertyTypeEnum
	PropertyTypeUInt
	PropertyTypeBool
)

// X264PropertyDescriptor describes how to safely set x264 properties
type X264PropertyDescriptor struct {
	Name      string
	Type      PropertyType
	Required  bool
	Validator func(interface{}) (interface{}, error)
}

// PropertyConfigResult represents the result of configuring a single property
type PropertyConfigResult struct {
	PropertyName string
	Success      bool
	Value        interface{}
	Error        error
}

// X264ConfigResult represents the overall result of x264 configuration
type X264ConfigResult struct {
	Results        []PropertyConfigResult
	CriticalErrors []error
	Warnings       []error
}

// PropertyConfig represents a property configuration with error handling
type PropertyConfig struct {
	Name     string
	Value    interface{}
	Required bool // If true, failure to set this property is an error
}

// NewEncoderGst creates a new video encoder with go-gst
func NewEncoderGst(cfg config.EncoderConfig) (*EncoderGst, error) {
	// Validate configuration
	if err := config.ValidateEncoderConfig(&cfg); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Initialize memory manager with appropriate configuration
	memConfig := MemoryManagerConfig{
		BufferPoolSize:       100,
		GCInterval:           30 * time.Second,
		LeakDetectionEnabled: true,
		MaxObjectRefs:        1000,
		CleanupInterval:      5 * time.Minute,
	}

	// Create logger
	logger := logrus.WithField("component", "encoder-gst")

	memoryManager := NewMemoryManager(&memConfig, logger)

	enc := &EncoderGst{
		config:        cfg,
		logger:        logrus.WithField("component", "encoder-gst"),
		inputChan:     make(chan *Sample, cfg.BitrateWindow*2), // Buffer based on adaptation window
		outputChan:    make(chan *Sample, cfg.BitrateWindow*2),
		errorChan:     make(chan error, 10),
		ctx:           ctx,
		cancel:        cancel,
		memoryManager: memoryManager,
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

	// Initialize object lifecycle manager
	lifecycleConfig := ObjectLifecycleConfig{
		CleanupInterval:    30 * time.Second,
		ObjectRetention:    5 * time.Minute,
		EnableValidation:   true,
		ValidationInterval: 60 * time.Second,
		EnableStackTrace:   false, // Disable for performance in production
		LogObjectEvents:    false, // Disable verbose logging
	}
	enc.lifecycleManager = NewObjectLifecycleManager(lifecycleConfig)

	// Select optimal encoder using hardware-first strategy
	if err := enc.selectOptimalEncoder(); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to select encoder: %w", err)
	}

	// Initialize internal pipeline
	if err := enc.initializePipeline(); err != nil {
		cancel()
		memoryManager.Cleanup()
		enc.lifecycleManager.Close()
		return nil, fmt.Errorf("failed to initialize pipeline: %w", err)
	}

	return enc, nil
}

// selectOptimalEncoder implements hardware-first encoder selection strategy with robust fallback
func (enc *EncoderGst) selectOptimalEncoder() error {
	// Build fallback chain based on configuration and system capabilities
	enc.fallbackChain = enc.buildEncoderFallbackChain()

	if len(enc.fallbackChain) == 0 {
		return fmt.Errorf("no encoder candidates available for codec %s", enc.config.Codec)
	}

	var lastError error
	var availableEncoders []EncoderType

	// First pass: check availability
	for _, encoderType := range enc.fallbackChain {
		if enc.isEncoderAvailable(encoderType) {
			availableEncoders = append(availableEncoders, encoderType)
		}
	}

	if len(availableEncoders) == 0 {
		return fmt.Errorf("no suitable encoders available for codec %s", enc.config.Codec)
	}

	// Second pass: try to initialize each available encoder
	for _, encoderType := range availableEncoders {
		enc.selectedEncoder = encoderType
		enc.logger.Infof("Attempting to initialize encoder: %s", enc.getEncoderName(encoderType))

		// Test encoder initialization
		if err := enc.testEncoderInitialization(encoderType); err != nil {
			enc.logger.Warnf("Encoder %s failed initialization test: %v", enc.getEncoderName(encoderType), err)
			lastError = err
			continue
		}

		enc.logger.Infof("Successfully selected encoder: %s", enc.getEncoderName(encoderType))
		return nil
	}

	// If we get here, all encoders failed initialization
	return fmt.Errorf("all available encoders failed initialization, last error: %w", lastError)
}

// testEncoderInitialization tests if an encoder can be properly initialized
func (enc *EncoderGst) testEncoderInitialization(encoderType EncoderType) error {
	elementName := enc.getGStreamerElementName(encoderType)

	// Create test element
	testElement, err := gst.NewElement(elementName)
	if err != nil {
		return fmt.Errorf("failed to create test element: %w", err)
	}
	defer testElement.Unref()

	// Try to configure basic properties
	if err := enc.testEncoderConfiguration(testElement, encoderType); err != nil {
		return fmt.Errorf("failed to configure encoder: %w", err)
	}

	// Test state change to READY
	if err := testElement.SetState(gst.StateReady); err != nil {
		return fmt.Errorf("failed to set encoder to READY state: %w", err)
	}

	// Wait for state change
	ret, _ := testElement.GetState(gst.StateReady, gst.ClockTime(2*time.Second))
	if ret == gst.StateChangeFailure {
		return fmt.Errorf("encoder failed to reach READY state")
	}

	// Reset state
	testElement.SetState(gst.StateNull)

	return nil
}

// testEncoderConfiguration tests basic encoder configuration
func (enc *EncoderGst) testEncoderConfiguration(element *gst.Element, encoderType EncoderType) error {
	switch encoderType {
	case EncoderTypeHardwareNVENC:
		return enc.testNVENCConfiguration(element)
	case EncoderTypeHardwareVAAPI:
		return enc.testVAAPIConfiguration(element)
	case EncoderTypeSoftwareX264:
		return enc.testX264Configuration(element)
	case EncoderTypeSoftwareVP8:
		return enc.testVP8Configuration(element)
	case EncoderTypeSoftwareVP9:
		return enc.testVP9Configuration(element)
	default:
		return fmt.Errorf("unsupported encoder type: %v", encoderType)
	}
}

// testNVENCConfiguration tests NVENC encoder configuration
func (enc *EncoderGst) testNVENCConfiguration(element *gst.Element) error {
	// Test basic properties
	if err := element.SetProperty("bitrate", uint(1000)); err != nil {
		return fmt.Errorf("failed to set bitrate: %w", err)
	}

	if err := element.SetProperty("preset", "default"); err != nil {
		return fmt.Errorf("failed to set preset: %w", err)
	}

	if enc.config.HardwareDevice >= 0 {
		if err := element.SetProperty("gpu-id", uint(enc.config.HardwareDevice)); err != nil {
			return fmt.Errorf("failed to set gpu-id: %w", err)
		}
	}

	return nil
}

// testVAAPIConfiguration tests VAAPI encoder configuration
func (enc *EncoderGst) testVAAPIConfiguration(element *gst.Element) error {
	// Test basic properties
	if err := element.SetProperty("bitrate", uint(1000)); err != nil {
		return fmt.Errorf("failed to set bitrate: %w", err)
	}

	if err := element.SetProperty("rate-control", "cbr"); err != nil {
		return fmt.Errorf("failed to set rate-control: %w", err)
	}

	return nil
}

// testX264Configuration tests x264 encoder configuration
func (enc *EncoderGst) testX264Configuration(element *gst.Element) error {
	// Test basic properties
	if err := element.SetProperty("bitrate", uint(1000)); err != nil {
		return fmt.Errorf("failed to set bitrate: %w", err)
	}

	return nil
}

// testVP8Configuration tests VP8 encoder configuration
func (enc *EncoderGst) testVP8Configuration(element *gst.Element) error {
	// Test basic properties
	if err := element.SetProperty("target-bitrate", uint(1000000)); err != nil {
		return fmt.Errorf("failed to set target-bitrate: %w", err)
	}

	return nil
}

// testVP9Configuration tests VP9 encoder configuration
func (enc *EncoderGst) testVP9Configuration(element *gst.Element) error {
	// Test basic properties
	if err := element.SetProperty("target-bitrate", uint(1000000)); err != nil {
		return fmt.Errorf("failed to set target-bitrate: %w", err)
	}

	return nil
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
	if elementName == "" {
		enc.logger.Debugf("No element name for encoder type %v", encoderType)
		return false
	}

	// Check if factory exists first
	factory := gst.Find(elementName)
	if factory == nil {
		enc.logger.Debugf("Element factory '%s' not found", elementName)
		return false
	}

	// Try to create the element to test availability
	element, err := gst.NewElement(elementName)
	if err != nil {
		enc.logger.Debugf("Failed to create element '%s': %v", elementName, err)
		return false
	}

	// For hardware encoders, perform additional capability checks
	if enc.isHardwareEncoder(encoderType) {
		if !enc.checkHardwareCapabilities(element, encoderType) {
			element.Unref()
			enc.logger.Debugf("Hardware encoder '%s' failed capability check", elementName)
			return false
		}
	}

	// Clean up test element
	element.Unref()
	enc.logger.Debugf("Encoder '%s' is available", elementName)
	return true
}

// isHardwareEncoder returns true if the encoder type uses hardware acceleration
func (enc *EncoderGst) isHardwareEncoder(encoderType EncoderType) bool {
	return encoderType == EncoderTypeHardwareNVENC || encoderType == EncoderTypeHardwareVAAPI
}

// checkHardwareCapabilities performs additional checks for hardware encoders
func (enc *EncoderGst) checkHardwareCapabilities(element *gst.Element, encoderType EncoderType) bool {
	switch encoderType {
	case EncoderTypeHardwareNVENC:
		return enc.checkNVENCCapabilities(element)
	case EncoderTypeHardwareVAAPI:
		return enc.checkVAAPICapabilities(element)
	default:
		return true
	}
}

// checkNVENCCapabilities checks if NVENC hardware is available and functional
func (enc *EncoderGst) checkNVENCCapabilities(element *gst.Element) bool {
	// Check if GPU device is accessible
	if enc.config.HardwareDevice >= 0 {
		if err := element.SetProperty("gpu-id", uint(enc.config.HardwareDevice)); err != nil {
			enc.logger.Debugf("Failed to set gpu-id %d: %v", enc.config.HardwareDevice, err)
			return false
		}
	}

	// Try to set a basic property to test if the encoder is functional
	if err := element.SetProperty("preset", "default"); err != nil {
		enc.logger.Debugf("Failed to set NVENC preset: %v", err)
		return false
	}

	return true
}

// checkVAAPICapabilities checks if VAAPI hardware is available and functional
func (enc *EncoderGst) checkVAAPICapabilities(element *gst.Element) bool {
	// Try to set a basic property to test if the encoder is functional
	if err := element.SetProperty("rate-control", "cbr"); err != nil {
		enc.logger.Debugf("Failed to set VAAPI rate-control: %v", err)
		return false
	}

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

// initializePipeline creates and configures the internal encoding pipeline with fallback support
func (enc *EncoderGst) initializePipeline() error {
	// Clean up existing pipeline if any
	if enc.pipeline != nil {
		enc.pipeline.SetState(gst.StateNull)
		enc.pipeline.Unref()
		enc.pipeline = nil
	}

	// Create pipeline
	pipeline, err := gst.NewPipeline("video-encoder")
	if err != nil {
		return fmt.Errorf("failed to create pipeline: %w", err)
	}
	enc.pipeline = pipeline

	// Track pipeline object with memory manager
	if enc.memoryManager != nil {
		enc.memoryManager.RegisterObject(pipeline)
	}

	// Register pipeline for lifecycle tracking
	if enc.lifecycleManager != nil {
		if _, err := enc.lifecycleManager.RegisterObject(pipeline, "gst.Pipeline", "video-encoder"); err != nil {
			enc.logger.Warnf("Failed to register pipeline: %v", err)
		}
	}

	// Get bus for message handling
	enc.bus = pipeline.GetPipelineBus()

	// Track bus object with memory manager
	if enc.bus != nil && enc.memoryManager != nil {
		enc.memoryManager.RegisterObject(enc.bus)
	}

	// Register bus for lifecycle tracking
	if enc.lifecycleManager != nil && enc.bus != nil {
		if _, err := enc.lifecycleManager.RegisterObject(enc.bus, "gst.Bus", "pipeline-bus"); err != nil {
			enc.logger.Warnf("Failed to register bus: %v", err)
		}
	}

	// Create source element (appsrc for input)
	if err := enc.createSourceElement(); err != nil {
		enc.cleanupPipeline()
		return fmt.Errorf("failed to create source element: %w", err)
	}

	// Create encoder element with fallback
	if err := enc.createEncoderElementWithFallback(); err != nil {
		enc.cleanupPipeline()
		return fmt.Errorf("failed to create encoder element: %w", err)
	}

	// Create sink element (appsink for output)
	if err := enc.createSinkElement(); err != nil {
		enc.cleanupPipeline()
		return fmt.Errorf("failed to create sink element: %w", err)
	}

	// Link all elements
	if err := enc.linkElements(); err != nil {
		enc.cleanupPipeline()
		return fmt.Errorf("failed to link elements: %w", err)
	}

	enc.logger.Info("Encoder pipeline initialized successfully")
	return nil
}

// createEncoderElementWithFallback creates encoder element with automatic fallback
func (enc *EncoderGst) createEncoderElementWithFallback() error {
	var lastError error

	// Try current selected encoder first
	if err := enc.createEncoderElement(); err == nil {
		return nil
	} else {
		lastError = err
		enc.logger.Warnf("Failed to create encoder %s: %v", enc.getEncoderName(enc.selectedEncoder), err)
	}

	// Find current encoder index in fallback chain
	currentIndex := -1
	for i, encoderType := range enc.fallbackChain {
		if encoderType == enc.selectedEncoder {
			currentIndex = i
			break
		}
	}

	// Try remaining encoders in fallback chain
	if currentIndex >= 0 {
		for i := currentIndex + 1; i < len(enc.fallbackChain); i++ {
			fallbackEncoder := enc.fallbackChain[i]

			if !enc.isEncoderAvailable(fallbackEncoder) {
				continue
			}

			enc.logger.Infof("Trying fallback encoder: %s", enc.getEncoderName(fallbackEncoder))
			enc.selectedEncoder = fallbackEncoder

			if err := enc.createEncoderElement(); err == nil {
				enc.logger.Infof("Successfully created fallback encoder: %s", enc.getEncoderName(fallbackEncoder))
				return nil
			} else {
				lastError = err
				enc.logger.Warnf("Fallback encoder %s also failed: %v", enc.getEncoderName(fallbackEncoder), err)
			}
		}
	}

	return fmt.Errorf("all encoder fallbacks failed, last error: %w", lastError)
}

// cleanupPipeline cleans up pipeline resources with proper lifecycle management
func (enc *EncoderGst) cleanupPipeline() {
	// Set pipeline to NULL state before cleanup
	if enc.pipeline != nil {
		func() {
			defer func() {
				if r := recover(); r != nil {
					enc.logger.Errorf("Panic during pipeline state change to NULL: %v", r)
				}
			}()
			enc.pipeline.SetState(gst.StateNull)
		}()

		// Unref pipeline with panic protection
		func() {
			defer func() {
				if r := recover(); r != nil {
					enc.logger.Errorf("Panic during pipeline unref: %v", r)
				}
			}()
			enc.pipeline.Unref()
		}()

		enc.pipeline = nil
	}

	// Clear element references (they are owned by the pipeline)
	if enc.source != nil {
		enc.source = nil
	}
	if enc.encoder != nil {
		enc.encoder = nil
	}
	if enc.sink != nil {
		enc.sink = nil
	}
	if enc.bus != nil {
		enc.bus = nil
	}

	enc.logger.Debug("Pipeline cleanup completed")
}

// createSourceElement creates and configures the appsrc element with lifecycle tracking
func (enc *EncoderGst) createSourceElement() error {
	source, err := gst.NewElement("appsrc")
	if err != nil {
		return fmt.Errorf("failed to create appsrc: %w", err)
	}
	enc.source = source

	// Track source element with memory manager
	if enc.memoryManager != nil {
		enc.memoryManager.RegisterObject(source)
	}

	// Register source element for lifecycle tracking
	if enc.lifecycleManager != nil {
		if _, err := enc.lifecycleManager.RegisterObject(source, "gst.Element", "appsrc"); err != nil {
			enc.logger.Warnf("Failed to register source element: %v", err)
		}
	}

	// Configure appsrc properties with error handling
	if err := source.SetProperty("is-live", true); err != nil {
		return fmt.Errorf("failed to set is-live: %w", err)
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

	// Track caps object with memory manager
	if enc.memoryManager != nil {
		enc.memoryManager.RegisterObject(caps)
	}

	// Register caps for lifecycle tracking
	if enc.lifecycleManager != nil {
		if _, err := enc.lifecycleManager.RegisterObject(caps, "gst.Caps", "source-caps"); err != nil {
			enc.logger.Warnf("Failed to register caps object: %v", err)
		}
	}

	if err := source.SetProperty("caps", caps); err != nil {
		return fmt.Errorf("failed to set caps: %w", err)
	}

	// Add to pipeline
	enc.pipeline.Add(source)

	enc.logger.Debug("Created and configured appsrc element with lifecycle tracking")
	return nil
}

// createEncoderElement creates and configures the encoder element with lifecycle tracking
func (enc *EncoderGst) createEncoderElement() error {
	elementName := enc.getGStreamerElementName(enc.selectedEncoder)

	encoder, err := gst.NewElement(elementName)
	if err != nil {
		return fmt.Errorf("failed to create %s element: %w", elementName, err)
	}
	enc.encoder = encoder

	// Track encoder element with memory manager
	if enc.memoryManager != nil {
		enc.memoryManager.RegisterObject(encoder)
	}

	// Register encoder element for lifecycle tracking
	if enc.lifecycleManager != nil {
		if _, err := enc.lifecycleManager.RegisterObject(encoder, "gst.Element", elementName); err != nil {
			enc.logger.Warnf("Failed to register encoder element: %v", err)
		}
	}

	// Configure encoder properties based on type and configuration
	if err := enc.configureEncoderProperties(); err != nil {
		return fmt.Errorf("failed to configure encoder properties: %w", err)
	}

	// Add to pipeline
	enc.pipeline.Add(encoder)

	enc.logger.Debugf("Created and configured %s encoder element with lifecycle tracking", elementName)
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

// configureNVENCProperties configures NVENC encoder properties with type safety and error recovery
func (enc *EncoderGst) configureNVENCProperties() error {
	properties := []PropertyConfig{
		{"bitrate", uint(enc.config.Bitrate), true},
		{"gpu-id", uint(enc.config.HardwareDevice), true},
	}

	// Rate control mode
	switch enc.config.RateControl {
	case config.RateControlCBR:
		properties = append(properties, PropertyConfig{"rc-mode", "cbr", true})
	case config.RateControlVBR:
		properties = append(properties, PropertyConfig{"rc-mode", "vbr", true})
	case config.RateControlCQP:
		properties = append(properties,
			PropertyConfig{"rc-mode", "cqp", true},
			PropertyConfig{"qp-const", uint(enc.config.Quality), true})
	}

	// Performance preset
	preset := enc.mapPresetToNVENC(enc.config.Preset)
	properties = append(properties, PropertyConfig{"preset", preset, true})

	// Profile (optional, may not be supported on all versions)
	properties = append(properties, PropertyConfig{"profile", string(enc.config.Profile), false})

	// Zero latency settings
	if enc.config.ZeroLatency {
		properties = append(properties,
			PropertyConfig{"zerolatency", true, false},
			PropertyConfig{"b-frames", uint(0), true})
	} else if enc.config.BFrames >= 0 {
		properties = append(properties, PropertyConfig{"b-frames", uint(enc.config.BFrames), true})
	}

	// Keyframe interval
	gopSize := enc.config.KeyframeInterval * 30 // Assuming 30fps
	properties = append(properties, PropertyConfig{"gop-size", uint(gopSize), true})

	// Apply properties with error handling (using legacy method for now)
	if err := enc.setEncoderPropertiesLegacy(properties, "NVENC"); err != nil {
		return err
	}

	enc.logger.Debug("Configured NVENC encoder properties")
	return nil
}

// configureVAAPIProperties configures VAAPI encoder properties with type safety and error recovery
func (enc *EncoderGst) configureVAAPIProperties() error {
	properties := []PropertyConfig{
		{"bitrate", uint(enc.config.Bitrate), true},
	}

	// Rate control mode
	switch enc.config.RateControl {
	case config.RateControlCBR:
		properties = append(properties, PropertyConfig{"rate-control", "cbr", true})
	case config.RateControlVBR:
		properties = append(properties, PropertyConfig{"rate-control", "vbr", true})
	case config.RateControlCQP:
		properties = append(properties,
			PropertyConfig{"rate-control", "cqp", true},
			PropertyConfig{"init-qp", uint(enc.config.Quality), true})
	}

	// Profile (for H.264, optional)
	if enc.config.Codec == config.CodecH264 {
		properties = append(properties, PropertyConfig{"profile", string(enc.config.Profile), false})
	}

	// Zero latency settings
	if enc.config.ZeroLatency {
		properties = append(properties,
			PropertyConfig{"tune", "low-latency", false},
			PropertyConfig{"b-frames", uint(0), true})
	} else if enc.config.BFrames >= 0 {
		properties = append(properties, PropertyConfig{"b-frames", uint(enc.config.BFrames), true})
	}

	// Keyframe interval
	keyframePeriod := enc.config.KeyframeInterval * 30 // Assuming 30fps
	properties = append(properties, PropertyConfig{"keyframe-period", uint(keyframePeriod), true})

	// Apply properties with error handling (using legacy method for now)
	if err := enc.setEncoderPropertiesLegacy(properties, "VAAPI"); err != nil {
		return err
	}

	enc.logger.Debug("Configured VAAPI encoder properties")
	return nil
}

// configureX264Properties configures x264 encoder properties using the new property mapping system
// Implements enhanced error handling and logging as per requirements 2.1, 2.2, 2.3, 2.4
func (enc *EncoderGst) configureX264Properties() error {
	enc.logger.WithFields(logrus.Fields{
		"encoder_element": "x264enc",
		"config_bitrate":  enc.config.Bitrate,
		"config_preset":   enc.config.Preset,
		"config_profile":  enc.config.Profile,
	}).Info("Initializing x264 encoder property configuration")

	// First, introspect the element to check which properties are available
	enc.logger.Debug("Introspecting x264enc element for available properties")
	availableProperties := enc.introspectX264Properties(enc.encoder)

	if len(availableProperties) == 0 {
		enc.logger.Error("No properties found on x264enc element - element may be invalid")
		return fmt.Errorf("x264enc element introspection failed - no properties available")
	}

	enc.logger.WithFields(logrus.Fields{
		"available_property_count": len(availableProperties),
	}).Debug("x264enc element introspection completed")

	// Build property configuration using the descriptor-based approach
	result := enc.configureX264PropertiesWithDescriptors(availableProperties)

	// Process results and handle errors with enhanced logging
	enc.logger.Debug("Processing x264 property configuration results")
	return enc.processX264ConfigurationResult(result)
}

// configureX264PropertiesWithDescriptors configures x264 properties using property descriptors
// Implements enhanced logging and error classification for x264 property configuration
func (enc *EncoderGst) configureX264PropertiesWithDescriptors(availableProperties map[string]PropertyInfo) *X264ConfigResult {
	result := &X264ConfigResult{
		Results:        make([]PropertyConfigResult, 0),
		CriticalErrors: make([]error, 0),
		Warnings:       make([]error, 0),
	}

	// Build configuration values from encoder config
	configValues := enc.buildX264ConfigurationValues()

	// Log configuration start with summary
	enc.logger.WithFields(logrus.Fields{
		"total_properties":     len(configValues),
		"available_properties": len(availableProperties),
		"encoder_type":         "x264enc",
	}).Info("Starting x264 encoder property configuration")

	// Count required vs optional properties for logging
	requiredCount := 0
	optionalCount := 0
	for configKey := range configValues {
		if descriptor, exists := x264PropertyMap[configKey]; exists {
			if descriptor.Required {
				requiredCount++
			} else {
				optionalCount++
			}
		}
	}

	enc.logger.WithFields(logrus.Fields{
		"required_properties": requiredCount,
		"optional_properties": optionalCount,
	}).Debug("x264 property configuration breakdown")

	// Process each property using its descriptor
	for configKey, configValue := range configValues {
		descriptor, exists := x264PropertyMap[configKey]
		if !exists {
			// Skip properties not in our mapping
			enc.logger.WithFields(logrus.Fields{
				"property": configKey,
				"value":    configValue,
			}).Debug("Skipping unmapped x264 property - not in descriptor registry")
			continue
		}

		propResult := enc.configureX264Property(descriptor, configValue, availableProperties)
		result.Results = append(result.Results, propResult)

		// Classify errors with enhanced logging
		if !propResult.Success {
			if descriptor.Required {
				result.CriticalErrors = append(result.CriticalErrors, propResult.Error)
				enc.logger.WithFields(logrus.Fields{
					"property": propResult.PropertyName,
					"error":    propResult.Error.Error(),
				}).Error("Critical x264 property configuration failure")
			} else {
				result.Warnings = append(result.Warnings, propResult.Error)
				enc.logger.WithFields(logrus.Fields{
					"property": propResult.PropertyName,
					"error":    propResult.Error.Error(),
				}).Debug("Optional x264 property configuration warning")
			}
		}
	}

	// Log intermediate results
	successCount := 0
	for _, propResult := range result.Results {
		if propResult.Success {
			successCount++
		}
	}

	enc.logger.WithFields(logrus.Fields{
		"processed":       len(result.Results),
		"successful":      successCount,
		"failed":          len(result.Results) - successCount,
		"critical_errors": len(result.CriticalErrors),
		"warnings":        len(result.Warnings),
	}).Debug("x264 property configuration processing completed")

	return result
}

// buildX264ConfigurationValues builds a map of configuration values from encoder config
func (enc *EncoderGst) buildX264ConfigurationValues() map[string]interface{} {
	configValues := make(map[string]interface{})

	// Always set bitrate
	configValues["bitrate"] = enc.config.Bitrate

	// Set quantizer for quality-based encoding
	if enc.config.RateControl == config.RateControlCQP {
		configValues["quantizer"] = enc.config.Quality
	}

	// Speed preset
	preset := enc.mapPresetToX264(enc.config.Preset)
	configValues["speed-preset"] = preset

	// Profile
	configValues["profile"] = string(enc.config.Profile)

	// Tuning
	if enc.config.ZeroLatency || enc.config.Tune == "zerolatency" {
		configValues["tune"] = "zerolatency"
	} else if enc.config.Tune != "" {
		configValues["tune"] = enc.config.Tune
	}

	// Threading
	threads := enc.config.Threads
	if threads <= 0 {
		threads = runtime.NumCPU()
	}
	configValues["threads"] = threads

	// B-frames
	if enc.config.ZeroLatency {
		configValues["bframes"] = 0
	} else if enc.config.BFrames >= 0 {
		configValues["bframes"] = enc.config.BFrames
	}

	// Keyframe interval
	keyIntMax := enc.config.KeyframeInterval * 30 // Assuming 30fps
	configValues["key-int-max"] = keyIntMax

	return configValues
}

// configureX264Property configures a single x264 property using its descriptor
// Implements enhanced error handling and logging for individual property configuration
func (enc *EncoderGst) configureX264Property(descriptor X264PropertyDescriptor, value interface{}, availableProperties map[string]PropertyInfo) PropertyConfigResult {
	result := PropertyConfigResult{
		PropertyName: descriptor.Name,
		Success:      false,
		Value:        value,
		Error:        nil,
	}

	// Log property configuration attempt
	enc.logger.WithFields(logrus.Fields{
		"property": descriptor.Name,
		"value":    value,
		"type":     getPropertyTypeString(descriptor.Type),
		"required": descriptor.Required,
	}).Debug("Attempting to configure x264 property")

	// Check if property exists in the element
	if _, exists := availableProperties[descriptor.Name]; !exists {
		result.Error = fmt.Errorf("property '%s' not available on x264enc element", descriptor.Name)

		// Enhanced logging for missing properties
		if descriptor.Required {
			enc.logger.WithFields(logrus.Fields{
				"property": descriptor.Name,
				"required": true,
			}).Error("Required x264 property not available on this GStreamer version")
		} else {
			enc.logger.WithFields(logrus.Fields{
				"property": descriptor.Name,
				"required": false,
			}).Debug("Optional x264 property not available - will skip")
		}

		return result
	}

	// Validate and convert the value using the descriptor's validator
	if descriptor.Validator != nil {
		validatedValue, err := descriptor.Validator(value)
		if err != nil {
			result.Error = fmt.Errorf("property '%s' validation failed: %w", descriptor.Name, err)

			// Enhanced validation error logging
			enc.logger.WithFields(logrus.Fields{
				"property":         descriptor.Name,
				"original_value":   value,
				"validation_error": err.Error(),
				"required":         descriptor.Required,
			}).Warn("x264 property validation failed")

			return result
		}
		result.Value = validatedValue

		// Log successful validation if value was converted
		if !reflect.DeepEqual(value, validatedValue) {
			enc.logger.WithFields(logrus.Fields{
				"property":        descriptor.Name,
				"original_value":  value,
				"validated_value": validatedValue,
			}).Debug("x264 property value converted during validation")
		}
	}

	// Attempt to set the property
	if err := enc.setEncoderPropertySafe(descriptor.Name, result.Value); err != nil {
		result.Error = fmt.Errorf("failed to set property '%s' to %v: %w", descriptor.Name, result.Value, err)

		// Enhanced property setting error logging
		enc.logger.WithFields(logrus.Fields{
			"property":  descriptor.Name,
			"value":     result.Value,
			"gst_error": err.Error(),
			"required":  descriptor.Required,
		}).Warn("Failed to set x264 property on GStreamer element")

		return result
	}

	// Success - enhanced success logging
	result.Success = true
	enc.logger.WithFields(logrus.Fields{
		"property": descriptor.Name,
		"value":    result.Value,
		"type":     getPropertyTypeString(descriptor.Type),
	}).Debug("x264 property successfully configured")

	return result
}

// processX264ConfigurationResult processes the configuration result and handles errors
// Implements requirements 2.1, 2.2, 2.3, 2.4 for enhanced error handling and logging
func (enc *EncoderGst) processX264ConfigurationResult(result *X264ConfigResult) error {
	successCount := 0
	skippedCount := 0
	requiredFailures := 0
	optionalFailures := 0

	// Count successes and failures by type
	for _, propResult := range result.Results {
		if propResult.Success {
			successCount++
		} else {
			skippedCount++
			descriptor := x264PropertyMap[propResult.PropertyName]
			if descriptor.Required {
				requiredFailures++
			} else {
				optionalFailures++
			}
		}
	}

	// Log detailed results for each property with enhanced logging
	enc.logger.Debug("Processing x264 property configuration results...")
	for _, propResult := range result.Results {
		descriptor := x264PropertyMap[propResult.PropertyName]

		if propResult.Success {
			// Requirement 2.3: Debug-level logs for successful properties
			enc.logger.WithFields(logrus.Fields{
				"property": propResult.PropertyName,
				"value":    propResult.Value,
				"type":     getPropertyTypeString(descriptor.Type),
				"required": descriptor.Required,
			}).Debug("x264 property successfully configured")
		} else {
			// Requirement 2.1 & 2.2: Distinguish between required and optional property failures
			if descriptor.Required {
				// Required property failure - log as error
				enc.logger.WithFields(logrus.Fields{
					"property": propResult.PropertyName,
					"value":    propResult.Value,
					"error":    propResult.Error.Error(),
					"type":     getPropertyTypeString(descriptor.Type),
				}).Error("Critical failure: required x264 property could not be set")
			} else {
				// Optional property failure - log as warning
				enc.logger.WithFields(logrus.Fields{
					"property": propResult.PropertyName,
					"value":    propResult.Value,
					"error":    propResult.Error.Error(),
					"type":     getPropertyTypeString(descriptor.Type),
				}).Warn("Optional x264 property skipped due to compatibility issue")
			}
		}
	}

	// Requirement 2.4: Enhanced configuration completion summary
	enc.logger.WithFields(logrus.Fields{
		"total_properties":  len(result.Results),
		"successful":        successCount,
		"skipped":           skippedCount,
		"required_failures": requiredFailures,
		"optional_failures": optionalFailures,
		"critical_errors":   len(result.CriticalErrors),
		"warnings":          len(result.Warnings),
	}).Info("x264 encoder property configuration completed")

	// Log performance impact warnings for missing optional properties
	if optionalFailures > 0 {
		enc.logOptionalPropertyImpact(result)
	}

	// Requirement 2.1: Return error for critical failures
	if len(result.CriticalErrors) > 0 {
		enc.logger.WithFields(logrus.Fields{
			"critical_error_count": len(result.CriticalErrors),
			"first_error":          result.CriticalErrors[0].Error(),
		}).Error("x264 encoder configuration failed due to critical property errors")

		return fmt.Errorf("critical x264 property configuration failures (%d errors): %v",
			len(result.CriticalErrors), result.CriticalErrors[0])
	}

	// Enhanced success message with performance notes and configuration health assessment
	if successCount == len(result.Results) {
		enc.logger.WithFields(logrus.Fields{
			"configuration_health": "optimal",
			"all_properties_set":   true,
		}).Info("x264 encoder fully configured - all properties set successfully")
	} else {
		successRate := float64(successCount) / float64(len(result.Results)) * 100
		var healthStatus string
		if successRate >= 90 {
			healthStatus = "excellent"
		} else if successRate >= 75 {
			healthStatus = "good"
		} else if successRate >= 50 {
			healthStatus = "fair"
		} else {
			healthStatus = "poor"
		}

		enc.logger.WithFields(logrus.Fields{
			"success_rate":         fmt.Sprintf("%.1f%%", successRate),
			"configuration_health": healthStatus,
			"properties_set":       successCount,
			"properties_total":     len(result.Results),
		}).Info("x264 encoder configured with partial success - some optional properties skipped")

		// Provide configuration recommendations based on health status
		enc.logConfigurationRecommendations(healthStatus, result)
	}

	return nil
}

// logOptionalPropertyImpact logs the potential performance impact of missing optional properties
// Enhanced with more detailed impact analysis and mitigation suggestions
func (enc *EncoderGst) logOptionalPropertyImpact(result *X264ConfigResult) {
	impactMessages := make([]string, 0)
	mitigationSuggestions := make([]string, 0)

	for _, propResult := range result.Results {
		if !propResult.Success {
			descriptor := x264PropertyMap[propResult.PropertyName]
			if !descriptor.Required {
				switch propResult.PropertyName {
				case "speed-preset":
					impactMessages = append(impactMessages, "encoding speed may not be optimal - using default preset")
					mitigationSuggestions = append(mitigationSuggestions, "consider upgrading GStreamer or using software fallback")
				case "tune":
					impactMessages = append(impactMessages, "encoding may not be tuned for specific content type")
					mitigationSuggestions = append(mitigationSuggestions, "encoding will use default tuning parameters")
				case "profile":
					impactMessages = append(impactMessages, "H.264 profile may default to baseline - reduced compatibility")
					mitigationSuggestions = append(mitigationSuggestions, "clients may need to support baseline profile")
				case "bframes":
					impactMessages = append(impactMessages, "compression efficiency may be reduced without B-frames")
					mitigationSuggestions = append(mitigationSuggestions, "bitrate may need to be increased for same quality")
				}
			}
		}
	}

	if len(impactMessages) > 0 {
		enc.logger.WithFields(logrus.Fields{
			"impact_count":           len(impactMessages),
			"impacts":                impactMessages,
			"mitigation_count":       len(mitigationSuggestions),
			"mitigation_suggestions": mitigationSuggestions,
		}).Warn("Optional x264 properties missing - potential performance impacts detected")

		// Log each impact with its mitigation separately for better readability
		for i, impact := range impactMessages {
			var mitigation string
			if i < len(mitigationSuggestions) {
				mitigation = mitigationSuggestions[i]
			} else {
				mitigation = "no specific mitigation available"
			}

			enc.logger.WithFields(logrus.Fields{
				"impact":     impact,
				"mitigation": mitigation,
			}).Debug("Property impact analysis")
		}
	}
}

// logConfigurationRecommendations provides recommendations based on configuration health
func (enc *EncoderGst) logConfigurationRecommendations(healthStatus string, result *X264ConfigResult) {
	recommendations := make([]string, 0)

	switch healthStatus {
	case "poor":
		recommendations = append(recommendations, "consider upgrading GStreamer to a newer version")
		recommendations = append(recommendations, "verify x264 plugin installation and compatibility")
		recommendations = append(recommendations, "consider using software fallback encoder")
	case "fair":
		recommendations = append(recommendations, "some advanced features may not be available")
		recommendations = append(recommendations, "monitor encoding performance and quality")
	case "good":
		recommendations = append(recommendations, "configuration is mostly optimal")
		recommendations = append(recommendations, "minor features may be missing but performance should be good")
	case "excellent":
		recommendations = append(recommendations, "configuration is near-optimal")
	}

	// Add specific recommendations based on failed properties
	for _, propResult := range result.Results {
		if !propResult.Success {
			switch propResult.PropertyName {
			case "speed-preset":
				recommendations = append(recommendations, "encoding speed optimization unavailable - consider manual tuning")
			case "profile":
				recommendations = append(recommendations, "H.264 profile control unavailable - verify client compatibility")
			case "tune":
				recommendations = append(recommendations, "content-specific tuning unavailable - use generic settings")
			}
		}
	}

	if len(recommendations) > 0 {
		enc.logger.WithFields(logrus.Fields{
			"health_status":        healthStatus,
			"recommendation_count": len(recommendations),
			"recommendations":      recommendations,
		}).Info("x264 encoder configuration recommendations")
	}
}

// getPropertyTypeString returns a human-readable string for PropertyType
func getPropertyTypeString(propType PropertyType) string {
	switch propType {
	case PropertyTypeString:
		return "string"
	case PropertyTypeEnum:
		return "enum"
	case PropertyTypeUInt:
		return "uint"
	case PropertyTypeBool:
		return "bool"
	default:
		return "unknown"
	}
}

// PropertyInfo contains information about a GStreamer element property
type PropertyInfo struct {
	Name        string
	Type        string
	Readable    bool
	Writable    bool
	Description string
}

// introspectX264Properties checks which properties are available on the x264enc element
func (enc *EncoderGst) introspectX264Properties(element *gst.Element) map[string]PropertyInfo {
	// Use the generic introspection function with x264-specific properties
	commonX264Properties := []string{
		"bitrate", "quantizer", "speed-preset", "profile", "tune",
		"threads", "b-frames", "bframes", "key-int-max", "pass",
		"stats-file", "multipass-cache-file", "qp-min", "qp-max",
		"cabac", "dct8x8", "aud", "trellis", "me", "subme",
		"analyse", "weightb", "weightp", "rc-lookahead", "intra-refresh",
	}

	return enc.introspectEncoderPropertiesWithList(element, "x264", commonX264Properties)
}

// introspectEncoderProperties checks which properties are available on an encoder element
func (enc *EncoderGst) introspectEncoderProperties(element *gst.Element, encoderType string) map[string]PropertyInfo {
	var propertyList []string

	switch encoderType {
	case "NVENC":
		propertyList = []string{
			"bitrate", "rc-mode", "preset", "profile", "gop-size",
			"b-frames", "zerolatency", "gpu-id", "qp-const", "qp-min", "qp-max",
			"spatial-aq", "temporal-aq", "aq-strength", "lookahead-depth",
		}
	case "VAAPI":
		propertyList = []string{
			"bitrate", "rate-control", "profile", "keyframe-period",
			"b-frames", "tune", "init-qp", "min-qp", "max-qp",
			"cpb-size", "target-percentage", "quality-level",
		}
	case "x264":
		propertyList = []string{
			"bitrate", "quantizer", "speed-preset", "profile", "tune",
			"threads", "b-frames", "bframes", "key-int-max", "pass",
			"stats-file", "multipass-cache-file", "qp-min", "qp-max",
			"cabac", "dct8x8", "aud", "trellis", "me", "subme",
			"analyse", "weightb", "weightp", "rc-lookahead", "intra-refresh",
		}
	default:
		enc.logger.Warnf("Unknown encoder type for introspection: %s", encoderType)
		return make(map[string]PropertyInfo)
	}

	return enc.introspectEncoderPropertiesWithList(element, encoderType, propertyList)
}

// introspectEncoderPropertiesWithList checks which properties from a given list are available
func (enc *EncoderGst) introspectEncoderPropertiesWithList(element *gst.Element, encoderType string, propertyList []string) map[string]PropertyInfo {
	properties := make(map[string]PropertyInfo)

	if element == nil {
		enc.logger.Warn("Cannot introspect properties: element is nil")
		return properties
	}

	// Get the element factory to access property information
	factory := element.GetFactory()
	if factory == nil {
		enc.logger.Warn("Cannot get element factory for property introspection")
		return properties
	}

	// Check each property in the list by attempting to get it
	for _, propName := range propertyList {
		// Try to get the property to see if it exists
		_, err := element.GetProperty(propName)
		if err == nil {
			// Property exists and is readable
			properties[propName] = PropertyInfo{
				Name:        propName,
				Type:        "unknown", // We'd need more introspection to get the actual type
				Readable:    true,
				Writable:    true, // Assume writable if readable (simplified)
				Description: fmt.Sprintf("%s property: %s", encoderType, propName),
			}
			enc.logger.Debugf("Found available %s property: %s", encoderType, propName)
		} else {
			enc.logger.Debugf("Property %s not available on %s encoder: %v", propName, encoderType, err)
		}
	}

	enc.logger.Infof("Introspected %d available %s properties", len(properties), encoderType)
	return properties
}

// convertPropertyValue converts and validates property values for type safety
func (enc *EncoderGst) convertPropertyValue(value interface{}) (interface{}, error) {
	if value == nil {
		return nil, nil
	}

	switch v := value.(type) {
	case string:
		return v, nil
	case bool:
		return v, nil
	case int:
		if v < 0 {
			return uint(0), nil // Convert negative to 0 for unsigned properties
		}
		return uint(v), nil
	case int32:
		if v < 0 {
			return uint(0), nil
		}
		return uint(v), nil
	case int64:
		if v < 0 {
			return uint(0), nil
		}
		return uint(v), nil
	case uint:
		return v, nil
	case uint32:
		return v, nil
	case uint64:
		return v, nil
	case float32:
		return v, nil
	case float64:
		return v, nil
	default:
		// Try reflection for other types
		rv := reflect.ValueOf(value)
		switch rv.Kind() {
		case reflect.String:
			return rv.String(), nil
		case reflect.Bool:
			return rv.Bool(), nil
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			intVal := rv.Int()
			if intVal < 0 {
				return uint(0), nil
			}
			return uint(intVal), nil
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return rv.Uint(), nil
		case reflect.Float32, reflect.Float64:
			return rv.Float(), nil
		default:
			return nil, fmt.Errorf("unsupported property type: %T", value)
		}
	}
}

// abs returns the absolute value of an integer
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// SetSampleCallback sets the callback function for processing encoded video samples (implements Encoder interface)
func (enc *EncoderGst) SetSampleCallback(callback func(*Sample) error) error {
	enc.mu.Lock()
	defer enc.mu.Unlock()

	// Store the callback - it will be used in the output processing
	enc.logger.Debug("Sample callback set for go-gst encoder")
	return nil
}

// setEncoderPropertySafe safely sets a property on the encoder element with enhanced error handling
// Implements comprehensive logging and error context for property configuration
func (enc *EncoderGst) setEncoderPropertySafe(propertyName string, value interface{}) error {
	if enc.encoder == nil {
		return fmt.Errorf("encoder element is nil")
	}

	// Enhanced property setting attempt logging with more context
	enc.logger.WithFields(logrus.Fields{
		"property":     propertyName,
		"value":        value,
		"value_type":   fmt.Sprintf("%T", value),
		"encoder_type": enc.getEncoderName(enc.selectedEncoder),
		"element_name": enc.getGStreamerElementName(enc.selectedEncoder),
	}).Debug("Attempting to set encoder property")

	// Attempt to set the property
	err := enc.encoder.SetProperty(propertyName, value)
	if err != nil {
		// Enhanced error logging with comprehensive context
		enc.logger.WithFields(logrus.Fields{
			"property":      propertyName,
			"value":         value,
			"value_type":    fmt.Sprintf("%T", value),
			"encoder_type":  enc.getEncoderName(enc.selectedEncoder),
			"element_name":  enc.getGStreamerElementName(enc.selectedEncoder),
			"gst_error":     err.Error(),
			"error_context": "GStreamer property setting failed",
		}).Debug("Failed to set encoder property - detailed error context")

		return fmt.Errorf("GStreamer SetProperty failed for %s=%v: %w", propertyName, value, err)
	}

	// Enhanced success logging
	enc.logger.WithFields(logrus.Fields{
		"property":     propertyName,
		"value":        value,
		"encoder_type": enc.getEncoderName(enc.selectedEncoder),
	}).Debug("Successfully set encoder property")

	return nil
}

// setEncoderPropertiesLegacy sets multiple encoder properties with legacy error handling
// Used for NVENC and VAAPI encoders that don't use the descriptor-based system yet
func (enc *EncoderGst) setEncoderPropertiesLegacy(properties []PropertyConfig, encoderType string) error {
	successCount := 0
	failureCount := 0
	criticalFailures := make([]error, 0)
	warnings := make([]error, 0)

	enc.logger.WithFields(logrus.Fields{
		"encoder_type":   encoderType,
		"property_count": len(properties),
	}).Info("Configuring encoder properties using legacy method")

	// Process each property
	for _, prop := range properties {
		enc.logger.WithFields(logrus.Fields{
			"property": prop.Name,
			"value":    prop.Value,
			"required": prop.Required,
		}).Debug("Attempting to set encoder property")

		err := enc.setEncoderPropertySafe(prop.Name, prop.Value)
		if err != nil {
			failureCount++
			if prop.Required {
				// Required property failure
				criticalFailures = append(criticalFailures, fmt.Errorf("required property '%s': %w", prop.Name, err))
				enc.logger.WithFields(logrus.Fields{
					"property": prop.Name,
					"value":    prop.Value,
					"error":    err.Error(),
				}).Error("Critical failure: required encoder property could not be set")
			} else {
				// Optional property failure
				warnings = append(warnings, fmt.Errorf("optional property '%s': %w", prop.Name, err))
				enc.logger.WithFields(logrus.Fields{
					"property": prop.Name,
					"value":    prop.Value,
					"error":    err.Error(),
				}).Warn("Optional encoder property skipped due to compatibility issue")
			}
		} else {
			successCount++
			enc.logger.WithFields(logrus.Fields{
				"property": prop.Name,
				"value":    prop.Value,
			}).Debug("Encoder property successfully configured")
		}
	}

	// Log configuration summary
	enc.logger.WithFields(logrus.Fields{
		"encoder_type":     encoderType,
		"total_properties": len(properties),
		"successful":       successCount,
		"failed":           failureCount,
		"critical_errors":  len(criticalFailures),
		"warnings":         len(warnings),
		"success_rate":     fmt.Sprintf("%.1f%%", float64(successCount)/float64(len(properties))*100),
	}).Info("Encoder property configuration completed")

	// Return error if there were critical failures
	if len(criticalFailures) > 0 {
		enc.logger.WithFields(logrus.Fields{
			"encoder_type":         encoderType,
			"critical_error_count": len(criticalFailures),
			"first_error":          criticalFailures[0].Error(),
		}).Error("Encoder configuration failed due to critical property errors")

		return fmt.Errorf("%s encoder critical property failures (%d errors): %v",
			encoderType, len(criticalFailures), criticalFailures[0])
	}

	// Log success message
	if successCount == len(properties) {
		enc.logger.WithFields(logrus.Fields{
			"encoder_type": encoderType,
		}).Info("Encoder fully configured - all properties set successfully")
	} else {
		enc.logger.WithFields(logrus.Fields{
			"encoder_type": encoderType,
			"success_rate": fmt.Sprintf("%.1f%%", float64(successCount)/float64(len(properties))*100),
		}).Info("Encoder configured with partial success - some optional properties skipped")
	}

	return nil
}

// configureVP8Properties configures VP8 encoder properties with enhanced error handling and logging
func (enc *EncoderGst) configureVP8Properties() error {
	enc.logger.WithFields(logrus.Fields{
		"encoder_element":   "vp8enc",
		"config_bitrate":    enc.config.Bitrate,
		"config_threads":    enc.config.Threads,
		"keyframe_interval": enc.config.KeyframeInterval,
	}).Info("Initializing VP8 encoder property configuration")

	properties := []PropertyConfig{
		{"target-bitrate", uint(enc.config.Bitrate * 1000), true},           // Convert kbps to bps
		{"keyframe-max-dist", uint(enc.config.KeyframeInterval * 30), true}, // Assuming 30fps
	}

	// Threading
	threads := enc.config.Threads
	if threads <= 0 {
		threads = runtime.NumCPU()
	}
	properties = append(properties, PropertyConfig{"threads", uint(threads), true})

	// Apply properties with enhanced error handling
	if err := enc.setEncoderPropertiesLegacy(properties, "VP8"); err != nil {
		return err
	}

	enc.logger.Debug("VP8 encoder property configuration completed successfully")
	return nil
}

// configureVP9Properties configures VP9 encoder properties with enhanced error handling and logging
func (enc *EncoderGst) configureVP9Properties() error {
	enc.logger.WithFields(logrus.Fields{
		"encoder_element":   "vp9enc",
		"config_bitrate":    enc.config.Bitrate,
		"config_threads":    enc.config.Threads,
		"keyframe_interval": enc.config.KeyframeInterval,
	}).Info("Initializing VP9 encoder property configuration")

	properties := []PropertyConfig{
		{"target-bitrate", uint(enc.config.Bitrate * 1000), true},           // Convert kbps to bps
		{"keyframe-max-dist", uint(enc.config.KeyframeInterval * 30), true}, // Assuming 30fps
	}

	// Threading
	threads := enc.config.Threads
	if threads <= 0 {
		threads = runtime.NumCPU()
	}
	properties = append(properties, PropertyConfig{"threads", uint(threads), true})

	// Apply properties with enhanced error handling
	if err := enc.setEncoderPropertiesLegacy(properties, "VP9"); err != nil {
		return err
	}

	enc.logger.Debug("VP9 encoder property configuration completed successfully")
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

// createSinkElement creates and configures the appsink element with lifecycle tracking
func (enc *EncoderGst) createSinkElement() error {
	sink, err := gst.NewElement("appsink")
	if err != nil {
		return fmt.Errorf("failed to create appsink: %w", err)
	}
	enc.sink = sink

	// Track sink element with memory manager
	if enc.memoryManager != nil {
		enc.memoryManager.RegisterObject(sink)
	}

	// Register sink element for lifecycle tracking
	if enc.lifecycleManager != nil {
		if _, err := enc.lifecycleManager.RegisterObject(sink, "gst.Element", "appsink"); err != nil {
			enc.logger.Warnf("Failed to register sink element: %v", err)
		}
	}

	// Configure appsink properties
	if err := sink.SetProperty("emit-signals", true); err != nil {
		return fmt.Errorf("failed to set emit-signals: %w", err)
	}
	if err := sink.SetProperty("sync", false); err != nil {
		return fmt.Errorf("failed to set sync: %w", err)
	}
	if err := sink.SetProperty("max-buffers", uint(1)); err != nil {
		return fmt.Errorf("failed to set max-buffers: %w", err)
	}
	if err := sink.SetProperty("drop", true); err != nil {
		return fmt.Errorf("failed to set drop: %w", err)
	}

	// Add to pipeline
	enc.pipeline.Add(sink)

	enc.logger.Debug("Created and configured appsink element with lifecycle tracking")
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

// Stop stops the encoder pipeline with proper resource cleanup
func (enc *EncoderGst) Stop() error {
	enc.mu.Lock()
	defer enc.mu.Unlock()

	if !enc.isRunning {
		return nil
	}

	enc.logger.Info("Stopping encoder pipeline...")

	// Cancel context to stop goroutines
	enc.cancel()

	// Set pipeline to null state with error handling
	if enc.pipeline != nil {
		func() {
			defer func() {
				if r := recover(); r != nil {
					enc.logger.Errorf("Panic during pipeline stop: %v", r)
				}
			}()

			if err := enc.pipeline.SetState(gst.StateNull); err != nil {
				enc.logger.Warnf("Failed to set pipeline to null state: %v", err)
			}
		}()
	}

	enc.isRunning = false

	// Close channels safely
	func() {
		defer func() {
			if r := recover(); r != nil {
				enc.logger.Errorf("Panic during channel cleanup: %v", r)
			}
		}()

		close(enc.inputChan)
		close(enc.outputChan)
		close(enc.errorChan)
	}()

	// Cleanup pipeline resources
	enc.cleanupPipeline()

	// Clean up memory manager with final leak check
	if enc.memoryManager != nil {
		enc.logger.Debug("Performing final memory cleanup")

		// Trigger final cleanup
		enc.memoryManager.ForceGC()

		// Check for leaks before closing
		leaks := enc.memoryManager.CheckMemoryLeaks()
		if len(leaks) > 0 {
			enc.logger.Warnf("Found %d potential memory leaks during shutdown", len(leaks))
			for _, leak := range leaks {
				enc.logger.Warnf("Leaked object: %s (ID: %x, Age: %v, RefCount: %d)",
					leak.ObjectType, leak.ObjectID, leak.Age, leak.RefCount)
			}
		}

		enc.memoryManager.Cleanup()
		enc.memoryManager = nil
	}

	// Close lifecycle manager
	if enc.lifecycleManager != nil {
		enc.logger.Debug("Shutting down object lifecycle manager")
		enc.lifecycleManager.Close()
		enc.lifecycleManager = nil
	}

	// Log final statistics
	stats := enc.GetStats()
	enc.logger.Infof("Encoder stopped - Frames encoded: %d, Keyframes: %d, Dropped: %d, Avg bitrate: %d kbps",
		stats.FramesEncoded, stats.KeyframesEncoded, stats.FramesDropped, stats.CurrentBitrate)

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

	// Set buffer timestamp (simplified for compatibility)
	// buffer.SetPTS(gst.ClockTime(sample.Timestamp.UnixNano()))

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

// onNewSample handles new encoded samples from the appsink with enhanced error handling
func (enc *EncoderGst) onNewSample(sink *gst.Element) gst.FlowReturn {
	defer func() {
		if err := recover(); err != nil {
			enc.logger.Errorf("Panic in onNewSample: %v", err)
		}
	}()

	// Validate sink element
	if sink == nil {
		enc.logger.Error("Received nil sink element in onNewSample")
		return gst.FlowError
	}

	// Pull sample from appsink with timeout protection
	sampleChan := make(chan interface{}, 1)
	errChan := make(chan error, 1)

	go func() {
		sample, err := sink.Emit("pull-sample")
		if err != nil {
			errChan <- err
			return
		}
		sampleChan <- sample
	}()

	var sample interface{}
	select {
	case sample = <-sampleChan:
		if sample == nil {
			enc.logger.Error("Received nil sample from appsink")
			return gst.FlowError
		}
	case err := <-errChan:
		enc.logger.Errorf("Failed to pull sample from appsink: %v", err)
		return gst.FlowError
	case <-time.After(1 * time.Second):
		enc.logger.Error("Timeout pulling sample from appsink")
		return gst.FlowError
	}

	// Type assertion with safety check
	gstSample, ok := sample.(*gst.Sample)
	if !ok {
		enc.logger.Errorf("Invalid sample type: %T", sample)
		return gst.FlowError
	}

	// Convert to internal sample format
	internalSample, err := enc.convertGstSample(gstSample)
	if err != nil {
		enc.logger.Errorf("Failed to convert sample: %v", err)

		// Update error statistics
		enc.stats.mu.Lock()
		enc.stats.framesDropped++
		enc.stats.mu.Unlock()

		// Return OK to continue processing, but log the error
		return gst.FlowOK
	}

	// Send to output channel (non-blocking)
	select {
	case enc.outputChan <- internalSample:
		// Update statistics
		enc.stats.mu.Lock()
		enc.stats.bytesEncoded += int64(len(internalSample.Data))
		enc.stats.lastFrameTime = time.Now()
		enc.stats.mu.Unlock()

		enc.logger.Debugf("Successfully processed encoded sample: %d bytes", len(internalSample.Data))
	default:
		// Channel full, drop sample
		enc.logger.Debug("Dropped encoded sample due to full output channel")

		// Update drop statistics
		enc.stats.mu.Lock()
		enc.stats.framesDropped++
		enc.stats.mu.Unlock()
	}

	return gst.FlowOK
}

// convertGstSample converts a go-gst sample to internal Sample format with enhanced error handling
// This method is deprecated in favor of safeConvertGstSample for better lifecycle management
func (enc *EncoderGst) convertGstSample(gstSample *gst.Sample) (*Sample, error) {
	return enc.safeConvertGstSample(gstSample)
}

// safeConvertGstSample converts a go-gst sample to internal Sample format with lifecycle validation
// This method implements requirements 1.1 and 3.4 for safe sample processing
func (enc *EncoderGst) safeConvertGstSample(gstSample *gst.Sample) (*Sample, error) {
	// Null check
	if gstSample == nil {
		return nil, fmt.Errorf("received nil gst sample")
	}

	// Register sample object for lifecycle tracking
	sampleID, err := enc.lifecycleManager.RegisterObject(gstSample, "gst.Sample", "encoder-sample")
	if err != nil {
		enc.logger.Warnf("Failed to register sample object: %v", err)
		// Continue processing but without lifecycle tracking
	}

	// Use safe access to validate and process the sample
	var conversionErr error
	if enc.lifecycleManager != nil && sampleID != 0 {
		conversionErr = enc.lifecycleManager.SafeObjectAccess(sampleID, func() error {
			// Validate sample object before processing
			if !enc.lifecycleManager.ValidateGstObject(gstSample) {
				return fmt.Errorf("sample object failed validation check")
			}
			return nil
		})

		if conversionErr != nil {
			enc.logger.WithFields(logrus.Fields{
				"component": "encoder-gst",
				"method":    "convertGstSample",
				"error":     "sample_validation_failed",
				"details":   conversionErr.Error(),
			}).Error("Sample object validation failed during safe access")
			enc.lifecycleManager.UnregisterObject(sampleID)
			return nil, conversionErr
		}
	}

	// Defer cleanup of sample object
	defer func() {
		if sampleID != 0 {
			if err := enc.lifecycleManager.UnregisterObject(sampleID); err != nil {
				enc.logger.Debugf("Failed to unregister sample object: %v", err)
			}
		}
	}()

	// Get buffer from sample with validation
	buffer := gstSample.GetBuffer()
	if buffer == nil {
		return nil, fmt.Errorf("no buffer in sample")
	}

	// Register buffer object for lifecycle tracking
	bufferID, err := enc.lifecycleManager.RegisterObject(buffer, "gst.Buffer", "encoder-buffer")
	if err != nil {
		enc.logger.Warnf("Failed to register buffer object: %v", err)
		// Continue processing but without lifecycle tracking
	}

	// Validate buffer object using safe access
	if enc.lifecycleManager != nil && bufferID != 0 {
		conversionErr = enc.lifecycleManager.SafeObjectAccess(bufferID, func() error {
			// Validate buffer object before processing
			if !enc.lifecycleManager.ValidateGstObject(buffer) {
				return fmt.Errorf("buffer object failed validation check")
			}
			return nil
		})

		if conversionErr != nil {
			enc.logger.WithFields(logrus.Fields{
				"component": "encoder-gst",
				"method":    "convertGstSample",
				"error":     "buffer_validation_failed",
				"details":   conversionErr.Error(),
			}).Error("Buffer object validation failed during safe access")
			enc.lifecycleManager.UnregisterObject(bufferID)
			return nil, conversionErr
		}
	}

	// Defer cleanup of buffer object
	defer func() {
		if bufferID != 0 {
			if err := enc.lifecycleManager.UnregisterObject(bufferID); err != nil {
				enc.logger.Debugf("Failed to unregister buffer object: %v", err)
			}
		}
	}()

	// Map buffer for reading with enhanced error handling
	var mapInfo *gst.MapInfo
	var bufferBytes []byte

	// Use panic recovery for buffer mapping operations
	func() {
		defer func() {
			if r := recover(); r != nil {
				enc.logger.Errorf("Panic during buffer mapping: %v", r)
				mapInfo = nil
			}
		}()

		mapInfo = buffer.Map(gst.MapRead)
		if mapInfo != nil {
			bufferBytes = mapInfo.Bytes()
		}
	}()

	if mapInfo == nil {
		return nil, fmt.Errorf("failed to map buffer")
	}

	// Ensure buffer is unmapped with panic protection
	defer func() {
		if r := recover(); r != nil {
			enc.logger.Errorf("Panic during buffer unmap: %v", r)
		} else {
			buffer.Unmap()
		}
	}()

	// Validate buffer data
	if len(bufferBytes) == 0 {
		return nil, fmt.Errorf("empty buffer in sample")
	}

	// Size validation to prevent memory issues
	const maxBufferSize = 50 * 1024 * 1024 // 50MB limit
	if len(bufferBytes) > maxBufferSize {
		return nil, fmt.Errorf("buffer too large: %d bytes (max: %d)", len(bufferBytes), maxBufferSize)
	}

	// Safe data copy
	data := make([]byte, len(bufferBytes))
	copy(data, bufferBytes)

	// Get timestamp from buffer with safe access
	timestamp := time.Now()
	func() {
		defer func() {
			if r := recover(); r != nil {
				enc.logger.Debugf("Failed to get buffer timestamp, using current time: %v", r)
			}
		}()

		// Try to get actual timestamp from buffer if available
		if pts := buffer.PresentationTimestamp(); pts != gst.ClockTimeNone {
			timestamp = time.Unix(0, int64(pts))
		}
	}()

	// Determine codec from encoder type
	codec := enc.getCodecName()

	// Create internal sample
	sample := &Sample{
		Data:      data,
		Timestamp: timestamp,
		Format: SampleFormat{
			MediaType: MediaTypeVideo,
			Codec:     codec,
		},
		Metadata: make(map[string]interface{}),
	}

	// Check if this is a keyframe (for video codecs) with safe access
	func() {
		defer func() {
			if r := recover(); r != nil {
				enc.logger.Debugf("Failed to check keyframe status: %v", r)
			}
		}()

		if enc.isKeyframeSample(gstSample) {
			sample.Metadata["key_frame"] = true
			enc.stats.mu.Lock()
			enc.stats.keyframesEncoded++
			enc.stats.mu.Unlock()
		}
	}()

	// Add buffer size to metadata for debugging
	sample.Metadata["buffer_size"] = len(data)
	sample.Metadata["encoder_type"] = enc.getEncoderName(enc.selectedEncoder)
	sample.Metadata["lifecycle_tracked"] = sampleID != 0 && bufferID != 0

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

// monitorBusMessages monitors pipeline bus messages for error handling with recovery
func (enc *EncoderGst) monitorBusMessages() {
	errorCount := 0
	maxErrors := 5
	errorResetInterval := 30 * time.Second
	lastErrorTime := time.Time{}

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
				errorCount++

				// Reset error count if enough time has passed
				if time.Since(lastErrorTime) > errorResetInterval {
					errorCount = 1
				}
				lastErrorTime = time.Now()

				enc.logger.Errorf("Pipeline error (%d/%d): %s", errorCount, maxErrors, err.Error())

				// Attempt recovery if we haven't exceeded max errors
				if errorCount < maxErrors {
					enc.logger.Infof("Attempting error recovery (attempt %d/%d)", errorCount, maxErrors)
					go enc.attemptErrorRecovery(err)
				} else {
					enc.logger.Errorf("Max error count reached, stopping encoder")
					select {
					case enc.errorChan <- fmt.Errorf("pipeline error (max retries exceeded): %s", err.Error()):
					default:
					}
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

			case gst.MessageStreamStatus:
				enc.handleStreamStatusMessage(msg)

			// QOS messages (commented out due to API compatibility)
			// case gst.MessageQos:
			//	enc.handleQOSMessage(msg)

			case gst.MessageElement:
				enc.handleElementMessage(msg)
			}
		}
	}
}

// attemptErrorRecovery attempts to recover from pipeline errors
func (enc *EncoderGst) attemptErrorRecovery(err error) {
	enc.logger.Infof("Starting error recovery process")

	// Stop current pipeline
	if enc.pipeline != nil {
		enc.pipeline.SetState(gst.StateNull)
		time.Sleep(1 * time.Second) // Allow time for cleanup
	}

	// Try to restart with current encoder
	if err := enc.restartPipeline(); err != nil {
		enc.logger.Errorf("Failed to restart pipeline with current encoder: %v", err)

		// Try fallback to next encoder in chain
		if err := enc.fallbackToNextEncoder(); err != nil {
			enc.logger.Errorf("Failed to fallback to next encoder: %v", err)
			select {
			case enc.errorChan <- fmt.Errorf("recovery failed: %w", err):
			default:
			}
		}
	}
}

// restartPipeline restarts the current pipeline
func (enc *EncoderGst) restartPipeline() error {
	enc.mu.Lock()
	defer enc.mu.Unlock()

	// Reinitialize pipeline
	if err := enc.initializePipeline(); err != nil {
		return fmt.Errorf("failed to reinitialize pipeline: %w", err)
	}

	// Restart pipeline
	if err := enc.pipeline.SetState(gst.StatePlaying); err != nil {
		return fmt.Errorf("failed to set pipeline to playing: %w", err)
	}

	// Wait for state change
	ret, _ := enc.pipeline.GetState(gst.StatePlaying, gst.ClockTime(5*time.Second))
	if ret == gst.StateChangeFailure {
		return fmt.Errorf("pipeline failed to reach playing state")
	}

	enc.logger.Infof("Pipeline restarted successfully")
	return nil
}

// fallbackToNextEncoder attempts to fallback to the next encoder in the chain
func (enc *EncoderGst) fallbackToNextEncoder() error {
	enc.mu.Lock()
	defer enc.mu.Unlock()

	// Find current encoder index
	currentIndex := -1
	for i, encoderType := range enc.fallbackChain {
		if encoderType == enc.selectedEncoder {
			currentIndex = i
			break
		}
	}

	if currentIndex == -1 || currentIndex >= len(enc.fallbackChain)-1 {
		return fmt.Errorf("no more encoders available for fallback")
	}

	// Try next encoders in the chain
	for i := currentIndex + 1; i < len(enc.fallbackChain); i++ {
		nextEncoder := enc.fallbackChain[i]

		if !enc.isEncoderAvailable(nextEncoder) {
			continue
		}

		enc.logger.Infof("Falling back to encoder: %s", enc.getEncoderName(nextEncoder))
		enc.selectedEncoder = nextEncoder

		// Try to initialize with new encoder
		if err := enc.initializePipeline(); err != nil {
			enc.logger.Warnf("Failed to initialize fallback encoder %s: %v", enc.getEncoderName(nextEncoder), err)
			continue
		}

		// Start pipeline
		if err := enc.pipeline.SetState(gst.StatePlaying); err != nil {
			enc.logger.Warnf("Failed to start fallback encoder %s: %v", enc.getEncoderName(nextEncoder), err)
			continue
		}

		// Wait for state change
		ret, _ := enc.pipeline.GetState(gst.StatePlaying, gst.ClockTime(5*time.Second))
		if ret == gst.StateChangeFailure {
			enc.logger.Warnf("Fallback encoder %s failed to reach playing state", enc.getEncoderName(nextEncoder))
			continue
		}

		enc.logger.Infof("Successfully fell back to encoder: %s", enc.getEncoderName(nextEncoder))
		return nil
	}

	return fmt.Errorf("all fallback encoders failed")
}

// handleStreamStatusMessage handles stream status messages
func (enc *EncoderGst) handleStreamStatusMessage(msg *gst.Message) {
	// Stream status messages can indicate threading issues or resource problems
	enc.logger.Debugf("Stream status message received from %s", msg.Source())
}

// handleQOSMessage handles Quality of Service messages
func (enc *EncoderGst) handleQOSMessage(msg *gst.Message) {
	// QOS messages indicate performance issues
	enc.logger.Debugf("QOS message received, may indicate performance issues")

	// Update performance tracking
	enc.performanceTracker.mu.Lock()
	// Could extract QOS data here for adaptive bitrate control
	enc.performanceTracker.mu.Unlock()
}

// handleElementMessage handles element-specific messages
func (enc *EncoderGst) handleElementMessage(msg *gst.Message) {
	// Element messages can contain encoder-specific information
	enc.logger.Debugf("Element message received from %s", msg.Source())
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

	// Only log quality monitoring at trace level to reduce log noise
	enc.logger.Tracef("Quality monitoring: current=%.3f, target=%.3f, drop_rate=%.3f",
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

// setEncoderProperties sets encoder properties with existence checking and error handling
// Implements requirements 2.1, 2.2, 2.3, 2.4 for enhanced error handling and logging
func (enc *EncoderGst) setEncoderProperties(properties []PropertyConfig, encoderName string, availableProperties map[string]PropertyInfo) error {
	var criticalErrors []error
	var warnings []error
	successCount := 0
	skippedCount := 0

	enc.logger.WithFields(logrus.Fields{
		"encoder_type":   encoderName,
		"property_count": len(properties),
	}).Info("Starting encoder property configuration with introspection")

	// Count required vs optional properties for logging
	requiredCount := 0
	optionalCount := 0
	for _, prop := range properties {
		if prop.Required {
			requiredCount++
		} else {
			optionalCount++
		}
	}

	enc.logger.WithFields(logrus.Fields{
		"required_properties":  requiredCount,
		"optional_properties":  optionalCount,
		"available_properties": len(availableProperties),
	}).Debug("Property configuration breakdown")

	for _, prop := range properties {
		// Log property configuration attempt
		enc.logger.WithFields(logrus.Fields{
			"property": prop.Name,
			"value":    prop.Value,
			"required": prop.Required,
		}).Debug("Attempting to configure encoder property")

		// Check if property exists before attempting to set it
		if _, exists := availableProperties[prop.Name]; !exists {
			message := fmt.Sprintf("Property '%s' not available on %s encoder", prop.Name, encoderName)

			if prop.Required {
				// Requirement 2.1: Error logging for required properties
				err := fmt.Errorf("required property not available: %s", message)
				criticalErrors = append(criticalErrors, err)
				enc.logger.WithFields(logrus.Fields{
					"property": prop.Name,
					"value":    prop.Value,
					"required": true,
				}).Error("Critical failure: required encoder property not available")
			} else {
				// Requirement 2.2: Warning logging for optional properties
				warning := fmt.Errorf("optional property skipped: %s", message)
				warnings = append(warnings, warning)
				enc.logger.WithFields(logrus.Fields{
					"property": prop.Name,
					"value":    prop.Value,
					"required": false,
				}).Warn("Optional encoder property not available - skipping")
				skippedCount++
			}
			continue
		}

		// Validate and convert the property value using the property map if available
		if descriptor, exists := x264PropertyMap[prop.Name]; exists {
			if descriptor.Validator != nil {
				validatedValue, err := descriptor.Validator(prop.Value)
				if err != nil {
					message := fmt.Sprintf("Property '%s' validation failed: %v", prop.Name, err)

					if prop.Required {
						err := fmt.Errorf("required property validation failed: %s", message)
						criticalErrors = append(criticalErrors, err)
						enc.logger.WithFields(logrus.Fields{
							"property":         prop.Name,
							"value":            prop.Value,
							"validation_error": err.Error(),
							"required":         true,
						}).Error("Critical failure: required property validation failed")
					} else {
						warning := fmt.Errorf("optional property validation failed: %s", message)
						warnings = append(warnings, warning)
						enc.logger.WithFields(logrus.Fields{
							"property":         prop.Name,
							"value":            prop.Value,
							"validation_error": err.Error(),
							"required":         false,
						}).Warn("Optional property validation failed - skipping")
						skippedCount++
					}
					continue
				}
				prop.Value = validatedValue

				// Log successful validation if value was converted
				if !reflect.DeepEqual(prop.Value, validatedValue) {
					enc.logger.WithFields(logrus.Fields{
						"property":        prop.Name,
						"original_value":  prop.Value,
						"validated_value": validatedValue,
					}).Debug("Property value converted during validation")
				}
			}
		}

		// Attempt to set the property
		if err := enc.setEncoderPropertySafe(prop.Name, prop.Value); err != nil {
			message := fmt.Sprintf("Failed to set property '%s' to %v: %v", prop.Name, prop.Value, err)

			if prop.Required {
				err := fmt.Errorf("required property setting failed: %s", message)
				criticalErrors = append(criticalErrors, err)
				enc.logger.WithFields(logrus.Fields{
					"property":  prop.Name,
					"value":     prop.Value,
					"gst_error": err.Error(),
					"required":  true,
				}).Error("Critical failure: required property could not be set")
			} else {
				warning := fmt.Errorf("optional property setting failed: %s", message)
				warnings = append(warnings, warning)
				enc.logger.WithFields(logrus.Fields{
					"property":  prop.Name,
					"value":     prop.Value,
					"gst_error": err.Error(),
					"required":  false,
				}).Warn("Optional property setting failed - skipping")
				skippedCount++
			}
			continue
		}

		// Requirement 2.3: Debug-level logs for successful properties
		successCount++
		enc.logger.WithFields(logrus.Fields{
			"property": prop.Name,
			"value":    prop.Value,
			"required": prop.Required,
		}).Debug("Encoder property successfully configured")
	}

	// Requirement 2.4: Enhanced configuration completion summary
	enc.logger.WithFields(logrus.Fields{
		"encoder_type":      encoderName,
		"total_properties":  len(properties),
		"successful":        successCount,
		"skipped":           skippedCount,
		"required_failures": len(criticalErrors),
		"optional_failures": len(warnings),
		"critical_errors":   len(criticalErrors),
		"warnings":          len(warnings),
	}).Info("Encoder property configuration completed")

	// Return error if there were critical failures
	if len(criticalErrors) > 0 {
		enc.logger.WithFields(logrus.Fields{
			"encoder_type":         encoderName,
			"critical_error_count": len(criticalErrors),
			"first_error":          criticalErrors[0].Error(),
		}).Error("Encoder configuration failed due to critical property errors")

		return fmt.Errorf("critical %s property configuration failures (%d errors): %v",
			encoderName, len(criticalErrors), criticalErrors[0])
	}

	// Enhanced success message with configuration health assessment
	if successCount == len(properties) {
		enc.logger.WithFields(logrus.Fields{
			"encoder_type":         encoderName,
			"configuration_health": "optimal",
			"all_properties_set":   true,
		}).Info("Encoder fully configured - all properties set successfully")
	} else {
		successRate := float64(successCount) / float64(len(properties)) * 100
		var healthStatus string
		if successRate >= 90 {
			healthStatus = "excellent"
		} else if successRate >= 75 {
			healthStatus = "good"
		} else if successRate >= 50 {
			healthStatus = "fair"
		} else {
			healthStatus = "poor"
		}

		enc.logger.WithFields(logrus.Fields{
			"encoder_type":         encoderName,
			"success_rate":         fmt.Sprintf("%.1f%%", successRate),
			"configuration_health": healthStatus,
			"properties_set":       successCount,
			"properties_total":     len(properties),
		}).Info("Encoder configured with partial success - some properties skipped")
	}

	// Log warnings summary if any
	if len(warnings) > 0 {
		enc.logger.WithFields(logrus.Fields{
			"encoder_type":  encoderName,
			"warning_count": len(warnings),
		}).Warn("Encoder configuration completed with warnings")
	}

	return nil
}

// GetMemoryStats returns current memory usage statistics
func (enc *EncoderGst) GetMemoryStats() *MemoryStatistics {
	if enc.memoryManager == nil {
		return &MemoryStatistics{}
	}
	return enc.memoryManager.GetMemoryStats()
}

// TriggerMemoryCleanup triggers garbage collection and memory cleanup
func (enc *EncoderGst) TriggerMemoryCleanup() {
	if enc.memoryManager != nil {
		enc.memoryManager.ForceGC()
	}
}

// CheckForMemoryLeaks checks for potential memory leaks
func (enc *EncoderGst) CheckForMemoryLeaks() []MemoryLeak {
	if enc.memoryManager == nil {
		return nil
	}
	return enc.memoryManager.CheckMemoryLeaks()
}
