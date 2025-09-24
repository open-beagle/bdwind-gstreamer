package gstreamer

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// EnhancedErrorContext provides comprehensive error context and recovery tracking
type EnhancedErrorContext struct {
	logger *logrus.Entry
	mu     sync.RWMutex

	// Core context
	errorHandler       *ErrorHandler
	debugContext       *DebugContext
	pipelineVisualizer *PipelineVisualizer
	trendAnalyzer      *TrendAnalyzer

	// Error tracking
	activeErrors    map[string]*TrackedError
	errorSequences  []ErrorSequence
	recoveryHistory []RecoveryEvent

	// Configuration
	config *EnhancedErrorConfig
}

// EnhancedErrorConfig configures the enhanced error context
type EnhancedErrorConfig struct {
	EnableDetailedLogging     bool
	EnableStackTraces         bool
	EnableBreadcrumbs         bool
	EnablePerformanceTracking bool
	EnableTrendAnalysis       bool
	EnableVisualization       bool
	MaxErrorHistory           int
	MaxSequenceLength         int
	ContextTimeout            time.Duration
}

// TrackedError represents an error being tracked with full context
type TrackedError struct {
	ID               string
	Error            *GStreamerError
	DebugContext     *DebugContext
	StartTime        time.Time
	LastUpdate       time.Time
	RecoveryAttempts []EnhancedRecoveryAttempt
	RelatedErrors    []string
	ContextData      map[string]interface{}
	Status           TrackedErrorStatus
}

// TrackedErrorStatus represents the status of a tracked error
type TrackedErrorStatus int

const (
	ErrorStatusActive TrackedErrorStatus = iota
	ErrorStatusRecovering
	ErrorStatusRecovered
	ErrorStatusFailed
	ErrorStatusEscalated
)

// String returns the string representation of TrackedErrorStatus
func (s TrackedErrorStatus) String() string {
	switch s {
	case ErrorStatusActive:
		return "Active"
	case ErrorStatusRecovering:
		return "Recovering"
	case ErrorStatusRecovered:
		return "Recovered"
	case ErrorStatusFailed:
		return "Failed"
	case ErrorStatusEscalated:
		return "Escalated"
	default:
		return "Unknown"
	}
}

// ErrorSequence represents a sequence of related errors
type ErrorSequence struct {
	ID        string
	StartTime time.Time
	EndTime   time.Time
	ErrorIDs  []string
	Pattern   string
	Severity  GStreamerErrorSeverity
	Component string
	Resolved  bool
	RootCause string
}

// EnhancedRecoveryAttempt extends RecoveryAttempt with additional context
type EnhancedRecoveryAttempt struct {
	RecoveryAttempt
	ContextSnapshot map[string]interface{}
	MemoryUsage     uint64
	SystemState     map[string]interface{}
	PreConditions   []string
	PostConditions  []string
}

// RecoveryEvent represents a recovery event with full context
type RecoveryEvent struct {
	Timestamp      time.Time
	ErrorID        string
	Action         RecoveryAction
	Success        bool
	Duration       time.Duration
	Context        map[string]interface{}
	SystemImpact   string
	LessonsLearned []string
}

// NewEnhancedErrorContext creates a new enhanced error context
func NewEnhancedErrorContext(logger *logrus.Entry, errorHandler *ErrorHandler, config *EnhancedErrorConfig) *EnhancedErrorContext {
	if config == nil {
		config = DefaultEnhancedErrorConfig()
	}

	if logger == nil {
		logger = logrus.WithField("component", "enhanced-error-context")
	}

	ctx := &EnhancedErrorContext{
		logger:          logger,
		errorHandler:    errorHandler,
		activeErrors:    make(map[string]*TrackedError),
		errorSequences:  make([]ErrorSequence, 0),
		recoveryHistory: make([]RecoveryEvent, 0),
		config:          config,
	}

	// Initialize optional components
	if config.EnableTrendAnalysis && errorHandler != nil {
		ctx.trendAnalyzer = errorHandler.trendAnalyzer
	}

	if config.EnableVisualization && errorHandler != nil {
		ctx.pipelineVisualizer = errorHandler.pipelineVisualizer
	}

	logger.WithFields(logrus.Fields{
		"detailed_logging":     config.EnableDetailedLogging,
		"stack_traces":         config.EnableStackTraces,
		"breadcrumbs":          config.EnableBreadcrumbs,
		"performance_tracking": config.EnablePerformanceTracking,
		"trend_analysis":       config.EnableTrendAnalysis,
		"visualization":        config.EnableVisualization,
	}).Info("Enhanced error context initialized")

	return ctx
}

