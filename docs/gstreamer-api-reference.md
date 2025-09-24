# GStreamer 新架构 API 参考文档

## 概述

本文档详细描述了重构后的 GStreamer 组件的所有外部接口和内部组件接口。新架构基于高内聚的设计原则，使用发布-订阅模式替代复杂的回调链，并严格遵循 go-gst 最佳实践。

## 核心接口

### Manager 接口

主要的 GStreamer 管理器接口，保持与现有系统的兼容性。

```go
// Manager GStreamer 主管理器接口
type Manager interface {
    // 生命周期管理
    Start(ctx context.Context) error
    Stop() error
    IsRunning() bool
    
    // 配置管理
    GetConfig() *config.GStreamerConfig
    UpdateConfig(config *config.GStreamerConfig) error
    ConfigureLogging(level string) error
    
    // 状态查询
    GetStatus() *Status
    GetStatistics() *Statistics
    
    // HTTP 处理器注册
    RegisterRoutes(router *mux.Router)
    
    // 媒体流提供者接口
    MediaStreamProvider
}
```

#### 方法详细说明

##### Start(ctx context.Context) error

启动 GStreamer 管道和所有相关组件。

**参数：**
- `ctx context.Context` - 用于控制启动过程的上下文

**返回值：**
- `error` - 启动失败时返回错误信息

**使用示例：**
```go
manager := NewGStreamerManager(config)
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

if err := manager.Start(ctx); err != nil {
    log.Fatalf("Failed to start GStreamer manager: %v", err)
}
```

##### Stop() error

优雅地停止 GStreamer 管道和所有组件。

**返回值：**
- `error` - 停止过程中的错误信息

**使用示例：**
```go
if err := manager.Stop(); err != nil {
    log.Errorf("Error during shutdown: %v", err)
}
```

##### IsRunning() bool

检查管理器是否正在运行。

**返回值：**
- `bool` - true 表示正在运行，false 表示已停止

##### GetConfig() *config.GStreamerConfig

获取当前的 GStreamer 配置。

**返回值：**
- `*config.GStreamerConfig` - 当前配置对象

##### UpdateConfig(config *config.GStreamerConfig) error

更新 GStreamer 配置。

**参数：**
- `config *config.GStreamerConfig` - 新的配置对象

**返回值：**
- `error` - 配置更新失败时的错误信息

##### ConfigureLogging(level string) error

配置日志级别。

**参数：**
- `level string` - 日志级别 ("debug", "info", "warn", "error")

**返回值：**
- `error` - 配置失败时的错误信息

### MediaStreamProvider 接口

新增的媒体流提供者接口，用于与 WebRTC 等组件集成。

```go
// MediaStreamProvider 媒体流提供者接口
type MediaStreamProvider interface {
    // 视频流订阅管理
    AddVideoSubscriber(subscriber VideoStreamSubscriber) error
    RemoveVideoSubscriber(subscriber VideoStreamSubscriber) error
    
    // 音频流订阅管理
    AddAudioSubscriber(subscriber AudioStreamSubscriber) error
    RemoveAudioSubscriber(subscriber AudioStreamSubscriber) error
    
    // 网络自适应
    AdaptToNetworkCondition(condition NetworkCondition) error
    
    // 媒体配置生成
    GenerateMediaConfigForWebRTC() *MediaConfig
    
    // 流控制
    PauseStream() error
    ResumeStream() error
    
    // 质量控制
    SetVideoQuality(quality VideoQuality) error
    SetAudioQuality(quality AudioQuality) error
}
```

#### 方法详细说明

##### AddVideoSubscriber(subscriber VideoStreamSubscriber) error

添加视频流订阅者。

**参数：**
- `subscriber VideoStreamSubscriber` - 视频流订阅者实现

**返回值：**
- `error` - 添加失败时的错误信息

**使用示例：**
```go
webrtcSubscriber := &WebRTCVideoSubscriber{
    peerConnection: pc,
    logger: logger,
}

if err := manager.AddVideoSubscriber(webrtcSubscriber); err != nil {
    log.Errorf("Failed to add video subscriber: %v", err)
}
```

##### AdaptToNetworkCondition(condition NetworkCondition) error

根据网络条件调整编码参数。

**参数：**
- `condition NetworkCondition` - 网络条件信息

