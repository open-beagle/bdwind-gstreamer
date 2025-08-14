package gstreamer

import (
	"testing"

	"github.com/open-beagle/bdwind-gstreamer/internal/config"
)

func TestEncoderFactory_CreateEncoder(t *testing.T) {
	factory := NewEncoderFactory()

	tests := []struct {
		name        string
		config      config.EncoderConfig
		expectError bool
	}{
		{
			name: "NVENC encoder",
			config: config.EncoderConfig{
				Type:  config.EncoderTypeNVENC,
				Codec: config.CodecH264,
			},
			expectError: true, // Will fail if NVENC not available
		},
		{
			name: "VAAPI encoder",
			config: config.EncoderConfig{
				Type:  config.EncoderTypeVAAPI,
				Codec: config.CodecH264,
			},
			expectError: false, // Should work if VAAPI available
		},
		{
			name: "x264 encoder",
			config: config.EncoderConfig{
				Type:  config.EncoderTypeX264,
				Codec: config.CodecH264,
			},
			expectError: false, // Should always work
		},
		{
			name: "Auto encoder",
			config: config.EncoderConfig{
				Type:           config.EncoderTypeAuto,
				Codec:          config.CodecH264,
				UseHardware:    true,
				FallbackToSoft: true,
			},
			expectError: false, // Should fallback to x264
		},
		{
			name: "Unsupported encoder",
			config: config.EncoderConfig{
				Type:  "unsupported",
				Codec: config.CodecH264,
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set default values for required fields
			cfg := config.DefaultEncoderConfig()
			cfg.Type = tt.config.Type
			cfg.Codec = tt.config.Codec
			if tt.config.UseHardware {
				cfg.UseHardware = tt.config.UseHardware
			}
			if tt.config.FallbackToSoft {
				cfg.FallbackToSoft = tt.config.FallbackToSoft
			}

			encoder, err := factory.CreateEncoder(cfg)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %v", err)
				}
				if encoder == nil {
					t.Error("Expected encoder to be created")
				}
			}
		})
	}
}

func TestEncoderFactory_CreateAutoEncoder(t *testing.T) {
	factory := NewEncoderFactory()

	tests := []struct {
		name           string
		codec          config.CodecType
		useHardware    bool
		fallbackToSoft bool
		expectError    bool
	}{
		{
			name:           "H264 with hardware and fallback",
			codec:          config.CodecH264,
			useHardware:    true,
			fallbackToSoft: true,
			expectError:    false, // Should work with fallback
		},
		{
			name:           "H264 hardware only",
			codec:          config.CodecH264,
			useHardware:    true,
			fallbackToSoft: false,
			expectError:    false, // May succeed if hardware is available
		},
		{
			name:           "H264 software only",
			codec:          config.CodecH264,
			useHardware:    false,
			fallbackToSoft: true,
			expectError:    false, // Should work with x264
		},
		{
			name:           "VP8 with hardware and fallback",
			codec:          config.CodecVP8,
			useHardware:    true,
			fallbackToSoft: true,
			expectError:    true, // VP8 software encoder not implemented
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.DefaultEncoderConfig()
			cfg.Type = config.EncoderTypeAuto
			cfg.Codec = tt.codec
			cfg.UseHardware = tt.useHardware
			cfg.FallbackToSoft = tt.fallbackToSoft

			encoder, err := factory.createAutoEncoder(cfg)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %v", err)
				}
				if encoder != nil {
					// Verify encoder properties
					if tt.useHardware && encoder.IsHardwareAccelerated() {
						t.Logf("Successfully created hardware encoder: %T", encoder)
					} else if !tt.useHardware && !encoder.IsHardwareAccelerated() {
						t.Logf("Successfully created software encoder: %T", encoder)
					}
				}
			}
		})
	}
}

func TestEncoderFactory_GetEncoderPreferences(t *testing.T) {
	factory := NewEncoderFactory()

	tests := []struct {
		name        string
		codec       config.CodecType
		useHardware bool
		minExpected int
	}{
		{
			name:        "H264 with hardware",
			codec:       config.CodecH264,
			useHardware: true,
			minExpected: 1, // At least x264 should be available
		},
		{
			name:        "H264 without hardware",
			codec:       config.CodecH264,
			useHardware: false,
			minExpected: 1, // x264 should be available
		},
		{
			name:        "VP8 with hardware",
			codec:       config.CodecVP8,
			useHardware: true,
			minExpected: 1, // VP8 software encoder
		},
		{
			name:        "VP9 with hardware",
			codec:       config.CodecVP9,
			useHardware: true,
			minExpected: 1, // VP9 software encoder
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.DefaultEncoderConfig()
			cfg.Codec = tt.codec
			cfg.UseHardware = tt.useHardware

			preferences := factory.getEncoderPreferences(cfg)

			if len(preferences) < tt.minExpected {
				t.Errorf("Expected at least %d encoder preferences, got %d", tt.minExpected, len(preferences))
			}

			t.Logf("Encoder preferences for %s (hardware=%v): %v", tt.codec, tt.useHardware, preferences)
		})
	}
}

