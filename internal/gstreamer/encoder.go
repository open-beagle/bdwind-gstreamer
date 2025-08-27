package gstreamer

import (
	"fmt"
	"math"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/open-beagle/bdwind-gstreamer/internal/config"
)

// EncoderStats represents comprehensive encoder statistics
type EncoderStats struct {
	// Frame statistics
	FramesEncoded    int64 `json:"frames_encoded"`
	KeyframesEncoded int64 `json:"keyframes_encoded"`
	DropFrames       int64 `json:"drop_frames"`
	SkippedFrames    int64 `json:"skipped_frames"`

	// Timing statistics
	EncodingLatency int64 `json:"encoding_latency_ms"`
	AverageLatency  int64 `json:"average_latency_ms"`
	MinLatency      int64 `json:"min_latency_ms"`
	MaxLatency      int64 `json:"max_latency_ms"`

	// Bitrate statistics
	CurrentBitrate int `json:"current_bitrate_kbps"`
	AverageBitrate int `json:"average_bitrate_kbps"`
	TargetBitrate  int `json:"target_bitrate_kbps"`
	PeakBitrate    int `json:"peak_bitrate_kbps"`

	// Quality statistics
	AverageQuality  float64 `json:"average_quality"`
	CurrentQuality  float64 `json:"current_quality"`
	QualityVariance float64 `json:"quality_variance"`

	// Performance statistics
	EncodingFPS float64 `json:"encoding_fps"`
	CPUUsage    float64 `json:"cpu_usage_percent"`
	MemoryUsage int64   `json:"memory_usage_bytes"`

	// Error statistics
	ErrorCount int64  `json:"error_count"`
	LastError  string `json:"last_error"`

	// Timing
	StartTime     time.Time `json:"start_time"`
	LastUpdate    time.Time `json:"last_update"`
	TotalDuration int64     `json:"total_duration_ms"`
}

// Encoder interface defines the contract for video encoders
type Encoder interface {
	// GetElement returns the GStreamer element name and properties for this encoder
	GetElement() (string, map[string]interface{}, error)

	// UpdateBitrate dynamically updates the encoder bitrate
	UpdateBitrate(bitrate int) error

	// SetBitrate dynamically sets the encoder bitrate (for WebRTC integration)
	SetBitrate(bitrate int) error

	// ForceKeyframe forces the encoder to generate a keyframe
	ForceKeyframe() error

	// GetStats returns current encoder statistics
	GetStats() EncoderStats

	// IsHardwareAccelerated returns true if this encoder uses hardware acceleration
	IsHardwareAccelerated() bool

	// GetSupportedCodecs returns the list of codecs supported by this encoder
	GetSupportedCodecs() []config.CodecType

	// Initialize initializes the encoder with the given configuration
	Initialize(config config.EncoderConfig) error

	// Cleanup cleans up encoder resources
	Cleanup() error

	// PushFrame pushes a raw video frame to the encoder
	PushFrame(sample *Sample) error

	// SetSampleCallback sets the callback function for processing encoded video samples
	SetSampleCallback(callback func(*Sample) error) error
}

// BaseEncoder provides common functionality for all encoders
type BaseEncoder struct {
	config config.EncoderConfig
	stats  EncoderStats
	mutex  sync.RWMutex

	// Statistics collection
	statsCollector *EncoderStatsCollector

	// GStreamer element reference (would be actual GStreamer element in real implementation)
	element interface{}
	// Callback for encoded samples
	sampleCallback func(*Sample) error
	// Logger for this encoder
	logger *logrus.Entry
}

// NewBaseEncoder creates a new base encoder
func NewBaseEncoder(cfg config.EncoderConfig) *BaseEncoder {
	now := time.Now()
	encoder := &BaseEncoder{
		config: cfg,
		stats: EncoderStats{
			StartTime:      now,
			LastUpdate:     now,
			TargetBitrate:  cfg.Bitrate,
			CurrentBitrate: cfg.Bitrate,
			MinLatency:     9999999, // Initialize to high value
		},
		logger: config.GetLoggerWithPrefix("gstreamer-encoder"),
	}
	encoder.statsCollector = NewEncoderStatsCollector(encoder)
	return encoder
}

// PushFrame pushes a raw video frame to the encoder (placeholder implementation)
func (e *BaseEncoder) PushFrame(sample *Sample) error {
	// This is a placeholder. In a real implementation, this method would
	// send the sample to the GStreamer element for encoding.
	// For now, we'll just log that a frame was received.
	e.logger.Debugf("Received frame with size %d", len(sample.Data))

	// Simulate encoding by updating stats
	e.statsCollector.RecordFrameEncoded(false, 10, 0.9, e.config.Bitrate)

	e.mutex.RLock()
	callback := e.sampleCallback
	e.mutex.RUnlock()

	if callback != nil {
		// For now, just pass the raw sample through. This will be replaced with actual encoded data.
		return callback(sample)
	}

	return nil
}

