package gstreamer

import (
	"fmt"
	"reflect"
	"time"

	"github.com/go-gst/go-gst/gst"
	"github.com/sirupsen/logrus"
)

// Element interface provides a common interface for GStreamer elements
type Element interface {
	SetProperty(name string, value interface{}) error
	GetProperty(name string) (interface{}, error)
	Link(sink Element) error
	GetStaticPad(name string) (Pad, error)
}

// Pad interface provides a common interface for GStreamer pads
type Pad interface {
	GetName() string
	GetDirection() string
}

// CaptureStats holds statistics for desktop capture
type CaptureStats struct {
	FramesTotal    int64
	FramesCapture  int64
	FramesDropped  int64
	CurrentFPS     float64
	AverageFPS     float64
	CaptureLatency int64
	MinLatency     int64
	MaxLatency     int64
	AvgLatency     int64
	LastFrameTime  int64
	StartTime      int64
	BytesProcessed int64
	ErrorCount     int64
	UptimeSeconds  float64
}

// EncoderStats holds statistics for encoder
type EncoderStats struct {
	FramesEncoded    int64
	KeyframesEncoded int64
	FramesDropped    int64
	DropFrames       int64
	BytesEncoded     int64
	EncodingLatency  int64
	CurrentBitrate   int64
	TargetBitrate    int64
	AverageBitrate   int64
	EncodingFPS      float64
	ErrorCount       int64
	StartTime        int64
	LastUpdate       int64
}

// PipelineState represents the state of a GStreamer pipeline
type PipelineState int

const (
	PipelineStateNull PipelineState = iota
	PipelineStateReady
	PipelineStatePaused
	PipelineStatePlaying
	StateNull = PipelineStateNull // Alias for compatibility
)

// StateTransition represents a pipeline state transition
type StateTransition struct {
	From     PipelineState
	To       PipelineState
	Duration time.Duration
	Success  bool
	Error    error
}

// ErrorType represents different types of errors
type ErrorType int

const (
	ErrorTypeGeneral ErrorType = iota
	ErrorTypePipeline
	ErrorTypeElement
	ErrorTypeResource
	ErrorStateChange
	ErrorElementFailure
	ErrorResourceUnavailable
	ErrorFormatNegotiation
	ErrorTimeout
	ErrorPermission
	ErrorMemoryExhaustion
	ErrorNetwork
	ErrorHardware
	ErrorUnknown
)

// String returns the string representation of ErrorType
func (et ErrorType) String() string {
	switch et {
	case ErrorTypeGeneral:
		return "General"
	case ErrorTypePipeline:
		return "Pipeline"
	case ErrorTypeElement:
		return "Element"
	case ErrorTypeResource:
		return "Resource"
	case ErrorStateChange:
		return "StateChange"
	case ErrorElementFailure:
		return "ElementFailure"
	case ErrorResourceUnavailable:
		return "ResourceUnavailable"
	case ErrorFormatNegotiation:
		return "FormatNegotiation"
	case ErrorTimeout:
		return "Timeout"
	case ErrorPermission:
		return "Permission"
	case ErrorMemoryExhaustion:
		return "MemoryExhaustion"
	case ErrorNetwork:
		return "Network"
	case ErrorHardware:
		return "Hardware"
	case ErrorUnknown:
		return "Unknown"
	default:
		return "Unknown"
	}
}

// HealthStatusLevel represents the health status level
type HealthStatusLevel int

const (
	HealthStatusHealthy HealthStatusLevel = iota
	HealthStatusWarning
	HealthStatusError
	HealthStatusCritical
)

// String returns the string representation of HealthStatusLevel
func (hsl HealthStatusLevel) String() string {
	switch hsl {
	case HealthStatusHealthy:
		return "Healthy"
	case HealthStatusWarning:
		return "Warning"
	case HealthStatusError:
		return "Error"
	case HealthStatusCritical:
		return "Critical"
	default:
		return "Unknown"
	}
}

// HealthStatus represents the health status of a component
type HealthStatus struct {
	Overall   HealthStatusLevel
	LastCheck int64
	Uptime    int64
	Issues    []string
	Checks    map[string]interface{}
}

// DesktopCapture interface for desktop capture functionality
type DesktopCapture interface {
	Start() error
	Stop() error
	IsRunning() bool
	GetStats() CaptureStats
	SetSampleCallback(callback func(*Sample) error) error
	GetSampleChannel() <-chan *Sample
	GetErrorChannel() <-chan error
	GetSource() (string, map[string]interface{})
	GetAppsink() Element
	SetFrameRate(frameRate int) error
}

// PadAddedCallback is a callback function for pad-added signals
type PadAddedCallback func(element *gst.Element, pad *gst.Pad)

// ElementErrorEvent represents an error event
type ElementErrorEvent struct {
	Type      ErrorType
	Message   string
	Timestamp int64
	Component string
}

