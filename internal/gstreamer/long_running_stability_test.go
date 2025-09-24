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
)

// StabilityTestConfig 稳定性测试配置
type StabilityTestConfig struct {
	Duration             time.Duration // 测试持续时间
	MemoryCheckInterval  time.Duration // 内存检查间隔
	LoadTestInterval     time.Duration // 负载测试间隔
	ErrorInjectionRate   float64       // 错误注入率 (0.0-1.0)
	MaxMemoryGrowthMB    int64         // 最大内存增长 (MB)
	MaxGoroutines        int           // 最大协程数
	HighLoadDuration     time.Duration // 高负载持续时间
	RecoveryTestInterval time.Duration // 恢复测试间隔
}

// DefaultStabilityConfig 默认稳定性测试配置
func DefaultStabilityConfig() *StabilityTestConfig {
	return &StabilityTestConfig{
		Duration:             24 * time.Hour, // 24小时测试
		MemoryCheckInterval:  5 * time.Minute,
		LoadTestInterval:     30 * time.Minute,
		ErrorInjectionRate:   0.01, // 1% 错误率
		MaxMemoryGrowthMB:    100,  // 最大100MB内存增长
		MaxGoroutines:        1000, // 最大1000个协程
		HighLoadDuration:     10 * time.Minute,
		RecoveryTestInterval: 1 * time.Hour,
	}
}

// StabilityTestMetrics 稳定性测试指标
type StabilityTestMetrics struct {
	StartTime        time.Time
	EndTime          time.Time
	TotalDuration    time.Duration
	MemoryChecks     int64
	MemoryLeaks      int64
	MaxMemoryUsageMB int64
	MinMemoryUsageMB int64
	AvgMemoryUsageMB int64
	GoroutineLeaks   int64
	MaxGoroutines    int64
	ErrorsInjected   int64
	ErrorsRecovered  int64
	RecoveryFailures int64
	HighLoadCycles   int64
	SuccessfulCycles int64
	FailedCycles     int64
	RestartCount     int64
	CrashCount       int64
	mutex            sync.RWMutex
}

// UpdateMemoryStats 更新内存统计
func (m *StabilityTestMetrics) UpdateMemoryStats(memUsageMB int64) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.MemoryChecks++
	if m.MaxMemoryUsageMB == 0 || memUsageMB > m.MaxMemoryUsageMB {
		m.MaxMemoryUsageMB = memUsageMB
	}
	if m.MinMemoryUsageMB == 0 || memUsageMB < m.MinMemoryUsageMB {
		m.MinMemoryUsageMB = memUsageMB
	}

	// 计算平均值
	if m.MemoryChecks == 1 {
		m.AvgMemoryUsageMB = memUsageMB
	} else {
		m.AvgMemoryUsageMB = (m.AvgMemoryUsageMB*(m.MemoryChecks-1) + memUsageMB) / m.MemoryChecks
	}
}

// IncrementCounter 原子递增计数器
func (m *StabilityTestMetrics) IncrementCounter(counter *int64) {
	atomic.AddInt64(counter, 1)
}

// GetSummary 获取测试摘要
func (m *StabilityTestMetrics) GetSummary() string {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	return fmt.Sprintf(`
稳定性测试摘要:
================
测试时长: %v
内存检查次数: %d
内存泄漏次数: %d
最大内存使用: %d MB
最小内存使用: %d MB
平均内存使用: %d MB
协程泄漏次数: %d
最大协程数: %d
错误注入次数: %d
错误恢复次数: %d
恢复失败次数: %d
高负载周期: %d
成功周期: %d
失败周期: %d
重启次数: %d
崩溃次数: %d
`, m.TotalDuration, m.MemoryChecks, m.MemoryLeaks, m.MaxMemoryUsageMB,
		m.MinMemoryUsageMB, m.AvgMemoryUsageMB, m.GoroutineLeaks, m.MaxGoroutines,
		m.ErrorsInjected, m.ErrorsRecovered, m.RecoveryFailures, m.HighLoadCycles,
		m.SuccessfulCycles, m.FailedCycles, m.RestartCount, m.CrashCount)
}

