package gstreamer

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// QualityController manages adaptive quality control for media streams
// Implements BitrateController and QualityAdaptor for network-based adjustments
// Performs all encoding parameter optimization within GStreamer components
type QualityController struct {
	// Configuration
	config *QualityControllerConfig
	logger *logrus.Entry

	// Sub-controllers
	bitrateController *BitrateController
	qualityAdaptor    *QualityAdaptor

	// Network monitoring
	networkMonitor *NetworkMonitor

	// Statistics and metrics
	stats      *QualityStatistics
	statsMutex sync.RWMutex

	// Lifecycle management
	ctx      context.Context
	cancel   context.CancelFunc
	running  bool
	stopChan chan struct{}
	wg       sync.WaitGroup
}

// QualityControllerConfig configuration for quality controller
type QualityControllerConfig struct {
	// Bitrate control
	InitialBitrate   int           `json:"initial_bitrate"`
	MinBitrate       int           `json:"min_bitrate"`
	MaxBitrate       int           `json:"max_bitrate"`
	BitrateStep      int           `json:"bitrate_step"`
	AdaptationWindow time.Duration `json:"adaptation_window"`

	// Quality control
	InitialQuality float64 `json:"initial_quality"`
	MinQuality     float64 `json:"min_quality"`
	MaxQuality     float64 `json:"max_quality"`
	QualityStep    float64 `json:"quality_step"`

	// Network thresholds
	RTTThreshold    time.Duration `json:"rtt_threshold"`
	LossThreshold   float64       `json:"loss_threshold"`
	JitterThreshold time.Duration `json:"jitter_threshold"`

	// Adaptation behavior
	AggressiveMode    bool          `json:"aggressive_mode"`
	SmoothTransitions bool          `json:"smooth_transitions"`
	UpdateInterval    time.Duration `json:"update_interval"`

	// Enable/disable features
	EnableBitrateControl bool `json:"enable_bitrate_control"`
	EnableQualityControl bool `json:"enable_quality_control"`
	EnableNetworkMonitor bool `json:"enable_network_monitor"`
}

// DefaultQualityControllerConfig returns default configuration
func DefaultQualityControllerConfig() *QualityControllerConfig {
	return &QualityControllerConfig{
		InitialBitrate:       2000000, // 2 Mbps
		MinBitrate:           500000,  // 500 kbps
		MaxBitrate:           8000000, // 8 Mbps
		BitrateStep:          200000,  // 200 kbps
		AdaptationWindow:     5 * time.Second,
		InitialQuality:       0.8,
		MinQuality:           0.3,
		MaxQuality:           1.0,
		QualityStep:          0.1,
		RTTThreshold:         100 * time.Millisecond,
		LossThreshold:        1.0, // 1%
		JitterThreshold:      20 * time.Millisecond,
		AggressiveMode:       false,
		SmoothTransitions:    true,
		UpdateInterval:       2 * time.Second,
		EnableBitrateControl: true,
		EnableQualityControl: true,
		EnableNetworkMonitor: true,
	}
}

// BitrateController handles adaptive bitrate control
type BitrateController struct {
	mu     sync.RWMutex
	config *QualityControllerConfig
	logger *logrus.Entry

	// Current state
	currentBitrate int
	targetBitrate  int
	lastAdjustment time.Time

	// History tracking
	bitrateHistory []BitratePoint
	maxHistorySize int

	// Adaptation logic
	adaptationState  AdaptationState
	consecutiveSteps int

	// Callbacks
	onBitrateChange func(int) error
}

// QualityAdaptor handles quality adaptation based on network conditions
type QualityAdaptor struct {
	mu     sync.RWMutex
	config *QualityControllerConfig
	logger *logrus.Entry

	// Current state
	currentQuality float64
	targetQuality  float64
	lastAdjustment time.Time

	// History tracking
	qualityHistory []QualityPoint
	maxHistorySize int

	// Network condition tracking
	networkHistory []NetworkCondition

	// Callbacks
	onQualityChange func(float64) error
}

