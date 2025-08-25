package config

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"

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
		c.File = other.File
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
		config.File = file
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
		formatter := &logrus.TextFormatter{
			TimestampFormat: "2006-01-02 15:04:05.000",
			FullTimestamp:   config.EnableTimestamp,
			ForceColors:     config.EnableColors,
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
