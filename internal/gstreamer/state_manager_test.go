package gstreamer

import (
	"fmt"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewStateManager(t *testing.T) {
	logger := logrus.NewEntry(logrus.New())

	// Test with default config
	sm := NewStateManager(nil, logger)
	assert.NotNil(t, sm)
	assert.NotNil(t, sm.config)
	assert.Equal(t, 10*time.Second, sm.config.StateChangeTimeout)
	assert.Equal(t, 3, sm.config.MaxRetryAttempts)
	assert.Equal(t, PipelineStateNull, sm.currentState)

	// Test with custom config
	config := &StateManagerConfig{
		StateChangeTimeout: 5 * time.Second,
		MaxRetryAttempts:   2,
	}
	sm2 := NewStateManager(config, logger)
	assert.NotNil(t, sm2)
	assert.Equal(t, 5*time.Second, sm2.config.StateChangeTimeout)
	assert.Equal(t, 2, sm2.config.MaxRetryAttempts)
}

func TestStateManagerLifecycle(t *testing.T) {
	logger := logrus.NewEntry(logrus.New())
	sm := NewStateManager(nil, logger)

	// Test start
	err := sm.Start()
	assert.NoError(t, err)
	assert.True(t, sm.running)

	// Test start when already running
	err = sm.Start()
	assert.Error(t, err)

	// Test stop
	err = sm.Stop()
	assert.NoError(t, err)
	assert.False(t, sm.running)

	// Test stop when not running
	err = sm.Stop()
	assert.NoError(t, err)
}

func TestGetCurrentState(t *testing.T) {
	logger := logrus.NewEntry(logrus.New())
	sm := NewStateManager(nil, logger)

	// Initial state should be NULL
	state := sm.GetCurrentState()
	assert.Equal(t, PipelineStateNull, state)

	// Force state change
	sm.ForceStateChange(PipelineStatePlaying)
	state = sm.GetCurrentState()
	assert.Equal(t, PipelineStatePlaying, state)
}

func TestGetTargetState(t *testing.T) {
	logger := logrus.NewEntry(logrus.New())
	sm := NewStateManager(nil, logger)

	// Initial target state should be NULL
	state := sm.GetTargetState()
	assert.Equal(t, PipelineStateNull, state)

	// Request state change
	sm.stateMutex.Lock()
	sm.targetState = PipelineStatePlaying
	sm.stateMutex.Unlock()

	state = sm.GetTargetState()
	assert.Equal(t, PipelineStatePlaying, state)
}

func TestRequestStateChange(t *testing.T) {
	logger := logrus.NewEntry(logrus.New())
	config := &StateManagerConfig{
		StateChangeTimeout: 1 * time.Second,
		MaxRetryAttempts:   1,
		RetryDelay:         100 * time.Millisecond,
		StateHistorySize:   10,
	}
	sm := NewStateManager(config, logger)

	err := sm.Start()
	require.NoError(t, err)
	defer sm.Stop()

	// Test successful state change (same state)
	err = sm.RequestStateChange(PipelineStateNull)
	assert.NoError(t, err)

	// Test state change to different state
	err = sm.RequestStateChange(PipelineStateReady)
	assert.NoError(t, err) // Should succeed with mock implementation

	// Check that current state was updated
	assert.Equal(t, PipelineStateReady, sm.GetCurrentState())

	// Check history
	history := sm.GetStateHistory()
	assert.Len(t, history, 1)
	assert.Equal(t, PipelineStateNull, history[0].FromState)
	assert.Equal(t, PipelineStateReady, history[0].ToState)
	assert.True(t, history[0].Success)
}

func TestStateChangeCallback(t *testing.T) {
	logger := logrus.NewEntry(logrus.New())
	sm := NewStateManager(nil, logger)

	err := sm.Start()
	require.NoError(t, err)
	defer sm.Stop()

	// Set up callback
	callbackCalled := false
	var callbackOldState, callbackNewState PipelineState
	var callbackTransition StateTransition

	sm.SetStateChangeCallback(func(oldState, newState PipelineState, transition StateTransition) {
		callbackCalled = true
		callbackOldState = oldState
		callbackNewState = newState
		callbackTransition = transition
	})

	// Request state change
	err = sm.RequestStateChange(PipelineStateReady)
	assert.NoError(t, err)

	// Wait for callback
	time.Sleep(100 * time.Millisecond)

	// Check callback was called
	assert.True(t, callbackCalled)
	assert.Equal(t, PipelineStateNull, callbackOldState)
	assert.Equal(t, PipelineStateReady, callbackNewState)
	assert.True(t, callbackTransition.Success)
}

func TestStateHistory(t *testing.T) {
	history := NewStateHistory(3)

	// Add transitions
	for i := 0; i < 5; i++ {
		transition := StateTransition{
			FromState: PipelineStateNull,
			ToState:   PipelineStateReady,
			Timestamp: time.Now(),
			Success:   true,
		}
		history.Add(transition)
	}

	// Check history is limited to max size
	all := history.GetAll()
	assert.Len(t, all, 3)

	// Test GetRecent
	recent := history.GetRecent(2)
	assert.Len(t, recent, 2)

	// Test GetRecent with count larger than history
	recent = history.GetRecent(10)
	assert.Len(t, recent, 3)

	// Test GetRecent with zero count
	recent = history.GetRecent(0)
	assert.Nil(t, recent)
}

func TestValidateStateTransition(t *testing.T) {
	logger := logrus.NewEntry(logrus.New())
	sm := NewStateManager(nil, logger)

	tests := []struct {
		from     PipelineState
		to       PipelineState
		expected bool
	}{
		{PipelineStateNull, PipelineStateReady, true},
		{PipelineStateReady, PipelineStateNull, true},
		{PipelineStateReady, PipelineStatePaused, true},
		{PipelineStatePaused, PipelineStateReady, true},
		{PipelineStatePaused, PipelineStatePlaying, true},
		{PipelineStatePlaying, PipelineStatePaused, true},
		{PipelineStateError, PipelineStateNull, true},

		// Invalid transitions
		{PipelineStateNull, PipelineStatePlaying, false},
		{PipelineStateReady, PipelineStatePlaying, false},
		{PipelineStatePlaying, PipelineStateNull, false},
	}

	for _, test := range tests {
		err := sm.ValidateStateTransition(test.from, test.to)
		if test.expected {
			assert.NoError(t, err, "Transition %s -> %s should be valid", test.from, test.to)
		} else {
			assert.Error(t, err, "Transition %s -> %s should be invalid", test.from, test.to)
		}
	}
}

func TestForceStateChange(t *testing.T) {
	logger := logrus.NewEntry(logrus.New())
	sm := NewStateManager(nil, logger)

	// Force state change
	sm.ForceStateChange(PipelineStatePlaying)

	// Check state was changed
	assert.Equal(t, PipelineStatePlaying, sm.GetCurrentState())
	assert.Equal(t, PipelineStatePlaying, sm.GetTargetState())

	// Check history
	history := sm.GetStateHistory()
	assert.Len(t, history, 1)
	assert.Equal(t, PipelineStateNull, history[0].FromState)
	assert.Equal(t, PipelineStatePlaying, history[0].ToState)
	assert.True(t, history[0].Success)
}

func TestIsHealthy(t *testing.T) {
	logger := logrus.NewEntry(logrus.New())
	sm := NewStateManager(nil, logger)

	// Not healthy when not running
	assert.False(t, sm.IsHealthy())

	// Start state manager
	err := sm.Start()
	require.NoError(t, err)
	defer sm.Stop()

	// Healthy when running and states match
	assert.True(t, sm.IsHealthy())

	// Not healthy when states don't match
	sm.stateMutex.Lock()
	sm.targetState = PipelineStatePlaying
	sm.stateMutex.Unlock()

	assert.False(t, sm.IsHealthy())

	// Healthy again when states match
	sm.stateMutex.Lock()
	sm.currentState = PipelineStatePlaying
	sm.stateMutex.Unlock()

	assert.True(t, sm.IsHealthy())
}

func TestGetStatistics(t *testing.T) {
	logger := logrus.NewEntry(logrus.New())
	sm := NewStateManager(nil, logger)

	err := sm.Start()
	require.NoError(t, err)
	defer sm.Stop()

	// Initial statistics
	stats := sm.GetStatistics()
	assert.Equal(t, PipelineStateNull, stats.CurrentState)
	assert.Equal(t, int64(0), stats.StateChanges)
	assert.Equal(t, int64(0), stats.SuccessfulChanges)
	assert.Equal(t, int64(0), stats.FailedChanges)

	// Perform state change
	err = sm.RequestStateChange(PipelineStateReady)
	assert.NoError(t, err)

	// Check updated statistics
	stats = sm.GetStatistics()
	assert.Equal(t, PipelineStateReady, stats.CurrentState)
	assert.Equal(t, int64(1), stats.StateChanges)
	assert.Equal(t, int64(1), stats.SuccessfulChanges)
	assert.Equal(t, int64(0), stats.FailedChanges)
}

func TestStateManagerGetStatus(t *testing.T) {
	logger := logrus.NewEntry(logrus.New())
	sm := NewStateManager(nil, logger)

	status := sm.GetStatus()
	assert.NotNil(t, status)
	assert.Contains(t, status, "running")
	assert.Contains(t, status, "current_state")
	assert.Contains(t, status, "target_state")
	assert.Contains(t, status, "healthy")
	assert.Contains(t, status, "statistics")

	assert.False(t, status["running"].(bool))
	assert.Equal(t, "NULL", status["current_state"].(string))
	assert.Equal(t, "NULL", status["target_state"].(string))
	assert.False(t, status["healthy"].(bool))
}

func TestGetRecentStateHistory(t *testing.T) {
	logger := logrus.NewEntry(logrus.New())
	sm := NewStateManager(nil, logger)

	err := sm.Start()
	require.NoError(t, err)
	defer sm.Stop()

	// Perform multiple state changes
	states := []PipelineState{PipelineStateReady, PipelineStatePaused, PipelineStatePlaying}
	for _, state := range states {
		err = sm.RequestStateChange(state)
		assert.NoError(t, err)
	}

	// Get recent history
	recent := sm.GetRecentStateHistory(2)
	assert.Len(t, recent, 2)

	// Should have the most recent transitions
	assert.Equal(t, PipelineStatePaused, recent[0].ToState)
	assert.Equal(t, PipelineStatePlaying, recent[1].ToState)

	// Test with count larger than history
	recent = sm.GetRecentStateHistory(10)
	assert.Len(t, recent, 3)

	// Test with zero count
	recent = sm.GetRecentStateHistory(0)
	assert.Nil(t, recent)
}

func TestPipelineStateString(t *testing.T) {
	tests := []struct {
		state    PipelineState
		expected string
	}{
		{PipelineStateNull, "NULL"},
		{PipelineStateReady, "READY"},
		{PipelineStatePaused, "PAUSED"},
		{PipelineStatePlaying, "PLAYING"},
		{PipelineStateError, "ERROR"},
		{PipelineState(999), "UNKNOWN"},
	}

	for _, test := range tests {
		assert.Equal(t, test.expected, test.state.String())
	}
}

func TestComponentStateString(t *testing.T) {
	tests := []struct {
		state    ComponentState
		expected string
	}{
		{ComponentStateUninitialized, "UNINITIALIZED"},
		{ComponentStateInitialized, "INITIALIZED"},
		{ComponentStateStarted, "STARTED"},
		{ComponentStateStopped, "STOPPED"},
		{ComponentStateError, "ERROR"},
		{ComponentState(999), "UNKNOWN"},
	}

	for _, test := range tests {
		assert.Equal(t, test.expected, test.state.String())
	}
}

func TestStateManagerHealthCheck(t *testing.T) {
	logger := logrus.NewEntry(logrus.New())
	config := &StateManagerConfig{
		HealthCheckInterval: 100 * time.Millisecond,
		StateHistorySize:    10,
	}
	sm := NewStateManager(config, logger)

	err := sm.Start()
	require.NoError(t, err)
	defer sm.Stop()

	// Wait for a few health checks
	time.Sleep(350 * time.Millisecond)

	// Check that health checks were performed
	stats := sm.GetStatistics()
	assert.Greater(t, stats.HealthChecksPassed, int64(0))
}

func TestStateTransitionRetry(t *testing.T) {
	logger := logrus.NewEntry(logrus.New())
	config := &StateManagerConfig{
		StateChangeTimeout: 100 * time.Millisecond,
		MaxRetryAttempts:   2,
		RetryDelay:         50 * time.Millisecond,
		StateHistorySize:   10,
	}
	sm := NewStateManager(config, logger)

	// Override performStateChange to simulate failure then success
	originalPerformStateChange := sm.performStateChange
	callCount := 0
	sm.performStateChange = func(fromState, toState PipelineState) error {
		callCount++
		if callCount == 1 {
			return fmt.Errorf("simulated failure")
		}
		return originalPerformStateChange(fromState, toState)
	}

	err := sm.Start()
	require.NoError(t, err)
	defer sm.Stop()

	// Request state change that will fail first time
	err = sm.RequestStateChange(PipelineStateReady)
	assert.NoError(t, err) // Should succeed on retry

	// Check that retry was attempted
	assert.Equal(t, 2, callCount)

	// Check history shows retry
	history := sm.GetStateHistory()
	assert.Len(t, history, 2) // One failed, one successful
	assert.False(t, history[0].Success)
	assert.Equal(t, 0, history[0].Retry)
	assert.True(t, history[1].Success)
	assert.Equal(t, 1, history[1].Retry)
}
