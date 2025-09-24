package gstreamer

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-gst/go-gst/gst"
	"github.com/sirupsen/logrus"
)

// StateManagerConfig configuration for the state manager
type StateManagerConfig struct {
	StateChangeTimeout  time.Duration `yaml:"state_change_timeout" json:"state_change_timeout"`
	MaxRetryAttempts    int           `yaml:"max_retry_attempts" json:"max_retry_attempts"`
	RetryDelay          time.Duration `yaml:"retry_delay" json:"retry_delay"`
	HealthCheckInterval time.Duration `yaml:"health_check_interval" json:"health_check_interval"`
	StateHistorySize    int           `yaml:"state_history_size" json:"state_history_size"`
}

// DefaultStateManagerConfig returns default state manager configuration
func DefaultStateManagerConfig() *StateManagerConfig {
	return &StateManagerConfig{
		StateChangeTimeout:  10 * time.Second,
		MaxRetryAttempts:    3,
		RetryDelay:          2 * time.Second,
		HealthCheckInterval: 30 * time.Second,
		StateHistorySize:    100,
	}
}

// StateTransitionEvent extends StateTransition with additional fields
type StateTransitionEvent struct {
	StateTransition           // Embed the existing StateTransition
	Timestamp       time.Time `json:"timestamp"`
	Retry           int       `json:"retry"`
}

// StateHistory maintains a history of state transitions
type StateHistory struct {
	transitions []StateTransitionEvent
	maxSize     int
	mutex       sync.RWMutex
}

// NewStateHistory creates a new state history
func NewStateHistory(maxSize int) *StateHistory {
	return &StateHistory{
		transitions: make([]StateTransitionEvent, 0, maxSize),
		maxSize:     maxSize,
	}
}

// Add adds a state transition to the history
func (sh *StateHistory) Add(transition StateTransitionEvent) {
	sh.mutex.Lock()
	defer sh.mutex.Unlock()

	// Add new transition
	sh.transitions = append(sh.transitions, transition)

	// Maintain max size by removing oldest entries
	if len(sh.transitions) > sh.maxSize {
		sh.transitions = sh.transitions[1:]
	}
}

// GetRecent returns the most recent transitions
func (sh *StateHistory) GetRecent(count int) []StateTransitionEvent {
	sh.mutex.RLock()
	defer sh.mutex.RUnlock()

	if count <= 0 || len(sh.transitions) == 0 {
		return nil
	}

	start := len(sh.transitions) - count
	if start < 0 {
		start = 0
	}

	result := make([]StateTransitionEvent, len(sh.transitions)-start)
	copy(result, sh.transitions[start:])
	return result
}

// GetAll returns all transitions in the history
func (sh *StateHistory) GetAll() []StateTransitionEvent {
	sh.mutex.RLock()
	defer sh.mutex.RUnlock()

	result := make([]StateTransitionEvent, len(sh.transitions))
	copy(result, sh.transitions)
	return result
}

// StateStatistics holds state management statistics
type StateStatistics struct {
	CurrentState       PipelineState `json:"current_state"`
	StateChanges       int64         `json:"state_changes"`
	SuccessfulChanges  int64         `json:"successful_changes"`
	FailedChanges      int64         `json:"failed_changes"`
	RetryAttempts      int64         `json:"retry_attempts"`
	AverageChangeTime  time.Duration `json:"average_change_time"`
	LastStateChange    time.Time     `json:"last_state_change"`
	LastError          error         `json:"last_error,omitempty"`
	LastErrorTime      time.Time     `json:"last_error_time,omitempty"`
	HealthChecksPassed int64         `json:"health_checks_passed"`
	HealthChecksFailed int64         `json:"health_checks_failed"`
}

// StateChangeCallback is called when a state change occurs
type StateChangeCallback func(oldState, newState PipelineState, transition StateTransitionEvent)

// StateManager manages pipeline state transitions and monitoring
type StateManager struct {
	config *StateManagerConfig
	logger *logrus.Entry

	// Current state tracking
	currentState PipelineState
	targetState  PipelineState
	stateMutex   sync.RWMutex

	// State history and statistics
	history *StateHistory
	stats   StateStatistics

	// Callbacks
	stateChangeCallback StateChangeCallback
	callbackMutex       sync.RWMutex

	// Lifecycle management
	ctx      context.Context
	cancel   context.CancelFunc
	running  bool
	stopChan chan struct{}
	wg       sync.WaitGroup
}

