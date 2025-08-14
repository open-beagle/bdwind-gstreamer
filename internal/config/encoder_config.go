package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v2"
)

// EncoderType represents the type of encoder
type EncoderType string

const (
	EncoderTypeNVENC EncoderType = "nvenc"
	EncoderTypeVAAPI EncoderType = "vaapi"
	EncoderTypeX264  EncoderType = "x264"
	EncoderTypeVP8   EncoderType = "vp8"
	EncoderTypeVP9   EncoderType = "vp9"
	EncoderTypeAuto  EncoderType = "auto"
)

// CodecType represents the codec type
type CodecType string

const (
	CodecH264 CodecType = "h264"
	CodecVP8  CodecType = "vp8"
	CodecVP9  CodecType = "vp9"
)

// RateControlMode represents the rate control mode
type RateControlMode string

const (
	RateControlCBR RateControlMode = "cbr" // Constant Bitrate
	RateControlVBR RateControlMode = "vbr" // Variable Bitrate
	RateControlCQP RateControlMode = "cqp" // Constant Quantization Parameter
)

// EncoderPreset represents the encoder preset
type EncoderPreset string

const (
	PresetUltraFast EncoderPreset = "ultrafast"
	PresetSuperFast EncoderPreset = "superfast"
	PresetVeryFast  EncoderPreset = "veryfast"
	PresetFaster    EncoderPreset = "faster"
	PresetFast      EncoderPreset = "fast"
	PresetMedium    EncoderPreset = "medium"
	PresetSlow      EncoderPreset = "slow"
	PresetSlower    EncoderPreset = "slower"
	PresetVerySlow  EncoderPreset = "veryslow"
	PresetPlacebo   EncoderPreset = "placebo"
)

// EncoderProfile represents the encoder profile
type EncoderProfile string

const (
	ProfileBaseline EncoderProfile = "baseline"
	ProfileMain     EncoderProfile = "main"
	ProfileHigh     EncoderProfile = "high"
	ProfileHigh10   EncoderProfile = "high-10"
	ProfileHigh422  EncoderProfile = "high-4:2:2"
	ProfileHigh444  EncoderProfile = "high-4:4:4"
)

// EncoderConfig contains configuration for video encoding
type EncoderConfig struct {
	// Basic encoder settings
	Type             EncoderType `yaml:"type" json:"type"`                           // Encoder type (nvenc, vaapi, x264, auto)
	Codec            CodecType   `yaml:"codec" json:"codec"`                         // Codec type (h264, vp8, vp9)
	Bitrate          int         `yaml:"bitrate" json:"bitrate"`                     // Bitrate in kbps
	MaxBitrate       int         `yaml:"max_bitrate" json:"max_bitrate"`             // Maximum bitrate in kbps
	MinBitrate       int         `yaml:"min_bitrate" json:"min_bitrate"`             // Minimum bitrate in kbps
	KeyframeInterval int         `yaml:"keyframe_interval" json:"keyframe_interval"` // Keyframe interval in seconds

	// Hardware acceleration settings
	UseHardware    bool `yaml:"use_hardware" json:"use_hardware"`         // Enable hardware acceleration
	HardwareDevice int  `yaml:"hardware_device" json:"hardware_device"`   // Hardware device index
	FallbackToSoft bool `yaml:"fallback_to_soft" json:"fallback_to_soft"` // Fallback to software encoding if hardware fails

	// Quality settings
	Preset      EncoderPreset   `yaml:"preset" json:"preset"`             // Encoder preset
	Profile     EncoderProfile  `yaml:"profile" json:"profile"`           // Encoder profile
	RateControl RateControlMode `yaml:"rate_control" json:"rate_control"` // Rate control mode
	Quality     int             `yaml:"quality" json:"quality"`           // Quality level (0-51 for x264, 0-63 for VP8/VP9)

	// Performance settings
	Threads   int `yaml:"threads" json:"threads"`       // Number of encoding threads
	LookAhead int `yaml:"look_ahead" json:"look_ahead"` // Look-ahead frames
	BFrames   int `yaml:"b_frames" json:"b_frames"`     // Number of B-frames
	RefFrames int `yaml:"ref_frames" json:"ref_frames"` // Number of reference frames

	// Latency settings
	ZeroLatency bool   `yaml:"zero_latency" json:"zero_latency"` // Enable zero-latency mode
	Tune        string `yaml:"tune" json:"tune"`                 // Tuning preset (zerolatency, film, animation, etc.)
	SliceMode   string `yaml:"slice_mode" json:"slice_mode"`     // Slice mode for low latency

	// Adaptive settings
	AdaptiveBitrate bool `yaml:"adaptive_bitrate" json:"adaptive_bitrate"` // Enable adaptive bitrate
	BitrateWindow   int  `yaml:"bitrate_window" json:"bitrate_window"`     // Bitrate adaptation window in seconds

	// Advanced settings
	CustomOptions    map[string]interface{} `yaml:"custom_options" json:"custom_options"`       // Custom encoder options
	GStreamerOptions map[string]interface{} `yaml:"gstreamer_options" json:"gstreamer_options"` // GStreamer-specific options
}

