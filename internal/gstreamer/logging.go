package gstreamer

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"

	"github.com/open-beagle/bdwind-gstreamer/internal/config"
)

// GStreamer logging related errors
var (
	ErrGStreamerLogConfigFailed = errors.New("gstreamer: log configuration failed")
	ErrGStreamerLogLevelInvalid = errors.New("gstreamer: invalid log level")
	ErrGStreamerLogFileAccess   = errors.New("gstreamer: log file access error")
)

// GStreamerLogConfig GStreamer日志配置
type GStreamerLogConfig struct {
	// Enabled 是否启用GStreamer日志
	Enabled bool `json:"enabled"`

	// Level GStreamer日志级别 (0-9)
	Level int `json:"level"`

	// OutputFile 日志输出文件路径（空表示控制台输出）
	OutputFile string `json:"output_file"`

	// Categories 特定类别的日志级别配置
	Categories map[string]int `json:"categories"`

	// Colored 是否启用颜色输出
	Colored bool `json:"colored"`
}

// GStreamerLogConfigurator GStreamer日志配置器接口
type GStreamerLogConfigurator interface {
	// ConfigureFromAppLogging 根据应用程序日志配置设置GStreamer日志
	ConfigureFromAppLogging(appConfig *config.LoggingConfig) error

	// SetLogLevel 设置GStreamer日志级别
	SetLogLevel(level int) error

	// SetLogFile 设置GStreamer日志文件
	SetLogFile(filePath string) error

	// EnableCategory 启用特定类别的日志
	EnableCategory(category string, level int) error

	// DisableLogging 禁用GStreamer日志
	DisableLogging() error

	// GetCurrentConfig 获取当前配置
	GetCurrentConfig() *GStreamerLogConfig

	// UpdateConfig 更新配置
	UpdateConfig(config *GStreamerLogConfig) error
}

// gstreamerLogConfigurator GStreamer日志配置器实现
type gstreamerLogConfigurator struct {
	logger        *logrus.Entry
	currentConfig *GStreamerLogConfig
	mutex         sync.RWMutex
}

// NewGStreamerLogConfigurator 创建新的GStreamer日志配置器
func NewGStreamerLogConfigurator(logger *logrus.Entry) GStreamerLogConfigurator {
	return &gstreamerLogConfigurator{
		logger: logger,
		currentConfig: &GStreamerLogConfig{
			Enabled:    false,
			Level:      0,
			OutputFile: "",
			Categories: make(map[string]int),
			Colored:    true,
		},
	}
}

// ConfigureFromAppLogging 根据应用程序日志配置设置GStreamer日志
func (g *gstreamerLogConfigurator) ConfigureFromAppLogging(appConfig *config.LoggingConfig) error {
	g.mutex.Lock()
	defer g.mutex.Unlock()

	g.logger.Debug("Starting GStreamer logging configuration from application config")

	if appConfig == nil {
		g.logger.Error("Cannot configure GStreamer logging: application config is nil")
		return fmt.Errorf("%w: app config is nil", ErrGStreamerLogConfigFailed)
	}

	g.logger.Debugf("Application logging config: level=%s, output=%s, file=%s, colors=%t",
		appConfig.Level, appConfig.Output, appConfig.File, appConfig.EnableColors)

	// 检查环境变量覆盖
	envLevel := os.Getenv("GST_DEBUG")
	envFile := os.Getenv("GST_DEBUG_FILE")

	if envLevel != "" || envFile != "" {
		g.logger.Infof("GStreamer environment variables detected - GST_DEBUG: %q, GST_DEBUG_FILE: %q", envLevel, envFile)
		g.logger.Info("Environment variables will override application logging configuration")

		if err := g.configureFromEnvironment(); err != nil {
			g.logger.Errorf("Failed to configure GStreamer logging from environment variables: %v", err)
			return fmt.Errorf("failed to configure from environment: %w", err)
		}

		g.logger.Info("Successfully configured GStreamer logging from environment variables")
		return nil
	}

	g.logger.Debug("No GStreamer environment variables found, using application logging configuration")

	// 验证应用程序配置
	if err := g.validateAppConfig(appConfig); err != nil {
		g.logger.Warnf("Application logging configuration has issues: %v", err)
		// 继续处理，但记录警告
	}

	// 根据应用程序日志级别确定GStreamer日志配置
	gstConfig := &GStreamerLogConfig{
		Enabled:    g.shouldEnableGStreamerLogging(appConfig.Level),
		Level:      g.mapAppLogLevelToGStreamerLevel(appConfig.Level),
		OutputFile: g.generateGStreamerLogFile(appConfig),
		Categories: g.getDefaultCategories(appConfig.Level),
		Colored:    appConfig.EnableColors && (appConfig.Output != "file"),
	}

	g.logger.Debugf("Generated GStreamer config from app config: enabled=%t, level=%d (%s), file=%s, colored=%t",
		gstConfig.Enabled, gstConfig.Level, g.getLogLevelDescription(gstConfig.Level),
		gstConfig.OutputFile, gstConfig.Colored)

	// 应用配置到GStreamer
	if err := g.applyConfig(gstConfig); err != nil {
		g.logger.Errorf("Failed to apply GStreamer logging configuration: %v", err)
		return fmt.Errorf("failed to apply configuration: %w", err)
	}

	g.logger.Debug("Successfully configured GStreamer logging from application config")
	return nil
}

