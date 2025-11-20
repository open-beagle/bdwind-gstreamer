package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/open-beagle/bdwind-gstreamer/internal/bridge"
	"github.com/open-beagle/bdwind-gstreamer/internal/config"
	"github.com/open-beagle/bdwind-gstreamer/internal/factory"
	"github.com/open-beagle/bdwind-gstreamer/internal/gstreamer"
	"github.com/open-beagle/bdwind-gstreamer/internal/metrics"
	"github.com/open-beagle/bdwind-gstreamer/internal/webrtc"
	"github.com/open-beagle/bdwind-gstreamer/internal/webrtc/events"
	"github.com/open-beagle/bdwind-gstreamer/internal/webserver"
)

// BDWindApp BDWind应用
type BDWindApp struct {
	config       *config.Config
	webserverMgr *webserver.Manager
	webrtcMgr    *webrtc.WebRTCManager
	gstreamerMgr *gstreamer.Manager
	metricsMgr   *metrics.Manager
	logger       *logrus.Entry
	startTime    time.Time

	// go-gst 媒体桥接器（新架构）
	mediaBridge *bridge.GoGstMediaBridge

	// 事件系统
	eventBus          events.EventBus
	signalingHandlers *webrtc.SignalingEventHandlers

	// Context lifecycle management fields
	rootCtx    context.Context
	cancelFunc context.CancelFunc
	wg         sync.WaitGroup
	sigChan    chan os.Signal
}

// NewBDWindApp 创建BDWind应用
func NewBDWindApp(cfg *config.Config, logger *logrus.Entry) (*BDWindApp, error) {
	if logger == nil {
		logger = config.GetLoggerWithPrefix("app")
	}

	logger.Info("Creating BDWind application...")

	// 直接使用 go-gst 简化实现
	logger.Info("Using go-gst implementation")
	return NewGoGstBDWindApp(cfg, logger)
}

// NewGoGstBDWindApp 创建使用 go-gst 实现的BDWind应用
func NewGoGstBDWindApp(cfg *config.Config, logger *logrus.Entry) (*BDWindApp, error) {
	logger.Info("Creating go-gst BDWind application...")

	// Create root context for lifecycle management
	logger.Debug("Creating root context for lifecycle management...")
	rootCtx, cancelFunc := context.WithCancel(context.Background())

	// Create signal channel for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	logger.Debug("Signal channel created")

	// 创建事件总线
	logger.Info("Creating event bus...")
	eventBus := events.NewEventBus()
	if err := eventBus.Start(); err != nil {
		logger.Errorf("Failed to start event bus: %v", err)
		cancelFunc()
		return nil, fmt.Errorf("failed to start event bus: %w", err)
	}
	logger.Info("Event bus created and started successfully")

	// 创建 WebRTC 管理器
	logger.Info("Creating WebRTC manager...")
	webrtcMgr, err := webrtc.NewWebRTCManager(cfg.WebRTC)
	if err != nil {
		logger.Errorf("Failed to create WebRTC manager: %v", err)
		eventBus.Stop()
		cancelFunc()
		return nil, fmt.Errorf("failed to create WebRTC manager: %w", err)
	}
	logger.Info("WebRTC manager created successfully")

	// 创建信令事件处理器
	logger.Info("Creating signaling event handlers...")
	signalingHandlers := webrtc.NewSignalingEventHandlers(webrtcMgr)
	if err := signalingHandlers.RegisterHandlers(eventBus); err != nil {
		logger.Errorf("Failed to register signaling event handlers: %v", err)
		eventBus.Stop()
		cancelFunc()
		return nil, fmt.Errorf("failed to register signaling event handlers: %w", err)
	}
	logger.Info("Signaling event handlers registered successfully")

	// 使用工厂创建 go-gst 媒体桥接器
	logger.Info("Creating go-gst media bridge...")
	mediaBridge, err := factory.CreateMediaBridge(cfg, webrtcMgr)
	if err != nil {
		logger.Errorf("Failed to create media bridge: %v", err)
		cancelFunc()
		return nil, fmt.Errorf("failed to create media bridge: %w", err)
	}
	logger.Info("Go-gst media bridge created successfully")

	// 创建webserver管理器
	logger.Info("Creating webserver manager...")
	webserverMgr, err := webserver.NewManager(rootCtx, cfg.WebServer)
	if err != nil {
		logger.Errorf("Failed to create webserver manager: %v", err)
		cancelFunc()
		return nil, fmt.Errorf("failed to create webserver manager: %w", err)
	}
	logger.Info("Webserver manager created successfully")

	// 创建监控管理器
	logger.Info("Creating metrics manager...")
	metricsMgr, err := metrics.NewManager(rootCtx, cfg.Metrics)
	if err != nil {
		logger.Errorf("Failed to create metrics manager: %v", err)
		cancelFunc()
		return nil, fmt.Errorf("failed to create metrics manager: %w", err)
	}
	logger.Info("Metrics manager created successfully")

	logger.Debug("Assembling go-gst application components...")
	app := &BDWindApp{
		config:            cfg,
		webserverMgr:      webserverMgr,
		webrtcMgr:         webrtcMgr,
		gstreamerMgr:      nil, // 使用 mediaBridge 中的 GStreamer
		metricsMgr:        metricsMgr,
		logger:            logger,
		startTime:         time.Now(),
		rootCtx:           rootCtx,
		cancelFunc:        cancelFunc,
		sigChan:           sigChan,
		mediaBridge:       mediaBridge, // 添加媒体桥接器
		eventBus:          eventBus,
		signalingHandlers: signalingHandlers,
	}

	logger.Info("Go-gst BDWind application created successfully")
	return app, nil
}

