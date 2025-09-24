package gstreamer

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGStreamerError(t *testing.T) {
	t.Run("Error interface implementation", func(t *testing.T) {
		err := &GStreamerError{
			Type:      ErrorTypePipelineCreation,
			Severity:  UnifiedSeverityError,
			Component: "test-component",
			Message:   "test error",
			Details:   "detailed error information",
		}

		assert.Contains(t, err.Error(), "PipelineCreation")
		assert.Contains(t, err.Error(), "Error")
		assert.Contains(t, err.Error(), "test-component")
		assert.Contains(t, err.Error(), "test error")
		assert.Contains(t, err.Error(), "detailed error information")
	})

	t.Run("Unwrap functionality", func(t *testing.T) {
		cause := errors.New("underlying error")
		err := &GStreamerError{
			Type:    ErrorTypeElementCreation,
			Message: "wrapper error",
			Cause:   cause,
		}

		assert.Equal(t, cause, err.Unwrap())
		assert.True(t, errors.Is(err, cause))
	})

	t.Run("IsRecoverable", func(t *testing.T) {
		recoverableErr := &GStreamerError{
			Recoverable: true,
			Suggested:   []RecoveryAction{RecoveryRetry},
		}
		assert.True(t, recoverableErr.IsRecoverable())

		nonRecoverableErr := &GStreamerError{
			Recoverable: false,
		}
		assert.False(t, nonRecoverableErr.IsRecoverable())

		noSuggestionsErr := &GStreamerError{
			Recoverable: true,
			Suggested:   []RecoveryAction{},
		}
		assert.False(t, noSuggestionsErr.IsRecoverable())
	})

	t.Run("IsCritical", func(t *testing.T) {
		criticalErr := &GStreamerError{Severity: UnifiedSeverityCritical}
		assert.True(t, criticalErr.IsCritical())

		fatalErr := &GStreamerError{Severity: UnifiedSeverityFatal}
		assert.True(t, fatalErr.IsCritical())

		warningErr := &GStreamerError{Severity: UnifiedSeverityWarning}
		assert.False(t, warningErr.IsCritical())
	})
}

func TestErrorHandler(t *testing.T) {
	logger := logrus.NewEntry(logrus.New())
	logger.Logger.SetLevel(logrus.DebugLevel)

	t.Run("NewErrorHandler with default config", func(t *testing.T) {
		eh := NewErrorHandler(logger, nil)
		assert.NotNil(t, eh)
		assert.NotNil(t, eh.config)
		assert.Equal(t, 1000, eh.config.MaxErrorHistory)
		assert.True(t, eh.config.AutoRecovery)
	})

	t.Run("NewErrorHandler with custom config", func(t *testing.T) {
		config := &UnifiedErrorHandlerConfig{
			MaxErrorHistory:     500,
			MaxRecoveryAttempts: 5,
			AutoRecovery:        false,
		}
		eh := NewErrorHandler(logger, config)
		assert.NotNil(t, eh)
		assert.Equal(t, config, eh.config)
	})

	t.Run("HandleError basic functionality", func(t *testing.T) {
		eh := NewErrorHandler(logger, nil)
		ctx := context.Background()

		err := eh.HandleError(ctx, ErrorTypeElementCreation, "test-component",
			"create-element", "failed to create element", errors.New("underlying error"))

		assert.NoError(t, err) // Should not propagate by default

		history := eh.GetErrorHistory()
		assert.Len(t, history, 1)

		gstErr := history[0]
		assert.Equal(t, ErrorTypeElementCreation, gstErr.Type)
		assert.Equal(t, "test-component", gstErr.Component)
		assert.Equal(t, "create-element", gstErr.Operation)
		assert.Equal(t, "failed to create element", gstErr.Message)
		assert.NotEmpty(t, gstErr.Code)
	})

	t.Run("Error propagation for critical errors", func(t *testing.T) {
		config := &UnifiedErrorHandlerConfig{
			PropagateErrors: true,
			AutoRecovery:    false,
		}
		eh := NewErrorHandler(logger, config)
		ctx := context.Background()

		err := eh.HandleError(ctx, ErrorTypeMemoryAllocation, "test-component",
			"allocate-buffer", "memory allocation failed", errors.New("out of memory"))

		assert.Error(t, err)
		assert.True(t, IsGStreamerError(err))

		gstErr, ok := GetGStreamerError(err)
		require.True(t, ok)
		assert.Equal(t, ErrorTypeMemoryAllocation, gstErr.Type)
		assert.True(t, gstErr.IsCritical())
	})

	t.Run("Component error tracking", func(t *testing.T) {
		eh := NewErrorHandler(logger, nil)
		ctx := context.Background()

		// Add multiple errors for the same component
		for i := 0; i < 3; i++ {
			eh.HandleError(ctx, ErrorTypeElementLinking, "test-component",
				"link-elements", "linking failed", nil)
		}

		componentErrors := eh.GetComponentErrors("test-component")
		assert.Len(t, componentErrors, 3)

		// Check if component should be restarted
		config := &UnifiedErrorHandlerConfig{
			ComponentRestartThreshold: 2,
		}
		eh.config = config

		assert.True(t, eh.shouldRestartComponent("test-component"))
	})

	t.Run("Metrics tracking", func(t *testing.T) {
		eh := NewErrorHandler(logger, nil)
		ctx := context.Background()

		// Add various errors
		eh.HandleError(ctx, ErrorTypeElementCreation, "comp1", "op1", "msg1", nil)
		eh.HandleError(ctx, ErrorTypeElementCreation, "comp2", "op2", "msg2", nil)
		eh.HandleError(ctx, ErrorTypePipelineState, "comp1", "op3", "msg3", nil)

		metrics := eh.GetMetrics()
		assert.Equal(t, int64(3), metrics.TotalErrors)
		assert.Equal(t, int64(2), metrics.ErrorsByType[ErrorTypeElementCreation])
		assert.Equal(t, int64(1), metrics.ErrorsByType[ErrorTypePipelineState])
		assert.Equal(t, int64(2), metrics.ErrorsByComponent["comp1"])
		assert.Equal(t, int64(1), metrics.ErrorsByComponent["comp2"])
	})
}