// validateAppConfig 验证应用程序配置
func (g *gstreamerLogConfigurator) validateAppConfig(appConfig *config.LoggingConfig) error {
	var issues []string

	// 验证日志级别
	validLevels := []string{"trace", "debug", "info", "warn", "error"}
	levelValid := false
	for _, validLevel := range validLevels {
		if strings.ToLower(appConfig.Level) == validLevel {
			levelValid = true
			break
		}
	}
	if !levelValid {
		issues = append(issues, fmt.Sprintf("unknown log level: %s", appConfig.Level))
	}

	// 验证输出类型
	if appConfig.Output != "stdout" && appConfig.Output != "stderr" && appConfig.Output != "file" {
		issues = append(issues, fmt.Sprintf("unknown output type: %s", appConfig.Output))
	}

	// 如果输出是文件，验证文件路径
	if appConfig.Output == "file" {
		if appConfig.File == "" {
			issues = append(issues, "file output specified but no file path provided")
		}
		// 注意：不再强制要求绝对路径，因为我们会在后续处理中规范化路径
	}

	if len(issues) > 0 {
		return fmt.Errorf("configuration issues: %s", strings.Join(issues, "; "))
	}

	return nil
}

// SetLogLevel 设置GStreamer日志级别
func (g *gstreamerLogConfigurator) SetLogLevel(level int) error {
	g.mutex.Lock()
	defer g.mutex.Unlock()

	g.logger.Debugf("Setting GStreamer log level to %d", level)

	if level < 0 || level > 9 {
		g.logger.Errorf("Invalid GStreamer log level: %d (must be between 0 and 9)", level)
		return fmt.Errorf("%w: level must be between 0 and 9, got %d", ErrGStreamerLogLevelInvalid, level)
	}

	oldLevel := g.currentConfig.Level
	oldEnabled := g.currentConfig.Enabled

	g.currentConfig.Level = level
	g.currentConfig.Enabled = level > 0

	if err := g.applyLogLevel(level); err != nil {
		// 回滚配置
		g.currentConfig.Level = oldLevel
		g.currentConfig.Enabled = oldEnabled
		g.logger.Errorf("Failed to set GStreamer log level, rolled back to previous level %d: %v", oldLevel, err)
		return fmt.Errorf("failed to set log level: %w", err)
	}

	g.logger.Infof("Successfully updated GStreamer log level: %d->%d (%s), enabled: %t->%t",
		oldLevel, level, g.getLogLevelDescription(level), oldEnabled, g.currentConfig.Enabled)

	return nil
}