// SetSampleCallback sets the callback function for processing encoded video samples
func (e *BaseEncoder) SetSampleCallback(callback func(*Sample) error) error {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	e.sampleCallback = callback
	return nil
}

// GetStats returns current encoder statistics (thread-safe)
func (e *BaseEncoder) GetStats() EncoderStats {
	e.mutex.RLock()
	defer e.mutex.RUnlock()
	return e.stats
}

// updateStats updates encoder statistics (internal method)
func (e *BaseEncoder) updateStats(framesEncoded, keyframesEncoded, dropFrames int64, latency int64, bitrate int) {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	e.stats.FramesEncoded = framesEncoded
	e.stats.KeyframesEncoded = keyframesEncoded
	e.stats.DropFrames = dropFrames
	e.stats.EncodingLatency = latency
	e.stats.CurrentBitrate = bitrate
	e.stats.LastUpdate = time.Now()
}

// GetConfig returns the encoder configuration
func (e *BaseEncoder) GetConfig() config.EncoderConfig {
	e.mutex.RLock()
	defer e.mutex.RUnlock()
	return e.config
}

// UpdateConfig updates the encoder configuration
func (e *BaseEncoder) UpdateConfig(newConfig config.EncoderConfig) error {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	if err := config.ValidateEncoderConfig(&newConfig); err != nil {
		return fmt.Errorf("invalid encoder config: %w", err)
	}

	e.config = newConfig
	return nil
}

// SetBitrate dynamically sets the encoder bitrate (for WebRTC integration)
func (e *BaseEncoder) SetBitrate(bitrate int) error {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	// Update configuration
	e.config.Bitrate = bitrate
	e.stats.TargetBitrate = bitrate
	e.stats.CurrentBitrate = bitrate

	// In a real implementation, this would update the GStreamer element properties
	// For now, we'll just update the internal state
	e.logger.Infof("Bitrate set to %d kbps", bitrate)
	return nil
}

// UpdateBitrate is an alias for SetBitrate for interface compatibility
func (e *BaseEncoder) UpdateBitrate(bitrate int) error {
	return e.SetBitrate(bitrate)
}

// EncoderFactory creates encoders based on type
type EncoderFactory struct{}

// NewEncoderFactory creates a new encoder factory
func NewEncoderFactory() *EncoderFactory {
	return &EncoderFactory{}
}

// CreateEncoder creates an encoder based on the configuration
func (f *EncoderFactory) CreateEncoder(cfg config.EncoderConfig) (Encoder, error) {
	switch cfg.Type {
	case config.EncoderTypeNVENC:
		return NewNVENCEncoder(cfg)
	case config.EncoderTypeVAAPI:
		return NewVAAPIEncoder(cfg)
	case config.EncoderTypeX264:
		return NewX264Encoder(cfg)
	case config.EncoderTypeVP8:
		return NewVP8Encoder(cfg)
	case config.EncoderTypeVP9:
		return NewVP9Encoder(cfg)
	case config.EncoderTypeAuto:
		return f.createAutoEncoder(cfg)
	default:
		return nil, fmt.Errorf("unsupported encoder type: %s", cfg.Type)
	}
}

// createAutoEncoder automatically selects the best available encoder
func (f *EncoderFactory) createAutoEncoder(cfg config.EncoderConfig) (Encoder, error) {
	var lastError error

	// Get encoder preferences based on codec and system capabilities
	preferences := f.getEncoderPreferences(cfg)

	for _, encoderType := range preferences {
		testConfig := cfg
		testConfig.Type = encoderType

		encoder, err := f.tryCreateEncoder(testConfig)
		if err == nil {
			return encoder, nil
		}
		lastError = err
	}

	// If all preferred encoders failed, try fallback to software if enabled
	if cfg.FallbackToSoft && !containsEncoderType(preferences, config.EncoderTypeX264) {
		x264Config := cfg
		x264Config.Type = config.EncoderTypeX264
		if encoder, err := NewX264Encoder(x264Config); err == nil {
			return encoder, nil
		}
	}

	if lastError != nil {
		return nil, fmt.Errorf("no suitable encoder found, last error: %w", lastError)
	}
	return nil, fmt.Errorf("no suitable encoder found")
}

