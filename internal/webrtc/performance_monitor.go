package webrtc

import (
	"context"
	"fmt"
	"log"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// PerformanceMonitor 性能监控器
type PerformanceMonitor struct {
	// 消息处理统计
	messageStats    *MessageProcessingStats
	connectionStats *ConnectionStats
	systemStats     *SystemStats
	routingStats    *MessageRoutingStats

	// 配置
	config *PerformanceMonitorConfig

	// 控制
	ctx    context.Context
	cancel context.CancelFunc
	mutex  sync.RWMutex

	// 报告生成
	reportInterval time.Duration
	lastReportTime time.Time

	// 事件回调
	alertCallbacks []func(alert *PerformanceAlert)
}

// PerformanceMonitorConfig 性能监控配置
type PerformanceMonitorConfig struct {
	// 监控间隔
	MonitorInterval time.Duration `json:"monitor_interval"`
	ReportInterval  time.Duration `json:"report_interval"`

	// 阈值配置
	SlowMessageThreshold    time.Duration `json:"slow_message_threshold"`
	HighMemoryThreshold     uint64        `json:"high_memory_threshold"`
	HighConnectionThreshold int           `json:"high_connection_threshold"`
	HighErrorRateThreshold  float64       `json:"high_error_rate_threshold"`

	// 性能优化配置
	EnableConcurrentRouting bool `json:"enable_concurrent_routing"`
	MaxConcurrentMessages   int  `json:"max_concurrent_messages"`
	MessageBufferSize       int  `json:"message_buffer_size"`

	// 统计保留配置
	StatsRetentionPeriod time.Duration `json:"stats_retention_period"`
	DetailedStatsEnabled bool          `json:"detailed_stats_enabled"`
}

// MessageProcessingStats 消息处理统计
type MessageProcessingStats struct {
	// 基础计数器
	TotalMessages      int64 `json:"total_messages"`
	SuccessfulMessages int64 `json:"successful_messages"`
	FailedMessages     int64 `json:"failed_messages"`

	// 按消息类型统计
	MessageTypeStats map[string]*MessageTypeStats `json:"message_type_stats"`

	// 延迟统计
	AverageLatency time.Duration `json:"average_latency"`
	MinLatency     time.Duration `json:"min_latency"`
	MaxLatency     time.Duration `json:"max_latency"`
	P95Latency     time.Duration `json:"p95_latency"`
	P99Latency     time.Duration `json:"p99_latency"`

	// 延迟分布
	LatencyBuckets map[string]int64 `json:"latency_buckets"`

	// 时间窗口统计
	LastMinuteStats *TimeWindowStats `json:"last_minute_stats"`
	LastHourStats   *TimeWindowStats `json:"last_hour_stats"`

	// 慢消息统计
	SlowMessages       int64                `json:"slow_messages"`
	SlowMessageDetails []*SlowMessageRecord `json:"slow_message_details"`

	mutex sync.RWMutex
}

// MessageTypeStats 按消息类型的统计
type MessageTypeStats struct {
	Count          int64         `json:"count"`
	SuccessCount   int64         `json:"success_count"`
	ErrorCount     int64         `json:"error_count"`
	AverageLatency time.Duration `json:"average_latency"`
	TotalLatency   time.Duration `json:"total_latency"`
	MaxLatency     time.Duration `json:"max_latency"`
	LastProcessed  time.Time     `json:"last_processed"`
}

// TimeWindowStats 时间窗口统计
type TimeWindowStats struct {
	StartTime        time.Time     `json:"start_time"`
	EndTime          time.Time     `json:"end_time"`
	MessageCount     int64         `json:"message_count"`
	ErrorCount       int64         `json:"error_count"`
	AverageLatency   time.Duration `json:"average_latency"`
	ThroughputPerSec float64       `json:"throughput_per_sec"`
}

// SlowMessageRecord 慢消息记录
type SlowMessageRecord struct {
	Timestamp      time.Time     `json:"timestamp"`
	ClientID       string        `json:"client_id"`
	MessageType    string        `json:"message_type"`
	ProcessingTime time.Duration `json:"processing_time"`
	Details        string        `json:"details"`
}

// ConnectionStats 连接统计
type ConnectionStats struct {
	// 连接数统计
	TotalConnections  int64 `json:"total_connections"`
	ActiveConnections int64 `json:"active_connections"`
	PeakConnections   int64 `json:"peak_connections"`

	// 连接生命周期统计
	AverageConnectionDuration time.Duration `json:"average_connection_duration"`
	ConnectionsPerMinute      float64       `json:"connections_per_minute"`
	DisconnectionsPerMinute   float64       `json:"disconnections_per_minute"`

	// 连接质量统计
	HealthyConnections   int64 `json:"healthy_connections"`
	UnhealthyConnections int64 `json:"unhealthy_connections"`
	ReconnectAttempts    int64 `json:"reconnect_attempts"`

	// 按应用统计
	AppConnectionStats map[string]*AppConnectionStats `json:"app_connection_stats"`

	mutex sync.RWMutex
}

// AppConnectionStats 按应用的连接统计
type AppConnectionStats struct {
	AppName           string    `json:"app_name"`
	ActiveConnections int64     `json:"active_connections"`
	TotalConnections  int64     `json:"total_connections"`
	LastActivity      time.Time `json:"last_activity"`
}

// SystemStats 系统统计
type SystemStats struct {
	// 内存使用
	MemoryUsage *MemoryUsage `json:"memory_usage"`

	// CPU使用
	CPUUsage float64 `json:"cpu_usage"`

	// Goroutine统计
	GoroutineCount int `json:"goroutine_count"`

	// GC统计
	GCStats *GCStats `json:"gc_stats"`

	// 系统负载
	SystemLoad *SystemLoad `json:"system_load"`

	// 更新时间
	LastUpdated time.Time `json:"last_updated"`

	mutex sync.RWMutex
}

// MemoryUsage 内存使用统计
type MemoryUsage struct {
	AllocatedBytes      uint64  `json:"allocated_bytes"`
	TotalAllocatedBytes uint64  `json:"total_allocated_bytes"`
	SystemBytes         uint64  `json:"system_bytes"`
	HeapBytes           uint64  `json:"heap_bytes"`
	StackBytes          uint64  `json:"stack_bytes"`
	GCPauseTotal        uint64  `json:"gc_pause_total"`
	UsagePercentage     float64 `json:"usage_percentage"`
}

// GCStats 垃圾回收统计
type GCStats struct {
	NumGC         uint32        `json:"num_gc"`
	PauseTotal    time.Duration `json:"pause_total"`
	LastPause     time.Duration `json:"last_pause"`
	AveragePause  time.Duration `json:"average_pause"`
	GCCPUFraction float64       `json:"gc_cpu_fraction"`
}

// SystemLoad 系统负载
type SystemLoad struct {
	MessageQueueDepth   int     `json:"message_queue_depth"`
	ConnectionPoolUsage float64 `json:"connection_pool_usage"`
	ThreadPoolUsage     float64 `json:"thread_pool_usage"`
}

// MessageRoutingStats 消息路由统计
type MessageRoutingStats struct {
	// 路由性能
	TotalRoutedMessages int64         `json:"total_routed_messages"`
	AverageRoutingTime  time.Duration `json:"average_routing_time"`
	RoutingErrors       int64         `json:"routing_errors"`

	// 协议统计
	ProtocolStats map[string]*ProtocolRoutingStats `json:"protocol_stats"`

	// 并发统计
	ConcurrentMessages int64 `json:"concurrent_messages"`
	MaxConcurrency     int64 `json:"max_concurrency"`
	QueuedMessages     int64 `json:"queued_messages"`

	mutex sync.RWMutex
}

// ProtocolRoutingStats 协议路由统计
type ProtocolRoutingStats struct {
	Protocol     string        `json:"protocol"`
	MessageCount int64         `json:"message_count"`
	AverageTime  time.Duration `json:"average_time"`
	ErrorCount   int64         `json:"error_count"`
	LastUsed     time.Time     `json:"last_used"`
}

// PerformanceAlert 性能警报
type PerformanceAlert struct {
	Type         string                 `json:"type"`
	Severity     string                 `json:"severity"`
	Message      string                 `json:"message"`
	Details      map[string]interface{} `json:"details"`
	Timestamp    time.Time              `json:"timestamp"`
	Threshold    interface{}            `json:"threshold"`
	CurrentValue interface{}            `json:"current_value"`
}

// DefaultPerformanceMonitorConfig 默认性能监控配置
func DefaultPerformanceMonitorConfig() *PerformanceMonitorConfig {
	return &PerformanceMonitorConfig{
		MonitorInterval:         5 * time.Second,
		ReportInterval:          60 * time.Second,
		SlowMessageThreshold:    1 * time.Second,
		HighMemoryThreshold:     1024 * 1024 * 1024, // 1GB
		HighConnectionThreshold: 1000,
		HighErrorRateThreshold:  0.05, // 5%
		EnableConcurrentRouting: true,
		MaxConcurrentMessages:   100,
		MessageBufferSize:       1000,
		StatsRetentionPeriod:    24 * time.Hour,
		DetailedStatsEnabled:    true,
	}
}

// NewPerformanceMonitor 创建性能监控器
func NewPerformanceMonitor(config *PerformanceMonitorConfig) *PerformanceMonitor {
	if config == nil {
		config = DefaultPerformanceMonitorConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	pm := &PerformanceMonitor{
		messageStats: &MessageProcessingStats{
			MessageTypeStats:   make(map[string]*MessageTypeStats),
			LatencyBuckets:     make(map[string]int64),
			SlowMessageDetails: make([]*SlowMessageRecord, 0),
			LastMinuteStats:    &TimeWindowStats{StartTime: time.Now()},
			LastHourStats:      &TimeWindowStats{StartTime: time.Now()},
		},
		connectionStats: &ConnectionStats{
			AppConnectionStats: make(map[string]*AppConnectionStats),
		},
		systemStats: &SystemStats{
			MemoryUsage: &MemoryUsage{},
			GCStats:     &GCStats{},
			SystemLoad:  &SystemLoad{},
		},
		routingStats: &MessageRoutingStats{
			ProtocolStats: make(map[string]*ProtocolRoutingStats),
		},
		config:         config,
		ctx:            ctx,
		cancel:         cancel,
		reportInterval: config.ReportInterval,
		lastReportTime: time.Now(),
		alertCallbacks: make([]func(alert *PerformanceAlert), 0),
	}

	// 初始化延迟桶
	pm.initializeLatencyBuckets()

	return pm
}

// initializeLatencyBuckets 初始化延迟桶
func (pm *PerformanceMonitor) initializeLatencyBuckets() {
	buckets := []string{
		"0-1ms", "1-5ms", "5-10ms", "10-50ms", "50-100ms",
		"100-500ms", "500ms-1s", "1s-5s", "5s+",
	}

	pm.messageStats.mutex.Lock()
	defer pm.messageStats.mutex.Unlock()

	for _, bucket := range buckets {
		pm.messageStats.LatencyBuckets[bucket] = 0
	}
}

// Start 启动性能监控
func (pm *PerformanceMonitor) Start() {
	log.Printf("🚀 Starting performance monitor with interval %v", pm.config.MonitorInterval)

	// 启动监控协程
	go pm.monitorLoop()

	// 启动报告协程
	go pm.reportLoop()

	log.Printf("✅ Performance monitor started")
}

// Stop 停止性能监控
func (pm *PerformanceMonitor) Stop() {
	log.Printf("🛑 Stopping performance monitor...")
	pm.cancel()
	log.Printf("✅ Performance monitor stopped")
}

// monitorLoop 监控循环
func (pm *PerformanceMonitor) monitorLoop() {
	ticker := time.NewTicker(pm.config.MonitorInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			pm.collectSystemStats()
			pm.checkAlerts()
		case <-pm.ctx.Done():
			return
		}
	}
}

// reportLoop 报告循环
func (pm *PerformanceMonitor) reportLoop() {
	ticker := time.NewTicker(pm.reportInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			pm.generatePerformanceReport()
		case <-pm.ctx.Done():
			return
		}
	}
}

