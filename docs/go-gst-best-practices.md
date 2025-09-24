# Go-GST 最佳实践指南

## 概述

本文档基于 go-gst 官方示例和社区最佳实践，为 bdwind-gstreamer 项目的重构提供指导。go-gst 是 GStreamer 的 Go 语言绑定，提供了类型安全的 API 来构建媒体处理管道。

## 核心概念

### GStreamer 基础架构

```go
// 基本的 GStreamer 应用程序结构
type GStreamerApp struct {
    pipeline *gst.Pipeline
    bus      *gst.Bus
    mainLoop *glib.MainLoop
    
    // 应用程序状态
    isRunning bool
    ctx       context.Context
    cancel    context.CancelFunc
}
```

### 初始化模式

```go
// 正确的初始化顺序
func NewGStreamerApp() (*GStreamerApp, error) {
    // 1. 初始化 GStreamer
    gst.Init(nil)
    
    // 2. 创建主循环
    mainLoop := glib.NewMainLoop(glib.MainContextDefault(), false)
    
    // 3. 创建管道
    pipeline, err := gst.NewPipeline("main-pipeline")
    if err != nil {
        return nil, fmt.Errorf("failed to create pipeline: %w", err)
    }
    
    // 4. 获取总线
    bus := pipeline.GetBus()
    
    // 5. 创建上下文
    ctx, cancel := context.WithCancel(context.Background())
    
    return &GStreamerApp{
        pipeline: pipeline,
        bus:      bus,
        mainLoop: mainLoop,
        ctx:      ctx,
        cancel:   cancel,
    }, nil
}
```

## 内存管理最佳实践

### 1. 对象生命周期管理

```go
// ObjectRef 用于管理 GStreamer 对象的生命周期
type ObjectRef struct {
    object   interface{}
    refCount int32
    mutex    sync.Mutex
}

// MemoryManager 管理所有 GStreamer 对象的内存
type MemoryManager struct {
    objects map[uintptr]*ObjectRef
    mutex   sync.RWMutex
    
    // GC 安全对象列表
    gcSafeObjects []interface{}
    gcMutex       sync.Mutex
}

// RegisterObject 注册需要管理的 GStreamer 对象
func (mm *MemoryManager) RegisterObject(obj interface{}) *ObjectRef {
    mm.mutex.Lock()
    defer mm.mutex.Unlock()
    
    ptr := reflect.ValueOf(obj).Pointer()
    if ref, exists := mm.objects[ptr]; exists {
        atomic.AddInt32(&ref.refCount, 1)
        return ref
    }
    
    ref := &ObjectRef{
        object:   obj,
        refCount: 1,
    }
    mm.objects[ptr] = ref
    
    return ref
}

// UnregisterObject 取消注册对象
func (mm *MemoryManager) UnregisterObject(ref *ObjectRef) error {
    if ref == nil {
        return nil
    }
    
    ref.mutex.Lock()
    defer ref.mutex.Unlock()
    
    if atomic.AddInt32(&ref.refCount, -1) <= 0 {
        mm.mutex.Lock()
        defer mm.mutex.Unlock()
        
        ptr := reflect.ValueOf(ref.object).Pointer()
        delete(mm.objects, ptr)
        
        // 如果是 GStreamer 对象，调用 Unref
        if gstObj, ok := ref.object.(interface{ Unref() }); ok {
            gstObj.Unref()
        }
    }
    
    return nil
}
```

### 2. GC 安全管理

```go
// KeepGCSafe 防止 Go 对象被过早回收
func (mm *MemoryManager) KeepGCSafe(obj interface{}) {
    mm.gcMutex.Lock()
    defer mm.gcMutex.Unlock()
    
    mm.gcSafeObjects = append(mm.gcSafeObjects, obj)
}

// ReleaseGCSafe 释放 GC 安全引用
func (mm *MemoryManager) ReleaseGCSafe(obj interface{}) {
    mm.gcMutex.Lock()
    defer mm.gcMutex.Unlock()
    
    for i, safeObj := range mm.gcSafeObjects {
        if safeObj == obj {
            // 移除对象
            mm.gcSafeObjects = append(mm.gcSafeObjects[:i], mm.gcSafeObjects[i+1:]...)
            break
        }
    }
}
```

