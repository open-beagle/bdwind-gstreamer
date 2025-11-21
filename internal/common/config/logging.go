package config

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"github.com/sirupsen/logrus"
)

// LoggingConfig 日志配置
type LoggingConfig struct {
	// Level 日志等级 (trace, debug, info, warn, error)
	Level string `yaml:"level" json:"level"`

	// Format 日志格式 (text, json)
	Format string `yaml:"format" json:"format"`

	// Output 输出目标 (stdout, stderr, file)
	Output string `yaml:"output" json:"output"`

	// File 日志文件路径 (当Output为file时使用)
	File string `yaml:"file" json:"file"`

	// EnableTimestamp 是否启用时间戳
	EnableTimestamp bool `yaml:"enable_timestamp" json:"enable_timestamp"`

	// EnableCaller 是否启用调用者信息
	EnableCaller bool `yaml:"enable_caller" json:"enable_caller"`

	// EnableColors 是否启用颜色输出
	EnableColors bool `yaml:"enable_colors" json:"enable_colors"`
}

// DefaultLoggingConfig 返回默认日志配置
func DefaultLoggingConfig() *LoggingConfig {
	return &LoggingConfig{
		Level:           "info",
		Format:          "text",
		Output:          "stdout",
		File:            "",
		EnableTimestamp: true,
		EnableCaller:    false,
		EnableColors:    true,
	}
}

// Validate 验证日志配置
func (c *LoggingConfig) Validate() error {
	// 验证日志等级
	if _, err := logrus.ParseLevel(c.Level); err != nil {
		return fmt.Errorf("invalid log level: %s", c.Level)
	}

	// 验证日志格式
	if c.Format != "text" && c.Format != "json" {
		return fmt.Errorf("invalid log format: %s, must be 'text' or 'json'", c.Format)
	}

	// 验证输出目标
	if c.Output != "stdout" && c.Output != "stderr" && c.Output != "file" {
		return fmt.Errorf("invalid log output: %s, must be 'stdout', 'stderr', or 'file'", c.Output)
	}

	// 如果输出到文件，检查文件路径
	if c.Output == "file" && c.File == "" {
		return fmt.Errorf("log file path is required when output is 'file'")
	}

	return nil
}

// Merge 合并日志配置
func (c *LoggingConfig) Merge(other *LoggingConfig) error {
	if other == nil {
		return nil
	}

	// 合并各个字段
	if other.Level != "" {
		c.Level = other.Level
	}

	if other.Format != "" {
		c.Format = other.Format
	}

	if other.Output != "" {
		c.Output = other.Output
	}

	if other.File != "" {
		c.File = NormalizeLogFilePath(other.File)
	}

	c.EnableTimestamp = other.EnableTimestamp
	c.EnableCaller = other.EnableCaller
	c.EnableColors = other.EnableColors

	return c.Validate()
}

// LoadLoggingConfigFromEnv 从环境变量加载日志配置
func LoadLoggingConfigFromEnv() *LoggingConfig {
	config := DefaultLoggingConfig()

	// 从环境变量读取日志等级
	if level := os.Getenv("BDWIND_LOG_LEVEL"); level != "" {
		config.Level = strings.ToLower(level)
	}

	// 从环境变量读取日志格式
	if format := os.Getenv("BDWIND_LOG_FORMAT"); format != "" {
		config.Format = format
	}

	// 从环境变量读取输出目标
	if output := os.Getenv("BDWIND_LOG_OUTPUT"); output != "" {
		config.Output = output
	}

	// 从环境变量读取日志文件路径
	if file := os.Getenv("BDWIND_LOG_FILE"); file != "" {
		config.File = NormalizeLogFilePath(file)
	}

	// 从环境变量读取时间戳设置
	if timestamp := os.Getenv("BDWIND_LOG_TIMESTAMP"); timestamp != "" {
		config.EnableTimestamp = strings.ToLower(timestamp) == "true"
	}

	// 从环境变量读取调用者信息设置
	if caller := os.Getenv("BDWIND_LOG_CALLER"); caller != "" {
		config.EnableCaller = strings.ToLower(caller) == "true"
	}

	// 从环境变量读取颜色设置
	if colors := os.Getenv("BDWIND_LOG_COLORS"); colors != "" {
		config.EnableColors = strings.ToLower(colors) == "true"
	}

	return config
}

// SetupLogger 根据配置设置 logrus
func SetupLogger(config *LoggingConfig) error {
	if config == nil {
		config = DefaultLoggingConfig()
	}

	if err := config.Validate(); err != nil {
		return fmt.Errorf("invalid logging config: %w", err)
	}

	// 设置日志等级
	level, err := logrus.ParseLevel(config.Level)
	if err != nil {
		return fmt.Errorf("failed to parse log level: %w", err)
	}
	logrus.SetLevel(level)

	// 设置输出目标
	var output io.Writer
	switch config.Output {
	case "stdout":
		output = os.Stdout
	case "stderr":
		output = os.Stderr
	case "file":
		file, err := os.OpenFile(config.File, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return fmt.Errorf("failed to open log file %s: %w", config.File, err)
		}
		output = file
	}
	logrus.SetOutput(output)

	// 设置日志格式
	if config.Format == "json" {
		logrus.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: "2006-01-02 15:04:05.000",
		})
	} else {
		// 智能颜色检测和控制
		enableColors := shouldEnableColors(output, config)

		formatter := &logrus.TextFormatter{
			TimestampFormat: "2006-01-02 15:04:05.000",
			FullTimestamp:   config.EnableTimestamp,
			ForceColors:     enableColors,
			DisableColors:   !enableColors, // 明确禁用颜色
		}
		logrus.SetFormatter(formatter)
	}

	// 设置调用者信息
	logrus.SetReportCaller(config.EnableCaller)

	return nil
}