// RecordMessageProcessing 记录消息处理
func (pm *PerformanceMonitor) RecordMessageProcessing(clientID, messageType string, processingTime time.Duration, success bool) {
	pm.messageStats.mutex.Lock()
	defer pm.messageStats.mutex.Unlock()

	// 更新总计数器
	atomic.AddInt64(&pm.messageStats.TotalMessages, 1)
	if success {
		atomic.AddInt64(&pm.messageStats.SuccessfulMessages, 1)
	} else {
		atomic.AddInt64(&pm.messageStats.FailedMessages, 1)
	}

	// 更新消息类型统计
	if pm.messageStats.MessageTypeStats[messageType] == nil {
		pm.messageStats.MessageTypeStats[messageType] = &MessageTypeStats{}
	}

	typeStats := pm.messageStats.MessageTypeStats[messageType]
	typeStats.Count++
	if success {
		typeStats.SuccessCount++
	} else {
		typeStats.ErrorCount++
	}

	// 更新延迟统计
	typeStats.TotalLatency += processingTime
	typeStats.AverageLatency = time.Duration(int64(typeStats.TotalLatency) / typeStats.Count)
	if processingTime > typeStats.MaxLatency {
		typeStats.MaxLatency = processingTime
	}
	typeStats.LastProcessed = time.Now()

	// 更新全局延迟统计
	pm.updateLatencyStats(processingTime)

	// 记录慢消息
	if processingTime > pm.config.SlowMessageThreshold {
		pm.recordSlowMessage(clientID, messageType, processingTime)
	}

	// 更新时间窗口统计
	pm.updateTimeWindowStats(processingTime, success)
}

