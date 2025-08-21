package gstreamer

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"

	"github.com/open-beagle/bdwind-gstreamer/internal/config"
)

// ManagerConfig GStreamer管理器配置
type ManagerConfig struct {
	Config *config.GStreamerConfig
	Logger *log.Logger
}

// Manager GStreamer管理器，统一管理所有GStreamer相关业务
type Manager struct {
	config *config.GStreamerConfig
	logger *log.Logger

	// GStreamer组件
	capture DesktopCapture
	encoder Encoder

	// HTTP处理器
	handlers *gstreamerHandlers

	// 外部进程管理
	xvfbCmd *exec.Cmd

	encodedSampleCallback func(*Sample) error

	// 状态管理
	running   bool
	startTime time.Time
	mutex     sync.RWMutex

	// Pipeline状态管理
	stateManager  *PipelineStateManager
	healthChecker *PipelineHealthChecker
	errorHandler  *PipelineErrorHandler

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
		return nil, fmt.Errorf("gstreamer config is required")
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
	m.capture, err = NewDesktopCapture(m.config.Capture)
	if err != nil {
		m.logger.Printf("Failed to create desktop capture: %v", err)
		// 不返回错误，允许在没有桌面捕获的情况下运行
		m.capture = nil
	} else {
		m.logger.Printf("Desktop capture created successfully")
	}

	// 2. 创建编码器
	encoderFactory := NewEncoderFactory()
	m.encoder, err = encoderFactory.CreateEncoder(m.config.Encoding)
	if err != nil {
		m.logger.Printf("Failed to create encoder: %v", err)
		// Depending on requirements, you might want to handle this more gracefully
		return err
	}
	m.logger.Printf("Encoder created successfully")

	// Set the callback for encoded samples from the encoder
	if m.encoder != nil {
		m.encoder.SetSampleCallback(m.processEncodedSample)
	}

	// 3. 初始化Pipeline状态管理组件
	if err := m.initializePipelineStateManagement(); err != nil {
		m.logger.Printf("Failed to initialize pipeline state management: %v", err)
		return err
	}

	// 4. 创建HTTP处理器
	m.handlers = newGStreamerHandlers(m)
	m.logger.Printf("GStreamer handlers created successfully")

	return nil
}

// initializePipelineStateManagement 初始化Pipeline状态管理组件
func (m *Manager) initializePipelineStateManagement() error {
	// 只有在有桌面捕获的情况下才初始化状态管理
	if m.capture == nil {
		m.logger.Printf("Skipping pipeline state management initialization - no desktop capture available")
		return nil
	}

	// 检查桌面捕获类型（需要先转换类型）
	if _, ok := m.capture.(*desktopCapture); !ok {
		m.logger.Printf("Warning: desktop capture is not the expected type, skipping state management")
		return nil
	}

	// 等待pipeline创建（在Start方法中创建）
	// 这里我们只初始化错误处理器，状态管理器将在Start方法中初始化

	// 1. 创建错误处理器
	errorConfig := &ErrorHandlerConfig{
		MaxRetryAttempts:    3,
		RetryDelay:          2 * time.Second,
		AutoRecovery:        true,
		MaxErrorHistorySize: 100,
	}
	m.errorHandler = NewPipelineErrorHandler(m.logger, errorConfig)
	m.logger.Printf("Pipeline error handler created successfully")

	return nil
}

// initializePipelineStateManagerForCapture 为桌面捕获初始化状态管理器
func (m *Manager) initializePipelineStateManagerForCapture() error {
	if m.capture == nil {
		return nil
	}

	// 获取桌面捕获的pipeline
	dc, ok := m.capture.(*desktopCapture)
	if !ok || dc.pipeline == nil {
		return fmt.Errorf("desktop capture pipeline not available")
	}

	// 创建状态管理器配置
	stateConfig := DefaultStateManagerConfig()

	// 创建状态管理器
	m.stateManager = NewPipelineStateManager(m.ctx, dc.pipeline, m.logger, stateConfig)

	// 设置错误处理器的状态管理器引用
	if m.errorHandler != nil {
		m.errorHandler.config.StateManager = m.stateManager
	}

	// 设置状态变化回调
	m.stateManager.SetStateChangeCallback(m.onPipelineStateChange)
	m.stateManager.SetErrorCallback(m.onPipelineError)

	// 启动状态管理器
	if err := m.stateManager.Start(); err != nil {
		return fmt.Errorf("failed to start pipeline state manager: %w", err)
	}

	m.logger.Printf("Pipeline state manager initialized and started successfully")
	return nil
}

// onPipelineStateChange 处理Pipeline状态变化回调
func (m *Manager) onPipelineStateChange(oldState, newState PipelineState, transition StateTransition) {
	m.logger.Printf("Pipeline state changed: %v -> %v (took %v, success: %v)",
		oldState, newState, transition.Duration, transition.Success)

	if !transition.Success && transition.Error != nil {
		m.logger.Printf("State transition failed: %v", transition.Error)

		// 通过错误处理器处理状态转换错误
		if m.errorHandler != nil {
			m.errorHandler.HandleError(ErrorStateChange,
				fmt.Sprintf("State transition failed: %v -> %v", oldState, newState),
				transition.Error.Error(),
				"PipelineStateManager")
		}
	}
}

// onPipelineError 处理Pipeline错误回调
func (m *Manager) onPipelineError(err error, context string) {
	m.logger.Printf("Pipeline error in %s: %v", context, err)

	// 通过错误处理器处理错误
	if m.errorHandler != nil {
		// 根据错误内容确定错误类型
		errorType := m.classifyError(err)
		m.errorHandler.HandleError(errorType, err.Error(), context, "Pipeline")
	}
}

