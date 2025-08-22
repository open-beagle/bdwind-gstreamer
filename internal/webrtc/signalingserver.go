package webrtc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v4"

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

// SignalingClient 信令客户端
type SignalingClient struct {
	ID               string
	AppName          string
	Conn             *websocket.Conn
	Send             chan []byte
	Server           *SignalingServer
	LastSeen         time.Time
	ConnectedAt      time.Time
	RemoteAddr       string
	UserAgent        string
	State            ClientState
	MessageCount     int64
	ErrorCount       int64
	LastError        *SignalingError
	IsSelkies        bool                     // 标记是否为 selkies 客户端
	Protocol         protocol.ProtocolVersion // 客户端使用的协议版本
	ProtocolDetected bool                     // 是否已检测到协议
	mutex            sync.RWMutex
}

// ClientState 客户端连接状态
type ClientState string

const (
	ClientStateConnecting    ClientState = "connecting"
	ClientStateConnected     ClientState = "connected"
	ClientStateDisconnecting ClientState = "disconnecting"
	ClientStateDisconnected  ClientState = "disconnected"
	ClientStateError         ClientState = "error"
)

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
		pcManager = NewPeerConnectionManager(ms, iceServers, log.Default())
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

	log.Printf("✅ Signaling server created with integrated protocol adapters")
	log.Printf("📋 Supported protocols: %v", protocolManager.GetSupportedProtocols())

	return server
}

// setupEventHandlers 设置事件处理器
func (s *SignalingServer) setupEventHandlers() {
	// 客户端协议检测事件
	s.eventBus.Subscribe("client:protocol-detected", func(data any) {
		if eventData, ok := data.(map[string]any); ok {
			clientID := eventData["client_id"].(string)
			protocol := eventData["protocol"].(protocol.ProtocolVersion)
			log.Printf("📡 Event: Client %s protocol detected as %s", clientID, protocol)
		}
	})

	// 协议降级事件
	s.eventBus.Subscribe("client:protocol-downgraded", func(data any) {
		if eventData, ok := data.(map[string]any); ok {
			clientID := eventData["client_id"].(string)
			fromProtocol := eventData["from_protocol"].(protocol.ProtocolVersion)
			toProtocol := eventData["to_protocol"].(protocol.ProtocolVersion)
			reason := eventData["reason"].(string)
			log.Printf("⬇️ Event: Client %s protocol downgraded from %s to %s (reason: %s)",
				clientID, fromProtocol, toProtocol, reason)
		}
	})

	// 连接状态变化事件
	s.eventBus.Subscribe("client:state-changed", func(data any) {
		if eventData, ok := data.(map[string]any); ok {
			clientID := eventData["client_id"].(string)
			oldState := eventData["old_state"].(ClientState)
			newState := eventData["new_state"].(ClientState)
			log.Printf("🔄 Event: Client %s state changed from %s to %s", clientID, oldState, newState)
		}
	})

	// 消息处理错误事件
	s.eventBus.Subscribe("message:processing-error", func(data any) {
		if eventData, ok := data.(map[string]any); ok {
			clientID := eventData["client_id"].(string)
			errorType := eventData["error_type"].(string)
			errorMessage := eventData["error_message"].(string)
			log.Printf("❌ Event: Message processing error for client %s: %s - %s",
				clientID, errorType, errorMessage)
		}
	})
}

// Start 启动信令服务器
func (s *SignalingServer) Start() {
	log.Printf("🚀 Starting signaling server...")
	s.running = true

	// 启动性能监控器
	if s.performanceMonitor != nil {
		s.performanceMonitor.Start()
		log.Printf("✅ Performance monitor started")

		// 添加性能警报回调
		s.performanceMonitor.AddAlertCallback(func(alert *PerformanceAlert) {
			log.Printf("🚨 Performance Alert: %s - %s", alert.Type, alert.Message)
			// 可以在这里添加更多的警报处理逻辑，如发送通知等
		})
	}

	// 启动并发消息路由器
	if s.concurrentRouter != nil {
		if err := s.concurrentRouter.Start(); err != nil {
			log.Printf("❌ Failed to start concurrent router: %v", err)
		} else {
			log.Printf("✅ Concurrent message router started")
		}
	}

	// 启动清理协程
	go s.cleanupRoutine()
	log.Printf("✅ Signaling server cleanup routine started")

	log.Printf("✅ Signaling server started and ready to accept connections")

	for s.running {
		select {
		case client := <-s.register:
			log.Printf("📝 Processing client registration: %s", client.ID)
			s.registerClient(client)

		case client := <-s.unregister:
			log.Printf("📝 Processing client unregistration: %s", client.ID)
			s.unregisterClient(client)

		case message := <-s.broadcast:
			log.Printf("📢 Processing broadcast message (length: %d bytes)", len(message))
			s.broadcastMessage(message)

		case <-s.ctx.Done():
			log.Printf("🛑 Signaling server received shutdown signal")
			s.Stop()
			return
		}
	}
}

// Stop 停止信令服务器
func (s *SignalingServer) Stop() {
	log.Printf("🛑 Stopping signaling server...")
	s.running = false
	s.cancel()

	// 停止并发消息路由器
	if s.concurrentRouter != nil {
		if err := s.concurrentRouter.Stop(); err != nil {
			log.Printf("❌ Error stopping concurrent router: %v", err)
		} else {
			log.Printf("✅ Concurrent message router stopped")
		}
	}

	// 停止性能监控器
	if s.performanceMonitor != nil {
		s.performanceMonitor.Stop()
		log.Printf("✅ Performance monitor stopped")
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()

	clientCount := len(s.clients)
	log.Printf("📊 Closing %d active client connections", clientCount)

	// 关闭所有客户端连接
	for clientID, client := range s.clients {
		log.Printf("🔌 Closing connection for client %s", clientID)
		close(client.Send)
	}

	// 关闭所有PeerConnection
	if s.peerConnectionManager != nil {
		log.Printf("🔌 Closing PeerConnection manager")
		s.peerConnectionManager.Close()
	}

	// 清空客户端映射
	s.clients = make(map[string]*SignalingClient)
	s.apps = make(map[string]map[string]*SignalingClient)

	log.Printf("✅ Signaling server stopped successfully")
}

// HandleWebSocket 处理WebSocket连接
func (s *SignalingServer) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}

	s.HandleWebSocketConnection(conn)
}

// HandleWebSocketConnection 处理WebSocket连接
func (s *SignalingServer) HandleWebSocketConnection(conn *websocket.Conn) {
	now := time.Now()
	client := &SignalingClient{
		ID:           generateSignalingClientID(),
		AppName:      "default",
		Conn:         conn,
		Send:         make(chan []byte, 256),
		Server:       s,
		LastSeen:     now,
		ConnectedAt:  now,
		RemoteAddr:   conn.RemoteAddr().String(),
		UserAgent:    conn.Subprotocol(), // 可以从请求头获取更详细信息
		State:        ClientStateConnecting,
		MessageCount: 0,
		ErrorCount:   0,
	}

	log.Printf("Creating new signaling client: %s from %s (User-Agent: %s)",
		client.ID, client.RemoteAddr, client.UserAgent)

	// 设置连接状态为连接中
	client.setState(ClientStateConnecting)

	// 先注册客户端
	s.register <- client

	// 发送欢迎消息
	welcome := SignalingMessage{
		Type:      "welcome",
		PeerID:    client.ID,
		MessageID: generateMessageID(),
		Timestamp: now.Unix(),
		Data: map[string]any{
			"client_id":    client.ID,
			"app_name":     client.AppName,
			"server_time":  now.Unix(),
			"capabilities": []string{"webrtc", "input", "stats"},
		},
	}

	if err := client.sendMessage(welcome); err != nil {
		log.Printf("Failed to send welcome message to client %s: %v", client.ID, err)
		client.recordError(&SignalingError{
			Code:    ErrorCodeConnectionFailed,
			Message: "Failed to send welcome message",
			Details: err.Error(),
			Type:    "connection_error",
		})
		conn.Close()
		return
	}

	// 设置连接状态为已连接
	client.setState(ClientStateConnected)
	log.Printf("Client %s successfully connected and welcomed", client.ID)

	// 启动读写协程
	go client.writePump()
	go client.readPump()
}

