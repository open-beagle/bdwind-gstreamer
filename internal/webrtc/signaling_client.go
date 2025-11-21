package webrtc

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/open-beagle/bdwind-gstreamer/internal/common/events"
	"github.com/open-beagle/bdwind-gstreamer/internal/common/protocol"
	"github.com/open-beagle/bdwind-gstreamer/internal/common/config"
	webrtcEvents "github.com/open-beagle/bdwind-gstreamer/internal/webrtc/events"
)

// SendFunc 定义发送消息的函数类型
type SendFunc func(message *protocol.StandardMessage) error

// SignalingClient 信令客户端
// 负责处理 WebRTC 相关的业务逻辑，通过 EventBus 与 WebRTCManager 交互
type SignalingClient struct {
	ID               string
	AppName          string
	SendFunc         SendFunc // 发送消息的回调函数
	LastSeen         time.Time
	ConnectedAt      time.Time
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
func NewSignalingClient(id string, appName string, sendFunc SendFunc, eventBus events.EventBus) *SignalingClient {
	now := time.Now()

	client := &SignalingClient{
		ID:           id,
		AppName:      appName,
		SendFunc:     sendFunc,
		LastSeen:     now,
		ConnectedAt:  now,
		State:        ClientStateConnected,
		MessageCount: 0,
		ErrorCount:   0,
		logger:       config.GetLoggerWithPrefix(fmt.Sprintf("signaling-client-%s", id)),
		eventBus:     eventBus,
	}

	// 订阅本地 ICE candidate 事件
	if eventBus != nil {
		eventBus.Subscribe(webrtcEvents.EventOnICECandidate, events.EventHandlerFunc(client.handleOnICECandidate))
	}

	return client
}

// SetState 设置客户端状态
func (c *SignalingClient) SetState(state ClientState) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	oldState := c.State
	c.State = state

	if oldState != state {
		c.logger.Infof("Client %s state changed: %s -> %s", c.ID, oldState, state)
	}
}

// GetState 获取客户端状态
func (c *SignalingClient) GetState() ClientState {
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

// HandleMessage 处理接收到的消息
func (c *SignalingClient) HandleMessage(message *protocol.StandardMessage) {
	if message == nil {
		c.logger.Warnf("Received nil message for client %s", c.ID)
		return
	}

	c.mutex.Lock()
	c.LastSeen = time.Now()
	c.MessageCount++
	// 如果未检测到协议，默认使用 GStreamer1.0
	if !c.ProtocolDetected {
		c.Protocol = protocol.ProtocolVersionGStreamer10
		c.ProtocolDetected = true
	}
	c.mutex.Unlock()

	startTime := time.Now()
	messageType := string(message.Type)

	c.logger.Infof("📨 Processing message from client %s: type=%s", c.ID, messageType)

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

// sendStandardMessage 发送标准化消息给客户端
func (c *SignalingClient) sendStandardMessage(message *protocol.StandardMessage) error {
	if message == nil {
		return fmt.Errorf("message is nil")
	}

	if c.SendFunc == nil {
		return fmt.Errorf("send function is not set")
	}

	// 获取客户端协议版本
	c.mutex.RLock()
	clientProtocol := c.Protocol
	c.mutex.RUnlock()

	// 如果未检测到协议，使用默认协议
	if clientProtocol == "" {
		clientProtocol = protocol.ProtocolVersionGStreamer10
	}

	c.logger.Infof("📤 Sending standard message to client %s: type=%s, protocol=%s",
		c.ID, message.Type, clientProtocol)

	return c.SendFunc(message)
}

// sendError 发送错误消息给客户端
func (c *SignalingClient) sendError(signalingError *protocol.MessageError) {
	c.sendStandardErrorMessage(signalingError.Code, signalingError.Message, signalingError.Details)
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
	createOfferEvent := webrtcEvents.NewWebRTCEvent(
		webrtcEvents.EventCreateOffer,
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
	processAnswerEvent := webrtcEvents.NewWebRTCEvent(
		webrtcEvents.EventProcessAnswer,
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
	addICEEvent := webrtcEvents.NewWebRTCEvent(
		webrtcEvents.EventAddICECandidate,
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

// handleOnICECandidate 处理本地生成的 ICE candidate 事件
func (c *SignalingClient) handleOnICECandidate(ctx context.Context, event events.Event) (*events.EventResult, error) {
	// 检查是否是当前会话的 candidate
	if event.SessionID() != c.ID {
		return nil, nil // 忽略其他会话的事件
	}

	c.logger.Debugf("Handling OnICECandidate event for session: %s", event.SessionID())

	// 提取 candidate 数据
	eventData := event.Data()
	candidateData, ok := eventData["candidate"].(map[string]interface{})
	if !ok {
		c.logger.Error("Invalid candidate data format in event")
		return nil, fmt.Errorf("invalid candidate data format")
	}

	// 构造发送给客户端的消息
	// 将 candidateData 的内容直接放到 Data 中，而不是嵌套在 candidate 字段下
	iceMessage := &protocol.StandardMessage{
		Version:   protocol.ProtocolVersionGStreamer10,
		Type:      protocol.MessageTypeICECandidate,
		ID:        generateMessageID(),
		PeerID:    c.ID,
		Timestamp: time.Now().Unix(),
		Data:      candidateData, // 直接使用 candidateData
	}

	// 发送给客户端
	if err := c.sendStandardMessage(iceMessage); err != nil {
		c.logger.Errorf("Failed to send ICE candidate to client: %v", err)
		return nil, err
	}

	c.logger.Infof("📤 Sent ICE candidate to client %s", c.ID)
	return events.SuccessResult("ICE candidate sent to client", nil), nil
}
