package gstreamer

import (
	"fmt"
	"reflect"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/sirupsen/logrus"
)

// GCSafetyManager manages garbage collection safety for GStreamer objects
// It ensures that finalizers don't access already released objects and provides
// safe object cleanup mechanisms
type GCSafetyManager struct {
	// Object state tracking
	objectStates map[uintptr]*ObjectState
	stateMutex   sync.RWMutex

	// Configuration
	config GCSafetyConfig
	logger *logrus.Entry

	// Statistics
	stats GCSafetyStats

	// Cleanup management
	cleanupTicker *time.Ticker
	stopCleanup   chan struct{}
	isShutdown    int32

	// GObject reference tracking to prevent double-free
	gobjectRefs  map[unsafe.Pointer]bool
	gobjectMutex sync.RWMutex
}

// ObjectState tracks the state of an object for GC safety
type ObjectState struct {
	// Object identification
	ID      uintptr
	Type    string
	Address unsafe.Pointer

	// State tracking
	IsValid    int32 // Atomic flag for validity
	IsReleased int32 // Atomic flag for release status
	RefCount   int32 // Atomic reference count
	CreatedAt  time.Time
	LastAccess time.Time

	// Finalizer management
	HasFinalizer    bool
	FinalizerCalled int32 // Atomic flag to prevent double finalization

	// Cleanup callbacks
	cleanupCallbacks []func()
	callbackMutex    sync.Mutex
}

// GCSafetyConfig configures the GC safety manager
type GCSafetyConfig struct {
	// Cleanup intervals
	CleanupInterval  time.Duration `json:"cleanup_interval"`
	StateRetention   time.Duration `json:"state_retention"`
	FinalizerTimeout time.Duration `json:"finalizer_timeout"`

	// Safety settings
	EnableStateChecking   bool `json:"enable_state_checking"`
	EnableFinalizerSafety bool `json:"enable_finalizer_safety"`
	MaxFinalizerRetries   int  `json:"max_finalizer_retries"`

	// Debugging
	LogStateChanges bool `json:"log_state_changes"`
	LogFinalizers   bool `json:"log_finalizers"`
}

// GCSafetyStats tracks statistics about GC safety operations
type GCSafetyStats struct {
	mu sync.RWMutex

	// Object tracking
	TotalObjects       int64
	ActiveObjects      int64
	ReleasedObjects    int64
	InvalidatedObjects int64

	// Finalizer stats
	FinalizersSet     int64
	FinalizersCalled  int64
	FinalizerFailures int64
	FinalizerTimeouts int64

	// Safety stats
	StateChecksPassed   int64
	StateChecksFailed   int64
	UnsafeAccessBlocked int64

	// Cleanup stats
	CleanupRuns    int64
	ObjectsCleaned int64
	LastCleanup    time.Time
}

// NewGCSafetyManager creates a new GC safety manager
func NewGCSafetyManager(config GCSafetyConfig) *GCSafetyManager {
	// Set default values
	if config.CleanupInterval == 0 {
		config.CleanupInterval = 30 * time.Second
	}
	if config.StateRetention == 0 {
		config.StateRetention = 5 * time.Minute
	}
	if config.FinalizerTimeout == 0 {
		config.FinalizerTimeout = 10 * time.Second
	}
	if config.MaxFinalizerRetries == 0 {
		config.MaxFinalizerRetries = 3
	}

	gsm := &GCSafetyManager{
		objectStates: make(map[uintptr]*ObjectState),
		config:       config,
		logger:       logrus.WithField("component", "gc-safety-manager"),
		stopCleanup:  make(chan struct{}),
	}

	// Start cleanup routine
	gsm.startCleanupRoutine()

	gsm.logger.Info("GC safety manager initialized")
	return gsm
}

