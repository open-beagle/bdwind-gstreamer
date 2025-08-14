package metrics

import (
	"sync"
	"time"

	"github.com/pion/webrtc/v4"
)

// WebRTCMetrics WebRTC指标收集器
type WebRTCMetrics struct {
	metrics Metrics
	mu      sync.RWMutex

	// 连接状态指标
	connectionState    Gauge // 连接状态 (0=new, 1=connecting, 2=connected, 3=disconnected, 4=failed, 5=closed)
	iceConnectionState Gauge // ICE连接状态
	signalingState     Gauge // 信令状态

	// 传输指标
	bytesSent     Counter // 发送字节数
	bytesReceived Counter // 接收字节数
	packetsSent   Counter // 发送包数
	packetsLost   Counter // 丢失包数

	// RTT指标
	currentRTT Gauge     // 当前RTT
	averageRTT Gauge     // 平均RTT
	minRTT     Gauge     // 最小RTT
	maxRTT     Gauge     // 最大RTT
	rttLatency Histogram // RTT延迟分布

	// 抖动指标
	currentJitter Gauge     // 当前抖动
	averageJitter Gauge     // 平均抖动
	jitterLatency Histogram // 抖动分布

	// 比特率指标
	videoBitrate Gauge // 视频比特率
	audioBitrate Gauge // 音频比特率
	totalBitrate Gauge // 总比特率

	// 帧率指标
	videoFrameRate Gauge // 视频帧率
	audioFrameRate Gauge // 音频帧率

	// 质量指标
	videoFramesDropped Counter // 视频丢帧数
	audioFramesDropped Counter // 音频丢帧数
	packetLossRate     Gauge   // 丢包率

	// 带宽指标
	availableBandwidth   Gauge // 可用带宽
	usedBandwidth        Gauge // 使用带宽
	bandwidthUtilization Gauge // 带宽利用率

	// 编解码器指标
	videoCodecInfo Gauge // 视频编解码器信息 (标签包含编解码器名称)
	audioCodecInfo Gauge // 音频编解码器信息

	// 连接质量指标
	connectionQuality Gauge   // 连接质量评分 (0-1)
	connectionUptime  Gauge   // 连接持续时间
	reconnectCount    Counter // 重连次数

	// 错误指标
	connectionErrors Counter // 连接错误数
	mediaErrors      Counter // 媒体错误数
}