// updateLatencyStats 更新延迟统计
func (pm *PerformanceMonitor) updateLatencyStats(latency time.Duration) {
	// 更新延迟桶
	bucket := pm.getLatencyBucket(latency)
	pm.messageStats.LatencyBuckets[bucket]++

	// 更新最小/最大延迟
	if pm.messageStats.MinLatency == 0 || latency < pm.messageStats.MinLatency {
		pm.messageStats.MinLatency = latency
	}
	if latency > pm.messageStats.MaxLatency {
		pm.messageStats.MaxLatency = latency
	}
}

// getLatencyBucket 获取延迟桶
func (pm *PerformanceMonitor) getLatencyBucket(latency time.Duration) string {
	ms := latency.Milliseconds()

	switch {
	case ms < 1:
		return "0-1ms"
	case ms < 5:
		return "1-5ms"
	case ms < 10:
		return "5-10ms"
	case ms < 50:
		return "10-50ms"
	case ms < 100:
		return "50-100ms"
	case ms < 500:
		return "100-500ms"
	case ms < 1000:
		return "500ms-1s"
	case ms < 5000:
		return "1s-5s"
	default:
		return "5s+"
	}
}

// recordSlowMessage 记录慢消息
func (pm *PerformanceMonitor) recordSlowMessage(clientID, messageType string, processingTime time.Duration) {
	atomic.AddInt64(&pm.messageStats.SlowMessages, 1)

	record := &SlowMessageRecord{
		Timestamp:      time.Now(),
		ClientID:       clientID,
		MessageType:    messageType,
		ProcessingTime: processingTime,
		Details:        fmt.Sprintf("Processing time exceeded threshold of %v", pm.config.SlowMessageThreshold),
	}

	// 限制慢消息记录数量
	if len(pm.messageStats.SlowMessageDetails) >= 100 {
		pm.messageStats.SlowMessageDetails = pm.messageStats.SlowMessageDetails[1:]
	}
	pm.messageStats.SlowMessageDetails = append(pm.messageStats.SlowMessageDetails, record)

	log.Printf("⚠️ Slow message detected: client=%s, type=%s, time=%v", clientID, messageType, processingTime)
}

