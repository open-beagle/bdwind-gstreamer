package gstreamer

import (
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/open-beagle/bdwind-gstreamer/internal/config"
)

// PipelineErrorHandler handles pipeline errors and implements recovery strategies
type PipelineErrorHandler struct {
	logger *logrus.Entry
	mu     sync.RWMutex

	// Configuration
	config *ErrorHandlerConfig

	// Error tracking
	errorHistory []ErrorEvent

	// Recovery strategies
	recoveryStrategies map[ErrorType]RecoveryStrategy
}

// ErrorHandlerConfig contains configuration for the error handler
type ErrorHandlerConfig struct {
	// Maximum retry attempts
	MaxRetryAttempts int

	// Delay between retry attempts
	RetryDelay time.Duration

	// Enable automatic recovery
	AutoRecovery bool

	// State manager reference for recovery actions
	StateManager *PipelineStateManager

	// Maximum error history entries to keep
	MaxErrorHistorySize int
}

// ErrorEvent represents an error event in the pipeline
type ErrorEvent struct {
	Type           ErrorType
	Severity       ErrorSeverity
	Message        string
	Details        string
	Timestamp      time.Time
	Source         string
	RecoveryAction RecoveryAction
	Resolved       bool
	ResolvedAt     time.Time
	RetryCount     int
}

// ErrorType represents the type of pipeline error
type ErrorType int

const (
	ErrorUnknown ErrorType = iota
	ErrorStateChange
	ErrorElementFailure
	ErrorResourceUnavailable
	ErrorFormatNegotiation
	ErrorDataFlow
	ErrorMemoryExhaustion
	ErrorTimeout
	ErrorPermission
	ErrorNetwork
	ErrorHardware
)

// String returns string representation of error type
func (et ErrorType) String() string {
	switch et {
	case ErrorStateChange:
		return "StateChange"
	case ErrorElementFailure:
		return "ElementFailure"
	case ErrorResourceUnavailable:
		return "ResourceUnavailable"
	case ErrorFormatNegotiation:
		return "FormatNegotiation"
	case ErrorDataFlow:
		return "DataFlow"
	case ErrorMemoryExhaustion:
		return "MemoryExhaustion"
	case ErrorTimeout:
		return "Timeout"
	case ErrorPermission:
		return "Permission"
	case ErrorNetwork:
		return "Network"
	case ErrorHardware:
		return "Hardware"
	default:
		return "Unknown"
	}
}

// ErrorSeverity represents the severity of an error
type ErrorSeverity int

const (
	SeverityLow ErrorSeverity = iota
	SeverityMedium
	SeverityHigh
	SeverityCritical
)

// String returns string representation of error severity
func (es ErrorSeverity) String() string {
	switch es {
	case SeverityLow:
		return "Low"
	case SeverityMedium:
		return "Medium"
	case SeverityHigh:
		return "High"
	case SeverityCritical:
		return "Critical"
	default:
		return "Unknown"
	}
}

// RecoveryAction represents the action taken to recover from an error
type RecoveryAction int

const (
	RecoveryNone RecoveryAction = iota
	RecoveryRetry
	RecoveryRestart
	RecoveryReconfigure
	RecoveryFallback
	RecoveryManualIntervention
)

// String returns string representation of recovery action
func (ra RecoveryAction) String() string {
	switch ra {
	case RecoveryRetry:
		return "Retry"
	case RecoveryRestart:
		return "Restart"
	case RecoveryReconfigure:
		return "Reconfigure"
	case RecoveryFallback:
		return "Fallback"
	case RecoveryManualIntervention:
		return "ManualIntervention"
	default:
		return "None"
	}
}

// RecoveryStrategy defines how to handle a specific type of error
type RecoveryStrategy struct {
	MaxRetries     int
	RetryDelay     time.Duration
	Actions        []RecoveryAction
	AutoRecover    bool
	EscalationTime time.Duration
}

// NewPipelineErrorHandler creates a new pipeline error handler
func NewPipelineErrorHandler(logger *logrus.Entry, errorConfig *ErrorHandlerConfig) *PipelineErrorHandler {
	if logger == nil {
		logger = config.GetLoggerWithPrefix("gstreamer-pipeline-error-handler")
	}

	if errorConfig == nil {
		errorConfig = &ErrorHandlerConfig{
			MaxRetryAttempts:    3,
			RetryDelay:          2 * time.Second,
			AutoRecovery:        true,
			MaxErrorHistorySize: 100,
		}
	}

	peh := &PipelineErrorHandler{
		logger:             logger,
		config:             errorConfig,
		errorHistory:       make([]ErrorEvent, 0, errorConfig.MaxErrorHistorySize),
		recoveryStrategies: make(map[ErrorType]RecoveryStrategy),
	}

	// Initialize default recovery strategies
	peh.initializeDefaultRecoveryStrategies()

	return peh
}