// ParseLogLevel 解析日志等级字符串（兼容旧代码）
func ParseLogLevel(level string) (string, error) {
	normalizedLevel := strings.ToLower(strings.TrimSpace(level))

	// 验证等级是否有效
	if _, err := logrus.ParseLevel(normalizedLevel); err != nil {
		return "info", fmt.Errorf("invalid log level: %s", level)
	}

	return normalizedLevel, nil
}

// GetLoggerWithPrefix 获取带前缀的logger
func GetLoggerWithPrefix(prefix string) *logrus.Entry {
	return logrus.WithField("component", prefix)
}

// SetGlobalLogLevel 动态设置全局日志等级
func SetGlobalLogLevel(level string) error {
	logLevel, err := logrus.ParseLevel(level)
	if err != nil {
		return fmt.Errorf("invalid log level: %w", err)
	}

	logrus.SetLevel(logLevel)
	return nil
}

// GetGlobalLogLevel 获取当前全局日志等级
func GetGlobalLogLevel() string {
	return logrus.GetLevel().String()
}

// GetStandardLogger 获取标准库兼容的logger
func GetStandardLogger() *log.Logger {
	return log.New(logrus.StandardLogger().Out, "", 0)
}

// GetStandardLoggerWithPrefix 获取带前缀的标准库兼容logger
func GetStandardLoggerWithPrefix(prefix string) *log.Logger {
	entry := GetLoggerWithPrefix(prefix)
	return log.New(&logrusWriter{entry: entry}, "", 0)
}

// shouldEnableColors 智能检测是否应该启用颜色输出
func shouldEnableColors(output io.Writer, config *LoggingConfig) bool {
	// 1. 检查环境变量 BDWIND_LOG_COLORS 的强制设置
	if colorsEnv := os.Getenv("BDWIND_LOG_COLORS"); colorsEnv != "" {
		return strings.ToLower(colorsEnv) == "true"
	}

	// 2. 如果配置中明确禁用颜色，则禁用
	if !config.EnableColors {
		return false
	}

	// 3. 检查输出类型
	switch v := output.(type) {
	case *os.File:
		// 文件输出：检查是否为真实文件（非终端）
		if isFileOutput(v) {
			return false // 文件输出强制禁用颜色
		}
		// 终端输出：检查是否为TTY
		return isTerminal(v)
	default:
		// 其他类型的输出（如管道、缓冲区等）禁用颜色
		return false
	}
}

// isFileOutput 检查是否为文件输出（非终端设备）
func isFileOutput(file *os.File) bool {
	// 获取文件信息
	fileInfo, err := file.Stat()
	if err != nil {
		return false
	}

	// 检查文件模式，如果是常规文件则返回true
	mode := fileInfo.Mode()
	return mode.IsRegular()
}

// isTerminal 检查文件描述符是否为终端
func isTerminal(file *os.File) bool {
	// 使用Linux系统调用检查是否为TTY
	fd := file.Fd()
	var termios syscall.Termios
	_, _, err := syscall.Syscall6(syscall.SYS_IOCTL, fd, syscall.TCGETS, uintptr(unsafe.Pointer(&termios)), 0, 0, 0)
	return err == 0
}

// isPipeOutput 检查是否为管道输出
func isPipeOutput(file *os.File) bool {
	fileInfo, err := file.Stat()
	if err != nil {
		return false
	}

	// 检查文件模式，如果是命名管道或字符设备但不是终端
	mode := fileInfo.Mode()
	return (mode&os.ModeNamedPipe != 0) || (mode&os.ModeCharDevice != 0 && !isTerminal(file))
}

