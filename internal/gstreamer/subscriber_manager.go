package gstreamer

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// SubscriberManager manages media stream subscribers with advanced features
// This provides dynamic subscription management and subscriber health monitoring
type SubscriberManager struct {
	// Context and lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	logger *logrus.Entry

	// Subscriber management
	mu          sync.RWMutex
	subscribers map[uint64]*ManagedSubscriber
	nextID      uint64

	// Health monitoring
	healthChecker *SubscriberHealthChecker

	// Configuration
	config SubscriberManagerConfig

	// Statistics
	stats *SubscriberManagerStats
}

// ManagedSubscriber wraps a subscriber with management metadata
type ManagedSubscriber struct {
	ID         uint64
	Subscriber MediaStreamSubscriber
	AddedAt    time.Time
	LastSeen   time.Time
	ErrorCount int64

	// Health status
	IsHealthy   bool
	LastError   error
	LastErrorAt time.Time

	// Performance metrics
	VideoFramesProcessed int64
	AudioFramesProcessed int64
	AvgProcessingTime    time.Duration

	// Configuration
	Config SubscriberConfig
}

// SubscriberConfig configuration for individual subscribers
type SubscriberConfig struct {
	// Error handling
	MaxErrors       int
	ErrorWindow     time.Duration
	RemoveOnFailure bool

	// Performance monitoring
	TimeoutThreshold    time.Duration
	HealthCheckInterval time.Duration

	// Stream filtering
	VideoEnabled bool
	AudioEnabled bool

	// Quality settings
	MaxVideoFrameRate int
	MaxAudioFrameRate int
}

// SubscriberManagerConfig configuration for the subscriber manager
type SubscriberManagerConfig struct {
	// Health monitoring
	EnableHealthCheck   bool
	HealthCheckInterval time.Duration
	UnhealthyThreshold  int

	// Cleanup settings
	EnableAutoCleanup bool
	CleanupInterval   time.Duration
	MaxInactiveTime   time.Duration

	// Performance settings
	MaxSubscribers int
	DefaultTimeout time.Duration

	// Logging
	LogSubscriberEvents bool
	LogHealthChecks     bool
}

// SubscriberManagerStats tracks subscriber management statistics
type SubscriberManagerStats struct {
	mu sync.RWMutex

	// Subscriber counts
	TotalSubscribers     int64
	ActiveSubscribers    int
	HealthySubscribers   int
	UnhealthySubscribers int
	RemovedSubscribers   int64

	// Performance metrics
	AvgSubscribers     float64
	PeakSubscribers    int
	SubscriberTurnover int64

	// Error tracking
	TotalErrors         int64
	HealthCheckFailures int64
	TimeoutErrors       int64

	// Timing
	StartTime       time.Time
	LastCleanup     time.Time
	LastHealthCheck time.Time
}

// SubscriberHealthChecker monitors subscriber health
type SubscriberHealthChecker struct {
	manager *SubscriberManager
	logger  *logrus.Entry
}

// DefaultSubscriberManagerConfig returns default configuration
func DefaultSubscriberManagerConfig() SubscriberManagerConfig {
	return SubscriberManagerConfig{
		EnableHealthCheck:   true,
		HealthCheckInterval: 30 * time.Second,
		UnhealthyThreshold:  5,
		EnableAutoCleanup:   true,
		CleanupInterval:     60 * time.Second,
		MaxInactiveTime:     300 * time.Second, // 5 minutes
		MaxSubscribers:      100,
		DefaultTimeout:      5 * time.Second,
		LogSubscriberEvents: true,
		LogHealthChecks:     false,
	}
}

// DefaultSubscriberConfig returns default subscriber configuration
func DefaultSubscriberConfig() SubscriberConfig {
	return SubscriberConfig{
		MaxErrors:           10,
		ErrorWindow:         60 * time.Second,
		RemoveOnFailure:     true,
		TimeoutThreshold:    1 * time.Second,
		HealthCheckInterval: 30 * time.Second,
		VideoEnabled:        true,
		AudioEnabled:        true,
		MaxVideoFrameRate:   60,
		MaxAudioFrameRate:   48000,
	}
}

