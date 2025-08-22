package webrtc

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPerformanceMonitor_Creation(t *testing.T) {
	config := DefaultPerformanceMonitorConfig()
	pm := NewPerformanceMonitor(config)

	assert.NotNil(t, pm)
	assert.NotNil(t, pm.messageStats)
	assert.NotNil(t, pm.connectionStats)
	assert.NotNil(t, pm.systemStats)
	assert.NotNil(t, pm.routingStats)
	assert.Equal(t, config, pm.config)
}

func TestPerformanceMonitor_MessageProcessingRecording(t *testing.T) {
	pm := NewPerformanceMonitor(DefaultPerformanceMonitorConfig())

	// 记录一些消息处理
	pm.RecordMessageProcessing("client1", "ping", 10*time.Millisecond, true)
	pm.RecordMessageProcessing("client1", "request-offer", 50*time.Millisecond, true)
	pm.RecordMessageProcessing("client2", "ping", 5*time.Millisecond, false)

	stats := pm.GetMessageStats()

	assert.Equal(t, int64(3), stats.TotalMessages)
	assert.Equal(t, int64(2), stats.SuccessfulMessages)
	assert.Equal(t, int64(1), stats.FailedMessages)

	// 检查消息类型统计
	assert.Contains(t, stats.MessageTypeStats, "ping")
	assert.Contains(t, stats.MessageTypeStats, "request-offer")

	pingStats := stats.MessageTypeStats["ping"]
	assert.Equal(t, int64(2), pingStats.Count)
	assert.Equal(t, int64(1), pingStats.SuccessCount)
	assert.Equal(t, int64(1), pingStats.ErrorCount)

	offerStats := stats.MessageTypeStats["request-offer"]
	assert.Equal(t, int64(1), offerStats.Count)
	assert.Equal(t, int64(1), offerStats.SuccessCount)
	assert.Equal(t, int64(0), offerStats.ErrorCount)
}

func TestPerformanceMonitor_ConnectionEventRecording(t *testing.T) {
	pm := NewPerformanceMonitor(DefaultPerformanceMonitorConfig())

	// 记录连接事件
	pm.RecordConnectionEvent("connect", "client1", "app1")
	pm.RecordConnectionEvent("connect", "client2", "app1")
	pm.RecordConnectionEvent("connect", "client3", "app2")
	pm.RecordConnectionEvent("disconnect", "client1", "app1")

	stats := pm.GetConnectionStats()

	assert.Equal(t, int64(3), stats.TotalConnections)
	assert.Equal(t, int64(2), stats.ActiveConnections)
	assert.Equal(t, int64(3), stats.PeakConnections)

	// 检查应用统计
	assert.Contains(t, stats.AppConnectionStats, "app1")
	assert.Contains(t, stats.AppConnectionStats, "app2")

	app1Stats := stats.AppConnectionStats["app1"]
	assert.Equal(t, int64(1), app1Stats.ActiveConnections)
	assert.Equal(t, int64(2), app1Stats.TotalConnections)

	app2Stats := stats.AppConnectionStats["app2"]
	assert.Equal(t, int64(1), app2Stats.ActiveConnections)
	assert.Equal(t, int64(1), app2Stats.TotalConnections)
}

func TestPerformanceMonitor_RoutingStatsRecording(t *testing.T) {
	pm := NewPerformanceMonitor(DefaultPerformanceMonitorConfig())

	// 记录路由统计
	pm.RecordRoutingStats("gstreamer", 5*time.Millisecond, true)
	pm.RecordRoutingStats("selkies", 10*time.Millisecond, true)
	pm.RecordRoutingStats("gstreamer", 15*time.Millisecond, false)

	stats := pm.routingStats

	assert.Equal(t, int64(3), stats.TotalRoutedMessages)
	assert.Equal(t, int64(1), stats.RoutingErrors)

	// 检查协议统计
	assert.Contains(t, stats.ProtocolStats, "gstreamer")
	assert.Contains(t, stats.ProtocolStats, "selkies")

	gstreamerStats := stats.ProtocolStats["gstreamer"]
	assert.Equal(t, int64(2), gstreamerStats.MessageCount)
	assert.Equal(t, int64(1), gstreamerStats.ErrorCount)

	selkiesStats := stats.ProtocolStats["selkies"]
	assert.Equal(t, int64(1), selkiesStats.MessageCount)
	assert.Equal(t, int64(0), selkiesStats.ErrorCount)
}