// SetLogFile 设置GStreamer日志文件
func (g *gstreamerLogConfigurator) SetLogFile(filePath string) error {
	g.mutex.Lock()
	defer g.mutex.Unlock()

	if filePath == "" {
		g.logger.Debug("Setting GStreamer log output to console")
	} else {
		g.logger.Debugf("Setting GStreamer log file to: %s", filePath)
	}

	oldOutputFile := g.currentConfig.OutputFile

	// 验证新的文件路径
	if filePath != "" {
		if err := g.validateLogFilePath(filePath); err != nil {
			g.logger.Errorf("Invalid GStreamer log file path %s: %v", filePath, err)
			return fmt.Errorf("invalid log file path: %w", err)
		}
	}

	g.currentConfig.OutputFile = filePath

	if err := g.applyLogFile(filePath); err != nil {
		// 回滚配置
		g.currentConfig.OutputFile = oldOutputFile
		g.logger.Errorf("Failed to set GStreamer log file, rolled back to previous file %s: %v", oldOutputFile, err)
		return fmt.Errorf("failed to set log file: %w", err)
	}

	g.logger.Infof("Successfully updated GStreamer log output: %s -> %s",
		g.getOutputDescription(oldOutputFile), g.getOutputDescription(filePath))

	return nil
}

// EnableCategory 启用特定类别的日志
func (g *gstreamerLogConfigurator) EnableCategory(category string, level int) error {
	g.mutex.Lock()
	defer g.mutex.Unlock()

	g.logger.Debugf("Enabling GStreamer category %s at level %d", category, level)

	if category == "" {
		g.logger.Error("Cannot enable GStreamer category: category name is empty")
		return fmt.Errorf("category name cannot be empty")
	}

	if level < 0 || level > 9 {
		g.logger.Errorf("Invalid level %d for GStreamer category %s (must be between 0 and 9)", level, category)
		return fmt.Errorf("%w: level must be between 0 and 9, got %d", ErrGStreamerLogLevelInvalid, level)
	}

	if g.currentConfig.Categories == nil {
		g.currentConfig.Categories = make(map[string]int)
	}

	oldLevel, existed := g.currentConfig.Categories[category]
	g.currentConfig.Categories[category] = level

	// Apply category-specific logging (would need C binding implementation)
	// For now, we just log the configuration
	if existed {
		g.logger.Infof("Updated GStreamer category %s: level %d->%d (%s)",
			category, oldLevel, level, g.getLogLevelDescription(level))
	} else {
		g.logger.Infof("Enabled GStreamer category %s at level %d (%s)",
			category, level, g.getLogLevelDescription(level))
	}

	// 如果全局日志被禁用，警告用户类别设置可能无效
	if !g.currentConfig.Enabled {
		g.logger.Warnf("GStreamer category %s enabled but global logging is disabled, category may not produce output", category)
	}

	return nil
}

// DisableLogging 禁用GStreamer日志
func (g *gstreamerLogConfigurator) DisableLogging() error {
	g.mutex.Lock()
	defer g.mutex.Unlock()

	g.logger.Debug("Disabling GStreamer logging")

	oldEnabled := g.currentConfig.Enabled
	oldLevel := g.currentConfig.Level

	g.currentConfig.Enabled = false
	g.currentConfig.Level = 0

	if err := g.applyLogLevel(0); err != nil {
		// 回滚配置
		g.currentConfig.Enabled = oldEnabled
		g.currentConfig.Level = oldLevel
		g.logger.Errorf("Failed to disable GStreamer logging, rolled back to previous state: %v", err)
		return fmt.Errorf("failed to disable logging: %w", err)
	}

	g.logger.Infof("Successfully disabled GStreamer logging (was: enabled=%t, level=%d)", oldEnabled, oldLevel)

	// 清理类别配置
	if len(g.currentConfig.Categories) > 0 {
		g.logger.Debugf("Cleared %d category-specific log configurations", len(g.currentConfig.Categories))
		g.currentConfig.Categories = make(map[string]int)
	}

	return nil
}

// GetCurrentConfig 获取当前配置
func (g *gstreamerLogConfigurator) GetCurrentConfig() *GStreamerLogConfig {
	g.mutex.RLock()
	defer g.mutex.RUnlock()

	// Return a copy to prevent external modification
	config := *g.currentConfig
	if g.currentConfig.Categories != nil {
		config.Categories = make(map[string]int)
		for k, v := range g.currentConfig.Categories {
			config.Categories[k] = v
		}
	}

	return &config
}

