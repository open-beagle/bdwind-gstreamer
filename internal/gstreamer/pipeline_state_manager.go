package gstreamer

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-gst/go-gst/gst"
	"github.com/sirupsen/logrus"
)

// PipelineStateError is defined in pipeline_manager.go

// String method is defined in pipeline_manager.go

// ExtendedStateTransition extends the existing StateTransition with additional fields
type ExtendedStateTransition struct {
	StateTransition
	Timestamp time.Time
	Context   map[string]interface{}
}

// Note: ErrorType is already defined in element.go

// Note: ErrorEvent is already defined in element.go

// Note: HealthStatus and HealthStatusLevel are already defined in element.go
// Using HealthStatusLevel for individual status values

// PipelineStateManagerConfig holds configuration for the state manager
type PipelineStateManagerConfig struct {
	// State transition timeout
	StateTransitionTimeout time.Duration

	// Health check interval
	HealthCheckInterval time.Duration

	// Maximum number of state transitions to keep in history
	MaxStateHistory int

	// Maximum number of error events to keep in history
	MaxErrorHistory int

	// Auto-recovery settings
	EnableAutoRecovery  bool
	MaxRecoveryAttempts int
	RecoveryDelay       time.Duration

	// Monitoring settings
	EnableDetailedLogging bool
	LogStateTransitions   bool
	LogHealthChecks       bool
}

// DefaultPipelineStateManagerConfig returns default configuration
func DefaultPipelineStateManagerConfig() *PipelineStateManagerConfig {
	return &PipelineStateManagerConfig{
		StateTransitionTimeout: 10 * time.Second,
		HealthCheckInterval:    30 * time.Second,
		MaxStateHistory:        100,
		MaxErrorHistory:        50,
		EnableAutoRecovery:     true,
		MaxRecoveryAttempts:    3,
		RecoveryDelay:          2 * time.Second,
		EnableDetailedLogging:  true,
		LogStateTransitions:    true,
		LogHealthChecks:        false,
	}
}

// PipelineStateManager manages pipeline state transitions and health monitoring
type PipelineStateManager struct {
	// Configuration
	config *PipelineStateManagerConfig
	logger *logrus.Entry

	// Pipeline reference
	pipeline *gst.Pipeline

	// State management
	mu           sync.RWMutex
	currentState PipelineState
	targetState  PipelineState

	// State history
	stateHistory []ExtendedStateTransition

	// Error tracking
	errorHistory []ErrorEvent

	// Health monitoring
	healthChecker *PipelineHealthChecker

	// Callbacks
	stateChangeCallback func(PipelineState, PipelineState, ExtendedStateTransition)
	errorCallback       func(ErrorType, string, error)

	// Context and lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Recovery management
	recoveryInProgress bool
	recoveryAttempts   int
	lastRecoveryTime   time.Time

	// Metrics
	metrics *StateManagerMetrics
}

// StateManagerMetrics tracks state manager performance metrics
type StateManagerMetrics struct {
	mu sync.RWMutex

	TotalStateTransitions int64
	SuccessfulTransitions int64
	FailedTransitions     int64
	TotalErrors           int64
	RecoveryAttempts      int64
	SuccessfulRecoveries  int64
	FailedRecoveries      int64
	AverageTransitionTime time.Duration
	LastTransitionTime    time.Time
	UptimeSeconds         float64
	StartTime             time.Time
}

// NewPipelineStateManager creates a new pipeline state manager
func NewPipelineStateManager(ctx context.Context, pipeline *gst.Pipeline, logger *logrus.Entry, config *PipelineStateManagerConfig) *PipelineStateManager {
	if config == nil {
		config = DefaultPipelineStateManagerConfig()
	}

	if logger == nil {
		logger = logrus.WithField("component", "pipeline-state-manager")
	}

	childCtx, cancel := context.WithCancel(ctx)

	psm := &PipelineStateManager{
		config:       config,
		logger:       logger,
		pipeline:     pipeline,
		currentState: PipelineStateNull,
		targetState:  PipelineStateNull,
		stateHistory: make([]ExtendedStateTransition, 0, config.MaxStateHistory),
		errorHistory: make([]ErrorEvent, 0, config.MaxErrorHistory),
		ctx:          childCtx,
		cancel:       cancel,
		metrics: &StateManagerMetrics{
			StartTime: time.Now(),
		},
	}

	// Initialize health checker
	psm.healthChecker = NewPipelineHealthChecker(logger.WithField("subcomponent", "health-checker"), psm)

	return psm
}

