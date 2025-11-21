# Configuration Examples

This document provides examples and guidance for configuring the bdwind-gstreamer project.

**Note**: As of the latest version, bdwind-gstreamer uses go-gst implementation exclusively for better performance and direct memory transfer. The `implementation` configuration options have been removed.

## Configuration Formats

### 1. Legacy Format
The original configuration format with a flat structure:

```yaml
web:
  host: "0.0.0.0"
  port: 8080
  enable_tls: false
  enable_cors: true

capture:
  width: 1920
  height: 1080
  framerate: 30
  display_id: ":0"
  use_wayland: false

encoding:
  codec: "h264"

auth:
  enabled: false
  default_role: "user"

monitoring:
  enabled: true
  metrics_port: 9090
```

### 2. Current Format
The current configuration format using go-gst implementation:

```yaml
webserver:
  host: "0.0.0.0"
  port: 8080
  enable_tls: false
  enable_cors: true
  auth:
    enabled: false
    default_role: "user"

gstreamer:
  capture:
    width: 1920
    height: 1080
    framerate: 30
    display_id: ":0"
    use_wayland: false
  encoding:
    codec: "h264"
    bitrate: 2500
    use_hardware: false

webrtc:
  ice_servers:
    - urls: ["stun:stun.l.google.com:19302"]
  signaling_path: "/api/signaling"
  session:
    keep_alive_timeout: 30s  # 可选：ICE断开后继续推流的时间（默认30秒）

metrics:
  external:
    enabled: true
    port: 9090

logging:
  level: "info"
  format: "text"

lifecycle:
  shutdown_timeout: "30s"
  startup_timeout: "60s"
  enable_graceful_shutdown: true
```

### 3. Minimal Format
The minimal configuration format for basic usage:

```yaml
gstreamer:
  capture:
    width: 1920
    height: 1080
    framerate: 30
    display_id: ":0"
  encoding:
    codec: "h264"
    bitrate: 2500
  # 按需推流配置（可选）
  on_demand:
    enabled: true              # 启用按需推流（默认：true）
    idle_timeout: 5s           # 无客户端后停止推流的时间（默认：5秒）
    quick_reconnect_window: 10s # 快速重连窗口（默认：10秒）

webrtc:
  ice_servers:
    - urls: ["stun:stun.l.google.com:19302"]
  # 会话配置（可选）
  session:
    keep_alive_timeout: 30s    # ICE断开后继续推流的时间（默认：30秒）

webserver:
  host: "0.0.0.0"
  port: 8080

logging:
  level: "info"
```

## Configuration Usage

### Loading Configuration

```go
package main

import (
    "fmt"
    "github.com/open-beagle/bdwind-gstreamer/internal/config"
)

func main() {
    // Load configuration from file
    cfg, err := config.LoadConfigFromFile("config.yaml")
    if err != nil {
        fmt.Printf("Failed to load configuration: %v\n", err)
        return
    }
    
    // Validate configuration
    if err := cfg.Validate(); err != nil {
        fmt.Printf("Configuration validation failed: %v\n", err)
        return
    }
    
    fmt.Println("Configuration loaded successfully!")
}
```

### Configuration Validation

```bash
# Start the application with configuration validation
./bdwind-gstreamer --config config.yaml

# The application will automatically validate the configuration on startup
```

## Configuration Options Reference

### GStreamer On-Demand Streaming (可选)

按需推流功能允许应用只在有客户端连接时才启动视频捕获和编码，节省系统资源。

```yaml
gstreamer:
  on_demand:
    enabled: true              # 是否启用按需推流（默认：true）
    idle_timeout: 5s           # 无客户端后多久停止推流（默认：5秒）
    quick_reconnect_window: 10s # 快速重连窗口（默认：10秒）
```

**参数说明：**

- `enabled`: 
  - `true`: 按需启动，有客户端才推流（推荐，节省资源）
  - `false`: 启动即推流，保持旧行为
  - 默认值：`true`

- `idle_timeout`: 
  - 最后一个客户端离开后的等待时间
  - 避免客户端快速重连导致频繁启停
  - 默认值：`5s`
  - 建议范围：5-10秒

