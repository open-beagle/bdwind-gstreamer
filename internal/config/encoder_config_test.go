package config

import (
	"os"
	"testing"
)

func TestDefaultEncoderConfig(t *testing.T) {
	config := DefaultEncoderConfig()

	// Test default values
	if config.Type != EncoderTypeAuto {
		t.Errorf("Expected encoder type to be %s, got %s", EncoderTypeAuto, config.Type)
	}
	if config.Codec != CodecH264 {
		t.Errorf("Expected codec to be %s, got %s", CodecH264, config.Codec)
	}
	if config.Bitrate != 2000 {
		t.Errorf("Expected bitrate to be 2000, got %d", config.Bitrate)
	}
	if config.KeyframeInterval != 2 {
		t.Errorf("Expected keyframe interval to be 2, got %d", config.KeyframeInterval)
	}
	if !config.UseHardware {
		t.Error("Expected hardware acceleration to be enabled by default")
	}
	if !config.FallbackToSoft {
		t.Error("Expected fallback to software to be enabled by default")
	}
	if config.Preset != PresetFast {
		t.Errorf("Expected preset to be %s, got %s", PresetFast, config.Preset)
	}
	if config.Profile != ProfileMain {
		t.Errorf("Expected profile to be %s, got %s", ProfileMain, config.Profile)
	}
	if config.RateControl != RateControlVBR {
		t.Errorf("Expected rate control to be %s, got %s", RateControlVBR, config.RateControl)
	}
	if config.Quality != 23 {
		t.Errorf("Expected quality to be 23, got %d", config.Quality)
	}
	if !config.ZeroLatency {
		t.Error("Expected zero latency to be enabled by default")
	}
	if config.Tune != "zerolatency" {
		t.Errorf("Expected tune to be 'zerolatency', got %s", config.Tune)
	}
	if !config.AdaptiveBitrate {
		t.Error("Expected adaptive bitrate to be enabled by default")
	}
}

func TestValidateEncoderConfig(t *testing.T) {
	tests := []struct {
		name        string
		config      EncoderConfig
		expectError bool
	}{
		{
			name:        "Valid default config",
			config:      DefaultEncoderConfig(),
			expectError: false,
		},
		{
			name: "Invalid encoder type",
			config: EncoderConfig{
				Type:             "invalid",
				Codec:            CodecH264,
				Bitrate:          2000,
				KeyframeInterval: 2,
				Quality:          23,
				RefFrames:        1,
				BitrateWindow:    5,
			},
			expectError: true,
		},
		{
			name: "Invalid codec type",
			config: EncoderConfig{
				Type:             EncoderTypeX264,
				Codec:            "invalid",
				Bitrate:          2000,
				KeyframeInterval: 2,
				Quality:          23,
				RefFrames:        1,
				BitrateWindow:    5,
			},
			expectError: true,
		},
		{
			name: "Invalid bitrate - too low",
			config: EncoderConfig{
				Type:             EncoderTypeX264,
				Codec:            CodecH264,
				Bitrate:          0,
				KeyframeInterval: 2,
				Quality:          23,
				RefFrames:        1,
				BitrateWindow:    5,
			},
			expectError: true,
		},
		{
			name: "Invalid bitrate - too high",
			config: EncoderConfig{
				Type:             EncoderTypeX264,
				Codec:            CodecH264,
				Bitrate:          60000,
				KeyframeInterval: 2,
				Quality:          23,
				RefFrames:        1,
				BitrateWindow:    5,
			},
			expectError: true,
		},
		{
			name: "Invalid max bitrate",
			config: EncoderConfig{
				Type:             EncoderTypeX264,
				Codec:            CodecH264,
				Bitrate:          2000,
				MaxBitrate:       1000,
				KeyframeInterval: 2,
				Quality:          23,
				RefFrames:        1,
				BitrateWindow:    5,
			},
			expectError: true,
		},
		{
			name: "Invalid min bitrate",
			config: EncoderConfig{
				Type:             EncoderTypeX264,
				Codec:            CodecH264,
				Bitrate:          2000,
				MinBitrate:       3000,
				KeyframeInterval: 2,
				Quality:          23,
				RefFrames:        1,
				BitrateWindow:    5,
			},
			expectError: true,
		},
		{
			name: "Invalid keyframe interval",
			config: EncoderConfig{
				Type:             EncoderTypeX264,
				Codec:            CodecH264,
				Bitrate:          2000,
				KeyframeInterval: 0,
				Quality:          23,
				RefFrames:        1,
				BitrateWindow:    5,
			},
			expectError: true,
		},
		{
			name: "Invalid quality for H264",
			config: EncoderConfig{
				Type:             EncoderTypeX264,
				Codec:            CodecH264,
				Bitrate:          2000,
				KeyframeInterval: 2,
				Quality:          60,
				RefFrames:        1,
				BitrateWindow:    5,
			},
			expectError: true,
		},
		{
			name: "Valid quality for VP8",
			config: EncoderConfig{
				Type:             EncoderTypeVP8,
				Codec:            CodecVP8,
				Bitrate:          2000,
				KeyframeInterval: 2,
				Quality:          60,
				RefFrames:        1,
				BitrateWindow:    5,
			},
			expectError: false,
		},
		{
			name: "Invalid threads",
			config: EncoderConfig{
				Type:             EncoderTypeX264,
				Codec:            CodecH264,
				Bitrate:          2000,
				KeyframeInterval: 2,
				Quality:          23,
				Threads:          100,
				RefFrames:        1,
				BitrateWindow:    5,
			},
			expectError: true,
		},
		{
			name: "Invalid reference frames",
			config: EncoderConfig{
				Type:             EncoderTypeX264,
				Codec:            CodecH264,
				Bitrate:          2000,
				KeyframeInterval: 2,
				Quality:          23,
				RefFrames:        0,
				BitrateWindow:    5,
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateEncoderConfig(&tt.config)
			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
		})
	}
}

