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

	"github.com/go-gst/go-gst/gst"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"

	"github.com/open-beagle/bdwind-gstreamer/internal/config"
)

// Note: PipelineStateManager, PipelineHealthChecker, and PipelineErrorHandler
// are now implemented in separate files:
// - pipeline_state_manager.go
// - pipeline_health_checker.go
// - pipeline_error_handler.go

// ManagerConfig GStreamer管理器配置
type ManagerConfig struct {
	Config *config.GStreamerConfig
}

// Manager GStreamer管理器，统一管理所有GStreamer相关业务
// 重构后使用go-gst库，保持三个核心集成接口不变：
// 1. 生命周期管理接口 (Start/Stop/IsRunning等)
// 2. 配置管理接口 (GetConfig/ConfigureLogging等)
// 3. 日志管理接口 (日志配置和监控接口)
type Manager struct {
	config *config.GStreamerConfig
	logger *logrus.Entry

	// 使用go-gst重构的组件 (内部实现变更，接口保持不变)
	capture *DesktopCaptureGst // 使用go-gst实现的桌面捕获
	encoder *EncoderGst        // 使用go-gst实现的编码器

	// HTTP处理器 (接口保持不变)
	handlers *gstreamerHandlers

	// 外部进程管理 (保持不变)
	xvfbCmd *exec.Cmd

	// 向后兼容的回调机制 (保持不变)
	encodedSampleCallback func(*Sample) error

	// 生命周期管理接口 (保持不变)
	running   bool
	startTime time.Time
	mutex     sync.RWMutex

	// Pipeline状态管理 (内部使用go-gst，接口保持不变)
	stateManager  *PipelineStateManager
	healthChecker *PipelineHealthChecker
	errorHandler  *PipelineErrorHandler

	// 样本处理统计 (保持不变)
	rawSampleCount     int64
	encodedSampleCount int64
	lastStatsLog       time.Time

	// 发布-订阅机制 (替代复杂回调链，消除GStreamer Bridge)
	streamPublisher *StreamPublisher

	// 向后兼容的订阅者列表 (保持接口不变)
	subscribers []MediaStreamSubscriber
	pubSubMutex sync.RWMutex

	// 日志配置管理接口 (保持不变)
	logConfigurator  GStreamerLogConfigurator
	currentLogConfig *GStreamerLogConfig

	// 配置变化监控 (保持不变)
	configChangeChannel chan *config.LoggingConfig

	// 上下文控制 (保持不变)
	ctx    context.Context
	cancel context.CancelFunc
}

// streamProcessor 流媒体数据处理器 (新增：标准发布-订阅机制)
type streamProcessor struct {
	manager   *Manager
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	logger    *logrus.Entry
	isRunning bool
	mu        sync.RWMutex
}

// NewManager 创建GStreamer管理器 (保持接口不变，内部使用go-gst重构)
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
	logger.Trace("Starting GStreamer manager creation with go-gst refactor")

	// 初始化 GStreamer 库 - 这是使用 go-gst 的必要步骤
	logger.Debug("Initializing GStreamer library via go-gst")
	gst.Init(nil)
	logger.Debug("GStreamer library initialized successfully")

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
		subscribers:         make([]MediaStreamSubscriber, 0),
	}

	// 初始化流发布器 (新增：标准发布-订阅机制)
	streamPublisherConfig := DefaultStreamPublisherConfig()
	manager.streamPublisher = NewStreamPublisher(childCtx, streamPublisherConfig, logger)

	// 初始化日志配置器 (保持接口不变)
	manager.logConfigurator = NewGStreamerLogConfigurator(logger)
	logger.Debug("GStreamer log configurator initialized")

	// Debug级别: 记录详细配置信息
	logger.Debug("Creating manager with go-gst configuration:")
	logger.Debugf("  Display ID: %s", cfg.Config.Capture.DisplayID)
	logger.Debugf("  Encoder Type: %s", cfg.Config.Encoding.Type)
	logger.Debugf("  Codec: %s", cfg.Config.Encoding.Codec)
	logger.Debugf("  Resolution: %dx%d", cfg.Config.Capture.Width, cfg.Config.Capture.Height)
	logger.Debugf("  Frame Rate: %d fps", cfg.Config.Capture.FrameRate)
	logger.Debugf("  Bitrate: %d kbps", cfg.Config.Encoding.Bitrate)
	logger.Debugf("  Use Wayland: %v", cfg.Config.Capture.UseWayland)
	logger.Debugf("  Hardware Acceleration: %v", cfg.Config.Encoding.UseHardware)

	// Trace级别: 记录组件初始化开始
	logger.Trace("Starting go-gst component initialization")

	// 初始化组件 (内部使用go-gst重构)
	if err := manager.initializeComponents(); err != nil {
		// Error级别: 组件初始化失败
		logger.Errorf("Failed to initialize go-gst components: %v", err)
		cancel()
		return nil, fmt.Errorf("failed to initialize components: %w", err)
	}

	// Trace级别: 记录组件初始化完成
	logger.Trace("Go-gst component initialization completed successfully")

	// Debug级别: 管理器创建成功
	logger.Debug("GStreamer manager created successfully with go-gst refactor")
	return manager, nil
}