// UpdateConfig 更新配置
func (g *gstreamerLogConfigurator) UpdateConfig(config *GStreamerLogConfig) error {
	g.mutex.Lock()
	defer g.mutex.Unlock()

	g.logger.Debug("Updating GStreamer logging configuration")

	if config == nil {
		g.logger.Error("Cannot update GStreamer config: provided config is nil")
		return fmt.Errorf("%w: config is nil", ErrGStreamerLogConfigFailed)
	}

	// 记录配置变化
	oldConfig := g.currentConfig
	g.logger.Debugf("Updating GStreamer config: enabled %t->%t, level %d->%d, file %s->%s, colored %t->%t",
		oldConfig.Enabled, config.Enabled,
		oldConfig.Level, config.Level,
		oldConfig.OutputFile, config.OutputFile,
		oldConfig.Colored, config.Colored)

	// 验证新配置
	if err := g.validateConfig(config); err != nil {
		g.logger.Errorf("New GStreamer configuration is invalid: %v", err)
		return fmt.Errorf("invalid configuration: %w", err)
	}

	// 应用新配置
	if err := g.applyConfig(config); err != nil {
		g.logger.Errorf("Failed to apply new GStreamer configuration: %v", err)
		return fmt.Errorf("failed to apply configuration: %w", err)
	}

	g.logger.Info("Successfully updated GStreamer logging configuration")
	return nil
}

// shouldEnableGStreamerLogging 判断是否应该启用GStreamer日志
func (g *gstreamerLogConfigurator) shouldEnableGStreamerLogging(appLogLevel string) bool {
	level := strings.ToLower(strings.TrimSpace(appLogLevel))
	return level == "debug" || level == "trace"
}

// mapAppLogLevelToGStreamerLevel 将应用程序日志级别映射到GStreamer日志级别
func (g *gstreamerLogConfigurator) mapAppLogLevelToGStreamerLevel(appLogLevel string) int {
	level := strings.ToLower(strings.TrimSpace(appLogLevel))

	switch level {
	case "trace":
		return 9 // GST_LEVEL_LOG - 最详细的日志
	case "debug":
		return 4 // GST_LEVEL_INFO - 调试信息
	case "info", "warn", "error":
		return 0 // GST_LEVEL_NONE - 禁用GStreamer日志
	default:
		return 0 // 默认禁用
	}
}

// generateGStreamerLogFile 生成GStreamer日志文件路径
func (g *gstreamerLogConfigurator) generateGStreamerLogFile(appConfig *config.LoggingConfig) string {
	if appConfig.Output != "file" || appConfig.File == "" {
		return "" // 控制台输出
	}

	// 确保应用程序日志文件路径是绝对路径
	appLogFile := appConfig.File
	if !filepath.IsAbs(appLogFile) {
		// 如果是相对路径，转换为绝对路径
		if workDir, err := os.Getwd(); err == nil {
			appLogFile = filepath.Join(workDir, appLogFile)
		}
	}

	// 在应用程序日志目录创建gstreamer.log
	dir := filepath.Dir(appLogFile)
	gstreamerLogFile := filepath.Join(dir, "gstreamer.log")

	g.logger.Debugf("Generated GStreamer log file path: %s (from app log: %s)", gstreamerLogFile, appConfig.File)

	return gstreamerLogFile
}

// getDefaultCategories 获取默认的类别配置
func (g *gstreamerLogConfigurator) getDefaultCategories(appLogLevel string) map[string]int {
	categories := make(map[string]int)

	level := strings.ToLower(strings.TrimSpace(appLogLevel))
	if level == "trace" {
		// Trace级别启用更多详细类别
		categories["GST_ELEMENT_FACTORY"] = 3
		categories["GST_PIPELINE"] = 4
		categories["GST_PADS"] = 3
		categories["GST_BUFFER"] = 2
	} else if level == "debug" {
		// Debug级别启用基本类别
		categories["GST_ELEMENT_FACTORY"] = 2
		categories["GST_PIPELINE"] = 3
	}

	return categories
}

