package webserver

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/open-beagle/bdwind-gstreamer/internal/config"
)

// Manager webserver组件管理器
// 实现 ComponentManager 接口，管理web服务器功能
type Manager struct {
	config    *config.WebServerConfig
	server    *http.Server
	webServer *WebServer
	logger    *log.Logger
	running   bool
	startTime time.Time
	mutex     sync.RWMutex
	ctx       context.Context
	cancel    context.CancelFunc
}

// NewManager 创建新的webserver管理器
func NewManager(ctx context.Context, cfg *config.WebServerConfig, logger *log.Logger) (*Manager, error) {
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

	// 创建日志器
	if logger == nil {
		logger = log.New(log.Writer(), "[WEBSERVER] ", log.LstdFlags)
	}

	// 创建webserver实例
	webServer, err := NewWebServer(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create webserver: %w", err)
	}

	// 创建子context
	childCtx, cancel := context.WithCancel(ctx)

	return &Manager{
		config:    cfg,
		webServer: webServer,
		logger:    logger,
		ctx:       childCtx,
		cancel:    cancel,
	}, nil
}

// Start 启动webserver管理器
func (m *Manager) Start(ctx context.Context) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.running {
		return fmt.Errorf("webserver manager already running")
	}

	m.logger.Println("Starting webserver manager...")

	// 创建HTTP服务器
	addr := fmt.Sprintf("%s:%d", m.config.Host, m.config.Port)

	m.logger.Println("Getting webserver handler...")
	handler := m.webServer.GetHandler()
	m.logger.Println("Webserver handler obtained successfully")

	m.server = &http.Server{
		Addr:    addr,
		Handler: handler,
	}

	// 在goroutine中启动服务器
	go func() {
		m.logger.Printf("Starting webserver on %s", addr)

		var err error
		if m.config.EnableTLS {
			err = m.server.ListenAndServeTLS(m.config.TLS.CertFile, m.config.TLS.KeyFile)
		} else {
			err = m.server.ListenAndServe()
		}

		if err != nil && err != http.ErrServerClosed {
			m.logger.Printf("Webserver error: %v", err)
		}
	}()

	// 等待服务器启动
	time.Sleep(100 * time.Millisecond)

	m.running = true
	m.startTime = time.Now()

	protocol := "http"
	if m.config.EnableTLS {
		protocol = "https"
	}

	m.logger.Printf("Webserver started successfully on %s://%s", protocol, addr)
	return nil
}

// Stop 停止webserver管理器
func (m *Manager) Stop(ctx context.Context) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if !m.running {
		return nil
	}

	m.logger.Println("Stopping webserver manager...")

	// 取消context以通知所有子组件停止
	if m.cancel != nil {
		m.cancel()
	}

	// 优雅关闭HTTP服务器
	if m.server != nil {
		m.logger.Println("Shutting down HTTP server...")
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
				m.logger.Printf("Error during server shutdown: %v", err)
				// 不返回错误，继续清理其他资源
			} else {
				m.logger.Println("HTTP server shutdown successfully")
			}
		case <-time.After(6 * time.Second):
			m.logger.Printf("HTTP server shutdown timeout, forcing close")
			// 强制关闭服务器
			if err := m.server.Close(); err != nil {
				m.logger.Printf("Error during server force close: %v", err)
			}
		}
	}

	m.running = false
	m.logger.Println("Webserver manager stopped successfully")
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
	m.logger.Println("Webserver config updated successfully")
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
