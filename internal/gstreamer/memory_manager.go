package gstreamer

import (
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// MemoryManagerConfig configuration for the memory manager
type MemoryManagerConfig struct {
	BufferPoolSize       int           `yaml:"buffer_pool_size" json:"buffer_pool_size"`
	GCInterval           time.Duration `yaml:"gc_interval" json:"gc_interval"`
	LeakDetectionEnabled bool          `yaml:"leak_detection" json:"leak_detection"`
	MaxObjectRefs        int           `yaml:"max_object_refs" json:"max_object_refs"`
	CleanupInterval      time.Duration `yaml:"cleanup_interval" json:"cleanup_interval"`
}

// DefaultMemoryManagerConfig returns default memory manager configuration
func DefaultMemoryManagerConfig() *MemoryManagerConfig {
	return &MemoryManagerConfig{
		BufferPoolSize:       50,
		GCInterval:           30 * time.Second,
		LeakDetectionEnabled: true,
		MaxObjectRefs:        1000,
		CleanupInterval:      60 * time.Second,
	}
}

// ObjectRef represents a reference to a GStreamer object for lifecycle management
type ObjectRef struct {
	ID         uint64
	Object     interface{}
	ObjectType string
	CreatedAt  time.Time
	LastAccess time.Time
	RefCount   int32
	IsGCSafe   bool
	Finalizer  func()
}

// BufferPool manages a pool of reusable buffers
type BufferPool struct {
	Name       string
	BufferSize int
	MaxBuffers int
	buffers    chan []byte
	created    int64
	reused     int64
	mutex      sync.RWMutex
}

// NewBufferPool creates a new buffer pool
func NewBufferPool(name string, bufferSize, maxBuffers int) *BufferPool {
	return &BufferPool{
		Name:       name,
		BufferSize: bufferSize,
		MaxBuffers: maxBuffers,
		buffers:    make(chan []byte, maxBuffers),
	}
}

// Get retrieves a buffer from the pool or creates a new one
func (bp *BufferPool) Get() []byte {
	select {
	case buffer := <-bp.buffers:
		bp.mutex.Lock()
		bp.reused++
		bp.mutex.Unlock()
		return buffer
	default:
		bp.mutex.Lock()
		bp.created++
		bp.mutex.Unlock()
		return make([]byte, bp.BufferSize)
	}
}

// Put returns a buffer to the pool
func (bp *BufferPool) Put(buffer []byte) {
	if len(buffer) != bp.BufferSize {
		return // Wrong size, don't pool it
	}

	select {
	case bp.buffers <- buffer:
		// Successfully returned to pool
	default:
		// Pool is full, let it be garbage collected
	}
}

// GetStats returns buffer pool statistics
func (bp *BufferPool) GetStats() map[string]interface{} {
	bp.mutex.RLock()
	defer bp.mutex.RUnlock()

	return map[string]interface{}{
		"name":        bp.Name,
		"buffer_size": bp.BufferSize,
		"max_buffers": bp.MaxBuffers,
		"pooled":      len(bp.buffers),
		"created":     bp.created,
		"reused":      bp.reused,
		"reuse_ratio": float64(bp.reused) / float64(bp.created+bp.reused),
	}
}

// MemoryStatistics holds memory usage statistics
type MemoryStatistics struct {
	ObjectRefsCount    int64                  `json:"object_refs_count"`
	BufferPoolsCount   int64                  `json:"buffer_pools_count"`
	GCSafeObjectsCount int64                  `json:"gc_safe_objects_count"`
	MemoryUsage        int64                  `json:"memory_usage"`
	LastGC             time.Time              `json:"last_gc"`
	GCCount            int64                  `json:"gc_count"`
	LeaksDetected      int64                  `json:"leaks_detected"`
	BufferPoolStats    map[string]interface{} `json:"buffer_pool_stats"`
}

// MemoryLeak represents a detected memory leak
type MemoryLeak struct {
	ObjectID   uint64        `json:"object_id"`
	ObjectType string        `json:"object_type"`
	CreatedAt  time.Time     `json:"created_at"`
	LastAccess time.Time     `json:"last_access"`
	RefCount   int32         `json:"ref_count"`
	Age        time.Duration `json:"age"`
}

// MemoryManager manages GStreamer object lifecycle and memory usage
// Based on go-gst best practices for memory management
type MemoryManager struct {
	config *MemoryManagerConfig
	logger *logrus.Entry

	// GStreamer object reference management
	objectRefs map[uint64]*ObjectRef
	refMutex   sync.RWMutex
	nextRefID  uint64

	// Buffer pool management
	bufferPools map[string]*BufferPool
	poolMutex   sync.RWMutex

	// GC safety management
	gcSafeObjects []interface{}
	gcMutex       sync.Mutex

	// Statistics and monitoring
	stats      MemoryStatistics
	statsMutex sync.RWMutex

	// Lifecycle management
	running  bool
	stopChan chan struct{}
	wg       sync.WaitGroup
}

// NewMemoryManager creates a new memory manager
func NewMemoryManager(config *MemoryManagerConfig, logger *logrus.Entry) *MemoryManager {
	if config == nil {
		config = DefaultMemoryManagerConfig()
	}
	if logger == nil {
		logger = logrus.NewEntry(logrus.StandardLogger())
	}

	mm := &MemoryManager{
		config:        config,
		logger:        logger,
		objectRefs:    make(map[uint64]*ObjectRef),
		bufferPools:   make(map[string]*BufferPool),
		gcSafeObjects: make([]interface{}, 0),
		stopChan:      make(chan struct{}),
		stats: MemoryStatistics{
			BufferPoolStats: make(map[string]interface{}),
		},
	}

	// Start background cleanup routines
	mm.startBackgroundTasks()

	mm.logger.Debug("Memory manager created successfully")
	return mm
}

// startBackgroundTasks starts background cleanup and monitoring tasks
func (mm *MemoryManager) startBackgroundTasks() {
	mm.running = true

	// Start GC monitoring goroutine
	mm.wg.Add(1)
	go mm.gcMonitorLoop()

	// Start cleanup goroutine
	mm.wg.Add(1)
	go mm.cleanupLoop()

	// Start leak detection goroutine if enabled
	if mm.config.LeakDetectionEnabled {
		mm.wg.Add(1)
		go mm.leakDetectionLoop()
	}
}

// RegisterObject registers a GStreamer object for lifecycle management
func (mm *MemoryManager) RegisterObject(obj interface{}) *ObjectRef {
	if obj == nil {
		return nil
	}

	mm.refMutex.Lock()
	defer mm.refMutex.Unlock()

	// Generate unique ID
	mm.nextRefID++
	id := mm.nextRefID

	// Create object reference
	ref := &ObjectRef{
		ID:         id,
		Object:     obj,
		ObjectType: fmt.Sprintf("%T", obj),
		CreatedAt:  time.Now(),
		LastAccess: time.Now(),
		RefCount:   1,
		IsGCSafe:   false,
	}

	// Store reference
	mm.objectRefs[id] = ref

	// Update statistics
	mm.statsMutex.Lock()
	mm.stats.ObjectRefsCount++
	mm.statsMutex.Unlock()

	mm.logger.Tracef("Registered object: ID=%d, Type=%s", id, ref.ObjectType)
	return ref
}

// UnregisterObject unregisters a GStreamer object
func (mm *MemoryManager) UnregisterObject(ref *ObjectRef) error {
	if ref == nil {
		return fmt.Errorf("object reference is nil")
	}

	mm.refMutex.Lock()
	defer mm.refMutex.Unlock()

	// Check if object exists
	if _, exists := mm.objectRefs[ref.ID]; !exists {
		return fmt.Errorf("object reference not found: ID=%d", ref.ID)
	}

	// Call finalizer if set
	if ref.Finalizer != nil {
		ref.Finalizer()
	}

	// Remove from GC safe objects if needed
	if ref.IsGCSafe {
		mm.releaseGCSafeObject(ref.Object)
	}

	// Remove reference
	delete(mm.objectRefs, ref.ID)

	// Update statistics
	mm.statsMutex.Lock()
	mm.stats.ObjectRefsCount--
	mm.statsMutex.Unlock()

	mm.logger.Tracef("Unregistered object: ID=%d, Type=%s", ref.ID, ref.ObjectType)
	return nil
}

// KeepGCSafe prevents an object from being garbage collected
func (mm *MemoryManager) KeepGCSafe(obj interface{}) {
	if obj == nil {
		return
	}

	mm.gcMutex.Lock()
	defer mm.gcMutex.Unlock()

	mm.gcSafeObjects = append(mm.gcSafeObjects, obj)

	// Update statistics
	mm.statsMutex.Lock()
	mm.stats.GCSafeObjectsCount++
	mm.statsMutex.Unlock()

	mm.logger.Tracef("Object kept GC safe: Type=%T", obj)
}

// ReleaseGCSafe allows an object to be garbage collected
func (mm *MemoryManager) ReleaseGCSafe(obj interface{}) {
	if obj == nil {
		return
	}

	mm.releaseGCSafeObject(obj)
}

// releaseGCSafeObject internal method to release GC safe object
func (mm *MemoryManager) releaseGCSafeObject(obj interface{}) {
	mm.gcMutex.Lock()
	defer mm.gcMutex.Unlock()

	// Find and remove the object
	for i, gcObj := range mm.gcSafeObjects {
		if gcObj == obj {
			// Remove by swapping with last element
			mm.gcSafeObjects[i] = mm.gcSafeObjects[len(mm.gcSafeObjects)-1]
			mm.gcSafeObjects = mm.gcSafeObjects[:len(mm.gcSafeObjects)-1]

			// Update statistics
			mm.statsMutex.Lock()
			mm.stats.GCSafeObjectsCount--
			mm.statsMutex.Unlock()

			mm.logger.Tracef("Object released from GC safe: Type=%T", obj)
			return
		}
	}

	mm.logger.Warnf("Object not found in GC safe list: Type=%T", obj)
}

// GetBufferPool gets or creates a buffer pool
func (mm *MemoryManager) GetBufferPool(name string, bufferSize int) *BufferPool {
	mm.poolMutex.Lock()
	defer mm.poolMutex.Unlock()

	// Check if pool already exists
	if pool, exists := mm.bufferPools[name]; exists {
		return pool
	}

	// Create new buffer pool
	pool := NewBufferPool(name, bufferSize, mm.config.BufferPoolSize)
	mm.bufferPools[name] = pool

	// Update statistics
	mm.statsMutex.Lock()
	mm.stats.BufferPoolsCount++
	mm.statsMutex.Unlock()

	mm.logger.Debugf("Created buffer pool: name=%s, size=%d", name, bufferSize)
	return pool
}

// ReleaseBufferPool releases a buffer pool
func (mm *MemoryManager) ReleaseBufferPool(name string) error {
	mm.poolMutex.Lock()
	defer mm.poolMutex.Unlock()

	if _, exists := mm.bufferPools[name]; !exists {
		return fmt.Errorf("buffer pool not found: %s", name)
	}

	delete(mm.bufferPools, name)

	// Update statistics
	mm.statsMutex.Lock()
	mm.stats.BufferPoolsCount--
	mm.statsMutex.Unlock()

	mm.logger.Debugf("Released buffer pool: name=%s", name)
	return nil
}

// GetMemoryUsage returns current memory usage in bytes
func (mm *MemoryManager) GetMemoryUsage() int64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return int64(m.Alloc)
}

