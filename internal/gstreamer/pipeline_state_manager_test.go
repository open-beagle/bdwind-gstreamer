package gstreamer

import (
	"context"
	"fmt"
	"log"
	"testing"
	"time"
)

// mockPipeline implements the Pipeline interface for testing
type mockPipeline struct {
	currentState PipelineState
	shouldFail   bool
	stateDelay   time.Duration
}

func (mp *mockPipeline) Create() error {
	return nil
}

func (mp *mockPipeline) CreateFromString(pipelineStr string) error {
	return nil
}

func (mp *mockPipeline) Start() error {
	return mp.SetState(StatePlaying)
}

func (mp *mockPipeline) Stop() error {
	return mp.SetState(StateNull)
}

func (mp *mockPipeline) GetElement(name string) (Element, error) {
	return nil, nil
}

func (mp *mockPipeline) AddElement(element Element) error {
	return nil
}

func (mp *mockPipeline) LinkElements(src, sink Element) error {
	return nil
}

func (mp *mockPipeline) LinkPad(srcElement, srcPad, sinkElement, sinkPad string) error {
	return nil
}

func (mp *mockPipeline) SetState(state PipelineState) error {
	if mp.shouldFail {
		return fmt.Errorf("mock pipeline failure")
	}

	// Simulate state change delay
	if mp.stateDelay > 0 {
		time.Sleep(mp.stateDelay)
	}

	mp.currentState = state
	return nil
}

func (mp *mockPipeline) GetState() (PipelineState, error) {
	if mp.shouldFail {
		return StateNull, fmt.Errorf("mock pipeline failure")
	}
	return mp.currentState, nil
}

func (mp *mockPipeline) GetBus() (Bus, error) {
	return &mockBus{}, nil
}

func (mp *mockPipeline) SendEOS() error {
	return nil
}

// mockBus implements the Bus interface for testing
type mockBus struct{}

func (mb *mockBus) AddWatch(callback BusCallback) (uint, error) {
	return 1, nil
}

func (mb *mockBus) RemoveWatch(id uint) error {
	return nil
}

func (mb *mockBus) Pop() (Message, error) {
	return nil, nil // No messages
}

func (mb *mockBus) TimedPop(timeout uint64) (Message, error) {
	return nil, nil // No messages
}

func TestPipelineStateManager_BasicStateTransitions(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockPipeline := &mockPipeline{currentState: StateNull}
	logger := log.New(log.Writer(), "[TEST] ", log.LstdFlags)

	config := DefaultStateManagerConfig()
	config.HealthCheckInterval = 100 * time.Millisecond // Faster for testing

	psm := NewPipelineStateManager(ctx, mockPipeline, logger, config)

	// Start the state manager
	err := psm.Start()
	if err != nil {
		t.Fatalf("Failed to start pipeline state manager: %v", err)
	}
	defer psm.Stop()

	// Test state transition to PLAYING
	err = psm.SetState(StatePlaying)
	if err != nil {
		t.Fatalf("Failed to set state to PLAYING: %v", err)
	}

	// Verify state
	currentState := psm.GetCurrentState()
	if currentState != StatePlaying {
		t.Errorf("Expected state PLAYING, got %v", currentState)
	}

	// Test state transition to PAUSED
	err = psm.SetState(StatePaused)
	if err != nil {
		t.Fatalf("Failed to set state to PAUSED: %v", err)
	}

	// Verify state
	currentState = psm.GetCurrentState()
	if currentState != StatePaused {
		t.Errorf("Expected state PAUSED, got %v", currentState)
	}

	// Test state transition back to NULL
	err = psm.SetState(StateNull)
	if err != nil {
		t.Fatalf("Failed to set state to NULL: %v", err)
	}

	// Verify state
	currentState = psm.GetCurrentState()
	if currentState != StateNull {
		t.Errorf("Expected state NULL, got %v", currentState)
	}
}

func TestPipelineStateManager_StateTransitionHistory(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockPipeline := &mockPipeline{currentState: StateNull}
	logger := log.New(log.Writer(), "[TEST] ", log.LstdFlags)

	psm := NewPipelineStateManager(ctx, mockPipeline, logger, nil)

	err := psm.Start()
	if err != nil {
		t.Fatalf("Failed to start pipeline state manager: %v", err)
	}
	defer psm.Stop()

	// Perform several state transitions
	states := []PipelineState{StatePlaying, StatePaused, StatePlaying, StateNull}

	for _, state := range states {
		err = psm.SetState(state)
		if err != nil {
			t.Fatalf("Failed to set state to %v: %v", state, err)
		}
	}

	// Check history
	history := psm.GetStateHistory()
	if len(history) != len(states) {
		t.Errorf("Expected %d transitions in history, got %d", len(states), len(history))
	}

	// Verify each transition
	for i, transition := range history {
		if transition.ToState != states[i] {
			t.Errorf("Transition %d: expected ToState %v, got %v", i, states[i], transition.ToState)
		}
		if !transition.Success {
			t.Errorf("Transition %d: expected success, got failure", i)
		}
	}
}

