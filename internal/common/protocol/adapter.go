package protocol

import (
	"encoding/json"
	"fmt"
	"time"
)

// NewStandardMessage 创建标准消息
func NewStandardMessage(msgType MessageType, peerID string, data any) *StandardMessage {
	return &StandardMessage{
		Version:   ProtocolVersionGStreamer10,
		Type:      msgType,
		ID:        generateMessageID(),
		Timestamp: time.Now().UnixMilli(),
		PeerID:    peerID,
		Data:      data,
		Metadata: &MessageMetadata{
			Protocol: ProtocolVersionGStreamer10,
		},
	}
}

// NewErrorMessage 创建错误消息
func NewErrorMessage(code, message, details string) *StandardMessage {
	return &StandardMessage{
		Version:   ProtocolVersionGStreamer10,
		Type:      MessageTypeError,
		ID:        generateMessageID(),
		Timestamp: time.Now().UnixMilli(),
		Error: &MessageError{
			Code:        code,
			Message:     message,
			Details:     details,
			Type:        "protocol_error",
			Recoverable: true,
		},
	}
}

// IsValid 检查消息是否有效
func (m *StandardMessage) IsValid() bool {
	return m.Type != "" && m.ID != "" && m.Timestamp > 0
}

// GetDataAs 获取指定类型的数据
func (m *StandardMessage) GetDataAs(target any) error {
	if m.Data == nil {
		return fmt.Errorf("message data is nil")
	}

	dataBytes, err := json.Marshal(m.Data)
	if err != nil {
		return fmt.Errorf("failed to marshal message data: %w", err)
	}

	if err := json.Unmarshal(dataBytes, target); err != nil {
		return fmt.Errorf("failed to unmarshal message data: %w", err)
	}

	return nil
}

// SetData 设置消息数据
func (m *StandardMessage) SetData(data any) {
	m.Data = data
}

// AddCapability 添加能力
func (m *StandardMessage) AddCapability(capability string) {
	if m.Metadata == nil {
		m.Metadata = &MessageMetadata{}
	}

	if m.Metadata.Capabilities == nil {
		m.Metadata.Capabilities = make([]string, 0)
	}

	// 检查是否已存在
	for _, existing := range m.Metadata.Capabilities {
		if existing == capability {
			return
		}
	}

	m.Metadata.Capabilities = append(m.Metadata.Capabilities, capability)
}

// HasCapability 检查是否具有指定能力
func (m *StandardMessage) HasCapability(capability string) bool {
	if m.Metadata == nil || m.Metadata.Capabilities == nil {
		return false
	}

	for _, existing := range m.Metadata.Capabilities {
		if existing == capability {
			return true
		}
	}

	return false
}

// generateMessageID 生成消息ID
func generateMessageID() string {
	return fmt.Sprintf("msg_%d_%03d", time.Now().UnixMilli(), time.Now().Nanosecond()%1000)
}

// NewHelloMessage 创建 HELLO 消息
func NewHelloMessage(peerID string, clientInfo *ClientInfo, capabilities []string, supportedProtocols []string, preferredProtocol string) *StandardMessage {
	data := &HelloData{
		ClientInfo:         clientInfo,
		Capabilities:       capabilities,
		SupportedProtocols: supportedProtocols,
		PreferredProtocol:  preferredProtocol,
	}
	return NewStandardMessage(MessageTypeHello, peerID, data)
}

// NewWelcomeMessage 创建 WELCOME 消息
func NewWelcomeMessage(clientID, appName string, serverCapabilities []string, protocol string, sessionConfig *SessionConfig) *StandardMessage {
	data := &WelcomeData{
		ClientID:           clientID,
		AppName:            appName,
		ServerTime:         time.Now().UnixMilli(),
		ServerCapabilities: serverCapabilities,
		Protocol:           protocol,
		SessionConfig:      sessionConfig,
	}
	return NewStandardMessage(MessageTypeWelcome, clientID, data)
}

// NewOfferMessage 创建 OFFER 消息
func NewOfferMessage(peerID string, sdpContent *SDPContent, iceServers []ICEServer) *StandardMessage {
	data := &SDPData{
		SDP:        sdpContent,
		ICEServers: iceServers,
	}
	return NewStandardMessage(MessageTypeOffer, peerID, data)
}

// NewAnswerMessage 创建 ANSWER 消息
func NewAnswerMessage(peerID string, sdpContent *SDPContent) *StandardMessage {
	data := &SDPData{
		SDP: sdpContent,
	}
	return NewStandardMessage(MessageTypeAnswer, peerID, data)
}

// NewICECandidateMessage 创建 ICE-CANDIDATE 消息
func NewICECandidateMessage(peerID string, candidate *ICECandidate) *StandardMessage {
	data := &ICECandidateData{
		Candidate: candidate,
	}
	return NewStandardMessage(MessageTypeICECandidate, peerID, data)
}

// NewPingMessage 创建 PING 消息
func NewPingMessage(peerID string, clientState string, sequence int) *StandardMessage {
	data := &PingData{
		ClientState: clientState,
		Sequence:    sequence,
	}
	return NewStandardMessage(MessageTypePing, peerID, data)
}

// NewPongMessage 创建 PONG 消息
func NewPongMessage(peerID string, serverState string, sequence int, latencyMS int) *StandardMessage {
	data := &PongData{
		ServerState: serverState,
		Sequence:    sequence,
		LatencyMS:   latencyMS,
	}
	return NewStandardMessage(MessageTypePong, peerID, data)
}

// NewStatsMessage 创建 STATS 消息
func NewStatsMessage(peerID string, webrtc *WebRTCStats, system *SystemStats, network *NetworkStats) *StandardMessage {
	data := &StatsData{
		WebRTC:  webrtc,
		System:  system,
		Network: network,
	}
	return NewStandardMessage(MessageTypeStats, peerID, data)
}

// NewMouseClickMessage 创建鼠标点击消息
func NewMouseClickMessage(peerID string, x, y int, button, action string, modifiers []string) *StandardMessage {
	data := &MouseClickData{
		X:         x,
		Y:         y,
		Button:    button,
		Action:    action,
		Modifiers: modifiers,
	}
	return NewStandardMessage(MessageTypeMouseClick, peerID, data)
}

// NewKeyPressMessage 创建按键消息
func NewKeyPressMessage(peerID string, key, code string, keyCode int, action string, modifiers []string, repeat bool) *StandardMessage {
	data := &KeyPressData{
		Key:       key,
		Code:      code,
		KeyCode:   keyCode,
		Action:    action,
		Modifiers: modifiers,
		Repeat:    repeat,
	}
	return NewStandardMessage(MessageTypeKeyPress, peerID, data)
}
