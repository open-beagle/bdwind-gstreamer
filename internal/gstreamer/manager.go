package gstreamer

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

	// 媒体流订阅者
	subscribers []MediaStreamSubscriber

	// 日志配置管理
	logConfigurator  GStreamerLogConfigurator
	currentLogConfig *GStreamerLogConfig

	// 配置变化监控
	configChangeChannel chan *config.LoggingConfig

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
		config:              cfg.Config,
		logger:              logger,
		ctx:                 childCtx,
		cancel:              cancel,
		rawSampleCount:      0,
		encodedSampleCount:  0,
		lastStatsLog:        time.Now(),
		configChangeChannel: make(chan *config.LoggingConfig, 10), // 缓冲通道避免阻塞
	}

	// 初始化日志配置器
	manager.logConfigurator = NewGStreamerLogConfigurator(logger)
	logger.Debug("GStreamer log configurator initialized")

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

	// Debug级别: 管理器创建成功
	logger.Debug("GStreamer manager created successfully")
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

	// Debug级别: 记录管理器启动开始
	m.logger.Debug("Starting GStreamer manager...")
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

	// 启动配置监控
	m.MonitorLoggingConfigChanges(m.configChangeChannel)

	// Debug级别: 记录管理器启动成功 - 内部状态
	uptime := time.Since(m.startTime)
	m.logger.Debugf("GStreamer manager started successfully (startup time: %v)", uptime)

	// Debug级别: 记录启动后的状态摘要
	m.logger.Debug("Final component status:")
	m.logger.Debugf("  Manager running: %v", m.running)
	m.logger.Debugf("  Desktop capture active: %v", m.capture != nil)
	m.logger.Debugf("  State manager active: %v", m.stateManager != nil)
	m.logger.Debugf("  Config monitoring active: %v", m.configChangeChannel != nil)

	return nil
}

