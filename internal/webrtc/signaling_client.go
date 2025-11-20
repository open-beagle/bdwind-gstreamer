package webrtc

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"

	"github.com/open-beagle/bdwind-gstreamer/internal/common/protocol"
	"github.com/open-beagle/bdwind-gstreamer/internal/config"
	"github.com/open-beagle/bdwind-gstreamer/internal/webrtc/events"
)

// SignalingClient 信令客户端
type SignalingClient struct {
	ID               string
	AppName          string
	Conn             *websocket.Conn
	Send             chan []byte
	LastSeen         time.Time
	ConnectedAt      time.Time
	RemoteAddr       string
	UserAgent        string
	State            ClientState
	MessageCount     int64
	ErrorCount       int64
	LastError        *protocol.MessageError
	Protocol         protocol.ProtocolVersion // 客户端使用的协议版本
	ProtocolDetected bool                     // 是否已检测到协议
	logger           *logrus.Entry            // 客户端专用日志记录器
	mutex            sync.RWMutex

	// 事件总线用于与WebRTC管理器通信
	eventBus events.EventBus
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

// NewSignalingClient 创建新的信令客户端
func NewSignalingClient(appName string, conn *websocket.Conn, eventBus events.EventBus) *SignalingClient {
	clientID := generateSignalingClientID()
	now := time.Now()

	return &SignalingClient{
		ID:           clientID,
		AppName:      appName,
		Conn:         conn,
		Send:         make(chan []byte, 256),
		LastSeen:     now,
		ConnectedAt:  now,
		RemoteAddr:   conn.RemoteAddr().String(),
		UserAgent:    conn.Subprotocol(),
		State:        ClientStateConnecting,
		MessageCount: 0,
		ErrorCount:   0,
		logger:       config.GetLoggerWithPrefix(fmt.Sprintf("signaling-client-%s", clientID)),
		eventBus:     eventBus,
	}
}

// setState 设置客户端状态
func (c *SignalingClient) setState(state ClientState) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	oldState := c.State
	c.State = state

	if oldState != state {
		c.logger.Infof("Client %s state changed: %s -> %s", c.ID, oldState, state)
	}
}

// getState 获取客户端状态
func (c *SignalingClient) getState() ClientState {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.State
}

// recordError 记录错误
func (c *SignalingClient) recordError(err *protocol.MessageError) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.ErrorCount++
	c.LastError = err

	c.logger.Infof("Client %s error recorded: %s - %s", c.ID, err.Code, err.Message)
}

// incrementMessageCount 增加消息计数
func (c *SignalingClient) incrementMessageCount() {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.MessageCount++
}

// sendMessage 发送消息给客户端
func (c *SignalingClient) sendMessage(message *protocol.StandardMessage) error {
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

	// 使用消息路由器格式化消息 - TODO: Refactor in Step 3
	messageBytes, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	if len(messageBytes) > MaxMessageSize {
		return fmt.Errorf("message too large: %d bytes (max: %d)", len(messageBytes), MaxMessageSize)
	}

	select {
	case c.Send <- messageBytes:
		c.logger.Infof("📤 Standard message sent to client %s: type=%s, protocol=%s",
			c.ID, message.Type, clientProtocol)
		return nil
	default:
		return ErrSignalingSendChannelFull
	}
}

// sendError 发送错误消息给客户端
func (c *SignalingClient) sendError(signalingError *protocol.MessageError) {
	errorMessage := &protocol.StandardMessage{
		Version:   protocol.ProtocolVersionGStreamer10,
		Type:      protocol.MessageTypeError,
		ID:        generateMessageID(),
		PeerID:    c.ID,
		Timestamp: time.Now().Unix(),
		Error:     signalingError,
	}

	if err := c.sendMessage(errorMessage); err != nil {
		c.logger.Infof("Failed to send error message to client %s: %v", c.ID, err)
	}
}

