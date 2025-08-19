package webrtc

import (
	"context"
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
	ID           string
	AppName      string
	Conn         *websocket.Conn
	Send         chan []byte
	Server       *SignalingServer
	LastSeen     time.Time
	ConnectedAt  time.Time
	RemoteAddr   string
	UserAgent    string
	State        ClientState
	MessageCount int64
	ErrorCount   int64
	LastError    *SignalingError
	mutex        sync.RWMutex
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
	ctx                   context.Context
	cancel                context.CancelFunc
}

// NewSignalingServer 创建信令服务器
func NewSignalingServer(ctx context.Context, encoderConfig *SignalingEncoderConfig, mediaStream any, iceServers []webrtc.ICEServer) *SignalingServer {
	sdpConfig := &SDPConfig{
		Codec: encoderConfig.Codec,
	}

	childCtx, cancel := context.WithCancel(ctx)

	// 创建PeerConnection管理器
	var pcManager *PeerConnectionManager
	if ms, ok := mediaStream.(MediaStream); ok {
		pcManager = NewPeerConnectionManager(ms, iceServers, log.Default())
	}

	return &SignalingServer{
		clients:               make(map[string]*SignalingClient),
		apps:                  make(map[string]map[string]*SignalingClient),
		sdpGenerator:          NewSDPGenerator(sdpConfig),
		mediaStream:           mediaStream,
		peerConnectionManager: pcManager,
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
}

// Start 启动信令服务器
func (s *SignalingServer) Start() {
	log.Printf("🚀 Starting signaling server...")
	s.running = true

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

			// 解析JSON消息
			var message SignalingMessage
			if err := json.Unmarshal(messageBytes, &message); err != nil {
				log.Printf("❌ Failed to parse JSON message from client %s: %v", c.ID, err)
				signalingError := &SignalingError{
					Code:    ErrorCodeMessageParsingFailed,
					Message: "Failed to parse JSON message",
					Details: err.Error(),
					Type:    "validation_error",
				}
				c.recordError(signalingError)
				c.sendError(signalingError)
				continue
			}

			log.Printf("✅ JSON message parsed successfully from client %s", c.ID)

			// 增加消息计数
			c.incrementMessageCount()

			// 处理消息
			c.handleMessage(message)
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

// handleMessage 处理客户端消息
func (c *SignalingClient) handleMessage(message SignalingMessage) {
	// 记录消息接收详情
	log.Printf("📨 Message received from client %s: type='%s', messageID='%s', timestamp=%d, dataSize=%d bytes",
		c.ID, message.Type, message.MessageID, message.Timestamp, getDataSize(message.Data))

	// 验证消息
	if validationError := validateSignalingMessage(&message); validationError != nil {
		log.Printf("❌ Message validation failed for client %s: %s (type: %s)", c.ID, validationError.Message, message.Type)
		c.recordError(validationError)
		c.sendError(validationError)
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

			// 发送ICE候选给客户端
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

	// 发送offer给客户端
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