func TestLoadEncoderConfigFromEnv(t *testing.T) {
	// Save original environment
	originalEnv := make(map[string]string)
	envVars := []string{
		"BDWIND_ENCODER_TYPE",
		"BDWIND_ENCODER_CODEC",
		"BDWIND_ENCODER_BITRATE",
		"BDWIND_ENCODER_MAX_BITRATE",
		"BDWIND_ENCODER_MIN_BITRATE",
		"BDWIND_ENCODER_KEYFRAME_INTERVAL",
		"BDWIND_ENCODER_USE_HARDWARE",
		"BDWIND_ENCODER_HARDWARE_DEVICE",
		"BDWIND_ENCODER_FALLBACK_TO_SOFT",
		"BDWIND_ENCODER_PRESET",
		"BDWIND_ENCODER_PROFILE",
		"BDWIND_ENCODER_RATE_CONTROL",
		"BDWIND_ENCODER_QUALITY",
		"BDWIND_ENCODER_THREADS",
		"BDWIND_ENCODER_LOOK_AHEAD",
		"BDWIND_ENCODER_B_FRAMES",
		"BDWIND_ENCODER_REF_FRAMES",
		"BDWIND_ENCODER_ZERO_LATENCY",
		"BDWIND_ENCODER_TUNE",
		"BDWIND_ENCODER_SLICE_MODE",
		"BDWIND_ENCODER_ADAPTIVE_BITRATE",
		"BDWIND_ENCODER_BITRATE_WINDOW",
	}

	for _, env := range envVars {
		if val := os.Getenv(env); val != "" {
			originalEnv[env] = val
		}
		os.Unsetenv(env)
	}

	// Restore environment after test
	defer func() {
		for _, env := range envVars {
			os.Unsetenv(env)
			if val, exists := originalEnv[env]; exists {
				os.Setenv(env, val)
			}
		}
	}()

	// Test with environment variables set
	os.Setenv("BDWIND_ENCODER_TYPE", "nvenc")
	os.Setenv("BDWIND_ENCODER_CODEC", "h264")
	os.Setenv("BDWIND_ENCODER_BITRATE", "3000")
	os.Setenv("BDWIND_ENCODER_MAX_BITRATE", "5000")
	os.Setenv("BDWIND_ENCODER_MIN_BITRATE", "1000")
	os.Setenv("BDWIND_ENCODER_KEYFRAME_INTERVAL", "3")
	os.Setenv("BDWIND_ENCODER_USE_HARDWARE", "false")
	os.Setenv("BDWIND_ENCODER_HARDWARE_DEVICE", "1")
	os.Setenv("BDWIND_ENCODER_FALLBACK_TO_SOFT", "false")
	os.Setenv("BDWIND_ENCODER_PRESET", "medium")
	os.Setenv("BDWIND_ENCODER_PROFILE", "high")
	os.Setenv("BDWIND_ENCODER_RATE_CONTROL", "cbr")
	os.Setenv("BDWIND_ENCODER_QUALITY", "20")
	os.Setenv("BDWIND_ENCODER_THREADS", "4")
	os.Setenv("BDWIND_ENCODER_LOOK_AHEAD", "10")
	os.Setenv("BDWIND_ENCODER_B_FRAMES", "2")
	os.Setenv("BDWIND_ENCODER_REF_FRAMES", "3")
	os.Setenv("BDWIND_ENCODER_ZERO_LATENCY", "false")
	os.Setenv("BDWIND_ENCODER_TUNE", "film")
	os.Setenv("BDWIND_ENCODER_SLICE_MODE", "multi")
	os.Setenv("BDWIND_ENCODER_ADAPTIVE_BITRATE", "false")
	os.Setenv("BDWIND_ENCODER_BITRATE_WINDOW", "10")

	config := LoadEncoderConfigFromEnv()

	// Test loaded values
	if config.Type != EncoderTypeNVENC {
		t.Errorf("Expected encoder type to be %s, got %s", EncoderTypeNVENC, config.Type)
	}
	if config.Codec != CodecH264 {
		t.Errorf("Expected codec to be %s, got %s", CodecH264, config.Codec)
	}
	if config.Bitrate != 3000 {
		t.Errorf("Expected bitrate to be 3000, got %d", config.Bitrate)
	}
	if config.MaxBitrate != 5000 {
		t.Errorf("Expected max bitrate to be 5000, got %d", config.MaxBitrate)
	}
	if config.MinBitrate != 1000 {
		t.Errorf("Expected min bitrate to be 1000, got %d", config.MinBitrate)
	}
	if config.KeyframeInterval != 3 {
		t.Errorf("Expected keyframe interval to be 3, got %d", config.KeyframeInterval)
	}
	if config.UseHardware {
		t.Error("Expected hardware acceleration to be disabled")
	}
	if config.HardwareDevice != 1 {
		t.Errorf("Expected hardware device to be 1, got %d", config.HardwareDevice)
	}
	if config.FallbackToSoft {
		t.Error("Expected fallback to software to be disabled")
	}
	if config.Preset != PresetMedium {
		t.Errorf("Expected preset to be %s, got %s", PresetMedium, config.Preset)
	}
	if config.Profile != ProfileHigh {
		t.Errorf("Expected profile to be %s, got %s", ProfileHigh, config.Profile)
	}
	if config.RateControl != RateControlCBR {
		t.Errorf("Expected rate control to be %s, got %s", RateControlCBR, config.RateControl)
	}
	if config.Quality != 20 {
		t.Errorf("Expected quality to be 20, got %d", config.Quality)
	}
	if config.Threads != 4 {
		t.Errorf("Expected threads to be 4, got %d", config.Threads)
	}
	if config.LookAhead != 10 {
		t.Errorf("Expected look ahead to be 10, got %d", config.LookAhead)
	}
	if config.BFrames != 2 {
		t.Errorf("Expected B frames to be 2, got %d", config.BFrames)
	}
	if config.RefFrames != 3 {
		t.Errorf("Expected ref frames to be 3, got %d", config.RefFrames)
	}
	if config.ZeroLatency {
		t.Error("Expected zero latency to be disabled")
	}
	if config.Tune != "film" {
		t.Errorf("Expected tune to be 'film', got %s", config.Tune)
	}
	if config.SliceMode != "multi" {
		t.Errorf("Expected slice mode to be 'multi', got %s", config.SliceMode)
	}
	if config.AdaptiveBitrate {
		t.Error("Expected adaptive bitrate to be disabled")
	}
	if config.BitrateWindow != 10 {
		t.Errorf("Expected bitrate window to be 10, got %d", config.BitrateWindow)
	}
}

