package gstreamer

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-gst/go-glib/glib"
	"github.com/go-gst/go-gst/gst"
	"github.com/sirupsen/logrus"

	"github.com/open-beagle/bdwind-gstreamer/internal/config"
)

// PipelineStateError extends the existing PipelineState enum
const (
	PipelineStateError PipelineState = iota + 4 // Continue from existing states
)

func (s PipelineState) String() string {
	switch s {
	case PipelineStateNull:
		return "NULL"
	case PipelineStateReady:
		return "READY"
	case PipelineStatePaused:
		return "PAUSED"
	case PipelineStatePlaying:
		return "PLAYING"
	case PipelineStateError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// PipelineComponent interface for pipeline components
type PipelineComponent interface {
	Initialize(pipeline *gst.Pipeline) error
	Start() error
	Stop() error
	GetState() ComponentState
	GetStatistics() ComponentStatistics
}

// ComponentState represents the state of a pipeline component
type ComponentState int

const (
	ComponentStateUninitialized ComponentState = iota
	ComponentStateInitialized
	ComponentStateStarted
	ComponentStateStopped
	ComponentStateError
)

func (s ComponentState) String() string {
	switch s {
	case ComponentStateUninitialized:
		return "UNINITIALIZED"
	case ComponentStateInitialized:
		return "INITIALIZED"
	case ComponentStateStarted:
		return "STARTED"
	case ComponentStateStopped:
		return "STOPPED"
	case ComponentStateError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// ComponentStatistics holds statistics for a pipeline component
type ComponentStatistics struct {
	StartTime     time.Time
	Uptime        time.Duration
	ProcessedData int64
	ErrorCount    int64
	LastError     error
	LastErrorTime time.Time
}

// PipelineManager manages the complete GStreamer pipeline lifecycle
// Based on go-gst best practices with Pipeline and MainLoop patterns
type PipelineManager struct {
	// go-gst core objects
	pipeline *gst.Pipeline
	bus      *gst.Bus
	mainLoop *glib.MainLoop

	// Sub-component management
	desktopCapture  PipelineComponent
	videoEncoder    PipelineComponent
	audioProcessor  PipelineComponent
	streamPublisher *StreamPublisher

	// State management
	state        PipelineState
	stateManager *StateManager
	errorManager *ErrorManager

	// Memory management
	memoryManager *MemoryManager

	// Configuration
	config *config.GStreamerConfig
	logger *logrus.Entry

	// Lifecycle management
	ctx     context.Context
	cancel  context.CancelFunc
	running bool
	mutex   sync.RWMutex

	// Statistics
	startTime time.Time
	stats     PipelineStatistics
}

// PipelineStatistics holds pipeline-level statistics
type PipelineStatistics struct {
	StartTime        time.Time
	Uptime           time.Duration
	StateChanges     int64
	ErrorCount       int64
	ComponentsActive int
	ComponentsTotal  int
	LastStateChange  time.Time
	LastError        error
	LastErrorTime    time.Time
	MemoryUsage      int64
	ProcessedFrames  int64
	DroppedFrames    int64
}

// NewPipelineManager creates a new pipeline manager based on go-gst best practices
func NewPipelineManager(ctx context.Context, config *config.GStreamerConfig, logger *logrus.Entry) (*PipelineManager, error) {
	if ctx == nil {
		return nil, fmt.Errorf("context is required")
	}
	if config == nil {
		return nil, fmt.Errorf("config is required")
	}
	if logger == nil {
		return nil, fmt.Errorf("logger is required")
	}

	// Create child context for pipeline management
	childCtx, cancel := context.WithCancel(ctx)

	// Initialize GStreamer library (idempotent)
	gst.Init(nil)

	pm := &PipelineManager{
		config:    config,
		logger:    logger.WithField("component", "pipeline-manager"),
		ctx:       childCtx,
		cancel:    cancel,
		state:     PipelineStateNull,
		startTime: time.Now(),
		stats: PipelineStatistics{
			StartTime: time.Now(),
		},
	}

	// Initialize memory manager
	memoryConfig := DefaultMemoryManagerConfig()
	pm.memoryManager = NewMemoryManager(memoryConfig, pm.logger.WithField("subcomponent", "memory-manager"))

	// Initialize state manager
	stateConfig := DefaultStateManagerConfig()
	pm.stateManager = NewStateManager(stateConfig, pm.logger.WithField("subcomponent", "state-manager"))

	// Initialize error manager
	errorConfig := DefaultErrorManagerConfig()
	pm.errorManager = NewErrorManager(errorConfig, pm.logger.WithField("subcomponent", "error-manager"))

	pm.logger.Debug("Pipeline manager created successfully")
	return pm, nil
}

// Initialize initializes the pipeline and all components
func (pm *PipelineManager) Initialize() error {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	if pm.pipeline != nil {
		return fmt.Errorf("pipeline already initialized")
	}

	pm.logger.Debug("Initializing GStreamer pipeline")

	// Create main pipeline
	pipeline, err := gst.NewPipeline("main-pipeline")
	if err != nil {
		return fmt.Errorf("failed to create pipeline: %w", err)
	}

	// Register pipeline with memory manager for lifecycle tracking
	pm.memoryManager.RegisterObject(pipeline)

	pm.pipeline = pipeline

	// Get pipeline bus for message handling
	pm.bus = pipeline.GetPipelineBus()
	if pm.bus != nil {
		pm.memoryManager.RegisterObject(pm.bus)
	}

	// Create main loop for go-gst event handling
	pm.mainLoop = glib.NewMainLoop(glib.MainContextDefault(), false)
	if pm.mainLoop != nil {
		pm.memoryManager.RegisterObject(pm.mainLoop)
	}

	// Initialize stream publisher
	publisherConfig := DefaultStreamPublisherConfig()
	pm.streamPublisher = NewStreamPublisher(pm.ctx, publisherConfig, pm.logger.WithField("subcomponent", "stream-publisher"))

	// Set up bus message handling
	if err := pm.setupBusMessageHandling(); err != nil {
		return fmt.Errorf("failed to setup bus message handling: %w", err)
	}

	// Initialize components (will be implemented in subsequent tasks)
	if err := pm.initializeComponents(); err != nil {
		return fmt.Errorf("failed to initialize components: %w", err)
	}

	pm.state = PipelineStateReady
	pm.stats.ComponentsTotal = pm.getComponentCount()
	pm.stats.LastStateChange = time.Now()

	pm.logger.Debug("Pipeline initialized successfully")
	return nil
}

// setupBusMessageHandling sets up GStreamer bus message handling
func (pm *PipelineManager) setupBusMessageHandling() error {
	if pm.bus == nil {
		return fmt.Errorf("bus not available")
	}

	// Add bus watch for message handling
	pm.bus.AddWatch(func(msg *gst.Message) bool {
		return pm.handleBusMessage(msg)
	})

	pm.logger.Debug("Bus message handling configured")
	return nil
}

// handleBusMessage handles GStreamer bus messages
func (pm *PipelineManager) handleBusMessage(msg *gst.Message) bool {
	if msg == nil {
		return true
	}

	switch msg.Type() {
	case gst.MessageEOS:
		pm.logger.Debug("Received EOS message")
		pm.handleEOS()

	case gst.MessageError:
		pm.logger.Error("Received error message from pipeline")
		pm.handleError(msg)

	case gst.MessageWarning:
		pm.logger.Warn("Received warning message from pipeline")
		pm.handleWarning(msg)

	case gst.MessageStateChanged:
		pm.handleStateChanged(msg)

	case gst.MessageBuffering:
		pm.handleBuffering(msg)

	default:
		// Log other messages at trace level
		pm.logger.Tracef("Received bus message: %s", msg.Type().String())
	}

	return true // Continue watching
}

// handleEOS handles End-Of-Stream messages
func (pm *PipelineManager) handleEOS() {
	pm.logger.Info("Pipeline reached end of stream")
	// Handle graceful shutdown if needed
}

// handleError handles error messages from the pipeline
func (pm *PipelineManager) handleError(msg *gst.Message) {
	if pm.errorManager != nil {
		// Extract error information and delegate to error manager
		pm.errorManager.HandlePipelineError(msg)
	}
	pm.stats.ErrorCount++
	pm.stats.LastErrorTime = time.Now()
}

// handleWarning handles warning messages from the pipeline
func (pm *PipelineManager) handleWarning(msg *gst.Message) {
	pm.logger.Warn("Pipeline warning received")
}

// handleStateChanged handles state change messages
func (pm *PipelineManager) handleStateChanged(msg *gst.Message) {
	if pm.stateManager != nil {
		pm.stateManager.HandleStateChange(msg)
	}
	pm.stats.StateChanges++
	pm.stats.LastStateChange = time.Now()
}

// handleBuffering handles buffering messages
func (pm *PipelineManager) handleBuffering(msg *gst.Message) {
	pm.logger.Trace("Pipeline buffering message received")
}

// initializeComponents initializes all pipeline components
func (pm *PipelineManager) initializeComponents() error {
	pm.logger.Debug("Initializing pipeline components")

	// Components will be initialized in subsequent tasks
	// For now, just set up the framework

	pm.stats.ComponentsActive = pm.getActiveComponentCount()
	pm.logger.Debugf("Initialized %d components", pm.stats.ComponentsActive)

	return nil
}

// Start starts the pipeline manager and all components
func (pm *PipelineManager) Start(ctx context.Context) error {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	if pm.running {
		return fmt.Errorf("pipeline manager already running")
	}

	pm.logger.Debug("Starting pipeline manager")

	// Initialize if not already done
	if pm.pipeline == nil {
		if err := pm.Initialize(); err != nil {
			return fmt.Errorf("failed to initialize pipeline: %w", err)
		}
	}

	// Start stream publisher
	if pm.streamPublisher != nil {
		if err := pm.streamPublisher.Start(); err != nil {
			return fmt.Errorf("failed to start stream publisher: %w", err)
		}
	}

	// Start state manager
	if pm.stateManager != nil {
		if err := pm.stateManager.Start(); err != nil {
			return fmt.Errorf("failed to start state manager: %w", err)
		}
	}

	// Start error manager
	if pm.errorManager != nil {
		if err := pm.errorManager.Start(); err != nil {
			return fmt.Errorf("failed to start error manager: %w", err)
		}
	}

	// Start components
	if err := pm.startComponents(); err != nil {
		return fmt.Errorf("failed to start components: %w", err)
	}

	// Set pipeline to playing state
	if err := pm.pipeline.SetState(gst.StatePlaying); err != nil {
		return fmt.Errorf("failed to set pipeline to playing state: %w", err)
	}

	// Start main loop in a separate goroutine
	go pm.runMainLoop()

	pm.running = true
	pm.state = PipelineStatePlaying
	pm.startTime = time.Now()
	pm.stats.StartTime = pm.startTime

	pm.logger.Info("Pipeline manager started successfully")
	return nil
}

// startComponents starts all pipeline components
func (pm *PipelineManager) startComponents() error {
	pm.logger.Debug("Starting pipeline components")

	// Start components (will be implemented in subsequent tasks)
	components := pm.getComponents()
	for _, component := range components {
		if component != nil {
			if err := component.Start(); err != nil {
				return fmt.Errorf("failed to start component: %w", err)
			}
		}
	}

	pm.stats.ComponentsActive = pm.getActiveComponentCount()
	pm.logger.Debugf("Started %d components", pm.stats.ComponentsActive)

	return nil
}

// runMainLoop runs the GLib main loop for event processing
func (pm *PipelineManager) runMainLoop() {
	pm.logger.Debug("Starting GLib main loop")

	defer func() {
		if r := recover(); r != nil {
			pm.logger.Errorf("Main loop panic recovered: %v", r)
		}
		pm.logger.Debug("GLib main loop stopped")
	}()

	if pm.mainLoop != nil {
		pm.mainLoop.Run()
	}
}

// Stop stops the pipeline manager and all components
func (pm *PipelineManager) Stop() error {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	if !pm.running {
		return nil
	}

	pm.logger.Debug("Stopping pipeline manager")

	// Stop components first
	if err := pm.stopComponents(); err != nil {
		pm.logger.Warnf("Error stopping components: %v", err)
	}

	// Stop pipeline
	if pm.pipeline != nil {
		if err := pm.pipeline.SetState(gst.StateNull); err != nil {
			pm.logger.Warnf("Error setting pipeline to null state: %v", err)
		}
	}

	// Stop main loop
	if pm.mainLoop != nil && pm.mainLoop.IsRunning() {
		pm.mainLoop.Quit()
	}

	// Stop managers
	if pm.errorManager != nil {
		if err := pm.errorManager.Stop(); err != nil {
			pm.logger.Warnf("Error stopping error manager: %v", err)
		}
	}

	if pm.stateManager != nil {
		if err := pm.stateManager.Stop(); err != nil {
			pm.logger.Warnf("Error stopping state manager: %v", err)
		}
	}

	if pm.streamPublisher != nil {
		if err := pm.streamPublisher.Stop(); err != nil {
			pm.logger.Warnf("Error stopping stream publisher: %v", err)
		}
	}

	// Clean up memory manager
	if pm.memoryManager != nil {
		pm.memoryManager.Cleanup()
	}

	// Cancel context
	pm.cancel()

	pm.running = false
	pm.state = PipelineStateNull

	pm.logger.Info("Pipeline manager stopped successfully")
	return nil
}

// IsRunning returns whether the pipeline manager is running
func (pm *PipelineManager) IsRunning() bool {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()
	return pm.running
}

// GetConfig returns the current configuration
func (pm *PipelineManager) GetConfig() *config.GStreamerConfig {
	return pm.config
}

// UpdateConfig updates the pipeline configuration
func (pm *PipelineManager) UpdateConfig(config *config.GStreamerConfig) error {
	if config == nil {
		return fmt.Errorf("config cannot be nil")
	}

	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	pm.config = config
	pm.logger.Debug("Pipeline configuration updated")

	// Apply configuration changes to components if running
	if pm.running {
		return pm.applyConfigurationChanges()
	}

	return nil
}

// applyConfigurationChanges applies configuration changes to running components
func (pm *PipelineManager) applyConfigurationChanges() error {
	pm.logger.Debug("Applying configuration changes to running components")

	// Apply changes to components (will be implemented in subsequent tasks)
	components := pm.getComponents()
	for _, component := range components {
		if component != nil {
			// Components will need to implement configuration update methods
			pm.logger.Trace("Configuration applied to component")
		}
	}

	return nil
}

// ConfigureLogging configures logging for the pipeline and components
func (pm *PipelineManager) ConfigureLogging(level string) error {
	pm.logger.Debugf("Configuring logging level: %s", level)

	// Configure logging for all components
	// This maintains compatibility with the existing interface

	return nil
}

// GetStatus returns the current pipeline status
func (pm *PipelineManager) GetStatus() *Status {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()

	return &Status{
		Running:    pm.running,
		State:      pm.state.String(),
		StartTime:  pm.startTime,
		Uptime:     time.Since(pm.startTime),
		Components: pm.getComponentStatus(),
	}
}

// Status represents the current status of the pipeline manager
type Status struct {
	Running    bool                   `json:"running"`
	State      string                 `json:"state"`
	StartTime  time.Time              `json:"start_time"`
	Uptime     time.Duration          `json:"uptime"`
	Components map[string]interface{} `json:"components"`
}

// GetStatistics returns pipeline statistics
func (pm *PipelineManager) GetStatistics() *PipelineStatistics {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()

	// Update runtime statistics
	pm.stats.Uptime = time.Since(pm.stats.StartTime)
	pm.stats.ComponentsActive = pm.getActiveComponentCount()

	// Get memory usage from memory manager
	if pm.memoryManager != nil {
		pm.stats.MemoryUsage = pm.memoryManager.GetMemoryUsage()
	}

	return &pm.stats
}

// Helper methods

// stopComponents stops all pipeline components
func (pm *PipelineManager) stopComponents() error {
	pm.logger.Debug("Stopping pipeline components")

	components := pm.getComponents()
	for _, component := range components {
		if component != nil {
			if err := component.Stop(); err != nil {
				pm.logger.Warnf("Error stopping component: %v", err)
			}
		}
	}

	return nil
}

// getComponents returns all pipeline components
func (pm *PipelineManager) getComponents() []PipelineComponent {
	var components []PipelineComponent

	if pm.desktopCapture != nil {
		components = append(components, pm.desktopCapture)
	}
	if pm.videoEncoder != nil {
		components = append(components, pm.videoEncoder)
	}
	if pm.audioProcessor != nil {
		components = append(components, pm.audioProcessor)
	}

	return components
}

// getComponentCount returns the total number of components
func (pm *PipelineManager) getComponentCount() int {
	return len(pm.getComponents())
}

// getActiveComponentCount returns the number of active components
func (pm *PipelineManager) getActiveComponentCount() int {
	count := 0
	components := pm.getComponents()
	for _, component := range components {
		if component != nil && component.GetState() == ComponentStateStarted {
			count++
		}
	}
	return count
}

// getComponentStatus returns the status of all components
func (pm *PipelineManager) getComponentStatus() map[string]interface{} {
	status := make(map[string]interface{})

	if pm.desktopCapture != nil {
		status["desktop_capture"] = map[string]interface{}{
			"state":      pm.desktopCapture.GetState().String(),
			"statistics": pm.desktopCapture.GetStatistics(),
		}
	}

	if pm.videoEncoder != nil {
		status["video_encoder"] = map[string]interface{}{
			"state":      pm.videoEncoder.GetState().String(),
			"statistics": pm.videoEncoder.GetStatistics(),
		}
	}

	if pm.audioProcessor != nil {
		status["audio_processor"] = map[string]interface{}{
			"state":      pm.audioProcessor.GetState().String(),
			"statistics": pm.audioProcessor.GetStatistics(),
		}
	}

	if pm.streamPublisher != nil {
		status["stream_publisher"] = pm.streamPublisher.GetStatus()
	}

	return status
}
