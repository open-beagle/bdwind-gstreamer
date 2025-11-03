package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
	"github.com/sirupsen/logrus"
)

// SimpleConfig provides direct configuration access for simplified implementations
// This removes validation layers and complex processing for better performance
type SimpleConfig struct {
	// Direct configuration access
	GStreamer *GStreamerConfig `yaml:"gstreamer" json:"gstreamer"`
	WebRTC    *WebRTCConfig    `yaml:"webrtc" json:"webrtc"`
	WebServer *WebServerConfig `yaml:"webserver" json:"webserver"`
	Metrics   *MetricsConfig   `yaml:"metrics" json:"metrics"`
	Logging   *LoggingConfig   `yaml:"logging" json:"logging"`

	// Implementation selection
	Implementation ImplementationConfig `yaml:"implementation" json:"implementation"`

	// Lifecycle management (simplified)
	Lifecycle SimpleLifecycleConfig `yaml:"lifecycle" json:"lifecycle"`
}

// SimpleLifecycleConfig simplified lifecycle configuration without complex validation
type SimpleLifecycleConfig struct {
	ShutdownTimeout      time.Duration `yaml:"shutdown_timeout" json:"shutdown_timeout"`
	ForceShutdownTimeout time.Duration `yaml:"force_shutdown_timeout" json:"force_shutdown_timeout"`
	StartupTimeout       time.Duration `yaml:"startup_timeout" json:"startup_timeout"`
	EnableGracefulShutdown bool        `yaml:"enable_graceful_shutdown" json:"enable_graceful_shutdown"`
}

// NewSimpleConfig creates a new simplified configuration with defaults
func NewSimpleConfig() *SimpleConfig {
	return &SimpleConfig{
		GStreamer: DefaultGStreamerConfig(),
		WebRTC:    DefaultWebRTCConfig(),
		WebServer: DefaultWebServerConfig(),
		Metrics:   DefaultMetricsConfig(),
		Logging:   DefaultLoggingConfig(),
		Implementation: ImplementationConfig{
			UseSimplified:      true,
			Type:              "simplified",
			AllowRuntimeSwitch: false,
		},
		Lifecycle: SimpleLifecycleConfig{
			ShutdownTimeout:        30 * time.Second,
			ForceShutdownTimeout:   10 * time.Second,
			StartupTimeout:         60 * time.Second,
			EnableGracefulShutdown: true,
		},
	}
}

// LoadSimpleConfigFromFile loads configuration from file with minimal validation
func LoadSimpleConfigFromFile(filename string) (*SimpleConfig, error) {
	// Start with defaults
	config := NewSimpleConfig()

	// Read file
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Try to parse as simplified config first
	if err := yaml.Unmarshal(data, config); err != nil {
		// If that fails, try to load as legacy config and convert
		legacyConfig, legacyErr := loadLegacyConfig(data)
		if legacyErr != nil {
			// If both fail, try to load as full config and convert
			fullConfig, fullErr := LoadConfigFromFile(filename)
			if fullErr != nil {
				return nil, fmt.Errorf("failed to parse config file (simple: %v, legacy: %v, full: %v)", 
					err, legacyErr, fullErr)
			}
			return ConvertToSimpleConfig(fullConfig), nil
		}
		config = convertLegacyToSimpleConfig(legacyConfig)
	}

	// Apply minimal validation only for critical settings
	if err := config.ValidateMinimal(); err != nil {
		return nil, fmt.Errorf("critical configuration error: %w", err)
	}

	return config, nil
}

// ValidateMinimal performs only essential validation for simplified config
func (c *SimpleConfig) ValidateMinimal() error {
	// Only validate critical settings that could cause runtime failures
	
	// Check that we have basic GStreamer config
	if c.GStreamer == nil {
		c.GStreamer = DefaultGStreamerConfig()
	}

	// Check that we have basic WebRTC config
	if c.WebRTC == nil {
		c.WebRTC = DefaultWebRTCConfig()
	}

	// Validate only critical GStreamer settings
	if c.GStreamer.Capture.Width <= 0 {
		c.GStreamer.Capture.Width = 1920
	}
	if c.GStreamer.Capture.Height <= 0 {
		c.GStreamer.Capture.Height = 1080
	}
	if c.GStreamer.Capture.FrameRate <= 0 {
		c.GStreamer.Capture.FrameRate = 30
	}

	// Validate only critical WebRTC settings
	if len(c.WebRTC.ICEServers) == 0 {
		c.WebRTC.SetDefaults()
	}

	// Validate lifecycle timeouts
	if c.Lifecycle.ShutdownTimeout <= 0 {
		c.Lifecycle.ShutdownTimeout = 30 * time.Second
	}
	if c.Lifecycle.ForceShutdownTimeout <= 0 {
		c.Lifecycle.ForceShutdownTimeout = 10 * time.Second
	}
	if c.Lifecycle.StartupTimeout <= 0 {
		c.Lifecycle.StartupTimeout = 60 * time.Second
	}

	return nil
}