// DefaultEncoderConfig returns a configuration with default values
func DefaultEncoderConfig() EncoderConfig {
	return EncoderConfig{
		Type:             EncoderTypeAuto,
		Codec:            CodecH264,
		Bitrate:          2000,
		MaxBitrate:       4000,
		MinBitrate:       500,
		KeyframeInterval: 2,
		UseHardware:      true,
		HardwareDevice:   0,
		FallbackToSoft:   true,
		Preset:           PresetFast,
		Profile:          ProfileMain,
		RateControl:      RateControlVBR,
		Quality:          23,
		Threads:          0, // Auto-detect
		LookAhead:        0,
		BFrames:          0,
		RefFrames:        1,
		ZeroLatency:      true,
		Tune:             "zerolatency",
		SliceMode:        "single",
		AdaptiveBitrate:  true,
		BitrateWindow:    5,
		CustomOptions:    make(map[string]interface{}),
		GStreamerOptions: make(map[string]interface{}),
	}
}

// ValidateEncoderConfig validates the encoder configuration
func ValidateEncoderConfig(config *EncoderConfig) error {
	// Validate encoder type
	validTypes := []EncoderType{EncoderTypeNVENC, EncoderTypeVAAPI, EncoderTypeX264, EncoderTypeVP8, EncoderTypeVP9, EncoderTypeAuto}
	if !isValidEncoderType(config.Type, validTypes) {
		return fmt.Errorf("invalid encoder type: %s", config.Type)
	}

	// Validate codec type
	validCodecs := []CodecType{CodecH264, CodecVP8, CodecVP9}
	if !isValidCodecType(config.Codec, validCodecs) {
		return fmt.Errorf("invalid codec type: %s", config.Codec)
	}

	// Validate bitrate settings
	if config.Bitrate <= 0 || config.Bitrate > 50000 {
		return fmt.Errorf("invalid bitrate: %d (must be between 1 and 50000 kbps)", config.Bitrate)
	}
	if config.MaxBitrate > 0 && config.MaxBitrate < config.Bitrate {
		return fmt.Errorf("max bitrate (%d) must be greater than or equal to bitrate (%d)", config.MaxBitrate, config.Bitrate)
	}
	if config.MinBitrate > 0 && config.MinBitrate > config.Bitrate {
		return fmt.Errorf("min bitrate (%d) must be less than or equal to bitrate (%d)", config.MinBitrate, config.Bitrate)
	}

	// Validate keyframe interval
	if config.KeyframeInterval <= 0 || config.KeyframeInterval > 60 {
		return fmt.Errorf("invalid keyframe interval: %d (must be between 1 and 60 seconds)", config.KeyframeInterval)
	}

	// Validate hardware device
	if config.HardwareDevice < 0 || config.HardwareDevice > 15 {
		return fmt.Errorf("invalid hardware device: %d (must be between 0 and 15)", config.HardwareDevice)
	}

	// Validate quality
	maxQuality := 51
	if config.Codec == CodecVP8 || config.Codec == CodecVP9 {
		maxQuality = 63
	}
	if config.Quality < 0 || config.Quality > maxQuality {
		return fmt.Errorf("invalid quality: %d (must be between 0 and %d for %s)", config.Quality, maxQuality, config.Codec)
	}

	// Validate threads
	if config.Threads < 0 || config.Threads > 64 {
		return fmt.Errorf("invalid threads: %d (must be between 0 and 64)", config.Threads)
	}

	// Validate look-ahead
	if config.LookAhead < 0 || config.LookAhead > 250 {
		return fmt.Errorf("invalid look-ahead: %d (must be between 0 and 250)", config.LookAhead)
	}

	// Validate B-frames
	if config.BFrames < 0 || config.BFrames > 16 {
		return fmt.Errorf("invalid B-frames: %d (must be between 0 and 16)", config.BFrames)
	}

	// Validate reference frames
	if config.RefFrames < 1 || config.RefFrames > 16 {
		return fmt.Errorf("invalid reference frames: %d (must be between 1 and 16)", config.RefFrames)
	}

	// Validate bitrate window
	if config.BitrateWindow <= 0 || config.BitrateWindow > 60 {
		return fmt.Errorf("invalid bitrate window: %d (must be between 1 and 60 seconds)", config.BitrateWindow)
	}

	return nil
}

