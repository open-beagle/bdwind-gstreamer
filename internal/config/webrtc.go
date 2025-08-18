package config

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// WebRTCConfig WebRTC配置模块
type WebRTCConfig struct {
	ICEServers    []ICEServerConfig `yaml:"ice_servers" json:"ice_servers"`
	SignalingPort int               `yaml:"signaling_port" json:"signaling_port"`
	MediaPort     int               `yaml:"media_port" json:"media_port"`
	SignalingPath string            `yaml:"signaling_path" json:"signaling_path"`
	EnableTURN    bool              `yaml:"enable_turn" json:"enable_turn"`
	TURNConfig    TURNConfig        `yaml:"turn" json:"turn"`
}

// ICEServerConfig ICE服务器配置
type ICEServerConfig struct {
	URLs       []string `yaml:"urls" json:"urls"`
	Username   string   `yaml:"username,omitempty" json:"username,omitempty"`
	Credential string   `yaml:"credential,omitempty" json:"credential,omitempty"`
}

// TURNConfig TURN服务器配置
type TURNConfig struct {
	Host     string `yaml:"host" json:"host"`
	Port     int    `yaml:"port" json:"port"`
	Username string `yaml:"username" json:"username"`
	Password string `yaml:"password" json:"password"`
	Realm    string `yaml:"realm" json:"realm"`
}

// DefaultWebRTCConfig 返回默认的WebRTC配置
func DefaultWebRTCConfig() *WebRTCConfig {
	config := &WebRTCConfig{}
	config.SetDefaults()
	return config
}

// SetDefaults 设置默认值
func (c *WebRTCConfig) SetDefaults() {
	// 默认ICE服务器配置（使用Google的公共STUN服务器）
	c.ICEServers = []ICEServerConfig{
		{
			URLs: []string{"stun:stun.l.google.com:19302"},
		},
		{
			URLs: []string{"stun:stun1.l.google.com:19302"},
		},
	}

	// 默认端口配置
	c.SignalingPort = 8081
	c.MediaPort = 8082
	c.SignalingPath = "/ws"

	// TURN服务器默认禁用
	c.EnableTURN = false
	c.TURNConfig = TURNConfig{
		Host:     "localhost",
		Port:     3478,
		Username: "",
		Password: "",
		Realm:    "bdwind",
	}
}

// Validate 验证配置
func (c *WebRTCConfig) Validate() error {
	// 验证信令端口
	if c.SignalingPort < 1 || c.SignalingPort > 65535 {
		return fmt.Errorf("invalid signaling port: %d (must be between 1 and 65535)", c.SignalingPort)
	}

	// 验证媒体端口
	if c.MediaPort < 1 || c.MediaPort > 65535 {
		return fmt.Errorf("invalid media port: %d (must be between 1 and 65535)", c.MediaPort)
	}

	// 验证端口不冲突
	if c.SignalingPort == c.MediaPort {
		return fmt.Errorf("signaling port and media port cannot be the same: %d", c.SignalingPort)
	}

	// 验证信令路径
	if c.SignalingPath == "" {
		return fmt.Errorf("signaling path cannot be empty")
	}
	if !strings.HasPrefix(c.SignalingPath, "/") {
		return fmt.Errorf("signaling path must start with '/': %s", c.SignalingPath)
	}

	// 验证ICE服务器配置
	if len(c.ICEServers) == 0 {
		return fmt.Errorf("at least one ICE server must be configured")
	}

	for i, server := range c.ICEServers {
		if err := c.validateICEServer(&server, i); err != nil {
			return fmt.Errorf("invalid ICE server %d: %w", i, err)
		}
	}

	// 验证TURN配置（如果启用）
	if c.EnableTURN {
		if err := c.validateTURNConfig(); err != nil {
			return fmt.Errorf("invalid TURN config: %w", err)
		}
	}

	return nil
}

// validateICEServer 验证ICE服务器配置
func (c *WebRTCConfig) validateICEServer(server *ICEServerConfig, index int) error {
	if len(server.URLs) == 0 {
		return fmt.Errorf("ICE server must have at least one URL")
	}

	for j, urlStr := range server.URLs {
		if err := c.validateICEServerURL(urlStr); err != nil {
			return fmt.Errorf("invalid URL %d: %w", j, err)
		}

		// 如果是TURN服务器，需要验证认证信息
		if strings.HasPrefix(urlStr, "turn:") || strings.HasPrefix(urlStr, "turns:") {
			if server.Username == "" {
				return fmt.Errorf("TURN server requires username")
			}
			if server.Credential == "" {
				return fmt.Errorf("TURN server requires credential")
			}
		}
	}

	return nil
}

// validateICEServerURL 验证ICE服务器URL
func (c *WebRTCConfig) validateICEServerURL(urlStr string) error {
	// 检查URL格式
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return fmt.Errorf("invalid URL format: %w", err)
	}

	// 检查协议
	validSchemes := []string{"stun", "stuns", "turn", "turns"}
	validScheme := false
	for _, scheme := range validSchemes {
		if parsedURL.Scheme == scheme {
			validScheme = true
			break
		}
	}
	if !validScheme {
		return fmt.Errorf("invalid scheme: %s (must be one of: %s)",
			parsedURL.Scheme, strings.Join(validSchemes, ", "))
	}

	// 对于STUN/TURN URL，主机名可能在Host或Opaque字段中
	var hostPart string
	if parsedURL.Host != "" {
		hostPart = parsedURL.Host
	} else if parsedURL.Opaque != "" {
		hostPart = parsedURL.Opaque
	} else {
		return fmt.Errorf("URL must have a host")
	}

	// 验证主机名和端口
	host, port, err := net.SplitHostPort(hostPart)
	if err != nil {
		// 如果没有端口，检查主机名
		if net.ParseIP(hostPart) == nil {
			// 不是IP地址，检查是否是有效的域名
			if len(hostPart) > 253 {
				return fmt.Errorf("hostname too long: %s", hostPart)
			}
		}
	} else {
		// 验证主机名
		if net.ParseIP(host) == nil && len(host) > 253 {
			return fmt.Errorf("hostname too long: %s", host)
		}

		// 验证端口
		if port == "" {
			return fmt.Errorf("port cannot be empty when specified")
		}
	}

	return nil
}

