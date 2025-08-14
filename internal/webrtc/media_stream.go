package webrtc

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
)

// MediaStreamConfig 媒体流配置结构
type MediaStreamConfig struct {
	VideoEnabled bool   `yaml:"video_enabled"`
	AudioEnabled bool   `yaml:"audio_enabled"`
	VideoTrackID string `yaml:"video_track_id"`
	AudioTrackID string `yaml:"audio_track_id"`

	// 视频配置
	VideoCodec     string `yaml:"video_codec"` // h264, vp8, vp9
	VideoWidth     int    `yaml:"video_width"`
	VideoHeight    int    `yaml:"video_height"`
	VideoFrameRate int    `yaml:"video_framerate"`
	VideoBitrate   int    `yaml:"video_bitrate"`

	// 音频配置
	AudioCodec      string `yaml:"audio_codec"` // opus, pcmu, pcma
	AudioChannels   int    `yaml:"audio_channels"`
	AudioSampleRate int    `yaml:"audio_sample_rate"`
	AudioBitrate    int    `yaml:"audio_bitrate"`
}

// DefaultMediaStreamConfig 返回默认媒体流配置
func DefaultMediaStreamConfig() *MediaStreamConfig {
	return &MediaStreamConfig{
		VideoEnabled:    true,
		AudioEnabled:    false,
		VideoTrackID:    "video",
		AudioTrackID:    "audio",
		VideoCodec:      "h264",
		VideoWidth:      1920,
		VideoHeight:     1080,
		VideoFrameRate:  30,
		VideoBitrate:    2000,
		AudioCodec:      "opus",
		AudioChannels:   2,
		AudioSampleRate: 48000,
		AudioBitrate:    128,
	}
}

// MediaStream WebRTC媒体流接口
type MediaStream interface {
	// AddTrack 添加媒体轨道
	AddTrack(track *webrtc.TrackLocalStaticSample) error

	// RemoveTrack 移除媒体轨道
	RemoveTrack(trackID string) error

	// WriteVideo 写入视频样本
	WriteVideo(sample media.Sample) error

	// WriteAudio 写入音频样本
	WriteAudio(sample media.Sample) error

	// GetVideoTrack 获取视频轨道
	GetVideoTrack() *webrtc.TrackLocalStaticSample

	// GetAudioTrack 获取音频轨道
	GetAudioTrack() *webrtc.TrackLocalStaticSample

	// GetTrackState 获取轨道状态
	GetTrackState(trackID string) (TrackState, error)

	// GetAllTracks 获取所有轨道信息
	GetAllTracks() []TrackInfo

	// StartTrackMonitoring 开始轨道监控
	StartTrackMonitoring() error

	// StopTrackMonitoring 停止轨道监控
	StopTrackMonitoring() error

	// Close 关闭媒体流
	Close() error

	// GetStats 获取媒体流统计信息
	GetStats() MediaStreamStats
}

// TrackState 轨道状态
type TrackState int

const (
	TrackStateUnknown TrackState = iota
	TrackStateActive
	TrackStateInactive
	TrackStateMuted
	TrackStateEnded
)

// String returns the string representation of TrackState
func (ts TrackState) String() string {
	switch ts {
	case TrackStateActive:
		return "active"
	case TrackStateInactive:
		return "inactive"
	case TrackStateMuted:
		return "muted"
	case TrackStateEnded:
		return "ended"
	default:
		return "unknown"
	}
}

// TrackInfo 轨道信息
type TrackInfo struct {
	ID          string
	Kind        webrtc.RTPCodecType
	MimeType    string
	State       TrackState
	CreatedAt   time.Time
	LastActive  time.Time
	FramesSent  int64
	BytesSent   int64
	PacketsSent int64
}

// MediaStreamStats 媒体流统计信息
type MediaStreamStats struct {
	VideoFramesSent    int64
	VideoBytesSent     int64
	VideoPacketsSent   int64
	VideoPacketsLost   int64
	AudioFramesSent    int64
	AudioBytesSent     int64
	AudioPacketsSent   int64
	AudioPacketsLost   int64
	LastVideoTimestamp time.Time
	LastAudioTimestamp time.Time
	ActiveTracks       int
	TotalTracks        int
}