// NetworkMonitor monitors network conditions for quality adaptation
type NetworkMonitor struct {
	mu     sync.RWMutex
	config *QualityControllerConfig
	logger *logrus.Entry

	// Current conditions
	currentCondition NetworkCondition
	lastUpdate       time.Time

	// History tracking
	conditionHistory []NetworkCondition
	maxHistorySize   int

	// Monitoring state
	monitoring bool

	// Callbacks
	onConditionChange func(NetworkCondition) error
}

// BitratePoint represents a point in bitrate history
type BitratePoint struct {
	Timestamp time.Time `json:"timestamp"`
	Bitrate   int       `json:"bitrate"`
	Quality   float64   `json:"quality"`
	RTT       int64     `json:"rtt_ms"`
	Loss      float64   `json:"loss_percent"`
}

// QualityPoint represents a point in quality history
type QualityPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Quality   float64   `json:"quality"`
	Bitrate   int       `json:"bitrate"`
	Score     float64   `json:"score"`
}

// AdaptationState represents the current adaptation state
type AdaptationState int

const (
	AdaptationStateStable AdaptationState = iota
	AdaptationStateIncreasing
	AdaptationStateDecreasing
	AdaptationStateRecovering
)

// QualityStatistics tracks quality control metrics
type QualityStatistics struct {
	// Bitrate statistics
	CurrentBitrate    int       `json:"current_bitrate"`
	TargetBitrate     int       `json:"target_bitrate"`
	MinBitrate        int       `json:"min_bitrate"`
	MaxBitrate        int       `json:"max_bitrate"`
	AvgBitrate        int       `json:"avg_bitrate"`
	BitrateChanges    int64     `json:"bitrate_changes"`
	LastBitrateChange time.Time `json:"last_bitrate_change"`

	// Quality statistics
	CurrentQuality    float64   `json:"current_quality"`
	TargetQuality     float64   `json:"target_quality"`
	MinQuality        float64   `json:"min_quality"`
	MaxQuality        float64   `json:"max_quality"`
	AvgQuality        float64   `json:"avg_quality"`
	QualityChanges    int64     `json:"quality_changes"`
	LastQualityChange time.Time `json:"last_quality_change"`

	// Network statistics
	CurrentRTT        int64     `json:"current_rtt_ms"`
	CurrentLoss       float64   `json:"current_loss_percent"`
	CurrentJitter     int64     `json:"current_jitter_ms"`
	NetworkUpdates    int64     `json:"network_updates"`
	LastNetworkUpdate time.Time `json:"last_network_update"`

	// Adaptation statistics
	AdaptationState  string `json:"adaptation_state"`
	TotalAdaptations int64  `json:"total_adaptations"`
	SuccessfulAdapts int64  `json:"successful_adaptations"`
	FailedAdapts     int64  `json:"failed_adaptations"`

	// Performance metrics
	AdaptationLatency int64   `json:"adaptation_latency_ms"`
	UpdateFrequency   float64 `json:"update_frequency_hz"`
}

// NewQualityController creates a new quality controller
func NewQualityController(ctx context.Context, config *QualityControllerConfig, logger *logrus.Entry) *QualityController {
	if config == nil {
		config = DefaultQualityControllerConfig()
	}
	if logger == nil {
		logger = logrus.WithField("component", "quality-controller")
	}

	childCtx, cancel := context.WithCancel(ctx)

	qc := &QualityController{
		config:   config,
		logger:   logger,
		ctx:      childCtx,
		cancel:   cancel,
		stopChan: make(chan struct{}),
		stats: &QualityStatistics{
			CurrentBitrate: config.InitialBitrate,
			TargetBitrate:  config.InitialBitrate,
			MinBitrate:     config.MinBitrate,
			MaxBitrate:     config.MaxBitrate,
			CurrentQuality: config.InitialQuality,
			TargetQuality:  config.InitialQuality,
			MinQuality:     config.MinQuality,
			MaxQuality:     config.MaxQuality,
		},
	}

	// Initialize sub-controllers
	if config.EnableBitrateControl {
		qc.bitrateController = NewBitrateController(config, logger)
	}

	if config.EnableQualityControl {
		qc.qualityAdaptor = NewQualityAdaptor(config, logger)
	}

	if config.EnableNetworkMonitor {
		qc.networkMonitor = NewNetworkMonitor(config, logger)
	}

	return qc
}