// NewSubscriberManager creates a new subscriber manager
func NewSubscriberManager(ctx context.Context, config SubscriberManagerConfig, logger *logrus.Entry) *SubscriberManager {
	managerCtx, cancel := context.WithCancel(ctx)

	sm := &SubscriberManager{
		ctx:         managerCtx,
		cancel:      cancel,
		logger:      logger.WithField("component", "subscriber-manager"),
		subscribers: make(map[uint64]*ManagedSubscriber),
		config:      config,
		stats: &SubscriberManagerStats{
			StartTime: time.Now(),
		},
	}

	// Initialize health checker
	sm.healthChecker = &SubscriberHealthChecker{
		manager: sm,
		logger:  logger.WithField("component", "subscriber-health-checker"),
	}

	return sm
}

// Start starts the subscriber manager
func (sm *SubscriberManager) Start() error {
	sm.logger.Info("Starting subscriber manager")

	// Start health checking if enabled
	if sm.config.EnableHealthCheck {
		go sm.runHealthChecks()
	}

	// Start auto cleanup if enabled
	if sm.config.EnableAutoCleanup {
		go sm.runAutoCleanup()
	}

	sm.logger.Info("Subscriber manager started successfully")
	return nil
}

// Stop stops the subscriber manager
func (sm *SubscriberManager) Stop() error {
	sm.logger.Info("Stopping subscriber manager")

	// Cancel context to stop background goroutines
	sm.cancel()

	// Clear all subscribers
	sm.mu.Lock()
	subscriberCount := len(sm.subscribers)
	sm.subscribers = make(map[uint64]*ManagedSubscriber)
	sm.mu.Unlock()

	sm.logger.Infof("Subscriber manager stopped, removed %d subscribers", subscriberCount)
	return nil
}

// AddSubscriber adds a new subscriber with configuration
func (sm *SubscriberManager) AddSubscriber(subscriber MediaStreamSubscriber, config SubscriberConfig) (uint64, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Check subscriber limit
	if len(sm.subscribers) >= sm.config.MaxSubscribers {
		return 0, fmt.Errorf("maximum subscribers limit reached (%d)", sm.config.MaxSubscribers)
	}

	// Generate unique ID
	sm.nextID++
	id := sm.nextID

	// Create managed subscriber
	managedSub := &ManagedSubscriber{
		ID:         id,
		Subscriber: subscriber,
		AddedAt:    time.Now(),
		LastSeen:   time.Now(),
		IsHealthy:  true,
		Config:     config,
	}

	// Add to map
	sm.subscribers[id] = managedSub

	// Update statistics
	sm.stats.mu.Lock()
	sm.stats.TotalSubscribers++
	sm.stats.ActiveSubscribers = len(sm.subscribers)
	if sm.stats.ActiveSubscribers > sm.stats.PeakSubscribers {
		sm.stats.PeakSubscribers = sm.stats.ActiveSubscribers
	}
	sm.stats.mu.Unlock()

	if sm.config.LogSubscriberEvents {
		sm.logger.Infof("Added subscriber (ID: %d), total active: %d", id, len(sm.subscribers))
	}

	return id, nil
}

// RemoveSubscriber removes a subscriber by ID
func (sm *SubscriberManager) RemoveSubscriber(id uint64) bool {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	managedSub, exists := sm.subscribers[id]
	if !exists {
		return false
	}

	delete(sm.subscribers, id)

	// Update statistics
	sm.stats.mu.Lock()
	sm.stats.RemovedSubscribers++
	sm.stats.ActiveSubscribers = len(sm.subscribers)
	sm.stats.SubscriberTurnover++
	sm.stats.mu.Unlock()

	if sm.config.LogSubscriberEvents {
		uptime := time.Since(managedSub.AddedAt)
		sm.logger.Infof("Removed subscriber (ID: %d), uptime: %v, remaining: %d",
			id, uptime, len(sm.subscribers))
	}

	return true
}

// GetSubscriber returns a managed subscriber by ID
func (sm *SubscriberManager) GetSubscriber(id uint64) (*ManagedSubscriber, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	managedSub, exists := sm.subscribers[id]
	return managedSub, exists
}

// GetAllSubscribers returns all active subscribers
func (sm *SubscriberManager) GetAllSubscribers() []*ManagedSubscriber {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	subscribers := make([]*ManagedSubscriber, 0, len(sm.subscribers))
	for _, managedSub := range sm.subscribers {
		subscribers = append(subscribers, managedSub)
	}

	return subscribers
}

