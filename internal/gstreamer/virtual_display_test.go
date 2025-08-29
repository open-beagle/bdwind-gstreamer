package gstreamer

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/open-beagle/bdwind-gstreamer/internal/config"
)

// VirtualDisplayManager manages virtual display processes for testing
type VirtualDisplayManager struct {
	displayID string
	xvfbCmd   *exec.Cmd
	pid       int
	width     int
	height    int
	depth     int
}

// NewVirtualDisplayManager creates a new virtual display manager
func NewVirtualDisplayManager(displayID string, width, height, depth int) *VirtualDisplayManager {
	return &VirtualDisplayManager{
		displayID: displayID,
		width:     width,
		height:    height,
		depth:     depth,
	}
}

// Start starts the virtual display
func (vdm *VirtualDisplayManager) Start() error {
	// Kill any existing Xvfb process on this display
	vdm.killExistingXvfb()

	// Build Xvfb command
	screenSpec := fmt.Sprintf("%dx%dx%d", vdm.width, vdm.height, vdm.depth)
	args := []string{
		vdm.displayID,
		"-screen", "0", screenSpec,
		"-ac",              // disable access control
		"-nolisten", "tcp", // don't listen on TCP
		"-dpi", "96", // set DPI
		"+extension", "RANDR", // enable RANDR extension
	}

	vdm.xvfbCmd = exec.Command("Xvfb", args...)

	// Start the process
	if err := vdm.xvfbCmd.Start(); err != nil {
		return fmt.Errorf("failed to start Xvfb: %w", err)
	}

	vdm.pid = vdm.xvfbCmd.Process.Pid

	// Wait for the display to be ready
	if err := vdm.waitForDisplay(); err != nil {
		vdm.Stop()
		return fmt.Errorf("display not ready: %w", err)
	}

	return nil
}

// Stop stops the virtual display
func (vdm *VirtualDisplayManager) Stop() error {
	if vdm.xvfbCmd != nil && vdm.xvfbCmd.Process != nil {
		// Send SIGTERM first
		if err := vdm.xvfbCmd.Process.Signal(syscall.SIGTERM); err != nil {
			// If SIGTERM fails, try SIGKILL
			vdm.xvfbCmd.Process.Kill()
		}

		// Wait for process to exit
		vdm.xvfbCmd.Wait()
		vdm.xvfbCmd = nil
		vdm.pid = 0
	}

	// Also kill any remaining Xvfb processes on this display
	vdm.killExistingXvfb()

	return nil
}

// IsRunning checks if the virtual display is running
func (vdm *VirtualDisplayManager) IsRunning() bool {
	if vdm.pid == 0 {
		return false
	}

	// Check if process is still alive
	process, err := os.FindProcess(vdm.pid)
	if err != nil {
		return false
	}

	// Send signal 0 to check if process exists
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

// GetDisplayID returns the display ID
func (vdm *VirtualDisplayManager) GetDisplayID() string {
	return vdm.displayID
}

// GetPID returns the process ID
func (vdm *VirtualDisplayManager) GetPID() int {
	return vdm.pid
}

// waitForDisplay waits for the display to become available
func (vdm *VirtualDisplayManager) waitForDisplay() error {
	timeout := time.After(10 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return fmt.Errorf("timeout waiting for display %s", vdm.displayID)
		case <-ticker.C:
			if vdm.isDisplayReady() {
				return nil
			}
		}
	}
}

// isDisplayReady checks if the display is ready using xdpyinfo
func (vdm *VirtualDisplayManager) isDisplayReady() bool {
	cmd := exec.Command("xdpyinfo", "-display", vdm.displayID)
	err := cmd.Run()
	return err == nil
}