### 3. 缓冲区池管理

```go
// BufferPool 管理 GStreamer 缓冲区
type BufferPool struct {
    pool     *gst.BufferPool
    config   *gst.Structure
    size     uint
    minBuffers uint
    maxBuffers uint
}

// NewBufferPool 创建新的缓冲区池
func NewBufferPool(size, minBuffers, maxBuffers uint) (*BufferPool, error) {
    pool := gst.NewBufferPool()
    
    config := gst.NewStructure("buffer-pool-config")
    config.SetValue("size", size)
    config.SetValue("min-buffers", minBuffers)
    config.SetValue("max-buffers", maxBuffers)
    
    if !pool.SetConfig(config) {
        return nil, fmt.Errorf("failed to set buffer pool config")
    }
    
    return &BufferPool{
        pool:       pool,
        config:     config,
        size:       size,
        minBuffers: minBuffers,
        maxBuffers: maxBuffers,
    }, nil
}

// AcquireBuffer 获取缓冲区
func (bp *BufferPool) AcquireBuffer() (*gst.Buffer, error) {
    buffer := bp.pool.AcquireBuffer(gst.BUFFER_POOL_ACQUIRE_FLAG_NONE)
    if buffer == nil {
        return nil, fmt.Errorf("failed to acquire buffer from pool")
    }
    return buffer, nil
}

// ReleaseBuffer 释放缓冲区
func (bp *BufferPool) ReleaseBuffer(buffer *gst.Buffer) {
    if buffer != nil {
        buffer.Unref()
    }
}
```

## 元素创建和配置

### 1. 元素工厂模式

```go
// ElementFactory 元素创建工厂
type ElementFactory struct {
    memManager *MemoryManager
    logger     *logrus.Entry
}

// CreateElement 创建 GStreamer 元素
func (ef *ElementFactory) CreateElement(factoryName, elementName string) (*gst.Element, error) {
    element, err := gst.NewElement(factoryName)
    if err != nil {
        return nil, fmt.Errorf("failed to create element %s: %w", factoryName, err)
    }
    
    if elementName != "" {
        element.SetName(elementName)
    }
    
    // 注册到内存管理器
    ef.memManager.RegisterObject(element)
    ef.memManager.KeepGCSafe(element)
    
    ef.logger.Debugf("Created element: %s (%s)", factoryName, elementName)
    
    return element, nil
}

// ConfigureElement 配置元素属性
func (ef *ElementFactory) ConfigureElement(element *gst.Element, properties map[string]interface{}) error {
    for key, value := range properties {
        if err := element.SetProperty(key, value); err != nil {
            return fmt.Errorf("failed to set property %s=%v: %w", key, value, err)
        }
        ef.logger.Debugf("Set property %s=%v on element %s", key, value, element.GetName())
    }
    return nil
}
```

### 2. 管道构建模式