// Start starts the pipeline state manager
func (psm *PipelineStateManager) Start() error {
	psm.mu.Lock()
	defer psm.mu.Unlock()

	psm.logger.Info("Starting pipeline state manager")

	// Start health monitoring
	psm.wg.Add(1)
	go psm.healthMonitorLoop()

	// Start bus message monitoring if pipeline is available
	if psm.pipeline != nil {
		psm.wg.Add(1)
		go psm.busMessageLoop()
	}

	psm.logger.Info("Pipeline state manager started successfully")
	return nil
}

// Stop stops the pipeline state manager
func (psm *PipelineStateManager) Stop() error {
	psm.logger.Info("Stopping pipeline state manager")

	// Cancel context to stop all goroutines
	psm.cancel()

	// Wait for all goroutines to finish
	psm.wg.Wait()

	psm.logger.Info("Pipeline state manager stopped successfully")
	return nil
}

// SetPipeline sets the pipeline to be managed
func (psm *PipelineStateManager) SetPipeline(pipeline *gst.Pipeline) {
	psm.mu.Lock()
	defer psm.mu.Unlock()

	psm.pipeline = pipeline
	psm.logger.Debug("Pipeline set for state management")
}

// GetCurrentState returns the current pipeline state
func (psm *PipelineStateManager) GetCurrentState() PipelineState {
	psm.mu.RLock()
	defer psm.mu.RUnlock()
	return psm.currentState
}

// GetTargetState returns the target pipeline state
func (psm *PipelineStateManager) GetTargetState() PipelineState {
	psm.mu.RLock()
	defer psm.mu.RUnlock()
	return psm.targetState
}

// SetState attempts to change the pipeline state
func (psm *PipelineStateManager) SetState(targetState PipelineState) error {
	psm.mu.Lock()
	defer psm.mu.Unlock()

	if psm.pipeline == nil {
		return fmt.Errorf("no pipeline set for state management")
	}

	oldState := psm.currentState
	psm.targetState = targetState

	psm.logger.Debugf("Attempting state transition: %s -> %s", oldState.String(), targetState.String())

	// Record transition start time
	transitionStart := time.Now()

	// Convert to GStreamer state
	gstState, err := psm.pipelineStateToGstState(targetState)
	if err != nil {
		return fmt.Errorf("invalid target state %s: %w", targetState.String(), err)
	}

	// Attempt state change
	err = psm.pipeline.SetState(gstState)
	if err != nil {
		// Record failed transition
		transition := ExtendedStateTransition{
			StateTransition: StateTransition{
				From:     oldState,
				To:       targetState,
				Duration: time.Since(transitionStart),
				Success:  false,
				Error:    err,
			},
			Timestamp: transitionStart,
			Context:   map[string]interface{}{"gst_state": gstState.String()},
		}

		psm.recordStateTransition(transition)
		psm.updateMetrics(transition)

		if psm.stateChangeCallback != nil {
			psm.stateChangeCallback(oldState, targetState, transition)
		}

		return fmt.Errorf("failed to set pipeline state to %s: %w", gstState.String(), err)
	}

	// Wait for state change to complete with timeout
	ctx, cancel := context.WithTimeout(psm.ctx, psm.config.StateTransitionTimeout)
	defer cancel()

	success := psm.waitForStateChange(ctx, gstState)

	// Update current state
	if success {
		psm.currentState = targetState
	}

	// Record transition
	transition := ExtendedStateTransition{
		StateTransition: StateTransition{
			From:     oldState,
			To:       targetState,
			Duration: time.Since(transitionStart),
			Success:  success,
			Error:    nil,
		},
		Timestamp: transitionStart,
		Context:   map[string]interface{}{"gst_state": gstState.String()},
	}

	if !success {
		transition.Error = fmt.Errorf("state transition timeout after %v", psm.config.StateTransitionTimeout)
	}

	psm.recordStateTransition(transition)
	psm.updateMetrics(transition)

	// Log transition
	if psm.config.LogStateTransitions {
		if success {
			psm.logger.Infof("State transition successful: %s -> %s (took %v)",
				oldState.String(), targetState.String(), transition.Duration)
		} else {
			psm.logger.Warnf("State transition failed: %s -> %s (took %v): %v",
				oldState.String(), targetState.String(), transition.Duration, transition.Error)
		}
	}

	// Notify callback
	if psm.stateChangeCallback != nil {
		psm.stateChangeCallback(oldState, targetState, transition)
	}

	// Attempt recovery if transition failed and auto-recovery is enabled
	if !success && psm.config.EnableAutoRecovery {
		go psm.attemptRecovery(oldState, targetState, transition.Error)
	}

	if !success {
		return transition.Error
	}

	return nil
}