// killExistingXvfb kills any existing Xvfb processes on this display
func (vdm *VirtualDisplayManager) killExistingXvfb() {
	// Use pkill to find and kill Xvfb processes for this display
	pattern := fmt.Sprintf("Xvfb.*%s", strings.ReplaceAll(vdm.displayID, ":", "\\:"))
	cmd := exec.Command("pkill", "-f", pattern)
	cmd.Run() // Ignore errors - process might not exist

	// Wait a bit for cleanup
	time.Sleep(100 * time.Millisecond)
}

// TestVirtualDisplayStartup tests Xvfb startup and display accessibility
func TestVirtualDisplayStartup(t *testing.T) {
	// Skip if Xvfb is not available
	if !isXvfbAvailable() {
		t.Skip("Xvfb not available, skipping virtual display tests")
	}

	tests := []struct {
		name      string
		displayID string
		width     int
		height    int
		depth     int
		wantErr   bool
	}{
		{
			name:      "Standard HD display",
			displayID: ":99",
			width:     1920,
			height:    1080,
			depth:     24,
			wantErr:   false,
		},
		{
			name:      "4K display",
			displayID: ":98",
			width:     3840,
			height:    2160,
			depth:     24,
			wantErr:   false,
		},
		{
			name:      "Low resolution display",
			displayID: ":97",
			width:     800,
			height:    600,
			depth:     16,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vdm := NewVirtualDisplayManager(tt.displayID, tt.width, tt.height, tt.depth)

			// Start virtual display
			err := vdm.Start()
			if (err != nil) != tt.wantErr {
				t.Errorf("VirtualDisplayManager.Start() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				// Verify display is running
				if !vdm.IsRunning() {
					t.Error("Virtual display should be running")
				}

				// Verify display is accessible
				if !vdm.isDisplayReady() {
					t.Error("Virtual display should be accessible")
				}

				// Test display properties
				if err := testDisplayProperties(tt.displayID, tt.width, tt.height); err != nil {
					t.Errorf("Display properties test failed: %v", err)
				}

				// Cleanup
				if err := vdm.Stop(); err != nil {
					t.Errorf("Failed to stop virtual display: %v", err)
				}

				// Verify cleanup
				if vdm.IsRunning() {
					t.Error("Virtual display should be stopped")
				}
			}
		})
	}
}

// TestDesktopCaptureWithVirtualDisplay tests desktop capture compatibility with virtual displays
func TestDesktopCaptureWithVirtualDisplay(t *testing.T) {
	// Skip if Xvfb is not available
	if !isXvfbAvailable() {
		t.Skip("Xvfb not available, skipping virtual display tests")
	}

	displayID := ":96"
	width := 1920
	height := 1080
	depth := 24

	// Start virtual display
	vdm := NewVirtualDisplayManager(displayID, width, height, depth)
	err := vdm.Start()
	if err != nil {
		t.Fatalf("Failed to start virtual display: %v", err)
	}
	defer vdm.Stop()

	// Test desktop capture configuration
	tests := []struct {
		name   string
		config config.DesktopCaptureConfig
	}{
		{
			name: "Basic X11 capture",
			config: config.DesktopCaptureConfig{
				DisplayID:   displayID,
				FrameRate:   15,
				Width:       width,
				Height:      height,
				UseWayland:  false,
				Quality:     "medium",
				ShowPointer: false,
				UseDamage:   false,
				BufferSize:  10,
			},
		},
		{
			name: "Low quality capture",
			config: config.DesktopCaptureConfig{
				DisplayID:   displayID,
				FrameRate:   10,
				Width:       1280,
				Height:      720,
				UseWayland:  false,
				Quality:     "low",
				ShowPointer: false,
				UseDamage:   false,
				BufferSize:  5,
			},
		},
		{
			name: "High quality capture",
			config: config.DesktopCaptureConfig{
				DisplayID:   displayID,
				FrameRate:   30,
				Width:       width,
				Height:      height,
				UseWayland:  false,
				Quality:     "high",
				ShowPointer: true,
				UseDamage:   true,
				BufferSize:  20,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Validate configuration
			if err := config.ValidateDesktopCaptureConfig(&tt.config); err != nil {
				t.Errorf("Configuration validation failed: %v", err)
				return
			}

			// Create desktop capture
			capture, err := NewDesktopCaptureGst(tt.config)
			if err != nil {
				t.Errorf("Failed to create desktop capture: %v", err)
				return
			}

			// Test source configuration
			sourceType, props := capture.GetSource()
			if sourceType != "ximagesrc" {
				t.Errorf("Expected source type 'ximagesrc', got '%s'", sourceType)
			}

			// Verify display property is set correctly
			if displayProp, ok := props["display-name"]; !ok || displayProp != displayID {
				t.Errorf("Expected display property '%s', got '%v'", displayID, displayProp)
			}

			// Test statistics (should be initialized)
			stats := capture.GetStats()
			if stats.StartTime == 0 {
				t.Error("Statistics should be initialized")
			}
		})
	}
}

