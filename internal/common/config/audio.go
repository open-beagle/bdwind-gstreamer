package config

import (
	"fmt"
)

// AudioConfig audio processing configuration
type AudioConfig struct {
	// Enable audio processing
	Enabled bool `yaml:"enabled" json:"enabled"`

	// Audio source type (auto, pulseaudio, alsa, test)
	Source string `yaml:"source" json:"source"`

	// Audio device (empty for default)
	Device string `yaml:"device" json:"device"`

	// Audio codec (opus, pcm)
	Codec string `yaml:"codec" json:"codec"`

	// Sample rate in Hz
	SampleRate int `yaml:"sample_rate" json:"sample_rate"`

	// Number of channels
	Channels int `yaml:"channels" json:"channels"`

	// Bitrate in bps (for compressed codecs)
	Bitrate int `yaml:"bitrate" json:"bitrate"`

	// Buffer size for audio processing
	BufferSize int `yaml:"buffer_size" json:"buffer_size"`

	// Audio quality preset (low, medium, high)
	Quality string `yaml:"quality" json:"quality"`

	// Enable noise suppression
	NoiseSuppress bool `yaml:"noise_suppress" json:"noise_suppress"`

	// Enable echo cancellation
	EchoCancellation bool `yaml:"echo_cancellation" json:"echo_cancellation"`

	// Enable automatic gain control
	AutoGainControl bool `yaml:"auto_gain_control" json:"auto_gain_control"`

	// Audio level threshold for silence detection
	SilenceThreshold float64 `yaml:"silence_threshold" json:"silence_threshold"`
}

// DefaultAudioConfig returns default audio configuration
func DefaultAudioConfig() AudioConfig {
	return AudioConfig{
		Enabled:          true,
		Source:           "auto",
		Device:           "",
		Codec:            "opus",
		SampleRate:       48000,
		Channels:         2,
		Bitrate:          128000, // 128 kbps
		BufferSize:       100,
		Quality:          "medium",
		NoiseSuppress:    false,
		EchoCancellation: false,
		AutoGainControl:  false,
		SilenceThreshold: 0.01,
	}
}

// ValidateAudioConfig validates audio configuration
func ValidateAudioConfig(cfg *AudioConfig) error {
	if cfg == nil {
		return fmt.Errorf("audio config cannot be nil")
	}

	// Validate source type
	validSources := []string{"auto", "pulseaudio", "alsa", "test"}
	if !isValidOption(cfg.Source, validSources) {
		return fmt.Errorf("invalid audio source '%s', must be one of: %v", cfg.Source, validSources)
	}

	// Validate codec
	validCodecs := []string{"opus", "pcm"}
	if !isValidOption(cfg.Codec, validCodecs) {
		return fmt.Errorf("invalid audio codec '%s', must be one of: %v", cfg.Codec, validCodecs)
	}

	// Validate sample rate
	validSampleRates := []int{8000, 16000, 22050, 44100, 48000, 96000}
	if !isValidSampleRate(cfg.SampleRate, validSampleRates) {
		return fmt.Errorf("invalid sample rate %d, must be one of: %v", cfg.SampleRate, validSampleRates)
	}

	// Validate channels
	if cfg.Channels < 1 || cfg.Channels > 8 {
		return fmt.Errorf("invalid channel count %d, must be between 1 and 8", cfg.Channels)
	}

	// Validate bitrate
	if cfg.Bitrate < 8000 || cfg.Bitrate > 512000 {
		return fmt.Errorf("invalid bitrate %d, must be between 8000 and 512000", cfg.Bitrate)
	}

	// Validate buffer size
	if cfg.BufferSize < 1 || cfg.BufferSize > 1000 {
		return fmt.Errorf("invalid buffer size %d, must be between 1 and 1000", cfg.BufferSize)
	}

	// Validate quality preset
	validQualities := []string{"low", "medium", "high"}
	if !isValidOption(cfg.Quality, validQualities) {
		return fmt.Errorf("invalid quality preset '%s', must be one of: %v", cfg.Quality, validQualities)
	}

	// Validate silence threshold
	if cfg.SilenceThreshold < 0.0 || cfg.SilenceThreshold > 1.0 {
		return fmt.Errorf("invalid silence threshold %f, must be between 0.0 and 1.0", cfg.SilenceThreshold)
	}

	return nil
}

// isValidOption checks if option is in valid options list
func isValidOption(option string, validOptions []string) bool {
	for _, validOption := range validOptions {
		if option == validOption {
			return true
		}
	}
	return false
}

// isValidSampleRate checks if sample rate is valid
func isValidSampleRate(rate int, validRates []int) bool {
	for _, validRate := range validRates {
		if rate == validRate {
			return true
		}
	}
	return false
}

// GetOptimalAudioSettings returns optimal audio settings based on use case
func (cfg *AudioConfig) GetOptimalAudioSettings(useCase string) AudioConfig {
	optimal := *cfg

	switch useCase {
	case "voice":
		optimal.SampleRate = 16000
		optimal.Channels = 1
		optimal.Bitrate = 32000
		optimal.NoiseSuppress = true
		optimal.EchoCancellation = true
		optimal.AutoGainControl = true
	case "music":
		optimal.SampleRate = 48000
		optimal.Channels = 2
		optimal.Bitrate = 256000
		optimal.NoiseSuppress = false
		optimal.EchoCancellation = false
		optimal.AutoGainControl = false
	case "gaming":
		optimal.SampleRate = 44100
		optimal.Channels = 2
		optimal.Bitrate = 128000
		optimal.NoiseSuppress = true
		optimal.EchoCancellation = true
		optimal.AutoGainControl = false
	default: // general
		// Keep current settings
	}

	return optimal
}

// GetCodecSettings returns codec-specific settings
func (cfg *AudioConfig) GetCodecSettings() map[string]interface{} {
	settings := make(map[string]interface{})

	switch cfg.Codec {
	case "opus":
		settings["complexity"] = 5
		settings["inband_fec"] = true
		settings["packet_loss_percentage"] = 1
		settings["dtx"] = false
		settings["audio_type"] = 2048 // generic audio
	case "pcm":
		settings["format"] = "S16LE"
		settings["layout"] = "interleaved"
	}

	return settings
}

// GetBufferSettings returns buffer-related settings
func (cfg *AudioConfig) GetBufferSettings() map[string]interface{} {
	settings := make(map[string]interface{})

	// Calculate buffer duration based on sample rate
	bufferDurationMs := (cfg.BufferSize * 1000) / (cfg.SampleRate / 1000)

	settings["buffer_duration_ms"] = bufferDurationMs
	settings["max_latency_ms"] = bufferDurationMs * 2
	settings["min_latency_ms"] = bufferDurationMs / 2

	return settings
}
