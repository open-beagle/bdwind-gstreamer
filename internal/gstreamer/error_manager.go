package gstreamer

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/go-gst/go-gst/gst"
	"github.com/sirupsen/logrus"
)

// Use existing GStreamerErrorType from errors.go
// Additional error types for the error manager
const (
	ErrorTypeEncoding GStreamerErrorType = iota + 100 // Start from 100 to avoid conflicts
	ErrorTypeSystem
	ErrorTypeStateChange
	ErrorTypeElementFailure
	ErrorTypeResourceUnavailable
	ErrorTypeMemoryExhaustion
)

func (et GStreamerErrorType) StringExtended() string {
	// First check existing types
	if s := et.String(); s != "Unknown" {
		return s
	}

	// Then check extended types
	switch et {
	case ErrorTypeEncoding:
		return "ENCODING"
	case ErrorTypeSystem:
		return "SYSTEM"
	case ErrorTypeStateChange:
		return "STATE_CHANGE"
	case ErrorTypeElementFailure:
		return "ELEMENT_FAILURE"
	case ErrorTypeResourceUnavailable:
		return "RESOURCE_UNAVAILABLE"
	case ErrorTypeMemoryExhaustion:
		return "MEMORY_EXHAUSTION"
	default:
		return "UNKNOWN"
	}
}

// ErrorSeverity represents the severity level of an error
type ErrorSeverity int

const (
	ErrorSeverityLow ErrorSeverity = iota
	ErrorSeverityMedium
	ErrorSeverityHigh
	ErrorSeverityCritical
)

func (es ErrorSeverity) String() string {
	switch es {
	case ErrorSeverityLow:
		return "LOW"
	case ErrorSeverityMedium:
		return "MEDIUM"
	case ErrorSeverityHigh:
		return "HIGH"
	case ErrorSeverityCritical:
		return "CRITICAL"
	default:
		return "UNKNOWN"
	}
}

// ErrorManagerConfig configuration for the error manager
type ErrorManagerConfig struct {
	MaxErrorHistory       int           `yaml:"max_error_history" json:"max_error_history"`
	RecoveryTimeout       time.Duration `yaml:"recovery_timeout" json:"recovery_timeout"`
	MaxRecoveryAttempts   int           `yaml:"max_recovery_attempts" json:"max_recovery_attempts"`
	RecoveryDelay         time.Duration `yaml:"recovery_delay" json:"recovery_delay"`
	ErrorThrottleInterval time.Duration `yaml:"error_throttle_interval" json:"error_throttle_interval"`
	EnableAutoRecovery    bool          `yaml:"enable_auto_recovery" json:"enable_auto_recovery"`
}

// DefaultErrorManagerConfig returns default error manager configuration
func DefaultErrorManagerConfig() *ErrorManagerConfig {
	return &ErrorManagerConfig{
		MaxErrorHistory:       100,
		RecoveryTimeout:       30 * time.Second,
		MaxRecoveryAttempts:   3,
		RecoveryDelay:         5 * time.Second,
		ErrorThrottleInterval: 1 * time.Second,
		EnableAutoRecovery:    true,
	}
}

// ErrorEvent represents an error event
type ErrorEvent struct {
	ID           uint64             `json:"id"`
	Type         GStreamerErrorType `json:"type"`
	Severity     ErrorSeverity      `json:"severity"`
	Message      string             `json:"message"`
	Details      string             `json:"details"`
	Source       string             `json:"source"`
	Timestamp    time.Time          `json:"timestamp"`
	Recovered    bool               `json:"recovered"`
	RecoveryTime time.Time          `json:"recovery_time,omitempty"`
	Attempts     int                `json:"attempts"`
}

// RecoveryStrategy defines how to recover from different types of errors
type RecoveryStrategy interface {
	CanRecover(errorType GStreamerErrorType) bool
	Recover(ctx context.Context, errorEvent *ErrorEvent) error
	GetRecoveryTime() time.Duration
	GetDescription() string
}

