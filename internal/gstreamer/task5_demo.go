package gstreamer

import (
	"fmt"

	"github.com/go-gst/go-gst/gst"
	"github.com/sirupsen/logrus"

	"github.com/open-beagle/bdwind-gstreamer/internal/config"
)

// DemoTask5EnhancedLogging demonstrates the enhanced error handling and logging
// implemented in Task 5. This function shows how the system now:
// 1. Distinguishes between required and optional property errors (Req 2.1, 2.2)
// 2. Logs successful properties at debug level (Req 2.3)
// 3. Provides comprehensive configuration summaries (Req 2.4)
func DemoTask5EnhancedLogging() {
	// Initialize GStreamer
	gst.Init(nil)

	// Set up logger to show all levels
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)
	logger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})

	fmt.Println("=== Task 5: Enhanced Error Handling and Logging Demo ===")
	fmt.Println()

	// Create test encoder configuration
	cfg := config.EncoderConfig{
		Bitrate: 2000,
		Threads: 4,
	}

	enc := &EncoderGst{
		config: cfg,
		logger: logrus.NewEntry(logger).WithField("component", "demo-encoder"),
	}

	// Try to create an x264enc element
	element, err := gst.NewElement("x264enc")
	if err != nil {
		fmt.Printf("x264enc not available: %v\n", err)
		return
	}
	defer element.Unref()

	enc.encoder = element

	fmt.Println("1. Demonstrating property introspection...")
	availableProperties := enc.introspectX264Properties(element)
	fmt.Printf("   Found %d available x264 properties\n", len(availableProperties))
	fmt.Println()

	fmt.Println("2. Demonstrating enhanced error handling and logging...")
	fmt.Println("   Setting properties with mix of required, optional, and invalid properties:")
	fmt.Println()

	// Test properties that demonstrate all aspects of enhanced logging
	properties := []PropertyConfig{
		// Required properties that should succeed
		{"bitrate", uint(2000), true},
		{"threads", uint(4), true},

		// Optional properties that should succeed
		{"speed-preset", "medium", false},
		{"tune", "film", false},

		// Required property that doesn't exist (should cause error)
		{"nonexistent-required", "value", true},

		// Optional property that doesn't exist (should cause warning)
		{"nonexistent-optional", "value", false},

		// Property with invalid value (should cause validation error)
		{"bframes", -5, false}, // Invalid value for bframes
	}

	// This will demonstrate all the enhanced logging features
	err = enc.setEncoderProperties(properties, "x264", availableProperties)

	fmt.Println()
	if err != nil {
		fmt.Printf("Configuration completed with errors (as expected): %v\n", err)
	} else {
		fmt.Println("Configuration completed successfully")
	}

	fmt.Println()
	fmt.Println("=== Demo Complete ===")
	fmt.Println("The logs above demonstrate:")
	fmt.Println("- Required property failures logged as ERROR level")
	fmt.Println("- Optional property failures logged as WARNING level")
	fmt.Println("- Successful properties logged as DEBUG level")
	fmt.Println("- Comprehensive configuration summary with statistics")
}