// initializeComponents 初始化所有GStreamer组件 (使用go-gst重构，保持接口不变)
func (m *Manager) initializeComponents() error {
	// Trace级别: 记录组件初始化详细步骤
	m.logger.Trace("Step 1: Initializing go-gst desktop capture component")

	// 1. 创建go-gst桌面捕获 (内部使用go-gst，接口保持不变)
	var err error
	m.capture, err = NewDesktopCaptureGst(m.config.Capture)
	if err != nil {
		// Error级别: 桌面捕获创建失败
		m.logger.Errorf("Failed to create go-gst desktop capture: %v", err)
		// 不返回错误，允许在没有桌面捕获的情况下运行
		m.capture = nil
		// Debug级别: 记录桌面捕获状态
		m.logger.Debug("Go-gst desktop capture status: unavailable (continuing without capture)")
	} else {
		// Debug级别: 记录桌面捕获详细状态
		m.logger.Debug("Go-gst desktop capture created successfully")
		captureMethod := "X11"
		if m.config.Capture.UseWayland {
			captureMethod = "Wayland"
		}
		m.logger.Debugf("  Capture method: %s (go-gst)", captureMethod)
		m.logger.Debugf("  Display: %s", m.config.Capture.DisplayID)
		m.logger.Debugf("  Capture region: %dx%d", m.config.Capture.Width, m.config.Capture.Height)
		m.logger.Debugf("  Show pointer: %v", m.config.Capture.ShowPointer)
		m.logger.Debugf("  Use damage events: %v", m.config.Capture.UseDamage)
		// Trace级别: 记录桌面捕获内部详细信息
		m.logger.Tracef("Go-gst desktop capture component initialized with buffer size: %d", m.config.Capture.BufferSize)
	}

	// Trace级别: 记录编码器初始化开始
	m.logger.Trace("Step 2: Initializing go-gst video encoder")

	// 2. 创建go-gst编码器 (内部使用go-gst，接口保持不变)
	m.encoder, err = NewEncoderGst(m.config.Encoding)
	if err != nil {
		// Error级别: 编码器创建失败
		m.logger.Errorf("Failed to create go-gst encoder: %v", err)
		return err
	}

	// Debug级别: 记录编码器详细状态
	m.logger.Debug("Go-gst video encoder created successfully")
	m.logger.Debugf("  Encoder type: %s (go-gst)", m.config.Encoding.Type)
	m.logger.Debugf("  Codec: %s", m.config.Encoding.Codec)
	m.logger.Debugf("  Bitrate: %d kbps (min: %d, max: %d)",
		m.config.Encoding.Bitrate, m.config.Encoding.MinBitrate, m.config.Encoding.MaxBitrate)
	m.logger.Debugf("  Hardware acceleration: %v", m.config.Encoding.UseHardware)
	m.logger.Debugf("  Preset: %s", m.config.Encoding.Preset)
	m.logger.Debugf("  Profile: %s", m.config.Encoding.Profile)
	m.logger.Debugf("  Rate control: %s", m.config.Encoding.RateControl)
	m.logger.Debugf("  Zero latency: %v", m.config.Encoding.ZeroLatency)

	// Trace级别: 记录编码器回调设置
	m.logger.Trace("Setting up go-gst encoder sample callback")

	// 设置编码器样本回调 (保持向后兼容)
	if m.encoder != nil {
		m.encoder.SetSampleCallback(m.processEncodedSample)
		// Trace级别: 记录回调设置完成
		m.logger.Trace("Go-gst encoder sample callback configured successfully")
	}

	// Trace级别: 记录Pipeline状态管理初始化开始
	m.logger.Trace("Step 3: Initializing go-gst pipeline state management")

	// 3. 初始化Pipeline状态管理组件 (内部使用go-gst)
	if err := m.initializePipelineStateManagement(); err != nil {
		// Error级别: Pipeline状态管理初始化失败
		m.logger.Errorf("Failed to initialize go-gst pipeline state management: %v", err)
		return err
	}

	// Debug级别: 记录Pipeline状态管理状态
	m.logger.Debug("Go-gst pipeline state management initialized")
	if m.errorHandler != nil {
		m.logger.Debug("  Error handler: enabled")
		m.logger.Debug("  Max retry attempts: 3")
		m.logger.Debug("  Auto recovery: enabled")
	}

	// Trace级别: 记录HTTP处理器初始化开始
	m.logger.Trace("Step 4: Initializing HTTP handlers")

	// 4. 创建HTTP处理器 (保持接口不变)
	m.handlers = newGStreamerHandlers(m)
	// Debug级别: 记录HTTP处理器状态
	m.logger.Debug("HTTP handlers created successfully")
	// Trace级别: 记录HTTP处理器详细信息
	m.logger.Trace("HTTP handlers ready for route registration")

	// Trace级别: 记录流发布器初始化开始
	m.logger.Trace("Step 5: Initializing stream publisher for publish-subscribe")

	// 5. 启动流发布器 (新增：发布-订阅机制)
	if err := m.streamPublisher.Start(); err != nil {
		m.logger.Errorf("Failed to start stream publisher: %v", err)
		return err
	}
	m.logger.Debug("Stream publisher initialized for publish-subscribe mechanism")

	// Trace级别: 记录所有组件初始化完成
	m.logger.Trace("All go-gst components initialized successfully")

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
	if m.capture == nil {
		m.logger.Warn("Desktop capture is not initialized, skipping state management")
		return nil
	}

	// 等待pipeline创建（在Start方法中创建）
	// 这里我们只初始化错误处理器，状态管理器将在Start方法中初始化

	// 1. 创建错误处理器
	errorConfig := DefaultPipelineErrorHandlerConfig()
	m.errorHandler = NewPipelineErrorHandler(m.logger.WithField("subcomponent", "error-handler"), errorConfig)
	m.logger.Debug("Pipeline error handler created successfully")

	return nil
}

