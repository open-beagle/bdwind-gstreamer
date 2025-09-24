package gstreamer

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ContinuousMemoryStabilityTest 连续内存稳定性测试
type ContinuousMemoryStabilityTest struct {
	config  *MemoryStabilityConfig
	metrics *MemoryStabilityMetrics
	ctx     context.Context
	cancel  context.CancelFunc
	logger  *logrus.Entry
	running atomic.Bool

	// 内存基线
	baselineMemoryMB   int64
	baselineGoroutines int

	// 测试组件
	memoryManager *MemoryManager
	stateManager  *StateManager
	errorManager  *ErrorManager

	// 监控通道
	memoryAlerts   chan MemoryAlert
	leakDetections chan LeakDetection
}

// MemoryStabilityConfig 内存稳定性测试配置
type MemoryStabilityConfig struct {
	TestDuration       time.Duration
	CheckInterval      time.Duration
	AlertThresholdMB   int64
	LeakThresholdMB    int64
	GoroutineThreshold int
	AllocationRate     int // 每秒分配次数
	AllocationSizeMB   int // 每次分配大小(MB)
	CleanupInterval    time.Duration
	GCInterval         time.Duration
	ReportInterval     time.Duration
}

// DefaultMemoryStabilityConfig 默认内存稳定性配置
func DefaultMemoryStabilityConfig() *MemoryStabilityConfig {
	return &MemoryStabilityConfig{
		TestDuration:       24 * time.Hour,
		CheckInterval:      30 * time.Second,
		AlertThresholdMB:   200,
		LeakThresholdMB:    500,
		GoroutineThreshold: 1000,
		AllocationRate:     100, // 100次/秒
		AllocationSizeMB:   1,   // 1MB每次
		CleanupInterval:    5 * time.Minute,
		GCInterval:         1 * time.Minute,
		ReportInterval:     10 * time.Minute,
	}
}

// MemoryStabilityMetrics 内存稳定性指标
type MemoryStabilityMetrics struct {
	StartTime          time.Time
	EndTime            time.Time
	TotalChecks        int64
	MemoryAlerts       int64
	LeakDetections     int64
	GoroutineLeaks     int64
	MaxMemoryUsageMB   int64
	MinMemoryUsageMB   int64
	AvgMemoryUsageMB   int64
	MaxGoroutines      int64
	TotalAllocations   int64
	TotalDeallocations int64
	GCCycles           int64
	CleanupCycles      int64

	// 内存使用历史
	MemoryHistory []ContinuousMemorySnapshot
	mutex         sync.RWMutex
}

// ContinuousMemorySnapshot 连续内存快照
type ContinuousMemorySnapshot struct {
	Timestamp      time.Time
	MemoryUsageMB  int64
	GoroutineCount int
	HeapSizeMB     int64
	StackSizeMB    int64
	GCCount        uint32
}

// MemoryAlert 内存警报
type MemoryAlert struct {
	Timestamp      time.Time
	AlertType      string
	CurrentUsageMB int64
	ThresholdMB    int64
	Message        string
}

// LeakDetection 泄漏检测
type LeakDetection struct {
	Timestamp      time.Time
	LeakType       string
	LeakSizeMB     int64
	GoroutineCount int
	StackTrace     string
}

// NewContinuousMemoryStabilityTest 创建连续内存稳定性测试
func NewContinuousMemoryStabilityTest(config *MemoryStabilityConfig) *ContinuousMemoryStabilityTest {
	if config == nil {
		config = DefaultMemoryStabilityConfig()
	}

	ctx, cancel := context.WithTimeout(context.Background(), config.TestDuration)

	return &ContinuousMemoryStabilityTest{
		config:         config,
		metrics:        &MemoryStabilityMetrics{StartTime: time.Now()},
		ctx:            ctx,
		cancel:         cancel,
		logger:         logrus.WithField("component", "memory_stability_test"),
		memoryAlerts:   make(chan MemoryAlert, 100),
		leakDetections: make(chan LeakDetection, 100),
	}
}