// configureFromEnvironment 从环境变量配置GStreamer日志
func (g *gstreamerLogConfigurator) configureFromEnvironment() error {
	envLevel := os.Getenv("GST_DEBUG")
	envFile := os.Getenv("GST_DEBUG_FILE")

	g.logger.Debug("Configuring GStreamer logging from environment variables")
	g.logger.Debugf("Environment variables: GST_DEBUG=%q, GST_DEBUG_FILE=%q", envLevel, envFile)

	// 解析GST_DEBUG级别
	level := 0
	if envLevel != "" {
		g.logger.Debugf("Processing GST_DEBUG environment variable: %q", envLevel)

		if parsed, err := parseGstDebugLevel(envLevel); err == nil {
			level = parsed
			g.logger.Debugf("Successfully parsed GST_DEBUG level: %d (%s)", level, g.getLogLevelDescription(level))
		} else {
			g.logger.Warnf("Failed to parse GST_DEBUG value %q: %v", envLevel, err)
			g.logger.Warnf("Using default level 0 (no debug output)")
			level = 0
		}
	} else {
		g.logger.Debug("GST_DEBUG environment variable not set, using level 0")
	}

	// 处理GST_DEBUG_FILE
	var fileValidationError error
	if envFile != "" {
		g.logger.Debugf("Processing GST_DEBUG_FILE environment variable: %q", envFile)

		// 验证环境变量指定的文件路径
		if err := g.validateLogFilePath(envFile); err != nil {
			g.logger.Warnf("GST_DEBUG_FILE path validation failed: %v", err)
			fileValidationError = err
			// 继续处理，但记录错误，稍后可能需要降级
		} else {
			g.logger.Debugf("GST_DEBUG_FILE path validation passed: %q", envFile)
		}
	} else {
		g.logger.Debug("GST_DEBUG_FILE environment variable not set, using console output")
	}

	// 确定颜色输出设置
	colored := true
	if envFile != "" {
		// 如果输出到文件，通常不需要颜色
		colored = false
		g.logger.Debug("Disabling colored output for file-based logging")
	} else {
		g.logger.Debug("Enabling colored output for console logging")
	}

	gstConfig := &GStreamerLogConfig{
		Enabled:    level > 0,
		Level:      level,
		OutputFile: envFile,
		Categories: make(map[string]int),
		Colored:    colored,
	}

	g.logger.Debugf("Generated GStreamer config from environment: enabled=%t, level=%d (%s), file=%q, colored=%t",
		gstConfig.Enabled, gstConfig.Level, g.getLogLevelDescription(gstConfig.Level),
		gstConfig.OutputFile, gstConfig.Colored)

	// 应用配置，如果文件验证失败，applyConfig会处理降级
	if err := g.applyConfig(gstConfig); err != nil {
		g.logger.Errorf("Failed to apply GStreamer config from environment variables: %v", err)

		// 如果是文件相关错误且我们之前检测到文件问题，提供更详细的错误信息
		if fileValidationError != nil {
			return fmt.Errorf("failed to apply environment configuration, file validation error: %w, apply error: %v", fileValidationError, err)
		}

		return fmt.Errorf("failed to apply environment configuration: %w", err)
	}

	g.logger.Debug("Successfully configured GStreamer logging from environment variables")
	return nil
}