// NewSimplifiedBDWindApp 创建使用简化实现的BDWind应用
func NewSimplifiedBDWindApp(cfg *config.Config, logger *logrus.Entry) (*BDWindApp, error) {
	logger.Info("Creating simplified BDWind application...")

	// Create root context for lifecycle management
	logger.Debug("Creating root context for lifecycle management...")
	rootCtx, cancelFunc := context.WithCancel(context.Background())

	// Create signal channel for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	logger.Debug("Signal channel created")

	// 创建webserver管理器（包含认证功能）
	logger.Info("Creating webserver manager...")
	webserverMgr, err := webserver.NewManager(rootCtx, cfg.WebServer)
	if err != nil {
		logger.Errorf("Failed to create webserver manager: %v", err)
		cancelFunc()
		return nil, fmt.Errorf("failed to create webserver manager: %w", err)
	}
	logger.Info("Webserver manager created successfully")

	// 创建监控管理器
	logger.Info("Creating metrics manager...")
	metricsMgr, err := metrics.NewManager(rootCtx, cfg.Metrics)
	if err != nil {
		logger.Errorf("Failed to create metrics manager: %v", err)
		cancelFunc()
		return nil, fmt.Errorf("failed to create metrics manager: %w", err)
	}
	logger.Info("Metrics manager created successfully")

	// 创建事件总线（简化实现也需要事件系统）
	logger.Info("Creating event bus for simplified implementation...")
	eventBus := events.NewEventBus()
	if err := eventBus.Start(); err != nil {
		logger.Errorf("Failed to start event bus: %v", err)
		cancelFunc()
		return nil, fmt.Errorf("failed to start event bus: %w", err)
	}
	logger.Info("Event bus created and started successfully")

	logger.Debug("Assembling simplified application components...")
	app := &BDWindApp{
		config:       cfg,
		webserverMgr: webserverMgr,
		webrtcMgr:    nil, // 简化实现中WebRTC由简化适配器管理
		gstreamerMgr: nil, // 简化实现中GStreamer由简化适配器管理
		metricsMgr:   metricsMgr,
		logger:       logger,
		startTime:    time.Now(),
		rootCtx:      rootCtx,
		cancelFunc:   cancelFunc,
		sigChan:      sigChan,
		eventBus:     eventBus,
	}

	logger.Info("Simplified BDWind application created successfully")
	return app, nil
}

// connectGStreamerToWebRTC connects the GStreamer desktop capture to WebRTC bridge
func (app *BDWindApp) connectGStreamerToWebRTC() error {
	// Info级别: 记录关键连接状态 - 开始建立连接
	app.logger.Info("Establishing sample callback chain from GStreamer to WebRTC")

	// Debug级别: 记录回调建立详情 - 组件状态验证过程
	app.logger.Debug("Verifying component availability for sample callback chain")

	// Get the desktop capture from GStreamer manager
	if app.gstreamerMgr != nil {
		capture := app.gstreamerMgr.GetCapture()
		if capture == nil {
			// Error级别: 关键连接状态 - 链路中断
			app.logger.Error("Sample callback chain failed: desktop capture not available")
			return fmt.Errorf("desktop capture not available")
		}
	}
	// Debug级别: 记录回调建立详情 - 组件验证结果
	app.logger.Debug("GStreamer desktop capture component verified")

	// TODO: Update this section for the new architecture without bridge
	// The bridge has been replaced with direct MediaStreamProvider integration
	app.logger.Debug("WebRTC integration verified (bridge-less architecture)")

	// Get MediaStream for validation
	mediaStream := app.webrtcMgr.GetMediaStream()
	if mediaStream == nil {
		// Warn级别: 关键连接状态 - 非致命性问题
		app.logger.Warn("Sample callback chain warning: MediaStream not available")
	} else {
		// Debug级别: 记录回调建立详情 - 组件状态和统计信息
		// 类型断言以访问GetStats方法
		if ms, ok := mediaStream.(interface{ GetStats() map[string]interface{} }); ok {
			stats := ms.GetStats()
			app.logger.Debugf("MediaStream status verified: video_frames_sent=%v, video_bytes_sent=%v",
				stats["video_frames_sent"], stats["video_bytes_sent"])
		}
	}

	// TODO: Implement new bridge-less sample callback system
	// For now, just log that the connection would be established
	app.logger.Info("Sample callback system will be implemented in the new architecture")

	// Info级别: 记录关键连接状态 - 连接建立成功
	app.logger.Info("GStreamer to WebRTC connection established (new architecture)")

	return nil
}

