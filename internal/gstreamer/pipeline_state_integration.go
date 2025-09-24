package gstreamer

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-gst/go-gst/gst"
	"github.com/sirupsen/logrus"
)

// PipelineStateIntegration provides a unified interface for pipeline state management
// This integrates the PipelineStateManager, PipelineHealthChecker, and PipelineErrorHandler
// into a single cohesive system that can be easily used by the Manager
type PipelineStateIntegration struct {
	// Core components
	stateManager  *PipelineStateManager
	healthChecker *PipelineHealthChecker
	errorHandler  *PipelineErrorHandler

	// Configuration
	config *PipelineStateIntegrationConfig
	logger *logrus.Entry

	// Pipeline reference
	pipeline *gst.Pipeline

	// Lifecycle management
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	mu     sync.RWMutex

	// State tracking
	isRunning bool
	startTime time.Time

	// Callbacks
	stateChangeCallback func(oldState, newState PipelineState, transition ExtendedStateTransition)
	errorCallback       func(ErrorType, string, error)
	healthCallback      func(OverallHealthStatus)

	// Metrics aggregation
	aggregatedMetrics *AggregatedMetrics
}

// PipelineStateIntegrationConfig holds configuration for the integration
type PipelineStateIntegrationConfig struct {
	// State manager configuration
	StateManagerConfig *PipelineStateManagerConfig

	// Health checker configuration
	HealthCheckerConfig *PipelineHealthCheckerConfig

	// Error handler configuration
	ErrorHandlerConfig *PipelineErrorHandlerConfig

	// Integration-specific settings
	EnableAutoRecovery     bool
	EnableHealthMonitoring bool
	EnableErrorHandling    bool

	// Callback settings
	NotifyStateChanges bool
	NotifyHealthIssues bool
	NotifyErrors       bool

	// Metrics settings
	EnableMetricsAggregation bool
	MetricsUpdateInterval    time.Duration
}

// AggregatedMetrics combines metrics from all components
type AggregatedMetrics struct {
	mu sync.RWMutex

	// State manager metrics
	StateManager StateManagerMetrics

	// Health checker metrics
	HealthChecker *HealthCheckerMetrics

	// Error handler metrics
	ErrorHandler *PipelineErrorHandlerMetrics

	// Integration metrics
	TotalOperations   int64
	SuccessfulOps     int64
	FailedOps         int64
	LastUpdateTime    time.Time
	IntegrationUptime time.Duration
}

// DefaultPipelineStateIntegrationConfig returns default configuration
func DefaultPipelineStateIntegrationConfig() *PipelineStateIntegrationConfig {
	return &PipelineStateIntegrationConfig{
		StateManagerConfig:       DefaultPipelineStateManagerConfig(),
		HealthCheckerConfig:      DefaultHealthCheckerConfig(),
		ErrorHandlerConfig:       DefaultPipelineErrorHandlerConfig(),
		EnableAutoRecovery:       true,
		EnableHealthMonitoring:   true,
		EnableErrorHandling:      true,
		NotifyStateChanges:       true,
		NotifyHealthIssues:       true,
		NotifyErrors:             true,
		EnableMetricsAggregation: true,
		MetricsUpdateInterval:    30 * time.Second,
	}
}

// NewPipelineStateIntegration creates a new pipeline state integration
func NewPipelineStateIntegration(ctx context.Context, pipeline *gst.Pipeline, logger *logrus.Entry, config *PipelineStateIntegrationConfig) *PipelineStateIntegration {
	if config == nil {
		config = DefaultPipelineStateIntegrationConfig()
	}

	if logger == nil {
		logger = logrus.WithField("component", "pipeline-state-integration")
	}

	childCtx, cancel := context.WithCancel(ctx)

	psi := &PipelineStateIntegration{
		config:    config,
		logger:    logger,
		pipeline:  pipeline,
		ctx:       childCtx,
		cancel:    cancel,
		startTime: time.Now(),
		aggregatedMetrics: &AggregatedMetrics{
			LastUpdateTime: time.Now(),
		},
	}

	// Initialize components
	psi.initializeComponents()

	return psi
}

