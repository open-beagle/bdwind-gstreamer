# GStreamer Object Lifecycle Manager

## Overview

The ObjectLifecycleManager is a comprehensive solution for managing GStreamer object lifecycles to prevent segmentation faults during garbage collection. This implementation addresses the core issue identified in task 3 of the GStreamer segfault fix specification.

## Problem Addressed

The original segfault occurred in the `BufferPool.GetBuffer` method during garbage collection, specifically when go-glib attempted to finalize GStreamer objects that were no longer valid. The root cause was improper lifecycle management of GStreamer objects.

## Solution Components

### 1. ObjectLifecycleManager (`object_lifecycle_manager.go`)

The core component that tracks GStreamer object lifecycles:

**Key Features:**
- **Object Registration**: Tracks GStreamer objects (Pipeline, Element, Bus, Sample) with unique IDs
- **Reference Counting**: Manages reference counts to prevent premature cleanup
- **Validation**: Provides object validity checking to prevent access to freed objects
- **Automatic Cleanup**: Background cleanup of old and invalid objects
- **Statistics**: Comprehensive tracking of object lifecycle statistics

**Main Methods:**
- `RegisterObject()`: Register a GStreamer object for tracking
- `UnregisterObject()`: Unregister an object (decrements ref count)
- `IsValidObject()`: Check if an object is still valid
- `ValidateGstObject()`: Type-specific validation for GStreamer objects
- `ForceInvalidateObject()`: Forcefully mark an object as invalid

### 2. GstObjectTracker (`object_lifecycle_helpers.go`)

A convenient wrapper around ObjectLifecycleManager:

**Key Features:**
- **Type-Safe Tracking**: Specific methods for different GStreamer object types
- **Safe Usage Patterns**: `SafeUse*` methods that validate objects before use
- **Health Monitoring**: Health status reporting and statistics
- **Configuration**: Easy configuration of lifecycle management settings

**Main Methods:**
- `TrackPipeline()`, `TrackElement()`, `TrackBus()`, `TrackSample()`: Type-specific tracking
- `SafeUsePipeline()`, `SafeUseElement()`, etc.: Safe object usage patterns
- `GetHealthStatus()`: Overall health reporting
- `EnableDebugLogging()`, `EnableStackTrace()`: Configuration options

### 3. Integration Example (`object_lifecycle_integration_example.go`)

A complete example showing how to integrate the ObjectLifecycleManager with existing GStreamer components:

**Features Demonstrated:**
- Safe pipeline creation and management
- Object tracking throughout the component lifecycle
- Error recovery mechanisms
- Health monitoring and statistics
- Proper cleanup procedures

## Usage Examples

### Basic Usage

```go
// Create object tracker
tracker := NewGstObjectTracker()
defer tracker.Close()

// Create and track a pipeline
pipeline, err := gst.NewPipeline("my-pipeline")
if err != nil {
    return err
}

pipelineID, err := tracker.TrackPipeline(pipeline, "my-pipeline")
if err != nil {
    return err
}

// Use pipeline safely
err = tracker.SafeUsePipeline(pipelineID, func(p *gst.Pipeline) error {
    // Safe to use pipeline here
    return p.SetState(gst.StatePlaying)
})

// Untrack when done
tracker.UntrackObject(pipelineID)
```

### Advanced Usage with Custom Configuration

```go
config := ObjectLifecycleConfig{
    CleanupInterval:    30 * time.Second,
    ObjectRetention:    5 * time.Minute,
    EnableValidation:   true,
    ValidationInterval: 60 * time.Second,
    EnableStackTrace:   true,  // For debugging
    LogObjectEvents:    true,  // For debugging
}

tracker := NewGstObjectTrackerWithConfig(config)
defer tracker.Close()

// Enable additional debugging
tracker.EnableDebugLogging(true)
tracker.EnableStackTrace(true)
```

## Key Benefits

1. **Segfault Prevention**: Prevents access to freed GStreamer objects during garbage collection
2. **Memory Safety**: Ensures objects are valid before use
3. **Automatic Cleanup**: Background cleanup prevents memory leaks
4. **Debugging Support**: Comprehensive logging and statistics for troubleshooting
5. **Easy Integration**: Simple API that can be integrated into existing components
6. **Health Monitoring**: Real-time health status and statistics
7. **Configurable**: Flexible configuration for different use cases

## Configuration Options

- `CleanupInterval`: How often to run cleanup (default: 30s)
- `ObjectRetention`: How long to keep released object info (default: 5m)
- `EnableValidation`: Enable periodic object validation (default: true)
- `ValidationInterval`: How often to validate objects (default: 60s)
- `EnableStackTrace`: Capture stack traces for debugging (default: false)
- `LogObjectEvents`: Log object lifecycle events (default: false)

## Statistics and Monitoring

The ObjectLifecycleManager provides comprehensive statistics:

- **Object Counts**: Total registered, currently active, released, invalid
- **Cleanup Stats**: Number of cleanup runs, objects cleaned
- **Validation Stats**: Validation runs, validation errors
- **Health Status**: Overall system health assessment

## Testing

The implementation includes comprehensive tests:

- **Unit Tests**: `object_lifecycle_manager_test.go` - Core functionality tests
- **Helper Tests**: `object_lifecycle_helpers_test.go` - Wrapper functionality tests  
- **Integration Tests**: `object_lifecycle_integration_example_test.go` - End-to-end tests

All tests pass successfully and demonstrate the robustness of the implementation.

## Requirements Satisfied

This implementation satisfies all requirements from task 3:

✅ **3.1**: Created ObjectLifecycleManager to track object states  
✅ **3.2**: Objects are registered on creation and unregistered on destruction  
✅ **3.3**: Added comprehensive object validity validation methods  

The implementation goes beyond the basic requirements by providing:
- Type-safe wrappers for different GStreamer object types
- Safe usage patterns that prevent segfaults
- Comprehensive health monitoring and statistics
- Configurable behavior for different use cases
- Complete integration examples and documentation

## Integration with Existing Code

The ObjectLifecycleManager can be easily integrated into existing GStreamer components:

1. **Replace direct GStreamer object usage** with tracked objects
2. **Use safe usage patterns** (`SafeUse*` methods) instead of direct access
3. **Add health monitoring** to detect and recover from object lifecycle issues
4. **Enable debugging** during development to identify lifecycle problems

This implementation provides a robust foundation for preventing GStreamer-related segfaults while maintaining performance and usability.