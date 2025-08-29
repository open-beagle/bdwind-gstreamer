package gstreamer

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// GStreamerError represents a unified GStreamer error with context and recovery information
type GStreamerError struct {
	// Core error information
	Type     GStreamerErrorType
	Severity GStreamerErrorSeverity
	Code     string
	Message  string
	Details  string
	Cause    error

	// Context information
	Component  string
	Operation  string
	Timestamp  time.Time
	StackTrace []string

	// Recovery information
	Recoverable bool
	Suggested   []RecoveryAction
	Attempted   []RecoveryAttempt

	// Metadata
	Metadata map[string]interface{}
}

// GStreamerErrorType defines the category of GStreamer errors
type GStreamerErrorType int

const (
	ErrorTypeUnknown GStreamerErrorType = iota
	ErrorTypeInitialization
	ErrorTypePipelineCreation
	ErrorTypePipelineState
	ErrorTypeElementCreation
	ErrorTypeElementLinking
	ErrorTypeElementProperty
	ErrorTypeDataFlow
	ErrorTypeMemoryAllocation
	ErrorTypeResourceAccess
	ErrorTypeFormatNegotiation
	ErrorTypeTimeout
	ErrorTypePermission
	ErrorTypeNetwork
	ErrorTypeHardware
	ErrorTypeConfiguration
	ErrorTypeValidation
)

// String returns the string representation of GStreamerErrorType
func (et GStreamerErrorType) String() string {
	switch et {
	case ErrorTypeInitialization:
		return "Initialization"
	case ErrorTypePipelineCreation:
		return "PipelineCreation"
	case ErrorTypePipelineState:
		return "PipelineState"
	case ErrorTypeElementCreation:
		return "ElementCreation"
	case ErrorTypeElementLinking:
		return "ElementLinking"
	case ErrorTypeElementProperty:
		return "ElementProperty"
	case ErrorTypeDataFlow:
		return "DataFlow"
	case ErrorTypeMemoryAllocation:
		return "MemoryAllocation"
	case ErrorTypeResourceAccess:
		return "ResourceAccess"
	case ErrorTypeFormatNegotiation:
		return "FormatNegotiation"
	case ErrorTypeTimeout:
		return "Timeout"
	case ErrorTypePermission:
		return "Permission"
	case ErrorTypeNetwork:
		return "Network"
	case ErrorTypeHardware:
		return "Hardware"
	case ErrorTypeConfiguration:
		return "Configuration"
	case ErrorTypeValidation:
		return "Validation"
	default:
		return "Unknown"
	}
}

// GStreamerErrorSeverity defines the severity level of errors
type GStreamerErrorSeverity int

const (
	UnifiedSeverityInfo GStreamerErrorSeverity = iota
	UnifiedSeverityWarning
	UnifiedSeverityError
	UnifiedSeverityCritical
	UnifiedSeverityFatal
)

// String returns the string representation of GStreamerErrorSeverity
func (es GStreamerErrorSeverity) String() string {
	switch es {
	case UnifiedSeverityInfo:
		return "Info"
	case UnifiedSeverityWarning:
		return "Warning"
	case UnifiedSeverityError:
		return "Error"
	case UnifiedSeverityCritical:
		return "Critical"
	case UnifiedSeverityFatal:
		return "Fatal"
	default:
		return "Unknown"
	}
}

// RecoveryAction defines the type of recovery action to take
type RecoveryAction int

const (
	RecoveryRetry RecoveryAction = iota
	RecoveryRestart
	RecoveryReconfigure
	RecoveryFallback
	RecoveryManualIntervention
	RecoveryNone
)

// String returns the string representation of RecoveryAction
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
	case RecoveryNone:
		return "None"
	default:
		return "Unknown"
	}
}

// RecoveryStrategy defines how to recover from a specific error type
type RecoveryStrategy struct {
	MaxRetries     int
	RetryDelay     time.Duration
	Actions        []RecoveryAction
	AutoRecover    bool
	EscalationTime time.Duration
}

// RecoveryAttempt represents an attempted recovery action
type RecoveryAttempt struct {
	Action    RecoveryAction
	Timestamp time.Time
	Success   bool
	Error     error
	Duration  time.Duration
}

// Error implements the error interface
func (ge *GStreamerError) Error() string {
	if ge.Details != "" {
		return fmt.Sprintf("[%s:%s] %s: %s - %s",
			ge.Type.String(), ge.Severity.String(), ge.Component, ge.Message, ge.Details)
	}
	return fmt.Sprintf("[%s:%s] %s: %s",
		ge.Type.String(), ge.Severity.String(), ge.Component, ge.Message)
}