func TestErrorCreationFunctions(t *testing.T) {
	t.Run("NewInitializationError", func(t *testing.T) {
		cause := errors.New("init failed")
		err := NewInitializationError("gstreamer", "initialization failed", cause)

		assert.Equal(t, ErrorTypeInitialization, err.Type)
		assert.Equal(t, UnifiedSeverityCritical, err.Severity)
		assert.Equal(t, "gstreamer", err.Component)
		assert.Equal(t, "initialization failed", err.Message)
		assert.Equal(t, cause, err.Cause)
		assert.False(t, err.Recoverable)
	})

	t.Run("NewPipelineError", func(t *testing.T) {
		err := NewPipelineError(ErrorTypePipelineCreation, "pipeline", "create", "failed", nil)

		assert.Equal(t, ErrorTypePipelineCreation, err.Type)
		assert.Equal(t, UnifiedSeverityCritical, err.Severity)
		assert.Equal(t, "pipeline", err.Component)
		assert.Equal(t, "create", err.Operation)
		assert.True(t, err.Recoverable)
		assert.Contains(t, err.Suggested, RecoveryRetry)
	})

	t.Run("NewElementError", func(t *testing.T) {
		err := NewElementError(ErrorTypeElementCreation, "encoder", "x264enc", "creation failed", nil)

		assert.Equal(t, ErrorTypeElementCreation, err.Type)
		assert.Equal(t, "encoder", err.Component)
		assert.Equal(t, "x264enc", err.Metadata["element_name"])
		assert.Contains(t, err.Suggested, RecoveryFallback)
	})

	t.Run("NewMemoryError", func(t *testing.T) {
		err := NewMemoryError("buffer-pool", "allocate", 1024*1024, errors.New("oom"))

		assert.Equal(t, ErrorTypeMemoryAllocation, err.Type)
		assert.Equal(t, UnifiedSeverityCritical, err.Severity)
		assert.Equal(t, "buffer-pool", err.Component)
		assert.Equal(t, int64(1024*1024), err.Metadata["requested_size"])
		assert.Contains(t, err.Message, "1048576 bytes")
	})

	t.Run("NewTimeoutError", func(t *testing.T) {
		timeout := 5 * time.Second
		err := NewTimeoutError("pipeline", "state-change", timeout, nil)

		assert.Equal(t, ErrorTypeTimeout, err.Type)
		assert.Equal(t, UnifiedSeverityWarning, err.Severity)
		assert.Equal(t, timeout, err.Metadata["timeout_duration"])
		assert.Contains(t, err.Message, "5s")
	})

	t.Run("NewValidationError", func(t *testing.T) {
		err := NewValidationError("config", "width", "invalid width value", -1)

		assert.Equal(t, ErrorTypeValidation, err.Type)
		assert.Equal(t, UnifiedSeverityInfo, err.Severity)
		assert.Equal(t, "width", err.Metadata["field"])
		assert.Equal(t, -1, err.Metadata["value"])
	})
}