// updateTimeWindowStats 更新时间窗口统计
func (pm *PerformanceMonitor) updateTimeWindowStats(latency time.Duration, success bool) {
	now := time.Now()

	// 更新最近一分钟统计
	if now.Sub(pm.messageStats.LastMinuteStats.StartTime) > time.Minute {
		pm.messageStats.LastMinuteStats = &TimeWindowStats{StartTime: now}
	}
	pm.messageStats.LastMinuteStats.MessageCount++
	if !success {
		pm.messageStats.LastMinuteStats.ErrorCount++
	}

	// 更新最近一小时统计
	if now.Sub(pm.messageStats.LastHourStats.StartTime) > time.Hour {
		pm.messageStats.LastHourStats = &TimeWindowStats{StartTime: now}
	}
	pm.messageStats.LastHourStats.MessageCount++
	if !success {
		pm.messageStats.LastHourStats.ErrorCount++
	}
}

// RecordConnectionEvent 记录连接事件
func (pm *PerformanceMonitor) RecordConnectionEvent(eventType string, clientID, appName string) {
	pm.connectionStats.mutex.Lock()
	defer pm.connectionStats.mutex.Unlock()

	switch eventType {
	case "connect":
		atomic.AddInt64(&pm.connectionStats.TotalConnections, 1)
		atomic.AddInt64(&pm.connectionStats.ActiveConnections, 1)

		// 更新峰值连接数
		if pm.connectionStats.ActiveConnections > pm.connectionStats.PeakConnections {
			pm.connectionStats.PeakConnections = pm.connectionStats.ActiveConnections
		}

		// 更新应用统计
		if pm.connectionStats.AppConnectionStats[appName] == nil {
			pm.connectionStats.AppConnectionStats[appName] = &AppConnectionStats{
				AppName: appName,
			}
		}
		appStats := pm.connectionStats.AppConnectionStats[appName]
		appStats.ActiveConnections++
		appStats.TotalConnections++
		appStats.LastActivity = time.Now()

	case "disconnect":
		atomic.AddInt64(&pm.connectionStats.ActiveConnections, -1)

		// 更新应用统计
		if appStats, exists := pm.connectionStats.AppConnectionStats[appName]; exists {
			appStats.ActiveConnections--
			appStats.LastActivity = time.Now()
		}
	}
}

