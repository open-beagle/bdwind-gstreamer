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

// TestBufferPoolConcurrentAccess tests concurrent access to BufferPool
// This addresses requirement 2.1: BufferPool should handle concurrent access safely
func TestBufferPoolConcurrentAccess(t *testing.T) {
	poolSize := 50
	maxSize := 1024 * 1024 // 1MB
	timeout := 100 * time.Millisecond

	bp := NewBufferPool(poolSize, maxSize, timeout)
	defer bp.Close()

	const numGoroutines = 20
	const operationsPerGoroutine = 100

	var wg sync.WaitGroup
	var successfulOperations int64
	var failedOperations int64

	// Test concurrent GetBuffer and ReturnBuffer operations
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()

			for j := 0; j < operationsPerGoroutine; j++ {
				// Get buffer
				bufferSize := 512 + (j % 512) // Vary buffer sizes
				buffer := bp.GetBuffer(bufferSize)

				if buffer != nil && len(buffer) == bufferSize {
					atomic.AddInt64(&successfulOperations, 1)

					// Simulate some work with the buffer
					for k := 0; k < len(buffer); k++ {
						buffer[k] = byte(k % 256)
					}

					// Return buffer after a short delay
					time.Sleep(time.Microsecond * 10)
					bp.ReturnBuffer(buffer)
				} else {
					atomic.AddInt64(&failedOperations, 1)
				}
			}
		}(i)
	}

	wg.Wait()

	// Verify that most operations succeeded
	totalOperations := int64(numGoroutines * operationsPerGoroutine)
	successRate := float64(successfulOperations) / float64(totalOperations)

	t.Logf("Concurrent access test results:")
	t.Logf("  Total operations: %d", totalOperations)
	t.Logf("  Successful operations: %d", successfulOperations)
	t.Logf("  Failed operations: %d", failedOperations)
	t.Logf("  Success rate: %.2f%%", successRate*100)

	// We expect at least 80% success rate in concurrent access
	assert.Greater(t, successRate, 0.8, "Success rate should be at least 80%")

	// Verify buffer pool statistics
	hits, misses, poolSize := bp.GetStats()
	t.Logf("Buffer pool stats: hits=%d, misses=%d, pool_size=%d", hits, misses, poolSize)

	assert.Greater(t, hits, int64(0), "Should have some buffer pool hits")
	assert.GreaterOrEqual(t, hits+misses, successfulOperations, "Total hits+misses should account for successful operations")
}

// TestBufferPoolConcurrentCloseAccess tests concurrent access during pool closure
// This addresses requirement 2.2: BufferPool should handle closure safely during concurrent access
func TestBufferPoolConcurrentCloseAccess(t *testing.T) {
	poolSize := 20
	maxSize := 1024
	timeout := 50 * time.Millisecond

	bp := NewBufferPool(poolSize, maxSize, timeout)

	const numGoroutines = 10
	var wg sync.WaitGroup
	var operationsAfterClose int64

	// Start goroutines that will access the pool
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for j := 0; j < 50; j++ {
				// Check if pool is closed
				if atomic.LoadInt32(&bp.closed) == 1 {
					atomic.AddInt64(&operationsAfterClose, 1)
				}

				buffer := bp.GetBuffer(512)
				if buffer != nil {
					// Simulate work
					time.Sleep(time.Microsecond * 100)
					bp.ReturnBuffer(buffer)
				}

				time.Sleep(time.Microsecond * 50)
			}
		}()
	}

	// Close the pool after a short delay
	time.Sleep(10 * time.Millisecond)
	bp.Close()

	wg.Wait()

	t.Logf("Operations attempted after close: %d", operationsAfterClose)

	// Verify that the pool is marked as closed
	assert.Equal(t, int32(1), atomic.LoadInt32(&bp.closed), "Pool should be marked as closed")

	// Verify that operations after close don't cause panics and return valid buffers
	buffer := bp.GetBuffer(256)
	assert.NotNil(t, buffer, "GetBuffer should return a buffer even after close")
	assert.Equal(t, 256, len(buffer), "Buffer should have correct size")

	// Returning buffer after close should not panic
	assert.NotPanics(t, func() {
		bp.ReturnBuffer(buffer)
	}, "ReturnBuffer should not panic after close")
}

