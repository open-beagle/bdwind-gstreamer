package gstreamer

import (
	"fmt"
	"reflect"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-gst/go-gst/gst"
	"github.com/sirupsen/logrus"
)

// ObjectLifecycleManager manages the lifecycle of GStreamer objects to prevent segfaults
// during garbage collection by tracking object validity and reference counts
type ObjectLifecycleManager struct {
	// Object tracking
	objects map[uintptr]*ObjectMetadata
	mutex   sync.RWMutex

	// Configuration
	config ObjectLifecycleConfig
	logger *logrus.Entry

	// Statistics
	stats ObjectLifecycleStats

	// Cleanup management
	cleanupTicker *time.Ticker
	stopCleanup   chan struct{}

	// GC safety management
	gcSafetyManager *GCSafetyManager
}

// ObjectMetadata contains metadata about tracked GStreamer objects
type ObjectMetadata struct {
	// Object identification
	ID      uintptr
	Type    string
	Name    string
	Address uintptr

	// Lifecycle tracking
	Created  time.Time
	RefCount int32
	Valid    bool
	Released bool
	LastUsed time.Time

	// Object reference (weak reference to avoid circular dependencies)
	ObjectPtr interface{}

	// Stack trace for debugging
	StackTrace string
}

// ObjectLifecycleConfig configures the object lifecycle manager
type ObjectLifecycleConfig struct {
	// Cleanup intervals
	CleanupInterval time.Duration `json:"cleanup_interval"`
	ObjectRetention time.Duration `json:"object_retention"`

	// Validation settings
	EnableValidation   bool          `json:"enable_validation"`
	ValidationInterval time.Duration `json:"validation_interval"`

	// Debugging
	EnableStackTrace bool `json:"enable_stack_trace"`
	LogObjectEvents  bool `json:"log_object_events"`
}

// ObjectLifecycleStats tracks statistics about object lifecycle management
type ObjectLifecycleStats struct {
	mu sync.RWMutex

	// Object counts
	TotalRegistered int64
	CurrentActive   int64
	TotalReleased   int64
	InvalidObjects  int64

	// Cleanup stats
	CleanupRuns    int64
	ObjectsCleaned int64
	LastCleanup    time.Time

	// Validation stats
	ValidationRuns   int64
	ValidationErrors int64
	LastValidation   time.Time
}

// NewObjectLifecycleManager creates a new object lifecycle manager
func NewObjectLifecycleManager(config ObjectLifecycleConfig) *ObjectLifecycleManager {
	// Set default values
	if config.CleanupInterval == 0 {
		config.CleanupInterval = 30 * time.Second
	}
	if config.ObjectRetention == 0 {
		config.ObjectRetention = 5 * time.Minute
	}
	if config.ValidationInterval == 0 {
		config.ValidationInterval = 60 * time.Second
	}

	// Initialize GC safety manager
	gcSafetyConfig := GCSafetyConfig{
		CleanupInterval:       config.CleanupInterval,
		StateRetention:        config.ObjectRetention,
		FinalizerTimeout:      10 * time.Second,
		EnableStateChecking:   config.EnableValidation,
		EnableFinalizerSafety: true,
		MaxFinalizerRetries:   3,
		LogStateChanges:       config.LogObjectEvents,
		LogFinalizers:         config.LogObjectEvents,
	}

	olm := &ObjectLifecycleManager{
		objects:         make(map[uintptr]*ObjectMetadata),
		config:          config,
		logger:          logrus.WithField("component", "object-lifecycle-manager"),
		stopCleanup:     make(chan struct{}),
		gcSafetyManager: NewGCSafetyManager(gcSafetyConfig),
	}

	// Start cleanup routine
	olm.startCleanupRoutine()

	olm.logger.Info("Object lifecycle manager initialized")
	return olm
}

