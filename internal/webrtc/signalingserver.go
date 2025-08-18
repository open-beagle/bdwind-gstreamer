package webrtc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v4"
)

// SignalingMessage 信令消息
type SignalingMessage struct {
	Type   string      `json:"type"`
	PeerID string      `json:"peer_id,omitempty"`
	Data   interface{} `json:"data,omitempty"`
	Error  string      `json:"error,omitempty"`
}

// SignalingClient 信令客户端
type SignalingClient struct {
	ID       string
	AppName  string
	Conn     *websocket.Conn
	Send     chan []byte
	Server   *SignalingServer
	LastSeen time.Time
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
	mediaStream           interface{} // WebRTC 媒体流
	peerConnectionManager *PeerConnectionManager
	ctx                   context.Context
	cancel                context.CancelFunc
}

// NewSignalingServer 创建信令服务器
func NewSignalingServer(ctx context.Context, encoderConfig *SignalingEncoderConfig, mediaStream interface{}, iceServers []webrtc.ICEServer) *SignalingServer {
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
	s.running = true

	// 启动清理协程
	go s.cleanupRoutine()

	for s.running {
		select {
		case client := <-s.register:
			s.registerClient(client)

		case client := <-s.unregister:
			s.unregisterClient(client)

		case message := <-s.broadcast:
			s.broadcastMessage(message)

		case <-s.ctx.Done():
			log.Printf("Signaling server received shutdown signal")
			s.Stop()
			return
		}
	}
}

// Stop 停止信令服务器
func (s *SignalingServer) Stop() {
	s.running = false
	s.cancel()

	s.mutex.Lock()
	defer s.mutex.Unlock()

	// 关闭所有客户端连接
	for _, client := range s.clients {
		close(client.Send)
	}

	// 关闭所有PeerConnection
	if s.peerConnectionManager != nil {
		s.peerConnectionManager.Close()
	}

	// 清空客户端映射
	s.clients = make(map[string]*SignalingClient)
	s.apps = make(map[string]map[string]*SignalingClient)
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
	client := &SignalingClient{
		ID:       generateSignalingClientID(),
		AppName:  "default",
		Conn:     conn,
		Send:     make(chan []byte, 256),
		Server:   s,
		LastSeen: time.Now(),
	}

	log.Printf("Creating new signaling client: %s from %s", client.ID, conn.RemoteAddr())

	// 先注册客户端
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
			log.Printf("Welcome message sent to client %s", client.ID)
		default:
			log.Printf("Failed to send welcome message to client %s: channel full", client.ID)
		}
	} else {
		log.Printf("Failed to marshal welcome message for client %s: %v", client.ID, err)
	}

	// 最后启动读写协程
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

	log.Printf("Client %s registered for app %s", client.ID, client.AppName)
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
		log.Printf("Client %s unregistered from app %s", client.ID, client.AppName)
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

	for _, client := range s.clients {
		// 增加过期时间到10分钟，避免过早清理活跃连接
		if now.Sub(client.LastSeen) > 10*time.Minute {
			expiredClients = append(expiredClients, client)
		}
	}

	for _, client := range expiredClients {
		log.Printf("Cleaning up expired client: %s (last seen: %v ago)", client.ID, now.Sub(client.LastSeen))
		// 发送关闭消息而不是直接注销
		client.Conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseGoingAway, "Connection expired"))
		s.unregisterClient(client)
	}
}

// generateSignalingClientID 生成客户端ID
func generateSignalingClientID() string {
	return fmt.Sprintf("client_%d_%d", time.Now().UnixNano(), rand.Intn(10000))
}

// 错误定义
var (
	ErrSignalingClientNotFound  = errors.New("signaling client not found")
	ErrSignalingSendChannelFull = errors.New("signaling send channel is full")
)

// readPump 读取客户端消息
func (c *SignalingClient) readPump() {
	defer func() {
		log.Printf("Client %s read pump exiting", c.ID)
		c.Server.unregister <- c
		c.Conn.Close()
	}()

	// 设置读取超时为更长时间，避免频繁超时
	c.Conn.SetReadDeadline(time.Now().Add(300 * time.Second)) // 5分钟
	c.Conn.SetPongHandler(func(string) error {
		c.Conn.SetReadDeadline(time.Now().Add(300 * time.Second))
		c.LastSeen = time.Now()
		return nil
	})

	log.Printf("Client %s read pump started", c.ID)

	for {
		messageType, messageBytes, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure, websocket.CloseNormalClosure) {
				log.Printf("WebSocket unexpected close error for client %s: %v", c.ID, err)
			} else {
				log.Printf("WebSocket connection closed for client %s: %v", c.ID, err)
			}
			break
		}

		c.LastSeen = time.Now()

		// 只处理文本消息，其他类型的消息由WebSocket库自动处理
		if messageType == websocket.TextMessage {
			// 解析JSON消息
			var message SignalingMessage
			if err := json.Unmarshal(messageBytes, &message); err != nil {
				log.Printf("Failed to parse message from client %s: %v", c.ID, err)
				continue
			}
			// 处理消息
			c.handleMessage(message)
		} else {
			log.Printf("Received non-text message from client %s (type: %d, length: %d)", c.ID, messageType, len(messageBytes))
		}
	}
}

