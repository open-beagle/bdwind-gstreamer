package gstreamer

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-gst/go-gst/gst"
	"github.com/sirupsen/logrus"
)

// HealthCheckResult represents the result of a single health check
type HealthCheckResult struct {
	Name      string
	Status    HealthStatusLevel
	Message   string
	Details   map[string]interface{}
	Duration  time.Duration
	Timestamp time.Time
	Error     error
}

// OverallHealthStatus represents the overall health status of the pipeline
type OverallHealthStatus struct {
	HealthStatus
	CheckCount  int
	FailedCount int
}

// HealthChecker interface defines a health check
type HealthChecker interface {
	Name() string
	Check(ctx context.Context, pipeline *gst.Pipeline) HealthCheckResult
}

// PipelineHealthCheckerConfig holds configuration for the health checker
type PipelineHealthCheckerConfig struct {
	// Health check timeout
	CheckTimeout time.Duration

	// Individual check timeouts
	StateCheckTimeout       time.Duration
	ElementCheckTimeout     time.Duration
	MemoryCheckTimeout      time.Duration
	PerformanceCheckTimeout time.Duration

	// Thresholds
	MemoryThresholdMB   int64
	CPUThresholdPercent float64
	LatencyThresholdMs  int64
	ErrorRateThreshold  float64

	// Check intervals
	QuickCheckInterval    time.Duration
	DetailedCheckInterval time.Duration

	// History settings
	MaxCheckHistory int

	// Feature flags
	EnableMemoryChecks      bool
	EnablePerformanceChecks bool
	EnableElementChecks     bool
	EnableStateChecks       bool
}

// DefaultHealthCheckerConfig returns default health checker configuration
func DefaultHealthCheckerConfig() *PipelineHealthCheckerConfig {
	return &PipelineHealthCheckerConfig{
		CheckTimeout:            30 * time.Second,
		StateCheckTimeout:       5 * time.Second,
		ElementCheckTimeout:     10 * time.Second,
		MemoryCheckTimeout:      5 * time.Second,
		PerformanceCheckTimeout: 15 * time.Second,
		MemoryThresholdMB:       512,
		CPUThresholdPercent:     80.0,
		LatencyThresholdMs:      100,
		ErrorRateThreshold:      0.05, // 5%
		QuickCheckInterval:      10 * time.Second,
		DetailedCheckInterval:   60 * time.Second,
		MaxCheckHistory:         100,
		EnableMemoryChecks:      true,
		EnablePerformanceChecks: true,
		EnableElementChecks:     true,
		EnableStateChecks:       true,
	}
}

// PipelineHealthChecker monitors pipeline health
type PipelineHealthChecker struct {
	// Configuration
	config *PipelineHealthCheckerConfig
	logger *logrus.Entry

	// Pipeline state manager reference
	stateManager *PipelineStateManager

	// Health checkers
	checkers []HealthChecker

	// Health status tracking
	mu                sync.RWMutex
	lastOverallStatus OverallHealthStatus
	checkHistory      []HealthCheckResult

	// Metrics
	metrics *HealthCheckerMetrics

	// Context and lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Internal state
	startTime time.Time
}

// HealthCheckerMetrics tracks health checker performance
type HealthCheckerMetrics struct {
	mu sync.RWMutex

	TotalChecks            int64
	SuccessfulChecks       int64
	FailedChecks           int64
	AverageCheckTime       time.Duration
	LastCheckTime          time.Time
	ChecksByStatus         map[HealthStatusLevel]int64
	ChecksByType           map[string]int64
	ConsecutiveFailures    int64
	MaxConsecutiveFailures int64
}

