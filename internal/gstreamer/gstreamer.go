// Package gstreamer provides Go bindings for GStreamer using CGO.
package gstreamer

/*
#cgo pkg-config: gstreamer-1.0 gstreamer-app-1.0 gstreamer-video-1.0
#include <gst/gst.h>
#include <gst/app/gstappsrc.h>
#include <gst/app/gstappsink.h>
#include <gst/video/video.h>
#include <glib.h>
#include <glib-object.h>
#include <stdlib.h>
#include <string.h>
#include <stdio.h>

// Forward declarations for callback functions
extern gboolean goGstBusWatchCallback(GstBus *bus, GstMessage *message, gpointer user_data);
extern void goGstPadAddedCallback(GstElement *element, GstPad *pad, gpointer user_data);
extern void goGstElementAddedCallback(GstBin *bin, GstElement *element, gpointer user_data);

// Helper function to initialize GStreamer
static gboolean gst_init_go(void) {
    gboolean init;
    int argc = 0;
    char **argv = NULL;
    GError *err = NULL;

    init = gst_init_check(&argc, &argv, &err);
    if (!init) {
        if (err) {
            g_error_free(err);
        }
        return FALSE;
    }
    return TRUE;
}

// Helper function to set up a bus watch
static guint gst_bus_add_watch_go(GstBus *bus, gpointer user_data) {
    return gst_bus_add_watch(bus, (GstBusFunc)goGstBusWatchCallback, user_data);
}

// Helper function to connect pad-added signal
static void gst_element_connect_pad_added(GstElement *element, gpointer user_data) {
    g_signal_connect(element, "pad-added", G_CALLBACK(goGstPadAddedCallback), user_data);
}

// Helper function to connect element-added signal
static void gst_bin_connect_element_added(GstBin *bin, gpointer user_data) {
    g_signal_connect(bin, "element-added", G_CALLBACK(goGstElementAddedCallback), user_data);
}

// Helper function to create a pipeline from a string description
static GstElement* gst_parse_launch_go(const gchar *pipeline_description, GError **error) {
    return gst_parse_launch(pipeline_description, error);
}

// Helper function to get element factory name
static const gchar* gst_element_factory_get_name_go(GstElement *element) {
    GstElementFactory *factory = gst_element_get_factory(element);
    if (factory == NULL) {
        return NULL;
    }
    return gst_plugin_feature_get_name(GST_PLUGIN_FEATURE(factory));
}

// Helper function to get element name
static const gchar* gst_element_get_name_go(GstElement *element) {
    return gst_element_get_name(element);
}

// Helper function to get pad name
static const gchar* gst_pad_get_name_go(GstPad *pad) {
    return gst_pad_get_name(pad);
}

// Helper function to get message type as string
static const gchar* gst_message_type_get_name_go(GstMessageType type) {
    return gst_message_type_get_name(type);
}

// Helper function to get state change return as string
static const gchar* gst_state_change_return_get_name_go(GstStateChangeReturn ret) {
    switch (ret) {
        case GST_STATE_CHANGE_FAILURE:
            return "GST_STATE_CHANGE_FAILURE";
        case GST_STATE_CHANGE_SUCCESS:
            return "GST_STATE_CHANGE_SUCCESS";
        case GST_STATE_CHANGE_ASYNC:
            return "GST_STATE_CHANGE_ASYNC";
        case GST_STATE_CHANGE_NO_PREROLL:
            return "GST_STATE_CHANGE_NO_PREROLL";
        default:
            return "UNKNOWN";
    }
}

// Helper function to get state as string
static const gchar* gst_state_get_name_go(GstState state) {
    switch (state) {
        case GST_STATE_VOID_PENDING:
            return "GST_STATE_VOID_PENDING";
        case GST_STATE_NULL:
            return "GST_STATE_NULL";
        case GST_STATE_READY:
            return "GST_STATE_READY";
        case GST_STATE_PAUSED:
            return "GST_STATE_PAUSED";
        case GST_STATE_PLAYING:
            return "GST_STATE_PLAYING";
        default:
            return "UNKNOWN";
    }
}

// Helper function to force a key unit event
static gboolean gst_force_key_unit_go(GstElement *element) {
    GstPad *pad = gst_element_get_static_pad(element, "sink");
    if (!pad) {
        return FALSE;
    }

    GstEvent *event = gst_video_event_new_upstream_force_key_unit(
        GST_CLOCK_TIME_NONE, TRUE, 1);

    gboolean ret = gst_pad_send_event(pad, event);
    gst_object_unref(pad);

    return ret;
}

// Helper function to get caps as string
static gchar* gst_caps_to_string_go(const GstCaps *caps) {
    return gst_caps_to_string(caps);
}

// Helper function to create caps from string
static GstCaps* gst_caps_from_string_go(const gchar *string) {
    return gst_caps_from_string(string);
}

// Helper function to get plugin features
static GList* gst_registry_get_feature_list_by_plugin_go(const gchar *plugin_name) {
    GstRegistry *registry = gst_registry_get();
    return gst_registry_get_feature_list_by_plugin(registry, plugin_name);
}

// Helper function to check if a plugin is available
static gboolean gst_plugin_is_available_go(const gchar *plugin_name) {
    GstRegistry *registry = gst_registry_get();
    GstPlugin *plugin = gst_registry_find_plugin(registry, plugin_name);
    if (plugin) {
        gst_object_unref(plugin);
        return TRUE;
    }
    return FALSE;
}

// Helper function to check if an element factory is available
static gboolean gst_element_factory_is_available_go(const gchar *factory_name) {
    GstElementFactory *factory = gst_element_factory_find(factory_name);
    if (factory) {
        gst_object_unref(factory);
        return TRUE;
    }
    return FALSE;
}

// GStreamer logging configuration functions
static void gst_debug_set_default_threshold_go(int level) {
    gst_debug_set_default_threshold(level);
}

static void gst_debug_set_log_file_go(const gchar *filename) {
    if (filename && strlen(filename) > 0) {
        // Remove any existing log function first
        gst_debug_remove_log_function(gst_debug_log_default);

        // Open the log file
        FILE *log_file = fopen(filename, "a");
        if (log_file) {
            // Add log function with file output
            gst_debug_add_log_function(gst_debug_log_default, log_file, NULL);
        } else {
            // Fall back to default (stderr) if file cannot be opened
            gst_debug_add_log_function(gst_debug_log_default, NULL, NULL);
        }
    } else {
        // Use default console output (stderr)
        gst_debug_remove_log_function(gst_debug_log_default);
        gst_debug_add_log_function(gst_debug_log_default, NULL, NULL);
    }
}

static void gst_debug_set_colored_go(gboolean colored) {
    gst_debug_set_colored(colored);
}

// Helper functions for setting properties
static void set_string_property(GstElement *element, const gchar *name, const gchar *value) {
    g_object_set(element, name, value, NULL);
}

static void set_int_property(GstElement *element, const gchar *name, gint value) {
    g_object_set(element, name, value, NULL);
}

static void set_uint_property(GstElement *element, const gchar *name, guint value) {
    g_object_set(element, name, value, NULL);
}

static void set_boolean_property(GstElement *element, const gchar *name, gboolean value) {
    g_object_set(element, name, value, NULL);
}

static void set_double_property(GstElement *element, const gchar *name, gdouble value) {
    g_object_set(element, name, value, NULL);
}

static void set_caps_property(GstElement *element, const gchar *name, GstCaps *value) {
    g_object_set(element, name, value, NULL);
}

// Helper functions for GObject macros
static GObjectClass* g_object_get_class_go(gpointer object) {
    return G_OBJECT_GET_CLASS(object);
}

static gboolean g_value_holds_string_go(const GValue *value) {
    return G_VALUE_HOLDS_STRING(value);
}

static gboolean g_value_holds_int_go(const GValue *value) {
    return G_VALUE_HOLDS_INT(value);
}

static gboolean g_value_holds_uint_go(const GValue *value) {
    return G_VALUE_HOLDS_UINT(value);
}

static gboolean g_value_holds_boolean_go(const GValue *value) {
    return G_VALUE_HOLDS_BOOLEAN(value);
}

static gboolean g_value_holds_float_go(const GValue *value) {
    return G_VALUE_HOLDS_FLOAT(value);
}

static gboolean g_value_holds_double_go(const GValue *value) {
    return G_VALUE_HOLDS_DOUBLE(value);
}
*/
import "C"
import (
	"errors"
	"fmt"
	"runtime"
	"sync"
	"unsafe"
)

