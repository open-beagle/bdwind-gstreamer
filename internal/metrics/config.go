package metrics

// MetricsConfig 监控配置
type MetricsConfig struct {
	Enabled bool   `yaml:"enabled" json:"enabled"`
	Port    int    `yaml:"port" json:"port"`
	Path    string `yaml:"path" json:"path"`
	Host    string `yaml:"host" json:"host"`
}

// DefaultMetricsConfig 返回默认监控配置（保持向后兼容）
func DefaultMetricsConfig() MetricsConfig {
	return MetricsConfig{
		Enabled: false, // 默认禁用外部metrics暴露以提高安全性
		Port:    9090,
		Path:    "/metrics",
		Host:    "0.0.0.0",
	}
}

// Validate 验证配置
func (c *MetricsConfig) Validate() error {
	// 如果监控被禁用，跳过大部分验证
	if !c.Enabled {
		return nil
	}

	// 只在启用时验证端口等配置
	if c.Port <= 0 || c.Port > 65535 {
		return ErrInvalidPort
	}
	if c.Path == "" {
		c.Path = "/metrics"
	}
	if c.Host == "" {
		c.Host = "0.0.0.0"
	}
	return nil
}
