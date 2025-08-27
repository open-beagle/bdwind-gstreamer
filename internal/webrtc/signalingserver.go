package webrtc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v4"
	"github.com/sirupsen/logrus"

	"github.com/open-beagle/bdwind-gstreamer/internal/config"
	"github.com/open-beagle/bdwind-gstreamer/internal/webrtc/protocol"
)

// SignalingMessage 信令消息
type SignalingMessage struct {
	Type      string          `json:"type"`
	PeerID    string          `json:"peer_id,omitempty"`
	Data      any             `json:"data,omitempty"`
	Error     *SignalingError `json:"error,omitempty"`
	MessageID string          `json:"message_id,omitempty"`
	Timestamp int64           `json:"timestamp,omitempty"`
}

// SignalingError 标准化的信令错误结构
type SignalingError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
	Type    string `json:"type,omitempty"`
}

// SignalingResponse 标准化的信令响应结构
type SignalingResponse struct {
	Success   bool            `json:"success"`
	Data      any             `json:"data,omitempty"`
	Error     *SignalingError `json:"error,omitempty"`
	MessageID string          `json:"message_id,omitempty"`
	Timestamp int64           `json:"timestamp"`
}

// SignalingEncoderConfig 编码器配置
type SignalingEncoderConfig struct {
	Codec string `yaml:"codec" json:"codec"`
}

// SignalingServer 信令服务器
type SignalingServer struct {
	clients               map[string]*SignalingClient
	apps                  map[string]map[string]*SignalingClient // appName -> clientID -> client
	broadcast             chan []byte
	register              chan *SignalingClient
	unregister            chan *SignalingClient
	upgrader              websocket.Upgrader
	mutex                 sync.RWMutex
	running               bool
	sdpGenerator          *SDPGenerator
	mediaStream           any // WebRTC 媒体流
	peerConnectionManager *PeerConnectionManager
	messageRouter         *MessageRouter
	protocolManager       *protocol.ProtocolManager
	protocolNegotiator    *ProtocolNegotiator
	logger                *logrus.Entry

	// 性能监控
	performanceMonitor *PerformanceMonitor
	concurrentRouter   *ConcurrentMessageRouter
	eventBus           EventBus // 事件总线
	ctx                context.Context
	cancel             context.CancelFunc
}

// EventBus 简单的事件总线接口
type EventBus interface {
	Emit(event string, data any)
	Subscribe(event string, handler func(data any))
	Unsubscribe(event string, handler func(data any))
}

// SimpleEventBus 简单的事件总线实现
type SimpleEventBus struct {
	handlers map[string][]func(data any)
	mutex    sync.RWMutex
}

// NewSimpleEventBus 创建简单事件总线
func NewSimpleEventBus() *SimpleEventBus {
	return &SimpleEventBus{
		handlers: make(map[string][]func(data any)),
	}
}

// Emit 发送事件
func (eb *SimpleEventBus) Emit(event string, data any) {
	eb.mutex.RLock()
	handlers, exists := eb.handlers[event]
	eb.mutex.RUnlock()

	if exists {
		for _, handler := range handlers {
			go handler(data) // 异步调用处理器
		}
	}
}

// Subscribe 订阅事件
func (eb *SimpleEventBus) Subscribe(event string, handler func(data any)) {
	eb.mutex.Lock()
	defer eb.mutex.Unlock()

	if eb.handlers[event] == nil {
		eb.handlers[event] = make([]func(data any), 0)
	}
	eb.handlers[event] = append(eb.handlers[event], handler)
}

// Unsubscribe 取消订阅事件
func (eb *SimpleEventBus) Unsubscribe(event string, handler func(data any)) {
	eb.mutex.Lock()
	defer eb.mutex.Unlock()

	handlers, exists := eb.handlers[event]
	if !exists {
		return
	}

	// 移除处理器（简单实现，实际应用中可能需要更复杂的匹配逻辑）
	for i, h := range handlers {
		if &h == &handler {
			eb.handlers[event] = append(handlers[:i], handlers[i+1:]...)
			break
		}
	}
}