// Global variables for callback management
var (
	busCallbackRegistry          = make(map[uint]BusCallback)
	padAddedCallbackRegistry     = make(map[uintptr]PadAddedCallback)
	elementAddedCallbackRegistry = make(map[uintptr]ElementAddedCallback)
	callbackMutex                sync.RWMutex
	nextCallbackID               uint = 1
)

// Initialize GStreamer on package import
func init() {
	runtime.LockOSThread()
	if ok := C.gst_init_go(); ok == 0 {
		panic("Failed to initialize GStreamer")
	}
}

// Common errors
var (
	ErrGStreamerNotInitialized    = errors.New("gstreamer: not initialized")
	ErrPipelineCreationFailed     = errors.New("gstreamer: pipeline creation failed")
	ErrElementNotFound            = errors.New("gstreamer: element not found")
	ErrLinkFailed                 = errors.New("gstreamer: link failed")
	ErrInvalidState               = errors.New("gstreamer: invalid state")
	ErrBusCreationFailed          = errors.New("gstreamer: bus creation failed")
	ErrCapsCreationFailed         = errors.New("gstreamer: caps creation failed")
	ErrPropertyNotFound           = errors.New("gstreamer: property not found")
	ErrPropertyTypeMismatch       = errors.New("gstreamer: property type mismatch")
	ErrPadNotFound                = errors.New("gstreamer: pad not found")
	ErrPluginNotAvailable         = errors.New("gstreamer: plugin not available")
	ErrElementFactoryNotAvailable = errors.New("gstreamer: element factory not available")
)

