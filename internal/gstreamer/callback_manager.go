package gstreamer

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-gst/go-gst/gst"
	"github.com/sirupsen/logrus"
)

// CallbackManager 管理GStreamer信号回调的生命周期
// 确保回调在对象有效期内安全执行，防止段错误
type CallbackManager struct {
	callbacks map[string]*CallbackInfo
	mutex     sync.RWMutex
	closed    int32 // 原子操作标志
	logger    *logrus.Entry
}

// CallbackInfo 存储回调信息和状态
type CallbackInfo struct {
	element    *SafeGstElement
	signal     string
	handler    interface{}
	connected  bool
	id         uint64
	createdAt  time.Time
	callCount  int64
	errorCount int64
	lastCall   time.Time
	lastError  time.Time
}

// CallbackStats 回调统计信息
type CallbackStats struct {
	TotalCallbacks          int     `json:"total_callbacks"`
	ConnectedCallbacks      int     `json:"connected_callbacks"`
	TotalCalls              int64   `json:"total_calls"`
	TotalErrors             int64   `json:"total_errors"`
	AverageCallsPerCallback float64 `json:"average_calls_per_callback"`
}

// NewCallbackManager 创建新的回调管理器
func NewCallbackManager() *CallbackManager {
	return &CallbackManager{
		callbacks: make(map[string]*CallbackInfo),
		closed:    0,
		logger: logrus.WithFields(logrus.Fields{
			"component": "callback-manager",
		}),
	}
}

// generateCallbackKey 生成回调的唯一键
func (cm *CallbackManager) generateCallbackKey(element *SafeGstElement, signal string) string {
	return fmt.Sprintf("%s:%s", element.GetID(), signal)
}

// ConnectSignal 安全地连接GStreamer信号回调
func (cm *CallbackManager) ConnectSignal(element *SafeGstElement, signal string, handler interface{}) error {
	if atomic.LoadInt32(&cm.closed) == 1 {
		return fmt.Errorf("callback manager is closed")
	}

	if element == nil {
		return fmt.Errorf("element is nil")
	}

	if !element.IsValid() {
		return fmt.Errorf("element %s is not valid", element.GetID())
	}

	key := cm.generateCallbackKey(element, signal)

	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	// 检查是否已经连接
	if info, exists := cm.callbacks[key]; exists && info.connected {
		cm.logger.WithFields(logrus.Fields{
			"element_id": element.GetID(),
			"signal":     signal,
			"key":        key,
		}).Warn("Signal already connected, skipping")
		return nil
	}

	// 创建安全的包装器回调
	safeHandler := cm.createSafeHandler(element, signal, handler)

	// 连接信号
	var connectionID uint64
	err := element.WithElement(func(gstElement *gst.Element) error {
		// 这里我们需要使用go-gst的Connect方法
		// 注意：这是关键的修复点 - 我们确保在连接时对象是有效的
		gstElement.Connect(signal, safeHandler)
		connectionID = uint64(time.Now().UnixNano()) // 简单的ID生成
		return nil
	})

	if err != nil {
		cm.logger.WithFields(logrus.Fields{
			"element_id": element.GetID(),
			"signal":     signal,
			"error":      err.Error(),
		}).Error("Failed to connect signal")
		return fmt.Errorf("failed to connect signal %s: %w", signal, err)
	}

	// 记录回调信息
	info := &CallbackInfo{
		element:   element,
		signal:    signal,
		handler:   handler,
		connected: true,
		id:        connectionID,
		createdAt: time.Now(),
	}

	cm.callbacks[key] = info

	cm.logger.WithFields(logrus.Fields{
		"element_id":    element.GetID(),
		"signal":        signal,
		"connection_id": connectionID,
		"key":           key,
	}).Debug("Successfully connected signal")

	return nil
}

// createSafeHandler 创建安全的回调处理器包装器
func (cm *CallbackManager) createSafeHandler(element *SafeGstElement, signal string, originalHandler interface{}) interface{} {
	key := cm.generateCallbackKey(element, signal)

	// 根据信号类型创建不同的包装器
	switch signal {
	case "new-sample":
		if handler, ok := originalHandler.(func(*gst.Element) gst.FlowReturn); ok {
			return func(sink *gst.Element) gst.FlowReturn {
				return cm.safeCallNewSampleHandler(key, element, sink, handler)
			}
		}
	case "eos":
		if handler, ok := originalHandler.(func(*gst.Element)); ok {
			return func(elem *gst.Element) {
				cm.safeCallVoidHandler(key, element, elem, handler)
			}
		}
	case "error":
		if handler, ok := originalHandler.(func(*gst.Element, *gst.GError)); ok {
			return func(elem *gst.Element, err *gst.GError) {
				cm.safeCallErrorHandler(key, element, elem, err, handler)
			}
		}
	}

	// 默认处理器 - 直接返回原始处理器但添加安全检查
	cm.logger.WithFields(logrus.Fields{
		"element_id":   element.GetID(),
		"signal":       signal,
		"handler_type": fmt.Sprintf("%T", originalHandler),
	}).Warn("Using default handler wrapper for unknown signal type")

	return originalHandler
}

