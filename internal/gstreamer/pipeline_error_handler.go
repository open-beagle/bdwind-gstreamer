package gstreamer

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// PipelineErrorHandlerConfig holds configuration for the pipeline error handler
type PipelineErrorHandlerConfig struct {
	// Maximum number of retry attempts for recoverable errors
	MaxRetryAttempts int

	// Base delay between retry attempts
	RetryDelay time.Duration

	// Maximum delay between retry attempts (for exponential backoff)
	MaxRetryDelay time.Duration

	// Enable automatic recovery for recoverable errors
	AutoRecovery bool

	// Maximum number of error events to keep in history
	MaxErrorHistorySize int

	// Error escalation timeout - how long to wait before escalating
	EscalationTimeout time.Duration

	// Component restart threshold - number of errors before suggesting restart
	ComponentRestartThreshold int

	// Time window for counting errors towards restart threshold
	RestartThresholdWindow time.Duration

	// Enable detailed error logging
	EnableDetailedLogging bool

	// Enable error notifications
	EnableNotifications bool
}

// DefaultPipelineErrorHandlerConfig returns default error handler configuration
func DefaultPipelineErrorHandlerConfig() *PipelineErrorHandlerConfig {
	return &PipelineErrorHandlerConfig{
		MaxRetryAttempts:          3,
		RetryDelay:                2 * time.Second,
		MaxRetryDelay:             30 * time.Second,
		AutoRecovery:              true,
		MaxErrorHistorySize:       100,
		EscalationTimeout:         5 * time.Minute,
		ComponentRestartThreshold: 5,
		RestartThresholdWindow:    10 * time.Minute,
		EnableDetailedLogging:     true,
		EnableNotifications:       true,
	}
}

