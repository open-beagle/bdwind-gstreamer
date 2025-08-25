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

	"github.com/open-beagle/bdwind-gstreamer/internal/config"
	"github.com/open-beagle/bdwind-gstreamer/internal/gstreamer"
	"github.com/open-beagle/bdwind-gstreamer/internal/metrics"
	"github.com/open-beagle/bdwind-gstreamer/internal/webrtc"
	"github.com/open-beagle/bdwind-gstreamer/internal/webserver"
)

// BDWindApp BDWind应用
type BDWindApp struct {
	config       *config.Config
	webserverMgr *webserver.Manager
	webrtcMgr    *webrtc.Manager
	gstreamerMgr *gstreamer.Manager
	metricsMgr   *metrics.Manager
	logger       *logrus.Entry
	startTime    time.Time

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

	// Create root context for lifecycle management
	rootCtx, cancelFunc := context.WithCancel(context.Background())

	// Create signal channel for graceful shutdown
	sigChan := make(chan os.Signal, 1)

	// 创建WebRTC管理器
	webrtcMgrConfig := &webrtc.ManagerConfig{
		Config: cfg,
	}

	webrtcMgr, err := webrtc.NewManager(rootCtx, webrtcMgrConfig)
	if err != nil {
		cancelFunc() // Clean up context on error
		return nil, fmt.Errorf("failed to create WebRTC manager: %w", err)
	}

	// 创建GStreamer管理器
	gstreamerMgrConfig := &gstreamer.ManagerConfig{
		Config: cfg.GStreamer,
	}

	gstreamerMgr, err := gstreamer.NewManager(rootCtx, gstreamerMgrConfig)
	if err != nil {
		cancelFunc() // Clean up context on error
		return nil, fmt.Errorf("failed to create GStreamer manager: %w", err)
	}

	// 创建webserver管理器（包含认证功能）
	webserverMgr, err := webserver.NewManager(rootCtx, cfg.WebServer, config.GetStandardLoggerWithPrefix("webserver"))
	if err != nil {
		cancelFunc() // Clean up context on error
		return nil, fmt.Errorf("failed to create webserver manager: %w", err)
	}

	// 创建监控管理器
	metricsMgr, err := metrics.NewManager(rootCtx, cfg.Metrics, config.GetStandardLoggerWithPrefix("metrics"))
	if err != nil {
		cancelFunc() // Clean up context on error
		return nil, fmt.Errorf("failed to create metrics manager: %w", err)
	}

	app := &BDWindApp{
		config:       cfg,
		webserverMgr: webserverMgr,
		webrtcMgr:    webrtcMgr,
		gstreamerMgr: gstreamerMgr,
		metricsMgr:   metricsMgr,
		logger:       logger,
		startTime:    time.Now(),
		rootCtx:      rootCtx,
		cancelFunc:   cancelFunc,
		sigChan:      sigChan,
	}

	return app, nil
}