// MediaFormat represents media format information
type MediaFormat struct {
	MediaType string
	Width     int
	Height    int
	FrameRate float64
}

// C is a placeholder for CGO functionality (not used in go-gst implementation)
var C struct{}

// ElementFactory provides a high-level interface for creating and configuring GStreamer elements
type ElementFactory struct {
	logger *logrus.Entry
}

// NewElementFactory creates a new element factory
func NewElementFactory() *ElementFactory {
	return &ElementFactory{
		logger: logrus.WithField("component", "element-factory"),
	}
}

// ElementConfig holds configuration for an element
type ElementConfig struct {
	Name       string
	Factory    string
	Properties map[string]interface{}
}

// CreateElement creates a new GStreamer element with the specified factory name
func (ef *ElementFactory) CreateElement(factory string) (*gst.Element, error) {
	element, err := gst.NewElement(factory)
	if err != nil {
		return nil, fmt.Errorf("failed to create element with factory '%s': %w", factory, err)
	}

	ef.logger.Debugf("Created element with factory '%s'", factory)
	return element, nil
}

// CreateElementWithName creates a new GStreamer element with factory and name
func (ef *ElementFactory) CreateElementWithName(factory, name string) (*gst.Element, error) {
	element, err := ef.CreateElement(factory)
	if err != nil {
		return nil, err
	}

	err = element.SetProperty("name", name)
	if err != nil {
		element.Unref()
		return nil, fmt.Errorf("failed to set name '%s' on element: %w", name, err)
	}
	ef.logger.Debugf("Created element '%s' with factory '%s'", name, factory)
	return element, nil
}

// CreateConfiguredElement creates and configures an element based on ElementConfig
func (ef *ElementFactory) CreateConfiguredElement(config ElementConfig) (*gst.Element, error) {
	element, err := ef.CreateElementWithName(config.Factory, config.Name)
	if err != nil {
		return nil, err
	}

	if err := ef.ConfigureElement(element, config.Properties); err != nil {
		element.Unref()
		return nil, fmt.Errorf("failed to configure element '%s': %w", config.Name, err)
	}

	return element, nil
}

// ConfigureElement sets properties on an element
func (ef *ElementFactory) ConfigureElement(element *gst.Element, properties map[string]interface{}) error {
	if properties == nil {
		return nil
	}

	for key, value := range properties {
		if err := ef.SetElementProperty(element, key, value); err != nil {
			return fmt.Errorf("failed to set property '%s': %w", key, err)
		}
	}

	return nil
}

// SetElementProperty sets a single property on an element with type safety
func (ef *ElementFactory) SetElementProperty(element *gst.Element, name string, value interface{}) error {
	// Convert Go types to GStreamer-compatible types
	gstValue, err := ef.convertToGstValue(value)
	if err != nil {
		return fmt.Errorf("failed to convert value for property '%s': %w", name, err)
	}

	err = element.SetProperty(name, gstValue)
	if err != nil {
		return fmt.Errorf("failed to set property '%s' on element '%s': %w", name, element.GetName(), err)
	}
	ef.logger.Debugf("Set property '%s' = %v on element '%s'", name, value, element.GetName())
	return nil
}

// GetElementProperty gets a property value from an element
func (ef *ElementFactory) GetElementProperty(element *gst.Element, name string) (interface{}, error) {
	value, err := element.GetProperty(name)
	if err != nil {
		return nil, fmt.Errorf("failed to get property '%s' from element '%s': %w", name, element.GetName(), err)
	}

	ef.logger.Debugf("Got property '%s' = %v from element '%s'", name, value, element.GetName())
	return value, nil
}

// convertToGstValue converts Go types to GStreamer-compatible values
func (ef *ElementFactory) convertToGstValue(value interface{}) (interface{}, error) {
	if value == nil {
		return nil, nil
	}

	switch v := value.(type) {
	case string, int, int32, int64, uint, uint32, uint64, float32, float64, bool:
		return v, nil
	default:
		// Try to handle other types through reflection
		rv := reflect.ValueOf(value)
		switch rv.Kind() {
		case reflect.String:
			return rv.String(), nil
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return rv.Int(), nil
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return rv.Uint(), nil
		case reflect.Float32, reflect.Float64:
			return rv.Float(), nil
		case reflect.Bool:
			return rv.Bool(), nil
		default:
			return nil, fmt.Errorf("unsupported property type: %T", value)
		}
	}
}

// IsElementAvailable checks if an element factory is available
func (ef *ElementFactory) IsElementAvailable(factory string) bool {
	elementFactory := gst.Find(factory)
	available := elementFactory != nil

	ef.logger.Debugf("Element factory '%s' availability: %t", factory, available)
	return available
}

