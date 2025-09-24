# GC Safety Mechanisms for GStreamer Objects

This document describes the garbage collection safety mechanisms implemented to prevent segfaults during GStreamer object lifecycle management.

## Overview

The GC safety system consists of three main components:

1. **GCSafetyManager**: Core safety manager that tracks object states and prevents unsafe access
2. **Enhanced MemoryManager**: Integrates GC safety into memory management operations
3. **Enhanced ObjectLifecycleManager**: Integrates GC safety into object lifecycle tracking

## Key Features

### 1. Object State Tracking

- Tracks object validity using atomic flags
- Prevents access to released or invalid objects
- Maintains reference counts safely

### 2. Safe Object Access

- Provides `SafeAccess` methods that check object validity before execution
- Includes panic recovery to prevent crashes
- Automatically invalidates objects after panics

### 3. Finalizer Safety

- Ensures finalizers don't access already released objects
- Prevents double finalization
- Includes timeout protection for finalizer execution

### 4. Cleanup Callbacks

- Allows registration of cleanup callbacks for objects
- Executes callbacks safely with panic recovery
- Prevents circular dependencies during cleanup

## Usage Examples

### Basic Object Tracking

```go
// Create GC safety manager
config := GCSafetyConfig{
    EnableStateChecking:   true,
    EnableFinalizerSafety: true,
}
gsm := NewGCSafetyManager(config)
defer gsm.Close()

// Register an object
obj := &MyObject{}
id, err := gsm.RegisterObject(obj, "MyObject")
if err != nil {
    return err
}

// Safe access to object
err = gsm.SafeAccess(id, func() error {
    // Access object safely here
    return obj.DoSomething()
})

// Unregister when done
gsm.UnregisterObject(id)
```

### Memory Manager Integration

```go
// Create memory manager with GC safety
config := MemoryManagerConfig{
    TrackObjectLifecycle: true,
    // ... other config
}
mm := NewMemoryManager(config)
defer mm.Close()

// Track object with GC safety
id := mm.TrackObject(obj, "MyObject", 1024)

// Safe object access
err := mm.SafeObjectAccess(id, func() error {
    // Access object safely
    return nil
})

// Release object
mm.ReleaseObject(id)
```

### Object Lifecycle Manager Integration

```go
// Create lifecycle manager with GC safety
config := ObjectLifecycleConfig{
    EnableValidation: true,
    // ... other config
}
olm := NewObjectLifecycleManager(config)
defer olm.Close()

// Register object with GC safety
id, err := olm.RegisterObject(obj, "MyObject", "instance1")
if err != nil {
    return err
}

// Safe object access
err = olm.SafeObjectAccess(id, func() error {
    // Access object safely
    return nil
})

// Unregister when done
olm.UnregisterObject(id)
```

## Configuration Options

### GCSafetyConfig

- `CleanupInterval`: How often to clean up old object states
- `StateRetention`: How long to keep object state after release
- `FinalizerTimeout`: Timeout for finalizer execution
- `EnableStateChecking`: Enable/disable state validation
- `EnableFinalizerSafety`: Enable/disable safe finalizers
- `MaxFinalizerRetries`: Maximum retries for failed finalizers
- `LogStateChanges`: Enable/disable state change logging
- `LogFinalizers`: Enable/disable finalizer logging

## Safety Guarantees

### 1. No Access to Released Objects

The system prevents access to objects that have been released or invalidated:

```go
// Object is released
gsm.UnregisterObject(id)

// This will fail safely
err := gsm.SafeAccess(id, func() error {
    // This code won't execute
    return nil
})
// err will contain "not valid for access"
```

### 2. Panic Recovery

If code panics during object access, the system recovers and invalidates the object:

```go
err := gsm.SafeAccess(id, func() error {
    panic("something went wrong")
})
// err will contain "panic during object access"
// Object is automatically invalidated
```

### 3. Safe Finalizers