// Unwrap returns the underlying cause error
func (ge *GStreamerError) Unwrap() error {
	return ge.Cause
}

// IsRecoverable returns true if the error can be recovered from
func (ge *GStreamerError) IsRecoverable() bool {
	return ge.Recoverable && len(ge.Suggested) > 0
}

// IsCritical returns true if the error is critical or fatal
func (ge *GStreamerError) IsCritical() bool {
	return ge.Severity >= UnifiedSeverityCritical
}

// ErrorHandler provides unified error handling for GStreamer components
type ErrorHandler struct {
	logger *logrus.Entry
	mu     sync.RWMutex

	// Configuration
	config *UnifiedErrorHandlerConfig

	// Error tracking
	errorHistory    []GStreamerError
	componentErrors map[string][]GStreamerError

	// Recovery management
	recoveryStrategies map[GStreamerErrorType]RecoveryStrategy
	activeRecoveries   map[string]*RecoveryContext

	// Metrics
	metrics *ErrorMetrics

	// Event handlers
	errorHandlers []ErrorEventHandler
}

// UnifiedErrorHandlerConfig contains configuration for the unified error handler
type UnifiedErrorHandlerConfig struct {
	// Maximum number of errors to keep in history
	MaxErrorHistory int

	// Maximum number of recovery attempts per error
	MaxRecoveryAttempts int

	// Base delay between recovery attempts
	BaseRecoveryDelay time.Duration

	// Maximum delay between recovery attempts
	MaxRecoveryDelay time.Duration

	// Enable automatic recovery
	AutoRecovery bool

	// Enable error propagation to parent components
	PropagateErrors bool

	// Component restart threshold (number of errors before restart)
	ComponentRestartThreshold int

	// Error escalation timeout
	EscalationTimeout time.Duration

	// Enable detailed stack traces
	EnableStackTraces bool
}

// RecoveryContext tracks the state of an ongoing recovery operation
type RecoveryContext struct {
	ErrorID       string
	Component     string
	StartTime     time.Time
	Attempts      []RecoveryAttempt
	CurrentAction RecoveryAction
	Cancelled     bool
	ctx           context.Context
	cancel        context.CancelFunc
}

// ErrorMetrics tracks error statistics
type ErrorMetrics struct {
	mu sync.RWMutex

	TotalErrors       int64
	ErrorsByType      map[GStreamerErrorType]int64
	ErrorsBySeverity  map[GStreamerErrorSeverity]int64
	ErrorsByComponent map[string]int64

	RecoveryAttempts  int64
	RecoverySuccesses int64
	RecoveryFailures  int64

	ComponentRestarts int64
	LastErrorTime     time.Time
}

// ErrorEventHandler is called when errors occur
type ErrorEventHandler func(error *GStreamerError) bool

// NewErrorHandler creates a new unified error handler
func NewErrorHandler(logger *logrus.Entry, config *UnifiedErrorHandlerConfig) *ErrorHandler {
	if logger == nil {
		logger = logrus.WithField("component", "gstreamer-error-handler")
	}

	if config == nil {
		config = &UnifiedErrorHandlerConfig{
			MaxErrorHistory:           1000,
			MaxRecoveryAttempts:       3,
			BaseRecoveryDelay:         time.Second,
			MaxRecoveryDelay:          30 * time.Second,
			AutoRecovery:              true,
			PropagateErrors:           false,
			ComponentRestartThreshold: 5,
			EscalationTimeout:         5 * time.Minute,
			EnableStackTraces:         true,
		}
	}

	eh := &ErrorHandler{
		logger:             logger,
		config:             config,
		errorHistory:       make([]GStreamerError, 0, config.MaxErrorHistory),
		componentErrors:    make(map[string][]GStreamerError),
		recoveryStrategies: make(map[GStreamerErrorType]RecoveryStrategy),
		activeRecoveries:   make(map[string]*RecoveryContext),
		metrics: &ErrorMetrics{
			ErrorsByType:      make(map[GStreamerErrorType]int64),
			ErrorsBySeverity:  make(map[GStreamerErrorSeverity]int64),
			ErrorsByComponent: make(map[string]int64),
		},
		errorHandlers: make([]ErrorEventHandler, 0),
	}

	// Initialize default recovery strategies
	eh.initializeDefaultRecoveryStrategies()

	return eh
}

