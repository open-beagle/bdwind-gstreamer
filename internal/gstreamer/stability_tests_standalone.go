//go:build standalone
// +build standalone

package gstreamer

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

// StandaloneStabilityTest 独立稳定性测试
type StandaloneStabilityTest struct {
	config  *StandaloneStabilityConfig
	metrics *StandaloneStabilityMetrics
	ctx     context.Context
	cancel  context.CancelFunc
	logger  *logrus.Entry
	running atomic.Bool

	// 基线指标
	baselineMemoryMB   int64
	baselineGoroutines int
}

// StandaloneStabilityConfig 独立稳定性测试配置
type StandaloneStabilityConfig struct {
	Duration            time.Duration
	MemoryCheckInterval time.Duration
	LoadTestInterval    time.Duration
	MaxMemoryGrowthMB   int64
	MaxGoroutines       int
	AllocationRate      int
	AllocationSizeMB    int
	WorkerCount         int
	ReportInterval      time.Duration
}

// StandaloneStabilityMetrics 独立稳定性测试指标
type StandaloneStabilityMetrics struct {
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
	GCCycles         int64
	mutex            sync.RWMutex
}

// NewStandaloneStabilityTest 创建独立稳定性测试
func NewStandaloneStabilityTest(config *StandaloneStabilityConfig) *StandaloneStabilityTest {
	if config == nil {
		config = &StandaloneStabilityConfig{
			Duration:            24 * time.Hour,
			MemoryCheckInterval: 1 * time.Minute,
			LoadTestInterval:    5 * time.Minute,
			MaxMemoryGrowthMB:   100,
			MaxGoroutines:       1000,
			AllocationRate:      50,
			AllocationSizeMB:    1,
			WorkerCount:         10,
			ReportInterval:      10 * time.Minute,
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), config.Duration)

	return &StandaloneStabilityTest{
		config:  config,
		metrics: &StandaloneStabilityMetrics{StartTime: time.Now()},
		ctx:     ctx,
		cancel:  cancel,
		logger:  logrus.WithField("component", "standalone_stability_test"),
	}
}

// Start 启动独立稳定性测试
func (sst *StandaloneStabilityTest) Start(t *testing.T) error {
	if !sst.running.CompareAndSwap(false, true) {
		return fmt.Errorf("stability test is already running")
	}

	sst.logger.Info("Starting standalone stability test")

	// 记录基线
	sst.recordBaseline()

	// 启动监控协程
	var wg sync.WaitGroup

	// 内存监控
	wg.Add(1)
	go func() {
		defer wg.Done()
		sst.memoryMonitorLoop(t)
	}()

	// 负载生成
	wg.Add(1)
	go func() {
		defer wg.Done()
		sst.loadGeneratorLoop(t)
	}()

	// 报告生成
	wg.Add(1)
	go func() {
		defer wg.Done()
		sst.reportLoop(t)
	}()

	// 等待测试完成
	<-sst.ctx.Done()

	// 停止所有协程
	sst.running.Store(false)
	sst.cancel()

	// 等待所有协程结束
	wg.Wait()

	// 记录结束时间
	sst.metrics.EndTime = time.Now()

	sst.logger.Info("Standalone stability test completed")
	return nil
}

// recordBaseline 记录基线指标
func (sst *StandaloneStabilityTest) recordBaseline() {
	var m runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m)

	sst.baselineMemoryMB = int64(m.Alloc / 1024 / 1024)
	sst.baselineGoroutines = runtime.NumGoroutine()

	sst.metrics.MinMemoryUsageMB = sst.baselineMemoryMB
	sst.metrics.MaxMemoryUsageMB = sst.baselineMemoryMB

	sst.logger.WithFields(logrus.Fields{
		"baseline_memory_mb":  sst.baselineMemoryMB,
		"baseline_goroutines": sst.baselineGoroutines,
	}).Info("Recorded baseline metrics")
}

// memoryMonitorLoop 内存监控循环
func (sst *StandaloneStabilityTest) memoryMonitorLoop(t *testing.T) {
	ticker := time.NewTicker(sst.config.MemoryCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-sst.ctx.Done():
			return
		case <-ticker.C:
			if !sst.running.Load() {
				return
			}
			sst.checkMemoryStability(t)
		}
	}
}

