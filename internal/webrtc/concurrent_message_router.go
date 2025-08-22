package webrtc

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"
)

// ConcurrentMessageRouter 并发消息路由器
type ConcurrentMessageRouter struct {
	// 基础路由器
	baseRouter *MessageRouter

	// 并发控制
	workerPool   *WorkerPool
	messageQueue chan *MessageTask

	// 性能监控
	performanceMonitor *PerformanceMonitor

	// 配置
	config *ConcurrentRouterConfig

	// 统计
	stats *ConcurrentRoutingStats

	// 控制
	ctx     context.Context
	cancel  context.CancelFunc
	running int32

	mutex sync.RWMutex
}

// ConcurrentRouterConfig 并发路由器配置
type ConcurrentRouterConfig struct {
	// 工作池配置
	WorkerCount      int           `json:"worker_count"`
	QueueSize        int           `json:"queue_size"`
	MaxQueueWaitTime time.Duration `json:"max_queue_wait_time"`

	// 性能优化配置
	EnableBatching bool          `json:"enable_batching"`
	BatchSize      int           `json:"batch_size"`
	BatchTimeout   time.Duration `json:"batch_timeout"`

	// 负载均衡配置
	LoadBalanceMode string `json:"load_balance_mode"` // "round_robin", "least_loaded", "hash"

	// 超时配置
	ProcessingTimeout time.Duration `json:"processing_timeout"`

	// 监控配置
	EnableMetrics   bool          `json:"enable_metrics"`
	MetricsInterval time.Duration `json:"metrics_interval"`
}

// MessageTask 消息任务
type MessageTask struct {
	ID           string
	ClientID     string
	MessageBytes []byte
	SubmitTime   time.Time
	ResponseChan chan *MessageTaskResult
	Context      context.Context
	Priority     int
	Retries      int
	MaxRetries   int
}

// MessageTaskResult 消息任务结果
type MessageTaskResult struct {
	Success        bool
	Result         *RouteResult
	Error          error
	ProcessingTime time.Duration
	WorkerID       int
}

// WorkerPool 工作池
type WorkerPool struct {
	workers      []*MessageWorker
	workerCount  int
	nextWorker   int64
	loadBalancer LoadBalancer

	// 统计
	totalTasks     int64
	completedTasks int64
	failedTasks    int64

	mutex sync.RWMutex
}

// MessageWorker 消息工作器
type MessageWorker struct {
	ID       int
	router   *MessageRouter
	taskChan chan *MessageTask

	// 统计
	processedTasks int64
	failedTasks    int64
	totalTime      time.Duration
	isIdle         int32

	// 控制
	ctx    context.Context
	cancel context.CancelFunc

	mutex sync.RWMutex
}

// LoadBalancer 负载均衡器接口
type LoadBalancer interface {
	SelectWorker(workers []*MessageWorker, task *MessageTask) *MessageWorker
}

// RoundRobinBalancer 轮询负载均衡器
type RoundRobinBalancer struct {
	counter int64
}

// LeastLoadedBalancer 最少负载均衡器
type LeastLoadedBalancer struct{}

// HashBalancer 哈希负载均衡器
type HashBalancer struct{}

// ConcurrentRoutingStats 并发路由统计
type ConcurrentRoutingStats struct {
	// 任务统计
	TotalTasks     int64 `json:"total_tasks"`
	CompletedTasks int64 `json:"completed_tasks"`
	FailedTasks    int64 `json:"failed_tasks"`
	QueuedTasks    int64 `json:"queued_tasks"`

	// 性能统计
	AverageQueueTime   time.Duration `json:"average_queue_time"`
	AverageProcessTime time.Duration `json:"average_process_time"`
	ThroughputPerSec   float64       `json:"throughput_per_sec"`

	// 工作器统计
	ActiveWorkers     int     `json:"active_workers"`
	IdleWorkers       int     `json:"idle_workers"`
	WorkerUtilization float64 `json:"worker_utilization"`

	// 队列统计
	QueueDepth     int   `json:"queue_depth"`
	MaxQueueDepth  int   `json:"max_queue_depth"`
	QueueOverflows int64 `json:"queue_overflows"`

	mutex sync.RWMutex
}

// DefaultConcurrentRouterConfig 默认并发路由器配置
func DefaultConcurrentRouterConfig() *ConcurrentRouterConfig {
	return &ConcurrentRouterConfig{
		WorkerCount:       10,
		QueueSize:         1000,
		MaxQueueWaitTime:  5 * time.Second,
		EnableBatching:    false,
		BatchSize:         10,
		BatchTimeout:      100 * time.Millisecond,
		LoadBalanceMode:   "least_loaded",
		ProcessingTimeout: 30 * time.Second,
		EnableMetrics:     true,
		MetricsInterval:   10 * time.Second,
	}
}