// startDesktopCaptureWithContext 启动桌面捕获并处理外部进程
func (m *Manager) startDesktopCaptureWithContext() error {
	// Debug级别: 记录样本回调链路建立开始
	m.logger.Debug("Starting desktop capture with sample callback chain")

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

	// Debug级别: 记录样本回调链路建立成功状态
	m.logger.Debug("Sample callback chain established successfully")
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

	// 转换为 WebRTC 兼容的流格式并发布给订阅者
	if err := m.publishEncodedStreamToSubscribers(sample); err != nil {
		m.logger.Warnf("Failed to publish encoded stream to subscribers: %v", err)
	}

	// 保持原有的回调机制用于向后兼容
	if callback != nil {
		if err := callback(sample); err != nil {
			// Warn级别: 样本处理异常
			m.logger.Warnf("Encoded sample processing failed: error in callback: %v", err)
			return err
		}
		// Trace级别: 记录编码样本成功处理
		m.logger.Trace("Encoded sample successfully processed through callback")
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

	// 关闭配置变化通道
	if m.configChangeChannel != nil {
		close(m.configChangeChannel)
		m.configChangeChannel = nil
		m.logger.Debug("Configuration change channel closed")
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

// publishEncodedStreamToSubscribers 将编码后的样本转换为 WebRTC 兼容格式并发布给订阅者
func (m *Manager) publishEncodedStreamToSubscribers(sample *Sample) error {
	if sample == nil {
		return fmt.Errorf("sample is nil")
	}

	// 根据样本类型发布不同的流
	switch sample.Format.MediaType {
	case MediaTypeVideo:
		return m.publishVideoStreamToSubscribers(sample)
	case MediaTypeAudio:
		return m.publishAudioStreamToSubscribers(sample)
	default:
		m.logger.Debugf("Unknown media type: %v, skipping publication", sample.Format.MediaType)
		return nil
	}
}

// publishVideoStreamToSubscribers 发布视频流给订阅者
func (m *Manager) publishVideoStreamToSubscribers(sample *Sample) error {
	// 转换为 WebRTC 兼容的视频流格式
	videoStream := &EncodedVideoStream{
		Codec:     sample.Format.Codec,
		Data:      sample.Data,
		Timestamp: sample.Timestamp.UnixMilli(),
		KeyFrame:  sample.IsKeyFrame(),
		Width:     sample.Format.Width,
		Height:    sample.Format.Height,
		Bitrate:   m.config.Encoding.Bitrate,
	}

	return m.PublishVideoStream(videoStream)
}

// publishAudioStreamToSubscribers 发布音频流给订阅者
func (m *Manager) publishAudioStreamToSubscribers(sample *Sample) error {
	// 转换为 WebRTC 兼容的音频流格式
	audioStream := &EncodedAudioStream{
		Codec:      sample.Format.Codec,
		Data:       sample.Data,
		Timestamp:  sample.Timestamp.UnixMilli(),
		SampleRate: sample.Format.SampleRate,
		Channels:   sample.Format.Channels,
		Bitrate:    m.config.Encoding.Bitrate, // 使用配置的比特率
	}

	return m.PublishAudioStream(audioStream)
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

		// 添加日志配置统计
		if m.currentLogConfig != nil {
			stats["logging_config"] = map[string]interface{}{
				"enabled":     m.currentLogConfig.Enabled,
				"level":       m.currentLogConfig.Level,
				"output_file": m.currentLogConfig.OutputFile,
				"colored":     m.currentLogConfig.Colored,
				"categories":  len(m.currentLogConfig.Categories),
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
		"manager":          m.running,
		"capture":          m.capture != nil,
		"handlers":         m.handlers != nil,
		"log_configurator": m.logConfigurator != nil,
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

// ConfigureLogging 配置GStreamer日志
func (m *Manager) ConfigureLogging(appConfig *config.LoggingConfig) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.logger.Debug("Starting GStreamer logging configuration")

	if m.logConfigurator == nil {
		m.logger.Error("Cannot configure GStreamer logging: log configurator not initialized")
		return fmt.Errorf("log configurator not initialized")
	}

	if appConfig == nil {
		m.logger.Error("Cannot configure GStreamer logging: application config is nil")
		return fmt.Errorf("application config is nil")
	}

	// 创建配置副本并规范化路径
	normalizedConfig := *appConfig
	if normalizedConfig.File != "" {
		normalizedConfig.File = config.NormalizeLogFilePath(normalizedConfig.File)
		m.logger.Debugf("Normalized log file path: %s -> %s", appConfig.File, normalizedConfig.File)
	}

	m.logger.Debugf("Configuring GStreamer logging with app config: level=%s, output=%s, file=%s",
		normalizedConfig.Level, normalizedConfig.Output, normalizedConfig.File)

	// 验证应用程序配置
	if err := m.validateAppLoggingConfig(&normalizedConfig); err != nil {
		m.logger.Warnf("Application logging configuration validation failed: %v", err)
		// 继续处理，但记录警告
	}

	// 使用日志配置器配置GStreamer日志
	if err := m.logConfigurator.ConfigureFromAppLogging(&normalizedConfig); err != nil {
		m.logger.Errorf("Failed to configure GStreamer logging: %v", err)

		// 尝试应用安全的默认配置
		m.logger.Info("Attempting to apply safe default GStreamer logging configuration")
		if fallbackErr := m.applyFallbackLoggingConfig(); fallbackErr != nil {
			m.logger.Errorf("Failed to apply fallback logging configuration: %v", fallbackErr)
			// 不返回错误，允许继续运行但记录严重警告
			m.logger.Warn("GStreamer logging configuration failed completely, continuing without GStreamer logging")
			return nil
		}

		m.logger.Info("Successfully applied fallback GStreamer logging configuration")
		return nil
	}

	// 更新当前配置缓存
	m.currentLogConfig = m.logConfigurator.GetCurrentConfig()

	m.logger.Tracef("GStreamer logging configured successfully: enabled=%t, level=%d (%s), output=%s",
		m.currentLogConfig.Enabled, m.currentLogConfig.Level,
		m.getLogLevelDescription(m.currentLogConfig.Level),
		m.getOutputDescription(m.currentLogConfig.OutputFile))

	return nil
}

// applyFallbackLoggingConfig 应用安全的默认日志配置
func (m *Manager) applyFallbackLoggingConfig() error {
	fallbackConfig := &GStreamerLogConfig{
		Enabled:    false, // 安全默认：禁用日志
		Level:      0,
		OutputFile: "", // 控制台输出
		Categories: make(map[string]int),
		Colored:    true,
	}

	m.logger.Debug("Applying fallback GStreamer logging configuration")

	if err := m.logConfigurator.UpdateConfig(fallbackConfig); err != nil {
		return fmt.Errorf("failed to apply fallback config: %w", err)
	}

	m.currentLogConfig = fallbackConfig
	m.logger.Debug("Successfully applied fallback GStreamer logging configuration")
	return nil
}

// getLogLevelDescription 获取日志级别描述（管理器版本）
func (m *Manager) getLogLevelDescription(level int) string {
	switch level {
	case 0:
		return "NONE"
	case 1:
		return "ERROR"
	case 2:
		return "WARNING"
	case 3:
		return "FIXME"
	case 4:
		return "INFO"
	case 5:
		return "DEBUG"
	case 6:
		return "LOG"
	case 7:
		return "TRACE"
	case 8:
		return "MEMDUMP"
	case 9:
		return "COUNT"
	default:
		return fmt.Sprintf("UNKNOWN(%d)", level)
	}
}

// getOutputDescription 获取输出描述（管理器版本）
func (m *Manager) getOutputDescription(outputFile string) string {
	if outputFile == "" {
		return "console"
	}
	return fmt.Sprintf("file(%s)", outputFile)
}

// UpdateLoggingConfig 动态更新GStreamer日志配置
func (m *Manager) UpdateLoggingConfig(appConfig *config.LoggingConfig) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.logger.Debug("Starting dynamic update of GStreamer logging configuration")

	if m.logConfigurator == nil {
		m.logger.Error("Cannot update GStreamer logging: log configurator not initialized")
		return fmt.Errorf("log configurator not initialized")
	}

	if appConfig == nil {
		m.logger.Error("Cannot update GStreamer logging: application config is nil")
		return fmt.Errorf("application config is nil")
	}

	// 创建配置副本并规范化路径
	normalizedConfig := *appConfig
	if normalizedConfig.File != "" {
		normalizedConfig.File = config.NormalizeLogFilePath(normalizedConfig.File)
		m.logger.Debugf("Normalized log file path: %s -> %s", appConfig.File, normalizedConfig.File)
	}

	m.logger.Debugf("Updating GStreamer logging with new app config: level=%s, output=%s, file=%s",
		normalizedConfig.Level, normalizedConfig.Output, normalizedConfig.File)

	// 保存当前配置以便回滚
	oldConfig := m.currentLogConfig

	// 验证应用程序日志配置
	if err := m.validateAppLoggingConfig(&normalizedConfig); err != nil {
		m.logger.Warnf("Application logging configuration validation failed: %v", err)
		// 对于更新操作，我们更严格一些，但仍然尝试继续
		m.logger.Info("Continuing with update despite validation warnings")
	}

	// 检查是否真的需要更新
	if oldConfig != nil {
		needsUpdate, changes := m.detectConfigChanges(&normalizedConfig, oldConfig)
		if !needsUpdate {
			m.logger.Debug("No changes detected in logging configuration, skipping update")
			return nil
		}
		m.logger.Debugf("Detected configuration changes: %v", changes)
	}

	// 使用日志配置器更新配置
	if err := m.logConfigurator.ConfigureFromAppLogging(&normalizedConfig); err != nil {
		m.logger.Errorf("Failed to update GStreamer logging configuration: %v", err)

		// 尝试恢复到之前的配置
		if oldConfig != nil {
			m.logger.Info("Attempting to restore previous GStreamer logging configuration")
			if restoreErr := m.logConfigurator.UpdateConfig(oldConfig); restoreErr != nil {
				m.logger.Errorf("Failed to restore previous configuration: %v", restoreErr)
				// 尝试应用安全默认配置
				if fallbackErr := m.applyFallbackLoggingConfig(); fallbackErr != nil {
					m.logger.Errorf("Failed to apply fallback configuration: %v", fallbackErr)
				}
			} else {
				m.logger.Info("Successfully restored previous GStreamer logging configuration")
			}
		}

		// 不返回错误，保持现有配置
		m.logger.Warn("GStreamer logging configuration update failed, maintaining current configuration")
		return nil
	}

	// 更新当前配置缓存
	m.currentLogConfig = m.logConfigurator.GetCurrentConfig()

	// 记录配置变化
	if oldConfig != nil {
		m.logger.Infof("GStreamer logging configuration updated successfully:")
		m.logger.Infof("  Enabled: %t -> %t", oldConfig.Enabled, m.currentLogConfig.Enabled)
		m.logger.Infof("  Level: %d (%s) -> %d (%s)",
			oldConfig.Level, m.getLogLevelDescription(oldConfig.Level),
			m.currentLogConfig.Level, m.getLogLevelDescription(m.currentLogConfig.Level))
		m.logger.Infof("  Output: %s -> %s",
			m.getOutputDescription(oldConfig.OutputFile),
			m.getOutputDescription(m.currentLogConfig.OutputFile))
		m.logger.Infof("  Colored: %t -> %t", oldConfig.Colored, m.currentLogConfig.Colored)
	} else {
		m.logger.Infof("GStreamer logging configuration updated: enabled=%t, level=%d (%s), output=%s, colored=%t",
			m.currentLogConfig.Enabled, m.currentLogConfig.Level,
			m.getLogLevelDescription(m.currentLogConfig.Level),
			m.getOutputDescription(m.currentLogConfig.OutputFile),
			m.currentLogConfig.Colored)
	}

	return nil
}

// ValidateLoggingConfig 验证GStreamer日志配置
func (m *Manager) ValidateLoggingConfig(gstConfig *GStreamerLogConfig) error {
	if gstConfig == nil {
		return fmt.Errorf("GStreamer log config is nil")
	}

	// 验证日志级别
	if gstConfig.Level < 0 || gstConfig.Level > 9 {
		return fmt.Errorf("invalid GStreamer log level: %d, must be between 0 and 9", gstConfig.Level)
	}

	// 验证日志文件路径
	if gstConfig.OutputFile != "" {
		if err := m.validateLogFilePath(gstConfig.OutputFile); err != nil {
			return fmt.Errorf("invalid log file path: %w", err)
		}
	}

	// 验证类别配置
	for category, level := range gstConfig.Categories {
		if level < 0 || level > 9 {
			return fmt.Errorf("invalid log level %d for category %s, must be between 0 and 9", level, category)
		}
		if category == "" {
			return fmt.Errorf("empty category name not allowed")
		}
	}

	return nil
}

// SyncLoggingWithApp 同步GStreamer日志配置与应用程序日志配置
func (m *Manager) SyncLoggingWithApp(appConfig *config.LoggingConfig) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.logConfigurator == nil {
		return fmt.Errorf("log configurator not initialized")
	}

	m.logger.Debug("Synchronizing GStreamer logging with application configuration")

	// 验证应用程序配置
	if err := m.validateAppLoggingConfig(appConfig); err != nil {
		return fmt.Errorf("invalid application logging configuration: %w", err)
	}

	// 获取当前GStreamer配置
	currentConfig := m.logConfigurator.GetCurrentConfig()

	// 检查是否需要更新
	needsUpdate, changes := m.detectConfigChanges(appConfig, currentConfig)
	if !needsUpdate {
		m.logger.Debug("No changes detected in logging configuration, skipping update")
		return nil
	}

	m.logger.Debugf("Detected logging configuration changes: %v", changes)

	// 应用更新
	if err := m.logConfigurator.ConfigureFromAppLogging(appConfig); err != nil {
		m.logger.Errorf("Failed to sync GStreamer logging configuration: %v", err)
		return fmt.Errorf("failed to sync logging configuration: %w", err)
	}

	// 更新缓存
	m.currentLogConfig = m.logConfigurator.GetCurrentConfig()

	m.logger.Infof("Successfully synchronized GStreamer logging configuration with application settings")
	return nil
}

// MonitorLoggingConfigChanges 监控应用程序日志配置变化
func (m *Manager) MonitorLoggingConfigChanges(configChannel <-chan *config.LoggingConfig) {
	if configChannel == nil {
		m.logger.Warn("Config channel is nil, cannot monitor logging configuration changes")
		return
	}

	m.logger.Debug("Starting logging configuration change monitor")

	go func() {
		for {
			select {
			case newConfig := <-configChannel:
				if newConfig == nil {
					m.logger.Debug("Received nil config, ignoring")
					continue
				}

				m.logger.Debug("Received logging configuration update")
				if err := m.SyncLoggingWithApp(newConfig); err != nil {
					m.logger.Errorf("Failed to sync logging configuration: %v", err)
				}

			case <-m.ctx.Done():
				m.logger.Debug("Context cancelled, stopping logging configuration monitor")
				return
			}
		}
	}()
}

// validateAppLoggingConfig 验证应用程序日志配置
func (m *Manager) validateAppLoggingConfig(appConfig *config.LoggingConfig) error {
	if appConfig == nil {
		m.logger.Error("Application logging config validation failed: config is nil")
		return fmt.Errorf("application logging config is nil")
	}

	m.logger.Debugf("Validating application logging config: level=%s, output=%s, file=%s, colors=%t",
		appConfig.Level, appConfig.Output, appConfig.File, appConfig.EnableColors)

	var validationErrors []string

	// 验证日志级别
	validLevels := []string{"trace", "debug", "info", "warn", "error"}
	levelValid := false
	normalizedLevel := strings.ToLower(strings.TrimSpace(appConfig.Level))

	for _, validLevel := range validLevels {
		if normalizedLevel == validLevel {
			levelValid = true
			break
		}
	}

	if !levelValid {
		errorMsg := fmt.Sprintf("invalid log level: %s, must be one of %v", appConfig.Level, validLevels)
		validationErrors = append(validationErrors, errorMsg)
		m.logger.Warnf("Log level validation failed: %s", errorMsg)
	} else {
		m.logger.Debugf("Log level validation passed: %s", normalizedLevel)
	}

	// 验证输出类型
	validOutputs := []string{"stdout", "stderr", "file"}
	outputValid := false

	for _, validOutput := range validOutputs {
		if appConfig.Output == validOutput {
			outputValid = true
			break
		}
	}

	if !outputValid {
		errorMsg := fmt.Sprintf("invalid log output: %s, must be one of %v", appConfig.Output, validOutputs)
		validationErrors = append(validationErrors, errorMsg)
		m.logger.Warnf("Log output validation failed: %s", errorMsg)
	} else {
		m.logger.Debugf("Log output validation passed: %s", appConfig.Output)
	}

	// 如果输出到文件，验证文件路径
	if appConfig.Output == "file" {
		if appConfig.File == "" {
			errorMsg := "log file path is required when output is 'file'"
			validationErrors = append(validationErrors, errorMsg)
			m.logger.Warnf("File path validation failed: %s", errorMsg)
		} else {
			m.logger.Debugf("Validating log file path: %s", appConfig.File)
			if err := m.validateLogFilePath(appConfig.File); err != nil {
				errorMsg := fmt.Sprintf("invalid log file path: %v", err)
				validationErrors = append(validationErrors, errorMsg)
				m.logger.Warnf("File path validation failed: %s", errorMsg)
			} else {
				m.logger.Debugf("File path validation passed: %s", appConfig.File)
			}
		}
	}

	// 检查文件路径与输出类型的一致性
	if appConfig.Output != "file" && appConfig.File != "" {
		warningMsg := fmt.Sprintf("file path specified (%s) but output is not 'file' (%s), file path will be ignored",
			appConfig.File, appConfig.Output)
		m.logger.Warnf("Configuration inconsistency: %s", warningMsg)
		// 这不是错误，只是警告
	}

	if len(validationErrors) > 0 {
		combinedError := fmt.Sprintf("application logging configuration validation failed: %s",
			strings.Join(validationErrors, "; "))
		m.logger.Errorf("Validation summary: %s", combinedError)
		return fmt.Errorf("%s", combinedError)
	}

	m.logger.Debug("Application logging configuration validation passed")
	return nil
}

// detectConfigChanges 检测配置变化
func (m *Manager) detectConfigChanges(appConfig *config.LoggingConfig, currentGstConfig *GStreamerLogConfig) (bool, []string) {
	if appConfig == nil || currentGstConfig == nil {
		return true, []string{"config is nil"}
	}

	var changes []string

	// 检查是否应该启用GStreamer日志
	shouldEnable := strings.ToLower(appConfig.Level) == "debug" || strings.ToLower(appConfig.Level) == "trace"
	if shouldEnable != currentGstConfig.Enabled {
		changes = append(changes, fmt.Sprintf("enabled: %t -> %t", currentGstConfig.Enabled, shouldEnable))
	}

	// 检查日志级别变化
	expectedLevel := m.mapAppLogLevelToGStreamerLevel(appConfig.Level)
	if expectedLevel != currentGstConfig.Level {
		changes = append(changes, fmt.Sprintf("level: %d -> %d", currentGstConfig.Level, expectedLevel))
	}

	// 检查输出文件变化
	expectedFile := m.generateGStreamerLogFile(appConfig)
	if expectedFile != currentGstConfig.OutputFile {
		changes = append(changes, fmt.Sprintf("output: %s -> %s",
			m.getOutputDescription(currentGstConfig.OutputFile),
			m.getOutputDescription(expectedFile)))
	}

	// 检查颜色设置变化
	expectedColored := appConfig.EnableColors && (appConfig.Output != "file")
	if expectedColored != currentGstConfig.Colored {
		changes = append(changes, fmt.Sprintf("colored: %t -> %t", currentGstConfig.Colored, expectedColored))
	}

	return len(changes) > 0, changes
}

// mapAppLogLevelToGStreamerLevel 映射应用程序日志级别到GStreamer级别（管理器版本）
func (m *Manager) mapAppLogLevelToGStreamerLevel(appLogLevel string) int {
	level := strings.ToLower(strings.TrimSpace(appLogLevel))

	switch level {
	case "trace":
		return 9 // GST_LEVEL_LOG - 最详细的日志
	case "debug":
		return 4 // GST_LEVEL_INFO - 调试信息
	case "info", "warn", "error":
		return 0 // GST_LEVEL_NONE - 禁用GStreamer日志
	default:
		m.logger.Warnf("Unknown application log level: %s, defaulting to 0", appLogLevel)
		return 0 // 默认禁用
	}
}

// generateGStreamerLogFile 生成GStreamer日志文件路径（管理器版本）
func (m *Manager) generateGStreamerLogFile(appConfig *config.LoggingConfig) string {
	if appConfig.Output != "file" || appConfig.File == "" {
		return "" // 控制台输出
	}

	// 在应用程序日志目录创建gstreamer.log
	dir := filepath.Dir(appConfig.File)
	return filepath.Join(dir, "gstreamer.log")
}

// validateLogFilePath 验证日志文件路径
func (m *Manager) validateLogFilePath(filePath string) error {
	if filePath == "" {
		return nil // 空路径表示控制台输出，是有效的
	}

	// 检查路径是否为绝对路径
	if !filepath.IsAbs(filePath) {
		return fmt.Errorf("log file path must be absolute: %s", filePath)
	}

	// 检查目录是否存在或可创建
	dir := filepath.Dir(filePath)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		// 尝试创建目录
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("cannot create log directory %s: %w", dir, err)
		}
	}

	// 检查文件是否可写
	if _, err := os.Stat(filePath); err == nil {
		// 文件存在，检查写权限
		file, err := os.OpenFile(filePath, os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return fmt.Errorf("cannot write to log file %s: %w", filePath, err)
		}
		file.Close()
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("cannot access log file %s: %w", filePath, err)
	}

	return nil
}

// NotifyLoggingConfigChange 通知日志配置变化
func (m *Manager) NotifyLoggingConfigChange(newConfig *config.LoggingConfig) {
	if m.configChangeChannel == nil {
		m.logger.Warn("Configuration change channel is not initialized")
		return
	}

	select {
	case m.configChangeChannel <- newConfig:
		m.logger.Debug("Logging configuration change notification sent")
	default:
		m.logger.Warn("Configuration change channel is full, dropping notification")
	}
}

// GetConfigChangeChannel 获取配置变化通道（用于外部监控）
func (m *Manager) GetConfigChangeChannel() chan<- *config.LoggingConfig {
	return m.configChangeChannel
}

// GetLoggingConfig 获取当前GStreamer日志配置
func (m *Manager) GetLoggingConfig() *GStreamerLogConfig {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	if m.logConfigurator == nil {
		return nil
	}

	return m.logConfigurator.GetCurrentConfig()
}

// AdaptToNetworkCondition 根据网络状况调整编码参数
func (m *Manager) AdaptToNetworkCondition(condition NetworkCondition) error {
	m.logger.Debugf("Adapting to network condition: packet_loss=%.2f%%, rtt=%dms, bandwidth=%d kbps",
		condition.PacketLoss*100, condition.RTT, condition.Bandwidth)

	if condition.PacketLoss > 0.05 {
		// 降低比特率 - 使用新的 SetBitrate 方法
		newBitrate := int(float64(m.config.Encoding.Bitrate) * 0.8)
		if newBitrate < m.config.Encoding.MinBitrate {
			newBitrate = m.config.Encoding.MinBitrate
		}

		if m.encoder != nil {
			// Use type assertion to ensure we have the correct interface
			if encoder, ok := m.encoder.(interface{ SetBitrate(int) error }); ok {
				if err := encoder.SetBitrate(newBitrate); err != nil {
					m.logger.Warnf("Failed to set encoder bitrate: %v", err)
					return err
				}
			} else {
				m.logger.Warnf("Encoder does not support SetBitrate method")
			}
		}

		m.config.Encoding.Bitrate = newBitrate
		m.logger.Infof("Adapted to network condition: reduced bitrate to %d kbps", newBitrate)
	}

	if condition.RTT > 200 {
		// 降低帧率 - 使用新的 SetFrameRate 方法
		newFrameRate := m.config.Capture.FrameRate - 5
		if newFrameRate < 15 {
			newFrameRate = 15 // 最低帧率限制
		}

		if m.capture != nil {
			// Use type assertion to ensure we have the correct interface
			if capture, ok := m.capture.(interface{ SetFrameRate(int) error }); ok {
				if err := capture.SetFrameRate(newFrameRate); err != nil {
					m.logger.Warnf("Failed to set capture frame rate: %v", err)
					return err
				}
			} else {
				m.logger.Warnf("Capture does not support SetFrameRate method")
			}
		}

		m.config.Capture.FrameRate = newFrameRate
		m.logger.Infof("Adapted to network condition: reduced frame rate to %d fps", newFrameRate)
	}

	return nil
}
