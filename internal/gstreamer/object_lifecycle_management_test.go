package gstreamer

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestObjectLifecycleManagerBasicOperations tests basic object lifecycle operations
// This addresses requirement 3.1: ObjectLifecycleManager should track object creation and destruction
func TestObjectLifecycleManagerBasicOperations(t *testing.T) {
	config := ObjectLifecycleConfig{
		CleanupInterval:    100 * time.Millisecond,
		ObjectRetention:    time.Second,
		EnableValidation:   true,
		ValidationInterval: 500 * time.Millisecond,
		EnableStackTrace:   true,
		LogObjectEvents:    false, // Disable for cleaner test output
	}

	olm := NewObjectLifecycleManager(config)
	defer olm.Close()

	// Test object registration
	testObj := &struct {
		data string
		id   int
	}{data: "test-object", id: 1}

	id, err := olm.RegisterObject(testObj, "test-struct", "test-object-1")
	require.NoError(t, err)
	assert.NotZero(t, id)

	// Verify object is tracked
	assert.True(t, olm.IsValidObject(id))

	// Get object info
	info := olm.GetObjectInfo(id)
	require.NotNil(t, info)
	assert.Equal(t, "test-struct", info.Type)
	assert.Equal(t, "test-object-1", info.Name)
	assert.Equal(t, int32(1), info.RefCount)
	assert.True(t, info.Valid)
	assert.False(t, info.Released)

	// Test object unregistration
	err = olm.UnregisterObject(id)
	require.NoError(t, err)

	// Verify object is no longer valid
	assert.False(t, olm.IsValidObject(id))

	// Verify object info reflects release
	info = olm.GetObjectInfo(id)
	require.NotNil(t, info)
	assert.True(t, info.Released)
	assert.False(t, info.Valid)
}

// TestObjectLifecycleManagerReferenceCountingConcurrent tests concurrent reference counting
// This addresses requirement 3.2: ObjectLifecycleManager should handle reference counting safely
func TestObjectLifecycleManagerReferenceCountingConcurrent(t *testing.T) {
	config := ObjectLifecycleConfig{
		CleanupInterval:  time.Second, // Disable cleanup during test
		ObjectRetention:  time.Hour,
		EnableValidation: true,
		EnableStackTrace: false, // Disable for performance
		LogObjectEvents:  false,
	}

	olm := NewObjectLifecycleManager(config)
	defer olm.Close()

	testObj := &struct{ data string }{data: "concurrent-test"}

	// Register object initially
	id, err := olm.RegisterObject(testObj, "concurrent-struct", "concurrent-test")
	require.NoError(t, err)

	const numGoroutines = 20
	const refsPerGoroutine = 50

	var wg sync.WaitGroup
	var registrationCount int64
	var unregistrationCount int64

	// Concurrent registration (should increment ref count)
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < refsPerGoroutine; j++ {
				// Register same object (should increment ref count)
				newID, err := olm.RegisterObject(testObj, "concurrent-struct", "concurrent-test")
				if err == nil && newID == id {
					atomic.AddInt64(&registrationCount, 1)
				}
			}
		}()
	}

	wg.Wait()

	// Verify high reference count
	info := olm.GetObjectInfo(id)
	require.NotNil(t, info)
	expectedRefCount := int32(1 + registrationCount) // Initial + concurrent registrations
	assert.Equal(t, expectedRefCount, info.RefCount)
	assert.True(t, info.Valid)

	// Concurrent unregistration
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < refsPerGoroutine; j++ {
				err := olm.UnregisterObject(id)
				if err == nil {
					atomic.AddInt64(&unregistrationCount, 1)
				}
			}
		}()
	}

	wg.Wait()

	// Verify final state
	info = olm.GetObjectInfo(id)
	require.NotNil(t, info)

	t.Logf("Reference counting test results:")
	t.Logf("  Initial registrations: %d", registrationCount)
	t.Logf("  Unregistrations: %d", unregistrationCount)
	t.Logf("  Final ref count: %d", info.RefCount)
	t.Logf("  Object valid: %v", info.Valid)
	t.Logf("  Object released: %v", info.Released)

	// Object should be released when ref count reaches 0
	if info.RefCount == 0 {
		assert.True(t, info.Released)
		assert.False(t, info.Valid)
	} else {
		assert.False(t, info.Released)
		assert.True(t, info.Valid)
	}
}