func TestIsNVENCAvailable(t *testing.T) {
	available := IsNVENCAvailable()
	t.Logf("NVENC available: %v", available)
	// Test doesn't fail - just logs the result
}

func TestIsVAAPIAvailable(t *testing.T) {
	available := IsVAAPIAvailable()
	t.Logf("VAAPI available: %v", available)
	// Test doesn't fail - just logs the result
}

func TestEncoderSelector_SelectBestEncoder(t *testing.T) {
	selector := NewEncoderSelector()

	tests := []struct {
		name         string
		codec        config.CodecType
		requirements EncoderRequirements
		expectError  bool
	}{
		{
			name:  "Low latency H264",
			codec: config.CodecH264,
			requirements: EncoderRequirements{
				MaxLatency:      50,
				PreferHardware:  true,
				RequireRealtime: true,
			},
			expectError: false,
		},
		{
			name:  "High quality H264",
			codec: config.CodecH264,
			requirements: EncoderRequirements{
				MinQuality:     0.9,
				MaxCPUUsage:    0.8,
				PreferHardware: false,
			},
			expectError: false,
		},
		{
			name:  "Low CPU usage",
			codec: config.CodecH264,
			requirements: EncoderRequirements{
				MaxCPUUsage:    0.2,
				PreferHardware: true,
			},
			expectError: false, // Should prefer hardware encoders
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.DefaultEncoderConfig()
			cfg.Codec = tt.codec
			cfg.UseHardware = tt.requirements.PreferHardware
			cfg.FallbackToSoft = true

			encoder, err := selector.SelectBestEncoder(cfg, tt.requirements)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %v", err)
				}
				if encoder != nil {
					t.Logf("Selected encoder: %T, hardware: %v", encoder, encoder.IsHardwareAccelerated())
				}
			}
		})
	}
}

func TestEncoderSelector_GetAvailableEncoders(t *testing.T) {
	selector := NewEncoderSelector()

	tests := []struct {
		name        string
		codec       config.CodecType
		minExpected int
	}{
		{
			name:        "H264 encoders",
			codec:       config.CodecH264,
			minExpected: 1, // At least x264
		},
		{
			name:        "VP8 encoders",
			codec:       config.CodecVP8,
			minExpected: 1, // VP8 software encoder
		},
		{
			name:        "VP9 encoders",
			codec:       config.CodecVP9,
			minExpected: 1, // VP9 software encoder
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			available := selector.getAvailableEncoders(tt.codec)

			if len(available) < tt.minExpected {
				t.Errorf("Expected at least %d available encoders for %s, got %d", tt.minExpected, tt.codec, len(available))
			}

			t.Logf("Available encoders for %s: %v", tt.codec, available)
		})
	}
}

func TestEncoderSelector_ScoreEncoders(t *testing.T) {
	selector := NewEncoderSelector()

	available := []config.EncoderType{
		config.EncoderTypeNVENC,
		config.EncoderTypeVAAPI,
		config.EncoderTypeX264,
	}

	cfg := config.DefaultEncoderConfig()
	cfg.Codec = config.CodecH264

	requirements := EncoderRequirements{
		MaxLatency:      50,
		PreferHardware:  true,
		RequireRealtime: true,
	}

	scored := selector.scoreEncoders(available, cfg, requirements)

	if len(scored) != len(available) {
		t.Errorf("Expected %d scored encoders, got %d", len(available), len(scored))
	}

	// Check that scores are in descending order
	for i := 1; i < len(scored); i++ {
		if scored[i].Score > scored[i-1].Score {
			t.Errorf("Scores not in descending order: %f > %f", scored[i].Score, scored[i-1].Score)
		}
	}

	// Log the scoring results
	for i, item := range scored {
		t.Logf("Rank %d: %s (score: %.1f) - %v", i+1, item.EncoderType, item.Score, item.Reasons)
	}
}