// waitForStateChange waits for the pipeline to reach the target state
func (psm *PipelineStateManager) waitForStateChange(ctx context.Context, targetGstState gst.State) bool {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return false
		case <-ticker.C:
			currentGstState := psm.pipeline.GetCurrentState()
			if currentGstState == targetGstState {
				return true
			}
		}
	}
}

// pipelineStateToGstState converts PipelineState to gst.State
func (psm *PipelineStateManager) pipelineStateToGstState(state PipelineState) (gst.State, error) {
	switch state {
	case PipelineStateNull:
		return gst.StateNull, nil
	case PipelineStateReady:
		return gst.StateReady, nil
	case PipelineStatePaused:
		return gst.StatePaused, nil
	case PipelineStatePlaying:
		return gst.StatePlaying, nil
	default:
		return gst.StateNull, fmt.Errorf("invalid pipeline state: %s", state.String())
	}
}

// gstStateToPipelineState converts gst.State to PipelineState
func (psm *PipelineStateManager) gstStateToPipelineState(gstState gst.State) PipelineState {
	switch gstState {
	case gst.StateNull:
		return PipelineStateNull
	case gst.StateReady:
		return PipelineStateReady
	case gst.StatePaused:
		return PipelineStatePaused
	case gst.StatePlaying:
		return PipelineStatePlaying
	default:
		return PipelineStateError
	}
}

// recordStateTransition records a state transition in history
func (psm *PipelineStateManager) recordStateTransition(transition ExtendedStateTransition) {
	// Add to history
	psm.stateHistory = append(psm.stateHistory, transition)

	// Trim history if needed
	if len(psm.stateHistory) > psm.config.MaxStateHistory {
		psm.stateHistory = psm.stateHistory[1:]
	}
}

// updateMetrics updates state manager metrics
func (psm *PipelineStateManager) updateMetrics(transition ExtendedStateTransition) {
	psm.metrics.mu.Lock()
	defer psm.metrics.mu.Unlock()

	psm.metrics.TotalStateTransitions++
	psm.metrics.LastTransitionTime = transition.Timestamp

	if transition.Success {
		psm.metrics.SuccessfulTransitions++
	} else {
		psm.metrics.FailedTransitions++
	}

	// Update average transition time
	if psm.metrics.TotalStateTransitions == 1 {
		psm.metrics.AverageTransitionTime = transition.Duration
	} else {
		// Calculate running average
		total := psm.metrics.AverageTransitionTime * time.Duration(psm.metrics.TotalStateTransitions-1)
		psm.metrics.AverageTransitionTime = (total + transition.Duration) / time.Duration(psm.metrics.TotalStateTransitions)
	}

	// Update uptime
	psm.metrics.UptimeSeconds = time.Since(psm.metrics.StartTime).Seconds()
}