// PipelineErrorHandler handles pipeline errors and recovery
type PipelineErrorHandler struct {
	// Configuration
	config *PipelineErrorHandlerConfig
	logger *logrus.Entry

	// Error tracking
	mu           sync.RWMutex
	errorHistory []ErrorEvent

	// Recovery management
	recoveryStrategies map[ErrorType]RecoveryStrategy
	activeRecoveries   map[string]*RecoveryContext

	// Component error tracking
	componentErrors map[string][]ErrorEvent

	// Metrics
	metrics   *PipelineErrorHandlerMetrics
	metricsMu sync.RWMutex

	// Event handlers
	errorHandlers    []func(ErrorEvent) bool
	recoveryHandlers []func(RecoveryContext) bool

	// State manager reference for recovery actions
	stateManager *PipelineStateManager

	// Context and lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// PipelineErrorHandlerMetrics tracks error handler performance
type PipelineErrorHandlerMetrics struct {
	TotalErrors          int64
	ErrorsByType         map[ErrorType]int64
	ErrorsByComponent    map[string]int64
	RecoveryAttempts     int64
	SuccessfulRecoveries int64
	FailedRecoveries     int64
	ComponentRestarts    int64
	AverageRecoveryTime  time.Duration
	LastErrorTime        time.Time
	LastRecoveryTime     time.Time
}

// NewPipelineErrorHandler creates a new pipeline error handler
func NewPipelineErrorHandler(logger *logrus.Entry, config *PipelineErrorHandlerConfig) *PipelineErrorHandler {
	if config == nil {
		config = DefaultPipelineErrorHandlerConfig()
	}

	if logger == nil {
		logger = logrus.WithField("component", "pipeline-error-handler")
	}

	peh := &PipelineErrorHandler{
		config:             config,
		logger:             logger,
		errorHistory:       make([]ErrorEvent, 0, config.MaxErrorHistorySize),
		recoveryStrategies: make(map[ErrorType]RecoveryStrategy),
		activeRecoveries:   make(map[string]*RecoveryContext),
		componentErrors:    make(map[string][]ErrorEvent),
		metrics: &PipelineErrorHandlerMetrics{
			ErrorsByType:      make(map[ErrorType]int64),
			ErrorsByComponent: make(map[string]int64),
		},
		errorHandlers:    make([]func(ErrorEvent) bool, 0),
		recoveryHandlers: make([]func(RecoveryContext) bool, 0),
	}

	// Initialize default recovery strategies
	peh.initializeDefaultRecoveryStrategies()

	return peh
}

// initializeDefaultRecoveryStrategies sets up default recovery strategies for different error types
func (peh *PipelineErrorHandler) initializeDefaultRecoveryStrategies() {
	// Temporarily simplified - use restart recovery for all error types
	restartStrategy := NewRestartRecoveryStrategy(3 * time.Second)

	peh.recoveryStrategies[ErrorStateChange] = restartStrategy
	peh.recoveryStrategies[ErrorElementFailure] = restartStrategy
	peh.recoveryStrategies[ErrorResourceUnavailable] = restartStrategy
	peh.recoveryStrategies[ErrorFormatNegotiation] = restartStrategy
	peh.recoveryStrategies[ErrorTimeout] = restartStrategy
	peh.recoveryStrategies[ErrorPermission] = restartStrategy
	peh.recoveryStrategies[ErrorMemoryExhaustion] = restartStrategy
	peh.recoveryStrategies[ErrorNetwork] = restartStrategy
	peh.recoveryStrategies[ErrorHardware] = restartStrategy
	peh.recoveryStrategies[ErrorUnknown] = restartStrategy

}

// Start starts the error handler
func (peh *PipelineErrorHandler) Start(ctx context.Context) error {
	peh.ctx, peh.cancel = context.WithCancel(ctx)

	peh.logger.Info("Starting pipeline error handler")

	// Start recovery monitoring loop
	peh.wg.Add(1)
	go peh.recoveryMonitorLoop()

	peh.logger.Info("Pipeline error handler started successfully")
	return nil
}

// Stop stops the error handler
func (peh *PipelineErrorHandler) Stop() error {
	peh.logger.Info("Stopping pipeline error handler")

	if peh.cancel != nil {
		peh.cancel()
	}

	// Cancel all active recoveries
	peh.mu.Lock()
	for _, recovery := range peh.activeRecoveries {
		if recovery.cancel != nil {
			recovery.cancel()
		}
	}
	peh.mu.Unlock()

	peh.wg.Wait()

	peh.logger.Info("Pipeline error handler stopped successfully")
	return nil
}

// SetStateManager sets the state manager reference for recovery actions
func (peh *PipelineErrorHandler) SetStateManager(stateManager *PipelineStateManager) {
	peh.mu.Lock()
	defer peh.mu.Unlock()
	peh.stateManager = stateManager
}

// HandleError handles a pipeline error
func (peh *PipelineErrorHandler) HandleError(errorType ErrorType, message, details, component string) {
	// Create error event
	errorEvent := ErrorEvent{
		Type:      GStreamerErrorType(errorType),
		Message:   message,
		Details:   details,
		Source:    component,
		Timestamp: time.Now(),
	}

	// Record error
	peh.recordError(errorEvent)

	// Update metrics
	peh.updateErrorMetrics(errorEvent)

	// Log error
	peh.logError(errorEvent)

	// Notify error handlers
	peh.notifyErrorHandlers(errorEvent)

	// Attempt recovery if enabled and strategy exists
	if peh.config.AutoRecovery {
		if strategy, exists := peh.recoveryStrategies[errorType]; exists {
			go peh.attemptRecovery(errorEvent, strategy)
		}
	}

	// Check if component should be restarted
	if peh.shouldRestartComponent(component) {
		peh.logger.Warnf("Component %s has exceeded error threshold, restart recommended", component)
		peh.metricsMu.Lock()
		peh.metrics.ComponentRestarts++
		peh.metricsMu.Unlock()
	}
}

// attemptRecovery attempts to recover from an error
func (peh *PipelineErrorHandler) attemptRecovery(errorEvent ErrorEvent, strategy RecoveryStrategy) {
	errorID := peh.generateErrorID(errorEvent)

	// Check if recovery is already in progress for this error
	peh.mu.RLock()
	if _, exists := peh.activeRecoveries[errorID]; exists {
		peh.mu.RUnlock()
		return
	}
	peh.mu.RUnlock()

	// Create recovery context
	ctx, cancel := context.WithTimeout(peh.ctx, strategy.GetRecoveryTime())
	recovery := &RecoveryContext{
		ErrorID:   errorID,
		Component: errorEvent.Source,
		StartTime: time.Now(),
		Attempts:  make([]RecoveryAttempt, 0),
		ctx:       ctx,
		cancel:    cancel,
	}

	// Register active recovery
	peh.mu.Lock()
	peh.activeRecoveries[errorID] = recovery
	peh.mu.Unlock()

	peh.logger.Infof("Starting recovery for error %s (type: %s, component: %s)",
		errorID, errorEvent.Type.String(), errorEvent.Source)

	// Update metrics
	peh.metricsMu.Lock()
	peh.metrics.RecoveryAttempts++
	peh.metrics.LastRecoveryTime = time.Now()
	peh.metricsMu.Unlock()

	// Execute recovery
	success := peh.executeRecovery(recovery, strategy)

	// Update recovery status
	endTime := time.Now()
	if success {
		peh.metricsMu.Lock()
		peh.metrics.SuccessfulRecoveries++
		peh.metricsMu.Unlock()
		peh.logger.Infof("Recovery successful for error %s", errorID)
	} else {
		peh.metricsMu.Lock()
		peh.metrics.FailedRecoveries++
		peh.metricsMu.Unlock()
		peh.logger.Errorf("Recovery failed for error %s", errorID)
	}

	// Update average recovery time
	peh.updateRecoveryMetrics(recovery, endTime)

	// Notify recovery handlers
	peh.notifyRecoveryHandlers(*recovery)

	// Clean up
	peh.mu.Lock()
	delete(peh.activeRecoveries, errorID)
	peh.mu.Unlock()

	cancel()
}

// executeRecovery executes the recovery strategy
func (peh *PipelineErrorHandler) executeRecovery(recovery *RecoveryContext, strategy RecoveryStrategy) bool {
	// Simplified recovery execution using the RecoveryStrategy interface
	// Create a dummy ErrorEvent for the strategy
	errorEvent := &ErrorEvent{
		Source:    recovery.Component,
		Timestamp: time.Now(),
	}

	// Execute the recovery strategy
	err := strategy.Recover(recovery.ctx, errorEvent)

	// Record the attempt
	recoveryAttempt := RecoveryAttempt{
		Timestamp: time.Now(),
		Success:   err == nil,
		Error:     err,
		Duration:  strategy.GetRecoveryTime(),
	}

	recovery.Attempts = append(recovery.Attempts, recoveryAttempt)

	if err == nil {
		peh.logger.Debugf("Recovery successful for error %s", recovery.ErrorID)
		return true
	}

	peh.logger.Debugf("Recovery failed for error %s: %v", recovery.ErrorID, err)
	return false
}

// executeRecoveryAction executes a specific recovery action
func (peh *PipelineErrorHandler) executeRecoveryAction(ctx context.Context, action RecoveryAction, component string) error {
	switch action {
	case RecoveryRetry:
		// For retry, we don't need to do anything special
		// The calling code will retry the original operation
		return nil

	case RecoveryRestart:
		// Restart the pipeline through state manager
		if peh.stateManager != nil {
			return peh.stateManager.RestartPipeline()
		}
		return fmt.Errorf("no state manager available for restart")

	case RecoveryReconfigure:
		// This would involve reconfiguring the component
		// Implementation depends on the specific component
		peh.logger.Infof("Reconfiguration requested for component %s", component)
		return fmt.Errorf("reconfiguration not implemented")

	case RecoveryFallback:
		// This would involve switching to fallback configuration
		peh.logger.Infof("Fallback configuration requested for component %s", component)
		return fmt.Errorf("fallback not implemented")

	case RecoveryManualIntervention:
		// Escalate to higher level error handling
		peh.logger.Warnf("Error escalated for component %s - manual intervention may be required", component)
		return nil

	default:
		return fmt.Errorf("unknown recovery action: %s", action.String())
	}
}

// Helper methods

func (peh *PipelineErrorHandler) recordError(errorEvent ErrorEvent) {
	peh.mu.Lock()
	defer peh.mu.Unlock()

	// Add to history
	peh.errorHistory = append(peh.errorHistory, errorEvent)

	// Trim history if needed
	if len(peh.errorHistory) > peh.config.MaxErrorHistorySize {
		peh.errorHistory = peh.errorHistory[1:]
	}

	// Add to component errors
	if peh.componentErrors[errorEvent.Source] == nil {
		peh.componentErrors[errorEvent.Source] = make([]ErrorEvent, 0)
	}
	peh.componentErrors[errorEvent.Source] = append(peh.componentErrors[errorEvent.Source], errorEvent)
}

func (peh *PipelineErrorHandler) updateErrorMetrics(errorEvent ErrorEvent) {
	peh.metricsMu.Lock()
	defer peh.metricsMu.Unlock()

	peh.metrics.TotalErrors++
	// Convert GStreamerErrorType to ErrorType for metrics
	if errorType, ok := convertGStreamerErrorType(errorEvent.Type); ok {
		peh.metrics.ErrorsByType[errorType]++
	}
	peh.metrics.ErrorsByComponent[errorEvent.Source]++
	peh.metrics.LastErrorTime = errorEvent.Timestamp
}

func (peh *PipelineErrorHandler) updateRecoveryMetrics(recovery *RecoveryContext, endTime time.Time) {
	peh.metricsMu.Lock()
	defer peh.metricsMu.Unlock()

	recoveryDuration := endTime.Sub(recovery.StartTime)

	// Update average recovery time
	totalRecoveries := peh.metrics.SuccessfulRecoveries + peh.metrics.FailedRecoveries
	if totalRecoveries == 1 {
		peh.metrics.AverageRecoveryTime = recoveryDuration
	} else {
		total := peh.metrics.AverageRecoveryTime * time.Duration(totalRecoveries-1)
		peh.metrics.AverageRecoveryTime = (total + recoveryDuration) / time.Duration(totalRecoveries)
	}
}

func (peh *PipelineErrorHandler) logError(errorEvent ErrorEvent) {
	severity := peh.getErrorSeverity(errorEvent.Type)

	logEntry := peh.logger.WithFields(logrus.Fields{
		"error_type": errorEvent.Type.String(),
		"component":  errorEvent.Source,
		"severity":   severity,
	})

	switch severity {
	case "Info":
		logEntry.Info(errorEvent.Message)
	case "Warning":
		logEntry.Warn(errorEvent.Message)
	case "Error":
		logEntry.Error(errorEvent.Message)
	case "Critical", "Fatal":
		logEntry.Error(errorEvent.Message)
	default:
		logEntry.Error(errorEvent.Message)
	}
}

func (peh *PipelineErrorHandler) notifyErrorHandlers(errorEvent ErrorEvent) {
	for _, handler := range peh.errorHandlers {
		if !handler(errorEvent) {
			break
		}
	}
}

func (peh *PipelineErrorHandler) notifyRecoveryHandlers(recovery RecoveryContext) {
	for _, handler := range peh.recoveryHandlers {
		if !handler(recovery) {
			break
		}
	}
}

func (peh *PipelineErrorHandler) shouldRestartComponent(component string) bool {
	peh.mu.RLock()
	defer peh.mu.RUnlock()

	errors, exists := peh.componentErrors[component]
	if !exists {
		return false
	}

	// Count recent errors within the threshold window
	recentErrors := 0
	cutoff := time.Now().Add(-peh.config.RestartThresholdWindow)

	for _, err := range errors {
		if err.Timestamp.After(cutoff) {
			recentErrors++
		}
	}

	return recentErrors >= peh.config.ComponentRestartThreshold
}

func (peh *PipelineErrorHandler) calculateBackoffDelay(baseDelay time.Duration, attempt int) time.Duration {
	delay := baseDelay * time.Duration(1<<uint(attempt)) // Exponential backoff
	if delay > peh.config.MaxRetryDelay {
		delay = peh.config.MaxRetryDelay
	}
	return delay
}

func (peh *PipelineErrorHandler) generateErrorID(errorEvent ErrorEvent) string {
	return fmt.Sprintf("%s_%s_%d", errorEvent.Type.String(), errorEvent.Source, errorEvent.Timestamp.Unix())
}

// convertGStreamerErrorType converts GStreamerErrorType to ErrorType
func convertGStreamerErrorType(gstType GStreamerErrorType) (ErrorType, bool) {
	switch gstType {
	case ErrorTypePipelineCreation, ErrorTypePipelineState:
		return ErrorTypePipeline, true
	case ErrorTypeElementCreation, ErrorTypeElementLinking, ErrorTypeElementProperty:
		return ErrorTypeElement, true
	case ErrorTypeResourceAccess:
		return ErrorTypeResource, true
	case ErrorTypeMemoryAllocation:
		return ErrorMemoryExhaustion, true
	case ErrorTypeNetwork:
		return ErrorNetwork, true
	case ErrorTypeHardware:
		return ErrorHardware, true
	case ErrorTypeTimeout:
		return ErrorTimeout, true
	case ErrorTypePermission:
		return ErrorPermission, true
	default:
		return ErrorTypeGeneral, true
	}
}

func (peh *PipelineErrorHandler) getErrorSeverity(errorType GStreamerErrorType) string {
	switch errorType {
	case ErrorTypePermission, ErrorTypeHardware:
		return "Fatal"
	case ErrorTypeMemoryAllocation:
		return "Critical"
	case ErrorTypeElementCreation, ErrorTypeResourceAccess:
		return "Error"
	case ErrorTypeFormatNegotiation, ErrorTypeTimeout, ErrorTypeNetwork:
		return "Warning"
	default:
		return "Info"
	}
}

func (peh *PipelineErrorHandler) recoveryMonitorLoop() {
	defer peh.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-peh.ctx.Done():
			return
		case <-ticker.C:
			peh.cleanupStaleRecoveries()
		}
	}
}

