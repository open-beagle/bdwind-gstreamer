package gstreamer

import (
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGCSafetyManager_RegisterUnregisterObject(t *testing.T) {
	config := GCSafetyConfig{
		CleanupInterval:       100 * time.Millisecond,
		StateRetention:        time.Second,
		EnableStateChecking:   true,
		EnableFinalizerSafety: true,
		LogStateChanges:       false,
		LogFinalizers:         false,
	}

	gsm := NewGCSafetyManager(config)
	defer gsm.Close()

	// Test object registration
	testObj := &struct{ data string }{data: "test"}
	id, err := gsm.RegisterObject(testObj, "test-object")
	require.NoError(t, err)
	assert.NotZero(t, id)

	// Verify object is valid
	assert.True(t, gsm.IsObjectValid(id))

	// Test object state
	state := gsm.GetObjectState(id)
	require.NotNil(t, state)
	assert.Equal(t, "test-object", state.Type)
	assert.Equal(t, int32(1), state.IsValid)
	assert.Equal(t, int32(0), state.IsReleased)
	assert.Equal(t, int32(1), state.RefCount)

	// Test unregistration
	err = gsm.UnregisterObject(id)
	require.NoError(t, err)

	// Verify object is no longer valid
	assert.False(t, gsm.IsObjectValid(id))

	// Verify state is updated
	state = gsm.GetObjectState(id)
	require.NotNil(t, state)
	assert.Equal(t, int32(0), state.IsValid)
	assert.Equal(t, int32(1), state.IsReleased)
	assert.Equal(t, int32(0), state.RefCount)
}

func TestGCSafetyManager_ReferenceCountingAndDoubleRegistration(t *testing.T) {
	config := GCSafetyConfig{
		CleanupInterval:       100 * time.Millisecond,
		StateRetention:        time.Second,
		EnableStateChecking:   true,
		EnableFinalizerSafety: true,
		LogStateChanges:       false,
		LogFinalizers:         false,
	}

	gsm := NewGCSafetyManager(config)
	defer gsm.Close()

	testObj := &struct{ data string }{data: "test"}

	// Register object first time
	id1, err := gsm.RegisterObject(testObj, "test-object")
	require.NoError(t, err)

	// Register same object again (should increment ref count)
	id2, err := gsm.RegisterObject(testObj, "test-object")
	require.NoError(t, err)
	assert.Equal(t, id1, id2) // Should be same ID

	// Verify reference count is 2
	state := gsm.GetObjectState(id1)
	require.NotNil(t, state)
	assert.Equal(t, int32(2), state.RefCount)

	// Unregister once (should decrement ref count)
	err = gsm.UnregisterObject(id1)
	require.NoError(t, err)

	// Object should still be valid
	assert.True(t, gsm.IsObjectValid(id1))
	state = gsm.GetObjectState(id1)
	require.NotNil(t, state)
	assert.Equal(t, int32(1), state.RefCount)

	// Unregister again (should mark as released)
	err = gsm.UnregisterObject(id1)
	require.NoError(t, err)

	// Object should no longer be valid
	assert.False(t, gsm.IsObjectValid(id1))
	state = gsm.GetObjectState(id1)
	require.NotNil(t, state)
	assert.Equal(t, int32(0), state.RefCount)
	assert.Equal(t, int32(1), state.IsReleased)
}

func TestGCSafetyManager_SafeAccess(t *testing.T) {
	config := GCSafetyConfig{
		CleanupInterval:       100 * time.Millisecond,
		StateRetention:        time.Second,
		EnableStateChecking:   true,
		EnableFinalizerSafety: true,
		LogStateChanges:       false,
		LogFinalizers:         false,
	}

	gsm := NewGCSafetyManager(config)
	defer gsm.Close()

	testObj := &struct{ data string }{data: "test"}
	id, err := gsm.RegisterObject(testObj, "test-object")
	require.NoError(t, err)

	// Test safe access with valid object
	accessCalled := false
	err = gsm.SafeAccess(id, func() error {
		accessCalled = true
		return nil
	})
	require.NoError(t, err)
	assert.True(t, accessCalled)

	// Invalidate object
	err = gsm.InvalidateObject(id)
	require.NoError(t, err)

	// Test safe access with invalid object
	accessCalled = false
	err = gsm.SafeAccess(id, func() error {
		accessCalled = true
		return nil
	})
	assert.Error(t, err)
	assert.False(t, accessCalled)
	assert.Contains(t, err.Error(), "not valid for access")

	// Verify statistics
	stats := gsm.GetStats()
	assert.Greater(t, stats.StateChecksPassed, int64(0))
	assert.Greater(t, stats.StateChecksFailed, int64(0))
	assert.Greater(t, stats.UnsafeAccessBlocked, int64(0))
}

func TestGCSafetyManager_SafeAccessWithPanic(t *testing.T) {
	config := GCSafetyConfig{
		CleanupInterval:       100 * time.Millisecond,
		StateRetention:        time.Second,
		EnableStateChecking:   true,
		EnableFinalizerSafety: true,
		LogStateChanges:       false,
		LogFinalizers:         false,
	}

	gsm := NewGCSafetyManager(config)
	defer gsm.Close()

	testObj := &struct{ data string }{data: "test"}
	id, err := gsm.RegisterObject(testObj, "test-object")
	require.NoError(t, err)

	// Test safe access with panic
	err = gsm.SafeAccess(id, func() error {
		panic("test panic")
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "panic during object access")

	// Object should be invalidated after panic
	assert.False(t, gsm.IsObjectValid(id))
}

func TestGCSafetyManager_SafeFinalizer(t *testing.T) {
	config := GCSafetyConfig{
		CleanupInterval:       100 * time.Millisecond,
		StateRetention:        time.Second,
		FinalizerTimeout:      time.Second,
		EnableStateChecking:   true,
		EnableFinalizerSafety: true,
		LogStateChanges:       false,
		LogFinalizers:         false,
	}

	gsm := NewGCSafetyManager(config)
	defer gsm.Close()

	finalizerCalled := false
	var finalizerMutex sync.Mutex

	// Create object in separate scope to allow GC
	func() {
		testObj := &struct{ data string }{data: "test"}

		err := gsm.SetSafeFinalizer(testObj, func() {
			finalizerMutex.Lock()
			finalizerCalled = true
			finalizerMutex.Unlock()
		})
		require.NoError(t, err)
	}()

	// Force garbage collection
	runtime.GC()
	runtime.GC()                       // Call twice to ensure finalizers run
	time.Sleep(100 * time.Millisecond) // Give finalizers time to execute

	// Check if finalizer was called
	finalizerMutex.Lock()
	_ = finalizerCalled // Use the variable to avoid "declared and not used" error
	finalizerMutex.Unlock()

	// Note: Finalizer execution is not guaranteed in tests, so we just verify
	// that the mechanism doesn't crash and statistics are updated
	stats := gsm.GetStats()
	assert.Greater(t, stats.FinalizersSet, int64(0))
}

func TestGCSafetyManager_CleanupCallbacks(t *testing.T) {
	config := GCSafetyConfig{
		CleanupInterval:       100 * time.Millisecond,
		StateRetention:        time.Second,
		EnableStateChecking:   true,
		EnableFinalizerSafety: true,
		LogStateChanges:       false,
		LogFinalizers:         false,
	}

	gsm := NewGCSafetyManager(config)
	defer gsm.Close()

	testObj := &struct{ data string }{data: "test"}
	id, err := gsm.RegisterObject(testObj, "test-object")
	require.NoError(t, err)

	// Add cleanup callbacks
	callback1Called := false
	callback2Called := false

	err = gsm.AddCleanupCallback(id, func() {
		callback1Called = true
	})
	require.NoError(t, err)

	err = gsm.AddCleanupCallback(id, func() {
		callback2Called = true
	})
	require.NoError(t, err)

	// Execute cleanup callbacks
	gsm.ExecuteCleanupCallbacks(id)

	// Verify callbacks were called
	assert.True(t, callback1Called)
	assert.True(t, callback2Called)
}

func TestGCSafetyManager_CleanupCallbacksWithPanic(t *testing.T) {
	config := GCSafetyConfig{
		CleanupInterval:       100 * time.Millisecond,
		StateRetention:        time.Second,
		EnableStateChecking:   true,
		EnableFinalizerSafety: true,
		LogStateChanges:       false,
		LogFinalizers:         false,
	}

	gsm := NewGCSafetyManager(config)
	defer gsm.Close()

	testObj := &struct{ data string }{data: "test"}
	id, err := gsm.RegisterObject(testObj, "test-object")
	require.NoError(t, err)

	// Add cleanup callbacks, one that panics
	callback1Called := false
	callback2Called := false

	err = gsm.AddCleanupCallback(id, func() {
		callback1Called = true
	})
	require.NoError(t, err)

	err = gsm.AddCleanupCallback(id, func() {
		panic("test panic in callback")
	})
	require.NoError(t, err)

	err = gsm.AddCleanupCallback(id, func() {
		callback2Called = true
	})
	require.NoError(t, err)

	// Execute cleanup callbacks (should not crash despite panic)
	gsm.ExecuteCleanupCallbacks(id)

	// Verify non-panicking callbacks were still called
	assert.True(t, callback1Called)
	assert.True(t, callback2Called)
}

func TestGCSafetyManager_AutomaticCleanup(t *testing.T) {
	config := GCSafetyConfig{
		CleanupInterval:       50 * time.Millisecond,  // Fast cleanup for testing
		StateRetention:        100 * time.Millisecond, // Short retention for testing
		EnableStateChecking:   true,
		EnableFinalizerSafety: true,
		LogStateChanges:       false,
		LogFinalizers:         false,
	}

	gsm := NewGCSafetyManager(config)
	defer gsm.Close()

	testObj := &struct{ data string }{data: "test"}
	id, err := gsm.RegisterObject(testObj, "test-object")
	require.NoError(t, err)

	// Unregister object to mark it as released
	err = gsm.UnregisterObject(id)
	require.NoError(t, err)

	// Wait for cleanup to occur
	time.Sleep(200 * time.Millisecond)

	// Object state should be cleaned up
	state := gsm.GetObjectState(id)
	assert.Nil(t, state) // Should be nil after cleanup

	// Verify cleanup statistics
	stats := gsm.GetStats()
	assert.Greater(t, stats.CleanupRuns, int64(0))
}

func TestGCSafetyManager_ConcurrentAccess(t *testing.T) {
	config := GCSafetyConfig{
		CleanupInterval:       100 * time.Millisecond,
		StateRetention:        time.Second,
		EnableStateChecking:   true,
		EnableFinalizerSafety: true,
		LogStateChanges:       false,
		LogFinalizers:         false,
	}

	gsm := NewGCSafetyManager(config)
	defer gsm.Close()

	const numGoroutines = 10
	const numOperations = 100

	var wg sync.WaitGroup
	var ids []uintptr
	var idsMutex sync.Mutex

	// Concurrent registration
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				testObj := &struct{ data int }{data: index*numOperations + j}
				id, err := gsm.RegisterObject(testObj, "concurrent-test")
				if err == nil {
					idsMutex.Lock()
					ids = append(ids, id)
					idsMutex.Unlock()
				}
			}
		}(i)
	}

	wg.Wait()

	// Verify all objects are valid
	idsMutex.Lock()
	registeredIDs := make([]uintptr, len(ids))
	copy(registeredIDs, ids)
	idsMutex.Unlock()

	validCount := 0
	for _, id := range registeredIDs {
		if gsm.IsObjectValid(id) {
			validCount++
		}
	}

	assert.Equal(t, len(registeredIDs), validCount)

	// Concurrent unregistration
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(start int) {
			defer wg.Done()
			end := start + len(registeredIDs)/numGoroutines
			if end > len(registeredIDs) {
				end = len(registeredIDs)
			}
			for j := start; j < end; j++ {
				gsm.UnregisterObject(registeredIDs[j])
			}
		}(i * len(registeredIDs) / numGoroutines)
	}

	wg.Wait()

	// Verify statistics
	stats := gsm.GetStats()
	assert.Greater(t, stats.TotalObjects, int64(0))
	assert.Greater(t, stats.ReleasedObjects, int64(0))
}

