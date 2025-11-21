package metrics

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"

	"github.com/open-beagle/bdwind-gstreamer/internal/common/config"
)

// Manager 监控组件管理器
// 实现 ComponentManager 接口，管理系统监控功能
type Manager struct {
	config          *config.MetricsConfig
	metrics         Metrics
	systemMetrics   SystemMetrics
	externalServer  *http.Server
	logger          *logrus.Entry
	internalRunning bool
	externalRunning bool
	startTime       time.Time
	mutex           sync.RWMutex
	ctx             context.Context
	cancel          context.CancelFunc
}

// NewManager 创建新的监控管理器
func NewManager(ctx context.Context, cfg *config.MetricsConfig) (*Manager, error) {
	if ctx == nil {
		return nil, fmt.Errorf("context is required")
	}

	if cfg == nil {
		return nil, fmt.Errorf("metrics config cannot be nil")
	}

	// 验证配置
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid metrics config: %w", err)
	}

	// 获取 logrus entry 用于结构化日志记录
	logger := config.GetLoggerWithPrefix("metrics")

	// 创建监控实例（内部监控始终创建）
	legacyConfig := MetricsConfig{
		Enabled: true, // 内部监控始终启用
		Port:    cfg.External.Port,
		Path:    cfg.External.Path,
		Host:    cfg.External.Host,
	}

	metrics, err := NewMetrics(legacyConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create metrics: %w", err)
	}

	// 创建系统监控实例
	systemMetrics, err := NewSystemMetrics(metrics)
	if err != nil {
		return nil, fmt.Errorf("failed to create system metrics: %w", err)
	}

	// Use the provided context instead of creating a new one
	childCtx, cancel := context.WithCancel(ctx)

	return &Manager{
		config:        cfg,
		metrics:       metrics,
		systemMetrics: systemMetrics,
		logger:        logger,
		ctx:           childCtx,
		cancel:        cancel,
	}, nil
}

// Start 启动监控管理器
func (m *Manager) Start(ctx context.Context) error {
	startTime := time.Now()
	m.logger.Info("Starting metrics manager...")

	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.internalRunning {
		m.logger.Warn("Metrics manager already running, skipping start")
		return fmt.Errorf("metrics manager already running")
	}

	// 记录配置信息
	m.logger.Debugf("Metrics configuration: external_enabled=%v, external_host=%s, external_port=%d, external_path=%s",
		m.config.External.Enabled, m.config.External.Host, m.config.External.Port, m.config.External.Path)

	// 始终启动内部监控（为web页面提供数据）
	m.logger.Debug("Starting internal metrics collection...")
	if err := m.startInternalMetrics(); err != nil {
		m.logger.Errorf("Failed to start internal metrics: %v", err)
		return fmt.Errorf("failed to start internal metrics: %w", err)
	}
	m.logger.Debug("Internal metrics collection started successfully")

	// 根据配置决定是否启动外部metrics暴露
	if m.config.External.Enabled {
		m.logger.Debug("Starting external metrics server...")
		if err := m.startExternalMetrics(); err != nil {
			m.logger.Errorf("Failed to start external metrics server: %v", err)
			// 外部metrics启动失败不影响内部监控，但记录警告
			m.logger.Warn("External metrics server failed to start, continuing with internal metrics only")
		} else {
			m.logger.Infof("External metrics server started on %s", m.config.GetExternalEndpoint())
		}
	} else {
		m.logger.Debug("External metrics disabled, only internal metrics will be available")
	}

	m.startTime = time.Now()
	duration := time.Since(startTime)
	
	// 验证启动状态
	internalStatus := "stopped"
	if m.internalRunning {
		internalStatus = "running"
	}
	externalStatus := "disabled"
	if m.config.External.Enabled {
		if m.externalRunning {
			externalStatus = "running"
		} else {
			externalStatus = "failed"
		}
	}

	m.logger.Infof("Metrics manager started successfully in %v (internal: %s, external: %s)", 
		duration, internalStatus, externalStatus)

	// 记录系统监控状态
	if m.systemMetrics.IsRunning() {
		m.logger.Info("System metrics collection is active")
	} else {
		m.logger.Warn("System metrics collection is not active")
	}

	return nil
}

