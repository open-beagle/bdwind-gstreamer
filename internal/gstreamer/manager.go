package gstreamer

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"sync"
	"time"

	"github.com/gorilla/mux"

	"github.com/open-beagle/bdwind-gstreamer/internal/config"
)

// ManagerConfig GStreamer管理器配置
type ManagerConfig struct {
	Config *config.DesktopCaptureConfig
	Logger *log.Logger
}

// Manager GStreamer管理器，统一管理所有GStreamer相关业务
type Manager struct {
	config *config.DesktopCaptureConfig
	logger *log.Logger

	// GStreamer组件
	capture DesktopCapture

	// HTTP处理器
	handlers *gstreamerHandlers

	// 外部进程管理
	xvfbCmd *exec.Cmd

	// 状态管理
	running   bool
	startTime time.Time
	mutex     sync.RWMutex

	// 上下文控制
	ctx    context.Context
	cancel context.CancelFunc
}

// NewManager 创建GStreamer管理器
func NewManager(ctx context.Context, cfg *ManagerConfig) (*Manager, error) {
	if ctx == nil {
		return nil, fmt.Errorf("context is required")
	}

	if cfg == nil {
		return nil, fmt.Errorf("manager config is required")
	}

	if cfg.Config == nil {
		return nil, fmt.Errorf("desktop capture config is required")
	}

	logger := cfg.Logger
	if logger == nil {
		logger = log.Default()
	}

	// Use the provided context instead of creating a new one
	childCtx, cancel := context.WithCancel(ctx)

	manager := &Manager{
		config: cfg.Config,
		logger: logger,
		ctx:    childCtx,
		cancel: cancel,
	}

	// 初始化组件
	if err := manager.initializeComponents(); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to initialize components: %w", err)
	}

	return manager, nil
}

// initializeComponents 初始化所有GStreamer组件
func (m *Manager) initializeComponents() error {
	// 1. 创建桌面捕获
	var err error
	m.capture, err = NewDesktopCapture(*m.config)
	if err != nil {
		m.logger.Printf("Failed to create desktop capture: %v", err)
		// 不返回错误，允许在没有桌面捕获的情况下运行
		m.capture = nil
	} else {
		m.logger.Printf("Desktop capture created successfully")
	}

	// 2. 创建HTTP处理器
	m.handlers = newGStreamerHandlers(m)
	m.logger.Printf("GStreamer handlers created successfully")

	return nil
}

// Start 启动GStreamer管理器
func (m *Manager) Start(ctx context.Context) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.running {
		return fmt.Errorf("GStreamer manager already running")
	}

	m.logger.Printf("Starting GStreamer manager...")
	m.startTime = time.Now()

	// 启动桌面捕获（如果可用）
	if m.capture != nil {
		if err := m.startDesktopCaptureWithContext(); err != nil {
			m.logger.Printf("Warning: failed to start desktop capture: %v", err)
			// 不返回错误，允许在捕获失败的情况下继续运行
		} else {
			m.logger.Printf("Desktop capture started successfully")
		}
	}

	m.running = true
	m.logger.Printf("GStreamer manager started successfully")

	return nil
}

// startDesktopCaptureWithContext 启动桌面捕获并处理外部进程
func (m *Manager) startDesktopCaptureWithContext() error {
	// 启动桌面捕获
	if err := m.capture.Start(); err != nil {
		return fmt.Errorf("failed to start desktop capture: %w", err)
	}

	// 设置sample回调来处理视频帧数据
	m.capture.SetSampleCallback(m.processSample)

	// 启动桌面捕获监控goroutine，监听context取消信号
	go m.monitorDesktopCapture()

	m.logger.Printf("Desktop capture started with sample callback configured")
	return nil
}