// HandleError processes a GStreamer error and attempts recovery if configured
func (eh *ErrorHandler) HandleError(ctx context.Context, errorType GStreamerErrorType,
	component, operation, message string, cause error) error {

	// Create unified error
	gstError := &GStreamerError{
		Type:        errorType,
		Severity:    eh.determineSeverity(errorType),
		Code:        eh.generateErrorCode(errorType, component),
		Message:     message,
		Component:   component,
		Operation:   operation,
		Timestamp:   time.Now(),
		Cause:       cause,
		Recoverable: eh.isRecoverable(errorType),
		Suggested:   eh.getSuggestedRecoveryActions(errorType),
		Metadata:    make(map[string]interface{}),
	}

	// Add stack trace if enabled
	if eh.config.EnableStackTraces {
		gstError.StackTrace = eh.captureStackTrace()
	}

	// Add context information
	if cause != nil {
		gstError.Details = cause.Error()
	}

	// Record error
	eh.recordError(gstError)

	// Update metrics
	eh.updateMetrics(gstError)

	// Log error
	eh.logError(gstError)

	// Notify error handlers
	eh.notifyErrorHandlers(gstError)

	// Attempt recovery if enabled and error is recoverable
	if eh.config.AutoRecovery && gstError.IsRecoverable() {
		if err := eh.attemptRecovery(ctx, gstError); err != nil {
			eh.logger.Errorf("Recovery failed for error %s: %v", gstError.Code, err)
		}
	}

	// Check if component should be restarted
	if eh.shouldRestartComponent(component) {
		eh.logger.Warnf("Component %s has exceeded error threshold, restart recommended", component)
		gstError.Metadata["restart_recommended"] = true
	}

	// Propagate error if configured
	if eh.config.PropagateErrors && gstError.IsCritical() {
		return gstError
	}

	return nil
}

// CreateError creates a new GStreamer error without handling it
func (eh *ErrorHandler) CreateError(errorType GStreamerErrorType, component, message string, cause error) *GStreamerError {
	return &GStreamerError{
		Type:        errorType,
		Severity:    eh.determineSeverity(errorType),
		Code:        eh.generateErrorCode(errorType, component),
		Message:     message,
		Component:   component,
		Timestamp:   time.Now(),
		Cause:       cause,
		Recoverable: eh.isRecoverable(errorType),
		Suggested:   eh.getSuggestedRecoveryActions(errorType),
		Metadata:    make(map[string]interface{}),
	}
}

// WrapError wraps an existing error as a GStreamer error
func (eh *ErrorHandler) WrapError(err error, errorType GStreamerErrorType, component, operation string) *GStreamerError {
	if err == nil {
		return nil
	}

	// Check if it's already a GStreamer error
	var gstErr *GStreamerError
	if errors.As(err, &gstErr) {
		// Update component and operation if not set
		if gstErr.Component == "" {
			gstErr.Component = component
		}
		if gstErr.Operation == "" {
			gstErr.Operation = operation
		}
		return gstErr
	}

	return &GStreamerError{
		Type:        errorType,
		Severity:    eh.determineSeverity(errorType),
		Code:        eh.generateErrorCode(errorType, component),
		Message:     err.Error(),
		Component:   component,
		Operation:   operation,
		Timestamp:   time.Now(),
		Cause:       err,
		Recoverable: eh.isRecoverable(errorType),
		Suggested:   eh.getSuggestedRecoveryActions(errorType),
		Metadata:    make(map[string]interface{}),
	}
}

// initializeDefaultRecoveryStrategies sets up default recovery strategies
func (eh *ErrorHandler) initializeDefaultRecoveryStrategies() {
	// Pipeline state errors - retry with increasing delays
	eh.recoveryStrategies[ErrorTypePipelineState] = RecoveryStrategy{
		MaxRetries:     3,
		RetryDelay:     time.Second,
		Actions:        []RecoveryAction{RecoveryRetry, RecoveryRestart},
		AutoRecover:    true,
		EscalationTime: 30 * time.Second,
	}

	// Element creation errors - try alternative elements
	eh.recoveryStrategies[ErrorTypeElementCreation] = RecoveryStrategy{
		MaxRetries:     2,
		RetryDelay:     2 * time.Second,
		Actions:        []RecoveryAction{RecoveryFallback, RecoveryRestart},
		AutoRecover:    true,
		EscalationTime: 60 * time.Second,
	}

	// Element linking errors - reconfigure and retry
	eh.recoveryStrategies[ErrorTypeElementLinking] = RecoveryStrategy{
		MaxRetries:     2,
		RetryDelay:     time.Second,
		Actions:        []RecoveryAction{RecoveryReconfigure, RecoveryRestart},
		AutoRecover:    true,
		EscalationTime: 30 * time.Second,
	}

	// Resource access errors - retry with backoff
	eh.recoveryStrategies[ErrorTypeResourceAccess] = RecoveryStrategy{
		MaxRetries:     5,
		RetryDelay:     3 * time.Second,
		Actions:        []RecoveryAction{RecoveryRetry, RecoveryFallback},
		AutoRecover:    true,
		EscalationTime: 120 * time.Second,
	}

	// Memory allocation errors - cleanup and restart
	eh.recoveryStrategies[ErrorTypeMemoryAllocation] = RecoveryStrategy{
		MaxRetries:     1,
		RetryDelay:     5 * time.Second,
		Actions:        []RecoveryAction{RecoveryRestart},
		AutoRecover:    true,
		EscalationTime: 30 * time.Second,
	}

	// Permission errors - manual intervention required
	eh.recoveryStrategies[ErrorTypePermission] = RecoveryStrategy{
		MaxRetries:     0,
		RetryDelay:     0,
		Actions:        []RecoveryAction{RecoveryManualIntervention},
		AutoRecover:    false,
		EscalationTime: 0,
	}

	// Hardware errors - limited recovery options
	eh.recoveryStrategies[ErrorTypeHardware] = RecoveryStrategy{
		MaxRetries:     1,
		RetryDelay:     5 * time.Second,
		Actions:        []RecoveryAction{RecoveryFallback, RecoveryManualIntervention},
		AutoRecover:    false,
		EscalationTime: 60 * time.Second,
	}

	// Configuration errors - validate and reconfigure
	eh.recoveryStrategies[ErrorTypeConfiguration] = RecoveryStrategy{
		MaxRetries:     2,
		RetryDelay:     time.Second,
		Actions:        []RecoveryAction{RecoveryReconfigure, RecoveryFallback},
		AutoRecover:    true,
		EscalationTime: 45 * time.Second,
	}
}