// PipelineState represents the state of a GStreamer pipeline
type PipelineState int

const (
	StateVoidPending PipelineState = 0
	StateNull        PipelineState = 1
	StateReady       PipelineState = 2
	StatePaused      PipelineState = 3
	StatePlaying     PipelineState = 4
)

// StateChangeReturn represents the return value of a state change operation
type StateChangeReturn int

const (
	StateChangeFailure   StateChangeReturn = 0
	StateChangeSuccess   StateChangeReturn = 1
	StateChangeAsync     StateChangeReturn = 2
	StateChangeNoPreroll StateChangeReturn = 3
)

// Pipeline represents a GStreamer pipeline
type Pipeline interface {
	// Create creates the pipeline
	Create() error

	// CreateFromString creates a pipeline from a string description
	CreateFromString(pipelineStr string) error

	// Start starts the pipeline
	Start() error

	// Stop stops the pipeline
	Stop() error

	// GetElement gets an element from the pipeline by name
	GetElement(name string) (Element, error)

	// AddElement adds an element to the pipeline
	AddElement(element Element) error

	// LinkElements links two elements
	LinkElements(src, sink Element) error

	// LinkPad links two elements' pads
	LinkPad(srcElement, srcPad, sinkElement, sinkPad string) error

	// SetState sets the pipeline state
	SetState(state PipelineState) error

	// GetState gets the current pipeline state
	GetState() (PipelineState, error)

	// GetBus gets the pipeline's bus
	GetBus() (Bus, error)

	// SendEOS sends an end-of-stream event to the pipeline
	SendEOS() error
}

// Element represents a GStreamer element
type Element interface {
	// SetProperty sets an element property
	SetProperty(name string, value interface{}) error

	// GetProperty gets an element property
	GetProperty(name string) (interface{}, error)

	// Link links this element to another element
	Link(sink Element) error

	// GetStaticPad gets a static pad from the element
	GetStaticPad(name string) (Pad, error)

	// GetName gets the element's name
	GetName() string

	// GetFactoryName gets the element's factory name
	GetFactoryName() string

	// ForceKeyUnit forces a key unit event (for video encoders)
	ForceKeyUnit() bool

	// ConnectPadAdded connects a pad-added signal handler
	ConnectPadAdded(callback PadAddedCallback) error

	// GetInternal gets the internal C GstElement pointer
	GetInternal() *C.GstElement
}

// Pad represents a GStreamer pad
type Pad interface {
	// GetName gets the pad's name
	GetName() string

	// Link links this pad to another pad
	Link(sink Pad) error

	// Unlink unlinks this pad from its peer
	Unlink() error

	// SetCaps sets caps on the pad
	SetCaps(caps Caps) error

	// GetCaps gets the pad's caps
	GetCaps() (Caps, error)
}

// Caps represents GStreamer caps
type Caps interface {
	// ToString converts caps to string
	ToString() string

	// IsAny checks if caps are any
	IsAny() bool

	// IsEmpty checks if caps are empty
	IsEmpty() bool
}

// Bus represents a GStreamer bus
type Bus interface {
	// AddWatch adds a watch to the bus
	AddWatch(callback BusCallback) (uint, error)

	// RemoveWatch removes a watch from the bus
	RemoveWatch(id uint) error

	// Pop pops a message from the bus
	Pop() (Message, error)

	// TimedPop pops a message from the bus with timeout
	TimedPop(timeout uint64) (Message, error)
}

// BusCallback is a callback function for bus messages
type BusCallback func(message Message) bool

// PadAddedCallback is a callback function for pad-added signals
type PadAddedCallback func(element Element, pad Pad)

// ElementAddedCallback is a callback function for element-added signals
type ElementAddedCallback func(bin Element, element Element)

// Message represents a GStreamer message
type Message interface {
	// GetType gets the message type
	GetType() MessageType

	// GetTypeName gets the message type name
	GetTypeName() string

	// ParseError parses an error message
	ParseError() (error, string)

	// ParseStateChanged parses a state-changed message
	ParseStateChanged() (PipelineState, PipelineState, PipelineState)

	// ParseEOS parses an EOS message
	ParseEOS() bool
}

// MessageType represents the type of a GStreamer message
type MessageType int