// Start 启动连续内存稳定性测试
func (cmst *ContinuousMemoryStabilityTest) Start(t *testing.T) error {
	if !cmst.running.CompareAndSwap(false, true) {
		return fmt.Errorf("memory stability test is already running")
	}

	cmst.logger.Info("Starting continuous memory stability test")

	// 初始化组件
	cmst.memoryManager = NewMemoryManager()
	cmst.stateManager = NewStateManager()
	cmst.errorManager = NewErrorManager()

	// 记录基线
	cmst.recordBaseline()

	// 启动监控协程
	var wg sync.WaitGroup

	// 内存监控协程
	wg.Add(1)
	go func() {
		defer wg.Done()
		cmst.memoryMonitorLoop(t)
	}()

	// 内存分配协程
	wg.Add(1)
	go func() {
		defer wg.Done()
		cmst.memoryAllocationLoop(t)
	}()

	// 清理协程
	wg.Add(1)
	go func() {
		defer wg.Done()
		cmst.cleanupLoop(t)
	}()

	// GC协程
	wg.Add(1)
	go func() {
		defer wg.Done()
		cmst.gcLoop(t)
	}()

	// 报告协程
	wg.Add(1)
	go func() {
		defer wg.Done()
		cmst.reportLoop(t)
	}()

	// 警报处理协程
	wg.Add(1)
	go func() {
		defer wg.Done()
		cmst.alertHandlerLoop(t)
	}()

	// 等待测试完成
	<-cmst.ctx.Done()

	// 停止所有协程
	cmst.running.Store(false)
	cmst.cancel()

	// 等待所有协程结束
	wg.Wait()

	// 记录结束时间
	cmst.metrics.EndTime = time.Now()

	cmst.logger.Info("Continuous memory stability test completed")
	return nil
}

// recordBaseline 记录基线指标
func (cmst *ContinuousMemoryStabilityTest) recordBaseline() {
	var m runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m)

	cmst.baselineMemoryMB = int64(m.Alloc / 1024 / 1024)
	cmst.baselineGoroutines = runtime.NumGoroutine()

	// 初始化指标
	cmst.metrics.MinMemoryUsageMB = cmst.baselineMemoryMB
	cmst.metrics.MaxMemoryUsageMB = cmst.baselineMemoryMB
	cmst.metrics.AvgMemoryUsageMB = cmst.baselineMemoryMB

	cmst.logger.WithFields(logrus.Fields{
		"baseline_memory_mb":  cmst.baselineMemoryMB,
		"baseline_goroutines": cmst.baselineGoroutines,
	}).Info("Recorded baseline metrics")
}

// memoryMonitorLoop 内存监控循环
func (cmst *ContinuousMemoryStabilityTest) memoryMonitorLoop(t *testing.T) {
	ticker := time.NewTicker(cmst.config.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-cmst.ctx.Done():
			return
		case <-ticker.C:
			if !cmst.running.Load() {
				return
			}
			cmst.performMemoryCheck(t)
		}
	}
}

// performMemoryCheck 执行内存检查
func (cmst *ContinuousMemoryStabilityTest) performMemoryCheck(t *testing.T) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	currentMemoryMB := int64(m.Alloc / 1024 / 1024)
	currentGoroutines := runtime.NumGoroutine()
	heapSizeMB := int64(m.HeapSys / 1024 / 1024)
	stackSizeMB := int64(m.StackSys / 1024 / 1024)

	// 创建内存快照
	snapshot := ContinuousMemorySnapshot{
		Timestamp:      time.Now(),
		MemoryUsageMB:  currentMemoryMB,
		GoroutineCount: currentGoroutines,
		HeapSizeMB:     heapSizeMB,
		StackSizeMB:    stackSizeMB,
		GCCount:        m.NumGC,
	}

	// 更新指标
	cmst.updateMetrics(snapshot)

	// 检查内存增长
	memoryGrowth := currentMemoryMB - cmst.baselineMemoryMB
	if memoryGrowth > cmst.config.AlertThresholdMB {
		alert := MemoryAlert{
			Timestamp:      time.Now(),
			AlertType:      "memory_growth",
			CurrentUsageMB: currentMemoryMB,
			ThresholdMB:    cmst.config.AlertThresholdMB,
			Message:        fmt.Sprintf("Memory growth exceeded threshold: %d MB", memoryGrowth),
		}

		select {
		case cmst.memoryAlerts <- alert:
		default:
			// 警报通道满了，记录日志
			cmst.logger.Warn("Memory alert channel is full")
		}
	}

	// 检查内存泄漏
	if memoryGrowth > cmst.config.LeakThresholdMB {
		leak := LeakDetection{
			Timestamp:      time.Now(),
			LeakType:       "memory_leak",
			LeakSizeMB:     memoryGrowth,
			GoroutineCount: currentGoroutines,
			StackTrace:     cmst.captureStackTrace(),
		}

		select {
		case cmst.leakDetections <- leak:
		default:
			cmst.logger.Warn("Leak detection channel is full")
		}
	}

	// 检查协程泄漏
	goroutineGrowth := currentGoroutines - cmst.baselineGoroutines
	if goroutineGrowth > cmst.config.GoroutineThreshold {
		atomic.AddInt64(&cmst.metrics.GoroutineLeaks, 1)

		cmst.logger.WithFields(logrus.Fields{
			"current_goroutines":  currentGoroutines,
			"baseline_goroutines": cmst.baselineGoroutines,
			"growth":              goroutineGrowth,
		}).Warn("Goroutine leak detected")
	}

	atomic.AddInt64(&cmst.metrics.TotalChecks, 1)
}