// GetGStreamerConfig returns GStreamer configuration with direct access
func (c *SimpleConfig) GetGStreamerConfig() *GStreamerConfig {
	if c.GStreamer == nil {
		c.GStreamer = DefaultGStreamerConfig()
	}
	return c.GStreamer
}

// GetWebRTCConfig returns WebRTC configuration with direct access
func (c *SimpleConfig) GetWebRTCConfig() *WebRTCConfig {
	if c.WebRTC == nil {
		c.WebRTC = DefaultWebRTCConfig()
	}
	return c.WebRTC
}

// GetWebServerConfig returns WebServer configuration with direct access
func (c *SimpleConfig) GetWebServerConfig() *WebServerConfig {
	if c.WebServer == nil {
		c.WebServer = DefaultWebServerConfig()
	}
	return c.WebServer
}

// GetMetricsConfig returns Metrics configuration with direct access
func (c *SimpleConfig) GetMetricsConfig() *MetricsConfig {
	if c.Metrics == nil {
		c.Metrics = DefaultMetricsConfig()
	}
	return c.Metrics
}

// GetLoggingConfig returns Logging configuration with direct access
func (c *SimpleConfig) GetLoggingConfig() *LoggingConfig {
	if c.Logging == nil {
		c.Logging = DefaultLoggingConfig()
	}
	return c.Logging
}

// ConvertToSimpleConfig converts a full Config to SimpleConfig
func ConvertToSimpleConfig(fullConfig *Config) *SimpleConfig {
	simple := NewSimpleConfig()

	// Direct assignment without validation
	if fullConfig.GStreamer != nil {
		simple.GStreamer = fullConfig.GStreamer
	}
	if fullConfig.WebRTC != nil {
		simple.WebRTC = fullConfig.WebRTC
	}
	if fullConfig.WebServer != nil {
		simple.WebServer = fullConfig.WebServer
	}
	if fullConfig.Metrics != nil {
		simple.Metrics = fullConfig.Metrics
	}
	if fullConfig.Logging != nil {
		simple.Logging = fullConfig.Logging
	}

	// Convert implementation config
	simple.Implementation = fullConfig.Implementation

	// Convert lifecycle config
	simple.Lifecycle = SimpleLifecycleConfig{
		ShutdownTimeout:        fullConfig.Lifecycle.ShutdownTimeout,
		ForceShutdownTimeout:   fullConfig.Lifecycle.ForceShutdownTimeout,
		StartupTimeout:         fullConfig.Lifecycle.StartupTimeout,
		EnableGracefulShutdown: fullConfig.Lifecycle.EnableGracefulShutdown,
	}

	return simple
}

// convertLegacyToSimpleConfig converts legacy config to simple config
func convertLegacyToSimpleConfig(legacy *LegacyConfig) *SimpleConfig {
	simple := NewSimpleConfig()

	// Convert WebServer config
	simple.WebServer.Host = legacy.Web.Host
	simple.WebServer.Port = legacy.Web.Port
	simple.WebServer.EnableTLS = legacy.Web.EnableTLS
	simple.WebServer.TLS.CertFile = legacy.Web.TLSCertFile
	simple.WebServer.TLS.KeyFile = legacy.Web.TLSKeyFile
	simple.WebServer.StaticDir = legacy.Web.StaticDir
	simple.WebServer.EnableCORS = legacy.Web.EnableCORS

	// Convert auth config
	simple.WebServer.Auth.Enabled = legacy.Auth.Enabled
	simple.WebServer.Auth.DefaultRole = legacy.Auth.DefaultRole
	simple.WebServer.Auth.AdminUsers = legacy.Auth.AdminUsers

	// Convert GStreamer config
	simple.GStreamer.Capture = legacy.Capture
	simple.GStreamer.Encoding.Codec = GetCodecTypeFromString(legacy.Encoding.Codec)

	// Convert Metrics config
	simple.Metrics.External.Enabled = legacy.Monitoring.Enabled
	simple.Metrics.External.Port = legacy.Monitoring.MetricsPort

	// Convert lifecycle config
	simple.Lifecycle = SimpleLifecycleConfig{
		ShutdownTimeout:        legacy.Lifecycle.ShutdownTimeout,
		ForceShutdownTimeout:   legacy.Lifecycle.ForceShutdownTimeout,
		StartupTimeout:         legacy.Lifecycle.StartupTimeout,
		EnableGracefulShutdown: legacy.Lifecycle.EnableGracefulShutdown,
	}

	return simple
}