// SetStateChangeCallback sets the state change callback
func (psm *PipelineStateManager) SetStateChangeCallback(callback func(PipelineState, PipelineState, ExtendedStateTransition)) {
	psm.mu.Lock()
	defer psm.mu.Unlock()
	psm.stateChangeCallback = callback
}

// SetErrorCallback sets the error callback
func (psm *PipelineStateManager) SetErrorCallback(callback func(ErrorType, string, error)) {
	psm.mu.Lock()
	defer psm.mu.Unlock()
	psm.errorCallback = callback
}

// GetStateHistory returns the state transition history
func (psm *PipelineStateManager) GetStateHistory() []ExtendedStateTransition {
	psm.mu.RLock()
	defer psm.mu.RUnlock()

	history := make([]ExtendedStateTransition, len(psm.stateHistory))
	copy(history, psm.stateHistory)
	return history
}

// GetErrorHistory returns the error history
func (psm *PipelineStateManager) GetErrorHistory() []ErrorEvent {
	psm.mu.RLock()
	defer psm.mu.RUnlock()

	history := make([]ErrorEvent, len(psm.errorHistory))
	copy(history, psm.errorHistory)
	return history
}

// GetMetrics returns state manager metrics
func (psm *PipelineStateManager) GetMetrics() StateManagerMetrics {
	psm.metrics.mu.RLock()
	defer psm.metrics.mu.RUnlock()

	// Update uptime before returning
	metrics := *psm.metrics
	metrics.UptimeSeconds = time.Since(psm.metrics.StartTime).Seconds()

	return metrics
}

// RestartPipeline attempts to restart the pipeline
func (psm *PipelineStateManager) RestartPipeline() error {
	psm.logger.Info("Attempting pipeline restart")

	// Stop pipeline first
	if err := psm.SetState(PipelineStateNull); err != nil {
		psm.logger.Errorf("Failed to stop pipeline during restart: %v", err)
		return fmt.Errorf("failed to stop pipeline: %w", err)
	}

	// Wait a moment for cleanup
	time.Sleep(1 * time.Second)

	// Start pipeline again
	if err := psm.SetState(PipelineStatePlaying); err != nil {
		psm.logger.Errorf("Failed to start pipeline during restart: %v", err)
		return fmt.Errorf("failed to start pipeline: %w", err)
	}

	psm.logger.Info("Pipeline restart completed successfully")
	return nil
}

// attemptRecovery attempts to recover from a failed state transition
func (psm *PipelineStateManager) attemptRecovery(fromState, toState PipelineState, err error) {
	psm.mu.Lock()

	// Check if recovery is already in progress
	if psm.recoveryInProgress {
		psm.mu.Unlock()
		return
	}

	// Check recovery attempts limit
	if psm.recoveryAttempts >= psm.config.MaxRecoveryAttempts {
		psm.mu.Unlock()
		psm.logger.Warnf("Maximum recovery attempts (%d) reached, giving up", psm.config.MaxRecoveryAttempts)
		return
	}

	psm.recoveryInProgress = true
	psm.recoveryAttempts++
	psm.lastRecoveryTime = time.Now()

	psm.mu.Unlock()

	psm.logger.Infof("Attempting recovery %d/%d for failed state transition %s -> %s: %v",
		psm.recoveryAttempts, psm.config.MaxRecoveryAttempts, fromState.String(), toState.String(), err)

	// Update metrics
	psm.metrics.mu.Lock()
	psm.metrics.RecoveryAttempts++
	psm.metrics.mu.Unlock()

	// Wait before recovery attempt
	time.Sleep(psm.config.RecoveryDelay)

	// Try to recover by restarting the pipeline
	recoveryErr := psm.RestartPipeline()

	psm.mu.Lock()
	psm.recoveryInProgress = false
	psm.mu.Unlock()

	// Update metrics
	psm.metrics.mu.Lock()
	if recoveryErr == nil {
		psm.metrics.SuccessfulRecoveries++
		psm.logger.Infof("Recovery attempt %d successful", psm.recoveryAttempts)
		// Reset recovery attempts on success
		psm.mu.Lock()
		psm.recoveryAttempts = 0
		psm.mu.Unlock()
	} else {
		psm.metrics.FailedRecoveries++
		psm.logger.Errorf("Recovery attempt %d failed: %v", psm.recoveryAttempts, recoveryErr)
	}
	psm.metrics.mu.Unlock()
}

