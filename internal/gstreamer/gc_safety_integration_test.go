package gstreamer

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGCSafetyIntegration_MemoryManager(t *testing.T) {
	config := MemoryManagerConfig{
		MaxBufferSize:        1024 * 1024, // 1MB
		BufferPoolSize:       10,
		BufferPoolTimeout:    100 * time.Millisecond,
		MonitorInterval:      time.Second,
		LeakCheckInterval:    time.Second,
		MaxMemoryThreshold:   10 * 1024 * 1024, // 10MB
		GCTriggerThreshold:   5 * 1024 * 1024,  // 5MB
		TrackObjectLifecycle: true,
		ObjectRetentionTime:  time.Second,
		EnableLeakDetection:  true,
		LeakThreshold:        time.Second,
	}

	mm := NewMemoryManager(config)
	defer mm.Close()

	// Test object tracking with GC safety
	testObj := &struct{ data string }{data: "test"}
	id := mm.TrackObject(testObj, "test-object", 100)
	require.NotZero(t, id)

	// Verify object is valid
	assert.True(t, mm.IsObjectValid(id))

	// Test safe object access
	accessCalled := false
	err := mm.SafeObjectAccess(id, func() error {
		accessCalled = true
		return nil
	})
	require.NoError(t, err)
	assert.True(t, accessCalled)

	// Release object
	mm.ReleaseObject(id)

	// Verify object is no longer valid
	assert.False(t, mm.IsObjectValid(id))

	// Test safe access after release
	accessCalled = false
	err = mm.SafeObjectAccess(id, func() error {
		accessCalled = true
		return nil
	})
	assert.Error(t, err)
	assert.False(t, accessCalled)
}

func TestGCSafetyIntegration_ObjectLifecycleManager(t *testing.T) {
	config := ObjectLifecycleConfig{
		CleanupInterval:    100 * time.Millisecond,
		ObjectRetention:    time.Second,
		EnableValidation:   true,
		ValidationInterval: time.Second,
		EnableStackTrace:   false,
		LogObjectEvents:    false,
	}

	olm := NewObjectLifecycleManager(config)
	defer olm.Close()

	// Test object registration with GC safety
	testObj := &struct{ data string }{data: "test"}
	id, err := olm.RegisterObject(testObj, "test-object", "test-instance")
	require.NoError(t, err)
	require.NotZero(t, id)

	// Verify object is valid
	assert.True(t, olm.IsValidObject(id))

	// Test safe object access
	accessCalled := false
	err = olm.SafeObjectAccess(id, func() error {
		accessCalled = true
		return nil
	})
	require.NoError(t, err)
	assert.True(t, accessCalled)

	// Unregister object
	err = olm.UnregisterObject(id)
	require.NoError(t, err)

	// Verify object is no longer valid
	assert.False(t, olm.IsValidObject(id))

	// Test safe access after unregistration
	accessCalled = false
	err = olm.SafeObjectAccess(id, func() error {
		accessCalled = true
		return nil
	})
	assert.Error(t, err)
	assert.False(t, accessCalled)
}

func TestGCSafetyIntegration_BufferPoolSafety(t *testing.T) {
	config := MemoryManagerConfig{
		MaxBufferSize:        1024,
		BufferPoolSize:       5,
		BufferPoolTimeout:    100 * time.Millisecond,
		MonitorInterval:      time.Second,
		TrackObjectLifecycle: true,
		ObjectRetentionTime:  time.Second,
	}

	mm := NewMemoryManager(config)
	defer mm.Close()

	// Test safe buffer allocation
	buffer, err := mm.SafeAllocateBuffer(512)
	require.NoError(t, err)
	require.NotNil(t, buffer)
	assert.Equal(t, 512, len(buffer))

	// Test safe buffer copy
	srcData := []byte("test data for buffer copy")
	copiedBuffer, err := mm.SafeCopyBuffer(srcData)
	require.NoError(t, err)
	require.NotNil(t, copiedBuffer)
	assert.Equal(t, srcData, copiedBuffer)

	// Return buffers to pool
	mm.ReturnBuffer(buffer)
	mm.ReturnBuffer(copiedBuffer)

	// Verify memory manager health
	assert.True(t, mm.IsHealthy())

	// Get health status
	healthStatus := mm.GetHealthStatus()
	assert.True(t, healthStatus["healthy"].(bool))
}

