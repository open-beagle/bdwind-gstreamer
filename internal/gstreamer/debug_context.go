package gstreamer

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// DebugContext provides enhanced debugging context for GStreamer operations
type DebugContext struct {
	logger *logrus.Entry
	mu     sync.RWMutex

	// Context tracking
	operationID   string
	component     string
	operation     string
	startTime     time.Time
	parentContext *DebugContext

	// Debug information
	debugData   map[string]interface{}
	breadcrumbs []DebugBreadcrumb
	stackTrace  []string
	metadata    map[string]interface{}

	// Performance tracking
	checkpoints []PerformanceCheckpoint
	memoryUsage []MemorySnapshot

	// Error context
	errorHistory []ContextualError
}

// DebugBreadcrumb represents a debug breadcrumb for operation tracking
type DebugBreadcrumb struct {
	Timestamp  time.Time
	Operation  string
	Component  string
	Message    string
	Level      logrus.Level
	Data       map[string]interface{}
	StackFrame string
}

// PerformanceCheckpoint represents a performance measurement point
type PerformanceCheckpoint struct {
	Name      string
	Timestamp time.Time
	Duration  time.Duration
	Memory    uint64
	Metadata  map[string]interface{}
}

// MemorySnapshot represents a memory usage snapshot
type MemorySnapshot struct {
	Timestamp     time.Time
	AllocBytes    uint64
	TotalAlloc    uint64
	SysBytes      uint64
	NumGC         uint32
	GCCPUFraction float64
}

// ContextualError represents an error with full debug context
type ContextualError struct {
	Error       error
	Timestamp   time.Time
	Component   string
	Operation   string
	StackTrace  []string
	Breadcrumbs []DebugBreadcrumb
	Metadata    map[string]interface{}
}

// NewDebugContext creates a new debug context
func NewDebugContext(logger *logrus.Entry, component, operation string) *DebugContext {
	if logger == nil {
		logger = logrus.WithField("component", "debug-context")
	}

	operationID := fmt.Sprintf("%s_%s_%d", component, operation, time.Now().UnixNano())

	ctx := &DebugContext{
		logger:       logger.WithField("operation_id", operationID),
		operationID:  operationID,
		component:    component,
		operation:    operation,
		startTime:    time.Now(),
		debugData:    make(map[string]interface{}),
		breadcrumbs:  make([]DebugBreadcrumb, 0),
		metadata:     make(map[string]interface{}),
		checkpoints:  make([]PerformanceCheckpoint, 0),
		memoryUsage:  make([]MemorySnapshot, 0),
		errorHistory: make([]ContextualError, 0),
	}

	// Capture initial stack trace
	ctx.stackTrace = ctx.captureStackTrace(3) // Skip NewDebugContext, caller, and runtime

	// Add initial breadcrumb
	ctx.AddBreadcrumb("context_created", fmt.Sprintf("Debug context created for %s.%s", component, operation), logrus.InfoLevel, nil)

	// Take initial memory snapshot
	ctx.TakeMemorySnapshot()

	ctx.logger.WithFields(logrus.Fields{
		"component":  component,
		"operation":  operation,
		"start_time": ctx.startTime,
	}).Debug("Debug context created")

	return ctx
}

// NewChildContext creates a child debug context
func (dc *DebugContext) NewChildContext(component, operation string) *DebugContext {
	child := NewDebugContext(dc.logger, component, operation)
	child.parentContext = dc

	// Inherit some context from parent
	child.metadata["parent_operation_id"] = dc.operationID
	child.metadata["parent_component"] = dc.component
	child.metadata["parent_operation"] = dc.operation

	dc.AddBreadcrumb("child_context_created",
		fmt.Sprintf("Created child context for %s.%s", component, operation),
		logrus.DebugLevel,
		map[string]interface{}{
			"child_operation_id": child.operationID,
		})

	return child
}

// AddBreadcrumb adds a debug breadcrumb
func (dc *DebugContext) AddBreadcrumb(operation, message string, level logrus.Level, data map[string]interface{}) {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	// Get caller information
	_, file, line, ok := runtime.Caller(1)
	stackFrame := "unknown"
	if ok {
		stackFrame = fmt.Sprintf("%s:%d", file, line)
	}

	breadcrumb := DebugBreadcrumb{
		Timestamp:  time.Now(),
		Operation:  operation,
		Component:  dc.component,
		Message:    message,
		Level:      level,
		Data:       data,
		StackFrame: stackFrame,
	}

	dc.breadcrumbs = append(dc.breadcrumbs, breadcrumb)

	// Log the breadcrumb
	logEntry := dc.logger.WithFields(logrus.Fields{
		"breadcrumb_operation": operation,
		"stack_frame":          stackFrame,
	})

	if data != nil {
		for key, value := range data {
			logEntry = logEntry.WithField(fmt.Sprintf("data_%s", key), value)
		}
	}

	logEntry.Log(level, message)
}