// handleError handles pipeline errors
func (psm *PipelineStateManager) handleError(errorType ErrorType, message, details, component string) {
	// Create error event
	errorEvent := ErrorEvent{
		Type:      convertErrorTypeToGStreamerErrorType(errorType),
		Message:   message + " - " + details,
		Source:    component,
		Timestamp: time.Now(),
	}

	// Record error
	psm.mu.Lock()
	psm.errorHistory = append(psm.errorHistory, errorEvent)
	if len(psm.errorHistory) > psm.config.MaxErrorHistory {
		psm.errorHistory = psm.errorHistory[1:]
	}
	psm.mu.Unlock()

	// Update metrics
	psm.metrics.mu.Lock()
	psm.metrics.TotalErrors++
	psm.metrics.mu.Unlock()

	// Get error severity
	severity := psm.getErrorSeverity(errorType)

	// Log error
	psm.logger.WithFields(logrus.Fields{
		"error_type": errorType.String(),
		"component":  component,
		"severity":   severity,
	}).Errorf("Pipeline error: %s - %s", message, details)

	// Notify callback
	if psm.errorCallback != nil {
		psm.errorCallback(errorType, message, fmt.Errorf("%s: %s", message, details))
	}

	// Set pipeline to error state for critical errors
	if severity == "Critical" || severity == "Fatal" {
		psm.mu.Lock()
		psm.currentState = PipelineStateError
		psm.mu.Unlock()
	}
}

// convertErrorTypeToGStreamerErrorType converts ErrorType to GStreamerErrorType
func convertErrorTypeToGStreamerErrorType(errorType ErrorType) GStreamerErrorType {
	switch errorType {
	case ErrorTypePipeline:
		return ErrorTypePipelineCreation
	case ErrorTypeElement:
		return ErrorTypeElementCreation
	case ErrorTypeResource:
		return ErrorTypeResourceAccess
	case ErrorMemoryExhaustion:
		return ErrorTypeMemoryAllocation
	case ErrorNetwork:
		return ErrorTypeNetwork
	case ErrorHardware:
		return ErrorTypeHardware
	case ErrorTimeout:
		return ErrorTypeTimeout
	case ErrorPermission:
		return ErrorTypePermission
	default:
		return ErrorTypeUnknown
	}
}

// getErrorSeverity determines error severity based on error type
func (psm *PipelineStateManager) getErrorSeverity(errorType ErrorType) string {
	switch errorType {
	case ErrorPermission, ErrorHardware:
		return "Fatal"
	case ErrorMemoryExhaustion, ErrorStateChange:
		return "Critical"
	case ErrorElementFailure, ErrorResourceUnavailable:
		return "Error"
	case ErrorFormatNegotiation, ErrorTimeout:
		return "Warning"
	default:
		return "Info"
	}
}

// healthMonitorLoop runs the health monitoring loop
func (psm *PipelineStateManager) healthMonitorLoop() {
	defer psm.wg.Done()

	ticker := time.NewTicker(psm.config.HealthCheckInterval)
	defer ticker.Stop()

	psm.logger.Debug("Health monitoring loop started")

	for {
		select {
		case <-psm.ctx.Done():
			psm.logger.Debug("Health monitoring loop stopped")
			return
		case <-ticker.C:
			psm.performHealthCheck()
		}
	}
}