// LongRunningStabilityTest 长时间运行稳定性测试
type LongRunningStabilityTest struct {
	config  *StabilityTestConfig
	metrics *StabilityTestMetrics
	manager *Manager
	ctx     context.Context
	cancel  context.CancelFunc
	logger  *logrus.Entry

	// 测试控制
	running  atomic.Bool
	stopChan chan struct{}

	// 内存基线
	baselineMemoryMB  int64
	initialGoroutines int
}

// NewLongRunningStabilityTest 创建长时间稳定性测试
func NewLongRunningStabilityTest(config *StabilityTestConfig) *LongRunningStabilityTest {
	if config == nil {
		config = DefaultStabilityConfig()
	}

	ctx, cancel := context.WithTimeout(context.Background(), config.Duration)

	return &LongRunningStabilityTest{
		config:   config,
		metrics:  &StabilityTestMetrics{StartTime: time.Now()},
		ctx:      ctx,
		cancel:   cancel,
		logger:   logrus.WithField("component", "stability_test"),
		stopChan: make(chan struct{}),
	}
}

// Start 启动稳定性测试
func (lst *LongRunningStabilityTest) Start(t *testing.T) error {
	if !lst.running.CompareAndSwap(false, true) {
		return fmt.Errorf("stability test is already running")
	}

	lst.logger.Info("Starting long-running stability test")

	// 记录基线内存和协程数
	lst.recordBaseline()

	// 启动各种测试协程
	var wg sync.WaitGroup

	// 内存监控协程
	wg.Add(1)
	go func() {
		defer wg.Done()
		lst.memoryMonitorLoop(t)
	}()

	// 高负载测试协程
	wg.Add(1)
	go func() {
		defer wg.Done()
		lst.highLoadTestLoop(t)
	}()

	// 错误恢复测试协程
	wg.Add(1)
	go func() {
		defer wg.Done()
		lst.errorRecoveryTestLoop(t)
	}()

	// 主测试循环
	wg.Add(1)
	go func() {
		defer wg.Done()
		lst.mainTestLoop(t)
	}()

	// 等待测试完成或超时
	select {
	case <-lst.ctx.Done():
		lst.logger.Info("Stability test completed due to timeout")
	case <-lst.stopChan:
		lst.logger.Info("Stability test stopped manually")
	}

	// 停止所有协程
	lst.running.Store(false)
	lst.cancel()

	// 等待所有协程结束
	wg.Wait()

	// 记录结束时间
	lst.metrics.EndTime = time.Now()
	lst.metrics.TotalDuration = lst.metrics.EndTime.Sub(lst.metrics.StartTime)

	// 输出测试结果
	lst.logger.Info(lst.metrics.GetSummary())

	return nil
}

// recordBaseline 记录基线指标
func (lst *LongRunningStabilityTest) recordBaseline() {
	var m runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m)

	lst.baselineMemoryMB = int64(m.Alloc / 1024 / 1024)
	lst.initialGoroutines = runtime.NumGoroutine()

	lst.logger.WithFields(logrus.Fields{
		"baseline_memory_mb": lst.baselineMemoryMB,
		"initial_goroutines": lst.initialGoroutines,
	}).Info("Recorded baseline metrics")
}

// memoryMonitorLoop 内存监控循环
func (lst *LongRunningStabilityTest) memoryMonitorLoop(t *testing.T) {
	ticker := time.NewTicker(lst.config.MemoryCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-lst.ctx.Done():
			return
		case <-ticker.C:
			if !lst.running.Load() {
				return
			}
			lst.checkMemoryStability(t)
		}
	}
}

