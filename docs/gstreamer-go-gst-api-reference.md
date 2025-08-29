# GStreamer Go-GST API 参考文档

## 概述

本文档提供 GStreamer Go-GST 重构版本的完整 API 参考，包括核心接口、数据结构和使用示例。

## 核心接口

### Manager 接口

Manager 是 GStreamer 组件的主要管理接口，负责组件的生命周期管理和协调。

```go
type ManagerInterface interface {
    // 生命周期管理
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
    ForceStop() error
    IsRunning() bool
    GetUptime() time.Duration

    // 配置管理
    GetConfig() *config.GStreamerConfig
    ConfigureLogging(appConfig *config.LoggingConfig) error

    // 组件状态
    GetComponentStatus() map[string]bool
    GetStats() map[string]interface{}
    GetContext() context.Context

    // Pipeline 管理
    GetPipelineState() (PipelineState, error)
    SetPipelineState(state PipelineState) error

    // 流媒体发布
    AddSubscriber(subscriber MediaStreamSubscriber) error
    RemoveSubscriber(subscriber MediaStreamSubscriber) error
}
```

#### 使用示例

```go
// 创建 Manager 实例
manager, err := gstreamer.NewManager(ctx, &ManagerConfig{
    Config: gstreamerConfig,
    Logger: logger,
})
if err != nil {
    return fmt.Errorf("failed to create manager: %w", err)
}

// 启动 GStreamer 组件
if err := manager.Start(ctx); err != nil {
    return fmt.Errorf("failed to start GStreamer: %w", err)
}

// 检查运行状态
if manager.IsRunning() {
    fmt.Println("GStreamer is running")
}

// 获取统计信息
stats := manager.GetStats()
fmt.Printf("Uptime: %v\n", stats["uptime"])

// 停止组件
if err := manager.Stop(ctx); err != nil {
    log.Printf("Failed to stop GStreamer: %v", err)
}
```

### DesktopCapture 接口

DesktopCapture 接口负责桌面视频捕获功能。

```go
type DesktopCapture interface {
    // 生命周期管理
    Start() error
    Stop() error
    IsRunning() bool

    // 数据输出
    GetOutputChannel() <-chan *Sample

    // 配置管理
    Configure(config *config.CaptureConfig) error
    GetConfig() *config.CaptureConfig

    // 状态和统计
    GetStats() CaptureStats
    GetState() CaptureState

    // 事件处理
    SetErrorHandler(handler ErrorHandler)
    SetStateChangeHandler(handler StateChangeHandler)
}
```

#### CaptureStats 数据结构

```go
type CaptureStats struct {
    FramesTotal     uint64        `json:"frames_total"`
    FramesDropped   uint64        `json:"frames_dropped"`
    FrameRate       float64       `json:"frame_rate"`
    AverageLatency  time.Duration `json:"average_latency"`
    CurrentLatency  time.Duration `json:"current_latency"`
    Resolution      Resolution    `json:"resolution"`
    LastFrameTime   time.Time     `json:"last_frame_time"`
    ErrorCount      uint64        `json:"error_count"`
    LastError       string        `json:"last_error,omitempty"`
}

type Resolution struct {
    Width  int `json:"width"`
    Height int `json:"height"`
}
```

#### 使用示例

```go
// 创建桌面捕获实例
capture := gstreamer.NewDesktopCaptureGst(&config.CaptureConfig{
    DisplayID:   ":0",
    Width:       1920,
    Height:      1080,
    FrameRate:   30,
    UseWayland:  false,
    ShowPointer: true,
})

// 启动捕获
if err := capture.Start(); err != nil {
    return fmt.Errorf("failed to start capture: %w", err)
}

// 处理捕获的样本
sampleChan := capture.GetOutputChannel()
go func() {
    for sample := range sampleChan {
        // 处理视频帧
        fmt.Printf("Received frame: %d bytes\n", sample.Size)
    }
}()

// 获取统计信息
stats := capture.GetStats()
fmt.Printf("Frame rate: %.2f fps\n", stats.FrameRate)
fmt.Printf("Latency: %v\n", stats.AverageLatency)
```

### VideoEncoder 接口

VideoEncoder 接口负责视频编码功能。

