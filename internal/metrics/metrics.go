package metrics

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics 监控接口
type Metrics interface {
	// Start 启动监控服务
	Start() error

	// Stop 停止监控服务
	Stop() error

	// RegisterGauge 注册仪表盘指标
	RegisterGauge(name, help string, labels []string) (Gauge, error)

	// RegisterCounter 注册计数器指标
	RegisterCounter(name, help string, labels []string) (Counter, error)

	// RegisterHistogram 注册直方图指标
	RegisterHistogram(name, help string, labels []string, buckets []float64) (Histogram, error)

	// GetRegistry 获取 Prometheus 注册表
	GetRegistry() *prometheus.Registry

	// IsRunning 检查服务是否运行
	IsRunning() bool
}

// Gauge 仪表盘接口
type Gauge interface {
	// Set 设置值
	Set(value float64, labels ...string)

	// Inc 增加1
	Inc(labels ...string)

	// Dec 减少1
	Dec(labels ...string)

	// Add 增加值
	Add(value float64, labels ...string)

	// Sub 减少值
	Sub(value float64, labels ...string)
}

// Counter 计数器接口
type Counter interface {
	// Inc 增加1
	Inc(labels ...string)

	// Add 增加值
	Add(value float64, labels ...string)
}

// Histogram 直方图接口
type Histogram interface {
	// Observe 观察值
	Observe(value float64, labels ...string)
}

// metricsImpl Metrics接口的实现
type metricsImpl struct {
	config   MetricsConfig
	registry *prometheus.Registry
	server   *http.Server
	running  bool
	mu       sync.RWMutex

	// 存储已注册的指标
	gauges     map[string]*prometheus.GaugeVec
	counters   map[string]*prometheus.CounterVec
	histograms map[string]*prometheus.HistogramVec
}

// NewMetrics 创建新的监控实例
func NewMetrics(config MetricsConfig) (Metrics, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	registry := prometheus.NewRegistry()

	return &metricsImpl{
		config:     config,
		registry:   registry,
		gauges:     make(map[string]*prometheus.GaugeVec),
		counters:   make(map[string]*prometheus.CounterVec),
		histograms: make(map[string]*prometheus.HistogramVec),
	}, nil
}

// Start 启动监控服务
func (m *metricsImpl) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.config.Enabled {
		// 当禁用时返回 nil 而不是错误，但不启动服务
		return nil
	}

	if m.running {
		return ErrServerAlreadyRunning
	}

	mux := http.NewServeMux()
	mux.Handle(m.config.Path, promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{}))

	m.server = &http.Server{
		Addr:         fmt.Sprintf("%s:%d", m.config.Host, m.config.Port),
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		if err := m.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			// 记录错误，但不阻塞
			fmt.Printf("Metrics server error: %v\n", err)
		}
	}()

	m.running = true
	return nil
}

// Stop 停止监控服务
func (m *metricsImpl) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return ErrServerNotRunning
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := m.server.Shutdown(ctx); err != nil {
		return err
	}

	m.running = false
	return nil
}

// RegisterGauge 注册仪表盘指标
func (m *metricsImpl) RegisterGauge(name, help string, labels []string) (Gauge, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.gauges[name]; exists {
		return nil, ErrMetricAlreadyRegistered
	}

	gauge := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: name,
			Help: help,
		},
		labels,
	)

	if err := m.registry.Register(gauge); err != nil {
		return nil, err
	}

	m.gauges[name] = gauge
	return &gaugeImpl{gauge: gauge}, nil
}

// RegisterCounter 注册计数器指标
func (m *metricsImpl) RegisterCounter(name, help string, labels []string) (Counter, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.counters[name]; exists {
		return nil, ErrMetricAlreadyRegistered
	}

	counter := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: name,
			Help: help,
		},
		labels,
	)

	if err := m.registry.Register(counter); err != nil {
		return nil, err
	}

	m.counters[name] = counter
	return &counterImpl{counter: counter}, nil
}

// RegisterHistogram 注册直方图指标
func (m *metricsImpl) RegisterHistogram(name, help string, labels []string, buckets []float64) (Histogram, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.histograms[name]; exists {
		return nil, ErrMetricAlreadyRegistered
	}

	histogram := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    name,
			Help:    help,
			Buckets: buckets,
		},
		labels,
	)

	if err := m.registry.Register(histogram); err != nil {
		return nil, err
	}

	m.histograms[name] = histogram
	return &histogramImpl{histogram: histogram}, nil
}

// GetRegistry 获取 Prometheus 注册表
func (m *metricsImpl) GetRegistry() *prometheus.Registry {
	return m.registry
}

// IsRunning 检查服务是否运行
func (m *metricsImpl) IsRunning() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.running
}

// gaugeImpl Gauge接口的实现
type gaugeImpl struct {
	gauge *prometheus.GaugeVec
}

// Set 设置值
func (g *gaugeImpl) Set(value float64, labels ...string) {
	g.gauge.WithLabelValues(labels...).Set(value)
}

// Inc 增加1
func (g *gaugeImpl) Inc(labels ...string) {
	g.gauge.WithLabelValues(labels...).Inc()
}

// Dec 减少1
func (g *gaugeImpl) Dec(labels ...string) {
	g.gauge.WithLabelValues(labels...).Dec()
}

// Add 增加值
func (g *gaugeImpl) Add(value float64, labels ...string) {
	g.gauge.WithLabelValues(labels...).Add(value)
}

// Sub 减少值
func (g *gaugeImpl) Sub(value float64, labels ...string) {
	g.gauge.WithLabelValues(labels...).Sub(value)
}

// counterImpl Counter接口的实现
type counterImpl struct {
	counter *prometheus.CounterVec
}

// Inc 增加1
func (c *counterImpl) Inc(labels ...string) {
	c.counter.WithLabelValues(labels...).Inc()
}

// Add 增加值
func (c *counterImpl) Add(value float64, labels ...string) {
	c.counter.WithLabelValues(labels...).Add(value)
}

// histogramImpl Histogram接口的实现
type histogramImpl struct {
	histogram *prometheus.HistogramVec
}

// Observe 观察值
func (h *histogramImpl) Observe(value float64, labels ...string) {
	h.histogram.WithLabelValues(labels...).Observe(value)
}
