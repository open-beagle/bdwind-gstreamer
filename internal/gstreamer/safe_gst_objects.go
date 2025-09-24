package gstreamer

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-gst/go-gst/gst"
	"github.com/sirupsen/logrus"
)

// SafeGstObject 提供安全的GStreamer对象访问，防止垃圾回收时的段错误
type SafeGstObject struct {
	object   *gst.Object
	valid    int32 // 原子操作标志：1=有效，0=无效
	refCount int32 // 引用计数
	mutex    sync.RWMutex
	id       string    // 用于调试的唯一标识
	created  time.Time // 创建时间
	objType  string    // 对象类型
}

// SafeGstElement 安全的GStreamer元素包装器
type SafeGstElement struct {
	*SafeGstObject
	element *gst.Element
}

// NewSafeGstObject 创建一个新的安全GStreamer对象包装器
func NewSafeGstObject(object *gst.Object, objType string) *SafeGstObject {
	if object == nil {
		return nil
	}

	id := fmt.Sprintf("%s_%p_%d", objType, object, time.Now().UnixNano())

	sgo := &SafeGstObject{
		object:   object,
		valid:    1, // 初始状态为有效
		refCount: 1, // 初始引用计数为1
		id:       id,
		created:  time.Now(),
		objType:  objType,
	}

	logrus.WithFields(logrus.Fields{
		"component":   "safe-gst-object",
		"object_id":   id,
		"object_type": objType,
		"address":     fmt.Sprintf("%p", object),
	}).Debug("Created safe GStreamer object wrapper")

	return sgo
}

// NewSafeGstElement 创建一个新的安全GStreamer元素包装器
func NewSafeGstElement(element *gst.Element, elementType string) *SafeGstElement {
	if element == nil {
		return nil
	}

	// 获取元素的基础对象
	baseObject := element.Object

	sgo := NewSafeGstObject(baseObject, fmt.Sprintf("Element.%s", elementType))
	if sgo == nil {
		return nil
	}

	sge := &SafeGstElement{
		SafeGstObject: sgo,
		element:       element,
	}

	logrus.WithFields(logrus.Fields{
		"component":    "safe-gst-element",
		"element_id":   sgo.id,
		"element_type": elementType,
		"element_name": element.GetName(),
		"address":      fmt.Sprintf("%p", element),
	}).Debug("Created safe GStreamer element wrapper")

	return sge
}

// IsValid 检查对象是否仍然有效
func (sgo *SafeGstObject) IsValid() bool {
	return atomic.LoadInt32(&sgo.valid) == 1
}

// GetID 返回对象的唯一标识符
func (sgo *SafeGstObject) GetID() string {
	return sgo.id
}

// GetType 返回对象类型
func (sgo *SafeGstObject) GetType() string {
	return sgo.objType
}

// GetRefCount 返回当前引用计数
func (sgo *SafeGstObject) GetRefCount() int32 {
	return atomic.LoadInt32(&sgo.refCount)
}

// AddRef 增加引用计数
func (sgo *SafeGstObject) AddRef() int32 {
	if !sgo.IsValid() {
		logrus.WithFields(logrus.Fields{
			"component": "safe-gst-object",
			"object_id": sgo.id,
			"action":    "add_ref_failed",
		}).Warn("Attempted to add reference to invalid object")
		return 0
	}

	newCount := atomic.AddInt32(&sgo.refCount, 1)

	logrus.WithFields(logrus.Fields{
		"component": "safe-gst-object",
		"object_id": sgo.id,
		"ref_count": newCount,
		"action":    "add_ref",
	}).Trace("Increased object reference count")

	return newCount
}

// Release 减少引用计数
func (sgo *SafeGstObject) Release() int32 {
	newCount := atomic.AddInt32(&sgo.refCount, -1)

	logrus.WithFields(logrus.Fields{
		"component": "safe-gst-object",
		"object_id": sgo.id,
		"ref_count": newCount,
		"action":    "release",
	}).Trace("Decreased object reference count")

	// 如果引用计数降到0且对象已无效，可以进行清理
	if newCount <= 0 && !sgo.IsValid() {
		logrus.WithFields(logrus.Fields{
			"component": "safe-gst-object",
			"object_id": sgo.id,
			"action":    "cleanup_ready",
		}).Debug("Object ready for cleanup")
	}

	return newCount
}