// GetElementFactoryInfo returns information about an element factory
func (ef *ElementFactory) GetElementFactoryInfo(factory string) (*ElementFactoryInfo, error) {
	elementFactory := gst.Find(factory)
	if elementFactory == nil {
		return nil, fmt.Errorf("element factory '%s' not found", factory)
	}

	info := &ElementFactoryInfo{
		Name:        elementFactory.GetName(),
		LongName:    elementFactory.GetMetadata("long-name"),
		Description: elementFactory.GetMetadata("description"),
		Author:      elementFactory.GetMetadata("author"),
		Rank:        0, // Rank is not easily accessible in go-gst
	}

	return info, nil
}

// ElementFactoryInfo contains information about an element factory
type ElementFactoryInfo struct {
	Name        string
	LongName    string
	Description string
	Author      string
	Rank        uint
}

// CreateVideoSource creates a video source element based on the source type
func (ef *ElementFactory) CreateVideoSource(sourceType VideoSourceType, config map[string]interface{}) (*gst.Element, error) {
	var factory string

	switch sourceType {
	case VideoSourceX11:
		factory = "ximagesrc"
	case VideoSourceWayland:
		factory = "waylandsink"
	case VideoSourceTest:
		factory = "videotestsrc"
	case VideoSourceV4L2:
		factory = "v4l2src"
	default:
		return nil, fmt.Errorf("unsupported video source type: %v", sourceType)
	}

	if !ef.IsElementAvailable(factory) {
		return nil, fmt.Errorf("video source factory '%s' is not available", factory)
	}

	element, err := ef.CreateElement(factory)
	if err != nil {
		return nil, err
	}

	if config != nil {
		if err := ef.ConfigureElement(element, config); err != nil {
			element.Unref()
			return nil, err
		}
	}

	return element, nil
}

// CreateVideoEncoder creates a video encoder element with fallback support
func (ef *ElementFactory) CreateVideoEncoder(codec VideoCodec, useHardware bool) (*gst.Element, error) {
	factories := ef.getEncoderFactories(codec, useHardware)

	for _, factory := range factories {
		if ef.IsElementAvailable(factory) {
			element, err := ef.CreateElement(factory)
			if err != nil {
				ef.logger.Warnf("Failed to create encoder '%s': %v", factory, err)
				continue
			}

			ef.logger.Infof("Created video encoder: %s", factory)
			return element, nil
		}
	}

	return nil, fmt.Errorf("no suitable video encoder found for codec %v (hardware: %t)", codec, useHardware)
}

// getEncoderFactories returns a prioritized list of encoder factories
func (ef *ElementFactory) getEncoderFactories(codec VideoCodec, useHardware bool) []string {
	var factories []string

	switch codec {
	case VideoCodecH264:
		if useHardware {
			factories = append(factories, "nvh264enc", "vaapih264enc", "qsvh264enc")
		}
		factories = append(factories, "x264enc", "openh264enc")
	case VideoCodecH265:
		if useHardware {
			factories = append(factories, "nvh265enc", "vaapih265enc", "qsvh265enc")
		}
		factories = append(factories, "x265enc")
	case VideoCodecVP8:
		if useHardware {
			factories = append(factories, "vaapivp8enc")
		}
		factories = append(factories, "vp8enc")
	case VideoCodecVP9:
		if useHardware {
			factories = append(factories, "vaapivp9enc")
		}
		factories = append(factories, "vp9enc")
	}

	return factories
}

// VideoSourceType represents different video source types
type VideoSourceType int

const (
	VideoSourceX11 VideoSourceType = iota
	VideoSourceWayland
	VideoSourceTest
	VideoSourceV4L2
)

// VideoCodec represents different video codecs
type VideoCodec int

const (
	VideoCodecH264 VideoCodec = iota
	VideoCodecH265
	VideoCodecVP8
	VideoCodecVP9
)

// CreateAppSink creates an appsink element with common configuration
func (ef *ElementFactory) CreateAppSink(config map[string]interface{}) (*gst.Element, error) {
	element, err := ef.CreateElement("appsink")
	if err != nil {
		return nil, err
	}

	// Set default appsink properties
	defaultConfig := map[string]interface{}{
		"emit-signals": true,
		"sync":         false,
		"max-buffers":  1,
		"drop":         true,
	}

	// Merge with user config
	if config != nil {
		for k, v := range config {
			defaultConfig[k] = v
		}
	}

	if err := ef.ConfigureElement(element, defaultConfig); err != nil {
		element.Unref()
		return nil, err
	}

	return element, nil
}

// CreateAppSrc creates an appsrc element with common configuration
func (ef *ElementFactory) CreateAppSrc(config map[string]interface{}) (*gst.Element, error) {
	element, err := ef.CreateElement("appsrc")
	if err != nil {
		return nil, err
	}

	// Set default appsrc properties
	defaultConfig := map[string]interface{}{
		"is-live":      true,
		"format":       gst.FormatTime,
		"do-timestamp": true,
	}

	// Merge with user config
	if config != nil {
		for k, v := range config {
			defaultConfig[k] = v
		}
	}

	if err := ef.ConfigureElement(element, defaultConfig); err != nil {
		element.Unref()
		return nil, err
	}

	return element, nil
}