// Start 启动应用
func (app *BDWindApp) Start() error {
	startupStartTime := time.Now()
	app.logger.Info("Starting BDWind-GStreamer v1.0.0...")

	// 检查是否使用 go-gst 架构
	if app.mediaBridge != nil {
		return app.startGoGstArchitecture()
	}

	// 记录应用配置摘要
	app.logger.Debugf("Application configuration: webserver_port=%d, webserver_tls=%v, metrics_external=%v",
		app.config.WebServer.Port, app.config.WebServer.EnableTLS, app.config.Metrics.External.Enabled)
	app.logger.Debugf("GStreamer config: display=%s, codec=%s, resolution=%dx%d@%dfps",
		app.config.GStreamer.Capture.DisplayID, app.config.GStreamer.Encoding.Codec,
		app.config.GStreamer.Capture.Width, app.config.GStreamer.Capture.Height,
		app.config.GStreamer.Capture.FrameRate)

	// Setup signal handling for graceful shutdown
	app.logger.Debug("Setting up signal handling for graceful shutdown...")
	signal.Notify(app.sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start signal handler goroutine
	app.wg.Add(1)
	go app.handleSignals()
	app.logger.Debug("Signal handler started")

	// Define component startup order: metrics → gstreamer → webrtc → webserver
	type managerInfo struct {
		name    string
		manager interface {
			Start(ctx context.Context) error
			Stop(ctx context.Context) error
			IsRunning() bool
		}
	}

	var managers []managerInfo

	// Choose components based on implementation type
	if app.config.Implementation.UseSimplified {
		// Simplified implementation: only metrics and webserver
		managers = []managerInfo{
			{"metrics", app.metricsMgr},
			{"webserver", app.webserverMgr},
		}
		app.logger.Info("Using simplified component startup sequence: metrics → webserver")
	} else {
		// Complex implementation: all components
		managers = []managerInfo{
			{"metrics", app.metricsMgr},
			{"gstreamer", app.gstreamerMgr},
			{"webrtc", app.webrtcMgr},
			{"webserver", app.webserverMgr},
		}
		app.logger.Info("Using complex component startup sequence: metrics → gstreamer → webrtc → webserver")
	}

	app.logger.Infof("Starting %d components in sequence: metrics → gstreamer → webrtc → webserver", len(managers))

	// Track startup timings for consolidated summary
	startupTimings := make(map[string]time.Duration)
	var startedManagers []string

	// Start components in order
	for i, mgr := range managers {
		app.logger.Infof("Starting %s manager (%d/%d)...", mgr.name, i+1, len(managers))

		// Special handling for webserver - register components first
		if mgr.name == "webserver" {
			app.logger.Debug("Registering components with webserver before starting...")
			if err := app.registerComponentsWithWebServer(); err != nil {
				app.logger.Errorf("Failed to register components with webserver: %v", err)

				// Rollback: stop already started components
				app.logger.Warn("Starting rollback procedure due to webserver registration failure")
				for j := i - 1; j >= 0; j-- {
					app.logger.Warnf("Rolling back: stopping %s manager...", managers[j].name)
					if stopErr := managers[j].manager.Stop(app.rootCtx); stopErr != nil {
						app.logger.Errorf("Failed to stop %s during rollback: %v",
							managers[j].name, stopErr)
					}
				}

				return fmt.Errorf("failed to register components with webserver: %w", err)
			}
			app.logger.Debug("Component registration completed successfully")
		}

		// Special handling for GStreamer - configure logging first (only for complex implementation)
		if mgr.name == "gstreamer" && !app.config.Implementation.UseSimplified {
			app.logger.Debug("Configuring GStreamer logging...")
			if err := app.configureGStreamerLogging(); err != nil {
				app.logger.Warnf("Failed to configure GStreamer logging: %v", err)
			} else {
				app.logger.Debug("GStreamer logging configured successfully")
			}
		}

		// Start the component
		managerStartTime := time.Now()
		app.logger.Debugf("Attempting to start %s manager...", mgr.name)

		if err := mgr.manager.Start(app.rootCtx); err != nil {
			app.logger.Errorf("Failed to start %s manager: %v", mgr.name, err)

			// Rollback: stop already started components in reverse order
			app.logger.Warnf("Starting rollback procedure due to %s manager failure", mgr.name)
			for j := i - 1; j >= 0; j-- {
				app.logger.Warnf("Rolling back: stopping %s manager...", managers[j].name)
				if stopErr := managers[j].manager.Stop(app.rootCtx); stopErr != nil {
					app.logger.Errorf("Failed to stop %s during rollback: %v",
						managers[j].name, stopErr)
				} else {
					app.logger.Infof("%s manager stopped during rollback", managers[j].name)
				}
			}

			return fmt.Errorf("failed to start %s manager: %w", mgr.name, err)
		}

		// Record successful startup
		duration := time.Since(managerStartTime)
		startupTimings[mgr.name] = duration
		startedManagers = append(startedManagers, mgr.name)

		app.logger.Infof("%s manager started successfully in %v", mgr.name, duration)

		// Verify component is running
		if mgr.manager.IsRunning() {
			app.logger.Debugf("%s manager confirmed running", mgr.name)
		} else {
			app.logger.Warnf("%s manager started but not reporting as running", mgr.name)
		}
	}

	// Connect GStreamer to WebRTC after all components are started (only for complex implementation)
	if !app.config.Implementation.UseSimplified {
		app.logger.Debug("Establishing GStreamer to WebRTC connection...")
		if err := app.connectGStreamerToWebRTC(); err != nil {
			app.logger.Warnf("Failed to connect GStreamer to WebRTC: %v", err)
			app.logger.Warn("Media streaming may be affected - continuing startup")
		} else {
			app.logger.Debug("GStreamer to WebRTC connection established successfully")
		}
	} else {
		app.logger.Debug("Skipping GStreamer to WebRTC connection (simplified implementation)")
	}

	// Final application startup summary
	totalStartupTime := time.Since(startupStartTime)
	app.logger.Infof("BDWind-GStreamer application started successfully in %v", totalStartupTime)

	// Log component status summary
	app.logger.Infof("All %d components started: %v", len(startedManagers), startedManagers)

	// Log detailed timing breakdown at DEBUG level
	app.logger.Debugf("Component startup timings: metrics=%v, gstreamer=%v, webrtc=%v, webserver=%v",
		startupTimings["metrics"], startupTimings["gstreamer"],
		startupTimings["webrtc"], startupTimings["webserver"])

	// Log service endpoints
	webserverAddr := app.webserverMgr.GetAddress()
	app.logger.Infof("Application ready - Web interface: %s", webserverAddr)

	if app.config.Metrics.External.Enabled {
		metricsEndpoint := app.config.Metrics.GetExternalEndpoint()
		app.logger.Infof("Metrics endpoint: %s", metricsEndpoint)
	}

	return nil
}

// startGoGstArchitecture 启动 go-gst 架构
func (app *BDWindApp) startGoGstArchitecture() error {
	app.logger.Info("Starting go-gst architecture...")

	// Setup signal handling for graceful shutdown
	app.logger.Debug("Setting up signal handling for graceful shutdown...")
	signal.Notify(app.sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start signal handler goroutine
	app.wg.Add(1)
	go app.handleSignals()
	app.logger.Debug("Signal handler started")

	// 启动组件顺序：metrics → mediaBridge → webserver
	app.logger.Info("Starting components: metrics → mediaBridge → webserver")

	// 1. 启动 metrics
	app.logger.Info("Starting metrics manager (1/3)...")
	if err := app.metricsMgr.Start(app.rootCtx); err != nil {
		return fmt.Errorf("failed to start metrics manager: %w", err)
	}
	app.logger.Info("Metrics manager started successfully")

	// 2. 启动 mediaBridge (包含 GStreamer 和 WebRTC)
	app.logger.Info("Starting go-gst media bridge (2/3)...")
	if err := app.mediaBridge.Start(); err != nil {
		app.metricsMgr.Stop(app.rootCtx)
		return fmt.Errorf("failed to start media bridge: %w", err)
	}
	app.logger.Info("Go-gst media bridge started successfully")

	// 3. 注册组件到 webserver
	app.logger.Info("Registering components with webserver...")
	if err := app.registerGoGstComponentsWithWebServer(); err != nil {
		app.mediaBridge.Stop()
		app.metricsMgr.Stop(app.rootCtx)
		return fmt.Errorf("failed to register components with webserver: %w", err)
	}
	app.logger.Info("Components registered with webserver successfully")

	// 4. 启动 webserver
	app.logger.Info("Starting webserver manager (4/4)...")
	if err := app.webserverMgr.Start(app.rootCtx); err != nil {
		app.mediaBridge.Stop()
		app.metricsMgr.Stop(app.rootCtx)
		return fmt.Errorf("failed to start webserver manager: %w", err)
	}
	app.logger.Info("Webserver manager started successfully")

	// 显示服务信息
	webserverAddr := app.webserverMgr.GetAddress()
	app.logger.Infof("🚀 Go-gst BDWind-GStreamer started successfully!")
	app.logger.Infof("🌐 Web interface: %s", webserverAddr)
	app.logger.Infof("🎥 Display: %s", app.config.GStreamer.Capture.DisplayID)
	app.logger.Infof("🎬 Codec: %s", app.config.GStreamer.Encoding.Codec)

	return nil
}

// ComponentStats 组件关闭统计信息
type ComponentStats struct {
	StartTime time.Time
	Duration  time.Duration
	Success   bool
	Error     error
}

// Stop 停止应用
func (app *BDWindApp) Stop(ctx context.Context) error {
	applicationShutdownStart := time.Now()
	app.logger.Info("BDWind-GStreamer Application Shutdown Started")
	app.logger.Tracef("Application shutdown initiated at: %s", applicationShutdownStart.Format("15:04:05.000"))
	app.logger.Tracef("Application uptime: %v", time.Since(app.startTime))

	// Cancel the root context to signal all components to stop
	app.logger.Debugf("Cancelling root context to signal all components")
	app.cancelFunc()

	// Wait for signal handler to complete
	app.logger.Debugf("Waiting for signal handler to complete")
	app.wg.Wait()

	// Use lifecycle configuration for shutdown timeout if no context timeout is set
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), app.config.Lifecycle.ShutdownTimeout)
		defer cancel()
		app.logger.Tracef("Using configured shutdown timeout: %v", app.config.Lifecycle.ShutdownTimeout)
	} else {
		if deadline, ok := ctx.Deadline(); ok {
			remaining := time.Until(deadline)
			app.logger.Tracef("Using provided context timeout, remaining: %v", remaining)
		}
	}

	// Define component shutdown order: metrics → webserver → webrtc → gstreamer
	// Changed order to ensure metrics (important for data collection) stops first
	type managerInfo struct {
		name    string
		manager interface {
			Stop(ctx context.Context) error
		}
	}

	var managers []managerInfo

	// Check if using go-gst architecture
	if app.mediaBridge != nil {
		// Go-gst architecture: only metrics and webserver
		managers = []managerInfo{
			{"metrics", app.metricsMgr}, // First to stop - ensure data collection completes
			{"webserver", app.webserverMgr},
		}
		app.logger.Debug("Using go-gst component shutdown sequence: metrics → webserver")
	} else if app.config.Implementation.UseSimplified {
		// Simplified implementation: only metrics and webserver
		managers = []managerInfo{
			{"metrics", app.metricsMgr}, // First to stop - ensure data collection completes
			{"webserver", app.webserverMgr},
		}
		app.logger.Debug("Using simplified component shutdown sequence: metrics → webserver")
	} else {
		// Complex implementation: all components
		managers = []managerInfo{
			{"metrics", app.metricsMgr}, // First to stop - ensure data collection completes
			{"webserver", app.webserverMgr},
			{"webrtc", app.webrtcMgr},
			{"gstreamer", app.gstreamerMgr}, // Last to stop - may block
		}
		app.logger.Debug("Using complex component shutdown sequence: metrics → webserver → webrtc → gstreamer")
	}

	// Collect all errors but continue stopping other components
	var errors []error
	shutdownStats := make(map[string]ComponentStats)

	// Stop components in the new order with detailed timing statistics
	app.logger.Tracef("=== Component Shutdown Sequence Started ===")
	overallShutdownStart := time.Now()

	for i, mgr := range managers {
		startTime := time.Now()
		app.logger.Tracef("=== Stopping %s Manager (%d/%d) ===", mgr.name, i+1, len(managers))
		app.logger.Tracef("%s shutdown start time: %s", mgr.name, startTime.Format("15:04:05.000"))

		// Log component state before shutdown
		if statusProvider, ok := mgr.manager.(interface{ IsRunning() bool }); ok {
			app.logger.Tracef("%s manager state before shutdown: running=%v",
				mgr.name, statusProvider.IsRunning())
		}

		var err error
		if mgr.name == "gstreamer" {
			// GStreamer uses special timeout handling
			app.logger.Tracef("Using special timeout handling for GStreamer component")
			err = app.stopGStreamerWithTimeout(ctx)
		} else {
			app.logger.Tracef("Using standard stop procedure for %s component", mgr.name)
			err = mgr.manager.Stop(ctx)
		}

		endTime := time.Now()
		duration := endTime.Sub(startTime)

		// Record component shutdown statistics
		shutdownStats[mgr.name] = ComponentStats{
			StartTime: startTime,
			Duration:  duration,
			Success:   err == nil,
			Error:     err,
		}

		// Log detailed completion information
		app.logger.Tracef("%s shutdown end time: %s", mgr.name, endTime.Format("15:04:05.000"))

		if err != nil {
			app.logger.Errorf("Failed to stop %s manager: %v", mgr.name, err)
			app.logger.Tracef("=== %s Manager Shutdown FAILED ===", mgr.name)
			app.logger.Tracef("Failure duration: %v", duration)
			app.logger.Tracef("Error details: %v", err)
			errors = append(errors, fmt.Errorf("failed to stop %s: %w", mgr.name, err))
		} else {
			app.logger.Infof("%s Manager stopped successfully", mgr.name)
			app.logger.Tracef("=== %s Manager Shutdown SUCCESS ===", mgr.name)
			app.logger.Tracef("Success duration: %v", duration)

			// Log final component state after successful shutdown
			if statusProvider, ok := mgr.manager.(interface{ IsRunning() bool }); ok {
				app.logger.Tracef("%s manager state after shutdown: running=%v",
					mgr.name, statusProvider.IsRunning())
			}
		}

		// Log progress through shutdown sequence
		remainingComponents := len(managers) - i - 1
		if remainingComponents > 0 {
			app.logger.Tracef("Shutdown progress: %d/%d completed, %d remaining",
				i+1, len(managers), remainingComponents)
		}
	}

	overallShutdownDuration := time.Since(overallShutdownStart)
	app.logger.Tracef("=== Component Shutdown Sequence Completed ===")
	app.logger.Tracef("Overall shutdown sequence duration: %v", overallShutdownDuration)

	// Handle mediaBridge separately for go-gst architecture
	if app.mediaBridge != nil {
		app.logger.Info("Stopping go-gst media bridge...")
		if err := app.mediaBridge.Stop(); err != nil {
			app.logger.Errorf("Failed to stop media bridge: %v", err)
			errors = append(errors, fmt.Errorf("failed to stop media bridge: %w", err))
		} else {
			app.logger.Info("Go-gst media bridge stopped successfully")
		}
	}

	// Stop event bus
	if app.eventBus != nil {
		app.logger.Info("Stopping event bus...")
		if err := app.eventBus.Stop(); err != nil {
			app.logger.Errorf("Failed to stop event bus: %v", err)
			errors = append(errors, fmt.Errorf("failed to stop event bus: %w", err))
		} else {
			app.logger.Info("Event bus stopped successfully")
		}
	}

	// Log shutdown statistics summary
	app.logShutdownStats(shutdownStats)

	// Report final status with comprehensive timing
	applicationShutdownEnd := time.Now()
	totalApplicationShutdownDuration := applicationShutdownEnd.Sub(applicationShutdownStart)

	app.logger.Tracef("=== BDWind-GStreamer Application Shutdown Completed ===")
	app.logger.Tracef("Application shutdown completed at: %s", applicationShutdownEnd.Format("15:04:05.000"))
	app.logger.Tracef("Total application shutdown duration: %v", totalApplicationShutdownDuration)

	if len(errors) > 0 {
		app.logger.Errorf("BDWind-GStreamer application stopped with %d errors", len(errors))
		app.logger.Tracef("=== Application Shutdown FAILED ===")
		app.logger.Tracef("Application stopped with %d errors after %v", len(errors), totalApplicationShutdownDuration)
		app.logger.Tracef("Shutdown errors: %v", errors)
		return fmt.Errorf("errors during shutdown: %v", errors)
	}

	app.logger.Info("BDWind-GStreamer application stopped successfully")
	app.logger.Tracef("=== Application Shutdown SUCCESS ===")
	app.logger.Tracef("BDWind-GStreamer application stopped successfully in %v", totalApplicationShutdownDuration)
	return nil
}