func TestGCSafetyIntegration_PanicRecovery(t *testing.T) {
	config := MemoryManagerConfig{
		MaxBufferSize:        1024,
		BufferPoolSize:       5,
		BufferPoolTimeout:    100 * time.Millisecond,
		TrackObjectLifecycle: true,
		ObjectRetentionTime:  time.Second,
	}

	mm := NewMemoryManager(config)
	defer mm.Close()

	// Track an object
	testObj := &struct{ data string }{data: "test"}
	id := mm.TrackObject(testObj, "panic-test-object", 100)
	require.NotZero(t, id)

	// Test panic recovery in safe access
	err := mm.SafeObjectAccess(id, func() error {
		panic("test panic for recovery")
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "panic during object access")

	// Object should be invalidated after panic
	assert.False(t, mm.IsObjectValid(id))
}

func TestGCSafetyIntegration_ConcurrentSafety(t *testing.T) {
	config := ObjectLifecycleConfig{
		CleanupInterval:    100 * time.Millisecond,
		ObjectRetention:    time.Second,
		EnableValidation:   true,
		ValidationInterval: time.Second,
		EnableStackTrace:   false,
		LogObjectEvents:    false,
	}

	olm := NewObjectLifecycleManager(config)
	defer olm.Close()

	const numGoroutines = 5
	const numOperations = 20

	// Test concurrent registration and access
	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(index int) {
			defer func() { done <- true }()

			for j := 0; j < numOperations; j++ {
				testObj := &struct{ data int }{data: index*numOperations + j}
				id, err := olm.RegisterObject(testObj, "concurrent-test", "instance")
				if err != nil {
					continue
				}

				// Test safe access
				olm.SafeObjectAccess(id, func() error {
					// Simulate some work
					time.Sleep(time.Microsecond)
					return nil
				})

				// Unregister
				olm.UnregisterObject(id)
			}
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < numGoroutines; i++ {
		select {
		case <-done:
			// Goroutine completed
		case <-time.After(10 * time.Second):
			t.Fatal("Test timed out waiting for goroutines to complete")
		}
	}

	// Verify final state
	stats := olm.GetStats()
	assert.Greater(t, stats.TotalRegistered, int64(0))
}

func TestGCSafetyIntegration_ShutdownSafety(t *testing.T) {
	config := MemoryManagerConfig{
		MaxBufferSize:        1024,
		BufferPoolSize:       5,
		BufferPoolTimeout:    100 * time.Millisecond,
		TrackObjectLifecycle: true,
		ObjectRetentionTime:  time.Second,
	}

	mm := NewMemoryManager(config)

	// Track some objects
	var ids []uintptr
	for i := 0; i < 5; i++ {
		testObj := &struct{ data int }{data: i}
		id := mm.TrackObject(testObj, "shutdown-test", int64(100+i))
		if id != 0 {
			ids = append(ids, id)
		}
	}

	// Verify objects are valid before shutdown
	for _, id := range ids {
		assert.True(t, mm.IsObjectValid(id))
	}

	// Close the memory manager
	mm.Close()

	// Verify objects are no longer valid after shutdown
	for _, id := range ids {
		assert.False(t, mm.IsObjectValid(id))
	}

	// Verify safe access fails after shutdown
	for _, id := range ids {
		err := mm.SafeObjectAccess(id, func() error {
			return nil
		})
		assert.Error(t, err)
	}
}