// RegisterObject registers a GStreamer object for lifecycle tracking with GC safety
func (olm *ObjectLifecycleManager) RegisterObject(obj interface{}, objType, name string) (uintptr, error) {
	if obj == nil {
		return 0, fmt.Errorf("cannot register nil object")
	}

	// Register with GC safety manager first
	var gcSafetyID uintptr
	if olm.gcSafetyManager != nil {
		var err error
		gcSafetyID, err = olm.gcSafetyManager.RegisterObject(obj, fmt.Sprintf("%s[%s]", objType, name))
		if err != nil {
			olm.logger.Warnf("Failed to register object with GC safety manager: %v", err)
		}
	}

	// Generate unique ID based on object address
	id := olm.generateObjectID(obj)
	if id == 0 {
		// If we registered with GC safety manager, unregister
		if gcSafetyID != 0 && olm.gcSafetyManager != nil {
			olm.gcSafetyManager.UnregisterObject(gcSafetyID)
		}
		return 0, fmt.Errorf("failed to generate object ID")
	}

	olm.mutex.Lock()
	defer olm.mutex.Unlock()

	// Check if object is already registered
	if existing, exists := olm.objects[id]; exists {
		if existing.Valid {
			// Object already registered and valid, increment ref count
			atomic.AddInt32(&existing.RefCount, 1)
			existing.LastUsed = time.Now()

			// Also increment ref count in GC safety manager
			if olm.gcSafetyManager != nil && gcSafetyID != 0 {
				// The GC safety manager handles reference counting internally
			}

			if olm.config.LogObjectEvents {
				olm.logger.Debugf("Object already registered, incremented ref count: %s[%s] ID=%x RefCount=%d",
					objType, name, id, existing.RefCount)
			}
			return id, nil
		}
		// Object exists but invalid, remove it first
		delete(olm.objects, id)
	}

	// Create new metadata
	metadata := &ObjectMetadata{
		ID:        id,
		Type:      objType,
		Name:      name,
		Address:   id,
		Created:   time.Now(),
		RefCount:  1,
		Valid:     true,
		Released:  false,
		LastUsed:  time.Now(),
		ObjectPtr: obj,
	}

	// Add stack trace if enabled
	if olm.config.EnableStackTrace {
		metadata.StackTrace = olm.getStackTrace()
	}

	olm.objects[id] = metadata

	// Add cleanup callback to GC safety manager (avoid circular calls)
	if gcSafetyID != 0 && olm.gcSafetyManager != nil {
		olm.gcSafetyManager.AddCleanupCallback(gcSafetyID, func() {
			olm.logger.Debugf("GC safety cleanup for object %s[%s] (ID: %x)", objType, name, id)
			// Mark object as invalid directly without calling ForceInvalidateObject to avoid deadlock
			// Use a separate goroutine to avoid deadlock during cleanup callbacks
			go func() {
				olm.mutex.Lock()
				if metadata, exists := olm.objects[id]; exists {
					metadata.Valid = false
					metadata.Released = true
				}
				olm.mutex.Unlock()
			}()
		})
	}

	// Update statistics
	olm.stats.mu.Lock()
	olm.stats.TotalRegistered++
	olm.stats.CurrentActive++
	olm.stats.mu.Unlock()

	if olm.config.LogObjectEvents {
		olm.logger.Debugf("Registered object: %s[%s] ID=%x", objType, name, id)
	}

	return id, nil
}

// UnregisterObject unregisters a GStreamer object from lifecycle tracking with GC safety
func (olm *ObjectLifecycleManager) UnregisterObject(id uintptr) error {
	if id == 0 {
		return fmt.Errorf("invalid object ID: 0")
	}

	olm.mutex.Lock()
	defer olm.mutex.Unlock()

	metadata, exists := olm.objects[id]
	if !exists {
		return fmt.Errorf("object ID %x not found", id)
	}

	// Decrement reference count
	newRefCount := atomic.AddInt32(&metadata.RefCount, -1)

	if newRefCount > 0 {
		// Still has references, just update last used time
		metadata.LastUsed = time.Now()

		if olm.config.LogObjectEvents {
			olm.logger.Debugf("Decremented ref count: %s[%s] ID=%x RefCount=%d",
				metadata.Type, metadata.Name, id, newRefCount)
		}
		return nil
	}

	// No more references, mark as released
	metadata.Released = true
	metadata.Valid = false
	metadata.LastUsed = time.Now()

	// Unregister from GC safety manager
	if olm.gcSafetyManager != nil {
		// Execute cleanup callbacks before unregistering
		olm.gcSafetyManager.ExecuteCleanupCallbacks(id)

		if err := olm.gcSafetyManager.UnregisterObject(id); err != nil {
			olm.logger.Debugf("Failed to unregister object from GC safety manager: %v", err)
		}
	}

	// Update statistics
	olm.stats.mu.Lock()
	olm.stats.CurrentActive--
	olm.stats.TotalReleased++
	olm.stats.mu.Unlock()

	if olm.config.LogObjectEvents {
		olm.logger.Debugf("Unregistered object: %s[%s] ID=%x (lifetime: %v)",
			metadata.Type, metadata.Name, id, time.Since(metadata.Created))
	}

	return nil
}