// DefaultEnhancedErrorConfig returns default configuration
func DefaultEnhancedErrorConfig() *EnhancedErrorConfig {
	return &EnhancedErrorConfig{
		EnableDetailedLogging:     true,
		EnableStackTraces:         true,
		EnableBreadcrumbs:         true,
		EnablePerformanceTracking: true,
		EnableTrendAnalysis:       true,
		EnableVisualization:       true,
		MaxErrorHistory:           1000,
		MaxSequenceLength:         50,
		ContextTimeout:            30 * time.Minute,
	}
}

// HandleErrorWithContext handles an error with comprehensive context tracking
func (eec *EnhancedErrorContext) HandleErrorWithContext(ctx context.Context, gstError *GStreamerError, debugCtx *DebugContext) error {
	eec.mu.Lock()
	defer eec.mu.Unlock()

	// Delegate to standard error handler first to get the generated error code
	var generatedErrorCode string
	if eec.errorHandler != nil {
		// Create a temporary error to get the generated code
		tempError := eec.errorHandler.CreateError(gstError.Type, gstError.Component, gstError.Message, gstError.Cause)
		generatedErrorCode = tempError.Code
		gstError.Code = generatedErrorCode // Update the error with the generated code
	}

	// Create tracked error
	trackedError := &TrackedError{
		ID:           gstError.Code,
		Error:        gstError,
		DebugContext: debugCtx,
		StartTime:    time.Now(),
		LastUpdate:   time.Now(),
		ContextData:  make(map[string]interface{}),
		Status:       ErrorStatusActive,
	}

	// Enhance error with additional context
	eec.enhanceErrorContext(trackedError)

	// Track the error
	eec.activeErrors[trackedError.ID] = trackedError

	// Log enhanced error information
	eec.logEnhancedError(trackedError)

	// Analyze error patterns
	eec.analyzeErrorPatterns(trackedError)

	// Update visualizations if enabled
	if eec.config.EnableVisualization && eec.pipelineVisualizer != nil {
		eec.pipelineVisualizer.RecordError(gstError.Component, gstError.Type, gstError.Message)
	}

	// Delegate to standard error handler
	if eec.errorHandler != nil {
		return eec.errorHandler.HandleError(ctx, gstError.Type, gstError.Component, gstError.Operation, gstError.Message, gstError.Cause)
	}

	return gstError
}

// enhanceErrorContext enhances error with additional context information
func (eec *EnhancedErrorContext) enhanceErrorContext(trackedError *TrackedError) {
	gstError := trackedError.Error

	// Add system context
	trackedError.ContextData["system_time"] = time.Now()
	trackedError.ContextData["goroutine_id"] = getGoroutineID()

	// Add memory information
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	trackedError.ContextData["memory_alloc"] = m.Alloc
	trackedError.ContextData["memory_sys"] = m.Sys
	trackedError.ContextData["num_gc"] = m.NumGC

	// Add stack trace if enabled
	if eec.config.EnableStackTraces {
		if len(gstError.StackTrace) == 0 {
			gstError.StackTrace = captureStackTrace(3) // Skip current function calls
		}
		trackedError.ContextData["stack_trace_depth"] = len(gstError.StackTrace)
	}

	// Add debug context information if available
	if trackedError.DebugContext != nil {
		summary := trackedError.DebugContext.GetSummary()
		for key, value := range summary {
			trackedError.ContextData[fmt.Sprintf("debug_%s", key)] = value
		}
	}

	// Add trend analysis context if available
	if eec.config.EnableTrendAnalysis && eec.trendAnalyzer != nil {
		trends := eec.trendAnalyzer.GetCurrentTrends()
		if trends != nil {
			if componentTrend, exists := trends.ComponentTrends[gstError.Component]; exists {
				trackedError.ContextData["component_health_score"] = componentTrend.HealthScore
				trackedError.ContextData["component_error_rate"] = componentTrend.ErrorRate
				trackedError.ContextData["component_trend_direction"] = componentTrend.TrendDirection.String()
			}
		}
	}

	// Add related error information
	trackedError.RelatedErrors = eec.findRelatedErrors(gstError)
}