// writePump 向客户端发送消息
func (c *SignalingClient) writePump() {
	ticker := time.NewTicker(240 * time.Second) // 4分钟ping一次，避免过于频繁
	defer func() {
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
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := c.Conn.WriteMessage(websocket.TextMessage, message); err != nil {
				log.Printf("WebSocket write error for client %s: %v", c.ID, err)
				return
			}

			log.Printf("Message sent to client %s (length: %d)", c.ID, len(message))

		case <-ticker.C:
			c.Conn.SetWriteDeadline(time.Now().Add(30 * time.Second))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				log.Printf("WebSocket ping failed for client %s: %v", c.ID, err)
				return
			}
			log.Printf("Ping sent to client %s", c.ID)
		}
	}
}

// handleMessage 处理客户端消息
func (c *SignalingClient) handleMessage(message SignalingMessage) {
	switch message.Type {
	case "ping":
		// 响应ping
		response := SignalingMessage{
			Type:   "pong",
			PeerID: c.ID,
			Data: map[string]interface{}{
				"timestamp": time.Now().Unix(),
			},
		}
		if responseData, err := json.Marshal(response); err == nil {
			c.Send <- responseData
		}

	case "request-offer":
		// 处理offer请求
		log.Printf("Client %s requested offer", c.ID)
		c.handleOfferRequest(message)

	case "answer":
		// 处理answer
		log.Printf("Client %s sent answer", c.ID)
		c.handleAnswer(message)

	case "ice-candidate":
		// 处理ICE候选
		log.Printf("Client %s sent ICE candidate", c.ID)
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
		// 发送错误响应
		errorResponse := SignalingMessage{
			Type:   "error",
			PeerID: c.ID,
			Error:  fmt.Sprintf("Unknown message type: %s", message.Type),
		}
		if errorData, err := json.Marshal(errorResponse); err == nil {
			c.Send <- errorData
		}
	}
}

// handleOfferRequest 处理offer请求
func (c *SignalingClient) handleOfferRequest(message SignalingMessage) {
	// 获取PeerConnection管理器
	pcManager := c.Server.peerConnectionManager
	if pcManager == nil {
		log.Printf("PeerConnection manager not available")
		c.sendError("PeerConnection manager not available")
		return
	}

	// 为客户端创建PeerConnection
	pc, err := pcManager.CreatePeerConnection(c.ID)
	if err != nil {
		log.Printf("Failed to create peer connection for client %s: %v", c.ID, err)
		c.sendError("Failed to create peer connection")
		return
	}

	// 设置ICE候选处理器
	pc.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate != nil {
			log.Printf("Generated ICE candidate for client %s", c.ID)

			// 发送ICE候选给客户端
			candidateMessage := SignalingMessage{
				Type:   "ice-candidate",
				PeerID: c.ID,
				Data: map[string]interface{}{
					"candidate":     candidate.String(),
					"sdpMid":        candidate.SDPMid,
					"sdpMLineIndex": candidate.SDPMLineIndex,
				},
			}

			if candidateData, err := json.Marshal(candidateMessage); err == nil {
				c.Send <- candidateData
			} else {
				log.Printf("Failed to marshal ICE candidate: %v", err)
			}
		} else {
			log.Printf("ICE gathering complete for client %s", c.ID)
		}
	})

	// 创建offer
	offer, err := pc.CreateOffer(nil)
	if err != nil {
		log.Printf("Failed to create offer for client %s: %v", c.ID, err)
		c.sendError("Failed to create offer")
		return
	}

	// 设置本地描述
	err = pc.SetLocalDescription(offer)
	if err != nil {
		log.Printf("Failed to set local description for client %s: %v", c.ID, err)
		c.sendError("Failed to set local description")
		return
	}

	// 发送offer给客户端
	offerMessage := SignalingMessage{
		Type:   "offer",
		PeerID: c.ID,
		Data: map[string]interface{}{
			"type": "offer",
			"sdp":  offer.SDP,
		},
	}

	log.Printf("Sending WebRTC offer to client %s", c.ID)
	if offerData, err := json.Marshal(offerMessage); err == nil {
		c.Send <- offerData
	} else {
		log.Printf("Failed to marshal offer: %v", err)
	}
}