const (
	MessageUnknown      MessageType = 0
	MessageEOS          MessageType = 1
	MessageError        MessageType = 2
	MessageWarning      MessageType = 3
	MessageInfo         MessageType = 4
	MessageStateChanged MessageType = 5
	MessageTag          MessageType = 6
	MessageBuffering    MessageType = 7
	MessageQOS          MessageType = 8
	MessageProgress     MessageType = 9
	MessageLatency      MessageType = 10
	MessageStreamStart  MessageType = 11
	MessageElement      MessageType = 12
	MessageApplication  MessageType = 13
	MessageClockLost    MessageType = 14
	MessageClockFound   MessageType = 15
	MessageStreamStatus MessageType = 16
	MessageNeedContext  MessageType = 17
	MessageHaveContext  MessageType = 18
	MessageDuration     MessageType = 19
	MessageAny          MessageType = 0xFFFFFFFF
)

// Implementation of the Pipeline interface
type pipeline struct {
	gstPipeline *C.GstElement
	elements    map[string]*C.GstElement
	mu          sync.Mutex
}

// NewPipeline creates a new GStreamer pipeline
func NewPipeline(name string) Pipeline {
	return &pipeline{
		elements: make(map[string]*C.GstElement),
	}
}

// Create creates the pipeline
func (p *pipeline) Create() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	cName := C.CString("pipeline")
	defer C.free(unsafe.Pointer(cName))

	p.gstPipeline = C.gst_pipeline_new(cName)
	if p.gstPipeline == nil {
		return ErrPipelineCreationFailed
	}

	return nil
}

// CreateFromString creates a pipeline from a string description
func (p *pipeline) CreateFromString(pipelineStr string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	cPipelineStr := C.CString(pipelineStr)
	defer C.free(unsafe.Pointer(cPipelineStr))

	var gerror *C.GError
	p.gstPipeline = C.gst_parse_launch_go(cPipelineStr, &gerror)

	if gerror != nil {
		errMsg := C.GoString(gerror.message)
		C.g_error_free(gerror)
		return fmt.Errorf("%w: %s", ErrPipelineCreationFailed, errMsg)
	}

	if p.gstPipeline == nil {
		return ErrPipelineCreationFailed
	}

	// Populate elements map with all elements in the pipeline
	// This would require traversing the bin, which is complex in CGO
	// For now, we'll leave the elements map empty and populate it as needed

	return nil
}

// Start starts the pipeline
func (p *pipeline) Start() error {
	return p.SetState(StatePlaying)
}

// Stop stops the pipeline
func (p *pipeline) Stop() error {
	err := p.SetState(StateNull)
	if err != nil {
		return err
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.gstPipeline != nil {
		C.gst_object_unref(C.gpointer(unsafe.Pointer(p.gstPipeline)))
		p.gstPipeline = nil
	}

	return nil
}

// GetElement gets an element from the pipeline by name or creates a new one
func (p *pipeline) GetElement(name string) (Element, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.gstPipeline == nil {
		return nil, ErrGStreamerNotInitialized
	}

	// First check if we already have this element
	if elem, ok := p.elements[name]; ok {
		return &element{elem: elem}, nil
	}

	// Try to find the element in the pipeline by name
	cName := C.CString(name)
	defer C.free(unsafe.Pointer(cName))

	// Check if this is an element in the pipeline
	elem := C.gst_bin_get_by_name((*C.GstBin)(unsafe.Pointer(p.gstPipeline)), cName)
	if elem != nil {
		p.elements[name] = elem
		return &element{elem: elem}, nil
	}

	// If not found, try to create a new element
	elem = C.gst_element_factory_make(cName, cName)
	if elem == nil {
		return nil, fmt.Errorf("%w: %s", ErrElementNotFound, name)
	}

	// Add element to pipeline
	C.gst_bin_add((*C.GstBin)(unsafe.Pointer(p.gstPipeline)), elem)

	// Store element for later use
	p.elements[name] = elem

	return &element{elem: elem}, nil
}

// AddElement adds an element to the pipeline
func (p *pipeline) AddElement(e Element) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.gstPipeline == nil {
		return ErrGStreamerNotInitialized
	}

	elem, ok := e.(*element)
	if !ok {
		return errors.New("invalid element type")
	}

	if elem.elem == nil {
		return ErrElementNotFound
	}

	// Add element to pipeline
	C.gst_bin_add((*C.GstBin)(unsafe.Pointer(p.gstPipeline)), elem.elem)

	// Store element for later use
	name := e.GetName()
	p.elements[name] = elem.elem

	return nil
}

// LinkElements links two elements
func (p *pipeline) LinkElements(src, sink Element) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.gstPipeline == nil {
		return ErrGStreamerNotInitialized
	}

	srcElem, ok := src.(*element)
	if !ok {
		return errors.New("invalid source element type")
	}

	sinkElem, ok := sink.(*element)
	if !ok {
		return errors.New("invalid sink element type")
	}

	if srcElem.elem == nil || sinkElem.elem == nil {
		return ErrElementNotFound
	}

	ret := C.gst_element_link(srcElem.elem, sinkElem.elem)
	if ret == 0 {
		return ErrLinkFailed
	}

	return nil
}