// initializeComponents initializes all the pipeline management components
func (psi *PipelineStateIntegration) initializeComponents() {
	// Initialize state manager
	if psi.config.StateManagerConfig != nil {
		psi.stateManager = NewPipelineStateManager(psi.ctx, psi.pipeline, psi.logger.WithField("subcomponent", "state-manager"), psi.config.StateManagerConfig)

		// Set up state change callback
		if psi.config.NotifyStateChanges {
			psi.stateManager.SetStateChangeCallback(psi.onStateChange)
		}
	}

	// Initialize health checker
	if psi.config.EnableHealthMonitoring && psi.config.HealthCheckerConfig != nil {
		psi.healthChecker = NewPipelineHealthChecker(psi.logger.WithField("subcomponent", "health-checker"), psi.stateManager)
	}

	// Initialize error handler
	if psi.config.EnableErrorHandling && psi.config.ErrorHandlerConfig != nil {
		psi.errorHandler = NewPipelineErrorHandler(psi.logger.WithField("subcomponent", "error-handler"), psi.config.ErrorHandlerConfig)

		// Set up error callback
		if psi.config.NotifyErrors {
			psi.errorHandler.SetErrorCallback(psi.onError)
		}
	}

	// Connect error handler to state manager if both are available
	if psi.stateManager != nil && psi.errorHandler != nil {
		psi.stateManager.SetErrorCallback(func(errorType ErrorType, message string, err error) {
			psi.errorHandler.HandleError(errorType, message, err.Error(), "StateManager")
		})
	}

	psi.logger.Debug("Pipeline state integration components initialized")
}

// Start starts all pipeline management components
func (psi *PipelineStateIntegration) Start() error {
	psi.mu.Lock()
	defer psi.mu.Unlock()

	if psi.isRunning {
		return fmt.Errorf("pipeline state integration is already running")
	}

	psi.logger.Info("Starting pipeline state integration")

	// Start state manager
	if psi.stateManager != nil {
		if err := psi.stateManager.Start(); err != nil {
			return fmt.Errorf("failed to start state manager: %w", err)
		}
	}

	// Start health checker
	if psi.healthChecker != nil {
		if err := psi.healthChecker.Start(psi.ctx); err != nil {
			return fmt.Errorf("failed to start health checker: %w", err)
		}
	}

	// Start error handler
	if psi.errorHandler != nil {
		if err := psi.errorHandler.Start(psi.ctx); err != nil {
			return fmt.Errorf("failed to start error handler: %w", err)
		}
	}

	// Start metrics aggregation if enabled
	if psi.config.EnableMetricsAggregation {
		psi.wg.Add(1)
		go psi.metricsAggregationLoop()
	}

	psi.isRunning = true
	psi.startTime = time.Now()

	psi.logger.Info("Pipeline state integration started successfully")
	return nil
}

// Stop stops all pipeline management components
func (psi *PipelineStateIntegration) Stop() error {
	psi.mu.Lock()
	defer psi.mu.Unlock()

	if !psi.isRunning {
		return nil
	}

	psi.logger.Info("Stopping pipeline state integration")

	// Cancel context to stop all goroutines
	psi.cancel()

	// Stop components in reverse order
	if psi.errorHandler != nil {
		if err := psi.errorHandler.Stop(); err != nil {
			psi.logger.Errorf("Error stopping error handler: %v", err)
		}
	}

	if psi.healthChecker != nil {
		if err := psi.healthChecker.Stop(); err != nil {
			psi.logger.Errorf("Error stopping health checker: %v", err)
		}
	}

	if psi.stateManager != nil {
		if err := psi.stateManager.Stop(); err != nil {
			psi.logger.Errorf("Error stopping state manager: %v", err)
		}
	}

	// Wait for all goroutines to finish
	psi.wg.Wait()

	psi.isRunning = false

	psi.logger.Info("Pipeline state integration stopped successfully")
	return nil
}

// IsRunning returns whether the integration is running
func (psi *PipelineStateIntegration) IsRunning() bool {
	psi.mu.RLock()
	defer psi.mu.RUnlock()
	return psi.isRunning
}

// SetPipeline sets the pipeline for all components
func (psi *PipelineStateIntegration) SetPipeline(pipeline *gst.Pipeline) {
	psi.mu.Lock()
	defer psi.mu.Unlock()

	psi.pipeline = pipeline

	if psi.stateManager != nil {
		psi.stateManager.SetPipeline(pipeline)
	}

	psi.logger.Debug("Pipeline set for state integration")
}

// GetCurrentState returns the current pipeline state
func (psi *PipelineStateIntegration) GetCurrentState() PipelineState {
	if psi.stateManager != nil {
		return psi.stateManager.GetCurrentState()
	}
	return PipelineStateNull
}

