package webserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"

	"github.com/open-beagle/bdwind-gstreamer/internal/common/protocol"
	"github.com/open-beagle/bdwind-gstreamer/internal/config"
)

// ClientType 客户端类型
type ClientType string

const (
	ClientTypeStreamer ClientType = "streamer" // 推流客户端
	ClientTypeUI       ClientType = "ui"       // UI客户端
)

// ConnectionInfo WebSocket连接信息
type ConnectionInfo struct {
	ID          string                 `json:"id"`
	ClientType  ClientType             `json:"client_type"`
	ClientID    string                 `json:"client_id"`
	RemoteAddr  string                 `json:"remote_addr"`
	UserAgent   string                 `json:"user_agent"`
	ConnectedAt time.Time              `json:"connected_at"`
	LastSeen    time.Time              `json:"last_seen"`
	Conn        *websocket.Conn        `json:"-"`
	Send        chan []byte            `json:"-"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// StreamClientInfo 推流客户端信息
type StreamClientInfo struct {
	*ConnectionInfo
	StreamID     string   `json:"stream_id"`
	Status       string   `json:"status"`
	Capabilities []string `json:"capabilities"`
}

// UIClientInfo UI客户端信息
type UIClientInfo struct {
	*ConnectionInfo
	SessionID   string `json:"session_id"`
	ConnectedTo string `json:"connected_to"` // 连接到的推流客户端ID
}

// StreamSession 推流会话
type StreamSession struct {
	ID         string    `json:"id"`
	StreamerID string    `json:"streamer_id"`
	UIID       string    `json:"ui_id"`
	CreatedAt  time.Time `json:"created_at"`
	Status     string    `json:"status"`
}

// SignalingServer WebServer组件中的信令服务器
// 只负责WebSocket连接管理和消息路由，不处理WebRTC逻辑
type SignalingServer struct {
	connections   map[*websocket.Conn]*ConnectionInfo
	streamClients map[string]*StreamClientInfo
	uiClients     map[string]*UIClientInfo
	sessions      map[string]*StreamSession
	router        *SignalingRouter
	upgrader      websocket.Upgrader
	mutex         sync.RWMutex
	logger        *logrus.Entry
	running       bool
	ctx           context.Context
	cancel        context.CancelFunc

	// 业务消息处理回调
	businessMessageHandler func(clientID string, messageType string, data map[string]interface{}) error

	// 统计信息
	totalConnections    int64
	totalMessages       int64
	totalErrors         int64
	connectionDurations []time.Duration
}

// NewSignalingServer 创建信令服务器
func NewSignalingServer() *SignalingServer {
	ctx, cancel := context.WithCancel(context.Background())

	server := &SignalingServer{
		connections:   make(map[*websocket.Conn]*ConnectionInfo),
		streamClients: make(map[string]*StreamClientInfo),
		uiClients:     make(map[string]*UIClientInfo),
		sessions:      make(map[string]*StreamSession),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // 允许跨域
			},
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		},
		logger: config.GetLoggerWithPrefix("webserver-signaling"),
		ctx:    ctx,
		cancel: cancel,
	}

	// 创建消息路由器
	server.router = NewSignalingRouter(server)

	return server
}

// SetBusinessMessageHandler 设置业务消息处理回调
// 这允许主程序处理 WebRTC 业务逻辑，而信令服务器只负责消息路由
func (s *SignalingServer) SetBusinessMessageHandler(handler func(clientID string, messageType string, data map[string]interface{}) error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.businessMessageHandler = handler
	if s.router != nil {
		s.router.SetBusinessMessageHandler(handler)
	}

	s.logger.Debug("Business message handler configured for signaling server")
}

// 实现 ComponentManager 接口

// SetupRoutes 设置路由
func (s *SignalingServer) SetupRoutes(router *mux.Router) error {
	s.logger.Debug("Setting up signaling server routes...")

	// WebSocket连接路由
	router.HandleFunc("/api/signaling", s.HandleWebSocket).Methods("GET")
	router.HandleFunc("/api/signaling/ws", s.HandleWebSocket).Methods("GET")

	// REST API路由
	apiRouter := router.PathPrefix("/api/signaling").Subrouter()
	apiRouter.HandleFunc("/status", s.handleStatus).Methods("GET")
	apiRouter.HandleFunc("/clients", s.handleClients).Methods("GET")
	apiRouter.HandleFunc("/sessions", s.handleSessions).Methods("GET")
	apiRouter.HandleFunc("/stats", s.handleStats).Methods("GET")

	s.logger.Debug("Signaling server routes setup completed")
	return nil
}

// Start 启动信令服务器
func (s *SignalingServer) Start(ctx context.Context) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.running {
		return fmt.Errorf("signaling server already running")
	}

	s.logger.Info("Starting signaling server...")
	s.running = true

	// 启动清理协程
	go s.cleanupRoutine()

	s.logger.Info("Signaling server started successfully")
	return nil
}

// Stop 停止信令服务器
func (s *SignalingServer) Stop(ctx context.Context) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if !s.running {
		return nil
	}

	s.logger.Info("Stopping signaling server...")
	s.running = false

	// 取消上下文
	if s.cancel != nil {
		s.cancel()
	}

	// 关闭所有连接
	for conn, info := range s.connections {
		s.logger.Debugf("Closing connection for client %s", info.ID)
		close(info.Send)
		conn.Close()
	}

	// 清空映射
	s.connections = make(map[*websocket.Conn]*ConnectionInfo)
	s.streamClients = make(map[string]*StreamClientInfo)
	s.uiClients = make(map[string]*UIClientInfo)
	s.sessions = make(map[string]*StreamSession)

	s.logger.Info("Signaling server stopped successfully")
	return nil
}

// IsEnabled 检查是否启用
func (s *SignalingServer) IsEnabled() bool {
	return true // 信令服务器始终启用
}

// IsRunning 检查是否运行中
func (s *SignalingServer) IsRunning() bool {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.running
}

// GetStats 获取统计信息
func (s *SignalingServer) GetStats() map[string]interface{} {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	stats := map[string]interface{}{
		"running":            s.running,
		"total_connections":  len(s.connections),
		"stream_clients":     len(s.streamClients),
		"ui_clients":         len(s.uiClients),
		"active_sessions":    len(s.sessions),
		"total_messages":     s.totalMessages,
		"total_errors":       s.totalErrors,
		"connection_history": s.totalConnections,
	}

	// 计算平均连接时长
	if len(s.connectionDurations) > 0 {
		var total time.Duration
		for _, duration := range s.connectionDurations {
			total += duration
		}
		stats["avg_connection_duration"] = total.Seconds() / float64(len(s.connectionDurations))
	}

	return stats
}

// GetContext 获取上下文
func (s *SignalingServer) GetContext() context.Context {
	return s.ctx
}

// HandleWebSocket 处理WebSocket连接
func (s *SignalingServer) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	s.logger.Debug("Handling WebSocket connection request...")

	// 升级HTTP连接为WebSocket
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.logger.Errorf("WebSocket upgrade failed: %v", err)
		return
	}

	s.logger.Infof("WebSocket connection established from %s", conn.RemoteAddr())

	// 创建连接信息
	connInfo := &ConnectionInfo{
		ID:          s.generateConnectionID(),
		RemoteAddr:  conn.RemoteAddr().String(),
		UserAgent:   r.Header.Get("User-Agent"),
		ConnectedAt: time.Now(),
		LastSeen:    time.Now(),
		Conn:        conn,
		Send:        make(chan []byte, 256),
		Metadata:    make(map[string]interface{}),
	}

	// 注册连接
	s.registerConnection(conn, connInfo)

	// 启动读写协程
	go s.writePump(connInfo)
	go s.readPump(connInfo)
}

// registerConnection 注册连接
func (s *SignalingServer) registerConnection(conn *websocket.Conn, info *ConnectionInfo) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.connections[conn] = info
	s.totalConnections++

	s.logger.Infof("Connection registered: %s from %s", info.ID, info.RemoteAddr)

	// 发送欢迎消息
	welcome := &protocol.StandardMessage{
		Version:   protocol.ProtocolVersionGStreamer10,
		Type:      protocol.MessageTypeWelcome,
		ID:        s.generateMessageID(),
		Timestamp: time.Now().Unix(),
		Data: &protocol.WelcomeData{
			ClientID:   info.ID,
			ServerTime: time.Now().Unix(),
			Protocol:   string(protocol.ProtocolVersionGStreamer10),
		},
	}

	s.sendMessage(info, welcome)
}

// unregisterConnection 注销连接
func (s *SignalingServer) unregisterConnection(conn *websocket.Conn) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	info, exists := s.connections[conn]
	if !exists {
		return
	}

	// 记录连接时长
	duration := time.Since(info.ConnectedAt)
	s.connectionDurations = append(s.connectionDurations, duration)

	// 从相应的客户端映射中移除
	if info.ClientType == ClientTypeStreamer {
		delete(s.streamClients, info.ClientID)
		s.logger.Infof("Stream client unregistered: %s", info.ClientID)
	} else if info.ClientType == ClientTypeUI {
		delete(s.uiClients, info.ClientID)
		s.logger.Infof("UI client unregistered: %s", info.ClientID)
	}

	// 从连接映射中移除
	delete(s.connections, conn)
	close(info.Send)

	s.logger.Infof("Connection unregistered: %s (connected for: %v)", info.ID, duration)
}

// readPump 读取消息
func (s *SignalingServer) readPump(info *ConnectionInfo) {
	defer func() {
		s.unregisterConnection(info.Conn)
		info.Conn.Close()
	}()

	// 设置读取超时
	info.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	info.Conn.SetPongHandler(func(string) error {
		info.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, messageData, err := info.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				s.logger.Errorf("WebSocket error for client %s: %v", info.ID, err)
				s.totalErrors++
			}
			break
		}

		// 更新最后活跃时间
		s.mutex.Lock()
		info.LastSeen = time.Now()
		s.totalMessages++
		s.mutex.Unlock()

		// 处理消息
		s.router.HandleMessage(info, messageData)
	}
}

// writePump 发送消息
func (s *SignalingServer) writePump(info *ConnectionInfo) {
	ticker := time.NewTicker(54 * time.Second)
	defer func() {
		ticker.Stop()
		info.Conn.Close()
	}()

	for {
		select {
		case message, ok := <-info.Send:
			info.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				info.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := info.Conn.WriteMessage(websocket.TextMessage, message); err != nil {
				s.logger.Errorf("Write error for client %s: %v", info.ID, err)
				s.totalErrors++
				return
			}

		case <-ticker.C:
			info.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := info.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// sendMessage 发送消息给指定连接
func (s *SignalingServer) sendMessage(info *ConnectionInfo, message *protocol.StandardMessage) error {
	data, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	select {
	case info.Send <- data:
		return nil
	default:
		return fmt.Errorf("send channel full")
	}
}

// RegisterStreamClient 注册推流客户端
func (s *SignalingServer) RegisterStreamClient(connInfo *ConnectionInfo, streamID string, capabilities []string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	streamClient := &StreamClientInfo{
		ConnectionInfo: connInfo,
		StreamID:       streamID,
		Status:         "active",
		Capabilities:   capabilities,
	}

	connInfo.ClientType = ClientTypeStreamer
	connInfo.ClientID = streamID
	s.streamClients[streamID] = streamClient

	s.logger.Infof("Stream client registered: %s with capabilities: %v", streamID, capabilities)
	return nil
}

// RegisterUIClient 注册UI客户端
func (s *SignalingServer) RegisterUIClient(connInfo *ConnectionInfo, sessionID string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	uiClient := &UIClientInfo{
		ConnectionInfo: connInfo,
		SessionID:      sessionID,
	}

	connInfo.ClientType = ClientTypeUI
	connInfo.ClientID = sessionID
	s.uiClients[sessionID] = uiClient

	s.logger.Infof("UI client registered: %s", sessionID)
	return nil
}

// RouteMessage 路由消息到目标客户端
func (s *SignalingServer) RouteMessage(fromID, toID string, message *protocol.StandardMessage) error {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	// 查找目标连接
	var targetInfo *ConnectionInfo

	// 先在推流客户端中查找
	if streamClient, exists := s.streamClients[toID]; exists {
		targetInfo = streamClient.ConnectionInfo
	}

	// 再在UI客户端中查找
	if targetInfo == nil {
		if uiClient, exists := s.uiClients[toID]; exists {
			targetInfo = uiClient.ConnectionInfo
		}
	}

	if targetInfo == nil {
		return fmt.Errorf("target client not found: %s", toID)
	}

	// 设置消息的目标ID
	message.PeerID = toID

	return s.sendMessage(targetInfo, message)
}

// BroadcastToStreamClients 广播消息给所有推流客户端
func (s *SignalingServer) BroadcastToStreamClients(message *protocol.StandardMessage) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	for _, streamClient := range s.streamClients {
		if err := s.sendMessage(streamClient.ConnectionInfo, message); err != nil {
			s.logger.Errorf("Failed to broadcast to stream client %s: %v", streamClient.StreamID, err)
		}
	}
}

// BroadcastToUIClients 广播消息给所有UI客户端
func (s *SignalingServer) BroadcastToUIClients(message *protocol.StandardMessage) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	for _, uiClient := range s.uiClients {
		if err := s.sendMessage(uiClient.ConnectionInfo, message); err != nil {
			s.logger.Errorf("Failed to broadcast to UI client %s: %v", uiClient.SessionID, err)
		}
	}
}

// GetStreamClients 获取所有推流客户端
func (s *SignalingServer) GetStreamClients() map[string]*StreamClientInfo {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	clients := make(map[string]*StreamClientInfo)
	for id, client := range s.streamClients {
		clients[id] = client
	}
	return clients
}

// GetUIClients 获取所有UI客户端
func (s *SignalingServer) GetUIClients() map[string]*UIClientInfo {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	clients := make(map[string]*UIClientInfo)
	for id, client := range s.uiClients {
		clients[id] = client
	}
	return clients
}

// cleanupRoutine 清理过期连接
func (s *SignalingServer) cleanupRoutine() {
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.cleanupExpiredConnections()
		case <-s.ctx.Done():
			s.logger.Debug("Cleanup routine shutting down")
			return
		}
	}
}

// cleanupExpiredConnections 清理过期连接
func (s *SignalingServer) cleanupExpiredConnections() {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	now := time.Now()
	expiredConns := make([]*websocket.Conn, 0)

	for conn, info := range s.connections {
		if now.Sub(info.LastSeen) > 10*time.Minute {
			expiredConns = append(expiredConns, conn)
		}
	}

	for _, conn := range expiredConns {
		info := s.connections[conn]
		s.logger.Infof("Cleaning up expired connection: %s (last seen: %v ago)",
			info.ID, now.Sub(info.LastSeen))

		conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseGoingAway, "Connection expired"))
		s.unregisterConnection(conn)
	}
}

// REST API处理器

// handleStatus 处理状态查询
func (s *SignalingServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	status := map[string]interface{}{
		"running":           s.IsRunning(),
		"total_connections": len(s.connections),
		"stream_clients":    len(s.streamClients),
		"ui_clients":        len(s.uiClients),
		"active_sessions":   len(s.sessions),
	}

	s.writeJSON(w, status)
}

// handleClients 处理客户端列表查询
func (s *SignalingServer) handleClients(w http.ResponseWriter, r *http.Request) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	clients := map[string]interface{}{
		"stream_clients": s.streamClients,
		"ui_clients":     s.uiClients,
	}

	s.writeJSON(w, clients)
}

// handleSessions 处理会话列表查询
func (s *SignalingServer) handleSessions(w http.ResponseWriter, r *http.Request) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	s.writeJSON(w, s.sessions)
}

// handleStats 处理统计信息查询
func (s *SignalingServer) handleStats(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, s.GetStats())
}

// writeJSON 写入JSON响应
func (s *SignalingServer) writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		s.logger.Errorf("Failed to encode JSON: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// 工具方法

// generateConnectionID 生成连接ID
func (s *SignalingServer) generateConnectionID() string {
	return fmt.Sprintf("conn_%d", time.Now().UnixNano())
}

// generateMessageID 生成消息ID
func (s *SignalingServer) generateMessageID() string {
	return fmt.Sprintf("msg_%d", time.Now().UnixNano())
}