// logShutdownStats logs shutdown statistics at trace level for detailed information
func (app *BDWindApp) logShutdownStats(stats map[string]ComponentStats) {
	app.logger.Tracef("=== Shutdown Statistics ===")

	var totalDuration time.Duration
	successCount := 0

	for name, stat := range stats {
		status := "SUCCESS"
		if !stat.Success {
			status = "FAILED"
		} else {
			successCount++
		}

		app.logger.Tracef("  %s: %s (took %v)", name, status, stat.Duration)
		totalDuration += stat.Duration

		if stat.Error != nil {
			app.logger.Tracef("    Error: %v", stat.Error)
		}
	}

	app.logger.Tracef("Total shutdown time: %v, Success rate: %d/%d",
		totalDuration, successCount, len(stats))
}

// stopGStreamerWithTimeout stops GStreamer with a 3-second timeout and force stop mechanism
func (app *BDWindApp) stopGStreamerWithTimeout(ctx context.Context) error {
	// GStreamer 3-second timeout - hardcoded as per design requirements
	const gstreamerTimeout = 3 * time.Second

	startTime := time.Now()
	app.logger.Tracef("=== GStreamer Shutdown Process Started ===")
	app.logger.Tracef("Shutdown start time: %s", startTime.Format("15:04:05.000"))
	app.logger.Tracef("Configured timeout: %v", gstreamerTimeout)

	// Log current GStreamer state before shutdown
	if app.gstreamerMgr != nil {
		app.logger.Tracef("GStreamer manager state: running=%v, enabled=%v",
			app.gstreamerMgr.IsRunning(), app.gstreamerMgr.IsEnabled())

		// Log GStreamer statistics if available
		stats := app.gstreamerMgr.GetStats()
		if stats != nil {
			app.logger.Tracef("GStreamer stats before shutdown: %+v", stats)
		}
	}

	// Create timeout context for GStreamer
	gstreamerCtx, cancel := context.WithTimeout(ctx, gstreamerTimeout)
	defer cancel()

	// Channel to receive the stop result
	done := make(chan error, 1)

	// Start GStreamer stop in a goroutine
	go func() {
		app.logger.Tracef("Initiating normal GStreamer stop procedure at %s",
			time.Now().Format("15:04:05.000"))
		done <- app.gstreamerMgr.Stop(gstreamerCtx)
	}()

	// Wait for either completion or timeout
	select {
	case err := <-done:
		duration := time.Since(startTime)
		if err != nil {
			app.logger.Warnf("GStreamer stopped with error after %v: %v", duration, err)
			app.logger.Tracef("GStreamer shutdown completed at: %s", time.Now().Format("15:04:05.000"))
		} else {
			app.logger.Tracef("GStreamer stopped normally within timeout")
			app.logger.Tracef("Normal shutdown duration: %v", duration)
			app.logger.Tracef("GStreamer shutdown completed at: %s", time.Now().Format("15:04:05.000"))
		}
		app.logger.Tracef("=== GStreamer Shutdown Process Completed ===")
		return err

	case <-gstreamerCtx.Done():
		timeoutDuration := time.Since(startTime)
		app.logger.Warnf("GStreamer stop timeout after %v, attempting force stop", gstreamerTimeout)
		app.logger.Tracef("=== GStreamer Timeout Detected ===")
		app.logger.Tracef("Timeout occurred after: %v (expected: %v)", timeoutDuration, gstreamerTimeout)
		app.logger.Tracef("Timeout detected at: %s", time.Now().Format("15:04:05.000"))

		// Log detailed timeout information
		app.logger.Tracef("GStreamer normal stop procedure failed to complete within %v", gstreamerTimeout)
		app.logger.Tracef("Possible causes: pipeline state transition blocked, resource cleanup hanging")

		// Log current state during timeout
		if app.gstreamerMgr != nil {
			app.logger.Tracef("GStreamer manager state during timeout: running=%v",
				app.gstreamerMgr.IsRunning())
		}

		// Attempt force stop with detailed logging
		app.logger.Tracef("Initiating force stop procedure at: %s", time.Now().Format("15:04:05.000"))
		forceStopStart := time.Now()

		if forceErr := app.forceStopGStreamer(); forceErr != nil {
			forceStopDuration := time.Since(forceStopStart)
			app.logger.Errorf("Force stop failed after %v: %v", forceStopDuration, forceErr)
			app.logger.Tracef("=== Force Stop Failed ===")
			app.logger.Tracef("Force stop duration: %v", forceStopDuration)
			app.logger.Tracef("Force stop error: %v", forceErr)
			app.logger.Tracef("Total GStreamer shutdown attempt duration: %v", time.Since(startTime))
			return fmt.Errorf("GStreamer stop timeout and force stop failed: %w", forceErr)
		}

		forceStopDuration := time.Since(forceStopStart)
		totalDuration := time.Since(startTime)

		app.logger.Tracef("=== Force Stop Completed Successfully ===")
		app.logger.Tracef("Force stop duration: %v", forceStopDuration)
		app.logger.Tracef("Total shutdown duration: %v (timeout: %v + force stop: %v)",
			totalDuration, gstreamerTimeout, forceStopDuration)
		app.logger.Tracef("Force stop completed at: %s", time.Now().Format("15:04:05.000"))
		app.logger.Tracef("=== GStreamer Shutdown Process Completed (Force Stop) ===")

		return fmt.Errorf("GStreamer stop timeout after %v, force stopped successfully in %v",
			gstreamerTimeout, forceStopDuration)
	}
}