// NewSignalingServer 创建信令服务器
func NewSignalingServer(ctx context.Context, encoderConfig *SignalingEncoderConfig, mediaStream any, iceServers []webrtc.ICEServer) *SignalingServer {
	sdpConfig := &SDPConfig{
		Codec: encoderConfig.Codec,
	}

	childCtx, cancel := context.WithCancel(ctx)

	// 创建事件总线
	eventBus := NewSimpleEventBus()

	// 创建PeerConnection管理器
	var pcManager *PeerConnectionManager
	if ms, ok := mediaStream.(MediaStream); ok {
		// 使用logrus进行日志记录
		pcManager = NewPeerConnectionManager(ms, iceServers)
	}

	// 创建协议管理器
	protocolManagerConfig := protocol.DefaultManagerConfig()
	protocolManagerConfig.EnableLogging = true
	protocolManager := protocol.NewProtocolManager(protocolManagerConfig)

	// 创建消息路由器
	messageRouterConfig := DefaultMessageRouterConfig()
	messageRouterConfig.EnableLogging = true
	messageRouterConfig.AutoProtocolDetect = true
	messageRouter := NewMessageRouter(protocolManager, messageRouterConfig)

	// 创建协议协商器
	negotiatorConfig := DefaultNegotiatorConfig()
	negotiatorConfig.EnableAutoDetection = true
	negotiatorConfig.EnableProtocolSwitch = true
	protocolNegotiator := NewProtocolNegotiator(messageRouter, negotiatorConfig)

	// 创建性能监控器
	performanceMonitor := NewPerformanceMonitor(DefaultPerformanceMonitorConfig())

	// 创建并发消息路由器
	concurrentRouterConfig := DefaultConcurrentRouterConfig()
	concurrentRouter := NewConcurrentMessageRouter(messageRouter, concurrentRouterConfig, performanceMonitor)

	server := &SignalingServer{
		clients:               make(map[string]*SignalingClient),
		apps:                  make(map[string]map[string]*SignalingClient),
		sdpGenerator:          NewSDPGenerator(sdpConfig),
		mediaStream:           mediaStream,
		peerConnectionManager: pcManager,
		protocolManager:       protocolManager,
		messageRouter:         messageRouter,
		protocolNegotiator:    protocolNegotiator,
		performanceMonitor:    performanceMonitor,
		concurrentRouter:      concurrentRouter,
		eventBus:              eventBus,
		logger:                config.GetLoggerWithPrefix("webrtc-signaling-server"),
		broadcast:             make(chan []byte),
		register:              make(chan *SignalingClient),
		unregister:            make(chan *SignalingClient),
		ctx:                   childCtx,
		cancel:                cancel,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // 允许跨域
			},
		},
	}

	// 设置事件处理器
	server.setupEventHandlers()

	server.logger.Trace("✅ Signaling server created with integrated protocol adapters")
	server.logger.Tracef("📋 Supported protocols: %v", protocolManager.GetSupportedProtocols())

	return server
}