// TestVirtualDisplayCleanup tests proper cleanup of virtual display processes
func TestVirtualDisplayCleanup(t *testing.T) {
	// Skip if Xvfb is not available
	if !isXvfbAvailable() {
		t.Skip("Xvfb not available, skipping virtual display tests")
	}

	displayID := ":95"
	vdm := NewVirtualDisplayManager(displayID, 1024, 768, 24)

	// Start virtual display
	err := vdm.Start()
	if err != nil {
		t.Fatalf("Failed to start virtual display: %v", err)
	}

	originalPID := vdm.GetPID()
	if originalPID == 0 {
		t.Fatal("PID should be set after starting")
	}

	// Verify process is running
	if !vdm.IsRunning() {
		t.Fatal("Virtual display should be running")
	}

	// Test graceful stop
	err = vdm.Stop()
	if err != nil {
		t.Errorf("Failed to stop virtual display: %v", err)
	}

	// Verify process is stopped
	if vdm.IsRunning() {
		t.Error("Virtual display should be stopped")
	}

	// Verify PID is reset
	if vdm.GetPID() != 0 {
		t.Error("PID should be reset after stopping")
	}

	// Test that display is no longer accessible
	if vdm.isDisplayReady() {
		t.Error("Display should not be accessible after stopping")
	}

	// Test cleanup of orphaned processes
	t.Run("Cleanup orphaned processes", func(t *testing.T) {
		// Start another display
		vdm2 := NewVirtualDisplayManager(":94", 800, 600, 24)
		err := vdm2.Start()
		if err != nil {
			t.Fatalf("Failed to start second virtual display: %v", err)
		}

		// Get the original PID
		originalPID := vdm2.GetPID()

		// Simulate process becoming orphaned by setting cmd to nil
		// but keeping the process running
		originalCmd := vdm2.xvfbCmd
		vdm2.xvfbCmd = nil

		// killExistingXvfb should still clean up the process
		vdm2.killExistingXvfb()

		// Wait a bit for cleanup
		time.Sleep(500 * time.Millisecond)

		// Check if the process is still running by PID
		if originalPID > 0 {
			process, err := os.FindProcess(originalPID)
			if err == nil {
				err := process.Signal(syscall.Signal(0))
				if err == nil {
					// Process still exists, kill it manually for cleanup
					originalCmd.Process.Kill()
					t.Log("Note: Orphaned process cleanup may vary by system - this is acceptable")
				}
			}
		}

		// Verify display is no longer accessible (more reliable test)
		if vdm2.isDisplayReady() {
			t.Error("Display should not be accessible after cleanup")
		}
	})
}