// startInternalMetrics 启动内部监控
func (m *Manager) startInternalMetrics() error {
	m.logger.Trace("Initializing internal metrics collection...")
	
	// 启动系统监控，使用管理器的context以确保能够响应取消信号
	m.logger.Trace("Starting system metrics collector...")
	if err := m.systemMetrics.Start(m.ctx); err != nil {
		m.logger.Errorf("System metrics collector failed to start: %v", err)
		return fmt.Errorf("failed to start system metrics: %w", err)
	}
	m.logger.Trace("System metrics collector started successfully")

	// 验证系统监控状态
	if m.systemMetrics.IsRunning() {
		m.logger.Debug("System metrics collector is running and collecting data")
	} else {
		m.logger.Warn("System metrics collector started but not reporting as running")
	}

	m.internalRunning = true
	m.logger.Debug("Internal metrics started successfully")
	return nil
}

// startExternalMetrics 启动外部metrics暴露服务器
func (m *Manager) startExternalMetrics() error {
	addr := fmt.Sprintf("%s:%d", m.config.External.Host, m.config.External.Port)
	m.logger.Tracef("Creating external metrics server for %s%s", addr, m.config.External.Path)

	// 创建外部metrics服务器
	mux := http.NewServeMux()

	// 注册metrics端点
	m.logger.Trace("Registering metrics endpoint handler...")
	mux.HandleFunc(m.config.External.Path, func(w http.ResponseWriter, r *http.Request) {
		// 这里应该返回Prometheus格式的metrics
		// 暂时返回基本信息
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprintf(w, "# HELP bdwind_uptime_seconds Total uptime in seconds\n")
		fmt.Fprintf(w, "# TYPE bdwind_uptime_seconds counter\n")
		fmt.Fprintf(w, "bdwind_uptime_seconds %.2f\n", time.Since(m.startTime).Seconds())

		// 添加系统metrics
		if m.systemMetrics.IsRunning() {
			fmt.Fprintf(w, "# HELP bdwind_cpu_usage_percent CPU usage percentage\n")
			fmt.Fprintf(w, "# TYPE bdwind_cpu_usage_percent gauge\n")
			fmt.Fprintf(w, "bdwind_cpu_usage_percent %.2f\n", m.systemMetrics.GetCPUUsage())

			memUsage := m.systemMetrics.GetMemoryUsage()
			fmt.Fprintf(w, "# HELP bdwind_memory_usage_percent Memory usage percentage\n")
			fmt.Fprintf(w, "# TYPE bdwind_memory_usage_percent gauge\n")
			fmt.Fprintf(w, "bdwind_memory_usage_percent %.2f\n", memUsage.Percent)
		}
	})

	m.logger.Debug("Creating HTTP server for external metrics...")
	m.externalServer = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	// 验证端口可用性
	m.logger.Debugf("Attempting to bind external metrics server to %s", addr)

	// 在goroutine中启动服务器
	serverStarted := make(chan error, 1)
	go func() {
		m.logger.Tracef("Starting external metrics server goroutine on %s", addr)
		if err := m.externalServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			m.logger.Errorf("External metrics server error: %v", err)
			serverStarted <- err
		}
	}()

	// 等待服务器启动并检查错误
	time.Sleep(50 * time.Millisecond)
	select {
	case err := <-serverStarted:
		m.logger.Errorf("External metrics server failed to start: %v", err)
		return fmt.Errorf("failed to start external metrics server: %w", err)
	default:
		// 没有错误，继续
	}

	m.externalRunning = true
	endpoint := fmt.Sprintf("http://%s%s", addr, m.config.External.Path)
	m.logger.Debugf("External metrics server started successfully on %s", endpoint)
	return nil
}

// Stop 停止监控管理器
func (m *Manager) Stop(ctx context.Context) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if !m.internalRunning && !m.externalRunning {
		return nil
	}

	m.logger.Trace("Stopping metrics manager...")

	// 取消context以通知所有子组件停止
	if m.cancel != nil {
		m.cancel()
	}

	var errors []error

	// 停止外部metrics服务器
	if m.externalRunning && m.externalServer != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := m.externalServer.Shutdown(shutdownCtx); err != nil {
			errors = append(errors, fmt.Errorf("failed to stop external metrics server: %w", err))
		} else {
			m.logger.Debug("External metrics server stopped")
		}
		m.externalRunning = false
	}

	// 停止内部监控
	if m.internalRunning {
		// 停止系统监控
		if err := m.systemMetrics.Stop(); err != nil {
			errors = append(errors, fmt.Errorf("failed to stop system metrics: %w", err))
		}
		m.internalRunning = false
	}

	if len(errors) > 0 {
		// 返回第一个错误，但记录所有错误
		for _, err := range errors[1:] {
			m.logger.Errorf("Additional stop error: %v", err)
		}
		return errors[0]
	}

	return nil
}