// getEncoderPreferences returns a prioritized list of encoder types based on codec and system
func (f *EncoderFactory) getEncoderPreferences(cfg config.EncoderConfig) []config.EncoderType {
	var preferences []config.EncoderType

	// Hardware encoders first if enabled
	if cfg.UseHardware {
		switch cfg.Codec {
		case config.CodecH264:
			// For H.264, prefer NVENC > VAAPI
			if IsNVENCAvailable() {
				preferences = append(preferences, config.EncoderTypeNVENC)
			}
			if IsVAAPIAvailable() {
				preferences = append(preferences, config.EncoderTypeVAAPI)
			}
		case config.CodecVP8, config.CodecVP9:
			// For VP8/VP9, prefer VAAPI (NVENC doesn't support VP8/VP9)
			if IsVAAPIAvailable() {
				preferences = append(preferences, config.EncoderTypeVAAPI)
			}
		}
	}

	// Software encoders as fallback
	switch cfg.Codec {
	case config.CodecH264:
		preferences = append(preferences, config.EncoderTypeX264)
	case config.CodecVP8:
		preferences = append(preferences, config.EncoderTypeVP8)
	case config.CodecVP9:
		preferences = append(preferences, config.EncoderTypeVP9)
	}

	return preferences
}

// tryCreateEncoder attempts to create an encoder with error handling
func (f *EncoderFactory) tryCreateEncoder(cfg config.EncoderConfig) (Encoder, error) {
	switch cfg.Type {
	case config.EncoderTypeNVENC:
		return NewNVENCEncoder(cfg)
	case config.EncoderTypeVAAPI:
		return NewVAAPIEncoder(cfg)
	case config.EncoderTypeX264:
		return NewX264Encoder(cfg)
	case config.EncoderTypeVP8:
		return NewVP8Encoder(cfg)
	case config.EncoderTypeVP9:
		return NewVP9Encoder(cfg)
	default:
		return nil, fmt.Errorf("unsupported encoder type: %s", cfg.Type)
	}
}

// containsEncoderType checks if a slice contains a specific encoder type
func containsEncoderType(slice []config.EncoderType, item config.EncoderType) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// Hardware availability check functions
func IsNVENCAvailable() bool {
	// Check for NVIDIA GPU devices
	if fileExists("/dev/nvidia0") || fileExists("/proc/driver/nvidia/version") {
		return true
	}

	// Check for nvidia kernel module
	if output := runCommand("lsmod"); output != "" {
		return strings.Contains(output, "nvidia")
	}

	return false
}

func IsVAAPIAvailable() bool {
	// Check for DRI devices (Intel/AMD GPUs)
	if fileExists("/dev/dri/renderD128") || fileExists("/dev/dri/card0") {
		return true
	}

	// Check for Intel/AMD kernel modules
	if output := runCommand("lsmod"); output != "" {
		return strings.Contains(output, "i915") ||
			strings.Contains(output, "amdgpu") ||
			strings.Contains(output, "radeon")
	}

	return false
}

// runCommand runs a system command and returns output
func runCommand(name string, args ...string) string {
	// In real implementation, this would use exec.Command
	// For now, return empty string as placeholder
	return ""
}

// Helper function to check file existence
func fileExists(filename string) bool {
	_, err := os.Stat(filename)
	return err == nil
}

// Stub implementations for other encoder types (to be implemented in separate tasks)

// NewVAAPIEncoder is implemented in vaapi_encoder.go

// NewX264Encoder is implemented in x264_encoder.go

// NewVP8Encoder creates a VP8 encoder
func NewVP8Encoder(cfg config.EncoderConfig) (Encoder, error) {
	encoder := &VP8Encoder{
		BaseEncoder: NewBaseEncoder(cfg),
	}

	if err := encoder.Initialize(cfg); err != nil {
		return nil, fmt.Errorf("failed to initialize VP8 encoder: %w", err)
	}

	return encoder, nil
}

// NewVP9Encoder creates a VP9 encoder
func NewVP9Encoder(cfg config.EncoderConfig) (Encoder, error) {
	encoder := &VP9Encoder{
		BaseEncoder: NewBaseEncoder(cfg),
	}

	if err := encoder.Initialize(cfg); err != nil {
		return nil, fmt.Errorf("failed to initialize VP9 encoder: %w", err)
	}

	return encoder, nil
}

// EncoderSelector provides advanced encoder selection capabilities
type EncoderSelector struct {
	factory *EncoderFactory
}

// NewEncoderSelector creates a new encoder selector
func NewEncoderSelector() *EncoderSelector {
	return &EncoderSelector{
		factory: NewEncoderFactory(),
	}
}

