package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config BDWind服务器配置聚合器
type Config struct {
	// Web服务器配置模块
	WebServer *WebServerConfig `yaml:"webserver" json:"webserver"`

	// GStreamer配置模块
	GStreamer *GStreamerConfig `yaml:"gstreamer" json:"gstreamer"`

	// WebRTC配置模块
	WebRTC *WebRTCConfig `yaml:"webrtc" json:"webrtc"`

	// Metrics配置模块
	Metrics *MetricsConfig `yaml:"metrics" json:"metrics"`

	// 生命周期管理配置
	Lifecycle LifecycleConfig `yaml:"lifecycle" json:"lifecycle"`
}

// LifecycleConfig 生命周期管理配置
type LifecycleConfig struct {
	// 优雅关闭超时时间
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout" json:"shutdown_timeout"`

	// 强制关闭超时时间
	ForceShutdownTimeout time.Duration `yaml:"force_shutdown_timeout" json:"force_shutdown_timeout"`

	// 组件启动超时时间
	StartupTimeout time.Duration `yaml:"startup_timeout" json:"startup_timeout"`

	// 是否启用优雅关闭
	EnableGracefulShutdown bool `yaml:"enable_graceful_shutdown" json:"enable_graceful_shutdown"`
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	cfg := &Config{
		WebServer: DefaultWebServerConfig(),
		GStreamer: DefaultGStreamerConfig(),
		WebRTC:    DefaultWebRTCConfig(),
		Metrics:   DefaultMetricsConfig(),
	}

	// 生命周期管理默认配置
	cfg.Lifecycle.ShutdownTimeout = 30 * time.Second
	cfg.Lifecycle.ForceShutdownTimeout = 10 * time.Second
	cfg.Lifecycle.StartupTimeout = 60 * time.Second
	cfg.Lifecycle.EnableGracefulShutdown = true

	return cfg
}

// LoadConfigFromFile 从文件加载配置
func LoadConfigFromFile(filename string) (*Config, error) {
	config := DefaultConfig()

	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %v", err)
	}

	// 先尝试解析为新的模块化格式
	if err := yaml.Unmarshal(data, config); err != nil {
		// 如果失败，尝试解析为旧格式并转换
		legacyConfig, legacyErr := loadLegacyConfig(data)
		if legacyErr != nil {
			return nil, fmt.Errorf("failed to parse config file (new format: %v, legacy format: %v)", err, legacyErr)
		}
		config = convertLegacyConfig(legacyConfig)
	}

	// 验证配置
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %v", err)
	}

	return config, nil
}

// Validate 验证配置
func (c *Config) Validate() error {
	// 验证各个配置模块
	if c.WebServer != nil {
		if err := c.WebServer.Validate(); err != nil {
			return fmt.Errorf("invalid webserver config: %w", err)
		}
	}

	if c.GStreamer != nil {
		if err := c.GStreamer.Validate(); err != nil {
			return fmt.Errorf("invalid gstreamer config: %w", err)
		}
	}

	if c.WebRTC != nil {
		if err := c.WebRTC.Validate(); err != nil {
			return fmt.Errorf("invalid webrtc config: %w", err)
		}
	}

	if c.Metrics != nil {
		if err := c.Metrics.Validate(); err != nil {
			return fmt.Errorf("invalid metrics config: %w", err)
		}
	}

	// 验证生命周期配置
	if err := c.validateLifecycleConfig(); err != nil {
		return fmt.Errorf("invalid lifecycle config: %w", err)
	}

	// 验证模块间的兼容性
	if err := c.validateCrossModuleCompatibility(); err != nil {
		return fmt.Errorf("module compatibility error: %w", err)
	}

	return nil
}

// validateLifecycleConfig 验证生命周期配置
func (c *Config) validateLifecycleConfig() error {
	if c.Lifecycle.ShutdownTimeout <= 0 {
		return fmt.Errorf("shutdown timeout must be positive, got: %v", c.Lifecycle.ShutdownTimeout)
	}

	if c.Lifecycle.ForceShutdownTimeout <= 0 {
		return fmt.Errorf("force shutdown timeout must be positive, got: %v", c.Lifecycle.ForceShutdownTimeout)
	}

	if c.Lifecycle.StartupTimeout <= 0 {
		return fmt.Errorf("startup timeout must be positive, got: %v", c.Lifecycle.StartupTimeout)
	}

	if c.Lifecycle.ShutdownTimeout < c.Lifecycle.ForceShutdownTimeout {
		return fmt.Errorf("shutdown timeout (%v) must be greater than or equal to force shutdown timeout (%v)",
			c.Lifecycle.ShutdownTimeout, c.Lifecycle.ForceShutdownTimeout)
	}

	return nil
}