func TestPipelineStateManager_ErrorHandling(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockPipeline := &mockPipeline{
		currentState: StateNull,
		shouldFail:   true, // Make pipeline operations fail
	}
	logger := log.New(log.Writer(), "[TEST] ", log.LstdFlags)

	config := DefaultStateManagerConfig()
	config.MaxRetryAttempts = 2 // Limit retries for faster testing

	psm := NewPipelineStateManager(ctx, mockPipeline, logger, config)

	err := psm.Start()
	if err != nil {
		t.Fatalf("Failed to start pipeline state manager: %v", err)
	}
	defer psm.Stop()

	// Try to set state - should fail
	err = psm.SetState(StatePlaying)
	if err == nil {
		t.Error("Expected state transition to fail, but it succeeded")
	}

	// Check that state remained unchanged
	currentState := psm.GetCurrentState()
	if currentState != StateNull {
		t.Errorf("Expected state to remain NULL after failed transition, got %v", currentState)
	}

	// Check history for failed transition
	history := psm.GetStateHistory()
	if len(history) == 0 {
		t.Error("Expected failed transition to be recorded in history")
	} else {
		lastTransition := history[len(history)-1]
		if lastTransition.Success {
			t.Error("Expected last transition to be marked as failed")
		}
		if lastTransition.Error == nil {
			t.Error("Expected error to be recorded in failed transition")
		}
	}
}

func TestPipelineStateManager_StateConsistencyCheck(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockPipeline := &mockPipeline{currentState: StateNull}
	logger := log.New(log.Writer(), "[TEST] ", log.LstdFlags)

	config := DefaultStateManagerConfig()
	config.HealthCheckInterval = 50 * time.Millisecond // Very fast for testing

	psm := NewPipelineStateManager(ctx, mockPipeline, logger, config)

	// Set up callback to detect state changes
	var callbackCalled bool
	var detectedOldState, detectedNewState PipelineState

	psm.SetStateChangeCallback(func(old, new PipelineState, transition StateTransition) {
		callbackCalled = true
		detectedOldState = old
		detectedNewState = new
	})

	err := psm.Start()
	if err != nil {
		t.Fatalf("Failed to start pipeline state manager: %v", err)
	}
	defer psm.Stop()

	// Simulate external state change (bypass state manager)
	mockPipeline.currentState = StatePlaying

	// Wait for consistency check to detect the change
	time.Sleep(200 * time.Millisecond)

	// Verify that the state manager detected the change
	currentState := psm.GetCurrentState()
	if currentState != StatePlaying {
		t.Errorf("Expected state manager to detect external state change to PLAYING, got %v", currentState)
	}

	// Verify callback was called
	if !callbackCalled {
		t.Error("Expected state change callback to be called")
	}

	if detectedOldState != StateNull || detectedNewState != StatePlaying {
		t.Errorf("Expected callback to detect NULL -> PLAYING transition, got %v -> %v",
			detectedOldState, detectedNewState)
	}
}

func TestPipelineStateManager_RestartPipeline(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockPipeline := &mockPipeline{currentState: StatePlaying}
	logger := log.New(log.Writer(), "[TEST] ", log.LstdFlags)

	psm := NewPipelineStateManager(ctx, mockPipeline, logger, nil)

	err := psm.Start()
	if err != nil {
		t.Fatalf("Failed to start pipeline state manager: %v", err)
	}
	defer psm.Stop()

	// Restart pipeline
	err = psm.RestartPipeline()
	if err != nil {
		t.Fatalf("Failed to restart pipeline: %v", err)
	}

	// Verify final state is PLAYING
	currentState := psm.GetCurrentState()
	if currentState != StatePlaying {
		t.Errorf("Expected state to be PLAYING after restart, got %v", currentState)
	}

	// Check history for restart sequence (NULL -> PLAYING)
	history := psm.GetStateHistory()
	if len(history) < 2 {
		t.Error("Expected at least 2 transitions for restart (stop + start)")
	}
}

func TestPipelineStateManager_Stats(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockPipeline := &mockPipeline{currentState: StateNull}
	logger := log.New(log.Writer(), "[TEST] ", log.LstdFlags)

	psm := NewPipelineStateManager(ctx, mockPipeline, logger, nil)

	err := psm.Start()
	if err != nil {
		t.Fatalf("Failed to start pipeline state manager: %v", err)
	}
	defer psm.Stop()

	// Perform some state transitions
	psm.SetState(StatePlaying)
	psm.SetState(StatePaused)
	psm.SetState(StateNull)

	// Get stats
	stats := psm.GetStats()

	// Verify stats structure
	if stats["current_state"] != StateNull {
		t.Errorf("Expected current_state to be NULL in stats")
	}

	if stats["successful_transitions"] == nil {
		t.Error("Expected successful_transitions in stats")
	}

	if stats["failed_transitions"] == nil {
		t.Error("Expected failed_transitions in stats")
	}

	if stats["state_history_size"] == nil {
		t.Error("Expected state_history_size in stats")
	}
}
