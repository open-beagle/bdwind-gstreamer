package gstreamer

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewErrorManager(t *testing.T) {
	logger := logrus.NewEntry(logrus.New())

	// Test with default config
	em := NewErrorManager(nil, logger)
	assert.NotNil(t, em)
	assert.NotNil(t, em.config)
	assert.Equal(t, 100, em.config.MaxErrorHistory)
	assert.True(t, em.config.EnableAutoRecovery)
	assert.Len(t, em.strategies, 2) // Default strategies

	// Test with custom config
	config := &ErrorManagerConfig{
		MaxErrorHistory:    50,
		EnableAutoRecovery: false,
	}
	em2 := NewErrorManager(config, logger)
	assert.NotNil(t, em2)
	assert.Equal(t, 50, em2.config.MaxErrorHistory)
	assert.False(t, em2.config.EnableAutoRecovery)
}

func TestErrorManagerLifecycle(t *testing.T) {
	logger := logrus.NewEntry(logrus.New())
	em := NewErrorManager(nil, logger)

	// Test start
	err := em.Start()
	assert.NoError(t, err)
	assert.True(t, em.running)

	// Test start when already running
	err = em.Start()
	assert.Error(t, err)

	// Test stop
	err = em.Stop()
	assert.NoError(t, err)
	assert.False(t, em.running)

	// Test stop when not running
	err = em.Stop()
	assert.NoError(t, err)
}

func TestHandleError(t *testing.T) {
	logger := logrus.NewEntry(logrus.New())
	config := &ErrorManagerConfig{
		MaxErrorHistory:       10,
		EnableAutoRecovery:    false, // Disable for testing
		ErrorThrottleInterval: 100 * time.Millisecond,
	}
	em := NewErrorManager(config, logger)

	err := em.Start()
	require.NoError(t, err)
	defer em.Stop()

	// Handle an error
	em.HandleError(ErrorTypeConfiguration, "Test error", "Test details", "Test source")

	// Check statistics
	stats := em.GetStatistics()
	assert.Equal(t, int64(1), stats.TotalErrors)
	assert.Equal(t, int64(1), stats.ErrorsByType[ErrorTypeConfiguration])
	assert.Equal(t, int64(1), stats.ErrorsBySeverity[ErrorSeverityCritical])

	// Check history
	history := em.GetErrorHistory()
	assert.Len(t, history, 1)
	assert.Equal(t, ErrorTypeConfiguration, history[0].Type)
	assert.Equal(t, "Test error", history[0].Message)
	assert.Equal(t, "Test details", history[0].Details)
	assert.Equal(t, "Test source", history[0].Source)
}

func TestErrorThrottling(t *testing.T) {
	logger := logrus.NewEntry(logrus.New())
	config := &ErrorManagerConfig{
		MaxErrorHistory:       10,
		EnableAutoRecovery:    false,
		ErrorThrottleInterval: 200 * time.Millisecond,
	}
	em := NewErrorManager(config, logger)

	err := em.Start()
	require.NoError(t, err)
	defer em.Stop()

	// Handle same error type multiple times quickly
	em.HandleError(ErrorTypeNetwork, "Error 1", "", "Source")
	em.HandleError(ErrorTypeNetwork, "Error 2", "", "Source") // Should be throttled
	em.HandleError(ErrorTypeNetwork, "Error 3", "", "Source") // Should be throttled

	// Check that only first error was recorded
	stats := em.GetStatistics()
	assert.Equal(t, int64(1), stats.TotalErrors)

	// Wait for throttle interval to pass
	time.Sleep(250 * time.Millisecond)

	// Handle error again - should not be throttled
	em.HandleError(ErrorTypeNetwork, "Error 4", "", "Source")

	stats = em.GetStatistics()
	assert.Equal(t, int64(2), stats.TotalErrors)
}