// RegisterObject registers an object for GC safety tracking
func (gsm *GCSafetyManager) RegisterObject(obj interface{}, objType string) (uintptr, error) {
	if obj == nil {
		return 0, fmt.Errorf("cannot register nil object")
	}

	// Check if manager is shutdown
	if atomic.LoadInt32(&gsm.isShutdown) == 1 {
		return 0, fmt.Errorf("GC safety manager is shutdown")
	}

	// Get object address
	id := gsm.getObjectID(obj)
	if id == 0 {
		return 0, fmt.Errorf("failed to get object ID")
	}

	gsm.stateMutex.Lock()
	defer gsm.stateMutex.Unlock()

	// Check if object is already registered
	if existing, exists := gsm.objectStates[id]; exists {
		// Increment reference count if still valid
		if atomic.LoadInt32(&existing.IsValid) == 1 {
			atomic.AddInt32(&existing.RefCount, 1)
			existing.LastAccess = time.Now()

			if gsm.config.LogStateChanges {
				gsm.logger.Debugf("Object already registered, incremented ref count: %s ID=%x RefCount=%d",
					objType, id, existing.RefCount)
			}
			return id, nil
		}
		// Object exists but invalid, remove it first
		delete(gsm.objectStates, id)
	}

	// Create new object state
	state := &ObjectState{
		ID:         id,
		Type:       objType,
		Address:    unsafe.Pointer(uintptr(id)),
		CreatedAt:  time.Now(),
		LastAccess: time.Now(),
	}

	// Initialize atomic values
	atomic.StoreInt32(&state.IsValid, 1)
	atomic.StoreInt32(&state.IsReleased, 0)
	atomic.StoreInt32(&state.RefCount, 1)
	atomic.StoreInt32(&state.FinalizerCalled, 0)

	gsm.objectStates[id] = state

	// Update statistics
	gsm.stats.mu.Lock()
	gsm.stats.TotalObjects++
	gsm.stats.ActiveObjects++
	gsm.stats.mu.Unlock()

	if gsm.config.LogStateChanges {
		gsm.logger.Debugf("Registered object: %s ID=%x", objType, id)
	}

	return id, nil
}

// UnregisterObject unregisters an object from GC safety tracking
func (gsm *GCSafetyManager) UnregisterObject(id uintptr) error {
	if id == 0 {
		return fmt.Errorf("invalid object ID: 0")
	}

	gsm.stateMutex.Lock()
	defer gsm.stateMutex.Unlock()

	state, exists := gsm.objectStates[id]
	if !exists {
		return fmt.Errorf("object ID %x not found", id)
	}

	// Decrement reference count
	newRefCount := atomic.AddInt32(&state.RefCount, -1)

	if newRefCount > 0 {
		// Still has references, just update last access time
		state.LastAccess = time.Now()

		if gsm.config.LogStateChanges {
			gsm.logger.Debugf("Decremented ref count: %s ID=%x RefCount=%d",
				state.Type, id, newRefCount)
		}
		return nil
	}

	// No more references, mark as released
	atomic.StoreInt32(&state.IsReleased, 1)
	atomic.StoreInt32(&state.IsValid, 0)
	state.LastAccess = time.Now()

	// Update statistics
	gsm.stats.mu.Lock()
	gsm.stats.ActiveObjects--
	gsm.stats.ReleasedObjects++
	gsm.stats.mu.Unlock()

	if gsm.config.LogStateChanges {
		gsm.logger.Debugf("Unregistered object: %s ID=%x (lifetime: %v)",
			state.Type, id, time.Since(state.CreatedAt))
	}

	return nil
}

