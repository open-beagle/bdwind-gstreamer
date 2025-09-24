package gstreamer

import (
	"testing"

	"github.com/go-gst/go-gst/gst"
	"github.com/open-beagle/bdwind-gstreamer/internal/config"
	"github.com/sirupsen/logrus"
)

func TestPropertyIntrospection(t *testing.T) {
	// Initialize GStreamer
	gst.Init(nil)

	// Create a test encoder
	enc := &EncoderGst{
		logger: logrus.WithField("component", "test-encoder"),
	}

	// Test x264 property introspection
	t.Run("X264PropertyIntrospection", func(t *testing.T) {
		// Try to create an x264enc element
		element, err := gst.NewElement("x264enc")
		if err != nil {
			t.Skipf("x264enc not available: %v", err)
			return
		}
		defer element.Unref()

		// Test introspection
		properties := enc.introspectX264Properties(element)

		// We should find at least some basic properties
		if len(properties) == 0 {
			t.Error("Expected to find at least some x264 properties")
		}

		// Check for common properties that should exist
		expectedProps := []string{"bitrate", "threads"}
		for _, prop := range expectedProps {
			if _, exists := properties[prop]; !exists {
				t.Errorf("Expected property '%s' not found in introspection results", prop)
			}
		}

		t.Logf("Found %d x264 properties", len(properties))
		for name, info := range properties {
			t.Logf("Property: %s - %s", name, info.Description)
		}
	})

	// Test generic encoder property introspection
	t.Run("GenericEncoderIntrospection", func(t *testing.T) {
		// Try to create an x264enc element
		element, err := gst.NewElement("x264enc")
		if err != nil {
			t.Skipf("x264enc not available: %v", err)
			return
		}
		defer element.Unref()

		// Test generic introspection
		properties := enc.introspectEncoderProperties(element, "x264")

		// We should find at least some basic properties
		if len(properties) == 0 {
			t.Error("Expected to find at least some x264 properties via generic introspection")
		}

		t.Logf("Found %d x264 properties via generic introspection", len(properties))
	})
}

func TestPropertyExistenceChecking(t *testing.T) {
	// Initialize GStreamer
	gst.Init(nil)

	// Create a test encoder with minimal config
	cfg := config.EncoderConfig{
		Bitrate: 1000,
		Threads: 2,
	}

	enc := &EncoderGst{
		config: cfg,
		logger: logrus.WithField("component", "test-encoder"),
	}

	// Try to create an x264enc element
	element, err := gst.NewElement("x264enc")
	if err != nil {
		t.Skipf("x264enc not available: %v", err)
		return
	}
	defer element.Unref()

	enc.encoder = element

	t.Run("PropertyExistenceCheck", func(t *testing.T) {
		// Get available properties
		availableProperties := enc.introspectX264Properties(element)

		// Test properties that should exist
		testProperties := []PropertyConfig{
			{"bitrate", uint(1000), true},
			{"threads", uint(2), true},
		}

		// Test properties that might not exist
		optionalProperties := []PropertyConfig{
			{"speed-preset", "medium", false},
			{"nonexistent-property", "value", false},
		}

		allProperties := append(testProperties, optionalProperties...)

		// Test the property setting with existence checking
		err := enc.setEncoderProperties(allProperties, "x264", availableProperties)
		if err != nil {
			t.Errorf("Property setting failed: %v", err)
		}
	})
}