// validateCrossModuleCompatibility 验证模块间的兼容性
func (c *Config) validateCrossModuleCompatibility() error {
	// 检查端口冲突
	usedPorts := make(map[int]string)

	if c.WebServer != nil {
		if existing, exists := usedPorts[c.WebServer.Port]; exists {
			return fmt.Errorf("port conflict: webserver port %d already used by %s", c.WebServer.Port, existing)
		}
		usedPorts[c.WebServer.Port] = "webserver"
	}

	if c.Metrics != nil && c.Metrics.External.Enabled {
		if existing, exists := usedPorts[c.Metrics.External.Port]; exists {
			return fmt.Errorf("port conflict: metrics port %d already used by %s", c.Metrics.External.Port, existing)
		}
		usedPorts[c.Metrics.External.Port] = "metrics"
	}

	return nil
}

// Merge 合并其他配置
func (c *Config) Merge(other *Config) error {
	if other == nil {
		return nil
	}

	// 合并各个模块
	if other.WebServer != nil {
		if c.WebServer == nil {
			c.WebServer = DefaultWebServerConfig()
		}
		if err := c.WebServer.Merge(other.WebServer); err != nil {
			return fmt.Errorf("failed to merge webserver config: %w", err)
		}
	}

	if other.GStreamer != nil {
		if c.GStreamer == nil {
			c.GStreamer = DefaultGStreamerConfig()
		}
		if err := c.GStreamer.Merge(other.GStreamer); err != nil {
			return fmt.Errorf("failed to merge gstreamer config: %w", err)
		}
	}

	if other.WebRTC != nil {
		if c.WebRTC == nil {
			c.WebRTC = DefaultWebRTCConfig()
		}
		if err := c.WebRTC.Merge(other.WebRTC); err != nil {
			return fmt.Errorf("failed to merge webrtc config: %w", err)
		}
	}

	if other.Metrics != nil {
		if c.Metrics == nil {
			c.Metrics = DefaultMetricsConfig()
		}
		if err := c.Metrics.Merge(other.Metrics); err != nil {
			return fmt.Errorf("failed to merge metrics config: %w", err)
		}
	}

	// 合并生命周期配置
	if other.Lifecycle.ShutdownTimeout != 0 {
		c.Lifecycle.ShutdownTimeout = other.Lifecycle.ShutdownTimeout
	}
	if other.Lifecycle.ForceShutdownTimeout != 0 {
		c.Lifecycle.ForceShutdownTimeout = other.Lifecycle.ForceShutdownTimeout
	}
	if other.Lifecycle.StartupTimeout != 0 {
		c.Lifecycle.StartupTimeout = other.Lifecycle.StartupTimeout
	}
	c.Lifecycle.EnableGracefulShutdown = other.Lifecycle.EnableGracefulShutdown

	return nil
}

// String 返回配置的字符串表示
func (c *Config) String() string {
	webInfo := "disabled"
	if c.WebServer != nil {
		webInfo = fmt.Sprintf("%s:%d", c.WebServer.Host, c.WebServer.Port)
	}

	captureInfo := "disabled"
	if c.GStreamer != nil {
		captureInfo = fmt.Sprintf("%dx%d@%dfps",
			c.GStreamer.Capture.Width, c.GStreamer.Capture.Height, c.GStreamer.Capture.FrameRate)
	}

	encodingInfo := "disabled"
	if c.GStreamer != nil {
		encodingInfo = string(c.GStreamer.Encoding.Codec)
	}

	return fmt.Sprintf("Config{WebServer: %s, Capture: %s, Encoding: %s}",
		webInfo, captureInfo, encodingInfo)
}