func TestPerformanceMonitor_SlowMessageDetection(t *testing.T) {
	config := DefaultPerformanceMonitorConfig()
	config.SlowMessageThreshold = 20 * time.Millisecond
	pm := NewPerformanceMonitor(config)

	// 记录正常和慢消息
	pm.RecordMessageProcessing("client1", "ping", 10*time.Millisecond, true)
	pm.RecordMessageProcessing("client1", "request-offer", 50*time.Millisecond, true) // 慢消息
	pm.RecordMessageProcessing("client2", "ping", 100*time.Millisecond, true)         // 慢消息

	stats := pm.GetMessageStats()

	assert.Equal(t, int64(2), stats.SlowMessages)
	assert.Len(t, stats.SlowMessageDetails, 2)

	// 检查慢消息详情
	slowMsg1 := stats.SlowMessageDetails[0]
	assert.Equal(t, "client1", slowMsg1.ClientID)
	assert.Equal(t, "request-offer", slowMsg1.MessageType)
	assert.Equal(t, 50*time.Millisecond, slowMsg1.ProcessingTime)

	slowMsg2 := stats.SlowMessageDetails[1]
	assert.Equal(t, "client2", slowMsg2.ClientID)
	assert.Equal(t, "ping", slowMsg2.MessageType)
	assert.Equal(t, 100*time.Millisecond, slowMsg2.ProcessingTime)
}

func TestPerformanceMonitor_LatencyBuckets(t *testing.T) {
	pm := NewPerformanceMonitor(DefaultPerformanceMonitorConfig())

	// 记录不同延迟的消息
	pm.RecordMessageProcessing("client1", "ping", 500*time.Microsecond, true) // 0-1ms
	pm.RecordMessageProcessing("client1", "ping", 3*time.Millisecond, true)   // 1-5ms
	pm.RecordMessageProcessing("client1", "ping", 25*time.Millisecond, true)  // 10-50ms
	pm.RecordMessageProcessing("client1", "ping", 200*time.Millisecond, true) // 100-500ms
	pm.RecordMessageProcessing("client1", "ping", 2*time.Second, true)        // 1s-5s

	stats := pm.GetMessageStats()

	assert.Equal(t, int64(1), stats.LatencyBuckets["0-1ms"])
	assert.Equal(t, int64(1), stats.LatencyBuckets["1-5ms"])
	assert.Equal(t, int64(1), stats.LatencyBuckets["10-50ms"])
	assert.Equal(t, int64(1), stats.LatencyBuckets["100-500ms"])
	assert.Equal(t, int64(1), stats.LatencyBuckets["1s-5s"])
}

func TestPerformanceMonitor_SystemStatsCollection(t *testing.T) {
	pm := NewPerformanceMonitor(DefaultPerformanceMonitorConfig())

	// 收集系统统计
	pm.collectSystemStats()

	stats := pm.GetSystemStats()

	assert.NotNil(t, stats.MemoryUsage)
	assert.NotNil(t, stats.GCStats)
	assert.Greater(t, stats.MemoryUsage.AllocatedBytes, uint64(0))
	assert.Greater(t, stats.GoroutineCount, 0)
	assert.False(t, stats.LastUpdated.IsZero())
}

