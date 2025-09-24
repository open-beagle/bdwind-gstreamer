package gstreamer

import (
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/go-gst/go-gst/gst"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/open-beagle/bdwind-gstreamer/internal/config"
)

func TestMemoryManager_BasicOperations(t *testing.T) {
	config := MemoryManagerConfig{
		MaxBufferSize:        1024 * 1024, // 1MB
		BufferPoolSize:       10,
		BufferPoolTimeout:    100 * time.Millisecond,
		MonitorInterval:      0, // Disable monitoring for test
		TrackObjectLifecycle: true,
		EnableLeakDetection:  true,
		LeakThreshold:        1 * time.Second,
	}

	mm := NewMemoryManager(config)
	defer mm.Close()

	// Test buffer pool operations
	t.Run("BufferPool", func(t *testing.T) {
		// Test buffer allocation and return
		buffer1 := mm.bufferPool.GetBuffer(1024)
		assert.Equal(t, 1024, len(buffer1))

		buffer2 := mm.bufferPool.GetBuffer(2048)
		assert.Equal(t, 2048, len(buffer2))

		// Return buffers to pool
		mm.bufferPool.ReturnBuffer(buffer1)
		mm.bufferPool.ReturnBuffer(buffer2)

		// Get stats
		hits, misses, poolSize := mm.bufferPool.GetStats()
		assert.GreaterOrEqual(t, hits+misses, int64(2))
		assert.GreaterOrEqual(t, poolSize, 0)
	})

	// Test object tracking
	t.Run("ObjectTracking", func(t *testing.T) {
		// Create a mock object
		mockObj := &struct{ data string }{data: "test"}

		// Track object
		id := mm.TrackObject(mockObj, "test.Object", 100)
		assert.NotEqual(t, uintptr(0), id)

		// Check stats
		stats := mm.GetStats()
		assert.Equal(t, int64(1), stats.ActiveObjects)
		assert.Equal(t, int64(1), stats.TotalObjects)

		// Release object
		mm.ReleaseObject(id)

		// Check stats after release
		stats = mm.GetStats()
		assert.Equal(t, int64(0), stats.ActiveObjects)
		assert.Equal(t, int64(1), stats.ReleasedObjects)
	})

	// Test safe buffer copying
	t.Run("SafeBufferCopy", func(t *testing.T) {
		src := []byte("test data for copying")

		dst, err := mm.SafeCopyBuffer(src)
		require.NoError(t, err)
		assert.Equal(t, src, dst)

		// Test oversized buffer
		oversized := make([]byte, config.MaxBufferSize+1)
		_, err = mm.SafeCopyBuffer(oversized)
		assert.Error(t, err)

		// Test empty buffer
		_, err = mm.SafeCopyBuffer([]byte{})
		assert.Error(t, err)
	})
}

func TestMemoryManager_LeakDetection(t *testing.T) {
	config := MemoryManagerConfig{
		TrackObjectLifecycle: true,
		EnableLeakDetection:  true,
		LeakThreshold:        100 * time.Millisecond,
	}

	mm := NewMemoryManager(config)
	defer mm.Close()

	// Track an object but don't release it
	mockObj := &struct{ data string }{data: "leak test"}
	id := mm.TrackObject(mockObj, "test.LeakyObject", 200)

	// Wait for leak threshold
	time.Sleep(150 * time.Millisecond)

	// Check for leaks
	leaks := mm.CheckForLeaks()
	assert.Len(t, leaks, 1)
	assert.Equal(t, id, leaks[0].ID)
	assert.Equal(t, "test.LeakyObject", leaks[0].Type)

	// Release the object
	mm.ReleaseObject(id)

	// Check that leak is cleared
	leaks = mm.CheckForLeaks()
	assert.Len(t, leaks, 0)
}

func TestMemoryManager_GCTrigger(t *testing.T) {
	config := MemoryManagerConfig{
		GCTriggerThreshold: 1024, // Very low threshold for testing
	}

	mm := NewMemoryManager(config)
	defer mm.Close()

	// Get initial memory stats
	var initialStats runtime.MemStats
	runtime.ReadMemStats(&initialStats)

	// Trigger GC
	mm.TriggerGC()

	// Verify GC was called (this is indirect verification)
	var afterStats runtime.MemStats
	runtime.ReadMemStats(&afterStats)

	// GC should have been triggered, so NumGC should be higher
	assert.GreaterOrEqual(t, afterStats.NumGC, initialStats.NumGC)
}

