package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

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
	logger       *log.Logger
	startTime    time.Time

	// Context lifecycle management fields
	rootCtx    context.Context
	cancelFunc context.CancelFunc
	wg         sync.WaitGroup
	sigChan    chan os.Signal
}

// NewBDWindApp 创建BDWind应用
func NewBDWindApp(config *config.Config, logger *log.Logger) (*BDWindApp, error) {
	if logger == nil {
		logger = log.Default()
	}

	// Create root context for lifecycle management
	rootCtx, cancelFunc := context.WithCancel(context.Background())

	// Create signal channel for graceful shutdown
	sigChan := make(chan os.Signal, 1)

	// 创建WebRTC管理器
	webrtcMgrConfig := &webrtc.ManagerConfig{
		Config: config,
		Logger: logger,
	}

	webrtcMgr, err := webrtc.NewManager(rootCtx, webrtcMgrConfig)
	if err != nil {
		cancelFunc() // Clean up context on error
		return nil, fmt.Errorf("failed to create WebRTC manager: %w", err)
	}

	// 创建GStreamer管理器
	gstreamerMgrConfig := &gstreamer.ManagerConfig{
		Config: config.GStreamer,
		Logger: logger,
	}

	gstreamerMgr, err := gstreamer.NewManager(rootCtx, gstreamerMgrConfig)
	if err != nil {
		cancelFunc() // Clean up context on error
		return nil, fmt.Errorf("failed to create GStreamer manager: %w", err)
	}

	// 创建webserver管理器（包含认证功能）
	webserverMgr, err := webserver.NewManager(rootCtx, config.WebServer, logger)
	if err != nil {
		cancelFunc() // Clean up context on error
		return nil, fmt.Errorf("failed to create webserver manager: %w", err)
	}

	// 创建监控管理器
	metricsMgr, err := metrics.NewManager(rootCtx, config.Metrics, logger)
	if err != nil {
		cancelFunc() // Clean up context on error
		return nil, fmt.Errorf("failed to create metrics manager: %w", err)
	}

	app := &BDWindApp{
		config:       config,
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
	app.logger.Printf("Connecting GStreamer desktop capture to WebRTC bridge...")

	// Get the desktop capture from GStreamer manager
	capture := app.gstreamerMgr.GetCapture()
	if capture == nil {
		return fmt.Errorf("desktop capture not available")
	}

	// Get the WebRTC bridge from WebRTC manager
	bridge := app.webrtcMgr.GetBridge()
	if bridge == nil {
		return fmt.Errorf("WebRTC bridge not available")
	}

	// Set up the connection: GStreamer sample callback -> WebRTC bridge
	capture.SetSampleCallback(func(sample *gstreamer.Sample) error {
		if sample == nil {
			return fmt.Errorf("received nil sample")
		}

		// Process the sample through WebRTC bridge
		if sample.IsVideo() {
			return bridge.ProcessVideoSample(sample)
		} else if sample.IsAudio() {
			return bridge.ProcessAudioSample(sample)
		}

		return nil
	})

	app.gstreamerMgr.SetEncodedSampleCallback(func(sample *gstreamer.Sample) error {
		if sample == nil {
			return fmt.Errorf("received nil sample")
		}
		// Process the sample through WebRTC bridge
		if sample.IsVideo() {
			return bridge.ProcessVideoSample(sample)
		} else if sample.IsAudio() {
			return bridge.ProcessAudioSample(sample)
		}
		return nil
	})

	app.logger.Printf("GStreamer desktop capture connected to WebRTC bridge successfully")
	return nil
}

// Start 启动应用
func (app *BDWindApp) Start() error {
	app.logger.Printf("Starting BDWind-GStreamer v1.0.0")

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
			app.logger.Printf("Registering components with webserver before starting...")
			if err := app.registerComponentsWithWebServer(); err != nil {
				app.logger.Printf("Failed to register components with webserver: %v", err)

				// Rollback: stop already started components
				for j := i - 1; j >= 0; j-- {
					app.logger.Printf("Rolling back: stopping %s manager...", managers[j].name)
					if stopErr := managers[j].manager.Stop(app.rootCtx); stopErr != nil {
						app.logger.Printf("Failed to stop %s during rollback: %v",
							managers[j].name, stopErr)
					}
				}

				return fmt.Errorf("failed to register components with webserver: %w", err)
			}
		}

		app.logger.Printf("Starting %s manager...", mgr.name)

		if err := mgr.manager.Start(app.rootCtx); err != nil {
			app.logger.Printf("Failed to start %s manager: %v", mgr.name, err)

			// Rollback: stop already started components in reverse order
			for j := i - 1; j >= 0; j-- {
				app.logger.Printf("Rolling back: stopping %s manager...", managers[j].name)
				if stopErr := managers[j].manager.Stop(app.rootCtx); stopErr != nil {
					app.logger.Printf("Failed to stop %s during rollback: %v",
						managers[j].name, stopErr)
				} else {
					app.logger.Printf("%s manager stopped during rollback", managers[j].name)
				}
			}

			return fmt.Errorf("failed to start %s manager: %w", mgr.name, err)
		}

		app.logger.Printf("%s manager started successfully", mgr.name)
	}

	// Connect GStreamer to WebRTC after all components are started
	if err := app.connectGStreamerToWebRTC(); err != nil {
		app.logger.Printf("Failed to connect GStreamer to WebRTC: %v", err)
		// Don't fail the startup, but log the error
	}

	app.logger.Printf("BDWind-GStreamer application started successfully")
	return nil
}

