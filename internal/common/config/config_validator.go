package config

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"
)

// ConfigValidator 配置验证器
type ConfigValidator struct {
	logger *logrus.Entry
}

// ValidationResult 验证结果
type ValidationResult struct {
	Valid    bool
	Errors   []ValidationError
	Warnings []ValidationWarning
}

// ValidationError 验证错误
type ValidationError struct {
	Field   string
	Value   interface{}
	Message string
	Code    string
}

// ValidationWarning 验证警告
type ValidationWarning struct {
	Field   string
	Value   interface{}
	Message string
	Code    string
}

// ValidationRule 验证规则接口
type ValidationRule interface {
	Validate(config *Config) []ValidationError
	GetRuleName() string
}

// NewConfigValidator 创建配置验证器
func NewConfigValidator(logger *logrus.Entry) *ConfigValidator {
	return &ConfigValidator{
		logger: logger,
	}
}

// ValidateConfig 验证配置
func (cv *ConfigValidator) ValidateConfig(config *Config) error {
	cv.logger.Debug("Starting comprehensive configuration validation")

	result := &ValidationResult{
		Valid:    true,
		Errors:   make([]ValidationError, 0),
		Warnings: make([]ValidationWarning, 0),
	}

	// 执行各种验证规则
	cv.validateBasicStructure(config, result)
	cv.validateWebServerConfig(config, result)
	cv.validateGStreamerConfig(config, result)
	cv.validateWebRTCConfig(config, result)
	cv.validateMetricsConfig(config, result)
	cv.validateLoggingConfig(config, result)
	cv.validateLifecycleConfig(config, result)
	cv.validateCrossModuleCompatibility(config, result)
	cv.validateSystemRequirements(config, result)
	cv.validateSecuritySettings(config, result)

	// 记录验证结果
	cv.logValidationResult(result)

	if !result.Valid {
		return cv.createValidationError(result)
	}

	return nil
}

// validateBasicStructure 验证基本结构
func (cv *ConfigValidator) validateBasicStructure(config *Config, result *ValidationResult) {
	if config == nil {
		cv.addError(result, "config", nil, "Configuration is nil", "CONFIG_NULL")
		return
	}

	// 验证必需的配置模块
	if config.WebServer == nil {
		cv.addWarning(result, "webserver", nil, "WebServer configuration is missing, using defaults", "WEBSERVER_MISSING")
	}

	if config.GStreamer == nil {
		cv.addError(result, "gstreamer", nil, "GStreamer configuration is required", "GSTREAMER_MISSING")
	}

	if config.Logging == nil {
		cv.addWarning(result, "logging", nil, "Logging configuration is missing, using defaults", "LOGGING_MISSING")
	}
}

// validateWebServerConfig 验证Web服务器配置
func (cv *ConfigValidator) validateWebServerConfig(config *Config, result *ValidationResult) {
	if config.WebServer == nil {
		return
	}

	ws := config.WebServer

	// 验证端口
	if ws.Port <= 0 || ws.Port > 65535 {
		cv.addError(result, "webserver.port", ws.Port, "Port must be between 1 and 65535", "INVALID_PORT")
	}

	// 验证主机地址
	if ws.Host == "" {
		cv.addWarning(result, "webserver.host", ws.Host, "Host is empty, will bind to all interfaces", "EMPTY_HOST")
	} else if net.ParseIP(ws.Host) == nil && ws.Host != "localhost" {
		cv.addWarning(result, "webserver.host", ws.Host, "Host is not a valid IP address or localhost", "INVALID_HOST")
	}

	// 验证TLS配置
	if ws.EnableTLS {
		if ws.TLS.CertFile == "" {
			cv.addError(result, "webserver.tls.cert_file", ws.TLS.CertFile, "TLS certificate file is required when TLS is enabled", "TLS_CERT_MISSING")
		} else if !cv.fileExists(ws.TLS.CertFile) {
			cv.addError(result, "webserver.tls.cert_file", ws.TLS.CertFile, "TLS certificate file does not exist", "TLS_CERT_NOT_FOUND")
		}

		if ws.TLS.KeyFile == "" {
			cv.addError(result, "webserver.tls.key_file", ws.TLS.KeyFile, "TLS key file is required when TLS is enabled", "TLS_KEY_MISSING")
		} else if !cv.fileExists(ws.TLS.KeyFile) {
			cv.addError(result, "webserver.tls.key_file", ws.TLS.KeyFile, "TLS key file does not exist", "TLS_KEY_NOT_FOUND")
		}
	}

	// 验证静态文件目录
	if ws.StaticDir != "" && !cv.dirExists(ws.StaticDir) {
		cv.addWarning(result, "webserver.static_dir", ws.StaticDir, "Static directory does not exist", "STATIC_DIR_NOT_FOUND")
	}

	// 验证认证配置
	if ws.Auth.Enabled {
		if ws.Auth.DefaultRole == "" {
			cv.addWarning(result, "webserver.auth.default_role", ws.Auth.DefaultRole, "Default role is empty", "AUTH_DEFAULT_ROLE_EMPTY")
		}

		if len(ws.Auth.AdminUsers) == 0 {
			cv.addWarning(result, "webserver.auth.admin_users", ws.Auth.AdminUsers, "No admin users configured", "AUTH_NO_ADMIN_USERS")
		}
	}
}