// connectGStreamerToWebRTC connects the GStreamer desktop capture to WebRTC bridge
func (app *BDWindApp) connectGStreamerToWebRTC() error {
	// Info级别: 记录样本回调链路建立成功/失败状态
	app.logger.Info("Establishing sample callback chain from GStreamer to WebRTC")

	// Debug级别: 记录组件状态验证结果
	app.logger.Debug("Verifying component availability for sample callback chain")

	// Get the desktop capture from GStreamer manager
	capture := app.gstreamerMgr.GetCapture()
	if capture == nil {
		// Error级别: 链路中断 - GStreamer捕获不可用
		app.logger.Error("Sample callback chain failed: desktop capture not available")
		return fmt.Errorf("desktop capture not available")
	}
	// Debug级别: 记录组件状态验证结果
	app.logger.Debug("GStreamer desktop capture component verified")

	// Get the WebRTC bridge from WebRTC manager
	bridge := app.webrtcMgr.GetBridge()
	if bridge == nil {
		// Error级别: 链路中断 - WebRTC桥接器不可用
		app.logger.Error("Sample callback chain failed: WebRTC bridge not available")
		return fmt.Errorf("WebRTC bridge not available")
	}
	// Debug级别: 记录组件状态验证结果
	app.logger.Debug("WebRTC bridge component verified")

	// Get MediaStream for validation
	mediaStream := app.webrtcMgr.GetMediaStream()
	if mediaStream == nil {
		// Warn级别: 样本处理异常 - MediaStream不可用
		app.logger.Warn("Sample callback chain warning: MediaStream not available")
	} else {
		// Debug级别: 记录组件状态验证结果和样本处理统计
		stats := mediaStream.GetStats()
		app.logger.Debugf("MediaStream status verified: total_tracks=%d, active_tracks=%d",
			stats.TotalTracks, stats.ActiveTracks)
	}

	// Trace级别: 记录样本处理统计初始化
	var videoSampleCount, audioSampleCount int64
	app.logger.Trace("Initializing sample processing counters")

	// Set up the connection: GStreamer sample callback -> WebRTC bridge
	capture.SetSampleCallback(func(sample *gstreamer.Sample) error {
		if sample == nil {
			// Warn级别: 样本处理异常
			app.logger.Warn("Sample callback received nil sample")
			return fmt.Errorf("received nil sample")
		}

		// Trace级别: 记录每个样本的详细处理信息和类型识别
		app.logger.Tracef("Processing sample: type=%s, size=%d bytes, timestamp=%v, dimensions=%dx%d",
			sample.Format.MediaType.String(), sample.Size(), sample.Timestamp,
			sample.Format.Width, sample.Format.Height)

		// Process the sample through WebRTC bridge
		var err error
		if sample.IsVideo() {
			videoSampleCount++
			err = bridge.ProcessVideoSample(sample)

			// Debug级别: 记录样本处理统计（每 1000 个样本）
			if videoSampleCount%1000 == 0 {
				app.logger.Debugf("Video sample processing milestone: %d samples processed", videoSampleCount)
			}

			// Trace级别: 记录每个样本的详细处理信息和类型识别
			app.logger.Tracef("Video sample processed: count=%d, codec=%s, processing_result=%v",
				videoSampleCount, sample.Format.Codec, err == nil)
		} else if sample.IsAudio() {
			audioSampleCount++
			err = bridge.ProcessAudioSample(sample)

			// Debug级别: 记录样本处理统计（每 1000 个样本）
			if audioSampleCount%1000 == 0 {
				app.logger.Debugf("Audio sample processing milestone: %d samples processed", audioSampleCount)
			}

			// Trace级别: 记录每个样本的详细处理信息和类型识别
			app.logger.Tracef("Audio sample processed: count=%d, codec=%s, channels=%d, sample_rate=%d, processing_result=%v",
				audioSampleCount, sample.Format.Codec, sample.Format.Channels, sample.Format.SampleRate, err == nil)
		}

		if err != nil {
			// Warn级别: 样本处理异常
			app.logger.Warnf("Sample processing failed for %s sample: %v",
				sample.Format.MediaType.String(), err)
		}

		return err
	})

	app.gstreamerMgr.SetEncodedSampleCallback(func(sample *gstreamer.Sample) error {
		if sample == nil {
			// Warn级别: 样本处理异常
			app.logger.Warn("Encoded sample callback received nil sample")
			return fmt.Errorf("received nil sample")
		}

		// Trace级别: 记录编码样本的详细处理信息和类型识别
		app.logger.Tracef("Processing encoded sample: type=%s, size=%d bytes, codec=%s, timestamp=%v",
			sample.Format.MediaType.String(), sample.Size(), sample.Format.Codec, sample.Timestamp)

		// Process the sample through WebRTC bridge
		var err error
		if sample.IsVideo() {
			err = bridge.ProcessVideoSample(sample)
			// Trace级别: 记录编码视频样本处理结果
			app.logger.Tracef("Encoded video sample processed: codec=%s, size=%d bytes, result=%v",
				sample.Format.Codec, sample.Size(), err == nil)
		} else if sample.IsAudio() {
			err = bridge.ProcessAudioSample(sample)
			// Trace级别: 记录编码音频样本处理结果
			app.logger.Tracef("Encoded audio sample processed: codec=%s, size=%d bytes, result=%v",
				sample.Format.Codec, sample.Size(), err == nil)
		}

		if err != nil {
			// Warn级别: 样本处理异常
			app.logger.Warnf("Encoded sample processing failed for %s sample: %v",
				sample.Format.MediaType.String(), err)
		}

		return err
	})

	// Info级别: 记录样本回调链路建立成功状态
	app.logger.Info("Sample callback chain established successfully")
	// Debug级别: 记录组件状态验证结果和链路配置详情
	app.logger.Debug("Sample callback chain configuration:")
	app.logger.Debug("  Raw sample callback: configured")
	app.logger.Debug("  Encoded sample callback: configured")
	app.logger.Debug("  Target bridge: WebRTC bridge")
	app.logger.Debug("  Component chain: GStreamer -> Bridge -> MediaStream")

	return nil
}

