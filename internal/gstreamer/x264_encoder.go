package gstreamer

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/open-beagle/bdwind-gstreamer/internal/config"
)

// X264Encoder implements software encoding using x264
type X264Encoder struct {
	*BaseEncoder
}

// NewX264Encoder creates a new x264 encoder
func NewX264Encoder(cfg config.EncoderConfig) (*X264Encoder, error) {
	encoder := &X264Encoder{
		BaseEncoder: NewBaseEncoder(cfg),
	}

	if err := encoder.Initialize(cfg); err != nil {
		return nil, fmt.Errorf("failed to initialize x264 encoder: %w", err)
	}

	return encoder, nil
}

// Initialize initializes the x264 encoder
func (e *X264Encoder) Initialize(cfg config.EncoderConfig) error {
	if err := config.ValidateEncoderConfig(&cfg); err != nil {
		return fmt.Errorf("invalid encoder config: %w", err)
	}

	// Validate codec support
	supportedCodecs := e.GetSupportedCodecs()
	codecSupported := false
	for _, codec := range supportedCodecs {
		if codec == cfg.Codec {
			codecSupported = true
			break
		}
	}
	if !codecSupported {
		return fmt.Errorf("codec %s is not supported by x264 encoder", cfg.Codec)
	}

	e.config = cfg
	return nil
}

// GetElement returns the GStreamer element name and properties for x264
func (e *X264Encoder) GetElement() (string, map[string]interface{}, error) {
	var elementName string
	properties := make(map[string]interface{})

	// x264 only supports H.264
	switch e.config.Codec {
	case config.CodecH264:
		elementName = "x264enc"
	default:
		return "", nil, fmt.Errorf("unsupported codec: %s (x264 only supports H.264)", e.config.Codec)
	}

	// Set basic properties
	properties["bitrate"] = e.config.Bitrate
	properties["key-int-max"] = e.config.KeyframeInterval * 30 // Convert seconds to frames (assuming 30fps)

	// Set rate control mode
	switch e.config.RateControl {
	case config.RateControlCBR:
		properties["pass"] = "cbr"
		properties["bitrate"] = e.config.Bitrate
	case config.RateControlVBR:
		properties["pass"] = "pass1" // Single-pass VBR
		properties["bitrate"] = e.config.Bitrate
		if e.config.MaxBitrate > 0 {
			properties["vbv-buf-capacity"] = e.config.MaxBitrate
		}
	case config.RateControlCQP:
		properties["pass"] = "qual"
		properties["quantizer"] = e.config.Quality
	default:
		properties["pass"] = "pass1"
		properties["bitrate"] = e.config.Bitrate
	}

	// Set preset (speed vs quality trade-off)
	switch e.config.Preset {
	case config.PresetUltraFast:
		properties["speed-preset"] = "ultrafast"
	case config.PresetSuperFast:
		properties["speed-preset"] = "superfast"
	case config.PresetVeryFast:
		properties["speed-preset"] = "veryfast"
	case config.PresetFaster:
		properties["speed-preset"] = "faster"
	case config.PresetFast:
		properties["speed-preset"] = "fast"
	case config.PresetMedium:
		properties["speed-preset"] = "medium"
	case config.PresetSlow:
		properties["speed-preset"] = "slow"
	case config.PresetSlower:
		properties["speed-preset"] = "slower"
	case config.PresetVerySlow:
		properties["speed-preset"] = "veryslow"
	case config.PresetPlacebo:
		properties["speed-preset"] = "placebo"
	default:
		properties["speed-preset"] = "fast"
	}

	// Set profile
	switch e.config.Profile {
	case config.ProfileBaseline:
		properties["profile"] = "baseline"
	case config.ProfileMain:
		properties["profile"] = "main"
	case config.ProfileHigh:
		properties["profile"] = "high"
	case config.ProfileHigh10:
		properties["profile"] = "high-10"
	case config.ProfileHigh422:
		properties["profile"] = "high-4:2:2"
	case config.ProfileHigh444:
		properties["profile"] = "high-4:4:4"
	default:
		properties["profile"] = "main"
	}

	// Set tuning
	if e.config.Tune != "" {
		properties["tune"] = e.config.Tune
	} else if e.config.ZeroLatency {
		properties["tune"] = "zerolatency"
	}

	// Threading
	threads := e.config.Threads
	if threads <= 0 {
		threads = runtime.NumCPU() // Auto-detect based on CPU cores
	}
	properties["threads"] = threads

	// B-frames
	if e.config.ZeroLatency {
		properties["b-frames"] = 0
	} else {
		properties["b-frames"] = e.config.BFrames
	}

	// Reference frames
	if e.config.RefFrames > 0 {
		properties["ref"] = e.config.RefFrames
	}

	// Look-ahead
	if e.config.LookAhead > 0 && !e.config.ZeroLatency {
		properties["rc-lookahead"] = e.config.LookAhead
	}

	// Slice mode for low latency
	if e.config.SliceMode != "" {
		if e.config.SliceMode == "single" {
			properties["sliced-threads"] = false
		} else if e.config.SliceMode == "multi" {
			properties["sliced-threads"] = true
		}
	}

	// Additional x264 specific options
	options := make([]string, 0)

	// Intra refresh for low latency
	if e.config.ZeroLatency {
		options = append(options, "intra-refresh=1")
		options = append(options, "no-scenecut=1")
	}

	// Bitrate limits
	if e.config.MaxBitrate > 0 && e.config.RateControl != config.RateControlCQP {
		vbvMaxrate := e.config.MaxBitrate
		vbvBufsize := e.config.MaxBitrate // Use same value for buffer size
		options = append(options, fmt.Sprintf("vbv-maxrate=%d", vbvMaxrate))
		options = append(options, fmt.Sprintf("vbv-bufsize=%d", vbvBufsize))
	}

	// Custom options from config
	for key, value := range e.config.CustomOptions {
		if strValue, ok := value.(string); ok {
			options = append(options, fmt.Sprintf("%s=%s", key, strValue))
		} else {
			options = append(options, fmt.Sprintf("%s=%v", key, value))
		}
	}

	// Set option-string if we have custom options
	if len(options) > 0 {
		properties["option-string"] = strings.Join(options, ":")
	}

	// Custom GStreamer options
	for key, value := range e.config.GStreamerOptions {
		properties[key] = value
	}

	return elementName, properties, nil
}