// validateGStreamerConfig 验证GStreamer配置
func (cv *ConfigValidator) validateGStreamerConfig(config *Config, result *ValidationResult) {
	if config.GStreamer == nil {
		return
	}

	gs := config.GStreamer

	// 验证桌面捕获配置
	cv.validateDesktopCaptureConfig(&gs.Capture, result)

	// 验证编码器配置
	cv.validateEncoderConfig(&gs.Encoding, result)
}

// validateDesktopCaptureConfig 验证桌面捕获配置
func (cv *ConfigValidator) validateDesktopCaptureConfig(capture *DesktopCaptureConfig, result *ValidationResult) {
	// 验证分辨率
	if capture.Width <= 0 || capture.Width > 7680 {
		cv.addError(result, "gstreamer.capture.width", capture.Width, "Width must be between 1 and 7680", "INVALID_WIDTH")
	}

	if capture.Height <= 0 || capture.Height > 4320 {
		cv.addError(result, "gstreamer.capture.height", capture.Height, "Height must be between 1 and 4320", "INVALID_HEIGHT")
	}

	// 验证帧率
	if capture.FrameRate <= 0 || capture.FrameRate > 120 {
		cv.addError(result, "gstreamer.capture.frame_rate", capture.FrameRate, "Frame rate must be between 1 and 120", "INVALID_FRAME_RATE")
	}

	// 验证显示ID
	if capture.DisplayID == "" {
		cv.addWarning(result, "gstreamer.capture.display_id", capture.DisplayID, "Display ID is empty, using default", "EMPTY_DISPLAY_ID")
	}

	// 验证缓冲区大小
	if capture.BufferSize <= 0 || capture.BufferSize > 100 {
		cv.addError(result, "gstreamer.capture.buffer_size", capture.BufferSize, "Buffer size must be between 1 and 100", "INVALID_BUFFER_SIZE")
	}

	// 验证质量设置
	validQualities := []string{"low", "medium", "high"}
	if !cv.stringInSlice(capture.Quality, validQualities) {
		cv.addError(result, "gstreamer.capture.quality", capture.Quality, "Quality must be one of: low, medium, high", "INVALID_QUALITY")
	}

	// 验证捕获区域
	if capture.CaptureRegion != nil {
		if capture.CaptureRegion.Width <= 0 || capture.CaptureRegion.Height <= 0 {
			cv.addError(result, "gstreamer.capture.capture_region", capture.CaptureRegion, "Capture region dimensions must be positive", "INVALID_CAPTURE_REGION")
		}

		if capture.CaptureRegion.X < 0 || capture.CaptureRegion.Y < 0 {
			cv.addError(result, "gstreamer.capture.capture_region", capture.CaptureRegion, "Capture region position must be non-negative", "INVALID_CAPTURE_POSITION")
		}
	}

	// 验证显示服务器兼容性
	if capture.UseWayland {
		if os.Getenv("WAYLAND_DISPLAY") == "" && os.Getenv("XDG_SESSION_TYPE") != "wayland" {
			cv.addWarning(result, "gstreamer.capture.use_wayland", capture.UseWayland, "Wayland enabled but no Wayland session detected", "WAYLAND_NOT_DETECTED")
		}
	} else {
		if os.Getenv("DISPLAY") == "" {
			cv.addWarning(result, "gstreamer.capture.use_wayland", capture.UseWayland, "X11 mode but no DISPLAY environment variable", "X11_DISPLAY_MISSING")
		}
	}
}

