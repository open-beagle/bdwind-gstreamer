# GStreamer Unified Error Handling System

This document describes the unified error handling mechanism implemented for GStreamer components in the go-gst refactor project.

## Overview

The unified error handling system provides:

- **Structured Error Types**: Hierarchical categorization of GStreamer-related errors
- **Severity Levels**: Clear severity classification from Info to Fatal
- **Recovery Mechanisms**: Automatic error recovery with configurable strategies
- **Error Aggregation**: Collection and analysis of multiple errors
- **Detailed Context**: Rich error information including stack traces and metadata
- **Component Isolation**: Prevents error propagation between components
- **Monitoring Integration**: Comprehensive metrics and logging

## Core Components

### 1. GStreamerError

The main error type that wraps all GStreamer-related errors:

```go
type GStreamerError struct {
    Type        GStreamerErrorType     // Category of error
    Severity    GStreamerErrorSeverity // Severity level
    Code        string                 // Unique error code
    Message     string                 // Human-readable message
    Details     string                 // Additional details
    Cause       error                  // Underlying error
    Component   string                 // Component that generated the error
    Operation   string                 // Operation being performed
    Timestamp   time.Time              // When the error occurred
    Recoverable bool                   // Whether error can be recovered
    Suggested   []RecoveryAction       // Suggested recovery actions
    Metadata    map[string]interface{} // Additional context
}
```

### 2. Error Types

Hierarchical categorization of errors:

- `ErrorTypeInitialization` - GStreamer initialization failures
- `ErrorTypePipelineCreation` - Pipeline creation errors
- `ErrorTypePipelineState` - Pipeline state transition errors
- `ErrorTypeElementCreation` - Element creation failures
- `ErrorTypeElementLinking` - Element linking problems
- `ErrorTypeElementProperty` - Property setting issues
- `ErrorTypeDataFlow` - Data flow interruptions
- `ErrorTypeMemoryAllocation` - Memory allocation failures
- `ErrorTypeResourceAccess` - Resource access problems
- `ErrorTypeFormatNegotiation` - Format negotiation failures
- `ErrorTypeTimeout` - Operation timeouts
- `ErrorTypePermission` - Permission denied errors
- `ErrorTypeNetwork` - Network-related errors
- `ErrorTypeHardware` - Hardware access failures
- `ErrorTypeConfiguration` - Configuration errors
- `ErrorTypeValidation` - Input validation failures

### 3. Severity Levels

- `UnifiedSeverityInfo` - Informational messages
- `UnifiedSeverityWarning` - Warning conditions
- `UnifiedSeverityError` - Error conditions
- `UnifiedSeverityCritical` - Critical errors requiring immediate attention
- `UnifiedSeverityFatal` - Fatal errors that prevent operation

### 4. Recovery Actions

- `RecoveryRetry` - Retry the failed operation
- `RecoveryRestart` - Restart the component
- `RecoveryReconfigure` - Reconfigure the component
- `RecoveryFallback` - Use alternative configuration
- `RecoveryManualIntervention` - Requires manual intervention

## Usage Examples

### Basic Error Creation

```go
// Create a simple error
err := &GStreamerError{
    Type:      ErrorTypeElementCreation,
    Severity:  UnifiedSeverityError,
    Component: "encoder",
    Message:   "Failed to create x264enc element",
    Timestamp: time.Now(),
}
```

### Using Convenience Functions

```go
// Initialization error
initErr := NewInitializationError("gstreamer", "Failed to initialize", cause)

// Pipeline error
pipelineErr := NewPipelineError(ErrorTypePipelineCreation, "pipeline", 
    "create", "Pipeline creation failed", cause)

// Element error
elementErr := NewElementError(ErrorTypeElementCreation, "encoder", 
    "x264enc", "Element creation failed", cause)

// Memory error
memErr := NewMemoryError("buffer-pool", "allocate", 1024*1024, cause)

// Timeout error
timeoutErr := NewTimeoutError("pipeline", "state-change", 5*time.Second, cause)

// Validation error
validationErr := NewValidationError("config", "width", "Invalid width", -1)
```

