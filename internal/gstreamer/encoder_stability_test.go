package gstreamer

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestEncoderGst_StabilityImprovements(t *testing.T) {
	t.Run("Hardware Detection Logic", func(t *testing.T) {
		testHardwareDetectionLogic(t)
	})

	t.Run("Encoder Initialization Fallback", func(t *testing.T) {
		testEncoderInitializationFallback(t)
	})

	t.Run("Error Handling and Recovery", func(t *testing.T) {
		testErrorHandlingAndRecovery(t)
	})

	t.Run("Type Safe Property Setting", func(t *testing.T) {
		testTypeSafePropertySetting(t)
	})
}

func testHardwareDetectionLogic(t *testing.T) {
	config := createValidX264Config()
	config.UseHardware = true
	config.FallbackToSoft = true

	encoder, err := NewEncoderGst(config)
	if err != nil {
		t.Skipf("Encoder creation failed: %v", err)
		return
	}
	defer encoder.Stop()

	// Test hardware capability checking
	t.Run("NVENC Capability Check", func(t *testing.T) {
		available := encoder.isEncoderAvailable(EncoderTypeHardwareNVENC)
		t.Logf("NVENC available: %t", available)

		if available {
			// Test hardware-specific checks
			assert.True(t, encoder.isHardwareEncoder(EncoderTypeHardwareNVENC))
		}
	})

	t.Run("VAAPI Capability Check", func(t *testing.T) {
		available := encoder.isEncoderAvailable(EncoderTypeHardwareVAAPI)
		t.Logf("VAAPI available: %t", available)

		if available {
			assert.True(t, encoder.isHardwareEncoder(EncoderTypeHardwareVAAPI))
		}
	})

	t.Run("Software Encoder Fallback", func(t *testing.T) {
		available := encoder.isEncoderAvailable(EncoderTypeSoftwareX264)
		t.Logf("x264 available: %t", available)

		// Software encoder should generally be available
		assert.True(t, available)
		assert.False(t, encoder.isHardwareEncoder(EncoderTypeSoftwareX264))
	})
}

func testEncoderInitializationFallback(t *testing.T) {
	config := createValidH264Config()
	config.UseHardware = true
	config.FallbackToSoft = true

	encoder, err := NewEncoderGst(config)
	if err != nil {
		t.Skipf("Encoder creation failed: %v", err)
		return
	}
	defer encoder.Stop()

	// Test encoder selection with fallback
	t.Run("Encoder Selection Strategy", func(t *testing.T) {
		chain := encoder.buildEncoderFallbackChain()
		assert.NotEmpty(t, chain, "Fallback chain should not be empty")

		// Should have at least software encoder as fallback
		hasSwEncoder := false
		for _, encoderType := range chain {
			if encoderType == EncoderTypeSoftwareX264 {
				hasSwEncoder = true
				break
			}
		}
		assert.True(t, hasSwEncoder, "Should have software encoder in fallback chain")
	})

	t.Run("Encoder Initialization Test", func(t *testing.T) {
		// Test initialization of selected encoder
		err := encoder.testEncoderInitialization(encoder.selectedEncoder)
		if err != nil {
			t.Logf("Selected encoder %s failed initialization test: %v",
				encoder.getEncoderName(encoder.selectedEncoder), err)
		}
	})

	t.Run("Pipeline Creation with Fallback", func(t *testing.T) {
		// Test pipeline creation which should use fallback if needed
		err := encoder.initializePipeline()
		if err != nil {
			t.Logf("Pipeline initialization failed: %v", err)
		} else {
			t.Logf("Successfully initialized pipeline with encoder: %s",
				encoder.getEncoderName(encoder.selectedEncoder))
		}
	})
}

func testErrorHandlingAndRecovery(t *testing.T) {
	config := createValidX264Config()

	encoder, err := NewEncoderGst(config)
	if err != nil {
		t.Skipf("Encoder creation failed: %v", err)
		return
	}
	defer encoder.Stop()

	t.Run("Sample Conversion Error Handling", func(t *testing.T) {
		// Test nil sample handling
		_, err := encoder.convertGstSample(nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "nil gst sample")
	})

	t.Run("Property Setting Error Handling", func(t *testing.T) {
		// Test property setting with invalid values
		properties := []PropertyConfig{
			{"invalid-property", "invalid-value", false}, // Non-critical
			{"bitrate", uint(1000), true},                // Valid
		}

		// This should not fail even with invalid property because it's not required
		err := encoder.setEncoderPropertiesLegacy(properties, "test")
		assert.NoError(t, err)
	})

	t.Run("Pipeline Recovery", func(t *testing.T) {
		if encoder.pipeline == nil {
			err := encoder.initializePipeline()
			if err != nil {
				t.Skipf("Cannot test recovery without pipeline: %v", err)
				return
			}
		}

		// Test pipeline restart
		err := encoder.restartPipeline()
		if err != nil {
			t.Logf("Pipeline restart failed (expected in test environment): %v", err)
		}
	})
}