// SetState attempts to change the pipeline state
func (psi *PipelineStateIntegration) SetState(targetState PipelineState) error {
	if psi.stateManager != nil {
		return psi.stateManager.SetState(targetState)
	}
	return fmt.Errorf("no state manager available")
}

// RestartPipeline attempts to restart the pipeline
func (psi *PipelineStateIntegration) RestartPipeline() error {
	if psi.stateManager != nil {
		return psi.stateManager.RestartPipeline()
	}
	return fmt.Errorf("no state manager available")
}

// CheckHealth performs a comprehensive health check
func (psi *PipelineStateIntegration) CheckHealth() OverallHealthStatus {
	if psi.healthChecker != nil {
		status := psi.healthChecker.CheckHealth()

		// Notify callback if configured
		if psi.config.NotifyHealthIssues && psi.healthCallback != nil && status.Overall != HealthStatusHealthy {
			psi.healthCallback(status)
		}

		return status
	}

	// Return unhealthy status if no health checker
	return OverallHealthStatus{
		HealthStatus: HealthStatus{
			Overall:   HealthStatusCritical,
			LastCheck: time.Now().Unix(),
			Uptime:    int64(time.Since(psi.startTime).Seconds()),
			Issues:    []string{"No health checker available"},
			Checks:    make(map[string]interface{}),
		},
		CheckCount:  0,
		FailedCount: 1,
	}
}

// GetHealthStatus returns the last health status
func (psi *PipelineStateIntegration) GetHealthStatus() OverallHealthStatus {
	if psi.healthChecker != nil {
		return psi.healthChecker.GetHealthStatus()
	}
	return psi.CheckHealth()
}

// IsHealthy returns true if the pipeline is healthy
func (psi *PipelineStateIntegration) IsHealthy() bool {
	return psi.CheckHealth().Overall == HealthStatusHealthy
}

// HandleError handles pipeline errors
func (psi *PipelineStateIntegration) HandleError(errorType ErrorType, message, details, component string) {
	if psi.errorHandler != nil {
		psi.errorHandler.HandleError(errorType, message, details, component)
	} else {
		// Log error if no error handler
		psi.logger.WithFields(logrus.Fields{
			"error_type": errorType.String(),
			"component":  component,
		}).Errorf("Pipeline error: %s - %s", message, details)
	}
}

// GetAggregatedMetrics returns aggregated metrics from all components
func (psi *PipelineStateIntegration) GetAggregatedMetrics() *AggregatedMetrics {
	psi.aggregatedMetrics.mu.RLock()
	defer psi.aggregatedMetrics.mu.RUnlock()

	// Create a copy of the metrics
	metrics := &AggregatedMetrics{
		TotalOperations:   psi.aggregatedMetrics.TotalOperations,
		SuccessfulOps:     psi.aggregatedMetrics.SuccessfulOps,
		FailedOps:         psi.aggregatedMetrics.FailedOps,
		LastUpdateTime:    psi.aggregatedMetrics.LastUpdateTime,
		IntegrationUptime: time.Since(psi.startTime),
	}

	// Copy component metrics
	if psi.aggregatedMetrics.StateManager.TotalStateTransitions > 0 {
		metrics.StateManager = psi.aggregatedMetrics.StateManager
	}

	if psi.aggregatedMetrics.HealthChecker != nil {
		metrics.HealthChecker = &HealthCheckerMetrics{}
		*metrics.HealthChecker = *psi.aggregatedMetrics.HealthChecker
	}

	if psi.aggregatedMetrics.ErrorHandler != nil {
		metrics.ErrorHandler = &PipelineErrorHandlerMetrics{}
		*metrics.ErrorHandler = *psi.aggregatedMetrics.ErrorHandler
	}

	return metrics
}

