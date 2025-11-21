package config

import (
	"fmt"
	"time"
)

// MetricsConfig Metrics配置模块
type MetricsConfig struct {
	// 内部监控配置（始终启用，为web页面提供数据）
	Internal InternalMetricsConfig `yaml:"internal" json:"internal"`

	// 外部暴露配置（默认禁用，为Grafana等外部工具提供数据）
	External ExternalMetricsConfig `yaml:"external" json:"external"`
}

// InternalMetricsConfig 内部监控配置
type InternalMetricsConfig struct {
	Enabled            bool          `yaml:"enabled" json:"enabled"`
	CollectionInterval time.Duration `yaml:"collection_interval" json:"collection_interval"`
	BufferSize         int           `yaml:"buffer_size" json:"buffer_size"`
	RetentionPeriod    time.Duration `yaml:"retention_period" json:"retention_period"`
}

// ExternalMetricsConfig 外部监控配置
type ExternalMetricsConfig struct {
	Enabled bool   `yaml:"enabled" json:"enabled"`
	Port    int    `yaml:"port" json:"port"`
	Path    string `yaml:"path" json:"path"`
	Host    string `yaml:"host" json:"host"`
}

// DefaultMetricsConfig 返回默认的Metrics配置
func DefaultMetricsConfig() *MetricsConfig {
	config := &MetricsConfig{}
	config.SetDefaults()
	return config
}

// SetDefaults 设置默认值
func (c *MetricsConfig) SetDefaults() {
	// 内部监控默认配置（始终启用）
	c.Internal = InternalMetricsConfig{
		Enabled:            true,
		CollectionInterval: 10 * time.Second,
		BufferSize:         1000,
		RetentionPeriod:    1 * time.Hour,
	}

	// 外部暴露默认配置（默认禁用以提高安全性）
	c.External = ExternalMetricsConfig{
		Enabled: false, // 默认禁用外部metrics暴露
		Port:    9090,
		Path:    "/metrics",
		Host:    "0.0.0.0",
	}
}

// Validate 验证配置
func (c *MetricsConfig) Validate() error {
	// 验证内部监控配置
	if err := c.validateInternalConfig(); err != nil {
		return fmt.Errorf("invalid internal metrics config: %w", err)
	}

	// 验证外部暴露配置（仅在启用时验证）
	if c.External.Enabled {
		if err := c.validateExternalConfig(); err != nil {
			return fmt.Errorf("invalid external metrics config: %w", err)
		}
	}

	return nil
}

// validateInternalConfig 验证内部监控配置
func (c *MetricsConfig) validateInternalConfig() error {
	// 内部监控必须启用
	if !c.Internal.Enabled {
		return fmt.Errorf("internal metrics cannot be disabled")
	}

	// 验证收集间隔
	if c.Internal.CollectionInterval <= 0 {
		return fmt.Errorf("collection interval must be positive, got: %v", c.Internal.CollectionInterval)
	}
	if c.Internal.CollectionInterval < time.Second {
		return fmt.Errorf("collection interval too short: %v (minimum: 1s)", c.Internal.CollectionInterval)
	}
	if c.Internal.CollectionInterval > 5*time.Minute {
		return fmt.Errorf("collection interval too long: %v (maximum: 5m)", c.Internal.CollectionInterval)
	}

	// 验证缓冲区大小
	if c.Internal.BufferSize <= 0 {
		return fmt.Errorf("buffer size must be positive, got: %d", c.Internal.BufferSize)
	}
	if c.Internal.BufferSize > 10000 {
		return fmt.Errorf("buffer size too large: %d (maximum: 10000)", c.Internal.BufferSize)
	}

	// 验证保留期
	if c.Internal.RetentionPeriod <= 0 {
		return fmt.Errorf("retention period must be positive, got: %v", c.Internal.RetentionPeriod)
	}
	if c.Internal.RetentionPeriod < 10*time.Minute {
		return fmt.Errorf("retention period too short: %v (minimum: 10m)", c.Internal.RetentionPeriod)
	}
	if c.Internal.RetentionPeriod > 24*time.Hour {
		return fmt.Errorf("retention period too long: %v (maximum: 24h)", c.Internal.RetentionPeriod)
	}

	return nil
}

// validateExternalConfig 验证外部暴露配置
func (c *MetricsConfig) validateExternalConfig() error {
	// 验证端口
	if c.External.Port < 1 || c.External.Port > 65535 {
		return fmt.Errorf("invalid port: %d (must be between 1 and 65535)", c.External.Port)
	}

	// 验证路径
	if c.External.Path == "" {
		return fmt.Errorf("path cannot be empty")
	}
	if c.External.Path[0] != '/' {
		return fmt.Errorf("path must start with '/': %s", c.External.Path)
	}

	// 验证主机地址
	if c.External.Host == "" {
		return fmt.Errorf("host cannot be empty")
	}

	return nil
}