func TestGetEncoderCapabilities(t *testing.T) {
	encoderTypes := []config.EncoderType{
		config.EncoderTypeNVENC,
		config.EncoderTypeVAAPI,
		config.EncoderTypeX264,
		config.EncoderTypeVP8,
		config.EncoderTypeVP9,
	}

	for _, encoderType := range encoderTypes {
		t.Run(string(encoderType), func(t *testing.T) {
			caps := GetEncoderCapabilities(encoderType)

			// Basic validation
			if len(caps.SupportedCodecs) == 0 {
				t.Errorf("Expected at least one supported codec for %s", encoderType)
			}

			if caps.TypicalLatency < 0 {
				t.Errorf("Expected non-negative latency for %s", encoderType)
			}

			if caps.TypicalCPUUsage < 0 || caps.TypicalCPUUsage > 1 {
				t.Errorf("Expected CPU usage between 0 and 1 for %s, got %f", encoderType, caps.TypicalCPUUsage)
			}

			if caps.QualityRating < 0 || caps.QualityRating > 1 {
				t.Errorf("Expected quality rating between 0 and 1 for %s, got %f", encoderType, caps.QualityRating)
			}

			if caps.PerformanceRating < 0 || caps.PerformanceRating > 1 {
				t.Errorf("Expected performance rating between 0 and 1 for %s, got %f", encoderType, caps.PerformanceRating)
			}

			t.Logf("%s capabilities: HW=%v, Codecs=%v, MaxRes=%s, Latency=%dms, CPU=%.1f%%, Quality=%.1f, Performance=%.1f",
				encoderType, caps.IsHardwareAccelerated, caps.SupportedCodecs, caps.MaxResolution,
				caps.TypicalLatency, caps.TypicalCPUUsage*100, caps.QualityRating, caps.PerformanceRating)
		})
	}
}

func TestContainsEncoderType(t *testing.T) {
	slice := []config.EncoderType{
		config.EncoderTypeNVENC,
		config.EncoderTypeVAAPI,
		config.EncoderTypeX264,
	}

	tests := []struct {
		item     config.EncoderType
		expected bool
	}{
		{config.EncoderTypeNVENC, true},
		{config.EncoderTypeVAAPI, true},
		{config.EncoderTypeX264, true},
		{config.EncoderTypeVP8, false},
		{config.EncoderTypeVP9, false},
	}

	for _, tt := range tests {
		result := containsEncoderType(slice, tt.item)
		if result != tt.expected {
			t.Errorf("containsEncoderType(%v, %s) = %v, expected %v", slice, tt.item, result, tt.expected)
		}
	}
}

func TestEncoderFallbackScenarios(t *testing.T) {
	factory := NewEncoderFactory()

	// Test scenario: NVENC not available, should fallback to VAAPI or x264
	t.Run("NVENC fallback", func(t *testing.T) {
		cfg := config.DefaultEncoderConfig()
		cfg.Type = config.EncoderTypeAuto
		cfg.Codec = config.CodecH264
		cfg.UseHardware = true
		cfg.FallbackToSoft = true

		encoder, err := factory.CreateEncoder(cfg)
		if err != nil {
			t.Errorf("Expected fallback to work, got error: %v", err)
		}
		if encoder == nil {
			t.Error("Expected encoder to be created through fallback")
		}
	})

	// Test scenario: Hardware disabled, should use software
	t.Run("Hardware disabled", func(t *testing.T) {
		cfg := config.DefaultEncoderConfig()
		cfg.Type = config.EncoderTypeAuto
		cfg.Codec = config.CodecH264
		cfg.UseHardware = false
		cfg.FallbackToSoft = true

		encoder, err := factory.CreateEncoder(cfg)
		if err != nil {
			t.Errorf("Expected software encoder to work, got error: %v", err)
		}
		if encoder == nil {
			t.Error("Expected software encoder to be created")
		}
		if encoder.IsHardwareAccelerated() {
			t.Error("Expected software encoder, got hardware encoder")
		}
	})

	// Test scenario: No fallback allowed
	t.Run("No fallback", func(t *testing.T) {
		cfg := config.DefaultEncoderConfig()
		cfg.Type = config.EncoderTypeAuto
		cfg.Codec = config.CodecH264
		cfg.UseHardware = true
		cfg.FallbackToSoft = false

		encoder, err := factory.CreateEncoder(cfg)
		// This may succeed if hardware is available, or fail if not
		if err != nil {
			t.Logf("No hardware encoder available (expected): %v", err)
		} else if encoder != nil && !encoder.IsHardwareAccelerated() {
			t.Error("Expected hardware encoder or error, got software encoder")
		}
	})
}
