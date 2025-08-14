package gstreamer

/*
#cgo pkg-config: gstreamer-1.0
#include <gst/gst.h>
#include <glib.h>
#include <stdlib.h>

// Helper function to create a bus message handler
static void setup_bus_callback(GstBus *bus, guint *watch_id, gpointer user_data) {
    *watch_id = gst_bus_add_watch(bus, NULL, user_data);
}

// Helper function to remove a bus watch
static void remove_bus_watch(guint watch_id) {
    g_source_remove(watch_id);
}

// Helper function to get message type
static GstMessageType get_message_type(GstMessage *msg) {
    return GST_MESSAGE_TYPE(msg);
}

// Helper function to get message source name
static const gchar* get_message_src_name(GstMessage *msg) {
    return GST_OBJECT_NAME(msg->src);
}

// Helper function to parse error message
static void parse_error_message(GstMessage *msg, GError **err, gchar **debug) {
    gst_message_parse_error(msg, err, debug);
}

// Helper function to parse warning message
static void parse_warning_message(GstMessage *msg, GError **err, gchar **debug) {
    gst_message_parse_warning(msg, err, debug);
}

// Helper function to parse info message
static void parse_info_message(GstMessage *msg, GError **err, gchar **debug) {
    gst_message_parse_info(msg, err, debug);
}

// Helper function to parse state changed message
static void parse_state_changed_message(GstMessage *msg, GstState *old_state, GstState *new_state, GstState *pending_state) {
    gst_message_parse_state_changed(msg, old_state, new_state, pending_state);
}

// Helper function to parse buffering message
static void parse_buffering_message(GstMessage *msg, gint *percent) {
    gst_message_parse_buffering(msg, percent);
}

// Helper function to parse QoS message
static void parse_qos_message(GstMessage *msg, gboolean *live, guint64 *running_time, guint64 *stream_time, guint64 *timestamp, guint64 *duration) {
    gst_message_parse_qos(msg, live, running_time, stream_time, timestamp, duration);
}

// Helper function to parse progress message
static void parse_progress_message(GstMessage *msg, GstProgressType *type, gchar **code, gchar **text) {
    gst_message_parse_progress(msg, type, code, text);
}

// Helper function to create a custom application message
static GstMessage* create_application_message(GstElement *src, const gchar *structure_name) {
    GstStructure *structure = gst_structure_new_empty(structure_name);
    return gst_message_new_application(GST_OBJECT(src), structure);
}

// Helper function to add a field to a message structure
static void add_field_to_message(GstMessage *msg, const gchar *field_name, const gchar *value) {
    GstStructure *structure = (GstStructure *)gst_message_get_structure(msg);
    gst_structure_set(structure, field_name, G_TYPE_STRING, value, NULL);
}

// Helper function to get a field from a message structure
static const gchar* get_field_from_message(GstMessage *msg, const gchar *field_name) {
    GstStructure *structure = (GstStructure *)gst_message_get_structure(msg);
    return gst_structure_get_string(structure, field_name);
}

// Helper function to create a custom event
static GstEvent* create_custom_event(const gchar *event_name) {
    GstStructure *structure = gst_structure_new_empty(event_name);
    return gst_event_new_custom(GST_EVENT_CUSTOM_DOWNSTREAM, structure);
}

// Helper function to add a field to an event structure
static void add_field_to_event(GstEvent *event, const gchar *field_name, const gchar *value) {
    GstStructure *structure = (GstStructure *)gst_event_get_structure(event);
    gst_structure_set(structure, field_name, G_TYPE_STRING, value, NULL);
}

// Helper function to send an event to an element
static gboolean send_event_to_element(GstElement *element, GstEvent *event) {
    return gst_element_send_event(element, event);
}
*/
import "C"
import (
	"errors"
	"fmt"
	"sync"
	"unsafe"
)

