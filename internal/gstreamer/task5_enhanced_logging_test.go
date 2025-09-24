package gstreamer

import (
	"bytes"
	"testing"

	"github.com/go-gst/go-gst/gst"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/open-beagle/bdwind-gstreamer/internal/config"
)

// TestTask5_EnhancedErrorHandlingAndLogging tests the implementation of task 5
// Requirements: 2.1, 2.2, 2.3, 2.4 - Enhanced error handling and logging
func TestTask5_EnhancedErrorHandlingAndLogging(t *testing.T) {
	// Initialize GStreamer
	gst.Init(nil)

	t.Run("RequiredPropertyFailureLogging", testRequiredPropertyFailureLogging)
	t.Run("OptionalPropertyWarningLogging", testOptionalPropertyWarningLogging)
	t.Run("SuccessfulPropertyDebugLogging", testSuccessfulPropertyDebugLogging)
	t.Run("ConfigurationSummaryLogging", testConfigurationSummaryLogging)
}

// testRequiredPropertyFailureLogging tests requirement 2.1:
// 区分必需属性和可选属性的错误处理 - 必需属性失败应该记录为错误
func testRequiredPropertyFailureLogging(t *testing.T) {
	// Create a buffer to capture log output
	var logBuffer bytes.Buffer
	logger := logrus.New()
	logger.SetOutput(&logBuffer)
	logger.SetLevel(logrus.DebugLevel)

	// Create test encoder
	cfg := config.EncoderConfig{
		Bitrate: 1000,
		Threads: 2,
	}

	enc := &EncoderGst{
		config: cfg,
		logger: logrus.NewEntry(logger).WithField("component", "test-encoder"),
	}

	// Try to create an x264enc element
	element, err := gst.NewElement("x264enc")
	if err != nil {
		t.Skipf("x264enc not available: %v", err)
		return
	}
	defer element.Unref()

	enc.encoder = element

	// Get available properties
	availableProperties := enc.introspectX264Properties(element)

	// Test with a required property that doesn't exist
	properties := []PropertyConfig{
		{"bitrate", uint(1000), true},                // Should succeed
		{"nonexistent-required-prop", "value", true}, // Should fail and log error
	}

	// This should return an error due to the missing required property
	err = enc.setEncoderProperties(properties, "x264", availableProperties)

	// Should return error for missing required property
	assert.Error(t, err, "Should return error for missing required property")

	// Check log output contains error level logging for required property
	logOutput := logBuffer.String()
	assert.Contains(t, logOutput, "level=error", "Should contain error level log")
	assert.Contains(t, logOutput, "Critical failure: required encoder property not available", "Should contain critical failure message")
	assert.Contains(t, logOutput, "nonexistent-required-prop", "Should mention the missing property")
}

// testOptionalPropertyWarningLogging tests requirement 2.2:
// 区分必需属性和可选属性的错误处理 - 可选属性失败应该记录为警告
func testOptionalPropertyWarningLogging(t *testing.T) {
	// Create a buffer to capture log output
	var logBuffer bytes.Buffer
	logger := logrus.New()
	logger.SetOutput(&logBuffer)
	logger.SetLevel(logrus.DebugLevel)

	// Create test encoder
	cfg := config.EncoderConfig{
		Bitrate: 1000,
		Threads: 2,
	}

	enc := &EncoderGst{
		config: cfg,
		logger: logrus.NewEntry(logger).WithField("component", "test-encoder"),
	}

	// Try to create an x264enc element
	element, err := gst.NewElement("x264enc")
	if err != nil {
		t.Skipf("x264enc not available: %v", err)
		return
	}
	defer element.Unref()

	enc.encoder = element

	// Get available properties
	availableProperties := enc.introspectX264Properties(element)

	// Test with optional properties that don't exist
	properties := []PropertyConfig{
		{"bitrate", uint(1000), true},                 // Should succeed
		{"nonexistent-optional-prop", "value", false}, // Should fail but only warn
	}

	// This should succeed despite the missing optional property
	err = enc.setEncoderProperties(properties, "x264", availableProperties)

	// Should not return error for missing optional property
	assert.NoError(t, err, "Should not return error for missing optional property")

	// Check log output contains warning level logging for optional property
	logOutput := logBuffer.String()
	assert.Contains(t, logOutput, "level=warning", "Should contain warning level log")
	assert.Contains(t, logOutput, "Optional encoder property not available", "Should contain optional property warning")
	assert.Contains(t, logOutput, "nonexistent-optional-prop", "Should mention the missing optional property")
}