// NewStateManager creates a new state manager
func NewStateManager(config *StateManagerConfig, logger *logrus.Entry) *StateManager {
	if config == nil {
		config = DefaultStateManagerConfig()
	}
	if logger == nil {
		logger = logrus.NewEntry(logrus.StandardLogger())
	}

	sm := &StateManager{
		config:       config,
		logger:       logger,
		currentState: PipelineStateNull,
		targetState:  PipelineStateNull,
		history:      NewStateHistory(config.StateHistorySize),
		stopChan:     make(chan struct{}),
		stats: StateStatistics{
			CurrentState: PipelineStateNull,
		},
	}

	sm.logger.Debug("State manager created successfully")
	return sm
}

// Start starts the state manager
func (sm *StateManager) Start() error {
	sm.stateMutex.Lock()
	defer sm.stateMutex.Unlock()

	if sm.running {
		return fmt.Errorf("state manager already running")
	}

	sm.logger.Debug("Starting state manager")

	// Create context for lifecycle management
	sm.ctx, sm.cancel = context.WithCancel(context.Background())

	// Start health check monitoring
	sm.wg.Add(1)
	go sm.healthCheckLoop()

	sm.running = true
	sm.logger.Info("State manager started successfully")
	return nil
}

// Stop stops the state manager
func (sm *StateManager) Stop() error {
	sm.stateMutex.Lock()
	defer sm.stateMutex.Unlock()

	if !sm.running {
		return nil
	}

	sm.logger.Debug("Stopping state manager")

	// Cancel context and stop background tasks
	sm.cancel()
	close(sm.stopChan)
	sm.wg.Wait()

	sm.running = false
	sm.logger.Info("State manager stopped successfully")
	return nil
}

// SetStateChangeCallback sets the callback for state changes
func (sm *StateManager) SetStateChangeCallback(callback StateChangeCallback) {
	sm.callbackMutex.Lock()
	defer sm.callbackMutex.Unlock()
	sm.stateChangeCallback = callback
}

// GetCurrentState returns the current pipeline state
func (sm *StateManager) GetCurrentState() PipelineState {
	sm.stateMutex.RLock()
	defer sm.stateMutex.RUnlock()
	return sm.currentState
}

// GetTargetState returns the target pipeline state
func (sm *StateManager) GetTargetState() PipelineState {
	sm.stateMutex.RLock()
	defer sm.stateMutex.RUnlock()
	return sm.targetState
}

// RequestStateChange requests a state change with retry logic
func (sm *StateManager) RequestStateChange(targetState PipelineState) error {
	sm.stateMutex.Lock()
	currentState := sm.currentState
	sm.targetState = targetState
	sm.stateMutex.Unlock()

	if currentState == targetState {
		sm.logger.Debugf("Already in target state: %s", targetState.String())
		return nil
	}

	sm.logger.Infof("Requesting state change: %s -> %s", currentState.String(), targetState.String())

	// Attempt state change with retries
	var lastError error
	for attempt := 0; attempt <= sm.config.MaxRetryAttempts; attempt++ {
		if attempt > 0 {
			sm.logger.Warnf("Retrying state change (attempt %d/%d): %s -> %s",
				attempt, sm.config.MaxRetryAttempts, currentState.String(), targetState.String())
			time.Sleep(sm.config.RetryDelay)
		}

		startTime := time.Now()
		err := sm.performStateChange(currentState, targetState)
		duration := time.Since(startTime)

		// Create transition record
		transition := StateTransitionEvent{
			StateTransition: StateTransition{
				From:     currentState,
				To:       targetState,
				Duration: duration,
				Success:  err == nil,
				Error:    err,
			},
			Timestamp: startTime,
			Retry:     attempt,
		}

		// Add to history
		sm.history.Add(transition)

		// Update statistics
		sm.updateStatistics(transition)

		if err == nil {
			// Success - update current state
			sm.stateMutex.Lock()
			sm.currentState = targetState
			sm.stateMutex.Unlock()

			sm.logger.Infof("State change successful: %s -> %s (took %v)",
				currentState.String(), targetState.String(), duration)

			// Call callback if set
			sm.callStateChangeCallback(currentState, targetState, transition)
			return nil
		}

		lastError = err
		sm.logger.Warnf("State change failed (attempt %d): %v", attempt+1, err)

		// Update retry statistics
		sm.stats.RetryAttempts++
	}

	sm.logger.Errorf("State change failed after %d attempts: %s -> %s, error: %v",
		sm.config.MaxRetryAttempts+1, currentState.String(), targetState.String(), lastError)

	return fmt.Errorf("state change failed after %d attempts: %w", sm.config.MaxRetryAttempts+1, lastError)
}

