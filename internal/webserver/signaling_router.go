package webserver

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/open-beagle/bdwind-gstreamer/internal/common/protocol"
	"github.com/open-beagle/bdwind-gstreamer/internal/config"
)

// SignalingRouter 信令消息路由器
// 负责解析和路由信令消息，不处理WebRTC逻辑
type SignalingRouter struct {
	server                 *SignalingServer
	protocolManager        *protocol.ProtocolManager
	businessMessageHandler func(clientID string, messageType string, data map[string]interface{}) error
	logger                 *logrus.Entry
}

// NewSignalingRouter 创建信令路由器
func NewSignalingRouter(server *SignalingServer) *SignalingRouter {
	// 创建协议管理器
	protocolConfig := protocol.DefaultManagerConfig()
	protocolConfig.EnableLogging = true
	protocolManager := protocol.NewProtocolManager(protocolConfig)

	return &SignalingRouter{
		server:          server,
		protocolManager: protocolManager,
		logger:          config.GetLoggerWithPrefix("webserver-signaling-router"),
	}
}

// HandleMessage 处理接收到的消息
func (r *SignalingRouter) HandleMessage(connInfo *ConnectionInfo, messageData []byte) {
	r.logger.Debugf("Handling message from client %s: %s", connInfo.ID, string(messageData))

	// 解析消息
	message, err := r.parseMessage(messageData)
	if err != nil {
		r.logger.Errorf("Failed to parse message from client %s: %v", connInfo.ID, err)
		r.sendError(connInfo, "PARSE_ERROR", fmt.Sprintf("Failed to parse message: %v", err))
		return
	}

	// 更新连接的最后活跃时间
	connInfo.LastSeen = time.Now()

	// 根据消息类型路由
	switch message.Type {
	case protocol.MessageTypeHello:
		r.handleHello(connInfo, message) // WebServer 自己处理的消息
	case protocol.MessageTypePing:
		r.handlePing(connInfo, message) // WebServer 自己处理的消息
	default:
		// 其他消息都是业务消息，通过回调转发给主程序处理
		r.handleBusinessMessage(connInfo, message)
	}
}

// parseMessage 解析消息
func (r *SignalingRouter) parseMessage(data []byte) (*protocol.StandardMessage, error) {
	// 尝试解析为标准消息格式
	var message protocol.StandardMessage
	if err := json.Unmarshal(data, &message); err != nil {
		return nil, fmt.Errorf("failed to unmarshal message: %w", err)
	}

	// 基本验证
	if message.Type == "" {
		return nil, fmt.Errorf("message type is required")
	}

	// 设置默认值
	if message.Version == "" {
		message.Version = protocol.ProtocolVersionGStreamer10
	}
	if message.ID == "" {
		message.ID = r.generateMessageID()
	}
	if message.Timestamp == 0 {
		message.Timestamp = time.Now().Unix()
	}

	return &message, nil
}

// handleHello 处理HELLO消息（客户端注册）
func (r *SignalingRouter) handleHello(connInfo *ConnectionInfo, message *protocol.StandardMessage) {
	r.logger.Debugf("Handling HELLO from client %s", connInfo.ID)

	// 解析HELLO数据
	helloData, ok := message.Data.(map[string]interface{})
	if !ok {
		r.sendError(connInfo, "INVALID_HELLO_DATA", "Hello data must be an object")
		return
	}

	// 获取客户端类型
	clientTypeStr, exists := helloData["client_type"].(string)
	if !exists {
		r.sendError(connInfo, "MISSING_CLIENT_TYPE", "Client type is required in hello message")
		return
	}

	clientType := ClientType(clientTypeStr)

	// 根据客户端类型注册
	switch clientType {
	case ClientTypeStreamer:
		r.handleStreamerRegistration(connInfo, helloData)
	case ClientTypeUI:
		r.handleUIRegistration(connInfo, helloData)
	default:
		r.sendError(connInfo, "INVALID_CLIENT_TYPE", fmt.Sprintf("Invalid client type: %s", clientTypeStr))
	}
}

