package gstreamer

/*
#cgo pkg-config: gstreamer-1.0
#include <gst/gst.h>
#include <glib.h>
#include <stdlib.h>

// Helper function to get element factory metadata
static const gchar* element_factory_get_metadata(GstElementFactory *factory, const gchar *key) {
    return gst_element_factory_get_metadata(factory, key);
}

// Helper function to get element factory klass
static const gchar* element_factory_get_klass(GstElementFactory *factory) {
    return gst_element_factory_get_metadata(factory, GST_ELEMENT_METADATA_KLASS);
}

// Helper function to get element factory description
static const gchar* element_factory_get_description(GstElementFactory *factory) {
    return gst_element_factory_get_metadata(factory, GST_ELEMENT_METADATA_DESCRIPTION);
}

// Helper function to get element factory author
static const gchar* element_factory_get_author(GstElementFactory *factory) {
    return gst_element_factory_get_metadata(factory, GST_ELEMENT_METADATA_AUTHOR);
}

// Helper function to get element factory long name
static const gchar* element_factory_get_longname(GstElementFactory *factory) {
    return gst_element_factory_get_metadata(factory, GST_ELEMENT_METADATA_LONGNAME);
}

// Helper function to get plugin feature name
static const gchar* plugin_feature_get_name(GstPluginFeature *feature) {
    return gst_plugin_feature_get_name(feature);
}

// Helper function to get plugin feature rank
static guint plugin_feature_get_rank(GstPluginFeature *feature) {
    return gst_plugin_feature_get_rank(feature);
}

// Helper function to get element state
static GstStateChangeReturn element_get_state(GstElement *element, GstState *state, GstState *pending, GstClockTime timeout) {
    return gst_element_get_state(element, state, pending, timeout);
}

// Helper function to send event to element
static gboolean element_send_event(GstElement *element, GstEvent *event) {
    return gst_element_send_event(element, event);
}

// Helper function to query element
static gboolean element_query(GstElement *element, GstQuery *query) {
    return gst_element_query(element, query);
}

// Helper function to get element factory
static GstElementFactory* element_get_factory(GstElement *element) {
    return gst_element_get_factory(element);
}

// Helper function to create element from factory
static GstElement* element_factory_create(GstElementFactory *factory, const gchar *name) {
    return gst_element_factory_create(factory, name);
}

// Helper function to find element factory
static GstElementFactory* element_factory_find(const gchar *name) {
    return gst_element_factory_find(name);
}

// Helper function to list element factories
static GList* registry_get_element_factories() {
    GstRegistry *registry = gst_registry_get();
    return gst_registry_get_feature_list(registry, GST_TYPE_ELEMENT_FACTORY);
}
*/
import "C"
import (
	"errors"
	"fmt"
	"strings"
	"unsafe"
)

// ElementInfo contains information about a GStreamer element
type ElementInfo struct {
	Name        string
	LongName    string
	ClassName   string
	Description string
	Author      string
	Rank        int
}

// ElementFactory represents a GStreamer element factory
type ElementFactory interface {
	// GetName gets the factory name
	GetName() string

	// GetLongName gets the factory long name
	GetLongName() string

	// GetClassName gets the factory class name
	GetClassName() string

	// GetDescription gets the factory description
	GetDescription() string

	// GetAuthor gets the factory author
	GetAuthor() string

	// GetRank gets the factory rank
	GetRank() int

	// CreateElement creates an element from the factory
	CreateElement(name string) (Element, error)
}

// Implementation of the ElementFactory interface
type elementFactory struct {
	factory *C.GstElementFactory
}

// GetName gets the factory name
func (f *elementFactory) GetName() string {
	if f.factory == nil {
		return ""
	}

	return C.GoString(C.plugin_feature_get_name((*C.GstPluginFeature)(unsafe.Pointer(f.factory))))
}

// GetLongName gets the factory long name
func (f *elementFactory) GetLongName() string {
	if f.factory == nil {
		return ""
	}

	return C.GoString(C.element_factory_get_longname(f.factory))
}

// GetClassName gets the factory class name
func (f *elementFactory) GetClassName() string {
	if f.factory == nil {
		return ""
	}

	return C.GoString(C.element_factory_get_klass(f.factory))
}

// GetDescription gets the factory description
func (f *elementFactory) GetDescription() string {
	if f.factory == nil {
		return ""
	}

	return C.GoString(C.element_factory_get_description(f.factory))
}

// GetAuthor gets the factory author
func (f *elementFactory) GetAuthor() string {
	if f.factory == nil {
		return ""
	}

	return C.GoString(C.element_factory_get_author(f.factory))
}