// setupEventHandlers 设置事件处理器
func (s *SignalingServer) setupEventHandlers() {
	// 客户端协议检测事件
	s.eventBus.Subscribe("client:protocol-detected", func(data any) {
		if eventData, ok := data.(map[string]any); ok {
			clientID := eventData["client_id"].(string)
			protocol := eventData["protocol"].(protocol.ProtocolVersion)
			s.logger.Tracef("📡 Event: Client %s protocol detected as %s", clientID, protocol)
		}
	})

	// 协议降级事件
	s.eventBus.Subscribe("client:protocol-downgraded", func(data any) {
		if eventData, ok := data.(map[string]any); ok {
			clientID := eventData["client_id"].(string)
			fromProtocol := eventData["from_protocol"].(protocol.ProtocolVersion)
			toProtocol := eventData["to_protocol"].(protocol.ProtocolVersion)
			reason := eventData["reason"].(string)
			s.logger.Tracef("⬇️ Event: Client %s protocol downgraded from %s to %s (reason: %s)",
				clientID, fromProtocol, toProtocol, reason)
		}
	})

	// 连接状态变化事件
	s.eventBus.Subscribe("client:state-changed", func(data any) {
		if eventData, ok := data.(map[string]any); ok {
			clientID := eventData["client_id"].(string)
			oldState := eventData["old_state"].(ClientState)
			newState := eventData["new_state"].(ClientState)
			s.logger.Tracef("🔄 Event: Client %s state changed from %s to %s", clientID, oldState, newState)
		}
	})

	// 消息处理错误事件
	s.eventBus.Subscribe("message:processing-error", func(data any) {
		if eventData, ok := data.(map[string]any); ok {
			clientID := eventData["client_id"].(string)
			errorType := eventData["error_type"].(string)
			errorMessage := eventData["error_message"].(string)
			s.logger.Tracef("❌ Event: Message processing error for client %s: %s - %s",
				clientID, errorType, errorMessage)
		}
	})

	// ICE候选处理成功事件
	s.eventBus.Subscribe("ice-candidate:processed", func(data any) {
		if eventData, ok := data.(map[string]any); ok {
			clientID := eventData["client_id"].(string)
			processingTime := eventData["processing_time"].(time.Duration)
			candidate := eventData["candidate"].(string)
			success := eventData["success"].(bool)

			if success {
				s.logger.Debugf("🧊 Event: ICE candidate processed successfully for client %s in %v (candidate: %.50s...)",
					clientID, processingTime, candidate)
			} else {
				s.logger.Infof("❌ Event: ICE candidate processing failed for client %s after %v (candidate: %.50s...)",
					clientID, processingTime, candidate)
			}
		}
	})

	// ICE候选处理失败事件
	s.eventBus.Subscribe("ice-candidate:failed", func(data any) {
		if eventData, ok := data.(map[string]any); ok {
			clientID := eventData["client_id"].(string)
			errorMessage := eventData["error"].(string)
			candidate := eventData["candidate"].(string)
			isNetworkError := eventData["network_error"].(bool)

			if isNetworkError {
				s.logger.Infof("🌐 Event: ICE candidate network error for client %s: %s (candidate: %.50s...)",
					clientID, errorMessage, candidate)
			} else {
				s.logger.Infof("❌ Event: ICE candidate processing error for client %s: %s (candidate: %.50s...)",
					clientID, errorMessage, candidate)
			}
		}
	})
}

// Start 启动信令服务器
func (s *SignalingServer) Start() {
	s.logger.Debugf("🚀 Starting signaling server...")
	s.running = true

	// 启动性能监控器
	if s.performanceMonitor != nil {
		s.performanceMonitor.Start()
		s.logger.Debugf("✅ Performance monitor started")

		// 添加性能警报回调
		s.performanceMonitor.AddAlertCallback(func(alert *PerformanceAlert) {
			s.logger.Warnf("🚨 Performance Alert: %s - %s", alert.Type, alert.Message)
			// 可以在这里添加更多的警报处理逻辑，如发送通知等
		})
	}

	// 启动并发消息路由器
	if s.concurrentRouter != nil {
		if err := s.concurrentRouter.Start(); err != nil {
			s.logger.Infof("❌ Failed to start concurrent router: %v", err)
		} else {
			s.logger.Debugf("✅ Concurrent message router started")
		}
	}

	// 启动清理协程
	go s.cleanupRoutine()
	s.logger.Tracef("Signaling server cleanup routine started")

	s.logger.Tracef("Signaling server started and ready to accept connections")

	for s.running {
		select {
		case client := <-s.register:
			s.logger.Debugf("📝 Processing client registration: %s", client.ID)
			s.registerClient(client)

		case client := <-s.unregister:
			s.logger.Debugf("📝 Processing client unregistration: %s", client.ID)
			s.unregisterClient(client)

		case message := <-s.broadcast:
			s.logger.Debugf("📢 Processing broadcast message (length: %d bytes)", len(message))
			s.broadcastMessage(message)

		case <-s.ctx.Done():
			s.logger.Infof("🛑 Signaling server received shutdown signal")
			s.Stop()
			return
		}
	}
}

