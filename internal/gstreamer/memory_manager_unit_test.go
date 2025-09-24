package gstreamer

import (
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMemoryManagerUnit tests the memory manager in isolation
func TestMemoryManagerUnit(t *testing.T) {
	config := DefaultMemoryManagerConfig()
	logger := logrus.NewEntry(logrus.StandardLogger())

	mm := NewMemoryManager(config, logger)
	require.NotNil(t, mm)

	defer mm.Cleanup()

	// Check initial state
	stats := mm.GetMemoryStats()
	assert.Equal(t, int64(0), stats.ObjectRefsCount)
	assert.Equal(t, int64(0), stats.BufferPoolsCount)
	assert.Equal(t, int64(0), stats.GCSafeObjectsCount)
	assert.NotNil(t, stats.BufferPoolStats)
}

func TestMemoryManagerObjectLifecycle(t *testing.T) {
	config := DefaultMemoryManagerConfig()
	mm := NewMemoryManager(config, nil)
	require.NotNil(t, mm)
	defer mm.Cleanup()

	// Create a test object
	testObj := "test-object"

	// Register the object
	ref := mm.RegisterObject(testObj)
	require.NotNil(t, ref)
	assert.NotEqual(t, uint64(0), ref.ID)
	assert.Equal(t, testObj, ref.Object)
	assert.Equal(t, int32(1), ref.RefCount)

	// Check statistics
	stats := mm.GetMemoryStats()
	assert.Equal(t, int64(1), stats.ObjectRefsCount)

	// Unregister the object
	err := mm.UnregisterObject(ref)
	require.NoError(t, err)

	// Check statistics
	stats = mm.GetMemoryStats()
	assert.Equal(t, int64(0), stats.ObjectRefsCount)
}

func TestMemoryManagerBufferPool(t *testing.T) {
	config := DefaultMemoryManagerConfig()
	mm := NewMemoryManager(config, nil)
	require.NotNil(t, mm)
	defer mm.Cleanup()

	// Get a buffer pool
	poolName := "test-pool"
	bufferSize := 1024
	pool := mm.GetBufferPool(poolName, bufferSize)
	require.NotNil(t, pool)
	assert.Equal(t, poolName, pool.Name)
	assert.Equal(t, bufferSize, pool.BufferSize)

	// Check statistics
	stats := mm.GetMemoryStats()
	assert.Equal(t, int64(1), stats.BufferPoolsCount)
	assert.Contains(t, stats.BufferPoolStats, poolName)

	// Test buffer operations
	buffer := pool.Get()
	require.NotNil(t, buffer)
	assert.Equal(t, bufferSize, len(buffer))

	pool.Put(buffer)

	// Release the buffer pool
	err := mm.ReleaseBufferPool(poolName)
	require.NoError(t, err)

	// Check statistics
	stats = mm.GetMemoryStats()
	assert.Equal(t, int64(0), stats.BufferPoolsCount)
}

func TestMemoryManagerGCSafety(t *testing.T) {
	config := DefaultMemoryManagerConfig()
	mm := NewMemoryManager(config, nil)
	require.NotNil(t, mm)
	defer mm.Cleanup()

	// Create an object to keep GC safe
	testObj := "gc-safe-object"

	// Initially no GC safe objects
	stats := mm.GetMemoryStats()
	assert.Equal(t, int64(0), stats.GCSafeObjectsCount)

	// Keep object GC safe
	mm.KeepGCSafe(testObj)

	// Check statistics
	stats = mm.GetMemoryStats()
	assert.Equal(t, int64(1), stats.GCSafeObjectsCount)

	// Release from GC safety
	mm.ReleaseGCSafe(testObj)

	// Check statistics
	stats = mm.GetMemoryStats()
	assert.Equal(t, int64(0), stats.GCSafeObjectsCount)
}

func TestMemoryManagerLeakDetection(t *testing.T) {
	config := DefaultMemoryManagerConfig()
	config.LeakDetectionEnabled = true
	mm := NewMemoryManager(config, nil)
	require.NotNil(t, mm)
	defer mm.Cleanup()

	// Create objects that might leak
	obj1 := "leak-object-1"
	obj2 := "leak-object-2"

	// Register objects
	ref1 := mm.RegisterObject(obj1)
	ref2 := mm.RegisterObject(obj2)

	// Simulate high reference count (potential leak)
	ref1.RefCount = 5
	ref2.RefCount = 3

	// Simulate old objects
	ref1.CreatedAt = time.Now().Add(-10 * time.Minute)
	ref1.LastAccess = time.Now().Add(-10 * time.Minute)
	ref2.CreatedAt = time.Now().Add(-8 * time.Minute)
	ref2.LastAccess = time.Now().Add(-8 * time.Minute)

	// Check for memory leaks
	leaks := mm.CheckMemoryLeaks()
	assert.Len(t, leaks, 2)

	for _, leak := range leaks {
		assert.Greater(t, leak.RefCount, int32(1))
		assert.Greater(t, leak.Age, 5*time.Minute)
		assert.Contains(t, leak.ObjectType, "string")
	}
}

func TestMemoryManagerForceGC(t *testing.T) {
	config := DefaultMemoryManagerConfig()
	mm := NewMemoryManager(config, nil)
	require.NotNil(t, mm)
	defer mm.Cleanup()

	// Get initial GC count
	initialStats := mm.GetMemoryStats()
	initialGCCount := initialStats.GCCount

	// Force GC
	mm.ForceGC()

	// Check that GC was called
	stats := mm.GetMemoryStats()
	assert.Greater(t, stats.GCCount, initialGCCount)
	assert.True(t, stats.LastGC.After(initialStats.LastGC))
}

func TestMemoryManagerObjectCount(t *testing.T) {
	config := DefaultMemoryManagerConfig()
	mm := NewMemoryManager(config, nil)
	require.NotNil(t, mm)
	defer mm.Cleanup()

	// Initially no objects
	assert.Equal(t, int64(0), mm.GetObjectCount())

	// Register some objects
	obj1 := "object-1"
	obj2 := "object-2"

	ref1 := mm.RegisterObject(obj1)
	ref2 := mm.RegisterObject(obj2)

	assert.Equal(t, int64(2), mm.GetObjectCount())

	// Unregister one
	mm.UnregisterObject(ref1)
	assert.Equal(t, int64(1), mm.GetObjectCount())

	// Unregister the other
	mm.UnregisterObject(ref2)
	assert.Equal(t, int64(0), mm.GetObjectCount())
}

func TestMemoryManagerFinalizer(t *testing.T) {
	config := DefaultMemoryManagerConfig()
	mm := NewMemoryManager(config, nil)
	require.NotNil(t, mm)
	defer mm.Cleanup()

	// Register an object
	testObj := "finalizer-object"
	ref := mm.RegisterObject(testObj)
	require.NotNil(t, ref)

	// Set a finalizer
	finalizerCalled := false
	finalizer := func() {
		finalizerCalled = true
	}
	mm.SetObjectFinalizer(ref, finalizer)

	// Unregister the object - should call finalizer
	err := mm.UnregisterObject(ref)
	require.NoError(t, err)
	assert.True(t, finalizerCalled)
}

// Benchmark tests for performance validation
func BenchmarkMemoryManagerRegisterUnregister(b *testing.B) {
	config := DefaultMemoryManagerConfig()
	mm := NewMemoryManager(config, nil)
	defer mm.Cleanup()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		obj := "benchmark-object"
		ref := mm.RegisterObject(obj)
		if ref == nil {
			b.Fatal("failed to register object")
		}

		err := mm.UnregisterObject(ref)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBufferPoolGetPut(b *testing.B) {
	config := DefaultMemoryManagerConfig()
	mm := NewMemoryManager(config, nil)
	defer mm.Cleanup()

	pool := mm.GetBufferPool("benchmark-pool", 1024)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		buffer := pool.Get()
		pool.Put(buffer)
	}
}

func BenchmarkMemoryManagerGCSafety(b *testing.B) {
	config := DefaultMemoryManagerConfig()
	mm := NewMemoryManager(config, nil)
	defer mm.Cleanup()

	objects := make([]string, 100)
	for i := range objects {
		objects[i] = "gc-safe-object"
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		obj := objects[i%len(objects)]
		mm.KeepGCSafe(obj)
		mm.ReleaseGCSafe(obj)
	}
}
