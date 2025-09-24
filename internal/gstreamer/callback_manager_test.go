package gstreamer

import (
	"testing"
	"time"

	"github.com/go-gst/go-gst/gst"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCallbackManager_Creation(t *testing.T) {
	cm := NewCallbackManager()
	require.NotNil(t, cm)

	assert.False(t, cm.IsClosed())
	assert.NotNil(t, cm.callbacks)
	assert.NotNil(t, cm.logger)

	// 测试初始统计
	stats := cm.GetStats()
	assert.Equal(t, 0, stats.TotalCallbacks)
	assert.Equal(t, 0, stats.ConnectedCallbacks)
	assert.Equal(t, int64(0), stats.TotalCalls)
	assert.Equal(t, int64(0), stats.TotalErrors)
}

func TestCallbackManager_ConnectSignal(t *testing.T) {
	// 初始化GStreamer
	gst.Init(nil)

	cm := NewCallbackManager()
	require.NotNil(t, cm)

	// 创建测试元素
	element, err := gst.NewElement("appsink")
	require.NoError(t, err)
	safeElement := NewSafeGstElement(element, "appsink")
	require.NotNil(t, safeElement)

	// 创建测试处理器
	callCount := 0
	handler := func(sink *gst.Element) gst.FlowReturn {
		callCount++
		return gst.FlowOK
	}

	// 连接信号
	err = cm.ConnectSignal(safeElement, "new-sample", handler)
	assert.NoError(t, err)

	// 验证连接状态
	assert.True(t, cm.IsConnected(safeElement, "new-sample"))

	// 验证统计信息
	stats := cm.GetStats()
	assert.Equal(t, 1, stats.TotalCallbacks)
	assert.Equal(t, 1, stats.ConnectedCallbacks)

	// 获取回调信息
	info, exists := cm.GetCallbackInfo(safeElement, "new-sample")
	assert.True(t, exists)
	assert.NotNil(t, info)
	assert.Equal(t, "new-sample", info.signal)
	assert.True(t, info.connected)
}

func TestCallbackManager_ConnectSignal_InvalidElement(t *testing.T) {
	cm := NewCallbackManager()
	require.NotNil(t, cm)

	// 测试nil元素
	err := cm.ConnectSignal(nil, "new-sample", func(*gst.Element) gst.FlowReturn {
		return gst.FlowOK
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "element is nil")

	// 测试无效元素
	gst.Init(nil)
	element, err := gst.NewElement("appsink")
	require.NoError(t, err)
	safeElement := NewSafeGstElement(element, "appsink")
	require.NotNil(t, safeElement)

	// 使元素无效
	safeElement.Invalidate()

	err = cm.ConnectSignal(safeElement, "new-sample", func(*gst.Element) gst.FlowReturn {
		return gst.FlowOK
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not valid")
}

func TestCallbackManager_DisconnectSignal(t *testing.T) {
	// 初始化GStreamer
	gst.Init(nil)

	cm := NewCallbackManager()
	require.NotNil(t, cm)

	// 创建测试元素
	element, err := gst.NewElement("appsink")
	require.NoError(t, err)
	safeElement := NewSafeGstElement(element, "appsink")
	require.NotNil(t, safeElement)

	// 连接信号
	handler := func(sink *gst.Element) gst.FlowReturn {
		return gst.FlowOK
	}
	err = cm.ConnectSignal(safeElement, "new-sample", handler)
	require.NoError(t, err)

	// 验证连接
	assert.True(t, cm.IsConnected(safeElement, "new-sample"))

	// 断开连接
	err = cm.DisconnectSignal(safeElement, "new-sample")
	assert.NoError(t, err)

	// 验证断开
	assert.False(t, cm.IsConnected(safeElement, "new-sample"))

	// 再次断开应该是安全的
	err = cm.DisconnectSignal(safeElement, "new-sample")
	assert.NoError(t, err)
}

func TestCallbackManager_DisconnectAll(t *testing.T) {
	// 初始化GStreamer
	gst.Init(nil)

	cm := NewCallbackManager()
	require.NotNil(t, cm)

	// 创建多个测试元素和连接
	elements := make([]*SafeGstElement, 3)
	for i := 0; i < 3; i++ {
		element, err := gst.NewElement("appsink")
		require.NoError(t, err)
		safeElement := NewSafeGstElement(element, "appsink")
		require.NotNil(t, safeElement)
		elements[i] = safeElement

		// 连接信号
		handler := func(sink *gst.Element) gst.FlowReturn {
			return gst.FlowOK
		}
		err = cm.ConnectSignal(safeElement, "new-sample", handler)
		require.NoError(t, err)
	}

	// 验证所有连接
	for _, elem := range elements {
		assert.True(t, cm.IsConnected(elem, "new-sample"))
	}

	stats := cm.GetStats()
	assert.Equal(t, 3, stats.ConnectedCallbacks)

	// 断开所有连接
	err := cm.DisconnectAll()
	assert.NoError(t, err)

	// 验证所有连接都已断开
	for _, elem := range elements {
		assert.False(t, cm.IsConnected(elem, "new-sample"))
	}

	stats = cm.GetStats()
	assert.Equal(t, 0, stats.ConnectedCallbacks)
	assert.Equal(t, 3, stats.TotalCallbacks) // 总数不变，只是状态改变
}

func TestCallbackManager_SafeHandlerWrapper(t *testing.T) {
	// 初始化GStreamer
	gst.Init(nil)

	cm := NewCallbackManager()
	require.NotNil(t, cm)

	// 创建测试元素
	element, err := gst.NewElement("appsink")
	require.NoError(t, err)
	safeElement := NewSafeGstElement(element, "appsink")
	require.NotNil(t, safeElement)

	// 测试new-sample处理器包装
	callCount := 0
	handler := func(sink *gst.Element) gst.FlowReturn {
		callCount++
		return gst.FlowOK
	}

	safeHandler := cm.createSafeHandler(safeElement, "new-sample", handler)
	assert.NotNil(t, safeHandler)

	// 验证包装器类型
	wrappedHandler, ok := safeHandler.(func(*gst.Element) gst.FlowReturn)
	assert.True(t, ok)

	// 模拟调用包装器
	key := cm.generateCallbackKey(safeElement, "new-sample")
	cm.callbacks[key] = &CallbackInfo{
		element:   safeElement,
		signal:    "new-sample",
		handler:   handler,
		connected: true,
		createdAt: time.Now(),
	}

	result := wrappedHandler(element)
	assert.Equal(t, gst.FlowOK, result)
	assert.Equal(t, 1, callCount)
}

func TestCallbackManager_SafeHandlerWithInvalidElement(t *testing.T) {
	// 初始化GStreamer
	gst.Init(nil)

	cm := NewCallbackManager()
	require.NotNil(t, cm)

	// 创建测试元素
	element, err := gst.NewElement("appsink")
	require.NoError(t, err)
	safeElement := NewSafeGstElement(element, "appsink")
	require.NotNil(t, safeElement)

	// 创建包装器
	callCount := 0
	handler := func(sink *gst.Element) gst.FlowReturn {
		callCount++
		return gst.FlowOK
	}

	safeHandler := cm.createSafeHandler(safeElement, "new-sample", handler)
	wrappedHandler := safeHandler.(func(*gst.Element) gst.FlowReturn)

	// 设置回调信息
	key := cm.generateCallbackKey(safeElement, "new-sample")
	cm.callbacks[key] = &CallbackInfo{
		element:   safeElement,
		signal:    "new-sample",
		handler:   handler,
		connected: true,
		createdAt: time.Now(),
	}

	// 使元素无效
	safeElement.Invalidate()

	// 调用包装器应该返回错误状态
	result := wrappedHandler(element)
	assert.Equal(t, gst.FlowError, result)
	assert.Equal(t, 0, callCount) // 原始处理器不应该被调用

	// 验证错误计数
	info, exists := cm.GetCallbackInfo(safeElement, "new-sample")
	assert.True(t, exists)
	assert.Equal(t, int64(1), info.errorCount)
}

func TestCallbackManager_SafeHandlerWithClosedManager(t *testing.T) {
	// 初始化GStreamer
	gst.Init(nil)

	cm := NewCallbackManager()
	require.NotNil(t, cm)

	// 创建测试元素
	element, err := gst.NewElement("appsink")
	require.NoError(t, err)
	safeElement := NewSafeGstElement(element, "appsink")
	require.NotNil(t, safeElement)

	// 创建包装器
	handler := func(sink *gst.Element) gst.FlowReturn {
		return gst.FlowOK
	}

	safeHandler := cm.createSafeHandler(safeElement, "new-sample", handler)
	wrappedHandler := safeHandler.(func(*gst.Element) gst.FlowReturn)

	// 关闭管理器
	err = cm.Close()
	require.NoError(t, err)

	// 调用包装器应该返回FlowFlushing
	result := wrappedHandler(element)
	assert.Equal(t, gst.FlowFlushing, result)
}

func TestCallbackManager_Statistics(t *testing.T) {
	// 初始化GStreamer
	gst.Init(nil)

	cm := NewCallbackManager()
	require.NotNil(t, cm)

	// 创建测试元素
	element, err := gst.NewElement("appsink")
	require.NoError(t, err)
	safeElement := NewSafeGstElement(element, "appsink")
	require.NotNil(t, safeElement)

	// 连接信号
	handler := func(sink *gst.Element) gst.FlowReturn {
		return gst.FlowOK
	}
	err = cm.ConnectSignal(safeElement, "new-sample", handler)
	require.NoError(t, err)

	// 模拟一些调用
	key := cm.generateCallbackKey(safeElement, "new-sample")
	for i := 0; i < 5; i++ {
		cm.recordCall(key)
	}
	for i := 0; i < 2; i++ {
		cm.recordError(key)
	}

	// 验证统计信息
	stats := cm.GetStats()
	assert.Equal(t, 1, stats.TotalCallbacks)
	assert.Equal(t, 1, stats.ConnectedCallbacks)
	assert.Equal(t, int64(5), stats.TotalCalls)
	assert.Equal(t, int64(2), stats.TotalErrors)
	assert.Equal(t, 5.0, stats.AverageCallsPerCallback)

	// 验证回调信息
	info, exists := cm.GetCallbackInfo(safeElement, "new-sample")
	assert.True(t, exists)
	assert.Equal(t, int64(5), info.callCount)
	assert.Equal(t, int64(2), info.errorCount)
}

func TestCallbackManager_Close(t *testing.T) {
	cm := NewCallbackManager()
	require.NotNil(t, cm)

	assert.False(t, cm.IsClosed())

	// 关闭管理器
	err := cm.Close()
	assert.NoError(t, err)
	assert.True(t, cm.IsClosed())

	// 再次关闭应该返回错误
	err = cm.Close()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already closed")

	// 关闭后的操作应该失败
	gst.Init(nil)
	element, err := gst.NewElement("appsink")
	require.NoError(t, err)
	safeElement := NewSafeGstElement(element, "appsink")

	err = cm.ConnectSignal(safeElement, "new-sample", func(*gst.Element) gst.FlowReturn {
		return gst.FlowOK
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "callback manager is closed")
}

func TestCallbackManager_ConcurrentAccess(t *testing.T) {
	// 初始化GStreamer
	gst.Init(nil)

	cm := NewCallbackManager()
	require.NotNil(t, cm)

	// 创建多个元素
	elements := make([]*SafeGstElement, 10)
	for i := 0; i < 10; i++ {
		element, err := gst.NewElement("appsink")
		require.NoError(t, err)
		safeElement := NewSafeGstElement(element, "appsink")
		require.NotNil(t, safeElement)
		elements[i] = safeElement
	}

	// 并发连接信号
	done := make(chan bool, 10)
	for i, elem := range elements {
		go func(index int, element *SafeGstElement) {
			defer func() { done <- true }()

			handler := func(sink *gst.Element) gst.FlowReturn {
				return gst.FlowOK
			}

			err := cm.ConnectSignal(element, "new-sample", handler)
			if err != nil {
				t.Errorf("Failed to connect signal for element %d: %v", index, err)
			}

			// 模拟一些调用
			key := cm.generateCallbackKey(element, "new-sample")
			for j := 0; j < 10; j++ {
				cm.recordCall(key)
			}
		}(i, elem)
	}

	// 等待所有goroutine完成
	for i := 0; i < 10; i++ {
		<-done
	}

	// 验证结果
	stats := cm.GetStats()
	assert.Equal(t, 10, stats.TotalCallbacks)
	assert.Equal(t, 10, stats.ConnectedCallbacks)
	assert.Equal(t, int64(100), stats.TotalCalls)

	// 断开所有连接
	err := cm.DisconnectAll()
	assert.NoError(t, err)

	stats = cm.GetStats()
	assert.Equal(t, 0, stats.ConnectedCallbacks)
}