### Error Handler Usage

```go
// Create error handler
logger := logrus.WithField("component", "gstreamer")
config := &UnifiedErrorHandlerConfig{
    MaxErrorHistory:           1000,
    MaxRecoveryAttempts:       3,
    AutoRecovery:              true,
    ComponentRestartThreshold: 5,
}
errorHandler := NewErrorHandler(logger, config)

// Handle an error
ctx := context.Background()
err := errorHandler.HandleError(ctx, ErrorTypeElementCreation, 
    "encoder", "create-element", "Failed to create encoder", cause)

// Get error history
history := errorHandler.GetErrorHistory()

// Get component-specific errors
componentErrors := errorHandler.GetComponentErrors("encoder")

// Get metrics
metrics := errorHandler.GetMetrics()
```

### Error Checking Utilities

```go
// Check if error is a GStreamer error
if IsGStreamerError(err) {
    gstErr, _ := GetGStreamerError(err)
    fmt.Printf("GStreamer error: %s", gstErr.Type.String())
}

// Check error type
if IsErrorType(err, ErrorTypeElementCreation) {
    fmt.Println("Element creation error")
}

// Check severity
if IsErrorSeverity(err, UnifiedSeverityCritical) {
    fmt.Println("Critical error")
}

// Check if critical
if IsCriticalError(err) {
    fmt.Println("This is a critical error")
}

// Check if recoverable
if IsRecoverableError(err) {
    fmt.Println("This error can be recovered")
}
```

### Error Aggregation

```go
// Create aggregator
aggregator := NewErrorAggregator()

// Add errors
aggregator.Add(err1)
aggregator.Add(err2)

// Check for errors
if aggregator.HasErrors() {
    fmt.Println("Errors detected")
}

if aggregator.HasCriticalErrors() {
    fmt.Println("Critical errors detected")
}

// Get summary
summary := aggregator.GetSummary()
fmt.Printf("Total errors: %d", summary["total_errors"])
```

### Global Error Handler

```go
// Set global handler
SetGlobalErrorHandler(errorHandler)

// Use global handler
err := HandleGlobalError(ctx, ErrorTypeElementCreation, 
    "encoder", "create", "Failed", cause)
```

## Integration with Components

### Desktop Capture Component

```go
func (dc *DesktopCaptureGst) handleCaptureError(operation string, err error) error {
    return dc.errorHandler.HandleError(context.Background(), 
        ErrorTypeResourceAccess, "desktop-capture", operation, 
        "Capture operation failed", err)
}
```

### Encoder Component

```go
func (e *EncoderGst) handleEncodingError(operation string, err error) error {
    return e.errorHandler.HandleError(context.Background(), 
        ErrorTypeElementCreation, "encoder", operation, 
        "Encoding operation failed", err)
}
```

### Pipeline Management

```go
func (p *PipelineWrapper) handlePipelineError(operation string, err error) error {
    return p.errorHandler.HandleError(context.Background(), 
        ErrorTypePipelineState, "pipeline", operation, 
        "Pipeline operation failed", err)
}
```

## Configuration

### Error Handler Configuration

```go
config := &UnifiedErrorHandlerConfig{
    // Maximum number of errors to keep in history
    MaxErrorHistory: 1000,
    
    // Maximum recovery attempts per error
    MaxRecoveryAttempts: 3,
    
    // Base delay between recovery attempts
    BaseRecoveryDelay: time.Second,
    
    // Maximum delay between recovery attempts
    MaxRecoveryDelay: 30 * time.Second,
    
    // Enable automatic recovery
    AutoRecovery: true,
    
    // Enable error propagation to parent components
    PropagateErrors: false,
    
    // Component restart threshold
    ComponentRestartThreshold: 5,
    
    // Error escalation timeout
    EscalationTimeout: 5 * time.Minute,
    
    // Enable detailed stack traces
    EnableStackTraces: true,
}
```

