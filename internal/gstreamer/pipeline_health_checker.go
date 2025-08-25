package gstreamer

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// PipelineHealthChecker monitors pipeline health and detects issues
type PipelineHealthChecker struct {
	pipeline Pipeline
	logger   *logrus.Entry
	mu       sync.RWMutex

	// Configuration
	config *HealthCheckerConfig

	// State
	running      bool
	lastCheck    time.Time
	healthStatus HealthStatus

	// Context and cancellation
	ctx    context.Context
	cancel context.CancelFunc

	// Callbacks
	healthCallback func(HealthStatus)
	issueCallback  func(HealthIssue)
}

// HealthCheckerConfig contains configuration for the health checker
type HealthCheckerConfig struct {
	// Check interval
	CheckInterval time.Duration

	// Timeout for health checks
	CheckTimeout time.Duration

	// State manager reference for recovery actions
	StateManager *PipelineStateManager

	// Enable detailed logging
	VerboseLogging bool
}

// HealthStatus represents the overall health status of the pipeline
type HealthStatus struct {
	Overall   HealthLevel
	LastCheck time.Time
	Issues    []HealthIssue
	Checks    map[string]HealthCheckResult
	Uptime    time.Duration
	StartTime time.Time
}

// HealthLevel represents the severity level of health status
type HealthLevel int

const (
	HealthUnknown HealthLevel = iota
	HealthHealthy
	HealthWarning
	HealthCritical
	HealthFailed
)

// String returns string representation of health level
func (hl HealthLevel) String() string {
	switch hl {
	case HealthHealthy:
		return "Healthy"
	case HealthWarning:
		return "Warning"
	case HealthCritical:
		return "Critical"
	case HealthFailed:
		return "Failed"
	default:
		return "Unknown"
	}
}

// HealthIssue represents a specific health issue
type HealthIssue struct {
	Type       IssueType
	Severity   HealthLevel
	Message    string
	Timestamp  time.Time
	CheckName  string
	Details    map[string]interface{}
	Resolved   bool
	ResolvedAt time.Time
}

// IssueType represents the type of health issue
type IssueType int

const (
	IssueUnknown IssueType = iota
	IssueStateInconsistency
	IssueStateTransitionTimeout
	IssueDataFlowStopped
	IssueBusError
	IssueElementError
	IssueMemoryLeak
	IssuePerformanceDegradation
)

// String returns string representation of issue type
func (it IssueType) String() string {
	switch it {
	case IssueStateInconsistency:
		return "StateInconsistency"
	case IssueStateTransitionTimeout:
		return "StateTransitionTimeout"
	case IssueDataFlowStopped:
		return "DataFlowStopped"
	case IssueBusError:
		return "BusError"
	case IssueElementError:
		return "ElementError"
	case IssueMemoryLeak:
		return "MemoryLeak"
	case IssuePerformanceDegradation:
		return "PerformanceDegradation"
	default:
		return "Unknown"
	}
}

// HealthCheckResult represents the result of a specific health check
type HealthCheckResult struct {
	Name      string
	Status    HealthLevel
	Message   string
	Timestamp time.Time
	Duration  time.Duration
	Details   map[string]interface{}
}

// NewPipelineHealthChecker creates a new pipeline health checker
func NewPipelineHealthChecker(ctx context.Context, pipeline Pipeline, logger *logrus.Entry, healthConfig *HealthCheckerConfig) *PipelineHealthChecker {
	if logger == nil {
		logger = logrus.WithField("component", "pipeline-health-checker")
	}

	if healthConfig == nil {
		healthConfig = &HealthCheckerConfig{
			CheckInterval: 5 * time.Second,
			CheckTimeout:  2 * time.Second,
		}
	}

	childCtx, cancel := context.WithCancel(ctx)

	return &PipelineHealthChecker{
		pipeline: pipeline,
		logger:   logger,
		config:   healthConfig,
		ctx:      childCtx,
		cancel:   cancel,
		healthStatus: HealthStatus{
			Overall:   HealthUnknown,
			StartTime: time.Now(),
			Checks:    make(map[string]HealthCheckResult),
			Issues:    make([]HealthIssue, 0),
		},
	}
}