```go
// PipelineBuilder 管道构建器
type PipelineBuilder struct {
    pipeline    *gst.Pipeline
    elements    []*gst.Element
    factory     *ElementFactory
    memManager  *MemoryManager
    logger      *logrus.Entry
}

// NewPipelineBuilder 创建管道构建器
func NewPipelineBuilder(name string, memManager *MemoryManager) (*PipelineBuilder, error) {
    pipeline, err := gst.NewPipeline(name)
    if err != nil {
        return nil, fmt.Errorf("failed to create pipeline: %w", err)
    }
    
    factory := &ElementFactory{
        memManager: memManager,
        logger:     logrus.WithField("component", "element-factory"),
    }
    
    return &PipelineBuilder{
        pipeline:   pipeline,
        elements:   make([]*gst.Element, 0),
        factory:    factory,
        memManager: memManager,
        logger:     logrus.WithField("component", "pipeline-builder"),
    }, nil
}

// AddElement 添加元素到管道
func (pb *PipelineBuilder) AddElement(factoryName, elementName string, properties map[string]interface{}) (*gst.Element, error) {
    element, err := pb.factory.CreateElement(factoryName, elementName)
    if err != nil {
        return nil, err
    }
    
    if properties != nil {
        if err := pb.factory.ConfigureElement(element, properties); err != nil {
            element.Unref()
            return nil, err
        }
    }
    
    if !pb.pipeline.Add(element) {
        element.Unref()
        return nil, fmt.Errorf("failed to add element %s to pipeline", elementName)
    }
    
    pb.elements = append(pb.elements, element)
    pb.logger.Debugf("Added element %s to pipeline", elementName)
    
    return element, nil
}

// LinkElements 链接元素
func (pb *PipelineBuilder) LinkElements(src, dst *gst.Element) error {
    if !gst.ElementLink(src, dst) {
        return fmt.Errorf("failed to link elements %s -> %s", src.GetName(), dst.GetName())
    }
    
    pb.logger.Debugf("Linked elements: %s -> %s", src.GetName(), dst.GetName())
    return nil
}

// Build 构建完成的管道
func (pb *PipelineBuilder) Build() *gst.Pipeline {
    pb.memManager.RegisterObject(pb.pipeline)
    pb.memManager.KeepGCSafe(pb.pipeline)
    
    pb.logger.Infof("Pipeline built with %d elements", len(pb.elements))
    return pb.pipeline
}
```

## 错误处理模式

### 1. 总线消息处理

```go
// BusMessageHandler 总线消息处理器
type BusMessageHandler struct {
    pipeline   *gst.Pipeline
    mainLoop   *glib.MainLoop
    errorChan  chan error
    eosReceived bool
    logger     *logrus.Entry
}

// StartMessageHandling 开始处理总线消息
func (bmh *BusMessageHandler) StartMessageHandling() {
    bus := bmh.pipeline.GetBus()
    
    // 设置消息处理回调
    bus.AddWatch(func(msg *gst.Message) bool {
        return bmh.handleMessage(msg)
    })
}

// handleMessage 处理单个消息
func (bmh *BusMessageHandler) handleMessage(msg *gst.Message) bool {
    switch msg.Type() {
    case gst.MESSAGE_ERROR:
        err, debug := msg.ParseError()
        bmh.logger.Errorf("Pipeline error: %s (debug: %s)", err.Error(), debug)
        bmh.errorChan <- fmt.Errorf("pipeline error: %s", err.Error())
        bmh.mainLoop.Quit()
        return false
        
    case gst.MESSAGE_WARNING:
        warning, debug := msg.ParseWarning()
        bmh.logger.Warnf("Pipeline warning: %s (debug: %s)", warning.Error(), debug)
        
    case gst.MESSAGE_INFO:
        info, debug := msg.ParseInfo()
        bmh.logger.Infof("Pipeline info: %s (debug: %s)", info.Error(), debug)
        
    case gst.MESSAGE_EOS:
        bmh.logger.Info("End of stream received")
        bmh.eosReceived = true
        bmh.mainLoop.Quit()
        return false
        
    case gst.MESSAGE_STATE_CHANGED:
        if msg.Source() == bmh.pipeline.Element {
            oldState, newState, _ := msg.ParseStateChanged()
            bmh.logger.Debugf("Pipeline state changed: %s -> %s", 
                oldState.String(), newState.String())
        }
        
    case gst.MESSAGE_BUFFERING:
        percent := msg.ParseBuffering()
        bmh.logger.Debugf("Buffering: %d%%", percent)
        
        // 在缓冲时暂停管道
        if percent < 100 {
            bmh.pipeline.SetState(gst.STATE_PAUSED)
        } else {
            bmh.pipeline.SetState(gst.STATE_PLAYING)
        }
    }
    
    return true
}
```

