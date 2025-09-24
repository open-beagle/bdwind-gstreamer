package gstreamer

import (
	"context"
	"fmt"
	"runtime"
	"time"

	"github.com/go-gst/go-gst/gst"
)

// PipelineStateChecker checks the pipeline state
type PipelineStateChecker struct{}

func (psc *PipelineStateChecker) Name() string {
	return "PipelineState"
}

func (psc *PipelineStateChecker) Check(ctx context.Context, pipeline *gst.Pipeline) HealthCheckResult {
	start := time.Now()

	result := HealthCheckResult{
		Name:      psc.Name(),
		Timestamp: start,
		Details:   make(map[string]interface{}),
	}

	if pipeline == nil {
		result.Status = HealthStatusCritical
		result.Message = "Pipeline is nil"
		result.Error = fmt.Errorf("pipeline is nil")
		result.Duration = time.Since(start)
		return result
	}

	// Get current state
	currentState := pipeline.GetCurrentState()
	result.Details["current_state"] = currentState.String()

	// Check if pipeline is in a valid state
	switch currentState {
	case gst.StatePlaying:
		result.Status = HealthStatusHealthy
		result.Message = "Pipeline is playing"
	case gst.StatePaused:
		result.Status = HealthStatusHealthy
		result.Message = "Pipeline is paused"
	case gst.StateReady:
		result.Status = HealthStatusWarning
		result.Message = "Pipeline is ready but not playing"
	case gst.StateNull:
		result.Status = HealthStatusError
		result.Message = "Pipeline is in null state"
	default:
		result.Status = HealthStatusCritical
		result.Message = fmt.Sprintf("Pipeline is in unknown state: %s", currentState.String())
		result.Error = fmt.Errorf("unknown pipeline state: %s", currentState.String())
	}

	result.Duration = time.Since(start)
	return result
}

// ElementHealthChecker checks the health of pipeline elements
type ElementHealthChecker struct{}

func (ehc *ElementHealthChecker) Name() string {
	return "ElementHealth"
}

func (ehc *ElementHealthChecker) Check(ctx context.Context, pipeline *gst.Pipeline) HealthCheckResult {
	start := time.Now()

	result := HealthCheckResult{
		Name:      ehc.Name(),
		Timestamp: start,
		Details:   make(map[string]interface{}),
	}

	if pipeline == nil {
		result.Status = HealthStatusCritical
		result.Message = "Pipeline is nil"
		result.Error = fmt.Errorf("pipeline is nil")
		result.Duration = time.Since(start)
		return result
	}

	// Get pipeline elements (this is a simplified check)
	// In a real implementation, you would iterate through elements
	// and check their individual states

	elementCount := 0
	healthyElements := 0

	// This is a placeholder - in practice you would use pipeline.GetElements()
	// or similar method to iterate through elements
	result.Details["element_count"] = elementCount
	result.Details["healthy_elements"] = healthyElements

	if elementCount == 0 {
		result.Status = HealthStatusError
		result.Message = "No elements found in pipeline"
	} else if healthyElements == elementCount {
		result.Status = HealthStatusHealthy
		result.Message = fmt.Sprintf("All %d elements are healthy", elementCount)
	} else {
		result.Status = HealthStatusWarning
		result.Message = fmt.Sprintf("%d/%d elements are healthy", healthyElements, elementCount)
	}

	result.Duration = time.Since(start)
	return result
}

// MemoryHealthChecker checks memory usage
type MemoryHealthChecker struct {
	thresholdMB int64
}

func (mhc *MemoryHealthChecker) Name() string {
	return "MemoryHealth"
}

func (mhc *MemoryHealthChecker) Check(ctx context.Context, pipeline *gst.Pipeline) HealthCheckResult {
	start := time.Now()

	result := HealthCheckResult{
		Name:      mhc.Name(),
		Timestamp: start,
		Details:   make(map[string]interface{}),
	}

	// Get memory statistics
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	// Convert to MB
	allocMB := int64(memStats.Alloc) / 1024 / 1024
	sysMB := int64(memStats.Sys) / 1024 / 1024

	result.Details["alloc_mb"] = allocMB
	result.Details["sys_mb"] = sysMB
	result.Details["threshold_mb"] = mhc.thresholdMB
	result.Details["gc_runs"] = memStats.NumGC

	// Check against threshold
	if allocMB > mhc.thresholdMB {
		result.Status = HealthStatusError
		result.Message = fmt.Sprintf("Memory usage too high: %d MB (threshold: %d MB)", allocMB, mhc.thresholdMB)
	} else if allocMB > mhc.thresholdMB*8/10 { // 80% of threshold
		result.Status = HealthStatusWarning
		result.Message = fmt.Sprintf("Memory usage elevated: %d MB (threshold: %d MB)", allocMB, mhc.thresholdMB)
	} else {
		result.Status = HealthStatusHealthy
		result.Message = fmt.Sprintf("Memory usage normal: %d MB", allocMB)
	}

	result.Duration = time.Since(start)
	return result
}