// HandleAppConnection 处理特定应用的连接
func (s *SignalingServer) HandleAppConnection(conn *websocket.Conn, appName string) {
	client := &SignalingClient{
		ID:       generateSignalingClientID(),
		AppName:  appName,
		Conn:     conn,
		Send:     make(chan []byte, 256),
		Server:   s,
		LastSeen: time.Now(),
	}

	log.Printf("Creating new signaling client for app %s: %s", appName, client.ID)

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
			log.Printf("Welcome message sent to client %s (app: %s)", client.ID, appName)
		default:
			log.Printf("Failed to send welcome message to client %s: channel full", client.ID)
		}
	} else {
		log.Printf("Failed to marshal welcome message for client %s: %v", client.ID, err)
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

	log.Printf("✅ Client %s registered for app %s (total clients: %d, app clients: %d)",
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

		log.Printf("❌ Client %s unregistered from app %s (connected for: %v, messages: %d, errors: %d, remaining clients: %d)",
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
		default:
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
			log.Printf("Failed to marshal message: %v", err)
			return
		}

		for _, client := range appClients {
			select {
			case client.Send <- messageBytes:
			default:
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
		log.Printf("✅ Performance statistics reset")
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
			log.Printf("Signaling server cleanup routine shutting down")
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
		log.Printf("Cleaning up %d expired clients out of %d total clients", len(expiredClients), totalClients)
	}

	for _, client := range expiredClients {
		lastSeenDuration := now.Sub(client.LastSeen)
		connectionDuration := now.Sub(client.ConnectedAt)

		log.Printf("Cleaning up expired client: %s (last seen: %v ago, connected for: %v, messages: %d, errors: %d)",
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

// generateSignalingClientID 生成客户端ID
func generateSignalingClientID() string {
	return fmt.Sprintf("client_%d_%d", time.Now().UnixNano(), rand.Intn(10000))
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
		"answer-ack":    true,
		"ice-ack":       true,
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
			Details: "Supported types: ping, pong, request-offer, offer, answer, ice-candidate, answer-ack, ice-ack, mouse-click, mouse-move, key-press, get-stats",
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

// setState 设置客户端状态
func (c *SignalingClient) setState(state ClientState) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	oldState := c.State
	c.State = state

	if oldState != state {
		log.Printf("Client %s state changed: %s -> %s", c.ID, oldState, state)
	}
}

// getState 获取客户端状态
func (c *SignalingClient) getState() ClientState {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.State
}

// recordError 记录错误
func (c *SignalingClient) recordError(err *SignalingError) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.ErrorCount++
	c.LastError = err

	log.Printf("Client %s error recorded: %s - %s", c.ID, err.Code, err.Message)
}

// incrementMessageCount 增加消息计数
func (c *SignalingClient) incrementMessageCount() {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.MessageCount++
}

// sendMessage 发送消息给客户端
func (c *SignalingClient) sendMessage(message SignalingMessage) error {
	messageBytes, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	if len(messageBytes) > MaxMessageSize {
		return fmt.Errorf("message too large: %d bytes (max: %d)", len(messageBytes), MaxMessageSize)
	}

	select {
	case c.Send <- messageBytes:
		return nil
	default:
		return ErrSignalingSendChannelFull
	}
}

// sendStandardMessage 发送标准化消息给客户端
func (c *SignalingClient) sendStandardMessage(message *protocol.StandardMessage) error {
	if message == nil {
		return fmt.Errorf("message is nil")
	}

	// 获取客户端协议版本
	c.mutex.RLock()
	clientProtocol := c.Protocol
	c.mutex.RUnlock()

	// 如果未检测到协议，使用默认协议
	if clientProtocol == "" {
		clientProtocol = protocol.ProtocolVersionGStreamer10
	}

	// 使用消息路由器格式化消息
	messageBytes, err := c.Server.messageRouter.FormatResponse(message, clientProtocol)
	if err != nil {
		return fmt.Errorf("failed to format standard message: %w", err)
	}

	if len(messageBytes) > MaxMessageSize {
		return fmt.Errorf("message too large: %d bytes (max: %d)", len(messageBytes), MaxMessageSize)
	}

	select {
	case c.Send <- messageBytes:
		log.Printf("📤 Standard message sent to client %s: type=%s, protocol=%s",
			c.ID, message.Type, clientProtocol)
		return nil
	default:
		return ErrSignalingSendChannelFull
	}
}

// sendError 发送错误消息给客户端
func (c *SignalingClient) sendError(signalingError *SignalingError) {
	errorMessage := SignalingMessage{
		Type:      "error",
		PeerID:    c.ID,
		MessageID: generateMessageID(),
		Timestamp: time.Now().Unix(),
		Error:     signalingError,
	}

	if err := c.sendMessage(errorMessage); err != nil {
		log.Printf("Failed to send error message to client %s: %v", c.ID, err)
	}
}

// generateMessageID 生成消息ID
func generateMessageID() string {
	return fmt.Sprintf("msg_%d_%d", time.Now().UnixNano(), rand.Intn(1000))
}

// getDataSize 获取消息数据大小
func getDataSize(data any) int {
	if data == nil {
		return 0
	}

	if dataBytes, err := json.Marshal(data); err == nil {
		return len(dataBytes)
	}

	return 0
}

// getSDPLength 获取SDP长度
func getSDPLength(data any) int {
	if data == nil {
		return 0
	}

	if dataMap, ok := data.(map[string]any); ok {
		if sdp, exists := dataMap["sdp"]; exists {
			if sdpStr, ok := sdp.(string); ok {
				return len(sdpStr)
			}
		}
	}

	return 0
}

// getICECandidateInfo 获取ICE候选信息
func getICECandidateInfo(data any) string {
	if data == nil {
		return "unknown"
	}

	if dataMap, ok := data.(map[string]any); ok {
		if candidate, exists := dataMap["candidate"]; exists {
			if candidateStr, ok := candidate.(string); ok {
				// 提取候选类型和地址信息
				parts := strings.Fields(candidateStr)
				if len(parts) >= 8 {
					return fmt.Sprintf("type=%s, protocol=%s, addr=%s:%s", parts[7], parts[2], parts[4], parts[5])
				}
				return candidateStr[:min(50, len(candidateStr))] + "..."
			}
		}
	}

	return "invalid"
}

// min 返回两个整数中的较小值
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// getLastErrorInfo 获取最后一个错误的信息
func (c *SignalingClient) getLastErrorInfo() map[string]any {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	if c.LastError == nil {
		return nil
	}

	return map[string]any{
		"code":    c.LastError.Code,
		"message": c.LastError.Message,
		"details": c.LastError.Details,
		"type":    c.LastError.Type,
	}
}

// recordMessageProcessingMetrics 记录消息处理性能指标
func (c *SignalingClient) recordMessageProcessingMetrics(messageType string, processingTime time.Duration, success bool) {
	// 记录处理时间统计
	log.Printf("📊 Message processing metrics for client %s - Type: %s, Duration: %dms, Success: %t",
		c.ID, messageType, processingTime.Milliseconds(), success)

	// 如果处理时间过长，记录警告
	if processingTime > 1*time.Second {
		log.Printf("⚠️ Slow message processing detected for client %s - Type: %s, Duration: %dms",
			c.ID, messageType, processingTime.Milliseconds())
	}

	// 更新客户端统计信息
	c.mutex.Lock()
	if !success {
		c.ErrorCount++
	}
	c.mutex.Unlock()

	// 发送性能指标事件
	if c.Server.eventBus != nil {
		c.Server.eventBus.Emit("message:processing-metrics", map[string]any{
			"client_id":       c.ID,
			"message_type":    messageType,
			"processing_time": processingTime.Milliseconds(),
			"success":         success,
			"timestamp":       time.Now().Unix(),
		})
	}
}

// readPump 读取客户端消息
func (c *SignalingClient) readPump() {
	defer func() {
		c.setState(ClientStateDisconnected)
		log.Printf("Client %s read pump exiting (connected for %v, messages: %d, errors: %d)",
			c.ID, time.Since(c.ConnectedAt), c.MessageCount, c.ErrorCount)
		c.Server.unregister <- c
		c.Conn.Close()
	}()

	// 设置读取超时为更长时间，避免频繁超时
	c.Conn.SetReadDeadline(time.Now().Add(300 * time.Second)) // 5分钟
	c.Conn.SetPongHandler(func(string) error {
		c.Conn.SetReadDeadline(time.Now().Add(300 * time.Second))
		c.LastSeen = time.Now()
		log.Printf("🏓 Pong received from client %s", c.ID)
		return nil
	})

	log.Printf("Client %s read pump started", c.ID)

	for {
		messageType, messageBytes, err := c.Conn.ReadMessage()
		if err != nil {
			// 详细的错误处理和记录
			var signalingError *SignalingError

			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure, websocket.CloseNormalClosure) {
				log.Printf("WebSocket unexpected close error for client %s: %v", c.ID, err)
				signalingError = &SignalingError{
					Code:    ErrorCodeConnectionLost,
					Message: "WebSocket connection lost unexpectedly",
					Details: err.Error(),
					Type:    "connection_error",
				}
			} else if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				log.Printf("WebSocket connection closed normally for client %s: %v", c.ID, err)
				signalingError = &SignalingError{
					Code:    ErrorCodeConnectionClosed,
					Message: "WebSocket connection closed",
					Details: err.Error(),
					Type:    "connection_info",
				}
			} else {
				log.Printf("WebSocket read error for client %s: %v", c.ID, err)
				signalingError = &SignalingError{
					Code:    ErrorCodeConnectionFailed,
					Message: "WebSocket read error",
					Details: err.Error(),
					Type:    "connection_error",
				}
			}

			c.recordError(signalingError)
			break
		}

		c.LastSeen = time.Now()

		// 检查消息大小
		if len(messageBytes) > MaxMessageSize {
			log.Printf("Message too large from client %s: %d bytes (max: %d)", c.ID, len(messageBytes), MaxMessageSize)
			signalingError := &SignalingError{
				Code:    ErrorCodeMessageTooLarge,
				Message: fmt.Sprintf("Message too large: %d bytes (max: %d)", len(messageBytes), MaxMessageSize),
				Type:    "validation_error",
			}
			c.recordError(signalingError)
			c.sendError(signalingError)
			continue
		}

		// 只处理文本消息
		if messageType == websocket.TextMessage {
			log.Printf("📨 Raw message received from client %s (length: %d bytes)", c.ID, len(messageBytes))

			// 增加消息计数
			c.incrementMessageCount()

			// 使用消息路由器处理消息
			c.handleMessageWithRouter(messageBytes)
		} else {
			log.Printf("Received non-text message from client %s (type: %d, length: %d)", c.ID, messageType, len(messageBytes))
			signalingError := &SignalingError{
				Code:    ErrorCodeInvalidMessage,
				Message: "Only text messages are supported",
				Details: fmt.Sprintf("Received message type: %d", messageType),
				Type:    "validation_error",
			}
			c.recordError(signalingError)
			c.sendError(signalingError)
		}
	}
}

// writePump 向客户端发送消息
func (c *SignalingClient) writePump() {
	ticker := time.NewTicker(240 * time.Second) // 4分钟ping一次，避免过于频繁
	defer func() {
		c.setState(ClientStateDisconnecting)
		log.Printf("Client %s write pump exiting", c.ID)
		ticker.Stop()
		c.Conn.Close()
	}()

	log.Printf("Client %s write pump started", c.ID)

	for {
		select {
		case message, ok := <-c.Send:
			c.Conn.SetWriteDeadline(time.Now().Add(30 * time.Second)) // 增加写入超时时间
			if !ok {
				log.Printf("Send channel closed for client %s", c.ID)
				c.Conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "Server shutting down"))
				return
			}

			if err := c.Conn.WriteMessage(websocket.TextMessage, message); err != nil {
				log.Printf("❌ WebSocket write error for client %s: %v", c.ID, err)
				signalingError := &SignalingError{
					Code:    ErrorCodeConnectionFailed,
					Message: "WebSocket write error",
					Details: err.Error(),
					Type:    "connection_error",
				}
				c.recordError(signalingError)
				return
			}

			log.Printf("📤 Message sent to client %s (length: %d bytes)", c.ID, len(message))

		case <-ticker.C:
			c.Conn.SetWriteDeadline(time.Now().Add(30 * time.Second))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				log.Printf("❌ WebSocket ping failed for client %s: %v", c.ID, err)
				signalingError := &SignalingError{
					Code:    ErrorCodeConnectionTimeout,
					Message: "WebSocket ping failed",
					Details: err.Error(),
					Type:    "connection_error",
				}
				c.recordError(signalingError)
				return
			}
			log.Printf("🏓 Ping sent to client %s", c.ID)
		}
	}
}

// handleMessageWithRouter 使用消息路由器处理客户端消息
func (c *SignalingClient) handleMessageWithRouter(messageBytes []byte) {
	startTime := time.Now()

	// 如果是第一条消息且未检测协议，进行协议自动检测
	if c.MessageCount == 1 && !c.ProtocolDetected {
		c.autoDetectProtocol(messageBytes)
	}

	// 优先使用并发路由器（如果可用且启用）
	var routeResult *RouteResult
	var err error
	var routingMethod string

	if c.Server.concurrentRouter != nil && c.Server.performanceMonitor != nil &&
		c.Server.performanceMonitor.config.EnableConcurrentRouting {
		routingMethod = "concurrent"
		routeResult, err = c.Server.concurrentRouter.RouteMessage(messageBytes, c.ID)
	} else {
		routingMethod = "standard"
		routeResult, err = c.Server.messageRouter.RouteMessage(messageBytes, c.ID)
	}

	routingTime := time.Since(startTime)

	// 记录路由性能指标
	if c.Server.performanceMonitor != nil {
		c.Server.performanceMonitor.RecordRoutingStats(routingMethod, routingTime, err == nil)
	}

	if err != nil {
		log.Printf("❌ Failed to route message from client %s using %s router: %v", c.ID, routingMethod, err)

		// 如果并发路由失败，尝试标准路由作为回退
		if routingMethod == "concurrent" {
			log.Printf("🔄 Falling back to standard router for client %s", c.ID)
			fallbackStart := time.Now()
			routeResult, err = c.Server.messageRouter.RouteMessage(messageBytes, c.ID)
			fallbackTime := time.Since(fallbackStart)

			if c.Server.performanceMonitor != nil {
				c.Server.performanceMonitor.RecordRoutingStats("fallback", fallbackTime, err == nil)
			}

			if err != nil {
				log.Printf("❌ Fallback routing also failed for client %s: %v", c.ID, err)
				c.handleProtocolError("MESSAGE_ROUTING_FAILED", err.Error())
				return
			}
		} else {
			c.handleProtocolError("MESSAGE_ROUTING_FAILED", err.Error())
			return
		}
	}

	// 记录路由结果中的警告
	for _, warning := range routeResult.Warnings {
		log.Printf("⚠️ Message routing warning for client %s: %s", c.ID, warning)
	}

	// 处理标准化消息
	c.handleStandardMessage(routeResult.Message, routeResult.OriginalProtocol)
}