- `quick_reconnect_window`:
  - 客户端断开后的快速重连窗口
  - 此时间内重连不会重启GStreamer
  - 默认值：`10s`
  - 建议范围：10-30秒

### WebRTC Session Configuration (可选)

WebRTC会话配置控制连接管理和重连行为。

```yaml
webrtc:
  session:
    keep_alive_timeout: 30s    # ICE断开后继续推流的时间（默认：30秒）
```

**参数说明：**

- `keep_alive_timeout`:
  - WebRTC连接彻底断开后继续推流的时间
  - 给客户端网络恢复的机会
  - 默认值：`30s`
  - 建议范围：30-60秒
  - 说明：30秒后仍未恢复则停止推流

**使用场景：**

1. **网络不稳定环境**：增加 `keep_alive_timeout` 到 60秒
2. **频繁重连场景**：增加 `idle_timeout` 到 10秒
3. **资源受限环境**：禁用按需推流（`enabled: false`）

**详细文档：** 参见 `docs/gstreamer-on-demand-streaming.md`

## Common Configuration Issues

### 1. Port Conflicts

**Issue**: Web server and metrics ports conflict
**Solution**: Update port numbers in the configuration

```yaml
webserver:
  port: 8080
metrics:
  external:
    port: 9090  # Different from web server port
```

### 2. Missing ICE Servers

**Issue**: WebRTC configuration missing ICE servers
**Solution**: Add STUN servers

```yaml
webrtc:
  ice_servers:
    - urls: ["stun:stun.l.google.com:19302"]
    - urls: ["stun:stun1.l.google.com:19302"]
```

### 3. Invalid Video Resolution

**Issue**: Video resolution not supported by system
**Solution**: Use standard resolutions

```yaml
gstreamer:
  capture:
    width: 1920   # Standard 1080p
    height: 1080
    framerate: 30
```

### 4. Hardware Encoding Unavailable

**Issue**: Hardware encoding enabled but not available
**Solution**: Disable hardware encoding

```yaml
gstreamer:
  encoding:
    codec: "h264"
    use_hardware: false  # Use software encoding
```

### 5. Display Access Issues

**Issue**: Cannot access display for screen capture
**Solution**: Configure virtual display or fix permissions

```yaml
gstreamer:
  capture:
    display_id: ":99"  # Use virtual display
    use_software_rendering: true
```

## Configuration Validation

### Automatic Validation

The application automatically validates configuration on startup:

```bash
# Start with configuration validation
./bdwind-gstreamer --config config.yaml

# Check logs for validation results
tail -f .tmp/bdwind-gstreamer.log | grep -i "config\|validation"
```

### Manual Validation

```go
// Validate configuration programmatically
cfg, err := config.LoadConfigFromFile("config.yaml")
if err != nil {
    log.Fatalf("Failed to load config: %v", err)
}

if err := cfg.Validate(); err != nil {
    log.Fatalf("Configuration validation failed: %v", err)
}
```

## Best Practices

### 1. Use Version Control
- Commit configuration changes to version control
- Tag releases with configuration information
- Document configuration decisions

### 2. Test Configuration Changes
- Test in development environment first
- Verify all functionality works as expected
- Monitor performance and resource usage

### 3. Monitor After Changes
- Check application logs for errors
- Monitor performance metrics
- Verify client connectivity

### 4. Keep Configurations Simple
- Use minimal configuration when possible
- Avoid unnecessary complexity
- Document custom settings

## Troubleshooting

### Application Issues

**Problem**: Application fails to start
**Solution**: Check logs and validate configuration

**Problem**: Poor video quality
**Solution**: Adjust bitrate and encoding settings

**Problem**: WebRTC connections fail
**Solution**: Verify ICE server configuration and network connectivity

**Problem**: High CPU usage
**Solution**: Enable hardware encoding if available, or reduce video quality

## Support

For additional help with configuration migration:

1. Check the application logs for detailed error messages
2. Use the validation tool to identify configuration issues
3. Review the generated migration guides
4. Test with example configurations
5. Consult the project documentation for specific configuration options