// updateMetrics 更新指标
func (cmst *ContinuousMemoryStabilityTest) updateMetrics(snapshot ContinuousMemorySnapshot) {
	cmst.metrics.mutex.Lock()
	defer cmst.metrics.mutex.Unlock()

	// 更新内存使用统计
	if snapshot.MemoryUsageMB > cmst.metrics.MaxMemoryUsageMB {
		cmst.metrics.MaxMemoryUsageMB = snapshot.MemoryUsageMB
	}
	if snapshot.MemoryUsageMB < cmst.metrics.MinMemoryUsageMB {
		cmst.metrics.MinMemoryUsageMB = snapshot.MemoryUsageMB
	}

	// 更新协程统计
	if int64(snapshot.GoroutineCount) > cmst.metrics.MaxGoroutines {
		cmst.metrics.MaxGoroutines = int64(snapshot.GoroutineCount)
	}

	// 计算平均内存使用
	totalChecks := atomic.LoadInt64(&cmst.metrics.TotalChecks)
	if totalChecks == 0 {
		cmst.metrics.AvgMemoryUsageMB = snapshot.MemoryUsageMB
	} else {
		cmst.metrics.AvgMemoryUsageMB = (cmst.metrics.AvgMemoryUsageMB*totalChecks + snapshot.MemoryUsageMB) / (totalChecks + 1)
	}

	// 添加到历史记录
	cmst.metrics.MemoryHistory = append(cmst.metrics.MemoryHistory, snapshot)

	// 限制历史记录大小
	const maxHistorySize = 10000
	if len(cmst.metrics.MemoryHistory) > maxHistorySize {
		cmst.metrics.MemoryHistory = cmst.metrics.MemoryHistory[1:]
	}
}

// memoryAllocationLoop 内存分配循环
func (cmst *ContinuousMemoryStabilityTest) memoryAllocationLoop(t *testing.T) {
	interval := time.Second / time.Duration(cmst.config.AllocationRate)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	allocations := make(map[int][]byte)
	allocationID := 0

	for {
		select {
		case <-cmst.ctx.Done():
			return
		case <-ticker.C:
			if !cmst.running.Load() {
				return
			}

			// 分配内存
			size := cmst.config.AllocationSizeMB * 1024 * 1024
			data := make([]byte, size)

			// 填充数据以确保实际分配
			for i := range data {
				data[i] = byte(allocationID % 256)
			}

			allocations[allocationID] = data
			allocationID++

			atomic.AddInt64(&cmst.metrics.TotalAllocations, 1)

			// 随机释放一些旧的分配
			if len(allocations) > 100 && allocationID%10 == 0 {
				// 释放最旧的分配
				for id := range allocations {
					delete(allocations, id)
					atomic.AddInt64(&cmst.metrics.TotalDeallocations, 1)
					break
				}
			}
		}
	}
}

// cleanupLoop 清理循环
func (cmst *ContinuousMemoryStabilityTest) cleanupLoop(t *testing.T) {
	ticker := time.NewTicker(cmst.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-cmst.ctx.Done():
			return
		case <-ticker.C:
			if !cmst.running.Load() {
				return
			}
			cmst.performCleanup()
		}
	}
}

// performCleanup 执行清理
func (cmst *ContinuousMemoryStabilityTest) performCleanup() {
	cmst.logger.Debug("Performing cleanup cycle")

	// 清理内存管理器
	if cmst.memoryManager != nil {
		stats := cmst.memoryManager.GetMemoryStats()
		cmst.logger.WithFields(logrus.Fields{
			"registered_objects": stats.RegisteredObjects,
			"buffer_pools":       stats.BufferPools,
		}).Debug("Memory manager stats before cleanup")

		// 执行内存管理器清理
		// 这里可以添加具体的清理逻辑
	}

	atomic.AddInt64(&cmst.metrics.CleanupCycles, 1)
}