**返回值：**
- `error` - 调整失败时的错误信息

**使用示例：**
```go
condition := NetworkCondition{
    Bandwidth:    1000000, // 1 Mbps
    PacketLoss:   0.01,    // 1% packet loss
    RTT:          50 * time.Millisecond,
    Jitter:       10 * time.Millisecond,
}

if err := manager.AdaptToNetworkCondition(condition); err != nil {
    log.Errorf("Failed to adapt to network condition: %v", err)
}
```

## 数据结构

### 媒体帧结构

#### EncodedVideoFrame

编码后的视频帧数据结构。

```go
// EncodedVideoFrame 编码后的视频帧
type EncodedVideoFrame struct {
    // 基本数据
    Data        []byte        `json:"data"`         // 编码后的视频数据
    Timestamp   time.Time     `json:"timestamp"`    // 时间戳
    Duration    time.Duration `json:"duration"`     // 帧持续时间
    
    // 视频属性
    Width       int           `json:"width"`        // 视频宽度
    Height      int           `json:"height"`       // 视频高度
    KeyFrame    bool          `json:"key_frame"`    // 是否关键帧
    
    // 编码信息
    Codec       string        `json:"codec"`        // 编码格式 (h264/vp8/vp9)
    Bitrate     int           `json:"bitrate"`      // 当前比特率
    Quality     int           `json:"quality"`      // 质量等级 (1-10)
    
    // 元数据
    SequenceNumber uint32     `json:"sequence_number"` // 序列号
    PTS            uint64     `json:"pts"`             // 显示时间戳
    DTS            uint64     `json:"dts"`             // 解码时间戳
}
```

#### EncodedAudioFrame

编码后的音频帧数据结构。

```go
// EncodedAudioFrame 编码后的音频帧
type EncodedAudioFrame struct {
    // 基本数据
    Data        []byte        `json:"data"`         // 编码后的音频数据
    Timestamp   time.Time     `json:"timestamp"`    // 时间戳
    Duration    time.Duration `json:"duration"`     // 音频持续时间
    
    // 音频属性
    SampleRate  int           `json:"sample_rate"`  // 采样率
    Channels    int           `json:"channels"`     // 声道数
    BitsPerSample int         `json:"bits_per_sample"` // 每样本位数
    
    // 编码信息
    Codec       string        `json:"codec"`        // 编码格式 (opus/aac/pcm)
    Bitrate     int           `json:"bitrate"`      // 当前比特率
    
    // 元数据
    SequenceNumber uint32     `json:"sequence_number"` // 序列号
    PTS            uint64     `json:"pts"`             // 显示时间戳
}
```

### 配置结构

#### GStreamerConfig

GStreamer 组件的完整配置结构。