// SelectBestEncoder selects the best encoder based on requirements and system capabilities
func (s *EncoderSelector) SelectBestEncoder(cfg config.EncoderConfig, requirements EncoderRequirements) (Encoder, error) {
	// Get available encoders
	available := s.getAvailableEncoders(cfg.Codec)
	if len(available) == 0 {
		return nil, fmt.Errorf("no encoders available for codec %s", cfg.Codec)
	}

	// Score and rank encoders
	scored := s.scoreEncoders(available, cfg, requirements)

	// Try encoders in order of score
	var lastError error
	for _, item := range scored {
		testConfig := cfg
		testConfig.Type = item.EncoderType

		encoder, err := s.factory.tryCreateEncoder(testConfig)
		if err == nil {
			return encoder, nil
		}
		lastError = err
	}

	return nil, fmt.Errorf("failed to create any suitable encoder, last error: %w", lastError)
}

// EncoderRequirements defines requirements for encoder selection
type EncoderRequirements struct {
	MaxLatency      int     // Maximum acceptable latency in milliseconds
	MinQuality      float64 // Minimum quality score (0-1)
	MaxCPUUsage     float64 // Maximum CPU usage (0-1)
	PreferHardware  bool    // Prefer hardware acceleration
	RequireRealtime bool    // Require real-time encoding capability
}

// ScoredEncoder represents an encoder with its suitability score
type ScoredEncoder struct {
	EncoderType config.EncoderType
	Score       float64
	Reasons     []string
}

// getAvailableEncoders returns available encoders for a given codec
func (s *EncoderSelector) getAvailableEncoders(codec config.CodecType) []config.EncoderType {
	var available []config.EncoderType

	switch codec {
	case config.CodecH264:
		if IsNVENCAvailable() {
			available = append(available, config.EncoderTypeNVENC)
		}
		if IsVAAPIAvailable() {
			available = append(available, config.EncoderTypeVAAPI)
		}
		available = append(available, config.EncoderTypeX264)
	case config.CodecVP8:
		if IsVAAPIAvailable() {
			available = append(available, config.EncoderTypeVAAPI)
		}
		available = append(available, config.EncoderTypeVP8)
	case config.CodecVP9:
		if IsVAAPIAvailable() {
			available = append(available, config.EncoderTypeVAAPI)
		}
		available = append(available, config.EncoderTypeVP9)
	}

	return available
}

// scoreEncoders scores and ranks encoders based on requirements
func (s *EncoderSelector) scoreEncoders(available []config.EncoderType, cfg config.EncoderConfig, req EncoderRequirements) []ScoredEncoder {
	var scored []ScoredEncoder

	for _, encoderType := range available {
		score, reasons := s.scoreEncoder(encoderType, cfg, req)
		scored = append(scored, ScoredEncoder{
			EncoderType: encoderType,
			Score:       score,
			Reasons:     reasons,
		})
	}

	// Sort by score (highest first)
	for i := 0; i < len(scored)-1; i++ {
		for j := i + 1; j < len(scored); j++ {
			if scored[j].Score > scored[i].Score {
				scored[i], scored[j] = scored[j], scored[i]
			}
		}
	}

	return scored
}

// scoreEncoder scores a single encoder type
func (s *EncoderSelector) scoreEncoder(encoderType config.EncoderType, cfg config.EncoderConfig, req EncoderRequirements) (float64, []string) {
	score := 0.0
	var reasons []string

	// Base scores for encoder types
	switch encoderType {
	case config.EncoderTypeNVENC:
		score += 90.0
		reasons = append(reasons, "NVENC: excellent performance and quality")
		if req.PreferHardware {
			score += 10.0
			reasons = append(reasons, "hardware acceleration preferred")
		}
		if req.RequireRealtime {
			score += 15.0
			reasons = append(reasons, "excellent real-time performance")
		}
	case config.EncoderTypeVAAPI:
		score += 80.0
		reasons = append(reasons, "VAAPI: good performance and quality")
		if req.PreferHardware {
			score += 10.0
			reasons = append(reasons, "hardware acceleration preferred")
		}
		if req.RequireRealtime {
			score += 10.0
			reasons = append(reasons, "good real-time performance")
		}
	case config.EncoderTypeX264:
		score += 70.0
		reasons = append(reasons, "x264: excellent quality, moderate performance")
		if !req.PreferHardware {
			score += 5.0
			reasons = append(reasons, "software encoding acceptable")
		}
		if req.RequireRealtime {
			score -= 10.0
			reasons = append(reasons, "may struggle with real-time at high quality")
		}
	case config.EncoderTypeVP8, config.EncoderTypeVP9:
		score += 60.0
		reasons = append(reasons, "VP8/VP9: good quality, moderate performance")
		if req.RequireRealtime {
			score -= 15.0
			reasons = append(reasons, "VP8/VP9 can be slow for real-time")
		}
	}

	// Adjust for latency requirements
	if req.MaxLatency > 0 {
		switch encoderType {
		case config.EncoderTypeNVENC:
			if req.MaxLatency < 50 {
				score += 10.0
				reasons = append(reasons, "excellent low-latency performance")
			}
		case config.EncoderTypeVAAPI:
			if req.MaxLatency < 100 {
				score += 5.0
				reasons = append(reasons, "good low-latency performance")
			}
		case config.EncoderTypeX264:
			if req.MaxLatency < 200 {
				score -= 5.0
				reasons = append(reasons, "may have higher latency")
			}
		}
	}

	// Adjust for CPU usage requirements
	if req.MaxCPUUsage > 0 {
		switch encoderType {
		case config.EncoderTypeNVENC, config.EncoderTypeVAAPI:
			if req.MaxCPUUsage < 0.3 {
				score += 15.0
				reasons = append(reasons, "low CPU usage")
			}
		case config.EncoderTypeX264:
			if req.MaxCPUUsage < 0.5 {
				score -= 10.0
				reasons = append(reasons, "high CPU usage")
			}
		}
	}

	return score, reasons
}

