package gstreamer

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSafeDesktopCapture_Creation(t *testing.T) {
	capture, err := NewSafeDesktopCapture()
	if err != nil {
		// Skip test if we can't create the capture (e.g., no X11 display)
		t.Skipf("Skipping test due to creation error: %v", err)
		return
	}
	require.NotNil(t, capture)
	defer capture.Close()

	// Check that all objects are tracked
	assert.NotEqual(t, uintptr(0), capture.pipelineID)
	assert.NotEqual(t, uintptr(0), capture.sourceID)
	assert.NotEqual(t, uintptr(0), capture.sinkID)
	assert.NotEqual(t, uintptr(0), capture.busID)

	// Check health
	assert.True(t, capture.IsHealthy())

	// Check stats
	stats := capture.GetStats()
	assert.NotNil(t, stats)

	lifecycleStats := stats["lifecycle_stats"].(ObjectLifecycleStats)
	assert.Equal(t, int64(4), lifecycleStats.TotalRegistered) // pipeline, source, sink, bus
	assert.Equal(t, int64(4), lifecycleStats.CurrentActive)
}

func TestSafeDesktopCapture_StartStop(t *testing.T) {
	capture, err := NewSafeDesktopCapture()
	if err != nil {
		t.Skipf("Skipping test due to creation error: %v", err)
		return
	}
	require.NotNil(t, capture)
	defer capture.Close()

	// Test start
	err = capture.Start()
	if err != nil {
		// Skip if we can't start (e.g., no display available)
		t.Skipf("Skipping start test due to error: %v", err)
		return
	}

	// Wait a bit
	time.Sleep(100 * time.Millisecond)

	// Test stop
	err = capture.Stop()
	assert.NoError(t, err)
}

func TestSafeDesktopCapture_ObjectValidation(t *testing.T) {
	capture, err := NewSafeDesktopCapture()
	if err != nil {
		t.Skipf("Skipping test due to creation error: %v", err)
		return
	}
	require.NotNil(t, capture)
	defer capture.Close()

	// All objects should be valid initially
	assert.True(t, capture.objectTracker.IsObjectValid(capture.pipelineID))
	assert.True(t, capture.objectTracker.IsObjectValid(capture.sourceID))
	assert.True(t, capture.objectTracker.IsObjectValid(capture.sinkID))
	assert.True(t, capture.objectTracker.IsObjectValid(capture.busID))

	// Force invalidate one object
	err = capture.objectTracker.ForceInvalidateObject(capture.sourceID)
	require.NoError(t, err)

	// Health should be affected
	assert.False(t, capture.IsHealthy())

	// Recovery should detect the invalid object
	err = capture.RecoverFromError()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "source")
}

func TestSafeDesktopCapture_ProcessSample(t *testing.T) {
	capture, err := NewSafeDesktopCapture()
	if err != nil {
		t.Skipf("Skipping test due to creation error: %v", err)
		return
	}
	require.NotNil(t, capture)
	defer capture.Close()

	// Test with nil sample
	err = capture.ProcessSample(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "sample is nil")
}

func TestSafeDesktopCapture_Stats(t *testing.T) {
	capture, err := NewSafeDesktopCapture()
	if err != nil {
		t.Skipf("Skipping test due to creation error: %v", err)
		return
	}
	require.NotNil(t, capture)
	defer capture.Close()

	stats := capture.GetStats()
	require.NotNil(t, stats)

	// Check that stats contain expected keys
	assert.Contains(t, stats, "lifecycle_stats")
	assert.Contains(t, stats, "health_status")
	assert.Contains(t, stats, "active_objects")

	// Check active objects count
	activeObjects := stats["active_objects"].(int)
	assert.Equal(t, 4, activeObjects) // pipeline, source, sink, bus
}

func TestSafeDesktopCapture_Close(t *testing.T) {
	capture, err := NewSafeDesktopCapture()
	if err != nil {
		t.Skipf("Skipping test due to creation error: %v", err)
		return
	}
	require.NotNil(t, capture)

	// Check initial state
	assert.True(t, capture.IsHealthy())

	// Close should not panic
	assert.NotPanics(t, func() {
		capture.Close()
	})

	// After close, objects should be invalid
	assert.False(t, capture.objectTracker.IsObjectValid(capture.pipelineID))
}