// applyConfig 应用配置到GStreamer
func (g *gstreamerLogConfigurator) applyConfig(config *GStreamerLogConfig) error {
	if config == nil {
		g.logger.Error("Cannot apply GStreamer log config: config is nil")
		return fmt.Errorf("%w: config is nil", ErrGStreamerLogConfigFailed)
	}

	g.logger.Debugf("Applying GStreamer log configuration: enabled=%t, level=%d, file=%q, colored=%t",
		config.Enabled, config.Level, config.OutputFile, config.Colored)

	// 验证配置
	if err := g.validateConfig(config); err != nil {
		g.logger.Warnf("GStreamer log configuration validation failed: %v", err)
		return fmt.Errorf("%w: %v", ErrGStreamerLogConfigFailed, err)
	}

	g.logger.Debug("GStreamer log configuration validation passed")

	// 应用日志级别
	if err := g.applyLogLevel(config.Level); err != nil {
		g.logger.Errorf("Failed to apply GStreamer log level %d: %v", config.Level, err)
		return fmt.Errorf("failed to apply log level: %w", err)
	}

	g.logger.Debugf("Successfully applied GStreamer log level: %d", config.Level)

	// 应用日志文件
	originalOutputFile := config.OutputFile
	if err := g.applyLogFile(config.OutputFile); err != nil {
		g.logger.Warnf("Failed to set GStreamer log file to %q: %v", config.OutputFile, err)
		g.logger.Info("Falling back to console output for GStreamer logs")

		// 降级到控制台输出
		if fallbackErr := g.applyLogFile(""); fallbackErr != nil {
			g.logger.Errorf("Failed to fallback to console output: %v", fallbackErr)
			return fmt.Errorf("failed to set log file and fallback failed: original error: %w, fallback error: %v", err, fallbackErr)
		}

		config.OutputFile = ""
		g.logger.Debugf("Successfully fell back to console output (original file: %q)", originalOutputFile)
	} else if config.OutputFile != "" {
		g.logger.Debugf("Successfully set GStreamer log file: %q", config.OutputFile)
	} else {
		g.logger.Debug("GStreamer logging configured for console output")
	}

	// 应用颜色设置
	g.applyColoredOutput(config.Colored)
	g.logger.Debugf("Applied GStreamer colored output setting: %t", config.Colored)

	// 更新当前配置
	g.currentConfig = config

	g.logger.Debugf("Successfully applied GStreamer logging configuration: enabled=%t, level=%d, output=%s",
		config.Enabled, config.Level, g.getOutputDescription(config.OutputFile))

	return nil
}

// validateConfig 验证配置
func (g *gstreamerLogConfigurator) validateConfig(config *GStreamerLogConfig) error {
	// 验证日志级别
	if config.Level < 0 || config.Level > 9 {
		g.logger.Warnf("Invalid GStreamer log level: %d (must be between 0 and 9)", config.Level)
		return fmt.Errorf("invalid log level: %d, must be between 0 and 9", config.Level)
	}

	// 验证启用状态与级别的一致性
	if config.Enabled && config.Level == 0 {
		g.logger.Warnf("GStreamer logging is enabled but level is 0, this may not produce any output")
	}

	if !config.Enabled && config.Level > 0 {
		g.logger.Warnf("GStreamer logging is disabled but level is %d, level will be ignored", config.Level)
	}

	// 如果指定了日志文件，检查目录是否存在
	if config.OutputFile != "" {
		dir := filepath.Dir(config.OutputFile)
		g.logger.Debugf("Validating GStreamer log directory: %s", dir)

		if _, err := os.Stat(dir); os.IsNotExist(err) {
			g.logger.Debugf("GStreamer log directory does not exist, attempting to create: %s", dir)
			// 尝试创建目录
			if err := os.MkdirAll(dir, 0755); err != nil {
				g.logger.Errorf("Failed to create GStreamer log directory %s: %v", dir, err)
				return fmt.Errorf("cannot create log directory %s: %w", dir, err)
			}
			g.logger.Debugf("Successfully created GStreamer log directory: %s", dir)
		} else if err != nil {
			g.logger.Errorf("Error checking GStreamer log directory %s: %v", dir, err)
			return fmt.Errorf("cannot access log directory %s: %w", dir, err)
		} else {
			g.logger.Debugf("GStreamer log directory exists and is accessible: %s", dir)
		}

		// 验证文件路径是否可写
		if err := g.validateLogFileWritable(config.OutputFile); err != nil {
			g.logger.Warnf("GStreamer log file may not be writable: %v", err)
			// 不返回错误，让后续的文件操作处理
		}
	}

	// 验证类别配置
	for category, level := range config.Categories {
		if level < 0 || level > 9 {
			g.logger.Warnf("Invalid log level %d for category %s (must be between 0 and 9)", level, category)
			return fmt.Errorf("invalid log level %d for category %s, must be between 0 and 9", level, category)
		}
	}

	g.logger.Debugf("GStreamer log configuration validation completed successfully")
	return nil
}