```go
// GStreamerConfig GStreamer 配置
type GStreamerConfig struct {
    // 管道配置
    Pipeline PipelineConfig `yaml:"pipeline" json:"pipeline"`
    
    // 桌面捕获配置
    DesktopCapture DesktopCaptureConfig `yaml:"desktop_capture" json:"desktop_capture"`
    
    // 视频编码配置
    VideoEncoder VideoEncoderConfig `yaml:"video_encoder" json:"video_encoder"`
    
    // 音频处理配置
    AudioProcessor AudioProcessorConfig `yaml:"audio_processor" json:"audio_processor"`
    
    // 流发布配置
    StreamPublisher StreamPublisherConfig `yaml:"stream_publisher" json:"stream_publisher"`
    
    // 内存管理配置
    MemoryManagement MemoryManagementConfig `yaml:"memory_management" json:"memory_management"`
    
    // 调试配置
    Debug DebugConfig `yaml:"debug" json:"debug"`
}

// PipelineConfig 管道配置
type PipelineConfig struct {
    Name                 string        `yaml:"name" json:"name"`
    AutoRecovery         bool          `yaml:"auto_recovery" json:"auto_recovery"`
    MaxRestartAttempts   int           `yaml:"max_restart_attempts" json:"max_restart_attempts"`
    RestartDelay         time.Duration `yaml:"restart_delay" json:"restart_delay"`
    StateChangeTimeout   time.Duration `yaml:"state_change_timeout" json:"state_change_timeout"`
}

// DesktopCaptureConfig 桌面捕获配置
type DesktopCaptureConfig struct {
    SourceType   string `yaml:"source_type" json:"source_type"`     // auto/x11/wayland
    Framerate    int    `yaml:"framerate" json:"framerate"`         // 帧率
    Width        int    `yaml:"width" json:"width"`                 // 宽度
    Height       int    `yaml:"height" json:"height"`               // 高度
    OffsetX      int    `yaml:"offset_x" json:"offset_x"`           // X 偏移
    OffsetY      int    `yaml:"offset_y" json:"offset_y"`           // Y 偏移
    ShowCursor   bool   `yaml:"show_cursor" json:"show_cursor"`     // 显示鼠标
    UseGPU       bool   `yaml:"use_gpu" json:"use_gpu"`             // 使用 GPU 加速
}

// VideoEncoderConfig 视频编码配置
type VideoEncoderConfig struct {
    Codec              string `yaml:"codec" json:"codec"`                           // h264/vp8/vp9
    EncoderPreference  string `yaml:"encoder_preference" json:"encoder_preference"` // hardware/software/auto
    Bitrate            int    `yaml:"bitrate" json:"bitrate"`                       // 比特率
    MaxBitrate         int    `yaml:"max_bitrate" json:"max_bitrate"`               // 最大比特率
    MinBitrate         int    `yaml:"min_bitrate" json:"min_bitrate"`               // 最小比特率
    KeyframeInterval   int    `yaml:"keyframe_interval" json:"keyframe_interval"`   // 关键帧间隔
    Quality            int    `yaml:"quality" json:"quality"`                       // 质量等级 (1-10)
    Preset             string `yaml:"preset" json:"preset"`                         // 编码预设
    Profile            string `yaml:"profile" json:"profile"`                       // 编码配置文件
    Level              string `yaml:"level" json:"level"`                           // 编码级别
}
```

### 状态和统计结构

#### Status

系统状态信息。

```go
// Status 系统状态
type Status struct {
    // 基本状态
    IsRunning       bool      `json:"is_running"`
    State           string    `json:"state"`
    Uptime          time.Duration `json:"uptime"`
    LastStartTime   time.Time `json:"last_start_time"`
    
    // 组件状态
    Pipeline        ComponentStatus `json:"pipeline"`
    DesktopCapture  ComponentStatus `json:"desktop_capture"`
    VideoEncoder    ComponentStatus `json:"video_encoder"`
    AudioProcessor  ComponentStatus `json:"audio_processor"`
    StreamPublisher ComponentStatus `json:"stream_publisher"`
    
    // 错误信息
    LastError       *ErrorInfo `json:"last_error,omitempty"`
    ErrorCount      int64      `json:"error_count"`
    
    // 网络状态
    NetworkCondition *NetworkCondition `json:"network_condition,omitempty"`
}

// ComponentStatus 组件状态
type ComponentStatus struct {
    Name        string    `json:"name"`
    State       string    `json:"state"`
    IsHealthy   bool      `json:"is_healthy"`
    LastUpdate  time.Time `json:"last_update"`
    ErrorCount  int64     `json:"error_count"`
    RestartCount int64    `json:"restart_count"`
}
```

#### Statistics

性能统计信息。

```go
// Statistics 性能统计
type Statistics struct {
    // 基本统计
    Timestamp       time.Time `json:"timestamp"`
    CollectionTime  time.Duration `json:"collection_time"`
    
    // 视频统计
    Video VideoStatistics `json:"video"`
    
    // 音频统计
    Audio AudioStatistics `json:"audio"`
    
    // 内存统计
    Memory MemoryStatistics `json:"memory"`
    
    // 网络统计
    Network NetworkStatistics `json:"network"`
    
    // 错误统计
    Errors ErrorStatistics `json:"errors"`
}

// VideoStatistics 视频统计
type VideoStatistics struct {
    FrameCount      int64         `json:"frame_count"`
    FrameRate       float64       `json:"frame_rate"`
    KeyFrameCount   int64         `json:"key_frame_count"`
    DroppedFrames   int64         `json:"dropped_frames"`
    AverageLatency  time.Duration `json:"average_latency"`
    CurrentBitrate  int           `json:"current_bitrate"`
    AverageBitrate  int           `json:"average_bitrate"`
    Resolution      string        `json:"resolution"`
    Codec           string        `json:"codec"`
    EncoderType     string        `json:"encoder_type"`
}

// AudioStatistics 音频统计
type AudioStatistics struct {
    SampleCount     int64         `json:"sample_count"`
    SampleRate      int           `json:"sample_rate"`
    Channels        int           `json:"channels"`
    DroppedSamples  int64         `json:"dropped_samples"`
    AverageLatency  time.Duration `json:"average_latency"`
    CurrentBitrate  int           `json:"current_bitrate"`
    AverageBitrate  int           `json:"average_bitrate"`
    Codec           string        `json:"codec"`
}
```