// MessageType constants
const (
	MessageTypeUnknown      = MessageType(C.GST_MESSAGE_UNKNOWN)
	MessageTypeEOS          = MessageType(C.GST_MESSAGE_EOS)
	MessageTypeError        = MessageType(C.GST_MESSAGE_ERROR)
	MessageTypeWarning      = MessageType(C.GST_MESSAGE_WARNING)
	MessageTypeInfo         = MessageType(C.GST_MESSAGE_INFO)
	MessageTypeTag          = MessageType(C.GST_MESSAGE_TAG)
	MessageTypeBuffering    = MessageType(C.GST_MESSAGE_BUFFERING)
	MessageTypeStateChanged = MessageType(C.GST_MESSAGE_STATE_CHANGED)
	MessageTypeQOS          = MessageType(C.GST_MESSAGE_QOS)
	MessageTypeProgress     = MessageType(C.GST_MESSAGE_PROGRESS)
	MessageTypeApplication  = MessageType(C.GST_MESSAGE_APPLICATION)
	MessageTypeAny          = MessageType(C.GST_MESSAGE_ANY)
)

// EventType constants
const (
	EventTypeUnknown          = "unknown"
	EventTypeEOS              = "eos"
	EventTypeFlushStart       = "flush-start"
	EventTypeFlushStop        = "flush-stop"
	EventTypeStreamStart      = "stream-start"
	EventTypeCustom           = "custom"
	EventTypeSegment          = "segment"
	EventTypeGap              = "gap"
	EventTypeBufferSize       = "buffer-size"
	EventTypeQOS              = "qos"
	EventTypeSeek             = "seek"
	EventTypeNavigation       = "navigation"
	EventTypeLatency          = "latency"
	EventTypeStep             = "step"
	EventTypeReconfigure      = "reconfigure"
	EventTypeTOC              = "toc"
	EventTypeProtection       = "protection"
	EventTypeSegmentDone      = "segment-done"
	EventTypeStreamCollection = "stream-collection"
	EventTypeStreamGroup      = "stream-group"
)

// BusWatcher watches a GStreamer bus for messages
type BusWatcher struct {
	bus      *C.GstBus
	watchID  C.guint
	handlers map[MessageType][]MessageHandler
	mu       sync.RWMutex
}

// MessageHandler is a callback function for handling GStreamer messages
type MessageHandler func(msg Message) bool

// NewBusWatcher creates a new bus watcher
func NewBusWatcher(p Pipeline) (*BusWatcher, error) {
	bus, err := p.GetBus()
	if err != nil {
		return nil, err
	}

	gstBus, ok := bus.(*gstBus)
	if !ok {
		return nil, errors.New("invalid bus type")
	}

	watcher := &BusWatcher{
		bus:      gstBus.bus,
		handlers: make(map[MessageType][]MessageHandler),
	}

	// Set up the bus watch
	var watchID C.guint
	C.setup_bus_callback(watcher.bus, &watchID, C.gpointer(unsafe.Pointer(watcher)))
	watcher.watchID = watchID

	return watcher, nil
}

// AddHandler adds a message handler for a specific message type
func (w *BusWatcher) AddHandler(msgType MessageType, handler MessageHandler) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.handlers[msgType] = append(w.handlers[msgType], handler)
}

// RemoveHandler removes all handlers for a specific message type
func (w *BusWatcher) RemoveHandler(msgType MessageType) {
	w.mu.Lock()
	defer w.mu.Unlock()

	delete(w.handlers, msgType)
}

// HandleMessage handles a GStreamer message
func (w *BusWatcher) HandleMessage(msg Message) bool {
	w.mu.RLock()
	defer w.mu.RUnlock()

	msgType := msg.GetType()

	// Call handlers for this message type
	handlers, ok := w.handlers[msgType]
	if ok {
		for _, handler := range handlers {
			if !handler(msg) {
				return false
			}
		}
	}

	// Call handlers for any message type
	handlers, ok = w.handlers[MessageTypeAny]
	if ok {
		for _, handler := range handlers {
			if !handler(msg) {
				return false
			}
		}
	}

	return true
}