// initializePipelineStateManagerForCapture 为go-gst桌面捕获初始化状态管理器
func (m *Manager) initializePipelineStateManagerForCapture() error {
	if m.capture == nil {
		return nil
	}

	// 获取go-gst桌面捕获的pipeline
	if m.capture == nil || m.capture.pipeline == nil {
		return fmt.Errorf("go-gst desktop capture pipeline not available")
	}

	// 创建状态管理器配置
	stateConfig := DefaultPipelineStateManagerConfig()

	// 创建状态管理器
	m.stateManager = NewPipelineStateManager(m.ctx, m.capture.pipeline,
		m.logger.WithField("subcomponent", "state-manager"), stateConfig)

	// 设置错误处理器的状态管理器引用
	if m.errorHandler != nil {
		m.errorHandler.SetStateManager(m.stateManager)
	}

	// 设置状态变化回调
	m.stateManager.SetStateChangeCallback(m.onPipelineStateChange)
	m.stateManager.SetErrorCallback(m.onPipelineError)

	// 启动状态管理器
	if err := m.stateManager.Start(); err != nil {
		return fmt.Errorf("failed to start go-gst pipeline state manager: %w", err)
	}

	// 启动错误处理器
	if m.errorHandler != nil {
		if err := m.errorHandler.Start(m.ctx); err != nil {
			return fmt.Errorf("failed to start pipeline error handler: %w", err)
		}
	}

	m.logger.Debug("Go-gst pipeline state manager initialized and started successfully")
	return nil
}

