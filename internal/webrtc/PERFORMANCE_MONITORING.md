# WebRTC 服务器性能监控系统

## 概述

本文档描述了 WebRTC 信令服务器的性能监控和优化系统。该系统提供了全面的性能指标收集、实时监控、警报机制和并发优化功能。

## 功能特性

### 1. 消息处理延迟监控

- **实时延迟跟踪**: 记录每个消息的处理时间
- **延迟分布统计**: 按时间桶统计延迟分布
- **慢消息检测**: 自动识别和记录处理时间超过阈值的消息
- **按消息类型统计**: 分别统计不同消息类型的性能指标

```go
// 记录消息处理性能
performanceMonitor.RecordMessageProcessing(clientID, messageType, processingTime, success)

// 获取消息统计
messageStats := performanceMonitor.GetMessageStats()
fmt.Printf("平均延迟: %v", messageStats.AverageLatency)
fmt.Printf("慢消息数量: %d", messageStats.SlowMessages)
```

### 2. 高并发消息路由优化

- **工作池模式**: 使用多个工作器并行处理消息
- **负载均衡**: 支持轮询、最少负载、哈希等负载均衡策略
- **队列管理**: 智能消息队列管理，防止队列溢出
- **回退机制**: 并发路由失败时自动回退到标准路由

```go
// 创建并发路由器
config := DefaultConcurrentRouterConfig()
config.WorkerCount = 10
config.LoadBalanceMode = "least_loaded"

concurrentRouter := NewConcurrentMessageRouter(baseRouter, config, performanceMonitor)
concurrentRouter.Start()

// 路由消息（自动使用并发处理）
result, err := concurrentRouter.RouteMessage(messageBytes, clientID)
```

### 3. 消息处理统计和报告

- **实时统计**: 持续收集和更新性能指标
- **定期报告**: 自动生成详细的性能报告
- **多维度统计**: 按客户端、消息类型、协议等维度统计
- **时间窗口分析**: 提供最近一分钟、一小时的统计数据

```go
// 获取详细统计报告
report := server.GetDetailedPerformanceReport()

// 包含的统计信息：
// - 服务器基础统计
// - 消息处理统计
// - 连接统计
// - 系统资源统计
// - 并发路由统计
```

### 4. 内存使用和连接数监控

- **内存使用监控**: 实时监控内存分配和使用情况
- **连接数统计**: 跟踪活跃连接、总连接数、峰值连接数
- **系统资源监控**: 监控 Goroutine 数量、GC 统计等
- **警报机制**: 资源使用超过阈值时自动触发警报

```go
// 获取系统统计
systemStats := performanceMonitor.GetSystemStats()
fmt.Printf("内存使用: %.2f MB", float64(systemStats.MemoryUsage.AllocatedBytes)/1024/1024)
fmt.Printf("活跃连接: %d", connectionStats.ActiveConnections)
```

## 架构设计

### 核心组件

1. **PerformanceMonitor**: 主性能监控器
2. **ConcurrentMessageRouter**: 并发消息路由器
3. **MessageProcessingStats**: 消息处理统计
4. **ConnectionStats**: 连接统计
5. **SystemStats**: 系统统计
6. **WorkerPool**: 工作池管理

### 数据流

```
客户端消息 → 并发路由器 → 工作池 → 消息处理 → 性能记录
                ↓
            性能监控器 ← 统计收集 ← 系统监控
                ↓
            警报系统 → 报告生成 → 统计输出
```

## 配置选项

### 性能监控配置

```go
type PerformanceMonitorConfig struct {
    MonitorInterval         time.Duration // 监控间隔
    ReportInterval          time.Duration // 报告间隔
    SlowMessageThreshold    time.Duration // 慢消息阈值
    HighMemoryThreshold     uint64        // 高内存使用阈值
    HighConnectionThreshold int           // 高连接数阈值
    HighErrorRateThreshold  float64       // 高错误率阈值
    EnableConcurrentRouting bool          // 启用并发路由
    MaxConcurrentMessages   int           // 最大并发消息数
    DetailedStatsEnabled    bool          // 启用详细统计
}
```

### 并发路由器配置

```go
type ConcurrentRouterConfig struct {
    WorkerCount       int           // 工作器数量
    QueueSize         int           // 队列大小
    MaxQueueWaitTime  time.Duration // 最大队列等待时间
    LoadBalanceMode   string        // 负载均衡模式
    ProcessingTimeout time.Duration // 处理超时时间
    EnableMetrics     bool          // 启用指标收集
}
```

## 使用示例

### 基础使用

```go
// 1. 创建性能监控器
config := DefaultPerformanceMonitorConfig()
performanceMonitor := NewPerformanceMonitor(config)

// 2. 启动监控
performanceMonitor.Start()
defer performanceMonitor.Stop()

// 3. 记录性能数据
performanceMonitor.RecordMessageProcessing("client1", "ping", 10*time.Millisecond, true)
performanceMonitor.RecordConnectionEvent("connect", "client1", "app1")

// 4. 获取统计信息
stats := performanceMonitor.GetStats()
```

### 集成到信令服务器

```go
// 创建带性能监控的信令服务器
server := NewSignalingServer(ctx, encoderConfig, mediaStream, iceServers)

// 服务器自动包含性能监控功能
server.Start() // 自动启动性能监控

// 获取性能统计
performanceStats := server.GetPerformanceStats()
messageStats := server.GetMessageStats()
connectionStats := server.GetConnectionStats()
```

### 自定义警报处理