```go
type VideoEncoder interface {
    // 生命周期管理
    Start() error
    Stop() error
    IsRunning() bool

    // 数据处理
    PushSample(sample *Sample) error
    GetOutputChannel() <-chan *Sample

    // 配置管理
    Configure(config *config.EncodingConfig) error
    GetConfig() *config.EncodingConfig

    // 编码器管理
    GetAvailableEncoders() []string
    GetCurrentEncoder() string
    SwitchEncoder(encoderType string) error

    // 状态和统计
    GetStats() EncoderStats
    GetState() EncoderState

    // 质量控制
    SetBitrate(bitrate int) error
    SetQuality(quality string) error
    RequestKeyFrame() error
}
```

#### EncoderStats 数据结构

```go
type EncoderStats struct {
    FramesEncoded   uint64        `json:"frames_encoded"`
    FramesDropped   uint64        `json:"frames_dropped"`
    EncodeRate      float64       `json:"encode_rate"`
    AverageLatency  time.Duration `json:"average_latency"`
    CurrentBitrate  int           `json:"current_bitrate"`
    TargetBitrate   int           `json:"target_bitrate"`
    KeyFrameCount   uint64        `json:"key_frame_count"`
    EncoderType     string        `json:"encoder_type"`
    Quality         string        `json:"quality"`
    ErrorCount      uint64        `json:"error_count"`
    LastError       string        `json:"last_error,omitempty"`
}
```

#### 使用示例

```go
// 创建编码器实例
encoder := gstreamer.NewEncoderGst(&config.EncodingConfig{
    Codec:       "h264",
    Bitrate:     2000000,
    UseHardware: true,
    Preset:      "ultrafast",
    Profile:     "baseline",
})

// 启动编码器
if err := encoder.Start(); err != nil {
    return fmt.Errorf("failed to start encoder: %w", err)
}

// 推送原始视频帧
rawSample := &Sample{
    Data:      frameData,
    Size:      len(frameData),
    Timestamp: time.Now(),
    Format: MediaFormat{
        MediaType: MediaTypeVideo,
        Width:     1920,
        Height:    1080,
        FrameRate: 30,
    },
}

if err := encoder.PushSample(rawSample); err != nil {
    log.Printf("Failed to push sample: %v", err)
}

// 处理编码后的数据
outputChan := encoder.GetOutputChannel()
go func() {
    for encodedSample := range outputChan {
        // 处理编码后的视频帧
        fmt.Printf("Encoded frame: %d bytes\n", encodedSample.Size)
    }
}()
```

### MediaStream 接口

MediaStream 接口定义了流媒体数据的发布和订阅机制。

```go
type MediaStreamPublisher interface {
    AddSubscriber(subscriber MediaStreamSubscriber) error
    RemoveSubscriber(subscriber MediaStreamSubscriber) error
    PublishVideoStream(stream *EncodedVideoStream) error
    PublishAudioStream(stream *EncodedAudioStream) error
    GetSubscriberCount() int
    GetPublishStats() PublishStats
}

type MediaStreamSubscriber interface {
    OnVideoStream(stream *EncodedVideoStream) error
    OnAudioStream(stream *EncodedAudioStream) error
    OnStreamError(err error) error
    GetSubscriberID() string
}
```

#### 流媒体数据结构

```go
type EncodedVideoStream struct {
    Codec     string    `json:"codec"`
    Data      []byte    `json:"data"`
    Timestamp int64     `json:"timestamp"`
    KeyFrame  bool      `json:"key_frame"`
    Width     int       `json:"width"`
    Height    int       `json:"height"`
    Bitrate   int       `json:"bitrate"`
    FrameType FrameType `json:"frame_type"`
}

type EncodedAudioStream struct {
    Codec      string `json:"codec"`
    Data       []byte `json:"data"`
    Timestamp  int64  `json:"timestamp"`
    SampleRate int    `json:"sample_rate"`
    Channels   int    `json:"channels"`
    Bitrate    int    `json:"bitrate"`
}

type FrameType int

const (
    FrameTypeI FrameType = iota // I帧（关键帧）
    FrameTypeP                  // P帧（预测帧）
    FrameTypeB                  // B帧（双向预测帧）
)
```

