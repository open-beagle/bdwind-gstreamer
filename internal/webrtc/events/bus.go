package events

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

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

// DefaultEventBus 默认事件总线实现
type DefaultEventBus struct {
	handlers map[EventType][]EventHandler
	mutex    sync.RWMutex
	logger   *logrus.Entry
	ctx      context.Context
	cancel   context.CancelFunc
	running  bool
}

// NewEventBus 创建新的事件总线
func NewEventBus() *DefaultEventBus {
	ctx, cancel := context.WithCancel(context.Background())

	return &DefaultEventBus{
		handlers: make(map[EventType][]EventHandler),
		logger:   logrus.WithField("component", "event-bus"),
		ctx:      ctx,
		cancel:   cancel,
	}
}

// Start 启动事件总线
func (eb *DefaultEventBus) Start() error {
	eb.mutex.Lock()
	defer eb.mutex.Unlock()

	if eb.running {
		return nil
	}

	eb.running = true
	eb.logger.Info("Event bus started")
	return nil
}

// Stop 停止事件总线
func (eb *DefaultEventBus) Stop() error {
	eb.mutex.Lock()
	defer eb.mutex.Unlock()

	if !eb.running {
		return nil
	}

	eb.cancel()
	eb.running = false
	eb.logger.Info("Event bus stopped")
	return nil
}

// Publish 异步发布事件
func (eb *DefaultEventBus) Publish(event Event) error {
	eb.mutex.RLock()
	defer eb.mutex.RUnlock()

	if !eb.running {
		return fmt.Errorf("event bus not running")
	}

	handlers, exists := eb.handlers[event.Type()]
	if !exists {
		eb.logger.Debugf("No handlers for event type: %s", event.Type())
		return nil
	}

	// 异步处理事件
	go func() {
		for _, handler := range handlers {
			go func(h EventHandler) {
				defer func() {
					if r := recover(); r != nil {
						eb.logger.Errorf("Event handler panic: %v", r)
					}
				}()

				ctx, cancel := context.WithTimeout(eb.ctx, 30*time.Second)
				defer cancel()

				if _, err := h.Handle(ctx, event); err != nil {
					eb.logger.Errorf("Event handler error: %v", err)
				}
			}(handler)
		}
	}()

	eb.logger.Debugf("Published event: %s (session: %s)", event.Type(), event.SessionID())
	return nil
}

// PublishSync 同步发布事件并等待结果
func (eb *DefaultEventBus) PublishSync(event Event) (*EventResult, error) {
	eb.mutex.RLock()
	defer eb.mutex.RUnlock()

	if !eb.running {
		return nil, fmt.Errorf("event bus not running")
	}

	handlers, exists := eb.handlers[event.Type()]
	if !exists {
		eb.logger.Debugf("No handlers for event type: %s", event.Type())
		return &EventResult{
			Success: true,
			Message: "No handlers registered",
		}, nil
	}

	// 同步处理事件，返回第一个成功的结果
	for _, handler := range handlers {
		ctx, cancel := context.WithTimeout(eb.ctx, 30*time.Second)

		result, err := handler.Handle(ctx, event)
		cancel()

		if err != nil {
			eb.logger.Errorf("Event handler error: %v", err)
			continue
		}

		if result != nil && result.Success {
			eb.logger.Debugf("Event handled successfully: %s (session: %s)", event.Type(), event.SessionID())
			return result, nil
		}
	}

	return &EventResult{
		Success: false,
		Message: "No handler processed the event successfully",
	}, fmt.Errorf("no handler processed the event successfully")
}

// Subscribe 订阅事件类型
func (eb *DefaultEventBus) Subscribe(eventType EventType, handler EventHandler) error {
	eb.mutex.Lock()
	defer eb.mutex.Unlock()

	if eb.handlers[eventType] == nil {
		eb.handlers[eventType] = make([]EventHandler, 0)
	}

	eb.handlers[eventType] = append(eb.handlers[eventType], handler)
	eb.logger.Debugf("Subscribed handler for event type: %s", eventType)
	return nil
}

// Unsubscribe 取消订阅
func (eb *DefaultEventBus) Unsubscribe(eventType EventType, handler EventHandler) error {
	eb.mutex.Lock()
	defer eb.mutex.Unlock()

	handlers, exists := eb.handlers[eventType]
	if !exists {
		return nil
	}

	// 查找并移除处理器
	for i, h := range handlers {
		if h == handler {
			eb.handlers[eventType] = append(handlers[:i], handlers[i+1:]...)
			eb.logger.Debugf("Unsubscribed handler for event type: %s", eventType)
			break
		}
	}

	return nil
}

// GetHandlerCount 获取指定事件类型的处理器数量
func (eb *DefaultEventBus) GetHandlerCount(eventType EventType) int {
	eb.mutex.RLock()
	defer eb.mutex.RUnlock()

	handlers, exists := eb.handlers[eventType]
	if !exists {
		return 0
	}

	return len(handlers)
}

// GetStats 获取事件总线统计信息
func (eb *DefaultEventBus) GetStats() map[string]interface{} {
	eb.mutex.RLock()
	defer eb.mutex.RUnlock()

	stats := map[string]interface{}{
		"running":        eb.running,
		"event_types":    len(eb.handlers),
		"total_handlers": 0,
	}

	totalHandlers := 0
	eventTypes := make(map[string]int)

	for eventType, handlers := range eb.handlers {
		handlerCount := len(handlers)
		totalHandlers += handlerCount
		eventTypes[string(eventType)] = handlerCount
	}

	stats["total_handlers"] = totalHandlers
	stats["handlers_by_type"] = eventTypes

	return stats
}