// validateEncoderConfig 验证编码器配置
func (cv *ConfigValidator) validateEncoderConfig(encoder *EncoderConfig, result *ValidationResult) {
	// 验证编码器类型
	validEncoderTypes := []string{"nvenc", "vaapi", "x264", "vp8", "vp9", "auto"}
	if !cv.stringInSlice(string(encoder.Type), validEncoderTypes) {
		cv.addError(result, "gstreamer.encoding.type", encoder.Type, "Invalid encoder type", "INVALID_ENCODER_TYPE")
	}

	// 验证编解码器
	validCodecs := []string{"h264", "vp8", "vp9"}
	if !cv.stringInSlice(string(encoder.Codec), validCodecs) {
		cv.addError(result, "gstreamer.encoding.codec", encoder.Codec, "Invalid codec", "INVALID_CODEC")
	}

	// 验证比特率
	if encoder.Bitrate <= 0 || encoder.Bitrate > 50000 {
		cv.addError(result, "gstreamer.encoding.bitrate", encoder.Bitrate, "Bitrate must be between 1 and 50000 kbps", "INVALID_BITRATE")
	}

	if encoder.MaxBitrate > 0 && encoder.MaxBitrate < encoder.Bitrate {
		cv.addError(result, "gstreamer.encoding.max_bitrate", encoder.MaxBitrate, "Max bitrate must be greater than or equal to bitrate", "INVALID_MAX_BITRATE")
	}

	if encoder.MinBitrate > 0 && encoder.MinBitrate > encoder.Bitrate {
		cv.addError(result, "gstreamer.encoding.min_bitrate", encoder.MinBitrate, "Min bitrate must be less than or equal to bitrate", "INVALID_MIN_BITRATE")
	}

	// 验证关键帧间隔
	if encoder.KeyframeInterval <= 0 || encoder.KeyframeInterval > 60 {
		cv.addError(result, "gstreamer.encoding.keyframe_interval", encoder.KeyframeInterval, "Keyframe interval must be between 1 and 60 seconds", "INVALID_KEYFRAME_INTERVAL")
	}

	// 验证质量设置
	maxQuality := 51
	if encoder.Codec == CodecVP8 || encoder.Codec == CodecVP9 {
		maxQuality = 63
	}
	if encoder.Quality < 0 || encoder.Quality > maxQuality {
		cv.addError(result, "gstreamer.encoding.quality", encoder.Quality, fmt.Sprintf("Quality must be between 0 and %d for %s", maxQuality, encoder.Codec), "INVALID_QUALITY")
	}

	// 验证线程数
	if encoder.Threads < 0 || encoder.Threads > 64 {
		cv.addError(result, "gstreamer.encoding.threads", encoder.Threads, "Threads must be between 0 and 64", "INVALID_THREADS")
	}

	// 验证硬件设备
	if encoder.HardwareDevice < 0 || encoder.HardwareDevice > 15 {
		cv.addError(result, "gstreamer.encoding.hardware_device", encoder.HardwareDevice, "Hardware device must be between 0 and 15", "INVALID_HARDWARE_DEVICE")
	}

	// 验证硬件编码器可用性
	if encoder.UseHardware {
		cv.validateHardwareEncoderAvailability(encoder, result)
	}
}

// validateHardwareEncoderAvailability 验证硬件编码器可用性
func (cv *ConfigValidator) validateHardwareEncoderAvailability(encoder *EncoderConfig, result *ValidationResult) {
	switch encoder.Type {
	case EncoderTypeNVENC:
		if !cv.checkNVENCAvailability() {
			cv.addWarning(result, "gstreamer.encoding.type", encoder.Type, "NVENC encoder requested but NVIDIA GPU not detected", "NVENC_NOT_AVAILABLE")
		}
	case EncoderTypeVAAPI:
		if !cv.checkVAAPIAvailability() {
			cv.addWarning(result, "gstreamer.encoding.type", encoder.Type, "VAAPI encoder requested but compatible GPU not detected", "VAAPI_NOT_AVAILABLE")
		}
	}
}

// validateWebRTCConfig 验证WebRTC配置
func (cv *ConfigValidator) validateWebRTCConfig(config *Config, result *ValidationResult) {
	if config.WebRTC == nil {
		return
	}

	// WebRTC配置验证逻辑
	// 这里可以添加WebRTC特定的验证规则
}

// validateMetricsConfig 验证指标配置
func (cv *ConfigValidator) validateMetricsConfig(config *Config, result *ValidationResult) {
	if config.Metrics == nil {
		return
	}

	metrics := config.Metrics

	// 验证外部指标端口
	if metrics.External.Enabled {
		if metrics.External.Port <= 0 || metrics.External.Port > 65535 {
			cv.addError(result, "metrics.external.port", metrics.External.Port, "Metrics port must be between 1 and 65535", "INVALID_METRICS_PORT")
		}
	}

	// 验证收集间隔
	if metrics.Internal.CollectionInterval <= 0 {
		cv.addError(result, "metrics.internal.collection_interval", metrics.Internal.CollectionInterval, "Collection interval must be positive", "INVALID_COLLECTION_INTERVAL")
	}
}

