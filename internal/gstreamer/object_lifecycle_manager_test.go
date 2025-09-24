package gstreamer

import (
	"testing"
	"time"

	"github.com/go-gst/go-gst/gst"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestObjectLifecycleManager_Creation(t *testing.T) {
	config := ObjectLifecycleConfig{
		CleanupInterval:    time.Second,
		ObjectRetention:    time.Minute,
		EnableValidation:   true,
		ValidationInterval: 30 * time.Second,
		EnableStackTrace:   false,
		LogObjectEvents:    false,
	}

	olm := NewObjectLifecycleManager(config)
	require.NotNil(t, olm)

	defer olm.Close()

	// Check initial stats
	stats := olm.GetStats()
	assert.Equal(t, int64(0), stats.TotalRegistered)
	assert.Equal(t, int64(0), stats.CurrentActive)
	assert.Equal(t, int64(0), stats.TotalReleased)
}

func TestObjectLifecycleManager_RegisterObject(t *testing.T) {
	gst.Init(nil)

	config := ObjectLifecycleConfig{
		CleanupInterval:  time.Hour, // Long interval to avoid cleanup during test
		ObjectRetention:  time.Hour,
		EnableValidation: false,
		LogObjectEvents:  false,
	}

	olm := NewObjectLifecycleManager(config)
	require.NotNil(t, olm)
	defer olm.Close()

	// Test registering a pipeline
	pipeline, err := gst.NewPipeline("test-pipeline")
	require.NoError(t, err)
	require.NotNil(t, pipeline)

	id, err := olm.RegisterObject(pipeline, "Pipeline", "test-pipeline")
	require.NoError(t, err)
	assert.NotEqual(t, uintptr(0), id)

	// Check stats
	stats := olm.GetStats()
	assert.Equal(t, int64(1), stats.TotalRegistered)
	assert.Equal(t, int64(1), stats.CurrentActive)

	// Check object validity
	assert.True(t, olm.IsValidObject(id))

	// Get object info
	info := olm.GetObjectInfo(id)
	require.NotNil(t, info)
	assert.Equal(t, "Pipeline", info.Type)
	assert.Equal(t, "test-pipeline", info.Name)
	assert.Equal(t, int32(1), info.RefCount)
	assert.True(t, info.Valid)
	assert.False(t, info.Released)

	pipeline.Unref()
}

func TestObjectLifecycleManager_UnregisterObject(t *testing.T) {
	gst.Init(nil)

	config := ObjectLifecycleConfig{
		CleanupInterval:  time.Hour,
		ObjectRetention:  time.Hour,
		EnableValidation: false,
		LogObjectEvents:  false,
	}

	olm := NewObjectLifecycleManager(config)
	require.NotNil(t, olm)
	defer olm.Close()

	// Register an element
	element, err := gst.NewElement("fakesrc")
	require.NoError(t, err)
	require.NotNil(t, element)

	id, err := olm.RegisterObject(element, "Element", "test-element")
	require.NoError(t, err)

	// Verify registration
	assert.True(t, olm.IsValidObject(id))
	stats := olm.GetStats()
	assert.Equal(t, int64(1), stats.CurrentActive)

	// Unregister the object
	err = olm.UnregisterObject(id)
	require.NoError(t, err)

	// Check that object is no longer valid
	assert.False(t, olm.IsValidObject(id))

	// Check stats
	stats = olm.GetStats()
	assert.Equal(t, int64(0), stats.CurrentActive)
	assert.Equal(t, int64(1), stats.TotalReleased)

	element.Unref()
}

func TestObjectLifecycleManager_MultipleReferences(t *testing.T) {
	gst.Init(nil)

	config := ObjectLifecycleConfig{
		CleanupInterval:  time.Hour,
		ObjectRetention:  time.Hour,
		EnableValidation: false,
		LogObjectEvents:  false,
	}

	olm := NewObjectLifecycleManager(config)
	require.NotNil(t, olm)
	defer olm.Close()

	// Register an element
	element, err := gst.NewElement("fakesink")
	require.NoError(t, err)
	require.NotNil(t, element)

	// Register the same object multiple times
	id1, err := olm.RegisterObject(element, "Element", "test-element")
	require.NoError(t, err)

	id2, err := olm.RegisterObject(element, "Element", "test-element")
	require.NoError(t, err)

	// Should get the same ID
	assert.Equal(t, id1, id2)

	// Check reference count
	info := olm.GetObjectInfo(id1)
	require.NotNil(t, info)
	assert.Equal(t, int32(2), info.RefCount)

	// Unregister once - should still be valid
	err = olm.UnregisterObject(id1)
	require.NoError(t, err)
	assert.True(t, olm.IsValidObject(id1))

	info = olm.GetObjectInfo(id1)
	require.NotNil(t, info)
	assert.Equal(t, int32(1), info.RefCount)

	// Unregister again - should become invalid
	err = olm.UnregisterObject(id1)
	require.NoError(t, err)
	assert.False(t, olm.IsValidObject(id1))

	element.Unref()
}

func TestObjectLifecycleManager_InvalidObject(t *testing.T) {
	config := ObjectLifecycleConfig{
		CleanupInterval:  time.Hour,
		ObjectRetention:  time.Hour,
		EnableValidation: false,
		LogObjectEvents:  false,
	}

	olm := NewObjectLifecycleManager(config)
	require.NotNil(t, olm)
	defer olm.Close()

	// Test with nil object
	id, err := olm.RegisterObject(nil, "Test", "nil-object")
	assert.Error(t, err)
	assert.Equal(t, uintptr(0), id)

	// Test with invalid ID
	assert.False(t, olm.IsValidObject(0))
	assert.False(t, olm.IsValidObject(12345))

	err = olm.UnregisterObject(0)
	assert.Error(t, err)

	err = olm.UnregisterObject(12345)
	assert.Error(t, err)
}

func TestObjectLifecycleManager_ForceInvalidate(t *testing.T) {
	gst.Init(nil)

	config := ObjectLifecycleConfig{
		CleanupInterval:  time.Hour,
		ObjectRetention:  time.Hour,
		EnableValidation: false,
		LogObjectEvents:  false,
	}

	olm := NewObjectLifecycleManager(config)
	require.NotNil(t, olm)
	defer olm.Close()

	// Register an element
	element, err := gst.NewElement("identity")
	require.NoError(t, err)
	require.NotNil(t, element)

	id, err := olm.RegisterObject(element, "Element", "test-element")
	require.NoError(t, err)

	// Verify it's valid
	assert.True(t, olm.IsValidObject(id))

	// Force invalidate
	err = olm.ForceInvalidateObject(id)
	require.NoError(t, err)

	// Should no longer be valid
	assert.False(t, olm.IsValidObject(id))

	// Check stats
	stats := olm.GetStats()
	assert.Equal(t, int64(1), stats.InvalidObjects)

	element.Unref()
}

func TestObjectLifecycleManager_ListActiveObjects(t *testing.T) {
	gst.Init(nil)

	config := ObjectLifecycleConfig{
		CleanupInterval:  time.Hour,
		ObjectRetention:  time.Hour,
		EnableValidation: false,
		LogObjectEvents:  false,
	}

	olm := NewObjectLifecycleManager(config)
	require.NotNil(t, olm)
	defer olm.Close()

	// Initially no active objects
	activeObjects := olm.ListActiveObjects()
	assert.Empty(t, activeObjects)

	// Register some objects
	pipeline, err := gst.NewPipeline("test-pipeline")
	require.NoError(t, err)
	id1, err := olm.RegisterObject(pipeline, "Pipeline", "test-pipeline")
	require.NoError(t, err)

	element, err := gst.NewElement("fakesrc")
	require.NoError(t, err)
	id2, err := olm.RegisterObject(element, "Element", "test-element")
	require.NoError(t, err)

	// Should have 2 active objects
	activeObjects = olm.ListActiveObjects()
	assert.Len(t, activeObjects, 2)

	// Unregister one
	err = olm.UnregisterObject(id1)
	require.NoError(t, err)

	// Should have 1 active object
	activeObjects = olm.ListActiveObjects()
	assert.Len(t, activeObjects, 1)
	assert.Equal(t, id2, activeObjects[0].ID)

	pipeline.Unref()
	element.Unref()
}

func TestObjectLifecycleManager_ValidateGstObject(t *testing.T) {
	gst.Init(nil)

	config := ObjectLifecycleConfig{
		CleanupInterval:  time.Hour,
		ObjectRetention:  time.Hour,
		EnableValidation: true,
		LogObjectEvents:  false,
	}

	olm := NewObjectLifecycleManager(config)
	require.NotNil(t, olm)
	defer olm.Close()

	// Test with nil object
	assert.False(t, olm.ValidateGstObject(nil))

	// Test with valid pipeline
	pipeline, err := gst.NewPipeline("test-pipeline")
	require.NoError(t, err)
	assert.True(t, olm.ValidateGstObject(pipeline))

	// Test with valid element
	element, err := gst.NewElement("fakesrc")
	require.NoError(t, err)
	assert.True(t, olm.ValidateGstObject(element))

	// Test with valid bus
	bus := pipeline.GetBus()
	assert.True(t, olm.ValidateGstObject(bus))

	pipeline.Unref()
	element.Unref()
}

func TestObjectLifecycleManager_Cleanup(t *testing.T) {
	gst.Init(nil)

	config := ObjectLifecycleConfig{
		CleanupInterval:  100 * time.Millisecond, // Short interval for testing
		ObjectRetention:  50 * time.Millisecond,  // Short retention for testing
		EnableValidation: false,
		LogObjectEvents:  false,
	}

	olm := NewObjectLifecycleManager(config)
	require.NotNil(t, olm)
	defer olm.Close()

	// Register and immediately unregister an object
	element, err := gst.NewElement("fakesrc")
	require.NoError(t, err)

	id, err := olm.RegisterObject(element, "Element", "test-element")
	require.NoError(t, err)

	err = olm.UnregisterObject(id)
	require.NoError(t, err)

	// Wait for cleanup to occur
	time.Sleep(200 * time.Millisecond)

	// Object should be cleaned up
	info := olm.GetObjectInfo(id)
	assert.Nil(t, info)

	// Check cleanup stats
	stats := olm.GetStats()
	assert.Greater(t, stats.CleanupRuns, int64(0))
	assert.Greater(t, stats.ObjectsCleaned, int64(0))

	element.Unref()
}

func TestObjectLifecycleManager_DefaultConfig(t *testing.T) {
	// Test with empty config to verify defaults are set
	config := ObjectLifecycleConfig{}

	olm := NewObjectLifecycleManager(config)
	require.NotNil(t, olm)
	defer olm.Close()

	// Verify defaults were applied
	assert.Equal(t, 30*time.Second, olm.config.CleanupInterval)
	assert.Equal(t, 5*time.Minute, olm.config.ObjectRetention)
	assert.Equal(t, 60*time.Second, olm.config.ValidationInterval)
}
