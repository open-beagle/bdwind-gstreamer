package gstreamer

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/open-beagle/bdwind-gstreamer/internal/config"
)

func TestManager_LoggingIntegration(t *testing.T) {
	// Use default GStreamer config for testing to ensure all required fields are set
	gstConfig := config.DefaultGStreamerConfig()

	managerConfig := &ManagerConfig{
		Config: gstConfig,
	}

	ctx := context.Background()

	t.Run("NewManager_InitializesLogConfigurator", func(t *testing.T) {
		manager, err := NewManager(ctx, managerConfig)
		require.NoError(t, err)
		defer manager.Stop(ctx)

		// Verify log configurator is initialized
		assert.NotNil(t, manager.logConfigurator)
		assert.NotNil(t, manager.GetLoggingConfig())
	})

	t.Run("ConfigureLogging_DebugLevel", func(t *testing.T) {
		manager, err := NewManager(ctx, managerConfig)
		require.NoError(t, err)
		defer manager.Stop(ctx)

		appConfig := &config.LoggingConfig{
			Level:  "debug",
			Output: "stdout",
		}

		err = manager.ConfigureLogging(appConfig)
		assert.NoError(t, err)

		logConfig := manager.GetLoggingConfig()
		assert.NotNil(t, logConfig)
		assert.True(t, logConfig.Enabled)
		assert.Equal(t, 4, logConfig.Level)
		assert.Empty(t, logConfig.OutputFile)
	})

	t.Run("ConfigureLogging_InfoLevel", func(t *testing.T) {
		manager, err := NewManager(ctx, managerConfig)
		require.NoError(t, err)
		defer manager.Stop(ctx)

		appConfig := &config.LoggingConfig{
			Level:  "info",
			Output: "stdout",
		}

		err = manager.ConfigureLogging(appConfig)
		assert.NoError(t, err)

		logConfig := manager.GetLoggingConfig()
		assert.NotNil(t, logConfig)
		assert.False(t, logConfig.Enabled)
		assert.Equal(t, 0, logConfig.Level)
	})

	t.Run("ConfigureLogging_FileOutput", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "manager_logging_test")
		require.NoError(t, err)
		defer os.RemoveAll(tempDir)

		manager, err := NewManager(ctx, managerConfig)
		require.NoError(t, err)
		defer manager.Stop(ctx)

		appLogFile := filepath.Join(tempDir, "app.log")
		expectedGstLogFile := filepath.Join(tempDir, "gstreamer.log")

		appConfig := &config.LoggingConfig{
			Level:  "debug",
			Output: "file",
			File:   appLogFile,
		}

		err = manager.ConfigureLogging(appConfig)
		assert.NoError(t, err)

		logConfig := manager.GetLoggingConfig()
		assert.NotNil(t, logConfig)
		assert.True(t, logConfig.Enabled)
		assert.Equal(t, expectedGstLogFile, logConfig.OutputFile)
	})

	t.Run("UpdateLoggingConfig", func(t *testing.T) {
		manager, err := NewManager(ctx, managerConfig)
		require.NoError(t, err)
		defer manager.Stop(ctx)

		// Initial configuration
		initialConfig := &config.LoggingConfig{
			Level:  "info",
			Output: "stdout",
		}

		err = manager.ConfigureLogging(initialConfig)
		assert.NoError(t, err)

		initialLogConfig := manager.GetLoggingConfig()
		assert.False(t, initialLogConfig.Enabled)

		// Update configuration
		updatedConfig := &config.LoggingConfig{
			Level:  "debug",
			Output: "stdout",
		}

		err = manager.UpdateLoggingConfig(updatedConfig)
		assert.NoError(t, err)

		updatedLogConfig := manager.GetLoggingConfig()
		assert.True(t, updatedLogConfig.Enabled)
		assert.Equal(t, 4, updatedLogConfig.Level)
	})

	t.Run("GetStats_IncludesLoggingConfig", func(t *testing.T) {
		manager, err := NewManager(ctx, managerConfig)
		require.NoError(t, err)
		defer manager.Stop(ctx)

		appConfig := &config.LoggingConfig{
			Level:  "debug",
			Output: "stdout",
		}

		err = manager.ConfigureLogging(appConfig)
		assert.NoError(t, err)

		// Start manager to get full stats
		err = manager.Start(ctx)
		assert.NoError(t, err)

		stats := manager.GetStats()
		assert.NotNil(t, stats)

		// Check if logging config is included in stats
		loggingStats, exists := stats["logging_config"]
		assert.True(t, exists)

		loggingStatsMap, ok := loggingStats.(map[string]interface{})
		assert.True(t, ok)
		assert.Equal(t, true, loggingStatsMap["enabled"])
		assert.Equal(t, 4, loggingStatsMap["level"])
	})

	t.Run("GetComponentStatus_IncludesLogConfigurator", func(t *testing.T) {
		manager, err := NewManager(ctx, managerConfig)
		require.NoError(t, err)
		defer manager.Stop(ctx)

		status := manager.GetComponentStatus()
		assert.NotNil(t, status)

		logConfiguratorStatus, exists := status["log_configurator"]
		assert.True(t, exists)
		assert.True(t, logConfiguratorStatus)
	})

	t.Run("ConfigureLogging_NilConfigurator", func(t *testing.T) {
		manager, err := NewManager(ctx, managerConfig)
		require.NoError(t, err)
		defer manager.Stop(ctx)

		// Simulate nil configurator
		manager.logConfigurator = nil

		appConfig := &config.LoggingConfig{
			Level:  "debug",
			Output: "stdout",
		}

		err = manager.ConfigureLogging(appConfig)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "log configurator not initialized")
	})

	t.Run("UpdateLoggingConfig_NilConfigurator", func(t *testing.T) {
		manager, err := NewManager(ctx, managerConfig)
		require.NoError(t, err)
		defer manager.Stop(ctx)

		// Simulate nil configurator
		manager.logConfigurator = nil

		appConfig := &config.LoggingConfig{
			Level:  "debug",
			Output: "stdout",
		}

		err = manager.UpdateLoggingConfig(appConfig)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "log configurator not initialized")
	})
}

func TestManager_LoggingIntegration_EnvironmentOverride(t *testing.T) {
	// Use default GStreamer config for testing to ensure all required fields are set
	gstConfig := config.DefaultGStreamerConfig()

	managerConfig := &ManagerConfig{
		Config: gstConfig,
	}

	ctx := context.Background()

	t.Run("GST_DEBUG_EnvironmentOverride", func(t *testing.T) {
		// Set environment variable
		os.Setenv("GST_DEBUG", "5")
		defer os.Unsetenv("GST_DEBUG")

		manager, err := NewManager(ctx, managerConfig)
		require.NoError(t, err)
		defer manager.Stop(ctx)

		appConfig := &config.LoggingConfig{
			Level:  "info", // This should be overridden by environment
			Output: "stdout",
		}

		err = manager.ConfigureLogging(appConfig)
		assert.NoError(t, err)

		logConfig := manager.GetLoggingConfig()
		assert.NotNil(t, logConfig)
		assert.True(t, logConfig.Enabled)
		assert.Equal(t, 5, logConfig.Level)
	})
}