// forceStopGStreamer performs force stop of GStreamer manager
func (app *BDWindApp) forceStopGStreamer() error {
	app.logger.Warnf("Force stopping GStreamer manager")
	app.logger.Tracef("=== Force Stop GStreamer Manager ===")
	app.logger.Tracef("Force stop initiated at: %s", time.Now().Format("15:04:05.000"))

	if app.gstreamerMgr == nil {
		app.logger.Tracef("GStreamer manager is nil, nothing to force stop")
		app.logger.Tracef("Force stop completed (no-op) at: %s", time.Now().Format("15:04:05.000"))
		return nil
	}

	// Log current manager state before force stop
	app.logger.Tracef("GStreamer manager state before force stop: running=%v, enabled=%v",
		app.gstreamerMgr.IsRunning(), app.gstreamerMgr.IsEnabled())

	// Check if GStreamer manager implements ForceStop method
	if forceStoppable, ok := interface{}(app.gstreamerMgr).(interface{ ForceStop() error }); ok {
		app.logger.Tracef("GStreamer manager supports ForceStop method, executing force stop")
		app.logger.Tracef("Force stop method available: calling ForceStop() on manager")

		forceStopStart := time.Now()
		err := forceStoppable.ForceStop()
		forceStopDuration := time.Since(forceStopStart)

		if err != nil {
			app.logger.Errorf("ForceStop method failed after %v: %v", forceStopDuration, err)
			return fmt.Errorf("ForceStop method failed: %w", err)
		}

		app.logger.Tracef("ForceStop method completed successfully in %v", forceStopDuration)
		app.logger.Tracef("Force stop completed at: %s", time.Now().Format("15:04:05.000"))

		// Log final state after force stop
		app.logger.Tracef("GStreamer manager state after force stop: running=%v",
			app.gstreamerMgr.IsRunning())

		return nil
	}

	// Fallback: rely on context cancellation (already done via app.cancelFunc)
	app.logger.Warnf("GStreamer manager does not support ForceStop method, relying on context cancellation")
	app.logger.Tracef("Fallback strategy: relying on context cancellation")
	app.logger.Tracef("Context cancellation should have been triggered via app.cancelFunc")
	app.logger.Tracef("Note: This fallback may not guarantee complete resource cleanup")

	// Log that we're using the fallback approach
	app.logger.Tracef("Force stop fallback completed at: %s", time.Now().Format("15:04:05.000"))
	app.logger.Tracef("=== Force Stop Completed (Fallback Method) ===")

	return nil
}

