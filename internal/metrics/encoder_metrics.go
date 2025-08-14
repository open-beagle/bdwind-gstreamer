package metrics

import (
	"sync"
	"time"
)

// EncoderMetrics 编码器指标收集器
type EncoderMetrics struct {
	metrics Metrics
	mu      sync.RWMutex

	// 编码性能指标
	framesEncoded    Counter // 总编码帧数
	keyframesEncoded Counter // 总关键帧数
	framesDropped    Counter // 总丢帧数
	framesSkipped    Counter // 总跳帧数
	encodingFPS      Gauge   // 编码帧率

	// 编码延迟指标
	encodingLatency Histogram // 编码延迟分布
	minLatency      Gauge     // 最小延迟
	maxLatency      Gauge     // 最大延迟
	avgLatency      Gauge     // 平均延迟

	// 比特率指标
	currentBitrate    Gauge // 当前比特率
	averageBitrate    Gauge // 平均比特率
	targetBitrate     Gauge // 目标比特率
	peakBitrate       Gauge // 峰值比特率
	bitrateEfficiency Gauge // 比特率效率

	// 编码质量指标
	currentQuality  Gauge // 当前质量
	averageQuality  Gauge // 平均质量
	qualityVariance Gauge // 质量方差
	keyframeRatio   Gauge // 关键帧比例
	dropRatio       Gauge // 丢帧比例

	// 系统资源指标
	cpuUsage      Gauge // CPU 使用率
	memoryUsage   Gauge // 内存使用量
	encoderActive Gauge // 编码器是否活跃

	// 错误指标
	encodingErrors Counter // 编码错误数
	restartCount   Counter // 重启次数

	// 运行时间指标
	encoderUptime Gauge // 编码器运行时间
	lastFrameTime Gauge // 最后一帧时间戳
}

