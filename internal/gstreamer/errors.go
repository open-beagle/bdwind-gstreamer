package gstreamer

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/go-gst/go-gst/gst"
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
	SeverityInfo GStreamerErrorSeverity = iota
	SeverityWarning
	SeverityError
	SeverityCritical
	SeverityFatal
)

// Legacy aliases for backward compatibility
const (
	UnifiedSeverityInfo     = SeverityInfo
	UnifiedSeverityWarning  = SeverityWarning
	UnifiedSeverityError    = SeverityError
	UnifiedSeverityCritical = SeverityCritical
	UnifiedSeverityFatal    = SeverityFatal
)

// String returns the string representation of GStreamerErrorSeverity
func (es GStreamerErrorSeverity) String() string {
	switch es {
	case SeverityInfo:
		return "Info"
	case SeverityWarning:
		return "Warning"
	case SeverityError:
		return "Error"
	case SeverityCritical:
		return "Critical"
	case SeverityFatal:
		return "Fatal"
	default:
		return "Unknown"
	}
}

// GetLogLevel returns the appropriate logrus level for this severity
func (es GStreamerErrorSeverity) GetLogLevel() logrus.Level {
	switch es {
	case SeverityInfo:
		return logrus.InfoLevel
	case SeverityWarning:
		return logrus.WarnLevel
	case SeverityError:
		return logrus.ErrorLevel
	case SeverityCritical:
		return logrus.ErrorLevel
	case SeverityFatal:
		return logrus.FatalLevel
	default:
		return logrus.ErrorLevel
	}
}

// IsHigherThan returns true if this severity is higher than the other
func (es GStreamerErrorSeverity) IsHigherThan(other GStreamerErrorSeverity) bool {
	return es > other
}