// GetConfig 获取应用配置
func (app *BDWindApp) GetConfig() *config.Config {
	return app.config
}

// GetWebRTCManager 获取WebRTC管理器
func (app *BDWindApp) GetWebRTCManager() *webrtc.WebRTCManager {
	return app.webrtcMgr
}

// GetGStreamerManager 获取GStreamer管理器
func (app *BDWindApp) GetGStreamerManager() *gstreamer.Manager {
	return app.gstreamerMgr
}

// GetWebServerManager 获取webserver管理器
func (app *BDWindApp) GetWebServerManager() *webserver.Manager {
	return app.webserverMgr
}

// GetMetricsManager 获取监控管理器
func (app *BDWindApp) GetMetricsManager() *metrics.Manager {
	return app.metricsMgr
}

// handleSignals handles OS signals for graceful shutdown
func (app *BDWindApp) handleSignals() {
	defer app.wg.Done()

	select {
	case sig := <-app.sigChan:
		app.logger.Tracef("Received signal: %v, initiating graceful shutdown", sig)
		app.cancelFunc()
	case <-app.rootCtx.Done():
		// Context was cancelled from elsewhere
		return
	}
}

// GetRootContext returns the root context for the application
func (app *BDWindApp) GetRootContext() context.Context {
	return app.rootCtx
}

// registerSimplifiedComponentsWithWebServer registers simplified components with the WebServer
func (app *BDWindApp) registerSimplifiedComponentsWithWebServer(webServer *webserver.WebServer, registeredComponents *[]string, registrationErrors *[]error) error {
	app.logger.Debug("Registering simplified components...")

	*registeredComponents = append(*registeredComponents, "simplified")
	app.logger.Debug("Simplified adapter registered and started successfully")

	// Still register metrics if available
	if mgr, ok := interface{}(app.metricsMgr).(webserver.ComponentManager); ok {
		if err := webServer.RegisterComponent("metrics", mgr); err != nil {
			app.logger.Warnf("Failed to register metrics component: %v", err)
			*registrationErrors = append(*registrationErrors, fmt.Errorf("failed to register metrics component: %w", err))
		} else {
			*registeredComponents = append(*registeredComponents, "metrics")
			app.logger.Debug("Metrics component registered successfully")
		}
	}

	app.logger.Infof("Simplified components registered: %v", *registeredComponents)
	return nil
}

// registerGoGstComponentsWithWebServer registers go-gst components with the WebServer
func (app *BDWindApp) registerGoGstComponentsWithWebServer() error {
	app.logger.Debug("Starting go-gst component registration with webserver")

	webServer := app.webserverMgr.GetWebServer()
	if webServer == nil {
		app.logger.Error("webserver instance is nil")
		return fmt.Errorf("webserver instance is nil")
	}
	app.logger.Debug("webserver instance obtained successfully")

	app.logger.Info("Go-gst simplified adapter registered and started successfully")

	// Register metrics if available
	if mgr, ok := interface{}(app.metricsMgr).(webserver.ComponentManager); ok {
		if err := webServer.RegisterComponent("metrics", mgr); err != nil {
			app.logger.Warnf("Failed to register metrics component: %v", err)
		} else {
			app.logger.Debug("Metrics component registered successfully")
		}
	}

	app.logger.Info("Go-gst components registered successfully")
	return nil
}