// NewConcurrentMessageRouter 创建并发消息路由器
func NewConcurrentMessageRouter(baseRouter *MessageRouter, config *ConcurrentRouterConfig, performanceMonitor *PerformanceMonitor) *ConcurrentMessageRouter {
	if config == nil {
		config = DefaultConcurrentRouterConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	router := &ConcurrentMessageRouter{
		baseRouter:         baseRouter,
		messageQueue:       make(chan *MessageTask, config.QueueSize),
		performanceMonitor: performanceMonitor,
		config:             config,
		stats:              &ConcurrentRoutingStats{},
		ctx:                ctx,
		cancel:             cancel,
	}

	// 创建工作池
	router.workerPool = router.createWorkerPool()

	return router
}

// createWorkerPool 创建工作池
func (cmr *ConcurrentMessageRouter) createWorkerPool() *WorkerPool {
	var loadBalancer LoadBalancer

	switch cmr.config.LoadBalanceMode {
	case "round_robin":
		loadBalancer = &RoundRobinBalancer{}
	case "hash":
		loadBalancer = &HashBalancer{}
	default:
		loadBalancer = &LeastLoadedBalancer{}
	}

	pool := &WorkerPool{
		workers:      make([]*MessageWorker, cmr.config.WorkerCount),
		workerCount:  cmr.config.WorkerCount,
		loadBalancer: loadBalancer,
	}

	// 创建工作器
	for i := 0; i < cmr.config.WorkerCount; i++ {
		worker := cmr.createWorker(i)
		pool.workers[i] = worker
	}

	return pool
}

// createWorker 创建工作器
func (cmr *ConcurrentMessageRouter) createWorker(id int) *MessageWorker {
	ctx, cancel := context.WithCancel(cmr.ctx)

	worker := &MessageWorker{
		ID:       id,
		router:   cmr.baseRouter,
		taskChan: make(chan *MessageTask, 10), // 每个工作器的小缓冲区
		ctx:      ctx,
		cancel:   cancel,
		isIdle:   1, // 初始状态为空闲
	}

	return worker
}

// Start 启动并发路由器
func (cmr *ConcurrentMessageRouter) Start() error {
	if !atomic.CompareAndSwapInt32(&cmr.running, 0, 1) {
		return fmt.Errorf("concurrent router is already running")
	}

	log.Printf("🚀 Starting concurrent message router with %d workers", cmr.config.WorkerCount)

	// 启动工作器
	for _, worker := range cmr.workerPool.workers {
		go worker.start()
	}

	// 启动任务分发器
	go cmr.taskDispatcher()

	// 启动监控器
	if cmr.config.EnableMetrics {
		go cmr.metricsCollector()
	}

	log.Printf("✅ Concurrent message router started")
	return nil
}

// Stop 停止并发路由器
func (cmr *ConcurrentMessageRouter) Stop() error {
	if !atomic.CompareAndSwapInt32(&cmr.running, 1, 0) {
		return fmt.Errorf("concurrent router is not running")
	}

	log.Printf("🛑 Stopping concurrent message router...")

	// 停止接收新任务
	close(cmr.messageQueue)

	// 停止所有工作器
	for _, worker := range cmr.workerPool.workers {
		worker.stop()
	}

	// 取消上下文
	cmr.cancel()

	log.Printf("✅ Concurrent message router stopped")
	return nil
}

// RouteMessage 路由消息（并发版本）
func (cmr *ConcurrentMessageRouter) RouteMessage(messageBytes []byte, clientID string) (*RouteResult, error) {
	if atomic.LoadInt32(&cmr.running) == 0 {
		return nil, fmt.Errorf("concurrent router is not running")
	}

	// 创建任务
	task := &MessageTask{
		ID:           fmt.Sprintf("task_%d_%s", time.Now().UnixNano(), clientID),
		ClientID:     clientID,
		MessageBytes: messageBytes,
		SubmitTime:   time.Now(),
		ResponseChan: make(chan *MessageTaskResult, 1),
		Context:      context.WithValue(context.Background(), "client_id", clientID),
		Priority:     0,
		MaxRetries:   3,
	}

	// 提交任务到队列
	select {
	case cmr.messageQueue <- task:
		atomic.AddInt64(&cmr.stats.TotalTasks, 1)
		atomic.AddInt64(&cmr.stats.QueuedTasks, 1)
	case <-time.After(cmr.config.MaxQueueWaitTime):
		atomic.AddInt64(&cmr.stats.QueueOverflows, 1)
		return nil, fmt.Errorf("message queue is full, timeout after %v", cmr.config.MaxQueueWaitTime)
	}

	// 等待结果
	select {
	case result := <-task.ResponseChan:
		atomic.AddInt64(&cmr.stats.QueuedTasks, -1)

		if result.Success {
			atomic.AddInt64(&cmr.stats.CompletedTasks, 1)

			// 记录性能指标
			if cmr.performanceMonitor != nil {
				queueTime := result.ProcessingTime // 这里简化了，实际应该分别记录队列时间和处理时间
				cmr.performanceMonitor.RecordRoutingStats("concurrent", queueTime, true)
			}

			return result.Result, nil
		} else {
			atomic.AddInt64(&cmr.stats.FailedTasks, 1)

			// 记录失败指标
			if cmr.performanceMonitor != nil {
				cmr.performanceMonitor.RecordRoutingStats("concurrent", result.ProcessingTime, false)
			}

			return nil, result.Error
		}

	case <-time.After(cmr.config.ProcessingTimeout):
		atomic.AddInt64(&cmr.stats.QueuedTasks, -1)
		atomic.AddInt64(&cmr.stats.FailedTasks, 1)
		return nil, fmt.Errorf("message processing timeout after %v", cmr.config.ProcessingTimeout)
	}
}

// taskDispatcher 任务分发器
func (cmr *ConcurrentMessageRouter) taskDispatcher() {
	log.Printf("📡 Task dispatcher started")

	for {
		select {
		case task, ok := <-cmr.messageQueue:
			if !ok {
				log.Printf("📡 Task dispatcher stopping - message queue closed")
				return
			}

			// 选择工作器
			worker := cmr.workerPool.loadBalancer.SelectWorker(cmr.workerPool.workers, task)
			if worker == nil {
				// 所有工作器都忙，使用轮询选择
				workerIndex := atomic.AddInt64(&cmr.workerPool.nextWorker, 1) % int64(cmr.workerPool.workerCount)
				worker = cmr.workerPool.workers[workerIndex]
			}

			// 分发任务到工作器
			select {
			case worker.taskChan <- task:
				// 任务成功分发
			case <-time.After(100 * time.Millisecond):
				// 工作器忙，发送失败响应
				task.ResponseChan <- &MessageTaskResult{
					Success:        false,
					Error:          fmt.Errorf("worker %d is busy", worker.ID),
					ProcessingTime: time.Since(task.SubmitTime),
					WorkerID:       worker.ID,
				}
			}

		case <-cmr.ctx.Done():
			log.Printf("📡 Task dispatcher stopping - context cancelled")
			return
		}
	}
}

// metricsCollector 指标收集器
func (cmr *ConcurrentMessageRouter) metricsCollector() {
	ticker := time.NewTicker(cmr.config.MetricsInterval)
	defer ticker.Stop()

	log.Printf("📊 Metrics collector started")

	for {
		select {
		case <-ticker.C:
			cmr.collectMetrics()
		case <-cmr.ctx.Done():
			log.Printf("📊 Metrics collector stopping")
			return
		}
	}
}

// collectMetrics 收集指标
func (cmr *ConcurrentMessageRouter) collectMetrics() {
	cmr.stats.mutex.Lock()
	defer cmr.stats.mutex.Unlock()

	// 更新队列深度
	cmr.stats.QueueDepth = len(cmr.messageQueue)
	if cmr.stats.QueueDepth > cmr.stats.MaxQueueDepth {
		cmr.stats.MaxQueueDepth = cmr.stats.QueueDepth
	}

	// 统计工作器状态
	activeWorkers := 0
	idleWorkers := 0

	for _, worker := range cmr.workerPool.workers {
		if atomic.LoadInt32(&worker.isIdle) == 0 {
			activeWorkers++
		} else {
			idleWorkers++
		}
	}

	cmr.stats.ActiveWorkers = activeWorkers
	cmr.stats.IdleWorkers = idleWorkers
	cmr.stats.WorkerUtilization = float64(activeWorkers) / float64(cmr.config.WorkerCount) * 100

	// 计算吞吐量
	if cmr.stats.CompletedTasks > 0 {
		duration := time.Since(time.Now().Add(-cmr.config.MetricsInterval))
		cmr.stats.ThroughputPerSec = float64(cmr.stats.CompletedTasks) / duration.Seconds()
	}

	// 记录详细指标
	log.Printf("📊 Concurrent Router Metrics: Queue=%d, Active=%d/%d, Throughput=%.2f/s, Utilization=%.1f%%",
		cmr.stats.QueueDepth, activeWorkers, cmr.config.WorkerCount,
		cmr.stats.ThroughputPerSec, cmr.stats.WorkerUtilization)
}

// start 启动工作器
func (mw *MessageWorker) start() {
	log.Printf("👷 Worker %d started", mw.ID)

	for {
		select {
		case task, ok := <-mw.taskChan:
			if !ok {
				log.Printf("👷 Worker %d stopping - task channel closed", mw.ID)
				return
			}

			mw.processTask(task)

		case <-mw.ctx.Done():
			log.Printf("👷 Worker %d stopping - context cancelled", mw.ID)
			return
		}
	}
}

// stop 停止工作器
func (mw *MessageWorker) stop() {
	log.Printf("👷 Worker %d stopping...", mw.ID)
	close(mw.taskChan)
	mw.cancel()
}

// processTask 处理任务
func (mw *MessageWorker) processTask(task *MessageTask) {
	startTime := time.Now()
	atomic.StoreInt32(&mw.isIdle, 0)       // 标记为忙碌
	defer atomic.StoreInt32(&mw.isIdle, 1) // 标记为空闲

	mw.mutex.Lock()
	mw.processedTasks++
	mw.mutex.Unlock()

	// 处理消息
	result, err := mw.router.RouteMessage(task.MessageBytes, task.ClientID)
	processingTime := time.Since(startTime)

	// 更新工作器统计
	mw.mutex.Lock()
	mw.totalTime += processingTime
	if err != nil {
		mw.failedTasks++
	}
	mw.mutex.Unlock()

	// 发送结果
	taskResult := &MessageTaskResult{
		Success:        err == nil,
		Result:         result,
		Error:          err,
		ProcessingTime: processingTime,
		WorkerID:       mw.ID,
	}

	select {
	case task.ResponseChan <- taskResult:
		// 结果发送成功
	case <-time.After(1 * time.Second):
		// 结果发送超时，记录错误
		log.Printf("❌ Worker %d failed to send result for task %s", mw.ID, task.ID)
	}
}

// GetStats 获取工作器统计
func (mw *MessageWorker) GetStats() map[string]interface{} {
	mw.mutex.RLock()
	defer mw.mutex.RUnlock()

	var avgTime time.Duration
	if mw.processedTasks > 0 {
		avgTime = mw.totalTime / time.Duration(mw.processedTasks)
	}

	return map[string]interface{}{
		"worker_id":       mw.ID,
		"processed_tasks": mw.processedTasks,
		"failed_tasks":    mw.failedTasks,
		"average_time":    avgTime,
		"is_idle":         atomic.LoadInt32(&mw.isIdle) == 1,
	}
}

// SelectWorker 轮询选择工作器
func (rrb *RoundRobinBalancer) SelectWorker(workers []*MessageWorker, task *MessageTask) *MessageWorker {
	if len(workers) == 0 {
		return nil
	}

	index := atomic.AddInt64(&rrb.counter, 1) % int64(len(workers))
	return workers[index]
}

// SelectWorker 选择负载最少的工作器
func (llb *LeastLoadedBalancer) SelectWorker(workers []*MessageWorker, task *MessageTask) *MessageWorker {
	if len(workers) == 0 {
		return nil
	}

	var bestWorker *MessageWorker
	var minLoad int64 = -1

	for _, worker := range workers {
		// 优先选择空闲的工作器
		if atomic.LoadInt32(&worker.isIdle) == 1 {
			return worker
		}

		// 选择处理任务最少的工作器
		worker.mutex.RLock()
		load := worker.processedTasks
		worker.mutex.RUnlock()

		if minLoad == -1 || load < minLoad {
			minLoad = load
			bestWorker = worker
		}
	}

	return bestWorker
}

// SelectWorker 基于哈希选择工作器
func (hb *HashBalancer) SelectWorker(workers []*MessageWorker, task *MessageTask) *MessageWorker {
	if len(workers) == 0 {
		return nil
	}

	// 使用客户端ID的哈希值选择工作器
	hash := 0
	for _, b := range []byte(task.ClientID) {
		hash = hash*31 + int(b)
	}

	index := hash % len(workers)
	if index < 0 {
		index = -index
	}

	return workers[index]
}

// GetStats 获取并发路由统计
func (cmr *ConcurrentMessageRouter) GetStats() *ConcurrentRoutingStats {
	cmr.stats.mutex.RLock()
	defer cmr.stats.mutex.RUnlock()

	stats := *cmr.stats
	return &stats
}

// GetWorkerStats 获取所有工作器统计
func (cmr *ConcurrentMessageRouter) GetWorkerStats() []map[string]interface{} {
	stats := make([]map[string]interface{}, len(cmr.workerPool.workers))

	for i, worker := range cmr.workerPool.workers {
		stats[i] = worker.GetStats()
	}

	return stats
}

// GetDetailedStats 获取详细统计信息
func (cmr *ConcurrentMessageRouter) GetDetailedStats() map[string]interface{} {
	return map[string]interface{}{
		"config":        cmr.config,
		"routing_stats": cmr.GetStats(),
		"worker_stats":  cmr.GetWorkerStats(),
		"queue_size":    len(cmr.messageQueue),
		"queue_cap":     cap(cmr.messageQueue),
		"running":       atomic.LoadInt32(&cmr.running) == 1,
	}
}