// RecordRoutingStats 记录路由统计
func (pm *PerformanceMonitor) RecordRoutingStats(protocol string, routingTime time.Duration, success bool) {
	pm.routingStats.mutex.Lock()
	defer pm.routingStats.mutex.Unlock()

	atomic.AddInt64(&pm.routingStats.TotalRoutedMessages, 1)
	if !success {
		atomic.AddInt64(&pm.routingStats.RoutingErrors, 1)
	}

	// 更新协议统计
	if pm.routingStats.ProtocolStats[protocol] == nil {
		pm.routingStats.ProtocolStats[protocol] = &ProtocolRoutingStats{
			Protocol: protocol,
		}
	}

	protocolStats := pm.routingStats.ProtocolStats[protocol]
	protocolStats.MessageCount++
	if !success {
		protocolStats.ErrorCount++
	}

	// 更新平均路由时间
	totalTime := time.Duration(int64(protocolStats.AverageTime) * (protocolStats.MessageCount - 1))
	protocolStats.AverageTime = (totalTime + routingTime) / time.Duration(protocolStats.MessageCount)
	protocolStats.LastUsed = time.Now()
}

// collectSystemStats 收集系统统计
func (pm *PerformanceMonitor) collectSystemStats() {
	pm.systemStats.mutex.Lock()
	defer pm.systemStats.mutex.Unlock()

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	// 更新内存使用统计
	pm.systemStats.MemoryUsage.AllocatedBytes = memStats.Alloc
	pm.systemStats.MemoryUsage.TotalAllocatedBytes = memStats.TotalAlloc
	pm.systemStats.MemoryUsage.SystemBytes = memStats.Sys
	pm.systemStats.MemoryUsage.HeapBytes = memStats.HeapAlloc
	pm.systemStats.MemoryUsage.StackBytes = memStats.StackInuse
	pm.systemStats.MemoryUsage.GCPauseTotal = memStats.PauseTotalNs

	// 计算内存使用百分比
	if pm.config.HighMemoryThreshold > 0 {
		pm.systemStats.MemoryUsage.UsagePercentage = float64(memStats.Alloc) / float64(pm.config.HighMemoryThreshold) * 100
	}

	// 更新GC统计
	pm.systemStats.GCStats.NumGC = memStats.NumGC
	pm.systemStats.GCStats.PauseTotal = time.Duration(memStats.PauseTotalNs)
	if memStats.NumGC > 0 {
		pm.systemStats.GCStats.LastPause = time.Duration(memStats.PauseNs[(memStats.NumGC+255)%256])
		pm.systemStats.GCStats.AveragePause = time.Duration(memStats.PauseTotalNs / uint64(memStats.NumGC))
	}
	pm.systemStats.GCStats.GCCPUFraction = memStats.GCCPUFraction

	// 更新Goroutine数量
	pm.systemStats.GoroutineCount = runtime.NumGoroutine()

	pm.systemStats.LastUpdated = time.Now()
}

