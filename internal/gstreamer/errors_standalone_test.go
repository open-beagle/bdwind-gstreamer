package gstreamer

import (
	"context"
	"errors"
	"testing"

	"github.com/sirupsen/logrus"
)

// Simple test to verify error handling functionality without dependencies
func TestErrorHandlingStandalone(t *testing.T) {
	// Test GStreamerError creation
	err := &GStreamerError{
		Type:      ErrorTypePipelineCreation,
		Severity:  UnifiedSeverityError,
		Component: "test-component",
		Message:   "test error",
		Details:   "detailed error information",
	}

	if err.Error() == "" {
		t.Error("Error() method should return non-empty string")
	}

	if !err.IsRecoverable() {
		// This should be false since no suggested actions
		t.Log("Error is not recoverable as expected")
	}

	// Test error handler creation
	logger := logrus.NewEntry(logrus.New())
	logger.Logger.SetLevel(logrus.ErrorLevel) // Reduce noise

	eh := NewErrorHandler(logger, nil)
	if eh == nil {
		t.Fatal("NewErrorHandler should not return nil")
	}

	// Test basic error handling
	ctx := context.Background()
	handleErr := eh.HandleError(ctx, ErrorTypeElementCreation, "test-component",
		"create-element", "failed to create element", errors.New("underlying error"))

	if handleErr != nil {
		t.Errorf("HandleError should not return error by default: %v", handleErr)
	}

	// Check error history
	history := eh.GetErrorHistory()
	if len(history) != 1 {
		t.Errorf("Expected 1 error in history, got %d", len(history))
	}

	// Test convenience functions
	initErr := NewInitializationError("gstreamer", "init failed", nil)
	if initErr.Type != ErrorTypeInitialization {
		t.Error("NewInitializationError should create initialization error")
	}

	pipelineErr := NewPipelineError(ErrorTypePipelineCreation, "pipeline", "create", "failed", nil)
	if pipelineErr.Type != ErrorTypePipelineCreation {
		t.Error("NewPipelineError should create pipeline error")
	}

	// Test error utilities
	if !IsGStreamerError(initErr) {
		t.Error("IsGStreamerError should return true for GStreamer error")
	}

	if IsGStreamerError(errors.New("regular error")) {
		t.Error("IsGStreamerError should return false for regular error")
	}

	// Test error aggregator
	aggregator := NewErrorAggregator()
	aggregator.Add(initErr)
	aggregator.Add(pipelineErr)

	if !aggregator.HasErrors() {
		t.Error("Aggregator should have errors")
	}

	summary := aggregator.GetSummary()
	if summary["total_errors"] != 2 {
		t.Errorf("Expected 2 total errors, got %v", summary["total_errors"])
	}

	t.Log("All error handling tests passed")
}

// Test error type string representations
func TestErrorTypeStrings(t *testing.T) {
	testCases := []struct {
		errorType GStreamerErrorType
		expected  string
	}{
		{ErrorTypeInitialization, "Initialization"},
		{ErrorTypePipelineCreation, "PipelineCreation"},
		{ErrorTypeElementCreation, "ElementCreation"},
		{ErrorTypeMemoryAllocation, "MemoryAllocation"},
		{ErrorTypeUnknown, "Unknown"},
	}

	for _, tc := range testCases {
		if tc.errorType.String() != tc.expected {
			t.Errorf("Expected %s, got %s", tc.expected, tc.errorType.String())
		}
	}
}

// Test severity string representations
func TestSeverityStrings(t *testing.T) {
	testCases := []struct {
		severity GStreamerErrorSeverity
		expected string
	}{
		{UnifiedSeverityInfo, "Info"},
		{UnifiedSeverityWarning, "Warning"},
		{UnifiedSeverityError, "Error"},
		{UnifiedSeverityCritical, "Critical"},
		{UnifiedSeverityFatal, "Fatal"},
	}

	for _, tc := range testCases {
		if tc.severity.String() != tc.expected {
			t.Errorf("Expected %s, got %s", tc.expected, tc.severity.String())
		}
	}
}

// Test recovery action string representations
func TestRecoveryActionStrings(t *testing.T) {
	testCases := []struct {
		action   RecoveryAction
		expected string
	}{
		{RecoveryRetry, "Retry"},
		{RecoveryRestart, "Restart"},
		{RecoveryReconfigure, "Reconfigure"},
		{RecoveryFallback, "Fallback"},
		{RecoveryManualIntervention, "ManualIntervention"},
		{RecoveryNone, "None"},
	}

	for _, tc := range testCases {
		if tc.action.String() != tc.expected {
			t.Errorf("Expected %s, got %s", tc.expected, tc.action.String())
		}
	}
}
