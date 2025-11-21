package webserver

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/open-beagle/bdwind-gstreamer/internal/common/config"
	"github.com/sirupsen/logrus"
)

// Manager webserver组件管理器
// 实现 ComponentManager 接口，管理web服务器功能
type Manager struct {
	config          *config.WebServerConfig
	server          *http.Server
	webServer       *WebServer
	signalingServer *SignalingServer
	logger          *logrus.Entry
	running         bool
	startTime       time.Time
	mutex           sync.RWMutex
	ctx             context.Context
	cancel          context.CancelFunc
}

// NewManager 创建新的webserver管理器
func NewManager(ctx context.Context, cfg *config.WebServerConfig) (*Manager, error) {
	if ctx == nil {
		return nil, fmt.Errorf("context is required")
	}

	if cfg == nil {
		return nil, fmt.Errorf("webserver config cannot be nil")
	}

	// 验证配置
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid webserver config: %w", err)
	}

	// 获取 logrus entry 用于结构化日志记录
	logger := config.GetLoggerWithPrefix("webserver")

	// 创建webserver实例
	webServer, err := NewWebServer(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create webserver: %w", err)
	}

	// 创建信令服务器实例
	signalingServer := NewSignalingServer()

	// 创建子context
	childCtx, cancel := context.WithCancel(ctx)

	return &Manager{
		config:          cfg,
		webServer:       webServer,
		signalingServer: signalingServer,
		logger:          logger,
		ctx:             childCtx,
		cancel:          cancel,
	}, nil
}

// Start 启动webserver管理器
func (m *Manager) Start(ctx context.Context) error {
	startTime := time.Now()
	m.logger.Info("Starting webserver manager...")

	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.running {
		m.logger.Warn("Webserver manager already running, skipping start")
		return fmt.Errorf("webserver manager already running")
	}

	// 记录配置信息
	addr := fmt.Sprintf("%s:%d", m.config.Host, m.config.Port)
	m.logger.Debugf("Webserver configuration: host=%s, port=%d, tls=%v, auth=%v, cors=%v",
		m.config.Host, m.config.Port, m.config.EnableTLS, m.config.Auth.Enabled, m.config.EnableCORS)

	// TLS配置验证和记录
	if m.config.EnableTLS {
		m.logger.Debug("TLS enabled, validating certificate files...")
		m.logger.Debugf("TLS certificate file: %s", m.config.TLS.CertFile)
		m.logger.Debugf("TLS key file: %s", m.config.TLS.KeyFile)

		// 验证TLS文件是否存在
		if m.config.TLS.CertFile == "" || m.config.TLS.KeyFile == "" {
			m.logger.Error("TLS enabled but certificate or key file not specified")
			return fmt.Errorf("TLS certificate or key file not specified")
		}
	} else {
		m.logger.Debug("TLS disabled, using HTTP")
	}

	// 启动信令服务器
	m.logger.Debug("Starting signaling server...")
	if err := m.signalingServer.Start(ctx); err != nil {
		m.logger.Errorf("Failed to start signaling server: %v", err)
		return fmt.Errorf("failed to start signaling server: %w", err)
	}
	m.logger.Debug("Signaling server started successfully")

	// 注册信令服务器到webserver
	m.logger.Debug("Registering signaling server with webserver...")
	if err := m.webServer.RegisterComponent("signaling", m.signalingServer); err != nil {
		m.logger.Errorf("Failed to register signaling server: %v", err)
		return fmt.Errorf("failed to register signaling server: %w", err)
	}
	m.logger.Debug("Signaling server registered successfully")

	// 获取处理器
	m.logger.Debug("Setting up webserver handler...")
	handler := m.webServer.GetHandler()
	if handler == nil {
		m.logger.Error("Failed to get webserver handler")
		return fmt.Errorf("webserver handler is nil")
	}
	m.logger.Debug("Webserver handler obtained successfully")

	// 创建HTTP服务器
	m.logger.Debug("Creating HTTP server instance...")
	m.server = &http.Server{
		Addr:    addr,
		Handler: handler,
	}
	m.logger.Debugf("HTTP server created for address: %s", addr)

	// 启动服务器监听
	protocol := "http"
	if m.config.EnableTLS {
		protocol = "https"
	}

	m.logger.Infof("Starting %s server on %s...", protocol, addr)

	// 在goroutine中启动服务器
	serverStarted := make(chan error, 1)
	go func() {
		m.logger.Tracef("Attempting to bind to %s", addr)

		var err error
		if m.config.EnableTLS {
			m.logger.Trace("Starting HTTPS server with TLS...")
			err = m.server.ListenAndServeTLS(m.config.TLS.CertFile, m.config.TLS.KeyFile)
		} else {
			m.logger.Trace("Starting HTTP server...")
			err = m.server.ListenAndServe()
		}

		if err != nil && err != http.ErrServerClosed {
			m.logger.Errorf("Server failed to start: %v", err)
			serverStarted <- err
		} else if err == http.ErrServerClosed {
			m.logger.Debug("Server closed normally")
		}
	}()

	// 等待服务器启动并验证
	m.logger.Debug("Waiting for server to start...")
	time.Sleep(100 * time.Millisecond)

	// 检查是否有启动错误
	select {
	case err := <-serverStarted:
		m.logger.Errorf("Server startup failed: %v", err)
		return fmt.Errorf("failed to start webserver: %w", err)
	default:
		// 没有错误，继续
	}

	// 标记为运行状态
	m.running = true
	m.startTime = time.Now()

	duration := time.Since(startTime)
	m.logger.Infof("Webserver started successfully on %s://%s in %v", protocol, addr, duration)

	// 记录服务状态
	m.logger.Infof("Webserver ready to accept connections")
	if m.config.Auth.Enabled {
		m.logger.Info("Authentication is enabled")
	}
	if m.config.EnableCORS {
		m.logger.Info("CORS is enabled")
	}

	return nil
}

