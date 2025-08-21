package protocol

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// GStreamerAdapter GStreamer 标准协议适配器
type GStreamerAdapter struct {
	config          *AdapterConfig
	supportedTypes  []MessageType
	validationRules map[MessageType]ValidationRule
}

// ValidationRule 验证规则
type ValidationRule struct {
	RequiredFields  []string                         `json:"required_fields"`
	OptionalFields  []string                         `json:"optional_fields"`
	DataValidator   func(data any) error             `json:"-"`
	CustomValidator func(msg *StandardMessage) error `json:"-"`
}

// NewGStreamerAdapter 创建 GStreamer 协议适配器
func NewGStreamerAdapter(config *AdapterConfig) *GStreamerAdapter {
	if config == nil {
		config = DefaultAdapterConfig(ProtocolVersionGStreamer10)
	}

	adapter := &GStreamerAdapter{
		config: config,
		supportedTypes: []MessageType{
			MessageTypeHello,
			MessageTypeOffer,
			MessageTypeAnswer,
			MessageTypeICECandidate,
			MessageTypeError,
			MessageTypePing,
			MessageTypePong,
			MessageTypeStats,
			MessageTypeRequestOffer,
			MessageTypeAnswerAck,
			MessageTypeICEAck,
			MessageTypeWelcome,
		},
		validationRules: make(map[MessageType]ValidationRule),
	}

	adapter.initValidationRules()
	return adapter
}

// GetVersion 获取协议版本
func (g *GStreamerAdapter) GetVersion() ProtocolVersion {
	return ProtocolVersionGStreamer10
}

// ParseMessage 解析消息
func (g *GStreamerAdapter) ParseMessage(data []byte) (*StandardMessage, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty message data")
	}

	if len(data) > g.config.MaxMessageSize {
		return nil, fmt.Errorf("message too large: %d bytes (max: %d)", len(data), g.config.MaxMessageSize)
	}

	var rawMessage map[string]any
	if err := json.Unmarshal(data, &rawMessage); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	// 创建标准消息
	msg := &StandardMessage{}

	// 解析基本字段
	if err := g.parseBasicFields(rawMessage, msg); err != nil {
		return nil, fmt.Errorf("failed to parse basic fields: %w", err)
	}

	// 解析数据字段
	if data, exists := rawMessage["data"]; exists {
		msg.Data = data
	}

	// 解析元数据
	if err := g.parseMetadata(rawMessage, msg); err != nil {
		return nil, fmt.Errorf("failed to parse metadata: %w", err)
	}

	// 解析错误信息
	if err := g.parseError(rawMessage, msg); err != nil {
		return nil, fmt.Errorf("failed to parse error: %w", err)
	}

	// 验证消息
	if g.config.StrictValidation {
		if err := g.ValidateMessage(msg); err != nil {
			return nil, fmt.Errorf("message validation failed: %w", err)
		}
	}

	return msg, nil
}