// Start starts the quality controller
func (qc *QualityController) Start() error {
	if qc.running {
		return fmt.Errorf("quality controller already running")
	}

	qc.logger.Debug("Starting quality controller")

	// Start sub-controllers
	if qc.bitrateController != nil {
		if err := qc.bitrateController.Start(); err != nil {
			return fmt.Errorf("failed to start bitrate controller: %w", err)
		}
	}

	if qc.qualityAdaptor != nil {
		if err := qc.qualityAdaptor.Start(); err != nil {
			return fmt.Errorf("failed to start quality adaptor: %w", err)
		}
	}

	if qc.networkMonitor != nil {
		if err := qc.networkMonitor.Start(); err != nil {
			return fmt.Errorf("failed to start network monitor: %w", err)
		}
	}

	// Start main control loop
	qc.wg.Add(1)
	go qc.controlLoop()

	qc.running = true
	qc.logger.Info("Quality controller started successfully")
	return nil
}

// Stop stops the quality controller
func (qc *QualityController) Stop() error {
	if !qc.running {
		return nil
	}

	qc.logger.Debug("Stopping quality controller")

	// Stop main loop
	qc.cancel()
	close(qc.stopChan)

	// Stop sub-controllers
	if qc.networkMonitor != nil {
		qc.networkMonitor.Stop()
	}
	if qc.qualityAdaptor != nil {
		qc.qualityAdaptor.Stop()
	}
	if qc.bitrateController != nil {
		qc.bitrateController.Stop()
	}

	// Wait for goroutines to finish
	qc.wg.Wait()

	qc.running = false
	qc.logger.Info("Quality controller stopped successfully")
	return nil
}

// AdaptToNetworkCondition adapts quality based on network conditions
func (qc *QualityController) AdaptToNetworkCondition(condition NetworkCondition) error {
	if !qc.running {
		return fmt.Errorf("quality controller not running")
	}

	// Update network monitor
	if qc.networkMonitor != nil {
		qc.networkMonitor.UpdateCondition(condition)
	}

	// Calculate adaptations
	bitrateChange := false
	qualityChange := false

	// Adapt bitrate if enabled
	if qc.bitrateController != nil {
		newBitrate := qc.calculateTargetBitrate(condition)
		if newBitrate != qc.stats.CurrentBitrate {
			if err := qc.bitrateController.SetTargetBitrate(newBitrate); err != nil {
				qc.logger.Warnf("Failed to set target bitrate: %v", err)
			} else {
				bitrateChange = true
			}
		}
	}

	// Adapt quality if enabled
	if qc.qualityAdaptor != nil {
		newQuality := qc.calculateTargetQuality(condition)
		if math.Abs(newQuality-qc.stats.CurrentQuality) > 0.05 { // 5% threshold
			if err := qc.qualityAdaptor.SetTargetQuality(newQuality); err != nil {
				qc.logger.Warnf("Failed to set target quality: %v", err)
			} else {
				qualityChange = true
			}
		}
	}

	// Update statistics
	qc.updateNetworkStats(condition)
	if bitrateChange || qualityChange {
		qc.statsMutex.Lock()
		qc.stats.TotalAdaptations++
		qc.statsMutex.Unlock()
	}

	qc.logger.Debugf("Adapted to network condition: RTT=%dms, Loss=%.2f%%, Jitter=%dms, BitrateChange=%v, QualityChange=%v",
		condition.RTT, condition.PacketLoss, condition.Jitter, bitrateChange, qualityChange)

	return nil
}

// GetStatistics returns current quality control statistics
func (qc *QualityController) GetStatistics() QualityStatistics {
	qc.statsMutex.RLock()
	defer qc.statsMutex.RUnlock()
	return *qc.stats
}

// SetBitrateChangeCallback sets callback for bitrate changes
func (qc *QualityController) SetBitrateChangeCallback(callback func(int) error) {
	if qc.bitrateController != nil {
		qc.bitrateController.SetChangeCallback(callback)
	}
}