// Stop 停止webserver管理器
func (m *Manager) Stop(ctx context.Context) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if !m.running {
		return nil
	}

	m.logger.Debug("Stopping webserver manager...")

	// 取消context以通知所有子组件停止
	if m.cancel != nil {
		m.cancel()
	}

	// 停止信令服务器
	if m.signalingServer != nil {
		m.logger.Debug("Stopping signaling server...")
		if err := m.signalingServer.Stop(ctx); err != nil {
			m.logger.Errorf("Error stopping signaling server: %v", err)
		} else {
			m.logger.Debug("Signaling server stopped successfully")
		}
	}

	// 优雅关闭HTTP服务器
	if m.server != nil {
		m.logger.Trace("Shutting down HTTP server...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// 在goroutine中执行shutdown，避免阻塞
		shutdownDone := make(chan error, 1)
		go func() {
			shutdownDone <- m.server.Shutdown(shutdownCtx)
		}()

		select {
		case err := <-shutdownDone:
			if err != nil {
				m.logger.Errorf("Error during server shutdown: %v", err)
				// 不返回错误，继续清理其他资源
			} else {
				m.logger.Trace("HTTP server shutdown successfully")
			}
		case <-time.After(6 * time.Second):
			m.logger.Warnf("HTTP server shutdown timeout, forcing close")
			// 强制关闭服务器
			if err := m.server.Close(); err != nil {
				m.logger.Errorf("Error during server force close: %v", err)
			}
		}
	}

	m.running = false
	m.logger.Trace("Webserver manager stopped successfully")
	return nil
}

// IsEnabled 检查webserver组件是否启用
// webserver是核心组件，始终启用
func (m *Manager) IsEnabled() bool {
	return true
}

// IsRunning 检查webserver管理器是否正在运行
func (m *Manager) IsRunning() bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.running
}

// GetStats 获取webserver管理器的统计信息
func (m *Manager) GetStats() map[string]interface{} {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	stats := map[string]interface{}{
		"running":      m.running,
		"start_time":   m.startTime.Unix(),
		"uptime":       time.Since(m.startTime).Seconds(),
		"address":      fmt.Sprintf("%s:%d", m.config.Host, m.config.Port),
		"tls_enabled":  m.config.EnableTLS,
		"auth_enabled": m.config.Auth.Enabled,
		"cors_enabled": m.config.EnableCORS,
	}

	// 添加信令服务器统计信息
	if m.signalingServer != nil {
		stats["signaling_server"] = m.signalingServer.GetStats()
	}

	return stats
}

// GetContext 获取组件的上下文
func (m *Manager) GetContext() context.Context {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.ctx
}

// GetWebServer 获取webserver实例（用于其他组件集成）
func (m *Manager) GetWebServer() *WebServer {
	return m.webServer
}

// GetConfig 获取webserver配置
func (m *Manager) GetConfig() *config.WebServerConfig {
	return m.config
}

// UpdateConfig 更新webserver配置（需要重启才能生效）
func (m *Manager) UpdateConfig(newConfig *config.WebServerConfig) error {
	if newConfig == nil {
		return fmt.Errorf("config cannot be nil")
	}

	if err := newConfig.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.running {
		return fmt.Errorf("cannot update config while webserver is running")
	}

	m.config = newConfig

	// 重新创建webserver实例
	webServer, err := NewWebServer(newConfig)
	if err != nil {
		return fmt.Errorf("failed to create webserver with new config: %w", err)
	}

	m.webServer = webServer
	m.logger.Trace("Webserver config updated successfully")
	return nil
}

// GetAddress 获取服务器地址
func (m *Manager) GetAddress() string {
	protocol := "http"
	if m.config.EnableTLS {
		protocol = "https"
	}
	return fmt.Sprintf("%s://%s:%d", protocol, m.config.Host, m.config.Port)
}

// IsAuthEnabled 检查认证是否启用
func (m *Manager) IsAuthEnabled() bool {
	return m.config.Auth.Enabled
}

// IsTLSEnabled 检查TLS是否启用
func (m *Manager) IsTLSEnabled() bool {
	return m.config.EnableTLS
}

// IsCORSEnabled 检查CORS是否启用
func (m *Manager) IsCORSEnabled() bool {
	return m.config.EnableCORS
}

// GetSignalingServer 获取信令服务器实例
func (m *Manager) GetSignalingServer() *SignalingServer {
	return m.signalingServer
}

// SetSignalingMessageHandler 设置信令消息处理回调
// 这允许主程序处理业务逻辑，而 WebServer 只负责消息路由
func (m *Manager) SetSignalingMessageHandler(handler func(clientID string, messageType string, data map[string]interface{}) error) {
	if m.signalingServer != nil {
		m.signalingServer.SetBusinessMessageHandler(handler)
		m.logger.Debug("Signaling message handler configured for webserver manager")
	}
}