// TestBufferPoolStressTest performs stress testing with high concurrency
// This addresses requirement 2.1: BufferPool should handle high concurrent load
func TestBufferPoolStressTest(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	poolSize := 100
	maxSize := 2 * 1024 * 1024 // 2MB
	timeout := 200 * time.Millisecond

	bp := NewBufferPool(poolSize, maxSize, timeout)
	defer bp.Close()

	const numGoroutines = 50
	const operationsPerGoroutine = 200
	const testDuration = 5 * time.Second

	var wg sync.WaitGroup
	var totalOperations int64
	var successfulOperations int64
	var timeoutOperations int64
	var panicCount int64

	startTime := time.Now()

	// High-intensity concurrent access
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()

			for j := 0; j < operationsPerGoroutine && time.Since(startTime) < testDuration; j++ {
				atomic.AddInt64(&totalOperations, 1)

				// Recover from any panics
				func() {
					defer func() {
						if r := recover(); r != nil {
							atomic.AddInt64(&panicCount, 1)
							t.Logf("Recovered from panic in goroutine %d: %v", goroutineID, r)
						}
					}()

					// Vary buffer sizes to stress different code paths
					bufferSize := 1024 + (j % 1024 * 1024) // 1KB to 1MB
					if bufferSize > maxSize {
						bufferSize = maxSize
					}

					startOp := time.Now()
					buffer := bp.GetBuffer(bufferSize)
					opDuration := time.Since(startOp)

					if buffer != nil && len(buffer) == bufferSize {
						atomic.AddInt64(&successfulOperations, 1)

						// Simulate intensive buffer usage
						for k := 0; k < len(buffer); k += 1024 {
							buffer[k] = byte(k % 256)
						}

						// Variable delay before returning
						delay := time.Microsecond * time.Duration(j%100)
						time.Sleep(delay)

						bp.ReturnBuffer(buffer)
					} else if opDuration >= timeout {
						atomic.AddInt64(&timeoutOperations, 1)
					}
				}()

				// Small delay to allow other goroutines to run
				if j%10 == 0 {
					runtime.Gosched()
				}
			}
		}(i)
	}

	wg.Wait()

	// Collect final statistics
	hits, misses, currentPoolSize := bp.GetStats()
	totalOps := atomic.LoadInt64(&totalOperations)
	successOps := atomic.LoadInt64(&successfulOperations)
	timeoutOps := atomic.LoadInt64(&timeoutOperations)
	panics := atomic.LoadInt64(&panicCount)

	t.Logf("Stress test results:")
	t.Logf("  Test duration: %v", time.Since(startTime))
	t.Logf("  Total operations: %d", totalOps)
	t.Logf("  Successful operations: %d", successOps)
	t.Logf("  Timeout operations: %d", timeoutOps)
	t.Logf("  Panics: %d", panics)
	t.Logf("  Buffer pool hits: %d", hits)
	t.Logf("  Buffer pool misses: %d", misses)
	t.Logf("  Current pool size: %d", currentPoolSize)

	// Verify stress test results
	assert.Equal(t, int64(0), panics, "Should not have any panics during stress test")
	assert.Greater(t, successOps, totalOps/2, "At least 50% of operations should succeed")
	assert.Greater(t, hits, int64(0), "Should have some buffer pool hits")

	// Verify pool is still healthy after stress test
	assert.True(t, bp.IsHealthy(), "Buffer pool should be healthy after stress test")
}