// SaveToFile saves the simple configuration to a file
func (c *SimpleConfig) SaveToFile(filename string) error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal simple config: %w", err)
	}

	if err := os.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("failed to write simple config file: %w", err)
	}

	return nil
}

// Merge merges another SimpleConfig into this one (direct assignment)
func (c *SimpleConfig) Merge(other *SimpleConfig) {
	if other == nil {
		return
	}

	// Direct merge without validation
	if other.GStreamer != nil {
		c.GStreamer = other.GStreamer
	}
	if other.WebRTC != nil {
		c.WebRTC = other.WebRTC
	}
	if other.WebServer != nil {
		c.WebServer = other.WebServer
	}
	if other.Metrics != nil {
		c.Metrics = other.Metrics
	}
	if other.Logging != nil {
		c.Logging = other.Logging
	}

	// Merge implementation config
	if other.Implementation.Type != "" {
		c.Implementation = other.Implementation
	}

	// Merge lifecycle config
	if other.Lifecycle.ShutdownTimeout != 0 {
		c.Lifecycle = other.Lifecycle
	}
}

// String returns a string representation of the simple config
func (c *SimpleConfig) String() string {
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

	return fmt.Sprintf("SimpleConfig{WebServer: %s, Capture: %s, Encoding: %s, Implementation: %s}",
		webInfo, captureInfo, encodingInfo, c.Implementation.Type)
}

// IsSimplifiedImplementation returns true if simplified implementation is enabled
func (c *SimpleConfig) IsSimplifiedImplementation() bool {
	return c.Implementation.UseSimplified || c.Implementation.Type == "simplified"
}

// GetLoggerWithPrefix creates a logger with the given prefix using the logging config
func (c *SimpleConfig) GetLoggerWithPrefix(prefix string) *logrus.Entry {
	logger := logrus.WithField("component", prefix)
	
	// Apply logging configuration if available
	if c.Logging != nil {
		// Set log level
		if level, err := logrus.ParseLevel(c.Logging.Level); err == nil {
			logrus.SetLevel(level)
		}
		
		// Set formatter
		if c.Logging.Format == "json" {
			logrus.SetFormatter(&logrus.JSONFormatter{})
		} else {
			logrus.SetFormatter(&logrus.TextFormatter{
				FullTimestamp: true,
			})
		}
	}
	
	return logger
}

// LoadSimpleConfigFromEnv loads configuration from environment variables
func LoadSimpleConfigFromEnv() *SimpleConfig {
	config := NewSimpleConfig()

	// Load GStreamer config from environment
	config.GStreamer.Capture = LoadDesktopCaptureConfigFromEnv()

	// Load logging config from environment
	config.Logging = LoadLoggingConfigFromEnv()

	return config
}

// CreateExampleSimpleConfig creates an example configuration file
func CreateExampleSimpleConfig(filename string) error {
	config := NewSimpleConfig()
	
	// Set some example values
	config.GStreamer.Capture.Width = 1920
	config.GStreamer.Capture.Height = 1080
	config.GStreamer.Capture.FrameRate = 30
	config.GStreamer.Encoding.Codec = "h264"
	config.GStreamer.Encoding.Bitrate = 2500
	
	config.WebServer.Host = "0.0.0.0"
	config.WebServer.Port = 8080
	
	config.Logging.Level = "info"
	config.Logging.Format = "text"
	
	return config.SaveToFile(filename)
}