// GetMemoryStats returns detailed memory statistics
func (mm *MemoryManager) GetMemoryStats() *MemoryStatistics {
	mm.statsMutex.Lock()
	defer mm.statsMutex.Unlock()

	// Update runtime statistics
	mm.stats.MemoryUsage = mm.GetMemoryUsage()

	// Update buffer pool statistics
	mm.poolMutex.RLock()
	for name, pool := range mm.bufferPools {
		mm.stats.BufferPoolStats[name] = pool.GetStats()
	}
	mm.poolMutex.RUnlock()

	return &mm.stats
}

// CheckMemoryLeaks detects potential memory leaks
func (mm *MemoryManager) CheckMemoryLeaks() []MemoryLeak {
	if !mm.config.LeakDetectionEnabled {
		return nil
	}

	mm.refMutex.RLock()
	defer mm.refMutex.RUnlock()

	var leaks []MemoryLeak
	now := time.Now()
	leakThreshold := 5 * time.Minute // Objects older than 5 minutes with high ref count

	for _, ref := range mm.objectRefs {
		age := now.Sub(ref.CreatedAt)
		timeSinceAccess := now.Sub(ref.LastAccess)

		// Detect potential leaks based on age and access patterns
		if age > leakThreshold && timeSinceAccess > leakThreshold && ref.RefCount > 1 {
			leaks = append(leaks, MemoryLeak{
				ObjectID:   ref.ID,
				ObjectType: ref.ObjectType,
				CreatedAt:  ref.CreatedAt,
				LastAccess: ref.LastAccess,
				RefCount:   ref.RefCount,
				Age:        age,
			})
		}
	}

	if len(leaks) > 0 {
		mm.statsMutex.Lock()
		mm.stats.LeaksDetected += int64(len(leaks))
		mm.statsMutex.Unlock()

		mm.logger.Warnf("Detected %d potential memory leaks", len(leaks))
	}

	return leaks
}