// Start starts the health checker
func (phc *PipelineHealthChecker) Start() error {
	phc.mu.Lock()
	defer phc.mu.Unlock()

	if phc.running {
		return fmt.Errorf("health checker already running")
	}

	phc.running = true
	phc.healthStatus.StartTime = time.Now()

	// Start health monitoring goroutine
	go phc.healthCheckLoop()

	phc.logger.Printf("Pipeline health checker started")
	return nil
}

// Stop stops the health checker
func (phc *PipelineHealthChecker) Stop() error {
	phc.mu.Lock()
	defer phc.mu.Unlock()

	if !phc.running {
		return nil
	}

	phc.running = false
	phc.cancel()

	phc.logger.Printf("Pipeline health checker stopped")
	return nil
}

// healthCheckLoop runs the main health checking loop
func (phc *PipelineHealthChecker) healthCheckLoop() {
	ticker := time.NewTicker(phc.config.CheckInterval)
	defer ticker.Stop()

	phc.logger.Printf("Starting pipeline health monitoring loop...")

	for {
		select {
		case <-phc.ctx.Done():
			phc.logger.Printf("Health check loop stopped")
			return
		case <-ticker.C:
			phc.performHealthCheck()
		}
	}
}

// performHealthCheck performs a comprehensive health check
func (phc *PipelineHealthChecker) performHealthCheck() {
	if phc.config.VerboseLogging {
		phc.logger.Printf("Performing pipeline health check...")
	}

	checkStart := time.Now()

	// Create context with timeout for this check
	ctx, cancel := context.WithTimeout(phc.ctx, phc.config.CheckTimeout)
	defer cancel()

	// Perform individual health checks
	checks := []func(context.Context) HealthCheckResult{
		phc.checkPipelineState,
		phc.checkBusMessages,
		phc.checkDataFlow,
		phc.checkElementHealth,
		phc.checkPerformance,
	}

	results := make(map[string]HealthCheckResult)
	var issues []HealthIssue
	overallHealth := HealthHealthy

	for _, checkFunc := range checks {
		result := checkFunc(ctx)
		results[result.Name] = result

		// Update overall health based on check result
		if result.Status > overallHealth {
			overallHealth = result.Status
		}

		// Create issue if check failed
		if result.Status >= HealthWarning {
			issue := HealthIssue{
				Type:      phc.getIssueTypeFromCheckName(result.Name),
				Severity:  result.Status,
				Message:   result.Message,
				Timestamp: result.Timestamp,
				CheckName: result.Name,
				Details:   result.Details,
			}
			issues = append(issues, issue)
		}
	}

	// Update health status
	phc.mu.Lock()
	phc.lastCheck = checkStart
	phc.healthStatus.Overall = overallHealth
	phc.healthStatus.LastCheck = checkStart
	phc.healthStatus.Checks = results
	phc.healthStatus.Uptime = time.Since(phc.healthStatus.StartTime)

	// Add new issues and resolve old ones
	phc.updateIssues(issues)
	phc.mu.Unlock()

	// Log health status
	if overallHealth >= HealthWarning || phc.config.VerboseLogging {
		phc.logger.Printf("Health check completed: %s (took %v)",
			overallHealth.String(), time.Since(checkStart))

		if len(issues) > 0 {
			phc.logger.Printf("Found %d health issues:", len(issues))
			for _, issue := range issues {
				phc.logger.Printf("  - %s: %s", issue.Type.String(), issue.Message)
			}
		}
	}

	// Notify callbacks
	if phc.healthCallback != nil {
		phc.healthCallback(phc.healthStatus)
	}

	for _, issue := range issues {
		if phc.issueCallback != nil {
			phc.issueCallback(issue)
		}
	}
}