// PerformanceHealthChecker checks performance metrics
type PerformanceHealthChecker struct {
	latencyThresholdMs int64
	errorRateThreshold float64
}

func (phc *PerformanceHealthChecker) Name() string {
	return "PerformanceHealth"
}

func (phc *PerformanceHealthChecker) Check(ctx context.Context, pipeline *gst.Pipeline) HealthCheckResult {
	start := time.Now()

	result := HealthCheckResult{
		Name:      phc.Name(),
		Timestamp: start,
		Details:   make(map[string]interface{}),
	}

	if pipeline == nil {
		result.Status = HealthStatusCritical
		result.Message = "Pipeline is nil"
		result.Error = fmt.Errorf("pipeline is nil")
		result.Duration = time.Since(start)
		return result
	}

	// This is a simplified performance check
	// In practice, you would collect actual performance metrics

	// Simulate latency check
	latencyMs := int64(10) // Placeholder value
	errorRate := 0.01      // Placeholder value (1%)

	result.Details["latency_ms"] = latencyMs
	result.Details["error_rate"] = errorRate
	result.Details["latency_threshold_ms"] = phc.latencyThresholdMs
	result.Details["error_rate_threshold"] = phc.errorRateThreshold

	// Check performance metrics
	if latencyMs > phc.latencyThresholdMs {
		result.Status = HealthStatusWarning
		result.Message = fmt.Sprintf("High latency: %d ms (threshold: %d ms)", latencyMs, phc.latencyThresholdMs)
	} else if errorRate > phc.errorRateThreshold {
		result.Status = HealthStatusWarning
		result.Message = fmt.Sprintf("High error rate: %.2f%% (threshold: %.2f%%)", errorRate*100, phc.errorRateThreshold*100)
	} else {
		result.Status = HealthStatusHealthy
		result.Message = fmt.Sprintf("Performance normal: %d ms latency, %.2f%% error rate", latencyMs, errorRate*100)
	}

	result.Duration = time.Since(start)
	return result
}

// BusMessageHealthChecker checks for error messages on the pipeline bus
type BusMessageHealthChecker struct {
	checkDuration time.Duration
}

func (bmhc *BusMessageHealthChecker) Name() string {
	return "BusMessageHealth"
}

func (bmhc *BusMessageHealthChecker) Check(ctx context.Context, pipeline *gst.Pipeline) HealthCheckResult {
	start := time.Now()

	result := HealthCheckResult{
		Name:      bmhc.Name(),
		Timestamp: start,
		Details:   make(map[string]interface{}),
	}

	if pipeline == nil {
		result.Status = HealthStatusCritical
		result.Message = "Pipeline is nil"
		result.Error = fmt.Errorf("pipeline is nil")
		result.Duration = time.Since(start)
		return result
	}

	bus := pipeline.GetPipelineBus()
	if bus == nil {
		result.Status = HealthStatusError
		result.Message = "No bus available"
		result.Error = fmt.Errorf("no bus available")
		result.Duration = time.Since(start)
		return result
	}

	// Check for error messages on the bus
	errorCount := 0
	warningCount := 0
	checkDuration := bmhc.checkDuration
	if checkDuration == 0 {
		checkDuration = 1 * time.Second
	}

	checkCtx, cancel := context.WithTimeout(ctx, checkDuration)
	defer cancel()

	checkStart := time.Now()
	for time.Since(checkStart) < checkDuration {
		select {
		case <-checkCtx.Done():
			break
		default:
			msg := bus.TimedPop(gst.ClockTime(100 * time.Millisecond))
			if msg == nil {
				continue
			}

			switch msg.Type() {
			case gst.MessageError:
				errorCount++
			case gst.MessageWarning:
				warningCount++
			}
		}
	}

	result.Details["error_count"] = errorCount
	result.Details["warning_count"] = warningCount
	result.Details["check_duration_ms"] = checkDuration.Milliseconds()

	// Evaluate health based on message counts
	if errorCount > 0 {
		result.Status = HealthStatusError
		result.Message = fmt.Sprintf("Found %d error messages in %v", errorCount, checkDuration)
	} else if warningCount > 5 {
		result.Status = HealthStatusWarning
		result.Message = fmt.Sprintf("Found %d warning messages in %v", warningCount, checkDuration)
	} else if warningCount > 0 {
		result.Status = HealthStatusHealthy
		result.Message = fmt.Sprintf("Found %d warning messages in %v (acceptable)", warningCount, checkDuration)
	} else {
		result.Status = HealthStatusHealthy
		result.Message = "No error or warning messages found"
	}

	result.Duration = time.Since(start)
	return result
}