// NewEncoderMetrics 创建编码器指标收集器
func NewEncoderMetrics(metrics Metrics) (*EncoderMetrics, error) {
	em := &EncoderMetrics{
		metrics: metrics,
	}

	var err error

	// 注册编码性能指标
	em.framesEncoded, err = metrics.RegisterCounter(
		"encoder_frames_encoded_total",
		"Total number of frames encoded",
		[]string{"encoder_type", "codec", "hardware_accel"},
	)
	if err != nil {
		return nil, err
	}

	em.keyframesEncoded, err = metrics.RegisterCounter(
		"encoder_keyframes_encoded_total",
		"Total number of keyframes encoded",
		[]string{"encoder_type", "codec", "hardware_accel"},
	)
	if err != nil {
		return nil, err
	}

	em.framesDropped, err = metrics.RegisterCounter(
		"encoder_frames_dropped_total",
		"Total number of frames dropped during encoding",
		[]string{"encoder_type", "codec", "hardware_accel"},
	)
	if err != nil {
		return nil, err
	}

	em.framesSkipped, err = metrics.RegisterCounter(
		"encoder_frames_skipped_total",
		"Total number of frames skipped during encoding",
		[]string{"encoder_type", "codec", "hardware_accel"},
	)
	if err != nil {
		return nil, err
	}

	em.encodingFPS, err = metrics.RegisterGauge(
		"encoder_fps",
		"Current encoding frames per second",
		[]string{"encoder_type", "codec", "hardware_accel"},
	)
	if err != nil {
		return nil, err
	}

	// 注册编码延迟指标
	latencyBuckets := []float64{1, 5, 10, 20, 50, 100, 200, 500, 1000, 2000}
	em.encodingLatency, err = metrics.RegisterHistogram(
		"encoder_latency_milliseconds",
		"Encoding latency in milliseconds",
		[]string{"encoder_type", "codec", "hardware_accel"},
		latencyBuckets,
	)
	if err != nil {
		return nil, err
	}

	em.minLatency, err = metrics.RegisterGauge(
		"encoder_latency_min_milliseconds",
		"Minimum encoding latency in milliseconds",
		[]string{"encoder_type", "codec", "hardware_accel"},
	)
	if err != nil {
		return nil, err
	}

	em.maxLatency, err = metrics.RegisterGauge(
		"encoder_latency_max_milliseconds",
		"Maximum encoding latency in milliseconds",
		[]string{"encoder_type", "codec", "hardware_accel"},
	)
	if err != nil {
		return nil, err
	}

	em.avgLatency, err = metrics.RegisterGauge(
		"encoder_latency_avg_milliseconds",
		"Average encoding latency in milliseconds",
		[]string{"encoder_type", "codec", "hardware_accel"},
	)
	if err != nil {
		return nil, err
	}

	// 注册比特率指标
	em.currentBitrate, err = metrics.RegisterGauge(
		"encoder_bitrate_current_kbps",
		"Current encoding bitrate in kbps",
		[]string{"encoder_type", "codec", "hardware_accel"},
	)
	if err != nil {
		return nil, err
	}

	em.averageBitrate, err = metrics.RegisterGauge(
		"encoder_bitrate_average_kbps",
		"Average encoding bitrate in kbps",
		[]string{"encoder_type", "codec", "hardware_accel"},
	)
	if err != nil {
		return nil, err
	}

	em.targetBitrate, err = metrics.RegisterGauge(
		"encoder_bitrate_target_kbps",
		"Target encoding bitrate in kbps",
		[]string{"encoder_type", "codec", "hardware_accel"},
	)
	if err != nil {
		return nil, err
	}

	em.peakBitrate, err = metrics.RegisterGauge(
		"encoder_bitrate_peak_kbps",
		"Peak encoding bitrate in kbps",
		[]string{"encoder_type", "codec", "hardware_accel"},
	)
	if err != nil {
		return nil, err
	}

	em.bitrateEfficiency, err = metrics.RegisterGauge(
		"encoder_bitrate_efficiency",
		"Bitrate efficiency (actual/target)",
		[]string{"encoder_type", "codec", "hardware_accel"},
	)
	if err != nil {
		return nil, err
	}

	// 注册编码质量指标
	em.currentQuality, err = metrics.RegisterGauge(
		"encoder_quality_current",
		"Current encoding quality (0-1)",
		[]string{"encoder_type", "codec", "hardware_accel"},
	)
	if err != nil {
		return nil, err
	}

	em.averageQuality, err = metrics.RegisterGauge(
		"encoder_quality_average",
		"Average encoding quality (0-1)",
		[]string{"encoder_type", "codec", "hardware_accel"},
	)
	if err != nil {
		return nil, err
	}

	em.qualityVariance, err = metrics.RegisterGauge(
		"encoder_quality_variance",
		"Encoding quality variance",
		[]string{"encoder_type", "codec", "hardware_accel"},
	)
	if err != nil {
		return nil, err
	}

	em.keyframeRatio, err = metrics.RegisterGauge(
		"encoder_keyframe_ratio",
		"Ratio of keyframes to total frames",
		[]string{"encoder_type", "codec", "hardware_accel"},
	)
	if err != nil {
		return nil, err
	}

	em.dropRatio, err = metrics.RegisterGauge(
		"encoder_drop_ratio",
		"Ratio of dropped frames to total frames",
		[]string{"encoder_type", "codec", "hardware_accel"},
	)
	if err != nil {
		return nil, err
	}

	// 注册系统资源指标
	em.cpuUsage, err = metrics.RegisterGauge(
		"encoder_cpu_usage_percent",
		"Encoder CPU usage percentage",
		[]string{"encoder_type", "codec", "hardware_accel"},
	)
	if err != nil {
		return nil, err
	}

	em.memoryUsage, err = metrics.RegisterGauge(
		"encoder_memory_usage_bytes",
		"Encoder memory usage in bytes",
		[]string{"encoder_type", "codec", "hardware_accel"},
	)
	if err != nil {
		return nil, err
	}

	em.encoderActive, err = metrics.RegisterGauge(
		"encoder_active",
		"Whether encoder is currently active (1=active, 0=inactive)",
		[]string{"encoder_type", "codec", "hardware_accel"},
	)
	if err != nil {
		return nil, err
	}

	// 注册错误指标
	em.encodingErrors, err = metrics.RegisterCounter(
		"encoder_errors_total",
		"Total number of encoding errors",
		[]string{"encoder_type", "codec", "hardware_accel", "error_type"},
	)
	if err != nil {
		return nil, err
	}

	em.restartCount, err = metrics.RegisterCounter(
		"encoder_restarts_total",
		"Total number of encoder restarts",
		[]string{"encoder_type", "codec", "hardware_accel"},
	)
	if err != nil {
		return nil, err
	}

	// 注册运行时间指标
	em.encoderUptime, err = metrics.RegisterGauge(
		"encoder_uptime_seconds",
		"Time since encoder started in seconds",
		[]string{"encoder_type", "codec", "hardware_accel"},
	)
	if err != nil {
		return nil, err
	}

	em.lastFrameTime, err = metrics.RegisterGauge(
		"encoder_last_frame_timestamp_seconds",
		"Timestamp of the last encoded frame",
		[]string{"encoder_type", "codec", "hardware_accel"},
	)
	if err != nil {
		return nil, err
	}

	return em, nil
}