// GetEncoderCapabilities returns capabilities of a specific encoder type
func GetEncoderCapabilities(encoderType config.EncoderType) EncoderCapabilities {
	switch encoderType {
	case config.EncoderTypeNVENC:
		return EncoderCapabilities{
			IsHardwareAccelerated: true,
			SupportedCodecs:       []config.CodecType{config.CodecH264},
			MaxResolution:         "4K",
			TypicalLatency:        20,
			TypicalCPUUsage:       0.1,
			QualityRating:         0.9,
			PerformanceRating:     0.95,
		}
	case config.EncoderTypeVAAPI:
		return EncoderCapabilities{
			IsHardwareAccelerated: true,
			SupportedCodecs:       []config.CodecType{config.CodecH264, config.CodecVP8, config.CodecVP9},
			MaxResolution:         "4K",
			TypicalLatency:        30,
			TypicalCPUUsage:       0.15,
			QualityRating:         0.8,
			PerformanceRating:     0.85,
		}
	case config.EncoderTypeX264:
		return EncoderCapabilities{
			IsHardwareAccelerated: false,
			SupportedCodecs:       []config.CodecType{config.CodecH264},
			MaxResolution:         "4K",
			TypicalLatency:        50,
			TypicalCPUUsage:       0.6,
			QualityRating:         0.95,
			PerformanceRating:     0.7,
		}
	case config.EncoderTypeVP8:
		return EncoderCapabilities{
			IsHardwareAccelerated: false,
			SupportedCodecs:       []config.CodecType{config.CodecVP8},
			MaxResolution:         "1080p",
			TypicalLatency:        80,
			TypicalCPUUsage:       0.7,
			QualityRating:         0.8,
			PerformanceRating:     0.6,
		}
	case config.EncoderTypeVP9:
		return EncoderCapabilities{
			IsHardwareAccelerated: false,
			SupportedCodecs:       []config.CodecType{config.CodecVP9},
			MaxResolution:         "4K",
			TypicalLatency:        100,
			TypicalCPUUsage:       0.8,
			QualityRating:         0.9,
			PerformanceRating:     0.5,
		}
	default:
		return EncoderCapabilities{}
	}
}

// EncoderCapabilities describes the capabilities of an encoder
type EncoderCapabilities struct {
	IsHardwareAccelerated bool
	SupportedCodecs       []config.CodecType
	MaxResolution         string
	TypicalLatency        int     // milliseconds
	TypicalCPUUsage       float64 // 0-1
	QualityRating         float64 // 0-1
	PerformanceRating     float64 // 0-1
}

// EncoderStatsCollector collects and manages encoder statistics
type EncoderStatsCollector struct {
	encoder *BaseEncoder

	// Internal tracking
	latencyHistory []int64
	bitrateHistory []int
	qualityHistory []float64
	fpsHistory     []float64

	// Timing
	lastFrameTime time.Time
	frameCount    int64

	// Moving averages
	latencyWindow int
	bitrateWindow int
	qualityWindow int
	fpsWindow     int
}

// NewEncoderStatsCollector creates a new statistics collector
func NewEncoderStatsCollector(encoder *BaseEncoder) *EncoderStatsCollector {
	return &EncoderStatsCollector{
		encoder:       encoder,
		latencyWindow: 100, // Keep last 100 latency measurements
		bitrateWindow: 60,  // Keep last 60 bitrate measurements (1 minute at 1Hz)
		qualityWindow: 30,  // Keep last 30 quality measurements
		fpsWindow:     30,  // Keep last 30 FPS measurements
		lastFrameTime: time.Now(),
	}
}