func TestPerformanceMonitor_AlertGeneration(t *testing.T) {
	config := DefaultPerformanceMonitorConfig()
	config.HighMemoryThreshold = 1024   // 很小的阈值，容易触发
	config.HighErrorRateThreshold = 0.3 // 30%错误率阈值

	pm := NewPerformanceMonitor(config)

	alertTriggered := false
	var receivedAlert *PerformanceAlert

	// 添加警报回调
	pm.AddAlertCallback(func(alert *PerformanceAlert) {
		alertTriggered = true
		receivedAlert = alert
	})

	// 记录高错误率的消息
	pm.RecordMessageProcessing("client1", "ping", 10*time.Millisecond, false)
	pm.RecordMessageProcessing("client1", "ping", 10*time.Millisecond, false)
	pm.RecordMessageProcessing("client1", "ping", 10*time.Millisecond, true)

	// 检查警报
	pm.checkAlerts()

	// 给回调一些时间执行（因为是异步的）
	time.Sleep(10 * time.Millisecond)

	assert.True(t, alertTriggered)
	assert.NotNil(t, receivedAlert)
	assert.Equal(t, "error_rate", receivedAlert.Type)
	assert.Equal(t, "critical", receivedAlert.Severity)
}

func TestPerformanceMonitor_StartStop(t *testing.T) {
	pm := NewPerformanceMonitor(DefaultPerformanceMonitorConfig())

	// 启动监控
	pm.Start()

	// 等待一小段时间让监控运行
	time.Sleep(100 * time.Millisecond)

	// 停止监控
	pm.Stop()

	// 验证上下文已取消
	select {
	case <-pm.ctx.Done():
		// 上下文已正确取消
	default:
		t.Error("Context should be cancelled after Stop()")
	}
}

func TestPerformanceMonitor_ResetStats(t *testing.T) {
	pm := NewPerformanceMonitor(DefaultPerformanceMonitorConfig())

	// 记录一些数据
	pm.RecordMessageProcessing("client1", "ping", 10*time.Millisecond, true)
	pm.RecordConnectionEvent("connect", "client1", "app1")
	pm.RecordRoutingStats("gstreamer", 5*time.Millisecond, true)

	// 验证数据已记录
	assert.Equal(t, int64(1), pm.messageStats.TotalMessages)
	assert.Equal(t, int64(1), pm.connectionStats.TotalConnections)
	assert.Equal(t, int64(1), pm.routingStats.TotalRoutedMessages)

	// 重置统计
	pm.ResetStats()

	// 验证数据已重置
	assert.Equal(t, int64(0), pm.messageStats.TotalMessages)
	assert.Equal(t, int64(0), pm.routingStats.TotalRoutedMessages)
	assert.Empty(t, pm.messageStats.MessageTypeStats)
	assert.Empty(t, pm.routingStats.ProtocolStats)
}

func TestConcurrentMessageRouter_Creation(t *testing.T) {
	// 创建基础路由器（模拟）
	baseRouter := &MessageRouter{} // 简化的模拟
	config := DefaultConcurrentRouterConfig()
	pm := NewPerformanceMonitor(DefaultPerformanceMonitorConfig())

	router := NewConcurrentMessageRouter(baseRouter, config, pm)

	assert.NotNil(t, router)
	assert.Equal(t, baseRouter, router.baseRouter)
	assert.Equal(t, config, router.config)
	assert.Equal(t, pm, router.performanceMonitor)
	assert.NotNil(t, router.workerPool)
	assert.Len(t, router.workerPool.workers, config.WorkerCount)
}

func TestConcurrentMessageRouter_StartStop(t *testing.T) {
	baseRouter := &MessageRouter{}
	config := DefaultConcurrentRouterConfig()
	config.WorkerCount = 2 // 减少工作器数量以便测试
	pm := NewPerformanceMonitor(DefaultPerformanceMonitorConfig())

	router := NewConcurrentMessageRouter(baseRouter, config, pm)

	// 启动路由器
	err := router.Start()
	require.NoError(t, err)

	// 验证运行状态
	assert.Equal(t, int32(1), router.running)

	// 停止路由器
	err = router.Stop()
	require.NoError(t, err)

	// 验证停止状态
	assert.Equal(t, int32(0), router.running)
}