// Merge 合并其他配置模块
func (c *MetricsConfig) Merge(other ConfigModule) error {
	otherConfig, ok := other.(*MetricsConfig)
	if !ok {
		return fmt.Errorf("cannot merge different config types")
	}

	// 合并内部监控配置
	// 注意：内部监控的Enabled字段不允许合并为false
	if otherConfig.Internal.CollectionInterval != 0 && otherConfig.Internal.CollectionInterval != 10*time.Second {
		c.Internal.CollectionInterval = otherConfig.Internal.CollectionInterval
	}
	if otherConfig.Internal.BufferSize != 0 && otherConfig.Internal.BufferSize != 1000 {
		c.Internal.BufferSize = otherConfig.Internal.BufferSize
	}
	if otherConfig.Internal.RetentionPeriod != 0 && otherConfig.Internal.RetentionPeriod != 1*time.Hour {
		c.Internal.RetentionPeriod = otherConfig.Internal.RetentionPeriod
	}

	// 合并外部暴露配置
	if otherConfig.External.Enabled != c.External.Enabled {
		c.External.Enabled = otherConfig.External.Enabled
	}
	if otherConfig.External.Port != 0 && otherConfig.External.Port != 9090 {
		c.External.Port = otherConfig.External.Port
	}
	if otherConfig.External.Path != "" && otherConfig.External.Path != "/metrics" {
		c.External.Path = otherConfig.External.Path
	}
	if otherConfig.External.Host != "" && otherConfig.External.Host != "0.0.0.0" {
		c.External.Host = otherConfig.External.Host
	}

	return nil
}

// EnableExternalMetrics 启用外部metrics暴露
func (c *MetricsConfig) EnableExternalMetrics(port int, path, host string) error {
	c.External.Enabled = true

	if port != 0 {
		c.External.Port = port
	}
	if path != "" {
		c.External.Path = path
	}
	if host != "" {
		c.External.Host = host
	}

	// 验证配置
	return c.validateExternalConfig()
}

// DisableExternalMetrics 禁用外部metrics暴露
func (c *MetricsConfig) DisableExternalMetrics() {
	c.External.Enabled = false
}

// IsExternalEnabled 检查外部metrics是否启用
func (c *MetricsConfig) IsExternalEnabled() bool {
	return c.External.Enabled
}

// IsInternalEnabled 检查内部metrics是否启用（始终为true）
func (c *MetricsConfig) IsInternalEnabled() bool {
	return c.Internal.Enabled
}

// GetExternalEndpoint 获取外部metrics端点
func (c *MetricsConfig) GetExternalEndpoint() string {
	if !c.External.Enabled {
		return ""
	}
	return fmt.Sprintf("http://%s:%d%s", c.External.Host, c.External.Port, c.External.Path)
}

// GetCollectionSettings 获取收集设置
func (c *MetricsConfig) GetCollectionSettings() (time.Duration, int, time.Duration) {
	return c.Internal.CollectionInterval, c.Internal.BufferSize, c.Internal.RetentionPeriod
}

// UpdateCollectionInterval 更新收集间隔
func (c *MetricsConfig) UpdateCollectionInterval(interval time.Duration) error {
	if interval <= 0 {
		return fmt.Errorf("collection interval must be positive")
	}
	if interval < time.Second {
		return fmt.Errorf("collection interval too short: %v (minimum: 1s)", interval)
	}
	if interval > 5*time.Minute {
		return fmt.Errorf("collection interval too long: %v (maximum: 5m)", interval)
	}

	c.Internal.CollectionInterval = interval
	return nil
}

// UpdateBufferSize 更新缓冲区大小
func (c *MetricsConfig) UpdateBufferSize(size int) error {
	if size <= 0 {
		return fmt.Errorf("buffer size must be positive")
	}
	if size > 10000 {
		return fmt.Errorf("buffer size too large: %d (maximum: 10000)", size)
	}

	c.Internal.BufferSize = size
	return nil
}

// UpdateRetentionPeriod 更新保留期
func (c *MetricsConfig) UpdateRetentionPeriod(period time.Duration) error {
	if period <= 0 {
		return fmt.Errorf("retention period must be positive")
	}
	if period < 10*time.Minute {
		return fmt.Errorf("retention period too short: %v (minimum: 10m)", period)
	}
	if period > 24*time.Hour {
		return fmt.Errorf("retention period too long: %v (maximum: 24h)", period)
	}

	c.Internal.RetentionPeriod = period
	return nil
}