// RestartRecoveryStrategy restarts components to recover from errors
type RestartRecoveryStrategy struct {
	description string
	timeout     time.Duration
}

// NewRestartRecoveryStrategy creates a new restart recovery strategy
func NewRestartRecoveryStrategy(timeout time.Duration) *RestartRecoveryStrategy {
	return &RestartRecoveryStrategy{
		description: "Restart components to recover from error",
		timeout:     timeout,
	}
}

func (rrs *RestartRecoveryStrategy) CanRecover(errorType GStreamerErrorType) bool {
	// Can recover from most errors by restarting
	switch errorType {
	case ErrorTypeConfiguration, ErrorTypePermission:
		return false // These require manual intervention
	default:
		return true
	}
}

func (rrs *RestartRecoveryStrategy) Recover(ctx context.Context, errorEvent *ErrorEvent) error {
	// Implementation would restart the affected component
	// This is a placeholder implementation
	time.Sleep(100 * time.Millisecond) // Simulate restart time
	return nil
}

func (rrs *RestartRecoveryStrategy) GetRecoveryTime() time.Duration {
	return rrs.timeout
}

func (rrs *RestartRecoveryStrategy) GetDescription() string {
	return rrs.description
}

// ResetRecoveryStrategy resets pipeline state to recover from errors
type ResetRecoveryStrategy struct {
	description string
	timeout     time.Duration
}

// NewResetRecoveryStrategy creates a new reset recovery strategy
func NewResetRecoveryStrategy(timeout time.Duration) *ResetRecoveryStrategy {
	return &ResetRecoveryStrategy{
		description: "Reset pipeline state to recover from error",
		timeout:     timeout,
	}
}

func (rrs *ResetRecoveryStrategy) CanRecover(errorType GStreamerErrorType) bool {
	// Can recover from state and element errors by resetting
	switch errorType {
	case ErrorTypeStateChange, ErrorTypeElementFailure, ErrorTypeFormatNegotiation:
		return true
	default:
		return false
	}
}

func (rrs *ResetRecoveryStrategy) Recover(ctx context.Context, errorEvent *ErrorEvent) error {
	// Implementation would reset pipeline state
	// This is a placeholder implementation
	time.Sleep(200 * time.Millisecond) // Simulate reset time
	return nil
}

func (rrs *ResetRecoveryStrategy) GetRecoveryTime() time.Duration {
	return rrs.timeout
}

func (rrs *ResetRecoveryStrategy) GetDescription() string {
	return rrs.description
}

// ErrorStatistics holds error management statistics
type ErrorStatistics struct {
	TotalErrors          int64                        `json:"total_errors"`
	ErrorsByType         map[GStreamerErrorType]int64 `json:"errors_by_type"`
	ErrorsBySeverity     map[ErrorSeverity]int64      `json:"errors_by_severity"`
	RecoveryAttempts     int64                        `json:"recovery_attempts"`
	SuccessfulRecoveries int64                        `json:"successful_recoveries"`
	FailedRecoveries     int64                        `json:"failed_recoveries"`
	LastError            *ErrorEvent                  `json:"last_error,omitempty"`
	LastRecovery         time.Time                    `json:"last_recovery,omitempty"`
	AverageRecoveryTime  time.Duration                `json:"average_recovery_time"`
}

// ErrorCallback is called when an error occurs
type ErrorCallback func(errorEvent *ErrorEvent)

// ErrorManager manages error handling and recovery strategies
type ErrorManager struct {
	config *ErrorManagerConfig
	logger *logrus.Entry

	// Error tracking
	errorHistory []ErrorEvent
	nextErrorID  uint64
	historyMutex sync.RWMutex

	// Recovery strategies
	strategies []RecoveryStrategy

	// Statistics
	stats      ErrorStatistics
	statsMutex sync.RWMutex

	// Error throttling
	lastErrorTime map[GStreamerErrorType]time.Time
	throttleMutex sync.RWMutex

	// Callbacks
	errorCallback ErrorCallback
	callbackMutex sync.RWMutex

	// Lifecycle management
	ctx      context.Context
	cancel   context.CancelFunc
	running  bool
	stopChan chan struct{}
	wg       sync.WaitGroup
}