func TestErrorClassification(t *testing.T) {
	logger := logrus.NewEntry(logrus.New())
	em := NewErrorManager(nil, logger)

	tests := []struct {
		errorType        ErrorType
		expectedSeverity ErrorSeverity
	}{
		{ErrorTypeConfiguration, ErrorSeverityCritical},
		{ErrorTypePermission, ErrorSeverityCritical},
		{ErrorTypeMemoryExhaustion, ErrorSeverityHigh},
		{ErrorTypeSystem, ErrorSeverityHigh},
		{ErrorTypeStateChange, ErrorSeverityMedium},
		{ErrorTypeElementFailure, ErrorSeverityMedium},
		{ErrorTypeNetwork, ErrorSeverityLow},
		{ErrorTypeTimeout, ErrorSeverityLow},
	}

	for _, test := range tests {
		severity := em.classifyErrorSeverity(test.errorType, "test message")
		assert.Equal(t, test.expectedSeverity, severity,
			"Error type %s should have severity %s", test.errorType, test.expectedSeverity)
	}
}

func TestRecoveryStrategies(t *testing.T) {
	// Test RestartRecoveryStrategy
	strategy := NewRestartRecoveryStrategy(5 * time.Second)
	assert.NotNil(t, strategy)
	assert.True(t, strategy.CanRecover(ErrorTypeElementFailure))
	assert.False(t, strategy.CanRecover(ErrorTypeConfiguration))
	assert.Equal(t, 5*time.Second, strategy.GetRecoveryTime())

	// Test recovery
	errorEvent := &ErrorEvent{
		Type: ErrorTypeElementFailure,
	}
	ctx := context.Background()
	err := strategy.Recover(ctx, errorEvent)
	assert.NoError(t, err)

	// Test ResetRecoveryStrategy
	resetStrategy := NewResetRecoveryStrategy(3 * time.Second)
	assert.NotNil(t, resetStrategy)
	assert.True(t, resetStrategy.CanRecover(ErrorTypeStateChange))
	assert.False(t, resetStrategy.CanRecover(ErrorTypeNetwork))
	assert.Equal(t, 3*time.Second, resetStrategy.GetRecoveryTime())
}

func TestErrorRecovery(t *testing.T) {
	logger := logrus.NewEntry(logrus.New())
	config := &ErrorManagerConfig{
		MaxErrorHistory:     10,
		EnableAutoRecovery:  true,
		MaxRecoveryAttempts: 2,
		RecoveryDelay:       50 * time.Millisecond,
	}
	em := NewErrorManager(config, logger)

	err := em.Start()
	require.NoError(t, err)
	defer em.Stop()

	// Handle a recoverable error
	em.HandleError(ErrorTypeElementFailure, "Recoverable error", "", "Test")

	// Wait for recovery attempt
	time.Sleep(200 * time.Millisecond)

	// Check statistics
	stats := em.GetStatistics()
	assert.Greater(t, stats.RecoveryAttempts, int64(0))
	assert.Greater(t, stats.SuccessfulRecoveries, int64(0))
}

func TestErrorHistory(t *testing.T) {
	logger := logrus.NewEntry(logrus.New())
	config := &ErrorManagerConfig{
		MaxErrorHistory:    3, // Small history for testing
		EnableAutoRecovery: false,
	}
	em := NewErrorManager(config, logger)

	err := em.Start()
	require.NoError(t, err)
	defer em.Stop()

	// Add more errors than history size
	for i := 0; i < 5; i++ {
		em.HandleError(ErrorTypeNetwork, fmt.Sprintf("Error %d", i), "", "Test")
	}

	// Check that history is limited
	history := em.GetErrorHistory()
	assert.Len(t, history, 3)

	// Check that we have the most recent errors
	assert.Equal(t, "Error 2", history[0].Message)
	assert.Equal(t, "Error 3", history[1].Message)
	assert.Equal(t, "Error 4", history[2].Message)

	// Test recent errors
	recent := em.GetRecentErrors(2)
	assert.Len(t, recent, 2)
	assert.Equal(t, "Error 3", recent[0].Message)
	assert.Equal(t, "Error 4", recent[1].Message)
}

func TestErrorCallback(t *testing.T) {
	logger := logrus.NewEntry(logrus.New())
	em := NewErrorManager(nil, logger)

	err := em.Start()
	require.NoError(t, err)
	defer em.Stop()

	// Set up callback
	callbackCalled := false
	var callbackError *ErrorEvent
	em.SetErrorCallback(func(errorEvent *ErrorEvent) {
		callbackCalled = true
		callbackError = errorEvent
	})

	// Handle an error
	em.HandleError(ErrorTypeNetwork, "Callback test", "Details", "Source")

	// Wait for callback
	time.Sleep(100 * time.Millisecond)

	// Check callback was called
	assert.True(t, callbackCalled)
	assert.NotNil(t, callbackError)
	assert.Equal(t, ErrorTypeNetwork, callbackError.Type)
	assert.Equal(t, "Callback test", callbackError.Message)
}

