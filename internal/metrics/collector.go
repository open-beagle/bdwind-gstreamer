package metrics

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// MetricsCollector 指标收集器
type MetricsCollector struct {
	registry *prometheus.Registry
	server   *http.Server
	mutex    sync.RWMutex
	running  bool

	// 系统指标收集器
	systemMetrics SystemMetrics

	// 系统指标
	cpuUsage    prometheus.Gauge
	memoryUsage prometheus.Gauge
	gpuUsage    prometheus.Gauge

	// 桌面捕获指标
	captureFrames  prometheus.Counter
	captureDropped prometheus.Counter
	captureFPS     prometheus.Gauge
	captureLatency prometheus.Histogram

	// 编码器指标
	encoderFrames    prometheus.Counter
	encoderKeyframes prometheus.Counter
	encoderBitrate   prometheus.Gauge
	encoderLatency   prometheus.Histogram

	// WebRTC指标
	webrtcConnections prometheus.Gauge
	webrtcBytesSent   prometheus.Counter
	webrtcPacketsSent prometheus.Counter
	webrtcPacketsLost prometheus.Counter
	webrtcRTT         prometheus.Histogram
}

// NewMetricsCollector 创建指标收集器
func NewMetricsCollector() *MetricsCollector {
	registry := prometheus.NewRegistry()

	// 创建系统指标收集器
	systemMetrics, err := NewSystemMetrics(&metricsImpl{registry: registry})
	if err != nil {
		// 如果创建失败，使用nil，后续会处理
		systemMetrics = nil
	}

	collector := &MetricsCollector{
		registry:      registry,
		systemMetrics: systemMetrics,
		// 系统指标
		cpuUsage: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "bdwind_system_cpu_usage_percent",
			Help: "Current CPU usage percentage",
		}),
		memoryUsage: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "bdwind_system_memory_usage_percent",
			Help: "Current memory usage percentage",
		}),
		gpuUsage: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "bdwind_system_gpu_usage_percent",
			Help: "Current GPU usage percentage",
		}),

		// 桌面捕获指标
		captureFrames: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "bdwind_capture_frames_total",
			Help: "Total number of captured frames",
		}),
		captureDropped: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "bdwind_capture_dropped_frames_total",
			Help: "Total number of dropped frames during capture",
		}),
		captureFPS: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "bdwind_capture_fps",
			Help: "Current capture frames per second",
		}),
		captureLatency: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "bdwind_capture_latency_seconds",
			Help:    "Capture latency in seconds",
			Buckets: prometheus.DefBuckets,
		}),

		// 编码器指标
		encoderFrames: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "bdwind_encoder_frames_total",
			Help: "Total number of encoded frames",
		}),
		encoderKeyframes: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "bdwind_encoder_keyframes_total",
			Help: "Total number of encoded keyframes",
		}),
		encoderBitrate: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "bdwind_encoder_bitrate_bps",
			Help: "Current encoder bitrate in bits per second",
		}),
		encoderLatency: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "bdwind_encoder_latency_seconds",
			Help:    "Encoder latency in seconds",
			Buckets: prometheus.DefBuckets,
		}),

		// WebRTC指标
		webrtcConnections: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "bdwind_webrtc_connections",
			Help: "Current number of WebRTC connections",
		}),
		webrtcBytesSent: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "bdwind_webrtc_bytes_sent_total",
			Help: "Total bytes sent via WebRTC",
		}),
		webrtcPacketsSent: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "bdwind_webrtc_packets_sent_total",
			Help: "Total packets sent via WebRTC",
		}),
		webrtcPacketsLost: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "bdwind_webrtc_packets_lost_total",
			Help: "Total packets lost in WebRTC transmission",
		}),
		webrtcRTT: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "bdwind_webrtc_rtt_seconds",
			Help:    "WebRTC round-trip time in seconds",
			Buckets: prometheus.DefBuckets,
		}),
	}

	// 注册所有指标
	collector.registerMetrics()

	return collector
}

// registerMetrics 注册所有指标
func (mc *MetricsCollector) registerMetrics() {
	// 系统指标
	mc.registry.MustRegister(mc.cpuUsage)
	mc.registry.MustRegister(mc.memoryUsage)
	mc.registry.MustRegister(mc.gpuUsage)

	// 桌面捕获指标
	mc.registry.MustRegister(mc.captureFrames)
	mc.registry.MustRegister(mc.captureDropped)
	mc.registry.MustRegister(mc.captureFPS)
	mc.registry.MustRegister(mc.captureLatency)

	// 编码器指标
	mc.registry.MustRegister(mc.encoderFrames)
	mc.registry.MustRegister(mc.encoderKeyframes)
	mc.registry.MustRegister(mc.encoderBitrate)
	mc.registry.MustRegister(mc.encoderLatency)

	// WebRTC指标
	mc.registry.MustRegister(mc.webrtcConnections)
	mc.registry.MustRegister(mc.webrtcBytesSent)
	mc.registry.MustRegister(mc.webrtcPacketsSent)
	mc.registry.MustRegister(mc.webrtcPacketsLost)
	mc.registry.MustRegister(mc.webrtcRTT)
}