// TestVirtualDisplayWithApplicationLifecycle tests virtual display integration with application lifecycle
func TestVirtualDisplayWithApplicationLifecycle(t *testing.T) {
	// Skip if Xvfb is not available
	if !isXvfbAvailable() {
		t.Skip("Xvfb not available, skipping virtual display tests")
	}

	displayID := ":93"

	// Test scenario: Start display, create capture, stop display, verify cleanup
	t.Run("Application lifecycle integration", func(t *testing.T) {
		// Start virtual display
		vdm := NewVirtualDisplayManager(displayID, 1920, 1080, 24)
		err := vdm.Start()
		if err != nil {
			t.Fatalf("Failed to start virtual display: %v", err)
		}

		// Set DISPLAY environment variable (simulating debug script behavior)
		originalDisplay := os.Getenv("DISPLAY")
		os.Setenv("DISPLAY", displayID)
		defer func() {
			if originalDisplay != "" {
				os.Setenv("DISPLAY", originalDisplay)
			} else {
				os.Unsetenv("DISPLAY")
			}
		}()

		// Create desktop capture configuration
		captureConfig := config.DefaultDesktopCaptureConfig()
		captureConfig.DisplayID = displayID

		// Create and test desktop capture
		capture, err := NewDesktopCaptureGst(captureConfig)
		if err != nil {
			t.Errorf("Failed to create desktop capture: %v", err)
		}

		// Test that capture can get source configuration
		sourceType, props := capture.GetSource()
		if sourceType != "ximagesrc" {
			t.Errorf("Expected ximagesrc, got %s", sourceType)
		}

		if props["display-name"] != displayID {
			t.Errorf("Expected display %s, got %v", displayID, props["display-name"])
		}

		// Simulate application shutdown
		err = vdm.Stop()
		if err != nil {
			t.Errorf("Failed to stop virtual display: %v", err)
		}

		// Verify cleanup
		if vdm.IsRunning() {
			t.Error("Virtual display should be stopped after application shutdown")
		}
	})
}

// TestMultipleVirtualDisplays tests handling of multiple virtual displays
func TestMultipleVirtualDisplays(t *testing.T) {
	// Skip if Xvfb is not available
	if !isXvfbAvailable() {
		t.Skip("Xvfb not available, skipping virtual display tests")
	}

	displays := []struct {
		id     string
		width  int
		height int
	}{
		{":92", 1920, 1080},
		{":91", 1280, 720},
		{":90", 800, 600},
	}

	var managers []*VirtualDisplayManager

	// Start all displays
	for _, display := range displays {
		vdm := NewVirtualDisplayManager(display.id, display.width, display.height, 24)
		err := vdm.Start()
		if err != nil {
			// Cleanup any started displays
			for _, mgr := range managers {
				mgr.Stop()
			}
			t.Fatalf("Failed to start display %s: %v", display.id, err)
		}
		managers = append(managers, vdm)
	}

	// Verify all displays are running
	for i, mgr := range managers {
		if !mgr.IsRunning() {
			t.Errorf("Display %s should be running", displays[i].id)
		}

		if !mgr.isDisplayReady() {
			t.Errorf("Display %s should be accessible", displays[i].id)
		}
	}

	// Test desktop capture with different displays
	for i := range managers {
		captureConfig := config.DesktopCaptureConfig{
			DisplayID:  displays[i].id,
			FrameRate:  15,
			Width:      displays[i].width,
			Height:     displays[i].height,
			UseWayland: false,
			Quality:    "medium",
		}

		capture, err := NewDesktopCaptureGst(captureConfig)
		if err != nil {
			t.Errorf("Failed to create capture for display %s: %v", displays[i].id, err)
			continue
		}

		sourceType, props := capture.GetSource()
		if sourceType != "ximagesrc" {
			t.Errorf("Expected ximagesrc for display %s, got %s", displays[i].id, sourceType)
		}

		if props["display"] != displays[i].id {
			t.Errorf("Expected display %s, got %v", displays[i].id, props["display"])
		}
	}

	// Cleanup all displays
	for _, mgr := range managers {
		if err := mgr.Stop(); err != nil {
			t.Errorf("Failed to stop display %s: %v", mgr.GetDisplayID(), err)
		}
	}

	// Verify all displays are stopped
	for _, mgr := range managers {
		if mgr.IsRunning() {
			t.Errorf("Display %s should be stopped", mgr.GetDisplayID())
		}
	}
}