// Invalidate 标记对象为无效，防止进一步访问
func (sgo *SafeGstObject) Invalidate() {
	if !atomic.CompareAndSwapInt32(&sgo.valid, 1, 0) {
		// 对象已经无效
		return
	}

	sgo.mutex.Lock()
	defer sgo.mutex.Unlock()

	logrus.WithFields(logrus.Fields{
		"component": "safe-gst-object",
		"object_id": sgo.id,
		"ref_count": atomic.LoadInt32(&sgo.refCount),
		"action":    "invalidate",
	}).Debug("Invalidated GStreamer object")

	// 清除对象引用，但不直接释放，让垃圾回收器处理
	sgo.object = nil
}

// WithObject 提供安全的对象访问模式
func (sgo *SafeGstObject) WithObject(fn func(*gst.Object) error) error {
	if !sgo.IsValid() {
		return fmt.Errorf("object %s is no longer valid", sgo.id)
	}

	// 增加引用计数防止在访问期间被垃圾回收
	sgo.AddRef()
	defer sgo.Release()

	sgo.mutex.RLock()
	defer sgo.mutex.RUnlock()

	if sgo.object == nil {
		return fmt.Errorf("object %s is nil", sgo.id)
	}

	// 再次检查有效性（双重检查）
	if !sgo.IsValid() {
		return fmt.Errorf("object %s became invalid during access", sgo.id)
	}

	return fn(sgo.object)
}

// WithElement 提供安全的元素访问模式
func (sge *SafeGstElement) WithElement(fn func(*gst.Element) error) error {
	if !sge.IsValid() {
		return fmt.Errorf("element %s is no longer valid", sge.id)
	}

	// 增加引用计数防止在访问期间被垃圾回收
	sge.AddRef()
	defer sge.Release()

	sge.mutex.RLock()
	defer sge.mutex.RUnlock()

	if sge.element == nil {
		return fmt.Errorf("element %s is nil", sge.id)
	}

	// 再次检查有效性（双重检查）
	if !sge.IsValid() {
		return fmt.Errorf("element %s became invalid during access", sge.id)
	}

	return fn(sge.element)
}

// GetElement 返回底层的GStreamer元素（不安全，仅用于兼容性）
// 警告：直接使用返回的元素可能导致段错误，建议使用WithElement方法
func (sge *SafeGstElement) GetElement() *gst.Element {
	if !sge.IsValid() {
		logrus.WithFields(logrus.Fields{
			"component":  "safe-gst-element",
			"element_id": sge.id,
			"action":     "get_element_unsafe",
		}).Warn("Unsafe access to invalid element")
		return nil
	}

	sge.mutex.RLock()
	defer sge.mutex.RUnlock()

	return sge.element
}

// GetName 安全地获取元素名称
func (sge *SafeGstElement) GetName() string {
	var name string
	err := sge.WithElement(func(element *gst.Element) error {
		name = element.GetName()
		return nil
	})

	if err != nil {
		logrus.WithFields(logrus.Fields{
			"component":  "safe-gst-element",
			"element_id": sge.id,
			"error":      err.Error(),
		}).Warn("Failed to get element name")
		return ""
	}

	return name
}

// SetProperty 安全地设置元素属性
func (sge *SafeGstElement) SetProperty(name string, value interface{}) error {
	return sge.WithElement(func(element *gst.Element) error {
		return element.SetProperty(name, value)
	})
}

// GetProperty 安全地获取元素属性
func (sge *SafeGstElement) GetProperty(name string) (interface{}, error) {
	var result interface{}
	err := sge.WithElement(func(element *gst.Element) error {
		var err error
		result, err = element.GetProperty(name)
		return err
	})
	return result, err
}