func TestErrorUtilities(t *testing.T) {
	t.Run("IsGStreamerError", func(t *testing.T) {
		gstErr := &GStreamerError{Type: ErrorTypeElementCreation}
		regularErr := errors.New("regular error")

		assert.True(t, IsGStreamerError(gstErr))
		assert.False(t, IsGStreamerError(regularErr))
	})

	t.Run("GetGStreamerError", func(t *testing.T) {
		gstErr := &GStreamerError{Type: ErrorTypeElementCreation}

		extracted, ok := GetGStreamerError(gstErr)
		assert.True(t, ok)
		assert.Equal(t, gstErr, extracted)

		_, ok = GetGStreamerError(errors.New("regular error"))
		assert.False(t, ok)
	})

	t.Run("IsErrorType", func(t *testing.T) {
		gstErr := &GStreamerError{Type: ErrorTypeElementCreation}

		assert.True(t, IsErrorType(gstErr, ErrorTypeElementCreation))
		assert.False(t, IsErrorType(gstErr, ErrorTypePipelineState))
		assert.False(t, IsErrorType(errors.New("regular"), ErrorTypeElementCreation))
	})

	t.Run("IsErrorSeverity", func(t *testing.T) {
		gstErr := &GStreamerError{Severity: UnifiedSeverityCritical}

		assert.True(t, IsErrorSeverity(gstErr, UnifiedSeverityCritical))
		assert.False(t, IsErrorSeverity(gstErr, UnifiedSeverityWarning))
	})

	t.Run("IsCriticalError", func(t *testing.T) {
		criticalErr := &GStreamerError{Severity: UnifiedSeverityCritical}
		warningErr := &GStreamerError{Severity: UnifiedSeverityWarning}

		assert.True(t, IsCriticalError(criticalErr))
		assert.False(t, IsCriticalError(warningErr))
		assert.False(t, IsCriticalError(errors.New("regular")))
	})

	t.Run("IsRecoverableError", func(t *testing.T) {
		recoverableErr := &GStreamerError{
			Recoverable: true,
			Suggested:   []RecoveryAction{RecoveryRetry},
		}
		nonRecoverableErr := &GStreamerError{Recoverable: false}

		assert.True(t, IsRecoverableError(recoverableErr))
		assert.False(t, IsRecoverableError(nonRecoverableErr))
		assert.False(t, IsRecoverableError(errors.New("regular")))
	})
}

func TestErrorAggregator(t *testing.T) {
	t.Run("Basic functionality", func(t *testing.T) {
		aggregator := NewErrorAggregator()
		assert.False(t, aggregator.HasErrors())

		err1 := &GStreamerError{Type: ErrorTypeElementCreation, Severity: UnifiedSeverityError}
		err2 := &GStreamerError{Type: ErrorTypePipelineState, Severity: UnifiedSeverityCritical}

		aggregator.Add(err1)
		aggregator.Add(err2)

		assert.True(t, aggregator.HasErrors())
		assert.True(t, aggregator.HasCriticalErrors())

		errors := aggregator.GetErrors()
		assert.Len(t, errors, 2)
	})

	t.Run("Summary generation", func(t *testing.T) {
		aggregator := NewErrorAggregator()

		aggregator.Add(&GStreamerError{
			Type:      ErrorTypeElementCreation,
			Severity:  UnifiedSeverityError,
			Component: "encoder",
		})
		aggregator.Add(&GStreamerError{
			Type:      ErrorTypeElementCreation,
			Severity:  UnifiedSeverityWarning,
			Component: "decoder",
		})
		aggregator.Add(&GStreamerError{
			Type:      ErrorTypePipelineState,
			Severity:  UnifiedSeverityCritical,
			Component: "encoder",
		})

		summary := aggregator.GetSummary()
		assert.Equal(t, 3, summary["total_errors"])

		byType := summary["by_type"].(map[string]int)
		assert.Equal(t, 2, byType["ElementCreation"])
		assert.Equal(t, 1, byType["PipelineState"])

		bySeverity := summary["by_severity"].(map[string]int)
		assert.Equal(t, 1, bySeverity["Error"])
		assert.Equal(t, 1, bySeverity["Warning"])
		assert.Equal(t, 1, bySeverity["Critical"])

		byComponent := summary["by_component"].(map[string]int)
		assert.Equal(t, 2, byComponent["encoder"])
		assert.Equal(t, 1, byComponent["decoder"])
	})

	t.Run("Clear functionality", func(t *testing.T) {
		aggregator := NewErrorAggregator()
		aggregator.Add(&GStreamerError{Type: ErrorTypeElementCreation})

		assert.True(t, aggregator.HasErrors())

		aggregator.Clear()
		assert.False(t, aggregator.HasErrors())
		assert.Len(t, aggregator.GetErrors(), 0)
	})
}

