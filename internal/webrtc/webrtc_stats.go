package webrtc

import (
	"context"
	"sync"
	"time"

	"github.com/pion/webrtc/v4"
)

// WebRTCStatsConfig WebRTC统计配置
type WebRTCStatsConfig struct {
	// 统计收集间隔
	CollectionInterval time.Duration `yaml:"collection_interval"`

	// 历史记录保留数量
	MaxHistorySize int `yaml:"max_history_size"`

	// 是否启用详细统计
	DetailedStats bool `yaml:"detailed_stats"`

	// 统计类型过滤
	EnabledStats []string `yaml:"enabled_stats"`
}

// DefaultWebRTCStatsConfig 返回默认WebRTC统计配置
func DefaultWebRTCStatsConfig() *WebRTCStatsConfig {
	return &WebRTCStatsConfig{
		CollectionInterval: time.Second * 2,
		MaxHistorySize:     100,
		DetailedStats:      true,
		EnabledStats: []string{
			"connection",
			"transport",
			"inbound-rtp",
			"outbound-rtp",
			"remote-inbound-rtp",
			"remote-outbound-rtp",
		},
	}
}

// WebRTCStatsCollector WebRTC统计收集器接口
type WebRTCStatsCollector interface {
	// Start 启动统计收集
	Start() error

	// Stop 停止统计收集
	Stop() error

	// SetPeerConnection 设置WebRTC连接
	SetPeerConnection(pc *webrtc.PeerConnection) error

	// GetCurrentStats 获取当前统计信息
	GetCurrentStats() WebRTCStats

	// GetStatsHistory 获取统计历史
	GetStatsHistory() []WebRTCStats

	// OnStatsUpdate 设置统计更新回调
	OnStatsUpdate(callback func(stats WebRTCStats)) error
}

// WebRTCStats WebRTC统计信息
type WebRTCStats struct {
	Timestamp time.Time `json:"timestamp"`

	// 连接状态统计
	ConnectionState    webrtc.PeerConnectionState `json:"connection_state"`
	ICEConnectionState webrtc.ICEConnectionState  `json:"ice_connection_state"`
	SignalingState     webrtc.SignalingState      `json:"signaling_state"`

	// 传输统计
	BytesSent     int64 `json:"bytes_sent"`
	BytesReceived int64 `json:"bytes_received"`
	PacketsSent   int64 `json:"packets_sent"`
	PacketsLost   int64 `json:"packets_lost"`

	// RTT统计
	CurrentRTT time.Duration `json:"current_rtt"`
	AverageRTT time.Duration `json:"average_rtt"`
	MinRTT     time.Duration `json:"min_rtt"`
	MaxRTT     time.Duration `json:"max_rtt"`

	// 抖动统计
	Jitter        time.Duration `json:"jitter"`
	AverageJitter time.Duration `json:"average_jitter"`

	// 比特率统计
	VideoBitrate int64 `json:"video_bitrate"`
	AudioBitrate int64 `json:"audio_bitrate"`

	// 帧率统计
	VideoFrameRate float64 `json:"video_frame_rate"`
	AudioFrameRate float64 `json:"audio_frame_rate"`

	// 质量统计
	VideoFramesDropped int64   `json:"video_frames_dropped"`
	AudioFramesDropped int64   `json:"audio_frames_dropped"`
	PacketLossRate     float64 `json:"packet_loss_rate"`

	// 带宽统计
	AvailableBandwidth int64 `json:"available_bandwidth"`
	UsedBandwidth      int64 `json:"used_bandwidth"`

	// 编解码器信息
	VideoCodec string `json:"video_codec"`
	AudioCodec string `json:"audio_codec"`
}