// performStateChange performs the actual state change
func (sm *StateManager) performStateChange(fromState, toState PipelineState) error {
	// This is a placeholder implementation
	// In a real implementation, this would interact with the actual GStreamer pipeline
	sm.logger.Debugf("Performing state change: %s -> %s", fromState.String(), toState.String())

	// Simulate state change logic
	switch toState {
	case PipelineStateNull:
		return sm.changeToNull()
	case PipelineStateReady:
		return sm.changeToReady()
	case PipelineStatePaused:
		return sm.changeToPaused()
	case PipelineStatePlaying:
		return sm.changeToPlaying()
	default:
		return fmt.Errorf("unsupported target state: %s", toState.String())
	}
}

// State change implementations
func (sm *StateManager) changeToNull() error {
	sm.logger.Debug("Changing to NULL state")
	// Implementation would set pipeline state to NULL
	return nil
}

func (sm *StateManager) changeToReady() error {
	sm.logger.Debug("Changing to READY state")
	// Implementation would set pipeline state to READY
	return nil
}

func (sm *StateManager) changeToPaused() error {
	sm.logger.Debug("Changing to PAUSED state")
	// Implementation would set pipeline state to PAUSED
	return nil
}

func (sm *StateManager) changeToPlaying() error {
	sm.logger.Debug("Changing to PLAYING state")
	// Implementation would set pipeline state to PLAYING
	return nil
}

// HandleStateChange handles state change messages from GStreamer bus
func (sm *StateManager) HandleStateChange(msg *gst.Message) {
	if msg == nil {
		return
	}

	// Extract state change information from GStreamer message
	oldState, newState := msg.ParseStateChanged()

	sm.logger.Debugf("GStreamer state change: %s -> %s",
		sm.gstStateToPipelineState(oldState).String(),
		sm.gstStateToPipelineState(newState).String())

	// Update current state
	pipelineNewState := sm.gstStateToPipelineState(newState)
	sm.stateMutex.Lock()
	oldPipelineState := sm.currentState
	sm.currentState = pipelineNewState
	sm.stateMutex.Unlock()

	// Create transition record
	transition := StateTransitionEvent{
		StateTransition: StateTransition{
			From:     oldPipelineState,
			To:       pipelineNewState,
			Duration: 0, // Duration not available from GStreamer message
			Success:  true,
			Error:    nil,
		},
		Timestamp: time.Now(),
		Retry:     0,
	}

	// Add to history
	sm.history.Add(transition)

	// Update statistics
	sm.updateStatistics(transition)

	// Call callback if set
	sm.callStateChangeCallback(oldPipelineState, pipelineNewState, transition)
}

// gstStateToPipelineState converts GStreamer state to pipeline state
func (sm *StateManager) gstStateToPipelineState(gstState gst.State) PipelineState {
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

// updateStatistics updates state management statistics
func (sm *StateManager) updateStatistics(transition StateTransitionEvent) {
	sm.stats.CurrentState = transition.To
	sm.stats.StateChanges++
	sm.stats.LastStateChange = transition.Timestamp

	if transition.Success {
		sm.stats.SuccessfulChanges++
	} else {
		sm.stats.FailedChanges++
		sm.stats.LastError = transition.Error
		sm.stats.LastErrorTime = transition.Timestamp
	}

	// Update average change time
	if transition.Duration > 0 {
		totalTime := sm.stats.AverageChangeTime * time.Duration(sm.stats.SuccessfulChanges-1)
		sm.stats.AverageChangeTime = (totalTime + transition.Duration) / time.Duration(sm.stats.SuccessfulChanges)
	}
}

// callStateChangeCallback calls the state change callback if set
func (sm *StateManager) callStateChangeCallback(oldState, newState PipelineState, transition StateTransitionEvent) {
	sm.callbackMutex.RLock()
	callback := sm.stateChangeCallback
	sm.callbackMutex.RUnlock()

	if callback != nil {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					sm.logger.Errorf("State change callback panic: %v", r)
				}
			}()
			callback(oldState, newState, transition)
		}()
	}
}