// RequiresImmediateAction returns true if this severity requires immediate action
func (es GStreamerErrorSeverity) RequiresImmediateAction() bool {
	return es >= SeverityCritical
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

// ErrorRecoveryStrategy defines how to recover from a specific error type
type ErrorRecoveryStrategy struct {
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
	recoveryStrategies map[GStreamerErrorType]ErrorRecoveryStrategy
	activeRecoveries   map[string]*RecoveryContext

	// Metrics
	metrics *ErrorMetrics

	// Event handlers
	errorHandlers []ErrorEventHandler

	// Enhanced debugging and analysis
	debugMode          bool
	pipelineVisualizer *PipelineVisualizer
	trendAnalyzer      *TrendAnalyzer
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

	// Enhanced metrics for trend analysis
	ErrorsPerHour   map[time.Time]int64
	ErrorTrends     *ErrorTrendAnalysis
	CriticalErrors  int64
	ErrorBursts     []ErrorBurst
	MeanTimeBetween time.Duration
	ErrorFrequency  float64 // errors per minute
}

// ErrorTrendAnalysis provides trend analysis for errors
type ErrorTrendAnalysis struct {
	mu sync.RWMutex

	// Time-based analysis
	HourlyErrorCounts map[int]int64    // hour of day -> error count
	DailyErrorCounts  map[string]int64 // date -> error count
	WeeklyErrorCounts map[int]int64    // week number -> error count

	// Component analysis
	ComponentTrends map[string]*ComponentErrorTrend

	// Severity trends
	SeverityTrends map[GStreamerErrorSeverity]*SeverityTrend

	// Recovery analysis
	RecoverySuccessRate float64
	AvgRecoveryTime     time.Duration

	// Prediction data
	PredictedErrorRate float64
	TrendDirection     TrendDirection
	ConfidenceLevel    float64
}

// ComponentErrorTrend tracks error trends for a specific component
type ComponentErrorTrend struct {
	Component           string
	ErrorCount          int64
	LastErrorTime       time.Time
	ErrorRate           float64 // errors per hour
	MostCommonErrorType GStreamerErrorType
	TrendDirection      TrendDirection
	HealthScore         float64 // 0-100, higher is better
}

// SeverityTrend tracks trends for a specific severity level
type SeverityTrend struct {
	Severity       GStreamerErrorSeverity
	Count          int64
	Rate           float64 // errors per hour
	TrendDirection TrendDirection
	RecentIncrease bool
}

// ErrorBurst represents a burst of errors in a short time period
type ErrorBurst struct {
	StartTime  time.Time
	EndTime    time.Time
	ErrorCount int64
	Duration   time.Duration
	Components []string
	Severity   GStreamerErrorSeverity
	Resolved   bool
}

// TrendDirection indicates the direction of an error trend
type TrendDirection int

const (
	TrendStable TrendDirection = iota
	TrendIncreasing
	TrendDecreasing
	TrendVolatile
)

// String returns the string representation of TrendDirection
func (td TrendDirection) String() string {
	switch td {
	case TrendStable:
		return "Stable"
	case TrendIncreasing:
		return "Increasing"
	case TrendDecreasing:
		return "Decreasing"
	case TrendVolatile:
		return "Volatile"
	default:
		return "Unknown"
	}
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
		recoveryStrategies: make(map[GStreamerErrorType]ErrorRecoveryStrategy),
		activeRecoveries:   make(map[string]*RecoveryContext),
		metrics: &ErrorMetrics{
			ErrorsByType:      make(map[GStreamerErrorType]int64),
			ErrorsBySeverity:  make(map[GStreamerErrorSeverity]int64),
			ErrorsByComponent: make(map[string]int64),
			ErrorsPerHour:     make(map[time.Time]int64),
			ErrorTrends:       NewErrorTrendAnalysis(),
			ErrorBursts:       make([]ErrorBurst, 0),
		},
		errorHandlers:      make([]ErrorEventHandler, 0),
		debugMode:          false,
		pipelineVisualizer: NewPipelineVisualizer(logger.WithField("subcomponent", "visualizer"), nil),
		trendAnalyzer:      NewTrendAnalyzer(logger.WithField("subcomponent", "trend-analyzer"), nil),
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

	// Update trend analysis
	eh.updateTrendAnalysis(gstError)

	// Log error with enhanced severity handling
	eh.logErrorWithSeverity(gstError)

	// Record error for pipeline visualization if in debug mode
	if eh.debugMode && eh.pipelineVisualizer != nil {
		eh.pipelineVisualizer.RecordError(component, errorType, message)
	}

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
	eh.recoveryStrategies[ErrorTypePipelineState] = ErrorRecoveryStrategy{
		MaxRetries:     3,
		RetryDelay:     time.Second,
		Actions:        []RecoveryAction{RecoveryRetry, RecoveryRestart},
		AutoRecover:    true,
		EscalationTime: 30 * time.Second,
	}

	// Element creation errors - try alternative elements
	eh.recoveryStrategies[ErrorTypeElementCreation] = ErrorRecoveryStrategy{
		MaxRetries:     2,
		RetryDelay:     2 * time.Second,
		Actions:        []RecoveryAction{RecoveryFallback, RecoveryRestart},
		AutoRecover:    true,
		EscalationTime: 60 * time.Second,
	}

	// Element linking errors - reconfigure and retry
	eh.recoveryStrategies[ErrorTypeElementLinking] = ErrorRecoveryStrategy{
		MaxRetries:     2,
		RetryDelay:     time.Second,
		Actions:        []RecoveryAction{RecoveryReconfigure, RecoveryRestart},
		AutoRecover:    true,
		EscalationTime: 30 * time.Second,
	}

	// Resource access errors - retry with backoff
	eh.recoveryStrategies[ErrorTypeResourceAccess] = ErrorRecoveryStrategy{
		MaxRetries:     5,
		RetryDelay:     3 * time.Second,
		Actions:        []RecoveryAction{RecoveryRetry, RecoveryFallback},
		AutoRecover:    true,
		EscalationTime: 120 * time.Second,
	}

	// Memory allocation errors - cleanup and restart
	eh.recoveryStrategies[ErrorTypeMemoryAllocation] = ErrorRecoveryStrategy{
		MaxRetries:     1,
		RetryDelay:     5 * time.Second,
		Actions:        []RecoveryAction{RecoveryRestart},
		AutoRecover:    true,
		EscalationTime: 30 * time.Second,
	}

	// Permission errors - manual intervention required
	eh.recoveryStrategies[ErrorTypePermission] = ErrorRecoveryStrategy{
		MaxRetries:     0,
		RetryDelay:     0,
		Actions:        []RecoveryAction{RecoveryManualIntervention},
		AutoRecover:    false,
		EscalationTime: 0,
	}

	// Hardware errors - limited recovery options
	eh.recoveryStrategies[ErrorTypeHardware] = ErrorRecoveryStrategy{
		MaxRetries:     1,
		RetryDelay:     5 * time.Second,
		Actions:        []RecoveryAction{RecoveryFallback, RecoveryManualIntervention},
		AutoRecover:    false,
		EscalationTime: 60 * time.Second,
	}

	// Configuration errors - validate and reconfigure
	eh.recoveryStrategies[ErrorTypeConfiguration] = ErrorRecoveryStrategy{
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

// executeRecoveryAction executes a specific recovery action with enhanced context and logging
func (eh *ErrorHandler) executeRecoveryAction(ctx context.Context, action RecoveryAction, gstError *GStreamerError) error {
	actionLogger := eh.logger.WithFields(logrus.Fields{
		"recovery_action": action.String(),
		"error_code":      gstError.Code,
		"error_type":      gstError.Type.String(),
		"component":       gstError.Component,
		"operation":       gstError.Operation,
	})

	actionLogger.Info("Executing recovery action")
	startTime := time.Now()

	defer func() {
		duration := time.Since(startTime)
		actionLogger.WithField("duration", duration).Debug("Recovery action completed")
	}()

	switch action {
	case RecoveryRetry:
		return eh.executeRetryRecovery(ctx, gstError, actionLogger)

	case RecoveryRestart:
		return eh.executeRestartRecovery(ctx, gstError, actionLogger)

	case RecoveryReconfigure:
		return eh.executeReconfigureRecovery(ctx, gstError, actionLogger)

	case RecoveryFallback:
		return eh.executeFallbackRecovery(ctx, gstError, actionLogger)

	case RecoveryManualIntervention:
		return eh.executeManualInterventionRecovery(ctx, gstError, actionLogger)

	default:
		actionLogger.Errorf("Unknown recovery action: %s", action.String())
		return fmt.Errorf("unknown recovery action: %s", action.String())
	}
}

// executeRetryRecovery handles retry recovery with enhanced context
func (eh *ErrorHandler) executeRetryRecovery(ctx context.Context, gstError *GStreamerError, logger *logrus.Entry) error {
	logger.Debug("Executing retry recovery - operation will be retried by caller")

	// Add retry context to error metadata
	if gstError.Metadata == nil {
		gstError.Metadata = make(map[string]interface{})
	}
	gstError.Metadata["retry_timestamp"] = time.Now()
	gstError.Metadata["retry_context"] = fmt.Sprintf("Retry for %s operation on %s", gstError.Operation, gstError.Component)

	// For retry, we don't need to do anything special
	// The calling code will retry the original operation
	return nil
}

// executeRestartRecovery handles component restart recovery
func (eh *ErrorHandler) executeRestartRecovery(ctx context.Context, gstError *GStreamerError, logger *logrus.Entry) error {
	logger.WithFields(logrus.Fields{
		"restart_reason": fmt.Sprintf("%s error in %s", gstError.Type.String(), gstError.Operation),
		"error_severity": gstError.Severity.String(),
	}).Info("Component restart recovery initiated")

	// Add restart context to error metadata
	if gstError.Metadata == nil {
		gstError.Metadata = make(map[string]interface{})
	}
	gstError.Metadata["restart_recommended"] = true
	gstError.Metadata["restart_reason"] = fmt.Sprintf("%s error", gstError.Type.String())
	gstError.Metadata["restart_timestamp"] = time.Now()

	// Update metrics
	eh.metrics.mu.Lock()
	eh.metrics.ComponentRestarts++
	eh.metrics.mu.Unlock()

	// This would typically involve restarting the component
	// Implementation depends on the specific component
	logger.Warn("Component restart recovery not fully implemented - manual restart may be required")
	return fmt.Errorf("restart recovery requires manual intervention for component %s", gstError.Component)
}

// executeReconfigureRecovery handles reconfiguration recovery
func (eh *ErrorHandler) executeReconfigureRecovery(ctx context.Context, gstError *GStreamerError, logger *logrus.Entry) error {
	logger.WithFields(logrus.Fields{
		"reconfigure_reason": fmt.Sprintf("%s error in %s", gstError.Type.String(), gstError.Operation),
		"current_config":     "unknown", // Would be populated with actual config
	}).Info("Reconfiguration recovery initiated")

	// Add reconfiguration context to error metadata
	if gstError.Metadata == nil {
		gstError.Metadata = make(map[string]interface{})
	}
	gstError.Metadata["reconfigure_recommended"] = true
	gstError.Metadata["reconfigure_reason"] = fmt.Sprintf("%s error", gstError.Type.String())
	gstError.Metadata["reconfigure_timestamp"] = time.Now()

	// This would involve adjusting configuration based on error type
	switch gstError.Type {
	case ErrorTypeElementProperty:
		logger.Debug("Attempting to reset element properties to default values")
		gstError.Metadata["reconfigure_action"] = "reset_element_properties"
	case ErrorTypeFormatNegotiation:
		logger.Debug("Attempting to use fallback format negotiation")
		gstError.Metadata["reconfigure_action"] = "fallback_format_negotiation"
	case ErrorTypeConfiguration:
		logger.Debug("Attempting to reload configuration from defaults")
		gstError.Metadata["reconfigure_action"] = "reload_default_config"
	default:
		logger.Debug("Generic reconfiguration attempt")
		gstError.Metadata["reconfigure_action"] = "generic_reconfigure"
	}

	logger.Warn("Reconfiguration recovery not fully implemented - configuration changes may be required")
	return fmt.Errorf("reconfigure recovery requires implementation for error type %s", gstError.Type.String())
}

// executeFallbackRecovery handles fallback recovery
func (eh *ErrorHandler) executeFallbackRecovery(ctx context.Context, gstError *GStreamerError, logger *logrus.Entry) error {
	logger.WithFields(logrus.Fields{
		"fallback_reason": fmt.Sprintf("%s error in %s", gstError.Type.String(), gstError.Operation),
		"error_component": gstError.Component,
	}).Info("Fallback recovery initiated")

	// Add fallback context to error metadata
	if gstError.Metadata == nil {
		gstError.Metadata = make(map[string]interface{})
	}
	gstError.Metadata["fallback_recommended"] = true
	gstError.Metadata["fallback_reason"] = fmt.Sprintf("%s error", gstError.Type.String())
	gstError.Metadata["fallback_timestamp"] = time.Now()

	// Suggest fallback options based on error type and component
	switch gstError.Type {
	case ErrorTypeElementCreation:
		logger.Debug("Suggesting alternative element for creation")
		gstError.Metadata["fallback_suggestion"] = "try_alternative_element"
	case ErrorTypeHardware:
		logger.Debug("Suggesting software fallback for hardware error")
		gstError.Metadata["fallback_suggestion"] = "use_software_implementation"
	case ErrorTypeNetwork:
		logger.Debug("Suggesting local fallback for network error")
		gstError.Metadata["fallback_suggestion"] = "use_local_resources"
	case ErrorTypeResourceAccess:
		logger.Debug("Suggesting alternative resource access method")
		gstError.Metadata["fallback_suggestion"] = "try_alternative_resource"
	default:
		logger.Debug("Generic fallback suggestion")
		gstError.Metadata["fallback_suggestion"] = "use_safe_defaults"
	}

	logger.Warn("Fallback recovery not fully implemented - alternative configuration may be needed")
	return fmt.Errorf("fallback recovery requires implementation for error type %s", gstError.Type.String())
}

// executeManualInterventionRecovery handles manual intervention recovery
func (eh *ErrorHandler) executeManualInterventionRecovery(ctx context.Context, gstError *GStreamerError, logger *logrus.Entry) error {
	logger.WithFields(logrus.Fields{
		"intervention_reason": fmt.Sprintf("%s error in %s", gstError.Type.String(), gstError.Operation),
		"error_severity":      gstError.Severity.String(),
		"component":           gstError.Component,
	}).Warn("Manual intervention required for error recovery")

	// Add manual intervention context to error metadata
	if gstError.Metadata == nil {
		gstError.Metadata = make(map[string]interface{})
	}
	gstError.Metadata["manual_intervention_required"] = true
	gstError.Metadata["intervention_reason"] = fmt.Sprintf("%s error", gstError.Type.String())
	gstError.Metadata["intervention_timestamp"] = time.Now()

	// Provide specific guidance based on error type
	var guidance string
	switch gstError.Type {
	case ErrorTypePermission:
		guidance = "Check file/device permissions and user privileges"
		gstError.Metadata["intervention_guidance"] = guidance
	case ErrorTypeHardware:
		guidance = "Verify hardware availability and driver installation"
		gstError.Metadata["intervention_guidance"] = guidance
	case ErrorTypeNetwork:
		guidance = "Check network connectivity and firewall settings"
		gstError.Metadata["intervention_guidance"] = guidance
	case ErrorTypeResourceAccess:
		guidance = "Verify resource availability and access permissions"
		gstError.Metadata["intervention_guidance"] = guidance
	default:
		guidance = "Review error details and system configuration"
		gstError.Metadata["intervention_guidance"] = guidance
	}

	logger.WithField("guidance", guidance).Error("Manual intervention guidance")

	return fmt.Errorf("manual intervention required: %s", guidance)
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
func (eh *ErrorHandler) SetRecoveryStrategy(errorType GStreamerErrorType, strategy ErrorRecoveryStrategy) {
	eh.mu.Lock()
	defer eh.mu.Unlock()
	eh.recoveryStrategies[errorType] = strategy
}

// GetRecoveryStrategy gets the recovery strategy for an error type
func (eh *ErrorHandler) GetRecoveryStrategy(errorType GStreamerErrorType) (ErrorRecoveryStrategy, bool) {
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

// Enhanced error handling methods

// updateTrendAnalysis updates the trend analysis with enhanced error context
func (eh *ErrorHandler) updateTrendAnalysis(gstError *GStreamerError) {
	if eh.trendAnalyzer == nil {
		eh.logger.Debug("Trend analyzer not available, skipping trend analysis update")
		return
	}

	recovered := false
	recoveryTime := time.Duration(0)

	// Enhanced recovery analysis
	if len(gstError.Attempted) > 0 {
		lastAttempt := gstError.Attempted[len(gstError.Attempted)-1]
		recovered = lastAttempt.Success

		// Calculate total recovery time from first attempt to last success
		if recovered {
			firstAttempt := gstError.Attempted[0].Timestamp
			recoveryTime = lastAttempt.Timestamp.Sub(firstAttempt) + lastAttempt.Duration
		} else {
			// If not recovered, use the duration of the last attempt
			recoveryTime = lastAttempt.Duration
		}
	}

	// Add error data point to trend analyzer
	eh.trendAnalyzer.AddErrorDataPoint(
		gstError.Type,
		gstError.Severity,
		gstError.Component,
		recovered,
		recoveryTime,
	)

	// Log trend analysis update with context
	eh.logger.WithFields(logrus.Fields{
		"error_code":    gstError.Code,
		"error_type":    gstError.Type.String(),
		"component":     gstError.Component,
		"recovered":     recovered,
		"recovery_time": recoveryTime,
		"attempt_count": len(gstError.Attempted),
		"trend_updated": true,
	}).Debug("Updated error trend analysis with enhanced context")

	// Log additional context for critical errors
	if gstError.IsCritical() {
		eh.logger.WithFields(logrus.Fields{
			"error_code":    gstError.Code,
			"severity":      gstError.Severity.String(),
			"component":     gstError.Component,
			"critical_flag": true,
		}).Info("Critical error added to trend analysis - monitoring for patterns")
	}
}

// logErrorWithSeverity logs error with enhanced severity-based formatting
func (eh *ErrorHandler) logErrorWithSeverity(gstError *GStreamerError) {
	logEntry := eh.logger.WithFields(logrus.Fields{
		"error_type":     gstError.Type.String(),
		"error_severity": gstError.Severity.String(),
		"error_code":     gstError.Code,
		"component":      gstError.Component,
		"operation":      gstError.Operation,
		"recoverable":    gstError.Recoverable,
		"timestamp":      gstError.Timestamp,
	})

	// Add additional context for critical errors
	if gstError.IsCritical() {
		logEntry = logEntry.WithFields(logrus.Fields{
			"requires_immediate_action": true,
			"suggested_actions":         gstError.Suggested,
		})
	}

	// Add recovery information if available
	if len(gstError.Attempted) > 0 {
		logEntry = logEntry.WithField("recovery_attempts", len(gstError.Attempted))
		lastAttempt := gstError.Attempted[len(gstError.Attempted)-1]
		logEntry = logEntry.WithFields(logrus.Fields{
			"last_recovery_action":  lastAttempt.Action.String(),
			"last_recovery_success": lastAttempt.Success,
		})
	}

	// Log at appropriate level based on severity
	switch gstError.Severity {
	case SeverityInfo:
		logEntry.Info(gstError.Message)
	case SeverityWarning:
		logEntry.Warn(gstError.Message)
	case SeverityError:
		logEntry.Error(gstError.Message)
	case SeverityCritical:
		logEntry.WithField("alert", "CRITICAL_ERROR").Error(gstError.Message)
	case SeverityFatal:
		logEntry.WithField("alert", "FATAL_ERROR").Fatal(gstError.Message)
	default:
		logEntry.Error(gstError.Message)
	}
}

// formatSuggestedActions formats suggested recovery actions for logging
func (eh *ErrorHandler) formatSuggestedActions(actions []RecoveryAction) string {
	if len(actions) == 0 {
		return "none"
	}

	actionStrings := make([]string, len(actions))
	for i, action := range actions {
		actionStrings[i] = action.String()
	}
	return strings.Join(actionStrings, ", ")
}

// formatEnhancedErrorMessage creates an enhanced error message with context
func (eh *ErrorHandler) formatEnhancedErrorMessage(gstError *GStreamerError) string {
	var builder strings.Builder

	// Base message
	builder.WriteString(gstError.Message)

	// Add operation context if available
	if gstError.Operation != "" {
		builder.WriteString(fmt.Sprintf(" [Operation: %s]", gstError.Operation))
	}

	// Add details if available
	if gstError.Details != "" {
		builder.WriteString(fmt.Sprintf(" [Details: %s]", gstError.Details))
	}

	// Add recovery information
	if gstError.Recoverable {
		builder.WriteString(" [Recoverable]")
		if len(gstError.Suggested) > 0 {
			builder.WriteString(fmt.Sprintf(" [Suggested: %s]", eh.formatSuggestedActions(gstError.Suggested)))
		}
	} else {
		builder.WriteString(" [Not Recoverable]")
	}

	// Add attempt count if any
	if len(gstError.Attempted) > 0 {
		builder.WriteString(fmt.Sprintf(" [Attempts: %d]", len(gstError.Attempted)))
	}

	return builder.String()
}

// logRecoveryAttempts logs detailed information about recovery attempts
func (eh *ErrorHandler) logRecoveryAttempts(gstError *GStreamerError) {
	if len(gstError.Attempted) == 0 {
		return
	}

	recoveryLogger := eh.logger.WithFields(logrus.Fields{
		"error_code": gstError.Code,
		"component":  gstError.Component,
	})

	recoveryLogger.Infof("Recovery attempts for error %s:", gstError.Code)

	for i, attempt := range gstError.Attempted {
		attemptLogger := recoveryLogger.WithFields(logrus.Fields{
			"attempt_number": i + 1,
			"action":         attempt.Action.String(),
			"success":        attempt.Success,
			"duration":       attempt.Duration,
			"timestamp":      attempt.Timestamp.Format(time.RFC3339),
		})

		if attempt.Success {
			attemptLogger.Info("Recovery attempt succeeded")
		} else {
			attemptFields := logrus.Fields{}
			if attempt.Error != nil {
				attemptFields["error"] = attempt.Error.Error()
			}
			attemptLogger.WithFields(attemptFields).Warn("Recovery attempt failed")
		}
	}
}

// logErrorTrendContext logs trend analysis context for the error
func (eh *ErrorHandler) logErrorTrendContext(gstError *GStreamerError) {
	trends := eh.trendAnalyzer.GetCurrentTrends()
	if trends == nil {
		return
	}

	trendLogger := eh.logger.WithFields(logrus.Fields{
		"error_code": gstError.Code,
		"component":  gstError.Component,
	})

	// Log component trend if available
	if componentTrend, exists := trends.ComponentTrends[gstError.Component]; exists {
		trendLogger.WithFields(logrus.Fields{
			"component_error_count":     componentTrend.ErrorCount,
			"component_error_rate":      fmt.Sprintf("%.2f/hour", componentTrend.ErrorRate),
			"component_trend_direction": componentTrend.TrendDirection.String(),
			"component_health_score":    fmt.Sprintf("%.1f/100", componentTrend.HealthScore),
			"most_common_error_type":    componentTrend.MostCommonErrorType.String(),
		}).Debug("Component error trend context")
	}

	// Log severity trend if available
	if severityTrend, exists := trends.SeverityTrends[gstError.Severity]; exists {
		trendLogger.WithFields(logrus.Fields{
			"severity_error_count":     severityTrend.Count,
			"severity_error_rate":      fmt.Sprintf("%.2f/hour", severityTrend.Rate),
			"severity_trend_direction": severityTrend.TrendDirection.String(),
			"severity_recent_increase": severityTrend.RecentIncrease,
		}).Debug("Severity error trend context")
	}

	// Log overall trend information
	trendLogger.WithFields(logrus.Fields{
		"overall_trend_direction": trends.TrendDirection.String(),
		"trend_confidence_level":  fmt.Sprintf("%.2f", trends.ConfidenceLevel),
		"predicted_error_rate":    fmt.Sprintf("%.2f/hour", trends.PredictedErrorRate),
		"recovery_success_rate":   fmt.Sprintf("%.2f%%", trends.RecoverySuccessRate*100),
		"avg_recovery_time":       trends.AvgRecoveryTime.String(),
	}).Debug("Overall error trend context")
}

// EnableDebugMode enables debug mode with enhanced logging and visualization
func (eh *ErrorHandler) EnableDebugMode() {
	eh.mu.Lock()
	defer eh.mu.Unlock()

	eh.debugMode = true
	eh.logger.Info("Error handler debug mode enabled")

	// Enable pipeline visualization export
	if eh.pipelineVisualizer != nil {
		eh.pipelineVisualizer.SetExportEnabled(true)
	}
}

// DisableDebugMode disables debug mode
func (eh *ErrorHandler) DisableDebugMode() {
	eh.mu.Lock()
	defer eh.mu.Unlock()

	eh.debugMode = false
	eh.logger.Info("Error handler debug mode disabled")

	// Disable pipeline visualization export
	if eh.pipelineVisualizer != nil {
		eh.pipelineVisualizer.SetExportEnabled(false)
	}
}

// IsDebugMode returns whether debug mode is enabled
func (eh *ErrorHandler) IsDebugMode() bool {
	eh.mu.RLock()
	defer eh.mu.RUnlock()
	return eh.debugMode
}

// RegisterPipelineForVisualization registers a pipeline for visualization
func (eh *ErrorHandler) RegisterPipelineForVisualization(name string, pipeline *gst.Pipeline) error {
	if eh.pipelineVisualizer == nil {
		return fmt.Errorf("pipeline visualizer not available")
	}

	return eh.pipelineVisualizer.RegisterPipeline(name, pipeline)
}

// ExportPipelineVisualization exports a pipeline visualization
func (eh *ErrorHandler) ExportPipelineVisualization(pipelineName, reason string) error {
	if eh.pipelineVisualizer == nil {
		return fmt.Errorf("pipeline visualizer not available")
	}

	return eh.pipelineVisualizer.ExportPipeline(pipelineName, reason)
}

// GetErrorTrends returns current error trend analysis
func (eh *ErrorHandler) GetErrorTrends() *ErrorTrendAnalysis {
	if eh.trendAnalyzer == nil {
		return NewErrorTrendAnalysis()
	}

	return eh.trendAnalyzer.GetCurrentTrends()
}

// GetEnhancedMetrics returns enhanced error metrics with trend analysis
func (eh *ErrorHandler) GetEnhancedMetrics() *EnhancedErrorMetrics {
	eh.metrics.mu.RLock()
	baseMetrics := *eh.metrics
	eh.metrics.mu.RUnlock()

	trends := eh.GetErrorTrends()

	return &EnhancedErrorMetrics{
		ErrorMetrics: baseMetrics,
		Trends:       trends,
		DebugMode:    eh.debugMode,
		LastAnalysis: time.Now(),
	}
}

// EnhancedErrorMetrics combines basic metrics with trend analysis
type EnhancedErrorMetrics struct {
	ErrorMetrics
	Trends       *ErrorTrendAnalysis
	DebugMode    bool
	LastAnalysis time.Time
}

// GetErrorStatistics returns comprehensive error statistics
func (eh *ErrorHandler) GetErrorStatistics() *GStreamerErrorStatistics {
	eh.mu.RLock()
	defer eh.mu.RUnlock()

	stats := &GStreamerErrorStatistics{
		TotalErrors:       eh.metrics.TotalErrors,
		ErrorsByType:      make(map[string]int64),
		ErrorsBySeverity:  make(map[string]int64),
		ErrorsByComponent: make(map[string]int64),
		RecoveryStats: RecoveryStatistics{
			TotalAttempts: eh.metrics.RecoveryAttempts,
			Successes:     eh.metrics.RecoverySuccesses,
			Failures:      eh.metrics.RecoveryFailures,
			SuccessRate:   0.0,
		},
		TimeStats: TimeStatistics{
			LastErrorTime: eh.metrics.LastErrorTime,
			Uptime:        time.Since(eh.metrics.LastErrorTime),
		},
	}

	// Convert maps to string keys for JSON serialization
	for errorType, count := range eh.metrics.ErrorsByType {
		stats.ErrorsByType[errorType.String()] = count
	}
	for severity, count := range eh.metrics.ErrorsBySeverity {
		stats.ErrorsBySeverity[severity.String()] = count
	}
	for component, count := range eh.metrics.ErrorsByComponent {
		stats.ErrorsByComponent[component] = count
	}

	// Calculate recovery success rate
	if eh.metrics.RecoveryAttempts > 0 {
		stats.RecoveryStats.SuccessRate = float64(eh.metrics.RecoverySuccesses) / float64(eh.metrics.RecoveryAttempts)
	}

	return stats
}

// GStreamerErrorStatistics provides comprehensive error statistics
type GStreamerErrorStatistics struct {
	TotalErrors       int64              `json:"total_errors"`
	ErrorsByType      map[string]int64   `json:"errors_by_type"`
	ErrorsBySeverity  map[string]int64   `json:"errors_by_severity"`
	ErrorsByComponent map[string]int64   `json:"errors_by_component"`
	RecoveryStats     RecoveryStatistics `json:"recovery_stats"`
	TimeStats         TimeStatistics     `json:"time_stats"`
}

// RecoveryStatistics provides recovery-related statistics
type RecoveryStatistics struct {
	TotalAttempts int64   `json:"total_attempts"`
	Successes     int64   `json:"successes"`
	Failures      int64   `json:"failures"`
	SuccessRate   float64 `json:"success_rate"`
}

// TimeStatistics provides time-related statistics
type TimeStatistics struct {
	LastErrorTime time.Time     `json:"last_error_time"`
	Uptime        time.Duration `json:"uptime"`
}

// GenerateErrorReport generates a comprehensive error report
func (eh *ErrorHandler) GenerateErrorReport() *ErrorReport {
	statistics := eh.GetErrorStatistics()
	trends := eh.GetErrorTrends()
	enhancedMetrics := eh.GetEnhancedMetrics()

	report := &ErrorReport{
		GeneratedAt:     time.Now(),
		Statistics:      statistics,
		Trends:          trends,
		EnhancedMetrics: enhancedMetrics,
		RecentErrors:    eh.getRecentErrors(10),
		Recommendations: eh.generateRecommendations(trends),
	}

	return report
}

// ErrorReport provides a comprehensive error analysis report
type ErrorReport struct {
	GeneratedAt     time.Time                 `json:"generated_at"`
	Statistics      *GStreamerErrorStatistics `json:"statistics"`
	Trends          *ErrorTrendAnalysis       `json:"trends"`
	EnhancedMetrics *EnhancedErrorMetrics     `json:"enhanced_metrics"`
	RecentErrors    []GStreamerError          `json:"recent_errors"`
	Recommendations []string                  `json:"recommendations"`
}

// getRecentErrors returns the most recent errors
func (eh *ErrorHandler) getRecentErrors(limit int) []GStreamerError {
	eh.mu.RLock()
	defer eh.mu.RUnlock()

	if len(eh.errorHistory) == 0 {
		return []GStreamerError{}
	}

	start := len(eh.errorHistory) - limit
	if start < 0 {
		start = 0
	}

	recent := make([]GStreamerError, len(eh.errorHistory)-start)
	copy(recent, eh.errorHistory[start:])

	return recent
}

// generateRecommendations generates recommendations based on error trends
func (eh *ErrorHandler) generateRecommendations(trends *ErrorTrendAnalysis) []string {
	var recommendations []string

	// Check recovery success rate
	if trends.RecoverySuccessRate < 0.5 {
		recommendations = append(recommendations,
			"Low recovery success rate detected. Consider reviewing recovery strategies.")
	}

	// Check for increasing error trends
	if trends.TrendDirection == TrendIncreasing {
		recommendations = append(recommendations,
			"Error rate is increasing. Investigate root causes and consider preventive measures.")
	}

	// Check for component-specific issues
	for component, trend := range trends.ComponentTrends {
		if trend.HealthScore < 50 {
			recommendations = append(recommendations,
				fmt.Sprintf("Component '%s' has low health score (%.1f). Consider maintenance or replacement.",
					component, trend.HealthScore))
		}
	}

	// Check for critical error trends
	if criticalTrend, exists := trends.SeverityTrends[SeverityCritical]; exists && criticalTrend.RecentIncrease {
		recommendations = append(recommendations,
			"Recent increase in critical errors detected. Immediate investigation recommended.")
	}

	if len(recommendations) == 0 {
		recommendations = append(recommendations, "No specific recommendations at this time. System appears stable.")
	}

	return recommendations
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