// classifyError 根据错误内容分类错误类型
func (m *Manager) classifyError(err error) ErrorType {
	errStr := err.Error()

	// 简单的错误分类逻辑
	switch {
	case strings.Contains(errStr, "state"):
		return ErrorStateChange
	case strings.Contains(errStr, "element"):
		return ErrorElementFailure
	case strings.Contains(errStr, "resource"):
		return ErrorResourceUnavailable
	case strings.Contains(errStr, "format"):
		return ErrorFormatNegotiation
	case strings.Contains(errStr, "timeout"):
		return ErrorTimeout
	case strings.Contains(errStr, "permission"):
		return ErrorPermission
	case strings.Contains(errStr, "memory"):
		return ErrorMemoryExhaustion
	case strings.Contains(errStr, "network"):
		return ErrorNetwork
	case strings.Contains(errStr, "hardware"):
		return ErrorHardware
	default:
		return ErrorUnknown
	}
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

			// 初始化Pipeline状态管理器（在桌面捕获启动后）
			if err := m.initializePipelineStateManagerForCapture(); err != nil {
				m.logger.Printf("Warning: failed to initialize pipeline state manager: %v", err)
				// 不返回错误，允许在状态管理失败的情况下继续运行
			}
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

	if m.encoder != nil {
		if err := m.encoder.PushFrame(sample); err != nil {
			m.logger.Printf("Error pushing frame to encoder: %v", err)
			return err
		}
	}

	return nil
}

// processEncodedSample is the callback for receiving encoded samples from the encoder
func (m *Manager) processEncodedSample(sample *Sample) error {
	m.mutex.RLock()
	callback := m.encodedSampleCallback
	m.mutex.RUnlock()

	if callback != nil {
		if err := callback(sample); err != nil {
			m.logger.Printf("Error in encoded sample callback: %v", err)
			return err
		}
	}
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

	// 停止Pipeline状态管理组件
	if m.stateManager != nil {
		if err := m.stateManager.Stop(); err != nil {
			m.logger.Printf("Warning: failed to stop pipeline state manager: %v", err)
		} else {
			m.logger.Printf("Pipeline state manager stopped successfully")
		}
	}

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

// SetEncodedSampleCallback sets the callback function for processing encoded video samples
func (m *Manager) SetEncodedSampleCallback(callback func(*Sample) error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.encodedSampleCallback = callback
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

		// 添加桌面捕获详细统计
		if m.capture != nil {
			captureStats := m.capture.GetStats()
			stats["capture_stats"] = map[string]interface{}{
				"frames_captured": captureStats.FramesCapture,
				"frames_dropped":  captureStats.FramesDropped,
				"current_fps":     captureStats.CurrentFPS,
				"average_fps":     captureStats.AverageFPS,
				"capture_latency": captureStats.CaptureLatency,
				"min_latency":     captureStats.MinLatency,
				"max_latency":     captureStats.MaxLatency,
				"avg_latency":     captureStats.AvgLatency,
				"last_frame_time": captureStats.LastFrameTime,
			}
		}

		// 添加Pipeline状态管理统计
		if m.stateManager != nil {
			stats["pipeline_state"] = m.stateManager.GetStats()
		}

		// 添加健康检查统计
		if m.healthChecker != nil {
			healthStatus := m.healthChecker.GetHealthStatus()
			stats["health_status"] = map[string]interface{}{
				"overall":      healthStatus.Overall.String(),
				"last_check":   healthStatus.LastCheck,
				"uptime":       healthStatus.Uptime.Seconds(),
				"issues_count": len(healthStatus.Issues),
				"checks_count": len(healthStatus.Checks),
			}
		}

		// 添加错误处理统计
		if m.errorHandler != nil {
			stats["error_handling"] = m.errorHandler.GetStats()
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
func (m *Manager) GetConfig() *config.GStreamerConfig {
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

// GetPipelineState 获取当前Pipeline状态
func (m *Manager) GetPipelineState() (PipelineState, error) {
	if m.stateManager == nil {
		return StateNull, fmt.Errorf("pipeline state manager not initialized")
	}
	return m.stateManager.GetCurrentState(), nil
}

// SetPipelineState 设置Pipeline状态
func (m *Manager) SetPipelineState(state PipelineState) error {
	if m.stateManager == nil {
		return fmt.Errorf("pipeline state manager not initialized")
	}
	return m.stateManager.SetState(state)
}

// GetPipelineHealthStatus 获取Pipeline健康状态
func (m *Manager) GetPipelineHealthStatus() (HealthStatus, error) {
	if m.healthChecker == nil {
		return HealthStatus{}, fmt.Errorf("pipeline health checker not initialized")
	}
	return m.healthChecker.GetHealthStatus(), nil
}

// GetPipelineErrorHistory 获取Pipeline错误历史
func (m *Manager) GetPipelineErrorHistory() []ErrorEvent {
	if m.errorHandler == nil {
		return nil
	}
	return m.errorHandler.GetErrorHistory()
}

// RestartPipeline 重启Pipeline
func (m *Manager) RestartPipeline() error {
	if m.stateManager == nil {
		return fmt.Errorf("pipeline state manager not initialized")
	}
	return m.stateManager.RestartPipeline()
}

// GetPipelineStateHistory 获取Pipeline状态变化历史
func (m *Manager) GetPipelineStateHistory() []StateTransition {
	if m.stateManager == nil {
		return nil
	}
	return m.stateManager.GetStateHistory()
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