### Recovery Strategies

```go
// Custom recovery strategy
strategy := RecoveryStrategy{
    MaxRetries:     5,
    RetryDelay:     2 * time.Second,
    Actions:        []RecoveryAction{RecoveryRetry, RecoveryFallback},
    AutoRecover:    true,
    EscalationTime: 60 * time.Second,
}

// Set custom strategy
errorHandler.SetRecoveryStrategy(ErrorTypeElementCreation, strategy)
```

## Monitoring and Metrics

### Error Metrics

The error handler provides comprehensive metrics:

```go
metrics := errorHandler.GetMetrics()

// Total error counts
fmt.Printf("Total errors: %d", metrics.TotalErrors)

// Errors by type
for errorType, count := range metrics.ErrorsByType {
    fmt.Printf("%s: %d", errorType, count)
}

// Errors by severity
for severity, count := range metrics.ErrorsBySeverity {
    fmt.Printf("%s: %d", severity, count)
}

// Recovery statistics
fmt.Printf("Recovery attempts: %d", metrics.RecoveryAttempts)
fmt.Printf("Recovery successes: %d", metrics.RecoverySuccesses)
fmt.Printf("Recovery failures: %d", metrics.RecoveryFailures)
```

### Event Handlers

```go
// Add custom error event handler
errorHandler.AddErrorHandler(func(err *GStreamerError) bool {
    if err.IsCritical() {
        // Send alert to monitoring system
        sendAlert(err)
    }
    return true // Continue processing
})
```

## Best Practices

### 1. Error Context

Always provide rich context when creating errors:

```go
err := NewElementError(ErrorTypeElementCreation, "encoder", elementName, 
    fmt.Sprintf("Failed to create %s element", elementName), cause)
err.Metadata["factory_name"] = factoryName
err.Metadata["caps"] = capsString
```

### 2. Component Isolation

Handle errors at component boundaries to prevent propagation:

```go
func (c *Component) processData() error {
    if err := c.internalOperation(); err != nil {
        // Handle internally, don't propagate
        return c.errorHandler.HandleError(ctx, ErrorTypeDataFlow, 
            c.name, "process", "Data processing failed", err)
    }
    return nil
}
```

### 3. Recovery Strategies

Define appropriate recovery strategies for each error type:

```go
// For transient errors
strategy := RecoveryStrategy{
    MaxRetries:  5,
    RetryDelay:  time.Second,
    Actions:     []RecoveryAction{RecoveryRetry},
    AutoRecover: true,
}

// For configuration errors
strategy := RecoveryStrategy{
    MaxRetries:  2,
    RetryDelay:  time.Second,
    Actions:     []RecoveryAction{RecoveryReconfigure, RecoveryFallback},
    AutoRecover: true,
}
```

### 4. Monitoring Integration

Integrate with monitoring systems for critical errors:

```go
errorHandler.AddErrorHandler(func(err *GStreamerError) bool {
    if err.Severity >= UnifiedSeverityCritical {
        metrics.IncrementCounter("gstreamer.critical_errors", 
            map[string]string{
                "component": err.Component,
                "type":      err.Type.String(),
            })
    }
    return true
})
```

## Requirements Mapping

This implementation addresses the following requirements from the specification:

- **4.1**: Unified error handling across all GStreamer components
- **4.2**: Automatic resource cleanup and error recovery
- **4.3**: Prevention of error propagation between components
- **5.1**: Detailed error logging with context and stack traces
- **5.3**: Performance monitoring and error metrics

The unified error handling mechanism ensures that errors are properly categorized, logged, and handled consistently across all GStreamer components, providing a robust foundation for the go-gst refactor project.