// Stop 停止信令服务器
func (s *SignalingServer) Stop() {
	s.logger.Infof("🛑 Stopping signaling server...")
	s.running = false
	s.cancel()

	// 停止并发消息路由器
	if s.concurrentRouter != nil {
		if err := s.concurrentRouter.Stop(); err != nil {
			s.logger.Infof("❌ Error stopping concurrent router: %v", err)
		} else {
			s.logger.Tracef("✅ Concurrent message router stopped")
		}
	}

	// 停止性能监控器
	if s.performanceMonitor != nil {
		s.performanceMonitor.Stop()
		s.logger.Tracef("✅ Performance monitor stopped")
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()

	clientCount := len(s.clients)
	s.logger.Tracef("📊 Closing %d active client connections", clientCount)

	// 关闭所有客户端连接
	for clientID, client := range s.clients {
		s.logger.Tracef("🔌 Closing connection for client %s", clientID)
		close(client.Send)
	}

	// 关闭所有PeerConnection
	if s.peerConnectionManager != nil {
		s.logger.Tracef("🔌 Closing PeerConnection manager")
		s.peerConnectionManager.Close()
	}

	// 清空客户端映射
	s.clients = make(map[string]*SignalingClient)
	s.apps = make(map[string]map[string]*SignalingClient)

	s.logger.Tracef("✅ Signaling server stopped successfully")
}

// HandleWebSocket 处理WebSocket连接
func (s *SignalingServer) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.logger.Infof("WebSocket upgrade failed: %v", err)
		return
	}

	s.HandleWebSocketConnection(conn)
}

// HandleWebSocketConnection 处理WebSocket连接
func (s *SignalingServer) HandleWebSocketConnection(conn *websocket.Conn) {
	client := NewSignalingClient(conn, s, "default")

	s.logger.Infof("Creating new signaling client: %s from %s (User-Agent: %s)",
		client.ID, client.RemoteAddr, client.UserAgent)

	// 设置连接状态为连接中
	client.setState(ClientStateConnecting)

	// 先注册客户端
	s.register <- client

	// 设置连接状态为已连接
	client.setState(ClientStateConnected)
	s.logger.Infof("Client %s successfully connected and welcomed", client.ID)

	// 启动读写协程
	go client.writePump()
	go client.readPump()
}

// HandleAppConnection 处理特定应用的连接
func (s *SignalingServer) HandleAppConnection(conn *websocket.Conn, appName string) {
	client := NewSignalingClient(conn, s, appName)

	s.logger.Infof("Creating new signaling client for app %s: %s", appName, client.ID)

	// 先启动读写协程，再注册客户端
	go client.writePump()
	go client.readPump()

	// 注册客户端
	s.register <- client

	// 发送欢迎消息
	welcome := SignalingMessage{
		Type:   "welcome",
		PeerID: client.ID,
		Data: map[string]interface{}{
			"client_id": client.ID,
			"app_name":  client.AppName,
		},
	}

	if welcomeData, err := json.Marshal(welcome); err == nil {
		select {
		case client.Send <- welcomeData:
			s.logger.Infof("Welcome message sent to client %s (app: %s)", client.ID, appName)
		default:
			s.logger.Infof("Failed to send welcome message to client %s: channel full", client.ID)
		}
	} else {
		s.logger.Infof("Failed to marshal welcome message for client %s: %v", client.ID, err)
	}
}

// HandleSignaling 处理特定应用的信令连接
func (s *SignalingServer) HandleSignaling(w http.ResponseWriter, r *http.Request, appName string) {
	s.HandleWebSocket(w, r)
}