// NewErrorManager creates a new error manager
func NewErrorManager(config *ErrorManagerConfig, logger *logrus.Entry) *ErrorManager {
	if config == nil {
		config = DefaultErrorManagerConfig()
	}
	if logger == nil {
		logger = logrus.NewEntry(logrus.StandardLogger())
	}

	em := &ErrorManager{
		config:        config,
		logger:        logger,
		errorHistory:  make([]ErrorEvent, 0, config.MaxErrorHistory),
		lastErrorTime: make(map[GStreamerErrorType]time.Time),
		stopChan:      make(chan struct{}),
		stats: ErrorStatistics{
			ErrorsByType:     make(map[GStreamerErrorType]int64),
			ErrorsBySeverity: make(map[ErrorSeverity]int64),
		},
	}

	// Initialize default recovery strategies
	em.initializeRecoveryStrategies()

	em.logger.Debug("Error manager created successfully")
	return em
}

// initializeRecoveryStrategies initializes default recovery strategies
func (em *ErrorManager) initializeRecoveryStrategies() {
	// Add restart recovery strategy
	restartStrategy := NewRestartRecoveryStrategy(em.config.RecoveryTimeout)
	em.strategies = append(em.strategies, restartStrategy)

	// Add reset recovery strategy
	resetStrategy := NewResetRecoveryStrategy(em.config.RecoveryTimeout)
	em.strategies = append(em.strategies, resetStrategy)

	em.logger.Debugf("Initialized %d recovery strategies", len(em.strategies))
}

// Start starts the error manager
func (em *ErrorManager) Start() error {
	em.historyMutex.Lock()
	defer em.historyMutex.Unlock()

	if em.running {
		return fmt.Errorf("error manager already running")
	}

	em.logger.Debug("Starting error manager")

	// Create context for lifecycle management
	em.ctx, em.cancel = context.WithCancel(context.Background())

	// Start background monitoring if needed
	if em.config.EnableAutoRecovery {
		em.wg.Add(1)
		go em.recoveryMonitorLoop()
	}

	em.running = true
	em.logger.Info("Error manager started successfully")
	return nil
}

// Stop stops the error manager
func (em *ErrorManager) Stop() error {
	em.historyMutex.Lock()
	defer em.historyMutex.Unlock()

	if !em.running {
		return nil
	}

	em.logger.Debug("Stopping error manager")

	// Cancel context and stop background tasks
	em.cancel()
	close(em.stopChan)
	em.wg.Wait()

	em.running = false
	em.logger.Info("Error manager stopped successfully")
	return nil
}

// SetErrorCallback sets the callback for error events
func (em *ErrorManager) SetErrorCallback(callback ErrorCallback) {
	em.callbackMutex.Lock()
	defer em.callbackMutex.Unlock()
	em.errorCallback = callback
}

// HandleError handles a generic error
func (em *ErrorManager) HandleError(errorType GStreamerErrorType, message, details, source string) {
	severity := em.classifyErrorSeverity(errorType, message)

	errorEvent := &ErrorEvent{
		Type:      errorType,
		Severity:  severity,
		Message:   message,
		Details:   details,
		Source:    source,
		Timestamp: time.Now(),
		Recovered: false,
		Attempts:  0,
	}

	em.processError(errorEvent)
}

// HandlePipelineError handles errors from GStreamer pipeline messages
func (em *ErrorManager) HandlePipelineError(msg *gst.Message) {
	if msg == nil {
		return
	}

	// Extract error information from GStreamer message
	err := msg.ParseError()

	errorType := em.classifyGStreamerError(err)
	severity := em.classifyErrorSeverity(errorType, err.Error())

	errorEvent := &ErrorEvent{
		Type:      errorType,
		Severity:  severity,
		Message:   err.Error(),
		Details:   "", // Debug info not available from ParseError
		Source:    "GStreamer Pipeline",
		Timestamp: time.Now(),
		Recovered: false,
		Attempts:  0,
	}

	em.processError(errorEvent)
}

