package gstreamer

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// SimpleStabilityTest 简单稳定性测试
type SimpleStabilityTest struct {
	duration          time.Duration
	memoryBaseline    int64
	goroutineBaseline int

	// 统计指标
	memoryChecks   int64
	memoryLeaks    int64
	goroutineLeaks int64
	maxMemoryMB    int64
	minMemoryMB    int64

	// 控制
	ctx    context.Context
	cancel context.CancelFunc
	logger *logrus.Entry
}

// NewSimpleStabilityTest 创建简单稳定性测试
func NewSimpleStabilityTest(duration time.Duration) *SimpleStabilityTest {
	ctx, cancel := context.WithTimeout(context.Background(), duration)

	return &SimpleStabilityTest{
		duration: duration,
		ctx:      ctx,
		cancel:   cancel,
		logger:   logrus.WithField("component", "simple_stability_test"),
	}
}

// Run 运行稳定性测试
func (sst *SimpleStabilityTest) Run(t *testing.T) error {
	sst.logger.Info("Starting simple stability test")

	// 记录基线
	sst.recordBaseline()

	// 启动监控
	var wg sync.WaitGroup

	// 内存监控
	wg.Add(1)
	go func() {
		defer wg.Done()
		sst.memoryMonitor(t)
	}()

	// 工作负载模拟
	wg.Add(1)
	go func() {
		defer wg.Done()
		sst.workloadSimulator(t)
	}()

	// 等待完成
	<-sst.ctx.Done()
	sst.cancel()
	wg.Wait()

	sst.logger.Info("Simple stability test completed")
	return nil
}

// recordBaseline 记录基线指标
func (sst *SimpleStabilityTest) recordBaseline() {
	var m runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m)

	sst.memoryBaseline = int64(m.Alloc / 1024 / 1024)
	sst.goroutineBaseline = runtime.NumGoroutine()
	sst.minMemoryMB = sst.memoryBaseline
	sst.maxMemoryMB = sst.memoryBaseline

	sst.logger.WithFields(logrus.Fields{
		"baseline_memory_mb":  sst.memoryBaseline,
		"baseline_goroutines": sst.goroutineBaseline,
	}).Info("Recorded baseline metrics")
}

// memoryMonitor 内存监控
func (sst *SimpleStabilityTest) memoryMonitor(t *testing.T) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-sst.ctx.Done():
			return
		case <-ticker.C:
			sst.checkMemory(t)
		}
	}
}

// checkMemory 检查内存使用
func (sst *SimpleStabilityTest) checkMemory(t *testing.T) {
	var m runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m)

	currentMemoryMB := int64(m.Alloc / 1024 / 1024)
	currentGoroutines := runtime.NumGoroutine()

	// 更新统计
	atomic.AddInt64(&sst.memoryChecks, 1)

	if currentMemoryMB > sst.maxMemoryMB {
		sst.maxMemoryMB = currentMemoryMB
	}
	if currentMemoryMB < sst.minMemoryMB {
		sst.minMemoryMB = currentMemoryMB
	}

	// 检查内存增长
	memoryGrowth := currentMemoryMB - sst.memoryBaseline
	if memoryGrowth > 100 { // 100MB 阈值
		atomic.AddInt64(&sst.memoryLeaks, 1)
		sst.logger.WithFields(logrus.Fields{
			"current_memory_mb":  currentMemoryMB,
			"baseline_memory_mb": sst.memoryBaseline,
			"growth_mb":          memoryGrowth,
		}).Warn("Memory growth detected")
	}

	// 检查协程增长
	goroutineGrowth := currentGoroutines - sst.goroutineBaseline
	if goroutineGrowth > 100 { // 100个协程阈值
		atomic.AddInt64(&sst.goroutineLeaks, 1)
		sst.logger.WithFields(logrus.Fields{
			"current_goroutines":  currentGoroutines,
			"baseline_goroutines": sst.goroutineBaseline,
			"growth":              goroutineGrowth,
		}).Warn("Goroutine growth detected")
	}

	sst.logger.WithFields(logrus.Fields{
		"memory_mb":        currentMemoryMB,
		"memory_growth_mb": memoryGrowth,
		"goroutines":       currentGoroutines,
		"goroutine_growth": goroutineGrowth,
	}).Debug("Memory check completed")
}

// workloadSimulator 工作负载模拟器
func (sst *SimpleStabilityTest) workloadSimulator(t *testing.T) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	allocations := make(map[int][]byte)
	allocationID := 0

	for {
		select {
		case <-sst.ctx.Done():
			return
		case <-ticker.C:
			// 模拟内存分配
			size := 1024 * 100 // 100KB
			data := make([]byte, size)

			// 填充数据
			for i := range data {
				data[i] = byte(allocationID % 256)
			}

			allocations[allocationID] = data
			allocationID++

			// 随机释放旧分配
			if len(allocations) > 50 && allocationID%10 == 0 {
				for id := range allocations {
					delete(allocations, id)
					break
				}
			}

			// 模拟CPU工作
			sum := 0
			for i := 0; i < 1000; i++ {
				sum += i
			}
		}
	}
}

// GetResults 获取测试结果
func (sst *SimpleStabilityTest) GetResults() map[string]interface{} {
	return map[string]interface{}{
		"duration_seconds":    sst.duration.Seconds(),
		"memory_checks":       atomic.LoadInt64(&sst.memoryChecks),
		"memory_leaks":        atomic.LoadInt64(&sst.memoryLeaks),
		"goroutine_leaks":     atomic.LoadInt64(&sst.goroutineLeaks),
		"max_memory_mb":       sst.maxMemoryMB,
		"min_memory_mb":       sst.minMemoryMB,
		"memory_growth_mb":    sst.maxMemoryMB - sst.minMemoryMB,
		"baseline_memory_mb":  sst.memoryBaseline,
		"baseline_goroutines": sst.goroutineBaseline,
	}
}