// validateLoggingConfig 验证日志配置
func (cv *ConfigValidator) validateLoggingConfig(config *Config, result *ValidationResult) {
	if config.Logging == nil {
		return
	}

	logging := config.Logging

	// 验证日志级别
	validLevels := []string{"trace", "debug", "info", "warn", "error", "fatal", "panic"}
	if !cv.stringInSlice(strings.ToLower(logging.Level), validLevels) {
		cv.addError(result, "logging.level", logging.Level, "Invalid log level", "INVALID_LOG_LEVEL")
	}

	// 验证日志文件路径
	if logging.File != "" {
		dir := filepath.Dir(logging.File)
		if !cv.dirExists(dir) {
			cv.addWarning(result, "logging.file", logging.File, "Log file directory does not exist", "LOG_DIR_NOT_FOUND")
		}
	}

	// Note: LoggingConfig doesn't have rotation settings in the current implementation
	// This validation is commented out as the current LoggingConfig structure doesn't include rotation
	// If rotation is needed, it should be added to the LoggingConfig struct
}

// validateLifecycleConfig 验证生命周期配置
func (cv *ConfigValidator) validateLifecycleConfig(config *Config, result *ValidationResult) {
	lifecycle := config.Lifecycle

	// 验证超时时间
	if lifecycle.ShutdownTimeout <= 0 {
		cv.addError(result, "lifecycle.shutdown_timeout", lifecycle.ShutdownTimeout, "Shutdown timeout must be positive", "INVALID_SHUTDOWN_TIMEOUT")
	}

	if lifecycle.ForceShutdownTimeout <= 0 {
		cv.addError(result, "lifecycle.force_shutdown_timeout", lifecycle.ForceShutdownTimeout, "Force shutdown timeout must be positive", "INVALID_FORCE_SHUTDOWN_TIMEOUT")
	}

	if lifecycle.StartupTimeout <= 0 {
		cv.addError(result, "lifecycle.startup_timeout", lifecycle.StartupTimeout, "Startup timeout must be positive", "INVALID_STARTUP_TIMEOUT")
	}

	// 验证超时时间关系
	if lifecycle.ShutdownTimeout < lifecycle.ForceShutdownTimeout {
		cv.addError(result, "lifecycle.shutdown_timeout", lifecycle.ShutdownTimeout, "Shutdown timeout must be greater than or equal to force shutdown timeout", "SHUTDOWN_TIMEOUT_TOO_SMALL")
	}
}

// validateCrossModuleCompatibility 验证模块间兼容性
func (cv *ConfigValidator) validateCrossModuleCompatibility(config *Config, result *ValidationResult) {
	// 检查端口冲突
	usedPorts := make(map[int]string)

	if config.WebServer != nil {
		port := config.WebServer.Port
		if existing, exists := usedPorts[port]; exists {
			cv.addError(result, "webserver.port", port, fmt.Sprintf("Port conflict: already used by %s", existing), "PORT_CONFLICT")
		} else {
			usedPorts[port] = "webserver"
		}
	}

	if config.Metrics != nil && config.Metrics.External.Enabled {
		port := config.Metrics.External.Port
		if existing, exists := usedPorts[port]; exists {
			cv.addError(result, "metrics.external.port", port, fmt.Sprintf("Port conflict: already used by %s", existing), "PORT_CONFLICT")
		} else {
			usedPorts[port] = "metrics"
		}
	}
}

// validateSystemRequirements 验证系统需求
func (cv *ConfigValidator) validateSystemRequirements(config *Config, result *ValidationResult) {
	// 检查必需的系统命令
	requiredCommands := []string{"gst-launch-1.0", "gst-inspect-1.0"}
	for _, cmd := range requiredCommands {
		if !cv.commandExists(cmd) {
			cv.addError(result, "system.commands", cmd, fmt.Sprintf("Required command '%s' not found in PATH", cmd), "COMMAND_NOT_FOUND")
		}
	}

	// 检查GStreamer插件
	if config.GStreamer != nil {
		cv.validateGStreamerPlugins(config.GStreamer, result)
	}
}