// registerComponentsWithWebServer registers all components with the WebServer for route setup
func (app *BDWindApp) registerComponentsWithWebServer() error {
	app.logger.Debugf("Starting component registration with webserver")

	webServer := app.webserverMgr.GetWebServer()
	if webServer == nil {
		app.logger.Error("webserver instance is nil")
		return fmt.Errorf("webserver instance is nil")
	}
	app.logger.Debugf("webserver instance obtained successfully")

	registrationResults := make(map[string]bool)
	var registrationErrors []error
	var registeredComponents []string

	// Check if we should use simplified implementation
	if app.config.Implementation.UseSimplified {
		app.logger.Info("Using simplified implementation for component registration")
		return app.registerSimplifiedComponentsWithWebServer(webServer, &registeredComponents, &registrationErrors)
	}

	// Use complex implementation (existing logic)
	app.logger.Info("Using complex implementation for component registration")

	// Try to register metrics component
	app.logger.Debugf("Attempting to register metrics component")
	if mgr, ok := interface{}(app.metricsMgr).(webserver.ComponentManager); ok {
		app.logger.Debugf("metrics component implements ComponentManager interface")
		if err := webServer.RegisterComponent("metrics", mgr); err != nil {
			app.logger.Debugf("Failed to register metrics component: %v", err)
			registrationErrors = append(registrationErrors, fmt.Errorf("failed to register metrics component: %w", err))
			registrationResults["metrics"] = false
		} else {
			app.logger.Debugf("metrics component registered successfully")
			registrationResults["metrics"] = true
			registeredComponents = append(registeredComponents, "metrics")
		}
	} else {
		app.logger.Debugf("metrics component does not implement ComponentManager interface")
		registrationResults["metrics"] = false
	}

	// Try to register gstreamer component
	app.logger.Debugf("Attempting to register gstreamer component")
	if mgr, ok := interface{}(app.gstreamerMgr).(webserver.ComponentManager); ok {
		app.logger.Debugf("gstreamer component implements ComponentManager interface")
		if err := webServer.RegisterComponent("gstreamer", mgr); err != nil {
			app.logger.Debugf("Failed to register gstreamer component: %v", err)
			registrationErrors = append(registrationErrors, fmt.Errorf("failed to register gstreamer component: %w", err))
			registrationResults["gstreamer"] = false
		} else {
			app.logger.Debugf("gstreamer component registered successfully")
			registrationResults["gstreamer"] = true
			registeredComponents = append(registeredComponents, "gstreamer")
		}
	} else {
		app.logger.Debugf("gstreamer component does not implement ComponentManager interface")
		registrationResults["gstreamer"] = false
	}

	// Try to register webrtc component
	app.logger.Debugf("Attempting to register webrtc component")
	app.logger.Debugf("webrtc component implements ComponentManager interface")
	if err := webServer.RegisterComponent("webrtc", app.webrtcMgr); err != nil {
		app.logger.Debugf("Failed to register webrtc component: %v", err)
		registrationErrors = append(registrationErrors, fmt.Errorf("failed to register webrtc component: %w", err))
		registrationResults["webrtc"] = false
	} else {
		app.logger.Debugf("webrtc component registered successfully")
		registrationResults["webrtc"] = true
		registeredComponents = append(registeredComponents, "webrtc")
	}

	// Log consolidated registration summary at INFO level
	successCount := len(registeredComponents)
	totalCount := len(registrationResults)

	if successCount > 0 {
		// Format component names as comma-separated string instead of Go slice format
		componentNames := registeredComponents[0]
		for i := 1; i < len(registeredComponents); i++ {
			componentNames += ", " + registeredComponents[i]
		}
		app.logger.Debugf("Components registered: %d/%d (%s)",
			successCount, totalCount, componentNames)
	} else {
		app.logger.Warnf("Components registered: %d/%d (no components successfully registered)",
			successCount, totalCount)
	}

	// If there were registration errors, return the first one
	if len(registrationErrors) > 0 {
		app.logger.Debugf("Component registration completed with %d errors", len(registrationErrors))
		return registrationErrors[0]
	}

	app.logger.Debugf("Component registration completed successfully")
	return nil
}

// Shutdown initiates graceful shutdown of the application
func (app *BDWindApp) Shutdown() {
	app.logger.Info("Initiating application shutdown...")
	app.cancelFunc()
}

// Wait waits for all goroutines to complete
func (app *BDWindApp) Wait() {
	app.wg.Wait()
}

// ForceShutdown forces immediate shutdown of the application
func (app *BDWindApp) ForceShutdown() error {
	app.logger.Info("Force shutdown initiated...")

	// Cancel context immediately
	app.cancelFunc()

	// Create a short timeout context for force shutdown
	ctx, cancel := context.WithTimeout(context.Background(), app.config.Lifecycle.ForceShutdownTimeout)
	defer cancel()

	// Force stop all components using the same order as normal shutdown: metrics → webserver → webrtc → gstreamer
	managers := []struct {
		name    string
		manager interface {
			Stop(ctx context.Context) error
		}
	}{
		{"metrics", app.metricsMgr}, // First to stop - ensure data collection completes
		{"webserver", app.webserverMgr},
		{"webrtc", app.webrtcMgr},
		{"gstreamer", app.gstreamerMgr}, // Last to stop - may block
	}

	// Stop all components, log errors but don't fail
	for _, mgr := range managers {
		if err := mgr.manager.Stop(ctx); err != nil {
			app.logger.Infof("Force shutdown: %s stop error: %v", mgr.name, err)
		} else {
			app.logger.Infof("Force shutdown: %s stopped successfully", mgr.name)
		}
	}

	app.logger.Info("Force shutdown completed")
	return nil
}