// logEnhancedError logs error with comprehensive context
func (eec *EnhancedErrorContext) logEnhancedError(trackedError *TrackedError) {
	if !eec.config.EnableDetailedLogging {
		return
	}

	gstError := trackedError.Error

	logEntry := eec.logger.WithFields(logrus.Fields{
		"tracked_error_id":     trackedError.ID,
		"error_type":           gstError.Type.String(),
		"error_severity":       gstError.Severity.String(),
		"component":            gstError.Component,
		"operation":            gstError.Operation,
		"recoverable":          gstError.Recoverable,
		"tracking_start_time":  trackedError.StartTime,
		"context_data_keys":    len(trackedError.ContextData),
		"related_errors_count": len(trackedError.RelatedErrors),
	})

	// Add context data to log entry
	for key, value := range trackedError.ContextData {
		logEntry = logEntry.WithField(fmt.Sprintf("ctx_%s", key), value)
	}

	// Add debug context breadcrumbs if available
	if eec.config.EnableBreadcrumbs && trackedError.DebugContext != nil {
		breadcrumbs := trackedError.DebugContext.GetBreadcrumbs()
		if len(breadcrumbs) > 0 {
			logEntry = logEntry.WithField("breadcrumb_count", len(breadcrumbs))
			// Log last few breadcrumbs
			recentBreadcrumbs := breadcrumbs
			if len(breadcrumbs) > 5 {
				recentBreadcrumbs = breadcrumbs[len(breadcrumbs)-5:]
			}

			var breadcrumbSummary []string
			for _, bc := range recentBreadcrumbs {
				breadcrumbSummary = append(breadcrumbSummary, fmt.Sprintf("%s:%s", bc.Operation, bc.Message))
			}
			logEntry = logEntry.WithField("recent_breadcrumbs", strings.Join(breadcrumbSummary, " -> "))
		}
	}

	// Log with appropriate severity
	level := gstError.Severity.GetLogLevel()
	message := fmt.Sprintf("Enhanced error context: %s", gstError.Message)
	logEntry.Log(level, message)

	// Log stack trace if available and enabled
	if eec.config.EnableStackTraces && len(gstError.StackTrace) > 0 {
		eec.logger.WithFields(logrus.Fields{
			"tracked_error_id": trackedError.ID,
			"stack_trace":      strings.Join(gstError.StackTrace, "\n"),
		}).Debug("Error stack trace")
	}
}

// analyzeErrorPatterns analyzes error patterns and sequences
func (eec *EnhancedErrorContext) analyzeErrorPatterns(trackedError *TrackedError) {
	// Look for error sequences
	recentErrors := eec.getRecentErrors(5 * time.Minute)
	if len(recentErrors) >= 2 {
		sequence := eec.detectErrorSequence(recentErrors)
		if sequence != nil {
			eec.errorSequences = append(eec.errorSequences, *sequence)

			eec.logger.WithFields(logrus.Fields{
				"sequence_id":       sequence.ID,
				"pattern":           sequence.Pattern,
				"error_count":       len(sequence.ErrorIDs),
				"sequence_duration": sequence.EndTime.Sub(sequence.StartTime),
				"root_cause":        sequence.RootCause,
			}).Warn("Error sequence detected")
		}
	}
}

// UpdateErrorStatus updates the status of a tracked error
func (eec *EnhancedErrorContext) UpdateErrorStatus(errorID string, status TrackedErrorStatus, context map[string]interface{}) {
	eec.mu.Lock()
	defer eec.mu.Unlock()

	trackedError, exists := eec.activeErrors[errorID]
	if !exists {
		eec.logger.Warnf("Attempted to update status of unknown error: %s", errorID)
		return
	}

	oldStatus := trackedError.Status
	trackedError.Status = status
	trackedError.LastUpdate = time.Now()

	// Add context information
	if context != nil {
		for key, value := range context {
			trackedError.ContextData[key] = value
		}
	}

	eec.logger.WithFields(logrus.Fields{
		"error_id":   errorID,
		"old_status": oldStatus.String(),
		"new_status": status.String(),
		"component":  trackedError.Error.Component,
	}).Info("Error status updated")

	// Clean up resolved errors
	if status == ErrorStatusRecovered || status == ErrorStatusFailed {
		// Move to history and remove from active
		delete(eec.activeErrors, errorID)
		eec.logger.Debugf("Removed resolved error from active tracking: %s", errorID)
	}
}