// GetHealthySubscribers returns only healthy subscribers
func (sm *SubscriberManager) GetHealthySubscribers() []*ManagedSubscriber {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	var healthySubscribers []*ManagedSubscriber
	for _, managedSub := range sm.subscribers {
		if managedSub.IsHealthy {
			healthySubscribers = append(healthySubscribers, managedSub)
		}
	}

	return healthySubscribers
}

// UpdateSubscriberHealth updates the health status of a subscriber
func (sm *SubscriberManager) UpdateSubscriberHealth(id uint64, isHealthy bool, err error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	managedSub, exists := sm.subscribers[id]
	if !exists {
		return
	}

	managedSub.IsHealthy = isHealthy
	managedSub.LastSeen = time.Now()

	if err != nil {
		managedSub.ErrorCount++
		managedSub.LastError = err
		managedSub.LastErrorAt = time.Now()

		// Update global error statistics
		sm.stats.mu.Lock()
		sm.stats.TotalErrors++
		sm.stats.mu.Unlock()

		// Check if subscriber should be removed due to too many errors
		if managedSub.Config.RemoveOnFailure &&
			managedSub.ErrorCount >= int64(managedSub.Config.MaxErrors) {
			sm.logger.Warnf("Removing subscriber (ID: %d) due to excessive errors (%d)",
				id, managedSub.ErrorCount)
			delete(sm.subscribers, id)

			sm.stats.mu.Lock()
			sm.stats.RemovedSubscribers++
			sm.stats.ActiveSubscribers = len(sm.subscribers)
			sm.stats.mu.Unlock()
		}
	}
}

// RecordSubscriberActivity records activity for a subscriber
func (sm *SubscriberManager) RecordSubscriberActivity(id uint64, isVideo bool, processingTime time.Duration) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	managedSub, exists := sm.subscribers[id]
	if !exists {
		return
	}

	managedSub.LastSeen = time.Now()

	if isVideo {
		managedSub.VideoFramesProcessed++
	} else {
		managedSub.AudioFramesProcessed++
	}

	// Update average processing time (simple moving average)
	if managedSub.AvgProcessingTime == 0 {
		managedSub.AvgProcessingTime = processingTime
	} else {
		managedSub.AvgProcessingTime = (managedSub.AvgProcessingTime + processingTime) / 2
	}
}

// runHealthChecks runs periodic health checks on subscribers
func (sm *SubscriberManager) runHealthChecks() {
	ticker := time.NewTicker(sm.config.HealthCheckInterval)
	defer ticker.Stop()

	sm.logger.Debug("Started subscriber health checking")

	for {
		select {
		case <-ticker.C:
			sm.performHealthChecks()

		case <-sm.ctx.Done():
			sm.logger.Debug("Subscriber health checking stopped")
			return
		}
	}
}

// performHealthChecks performs health checks on all subscribers
func (sm *SubscriberManager) performHealthChecks() {
	sm.mu.RLock()
	subscribers := make([]*ManagedSubscriber, 0, len(sm.subscribers))
	for _, managedSub := range sm.subscribers {
		subscribers = append(subscribers, managedSub)
	}
	sm.mu.RUnlock()

	healthyCount := 0
	unhealthyCount := 0

	for _, managedSub := range subscribers {
		isHealthy := sm.healthChecker.CheckSubscriberHealth(managedSub)

		if isHealthy {
			healthyCount++
		} else {
			unhealthyCount++
		}

		sm.UpdateSubscriberHealth(managedSub.ID, isHealthy, nil)
	}

	// Update statistics
	sm.stats.mu.Lock()
	sm.stats.HealthySubscribers = healthyCount
	sm.stats.UnhealthySubscribers = unhealthyCount
	sm.stats.LastHealthCheck = time.Now()
	sm.stats.mu.Unlock()

	if sm.config.LogHealthChecks {
		sm.logger.Debugf("Health check completed: %d healthy, %d unhealthy subscribers",
			healthyCount, unhealthyCount)
	}
}

// runAutoCleanup runs periodic cleanup of inactive subscribers
func (sm *SubscriberManager) runAutoCleanup() {
	ticker := time.NewTicker(sm.config.CleanupInterval)
	defer ticker.Stop()

	sm.logger.Debug("Started subscriber auto cleanup")

	for {
		select {
		case <-ticker.C:
			sm.performCleanup()

		case <-sm.ctx.Done():
			sm.logger.Debug("Subscriber auto cleanup stopped")
			return
		}
	}
}

