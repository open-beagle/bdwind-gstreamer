package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/open-beagle/bdwind-gstreamer/internal/config"
)

const (
	AppName    = "BDWind-GStreamer"
	AppVersion = "1.0.0"
)

// checkPortAvailability 检查配置中的端口是否可用
func checkPortAvailability(cfg *config.Config) error {
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
		log.Printf("  ✅ Port %d (%s) is available", port, service)
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

	if *version {
		fmt.Printf("%s v%s\n", AppName, AppVersion)
		fmt.Println("High-performance WebRTC desktop streaming server")
		return
	}

	// 加载配置
	var cfg *config.Config
	var err error

	if *configFile != "" {
		log.Printf("Loading configuration from: %s", *configFile)
		cfg, err = config.LoadConfigFromFile(*configFile)
		if err != nil {
			log.Fatalf("Failed to load configuration: %v", err)
		}
		log.Printf("Configuration loaded successfully")
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
			log.Printf("Invalid log level '%s': %v", *logLevel, err)
		}
	}
	if *logOutput != "" {
		cfg.Logging.Output = *logOutput
	}
	if *logFile != "" {
		cfg.Logging.File = *logFile
		if cfg.Logging.Output == "" {
			cfg.Logging.Output = "file"
		}
	}

	// 验证配置
	if err := cfg.Validate(); err != nil {
		log.Fatalf("Invalid configuration: %v", err)
	}

	// 初始化日志系统
	if err := config.SetupLogger(cfg.Logging); err != nil {
		log.Fatalf("Failed to setup logger: %v", err)
	}

	// 检查端口占用
	log.Printf("Checking port availability...")
	if err := checkPortAvailability(cfg); err != nil {
		log.Printf("❌ Port availability check failed: %v", err)
		log.Printf("💡 Please ensure the required ports are not in use by other applications")
		log.Printf("💡 You can check port usage with: netstat -tlnp | grep :<port>")
		os.Exit(1)
	}
	log.Printf("✅ All required ports are available")

	// 创建应用
	app, err := NewBDWindApp(cfg, nil)
	if err != nil {
		log.Fatalf("Failed to create application: %v", err)
	}

	// 设置信号处理
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// 启动应用
	go func() {
		if err := app.Start(); err != nil {
			log.Fatalf("Application failed to start: %v", err)
		}
	}()

	// 显示启动信息
	protocol := "http"
	if cfg.WebServer.EnableTLS {
		protocol = "https"
	}

	fmt.Printf("\n🚀 %s v%s started successfully!\n", AppName, AppVersion)
	fmt.Printf("📱 Web Interface: %s://%s:%d\n", protocol, cfg.WebServer.Host, cfg.WebServer.Port)
	if cfg.Metrics.External.Enabled {
		fmt.Printf("📊 Metrics: http://%s:%d/metrics\n", cfg.WebServer.Host, cfg.Metrics.External.Port)
	}
	fmt.Printf("🖥️  Display: %s\n", cfg.GStreamer.Capture.DisplayID)
	fmt.Printf("🔐 Authentication: %v\n", cfg.WebServer.Auth.Enabled)
	fmt.Printf("🎬 Codec: %s\n", cfg.GStreamer.Encoding.Codec)
	fmt.Println("\nPress Ctrl+C to stop")

	// 等待信号
	<-sigChan

	// 优雅关闭
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := app.Stop(ctx); err != nil {
		log.Printf("Application shutdown error: %v", err)
	} else {
		log.Println("Application stopped gracefully")
	}
}