// initializeDefaultRecoveryStrategies sets up default recovery strategies for different error types
func (peh *PipelineErrorHandler) initializeDefaultRecoveryStrategies() {
	// State change errors - retry with increasing delays
	peh.recoveryStrategies[ErrorStateChange] = RecoveryStrategy{
		MaxRetries:     3,
		RetryDelay:     1 * time.Second,
		Actions:        []RecoveryAction{RecoveryRetry, RecoveryRestart},
		AutoRecover:    true,
		EscalationTime: 30 * time.Second,
	}

	// Element failure - restart pipeline
	peh.recoveryStrategies[ErrorElementFailure] = RecoveryStrategy{
		MaxRetries:     2,
		RetryDelay:     2 * time.Second,
		Actions:        []RecoveryAction{RecoveryRestart, RecoveryReconfigure},
		AutoRecover:    true,
		EscalationTime: 60 * time.Second,
	}

	// Resource unavailable - retry with backoff
	peh.recoveryStrategies[ErrorResourceUnavailable] = RecoveryStrategy{
		MaxRetries:     5,
		RetryDelay:     5 * time.Second,
		Actions:        []RecoveryAction{RecoveryRetry, RecoveryFallback},
		AutoRecover:    true,
		EscalationTime: 120 * time.Second,
	}

	// Format negotiation - reconfigure and retry
	peh.recoveryStrategies[ErrorFormatNegotiation] = RecoveryStrategy{
		MaxRetries:     2,
		RetryDelay:     1 * time.Second,
		Actions:        []RecoveryAction{RecoveryReconfigure, RecoveryRestart},
		AutoRecover:    true,
		EscalationTime: 30 * time.Second,
	}

	// Data flow issues - restart pipeline
	peh.recoveryStrategies[ErrorDataFlow] = RecoveryStrategy{
		MaxRetries:     3,
		RetryDelay:     3 * time.Second,
		Actions:        []RecoveryAction{RecoveryRestart},
		AutoRecover:    true,
		EscalationTime: 60 * time.Second,
	}

	// Memory exhaustion - restart with cleanup
	peh.recoveryStrategies[ErrorMemoryExhaustion] = RecoveryStrategy{
		MaxRetries:     1,
		RetryDelay:     5 * time.Second,
		Actions:        []RecoveryAction{RecoveryRestart},
		AutoRecover:    true,
		EscalationTime: 30 * time.Second,
	}

	// Timeout errors - retry with longer timeout
	peh.recoveryStrategies[ErrorTimeout] = RecoveryStrategy{
		MaxRetries:     2,
		RetryDelay:     2 * time.Second,
		Actions:        []RecoveryAction{RecoveryRetry, RecoveryRestart},
		AutoRecover:    true,
		EscalationTime: 45 * time.Second,
	}

	// Permission errors - manual intervention required
	peh.recoveryStrategies[ErrorPermission] = RecoveryStrategy{
		MaxRetries:     0,
		RetryDelay:     0,
		Actions:        []RecoveryAction{RecoveryManualIntervention},
		AutoRecover:    false,
		EscalationTime: 0,
	}

	// Network errors - retry with backoff
	peh.recoveryStrategies[ErrorNetwork] = RecoveryStrategy{
		MaxRetries:     5,
		RetryDelay:     3 * time.Second,
		Actions:        []RecoveryAction{RecoveryRetry, RecoveryFallback},
		AutoRecover:    true,
		EscalationTime: 180 * time.Second,
	}

	// Hardware errors - limited recovery options
	peh.recoveryStrategies[ErrorHardware] = RecoveryStrategy{
		MaxRetries:     1,
		RetryDelay:     5 * time.Second,
		Actions:        []RecoveryAction{RecoveryRestart, RecoveryManualIntervention},
		AutoRecover:    false,
		EscalationTime: 60 * time.Second,
	}
}

// HandleError handles a pipeline error and attempts recovery
func (peh *PipelineErrorHandler) HandleError(errorType ErrorType, message, details, source string) error {
	peh.logger.Debugf("Handling pipeline error: %s - %s", errorType.String(), message)

	// Create error event
	errorEvent := ErrorEvent{
		Type:      errorType,
		Severity:  peh.determineSeverity(errorType),
		Message:   message,
		Details:   details,
		Timestamp: time.Now(),
		Source:    source,
	}

	// Record error in history
	peh.recordError(errorEvent)

	// Determine if automatic recovery should be attempted
	if !peh.config.AutoRecovery {
		peh.logger.Infof("Automatic recovery disabled, manual intervention required")
		errorEvent.RecoveryAction = RecoveryManualIntervention
		peh.updateErrorEvent(errorEvent)
		return fmt.Errorf("pipeline error requires manual intervention: %s", message)
	}

	// Get recovery strategy for this error type
	strategy, exists := peh.recoveryStrategies[errorType]
	if !exists {
		peh.logger.Errorf("No recovery strategy defined for error type: %s", errorType.String())
		errorEvent.RecoveryAction = RecoveryManualIntervention
		peh.updateErrorEvent(errorEvent)
		return fmt.Errorf("no recovery strategy for error type %s: %s", errorType.String(), message)
	}

	// Attempt recovery
	return peh.attemptRecovery(errorEvent, strategy)
}