// SetQualityChangeCallback sets callback for quality changes
func (qc *QualityController) SetQualityChangeCallback(callback func(float64) error) {
	if qc.qualityAdaptor != nil {
		qc.qualityAdaptor.SetChangeCallback(callback)
	}
}

// controlLoop main control loop for quality controller
func (qc *QualityController) controlLoop() {
	defer qc.wg.Done()

	ticker := time.NewTicker(qc.config.UpdateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			qc.performPeriodicUpdate()

		case <-qc.stopChan:
			return
		case <-qc.ctx.Done():
			return
		}
	}
}

// performPeriodicUpdate performs periodic quality control updates
func (qc *QualityController) performPeriodicUpdate() {
	// Update statistics
	qc.updateStatistics()

	// Check for adaptation opportunities
	if qc.shouldAdapt() {
		qc.performAdaptation()
	}

	// Clean up old history
	qc.cleanupHistory()
}

// calculateTargetBitrate calculates target bitrate based on network conditions
func (qc *QualityController) calculateTargetBitrate(condition NetworkCondition) int {
	baseBitrate := qc.config.InitialBitrate

	// Adjust based on available bandwidth
	if condition.Bandwidth > 0 {
		// Use 80% of available bandwidth
		maxBitrate := int(float64(condition.Bandwidth) * 0.8)
		if maxBitrate < baseBitrate {
			baseBitrate = maxBitrate
		}
	}

	// Adjust based on packet loss
	if condition.PacketLoss > qc.config.LossThreshold {
		lossReduction := 1.0 - (condition.PacketLoss / 100.0 * 0.5)
		baseBitrate = int(float64(baseBitrate) * lossReduction)
	}

	// Adjust based on RTT
	if condition.RTT > int64(qc.config.RTTThreshold.Milliseconds()) {
		rttReduction := 1.0 - (float64(condition.RTT-int64(qc.config.RTTThreshold.Milliseconds())) / 1000.0 * 0.3)
		if rttReduction < 0.5 {
			rttReduction = 0.5
		}
		baseBitrate = int(float64(baseBitrate) * rttReduction)
	}

	// Adjust based on jitter
	if condition.Jitter > int64(qc.config.JitterThreshold.Milliseconds()) {
		jitterReduction := 1.0 - (float64(condition.Jitter-int64(qc.config.JitterThreshold.Milliseconds())) / 100.0 * 0.2)
		if jitterReduction < 0.7 {
			jitterReduction = 0.7
		}
		baseBitrate = int(float64(baseBitrate) * jitterReduction)
	}

	// Ensure bitrate is within bounds
	if baseBitrate < qc.config.MinBitrate {
		baseBitrate = qc.config.MinBitrate
	}
	if baseBitrate > qc.config.MaxBitrate {
		baseBitrate = qc.config.MaxBitrate
	}

	return baseBitrate
}

// calculateTargetQuality calculates target quality based on network conditions
func (qc *QualityController) calculateTargetQuality(condition NetworkCondition) float64 {
	baseQuality := qc.config.InitialQuality

	// Adjust based on network conditions
	if condition.PacketLoss > qc.config.LossThreshold {
		qualityReduction := condition.PacketLoss / 100.0 * 0.3
		baseQuality -= qualityReduction
	}

	if condition.RTT > int64(qc.config.RTTThreshold.Milliseconds()) {
		rttReduction := float64(condition.RTT-int64(qc.config.RTTThreshold.Milliseconds())) / 1000.0 * 0.2
		baseQuality -= rttReduction
	}

	if condition.Jitter > int64(qc.config.JitterThreshold.Milliseconds()) {
		jitterReduction := float64(condition.Jitter-int64(qc.config.JitterThreshold.Milliseconds())) / 100.0 * 0.1
		baseQuality -= jitterReduction
	}

	// Ensure quality is within bounds
	if baseQuality < qc.config.MinQuality {
		baseQuality = qc.config.MinQuality
	}
	if baseQuality > qc.config.MaxQuality {
		baseQuality = qc.config.MaxQuality
	}

	return baseQuality
}