// autoDetectProtocol 自动检测客户端协议
func (c *SignalingClient) autoDetectProtocol(messageBytes []byte) {
	if c.Server.protocolNegotiator == nil {
		log.Printf("⚠️ Protocol negotiator not available for client %s", c.ID)
		return
	}

	// 使用协议协商器检测协议
	negotiationResult := c.Server.protocolNegotiator.DetectProtocol(messageBytes)

	c.mutex.Lock()
	c.Protocol = negotiationResult.SelectedProtocol
	c.ProtocolDetected = true
	c.mutex.Unlock()

	log.Printf("🔍 Protocol detected for client %s: %s (confidence: %.2f, method: %s)",
		c.ID, negotiationResult.SelectedProtocol, negotiationResult.Confidence, negotiationResult.DetectionMethod)

	// 如果使用了回退协议，记录警告
	if negotiationResult.FallbackUsed {
		log.Printf("⚠️ Client %s using fallback protocol: %s", c.ID, negotiationResult.SelectedProtocol)
	}

	// 触发协议检测事件
	if c.Server.eventBus != nil {
		c.Server.eventBus.Emit("client:protocol-detected", map[string]any{
			"client_id": c.ID,
			"protocol":  negotiationResult.SelectedProtocol,
			"result":    negotiationResult,
		})
	}
}

// handleStandardMessage 处理标准化消息
func (c *SignalingClient) handleStandardMessage(message *protocol.StandardMessage, originalProtocol protocol.ProtocolVersion) {
	if message == nil {
		log.Printf("❌ Received nil standard message from client %s", c.ID)
		return
	}

	startTime := time.Now()
	messageType := string(message.Type)

	log.Printf("📨 Processing standard message from client %s: type=%s, protocol=%s",
		c.ID, message.Type, originalProtocol)

	// 更新客户端最后活动时间
	c.LastSeen = time.Now()

	var success bool = true

	// 根据消息类型处理
	switch message.Type {
	case protocol.MessageTypeHello:
		c.handleHelloMessage(message)
	case protocol.MessageTypePing:
		c.handlePingMessage(message)
	case protocol.MessageTypeRequestOffer:
		c.handleRequestOfferMessage(message)
	case protocol.MessageTypeAnswer:
		c.handleAnswerMessage(message)
	case protocol.MessageTypeICECandidate:
		c.handleICECandidateMessage(message)
	case protocol.MessageType("protocol-negotiation"):
		c.handleProtocolNegotiationMessage(message)
	case protocol.MessageType("get-stats"):
		c.handleGetStatsMessage(message)
	case protocol.MessageType("mouse-click"), protocol.MessageType("mouse-move"), protocol.MessageType("key-press"):
		c.handleInputMessage(message)
	default:
		log.Printf("⚠️ Unhandled message type from client %s: %s", c.ID, message.Type)
		c.sendStandardErrorMessage("UNSUPPORTED_MESSAGE_TYPE",
			fmt.Sprintf("Message type '%s' is not supported", message.Type), "")
		success = false
	}

	// 记录消息处理性能指标
	processingTime := time.Since(startTime)

	// 使用现有的方法记录指标
	c.recordMessageProcessingMetrics(messageType, processingTime, success)

	// 同时记录到性能监控器
	if c.Server.performanceMonitor != nil {
		c.Server.performanceMonitor.RecordMessageProcessing(c.ID, messageType, processingTime, success)
	}
}

// handleHelloMessage 处理 HELLO 消息
func (c *SignalingClient) handleHelloMessage(message *protocol.StandardMessage) {
	log.Printf("👋 Received HELLO from client %s", c.ID)

	// 解析 HELLO 数据
	var helloData protocol.HelloData
	if err := message.GetDataAs(&helloData); err != nil {
		log.Printf("❌ Failed to parse HELLO data from client %s: %v", c.ID, err)
		c.sendStandardErrorMessage("INVALID_HELLO_DATA", "Failed to parse HELLO message data", err.Error())
		return
	}

	// 更新客户端信息
	if helloData.PeerID != "" {
		c.ID = helloData.PeerID
	}

	// 发送欢迎响应
	welcomeData := &protocol.HelloData{
		PeerID:       c.ID,
		Capabilities: []string{"webrtc", "input", "stats", "protocol-negotiation"},
		Metadata: map[string]any{
			"server_version": "1.0.0",
			"server_time":    time.Now().Unix(),
		},
	}

	welcomeMessage := c.Server.messageRouter.CreateStandardResponse(
		protocol.MessageTypeWelcome, c.ID, welcomeData)

	if err := c.sendStandardMessage(welcomeMessage); err != nil {
		log.Printf("❌ Failed to send welcome message to client %s: %v", c.ID, err)
	}
}

// handlePingMessage 处理 PING 消息
func (c *SignalingClient) handlePingMessage(message *protocol.StandardMessage) {
	startTime := time.Now()
	log.Printf("🏓 Received PING from client %s (messageID: %s)", c.ID, message.ID)

	// 记录客户端状态跟踪信息
	c.mutex.RLock()
	clientState := c.State
	lastSeen := c.LastSeen
	messageCount := c.MessageCount
	errorCount := c.ErrorCount
	c.mutex.RUnlock()

	log.Printf("📊 Client %s state tracking - State: %s, LastSeen: %v ago, Messages: %d, Errors: %d",
		c.ID, clientState, time.Since(lastSeen), messageCount, errorCount)

	// 解析ping数据并记录详细信息
	var pingTimestamp int64
	var clientStateInfo string
	var additionalData map[string]any

	if message.Data != nil {
		if pingData, ok := message.Data.(map[string]any); ok {
			additionalData = pingData
			if timestamp, exists := pingData["timestamp"]; exists {
				// 处理不同类型的时间戳
				switch ts := timestamp.(type) {
				case float64:
					pingTimestamp = int64(ts)
				case int64:
					pingTimestamp = ts
				case int:
					pingTimestamp = int64(ts)
				}
				if pingTimestamp > 0 {
					log.Printf("🏓 Ping from client %s with timestamp: %d (latency: %dms)",
						c.ID, pingTimestamp, time.Now().Unix()-pingTimestamp)
				}
			}
			if state, exists := pingData["client_state"]; exists {
				if stateStr, ok := state.(string); ok {
					clientStateInfo = stateStr
					log.Printf("📊 Client %s reported state: %s", c.ID, clientStateInfo)
				}
			}
			// 记录其他ping数据
			if len(pingData) > 2 { // 除了timestamp和client_state之外的数据
				log.Printf("📋 Additional ping data from client %s: %+v", c.ID, additionalData)
			}
		}
	}

	// 创建 PONG 响应
	serverTime := time.Now().Unix()
	pongData := map[string]any{
		"timestamp":   serverTime,
		"client_id":   c.ID,
		"server_time": serverTime,
	}

	// 如果 PING 消息包含时间戳，添加到响应中
	if pingTimestamp > 0 {
		pongData["ping_timestamp"] = pingTimestamp
		pongData["round_trip_time"] = serverTime - pingTimestamp
	}

	// 如果 PING 消息包含客户端状态，添加到响应中
	if clientStateInfo != "" {
		pongData["client_state"] = clientStateInfo
	}

	// 添加服务器状态信息
	pongData["server_state"] = map[string]any{
		"client_count":   c.Server.GetClientCount(),
		"uptime_seconds": time.Since(c.ConnectedAt).Seconds(),
		"message_count":  messageCount,
		"error_count":    errorCount,
		"last_error":     c.getLastErrorInfo(),
	}

	pongMessage := c.Server.messageRouter.CreateStandardResponse(
		protocol.MessageTypePong, c.ID, pongData)

	// 记录处理性能指标
	processingTime := time.Since(startTime)

	sendErr := c.sendStandardMessage(pongMessage)
	if sendErr != nil {
		// 增强错误处理和上下文信息
		errorContext := map[string]any{
			"client_id":       c.ID,
			"message_id":      message.ID,
			"processing_time": processingTime.Milliseconds(),
			"client_state":    clientState,
			"ping_timestamp":  pingTimestamp,
			"server_time":     serverTime,
			"error_details":   sendErr.Error(),
		}

		log.Printf("❌ Failed to send pong message to client %s: %v (context: %+v)",
			c.ID, sendErr, errorContext)

		// 记录错误到客户端错误历史
		c.recordError(&SignalingError{
			Code:    ErrorCodeInternalError,
			Message: "Failed to send pong response",
			Details: fmt.Sprintf("Error: %v, Context: %+v", sendErr, errorContext),
			Type:    "ping_response_error",
		})
	} else {
		log.Printf("✅ Pong sent to client %s successfully (processing time: %dms, data size: %d bytes)",
			c.ID, processingTime.Milliseconds(), len(fmt.Sprintf("%+v", pongData)))
	}

	// 记录性能指标
	c.recordMessageProcessingMetrics("ping", processingTime, sendErr == nil)
}

