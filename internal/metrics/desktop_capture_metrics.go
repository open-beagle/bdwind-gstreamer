package metrics

import (
	"sync"
	"time"
)

// DesktopCaptureMetrics 桌面捕获指标收集器
type DesktopCaptureMetrics struct {
	metrics Metrics
	mu      sync.RWMutex

	// 帧率指标
	framesCapture Counter // 总捕获帧数
	framesDropped Counter // 总丢帧数
	currentFPS    Gauge   // 当前帧率
	averageFPS    Gauge   // 平均帧率

	// 延迟指标
	captureLatency Histogram // 捕获延迟分布
	minLatency     Gauge     // 最小延迟
	maxLatency     Gauge     // 最大延迟
	avgLatency     Gauge     // 平均延迟

	// 系统指标
	captureActive Gauge // 捕获是否活跃
	lastFrameTime Gauge // 最后一帧时间戳
	captureUptime Gauge // 捕获运行时间

	// 质量指标
	captureErrors Counter // 捕获错误数
	restartCount  Counter // 重启次数
}

// NewDesktopCaptureMetrics 创建桌面捕获指标收集器
func NewDesktopCaptureMetrics(metrics Metrics) (*DesktopCaptureMetrics, error) {
	dcm := &DesktopCaptureMetrics{
		metrics: metrics,
	}

	var err error

	// 注册帧率指标
	dcm.framesCapture, err = metrics.RegisterCounter(
		"desktop_capture_frames_total",
		"Total number of frames captured",
		[]string{"display", "source_type"},
	)
	if err != nil {
		return nil, err
	}

	dcm.framesDropped, err = metrics.RegisterCounter(
		"desktop_capture_frames_dropped_total",
		"Total number of frames dropped",
		[]string{"display", "source_type"},
	)
	if err != nil {
		return nil, err
	}

	dcm.currentFPS, err = metrics.RegisterGauge(
		"desktop_capture_fps_current",
		"Current frames per second",
		[]string{"display", "source_type"},
	)
	if err != nil {
		return nil, err
	}

	dcm.averageFPS, err = metrics.RegisterGauge(
		"desktop_capture_fps_average",
		"Average frames per second since start",
		[]string{"display", "source_type"},
	)
	if err != nil {
		return nil, err
	}

	// 注册延迟指标
	latencyBuckets := []float64{1, 5, 10, 20, 50, 100, 200, 500, 1000, 2000}
	dcm.captureLatency, err = metrics.RegisterHistogram(
		"desktop_capture_latency_milliseconds",
		"Desktop capture latency in milliseconds",
		[]string{"display", "source_type"},
		latencyBuckets,
	)
	if err != nil {
		return nil, err
	}

	dcm.minLatency, err = metrics.RegisterGauge(
		"desktop_capture_latency_min_milliseconds",
		"Minimum capture latency in milliseconds",
		[]string{"display", "source_type"},
	)
	if err != nil {
		return nil, err
	}

	dcm.maxLatency, err = metrics.RegisterGauge(
		"desktop_capture_latency_max_milliseconds",
		"Maximum capture latency in milliseconds",
		[]string{"display", "source_type"},
	)
	if err != nil {
		return nil, err
	}

	dcm.avgLatency, err = metrics.RegisterGauge(
		"desktop_capture_latency_avg_milliseconds",
		"Average capture latency in milliseconds",
		[]string{"display", "source_type"},
	)
	if err != nil {
		return nil, err
	}

	// 注册系统指标
	dcm.captureActive, err = metrics.RegisterGauge(
		"desktop_capture_active",
		"Whether desktop capture is currently active (1=active, 0=inactive)",
		[]string{"display", "source_type"},
	)
	if err != nil {
		return nil, err
	}

	dcm.lastFrameTime, err = metrics.RegisterGauge(
		"desktop_capture_last_frame_timestamp_seconds",
		"Timestamp of the last captured frame",
		[]string{"display", "source_type"},
	)
	if err != nil {
		return nil, err
	}

	dcm.captureUptime, err = metrics.RegisterGauge(
		"desktop_capture_uptime_seconds",
		"Time since capture started in seconds",
		[]string{"display", "source_type"},
	)
	if err != nil {
		return nil, err
	}

	// 注册质量指标
	dcm.captureErrors, err = metrics.RegisterCounter(
		"desktop_capture_errors_total",
		"Total number of capture errors",
		[]string{"display", "source_type", "error_type"},
	)
	if err != nil {
		return nil, err
	}

	dcm.restartCount, err = metrics.RegisterCounter(
		"desktop_capture_restarts_total",
		"Total number of capture restarts",
		[]string{"display", "source_type"},
	)
	if err != nil {
		return nil, err
	}

	return dcm, nil
}

// RecordFrameCapture 记录帧捕获事件
func (dcm *DesktopCaptureMetrics) RecordFrameCapture(display, sourceType string) {
	dcm.mu.Lock()
	defer dcm.mu.Unlock()

	dcm.framesCapture.Inc(display, sourceType)
	dcm.lastFrameTime.Set(float64(time.Now().Unix()), display, sourceType)
}

// RecordFrameDropped 记录丢帧事件
func (dcm *DesktopCaptureMetrics) RecordFrameDropped(display, sourceType string) {
	dcm.mu.Lock()
	defer dcm.mu.Unlock()

	dcm.framesDropped.Inc(display, sourceType)
}