## 订阅者接口

### VideoStreamSubscriber

视频流订阅者接口。

```go
// VideoStreamSubscriber 视频流订阅者接口
type VideoStreamSubscriber interface {
    // 接收视频帧
    OnVideoFrame(frame *EncodedVideoFrame) error
    
    // 处理视频错误
    OnVideoError(err error) error
    
    // 获取订阅者信息
    GetSubscriberInfo() SubscriberInfo
    
    // 网络反馈
    OnNetworkFeedback(feedback NetworkFeedback) error
    
    // 质量变化通知
    OnQualityChanged(quality VideoQuality) error
}
```

#### 方法详细说明

##### OnVideoFrame(frame *EncodedVideoFrame) error

接收编码后的视频帧。

**参数：**
- `frame *EncodedVideoFrame` - 编码后的视频帧数据

**返回值：**
- `error` - 处理失败时的错误信息

**使用示例：**
```go
type WebRTCVideoSubscriber struct {
    track  *webrtc.TrackLocalStaticSample
    logger *logrus.Entry
}

func (w *WebRTCVideoSubscriber) OnVideoFrame(frame *EncodedVideoFrame) error {
    sample := media.Sample{
        Data:     frame.Data,
        Duration: frame.Duration,
    }
    
    if err := w.track.WriteSample(sample); err != nil {
        return fmt.Errorf("failed to write video sample: %w", err)
    }
    
    w.logger.Debugf("Sent video frame: %d bytes, keyframe=%v", 
        len(frame.Data), frame.KeyFrame)
    
    return nil
}
```

### AudioStreamSubscriber

音频流订阅者接口。

```go
// AudioStreamSubscriber 音频流订阅者接口
type AudioStreamSubscriber interface {
    // 接收音频帧
    OnAudioFrame(frame *EncodedAudioFrame) error
    
    // 处理音频错误
    OnAudioError(err error) error
    
    // 获取订阅者信息
    GetSubscriberInfo() SubscriberInfo
    
    // 网络反馈
    OnNetworkFeedback(feedback NetworkFeedback) error
    
    // 质量变化通知
    OnQualityChanged(quality AudioQuality) error
}
```

## 内部组件接口

### PipelineComponent

管道组件基础接口。

```go
// PipelineComponent 管道组件接口
type PipelineComponent interface {
    // 生命周期管理
    Initialize(pipeline *gst.Pipeline) error
    Start() error
    Stop() error
    Cleanup() error
    
    // 状态管理
    GetState() ComponentState
    SetState(state ComponentState) error
    IsHealthy() bool
    
    // 配置管理
    Configure(config interface{}) error
    GetConfiguration() interface{}
    
    // 统计信息
    GetStatistics() ComponentStatistics
    ResetStatistics() error
    
    // 错误处理
    HandleError(err error) error
    GetLastError() *ErrorInfo
}
```

### StreamProcessor

流处理器接口。

```go
// StreamProcessor 流处理器接口
type StreamProcessor interface {
    // 数据处理
    ProcessFrame(frame interface{}) error
    
    // 输出回调
    SetOutputCallback(callback func(interface{}) error)
    RemoveOutputCallback()
    
    // 处理统计
    GetProcessingStats() ProcessingStatistics
    
    // 质量控制
    AdjustQuality(adjustment QualityAdjustment) error
    GetCurrentQuality() QualityInfo
}
```

### MemoryManager

内存管理器接口。