// attemptRecovery attempts to recover from an error
func (eh *ErrorHandler) attemptRecovery(ctx context.Context, gstError *GStreamerError) error {
	strategy, exists := eh.recoveryStrategies[gstError.Type]
	if !exists || !strategy.AutoRecover {
		return fmt.Errorf("no recovery strategy available for error type %s", gstError.Type.String())
	}

	// Create recovery context
	recoveryCtx, cancel := context.WithTimeout(ctx, strategy.EscalationTime)
	defer cancel()

	recovery := &RecoveryContext{
		ErrorID:   gstError.Code,
		Component: gstError.Component,
		StartTime: time.Now(),
		Attempts:  make([]RecoveryAttempt, 0),
		ctx:       recoveryCtx,
		cancel:    cancel,
	}

	eh.mu.Lock()
	eh.activeRecoveries[gstError.Code] = recovery
	eh.mu.Unlock()

	defer func() {
		eh.mu.Lock()
		delete(eh.activeRecoveries, gstError.Code)
		eh.mu.Unlock()
	}()

	// Try each recovery action
	for _, action := range strategy.Actions {
		if recovery.Cancelled {
			return fmt.Errorf("recovery cancelled")
		}

		recovery.CurrentAction = action

		for attempt := 0; attempt < strategy.MaxRetries+1; attempt++ {
			if attempt > 0 {
				delay := eh.calculateBackoffDelay(strategy.RetryDelay, attempt)
				select {
				case <-time.After(delay):
				case <-recoveryCtx.Done():
					return recoveryCtx.Err()
				}
			}

			attemptStart := time.Now()
			err := eh.executeRecoveryAction(recoveryCtx, action, gstError)

			recoveryAttempt := RecoveryAttempt{
				Action:    action,
				Timestamp: attemptStart,
				Success:   err == nil,
				Error:     err,
				Duration:  time.Since(attemptStart),
			}

			recovery.Attempts = append(recovery.Attempts, recoveryAttempt)
			gstError.Attempted = append(gstError.Attempted, recoveryAttempt)

			eh.metrics.mu.Lock()
			eh.metrics.RecoveryAttempts++
			if err == nil {
				eh.metrics.RecoverySuccesses++
			} else {
				eh.metrics.RecoveryFailures++
			}
			eh.metrics.mu.Unlock()

			if err == nil {
				eh.logger.Infof("Recovery successful for error %s using action %s",
					gstError.Code, action.String())
				return nil
			}

			eh.logger.Debugf("Recovery attempt %d failed for error %s: %v",
				attempt+1, gstError.Code, err)
		}
	}

	return fmt.Errorf("all recovery attempts failed for error %s", gstError.Code)
}