// processError processes an error event
func (em *ErrorManager) processError(errorEvent *ErrorEvent) {
	// Check if error should be throttled
	if em.shouldThrottleError(errorEvent.Type) {
		em.logger.Debugf("Error throttled: %s", errorEvent.Type.String())
		return
	}

	// Assign unique ID
	em.historyMutex.Lock()
	em.nextErrorID++
	errorEvent.ID = em.nextErrorID
	em.historyMutex.Unlock()

	em.logger.Errorf("Processing error: ID=%d, Type=%s, Severity=%s, Message=%s",
		errorEvent.ID, errorEvent.Type.String(), errorEvent.Severity.String(), errorEvent.Message)

	// Add to history
	em.addToHistory(errorEvent)

	// Update statistics
	em.updateStatistics(errorEvent)

	// Update throttle time
	em.updateThrottleTime(errorEvent.Type)

	// Call error callback if set
	em.callErrorCallback(errorEvent)

	// Attempt recovery if enabled and appropriate
	if em.config.EnableAutoRecovery && em.shouldAttemptRecovery(errorEvent) {
		go em.attemptRecovery(errorEvent)
	}
}

// shouldThrottleError checks if an error should be throttled
func (em *ErrorManager) shouldThrottleError(errorType GStreamerErrorType) bool {
	em.throttleMutex.RLock()
	defer em.throttleMutex.RUnlock()

	lastTime, exists := em.lastErrorTime[errorType]
	if !exists {
		return false
	}

	return time.Since(lastTime) < em.config.ErrorThrottleInterval
}

// updateThrottleTime updates the last error time for throttling
func (em *ErrorManager) updateThrottleTime(errorType GStreamerErrorType) {
	em.throttleMutex.Lock()
	defer em.throttleMutex.Unlock()
	em.lastErrorTime[errorType] = time.Now()
}

// addToHistory adds an error event to the history
func (em *ErrorManager) addToHistory(errorEvent *ErrorEvent) {
	em.historyMutex.Lock()
	defer em.historyMutex.Unlock()

	// Add to history
	em.errorHistory = append(em.errorHistory, *errorEvent)

	// Maintain max history size
	if len(em.errorHistory) > em.config.MaxErrorHistory {
		em.errorHistory = em.errorHistory[1:]
	}
}

// updateStatistics updates error statistics
func (em *ErrorManager) updateStatistics(errorEvent *ErrorEvent) {
	em.statsMutex.Lock()
	defer em.statsMutex.Unlock()

	em.stats.TotalErrors++
	em.stats.ErrorsByType[errorEvent.Type]++
	em.stats.ErrorsBySeverity[errorEvent.Severity]++
	em.stats.LastError = errorEvent
}

// shouldAttemptRecovery determines if recovery should be attempted
func (em *ErrorManager) shouldAttemptRecovery(errorEvent *ErrorEvent) bool {
	// Don't attempt recovery for low severity errors
	if errorEvent.Severity == ErrorSeverityLow {
		return false
	}

	// Don't attempt recovery if max attempts reached
	if errorEvent.Attempts >= em.config.MaxRecoveryAttempts {
		return false
	}

	// Check if any strategy can handle this error type
	for _, strategy := range em.strategies {
		if strategy.CanRecover(errorEvent.Type) {
			return true
		}
	}

	return false
}