// registerClient 注册客户端
func (s *SignalingServer) registerClient(client *SignalingClient) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.clients[client.ID] = client

	// 按应用分组
	if s.apps[client.AppName] == nil {
		s.apps[client.AppName] = make(map[string]*SignalingClient)
	}
	s.apps[client.AppName][client.ID] = client

	totalClients := len(s.clients)
	appClients := len(s.apps[client.AppName])

	// 记录连接事件到性能监控器
	if s.performanceMonitor != nil {
		s.performanceMonitor.RecordConnectionEvent("connect", client.ID, client.AppName)
	}

	s.logger.Infof("✅ Client %s registered for app %s (total clients: %d, app clients: %d)",
		client.ID, client.AppName, totalClients, appClients)
}

// unregisterClient 注销客户端
func (s *SignalingServer) unregisterClient(client *SignalingClient) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if _, ok := s.clients[client.ID]; ok {
		delete(s.clients, client.ID)

		// 从应用分组中移除
		if appClients, exists := s.apps[client.AppName]; exists {
			delete(appClients, client.ID)
			if len(appClients) == 0 {
				delete(s.apps, client.AppName)
			}
		}

		close(client.Send)

		totalClients := len(s.clients)
		connectionDuration := time.Since(client.ConnectedAt)

		// 记录断开连接事件到性能监控器
		if s.performanceMonitor != nil {
			s.performanceMonitor.RecordConnectionEvent("disconnect", client.ID, client.AppName)
		}

		s.logger.Infof("❌ Client %s unregistered from app %s (connected for: %v, messages: %d, errors: %d, remaining clients: %d)",
			client.ID, client.AppName, connectionDuration, client.MessageCount, client.ErrorCount, totalClients)
	}
}

// broadcastMessage 广播消息
func (s *SignalingServer) broadcastMessage(message []byte) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	for _, client := range s.clients {
		select {
		case client.Send <- message:
			s.logger.Debugf("📤 Message broadcasted to client %s", client.ID)
		default:
			s.logger.Debugf("❌ Failed to broadcast to client %s: channel full, closing connection", client.ID)
			close(client.Send)
			delete(s.clients, client.ID)
		}
	}
}

// BroadcastToApp 向特定应用广播消息
func (s *SignalingServer) BroadcastToApp(appName string, message SignalingMessage) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	if appClients, exists := s.apps[appName]; exists {
		messageBytes, err := json.Marshal(message)
		if err != nil {
			s.logger.Infof("Failed to marshal message: %v", err)
			return
		}

		for _, client := range appClients {
			select {
			case client.Send <- messageBytes:
				s.logger.Debugf("📤 Message broadcasted to client %s in app %s", client.ID, appName)
			default:
				s.logger.Debugf("❌ Failed to broadcast to client %s: channel full, closing connection", client.ID)
				close(client.Send)
				delete(s.clients, client.ID)
				delete(appClients, client.ID)
			}
		}
	}
}

// SendToClient 向特定客户端发送消息
func (s *SignalingServer) SendToClient(clientID string, message SignalingMessage) error {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	if client, exists := s.clients[clientID]; exists {
		messageBytes, err := json.Marshal(message)
		if err != nil {
			return err
		}

		select {
		case client.Send <- messageBytes:
			return nil
		default:
			return ErrSignalingSendChannelFull
		}
	}

	return ErrSignalingClientNotFound
}

// GetClientCount 获取客户端数量
func (s *SignalingServer) GetClientCount() int {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	return len(s.clients)
}

// GetAppClientCount 获取特定应用的客户端数量
func (s *SignalingServer) GetAppClientCount(appName string) int {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	if appClients, exists := s.apps[appName]; exists {
		return len(appClients)
	}

	return 0
}

// GetPerformanceStats 获取性能统计信息
func (s *SignalingServer) GetPerformanceStats() map[string]interface{} {
	if s.performanceMonitor == nil {
		return map[string]interface{}{
			"error": "Performance monitor not available",
		}
	}

	return s.performanceMonitor.GetStats()
}

// GetMessageStats 获取消息处理统计
func (s *SignalingServer) GetMessageStats() *MessageProcessingStats {
	if s.performanceMonitor == nil {
		return nil
	}

	return s.performanceMonitor.GetMessageStats()
}