// RecordFrameEncoded 记录帧编码事件
func (em *EncoderMetrics) RecordFrameEncoded(encoderType, codec, hardwareAccel string, isKeyframe bool) {
	em.mu.Lock()
	defer em.mu.Unlock()

	em.framesEncoded.Inc(encoderType, codec, hardwareAccel)
	if isKeyframe {
		em.keyframesEncoded.Inc(encoderType, codec, hardwareAccel)
	}
	em.lastFrameTime.Set(float64(time.Now().Unix()), encoderType, codec, hardwareAccel)
}

// RecordFrameDropped 记录丢帧事件
func (em *EncoderMetrics) RecordFrameDropped(encoderType, codec, hardwareAccel string) {
	em.mu.Lock()
	defer em.mu.Unlock()

	em.framesDropped.Inc(encoderType, codec, hardwareAccel)
}

// RecordFrameSkipped 记录跳帧事件
func (em *EncoderMetrics) RecordFrameSkipped(encoderType, codec, hardwareAccel string) {
	em.mu.Lock()
	defer em.mu.Unlock()

	em.framesSkipped.Inc(encoderType, codec, hardwareAccel)
}

// UpdateEncodingFPS 更新编码帧率
func (em *EncoderMetrics) UpdateEncodingFPS(encoderType, codec, hardwareAccel string, fps float64) {
	em.mu.Lock()
	defer em.mu.Unlock()

	em.encodingFPS.Set(fps, encoderType, codec, hardwareAccel)
}

// RecordEncodingLatency 记录编码延迟
func (em *EncoderMetrics) RecordEncodingLatency(encoderType, codec, hardwareAccel string, latencyMs float64) {
	em.mu.Lock()
	defer em.mu.Unlock()

	em.encodingLatency.Observe(latencyMs, encoderType, codec, hardwareAccel)
}

// UpdateLatencyStats 更新延迟统计指标
func (em *EncoderMetrics) UpdateLatencyStats(encoderType, codec, hardwareAccel string, minLatency, maxLatency, avgLatency float64) {
	em.mu.Lock()
	defer em.mu.Unlock()

	em.minLatency.Set(minLatency, encoderType, codec, hardwareAccel)
	em.maxLatency.Set(maxLatency, encoderType, codec, hardwareAccel)
	em.avgLatency.Set(avgLatency, encoderType, codec, hardwareAccel)
}

// UpdateBitrateStats 更新比特率统计指标
func (em *EncoderMetrics) UpdateBitrateStats(encoderType, codec, hardwareAccel string, current, average, target, peak int, efficiency float64) {
	em.mu.Lock()
	defer em.mu.Unlock()

	em.currentBitrate.Set(float64(current), encoderType, codec, hardwareAccel)
	em.averageBitrate.Set(float64(average), encoderType, codec, hardwareAccel)
	em.targetBitrate.Set(float64(target), encoderType, codec, hardwareAccel)
	em.peakBitrate.Set(float64(peak), encoderType, codec, hardwareAccel)
	em.bitrateEfficiency.Set(efficiency, encoderType, codec, hardwareAccel)
}

