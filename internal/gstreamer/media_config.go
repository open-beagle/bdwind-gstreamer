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
	Bandwidth  int     // 可用带宽
	Jitter     int64   // 抖动(毫秒)
}

// MediaStreamSubscriber 媒体流订阅者接口
type MediaStreamSubscriber interface {
	OnVideoStream(stream *EncodedVideoStream) error
	OnAudioStream(stream *EncodedAudioStream) error
	OnStreamError(err error) error
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

// AddSubscriber 添加媒体流订阅者
func (m *Manager) AddSubscriber(subscriber MediaStreamSubscriber) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// 初始化subscribers切片如果还没有
	if m.subscribers == nil {
		m.subscribers = make([]MediaStreamSubscriber, 0)
	}

	m.subscribers = append(m.subscribers, subscriber)
	m.logger.Infof("Added subscriber, total subscribers: %d", len(m.subscribers))
	return nil
}

// PublishVideoStream 发布视频流给所有订阅者
func (m *Manager) PublishVideoStream(stream *EncodedVideoStream) error {
	m.mutex.RLock()
	subscribers := make([]MediaStreamSubscriber, len(m.subscribers))
	copy(subscribers, m.subscribers)
	m.mutex.RUnlock()

	// 发布给所有订阅者
	for _, subscriber := range subscribers {
		if err := subscriber.OnVideoStream(stream); err != nil {
			m.logger.Warnf("Failed to publish video stream to subscriber: %v", err)
			// 通知订阅者错误
			subscriber.OnStreamError(err)
		}
	}
	return nil
}

// PublishAudioStream 发布音频流给所有订阅者
func (m *Manager) PublishAudioStream(stream *EncodedAudioStream) error {
	m.mutex.RLock()
	subscribers := make([]MediaStreamSubscriber, len(m.subscribers))
	copy(subscribers, m.subscribers)
	m.mutex.RUnlock()

	// 发布给所有订阅者
	for _, subscriber := range subscribers {
		if err := subscriber.OnAudioStream(stream); err != nil {
			m.logger.Warnf("Failed to publish audio stream to subscriber: %v", err)
			// 通知订阅者错误
			subscriber.OnStreamError(err)
		}
	}
	return nil
}