// executeRecoveryAction executes a specific recovery action
func (eh *ErrorHandler) executeRecoveryAction(ctx context.Context, action RecoveryAction, gstError *GStreamerError) error {
	eh.logger.Debugf("Executing recovery action %s for error %s", action.String(), gstError.Code)

	switch action {
	case RecoveryRetry:
		// For retry, we don't need to do anything special
		// The calling code will retry the original operation
		return nil

	case RecoveryRestart:
		// This would typically involve restarting the component
		// Implementation depends on the specific component
		eh.logger.Infof("Component restart recommended for %s", gstError.Component)
		return fmt.Errorf("restart recovery not implemented")

	case RecoveryReconfigure:
		// This would involve adjusting configuration
		eh.logger.Infof("Reconfiguration recommended for %s", gstError.Component)
		return fmt.Errorf("reconfigure recovery not implemented")

	case RecoveryFallback:
		// This would involve switching to alternative configuration
		eh.logger.Infof("Fallback configuration recommended for %s", gstError.Component)
		return fmt.Errorf("fallback recovery not implemented")

	case RecoveryManualIntervention:
		return fmt.Errorf("manual intervention required")

	default:
		return fmt.Errorf("unknown recovery action: %s", action.String())
	}
}

// Helper methods for error handling

func (eh *ErrorHandler) determineSeverity(errorType GStreamerErrorType) GStreamerErrorSeverity {
	switch errorType {
	case ErrorTypePermission, ErrorTypeHardware:
		return UnifiedSeverityFatal
	case ErrorTypeMemoryAllocation, ErrorTypeInitialization:
		return UnifiedSeverityCritical
	case ErrorTypePipelineCreation, ErrorTypePipelineState, ErrorTypeElementCreation:
		return UnifiedSeverityError
	case ErrorTypeElementLinking, ErrorTypeElementProperty, ErrorTypeDataFlow:
		return UnifiedSeverityWarning
	case ErrorTypeConfiguration, ErrorTypeValidation:
		return UnifiedSeverityInfo
	default:
		return UnifiedSeverityError
	}
}

func (eh *ErrorHandler) generateErrorCode(errorType GStreamerErrorType, component string) string {
	timestamp := time.Now().Unix()
	return fmt.Sprintf("GST_%s_%s_%d",
		strings.ToUpper(errorType.String()),
		strings.ToUpper(component),
		timestamp)
}

func (eh *ErrorHandler) isRecoverable(errorType GStreamerErrorType) bool {
	switch errorType {
	case ErrorTypePermission, ErrorTypeHardware:
		return false
	default:
		return true
	}
}

func (eh *ErrorHandler) getSuggestedRecoveryActions(errorType GStreamerErrorType) []RecoveryAction {
	strategy, exists := eh.recoveryStrategies[errorType]
	if !exists {
		return []RecoveryAction{RecoveryManualIntervention}
	}
	return strategy.Actions
}

func (eh *ErrorHandler) captureStackTrace() []string {
	// This is a placeholder for stack trace capture
	// In a real implementation, you would use runtime.Caller or similar
	return []string{"stack trace not implemented"}
}

func (eh *ErrorHandler) recordError(gstError *GStreamerError) {
	eh.mu.Lock()
	defer eh.mu.Unlock()

	// Add to history
	eh.errorHistory = append(eh.errorHistory, *gstError)

	// Trim history if needed
	if len(eh.errorHistory) > eh.config.MaxErrorHistory {
		eh.errorHistory = eh.errorHistory[1:]
	}

	// Add to component errors
	if eh.componentErrors[gstError.Component] == nil {
		eh.componentErrors[gstError.Component] = make([]GStreamerError, 0)
	}
	eh.componentErrors[gstError.Component] = append(eh.componentErrors[gstError.Component], *gstError)
}

func (eh *ErrorHandler) updateMetrics(gstError *GStreamerError) {
	eh.metrics.mu.Lock()
	defer eh.metrics.mu.Unlock()

	eh.metrics.TotalErrors++
	eh.metrics.ErrorsByType[gstError.Type]++
	eh.metrics.ErrorsBySeverity[gstError.Severity]++
	eh.metrics.ErrorsByComponent[gstError.Component]++
	eh.metrics.LastErrorTime = gstError.Timestamp
}

func (eh *ErrorHandler) logError(gstError *GStreamerError) {
	logEntry := eh.logger.WithFields(logrus.Fields{
		"error_type":     gstError.Type.String(),
		"error_severity": gstError.Severity.String(),
		"error_code":     gstError.Code,
		"component":      gstError.Component,
		"operation":      gstError.Operation,
		"recoverable":    gstError.Recoverable,
	})

	switch gstError.Severity {
	case UnifiedSeverityInfo:
		logEntry.Info(gstError.Message)
	case UnifiedSeverityWarning:
		logEntry.Warn(gstError.Message)
	case UnifiedSeverityError:
		logEntry.Error(gstError.Message)
	case UnifiedSeverityCritical, UnifiedSeverityFatal:
		logEntry.Error(gstError.Message)
	}
}