// RecordFrameEncoded records that a frame was encoded
func (c *EncoderStatsCollector) RecordFrameEncoded(isKeyframe bool, latency int64, quality float64, bitrate int) {
	c.encoder.mutex.Lock()
	defer c.encoder.mutex.Unlock()

	now := time.Now()

	// Update frame counts
	c.encoder.stats.FramesEncoded++
	c.frameCount++
	if isKeyframe {
		c.encoder.stats.KeyframesEncoded++
	}

	// Update latency statistics
	c.recordLatency(latency)

	// Update bitrate statistics
	c.recordBitrate(bitrate)

	// Update quality statistics
	c.recordQuality(quality)

	// Update FPS
	c.updateFPS(now)

	// Update timing
	c.encoder.stats.LastUpdate = now
	c.encoder.stats.TotalDuration = now.Sub(c.encoder.stats.StartTime).Milliseconds()
	c.lastFrameTime = now
}

// RecordFrameDropped records that a frame was dropped
func (c *EncoderStatsCollector) RecordFrameDropped() {
	c.encoder.mutex.Lock()
	defer c.encoder.mutex.Unlock()

	c.encoder.stats.DropFrames++
	c.encoder.stats.LastUpdate = time.Now()
}

// RecordFrameSkipped records that a frame was skipped
func (c *EncoderStatsCollector) RecordFrameSkipped() {
	c.encoder.mutex.Lock()
	defer c.encoder.mutex.Unlock()

	c.encoder.stats.SkippedFrames++
	c.encoder.stats.LastUpdate = time.Now()
}

// RecordError records an encoding error
func (c *EncoderStatsCollector) RecordError(err error) {
	c.encoder.mutex.Lock()
	defer c.encoder.mutex.Unlock()

	c.encoder.stats.ErrorCount++
	c.encoder.stats.LastError = err.Error()
	c.encoder.stats.LastUpdate = time.Now()
}

// UpdateResourceUsage updates CPU and memory usage statistics
func (c *EncoderStatsCollector) UpdateResourceUsage(cpuUsage float64, memoryUsage int64) {
	c.encoder.mutex.Lock()
	defer c.encoder.mutex.Unlock()

	c.encoder.stats.CPUUsage = cpuUsage
	c.encoder.stats.MemoryUsage = memoryUsage
	c.encoder.stats.LastUpdate = time.Now()
}

// recordLatency updates latency statistics
func (c *EncoderStatsCollector) recordLatency(latency int64) {
	c.encoder.stats.EncodingLatency = latency

	// Update min/max
	if latency < c.encoder.stats.MinLatency {
		c.encoder.stats.MinLatency = latency
	}
	if latency > c.encoder.stats.MaxLatency {
		c.encoder.stats.MaxLatency = latency
	}

	// Add to history
	c.latencyHistory = append(c.latencyHistory, latency)
	if len(c.latencyHistory) > c.latencyWindow {
		c.latencyHistory = c.latencyHistory[1:]
	}

	// Calculate average
	if len(c.latencyHistory) > 0 {
		var sum int64
		for _, l := range c.latencyHistory {
			sum += l
		}
		c.encoder.stats.AverageLatency = sum / int64(len(c.latencyHistory))
	}
}

// recordBitrate updates bitrate statistics
func (c *EncoderStatsCollector) recordBitrate(bitrate int) {
	c.encoder.stats.CurrentBitrate = bitrate

	// Update peak
	if bitrate > c.encoder.stats.PeakBitrate {
		c.encoder.stats.PeakBitrate = bitrate
	}

	// Add to history
	c.bitrateHistory = append(c.bitrateHistory, bitrate)
	if len(c.bitrateHistory) > c.bitrateWindow {
		c.bitrateHistory = c.bitrateHistory[1:]
	}

	// Calculate average
	if len(c.bitrateHistory) > 0 {
		var sum int
		for _, b := range c.bitrateHistory {
			sum += b
		}
		c.encoder.stats.AverageBitrate = sum / len(c.bitrateHistory)
	}
}

// recordQuality updates quality statistics
func (c *EncoderStatsCollector) recordQuality(quality float64) {
	c.encoder.stats.CurrentQuality = quality

	// Add to history
	c.qualityHistory = append(c.qualityHistory, quality)
	if len(c.qualityHistory) > c.qualityWindow {
		c.qualityHistory = c.qualityHistory[1:]
	}

	// Calculate average and variance
	if len(c.qualityHistory) > 0 {
		var sum float64
		for _, q := range c.qualityHistory {
			sum += q
		}
		c.encoder.stats.AverageQuality = sum / float64(len(c.qualityHistory))

		// Calculate variance
		if len(c.qualityHistory) > 1 {
			var variance float64
			for _, q := range c.qualityHistory {
				diff := q - c.encoder.stats.AverageQuality
				variance += diff * diff
			}
			c.encoder.stats.QualityVariance = variance / float64(len(c.qualityHistory)-1)
		}
	}
}

