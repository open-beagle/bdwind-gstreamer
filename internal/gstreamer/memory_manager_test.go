package gstreamer

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/go-gst/go-gst/gst"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMemoryManager_Creation(t *testing.T) {
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

func TestMemoryManager_DefaultConfig(t *testing.T) {
	// Test with nil config to verify defaults are used
	mm := NewMemoryManager(nil, nil)
	require.NotNil(t, mm)
	defer mm.Cleanup()

	// Verify default config values
	assert.Equal(t, 50, mm.config.BufferPoolSize)
	assert.Equal(t, 30*time.Second, mm.config.GCInterval)
	assert.True(t, mm.config.LeakDetectionEnabled)
	assert.Equal(t, 1000, mm.config.MaxObjectRefs)
	assert.Equal(t, 60*time.Second, mm.config.CleanupInterval)
}

func TestMemoryManager_RegisterObject(t *testing.T) {
	gst.Init(nil)

	config := DefaultMemoryManagerConfig()
	mm := NewMemoryManager(config, nil)
	require.NotNil(t, mm)
	defer mm.Cleanup()

	// Test registering a GStreamer pipeline
	pipeline, err := gst.NewPipeline("test-pipeline")
	require.NoError(t, err)
	require.NotNil(t, pipeline)
	defer pipeline.Unref()

	// Register the object
	ref := mm.RegisterObject(pipeline)
	require.NotNil(t, ref)
	assert.NotEqual(t, uint64(0), ref.ID)
	assert.Equal(t, pipeline, ref.Object)
	assert.Contains(t, ref.ObjectType, "Pipeline")
	assert.Equal(t, int32(1), ref.RefCount)
	assert.False(t, ref.IsGCSafe)

	// Check statistics
	stats := mm.GetMemoryStats()
	assert.Equal(t, int64(1), stats.ObjectRefsCount)

	// Test registering nil object
	nilRef := mm.RegisterObject(nil)
	assert.Nil(t, nilRef)
}

func TestMemoryManager_UnregisterObject(t *testing.T) {
	gst.Init(nil)

	config := DefaultMemoryManagerConfig()
	mm := NewMemoryManager(config, nil)
	require.NotNil(t, mm)
	defer mm.Cleanup()

	// Register an element
	element, err := gst.NewElement("fakesrc")
	require.NoError(t, err)
	require.NotNil(t, element)
	defer element.Unref()

	ref := mm.RegisterObject(element)
	require.NotNil(t, ref)

	// Verify registration
	stats := mm.GetMemoryStats()
	assert.Equal(t, int64(1), stats.ObjectRefsCount)

	// Unregister the object
	err = mm.UnregisterObject(ref)
	require.NoError(t, err)

	// Check statistics
	stats = mm.GetMemoryStats()
	assert.Equal(t, int64(0), stats.ObjectRefsCount)

	// Test unregistering nil reference
	err = mm.UnregisterObject(nil)
	assert.Error(t, err)

	// Test unregistering non-existent reference
	fakeRef := &ObjectRef{ID: 99999}
	err = mm.UnregisterObject(fakeRef)
	assert.Error(t, err)
}

func TestMemoryManager_ObjectFinalizer(t *testing.T) {
	gst.Init(nil)

	config := DefaultMemoryManagerConfig()
	mm := NewMemoryManager(config, nil)
	require.NotNil(t, mm)
	defer mm.Cleanup()

	// Register an object
	element, err := gst.NewElement("identity")
	require.NoError(t, err)
	require.NotNil(t, element)
	defer element.Unref()

	ref := mm.RegisterObject(element)
	require.NotNil(t, ref)

	// Set a finalizer
	finalizerCalled := false
	finalizer := func() {
		finalizerCalled = true
	}
	mm.SetObjectFinalizer(ref, finalizer)

	// Unregister the object - should call finalizer
	err = mm.UnregisterObject(ref)
	require.NoError(t, err)
	assert.True(t, finalizerCalled)
}