// LinkPad links two elements' pads
func (p *pipeline) LinkPad(srcElement, srcPad, sinkElement, sinkPad string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.gstPipeline == nil {
		return ErrGStreamerNotInitialized
	}

	srcElem, ok := p.elements[srcElement]
	if !ok {
		return fmt.Errorf("%w: %s", ErrElementNotFound, srcElement)
	}

	sinkElem, ok := p.elements[sinkElement]
	if !ok {
		return fmt.Errorf("%w: %s", ErrElementNotFound, sinkElement)
	}

	cSrcPad := C.CString(srcPad)
	defer C.free(unsafe.Pointer(cSrcPad))

	cSinkPad := C.CString(sinkPad)
	defer C.free(unsafe.Pointer(cSinkPad))

	ret := C.gst_element_link_pads(srcElem, cSrcPad, sinkElem, cSinkPad)
	if ret == 0 {
		return ErrLinkFailed
	}

	return nil
}

// SetState sets the pipeline state
func (p *pipeline) SetState(state PipelineState) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.gstPipeline == nil {
		return ErrGStreamerNotInitialized
	}

	ret := C.gst_element_set_state(p.gstPipeline, C.GstState(state))
	if ret == C.GST_STATE_CHANGE_FAILURE {
		return ErrInvalidState
	}

	return nil
}

// GetState gets the current pipeline state
func (p *pipeline) GetState() (PipelineState, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.gstPipeline == nil {
		return StateNull, ErrGStreamerNotInitialized
	}

	var state, pending C.GstState
	var timeout C.GstClockTime = 0 // Immediate query

	ret := C.gst_element_get_state(p.gstPipeline, &state, &pending, timeout)
	if ret == C.GST_STATE_CHANGE_FAILURE {
		return StateNull, ErrInvalidState
	}

	return PipelineState(state), nil
}

// GetBus gets the pipeline's bus
func (p *pipeline) GetBus() (Bus, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.gstPipeline == nil {
		return nil, ErrGStreamerNotInitialized
	}

	bus := C.gst_pipeline_get_bus((*C.GstPipeline)(unsafe.Pointer(p.gstPipeline)))
	if bus == nil {
		return nil, ErrBusCreationFailed
	}

	return &gstBus{bus: bus}, nil
}

// SendEOS sends an end-of-stream event to the pipeline
func (p *pipeline) SendEOS() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.gstPipeline == nil {
		return ErrGStreamerNotInitialized
	}

	ret := C.gst_element_send_event(p.gstPipeline, C.gst_event_new_eos())
	if ret == 0 {
		return errors.New("failed to send EOS event")
	}

	return nil
}

// Implementation of the Element interface
type element struct {
	elem *C.GstElement
}

// SetProperty sets an element property
func (e *element) SetProperty(name string, value interface{}) error {
	if e.elem == nil {
		return ErrElementNotFound
	}

	cName := C.CString(name)
	defer C.free(unsafe.Pointer(cName))

	switch v := value.(type) {
	case string:
		cValue := C.CString(v)
		defer C.free(unsafe.Pointer(cValue))
		C.set_string_property(e.elem, cName, cValue)
	case int:
		C.set_int_property(e.elem, cName, C.gint(v))
	case uint:
		C.set_uint_property(e.elem, cName, C.guint(v))
	case bool:
		var cValue C.gboolean
		if v {
			cValue = 1
		}
		C.set_boolean_property(e.elem, cName, cValue)
	case float64:
		C.set_double_property(e.elem, cName, C.gdouble(v))
	case Caps:
		caps, ok := v.(*gstCaps)
		if !ok {
			return errors.New("invalid caps type")
		}
		C.set_caps_property(e.elem, cName, caps.caps)
	default:
		return fmt.Errorf("unsupported property type: %T", value)
	}

	return nil
}