// Stop stops the bus watcher
func (w *BusWatcher) Stop() {
	if w.watchID != 0 {
		C.remove_bus_watch(w.watchID)
		w.watchID = 0
	}
}

// EnhancedMessage provides additional functionality for GStreamer messages
type EnhancedMessage struct {
	*gstMessage
}

// NewEnhancedMessage creates a new enhanced message
func NewEnhancedMessage(msg Message) *EnhancedMessage {
	gstMsg, ok := msg.(*gstMessage)
	if !ok {
		return nil
	}

	return &EnhancedMessage{gstMessage: gstMsg}
}

// GetSourceName gets the name of the message source
func (m *EnhancedMessage) GetSourceName() string {
	if m.msg == nil {
		return ""
	}

	cName := C.get_message_src_name(m.msg)
	if cName == nil {
		return ""
	}

	return C.GoString(cName)
}

// ParseBuffering parses a buffering message
func (m *EnhancedMessage) ParseBuffering() (int, error) {
	if m.msg == nil {
		return 0, errors.New("nil message")
	}

	if m.GetType() != MessageTypeBuffering {
		return 0, fmt.Errorf("not a buffering message: %s", m.GetTypeName())
	}

	var percent C.gint
	C.parse_buffering_message(m.msg, &percent)

	return int(percent), nil
}

// ParseQOS parses a QOS message
func (m *EnhancedMessage) ParseQOS() (bool, uint64, uint64, uint64, uint64, error) {
	if m.msg == nil {
		return false, 0, 0, 0, 0, errors.New("nil message")
	}

	if m.GetType() != MessageTypeQOS {
		return false, 0, 0, 0, 0, fmt.Errorf("not a QOS message: %s", m.GetTypeName())
	}

	var live C.gboolean
	var runningTime, streamTime, timestamp, duration C.guint64

	C.parse_qos_message(m.msg, &live, &runningTime, &streamTime, &timestamp, &duration)

	return live != 0, uint64(runningTime), uint64(streamTime), uint64(timestamp), uint64(duration), nil
}

// ParseProgress parses a progress message
func (m *EnhancedMessage) ParseProgress() (int, string, string, error) {
	if m.msg == nil {
		return 0, "", "", errors.New("nil message")
	}

	if m.GetType() != MessageTypeProgress {
		return 0, "", "", fmt.Errorf("not a progress message: %s", m.GetTypeName())
	}

	var progressType C.GstProgressType
	var code, text *C.gchar

	C.parse_progress_message(m.msg, &progressType, &code, &text)

	goCode := C.GoString(code)
	goText := C.GoString(text)

	C.g_free(C.gpointer(unsafe.Pointer(code)))
	C.g_free(C.gpointer(unsafe.Pointer(text)))

	return int(progressType), goCode, goText, nil
}

// ParseWarning parses a warning message
func (m *EnhancedMessage) ParseWarning() (error, string) {
	if m.msg == nil {
		return errors.New("nil message"), ""
	}

	if m.GetType() != MessageTypeWarning {
		return fmt.Errorf("not a warning message: %s", m.GetTypeName()), ""
	}

	var gerror *C.GError
	var debug *C.gchar

	C.parse_warning_message(m.msg, &gerror, &debug)

	if gerror == nil {
		return errors.New("unknown warning"), ""
	}

	goError := errors.New(C.GoString(gerror.message))
	goDebug := C.GoString(debug)

	C.g_error_free(gerror)
	C.g_free(C.gpointer(unsafe.Pointer(debug)))

	return goError, goDebug
}

// ParseInfo parses an info message
func (m *EnhancedMessage) ParseInfo() (error, string) {
	if m.msg == nil {
		return errors.New("nil message"), ""
	}

	if m.GetType() != MessageTypeInfo {
		return fmt.Errorf("not an info message: %s", m.GetTypeName()), ""
	}

	var gerror *C.GError
	var debug *C.gchar

	C.parse_info_message(m.msg, &gerror, &debug)

	if gerror == nil {
		return errors.New("unknown info"), ""
	}

	goError := errors.New(C.GoString(gerror.message))
	goDebug := C.GoString(debug)

	C.g_error_free(gerror)
	C.g_free(C.gpointer(unsafe.Pointer(debug)))

	return goError, goDebug
}