// mediaStreamImpl WebRTC媒体流实现
type mediaStreamImpl struct {
	config     *MediaStreamConfig
	videoTrack *webrtc.TrackLocalStaticSample
	audioTrack *webrtc.TrackLocalStaticSample

	// 轨道信息和状态
	trackInfo      map[string]*TrackInfo
	trackStates    map[string]TrackState
	monitoringWg   sync.WaitGroup
	monitoringStop chan struct{}
	monitoring     bool

	// 统计信息
	stats MediaStreamStats
	mutex sync.RWMutex

	// 上下文和取消函数
	ctx    context.Context
	cancel context.CancelFunc
}

// NewMediaStream 创建新的媒体流
func NewMediaStream(config *MediaStreamConfig) (MediaStream, error) {
	if config == nil {
		config = DefaultMediaStreamConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	ms := &mediaStreamImpl{
		config:         config,
		trackInfo:      make(map[string]*TrackInfo),
		trackStates:    make(map[string]TrackState),
		monitoringStop: make(chan struct{}),
		ctx:            ctx,
		cancel:         cancel,
	}

	// 创建视频轨道
	if config.VideoEnabled {
		videoTrack, err := webrtc.NewTrackLocalStaticSample(
			webrtc.RTPCodecCapability{MimeType: getVideoMimeType(config.VideoCodec)},
			config.VideoTrackID,
			"video-stream",
		)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("failed to create video track: %w", err)
		}
		ms.videoTrack = videoTrack

		// 注册轨道信息
		ms.trackInfo[config.VideoTrackID] = &TrackInfo{
			ID:        config.VideoTrackID,
			Kind:      webrtc.RTPCodecTypeVideo,
			MimeType:  getVideoMimeType(config.VideoCodec),
			State:     TrackStateActive,
			CreatedAt: time.Now(),
		}
		ms.trackStates[config.VideoTrackID] = TrackStateActive
		ms.stats.TotalTracks++
		ms.stats.ActiveTracks++
	}

	// 创建音频轨道
	if config.AudioEnabled {
		audioTrack, err := webrtc.NewTrackLocalStaticSample(
			webrtc.RTPCodecCapability{MimeType: getAudioMimeType(config.AudioCodec)},
			config.AudioTrackID,
			"audio-stream",
		)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("failed to create audio track: %w", err)
		}
		ms.audioTrack = audioTrack

		// 注册轨道信息
		ms.trackInfo[config.AudioTrackID] = &TrackInfo{
			ID:        config.AudioTrackID,
			Kind:      webrtc.RTPCodecTypeAudio,
			MimeType:  getAudioMimeType(config.AudioCodec),
			State:     TrackStateActive,
			CreatedAt: time.Now(),
		}
		ms.trackStates[config.AudioTrackID] = TrackStateActive
		ms.stats.TotalTracks++
		ms.stats.ActiveTracks++
	}

	return ms, nil
}

// AddTrack 添加媒体轨道
func (ms *mediaStreamImpl) AddTrack(track *webrtc.TrackLocalStaticSample) error {
	ms.mutex.Lock()
	defer ms.mutex.Unlock()

	trackID := track.ID()

	// 根据轨道类型添加
	if track.Kind() == webrtc.RTPCodecTypeVideo {
		ms.videoTrack = track
	} else if track.Kind() == webrtc.RTPCodecTypeAudio {
		ms.audioTrack = track
	} else {
		return fmt.Errorf("unsupported track type: %v", track.Kind())
	}

	// 注册轨道信息
	ms.trackInfo[trackID] = &TrackInfo{
		ID:        trackID,
		Kind:      track.Kind(),
		MimeType:  track.Codec().MimeType,
		State:     TrackStateActive,
		CreatedAt: time.Now(),
	}
	ms.trackStates[trackID] = TrackStateActive
	ms.stats.TotalTracks++
	ms.stats.ActiveTracks++

	return nil
}

