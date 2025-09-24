package gstreamer

import (
	"context"
	"testing"
	"time"

	"github.com/go-gst/go-gst/gst"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPipelineStateManager_Creation(t *testing.T) {
	// Initialize GStreamer for testing
	gst.Init(nil)

	// Create a test pipeline
	pipeline, err := gst.NewPipeline("test-pipeline")
	require.NoError(t, err)
	defer pipeline.Unref()

	// Create logger
	logger := logrus.WithField("test", "pipeline-state-manager")

	// Create state manager
	ctx := context.Background()
	psm := NewPipelineStateManager(ctx, pipeline, logger, nil)

	assert.NotNil(t, psm)
	assert.Equal(t, PipelineStateNull, psm.GetCurrentState())
	assert.Equal(t, PipelineStateNull, psm.GetTargetState())
}

func TestPipelineStateManager_StateTransitions(t *testing.T) {
	// Initialize GStreamer for testing
	gst.Init(nil)

	// Create a simple test pipeline with videotestsrc and fakesink
	pipeline, err := gst.NewPipeline("test-pipeline")
	require.NoError(t, err)
	defer pipeline.Unref()

	// Add elements to make the pipeline functional
	src, err := gst.NewElement("videotestsrc")
	require.NoError(t, err)
	defer src.Unref()

	sink, err := gst.NewElement("fakesink")
	require.NoError(t, err)
	defer sink.Unref()

	// Add elements to pipeline
	err = pipeline.Add(src)
	require.NoError(t, err)
	err = pipeline.Add(sink)
	require.NoError(t, err)

	// Link elements
	err = src.Link(sink)
	require.NoError(t, err)

	// Create logger
	logger := logrus.WithField("test", "pipeline-state-manager")

	// Create state manager
	ctx := context.Background()
	config := DefaultPipelineStateManagerConfig()
	config.StateTransitionTimeout = 5 * time.Second

	psm := NewPipelineStateManager(ctx, pipeline, logger, config)

	// Start state manager
	err = psm.Start()
	require.NoError(t, err)
	defer psm.Stop()

	// Test state transition to READY
	err = psm.SetState(PipelineStateReady)
	assert.NoError(t, err)
	assert.Equal(t, PipelineStateReady, psm.GetCurrentState())

	// Test state transition to PLAYING
	err = psm.SetState(PipelineStatePlaying)
	assert.NoError(t, err)
	assert.Equal(t, PipelineStatePlaying, psm.GetCurrentState())

	// Test state transition back to NULL
	err = psm.SetState(PipelineStateNull)
	assert.NoError(t, err)
	assert.Equal(t, PipelineStateNull, psm.GetCurrentState())

	// Check state history
	history := psm.GetStateHistory()
	assert.GreaterOrEqual(t, len(history), 3) // At least 3 transitions
}

func TestPipelineStateManager_StateCallbacks(t *testing.T) {
	// Initialize GStreamer for testing
	gst.Init(nil)

	// Create a test pipeline
	pipeline, err := gst.NewPipeline("test-pipeline")
	require.NoError(t, err)
	defer pipeline.Unref()

	// Create logger
	logger := logrus.WithField("test", "pipeline-state-manager")

	// Create state manager
	ctx := context.Background()
	psm := NewPipelineStateManager(ctx, pipeline, logger, nil)

	// Set up callback tracking
	var callbackCalled bool
	var lastTransition StateTransition

	psm.SetStateChangeCallback(func(oldState, newState PipelineState, transition ExtendedStateTransition) {
		callbackCalled = true
		lastTransition = transition.StateTransition
	})

	// Start state manager
	err = psm.Start()
	require.NoError(t, err)
	defer psm.Stop()

	// Trigger state change
	err = psm.SetState(PipelineStateReady)

	// Verify callback was called
	assert.True(t, callbackCalled)
	assert.Equal(t, PipelineStateNull, lastTransition.From)
	assert.Equal(t, PipelineStateReady, lastTransition.To)
}

func TestPipelineStateManager_ErrorHandling(t *testing.T) {
	// Initialize GStreamer for testing
	gst.Init(nil)

	// Create logger
	logger := logrus.WithField("test", "pipeline-state-manager")

	// Create state manager with nil pipeline (should handle gracefully)
	ctx := context.Background()
	psm := NewPipelineStateManager(ctx, nil, logger, nil)

	// Start state manager
	err := psm.Start()
	require.NoError(t, err)
	defer psm.Stop()

	// Try to set state on nil pipeline (should return error)
	err = psm.SetState(PipelineStateReady)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no pipeline set")
}

func TestPipelineStateManager_Metrics(t *testing.T) {
	// Initialize GStreamer for testing
	gst.Init(nil)

	// Create a test pipeline
	pipeline, err := gst.NewPipeline("test-pipeline")
	require.NoError(t, err)
	defer pipeline.Unref()

	// Create logger
	logger := logrus.WithField("test", "pipeline-state-manager")

	// Create state manager
	ctx := context.Background()
	psm := NewPipelineStateManager(ctx, pipeline, logger, nil)

	// Start state manager
	err = psm.Start()
	require.NoError(t, err)
	defer psm.Stop()

	// Get initial metrics
	metrics := psm.GetMetrics()
	assert.Equal(t, int64(0), metrics.TotalStateTransitions)

	// Perform some state transitions
	psm.SetState(PipelineStateReady)
	psm.SetState(PipelineStatePlaying)
	psm.SetState(PipelineStateNull)

	// Check updated metrics
	metrics = psm.GetMetrics()
	assert.GreaterOrEqual(t, metrics.TotalStateTransitions, int64(3))
	assert.Greater(t, metrics.UptimeSeconds, 0.0)
}

func TestPipelineStateManager_RestartPipeline(t *testing.T) {
	// Initialize GStreamer for testing
	gst.Init(nil)

	// Create a simple test pipeline
	pipeline, err := gst.NewPipeline("test-pipeline")
	require.NoError(t, err)
	defer pipeline.Unref()

	// Add a simple element to make pipeline functional
	src, err := gst.NewElement("videotestsrc")
	require.NoError(t, err)
	defer src.Unref()

	sink, err := gst.NewElement("fakesink")
	require.NoError(t, err)
	defer sink.Unref()

	err = pipeline.Add(src)
	require.NoError(t, err)
	err = pipeline.Add(sink)
	require.NoError(t, err)

	err = src.Link(sink)
	require.NoError(t, err)

	// Create logger
	logger := logrus.WithField("test", "pipeline-state-manager")

	// Create state manager
	ctx := context.Background()
	config := DefaultPipelineStateManagerConfig()
	config.StateTransitionTimeout = 5 * time.Second

	psm := NewPipelineStateManager(ctx, pipeline, logger, config)

	// Start state manager
	err = psm.Start()
	require.NoError(t, err)
	defer psm.Stop()

	// Start pipeline
	err = psm.SetState(PipelineStatePlaying)
	require.NoError(t, err)

	// Restart pipeline
	err = psm.RestartPipeline()
	assert.NoError(t, err)

	// Verify pipeline is playing again
	assert.Equal(t, PipelineStatePlaying, psm.GetCurrentState())
}

func TestPipelineHealthChecker_Creation(t *testing.T) {
	logger := logrus.WithField("test", "health-checker")

	// Create health checker
	phc := NewPipelineHealthChecker(logger, nil)

	assert.NotNil(t, phc)
	assert.NotEmpty(t, phc.GetCheckerNames())
}

func TestPipelineHealthChecker_HealthCheck(t *testing.T) {
	// Initialize GStreamer for testing
	gst.Init(nil)

	// Create a test pipeline
	pipeline, err := gst.NewPipeline("test-pipeline")
	require.NoError(t, err)
	defer pipeline.Unref()

	logger := logrus.WithField("test", "health-checker")

	// Create health checker
	phc := NewPipelineHealthChecker(logger, nil)

	// Start health checker
	ctx := context.Background()
	err = phc.Start(ctx)
	require.NoError(t, err)
	defer phc.Stop()

	// Perform health check
	status := phc.CheckHealth()

	assert.NotEqual(t, HealthStatusHealthy, status.Overall) // Should be unhealthy due to null pipeline state
	assert.Greater(t, status.CheckCount, 0)
	assert.NotEmpty(t, status.Checks)
}

func TestPipelineErrorHandler_Creation(t *testing.T) {
	logger := logrus.WithField("test", "error-handler")

	// Create error handler
	peh := NewPipelineErrorHandler(logger, nil)

	assert.NotNil(t, peh)

	// Test error handling
	peh.HandleError(ErrorStateChange, "Test error", "Test details", "TestComponent")

	// Check error history
	history := peh.GetErrorHistory()
	assert.Len(t, history, 1)
	assert.Equal(t, ErrorStateChange, history[0].Type)
	assert.Equal(t, "Test error", history[0].Message)
}

func TestPipelineErrorHandler_Metrics(t *testing.T) {
	logger := logrus.WithField("test", "error-handler")

	// Create error handler
	peh := NewPipelineErrorHandler(logger, nil)

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
}