// updateFPS updates FPS statistics
func (c *EncoderStatsCollector) updateFPS(now time.Time) {
	if !c.lastFrameTime.IsZero() {
		frameDuration := now.Sub(c.lastFrameTime)
		if frameDuration > 0 {
			fps := 1.0 / frameDuration.Seconds()

			// Add to history
			c.fpsHistory = append(c.fpsHistory, fps)
			if len(c.fpsHistory) > c.fpsWindow {
				c.fpsHistory = c.fpsHistory[1:]
			}

			// Calculate average FPS
			if len(c.fpsHistory) > 0 {
				var sum float64
				for _, f := range c.fpsHistory {
					sum += f
				}
				c.encoder.stats.EncodingFPS = sum / float64(len(c.fpsHistory))
			}
		}
	}
}

// GetDetailedStats returns detailed statistics with additional computed metrics
func (c *EncoderStatsCollector) GetDetailedStats() DetailedEncoderStats {
	c.encoder.mutex.RLock()
	defer c.encoder.mutex.RUnlock()

	stats := c.encoder.stats

	// Calculate additional metrics
	var keyframeRatio float64
	if stats.FramesEncoded > 0 {
		keyframeRatio = float64(stats.KeyframesEncoded) / float64(stats.FramesEncoded)
	}

	var dropRatio float64
	totalFrames := stats.FramesEncoded + stats.DropFrames
	if totalFrames > 0 {
		dropRatio = float64(stats.DropFrames) / float64(totalFrames)
	}

	var bitrateEfficiency float64
	if stats.TargetBitrate > 0 {
		bitrateEfficiency = float64(stats.AverageBitrate) / float64(stats.TargetBitrate)
	}

	return DetailedEncoderStats{
		EncoderStats:      stats,
		KeyframeRatio:     keyframeRatio,
		DropRatio:         dropRatio,
		BitrateEfficiency: bitrateEfficiency,
		LatencyStdDev:     c.calculateLatencyStdDev(),
		QualityStdDev:     c.calculateQualityStdDev(),
	}
}

// calculateLatencyStdDev calculates standard deviation of latency
func (c *EncoderStatsCollector) calculateLatencyStdDev() float64 {
	if len(c.latencyHistory) < 2 {
		return 0
	}

	mean := float64(c.encoder.stats.AverageLatency)
	var variance float64

	for _, latency := range c.latencyHistory {
		diff := float64(latency) - mean
		variance += diff * diff
	}

	variance /= float64(len(c.latencyHistory) - 1)
	return math.Sqrt(variance)
}

// calculateQualityStdDev calculates standard deviation of quality
func (c *EncoderStatsCollector) calculateQualityStdDev() float64 {
	return math.Sqrt(c.encoder.stats.QualityVariance)
}

// Reset resets all statistics
func (c *EncoderStatsCollector) Reset() {
	c.encoder.mutex.Lock()
	defer c.encoder.mutex.Unlock()

	now := time.Now()
	c.encoder.stats = EncoderStats{
		StartTime:      now,
		LastUpdate:     now,
		TargetBitrate:  c.encoder.config.Bitrate,
		CurrentBitrate: c.encoder.config.Bitrate,
		MinLatency:     9999999,
	}

	// Clear history
	c.latencyHistory = nil
	c.bitrateHistory = nil
	c.qualityHistory = nil
	c.fpsHistory = nil
	c.frameCount = 0
	c.lastFrameTime = now
}

// DetailedEncoderStats provides additional computed statistics
type DetailedEncoderStats struct {
	EncoderStats
	KeyframeRatio     float64 `json:"keyframe_ratio"`
	DropRatio         float64 `json:"drop_ratio"`
	BitrateEfficiency float64 `json:"bitrate_efficiency"`
	LatencyStdDev     float64 `json:"latency_std_dev"`
	QualityStdDev     float64 `json:"quality_std_dev"`
}

// VP8Encoder implements VP8 encoding
type VP8Encoder struct {
	*BaseEncoder
}

// Initialize initializes the VP8 encoder
func (e *VP8Encoder) Initialize(cfg config.EncoderConfig) error {
	if err := config.ValidateEncoderConfig(&cfg); err != nil {
		return fmt.Errorf("invalid encoder config: %w", err)
	}

	if cfg.Codec != config.CodecVP8 {
		return fmt.Errorf("VP8 encoder only supports VP8 codec, got %s", cfg.Codec)
	}

	e.config = cfg
	return nil
}

