package webrtc

import (
	"fmt"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/open-beagle/bdwind-gstreamer/internal/config"
	"github.com/open-beagle/bdwind-gstreamer/internal/webrtc/protocol"
)

// MessageRouter 消息路由器
type MessageRouter struct {
	protocolManager *protocol.ProtocolManager
	validator       *protocol.MessageValidator
	config          *MessageRouterConfig
	logger          *logrus.Entry
}

// MessageRouterConfig 消息路由器配置
type MessageRouterConfig struct {
	EnableLogging      bool `json:"enable_logging"`
	StrictValidation   bool `json:"strict_validation"`
	AutoProtocolDetect bool `json:"auto_protocol_detect"`
	MaxRetries         int  `json:"max_retries"`
}

// RouteResult 路由结果
type RouteResult struct {
	Message          *protocol.StandardMessage  `json:"message"`
	OriginalProtocol protocol.ProtocolVersion   `json:"original_protocol"`
	ProcessedBy      string                     `json:"processed_by"`
	ValidationResult *protocol.ValidationResult `json:"validation_result"`
	Errors           []string                   `json:"errors"`
	Warnings         []string                   `json:"warnings"`
}

// NewMessageRouter 创建消息路由器
func NewMessageRouter(protocolManager *protocol.ProtocolManager, cfg *MessageRouterConfig) *MessageRouter {
	if cfg == nil {
		cfg = DefaultMessageRouterConfig()
	}

	return &MessageRouter{
		protocolManager: protocolManager,
		config:          cfg,
		logger:          config.GetLoggerWithPrefix("webrtc-message-router"),
	}
}

// DefaultMessageRouterConfig 默认消息路由器配置
func DefaultMessageRouterConfig() *MessageRouterConfig {
	return &MessageRouterConfig{
		EnableLogging:      true,
		StrictValidation:   true,
		AutoProtocolDetect: true,
		MaxRetries:         3,
	}
}

// RouteMessage 路由消息
func (mr *MessageRouter) RouteMessage(data []byte, clientID string) (*RouteResult, error) {
	result := &RouteResult{
		Errors:   make([]string, 0),
		Warnings: make([]string, 0),
	}

	mr.logger.Debugf("📨 Routing message for client %s (size: %d bytes)", clientID, len(data))

	// 使用协议管理器解析消息
	parseResult, err := mr.protocolManager.ParseMessage(data)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("parse error: %v", err))
		return result, fmt.Errorf("failed to parse message: %w", err)
	}

	result.Message = parseResult.Message
	result.OriginalProtocol = parseResult.OriginalProtocol
	result.ValidationResult = parseResult.ValidationResult
	result.ProcessedBy = "protocol_manager"

	// 收集解析过程中的错误和警告
	result.Errors = append(result.Errors, parseResult.Errors...)
	result.Warnings = append(result.Warnings, parseResult.Warnings...)

	// 设置客户端ID（如果消息中没有）
	if result.Message.PeerID == "" {
		result.Message.PeerID = clientID
	}

	// 设置时间戳（如果消息中没有）
	if result.Message.Timestamp == 0 {
		result.Message.Timestamp = time.Now().Unix()
	}

	// 设置消息ID（如果消息中没有）
	if result.Message.ID == "" {
		result.Message.ID = generateMessageID()
	}

	mr.logger.Debugf("✅ Message routed successfully: type=%s, protocol=%s, client=%s",
		result.Message.Type, result.OriginalProtocol, clientID)

	return result, nil
}

// FormatResponse 格式化响应消息
func (mr *MessageRouter) FormatResponse(message *protocol.StandardMessage, targetProtocol ...protocol.ProtocolVersion) ([]byte, error) {
	if message == nil {
		return nil, fmt.Errorf("message is nil")
	}

	// 使用协议管理器格式化消息
	data, err := mr.protocolManager.FormatMessage(message, targetProtocol...)
	if err != nil {
		return nil, fmt.Errorf("failed to format message: %w", err)
	}

	mr.logger.Debugf("📤 Response formatted: type=%s, size=%d bytes", message.Type, len(data))

	return data, nil
}