// readPump 读取客户端消息
func (c *SignalingClient) readPump() {
	defer func() {
		c.setState(ClientStateDisconnected)
		c.logger.Infof("Client %s read pump exiting (connected for %v, messages: %d, errors: %d)",
			c.ID, time.Since(c.ConnectedAt), c.MessageCount, c.ErrorCount)
		// Client disconnection will be handled by the signaling server
		c.Conn.Close()
	}()

	// 设置读取超时为更长时间，避免频繁超时
	c.Conn.SetReadDeadline(time.Now().Add(300 * time.Second)) // 5分钟
	c.Conn.SetPongHandler(func(string) error {
		c.Conn.SetReadDeadline(time.Now().Add(300 * time.Second))
		c.LastSeen = time.Now()
		c.logger.Infof("🏓 Pong received from client %s", c.ID)
		return nil
	})

	c.logger.Infof("Client %s read pump started", c.ID)

	for {
		messageType, messageBytes, err := c.Conn.ReadMessage()
		if err != nil {
			// 详细的错误处理和记录
			var signalingError *protocol.MessageError

			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure, websocket.CloseNormalClosure) {
				c.logger.Infof("WebSocket unexpected close error for client %s: %v", c.ID, err)
				signalingError = &SignalingError{
					Code:    protocol.ErrorCodeConnectionLost,
					Message: "WebSocket connection lost unexpectedly",
					Details: err.Error(),
					Type:    "connection_error",
				}
			} else if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				c.logger.Infof("WebSocket connection closed normally for client %s: %v", c.ID, err)
				signalingError = &SignalingError{
					Code:    protocol.ErrorCodeConnectionLost,
					Message: "WebSocket connection closed",
					Details: err.Error(),
					Type:    "connection_info",
				}
			} else {
				c.logger.Infof("WebSocket read error for client %s: %v", c.ID, err)
				signalingError = &protocol.MessageError{
					Code:    protocol.ErrorCodeConnectionFailed,
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
			c.logger.Infof("Message too large from client %s: %d bytes (max: %d)", c.ID, len(messageBytes), MaxMessageSize)
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
			c.logger.Infof("📨 Raw message received from client %s (length: %d bytes)", c.ID, len(messageBytes))

			// 增加消息计数
			c.incrementMessageCount()

			// 使用消息路由器处理消息
			c.handleMessageWithRouter(messageBytes)
		} else {
			c.logger.Infof("Received non-text message from client %s (type: %d, length: %d)", c.ID, messageType, len(messageBytes))
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
		c.logger.Infof("Client %s write pump exiting", c.ID)
		ticker.Stop()
		c.Conn.Close()
	}()

	c.logger.Infof("Client %s write pump started", c.ID)

	for {
		select {
		case message, ok := <-c.Send:
			c.Conn.SetWriteDeadline(time.Now().Add(30 * time.Second)) // 增加写入超时时间
			if !ok {
				c.logger.Infof("Send channel closed for client %s", c.ID)
				c.Conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "Server shutting down"))
				return
			}

			if err := c.Conn.WriteMessage(websocket.TextMessage, message); err != nil {
				c.logger.Infof("❌ WebSocket write error for client %s: %v", c.ID, err)
				signalingError := &SignalingError{
					Code:    ErrorCodeConnectionFailed,
					Message: "WebSocket write error",
					Details: err.Error(),
					Type:    "connection_error",
				}
				c.recordError(signalingError)
				return
			}

			c.logger.Infof("📤 Message sent to client %s (length: %d bytes)", c.ID, len(message))

		case <-ticker.C:
			c.Conn.SetWriteDeadline(time.Now().Add(30 * time.Second))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				c.logger.Infof("❌ WebSocket ping failed for client %s: %v", c.ID, err)
				signalingError := &SignalingError{
					Code:    ErrorCodeConnectionTimeout,
					Message: "WebSocket ping failed",
					Details: err.Error(),
					Type:    "connection_error",
				}
				c.recordError(signalingError)
				return
			}
			c.logger.Infof("🏓 Ping sent to client %s", c.ID)
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

	// Simplified message handling for event-driven architecture
	// TODO: Implement proper message routing in Step 4
	processingTime := time.Since(startTime)
	c.logger.Infof("📨 Processing message from client %s (length: %d bytes, processing time: %dms)",
		c.ID, len(messageBytes), processingTime.Milliseconds())

	// For now, create a basic standard message for compatibility
	var message protocol.StandardMessage
	if err := json.Unmarshal(messageBytes, &message); err != nil {
		c.logger.Infof("❌ Failed to parse message from client %s: %v", c.ID, err)
		c.handleProtocolError("MESSAGE_PARSING_FAILED", err.Error())
		return
	}

	// Handle the standard message
	c.handleStandardMessage(&message, protocol.ProtocolVersionGStreamer10)
}

