package protocol

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

// SelkiesAdapter Selkies 兼容协议适配器
type SelkiesAdapter struct {
	config          *AdapterConfig
	protocolMapping map[string]MessageType
	legacyMode      bool
}

// SelkiesMessage Selkies 原始消息格式
type SelkiesMessage struct {
	Type   string `json:"type,omitempty"`
	SDP    any    `json:"sdp,omitempty"`
	ICE    any    `json:"ice,omitempty"`
	Data   any    `json:"data,omitempty"`
	PeerID string `json:"peer_id,omitempty"`
}

// SelkiesHelloMessage Selkies HELLO 消息格式
type SelkiesHelloMessage struct {
	Command  string         `json:"command"`
	PeerID   string         `json:"peer_id"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// NewSelkiesAdapter 创建 Selkies 协议适配器
func NewSelkiesAdapter(config *AdapterConfig) *SelkiesAdapter {
	if config == nil {
		config = DefaultAdapterConfig(ProtocolVersionSelkies)
		config.Version = ProtocolVersionSelkies
	}

	adapter := &SelkiesAdapter{
		config:     config,
		legacyMode: true,
		protocolMapping: map[string]MessageType{
			"hello":         MessageTypeHello,
			"sdp":           MessageTypeOffer, // Selkies 使用 "sdp" 表示 offer/answer
			"ice":           MessageTypeICECandidate,
			"error":         MessageTypeError,
			"ping":          MessageTypePing,
			"pong":          MessageTypePong,
			"stats":         MessageTypeStats,
			"request-offer": MessageTypeRequestOffer,
		},
	}

	return adapter
}

// GetVersion 获取协议版本
func (s *SelkiesAdapter) GetVersion() ProtocolVersion {
	return ProtocolVersionSelkies
}

// ParseMessage 解析消息
func (s *SelkiesAdapter) ParseMessage(data []byte) (*StandardMessage, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty message data")
	}

	if len(data) > s.config.MaxMessageSize {
		return nil, fmt.Errorf("message too large: %d bytes (max: %d)", len(data), s.config.MaxMessageSize)
	}

	// 尝试解析为文本消息 (HELLO 格式)
	messageText := string(data)
	if strings.HasPrefix(messageText, "HELLO ") {
		return s.parseHelloMessage(messageText)
	}

	// 尝试解析为 JSON 消息
	var rawMessage map[string]any
	if err := json.Unmarshal(data, &rawMessage); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	// 检查是否是 Selkies 格式的 SDP 消息
	if sdpData, exists := rawMessage["sdp"]; exists {
		return s.parseSDPMessage(sdpData, rawMessage)
	}

	// 检查是否是 Selkies 格式的 ICE 消息
	if iceData, exists := rawMessage["ice"]; exists {
		return s.parseICEMessage(iceData, rawMessage)
	}

	// 尝试解析为标准 JSON 消息格式
	return s.parseStandardMessage(rawMessage)
}

// FormatMessage 格式化消息
func (s *SelkiesAdapter) FormatMessage(msg *StandardMessage) ([]byte, error) {
	if msg == nil {
		return nil, fmt.Errorf("message is nil")
	}

	// 根据消息类型选择格式化方式
	switch msg.Type {
	case MessageTypeHello:
		return s.formatHelloMessage(msg)
	case MessageTypeOffer, MessageTypeAnswer:
		return s.formatSDPMessage(msg)
	case MessageTypeICECandidate:
		return s.formatICEMessage(msg)
	case MessageTypeError:
		return s.formatErrorMessage(msg)
	default:
		// 对于其他消息类型，使用标准格式
		return s.formatStandardMessage(msg)
	}
}

// ValidateMessage 验证消息
func (s *SelkiesAdapter) ValidateMessage(msg *StandardMessage) error {
	if msg == nil {
		return fmt.Errorf("message is nil")
	}

	// 基本验证
	if msg.Type == "" {
		return fmt.Errorf("message type is required")
	}

	// 验证消息类型是否支持
	if !s.isMessageTypeSupported(msg.Type) {
		return fmt.Errorf("unsupported message type: %s", msg.Type)
	}

	// 根据消息类型进行特定验证
	switch msg.Type {
	case MessageTypeHello:
		return s.validateHelloMessage(msg)
	case MessageTypeOffer, MessageTypeAnswer:
		return s.validateSDPMessage(msg)
	case MessageTypeICECandidate:
		return s.validateICEMessage(msg)
	case MessageTypeError:
		return s.validateErrorMessage(msg)
	}

	return nil
}

// GetSupportedMessageTypes 获取支持的消息类型
func (s *SelkiesAdapter) GetSupportedMessageTypes() []MessageType {
	types := make([]MessageType, 0, len(s.protocolMapping))
	for _, msgType := range s.protocolMapping {
		types = append(types, msgType)
	}
	return types
}

// IsCompatible 检查是否兼容指定的消息格式
func (s *SelkiesAdapter) IsCompatible(data []byte) bool {
	if len(data) == 0 {
		return false
	}

	messageText := string(data)

	// 检查 HELLO 消息格式
	if strings.HasPrefix(messageText, "HELLO ") {
		return true
	}

	// 检查 JSON 格式
	var rawMessage map[string]any
	if err := json.Unmarshal(data, &rawMessage); err != nil {
		return false
	}

	// 检查是否有 Selkies 特有的字段
	if _, hasSDP := rawMessage["sdp"]; hasSDP {
		return true
	}

	if _, hasICE := rawMessage["ice"]; hasICE {
		return true
	}

	// 检查是否有标准字段但使用 Selkies 消息类型
	if msgType, hasType := rawMessage["type"]; hasType {
		if msgTypeStr, ok := msgType.(string); ok {
			_, isSelkiesType := s.protocolMapping[msgTypeStr]
			return isSelkiesType
		}
	}

	return false
}

// ConvertFromSelkies 从 Selkies 格式转换为标准格式
func (s *SelkiesAdapter) ConvertFromSelkies(data []byte) (*StandardMessage, error) {
	return s.ParseMessage(data)
}

// ConvertToSelkies 从标准格式转换为 Selkies 格式
func (s *SelkiesAdapter) ConvertToSelkies(msg *StandardMessage) ([]byte, error) {
	return s.FormatMessage(msg)
}

// parseHelloMessage 解析 HELLO 消息
func (s *SelkiesAdapter) parseHelloMessage(messageText string) (*StandardMessage, error) {
	// 解析 HELLO 消息: "HELLO ${peer_id} ${btoa(JSON.stringify(meta))}"
	parts := strings.SplitN(messageText, " ", 3)
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid HELLO message format")
	}

	peerID := parts[1]
	var metadata map[string]any

	// 解析元数据 (如果存在)
	if len(parts) >= 3 {
		metaEncoded := parts[2]
		if metaBytes, err := base64.StdEncoding.DecodeString(metaEncoded); err == nil {
			if err := json.Unmarshal(metaBytes, &metadata); err != nil {
				// 忽略元数据解析错误，继续处理
				metadata = make(map[string]any)
			}
		}
	}

	// 创建标准消息
	msg := NewStandardMessage(MessageTypeHello, peerID, HelloData{
		PeerID:   peerID,
		Metadata: metadata,
	})

	msg.Version = ProtocolVersionSelkies

	// 设置元数据
	if msg.Metadata == nil {
		msg.Metadata = &MessageMetadata{}
	}
	msg.Metadata.Protocol = ProtocolVersionSelkies

	return msg, nil
}

// parseSDPMessage 解析 SDP 消息
func (s *SelkiesAdapter) parseSDPMessage(sdpData any, rawMessage map[string]any) (*StandardMessage, error) {
	sdpMap, ok := sdpData.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid SDP data format")
	}

	// 提取 SDP 信息
	sdpType, _ := sdpMap["type"].(string)
	sdpContent, _ := sdpMap["sdp"].(string)

	if sdpContent == "" {
		return nil, fmt.Errorf("SDP content is required")
	}

	// 确定消息类型
	var msgType MessageType
	if sdpType == "offer" {
		msgType = MessageTypeOffer
	} else if sdpType == "answer" {
		msgType = MessageTypeAnswer
	} else {
		// 默认为 offer
		msgType = MessageTypeOffer
	}

	// 提取对等ID
	peerID, _ := rawMessage["peer_id"].(string)

	// 创建标准消息
	msg := NewStandardMessage(msgType, peerID, SDPData{
		Type: sdpType,
		SDP:  sdpContent,
	})

	msg.Version = ProtocolVersionSelkies

	// 设置元数据
	if msg.Metadata == nil {
		msg.Metadata = &MessageMetadata{}
	}
	msg.Metadata.Protocol = ProtocolVersionSelkies

	return msg, nil
}

// parseICEMessage 解析 ICE 消息
func (s *SelkiesAdapter) parseICEMessage(iceData any, rawMessage map[string]any) (*StandardMessage, error) {
	iceMap, ok := iceData.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid ICE data format")
	}

	// 提取 ICE 候选信息
	candidate, _ := iceMap["candidate"].(string)
	if candidate == "" {
		return nil, fmt.Errorf("ICE candidate is required")
	}

	// 提取其他字段
	sdpMid, _ := iceMap["sdpMid"].(string)
	var sdpMLineIndex *uint16
	if index, exists := iceMap["sdpMLineIndex"]; exists {
		switch idx := index.(type) {
		case float64:
			val := uint16(idx)
			sdpMLineIndex = &val
		case int:
			val := uint16(idx)
			sdpMLineIndex = &val
		}
	}

	usernameFragment, _ := iceMap["usernameFragment"].(string)

	// 提取对等ID
	peerID, _ := rawMessage["peer_id"].(string)

	// 创建标准消息
	msg := NewStandardMessage(MessageTypeICECandidate, peerID, ICECandidateData{
		Candidate:        candidate,
		SDPMid:           &sdpMid,
		SDPMLineIndex:    sdpMLineIndex,
		UsernameFragment: &usernameFragment,
	})

	msg.Version = ProtocolVersionSelkies

	// 设置元数据
	if msg.Metadata == nil {
		msg.Metadata = &MessageMetadata{}
	}
	msg.Metadata.Protocol = ProtocolVersionSelkies

	return msg, nil
}

// parseStandardMessage 解析标准消息
func (s *SelkiesAdapter) parseStandardMessage(rawMessage map[string]any) (*StandardMessage, error) {
	// 提取消息类型
	msgTypeRaw, exists := rawMessage["type"]
	if !exists {
		return nil, fmt.Errorf("message type is required")
	}

	msgTypeStr, ok := msgTypeRaw.(string)
	if !ok {
		return nil, fmt.Errorf("message type must be a string")
	}

	// 映射消息类型
	msgType, exists := s.protocolMapping[msgTypeStr]
	if !exists {
		return nil, fmt.Errorf("unsupported message type: %s", msgTypeStr)
	}

	// 提取其他字段
	peerID, _ := rawMessage["peer_id"].(string)
	data := rawMessage["data"]

	// 创建标准消息
	msg := NewStandardMessage(msgType, peerID, data)
	msg.Version = ProtocolVersionSelkies

	// 设置元数据
	if msg.Metadata == nil {
		msg.Metadata = &MessageMetadata{}
	}
	msg.Metadata.Protocol = ProtocolVersionSelkies

	return msg, nil
}

// formatHelloMessage 格式化 HELLO 消息
func (s *SelkiesAdapter) formatHelloMessage(msg *StandardMessage) ([]byte, error) {
	var helloData HelloData
	if err := msg.GetDataAs(&helloData); err != nil {
		return nil, fmt.Errorf("invalid hello data: %w", err)
	}

	// 构建 HELLO 消息: "HELLO ${peer_id} ${btoa(JSON.stringify(meta))}"
	helloMsg := fmt.Sprintf("HELLO %s", helloData.PeerID)

	if helloData.Metadata != nil && len(helloData.Metadata) > 0 {
		metaBytes, err := json.Marshal(helloData.Metadata)
		if err == nil {
			metaEncoded := base64.StdEncoding.EncodeToString(metaBytes)
			helloMsg += " " + metaEncoded
		}
	}

	return []byte(helloMsg), nil
}

// formatSDPMessage 格式化 SDP 消息
func (s *SelkiesAdapter) formatSDPMessage(msg *StandardMessage) ([]byte, error) {
	var sdpData SDPData
	if err := msg.GetDataAs(&sdpData); err != nil {
		return nil, fmt.Errorf("invalid SDP data: %w", err)
	}

	// 构建 Selkies SDP 消息格式
	output := map[string]any{
		"sdp": map[string]any{
			"type": sdpData.Type,
			"sdp":  sdpData.SDP,
		},
	}

	if msg.PeerID != "" {
		output["peer_id"] = msg.PeerID
	}

	return json.Marshal(output)
}

// formatICEMessage 格式化 ICE 消息
func (s *SelkiesAdapter) formatICEMessage(msg *StandardMessage) ([]byte, error) {
	var iceData ICECandidateData
	if err := msg.GetDataAs(&iceData); err != nil {
		return nil, fmt.Errorf("invalid ICE data: %w", err)
	}

	// 构建 Selkies ICE 消息格式
	iceOutput := map[string]any{
		"candidate": iceData.Candidate,
	}

	if iceData.SDPMid != nil {
		iceOutput["sdpMid"] = *iceData.SDPMid
	}

	if iceData.SDPMLineIndex != nil {
		iceOutput["sdpMLineIndex"] = *iceData.SDPMLineIndex
	}

	if iceData.UsernameFragment != nil {
		iceOutput["usernameFragment"] = *iceData.UsernameFragment
	}

	output := map[string]any{
		"ice": iceOutput,
	}

	if msg.PeerID != "" {
		output["peer_id"] = msg.PeerID
	}

	return json.Marshal(output)
}

// formatErrorMessage 格式化错误消息
func (s *SelkiesAdapter) formatErrorMessage(msg *StandardMessage) ([]byte, error) {
	if msg.Error == nil {
		return nil, fmt.Errorf("error information is required")
	}

	// 构建简单的错误消息格式
	errorMsg := fmt.Sprintf("ERROR %s", msg.Error.Message)
	return []byte(errorMsg), nil
}

// formatStandardMessage 格式化标准消息
func (s *SelkiesAdapter) formatStandardMessage(msg *StandardMessage) ([]byte, error) {
	// 找到对应的 Selkies 消息类型
	var selkiesType string
	for selkiesT, standardT := range s.protocolMapping {
		if standardT == msg.Type {
			selkiesType = selkiesT
			break
		}
	}

	if selkiesType == "" {
		return nil, fmt.Errorf("unsupported message type for Selkies format: %s", msg.Type)
	}

	// 构建输出消息
	output := map[string]any{
		"type": selkiesType,
	}

	if msg.PeerID != "" {
		output["peer_id"] = msg.PeerID
	}

	if msg.Data != nil {
		output["data"] = msg.Data
	}

	return json.Marshal(output)
}

// validateHelloMessage 验证 HELLO 消息
func (s *SelkiesAdapter) validateHelloMessage(msg *StandardMessage) error {
	if msg.Data == nil {
		return fmt.Errorf("hello data is required")
	}

	var helloData HelloData
	if err := msg.GetDataAs(&helloData); err != nil {
		return fmt.Errorf("invalid hello data format: %w", err)
	}

	if helloData.PeerID == "" {
		return fmt.Errorf("peer_id is required in hello data")
	}

	return nil
}

// validateSDPMessage 验证 SDP 消息
func (s *SelkiesAdapter) validateSDPMessage(msg *StandardMessage) error {
	if msg.Data == nil {
		return fmt.Errorf("SDP data is required")
	}

	var sdpData SDPData
	if err := msg.GetDataAs(&sdpData); err != nil {
		return fmt.Errorf("invalid SDP data format: %w", err)
	}

	if sdpData.SDP == "" {
		return fmt.Errorf("SDP content is required")
	}

	// 验证 SDP 类型
	if msg.Type == MessageTypeOffer && sdpData.Type != "offer" {
		return fmt.Errorf("invalid SDP type for offer message: %s", sdpData.Type)
	}

	if msg.Type == MessageTypeAnswer && sdpData.Type != "answer" {
		return fmt.Errorf("invalid SDP type for answer message: %s", sdpData.Type)
	}

	return nil
}

// validateICEMessage 验证 ICE 消息
func (s *SelkiesAdapter) validateICEMessage(msg *StandardMessage) error {
	if msg.Data == nil {
		return fmt.Errorf("ICE data is required")
	}

	var iceData ICECandidateData
	if err := msg.GetDataAs(&iceData); err != nil {
		return fmt.Errorf("invalid ICE data format: %w", err)
	}

	if iceData.Candidate == "" {
		return fmt.Errorf("ICE candidate is required")
	}

	// 验证候选格式
	if !strings.HasPrefix(iceData.Candidate, "candidate:") {
		return fmt.Errorf("invalid candidate format: must start with 'candidate:'")
	}

	return nil
}

// validateErrorMessage 验证错误消息
func (s *SelkiesAdapter) validateErrorMessage(msg *StandardMessage) error {
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
}

// isMessageTypeSupported 检查消息类型是否支持
func (s *SelkiesAdapter) isMessageTypeSupported(msgType MessageType) bool {
	for _, supportedType := range s.protocolMapping {
		if supportedType == msgType {
			return true
		}
	}
	return false
}

// SetLegacyMode 设置遗留模式
func (s *SelkiesAdapter) SetLegacyMode(enabled bool) {
	s.legacyMode = enabled
}

// IsLegacyMode 检查是否为遗留模式
func (s *SelkiesAdapter) IsLegacyMode() bool {
	return s.legacyMode
}

// AddProtocolMapping 添加协议映射
func (s *SelkiesAdapter) AddProtocolMapping(selkiesType string, standardType MessageType) {
	s.protocolMapping[selkiesType] = standardType
}

// RemoveProtocolMapping 移除协议映射
func (s *SelkiesAdapter) RemoveProtocolMapping(selkiesType string) {
	delete(s.protocolMapping, selkiesType)
}

// GetProtocolMappings 获取所有协议映射
func (s *SelkiesAdapter) GetProtocolMappings() map[string]MessageType {
	mappings := make(map[string]MessageType)
	for k, v := range s.protocolMapping {
		mappings[k] = v
	}
	return mappings
}
