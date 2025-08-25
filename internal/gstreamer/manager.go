package gstreamer

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"

	"github.com/open-beagle/bdwind-gstreamer/internal/config"
)

// ManagerConfig GStreamer管理器配置
type ManagerConfig struct {
	Config *config.GStreamerConfig
}

// Manager GStreamer管理器，统一管理所有GStreamer相关业务
type Manager struct {
	config *config.GStreamerConfig
	logger *logrus.Entry

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

	// 样本处理统计
	rawSampleCount     int64
	encodedSampleCount int64
	lastStatsLog       time.Time

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

	// 获取 logrus entry 用于不同级别的日志记录
	logger := config.GetLoggerWithPrefix("gstreamer")

	// Trace级别: 记录管理器创建开始
	logger.Trace("Starting GStreamer manager creation")

	// Use the provided context instead of creating a new one
	childCtx, cancel := context.WithCancel(ctx)

	manager := &Manager{
		config:             cfg.Config,
		logger:             logger,
		ctx:                childCtx,
		cancel:             cancel,
		rawSampleCount:     0,
		encodedSampleCount: 0,
		lastStatsLog:       time.Now(),
	}

	// Debug级别: 记录详细配置信息
	logger.Debug("Creating manager with configuration:")
	logger.Debugf("  Display ID: %s", cfg.Config.Capture.DisplayID)
	logger.Debugf("  Encoder Type: %s", cfg.Config.Encoding.Type)
	logger.Debugf("  Codec: %s", cfg.Config.Encoding.Codec)
	logger.Debugf("  Resolution: %dx%d", cfg.Config.Capture.Width, cfg.Config.Capture.Height)
	logger.Debugf("  Frame Rate: %d fps", cfg.Config.Capture.FrameRate)
	logger.Debugf("  Bitrate: %d kbps", cfg.Config.Encoding.Bitrate)
	logger.Debugf("  Use Wayland: %v", cfg.Config.Capture.UseWayland)
	logger.Debugf("  Hardware Acceleration: %v", cfg.Config.Encoding.UseHardware)

	// Trace级别: 记录组件初始化开始
	logger.Trace("Starting component initialization")

	// 初始化组件
	if err := manager.initializeComponents(); err != nil {
		// Error级别: 组件初始化失败
		logger.Errorf("Failed to initialize GStreamer components: %v", err)
		cancel()
		return nil, fmt.Errorf("failed to initialize components: %w", err)
	}

	// Trace级别: 记录组件初始化完成
	logger.Trace("Component initialization completed successfully")

	// Info级别: 管理器创建成功
	logger.Info("GStreamer manager created successfully")
	return manager, nil
}