// updateNetworkStats updates network-related statistics
func (qc *QualityController) updateNetworkStats(condition NetworkCondition) {
	qc.statsMutex.Lock()
	defer qc.statsMutex.Unlock()

	qc.stats.CurrentRTT = condition.RTT
	qc.stats.CurrentLoss = condition.PacketLoss
	qc.stats.CurrentJitter = condition.Jitter
	qc.stats.NetworkUpdates++
	qc.stats.LastNetworkUpdate = time.Now()
}

// updateStatistics updates internal statistics
func (qc *QualityController) updateStatistics() {
	qc.statsMutex.Lock()
	defer qc.statsMutex.Unlock()

	// Update frequency calculation
	now := time.Now()
	if !qc.stats.LastNetworkUpdate.IsZero() {
		elapsed := now.Sub(qc.stats.LastNetworkUpdate).Seconds()
		if elapsed > 0 {
			qc.stats.UpdateFrequency = 1.0 / elapsed
		}
	}

	// Update adaptation state
	if qc.bitrateController != nil {
		qc.stats.AdaptationState = qc.bitrateController.GetAdaptationState().String()
	}
}

// shouldAdapt determines if adaptation should be performed
func (qc *QualityController) shouldAdapt() bool {
	// Check if enough time has passed since last adaptation
	now := time.Now()
	if now.Sub(qc.stats.LastBitrateChange) < qc.config.AdaptationWindow {
		return false
	}
	if now.Sub(qc.stats.LastQualityChange) < qc.config.AdaptationWindow {
		return false
	}

	return true
}

// performAdaptation performs quality adaptation
func (qc *QualityController) performAdaptation() {
	// Implementation would trigger adaptation based on current conditions
	// This is a placeholder for more complex adaptation logic
	qc.logger.Debug("Performing quality adaptation")
}

// cleanupHistory cleans up old history entries
func (qc *QualityController) cleanupHistory() {
	// Clean up history in sub-controllers
	if qc.bitrateController != nil {
		qc.bitrateController.CleanupHistory()
	}
	if qc.qualityAdaptor != nil {
		qc.qualityAdaptor.CleanupHistory()
	}
	if qc.networkMonitor != nil {
		qc.networkMonitor.CleanupHistory()
	}
}

// String returns string representation of adaptation state
func (as AdaptationState) String() string {
	switch as {
	case AdaptationStateStable:
		return "stable"
	case AdaptationStateIncreasing:
		return "increasing"
	case AdaptationStateDecreasing:
		return "decreasing"
	case AdaptationStateRecovering:
		return "recovering"
	default:
		return "unknown"
	}
}

// NewBitrateController creates a new bitrate controller
func NewBitrateController(config *QualityControllerConfig, logger *logrus.Entry) *BitrateController {
	return &BitrateController{
		config:          config,
		logger:          logger,
		currentBitrate:  config.InitialBitrate,
		targetBitrate:   config.InitialBitrate,
		maxHistorySize:  100,
		adaptationState: AdaptationStateStable,
	}
}

// Start starts the bitrate controller
func (bc *BitrateController) Start() error {
	bc.logger.Debug("Bitrate controller started")
	return nil
}

// Stop stops the bitrate controller
func (bc *BitrateController) Stop() error {
	bc.logger.Debug("Bitrate controller stopped")
	return nil
}

// SetTargetBitrate sets the target bitrate
func (bc *BitrateController) SetTargetBitrate(bitrate int) error {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	if bitrate == bc.targetBitrate {
		return nil
	}

	bc.targetBitrate = bitrate
	bc.lastAdjustment = time.Now()

	// Add to history
	point := BitratePoint{
		Timestamp: time.Now(),
		Bitrate:   bitrate,
	}
	bc.bitrateHistory = append(bc.bitrateHistory, point)

	// Trigger callback if set
	if bc.onBitrateChange != nil {
		if err := bc.onBitrateChange(bitrate); err != nil {
			return fmt.Errorf("bitrate change callback failed: %w", err)
		}
	}

	bc.currentBitrate = bitrate
	bc.logger.Debugf("Bitrate changed to %d", bitrate)
	return nil
}

// SetChangeCallback sets the bitrate change callback
func (bc *BitrateController) SetChangeCallback(callback func(int) error) {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	bc.onBitrateChange = callback
}