// LoadEncoderConfigFromEnv loads encoder configuration from environment variables
func LoadEncoderConfigFromEnv() EncoderConfig {
	config := DefaultEncoderConfig()

	// Load from environment variables with BDWIND_ENCODER_ prefix
	if encoderType := os.Getenv("BDWIND_ENCODER_TYPE"); encoderType != "" {
		config.Type = EncoderType(encoderType)
	}

	if codec := os.Getenv("BDWIND_ENCODER_CODEC"); codec != "" {
		config.Codec = CodecType(codec)
	}

	if bitrateStr := os.Getenv("BDWIND_ENCODER_BITRATE"); bitrateStr != "" {
		if bitrate, err := strconv.Atoi(bitrateStr); err == nil {
			config.Bitrate = bitrate
		}
	}

	if maxBitrateStr := os.Getenv("BDWIND_ENCODER_MAX_BITRATE"); maxBitrateStr != "" {
		if maxBitrate, err := strconv.Atoi(maxBitrateStr); err == nil {
			config.MaxBitrate = maxBitrate
		}
	}

	if minBitrateStr := os.Getenv("BDWIND_ENCODER_MIN_BITRATE"); minBitrateStr != "" {
		if minBitrate, err := strconv.Atoi(minBitrateStr); err == nil {
			config.MinBitrate = minBitrate
		}
	}

	if keyframeStr := os.Getenv("BDWIND_ENCODER_KEYFRAME_INTERVAL"); keyframeStr != "" {
		if keyframe, err := strconv.Atoi(keyframeStr); err == nil {
			config.KeyframeInterval = keyframe
		}
	}

	if useHardwareStr := os.Getenv("BDWIND_ENCODER_USE_HARDWARE"); useHardwareStr != "" {
		if useHardware, err := strconv.ParseBool(useHardwareStr); err == nil {
			config.UseHardware = useHardware
		}
	}

	if deviceStr := os.Getenv("BDWIND_ENCODER_HARDWARE_DEVICE"); deviceStr != "" {
		if device, err := strconv.Atoi(deviceStr); err == nil {
			config.HardwareDevice = device
		}
	}

	if fallbackStr := os.Getenv("BDWIND_ENCODER_FALLBACK_TO_SOFT"); fallbackStr != "" {
		if fallback, err := strconv.ParseBool(fallbackStr); err == nil {
			config.FallbackToSoft = fallback
		}
	}

	if preset := os.Getenv("BDWIND_ENCODER_PRESET"); preset != "" {
		config.Preset = EncoderPreset(preset)
	}

	if profile := os.Getenv("BDWIND_ENCODER_PROFILE"); profile != "" {
		config.Profile = EncoderProfile(profile)
	}

	if rateControl := os.Getenv("BDWIND_ENCODER_RATE_CONTROL"); rateControl != "" {
		config.RateControl = RateControlMode(rateControl)
	}

	if qualityStr := os.Getenv("BDWIND_ENCODER_QUALITY"); qualityStr != "" {
		if quality, err := strconv.Atoi(qualityStr); err == nil {
			config.Quality = quality
		}
	}

	if threadsStr := os.Getenv("BDWIND_ENCODER_THREADS"); threadsStr != "" {
		if threads, err := strconv.Atoi(threadsStr); err == nil {
			config.Threads = threads
		}
	}

	if lookAheadStr := os.Getenv("BDWIND_ENCODER_LOOK_AHEAD"); lookAheadStr != "" {
		if lookAhead, err := strconv.Atoi(lookAheadStr); err == nil {
			config.LookAhead = lookAhead
		}
	}

	if bFramesStr := os.Getenv("BDWIND_ENCODER_B_FRAMES"); bFramesStr != "" {
		if bFrames, err := strconv.Atoi(bFramesStr); err == nil {
			config.BFrames = bFrames
		}
	}

	if refFramesStr := os.Getenv("BDWIND_ENCODER_REF_FRAMES"); refFramesStr != "" {
		if refFrames, err := strconv.Atoi(refFramesStr); err == nil {
			config.RefFrames = refFrames
		}
	}

	if zeroLatencyStr := os.Getenv("BDWIND_ENCODER_ZERO_LATENCY"); zeroLatencyStr != "" {
		if zeroLatency, err := strconv.ParseBool(zeroLatencyStr); err == nil {
			config.ZeroLatency = zeroLatency
		}
	}

	if tune := os.Getenv("BDWIND_ENCODER_TUNE"); tune != "" {
		config.Tune = tune
	}

	if sliceMode := os.Getenv("BDWIND_ENCODER_SLICE_MODE"); sliceMode != "" {
		config.SliceMode = sliceMode
	}

	if adaptiveBitrateStr := os.Getenv("BDWIND_ENCODER_ADAPTIVE_BITRATE"); adaptiveBitrateStr != "" {
		if adaptiveBitrate, err := strconv.ParseBool(adaptiveBitrateStr); err == nil {
			config.AdaptiveBitrate = adaptiveBitrate
		}
	}

	if bitrateWindowStr := os.Getenv("BDWIND_ENCODER_BITRATE_WINDOW"); bitrateWindowStr != "" {
		if bitrateWindow, err := strconv.Atoi(bitrateWindowStr); err == nil {
			config.BitrateWindow = bitrateWindow
		}
	}

	return config
}