// Start 启动应用
func (app *BDWindApp) Start() error {
	app.logger.Info("Starting BDWind-GStreamer v1.0.0")

	// Setup signal handling for graceful shutdown
	signal.Notify(app.sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start signal handler goroutine
	app.wg.Add(1)
	go app.handleSignals()

	// Define component startup order: metrics → gstreamer → webrtc → webserver
	// Note: webserver.Manager doesn't implement ComponentManager interface as it IS the webserver
	type managerInfo struct {
		name    string
		manager interface {
			Start(ctx context.Context) error
			Stop(ctx context.Context) error
			IsRunning() bool
		}
	}

	managers := []managerInfo{
		{"metrics", app.metricsMgr},
		{"gstreamer", app.gstreamerMgr},
		{"webrtc", app.webrtcMgr},
		{"webserver", app.webserverMgr},
	}

	// Start components in order, but handle webserver specially
	for i, mgr := range managers {
		if mgr.name == "webserver" {
			// Before starting webserver, register other components for route setup
			app.logger.Infof("Registering components with webserver before starting...")
			if err := app.registerComponentsWithWebServer(); err != nil {
				app.logger.Errorf("Failed to register components with webserver: %v", err)

				// Rollback: stop already started components
				for j := i - 1; j >= 0; j-- {
					app.logger.Warnf("Rolling back: stopping %s manager...", managers[j].name)
					if stopErr := managers[j].manager.Stop(app.rootCtx); stopErr != nil {
						app.logger.Errorf("Failed to stop %s during rollback: %v",
							managers[j].name, stopErr)
					}
				}

				return fmt.Errorf("failed to register components with webserver: %w", err)
			}
		}

		app.logger.Infof("Starting %s manager...", mgr.name)

		if err := mgr.manager.Start(app.rootCtx); err != nil {
			app.logger.Errorf("Failed to start %s manager: %v", mgr.name, err)

			// Rollback: stop already started components in reverse order
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

		app.logger.Infof("%s manager started successfully", mgr.name)
	}

	// Connect GStreamer to WebRTC after all components are started
	if err := app.connectGStreamerToWebRTC(); err != nil {
		app.logger.Warnf("Failed to connect GStreamer to WebRTC: %v", err)
		// Don't fail the startup, but log the error
	}

	app.logger.Infof("BDWind-GStreamer application started successfully")
	return nil
}

// Stop 停止应用
func (app *BDWindApp) Stop(ctx context.Context) error {
	app.logger.Info("Stopping BDWind-GStreamer application...")

	// Cancel the root context to signal all components to stop
	app.cancelFunc()

	// Wait for signal handler to complete
	app.wg.Wait()

	// Use lifecycle configuration for shutdown timeout if no context timeout is set
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), app.config.Lifecycle.ShutdownTimeout)
		defer cancel()
	}

	// Define component shutdown order: webserver → webrtc → gstreamer → metrics (reverse of startup)
	type managerInfo struct {
		name    string
		manager interface {
			Stop(ctx context.Context) error
		}
	}

	managers := []managerInfo{
		{"webserver", app.webserverMgr},
		{"webrtc", app.webrtcMgr},
		{"gstreamer", app.gstreamerMgr},
		{"metrics", app.metricsMgr},
	}

	// Collect all errors but continue stopping other components
	var errors []error

	// Stop components in reverse order
	for _, mgr := range managers {
		app.logger.Infof("Stopping %s manager...", mgr.name)

		if err := mgr.manager.Stop(ctx); err != nil {
			app.logger.Errorf("Failed to stop %s manager: %v", mgr.name, err)
			errors = append(errors, fmt.Errorf("failed to stop %s: %w", mgr.name, err))
		} else {
			app.logger.Infof("%s manager stopped successfully", mgr.name)
		}
	}

	// Report final status
	if len(errors) > 0 {
		app.logger.Errorf("BDWind-GStreamer application stopped with %d errors", len(errors))
		return fmt.Errorf("errors during shutdown: %v", errors)
	}

	app.logger.Info("BDWind-GStreamer application stopped successfully")
	return nil
}

// GetConfig 获取应用配置
func (app *BDWindApp) GetConfig() *config.Config {
	return app.config
}