```go
// MemoryManager 内存管理器接口
type MemoryManager interface {
    // 对象生命周期管理
    RegisterObject(obj interface{}) *ObjectRef
    UnregisterObject(ref *ObjectRef) error
    
    // GC 安全管理
    KeepGCSafe(obj interface{})
    ReleaseGCSafe(obj interface{})
    
    // 缓冲区管理
    GetBufferPool(name string, size int) *BufferPool
    ReleaseBufferPool(name string) error
    
    // 内存统计
    GetMemoryStats() *MemoryStatistics
    CheckMemoryLeaks() []MemoryLeak
    
    // 清理
    ReleaseAll() error
}
```

## 错误处理

### ErrorInfo

错误信息结构。

```go
// ErrorInfo 错误信息
type ErrorInfo struct {
    Type        ErrorType `json:"type"`
    Code        string    `json:"code"`
    Message     string    `json:"message"`
    Component   string    `json:"component"`
    Timestamp   time.Time `json:"timestamp"`
    Stack       string    `json:"stack,omitempty"`
    Context     map[string]interface{} `json:"context,omitempty"`
    Recoverable bool      `json:"recoverable"`
}

// ErrorType 错误类型
type ErrorType int

const (
    ErrorTypeConfiguration ErrorType = iota
    ErrorTypeResource
    ErrorTypeNetwork
    ErrorTypeEncoding
    ErrorTypeMemory
    ErrorTypeSystem
    ErrorTypeTimeout
    ErrorTypePermission
)
```

### RecoveryStrategy

恢复策略接口。

```go
// RecoveryStrategy 恢复策略接口
type RecoveryStrategy interface {
    // 检查是否可以恢复
    CanRecover(err error) bool
    
    // 执行恢复操作
    Recover(ctx context.Context, component PipelineComponent) error
    
    // 获取恢复时间
    GetRecoveryTime() time.Duration
    
    // 获取最大重试次数
    GetMaxRetries() int
}
```

## 网络自适应

### NetworkCondition

网络条件信息。

```go
// NetworkCondition 网络条件
type NetworkCondition struct {
    Bandwidth    int64         `json:"bandwidth"`     // 带宽 (bps)
    PacketLoss   float64       `json:"packet_loss"`   // 丢包率 (0-1)
    RTT          time.Duration `json:"rtt"`           // 往返时间
    Jitter       time.Duration `json:"jitter"`        // 抖动
    Timestamp    time.Time     `json:"timestamp"`     // 测量时间
    Quality      string        `json:"quality"`       // 网络质量 (excellent/good/fair/poor)
}
```

### NetworkFeedback

网络反馈信息。

```go
// NetworkFeedback 网络反馈
type NetworkFeedback struct {
    SubscriberID     string        `json:"subscriber_id"`
    PacketsReceived  int64         `json:"packets_received"`
    PacketsLost      int64         `json:"packets_lost"`
    BytesReceived    int64         `json:"bytes_received"`
    Jitter           time.Duration `json:"jitter"`
    RTT              time.Duration `json:"rtt"`
    Timestamp        time.Time     `json:"timestamp"`
    RequestedBitrate int           `json:"requested_bitrate"`
    RequestedQuality int           `json:"requested_quality"`
}
```

## 质量控制

### VideoQuality

视频质量配置。

```go
// VideoQuality 视频质量
type VideoQuality struct {
    Width      int    `json:"width"`
    Height     int    `json:"height"`
    Framerate  int    `json:"framerate"`
    Bitrate    int    `json:"bitrate"`
    Quality    int    `json:"quality"`    // 1-10
    Preset     string `json:"preset"`     // ultrafast/fast/medium/slow
    Profile    string `json:"profile"`    // baseline/main/high
    Level      string `json:"level"`      // 3.1/4.0/4.1/5.1
}
```

### AudioQuality

音频质量配置。

```go
// AudioQuality 音频质量
type AudioQuality struct {
    SampleRate    int    `json:"sample_rate"`     // 采样率
    Channels      int    `json:"channels"`        // 声道数
    Bitrate       int    `json:"bitrate"`         // 比特率
    Quality       int    `json:"quality"`         // 1-10
    Codec         string `json:"codec"`           // opus/aac/pcm
    Application   string `json:"application"`     // voip/audio/lowdelay
}
```

## 使用示例

### 基本使用