// SetDebugData sets debug data for the context
func (dc *DebugContext) SetDebugData(key string, value interface{}) {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	dc.debugData[key] = value
	dc.logger.WithField(key, value).Debug("Debug data updated")
}

// GetDebugData gets debug data from the context
func (dc *DebugContext) GetDebugData(key string) (interface{}, bool) {
	dc.mu.RLock()
	defer dc.mu.RUnlock()

	value, exists := dc.debugData[key]
	return value, exists
}

// SetMetadata sets metadata for the context
func (dc *DebugContext) SetMetadata(key string, value interface{}) {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	dc.metadata[key] = value
}

// AddPerformanceCheckpoint adds a performance checkpoint
func (dc *DebugContext) AddPerformanceCheckpoint(name string, metadata map[string]interface{}) {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	now := time.Now()
	duration := now.Sub(dc.startTime)

	// Get memory stats
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	checkpoint := PerformanceCheckpoint{
		Name:      name,
		Timestamp: now,
		Duration:  duration,
		Memory:    m.Alloc,
		Metadata:  metadata,
	}

	dc.checkpoints = append(dc.checkpoints, checkpoint)

	dc.logger.WithFields(logrus.Fields{
		"checkpoint_name": name,
		"duration":        duration,
		"memory_alloc":    m.Alloc,
	}).Debug("Performance checkpoint added")
}

// TakeMemorySnapshot takes a memory usage snapshot
func (dc *DebugContext) TakeMemorySnapshot() {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	snapshot := MemorySnapshot{
		Timestamp:     time.Now(),
		AllocBytes:    m.Alloc,
		TotalAlloc:    m.TotalAlloc,
		SysBytes:      m.Sys,
		NumGC:         m.NumGC,
		GCCPUFraction: m.GCCPUFraction,
	}

	dc.memoryUsage = append(dc.memoryUsage, snapshot)

	dc.logger.WithFields(logrus.Fields{
		"alloc_bytes":     m.Alloc,
		"total_alloc":     m.TotalAlloc,
		"sys_bytes":       m.Sys,
		"num_gc":          m.NumGC,
		"gc_cpu_fraction": m.GCCPUFraction,
	}).Debug("Memory snapshot taken")
}

// RecordError records an error with full context
func (dc *DebugContext) RecordError(err error, operation string, metadata map[string]interface{}) {
	if err == nil {
		return
	}

	dc.mu.Lock()
	defer dc.mu.Unlock()

	contextualError := ContextualError{
		Error:       err,
		Timestamp:   time.Now(),
		Component:   dc.component,
		Operation:   operation,
		StackTrace:  dc.captureStackTrace(2), // Skip RecordError and caller
		Breadcrumbs: make([]DebugBreadcrumb, len(dc.breadcrumbs)),
		Metadata:    metadata,
	}

	// Copy breadcrumbs
	copy(contextualError.Breadcrumbs, dc.breadcrumbs)

	dc.errorHistory = append(dc.errorHistory, contextualError)

	// Log the error with full context
	dc.logger.WithFields(logrus.Fields{
		"error":            err.Error(),
		"error_operation":  operation,
		"breadcrumb_count": len(dc.breadcrumbs),
		"stack_depth":      len(contextualError.StackTrace),
	}).Error("Error recorded with debug context")
}

// GetSummary returns a summary of the debug context
func (dc *DebugContext) GetSummary() map[string]interface{} {
	dc.mu.RLock()
	defer dc.mu.RUnlock()

	duration := time.Since(dc.startTime)

	summary := map[string]interface{}{
		"operation_id":          dc.operationID,
		"component":             dc.component,
		"operation":             dc.operation,
		"start_time":            dc.startTime,
		"duration":              duration,
		"breadcrumb_count":      len(dc.breadcrumbs),
		"checkpoint_count":      len(dc.checkpoints),
		"memory_snapshot_count": len(dc.memoryUsage),
		"error_count":           len(dc.errorHistory),
		"debug_data_keys":       len(dc.debugData),
		"metadata_keys":         len(dc.metadata),
	}

	if dc.parentContext != nil {
		summary["parent_operation_id"] = dc.parentContext.operationID
	}

	// Add latest memory info if available
	if len(dc.memoryUsage) > 0 {
		latest := dc.memoryUsage[len(dc.memoryUsage)-1]
		summary["latest_memory_alloc"] = latest.AllocBytes
		summary["latest_memory_sys"] = latest.SysBytes
	}

	// Add latest checkpoint info if available
	if len(dc.checkpoints) > 0 {
		latest := dc.checkpoints[len(dc.checkpoints)-1]
		summary["latest_checkpoint"] = latest.Name
		summary["latest_checkpoint_duration"] = latest.Duration
	}

	return summary
}