// NewPipelineHealthChecker creates a new pipeline health checker
func NewPipelineHealthChecker(logger *logrus.Entry, stateManager *PipelineStateManager) *PipelineHealthChecker {
	if logger == nil {
		logger = logrus.WithField("component", "pipeline-health-checker")
	}

	config := DefaultHealthCheckerConfig()

	phc := &PipelineHealthChecker{
		config:       config,
		logger:       logger,
		stateManager: stateManager,
		checkers:     make([]HealthChecker, 0),
		checkHistory: make([]HealthCheckResult, 0, config.MaxCheckHistory),
		startTime:    time.Now(),
		metrics: &HealthCheckerMetrics{
			ChecksByStatus: make(map[HealthStatusLevel]int64),
			ChecksByType:   make(map[string]int64),
		},
	}

	// Initialize default health checkers
	phc.initializeDefaultCheckers()

	return phc
}

// initializeDefaultCheckers initializes the default set of health checkers
func (phc *PipelineHealthChecker) initializeDefaultCheckers() {
	if phc.config.EnableStateChecks {
		phc.checkers = append(phc.checkers, &PipelineStateChecker{})
	}

	if phc.config.EnableElementChecks {
		phc.checkers = append(phc.checkers, &ElementHealthChecker{})
	}

	if phc.config.EnableMemoryChecks {
		phc.checkers = append(phc.checkers, &MemoryHealthChecker{
			thresholdMB: phc.config.MemoryThresholdMB,
		})
	}

	if phc.config.EnablePerformanceChecks {
		phc.checkers = append(phc.checkers, &PerformanceHealthChecker{
			latencyThresholdMs: phc.config.LatencyThresholdMs,
			errorRateThreshold: phc.config.ErrorRateThreshold,
		})
	}

	phc.logger.Debugf("Initialized %d health checkers", len(phc.checkers))
}

// Start starts the health checker
func (phc *PipelineHealthChecker) Start(ctx context.Context) error {
	phc.ctx, phc.cancel = context.WithCancel(ctx)

	phc.logger.Info("Starting pipeline health checker")

	// Start health monitoring loop
	phc.wg.Add(1)
	go phc.healthMonitorLoop()

	phc.logger.Info("Pipeline health checker started successfully")
	return nil
}

// Stop stops the health checker
func (phc *PipelineHealthChecker) Stop() error {
	phc.logger.Info("Stopping pipeline health checker")

	if phc.cancel != nil {
		phc.cancel()
	}

	phc.wg.Wait()

	phc.logger.Info("Pipeline health checker stopped successfully")
	return nil
}

// CheckHealth performs a comprehensive health check
func (phc *PipelineHealthChecker) CheckHealth() OverallHealthStatus {
	checkStart := time.Now()

	// Get pipeline from state manager
	var pipeline *gst.Pipeline
	if phc.stateManager != nil {
		pipeline = phc.stateManager.pipeline
	}

	if pipeline == nil {
		return OverallHealthStatus{
			HealthStatus: HealthStatus{
				Overall:   HealthStatusCritical,
				LastCheck: time.Now().Unix(),
				Uptime:    int64(time.Since(phc.startTime).Seconds()),
				Issues:    []string{"No pipeline available for health check"},
				Checks:    make(map[string]interface{}),
			},
			CheckCount:  0,
			FailedCount: 1,
		}
	}

	// Create context with timeout
	var ctx context.Context
	var cancel context.CancelFunc

	if phc.ctx != nil {
		ctx, cancel = context.WithTimeout(phc.ctx, phc.config.CheckTimeout)
	} else {
		// If health checker hasn't been started yet, use background context
		ctx, cancel = context.WithTimeout(context.Background(), phc.config.CheckTimeout)
	}
	defer cancel()

	// Run all health checks
	results := make([]HealthCheckResult, 0, len(phc.checkers))
	overallStatus := HealthStatusHealthy
	issues := make([]string, 0)
	checks := make(map[string]interface{})
	failedCount := 0

	for _, checker := range phc.checkers {
		result := checker.Check(ctx, pipeline)
		results = append(results, result)

		// Update overall status
		if result.Status > overallStatus {
			overallStatus = result.Status
		}

		// Collect issues
		if result.Status != HealthStatusHealthy {
			failedCount++
			if result.Error != nil {
				issues = append(issues, fmt.Sprintf("%s: %s", result.Name, result.Error.Error()))
			} else {
				issues = append(issues, fmt.Sprintf("%s: %s", result.Name, result.Message))
			}
		}

		// Add to checks map
		checks[result.Name] = map[string]interface{}{
			"status":    result.Status.String(),
			"message":   result.Message,
			"duration":  result.Duration.Milliseconds(),
			"timestamp": result.Timestamp.Unix(),
		}

		// Record in history
		phc.recordCheckResult(result)

		// Update metrics
		phc.updateMetrics(result)
	}

	// Create overall status
	status := OverallHealthStatus{
		HealthStatus: HealthStatus{
			Overall:   overallStatus,
			LastCheck: time.Now().Unix(),
			Uptime:    int64(time.Since(phc.startTime).Seconds()),
			Issues:    issues,
			Checks:    checks,
		},
		CheckCount:  len(results),
		FailedCount: failedCount,
	}

	// Update last status
	phc.mu.Lock()
	phc.lastOverallStatus = status
	phc.mu.Unlock()

	// Log health check summary
	checkDuration := time.Since(checkStart)
	phc.logger.Debugf("Health check completed in %v: %s (%d/%d checks passed)",
		checkDuration, overallStatus.String(), len(results)-failedCount, len(results))

	if overallStatus != HealthStatusHealthy {
		phc.logger.Warnf("Health issues detected: %v", issues)
	}

	return status
}