// GetComprehensiveStats returns comprehensive statistics from all components
func (psi *PipelineStateIntegration) GetComprehensiveStats() map[string]interface{} {
	stats := make(map[string]interface{})

	// Integration stats
	stats["integration"] = map[string]interface{}{
		"is_running":    psi.IsRunning(),
		"uptime":        time.Since(psi.startTime).Seconds(),
		"current_state": psi.GetCurrentState().String(),
		"is_healthy":    psi.IsHealthy(),
	}

	// State manager stats
	if psi.stateManager != nil {
		stats["state_manager"] = psi.stateManager.GetStats()
	}

	// Health checker stats
	if psi.healthChecker != nil {
		stats["health_checker"] = map[string]interface{}{
			"metrics":       psi.healthChecker.GetMetrics(),
			"health_status": psi.healthChecker.GetHealthStatus(),
			"checker_names": psi.healthChecker.GetCheckerNames(),
			"is_healthy":    psi.healthChecker.IsHealthy(),
			"issues":        psi.healthChecker.GetIssues(),
		}
	}

	// Error handler stats
	if psi.errorHandler != nil {
		stats["error_handler"] = psi.errorHandler.GetStats()
	}

	// Aggregated metrics
	stats["aggregated_metrics"] = psi.GetAggregatedMetrics()

	return stats
}

// SetStateChangeCallback sets the state change callback
func (psi *PipelineStateIntegration) SetStateChangeCallback(callback func(oldState, newState PipelineState, transition ExtendedStateTransition)) {
	psi.mu.Lock()
	defer psi.mu.Unlock()
	psi.stateChangeCallback = callback
}

// SetErrorCallback sets the error callback
func (psi *PipelineStateIntegration) SetErrorCallback(callback func(ErrorType, string, error)) {
	psi.mu.Lock()
	defer psi.mu.Unlock()
	psi.errorCallback = callback
}

// SetHealthCallback sets the health callback
func (psi *PipelineStateIntegration) SetHealthCallback(callback func(OverallHealthStatus)) {
	psi.mu.Lock()
	defer psi.mu.Unlock()
	psi.healthCallback = callback
}

// onStateChange handles state change events
func (psi *PipelineStateIntegration) onStateChange(oldState, newState PipelineState, transition ExtendedStateTransition) {
	psi.logger.Debugf("State change: %s -> %s (success: %t)", oldState.String(), newState.String(), transition.Success)

	// Update metrics
	psi.aggregatedMetrics.mu.Lock()
	psi.aggregatedMetrics.TotalOperations++
	if transition.Success {
		psi.aggregatedMetrics.SuccessfulOps++
	} else {
		psi.aggregatedMetrics.FailedOps++
	}
	psi.aggregatedMetrics.mu.Unlock()

	// Notify callback
	if psi.stateChangeCallback != nil {
		psi.stateChangeCallback(oldState, newState, transition)
	}
}

// onError handles error events
func (psi *PipelineStateIntegration) onError(errorType ErrorType, message string, err error) {
	psi.logger.WithField("error_type", errorType.String()).Errorf("Pipeline error: %s - %v", message, err)

	// Update metrics
	psi.aggregatedMetrics.mu.Lock()
	psi.aggregatedMetrics.TotalOperations++
	psi.aggregatedMetrics.FailedOps++
	psi.aggregatedMetrics.mu.Unlock()

	// Notify callback
	if psi.errorCallback != nil {
		psi.errorCallback(errorType, message, err)
	}
}

// metricsAggregationLoop runs the metrics aggregation loop
func (psi *PipelineStateIntegration) metricsAggregationLoop() {
	defer psi.wg.Done()

	ticker := time.NewTicker(psi.config.MetricsUpdateInterval)
	defer ticker.Stop()

	psi.logger.Debug("Metrics aggregation loop started")

	for {
		select {
		case <-psi.ctx.Done():
			psi.logger.Debug("Metrics aggregation loop stopped")
			return
		case <-ticker.C:
			psi.updateAggregatedMetrics()
		}
	}
}

// updateAggregatedMetrics updates the aggregated metrics from all components
func (psi *PipelineStateIntegration) updateAggregatedMetrics() {
	psi.aggregatedMetrics.mu.Lock()
	defer psi.aggregatedMetrics.mu.Unlock()

	// Update state manager metrics
	if psi.stateManager != nil {
		psi.aggregatedMetrics.StateManager = psi.stateManager.GetMetrics()
	}

	// Update health checker metrics
	if psi.healthChecker != nil {
		psi.aggregatedMetrics.HealthChecker = psi.healthChecker.GetMetrics()
	}

	// Update error handler metrics
	if psi.errorHandler != nil {
		psi.aggregatedMetrics.ErrorHandler = psi.errorHandler.GetMetrics()
	}

	// Update integration metrics
	psi.aggregatedMetrics.LastUpdateTime = time.Now()
	psi.aggregatedMetrics.IntegrationUptime = time.Since(psi.startTime)
}