// UpdateQualityStats 更新质量统计指标
func (em *EncoderMetrics) UpdateQualityStats(encoderType, codec, hardwareAccel string, current, average, variance float64) {
	em.mu.Lock()
	defer em.mu.Unlock()

	em.currentQuality.Set(current, encoderType, codec, hardwareAccel)
	em.averageQuality.Set(average, encoderType, codec, hardwareAccel)
	em.qualityVariance.Set(variance, encoderType, codec, hardwareAccel)
}

// UpdateRatios 更新比例指标
func (em *EncoderMetrics) UpdateRatios(encoderType, codec, hardwareAccel string, keyframeRatio, dropRatio float64) {
	em.mu.Lock()
	defer em.mu.Unlock()

	em.keyframeRatio.Set(keyframeRatio, encoderType, codec, hardwareAccel)
	em.dropRatio.Set(dropRatio, encoderType, codec, hardwareAccel)
}

// UpdateResourceUsage 更新资源使用指标
func (em *EncoderMetrics) UpdateResourceUsage(encoderType, codec, hardwareAccel string, cpuUsage float64, memoryUsage int64) {
	em.mu.Lock()
	defer em.mu.Unlock()

	em.cpuUsage.Set(cpuUsage, encoderType, codec, hardwareAccel)
	em.memoryUsage.Set(float64(memoryUsage), encoderType, codec, hardwareAccel)
}

// SetEncoderActive 设置编码器活跃状态
func (em *EncoderMetrics) SetEncoderActive(encoderType, codec, hardwareAccel string, active bool) {
	em.mu.Lock()
	defer em.mu.Unlock()

	if active {
		em.encoderActive.Set(1, encoderType, codec, hardwareAccel)
	} else {
		em.encoderActive.Set(0, encoderType, codec, hardwareAccel)
	}
}

// UpdateUptime 更新运行时间
func (em *EncoderMetrics) UpdateUptime(encoderType, codec, hardwareAccel string, startTime time.Time) {
	em.mu.Lock()
	defer em.mu.Unlock()

	uptime := time.Since(startTime).Seconds()
	em.encoderUptime.Set(uptime, encoderType, codec, hardwareAccel)
}

// RecordError 记录错误事件
func (em *EncoderMetrics) RecordError(encoderType, codec, hardwareAccel, errorType string) {
	em.mu.Lock()
	defer em.mu.Unlock()

	em.encodingErrors.Inc(encoderType, codec, hardwareAccel, errorType)
}

// RecordRestart 记录重启事件
func (em *EncoderMetrics) RecordRestart(encoderType, codec, hardwareAccel string) {
	em.mu.Lock()
	defer em.mu.Unlock()

	em.restartCount.Inc(encoderType, codec, hardwareAccel)
}

// EncoderStatsCollector 编码器统计收集器
type EncoderStatsCollector struct {
	metrics       *EncoderMetrics
	encoderType   string
	codec         string
	hardwareAccel string
	startTime     time.Time
	mu            sync.RWMutex
}

// NewEncoderStatsCollector 创建编码器统计收集器
func NewEncoderStatsCollector(metrics *EncoderMetrics, encoderType, codec string, hardwareAccel bool) *EncoderStatsCollector {
	hardwareAccelStr := "false"
	if hardwareAccel {
		hardwareAccelStr = "true"
	}

	return &EncoderStatsCollector{
		metrics:       metrics,
		encoderType:   encoderType,
		codec:         codec,
		hardwareAccel: hardwareAccelStr,
		startTime:     time.Now(),
	}
}

// Start 开始统计收集
func (esc *EncoderStatsCollector) Start() {
	esc.mu.Lock()
	defer esc.mu.Unlock()

	esc.startTime = time.Now()
	esc.metrics.SetEncoderActive(esc.encoderType, esc.codec, esc.hardwareAccel, true)
}

// Stop 停止统计收集
func (esc *EncoderStatsCollector) Stop() {
	esc.mu.Lock()
	defer esc.mu.Unlock()

	esc.metrics.SetEncoderActive(esc.encoderType, esc.codec, esc.hardwareAccel, false)
}