// RemoveTrack 移除媒体轨道
func (ms *mediaStreamImpl) RemoveTrack(trackID string) error {
	ms.mutex.Lock()
	defer ms.mutex.Unlock()

	// 检查轨道是否存在
	trackInfo, exists := ms.trackInfo[trackID]
	if !exists {
		return fmt.Errorf("track not found: %s", trackID)
	}

	// 移除轨道引用
	if ms.videoTrack != nil && ms.videoTrack.ID() == trackID {
		ms.videoTrack = nil
	}

	if ms.audioTrack != nil && ms.audioTrack.ID() == trackID {
		ms.audioTrack = nil
	}

	// 更新轨道状态
	if trackInfo.State == TrackStateActive {
		ms.stats.ActiveTracks--
	}
	trackInfo.State = TrackStateEnded
	ms.trackStates[trackID] = TrackStateEnded

	// 清理轨道信息
	delete(ms.trackInfo, trackID)
	delete(ms.trackStates, trackID)

	return nil
}

// WriteVideo 写入视频样本
func (ms *mediaStreamImpl) WriteVideo(sample media.Sample) error {
	ms.mutex.RLock()
	track := ms.videoTrack
	ms.mutex.RUnlock()

	if track == nil {
		fmt.Printf("MediaStream: Video track not available\n")
		return fmt.Errorf("video track not available")
	}

	fmt.Printf("MediaStream: Writing video sample to track - size=%d bytes, track_id=%s\n",
		len(sample.Data), track.ID())

	err := track.WriteSample(sample)
	if err != nil {
		fmt.Printf("MediaStream: Error writing sample to track: %v\n", err)
		return fmt.Errorf("failed to write video sample: %w", err)
	}

	fmt.Printf("MediaStream: Video sample written to track successfully\n")

	// 更新统计信息
	ms.mutex.Lock()
	ms.stats.VideoFramesSent++
	ms.stats.VideoBytesSent += int64(len(sample.Data))
	ms.stats.LastVideoTimestamp = time.Now()

	// 更新轨道信息
	if trackInfo, exists := ms.trackInfo[track.ID()]; exists {
		trackInfo.FramesSent++
		trackInfo.BytesSent += int64(len(sample.Data))
		trackInfo.LastActive = time.Now()
	}
	ms.mutex.Unlock()

	return nil
}

// WriteAudio 写入音频样本
func (ms *mediaStreamImpl) WriteAudio(sample media.Sample) error {
	ms.mutex.RLock()
	track := ms.audioTrack
	ms.mutex.RUnlock()

	if track == nil {
		return fmt.Errorf("audio track not available")
	}

	err := track.WriteSample(sample)
	if err != nil {
		return fmt.Errorf("failed to write audio sample: %w", err)
	}

	// 更新统计信息
	ms.mutex.Lock()
	ms.stats.AudioFramesSent++
	ms.stats.AudioBytesSent += int64(len(sample.Data))
	ms.stats.LastAudioTimestamp = time.Now()

	// 更新轨道信息
	if trackInfo, exists := ms.trackInfo[track.ID()]; exists {
		trackInfo.FramesSent++
		trackInfo.BytesSent += int64(len(sample.Data))
		trackInfo.LastActive = time.Now()
	}
	ms.mutex.Unlock()

	return nil
}

// GetVideoTrack 获取视频轨道
func (ms *mediaStreamImpl) GetVideoTrack() *webrtc.TrackLocalStaticSample {
	ms.mutex.RLock()
	defer ms.mutex.RUnlock()
	return ms.videoTrack
}

// GetAudioTrack 获取音频轨道
func (ms *mediaStreamImpl) GetAudioTrack() *webrtc.TrackLocalStaticSample {
	ms.mutex.RLock()
	defer ms.mutex.RUnlock()
	return ms.audioTrack
}

// GetTrackState 获取轨道状态
func (ms *mediaStreamImpl) GetTrackState(trackID string) (TrackState, error) {
	ms.mutex.RLock()
	defer ms.mutex.RUnlock()

	state, exists := ms.trackStates[trackID]
	if !exists {
		return TrackStateUnknown, fmt.Errorf("track not found: %s", trackID)
	}

	return state, nil
}

// GetAllTracks 获取所有轨道信息
func (ms *mediaStreamImpl) GetAllTracks() []TrackInfo {
	ms.mutex.RLock()
	defer ms.mutex.RUnlock()

	tracks := make([]TrackInfo, 0, len(ms.trackInfo))
	for _, trackInfo := range ms.trackInfo {
		tracks = append(tracks, *trackInfo)
	}

	return tracks
}

