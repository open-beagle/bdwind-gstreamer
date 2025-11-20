package protocol

// ProtocolVersion 协议版本
type ProtocolVersion string

const (
	ProtocolVersionGStreamer10 ProtocolVersion = "1.0"
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
	MessageTypeWelcome      MessageType = "welcome"

	// 媒体控制消息类型
	MessageTypeMouseClick MessageType = "mouse-click"
	MessageTypeMouseMove  MessageType = "mouse-move"
	MessageTypeKeyPress   MessageType = "key-press"
	MessageTypeGetStats   MessageType = "get-stats"

	// 协议协商消息类型
	MessageTypeProtocolNegotiation MessageType = "protocol-negotiation"
	MessageTypeProtocolSelected    MessageType = "protocol-selected"

	// 分片消息类型
	MessageTypeMessageFragment MessageType = "message-fragment"
)

// 错误代码常量
const (
	// 连接错误
	ErrorCodeConnectionFailed  = "CONNECTION_FAILED"
	ErrorCodeConnectionTimeout = "CONNECTION_TIMEOUT"
	ErrorCodeConnectionLost    = "CONNECTION_LOST"

	// 验证错误
	ErrorCodeInvalidMessage     = "INVALID_MESSAGE"
	ErrorCodeInvalidMessageType = "INVALID_MESSAGE_TYPE"
	ErrorCodeInvalidMessageData = "INVALID_MESSAGE_DATA"
	ErrorCodeMessageTooLarge    = "MESSAGE_TOO_LARGE"

	// WebRTC 错误
	ErrorCodeSDPProcessingFailed  = "SDP_PROCESSING_FAILED"
	ErrorCodeICECandidateFailed   = "ICE_CANDIDATE_FAILED"
	ErrorCodePeerConnectionFailed = "PEER_CONNECTION_FAILED"

	// 服务器错误
	ErrorCodeServerUnavailable = "SERVER_UNAVAILABLE"
	ErrorCodeInternalError     = "INTERNAL_ERROR"
	ErrorCodeRateLimited       = "RATE_LIMITED"
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
	UserAgent        string   `json:"user_agent,omitempty"`
	Platform         string   `json:"platform,omitempty"`
	Version          string   `json:"version,omitempty"`
	Capabilities     []string `json:"capabilities,omitempty"`
	ScreenResolution string   `json:"screen_resolution,omitempty"`
	DevicePixelRatio float64  `json:"device_pixel_ratio,omitempty"`
	Language         string   `json:"language,omitempty"`
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
	SDP        *SDPContent `json:"sdp"`
	ICEServers []ICEServer `json:"ice_servers,omitempty"`
}

// SDPContent SDP 内容结构
type SDPContent struct {
	Type string `json:"type"`
	SDP  string `json:"sdp"`
}

// ICEServer ICE 服务器配置
type ICEServer struct {
	URLs       []string `json:"urls"`
	Username   string   `json:"username,omitempty"`
	Credential string   `json:"credential,omitempty"`
}

// ICECandidateData ICE 候选数据结构
type ICECandidateData struct {
	Candidate *ICECandidate `json:"candidate"`
}

// ICECandidate ICE 候选信息
type ICECandidate struct {
	Candidate        string  `json:"candidate"`
	SDPMid           *string `json:"sdpMid,omitempty"`
	SDPMLineIndex    *int    `json:"sdpMLineIndex,omitempty"`
	UsernameFragment *string `json:"usernameFragment,omitempty"`
}

// HelloData HELLO 消息数据结构
type HelloData struct {
	ClientInfo         *ClientInfo `json:"client_info,omitempty"`
	Capabilities       []string    `json:"capabilities,omitempty"`
	SupportedProtocols []string    `json:"supported_protocols,omitempty"`
	PreferredProtocol  string      `json:"preferred_protocol,omitempty"`
}

// WelcomeData WELCOME 消息数据结构
type WelcomeData struct {
	ClientID           string         `json:"client_id"`
	AppName            string         `json:"app_name"`
	ServerTime         int64          `json:"server_time"`
	ServerCapabilities []string       `json:"server_capabilities,omitempty"`
	Protocol           string         `json:"protocol"`
	SessionConfig      *SessionConfig `json:"session_config,omitempty"`
}

