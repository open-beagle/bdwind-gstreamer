package webrtc

import (
	"fmt"
	"time"
)

// ExampleHealthCheckUsage demonstrates how to use the MediaStream HealthCheck functionality
func ExampleHealthCheckUsage() {
	// Create a MediaStream with video enabled
	config := &MediaStreamConfig{
		VideoEnabled: true,
		AudioEnabled: false,
		VideoTrackID: "video",
		VideoCodec:   "h264",
	}

	ms, err := NewMediaStream(config)
	if err != nil {
		fmt.Printf("Failed to create MediaStream: %v\n", err)
		return
	}
	defer ms.Close()

	// Perform initial health check
	fmt.Println("=== Initial Health Check ===")
	if err := ms.HealthCheck(); err != nil {
		fmt.Printf("Health check failed: %v\n", err)
	} else {
		fmt.Println("Health check passed!")
	}

	// Simulate some activity by updating track activity time
	impl := ms.(*mediaStreamImpl)
	impl.mutex.Lock()
	if trackInfo, exists := impl.trackInfo[config.VideoTrackID]; exists {
		trackInfo.LastActive = time.Now()
		trackInfo.FramesSent = 1000
		trackInfo.BytesSent = 1024 * 1024 // 1MB
	}
	impl.mutex.Unlock()

	// Perform health check after activity
	fmt.Println("\n=== Health Check After Activity ===")
	if err := ms.HealthCheck(); err != nil {
		fmt.Printf("Health check failed: %v\n", err)
	} else {
		fmt.Println("Health check passed!")
	}

	// Simulate inactivity by setting old timestamp
	impl.mutex.Lock()
	if trackInfo, exists := impl.trackInfo[config.VideoTrackID]; exists {
		trackInfo.LastActive = time.Now().Add(-time.Hour) // 1 hour ago
	}
	impl.mutex.Unlock()

	// Perform health check after long inactivity
	fmt.Println("\n=== Health Check After Long Inactivity ===")
	if err := ms.HealthCheck(); err != nil {
		fmt.Printf("Health check failed: %v\n", err)
	} else {
		fmt.Println("Health check passed!")
	}

	// Example of using health check in a monitoring loop
	fmt.Println("\n=== Monitoring Loop Example ===")
	for i := 0; i < 3; i++ {
		fmt.Printf("Health check iteration %d: ", i+1)
		if err := ms.HealthCheck(); err != nil {
			fmt.Printf("FAILED - %v\n", err)
		} else {
			fmt.Printf("PASSED\n")
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// HealthCheckIntegrationExample shows how to integrate health checks with other components
func HealthCheckIntegrationExample() {
	config := &MediaStreamConfig{
		VideoEnabled: true,
		AudioEnabled: true,
		VideoTrackID: "video",
		AudioTrackID: "audio",
		VideoCodec:   "h264",
		AudioCodec:   "opus",
	}

	ms, err := NewMediaStream(config)
	if err != nil {
		fmt.Printf("Failed to create MediaStream: %v\n", err)
		return
	}
	defer ms.Close()

	// Example of periodic health monitoring
	healthCheckTicker := time.NewTicker(5 * time.Second)
	defer healthCheckTicker.Stop()

	fmt.Println("Starting periodic health monitoring...")

	// Simulate running for a short time
	timeout := time.After(2 * time.Second)

	for {
		select {
		case <-healthCheckTicker.C:
			fmt.Println("Performing scheduled health check...")
			if err := ms.HealthCheck(); err != nil {
				fmt.Printf("⚠️  Health check failed: %v\n", err)
				// In a real application, you might:
				// - Log the error
				// - Send an alert
				// - Attempt recovery
				// - Update metrics
			} else {
				fmt.Println("✅ Health check passed")
			}

		case <-timeout:
			fmt.Println("Health monitoring example completed")
			return
		}
	}
}