// sendError 发送错误消息
func (c *SignalingClient) sendError(message string) {
	errorMsg := SignalingMessage{
		Type:  "error",
		Error: message,
	}

	if errorData, err := json.Marshal(errorMsg); err == nil {
		c.Send <- errorData
	} else {
		log.Printf("Failed to marshal error message: %v", err)
	}
}

// handleAnswer 处理answer
func (c *SignalingClient) handleAnswer(message SignalingMessage) {
	log.Printf("Processing answer from client %s", c.ID)

	// 获取PeerConnection
	pcManager := c.Server.peerConnectionManager
	if pcManager == nil {
		log.Printf("PeerConnection manager not available")
		c.sendError("PeerConnection manager not available")
		return
	}

	pc, exists := pcManager.GetPeerConnection(c.ID)
	if !exists {
		log.Printf("PeerConnection not found for client %s", c.ID)
		c.sendError("PeerConnection not found")
		return
	}

	// 解析answer数据
	answerData, ok := message.Data.(map[string]interface{})
	if !ok {
		log.Printf("Invalid answer data format from client %s", c.ID)
		c.sendError("Invalid answer data format")
		return
	}

	sdp, ok := answerData["sdp"].(string)
	if !ok {
		log.Printf("Invalid SDP in answer from client %s", c.ID)
		c.sendError("Invalid SDP in answer")
		return
	}

	// 设置远程描述
	answer := webrtc.SessionDescription{
		Type: webrtc.SDPTypeAnswer,
		SDP:  sdp,
	}

	err := pc.SetRemoteDescription(answer)
	if err != nil {
		log.Printf("Failed to set remote description for client %s: %v", c.ID, err)
		c.sendError("Failed to set remote description")
		return
	}

	log.Printf("Successfully processed answer from client %s", c.ID)

	// 发送确认消息
	ack := SignalingMessage{
		Type:   "answer-ack",
		PeerID: c.ID,
		Data: map[string]interface{}{
			"status": "received",
		},
	}

	if ackData, err := json.Marshal(ack); err == nil {
		c.Send <- ackData
	}
}

// handleIceCandidate 处理ICE候选
func (c *SignalingClient) handleIceCandidate(message SignalingMessage) {
	log.Printf("Processing ICE candidate from client %s", c.ID)

	// 获取PeerConnection
	pcManager := c.Server.peerConnectionManager
	if pcManager == nil {
		log.Printf("PeerConnection manager not available")
		c.sendError("PeerConnection manager not available")
		return
	}

	pc, exists := pcManager.GetPeerConnection(c.ID)
	if !exists {
		log.Printf("PeerConnection not found for client %s", c.ID)
		c.sendError("PeerConnection not found")
		return
	}

	// 解析ICE候选数据
	candidateData, ok := message.Data.(map[string]interface{})
	if !ok {
		log.Printf("Invalid ICE candidate data format from client %s", c.ID)
		c.sendError("Invalid ICE candidate data format")
		return
	}

	candidate, ok := candidateData["candidate"].(string)
	if !ok {
		log.Printf("Invalid candidate in ICE candidate from client %s", c.ID)
		c.sendError("Invalid candidate in ICE candidate")
		return
	}

	sdpMid, _ := candidateData["sdpMid"].(string)
	sdpMLineIndex, _ := candidateData["sdpMLineIndex"].(float64)

	// 创建ICE候选
	iceCandidate := webrtc.ICECandidateInit{
		Candidate:     candidate,
		SDPMid:        &sdpMid,
		SDPMLineIndex: (*uint16)(&[]uint16{uint16(sdpMLineIndex)}[0]),
	}

	// 添加ICE候选
	err := pc.AddICECandidate(iceCandidate)
	if err != nil {
		log.Printf("Failed to add ICE candidate for client %s: %v", c.ID, err)
		c.sendError("Failed to add ICE candidate")
		return
	}

	log.Printf("Successfully added ICE candidate for client %s", c.ID)

	// 发送确认消息
	ack := SignalingMessage{
		Type:   "ice-ack",
		PeerID: c.ID,
		Data: map[string]interface{}{
			"status": "received",
		},
	}

	if ackData, err := json.Marshal(ack); err == nil {
		c.Send <- ackData
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
	stats := SignalingMessage{
		Type:   "stats",
		PeerID: c.ID,
		Data: map[string]interface{}{
			"connection_time": time.Since(c.LastSeen).Seconds(),
			"messages_sent":   0, // 这里应该记录实际的消息数量
			"app_name":        c.AppName,
			"client_count":    c.Server.GetClientCount(),
		},
	}

	if statsData, err := json.Marshal(stats); err == nil {
		c.Send <- statsData
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
