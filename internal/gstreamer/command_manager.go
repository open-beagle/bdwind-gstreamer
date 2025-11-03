package gstreamer

import (
	"context"
	"fmt"
	"os/exec"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/open-beagle/bdwind-gstreamer/internal/config"
)

// CommandGStreamerManager uses command-line GStreamer to avoid Go binding issues
type CommandGStreamerManager struct {
	config *config.GStreamerConfig
	logger *logrus.Entry

	// Process management
	cmd    *exec.Cmd
	ctx    context.Context
	cancel context.CancelFunc

	// State management
	running bool
	mutex   sync.RWMutex

	// Video callback
	videoCallback func([]byte, time.Duration) error
}

// NewCommandGStreamerManager creates a new command-based GStreamer manager
func NewCommandGStreamerManager(cfg *config.GStreamerConfig) (*CommandGStreamerManager, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	ctx, cancel := context.WithCancel(context.Background())
	logger := logrus.WithField("component", "command-gstreamer")

	manager := &CommandGStreamerManager{
		config: cfg,
		logger: logger,
		ctx:    ctx,
		cancel: cancel,
	}

	logger.Debug("Command GStreamer manager created successfully")
	return manager, nil
}

// Start starts the GStreamer pipeline using command line
func (m *CommandGStreamerManager) Start() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.running {
		return fmt.Errorf("GStreamer manager already running")
	}

	m.logger.Info("Starting command GStreamer manager...")

	// Check configuration
	if m.config == nil {
		return fmt.Errorf("configuration is nil")
	}
	if m.config.Capture.DisplayID == "" {
		return fmt.Errorf("display ID is empty")
	}

	// Log configuration for debugging
	m.logger.Infof("Display ID: %s", m.config.Capture.DisplayID)
	m.logger.Infof("Width: %d, Height: %d, FrameRate: %d",
		m.config.Capture.Width, m.config.Capture.Height, m.config.Capture.FrameRate)

	// Build GStreamer command
	pipeline := m.buildPipeline()
	m.logger.Infof("GStreamer pipeline: gst-launch-1.0 %s", pipeline)

	// Create command - use shell to handle the pipeline properly
	m.cmd = exec.CommandContext(m.ctx, "sh", "-c", fmt.Sprintf("gst-launch-1.0 %s", pipeline))

	// Capture stderr for debugging
	m.cmd.Stderr = nil // Let it go to our logs

	// Start the process
	if err := m.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start GStreamer process: %w", err)
	}

	m.running = true
	m.logger.Info("Command GStreamer manager started successfully")

	// Monitor process in background
	go m.monitorProcess()

	return nil
}

// Stop stops the GStreamer process
func (m *CommandGStreamerManager) Stop() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if !m.running {
		return nil
	}

	m.logger.Info("Stopping command GStreamer manager...")

	// Cancel context to stop process
	if m.cancel != nil {
		m.cancel()
	}

	// Wait for process to exit
	if m.cmd != nil && m.cmd.Process != nil {
		m.cmd.Process.Kill()
		m.cmd.Wait()
	}

	m.running = false
	m.logger.Info("Command GStreamer manager stopped successfully")

	return nil
}

// IsRunning returns whether the manager is running
func (m *CommandGStreamerManager) IsRunning() bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.running
}

// SetVideoCallback sets the video data callback (not used in command mode)
func (m *CommandGStreamerManager) SetVideoCallback(callback func([]byte, time.Duration) error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.videoCallback = callback
	m.logger.Debug("Video callback set (command mode - callback not used)")
}

// buildPipeline builds the GStreamer pipeline command
func (m *CommandGStreamerManager) buildPipeline() string {
	// Build optimized pipeline for desktop capture with UDP output for WebRTC
	// Add queues and buffer control to reduce latency and improve performance
	pipeline := fmt.Sprintf(
		"ximagesrc display-name=%s show-pointer=true use-damage=false ! "+
			"videoconvert ! "+
			"videoscale ! "+
			"video/x-raw,format=I420,width=%d,height=%d,framerate=%d/1 ! "+
			"x264enc bitrate=%d speed-preset=ultrafast tune=zerolatency key-int-max=10 cabac=false ! "+
			"rtph264pay config-interval=1 pt=96 mtu=1200 ! "+
			"udpsink host=127.0.0.1 port=5004 sync=false",
		m.config.Capture.DisplayID,
		m.config.Capture.Width,
		m.config.Capture.Height,
		m.config.Capture.FrameRate,
		m.config.Encoding.Bitrate,
	)

	return pipeline
}

// monitorProcess monitors the GStreamer process
func (m *CommandGStreamerManager) monitorProcess() {
	if m.cmd == nil {
		return
	}

	err := m.cmd.Wait()

	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.running {
		if err != nil {
			// Get more detailed error information
			if exitError, ok := err.(*exec.ExitError); ok {
				m.logger.Errorf("GStreamer process exited with code %d: %v", exitError.ExitCode(), err)
				if len(exitError.Stderr) > 0 {
					m.logger.Errorf("GStreamer stderr: %s", string(exitError.Stderr))
				}
			} else {
				m.logger.Errorf("GStreamer process exited with error: %v", err)
			}
		} else {
			m.logger.Info("GStreamer process exited normally")
		}
		m.running = false
	}
}