// autoDetectProtocol 自动检测客户端协议
func (c *SignalingClient) autoDetectProtocol(messageBytes []byte) {
	// 简化的协议检测，使用默认协议
	c.mutex.Lock()
	c.Protocol = protocol.ProtocolVersionGStreamer10
	c.ProtocolDetected = true
	c.mutex.Unlock()

	c.logger.Infof("🔍 Protocol set for client %s: %s (default)", c.ID, c.Protocol)

	// 通过事件总线发布协议检测事件
	if c.eventBus != nil {
		protocolEvent := events.NewSignalingEvent(
			events.EventSignalingMessage,
			c.ID,
			"protocol-detected",
			c.ID,
			map[string]interface{}{
				"protocol": c.Protocol,
				"method":   "default",
			},
		)
		c.eventBus.Publish(protocolEvent)
	}
}

// handleStandardMessage 处理标准化消息
func (c *SignalingClient) handleStandardMessage(message *protocol.StandardMessage, originalProtocol protocol.ProtocolVersion) {
	if message == nil {
		c.logger.Infof("❌ Received nil standard message from client %s", c.ID)
		return
	}

	startTime := time.Now()
	messageType := string(message.Type)

	c.logger.Infof("📨 Processing standard message from client %s: type=%s, protocol=%s",
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
	default:
		c.logger.Infof("⚠️ Unhandled message type from client %s: %s", c.ID, message.Type)
		c.sendStandardErrorMessage("UNSUPPORTED_MESSAGE_TYPE",
			fmt.Sprintf("Message type '%s' is not supported", message.Type), "")
		success = false
	}

	// 记录消息处理性能指标
	processingTime := time.Since(startTime)
	c.logger.Infof("📨 Message processed for client %s: type=%s, time=%dms, success=%t",
		c.ID, messageType, processingTime.Milliseconds(), success)
}

// handleHelloMessage 处理 HELLO 消息
func (c *SignalingClient) handleHelloMessage(message *protocol.StandardMessage) {
	c.logger.Infof("👋 Received HELLO from client %s", c.ID)

	// 解析 HELLO 数据
	var helloData protocol.HelloData
	if err := message.GetDataAs(&helloData); err != nil {
		c.logger.Infof("❌ Failed to parse HELLO data from client %s: %v", c.ID, err)
		c.sendStandardErrorMessage("INVALID_HELLO_DATA", "Failed to parse HELLO message data", err.Error())
		return
	}

	// 发送欢迎响应
	welcomeData := &protocol.HelloData{
		Capabilities: []string{"webrtc", "input", "stats", "protocol-negotiation"},
	}

	welcomeMessage := &protocol.StandardMessage{
		Version:   protocol.ProtocolVersionGStreamer10,
		Type:      protocol.MessageTypeWelcome,
		ID:        generateMessageID(),
		PeerID:    c.ID,
		Timestamp: time.Now().Unix(),
		Data:      welcomeData,
	}

	if err := c.sendStandardMessage(welcomeMessage); err != nil {
		c.logger.Infof("❌ Failed to send welcome message to client %s: %v", c.ID, err)
	}
}