// GetStatistics returns state management statistics
func (sm *StateManager) GetStatistics() StateStatistics {
	return sm.stats
}

// GetStateHistory returns the state transition history
func (sm *StateManager) GetStateHistory() []StateTransitionEvent {
	return sm.history.GetAll()
}

// GetRecentStateHistory returns recent state transitions
func (sm *StateManager) GetRecentStateHistory(count int) []StateTransitionEvent {
	return sm.history.GetRecent(count)
}

// IsHealthy checks if the state manager is healthy
func (sm *StateManager) IsHealthy() bool {
	sm.stateMutex.RLock()
	defer sm.stateMutex.RUnlock()

	// Consider healthy if running and current state matches target state
	return sm.running && sm.currentState == sm.targetState
}

// GetStatus returns the current status of the state manager
func (sm *StateManager) GetStatus() map[string]interface{} {
	sm.stateMutex.RLock()
	defer sm.stateMutex.RUnlock()

	return map[string]interface{}{
		"running":       sm.running,
		"current_state": sm.currentState.String(),
		"target_state":  sm.targetState.String(),
		"healthy":       sm.IsHealthy(),
		"statistics":    sm.stats,
	}
}

// Background monitoring loops

// healthCheckLoop performs periodic health checks
func (sm *StateManager) healthCheckLoop() {
	defer sm.wg.Done()

	ticker := time.NewTicker(sm.config.HealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			sm.performHealthCheck()
		case <-sm.stopChan:
			return
		case <-sm.ctx.Done():
			return
		}
	}
}

// performHealthCheck performs a health check
func (sm *StateManager) performHealthCheck() {
	sm.logger.Trace("Performing state manager health check")

	healthy := sm.IsHealthy()

	if healthy {
		sm.stats.HealthChecksPassed++
		sm.logger.Trace("State manager health check passed")
	} else {
		sm.stats.HealthChecksFailed++
		sm.logger.Warn("State manager health check failed")

		// Log current state information for debugging
		sm.stateMutex.RLock()
		sm.logger.Warnf("Health check details - Current: %s, Target: %s, Running: %v",
			sm.currentState.String(), sm.targetState.String(), sm.running)
		sm.stateMutex.RUnlock()
	}
}

// ValidateStateTransition validates if a state transition is valid
func (sm *StateManager) ValidateStateTransition(fromState, toState PipelineState) error {
	// Define valid state transitions
	validTransitions := map[PipelineState][]PipelineState{
		PipelineStateNull:    {PipelineStateReady},
		PipelineStateReady:   {PipelineStateNull, PipelineStatePaused},
		PipelineStatePaused:  {PipelineStateReady, PipelineStatePlaying},
		PipelineStatePlaying: {PipelineStatePaused},
		PipelineStateError:   {PipelineStateNull}, // Can only recover by going to NULL
	}

	validTargets, exists := validTransitions[fromState]
	if !exists {
		return fmt.Errorf("no valid transitions defined for state: %s", fromState.String())
	}

	for _, validTarget := range validTargets {
		if validTarget == toState {
			return nil // Valid transition
		}
	}

	return fmt.Errorf("invalid state transition: %s -> %s", fromState.String(), toState.String())
}

// ForceStateChange forces a state change without validation (use with caution)
func (sm *StateManager) ForceStateChange(newState PipelineState) {
	sm.stateMutex.Lock()
	defer sm.stateMutex.Unlock()

	oldState := sm.currentState
	sm.currentState = newState
	sm.targetState = newState

	sm.logger.Warnf("Forced state change: %s -> %s", oldState.String(), newState.String())

	// Create transition record
	transition := StateTransitionEvent{
		StateTransition: StateTransition{
			From:     oldState,
			To:       newState,
			Duration: 0,
			Success:  true,
			Error:    nil,
		},
		Timestamp: time.Now(),
		Retry:     0,
	}

	// Add to history
	sm.history.Add(transition)

	// Update statistics
	sm.updateStatistics(transition)

	// Call callback if set
	sm.callStateChangeCallback(oldState, newState, transition)
}