// handleStreamerRegistration 处理推流客户端注册
func (r *SignalingRouter) handleStreamerRegistration(connInfo *ConnectionInfo, helloData map[string]interface{}) {
	streamID, exists := helloData["stream_id"].(string)
	if !exists || streamID == "" {
		r.sendError(connInfo, "MISSING_STREAM_ID", "Stream ID is required for streamer clients")
		return
	}

	// 获取能力列表
	var capabilities []string
	if caps, exists := helloData["capabilities"].([]interface{}); exists {
		for _, cap := range caps {
			if capStr, ok := cap.(string); ok {
				capabilities = append(capabilities, capStr)
			}
		}
	}

	// 注册推流客户端
	if err := r.server.RegisterStreamClient(connInfo, streamID, capabilities); err != nil {
		r.sendError(connInfo, "REGISTRATION_FAILED", fmt.Sprintf("Failed to register stream client: %v", err))
		return
	}

	// 发送注册成功响应
	response := &protocol.StandardMessage{
		Version:   protocol.ProtocolVersionGStreamer10,
		Type:      protocol.MessageTypeWelcome,
		ID:        r.generateMessageID(),
		Timestamp: time.Now().Unix(),
		Data: &protocol.WelcomeData{
			ClientID:   streamID,
			ServerTime: time.Now().Unix(),
			Protocol:   string(protocol.ProtocolVersionGStreamer10),
		},
	}

	r.server.sendMessage(connInfo, response)
	r.logger.Infof("Stream client registered successfully: %s", streamID)
}

// handleUIRegistration 处理UI客户端注册
func (r *SignalingRouter) handleUIRegistration(connInfo *ConnectionInfo, helloData map[string]interface{}) {
	sessionID, exists := helloData["session_id"].(string)
	if !exists || sessionID == "" {
		// 如果没有提供session_id，生成一个
		sessionID = fmt.Sprintf("ui_%d", time.Now().UnixNano())
	}

	// 注册UI客户端
	if err := r.server.RegisterUIClient(connInfo, sessionID); err != nil {
		r.sendError(connInfo, "REGISTRATION_FAILED", fmt.Sprintf("Failed to register UI client: %v", err))
		return
	}

	// 发送注册成功响应，包含可用的推流服务列表
	streamClients := r.server.GetStreamClients()
	availableStreams := make([]map[string]interface{}, 0, len(streamClients))

	for streamID, streamClient := range streamClients {
		availableStreams = append(availableStreams, map[string]interface{}{
			"stream_id":    streamID,
			"status":       streamClient.Status,
			"capabilities": streamClient.Capabilities,
		})
	}

	response := &protocol.StandardMessage{
		Version:   protocol.ProtocolVersionGStreamer10,
		Type:      protocol.MessageTypeWelcome,
		ID:        r.generateMessageID(),
		Timestamp: time.Now().Unix(),
		Data: map[string]interface{}{
			"client_id":         sessionID,
			"server_time":       time.Now().Unix(),
			"protocol":          string(protocol.ProtocolVersionGStreamer10),
			"available_streams": availableStreams,
		},
	}

	r.server.sendMessage(connInfo, response)
	r.logger.Infof("UI client registered successfully: %s", sessionID)
}

// handlePing 处理PING消息
func (r *SignalingRouter) handlePing(connInfo *ConnectionInfo, message *protocol.StandardMessage) {
	r.logger.Debugf("Handling PING from client %s", connInfo.ID)

	// 发送PONG响应
	response := &protocol.StandardMessage{
		Version:   message.Version,
		Type:      protocol.MessageTypePong,
		ID:        r.generateMessageID(),
		Timestamp: time.Now().Unix(),
		Data: &protocol.PongData{
			ServerState: "running",
			Sequence:    0, // 可以从ping消息中获取序列号
		},
	}

	r.server.sendMessage(connInfo, response)
}

