package gstreamer

import (
	"fmt"
	"time"

	"github.com/go-gst/go-gst/gst"
	"github.com/sirupsen/logrus"
)

// GstObjectTracker provides a convenient interface for tracking GStreamer objects
type GstObjectTracker struct {
	lifecycleManager *ObjectLifecycleManager
	logger           *logrus.Entry
}

// NewGstObjectTracker creates a new GStreamer object tracker
func NewGstObjectTracker() *GstObjectTracker {
	config := ObjectLifecycleConfig{
		CleanupInterval:    30 * time.Second,
		ObjectRetention:    5 * time.Minute,
		EnableValidation:   true,
		ValidationInterval: 60 * time.Second,
		EnableStackTrace:   false, // Disable by default for performance
		LogObjectEvents:    false, // Disable by default to reduce noise
	}

	return &GstObjectTracker{
		lifecycleManager: NewObjectLifecycleManager(config),
		logger:           logrus.WithField("component", "gst-object-tracker"),
	}
}

// NewGstObjectTrackerWithConfig creates a new GStreamer object tracker with custom config
func NewGstObjectTrackerWithConfig(config ObjectLifecycleConfig) *GstObjectTracker {
	return &GstObjectTracker{
		lifecycleManager: NewObjectLifecycleManager(config),
		logger:           logrus.WithField("component", "gst-object-tracker"),
	}
}

// TrackPipeline registers a GStreamer pipeline for lifecycle tracking
func (got *GstObjectTracker) TrackPipeline(pipeline *gst.Pipeline, name string) (uintptr, error) {
	if pipeline == nil {
		return 0, fmt.Errorf("cannot track nil pipeline")
	}

	id, err := got.lifecycleManager.RegisterObject(pipeline, "Pipeline", name)
	if err != nil {
		got.logger.Errorf("Failed to track pipeline %s: %v", name, err)
		return 0, err
	}

	got.logger.Debugf("Tracking pipeline: %s (ID: %x)", name, id)
	return id, nil
}

// TrackElement registers a GStreamer element for lifecycle tracking
func (got *GstObjectTracker) TrackElement(element *gst.Element, name string) (uintptr, error) {
	if element == nil {
		return 0, fmt.Errorf("cannot track nil element")
	}

	id, err := got.lifecycleManager.RegisterObject(element, "Element", name)
	if err != nil {
		got.logger.Errorf("Failed to track element %s: %v", name, err)
		return 0, err
	}

	got.logger.Debugf("Tracking element: %s (ID: %x)", name, id)
	return id, nil
}

// TrackBus registers a GStreamer bus for lifecycle tracking
func (got *GstObjectTracker) TrackBus(bus *gst.Bus, name string) (uintptr, error) {
	if bus == nil {
		return 0, fmt.Errorf("cannot track nil bus")
	}

	id, err := got.lifecycleManager.RegisterObject(bus, "Bus", name)
	if err != nil {
		got.logger.Errorf("Failed to track bus %s: %v", name, err)
		return 0, err
	}

	got.logger.Debugf("Tracking bus: %s (ID: %x)", name, id)
	return id, nil
}

// TrackSample registers a GStreamer sample for lifecycle tracking
func (got *GstObjectTracker) TrackSample(sample *gst.Sample, name string) (uintptr, error) {
	if sample == nil {
		return 0, fmt.Errorf("cannot track nil sample")
	}

	id, err := got.lifecycleManager.RegisterObject(sample, "Sample", name)
	if err != nil {
		got.logger.Errorf("Failed to track sample %s: %v", name, err)
		return 0, err
	}

	got.logger.Debugf("Tracking sample: %s (ID: %x)", name, id)
	return id, nil
}

// UntrackObject unregisters an object from lifecycle tracking
func (got *GstObjectTracker) UntrackObject(id uintptr) error {
	if id == 0 {
		return fmt.Errorf("invalid object ID: 0")
	}

	err := got.lifecycleManager.UnregisterObject(id)
	if err != nil {
		got.logger.Errorf("Failed to untrack object ID %x: %v", id, err)
		return err
	}

	got.logger.Debugf("Untracked object: ID %x", id)
	return nil
}

// IsObjectValid checks if a tracked object is still valid
func (got *GstObjectTracker) IsObjectValid(id uintptr) bool {
	return got.lifecycleManager.IsValidObject(id)
}

// ValidateObject validates a GStreamer object
func (got *GstObjectTracker) ValidateObject(obj interface{}) bool {
	return got.lifecycleManager.ValidateGstObject(obj)
}

// SafeUsePipeline safely uses a pipeline with validation
func (got *GstObjectTracker) SafeUsePipeline(id uintptr, fn func(*gst.Pipeline) error) error {
	if !got.IsObjectValid(id) {
		return fmt.Errorf("pipeline ID %x is not valid", id)
	}

	info := got.lifecycleManager.GetObjectInfo(id)
	if info == nil {
		return fmt.Errorf("pipeline ID %x not found", id)
	}

	pipeline, ok := info.ObjectPtr.(*gst.Pipeline)
	if !ok {
		return fmt.Errorf("object ID %x is not a pipeline", id)
	}

	if !got.ValidateObject(pipeline) {
		got.lifecycleManager.ForceInvalidateObject(id)
		return fmt.Errorf("pipeline validation failed for ID %x", id)
	}

	return fn(pipeline)
}