// NewWebRTCMetrics 创建WebRTC指标收集器
func NewWebRTCMetrics(metrics Metrics) (*WebRTCMetrics, error) {
	wm := &WebRTCMetrics{
		metrics: metrics,
	}

	var err error

	// 注册连接状态指标
	wm.connectionState, err = metrics.RegisterGauge(
		"webrtc_connection_state",
		"WebRTC connection state (0=new, 1=connecting, 2=connected, 3=disconnected, 4=failed, 5=closed)",
		[]string{"peer_id", "session_id"},
	)
	if err != nil {
		return nil, err
	}

	wm.iceConnectionState, err = metrics.RegisterGauge(
		"webrtc_ice_connection_state",
		"WebRTC ICE connection state",
		[]string{"peer_id", "session_id"},
	)
	if err != nil {
		return nil, err
	}

	wm.signalingState, err = metrics.RegisterGauge(
		"webrtc_signaling_state",
		"WebRTC signaling state",
		[]string{"peer_id", "session_id"},
	)
	if err != nil {
		return nil, err
	}

	// 注册传输指标
	wm.bytesSent, err = metrics.RegisterCounter(
		"webrtc_bytes_sent_total",
		"Total bytes sent over WebRTC",
		[]string{"peer_id", "session_id", "media_type"},
	)
	if err != nil {
		return nil, err
	}

	wm.bytesReceived, err = metrics.RegisterCounter(
		"webrtc_bytes_received_total",
		"Total bytes received over WebRTC",
		[]string{"peer_id", "session_id", "media_type"},
	)
	if err != nil {
		return nil, err
	}

	wm.packetsSent, err = metrics.RegisterCounter(
		"webrtc_packets_sent_total",
		"Total packets sent over WebRTC",
		[]string{"peer_id", "session_id", "media_type"},
	)
	if err != nil {
		return nil, err
	}

	wm.packetsLost, err = metrics.RegisterCounter(
		"webrtc_packets_lost_total",
		"Total packets lost over WebRTC",
		[]string{"peer_id", "session_id", "media_type"},
	)
	if err != nil {
		return nil, err
	}

	// 注册RTT指标
	wm.currentRTT, err = metrics.RegisterGauge(
		"webrtc_rtt_current_milliseconds",
		"Current round-trip time in milliseconds",
		[]string{"peer_id", "session_id"},
	)
	if err != nil {
		return nil, err
	}

	wm.averageRTT, err = metrics.RegisterGauge(
		"webrtc_rtt_average_milliseconds",
		"Average round-trip time in milliseconds",
		[]string{"peer_id", "session_id"},
	)
	if err != nil {
		return nil, err
	}

	wm.minRTT, err = metrics.RegisterGauge(
		"webrtc_rtt_min_milliseconds",
		"Minimum round-trip time in milliseconds",
		[]string{"peer_id", "session_id"},
	)
	if err != nil {
		return nil, err
	}

	wm.maxRTT, err = metrics.RegisterGauge(
		"webrtc_rtt_max_milliseconds",
		"Maximum round-trip time in milliseconds",
		[]string{"peer_id", "session_id"},
	)
	if err != nil {
		return nil, err
	}

	rttBuckets := []float64{1, 5, 10, 20, 50, 100, 200, 500, 1000, 2000, 5000}
	wm.rttLatency, err = metrics.RegisterHistogram(
		"webrtc_rtt_latency_milliseconds",
		"WebRTC round-trip time latency distribution",
		[]string{"peer_id", "session_id"},
		rttBuckets,
	)
	if err != nil {
		return nil, err
	}

	// 注册抖动指标
	wm.currentJitter, err = metrics.RegisterGauge(
		"webrtc_jitter_current_milliseconds",
		"Current jitter in milliseconds",
		[]string{"peer_id", "session_id", "media_type"},
	)
	if err != nil {
		return nil, err
	}

	wm.averageJitter, err = metrics.RegisterGauge(
		"webrtc_jitter_average_milliseconds",
		"Average jitter in milliseconds",
		[]string{"peer_id", "session_id", "media_type"},
	)
	if err != nil {
		return nil, err
	}

	jitterBuckets := []float64{0.1, 0.5, 1, 2, 5, 10, 20, 50, 100, 200}
	wm.jitterLatency, err = metrics.RegisterHistogram(
		"webrtc_jitter_latency_milliseconds",
		"WebRTC jitter latency distribution",
		[]string{"peer_id", "session_id", "media_type"},
		jitterBuckets,
	)
	if err != nil {
		return nil, err
	}

	// 注册比特率指标
	wm.videoBitrate, err = metrics.RegisterGauge(
		"webrtc_video_bitrate_bps",
		"Video bitrate in bits per second",
		[]string{"peer_id", "session_id", "codec"},
	)
	if err != nil {
		return nil, err
	}

	wm.audioBitrate, err = metrics.RegisterGauge(
		"webrtc_audio_bitrate_bps",
		"Audio bitrate in bits per second",
		[]string{"peer_id", "session_id", "codec"},
	)
	if err != nil {
		return nil, err
	}

	wm.totalBitrate, err = metrics.RegisterGauge(
		"webrtc_total_bitrate_bps",
		"Total bitrate in bits per second",
		[]string{"peer_id", "session_id"},
	)
	if err != nil {
		return nil, err
	}

	// 注册帧率指标
	wm.videoFrameRate, err = metrics.RegisterGauge(
		"webrtc_video_frame_rate_fps",
		"Video frame rate in frames per second",
		[]string{"peer_id", "session_id", "codec"},
	)
	if err != nil {
		return nil, err
	}

	wm.audioFrameRate, err = metrics.RegisterGauge(
		"webrtc_audio_frame_rate_fps",
		"Audio frame rate in frames per second",
		[]string{"peer_id", "session_id", "codec"},
	)
	if err != nil {
		return nil, err
	}

	// 注册质量指标
	wm.videoFramesDropped, err = metrics.RegisterCounter(
		"webrtc_video_frames_dropped_total",
		"Total video frames dropped",
		[]string{"peer_id", "session_id", "codec"},
	)
	if err != nil {
		return nil, err
	}

	wm.audioFramesDropped, err = metrics.RegisterCounter(
		"webrtc_audio_frames_dropped_total",
		"Total audio frames dropped",
		[]string{"peer_id", "session_id", "codec"},
	)
	if err != nil {
		return nil, err
	}

	wm.packetLossRate, err = metrics.RegisterGauge(
		"webrtc_packet_loss_rate",
		"Packet loss rate (0-1)",
		[]string{"peer_id", "session_id", "media_type"},
	)
	if err != nil {
		return nil, err
	}

	// 注册带宽指标
	wm.availableBandwidth, err = metrics.RegisterGauge(
		"webrtc_available_bandwidth_bps",
		"Available bandwidth in bits per second",
		[]string{"peer_id", "session_id"},
	)
	if err != nil {
		return nil, err
	}

	wm.usedBandwidth, err = metrics.RegisterGauge(
		"webrtc_used_bandwidth_bps",
		"Used bandwidth in bits per second",
		[]string{"peer_id", "session_id"},
	)
	if err != nil {
		return nil, err
	}

	wm.bandwidthUtilization, err = metrics.RegisterGauge(
		"webrtc_bandwidth_utilization",
		"Bandwidth utilization ratio (0-1)",
		[]string{"peer_id", "session_id"},
	)
	if err != nil {
		return nil, err
	}

	// 注册编解码器指标
	wm.videoCodecInfo, err = metrics.RegisterGauge(
		"webrtc_video_codec_info",
		"Video codec information (1=active, 0=inactive)",
		[]string{"peer_id", "session_id", "codec", "profile", "level"},
	)
	if err != nil {
		return nil, err
	}

	wm.audioCodecInfo, err = metrics.RegisterGauge(
		"webrtc_audio_codec_info",
		"Audio codec information (1=active, 0=inactive)",
		[]string{"peer_id", "session_id", "codec", "sample_rate", "channels"},
	)
	if err != nil {
		return nil, err
	}

	// 注册连接质量指标
	wm.connectionQuality, err = metrics.RegisterGauge(
		"webrtc_connection_quality",
		"Connection quality score (0-1)",
		[]string{"peer_id", "session_id"},
	)
	if err != nil {
		return nil, err
	}

	wm.connectionUptime, err = metrics.RegisterGauge(
		"webrtc_connection_uptime_seconds",
		"Connection uptime in seconds",
		[]string{"peer_id", "session_id"},
	)
	if err != nil {
		return nil, err
	}

	wm.reconnectCount, err = metrics.RegisterCounter(
		"webrtc_reconnect_total",
		"Total number of reconnections",
		[]string{"peer_id", "session_id"},
	)
	if err != nil {
		return nil, err
	}

	// 注册错误指标
	wm.connectionErrors, err = metrics.RegisterCounter(
		"webrtc_connection_errors_total",
		"Total connection errors",
		[]string{"peer_id", "session_id", "error_type"},
	)
	if err != nil {
		return nil, err
	}

	wm.mediaErrors, err = metrics.RegisterCounter(
		"webrtc_media_errors_total",
		"Total media errors",
		[]string{"peer_id", "session_id", "media_type", "error_type"},
	)
	if err != nil {
		return nil, err
	}

	return wm, nil
}