// UpdateBitrate dynamically updates the encoder bitrate
func (e *X264Encoder) UpdateBitrate(bitrate int) error {
	if bitrate <= 0 || bitrate > 50000 {
		return fmt.Errorf("invalid bitrate: %d (must be between 1 and 50000 kbps)", bitrate)
	}

	e.mutex.Lock()
	defer e.mutex.Unlock()

	// Update configuration
	e.config.Bitrate = bitrate

	// In real implementation, this would update the GStreamer element property
	// For now, we'll simulate the update
	if e.element != nil {
		// gst_element_set_property(e.element, "bitrate", bitrate)
	}

	return nil
}

// ForceKeyframe forces the encoder to generate a keyframe
func (e *X264Encoder) ForceKeyframe() error {
	// In real implementation, this would send a force-keyframe event to the encoder
	// For now, we'll simulate the keyframe generation
	if e.element != nil {
		// Send force-keyframe event
		// gst_element_send_event(e.element, gst_video_event_new_downstream_force_key_unit(...))
	}

	// Update statistics
	e.mutex.Lock()
	e.stats.KeyframesEncoded++
	e.mutex.Unlock()

	return nil
}

// IsHardwareAccelerated returns false since x264 is software-based
func (e *X264Encoder) IsHardwareAccelerated() bool {
	return false
}

// GetSupportedCodecs returns the codecs supported by x264
func (e *X264Encoder) GetSupportedCodecs() []config.CodecType {
	return []config.CodecType{
		config.CodecH264, // x264 only supports H.264
	}
}

// Cleanup cleans up x264 encoder resources
func (e *X264Encoder) Cleanup() error {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	// In real implementation, this would clean up GStreamer resources
	if e.element != nil {
		// gst_object_unref(e.element)
		e.element = nil
	}

	return nil
}

// GetOptimalX264Config returns optimal x264 configuration based on system capabilities
func GetOptimalX264Config() config.EncoderConfig {
	cfg := config.DefaultEncoderConfig()
	cfg.Type = config.EncoderTypeX264
	cfg.UseHardware = false

	// Adjust settings based on CPU capabilities
	cpuInfo := detectCPUCapabilities()

	// Set threads based on CPU cores
	cfg.Threads = cpuInfo.Cores
	if cfg.Threads > 16 {
		cfg.Threads = 16 // Cap at 16 threads for x264
	}

	// Adjust preset based on CPU performance
	if cpuInfo.Cores >= 8 {
		cfg.Preset = config.PresetMedium // Better quality on powerful CPUs
	} else if cpuInfo.Cores >= 4 {
		cfg.Preset = config.PresetFast
	} else {
		cfg.Preset = config.PresetVeryFast // Faster encoding on weaker CPUs
	}

	// Adjust settings for low latency if needed
	if cfg.ZeroLatency {
		cfg.Preset = config.PresetVeryFast // Prioritize speed for low latency
		cfg.BFrames = 0
		cfg.LookAhead = 0
		cfg.Tune = "zerolatency"
	}

	return cfg
}