// initializeComponents 初始化所有GStreamer组件
func (m *Manager) initializeComponents() error {
	// Trace级别: 记录组件初始化详细步骤
	m.logger.Trace("Step 1: Initializing desktop capture component")

	// 1. 创建桌面捕获
	var err error
	m.capture, err = NewDesktopCapture(m.config.Capture)
	if err != nil {
		// Error级别: 桌面捕获创建失败
		m.logger.Errorf("Failed to create desktop capture: %v", err)
		// 不返回错误，允许在没有桌面捕获的情况下运行
		m.capture = nil
		// Debug级别: 记录桌面捕获状态
		m.logger.Debug("Desktop capture status: unavailable (continuing without capture)")
	} else {
		// Debug级别: 记录桌面捕获详细状态
		m.logger.Debug("Desktop capture created successfully")
		captureMethod := "X11"
		if m.config.Capture.UseWayland {
			captureMethod = "Wayland"
		}
		m.logger.Debugf("  Capture method: %s", captureMethod)
		m.logger.Debugf("  Display: %s", m.config.Capture.DisplayID)
		m.logger.Debugf("  Capture region: %dx%d", m.config.Capture.Width, m.config.Capture.Height)
		m.logger.Debugf("  Show pointer: %v", m.config.Capture.ShowPointer)
		m.logger.Debugf("  Use damage events: %v", m.config.Capture.UseDamage)
		// Trace级别: 记录桌面捕获内部详细信息
		m.logger.Tracef("Desktop capture component initialized with buffer size: %d", m.config.Capture.BufferSize)
	}

	// Trace级别: 记录编码器初始化开始
	m.logger.Trace("Step 2: Initializing video encoder")

	// 2. 创建编码器
	encoderFactory := NewEncoderFactory()
	m.encoder, err = encoderFactory.CreateEncoder(m.config.Encoding)
	if err != nil {
		// Error级别: 编码器创建失败
		m.logger.Errorf("Failed to create encoder: %v", err)
		return err
	}

	// Debug级别: 记录编码器详细状态
	m.logger.Debug("Video encoder created successfully")
	m.logger.Debugf("  Encoder type: %s", m.config.Encoding.Type)
	m.logger.Debugf("  Codec: %s", m.config.Encoding.Codec)
	m.logger.Debugf("  Bitrate: %d kbps (min: %d, max: %d)",
		m.config.Encoding.Bitrate, m.config.Encoding.MinBitrate, m.config.Encoding.MaxBitrate)
	m.logger.Debugf("  Hardware acceleration: %v", m.config.Encoding.UseHardware)
	m.logger.Debugf("  Preset: %s", m.config.Encoding.Preset)
	m.logger.Debugf("  Profile: %s", m.config.Encoding.Profile)
	m.logger.Debugf("  Rate control: %s", m.config.Encoding.RateControl)
	m.logger.Debugf("  Zero latency: %v", m.config.Encoding.ZeroLatency)

	// Trace级别: 记录编码器回调设置
	m.logger.Trace("Setting up encoder sample callback")

	// Set the callback for encoded samples from the encoder
	if m.encoder != nil {
		m.encoder.SetSampleCallback(m.processEncodedSample)
		// Trace级别: 记录回调设置完成
		m.logger.Trace("Encoder sample callback configured successfully")
	}

	// Trace级别: 记录Pipeline状态管理初始化开始
	m.logger.Trace("Step 3: Initializing pipeline state management")

	// 3. 初始化Pipeline状态管理组件
	if err := m.initializePipelineStateManagement(); err != nil {
		// Error级别: Pipeline状态管理初始化失败
		m.logger.Errorf("Failed to initialize pipeline state management: %v", err)
		return err
	}

	// Debug级别: 记录Pipeline状态管理状态
	m.logger.Debug("Pipeline state management initialized")
	if m.errorHandler != nil {
		m.logger.Debug("  Error handler: enabled")
		m.logger.Debug("  Max retry attempts: 3")
		m.logger.Debug("  Auto recovery: enabled")
	}

	// Trace级别: 记录HTTP处理器初始化开始
	m.logger.Trace("Step 4: Initializing HTTP handlers")

	// 4. 创建HTTP处理器
	m.handlers = newGStreamerHandlers(m)
	// Debug级别: 记录HTTP处理器状态
	m.logger.Debug("HTTP handlers created successfully")
	// Trace级别: 记录HTTP处理器详细信息
	m.logger.Trace("HTTP handlers ready for route registration")

	// Trace级别: 记录所有组件初始化完成
	m.logger.Trace("All components initialized successfully")

	return nil
}