func (peh *PipelineErrorHandler) cleanupStaleRecoveries() {
	peh.mu.Lock()
	defer peh.mu.Unlock()

	now := time.Now()
	for errorID, recovery := range peh.activeRecoveries {
		// Clean up recoveries that have been running too long
		if now.Sub(recovery.StartTime) > 10*time.Minute {
			peh.logger.Warnf("Cleaning up stale recovery for error %s", errorID)
			if recovery.cancel != nil {
				recovery.cancel()
			}
			delete(peh.activeRecoveries, errorID)
		}
	}
}

// Public API methods

// GetErrorHistory returns the error history
func (peh *PipelineErrorHandler) GetErrorHistory() []ErrorEvent {
	peh.mu.RLock()
	defer peh.mu.RUnlock()

	history := make([]ErrorEvent, len(peh.errorHistory))
	copy(history, peh.errorHistory)
	return history
}

// GetComponentErrors returns errors for a specific component
func (peh *PipelineErrorHandler) GetComponentErrors(component string) []ErrorEvent {
	peh.mu.RLock()
	defer peh.mu.RUnlock()

	errors, exists := peh.componentErrors[component]
	if !exists {
		return nil
	}

	result := make([]ErrorEvent, len(errors))
	copy(result, errors)
	return result
}

// GetActiveRecoveries returns currently active recovery operations
func (peh *PipelineErrorHandler) GetActiveRecoveries() map[string]RecoveryContext {
	peh.mu.RLock()
	defer peh.mu.RUnlock()

	result := make(map[string]RecoveryContext)
	for id, recovery := range peh.activeRecoveries {
		result[id] = *recovery
	}
	return result
}