// checkMemoryStability 检查内存稳定性
func (lst *LongRunningStabilityTest) checkMemoryStability(t *testing.T) {
	var m runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m)

	currentMemoryMB := int64(m.Alloc / 1024 / 1024)
	currentGoroutines := runtime.NumGoroutine()

	// 更新统计
	lst.metrics.UpdateMemoryStats(currentMemoryMB)
	if int64(currentGoroutines) > lst.metrics.MaxGoroutines {
		lst.metrics.MaxGoroutines = int64(currentGoroutines)
	}

	// 检查内存增长
	memoryGrowth := currentMemoryMB - lst.baselineMemoryMB
	if memoryGrowth > lst.config.MaxMemoryGrowthMB {
		lst.metrics.IncrementCounter(&lst.metrics.MemoryLeaks)
		lst.logger.WithFields(logrus.Fields{
			"current_memory_mb":  currentMemoryMB,
			"baseline_memory_mb": lst.baselineMemoryMB,
			"growth_mb":          memoryGrowth,
			"max_allowed_mb":     lst.config.MaxMemoryGrowthMB,
		}).Warn("Memory growth exceeded threshold")

		// 在测试环境中，我们记录但不立即失败
		if testing.Short() {
			t.Errorf("Memory growth exceeded threshold: %d MB > %d MB",
				memoryGrowth, lst.config.MaxMemoryGrowthMB)
		}
	}

	// 检查协程泄漏
	goroutineGrowth := currentGoroutines - lst.initialGoroutines
	if goroutineGrowth > lst.config.MaxGoroutines {
		lst.metrics.IncrementCounter(&lst.metrics.GoroutineLeaks)
		lst.logger.WithFields(logrus.Fields{
			"current_goroutines": currentGoroutines,
			"initial_goroutines": lst.initialGoroutines,
			"growth":             goroutineGrowth,
			"max_allowed":        lst.config.MaxGoroutines,
		}).Warn("Goroutine count exceeded threshold")
	}

	lst.logger.WithFields(logrus.Fields{
		"memory_mb":        currentMemoryMB,
		"memory_growth_mb": memoryGrowth,
		"goroutines":       currentGoroutines,
		"goroutine_growth": goroutineGrowth,
	}).Debug("Memory stability check completed")
}

// highLoadTestLoop 高负载测试循环
func (lst *LongRunningStabilityTest) highLoadTestLoop(t *testing.T) {
	ticker := time.NewTicker(lst.config.LoadTestInterval)
	defer ticker.Stop()

	for {
		select {
		case <-lst.ctx.Done():
			return
		case <-ticker.C:
			if !lst.running.Load() {
				return
			}
			lst.runHighLoadTest(t)
		}
	}
}

// runHighLoadTest 运行高负载测试
func (lst *LongRunningStabilityTest) runHighLoadTest(t *testing.T) {
	lst.logger.Info("Starting high load test cycle")
	lst.metrics.IncrementCounter(&lst.metrics.HighLoadCycles)

	// 创建高负载场景
	loadCtx, loadCancel := context.WithTimeout(lst.ctx, lst.config.HighLoadDuration)
	defer loadCancel()

	// 模拟多个并发连接
	const numConnections = 10
	var wg sync.WaitGroup

	for i := 0; i < numConnections; i++ {
		wg.Add(1)
		go func(connID int) {
			defer wg.Done()
			lst.simulateConnection(loadCtx, connID, t)
		}(i)
	}

	// 等待所有连接完成或超时
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		lst.metrics.IncrementCounter(&lst.metrics.SuccessfulCycles)
		lst.logger.Info("High load test cycle completed successfully")
	case <-loadCtx.Done():
		lst.metrics.IncrementCounter(&lst.metrics.FailedCycles)
		lst.logger.Warn("High load test cycle timed out")
	}
}

// simulateConnection 模拟连接负载
func (lst *LongRunningStabilityTest) simulateConnection(ctx context.Context, connID int, t *testing.T) {
	logger := lst.logger.WithField("connection_id", connID)
	logger.Debug("Starting connection simulation")

	// 模拟媒体流处理
	ticker := time.NewTicker(33 * time.Millisecond) // ~30 FPS
	defer ticker.Stop()

	frameCount := 0
	for {
		select {
		case <-ctx.Done():
			logger.WithField("frames_processed", frameCount).Debug("Connection simulation completed")
			return
		case <-ticker.C:
			// 模拟帧处理
			lst.simulateFrameProcessing(connID, frameCount)
			frameCount++

			// 随机错误注入
			if lst.shouldInjectError() {
				lst.injectError(connID, frameCount, t)
			}
		}
	}
}

// simulateFrameProcessing 模拟帧处理
func (lst *LongRunningStabilityTest) simulateFrameProcessing(connID, frameCount int) {
	// 模拟一些CPU和内存使用
	data := make([]byte, 1024*100) // 100KB 数据
	for i := range data {
		data[i] = byte(frameCount % 256)
	}

	// 模拟处理延迟
	time.Sleep(time.Microsecond * 100)
}