// GetProperty gets an element property
func (e *element) GetProperty(name string) (interface{}, error) {
	if e.elem == nil {
		return nil, ErrElementNotFound
	}

	cName := C.CString(name)
	defer C.free(unsafe.Pointer(cName))

	// Get property type
	var gValue C.GValue
	C.memset(unsafe.Pointer(&gValue), 0, C.sizeof_GValue)

	// Get property info
	var paramSpec *C.GParamSpec = C.g_object_class_find_property(
		(*C.GObjectClass)(unsafe.Pointer(C.g_object_get_class_go(C.gpointer(e.elem)))),
		cName)

	if paramSpec == nil {
		return nil, fmt.Errorf("%w: %s", ErrPropertyNotFound, name)
	}

	// Initialize GValue according to property type
	C.g_value_init(&gValue, paramSpec.value_type)

	// Get property value
	C.g_object_get_property((*C.GObject)(unsafe.Pointer(e.elem)), cName, &gValue)

	// Extract value based on type
	var result interface{}

	switch {
	case C.g_value_holds_string_go(&gValue) != 0:
		cStr := C.g_value_get_string(&gValue)
		result = C.GoString(cStr)
	case C.g_value_holds_int_go(&gValue) != 0:
		result = int(C.g_value_get_int(&gValue))
	case C.g_value_holds_uint_go(&gValue) != 0:
		result = uint(C.g_value_get_uint(&gValue))
	case C.g_value_holds_boolean_go(&gValue) != 0:
		result = C.g_value_get_boolean(&gValue) != 0
	case C.g_value_holds_float_go(&gValue) != 0:
		result = float32(C.g_value_get_float(&gValue))
	case C.g_value_holds_double_go(&gValue) != 0:
		result = float64(C.g_value_get_double(&gValue))
	default:
		C.g_value_unset(&gValue)
		return nil, fmt.Errorf("%w: %s", ErrPropertyTypeMismatch, name)
	}

	// Clean up
	C.g_value_unset(&gValue)

	return result, nil
}

// Link links this element to another element
func (e *element) Link(sink Element) error {
	if e.elem == nil {
		return ErrElementNotFound
	}

	sinkElem, ok := sink.(*element)
	if !ok {
		return errors.New("invalid sink element type")
	}

	if sinkElem.elem == nil {
		return ErrElementNotFound
	}

	ret := C.gst_element_link(e.elem, sinkElem.elem)
	if ret == 0 {
		return ErrLinkFailed
	}

	return nil
}

// GetStaticPad gets a static pad from the element
func (e *element) GetStaticPad(name string) (Pad, error) {
	if e.elem == nil {
		return nil, ErrElementNotFound
	}

	cName := C.CString(name)
	defer C.free(unsafe.Pointer(cName))

	pad := C.gst_element_get_static_pad(e.elem, cName)
	if pad == nil {
		return nil, fmt.Errorf("%w: %s", ErrPadNotFound, name)
	}

	return &gstPad{pad: pad}, nil
}

// GetName gets the element's name
func (e *element) GetName() string {
	if e.elem == nil {
		return ""
	}

	cName := C.gst_element_get_name_go(e.elem)
	if cName == nil {
		return ""
	}

	return C.GoString(cName)
}

// GetFactoryName gets the element's factory name
func (e *element) GetFactoryName() string {
	if e.elem == nil {
		return ""
	}

	cName := C.gst_element_factory_get_name_go(e.elem)
	if cName == nil {
		return ""
	}

	return C.GoString(cName)
}

// ForceKeyUnit forces a key unit event (for video encoders)
func (e *element) ForceKeyUnit() bool {
	if e.elem == nil {
		return false
	}

	return C.gst_force_key_unit_go(e.elem) != 0
}

// ConnectPadAdded connects a pad-added signal handler
func (e *element) ConnectPadAdded(callback PadAddedCallback) error {
	if e.elem == nil {
		return ErrElementNotFound
	}

	callbackMutex.Lock()
	defer callbackMutex.Unlock()

	// Store the callback in the registry
	elemPtr := uintptr(unsafe.Pointer(e.elem))
	padAddedCallbackRegistry[elemPtr] = callback

	// Connect the signal
	C.gst_element_connect_pad_added(e.elem, C.gpointer(unsafe.Pointer(uintptr(elemPtr))))

	return nil
}

// GetInternal gets the internal C GstElement pointer
func (e *element) GetInternal() *C.GstElement {
	return e.elem
}

// Implementation of the Pad interface
type gstPad struct {
	pad *C.GstPad
}

// GetName gets the pad's name
func (p *gstPad) GetName() string {
	if p.pad == nil {
		return ""
	}

	cName := C.gst_pad_get_name_go(p.pad)
	if cName == nil {
		return ""
	}

	return C.GoString(cName)
}

// Link links this pad to another pad
func (p *gstPad) Link(sink Pad) error {
	if p.pad == nil {
		return ErrPadNotFound
	}

	sinkPad, ok := sink.(*gstPad)
	if !ok {
		return errors.New("invalid sink pad type")
	}

	if sinkPad.pad == nil {
		return ErrPadNotFound
	}

	ret := C.gst_pad_link(p.pad, sinkPad.pad)
	if ret != 0 { // GST_PAD_LINK_OK is 0
		return ErrLinkFailed
	}

	return nil
}

// Unlink unlinks this pad from its peer
func (p *gstPad) Unlink() error {
	if p.pad == nil {
		return ErrPadNotFound
	}

	peer := C.gst_pad_get_peer(p.pad)
	if peer == nil {
		return nil // No peer to unlink
	}

	C.gst_pad_unlink(p.pad, peer)
	C.gst_object_unref(C.gpointer(unsafe.Pointer(peer)))

	return nil
}

