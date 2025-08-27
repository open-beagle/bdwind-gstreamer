package gstreamer

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/open-beagle/bdwind-gstreamer/internal/config"
)

func TestManager_UpdateLoggingConfig(t *testing.T) {
	// 创建临时目录用于测试
	tempDir, err := os.MkdirTemp("", "gstreamer_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// 创建测试管理器
	manager := createTestManager(t)
	defer manager.Stop(context.Background())

	// 测试用例
	tests := []struct {
		name      string
		config    *config.LoggingConfig
		expectErr bool
	}{
		{
			name: "valid debug config",
			config: &config.LoggingConfig{
				Level:           "debug",
				Format:          "text",
				Output:          "file",
				File:            filepath.Join(tempDir, "app.log"),
				EnableTimestamp: true,
				EnableCaller:    false,
				EnableColors:    true,
			},
			expectErr: false,
		},
		{
			name: "valid info config",
			config: &config.LoggingConfig{
				Level:           "info",
				Format:          "text",
				Output:          "stdout",
				EnableTimestamp: true,
				EnableCaller:    false,
				EnableColors:    false,
			},
			expectErr: false,
		},
		{
			name:      "invalid config - nil",
			config:    nil,
			expectErr: true,
		},
		{
			name: "invalid config - bad level",
			config: &config.LoggingConfig{
				Level:  "invalid",
				Output: "stdout",
			},
			expectErr: true,
		},
		{
			name: "invalid config - bad output",
			config: &config.LoggingConfig{
				Level:  "debug",
				Output: "invalid",
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := manager.UpdateLoggingConfig(tt.config)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)

				// 验证配置是否正确应用
				if tt.config != nil {
					currentConfig := manager.GetLoggingConfig()
					assert.NotNil(t, currentConfig)

					// 验证启用状态
					expectedEnabled := tt.config.Level == "debug" || tt.config.Level == "trace"
					assert.Equal(t, expectedEnabled, currentConfig.Enabled)

					// 验证日志级别
					expectedLevel := manager.mapAppLogLevelToGStreamerLevel(tt.config.Level)
					assert.Equal(t, expectedLevel, currentConfig.Level)
				}
			}
		})
	}
}