// GetElement returns the GStreamer element name and properties for VP8
func (e *VP8Encoder) GetElement() (string, map[string]interface{}, error) {
	properties := make(map[string]interface{})
	properties["target-bitrate"] = e.config.Bitrate * 1000           // Convert kbps to bps
	properties["keyframe-max-dist"] = e.config.KeyframeInterval * 30 // Convert seconds to frames

	return "vp8enc", properties, nil
}

// IsHardwareAccelerated returns false since VP8 is software-based
func (e *VP8Encoder) IsHardwareAccelerated() bool {
	return false
}

// GetSupportedCodecs returns the codecs supported by VP8 encoder
func (e *VP8Encoder) GetSupportedCodecs() []config.CodecType {
	return []config.CodecType{config.CodecVP8}
}

// ForceKeyframe forces the encoder to generate a keyframe
func (e *VP8Encoder) ForceKeyframe() error {
	// Update statistics
	e.mutex.Lock()
	e.stats.KeyframesEncoded++
	e.mutex.Unlock()
	return nil
}

// Cleanup cleans up VP8 encoder resources
func (e *VP8Encoder) Cleanup() error {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	if e.element != nil {
		e.element = nil
	}
	return nil
}

// VP9Encoder implements VP9 encoding
type VP9Encoder struct {
	*BaseEncoder
}

// Initialize initializes the VP9 encoder
func (e *VP9Encoder) Initialize(cfg config.EncoderConfig) error {
	if err := config.ValidateEncoderConfig(&cfg); err != nil {
		return fmt.Errorf("invalid encoder config: %w", err)
	}

	if cfg.Codec != config.CodecVP9 {
		return fmt.Errorf("VP9 encoder only supports VP9 codec, got %s", cfg.Codec)
	}

	e.config = cfg
	return nil
}

// GetElement returns the GStreamer element name and properties for VP9
func (e *VP9Encoder) GetElement() (string, map[string]interface{}, error) {
	properties := make(map[string]interface{})
	properties["target-bitrate"] = e.config.Bitrate * 1000           // Convert kbps to bps
	properties["keyframe-max-dist"] = e.config.KeyframeInterval * 30 // Convert seconds to frames

	return "vp9enc", properties, nil
}

// IsHardwareAccelerated returns false since VP9 is software-based
func (e *VP9Encoder) IsHardwareAccelerated() bool {
	return false
}

// GetSupportedCodecs returns the codecs supported by VP9 encoder
func (e *VP9Encoder) GetSupportedCodecs() []config.CodecType {
	return []config.CodecType{config.CodecVP9}
}

// ForceKeyframe forces the encoder to generate a keyframe
func (e *VP9Encoder) ForceKeyframe() error {
	// Update statistics
	e.mutex.Lock()
	e.stats.KeyframesEncoded++
	e.mutex.Unlock()
	return nil
}

// Cleanup cleans up VP9 encoder resources
func (e *VP9Encoder) Cleanup() error {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	if e.element != nil {
		e.element = nil
	}
	return nil
}

// EncoderStatsSnapshot represents a point-in-time snapshot of encoder statistics
type EncoderStatsSnapshot struct {
	Timestamp time.Time    `json:"timestamp"`
	Stats     EncoderStats `json:"stats"`
}

// EncoderStatsHistory maintains a history of encoder statistics snapshots
type EncoderStatsHistory struct {
	snapshots []EncoderStatsSnapshot
	maxSize   int
	mutex     sync.RWMutex
}

// NewEncoderStatsHistory creates a new statistics history tracker
func NewEncoderStatsHistory(maxSize int) *EncoderStatsHistory {
	return &EncoderStatsHistory{
		maxSize: maxSize,
	}
}

// AddSnapshot adds a new statistics snapshot
func (h *EncoderStatsHistory) AddSnapshot(stats EncoderStats) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	snapshot := EncoderStatsSnapshot{
		Timestamp: time.Now(),
		Stats:     stats,
	}

	h.snapshots = append(h.snapshots, snapshot)
	if len(h.snapshots) > h.maxSize {
		h.snapshots = h.snapshots[1:]
	}
}

// GetSnapshots returns all snapshots
func (h *EncoderStatsHistory) GetSnapshots() []EncoderStatsSnapshot {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	// Return a copy to avoid race conditions
	result := make([]EncoderStatsSnapshot, len(h.snapshots))
	copy(result, h.snapshots)
	return result
}

// GetSnapshotsSince returns snapshots since a specific time
func (h *EncoderStatsHistory) GetSnapshotsSince(since time.Time) []EncoderStatsSnapshot {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	var result []EncoderStatsSnapshot
	for _, snapshot := range h.snapshots {
		if snapshot.Timestamp.After(since) {
			result = append(result, snapshot)
		}
	}
	return result
}

// Clear clears all snapshots
func (h *EncoderStatsHistory) Clear() {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	h.snapshots = nil
}