// handleRequestOfferMessage 处理请求 Offer 消息
func (c *SignalingClient) handleRequestOfferMessage(message *protocol.StandardMessage) {
	startTime := time.Now()
	log.Printf("📞 Received request-offer from client %s (messageID: %s)", c.ID, message.ID)

	// 记录详细的请求上下文信息
	c.mutex.RLock()
	clientState := c.State
	messageCount := c.MessageCount
	errorCount := c.ErrorCount
	connectionDuration := time.Since(c.ConnectedAt)
	c.mutex.RUnlock()

	requestContext := map[string]any{
		"client_id":           c.ID,
		"message_id":          message.ID,
		"client_state":        clientState,
		"connection_duration": connectionDuration.String(),
		"message_count":       messageCount,
		"error_count":         errorCount,
		"remote_addr":         c.RemoteAddr,
		"user_agent":          c.UserAgent,
	}

	log.Printf("📋 Request-offer context for client %s: %+v", c.ID, requestContext)

	// 解析请求数据并记录详细信息
	var requestData map[string]any
	var constraints map[string]any
	var codecPreferences []string

	if message.Data != nil {
		if data, ok := message.Data.(map[string]any); ok {
			requestData = data
			log.Printf("📋 Request data from client %s: %+v", c.ID, requestData)

			if constraintsData, exists := data["constraints"]; exists {
				if constraintsMap, ok := constraintsData.(map[string]any); ok {
					constraints = constraintsMap
					log.Printf("🎥 Media constraints from client %s: %+v", c.ID, constraints)
				}
			}

			if codecPrefs, exists := data["codec_preferences"]; exists {
				if prefs, ok := codecPrefs.([]any); ok {
					for _, pref := range prefs {
						if prefStr, ok := pref.(string); ok {
							codecPreferences = append(codecPreferences, prefStr)
						}
					}
					log.Printf("🎵 Codec preferences from client %s: %v", c.ID, codecPreferences)
				}
			}
		}
	}

	// 使用 PeerConnection 管理器创建 Offer
	if c.Server.peerConnectionManager == nil {
		errorContext := map[string]any{
			"client_id":       c.ID,
			"message_id":      message.ID,
			"processing_time": time.Since(startTime).Milliseconds(),
			"error_stage":     "peer_connection_manager_check",
			"request_context": requestContext,
		}

		log.Printf("❌ PeerConnection manager not available for client %s (context: %+v)", c.ID, errorContext)
		c.sendStandardErrorMessage("PEER_CONNECTION_UNAVAILABLE",
			"PeerConnection manager is not available",
			fmt.Sprintf("Context: %+v", errorContext))
		c.recordMessageProcessingMetrics("request-offer", time.Since(startTime), false)
		return
	}

	// 创建 PeerConnection
	pcCreationStart := time.Now()
	pc, err := c.Server.peerConnectionManager.CreatePeerConnection(c.ID)
	pcCreationTime := time.Since(pcCreationStart)

	if err != nil {
		errorContext := map[string]any{
			"client_id":         c.ID,
			"message_id":        message.ID,
			"processing_time":   time.Since(startTime).Milliseconds(),
			"pc_creation_time":  pcCreationTime.Milliseconds(),
			"error_stage":       "peer_connection_creation",
			"error_details":     err.Error(),
			"request_context":   requestContext,
			"constraints":       constraints,
			"codec_preferences": codecPreferences,
		}

		log.Printf("❌ Failed to create PeerConnection for client %s: %v (context: %+v)",
			c.ID, err, errorContext)
		c.sendStandardErrorMessage("PEER_CONNECTION_CREATION_FAILED",
			"Failed to create PeerConnection",
			fmt.Sprintf("Error: %v, Context: %+v", err, errorContext))
		c.recordMessageProcessingMetrics("request-offer", time.Since(startTime), false)
		return
	}

	log.Printf("✅ PeerConnection created for client %s (creation time: %dms)",
		c.ID, pcCreationTime.Milliseconds())

	// 创建 SDP Offer
	offerCreationStart := time.Now()
	offer, err := pc.CreateOffer(nil)
	offerCreationTime := time.Since(offerCreationStart)

	if err != nil {
		errorContext := map[string]any{
			"client_id":           c.ID,
			"message_id":          message.ID,
			"processing_time":     time.Since(startTime).Milliseconds(),
			"pc_creation_time":    pcCreationTime.Milliseconds(),
			"offer_creation_time": offerCreationTime.Milliseconds(),
			"error_stage":         "offer_creation",
			"error_details":       err.Error(),
			"request_context":     requestContext,
			"constraints":         constraints,
			"codec_preferences":   codecPreferences,
		}

		log.Printf("❌ Failed to create offer for client %s: %v (context: %+v)",
			c.ID, err, errorContext)
		c.sendStandardErrorMessage("OFFER_CREATION_FAILED",
			"Failed to create SDP offer",
			fmt.Sprintf("Error: %v, Context: %+v", err, errorContext))
		c.recordMessageProcessingMetrics("request-offer", time.Since(startTime), false)
		return
	}

	log.Printf("✅ SDP offer created for client %s (type: %s, length: %d bytes, creation time: %dms)",
		c.ID, offer.Type, len(offer.SDP), offerCreationTime.Milliseconds())

	// 设置本地描述
	localDescStart := time.Now()
	if err := pc.SetLocalDescription(offer); err != nil {
		localDescTime := time.Since(localDescStart)
		errorContext := map[string]any{
			"client_id":           c.ID,
			"message_id":          message.ID,
			"processing_time":     time.Since(startTime).Milliseconds(),
			"pc_creation_time":    pcCreationTime.Milliseconds(),
			"offer_creation_time": offerCreationTime.Milliseconds(),
			"local_desc_time":     localDescTime.Milliseconds(),
			"error_stage":         "local_description_setting",
			"error_details":       err.Error(),
			"offer_type":          offer.Type.String(),
			"offer_sdp_length":    len(offer.SDP),
			"request_context":     requestContext,
			"constraints":         constraints,
			"codec_preferences":   codecPreferences,
		}

		log.Printf("❌ Failed to set local description for client %s: %v (context: %+v)",
			c.ID, err, errorContext)
		c.sendStandardErrorMessage("LOCAL_DESCRIPTION_FAILED",
			"Failed to set local description",
			fmt.Sprintf("Error: %v, Context: %+v", err, errorContext))
		c.recordMessageProcessingMetrics("request-offer", time.Since(startTime), false)
		return
	}
	localDescTime := time.Since(localDescStart)

	log.Printf("✅ Local description set for client %s (time: %dms)",
		c.ID, localDescTime.Milliseconds())

	// 发送 Offer
	sdpData := &protocol.SDPData{
		Type: offer.Type.String(),
		SDP:  offer.SDP,
	}

	offerMessage := c.Server.messageRouter.CreateStandardResponse(
		protocol.MessageTypeOffer, c.ID, sdpData)

	sendStart := time.Now()
	if err := c.sendStandardMessage(offerMessage); err != nil {
		sendTime := time.Since(sendStart)
		totalProcessingTime := time.Since(startTime)

		errorContext := map[string]any{
			"client_id":             c.ID,
			"message_id":            message.ID,
			"total_processing_time": totalProcessingTime.Milliseconds(),
			"pc_creation_time":      pcCreationTime.Milliseconds(),
			"offer_creation_time":   offerCreationTime.Milliseconds(),
			"local_desc_time":       localDescTime.Milliseconds(),
			"send_time":             sendTime.Milliseconds(),
			"error_stage":           "offer_sending",
			"error_details":         err.Error(),
			"offer_type":            offer.Type.String(),
			"offer_sdp_length":      len(offer.SDP),
			"request_context":       requestContext,
			"constraints":           constraints,
			"codec_preferences":     codecPreferences,
		}

		log.Printf("❌ Failed to send offer to client %s: %v (context: %+v)",
			c.ID, err, errorContext)

		// 记录错误到客户端错误历史
		c.recordError(&SignalingError{
			Code:    ErrorCodeInternalError,
			Message: "Failed to send offer response",
			Details: fmt.Sprintf("Error: %v, Context: %+v", err, errorContext),
			Type:    "offer_response_error",
		})
		c.recordMessageProcessingMetrics("request-offer", totalProcessingTime, false)
	} else {
		sendTime := time.Since(sendStart)
		totalProcessingTime := time.Since(startTime)

		successContext := map[string]any{
			"client_id":             c.ID,
			"message_id":            message.ID,
			"total_processing_time": totalProcessingTime.Milliseconds(),
			"pc_creation_time":      pcCreationTime.Milliseconds(),
			"offer_creation_time":   offerCreationTime.Milliseconds(),
			"local_desc_time":       localDescTime.Milliseconds(),
			"send_time":             sendTime.Milliseconds(),
			"offer_type":            offer.Type.String(),
			"offer_sdp_length":      len(offer.SDP),
			"constraints":           constraints,
			"codec_preferences":     codecPreferences,
		}

		log.Printf("✅ Offer sent to client %s successfully (context: %+v)", c.ID, successContext)
		c.recordMessageProcessingMetrics("request-offer", totalProcessingTime, true)
	}
}

// handleAnswerMessage 处理 Answer 消息
func (c *SignalingClient) handleAnswerMessage(message *protocol.StandardMessage) {
	log.Printf("📞 Received answer from client %s", c.ID)

	// 解析 SDP Answer
	var sdpData protocol.SDPData
	if err := message.GetDataAs(&sdpData); err != nil {
		log.Printf("❌ Failed to parse answer data from client %s: %v", c.ID, err)
		c.sendStandardErrorMessage("INVALID_ANSWER_DATA", "Failed to parse answer data", err.Error())
		return
	}

	// 获取 PeerConnection
	if c.Server.peerConnectionManager == nil {
		log.Printf("❌ PeerConnection manager not available for client %s", c.ID)
		c.sendStandardErrorMessage("PEER_CONNECTION_UNAVAILABLE",
			"PeerConnection manager is not available", "")
		return
	}

	pc, exists := c.Server.peerConnectionManager.GetPeerConnection(c.ID)
	if !exists {
		log.Printf("❌ PeerConnection not found for client %s", c.ID)
		c.sendStandardErrorMessage("PEER_CONNECTION_NOT_FOUND",
			"PeerConnection not found", "")
		return
	}

	// 设置远程描述
	answer := webrtc.SessionDescription{
		Type: webrtc.SDPTypeAnswer,
		SDP:  sdpData.SDP,
	}

	if err := pc.SetRemoteDescription(answer); err != nil {
		log.Printf("❌ Failed to set remote description for client %s: %v", c.ID, err)
		c.sendStandardErrorMessage("REMOTE_DESCRIPTION_FAILED",
			"Failed to set remote description", err.Error())
		return
	}

	// 发送确认
	ackMessage := c.Server.messageRouter.CreateStandardResponse(
		protocol.MessageTypeAnswerAck, c.ID, map[string]any{
			"status":    "success",
			"timestamp": time.Now().Unix(),
		})

	if err := c.sendStandardMessage(ackMessage); err != nil {
		log.Printf("❌ Failed to send answer ack to client %s: %v", c.ID, err)
	} else {
		log.Printf("✅ Answer processed for client %s", c.ID)
	}
}

// handleICECandidateMessage 处理 ICE 候选消息
func (c *SignalingClient) handleICECandidateMessage(message *protocol.StandardMessage) {
	log.Printf("🧊 Received ICE candidate from client %s", c.ID)

	// 解析 ICE 候选数据
	var iceData protocol.ICECandidateData
	if err := message.GetDataAs(&iceData); err != nil {
		log.Printf("❌ Failed to parse ICE candidate data from client %s: %v", c.ID, err)
		c.sendStandardErrorMessage("INVALID_ICE_DATA", "Failed to parse ICE candidate data", err.Error())
		return
	}

	// 获取 PeerConnection
	if c.Server.peerConnectionManager == nil {
		log.Printf("❌ PeerConnection manager not available for client %s", c.ID)
		c.sendStandardErrorMessage("PEER_CONNECTION_UNAVAILABLE",
			"PeerConnection manager is not available", "")
		return
	}

	pc, exists := c.Server.peerConnectionManager.GetPeerConnection(c.ID)
	if !exists {
		log.Printf("❌ PeerConnection not found for client %s", c.ID)
		c.sendStandardErrorMessage("PEER_CONNECTION_NOT_FOUND",
			"PeerConnection not found", "")
		return
	}

	// 创建 ICE 候选
	candidate := webrtc.ICECandidateInit{
		Candidate: iceData.Candidate,
	}

	if iceData.SDPMid != nil {
		candidate.SDPMid = iceData.SDPMid
	}

	if iceData.SDPMLineIndex != nil {
		candidate.SDPMLineIndex = iceData.SDPMLineIndex
	}

	// 添加 ICE 候选
	if err := pc.AddICECandidate(candidate); err != nil {
		log.Printf("❌ Failed to add ICE candidate for client %s: %v", c.ID, err)
		c.sendStandardErrorMessage("ICE_CANDIDATE_FAILED",
			"Failed to add ICE candidate", err.Error())
		return
	}

	// 发送确认
	ackMessage := c.Server.messageRouter.CreateStandardResponse(
		protocol.MessageTypeICEAck, c.ID, map[string]any{
			"status":    "success",
			"timestamp": time.Now().Unix(),
		})

	if err := c.sendStandardMessage(ackMessage); err != nil {
		log.Printf("❌ Failed to send ICE ack to client %s: %v", c.ID, err)
	} else {
		log.Printf("✅ ICE candidate processed for client %s", c.ID)
	}
}

// handleProtocolNegotiationMessage 处理协议协商消息
func (c *SignalingClient) handleProtocolNegotiationMessage(message *protocol.StandardMessage) {
	log.Printf("🤝 Received protocol negotiation from client %s", c.ID)

	if c.Server.messageRouter == nil {
		log.Printf("❌ Message router not available for client %s", c.ID)
		c.sendStandardErrorMessage("MESSAGE_ROUTER_UNAVAILABLE",
			"Message router is not available", "")
		return
	}

	// 使用消息路由器处理协议协商
	response, err := c.Server.messageRouter.HandleProtocolNegotiation([]byte("{}"), c.ID)
	if err != nil {
		log.Printf("❌ Protocol negotiation failed for client %s: %v", c.ID, err)
		c.sendStandardErrorMessage("PROTOCOL_NEGOTIATION_FAILED",
			"Protocol negotiation failed", err.Error())
		return
	}

	// 发送协商响应
	if err := c.sendStandardMessage(response); err != nil {
		log.Printf("❌ Failed to send protocol negotiation response to client %s: %v", c.ID, err)
	} else {
		log.Printf("✅ Protocol negotiation completed for client %s", c.ID)
	}
}

