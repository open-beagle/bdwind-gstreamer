package gstreamer

import (
	"testing"

	"github.com/open-beagle/bdwind-gstreamer/internal/config"
	"github.com/sirupsen/logrus"
)

func TestConfigureX264PropertiesRefactor(t *testing.T) {
	// Test the refactored configureX264Properties method
	t.Run("PropertyDescriptorMapping", func(t *testing.T) {
		// Create a mock encoder with test configuration
		enc := &EncoderGst{
			config: config.EncoderConfig{
				Bitrate:          2000,
				Preset:           config.PresetFast,
				Profile:          config.ProfileMain,
				Tune:             "zerolatency",
				Threads:          4,
				BFrames:          2,
				KeyframeInterval: 2,
				RateControl:      config.RateControlCBR,
				ZeroLatency:      true,
			},
			logger: logrus.WithField("component", "test-encoder"),
		}

		// Test buildX264ConfigurationValues
		configValues := enc.buildX264ConfigurationValues()

		// Verify expected configuration values
		expectedValues := map[string]interface{}{
			"bitrate":      2000,
			"speed-preset": "fast",
			"profile":      "main",
			"tune":         "zerolatency",
			"threads":      4,
			"bframes":      0,  // Should be 0 due to ZeroLatency
			"key-int-max":  60, // 2 * 30fps
		}

		for key, expectedValue := range expectedValues {
			if actualValue, exists := configValues[key]; !exists {
				t.Errorf("Expected configuration key '%s' not found", key)
			} else if actualValue != expectedValue {
				t.Errorf("Configuration key '%s': expected %v, got %v", key, expectedValue, actualValue)
			}
		}
	})

	t.Run("PropertyValidation", func(t *testing.T) {
		// Test individual property validation
		testCases := []struct {
			name          string
			property      string
			value         interface{}
			shouldSucceed bool
		}{
			{"ValidBitrate", "bitrate", 2000, true},
			{"InvalidBitrate", "bitrate", -1, false},
			{"ValidSpeedPreset", "speed-preset", "fast", true},
			{"InvalidSpeedPreset", "speed-preset", "invalid", false},
			{"ValidProfile", "profile", "main", true},
			{"InvalidProfile", "profile", "invalid", false},
			{"ValidTune", "tune", "zerolatency", true},
			{"InvalidTune", "tune", "invalid", false},
			{"ValidBFrames", "bframes", 2, true},
			{"InvalidBFrames", "bframes", -1, false},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				descriptor, exists := x264PropertyMap[tc.property]
				if !exists {
					t.Fatalf("Property descriptor not found for '%s'", tc.property)
				}

				if descriptor.Validator != nil {
					_, err := descriptor.Validator(tc.value)
					if tc.shouldSucceed && err != nil {
						t.Errorf("Expected validation to succeed for %s=%v, but got error: %v", tc.property, tc.value, err)
					} else if !tc.shouldSucceed && err == nil {
						t.Errorf("Expected validation to fail for %s=%v, but it succeeded", tc.property, tc.value)
					}
				}
			})
		}
	})

	t.Run("ErrorClassification", func(t *testing.T) {
		// Test that errors are properly classified as critical vs warnings
		result := &X264ConfigResult{
			Results:        make([]PropertyConfigResult, 0),
			CriticalErrors: make([]error, 0),
			Warnings:       make([]error, 0),
		}

		// Simulate some property results
		results := []PropertyConfigResult{
			{PropertyName: "bitrate", Success: true, Value: 2000, Error: nil},
			{PropertyName: "speed-preset", Success: false, Value: "fast", Error: &PropertyError{"speed-preset", "type mismatch"}},
			{PropertyName: "profile", Success: false, Value: "main", Error: &PropertyError{"profile", "not available"}},
			{PropertyName: "threads", Success: false, Value: 4, Error: &PropertyError{"threads", "critical failure"}},
		}

		for _, propResult := range results {
			result.Results = append(result.Results, propResult)

			if !propResult.Success {
				descriptor := x264PropertyMap[propResult.PropertyName]
				if descriptor.Required {
					result.CriticalErrors = append(result.CriticalErrors, propResult.Error)
				} else {
					result.Warnings = append(result.Warnings, propResult.Error)
				}
			}
		}

		// Verify error classification
		if len(result.CriticalErrors) != 1 {
			t.Errorf("Expected 1 critical error, got %d", len(result.CriticalErrors))
		}

		if len(result.Warnings) != 2 {
			t.Errorf("Expected 2 warnings, got %d", len(result.Warnings))
		}

		// Verify that required properties generate critical errors
		threadsDescriptor := x264PropertyMap["threads"]
		if !threadsDescriptor.Required {
			t.Error("Expected 'threads' property to be required")
		}

		// Verify that optional properties generate warnings
		presetDescriptor := x264PropertyMap["speed-preset"]
		if presetDescriptor.Required {
			t.Error("Expected 'speed-preset' property to be optional")
		}
	})
}

// PropertyError is a simple error type for testing
type PropertyError struct {
	Property string
	Message  string
}

func (e *PropertyError) Error() string {
	return e.Message
}