#### 使用示例

```go
// 实现 MediaStreamSubscriber 接口
type WebRTCSubscriber struct {
    id         string
    videoTrack *webrtc.TrackLocalStaticSample
}

func (w *WebRTCSubscriber) OnVideoStream(stream *EncodedVideoStream) error {
    sample := media.Sample{
        Data:     stream.Data,
        Duration: time.Millisecond * 33, // 30fps
    }
    return w.videoTrack.WriteSample(sample)
}

func (w *WebRTCSubscriber) OnAudioStream(stream *EncodedAudioStream) error {
    // 处理音频流
    return nil
}

func (w *WebRTCSubscriber) OnStreamError(err error) error {
    log.Printf("Stream error: %v", err)
    return nil
}

func (w *WebRTCSubscriber) GetSubscriberID() string {
    return w.id
}

// 注册订阅者
subscriber := &WebRTCSubscriber{
    id:         "webrtc-1",
    videoTrack: videoTrack,
}

if err := manager.AddSubscriber(subscriber); err != nil {
    return fmt.Errorf("failed to add subscriber: %w", err)
}
```

## 基础设施接口

### Pipeline 接口

Pipeline 接口提供 GStreamer Pipeline 的封装。

```go
type PipelineInterface interface {
    // Pipeline 管理
    Create(name string) error
    Destroy() error

    // 元素管理
    AddElement(name, factory string) (*Element, error)
    RemoveElement(name string) error
    GetElement(name string) (*Element, error)

    // 链接管理
    LinkElements(src, sink string) error
    UnlinkElements(src, sink string) error

    // 状态管理
    SetState(state gst.State) error
    GetState() gst.State
    WaitForState(state gst.State, timeout time.Duration) error

    // 消息处理
    GetBus() *Bus

    // 调试和监控
    DumpToDot(filename string) error
    GetLatency() time.Duration
}
```

### Element 接口

Element 接口提供 GStreamer 元素的封装。

```go
type ElementInterface interface {
    // 基本属性
    GetName() string
    GetFactory() string

    // 属性管理
    SetProperty(name string, value interface{}) error
    GetProperty(name string) (interface{}, error)
    HasProperty(name string) bool

    // 状态管理
    SetState(state gst.State) error
    GetState() gst.State

    // Pad 管理
    GetStaticPad(name string) (*Pad, error)
    GetRequestPad(name string) (*Pad, error)

    // 信号连接
    Connect(signal string, callback interface{}) error
    Disconnect(signal string) error

    // 能力查询
    GetCapabilities() []string
    IsCompatibleWith(other ElementInterface) bool
}
```

### Bus 接口

Bus 接口提供消息总线功能。

```go
type BusInterface interface {
    // 消息处理
    SetMessageHandler(handler MessageHandler) error
    RemoveMessageHandler() error

    // 同步消息
    Pop() (*Message, error)
    PopFiltered(types MessageType) (*Message, error)

    // 异步消息
    AddWatch(handler MessageHandler) error
    RemoveWatch() error

    // 状态查询
    HavePending() bool
    Peek() (*Message, error)
}

type MessageHandler func(message *Message) bool

type Message struct {
    Type      MessageType `json:"type"`
    Source    string      `json:"source"`
    Timestamp time.Time   `json:"timestamp"`
    Content   interface{} `json:"content"`
}

type MessageType int

const (
    MessageTypeEOS MessageType = iota
    MessageTypeError
    MessageTypeWarning
    MessageTypeInfo
    MessageTypeStateChanged
    MessageTypeStreamStart
    MessageTypeStreamEnd
)
```

## 配置数据结构

### GStreamer 配置