// CPUInfo represents CPU capabilities
type CPUInfo struct {
	Cores     int
	Threads   int
	Model     string
	Frequency float64
	HasAVX    bool
	HasAVX2   bool
	HasSSE42  bool
}

// detectCPUCapabilities detects CPU capabilities (simplified implementation)
func detectCPUCapabilities() CPUInfo {
	info := CPUInfo{
		Cores:   runtime.NumCPU(),
		Threads: runtime.NumCPU(),
		Model:   "Unknown CPU",
	}

	// Try to get more detailed CPU info
	if cpuInfo := getCPUInfo(); cpuInfo != "" {
		info.Model = extractCPUModel(cpuInfo)
		info.HasAVX = strings.Contains(cpuInfo, "avx")
		info.HasAVX2 = strings.Contains(cpuInfo, "avx2")
		info.HasSSE42 = strings.Contains(cpuInfo, "sse4_2")
	}

	return info
}

// getCPUInfo gets CPU information from /proc/cpuinfo
func getCPUInfo() string {
	// In real implementation, this would read /proc/cpuinfo
	// For now, return empty string
	return ""
}

// extractCPUModel extracts CPU model from cpuinfo string
func extractCPUModel(cpuInfo string) string {
	lines := strings.Split(cpuInfo, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "model name") {
			parts := strings.Split(line, ":")
			if len(parts) > 1 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return "Unknown CPU"
}

// GetX264PresetForCPU returns optimal x264 preset based on CPU capabilities
func GetX264PresetForCPU(cpuInfo CPUInfo) config.EncoderPreset {
	// High-end CPUs (8+ cores)
	if cpuInfo.Cores >= 8 {
		if cpuInfo.HasAVX2 {
			return config.PresetMedium // Can handle better quality
		}
		return config.PresetFast
	}

	// Mid-range CPUs (4-7 cores)
	if cpuInfo.Cores >= 4 {
		if cpuInfo.HasAVX {
			return config.PresetFast
		}
		return config.PresetVeryFast
	}

	// Low-end CPUs (1-3 cores)
	return config.PresetVeryFast
}

// GetX264ThreadsForCPU returns optimal thread count for x264 based on CPU
func GetX264ThreadsForCPU(cpuInfo CPUInfo) int {
	// x264 generally benefits from 1.5x the number of cores, but cap at 16
	threads := int(float64(cpuInfo.Cores) * 1.5)
	if threads > 16 {
		threads = 16
	}
	if threads < 1 {
		threads = 1
	}
	return threads
}

// EstimateX264Performance estimates encoding performance for given settings
func EstimateX264Performance(cfg config.EncoderConfig, cpuInfo CPUInfo) X264Performance {
	perf := X264Performance{
		EstimatedFPS: 30.0, // Default assumption
		CPUUsage:     50.0, // Default assumption
		Quality:      "medium",
	}

	// Adjust based on preset
	switch cfg.Preset {
	case config.PresetUltraFast, config.PresetSuperFast:
		perf.EstimatedFPS = 60.0
		perf.CPUUsage = 30.0
		perf.Quality = "low"
	case config.PresetVeryFast, config.PresetFaster:
		perf.EstimatedFPS = 45.0
		perf.CPUUsage = 40.0
		perf.Quality = "medium-low"
	case config.PresetFast:
		perf.EstimatedFPS = 35.0
		perf.CPUUsage = 50.0
		perf.Quality = "medium"
	case config.PresetMedium:
		perf.EstimatedFPS = 25.0
		perf.CPUUsage = 65.0
		perf.Quality = "medium-high"
	case config.PresetSlow, config.PresetSlower:
		perf.EstimatedFPS = 15.0
		perf.CPUUsage = 80.0
		perf.Quality = "high"
	case config.PresetVerySlow, config.PresetPlacebo:
		perf.EstimatedFPS = 10.0
		perf.CPUUsage = 95.0
		perf.Quality = "very-high"
	}

	// Adjust based on CPU capabilities
	cpuMultiplier := float64(cpuInfo.Cores) / 4.0 // Normalize to 4 cores
	if cpuMultiplier > 2.0 {
		cpuMultiplier = 2.0 // Cap the benefit
	}

	perf.EstimatedFPS *= cpuMultiplier
	perf.CPUUsage /= cpuMultiplier

	// Adjust for resolution (assuming 1080p as baseline)
	// Higher resolutions reduce FPS and increase CPU usage

	return perf
}

// X264Performance represents estimated x264 encoding performance
type X264Performance struct {
	EstimatedFPS float64
	CPUUsage     float64
	Quality      string
}