### 2. 状态管理

```go
// StateManager 状态管理器
type StateManager struct {
    pipeline      *gst.Pipeline
    currentState  gst.State
    targetState   gst.State
    stateMutex    sync.RWMutex
    stateChanges  chan StateChange
    logger        *logrus.Entry
}

type StateChange struct {
    From      gst.State
    To        gst.State
    Timestamp time.Time
    Success   bool
    Error     error
}

// SetState 设置管道状态
func (sm *StateManager) SetState(state gst.State) error {
    sm.stateMutex.Lock()
    defer sm.stateMutex.Unlock()
    
    oldState := sm.currentState
    sm.targetState = state
    
    sm.logger.Debugf("Setting pipeline state: %s -> %s", oldState.String(), state.String())
    
    // 设置状态并等待完成
    ret := sm.pipeline.SetState(state)
    
    stateChange := StateChange{
        From:      oldState,
        To:        state,
        Timestamp: time.Now(),
    }
    
    switch ret {
    case gst.STATE_CHANGE_SUCCESS:
        sm.currentState = state
        stateChange.Success = true
        sm.logger.Debugf("State change successful: %s", state.String())
        
    case gst.STATE_CHANGE_ASYNC:
        // 异步状态变化，等待完成
        finalRet, finalState, _ := sm.pipeline.GetState(gst.CLOCK_TIME_NONE)
        if finalRet == gst.STATE_CHANGE_SUCCESS {
            sm.currentState = finalState
            stateChange.Success = true
            stateChange.To = finalState
        } else {
            stateChange.Success = false
            stateChange.Error = fmt.Errorf("async state change failed: %s", finalRet.String())
        }
        
    case gst.STATE_CHANGE_FAILURE:
        stateChange.Success = false
        stateChange.Error = fmt.Errorf("state change failed: %s", ret.String())
        sm.logger.Errorf("Failed to set state to %s: %s", state.String(), ret.String())
        
    case gst.STATE_CHANGE_NO_PREROLL:
        sm.currentState = state
        stateChange.Success = true
        sm.logger.Debugf("State change no preroll: %s", state.String())
    }
    
    // 发送状态变化通知
    select {
    case sm.stateChanges <- stateChange:
    default:
        // 通道满了，丢弃旧的状态变化
    }
    
    return stateChange.Error
}

// GetState 获取当前状态
func (sm *StateManager) GetState() gst.State {
    sm.stateMutex.RLock()
    defer sm.stateMutex.RUnlock()
    return sm.currentState
}

// WaitForState 等待特定状态
func (sm *StateManager) WaitForState(state gst.State, timeout time.Duration) error {
    deadline := time.Now().Add(timeout)
    
    for time.Now().Before(deadline) {
        if sm.GetState() == state {
            return nil
        }
        time.Sleep(10 * time.Millisecond)
    }
    
    return fmt.Errorf("timeout waiting for state %s", state.String())
}
```

## 回调和信号处理

### 1. 安全的回调管理