// performCleanup removes inactive or unhealthy subscribers
func (sm *SubscriberManager) performCleanup() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	now := time.Now()
	var toRemove []uint64

	for id, managedSub := range sm.subscribers {
		// Check for inactive subscribers
		if now.Sub(managedSub.LastSeen) > sm.config.MaxInactiveTime {
			toRemove = append(toRemove, id)
			sm.logger.Infof("Marking inactive subscriber (ID: %d) for removal, last seen: %v ago",
				id, now.Sub(managedSub.LastSeen))
			continue
		}

		// Check for persistently unhealthy subscribers
		if !managedSub.IsHealthy &&
			managedSub.ErrorCount >= int64(sm.config.UnhealthyThreshold) {
			toRemove = append(toRemove, id)
			sm.logger.Infof("Marking unhealthy subscriber (ID: %d) for removal, errors: %d",
				id, managedSub.ErrorCount)
		}
	}

	// Remove marked subscribers
	removedCount := 0
	for _, id := range toRemove {
		delete(sm.subscribers, id)
		removedCount++
	}

	if removedCount > 0 {
		// Update statistics
		sm.stats.mu.Lock()
		sm.stats.RemovedSubscribers += int64(removedCount)
		sm.stats.ActiveSubscribers = len(sm.subscribers)
		sm.stats.LastCleanup = now
		sm.stats.mu.Unlock()

		sm.logger.Infof("Cleanup completed: removed %d subscribers, %d remaining",
			removedCount, len(sm.subscribers))
	}
}

// GetStats returns current subscriber manager statistics
func (sm *SubscriberManager) GetStats() SubscriberManagerStats {
	sm.stats.mu.RLock()
	defer sm.stats.mu.RUnlock()

	sm.mu.RLock()
	activeCount := len(sm.subscribers)
	sm.mu.RUnlock()

	stats := *sm.stats // Copy stats
	stats.ActiveSubscribers = activeCount

	// Calculate average subscribers
	uptime := time.Since(stats.StartTime).Seconds()
	if uptime > 0 {
		stats.AvgSubscribers = float64(stats.TotalSubscribers) / uptime
	}

	return stats
}

// CheckSubscriberHealth checks the health of a specific subscriber
func (shc *SubscriberHealthChecker) CheckSubscriberHealth(managedSub *ManagedSubscriber) bool {
	// Check if subscriber has been inactive for too long
	if time.Since(managedSub.LastSeen) > managedSub.Config.HealthCheckInterval*2 {
		shc.logger.Debugf("Subscriber (ID: %d) marked unhealthy: inactive for %v",
			managedSub.ID, time.Since(managedSub.LastSeen))
		return false
	}

	// Check error rate
	if managedSub.ErrorCount > 0 {
		errorWindow := managedSub.Config.ErrorWindow
		if errorWindow == 0 {
			errorWindow = 60 * time.Second
		}

		// If we have recent errors within the error window
		if !managedSub.LastErrorAt.IsZero() &&
			time.Since(managedSub.LastErrorAt) < errorWindow &&
			managedSub.ErrorCount >= int64(managedSub.Config.MaxErrors/2) {
			shc.logger.Debugf("Subscriber (ID: %d) marked unhealthy: high error rate (%d errors)",
				managedSub.ID, managedSub.ErrorCount)
			return false
		}
	}

	// Check processing time
	if managedSub.AvgProcessingTime > managedSub.Config.TimeoutThreshold {
		shc.logger.Debugf("Subscriber (ID: %d) marked unhealthy: slow processing (%v > %v)",
			managedSub.ID, managedSub.AvgProcessingTime, managedSub.Config.TimeoutThreshold)
		return false
	}

	return true
}

// GetSubscriberCount returns the current number of active subscribers
func (sm *SubscriberManager) GetSubscriberCount() int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return len(sm.subscribers)
}

// GetHealthySubscriberCount returns the number of healthy subscribers
func (sm *SubscriberManager) GetHealthySubscriberCount() int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	healthyCount := 0
	for _, managedSub := range sm.subscribers {
		if managedSub.IsHealthy {
			healthyCount++
		}
	}

	return healthyCount
}