func TestMemoryManager_GCSafety(t *testing.T) {
	gst.Init(nil)

	config := DefaultMemoryManagerConfig()
	mm := NewMemoryManager(config, nil)
	require.NotNil(t, mm)
	defer mm.Cleanup()

	// Create an object to keep GC safe
	element, err := gst.NewElement("fakesink")
	require.NoError(t, err)
	require.NotNil(t, element)
	defer element.Unref()

	// Initially no GC safe objects
	stats := mm.GetMemoryStats()
	assert.Equal(t, int64(0), stats.GCSafeObjectsCount)

	// Keep object GC safe
	mm.KeepGCSafe(element)

	// Check statistics
	stats = mm.GetMemoryStats()
	assert.Equal(t, int64(1), stats.GCSafeObjectsCount)

	// Release from GC safety
	mm.ReleaseGCSafe(element)

	// Check statistics
	stats = mm.GetMemoryStats()
	assert.Equal(t, int64(0), stats.GCSafeObjectsCount)

	// Test with nil object
	mm.KeepGCSafe(nil)
	mm.ReleaseGCSafe(nil)
	stats = mm.GetMemoryStats()
	assert.Equal(t, int64(0), stats.GCSafeObjectsCount)
}

func TestMemoryManager_BufferPool(t *testing.T) {
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

	// Get the same pool again - should return existing
	pool2 := mm.GetBufferPool(poolName, bufferSize)
	assert.Equal(t, pool, pool2)

	// Statistics should remain the same
	stats = mm.GetMemoryStats()
	assert.Equal(t, int64(1), stats.BufferPoolsCount)

	// Release the buffer pool
	err := mm.ReleaseBufferPool(poolName)
	require.NoError(t, err)

	// Check statistics
	stats = mm.GetMemoryStats()
	assert.Equal(t, int64(0), stats.BufferPoolsCount)

	// Test releasing non-existent pool
	err = mm.ReleaseBufferPool("non-existent")
	assert.Error(t, err)
}

func TestBufferPool_Operations(t *testing.T) {
	poolName := "test-pool"
	bufferSize := 512
	maxBuffers := 10

	pool := NewBufferPool(poolName, bufferSize, maxBuffers)
	require.NotNil(t, pool)

	// Get a buffer
	buffer1 := pool.Get()
	require.NotNil(t, buffer1)
	assert.Equal(t, bufferSize, len(buffer1))

	// Get another buffer
	buffer2 := pool.Get()
	require.NotNil(t, buffer2)
	assert.Equal(t, bufferSize, len(buffer2))

	// Put buffer back
	pool.Put(buffer1)

	// Get buffer again - should reuse
	buffer3 := pool.Get()
	require.NotNil(t, buffer3)
	assert.Equal(t, bufferSize, len(buffer3))

	// Check statistics
	stats := pool.GetStats()
	assert.Equal(t, poolName, stats["name"])
	assert.Equal(t, bufferSize, stats["buffer_size"])
	assert.Equal(t, maxBuffers, stats["max_buffers"])
	assert.Greater(t, stats["created"].(int64), int64(0))

	// Test putting wrong size buffer
	wrongSizeBuffer := make([]byte, 256)
	pool.Put(wrongSizeBuffer) // Should be ignored

	// Test putting buffer when pool is full
	for i := 0; i < maxBuffers+5; i++ {
		buffer := pool.Get()
		pool.Put(buffer)
	}
}

func TestMemoryManager_MemoryLeakDetection(t *testing.T) {
	gst.Init(nil)

	config := DefaultMemoryManagerConfig()
	config.LeakDetectionEnabled = true
	mm := NewMemoryManager(config, nil)
	require.NotNil(t, mm)
	defer mm.Cleanup()

	// Create objects that might leak
	element1, err := gst.NewElement("fakesrc")
	require.NoError(t, err)
	defer element1.Unref()

	element2, err := gst.NewElement("fakesink")
	require.NoError(t, err)
	defer element2.Unref()

	// Register objects
	ref1 := mm.RegisterObject(element1)
	ref2 := mm.RegisterObject(element2)

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
		assert.Contains(t, leak.ObjectType, "Element")
	}

	// Test with leak detection disabled
	config.LeakDetectionEnabled = false
	mm2 := NewMemoryManager(config, nil)
	defer mm2.Cleanup()

	leaks = mm2.CheckMemoryLeaks()
	assert.Nil(t, leaks)
}