```go
type GStreamerConfig struct {
    Capture  CaptureConfig  `yaml:"capture" json:"capture"`
    Encoding EncodingConfig `yaml:"encoding" json:"encoding"`
    Audio    AudioConfig    `yaml:"audio" json:"audio"`
    Debug    DebugConfig    `yaml:"debug" json:"debug"`
}

type CaptureConfig struct {
    DisplayID     string         `yaml:"display_id" json:"display_id"`
    Width         int            `yaml:"width" json:"width"`
    Height        int            `yaml:"height" json:"height"`
    FrameRate     int            `yaml:"frame_rate" json:"frame_rate"`
    UseWayland    bool           `yaml:"use_wayland" json:"use_wayland"`
    ShowPointer   bool           `yaml:"show_pointer" json:"show_pointer"`
    UseDamage     bool           `yaml:"use_damage" json:"use_damage"`
    Quality       string         `yaml:"quality" json:"quality"`
    BufferSize    int            `yaml:"buffer_size" json:"buffer_size"`
    CaptureRegion *CaptureRegion `yaml:"capture_region,omitempty" json:"capture_region,omitempty"`
}

type CaptureRegion struct {
    X      int `yaml:"x" json:"x"`
    Y      int `yaml:"y" json:"y"`
    Width  int `yaml:"width" json:"width"`
    Height int `yaml:"height" json:"height"`
}

type EncodingConfig struct {
    Type         string            `yaml:"type" json:"type"`
    Codec        string            `yaml:"codec" json:"codec"`
    Bitrate      int               `yaml:"bitrate" json:"bitrate"`
    MinBitrate   int               `yaml:"min_bitrate" json:"min_bitrate"`
    MaxBitrate   int               `yaml:"max_bitrate" json:"max_bitrate"`
    UseHardware  bool              `yaml:"use_hardware" json:"use_hardware"`
    Preset       string            `yaml:"preset" json:"preset"`
    Profile      string            `yaml:"profile" json:"profile"`
    RateControl  string            `yaml:"rate_control" json:"rate_control"`
    ZeroLatency  bool              `yaml:"zero_latency" json:"zero_latency"`
    KeyFrameInt  int               `yaml:"key_frame_interval" json:"key_frame_interval"`
    BFrames      int               `yaml:"b_frames" json:"b_frames"`
    Properties   map[string]string `yaml:"properties,omitempty" json:"properties,omitempty"`
}
```

### 日志配置

```go
type LoggingConfig struct {
    Level      string            `yaml:"level" json:"level"`
    Output     string            `yaml:"output" json:"output"`
    File       string            `yaml:"file" json:"file"`
    Colored    bool              `yaml:"colored" json:"colored"`
    Format     string            `yaml:"format" json:"format"`
    Categories map[string]string `yaml:"categories,omitempty" json:"categories,omitempty"`
    Rotation   *LogRotation      `yaml:"rotation,omitempty" json:"rotation,omitempty"`
}

type LogRotation struct {
    MaxSize  string `yaml:"max_size" json:"max_size"`
    MaxFiles int    `yaml:"max_files" json:"max_files"`
    MaxAge   string `yaml:"max_age" json:"max_age"`
}
```

## 错误处理

### 错误类型定义

```go
type GStreamerError struct {
    Component string        `json:"component"`
    Severity  ErrorSeverity `json:"severity"`
    Message   string        `json:"message"`
    Cause     error         `json:"cause,omitempty"`
    Timestamp time.Time     `json:"timestamp"`
    Context   ErrorContext  `json:"context,omitempty"`
}

type ErrorSeverity int

const (
    ErrorSeverityInfo ErrorSeverity = iota
    ErrorSeverityWarning
    ErrorSeverityError
    ErrorSeverityCritical
)

type ErrorContext map[string]interface{}

func (e *GStreamerError) Error() string {
    return fmt.Sprintf("[%s] %s: %s", e.Component, e.Severity, e.Message)
}
```

### 错误处理接口

```go
type ErrorHandler interface {
    HandleError(err *GStreamerError) error
    SetRecoveryStrategy(strategy RecoveryStrategy)
    GetErrorStats() ErrorStats
}

type RecoveryStrategy interface {
    ShouldRecover(err *GStreamerError) bool
    Recover(err *GStreamerError) error
}

type ErrorStats struct {
    TotalErrors     uint64            `json:"total_errors"`
    ErrorsByType    map[string]uint64 `json:"errors_by_type"`
    LastError       *GStreamerError   `json:"last_error,omitempty"`
    RecoveryCount   uint64            `json:"recovery_count"`
    RecoverySuccess uint64            `json:"recovery_success"`
}
```

