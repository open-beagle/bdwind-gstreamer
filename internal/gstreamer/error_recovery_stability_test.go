package gstreamer

import (
	"context"
	"fmt"
	"math/rand"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ErrorRecoveryStabilityTest 错误恢复稳定性测试
type ErrorRecoveryStabilityTest struct {
	config  *ErrorRecoveryConfig
	metrics *ErrorRecoveryMetrics
	ctx     context.Context
	cancel  context.CancelFunc
	logger  *logrus.Entry
	running atomic.Bool

	// 组件管理
	errorManager  *ErrorManager
	stateManager  *StateManager
	memoryManager *MemoryManager

	// 错误注入
	errorInjector *ErrorInjector

	// 恢复策略
	recoveryStrategies map[ErrorType]*StabilityRecoveryStrategy

	// 监控通道
	errorEvents    chan StabilityErrorEvent
	recoveryEvents chan StabilityRecoveryEvent
}

// ErrorRecoveryConfig 错误恢复测试配置
type ErrorRecoveryConfig struct {
	TestDuration        time.Duration
	ErrorInjectionRate  float64       // 错误注入率 (0.0-1.0)
	RecoveryTimeout     time.Duration // 恢复超时时间
	MaxRecoveryAttempts int           // 最大恢复尝试次数
	ErrorTypes          []ErrorType   // 要测试的错误类型
	ComponentCount      int           // 模拟组件数量
	ConcurrentErrors    int           // 并发错误数量
	MonitorInterval     time.Duration // 监控间隔
	ReportInterval      time.Duration // 报告间隔
}

// DefaultErrorRecoveryConfig 默认错误恢复配置
func DefaultErrorRecoveryConfig() *ErrorRecoveryConfig {
	return &ErrorRecoveryConfig{
		TestDuration:        4 * time.Hour,
		ErrorInjectionRate:  0.02, // 2% 错误率
		RecoveryTimeout:     30 * time.Second,
		MaxRecoveryAttempts: 3,
		ErrorTypes: []ErrorType{
			ErrorTypeEncoding,
			ErrorTypeNetwork,
			ErrorTypeResource,
			ErrorTypeMemory,
		},
		ComponentCount:   10,
		ConcurrentErrors: 3,
		MonitorInterval:  10 * time.Second,
		ReportInterval:   10 * time.Minute,
	}
}

// ErrorRecoveryMetrics 错误恢复指标
type ErrorRecoveryMetrics struct {
	StartTime           time.Time
	EndTime             time.Time
	TotalErrors         int64
	RecoveredErrors     int64
	FailedRecoveries    int64
	RecoveryAttempts    int64
	AverageRecoveryTime time.Duration
	MaxRecoveryTime     time.Duration
	MinRecoveryTime     time.Duration

	// 按错误类型统计
	ErrorsByType     map[ErrorType]int64
	RecoveriesByType map[ErrorType]int64
	FailuresByType   map[ErrorType]int64

	// 恢复时间统计
	RecoveryTimes []time.Duration

	// 系统稳定性
	SystemRestarts    int64
	ComponentFailures int64
	CascadeFailures   int64

	mutex sync.RWMutex
}

// StabilityErrorEvent 稳定性测试错误事件
type StabilityErrorEvent struct {
	Timestamp   time.Time
	ErrorType   ErrorType
	ComponentID string
	ErrorMsg    string
	Severity    ErrorSeverity
}

// StabilityRecoveryEvent 稳定性测试恢复事件
type StabilityRecoveryEvent struct {
	Timestamp    time.Time
	ErrorType    ErrorType
	ComponentID  string
	RecoveryTime time.Duration
	Attempts     int
	Success      bool
	Strategy     string
}

// ErrorInjector 错误注入器
type ErrorInjector struct {
	config     *ErrorRecoveryConfig
	components []*MockComponent
	logger     *logrus.Entry
	rand       *rand.Rand
}

// MockComponent 模拟组件
type MockComponent struct {
	ID         string
	State      StabilityComponentState
	ErrorCount int64
	LastError  time.Time
	mutex      sync.RWMutex
}

// StabilityComponentState 稳定性测试组件状态
type StabilityComponentState int

const (
	StabilityStateRunning StabilityComponentState = iota
	StabilityStateError
	StabilityStateRecovering
	StabilityStateStopped
)

// NewErrorRecoveryStabilityTest 创建错误恢复稳定性测试
func NewErrorRecoveryStabilityTest(config *ErrorRecoveryConfig) *ErrorRecoveryStabilityTest {
	if config == nil {
		config = DefaultErrorRecoveryConfig()
	}

	ctx, cancel := context.WithTimeout(context.Background(), config.TestDuration)

	return &ErrorRecoveryStabilityTest{
		config:         config,
		metrics:        newErrorRecoveryMetrics(),
		ctx:            ctx,
		cancel:         cancel,
		logger:         logrus.WithField("component", "error_recovery_test"),
		errorEvents:    make(chan StabilityErrorEvent, 1000),
		recoveryEvents: make(chan StabilityRecoveryEvent, 1000),
	}
}

// newErrorRecoveryMetrics 创建错误恢复指标
func newErrorRecoveryMetrics() *ErrorRecoveryMetrics {
	return &ErrorRecoveryMetrics{
		StartTime:        time.Now(),
		ErrorsByType:     make(map[ErrorType]int64),
		RecoveriesByType: make(map[ErrorType]int64),
		FailuresByType:   make(map[ErrorType]int64),
		RecoveryTimes:    make([]time.Duration, 0),
		MinRecoveryTime:  time.Hour, // 初始化为一个大值
	}
}

// Start 启动错误恢复稳定性测试
func (erst *ErrorRecoveryStabilityTest) Start(t *testing.T) error {
	if !erst.running.CompareAndSwap(false, true) {
		return fmt.Errorf("error recovery test is already running")
	}

	erst.logger.Info("Starting error recovery stability test")

	// 初始化组件
	erst.initializeComponents()

	// 启动监控协程
	var wg sync.WaitGroup

	// 错误注入协程
	wg.Add(1)
	go func() {
		defer wg.Done()
		erst.errorInjectionLoop(t)
	}()

	// 错误监控协程
	wg.Add(1)
	go func() {
		defer wg.Done()
		erst.errorMonitorLoop(t)
	}()

	// 恢复处理协程
	wg.Add(1)
	go func() {
		defer wg.Done()
		erst.recoveryHandlerLoop(t)
	}()

	// 系统监控协程
	wg.Add(1)
	go func() {
		defer wg.Done()
		erst.systemMonitorLoop(t)
	}()

	// 报告协程
	wg.Add(1)
	go func() {
		defer wg.Done()
		erst.reportLoop(t)
	}()

	// 等待测试完成
	<-erst.ctx.Done()

	// 停止所有协程
	erst.running.Store(false)
	erst.cancel()

	// 等待所有协程结束
	wg.Wait()

	// 记录结束时间
	erst.metrics.EndTime = time.Now()

	erst.logger.Info("Error recovery stability test completed")
	return nil
}

// initializeComponents 初始化组件
func (erst *ErrorRecoveryStabilityTest) initializeComponents() {
	// 初始化管理器
	erst.errorManager = NewErrorManager()
	erst.stateManager = NewStateManager()
	erst.memoryManager = NewMemoryManager()

	// 初始化错误注入器
	erst.errorInjector = &ErrorInjector{
		config:     erst.config,
		components: make([]*MockComponent, erst.config.ComponentCount),
		logger:     erst.logger.WithField("subcomponent", "error_injector"),
		rand:       rand.New(rand.NewSource(time.Now().UnixNano())),
	}

	// 创建模拟组件
	for i := 0; i < erst.config.ComponentCount; i++ {
		erst.errorInjector.components[i] = &MockComponent{
			ID:    fmt.Sprintf("component_%d", i),
			State: StabilityStateRunning,
		}
	}

	// 初始化恢复策略
	erst.initializeRecoveryStrategies()

	erst.logger.WithField("component_count", erst.config.ComponentCount).Info("Components initialized")
}

// initializeRecoveryStrategies 初始化恢复策略
func (erst *ErrorRecoveryStabilityTest) initializeRecoveryStrategies() {
	erst.recoveryStrategies = map[ErrorType]*StabilityRecoveryStrategy{
		ErrorTypeEncoding: {
			MaxAttempts:   3,
			RetryDelay:    time.Second * 2,
			BackoffFactor: 1.5,
			Timeout:       time.Second * 30,
		},
		ErrorTypeNetwork: {
			MaxAttempts:   5,
			RetryDelay:    time.Second * 1,
			BackoffFactor: 2.0,
			Timeout:       time.Second * 60,
		},
		ErrorTypeResource: {
			MaxAttempts:   2,
			RetryDelay:    time.Second * 5,
			BackoffFactor: 1.0,
			Timeout:       time.Second * 20,
		},
		ErrorTypeMemory: {
			MaxAttempts:   1,
			RetryDelay:    time.Second * 1,
			BackoffFactor: 1.0,
			Timeout:       time.Second * 10,
		},
	}
}

// StabilityRecoveryStrategy 稳定性测试恢复策略
type StabilityRecoveryStrategy struct {
	MaxAttempts   int
	RetryDelay    time.Duration
	BackoffFactor float64
	Timeout       time.Duration
}

// errorInjectionLoop 错误注入循环
func (erst *ErrorRecoveryStabilityTest) errorInjectionLoop(t *testing.T) {
	// 计算注入间隔
	injectionInterval := time.Duration(float64(time.Second) / erst.config.ErrorInjectionRate)
	ticker := time.NewTicker(injectionInterval)
	defer ticker.Stop()

	for {
		select {
		case <-erst.ctx.Done():
			return
		case <-ticker.C:
			if !erst.running.Load() {
				return
			}
			erst.injectRandomError()
		}
	}
}

// injectRandomError 注入随机错误
func (erst *ErrorRecoveryStabilityTest) injectRandomError() {
	// 选择随机错误类型
	errorType := erst.config.ErrorTypes[erst.errorInjector.rand.Intn(len(erst.config.ErrorTypes))]

	// 选择随机组件
	componentIndex := erst.errorInjector.rand.Intn(len(erst.errorInjector.components))
	component := erst.errorInjector.components[componentIndex]

	// 检查组件状态
	component.mutex.Lock()
	if component.State != StabilityStateRunning {
		component.mutex.Unlock()
		return // 组件不在运行状态，跳过
	}

	component.State = StabilityStateError
	component.LastError = time.Now()
	atomic.AddInt64(&component.ErrorCount, 1)
	component.mutex.Unlock()

	// 创建错误事件
	errorEvent := StabilityErrorEvent{
		Timestamp:   time.Now(),
		ErrorType:   errorType,
		ComponentID: component.ID,
		ErrorMsg:    fmt.Sprintf("Injected %s error", errorType),
		Severity:    erst.getErrorSeverity(errorType),
	}

	// 发送错误事件
	select {
	case erst.errorEvents <- errorEvent:
	default:
		erst.logger.Warn("Error events channel is full")
	}

	// 更新统计
	atomic.AddInt64(&erst.metrics.TotalErrors, 1)
	erst.metrics.mutex.Lock()
	erst.metrics.ErrorsByType[errorType]++
	erst.metrics.mutex.Unlock()

	erst.logger.WithFields(logrus.Fields{
		"error_type":   errorType,
		"component_id": component.ID,
		"severity":     errorEvent.Severity,
	}).Debug("Error injected")
}

// getErrorSeverity 获取错误严重程度
func (erst *ErrorRecoveryStabilityTest) getErrorSeverity(errorType ErrorType) ErrorSeverity {
	switch errorType {
	case ErrorTypeMemory:
		return SeverityCritical
	case ErrorTypeResource:
		return SeverityHigh
	case ErrorTypeEncoding:
		return SeverityMedium
	case ErrorTypeNetwork:
		return SeverityLow
	default:
		return SeverityMedium
	}
}

// errorMonitorLoop 错误监控循环
func (erst *ErrorRecoveryStabilityTest) errorMonitorLoop(t *testing.T) {
	for {
		select {
		case <-erst.ctx.Done():
			return
		case errorEvent := <-erst.errorEvents:
			erst.handleErrorEvent(errorEvent, t)
		}
	}
}

// handleErrorEvent 处理错误事件
func (erst *ErrorRecoveryStabilityTest) handleErrorEvent(event StabilityErrorEvent, t *testing.T) {
	erst.logger.WithFields(logrus.Fields{
		"error_type":   event.ErrorType,
		"component_id": event.ComponentID,
		"severity":     event.Severity,
	}).Info("Handling error event")

	// 启动恢复过程
	go erst.attemptRecovery(event, t)
}

// attemptRecovery 尝试恢复
func (erst *ErrorRecoveryStabilityTest) attemptRecovery(errorEvent StabilityErrorEvent, t *testing.T) {
	startTime := time.Now()
	strategy := erst.recoveryStrategies[errorEvent.ErrorType]

	// 找到对应的组件
	var component *MockComponent
	for _, comp := range erst.errorInjector.components {
		if comp.ID == errorEvent.ComponentID {
			component = comp
			break
		}
	}

	if component == nil {
		erst.logger.WithField("component_id", errorEvent.ComponentID).Error("Component not found")
		return
	}

	// 设置组件为恢复状态
	component.mutex.Lock()
	component.State = StabilityStateRecovering
	component.mutex.Unlock()

	success := false
	attempts := 0

	for attempts < strategy.MaxAttempts {
		attempts++
		atomic.AddInt64(&erst.metrics.RecoveryAttempts, 1)

		erst.logger.WithFields(logrus.Fields{
			"component_id": component.ID,
			"error_type":   errorEvent.ErrorType,
			"attempt":      attempts,
		}).Debug("Recovery attempt")

		// 模拟恢复过程
		if erst.simulateRecovery(errorEvent.ErrorType, attempts) {
			success = true
			break
		}

		// 等待重试延迟
		if attempts < strategy.MaxAttempts {
			delay := time.Duration(float64(strategy.RetryDelay) *
				(1.0 + (strategy.BackoffFactor-1.0)*float64(attempts-1)))
			time.Sleep(delay)
		}
	}

	recoveryTime := time.Since(startTime)

	// 更新组件状态
	component.mutex.Lock()
	if success {
		component.State = StabilityStateRunning
	} else {
		component.State = StabilityStateStopped
	}
	component.mutex.Unlock()

	// 创建恢复事件
	recoveryEvent := StabilityRecoveryEvent{
		Timestamp:    time.Now(),
		ErrorType:    errorEvent.ErrorType,
		ComponentID:  errorEvent.ComponentID,
		RecoveryTime: recoveryTime,
		Attempts:     attempts,
		Success:      success,
		Strategy:     fmt.Sprintf("strategy_%s", errorEvent.ErrorType),
	}

	// 发送恢复事件
	select {
	case erst.recoveryEvents <- recoveryEvent:
	default:
		erst.logger.Warn("Recovery events channel is full")
	}

	// 更新统计
	if success {
		atomic.AddInt64(&erst.metrics.RecoveredErrors, 1)
		erst.metrics.mutex.Lock()
		erst.metrics.RecoveriesByType[errorEvent.ErrorType]++
		erst.updateRecoveryTimeStats(recoveryTime)
		erst.metrics.mutex.Unlock()
	} else {
		atomic.AddInt64(&erst.metrics.FailedRecoveries, 1)
		erst.metrics.mutex.Lock()
		erst.metrics.FailuresByType[errorEvent.ErrorType]++
		erst.metrics.mutex.Unlock()
	}

	erst.logger.WithFields(logrus.Fields{
		"component_id":  component.ID,
		"error_type":    errorEvent.ErrorType,
		"success":       success,
		"attempts":      attempts,
		"recovery_time": recoveryTime,
	}).Info("Recovery completed")
}

// simulateRecovery 模拟恢复过程
func (erst *ErrorRecoveryStabilityTest) simulateRecovery(errorType ErrorType, attempt int) bool {
	// 不同错误类型有不同的恢复成功率
	var successRate float64

	switch errorType {
	case ErrorTypeEncoding:
		// 编码错误通常在第2-3次尝试后恢复
		successRate = 0.3 + 0.4*float64(attempt-1)
	case ErrorTypeNetwork:
		// 网络错误恢复率较高
		successRate = 0.6 + 0.2*float64(attempt-1)
	case ErrorTypeResource:
		// 资源错误恢复率中等
		successRate = 0.5 + 0.3*float64(attempt-1)
	case ErrorTypeMemory:
		// 内存错误通常能立即恢复
		successRate = 0.9
	default:
		successRate = 0.5
	}

	// 限制成功率在合理范围内
	if successRate > 0.95 {
		successRate = 0.95
	}

	// 模拟恢复时间
	recoveryDelay := time.Duration(erst.errorInjector.rand.Intn(1000)) * time.Millisecond
	time.Sleep(recoveryDelay)

	// 根据成功率决定是否恢复成功
	return erst.errorInjector.rand.Float64() < successRate
}

// updateRecoveryTimeStats 更新恢复时间统计
func (erst *ErrorRecoveryStabilityTest) updateRecoveryTimeStats(recoveryTime time.Duration) {
	erst.metrics.RecoveryTimes = append(erst.metrics.RecoveryTimes, recoveryTime)

	// 更新最大最小值
	if recoveryTime > erst.metrics.MaxRecoveryTime {
		erst.metrics.MaxRecoveryTime = recoveryTime
	}
	if recoveryTime < erst.metrics.MinRecoveryTime {
		erst.metrics.MinRecoveryTime = recoveryTime
	}

	// 计算平均值
	total := time.Duration(0)
	for _, t := range erst.metrics.RecoveryTimes {
		total += t
	}
	erst.metrics.AverageRecoveryTime = total / time.Duration(len(erst.metrics.RecoveryTimes))
}

// recoveryHandlerLoop 恢复处理循环
func (erst *ErrorRecoveryStabilityTest) recoveryHandlerLoop(t *testing.T) {
	for {
		select {
		case <-erst.ctx.Done():
			return
		case recoveryEvent := <-erst.recoveryEvents:
			erst.handleRecoveryEvent(recoveryEvent, t)
		}
	}
}

// handleRecoveryEvent 处理恢复事件
func (erst *ErrorRecoveryStabilityTest) handleRecoveryEvent(event StabilityRecoveryEvent, t *testing.T) {
	erst.logger.WithFields(logrus.Fields{
		"component_id":  event.ComponentID,
		"error_type":    event.ErrorType,
		"success":       event.Success,
		"recovery_time": event.RecoveryTime,
		"attempts":      event.Attempts,
	}).Debug("Recovery event processed")

	// 检查是否需要系统级恢复
	if !event.Success && event.ErrorType == ErrorTypeMemory {
		erst.triggerSystemRecovery(t)
	}
}

// triggerSystemRecovery 触发系统级恢复
func (erst *ErrorRecoveryStabilityTest) triggerSystemRecovery(t *testing.T) {
	erst.logger.Warn("Triggering system-level recovery")

	// 强制GC
	runtime.GC()
	runtime.GC()

	// 重启失败的组件
	for _, component := range erst.errorInjector.components {
		component.mutex.Lock()
		if component.State == StabilityStateStopped {
			component.State = StabilityStateRunning
			atomic.AddInt64(&erst.metrics.SystemRestarts, 1)
		}
		component.mutex.Unlock()
	}

	erst.logger.Info("System-level recovery completed")
}

// systemMonitorLoop 系统监控循环
func (erst *ErrorRecoveryStabilityTest) systemMonitorLoop(t *testing.T) {
	ticker := time.NewTicker(erst.config.MonitorInterval)
	defer ticker.Stop()

	for {
		select {
		case <-erst.ctx.Done():
			return
		case <-ticker.C:
			if !erst.running.Load() {
				return
			}
			erst.monitorSystemHealth(t)
		}
	}
}

// monitorSystemHealth 监控系统健康状态
func (erst *ErrorRecoveryStabilityTest) monitorSystemHealth(t *testing.T) {
	runningComponents := 0
	errorComponents := 0
	stoppedComponents := 0

	for _, component := range erst.errorInjector.components {
		component.mutex.RLock()
		switch component.State {
		case StabilityStateRunning:
			runningComponents++
		case StabilityStateError, StabilityStateRecovering:
			errorComponents++
		case StabilityStateStopped:
			stoppedComponents++
		}
		component.mutex.RUnlock()
	}

	// 检查系统健康状态
	healthyRatio := float64(runningComponents) / float64(len(erst.errorInjector.components))

	if healthyRatio < 0.5 {
		erst.logger.WithFields(logrus.Fields{
			"running_components": runningComponents,
			"error_components":   errorComponents,
			"stopped_components": stoppedComponents,
			"healthy_ratio":      healthyRatio,
		}).Warn("System health degraded")

		// 触发级联故障恢复
		atomic.AddInt64(&erst.metrics.CascadeFailures, 1)
		erst.triggerSystemRecovery(t)
	}

	erst.logger.WithFields(logrus.Fields{
		"running":       runningComponents,
		"error":         errorComponents,
		"stopped":       stoppedComponents,
		"healthy_ratio": healthyRatio,
	}).Debug("System health check")
}

// reportLoop 报告循环
func (erst *ErrorRecoveryStabilityTest) reportLoop(t *testing.T) {
	ticker := time.NewTicker(erst.config.ReportInterval)
	defer ticker.Stop()

	for {
		select {
		case <-erst.ctx.Done():
			return
		case <-ticker.C:
			if !erst.running.Load() {
				return
			}
			erst.generateProgressReport()
		}
	}
}

// generateProgressReport 生成进度报告
func (erst *ErrorRecoveryStabilityTest) generateProgressReport() {
	elapsed := time.Since(erst.metrics.StartTime)
	remaining := erst.config.TestDuration - elapsed

	totalErrors := atomic.LoadInt64(&erst.metrics.TotalErrors)
	recoveredErrors := atomic.LoadInt64(&erst.metrics.RecoveredErrors)
	failedRecoveries := atomic.LoadInt64(&erst.metrics.FailedRecoveries)

	var recoveryRate float64
	if totalErrors > 0 {
		recoveryRate = float64(recoveredErrors) / float64(totalErrors) * 100
	}

	erst.logger.WithFields(logrus.Fields{
		"elapsed":           elapsed,
		"remaining":         remaining,
		"progress_percent":  float64(elapsed) / float64(erst.config.TestDuration) * 100,
		"total_errors":      totalErrors,
		"recovered_errors":  recoveredErrors,
		"failed_recoveries": failedRecoveries,
		"recovery_rate":     recoveryRate,
		"avg_recovery_time": erst.metrics.AverageRecoveryTime,
		"system_restarts":   atomic.LoadInt64(&erst.metrics.SystemRestarts),
		"cascade_failures":  atomic.LoadInt64(&erst.metrics.CascadeFailures),
	}).Info("Error recovery test progress report")
}

// GetFinalReport 获取最终报告
func (erst *ErrorRecoveryStabilityTest) GetFinalReport() string {
	duration := erst.metrics.EndTime.Sub(erst.metrics.StartTime)

	totalErrors := atomic.LoadInt64(&erst.metrics.TotalErrors)
	recoveredErrors := atomic.LoadInt64(&erst.metrics.RecoveredErrors)
	failedRecoveries := atomic.LoadInt64(&erst.metrics.FailedRecoveries)

	var recoveryRate float64
	if totalErrors > 0 {
		recoveryRate = float64(recoveredErrors) / float64(totalErrors) * 100
	}

	return fmt.Sprintf(`
错误恢复稳定性测试报告:
========================
测试时长: %v
总错误数: %d
恢复成功: %d
恢复失败: %d
恢复成功率: %.2f%%
平均恢复时间: %v
最大恢复时间: %v
最小恢复时间: %v
系统重启次数: %d
级联故障次数: %d
恢复尝试次数: %d

按错误类型统计:
`, duration, totalErrors, recoveredErrors, failedRecoveries, recoveryRate,
		erst.metrics.AverageRecoveryTime, erst.metrics.MaxRecoveryTime, erst.metrics.MinRecoveryTime,
		atomic.LoadInt64(&erst.metrics.SystemRestarts), atomic.LoadInt64(&erst.metrics.CascadeFailures),
		atomic.LoadInt64(&erst.metrics.RecoveryAttempts))
}

// TestErrorRecoveryStabilityLongRunning 长时间错误恢复稳定性测试
func TestErrorRecoveryStabilityLongRunning(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping long-running error recovery stability test in short mode")
	}

	if !isLongRunningTestEnabled() {
		t.Skip("Long-running tests are disabled. Set ENABLE_LONG_RUNNING_TESTS=true to enable")
	}

	// 获取测试持续时间
	duration := getTestDuration(4 * time.Hour)

	config := &ErrorRecoveryConfig{
		TestDuration:        duration,
		ErrorInjectionRate:  0.02, // 2% 错误率
		RecoveryTimeout:     30 * time.Second,
		MaxRecoveryAttempts: 3,
		ErrorTypes: []ErrorType{
			ErrorTypeEncoding,
			ErrorTypeNetwork,
			ErrorTypeResource,
			ErrorTypeMemory,
		},
		ComponentCount:   10,
		ConcurrentErrors: 3,
		MonitorInterval:  10 * time.Second,
		ReportInterval:   10 * time.Minute,
	}

	test := NewErrorRecoveryStabilityTest(config)

	// 启动测试
	err := test.Start(t)
	require.NoError(t, err, "Error recovery stability test should complete without errors")

	// 验证测试结果
	metrics := test.metrics

	totalErrors := atomic.LoadInt64(&metrics.TotalErrors)
	recoveredErrors := atomic.LoadInt64(&metrics.RecoveredErrors)

	// 验证错误恢复效果
	if totalErrors > 0 {
		recoveryRate := float64(recoveredErrors) / float64(totalErrors)
		assert.GreaterOrEqual(t, recoveryRate, 0.85,
			"Error recovery rate should be ≥85%")
	}

	// 验证恢复时间
	assert.LessOrEqual(t, metrics.AverageRecoveryTime, 15*time.Second,
		"Average recovery time should be ≤15 seconds")

	// 验证系统稳定性
	systemRestarts := atomic.LoadInt64(&metrics.SystemRestarts)
	cascadeFailures := atomic.LoadInt64(&metrics.CascadeFailures)

	assert.LessOrEqual(t, systemRestarts, int64(10),
		"System restarts should be minimal (≤10)")
	assert.LessOrEqual(t, cascadeFailures, int64(5),
		"Cascade failures should be minimal (≤5)")

	// 输出最终报告
	t.Logf("Error recovery stability test completed:\n%s", test.GetFinalReport())
}