// IsValidObject checks if a GStreamer object is still valid with GC safety
func (olm *ObjectLifecycleManager) IsValidObject(id uintptr) bool {
	if id == 0 {
		return false
	}

	// Use GC safety manager for validation if available
	if olm.gcSafetyManager != nil {
		return olm.gcSafetyManager.IsObjectValid(id)
	}

	// Fallback to basic validation
	olm.mutex.RLock()
	defer olm.mutex.RUnlock()

	metadata, exists := olm.objects[id]
	if !exists {
		return false
	}

	// Update last used time for valid objects
	if metadata.Valid && !metadata.Released {
		metadata.LastUsed = time.Now()
		return true
	}

	return false
}

// ValidateGstObject validates a specific GStreamer object type
func (olm *ObjectLifecycleManager) ValidateGstObject(obj interface{}) bool {
	if obj == nil {
		return false
	}

	// Type-specific validation for different GStreamer objects
	switch v := obj.(type) {
	case *gst.Pipeline:
		return olm.validatePipeline(v)
	case *gst.Element:
		return olm.validateElement(v)
	case *gst.Bus:
		return olm.validateBus(v)
	case *gst.Sample:
		return olm.validateSample(v)
	default:
		// For unknown types, just check if it's not nil
		return true
	}
}

// SafeObjectAccess provides safe access to a tracked object with GC safety checks
func (olm *ObjectLifecycleManager) SafeObjectAccess(id uintptr, accessFunc func() error) error {
	if id == 0 {
		return fmt.Errorf("invalid object ID: 0")
	}

	// Use GC safety manager for safe access if available
	if olm.gcSafetyManager != nil {
		return olm.gcSafetyManager.SafeAccess(id, accessFunc)
	}

	// Fallback to basic validation
	if !olm.IsValidObject(id) {
		return fmt.Errorf("object ID %x is not valid for access", id)
	}

	// Execute access function with panic recovery
	var err error
	func() {
		defer func() {
			if r := recover(); r != nil {
				olm.logger.Errorf("Panic during safe object access ID %x: %v", id, r)
				err = fmt.Errorf("panic during object access: %v", r)

				// Mark object as invalid after panic
				olm.ForceInvalidateObject(id)
			}
		}()
		err = accessFunc()
	}()

	return err
}

// validatePipeline validates a GStreamer pipeline object
func (olm *ObjectLifecycleManager) validatePipeline(pipeline *gst.Pipeline) bool {
	if pipeline == nil {
		return false
	}

	// Check if pipeline is in a valid state
	// Note: We avoid calling methods that might cause segfaults
	defer func() {
		if r := recover(); r != nil {
			olm.logger.Warnf("Pipeline validation failed with panic: %v", r)
		}
	}()

	// Basic validation - just check if the pointer is not nil
	// More sophisticated validation could be added here if safe methods are available
	return true
}

// validateElement validates a GStreamer element object
func (olm *ObjectLifecycleManager) validateElement(element *gst.Element) bool {
	if element == nil {
		return false
	}

	defer func() {
		if r := recover(); r != nil {
			olm.logger.Warnf("Element validation failed with panic: %v", r)
		}
	}()

	// Basic validation - just check if the pointer is not nil
	return true
}

// validateBus validates a GStreamer bus object
func (olm *ObjectLifecycleManager) validateBus(bus *gst.Bus) bool {
	if bus == nil {
		return false
	}

	defer func() {
		if r := recover(); r != nil {
			olm.logger.Warnf("Bus validation failed with panic: %v", r)
		}
	}()

	// Basic validation - just check if the pointer is not nil
	return true
}