// validateGStreamerPlugins 验证GStreamer插件
func (cv *ConfigValidator) validateGStreamerPlugins(gstreamer *GStreamerConfig, result *ValidationResult) {
	// 检查桌面捕获插件
	if gstreamer.Capture.UseWayland {
		if !cv.gstreamerPluginExists("waylandsink") {
			cv.addWarning(result, "gstreamer.capture.use_wayland", true, "Wayland capture plugin not available", "WAYLAND_PLUGIN_MISSING")
		}
	} else {
		if !cv.gstreamerPluginExists("ximagesrc") {
			cv.addError(result, "gstreamer.capture.use_wayland", false, "X11 capture plugin not available", "X11_PLUGIN_MISSING")
		}
	}

	// 检查编码器插件
	switch gstreamer.Encoding.Type {
	case EncoderTypeNVENC:
		if !cv.gstreamerPluginExists("nvh264enc") {
			cv.addWarning(result, "gstreamer.encoding.type", gstreamer.Encoding.Type, "NVENC plugin not available", "NVENC_PLUGIN_MISSING")
		}
	case EncoderTypeVAAPI:
		if !cv.gstreamerPluginExists("vaapih264enc") {
			cv.addWarning(result, "gstreamer.encoding.type", gstreamer.Encoding.Type, "VAAPI plugin not available", "VAAPI_PLUGIN_MISSING")
		}
	case EncoderTypeX264:
		if !cv.gstreamerPluginExists("x264enc") {
			cv.addError(result, "gstreamer.encoding.type", gstreamer.Encoding.Type, "x264 plugin not available", "X264_PLUGIN_MISSING")
		}
	}
}

// validateSecuritySettings 验证安全设置
func (cv *ConfigValidator) validateSecuritySettings(config *Config, result *ValidationResult) {
	if config.WebServer != nil {
		// 检查是否在生产环境中启用了不安全的设置
		if config.WebServer.EnableCORS {
			cv.addWarning(result, "webserver.enable_cors", true, "CORS is enabled, ensure this is intended for production", "CORS_ENABLED")
		}

		if !config.WebServer.EnableTLS && config.WebServer.Port == 80 {
			cv.addWarning(result, "webserver.enable_tls", false, "TLS is disabled on standard HTTP port, consider enabling HTTPS", "TLS_DISABLED")
		}
	}
}

// 辅助方法

func (cv *ConfigValidator) addError(result *ValidationResult, field string, value interface{}, message, code string) {
	result.Valid = false
	result.Errors = append(result.Errors, ValidationError{
		Field:   field,
		Value:   value,
		Message: message,
		Code:    code,
	})
}

func (cv *ConfigValidator) addWarning(result *ValidationResult, field string, value interface{}, message, code string) {
	result.Warnings = append(result.Warnings, ValidationWarning{
		Field:   field,
		Value:   value,
		Message: message,
		Code:    code,
	})
}

func (cv *ConfigValidator) fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (cv *ConfigValidator) dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func (cv *ConfigValidator) commandExists(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}

func (cv *ConfigValidator) stringInSlice(str string, slice []string) bool {
	for _, s := range slice {
		if s == str {
			return true
		}
	}
	return false
}

func (cv *ConfigValidator) checkNVENCAvailability() bool {
	// 检查NVIDIA GPU设备文件
	if _, err := os.Stat("/dev/nvidia0"); err == nil {
		return true
	}

	// 检查nvidia-smi命令
	if _, err := exec.LookPath("nvidia-smi"); err == nil {
		return true
	}

	return false
}

func (cv *ConfigValidator) checkVAAPIAvailability() bool {
	// 检查VAAPI设备文件
	if _, err := os.Stat("/dev/dri/renderD128"); err == nil {
		return true
	}

	return false
}

func (cv *ConfigValidator) gstreamerPluginExists(plugin string) bool {
	cmd := exec.Command("gst-inspect-1.0", plugin)
	err := cmd.Run()
	return err == nil
}

func (cv *ConfigValidator) logValidationResult(result *ValidationResult) {
	if result.Valid {
		cv.logger.Info("Configuration validation passed successfully")
		if len(result.Warnings) > 0 {
			cv.logger.Warnf("Configuration validation completed with %d warnings", len(result.Warnings))
			for _, warning := range result.Warnings {
				cv.logger.Warnf("  Warning [%s]: %s (field: %s, value: %v)", warning.Code, warning.Message, warning.Field, warning.Value)
			}
		}
	} else {
		cv.logger.Errorf("Configuration validation failed with %d errors", len(result.Errors))
		for _, err := range result.Errors {
			cv.logger.Errorf("  Error [%s]: %s (field: %s, value: %v)", err.Code, err.Message, err.Field, err.Value)
		}
	}
}

func (cv *ConfigValidator) createValidationError(result *ValidationResult) error {
	var errorMessages []string
	for _, err := range result.Errors {
		errorMessages = append(errorMessages, fmt.Sprintf("[%s] %s: %s", err.Code, err.Field, err.Message))
	}

	return fmt.Errorf("configuration validation failed:\n  %s", strings.Join(errorMessages, "\n  "))
}