// gcLoop GC循环
func (cmst *ContinuousMemoryStabilityTest) gcLoop(t *testing.T) {
	ticker := time.NewTicker(cmst.config.GCInterval)
	defer ticker.Stop()

	for {
		select {
		case <-cmst.ctx.Done():
			return
		case <-ticker.C:
			if !cmst.running.Load() {
				return
			}
			cmst.performGC()
		}
	}
}

// performGC 执行GC
func (cmst *ContinuousMemoryStabilityTest) performGC() {
	cmst.logger.Debug("Performing GC cycle")

	var beforeStats, afterStats runtime.MemStats
	runtime.ReadMemStats(&beforeStats)

	runtime.GC()

	runtime.ReadMemStats(&afterStats)

	freedMB := int64(beforeStats.Alloc-afterStats.Alloc) / 1024 / 1024

	cmst.logger.WithFields(logrus.Fields{
		"freed_mb": freedMB,
		"gc_count": afterStats.NumGC,
	}).Debug("GC cycle completed")

	atomic.AddInt64(&cmst.metrics.GCCycles, 1)
}

// reportLoop 报告循环
func (cmst *ContinuousMemoryStabilityTest) reportLoop(t *testing.T) {
	ticker := time.NewTicker(cmst.config.ReportInterval)
	defer ticker.Stop()

	for {
		select {
		case <-cmst.ctx.Done():
			return
		case <-ticker.C:
			if !cmst.running.Load() {
				return
			}
			cmst.generateReport()
		}
	}
}

// generateReport 生成报告
func (cmst *ContinuousMemoryStabilityTest) generateReport() {
	elapsed := time.Since(cmst.metrics.StartTime)
	remaining := cmst.config.TestDuration - elapsed

	var currentStats runtime.MemStats
	runtime.ReadMemStats(&currentStats)

	currentMemoryMB := int64(currentStats.Alloc / 1024 / 1024)
	memoryGrowth := currentMemoryMB - cmst.baselineMemoryMB

	cmst.logger.WithFields(logrus.Fields{
		"elapsed":           elapsed,
		"remaining":         remaining,
		"progress_percent":  float64(elapsed) / float64(cmst.config.TestDuration) * 100,
		"current_memory_mb": currentMemoryMB,
		"memory_growth_mb":  memoryGrowth,
		"total_checks":      atomic.LoadInt64(&cmst.metrics.TotalChecks),
		"memory_alerts":     atomic.LoadInt64(&cmst.metrics.MemoryAlerts),
		"leak_detections":   atomic.LoadInt64(&cmst.metrics.LeakDetections),
		"goroutine_leaks":   atomic.LoadInt64(&cmst.metrics.GoroutineLeaks),
		"total_allocations": atomic.LoadInt64(&cmst.metrics.TotalAllocations),
		"gc_cycles":         atomic.LoadInt64(&cmst.metrics.GCCycles),
	}).Info("Memory stability test progress report")
}

// alertHandlerLoop 警报处理循环
func (cmst *ContinuousMemoryStabilityTest) alertHandlerLoop(t *testing.T) {
	for {
		select {
		case <-cmst.ctx.Done():
			return
		case alert := <-cmst.memoryAlerts:
			cmst.handleMemoryAlert(alert, t)
		case leak := <-cmst.leakDetections:
			cmst.handleLeakDetection(leak, t)
		}
	}
}

// handleMemoryAlert 处理内存警报
func (cmst *ContinuousMemoryStabilityTest) handleMemoryAlert(alert MemoryAlert, t *testing.T) {
	atomic.AddInt64(&cmst.metrics.MemoryAlerts, 1)

	cmst.logger.WithFields(logrus.Fields{
		"alert_type":       alert.AlertType,
		"current_usage_mb": alert.CurrentUsageMB,
		"threshold_mb":     alert.ThresholdMB,
		"message":          alert.Message,
	}).Warn("Memory alert triggered")

	// 在测试模式下记录警报但不立即失败
	if testing.Short() {
		t.Logf("Memory alert: %s", alert.Message)
	}
}