// webrtcStatsCollectorImpl WebRTC统计收集器实现
type webrtcStatsCollectorImpl struct {
	config         *WebRTCStatsConfig
	peerConnection *webrtc.PeerConnection

	// 状态管理
	running bool
	mutex   sync.RWMutex

	// 统计数据
	currentStats WebRTCStats
	statsHistory []WebRTCStats

	// RTT历史用于计算平均值
	rttHistory    []time.Duration
	jitterHistory []time.Duration

	// 回调函数
	updateCallback func(WebRTCStats)

	// 上下文和控制
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewWebRTCStatsCollector 创建新的WebRTC统计收集器
func NewWebRTCStatsCollector(config *WebRTCStatsConfig) WebRTCStatsCollector {
	if config == nil {
		config = DefaultWebRTCStatsConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &webrtcStatsCollectorImpl{
		config:        config,
		rttHistory:    make([]time.Duration, 0, config.MaxHistorySize),
		jitterHistory: make([]time.Duration, 0, config.MaxHistorySize),
		ctx:           ctx,
		cancel:        cancel,
	}
}

// Start 启动统计收集
func (wsc *webrtcStatsCollectorImpl) Start() error {
	wsc.mutex.Lock()
	defer wsc.mutex.Unlock()

	if wsc.running {
		return nil
	}

	wsc.running = true

	// 启动统计收集协程（即使没有peer connection也启动，在协程中检查）
	wsc.wg.Add(1)
	go wsc.collectStats()

	return nil
}

// Stop 停止统计收集
func (wsc *webrtcStatsCollectorImpl) Stop() error {
	wsc.mutex.Lock()
	if !wsc.running {
		wsc.mutex.Unlock()
		return nil
	}
	wsc.running = false
	wsc.mutex.Unlock()

	// 取消上下文
	wsc.cancel()

	// 等待协程结束
	wsc.wg.Wait()

	return nil
}

// SetPeerConnection 设置WebRTC连接
func (wsc *webrtcStatsCollectorImpl) SetPeerConnection(pc *webrtc.PeerConnection) error {
	wsc.mutex.Lock()
	defer wsc.mutex.Unlock()

	wsc.peerConnection = pc
	return nil
}

// GetCurrentStats 获取当前统计信息
func (wsc *webrtcStatsCollectorImpl) GetCurrentStats() WebRTCStats {
	wsc.mutex.RLock()
	defer wsc.mutex.RUnlock()
	return wsc.currentStats
}

// GetStatsHistory 获取统计历史
func (wsc *webrtcStatsCollectorImpl) GetStatsHistory() []WebRTCStats {
	wsc.mutex.RLock()
	defer wsc.mutex.RUnlock()

	// 返回历史记录的副本
	history := make([]WebRTCStats, len(wsc.statsHistory))
	copy(history, wsc.statsHistory)
	return history
}

// OnStatsUpdate 设置统计更新回调
func (wsc *webrtcStatsCollectorImpl) OnStatsUpdate(callback func(stats WebRTCStats)) error {
	wsc.mutex.Lock()
	defer wsc.mutex.Unlock()

	wsc.updateCallback = callback
	return nil
}

// collectStats 统计收集协程
func (wsc *webrtcStatsCollectorImpl) collectStats() {
	defer wsc.wg.Done()

	ticker := time.NewTicker(wsc.config.CollectionInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			wsc.performStatsCollection()
		case <-wsc.ctx.Done():
			return
		}
	}
}

// performStatsCollection 执行统计收集
func (wsc *webrtcStatsCollectorImpl) performStatsCollection() {
	wsc.mutex.Lock()
	pc := wsc.peerConnection
	wsc.mutex.Unlock()

	if pc == nil {
		return
	}

	// 收集WebRTC统计信息
	stats := pc.GetStats()
	webrtcStats := wsc.processWebRTCStats(stats, pc)

	wsc.mutex.Lock()
	defer wsc.mutex.Unlock()

	// 更新当前统计
	wsc.currentStats = webrtcStats

	// 添加到历史记录
	wsc.statsHistory = append(wsc.statsHistory, webrtcStats)
	if len(wsc.statsHistory) > wsc.config.MaxHistorySize {
		wsc.statsHistory = wsc.statsHistory[1:]
	}

	// 更新RTT和抖动历史
	if webrtcStats.CurrentRTT > 0 {
		wsc.rttHistory = append(wsc.rttHistory, webrtcStats.CurrentRTT)
		if len(wsc.rttHistory) > wsc.config.MaxHistorySize {
			wsc.rttHistory = wsc.rttHistory[1:]
		}
	}

	if webrtcStats.Jitter > 0 {
		wsc.jitterHistory = append(wsc.jitterHistory, webrtcStats.Jitter)
		if len(wsc.jitterHistory) > wsc.config.MaxHistorySize {
			wsc.jitterHistory = wsc.jitterHistory[1:]
		}
	}

	// 调用回调函数
	if wsc.updateCallback != nil {
		go wsc.updateCallback(webrtcStats)
	}
}

// processWebRTCStats 处理WebRTC统计信息
func (wsc *webrtcStatsCollectorImpl) processWebRTCStats(statsReport webrtc.StatsReport, pc *webrtc.PeerConnection) WebRTCStats {
	stats := WebRTCStats{
		Timestamp: time.Now(),
	}

	// 获取连接状态
	stats.ConnectionState = pc.ConnectionState()
	stats.ICEConnectionState = pc.ICEConnectionState()
	stats.SignalingState = pc.SignalingState()

	// 处理统计报告
	for _, stat := range statsReport {
		switch s := stat.(type) {
		case *webrtc.OutboundRTPStreamStats:
			stats.BytesSent += int64(s.BytesSent)
			stats.PacketsSent += int64(s.PacketsSent)

			// 简单的比特率计算（实际应该基于时间差和媒体类型）
			// 这里我们将所有出站流的比特率加到视频比特率中
			stats.VideoBitrate += int64(s.BytesSent * 8) // 转换为bits

		case *webrtc.InboundRTPStreamStats:
			stats.BytesReceived += int64(s.BytesReceived)

		case *webrtc.RemoteInboundRTPStreamStats:
			stats.PacketsLost += int64(s.PacketsLost)

			// RTT统计
			if s.RoundTripTime > 0 {
				rtt := time.Duration(s.RoundTripTime * float64(time.Second))
				stats.CurrentRTT = rtt
				wsc.updateRTTStats(&stats, rtt)
			}

			// 抖动统计
			if s.Jitter > 0 {
				jitter := time.Duration(s.Jitter * float64(time.Second))
				stats.Jitter = jitter
				wsc.updateJitterStats(&stats, jitter)
			}

		case *webrtc.TransportStats:
			stats.BytesSent += int64(s.BytesSent)
			stats.BytesReceived += int64(s.BytesReceived)

		case *webrtc.ICECandidatePairStats:
			if s.State == webrtc.StatsICECandidatePairStateSucceeded {
				stats.AvailableBandwidth = int64(s.AvailableOutgoingBitrate)
			}
		}
	}

	// 计算丢包率
	if stats.PacketsSent > 0 {
		stats.PacketLossRate = float64(stats.PacketsLost) / float64(stats.PacketsSent)
	}

	// 计算使用带宽
	stats.UsedBandwidth = stats.VideoBitrate + stats.AudioBitrate

	return stats
}

// updateRTTStats 更新RTT统计信息
func (wsc *webrtcStatsCollectorImpl) updateRTTStats(stats *WebRTCStats, currentRTT time.Duration) {
	if len(wsc.rttHistory) == 0 {
		stats.MinRTT = currentRTT
		stats.MaxRTT = currentRTT
		stats.AverageRTT = currentRTT
		return
	}

	// 计算平均RTT
	var total time.Duration
	minRTT := wsc.rttHistory[0]
	maxRTT := wsc.rttHistory[0]

	for _, rtt := range wsc.rttHistory {
		total += rtt
		if rtt < minRTT {
			minRTT = rtt
		}
		if rtt > maxRTT {
			maxRTT = rtt
		}
	}

	stats.AverageRTT = total / time.Duration(len(wsc.rttHistory))
	stats.MinRTT = minRTT
	stats.MaxRTT = maxRTT
}

// updateJitterStats 更新抖动统计信息
func (wsc *webrtcStatsCollectorImpl) updateJitterStats(stats *WebRTCStats, currentJitter time.Duration) {
	if len(wsc.jitterHistory) == 0 {
		stats.AverageJitter = currentJitter
		return
	}

	// 计算平均抖动
	var total time.Duration
	for _, jitter := range wsc.jitterHistory {
		total += jitter
	}

	stats.AverageJitter = total / time.Duration(len(wsc.jitterHistory))
}
