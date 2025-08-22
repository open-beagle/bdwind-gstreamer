package webrtc

import (
	"context"
	"fmt"
	"log"
	"time"
)

// ExamplePerformanceMonitoring demonstrates how to use the performance monitoring system
func ExamplePerformanceMonitoring() {
	log.Printf("🚀 Starting Performance Monitoring Example")

	// 1. 创建性能监控器
	config := DefaultPerformanceMonitorConfig()
	config.MonitorInterval = 2 * time.Second
	config.ReportInterval = 5 * time.Second
	config.SlowMessageThreshold = 100 * time.Millisecond

	performanceMonitor := NewPerformanceMonitor(config)

	// 2. 添加警报回调
	performanceMonitor.AddAlertCallback(func(alert *PerformanceAlert) {
		log.Printf("🚨 ALERT: %s - %s (Current: %v, Threshold: %v)",
			alert.Type, alert.Message, alert.CurrentValue, alert.Threshold)
	})

	// 3. 启动性能监控
	performanceMonitor.Start()
	defer performanceMonitor.Stop()

	// 4. 模拟一些活动
	log.Printf("📊 Simulating message processing...")

	// 模拟正常消息处理
	for i := 0; i < 20; i++ {
		clientID := fmt.Sprintf("client_%d", i%3+1)
		messageType := []string{"ping", "request-offer", "answer", "ice-candidate"}[i%4]
		processingTime := time.Duration(10+i*5) * time.Millisecond
		success := i%10 != 0 // 90% 成功率

		performanceMonitor.RecordMessageProcessing(clientID, messageType, processingTime, success)

		if i%5 == 0 {
			log.Printf("📨 Processed message %d: client=%s, type=%s, time=%v, success=%t",
				i+1, clientID, messageType, processingTime, success)
		}

		time.Sleep(100 * time.Millisecond)
	}

	// 5. 模拟连接事件
	log.Printf("🔗 Simulating connection events...")

	for i := 0; i < 10; i++ {
		clientID := fmt.Sprintf("client_%d", i+1)
		appName := fmt.Sprintf("app_%d", i%2+1)

		performanceMonitor.RecordConnectionEvent("connect", clientID, appName)

		// 模拟一些客户端断开连接
		if i%3 == 0 {
			performanceMonitor.RecordConnectionEvent("disconnect", clientID, appName)
		}

		time.Sleep(50 * time.Millisecond)
	}

	// 6. 模拟路由统计
	log.Printf("🚦 Simulating routing statistics...")

	protocols := []string{"gstreamer", "selkies", "standard"}
	for i := 0; i < 15; i++ {
		protocol := protocols[i%len(protocols)]
		routingTime := time.Duration(1+i) * time.Millisecond
		success := i%8 != 0 // 87.5% 成功率

		performanceMonitor.RecordRoutingStats(protocol, routingTime, success)

		time.Sleep(80 * time.Millisecond)
	}

	// 7. 等待一些监控周期完成
	log.Printf("⏳ Waiting for monitoring cycles...")
	time.Sleep(8 * time.Second)

	// 8. 获取并显示统计信息
	log.Printf("📊 === Final Performance Statistics ===")

	// 消息统计
	messageStats := performanceMonitor.GetMessageStats()
	if messageStats != nil {
		log.Printf("📨 Message Statistics:")
		log.Printf("  Total Messages: %d", messageStats.TotalMessages)
		log.Printf("  Success Rate: %.2f%%",
			float64(messageStats.SuccessfulMessages)/float64(messageStats.TotalMessages)*100)
		log.Printf("  Average Latency: %v", messageStats.AverageLatency)
		log.Printf("  Slow Messages: %d", messageStats.SlowMessages)

		log.Printf("📋 Message Types:")
		for msgType, stats := range messageStats.MessageTypeStats {
			successRate := float64(stats.SuccessCount) / float64(stats.Count) * 100
			log.Printf("  %s: %d messages, %.1f%% success, avg: %v",
				msgType, stats.Count, successRate, stats.AverageLatency)
		}

		log.Printf("⏱️ Latency Distribution:")
		for bucket, count := range messageStats.LatencyBuckets {
			if count > 0 {
				log.Printf("  %s: %d messages", bucket, count)
			}
		}
	}

	// 连接统计
	connectionStats := performanceMonitor.GetConnectionStats()
	if connectionStats != nil {
		log.Printf("🔗 Connection Statistics:")
		log.Printf("  Total Connections: %d", connectionStats.TotalConnections)
		log.Printf("  Active Connections: %d", connectionStats.ActiveConnections)
		log.Printf("  Peak Connections: %d", connectionStats.PeakConnections)

		log.Printf("📱 Application Statistics:")
		for appName, stats := range connectionStats.AppConnectionStats {
			log.Printf("  %s: %d active, %d total",
				appName, stats.ActiveConnections, stats.TotalConnections)
		}
	}

	// 系统统计
	systemStats := performanceMonitor.GetSystemStats()
	if systemStats != nil {
		log.Printf("💻 System Statistics:")
		log.Printf("  Memory Usage: %.2f MB (%.1f%%)",
			float64(systemStats.MemoryUsage.AllocatedBytes)/1024/1024,
			systemStats.MemoryUsage.UsagePercentage)
		log.Printf("  Goroutines: %d", systemStats.GoroutineCount)
		log.Printf("  GC Collections: %d", systemStats.GCStats.NumGC)
		log.Printf("  GC CPU Fraction: %.2f%%", systemStats.GCStats.GCCPUFraction*100)
	}

	log.Printf("✅ Performance Monitoring Example Completed")
}