// CreateApplicationMessage creates a custom application message
func CreateApplicationMessage(e Element, name string) (Message, error) {
	elem, ok := e.(*element)
	if !ok {
		return nil, errors.New("invalid element type")
	}

	if elem.elem == nil {
		return nil, ErrElementNotFound
	}

	cName := C.CString(name)
	defer C.free(unsafe.Pointer(cName))

	msg := C.create_application_message(elem.elem, cName)
	if msg == nil {
		return nil, errors.New("failed to create application message")
	}

	return &gstMessage{msg: msg}, nil
}

// AddFieldToMessage adds a field to a message
func AddFieldToMessage(msg Message, fieldName, value string) error {
	gstMsg, ok := msg.(*gstMessage)
	if !ok {
		return errors.New("invalid message type")
	}

	if gstMsg.msg == nil {
		return errors.New("nil message")
	}

	cFieldName := C.CString(fieldName)
	defer C.free(unsafe.Pointer(cFieldName))

	cValue := C.CString(value)
	defer C.free(unsafe.Pointer(cValue))

	C.add_field_to_message(gstMsg.msg, cFieldName, cValue)

	return nil
}

// GetFieldFromMessage gets a field from a message
func GetFieldFromMessage(msg Message, fieldName string) (string, error) {
	gstMsg, ok := msg.(*gstMessage)
	if !ok {
		return "", errors.New("invalid message type")
	}

	if gstMsg.msg == nil {
		return "", errors.New("nil message")
	}

	cFieldName := C.CString(fieldName)
	defer C.free(unsafe.Pointer(cFieldName))

	cValue := C.get_field_from_message(gstMsg.msg, cFieldName)
	if cValue == nil {
		return "", fmt.Errorf("field not found: %s", fieldName)
	}

	return C.GoString(cValue), nil
}

// CreateCustomEvent creates a custom event
func CreateCustomEvent(name string) (*C.GstEvent, error) {
	cName := C.CString(name)
	defer C.free(unsafe.Pointer(cName))

	event := C.create_custom_event(cName)
	if event == nil {
		return nil, errors.New("failed to create custom event")
	}

	return event, nil
}

// AddFieldToEvent adds a field to an event
func AddFieldToEvent(event *C.GstEvent, fieldName, value string) error {
	if event == nil {
		return errors.New("nil event")
	}

	cFieldName := C.CString(fieldName)
	defer C.free(unsafe.Pointer(cFieldName))

	cValue := C.CString(value)
	defer C.free(unsafe.Pointer(cValue))

	C.add_field_to_event(event, cFieldName, cValue)

	return nil
}

// SendCustomEvent sends a custom event to an element
func SendCustomEvent(e Element, eventName string, fields map[string]string) error {
	elem, ok := e.(*element)
	if !ok {
		return errors.New("invalid element type")
	}

	if elem.elem == nil {
		return ErrElementNotFound
	}

	event, err := CreateCustomEvent(eventName)
	if err != nil {
		return err
	}

	for name, value := range fields {
		err = AddFieldToEvent(event, name, value)
		if err != nil {
			return err
		}
	}

	ret := C.send_event_to_element(elem.elem, event)
	if ret == 0 {
		return fmt.Errorf("failed to send event: %s", eventName)
	}

	return nil
}

// RegisterBusCallback registers a callback for bus messages
func RegisterBusCallback(p Pipeline, handler MessageHandler) (uint, error) {
	bus, err := p.GetBus()
	if err != nil {
		return 0, err
	}

	return bus.AddWatch(func(msg Message) bool {
		return handler(msg)
	})
}

// UnregisterBusCallback unregisters a bus callback
func UnregisterBusCallback(p Pipeline, id uint) error {
	bus, err := p.GetBus()
	if err != nil {
		return err
	}

	return bus.RemoveWatch(id)
}