// GetConnectionStats 获取连接统计
func (s *SignalingServer) GetConnectionStats() *ConnectionStats {
	if s.performanceMonitor == nil {
		return nil
	}

	return s.performanceMonitor.GetConnectionStats()
}

// GetSystemStats 获取系统统计
func (s *SignalingServer) GetSystemStats() *SystemStats {
	if s.performanceMonitor == nil {
		return nil
	}

	return s.performanceMonitor.GetSystemStats()
}

// GetConcurrentRouterStats 获取并发路由器统计
func (s *SignalingServer) GetConcurrentRouterStats() *ConcurrentRoutingStats {
	if s.concurrentRouter == nil {
		return nil
	}

	return s.concurrentRouter.GetStats()
}

// GetDetailedPerformanceReport 获取详细性能报告
func (s *SignalingServer) GetDetailedPerformanceReport() map[string]interface{} {
	report := make(map[string]interface{})

	// 基础服务器统计
	s.mutex.RLock()
	report["server_stats"] = map[string]interface{}{
		"total_clients": len(s.clients),
		"total_apps":    len(s.apps),
		"running":       s.running,
	}

	// 按应用的客户端统计
	appStats := make(map[string]interface{})
	for appName, clients := range s.apps {
		appStats[appName] = len(clients)
	}
	report["app_client_counts"] = appStats
	s.mutex.RUnlock()

	// 性能监控统计
	if s.performanceMonitor != nil {
		report["performance_monitor"] = s.performanceMonitor.GetStats()
	}

	// 并发路由器统计
	if s.concurrentRouter != nil {
		report["concurrent_router"] = s.concurrentRouter.GetDetailedStats()
	}

	// PeerConnection管理器统计
	if s.peerConnectionManager != nil {
		report["peer_connection_manager"] = s.peerConnectionManager.GetMetrics()
	}

	// 消息路由器统计
	if s.messageRouter != nil {
		report["message_router"] = s.messageRouter.GetStats()
	}

	return report
}

// ResetPerformanceStats 重置性能统计
func (s *SignalingServer) ResetPerformanceStats() {
	if s.performanceMonitor != nil {
		s.performanceMonitor.ResetStats()
		s.logger.Infof("✅ Performance statistics reset")
	}
}

// cleanupRoutine 清理过期连接
func (s *SignalingServer) cleanupRoutine() {
	ticker := time.NewTicker(2 * time.Minute) // 减少清理频率到2分钟一次
	defer ticker.Stop()

	for s.running {
		select {
		case <-ticker.C:
			s.cleanupExpiredClients()
		case <-s.ctx.Done():
			s.logger.Infof("Signaling server cleanup routine shutting down")
			return
		}
	}
}

// cleanupExpiredClients 清理过期客户端
func (s *SignalingServer) cleanupExpiredClients() {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	now := time.Now()
	expiredClients := make([]*SignalingClient, 0)
	totalClients := len(s.clients)

	for _, client := range s.clients {
		// 增加过期时间到10分钟，避免过早清理活跃连接
		if now.Sub(client.LastSeen) > 10*time.Minute {
			expiredClients = append(expiredClients, client)
		}
	}

	if len(expiredClients) > 0 {
		s.logger.Infof("Cleaning up %d expired clients out of %d total clients", len(expiredClients), totalClients)
	}

	for _, client := range expiredClients {
		lastSeenDuration := now.Sub(client.LastSeen)
		connectionDuration := now.Sub(client.ConnectedAt)

		s.logger.Infof("Cleaning up expired client: %s (last seen: %v ago, connected for: %v, messages: %d, errors: %d)",
			client.ID, lastSeenDuration, connectionDuration, client.MessageCount, client.ErrorCount)

		// 记录清理原因
		client.recordError(&SignalingError{
			Code:    ErrorCodeConnectionTimeout,
			Message: "Connection expired due to inactivity",
			Details: fmt.Sprintf("Last seen %v ago", lastSeenDuration),
			Type:    "cleanup_info",
		})

		// 发送关闭消息而不是直接注销
		client.Conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseGoingAway, "Connection expired"))
		s.unregisterClient(client)
	}
}