// IsEnabled 检查监控组件是否启用
// 监控组件始终启用（内部监控），外部暴露可选
func (m *Manager) IsEnabled() bool {
	return true
}

// IsRunning 检查监控管理器是否正在运行
func (m *Manager) IsRunning() bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.internalRunning
}

// IsInternalRunning 检查内部监控是否正在运行
func (m *Manager) IsInternalRunning() bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.internalRunning
}

// IsExternalRunning 检查外部metrics服务器是否正在运行
func (m *Manager) IsExternalRunning() bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.externalRunning
}

// IsExternalEnabled 检查外部metrics是否启用
func (m *Manager) IsExternalEnabled() bool {
	return m.config.External.Enabled
}

// GetStats 获取监控管理器的统计信息
func (m *Manager) GetStats() map[string]interface{} {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	stats := map[string]interface{}{
		"internal_running": m.internalRunning,
		"external_running": m.externalRunning,
		"external_enabled": m.config.External.Enabled,
		"start_time":       m.startTime.Unix(),
		"uptime":           time.Since(m.startTime).Seconds(),
	}

	if m.internalRunning {
		// 添加系统监控统计
		stats["cpu_usage"] = m.systemMetrics.GetCPUUsage()
		stats["memory_usage"] = m.systemMetrics.GetMemoryUsage()
		stats["gpu_usage"] = m.systemMetrics.GetGPUUsage()

		// 添加监控服务状态
		stats["system_metrics_running"] = m.systemMetrics.IsRunning()
	}

	if m.externalRunning {
		stats["external_endpoint"] = m.config.GetExternalEndpoint()
	}

	return stats
}

// GetContext 获取组件的上下文
func (m *Manager) GetContext() context.Context {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.ctx
}

// SetupRoutes 设置监控相关的HTTP路由（内部web页面使用）
func (m *Manager) SetupRoutes(router *mux.Router) error {
	m.logger.Debug("Setting up internal metrics routes...")

	// 创建系统监控处理器
	handler := NewSystemHandler(m.systemMetrics)

	// 注册系统监控路由 - 使用 /api/system 前缀
	systemRouter := router.PathPrefix("/api/system").Subrouter()

	// 注册具体的路由（这些路由始终可用，为web页面提供数据）
	systemRouter.HandleFunc("/stats", handler.handleStats).Methods("GET")
	systemRouter.HandleFunc("/logs", handler.handleLogs).Methods("GET")
	systemRouter.HandleFunc("/processes", handler.handleProcesses).Methods("GET")
	systemRouter.HandleFunc("/resources", handler.handleResources).Methods("GET")
	systemRouter.HandleFunc("/info", handler.handleInfo).Methods("GET")

	// 添加metrics状态路由
	systemRouter.HandleFunc("/metrics-status", m.handleMetricsStatus).Methods("GET")

	m.logger.Debug("Internal metrics routes registered successfully")
	return nil
}

// handleMetricsStatus 处理metrics状态请求
func (m *Manager) handleMetricsStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	stats := m.GetStats()

	// 简单的JSON序列化
	response := fmt.Sprintf(`{
		"internal_running": %t,
		"external_running": %t,
		"external_enabled": %t,
		"uptime": %.2f,
		"cpu_usage": %.2f,
		"memory_usage": %.2f
	}`,
		stats["internal_running"].(bool),
		stats["external_running"].(bool),
		stats["external_enabled"].(bool),
		stats["uptime"].(float64),
		stats["cpu_usage"].(float64),
		stats["memory_usage"].(MemoryUsage).Percent,
	)

	w.Write([]byte(response))
}

// GetMetrics 获取监控实例（用于其他组件集成）
func (m *Manager) GetMetrics() Metrics {
	return m.metrics
}

// GetSystemMetrics 获取系统监控实例（用于其他组件集成）
func (m *Manager) GetSystemMetrics() SystemMetrics {
	return m.systemMetrics
}