// UpdateConnectionState 更新连接状态
func (wm *WebRTCMetrics) UpdateConnectionState(peerID, sessionID string, state webrtc.PeerConnectionState) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	stateValue := float64(state)
	wm.connectionState.Set(stateValue, peerID, sessionID)
}

// UpdateICEConnectionState 更新ICE连接状态
func (wm *WebRTCMetrics) UpdateICEConnectionState(peerID, sessionID string, state webrtc.ICEConnectionState) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	stateValue := float64(state)
	wm.iceConnectionState.Set(stateValue, peerID, sessionID)
}

// UpdateSignalingState 更新信令状态
func (wm *WebRTCMetrics) UpdateSignalingState(peerID, sessionID string, state webrtc.SignalingState) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	stateValue := float64(state)
	wm.signalingState.Set(stateValue, peerID, sessionID)
}

// RecordBytesSent 记录发送字节数
func (wm *WebRTCMetrics) RecordBytesSent(peerID, sessionID, mediaType string, bytes int64) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	wm.bytesSent.Add(float64(bytes), peerID, sessionID, mediaType)
}

// RecordBytesReceived 记录接收字节数
func (wm *WebRTCMetrics) RecordBytesReceived(peerID, sessionID, mediaType string, bytes int64) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	wm.bytesReceived.Add(float64(bytes), peerID, sessionID, mediaType)
}

// RecordPacketsSent 记录发送包数
func (wm *WebRTCMetrics) RecordPacketsSent(peerID, sessionID, mediaType string, packets int64) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	wm.packetsSent.Add(float64(packets), peerID, sessionID, mediaType)
}

