package gstreamer

import (
	"context"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPipelineStateManagerCreation tests the creation of pipeline state manager without GStreamer dependencies
func TestPipelineStateManagerCreation(t *testing.T) {
	// Create logger
	logger := logrus.WithField("test", "pipeline-state-manager")

	// Create state manager with nil pipeline (should handle gracefully)
	ctx := context.Background()
	config := DefaultPipelineStateManagerConfig()

	psm := NewPipelineStateManager(ctx, nil, logger, config)

	assert.NotNil(t, psm)
	assert.Equal(t, PipelineStateNull, psm.GetCurrentState())
	assert.Equal(t, PipelineStateNull, psm.GetTargetState())
	assert.NotNil(t, psm.config)
	assert.NotNil(t, psm.logger)
	assert.NotNil(t, psm.metrics)
	assert.NotNil(t, psm.healthChecker)
}

// TestPipelineStateManagerConfiguration tests the configuration
func TestPipelineStateManagerConfiguration(t *testing.T) {
	config := DefaultPipelineStateManagerConfig()

	assert.Equal(t, 10*time.Second, config.StateTransitionTimeout)
	assert.Equal(t, 30*time.Second, config.HealthCheckInterval)
	assert.Equal(t, 100, config.MaxStateHistory)
	assert.Equal(t, 50, config.MaxErrorHistory)
	assert.True(t, config.EnableAutoRecovery)
	assert.Equal(t, 3, config.MaxRecoveryAttempts)
	assert.Equal(t, 2*time.Second, config.RecoveryDelay)
	assert.True(t, config.EnableDetailedLogging)
	assert.True(t, config.LogStateTransitions)
	assert.False(t, config.LogHealthChecks)
}

// TestPipelineStateManagerLifecycle tests the start/stop lifecycle
func TestPipelineStateManagerLifecycle(t *testing.T) {
	logger := logrus.WithField("test", "pipeline-state-manager")
	ctx := context.Background()

	psm := NewPipelineStateManager(ctx, nil, logger, nil)

	// Start state manager
	err := psm.Start()
	require.NoError(t, err)

	// Stop state manager
	err = psm.Stop()
	require.NoError(t, err)
}

// TestPipelineStateManagerCallbacks tests callback functionality
func TestPipelineStateManagerCallbacks(t *testing.T) {
	logger := logrus.WithField("test", "pipeline-state-manager")
	ctx := context.Background()

	psm := NewPipelineStateManager(ctx, nil, logger, nil)

	// Test state change callback
	var stateCallbackCalled bool
	var lastTransition ExtendedStateTransition

	psm.SetStateChangeCallback(func(oldState, newState PipelineState, transition ExtendedStateTransition) {
		stateCallbackCalled = true
		lastTransition = transition
	})

	// Use the variables to avoid unused variable errors
	_ = stateCallbackCalled
	_ = lastTransition

	// Test error callback
	var errorCallbackCalled bool
	var lastErrorType ErrorType
	var lastErrorMessage string

	psm.SetErrorCallback(func(errorType ErrorType, message string, err error) {
		errorCallbackCalled = true
		lastErrorType = errorType
		lastErrorMessage = message
	})

	// Simulate error handling
	psm.handleError(ErrorStateChange, "Test error", "Test details", "TestComponent")

	assert.True(t, errorCallbackCalled)
	assert.Equal(t, ErrorStateChange, lastErrorType)
	assert.Equal(t, "Test error", lastErrorMessage)
}

// TestPipelineStateManagerMetrics tests metrics functionality
func TestPipelineStateManagerMetrics(t *testing.T) {
	logger := logrus.WithField("test", "pipeline-state-manager")
	ctx := context.Background()

	psm := NewPipelineStateManager(ctx, nil, logger, nil)

	// Get initial metrics
	metrics := psm.GetMetrics()
	assert.Equal(t, int64(0), metrics.TotalStateTransitions)
	assert.Equal(t, int64(0), metrics.SuccessfulTransitions)
	assert.Equal(t, int64(0), metrics.FailedTransitions)
	assert.Greater(t, metrics.UptimeSeconds, 0.0)
}

// TestPipelineStateManagerErrorHandling tests error handling
func TestPipelineStateManagerErrorHandling(t *testing.T) {
	logger := logrus.WithField("test", "pipeline-state-manager")
	ctx := context.Background()

	psm := NewPipelineStateManager(ctx, nil, logger, nil)

	// Test error handling
	psm.handleError(ErrorStateChange, "Test error", "Test details", "TestComponent")

	// Check error history
	errorHistory := psm.GetErrorHistory()
	assert.Len(t, errorHistory, 1)
	assert.Equal(t, ErrorStateChange, errorHistory[0].Type)
	assert.Contains(t, errorHistory[0].Message, "Test error")
	assert.Equal(t, "TestComponent", errorHistory[0].Component)
}

// TestPipelineStateManagerStateTransitions tests state transition without actual pipeline
func TestPipelineStateManagerStateTransitions(t *testing.T) {
	logger := logrus.WithField("test", "pipeline-state-manager")
	ctx := context.Background()

	psm := NewPipelineStateManager(ctx, nil, logger, nil)

	// Try to set state on nil pipeline (should return error)
	err := psm.SetState(PipelineStateReady)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no pipeline set")

	// Current state should remain unchanged
	assert.Equal(t, PipelineStateNull, psm.GetCurrentState())
}

// TestPipelineHealthCheckerCreation tests health checker creation
func TestPipelineHealthCheckerCreation(t *testing.T) {
	logger := logrus.WithField("test", "health-checker")

	// Create health checker
	phc := NewPipelineHealthChecker(logger, nil)

	assert.NotNil(t, phc)
	assert.NotEmpty(t, phc.GetCheckerNames())
	assert.NotNil(t, phc.config)
	assert.NotNil(t, phc.logger)
	assert.NotNil(t, phc.metrics)
}

// TestPipelineHealthCheckerConfiguration tests health checker configuration
func TestPipelineHealthCheckerConfiguration(t *testing.T) {
	config := DefaultHealthCheckerConfig()

	assert.Equal(t, 30*time.Second, config.CheckTimeout)
	assert.Equal(t, 5*time.Second, config.StateCheckTimeout)
	assert.Equal(t, 10*time.Second, config.ElementCheckTimeout)
	assert.Equal(t, 5*time.Second, config.MemoryCheckTimeout)
	assert.Equal(t, 15*time.Second, config.PerformanceCheckTimeout)
	assert.Equal(t, int64(512), config.MemoryThresholdMB)
	assert.Equal(t, 80.0, config.CPUThresholdPercent)
	assert.Equal(t, int64(100), config.LatencyThresholdMs)
	assert.Equal(t, 0.05, config.ErrorRateThreshold)
	assert.True(t, config.EnableMemoryChecks)
	assert.True(t, config.EnablePerformanceChecks)
	assert.True(t, config.EnableElementChecks)
	assert.True(t, config.EnableStateChecks)
}

// TestPipelineErrorHandlerCreation tests error handler creation
func TestPipelineErrorHandlerCreation(t *testing.T) {
	logger := logrus.WithField("test", "error-handler")
	config := DefaultPipelineErrorHandlerConfig()

	// Create error handler
	peh := NewPipelineErrorHandler(logger, config)

	assert.NotNil(t, peh)
	assert.NotNil(t, peh.config)
	assert.NotNil(t, peh.logger)
	assert.NotNil(t, peh.metrics)
	assert.NotNil(t, peh.recoveryStrategies)
	assert.NotEmpty(t, peh.recoveryStrategies)
}

// TestPipelineErrorHandlerConfiguration tests error handler configuration
func TestPipelineErrorHandlerConfiguration(t *testing.T) {
	config := DefaultPipelineErrorHandlerConfig()

	assert.Equal(t, 3, config.MaxRetryAttempts)
	assert.Equal(t, 2*time.Second, config.RetryDelay)
	assert.Equal(t, 30*time.Second, config.MaxRetryDelay)
	assert.True(t, config.AutoRecovery)
	assert.Equal(t, 100, config.MaxErrorHistorySize)
	assert.Equal(t, 5*time.Minute, config.EscalationTimeout)
	assert.Equal(t, 5, config.ComponentRestartThreshold)
	assert.Equal(t, 10*time.Minute, config.RestartThresholdWindow)
	assert.True(t, config.EnableDetailedLogging)
	assert.True(t, config.EnableNotifications)
}

// TestPipelineErrorHandlerErrorHandling tests error handling functionality
func TestPipelineErrorHandlerErrorHandling(t *testing.T) {
	logger := logrus.WithField("test", "error-handler")
	config := DefaultPipelineErrorHandlerConfig()

	peh := NewPipelineErrorHandler(logger, config)

	// Start error handler
	ctx := context.Background()
	err := peh.Start(ctx)
	require.NoError(t, err)
	defer peh.Stop()

	// Get initial metrics
	metrics := peh.GetMetrics()
	assert.Equal(t, int64(0), metrics.TotalErrors)

	// Handle some errors
	peh.HandleError(ErrorStateChange, "Error 1", "Details 1", "Component1")
	peh.HandleError(ErrorElementFailure, "Error 2", "Details 2", "Component2")

	// Check updated metrics
	metrics = peh.GetMetrics()
	assert.Equal(t, int64(2), metrics.TotalErrors)
	assert.Equal(t, int64(1), metrics.ErrorsByType[ErrorStateChange])
	assert.Equal(t, int64(1), metrics.ErrorsByType[ErrorElementFailure])
	assert.Equal(t, int64(1), metrics.ErrorsByComponent["Component1"])
	assert.Equal(t, int64(1), metrics.ErrorsByComponent["Component2"])

	// Check error history
	history := peh.GetErrorHistory()
	assert.Len(t, history, 2)
}

// TestPipelineStateIntegrationCreation tests the integration creation
func TestPipelineStateIntegrationCreation(t *testing.T) {
	logger := logrus.WithField("test", "integration")
	ctx := context.Background()
	config := DefaultPipelineStateIntegrationConfig()

	psi := NewPipelineStateIntegration(ctx, nil, logger, config)

	assert.NotNil(t, psi)
	assert.NotNil(t, psi.config)
	assert.NotNil(t, psi.logger)
	assert.NotNil(t, psi.stateManager)
	assert.NotNil(t, psi.healthChecker)
	assert.NotNil(t, psi.errorHandler)
	assert.NotNil(t, psi.aggregatedMetrics)
}

// TestPipelineStateIntegrationLifecycle tests integration lifecycle
func TestPipelineStateIntegrationLifecycle(t *testing.T) {
	logger := logrus.WithField("test", "integration")
	ctx := context.Background()
	config := DefaultPipelineStateIntegrationConfig()

	psi := NewPipelineStateIntegration(ctx, nil, logger, config)

	// Test lifecycle
	assert.False(t, psi.IsRunning())

	err := psi.Start()
	require.NoError(t, err)
	assert.True(t, psi.IsRunning())

	err = psi.Stop()
	require.NoError(t, err)
	assert.False(t, psi.IsRunning())
}

// TestPipelineStateIntegrationMetrics tests aggregated metrics
func TestPipelineStateIntegrationMetrics(t *testing.T) {
	logger := logrus.WithField("test", "integration")
	ctx := context.Background()
	config := DefaultPipelineStateIntegrationConfig()

	psi := NewPipelineStateIntegration(ctx, nil, logger, config)

	// Get aggregated metrics
	metrics := psi.GetAggregatedMetrics()
	assert.NotNil(t, metrics)
	assert.Equal(t, int64(0), metrics.TotalOperations)
	assert.Equal(t, int64(0), metrics.SuccessfulOps)
	assert.Equal(t, int64(0), metrics.FailedOps)
	assert.Greater(t, metrics.IntegrationUptime.Seconds(), 0.0)

	// Get comprehensive stats
	stats := psi.GetComprehensiveStats()
	assert.NotNil(t, stats)
	assert.Contains(t, stats, "integration")
	assert.Contains(t, stats, "state_manager")
	assert.Contains(t, stats, "health_checker")
	assert.Contains(t, stats, "error_handler")
	assert.Contains(t, stats, "aggregated_metrics")
}
