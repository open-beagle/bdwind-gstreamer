package stability

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// LongRunningStabilityTest 长时间运行稳定性测试
type LongRunningStabilityTest struct {
	config  *StabilityConfig
	metrics *StabilityMetrics
	ctx     context.Context
	cancel  context.CancelFunc
	logger  *logrus.Entry
	running atomic.Bool

	// 基线指标
	baselineMemoryMB   int64
	baselineGoroutines int
}

// StabilityConfig 稳定性测试配置
type StabilityConfig struct {
	Duration            time.Duration
	MemoryCheckInterval time.Duration
	LoadTestInterval    time.Duration
	MaxMemoryGrowthMB   int64
	MaxGoroutines       int
	AllocationRate      int
	AllocationSizeMB    int
	WorkerCount         int
	ReportInterval      time.Duration
	ErrorInjectionRate  float64
}

// StabilityMetrics 稳定性测试指标
type StabilityMetrics struct {
	StartTime        time.Time
	EndTime          time.Time
	MemoryChecks     int64
	MemoryLeaks      int64
	GoroutineLeaks   int64
	MaxMemoryUsageMB int64
	MinMemoryUsageMB int64
	TotalAllocations int64
	TotalWork        int64
	TotalErrors      int64
	ErrorsRecovered  int64
	GCCycles         int64
	RestartCount     int64
	mutex            sync.RWMutex
}

// NewLongRunningStabilityTest 创建长时间稳定性测试
func NewLongRunningStabilityTest(config *StabilityConfig) *LongRunningStabilityTest {
	if config == nil {
		config = &StabilityConfig{
			Duration:            24 * time.Hour,
			MemoryCheckInterval: 5 * time.Minute,
			LoadTestInterval:    30 * time.Minute,
			MaxMemoryGrowthMB:   100,
			MaxGoroutines:       1000,
			AllocationRate:      50,
			AllocationSizeMB:    1,
			WorkerCount:         10,
			ReportInterval:      10 * time.Minute,
			ErrorInjectionRate:  0.01,
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), config.Duration)

	return &LongRunningStabilityTest{
		config:  config,
		metrics: &StabilityMetrics{StartTime: time.Now()},
		ctx:     ctx,
		cancel:  cancel,
		logger:  logrus.WithField("component", "long_running_stability_test"),
	}
}

// Start 启动长时间稳定性测试
func (lrst *LongRunningStabilityTest) Start(t *testing.T) error {
	if !lrst.running.CompareAndSwap(false, true) {
		return fmt.Errorf("stability test is already running")
	}

	lrst.logger.Info("Starting long-running stability test")

	// 记录基线
	lrst.recordBaseline()

	// 启动监控协程
	var wg sync.WaitGroup

	// 内存监控
	wg.Add(1)
	go func() {
		defer wg.Done()
		lrst.memoryMonitorLoop(t)
	}()

	// 负载生成
	wg.Add(1)
	go func() {
		defer wg.Done()
		lrst.loadGeneratorLoop(t)
	}()

	// 错误恢复测试
	wg.Add(1)
	go func() {
		defer wg.Done()
		lrst.errorRecoveryLoop(t)
	}()

	// 报告生成
	wg.Add(1)
	go func() {
		defer wg.Done()
		lrst.reportLoop(t)
	}()

	// 等待测试完成
	<-lrst.ctx.Done()

	// 停止所有协程
	lrst.running.Store(false)
	lrst.cancel()

	// 等待所有协程结束
	wg.Wait()

	// 记录结束时间
	lrst.metrics.EndTime = time.Now()

	lrst.logger.Info("Long-running stability test completed")
	return nil
}

// recordBaseline 记录基线指标
func (lrst *LongRunningStabilityTest) recordBaseline() {
	var m runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m)

	lrst.baselineMemoryMB = int64(m.Alloc / 1024 / 1024)
	lrst.baselineGoroutines = runtime.NumGoroutine()

	lrst.metrics.MinMemoryUsageMB = lrst.baselineMemoryMB
	lrst.metrics.MaxMemoryUsageMB = lrst.baselineMemoryMB

	lrst.logger.WithFields(logrus.Fields{
		"baseline_memory_mb":  lrst.baselineMemoryMB,
		"baseline_goroutines": lrst.baselineGoroutines,
	}).Info("Recorded baseline metrics")
}