// ConvertMessage 转换消息协议
func (mr *MessageRouter) ConvertMessage(data []byte, fromProtocol, toProtocol protocol.ProtocolVersion) ([]byte, error) {
	convertedData, err := mr.protocolManager.ConvertMessage(data, fromProtocol, toProtocol)
	if err != nil {
		return nil, fmt.Errorf("failed to convert message: %w", err)
	}

	mr.logger.Debugf("🔄 Message converted: %s -> %s, size: %d -> %d bytes",
		fromProtocol, toProtocol, len(data), len(convertedData))

	return convertedData, nil
}

// DetectProtocol 检测协议类型
func (mr *MessageRouter) DetectProtocol(data []byte) *protocol.DetectionResult {
	return mr.protocolManager.DetectProtocol(data)
}

// ValidateMessage 验证消息
func (mr *MessageRouter) ValidateMessage(message *protocol.StandardMessage) *protocol.ValidationResult {
	return mr.protocolManager.ValidateMessage(message)
}

// CreateStandardResponse 创建标准响应消息
func (mr *MessageRouter) CreateStandardResponse(msgType protocol.MessageType, peerID string, data any) *protocol.StandardMessage {
	return protocol.NewStandardMessage(msgType, peerID, data)
}

// CreateErrorResponse 创建错误响应消息
func (mr *MessageRouter) CreateErrorResponse(code, message, details string) *protocol.StandardMessage {
	return protocol.NewErrorMessage(code, message, details)
}

// GetSupportedProtocols 获取支持的协议列表
func (mr *MessageRouter) GetSupportedProtocols() []protocol.ProtocolVersion {
	return mr.protocolManager.GetSupportedProtocols()
}

// GetSupportedMessageTypes 获取指定协议支持的消息类型
func (mr *MessageRouter) GetSupportedMessageTypes(protocolVersion protocol.ProtocolVersion) ([]protocol.MessageType, error) {
	return mr.protocolManager.GetSupportedMessageTypes(protocolVersion)
}

// UpdateConfig 更新配置
func (mr *MessageRouter) UpdateConfig(config *MessageRouterConfig) {
	if config != nil {
		mr.config = config
	}
}

// GetConfig 获取当前配置
func (mr *MessageRouter) GetConfig() *MessageRouterConfig {
	configCopy := *mr.config
	return &configCopy
}

// GetStats 获取统计信息
func (mr *MessageRouter) GetStats() map[string]any {
	stats := map[string]any{
		"config": mr.config,
	}

	// 添加协议管理器统计
	if mr.protocolManager != nil {
		stats["protocol_manager"] = mr.protocolManager.GetStats()
	}

	return stats
}

// HandleProtocolNegotiation 处理协议协商
func (mr *MessageRouter) HandleProtocolNegotiation(data []byte, clientID string) (*protocol.StandardMessage, error) {
	// 解析协商请求
	routeResult, err := mr.RouteMessage(data, clientID)
	if err != nil {
		return nil, fmt.Errorf("failed to parse negotiation request: %w", err)
	}

	if routeResult.Message.Type != protocol.MessageType("protocol-negotiation") {
		return nil, fmt.Errorf("invalid message type for protocol negotiation: %s", routeResult.Message.Type)
	}

	// 解析协商数据
	negotiationData, ok := routeResult.Message.Data.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid negotiation data format")
	}

	// 获取客户端支持的协议
	supportedProtocols, ok := negotiationData["supported_protocols"].([]any)
	if !ok {
		return nil, fmt.Errorf("supported_protocols field is required")
	}

	// 转换为协议版本列表
	clientProtocols := make([]protocol.ProtocolVersion, 0)
	for _, p := range supportedProtocols {
		if protocolStr, ok := p.(string); ok {
			clientProtocols = append(clientProtocols, protocol.ProtocolVersion(protocolStr))
		}
	}

	// 获取服务器支持的协议
	serverProtocols := mr.GetSupportedProtocols()

	// 找到最佳匹配协议
	selectedProtocol := mr.selectBestProtocol(clientProtocols, serverProtocols)

	// 创建协商响应
	responseData := map[string]any{
		"selected_protocol":  string(selectedProtocol),
		"server_protocols":   serverProtocols,
		"negotiation_result": "success",
		"fallback_protocols": mr.getFallbackProtocols(selectedProtocol, serverProtocols),
	}

	response := mr.CreateStandardResponse(
		protocol.MessageType("protocol-negotiation-response"),
		clientID,
		responseData,
	)

	return response, nil
}