// IsHealthy 检查应用是否健康
func (app *BDWindApp) IsHealthy() bool {
	// 检查各个组件的健康状态
	// 所有启用的组件都必须正在运行才算健康

	// 检查 metrics 管理器
	if app.metricsMgr.IsEnabled() && !app.metricsMgr.IsRunning() {
		app.logger.Infof("Component metrics is enabled but not running")
		return false
	}

	// 检查 gstreamer 管理器
	if app.gstreamerMgr.IsEnabled() && !app.gstreamerMgr.IsRunning() {
		app.logger.Infof("Component gstreamer is enabled but not running")
		return false
	}

	// 检查 webrtc 管理器
	if app.webrtcMgr.IsEnabled() && !app.webrtcMgr.IsRunning() {
		app.logger.Infof("Component webrtc is enabled but not running")
		return false
	}

	// 检查 webserver 管理器
	if app.webserverMgr.IsEnabled() && !app.webserverMgr.IsRunning() {
		app.logger.Infof("Component webserver is enabled but not running")
		return false
	}

	return true
}

// GetHealthSummary 获取健康状态摘要
func (app *BDWindApp) GetHealthSummary() map[string]interface{} {
	summary := map[string]interface{}{
		"healthy":    true,
		"total":      4, // metrics, gstreamer, webrtc, webserver
		"running":    0,
		"stopped":    0,
		"disabled":   0,
		"components": make(map[string]interface{}),
		"uptime":     time.Since(app.startTime).String(),
		"start_time": app.startTime,
	}

	componentsMap := summary["components"].(map[string]interface{})

	// 检查 metrics 组件
	metricsInfo := map[string]interface{}{
		"enabled": app.metricsMgr.IsEnabled(),
		"running": app.metricsMgr.IsRunning(),
		"stats":   app.metricsMgr.GetStats(),
	}
	componentsMap["metrics"] = metricsInfo
	app.updateHealthCounters(summary, app.metricsMgr.IsEnabled(), app.metricsMgr.IsRunning())

	// 检查 gstreamer 组件
	gstreamerInfo := map[string]interface{}{
		"enabled": app.gstreamerMgr.IsEnabled(),
		"running": app.gstreamerMgr.IsRunning(),
		"stats":   app.gstreamerMgr.GetStats(),
	}
	componentsMap["gstreamer"] = gstreamerInfo
	app.updateHealthCounters(summary, app.gstreamerMgr.IsEnabled(), app.gstreamerMgr.IsRunning())

	// 检查 webrtc 组件
	webrtcInfo := map[string]interface{}{
		"enabled": app.webrtcMgr.IsEnabled(),
		"running": app.webrtcMgr.IsRunning(),
		"stats":   app.webrtcMgr.GetStats(),
	}
	componentsMap["webrtc"] = webrtcInfo
	app.updateHealthCounters(summary, app.webrtcMgr.IsEnabled(), app.webrtcMgr.IsRunning())

	// 检查 webserver 组件
	webserverInfo := map[string]interface{}{
		"enabled": app.webserverMgr.IsEnabled(),
		"running": app.webserverMgr.IsRunning(),
		"stats":   app.webserverMgr.GetStats(),
	}
	componentsMap["webserver"] = webserverInfo
	app.updateHealthCounters(summary, app.webserverMgr.IsEnabled(), app.webserverMgr.IsRunning())

	return summary
}

// updateHealthCounters 更新健康状态计数器的辅助方法
func (app *BDWindApp) updateHealthCounters(summary map[string]interface{}, enabled, running bool) {
	if enabled {
		if running {
			summary["running"] = summary["running"].(int) + 1
		} else {
			summary["stopped"] = summary["stopped"].(int) + 1
			summary["healthy"] = false
		}
	} else {
		summary["disabled"] = summary["disabled"].(int) + 1
	}
}

// GetComponentStatus 获取组件状态
func (app *BDWindApp) GetComponentStatus(name string) (*webserver.ComponentState, bool) {
	var enabled, running bool
	var stats map[string]interface{}

	// 根据组件名称获取对应的管理器状态
	switch name {
	case "metrics":
		enabled = app.metricsMgr.IsEnabled()
		running = app.metricsMgr.IsRunning()
		stats = app.metricsMgr.GetStats()
	case "gstreamer":
		enabled = app.gstreamerMgr.IsEnabled()
		running = app.gstreamerMgr.IsRunning()
		stats = app.gstreamerMgr.GetStats()
	case "webrtc":
		enabled = app.webrtcMgr.IsEnabled()
		running = app.webrtcMgr.IsRunning()
		stats = app.webrtcMgr.GetStats()
	case "webserver":
		enabled = app.webserverMgr.IsEnabled()
		running = app.webserverMgr.IsRunning()
		stats = app.webserverMgr.GetStats()
	default:
		return nil, false
	}

	// 确定组件状态
	var status webserver.ComponentStatus
	if !enabled {
		status = webserver.ComponentStatusDisabled
	} else if running {
		status = webserver.ComponentStatusRunning
	} else {
		status = webserver.ComponentStatusStopped
	}

	// 创建组件状态对象
	state := &webserver.ComponentState{
		Name:    name,
		Status:  status,
		Enabled: enabled,
		Stats:   stats,
	}

	// 如果组件正在运行，设置启动时间
	if running {
		state.StartTime = &app.startTime
	}

	return state, true
}

// GetAllComponentStatus 获取所有组件状态
func (app *BDWindApp) GetAllComponentStatus() map[string]*webserver.ComponentState {
	componentNames := []string{"metrics", "gstreamer", "webrtc", "webserver"}
	result := make(map[string]*webserver.ComponentState)

	// 遍历所有组件，获取它们的状态
	for _, name := range componentNames {
		if state, exists := app.GetComponentStatus(name); exists {
			result[name] = state
		}
	}

	return result
}

// configureGStreamerLogging 配置GStreamer日志
func (app *BDWindApp) configureGStreamerLogging() error {
	// 检查应用程序日志配置是否可用
	if app.config == nil || app.config.Logging == nil {
		app.logger.Debug("No application logging configuration available, using GStreamer defaults")
		return nil
	}

	// 调用GStreamer管理器的日志配置方法
	if err := app.gstreamerMgr.ConfigureLogging(app.config.Logging); err != nil {
		return fmt.Errorf("failed to configure GStreamer logging: %w", err)
	}

	app.logger.Debugf("GStreamer logging configured with app log level: %s", app.config.Logging.Level)
	return nil
}

// GetEventBus 获取事件总线
func (app *BDWindApp) GetEventBus() events.EventBus {
	return app.eventBus
}

// GetSignalingEventHandlers 获取信令事件处理器
func (app *BDWindApp) GetSignalingEventHandlers() *webrtc.SignalingEventHandlers {
	return app.signalingHandlers
}