// IsObjectValid checks if an object is still valid for access
func (gsm *GCSafetyManager) IsObjectValid(id uintptr) bool {
	if id == 0 {
		return false
	}

	// Check if manager is shutdown
	if atomic.LoadInt32(&gsm.isShutdown) == 1 {
		return false
	}

	gsm.stateMutex.RLock()
	defer gsm.stateMutex.RUnlock()

	state, exists := gsm.objectStates[id]
	if !exists {
		gsm.stats.mu.Lock()
		gsm.stats.StateChecksFailed++
		gsm.stats.mu.Unlock()
		return false
	}

	// Check atomic flags
	isValid := atomic.LoadInt32(&state.IsValid) == 1
	isReleased := atomic.LoadInt32(&state.IsReleased) == 1

	if isValid && !isReleased {
		// Update last access time for valid objects
		state.LastAccess = time.Now()
		gsm.stats.mu.Lock()
		gsm.stats.StateChecksPassed++
		gsm.stats.mu.Unlock()
		return true
	}

	gsm.stats.mu.Lock()
	gsm.stats.StateChecksFailed++
	gsm.stats.mu.Unlock()
	return false
}

// SafeAccess provides safe access to an object with state checking
func (gsm *GCSafetyManager) SafeAccess(id uintptr, accessFunc func() error) error {
	if !gsm.config.EnableStateChecking {
		// State checking disabled, execute directly
		return accessFunc()
	}

	if !gsm.IsObjectValid(id) {
		gsm.stats.mu.Lock()
		gsm.stats.UnsafeAccessBlocked++
		gsm.stats.mu.Unlock()
		return fmt.Errorf("object ID %x is not valid for access", id)
	}

	// Execute the access function with panic recovery
	var err error
	func() {
		defer func() {
			if r := recover(); r != nil {
				gsm.logger.Errorf("Panic during safe access to object ID %x: %v", id, r)
				err = fmt.Errorf("panic during object access: %v", r)

				// Mark object as invalid after panic
				gsm.InvalidateObject(id)
			}
		}()
		err = accessFunc()
	}()

	return err
}

// InvalidateObject marks an object as invalid
func (gsm *GCSafetyManager) InvalidateObject(id uintptr) error {
	if id == 0 {
		return fmt.Errorf("invalid object ID: 0")
	}

	gsm.stateMutex.Lock()
	defer gsm.stateMutex.Unlock()

	state, exists := gsm.objectStates[id]
	if !exists {
		return fmt.Errorf("object ID %x not found", id)
	}

	// Mark as invalid atomically
	wasValid := atomic.SwapInt32(&state.IsValid, 0)
	if wasValid == 1 {
		state.LastAccess = time.Now()

		// Update statistics
		gsm.stats.mu.Lock()
		gsm.stats.InvalidatedObjects++
		if atomic.LoadInt32(&state.IsReleased) == 0 {
			gsm.stats.ActiveObjects--
		}
		gsm.stats.mu.Unlock()

		if gsm.config.LogStateChanges {
			gsm.logger.Debugf("Invalidated object: %s ID=%x", state.Type, id)
		}
	}

	return nil
}

// SetSafeFinalizer sets a finalizer that checks object state before execution
func (gsm *GCSafetyManager) SetSafeFinalizer(obj interface{}, finalizer func()) error {
	if !gsm.config.EnableFinalizerSafety {
		// Finalizer safety disabled, set directly
		runtime.SetFinalizer(obj, finalizer)
		return nil
	}

	id, err := gsm.RegisterObject(obj, "finalizer-tracked")
	if err != nil {
		return fmt.Errorf("failed to register object for finalizer: %w", err)
	}

	// Create safe finalizer wrapper
	safeFinalizer := func(obj interface{}) {
		gsm.executeSafeFinalizer(id, finalizer)
	}

	// Set the safe finalizer
	runtime.SetFinalizer(obj, safeFinalizer)

	// Mark that object has finalizer
	gsm.stateMutex.Lock()
	if state, exists := gsm.objectStates[id]; exists {
		state.HasFinalizer = true
		gsm.stats.mu.Lock()
		gsm.stats.FinalizersSet++
		gsm.stats.mu.Unlock()
	}
	gsm.stateMutex.Unlock()

	if gsm.config.LogFinalizers {
		gsm.logger.Debugf("Set safe finalizer for object ID=%x", id)
	}

	return nil
}