// attemptRecovery attempts to recover from an error
func (em *ErrorManager) attemptRecovery(errorEvent *ErrorEvent) {
	em.logger.Infof("Attempting recovery for error: ID=%d, Type=%s", errorEvent.ID, errorEvent.Type.String())

	// Find appropriate recovery strategy
	var selectedStrategy RecoveryStrategy
	for _, strategy := range em.strategies {
		if strategy.CanRecover(errorEvent.Type) {
			selectedStrategy = strategy
			break
		}
	}

	if selectedStrategy == nil {
		em.logger.Warnf("No recovery strategy found for error type: %s", errorEvent.Type.String())
		return
	}

	// Update attempt count
	errorEvent.Attempts++

	// Update statistics
	em.statsMutex.Lock()
	em.stats.RecoveryAttempts++
	em.statsMutex.Unlock()

	em.logger.Infof("Using recovery strategy: %s (attempt %d/%d)",
		selectedStrategy.GetDescription(), errorEvent.Attempts, em.config.MaxRecoveryAttempts)

	// Wait for recovery delay if this is a retry
	if errorEvent.Attempts > 1 {
		time.Sleep(em.config.RecoveryDelay)
	}

	// Create recovery context with timeout
	recoveryCtx, cancel := context.WithTimeout(em.ctx, selectedStrategy.GetRecoveryTime())
	defer cancel()

	// Attempt recovery
	startTime := time.Now()
	err := selectedStrategy.Recover(recoveryCtx, errorEvent)
	recoveryDuration := time.Since(startTime)

	if err != nil {
		em.logger.Errorf("Recovery failed for error ID=%d: %v", errorEvent.ID, err)

		// Update statistics
		em.statsMutex.Lock()
		em.stats.FailedRecoveries++
		em.statsMutex.Unlock()

		// Retry if attempts remaining
		if errorEvent.Attempts < em.config.MaxRecoveryAttempts {
			em.logger.Infof("Retrying recovery for error ID=%d", errorEvent.ID)
			go em.attemptRecovery(errorEvent)
		} else {
			em.logger.Errorf("Recovery exhausted for error ID=%d after %d attempts", errorEvent.ID, errorEvent.Attempts)
		}
		return
	}

	// Recovery successful
	errorEvent.Recovered = true
	errorEvent.RecoveryTime = time.Now()

	em.logger.Infof("Recovery successful for error ID=%d (took %v)", errorEvent.ID, recoveryDuration)

	// Update statistics
	em.statsMutex.Lock()
	em.stats.SuccessfulRecoveries++
	em.stats.LastRecovery = errorEvent.RecoveryTime

	// Update average recovery time
	totalTime := em.stats.AverageRecoveryTime * time.Duration(em.stats.SuccessfulRecoveries-1)
	em.stats.AverageRecoveryTime = (totalTime + recoveryDuration) / time.Duration(em.stats.SuccessfulRecoveries)
	em.statsMutex.Unlock()

	// Update error in history
	em.updateErrorInHistory(errorEvent)
}

// updateErrorInHistory updates an error event in the history
func (em *ErrorManager) updateErrorInHistory(errorEvent *ErrorEvent) {
	em.historyMutex.Lock()
	defer em.historyMutex.Unlock()

	for i := range em.errorHistory {
		if em.errorHistory[i].ID == errorEvent.ID {
			em.errorHistory[i] = *errorEvent
			break
		}
	}
}

// callErrorCallback calls the error callback if set
func (em *ErrorManager) callErrorCallback(errorEvent *ErrorEvent) {
	em.callbackMutex.RLock()
	callback := em.errorCallback
	em.callbackMutex.RUnlock()

	if callback != nil {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					em.logger.Errorf("Error callback panic: %v", r)
				}
			}()
			callback(errorEvent)
		}()
	}
}

// classifyGStreamerError classifies a GStreamer error into our error types
func (em *ErrorManager) classifyGStreamerError(err error) GStreamerErrorType {
	if err == nil {
		return ErrorTypeUnknown
	}

	errStr := strings.ToLower(err.Error())

	switch {
	case strings.Contains(errStr, "state"):
		return ErrorTypeStateChange
	case strings.Contains(errStr, "element"):
		return ErrorTypeElementFailure
	case strings.Contains(errStr, "resource"):
		return ErrorTypeResourceUnavailable
	case strings.Contains(errStr, "format"):
		return ErrorTypeFormatNegotiation
	case strings.Contains(errStr, "timeout"):
		return ErrorTypeTimeout
	case strings.Contains(errStr, "permission"):
		return ErrorTypePermission
	case strings.Contains(errStr, "memory"):
		return ErrorTypeMemoryExhaustion
	case strings.Contains(errStr, "network"):
		return ErrorTypeNetwork
	case strings.Contains(errStr, "hardware"):
		return ErrorTypeHardware
	default:
		return ErrorTypeUnknown
	}
}