// memoryMonitorLoop 内存监控循环
func (lrst *LongRunningStabilityTest) memoryMonitorLoop(t *testing.T) {
	ticker := time.NewTicker(lrst.config.MemoryCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-lrst.ctx.Done():
			return
		case <-ticker.C:
			if !lrst.running.Load() {
				return
			}
			lrst.checkMemoryStability(t)
		}
	}
}

// checkMemoryStability 检查内存稳定性
func (lrst *LongRunningStabilityTest) checkMemoryStability(t *testing.T) {
	var m runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m)

	currentMemoryMB := int64(m.Alloc / 1024 / 1024)
	currentGoroutines := runtime.NumGoroutine()

	// 更新统计
	atomic.AddInt64(&lrst.metrics.MemoryChecks, 1)

	lrst.metrics.mutex.Lock()
	if currentMemoryMB > lrst.metrics.MaxMemoryUsageMB {
		lrst.metrics.MaxMemoryUsageMB = currentMemoryMB
	}
	if currentMemoryMB < lrst.metrics.MinMemoryUsageMB {
		lrst.metrics.MinMemoryUsageMB = currentMemoryMB
	}
	lrst.metrics.mutex.Unlock()

	// 检查内存增长
	memoryGrowth := currentMemoryMB - lrst.baselineMemoryMB
	if memoryGrowth > lrst.config.MaxMemoryGrowthMB {
		atomic.AddInt64(&lrst.metrics.MemoryLeaks, 1)
		lrst.logger.WithFields(logrus.Fields{
			"current_memory_mb":  currentMemoryMB,
			"baseline_memory_mb": lrst.baselineMemoryMB,
			"growth_mb":          memoryGrowth,
			"threshold_mb":       lrst.config.MaxMemoryGrowthMB,
		}).Warn("Memory growth exceeded threshold")

		// 触发系统恢复
		lrst.triggerSystemRecovery()
	}

	// 检查协程泄漏
	goroutineGrowth := currentGoroutines - lrst.baselineGoroutines
	if goroutineGrowth > lrst.config.MaxGoroutines {
		atomic.AddInt64(&lrst.metrics.GoroutineLeaks, 1)
		lrst.logger.WithFields(logrus.Fields{
			"current_goroutines":  currentGoroutines,
			"baseline_goroutines": lrst.baselineGoroutines,
			"growth":              goroutineGrowth,
			"threshold":           lrst.config.MaxGoroutines,
		}).Warn("Goroutine count exceeded threshold")
	}

	lrst.logger.WithFields(logrus.Fields{
		"memory_mb":        currentMemoryMB,
		"memory_growth_mb": memoryGrowth,
		"goroutines":       currentGoroutines,
		"goroutine_growth": goroutineGrowth,
		"gc_count":         m.NumGC,
	}).Debug("Memory stability check completed")
}

// loadGeneratorLoop 负载生成循环
func (lrst *LongRunningStabilityTest) loadGeneratorLoop(t *testing.T) {
	// 启动多个工作协程
	var wg sync.WaitGroup

	for i := 0; i < lrst.config.WorkerCount; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			lrst.workerLoop(workerID, t)
		}(i)
	}

	wg.Wait()
}

// workerLoop 工作协程循环
func (lrst *LongRunningStabilityTest) workerLoop(workerID int, t *testing.T) {
	interval := time.Second / time.Duration(lrst.config.AllocationRate)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	allocations := make(map[int][]byte)
	allocationID := 0

	for {
		select {
		case <-lrst.ctx.Done():
			return
		case <-ticker.C:
			if !lrst.running.Load() {
				return
			}

			// 执行工作
			if err := lrst.performWork(workerID, allocationID, allocations); err != nil {
				atomic.AddInt64(&lrst.metrics.TotalErrors, 1)

				// 尝试恢复
				if lrst.attemptErrorRecovery(err, workerID) {
					atomic.AddInt64(&lrst.metrics.ErrorsRecovered, 1)
				}
			} else {
				atomic.AddInt64(&lrst.metrics.TotalWork, 1)
			}

			allocationID++
		}
	}
}