func TestCustomRecoveryStrategy(t *testing.T) {
	logger := logrus.NewEntry(logrus.New())
	em := NewErrorManager(nil, logger)

	// Create custom recovery strategy
	customStrategy := &testRecoveryStrategy{
		canRecoverTypes: []ErrorType{ErrorTypeCustom},
		recoveryTime:    1 * time.Second,
	}

	// Add custom strategy
	em.AddRecoveryStrategy(customStrategy)

	// Verify strategy was added
	found := false
	for _, strategy := range em.strategies {
		if strategy == customStrategy {
			found = true
			break
		}
	}
	assert.True(t, found)
}

func TestClearErrorHistory(t *testing.T) {
	logger := logrus.NewEntry(logrus.New())
	em := NewErrorManager(nil, logger)

	err := em.Start()
	require.NoError(t, err)
	defer em.Stop()

	// Add some errors
	em.HandleError(ErrorTypeNetwork, "Error 1", "", "Test")
	em.HandleError(ErrorTypeSystem, "Error 2", "", "Test")

	// Check history has errors
	history := em.GetErrorHistory()
	assert.Len(t, history, 2)

	// Clear history
	em.ClearErrorHistory()

	// Check history is empty
	history = em.GetErrorHistory()
	assert.Len(t, history, 0)
}

func TestGetStatus(t *testing.T) {
	logger := logrus.NewEntry(logrus.New())
	em := NewErrorManager(nil, logger)

	status := em.GetStatus()
	assert.NotNil(t, status)
	assert.Contains(t, status, "running")
	assert.Contains(t, status, "auto_recovery")
	assert.Contains(t, status, "recovery_strategies")
	assert.Contains(t, status, "statistics")

	assert.False(t, status["running"].(bool))
	assert.True(t, status["auto_recovery"].(bool))
	assert.Equal(t, 2, status["recovery_strategies"].(int)) // Default strategies
}

// Custom error type for testing
const ErrorTypeCustom ErrorType = 999

// Test recovery strategy implementation
type testRecoveryStrategy struct {
	canRecoverTypes []ErrorType
	recoveryTime    time.Duration
	recoveryCalled  bool
}

func (trs *testRecoveryStrategy) CanRecover(errorType ErrorType) bool {
	for _, t := range trs.canRecoverTypes {
		if t == errorType {
			return true
		}
	}
	return false
}

func (trs *testRecoveryStrategy) Recover(ctx context.Context, errorEvent *ErrorEvent) error {
	trs.recoveryCalled = true
	return nil
}

func (trs *testRecoveryStrategy) GetRecoveryTime() time.Duration {
	return trs.recoveryTime
}

func (trs *testRecoveryStrategy) GetDescription() string {
	return "Test recovery strategy"
}

func TestErrorTypeString(t *testing.T) {
	tests := []struct {
		errorType ErrorType
		expected  string
	}{
		{ErrorTypeConfiguration, "CONFIGURATION"},
		{ErrorTypeResource, "RESOURCE"},
		{ErrorTypeNetwork, "NETWORK"},
		{ErrorTypeEncoding, "ENCODING"},
		{ErrorTypeMemory, "MEMORY"},
		{ErrorTypeSystem, "SYSTEM"},
		{ErrorTypeStateChange, "STATE_CHANGE"},
		{ErrorTypeElementFailure, "ELEMENT_FAILURE"},
		{ErrorTypeUnknown, "UNKNOWN"},
	}

	for _, test := range tests {
		assert.Equal(t, test.expected, test.errorType.String())
	}
}

func TestErrorSeverityString(t *testing.T) {
	tests := []struct {
		severity ErrorSeverity
		expected string
	}{
		{ErrorSeverityLow, "LOW"},
		{ErrorSeverityMedium, "MEDIUM"},
		{ErrorSeverityHigh, "HIGH"},
		{ErrorSeverityCritical, "CRITICAL"},
	}

	for _, test := range tests {
		assert.Equal(t, test.expected, test.severity.String())
	}
}
