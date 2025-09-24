package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/open-beagle/bdwind-gstreamer/internal/config"
)

const (
	AppName    = "BDWind-GStreamer"
	AppVersion = "1.0.0"
)

// checkPortAvailability 检查配置中的端口是否可用
func checkPortAvailability(cfg *config.Config, logger *logrus.Entry) error {
	portsToCheck := make(map[int]string)

	// 添加 WebServer 端口
	if cfg.WebServer != nil {
		portsToCheck[cfg.WebServer.Port] = "WebServer"
	}

	// 添加 Metrics 端口
	if cfg.Metrics != nil && cfg.Metrics.External.Enabled {
		portsToCheck[cfg.Metrics.External.Port] = "Metrics"
	}

	// 检查每个端口
	for port, service := range portsToCheck {
		if err := checkPortInUse(port); err != nil {
			return fmt.Errorf("%s port %d is already in use: %v", service, port, err)
		}
		logger.Tracef("Port %d (%s) is available", port, service)
	}

	return nil
}

// checkPortInUse 检查指定端口是否被占用
func checkPortInUse(port int) error {
	// 检查 TCP 端口
	tcpAddr := fmt.Sprintf(":%d", port)
	tcpListener, err := net.Listen("tcp", tcpAddr)
	if err != nil {
		return fmt.Errorf("TCP port occupied: %v", err)
	}
	tcpListener.Close()

	// 检查 UDP 端口（对于某些服务可能需要）
	udpAddr := fmt.Sprintf(":%d", port)
	udpConn, err := net.ListenPacket("udp", udpAddr)
	if err != nil {
		return fmt.Errorf("UDP port occupied: %v", err)
	}
	udpConn.Close()

	return nil
}

func main() {
	// 解析命令行参数
	var (
		configFile = flag.String("config", "", "Configuration file path")
		port       = flag.Int("port", 8080, "Web server port")
		host       = flag.String("host", "0.0.0.0", "Web server host")
		staticDir  = flag.String("static", "web/dist", "Static files directory")
		enableTLS  = flag.Bool("tls", false, "Enable TLS")
		certFile   = flag.String("cert", "", "TLS certificate file")
		keyFile    = flag.String("key", "", "TLS key file")
		displayID  = flag.String("display", ":0", "Display ID for capture")
		logLevel   = flag.String("log-level", "", "Log level (TRACE, DEBUG, INFO, WARN, ERROR)")
		logOutput  = flag.String("log-output", "", "Log output (stdout, stderr, file)")
		logFile    = flag.String("log-file", "", "Log file path (when log-output is file)")
		version    = flag.Bool("version", false, "Show version information")
	)
	flag.Parse()

	// 加载配置
	var cfg *config.Config
	var err error

	if *configFile != "" {
		cfg, err = config.LoadConfigFromFile(*configFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
			os.Exit(1)
		}
	} else {
		cfg = config.DefaultConfig()
	}

	// 命令行参数覆盖配置
	if *port != 8080 {
		cfg.WebServer.Port = *port
	}
	if *host != "0.0.0.0" {
		cfg.WebServer.Host = *host
	}
	if *staticDir != "web/dist" {
		cfg.WebServer.StaticDir = *staticDir
	}
	if *enableTLS {
		cfg.WebServer.EnableTLS = true
		cfg.WebServer.TLS.CertFile = *certFile
		cfg.WebServer.TLS.KeyFile = *keyFile
	}
	if *displayID != ":0" {
		cfg.GStreamer.Capture.DisplayID = *displayID
	}

	// 日志配置覆盖
	if *logLevel != "" {
		if level, err := config.ParseLogLevel(*logLevel); err == nil {
			cfg.Logging.Level = level
		} else {
			fmt.Fprintf(os.Stderr, "Invalid log level '%s': %v\n", *logLevel, err)
		}
	}
	if *logOutput != "" {
		cfg.Logging.Output = *logOutput
	}
	if *logFile != "" {
		cfg.Logging.File = config.NormalizeLogFilePath(*logFile)
		if cfg.Logging.Output == "" {
			cfg.Logging.Output = "file"
		}
	}

	// 验证配置
	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "Invalid configuration: %v\n", err)
		os.Exit(1)
	}

	// 验证日志配置
	if err := config.ValidateLoggingSetup(cfg.Logging); err != nil {
		fmt.Fprintf(os.Stderr, "Logging configuration validation failed: %v\n", err)
		os.Exit(1)
	}

	// 初始化日志系统
	if err := config.SetupLogger(cfg.Logging); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to setup logger: %v\n", err)
		os.Exit(1)
	}

	// 日志系统初始化完成，现在开始输出日志
	logger := config.GetLoggerWithPrefix("app")

	// 处理 version 请求
	if *version {
		logger.Infof("%s v%s", AppName, AppVersion)
		logger.Info("High-performance WebRTC desktop streaming server")
		return
	}

	// 输出详细的日志配置信息
	config.PrintLoggingInfo(cfg.Logging)
	logger.Trace("✅ Logging system initialized successfully")

	// 检查端口占用
	logger.Trace("Checking port availability...")
	if err := checkPortAvailability(cfg, logger); err != nil {
		logger.Errorf("❌ Port availability check failed: %v", err)
		logger.Info("💡 Please ensure the required ports are not in use by other applications")
		logger.Info("💡 You can check port usage with: netstat -tlnp | grep :<port>")
		os.Exit(1)
	}
	logger.Trace("✅ All required ports are available")

	// 创建应用
	app, err := NewBDWindApp(cfg, nil)
	if err != nil {
		logger.Fatalf("Failed to create application: %v", err)
	}

	// 设置信号处理
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// 启动应用
	go func() {
		if err := app.Start(); err != nil {
			logger.Fatalf("Application failed to start: %v", err)
		}
	}()

	// 显示启动信息
	protocol := "http"
	if cfg.WebServer.EnableTLS {
		protocol = "https"
	}

	logger.Infof("%s v%s started successfully!", AppName, AppVersion)
	logger.Infof("Web Interface: %s://%s:%d", protocol, cfg.WebServer.Host, cfg.WebServer.Port)
	if cfg.Metrics.External.Enabled {
		logger.Infof("Metrics: http://%s:%d/metrics", cfg.WebServer.Host, cfg.Metrics.External.Port)
	}
	logger.Infof("Display: %s", cfg.GStreamer.Capture.DisplayID)
	logger.Infof("Authentication: %v", cfg.WebServer.Auth.Enabled)
	logger.Infof("Codec: %s", cfg.GStreamer.Encoding.Codec)

	// 等待信号
	<-sigChan

	// 优雅关闭
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := app.Stop(ctx); err != nil {
		logger.Errorf("Application shutdown error: %v", err)
	} else {
		logger.Info("Application stopped gracefully")
	}

	// Force garbage collection and wait for finalizers to complete
	// This prevents segfaults during GObject cleanup
	logger.Debug("Forcing garbage collection to prevent segfaults...")
	runtime.GC()
	runtime.GC() // Run twice to ensure all finalizers are processed

	// Give finalizers time to complete
	time.Sleep(100 * time.Millisecond)

	logger.Debug("Garbage collection completed, application exiting safely")
}