// checkAlerts 检查警报
func (pm *PerformanceMonitor) checkAlerts() {
	// 检查内存使用警报
	if pm.systemStats.MemoryUsage.UsagePercentage > 80 {
		pm.triggerAlert(&PerformanceAlert{
			Type:         "memory",
			Severity:     "warning",
			Message:      "High memory usage detected",
			Timestamp:    time.Now(),
			Threshold:    "80%",
			CurrentValue: pm.systemStats.MemoryUsage.UsagePercentage,
			Details: map[string]interface{}{
				"allocated_bytes": pm.systemStats.MemoryUsage.AllocatedBytes,
				"system_bytes":    pm.systemStats.MemoryUsage.SystemBytes,
			},
		})
	}

	// 检查连接数警报
	if pm.connectionStats.ActiveConnections > int64(pm.config.HighConnectionThreshold) {
		pm.triggerAlert(&PerformanceAlert{
			Type:         "connections",
			Severity:     "warning",
			Message:      "High connection count detected",
			Timestamp:    time.Now(),
			Threshold:    pm.config.HighConnectionThreshold,
			CurrentValue: pm.connectionStats.ActiveConnections,
		})
	}

	// 检查错误率警报
	if pm.messageStats.TotalMessages > 0 {
		errorRate := float64(pm.messageStats.FailedMessages) / float64(pm.messageStats.TotalMessages)
		if errorRate > pm.config.HighErrorRateThreshold {
			pm.triggerAlert(&PerformanceAlert{
				Type:         "error_rate",
				Severity:     "critical",
				Message:      "High error rate detected",
				Timestamp:    time.Now(),
				Threshold:    pm.config.HighErrorRateThreshold,
				CurrentValue: errorRate,
				Details: map[string]interface{}{
					"total_messages":  pm.messageStats.TotalMessages,
					"failed_messages": pm.messageStats.FailedMessages,
				},
			})
		}
	}
}

// triggerAlert 触发警报
func (pm *PerformanceMonitor) triggerAlert(alert *PerformanceAlert) {
	log.Printf("🚨 Performance Alert: %s - %s (Current: %v, Threshold: %v)",
		alert.Type, alert.Message, alert.CurrentValue, alert.Threshold)

	// 调用警报回调
	for _, callback := range pm.alertCallbacks {
		go callback(alert)
	}
}

// AddAlertCallback 添加警报回调
func (pm *PerformanceMonitor) AddAlertCallback(callback func(alert *PerformanceAlert)) {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()
	pm.alertCallbacks = append(pm.alertCallbacks, callback)
}