## HTTP API

### REST 端点

#### 健康检查

```http
GET /health
```

响应:

```json
{
  "status": "healthy",
  "timestamp": "2024-01-01T12:00:00Z",
  "uptime": "1h30m45s",
  "components": {
    "gstreamer": "running",
    "webrtc": "running",
    "capture": "running",
    "encoder": "running"
  }
}
```

#### 配置管理

```http
GET /api/config
```

响应:

```json
{
  "gstreamer": {
    "capture": {
      "display_id": ":0",
      "width": 1920,
      "height": 1080,
      "frame_rate": 30
    },
    "encoding": {
      "codec": "h264",
      "bitrate": 2000000,
      "use_hardware": true
    }
  }
}
```

```http
PUT /api/config
Content-Type: application/json

{
  "gstreamer": {
    "encoding": {
      "bitrate": 3000000
    }
  }
}
```

#### 统计信息

```http
GET /api/stats
```

响应:

```json
{
  "capture": {
    "frames_total": 54000,
    "frames_dropped": 12,
    "frame_rate": 29.97,
    "average_latency": "16ms"
  },
  "encoder": {
    "frames_encoded": 53988,
    "encode_rate": 29.95,
    "current_bitrate": 2048576,
    "encoder_type": "nvh264enc"
  },
  "system": {
    "memory_usage": "256MB",
    "cpu_usage": "15.2%",
    "uptime": "2h15m30s"
  }
}
```

#### 控制操作

```http
POST /api/control/start
```

```http
POST /api/control/stop
```

```http
POST /api/control/restart
```

```http
POST /api/encoder/keyframe
```

### WebSocket API

#### 连接建立

```javascript
const ws = new WebSocket("ws://localhost:8080/ws");

ws.onopen = function (event) {
  console.log("WebSocket connected");
};

ws.onmessage = function (event) {
  const data = JSON.parse(event.data);
  handleMessage(data);
};
```

#### 消息格式

```json
{
  "type": "stats_update",
  "timestamp": "2024-01-01T12:00:00Z",
  "data": {
    "capture_fps": 30.0,
    "encoder_fps": 29.8,
    "bitrate": 2048576
  }
}
```

```json
{
  "type": "error",
  "timestamp": "2024-01-01T12:00:00Z",
  "data": {
    "component": "encoder",
    "severity": "warning",
    "message": "Encoder performance degraded"
  }
}
```

## 使用示例

### 完整应用示例

```go
package main

import (
    "context"
    "log"
    "os"
    "os/signal"
    "syscall"
    "time"

    "github.com/open-beagle/bdwind-gstreamer/internal/gstreamer"
    "github.com/open-beagle/bdwind-gstreamer/internal/config"
)

func main() {
    // 创建上下文
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    // 加载配置
    cfg, err := config.LoadConfig("config.yaml")
    if err != nil {
        log.Fatalf("Failed to load config: %v", err)
    }

    // 创建 GStreamer Manager
    manager, err := gstreamer.NewManager(ctx, &gstreamer.ManagerConfig{
        Config: cfg.GStreamer,
        Logger: log.Default(),
    })
    if err != nil {
        log.Fatalf("Failed to create GStreamer manager: %v", err)
    }

    // 启动 GStreamer
    if err := manager.Start(ctx); err != nil {
        log.Fatalf("Failed to start GStreamer: %v", err)
    }

    log.Println("GStreamer started successfully")

    // 监控统计信息
    go func() {
        ticker := time.NewTicker(5 * time.Second)
        defer ticker.Stop()

        for {
            select {
            case <-ticker.C:
                stats := manager.GetStats()
                log.Printf("Stats: %+v", stats)
            case <-ctx.Done():
                return
            }
        }
    }()

    // 等待信号
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

    <-sigChan
    log.Println("Shutting down...")

    // 优雅关闭
    shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer shutdownCancel()

    if err := manager.Stop(shutdownCtx); err != nil {
        log.Printf("Error during shutdown: %v", err)
    }

    log.Println("Shutdown completed")
}
```

这个 API 参考文档提供了 GStreamer Go-GST 重构版本的完整接口定义和使用指南，帮助开发者快速理解和使用新的架构。