// handleGetStatsMessage 处理获取统计信息消息
func (c *SignalingClient) handleGetStatsMessage(message *protocol.StandardMessage) {
	log.Printf("📊 Received get-stats from client %s", c.ID)

	// 收集统计信息
	stats := &protocol.StatsData{
		SessionID:        c.ID,
		ConnectionState:  string(c.getState()),
		MessagesSent:     c.MessageCount,
		MessagesReceived: c.MessageCount,
		BytesSent:        0, // TODO: 实现字节计数
		BytesReceived:    0, // TODO: 实现字节计数
		ConnectionTime:   time.Since(c.ConnectedAt).Seconds(),
		LastActivity:     c.LastSeen.Unix(),
		Quality:          "good", // TODO: 实现连接质量评估
		Details: map[string]any{
			"protocol":    c.Protocol,
			"error_count": c.ErrorCount,
			"last_error":  c.LastError,
			"remote_addr": c.RemoteAddr,
			"user_agent":  c.UserAgent,
		},
	}

	statsMessage := c.Server.messageRouter.CreateStandardResponse(
		protocol.MessageTypeStats, c.ID, stats)

	if err := c.sendStandardMessage(statsMessage); err != nil {
		log.Printf("❌ Failed to send stats to client %s: %v", c.ID, err)
	} else {
		log.Printf("✅ Stats sent to client %s", c.ID)
	}
}

// handleInputMessage 处理输入消息
func (c *SignalingClient) handleInputMessage(message *protocol.StandardMessage) {
	log.Printf("🖱️ Received input message from client %s: type=%s", c.ID, message.Type)

	// TODO: 实现输入事件处理
	// 这里应该将输入事件转发给桌面捕获系统

	// 发送确认（可选）
	ackMessage := c.Server.messageRouter.CreateStandardResponse(
		protocol.MessageType("input-ack"), c.ID, map[string]any{
			"status":    "received",
			"type":      message.Type,
			"timestamp": time.Now().Unix(),
		})

	if err := c.sendStandardMessage(ackMessage); err != nil {
		log.Printf("❌ Failed to send input ack to client %s: %v", c.ID, err)
	}
}

// handleProtocolError 处理协议错误
func (c *SignalingClient) handleProtocolError(errorCode, errorMessage string) {
	log.Printf("❌ Protocol error for client %s: %s - %s", c.ID, errorCode, errorMessage)

	// 尝试协议降级
	if c.Server.protocolNegotiator != nil && c.Protocol != "" {
		log.Printf("🔄 Attempting protocol downgrade for client %s from %s", c.ID, c.Protocol)

		// 尝试降级到下一个协议
		fallbackProtocols := []protocol.ProtocolVersion{
			protocol.ProtocolVersionSelkies,
		}

		for _, fallback := range fallbackProtocols {
			if fallback != c.Protocol {
				c.mutex.Lock()
				oldProtocol := c.Protocol
				c.Protocol = fallback
				c.mutex.Unlock()

				log.Printf("🔄 Protocol downgraded for client %s: %s -> %s", c.ID, oldProtocol, fallback)

				// 发送协议降级通知
				downgradedMessage := c.Server.messageRouter.CreateStandardResponse(
					protocol.MessageType("protocol-downgraded"), c.ID, map[string]any{
						"old_protocol": oldProtocol,
						"new_protocol": fallback,
						"reason":       errorCode,
					})

				if err := c.sendStandardMessage(downgradedMessage); err != nil {
					log.Printf("❌ Failed to send protocol downgrade notification to client %s: %v", c.ID, err)
				}
				return
			}
		}
	}

	// 如果无法降级，发送错误消息
	c.sendStandardErrorMessage(errorCode, errorMessage, "Protocol error occurred")
}

// sendStandardErrorMessage 发送标准错误消息
func (c *SignalingClient) sendStandardErrorMessage(code, message, details string) {
	startTime := time.Now()

	// 收集诊断信息
	c.mutex.RLock()
	clientState := c.State
	messageCount := c.MessageCount
	errorCount := c.ErrorCount
	lastError := c.LastError
	connectionDuration := time.Since(c.ConnectedAt)
	c.mutex.RUnlock()

	// 构建增强的错误上下文
	diagnosticInfo := map[string]any{
		"client_id":           c.ID,
		"client_state":        clientState,
		"connection_duration": connectionDuration.String(),
		"message_count":       messageCount,
		"error_count":         errorCount,
		"remote_addr":         c.RemoteAddr,
		"user_agent":          c.UserAgent,
		"timestamp":           time.Now().Unix(),
		"server_uptime":       time.Since(c.ConnectedAt).String(),
	}

	// 添加最后一个错误信息（如果存在）
	if lastError != nil {
		diagnosticInfo["last_error"] = map[string]any{
			"code":    lastError.Code,
			"message": lastError.Message,
			"type":    lastError.Type,
		}
	}

	// 添加服务器状态信息
	diagnosticInfo["server_info"] = map[string]any{
		"total_clients":       c.Server.GetClientCount(),
		"peer_connection_mgr": c.Server.peerConnectionManager != nil,
		"message_router":      c.Server.messageRouter != nil,
		"protocol_negotiator": c.Server.protocolNegotiator != nil,
	}

	// 将诊断信息添加到详细信息中
	enhancedDetails := details
	if details != "" {
		enhancedDetails = fmt.Sprintf("%s | Diagnostics: %+v", details, diagnosticInfo)
	} else {
		enhancedDetails = fmt.Sprintf("Diagnostics: %+v", diagnosticInfo)
	}

	log.Printf("🚨 Sending error to client %s - Code: %s, Message: %s, Diagnostics: %+v",
		c.ID, code, message, diagnosticInfo)

	errorMessage := c.Server.messageRouter.CreateErrorResponse(code, message, enhancedDetails)
	errorMessage.PeerID = c.ID

	// 添加额外的错误元数据
	if errorMessage.Data == nil {
		errorMessage.Data = make(map[string]any)
	}

	if errorData, ok := errorMessage.Data.(map[string]any); ok {
		errorData["diagnostic_info"] = diagnosticInfo
		errorData["error_timestamp"] = time.Now().Unix()
		errorData["error_sequence"] = errorCount + 1
	}

	sendTime := time.Now()
	sendErr := c.sendStandardMessage(errorMessage)
	if sendErr != nil {
		sendDuration := time.Since(sendTime)
		totalDuration := time.Since(startTime)

		// 记录发送错误的详细信息
		sendErrorContext := map[string]any{
			"original_error_code":    code,
			"original_error_message": message,
			"send_error":             sendErr.Error(),
			"send_duration":          sendDuration.Milliseconds(),
			"total_duration":         totalDuration.Milliseconds(),
			"diagnostic_info":        diagnosticInfo,
		}

		log.Printf("❌ Failed to send error message to client %s: %v (context: %+v)",
			c.ID, sendErr, sendErrorContext)

		// 记录发送失败的错误
		c.recordError(&SignalingError{
			Code:    ErrorCodeInternalError,
			Message: "Failed to send error message",
			Details: fmt.Sprintf("Original error: %s - %s, Send error: %v, Context: %+v",
				code, message, sendErr, sendErrorContext),
			Type: "error_send_failure",
		})
	} else {
		sendDuration := time.Since(sendTime)
		totalDuration := time.Since(startTime)

		log.Printf("✅ Error message sent to client %s successfully (send time: %dms, total time: %dms)",
			c.ID, sendDuration.Milliseconds(), totalDuration.Milliseconds())
	}

	// 记录错误消息发送的性能指标
	c.recordMessageProcessingMetrics("error", time.Since(startTime), sendErr == nil)
}

// handleMessage 处理客户端消息（保持向后兼容）
func (c *SignalingClient) handleMessage(message SignalingMessage) {
	// 记录消息接收详情
	log.Printf("📨 Message received from client %s: type='%s', messageID='%s', timestamp=%d, dataSize=%d bytes",
		c.ID, message.Type, message.MessageID, message.Timestamp, getDataSize(message.Data))

	// 验证消息
	if validationError := validateSignalingMessage(&message); validationError != nil {
		log.Printf("❌ Message validation failed for client %s: %s (type: %s)", c.ID, validationError.Message, message.Type)
		c.recordError(validationError)
		c.sendError(validationError)

		// 尝试协议降级
		c.handleProtocolError(validationError.Code, validationError.Message)
		return
	}

	log.Printf("✅ Processing validated message type '%s' from client %s", message.Type, c.ID)

	switch message.Type {
	case "ping":
		// 响应ping
		log.Printf("🏓 Ping received from client %s, sending pong", c.ID)
		response := SignalingMessage{
			Type:      "pong",
			PeerID:    c.ID,
			MessageID: generateMessageID(),
			Timestamp: time.Now().Unix(),
			Data: map[string]any{
				"timestamp":     time.Now().Unix(),
				"client_state":  c.getState(),
				"message_count": c.MessageCount,
			},
		}
		if err := c.sendMessage(response); err != nil {
			log.Printf("❌ Failed to send pong to client %s: %v", c.ID, err)
			signalingError := &SignalingError{
				Code:    ErrorCodeInternalError,
				Message: "Failed to send pong response",
				Details: err.Error(),
				Type:    "server_error",
			}
			c.recordError(signalingError)
		} else {
			log.Printf("✅ Pong sent to client %s", c.ID)
		}

	case "request-offer":
		// 处理offer请求
		log.Printf("🔄 WebRTC negotiation started: Client %s requested offer", c.ID)
		c.handleOfferRequest(message)

	case "answer":
		// 处理answer
		log.Printf("📞 WebRTC negotiation step 2: Client %s sent answer (SDP length: %d)", c.ID, getSDPLength(message.Data))
		c.handleAnswer(message)

	case "ice-candidate":
		// 处理ICE候选
		log.Printf("🧊 WebRTC negotiation step 3: Client %s sent ICE candidate (candidate: %s)", c.ID, getICECandidateInfo(message.Data))
		c.handleIceCandidate(message)

	case "answer-ack":
		// 处理answer确认
		log.Printf("Client %s acknowledged answer", c.ID)
		c.handleAnswerAck(message)

	case "ice-ack":
		// 处理ICE确认
		log.Printf("Client %s acknowledged ICE candidate", c.ID)
		c.handleIceAck(message)

	case "mouse-click":
		// 处理鼠标点击事件
		c.handleMouseClick(message)

	case "mouse-move":
		// 处理鼠标移动事件
		c.handleMouseMove(message)

	case "key-press":
		// 处理键盘按键事件
		c.handleKeyPress(message)

	case "get-stats":
		// 处理统计信息请求
		c.handleStatsRequest(message)

	case "protocol-negotiation":
		// 处理协议协商请求
		log.Printf("🔄 Protocol negotiation requested by client %s", c.ID)
		c.handleProtocolNegotiation(message)

	default:
		log.Printf("Unknown message type: %s from client %s", message.Type, c.ID)
		// 发送标准化错误响应
		signalingError := &SignalingError{
			Code:    ErrorCodeInvalidMessageType,
			Message: fmt.Sprintf("Unknown message type: %s", message.Type),
			Details: "This message type is not supported by the server",
			Type:    "validation_error",
		}
		c.recordError(signalingError)
		c.sendError(signalingError)
	}
}