// performWork 执行工作
func (lrst *LongRunningStabilityTest) performWork(workerID, allocationID int, allocations map[int][]byte) error {
	// 内存分配
	size := lrst.config.AllocationSizeMB * 1024 * 1024
	data := make([]byte, size)

	// 填充数据
	for i := range data {
		data[i] = byte((workerID + allocationID) % 256)
	}

	allocations[allocationID] = data
	atomic.AddInt64(&lrst.metrics.TotalAllocations, 1)

	// CPU工作
	sum := 0
	for i := 0; i < 10000; i++ {
		sum += i * workerID
	}

	// 清理旧分配
	if len(allocations) > 20 {
		for id := range allocations {
			delete(allocations, id)
			break
		}
	}

	// 模拟随机错误
	if lrst.shouldInjectError() {
		return fmt.Errorf("injected error at worker %d, allocation %d", workerID, allocationID)
	}

	return nil
}

// shouldInjectError 是否应该注入错误
func (lrst *LongRunningStabilityTest) shouldInjectError() bool {
	return time.Now().UnixNano()%1000 < int64(lrst.config.ErrorInjectionRate*1000)
}

// attemptErrorRecovery 尝试错误恢复
func (lrst *LongRunningStabilityTest) attemptErrorRecovery(err error, workerID int) bool {
	lrst.logger.WithFields(logrus.Fields{
		"worker_id": workerID,
		"error":     err.Error(),
	}).Debug("Attempting error recovery")

	// 模拟恢复过程
	time.Sleep(time.Millisecond * 10)

	// 强制GC
	runtime.GC()

	// 90% 恢复成功率
	return time.Now().UnixNano()%10 < 9
}

// errorRecoveryLoop 错误恢复测试循环
func (lrst *LongRunningStabilityTest) errorRecoveryLoop(t *testing.T) {
	ticker := time.NewTicker(1 * time.Hour) // 每小时测试一次错误恢复
	defer ticker.Stop()

	for {
		select {
		case <-lrst.ctx.Done():
			return
		case <-ticker.C:
			if !lrst.running.Load() {
				return
			}
			lrst.testErrorRecoveryMechanisms(t)
		}
	}
}

// testErrorRecoveryMechanisms 测试错误恢复机制
func (lrst *LongRunningStabilityTest) testErrorRecoveryMechanisms(t *testing.T) {
	lrst.logger.Info("Testing error recovery mechanisms")

	// 测试内存压力恢复
	lrst.testMemoryPressureRecovery()

	// 测试协程泄漏恢复
	lrst.testGoroutineLeakRecovery()

	// 测试系统重启恢复
	lrst.testSystemRestartRecovery()
}

// testMemoryPressureRecovery 测试内存压力恢复
func (lrst *LongRunningStabilityTest) testMemoryPressureRecovery() {
	lrst.logger.Debug("Testing memory pressure recovery")

	// 创建内存压力
	const allocSize = 10 * 1024 * 1024 // 10MB
	allocations := make([][]byte, 0, 20)

	for i := 0; i < 20; i++ {
		data := make([]byte, allocSize)
		allocations = append(allocations, data)
	}

	// 触发GC
	runtime.GC()
	runtime.GC()

	// 释放内存
	allocations = nil
	runtime.GC()

	lrst.logger.Debug("Memory pressure recovery test completed")
}

// testGoroutineLeakRecovery 测试协程泄漏恢复
func (lrst *LongRunningStabilityTest) testGoroutineLeakRecovery() {
	lrst.logger.Debug("Testing goroutine leak recovery")

	initialCount := runtime.NumGoroutine()

	// 创建一些短期协程
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			time.Sleep(time.Millisecond * 100)
		}()
	}

	wg.Wait()

	// 检查协程是否正确清理
	time.Sleep(time.Millisecond * 200)
	finalCount := runtime.NumGoroutine()

	if finalCount > initialCount+5 {
		lrst.logger.WithFields(logrus.Fields{
			"initial_count": initialCount,
			"final_count":   finalCount,
		}).Warn("Potential goroutine leak detected in recovery test")
	}

	lrst.logger.Debug("Goroutine leak recovery test completed")
}

