package gstreamer

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/open-beagle/bdwind-gstreamer/internal/config"
)

func TestSetDebugLevel(t *testing.T) {
	// Test setting different debug levels
	testLevels := []int{0, 1, 4, 9}

	for _, level := range testLevels {
		t.Run(fmt.Sprintf("level_%d", level), func(t *testing.T) {
			// This should not panic or cause errors
			SetDebugLevel(level)
		})
	}
}

func TestSetLogFile(t *testing.T) {
	// Create a temporary directory for test log files
	tempDir, err := os.MkdirTemp("", "gstreamer_log_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Test setting log file
	logFile := filepath.Join(tempDir, "test_gstreamer.log")

	t.Run("set_log_file", func(t *testing.T) {
		// This should not panic or cause errors
		SetLogFile(logFile)

		// Enable some logging to test file creation
		SetDebugLevel(4)

		// Give it a moment for any potential file operations
		time.Sleep(100 * time.Millisecond)

		// Reset to console logging
		SetLogFile("")
		SetDebugLevel(0)
	})
}

func TestSetColoredOutput(t *testing.T) {
	// Test enabling and disabling colored output
	t.Run("enable_colored", func(t *testing.T) {
		// This should not panic or cause errors
		SetColoredOutput(true)
	})

	t.Run("disable_colored", func(t *testing.T) {
		// This should not panic or cause errors
		SetColoredOutput(false)
	})
}

func TestLoggingFunctionsCombined(t *testing.T) {
	// Test using all logging functions together
	tempDir, err := os.MkdirTemp("", "gstreamer_combined_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	logFile := filepath.Join(tempDir, "combined_test.log")

	// Configure logging
	SetDebugLevel(4)
	SetLogFile(logFile)
	SetColoredOutput(false) // Disable colors for file output

	// Give it a moment
	time.Sleep(100 * time.Millisecond)

	// Reset to defaults
	SetDebugLevel(0)
	SetLogFile("")
	SetColoredOutput(true)
}

func TestGStreamerLogConfigurator(t *testing.T) {
	logger := logrus.NewEntry(logrus.New())
	configurator := NewGStreamerLogConfigurator(logger)

	t.Run("ConfigureFromAppLogging_Debug", func(t *testing.T) {
		appConfig := &config.LoggingConfig{
			Level:  "debug",
			Output: "stdout",
		}

		err := configurator.ConfigureFromAppLogging(appConfig)
		assert.NoError(t, err)

		config := configurator.GetCurrentConfig()
		assert.True(t, config.Enabled)
		assert.Equal(t, 4, config.Level)
		assert.Empty(t, config.OutputFile)
	})

	t.Run("ConfigureFromAppLogging_Info", func(t *testing.T) {
		appConfig := &config.LoggingConfig{
			Level:  "info",
			Output: "stdout",
		}

		err := configurator.ConfigureFromAppLogging(appConfig)
		assert.NoError(t, err)

		config := configurator.GetCurrentConfig()
		assert.False(t, config.Enabled)
		assert.Equal(t, 0, config.Level)
	})

	t.Run("ConfigureFromAppLogging_FileOutput", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "gstreamer_config_test")
		require.NoError(t, err)
		defer os.RemoveAll(tempDir)

		appLogFile := filepath.Join(tempDir, "app.log")
		expectedGstLogFile := filepath.Join(tempDir, "gstreamer.log")

		appConfig := &config.LoggingConfig{
			Level:  "debug",
			Output: "file",
			File:   appLogFile,
		}

		err = configurator.ConfigureFromAppLogging(appConfig)
		assert.NoError(t, err)

		config := configurator.GetCurrentConfig()
		assert.True(t, config.Enabled)
		assert.Equal(t, expectedGstLogFile, config.OutputFile)
	})

	t.Run("SetLogLevel", func(t *testing.T) {
		err := configurator.SetLogLevel(5)
		assert.NoError(t, err)

		config := configurator.GetCurrentConfig()
		assert.Equal(t, 5, config.Level)
		assert.True(t, config.Enabled)

		// Test invalid level
		err = configurator.SetLogLevel(10)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "level must be between 0 and 9")
	})

	t.Run("SetLogFile", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "gstreamer_logfile_test")
		require.NoError(t, err)
		defer os.RemoveAll(tempDir)

		logFile := filepath.Join(tempDir, "test.log")

		err = configurator.SetLogFile(logFile)
		assert.NoError(t, err)

		config := configurator.GetCurrentConfig()
		assert.Equal(t, logFile, config.OutputFile)
	})

	t.Run("EnableCategory", func(t *testing.T) {
		err := configurator.EnableCategory("GST_PIPELINE", 4)
		assert.NoError(t, err)

		config := configurator.GetCurrentConfig()
		assert.Equal(t, 4, config.Categories["GST_PIPELINE"])

		// Test invalid level
		err = configurator.EnableCategory("GST_ELEMENT", 15)
		assert.Error(t, err)
	})

	t.Run("DisableLogging", func(t *testing.T) {
		err := configurator.DisableLogging()
		assert.NoError(t, err)

		config := configurator.GetCurrentConfig()
		assert.False(t, config.Enabled)
		assert.Equal(t, 0, config.Level)
	})
}