func TestMemoryManager_ForceGC(t *testing.T) {
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

func TestMemoryManager_ObjectAccessTracking(t *testing.T) {
	gst.Init(nil)

	config := DefaultMemoryManagerConfig()
	mm := NewMemoryManager(config, nil)
	require.NotNil(t, mm)
	defer mm.Cleanup()

	// Register an object
	element, err := gst.NewElement("identity")
	require.NoError(t, err)
	defer element.Unref()

	ref := mm.RegisterObject(element)
	require.NotNil(t, ref)

	initialAccess := ref.LastAccess

	// Wait a bit to ensure time difference
	time.Sleep(10 * time.Millisecond)

	// Update object access
	mm.UpdateObjectAccess(ref)

	// Get the updated reference
	updatedRef := mm.GetObjectRef(ref.ID)
	require.NotNil(t, updatedRef)
	assert.True(t, updatedRef.LastAccess.After(initialAccess))

	// Test with nil reference
	mm.UpdateObjectAccess(nil) // Should not panic

	// Test with non-existent reference
	fakeRef := &ObjectRef{ID: 99999}
	mm.UpdateObjectAccess(fakeRef) // Should not panic
}

func TestMemoryManager_ConcurrentAccess(t *testing.T) {
	gst.Init(nil)

	config := DefaultMemoryManagerConfig()
	mm := NewMemoryManager(config, nil)
	require.NotNil(t, mm)
	defer mm.Cleanup()

	const numGoroutines = 10
	const numOperations = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Run concurrent operations
	for i := 0; i < numGoroutines; i++ {
		go func(goroutineID int) {
			defer wg.Done()

			for j := 0; j < numOperations; j++ {
				// Create and register objects
				element, err := gst.NewElement("identity")
				if err != nil {
					continue
				}

				ref := mm.RegisterObject(element)
				if ref != nil {
					// Update access
					mm.UpdateObjectAccess(ref)

					// Keep some objects GC safe
					if j%3 == 0 {
						mm.KeepGCSafe(element)
						mm.ReleaseGCSafe(element)
					}

					// Get buffer pool
					poolName := fmt.Sprintf("pool-%d-%d", goroutineID, j)
					pool := mm.GetBufferPool(poolName, 1024)
					if pool != nil {
						buffer := pool.Get()
						pool.Put(buffer)
						mm.ReleaseBufferPool(poolName)
					}

					// Unregister object
					mm.UnregisterObject(ref)
				}

				element.Unref()
			}
		}(i)
	}

	wg.Wait()

	// Check final statistics
	stats := mm.GetMemoryStats()
	assert.Equal(t, int64(0), stats.ObjectRefsCount)
	assert.Equal(t, int64(0), stats.BufferPoolsCount)
	assert.Equal(t, int64(0), stats.GCSafeObjectsCount)
}

func TestMemoryManager_BackgroundTasks(t *testing.T) {
	config := DefaultMemoryManagerConfig()
	config.GCInterval = 50 * time.Millisecond
	config.CleanupInterval = 50 * time.Millisecond
	config.LeakDetectionEnabled = true

	mm := NewMemoryManager(config, nil)
	require.NotNil(t, mm)

	// Let background tasks run for a bit
	time.Sleep(200 * time.Millisecond)

	// Check that background tasks have run
	stats := mm.GetMemoryStats()
	assert.Greater(t, stats.GCCount, int64(0))

	// Cleanup should stop background tasks
	mm.Cleanup()

	// Verify cleanup completed
	assert.False(t, mm.running)
}

func TestMemoryManager_MemoryUsage(t *testing.T) {
	config := DefaultMemoryManagerConfig()
	mm := NewMemoryManager(config, nil)
	require.NotNil(t, mm)
	defer mm.Cleanup()

	// Get memory usage
	usage := mm.GetMemoryUsage()
	assert.Greater(t, usage, int64(0))

	// Memory usage should be included in stats
	stats := mm.GetMemoryStats()
	assert.Equal(t, usage, stats.MemoryUsage)
}

func TestMemoryManager_ObjectCount(t *testing.T) {
	gst.Init(nil)

	config := DefaultMemoryManagerConfig()
	mm := NewMemoryManager(config, nil)
	require.NotNil(t, mm)
	defer mm.Cleanup()

	// Initially no objects
	assert.Equal(t, int64(0), mm.GetObjectCount())

	// Register some objects
	element1, err := gst.NewElement("fakesrc")
	require.NoError(t, err)
	defer element1.Unref()

	element2, err := gst.NewElement("fakesink")
	require.NoError(t, err)
	defer element2.Unref()

	ref1 := mm.RegisterObject(element1)
	ref2 := mm.RegisterObject(element2)

	assert.Equal(t, int64(2), mm.GetObjectCount())

	// Unregister one
	mm.UnregisterObject(ref1)
	assert.Equal(t, int64(1), mm.GetObjectCount())

	// Unregister the other
	mm.UnregisterObject(ref2)
	assert.Equal(t, int64(0), mm.GetObjectCount())
}

func TestMemoryManager_StaleObjectCleanup(t *testing.T) {
	gst.Init(nil)

	config := DefaultMemoryManagerConfig()
	config.CleanupInterval = 50 * time.Millisecond
	mm := NewMemoryManager(config, nil)
	require.NotNil(t, mm)
	defer mm.Cleanup()

	// Register an object
	element, err := gst.NewElement("identity")
	require.NoError(t, err)
	defer element.Unref()

	ref := mm.RegisterObject(element)
	require.NotNil(t, ref)

	// Manually set ref count to 0 and old access time to simulate stale object
	ref.RefCount = 0
	ref.LastAccess = time.Now().Add(-15 * time.Minute)

	// Wait for cleanup to run
	time.Sleep(150 * time.Millisecond)

	// Object should be cleaned up
	cleanedRef := mm.GetObjectRef(ref.ID)
	assert.Nil(t, cleanedRef)
}

// Benchmark tests for performance validation

func BenchmarkMemoryManager_RegisterUnregister(b *testing.B) {
	gst.Init(nil)

	config := DefaultMemoryManagerConfig()
	mm := NewMemoryManager(config, nil)
	defer mm.Cleanup()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		element, err := gst.NewElement("identity")
		if err != nil {
			b.Fatal(err)
		}

		ref := mm.RegisterObject(element)
		if ref == nil {
			b.Fatal("failed to register object")
		}

		err = mm.UnregisterObject(ref)
		if err != nil {
			b.Fatal(err)
		}

		element.Unref()
	}
}