// GetHealthStatus returns the last health status
func (phc *PipelineHealthChecker) GetHealthStatus() OverallHealthStatus {
	phc.mu.RLock()
	defer phc.mu.RUnlock()
	return phc.lastOverallStatus
}

// recordCheckResult records a health check result in history
func (phc *PipelineHealthChecker) recordCheckResult(result HealthCheckResult) {
	phc.mu.Lock()
	defer phc.mu.Unlock()

	phc.checkHistory = append(phc.checkHistory, result)

	// Trim history if needed
	if len(phc.checkHistory) > phc.config.MaxCheckHistory {
		phc.checkHistory = phc.checkHistory[1:]
	}
}

// updateMetrics updates health checker metrics
func (phc *PipelineHealthChecker) updateMetrics(result HealthCheckResult) {
	phc.metrics.mu.Lock()
	defer phc.metrics.mu.Unlock()

	phc.metrics.TotalChecks++
	phc.metrics.LastCheckTime = result.Timestamp
	phc.metrics.ChecksByType[result.Name]++
	phc.metrics.ChecksByStatus[result.Status]++

	if result.Status == HealthStatusHealthy {
		phc.metrics.SuccessfulChecks++
		phc.metrics.ConsecutiveFailures = 0
	} else {
		phc.metrics.FailedChecks++
		phc.metrics.ConsecutiveFailures++
		if phc.metrics.ConsecutiveFailures > phc.metrics.MaxConsecutiveFailures {
			phc.metrics.MaxConsecutiveFailures = phc.metrics.ConsecutiveFailures
		}
	}

	// Update average check time
	if phc.metrics.TotalChecks == 1 {
		phc.metrics.AverageCheckTime = result.Duration
	} else {
		total := phc.metrics.AverageCheckTime * time.Duration(phc.metrics.TotalChecks-1)
		phc.metrics.AverageCheckTime = (total + result.Duration) / time.Duration(phc.metrics.TotalChecks)
	}
}

// healthMonitorLoop runs the continuous health monitoring
func (phc *PipelineHealthChecker) healthMonitorLoop() {
	defer phc.wg.Done()

	quickTicker := time.NewTicker(phc.config.QuickCheckInterval)
	defer quickTicker.Stop()

	detailedTicker := time.NewTicker(phc.config.DetailedCheckInterval)
	defer detailedTicker.Stop()

	phc.logger.Debug("Health monitoring loop started")

	for {
		select {
		case <-phc.ctx.Done():
			phc.logger.Debug("Health monitoring loop stopped")
			return
		case <-quickTicker.C:
			// Perform quick health check
			phc.performQuickHealthCheck()
		case <-detailedTicker.C:
			// Perform detailed health check
			phc.CheckHealth()
		}
	}
}