// FormatMessage 格式化消息
func (g *GStreamerAdapter) FormatMessage(msg *StandardMessage) ([]byte, error) {
	if msg == nil {
		return nil, fmt.Errorf("message is nil")
	}

	// 验证消息
	if g.config.StrictValidation {
		if err := g.ValidateMessage(msg); err != nil {
			return nil, fmt.Errorf("message validation failed: %w", err)
		}
	}

	// 构建输出消息
	output := make(map[string]any)

	// 基本字段
	output["version"] = string(msg.Version)
	output["type"] = string(msg.Type)
	output["id"] = msg.ID
	output["timestamp"] = msg.Timestamp

	if msg.PeerID != "" {
		output["peer_id"] = msg.PeerID
	}

	if msg.Data != nil {
		output["data"] = msg.Data
	}

	// 元数据
	if msg.Metadata != nil {
		metadata := make(map[string]any)
		metadata["protocol"] = string(msg.Metadata.Protocol)

		if msg.Metadata.ClientInfo != nil {
			clientInfo := make(map[string]any)
			if msg.Metadata.ClientInfo.UserAgent != "" {
				clientInfo["user_agent"] = msg.Metadata.ClientInfo.UserAgent
			}
			if msg.Metadata.ClientInfo.Platform != "" {
				clientInfo["platform"] = msg.Metadata.ClientInfo.Platform
			}
			if msg.Metadata.ClientInfo.Version != "" {
				clientInfo["version"] = msg.Metadata.ClientInfo.Version
			}
			if len(msg.Metadata.ClientInfo.Capabilities) > 0 {
				clientInfo["capabilities"] = msg.Metadata.ClientInfo.Capabilities
			}
			if len(clientInfo) > 0 {
				metadata["client_info"] = clientInfo
			}
		}

		if msg.Metadata.ServerInfo != nil {
			serverInfo := make(map[string]any)
			if msg.Metadata.ServerInfo.Version != "" {
				serverInfo["version"] = msg.Metadata.ServerInfo.Version
			}
			if len(msg.Metadata.ServerInfo.Capabilities) > 0 {
				serverInfo["capabilities"] = msg.Metadata.ServerInfo.Capabilities
			}
			if msg.Metadata.ServerInfo.Timestamp > 0 {
				serverInfo["timestamp"] = msg.Metadata.ServerInfo.Timestamp
			}
			if len(serverInfo) > 0 {
				metadata["server_info"] = serverInfo
			}
		}

		if len(msg.Metadata.Capabilities) > 0 {
			metadata["capabilities"] = msg.Metadata.Capabilities
		}

		if len(metadata) > 0 {
			output["metadata"] = metadata
		}
	}

	// 错误信息
	if msg.Error != nil {
		errorInfo := make(map[string]any)
		errorInfo["code"] = msg.Error.Code
		errorInfo["message"] = msg.Error.Message

		if msg.Error.Details != "" {
			errorInfo["details"] = msg.Error.Details
		}
		if msg.Error.Type != "" {
			errorInfo["type"] = msg.Error.Type
		}
		errorInfo["recoverable"] = msg.Error.Recoverable
		if len(msg.Error.Suggestions) > 0 {
			errorInfo["suggestions"] = msg.Error.Suggestions
		}

		output["error"] = errorInfo
	}

	// 序列化为JSON
	data, err := json.Marshal(output)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal message: %w", err)
	}

	if len(data) > g.config.MaxMessageSize {
		return nil, fmt.Errorf("formatted message too large: %d bytes (max: %d)", len(data), g.config.MaxMessageSize)
	}

	return data, nil
}

// ValidateMessage 验证消息
func (g *GStreamerAdapter) ValidateMessage(msg *StandardMessage) error {
	if msg == nil {
		return fmt.Errorf("message is nil")
	}

	// 验证基本字段
	if msg.Type == "" {
		return fmt.Errorf("message type is required")
	}

	if msg.ID == "" {
		return fmt.Errorf("message ID is required")
	}

	if msg.Timestamp <= 0 {
		return fmt.Errorf("message timestamp is required")
	}

	// 验证协议版本
	if msg.Version != ProtocolVersionGStreamer10 {
		return fmt.Errorf("unsupported protocol version: %s", msg.Version)
	}

	// 验证消息类型
	if !g.isMessageTypeSupported(msg.Type) {
		return fmt.Errorf("unsupported message type: %s", msg.Type)
	}

	// 应用特定的验证规则
	if rule, exists := g.validationRules[msg.Type]; exists {
		if err := g.applyValidationRule(msg, rule); err != nil {
			return fmt.Errorf("validation rule failed for type %s: %w", msg.Type, err)
		}
	}

	return nil
}

// GetSupportedMessageTypes 获取支持的消息类型
func (g *GStreamerAdapter) GetSupportedMessageTypes() []MessageType {
	return append([]MessageType(nil), g.supportedTypes...)
}