// handleOfferRequest 处理offer请求
func (c *SignalingClient) handleOfferRequest(message SignalingMessage) {
	log.Printf("🚀 Starting WebRTC offer creation process for client %s", c.ID)

	// 获取PeerConnection管理器
	pcManager := c.Server.peerConnectionManager
	if pcManager == nil {
		log.Printf("❌ WebRTC negotiation failed: PeerConnection manager not available for client %s", c.ID)
		signalingError := &SignalingError{
			Code:    ErrorCodeServerUnavailable,
			Message: "PeerConnection manager not available",
			Details: "The WebRTC peer connection manager is not initialized",
			Type:    "server_error",
		}
		c.recordError(signalingError)
		c.sendError(signalingError)
		return
	}

	log.Printf("✅ PeerConnection manager available, creating peer connection for client %s", c.ID)

	// 为客户端创建PeerConnection
	pc, err := pcManager.CreatePeerConnection(c.ID)
	if err != nil {
		log.Printf("❌ WebRTC negotiation failed: Failed to create peer connection for client %s: %v", c.ID, err)
		signalingError := &SignalingError{
			Code:    ErrorCodePeerConnectionFailed,
			Message: "Failed to create peer connection",
			Details: err.Error(),
			Type:    "webrtc_error",
		}
		c.recordError(signalingError)
		c.sendError(signalingError)
		return
	}

	log.Printf("✅ PeerConnection created successfully for client %s", c.ID)

	// 设置ICE候选处理器
	pc.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate != nil {
			log.Printf("🧊 Generated ICE candidate for client %s: type=%s, protocol=%s, address=%s",
				c.ID, candidate.Typ.String(), candidate.Protocol.String(), candidate.Address)

			// 发送ICE候选给客户端 - 支持 selkies 协议
			if c.isSelkiesClient() {
				// selkies 协议格式
				selkiesICE := map[string]any{
					"ice": map[string]any{
						"candidate":     candidate.String(),
						"sdpMid":        candidate.SDPMid,
						"sdpMLineIndex": candidate.SDPMLineIndex,
					},
				}
				if iceBytes, err := json.Marshal(selkiesICE); err == nil {
					c.Send <- iceBytes
					log.Printf("📤 Selkies ICE candidate sent to client %s", c.ID)
				} else {
					log.Printf("❌ Failed to marshal selkies ICE candidate for client %s: %v", c.ID, err)
				}
			} else {
				// 原有协议格式
				candidateMessage := SignalingMessage{
					Type:      "ice-candidate",
					PeerID:    c.ID,
					MessageID: generateMessageID(),
					Timestamp: time.Now().Unix(),
					Data: map[string]interface{}{
						"candidate":     candidate.String(),
						"sdpMid":        candidate.SDPMid,
						"sdpMLineIndex": candidate.SDPMLineIndex,
					},
				}

				if candidateData, err := json.Marshal(candidateMessage); err == nil {
					c.Send <- candidateData
					log.Printf("📤 ICE candidate sent to client %s", c.ID)
				} else {
					log.Printf("❌ Failed to marshal ICE candidate for client %s: %v", c.ID, err)
				}
			}
		} else {
			log.Printf("🏁 ICE gathering complete for client %s", c.ID)
		}
	})

	log.Printf("🔧 Creating WebRTC offer for client %s", c.ID)

	// 创建offer
	offer, err := pc.CreateOffer(nil)
	if err != nil {
		log.Printf("❌ WebRTC negotiation failed: Failed to create offer for client %s: %v", c.ID, err)
		signalingError := &SignalingError{
			Code:    ErrorCodeOfferCreationFailed,
			Message: "Failed to create WebRTC offer",
			Details: err.Error(),
			Type:    "webrtc_error",
		}
		c.recordError(signalingError)
		c.sendError(signalingError)
		return
	}

	log.Printf("✅ WebRTC offer created successfully for client %s (SDP length: %d)", c.ID, len(offer.SDP))

	// 设置本地描述
	log.Printf("🔧 Setting local description for client %s", c.ID)
	err = pc.SetLocalDescription(offer)
	if err != nil {
		log.Printf("❌ WebRTC negotiation failed: Failed to set local description for client %s: %v", c.ID, err)
		signalingError := &SignalingError{
			Code:    ErrorCodeSDPProcessingFailed,
			Message: "Failed to set local description",
			Details: err.Error(),
			Type:    "webrtc_error",
		}
		c.recordError(signalingError)
		c.sendError(signalingError)
		return
	}

	// 发送offer给客户端 - 支持 selkies 协议
	if c.isSelkiesClient() {
		// selkies 协议格式
		selkiesOffer := map[string]any{
			"sdp": map[string]any{
				"type": "offer",
				"sdp":  offer.SDP,
			},
		}
		if offerBytes, err := json.Marshal(selkiesOffer); err == nil {
			c.Send <- offerBytes
			log.Printf("📤 Selkies offer sent to client %s", c.ID)
		} else {
			log.Printf("❌ Failed to marshal selkies offer for client %s: %v", c.ID, err)
		}
	} else {
		// 原有协议格式
		offerMessage := SignalingMessage{
			Type:      "offer",
			PeerID:    c.ID,
			MessageID: generateMessageID(),
			Timestamp: time.Now().Unix(),
			Data: map[string]any{
				"type": "offer",
				"sdp":  offer.SDP,
			},
		}

		log.Printf("Sending WebRTC offer to client %s", c.ID)
		if err := c.sendMessage(offerMessage); err != nil {
			log.Printf("Failed to send offer to client %s: %v", c.ID, err)
			signalingError := &SignalingError{
				Code:    ErrorCodeInternalError,
				Message: "Failed to send offer message",
				Details: err.Error(),
				Type:    "server_error",
			}
			c.recordError(signalingError)
		}
	}
}

// sendErrorMessage 发送简单错误消息（向后兼容）
func (c *SignalingClient) sendErrorMessage(message string) {
	signalingError := &SignalingError{
		Code:    ErrorCodeInternalError,
		Message: message,
		Type:    "server_error",
	}
	c.sendError(signalingError)
}

// handleAnswer 处理answer
func (c *SignalingClient) handleAnswer(message SignalingMessage) {
	log.Printf("🔄 WebRTC negotiation step 2: Processing answer from client %s", c.ID)

	// 获取PeerConnection
	pcManager := c.Server.peerConnectionManager
	if pcManager == nil {
		log.Printf("❌ WebRTC negotiation failed: PeerConnection manager not available for client %s", c.ID)
		signalingError := &SignalingError{
			Code:    ErrorCodeServerUnavailable,
			Message: "PeerConnection manager not available",
			Details: "The WebRTC peer connection manager is not initialized",
			Type:    "server_error",
		}
		c.recordError(signalingError)
		c.sendError(signalingError)
		return
	}

	log.Printf("✅ PeerConnection manager available, looking up connection for client %s", c.ID)

	pc, exists := pcManager.GetPeerConnection(c.ID)
	if !exists {
		log.Printf("❌ WebRTC negotiation failed: PeerConnection not found for client %s", c.ID)
		signalingError := &SignalingError{
			Code:    ErrorCodeClientNotFound,
			Message: "PeerConnection not found",
			Details: "No peer connection exists for this client",
			Type:    "webrtc_error",
		}
		c.recordError(signalingError)
		c.sendError(signalingError)
		return
	}

	log.Printf("✅ PeerConnection found for client %s", c.ID)

	// 解析answer数据 - 这里验证已经在validateSignalingMessage中完成
	log.Printf("🔍 Parsing answer data from client %s", c.ID)

	answerData, ok := message.Data.(map[string]any)
	if !ok {
		log.Printf("❌ WebRTC negotiation failed: Invalid answer data format from client %s", c.ID)
		signalingError := &SignalingError{
			Code:    ErrorCodeInvalidMessageData,
			Message: "Invalid answer data format",
			Details: "Answer data must be an object",
			Type:    "validation_error",
		}
		c.recordError(signalingError)
		c.sendError(signalingError)
		return
	}

	sdp, ok := answerData["sdp"].(string)
	if !ok {
		log.Printf("❌ WebRTC negotiation failed: Invalid SDP in answer from client %s", c.ID)
		signalingError := &SignalingError{
			Code:    ErrorCodeInvalidMessageData,
			Message: "Invalid SDP in answer",
			Details: "SDP must be a string",
			Type:    "validation_error",
		}
		c.recordError(signalingError)
		c.sendError(signalingError)
		return
	}

	log.Printf("✅ Answer SDP parsed successfully from client %s (length: %d)", c.ID, len(sdp))

	// 设置远程描述
	log.Printf("🔧 Setting remote description (answer) for client %s", c.ID)

	answer := webrtc.SessionDescription{
		Type: webrtc.SDPTypeAnswer,
		SDP:  sdp,
	}

	err := pc.SetRemoteDescription(answer)
	if err != nil {
		log.Printf("❌ WebRTC negotiation failed: Failed to set remote description for client %s: %v", c.ID, err)
		signalingError := &SignalingError{
			Code:    ErrorCodeAnswerProcessingFailed,
			Message: "Failed to set remote description",
			Details: err.Error(),
			Type:    "webrtc_error",
		}
		c.recordError(signalingError)
		c.sendError(signalingError)
		return
	}

	log.Printf("🎉 WebRTC negotiation step 2 completed: Answer processed successfully for client %s", c.ID)

	// 发送确认消息
	log.Printf("📤 Sending answer acknowledgment to client %s", c.ID)

	ack := SignalingMessage{
		Type:      "answer-ack",
		PeerID:    c.ID,
		MessageID: generateMessageID(),
		Timestamp: time.Now().Unix(),
		Data: map[string]any{
			"status":    "received",
			"processed": true,
		},
	}

	if err := c.sendMessage(ack); err != nil {
		log.Printf("❌ Failed to send answer acknowledgment to client %s: %v", c.ID, err)
		signalingError := &SignalingError{
			Code:    ErrorCodeInternalError,
			Message: "Failed to send answer acknowledgment",
			Details: err.Error(),
			Type:    "server_error",
		}
		c.recordError(signalingError)
	} else {
		log.Printf("✅ Answer acknowledgment sent to client %s", c.ID)
	}
}

