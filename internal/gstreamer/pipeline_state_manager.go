package gstreamer

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// PipelineStateManager manages GStreamer pipeline states and provides monitoring and recovery capabilities
type PipelineStateManager struct {
	pipeline Pipeline
	logger   *logrus.Entry
	mu       sync.RWMutex

	// State tracking
	currentState    PipelineState
	targetState     PipelineState
	lastStateChange time.Time
	stateHistory    []StateTransition

	// Health monitoring
	healthChecker *PipelineHealthChecker
	errorHandler  *PipelineErrorHandler

	// Configuration
	config *StateManagerConfig

	// Context and cancellation
	ctx    context.Context
	cancel context.CancelFunc

	// Callbacks
	stateChangeCallback func(old, new PipelineState, transition StateTransition)
	errorCallback       func(error, string)
}

// StateManagerConfig contains configuration for the pipeline state manager
type StateManagerConfig struct {
	// Health check interval
	HealthCheckInterval time.Duration

	// State transition timeout
	StateTransitionTimeout time.Duration

	// Maximum retry attempts for state changes
	MaxRetryAttempts int

	// Retry delay between attempts
	RetryDelay time.Duration

	// Enable automatic recovery
	AutoRecovery bool

	// Maximum state history entries to keep
	MaxStateHistorySize int
}

// DefaultStateManagerConfig returns default configuration for state manager
func DefaultStateManagerConfig() *StateManagerConfig {
	return &StateManagerConfig{
		HealthCheckInterval:    5 * time.Second,
		StateTransitionTimeout: 10 * time.Second,
		MaxRetryAttempts:       3,
		RetryDelay:             2 * time.Second,
		AutoRecovery:           true,
		MaxStateHistorySize:    100,
	}
}

// StateTransition represents a pipeline state transition
type StateTransition struct {
	FromState  PipelineState
	ToState    PipelineState
	Timestamp  time.Time
	Duration   time.Duration
	Success    bool
	Error      error
	RetryCount int
}

// NewPipelineStateManager creates a new pipeline state manager
func NewPipelineStateManager(ctx context.Context, pipeline Pipeline, logger *logrus.Entry, config *StateManagerConfig) *PipelineStateManager {
	if config == nil {
		config = DefaultStateManagerConfig()
	}

	if logger == nil {
		logger = logrus.WithField("component", "pipeline-state-manager")
	}

	childCtx, cancel := context.WithCancel(ctx)

	psm := &PipelineStateManager{
		pipeline:        pipeline,
		logger:          logger,
		config:          config,
		ctx:             childCtx,
		cancel:          cancel,
		currentState:    StateNull,
		targetState:     StateNull,
		lastStateChange: time.Now(),
		stateHistory:    make([]StateTransition, 0, config.MaxStateHistorySize),
	}

	// Initialize health checker
	psm.healthChecker = NewPipelineHealthChecker(childCtx, pipeline, logger, &HealthCheckerConfig{
		CheckInterval: config.HealthCheckInterval,
		StateManager:  psm,
	})

	// Initialize error handler
	psm.errorHandler = NewPipelineErrorHandler(logger, &ErrorHandlerConfig{
		MaxRetryAttempts: config.MaxRetryAttempts,
		RetryDelay:       config.RetryDelay,
		AutoRecovery:     config.AutoRecovery,
		StateManager:     psm,
	})

	return psm
}

// Start starts the pipeline state manager
func (psm *PipelineStateManager) Start() error {
	psm.mu.Lock()
	defer psm.mu.Unlock()

	psm.logger.Info("Starting pipeline state manager...")

	// Get initial pipeline state
	initialState, err := psm.pipeline.GetState()
	if err != nil {
		psm.logger.Warnf("Failed to get initial pipeline state: %v", err)
		initialState = StateNull
	}

	psm.currentState = initialState
	psm.targetState = initialState
	psm.lastStateChange = time.Now()

	// Start health checker
	if err := psm.healthChecker.Start(); err != nil {
		return fmt.Errorf("failed to start health checker: %w", err)
	}

	// Start state monitoring goroutine
	go psm.monitorStateTransitions()

	psm.logger.Infof("Pipeline state manager started, initial state: %v", initialState)
	return nil
}