func (eh *ErrorHandler) notifyErrorHandlers(gstError *GStreamerError) {
	for _, handler := range eh.errorHandlers {
		if !handler(gstError) {
			break
		}
	}
}

func (eh *ErrorHandler) shouldRestartComponent(component string) bool {
	eh.mu.RLock()
	defer eh.mu.RUnlock()

	errors, exists := eh.componentErrors[component]
	if !exists {
		return false
	}

	// Count recent errors (within last 5 minutes)
	recentErrors := 0
	cutoff := time.Now().Add(-5 * time.Minute)

	for _, err := range errors {
		if err.Timestamp.After(cutoff) {
			recentErrors++
		}
	}

	return recentErrors >= eh.config.ComponentRestartThreshold
}

func (eh *ErrorHandler) calculateBackoffDelay(baseDelay time.Duration, attempt int) time.Duration {
	delay := baseDelay * time.Duration(1<<uint(attempt)) // Exponential backoff
	if delay > eh.config.MaxRecoveryDelay {
		delay = eh.config.MaxRecoveryDelay
	}
	return delay
}

// Public API methods

// AddErrorHandler adds an error event handler
func (eh *ErrorHandler) AddErrorHandler(handler ErrorEventHandler) {
	eh.mu.Lock()
	defer eh.mu.Unlock()
	eh.errorHandlers = append(eh.errorHandlers, handler)
}

// GetErrorHistory returns the error history
func (eh *ErrorHandler) GetErrorHistory() []GStreamerError {
	eh.mu.RLock()
	defer eh.mu.RUnlock()

	history := make([]GStreamerError, len(eh.errorHistory))
	copy(history, eh.errorHistory)
	return history
}

// GetComponentErrors returns errors for a specific component
func (eh *ErrorHandler) GetComponentErrors(component string) []GStreamerError {
	eh.mu.RLock()
	defer eh.mu.RUnlock()

	errors, exists := eh.componentErrors[component]
	if !exists {
		return nil
	}

	result := make([]GStreamerError, len(errors))
	copy(result, errors)
	return result
}

// GetMetrics returns error metrics
func (eh *ErrorHandler) GetMetrics() ErrorMetrics {
	eh.metrics.mu.RLock()
	defer eh.metrics.mu.RUnlock()

	// Create a copy of metrics
	metrics := ErrorMetrics{
		TotalErrors:       eh.metrics.TotalErrors,
		ErrorsByType:      make(map[GStreamerErrorType]int64),
		ErrorsBySeverity:  make(map[GStreamerErrorSeverity]int64),
		ErrorsByComponent: make(map[string]int64),
		RecoveryAttempts:  eh.metrics.RecoveryAttempts,
		RecoverySuccesses: eh.metrics.RecoverySuccesses,
		RecoveryFailures:  eh.metrics.RecoveryFailures,
		ComponentRestarts: eh.metrics.ComponentRestarts,
		LastErrorTime:     eh.metrics.LastErrorTime,
	}

	for k, v := range eh.metrics.ErrorsByType {
		metrics.ErrorsByType[k] = v
	}
	for k, v := range eh.metrics.ErrorsBySeverity {
		metrics.ErrorsBySeverity[k] = v
	}
	for k, v := range eh.metrics.ErrorsByComponent {
		metrics.ErrorsByComponent[k] = v
	}

	return metrics
}

// CancelRecovery cancels an active recovery operation
func (eh *ErrorHandler) CancelRecovery(errorCode string) error {
	eh.mu.Lock()
	defer eh.mu.Unlock()

	recovery, exists := eh.activeRecoveries[errorCode]
	if !exists {
		return fmt.Errorf("no active recovery found for error %s", errorCode)
	}

	recovery.Cancelled = true
	recovery.cancel()

	return nil
}

// SetRecoveryStrategy sets a custom recovery strategy for an error type
func (eh *ErrorHandler) SetRecoveryStrategy(errorType GStreamerErrorType, strategy RecoveryStrategy) {
	eh.mu.Lock()
	defer eh.mu.Unlock()
	eh.recoveryStrategies[errorType] = strategy
}

// GetRecoveryStrategy gets the recovery strategy for an error type
func (eh *ErrorHandler) GetRecoveryStrategy(errorType GStreamerErrorType) (RecoveryStrategy, bool) {
	eh.mu.RLock()
	defer eh.mu.RUnlock()
	strategy, exists := eh.recoveryStrategies[errorType]
	return strategy, exists
}

// Convenience functions for common error scenarios

// NewInitializationError creates an initialization error
func NewInitializationError(component, message string, cause error) *GStreamerError {
	return &GStreamerError{
		Type:        ErrorTypeInitialization,
		Severity:    UnifiedSeverityCritical,
		Component:   component,
		Message:     message,
		Cause:       cause,
		Timestamp:   time.Now(),
		Recoverable: false,
		Metadata:    make(map[string]interface{}),
	}
}