```go
// CallbackManager 回调管理器
type CallbackManager struct {
    callbacks map[string][]interface{}
    mutex     sync.RWMutex
    memManager *MemoryManager
    logger    *logrus.Entry
}

// RegisterCallback 注册回调函数
func (cm *CallbackManager) RegisterCallback(signal string, callback interface{}) {
    cm.mutex.Lock()
    defer cm.mutex.Unlock()
    
    if cm.callbacks[signal] == nil {
        cm.callbacks[signal] = make([]interface{}, 0)
    }
    
    cm.callbacks[signal] = append(cm.callbacks[signal], callback)
    
    // 保持回调函数不被 GC
    cm.memManager.KeepGCSafe(callback)
    
    cm.logger.Debugf("Registered callback for signal: %s", signal)
}

// UnregisterCallback 取消注册回调函数
func (cm *CallbackManager) UnregisterCallback(signal string, callback interface{}) {
    cm.mutex.Lock()
    defer cm.mutex.Unlock()
    
    callbacks := cm.callbacks[signal]
    for i, cb := range callbacks {
        if cb == callback {
            // 移除回调
            cm.callbacks[signal] = append(callbacks[:i], callbacks[i+1:]...)
            
            // 释放 GC 安全引用
            cm.memManager.ReleaseGCSafe(callback)
            
            cm.logger.Debugf("Unregistered callback for signal: %s", signal)
            break
        }
    }
}
```

### 2. Pad 探测器模式

```go
// PadProbe Pad 探测器
type PadProbe struct {
    pad        *gst.Pad
    probeID    gst.PadProbeID
    callback   gst.PadProbeCallback
    memManager *MemoryManager
    logger     *logrus.Entry
}

// NewPadProbe 创建 Pad 探测器
func NewPadProbe(pad *gst.Pad, probeType gst.PadProbeType, callback gst.PadProbeCallback, memManager *MemoryManager) *PadProbe {
    probe := &PadProbe{
        pad:        pad,
        callback:   callback,
        memManager: memManager,
        logger:     logrus.WithField("component", "pad-probe"),
    }
    
    // 保持回调函数不被 GC
    memManager.KeepGCSafe(callback)
    
    // 添加探测器
    probe.probeID = pad.AddProbe(probeType, callback)
    
    probe.logger.Debugf("Added pad probe on %s", pad.GetName())
    
    return probe
}

// Remove 移除探测器
func (pp *PadProbe) Remove() {
    if pp.probeID != 0 {
        pp.pad.RemoveProbe(pp.probeID)
        pp.memManager.ReleaseGCSafe(pp.callback)
        pp.probeID = 0
        pp.logger.Debugf("Removed pad probe from %s", pp.pad.GetName())
    }
}
```

## 应用程序生命周期

### 1. 优雅启动

```go
// Start 启动应用程序
func (app *GStreamerApp) Start(ctx context.Context) error {
    app.logger.Info("Starting GStreamer application")
    
    // 1. 设置总线消息处理
    app.setupBusHandling()
    
    // 2. 设置管道状态为 READY
    if err := app.stateManager.SetState(gst.STATE_READY); err != nil {
        return fmt.Errorf("failed to set pipeline to READY: %w", err)
    }
    
    // 3. 等待 READY 状态
    if err := app.stateManager.WaitForState(gst.STATE_READY, 5*time.Second); err != nil {
        return fmt.Errorf("timeout waiting for READY state: %w", err)
    }
    
    // 4. 设置管道状态为 PLAYING
    if err := app.stateManager.SetState(gst.STATE_PLAYING); err != nil {
        return fmt.Errorf("failed to set pipeline to PLAYING: %w", err)
    }
    
    // 5. 启动主循环
    go func() {
        app.logger.Info("Starting main loop")
        app.mainLoop.Run()
        app.logger.Info("Main loop stopped")
    }()
    
    app.isRunning = true
    app.logger.Info("GStreamer application started successfully")
    
    return nil
}
```

### 2. 优雅关闭