// attemptRecovery attempts to recover from an error using the specified strategy
func (peh *PipelineErrorHandler) attemptRecovery(errorEvent ErrorEvent, strategy RecoveryStrategy) error {
	if !strategy.AutoRecover {
		peh.logger.Infof("Automatic recovery not enabled for error type: %s", errorEvent.Type.String())
		errorEvent.RecoveryAction = RecoveryManualIntervention
		peh.updateErrorEvent(errorEvent)
		return fmt.Errorf("manual intervention required for error: %s", errorEvent.Message)
	}

	// Try each recovery action in sequence
	var lastErr error
	for _, action := range strategy.Actions {
		peh.logger.Debugf("Attempting recovery action: %s for error: %s", action.String(), errorEvent.Type.String())

		errorEvent.RecoveryAction = action
		peh.updateErrorEvent(errorEvent)

		// Attempt recovery with retries
		for attempt := 0; attempt < strategy.MaxRetries+1; attempt++ {
			if attempt > 0 {
				peh.logger.Debugf("Recovery attempt %d/%d for action: %s",
					attempt+1, strategy.MaxRetries+1, action.String())
				time.Sleep(strategy.RetryDelay)
			}

			errorEvent.RetryCount = attempt

			// Execute recovery action
			err := peh.executeRecoveryAction(action, errorEvent)
			if err == nil {
				// Recovery successful
				peh.logger.Infof("Recovery successful using action: %s", action.String())
				errorEvent.Resolved = true
				errorEvent.ResolvedAt = time.Now()
				peh.updateErrorEvent(errorEvent)
				return nil
			}

			lastErr = err
			peh.logger.Debugf("Recovery attempt %d failed: %v", attempt+1, err)
		}

		peh.logger.Debugf("Recovery action %s failed after %d attempts", action.String(), strategy.MaxRetries+1)
	}

	// All recovery actions failed
	peh.logger.Errorf("All recovery actions failed for error: %s", errorEvent.Type.String())
	errorEvent.RecoveryAction = RecoveryManualIntervention
	peh.updateErrorEvent(errorEvent)

	return fmt.Errorf("recovery failed for error %s after trying all strategies: %s (last error: %v)",
		errorEvent.Type.String(), errorEvent.Message, lastErr)
}

// executeRecoveryAction executes a specific recovery action
func (peh *PipelineErrorHandler) executeRecoveryAction(action RecoveryAction, errorEvent ErrorEvent) error {
	switch action {
	case RecoveryRetry:
		return peh.executeRetry(errorEvent)
	case RecoveryRestart:
		return peh.executeRestart(errorEvent)
	case RecoveryReconfigure:
		return peh.executeReconfigure(errorEvent)
	case RecoveryFallback:
		return peh.executeFallback(errorEvent)
	case RecoveryManualIntervention:
		return fmt.Errorf("manual intervention required")
	default:
		return fmt.Errorf("unknown recovery action: %s", action.String())
	}
}

// executeRetry executes a retry recovery action
func (peh *PipelineErrorHandler) executeRetry(errorEvent ErrorEvent) error {
	peh.logger.Debugf("Executing retry recovery for error: %s", errorEvent.Type.String())

	// For retry, we don't need to do anything special - the calling code
	// will retry the original operation
	return nil
}

// executeRestart executes a restart recovery action
func (peh *PipelineErrorHandler) executeRestart(errorEvent ErrorEvent) error {
	peh.logger.Debugf("Executing restart recovery for error: %s", errorEvent.Type.String())

	if peh.config.StateManager == nil {
		return fmt.Errorf("state manager not available for restart recovery")
	}

	// Restart the pipeline through the state manager
	return peh.config.StateManager.RestartPipeline()
}

// executeReconfigure executes a reconfigure recovery action
func (peh *PipelineErrorHandler) executeReconfigure(errorEvent ErrorEvent) error {
	peh.logger.Debugf("Executing reconfigure recovery for error: %s", errorEvent.Type.String())

	// This is a placeholder for reconfiguration logic
	// In a real implementation, you would adjust pipeline configuration
	// based on the specific error type and context

	peh.logger.Debugf("Reconfigure recovery not fully implemented yet")
	return fmt.Errorf("reconfigure recovery not implemented")
}