// RecordPacketsLost 记录丢失包数
func (wm *WebRTCMetrics) RecordPacketsLost(peerID, sessionID, mediaType string, packets int64) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	wm.packetsLost.Add(float64(packets), peerID, sessionID, mediaType)
}

// UpdateRTTStats 更新RTT统计
func (wm *WebRTCMetrics) UpdateRTTStats(peerID, sessionID string, current, average, min, max time.Duration) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	currentMs := float64(current.Nanoseconds()) / 1e6
	averageMs := float64(average.Nanoseconds()) / 1e6
	minMs := float64(min.Nanoseconds()) / 1e6
	maxMs := float64(max.Nanoseconds()) / 1e6

	wm.currentRTT.Set(currentMs, peerID, sessionID)
	wm.averageRTT.Set(averageMs, peerID, sessionID)
	wm.minRTT.Set(minMs, peerID, sessionID)
	wm.maxRTT.Set(maxMs, peerID, sessionID)

	// 记录RTT分布
	wm.rttLatency.Observe(currentMs, peerID, sessionID)
}

// UpdateJitterStats 更新抖动统计
func (wm *WebRTCMetrics) UpdateJitterStats(peerID, sessionID, mediaType string, current, average time.Duration) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	currentMs := float64(current.Nanoseconds()) / 1e6
	averageMs := float64(average.Nanoseconds()) / 1e6

	wm.currentJitter.Set(currentMs, peerID, sessionID, mediaType)
	wm.averageJitter.Set(averageMs, peerID, sessionID, mediaType)

	// 记录抖动分布
	wm.jitterLatency.Observe(currentMs, peerID, sessionID, mediaType)
}

// UpdateBitrateStats 更新比特率统计
func (wm *WebRTCMetrics) UpdateBitrateStats(peerID, sessionID string, videoBitrate, audioBitrate int64, videoCodec, audioCodec string) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	wm.videoBitrate.Set(float64(videoBitrate), peerID, sessionID, videoCodec)
	wm.audioBitrate.Set(float64(audioBitrate), peerID, sessionID, audioCodec)
	wm.totalBitrate.Set(float64(videoBitrate+audioBitrate), peerID, sessionID)
}

// UpdateFrameRateStats 更新帧率统计
func (wm *WebRTCMetrics) UpdateFrameRateStats(peerID, sessionID string, videoFPS, audioFPS float64, videoCodec, audioCodec string) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	wm.videoFrameRate.Set(videoFPS, peerID, sessionID, videoCodec)
	wm.audioFrameRate.Set(audioFPS, peerID, sessionID, audioCodec)
}

// RecordFramesDropped 记录丢帧事件
func (wm *WebRTCMetrics) RecordFramesDropped(peerID, sessionID, mediaType, codec string, count int64) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	if mediaType == "video" {
		wm.videoFramesDropped.Add(float64(count), peerID, sessionID, codec)
	} else if mediaType == "audio" {
		wm.audioFramesDropped.Add(float64(count), peerID, sessionID, codec)
	}
}

// UpdatePacketLossRate 更新丢包率
func (wm *WebRTCMetrics) UpdatePacketLossRate(peerID, sessionID, mediaType string, lossRate float64) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	wm.packetLossRate.Set(lossRate, peerID, sessionID, mediaType)
}

// UpdateBandwidthStats 更新带宽统计
func (wm *WebRTCMetrics) UpdateBandwidthStats(peerID, sessionID string, available, used int64) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	wm.availableBandwidth.Set(float64(available), peerID, sessionID)
	wm.usedBandwidth.Set(float64(used), peerID, sessionID)

	// 计算带宽利用率
	var utilization float64
	if available > 0 {
		utilization = float64(used) / float64(available)
	}
	wm.bandwidthUtilization.Set(utilization, peerID, sessionID)
}

// UpdateCodecInfo 更新编解码器信息
func (wm *WebRTCMetrics) UpdateCodecInfo(peerID, sessionID string, videoCodec, audioCodec string, videoProfile, videoLevel, audioSampleRate, audioChannels string) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	// 重置所有编解码器为非活跃状态（这里简化处理，实际应该更精确）
	wm.videoCodecInfo.Set(1, peerID, sessionID, videoCodec, videoProfile, videoLevel)
	wm.audioCodecInfo.Set(1, peerID, sessionID, audioCodec, audioSampleRate, audioChannels)
}

// UpdateConnectionQuality 更新连接质量
func (wm *WebRTCMetrics) UpdateConnectionQuality(peerID, sessionID string, quality float64) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	wm.connectionQuality.Set(quality, peerID, sessionID)
}