// handlePingMessage 处理 PING 消息
func (c *SignalingClient) handlePingMessage(message *protocol.StandardMessage) {
	c.logger.Infof("🏓 Received PING from client %s", c.ID)

	// 创建简单的 PONG 响应
	pongData := map[string]any{
		"timestamp":   time.Now().Unix(),
		"client_id":   c.ID,
		"server_time": time.Now().Unix(),
	}

	pongMessage := &protocol.StandardMessage{
		Version:   protocol.ProtocolVersionGStreamer10,
		Type:      protocol.MessageTypePong,
		ID:        generateMessageID(),
		PeerID:    c.ID,
		Timestamp: time.Now().Unix(),
		Data:      pongData,
	}

	if err := c.sendStandardMessage(pongMessage); err != nil {
		c.logger.Infof("❌ Failed to send pong message to client %s: %v", c.ID, err)
		c.recordError(&SignalingError{
			Code:    ErrorCodeInternalError,
			Message: "Failed to send pong response",
			Details: err.Error(),
			Type:    "ping_response_error",
		})
	} else {
		c.logger.Infof("✅ Pong sent to client %s successfully", c.ID)
	}
}

// handleRequestOfferMessage 处理请求 Offer 消息
func (c *SignalingClient) handleRequestOfferMessage(message *protocol.StandardMessage) {
	c.logger.Infof("📞 Received request-offer from client %s", c.ID)

	// 使用事件系统创建 Offer
	if c.eventBus == nil {
		c.logger.Infof("❌ Event bus not available for client %s", c.ID)
		c.sendStandardErrorMessage("EVENT_BUS_UNAVAILABLE", "Event bus is not available", "")
		return
	}

	// 创建 CreateOffer 事件
	createOfferEvent := events.NewWebRTCEvent(
		events.EventCreateOffer,
		c.ID, // sessionID
		c.ID, // peerID
		map[string]interface{}{
			"request_data": message.Data,
		},
	)

	// 同步发布事件并等待结果
	result, err := c.eventBus.PublishSync(createOfferEvent)
	if err != nil || result == nil || !result.Success {
		errorMsg := "Unknown error"
		if err != nil {
			errorMsg = err.Error()
		} else if result != nil && result.Error != "" {
			errorMsg = result.Error
		}

		c.logger.Infof("❌ Failed to create offer for client %s: %v", c.ID, errorMsg)
		c.sendStandardErrorMessage("OFFER_CREATION_FAILED", "Failed to create SDP offer", errorMsg)
		return
	}

	// 从事件结果中提取 SDP offer
	offerData, ok := result.Data["offer"].(map[string]interface{})
	if !ok {
		c.logger.Infof("❌ Invalid offer data format from event result for client %s", c.ID)
		c.sendStandardErrorMessage("INVALID_OFFER_DATA", "Invalid offer data format", "")
		return
	}

	sdpType, _ := offerData["type"].(string)
	sdpContent, _ := offerData["sdp"].(string)

	c.logger.Infof("✅ SDP offer created for client %s (type: %s, length: %d bytes)", c.ID, sdpType, len(sdpContent))

	// 发送 Offer
	sdpData := &protocol.SDPData{
		SDP: &protocol.SDPContent{
			Type: sdpType,
			SDP:  sdpContent,
		},
	}

	offerMessage := &protocol.StandardMessage{
		Version:   protocol.ProtocolVersionGStreamer10,
		Type:      protocol.MessageTypeOffer,
		ID:        generateMessageID(),
		PeerID:    c.ID,
		Timestamp: time.Now().Unix(),
		Data:      sdpData,
	}

	if err := c.sendStandardMessage(offerMessage); err != nil {
		c.logger.Infof("❌ Failed to send offer to client %s: %v", c.ID, err)
		c.recordError(&SignalingError{
			Code:    ErrorCodeInternalError,
			Message: "Failed to send offer response",
			Details: err.Error(),
			Type:    "offer_response_error",
		})
	} else {
		c.logger.Infof("✅ Offer sent to client %s successfully", c.ID)
	}
}