// validateSignalingMessage 验证信令消息
func validateSignalingMessage(message *SignalingMessage) *SignalingError {
	if message == nil {
		return &SignalingError{
			Code:    ErrorCodeInvalidMessage,
			Message: "Message is null",
			Type:    "validation_error",
		}
	}

	// 验证消息类型
	if message.Type == "" {
		return &SignalingError{
			Code:    ErrorCodeInvalidMessageType,
			Message: "Message type is required",
			Type:    "validation_error",
		}
	}

	// 验证已知的消息类型
	validTypes := map[string]bool{
		"ping":          true,
		"pong":          true,
		"request-offer": true,
		"offer":         true,
		"answer":        true,
		"ice-candidate": true,
		"mouse-click":   true,
		"mouse-move":    true,
		"key-press":     true,
		"get-stats":     true,
		"error":         true,
		"welcome":       true,
		"stats":         true,
		// selkies 协议支持
		"hello": true,
		"sdp":   true,
		"ice":   true,
		// 协议协商支持
		"protocol-negotiation":          true,
		"protocol-negotiation-response": true,
	}

	if !validTypes[message.Type] {
		return &SignalingError{
			Code:    ErrorCodeInvalidMessageType,
			Message: fmt.Sprintf("Unknown message type: %s", message.Type),
			Details: "Supported types: ping, pong, request-offer, offer, answer, ice-candidate, mouse-click, mouse-move, key-press, get-stats",
			Type:    "validation_error",
		}
	}

	// 验证特定消息类型的数据
	switch message.Type {
	case "answer":
		if err := validateAnswerData(message.Data); err != nil {
			return err
		}
	case "ice-candidate":
		if err := validateICECandidateData(message.Data); err != nil {
			return err
		}
	case "mouse-click", "mouse-move":
		if err := validateMouseEventData(message.Data); err != nil {
			return err
		}
	case "key-press":
		if err := validateKeyEventData(message.Data); err != nil {
			return err
		}
	}

	return nil
}

// validateAnswerData 验证answer消息数据
func validateAnswerData(data any) *SignalingError {
	if data == nil {
		return &SignalingError{
			Code:    ErrorCodeInvalidMessageData,
			Message: "Answer data is required",
			Type:    "validation_error",
		}
	}

	answerData, ok := data.(map[string]any)
	if !ok {
		return &SignalingError{
			Code:    ErrorCodeInvalidMessageData,
			Message: "Answer data must be an object",
			Type:    "validation_error",
		}
	}

	sdp, exists := answerData["sdp"]
	if !exists {
		return &SignalingError{
			Code:    ErrorCodeInvalidMessageData,
			Message: "Answer must contain 'sdp' field",
			Type:    "validation_error",
		}
	}

	sdpStr, ok := sdp.(string)
	if !ok || sdpStr == "" {
		return &SignalingError{
			Code:    ErrorCodeInvalidMessageData,
			Message: "SDP must be a non-empty string",
			Type:    "validation_error",
		}
	}

	return nil
}

// validateICECandidateData 验证ICE候选数据
func validateICECandidateData(data any) *SignalingError {
	if data == nil {
		return &SignalingError{
			Code:    ErrorCodeInvalidMessageData,
			Message: "ICE candidate data is required",
			Type:    "validation_error",
		}
	}

	candidateData, ok := data.(map[string]any)
	if !ok {
		return &SignalingError{
			Code:    ErrorCodeInvalidMessageData,
			Message: "ICE candidate data must be an object",
			Type:    "validation_error",
		}
	}

	candidate, exists := candidateData["candidate"]
	if !exists {
		return &SignalingError{
			Code:    ErrorCodeInvalidMessageData,
			Message: "ICE candidate must contain 'candidate' field",
			Type:    "validation_error",
		}
	}

	candidateStr, ok := candidate.(string)
	if !ok || candidateStr == "" {
		return &SignalingError{
			Code:    ErrorCodeInvalidMessageData,
			Message: "Candidate must be a non-empty string",
			Type:    "validation_error",
		}
	}

	return nil
}