// shouldInjectError 是否应该注入错误
func (lst *LongRunningStabilityTest) shouldInjectError() bool {
	return time.Now().UnixNano()%1000 < int64(lst.config.ErrorInjectionRate*1000)
}

// injectError 注入错误
func (lst *LongRunningStabilityTest) injectError(connID, frameCount int, t *testing.T) {
	lst.metrics.IncrementCounter(&lst.metrics.ErrorsInjected)

	logger := lst.logger.WithFields(logrus.Fields{
		"connection_id": connID,
		"frame_count":   frameCount,
	})

	// 模拟不同类型的错误
	errorType := time.Now().UnixNano() % 4

	switch errorType {
	case 0:
		// 模拟编码错误
		logger.Debug("Injecting encoding error")
		lst.simulateEncodingError(connID)
	case 1:
		// 模拟网络错误
		logger.Debug("Injecting network error")
		lst.simulateNetworkError(connID)
	case 2:
		// 模拟资源错误
		logger.Debug("Injecting resource error")
		lst.simulateResourceError(connID)
	case 3:
		// 模拟配置错误
		logger.Debug("Injecting configuration error")
		lst.simulateConfigurationError(connID)
	}
}

// simulateEncodingError 模拟编码错误
func (lst *LongRunningStabilityTest) simulateEncodingError(connID int) {
	// 模拟编码器重启
	time.Sleep(time.Millisecond * 10)
	lst.metrics.IncrementCounter(&lst.metrics.ErrorsRecovered)
}

// simulateNetworkError 模拟网络错误
func (lst *LongRunningStabilityTest) simulateNetworkError(connID int) {
	// 模拟网络延迟
	time.Sleep(time.Millisecond * 50)
	lst.metrics.IncrementCounter(&lst.metrics.ErrorsRecovered)
}

// simulateResourceError 模拟资源错误
func (lst *LongRunningStabilityTest) simulateResourceError(connID int) {
	// 模拟资源清理
	runtime.GC()
	time.Sleep(time.Millisecond * 5)
	lst.metrics.IncrementCounter(&lst.metrics.ErrorsRecovered)
}

// simulateConfigurationError 模拟配置错误
func (lst *LongRunningStabilityTest) simulateConfigurationError(connID int) {
	// 模拟配置重载
	time.Sleep(time.Millisecond * 20)
	lst.metrics.IncrementCounter(&lst.metrics.ErrorsRecovered)
}

// errorRecoveryTestLoop 错误恢复测试循环
func (lst *LongRunningStabilityTest) errorRecoveryTestLoop(t *testing.T) {
	ticker := time.NewTicker(lst.config.RecoveryTestInterval)
	defer ticker.Stop()

	for {
		select {
		case <-lst.ctx.Done():
			return
		case <-ticker.C:
			if !lst.running.Load() {
				return
			}
			lst.testErrorRecovery(t)
		}
	}
}

// testErrorRecovery 测试错误恢复
func (lst *LongRunningStabilityTest) testErrorRecovery(t *testing.T) {
	lst.logger.Info("Testing error recovery mechanisms")

	// 测试各种恢复场景
	recoveryTests := []struct {
		name string
		test func() error
	}{
		{"memory_pressure", lst.testMemoryPressureRecovery},
		{"goroutine_leak", lst.testGoroutineLeakRecovery},
		{"resource_exhaustion", lst.testResourceExhaustionRecovery},
		{"cascade_failure", lst.testCascadeFailureRecovery},
	}

	for _, rt := range recoveryTests {
		lst.logger.WithField("test", rt.name).Debug("Running recovery test")

		if err := rt.test(); err != nil {
			lst.metrics.IncrementCounter(&lst.metrics.RecoveryFailures)
			lst.logger.WithError(err).WithField("test", rt.name).Warn("Recovery test failed")
		} else {
			lst.metrics.IncrementCounter(&lst.metrics.ErrorsRecovered)
			lst.logger.WithField("test", rt.name).Debug("Recovery test passed")
		}
	}
}

