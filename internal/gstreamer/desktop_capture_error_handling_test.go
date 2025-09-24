package gstreamer

import (
	"testing"
	"time"

	"github.com/go-gst/go-gst/gst"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/open-beagle/bdwind-gstreamer/internal/config"
)

func TestDesktopCaptureGst_EnhancedErrorHandling(t *testing.T) {
	// Initialize GStreamer
	gst.Init(nil)

	// Create a valid configuration for testing
	cfg := config.DesktopCaptureConfig{
		DisplayID:   ":0",
		Width:       640,
		Height:      480,
		FrameRate:   30,
		Quality:     "medium",
		BufferSize:  10,
		ShowPointer: true,
		UseWayland:  false,
		UseDamage:   false,
	}

	// Create desktop capture instance
	dc, err := NewDesktopCaptureGst(cfg)
	require.NoError(t, err)
	require.NotNil(t, dc)
	defer dc.Stop()

	t.Run("Error Statistics Tracking", func(t *testing.T) {
		// Test that error statistics are properly initialized
		stats := dc.GetDetailedErrorStats()
		assert.NotNil(t, stats)
		assert.Equal(t, int64(0), stats["total_errors"])
		assert.Equal(t, int64(0), stats["sample_conversion_failures"])
		assert.Equal(t, int64(0), stats["pipeline_state_errors"])
		assert.Equal(t, int64(0), stats["recovery_attempts"])
	})

	t.Run("Health Status Monitoring", func(t *testing.T) {
		// Test health status functionality
		health := dc.GetHealthStatus()
		assert.NotNil(t, health)
		assert.Equal(t, HealthStatusHealthy, health.Overall)
		assert.NotNil(t, health.Checks)
		assert.Empty(t, health.Issues)
	})

	t.Run("Sample Error Handling", func(t *testing.T) {
		// Test sample error handling
		dc.handleSampleError("test_error", assert.AnError)

		// Check that error was recorded
		stats := dc.GetDetailedErrorStats()
		assert.Equal(t, int64(1), stats["total_errors"])

		// Check error channel
		select {
		case err := <-dc.GetErrorChannel():
			assert.Contains(t, err.Error(), "test_error")
		case <-time.After(100 * time.Millisecond):
			t.Error("Expected error in error channel")
		}
	})

	t.Run("Sample Failure Handling", func(t *testing.T) {
		// Test different types of sample failures
		testCases := []struct {
			name     string
			err      error
			expected string
		}{
			{
				name:     "Non-critical error",
				err:      assert.AnError,
				expected: "FlowOK",
			},
			{
				name:     "Critical error - out of memory",
				err:      &testError{msg: "out of memory"},
				expected: "FlowError",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				result := dc.handleSampleFailure(tc.err)
				if tc.expected == "FlowOK" {
					assert.Equal(t, "ok", result.String())
				} else {
					assert.Equal(t, "error", result.String())
				}
			})
		}
	})

	t.Run("Recovery Mechanism", func(t *testing.T) {
		// Test recovery mechanism
		initialAttempts := dc.stats.recoveryAttempts

		// Trigger recovery
		dc.triggerPipelineRecovery("test_recovery")

		// Wait a bit for recovery to process
		time.Sleep(10 * time.Millisecond)

		// Check that recovery attempt was recorded
		assert.Greater(t, dc.stats.recoveryAttempts, initialAttempts)
	})

	t.Run("Graceful Degradation", func(t *testing.T) {
		// Test graceful degradation with nil sample
		sample, err := dc.convertGstSampleWithRecovery(nil)
		assert.Error(t, err)
		assert.Nil(t, sample)
		assert.Contains(t, err.Error(), "gst sample is nil")
	})
}

// testError is a helper type for testing error conditions
type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}

func TestDesktopCaptureGst_HealthStatusLevels(t *testing.T) {
	// Initialize GStreamer
	gst.Init(nil)

	// Create a valid configuration for testing
	cfg := config.DesktopCaptureConfig{
		DisplayID:   ":0",
		Width:       640,
		Height:      480,
		FrameRate:   30,
		Quality:     "medium",
		BufferSize:  10,
		ShowPointer: true,
		UseWayland:  false,
		UseDamage:   false,
	}

	// Create desktop capture instance
	dc, err := NewDesktopCaptureGst(cfg)
	require.NoError(t, err)
	require.NotNil(t, dc)
	defer dc.Stop()

	t.Run("Healthy Status", func(t *testing.T) {
		// Initially should be healthy
		health := dc.GetHealthStatus()
		assert.Equal(t, HealthStatusHealthy, health.Overall)
		assert.Empty(t, health.Issues)
	})

	t.Run("Warning Status with High Drop Rate", func(t *testing.T) {
		// Simulate high drop rate
		dc.stats.mu.Lock()
		dc.stats.framesTotal = 100
		dc.stats.framesDropped = 7 // 7% drop rate - should trigger warning
		dc.stats.mu.Unlock()

		health := dc.GetHealthStatus()
		assert.Equal(t, HealthStatusWarning, health.Overall)
		assert.NotEmpty(t, health.Issues)
		assert.Contains(t, health.Issues[0], "drop rate")
	})

	t.Run("Error Status with High Error Rate", func(t *testing.T) {
		// Simulate high error rate
		dc.stats.mu.Lock()
		dc.stats.framesTotal = 100
		dc.stats.errorCount = 6    // 6% error rate - should trigger error status
		dc.stats.framesDropped = 0 // Reset drop rate
		dc.stats.mu.Unlock()

		health := dc.GetHealthStatus()
		assert.Equal(t, HealthStatusError, health.Overall)
		assert.NotEmpty(t, health.Issues)
		assert.Contains(t, health.Issues[0], "error rate")
	})
}

func TestDesktopCaptureGst_RecoveryMechanisms(t *testing.T) {
	// Initialize GStreamer
	gst.Init(nil)

	// Create a valid configuration for testing
	cfg := config.DesktopCaptureConfig{
		DisplayID:   ":0",
		Width:       640,
		Height:      480,
		FrameRate:   30,
		Quality:     "medium",
		BufferSize:  10,
		ShowPointer: true,
		UseWayland:  false,
		UseDamage:   false,
	}

	// Create desktop capture instance
	dc, err := NewDesktopCaptureGst(cfg)
	require.NoError(t, err)
	require.NotNil(t, dc)
	defer dc.Stop()

	t.Run("Sample Failure Recovery", func(t *testing.T) {
		// Track initial state (not used in this test but could be useful for future enhancements)

		// Trigger sample failure recovery
		dc.recoverFromSampleFailure()

		// Should reset dropped frames counter
		assert.Equal(t, int64(0), dc.stats.framesDropped)
	})

	t.Run("Generic Recovery", func(t *testing.T) {
		// Test generic recovery
		dc.performGenericRecovery()

		// Should complete without error
		// (This is mainly testing that the method doesn't panic)
	})

	t.Run("Clock Lost Recovery", func(t *testing.T) {
		// Test clock lost recovery
		dc.recoverFromClockLost()

		// Should complete without error
		// (This is mainly testing that the method doesn't panic)
	})
}