// selectBestProtocol 选择最佳协议
func (mr *MessageRouter) selectBestProtocol(clientProtocols, serverProtocols []protocol.ProtocolVersion) protocol.ProtocolVersion {
	// 优先级顺序：GStreamer > Selkies
	priorityOrder := []protocol.ProtocolVersion{
		protocol.ProtocolVersionGStreamer10,
		protocol.ProtocolVersionSelkies,
	}

	// 按优先级查找匹配的协议
	for _, preferred := range priorityOrder {
		// 检查客户端是否支持
		clientSupports := false
		for _, clientProto := range clientProtocols {
			if clientProto == preferred {
				clientSupports = true
				break
			}
		}

		// 检查服务器是否支持
		serverSupports := false
		for _, serverProto := range serverProtocols {
			if serverProto == preferred {
				serverSupports = true
				break
			}
		}

		if clientSupports && serverSupports {
			return preferred
		}
	}

	// 如果没有找到匹配的协议，返回服务器的默认协议
	if len(serverProtocols) > 0 {
		return serverProtocols[0]
	}

	// 最后的回退
	return protocol.ProtocolVersionGStreamer10
}

// getFallbackProtocols 获取回退协议列表
func (mr *MessageRouter) getFallbackProtocols(selected protocol.ProtocolVersion, serverProtocols []protocol.ProtocolVersion) []protocol.ProtocolVersion {
	fallbacks := make([]protocol.ProtocolVersion, 0)

	for _, proto := range serverProtocols {
		if proto != selected {
			fallbacks = append(fallbacks, proto)
		}
	}

	return fallbacks
}

// HandleProtocolError 处理协议错误
func (mr *MessageRouter) HandleProtocolError(errorCode, errorMessage string, clientID string) *protocol.StandardMessage {
	errorResponse := mr.CreateErrorResponse(errorCode, errorMessage, "Protocol processing error")
	errorResponse.PeerID = clientID

	// 添加协议降级建议
	if errorResponse.Error != nil {
		errorResponse.Error.Suggestions = []string{
			"Try using a different protocol version",
			"Check message format compatibility",
			"Verify protocol negotiation",
		}
		errorResponse.Error.Recoverable = true
	}

	return errorResponse
}

// IsProtocolSupported 检查协议是否支持
func (mr *MessageRouter) IsProtocolSupported(protocolVersion protocol.ProtocolVersion) bool {
	supportedProtocols := mr.GetSupportedProtocols()
	for _, supported := range supportedProtocols {
		if supported == protocolVersion {
			return true
		}
	}
	return false
}

// GetProtocolCapabilities 获取协议能力
func (mr *MessageRouter) GetProtocolCapabilities(protocolVersion protocol.ProtocolVersion) (map[string]any, error) {
	if !mr.IsProtocolSupported(protocolVersion) {
		return nil, fmt.Errorf("protocol not supported: %s", protocolVersion)
	}

	messageTypes, err := mr.GetSupportedMessageTypes(protocolVersion)
	if err != nil {
		return nil, fmt.Errorf("failed to get message types: %w", err)
	}

	capabilities := map[string]any{
		"protocol":      string(protocolVersion),
		"message_types": messageTypes,
		"features": map[string]bool{
			"auto_detection":     true,
			"protocol_switching": true,
			"message_validation": true,
			"error_recovery":     true,
		},
	}

	return capabilities, nil
}