// RecordFrameEncoded 记录帧编码事件
func (esc *EncoderStatsCollector) RecordFrameEncoded(isKeyframe bool) {
	esc.mu.RLock()
	defer esc.mu.RUnlock()

	esc.metrics.RecordFrameEncoded(esc.encoderType, esc.codec, esc.hardwareAccel, isKeyframe)
	esc.metrics.UpdateUptime(esc.encoderType, esc.codec, esc.hardwareAccel, esc.startTime)
}

// RecordFrameDropped 记录丢帧事件
func (esc *EncoderStatsCollector) RecordFrameDropped() {
	esc.mu.RLock()
	defer esc.mu.RUnlock()

	esc.metrics.RecordFrameDropped(esc.encoderType, esc.codec, esc.hardwareAccel)
}

// RecordFrameSkipped 记录跳帧事件
func (esc *EncoderStatsCollector) RecordFrameSkipped() {
	esc.mu.RLock()
	defer esc.mu.RUnlock()

	esc.metrics.RecordFrameSkipped(esc.encoderType, esc.codec, esc.hardwareAccel)
}

// UpdateEncodingFPS 更新编码帧率
func (esc *EncoderStatsCollector) UpdateEncodingFPS(fps float64) {
	esc.mu.RLock()
	defer esc.mu.RUnlock()

	esc.metrics.UpdateEncodingFPS(esc.encoderType, esc.codec, esc.hardwareAccel, fps)
}

// RecordEncodingLatency 记录编码延迟
func (esc *EncoderStatsCollector) RecordEncodingLatency(latencyMs float64) {
	esc.mu.RLock()
	defer esc.mu.RUnlock()

	esc.metrics.RecordEncodingLatency(esc.encoderType, esc.codec, esc.hardwareAccel, latencyMs)
}

// UpdateLatencyStats 更新延迟统计
func (esc *EncoderStatsCollector) UpdateLatencyStats(minLatency, maxLatency, avgLatency float64) {
	esc.mu.RLock()
	defer esc.mu.RUnlock()

	esc.metrics.UpdateLatencyStats(esc.encoderType, esc.codec, esc.hardwareAccel, minLatency, maxLatency, avgLatency)
}

// UpdateBitrateStats 更新比特率统计
func (esc *EncoderStatsCollector) UpdateBitrateStats(current, average, target, peak int, efficiency float64) {
	esc.mu.RLock()
	defer esc.mu.RUnlock()

	esc.metrics.UpdateBitrateStats(esc.encoderType, esc.codec, esc.hardwareAccel, current, average, target, peak, efficiency)
}

// UpdateQualityStats 更新质量统计
func (esc *EncoderStatsCollector) UpdateQualityStats(current, average, variance float64) {
	esc.mu.RLock()
	defer esc.mu.RUnlock()

	esc.metrics.UpdateQualityStats(esc.encoderType, esc.codec, esc.hardwareAccel, current, average, variance)
}

// UpdateRatios 更新比例指标
func (esc *EncoderStatsCollector) UpdateRatios(keyframeRatio, dropRatio float64) {
	esc.mu.RLock()
	defer esc.mu.RUnlock()

	esc.metrics.UpdateRatios(esc.encoderType, esc.codec, esc.hardwareAccel, keyframeRatio, dropRatio)
}

// UpdateResourceUsage 更新资源使用
func (esc *EncoderStatsCollector) UpdateResourceUsage(cpuUsage float64, memoryUsage int64) {
	esc.mu.RLock()
	defer esc.mu.RUnlock()

	esc.metrics.UpdateResourceUsage(esc.encoderType, esc.codec, esc.hardwareAccel, cpuUsage, memoryUsage)
}

// RecordError 记录错误
func (esc *EncoderStatsCollector) RecordError(errorType string) {
	esc.mu.RLock()
	defer esc.mu.RUnlock()

	esc.metrics.RecordError(esc.encoderType, esc.codec, esc.hardwareAccel, errorType)
}

// RecordRestart 记录重启
func (esc *EncoderStatsCollector) RecordRestart() {
	esc.mu.RLock()
	defer esc.mu.RUnlock()

	esc.metrics.RecordRestart(esc.encoderType, esc.codec, esc.hardwareAccel)
}