// executeSafeFinalizer executes a finalizer with safety checks
func (gsm *GCSafetyManager) executeSafeFinalizer(id uintptr, finalizer func()) {
	// Check if finalizer was already called
	gsm.stateMutex.RLock()
	state, exists := gsm.objectStates[id]
	gsm.stateMutex.RUnlock()

	if !exists {
		if gsm.config.LogFinalizers {
			gsm.logger.Debugf("Finalizer called for unknown object ID=%x", id)
		}
		return
	}

	// Check if finalizer was already called atomically
	if !atomic.CompareAndSwapInt32(&state.FinalizerCalled, 0, 1) {
		if gsm.config.LogFinalizers {
			gsm.logger.Debugf("Finalizer already called for object ID=%x", id)
		}
		return
	}

	// Update statistics
	gsm.stats.mu.Lock()
	gsm.stats.FinalizersCalled++
	gsm.stats.mu.Unlock()

	if gsm.config.LogFinalizers {
		gsm.logger.Debugf("Executing safe finalizer for object ID=%x", id)
	}

	// Execute finalizer with timeout and panic recovery
	done := make(chan struct{})
	var finalizerErr error

	go func() {
		defer func() {
			if r := recover(); r != nil {
				gsm.logger.Errorf("Panic in finalizer for object ID %x: %v", id, r)
				finalizerErr = fmt.Errorf("finalizer panic: %v", r)
				gsm.stats.mu.Lock()
				gsm.stats.FinalizerFailures++
				gsm.stats.mu.Unlock()
			}
			close(done)
		}()

		// Only execute finalizer if object is still tracked and not already released
		if gsm.IsObjectValid(id) || atomic.LoadInt32(&state.IsReleased) == 0 {
			finalizer()
		} else {
			if gsm.config.LogFinalizers {
				gsm.logger.Debugf("Skipped finalizer for already released object ID=%x", id)
			}
		}
	}()

	// Wait for finalizer completion with timeout
	select {
	case <-done:
		if finalizerErr != nil && gsm.config.LogFinalizers {
			gsm.logger.Errorf("Finalizer failed for object ID %x: %v", id, finalizerErr)
		}
	case <-time.After(gsm.config.FinalizerTimeout):
		gsm.logger.Errorf("Finalizer timeout for object ID %x", id)
		gsm.stats.mu.Lock()
		gsm.stats.FinalizerTimeouts++
		gsm.stats.mu.Unlock()
	}

	// Mark object as released after finalizer execution
	atomic.StoreInt32(&state.IsReleased, 1)
	atomic.StoreInt32(&state.IsValid, 0)
}

// AddCleanupCallback adds a cleanup callback to an object
func (gsm *GCSafetyManager) AddCleanupCallback(id uintptr, callback func()) error {
	if id == 0 {
		return fmt.Errorf("invalid object ID: 0")
	}

	gsm.stateMutex.RLock()
	state, exists := gsm.objectStates[id]
	gsm.stateMutex.RUnlock()

	if !exists {
		return fmt.Errorf("object ID %x not found", id)
	}

	state.callbackMutex.Lock()
	defer state.callbackMutex.Unlock()

	state.cleanupCallbacks = append(state.cleanupCallbacks, callback)
	return nil
}

// ExecuteCleanupCallbacks executes all cleanup callbacks for an object
func (gsm *GCSafetyManager) ExecuteCleanupCallbacks(id uintptr) {
	if id == 0 {
		return
	}

	// Check if manager is shutdown to avoid deadlocks during shutdown
	if atomic.LoadInt32(&gsm.isShutdown) == 1 {
		return
	}

	gsm.stateMutex.RLock()
	state, exists := gsm.objectStates[id]
	gsm.stateMutex.RUnlock()

	if !exists {
		return
	}

	state.callbackMutex.Lock()
	callbacks := make([]func(), len(state.cleanupCallbacks))
	copy(callbacks, state.cleanupCallbacks)
	state.callbackMutex.Unlock()

	// Execute callbacks with panic recovery
	for i, callback := range callbacks {
		func() {
			defer func() {
				if r := recover(); r != nil {
					gsm.logger.Errorf("Panic in cleanup callback %d for object ID %x: %v", i, id, r)
				}
			}()
			callback()
		}()
	}
}