func TestGlobalErrorHandler(t *testing.T) {
	t.Run("Set and get global handler", func(t *testing.T) {
		logger := logrus.NewEntry(logrus.New())
		handler := NewErrorHandler(logger, nil)

		SetGlobalErrorHandler(handler)
		retrieved := GetGlobalErrorHandler()

		assert.Equal(t, handler, retrieved)
	})

	t.Run("HandleGlobalError with handler", func(t *testing.T) {
		logger := logrus.NewEntry(logrus.New())
		handler := NewErrorHandler(logger, nil)
		SetGlobalErrorHandler(handler)

		ctx := context.Background()
		err := HandleGlobalError(ctx, ErrorTypeElementCreation, "test", "op", "msg", nil)

		assert.NoError(t, err)

		history := handler.GetErrorHistory()
		assert.Len(t, history, 1)
	})

	t.Run("HandleGlobalError without handler", func(t *testing.T) {
		SetGlobalErrorHandler(nil)

		ctx := context.Background()
		err := HandleGlobalError(ctx, ErrorTypeElementCreation, "test", "op", "msg", nil)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "ElementCreation")
		assert.Contains(t, err.Error(), "test")
	})
}

func TestErrorRecoveryStrategies(t *testing.T) {
	t.Run("Default recovery strategies", func(t *testing.T) {
		logger := logrus.NewEntry(logrus.New())
		eh := NewErrorHandler(logger, nil)

		// Test that default strategies are set
		strategy, exists := eh.GetRecoveryStrategy(ErrorTypePipelineState)
		assert.True(t, exists)
		assert.True(t, strategy.AutoRecover)
		assert.Contains(t, strategy.Actions, RecoveryRetry)

		strategy, exists = eh.GetRecoveryStrategy(ErrorTypePermission)
		assert.True(t, exists)
		assert.False(t, strategy.AutoRecover)
		assert.Contains(t, strategy.Actions, RecoveryManualIntervention)
	})

	t.Run("Custom recovery strategy", func(t *testing.T) {
		logger := logrus.NewEntry(logrus.New())
		eh := NewErrorHandler(logger, nil)

		customStrategy := RecoveryStrategy{
			MaxRetries:  5,
			RetryDelay:  2 * time.Second,
			Actions:     []RecoveryAction{RecoveryFallback},
			AutoRecover: true,
		}

		eh.SetRecoveryStrategy(ErrorTypeElementCreation, customStrategy)

		retrieved, exists := eh.GetRecoveryStrategy(ErrorTypeElementCreation)
		assert.True(t, exists)
		assert.Equal(t, customStrategy.MaxRetries, retrieved.MaxRetries)
		assert.Equal(t, customStrategy.RetryDelay, retrieved.RetryDelay)
		assert.Equal(t, customStrategy.Actions, retrieved.Actions)
	})
}

func TestErrorHandlerConfiguration(t *testing.T) {
	t.Run("Error history limit", func(t *testing.T) {
		config := &UnifiedErrorHandlerConfig{
			MaxErrorHistory: 2,
			AutoRecovery:    false,
		}
		logger := logrus.NewEntry(logrus.New())
		eh := NewErrorHandler(logger, config)
		ctx := context.Background()

		// Add more errors than the limit
		for i := 0; i < 5; i++ {
			eh.HandleError(ctx, ErrorTypeElementCreation, "test", "op", "msg", nil)
		}

		history := eh.GetErrorHistory()
		assert.Len(t, history, 2) // Should be limited to MaxErrorHistory
	})

	t.Run("Component restart threshold", func(t *testing.T) {
		config := &UnifiedErrorHandlerConfig{
			ComponentRestartThreshold: 2,
			AutoRecovery:              false,
		}
		logger := logrus.NewEntry(logrus.New())
		eh := NewErrorHandler(logger, config)
		ctx := context.Background()

		// Add errors below threshold
		eh.HandleError(ctx, ErrorTypeElementCreation, "test-component", "op", "msg", nil)
		assert.False(t, eh.shouldRestartComponent("test-component"))

		// Add error to reach threshold
		eh.HandleError(ctx, ErrorTypeElementCreation, "test-component", "op", "msg", nil)
		assert.True(t, eh.shouldRestartComponent("test-component"))
	})
}

// Benchmark tests
func BenchmarkErrorHandling(b *testing.B) {
	logger := logrus.NewEntry(logrus.New())
	logger.Logger.SetLevel(logrus.ErrorLevel) // Reduce logging overhead

	eh := NewErrorHandler(logger, &UnifiedErrorHandlerConfig{
		AutoRecovery: false, // Disable recovery for benchmarking
	})
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		eh.HandleError(ctx, ErrorTypeElementCreation, "benchmark", "test", "message", nil)
	}
}

func BenchmarkErrorCreation(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		NewElementError(ErrorTypeElementCreation, "component", "element", "message", nil)
	}
}