```go
// 添加警报回调
performanceMonitor.AddAlertCallback(func(alert *PerformanceAlert) {
    switch alert.Type {
    case "memory":
        log.Printf("内存使用过高: %v", alert.CurrentValue)
        // 执行内存清理操作
    case "connections":
        log.Printf("连接数过多: %v", alert.CurrentValue)
        // 限制新连接或清理无效连接
    case "error_rate":
        log.Printf("错误率过高: %v", alert.CurrentValue)
        // 检查系统状态或降级服务
    }
})
```

## 性能指标

### 消息处理指标

- **总消息数**: 处理的消息总数
- **成功率**: 成功处理的消息百分比
- **平均延迟**: 消息处理的平均时间
- **P95/P99延迟**: 95%和99%分位数延迟
- **慢消息数**: 超过阈值的消息数量
- **延迟分布**: 按时间桶的延迟分布

### 连接指标

- **活跃连接数**: 当前活跃的连接数
- **总连接数**: 累计连接总数
- **峰值连接数**: 历史最高连接数
- **平均连接时长**: 连接的平均持续时间
- **连接/断开速率**: 每分钟的连接和断开次数

### 系统指标

- **内存使用**: 当前内存分配和使用情况
- **CPU使用**: CPU使用百分比
- **Goroutine数量**: 当前Goroutine数量
- **GC统计**: 垃圾回收次数和暂停时间
- **系统负载**: 消息队列深度等负载指标

### 路由指标

- **路由消息数**: 路由处理的消息总数
- **路由错误数**: 路由处理失败的消息数
- **平均路由时间**: 消息路由的平均时间
- **并发度**: 当前并发处理的消息数
- **队列深度**: 待处理消息队列的深度

## 最佳实践

### 1. 配置优化

```go
// 生产环境推荐配置
config := &PerformanceMonitorConfig{
    MonitorInterval:         5 * time.Second,
    ReportInterval:          60 * time.Second,
    SlowMessageThreshold:    100 * time.Millisecond,
    HighMemoryThreshold:     2 * 1024 * 1024 * 1024, // 2GB
    HighConnectionThreshold: 10000,
    HighErrorRateThreshold:  0.01, // 1%
    EnableConcurrentRouting: true,
    MaxConcurrentMessages:   1000,
    DetailedStatsEnabled:    true,
}
```

### 2. 监控策略

- **实时监控**: 关注关键指标的实时变化
- **趋势分析**: 分析性能指标的长期趋势
- **异常检测**: 设置合理的阈值进行异常检测
- **容量规划**: 基于历史数据进行容量规划

### 3. 性能优化

- **并发调优**: 根据系统负载调整工作器数量
- **内存管理**: 定期检查内存使用，避免内存泄漏
- **连接管理**: 及时清理无效连接，控制连接数
- **消息优化**: 优化消息处理逻辑，减少处理时间

### 4. 故障排查

- **慢消息分析**: 分析慢消息的原因和模式
- **错误率监控**: 监控错误率变化，及时发现问题
- **资源瓶颈**: 识别CPU、内存、网络等资源瓶颈
- **并发问题**: 检查并发处理中的竞争条件

## 扩展功能

### 1. 自定义指标

```go
// 添加自定义指标收集
type CustomMetrics struct {
    BusinessMetric1 int64
    BusinessMetric2 float64
}

// 扩展性能监控器
func (pm *PerformanceMonitor) RecordCustomMetric(metric *CustomMetrics) {
    // 自定义指标记录逻辑
}
```

### 2. 外部集成

```go
// 集成外部监控系统（如 Prometheus）
func (pm *PerformanceMonitor) ExportToPrometheus() {
    // 导出指标到 Prometheus
}

// 集成日志系统
func (pm *PerformanceMonitor) ExportToElasticsearch() {
    // 导出日志到 Elasticsearch
}
```

### 3. 可视化界面

- **实时仪表板**: 显示关键性能指标
- **历史趋势图**: 展示性能指标的历史变化
- **警报面板**: 显示当前警报状态
- **详细报告**: 生成详细的性能分析报告

## 故障排查指南

### 常见问题

1. **内存使用过高**
   - 检查是否有内存泄漏
   - 分析大对象的分配
   - 调整GC参数

2. **消息处理延迟高**
   - 检查消息处理逻辑
   - 分析网络延迟
   - 优化数据库查询

3. **连接数异常**
   - 检查连接清理逻辑
   - 分析客户端行为
   - 调整连接超时设置

4. **并发路由问题**
   - 检查工作器配置
   - 分析队列深度
   - 调整负载均衡策略

### 调试技巧

```go
// 启用详细日志
config.EnableLogging = true

// 降低监控间隔以获得更细粒度的数据
config.MonitorInterval = 1 * time.Second

// 启用详细统计
config.DetailedStatsEnabled = true

// 添加调试回调
performanceMonitor.AddAlertCallback(func(alert *PerformanceAlert) {
    log.Printf("DEBUG: Alert details: %+v", alert.Details)
})
```

## 总结

WebRTC 服务器性能监控系统提供了全面的性能监控和优化功能，包括：

1. **全面的指标收集**: 涵盖消息处理、连接管理、系统资源等各个方面
2. **实时监控和警报**: 及时发现和响应性能问题
3. **并发优化**: 通过并发路由提高消息处理性能
4. **灵活的配置**: 支持根据不同场景进行配置调优
5. **易于集成**: 无缝集成到现有的信令服务器中

通过合理使用这些功能，可以显著提高 WebRTC 信令服务器的性能和稳定性，为用户提供更好的实时通信体验。