// LegacyConfig 旧版配置结构（用于向后兼容）
type LegacyConfig struct {
	Web struct {
		Host        string `yaml:"host" json:"host"`
		Port        int    `yaml:"port" json:"port"`
		TLSCertFile string `yaml:"tls_cert_file" json:"tls_cert_file"`
		TLSKeyFile  string `yaml:"tls_key_file" json:"tls_key_file"`
		StaticDir   string `yaml:"static_dir" json:"static_dir"`
		EnableTLS   bool   `yaml:"enable_tls" json:"enable_tls"`
		EnableAuth  bool   `yaml:"enable_auth" json:"enable_auth"`
		EnableCORS  bool   `yaml:"enable_cors" json:"enable_cors"`
	} `yaml:"web" json:"web"`

	Capture  DesktopCaptureConfig `yaml:"capture" json:"capture"`
	Encoding struct {
		Codec string `yaml:"codec" json:"codec"`
	} `yaml:"encoding" json:"encoding"`

	Auth struct {
		Enabled     bool     `yaml:"enabled" json:"enabled"`
		DefaultRole string   `yaml:"default_role" json:"default_role"`
		AdminUsers  []string `yaml:"admin_users" json:"admin_users"`
	} `yaml:"auth" json:"auth"`

	Monitoring struct {
		Enabled     bool `yaml:"enabled" json:"enabled"`
		MetricsPort int  `yaml:"metrics_port" json:"metrics_port"`
	} `yaml:"monitoring" json:"monitoring"`

	Lifecycle LifecycleConfig `yaml:"lifecycle" json:"lifecycle"`
}

// loadLegacyConfig 加载旧版配置格式
func loadLegacyConfig(data []byte) (*LegacyConfig, error) {
	config := &LegacyConfig{}
	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("failed to parse legacy config: %w", err)
	}
	return config, nil
}

// convertLegacyConfig 将旧版配置转换为新版配置
func convertLegacyConfig(legacy *LegacyConfig) *Config {
	config := DefaultConfig()

	// 转换WebServer配置
	config.WebServer.Host = legacy.Web.Host
	config.WebServer.Port = legacy.Web.Port
	config.WebServer.EnableTLS = legacy.Web.EnableTLS
	config.WebServer.TLS.CertFile = legacy.Web.TLSCertFile
	config.WebServer.TLS.KeyFile = legacy.Web.TLSKeyFile
	config.WebServer.StaticDir = legacy.Web.StaticDir
	config.WebServer.EnableCORS = legacy.Web.EnableCORS

	// 转换认证配置
	config.WebServer.Auth.Enabled = legacy.Auth.Enabled
	config.WebServer.Auth.DefaultRole = legacy.Auth.DefaultRole
	config.WebServer.Auth.AdminUsers = legacy.Auth.AdminUsers

	// 转换GStreamer配置
	config.GStreamer.Capture = legacy.Capture
	// This is a simplification. A more robust conversion would map all legacy fields.
	config.GStreamer.Encoding.Codec = GetCodecTypeFromString(legacy.Encoding.Codec)

	// 转换Metrics配置
	config.Metrics.External.Enabled = legacy.Monitoring.Enabled
	config.Metrics.External.Port = legacy.Monitoring.MetricsPort

	// 转换生命周期配置
	config.Lifecycle = legacy.Lifecycle

	return config
}

// GetWebServerConfig 获取WebServer配置
func (c *Config) GetWebServerConfig() *WebServerConfig {
	if c.WebServer == nil {
		c.WebServer = DefaultWebServerConfig()
	}
	return c.WebServer
}

// GetGStreamerConfig 获取GStreamer配置
func (c *Config) GetGStreamerConfig() *GStreamerConfig {
	if c.GStreamer == nil {
		c.GStreamer = DefaultGStreamerConfig()
	}
	return c.GStreamer
}

// GetWebRTCConfig 获取WebRTC配置
func (c *Config) GetWebRTCConfig() *WebRTCConfig {
	if c.WebRTC == nil {
		c.WebRTC = DefaultWebRTCConfig()
	}
	return c.WebRTC
}

// GetMetricsConfig 获取Metrics配置
func (c *Config) GetMetricsConfig() *MetricsConfig {
	if c.Metrics == nil {
		c.Metrics = DefaultMetricsConfig()
	}
	return c.Metrics
}

// SaveToFile 保存配置到文件
func (c *Config) SaveToFile(filename string) error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// LoadConfigFromEnv 从环境变量加载配置
func LoadConfigFromEnv() *Config {
	config := DefaultConfig()

	// 从环境变量加载GStreamer配置
	config.GStreamer.Capture = LoadDesktopCaptureConfigFromEnv()

	return config
}
