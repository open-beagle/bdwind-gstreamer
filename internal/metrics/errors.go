package metrics

import "errors"

var (
	// ErrInvalidPort 无效端口错误
	ErrInvalidPort = errors.New("metrics: invalid port number")

	// ErrMetricsNotEnabled 监控未启用错误
	ErrMetricsNotEnabled = errors.New("metrics: metrics not enabled")

	// ErrServerAlreadyRunning 服务器已运行错误
	ErrServerAlreadyRunning = errors.New("metrics: server already running")

	// ErrServerNotRunning 服务器未运行错误
	ErrServerNotRunning = errors.New("metrics: server not running")

	// ErrMetricAlreadyRegistered 指标已注册错误
	ErrMetricAlreadyRegistered = errors.New("metrics: metric already registered")

	// ErrMetricNotFound 指标未找到错误
	ErrMetricNotFound = errors.New("metrics: metric not found")
)