// GetMetrics returns error handler metrics
func (peh *PipelineErrorHandler) GetMetrics() *PipelineErrorHandlerMetrics {
	peh.metricsMu.RLock()
	defer peh.metricsMu.RUnlock()

	// Create a copy of metrics
	metrics := &PipelineErrorHandlerMetrics{
		TotalErrors:          peh.metrics.TotalErrors,
		RecoveryAttempts:     peh.metrics.RecoveryAttempts,
		SuccessfulRecoveries: peh.metrics.SuccessfulRecoveries,
		FailedRecoveries:     peh.metrics.FailedRecoveries,
		ComponentRestarts:    peh.metrics.ComponentRestarts,
		AverageRecoveryTime:  peh.metrics.AverageRecoveryTime,
		LastErrorTime:        peh.metrics.LastErrorTime,
		LastRecoveryTime:     peh.metrics.LastRecoveryTime,
		ErrorsByType:         make(map[ErrorType]int64),
		ErrorsByComponent:    make(map[string]int64),
	}

	for k, v := range peh.metrics.ErrorsByType {
		metrics.ErrorsByType[k] = v
	}
	for k, v := range peh.metrics.ErrorsByComponent {
		metrics.ErrorsByComponent[k] = v
	}

	return metrics
}

// GetStats returns comprehensive error handler statistics
func (peh *PipelineErrorHandler) GetStats() interface{} {
	metrics := peh.GetMetrics()
	activeRecoveries := peh.GetActiveRecoveries()

	return map[string]interface{}{
		"metrics":            metrics,
		"active_recoveries":  len(activeRecoveries),
		"error_history_size": len(peh.errorHistory),
		"component_count":    len(peh.componentErrors),
	}
}