// Stop 停止应用
func (app *BDWindApp) Stop(ctx context.Context) error {
	app.logger.Println("Stopping BDWind-GStreamer application...")

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
		app.logger.Printf("Stopping %s manager...", mgr.name)

		if err := mgr.manager.Stop(ctx); err != nil {
			app.logger.Printf("Failed to stop %s manager: %v", mgr.name, err)
			errors = append(errors, fmt.Errorf("failed to stop %s: %w", mgr.name, err))
		} else {
			app.logger.Printf("%s manager stopped successfully", mgr.name)
		}
	}

	// Report final status
	if len(errors) > 0 {
		app.logger.Printf("BDWind-GStreamer application stopped with %d errors", len(errors))
		return fmt.Errorf("errors during shutdown: %v", errors)
	}

	app.logger.Println("BDWind-GStreamer application stopped successfully")
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
		app.logger.Printf("Received signal: %v, initiating graceful shutdown", sig)
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
	webServer := app.webserverMgr.GetWebServer()
	if webServer == nil {
		return fmt.Errorf("webserver instance is nil")
	}

	// Register components that need to expose HTTP routes
	// For now, we'll register components that implement the ComponentManager interface
	// Components that don't implement SetupRoutes will be skipped with a warning

	// Try to register metrics component
	if mgr, ok := interface{}(app.metricsMgr).(webserver.ComponentManager); ok {
		app.logger.Printf("Registering metrics component with webserver...")
		if err := webServer.RegisterComponent("metrics", mgr); err != nil {
			return fmt.Errorf("failed to register metrics component: %w", err)
		}
		app.logger.Printf("metrics component registered successfully")
	} else {
		app.logger.Printf("metrics component does not implement ComponentManager interface, skipping route registration")
	}

	// Try to register gstreamer component
	if mgr, ok := interface{}(app.gstreamerMgr).(webserver.ComponentManager); ok {
		app.logger.Printf("Registering gstreamer component with webserver...")
		if err := webServer.RegisterComponent("gstreamer", mgr); err != nil {
			return fmt.Errorf("failed to register gstreamer component: %w", err)
		}
		app.logger.Printf("gstreamer component registered successfully")
	} else {
		app.logger.Printf("gstreamer component does not implement ComponentManager interface, skipping route registration")
	}

	// Try to register webrtc component
	if mgr, ok := interface{}(app.webrtcMgr).(webserver.ComponentManager); ok {
		app.logger.Printf("Registering webrtc component with webserver...")
		if err := webServer.RegisterComponent("webrtc", mgr); err != nil {
			return fmt.Errorf("failed to register webrtc component: %w", err)
		}
		app.logger.Printf("webrtc component registered successfully")
	} else {
		app.logger.Printf("webrtc component does not implement ComponentManager interface, skipping route registration")
	}

	app.logger.Printf("Component registration completed")
	return nil
}

// Shutdown initiates graceful shutdown of the application
func (app *BDWindApp) Shutdown() {
	app.logger.Println("Initiating application shutdown...")
	app.cancelFunc()
}

// Wait waits for all goroutines to complete
func (app *BDWindApp) Wait() {
	app.wg.Wait()
}

// ForceShutdown forces immediate shutdown of the application
func (app *BDWindApp) ForceShutdown() error {
	app.logger.Println("Force shutdown initiated...")

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
			app.logger.Printf("Force shutdown: %s stop error: %v", mgr.name, err)
		} else {
			app.logger.Printf("Force shutdown: %s stopped successfully", mgr.name)
		}
	}

	app.logger.Println("Force shutdown completed")
	return nil
}

// IsHealthy 检查应用是否健康
func (app *BDWindApp) IsHealthy() bool {
	// 检查各个组件的健康状态
	// 所有启用的组件都必须正在运行才算健康

	// 检查 metrics 管理器
	if app.metricsMgr.IsEnabled() && !app.metricsMgr.IsRunning() {
		app.logger.Printf("Component metrics is enabled but not running")
		return false
	}

	// 检查 gstreamer 管理器
	if app.gstreamerMgr.IsEnabled() && !app.gstreamerMgr.IsRunning() {
		app.logger.Printf("Component gstreamer is enabled but not running")
		return false
	}

	// 检查 webrtc 管理器
	if app.webrtcMgr.IsEnabled() && !app.webrtcMgr.IsRunning() {
		app.logger.Printf("Component webrtc is enabled but not running")
		return false
	}

	// 检查 webserver 管理器
	if app.webserverMgr.IsEnabled() && !app.webserverMgr.IsRunning() {
		app.logger.Printf("Component webserver is enabled but not running")
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