// StartTrackMonitoring 开始轨道监控
func (ms *mediaStreamImpl) StartTrackMonitoring() error {
	ms.mutex.Lock()
	defer ms.mutex.Unlock()

	if ms.monitoring {
		return fmt.Errorf("track monitoring already started")
	}

	ms.monitoring = true
	ms.monitoringWg.Add(1)
	go ms.monitorTracks()

	return nil
}

// StopTrackMonitoring 停止轨道监控
func (ms *mediaStreamImpl) StopTrackMonitoring() error {
	ms.mutex.Lock()
	if !ms.monitoring {
		ms.mutex.Unlock()
		return nil
	}
	ms.monitoring = false
	ms.mutex.Unlock()

	close(ms.monitoringStop)
	ms.monitoringWg.Wait()

	// 重新创建停止通道以便下次使用
	ms.mutex.Lock()
	ms.monitoringStop = make(chan struct{})
	ms.mutex.Unlock()

	return nil
}

// monitorTracks 监控轨道状态的协程
func (ms *mediaStreamImpl) monitorTracks() {
	defer ms.monitoringWg.Done()

	ticker := time.NewTicker(time.Second * 5) // 每5秒检查一次
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ms.checkTrackStates()
		case <-ms.monitoringStop:
			return
		case <-ms.ctx.Done():
			return
		}
	}
}

// checkTrackStates 检查轨道状态
func (ms *mediaStreamImpl) checkTrackStates() {
	ms.mutex.Lock()
	defer ms.mutex.Unlock()

	now := time.Now()
	inactiveThreshold := time.Second * 10 // 10秒无活动认为不活跃

	for trackID, trackInfo := range ms.trackInfo {
		if trackInfo.State == TrackStateActive {
			// 检查是否长时间无活动
			if now.Sub(trackInfo.LastActive) > inactiveThreshold {
				trackInfo.State = TrackStateInactive
				ms.trackStates[trackID] = TrackStateInactive
				ms.stats.ActiveTracks--
			}
		} else if trackInfo.State == TrackStateInactive {
			// 检查是否恢复活动
			if now.Sub(trackInfo.LastActive) <= inactiveThreshold {
				trackInfo.State = TrackStateActive
				ms.trackStates[trackID] = TrackStateActive
				ms.stats.ActiveTracks++
			}
		}
	}
}

// Close 关闭媒体流
func (ms *mediaStreamImpl) Close() error {
	// 停止监控
	ms.StopTrackMonitoring()

	ms.cancel()

	ms.mutex.Lock()
	defer ms.mutex.Unlock()

	// 清理轨道
	for trackID, trackInfo := range ms.trackInfo {
		trackInfo.State = TrackStateEnded
		ms.trackStates[trackID] = TrackStateEnded
	}

	ms.videoTrack = nil
	ms.audioTrack = nil
	ms.stats.ActiveTracks = 0

	return nil
}

// GetStats 获取媒体流统计信息
func (ms *mediaStreamImpl) GetStats() MediaStreamStats {
	ms.mutex.RLock()
	defer ms.mutex.RUnlock()

	// 更新活跃轨道数量
	stats := ms.stats
	stats.ActiveTracks = 0
	stats.TotalTracks = len(ms.trackInfo)

	for _, state := range ms.trackStates {
		if state == TrackStateActive {
			stats.ActiveTracks++
		}
	}

	return stats
}

// getVideoMimeType 根据编码器类型获取视频MIME类型
func getVideoMimeType(codec string) string {
	switch codec {
	case "h264":
		return webrtc.MimeTypeH264
	case "vp8":
		return webrtc.MimeTypeVP8
	case "vp9":
		return webrtc.MimeTypeVP9
	default:
		return webrtc.MimeTypeH264
	}
}

// getAudioMimeType 根据编码器类型获取音频MIME类型
func getAudioMimeType(codec string) string {
	switch codec {
	case "opus":
		return webrtc.MimeTypeOpus
	case "pcmu":
		return webrtc.MimeTypePCMU
	case "pcma":
		return webrtc.MimeTypePCMA
	default:
		return webrtc.MimeTypeOpus
	}
}
