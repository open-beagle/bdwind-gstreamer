package config

import (
	"fmt"
)

// SimpleConfigAdapter provides an adapter interface for creating simplified managers
// This allows seamless switching between full config and simple config
type SimpleConfigAdapter struct {
	simpleConfig *SimpleConfig
	fullConfig   *Config
	useSimple    bool
}

// NewSimpleConfigAdapter creates a new adapter from SimpleConfig
func NewSimpleConfigAdapter(cfg *SimpleConfig) *SimpleConfigAdapter {
	return &SimpleConfigAdapter{
		simpleConfig: cfg,
		useSimple:    true,
	}
}

// NewSimpleConfigAdapterFromFull creates a new adapter from full Config
func NewSimpleConfigAdapterFromFull(cfg *Config) *SimpleConfigAdapter {
	return &SimpleConfigAdapter{
		fullConfig: cfg,
		useSimple:  cfg.Implementation.UseSimplified,
	}
}

// GetGStreamerConfig returns GStreamer configuration
func (a *SimpleConfigAdapter) GetGStreamerConfig() *GStreamerConfig {
	if a.useSimple && a.simpleConfig != nil {
		return a.simpleConfig.GetGStreamerConfig()
	}
	if a.fullConfig != nil {
		return a.fullConfig.GetGStreamerConfig()
	}
	return DefaultGStreamerConfig()
}

// GetWebRTCConfig returns WebRTC configuration
func (a *SimpleConfigAdapter) GetWebRTCConfig() *WebRTCConfig {
	if a.useSimple && a.simpleConfig != nil {
		return a.simpleConfig.GetWebRTCConfig()
	}
	if a.fullConfig != nil {
		return a.fullConfig.GetWebRTCConfig()
	}
	return DefaultWebRTCConfig()
}

// GetWebServerConfig returns WebServer configuration
func (a *SimpleConfigAdapter) GetWebServerConfig() *WebServerConfig {
	if a.useSimple && a.simpleConfig != nil {
		return a.simpleConfig.GetWebServerConfig()
	}
	if a.fullConfig != nil {
		return a.fullConfig.GetWebServerConfig()
	}
	return DefaultWebServerConfig()
}

// GetMetricsConfig returns Metrics configuration
func (a *SimpleConfigAdapter) GetMetricsConfig() *MetricsConfig {
	if a.useSimple && a.simpleConfig != nil {
		return a.simpleConfig.GetMetricsConfig()
	}
	if a.fullConfig != nil {
		return a.fullConfig.GetMetricsConfig()
	}
	return DefaultMetricsConfig()
}

// GetLoggingConfig returns Logging configuration
func (a *SimpleConfigAdapter) GetLoggingConfig() *LoggingConfig {
	if a.useSimple && a.simpleConfig != nil {
		return a.simpleConfig.GetLoggingConfig()
	}
	if a.fullConfig != nil {
		return a.fullConfig.GetLoggingConfig()
	}
	return DefaultLoggingConfig()
}

// IsSimplifiedImplementation returns true if simplified implementation should be used
func (a *SimpleConfigAdapter) IsSimplifiedImplementation() bool {
	if a.simpleConfig != nil {
		return a.simpleConfig.IsSimplifiedImplementation()
	}
	if a.fullConfig != nil {
		return a.fullConfig.Implementation.UseSimplified
	}
	return true // Default to simplified
}

// GetSimpleConfig returns the SimpleConfig if available, otherwise converts from full config
func (a *SimpleConfigAdapter) GetSimpleConfig() *SimpleConfig {
	if a.simpleConfig != nil {
		return a.simpleConfig
	}
	if a.fullConfig != nil {
		return ConvertToSimpleConfig(a.fullConfig)
	}
	return NewSimpleConfig()
}

// GetFullConfig returns the full Config if available, otherwise converts from simple config
func (a *SimpleConfigAdapter) GetFullConfig() *Config {
	if a.fullConfig != nil {
		return a.fullConfig
	}
	if a.simpleConfig != nil {
		return a.convertSimpleToFullConfig()
	}
	return DefaultConfig()
}