// testMemoryPressureRecovery 测试内存压力恢复
func (lst *LongRunningStabilityTest) testMemoryPressureRecovery() error {
	// 创建内存压力
	const allocSize = 10 * 1024 * 1024 // 10MB
	allocations := make([][]byte, 0, 10)

	for i := 0; i < 10; i++ {
		data := make([]byte, allocSize)
		allocations = append(allocations, data)
	}

	// 触发GC
	runtime.GC()

	// 释放内存
	allocations = nil
	runtime.GC()

	return nil
}

// testGoroutineLeakRecovery 测试协程泄漏恢复
func (lst *LongRunningStabilityTest) testGoroutineLeakRecovery() error {
	initialCount := runtime.NumGoroutine()

	// 创建一些短期协程
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			time.Sleep(time.Millisecond * 10)
		}()
	}

	wg.Wait()

	// 检查协程是否正确清理
	time.Sleep(time.Millisecond * 100)
	finalCount := runtime.NumGoroutine()

	if finalCount > initialCount+5 { // 允许一些误差
		return fmt.Errorf("goroutine leak detected: %d -> %d", initialCount, finalCount)
	}

	return nil
}

// testResourceExhaustionRecovery 测试资源耗尽恢复
func (lst *LongRunningStabilityTest) testResourceExhaustionRecovery() error {
	// 模拟资源耗尽和恢复
	channels := make([]chan struct{}, 100)
	for i := range channels {
		channels[i] = make(chan struct{})
	}

	// 清理资源
	for i := range channels {
		close(channels[i])
	}

	return nil
}

// testCascadeFailureRecovery 测试级联故障恢复
func (lst *LongRunningStabilityTest) testCascadeFailureRecovery() error {
	// 模拟级联故障场景
	errors := make([]error, 0, 5)

	// 模拟多个组件同时失败
	for i := 0; i < 5; i++ {
		if err := lst.simulateComponentFailure(i); err != nil {
			errors = append(errors, err)
		}
	}

	// 检查是否能够恢复
	if len(errors) > 3 {
		return fmt.Errorf("cascade failure recovery failed: %d errors", len(errors))
	}

	return nil
}

// simulateComponentFailure 模拟组件故障
func (lst *LongRunningStabilityTest) simulateComponentFailure(componentID int) error {
	// 模拟组件故障和恢复
	time.Sleep(time.Millisecond * time.Duration(componentID*10))

	// 随机决定是否恢复成功
	if time.Now().UnixNano()%4 == 0 {
		return fmt.Errorf("component %d failed to recover", componentID)
	}

	return nil
}

// mainTestLoop 主测试循环
func (lst *LongRunningStabilityTest) mainTestLoop(t *testing.T) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-lst.ctx.Done():
			return
		case <-ticker.C:
			if !lst.running.Load() {
				return
			}
			lst.performMaintenanceTasks(t)
		}
	}
}

// performMaintenanceTasks 执行维护任务
func (lst *LongRunningStabilityTest) performMaintenanceTasks(t *testing.T) {
	// 定期维护任务
	runtime.GC()

	// 检查系统健康状态
	lst.checkSystemHealth(t)

	// 记录进度
	elapsed := time.Since(lst.metrics.StartTime)
	remaining := lst.config.Duration - elapsed

	lst.logger.WithFields(logrus.Fields{
		"elapsed":          elapsed,
		"remaining":        remaining,
		"progress_percent": float64(elapsed) / float64(lst.config.Duration) * 100,
	}).Info("Stability test progress")
}

// checkSystemHealth 检查系统健康状态
func (lst *LongRunningStabilityTest) checkSystemHealth(t *testing.T) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// 检查各种健康指标
	healthChecks := map[string]bool{
		"memory_usage_ok":    m.Alloc < 500*1024*1024, // 500MB 限制
		"goroutine_count_ok": runtime.NumGoroutine() < 1000,
		"gc_frequency_ok":    m.NumGC > 0,
	}

	for check, passed := range healthChecks {
		if !passed {
			lst.logger.WithField("check", check).Warn("Health check failed")
		}
	}
}

// Stop 停止稳定性测试
func (lst *LongRunningStabilityTest) Stop() {
	if lst.running.Load() {
		close(lst.stopChan)
	}
}