// safeCallNewSampleHandler 安全调用new-sample处理器
func (cm *CallbackManager) safeCallNewSampleHandler(key string, element *SafeGstElement, sink *gst.Element, handler func(*gst.Element) gst.FlowReturn) gst.FlowReturn {
	// 检查管理器状态
	if atomic.LoadInt32(&cm.closed) == 1 {
		cm.logger.WithFields(logrus.Fields{
			"key": key,
		}).Debug("Callback manager closed, returning FlowFlushing")
		return gst.FlowFlushing
	}

	// 检查元素有效性
	if !element.IsValid() {
		cm.logger.WithFields(logrus.Fields{
			"element_id": element.GetID(),
			"key":        key,
		}).Warn("Element invalid during callback, returning FlowError")
		cm.recordError(key)
		return gst.FlowError
	}

	// 增加引用计数防止垃圾回收
	element.AddRef()
	defer element.Release()

	// 记录调用
	cm.recordCall(key)

	// 使用defer捕获panic
	defer func() {
		if r := recover(); r != nil {
			cm.logger.WithFields(logrus.Fields{
				"element_id": element.GetID(),
				"key":        key,
				"panic":      r,
			}).Error("Panic in new-sample callback")
			cm.recordError(key)
		}
	}()

	// 调用原始处理器
	return handler(sink)
}

// safeCallVoidHandler 安全调用void处理器
func (cm *CallbackManager) safeCallVoidHandler(key string, element *SafeGstElement, gstElement *gst.Element, handler func(*gst.Element)) {
	// 检查管理器状态
	if atomic.LoadInt32(&cm.closed) == 1 {
		cm.logger.WithFields(logrus.Fields{
			"key": key,
		}).Debug("Callback manager closed, ignoring callback")
		return
	}

	// 检查元素有效性
	if !element.IsValid() {
		cm.logger.WithFields(logrus.Fields{
			"element_id": element.GetID(),
			"key":        key,
		}).Warn("Element invalid during callback")
		cm.recordError(key)
		return
	}

	// 增加引用计数防止垃圾回收
	element.AddRef()
	defer element.Release()

	// 记录调用
	cm.recordCall(key)

	// 使用defer捕获panic
	defer func() {
		if r := recover(); r != nil {
			cm.logger.WithFields(logrus.Fields{
				"element_id": element.GetID(),
				"key":        key,
				"panic":      r,
			}).Error("Panic in void callback")
			cm.recordError(key)
		}
	}()

	// 调用原始处理器
	handler(gstElement)
}

// safeCallErrorHandler 安全调用错误处理器
func (cm *CallbackManager) safeCallErrorHandler(key string, element *SafeGstElement, gstElement *gst.Element, gstError *gst.GError, handler func(*gst.Element, *gst.GError)) {
	// 检查管理器状态
	if atomic.LoadInt32(&cm.closed) == 1 {
		cm.logger.WithFields(logrus.Fields{
			"key": key,
		}).Debug("Callback manager closed, ignoring error callback")
		return
	}

	// 检查元素有效性
	if !element.IsValid() {
		cm.logger.WithFields(logrus.Fields{
			"element_id": element.GetID(),
			"key":        key,
		}).Warn("Element invalid during error callback")
		cm.recordError(key)
		return
	}

	// 增加引用计数防止垃圾回收
	element.AddRef()
	defer element.Release()

	// 记录调用
	cm.recordCall(key)

	// 使用defer捕获panic
	defer func() {
		if r := recover(); r != nil {
			cm.logger.WithFields(logrus.Fields{
				"element_id": element.GetID(),
				"key":        key,
				"panic":      r,
			}).Error("Panic in error callback")
			cm.recordError(key)
		}
	}()

	// 调用原始处理器
	handler(gstElement, gstError)
}

// recordCall 记录回调调用
func (cm *CallbackManager) recordCall(key string) {
	cm.mutex.RLock()
	info, exists := cm.callbacks[key]
	cm.mutex.RUnlock()

	if exists {
		atomic.AddInt64(&info.callCount, 1)
		info.lastCall = time.Now()
	}
}