// TestBufferPoolMemoryPressure tests BufferPool behavior under memory pressure
// This addresses requirement 2.2: BufferPool should handle memory pressure gracefully
func TestBufferPoolMemoryPressure(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping memory pressure test in short mode")
	}

	poolSize := 10
	maxSize := 10 * 1024 * 1024 // 10MB - large buffers to create memory pressure
	timeout := 100 * time.Millisecond

	bp := NewBufferPool(poolSize, maxSize, timeout)
	defer bp.Close()

	const numGoroutines = 20
	var wg sync.WaitGroup
	var largeBufferOperations int64
	var memoryPressureDetected int64

	// Get initial memory stats
	var initialMemStats runtime.MemStats
	runtime.ReadMemStats(&initialMemStats)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for j := 0; j < 20; j++ {
				// Request large buffers to create memory pressure
				bufferSize := maxSize - (j * 1024) // Vary size slightly
				buffer := bp.GetBuffer(bufferSize)

				if buffer != nil {
					atomic.AddInt64(&largeBufferOperations, 1)

					// Check memory usage
					var memStats runtime.MemStats
					runtime.ReadMemStats(&memStats)

					if memStats.Alloc > initialMemStats.Alloc*2 {
						atomic.AddInt64(&memoryPressureDetected, 1)
					}

					// Simulate work with large buffer
					for k := 0; k < len(buffer); k += 4096 {
						buffer[k] = byte(k % 256)
					}

					time.Sleep(10 * time.Millisecond)
					bp.ReturnBuffer(buffer)
				}

				// Trigger GC periodically
				if j%5 == 0 {
					runtime.GC()
				}
			}
		}()
	}

	wg.Wait()

	// Force GC to clean up
	runtime.GC()
	runtime.GC()

	var finalMemStats runtime.MemStats
	runtime.ReadMemStats(&finalMemStats)

	t.Logf("Memory pressure test results:")
	t.Logf("  Large buffer operations: %d", largeBufferOperations)
	t.Logf("  Memory pressure detected: %d", memoryPressureDetected)
	t.Logf("  Initial memory: %d bytes", initialMemStats.Alloc)
	t.Logf("  Final memory: %d bytes", finalMemStats.Alloc)
	t.Logf("  Memory growth: %d bytes", int64(finalMemStats.Alloc)-int64(initialMemStats.Alloc))

	// Verify that operations completed successfully
	assert.Greater(t, largeBufferOperations, int64(0), "Should have completed some large buffer operations")

	// Verify pool is still functional
	testBuffer := bp.GetBuffer(1024)
	assert.NotNil(t, testBuffer, "Pool should still be functional after memory pressure")
	assert.Equal(t, 1024, len(testBuffer), "Buffer should have correct size")
	bp.ReturnBuffer(testBuffer)
}

// TestBufferPoolRaceConditions tests for race conditions in BufferPool
// This addresses requirement 2.1: BufferPool should be free of race conditions
func TestBufferPoolRaceConditions(t *testing.T) {
	poolSize := 5 // Small pool to increase contention
	maxSize := 1024
	timeout := 10 * time.Millisecond // Short timeout to increase race conditions

	bp := NewBufferPool(poolSize, maxSize, timeout)
	defer bp.Close()

	const numGoroutines = 100
	const operationsPerGoroutine = 50

	var wg sync.WaitGroup
	var raceConditionDetected int64

	// Pre-fill the pool
	var initialBuffers [][]byte
	for i := 0; i < poolSize; i++ {
		buffer := make([]byte, 512)
		bp.ReturnBuffer(buffer)
		initialBuffers = append(initialBuffers, buffer)
	}

	// Start many goroutines to create race conditions
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()

			for j := 0; j < operationsPerGoroutine; j++ {
				// Rapid get/return cycles to create race conditions
				buffer1 := bp.GetBuffer(256)
				buffer2 := bp.GetBuffer(512)

				// Check for potential race condition indicators
				if buffer1 != nil && buffer2 != nil {
					// Check if we got the same buffer (race condition)
					if &buffer1[0] == &buffer2[0] && len(buffer1) > 0 && len(buffer2) > 0 {
						atomic.AddInt64(&raceConditionDetected, 1)
					}
				}

				// Return buffers in different order
				if j%2 == 0 {
					if buffer2 != nil {
						bp.ReturnBuffer(buffer2)
					}
					if buffer1 != nil {
						bp.ReturnBuffer(buffer1)
					}
				} else {
					if buffer1 != nil {
						bp.ReturnBuffer(buffer1)
					}
					if buffer2 != nil {
						bp.ReturnBuffer(buffer2)
					}
				}

				// No delay to maximize race condition potential
			}
		}(i)
	}

	wg.Wait()

	t.Logf("Race condition test results:")
	t.Logf("  Race conditions detected: %d", raceConditionDetected)

	// Verify no race conditions were detected
	assert.Equal(t, int64(0), raceConditionDetected, "Should not detect any race conditions")

	// Verify pool integrity
	hits, misses, currentPoolSize := bp.GetStats()
	t.Logf("  Final pool stats: hits=%d, misses=%d, size=%d", hits, misses, currentPoolSize)

	assert.True(t, bp.IsHealthy(), "Pool should be healthy after race condition test")
}