// testSuccessfulPropertyDebugLogging tests requirement 2.3:
// 为成功设置的属性添加调试级别日志
func testSuccessfulPropertyDebugLogging(t *testing.T) {
	// Create a buffer to capture log output
	var logBuffer bytes.Buffer
	logger := logrus.New()
	logger.SetOutput(&logBuffer)
	logger.SetLevel(logrus.DebugLevel)

	// Create test encoder
	cfg := config.EncoderConfig{
		Bitrate: 1000,
		Threads: 2,
	}

	enc := &EncoderGst{
		config: cfg,
		logger: logrus.NewEntry(logger).WithField("component", "test-encoder"),
	}

	// Try to create an x264enc element
	element, err := gst.NewElement("x264enc")
	if err != nil {
		t.Skipf("x264enc not available: %v", err)
		return
	}
	defer element.Unref()

	enc.encoder = element

	// Get available properties
	availableProperties := enc.introspectX264Properties(element)

	// Test with properties that should exist and succeed
	properties := []PropertyConfig{
		{"bitrate", uint(1000), true},
		{"threads", uint(2), true},
	}

	// This should succeed
	err = enc.setEncoderProperties(properties, "x264", availableProperties)
	require.NoError(t, err, "Property setting should succeed")

	// Check log output contains debug level logging for successful properties
	logOutput := logBuffer.String()
	assert.Contains(t, logOutput, "level=debug", "Should contain debug level log")
	assert.Contains(t, logOutput, "Encoder property successfully configured", "Should contain success message")
	assert.Contains(t, logOutput, "bitrate", "Should mention bitrate property")
	assert.Contains(t, logOutput, "threads", "Should mention threads property")
}

// testConfigurationSummaryLogging tests requirement 2.4:
// 添加配置完成后的摘要日志
func testConfigurationSummaryLogging(t *testing.T) {
	// Create a buffer to capture log output
	var logBuffer bytes.Buffer
	logger := logrus.New()
	logger.SetOutput(&logBuffer)
	logger.SetLevel(logrus.DebugLevel)

	// Create test encoder
	cfg := config.EncoderConfig{
		Bitrate: 1000,
		Threads: 2,
	}

	enc := &EncoderGst{
		config: cfg,
		logger: logrus.NewEntry(logger).WithField("component", "test-encoder"),
	}

	// Try to create an x264enc element
	element, err := gst.NewElement("x264enc")
	if err != nil {
		t.Skipf("x264enc not available: %v", err)
		return
	}
	defer element.Unref()

	enc.encoder = element

	// Get available properties
	availableProperties := enc.introspectX264Properties(element)

	// Test with a mix of properties
	properties := []PropertyConfig{
		{"bitrate", uint(1000), true},            // Should succeed
		{"threads", uint(2), true},               // Should succeed
		{"nonexistent-optional", "value", false}, // Should warn but not fail
	}

	// This should succeed
	err = enc.setEncoderProperties(properties, "x264", availableProperties)
	require.NoError(t, err, "Property setting should succeed")

	// Check log output contains configuration summary
	logOutput := logBuffer.String()
	assert.Contains(t, logOutput, "Encoder property configuration completed", "Should contain completion summary")
	assert.Contains(t, logOutput, "total_properties", "Should include total properties count")
	assert.Contains(t, logOutput, "successful", "Should include successful count")
	assert.Contains(t, logOutput, "skipped", "Should include skipped count")
	assert.Contains(t, logOutput, "warnings", "Should include warnings count")
	assert.Contains(t, logOutput, "critical_errors", "Should include critical errors count")
}