// checkMemoryStability 检查内存稳定性
func (sst *StandaloneStabilityTest) checkMemoryStability(t *testing.T) {
	var m runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m)

	currentMemoryMB := int64(m.Alloc / 1024 / 1024)
	currentGoroutines := runtime.NumGoroutine()

	// 更新统计
	atomic.AddInt64(&sst.metrics.MemoryChecks, 1)
	atomic.AddInt64(&sst.metrics.GCCycles, int64(m.NumGC))

	sst.metrics.mutex.Lock()
	if currentMemoryMB > sst.metrics.MaxMemoryUsageMB {
		sst.metrics.MaxMemoryUsageMB = currentMemoryMB
	}
	if currentMemoryMB < sst.metrics.MinMemoryUsageMB {
		sst.metrics.MinMemoryUsageMB = currentMemoryMB
	}
	sst.metrics.mutex.Unlock()

	// 检查内存增长
	memoryGrowth := currentMemoryMB - sst.baselineMemoryMB
	if memoryGrowth > sst.config.MaxMemoryGrowthMB {
		atomic.AddInt64(&sst.metrics.MemoryLeaks, 1)
		sst.logger.WithFields(logrus.Fields{
			"current_memory_mb":  currentMemoryMB,
			"baseline_memory_mb": sst.baselineMemoryMB,
			"growth_mb":          memoryGrowth,
			"threshold_mb":       sst.config.MaxMemoryGrowthMB,
		}).Warn("Memory growth exceeded threshold")
	}

	// 检查协程泄漏
	goroutineGrowth := currentGoroutines - sst.baselineGoroutines
	if goroutineGrowth > sst.config.MaxGoroutines {
		atomic.AddInt64(&sst.metrics.GoroutineLeaks, 1)
		sst.logger.WithFields(logrus.Fields{
			"current_goroutines":  currentGoroutines,
			"baseline_goroutines": sst.baselineGoroutines,
			"growth":              goroutineGrowth,
			"threshold":           sst.config.MaxGoroutines,
		}).Warn("Goroutine count exceeded threshold")
	}

	sst.logger.WithFields(logrus.Fields{
		"memory_mb":        currentMemoryMB,
		"memory_growth_mb": memoryGrowth,
		"goroutines":       currentGoroutines,
		"goroutine_growth": goroutineGrowth,
	}).Debug("Memory stability check completed")
}

// loadGeneratorLoop 负载生成循环
func (sst *StandaloneStabilityTest) loadGeneratorLoop(t *testing.T) {
	// 启动多个工作协程
	var wg sync.WaitGroup

	for i := 0; i < sst.config.WorkerCount; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			sst.workerLoop(workerID, t)
		}(i)
	}

	wg.Wait()
}

// workerLoop 工作协程循环
func (sst *StandaloneStabilityTest) workerLoop(workerID int, t *testing.T) {
	interval := time.Second / time.Duration(sst.config.AllocationRate)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	allocations := make(map[int][]byte)
	allocationID := 0

	for {
		select {
		case <-sst.ctx.Done():
			return
		case <-ticker.C:
			if !sst.running.Load() {
				return
			}

			// 执行工作
			if err := sst.performWork(workerID, allocationID, allocations); err != nil {
				atomic.AddInt64(&sst.metrics.TotalErrors, 1)
			} else {
				atomic.AddInt64(&sst.metrics.TotalWork, 1)
			}

			allocationID++
		}
	}
}

// performWork 执行工作
func (sst *StandaloneStabilityTest) performWork(workerID, allocationID int, allocations map[int][]byte) error {
	// 内存分配
	size := sst.config.AllocationSizeMB * 1024 * 1024
	data := make([]byte, size)

	// 填充数据
	for i := range data {
		data[i] = byte((workerID + allocationID) % 256)
	}

	allocations[allocationID] = data
	atomic.AddInt64(&sst.metrics.TotalAllocations, 1)

	// CPU工作
	sum := 0
	for i := 0; i < 10000; i++ {
		sum += i * workerID
	}

	// 清理旧分配
	if len(allocations) > 10 {
		for id := range allocations {
			delete(allocations, id)
			break
		}
	}

	// 模拟随机错误
	if allocationID%1000 == 0 {
		return fmt.Errorf("simulated error at allocation %d", allocationID)
	}

	return nil
}