// validateLogFileWritable 验证日志文件是否可写
func (g *gstreamerLogConfigurator) validateLogFileWritable(filePath string) error {
	// 尝试创建或打开文件进行写入测试
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("cannot open log file for writing: %w", err)
	}
	defer file.Close()

	// 尝试写入一个测试字符串
	testData := []byte("# GStreamer log file validation test\n")
	if _, err := file.Write(testData); err != nil {
		return fmt.Errorf("cannot write to log file: %w", err)
	}

	return nil
}

// testFileWritability 测试文件是否可写（用于实际应用配置时）
func (g *gstreamerLogConfigurator) testFileWritability(filePath string) error {
	// 尝试创建或打开文件进行写入测试
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("cannot open file for writing: %w", err)
	}
	defer file.Close()

	// 尝试写入一个测试字符串
	testData := []byte("# GStreamer log file test\n")
	if _, err := file.Write(testData); err != nil {
		return fmt.Errorf("cannot write to file: %w", err)
	}

	return nil
}

// getOutputDescription 获取输出描述
func (g *gstreamerLogConfigurator) getOutputDescription(outputFile string) string {
	if outputFile == "" {
		return "console"
	}
	return fmt.Sprintf("file(%s)", outputFile)
}

// applyLogLevel 应用日志级别到GStreamer
func (g *gstreamerLogConfigurator) applyLogLevel(level int) error {
	g.logger.Debugf("Applying GStreamer log level: %d", level)

	// 验证级别范围
	if level < 0 || level > 9 {
		g.logger.Errorf("Invalid GStreamer log level: %d (must be 0-9)", level)
		return fmt.Errorf("%w: level %d is out of range (0-9)", ErrGStreamerLogLevelInvalid, level)
	}

	// 记录级别含义
	levelDescription := g.getLogLevelDescription(level)
	g.logger.Debugf("Setting GStreamer log level to %d (%s)", level, levelDescription)

	// 调用go-gst函数设置GStreamer日志级别
	// 注意：这里假设函数不会失败，但在实际实现中可能需要错误检查
	setDebugLevel(level)

	g.logger.Debugf("Successfully applied GStreamer log level: %d (%s)", level, levelDescription)
	return nil
}

// applyLogFile 应用日志文件到GStreamer
func (g *gstreamerLogConfigurator) applyLogFile(filePath string) error {
	if filePath == "" {
		g.logger.Debug("Setting GStreamer log output to console")
	} else {
		g.logger.Debugf("Setting GStreamer log file to: %s", filePath)

		// 验证文件路径
		if err := g.validateLogFilePath(filePath); err != nil {
			g.logger.Errorf("Invalid GStreamer log file path %s: %v", filePath, err)
			return fmt.Errorf("%w: %v", ErrGStreamerLogFileAccess, err)
		}

		// 尝试实际创建/写入文件以确保可行性
		if err := g.testFileWritability(filePath); err != nil {
			g.logger.Errorf("Cannot write to GStreamer log file %s: %v", filePath, err)
			return fmt.Errorf("%w: cannot write to file: %v", ErrGStreamerLogFileAccess, err)
		}
	}

	// 调用go-gst函数设置GStreamer日志文件
	// 注意：这里假设函数不会失败，但在实际实现中可能需要错误检查
	setLogFile(filePath)

	if filePath == "" {
		g.logger.Debug("Successfully configured GStreamer log output to console")
	} else {
		g.logger.Debugf("Successfully configured GStreamer log file: %s", filePath)
	}

	return nil
}