func TestGetEncoderTypeFromString(t *testing.T) {
	tests := []struct {
		input    string
		expected EncoderType
	}{
		{"nvenc", EncoderTypeNVENC},
		{"NVENC", EncoderTypeNVENC},
		{"vaapi", EncoderTypeVAAPI},
		{"VAAPI", EncoderTypeVAAPI},
		{"x264", EncoderTypeX264},
		{"X264", EncoderTypeX264},
		{"vp8", EncoderTypeVP8},
		{"VP8", EncoderTypeVP8},
		{"vp9", EncoderTypeVP9},
		{"VP9", EncoderTypeVP9},
		{"auto", EncoderTypeAuto},
		{"AUTO", EncoderTypeAuto},
		{"invalid", EncoderTypeAuto},
		{"", EncoderTypeAuto},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := GetEncoderTypeFromString(tt.input)
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestGetCodecTypeFromString(t *testing.T) {
	tests := []struct {
		input    string
		expected CodecType
	}{
		{"h264", CodecH264},
		{"H264", CodecH264},
		{"vp8", CodecVP8},
		{"VP8", CodecVP8},
		{"vp9", CodecVP9},
		{"VP9", CodecVP9},
		{"invalid", CodecH264},
		{"", CodecH264},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := GetCodecTypeFromString(tt.input)
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestGetOptimalEncoderConfig(t *testing.T) {
	config := GetOptimalEncoderConfig()

	// Test that we get a valid configuration
	err := ValidateEncoderConfig(&config)
	if err != nil {
		t.Errorf("Optimal config should be valid, got error: %v", err)
	}

	// Test that encoder type is set appropriately
	validTypes := []EncoderType{EncoderTypeNVENC, EncoderTypeVAAPI, EncoderTypeX264}
	found := false
	for _, validType := range validTypes {
		if config.Type == validType {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected encoder type to be one of %v, got %s", validTypes, config.Type)
	}
}

func TestLoadSaveEncoderConfigFile(t *testing.T) {
	// Create a temporary file
	tmpFile := "/tmp/test_encoder_config.yaml"
	defer os.Remove(tmpFile)

	// Create a test config
	originalConfig := DefaultEncoderConfig()
	originalConfig.Type = EncoderTypeNVENC
	originalConfig.Bitrate = 3000
	originalConfig.Quality = 20

	// Save config to file
	err := SaveEncoderConfigToFile(originalConfig, tmpFile)
	if err != nil {
		t.Fatalf("Failed to save config to file: %v", err)
	}

	// Load config from file
	loadedConfig, err := LoadEncoderConfigFromFile(tmpFile)
	if err != nil {
		t.Fatalf("Failed to load config from file: %v", err)
	}

	// Compare configs
	if loadedConfig.Type != originalConfig.Type {
		t.Errorf("Expected encoder type %s, got %s", originalConfig.Type, loadedConfig.Type)
	}
	if loadedConfig.Bitrate != originalConfig.Bitrate {
		t.Errorf("Expected bitrate %d, got %d", originalConfig.Bitrate, loadedConfig.Bitrate)
	}
	if loadedConfig.Quality != originalConfig.Quality {
		t.Errorf("Expected quality %d, got %d", originalConfig.Quality, loadedConfig.Quality)
	}
}