// TestObjectLifecycleManagerSafeAccess tests safe object access patterns
// This addresses requirement 3.1: ObjectLifecycleManager should provide safe access to objects
func TestObjectLifecycleManagerSafeAccess(t *testing.T) {
	config := ObjectLifecycleConfig{
		CleanupInterval:  time.Second,
		ObjectRetention:  time.Hour,
		EnableValidation: true,
		EnableStackTrace: true,
		LogObjectEvents:  false,
	}

	olm := NewObjectLifecycleManager(config)
	defer olm.Close()

	testObj := &struct {
		data  string
		value int
	}{data: "safe-access-test", value: 42}

	id, err := olm.RegisterObject(testObj, "safe-access-struct", "safe-access-test")
	require.NoError(t, err)

	// Test safe access with valid object
	accessCount := 0
	err = olm.SafeObjectAccess(id, func() error {
		accessCount++
		// Simulate object access
		testObj.value = 100
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 1, accessCount)
	assert.Equal(t, 100, testObj.value)

	// Test safe access with panic recovery
	err = olm.SafeObjectAccess(id, func() error {
		panic("test panic in safe access")
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "panic during object access")

	// Object should be invalidated after panic
	assert.False(t, olm.IsValidObject(id))

	// Register new object for invalid access test
	testObj2 := &struct{ data string }{data: "invalid-test"}
	id2, err := olm.RegisterObject(testObj2, "invalid-struct", "invalid-test")
	require.NoError(t, err)

	// Unregister object
	err = olm.UnregisterObject(id2)
	require.NoError(t, err)

	// Test safe access with invalid object
	accessCount = 0
	err = olm.SafeObjectAccess(id2, func() error {
		accessCount++
		return nil
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not valid for access")
	assert.Equal(t, 0, accessCount) // Access function should not be called
}

// TestObjectLifecycleManagerConcurrentSafeAccess tests concurrent safe access
// This addresses requirement 3.2: ObjectLifecycleManager should handle concurrent safe access
func TestObjectLifecycleManagerConcurrentSafeAccess(t *testing.T) {
	config := ObjectLifecycleConfig{
		CleanupInterval:  time.Second,
		ObjectRetention:  time.Hour,
		EnableValidation: true,
		EnableStackTrace: false, // Disable for performance
		LogObjectEvents:  false,
	}

	olm := NewObjectLifecycleManager(config)
	defer olm.Close()

	testObj := &struct {
		data    string
		counter int64
		mutex   sync.Mutex
	}{data: "concurrent-safe-access"}

	id, err := olm.RegisterObject(testObj, "concurrent-safe-struct", "concurrent-safe-test")
	require.NoError(t, err)

	const numGoroutines = 15
	const accessesPerGoroutine = 100

	var wg sync.WaitGroup
	var successfulAccesses int64
	var failedAccesses int64

	// Concurrent safe access
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()

			for j := 0; j < accessesPerGoroutine; j++ {
				err := olm.SafeObjectAccess(id, func() error {
					// Simulate thread-safe object access
					testObj.mutex.Lock()
					testObj.counter++
					currentCount := testObj.counter
					testObj.mutex.Unlock()

					// Simulate some work
					time.Sleep(time.Microsecond * 10)

					// Verify object state is consistent
					testObj.mutex.Lock()
					if testObj.counter < currentCount {
						testObj.mutex.Unlock()
						return assert.AnError // Inconsistent state
					}
					testObj.mutex.Unlock()

					return nil
				})

				if err == nil {
					atomic.AddInt64(&successfulAccesses, 1)
				} else {
					atomic.AddInt64(&failedAccesses, 1)
				}
			}
		}(i)
	}

	wg.Wait()

	expectedAccesses := int64(numGoroutines * accessesPerGoroutine)
	t.Logf("Concurrent safe access test results:")
	t.Logf("  Expected accesses: %d", expectedAccesses)
	t.Logf("  Successful accesses: %d", successfulAccesses)
	t.Logf("  Failed accesses: %d", failedAccesses)
	t.Logf("  Final counter value: %d", testObj.counter)

	// Verify all accesses succeeded
	assert.Equal(t, expectedAccesses, successfulAccesses)
	assert.Equal(t, int64(0), failedAccesses)
	assert.Equal(t, expectedAccesses, testObj.counter)

	// Verify object is still valid
	assert.True(t, olm.IsValidObject(id))
}

// TestObjectLifecycleManagerGarbageCollectionSafety tests GC safety
// This addresses requirement 3.3: ObjectLifecycleManager should handle GC safely
func TestObjectLifecycleManagerGarbageCollectionSafety(t *testing.T) {
	config := ObjectLifecycleConfig{
		CleanupInterval:  100 * time.Millisecond,
		ObjectRetention:  200 * time.Millisecond,
		EnableValidation: true,
		EnableStackTrace: false,
		LogObjectEvents:  false,
	}

	olm := NewObjectLifecycleManager(config)
	defer olm.Close()

	var objectIDs []uintptr
	var finalizerCallCount int64

	// Create objects in separate scope to allow GC
	func() {
		for i := 0; i < 50; i++ {
			testObj := &struct {
				data  string
				index int
			}{
				data:  "gc-safety-test",
				index: i,
			}

			id, err := olm.RegisterObject(testObj, "gc-safety-struct", "gc-safety-test")
			require.NoError(t, err)
			objectIDs = append(objectIDs, id)

			// Set finalizer to track GC
			runtime.SetFinalizer(testObj, func(obj *struct {
				data  string
				index int
			}) {
				atomic.AddInt64(&finalizerCallCount, 1)
			})
		}
	}()

	// Verify all objects are initially valid
	validCount := 0
	for _, id := range objectIDs {
		if olm.IsValidObject(id) {
			validCount++
		}
	}
	assert.Equal(t, len(objectIDs), validCount)

	// Unregister half the objects
	for i := 0; i < len(objectIDs)/2; i++ {
		err := olm.UnregisterObject(objectIDs[i])
		require.NoError(t, err)
	}

	// Force garbage collection multiple times
	for i := 0; i < 5; i++ {
		runtime.GC()
		runtime.GC()
		time.Sleep(50 * time.Millisecond)
	}

	// Wait for cleanup to occur
	time.Sleep(300 * time.Millisecond)

	// Check object states after GC
	validAfterGC := 0
	releasedAfterGC := 0
	for _, id := range objectIDs {
		if olm.IsValidObject(id) {
			validAfterGC++
		} else {
			info := olm.GetObjectInfo(id)
			if info != nil && info.Released {
				releasedAfterGC++
			}
		}
	}

	t.Logf("GC safety test results:")
	t.Logf("  Total objects created: %d", len(objectIDs))
	t.Logf("  Objects unregistered: %d", len(objectIDs)/2)
	t.Logf("  Valid objects after GC: %d", validAfterGC)
	t.Logf("  Released objects after GC: %d", releasedAfterGC)
	t.Logf("  Finalizers called: %d", finalizerCallCount)

	// Verify GC safety - no crashes should occur
	assert.Greater(t, finalizerCallCount, int64(0), "Some finalizers should have been called")
	assert.LessOrEqual(t, validAfterGC, len(objectIDs)/2+5, "Should not have more valid objects than expected")

	// Verify manager is still functional after GC
	newTestObj := &struct{ data string }{data: "post-gc-test"}
	newID, err := olm.RegisterObject(newTestObj, "post-gc-struct", "post-gc-test")
	require.NoError(t, err)
	assert.True(t, olm.IsValidObject(newID))
}

// TestObjectLifecycleManagerCleanupAndValidation tests cleanup and validation
// This addresses requirement 3.1: ObjectLifecycleManager should clean up invalid objects
func TestObjectLifecycleManagerCleanupAndValidation(t *testing.T) {
	config := ObjectLifecycleConfig{
		CleanupInterval:    50 * time.Millisecond,  // Fast cleanup
		ObjectRetention:    100 * time.Millisecond, // Short retention
		EnableValidation:   true,
		ValidationInterval: 75 * time.Millisecond,
		EnableStackTrace:   false,
		LogObjectEvents:    false,
	}

	olm := NewObjectLifecycleManager(config)
	defer olm.Close()

	var objectIDs []uintptr

	// Create and register multiple objects
	for i := 0; i < 20; i++ {
		testObj := &struct {
			data  string
			index int
		}{
			data:  "cleanup-test",
			index: i,
		}

		id, err := olm.RegisterObject(testObj, "cleanup-struct", "cleanup-test")
		require.NoError(t, err)
		objectIDs = append(objectIDs, id)
	}

	// Verify all objects are initially active
	activeObjects := olm.ListActiveObjects()
	assert.Equal(t, len(objectIDs), len(activeObjects))

	// Unregister half the objects
	for i := 0; i < len(objectIDs)/2; i++ {
		err := olm.UnregisterObject(objectIDs[i])
		require.NoError(t, err)
	}

	// Force invalidate some objects
	for i := len(objectIDs) / 2; i < len(objectIDs)*3/4; i++ {
		err := olm.ForceInvalidateObject(objectIDs[i])
		require.NoError(t, err)
	}

	// Wait for cleanup and validation cycles
	time.Sleep(200 * time.Millisecond)

	// Check final state
	finalActiveObjects := olm.ListActiveObjects()
	stats := olm.GetStats()

	t.Logf("Cleanup and validation test results:")
	t.Logf("  Initial objects: %d", len(objectIDs))
	t.Logf("  Objects unregistered: %d", len(objectIDs)/2)
	t.Logf("  Objects force invalidated: %d", len(objectIDs)/4)
	t.Logf("  Final active objects: %d", len(finalActiveObjects))
	t.Logf("  Total registered: %d", stats.TotalRegistered)
	t.Logf("  Current active: %d", stats.CurrentActive)
	t.Logf("  Total released: %d", stats.TotalReleased)
	t.Logf("  Invalid objects: %d", stats.InvalidObjects)
	t.Logf("  Cleanup runs: %d", stats.CleanupRuns)
	t.Logf("  Validation runs: %d", stats.ValidationRuns)

	// Verify cleanup occurred
	assert.Greater(t, stats.CleanupRuns, int64(0), "Cleanup should have run")
	assert.Greater(t, stats.ValidationRuns, int64(0), "Validation should have run")
	assert.LessOrEqual(t, int64(len(finalActiveObjects)), stats.CurrentActive)

	// Verify only valid objects remain active
	for _, activeObj := range finalActiveObjects {
		assert.True(t, activeObj.Valid, "Active object should be valid")
		assert.False(t, activeObj.Released, "Active object should not be released")
	}
}

// TestObjectLifecycleManagerStressTest performs stress testing
// This addresses requirement 3.2: ObjectLifecycleManager should handle high load
func TestObjectLifecycleManagerStressTest(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	config := ObjectLifecycleConfig{
		CleanupInterval:    200 * time.Millisecond,
		ObjectRetention:    500 * time.Millisecond,
		EnableValidation:   true,
		ValidationInterval: 300 * time.Millisecond,
		EnableStackTrace:   false, // Disable for performance
		LogObjectEvents:    false,
	}

	olm := NewObjectLifecycleManager(config)
	defer olm.Close()

	const numGoroutines = 25
	const objectsPerGoroutine = 100
	const testDuration = 3 * time.Second

	var wg sync.WaitGroup
	var totalRegistrations int64
	var totalUnregistrations int64
	var totalSafeAccesses int64
	var panicCount int64

	startTime := time.Now()

	// High-intensity concurrent operations
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()

			var localObjectIDs []uintptr

			for j := 0; j < objectsPerGoroutine && time.Since(startTime) < testDuration; j++ {
				// Recover from any panics
				func() {
					defer func() {
						if r := recover(); r != nil {
							atomic.AddInt64(&panicCount, 1)
						}
					}()

					// Create and register object
					testObj := &struct {
						data      string
						goroutine int
						index     int
					}{
						data:      "stress-test",
						goroutine: goroutineID,
						index:     j,
					}

					id, err := olm.RegisterObject(testObj, "stress-struct", "stress-test")
					if err == nil {
						atomic.AddInt64(&totalRegistrations, 1)
						localObjectIDs = append(localObjectIDs, id)

						// Perform safe access
						err = olm.SafeObjectAccess(id, func() error {
							atomic.AddInt64(&totalSafeAccesses, 1)
							// Simulate work
							testObj.data = "accessed"
							return nil
						})
					}

					// Randomly unregister some objects
					if len(localObjectIDs) > 5 && j%3 == 0 {
						idx := j % len(localObjectIDs)
						err := olm.UnregisterObject(localObjectIDs[idx])
						if err == nil {
							atomic.AddInt64(&totalUnregistrations, 1)
						}
						// Remove from local tracking
						localObjectIDs = append(localObjectIDs[:idx], localObjectIDs[idx+1:]...)
					}
				}()

				// Yield occasionally
				if j%10 == 0 {
					runtime.Gosched()
				}
			}

			// Cleanup remaining objects
			for _, id := range localObjectIDs {
				olm.UnregisterObject(id)
				atomic.AddInt64(&totalUnregistrations, 1)
			}
		}(i)
	}

	wg.Wait()

	// Wait for final cleanup
	time.Sleep(600 * time.Millisecond)

	// Collect final statistics
	stats := olm.GetStats()
	activeObjects := olm.ListActiveObjects()

	t.Logf("Stress test results:")
	t.Logf("  Test duration: %v", time.Since(startTime))
	t.Logf("  Total registrations: %d", totalRegistrations)
	t.Logf("  Total unregistrations: %d", totalUnregistrations)
	t.Logf("  Total safe accesses: %d", totalSafeAccesses)
	t.Logf("  Panics: %d", panicCount)
	t.Logf("  Final active objects: %d", len(activeObjects))
	t.Logf("  Manager stats - Total: %d, Active: %d, Released: %d",
		stats.TotalRegistered, stats.CurrentActive, stats.TotalReleased)

	// Verify stress test results
	assert.Equal(t, int64(0), panicCount, "Should not have any panics during stress test")
	assert.Greater(t, totalRegistrations, int64(0), "Should have registered objects")
	assert.Greater(t, totalSafeAccesses, int64(0), "Should have performed safe accesses")
	assert.Greater(t, stats.CleanupRuns, int64(0), "Cleanup should have run")

	// Verify manager is still functional
	testObj := &struct{ data string }{data: "post-stress-test"}
	id, err := olm.RegisterObject(testObj, "post-stress-struct", "post-stress-test")
	require.NoError(t, err)
	assert.True(t, olm.IsValidObject(id))
}