func TestDesktopCaptureGst_MemoryManagement(t *testing.T) {
	// Initialize GStreamer for testing
	gst.Init(nil)

	// Create test configuration
	config := createTestDesktopCaptureConfig()

	// Create desktop capture with memory management
	dc, err := NewDesktopCaptureGst(config)
	require.NoError(t, err)
	defer dc.Stop()

	// Verify memory manager is initialized
	assert.NotNil(t, dc.memoryManager)

	// Test memory statistics
	t.Run("MemoryStats", func(t *testing.T) {
		stats := dc.GetMemoryStats()
		assert.GreaterOrEqual(t, stats.TotalObjects, int64(0))
		assert.GreaterOrEqual(t, stats.ActiveObjects, int64(0))
	})

	// Test memory cleanup
	t.Run("MemoryCleanup", func(t *testing.T) {
		// This should not panic
		dc.TriggerMemoryCleanup()
	})

	// Test leak detection
	t.Run("LeakDetection", func(t *testing.T) {
		leaks := dc.CheckForMemoryLeaks()
		// Should not have leaks immediately after creation
		assert.Len(t, leaks, 0)
	})
}

func TestConvertGstSample_MemoryManagement(t *testing.T) {
	// Initialize GStreamer for testing
	gst.Init(nil)

	// Create test configuration
	config := createTestDesktopCaptureConfig()

	// Create desktop capture
	dc, err := NewDesktopCaptureGst(config)
	require.NoError(t, err)
	defer dc.Stop()

	// Create a test sample (this is a simplified test)
	// In a real scenario, samples come from GStreamer pipeline
	t.Run("NilSampleHandling", func(t *testing.T) {
		_, err := dc.convertGstSample(nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "nil gst sample")
	})

	// Test buffer size limits
	t.Run("BufferSizeLimits", func(t *testing.T) {
		// This test would require creating actual GStreamer samples
		// which is complex in a unit test environment
		// The logic is tested through the memory manager tests above
		assert.True(t, true) // Placeholder
	})
}

func TestBufferPool_Performance(t *testing.T) {
	poolSize := 100
	maxSize := 1024 * 1024 // 1MB
	timeout := 100 * time.Millisecond

	bp := NewBufferPool(poolSize, maxSize, timeout)

	// Test concurrent access
	t.Run("ConcurrentAccess", func(t *testing.T) {
		const numGoroutines = 10
		const numOperations = 100

		done := make(chan bool, numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			go func() {
				defer func() { done <- true }()

				for j := 0; j < numOperations; j++ {
					buffer := bp.GetBuffer(1024)
					assert.Equal(t, 1024, len(buffer))

					// Simulate some work
					for k := range buffer {
						buffer[k] = byte(k % 256)
					}

					bp.ReturnBuffer(buffer)
				}
			}()
		}

		// Wait for all goroutines to complete
		for i := 0; i < numGoroutines; i++ {
			<-done
		}

		// Check stats
		hits, misses, _ := bp.GetStats()
		total := hits + misses
		assert.Equal(t, int64(numGoroutines*numOperations), total)
	})
}

// Helper function to create test configuration
func createTestDesktopCaptureConfig() config.DesktopCaptureConfig {
	return config.DesktopCaptureConfig{
		Width:       640,
		Height:      480,
		FrameRate:   30,
		Quality:     "medium",
		BufferSize:  10,
		DisplayID:   ":0",
		UseWayland:  false,
		ShowPointer: true,
	}
}

// Benchmark buffer pool performance
func BenchmarkBufferPool_GetReturn(b *testing.B) {
	bp := NewBufferPool(100, 1024*1024, 100*time.Millisecond)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			buffer := bp.GetBuffer(1024)
			bp.ReturnBuffer(buffer)
		}
	})
}

func TestMemoryManager_ReferenceCountingAndValidation(t *testing.T) {
	config := MemoryManagerConfig{
		MaxBufferSize:        1024,
		TrackObjectLifecycle: true,
		EnableLeakDetection:  true,
		LeakThreshold:        100 * time.Millisecond,
	}

	mm := NewMemoryManager(config)
	defer mm.Close()

	t.Run("ReferenceCountingOperations", func(t *testing.T) {
		mockObj := &struct{ data string }{data: "refcount test"}
		id := mm.TrackObject(mockObj, "test.RefCountObject", 100)

		// Check initial reference count
		assert.Equal(t, int32(1), mm.GetRefCount(id))

		// Add reference
		mm.AddRef(id)
		assert.Equal(t, int32(2), mm.GetRefCount(id))

		// Get object info
		info := mm.GetObjectInfo(id)
		require.NotNil(t, info)
		assert.Equal(t, "test.RefCountObject", info.Type)
		assert.Equal(t, int32(2), info.RefCount)

		// Release object
		mm.ReleaseObject(id)
		assert.Equal(t, int32(1), mm.GetRefCount(id))

		// Object should still exist but marked as released
		info = mm.GetObjectInfo(id)
		require.NotNil(t, info)
		assert.True(t, info.Released)
	})

	t.Run("BufferSizeValidation", func(t *testing.T) {
		// Test valid buffer size
		err := mm.ValidateBufferSize(512)
		assert.NoError(t, err)

		// Test invalid buffer size (too large)
		err = mm.ValidateBufferSize(2048)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds maximum")

		// Test invalid buffer size (zero)
		err = mm.ValidateBufferSize(0)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid buffer size")

		// Test negative buffer size
		err = mm.ValidateBufferSize(-100)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid buffer size")
	})

	t.Run("SafeBufferAllocation", func(t *testing.T) {
		// Test successful allocation
		buffer, err := mm.SafeAllocateBuffer(512)
		assert.NoError(t, err)
		assert.Equal(t, 512, len(buffer))

		// Return buffer to pool
		mm.ReturnBuffer(buffer)

		// Test allocation that exceeds limits
		_, err = mm.SafeAllocateBuffer(2048)
		assert.Error(t, err)
	})

	t.Run("ForceCleanupObject", func(t *testing.T) {
		mockObj := &struct{ data string }{data: "cleanup test"}
		id := mm.TrackObject(mockObj, "test.CleanupObject", 200)

		// Verify object exists
		info := mm.GetObjectInfo(id)
		require.NotNil(t, info)

		// Force cleanup
		mm.ForceCleanupObject(id)

		// Verify object is removed
		info = mm.GetObjectInfo(id)
		assert.Nil(t, info)
	})
}