// AddErrorHandler adds an error event handler
func (peh *PipelineErrorHandler) AddErrorHandler(handler func(ErrorEvent) bool) {
	peh.mu.Lock()
	defer peh.mu.Unlock()
	peh.errorHandlers = append(peh.errorHandlers, handler)
}

// AddRecoveryHandler adds a recovery event handler
func (peh *PipelineErrorHandler) AddRecoveryHandler(handler func(RecoveryContext) bool) {
	peh.mu.Lock()
	defer peh.mu.Unlock()
	peh.recoveryHandlers = append(peh.recoveryHandlers, handler)
}

// SetRecoveryStrategy sets a custom recovery strategy for an error type
func (peh *PipelineErrorHandler) SetRecoveryStrategy(errorType ErrorType, strategy RecoveryStrategy) {
	peh.mu.Lock()
	defer peh.mu.Unlock()
	peh.recoveryStrategies[errorType] = strategy
}

// SetErrorCallback sets the error callback
func (peh *PipelineErrorHandler) SetErrorCallback(callback func(ErrorType, string, error)) {
	peh.mu.Lock()
	defer peh.mu.Unlock()
	// Store the callback for use in error handling
	peh.AddErrorHandler(func(errorEvent ErrorEvent) bool {
		if callback != nil {
			if errorType, ok := convertGStreamerErrorType(errorEvent.Type); ok {
				callback(errorType, errorEvent.Message, fmt.Errorf("%s", errorEvent.Message))
			}
		}
		return true // Continue processing other handlers
	})
}

// GetRecoveryStrategy gets the recovery strategy for an error type
func (peh *PipelineErrorHandler) GetRecoveryStrategy(errorType ErrorType) (RecoveryStrategy, bool) {
	peh.mu.RLock()
	defer peh.mu.RUnlock()
	strategy, exists := peh.recoveryStrategies[errorType]
	return strategy, exists
}