// UpdateConnectionUptime 更新连接持续时间
func (wm *WebRTCMetrics) UpdateConnectionUptime(peerID, sessionID string, startTime time.Time) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	uptime := time.Since(startTime).Seconds()
	wm.connectionUptime.Set(uptime, peerID, sessionID)
}

// RecordReconnect 记录重连事件
func (wm *WebRTCMetrics) RecordReconnect(peerID, sessionID string) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	wm.reconnectCount.Inc(peerID, sessionID)
}

// RecordConnectionError 记录连接错误
func (wm *WebRTCMetrics) RecordConnectionError(peerID, sessionID, errorType string) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	wm.connectionErrors.Inc(peerID, sessionID, errorType)
}

// RecordMediaError 记录媒体错误
func (wm *WebRTCMetrics) RecordMediaError(peerID, sessionID, mediaType, errorType string) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	wm.mediaErrors.Inc(peerID, sessionID, mediaType, errorType)
}

// WebRTCStatsCollector WebRTC统计收集器
type WebRTCStatsCollector struct {
	metrics   *WebRTCMetrics
	peerID    string
	sessionID string
	startTime time.Time
	mu        sync.RWMutex
}

// NewWebRTCStatsCollector 创建WebRTC统计收集器
func NewWebRTCStatsCollector(metrics *WebRTCMetrics, peerID, sessionID string) *WebRTCStatsCollector {
	return &WebRTCStatsCollector{
		metrics:   metrics,
		peerID:    peerID,
		sessionID: sessionID,
		startTime: time.Now(),
	}
}

// Start 开始统计收集
func (wsc *WebRTCStatsCollector) Start() {
	wsc.mu.Lock()
	defer wsc.mu.Unlock()

	wsc.startTime = time.Now()
}

// Stop 停止统计收集
func (wsc *WebRTCStatsCollector) Stop() {
	// 可以在这里进行清理工作
}

// UpdateConnectionState 更新连接状态
func (wsc *WebRTCStatsCollector) UpdateConnectionState(state webrtc.PeerConnectionState) {
	wsc.mu.RLock()
	defer wsc.mu.RUnlock()

	wsc.metrics.UpdateConnectionState(wsc.peerID, wsc.sessionID, state)
	wsc.metrics.UpdateConnectionUptime(wsc.peerID, wsc.sessionID, wsc.startTime)
}

// UpdateICEConnectionState 更新ICE连接状态
func (wsc *WebRTCStatsCollector) UpdateICEConnectionState(state webrtc.ICEConnectionState) {
	wsc.mu.RLock()
	defer wsc.mu.RUnlock()

	wsc.metrics.UpdateICEConnectionState(wsc.peerID, wsc.sessionID, state)
}

// UpdateSignalingState 更新信令状态
func (wsc *WebRTCStatsCollector) UpdateSignalingState(state webrtc.SignalingState) {
	wsc.mu.RLock()
	defer wsc.mu.RUnlock()

	wsc.metrics.UpdateSignalingState(wsc.peerID, wsc.sessionID, state)
}

// RecordBytesSent 记录发送字节数
func (wsc *WebRTCStatsCollector) RecordBytesSent(mediaType string, bytes int64) {
	wsc.mu.RLock()
	defer wsc.mu.RUnlock()

	wsc.metrics.RecordBytesSent(wsc.peerID, wsc.sessionID, mediaType, bytes)
}

// RecordBytesReceived 记录接收字节数
func (wsc *WebRTCStatsCollector) RecordBytesReceived(mediaType string, bytes int64) {
	wsc.mu.RLock()
	defer wsc.mu.RUnlock()

	wsc.metrics.RecordBytesReceived(wsc.peerID, wsc.sessionID, mediaType, bytes)
}

// RecordPacketsSent 记录发送包数
func (wsc *WebRTCStatsCollector) RecordPacketsSent(mediaType string, packets int64) {
	wsc.mu.RLock()
	defer wsc.mu.RUnlock()

	wsc.metrics.RecordPacketsSent(wsc.peerID, wsc.sessionID, mediaType, packets)
}

// RecordPacketsLost 记录丢失包数
func (wsc *WebRTCStatsCollector) RecordPacketsLost(mediaType string, packets int64) {
	wsc.mu.RLock()
	defer wsc.mu.RUnlock()

	wsc.metrics.RecordPacketsLost(wsc.peerID, wsc.sessionID, mediaType, packets)
}