// handleLeakDetection 处理泄漏检测
func (cmst *ContinuousMemoryStabilityTest) handleLeakDetection(leak LeakDetection, t *testing.T) {
	atomic.AddInt64(&cmst.metrics.LeakDetections, 1)

	cmst.logger.WithFields(logrus.Fields{
		"leak_type":       leak.LeakType,
		"leak_size_mb":    leak.LeakSizeMB,
		"goroutine_count": leak.GoroutineCount,
	}).Error("Memory leak detected")

	// 记录堆栈跟踪
	if leak.StackTrace != "" {
		cmst.logger.WithField("stack_trace", leak.StackTrace).Debug("Leak stack trace")
	}
}

// captureStackTrace 捕获堆栈跟踪
func (cmst *ContinuousMemoryStabilityTest) captureStackTrace() string {
	buf := make([]byte, 4096)
	n := runtime.Stack(buf, false)
	return string(buf[:n])
}

// GetFinalReport 获取最终报告
func (cmst *ContinuousMemoryStabilityTest) GetFinalReport() string {
	duration := cmst.metrics.EndTime.Sub(cmst.metrics.StartTime)

	return fmt.Sprintf(`
连续内存稳定性测试报告:
========================
测试时长: %v
总检查次数: %d
内存警报次数: %d
泄漏检测次数: %d
协程泄漏次数: %d
最大内存使用: %d MB
最小内存使用: %d MB
平均内存使用: %d MB
最大协程数: %d
总分配次数: %d
总释放次数: %d
GC周期数: %d
清理周期数: %d
`, duration, cmst.metrics.TotalChecks, cmst.metrics.MemoryAlerts,
		cmst.metrics.LeakDetections, cmst.metrics.GoroutineLeaks,
		cmst.metrics.MaxMemoryUsageMB, cmst.metrics.MinMemoryUsageMB,
		cmst.metrics.AvgMemoryUsageMB, cmst.metrics.MaxGoroutines,
		cmst.metrics.TotalAllocations, cmst.metrics.TotalDeallocations,
		cmst.metrics.GCCycles, cmst.metrics.CleanupCycles)
}

// TestContinuousMemoryStability24Hours 24小时连续内存稳定性测试
func TestContinuousMemoryStability24Hours(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping 24-hour continuous memory stability test in short mode")
	}

	if !isLongRunningTestEnabled() {
		t.Skip("Long-running tests are disabled. Set ENABLE_LONG_RUNNING_TESTS=true to enable")
	}

	// 获取测试持续时间
	duration := getTestDuration(24 * time.Hour)

	config := &MemoryStabilityConfig{
		TestDuration:       duration,
		CheckInterval:      30 * time.Second,
		AlertThresholdMB:   200,
		LeakThresholdMB:    500,
		GoroutineThreshold: 1000,
		AllocationRate:     50, // 50次/秒，适中的分配率
		AllocationSizeMB:   1,  // 1MB每次
		CleanupInterval:    5 * time.Minute,
		GCInterval:         1 * time.Minute,
		ReportInterval:     10 * time.Minute,
	}

	test := NewContinuousMemoryStabilityTest(config)

	// 启动测试
	err := test.Start(t)
	require.NoError(t, err, "Continuous memory stability test should complete without errors")

	// 验证测试结果
	metrics := test.metrics

	// 验证内存稳定性
	assert.LessOrEqual(t, metrics.LeakDetections, int64(5),
		"Memory leak detections should be minimal (≤5)")

	memoryGrowth := metrics.MaxMemoryUsageMB - metrics.MinMemoryUsageMB
	assert.LessOrEqual(t, memoryGrowth, int64(300),
		"Memory usage variation should be reasonable (≤300MB)")

	// 验证协程稳定性
	assert.LessOrEqual(t, metrics.GoroutineLeaks, int64(10),
		"Goroutine leaks should be minimal (≤10)")

	// 验证分配/释放平衡
	if metrics.TotalAllocations > 0 {
		deallocationRate := float64(metrics.TotalDeallocations) / float64(metrics.TotalAllocations)
		assert.GreaterOrEqual(t, deallocationRate, 0.8,
			"Deallocation rate should be reasonable (≥80%)")
	}

	// 验证GC效率
	assert.Greater(t, metrics.GCCycles, int64(0),
		"GC should run during the test")

	// 输出最终报告
	t.Logf("Continuous memory stability test completed:\n%s", test.GetFinalReport())
}
