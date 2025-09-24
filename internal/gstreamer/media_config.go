package gstreamer

import (
	"fmt"
)

// MediaConfig WebRTC需要的简化媒体配置
type MediaConfig struct {
	VideoCodec   string // "h264", "vp8", "vp9"
	VideoProfile string // "baseline", "main", "high"
	Bitrate      int    // 比特率
	Resolution   string // "1920x1080"
	FrameRate    int    // 帧率
}

// NetworkCondition 网络状况结构
type NetworkCondition struct {
	PacketLoss float64 // 丢包率
	RTT        int64   // 往返时延(毫秒)
	Bandwidth  int64   // 可用带宽 (changed to int64 for consistency)
	Jitter     int64   // 抖动(毫秒)
	Congestion bool    // 网络拥塞状态
}

// MediaStreamSubscriber 媒体流订阅者接口
type MediaStreamSubscriber interface {
	OnVideoStream(stream *EncodedVideoStream) error
	OnAudioStream(stream *EncodedAudioStream) error
	OnStreamError(err error) error
	OnVideoFrame(frame *EncodedVideoFrame) error
	OnAudioFrame(frame *EncodedAudioFrame) error
	OnVideoError(err error) error
	OnAudioError(err error) error
	OnStreamStart() error
	OnStreamStop() error
	GetSubscriberID() uint64
	IsActive() bool
}

// MediaStreamProvider 媒体流提供者接口 (新增，用于WebRTC集成)
type MediaStreamProvider interface {
	// 订阅管理
	AddVideoSubscriber(subscriber VideoStreamSubscriber) (uint64, error)
	RemoveVideoSubscriber(id uint64) bool
	AddAudioSubscriber(subscriber AudioStreamSubscriber) (uint64, error)
	RemoveAudioSubscriber(id uint64) bool

	// 网络自适应
	AdaptToNetworkCondition(condition NetworkCondition) error

	// 配置生成 (为WebRTC提供媒体配置)
	GenerateMediaConfigForWebRTC() *MediaConfig
}

// EncodedVideoStream 编码后的视频流
type EncodedVideoStream struct {
	Codec     string // "h264", "vp8", "vp9"
	Data      []byte // 编码后的视频数据
	Timestamp int64  // 时间戳(毫秒)
	KeyFrame  bool   // 是否关键帧
	Width     int    // 视频宽度
	Height    int    // 视频高度
	Bitrate   int    // 当前比特率
}

// EncodedAudioStream 编码后的音频流
type EncodedAudioStream struct {
	Codec      string // "opus", "pcm"
	Data       []byte // 编码后的音频数据
	Timestamp  int64  // 时间戳(毫秒)
	SampleRate int    // 采样率
	Channels   int    // 声道数
	Bitrate    int    // 当前比特率
}

// GenerateMediaConfigForWebRTC 为WebRTC生成简化的媒体配置
func (m *Manager) GenerateMediaConfigForWebRTC() *MediaConfig {
	return &MediaConfig{
		VideoCodec:   string(m.config.Encoding.Codec),
		VideoProfile: string(m.config.Encoding.Profile),
		Bitrate:      m.config.Encoding.Bitrate,
		Resolution:   fmt.Sprintf("%dx%d", m.config.Capture.Width, m.config.Capture.Height),
		FrameRate:    m.config.Capture.FrameRate,
	}
}