// ForceGC forces garbage collection
func (mm *MemoryManager) ForceGC() {
	mm.logger.Debug("Forcing garbage collection")
	runtime.GC()

	mm.statsMutex.Lock()
	mm.stats.LastGC = time.Now()
	mm.stats.GCCount++
	mm.statsMutex.Unlock()
}

// Cleanup performs cleanup of all managed resources
func (mm *MemoryManager) Cleanup() {
	mm.logger.Debug("Starting memory manager cleanup")

	// Stop background tasks
	if mm.running {
		mm.running = false
		close(mm.stopChan)
		mm.wg.Wait()
	}

	// Clean up object references
	mm.refMutex.Lock()
	for id, ref := range mm.objectRefs {
		if ref.Finalizer != nil {
			ref.Finalizer()
		}
		delete(mm.objectRefs, id)
	}
	mm.refMutex.Unlock()

	// Clean up buffer pools
	mm.poolMutex.Lock()
	for name := range mm.bufferPools {
		delete(mm.bufferPools, name)
	}
	mm.poolMutex.Unlock()

	// Clear GC safe objects
	mm.gcMutex.Lock()
	mm.gcSafeObjects = mm.gcSafeObjects[:0]
	mm.gcMutex.Unlock()

	// Force final GC
	mm.ForceGC()

	mm.logger.Info("Memory manager cleanup completed")
}