// checkPipelineState checks the pipeline state consistency
func (phc *PipelineHealthChecker) checkPipelineState(ctx context.Context) HealthCheckResult {
	start := time.Now()
	result := HealthCheckResult{
		Name:      "PipelineState",
		Timestamp: start,
		Details:   make(map[string]interface{}),
	}

	// Get current pipeline state
	state, err := phc.pipeline.GetState()
	if err != nil {
		result.Status = HealthCritical
		result.Message = fmt.Sprintf("Failed to get pipeline state: %v", err)
		result.Details["error"] = err.Error()
	} else {
		result.Status = HealthHealthy
		result.Message = fmt.Sprintf("Pipeline state: %v", state)
		result.Details["state"] = state

		// Check if state manager is available and states match
		if phc.config.StateManager != nil {
			trackedState := phc.config.StateManager.GetCurrentState()
			if state != trackedState {
				result.Status = HealthWarning
				result.Message = fmt.Sprintf("State inconsistency: actual=%v, tracked=%v", state, trackedState)
				result.Details["actual_state"] = state
				result.Details["tracked_state"] = trackedState
			}
		}
	}

	result.Duration = time.Since(start)
	return result
}

// checkBusMessages checks for error messages on the pipeline bus
func (phc *PipelineHealthChecker) checkBusMessages(ctx context.Context) HealthCheckResult {
	start := time.Now()
	result := HealthCheckResult{
		Name:      "BusMessages",
		Timestamp: start,
		Details:   make(map[string]interface{}),
	}

	// Get pipeline bus
	bus, err := phc.pipeline.GetBus()
	if err != nil {
		result.Status = HealthCritical
		result.Message = fmt.Sprintf("Failed to get pipeline bus: %v", err)
		result.Details["error"] = err.Error()
		result.Duration = time.Since(start)
		return result
	}

	// Check for messages (non-blocking)
	var errorCount, warningCount int
	var lastError, lastWarning string

	for {
		select {
		case <-ctx.Done():
			// Timeout reached
			result.Status = HealthWarning
			result.Message = "Bus message check timed out"
			result.Duration = time.Since(start)
			return result
		default:
			// Try to pop a message
			msg, err := bus.Pop()
			if err != nil {
				result.Status = HealthWarning
				result.Message = fmt.Sprintf("Failed to pop bus message: %v", err)
				result.Details["error"] = err.Error()
				result.Duration = time.Since(start)
				return result
			}

			if msg == nil {
				// No more messages
				break
			}

			// Process message based on type
			msgType := msg.GetType()
			switch msgType {
			case MessageError:
				errorCount++
				if err, debug := msg.ParseError(); err != nil {
					lastError = fmt.Sprintf("%v (debug: %s)", err, debug)
				}
			case MessageWarning:
				warningCount++
				// Warning parsing would be similar to error parsing
				lastWarning = "Warning message received"
			}
		}
	}

	// Determine health status based on messages
	if errorCount > 0 {
		result.Status = HealthCritical
		result.Message = fmt.Sprintf("Found %d error messages, last: %s", errorCount, lastError)
		result.Details["error_count"] = errorCount
		result.Details["last_error"] = lastError
	} else if warningCount > 0 {
		result.Status = HealthWarning
		result.Message = fmt.Sprintf("Found %d warning messages", warningCount)
		result.Details["warning_count"] = warningCount
		result.Details["last_warning"] = lastWarning
	} else {
		result.Status = HealthHealthy
		result.Message = "No error or warning messages"
	}

	result.Duration = time.Since(start)
	return result
}

// checkDataFlow checks if data is flowing through the pipeline
func (phc *PipelineHealthChecker) checkDataFlow(ctx context.Context) HealthCheckResult {
	start := time.Now()
	result := HealthCheckResult{
		Name:      "DataFlow",
		Timestamp: start,
		Details:   make(map[string]interface{}),
	}

	// This is a simplified check - in a real implementation, you would
	// monitor actual data flow through the pipeline elements

	// For now, we'll check if the pipeline is in PLAYING state
	state, err := phc.pipeline.GetState()
	if err != nil {
		result.Status = HealthWarning
		result.Message = fmt.Sprintf("Cannot check data flow: %v", err)
		result.Details["error"] = err.Error()
	} else if state != StatePlaying {
		result.Status = HealthWarning
		result.Message = fmt.Sprintf("Pipeline not playing, data flow may be stopped (state: %v)", state)
		result.Details["state"] = state
	} else {
		result.Status = HealthHealthy
		result.Message = "Pipeline is playing, data flow appears normal"
		result.Details["state"] = state
	}

	result.Duration = time.Since(start)
	return result
}