// handleIceCandidate 处理ICE候选
func (c *SignalingClient) handleIceCandidate(message SignalingMessage) {
	log.Printf("🧊 WebRTC negotiation step 3: Processing ICE candidate from client %s", c.ID)

	// 获取PeerConnection
	pcManager := c.Server.peerConnectionManager
	if pcManager == nil {
		log.Printf("❌ WebRTC negotiation failed: PeerConnection manager not available for client %s", c.ID)
		signalingError := &SignalingError{
			Code:    ErrorCodeServerUnavailable,
			Message: "PeerConnection manager not available",
			Details: "The WebRTC peer connection manager is not initialized",
			Type:    "server_error",
		}
		c.recordError(signalingError)
		c.sendError(signalingError)
		return
	}

	pc, exists := pcManager.GetPeerConnection(c.ID)
	if !exists {
		log.Printf("❌ WebRTC negotiation failed: PeerConnection not found for client %s", c.ID)
		signalingError := &SignalingError{
			Code:    ErrorCodeClientNotFound,
			Message: "PeerConnection not found",
			Details: "No peer connection exists for this client",
			Type:    "webrtc_error",
		}
		c.recordError(signalingError)
		c.sendError(signalingError)
		return
	}

	log.Printf("✅ PeerConnection found for ICE candidate processing, client %s", c.ID)

	// 解析ICE候选数据 - 验证已经在validateSignalingMessage中完成
	log.Printf("🔍 Parsing ICE candidate data from client %s", c.ID)

	candidateData, ok := message.Data.(map[string]any)
	if !ok {
		log.Printf("❌ WebRTC negotiation failed: Invalid ICE candidate data format from client %s", c.ID)
		signalingError := &SignalingError{
			Code:    ErrorCodeInvalidMessageData,
			Message: "Invalid ICE candidate data format",
			Details: "ICE candidate data must be an object",
			Type:    "validation_error",
		}
		c.recordError(signalingError)
		c.sendError(signalingError)
		return
	}

	candidate, ok := candidateData["candidate"].(string)
	if !ok {
		log.Printf("❌ WebRTC negotiation failed: Invalid candidate in ICE candidate from client %s", c.ID)
		signalingError := &SignalingError{
			Code:    ErrorCodeInvalidMessageData,
			Message: "Invalid candidate in ICE candidate",
			Details: "Candidate must be a string",
			Type:    "validation_error",
		}
		c.recordError(signalingError)
		c.sendError(signalingError)
		return
	}

	log.Printf("✅ ICE candidate parsed from client %s: %s", c.ID, getICECandidateInfo(message.Data))

	sdpMid, _ := candidateData["sdpMid"].(string)
	sdpMLineIndex, _ := candidateData["sdpMLineIndex"].(float64)

	log.Printf("🔧 Creating ICE candidate for client %s (sdpMid: %s, sdpMLineIndex: %.0f)", c.ID, sdpMid, sdpMLineIndex)

	// 创建ICE候选
	iceCandidate := webrtc.ICECandidateInit{
		Candidate:     candidate,
		SDPMid:        &sdpMid,
		SDPMLineIndex: (*uint16)(&[]uint16{uint16(sdpMLineIndex)}[0]),
	}

	// 添加ICE候选
	log.Printf("🔧 Adding ICE candidate to PeerConnection for client %s", c.ID)
	err := pc.AddICECandidate(iceCandidate)
	if err != nil {
		log.Printf("❌ WebRTC negotiation failed: Failed to add ICE candidate for client %s: %v", c.ID, err)
		signalingError := &SignalingError{
			Code:    ErrorCodeICECandidateFailed,
			Message: "Failed to add ICE candidate",
			Details: err.Error(),
			Type:    "webrtc_error",
		}
		c.recordError(signalingError)
		c.sendError(signalingError)
		return
	}

	log.Printf("🎉 WebRTC negotiation step 3 completed: ICE candidate added successfully for client %s", c.ID)

	// 发送确认消息
	log.Printf("📤 Sending ICE candidate acknowledgment to client %s", c.ID)

	ack := SignalingMessage{
		Type:      "ice-ack",
		PeerID:    c.ID,
		MessageID: generateMessageID(),
		Timestamp: time.Now().Unix(),
		Data: map[string]any{
			"status":    "received",
			"processed": true,
		},
	}

	if err := c.sendMessage(ack); err != nil {
		log.Printf("❌ Failed to send ICE candidate acknowledgment to client %s: %v", c.ID, err)
		signalingError := &SignalingError{
			Code:    ErrorCodeInternalError,
			Message: "Failed to send ICE candidate acknowledgment",
			Details: err.Error(),
			Type:    "server_error",
		}
		c.recordError(signalingError)
	} else {
		log.Printf("✅ ICE candidate acknowledgment sent to client %s", c.ID)
	}
}

// handleMouseClick 处理鼠标点击事件
func (c *SignalingClient) handleMouseClick(message SignalingMessage) {
	log.Printf("Mouse click from client %s: %+v", c.ID, message.Data)
	// 这里应该实现实际的鼠标点击处理
}

// handleMouseMove 处理鼠标移动事件
func (c *SignalingClient) handleMouseMove(message SignalingMessage) {
	// 鼠标移动事件较频繁，使用debug级别日志
	// log.Printf("Mouse move from client %s: %+v", c.ID, message.Data)
	// 这里应该实现实际的鼠标移动处理
}

// handleKeyPress 处理键盘按键事件
func (c *SignalingClient) handleKeyPress(message SignalingMessage) {
	log.Printf("Key press from client %s: %+v", c.ID, message.Data)
	// 这里应该实现实际的键盘按键处理
}

// handleStatsRequest 处理统计信息请求
func (c *SignalingClient) handleStatsRequest(message SignalingMessage) {
	c.mutex.RLock()
	connectionDuration := time.Since(c.ConnectedAt)
	lastSeenDuration := time.Since(c.LastSeen)
	messageCount := c.MessageCount
	errorCount := c.ErrorCount
	lastError := c.LastError
	state := c.State
	c.mutex.RUnlock()

	statsData := map[string]any{
		"client_id":       c.ID,
		"app_name":        c.AppName,
		"state":           state,
		"connected_at":    c.ConnectedAt.Unix(),
		"connection_time": connectionDuration.Seconds(),
		"last_seen":       c.LastSeen.Unix(),
		"last_seen_ago":   lastSeenDuration.Seconds(),
		"message_count":   messageCount,
		"error_count":     errorCount,
		"remote_addr":     c.RemoteAddr,
		"user_agent":      c.UserAgent,
		"server_stats": map[string]any{
			"total_clients":    c.Server.GetClientCount(),
			"app_client_count": c.Server.GetAppClientCount(c.AppName),
		},
	}

	if lastError != nil {
		statsData["last_error"] = map[string]any{
			"code":    lastError.Code,
			"message": lastError.Message,
			"type":    lastError.Type,
		}
	}

	stats := SignalingMessage{
		Type:      "stats",
		PeerID:    c.ID,
		MessageID: generateMessageID(),
		Timestamp: time.Now().Unix(),
		Data:      statsData,
	}

	if err := c.sendMessage(stats); err != nil {
		log.Printf("Failed to send stats to client %s: %v", c.ID, err)
		signalingError := &SignalingError{
			Code:    ErrorCodeInternalError,
			Message: "Failed to send statistics",
			Details: err.Error(),
			Type:    "server_error",
		}
		c.recordError(signalingError)
	}
}

// handleAnswerAck 处理answer确认
func (c *SignalingClient) handleAnswerAck(message SignalingMessage) {
	log.Printf("Answer acknowledged by client %s", c.ID)
	// 这里可以添加确认处理逻辑，比如更新连接状态
}

// handleIceAck 处理ICE确认
func (c *SignalingClient) handleIceAck(message SignalingMessage) {
	log.Printf("ICE candidate acknowledged by client %s", c.ID)
	// 这里可以添加ICE确认处理逻辑
}

// detectProtocolFromMessage 从消息中检测协议类型
func (c *SignalingClient) detectProtocolFromMessage(messageBytes []byte) string {
	messageText := string(messageBytes)

	// 检测 Selkies 文本协议
	if strings.HasPrefix(messageText, "HELLO ") ||
		strings.HasPrefix(messageText, "ERROR ") ||
		messageText == "HELLO" {
		log.Printf("🔍 Protocol detected for client %s: selkies (text format)", c.ID)
		return "selkies"
	}

	// 尝试解析 JSON
	var jsonMessage map[string]any
	if err := json.Unmarshal(messageBytes, &jsonMessage); err != nil {
		log.Printf("🔍 Protocol detection failed for client %s: not valid JSON", c.ID)
		return "unknown"
	}

	// 检测标准协议（有 version 和 metadata 字段）
	if version, hasVersion := jsonMessage["version"]; hasVersion {
		if metadata, hasMetadata := jsonMessage["metadata"]; hasMetadata {
			if metadataMap, ok := metadata.(map[string]any); ok {
				if protocol, hasProtocol := metadataMap["protocol"]; hasProtocol {
					if protocolStr, ok := protocol.(string); ok {
						log.Printf("🔍 Protocol detected for client %s: %s (version: %v)", c.ID, protocolStr, version)
						return protocolStr
					}
				}
			}
		}
		log.Printf("🔍 Protocol detected for client %s: gstreamer-webrtc (has version field)", c.ID)
		return "gstreamer-webrtc"
	}

	// 检测 Selkies JSON 协议（有 sdp 或 ice 字段但没有 version）
	if _, hasSDP := jsonMessage["sdp"]; hasSDP {
		log.Printf("🔍 Protocol detected for client %s: selkies (JSON with SDP)", c.ID)
		return "selkies"
	}

	if _, hasICE := jsonMessage["ice"]; hasICE {
		log.Printf("🔍 Protocol detected for client %s: selkies (JSON with ICE)", c.ID)
		return "selkies"
	}

	// 检测标准协议消息类型
	if msgType, hasType := jsonMessage["type"]; hasType {
		if typeStr, ok := msgType.(string); ok {
			standardTypes := map[string]bool{
				"protocol-negotiation": true,
				"ping":                 true,
				"pong":                 true,
				"request-offer":        true,
				"offer":                true,
				"answer":               true,
				"ice-candidate":        true,
			}

			if standardTypes[typeStr] {
				log.Printf("🔍 Protocol detected for client %s: gstreamer-webrtc (standard message type: %s)", c.ID, typeStr)
				return "gstreamer-webrtc"
			}
		}
	}

	log.Printf("🔍 Protocol detection for client %s: unknown/legacy", c.ID)
	return "unknown"
}

// getDetectionConfidence 获取协议检测置信度
func (c *SignalingClient) getDetectionConfidence(protocol string, messageBytes []byte) float64 {
	messageText := string(messageBytes)

	switch protocol {
	case "selkies":
		if strings.HasPrefix(messageText, "HELLO ") {
			return 0.95 // 高置信度
		}

		var jsonMessage map[string]any
		if json.Unmarshal(messageBytes, &jsonMessage) == nil {
			if _, hasSDP := jsonMessage["sdp"]; hasSDP {
				return 0.90
			}
			if _, hasICE := jsonMessage["ice"]; hasICE {
				return 0.85
			}
		}
		return 0.70

	case "gstreamer-webrtc":
		var jsonMessage map[string]any
		if json.Unmarshal(messageBytes, &jsonMessage) == nil {
			confidence := 0.60

			if _, hasVersion := jsonMessage["version"]; hasVersion {
				confidence += 0.20
			}

			if metadata, hasMetadata := jsonMessage["metadata"]; hasMetadata {
				if metadataMap, ok := metadata.(map[string]any); ok {
					if _, hasProtocol := metadataMap["protocol"]; hasProtocol {
						confidence += 0.15
					}
				}
			}

			return confidence
		}
		return 0.50

	default:
		return 0.30
	}
}

// handleSelkiesMessage 处理 selkies 协议消息
func (c *SignalingClient) handleSelkiesMessage(messageText string) bool {
	// 检查是否是 HELLO 消息
	if strings.HasPrefix(messageText, "HELLO ") {
		log.Printf("🔄 Selkies HELLO message received from client %s: %s", c.ID, messageText)
		c.handleSelkiesHello(messageText)
		return true
	}

	// 尝试解析为 JSON (SDP/ICE 消息)
	var jsonMsg map[string]any
	if err := json.Unmarshal([]byte(messageText), &jsonMsg); err == nil {
		// 检查是否是 selkies 格式的 SDP 消息
		if sdpData, exists := jsonMsg["sdp"]; exists {
			log.Printf("📞 Selkies SDP message received from client %s", c.ID)
			c.handleSelkiesSDP(sdpData)
			return true
		}

		// 检查是否是 selkies 格式的 ICE 消息
		if iceData, exists := jsonMsg["ice"]; exists {
			log.Printf("🧊 Selkies ICE message received from client %s", c.ID)
			c.handleSelkiesICE(iceData)
			return true
		}
	}

	// 不是 selkies 协议消息
	return false
}

// handleSelkiesHello 处理 selkies HELLO 消息
func (c *SignalingClient) handleSelkiesHello(messageText string) {
	// 解析 HELLO 消息: "HELLO ${peer_id} ${btoa(JSON.stringify(meta))}"
	parts := strings.SplitN(messageText, " ", 3)
	if len(parts) < 2 {
		log.Printf("❌ Invalid HELLO message format from client %s: %s", c.ID, messageText)
		c.sendSelkiesError("Invalid HELLO message format")
		return
	}

	peerID := parts[1]
	var meta map[string]any

	// 解析元数据 (如果存在)
	if len(parts) >= 3 {
		metaEncoded := parts[2]
		if metaBytes, err := base64.StdEncoding.DecodeString(metaEncoded); err == nil {
			if err := json.Unmarshal(metaBytes, &meta); err != nil {
				log.Printf("⚠️ Failed to parse HELLO metadata from client %s: %v", c.ID, err)
			}
		}
	}

	log.Printf("✅ Selkies HELLO processed: client=%s, peerID=%s, meta=%+v", c.ID, peerID, meta)

	// 标记为 selkies 客户端
	c.mutex.Lock()
	c.IsSelkies = true
	c.mutex.Unlock()

	// 发送简单的 HELLO 响应 (selkies 协议)
	c.sendSelkiesMessage("HELLO")

	// 触发 offer 请求处理
	c.handleOfferRequest(SignalingMessage{
		Type:   "request-offer",
		PeerID: c.ID,
	})
}