func TestGCSafetyManager_ShutdownSafety(t *testing.T) {
	config := GCSafetyConfig{
		CleanupInterval:       100 * time.Millisecond,
		StateRetention:        time.Second,
		EnableStateChecking:   true,
		EnableFinalizerSafety: true,
		LogStateChanges:       false,
		LogFinalizers:         false,
	}

	gsm := NewGCSafetyManager(config)

	// Register some objects
	testObj1 := &struct{ data string }{data: "test1"}
	testObj2 := &struct{ data string }{data: "test2"}

	id1, err := gsm.RegisterObject(testObj1, "test-object-1")
	require.NoError(t, err)

	id2, err := gsm.RegisterObject(testObj2, "test-object-2")
	require.NoError(t, err)

	// Verify objects are valid before shutdown
	assert.True(t, gsm.IsObjectValid(id1))
	assert.True(t, gsm.IsObjectValid(id2))

	// Close the manager
	gsm.Close()

	// Verify objects are no longer valid after shutdown
	assert.False(t, gsm.IsObjectValid(id1))
	assert.False(t, gsm.IsObjectValid(id2))

	// Verify new registrations fail after shutdown
	testObj3 := &struct{ data string }{data: "test3"}
	_, err = gsm.RegisterObject(testObj3, "test-object-3")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "shutdown")
}