// GetAdaptationState returns the current adaptation state
func (bc *BitrateController) GetAdaptationState() AdaptationState {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	return bc.adaptationState
}

// CleanupHistory cleans up old history entries
func (bc *BitrateController) CleanupHistory() {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	if len(bc.bitrateHistory) > bc.maxHistorySize {
		bc.bitrateHistory = bc.bitrateHistory[len(bc.bitrateHistory)-bc.maxHistorySize:]
	}
}

// NewQualityAdaptor creates a new quality adaptor
func NewQualityAdaptor(config *QualityControllerConfig, logger *logrus.Entry) *QualityAdaptor {
	return &QualityAdaptor{
		config:         config,
		logger:         logger,
		currentQuality: config.InitialQuality,
		targetQuality:  config.InitialQuality,
		maxHistorySize: 100,
	}
}

// Start starts the quality adaptor
func (qa *QualityAdaptor) Start() error {
	qa.logger.Debug("Quality adaptor started")
	return nil
}

// Stop stops the quality adaptor
func (qa *QualityAdaptor) Stop() error {
	qa.logger.Debug("Quality adaptor stopped")
	return nil
}

// SetTargetQuality sets the target quality
func (qa *QualityAdaptor) SetTargetQuality(quality float64) error {
	qa.mu.Lock()
	defer qa.mu.Unlock()

	if math.Abs(quality-qa.targetQuality) < 0.01 {
		return nil
	}

	qa.targetQuality = quality
	qa.lastAdjustment = time.Now()

	// Add to history
	point := QualityPoint{
		Timestamp: time.Now(),
		Quality:   quality,
	}
	qa.qualityHistory = append(qa.qualityHistory, point)

	// Trigger callback if set
	if qa.onQualityChange != nil {
		if err := qa.onQualityChange(quality); err != nil {
			return fmt.Errorf("quality change callback failed: %w", err)
		}
	}

	qa.currentQuality = quality
	qa.logger.Debugf("Quality changed to %.2f", quality)
	return nil
}

// SetChangeCallback sets the quality change callback
func (qa *QualityAdaptor) SetChangeCallback(callback func(float64) error) {
	qa.mu.Lock()
	defer qa.mu.Unlock()
	qa.onQualityChange = callback
}

// CleanupHistory cleans up old history entries
func (qa *QualityAdaptor) CleanupHistory() {
	qa.mu.Lock()
	defer qa.mu.Unlock()

	if len(qa.qualityHistory) > qa.maxHistorySize {
		qa.qualityHistory = qa.qualityHistory[len(qa.qualityHistory)-qa.maxHistorySize:]
	}
}

// NewNetworkMonitor creates a new network monitor
func NewNetworkMonitor(config *QualityControllerConfig, logger *logrus.Entry) *NetworkMonitor {
	return &NetworkMonitor{
		config:         config,
		logger:         logger,
		maxHistorySize: 100,
	}
}

// Start starts the network monitor
func (nm *NetworkMonitor) Start() error {
	nm.mu.Lock()
	defer nm.mu.Unlock()
	nm.monitoring = true
	nm.logger.Debug("Network monitor started")
	return nil
}

// Stop stops the network monitor
func (nm *NetworkMonitor) Stop() error {
	nm.mu.Lock()
	defer nm.mu.Unlock()
	nm.monitoring = false
	nm.logger.Debug("Network monitor stopped")
	return nil
}

// UpdateCondition updates the current network condition
func (nm *NetworkMonitor) UpdateCondition(condition NetworkCondition) {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	nm.currentCondition = condition
	nm.lastUpdate = time.Now()

	// Add to history
	nm.conditionHistory = append(nm.conditionHistory, condition)

	// Trigger callback if set
	if nm.onConditionChange != nil {
		go func() {
			if err := nm.onConditionChange(condition); err != nil {
				nm.logger.Warnf("Network condition change callback failed: %v", err)
			}
		}()
	}
}

// CleanupHistory cleans up old history entries
func (nm *NetworkMonitor) CleanupHistory() {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	if len(nm.conditionHistory) > nm.maxHistorySize {
		nm.conditionHistory = nm.conditionHistory[len(nm.conditionHistory)-nm.maxHistorySize:]
	}
}