// GetObjectState returns the current state of an object
func (gsm *GCSafetyManager) GetObjectState(id uintptr) *ObjectState {
	if id == 0 {
		return nil
	}

	gsm.stateMutex.RLock()
	defer gsm.stateMutex.RUnlock()

	state, exists := gsm.objectStates[id]
	if !exists {
		return nil
	}

	// Return a copy to avoid race conditions
	stateCopy := *state
	stateCopy.IsValid = atomic.LoadInt32(&state.IsValid)
	stateCopy.IsReleased = atomic.LoadInt32(&state.IsReleased)
	stateCopy.RefCount = atomic.LoadInt32(&state.RefCount)
	stateCopy.FinalizerCalled = atomic.LoadInt32(&state.FinalizerCalled)

	return &stateCopy
}

// GetStats returns current GC safety statistics
func (gsm *GCSafetyManager) GetStats() GCSafetyStats {
	gsm.stats.mu.RLock()
	defer gsm.stats.mu.RUnlock()

	// Return a copy of the stats
	return GCSafetyStats{
		TotalObjects:        gsm.stats.TotalObjects,
		ActiveObjects:       gsm.stats.ActiveObjects,
		ReleasedObjects:     gsm.stats.ReleasedObjects,
		InvalidatedObjects:  gsm.stats.InvalidatedObjects,
		FinalizersSet:       gsm.stats.FinalizersSet,
		FinalizersCalled:    gsm.stats.FinalizersCalled,
		FinalizerFailures:   gsm.stats.FinalizerFailures,
		FinalizerTimeouts:   gsm.stats.FinalizerTimeouts,
		StateChecksPassed:   gsm.stats.StateChecksPassed,
		StateChecksFailed:   gsm.stats.StateChecksFailed,
		UnsafeAccessBlocked: gsm.stats.UnsafeAccessBlocked,
		CleanupRuns:         gsm.stats.CleanupRuns,
		ObjectsCleaned:      gsm.stats.ObjectsCleaned,
		LastCleanup:         gsm.stats.LastCleanup,
	}
}

// getObjectID gets a unique ID for an object
func (gsm *GCSafetyManager) getObjectID(obj interface{}) uintptr {
	// Use reflection to get the object's memory address
	v := reflect.ValueOf(obj)
	if v.Kind() == reflect.Ptr && !v.IsNil() {
		return v.Pointer()
	}
	// For non-pointer types, generate a unique ID
	return uintptr(time.Now().UnixNano())
}

// startCleanupRoutine starts the background cleanup routine
func (gsm *GCSafetyManager) startCleanupRoutine() {
	gsm.cleanupTicker = time.NewTicker(gsm.config.CleanupInterval)

	go func() {
		defer gsm.cleanupTicker.Stop()

		for {
			select {
			case <-gsm.cleanupTicker.C:
				gsm.performCleanup()
			case <-gsm.stopCleanup:
				return
			}
		}
	}()
}