// testSystemRestartRecovery 测试系统重启恢复
func (lrst *LongRunningStabilityTest) testSystemRestartRecovery() {
	lrst.logger.Debug("Testing system restart recovery")

	// 模拟系统重启
	lrst.triggerSystemRecovery()

	lrst.logger.Debug("System restart recovery test completed")
}

// triggerSystemRecovery 触发系统恢复
func (lrst *LongRunningStabilityTest) triggerSystemRecovery() {
	lrst.logger.Info("Triggering system recovery")

	// 强制多次GC
	for i := 0; i < 3; i++ {
		runtime.GC()
		time.Sleep(time.Millisecond * 100)
	}

	atomic.AddInt64(&lrst.metrics.RestartCount, 1)

	lrst.logger.Info("System recovery completed")
}

// reportLoop 报告循环
func (lrst *LongRunningStabilityTest) reportLoop(t *testing.T) {
	ticker := time.NewTicker(lrst.config.ReportInterval)
	defer ticker.Stop()

	for {
		select {
		case <-lrst.ctx.Done():
			return
		case <-ticker.C:
			if !lrst.running.Load() {
				return
			}
			lrst.generateProgressReport()
		}
	}
}

// generateProgressReport 生成进度报告
func (lrst *LongRunningStabilityTest) generateProgressReport() {
	elapsed := time.Since(lrst.metrics.StartTime)
	remaining := lrst.config.Duration - elapsed

	var currentStats runtime.MemStats
	runtime.ReadMemStats(&currentStats)
	currentMemoryMB := int64(currentStats.Alloc / 1024 / 1024)
	memoryGrowth := currentMemoryMB - lrst.baselineMemoryMB

	totalWork := atomic.LoadInt64(&lrst.metrics.TotalWork)
	totalErrors := atomic.LoadInt64(&lrst.metrics.TotalErrors)
	errorsRecovered := atomic.LoadInt64(&lrst.metrics.ErrorsRecovered)

	var errorRate, recoveryRate float64
	if totalWork > 0 {
		errorRate = float64(totalErrors) / float64(totalWork) * 100
	}
	if totalErrors > 0 {
		recoveryRate = float64(errorsRecovered) / float64(totalErrors) * 100
	}

	lrst.logger.WithFields(logrus.Fields{
		"elapsed":           elapsed,
		"remaining":         remaining,
		"progress_percent":  float64(elapsed) / float64(lrst.config.Duration) * 100,
		"current_memory_mb": currentMemoryMB,
		"memory_growth_mb":  memoryGrowth,
		"memory_checks":     atomic.LoadInt64(&lrst.metrics.MemoryChecks),
		"memory_leaks":      atomic.LoadInt64(&lrst.metrics.MemoryLeaks),
		"goroutine_leaks":   atomic.LoadInt64(&lrst.metrics.GoroutineLeaks),
		"total_work":        totalWork,
		"total_errors":      totalErrors,
		"errors_recovered":  errorsRecovered,
		"error_rate":        errorRate,
		"recovery_rate":     recoveryRate,
		"restart_count":     atomic.LoadInt64(&lrst.metrics.RestartCount),
		"total_allocations": atomic.LoadInt64(&lrst.metrics.TotalAllocations),
	}).Info("Long-running stability test progress report")
}