// LogSummary logs a summary of the debug context
func (dc *DebugContext) LogSummary(level logrus.Level) {
	summary := dc.GetSummary()

	logEntry := dc.logger.WithFields(logrus.Fields{
		"operation_summary": true,
	})

	for key, value := range summary {
		logEntry = logEntry.WithField(key, value)
	}

	message := fmt.Sprintf("Debug context summary for %s.%s", dc.component, dc.operation)
	logEntry.Log(level, message)
}

// Close closes the debug context and logs final summary
func (dc *DebugContext) Close() {
	dc.AddBreadcrumb("context_closing", "Debug context closing", logrus.InfoLevel, nil)
	dc.TakeMemorySnapshot()
	dc.AddPerformanceCheckpoint("final", map[string]interface{}{
		"total_duration": time.Since(dc.startTime),
	})

	// Log final summary
	dc.LogSummary(logrus.InfoLevel)

	dc.logger.WithFields(logrus.Fields{
		"total_duration":    time.Since(dc.startTime),
		"final_breadcrumbs": len(dc.breadcrumbs),
		"final_errors":      len(dc.errorHistory),
	}).Info("Debug context closed")
}

// captureStackTrace captures the current stack trace
func (dc *DebugContext) captureStackTrace(skip int) []string {
	const maxDepth = 20
	pcs := make([]uintptr, maxDepth)
	n := runtime.Callers(skip, pcs)

	frames := runtime.CallersFrames(pcs[:n])
	var stackTrace []string

	for {
		frame, more := frames.Next()
		stackTrace = append(stackTrace, fmt.Sprintf("%s:%d %s", frame.File, frame.Line, frame.Function))
		if !more {
			break
		}
	}

	return stackTrace
}

// GetBreadcrumbs returns a copy of all breadcrumbs
func (dc *DebugContext) GetBreadcrumbs() []DebugBreadcrumb {
	dc.mu.RLock()
	defer dc.mu.RUnlock()

	breadcrumbs := make([]DebugBreadcrumb, len(dc.breadcrumbs))
	copy(breadcrumbs, dc.breadcrumbs)
	return breadcrumbs
}

// GetErrorHistory returns a copy of all recorded errors
func (dc *DebugContext) GetErrorHistory() []ContextualError {
	dc.mu.RLock()
	defer dc.mu.RUnlock()

	errors := make([]ContextualError, len(dc.errorHistory))
	copy(errors, dc.errorHistory)
	return errors
}

// GetPerformanceCheckpoints returns a copy of all performance checkpoints
func (dc *DebugContext) GetPerformanceCheckpoints() []PerformanceCheckpoint {
	dc.mu.RLock()
	defer dc.mu.RUnlock()

	checkpoints := make([]PerformanceCheckpoint, len(dc.checkpoints))
	copy(checkpoints, dc.checkpoints)
	return checkpoints
}

// FormatBreadcrumbsAsString formats breadcrumbs as a readable string
func (dc *DebugContext) FormatBreadcrumbsAsString() string {
	breadcrumbs := dc.GetBreadcrumbs()
	if len(breadcrumbs) == 0 {
		return "No breadcrumbs available"
	}

	var builder strings.Builder
	builder.WriteString("Debug Breadcrumbs:\n")

	for i, breadcrumb := range breadcrumbs {
		builder.WriteString(fmt.Sprintf("  %d. [%s] %s.%s: %s\n",
			i+1,
			breadcrumb.Timestamp.Format("15:04:05.000"),
			breadcrumb.Component,
			breadcrumb.Operation,
			breadcrumb.Message))

		if breadcrumb.Data != nil && len(breadcrumb.Data) > 0 {
			builder.WriteString("     Data: ")
			for key, value := range breadcrumb.Data {
				builder.WriteString(fmt.Sprintf("%s=%v ", key, value))
			}
			builder.WriteString("\n")
		}
	}

	return builder.String()
}

// WithContext creates a new context.Context with this debug context
func (dc *DebugContext) WithContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, "debug_context", dc)
}

// FromContext extracts a debug context from context.Context
func FromContext(ctx context.Context) (*DebugContext, bool) {
	if ctx == nil {
		return nil, false
	}

	debugCtx, ok := ctx.Value("debug_context").(*DebugContext)
	return debugCtx, ok
}