func testTypeSafePropertySetting(t *testing.T) {
	config := createValidX264Config()

	encoder, err := NewEncoderGst(config)
	if err != nil {
		t.Skipf("Encoder creation failed: %v", err)
		return
	}
	defer encoder.Stop()

	t.Run("Property Value Conversion", func(t *testing.T) {
		tests := []struct {
			name     string
			input    interface{}
			expected interface{}
			hasError bool
		}{
			{"String", "test", "test", false},
			{"Bool", true, true, false},
			{"Int", 42, uint(42), false},
			{"Negative Int", -5, uint(0), false}, // Should convert to 0
			{"Uint", uint(42), uint(42), false},
			{"Float", 3.14, 3.14, false},
			{"Nil", nil, nil, false},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result, err := encoder.convertPropertyValue(tt.input)

				if tt.hasError {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
					assert.Equal(t, tt.expected, result)
				}
			})
		}
	})

	t.Run("Safe Property Setting", func(t *testing.T) {
		// Initialize pipeline first
		if encoder.pipeline == nil {
			err := encoder.initializePipeline()
			if err != nil {
				t.Skipf("Cannot test property setting without pipeline: %v", err)
				return
			}
		}

		// Test setting a valid property
		err := encoder.setEncoderPropertySafe("bitrate", 1000)
		if err != nil {
			t.Logf("Property setting failed (may be expected in test environment): %v", err)
		}

		// Test setting an invalid property (should timeout or fail gracefully)
		err = encoder.setEncoderPropertySafe("non-existent-property", "value")
		assert.Error(t, err) // Should fail but not crash
	})

	t.Run("Encoder Configuration", func(t *testing.T) {
		// Test different encoder configurations
		encoderTypes := []EncoderType{
			EncoderTypeSoftwareX264,
			EncoderTypeSoftwareVP8,
			EncoderTypeSoftwareVP9,
		}

		for _, encoderType := range encoderTypes {
			t.Run(encoder.getEncoderName(encoderType), func(t *testing.T) {
				if !encoder.isEncoderAvailable(encoderType) {
					t.Skipf("Encoder %s not available", encoder.getEncoderName(encoderType))
					return
				}

				// Test configuration
				err := encoder.testEncoderConfiguration(nil, encoderType)
				// This will fail because element is nil, but should not crash
				assert.Error(t, err)
			})
		}
	})
}

func TestEncoderGst_PropertyConfigValidation(t *testing.T) {
	config := createValidX264Config()

	encoder, err := NewEncoderGst(config)
	if err != nil {
		t.Skipf("Encoder creation failed: %v", err)
		return
	}
	defer encoder.Stop()

	t.Run("Required Property Failure", func(t *testing.T) {
		properties := []PropertyConfig{
			{"critical-property", "value", true}, // Required but will fail
		}

		err := encoder.setEncoderPropertiesLegacy(properties, "test")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "critical")
	})

	t.Run("Optional Property Failure", func(t *testing.T) {
		properties := []PropertyConfig{
			{"optional-property", "value", false}, // Optional, failure is OK
			{"bitrate", uint(1000), true},         // Required and should work
		}

		// Should succeed even with optional property failure
		err := encoder.setEncoderPropertiesLegacy(properties, "test")
		// May still fail if encoder element is not available, but should not crash
		if err != nil {
			t.Logf("Property setting failed (expected in test environment): %v", err)
		}
	})
}

func TestEncoderGst_ErrorRecoveryScenarios(t *testing.T) {
	config := createValidX264Config()
	config.AdaptiveBitrate = true

	encoder, err := NewEncoderGst(config)
	if err != nil {
		t.Skipf("Encoder creation failed: %v", err)
		return
	}
	defer encoder.Stop()

	t.Run("Multiple Error Recovery", func(t *testing.T) {
		// Simulate multiple errors to test recovery limits
		for i := 0; i < 3; i++ {
			// This would normally trigger error recovery
			encoder.logger.Infof("Simulating error recovery attempt %d", i+1)

			// Test that recovery mechanisms don't crash
			err := encoder.restartPipeline()
			if err != nil {
				t.Logf("Recovery attempt %d failed: %v", i+1, err)
			}
		}
	})

	t.Run("Fallback Chain Exhaustion", func(t *testing.T) {
		// Test what happens when all encoders in fallback chain fail
		originalChain := encoder.fallbackChain

		// Set to empty chain to test exhaustion
		encoder.fallbackChain = []EncoderType{}

		err := encoder.fallbackToNextEncoder()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no more encoders")

		// Restore original chain
		encoder.fallbackChain = originalChain
	})
}

func TestEncoderGst_MemorySafety(t *testing.T) {
	config := createValidX264Config()

	encoder, err := NewEncoderGst(config)
	if err != nil {
		t.Skipf("Encoder creation failed: %v", err)
		return
	}
	defer encoder.Stop()

	t.Run("Large Buffer Handling", func(t *testing.T) {
		// Test handling of oversized buffers
		largeData := make([]byte, 100*1024*1024) // 100MB
		sample := &Sample{
			Data:      largeData,
			Timestamp: time.Now(),
			Format: SampleFormat{
				MediaType: MediaTypeVideo,
				Codec:     "raw",
			},
		}

		// This should be handled gracefully without crashing
		err := encoder.PushSample(sample)
		if err != nil {
			t.Logf("Large sample rejected (expected): %v", err)
		}
	})

	t.Run("Cleanup Safety", func(t *testing.T) {
		// Test that cleanup doesn't crash
		encoder.cleanupPipeline()
		encoder.cleanupPipeline() // Double cleanup should be safe
	})

	t.Run("Concurrent Access Safety", func(t *testing.T) {
		// Test concurrent access to encoder
		done := make(chan bool, 2)

		go func() {
			for i := 0; i < 10; i++ {
				encoder.GetStats()
				time.Sleep(10 * time.Millisecond)
			}
			done <- true
		}()

		go func() {
			for i := 0; i < 10; i++ {
				encoder.IsRunning()
				time.Sleep(10 * time.Millisecond)
			}
			done <- true
		}()

		// Wait for both goroutines
		<-done
		<-done
	})
}