// validateSample validates a GStreamer sample object
func (olm *ObjectLifecycleManager) validateSample(sample *gst.Sample) bool {
	if sample == nil {
		return false
	}

	defer func() {
		if r := recover(); r != nil {
			olm.logger.Warnf("Sample validation failed with panic: %v", r)
		}
	}()

	// Basic validation - just check if the pointer is not nil
	return true
}

// GetObjectInfo returns information about a tracked object
func (olm *ObjectLifecycleManager) GetObjectInfo(id uintptr) *ObjectMetadata {
	if id == 0 {
		return nil
	}

	olm.mutex.RLock()
	defer olm.mutex.RUnlock()

	metadata, exists := olm.objects[id]
	if !exists {
		return nil
	}

	// Return a copy to avoid race conditions
	metadataCopy := *metadata
	return &metadataCopy
}

// GetStats returns current lifecycle management statistics
func (olm *ObjectLifecycleManager) GetStats() ObjectLifecycleStats {
	olm.stats.mu.RLock()
	defer olm.stats.mu.RUnlock()

	// Return a copy of the stats
	return ObjectLifecycleStats{
		TotalRegistered:  olm.stats.TotalRegistered,
		CurrentActive:    olm.stats.CurrentActive,
		TotalReleased:    olm.stats.TotalReleased,
		InvalidObjects:   olm.stats.InvalidObjects,
		CleanupRuns:      olm.stats.CleanupRuns,
		ObjectsCleaned:   olm.stats.ObjectsCleaned,
		LastCleanup:      olm.stats.LastCleanup,
		ValidationRuns:   olm.stats.ValidationRuns,
		ValidationErrors: olm.stats.ValidationErrors,
		LastValidation:   olm.stats.LastValidation,
	}
}

// ListActiveObjects returns a list of currently active objects
func (olm *ObjectLifecycleManager) ListActiveObjects() []*ObjectMetadata {
	olm.mutex.RLock()
	defer olm.mutex.RUnlock()

	var activeObjects []*ObjectMetadata
	for _, metadata := range olm.objects {
		if metadata.Valid && !metadata.Released {
			// Create a copy to avoid race conditions
			metadataCopy := *metadata
			activeObjects = append(activeObjects, &metadataCopy)
		}
	}

	return activeObjects
}

// ForceInvalidateObject forcefully marks an object as invalid
func (olm *ObjectLifecycleManager) ForceInvalidateObject(id uintptr) error {
	if id == 0 {
		return fmt.Errorf("invalid object ID: 0")
	}

	olm.mutex.Lock()
	defer olm.mutex.Unlock()

	metadata, exists := olm.objects[id]
	if !exists {
		return fmt.Errorf("object ID %x not found", id)
	}

	if !metadata.Valid {
		return fmt.Errorf("object ID %x is already invalid", id)
	}

	// Mark as invalid
	metadata.Valid = false
	metadata.Released = true
	metadata.LastUsed = time.Now()

	// Update statistics
	olm.stats.mu.Lock()
	olm.stats.CurrentActive--
	olm.stats.InvalidObjects++
	olm.stats.mu.Unlock()

	if olm.config.LogObjectEvents {
		olm.logger.Warnf("Force invalidated object: %s[%s] ID=%x",
			metadata.Type, metadata.Name, id)
	}

	return nil
}

// generateObjectID generates a unique ID for an object
func (olm *ObjectLifecycleManager) generateObjectID(obj interface{}) uintptr {
	// Use the object's memory address as a unique identifier using reflection
	// This is the same approach used by the existing MemoryManager
	v := reflect.ValueOf(obj)
	if v.Kind() == reflect.Ptr && !v.IsNil() {
		return v.Pointer()
	}

	// For non-pointer types or nil pointers, generate a unique ID
	return uintptr(time.Now().UnixNano())
}

// getStackTrace returns a stack trace for debugging
func (olm *ObjectLifecycleManager) getStackTrace() string {
	buf := make([]byte, 1024)
	n := runtime.Stack(buf, false)
	return string(buf[:n])
}