// GetRank gets the factory rank
func (f *elementFactory) GetRank() int {
	if f.factory == nil {
		return 0
	}

	return int(C.plugin_feature_get_rank((*C.GstPluginFeature)(unsafe.Pointer(f.factory))))
}

// CreateElement creates an element from the factory
func (f *elementFactory) CreateElement(name string) (Element, error) {
	if f.factory == nil {
		return nil, ErrElementFactoryNotAvailable
	}

	cName := C.CString(name)
	defer C.free(unsafe.Pointer(cName))

	elem := C.element_factory_create(f.factory, cName)
	if elem == nil {
		return nil, fmt.Errorf("%w: %s", ErrElementNotFound, name)
	}

	return &element{elem: elem}, nil
}

// FindElementFactory finds an element factory by name
func FindElementFactory(name string) (ElementFactory, error) {
	cName := C.CString(name)
	defer C.free(unsafe.Pointer(cName))

	factory := C.element_factory_find(cName)
	if factory == nil {
		return nil, fmt.Errorf("%w: %s", ErrElementFactoryNotAvailable, name)
	}

	return &elementFactory{factory: factory}, nil
}

// ListElementFactories lists all available element factories
func ListElementFactories() []ElementInfo {
	factories := C.registry_get_element_factories()
	if factories == nil {
		return nil
	}
	defer C.g_list_free(factories)

	var result []ElementInfo
	for item := factories; item != nil; item = item.next {
		factory := (*C.GstElementFactory)(unsafe.Pointer(item.data))
		if factory == nil {
			continue
		}

		info := ElementInfo{
			Name:        C.GoString(C.plugin_feature_get_name((*C.GstPluginFeature)(unsafe.Pointer(factory)))),
			LongName:    C.GoString(C.element_factory_get_longname(factory)),
			ClassName:   C.GoString(C.element_factory_get_klass(factory)),
			Description: C.GoString(C.element_factory_get_description(factory)),
			Author:      C.GoString(C.element_factory_get_author(factory)),
			Rank:        int(C.plugin_feature_get_rank((*C.GstPluginFeature)(unsafe.Pointer(factory)))),
		}

		result = append(result, info)
	}

	return result
}

// FindElementFactoriesByClass finds element factories by class
func FindElementFactoriesByClass(className string) []ElementInfo {
	factories := ListElementFactories()
	var result []ElementInfo

	for _, factory := range factories {
		if strings.Contains(strings.ToLower(factory.ClassName), strings.ToLower(className)) {
			result = append(result, factory)
		}
	}

	return result
}

// ElementState represents the state of a GStreamer element
type ElementState int

const (
	ElementStateVoidPending ElementState = -1
	ElementStateNull        ElementState = 0
	ElementStateReady       ElementState = 1
	ElementStatePaused      ElementState = 2
	ElementStatePlaying     ElementState = 3
)

// GetElementState gets the state of an element
func GetElementState(e Element) (ElementState, error) {
	elem, ok := e.(*element)
	if !ok {
		return ElementStateNull, errors.New("invalid element type")
	}

	if elem.elem == nil {
		return ElementStateNull, ErrElementNotFound
	}

	var state, pending C.GstState
	var timeout C.GstClockTime = 0 // Immediate query

	ret := C.element_get_state(elem.elem, &state, &pending, timeout)
	if ret == C.GST_STATE_CHANGE_FAILURE {
		return ElementStateNull, ErrInvalidState
	}

	return ElementState(state), nil
}

// SendEvent sends an event to an element
func SendEvent(e Element, eventType string) error {
	elem, ok := e.(*element)
	if !ok {
		return errors.New("invalid element type")
	}

	if elem.elem == nil {
		return ErrElementNotFound
	}

	var event *C.GstEvent

	switch eventType {
	case "eos":
		event = C.gst_event_new_eos()
	case "flush-start":
		event = C.gst_event_new_flush_start()
	case "flush-stop":
		event = C.gst_event_new_flush_stop(1) // Reset time
	default:
		return fmt.Errorf("unsupported event type: %s", eventType)
	}

	if event == nil {
		return fmt.Errorf("failed to create event: %s", eventType)
	}

	ret := C.element_send_event(elem.elem, event)
	if ret == 0 {
		return fmt.Errorf("failed to send event: %s", eventType)
	}

	return nil
}

// GetElementFactory gets the factory of an element
func GetElementFactory(e Element) (ElementFactory, error) {
	elem, ok := e.(*element)
	if !ok {
		return nil, errors.New("invalid element type")
	}

	if elem.elem == nil {
		return nil, ErrElementNotFound
	}

	factory := C.element_get_factory(elem.elem)
	if factory == nil {
		return nil, ErrElementFactoryNotAvailable
	}

	return &elementFactory{factory: factory}, nil
}