```go
// Stop 停止应用程序
func (app *GStreamerApp) Stop() error {
    if !app.isRunning {
        return nil
    }
    
    app.logger.Info("Stopping GStreamer application")
    
    // 1. 取消上下文
    app.cancel()
    
    // 2. 发送 EOS 事件
    if !app.pipeline.SendEvent(gst.NewEOSEvent()) {
        app.logger.Warn("Failed to send EOS event")
    }
    
    // 3. 等待 EOS 或超时
    select {
    case <-time.After(5 * time.Second):
        app.logger.Warn("Timeout waiting for EOS, forcing shutdown")
    case <-app.ctx.Done():
        app.logger.Debug("Context cancelled, proceeding with shutdown")
    }
    
    // 4. 停止主循环
    app.mainLoop.Quit()
    
    // 5. 设置管道状态为 NULL
    if err := app.stateManager.SetState(gst.STATE_NULL); err != nil {
        app.logger.Errorf("Failed to set pipeline to NULL: %v", err)
    }
    
    // 6. 清理资源
    app.cleanup()
    
    app.isRunning = false
    app.logger.Info("GStreamer application stopped")
    
    return nil
}

// cleanup 清理资源
func (app *GStreamerApp) cleanup() {
    // 清理回调管理器
    if app.callbackManager != nil {
        app.callbackManager.UnregisterAll()
    }
    
    // 清理内存管理器
    if app.memManager != nil {
        app.memManager.ReleaseAll()
    }
    
    // 取消注册总线监听
    if app.bus != nil {
        app.bus.RemoveWatch()
    }
}
```

## 调试和监控

### 1. 调试信息收集

```go
// DebugInfo 调试信息
type DebugInfo struct {
    PipelineState    string                 `json:"pipeline_state"`
    Elements         []ElementInfo          `json:"elements"`
    MemoryUsage      MemoryStats           `json:"memory_usage"`
    ErrorHistory     []ErrorRecord         `json:"error_history"`
    PerformanceStats PerformanceStats      `json:"performance_stats"`
    Timestamp        time.Time             `json:"timestamp"`
}

type ElementInfo struct {
    Name         string            `json:"name"`
    Factory      string            `json:"factory"`
    State        string            `json:"state"`
    Properties   map[string]interface{} `json:"properties"`
    PadInfo      []PadInfo         `json:"pads"`
}

type PadInfo struct {
    Name      string `json:"name"`
    Direction string `json:"direction"`
    IsLinked  bool   `json:"is_linked"`
    Caps      string `json:"caps"`
}

// CollectDebugInfo 收集调试信息
func (app *GStreamerApp) CollectDebugInfo() *DebugInfo {
    info := &DebugInfo{
        PipelineState: app.stateManager.GetState().String(),
        Timestamp:     time.Now(),
    }
    
    // 收集元素信息
    info.Elements = app.collectElementInfo()
    
    // 收集内存使用信息
    info.MemoryUsage = app.memManager.GetStats()
    
    // 收集错误历史
    info.ErrorHistory = app.errorManager.GetErrorHistory()
    
    // 收集性能统计
    info.PerformanceStats = app.performanceMonitor.GetStats()
    
    return info
}
```

### 2. 性能监控

```go
// PerformanceMonitor 性能监控器
type PerformanceMonitor struct {
    frameCount    int64
    lastFrameTime time.Time
    frameRate     float64
    
    bytesProcessed int64
    lastByteTime   time.Time
    throughput     float64
    
    latencySum     time.Duration
    latencyCount   int64
    avgLatency     time.Duration
    
    mutex          sync.RWMutex
    logger         *logrus.Entry
}

// RecordFrame 记录帧处理
func (pm *PerformanceMonitor) RecordFrame() {
    pm.mutex.Lock()
    defer pm.mutex.Unlock()
    
    now := time.Now()
    pm.frameCount++
    
    if !pm.lastFrameTime.IsZero() {
        duration := now.Sub(pm.lastFrameTime)
        pm.frameRate = 1.0 / duration.Seconds()
    }
    
    pm.lastFrameTime = now
}

// RecordBytes 记录字节处理
func (pm *PerformanceMonitor) RecordBytes(bytes int64) {
    pm.mutex.Lock()
    defer pm.mutex.Unlock()
    
    now := time.Now()
    pm.bytesProcessed += bytes
    
    if !pm.lastByteTime.IsZero() {
        duration := now.Sub(pm.lastByteTime)
        pm.throughput = float64(bytes) / duration.Seconds()
    }
    
    pm.lastByteTime = now
}

// RecordLatency 记录延迟
func (pm *PerformanceMonitor) RecordLatency(latency time.Duration) {
    pm.mutex.Lock()
    defer pm.mutex.Unlock()
    
    pm.latencySum += latency
    pm.latencyCount++
    pm.avgLatency = pm.latencySum / time.Duration(pm.latencyCount)
}

// GetStats 获取性能统计
func (pm *PerformanceMonitor) GetStats() PerformanceStats {
    pm.mutex.RLock()
    defer pm.mutex.RUnlock()
    
    return PerformanceStats{
        FrameCount:     pm.frameCount,
        FrameRate:      pm.frameRate,
        BytesProcessed: pm.bytesProcessed,
        Throughput:     pm.throughput,
        AverageLatency: pm.avgLatency,
        Timestamp:      time.Now(),
    }
}
```