// ExampleConcurrentRouting demonstrates the concurrent message routing functionality
func ExampleConcurrentRouting() {
	log.Printf("🚀 Starting Concurrent Routing Example")

	// 1. 创建基础组件
	performanceMonitor := NewPerformanceMonitor(DefaultPerformanceMonitorConfig())
	performanceMonitor.Start()
	defer performanceMonitor.Stop()

	// 创建模拟的基础路由器
	baseRouter := &MessageRouter{
		config: DefaultMessageRouterConfig(),
	}

	// 2. 创建并发路由器
	concurrentConfig := DefaultConcurrentRouterConfig()
	concurrentConfig.WorkerCount = 5
	concurrentConfig.QueueSize = 100
	concurrentConfig.LoadBalanceMode = "least_loaded"

	concurrentRouter := NewConcurrentMessageRouter(baseRouter, concurrentConfig, performanceMonitor)

	// 3. 启动并发路由器
	if err := concurrentRouter.Start(); err != nil {
		log.Printf("❌ Failed to start concurrent router: %v", err)
		return
	}
	defer concurrentRouter.Stop()

	log.Printf("✅ Concurrent router started with %d workers", concurrentConfig.WorkerCount)

	// 4. 等待一些监控周期
	time.Sleep(3 * time.Second)

	// 5. 获取并显示统计信息
	stats := concurrentRouter.GetStats()
	if stats != nil {
		log.Printf("📊 Concurrent Router Statistics:")
		log.Printf("  Total Tasks: %d", stats.TotalTasks)
		log.Printf("  Completed Tasks: %d", stats.CompletedTasks)
		log.Printf("  Failed Tasks: %d", stats.FailedTasks)
		log.Printf("  Active Workers: %d", stats.ActiveWorkers)
		log.Printf("  Worker Utilization: %.1f%%", stats.WorkerUtilization)
		log.Printf("  Queue Depth: %d", stats.QueueDepth)
		log.Printf("  Throughput: %.2f tasks/sec", stats.ThroughputPerSec)
	}

	// 6. 获取工作器详细统计
	workerStats := concurrentRouter.GetWorkerStats()
	log.Printf("👷 Worker Statistics:")
	for _, workerStat := range workerStats {
		log.Printf("  Worker %d: processed=%d, failed=%d, avg_time=%v, idle=%t",
			workerStat["worker_id"], workerStat["processed_tasks"],
			workerStat["failed_tasks"], workerStat["average_time"], workerStat["is_idle"])
	}

	log.Printf("✅ Concurrent Routing Example Completed")
}

// ExampleSignalingServerWithMonitoring demonstrates how to use the enhanced signaling server
func ExampleSignalingServerWithMonitoring() {
	log.Printf("🚀 Starting Enhanced Signaling Server Example")

	// 1. 创建信令服务器配置
	encoderConfig := &SignalingEncoderConfig{
		Codec: "h264",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 2. 创建信令服务器（包含性能监控）
	server := NewSignalingServer(ctx, encoderConfig, nil, nil)

	// 3. 启动服务器（这会自动启动性能监控）
	go server.Start()

	// 4. 等待服务器启动
	time.Sleep(2 * time.Second)

	// 5. 获取性能统计
	log.Printf("📊 Getting performance statistics...")

	performanceStats := server.GetPerformanceStats()
	if performanceStats != nil {
		log.Printf("✅ Performance monitoring is active")
	}

	messageStats := server.GetMessageStats()
	if messageStats != nil {
		log.Printf("📨 Message stats available: %d total messages", messageStats.TotalMessages)
	}

	connectionStats := server.GetConnectionStats()
	if connectionStats != nil {
		log.Printf("🔗 Connection stats available: %d total connections", connectionStats.TotalConnections)
	}

	systemStats := server.GetSystemStats()
	if systemStats != nil {
		log.Printf("💻 System stats available: %d goroutines", systemStats.GoroutineCount)
	}

	concurrentStats := server.GetConcurrentRouterStats()
	if concurrentStats != nil {
		log.Printf("🚦 Concurrent router stats available: %d total tasks", concurrentStats.TotalTasks)
	}

	// 6. 获取详细报告
	detailedReport := server.GetDetailedPerformanceReport()
	log.Printf("📋 Detailed performance report contains %d sections", len(detailedReport))

	// 7. 等待一些监控周期
	time.Sleep(5 * time.Second)

	// 8. 停止服务器
	server.Stop()

	log.Printf("✅ Enhanced Signaling Server Example Completed")
}

// RunAllExamples runs all performance monitoring examples
func RunAllExamples() {
	log.Printf("🎯 Running All Performance Monitoring Examples")
	log.Printf("%s", "="+fmt.Sprintf("%50s", "="))

	log.Printf("\n1️⃣ Performance Monitoring Example")
	ExamplePerformanceMonitoring()

	log.Printf("\n2️⃣ Concurrent Routing Example")
	ExampleConcurrentRouting()

	log.Printf("\n3️⃣ Enhanced Signaling Server Example")
	ExampleSignalingServerWithMonitoring()

	log.Printf("\n🎉 All Examples Completed Successfully!")
}