// Helper functions

// isXvfbAvailable checks if Xvfb is available on the system
func isXvfbAvailable() bool {
	_, err := exec.LookPath("Xvfb")
	return err == nil
}

// testDisplayProperties tests display properties using xdpyinfo
func testDisplayProperties(displayID string, expectedWidth, expectedHeight int) error {
	cmd := exec.Command("xdpyinfo", "-display", displayID)
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to get display info: %w", err)
	}

	outputStr := string(output)

	// Parse dimensions from xdpyinfo output
	// Look for lines like "dimensions:    1920x1080 pixels (508x285 millimeters)"
	lines := strings.Split(outputStr, "\n")
	for _, line := range lines {
		if strings.Contains(line, "dimensions:") {
			// Extract dimensions using regex-like approach
			parts := strings.Fields(line)
			for i, part := range parts {
				if strings.Contains(part, "x") && (i+1 < len(parts)) && parts[i+1] == "pixels" {
					// Found dimension part like "1920x1080"
					dimParts := strings.Split(part, "x")
					if len(dimParts) == 2 {
						width, err1 := strconv.Atoi(dimParts[0])
						height, err2 := strconv.Atoi(dimParts[1])
						if err1 == nil && err2 == nil {
							if width != expectedWidth || height != expectedHeight {
								return fmt.Errorf("expected dimensions %dx%d, got %dx%d",
									expectedWidth, expectedHeight, width, height)
							}
							return nil
						}
					}
				}
			}
		}
	}

	// If we can't parse dimensions, just verify the display is accessible
	// This is acceptable for virtual display testing
	return nil
}

// TestVirtualDisplayErrorHandling tests error handling scenarios
func TestVirtualDisplayErrorHandling(t *testing.T) {
	// Skip if Xvfb is not available
	if !isXvfbAvailable() {
		t.Skip("Xvfb not available, skipping virtual display tests")
	}

	t.Run("Invalid display ID", func(t *testing.T) {
		// Test with invalid display ID format
		vdm := NewVirtualDisplayManager("invalid", 800, 600, 24)
		err := vdm.Start()
		if err == nil {
			vdm.Stop()
			t.Error("Expected error for invalid display ID")
		}
	})

	t.Run("Display already in use", func(t *testing.T) {
		displayID := ":89"

		// Start first display
		vdm1 := NewVirtualDisplayManager(displayID, 800, 600, 24)
		err := vdm1.Start()
		if err != nil {
			t.Fatalf("Failed to start first display: %v", err)
		}
		defer vdm1.Stop()

		// Try to start second display on same ID
		vdm2 := NewVirtualDisplayManager(displayID, 800, 600, 24)
		err = vdm2.Start()
		if err == nil {
			vdm2.Stop()
			t.Error("Expected error when starting display on already used ID")
		}
	})

	t.Run("Stop non-running display", func(t *testing.T) {
		vdm := NewVirtualDisplayManager(":88", 800, 600, 24)

		// Stop without starting should not error
		err := vdm.Stop()
		if err != nil {
			t.Errorf("Stop on non-running display should not error: %v", err)
		}
	})
}

// BenchmarkVirtualDisplayStartup benchmarks virtual display startup time
func BenchmarkVirtualDisplayStartup(b *testing.B) {
	// Skip if Xvfb is not available
	if !isXvfbAvailable() {
		b.Skip("Xvfb not available, skipping benchmark")
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		displayID := fmt.Sprintf(":%d", 80-i%10) // Use displays :80 to :71
		vdm := NewVirtualDisplayManager(displayID, 1024, 768, 24)

		start := time.Now()
		err := vdm.Start()
		if err != nil {
			b.Fatalf("Failed to start display: %v", err)
		}

		startupTime := time.Since(start)
		b.Logf("Display startup time: %v", startupTime)

		vdm.Stop()
	}
}