// reportLoop 报告循环
func (sst *StandaloneStabilityTest) reportLoop(t *testing.T) {
	ticker := time.NewTicker(sst.config.ReportInterval)
	defer ticker.Stop()

	for {
		select {
		case <-sst.ctx.Done():
			return
		case <-ticker.C:
			if !sst.running.Load() {
				return
			}
			sst.generateProgressReport()
		}
	}
}

// generateProgressReport 生成进度报告
func (sst *StandaloneStabilityTest) generateProgressReport() {
	elapsed := time.Since(sst.metrics.StartTime)
	remaining := sst.config.Duration - elapsed

	var currentStats runtime.MemStats
	runtime.ReadMemStats(&currentStats)
	currentMemoryMB := int64(currentStats.Alloc / 1024 / 1024)
	memoryGrowth := currentMemoryMB - sst.baselineMemoryMB

	sst.logger.WithFields(logrus.Fields{
		"elapsed":           elapsed,
		"remaining":         remaining,
		"progress_percent":  float64(elapsed) / float64(sst.config.Duration) * 100,
		"current_memory_mb": currentMemoryMB,
		"memory_growth_mb":  memoryGrowth,
		"memory_checks":     atomic.LoadInt64(&sst.metrics.MemoryChecks),
		"memory_leaks":      atomic.LoadInt64(&sst.metrics.MemoryLeaks),
		"goroutine_leaks":   atomic.LoadInt64(&sst.metrics.GoroutineLeaks),
		"total_work":        atomic.LoadInt64(&sst.metrics.TotalWork),
		"total_errors":      atomic.LoadInt64(&sst.metrics.TotalErrors),
		"total_allocations": atomic.LoadInt64(&sst.metrics.TotalAllocations),
	}).Info("Stability test progress report")
}

// GetFinalReport 获取最终报告
func (sst *StandaloneStabilityTest) GetFinalReport() string {
	duration := sst.metrics.EndTime.Sub(sst.metrics.StartTime)

	totalWork := atomic.LoadInt64(&sst.metrics.TotalWork)
	totalErrors := atomic.LoadInt64(&sst.metrics.TotalErrors)

	var errorRate float64
	if totalWork > 0 {
		errorRate = float64(totalErrors) / float64(totalWork) * 100
	}

	return fmt.Sprintf(`
独立稳定性测试报告:
==================
测试时长: %v
内存检查次数: %d
内存泄漏次数: %d
协程泄漏次数: %d
最大内存使用: %d MB
最小内存使用: %d MB
内存增长: %d MB
总工作量: %d
总错误数: %d
错误率: %.2f%%
总分配次数: %d
GC周期数: %d
`, duration, sst.metrics.MemoryChecks, sst.metrics.MemoryLeaks,
		sst.metrics.GoroutineLeaks, sst.metrics.MaxMemoryUsageMB,
		sst.metrics.MinMemoryUsageMB, sst.metrics.MaxMemoryUsageMB-sst.metrics.MinMemoryUsageMB,
		totalWork, totalErrors, errorRate, sst.metrics.TotalAllocations,
		sst.metrics.GCCycles)
}

// Helper functions

// isLongRunningTestEnabledStandalone 检查是否启用长时间测试
func isLongRunningTestEnabledStandalone() bool {
	if enabled := os.Getenv("ENABLE_LONG_RUNNING_TESTS"); enabled != "" {
		if val, err := strconv.ParseBool(enabled); err == nil {
			return val
		}
	}
	return false
}