func TestManager_ValidateLoggingConfig(t *testing.T) {
	manager := createTestManager(t)
	defer manager.Stop(context.Background())

	tests := []struct {
		name      string
		config    *GStreamerLogConfig
		expectErr bool
	}{
		{
			name: "valid config",
			config: &GStreamerLogConfig{
				Enabled:    true,
				Level:      4,
				OutputFile: "",
				Categories: map[string]int{"GST_PIPELINE": 3},
				Colored:    true,
			},
			expectErr: false,
		},
		{
			name:      "nil config",
			config:    nil,
			expectErr: true,
		},
		{
			name: "invalid level - too low",
			config: &GStreamerLogConfig{
				Level: -1,
			},
			expectErr: true,
		},
		{
			name: "invalid level - too high",
			config: &GStreamerLogConfig{
				Level: 10,
			},
			expectErr: true,
		},
		{
			name: "invalid category level",
			config: &GStreamerLogConfig{
				Level:      4,
				Categories: map[string]int{"GST_PIPELINE": 10},
			},
			expectErr: true,
		},
		{
			name: "empty category name",
			config: &GStreamerLogConfig{
				Level:      4,
				Categories: map[string]int{"": 3},
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := manager.ValidateLoggingConfig(tt.config)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestManager_SyncLoggingWithApp(t *testing.T) {
	// 创建临时目录用于测试
	tempDir, err := os.MkdirTemp("", "gstreamer_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	manager := createTestManager(t)
	defer manager.Stop(context.Background())

	// 初始配置
	initialConfig := &config.LoggingConfig{
		Level:  "info",
		Output: "stdout",
	}
	err = manager.ConfigureLogging(initialConfig)
	require.NoError(t, err)

	// 新配置
	newConfig := &config.LoggingConfig{
		Level:           "debug",
		Format:          "text",
		Output:          "file",
		File:            filepath.Join(tempDir, "app.log"),
		EnableTimestamp: true,
		EnableCaller:    false,
		EnableColors:    true,
	}

	// 同步配置
	err = manager.SyncLoggingWithApp(newConfig)
	assert.NoError(t, err)

	// 验证配置已更新
	currentConfig := manager.GetLoggingConfig()
	assert.NotNil(t, currentConfig)
	assert.True(t, currentConfig.Enabled)
	assert.Equal(t, 4, currentConfig.Level) // debug level maps to 4
	assert.Equal(t, filepath.Join(tempDir, "gstreamer.log"), currentConfig.OutputFile)
}

func TestManager_DetectConfigChanges(t *testing.T) {
	manager := createTestManager(t)
	defer manager.Stop(context.Background())

	// 当前配置
	currentConfig := &GStreamerLogConfig{
		Enabled:    false,
		Level:      0,
		OutputFile: "",
		Colored:    true,
	}

	// 新的应用程序配置
	appConfig := &config.LoggingConfig{
		Level:        "debug",
		Output:       "stdout",
		EnableColors: true,
	}

	needsUpdate, changes := manager.detectConfigChanges(appConfig, currentConfig)
	assert.True(t, needsUpdate)
	assert.NotEmpty(t, changes)

	// 验证检测到的变化
	assert.Contains(t, changes[0], "enabled: false -> true")
	assert.Contains(t, changes[1], "level: 0 -> 4")
}

func TestManager_MonitorLoggingConfigChanges(t *testing.T) {
	manager := createTestManager(t)
	defer manager.Stop(context.Background())

	// 启动管理器以初始化监控
	ctx := context.Background()
	err := manager.Start(ctx)
	require.NoError(t, err)

	// 发送配置变化
	newConfig := &config.LoggingConfig{
		Level:  "debug",
		Output: "stdout",
	}

	manager.NotifyLoggingConfigChange(newConfig)

	// 等待配置处理
	time.Sleep(100 * time.Millisecond)

	// 验证配置已更新
	currentConfig := manager.GetLoggingConfig()
	assert.NotNil(t, currentConfig)
	assert.True(t, currentConfig.Enabled)
}

func TestManager_ValidateLogFilePath(t *testing.T) {
	// 创建临时目录用于测试
	tempDir, err := os.MkdirTemp("", "gstreamer_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	manager := createTestManager(t)
	defer manager.Stop(context.Background())

	tests := []struct {
		name      string
		filePath  string
		expectErr bool
	}{
		{
			name:      "empty path (console output)",
			filePath:  "",
			expectErr: false,
		},
		{
			name:      "valid absolute path",
			filePath:  filepath.Join(tempDir, "test.log"),
			expectErr: false,
		},
		{
			name:      "relative path",
			filePath:  "relative/path.log",
			expectErr: true,
		},
		{
			name:      "invalid directory",
			filePath:  "/invalid/nonexistent/deep/path/test.log",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := manager.validateLogFilePath(tt.filePath)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// createTestManager 创建用于测试的管理器实例
func createTestManager(t *testing.T) *Manager {
	// 使用默认配置以确保所有必需字段都设置正确
	captureConfig := config.DefaultDesktopCaptureConfig()
	encoderConfig := config.DefaultEncoderConfig()

	// 创建测试配置
	gstConfig := &config.GStreamerConfig{
		Capture:  captureConfig,
		Encoding: encoderConfig,
	}

	cfg := &ManagerConfig{
		Config: gstConfig,
	}

	// 设置测试日志级别
	logrus.SetLevel(logrus.DebugLevel)

	ctx := context.Background()
	manager, err := NewManager(ctx, cfg)
	require.NoError(t, err)
	require.NotNil(t, manager)

	return manager
}