// Stop stops the pipeline state manager
func (psm *PipelineStateManager) Stop() error {
	psm.mu.Lock()
	defer psm.mu.Unlock()

	psm.logger.Info("Stopping pipeline state manager...")

	// Stop health checker
	if err := psm.healthChecker.Stop(); err != nil {
		psm.logger.Warnf("Failed to stop health checker: %v", err)
	}

	// Cancel context to stop monitoring goroutines
	psm.cancel()

	psm.logger.Info("Pipeline state manager stopped")
	return nil
}

// SetState sets the target pipeline state and manages the transition
func (psm *PipelineStateManager) SetState(targetState PipelineState) error {
	psm.mu.Lock()
	currentState := psm.currentState
	psm.targetState = targetState
	psm.mu.Unlock()

	if currentState == targetState {
		psm.logger.Debugf("Pipeline already in target state: %v", targetState)
		return nil
	}

	psm.logger.Infof("Initiating state transition: %v -> %v", currentState, targetState)

	transition := StateTransition{
		FromState: currentState,
		ToState:   targetState,
		Timestamp: time.Now(),
	}

	// Attempt state change with retries
	err := psm.attemptStateChangeWithRetry(targetState, &transition)

	// Record transition in history
	psm.recordStateTransition(transition)

	// Notify callback if set
	if psm.stateChangeCallback != nil {
		psm.stateChangeCallback(currentState, targetState, transition)
	}

	return err
}

// attemptStateChangeWithRetry attempts to change pipeline state with retry logic
func (psm *PipelineStateManager) attemptStateChangeWithRetry(targetState PipelineState, transition *StateTransition) error {
	var lastErr error

	for attempt := 0; attempt < psm.config.MaxRetryAttempts; attempt++ {
		if attempt > 0 {
			psm.logger.Debugf("Retrying state change attempt %d/%d after %v",
				attempt+1, psm.config.MaxRetryAttempts, psm.config.RetryDelay)
			time.Sleep(psm.config.RetryDelay)
		}

		transition.RetryCount = attempt
		startTime := time.Now()

		// Attempt the state change
		err := psm.pipeline.SetState(targetState)
		if err != nil {
			lastErr = err
			psm.logger.Warnf("State change attempt %d failed: %v", attempt+1, err)
			continue
		}

		// Wait for state change to complete with timeout
		if err := psm.waitForStateChange(targetState, psm.config.StateTransitionTimeout); err != nil {
			lastErr = err
			psm.logger.Warnf("State change timeout on attempt %d: %v", attempt+1, err)
			continue
		}

		// Success
		transition.Duration = time.Since(startTime)
		transition.Success = true

		psm.mu.Lock()
		psm.currentState = targetState
		psm.lastStateChange = time.Now()
		psm.mu.Unlock()

		psm.logger.Infof("State transition successful: %v (took %v)", targetState, transition.Duration)
		return nil
	}

	// All attempts failed
	transition.Duration = time.Since(transition.Timestamp)
	transition.Success = false
	transition.Error = lastErr

	psm.logger.Errorf("State transition failed after %d attempts: %v", psm.config.MaxRetryAttempts, lastErr)

	// Trigger error handling
	if psm.errorCallback != nil {
		psm.errorCallback(lastErr, fmt.Sprintf("Failed to transition to state %v", targetState))
	}

	return fmt.Errorf("failed to change pipeline state to %v after %d attempts: %w",
		targetState, psm.config.MaxRetryAttempts, lastErr)
}

// waitForStateChange waits for the pipeline to reach the target state
func (psm *PipelineStateManager) waitForStateChange(targetState PipelineState, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(psm.ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for state change to %v", targetState)
		case <-ticker.C:
			currentState, err := psm.pipeline.GetState()
			if err != nil {
				psm.logger.Warnf("Failed to get pipeline state during transition: %v", err)
				continue
			}

			if currentState == targetState {
				return nil
			}
		}
	}
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

// GetStateHistory returns the state transition history
func (psm *PipelineStateManager) GetStateHistory() []StateTransition {
	psm.mu.RLock()
	defer psm.mu.RUnlock()

	// Return a copy to prevent external modification
	history := make([]StateTransition, len(psm.stateHistory))
	copy(history, psm.stateHistory)
	return history
}

