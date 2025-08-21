package protocol

import (
	"encoding/json"
	"fmt"
	"time"
)

// ProtocolVersion 协议版本
type ProtocolVersion string

const (
	ProtocolVersionGStreamer10 ProtocolVersion = "gstreamer-1.0"
	ProtocolVersionSelkies     ProtocolVersion = "selkies"
)

// MessageType 消息类型
type MessageType string

const (
	// 标准 GStreamer 消息类型
	MessageTypeHello        MessageType = "hello"
	MessageTypeOffer        MessageType = "offer"
	MessageTypeAnswer       MessageType = "answer"
	MessageTypeICECandidate MessageType = "ice-candidate"
	MessageTypeError        MessageType = "error"
	MessageTypePing         MessageType = "ping"
	MessageTypePong         MessageType = "pong"
	MessageTypeStats        MessageType = "stats"

	// 扩展消息类型
	MessageTypeRequestOffer MessageType = "request-offer"
	MessageTypeAnswerAck    MessageType = "answer-ack"
	MessageTypeICEAck       MessageType = "ice-ack"
	MessageTypeWelcome      MessageType = "welcome"
)

// StandardMessage 标准化的信令消息结构
type StandardMessage struct {
	Version   ProtocolVersion  `json:"version"`
	Type      MessageType      `json:"type"`
	ID        string           `json:"id"`
	Timestamp int64            `json:"timestamp"`
	PeerID    string           `json:"peer_id,omitempty"`
	Data      any              `json:"data,omitempty"`
	Metadata  *MessageMetadata `json:"metadata,omitempty"`
	Error     *MessageError    `json:"error,omitempty"`
}

// MessageMetadata 消息元数据
type MessageMetadata struct {
	Protocol     ProtocolVersion `json:"protocol"`
	ClientInfo   *ClientInfo     `json:"client_info,omitempty"`
	ServerInfo   *ServerInfo     `json:"server_info,omitempty"`
	Capabilities []string        `json:"capabilities,omitempty"`
}

// ClientInfo 客户端信息
type ClientInfo struct {
	UserAgent    string   `json:"user_agent,omitempty"`
	Platform     string   `json:"platform,omitempty"`
	Version      string   `json:"version,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
}

// ServerInfo 服务器信息
type ServerInfo struct {
	Version      string   `json:"version,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
	Timestamp    int64    `json:"timestamp,omitempty"`
}

// MessageError 消息错误信息
type MessageError struct {
	Code        string   `json:"code"`
	Message     string   `json:"message"`
	Details     string   `json:"details,omitempty"`
	Type        string   `json:"type,omitempty"`
	Recoverable bool     `json:"recoverable,omitempty"`
	Suggestions []string `json:"suggestions,omitempty"`
}

// SDPData SDP 数据结构
type SDPData struct {
	Type        string         `json:"type"`
	SDP         string         `json:"sdp"`
	Constraints map[string]any `json:"constraints,omitempty"`
}

// ICECandidateData ICE 候选数据结构
type ICECandidateData struct {
	Candidate        string  `json:"candidate"`
	SDPMid           *string `json:"sdpMid,omitempty"`
	SDPMLineIndex    *uint16 `json:"sdpMLineIndex,omitempty"`
	UsernameFragment *string `json:"usernameFragment,omitempty"`
}

// HelloData HELLO 消息数据结构
type HelloData struct {
	PeerID       string         `json:"peer_id"`
	Capabilities []string       `json:"capabilities,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
}

// StatsData 统计数据结构
type StatsData struct {
	SessionID        string         `json:"session_id"`
	ConnectionState  string         `json:"connection_state"`
	MessagesSent     int64          `json:"messages_sent"`
	MessagesReceived int64          `json:"messages_received"`
	BytesSent        int64          `json:"bytes_sent"`
	BytesReceived    int64          `json:"bytes_received"`
	ConnectionTime   float64        `json:"connection_time"`
	LastActivity     int64          `json:"last_activity"`
	Quality          string         `json:"quality,omitempty"`
	Details          map[string]any `json:"details,omitempty"`
}

// ProtocolAdapter 协议适配器接口
type ProtocolAdapter interface {
	// GetVersion 获取协议版本
	GetVersion() ProtocolVersion

	// ParseMessage 解析消息
	ParseMessage(data []byte) (*StandardMessage, error)

	// FormatMessage 格式化消息
	FormatMessage(msg *StandardMessage) ([]byte, error)

	// ValidateMessage 验证消息
	ValidateMessage(msg *StandardMessage) error

	// GetSupportedMessageTypes 获取支持的消息类型
	GetSupportedMessageTypes() []MessageType

	// IsCompatible 检查是否兼容指定的消息格式
	IsCompatible(data []byte) bool
}

// AdapterConfig 适配器配置
type AdapterConfig struct {
	Version             ProtocolVersion `json:"version"`
	StrictValidation    bool            `json:"strict_validation"`
	MaxMessageSize      int             `json:"max_message_size"`
	EnableCompression   bool            `json:"enable_compression"`
	SupportedExtensions []string        `json:"supported_extensions"`
}

// DefaultAdapterConfig 默认适配器配置
func DefaultAdapterConfig(version ProtocolVersion) *AdapterConfig {
	return &AdapterConfig{
		Version:             version,
		StrictValidation:    true,
		MaxMessageSize:      64 * 1024, // 64KB
		EnableCompression:   false,
		SupportedExtensions: []string{},
	}
}

// NewStandardMessage 创建标准消息
func NewStandardMessage(msgType MessageType, peerID string, data any) *StandardMessage {
	return &StandardMessage{
		Version:   ProtocolVersionGStreamer10,
		Type:      msgType,
		ID:        generateMessageID(),
		Timestamp: time.Now().Unix(),
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
		Timestamp: time.Now().Unix(),
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
	return fmt.Sprintf("msg_%d_%d", time.Now().UnixNano(), time.Now().Nanosecond()%1000)
}