// NewPipelineError creates a pipeline-related error
func NewPipelineError(errorType GStreamerErrorType, component, operation, message string, cause error) *GStreamerError {
	severity := UnifiedSeverityError
	if errorType == ErrorTypePipelineCreation {
		severity = UnifiedSeverityCritical
	}

	return &GStreamerError{
		Type:        errorType,
		Severity:    severity,
		Component:   component,
		Operation:   operation,
		Message:     message,
		Cause:       cause,
		Timestamp:   time.Now(),
		Recoverable: true,
		Suggested:   []RecoveryAction{RecoveryRetry, RecoveryRestart},
		Metadata:    make(map[string]interface{}),
	}
}

// NewElementError creates an element-related error
func NewElementError(errorType GStreamerErrorType, component, elementName, message string, cause error) *GStreamerError {
	gstError := &GStreamerError{
		Type:        errorType,
		Severity:    UnifiedSeverityError,
		Component:   component,
		Message:     message,
		Cause:       cause,
		Timestamp:   time.Now(),
		Recoverable: true,
		Metadata:    make(map[string]interface{}),
	}

	gstError.Metadata["element_name"] = elementName

	switch errorType {
	case ErrorTypeElementCreation:
		gstError.Suggested = []RecoveryAction{RecoveryFallback, RecoveryRestart}
	case ErrorTypeElementLinking:
		gstError.Suggested = []RecoveryAction{RecoveryReconfigure, RecoveryRestart}
	case ErrorTypeElementProperty:
		gstError.Suggested = []RecoveryAction{RecoveryReconfigure, RecoveryRetry}
	default:
		gstError.Suggested = []RecoveryAction{RecoveryRetry}
	}

	return gstError
}

// NewResourceError creates a resource access error
func NewResourceError(component, resource, message string, cause error) *GStreamerError {
	gstError := &GStreamerError{
		Type:        ErrorTypeResourceAccess,
		Severity:    UnifiedSeverityWarning,
		Component:   component,
		Message:     message,
		Cause:       cause,
		Timestamp:   time.Now(),
		Recoverable: true,
		Suggested:   []RecoveryAction{RecoveryRetry, RecoveryFallback},
		Metadata:    make(map[string]interface{}),
	}

	gstError.Metadata["resource"] = resource
	return gstError
}

// NewMemoryError creates a memory allocation error
func NewMemoryError(component, operation string, requestedSize int64, cause error) *GStreamerError {
	gstError := &GStreamerError{
		Type:        ErrorTypeMemoryAllocation,
		Severity:    UnifiedSeverityCritical,
		Component:   component,
		Operation:   operation,
		Message:     fmt.Sprintf("Memory allocation failed for %d bytes", requestedSize),
		Cause:       cause,
		Timestamp:   time.Now(),
		Recoverable: true,
		Suggested:   []RecoveryAction{RecoveryRestart},
		Metadata:    make(map[string]interface{}),
	}

	gstError.Metadata["requested_size"] = requestedSize
	return gstError
}

// NewTimeoutError creates a timeout error
func NewTimeoutError(component, operation string, timeout time.Duration, cause error) *GStreamerError {
	gstError := &GStreamerError{
		Type:        ErrorTypeTimeout,
		Severity:    UnifiedSeverityWarning,
		Component:   component,
		Operation:   operation,
		Message:     fmt.Sprintf("Operation timed out after %v", timeout),
		Cause:       cause,
		Timestamp:   time.Now(),
		Recoverable: true,
		Suggested:   []RecoveryAction{RecoveryRetry, RecoveryReconfigure},
		Metadata:    make(map[string]interface{}),
	}

	gstError.Metadata["timeout_duration"] = timeout
	return gstError
}

// NewValidationError creates a validation error
func NewValidationError(component, field, message string, value interface{}) *GStreamerError {
	gstError := &GStreamerError{
		Type:        ErrorTypeValidation,
		Severity:    UnifiedSeverityInfo,
		Component:   component,
		Message:     message,
		Timestamp:   time.Now(),
		Recoverable: true,
		Suggested:   []RecoveryAction{RecoveryReconfigure},
		Metadata:    make(map[string]interface{}),
	}

	gstError.Metadata["field"] = field
	gstError.Metadata["value"] = value
	return gstError
}

// Error checking utilities

// IsGStreamerError checks if an error is a GStreamer error
func IsGStreamerError(err error) bool {
	var gstErr *GStreamerError
	return errors.As(err, &gstErr)
}