// validateLogFilePath 验证日志文件路径
func (g *gstreamerLogConfigurator) validateLogFilePath(filePath string) error {
	if filePath == "" {
		return nil // 空路径表示控制台输出，这是有效的
	}

	// 规范化路径：如果是相对路径，转换为绝对路径
	normalizedPath := filePath
	if !filepath.IsAbs(filePath) {
		if workDir, err := os.Getwd(); err == nil {
			normalizedPath = filepath.Join(workDir, filePath)
			g.logger.Debugf("Normalized relative path %s to absolute path %s", filePath, normalizedPath)
		} else {
			g.logger.Warnf("Cannot get working directory to normalize path %s: %v", filePath, err)
		}
	}

	// 检查目录是否存在
	dir := filepath.Dir(normalizedPath)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return fmt.Errorf("log directory does not exist: %s", dir)
	} else if err != nil {
		return fmt.Errorf("cannot access log directory: %w", err)
	}

	return nil
}

// getLogLevelDescription 获取日志级别描述
func (g *gstreamerLogConfigurator) getLogLevelDescription(level int) string {
	switch level {
	case 0:
		return "NONE - No debug output"
	case 1:
		return "ERROR - Error messages"
	case 2:
		return "WARNING - Warning messages"
	case 3:
		return "FIXME - Fixme messages"
	case 4:
		return "INFO - Informational messages"
	case 5:
		return "DEBUG - Debug messages"
	case 6:
		return "LOG - Log messages"
	case 7:
		return "TRACE - Trace messages"
	case 8:
		return "MEMDUMP - Memory dump messages"
	case 9:
		return "COUNT - All messages"
	default:
		return fmt.Sprintf("UNKNOWN(%d)", level)
	}
}

// applyColoredOutput 应用颜色输出设置到GStreamer
func (g *gstreamerLogConfigurator) applyColoredOutput(colored bool) {
	g.logger.Debugf("Setting GStreamer colored output to: %t", colored)

	// 调用go-gst函数设置GStreamer颜色输出
	setColoredOutput(colored)
}

// parseGstDebugLevel 解析GST_DEBUG环境变量中的级别
func parseGstDebugLevel(gstDebug string) (int, error) {
	// GST_DEBUG可能的格式:
	// - 数字: "4"
	// - 类别:级别: "GST_PIPELINE:4,GST_ELEMENT:3"
	// - 混合: "4" 或 "*:4"

	gstDebug = strings.TrimSpace(gstDebug)
	if gstDebug == "" {
		return 0, fmt.Errorf("empty GST_DEBUG value")
	}

	// 如果是纯数字，直接解析
	if len(gstDebug) == 1 && gstDebug >= "0" && gstDebug <= "9" {
		return int(gstDebug[0] - '0'), nil
	}

	// 如果包含冒号，可能是类别:级别格式
	if strings.Contains(gstDebug, ":") {
		// 查找全局级别设置 (*:级别)
		parts := strings.Split(gstDebug, ",")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if prefix, levelStr, found := strings.Cut(part, ":"); found && prefix == "*" {
				levelStr = strings.TrimSpace(levelStr)
				if len(levelStr) == 1 && levelStr >= "0" && levelStr <= "9" {
					return int(levelStr[0] - '0'), nil
				}
			}
		}

		// 如果没有全局设置，但有类别设置，返回默认级别4
		return 4, nil
	}

	return 0, fmt.Errorf("invalid GST_DEBUG format: %s", gstDebug)
}

// Placeholder functions for go-gst logging (these would be implemented with actual go-gst calls)

// setDebugLevel sets the GStreamer debug level using go-gst
func setDebugLevel(level int) {
	// In a real implementation, this would use go-gst to set the debug level
	// For now, this is a placeholder
}

// SetDebugLevel sets the GStreamer debug level (exported for testing)
func SetDebugLevel(level int) {
	setDebugLevel(level)
}

// setLogFile sets the GStreamer log file using go-gst
func setLogFile(filePath string) {
	// In a real implementation, this would use go-gst to set the log file
	// For now, this is a placeholder
}

// SetLogFile sets the GStreamer log file (exported for testing)
func SetLogFile(filePath string) {
	setLogFile(filePath)
}

// setColoredOutput sets the GStreamer colored output using go-gst
func setColoredOutput(colored bool) {
	// In a real implementation, this would use go-gst to set colored output
	// For now, this is a placeholder
}

// SetColoredOutput sets the GStreamer colored output (exported for testing)
func SetColoredOutput(colored bool) {
	setColoredOutput(colored)
}
