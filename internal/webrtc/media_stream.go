package webrtc

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
	"github.com/sirupsen/logrus"

	"github.com/open-beagle/bdwind-gstreamer/internal/config"
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

	// HealthCheck 执行媒体流健康检查
	HealthCheck() error
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
	logger     *logrus.Entry // 使用 logrus entry 来实现日志管理
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
func NewMediaStream(cfg *MediaStreamConfig) (MediaStream, error) {
	if cfg == nil {
		cfg = DefaultMediaStreamConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())
	logger := config.GetLoggerWithPrefix("webrtc-media-stream")

	ms := &mediaStreamImpl{
		config:         cfg,
		logger:         logger,
		trackInfo:      make(map[string]*TrackInfo),
		trackStates:    make(map[string]TrackState),
		monitoringStop: make(chan struct{}),
		ctx:            ctx,
		cancel:         cancel,
	}

	// 创建视频轨道
	if cfg.VideoEnabled {
		videoTrack, err := webrtc.NewTrackLocalStaticSample(
			webrtc.RTPCodecCapability{MimeType: getVideoMimeType(cfg.VideoCodec)},
			cfg.VideoTrackID,
			"video-stream",
		)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("failed to create video track: %w", err)
		}
		ms.videoTrack = videoTrack

		// 注册轨道信息
		ms.trackInfo[cfg.VideoTrackID] = &TrackInfo{
			ID:        cfg.VideoTrackID,
			Kind:      webrtc.RTPCodecTypeVideo,
			MimeType:  getVideoMimeType(cfg.VideoCodec),
			State:     TrackStateActive,
			CreatedAt: time.Now(),
		}
		ms.trackStates[cfg.VideoTrackID] = TrackStateActive
		ms.stats.TotalTracks++
		ms.stats.ActiveTracks++
	}

	// 创建音频轨道
	if cfg.AudioEnabled {
		audioTrack, err := webrtc.NewTrackLocalStaticSample(
			webrtc.RTPCodecCapability{MimeType: getAudioMimeType(cfg.AudioCodec)},
			cfg.AudioTrackID,
			"audio-stream",
		)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("failed to create audio track: %w", err)
		}
		ms.audioTrack = audioTrack

		// 注册轨道信息
		ms.trackInfo[cfg.AudioTrackID] = &TrackInfo{
			ID:        cfg.AudioTrackID,
			Kind:      webrtc.RTPCodecTypeAudio,
			MimeType:  getAudioMimeType(cfg.AudioCodec),
			State:     TrackStateActive,
			CreatedAt: time.Now(),
		}
		ms.trackStates[cfg.AudioTrackID] = TrackStateActive
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
		// Warn级别: 样本处理异常 - 视频轨道不可用
		ms.logger.Warn("Video sample write failed: video track not available")
		return fmt.Errorf("video track not available")
	}

	// Trace级别: 记录每个样本的详细处理信息
	ms.logger.Tracef("Writing video sample to track: size=%d bytes, track_id=%s, timestamp=%v",
		len(sample.Data), track.ID(), sample.Timestamp)

	err := track.WriteSample(sample)
	if err != nil {
		// Warn级别: 样本处理异常
		ms.logger.Warnf("Failed to write video sample to track %s: %v", track.ID(), err)
		return fmt.Errorf("failed to write video sample: %w", err)
	}

	// Trace级别: 记录成功写入
	ms.logger.Trace("Video sample written to track successfully")

	// 更新统计信息
	ms.mutex.Lock()
	ms.stats.VideoFramesSent++
	ms.stats.VideoBytesSent += int64(len(sample.Data))
	ms.stats.LastVideoTimestamp = time.Now()
	framesSent := ms.stats.VideoFramesSent

	// 更新轨道信息
	if trackInfo, exists := ms.trackInfo[track.ID()]; exists {
		trackInfo.FramesSent++
		trackInfo.BytesSent += int64(len(sample.Data))
		trackInfo.LastActive = time.Now()
	}
	ms.mutex.Unlock()

	// Debug级别: 记录样本处理统计（每 1000 个样本）
	if framesSent%1000 == 0 {
		ms.logger.Debugf("Video sample processing milestone: %d frames sent to track %s",
			framesSent, track.ID())
	}

	return nil
}