// startCleanupRoutine starts the background cleanup routine
func (olm *ObjectLifecycleManager) startCleanupRoutine() {
	olm.cleanupTicker = time.NewTicker(olm.config.CleanupInterval)

	go func() {
		defer olm.cleanupTicker.Stop()

		for {
			select {
			case <-olm.cleanupTicker.C:
				olm.performCleanup()

				if olm.config.EnableValidation {
					olm.performValidation()
				}

			case <-olm.stopCleanup:
				return
			}
		}
	}()
}

// performCleanup removes old and invalid objects from tracking
func (olm *ObjectLifecycleManager) performCleanup() {
	olm.mutex.Lock()
	defer olm.mutex.Unlock()

	now := time.Now()
	cleaned := 0

	for id, metadata := range olm.objects {
		shouldCleanup := false

		// Clean up released objects that are old enough
		if metadata.Released && now.Sub(metadata.LastUsed) > olm.config.ObjectRetention {
			shouldCleanup = true
		}

		// Clean up invalid objects
		if !metadata.Valid {
			shouldCleanup = true
		}

		if shouldCleanup {
			delete(olm.objects, id)
			cleaned++

			if olm.config.LogObjectEvents {
				olm.logger.Debugf("Cleaned up object: %s[%s] ID=%x (age: %v)",
					metadata.Type, metadata.Name, id, now.Sub(metadata.Created))
			}
		}
	}

	// Update statistics
	olm.stats.mu.Lock()
	olm.stats.CleanupRuns++
	olm.stats.ObjectsCleaned += int64(cleaned)
	olm.stats.LastCleanup = now
	olm.stats.mu.Unlock()

	if cleaned > 0 {
		olm.logger.Debugf("Cleanup completed: removed %d objects", cleaned)
	}
}

// performValidation validates all tracked objects
func (olm *ObjectLifecycleManager) performValidation() {
	olm.mutex.RLock()
	objects := make([]*ObjectMetadata, 0, len(olm.objects))
	for _, metadata := range olm.objects {
		if metadata.Valid && !metadata.Released {
			metadataCopy := *metadata
			objects = append(objects, &metadataCopy)
		}
	}
	olm.mutex.RUnlock()

	validationErrors := 0
	for _, metadata := range objects {
		if !olm.ValidateGstObject(metadata.ObjectPtr) {
			// Mark object as invalid
			olm.ForceInvalidateObject(metadata.ID)
			validationErrors++

			olm.logger.Warnf("Validation failed for object: %s[%s] ID=%x",
				metadata.Type, metadata.Name, metadata.ID)
		}
	}

	// Update statistics
	olm.stats.mu.Lock()
	olm.stats.ValidationRuns++
	olm.stats.ValidationErrors += int64(validationErrors)
	olm.stats.LastValidation = time.Now()
	olm.stats.mu.Unlock()

	if validationErrors > 0 {
		olm.logger.Warnf("Validation completed: found %d invalid objects", validationErrors)
	}
}

// Close shuts down the object lifecycle manager
func (olm *ObjectLifecycleManager) Close() {
	olm.logger.Info("Shutting down object lifecycle manager")

	// Stop cleanup routine
	close(olm.stopCleanup)

	// Final cleanup
	olm.mutex.Lock()
	activeObjects := len(olm.objects)

	// Mark all objects as invalid to prevent further use
	for _, metadata := range olm.objects {
		metadata.Valid = false
		metadata.Released = true
	}

	// Clear the objects map
	olm.objects = make(map[uintptr]*ObjectMetadata)
	olm.mutex.Unlock()

	// Close GC safety manager
	if olm.gcSafetyManager != nil {
		olm.gcSafetyManager.Close()
		olm.gcSafetyManager = nil
	}

	// Log final statistics
	stats := olm.GetStats()
	olm.logger.Infof("Object lifecycle manager shutdown complete - "+
		"Total registered: %d, Active at shutdown: %d, Total released: %d, Invalid: %d",
		stats.TotalRegistered, activeObjects, stats.TotalReleased, stats.InvalidObjects)
}