// IsCompatible 检查是否兼容指定的消息格式
func (g *GStreamerAdapter) IsCompatible(data []byte) bool {
	if len(data) == 0 {
		return false
	}

	var rawMessage map[string]any
	if err := json.Unmarshal(data, &rawMessage); err != nil {
		return false
	}

	// 检查是否有必需的字段
	msgType, hasType := rawMessage["type"]
	if !hasType {
		return false
	}

	msgTypeStr, ok := msgType.(string)
	if !ok {
		return false
	}

	// 检查消息类型是否支持
	return g.isMessageTypeSupported(MessageType(msgTypeStr))
}

// parseBasicFields 解析基本字段
func (g *GStreamerAdapter) parseBasicFields(rawMessage map[string]any, msg *StandardMessage) error {
	// 解析版本
	if version, exists := rawMessage["version"]; exists {
		if versionStr, ok := version.(string); ok {
			msg.Version = ProtocolVersion(versionStr)
		} else {
			msg.Version = ProtocolVersionGStreamer10 // 默认版本
		}
	} else {
		msg.Version = ProtocolVersionGStreamer10 // 默认版本
	}

	// 解析类型
	msgType, exists := rawMessage["type"]
	if !exists {
		return fmt.Errorf("message type is required")
	}

	msgTypeStr, ok := msgType.(string)
	if !ok {
		return fmt.Errorf("message type must be a string")
	}

	msg.Type = MessageType(msgTypeStr)

	// 解析ID
	if id, exists := rawMessage["id"]; exists {
		if idStr, ok := id.(string); ok {
			msg.ID = idStr
		} else {
			msg.ID = generateMessageID() // 生成默认ID
		}
	} else {
		msg.ID = generateMessageID() // 生成默认ID
	}

	// 解析时间戳
	if timestamp, exists := rawMessage["timestamp"]; exists {
		switch ts := timestamp.(type) {
		case float64:
			msg.Timestamp = int64(ts)
		case int64:
			msg.Timestamp = ts
		case int:
			msg.Timestamp = int64(ts)
		default:
			msg.Timestamp = time.Now().Unix() // 使用当前时间
		}
	} else {
		msg.Timestamp = time.Now().Unix() // 使用当前时间
	}

	// 解析对等ID
	if peerID, exists := rawMessage["peer_id"]; exists {
		if peerIDStr, ok := peerID.(string); ok {
			msg.PeerID = peerIDStr
		}
	}

	return nil
}