// LoadEncoderConfigFromFile loads encoder configuration from a YAML file
func LoadEncoderConfigFromFile(filename string) (EncoderConfig, error) {
	config := DefaultEncoderConfig()

	data, err := os.ReadFile(filename)
	if err != nil {
		return config, fmt.Errorf("failed to read encoder config file: %w", err)
	}

	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return config, fmt.Errorf("failed to parse encoder config file: %w", err)
	}

	return config, nil
}

// SaveEncoderConfigToFile saves encoder configuration to a YAML file
func SaveEncoderConfigToFile(config EncoderConfig, filename string) error {
	data, err := yaml.Marshal(&config)
	if err != nil {
		return fmt.Errorf("failed to marshal encoder config: %w", err)
	}

	err = os.WriteFile(filename, data, 0644)
	if err != nil {
		return fmt.Errorf("failed to write encoder config file: %w", err)
	}

	return nil
}

// GetOptimalEncoderConfig returns an optimal encoder configuration based on system capabilities
func GetOptimalEncoderConfig() EncoderConfig {
	config := DefaultEncoderConfig()

	// Try to detect optimal settings based on environment
	// This is a simplified implementation - in practice, we would probe hardware capabilities

	// Check for NVIDIA GPU
	if _, err := os.Stat("/dev/nvidia0"); err == nil {
		config.Type = EncoderTypeNVENC
		config.UseHardware = true
	} else if _, err := os.Stat("/dev/dri/renderD128"); err == nil {
		// Check for Intel/AMD GPU with VAAPI support
		config.Type = EncoderTypeVAAPI
		config.UseHardware = true
	} else {
		// Fallback to software encoding
		config.Type = EncoderTypeX264
		config.UseHardware = false
	}

	return config
}

// Helper functions for validation
func isValidEncoderType(t EncoderType, valid []EncoderType) bool {
	for _, v := range valid {
		if t == v {
			return true
		}
	}
	return false
}

func isValidCodecType(c CodecType, valid []CodecType) bool {
	for _, v := range valid {
		if c == v {
			return true
		}
	}
	return false
}

// GetEncoderTypeFromString converts string to EncoderType
func GetEncoderTypeFromString(s string) EncoderType {
	switch strings.ToLower(s) {
	case "nvenc":
		return EncoderTypeNVENC
	case "vaapi":
		return EncoderTypeVAAPI
	case "x264":
		return EncoderTypeX264
	case "vp8":
		return EncoderTypeVP8
	case "vp9":
		return EncoderTypeVP9
	case "auto":
		return EncoderTypeAuto
	default:
		return EncoderTypeAuto
	}
}

// GetCodecTypeFromString converts string to CodecType
func GetCodecTypeFromString(s string) CodecType {
	switch strings.ToLower(s) {
	case "h264":
		return CodecH264
	case "vp8":
		return CodecVP8
	case "vp9":
		return CodecVP9
	default:
		return CodecH264
	}
}
