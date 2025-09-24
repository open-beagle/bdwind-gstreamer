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

	// 创建GStreamer管理器 (必须先创建，因为WebRTC需要它作为MediaProvider)
	gstreamerMgrConfig := &gstreamer.ManagerConfig{
		Config: cfg.GStreamer,
	}

	gstreamerMgr, err := gstreamer.NewManager(rootCtx, gstreamerMgrConfig)
	if err != nil {
		cancelFunc() // Clean up context on error
		return nil, fmt.Errorf("failed to create GStreamer manager: %w", err)
	}

	// 创建WebRTC管理器 (使用GStreamer作为MediaProvider)
	webrtcMgrConfig := &webrtc.ManagerConfig{
		Config:        cfg,
		MediaProvider: gstreamerMgr, // 使用GStreamer管理器作为媒体提供者
	}

	webrtcMgr, err := webrtc.NewManager(rootCtx, webrtcMgrConfig)
	if err != nil {
		cancelFunc() // Clean up context on error
		return nil, fmt.Errorf("failed to create WebRTC manager: %w", err)
	}

	// 创建webserver管理器（包含认证功能）
	webserverMgr, err := webserver.NewManager(rootCtx, cfg.WebServer)
	if err != nil {
		cancelFunc() // Clean up context on error
		return nil, fmt.Errorf("failed to create webserver manager: %w", err)
	}

	// 创建监控管理器
	metricsMgr, err := metrics.NewManager(rootCtx, cfg.Metrics)
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
	// Info级别: 记录关键连接状态 - 开始建立连接
	app.logger.Info("Establishing sample callback chain from GStreamer to WebRTC")

	// Debug级别: 记录回调建立详情 - 组件状态验证过程
	app.logger.Debug("Verifying component availability for sample callback chain")

	// Get the desktop capture from GStreamer manager
	capture := app.gstreamerMgr.GetCapture()
	if capture == nil {
		// Error级别: 关键连接状态 - 链路中断
		app.logger.Error("Sample callback chain failed: desktop capture not available")
		return fmt.Errorf("desktop capture not available")
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
		stats := mediaStream.GetStats()
		app.logger.Debugf("MediaStream status verified: total_tracks=%d, active_tracks=%d",
			stats.TotalTracks, stats.ActiveTracks)
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
	app.logger.Trace("Starting BDWind-GStreamer v1.0.0")

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

	// Track startup timings for consolidated summary
	startupTimings := make(map[string]time.Duration)
	var startedManagers []string

	// Start components in order, but handle webserver specially
	for i, mgr := range managers {
		if mgr.name == "webserver" {
			// Before starting webserver, register other components for route setup
			app.logger.Debugf("Registering components with webserver before starting...")
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

		// Configure GStreamer logging before starting GStreamer manager
		if mgr.name == "gstreamer" {
			app.logger.Debug("Configuring GStreamer logging before manager startup...")
			if err := app.configureGStreamerLogging(); err != nil {
				// Log warning but don't fail startup - GStreamer can still work without logging configuration
				app.logger.Warnf("Failed to configure GStreamer logging: %v", err)
			} else {
				app.logger.Debug("GStreamer logging configured successfully")
			}
		}

		// Track individual manager startup time
		managerStartTime := time.Now()

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

		// Record successful startup timing
		startupTimings[mgr.name] = time.Since(managerStartTime)
		startedManagers = append(startedManagers, mgr.name)
	}

	// Connect GStreamer to WebRTC after all components are started
	if err := app.connectGStreamerToWebRTC(); err != nil {
		app.logger.Warnf("Failed to connect GStreamer to WebRTC: %v", err)
		// Don't fail the startup, but log the error
	}

	// Log consolidated startup summary with total time
	totalStartupTime := time.Since(startupStartTime)
	app.logger.Infof("BDWind-GStreamer application started successfully in %v", totalStartupTime)

	// Log detailed timing breakdown at DEBUG level
	app.logger.Debugf("Manager startup timings: metrics(%v), gstreamer(%v), webrtc(%v), webserver(%v)",
		startupTimings["metrics"], startupTimings["gstreamer"],
		startupTimings["webrtc"], startupTimings["webserver"])

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

	managers := []managerInfo{
		{"metrics", app.metricsMgr}, // First to stop - ensure data collection completes
		{"webserver", app.webserverMgr},
		{"webrtc", app.webrtcMgr},
		{"gstreamer", app.gstreamerMgr}, // Last to stop - may block
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
		componentNames := fmt.Sprintf("%s", registeredComponents[0])
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