// GetGStreamerError extracts a GStreamer error from an error
func GetGStreamerError(err error) (*GStreamerError, bool) {
	var gstErr *GStreamerError
	if errors.As(err, &gstErr) {
		return gstErr, true
	}
	return nil, false
}

// IsErrorType checks if an error is of a specific GStreamer error type
func IsErrorType(err error, errorType GStreamerErrorType) bool {
	if gstErr, ok := GetGStreamerError(err); ok {
		return gstErr.Type == errorType
	}
	return false
}

// IsErrorSeverity checks if an error has a specific severity level
func IsErrorSeverity(err error, severity GStreamerErrorSeverity) bool {
	if gstErr, ok := GetGStreamerError(err); ok {
		return gstErr.Severity == severity
	}
	return false
}

// IsCriticalError checks if an error is critical or fatal
func IsCriticalError(err error) bool {
	if gstErr, ok := GetGStreamerError(err); ok {
		return gstErr.IsCritical()
	}
	return false
}

// IsRecoverableError checks if an error is recoverable
func IsRecoverableError(err error) bool {
	if gstErr, ok := GetGStreamerError(err); ok {
		return gstErr.IsRecoverable()
	}
	return false
}

// Error aggregation utilities

// ErrorAggregator collects multiple errors and provides summary information
type ErrorAggregator struct {
	errors []GStreamerError
	mu     sync.RWMutex
}

// NewErrorAggregator creates a new error aggregator
func NewErrorAggregator() *ErrorAggregator {
	return &ErrorAggregator{
		errors: make([]GStreamerError, 0),
	}
}

// Add adds an error to the aggregator
func (ea *ErrorAggregator) Add(err *GStreamerError) {
	if err == nil {
		return
	}

	ea.mu.Lock()
	defer ea.mu.Unlock()
	ea.errors = append(ea.errors, *err)
}

// GetErrors returns all collected errors
func (ea *ErrorAggregator) GetErrors() []GStreamerError {
	ea.mu.RLock()
	defer ea.mu.RUnlock()

	result := make([]GStreamerError, len(ea.errors))
	copy(result, ea.errors)
	return result
}

// HasErrors returns true if there are any errors
func (ea *ErrorAggregator) HasErrors() bool {
	ea.mu.RLock()
	defer ea.mu.RUnlock()
	return len(ea.errors) > 0
}

// HasCriticalErrors returns true if there are any critical errors
func (ea *ErrorAggregator) HasCriticalErrors() bool {
	ea.mu.RLock()
	defer ea.mu.RUnlock()

	for _, err := range ea.errors {
		if err.IsCritical() {
			return true
		}
	}
	return false
}

// GetSummary returns a summary of collected errors
func (ea *ErrorAggregator) GetSummary() map[string]interface{} {
	ea.mu.RLock()
	defer ea.mu.RUnlock()

	summary := map[string]interface{}{
		"total_errors": len(ea.errors),
	}

	if len(ea.errors) == 0 {
		return summary
	}

	// Count by type and severity
	byType := make(map[string]int)
	bySeverity := make(map[string]int)
	byComponent := make(map[string]int)

	for _, err := range ea.errors {
		byType[err.Type.String()]++
		bySeverity[err.Severity.String()]++
		byComponent[err.Component]++
	}

	summary["by_type"] = byType
	summary["by_severity"] = bySeverity
	summary["by_component"] = byComponent

	return summary
}

// Clear clears all collected errors
func (ea *ErrorAggregator) Clear() {
	ea.mu.Lock()
	defer ea.mu.Unlock()
	ea.errors = ea.errors[:0]
}

// Global error handler instance (optional singleton pattern)
var (
	globalErrorHandler *ErrorHandler
	globalErrorMutex   sync.RWMutex
)

// SetGlobalErrorHandler sets the global error handler instance
func SetGlobalErrorHandler(handler *ErrorHandler) {
	globalErrorMutex.Lock()
	defer globalErrorMutex.Unlock()
	globalErrorHandler = handler
}

// GetGlobalErrorHandler returns the global error handler instance
func GetGlobalErrorHandler() *ErrorHandler {
	globalErrorMutex.RLock()
	defer globalErrorMutex.RUnlock()
	return globalErrorHandler
}

// HandleGlobalError handles an error using the global error handler
func HandleGlobalError(ctx context.Context, errorType GStreamerErrorType,
	component, operation, message string, cause error) error {

	handler := GetGlobalErrorHandler()
	if handler == nil {
		// Fallback to basic error handling
		return fmt.Errorf("[%s:%s] %s: %s", errorType.String(), component, operation, message)
	}

	return handler.HandleError(ctx, errorType, component, operation, message, cause)
}