// validateMouseEventData 验证鼠标事件数据
func validateMouseEventData(data any) *SignalingError {
	if data == nil {
		return nil // 鼠标事件数据可以为空
	}

	eventData, ok := data.(map[string]any)
	if !ok {
		return &SignalingError{
			Code:    ErrorCodeInvalidMessageData,
			Message: "Mouse event data must be an object",
			Type:    "validation_error",
		}
	}

	// 验证坐标
	if x, exists := eventData["x"]; exists {
		if _, ok := x.(float64); !ok {
			return &SignalingError{
				Code:    ErrorCodeInvalidMessageData,
				Message: "Mouse event x coordinate must be a number",
				Type:    "validation_error",
			}
		}
	}

	if y, exists := eventData["y"]; exists {
		if _, ok := y.(float64); !ok {
			return &SignalingError{
				Code:    ErrorCodeInvalidMessageData,
				Message: "Mouse event y coordinate must be a number",
				Type:    "validation_error",
			}
		}
	}

	return nil
}

// validateKeyEventData 验证键盘事件数据
func validateKeyEventData(data any) *SignalingError {
	if data == nil {
		return &SignalingError{
			Code:    ErrorCodeInvalidMessageData,
			Message: "Key event data is required",
			Type:    "validation_error",
		}
	}

	eventData, ok := data.(map[string]any)
	if !ok {
		return &SignalingError{
			Code:    ErrorCodeInvalidMessageData,
			Message: "Key event data must be an object",
			Type:    "validation_error",
		}
	}

	// 验证按键代码或键值
	if key, exists := eventData["key"]; exists {
		if _, ok := key.(string); !ok {
			return &SignalingError{
				Code:    ErrorCodeInvalidMessageData,
				Message: "Key event key must be a string",
				Type:    "validation_error",
			}
		}
	}

	return nil
}

// 错误定义
var (
	ErrSignalingClientNotFound  = errors.New("signaling client not found")
	ErrSignalingSendChannelFull = errors.New("signaling send channel is full")
)

// 信令错误代码常量
const (
	// 连接相关错误
	ErrorCodeConnectionFailed  = "CONNECTION_FAILED"
	ErrorCodeConnectionTimeout = "CONNECTION_TIMEOUT"
	ErrorCodeConnectionClosed  = "CONNECTION_CLOSED"
	ErrorCodeConnectionLost    = "CONNECTION_LOST"

	// 消息相关错误
	ErrorCodeInvalidMessage       = "INVALID_MESSAGE"
	ErrorCodeInvalidMessageType   = "INVALID_MESSAGE_TYPE"
	ErrorCodeInvalidMessageData   = "INVALID_MESSAGE_DATA"
	ErrorCodeMessageTooLarge      = "MESSAGE_TOO_LARGE"
	ErrorCodeMessageParsingFailed = "MESSAGE_PARSING_FAILED"

	// WebRTC相关错误
	ErrorCodePeerConnectionFailed   = "PEER_CONNECTION_FAILED"
	ErrorCodeOfferCreationFailed    = "OFFER_CREATION_FAILED"
	ErrorCodeAnswerProcessingFailed = "ANSWER_PROCESSING_FAILED"
	ErrorCodeICECandidateFailed     = "ICE_CANDIDATE_FAILED"
	ErrorCodeSDPProcessingFailed    = "SDP_PROCESSING_FAILED"

	// 服务器相关错误
	ErrorCodeServerUnavailable = "SERVER_UNAVAILABLE"
	ErrorCodeInternalError     = "INTERNAL_ERROR"
	ErrorCodeClientNotFound    = "CLIENT_NOT_FOUND"
	ErrorCodeChannelFull       = "CHANNEL_FULL"
	ErrorCodeRateLimited       = "RATE_LIMITED"
)

// 消息验证相关常量
const (
	MaxMessageSize = 64 * 1024 // 64KB 最大消息大小
	MaxDataSize    = 32 * 1024 // 32KB 最大数据大小
)
