package events

import (
	"context"
	"time"
)

// EventType 事件类型
type EventType string

// EventHandler 事件处理器接口
type EventHandler interface {
	// Handle 处理事件
	Handle(ctx context.Context, event Event) (*EventResult, error)
}

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

// EventResult 事件处理结果
type EventResult struct {
	Success bool                   `json:"success"`
	Message string                 `json:"message,omitempty"`
	Data    map[string]interface{} `json:"data,omitempty"`
	Error   string                 `json:"error,omitempty"`
}

// EventBus 事件总线接口
type EventBus interface {
	// Publish 异步发布事件
	Publish(event Event) error

	// PublishSync 同步发布事件并等待结果
	PublishSync(event Event) (*EventResult, error)

	// Subscribe 订阅事件类型
	Subscribe(eventType EventType, handler EventHandler) error

	// Unsubscribe 取消订阅
	Unsubscribe(eventType EventType, handler EventHandler) error

	// Start 启动事件总线
	Start() error

	// Stop 停止事件总线
	Stop() error
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