// handleAnswerMessage 处理 Answer 消息
func (c *SignalingClient) handleAnswerMessage(message *protocol.StandardMessage) {
	c.logger.Infof("📞 Processing Answer SDP from client %s (protocol step 2/3)", c.ID)

	// 解析 SDP Answer
	var sdpData protocol.SDPData
	if err := message.GetDataAs(&sdpData); err != nil {
		c.logger.Infof("❌ Failed to parse answer data from client %s: %v", c.ID, err)
		c.sendStandardErrorMessage("INVALID_ANSWER_DATA", "Failed to parse answer data", err.Error())
		return
	}

	// 使用事件系统设置远程描述
	if c.eventBus == nil {
		c.logger.Infof("❌ Event bus not available for client %s", c.ID)
		c.sendStandardErrorMessage("EVENT_BUS_UNAVAILABLE",
			"Event bus is not available", "")
		return
	}

	// 创建 ProcessAnswer 事件
	processAnswerEvent := events.NewWebRTCEvent(
		events.EventProcessAnswer,
		c.ID, // sessionID
		c.ID, // peerID
		map[string]interface{}{
			"answer": map[string]interface{}{
				"type": "answer",
				"sdp":  sdpData.SDP.SDP,
			},
		},
	)

	// 同步发布事件并等待结果
	result, err := c.eventBus.PublishSync(processAnswerEvent)
	if err != nil || result == nil || !result.Success {
		errorMsg := "Unknown error"
		if err != nil {
			errorMsg = err.Error()
		} else if result != nil && result.Error != "" {
			errorMsg = result.Error
		}

		c.logger.Infof("❌ Failed to set remote description for client %s: %v", c.ID, errorMsg)
		c.sendStandardErrorMessage("REMOTE_DESCRIPTION_FAILED",
			"Failed to set remote description", errorMsg)
		return
	}

	// Answer 处理成功，ICE 候选收集将自动开始
	c.logger.Infof("✅ Answer SDP processed successfully for client %s", c.ID)
	c.logger.Infof("🧊 ICE candidate collection started automatically for client %s (no ACK message sent)", c.ID)
	c.logger.Infof("📋 Protocol flow: Offer -> Answer -> ICE candidates (correct flow) for client %s", c.ID)
}

// handleICECandidateMessage 处理 ICE 候选消息
func (c *SignalingClient) handleICECandidateMessage(message *protocol.StandardMessage) {
	c.logger.Infof("🧊 Received ICE candidate from client %s", c.ID)

	// 解析 ICE 候选数据
	var iceData protocol.ICECandidateData
	if err := message.GetDataAs(&iceData); err != nil {
		c.logger.Infof("❌ ICE candidate parsing failed for client %s: %v", c.ID, err)
		c.recordError(&SignalingError{
			Code:    "INVALID_ICE_DATA",
			Message: "Failed to parse ICE candidate data",
			Details: err.Error(),
			Type:    "parsing_error",
		})
		c.sendStandardErrorMessage("INVALID_ICE_DATA", "Failed to parse ICE candidate data", err.Error())
		return
	}

	// 验证ICE候选数据完整性
	if iceData.Candidate.Candidate == "" {
		c.logger.Infof("❌ ICE candidate validation failed for client %s: empty candidate string", c.ID)
		c.sendStandardErrorMessage("INVALID_ICE_DATA", "ICE candidate string cannot be empty", "")
		return
	}

	// 使用事件系统处理 ICE candidate
	if c.eventBus == nil {
		c.logger.Infof("❌ Event bus not available for client %s", c.ID)
		c.sendStandardErrorMessage("EVENT_BUS_UNAVAILABLE", "Event bus is not available", "")
		return
	}

	// 创建 AddICECandidate 事件
	addICEEvent := events.NewWebRTCEvent(
		events.EventAddICECandidate,
		c.ID, // sessionID
		c.ID, // peerID
		map[string]interface{}{
			"candidate": map[string]interface{}{
				"candidate":     iceData.Candidate.Candidate,
				"sdpMid":        iceData.Candidate.SDPMid,
				"sdpMLineIndex": iceData.Candidate.SDPMLineIndex,
			},
		},
	)

	// 同步发布事件并等待结果
	result, err := c.eventBus.PublishSync(addICEEvent)
	if err != nil || result == nil || !result.Success {
		errorMsg := "Unknown error"
		if err != nil {
			errorMsg = err.Error()
		} else if result != nil && result.Error != "" {
			errorMsg = result.Error
		}

		c.logger.Infof("❌ ICE candidate processing failed for client %s: %v", c.ID, errorMsg)
		c.recordError(&SignalingError{
			Code:    "ICE_CANDIDATE_FAILED",
			Message: "Failed to handle ICE candidate",
			Details: errorMsg,
			Type:    "webrtc_error",
		})
		c.sendStandardErrorMessage("ICE_CANDIDATE_FAILED", "Failed to handle ICE candidate", errorMsg)
		return
	}

	c.logger.Infof("✅ ICE candidate processed successfully for client %s", c.ID)
}