```go
package main

import (
    "context"
    "log"
    "time"
    
    "github.com/bdwind-gstreamer/internal/gstreamer"
    "github.com/bdwind-gstreamer/internal/config"
)

func main() {
    // 创建配置
    cfg := &config.GStreamerConfig{
        Pipeline: config.PipelineConfig{
            Name:               "main-pipeline",
            AutoRecovery:       true,
            MaxRestartAttempts: 3,
            RestartDelay:       5 * time.Second,
        },
        DesktopCapture: config.DesktopCaptureConfig{
            SourceType: "auto",
            Framerate:  30,
            Width:      1920,
            Height:     1080,
        },
        VideoEncoder: config.VideoEncoderConfig{
            Codec:             "h264",
            EncoderPreference: "hardware",
            Bitrate:           2000000,
            KeyframeInterval:  30,
        },
    }
    
    // 创建管理器
    manager, err := gstreamer.NewManager(cfg)
    if err != nil {
        log.Fatalf("Failed to create manager: %v", err)
    }
    
    // 启动
    ctx := context.Background()
    if err := manager.Start(ctx); err != nil {
        log.Fatalf("Failed to start manager: %v", err)
    }
    
    // 添加视频订阅者
    subscriber := &MyVideoSubscriber{}
    if err := manager.AddVideoSubscriber(subscriber); err != nil {
        log.Fatalf("Failed to add subscriber: %v", err)
    }
    
    // 运行一段时间
    time.Sleep(60 * time.Second)
    
    // 停止
    if err := manager.Stop(); err != nil {
        log.Errorf("Error during shutdown: %v", err)
    }
}

// MyVideoSubscriber 示例视频订阅者
type MyVideoSubscriber struct{}

func (m *MyVideoSubscriber) OnVideoFrame(frame *gstreamer.EncodedVideoFrame) error {
    log.Printf("Received video frame: %d bytes, %dx%d, keyframe=%v",
        len(frame.Data), frame.Width, frame.Height, frame.KeyFrame)
    return nil
}

func (m *MyVideoSubscriber) OnVideoError(err error) error {
    log.Printf("Video error: %v", err)
    return nil
}

func (m *MyVideoSubscriber) GetSubscriberInfo() gstreamer.SubscriberInfo {
    return gstreamer.SubscriberInfo{
        ID:   "my-subscriber",
        Type: "example",
    }
}

func (m *MyVideoSubscriber) OnNetworkFeedback(feedback gstreamer.NetworkFeedback) error {
    log.Printf("Network feedback: %+v", feedback)
    return nil
}

func (m *MyVideoSubscriber) OnQualityChanged(quality gstreamer.VideoQuality) error {
    log.Printf("Quality changed: %+v", quality)
    return nil
}
```

### WebRTC 集成示例

```go
// WebRTC 集成示例
func setupWebRTCIntegration(manager gstreamer.Manager, pc *webrtc.PeerConnection) error {
    // 创建视频轨道
    videoTrack, err := webrtc.NewTrackLocalStaticSample(
        webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264},
        "video",
        "gstreamer-video",
    )
    if err != nil {
        return fmt.Errorf("failed to create video track: %w", err)
    }
    
    // 添加轨道到 PeerConnection
    if _, err := pc.AddTrack(videoTrack); err != nil {
        return fmt.Errorf("failed to add video track: %w", err)
    }
    
    // 创建 WebRTC 视频订阅者
    videoSubscriber := &WebRTCVideoSubscriber{
        track:  videoTrack,
        logger: logrus.WithField("component", "webrtc-video"),
    }
    
    // 添加到 GStreamer 管理器
    if err := manager.AddVideoSubscriber(videoSubscriber); err != nil {
        return fmt.Errorf("failed to add video subscriber: %w", err)
    }
    
    // 创建音频轨道和订阅者（类似处理）
    // ...
    
    return nil
}
```

## 总结

本 API 参考文档提供了重构后 GStreamer 组件的完整接口说明，包括：

1. **核心接口** - Manager 和 MediaStreamProvider 的详细方法说明
2. **数据结构** - 媒体帧、配置、状态等关键数据结构
3. **订阅者接口** - 视频和音频流订阅者的实现规范
4. **内部组件** - 管道组件、流处理器等内部接口
5. **错误处理** - 完整的错误信息和恢复策略
6. **网络自适应** - 网络条件和反馈机制
7. **质量控制** - 视频和音频质量配置
8. **使用示例** - 实际的集成和使用代码示例

这些接口设计确保了新架构的高内聚性、可扩展性和与现有系统的兼容性。