func TestLoadBalancers(t *testing.T) {
	// 创建模拟工作器
	workers := make([]*MessageWorker, 3)
	for i := 0; i < 3; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		workers[i] = &MessageWorker{
			ID:     i,
			ctx:    ctx,
			cancel: cancel,
			isIdle: 1, // 初始为空闲
		}
	}

	task := &MessageTask{ClientID: "test-client"}

	t.Run("RoundRobinBalancer", func(t *testing.T) {
		balancer := &RoundRobinBalancer{}

		// 测试轮询选择
		selected1 := balancer.SelectWorker(workers, task)
		selected2 := balancer.SelectWorker(workers, task)
		selected3 := balancer.SelectWorker(workers, task)
		selected4 := balancer.SelectWorker(workers, task) // 应该回到第一个

		assert.NotNil(t, selected1)
		assert.NotNil(t, selected2)
		assert.NotNil(t, selected3)
		assert.NotNil(t, selected4)

		// 验证轮询行为
		assert.NotEqual(t, selected1.ID, selected2.ID)
		assert.NotEqual(t, selected2.ID, selected3.ID)
		assert.Equal(t, selected1.ID, selected4.ID) // 应该回到第一个
	})

	t.Run("LeastLoadedBalancer", func(t *testing.T) {
		balancer := &LeastLoadedBalancer{}

		// 所有工作器都空闲时，应该选择第一个空闲的
		selected := balancer.SelectWorker(workers, task)
		assert.NotNil(t, selected)

		// 模拟一个工作器忙碌
		workers[0].isIdle = 0
		workers[0].processedTasks = 10
		workers[1].processedTasks = 5
		workers[2].processedTasks = 1

		// 应该选择处理任务最少的空闲工作器
		selected = balancer.SelectWorker(workers, task)
		assert.NotNil(t, selected)
		// 由于workers[0]忙碌，应该在workers[1]和workers[2]中选择处理任务更少的
	})

	t.Run("HashBalancer", func(t *testing.T) {
		balancer := &HashBalancer{}

		// 相同客户端ID应该选择相同的工作器
		selected1 := balancer.SelectWorker(workers, task)
		selected2 := balancer.SelectWorker(workers, task)

		assert.NotNil(t, selected1)
		assert.NotNil(t, selected2)
		assert.Equal(t, selected1.ID, selected2.ID)

		// 不同客户端ID可能选择不同的工作器
		differentTask := &MessageTask{ClientID: "different-client"}
		selected3 := balancer.SelectWorker(workers, differentTask)
		assert.NotNil(t, selected3)
	})

	// 清理
	for _, worker := range workers {
		worker.cancel()
	}
}

func TestPerformanceMonitor_Integration(t *testing.T) {
	// 创建完整的性能监控系统
	config := DefaultPerformanceMonitorConfig()
	config.MonitorInterval = 50 * time.Millisecond
	config.ReportInterval = 100 * time.Millisecond

	pm := NewPerformanceMonitor(config)
	pm.Start()
	defer pm.Stop()

	// 模拟一些活动
	for i := 0; i < 10; i++ {
		pm.RecordMessageProcessing("client1", "ping", time.Duration(i)*time.Millisecond, i%3 != 0)
		pm.RecordConnectionEvent("connect", fmt.Sprintf("client%d", i), "app1")
		pm.RecordRoutingStats("gstreamer", time.Duration(i)*time.Millisecond, i%4 != 0)

		time.Sleep(10 * time.Millisecond)
	}

	// 等待监控和报告运行
	time.Sleep(200 * time.Millisecond)

	// 验证统计数据
	stats := pm.GetStats()
	assert.NotNil(t, stats)

	messageStats := pm.GetMessageStats()
	assert.Equal(t, int64(10), messageStats.TotalMessages)
	assert.Greater(t, messageStats.SuccessfulMessages, int64(0))
	assert.Greater(t, messageStats.FailedMessages, int64(0))

	connectionStats := pm.GetConnectionStats()
	assert.Equal(t, int64(10), connectionStats.TotalConnections)
	assert.Equal(t, int64(10), connectionStats.ActiveConnections)

	systemStats := pm.GetSystemStats()
	assert.NotNil(t, systemStats.MemoryUsage)
	assert.Greater(t, systemStats.GoroutineCount, 0)
}