// TestBufferPoolTimeoutHandling tests timeout handling in BufferPool
// This addresses requirement 2.2: BufferPool should handle timeouts correctly
func TestBufferPoolTimeoutHandling(t *testing.T) {
	poolSize := 2 // Very small pool
	maxSize := 1024
	timeout := 50 * time.Millisecond // Short timeout

	bp := NewBufferPool(poolSize, maxSize, timeout)
	defer bp.Close()

	// Fill the pool completely
	buffer1 := make([]byte, 512)
	buffer2 := make([]byte, 512)
	bp.ReturnBuffer(buffer1)
	bp.ReturnBuffer(buffer2)

	// Get all buffers from pool
	gotBuffer1 := bp.GetBuffer(512)
	gotBuffer2 := bp.GetBuffer(512)

	require.NotNil(t, gotBuffer1, "Should get first buffer")
	require.NotNil(t, gotBuffer2, "Should get second buffer")

	// Now pool should be empty, next request should timeout
	startTime := time.Now()
	timeoutBuffer := bp.GetBuffer(512)
	elapsed := time.Since(startTime)

	t.Logf("Timeout test results:")
	t.Logf("  Elapsed time: %v", elapsed)
	t.Logf("  Expected timeout: %v", timeout)
	t.Logf("  Buffer received: %v", timeoutBuffer != nil)

	// Should have timed out and created new buffer
	assert.NotNil(t, timeoutBuffer, "Should get a buffer even after timeout")
	assert.GreaterOrEqual(t, elapsed, timeout, "Should have waited at least the timeout duration")
	assert.Less(t, elapsed, timeout*2, "Should not have waited much longer than timeout")

	// Verify timeout was recorded in stats
	_, misses, _ := bp.GetStats()
	assert.Greater(t, misses, int64(0), "Should have recorded timeout as miss")

	// Return buffers
	bp.ReturnBuffer(gotBuffer1)
	bp.ReturnBuffer(gotBuffer2)
	bp.ReturnBuffer(timeoutBuffer)
}

// BenchmarkBufferPoolConcurrentAccess benchmarks concurrent access performance
func BenchmarkBufferPoolConcurrentAccess(b *testing.B) {
	poolSize := 100
	maxSize := 1024 * 1024
	timeout := 100 * time.Millisecond

	bp := NewBufferPool(poolSize, maxSize, timeout)
	defer bp.Close()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		bufferSize := 4096
		for pb.Next() {
			buffer := bp.GetBuffer(bufferSize)
			if buffer != nil {
				// Simulate some work
				for i := 0; i < len(buffer); i += 1024 {
					buffer[i] = byte(i % 256)
				}
				bp.ReturnBuffer(buffer)
			}
		}
	})

	hits, misses, _ := bp.GetStats()
	b.Logf("Benchmark results: hits=%d, misses=%d, hit_ratio=%.2f%%",
		hits, misses, float64(hits)/float64(hits+misses)*100)
}