// SafeUseElement safely uses an element with validation
func (got *GstObjectTracker) SafeUseElement(id uintptr, fn func(*gst.Element) error) error {
	if !got.IsObjectValid(id) {
		return fmt.Errorf("element ID %x is not valid", id)
	}

	info := got.lifecycleManager.GetObjectInfo(id)
	if info == nil {
		return fmt.Errorf("element ID %x not found", id)
	}

	element, ok := info.ObjectPtr.(*gst.Element)
	if !ok {
		return fmt.Errorf("object ID %x is not an element", id)
	}

	if !got.ValidateObject(element) {
		got.lifecycleManager.ForceInvalidateObject(id)
		return fmt.Errorf("element validation failed for ID %x", id)
	}

	return fn(element)
}

// SafeUseBus safely uses a bus with validation
func (got *GstObjectTracker) SafeUseBus(id uintptr, fn func(*gst.Bus) error) error {
	if !got.IsObjectValid(id) {
		return fmt.Errorf("bus ID %x is not valid", id)
	}

	info := got.lifecycleManager.GetObjectInfo(id)
	if info == nil {
		return fmt.Errorf("bus ID %x not found", id)
	}

	bus, ok := info.ObjectPtr.(*gst.Bus)
	if !ok {
		return fmt.Errorf("object ID %x is not a bus", id)
	}

	if !got.ValidateObject(bus) {
		got.lifecycleManager.ForceInvalidateObject(id)
		return fmt.Errorf("bus validation failed for ID %x", id)
	}

	return fn(bus)
}

// SafeUseSample safely uses a sample with validation
func (got *GstObjectTracker) SafeUseSample(id uintptr, fn func(*gst.Sample) error) error {
	if !got.IsObjectValid(id) {
		return fmt.Errorf("sample ID %x is not valid", id)
	}

	info := got.lifecycleManager.GetObjectInfo(id)
	if info == nil {
		return fmt.Errorf("sample ID %x not found", id)
	}

	sample, ok := info.ObjectPtr.(*gst.Sample)
	if !ok {
		return fmt.Errorf("object ID %x is not a sample", id)
	}

	if !got.ValidateObject(sample) {
		got.lifecycleManager.ForceInvalidateObject(id)
		return fmt.Errorf("sample validation failed for ID %x", id)
	}

	return fn(sample)
}

// GetStats returns lifecycle management statistics
func (got *GstObjectTracker) GetStats() ObjectLifecycleStats {
	return got.lifecycleManager.GetStats()
}

// ListActiveObjects returns a list of currently active objects
func (got *GstObjectTracker) ListActiveObjects() []*ObjectMetadata {
	return got.lifecycleManager.ListActiveObjects()
}

// ForceInvalidateObject forcefully marks an object as invalid
func (got *GstObjectTracker) ForceInvalidateObject(id uintptr) error {
	return got.lifecycleManager.ForceInvalidateObject(id)
}

// Close shuts down the object tracker
func (got *GstObjectTracker) Close() {
	got.logger.Info("Shutting down GStreamer object tracker")
	got.lifecycleManager.Close()
}

// GetLifecycleManager returns the underlying lifecycle manager
// This is provided for advanced use cases where direct access is needed
func (got *GstObjectTracker) GetLifecycleManager() *ObjectLifecycleManager {
	return got.lifecycleManager
}

// EnableDebugLogging enables debug logging for object events
func (got *GstObjectTracker) EnableDebugLogging(enable bool) {
	got.lifecycleManager.config.LogObjectEvents = enable
	if enable {
		got.logger.Info("Debug logging enabled for object lifecycle events")
	} else {
		got.logger.Info("Debug logging disabled for object lifecycle events")
	}
}

// EnableStackTrace enables stack trace capture for object registration
func (got *GstObjectTracker) EnableStackTrace(enable bool) {
	got.lifecycleManager.config.EnableStackTrace = enable
	if enable {
		got.logger.Info("Stack trace capture enabled for object registration")
	} else {
		got.logger.Info("Stack trace capture disabled for object registration")
	}
}

// GetHealthStatus returns health information about the object tracker
func (got *GstObjectTracker) GetHealthStatus() map[string]interface{} {
	stats := got.GetStats()

	// Calculate health based on various factors
	healthy := true

	// Check if we have too many invalid objects (more than 50% of total)
	if stats.TotalRegistered > 0 && stats.InvalidObjects > stats.TotalRegistered/2 {
		healthy = false
	}

	// Check if current active count is reasonable
	if stats.CurrentActive < 0 {
		healthy = false
	}

	return map[string]interface{}{
		"healthy":           healthy,
		"total_registered":  stats.TotalRegistered,
		"current_active":    stats.CurrentActive,
		"total_released":    stats.TotalReleased,
		"invalid_objects":   stats.InvalidObjects,
		"cleanup_runs":      stats.CleanupRuns,
		"objects_cleaned":   stats.ObjectsCleaned,
		"last_cleanup":      stats.LastCleanup,
		"validation_runs":   stats.ValidationRuns,
		"validation_errors": stats.ValidationErrors,
		"last_validation":   stats.LastValidation,
	}
}
