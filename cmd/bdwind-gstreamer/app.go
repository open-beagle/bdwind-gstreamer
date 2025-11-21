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

	"github.com/open-beagle/bdwind-gstreamer/internal/common/events"
	"github.com/open-beagle/bdwind-gstreamer/internal/common/protocol"
	"github.com/open-beagle/bdwind-gstreamer/internal/common/config"
	"github.com/open-beagle/bdwind-gstreamer/internal/gstreamer"
	"github.com/open-beagle/bdwind-gstreamer/internal/metrics"
	"github.com/open-beagle/bdwind-gstreamer/internal/webrtc"
	"github.com/open-beagle/bdwind-gstreamer/internal/webserver"
)

// BDWindApp BDWind应用（go-gst架构）
type BDWindApp struct {
	config       *config.Config
	webserverMgr *webserver.Manager
	webrtcMgr    *webrtc.WebRTCManager
	metricsMgr   *metrics.Manager
	logger       *logrus.Entry
	startTime    time.Time

	// GStreamer 管理器
	gstreamerMgr *gstreamer.Manager

	// 事件系统
	eventBus          events.EventBus
	signalingHandlers *webrtc.SignalingEventHandlers

	// Context lifecycle management
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
	logger.Info("Using go-gst implementation")

	return NewGoGstBDWindApp(cfg, logger)
}

// NewGoGstBDWindApp 创建使用 go-gst 实现的BDWind应用
func NewGoGstBDWindApp(cfg *config.Config, logger *logrus.Entry) (*BDWindApp, error) {
	logger.Info("Creating go-gst BDWind application...")

	// Create root context for lifecycle management
	rootCtx, cancelFunc := context.WithCancel(context.Background())

	// Create signal channel for graceful shutdown
	sigChan := make(chan os.Signal, 1)

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
	webrtcMgr.SetEventBus(eventBus)
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
	logger.Info("Creating GStreamer manager...")
	gstreamerMgr, err := gstreamer.NewManager(cfg.GStreamer)
	if err != nil {
		logger.Errorf("Failed to create GStreamer manager: %v", err)
		cancelFunc()
		return nil, fmt.Errorf("failed to create GStreamer manager: %w", err)
	}
	logger.Info("GStreamer manager created successfully")

	// 将 WebRTC 作为视频数据接收器注入到 GStreamer
	logger.Debug("Connecting GStreamer to WebRTC via VideoDataSink interface...")
	gstreamerMgr.SetVideoSink(webrtcMgr)
	logger.Debug("GStreamer connected to WebRTC successfully")

	// 订阅WebRTC事件到GStreamer（用于按需推流）
	logger.Info("Subscribing GStreamer to WebRTC lifecycle events...")
	gstreamerMgr.SubscribeToWebRTCEvents(eventBus)
	logger.Info("GStreamer subscribed to WebRTC events successfully")

	// 创建webserver管理器
	logger.Info("Creating webserver manager...")
	webserverMgr, err := webserver.NewManager(rootCtx, cfg.WebServer)
	if err != nil {
		logger.Errorf("Failed to create webserver manager: %v", err)
		cancelFunc()
		return nil, fmt.Errorf("failed to create webserver manager: %w", err)
	}
	logger.Info("Webserver manager created successfully")

	// 设置WebRTC配置到信令服务器
	logger.Info("Configuring signaling server with WebRTC config...")
	webserverMgr.GetSignalingServer().SetWebRTCConfig(cfg.WebRTC)
	logger.Infof("Signaling server configured with %d ICE servers", len(cfg.WebRTC.ICEServers))

	// 设置客户端断开连接回调
	webserverMgr.GetSignalingServer().SetClientDisconnectHandler(func(clientID string) {
		logger.Infof("Client disconnected: %s", clientID)
		webrtcMgr.OnClientDisconnected(clientID)
	})

	// 配置信令消息处理器
	logger.Info("Configuring signaling message handler...")
	signalingClients := make(map[string]*webrtc.SignalingClient)
	var clientsMutex sync.Mutex

	webserverMgr.SetSignalingMessageHandler(func(clientID string, messageType string, data map[string]interface{}) error {
		clientsMutex.Lock()
		client, ok := signalingClients[clientID]
		if !ok {
			// 创建新的信令客户端
			sendFunc := func(msg *protocol.StandardMessage) error {
				return webserverMgr.GetSignalingServer().SendMessageByClientID(clientID, msg)
			}
			client = webrtc.NewSignalingClient(clientID, "bdwind", sendFunc, eventBus)
			signalingClients[clientID] = client
			logger.Infof("Created new signaling client handler for %s", clientID)

			// 通知WebRTC Manager有新客户端连接
			webrtcMgr.SetCurrentSessionID(clientID)
			webrtcMgr.OnClientConnected(clientID)
		}
		clientsMutex.Unlock()

		// 构造标准消息
		msg := &protocol.StandardMessage{
			Type: protocol.MessageType(messageType),
			Data: data,
		}

		// 尝试从数据中恢复元数据
		if peerID, ok := data["peer_id"].(string); ok {
			msg.PeerID = peerID
		}
		if timestamp, ok := data["timestamp"].(int64); ok {
			msg.Timestamp = timestamp
		} else if timestamp, ok := data["timestamp"].(float64); ok {
			msg.Timestamp = int64(timestamp)
		}

		// 处理消息
		client.HandleMessage(msg)
		return nil
	})

	// 创建监控管理器
	logger.Info("Creating metrics manager...")
	metricsMgr, err := metrics.NewManager(rootCtx, cfg.Metrics)
	if err != nil {
		logger.Errorf("Failed to create metrics manager: %v", err)
		cancelFunc()
		return nil, fmt.Errorf("failed to create metrics manager: %w", err)
	}
	logger.Info("Metrics manager created successfully")

	app := &BDWindApp{
		config:            cfg,
		webserverMgr:      webserverMgr,
		webrtcMgr:         webrtcMgr,
		metricsMgr:        metricsMgr,
		logger:            logger,
		startTime:         time.Now(),
		rootCtx:           rootCtx,
		cancelFunc:        cancelFunc,
		sigChan:           sigChan,
		gstreamerMgr:      gstreamerMgr,
		eventBus:          eventBus,
		signalingHandlers: signalingHandlers,
	}

	logger.Info("Go-gst BDWind application created successfully")
	return app, nil
}