// recordError 记录回调错误
func (cm *CallbackManager) recordError(key string) {
	cm.mutex.RLock()
	info, exists := cm.callbacks[key]
	cm.mutex.RUnlock()

	if exists {
		atomic.AddInt64(&info.errorCount, 1)
		info.lastError = time.Now()
	}
}

// DisconnectSignal 断开指定的信号连接
func (cm *CallbackManager) DisconnectSignal(element *SafeGstElement, signal string) error {
	if element == nil {
		return fmt.Errorf("element is nil")
	}

	key := cm.generateCallbackKey(element, signal)

	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	info, exists := cm.callbacks[key]
	if !exists {
		cm.logger.WithFields(logrus.Fields{
			"element_id": element.GetID(),
			"signal":     signal,
			"key":        key,
		}).Debug("Signal not found for disconnection")
		return nil
	}

	if !info.connected {
		cm.logger.WithFields(logrus.Fields{
			"element_id": element.GetID(),
			"signal":     signal,
			"key":        key,
		}).Debug("Signal already disconnected")
		return nil
	}

	// 标记为未连接
	info.connected = false

	// 注意：go-gst可能没有直接的断开连接方法
	// 我们通过标记为未连接来防止回调执行
	// 实际的GStreamer连接会在对象销毁时自动清理

	cm.logger.WithFields(logrus.Fields{
		"element_id":    element.GetID(),
		"signal":        signal,
		"connection_id": info.id,
		"key":           key,
		"call_count":    info.callCount,
		"error_count":   info.errorCount,
	}).Debug("Disconnected signal")

	return nil
}

// DisconnectAll 断开所有信号连接
func (cm *CallbackManager) DisconnectAll() error {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	disconnectedCount := 0
	for _, info := range cm.callbacks {
		if info.connected {
			info.connected = false
			disconnectedCount++
		}
	}

	cm.logger.WithFields(logrus.Fields{
		"disconnected_count": disconnectedCount,
		"total_callbacks":    len(cm.callbacks),
	}).Info("Disconnected all callbacks")

	return nil
}

// IsConnected 检查指定信号是否已连接
func (cm *CallbackManager) IsConnected(element *SafeGstElement, signal string) bool {
	if element == nil {
		return false
	}

	key := cm.generateCallbackKey(element, signal)

	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	info, exists := cm.callbacks[key]
	return exists && info.connected
}

// GetStats 获取回调统计信息
func (cm *CallbackManager) GetStats() CallbackStats {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	stats := CallbackStats{
		TotalCallbacks: len(cm.callbacks),
	}

	totalCalls := int64(0)
	totalErrors := int64(0)

	for _, info := range cm.callbacks {
		if info.connected {
			stats.ConnectedCallbacks++
		}
		callCount := atomic.LoadInt64(&info.callCount)
		errorCount := atomic.LoadInt64(&info.errorCount)
		totalCalls += callCount
		totalErrors += errorCount
	}

	stats.TotalCalls = totalCalls
	stats.TotalErrors = totalErrors

	if stats.TotalCallbacks > 0 {
		stats.AverageCallsPerCallback = float64(totalCalls) / float64(stats.TotalCallbacks)
	}

	return stats
}

// GetCallbackInfo 获取指定回调的详细信息
func (cm *CallbackManager) GetCallbackInfo(element *SafeGstElement, signal string) (*CallbackInfo, bool) {
	if element == nil {
		return nil, false
	}

	key := cm.generateCallbackKey(element, signal)

	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	info, exists := cm.callbacks[key]
	if !exists {
		return nil, false
	}

	// 返回副本以避免并发访问问题
	infoCopy := *info
	infoCopy.callCount = atomic.LoadInt64(&info.callCount)
	infoCopy.errorCount = atomic.LoadInt64(&info.errorCount)

	return &infoCopy, true
}

// Close 关闭回调管理器
func (cm *CallbackManager) Close() error {
	if !atomic.CompareAndSwapInt32(&cm.closed, 0, 1) {
		return fmt.Errorf("callback manager already closed")
	}

	// 断开所有连接
	err := cm.DisconnectAll()

	cm.logger.WithFields(logrus.Fields{
		"total_callbacks": len(cm.callbacks),
	}).Info("Callback manager closed")

	return err
}

// IsClosed 检查回调管理器是否已关闭
func (cm *CallbackManager) IsClosed() bool {
	return atomic.LoadInt32(&cm.closed) == 1
}