func TestGStreamerLogConfigurator_EnvironmentOverride(t *testing.T) {
	logger := logrus.NewEntry(logrus.New())
	configurator := NewGStreamerLogConfigurator(logger)

	t.Run("GST_DEBUG_Override", func(t *testing.T) {
		// Set environment variable
		os.Setenv("GST_DEBUG", "5")
		defer os.Unsetenv("GST_DEBUG")

		appConfig := &config.LoggingConfig{
			Level:  "info", // This should be overridden
			Output: "stdout",
		}

		err := configurator.ConfigureFromAppLogging(appConfig)
		assert.NoError(t, err)

		config := configurator.GetCurrentConfig()
		assert.True(t, config.Enabled)
		assert.Equal(t, 5, config.Level)
	})

	t.Run("GST_DEBUG_FILE_Override", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "gstreamer_env_test")
		require.NoError(t, err)
		defer os.RemoveAll(tempDir)

		envLogFile := filepath.Join(tempDir, "env_gstreamer.log")

		// Set environment variables
		os.Setenv("GST_DEBUG", "4")
		os.Setenv("GST_DEBUG_FILE", envLogFile)
		defer func() {
			os.Unsetenv("GST_DEBUG")
			os.Unsetenv("GST_DEBUG_FILE")
		}()

		appConfig := &config.LoggingConfig{
			Level:  "info",
			Output: "stdout",
		}

		err = configurator.ConfigureFromAppLogging(appConfig)
		assert.NoError(t, err)

		config := configurator.GetCurrentConfig()
		assert.Equal(t, envLogFile, config.OutputFile)
	})

	t.Run("GST_DEBUG_FILE_Only_Override", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "gstreamer_env_file_test")
		require.NoError(t, err)
		defer os.RemoveAll(tempDir)

		envLogFile := filepath.Join(tempDir, "env_only_gstreamer.log")

		// Set only GST_DEBUG_FILE environment variable
		os.Setenv("GST_DEBUG_FILE", envLogFile)
		defer os.Unsetenv("GST_DEBUG_FILE")

		appConfig := &config.LoggingConfig{
			Level:  "debug", // This should be overridden
			Output: "stdout",
		}

		err = configurator.ConfigureFromAppLogging(appConfig)
		assert.NoError(t, err)

		config := configurator.GetCurrentConfig()
		// Should use environment file but level 0 since GST_DEBUG not set
		assert.Equal(t, envLogFile, config.OutputFile)
		assert.Equal(t, 0, config.Level)
		assert.False(t, config.Enabled)
	})

	t.Run("No_Environment_Variables", func(t *testing.T) {
		// Ensure no environment variables are set
		os.Unsetenv("GST_DEBUG")
		os.Unsetenv("GST_DEBUG_FILE")

		appConfig := &config.LoggingConfig{
			Level:  "debug",
			Output: "stdout",
		}

		err := configurator.ConfigureFromAppLogging(appConfig)
		assert.NoError(t, err)

		config := configurator.GetCurrentConfig()
		// Should use application configuration
		assert.True(t, config.Enabled)
		assert.Equal(t, 4, config.Level)
		assert.Empty(t, config.OutputFile)
	})

	t.Run("Complex_GST_DEBUG_Format", func(t *testing.T) {
		// Test complex GST_DEBUG format with categories
		os.Setenv("GST_DEBUG", "GST_PIPELINE:4,GST_ELEMENT:3,*:2")
		defer os.Unsetenv("GST_DEBUG")

		appConfig := &config.LoggingConfig{
			Level:  "info", // This should be overridden
			Output: "stdout",
		}

		err := configurator.ConfigureFromAppLogging(appConfig)
		assert.NoError(t, err)

		config := configurator.GetCurrentConfig()
		assert.True(t, config.Enabled)
		assert.Equal(t, 2, config.Level) // Should use global level from *:2
	})

	t.Run("Invalid_GST_DEBUG_Format", func(t *testing.T) {
		// Test invalid GST_DEBUG format
		os.Setenv("GST_DEBUG", "invalid_format")
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
}

func TestGStreamerLogConfigurator_LogLevelMapping(t *testing.T) {
	logger := logrus.NewEntry(logrus.New())
	configurator := NewGStreamerLogConfigurator(logger).(*gstreamerLogConfigurator)

	testCases := []struct {
		appLevel     string
		expectedGst  int
		shouldEnable bool
	}{
		{"trace", 9, true},
		{"debug", 4, true},
		{"info", 0, false},
		{"warn", 0, false},
		{"error", 0, false},
		{"unknown", 0, false},
	}

	for _, tc := range testCases {
		t.Run(tc.appLevel, func(t *testing.T) {
			gstLevel := configurator.mapAppLogLevelToGStreamerLevel(tc.appLevel)
			assert.Equal(t, tc.expectedGst, gstLevel)

			shouldEnable := configurator.shouldEnableGStreamerLogging(tc.appLevel)
			assert.Equal(t, tc.shouldEnable, shouldEnable)
		})
	}
}

func TestGStreamerLogConfigurator_LogFileGeneration(t *testing.T) {
	logger := logrus.NewEntry(logrus.New())
	configurator := NewGStreamerLogConfigurator(logger).(*gstreamerLogConfigurator)

	t.Run("ConsoleOutput", func(t *testing.T) {
		appConfig := &config.LoggingConfig{
			Output: "stdout",
		}

		logFile := configurator.generateGStreamerLogFile(appConfig)
		assert.Empty(t, logFile)
	})

	t.Run("FileOutput", func(t *testing.T) {
		appConfig := &config.LoggingConfig{
			Output: "file",
			File:   "/var/log/app/application.log",
		}

		logFile := configurator.generateGStreamerLogFile(appConfig)
		assert.Equal(t, "/var/log/app/gstreamer.log", logFile)
	})
}

func TestParseGstDebugLevel(t *testing.T) {
	testCases := []struct {
		input    string
		expected int
		hasError bool
	}{
		{"4", 4, false},
		{"0", 0, false},
		{"9", 9, false},
		{"*:5", 5, false},
		{"GST_PIPELINE:4,*:3", 3, false},
		{"GST_PIPELINE:4", 4, false}, // Default when no global level
		{"invalid", 0, true},
		{"", 0, true},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			result, err := parseGstDebugLevel(tc.input)
			if tc.hasError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expected, result)
			}
		})
	}
}