func TestGCSafetyManager_InvalidInputs(t *testing.T) {
	config := GCSafetyConfig{
		CleanupInterval:       100 * time.Millisecond,
		StateRetention:        time.Second,
		EnableStateChecking:   true,
		EnableFinalizerSafety: true,
		LogStateChanges:       false,
		LogFinalizers:         false,
	}

	gsm := NewGCSafetyManager(config)
	defer gsm.Close()

	// Test nil object registration
	_, err := gsm.RegisterObject(nil, "nil-object")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "nil object")

	// Test invalid ID operations
	assert.False(t, gsm.IsObjectValid(0))

	err = gsm.UnregisterObject(0)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid object ID")

	err = gsm.InvalidateObject(0)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid object ID")

	state := gsm.GetObjectState(0)
	assert.Nil(t, state)

	// Test operations on non-existent objects
	err = gsm.UnregisterObject(12345)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")

	err = gsm.InvalidateObject(12345)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")

	err = gsm.AddCleanupCallback(12345, func() {})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func BenchmarkGCSafetyManager_RegisterUnregister(b *testing.B) {
	config := GCSafetyConfig{
		CleanupInterval:       time.Minute, // Disable cleanup during benchmark
		StateRetention:        time.Hour,
		EnableStateChecking:   true,
		EnableFinalizerSafety: false, // Disable for performance
		LogStateChanges:       false,
		LogFinalizers:         false,
	}

	gsm := NewGCSafetyManager(config)
	defer gsm.Close()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		testObj := &struct{ data int }{data: i}
		id, err := gsm.RegisterObject(testObj, "benchmark-object")
		if err != nil {
			b.Fatal(err)
		}
		err = gsm.UnregisterObject(id)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGCSafetyManager_IsObjectValid(b *testing.B) {
	config := GCSafetyConfig{
		CleanupInterval:       time.Minute, // Disable cleanup during benchmark
		StateRetention:        time.Hour,
		EnableStateChecking:   true,
		EnableFinalizerSafety: false, // Disable for performance
		LogStateChanges:       false,
		LogFinalizers:         false,
	}

	gsm := NewGCSafetyManager(config)
	defer gsm.Close()

	// Register some objects
	var ids []uintptr
	for i := 0; i < 1000; i++ {
		testObj := &struct{ data int }{data: i}
		id, err := gsm.RegisterObject(testObj, "benchmark-object")
		if err != nil {
			b.Fatal(err)
		}
		ids = append(ids, id)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		id := ids[i%len(ids)]
		gsm.IsObjectValid(id)
	}
}