// checkElementHealth checks the health of individual pipeline elements
func (phc *PipelineHealthChecker) checkElementHealth(ctx context.Context) HealthCheckResult {
	start := time.Now()
	result := HealthCheckResult{
		Name:      "ElementHealth",
		Timestamp: start,
		Details:   make(map[string]interface{}),
	}

	// This is a placeholder for element-specific health checks
	// In a real implementation, you would iterate through pipeline elements
	// and check their individual states and properties

	result.Status = HealthHealthy
	result.Message = "Element health check not implemented yet"
	result.Details["note"] = "Placeholder implementation"

	result.Duration = time.Since(start)
	return result
}

// checkPerformance checks pipeline performance metrics
func (phc *PipelineHealthChecker) checkPerformance(ctx context.Context) HealthCheckResult {
	start := time.Now()
	result := HealthCheckResult{
		Name:      "Performance",
		Timestamp: start,
		Details:   make(map[string]interface{}),
	}

	// This is a placeholder for performance monitoring
	// In a real implementation, you would check metrics like:
	// - CPU usage
	// - Memory usage
	// - Frame rate
	// - Latency

	result.Status = HealthHealthy
	result.Message = "Performance monitoring not implemented yet"
	result.Details["note"] = "Placeholder implementation"

	result.Duration = time.Since(start)
	return result
}

// updateIssues updates the issues list, resolving old issues and adding new ones
func (phc *PipelineHealthChecker) updateIssues(newIssues []HealthIssue) {
	now := time.Now()

	// Mark existing issues as resolved if they're not in the new issues
	for i := range phc.healthStatus.Issues {
		if !phc.healthStatus.Issues[i].Resolved {
			found := false
			for _, newIssue := range newIssues {
				if phc.healthStatus.Issues[i].Type == newIssue.Type &&
					phc.healthStatus.Issues[i].CheckName == newIssue.CheckName {
					found = true
					break
				}
			}
			if !found {
				phc.healthStatus.Issues[i].Resolved = true
				phc.healthStatus.Issues[i].ResolvedAt = now
			}
		}
	}

	// Add new issues
	for _, newIssue := range newIssues {
		// Check if this issue already exists
		found := false
		for _, existingIssue := range phc.healthStatus.Issues {
			if existingIssue.Type == newIssue.Type &&
				existingIssue.CheckName == newIssue.CheckName &&
				!existingIssue.Resolved {
				found = true
				break
			}
		}
		if !found {
			phc.healthStatus.Issues = append(phc.healthStatus.Issues, newIssue)
		}
	}
}

// getIssueTypeFromCheckName maps check names to issue types
func (phc *PipelineHealthChecker) getIssueTypeFromCheckName(checkName string) IssueType {
	switch checkName {
	case "PipelineState":
		return IssueStateInconsistency
	case "BusMessages":
		return IssueBusError
	case "DataFlow":
		return IssueDataFlowStopped
	case "ElementHealth":
		return IssueElementError
	case "Performance":
		return IssuePerformanceDegradation
	default:
		return IssueUnknown
	}
}

// GetHealthStatus returns the current health status
func (phc *PipelineHealthChecker) GetHealthStatus() HealthStatus {
	phc.mu.RLock()
	defer phc.mu.RUnlock()

	// Return a copy to prevent external modification
	status := phc.healthStatus
	status.Issues = make([]HealthIssue, len(phc.healthStatus.Issues))
	copy(status.Issues, phc.healthStatus.Issues)

	return status
}

// SetHealthCallback sets a callback for health status updates
func (phc *PipelineHealthChecker) SetHealthCallback(callback func(HealthStatus)) {
	phc.mu.Lock()
	defer phc.mu.Unlock()
	phc.healthCallback = callback
}

// SetIssueCallback sets a callback for health issue notifications
func (phc *PipelineHealthChecker) SetIssueCallback(callback func(HealthIssue)) {
	phc.mu.Lock()
	defer phc.mu.Unlock()
	phc.issueCallback = callback
}