// handleProtocolNegotiationMessage 处理协议协商消息
func (c *SignalingClient) handleProtocolNegotiationMessage(message *protocol.StandardMessage) {
	c.logger.Infof("🤝 Received protocol negotiation from client %s", c.ID)

	// 创建简单的协议协商响应
	response := &protocol.StandardMessage{
		Version:   protocol.ProtocolVersionGStreamer10,
		Type:      protocol.MessageType("protocol-negotiation-response"),
		ID:        generateMessageID(),
		PeerID:    c.ID,
		Timestamp: time.Now().Unix(),
		Data: map[string]interface{}{
			"success":           true,
			"selected_protocol": protocol.ProtocolVersionGStreamer10,
			"capabilities":      []string{"webrtc", "input", "stats"},
		},
	}

	// 发送协商响应
	if err := c.sendStandardMessage(response); err != nil {
		c.logger.Infof("❌ Failed to send protocol negotiation response to client %s: %v", c.ID, err)
	} else {
		c.logger.Infof("✅ Protocol negotiation completed for client %s", c.ID)
	}
}

// handleProtocolError 处理协议错误
func (c *SignalingClient) handleProtocolError(errorCode, errorMessage string) {
	c.logger.Infof("❌ Protocol error for client %s: %s - %s", c.ID, errorCode, errorMessage)

	// 简化的协议降级处理
	if c.Protocol != "" {
		c.logger.Infof("🔄 Protocol error for client %s, maintaining current protocol: %s", c.ID, c.Protocol)

		// 发送协议错误通知
		errorNotification := &protocol.StandardMessage{
			Version:   protocol.ProtocolVersionGStreamer10,
			Type:      protocol.MessageType("protocol-error"),
			ID:        generateMessageID(),
			PeerID:    c.ID,
			Timestamp: time.Now().Unix(),
			Data: map[string]interface{}{
				"error_code": errorCode,
				"message":    errorMessage,
				"protocol":   c.Protocol,
			},
		}

		if err := c.sendStandardMessage(errorNotification); err != nil {
			c.logger.Infof("❌ Failed to send protocol error notification to client %s: %v", c.ID, err)
		}
		return
	}

	// 如果无法降级，发送错误消息
	c.sendStandardErrorMessage(errorCode, errorMessage, "Protocol error occurred")
}

// sendStandardErrorMessage 发送标准错误消息
func (c *SignalingClient) sendStandardErrorMessage(code, message, details string) {
	c.logger.Infof("🚨 Sending error to client %s - Code: %s, Message: %s", c.ID, code, message)

	errorMessage := &protocol.StandardMessage{
		Version:   protocol.ProtocolVersionGStreamer10,
		Type:      protocol.MessageTypeError,
		ID:        generateMessageID(),
		PeerID:    c.ID,
		Timestamp: time.Now().Unix(),
		Error: &protocol.MessageError{
			Code:    code,
			Message: message,
			Details: details,
			Type:    "client_error",
		},
	}

	if err := c.sendStandardMessage(errorMessage); err != nil {
		c.logger.Infof("❌ Failed to send error message to client %s: %v", c.ID, err)
		c.recordError(&SignalingError{
			Code:    ErrorCodeInternalError,
			Message: "Failed to send error message",
			Details: err.Error(),
			Type:    "error_send_failure",
		})
	}
}