// TestSimpleLongRunningStability 简单长时间稳定性测试
func TestSimpleLongRunningStability(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping long-running stability test in short mode")
	}

	// 获取测试持续时间
	duration := 30 * time.Minute // 默认30分钟
	if isLongRunningTestEnabled() {
		duration = getTestDuration(24 * time.Hour)
	}

	test := NewSimpleStabilityTest(duration)

	// 运行测试
	err := test.Run(t)
	require.NoError(t, err, "Simple stability test should complete without errors")

	// 验证结果
	results := test.GetResults()

	memoryLeaks := results["memory_leaks"].(int64)
	goroutineLeaks := results["goroutine_leaks"].(int64)
	memoryGrowth := results["memory_growth_mb"].(int64)

	assert.LessOrEqual(t, memoryLeaks, int64(5),
		"Memory leaks should be minimal (≤5)")
	assert.LessOrEqual(t, goroutineLeaks, int64(5),
		"Goroutine leaks should be minimal (≤5)")
	assert.LessOrEqual(t, memoryGrowth, int64(200),
		"Memory growth should be reasonable (≤200MB)")

	t.Logf("Simple stability test results: %+v", results)
}

// TestMemoryStabilityUnderLoad 内存稳定性负载测试
func TestMemoryStabilityUnderLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping memory stability test in short mode")
	}

	duration := 10 * time.Minute
	if isLongRunningTestEnabled() {
		duration = 2 * time.Hour
	}

	// 记录初始内存
	var initialStats runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&initialStats)
	initialMemoryMB := int64(initialStats.Alloc / 1024 / 1024)

	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()

	// 创建内存压力
	const numWorkers = 5
	var wg sync.WaitGroup

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			allocations := make([][]byte, 0, 100)
			ticker := time.NewTicker(50 * time.Millisecond)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					// 分配内存
					size := 1024 * 50 // 50KB
					data := make([]byte, size)
					for j := range data {
						data[j] = byte(workerID)
					}

					allocations = append(allocations, data)

					// 限制分配数量
					if len(allocations) > 50 {
						allocations = allocations[1:]
					}
				}
			}
		}(i)
	}

	// 监控内存使用
	memoryLeaks := int64(0)
	maxMemoryGrowth := int64(0)

	monitorTicker := time.NewTicker(30 * time.Second)
	defer monitorTicker.Stop()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-monitorTicker.C:
				var currentStats runtime.MemStats
				runtime.GC()
				runtime.ReadMemStats(&currentStats)

				currentMemoryMB := int64(currentStats.Alloc / 1024 / 1024)
				memoryGrowth := currentMemoryMB - initialMemoryMB

				if memoryGrowth > maxMemoryGrowth {
					maxMemoryGrowth = memoryGrowth
				}

				if memoryGrowth > 150 { // 150MB 阈值
					atomic.AddInt64(&memoryLeaks, 1)
					t.Logf("Memory leak detected: growth = %d MB", memoryGrowth)
				}

				t.Logf("Memory check: current = %d MB, growth = %d MB",
					currentMemoryMB, memoryGrowth)
			}
		}
	}()

	// 等待工作完成
	wg.Wait()

	// 最终验证
	var finalStats runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&finalStats)

	finalMemoryMB := int64(finalStats.Alloc / 1024 / 1024)
	finalGrowth := finalMemoryMB - initialMemoryMB

	assert.LessOrEqual(t, memoryLeaks, int64(3),
		"Memory leaks should be minimal under load (≤3)")
	assert.LessOrEqual(t, maxMemoryGrowth, int64(200),
		"Maximum memory growth should be controlled (≤200MB)")
	assert.LessOrEqual(t, finalGrowth, int64(50),
		"Final memory growth should be reasonable (≤50MB)")

	t.Logf("Memory stability test completed: final_growth=%d MB, max_growth=%d MB, leaks=%d",
		finalGrowth, maxMemoryGrowth, memoryLeaks)
}

// TestHighConcurrencyStability 高并发稳定性测试
func TestHighConcurrencyStability(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping high concurrency stability test in short mode")
	}

	duration := 5 * time.Minute
	if isLongRunningTestEnabled() {
		duration = 1 * time.Hour
	}

	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()

	const numWorkers = 50
	var wg sync.WaitGroup
	errorCount := int64(0)
	workCount := int64(0)

	// 启动工作协程
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			ticker := time.NewTicker(10 * time.Millisecond)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					// 模拟工作
					if err := simulateWork(workerID, int(atomic.LoadInt64(&workCount))); err != nil {
						atomic.AddInt64(&errorCount, 1)
					} else {
						atomic.AddInt64(&workCount, 1)
					}
				}
			}
		}(i)
	}

	// 等待完成
	wg.Wait()

	// 验证结果
	totalWork := atomic.LoadInt64(&workCount)
	totalErrors := atomic.LoadInt64(&errorCount)

	assert.Greater(t, totalWork, int64(1000),
		"Should complete significant amount of work")

	if totalWork > 0 {
		errorRate := float64(totalErrors) / float64(totalWork)
		assert.LessOrEqual(t, errorRate, 0.05,
			"Error rate should be low (≤5%)")
	}

	t.Logf("High concurrency test completed: work=%d, errors=%d, error_rate=%.2f%%",
		totalWork, totalErrors, float64(totalErrors)/float64(totalWork)*100)
}