// Background task loops

// gcMonitorLoop monitors and triggers garbage collection
func (mm *MemoryManager) gcMonitorLoop() {
	defer mm.wg.Done()

	ticker := time.NewTicker(mm.config.GCInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			mm.ForceGC()
		case <-mm.stopChan:
			return
		}
	}
}

// cleanupLoop performs periodic cleanup of stale references
func (mm *MemoryManager) cleanupLoop() {
	defer mm.wg.Done()

	ticker := time.NewTicker(mm.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			mm.performCleanup()
		case <-mm.stopChan:
			return
		}
	}
}

// leakDetectionLoop performs periodic leak detection
func (mm *MemoryManager) leakDetectionLoop() {
	defer mm.wg.Done()

	ticker := time.NewTicker(2 * mm.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			leaks := mm.CheckMemoryLeaks()
			if len(leaks) > 0 {
				mm.logger.Warnf("Memory leak detection found %d potential leaks", len(leaks))
				for _, leak := range leaks {
					mm.logger.Warnf("Leak: ID=%d, Type=%s, Age=%v, RefCount=%d",
						leak.ObjectID, leak.ObjectType, leak.Age, leak.RefCount)
				}
			}
		case <-mm.stopChan:
			return
		}
	}
}

// performCleanup performs cleanup of stale object references
func (mm *MemoryManager) performCleanup() {
	mm.refMutex.Lock()
	defer mm.refMutex.Unlock()

	now := time.Now()
	cleanupThreshold := 10 * time.Minute
	cleaned := 0

	for id, ref := range mm.objectRefs {
		// Clean up objects that haven't been accessed recently and have zero ref count
		if ref.RefCount <= 0 && now.Sub(ref.LastAccess) > cleanupThreshold {
			if ref.Finalizer != nil {
				ref.Finalizer()
			}
			delete(mm.objectRefs, id)
			cleaned++
		}
	}

	if cleaned > 0 {
		mm.logger.Debugf("Cleaned up %d stale object references", cleaned)
	}
}

// UpdateObjectAccess updates the last access time for an object reference
func (mm *MemoryManager) UpdateObjectAccess(ref *ObjectRef) {
	if ref == nil {
		return
	}

	mm.refMutex.Lock()
	defer mm.refMutex.Unlock()

	if objRef, exists := mm.objectRefs[ref.ID]; exists {
		objRef.LastAccess = time.Now()
	}
}

// SetObjectFinalizer sets a finalizer function for an object
func (mm *MemoryManager) SetObjectFinalizer(ref *ObjectRef, finalizer func()) {
	if ref == nil {
		return
	}

	mm.refMutex.Lock()
	defer mm.refMutex.Unlock()

	if objRef, exists := mm.objectRefs[ref.ID]; exists {
		objRef.Finalizer = finalizer
	}
}

// GetObjectRef retrieves an object reference by ID
func (mm *MemoryManager) GetObjectRef(id uint64) *ObjectRef {
	mm.refMutex.RLock()
	defer mm.refMutex.RUnlock()

	if ref, exists := mm.objectRefs[id]; exists {
		ref.LastAccess = time.Now()
		return ref
	}

	return nil
}

// GetObjectCount returns the current number of managed objects
func (mm *MemoryManager) GetObjectCount() int64 {
	mm.refMutex.RLock()
	defer mm.refMutex.RUnlock()
	return int64(len(mm.objectRefs))
}