// parseMetadata 解析元数据
func (g *GStreamerAdapter) parseMetadata(rawMessage map[string]any, msg *StandardMessage) error {
	metadata, exists := rawMessage["metadata"]
	if !exists {
		return nil
	}

	metadataMap, ok := metadata.(map[string]any)
	if !ok {
		return fmt.Errorf("metadata must be an object")
	}

	msg.Metadata = &MessageMetadata{}

	// 解析协议
	if protocol, exists := metadataMap["protocol"]; exists {
		if protocolStr, ok := protocol.(string); ok {
			msg.Metadata.Protocol = ProtocolVersion(protocolStr)
		}
	}

	// 解析客户端信息
	if clientInfo, exists := metadataMap["client_info"]; exists {
		if clientInfoMap, ok := clientInfo.(map[string]any); ok {
			msg.Metadata.ClientInfo = &ClientInfo{}

			if userAgent, exists := clientInfoMap["user_agent"]; exists {
				if userAgentStr, ok := userAgent.(string); ok {
					msg.Metadata.ClientInfo.UserAgent = userAgentStr
				}
			}

			if platform, exists := clientInfoMap["platform"]; exists {
				if platformStr, ok := platform.(string); ok {
					msg.Metadata.ClientInfo.Platform = platformStr
				}
			}

			if version, exists := clientInfoMap["version"]; exists {
				if versionStr, ok := version.(string); ok {
					msg.Metadata.ClientInfo.Version = versionStr
				}
			}

			if capabilities, exists := clientInfoMap["capabilities"]; exists {
				if capabilitiesSlice, ok := capabilities.([]any); ok {
					for _, cap := range capabilitiesSlice {
						if capStr, ok := cap.(string); ok {
							msg.Metadata.ClientInfo.Capabilities = append(msg.Metadata.ClientInfo.Capabilities, capStr)
						}
					}
				}
			}
		}
	}

	// 解析服务器信息
	if serverInfo, exists := metadataMap["server_info"]; exists {
		if serverInfoMap, ok := serverInfo.(map[string]any); ok {
			msg.Metadata.ServerInfo = &ServerInfo{}

			if version, exists := serverInfoMap["version"]; exists {
				if versionStr, ok := version.(string); ok {
					msg.Metadata.ServerInfo.Version = versionStr
				}
			}

			if capabilities, exists := serverInfoMap["capabilities"]; exists {
				if capabilitiesSlice, ok := capabilities.([]any); ok {
					for _, cap := range capabilitiesSlice {
						if capStr, ok := cap.(string); ok {
							msg.Metadata.ServerInfo.Capabilities = append(msg.Metadata.ServerInfo.Capabilities, capStr)
						}
					}
				}
			}

			if timestamp, exists := serverInfoMap["timestamp"]; exists {
				switch ts := timestamp.(type) {
				case float64:
					msg.Metadata.ServerInfo.Timestamp = int64(ts)
				case int64:
					msg.Metadata.ServerInfo.Timestamp = ts
				case int:
					msg.Metadata.ServerInfo.Timestamp = int64(ts)
				}
			}
		}
	}

	// 解析能力列表
	if capabilities, exists := metadataMap["capabilities"]; exists {
		if capabilitiesSlice, ok := capabilities.([]any); ok {
			for _, cap := range capabilitiesSlice {
				if capStr, ok := cap.(string); ok {
					msg.Metadata.Capabilities = append(msg.Metadata.Capabilities, capStr)
				}
			}
		}
	}

	return nil
}

// parseError 解析错误信息
func (g *GStreamerAdapter) parseError(rawMessage map[string]any, msg *StandardMessage) error {
	errorInfo, exists := rawMessage["error"]
	if !exists {
		return nil
	}

	errorMap, ok := errorInfo.(map[string]any)
	if !ok {
		return fmt.Errorf("error must be an object")
	}

	msg.Error = &MessageError{}

	// 解析错误代码
	if code, exists := errorMap["code"]; exists {
		if codeStr, ok := code.(string); ok {
			msg.Error.Code = codeStr
		}
	}

	// 解析错误消息
	if message, exists := errorMap["message"]; exists {
		if messageStr, ok := message.(string); ok {
			msg.Error.Message = messageStr
		}
	}

	// 解析错误详情
	if details, exists := errorMap["details"]; exists {
		if detailsStr, ok := details.(string); ok {
			msg.Error.Details = detailsStr
		}
	}

	// 解析错误类型
	if errorType, exists := errorMap["type"]; exists {
		if errorTypeStr, ok := errorType.(string); ok {
			msg.Error.Type = errorTypeStr
		}
	}

	// 解析是否可恢复
	if recoverable, exists := errorMap["recoverable"]; exists {
		if recoverableBool, ok := recoverable.(bool); ok {
			msg.Error.Recoverable = recoverableBool
		}
	}

	// 解析建议
	if suggestions, exists := errorMap["suggestions"]; exists {
		if suggestionsSlice, ok := suggestions.([]any); ok {
			for _, suggestion := range suggestionsSlice {
				if suggestionStr, ok := suggestion.(string); ok {
					msg.Error.Suggestions = append(msg.Error.Suggestions, suggestionStr)
				}
			}
		}
	}

	return nil
}

// isMessageTypeSupported 检查消息类型是否支持
func (g *GStreamerAdapter) isMessageTypeSupported(msgType MessageType) bool {
	for _, supportedType := range g.supportedTypes {
		if supportedType == msgType {
			return true
		}
	}
	return false
}