// RecordRecoveryAttempt records an enhanced recovery attempt
func (eec *EnhancedErrorContext) RecordRecoveryAttempt(errorID string, attempt RecoveryAttempt, context map[string]interface{}) {
	eec.mu.Lock()
	defer eec.mu.Unlock()

	trackedError, exists := eec.activeErrors[errorID]
	if !exists {
		eec.logger.Warnf("Attempted to record recovery for unknown error: %s", errorID)
		return
	}

	// Create enhanced recovery attempt
	enhancedAttempt := EnhancedRecoveryAttempt{
		RecoveryAttempt: attempt,
		ContextSnapshot: make(map[string]interface{}),
		SystemState:     make(map[string]interface{}),
		PreConditions:   make([]string, 0),
		PostConditions:  make([]string, 0),
	}

	// Add memory usage
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	enhancedAttempt.MemoryUsage = m.Alloc

	// Copy context
	if context != nil {
		for key, value := range context {
			enhancedAttempt.ContextSnapshot[key] = value
		}
	}

	// Add system state information
	enhancedAttempt.SystemState["timestamp"] = time.Now()
	enhancedAttempt.SystemState["goroutine_count"] = runtime.NumGoroutine()
	enhancedAttempt.SystemState["memory_alloc"] = m.Alloc

	trackedError.RecoveryAttempts = append(trackedError.RecoveryAttempts, enhancedAttempt)

	// Create recovery event
	recoveryEvent := RecoveryEvent{
		Timestamp:      attempt.Timestamp,
		ErrorID:        errorID,
		Action:         attempt.Action,
		Success:        attempt.Success,
		Duration:       attempt.Duration,
		Context:        enhancedAttempt.ContextSnapshot,
		SystemImpact:   eec.assessSystemImpact(attempt),
		LessonsLearned: eec.extractLessonsLearned(attempt, trackedError),
	}

	eec.recoveryHistory = append(eec.recoveryHistory, recoveryEvent)

	eec.logger.WithFields(logrus.Fields{
		"error_id":        errorID,
		"recovery_action": attempt.Action.String(),
		"success":         attempt.Success,
		"duration":        attempt.Duration,
		"memory_usage":    enhancedAttempt.MemoryUsage,
		"attempt_count":   len(trackedError.RecoveryAttempts),
	}).Info("Enhanced recovery attempt recorded")
}

// Helper methods

func (eec *EnhancedErrorContext) findRelatedErrors(gstError *GStreamerError) []string {
	var related []string

	for id, trackedError := range eec.activeErrors {
		if id == gstError.Code {
			continue
		}

		// Check for same component
		if trackedError.Error.Component == gstError.Component {
			related = append(related, id)
		}

		// Check for same error type
		if trackedError.Error.Type == gstError.Type {
			related = append(related, id)
		}
	}

	return related
}

func (eec *EnhancedErrorContext) getRecentErrors(duration time.Duration) []*TrackedError {
	cutoff := time.Now().Add(-duration)
	var recent []*TrackedError

	for _, trackedError := range eec.activeErrors {
		if trackedError.StartTime.After(cutoff) {
			recent = append(recent, trackedError)
		}
	}

	return recent
}

func (eec *EnhancedErrorContext) detectErrorSequence(errors []*TrackedError) *ErrorSequence {
	if len(errors) < 2 {
		return nil
	}

	// Simple pattern detection - same component errors in sequence
	component := errors[0].Error.Component
	allSameComponent := true

	for _, err := range errors {
		if err.Error.Component != component {
			allSameComponent = false
			break
		}
	}

	if allSameComponent {
		errorIDs := make([]string, len(errors))
		for i, err := range errors {
			errorIDs[i] = err.ID
		}

		return &ErrorSequence{
			ID:        fmt.Sprintf("seq_%s_%d", component, time.Now().Unix()),
			StartTime: errors[0].StartTime,
			EndTime:   errors[len(errors)-1].StartTime,
			ErrorIDs:  errorIDs,
			Pattern:   fmt.Sprintf("component_%s_sequence", component),
			Component: component,
			RootCause: "Component instability detected",
		}
	}

	return nil
}