// convertSimpleToFullConfig converts SimpleConfig to full Config
func (a *SimpleConfigAdapter) convertSimpleToFullConfig() *Config {
	full := DefaultConfig()
	
	if a.simpleConfig.GStreamer != nil {
		full.GStreamer = a.simpleConfig.GStreamer
	}
	if a.simpleConfig.WebRTC != nil {
		full.WebRTC = a.simpleConfig.WebRTC
	}
	if a.simpleConfig.WebServer != nil {
		full.WebServer = a.simpleConfig.WebServer
	}
	if a.simpleConfig.Metrics != nil {
		full.Metrics = a.simpleConfig.Metrics
	}
	if a.simpleConfig.Logging != nil {
		full.Logging = a.simpleConfig.Logging
	}
	
	// Convert implementation config
	full.Implementation = a.simpleConfig.Implementation
	
	// Convert lifecycle config
	full.Lifecycle.ShutdownTimeout = a.simpleConfig.Lifecycle.ShutdownTimeout
	full.Lifecycle.ForceShutdownTimeout = a.simpleConfig.Lifecycle.ForceShutdownTimeout
	full.Lifecycle.StartupTimeout = a.simpleConfig.Lifecycle.StartupTimeout
	full.Lifecycle.EnableGracefulShutdown = a.simpleConfig.Lifecycle.EnableGracefulShutdown
	
	return full
}

// String returns a string representation of the adapter
func (a *SimpleConfigAdapter) String() string {
	if a.useSimple && a.simpleConfig != nil {
		return fmt.Sprintf("SimpleConfigAdapter{simple: %s}", a.simpleConfig.String())
	}
	if a.fullConfig != nil {
		return fmt.Sprintf("SimpleConfigAdapter{full: %s}", a.fullConfig.String())
	}
	return "SimpleConfigAdapter{empty}"
}

// ConfigCompatibilityChecker provides methods to check configuration compatibility
type ConfigCompatibilityChecker struct{}

// NewConfigCompatibilityChecker creates a new compatibility checker
func NewConfigCompatibilityChecker() *ConfigCompatibilityChecker {
	return &ConfigCompatibilityChecker{}
}

// CheckSimpleConfigCompatibility checks if a SimpleConfig is compatible with the current system
func (c *ConfigCompatibilityChecker) CheckSimpleConfigCompatibility(cfg *SimpleConfig) []string {
	var issues []string
	
	if cfg == nil {
		issues = append(issues, "configuration is nil")
		return issues
	}
	
	// Check GStreamer config
	if cfg.GStreamer != nil {
		if cfg.GStreamer.Capture.Width <= 0 || cfg.GStreamer.Capture.Height <= 0 {
			issues = append(issues, "invalid video resolution")
		}
		if cfg.GStreamer.Capture.FrameRate <= 0 {
			issues = append(issues, "invalid frame rate")
		}
		if cfg.GStreamer.Encoding.Bitrate <= 0 {
			issues = append(issues, "invalid bitrate")
		}
	}
	
	// Check WebRTC config
	if cfg.WebRTC != nil {
		if len(cfg.WebRTC.ICEServers) == 0 {
			issues = append(issues, "no ICE servers configured")
		}
	}
	
	// Check WebServer config
	if cfg.WebServer != nil {
		if cfg.WebServer.Port <= 0 || cfg.WebServer.Port > 65535 {
			issues = append(issues, "invalid web server port")
		}
	}
	
	return issues
}

// CheckFullConfigCompatibility checks if a full Config is compatible with simplified implementation
func (c *ConfigCompatibilityChecker) CheckFullConfigCompatibility(cfg *Config) []string {
	var issues []string
	
	if cfg == nil {
		issues = append(issues, "configuration is nil")
		return issues
	}
	
	// Check if simplified implementation is enabled
	if !cfg.Implementation.UseSimplified {
		issues = append(issues, "simplified implementation is disabled")
	}
	
	// Check for complex features that might not be supported in simplified mode
	if cfg.WebServer != nil && cfg.WebServer.Auth.Enabled {
		issues = append(issues, "authentication may have limited support in simplified mode")
	}
	
	if cfg.Metrics != nil && cfg.Metrics.External.Enabled {
		issues = append(issues, "external metrics may have limited support in simplified mode")
	}
	
	return issues
}

// MigrateConfigFormat migrates configuration from one format to another
func (c *ConfigCompatibilityChecker) MigrateConfigFormat(from interface{}, to string) (interface{}, error) {
	switch to {
	case "simple":
		switch cfg := from.(type) {
		case *Config:
			return ConvertToSimpleConfig(cfg), nil
		case *SimpleConfig:
			return cfg, nil
		default:
			return nil, fmt.Errorf("unsupported source config type")
		}
	case "full":
		switch cfg := from.(type) {
		case *SimpleConfig:
			adapter := NewSimpleConfigAdapter(cfg)
			return adapter.GetFullConfig(), nil
		case *Config:
			return cfg, nil
		default:
			return nil, fmt.Errorf("unsupported source config type")
		}
	default:
		return nil, fmt.Errorf("unsupported target format: %s", to)
	}
}