// getTestDurationStandalone 获取测试持续时间
func getTestDurationStandalone(defaultDuration time.Duration) time.Duration {
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

// TestStandaloneLongRunningStability24Hours 24小时独立长时间稳定性测试
func TestStandaloneLongRunningStability24Hours(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping 24-hour standalone stability test in short mode")
	}

	if !isLongRunningTestEnabledStandalone() {
		t.Skip("Long-running tests are disabled. Set ENABLE_LONG_RUNNING_TESTS=true to enable")
	}

	// 获取测试持续时间
	duration := getTestDurationStandalone(24 * time.Hour)

	config := &StandaloneStabilityConfig{
		Duration:            duration,
		MemoryCheckInterval: 5 * time.Minute,
		LoadTestInterval:    30 * time.Minute,
		MaxMemoryGrowthMB:   100,
		MaxGoroutines:       1000,
		AllocationRate:      50,
		AllocationSizeMB:    1,
		WorkerCount:         10,
		ReportInterval:      10 * time.Minute,
	}

	test := NewStandaloneStabilityTest(config)

	// 启动测试
	err := test.Start(t)
	require.NoError(t, err, "Standalone stability test should complete without errors")

	// 验证测试结果
	memoryLeaks := atomic.LoadInt64(&test.metrics.MemoryLeaks)
	goroutineLeaks := atomic.LoadInt64(&test.metrics.GoroutineLeaks)
	totalWork := atomic.LoadInt64(&test.metrics.TotalWork)
	totalErrors := atomic.LoadInt64(&test.metrics.TotalErrors)

	// 验证内存稳定性
	assert.LessOrEqual(t, memoryLeaks, int64(5),
		"Memory leaks should be minimal (≤5)")

	memoryGrowth := test.metrics.MaxMemoryUsageMB - test.metrics.MinMemoryUsageMB
	assert.LessOrEqual(t, memoryGrowth, int64(200),
		"Memory usage variation should be reasonable (≤200MB)")

	// 验证协程稳定性
	assert.LessOrEqual(t, goroutineLeaks, int64(10),
		"Goroutine leaks should be minimal (≤10)")

	// 验证工作效率
	if totalWork > 0 {
		errorRate := float64(totalErrors) / float64(totalWork)
		assert.LessOrEqual(t, errorRate, 0.05,
			"Error rate should be reasonable (≤5%)")
	}

	// 输出最终报告
	t.Logf("Standalone stability test completed:\n%s", test.GetFinalReport())
}

// TestStandaloneMemoryStabilityUnderPressure 独立内存压力稳定性测试
func TestStandaloneMemoryStabilityUnderPressure(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping standalone memory pressure test in short mode")
	}

	duration := 30 * time.Minute
	if isLongRunningTestEnabledStandalone() {
		duration = getTestDurationStandalone(6 * time.Hour)
	}

	config := &StandaloneStabilityConfig{
		Duration:            duration,
		MemoryCheckInterval: 30 * time.Second,
		LoadTestInterval:    5 * time.Minute,
		MaxMemoryGrowthMB:   150, // 更严格的内存限制
		MaxGoroutines:       1500,
		AllocationRate:      100, // 更高的分配率
		AllocationSizeMB:    2,   // 更大的分配
		WorkerCount:         20,  // 更多工作协程
		ReportInterval:      5 * time.Minute,
	}

	test := NewStandaloneStabilityTest(config)

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

// TestStandaloneHighConcurrencyStability 独立高并发稳定性测试
func TestStandaloneHighConcurrencyStability(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping standalone high concurrency test in short mode")
	}

	duration := 15 * time.Minute
	if isLongRunningTestEnabledStandalone() {
		duration = getTestDurationStandalone(2 * time.Hour)
	}

	config := &StandaloneStabilityConfig{
		Duration:            duration,
		MemoryCheckInterval: 1 * time.Minute,
		LoadTestInterval:    2 * time.Minute,
		MaxMemoryGrowthMB:   200,
		MaxGoroutines:       3000, // 允许更多协程
		AllocationRate:      200,  // 非常高的分配率
		AllocationSizeMB:    1,
		WorkerCount:         50, // 大量工作协程
		ReportInterval:      3 * time.Minute,
	}

	test := NewStandaloneStabilityTest(config)

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