// GetWebRTCManager 获取WebRTC管理器
func (app *BDWindApp) GetWebRTCManager() *webrtc.Manager {
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
		app.logger.Infof("Received signal: %v, initiating graceful shutdown", sig)
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

// registerComponentsWithWebServer registers all components with the WebServer for route setup
func (app *BDWindApp) registerComponentsWithWebServer() error {
	app.logger.Infof("=== Starting component registration with webserver ===")

	webServer := app.webserverMgr.GetWebServer()
	if webServer == nil {
		app.logger.Error("webserver instance is nil")
		return fmt.Errorf("webserver instance is nil")
	}
	app.logger.Debug("webserver instance obtained successfully")

	// Register components that need to expose HTTP routes
	// For now, we'll register components that implement the ComponentManager interface
	// Components that don't implement SetupRoutes will be skipped with a warning

	registrationResults := make(map[string]bool)
	var registrationErrors []error

	// Try to register metrics component
	app.logger.Infof("--- Attempting to register metrics component ---")
	app.logger.Debugf("metrics manager instance: %T", app.metricsMgr)
	app.logger.Debugf("metrics manager nil check: %v", app.metricsMgr == nil)

	// Check if metrics manager implements ComponentManager interface
	if mgr, ok := interface{}(app.metricsMgr).(webserver.ComponentManager); ok {
		app.logger.Infof("SUCCESS: metrics component implements ComponentManager interface")
		app.logger.Infof("Registering metrics component with webserver...")

		if err := webServer.RegisterComponent("metrics", mgr); err != nil {
			app.logger.Errorf("Failed to register metrics component: %v", err)
			registrationErrors = append(registrationErrors, fmt.Errorf("failed to register metrics component: %w", err))
			registrationResults["metrics"] = false
		} else {
			app.logger.Infof("SUCCESS: metrics component registered successfully")
			registrationResults["metrics"] = true
		}
	} else {
		app.logger.Infof("SKIP: metrics component does not implement ComponentManager interface")
		registrationResults["metrics"] = false
	}

	// Try to register gstreamer component
	app.logger.Infof("--- Attempting to register gstreamer component ---")
	app.logger.Debugf("gstreamer manager instance: %T", app.gstreamerMgr)
	app.logger.Debugf("gstreamer manager nil check: %v", app.gstreamerMgr == nil)

	// Check if gstreamer manager implements ComponentManager interface
	if mgr, ok := interface{}(app.gstreamerMgr).(webserver.ComponentManager); ok {
		app.logger.Infof("SUCCESS: gstreamer component implements ComponentManager interface")
		app.logger.Infof("Registering gstreamer component with webserver...")

		if err := webServer.RegisterComponent("gstreamer", mgr); err != nil {
			app.logger.Errorf("Failed to register gstreamer component: %v", err)
			registrationErrors = append(registrationErrors, fmt.Errorf("failed to register gstreamer component: %w", err))
			registrationResults["gstreamer"] = false
		} else {
			app.logger.Infof("SUCCESS: gstreamer component registered successfully")
			registrationResults["gstreamer"] = true
		}
	} else {
		app.logger.Infof("SKIP: gstreamer component does not implement ComponentManager interface")
		registrationResults["gstreamer"] = false
	}

	// Try to register webrtc component
	app.logger.Infof("--- Attempting to register webrtc component ---")
	app.logger.Debugf("webrtc manager instance: %T", app.webrtcMgr)
	app.logger.Debugf("webrtc manager nil check: %v", app.webrtcMgr == nil)

	// Since the WebRTC manager implements ComponentManager interface (verified by successful build),
	// we can directly register it without type assertion
	app.logger.Infof("SUCCESS: webrtc component implements ComponentManager interface")
	app.logger.Infof("Registering webrtc component with webserver...")

	if err := webServer.RegisterComponent("webrtc", app.webrtcMgr); err != nil {
		app.logger.Errorf("Failed to register webrtc component: %v", err)
		registrationErrors = append(registrationErrors, fmt.Errorf("failed to register webrtc component: %w", err))
		registrationResults["webrtc"] = false
	} else {
		app.logger.Infof("SUCCESS: webrtc component registered successfully")
		registrationResults["webrtc"] = true
	}

	// Summary of registration results
	app.logger.Infof("=== Component registration summary ===")
	successCount := 0
	for component, success := range registrationResults {
		status := "FAILED"
		if success {
			status = "SUCCESS"
			successCount++
		}
		app.logger.Infof("Component '%s': %s", component, status)
	}

	app.logger.Infof("Total components processed: %d", len(registrationResults))
	app.logger.Infof("Successfully registered: %d", successCount)
	app.logger.Infof("Failed/Skipped: %d", len(registrationResults)-successCount)

	// If there were registration errors, return the first one
	if len(registrationErrors) > 0 {
		app.logger.Errorf("Component registration completed with %d errors", len(registrationErrors))
		return registrationErrors[0]
	}

	app.logger.Infof("=== Component registration completed successfully ===")
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

	// Force stop all components in reverse order (same as normal shutdown)
	managers := []struct {
		name    string
		manager interface {
			Stop(ctx context.Context) error
		}
	}{
		{"webserver", app.webserverMgr},
		{"webrtc", app.webrtcMgr},
		{"gstreamer", app.gstreamerMgr},
		{"metrics", app.metricsMgr},
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