// initValidationRules 初始化验证规则
func (g *GStreamerAdapter) initValidationRules() {
	// HELLO 消息验证规则
	g.validationRules[MessageTypeHello] = ValidationRule{
		RequiredFields: []string{"data"},
		DataValidator: func(data any) error {
			if data == nil {
				return fmt.Errorf("hello data is required")
			}

			var helloData HelloData
			if err := convertToStruct(data, &helloData); err != nil {
				return fmt.Errorf("invalid hello data format: %w", err)
			}

			if helloData.PeerID == "" {
				return fmt.Errorf("peer_id is required in hello data")
			}

			return nil
		},
	}

	// OFFER 消息验证规则
	g.validationRules[MessageTypeOffer] = ValidationRule{
		RequiredFields: []string{"data"},
		DataValidator: func(data any) error {
			if data == nil {
				return fmt.Errorf("offer data is required")
			}

			var sdpData SDPData
			if err := convertToStruct(data, &sdpData); err != nil {
				return fmt.Errorf("invalid offer data format: %w", err)
			}

			if sdpData.SDP == "" {
				return fmt.Errorf("sdp is required in offer data")
			}

			if sdpData.Type != "offer" {
				return fmt.Errorf("invalid sdp type for offer: %s", sdpData.Type)
			}

			return nil
		},
	}

	// ANSWER 消息验证规则
	g.validationRules[MessageTypeAnswer] = ValidationRule{
		RequiredFields: []string{"data"},
		DataValidator: func(data any) error {
			if data == nil {
				return fmt.Errorf("answer data is required")
			}

			var sdpData SDPData
			if err := convertToStruct(data, &sdpData); err != nil {
				return fmt.Errorf("invalid answer data format: %w", err)
			}

			if sdpData.SDP == "" {
				return fmt.Errorf("sdp is required in answer data")
			}

			if sdpData.Type != "answer" {
				return fmt.Errorf("invalid sdp type for answer: %s", sdpData.Type)
			}

			return nil
		},
	}

	// ICE_CANDIDATE 消息验证规则
	g.validationRules[MessageTypeICECandidate] = ValidationRule{
		RequiredFields: []string{"data"},
		DataValidator: func(data any) error {
			if data == nil {
				return fmt.Errorf("ice candidate data is required")
			}

			var iceData ICECandidateData
			if err := convertToStruct(data, &iceData); err != nil {
				return fmt.Errorf("invalid ice candidate data format: %w", err)
			}

			if iceData.Candidate == "" {
				return fmt.Errorf("candidate is required in ice candidate data")
			}

			// 验证候选格式
			if !strings.HasPrefix(iceData.Candidate, "candidate:") {
				return fmt.Errorf("invalid candidate format: must start with 'candidate:'")
			}

			return nil
		},
	}

	// ERROR 消息验证规则
	g.validationRules[MessageTypeError] = ValidationRule{
		RequiredFields: []string{"error"},
		CustomValidator: func(msg *StandardMessage) error {
			if msg.Error == nil {
				return fmt.Errorf("error information is required for error message")
			}

			if msg.Error.Code == "" {
				return fmt.Errorf("error code is required")
			}

			if msg.Error.Message == "" {
				return fmt.Errorf("error message is required")
			}

			return nil
		},
	}
}

// applyValidationRule 应用验证规则
func (g *GStreamerAdapter) applyValidationRule(msg *StandardMessage, rule ValidationRule) error {
	// 应用数据验证器
	if rule.DataValidator != nil {
		if err := rule.DataValidator(msg.Data); err != nil {
			return err
		}
	}

	// 应用自定义验证器
	if rule.CustomValidator != nil {
		if err := rule.CustomValidator(msg); err != nil {
			return err
		}
	}

	return nil
}

// convertToStruct 将任意数据转换为指定结构体
func convertToStruct(data any, target any) error {
	dataBytes, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal data: %w", err)
	}

	if err := json.Unmarshal(dataBytes, target); err != nil {
		return fmt.Errorf("failed to unmarshal data: %w", err)
	}

	return nil
}