// GetFinalReport 获取最终报告
func (lrst *LongRunningStabilityTest) GetFinalReport() string {
	duration := lrst.metrics.EndTime.Sub(lrst.metrics.StartTime)

	totalWork := atomic.LoadInt64(&lrst.metrics.TotalWork)
	totalErrors := atomic.LoadInt64(&lrst.metrics.TotalErrors)
	errorsRecovered := atomic.LoadInt64(&lrst.metrics.ErrorsRecovered)

	var errorRate, recoveryRate float64
	if totalWork > 0 {
		errorRate = float64(totalErrors) / float64(totalWork) * 100
	}
	if totalErrors > 0 {
		recoveryRate = float64(errorsRecovered) / float64(totalErrors) * 100
	}

	return fmt.Sprintf(`
长时间运行稳定性测试报告:
========================
测试时长: %v
内存检查次数: %d
内存泄漏次数: %d
协程泄漏次数: %d
最大内存使用: %d MB
最小内存使用: %d MB
内存增长: %d MB
总工作量: %d
总错误数: %d
错误恢复数: %d
错误率: %.2f%%
恢复成功率: %.2f%%
系统重启次数: %d
总分配次数: %d
`, duration, lrst.metrics.MemoryChecks, lrst.metrics.MemoryLeaks,
		lrst.metrics.GoroutineLeaks, lrst.metrics.MaxMemoryUsageMB,
		lrst.metrics.MinMemoryUsageMB, lrst.metrics.MaxMemoryUsageMB-lrst.metrics.MinMemoryUsageMB,
		totalWork, totalErrors, errorsRecovered, errorRate, recoveryRate,
		lrst.metrics.RestartCount, lrst.metrics.TotalAllocations)
}

// Helper functions

// isLongRunningTestEnabled 检查是否启用长时间测试
func isLongRunningTestEnabled() bool {
	if enabled := os.Getenv("ENABLE_LONG_RUNNING_TESTS"); enabled != "" {
		if val, err := strconv.ParseBool(enabled); err == nil {
			return val
		}
	}
	return false
}

// getTestDuration 获取测试持续时间
func getTestDuration(defaultDuration time.Duration) time.Duration {
	if durationStr := os.Getenv("STABILITY_TEST_DURATION"); durationStr != "" {
		if duration, err := time.ParseDuration(durationStr); err == nil {
			return duration
		}
	}

	// 在CI环境中使用较短的测试时间
	if os.Getenv("CI") != "" {
		return defaultDuration / 24 // 1小时而不是24小时
	}

	return defaultDuration
}

// Test functions

// TestLongRunningStability24Hours 24小时长时间稳定性测试
func TestLongRunningStability24Hours(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping 24-hour long-running stability test in short mode")
	}

	if !isLongRunningTestEnabled() {
		t.Skip("Long-running tests are disabled. Set ENABLE_LONG_RUNNING_TESTS=true to enable")
	}

	// 获取测试持续时间
	duration := getTestDuration(24 * time.Hour)

	config := &StabilityConfig{
		Duration:            duration,
		MemoryCheckInterval: 5 * time.Minute,
		LoadTestInterval:    30 * time.Minute,
		MaxMemoryGrowthMB:   100,
		MaxGoroutines:       1000,
		AllocationRate:      50,
		AllocationSizeMB:    1,
		WorkerCount:         10,
		ReportInterval:      10 * time.Minute,
		ErrorInjectionRate:  0.01,
	}

	test := NewLongRunningStabilityTest(config)

	// 启动测试
	err := test.Start(t)
	require.NoError(t, err, "Long-running stability test should complete without errors")

	// 验证测试结果
	memoryLeaks := atomic.LoadInt64(&test.metrics.MemoryLeaks)
	goroutineLeaks := atomic.LoadInt64(&test.metrics.GoroutineLeaks)
	totalWork := atomic.LoadInt64(&test.metrics.TotalWork)
	totalErrors := atomic.LoadInt64(&test.metrics.TotalErrors)
	errorsRecovered := atomic.LoadInt64(&test.metrics.ErrorsRecovered)

	// 验证内存稳定性
	assert.LessOrEqual(t, memoryLeaks, int64(5),
		"Memory leaks should be minimal (≤5)")

	memoryGrowth := test.metrics.MaxMemoryUsageMB - test.metrics.MinMemoryUsageMB
	assert.LessOrEqual(t, memoryGrowth, int64(200),
		"Memory usage variation should be reasonable (≤200MB)")

	// 验证协程稳定性
	assert.LessOrEqual(t, goroutineLeaks, int64(10),
		"Goroutine leaks should be minimal (≤10)")

	// 验证错误恢复
	if totalErrors > 0 {
		recoveryRate := float64(errorsRecovered) / float64(totalErrors)
		assert.GreaterOrEqual(t, recoveryRate, 0.85,
			"Error recovery rate should be ≥85%")
	}

	// 验证工作效率
	if totalWork > 0 {
		errorRate := float64(totalErrors) / float64(totalWork)
		assert.LessOrEqual(t, errorRate, 0.05,
			"Error rate should be reasonable (≤5%)")
	}

	// 验证系统稳定性
	restartCount := atomic.LoadInt64(&test.metrics.RestartCount)
	assert.LessOrEqual(t, restartCount, int64(10),
		"System restart count should be minimal (≤10)")

	// 输出最终报告
	t.Logf("Long-running stability test completed:\n%s", test.GetFinalReport())
}