// 所有 WebRTC 业务处理方法已移除
// 业务消息现在通过 handleBusinessMessage 和回调处理

// sendError 发送错误消息
func (r *SignalingRouter) sendError(connInfo *ConnectionInfo, code, message string) {
	errorMsg := &protocol.StandardMessage{
		Version:   protocol.ProtocolVersionGStreamer10,
		Type:      protocol.MessageTypeError,
		ID:        r.generateMessageID(),
		Timestamp: time.Now().Unix(),
		Error: &protocol.MessageError{
			Code:        code,
			Message:     message,
			Type:        "routing_error",
			Recoverable: true,
		},
	}

	if err := r.server.sendMessage(connInfo, errorMsg); err != nil {
		r.logger.Errorf("Failed to send error message to client %s: %v", connInfo.ID, err)
	}
}

// generateMessageID 生成消息ID
func (r *SignalingRouter) generateMessageID() string {
	return fmt.Sprintf("msg_%d", time.Now().UnixNano())
}

// SetBusinessMessageHandler 设置业务消息处理回调
func (r *SignalingRouter) SetBusinessMessageHandler(handler func(clientID string, messageType string, data map[string]interface{}) error) {
	r.businessMessageHandler = handler
	r.logger.Debug("Business message handler configured for signaling router")
}

// handleBusinessMessage 处理业务消息
// 将业务消息通过回调转发给主程序处理，而不是在 WebServer 中处理
func (r *SignalingRouter) handleBusinessMessage(connInfo *ConnectionInfo, message *protocol.StandardMessage) {
	r.logger.Debugf("Handling business message %s from client %s", message.Type, connInfo.ID)

	// 如果没有设置业务消息处理器，则进行简单的消息路由
	if r.businessMessageHandler == nil {
		r.logger.Debugf("No business message handler set, using simple routing for message type %s", message.Type)
		r.routeMessage(connInfo, message)
		return
	}

	// 提取消息数据
	var data map[string]interface{}
	if message.Data != nil {
		if dataMap, ok := message.Data.(map[string]interface{}); ok {
			data = dataMap
		} else {
			// 尝试将其他类型转换为 map
			data = map[string]interface{}{
				"raw_data": message.Data,
			}
		}
	} else {
		data = make(map[string]interface{})
	}

	// 添加消息的其他字段到数据中
	if message.PeerID != "" {
		data["peer_id"] = message.PeerID
	}
	if message.Timestamp != 0 {
		data["timestamp"] = message.Timestamp
	}

	// 调用业务消息处理器
	if err := r.businessMessageHandler(connInfo.ClientID, string(message.Type), data); err != nil {
		r.logger.Errorf("Business message handler error for client %s: %v", connInfo.ID, err)
		r.sendError(connInfo, "BUSINESS_HANDLER_ERROR", fmt.Sprintf("Failed to process message: %v", err))
		return
	}

	r.logger.Debugf("Business message %s processed successfully for client %s", message.Type, connInfo.ID)
}

// routeMessage 简单的消息路由（当没有业务处理器时的回退方案）
func (r *SignalingRouter) routeMessage(connInfo *ConnectionInfo, message *protocol.StandardMessage) {
	r.logger.Debugf("Routing message %s from client %s", message.Type, connInfo.ID)

	// 如果消息有目标 PeerID，尝试路由到目标客户端
	if message.PeerID != "" {
		if err := r.server.RouteMessage(connInfo.ClientID, message.PeerID, message); err != nil {
			r.sendError(connInfo, "ROUTING_FAILED", fmt.Sprintf("Failed to route message: %v", err))
			return
		}
		r.logger.Debugf("Message %s routed from %s to %s", message.Type, connInfo.ClientID, message.PeerID)
	} else {
		r.logger.Warnf("Message %s from client %s has no target, cannot route", message.Type, connInfo.ID)
		r.sendError(connInfo, "NO_TARGET", "Message has no target for routing")
	}
}