// SetCaps sets caps on the pad
func (p *gstPad) SetCaps(caps Caps) error {
	if p.pad == nil {
		return ErrPadNotFound
	}

	gstCaps, ok := caps.(*gstCaps)
	if !ok {
		return errors.New("invalid caps type")
	}

	if gstCaps.caps == nil {
		return ErrCapsCreationFailed
	}

	ret := C.gst_pad_set_caps(p.pad, gstCaps.caps)
	if ret == 0 {
		return errors.New("failed to set caps")
	}

	return nil
}

// GetCaps gets the pad's caps
func (p *gstPad) GetCaps() (Caps, error) {
	if p.pad == nil {
		return nil, ErrPadNotFound
	}

	caps := C.gst_pad_get_current_caps(p.pad)
	if caps == nil {
		return nil, ErrCapsCreationFailed
	}

	return &gstCaps{caps: caps}, nil
}

// Implementation of the Caps interface
type gstCaps struct {
	caps *C.GstCaps
}

// NewCapsFromString creates new caps from a string
func NewCapsFromString(capsStr string) (Caps, error) {
	cCapsStr := C.CString(capsStr)
	defer C.free(unsafe.Pointer(cCapsStr))

	caps := C.gst_caps_from_string_go(cCapsStr)
	if caps == nil {
		return nil, ErrCapsCreationFailed
	}

	return &gstCaps{caps: caps}, nil
}

// ToString converts caps to string
func (c *gstCaps) ToString() string {
	if c.caps == nil {
		return ""
	}

	cStr := C.gst_caps_to_string_go(c.caps)
	if cStr == nil {
		return ""
	}

	str := C.GoString(cStr)
	C.g_free(C.gpointer(unsafe.Pointer(cStr)))

	return str
}

// IsAny checks if caps are any
func (c *gstCaps) IsAny() bool {
	if c.caps == nil {
		return false
	}

	return C.gst_caps_is_any(c.caps) != 0
}

// IsEmpty checks if caps are empty
func (c *gstCaps) IsEmpty() bool {
	if c.caps == nil {
		return true
	}

	return C.gst_caps_is_empty(c.caps) != 0
}

// Implementation of the Bus interface
type gstBus struct {
	bus *C.GstBus
}

// AddWatch adds a watch to the bus
func (b *gstBus) AddWatch(callback BusCallback) (uint, error) {
	if b.bus == nil {
		return 0, ErrBusCreationFailed
	}

	callbackMutex.Lock()
	defer callbackMutex.Unlock()

	// Generate a unique ID for this callback
	id := nextCallbackID
	nextCallbackID++

	// Store the callback in the registry
	busCallbackRegistry[id] = callback

	// Add the watch
	C.gst_bus_add_watch_go(b.bus, C.gpointer(unsafe.Pointer(uintptr(id))))

	return id, nil
}

// RemoveWatch removes a watch from the bus
func (b *gstBus) RemoveWatch(id uint) error {
	if b.bus == nil {
		return ErrBusCreationFailed
	}

	callbackMutex.Lock()
	defer callbackMutex.Unlock()

	// Remove the callback from the registry
	delete(busCallbackRegistry, id)

	// In a real implementation, we would need to remove the watch from the bus
	// This is complex in CGO and would require additional helper functions

	return nil
}

// Pop pops a message from the bus
func (b *gstBus) Pop() (Message, error) {
	if b.bus == nil {
		return nil, ErrBusCreationFailed
	}

	msg := C.gst_bus_pop(b.bus)
	if msg == nil {
		return nil, nil // No message available
	}

	return &gstMessage{msg: msg}, nil
}

// TimedPop pops a message from the bus with timeout
func (b *gstBus) TimedPop(timeout uint64) (Message, error) {
	if b.bus == nil {
		return nil, ErrBusCreationFailed
	}

	msg := C.gst_bus_timed_pop(b.bus, C.GstClockTime(timeout))
	if msg == nil {
		return nil, nil // No message available
	}

	return &gstMessage{msg: msg}, nil
}

// Implementation of the Message interface
type gstMessage struct {
	msg *C.GstMessage
}

// GetType gets the message type
func (m *gstMessage) GetType() MessageType {
	if m.msg == nil {
		return MessageUnknown
	}

	return MessageType(m.msg._type)
}

// GetTypeName gets the message type name
func (m *gstMessage) GetTypeName() string {
	if m.msg == nil {
		return "unknown"
	}

	cName := C.gst_message_type_get_name_go(m.msg._type)
	if cName == nil {
		return "unknown"
	}

	return C.GoString(cName)
}

