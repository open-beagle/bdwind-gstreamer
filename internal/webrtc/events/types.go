package events

import (
	"context"
	"time"
)

// EventType 事件类型
type EventType string

const (
	// WebRTC 协商事件
	EventCreateOffer     EventType = "webrtc.create_offer"
	EventProcessAnswer   EventType = "webrtc.process_answer"
	EventAddICECandidate EventType = "webrtc.add_ice_candidate"
	EventOnICECandidate  EventType = "webrtc.on_ice_candidate"

	// 媒体流事件
	EventStartStreaming EventType = "media.start_streaming"
	EventStopStreaming  EventType = "media.stop_streaming"
	EventVideoData      EventType = "media.video_data"

	// 连接状态事件
	EventConnectionState  EventType = "connection.state_changed"
	EventPeerConnected    EventType = "connection.peer_connected"
	EventPeerDisconnected EventType = "connection.peer_disconnected"

	// 信令事件
	EventSignalingMessage EventType = "signaling.message"
	EventSignalingError   EventType = "signaling.error"
)

// Event 事件接口
type Event interface {
	// Type 获取事件类型
	Type() EventType

	// SessionID 获取会话ID
	SessionID() string

	// Data 获取事件数据
	Data() map[string]interface{}

	// Timestamp 获取事件时间戳
	Timestamp() time.Time
}

// EventHandler 事件处理器接口
type EventHandler interface {
	// Handle 处理事件
	Handle(ctx context.Context, event Event) (*EventResult, error)
}

// EventResult 事件处理结果
type EventResult struct {
	Success bool                   `json:"success"`
	Message string                 `json:"message,omitempty"`
	Data    map[string]interface{} `json:"data,omitempty"`
	Error   string                 `json:"error,omitempty"`
}

// BaseEvent 基础事件实现
type BaseEvent struct {
	eventType EventType
	sessionID string
	data      map[string]interface{}
	timestamp time.Time
}

// NewBaseEvent 创建基础事件
func NewBaseEvent(eventType EventType, sessionID string, data map[string]interface{}) *BaseEvent {
	return &BaseEvent{
		eventType: eventType,
		sessionID: sessionID,
		data:      data,
		timestamp: time.Now(),
	}
}

// Type 获取事件类型
func (e *BaseEvent) Type() EventType {
	return e.eventType
}

// SessionID 获取会话ID
func (e *BaseEvent) SessionID() string {
	return e.sessionID
}

// Data 获取事件数据
func (e *BaseEvent) Data() map[string]interface{} {
	return e.data
}

// Timestamp 获取事件时间戳
func (e *BaseEvent) Timestamp() time.Time {
	return e.timestamp
}

// WebRTCEvent WebRTC相关事件
type WebRTCEvent struct {
	*BaseEvent
	PeerID string `json:"peer_id"`
}

// NewWebRTCEvent 创建WebRTC事件
func NewWebRTCEvent(eventType EventType, sessionID, peerID string, data map[string]interface{}) *WebRTCEvent {
	return &WebRTCEvent{
		BaseEvent: NewBaseEvent(eventType, sessionID, data),
		PeerID:    peerID,
	}
}

// MediaEvent 媒体相关事件
type MediaEvent struct {
	*BaseEvent
	StreamID string `json:"stream_id"`
}

// NewMediaEvent 创建媒体事件
func NewMediaEvent(eventType EventType, sessionID, streamID string, data map[string]interface{}) *MediaEvent {
	return &MediaEvent{
		BaseEvent: NewBaseEvent(eventType, sessionID, data),
		StreamID:  streamID,
	}
}

// SignalingEvent 信令相关事件
type SignalingEvent struct {
	*BaseEvent
	MessageType string `json:"message_type"`
	PeerID      string `json:"peer_id"`
}

// NewSignalingEvent 创建信令事件
func NewSignalingEvent(eventType EventType, sessionID, messageType, peerID string, data map[string]interface{}) *SignalingEvent {
	return &SignalingEvent{
		BaseEvent:   NewBaseEvent(eventType, sessionID, data),
		MessageType: messageType,
		PeerID:      peerID,
	}
}

// ConnectionEvent 连接状态事件
type ConnectionEvent struct {
	*BaseEvent
	PeerID        string `json:"peer_id"`
	State         string `json:"state"`
	PreviousState string `json:"previous_state,omitempty"`
}

// NewConnectionEvent 创建连接事件
func NewConnectionEvent(eventType EventType, sessionID, peerID, state, previousState string, data map[string]interface{}) *ConnectionEvent {
	return &ConnectionEvent{
		BaseEvent:     NewBaseEvent(eventType, sessionID, data),
		PeerID:        peerID,
		State:         state,
		PreviousState: previousState,
	}
}

// EventHandlerFunc 事件处理函数类型
type EventHandlerFunc func(ctx context.Context, event Event) (*EventResult, error)

// Handle 实现EventHandler接口
func (f EventHandlerFunc) Handle(ctx context.Context, event Event) (*EventResult, error) {
	return f(ctx, event)
}

// SuccessResult 创建成功结果
func SuccessResult(message string, data map[string]interface{}) *EventResult {
	return &EventResult{
		Success: true,
		Message: message,
		Data:    data,
	}
}

// ErrorResult 创建错误结果
func ErrorResult(message, errorMsg string) *EventResult {
	return &EventResult{
		Success: false,
		Message: message,
		Error:   errorMsg,
	}
}
