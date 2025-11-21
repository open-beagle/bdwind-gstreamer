package config

import (
	"fmt"
	"time"
)

// ConfigModule 配置模块接口
type ConfigModule interface {
	Validate() error
	SetDefaults()
	Merge(other ConfigModule) error
}

// WebServerConfig Web服务器配置模块
type WebServerConfig struct {
	Host        string     `yaml:"host" json:"host"`
	Port        int        `yaml:"port" json:"port"`
	EnableTLS   bool       `yaml:"enable_tls" json:"enable_tls"`
	TLS         TLSConfig  `yaml:"tls" json:"tls"`
	StaticDir   string     `yaml:"static_dir" json:"static_dir"`
	DefaultFile string     `yaml:"default_file" json:"default_file"`
	EnableCORS  bool       `yaml:"enable_cors" json:"enable_cors"`
	Auth        AuthConfig `yaml:"auth" json:"auth"`
}

// TLSConfig TLS配置
type TLSConfig struct {
	CertFile string `yaml:"cert_file" json:"cert_file"`
	KeyFile  string `yaml:"key_file" json:"key_file"`
}

// AuthConfig 认证配置
type AuthConfig struct {
	Enabled     bool          `yaml:"enabled" json:"enabled"`
	DefaultRole string        `yaml:"default_role" json:"default_role"`
	AdminUsers  []string      `yaml:"admin_users" json:"admin_users"`
	TokenExpiry time.Duration `yaml:"token_expiry" json:"token_expiry"`
}

// DefaultWebServerConfig 返回默认的WebServer配置
func DefaultWebServerConfig() *WebServerConfig {
	config := &WebServerConfig{}
	config.SetDefaults()
	return config
}

// SetDefaults 设置默认值
func (c *WebServerConfig) SetDefaults() {
	c.Host = "0.0.0.0"
	c.Port = 8080
	c.EnableTLS = false
	c.StaticDir = "web/dist"
	c.DefaultFile = "index.html"
	c.EnableCORS = true

	// TLS默认配置
	c.TLS = TLSConfig{
		CertFile: "",
		KeyFile:  "",
	}

	// 认证默认配置
	c.Auth = AuthConfig{
		Enabled:     false,
		DefaultRole: "viewer",
		AdminUsers:  []string{"admin"},
		TokenExpiry: 24 * time.Hour,
	}
}

// Validate 验证配置
func (c *WebServerConfig) Validate() error {
	// 验证端口
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("invalid server port: %d (must be between 1 and 65535)", c.Port)
	}

	// 验证主机地址
	if c.Host == "" {
		return fmt.Errorf("host cannot be empty")
	}

	// 验证TLS配置
	if c.EnableTLS {
		if c.TLS.CertFile == "" {
			return fmt.Errorf("TLS cert file is required when TLS is enabled")
		}
		if c.TLS.KeyFile == "" {
			return fmt.Errorf("TLS key file is required when TLS is enabled")
		}
	}

	// 验证静态文件目录
	if c.StaticDir == "" {
		return fmt.Errorf("static directory cannot be empty")
	}

	// 验证默认文件
	if c.DefaultFile == "" {
		return fmt.Errorf("default file cannot be empty")
	}

	// 验证认证配置
	if err := c.validateAuthConfig(); err != nil {
		return fmt.Errorf("invalid auth config: %w", err)
	}

	return nil
}

// validateAuthConfig 验证认证配置
func (c *WebServerConfig) validateAuthConfig() error {
	if c.Auth.Enabled {
		// 验证默认角色
		if c.Auth.DefaultRole == "" {
			return fmt.Errorf("default role cannot be empty when auth is enabled")
		}

		// 验证管理员用户列表
		if len(c.Auth.AdminUsers) == 0 {
			return fmt.Errorf("at least one admin user must be specified when auth is enabled")
		}

		// 验证token过期时间
		if c.Auth.TokenExpiry <= 0 {
			return fmt.Errorf("token expiry must be positive, got: %v", c.Auth.TokenExpiry)
		}
	}

	return nil
}

// Merge 合并其他配置模块
func (c *WebServerConfig) Merge(other ConfigModule) error {
	otherConfig, ok := other.(*WebServerConfig)
	if !ok {
		return fmt.Errorf("cannot merge different config types")
	}

	// 合并非零值
	if otherConfig.Host != "" && otherConfig.Host != "0.0.0.0" {
		c.Host = otherConfig.Host
	}
	if otherConfig.Port != 0 && otherConfig.Port != 8080 {
		c.Port = otherConfig.Port
	}
	if otherConfig.EnableTLS {
		c.EnableTLS = otherConfig.EnableTLS
	}
	if otherConfig.TLS.CertFile != "" {
		c.TLS.CertFile = otherConfig.TLS.CertFile
	}
	if otherConfig.TLS.KeyFile != "" {
		c.TLS.KeyFile = otherConfig.TLS.KeyFile
	}
	if otherConfig.StaticDir != "" && otherConfig.StaticDir != "web/dist" {
		c.StaticDir = otherConfig.StaticDir
	}
	if otherConfig.DefaultFile != "" && otherConfig.DefaultFile != "index.html" {
		c.DefaultFile = otherConfig.DefaultFile
	}
	if otherConfig.EnableCORS {
		c.EnableCORS = otherConfig.EnableCORS
	}

	// 合并认证配置
	if otherConfig.Auth.Enabled != c.Auth.Enabled {
		c.Auth.Enabled = otherConfig.Auth.Enabled
	}
	if otherConfig.Auth.DefaultRole != "" && otherConfig.Auth.DefaultRole != "viewer" {
		c.Auth.DefaultRole = otherConfig.Auth.DefaultRole
	}
	if len(otherConfig.Auth.AdminUsers) > 0 {
		c.Auth.AdminUsers = otherConfig.Auth.AdminUsers
	}
	if otherConfig.Auth.TokenExpiry != 0 && otherConfig.Auth.TokenExpiry != 24*time.Hour {
		c.Auth.TokenExpiry = otherConfig.Auth.TokenExpiry
	}

	return nil
}