Finalizers are wrapped to prevent access to invalid objects:

```go
err := gsm.SetSafeFinalizer(obj, func() {
    // This finalizer will only run if the object is still valid
    // and will be protected by timeout
})
```

### 4. Concurrent Safety

All operations are thread-safe and can be called concurrently:

```go
// Multiple goroutines can safely access the same object
go func() {
    gsm.SafeAccess(id, func() error { /* ... */ })
}()
go func() {
    gsm.SafeAccess(id, func() error { /* ... */ })
}()
```

## Integration with Existing Components

### Desktop Capture Integration

The desktop capture component uses GC safety for sample processing:

```go
// Safe sample conversion with lifecycle tracking
func (dc *DesktopCaptureGst) convertGstSample(gstSample *gst.Sample) (*Sample, error) {
    // Register sample with lifecycle manager
    sampleID, err := dc.lifecycleManager.RegisterObject(gstSample, "gst.Sample", "capture-sample")
    if err != nil {
        return nil, err
    }
    defer dc.lifecycleManager.UnregisterObject(sampleID)

    // Use safe access for sample processing
    var result *Sample
    err = dc.lifecycleManager.SafeObjectAccess(sampleID, func() error {
        // Process sample safely
        result, err = dc.processSample(gstSample)
        return err
    })

    return result, err
}
```

### Buffer Pool Safety

The buffer pool includes safety mechanisms to prevent crashes:

```go
// Safe buffer operations
func (bp *BufferPool) GetBuffer(size int) []byte {
    // Check if pool is closed
    if atomic.LoadInt32(&bp.closed) == 1 {
        return make([]byte, size)
    }

    // Use timeout to prevent blocking
    select {
    case buffer := <-bp.pool:
        return buffer
    case <-time.After(bp.timeout):
        return make([]byte, size)
    }
}
```

## Performance Considerations

### 1. Atomic Operations

The system uses atomic operations for state flags to minimize locking overhead:

```go
// Fast validity check without locks
func (gsm *GCSafetyManager) IsObjectValid(id uintptr) bool {
    state := gsm.getObjectState(id)
    return atomic.LoadInt32(&state.IsValid) == 1
}
```

### 2. Configurable Logging

Logging can be disabled for performance in production:

```go
config := GCSafetyConfig{
    LogStateChanges: false, // Disable for performance
    LogFinalizers:   false, // Disable for performance
}
```

### 3. Cleanup Intervals

Cleanup intervals can be tuned based on requirements:

```go
config := GCSafetyConfig{
    CleanupInterval: 30 * time.Second, // Adjust based on needs
    StateRetention:  5 * time.Minute,  // Adjust based on needs
}
```

## Troubleshooting

### Common Issues

1. **Deadlocks**: Avoid calling GC safety methods from within cleanup callbacks
2. **Memory Leaks**: Ensure all registered objects are eventually unregistered
3. **Performance**: Disable logging in production environments

### Debugging

Enable detailed logging to debug issues:

```go
config := GCSafetyConfig{
    LogStateChanges: true,
    LogFinalizers:   true,
}
```

### Statistics

Monitor GC safety statistics:

```go
stats := gsm.GetStats()
fmt.Printf("Total objects: %d, Active: %d, Finalizer failures: %d\n",
    stats.TotalObjects, stats.ActiveObjects, stats.FinalizerFailures)
```

## Testing

The system includes comprehensive tests:

- Unit tests for individual components
- Integration tests for component interaction
- Concurrent safety tests
- Panic recovery tests
- Shutdown safety tests

Run tests with:

```bash
go test -v ./internal/gstreamer -run TestGCSafety
```

## Future Improvements

1. **Weak References**: Implement weak reference support for better memory management
2. **Object Pools**: Add object pooling for frequently created/destroyed objects
3. **Metrics Integration**: Add Prometheus metrics for monitoring
4. **Performance Profiling**: Add detailed performance profiling support