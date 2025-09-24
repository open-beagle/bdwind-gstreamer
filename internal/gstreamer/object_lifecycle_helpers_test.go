package gstreamer

import (
	"testing"
	"time"

	"github.com/go-gst/go-gst/gst"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGstObjectTracker_Creation(t *testing.T) {
	tracker := NewGstObjectTracker()
	require.NotNil(t, tracker)
	defer tracker.Close()

	// Check initial stats
	stats := tracker.GetStats()
	assert.Equal(t, int64(0), stats.TotalRegistered)
	assert.Equal(t, int64(0), stats.CurrentActive)
}

func TestGstObjectTracker_TrackPipeline(t *testing.T) {
	gst.Init(nil)

	tracker := NewGstObjectTracker()
	require.NotNil(t, tracker)
	defer tracker.Close()

	// Create and track a pipeline
	pipeline, err := gst.NewPipeline("test-pipeline")
	require.NoError(t, err)
	require.NotNil(t, pipeline)

	id, err := tracker.TrackPipeline(pipeline, "test-pipeline")
	require.NoError(t, err)
	assert.NotEqual(t, uintptr(0), id)

	// Verify tracking
	assert.True(t, tracker.IsObjectValid(id))

	// Test safe usage
	usageCount := 0
	err = tracker.SafeUsePipeline(id, func(p *gst.Pipeline) error {
		usageCount++
		assert.Equal(t, pipeline, p)
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 1, usageCount)

	// Untrack
	err = tracker.UntrackObject(id)
	require.NoError(t, err)
	assert.False(t, tracker.IsObjectValid(id))

	pipeline.Unref()
}

func TestGstObjectTracker_TrackElement(t *testing.T) {
	gst.Init(nil)

	tracker := NewGstObjectTracker()
	require.NotNil(t, tracker)
	defer tracker.Close()

	// Create and track an element
	element, err := gst.NewElement("fakesrc")
	require.NoError(t, err)
	require.NotNil(t, element)

	id, err := tracker.TrackElement(element, "test-element")
	require.NoError(t, err)
	assert.NotEqual(t, uintptr(0), id)

	// Verify tracking
	assert.True(t, tracker.IsObjectValid(id))

	// Test safe usage
	usageCount := 0
	err = tracker.SafeUseElement(id, func(e *gst.Element) error {
		usageCount++
		assert.Equal(t, element, e)
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 1, usageCount)

	element.Unref()
}

func TestGstObjectTracker_TrackBus(t *testing.T) {
	gst.Init(nil)

	tracker := NewGstObjectTracker()
	require.NotNil(t, tracker)
	defer tracker.Close()

	// Create pipeline and get its bus
	pipeline, err := gst.NewPipeline("test-pipeline")
	require.NoError(t, err)
	require.NotNil(t, pipeline)

	bus := pipeline.GetBus()
	require.NotNil(t, bus)

	id, err := tracker.TrackBus(bus, "test-bus")
	require.NoError(t, err)
	assert.NotEqual(t, uintptr(0), id)

	// Verify tracking
	assert.True(t, tracker.IsObjectValid(id))

	// Test safe usage
	usageCount := 0
	err = tracker.SafeUseBus(id, func(b *gst.Bus) error {
		usageCount++
		assert.Equal(t, bus, b)
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 1, usageCount)

	pipeline.Unref()
}

func TestGstObjectTracker_InvalidObjects(t *testing.T) {
	tracker := NewGstObjectTracker()
	require.NotNil(t, tracker)
	defer tracker.Close()

	// Test with nil objects
	id, err := tracker.TrackPipeline(nil, "nil-pipeline")
	assert.Error(t, err)
	assert.Equal(t, uintptr(0), id)

	id, err = tracker.TrackElement(nil, "nil-element")
	assert.Error(t, err)
	assert.Equal(t, uintptr(0), id)

	id, err = tracker.TrackBus(nil, "nil-bus")
	assert.Error(t, err)
	assert.Equal(t, uintptr(0), id)

	id, err = tracker.TrackSample(nil, "nil-sample")
	assert.Error(t, err)
	assert.Equal(t, uintptr(0), id)

	// Test with invalid IDs
	assert.False(t, tracker.IsObjectValid(0))
	assert.False(t, tracker.IsObjectValid(12345))

	err = tracker.UntrackObject(0)
	assert.Error(t, err)

	err = tracker.UntrackObject(12345)
	assert.Error(t, err)
}

func TestGstObjectTracker_SafeUsageWithInvalidObjects(t *testing.T) {
	gst.Init(nil)

	tracker := NewGstObjectTracker()
	require.NotNil(t, tracker)
	defer tracker.Close()

	// Create and track an element
	element, err := gst.NewElement("fakesrc")
	require.NoError(t, err)

	id, err := tracker.TrackElement(element, "test-element")
	require.NoError(t, err)

	// Force invalidate the object
	err = tracker.ForceInvalidateObject(id)
	require.NoError(t, err)

	// Safe usage should fail
	err = tracker.SafeUseElement(id, func(e *gst.Element) error {
		t.Fatal("Should not be called with invalid object")
		return nil
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not valid")

	element.Unref()
}

func TestGstObjectTracker_ListActiveObjects(t *testing.T) {
	gst.Init(nil)

	tracker := NewGstObjectTracker()
	require.NotNil(t, tracker)
	defer tracker.Close()

	// Initially no active objects
	activeObjects := tracker.ListActiveObjects()
	assert.Empty(t, activeObjects)

	// Track some objects
	pipeline, err := gst.NewPipeline("test-pipeline")
	require.NoError(t, err)
	id1, err := tracker.TrackPipeline(pipeline, "test-pipeline")
	require.NoError(t, err)

	element, err := gst.NewElement("fakesrc")
	require.NoError(t, err)
	id2, err := tracker.TrackElement(element, "test-element")
	require.NoError(t, err)

	// Should have 2 active objects
	activeObjects = tracker.ListActiveObjects()
	assert.Len(t, activeObjects, 2)

	// Untrack one
	err = tracker.UntrackObject(id1)
	require.NoError(t, err)

	// Should have 1 active object
	activeObjects = tracker.ListActiveObjects()
	assert.Len(t, activeObjects, 1)
	assert.Equal(t, id2, activeObjects[0].ID)

	pipeline.Unref()
	element.Unref()
}

func TestGstObjectTracker_ValidateObject(t *testing.T) {
	gst.Init(nil)

	tracker := NewGstObjectTracker()
	require.NotNil(t, tracker)
	defer tracker.Close()

	// Test with nil object
	assert.False(t, tracker.ValidateObject(nil))

	// Test with valid objects
	pipeline, err := gst.NewPipeline("test-pipeline")
	require.NoError(t, err)
	assert.True(t, tracker.ValidateObject(pipeline))

	element, err := gst.NewElement("fakesrc")
	require.NoError(t, err)
	assert.True(t, tracker.ValidateObject(element))

	bus := pipeline.GetBus()
	assert.True(t, tracker.ValidateObject(bus))

	pipeline.Unref()
	element.Unref()
}

func TestGstObjectTracker_GetHealthStatus(t *testing.T) {
	gst.Init(nil)

	tracker := NewGstObjectTracker()
	require.NotNil(t, tracker)
	defer tracker.Close()

	// Check initial health status
	health := tracker.GetHealthStatus()
	assert.True(t, health["healthy"].(bool))
	assert.Equal(t, int64(0), health["total_registered"])
	assert.Equal(t, int64(0), health["current_active"])

	// Track some objects
	pipeline, err := gst.NewPipeline("test-pipeline")
	require.NoError(t, err)
	_, err = tracker.TrackPipeline(pipeline, "test-pipeline")
	require.NoError(t, err)

	element, err := gst.NewElement("fakesrc")
	require.NoError(t, err)
	_, err = tracker.TrackElement(element, "test-element")
	require.NoError(t, err)

	// Check updated health status
	health = tracker.GetHealthStatus()
	assert.True(t, health["healthy"].(bool))
	assert.Equal(t, int64(2), health["total_registered"])
	assert.Equal(t, int64(2), health["current_active"])

	pipeline.Unref()
	element.Unref()
}

func TestGstObjectTracker_DebugLogging(t *testing.T) {
	tracker := NewGstObjectTracker()
	require.NotNil(t, tracker)
	defer tracker.Close()

	// Test enabling/disabling debug logging
	tracker.EnableDebugLogging(true)
	assert.True(t, tracker.lifecycleManager.config.LogObjectEvents)

	tracker.EnableDebugLogging(false)
	assert.False(t, tracker.lifecycleManager.config.LogObjectEvents)
}

func TestGstObjectTracker_StackTrace(t *testing.T) {
	tracker := NewGstObjectTracker()
	require.NotNil(t, tracker)
	defer tracker.Close()

	// Test enabling/disabling stack trace
	tracker.EnableStackTrace(true)
	assert.True(t, tracker.lifecycleManager.config.EnableStackTrace)

	tracker.EnableStackTrace(false)
	assert.False(t, tracker.lifecycleManager.config.EnableStackTrace)
}

func TestGstObjectTracker_CustomConfig(t *testing.T) {
	config := ObjectLifecycleConfig{
		CleanupInterval:    time.Second,
		ObjectRetention:    time.Minute,
		EnableValidation:   false,
		ValidationInterval: 30 * time.Second,
		EnableStackTrace:   true,
		LogObjectEvents:    true,
	}

	tracker := NewGstObjectTrackerWithConfig(config)
	require.NotNil(t, tracker)
	defer tracker.Close()

	// Verify config was applied
	assert.Equal(t, time.Second, tracker.lifecycleManager.config.CleanupInterval)
	assert.Equal(t, time.Minute, tracker.lifecycleManager.config.ObjectRetention)
	assert.False(t, tracker.lifecycleManager.config.EnableValidation)
	assert.True(t, tracker.lifecycleManager.config.EnableStackTrace)
	assert.True(t, tracker.lifecycleManager.config.LogObjectEvents)
}