## 常见陷阱和解决方案

### 1. 内存泄漏

**问题：** GStreamer 对象没有正确释放引用

**解决方案：**
```go
// 错误的做法
element, _ := gst.NewElement("videotestsrc")
// 忘记调用 Unref()

// 正确的做法
element, err := gst.NewElement("videotestsrc")
if err != nil {
    return err
}
defer element.Unref() // 确保释放引用

// 或者使用内存管理器
ref := memManager.RegisterObject(element)
defer memManager.UnregisterObject(ref)
```

### 2. 回调函数被 GC

**问题：** Go 回调函数被垃圾回收器回收

**解决方案：**
```go
// 错误的做法
pad.AddProbe(gst.PAD_PROBE_TYPE_BUFFER, func(pad *gst.Pad, info *gst.PadProbeInfo) gst.PadProbeReturn {
    // 这个匿名函数可能被 GC
    return gst.PAD_PROBE_OK
})

// 正确的做法
callback := func(pad *gst.Pad, info *gst.PadProbeInfo) gst.PadProbeReturn {
    return gst.PAD_PROBE_OK
}
memManager.KeepGCSafe(callback) // 防止被 GC
probeID := pad.AddProbe(gst.PAD_PROBE_TYPE_BUFFER, callback)
defer func() {
    pad.RemoveProbe(probeID)
    memManager.ReleaseGCSafe(callback)
}()
```

### 3. 状态变化竞争

**问题：** 并发状态变化导致的竞争条件

**解决方案：**
```go
// 使用状态管理器确保线程安全
func (app *GStreamerApp) SafeStateChange(newState gst.State) error {
    app.stateMutex.Lock()
    defer app.stateMutex.Unlock()
    
    currentState := app.pipeline.GetCurrentState()
    if currentState == newState {
        return nil // 已经是目标状态
    }
    
    return app.stateManager.SetState(newState)
}
```

### 4. 管道死锁

**问题：** 不正确的元素链接导致管道死锁

**解决方案：**
```go
// 使用队列元素避免死锁
func (pb *PipelineBuilder) AddQueueElement(name string) (*gst.Element, error) {
    return pb.AddElement("queue", name, map[string]interface{}{
        "max-size-buffers": 10,
        "max-size-bytes":   0,
        "max-size-time":    0,
        "leaky":           2, // 丢弃旧缓冲区
    })
}
```

## 总结

本文档提供了基于 go-gst 的 GStreamer 应用程序开发最佳实践，包括：

1. **内存管理** - 正确的对象生命周期管理和 GC 安全
2. **元素管理** - 安全的元素创建、配置和链接
3. **错误处理** - 完善的错误处理和状态管理
4. **回调管理** - 安全的回调函数注册和清理
5. **生命周期** - 优雅的启动和关闭流程
6. **调试监控** - 完整的调试信息收集和性能监控

遵循这些最佳实践可以确保 GStreamer 应用程序的稳定性、性能和可维护性。