// Start 启动指标收集器
func (mc *MetricsCollector) Start() {
	mc.mutex.Lock()
	defer mc.mutex.Unlock()

	if mc.running {
		return
	}

	mc.running = true

	// 启动系统指标收集
	if mc.systemMetrics != nil {
		go mc.systemMetrics.Start(context.Background())
	}

	// 启动定期更新协程
	go mc.updateLoop()
}

// Stop 停止指标收集器
func (mc *MetricsCollector) Stop() {
	mc.mutex.Lock()
	defer mc.mutex.Unlock()

	mc.running = false

	// 停止系统指标收集
	if mc.systemMetrics != nil {
		mc.systemMetrics.Stop()
	}

	if mc.server != nil {
		mc.server.Close()
	}
}

// Handler 返回HTTP处理器
func (mc *MetricsCollector) Handler() http.Handler {
	return promhttp.HandlerFor(mc.registry, promhttp.HandlerOpts{})
}

// updateLoop 定期更新指标
func (mc *MetricsCollector) updateLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		mc.mutex.RLock()
		running := mc.running
		mc.mutex.RUnlock()

		if !running {
			break
		}

		select {
		case <-ticker.C:
			mc.updateSystemMetrics()
		}
	}
}

// updateSystemMetrics 更新系统指标
func (mc *MetricsCollector) updateSystemMetrics() {
	// 这里应该实现实际的系统指标收集
	// 暂时使用模拟数据
	mc.cpuUsage.Set(25.5)
	mc.memoryUsage.Set(60.2)
	mc.gpuUsage.Set(45.8)
}

// 系统指标更新方法
func (mc *MetricsCollector) UpdateCPUUsage(usage float64) {
	mc.cpuUsage.Set(usage)
}

func (mc *MetricsCollector) UpdateMemoryUsage(usage float64) {
	mc.memoryUsage.Set(usage)
}

func (mc *MetricsCollector) UpdateGPUUsage(usage float64) {
	mc.gpuUsage.Set(usage)
}

// 桌面捕获指标更新方法
func (mc *MetricsCollector) IncrementCaptureFrames() {
	mc.captureFrames.Inc()
}

func (mc *MetricsCollector) IncrementCaptureDropped() {
	mc.captureDropped.Inc()
}

func (mc *MetricsCollector) UpdateCaptureFPS(fps float64) {
	mc.captureFPS.Set(fps)
}

func (mc *MetricsCollector) ObserveCaptureLatency(latency time.Duration) {
	mc.captureLatency.Observe(latency.Seconds())
}

// 编码器指标更新方法
func (mc *MetricsCollector) IncrementEncoderFrames() {
	mc.encoderFrames.Inc()
}

func (mc *MetricsCollector) IncrementEncoderKeyframes() {
	mc.encoderKeyframes.Inc()
}

func (mc *MetricsCollector) UpdateEncoderBitrate(bitrate float64) {
	mc.encoderBitrate.Set(bitrate)
}

func (mc *MetricsCollector) ObserveEncoderLatency(latency time.Duration) {
	mc.encoderLatency.Observe(latency.Seconds())
}

// WebRTC指标更新方法
func (mc *MetricsCollector) UpdateWebRTCConnections(count float64) {
	mc.webrtcConnections.Set(count)
}

func (mc *MetricsCollector) IncrementWebRTCBytesSent(bytes float64) {
	mc.webrtcBytesSent.Add(bytes)
}

func (mc *MetricsCollector) IncrementWebRTCPacketsSent() {
	mc.webrtcPacketsSent.Inc()
}

func (mc *MetricsCollector) IncrementWebRTCPacketsLost() {
	mc.webrtcPacketsLost.Inc()
}

func (mc *MetricsCollector) ObserveWebRTCRTT(rtt time.Duration) {
	mc.webrtcRTT.Observe(rtt.Seconds())
}

// GetSystemMetrics 获取系统指标收集器
func (mc *MetricsCollector) GetSystemMetrics() SystemMetrics {
	return mc.systemMetrics
}