// recordStateTransition records a state transition in the history
func (psm *PipelineStateManager) recordStateTransition(transition StateTransition) {
	psm.mu.Lock()
	defer psm.mu.Unlock()

	psm.stateHistory = append(psm.stateHistory, transition)

	// Trim history if it exceeds maximum size
	if len(psm.stateHistory) > psm.config.MaxStateHistorySize {
		psm.stateHistory = psm.stateHistory[1:]
	}
}

// monitorStateTransitions monitors pipeline state changes
func (psm *PipelineStateManager) monitorStateTransitions() {
	// Use the same interval as health checks for consistency monitoring
	ticker := time.NewTicker(psm.config.HealthCheckInterval)
	defer ticker.Stop()

	psm.logger.Debug("Starting pipeline state monitoring...")

	for {
		select {
		case <-psm.ctx.Done():
			psm.logger.Debug("Pipeline state monitoring stopped")
			return
		case <-ticker.C:
			psm.checkStateConsistency()
		}
	}
}

// checkStateConsistency checks if the actual pipeline state matches our tracked state
func (psm *PipelineStateManager) checkStateConsistency() {
	actualState, err := psm.pipeline.GetState()
	if err != nil {
		psm.logger.Warnf("Failed to get pipeline state for consistency check: %v", err)
		return
	}

	psm.mu.Lock()
	trackedState := psm.currentState

	if actualState != trackedState {
		psm.logger.Warnf("State inconsistency detected: tracked=%v, actual=%v", trackedState, actualState)

		// Update our tracked state
		psm.currentState = actualState
		psm.lastStateChange = time.Now()

		// Record unexpected transition
		transition := StateTransition{
			FromState: trackedState,
			ToState:   actualState,
			Timestamp: time.Now(),
			Success:   true,
			Error:     fmt.Errorf("unexpected state change detected"),
		}

		// Add to history while still holding the lock
		psm.stateHistory = append(psm.stateHistory, transition)
		if len(psm.stateHistory) > psm.config.MaxStateHistorySize {
			psm.stateHistory = psm.stateHistory[1:]
		}

		// Get callback reference while holding lock
		callback := psm.stateChangeCallback
		psm.mu.Unlock()

		// Notify callback outside of lock to avoid deadlock
		if callback != nil {
			callback(trackedState, actualState, transition)
		}
	} else {
		psm.mu.Unlock()
	}
}

// SetStateChangeCallback sets a callback for state change notifications
func (psm *PipelineStateManager) SetStateChangeCallback(callback func(old, new PipelineState, transition StateTransition)) {
	psm.mu.Lock()
	defer psm.mu.Unlock()
	psm.stateChangeCallback = callback
}

// SetErrorCallback sets a callback for error notifications
func (psm *PipelineStateManager) SetErrorCallback(callback func(error, string)) {
	psm.mu.Lock()
	defer psm.mu.Unlock()
	psm.errorCallback = callback
}

// RestartPipeline attempts to restart the pipeline
func (psm *PipelineStateManager) RestartPipeline() error {
	psm.logger.Info("Attempting to restart pipeline...")

	// Stop pipeline first
	if err := psm.SetState(StateNull); err != nil {
		psm.logger.Warnf("Failed to stop pipeline during restart: %v", err)
	}

	// Wait a moment for cleanup
	time.Sleep(1 * time.Second)

	// Start pipeline again
	if err := psm.SetState(StatePlaying); err != nil {
		return fmt.Errorf("failed to restart pipeline: %w", err)
	}

	psm.logger.Info("Pipeline restarted successfully")
	return nil
}

// GetStats returns statistics about the state manager
func (psm *PipelineStateManager) GetStats() map[string]interface{} {
	psm.mu.RLock()
	defer psm.mu.RUnlock()

	stats := map[string]interface{}{
		"current_state":      psm.currentState,
		"target_state":       psm.targetState,
		"last_state_change":  psm.lastStateChange,
		"state_history_size": len(psm.stateHistory),
	}

	// Add transition statistics
	var successfulTransitions, failedTransitions int
	var totalTransitionTime time.Duration

	for _, transition := range psm.stateHistory {
		if transition.Success {
			successfulTransitions++
			totalTransitionTime += transition.Duration
		} else {
			failedTransitions++
		}
	}

	stats["successful_transitions"] = successfulTransitions
	stats["failed_transitions"] = failedTransitions

	if successfulTransitions > 0 {
		stats["average_transition_time"] = totalTransitionTime / time.Duration(successfulTransitions)
	}

	return stats
}