func BenchmarkBufferPool_GetPut(b *testing.B) {
	pool := NewBufferPool("benchmark-pool", 1024, 100)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		buffer := pool.Get()
		pool.Put(buffer)
	}
}

func BenchmarkMemoryManager_GCSafety(b *testing.B) {
	gst.Init(nil)

	config := DefaultMemoryManagerConfig()
	mm := NewMemoryManager(config, nil)
	defer mm.Cleanup()

	elements := make([]*gst.Element, 100)
	for i := range elements {
		element, err := gst.NewElement("identity")
		if err != nil {
			b.Fatal(err)
		}
		elements[i] = element
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		element := elements[i%len(elements)]
		mm.KeepGCSafe(element)
		mm.ReleaseGCSafe(element)
	}

	b.StopTimer()

	for _, element := range elements {
		element.Unref()
	}
}

// Helper function to create test objects
func createTestElement(t *testing.T, elementType string) *gst.Element {
	gst.Init(nil)
	element, err := gst.NewElement(elementType)
	require.NoError(t, err)
	require.NotNil(t, element)
	return element
}

// Test memory manager integration with real GStreamer objects
func TestMemoryManager_GStreamerIntegration(t *testing.T) {
	gst.Init(nil)

	config := DefaultMemoryManagerConfig()
	mm := NewMemoryManager(config, nil)
	require.NotNil(t, mm)
	defer mm.Cleanup()

	// Create a simple pipeline
	pipeline, err := gst.NewPipeline("test-pipeline")
	require.NoError(t, err)
	defer pipeline.Unref()

	src := createTestElement(t, "fakesrc")
	defer src.Unref()

	sink := createTestElement(t, "fakesink")
	defer sink.Unref()

	// Register all objects
	pipelineRef := mm.RegisterObject(pipeline)
	srcRef := mm.RegisterObject(src)
	sinkRef := mm.RegisterObject(sink)

	require.NotNil(t, pipelineRef)
	require.NotNil(t, srcRef)
	require.NotNil(t, sinkRef)

	// Keep objects GC safe during pipeline operations
	mm.KeepGCSafe(pipeline)
	mm.KeepGCSafe(src)
	mm.KeepGCSafe(sink)

	// Add elements to pipeline
	pipeline.AddMany(src, sink)
	src.Link(sink)

	// Set pipeline to playing state
	pipeline.SetState(gst.StatePlaying)

	// Wait a bit
	time.Sleep(100 * time.Millisecond)

	// Stop pipeline
	pipeline.SetState(gst.StateNull)

	// Release GC safety
	mm.ReleaseGCSafe(pipeline)
	mm.ReleaseGCSafe(src)
	mm.ReleaseGCSafe(sink)

	// Unregister objects
	err = mm.UnregisterObject(pipelineRef)
	assert.NoError(t, err)

	err = mm.UnregisterObject(srcRef)
	assert.NoError(t, err)

	err = mm.UnregisterObject(sinkRef)
	assert.NoError(t, err)

	// Check final statistics
	stats := mm.GetMemoryStats()
	assert.Equal(t, int64(0), stats.ObjectRefsCount)
	assert.Equal(t, int64(0), stats.GCSafeObjectsCount)
}