// ParseError parses an error message
func (m *gstMessage) ParseError() (error, string) {
	if m.msg == nil {
		return errors.New("nil message"), ""
	}

	var gerror *C.GError
	var debug *C.gchar

	C.gst_message_parse_error(m.msg, &gerror, &debug)

	if gerror == nil {
		return errors.New("unknown error"), ""
	}

	goError := errors.New(C.GoString(gerror.message))
	goDebug := C.GoString(debug)

	C.g_error_free(gerror)
	C.g_free(C.gpointer(unsafe.Pointer(debug)))

	return goError, goDebug
}

// ParseStateChanged parses a state-changed message
func (m *gstMessage) ParseStateChanged() (PipelineState, PipelineState, PipelineState) {
	if m.msg == nil {
		return StateNull, StateNull, StateNull
	}

	var oldState, newState, pendingState C.GstState
	C.gst_message_parse_state_changed(m.msg, &oldState, &newState, &pendingState)

	return PipelineState(oldState), PipelineState(newState), PipelineState(pendingState)
}

// ParseEOS parses an EOS message
func (m *gstMessage) ParseEOS() bool {
	if m.msg == nil {
		return false
	}

	return m.GetType() == MessageEOS
}

// Version returns the GStreamer version
func Version() string {
	var major, minor, micro, nano C.guint
	C.gst_version(&major, &minor, &micro, &nano)
	return fmt.Sprintf("%d.%d.%d.%d", major, minor, micro, nano)
}

// IsPluginAvailable checks if a GStreamer plugin is available
func IsPluginAvailable(pluginName string) bool {
	cPluginName := C.CString(pluginName)
	defer C.free(unsafe.Pointer(cPluginName))

	return C.gst_plugin_is_available_go(cPluginName) != 0
}

// IsElementFactoryAvailable checks if a GStreamer element factory is available
func IsElementFactoryAvailable(factoryName string) bool {
	cFactoryName := C.CString(factoryName)
	defer C.free(unsafe.Pointer(cFactoryName))

	return C.gst_element_factory_is_available_go(cFactoryName) != 0
}

// GStreamer logging configuration functions

// SetDebugLevel sets the global GStreamer debug level
// Level should be between 0 (NONE) and 9 (LOG)
func SetDebugLevel(level int) {
	C.gst_debug_set_default_threshold_go(C.int(level))
}

// SetLogFile sets the GStreamer log output file
// If filename is empty, logging will go to console (stderr)
func SetLogFile(filename string) {
	if filename == "" {
		C.gst_debug_set_log_file_go(nil)
	} else {
		cFilename := C.CString(filename)
		defer C.free(unsafe.Pointer(cFilename))
		C.gst_debug_set_log_file_go(cFilename)
	}
}

// SetColoredOutput enables or disables colored log output
func SetColoredOutput(colored bool) {
	var cColored C.gboolean
	if colored {
		cColored = 1
	} else {
		cColored = 0
	}
	C.gst_debug_set_colored_go(cColored)
}

//export goGstBusWatchCallback
func goGstBusWatchCallback(bus *C.GstBus, msg *C.GstMessage, userData C.gpointer) C.gboolean {
	callbackMutex.RLock()
	defer callbackMutex.RUnlock()

	// Get the callback ID from user data
	id := uint(uintptr(userData))

	// Look up the callback
	callback, ok := busCallbackRegistry[id]
	if !ok {
		return 0 // Remove the watch
	}

	// Create a Go message wrapper
	message := &gstMessage{msg: msg}

	// Call the callback
	if callback(message) {
		return 1 // Keep the watch
	}

	return 0 // Remove the watch
}

//export goGstPadAddedCallback
func goGstPadAddedCallback(gstElement *C.GstElement, pad *C.GstPad, userData C.gpointer) {
	callbackMutex.RLock()
	defer callbackMutex.RUnlock()

	// Get the element pointer from user data
	elemPtr := uintptr(userData)

	// Look up the callback
	callback, ok := padAddedCallbackRegistry[elemPtr]
	if !ok {
		return
	}

	// Create Go wrappers
	elemWrapper := &element{elem: gstElement}
	padWrapper := &gstPad{pad: pad}

	// Call the callback
	callback(elemWrapper, padWrapper)
}

//export goGstElementAddedCallback
func goGstElementAddedCallback(bin *C.GstBin, gstElement *C.GstElement, userData C.gpointer) {
	callbackMutex.RLock()
	defer callbackMutex.RUnlock()

	// Get the bin pointer from user data
	binPtr := uintptr(userData)

	// Look up the callback
	callback, ok := elementAddedCallbackRegistry[binPtr]
	if !ok {
		return
	}

	// Create Go wrappers
	binWrapper := &element{elem: (*C.GstElement)(unsafe.Pointer(bin))}
	elemWrapper := &element{elem: gstElement}

	// Call the callback
	callback(binWrapper, elemWrapper)
}