// processSample 处理从GStreamer pipeline接收到的视频帧数据
func (m *Manager) processSample(sample *Sample) error {
	if sample == nil {
		return fmt.Errorf("received nil sample")
	}

	m.logger.Printf("Received video sample: size=%d bytes, format=%s, dimensions=%dx%d, timestamp=%v",
		sample.Size(), sample.Format.Codec, sample.Format.Width, sample.Format.Height, sample.Timestamp)

	// TODO: 这里应该将sample传递给WebRTC bridge
	// 目前只是记录日志来验证数据流是否正常工作

	return nil
}

// monitorDesktopCapture 监控桌面捕获进程，响应context取消
func (m *Manager) monitorDesktopCapture() {
	<-m.ctx.Done()
	m.logger.Printf("Desktop capture received shutdown signal, stopping...")

	// 停止桌面捕获
	if m.capture != nil {
		if err := m.capture.Stop(); err != nil {
			m.logger.Printf("Error stopping desktop capture: %v", err)
		}
	}
}

// Stop 停止GStreamer管理器和所有组件
func (m *Manager) Stop(ctx context.Context) error {
	m.mutex.Lock()
	if !m.running {
		m.mutex.Unlock()
		return nil
	}
	m.running = false
	m.mutex.Unlock()

	m.logger.Printf("Stopping GStreamer manager...")

	// 停止桌面捕获
	if m.capture != nil {
		if err := m.capture.Stop(); err != nil {
			m.logger.Printf("Warning: failed to stop desktop capture: %v", err)
		} else {
			m.logger.Printf("Desktop capture stopped successfully")
		}
	}

	// 取消上下文
	m.cancel()

	m.logger.Printf("GStreamer manager stopped successfully")
	return nil
}

// IsRunning 检查管理器是否运行中
func (m *Manager) IsRunning() bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.running
}

// IsEnabled 检查组件是否启用
// GStreamer是核心组件，始终启用
func (m *Manager) IsEnabled() bool {
	return true
}

// GetCapture 获取桌面捕获实例
func (m *Manager) GetCapture() DesktopCapture {
	return m.capture
}

// GetStats 获取GStreamer管理器统计信息
func (m *Manager) GetStats() map[string]interface{} {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	stats := map[string]interface{}{
		"running":    m.running,
		"start_time": m.startTime,
		"uptime":     time.Since(m.startTime).Seconds(),
	}

	if m.running {
		// 添加桌面捕获统计
		stats["capture"] = map[string]interface{}{
			"initialized": m.capture != nil,
			"available":   m.capture != nil,
		}

		// TODO: 添加更多统计信息，如帧率、编码统计等
		if m.capture != nil {
			stats["capture_stats"] = map[string]interface{}{
				"frames_captured": 0, // 需要从实际捕获实例获取
				"frames_dropped":  0,
				"current_fps":     0.0,
				"average_fps":     0.0,
			}
		}
	}

	return stats
}

// GetContext 获取组件的上下文
func (m *Manager) GetContext() context.Context {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.ctx
}

// GetConfig 获取配置信息
func (m *Manager) GetConfig() *config.DesktopCaptureConfig {
	return m.config
}

// GetUptime 获取运行时间
func (m *Manager) GetUptime() time.Duration {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	if !m.running {
		return 0
	}

	return time.Since(m.startTime)
}

// GetComponentStatus 获取各组件状态
func (m *Manager) GetComponentStatus() map[string]bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	return map[string]bool{
		"manager":  m.running,
		"capture":  m.capture != nil,
		"handlers": m.handlers != nil,
	}
}

// SetupRoutes 设置GStreamer组件的HTTP路由
// 实现webserver.RouteSetup接口
func (m *Manager) SetupRoutes(router *mux.Router) error {
	if m.handlers == nil {
		return fmt.Errorf("GStreamer handlers not initialized")
	}

	m.logger.Printf("Setting up GStreamer routes...")

	// 设置GStreamer相关的所有路由
	if err := m.handlers.setupGStreamerRoutes(router); err != nil {
		return fmt.Errorf("failed to setup GStreamer routes: %w", err)
	}

	m.logger.Printf("GStreamer routes setup completed")
	return nil
}