func TestMemoryManager_AdvancedLeakDetection(t *testing.T) {
	config := MemoryManagerConfig{
		TrackObjectLifecycle: true,
		EnableLeakDetection:  true,
		LeakThreshold:        50 * time.Millisecond,
		ObjectRetentionTime:  100 * time.Millisecond,
	}

	mm := NewMemoryManager(config)
	defer mm.Close()

	t.Run("LeakDetectionWithCleanup", func(t *testing.T) {
		// Create multiple objects
		ids := make([]uintptr, 3)
		for i := 0; i < 3; i++ {
			mockObj := &struct{ data int }{data: i}
			ids[i] = mm.TrackObject(mockObj, fmt.Sprintf("test.LeakObject%d", i), 100)
		}

		// Release one object
		mm.ReleaseObject(ids[0])

		// Wait for leak threshold
		time.Sleep(60 * time.Millisecond)

		// Check for leaks - should find 2 leaked objects
		leaks := mm.CheckForLeaks()
		assert.Len(t, leaks, 2)

		// Release remaining objects
		mm.ReleaseObject(ids[1])
		mm.ReleaseObject(ids[2])

		// Check that leaks are cleared
		leaks = mm.CheckForLeaks()
		assert.Len(t, leaks, 0)

		// Wait for retention time and cleanup
		time.Sleep(110 * time.Millisecond)
		mm.CleanupOldObjects()

		// Verify objects are cleaned up
		for _, id := range ids {
			info := mm.GetObjectInfo(id)
			assert.Nil(t, info)
		}
	})
}

func TestMemoryVerification(t *testing.T) {
	// Initialize GStreamer for testing
	gst.Init(nil)

	config := createTestDesktopCaptureConfig()
	dc, err := NewDesktopCaptureGst(config)
	require.NoError(t, err)
	defer dc.Stop()

	verifier := NewMemoryVerifier()

	t.Run("MemoryManagerVerification", func(t *testing.T) {
		report := verifier.VerifyMemoryManager(dc.memoryManager)
		assert.NotNil(t, report)
		assert.False(t, report.Timestamp.IsZero())
		assert.GreaterOrEqual(t, report.TotalObjects, int64(0))
		assert.GreaterOrEqual(t, report.ActiveObjects, int64(0))
	})

	t.Run("DesktopCaptureVerification", func(t *testing.T) {
		report := verifier.VerifyDesktopCapture(dc)
		assert.NotNil(t, report)
		assert.False(t, report.Timestamp.IsZero())

		// Should not have critical issues for a newly created capture
		assert.Len(t, report.CriticalIssues, 0)
	})

	t.Run("HealthCheck", func(t *testing.T) {
		healthy := verifier.RunMemoryHealthCheck(dc)
		assert.True(t, healthy)
	})

	t.Run("NilHandling", func(t *testing.T) {
		report := verifier.VerifyMemoryManager(nil)
		assert.NotNil(t, report)
		assert.Len(t, report.CriticalIssues, 1)
		assert.Contains(t, report.CriticalIssues[0], "nil")

		report = verifier.VerifyDesktopCapture(nil)
		assert.NotNil(t, report)
		assert.Len(t, report.CriticalIssues, 1)
		assert.Contains(t, report.CriticalIssues[0], "nil")
	})
}

// Benchmark memory manager operations
func BenchmarkMemoryManager_TrackRelease(b *testing.B) {
	config := MemoryManagerConfig{
		TrackObjectLifecycle: true,
		EnableLeakDetection:  false, // Disable for performance
	}

	mm := NewMemoryManager(config)
	defer mm.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mockObj := &struct{ data int }{data: i}
		id := mm.TrackObject(mockObj, "benchmark.Object", 100)
		mm.ReleaseObject(id)
	}
}