// SessionConfig 会话配置
type SessionConfig struct {
	HeartbeatInterval int         `json:"heartbeat_interval"`
	MaxMessageSize    int         `json:"max_message_size"`
	ICEServers        []ICEServer `json:"ice_servers,omitempty"`
}

// RequestOfferData REQUEST-OFFER 消息数据结构
type RequestOfferData struct {
	Constraints      *MediaConstraints `json:"constraints,omitempty"`
	CodecPreferences []string          `json:"codec_preferences,omitempty"`
}

// MediaConstraints 媒体约束
type MediaConstraints struct {
	Video       bool `json:"video"`
	Audio       bool `json:"audio"`
	DataChannel bool `json:"data_channel"`
}

// StatsData 统计数据结构
type StatsData struct {
	WebRTC  *WebRTCStats  `json:"webrtc,omitempty"`
	System  *SystemStats  `json:"system,omitempty"`
	Network *NetworkStats `json:"network,omitempty"`
}

// WebRTCStats WebRTC 统计信息
type WebRTCStats struct {
	BytesSent       int64   `json:"bytes_sent"`
	BytesReceived   int64   `json:"bytes_received"`
	PacketsSent     int64   `json:"packets_sent"`
	PacketsReceived int64   `json:"packets_received"`
	PacketsLost     int64   `json:"packets_lost"`
	Jitter          float64 `json:"jitter"`
	RTT             float64 `json:"rtt"`
	Bandwidth       int64   `json:"bandwidth"`
}

// SystemStats 系统统计信息
type SystemStats struct {
	CPUUsage    float64 `json:"cpu_usage"`
	MemoryUsage int64   `json:"memory_usage"`
	GPUUsage    float64 `json:"gpu_usage"`
	FPS         int     `json:"fps"`
}

// NetworkStats 网络统计信息
type NetworkStats struct {
	ConnectionType string  `json:"connection_type"`
	EffectiveType  string  `json:"effective_type"`
	Downlink       float64 `json:"downlink"`
	RTT            int     `json:"rtt"`
}

// GetStatsData GET-STATS 消息数据结构
type GetStatsData struct {
	StatsType []string `json:"stats_type"`
	Interval  int      `json:"interval"`
}

// MouseClickData 鼠标点击数据结构
type MouseClickData struct {
	X         int      `json:"x"`
	Y         int      `json:"y"`
	Button    string   `json:"button"`
	Action    string   `json:"action"`
	Modifiers []string `json:"modifiers,omitempty"`
}

// MouseMoveData 鼠标移动数据结构
type MouseMoveData struct {
	X         int `json:"x"`
	Y         int `json:"y"`
	RelativeX int `json:"relative_x"`
	RelativeY int `json:"relative_y"`
}

// KeyPressData 按键数据结构
type KeyPressData struct {
	Key       string   `json:"key"`
	Code      string   `json:"code"`
	KeyCode   int      `json:"key_code"`
	Action    string   `json:"action"`
	Modifiers []string `json:"modifiers,omitempty"`
	Repeat    bool     `json:"repeat"`
}

// ProtocolNegotiationData 协议协商数据结构
type ProtocolNegotiationData struct {
	SupportedProtocols []string `json:"supported_protocols"`
	PreferredProtocol  string   `json:"preferred_protocol"`
	ClientCapabilities []string `json:"client_capabilities"`
}

// ProtocolSelectedData 协议选择数据结构
type ProtocolSelectedData struct {
	SelectedProtocol   string   `json:"selected_protocol"`
	ProtocolVersion    string   `json:"protocol_version"`
	ServerCapabilities []string `json:"server_capabilities"`
	FallbackProtocols  []string `json:"fallback_protocols,omitempty"`
}

// MessageFragmentData 消息分片数据结构
type MessageFragmentData struct {
	FragmentID     string `json:"fragment_id"`
	FragmentIndex  int    `json:"fragment_index"`
	TotalFragments int    `json:"total_fragments"`
	OriginalType   string `json:"original_type"`
	FragmentData   string `json:"fragment_data"`
}

// PingData PING 消息数据结构
type PingData struct {
	ClientState string `json:"client_state"`
	Sequence    int    `json:"sequence"`
}

// PongData PONG 消息数据结构
type PongData struct {
	ServerState string `json:"server_state"`
	Sequence    int    `json:"sequence"`
	LatencyMS   int    `json:"latency_ms"`
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