// onPipelineStateChange 处理Pipeline状态变化回调
func (m *Manager) onPipelineStateChange(oldState, newState PipelineState, transition ExtendedStateTransition) {
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
func (m *Manager) onPipelineError(errorType ErrorType, message string, err error) {
	m.logger.Errorf("Pipeline error: %s - %v", message, err)

	// 通过错误处理器处理错误
	if m.errorHandler != nil {
		details := ""
		if err != nil {
			details = err.Error()
		}
		m.errorHandler.HandleError(errorType, message, details, "Pipeline")
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

// Start 启动GStreamer管理器 (保持生命周期管理接口不变)
func (m *Manager) Start(ctx context.Context) error {
	startTime := time.Now()
	m.logger.Info("Starting GStreamer manager...")

	m.mutex.Lock()
	if m.running {
		m.mutex.Unlock()
		m.logger.Warn("GStreamer manager already running, skipping start")
		return fmt.Errorf("GStreamer manager already running")
	}
	// Release mutex early to avoid deadlock with goroutines
	m.mutex.Unlock()

	// 记录GStreamer配置信息
	m.logger.Debugf("GStreamer configuration: display=%s, codec=%s, resolution=%dx%d@%dfps, bitrate=%dkbps",
		m.config.Capture.DisplayID, m.config.Encoding.Codec,
		m.config.Capture.Width, m.config.Capture.Height, m.config.Capture.FrameRate,
		m.config.Encoding.Bitrate)
	m.logger.Debugf("Hardware acceleration: %v, preset: %s, profile: %s",
		m.config.Encoding.UseHardware, m.config.Encoding.Preset, m.config.Encoding.Profile)

	m.startTime = time.Now()

	// 验证组件状态
	m.logger.Debug("Verifying component availability before startup...")
	m.logger.Debugf("Component status: capture=%v, encoder=%v, error_handler=%v, http_handlers=%v, stream_publisher=%v",
		m.capture != nil, m.encoder != nil, m.errorHandler != nil, m.handlers != nil, m.streamPublisher != nil)

	// 启动桌面捕获组件 (异步启动以避免阻塞主启动序列)
	if m.capture != nil {
		m.logger.Debug("Starting desktop capture component...")
		m.logger.Tracef("Display ID: %s, capture region: %dx%d", 
			m.config.Capture.DisplayID, m.config.Capture.Width, m.config.Capture.Height)

		// Start desktop capture in a goroutine with timeout to avoid blocking startup
		go func() {
			m.logger.Info("DEBUG: Desktop capture goroutine started")
			// Use a timeout context for desktop capture startup
			captureCtx, captureCancel := context.WithTimeout(m.ctx, 15*time.Second)
			defer captureCancel()

			done := make(chan error, 1)
			go func() {
				m.logger.Info("DEBUG: About to call startDesktopCaptureWithContext")
				done <- m.startDesktopCaptureWithContext()
			}()

			select {
			case err := <-done:
				m.logger.Info("DEBUG: Desktop capture goroutine completed")
				if err != nil {
					m.logger.Errorf("Failed to start desktop capture: %v", err)
					m.logger.Warn("Desktop capture failed - continuing without video capture")
				} else {
					m.logger.Debug("Desktop capture started successfully")
					m.logger.Debug("Sample callback chain established")

					// 初始化Pipeline状态管理器
					m.logger.Debug("Initializing pipeline state manager...")
					if err := m.initializePipelineStateManagerForCapture(); err != nil {
						m.logger.Errorf("Failed to initialize pipeline state manager: %v", err)
						m.logger.Warn("Pipeline state management disabled - continuing without monitoring")
					} else {
						m.logger.Debug("Pipeline state manager initialized successfully")
					}
				}
			case <-captureCtx.Done():
				m.logger.Warn("Desktop capture startup timed out after 15 seconds - continuing without video capture")
			}
			m.logger.Info("DEBUG: Desktop capture goroutine exiting")
		}()

		// Don't wait for desktop capture to complete - continue with startup
		m.logger.Debug("Desktop capture startup initiated asynchronously")
	} else {
		m.logger.Warn("Desktop capture component not available")
	}

	// 启动视频编码器 (异步启动以避免阻塞)
	m.logger.Info("DEBUG: About to start video encoder")
	if m.encoder != nil {
		m.logger.Debug("Starting video encoder...")
		m.logger.Tracef("Encoder codec: %s, bitrate: %dkbps, hardware: %v", 
			m.config.Encoding.Codec, m.config.Encoding.Bitrate, m.config.Encoding.UseHardware)
		
		// Start encoder asynchronously to avoid blocking startup
		go func() {
			m.logger.Info("DEBUG: Encoder goroutine started")
			if err := m.encoder.Start(); err != nil {
				m.logger.Errorf("Failed to start video encoder: %v", err)
				m.logger.Warn("Video encoder failed - continuing without encoding")
			} else {
				m.logger.Debug("Video encoder started successfully")
			}
			m.logger.Info("DEBUG: Encoder goroutine completed")
		}()
		
		m.logger.Debug("Video encoder startup initiated asynchronously")
	} else {
		m.logger.Warn("Video encoder component not available")
	}
	m.logger.Info("DEBUG: Video encoder section completed")

	// 启动流发布器
	m.logger.Info("DEBUG: About to check stream publisher")
	if m.streamPublisher != nil {
		m.logger.Debug("Starting stream publisher...")
		m.logger.Info("DEBUG: About to call IsRunning()")
		if !m.streamPublisher.IsRunning() {
			m.logger.Warn("Stream publisher not running - media distribution may be affected")
		} else {
			m.logger.Info("DEBUG: About to call GetSubscriberCount()")
			subscriberCount := m.streamPublisher.GetSubscriberCount()
			m.logger.Debugf("Stream publisher active with %d subscribers", subscriberCount)
		}
		m.logger.Info("DEBUG: Stream publisher check completed")
	}

	// 启动配置监控
	m.logger.Debug("Starting configuration monitoring...")
	m.logger.Info("DEBUG: About to call MonitorLoggingConfigChanges")
	m.MonitorLoggingConfigChanges(m.configChangeChannel)
	m.logger.Info("DEBUG: MonitorLoggingConfigChanges completed")

	m.logger.Info("DEBUG: About to set running = true")
	m.mutex.Lock()
	m.running = true
	m.mutex.Unlock()
	m.logger.Info("DEBUG: Set running = true")
	duration := time.Since(startTime)
	
	// 记录最终状态
	captureStatus := "disabled"
	if m.capture != nil {
		captureStatus = "active"
	}
	encoderStatus := "disabled"
	if m.encoder != nil {
		encoderStatus = "active"
	}
	
	m.logger.Infof("GStreamer manager started successfully in %v (capture: %s, encoder: %s)", 
		duration, captureStatus, encoderStatus)
	
	// 记录组件摘要
	m.logger.Infof("GStreamer ready: display capture from %s, %s encoding", 
		m.config.Capture.DisplayID, m.config.Encoding.Codec)

	// Add explicit completion log to help debug startup sequence
	m.logger.Info("GStreamer manager Start() method completing successfully")
	m.logger.Info("DEBUG: About to return from GStreamer Start() method")

	return nil
}

// startDesktopCaptureWithContext 启动go-gst桌面捕获并处理外部进程
func (m *Manager) startDesktopCaptureWithContext() error {
	// Debug级别: 记录样本回调链路建立开始
	m.logger.Debug("Starting go-gst desktop capture with sample callback chain")

	// 启动go-gst桌面捕获
	if err := m.capture.Start(); err != nil {
		// Error级别: 链路中断 - 桌面捕获启动失败
		m.logger.Errorf("Go-gst sample callback chain failed: desktop capture start error: %v", err)
		return fmt.Errorf("failed to start go-gst desktop capture: %w", err)
	}

	// Debug级别: 记录组件状态验证结果
	m.logger.Debug("Go-gst desktop capture started successfully")

	// 设置sample回调来处理视频帧数据 (保持向后兼容)
	if err := m.capture.SetSampleCallback(m.processSample); err != nil {
		// Error级别: 链路中断 - 样本回调设置失败
		m.logger.Errorf("Go-gst sample callback chain failed: callback setup error: %v", err)
		return fmt.Errorf("failed to set go-gst sample callback: %w", err)
	}

	// Debug级别: 记录样本回调链路建立成功状态
	m.logger.Debug("Go-gst sample callback chain established successfully")
	m.logger.Debug("Go-gst sample callback chain configuration:")
	m.logger.Debug("  Raw sample callback: configured")
	m.logger.Debug("  Target processor: GStreamer manager")
	m.logger.Debug("  Encoder integration: enabled (go-gst)")
	m.logger.Debug("  Publish-subscribe: enabled")

	// 启动桌面捕获监控goroutine，监听context取消信号
	go m.monitorDesktopCapture()

	return nil
}

// processSample 处理从go-gst pipeline接收到的视频帧数据
func (m *Manager) processSample(sample *Sample) error {
	if sample == nil {
		// Warn级别: 样本处理异常
		m.logger.Warn("Go-gst sample processing failed: received nil sample")
		return fmt.Errorf("received nil sample")
	}

	// 更新统计计数
	m.mutex.Lock()
	m.rawSampleCount++
	currentCount := m.rawSampleCount
	m.mutex.Unlock()

	// Trace级别: 记录每个样本的详细处理信息和类型识别
	m.logger.Tracef("Processing go-gst raw sample #%d: type=%s, size=%d bytes, format=%s, dimensions=%dx%d, timestamp=%v",
		currentCount, sample.Format.MediaType.String(), sample.Size(), sample.Format.Codec,
		sample.Format.Width, sample.Format.Height, sample.Timestamp)

	// Debug级别: 记录样本处理统计（每 1000 个样本）
	if currentCount%1000 == 0 {
		m.logger.Debugf("Go-gst raw sample processing milestone: %d samples processed", currentCount)
	}

	if m.encoder != nil {
		if err := m.encoder.PushSample(sample); err != nil {
			// Warn级别: 样本处理异常
			m.logger.Warnf("Go-gst sample processing failed: error pushing sample to encoder: %v", err)
			return err
		}
		// Trace级别: 记录样本成功推送到编码器
		m.logger.Trace("Go-gst raw sample successfully pushed to encoder")
	} else {
		// Warn级别: 样本处理异常 - 编码器不可用
		m.logger.Warn("Go-gst sample processing warning: encoder not available")
	}

	return nil
}

// processEncodedSample 处理从go-gst编码器接收到的编码样本 (使用发布-订阅机制)
func (m *Manager) processEncodedSample(sample *Sample) error {
	if sample == nil {
		// Warn级别: 样本处理异常
		m.logger.Warn("Go-gst encoded sample processing failed: received nil sample")
		return fmt.Errorf("received nil sample")
	}

	// 更新统计计数
	m.mutex.Lock()
	m.encodedSampleCount++
	currentCount := m.encodedSampleCount
	callback := m.encodedSampleCallback
	m.mutex.Unlock()

	// Trace级别: 记录每个样本的详细处理信息和类型识别
	m.logger.Tracef("Processing go-gst encoded sample #%d: type=%s, size=%d bytes, codec=%s, timestamp=%v",
		currentCount, sample.Format.MediaType.String(), sample.Size(), sample.Format.Codec, sample.Timestamp)

	// Debug级别: 记录样本处理统计（每 1000 个样本）
	if currentCount%1000 == 0 {
		m.logger.Debugf("Go-gst encoded sample processing milestone: %d samples processed", currentCount)
	}

	// 使用发布-订阅机制发布流数据 (替代GStreamer Bridge)
	if err := m.streamPublisher.PublishVideoSample(sample); err != nil {
		m.logger.Warnf("Failed to publish go-gst encoded sample to stream publisher: %v", err)
	}

	// 保持原有的回调机制用于向后兼容 (保持接口不变)
	if callback != nil {
		if err := callback(sample); err != nil {
			// Warn级别: 样本处理异常
			m.logger.Warnf("Go-gst encoded sample processing failed: error in callback: %v", err)
			return err
		}
		// Trace级别: 记录编码样本成功处理
		m.logger.Trace("Go-gst encoded sample successfully processed through callback")
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

// Stop 停止GStreamer管理器和所有组件 (保持生命周期管理接口不变)
func (m *Manager) Stop(ctx context.Context) error {
	m.mutex.Lock()
	if !m.running {
		m.mutex.Unlock()
		return nil
	}
	m.running = false
	m.mutex.Unlock()

	m.logger.Trace("Stopping GStreamer manager with go-gst refactor...")

	// 停止流发布器 (新增：发布-订阅机制)
	if m.streamPublisher != nil {
		m.logger.Debug("Stopping stream publisher...")
		if err := m.streamPublisher.Stop(); err != nil {
			m.logger.Warnf("Failed to stop stream publisher: %v", err)
		} else {
			m.logger.Debug("Stream publisher stopped successfully")
		}
	}

	// 停止go-gst编码器
	if m.encoder != nil {
		if err := m.encoder.Stop(); err != nil {
			m.logger.Warnf("Failed to stop go-gst encoder: %v", err)
		} else {
			m.logger.Debug("Go-gst encoder stopped successfully")
		}
	}

	// 停止Pipeline状态管理组件
	if m.stateManager != nil {
		if err := m.stateManager.Stop(); err != nil {
			m.logger.Warnf("Failed to stop go-gst pipeline state manager: %v", err)
		} else {
			m.logger.Debug("Go-gst pipeline state manager stopped successfully")
		}
	}

	// 停止错误处理器
	if m.errorHandler != nil {
		if err := m.errorHandler.Stop(); err != nil {
			m.logger.Warnf("Failed to stop pipeline error handler: %v", err)
		} else {
			m.logger.Debug("Pipeline error handler stopped successfully")
		}
	}

	// 停止go-gst桌面捕获
	if m.capture != nil {
		if err := m.capture.Stop(); err != nil {
			m.logger.Warnf("Failed to stop go-gst desktop capture: %v", err)
		} else {
			m.logger.Debug("Go-gst desktop capture stopped successfully")
		}
	}

	// 流发布器已在上面停止，无需额外关闭通道

	// 关闭配置变化通道 (保持接口不变)
	if m.configChangeChannel != nil {
		close(m.configChangeChannel)
		m.configChangeChannel = nil
		m.logger.Debug("Configuration change channel closed")
	}

	// 取消上下文
	m.cancel()

	m.logger.Info("GStreamer manager stopped successfully with go-gst refactor")
	return nil
}

// ForceStop 强制停止GStreamer管理器，直接停止pipeline和状态管理器
func (m *Manager) ForceStop() error {
	m.logger.Warn("Force stopping GStreamer manager")

	m.mutex.Lock()
	if !m.running {
		m.mutex.Unlock()
		m.logger.Debug("GStreamer manager is not running, nothing to force stop")
		return nil
	}
	m.running = false
	m.mutex.Unlock()

	// 立即取消context，停止所有监控goroutine
	m.cancel()
	m.logger.Debug("Context cancelled for force stop")

	// 强制停止状态管理器
	if m.stateManager != nil {
		if err := m.forceStopStateManager(); err != nil {
			m.logger.Errorf("Failed to force stop state manager: %v", err)
		} else {
			m.logger.Debug("State manager force stopped successfully")
		}
	}

	// 强制停止pipeline
	if m.capture != nil {
		if err := m.forceStopPipeline(); err != nil {
			m.logger.Errorf("Failed to force stop pipeline: %v", err)
		} else {
			m.logger.Debug("Pipeline force stopped successfully")
		}
	}

	// 强制关闭配置变化通道
	if m.configChangeChannel != nil {
		close(m.configChangeChannel)
		m.configChangeChannel = nil
		m.logger.Debug("Configuration change channel force closed")
	}

	m.logger.Info("GStreamer manager force stopped successfully")
	return nil
}

// forceStopStateManager 强制停止状态管理器
func (m *Manager) forceStopStateManager() error {
	if m.stateManager == nil {
		return nil
	}

	m.logger.Debug("Force stopping pipeline state manager")

	// 检查状态管理器是否实现了ForceStop方法
	if forceStoppable, ok := interface{}(m.stateManager).(interface{ ForceStop() error }); ok {
		m.logger.Debug("State manager supports ForceStop method, calling it")
		return forceStoppable.ForceStop()
	}

	// 否则使用常规Stop方法
	m.logger.Debug("State manager does not support ForceStop, using regular Stop")
	return m.stateManager.Stop()
}

// forceStopPipeline 强制停止pipeline，直接设置状态为NULL并释放资源
func (m *Manager) forceStopPipeline() error {
	if m.capture == nil {
		return nil
	}

	m.logger.Debug("Force stopping GStreamer pipeline")

	// 尝试访问desktop capture pipeline
	if m.capture != nil && m.capture.pipeline != nil {
		m.logger.Debug("Accessing desktop capture pipeline for force stop")

		// 直接设置pipeline状态为NULL，不等待转换完成
		m.logger.Debug("Setting pipeline state to NULL immediately")
		if err := m.capture.pipeline.SetState(gst.StateNull); err != nil {
			m.logger.Warnf("Failed to set pipeline state to NULL: %v", err)
			// 继续执行，不返回错误
		}

		// 发送EOS事件以确保pipeline清理
		m.logger.Debug("Sending EOS event to pipeline")
		// Note: go-gst Pipeline doesn't have SendEOS method, using SendEvent instead
		// This is commented out for now as the exact API needs to be verified
		// if err := m.capture.pipeline.SendEvent(gst.NewEOSEvent()); err != nil {
		//     m.logger.Debugf("Failed to send EOS (expected during force stop): %v", err)
		// }

		m.logger.Debug("Pipeline force stop completed")
	} else {
		m.logger.Debug("Desktop capture is not the expected type or pipeline is nil")

		// 尝试使用常规Stop方法
		if err := m.capture.Stop(); err != nil {
			m.logger.Warnf("Failed to stop capture using regular method: %v", err)
			return err
		}
	}

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

// GetCapture 获取桌面捕获实例 (保持接口不变，内部返回go-gst实现)
func (m *Manager) GetCapture() DesktopCapture {
	// 返回go-gst实现，但保持DesktopCapture接口不变
	return m.capture
}

// SetEncodedSampleCallback sets the callback function for processing encoded video samples
func (m *Manager) SetEncodedSampleCallback(callback func(*Sample) error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.encodedSampleCallback = callback
}

// AddSubscriber 添加媒体流订阅者 (使用新的发布-订阅机制)
func (m *Manager) AddSubscriber(subscriber MediaStreamSubscriber) (uint64, error) {
	// 添加到流发布器
	subID, err := m.streamPublisher.AddMediaSubscriber(subscriber)
	if err != nil {
		return 0, err
	}

	// 保持向后兼容的订阅者列表
	m.pubSubMutex.Lock()
	m.subscribers = append(m.subscribers, subscriber)
	m.pubSubMutex.Unlock()

	m.logger.Infof("Added subscriber (ID: %d), total subscribers: %d", subID, len(m.subscribers))
	return subID, nil
}

// RemoveSubscriber 移除媒体流订阅者
func (m *Manager) RemoveSubscriber(id uint64) bool {
	// 从流发布器移除
	removed := m.streamPublisher.RemoveMediaSubscriber(id)
	if !removed {
		return false
	}

	// 从向后兼容列表中移除 (需要通过ID查找，这里简化处理)
	m.pubSubMutex.Lock()
	// 注意：这里无法直接通过ID移除，因为原有接口没有ID概念
	// 实际使用中应该维护ID到subscriber的映射
	m.pubSubMutex.Unlock()

	m.logger.Infof("Removed subscriber (ID: %d)", id)
	return true
}

// PublishVideoStream 发布视频流给所有订阅者 (保持向后兼容接口)
func (m *Manager) PublishVideoStream(stream *EncodedVideoStream) error {
	// 通过流发布器发布 (内部会处理所有订阅者)
	// 这里需要将EncodedVideoStream转换回Sample格式
	sample := &Sample{
		Data:      stream.Data,
		Timestamp: time.UnixMilli(stream.Timestamp),
		Format: SampleFormat{
			MediaType: MediaTypeVideo,
			Codec:     stream.Codec,
			Width:     stream.Width,
			Height:    stream.Height,
		},
		Metadata: map[string]interface{}{
			"key_frame": stream.KeyFrame,
			"bitrate":   stream.Bitrate,
		},
	}

	return m.streamPublisher.PublishVideoSample(sample)
}

// PublishAudioStream 发布音频流给所有订阅者 (保持向后兼容接口)
func (m *Manager) PublishAudioStream(stream *EncodedAudioStream) error {
	// 通过流发布器发布 (内部会处理所有订阅者)
	// 这里需要将EncodedAudioStream转换回Sample格式
	sample := &Sample{
		Data:      stream.Data,
		Timestamp: time.UnixMilli(stream.Timestamp),
		Format: SampleFormat{
			MediaType:  MediaTypeAudio,
			Codec:      stream.Codec,
			SampleRate: stream.SampleRate,
			Channels:   stream.Channels,
		},
		Metadata: map[string]interface{}{
			"bitrate": stream.Bitrate,
		},
	}

	return m.streamPublisher.PublishAudioSample(sample)
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
				"uptime":       healthStatus.Uptime,
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

		// 添加流发布器统计 (新增：发布-订阅机制统计)
		if m.streamPublisher != nil {
			publisherStats := m.streamPublisher.GetStatistics()
			stats["stream_publisher"] = map[string]interface{}{
				"running":                m.streamPublisher.IsRunning(),
				"video_frames_published": publisherStats.VideoFramesPublished,
				"audio_frames_published": publisherStats.AudioFramesPublished,
				"video_frames_dropped":   publisherStats.VideoFramesDropped,
				"audio_frames_dropped":   publisherStats.AudioFramesDropped,
				"video_subscribers":      publisherStats.VideoSubscribers,
				"audio_subscribers":      publisherStats.AudioSubscribers,
				"total_subscribers":      publisherStats.TotalSubscribers,
				"active_subscribers":     publisherStats.ActiveSubscribers,
				"publish_errors":         publisherStats.PublishErrors,
				"avg_video_frame_size":   publisherStats.AverageVideoFrameSize,
				"avg_audio_frame_size":   publisherStats.AverageAudioFrameSize,
			}

			// Stream processor stats are included in main statistics

			// Subscriber manager stats are included in main statistics
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
	overallStatus := m.healthChecker.GetHealthStatus()
	return overallStatus.HealthStatus, nil
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
	extendedHistory := m.stateManager.GetStateHistory()
	history := make([]StateTransition, len(extendedHistory))
	for i, ext := range extendedHistory {
		history[i] = ext.StateTransition
	}
	return history
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

// AdaptToNetworkCondition 根据网络状况调整编码参数 (保持接口不变，内部使用go-gst)
func (m *Manager) AdaptToNetworkCondition(condition NetworkCondition) error {
	m.logger.Debugf("Adapting go-gst components to network condition: packet_loss=%.2f%%, rtt=%dms, bandwidth=%d kbps",
		condition.PacketLoss*100, condition.RTT, condition.Bandwidth)

	if condition.PacketLoss > 0.05 {
		// 降低比特率 - 使用go-gst编码器的SetBitrate方法
		newBitrate := int(float64(m.config.Encoding.Bitrate) * 0.8)
		if newBitrate < m.config.Encoding.MinBitrate {
			newBitrate = m.config.Encoding.MinBitrate
		}

		if m.encoder != nil {
			// go-gst编码器支持SetBitrate方法
			if err := m.encoder.SetBitrate(newBitrate); err != nil {
				m.logger.Warnf("Failed to set go-gst encoder bitrate: %v", err)
				return err
			}
		}

		m.config.Encoding.Bitrate = newBitrate
		m.logger.Infof("Adapted go-gst encoder to network condition: reduced bitrate to %d kbps", newBitrate)
	}

	if condition.RTT > 200 {
		// 降低帧率 - 使用go-gst桌面捕获的SetFrameRate方法
		newFrameRate := m.config.Capture.FrameRate - 5
		if newFrameRate < 15 {
			newFrameRate = 15 // 最低帧率限制
		}

		if m.capture != nil {
			// go-gst桌面捕获支持SetFrameRate方法
			if err := m.capture.SetFrameRate(newFrameRate); err != nil {
				m.logger.Warnf("Failed to set go-gst capture frame rate: %v", err)
				return err
			}
		}

		m.config.Capture.FrameRate = newFrameRate
		m.logger.Infof("Adapted go-gst capture to network condition: reduced frame rate to %d fps", newFrameRate)
	}

	return nil
}

// MediaStreamProvider interface implementation
// These methods provide the new interface for WebRTC integration

// AddVideoSubscriber adds a video stream subscriber and returns its unique ID
// Implements MediaStreamProvider interface
func (m *Manager) AddVideoSubscriber(subscriber VideoStreamSubscriber) (uint64, error) {
	m.logger.Debug("AddVideoSubscriber called on GStreamer manager")
	
	if m.streamPublisher == nil {
		m.logger.Error("Stream publisher not initialized")
		return 0, fmt.Errorf("stream publisher not initialized")
	}
	
	m.logger.Debugf("Stream publisher status: running=%v", m.streamPublisher.IsRunning())
	m.logger.Debug("Calling AddVideoSubscriber on stream publisher...")

	id, err := m.streamPublisher.AddVideoSubscriber(subscriber)
	if err != nil {
		m.logger.Errorf("Failed to add video subscriber: %v", err)
		return 0, err
	}

	m.logger.Infof("Added video subscriber with ID %d", id)
	return id, nil
}

// RemoveVideoSubscriber removes a video stream subscriber by ID
// Implements MediaStreamProvider interface
func (m *Manager) RemoveVideoSubscriber(id uint64) bool {
	if m.streamPublisher == nil {
		m.logger.Warn("Stream publisher not initialized")
		return false
	}

	removed := m.streamPublisher.RemoveVideoSubscriber(id)
	if removed {
		m.logger.Infof("Removed video subscriber with ID %d", id)
	} else {
		m.logger.Warnf("Failed to remove video subscriber with ID %d (not found)", id)
	}

	return removed
}

// AddAudioSubscriber adds an audio stream subscriber and returns its unique ID
// Implements MediaStreamProvider interface
func (m *Manager) AddAudioSubscriber(subscriber AudioStreamSubscriber) (uint64, error) {
	if m.streamPublisher == nil {
		return 0, fmt.Errorf("stream publisher not initialized")
	}

	id, err := m.streamPublisher.AddAudioSubscriber(subscriber)
	if err != nil {
		m.logger.Errorf("Failed to add audio subscriber: %v", err)
		return 0, err
	}

	m.logger.Infof("Added audio subscriber with ID %d", id)
	return id, nil
}

// RemoveAudioSubscriber removes an audio stream subscriber by ID
// Implements MediaStreamProvider interface
func (m *Manager) RemoveAudioSubscriber(id uint64) bool {
	if m.streamPublisher == nil {
		m.logger.Warn("Stream publisher not initialized")
		return false
	}

	removed := m.streamPublisher.RemoveAudioSubscriber(id)
	if removed {
		m.logger.Infof("Removed audio subscriber with ID %d", id)
	} else {
		m.logger.Warnf("Failed to remove audio subscriber with ID %d (not found)", id)
	}

	return removed
}

// Compile-time check to ensure Manager implements MediaStreamProvider interface
var _ MediaStreamProvider = (*Manager)(nil)