func (eec *EnhancedErrorContext) assessSystemImpact(attempt RecoveryAttempt) string {
	switch attempt.Action {
	case RecoveryRestart:
		return "High - Component restart required"
	case RecoveryReconfigure:
		return "Medium - Configuration changes required"
	case RecoveryFallback:
		return "Medium - Alternative implementation used"
	case RecoveryRetry:
		return "Low - Simple retry operation"
	case RecoveryManualIntervention:
		return "Critical - Manual intervention required"
	default:
		return "Unknown"
	}
}

func (eec *EnhancedErrorContext) extractLessonsLearned(attempt RecoveryAttempt, trackedError *TrackedError) []string {
	var lessons []string

	if attempt.Success {
		lessons = append(lessons, fmt.Sprintf("Recovery action %s successful for %s errors",
			attempt.Action.String(), trackedError.Error.Type.String()))
	} else {
		lessons = append(lessons, fmt.Sprintf("Recovery action %s failed for %s errors - consider alternative approaches",
			attempt.Action.String(), trackedError.Error.Type.String()))
	}

	// Add component-specific lessons
	if trackedError.Error.Component != "" {
		lessons = append(lessons, fmt.Sprintf("Component %s requires attention for %s error types",
			trackedError.Error.Component, trackedError.Error.Type.String()))
	}

	return lessons
}

// Utility functions

func getGoroutineID() int {
	// This is a simplified implementation
	// In production, you might want to use a more robust method
	return runtime.NumGoroutine()
}

func captureStackTrace(skip int) []string {
	const maxDepth = 20
	pcs := make([]uintptr, maxDepth)
	n := runtime.Callers(skip, pcs)

	frames := runtime.CallersFrames(pcs[:n])
	var stackTrace []string

	for {
		frame, more := frames.Next()
		stackTrace = append(stackTrace, fmt.Sprintf("%s:%d %s", frame.File, frame.Line, frame.Function))
		if !more {
			break
		}
	}

	return stackTrace
}

// GetActiveErrors returns a copy of all active errors
func (eec *EnhancedErrorContext) GetActiveErrors() map[string]*TrackedError {
	eec.mu.RLock()
	defer eec.mu.RUnlock()

	active := make(map[string]*TrackedError)
	for id, trackedError := range eec.activeErrors {
		// Create a copy to prevent external modification
		active[id] = &TrackedError{
			ID:               trackedError.ID,
			Error:            trackedError.Error,
			DebugContext:     trackedError.DebugContext,
			StartTime:        trackedError.StartTime,
			LastUpdate:       trackedError.LastUpdate,
			RecoveryAttempts: make([]EnhancedRecoveryAttempt, len(trackedError.RecoveryAttempts)),
			RelatedErrors:    make([]string, len(trackedError.RelatedErrors)),
			ContextData:      make(map[string]interface{}),
			Status:           trackedError.Status,
		}

		// Copy recovery attempts
		copy(active[id].RecoveryAttempts, trackedError.RecoveryAttempts)

		// Copy related errors
		copy(active[id].RelatedErrors, trackedError.RelatedErrors)

		// Copy context data
		for key, value := range trackedError.ContextData {
			active[id].ContextData[key] = value
		}
	}

	return active
}

// GetErrorSequences returns a copy of detected error sequences
func (eec *EnhancedErrorContext) GetErrorSequences() []ErrorSequence {
	eec.mu.RLock()
	defer eec.mu.RUnlock()

	sequences := make([]ErrorSequence, len(eec.errorSequences))
	copy(sequences, eec.errorSequences)
	return sequences
}

// GetRecoveryHistory returns a copy of recovery history
func (eec *EnhancedErrorContext) GetRecoveryHistory() []RecoveryEvent {
	eec.mu.RLock()
	defer eec.mu.RUnlock()

	history := make([]RecoveryEvent, len(eec.recoveryHistory))
	copy(history, eec.recoveryHistory)
	return history
}