// BenchmarkObjectLifecycleManagerOperations benchmarks lifecycle operations
func BenchmarkObjectLifecycleManagerOperations(b *testing.B) {
	config := ObjectLifecycleConfig{
		CleanupInterval:  time.Minute, // Disable cleanup during benchmark
		ObjectRetention:  time.Hour,
		EnableValidation: false, // Disable for performance
		EnableStackTrace: false,
		LogObjectEvents:  false,
	}

	olm := NewObjectLifecycleManager(config)
	defer olm.Close()

	b.ResetTimer()

	b.Run("RegisterUnregister", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			testObj := &struct{ data int }{data: i}
			id, err := olm.RegisterObject(testObj, "benchmark-struct", "benchmark-test")
			if err != nil {
				b.Fatal(err)
			}
			err = olm.UnregisterObject(id)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("IsValidObject", func(b *testing.B) {
		// Pre-register some objects
		var ids []uintptr
		for i := 0; i < 1000; i++ {
			testObj := &struct{ data int }{data: i}
			id, _ := olm.RegisterObject(testObj, "benchmark-struct", "benchmark-test")
			ids = append(ids, id)
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			id := ids[i%len(ids)]
			olm.IsValidObject(id)
		}
	})

	b.Run("SafeObjectAccess", func(b *testing.B) {
		testObj := &struct{ data int }{data: 42}
		id, _ := olm.RegisterObject(testObj, "benchmark-struct", "benchmark-test")

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			olm.SafeObjectAccess(id, func() error {
				testObj.data = i
				return nil
			})
		}
	})
}