// Link 安全地链接到另一个元素
func (sge *SafeGstElement) Link(dst *SafeGstElement) error {
	if dst == nil {
		return fmt.Errorf("destination element is nil")
	}

	return sge.WithElement(func(srcElement *gst.Element) error {
		return dst.WithElement(func(dstElement *gst.Element) error {
			return srcElement.Link(dstElement)
		})
	})
}

// SetState 安全地设置元素状态
func (sge *SafeGstElement) SetState(state gst.State) error {
	return sge.WithElement(func(element *gst.Element) error {
		return element.SetState(state)
	})
}

// GetState 安全地获取元素状态 - 暂时简化实现
// TODO: 实现正确的GetState API调用
func (sge *SafeGstElement) GetState() error {
	return sge.WithElement(func(element *gst.Element) error {
		// 暂时只验证元素可访问性，不调用GetState
		// 因为API签名不清楚
		return nil
	})
}

// ObjectStats 对象统计信息
type ObjectStats struct {
	TotalObjects    int     `json:"total_objects"`
	ValidObjects    int     `json:"valid_objects"`
	InvalidObjects  int     `json:"invalid_objects"`
	TotalRefCount   int     `json:"total_ref_count"`
	AverageRefCount float64 `json:"average_ref_count"`
}

// SafeObjectRegistry 安全对象注册表，用于跟踪所有创建的安全对象
type SafeObjectRegistry struct {
	objects map[string]*SafeGstObject
	mutex   sync.RWMutex
}

var globalRegistry = &SafeObjectRegistry{
	objects: make(map[string]*SafeGstObject),
}

// RegisterObject 注册一个安全对象到全局注册表
func (sor *SafeObjectRegistry) RegisterObject(obj *SafeGstObject) {
	if obj == nil {
		return
	}

	sor.mutex.Lock()
	defer sor.mutex.Unlock()

	sor.objects[obj.id] = obj

	logrus.WithFields(logrus.Fields{
		"component":     "safe-object-registry",
		"object_id":     obj.id,
		"object_type":   obj.objType,
		"total_objects": len(sor.objects),
	}).Trace("Registered safe object")
}

// UnregisterObject 从全局注册表中注销一个安全对象
func (sor *SafeObjectRegistry) UnregisterObject(obj *SafeGstObject) {
	if obj == nil {
		return
	}

	sor.mutex.Lock()
	defer sor.mutex.Unlock()

	delete(sor.objects, obj.id)

	logrus.WithFields(logrus.Fields{
		"component":     "safe-object-registry",
		"object_id":     obj.id,
		"total_objects": len(sor.objects),
	}).Trace("Unregistered safe object")
}

// GetStats 获取对象统计信息
func (sor *SafeObjectRegistry) GetStats() ObjectStats {
	sor.mutex.RLock()
	defer sor.mutex.RUnlock()

	stats := ObjectStats{
		TotalObjects: len(sor.objects),
	}

	totalRefCount := 0
	for _, obj := range sor.objects {
		if obj.IsValid() {
			stats.ValidObjects++
		} else {
			stats.InvalidObjects++
		}
		refCount := int(obj.GetRefCount())
		totalRefCount += refCount
	}

	stats.TotalRefCount = totalRefCount
	if stats.TotalObjects > 0 {
		stats.AverageRefCount = float64(totalRefCount) / float64(stats.TotalObjects)
	}

	return stats
}

// GetGlobalObjectStats 获取全局对象统计信息
func GetGlobalObjectStats() ObjectStats {
	return globalRegistry.GetStats()
}

// InvalidateAllObjects 使所有注册的对象无效（用于清理）
func InvalidateAllObjects() {
	globalRegistry.mutex.RLock()
	objects := make([]*SafeGstObject, 0, len(globalRegistry.objects))
	for _, obj := range globalRegistry.objects {
		objects = append(objects, obj)
	}
	globalRegistry.mutex.RUnlock()

	for _, obj := range objects {
		obj.Invalidate()
	}

	logrus.WithFields(logrus.Fields{
		"component":   "safe-object-registry",
		"invalidated": len(objects),
	}).Info("Invalidated all registered safe objects")
}