// handleSelkiesSDP 处理 selkies SDP 消息
func (c *SignalingClient) handleSelkiesSDP(sdpData any) {
	sdpMap, ok := sdpData.(map[string]any)
	if !ok {
		log.Printf("❌ Invalid SDP data format from client %s", c.ID)
		c.sendSelkiesError("Invalid SDP data format")
		return
	}

	// 转换为标准 SignalingMessage 格式
	message := SignalingMessage{
		Type:   "answer",
		PeerID: c.ID,
		Data:   sdpMap,
	}

	log.Printf("🔄 Converting selkies SDP to standard format for client %s", c.ID)
	c.handleAnswer(message)
}

// handleSelkiesICE 处理 selkies ICE 消息
func (c *SignalingClient) handleSelkiesICE(iceData any) {
	iceMap, ok := iceData.(map[string]any)
	if !ok {
		log.Printf("❌ Invalid ICE data format from client %s", c.ID)
		c.sendSelkiesError("Invalid ICE data format")
		return
	}

	// 转换为标准 SignalingMessage 格式
	message := SignalingMessage{
		Type:   "ice-candidate",
		PeerID: c.ID,
		Data:   iceMap,
	}

	log.Printf("🔄 Converting selkies ICE to standard format for client %s", c.ID)
	c.handleIceCandidate(message)
}

// sendSelkiesMessage 发送 selkies 协议消息
func (c *SignalingClient) sendSelkiesMessage(message string) {
	select {
	case c.Send <- []byte(message):
		log.Printf("📤 Selkies message sent to client %s: %s", c.ID, message)
	default:
		log.Printf("❌ Failed to send selkies message to client %s: channel full", c.ID)
	}
}

// sendSelkiesError 发送 selkies 协议错误消息
func (c *SignalingClient) sendSelkiesError(errorMsg string) {
	errorMessage := fmt.Sprintf("ERROR %s", errorMsg)
	c.sendSelkiesMessage(errorMessage)
}

// isSelkiesClient 检查客户端是否使用 selkies 协议
func (c *SignalingClient) isSelkiesClient() bool {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.IsSelkies
}

// handleProtocolNegotiation 处理协议协商请求
func (c *SignalingClient) handleProtocolNegotiation(message SignalingMessage) {
	log.Printf("🔄 Processing protocol negotiation for client %s", c.ID)

	// 解析协商数据
	negotiationData, ok := message.Data.(map[string]any)
	if !ok {
		log.Printf("❌ Invalid protocol negotiation data from client %s", c.ID)
		c.sendProtocolNegotiationError("INVALID_NEGOTIATION_DATA", "Protocol negotiation data must be an object", message.MessageID)
		return
	}

	// 获取客户端支持的协议
	supportedProtocols, ok := negotiationData["supported_protocols"].([]any)
	if !ok {
		log.Printf("❌ Missing supported_protocols in negotiation from client %s", c.ID)
		c.sendProtocolNegotiationError("MISSING_SUPPORTED_PROTOCOLS", "supported_protocols field is required", message.MessageID)
		return
	}

	// 转换为字符串切片
	clientProtocols := make([]string, 0, len(supportedProtocols))
	for _, protocol := range supportedProtocols {
		if protocolStr, ok := protocol.(string); ok {
			clientProtocols = append(clientProtocols, protocolStr)
		}
	}

	log.Printf("📋 Client %s supports protocols: %v", c.ID, clientProtocols)

	// 服务器支持的协议（按优先级排序）
	serverProtocols := []string{
		"gstreamer-1.0",
		"selkies",
		"legacy",
	}

	// 协议协商逻辑：选择双方都支持的最高优先级协议
	selectedProtocol := c.negotiateProtocol(clientProtocols, serverProtocols)

	if selectedProtocol == "" {
		log.Printf("❌ No compatible protocol found for client %s", c.ID)
		c.sendProtocolNegotiationError("NO_COMPATIBLE_PROTOCOL", "No mutually supported protocol found", message.MessageID)
		return
	}

	log.Printf("✅ Protocol negotiated for client %s: %s", c.ID, selectedProtocol)

	// 更新客户端协议模式
	c.setProtocolMode(selectedProtocol)

	// 发送协商成功响应
	response := SignalingMessage{
		Type:      "protocol-negotiation-response",
		PeerID:    c.ID,
		MessageID: message.MessageID, // 使用相同的消息ID用于响应
		Timestamp: time.Now().Unix(),
		Data: map[string]any{
			"success":           true,
			"selected_protocol": selectedProtocol,
			"server_protocols":  serverProtocols,
			"protocol_info": map[string]any{
				"version":      c.getProtocolVersion(selectedProtocol),
				"capabilities": c.getProtocolCapabilities(selectedProtocol),
				"features":     c.getProtocolFeatures(selectedProtocol),
			},
		},
	}

	if err := c.sendMessage(response); err != nil {
		log.Printf("❌ Failed to send protocol negotiation response to client %s: %v", c.ID, err)
		signalingError := &SignalingError{
			Code:    ErrorCodeInternalError,
			Message: "Failed to send protocol negotiation response",
			Details: err.Error(),
			Type:    "server_error",
		}
		c.recordError(signalingError)
	} else {
		log.Printf("✅ Protocol negotiation response sent to client %s", c.ID)
	}
}

// negotiateProtocol 协商协议版本
func (c *SignalingClient) negotiateProtocol(clientProtocols, serverProtocols []string) string {
	// 按服务器优先级顺序查找匹配的协议
	for _, serverProtocol := range serverProtocols {
		for _, clientProtocol := range clientProtocols {
			if c.isProtocolCompatible(serverProtocol, clientProtocol) {
				return serverProtocol
			}
		}
	}
	return ""
}

// isProtocolCompatible 检查协议兼容性
func (c *SignalingClient) isProtocolCompatible(serverProtocol, clientProtocol string) bool {
	// 精确匹配
	if serverProtocol == clientProtocol {
		return true
	}

	// 版本兼容性检查
	compatibilityMap := map[string][]string{
		"gstreamer-1.0": {"gstreamer-1.0", "gstreamer"},
		"selkies":       {"selkies", "selkies-1.0"},
		"legacy":        {"legacy", "unknown"},
	}

	if compatibleVersions, exists := compatibilityMap[serverProtocol]; exists {
		for _, compatible := range compatibleVersions {
			if compatible == clientProtocol {
				return true
			}
		}
	}

	return false
}

// setProtocolMode 设置客户端协议模式
func (c *SignalingClient) setProtocolMode(protocol string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	switch protocol {
	case "selkies", "selkies-1.0":
		c.IsSelkies = true
	default:
		c.IsSelkies = false
	}

	log.Printf("Client %s protocol mode set to: %s (selkies: %v)", c.ID, protocol, c.IsSelkies)
}

// getProtocolVersion 获取协议版本信息
func (c *SignalingClient) getProtocolVersion(protocol string) string {
	versionMap := map[string]string{
		"gstreamer-1.0": "1.0",
		"selkies":       "1.0",
		"legacy":        "0.9",
	}

	if version, exists := versionMap[protocol]; exists {
		return version
	}
	return "1.0"
}

// getProtocolCapabilities 获取协议能力
func (c *SignalingClient) getProtocolCapabilities(protocol string) []string {
	capabilityMap := map[string][]string{
		"gstreamer-1.0": {
			"webrtc",
			"datachannel",
			"video-h264",
			"video-vp8",
			"video-vp9",
			"audio-opus",
			"input-events",
			"statistics",
			"error-recovery",
		},
		"selkies": {
			"webrtc",
			"video-h264",
			"audio-opus",
			"input-events",
			"basic-statistics",
		},
		"legacy": {
			"webrtc",
			"video-h264",
			"basic-input",
		},
	}

	if capabilities, exists := capabilityMap[protocol]; exists {
		return capabilities
	}
	return []string{"webrtc"}
}

// getProtocolFeatures 获取协议特性
func (c *SignalingClient) getProtocolFeatures(protocol string) map[string]any {
	featureMap := map[string]map[string]any{
		"gstreamer-1.0": {
			"message_validation":     true,
			"error_recovery":         true,
			"protocol_versioning":    true,
			"capability_negotiation": true,
			"statistics_reporting":   true,
		},
		"selkies": {
			"message_validation":     false,
			"error_recovery":         false,
			"protocol_versioning":    false,
			"backward_compatibility": true,
		},
		"legacy": {
			"message_validation": false,
			"error_recovery":     false,
			"minimal_features":   true,
		},
	}

	if features, exists := featureMap[protocol]; exists {
		return features
	}
	return map[string]any{"basic": true}
}

// sendProtocolNegotiationError 发送协议协商错误
func (c *SignalingClient) sendProtocolNegotiationError(code, message, messageID string) {
	errorResponse := SignalingMessage{
		Type:      "protocol-negotiation-response",
		PeerID:    c.ID,
		MessageID: messageID,
		Timestamp: time.Now().Unix(),
		Data: map[string]any{
			"success": false,
			"error":   message,
			"code":    code,
		},
	}

	if err := c.sendMessage(errorResponse); err != nil {
		log.Printf("❌ Failed to send protocol negotiation error to client %s: %v", c.ID, err)
	}

	// 记录错误
	signalingError := &SignalingError{
		Code:    code,
		Message: message,
		Type:    "protocol_negotiation_error",
	}
	c.recordError(signalingError)
}

// downgradeProtocol 协议降级处理
func (c *SignalingClient) downgradeProtocol(currentProtocol, reason string) string {
	log.Printf("🔽 Protocol downgrade requested for client %s: %s -> reason: %s", c.ID, currentProtocol, reason)

	// 协议降级层次结构
	protocolHierarchy := []string{
		"gstreamer-1.0",
		"selkies",
		"legacy",
	}

	// 找到当前协议在层次结构中的位置
	currentIndex := -1
	for i, protocol := range protocolHierarchy {
		if protocol == currentProtocol {
			currentIndex = i
			break
		}
	}

	// 如果找不到当前协议或已经是最低级协议
	if currentIndex == -1 || currentIndex >= len(protocolHierarchy)-1 {
		log.Printf("❌ Cannot downgrade protocol for client %s: no lower version available", c.ID)
		return currentProtocol
	}

	// 降级到下一个协议
	targetProtocol := protocolHierarchy[currentIndex+1]

	log.Printf("🔽 Downgrading client %s protocol: %s -> %s", c.ID, currentProtocol, targetProtocol)

	// 更新客户端协议模式
	c.setProtocolMode(targetProtocol)

	// 发送协议降级通知
	notification := SignalingMessage{
		Type:      "protocol-downgraded",
		PeerID:    c.ID,
		MessageID: generateMessageID(),
		Timestamp: time.Now().Unix(),
		Data: map[string]any{
			"from_protocol":  currentProtocol,
			"to_protocol":    targetProtocol,
			"reason":         reason,
			"downgrade_time": time.Now().Unix(),
			"protocol_info": map[string]any{
				"version":      c.getProtocolVersion(targetProtocol),
				"capabilities": c.getProtocolCapabilities(targetProtocol),
				"features":     c.getProtocolFeatures(targetProtocol),
			},
		},
	}

	if err := c.sendMessage(notification); err != nil {
		log.Printf("⚠️ Failed to send protocol downgrade notification to client %s: %v", c.ID, err)
	} else {
		log.Printf("✅ Protocol downgrade notification sent to client %s", c.ID)
	}

	return targetProtocol
}