// WriteAudio 写入音频样本
func (ms *mediaStreamImpl) WriteAudio(sample media.Sample) error {
	ms.mutex.RLock()
	track := ms.audioTrack
	ms.mutex.RUnlock()

	if track == nil {
		// Warn级别: 样本处理异常 - 音频轨道不可用
		ms.logger.Warn("Audio sample write failed: audio track not available")
		return fmt.Errorf("audio track not available")
	}

	// Trace级别: 记录每个样本的详细处理信息
	ms.logger.Tracef("Writing audio sample to track: size=%d bytes, track_id=%s, timestamp=%v",
		len(sample.Data), track.ID(), sample.Timestamp)

	err := track.WriteSample(sample)
	if err != nil {
		// Warn级别: 样本处理异常
		ms.logger.Warnf("Failed to write audio sample to track %s: %v", track.ID(), err)
		return fmt.Errorf("failed to write audio sample: %w", err)
	}

	// Trace级别: 记录成功写入
	ms.logger.Trace("Audio sample written to track successfully")

	// 更新统计信息
	ms.mutex.Lock()
	ms.stats.AudioFramesSent++
	ms.stats.AudioBytesSent += int64(len(sample.Data))
	ms.stats.LastAudioTimestamp = time.Now()
	framesSent := ms.stats.AudioFramesSent

	// 更新轨道信息
	if trackInfo, exists := ms.trackInfo[track.ID()]; exists {
		trackInfo.FramesSent++
		trackInfo.BytesSent += int64(len(sample.Data))
		trackInfo.LastActive = time.Now()
	}
	ms.mutex.Unlock()

	// Debug级别: 记录样本处理统计（每 1000 个样本）
	if framesSent%1000 == 0 {
		ms.logger.Debugf("Audio sample processing milestone: %d frames sent to track %s",
			framesSent, track.ID())
	}

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

// HealthCheck 执行媒体流健康检查
func (ms *mediaStreamImpl) HealthCheck() error {
	ms.mutex.RLock()
	defer ms.mutex.RUnlock()

	// Trace级别: 记录详细的健康检查步骤和指标
	ms.logger.Trace("Starting MediaStream health check")

	var issues []string
	now := time.Now()
	inactivityThreshold := time.Minute * 2 // 2分钟无活动认为异常

	// Trace级别: 记录检查步骤
	ms.logger.Tracef("Health check configuration: video_enabled=%v, audio_enabled=%v, inactivity_threshold=%v",
		ms.config.VideoEnabled, ms.config.AudioEnabled, inactivityThreshold)

	// 检查轨道状态
	if ms.config.VideoEnabled {
		// Trace级别: 记录视频轨道检查步骤
		ms.logger.Trace("Checking video track health")

		if ms.videoTrack == nil {
			issues = append(issues, "video enabled but track is nil")
			// Debug级别: 记录轨道状态检查结果
			ms.logger.Debug("Video track health check failed: track is nil")
		} else {
			// 检查视频轨道配置一致性
			expectedMimeType := getVideoMimeType(ms.config.VideoCodec)
			actualMimeType := ms.videoTrack.Codec().MimeType
			if actualMimeType != expectedMimeType {
				issues = append(issues, fmt.Sprintf("video track MIME type mismatch: expected %s, got %s",
					expectedMimeType, actualMimeType))
				// Debug级别: 记录配置一致性验证结果
				ms.logger.Debugf("Video track MIME type mismatch: expected=%s, actual=%s",
					expectedMimeType, actualMimeType)
			}

			// 检查视频轨道活动状态
			if trackInfo, exists := ms.trackInfo[ms.videoTrack.ID()]; exists {
				if trackInfo.State != TrackStateActive {
					issues = append(issues, fmt.Sprintf("video track is not active: state=%s", trackInfo.State.String()))
					// Debug级别: 记录轨道状态检查结果
					ms.logger.Debugf("Video track state check failed: state=%s", trackInfo.State.String())
				}

				// 检查最近活动时间
				if now.Sub(trackInfo.LastActive) > inactivityThreshold {
					issues = append(issues, fmt.Sprintf("video track inactive for %v", now.Sub(trackInfo.LastActive)))
					// Debug级别: 记录轨道状态检查结果
					ms.logger.Debugf("Video track inactivity detected: last_active=%v, inactive_duration=%v",
						trackInfo.LastActive, now.Sub(trackInfo.LastActive))
				}

				// Trace级别: 记录视频轨道详细指标
				ms.logger.Tracef("Video track metrics: frames_sent=%d, bytes_sent=%d, last_active=%v",
					trackInfo.FramesSent, trackInfo.BytesSent, trackInfo.LastActive)
			} else {
				issues = append(issues, "video track info not found")
				// Debug级别: 记录轨道状态检查结果
				ms.logger.Debug("Video track info not found in track registry")
			}
		}
	}

	if ms.config.AudioEnabled {
		// Trace级别: 记录音频轨道检查步骤
		ms.logger.Trace("Checking audio track health")

		if ms.audioTrack == nil {
			issues = append(issues, "audio enabled but track is nil")
			// Debug级别: 记录轨道状态检查结果
			ms.logger.Debug("Audio track health check failed: track is nil")
		} else {
			// 检查音频轨道配置一致性
			expectedMimeType := getAudioMimeType(ms.config.AudioCodec)
			actualMimeType := ms.audioTrack.Codec().MimeType
			if actualMimeType != expectedMimeType {
				issues = append(issues, fmt.Sprintf("audio track MIME type mismatch: expected %s, got %s",
					expectedMimeType, actualMimeType))
				// Debug级别: 记录配置一致性验证结果
				ms.logger.Debugf("Audio track MIME type mismatch: expected=%s, actual=%s",
					expectedMimeType, actualMimeType)
			}

			// 检查音频轨道活动状态
			if trackInfo, exists := ms.trackInfo[ms.audioTrack.ID()]; exists {
				if trackInfo.State != TrackStateActive {
					issues = append(issues, fmt.Sprintf("audio track is not active: state=%s", trackInfo.State.String()))
					// Debug级别: 记录轨道状态检查结果
					ms.logger.Debugf("Audio track state check failed: state=%s", trackInfo.State.String())
				}

				// 检查最近活动时间
				if now.Sub(trackInfo.LastActive) > inactivityThreshold {
					issues = append(issues, fmt.Sprintf("audio track inactive for %v", now.Sub(trackInfo.LastActive)))
					// Debug级别: 记录轨道状态检查结果
					ms.logger.Debugf("Audio track inactivity detected: last_active=%v, inactive_duration=%v",
						trackInfo.LastActive, now.Sub(trackInfo.LastActive))
				}

				// Trace级别: 记录音频轨道详细指标
				ms.logger.Tracef("Audio track metrics: frames_sent=%d, bytes_sent=%d, last_active=%v",
					trackInfo.FramesSent, trackInfo.BytesSent, trackInfo.LastActive)
			} else {
				issues = append(issues, "audio track info not found")
				// Debug级别: 记录轨道状态检查结果
				ms.logger.Debug("Audio track info not found in track registry")
			}
		}
	}

	// 检查统计信息一致性
	stats := ms.stats
	expectedActiveTracks := 0
	for _, state := range ms.trackStates {
		if state == TrackStateActive {
			expectedActiveTracks++
		}
	}

	if stats.ActiveTracks != expectedActiveTracks {
		issues = append(issues, fmt.Sprintf("active track count mismatch: stats=%d, actual=%d",
			stats.ActiveTracks, expectedActiveTracks))
		// Debug级别: 记录配置一致性验证结果
		ms.logger.Debugf("Active track count mismatch: stats_count=%d, calculated_count=%d",
			stats.ActiveTracks, expectedActiveTracks)
	}

	// Trace级别: 记录详细的健康检查指标
	ms.logger.Tracef("Health check statistics: total_tracks=%d, active_tracks=%d, video_frames=%d, audio_frames=%d",
		stats.TotalTracks, stats.ActiveTracks, stats.VideoFramesSent, stats.AudioFramesSent)

	// 生成健康检查结果
	if len(issues) > 0 {
		// Info级别: 记录健康检查结果摘要（失败）
		ms.logger.Infof("MediaStream health check failed with %d issues", len(issues))
		// Debug级别: 记录详细的问题列表
		for i, issue := range issues {
			ms.logger.Debugf("Health check issue %d: %s", i+1, issue)
		}
		return fmt.Errorf("MediaStream health check failed: %v", issues)
	}

	// Info级别: 记录健康检查结果摘要（通过）
	ms.logger.Info("MediaStream health check passed successfully")
	// Debug级别: 记录健康检查通过的详细信息
	ms.logger.Debugf("Health check passed: %d tracks active, video_enabled=%v, audio_enabled=%v",
		expectedActiveTracks, ms.config.VideoEnabled, ms.config.AudioEnabled)

	return nil
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