// Start 启动应用
func (app *BDWindApp) Start() error {
	app.logger.Info("Starting BDWind-GStreamer v1.0.0...")
	app.logger.Info("Starting go-gst architecture...")

	// Setup signal handling for graceful shutdown
	signal.Notify(app.sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start signal handler goroutine
	app.wg.Add(1)
	go app.handleSignals()

	// 启动组件顺序：metrics → gstreamerMgr → webserver
	app.logger.Info("Starting components: metrics → gstreamerMgr → webserver")

	// 1. 启动 metrics
	app.logger.Info("Starting metrics manager (1/3)...")
	if err := app.metricsMgr.Start(app.rootCtx); err != nil {
		return fmt.Errorf("failed to start metrics manager: %w", err)
	}
	app.logger.Info("Metrics manager started successfully")

	// 2. 启动 WebRTC
	app.logger.Info("Starting WebRTC manager (2/4)...")
	if err := app.webrtcMgr.Start(app.rootCtx); err != nil {
		app.metricsMgr.Stop(app.rootCtx)
		return fmt.Errorf("failed to start WebRTC: %w", err)
	}
	app.logger.Info("WebRTC manager started successfully")

	// 3. 启动 GStreamer（按需启动模式下跳过）
	if app.config.GStreamer.OnDemand.Enabled {
		app.logger.Info("GStreamer on-demand mode enabled, will start when client connects (3/4)")
	} else {
		app.logger.Info("Starting GStreamer manager (3/4)...")
		if err := app.gstreamerMgr.Start(); err != nil {
			app.webrtcMgr.Stop(app.rootCtx)
			app.metricsMgr.Stop(app.rootCtx)
			return fmt.Errorf("failed to start GStreamer: %w", err)
		}
		app.logger.Info("GStreamer manager started successfully")
	}

	// 4. 注册组件到 webserver
	app.logger.Info("Registering components with webserver...")
	if err := app.registerGoGstComponentsWithWebServer(); err != nil {
		app.gstreamerMgr.Stop()
		app.metricsMgr.Stop(app.rootCtx)
		return fmt.Errorf("failed to register components with webserver: %w", err)
	}
	app.logger.Info("Components registered with webserver successfully")

	// 5. 启动 webserver
	app.logger.Info("Starting webserver manager (4/4)...")
	if err := app.webserverMgr.Start(app.rootCtx); err != nil {
		app.gstreamerMgr.Stop()
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

// Stop 停止应用
func (app *BDWindApp) Stop(ctx context.Context) error {
	app.logger.Info("BDWind-GStreamer Application Shutdown Started")

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

	// Collect all errors but continue stopping other components
	var errors []error

	// Stop components in order: metrics → webserver → gstreamerMgr
	app.logger.Info("Stopping metrics manager...")
	if err := app.metricsMgr.Stop(ctx); err != nil {
		app.logger.Errorf("Failed to stop metrics manager: %v", err)
		errors = append(errors, fmt.Errorf("failed to stop metrics: %w", err))
	} else {
		app.logger.Info("Metrics manager stopped successfully")
	}

	app.logger.Info("Stopping webserver manager...")
	if err := app.webserverMgr.Stop(ctx); err != nil {
		app.logger.Errorf("Failed to stop webserver manager: %v", err)
		errors = append(errors, fmt.Errorf("failed to stop webserver: %w", err))
	} else {
		app.logger.Info("Webserver manager stopped successfully")
	}

	app.logger.Info("Stopping go-gst media bridge...")
	if err := app.gstreamerMgr.Stop(); err != nil {
		app.logger.Errorf("Failed to stop media bridge: %v", err)
		errors = append(errors, fmt.Errorf("failed to stop media bridge: %w", err))
	} else {
		app.logger.Info("Go-gst media bridge stopped successfully")
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

	if len(errors) > 0 {
		app.logger.Errorf("BDWind-GStreamer application stopped with %d errors", len(errors))
		return fmt.Errorf("errors during shutdown: %v", errors)
	}

	app.logger.Info("BDWind-GStreamer application stopped successfully")
	return nil
}

// handleSignals 处理系统信号
func (app *BDWindApp) handleSignals() {
	defer app.wg.Done()

	select {
	case sig := <-app.sigChan:
		app.logger.Infof("Received signal: %v", sig)
		app.logger.Info("Initiating graceful shutdown...")

		// Create shutdown context with timeout
		ctx, cancel := context.WithTimeout(context.Background(), app.config.Lifecycle.ShutdownTimeout)
		defer cancel()

		if err := app.Stop(ctx); err != nil {
			app.logger.Errorf("Error during shutdown: %v", err)
		}
	case <-app.rootCtx.Done():
		app.logger.Info("Root context cancelled, signal handler exiting")
	}
}

// registerGoGstComponentsWithWebServer 注册 go-gst 组件到 webserver
func (app *BDWindApp) registerGoGstComponentsWithWebServer() error {
	webServer := app.webserverMgr.GetWebServer()
	if webServer == nil {
		return fmt.Errorf("webserver is nil")
	}

	app.logger.Debug("Registering go-gst components with webserver...")

	// Register metrics component
	if err := webServer.RegisterComponent("metrics", app.metricsMgr); err != nil {
		return fmt.Errorf("failed to register metrics component: %w", err)
	}
	app.logger.Debug("Metrics component registered successfully")

	// Register WebRTC component
	if err := webServer.RegisterComponent("webrtc", app.webrtcMgr); err != nil {
		return fmt.Errorf("failed to register webrtc component: %w", err)
	}
	app.logger.Debug("WebRTC component registered successfully")

	// Note: gstreamerMgr 不需要注册为组件，它是内部桥接器

	app.logger.Info("All go-gst components registered with webserver")
	return nil
}

// GetGStreamerManager 获取 GStreamer 管理器
func (app *BDWindApp) GetGStreamerManager() *gstreamer.Manager {
	return app.gstreamerMgr
}

// GetWebRTCManager 获取WebRTC管理器
func (app *BDWindApp) GetWebRTCManager() *webrtc.WebRTCManager {
	return app.webrtcMgr
}

// GetWebServerManager 获取WebServer管理器
func (app *BDWindApp) GetWebServerManager() *webserver.Manager {
	return app.webserverMgr
}

// GetMetricsManager 获取Metrics管理器
func (app *BDWindApp) GetMetricsManager() *metrics.Manager {
	return app.metricsMgr
}