// classifyErrorSeverity classifies error severity based on type and message
func (em *ErrorManager) classifyErrorSeverity(errorType GStreamerErrorType, message string) ErrorSeverity {
	switch errorType {
	case ErrorTypeConfiguration, ErrorTypePermission:
		return ErrorSeverityCritical
	case ErrorTypeMemoryExhaustion, ErrorTypeSystem:
		return ErrorSeverityHigh
	case ErrorTypeStateChange, ErrorTypeElementFailure, ErrorTypeResourceUnavailable:
		return ErrorSeverityMedium
	case ErrorTypeNetwork, ErrorTypeTimeout:
		return ErrorSeverityLow
	default:
		return ErrorSeverityMedium
	}
}

// GetStatistics returns error management statistics
func (em *ErrorManager) GetStatistics() ErrorStatistics {
	em.statsMutex.RLock()
	defer em.statsMutex.RUnlock()
	return em.stats
}

// GetErrorHistory returns the error history
func (em *ErrorManager) GetErrorHistory() []ErrorEvent {
	em.historyMutex.RLock()
	defer em.historyMutex.RUnlock()

	result := make([]ErrorEvent, len(em.errorHistory))
	copy(result, em.errorHistory)
	return result
}

// GetRecentErrors returns recent error events
func (em *ErrorManager) GetRecentErrors(count int) []ErrorEvent {
	em.historyMutex.RLock()
	defer em.historyMutex.RUnlock()

	if count <= 0 || len(em.errorHistory) == 0 {
		return nil
	}

	start := len(em.errorHistory) - count
	if start < 0 {
		start = 0
	}

	result := make([]ErrorEvent, len(em.errorHistory)-start)
	copy(result, em.errorHistory[start:])
	return result
}

// GetStatus returns the current status of the error manager
func (em *ErrorManager) GetStatus() map[string]interface{} {
	return map[string]interface{}{
		"running":             em.running,
		"auto_recovery":       em.config.EnableAutoRecovery,
		"recovery_strategies": len(em.strategies),
		"statistics":          em.GetStatistics(),
	}
}

// AddRecoveryStrategy adds a custom recovery strategy
func (em *ErrorManager) AddRecoveryStrategy(strategy RecoveryStrategy) {
	em.strategies = append(em.strategies, strategy)
	em.logger.Debugf("Added recovery strategy: %s", strategy.GetDescription())
}

// ClearErrorHistory clears the error history
func (em *ErrorManager) ClearErrorHistory() {
	em.historyMutex.Lock()
	defer em.historyMutex.Unlock()

	em.errorHistory = em.errorHistory[:0]
	em.logger.Debug("Error history cleared")
}

// Background monitoring loops

// recoveryMonitorLoop monitors for recovery opportunities
func (em *ErrorManager) recoveryMonitorLoop() {
	defer em.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			em.checkForRecoveryOpportunities()
		case <-em.stopChan:
			return
		case <-em.ctx.Done():
			return
		}
	}
}

// checkForRecoveryOpportunities checks for errors that might benefit from retry
func (em *ErrorManager) checkForRecoveryOpportunities() {
	em.historyMutex.RLock()
	defer em.historyMutex.RUnlock()

	now := time.Now()
	retryThreshold := 5 * time.Minute

	for i := range em.errorHistory {
		errorEvent := &em.errorHistory[i]

		// Check if error is eligible for retry
		if !errorEvent.Recovered &&
			errorEvent.Attempts < em.config.MaxRecoveryAttempts &&
			now.Sub(errorEvent.Timestamp) > retryThreshold {

			em.logger.Infof("Found recovery opportunity for error ID=%d", errorEvent.ID)
			go em.attemptRecovery(errorEvent)
		}
	}
}