// UpdateFPS 更新帧率指标
func (dcm *DesktopCaptureMetrics) UpdateFPS(display, sourceType string, currentFPS, averageFPS float64) {
	dcm.mu.Lock()
	defer dcm.mu.Unlock()

	dcm.currentFPS.Set(currentFPS, display, sourceType)
	dcm.averageFPS.Set(averageFPS, display, sourceType)
}

// RecordLatency 记录延迟指标
func (dcm *DesktopCaptureMetrics) RecordLatency(display, sourceType string, latencyMs float64) {
	dcm.mu.Lock()
	defer dcm.mu.Unlock()

	dcm.captureLatency.Observe(latencyMs, display, sourceType)
}

// UpdateLatencyStats 更新延迟统计指标
func (dcm *DesktopCaptureMetrics) UpdateLatencyStats(display, sourceType string, minLatency, maxLatency, avgLatency float64) {
	dcm.mu.Lock()
	defer dcm.mu.Unlock()

	dcm.minLatency.Set(minLatency, display, sourceType)
	dcm.maxLatency.Set(maxLatency, display, sourceType)
	dcm.avgLatency.Set(avgLatency, display, sourceType)
}

// SetCaptureActive 设置捕获活跃状态
func (dcm *DesktopCaptureMetrics) SetCaptureActive(display, sourceType string, active bool) {
	dcm.mu.Lock()
	defer dcm.mu.Unlock()

	if active {
		dcm.captureActive.Set(1, display, sourceType)
	} else {
		dcm.captureActive.Set(0, display, sourceType)
	}
}

// UpdateUptime 更新运行时间
func (dcm *DesktopCaptureMetrics) UpdateUptime(display, sourceType string, startTime time.Time) {
	dcm.mu.Lock()
	defer dcm.mu.Unlock()

	uptime := time.Since(startTime).Seconds()
	dcm.captureUptime.Set(uptime, display, sourceType)
}

// RecordError 记录错误事件
func (dcm *DesktopCaptureMetrics) RecordError(display, sourceType, errorType string) {
	dcm.mu.Lock()
	defer dcm.mu.Unlock()

	dcm.captureErrors.Inc(display, sourceType, errorType)
}

// RecordRestart 记录重启事件
func (dcm *DesktopCaptureMetrics) RecordRestart(display, sourceType string) {
	dcm.mu.Lock()
	defer dcm.mu.Unlock()

	dcm.restartCount.Inc(display, sourceType)
}

// CaptureStatsCollector 桌面捕获统计收集器
type CaptureStatsCollector struct {
	metrics    *DesktopCaptureMetrics
	display    string
	sourceType string
	startTime  time.Time
	mu         sync.RWMutex
}

// NewCaptureStatsCollector 创建捕获统计收集器
func NewCaptureStatsCollector(metrics *DesktopCaptureMetrics, display, sourceType string) *CaptureStatsCollector {
	return &CaptureStatsCollector{
		metrics:    metrics,
		display:    display,
		sourceType: sourceType,
		startTime:  time.Now(),
	}
}

// Start 开始统计收集
func (csc *CaptureStatsCollector) Start() {
	csc.mu.Lock()
	defer csc.mu.Unlock()

	csc.startTime = time.Now()
	csc.metrics.SetCaptureActive(csc.display, csc.sourceType, true)
}

// Stop 停止统计收集
func (csc *CaptureStatsCollector) Stop() {
	csc.mu.Lock()
	defer csc.mu.Unlock()

	csc.metrics.SetCaptureActive(csc.display, csc.sourceType, false)
}

// RecordFrame 记录帧事件
func (csc *CaptureStatsCollector) RecordFrame() {
	csc.mu.RLock()
	defer csc.mu.RUnlock()

	csc.metrics.RecordFrameCapture(csc.display, csc.sourceType)
	csc.metrics.UpdateUptime(csc.display, csc.sourceType, csc.startTime)
}

// RecordDroppedFrame 记录丢帧事件
func (csc *CaptureStatsCollector) RecordDroppedFrame() {
	csc.mu.RLock()
	defer csc.mu.RUnlock()

	csc.metrics.RecordFrameDropped(csc.display, csc.sourceType)
}

// UpdateFPS 更新帧率
func (csc *CaptureStatsCollector) UpdateFPS(currentFPS, averageFPS float64) {
	csc.mu.RLock()
	defer csc.mu.RUnlock()

	csc.metrics.UpdateFPS(csc.display, csc.sourceType, currentFPS, averageFPS)
}

// RecordLatency 记录延迟
func (csc *CaptureStatsCollector) RecordLatency(latencyMs float64) {
	csc.mu.RLock()
	defer csc.mu.RUnlock()

	csc.metrics.RecordLatency(csc.display, csc.sourceType, latencyMs)
}

// UpdateLatencyStats 更新延迟统计
func (csc *CaptureStatsCollector) UpdateLatencyStats(minLatency, maxLatency, avgLatency float64) {
	csc.mu.RLock()
	defer csc.mu.RUnlock()

	csc.metrics.UpdateLatencyStats(csc.display, csc.sourceType, minLatency, maxLatency, avgLatency)
}

// RecordError 记录错误
func (csc *CaptureStatsCollector) RecordError(errorType string) {
	csc.mu.RLock()
	defer csc.mu.RUnlock()

	csc.metrics.RecordError(csc.display, csc.sourceType, errorType)
}

// RecordRestart 记录重启
func (csc *CaptureStatsCollector) RecordRestart() {
	csc.mu.RLock()
	defer csc.mu.RUnlock()

	csc.metrics.RecordRestart(csc.display, csc.sourceType)
}
