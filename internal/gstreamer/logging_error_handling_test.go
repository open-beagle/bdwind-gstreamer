package gstreamer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/open-beagle/bdwind-gstreamer/internal/config"
)

func TestGStreamerLogConfigurator_ErrorHandling(t *testing.T) {
	logger := logrus.NewEntry(logrus.New())
	configurator := NewGStreamerLogConfigurator(logger)

	t.Run("ConfigureFromAppLogging_NilConfig", func(t *testing.T) {
		err := configurator.ConfigureFromAppLogging(nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "app config is nil")
	})

	t.Run("SetLogLevel_InvalidLevel", func(t *testing.T) {
		// Test negative level
		err := configurator.SetLogLevel(-1)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "level must be between 0 and 9")

		// Test level too high
		err = configurator.SetLogLevel(10)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "level must be between 0 and 9")
	})

	t.Run("EnableCategory_InvalidLevel", func(t *testing.T) {
		err := configurator.EnableCategory("GST_PIPELINE", -1)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "level must be between 0 and 9")

		err = configurator.EnableCategory("GST_PIPELINE", 15)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "level must be between 0 and 9")
	})

	t.Run("EnableCategory_EmptyCategory", func(t *testing.T) {
		err := configurator.EnableCategory("", 4)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "category name cannot be empty")
	})

	t.Run("UpdateConfig_NilConfig", func(t *testing.T) {
		err := configurator.UpdateConfig(nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "config is nil")
	})
}

func TestGStreamerLogConfigurator_FileErrorHandling(t *testing.T) {
	logger := logrus.NewEntry(logrus.New())
	configurator := NewGStreamerLogConfigurator(logger)

	t.Run("InvalidLogDirectory", func(t *testing.T) {
		// Try to use a file as a directory (should fail)
		tempFile, err := os.CreateTemp("", "test_file")
		require.NoError(t, err)
		defer os.Remove(tempFile.Name())
		tempFile.Close()

		invalidLogFile := filepath.Join(tempFile.Name(), "gstreamer.log")

		appConfig := &config.LoggingConfig{
			Level:  "debug",
			Output: "file",
			File:   invalidLogFile,
		}

		err = configurator.ConfigureFromAppLogging(appConfig)
		// Should not fail completely, but should fallback to console
		assert.NoError(t, err)

		config := configurator.GetCurrentConfig()
		// Should have fallen back to console output
		assert.Empty(t, config.OutputFile)
	})

	t.Run("ReadOnlyDirectory", func(t *testing.T) {
		if os.Getuid() == 0 {
			t.Skip("Skipping read-only directory test when running as root")
		}

		// Create a read-only directory
		tempDir, err := os.MkdirTemp("", "readonly_test")
		require.NoError(t, err)
		defer os.RemoveAll(tempDir)

		// Make directory read-only
		err = os.Chmod(tempDir, 0444)
		require.NoError(t, err)
		defer os.Chmod(tempDir, 0755) // Restore permissions for cleanup

		logFile := filepath.Join(tempDir, "gstreamer.log")

		appConfig := &config.LoggingConfig{
			Level:  "debug",
			Output: "file",
			File:   logFile,
		}

		err = configurator.ConfigureFromAppLogging(appConfig)
		// Should not fail completely, but should fallback to console
		assert.NoError(t, err)

		config := configurator.GetCurrentConfig()
		// Should have fallen back to console output
		assert.Empty(t, config.OutputFile)
	})
}

func TestGStreamerLogConfigurator_ConfigValidation(t *testing.T) {
	logger := logrus.NewEntry(logrus.New())
	configurator := NewGStreamerLogConfigurator(logger).(*gstreamerLogConfigurator)

	t.Run("ValidateConfig_InvalidLevel", func(t *testing.T) {
		config := &GStreamerLogConfig{
			Level: -1,
		}

		err := configurator.validateConfig(config)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid log level")
	})

	t.Run("ValidateConfig_InvalidCategoryLevel", func(t *testing.T) {
		config := &GStreamerLogConfig{
			Level: 4,
			Categories: map[string]int{
				"GST_PIPELINE": 15,
			},
		}

		err := configurator.validateConfig(config)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid log level")
		assert.Contains(t, err.Error(), "GST_PIPELINE")
	})

	t.Run("ValidateConfig_NonexistentDirectory", func(t *testing.T) {
		config := &GStreamerLogConfig{
			Level:      4,
			OutputFile: "/nonexistent/directory/gstreamer.log",
		}

		err := configurator.validateConfig(config)
		// Should try to create directory and may fail
		if err != nil {
			assert.Contains(t, err.Error(), "cannot create log directory")
		}
	})
}