// validateTURNConfig 验证TURN配置
func (c *WebRTCConfig) validateTURNConfig() error {
	if c.TURNConfig.Host == "" {
		return fmt.Errorf("TURN host cannot be empty")
	}

	if c.TURNConfig.Port < 1 || c.TURNConfig.Port > 65535 {
		return fmt.Errorf("invalid TURN port: %d (must be between 1 and 65535)", c.TURNConfig.Port)
	}

	if c.TURNConfig.Username == "" {
		return fmt.Errorf("TURN username cannot be empty")
	}

	if c.TURNConfig.Password == "" {
		return fmt.Errorf("TURN password cannot be empty")
	}

	if c.TURNConfig.Realm == "" {
		return fmt.Errorf("TURN realm cannot be empty")
	}

	return nil
}

// Merge 合并其他配置模块
func (c *WebRTCConfig) Merge(other ConfigModule) error {
	otherConfig, ok := other.(*WebRTCConfig)
	if !ok {
		return fmt.Errorf("cannot merge different config types")
	}

	// 合并端口配置
	if otherConfig.SignalingPort != 0 && otherConfig.SignalingPort != 8081 {
		c.SignalingPort = otherConfig.SignalingPort
	}
	if otherConfig.MediaPort != 0 && otherConfig.MediaPort != 8082 {
		c.MediaPort = otherConfig.MediaPort
	}

	// 合并信令路径
	if otherConfig.SignalingPath != "" && otherConfig.SignalingPath != "/ws" {
		c.SignalingPath = otherConfig.SignalingPath
	}

	// 合并ICE服务器配置
	if len(otherConfig.ICEServers) > 0 {
		c.ICEServers = otherConfig.ICEServers
	}

	// 合并TURN配置
	if otherConfig.EnableTURN != c.EnableTURN {
		c.EnableTURN = otherConfig.EnableTURN
	}
	if otherConfig.TURNConfig.Host != "" && otherConfig.TURNConfig.Host != "localhost" {
		c.TURNConfig.Host = otherConfig.TURNConfig.Host
	}
	if otherConfig.TURNConfig.Port != 0 && otherConfig.TURNConfig.Port != 3478 {
		c.TURNConfig.Port = otherConfig.TURNConfig.Port
	}
	if otherConfig.TURNConfig.Username != "" {
		c.TURNConfig.Username = otherConfig.TURNConfig.Username
	}
	if otherConfig.TURNConfig.Password != "" {
		c.TURNConfig.Password = otherConfig.TURNConfig.Password
	}
	if otherConfig.TURNConfig.Realm != "" && otherConfig.TURNConfig.Realm != "bdwind" {
		c.TURNConfig.Realm = otherConfig.TURNConfig.Realm
	}

	return nil
}

// AddICEServer 添加ICE服务器
func (c *WebRTCConfig) AddICEServer(urls []string, username, credential string) error {
	server := ICEServerConfig{
		URLs:       urls,
		Username:   username,
		Credential: credential,
	}

	// 验证新的ICE服务器
	if err := c.validateICEServer(&server, len(c.ICEServers)); err != nil {
		return fmt.Errorf("invalid ICE server: %w", err)
	}

	c.ICEServers = append(c.ICEServers, server)
	return nil
}

// RemoveICEServer 移除ICE服务器
func (c *WebRTCConfig) RemoveICEServer(index int) error {
	if index < 0 || index >= len(c.ICEServers) {
		return fmt.Errorf("invalid ICE server index: %d", index)
	}

	// 确保至少保留一个ICE服务器
	if len(c.ICEServers) <= 1 {
		return fmt.Errorf("cannot remove the last ICE server")
	}

	c.ICEServers = append(c.ICEServers[:index], c.ICEServers[index+1:]...)
	return nil
}

// GetSTUNServers 获取STUN服务器列表
func (c *WebRTCConfig) GetSTUNServers() []string {
	var stunServers []string
	for _, server := range c.ICEServers {
		for _, url := range server.URLs {
			if strings.HasPrefix(url, "stun:") || strings.HasPrefix(url, "stuns:") {
				stunServers = append(stunServers, url)
			}
		}
	}
	return stunServers
}

// GetTURNServers 获取TURN服务器列表
func (c *WebRTCConfig) GetTURNServers() []ICEServerConfig {
	var turnServers []ICEServerConfig
	for _, server := range c.ICEServers {
		hasTURN := false
		for _, url := range server.URLs {
			if strings.HasPrefix(url, "turn:") || strings.HasPrefix(url, "turns:") {
				hasTURN = true
				break
			}
		}
		if hasTURN {
			turnServers = append(turnServers, server)
		}
	}
	return turnServers
}