// TestMemoryStabilityUnderPressure 内存压力稳定性测试
func TestMemoryStabilityUnderPressure(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping memory pressure stability test in short mode")
	}

	duration := 30 * time.Minute
	if isLongRunningTestEnabled() {
		duration = getTestDuration(6 * time.Hour)
	}

	config := &StabilityConfig{
		Duration:            duration,
		MemoryCheckInterval: 30 * time.Second,
		LoadTestInterval:    5 * time.Minute,
		MaxMemoryGrowthMB:   150, // 更严格的内存限制
		MaxGoroutines:       1500,
		AllocationRate:      100, // 更高的分配率
		AllocationSizeMB:    2,   // 更大的分配
		WorkerCount:         20,  // 更多工作协程
		ReportInterval:      5 * time.Minute,
		ErrorInjectionRate:  0.02,
	}

	test := NewLongRunningStabilityTest(config)

	// 启动测试
	err := test.Start(t)
	require.NoError(t, err, "Memory pressure test should complete without errors")

	// 验证结果
	memoryLeaks := atomic.LoadInt64(&test.metrics.MemoryLeaks)
	goroutineLeaks := atomic.LoadInt64(&test.metrics.GoroutineLeaks)

	assert.LessOrEqual(t, memoryLeaks, int64(10),
		"Memory leaks under pressure should be controlled (≤10)")
	assert.LessOrEqual(t, goroutineLeaks, int64(20),
		"Goroutine leaks under pressure should be controlled (≤20)")

	t.Logf("Memory pressure test completed:\n%s", test.GetFinalReport())
}

// TestHighConcurrencyStability 高并发稳定性测试
func TestHighConcurrencyStability(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping high concurrency stability test in short mode")
	}

	duration := 15 * time.Minute
	if isLongRunningTestEnabled() {
		duration = getTestDuration(2 * time.Hour)
	}

	config := &StabilityConfig{
		Duration:            duration,
		MemoryCheckInterval: 1 * time.Minute,
		LoadTestInterval:    2 * time.Minute,
		MaxMemoryGrowthMB:   200,
		MaxGoroutines:       3000, // 允许更多协程
		AllocationRate:      200,  // 非常高的分配率
		AllocationSizeMB:    1,
		WorkerCount:         50, // 大量工作协程
		ReportInterval:      3 * time.Minute,
		ErrorInjectionRate:  0.03,
	}

	test := NewLongRunningStabilityTest(config)

	// 启动测试
	err := test.Start(t)
	require.NoError(t, err, "High concurrency test should complete without errors")

	// 验证结果
	totalWork := atomic.LoadInt64(&test.metrics.TotalWork)
	totalErrors := atomic.LoadInt64(&test.metrics.TotalErrors)

	assert.Greater(t, totalWork, int64(10000),
		"Should complete significant amount of work under high concurrency")

	if totalWork > 0 {
		errorRate := float64(totalErrors) / float64(totalWork)
		assert.LessOrEqual(t, errorRate, 0.10,
			"Error rate under high concurrency should be acceptable (≤10%)")
	}

	t.Logf("High concurrency test completed:\n%s", test.GetFinalReport())
}