// performCleanup removes old and invalid objects from tracking
func (gsm *GCSafetyManager) performCleanup() {
	gsm.stateMutex.Lock()
	defer gsm.stateMutex.Unlock()

	now := time.Now()
	cleaned := 0

	for id, state := range gsm.objectStates {
		shouldCleanup := false

		// Clean up released objects that are old enough
		if atomic.LoadInt32(&state.IsReleased) == 1 &&
			now.Sub(state.LastAccess) > gsm.config.StateRetention {
			shouldCleanup = true
		}

		// Clean up invalid objects that are old enough
		if atomic.LoadInt32(&state.IsValid) == 0 &&
			now.Sub(state.LastAccess) > gsm.config.StateRetention {
			shouldCleanup = true
		}

		if shouldCleanup {
			// Execute cleanup callbacks before removing
			gsm.ExecuteCleanupCallbacks(id)

			delete(gsm.objectStates, id)
			cleaned++

			if gsm.config.LogStateChanges {
				gsm.logger.Debugf("Cleaned up object state: %s ID=%x (age: %v)",
					state.Type, id, now.Sub(state.CreatedAt))
			}
		}
	}

	// Update statistics
	gsm.stats.mu.Lock()
	gsm.stats.CleanupRuns++
	gsm.stats.ObjectsCleaned += int64(cleaned)
	gsm.stats.LastCleanup = now
	gsm.stats.mu.Unlock()

	if cleaned > 0 {
		gsm.logger.Debugf("Cleanup completed: removed %d object states", cleaned)
	}
}

// Close shuts down the GC safety manager
func (gsm *GCSafetyManager) Close() {
	gsm.logger.Info("Shutting down GC safety manager")

	// Set shutdown flag
	atomic.StoreInt32(&gsm.isShutdown, 1)

	// Stop cleanup routine
	close(gsm.stopCleanup)

	// Final cleanup - collect IDs first to avoid deadlock
	gsm.stateMutex.Lock()
	var activeObjectIDs []uintptr
	for id, state := range gsm.objectStates {
		if atomic.LoadInt32(&state.IsValid) == 1 {
			activeObjectIDs = append(activeObjectIDs, id)
			// Mark as invalid
			atomic.StoreInt32(&state.IsValid, 0)
			atomic.StoreInt32(&state.IsReleased, 1)
		}
	}

	// Clear the objects map
	gsm.objectStates = make(map[uintptr]*ObjectState)
	gsm.stateMutex.Unlock()

	// Execute cleanup callbacks outside of the lock to avoid deadlock
	for _, id := range activeObjectIDs {
		gsm.ExecuteCleanupCallbacks(id)
	}

	activeObjects := len(activeObjectIDs)

	// Log final statistics
	stats := gsm.GetStats()
	gsm.logger.Infof("GC safety manager shutdown complete - "+
		"Total objects: %d, Active at shutdown: %d, Finalizers called: %d, Failures: %d",
		stats.TotalObjects, activeObjects, stats.FinalizersCalled, stats.FinalizerFailures)
}

// SafeGObjectCleanup safely handles GObject cleanup to prevent segfaults
// This method should be called before any GObject is unreferenced
func (gsm *GCSafetyManager) SafeGObjectCleanup(objPtr unsafe.Pointer) bool {
	if objPtr == nil {
		return false
	}

	gsm.gobjectMutex.Lock()
	defer gsm.gobjectMutex.Unlock()

	// Check if this GObject has already been cleaned up
	if gsm.gobjectRefs == nil {
		gsm.gobjectRefs = make(map[unsafe.Pointer]bool)
	}

	if gsm.gobjectRefs[objPtr] {
		// Already cleaned up, don't try to unref again
		if gsm.config.LogStateChanges {
			gsm.logger.Debugf("GObject already cleaned up, skipping unref: %p", objPtr)
		}
		return false
	}

	// Mark as cleaned up
	gsm.gobjectRefs[objPtr] = true
	return true
}

// ClearGObjectRef removes a GObject reference from tracking
func (gsm *GCSafetyManager) ClearGObjectRef(objPtr unsafe.Pointer) {
	if objPtr == nil {
		return
	}

	gsm.gobjectMutex.Lock()
	defer gsm.gobjectMutex.Unlock()

	if gsm.gobjectRefs != nil {
		delete(gsm.gobjectRefs, objPtr)
	}
}