// performQuickHealthCheck performs a quick health check with essential checks only
func (phc *PipelineHealthChecker) performQuickHealthCheck() {
	// Only run state and basic element checks for quick check
	if phc.stateManager == nil || phc.stateManager.pipeline == nil {
		return
	}

	ctx, cancel := context.WithTimeout(phc.ctx, 5*time.Second)
	defer cancel()

	// Quick state check
	stateChecker := &PipelineStateChecker{}
	result := stateChecker.Check(ctx, phc.stateManager.pipeline)

	phc.recordCheckResult(result)
	phc.updateMetrics(result)

	if result.Status != HealthStatusHealthy {
		phc.logger.Warnf("Quick health check failed: %s", result.Message)
	}
}

// GetMetrics returns health checker metrics
func (phc *PipelineHealthChecker) GetMetrics() *HealthCheckerMetrics {
	phc.metrics.mu.RLock()
	defer phc.metrics.mu.RUnlock()

	// Create a copy of metrics
	metrics := &HealthCheckerMetrics{
		TotalChecks:            phc.metrics.TotalChecks,
		SuccessfulChecks:       phc.metrics.SuccessfulChecks,
		FailedChecks:           phc.metrics.FailedChecks,
		AverageCheckTime:       phc.metrics.AverageCheckTime,
		LastCheckTime:          phc.metrics.LastCheckTime,
		ConsecutiveFailures:    phc.metrics.ConsecutiveFailures,
		MaxConsecutiveFailures: phc.metrics.MaxConsecutiveFailures,
		ChecksByStatus:         make(map[HealthStatusLevel]int64),
		ChecksByType:           make(map[string]int64),
	}

	for k, v := range phc.metrics.ChecksByStatus {
		metrics.ChecksByStatus[k] = v
	}
	for k, v := range phc.metrics.ChecksByType {
		metrics.ChecksByType[k] = v
	}

	return metrics
}

// GetCheckHistory returns the health check history
func (phc *PipelineHealthChecker) GetCheckHistory() []HealthCheckResult {
	phc.mu.RLock()
	defer phc.mu.RUnlock()

	history := make([]HealthCheckResult, len(phc.checkHistory))
	copy(history, phc.checkHistory)
	return history
}

// AddChecker adds a custom health checker
func (phc *PipelineHealthChecker) AddChecker(checker HealthChecker) {
	phc.mu.Lock()
	defer phc.mu.Unlock()

	phc.checkers = append(phc.checkers, checker)
	phc.logger.Debugf("Added custom health checker: %s", checker.Name())
}

// RemoveChecker removes a health checker by name
func (phc *PipelineHealthChecker) RemoveChecker(name string) bool {
	phc.mu.Lock()
	defer phc.mu.Unlock()

	for i, checker := range phc.checkers {
		if checker.Name() == name {
			phc.checkers = append(phc.checkers[:i], phc.checkers[i+1:]...)
			phc.logger.Debugf("Removed health checker: %s", name)
			return true
		}
	}

	return false
}

// GetCheckerNames returns the names of all registered health checkers
func (phc *PipelineHealthChecker) GetCheckerNames() []string {
	phc.mu.RLock()
	defer phc.mu.RUnlock()

	names := make([]string, len(phc.checkers))
	for i, checker := range phc.checkers {
		names[i] = checker.Name()
	}

	return names
}

// IsHealthy returns true if the pipeline is healthy
func (phc *PipelineHealthChecker) IsHealthy() bool {
	phc.mu.RLock()
	defer phc.mu.RUnlock()
	return phc.lastOverallStatus.Overall == HealthStatusHealthy
}

// GetIssues returns current health issues
func (phc *PipelineHealthChecker) GetIssues() []string {
	phc.mu.RLock()
	defer phc.mu.RUnlock()

	issues := make([]string, len(phc.lastOverallStatus.Issues))
	copy(issues, phc.lastOverallStatus.Issues)
	return issues
}