// performHealthCheck performs a health check on the pipeline
func (psm *PipelineStateManager) performHealthCheck() {
	if psm.healthChecker != nil {
		healthStatus := psm.healthChecker.CheckHealth()

		if psm.config.LogHealthChecks {
			psm.logger.Debugf("Health check completed: %s", healthStatus.Overall.String())
		}

		// Handle unhealthy states
		if healthStatus.Overall == HealthStatusCritical && psm.config.EnableAutoRecovery {
			psm.logger.Warn("Critical health status detected, attempting recovery")
			go psm.attemptRecovery(psm.currentState, psm.targetState, fmt.Errorf("critical health status"))
		}
	}
}

// busMessageLoop monitors pipeline bus messages
func (psm *PipelineStateManager) busMessageLoop() {
	defer psm.wg.Done()

	if psm.pipeline == nil {
		psm.logger.Warn("No pipeline available for bus message monitoring")
		return
	}

	bus := psm.pipeline.GetPipelineBus()
	if bus == nil {
		psm.logger.Warn("No bus available for message monitoring")
		return
	}

	psm.logger.Debug("Bus message monitoring loop started")

	for {
		select {
		case <-psm.ctx.Done():
			psm.logger.Debug("Bus message monitoring loop stopped")
			return
		default:
			// Poll for messages with timeout
			msg := bus.TimedPop(gst.ClockTime(100 * time.Millisecond))
			if msg == nil {
				continue
			}

			psm.processBusMessage(msg)
		}
	}
}

// processBusMessage processes a bus message
func (psm *PipelineStateManager) processBusMessage(msg *gst.Message) {
	switch msg.Type() {
	case gst.MessageError:
		err := msg.ParseError()
		psm.handleError(ErrorElementFailure, "Pipeline error", err.Error(), msg.Source())

	case gst.MessageWarning:
		warning := msg.ParseWarning()
		psm.handleError(ErrorElementFailure, "Pipeline warning", warning.Error(), msg.Source())

	case gst.MessageStateChanged:
		// Handle state change messages
		if msg.Source() == psm.pipeline.GetName() {
			oldState, newState := msg.ParseStateChanged()
			psm.onPipelineStateChanged(oldState, newState)
		}

	case gst.MessageEOS:
		psm.logger.Info("End of stream received")

	case gst.MessageStreamStart:
		psm.logger.Debug("Stream started")

	case gst.MessageAsyncDone:
		psm.logger.Debug("Async operation completed")

	default:
		// Log other message types at trace level
		psm.logger.Tracef("Bus message: %s from %s", msg.Type().String(), msg.Source())
	}
}

// onPipelineStateChanged handles pipeline state change messages
func (psm *PipelineStateManager) onPipelineStateChanged(oldGstState, newGstState gst.State) {
	oldState := psm.gstStateToPipelineState(oldGstState)
	newState := psm.gstStateToPipelineState(newGstState)

	psm.mu.Lock()
	psm.currentState = newState
	psm.mu.Unlock()

	if psm.config.LogStateTransitions {
		psm.logger.Debugf("Pipeline state changed: %s -> %s", oldState.String(), newState.String())
	}
}

// HandleError exposes the internal handleError method for testing and external use
func (psm *PipelineStateManager) HandleError(errorType ErrorType, message, details, component string) {
	psm.handleError(errorType, message, details, component)
}

// GetStats returns comprehensive state manager statistics
func (psm *PipelineStateManager) GetStats() interface{} {
	metrics := psm.GetMetrics()

	psm.mu.RLock()
	currentState := psm.currentState
	targetState := psm.targetState
	recoveryInProgress := psm.recoveryInProgress
	recoveryAttempts := psm.recoveryAttempts
	psm.mu.RUnlock()

	var healthStatus interface{}
	if psm.healthChecker != nil {
		healthStatus = psm.healthChecker.GetHealthStatus()
	}

	return map[string]interface{}{
		"current_state":        currentState.String(),
		"target_state":         targetState.String(),
		"recovery_in_progress": recoveryInProgress,
		"recovery_attempts":    recoveryAttempts,
		"metrics":              metrics,
		"health_status":        healthStatus,
		"state_history_count":  len(psm.stateHistory),
		"error_history_count":  len(psm.errorHistory),
	}
}