func TestGStreamerLogConfigurator_EnvironmentErrorHandling(t *testing.T) {
	logger := logrus.NewEntry(logrus.New())
	configurator := NewGStreamerLogConfigurator(logger)

	t.Run("InvalidGST_DEBUG_Format", func(t *testing.T) {
		os.Setenv("GST_DEBUG", "invalid_format_here")
		defer os.Unsetenv("GST_DEBUG")

		appConfig := &config.LoggingConfig{
			Level:  "debug",
			Output: "stdout",
		}

		err := configurator.ConfigureFromAppLogging(appConfig)
		assert.NoError(t, err) // Should not fail, just use level 0

		config := configurator.GetCurrentConfig()
		assert.False(t, config.Enabled)
		assert.Equal(t, 0, config.Level)
	})

	t.Run("InvalidGST_DEBUG_FILE", func(t *testing.T) {
		// Create a file and try to use it as a directory
		tempFile, err := os.CreateTemp("", "test_file")
		require.NoError(t, err)
		defer os.Remove(tempFile.Name())
		tempFile.Close()

		invalidPath := filepath.Join(tempFile.Name(), "gstreamer.log")

		os.Setenv("GST_DEBUG", "4")
		os.Setenv("GST_DEBUG_FILE", invalidPath)
		defer func() {
			os.Unsetenv("GST_DEBUG")
			os.Unsetenv("GST_DEBUG_FILE")
		}()

		appConfig := &config.LoggingConfig{
			Level:  "info", // Should be overridden
			Output: "stdout",
		}

		err = configurator.ConfigureFromAppLogging(appConfig)
		assert.NoError(t, err) // Should not fail, should fallback to console

		config := configurator.GetCurrentConfig()
		assert.True(t, config.Enabled)
		assert.Equal(t, 4, config.Level)
		// Should have fallen back to console output
		assert.Empty(t, config.OutputFile)
	})
}

func TestParseGstDebugLevel_ErrorHandling(t *testing.T) {
	testCases := []struct {
		input         string
		expectError   bool
		expectedLevel int
		description   string
	}{
		{"", true, 0, "empty string"},
		{"invalid", true, 0, "invalid format"},
		{"10", true, 0, "level too high"},
		{"a", true, 0, "non-numeric"},
		{"GST_PIPELINE:15", false, 4, "category level too high - returns default level 4"},
		{":", false, 4, "empty category and level - returns default level 4"},
		{"GST_PIPELINE:", false, 4, "empty level - returns default level 4"},
		{":4", false, 4, "empty category - returns default level 4"},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			result, err := parseGstDebugLevel(tc.input)
			if tc.expectError {
				assert.Error(t, err, "Expected error for input: %s", tc.input)
			} else {
				assert.NoError(t, err, "Expected no error for input: %s", tc.input)
				assert.Equal(t, tc.expectedLevel, result, "Expected level %d for input: %s", tc.expectedLevel, tc.input)
			}
		})
	}
}

func TestGStreamerLogConfigurator_FallbackBehavior(t *testing.T) {
	logger := logrus.NewEntry(logrus.New())
	configurator := NewGStreamerLogConfigurator(logger)

	t.Run("FileFallbackToConsole", func(t *testing.T) {
		// Create a file where we expect a directory to simulate failure
		tempFile, err := os.CreateTemp("", "fallback_test")
		require.NoError(t, err)
		defer os.Remove(tempFile.Name())
		tempFile.Close()

		// Try to create a log file inside what should be a directory but is actually a file
		invalidLogDir := tempFile.Name()
		appLogFile := filepath.Join(invalidLogDir, "app.log")

		appConfig := &config.LoggingConfig{
			Level:  "debug",
			Output: "file",
			File:   appLogFile, // This will generate a gstreamer.log path that can't be created
		}

		err = configurator.ConfigureFromAppLogging(appConfig)
		assert.NoError(t, err) // Should not fail, should fallback

		config := configurator.GetCurrentConfig()
		assert.True(t, config.Enabled)
		assert.Equal(t, 4, config.Level)
		// Should have fallen back to console output
		assert.Empty(t, config.OutputFile)
	})
}

func TestGStreamerLogConfigurator_LoggingOutput(t *testing.T) {
	// Create a custom logger to capture log output
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	var logOutput strings.Builder
	logger.SetOutput(&logOutput)

	entry := logrus.NewEntry(logger)
	configurator := NewGStreamerLogConfigurator(entry)

	t.Run("DebugLoggingForConfiguration", func(t *testing.T) {
		appConfig := &config.LoggingConfig{
			Level:  "debug",
			Output: "stdout",
		}

		err := configurator.ConfigureFromAppLogging(appConfig)
		assert.NoError(t, err)

		logContent := logOutput.String()

		// Check that debug messages are present
		assert.Contains(t, logContent, "Starting GStreamer logging configuration")
		assert.Contains(t, logContent, "No GStreamer environment variables found")
		assert.Contains(t, logContent, "Generated GStreamer config from app config")
		assert.Contains(t, logContent, "Successfully applied GStreamer logging configuration")
	})
}

func TestGStreamerLogConfigurator_ConfigurationStatus(t *testing.T) {
	logger := logrus.NewEntry(logrus.New())
	configurator := NewGStreamerLogConfigurator(logger)

	t.Run("StatusLoggingForDifferentLevels", func(t *testing.T) {
		testCases := []struct {
			level    string
			expected bool
		}{
			{"trace", true},
			{"debug", true},
			{"info", false},
			{"warn", false},
			{"error", false},
		}

		for _, tc := range testCases {
			t.Run(fmt.Sprintf("level_%s", tc.level), func(t *testing.T) {
				appConfig := &config.LoggingConfig{
					Level:  tc.level,
					Output: "stdout",
				}

				err := configurator.ConfigureFromAppLogging(appConfig)
				assert.NoError(t, err)

				config := configurator.GetCurrentConfig()
				assert.Equal(t, tc.expected, config.Enabled)
			})
		}
	})
}