// PrintLoggingInfo 输出当前日志配置信息
func PrintLoggingInfo(cfg *LoggingConfig) {
	logger := GetLoggerWithPrefix("config")

	logger.Trace("📋 Current Logging Configuration:")
	logger.Tracef("  ✅ Level: %s", cfg.Level)
	logger.Tracef("  ✅ Format: %s", cfg.Format)
	logger.Tracef("  ✅ Output: %s", cfg.Output)

	if cfg.Output == "file" && cfg.File != "" {
		logger.Tracef("  ✅ File: %s", cfg.File)

		// 检查文件状态
		if fileInfo, err := os.Stat(cfg.File); err == nil {
			logger.Tracef("  📁 File Size: %d bytes", fileInfo.Size())
			logger.Tracef("  📅 Last Modified: %s", fileInfo.ModTime().Format("2006-01-02 15:04:05"))
		}
	}

	logger.Tracef("  ✅ Timestamp: %t", cfg.EnableTimestamp)
	logger.Tracef("  ✅ Caller Info: %t", cfg.EnableCaller)
	logger.Tracef("  ✅ Colors: %t", cfg.EnableColors)

	// 输出当前 logrus 状态
	logger.Tracef("  📊 Current Level: %s", logrus.GetLevel().String())

	// 检查环境变量覆盖
	envOverrides := []string{}
	if os.Getenv("BDWIND_LOG_LEVEL") != "" {
		envOverrides = append(envOverrides, "BDWIND_LOG_LEVEL")
	}
	if os.Getenv("BDWIND_LOG_OUTPUT") != "" {
		envOverrides = append(envOverrides, "BDWIND_LOG_OUTPUT")
	}
	if os.Getenv("BDWIND_LOG_FILE") != "" {
		envOverrides = append(envOverrides, "BDWIND_LOG_FILE")
	}

	if len(envOverrides) > 0 {
		logger.Tracef("  🔧 Environment Overrides: %v", envOverrides)
	}
}

// ValidateLoggingSetup 验证日志系统设置
func ValidateLoggingSetup(cfg *LoggingConfig) error {
	logger := GetLoggerWithPrefix("config")

	logger.Debug("Starting logging configuration validation...")

	// 基本配置验证
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("configuration validation failed: %w", err)
	}
	logger.Debug("✅ Basic configuration validation passed")

	// 文件输出特殊验证
	if cfg.Output == "file" {
		if err := validateLogFileAccess(cfg.File, logger); err != nil {
			return fmt.Errorf("log file validation failed: %w", err)
		}
		logger.Debug("✅ Log file access validation passed")
	}

	// 权限验证
	if err := validateLoggingPermissions(cfg, logger); err != nil {
		return fmt.Errorf("permissions validation failed: %w", err)
	}
	logger.Debug("✅ Permissions validation passed")

	logger.Debug("✅ Logging configuration validation completed successfully")
	return nil
}

// validateLogFileAccess 验证日志文件访问权限
func validateLogFileAccess(logFile string, logger *logrus.Entry) error {
	// 检查目录是否存在，不存在则尝试创建
	dir := strings.TrimSuffix(logFile, "/"+strings.Split(logFile, "/")[len(strings.Split(logFile, "/"))-1])
	if dir != logFile { // 确保有目录部分
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			logger.Debugf("Creating log directory: %s", dir)
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("cannot create log directory %s: %w", dir, err)
			}
		}
	}

	// 尝试打开文件进行写入测试
	file, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("cannot open log file %s for writing: %w", logFile, err)
	}
	defer file.Close()

	// 写入测试消息
	testMsg := fmt.Sprintf("# Copyright © 2018 北京比格大数据有限公司. BDWind Team. All rights reserved. - %s\n",
		fmt.Sprintf("%d", os.Getpid()))
	if _, err := file.WriteString(testMsg); err != nil {
		return fmt.Errorf("cannot write to log file %s: %w", logFile, err)
	}

	logger.Debugf("Log file %s is writable", logFile)
	return nil
}

// validateLoggingPermissions 验证日志相关权限
func validateLoggingPermissions(cfg *LoggingConfig, logger *logrus.Entry) error {
	// 检查当前用户权限
	uid := os.Getuid()
	gid := os.Getgid()
	logger.Debugf("Running as UID: %d, GID: %d", uid, gid)

	// 如果是文件输出，检查文件权限
	if cfg.Output == "file" && cfg.File != "" {
		fileInfo, err := os.Stat(cfg.File)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("cannot stat log file %s: %w", cfg.File, err)
		}

		if fileInfo != nil {
			mode := fileInfo.Mode()
			logger.Debugf("Log file permissions: %s", mode.String())

			// 检查是否有写权限
			if mode.Perm()&0200 == 0 { // 检查所有者写权限
				return fmt.Errorf("log file %s is not writable", cfg.File)
			}
		}
	}

	return nil
}

// NormalizeLogFilePath 规范化日志文件路径，将相对路径转换为绝对路径
func NormalizeLogFilePath(filePath string) string {
	if filePath == "" {
		return ""
	}

	// 如果已经是绝对路径，直接返回
	if filepath.IsAbs(filePath) {
		return filePath
	}

	// 获取当前工作目录
	workDir, err := os.Getwd()
	if err != nil {
		// 如果无法获取工作目录，返回原路径
		return filePath
	}

	// 将相对路径转换为绝对路径
	absPath := filepath.Join(workDir, filePath)

	// 清理路径（去除 . 和 .. 等）
	return filepath.Clean(absPath)
}

// logrusWriter 将 logrus.Entry 包装为 io.Writer
type logrusWriter struct {
	entry *logrus.Entry
}

// Write 实现 io.Writer 接口
func (w *logrusWriter) Write(p []byte) (n int, err error) {
	message := string(p)
	// 移除末尾的换行符，因为logrus会自动添加
	if len(message) > 0 && message[len(message)-1] == '\n' {
		message = message[:len(message)-1]
	}

	w.entry.Info(message)
	return len(p), nil
}