// UpdateRTTStats 更新RTT统计
func (wsc *WebRTCStatsCollector) UpdateRTTStats(current, average, min, max time.Duration) {
	wsc.mu.RLock()
	defer wsc.mu.RUnlock()

	wsc.metrics.UpdateRTTStats(wsc.peerID, wsc.sessionID, current, average, min, max)
}

// UpdateJitterStats 更新抖动统计
func (wsc *WebRTCStatsCollector) UpdateJitterStats(mediaType string, current, average time.Duration) {
	wsc.mu.RLock()
	defer wsc.mu.RUnlock()

	wsc.metrics.UpdateJitterStats(wsc.peerID, wsc.sessionID, mediaType, current, average)
}

// UpdateBitrateStats 更新比特率统计
func (wsc *WebRTCStatsCollector) UpdateBitrateStats(videoBitrate, audioBitrate int64, videoCodec, audioCodec string) {
	wsc.mu.RLock()
	defer wsc.mu.RUnlock()

	wsc.metrics.UpdateBitrateStats(wsc.peerID, wsc.sessionID, videoBitrate, audioBitrate, videoCodec, audioCodec)
}

// UpdateFrameRateStats 更新帧率统计
func (wsc *WebRTCStatsCollector) UpdateFrameRateStats(videoFPS, audioFPS float64, videoCodec, audioCodec string) {
	wsc.mu.RLock()
	defer wsc.mu.RUnlock()

	wsc.metrics.UpdateFrameRateStats(wsc.peerID, wsc.sessionID, videoFPS, audioFPS, videoCodec, audioCodec)
}

// RecordFramesDropped 记录丢帧事件
func (wsc *WebRTCStatsCollector) RecordFramesDropped(mediaType, codec string, count int64) {
	wsc.mu.RLock()
	defer wsc.mu.RUnlock()

	wsc.metrics.RecordFramesDropped(wsc.peerID, wsc.sessionID, mediaType, codec, count)
}

// UpdatePacketLossRate 更新丢包率
func (wsc *WebRTCStatsCollector) UpdatePacketLossRate(mediaType string, lossRate float64) {
	wsc.mu.RLock()
	defer wsc.mu.RUnlock()

	wsc.metrics.UpdatePacketLossRate(wsc.peerID, wsc.sessionID, mediaType, lossRate)
}

// UpdateBandwidthStats 更新带宽统计
func (wsc *WebRTCStatsCollector) UpdateBandwidthStats(available, used int64) {
	wsc.mu.RLock()
	defer wsc.mu.RUnlock()

	wsc.metrics.UpdateBandwidthStats(wsc.peerID, wsc.sessionID, available, used)
}

// UpdateCodecInfo 更新编解码器信息
func (wsc *WebRTCStatsCollector) UpdateCodecInfo(videoCodec, audioCodec, videoProfile, videoLevel, audioSampleRate, audioChannels string) {
	wsc.mu.RLock()
	defer wsc.mu.RUnlock()

	wsc.metrics.UpdateCodecInfo(wsc.peerID, wsc.sessionID, videoCodec, audioCodec, videoProfile, videoLevel, audioSampleRate, audioChannels)
}

// UpdateConnectionQuality 更新连接质量
func (wsc *WebRTCStatsCollector) UpdateConnectionQuality(quality float64) {
	wsc.mu.RLock()
	defer wsc.mu.RUnlock()

	wsc.metrics.UpdateConnectionQuality(wsc.peerID, wsc.sessionID, quality)
}

// RecordReconnect 记录重连事件
func (wsc *WebRTCStatsCollector) RecordReconnect() {
	wsc.mu.RLock()
	defer wsc.mu.RUnlock()

	wsc.metrics.RecordReconnect(wsc.peerID, wsc.sessionID)
}

// RecordConnectionError 记录连接错误
func (wsc *WebRTCStatsCollector) RecordConnectionError(errorType string) {
	wsc.mu.RLock()
	defer wsc.mu.RUnlock()

	wsc.metrics.RecordConnectionError(wsc.peerID, wsc.sessionID, errorType)
}

// RecordMediaError 记录媒体错误
func (wsc *WebRTCStatsCollector) RecordMediaError(mediaType, errorType string) {
	wsc.mu.RLock()
	defer wsc.mu.RUnlock()

	wsc.metrics.RecordMediaError(wsc.peerID, wsc.sessionID, mediaType, errorType)
}