// generatePerformanceReport 生成性能报告
func (pm *PerformanceMonitor) generatePerformanceReport() {
	now := time.Now()
	duration := now.Sub(pm.lastReportTime)

	log.Printf("📊 === Performance Report (Period: %v) ===", duration)

	// 消息处理统计
	pm.messageStats.mutex.RLock()
	log.Printf("📨 Message Processing:")
	log.Printf("  Total Messages: %d", pm.messageStats.TotalMessages)
	log.Printf("  Success Rate: %.2f%%", float64(pm.messageStats.SuccessfulMessages)/float64(pm.messageStats.TotalMessages)*100)
	log.Printf("  Average Latency: %v", pm.messageStats.AverageLatency)
	log.Printf("  Slow Messages: %d", pm.messageStats.SlowMessages)

	// 按消息类型统计
	log.Printf("📋 Message Types:")
	for msgType, stats := range pm.messageStats.MessageTypeStats {
		successRate := float64(stats.SuccessCount) / float64(stats.Count) * 100
		log.Printf("  %s: %d messages, %.2f%% success, avg latency: %v",
			msgType, stats.Count, successRate, stats.AverageLatency)
	}
	pm.messageStats.mutex.RUnlock()

	// 连接统计
	pm.connectionStats.mutex.RLock()
	log.Printf("🔗 Connections:")
	log.Printf("  Active: %d, Total: %d, Peak: %d",
		pm.connectionStats.ActiveConnections,
		pm.connectionStats.TotalConnections,
		pm.connectionStats.PeakConnections)

	// 按应用统计
	log.Printf("📱 Applications:")
	for appName, stats := range pm.connectionStats.AppConnectionStats {
		log.Printf("  %s: %d active, %d total", appName, stats.ActiveConnections, stats.TotalConnections)
	}
	pm.connectionStats.mutex.RUnlock()

	// 系统统计
	pm.systemStats.mutex.RLock()
	log.Printf("💻 System:")
	log.Printf("  Memory: %.2f%% (%.2f MB allocated)",
		pm.systemStats.MemoryUsage.UsagePercentage,
		float64(pm.systemStats.MemoryUsage.AllocatedBytes)/1024/1024)
	log.Printf("  Goroutines: %d", pm.systemStats.GoroutineCount)
	log.Printf("  GC: %d collections, %.2f%% CPU",
		pm.systemStats.GCStats.NumGC,
		pm.systemStats.GCStats.GCCPUFraction*100)
	pm.systemStats.mutex.RUnlock()

	// 路由统计
	pm.routingStats.mutex.RLock()
	log.Printf("🚦 Message Routing:")
	log.Printf("  Total Routed: %d", pm.routingStats.TotalRoutedMessages)
	log.Printf("  Routing Errors: %d", pm.routingStats.RoutingErrors)
	log.Printf("  Average Routing Time: %v", pm.routingStats.AverageRoutingTime)
	pm.routingStats.mutex.RUnlock()

	log.Printf("📊 === End Performance Report ===")

	pm.lastReportTime = now
}

// GetStats 获取所有统计信息
func (pm *PerformanceMonitor) GetStats() map[string]interface{} {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()

	return map[string]interface{}{
		"message_stats":    pm.messageStats,
		"connection_stats": pm.connectionStats,
		"system_stats":     pm.systemStats,
		"routing_stats":    pm.routingStats,
		"config":           pm.config,
		"last_report_time": pm.lastReportTime,
	}
}

// GetMessageStats 获取消息统计
func (pm *PerformanceMonitor) GetMessageStats() *MessageProcessingStats {
	pm.messageStats.mutex.RLock()
	defer pm.messageStats.mutex.RUnlock()

	// 返回副本以避免并发访问问题
	stats := *pm.messageStats
	return &stats
}

// GetConnectionStats 获取连接统计
func (pm *PerformanceMonitor) GetConnectionStats() *ConnectionStats {
	pm.connectionStats.mutex.RLock()
	defer pm.connectionStats.mutex.RUnlock()

	stats := *pm.connectionStats
	return &stats
}

// GetSystemStats 获取系统统计
func (pm *PerformanceMonitor) GetSystemStats() *SystemStats {
	pm.systemStats.mutex.RLock()
	defer pm.systemStats.mutex.RUnlock()

	stats := *pm.systemStats
	return &stats
}

// ResetStats 重置统计信息
func (pm *PerformanceMonitor) ResetStats() {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	log.Printf("🔄 Resetting performance statistics")

	// 重置消息统计
	pm.messageStats.mutex.Lock()
	pm.messageStats.TotalMessages = 0
	pm.messageStats.SuccessfulMessages = 0
	pm.messageStats.FailedMessages = 0
	pm.messageStats.SlowMessages = 0
	pm.messageStats.MessageTypeStats = make(map[string]*MessageTypeStats)
	pm.messageStats.SlowMessageDetails = make([]*SlowMessageRecord, 0)
	pm.initializeLatencyBuckets()
	pm.messageStats.mutex.Unlock()

	// 重置路由统计
	pm.routingStats.mutex.Lock()
	pm.routingStats.TotalRoutedMessages = 0
	pm.routingStats.RoutingErrors = 0
	pm.routingStats.ProtocolStats = make(map[string]*ProtocolRoutingStats)
	pm.routingStats.mutex.Unlock()

	log.Printf("✅ Performance statistics reset completed")
}