// executeFallback executes a fallback recovery action
func (peh *PipelineErrorHandler) executeFallback(errorEvent ErrorEvent) error {
	peh.logger.Debugf("Executing fallback recovery for error: %s", errorEvent.Type.String())

	// This is a placeholder for fallback logic
	// In a real implementation, you would switch to alternative
	// configurations or elements based on the error type

	peh.logger.Debugf("Fallback recovery not fully implemented yet")
	return fmt.Errorf("fallback recovery not implemented")
}

// determineSeverity determines the severity of an error based on its type
func (peh *PipelineErrorHandler) determineSeverity(errorType ErrorType) ErrorSeverity {
	switch errorType {
	case ErrorPermission, ErrorHardware:
		return SeverityCritical
	case ErrorElementFailure, ErrorMemoryExhaustion:
		return SeverityHigh
	case ErrorStateChange, ErrorDataFlow, ErrorTimeout:
		return SeverityMedium
	case ErrorFormatNegotiation, ErrorNetwork:
		return SeverityLow
	default:
		return SeverityMedium
	}
}

// recordError records an error event in the history
func (peh *PipelineErrorHandler) recordError(errorEvent ErrorEvent) {
	peh.mu.Lock()
	defer peh.mu.Unlock()

	peh.errorHistory = append(peh.errorHistory, errorEvent)

	// Trim history if it exceeds maximum size
	if len(peh.errorHistory) > peh.config.MaxErrorHistorySize {
		peh.errorHistory = peh.errorHistory[1:]
	}
}

// updateErrorEvent updates an existing error event in the history
func (peh *PipelineErrorHandler) updateErrorEvent(errorEvent ErrorEvent) {
	peh.mu.Lock()
	defer peh.mu.Unlock()

	// Find and update the most recent matching error event
	for i := len(peh.errorHistory) - 1; i >= 0; i-- {
		if peh.errorHistory[i].Timestamp == errorEvent.Timestamp &&
			peh.errorHistory[i].Type == errorEvent.Type {
			peh.errorHistory[i] = errorEvent
			break
		}
	}
}

// GetErrorHistory returns the error history
func (peh *PipelineErrorHandler) GetErrorHistory() []ErrorEvent {
	peh.mu.RLock()
	defer peh.mu.RUnlock()

	// Return a copy to prevent external modification
	history := make([]ErrorEvent, len(peh.errorHistory))
	copy(history, peh.errorHistory)
	return history
}

// GetUnresolvedErrors returns errors that haven't been resolved
func (peh *PipelineErrorHandler) GetUnresolvedErrors() []ErrorEvent {
	peh.mu.RLock()
	defer peh.mu.RUnlock()

	var unresolved []ErrorEvent
	for _, errorEvent := range peh.errorHistory {
		if !errorEvent.Resolved {
			unresolved = append(unresolved, errorEvent)
		}
	}

	return unresolved
}

// SetRecoveryStrategy sets a custom recovery strategy for an error type
func (peh *PipelineErrorHandler) SetRecoveryStrategy(errorType ErrorType, strategy RecoveryStrategy) {
	peh.mu.Lock()
	defer peh.mu.Unlock()

	peh.recoveryStrategies[errorType] = strategy
	peh.logger.Debugf("Updated recovery strategy for error type: %s", errorType.String())
}

// GetRecoveryStrategy gets the recovery strategy for an error type
func (peh *PipelineErrorHandler) GetRecoveryStrategy(errorType ErrorType) (RecoveryStrategy, bool) {
	peh.mu.RLock()
	defer peh.mu.RUnlock()

	strategy, exists := peh.recoveryStrategies[errorType]
	return strategy, exists
}

// GetStats returns statistics about error handling
func (peh *PipelineErrorHandler) GetStats() map[string]interface{} {
	peh.mu.RLock()
	defer peh.mu.RUnlock()

	stats := map[string]interface{}{
		"total_errors": len(peh.errorHistory),
	}

	// Count errors by type and severity
	errorsByType := make(map[string]int)
	errorsBySeverity := make(map[string]int)
	resolvedCount := 0

	for _, errorEvent := range peh.errorHistory {
		errorsByType[errorEvent.Type.String()]++
		errorsBySeverity[errorEvent.Severity.String()]++
		if errorEvent.Resolved {
			resolvedCount++
		}
	}

	stats["errors_by_type"] = errorsByType
	stats["errors_by_severity"] = errorsBySeverity
	stats["resolved_errors"] = resolvedCount
	stats["unresolved_errors"] = len(peh.errorHistory) - resolvedCount

	if len(peh.errorHistory) > 0 {
		stats["resolution_rate"] = float64(resolvedCount) / float64(len(peh.errorHistory))
	}

	return stats
}