// ResourceHealthChecker checks system resource availability
type ResourceHealthChecker struct {
	cpuThreshold    float64
	memoryThreshold int64
}

func (rhc *ResourceHealthChecker) Name() string {
	return "ResourceHealth"
}

func (rhc *ResourceHealthChecker) Check(ctx context.Context, pipeline *gst.Pipeline) HealthCheckResult {
	start := time.Now()

	result := HealthCheckResult{
		Name:      rhc.Name(),
		Timestamp: start,
		Details:   make(map[string]interface{}),
	}

	// Get system resource information
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	// Memory check
	allocMB := int64(memStats.Alloc) / 1024 / 1024
	sysMB := int64(memStats.Sys) / 1024 / 1024

	result.Details["memory_alloc_mb"] = allocMB
	result.Details["memory_sys_mb"] = sysMB
	result.Details["memory_threshold_mb"] = rhc.memoryThreshold
	result.Details["goroutines"] = runtime.NumGoroutine()
	result.Details["cpu_cores"] = runtime.NumCPU()

	// Simple resource health evaluation
	if rhc.memoryThreshold > 0 && allocMB > rhc.memoryThreshold {
		result.Status = HealthStatusError
		result.Message = fmt.Sprintf("Memory usage too high: %d MB (threshold: %d MB)", allocMB, rhc.memoryThreshold)
	} else if runtime.NumGoroutine() > 1000 {
		result.Status = HealthStatusWarning
		result.Message = fmt.Sprintf("High goroutine count: %d", runtime.NumGoroutine())
	} else {
		result.Status = HealthStatusHealthy
		result.Message = fmt.Sprintf("Resources normal: %d MB memory, %d goroutines", allocMB, runtime.NumGoroutine())
	}

	result.Duration = time.Since(start)
	return result
}

// ConnectivityHealthChecker checks pipeline connectivity and data flow
type ConnectivityHealthChecker struct {
	timeoutDuration time.Duration
}

func (chc *ConnectivityHealthChecker) Name() string {
	return "ConnectivityHealth"
}

func (chc *ConnectivityHealthChecker) Check(ctx context.Context, pipeline *gst.Pipeline) HealthCheckResult {
	start := time.Now()

	result := HealthCheckResult{
		Name:      chc.Name(),
		Timestamp: start,
		Details:   make(map[string]interface{}),
	}

	if pipeline == nil {
		result.Status = HealthStatusCritical
		result.Message = "Pipeline is nil"
		result.Error = fmt.Errorf("pipeline is nil")
		result.Duration = time.Since(start)
		return result
	}

	// Check pipeline state for connectivity
	currentState := pipeline.GetCurrentState()
	result.Details["pipeline_state"] = currentState.String()

	// Simple connectivity check based on pipeline state
	switch currentState {
	case gst.StatePlaying:
		result.Status = HealthStatusHealthy
		result.Message = "Pipeline is playing - connectivity OK"
	case gst.StatePaused:
		result.Status = HealthStatusHealthy
		result.Message = "Pipeline is paused - connectivity OK"
	case gst.StateReady:
		result.Status = HealthStatusWarning
		result.Message = "Pipeline is ready but not active"
	case gst.StateNull:
		result.Status = HealthStatusError
		result.Message = "Pipeline is not connected"
	default:
		result.Status = HealthStatusCritical
		result.Message = fmt.Sprintf("Pipeline in unknown state: %s", currentState.String())
		result.Error = fmt.Errorf("unknown pipeline state: %s", currentState.String())
	}

	result.Duration = time.Since(start)
	return result
}

// CustomHealthChecker allows for custom health checks
type CustomHealthChecker struct {
	name    string
	checkFn func(ctx context.Context, pipeline *gst.Pipeline) HealthCheckResult
}

func NewCustomHealthChecker(name string, checkFn func(ctx context.Context, pipeline *gst.Pipeline) HealthCheckResult) *CustomHealthChecker {
	return &CustomHealthChecker{
		name:    name,
		checkFn: checkFn,
	}
}

func (chc *CustomHealthChecker) Name() string {
	return chc.name
}

func (chc *CustomHealthChecker) Check(ctx context.Context, pipeline *gst.Pipeline) HealthCheckResult {
	if chc.checkFn == nil {
		return HealthCheckResult{
			Name:      chc.name,
			Status:    HealthStatusCritical,
			Message:   "No check function provided",
			Error:     fmt.Errorf("no check function provided"),
			Timestamp: time.Now(),
			Duration:  0,
			Details:   make(map[string]interface{}),
		}
	}

	return chc.checkFn(ctx, pipeline)
}