// initializePipelineStateManagement 初始化Pipeline状态管理组件
func (m *Manager) initializePipelineStateManagement() error {
	// 只有在有桌面捕获的情况下才初始化状态管理
	if m.capture == nil {
		m.logger.Debug("Skipping pipeline state management initialization - no desktop capture available")
		return nil
	}

	// 检查桌面捕获类型（需要先转换类型）
	if _, ok := m.capture.(*desktopCapture); !ok {
		m.logger.Warn("Desktop capture is not the expected type, skipping state management")
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
	m.logger.Debug("Pipeline error handler created successfully")

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

	m.logger.Debug("Pipeline state manager initialized and started successfully")
	return nil
}

// onPipelineStateChange 处理Pipeline状态变化回调
func (m *Manager) onPipelineStateChange(oldState, newState PipelineState, transition StateTransition) {
	m.logger.Debugf("Pipeline state changed: %v -> %v (took %v, success: %v)",
		oldState, newState, transition.Duration, transition.Success)

	if !transition.Success && transition.Error != nil {
		m.logger.Errorf("State transition failed: %v", transition.Error)

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
	m.logger.Errorf("Pipeline error in %s: %v", context, err)

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
		// Error级别: 管理器已经在运行
		m.logger.Error("Manager is already running")
		return fmt.Errorf("GStreamer manager already running")
	}

	// Info级别: 记录管理器启动开始
	m.logger.Info("Starting GStreamer manager...")
	// Trace级别: 记录启动时间
	m.startTime = time.Now()
	m.logger.Tracef("Manager start time: %v", m.startTime)

	// Debug级别: 记录启动前的组件状态检查
	m.logger.Debug("Pre-start component status check:")
	m.logger.Debugf("  Desktop capture available: %v", m.capture != nil)
	m.logger.Debugf("  Video encoder available: %v", m.encoder != nil)
	m.logger.Debugf("  Error handler available: %v", m.errorHandler != nil)
	m.logger.Debugf("  HTTP handlers available: %v", m.handlers != nil)

	// 启动桌面捕获（如果可用）
	if m.capture != nil {
		// Trace级别: 记录桌面捕获启动开始
		m.logger.Trace("Starting desktop capture component")

		if err := m.startDesktopCaptureWithContext(); err != nil {
			// Error级别: 桌面捕获启动失败（但不阻止管理器启动）
			m.logger.Errorf("Failed to start desktop capture: %v", err)
			// Debug级别: 记录桌面捕获详细状态
			m.logger.Debug("Desktop capture status: failed (continuing without capture)")
		} else {
			// Debug级别: 记录桌面捕获启动成功
			m.logger.Debug("Desktop capture started successfully")
			m.logger.Debug("  Sample callback configured: yes")
			m.logger.Debug("  Monitoring goroutine: started")

			// Trace级别: 记录Pipeline状态管理器初始化开始
			m.logger.Trace("Initializing pipeline state manager for capture")

			// 初始化Pipeline状态管理器（在桌面捕获启动后）
			if err := m.initializePipelineStateManagerForCapture(); err != nil {
				// Error级别: Pipeline状态管理器初始化失败（但不阻止管理器启动）
				m.logger.Errorf("Failed to initialize pipeline state manager: %v", err)
				// Debug级别: 记录状态管理器详细状态
				m.logger.Debug("Pipeline state manager status: failed (continuing without state management)")
			} else {
				// Debug级别: 记录状态管理器启动成功
				m.logger.Debug("Pipeline state manager initialized successfully")
				m.logger.Debug("  State change callbacks: configured")
				m.logger.Debug("  Error callbacks: configured")
				// Trace级别: 记录状态管理器详细信息
				m.logger.Trace("Pipeline state manager monitoring started")
			}
		}
	} else {
		// Debug级别: 记录桌面捕获不可用
		m.logger.Debug("Desktop capture not available, skipping capture startup")
	}

	// Trace级别: 记录管理器状态设置
	m.logger.Trace("Setting manager running state to true")
	m.running = true

	// Info级别: 记录管理器启动成功
	m.logger.Info("GStreamer manager started successfully")

	// Debug级别: 记录启动后的状态摘要
	uptime := time.Since(m.startTime)
	m.logger.Debugf("Manager startup completed in %v", uptime)
	m.logger.Debug("Final component status:")
	m.logger.Debugf("  Manager running: %v", m.running)
	m.logger.Debugf("  Desktop capture active: %v", m.capture != nil)
	m.logger.Debugf("  State manager active: %v", m.stateManager != nil)

	return nil
}

// startDesktopCaptureWithContext 启动桌面捕获并处理外部进程
func (m *Manager) startDesktopCaptureWithContext() error {
	// Info级别: 记录样本回调链路建立开始
	m.logger.Info("Starting desktop capture with sample callback chain")

	// 启动桌面捕获
	if err := m.capture.Start(); err != nil {
		// Error级别: 链路中断 - 桌面捕获启动失败
		m.logger.Errorf("Sample callback chain failed: desktop capture start error: %v", err)
		return fmt.Errorf("failed to start desktop capture: %w", err)
	}

	// Debug级别: 记录组件状态验证结果
	m.logger.Debug("Desktop capture started successfully")

	// 设置sample回调来处理视频帧数据
	if err := m.capture.SetSampleCallback(m.processSample); err != nil {
		// Error级别: 链路中断 - 样本回调设置失败
		m.logger.Errorf("Sample callback chain failed: callback setup error: %v", err)
		return fmt.Errorf("failed to set sample callback: %w", err)
	}

	// Info级别: 记录样本回调链路建立成功状态
	m.logger.Info("Sample callback chain established successfully")
	// Debug级别: 记录组件状态验证结果
	m.logger.Debug("Sample callback chain configuration:")
	m.logger.Debug("  Raw sample callback: configured")
	m.logger.Debug("  Target processor: GStreamer manager")
	m.logger.Debug("  Encoder integration: enabled")

	// 启动桌面捕获监控goroutine，监听context取消信号
	go m.monitorDesktopCapture()

	return nil
}

// processSample 处理从GStreamer pipeline接收到的视频帧数据
func (m *Manager) processSample(sample *Sample) error {
	if sample == nil {
		// Warn级别: 样本处理异常
		m.logger.Warn("Sample processing failed: received nil sample")
		return fmt.Errorf("received nil sample")
	}

	// 更新统计计数
	m.mutex.Lock()
	m.rawSampleCount++
	currentCount := m.rawSampleCount
	m.mutex.Unlock()

	// Trace级别: 记录每个样本的详细处理信息和类型识别
	m.logger.Tracef("Processing raw sample #%d: type=%s, size=%d bytes, format=%s, dimensions=%dx%d, timestamp=%v",
		currentCount, sample.Format.MediaType.String(), sample.Size(), sample.Format.Codec,
		sample.Format.Width, sample.Format.Height, sample.Timestamp)

	// Debug级别: 记录样本处理统计（每 1000 个样本）
	if currentCount%1000 == 0 {
		m.logger.Debugf("Raw sample processing milestone: %d samples processed", currentCount)
	}

	if m.encoder != nil {
		if err := m.encoder.PushFrame(sample); err != nil {
			// Warn级别: 样本处理异常
			m.logger.Warnf("Sample processing failed: error pushing frame to encoder: %v", err)
			return err
		}
		// Trace级别: 记录样本成功推送到编码器
		m.logger.Trace("Raw sample successfully pushed to encoder")
	} else {
		// Warn级别: 样本处理异常 - 编码器不可用
		m.logger.Warn("Sample processing warning: encoder not available")
	}

	return nil
}

// processEncodedSample is the callback for receiving encoded samples from the encoder
func (m *Manager) processEncodedSample(sample *Sample) error {
	if sample == nil {
		// Warn级别: 样本处理异常
		m.logger.Warn("Encoded sample processing failed: received nil sample")
		return fmt.Errorf("received nil sample")
	}

	// 更新统计计数
	m.mutex.Lock()
	m.encodedSampleCount++
	currentCount := m.encodedSampleCount
	callback := m.encodedSampleCallback
	m.mutex.Unlock()

	// Trace级别: 记录每个样本的详细处理信息和类型识别
	m.logger.Tracef("Processing encoded sample #%d: type=%s, size=%d bytes, codec=%s, timestamp=%v",
		currentCount, sample.Format.MediaType.String(), sample.Size(), sample.Format.Codec, sample.Timestamp)

	// Debug级别: 记录样本处理统计（每 1000 个样本）
	if currentCount%1000 == 0 {
		m.logger.Debugf("Encoded sample processing milestone: %d samples processed", currentCount)
	}

	if callback != nil {
		if err := callback(sample); err != nil {
			// Warn级别: 样本处理异常
			m.logger.Warnf("Encoded sample processing failed: error in callback: %v", err)
			return err
		}
		// Trace级别: 记录编码样本成功处理
		m.logger.Trace("Encoded sample successfully processed through callback")
	} else {
		// Warn级别: 样本处理异常 - 回调不可用
		m.logger.Warn("Encoded sample processing warning: callback not set")
	}

	return nil
}

// monitorDesktopCapture 监控桌面捕获进程，响应context取消
func (m *Manager) monitorDesktopCapture() {
	<-m.ctx.Done()
	m.logger.Debug("Desktop capture received shutdown signal, stopping...")

	// 停止桌面捕获
	if m.capture != nil {
		if err := m.capture.Stop(); err != nil {
			m.logger.Errorf("Error stopping desktop capture: %v", err)
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

	m.logger.Info("Stopping GStreamer manager...")

	// 停止Pipeline状态管理组件
	if m.stateManager != nil {
		if err := m.stateManager.Stop(); err != nil {
			m.logger.Warnf("Failed to stop pipeline state manager: %v", err)
		} else {
			m.logger.Debug("Pipeline state manager stopped successfully")
		}
	}

	// 停止桌面捕获
	if m.capture != nil {
		if err := m.capture.Stop(); err != nil {
			m.logger.Warnf("Failed to stop desktop capture: %v", err)
		} else {
			m.logger.Debug("Desktop capture stopped successfully")
		}
	}

	// 取消上下文
	m.cancel()

	m.logger.Info("GStreamer manager stopped successfully")
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

		// 添加样本处理统计
		stats["sample_processing"] = map[string]interface{}{
			"raw_samples_processed":     m.rawSampleCount,
			"encoded_samples_processed": m.encodedSampleCount,
			"last_stats_log":            m.lastStatsLog,
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

	m.logger.Debug("Setting up GStreamer routes...")

	// 设置GStreamer相关的所有路由
	if err := m.handlers.setupGStreamerRoutes(router); err != nil {
		return fmt.Errorf("failed to setup GStreamer routes: %w", err)
	}

	m.logger.Debug("GStreamer routes setup completed")
	return nil
}
