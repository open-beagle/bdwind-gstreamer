# WebRTC 信令服务器故障排除指南

## 概述

本指南提供了 WebRTC 信令服务器常见问题的诊断和解决方案，包括连接问题、消息处理错误、性能问题和配置错误的排查方法。

## 常见问题分类

### 1. 连接问题

#### 1.1 WebSocket 连接失败

**症状**:
- 客户端无法连接到服务器
- 连接立即断开
- 连接超时

**可能原因**:
- 服务器未启动或端口被占用
- 防火墙阻止连接
- 网络配置问题
- SSL/TLS 证书问题（wss://）

**诊断步骤**:

```bash
# 1. 检查服务器是否运行
ps aux | grep bdwind-gstreamer
netstat -tlnp | grep :8080

# 2. 测试端口连通性
telnet localhost 8080
nc -zv localhost 8080

# 3. 检查防火墙规则
sudo ufw status
sudo iptables -L

# 4. 测试 WebSocket 连接
curl -i -N -H "Connection: Upgrade" \
     -H "Upgrade: websocket" \
     -H "Sec-WebSocket-Key: SGVsbG8sIHdvcmxkIQ==" \
     -H "Sec-WebSocket-Version: 13" \
     http://localhost:8080/ws
```

**解决方案**:

```yaml
# 配置文件检查 (config.yaml)
webserver:
  host: "0.0.0.0"  # 确保绑定到正确的地址
  port: 8080       # 确保端口未被占用
  
webrtc:
  signaling:
    enabled: true
    max_connections: 1000
```

```bash
# 启动服务器并检查日志
./bdwind-gstreamer --config config.yaml --log-level debug

# 检查端口占用
sudo lsof -i :8080
```

#### 1.2 连接频繁断开

**症状**:
- 连接建立后很快断开
- 心跳超时
- 间歇性连接丢失

**可能原因**:
- 网络不稳定
- 心跳配置不当
- 服务器资源不足
- 客户端实现问题

**诊断步骤**:

```bash
# 检查服务器日志
tail -f /var/log/bdwind-gstreamer.log | grep -E "(disconnect|timeout|error)"

# 监控网络连接
ss -tuln | grep :8080
watch -n 1 'ss -tuln | grep :8080'

# 检查系统资源
top -p $(pgrep bdwind-gstreamer)
free -h
df -h
```

**解决方案**:

```yaml
# 调整心跳和超时配置
webrtc:
  signaling:
    heartbeat_interval: "30s"
    connection_timeout: "60s"
    cleanup_interval: "5m"
    max_message_size: 65536
```

```go
// 客户端心跳实现
func (c *SignalingClient) startHeartbeat() {
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()
    
    for {
        select {
        case <-ticker.C:
            if err := c.sendPing(); err != nil {
                log.Printf("Heartbeat failed: %v", err)
                return
            }
        case <-c.stopChan:
            return
        }
    }
}
```

### 2. 消息处理问题

#### 2.1 Request-Offer 消息无响应

**症状**:
- 客户端发送 request-offer 但收不到 offer 响应
- 服务器日志显示收到消息但无处理记录
- WebRTC 连接无法建立

**诊断步骤**:

```bash
# 启用详细日志
export LOG_LEVEL=debug
./bdwind-gstreamer --config config.yaml

# 检查消息处理日志
grep -E "(request-offer|handleRequestOfferMessage)" /var/log/bdwind-gstreamer.log

# 检查 PeerConnection 管理器状态
curl http://localhost:8080/api/signaling/status
```

**可能原因和解决方案**:

1. **消息路由问题**:
```go
// 检查消息路由器配置
func (c *SignalingClient) handleStandardMessage(message *protocol.StandardMessage) {
    log.Printf("📨 Processing message type: %s", message.Type)
    
    switch message.Type {
    case protocol.MessageTypeRequestOffer:
        c.handleRequestOfferMessage(message)  // 确保此行存在
    // ...
    }
}
```

2. **PeerConnection 管理器未初始化**:
```go
// 确保 PeerConnection 管理器已初始化
func (s *SignalingServer) Start() error {
    if s.peerConnectionManager == nil {
        s.peerConnectionManager = NewPeerConnectionManager()
    }
    // ...
}
```

3. **消息格式问题**:
```json
// 正确的 request-offer 格式
{
  "type": "request-offer",
  "peer_id": "client_001",
  "message_id": "msg_123",
  "timestamp": 1640995240000,
  "data": {
    "constraints": {
      "video": true,
      "audio": true
    }
  }
}
```

#### 2.2 Ping 消息无 Pong 响应

**症状**:
- 客户端发送 ping 但收不到 pong
- 连接状态检测失效
- 心跳机制不工作

**诊断步骤**:

```bash
# 检查 ping/pong 消息日志
grep -E "(ping|pong|handlePingMessage)" /var/log/bdwind-gstreamer.log

# 使用 WebSocket 客户端测试
wscat -c ws://localhost:8080/ws
# 发送: {"type":"ping","peer_id":"test","message_id":"test_123","timestamp":1640995240000}
```

**解决方案**:

```go
// 确保 ping 处理器正确实现
func (c *SignalingClient) handlePingMessage(message *protocol.StandardMessage) {
    log.Printf("🏓 Processing ping from client %s", c.ID)
    
    // 构造 pong 响应
    pongData := map[string]any{
        "timestamp":   time.Now().Unix(),
        "server_time": time.Now().Unix(),
        "client_id":   c.ID,
    }
    
    // 包含原始时间戳
    if message.Data != nil {
        if data, ok := message.Data.(map[string]any); ok {
            if timestamp, exists := data["timestamp"]; exists {
                pongData["ping_timestamp"] = timestamp
            }
        }
    }
    
    response := SignalingMessage{
        Type:      "pong",
        PeerID:    c.ID,
        MessageID: message.MessageID,
        Timestamp: time.Now().Unix(),
        Data:      pongData,
    }
    
    if err := c.sendMessage(response); err != nil {
        log.Printf("❌ Failed to send pong: %v", err)
    }
}
```

#### 2.3 消息解析错误

**症状**:
- 服务器日志显示消息解析失败
- 客户端收到格式错误响应
- 协议兼容性问题

**诊断步骤**:

```bash
# 检查消息解析错误
grep -E "(parse|unmarshal|json)" /var/log/bdwind-gstreamer.log

# 验证消息格式
echo '{"type":"ping","peer_id":"test"}' | jq .
```

**解决方案**:

```go
// 增强消息解析错误处理
func (c *SignalingClient) handleMessageWithRouter(messageBytes []byte) {
    log.Printf("📨 Raw message: %s", string(messageBytes))
    
    // 尝试标准 JSON 解析
    var jsonMessage SignalingMessage
    if err := json.Unmarshal(messageBytes, &jsonMessage); err == nil {
        c.handleMessage(jsonMessage)
        return
    }
    
    // 使用消息路由器
    routeResult, err := c.Server.messageRouter.RouteMessage(messageBytes, c.ID)
    if err != nil {
        log.Printf("❌ Message routing failed: %v", err)
        
        // 回退处理
        if err := c.handleRawJSONMessage(messageBytes); err != nil {
            log.Printf("❌ Fallback parsing failed: %v", err)
            c.sendStandardErrorMessage("MESSAGE_PARSING_FAILED", 
                "Failed to parse message", err.Error())
        }
        return
    }
    
    c.handleStandardMessage(routeResult.Message, routeResult.OriginalProtocol)
}
```

### 3. WebRTC 连接问题

#### 3.1 SDP Offer 创建失败

**症状**:
- request-offer 处理时 SDP 创建失败
- 客户端收到错误响应
- WebRTC 协商无法开始

**诊断步骤**:

```bash
# 检查 WebRTC 相关错误
grep -E "(offer|sdp|webrtc)" /var/log/bdwind-gstreamer.log

# 检查 GStreamer 管道状态
gst-inspect-1.0 webrtcbin
gst-launch-1.0 --version
```

**可能原因和解决方案**:

1. **GStreamer 配置问题**:
```yaml
gstreamer:
  pipeline:
    video_source: "videotestsrc"
    audio_source: "audiotestsrc"
    video_encoder: "x264enc"
    audio_encoder: "opusenc"
```

2. **WebRTC 配置问题**:
```yaml
webrtc:
  ice_servers:
    - urls: ["stun:stun.l.google.com:19302"]
  video_codecs: ["H264", "VP8", "VP9"]
  audio_codecs: ["OPUS", "PCMU"]
```

3. **权限问题**:
```bash
# 检查设备权限
ls -la /dev/video*
groups $USER
# 添加用户到 video 组
sudo usermod -a -G video $USER
```

#### 3.2 ICE 连接失败

**症状**:
- SDP 协商成功但媒体流无法传输
- ICE 连接状态为 failed
- 网络连通性问题

**诊断步骤**:

```bash
# 测试 STUN 服务器连通性
stunclient stun.l.google.com 19302

# 检查网络配置
ip route show
ip addr show

# 测试 UDP 连通性
nc -u -l 12345  # 在一个终端
nc -u localhost 12345  # 在另一个终端
```

**解决方案**:

```yaml
# 配置多个 ICE 服务器
webrtc:
  ice_servers:
    - urls: ["stun:stun.l.google.com:19302"]
    - urls: ["stun:stun1.l.google.com:19302"]
    - urls: ["turn:your-turn-server.com:3478"]
      username: "user"
      credential: "pass"
```

### 4. 性能问题

#### 4.1 高延迟

**症状**:
- ping/pong 延迟过高
- 消息处理缓慢
- 用户体验差

**诊断步骤**:

```bash
# 监控系统性能
top -p $(pgrep bdwind-gstreamer)
iostat -x 1
sar -u 1

# 检查网络延迟
ping localhost
traceroute localhost

# 分析日志中的时间戳
grep -E "timestamp.*latency" /var/log/bdwind-gstreamer.log
```

**解决方案**:

```yaml
# 优化配置
webrtc:
  signaling:
    heartbeat_interval: "15s"  # 减少心跳间隔
    max_message_size: 32768    # 减少消息大小限制
    
gstreamer:
  buffer_size: 1024           # 减少缓冲区大小
  latency: "low"              # 设置低延迟模式
```

```go
// 优化消息处理
func (c *SignalingClient) handleMessage(message SignalingMessage) {
    start := time.Now()
    defer func() {
        duration := time.Since(start)
        if duration > 100*time.Millisecond {
            log.Printf("⚠️ Slow message processing: %v for %s", duration, message.Type)
        }
    }()
    
    // 处理消息...
}
```

#### 4.2 内存泄漏

**症状**:
- 内存使用持续增长
- 服务器性能逐渐下降
- 最终导致 OOM

**诊断步骤**:

```bash
# 监控内存使用
watch -n 5 'ps -p $(pgrep bdwind-gstreamer) -o pid,ppid,cmd,%mem,%cpu,rss'

# 使用 pprof 分析（如果启用）
go tool pprof http://localhost:8080/debug/pprof/heap

# 检查文件描述符泄漏
lsof -p $(pgrep bdwind-gstreamer) | wc -l
```

**解决方案**:

```go
// 确保资源正确清理
func (c *SignalingClient) cleanup() {
    // 关闭通道
    if c.Send != nil {
        close(c.Send)
        c.Send = nil
    }
    
    // 清理 WebRTC 连接
    if c.peerConnection != nil {
        c.peerConnection.Close()
        c.peerConnection = nil
    }
    
    // 清理定时器
    if c.heartbeatTimer != nil {
        c.heartbeatTimer.Stop()
        c.heartbeatTimer = nil
    }
}

// 定期垃圾回收
func (s *SignalingServer) startGCRoutine() {
    ticker := time.NewTicker(5 * time.Minute)
    defer ticker.Stop()
    
    for range ticker.C {
        runtime.GC()
        debug.FreeOSMemory()
    }
}
```

### 5. 配置问题

#### 5.1 配置文件错误

**症状**:
- 服务器启动失败
- 功能异常
- 默认配置被使用

**诊断步骤**:

```bash
# 验证配置文件语法
./bdwind-gstreamer --config config.yaml --validate-config

# 检查配置文件权限
ls -la config.yaml

# 使用默认配置启动
./bdwind-gstreamer --generate-config > default-config.yaml
```

**解决方案**:

```yaml
# 完整配置示例
webserver:
  host: "0.0.0.0"
  port: 8080
  static_files: "./internal/webserver/static"

webrtc:
  signaling:
    enabled: true
    max_connections: 1000
    heartbeat_interval: "30s"
    connection_timeout: "60s"
  ice_servers:
    - urls: ["stun:stun.l.google.com:19302"]

gstreamer:
  pipeline:
    video_source: "videotestsrc"
    audio_source: "audiotestsrc"
  encoder:
    video_codec: "x264"
    audio_codec: "opus"
    bitrate: 2000000

logging:
  level: "info"
  format: "json"
  output: "/var/log/bdwind-gstreamer.log"
```

#### 5.2 环境变量配置

**症状**:
- 配置不生效
- 环境相关的问题

**解决方案**:

```bash
# 设置环境变量
export BDWIND_LOG_LEVEL=debug
export BDWIND_CONFIG_PATH=/etc/bdwind-gstreamer/config.yaml
export GST_DEBUG=3

# 检查环境变量
env | grep -E "(BDWIND|GST)"

# 使用 systemd 服务
cat > /etc/systemd/system/bdwind-gstreamer.service << EOF
[Unit]
Description=BDWind GStreamer Service
After=network.target

[Service]
Type=simple
User=bdwind
Group=bdwind
Environment=GST_DEBUG=2
Environment=BDWIND_LOG_LEVEL=info
ExecStart=/usr/local/bin/bdwind-gstreamer --config /etc/bdwind-gstreamer/config.yaml
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF
```

## 调试工具和技巧

### 1. 日志分析

```bash
# 实时监控日志
tail -f /var/log/bdwind-gstreamer.log | grep -E "(ERROR|WARN|ping|offer)"

# 分析错误模式
grep "ERROR" /var/log/bdwind-gstreamer.log | sort | uniq -c | sort -nr

# 提取性能指标
grep "latency" /var/log/bdwind-gstreamer.log | awk '{print $NF}' | sort -n
```

### 2. 网络调试

```bash
# 抓包分析
sudo tcpdump -i lo -w websocket.pcap port 8080
wireshark websocket.pcap

# WebSocket 消息监控
websocat ws://localhost:8080/ws --text
```

### 3. 性能分析

```bash
# CPU 性能分析
perf record -g ./bdwind-gstreamer --config config.yaml
perf report

# 内存分析
valgrind --tool=memcheck --leak-check=full ./bdwind-gstreamer
```

### 4. 自动化测试

```bash
#!/bin/bash
# 连接测试脚本
test_connection() {
    local url=$1
    local timeout=5
    
    if timeout $timeout wscat -c "$url" -x '{"type":"ping","peer_id":"test"}' 2>/dev/null; then
        echo "✅ Connection test passed: $url"
        return 0
    else
        echo "❌ Connection test failed: $url"
        return 1
    fi
}

# 测试不同端点
test_connection "ws://localhost:8080/ws"
test_connection "ws://localhost:8080/ws/test-app"
```

## 监控和告警

### 1. 健康检查端点

```go
// 健康检查处理器
func (s *SignalingServer) HandleHealth(w http.ResponseWriter, r *http.Request) {
    health := map[string]interface{}{
        "status":      "healthy",
        "timestamp":   time.Now().Unix(),
        "clients":     len(s.clients),
        "uptime":      time.Since(s.startTime).Seconds(),
        "memory_mb":   getMemoryUsage() / 1024 / 1024,
    }
    
    // 检查关键组件
    if s.peerConnectionManager == nil {
        health["status"] = "unhealthy"
        health["error"] = "PeerConnection manager not available"
        w.WriteHeader(http.StatusServiceUnavailable)
    }
    
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(health)
}
```

### 2. 指标收集

```yaml
# Prometheus 配置
global:
  scrape_interval: 15s

scrape_configs:
  - job_name: 'bdwind-gstreamer'
    static_configs:
      - targets: ['localhost:8080']
    metrics_path: '/metrics'
```

### 3. 告警规则

```yaml
# AlertManager 规则
groups:
  - name: bdwind-gstreamer
    rules:
      - alert: HighConnectionCount
        expr: bdwind_active_connections > 800
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High connection count detected"
          
      - alert: HighErrorRate
        expr: rate(bdwind_errors_total[5m]) > 0.1
        for: 2m
        labels:
          severity: critical
        annotations:
          summary: "High error rate detected"
```

## 常见错误代码参考

| 错误代码 | 描述 | 可能原因 | 解决方案 |
|---------|------|---------|---------|
| `CONNECTION_FAILED` | 连接失败 | 网络问题、服务器未启动 | 检查网络和服务器状态 |
| `MESSAGE_PARSING_FAILED` | 消息解析失败 | 格式错误、编码问题 | 验证消息格式 |
| `PEER_CONNECTION_UNAVAILABLE` | PeerConnection 不可用 | 管理器未初始化 | 检查服务器配置 |
| `OFFER_CREATION_FAILED` | Offer 创建失败 | GStreamer 问题 | 检查媒体配置 |
| `ICE_CANDIDATE_FAILED` | ICE 候选失败 | 网络配置问题 | 检查 STUN/TURN 配置 |
| `CONNECTION_TIMEOUT` | 连接超时 | 网络延迟、配置问题 | 调整超时配置 |

## 预防措施

### 1. 配置验证

```go
// 配置验证函数
func ValidateConfig(config *Config) error {
    if config.WebServer.Port <= 0 || config.WebServer.Port > 65535 {
        return fmt.Errorf("invalid port: %d", config.WebServer.Port)
    }
    
    if config.WebRTC.Signaling.MaxConnections <= 0 {
        return fmt.Errorf("max_connections must be positive")
    }
    
    if len(config.WebRTC.ICEServers) == 0 {
        return fmt.Errorf("at least one ICE server must be configured")
    }
    
    return nil
}
```

### 2. 资源限制

```yaml
# systemd 服务限制
[Service]
LimitNOFILE=65536
LimitNPROC=4096
MemoryLimit=2G
CPUQuota=200%
```

### 3. 监控脚本

```bash
#!/bin/bash
# 监控脚本
LOGFILE="/var/log/bdwind-monitor.log"
PIDFILE="/var/run/bdwind-gstreamer.pid"

check_process() {
    if ! pgrep -f bdwind-gstreamer > /dev/null; then
        echo "$(date): Process not running, restarting..." >> $LOGFILE
        systemctl restart bdwind-gstreamer
    fi
}

check_memory() {
    local mem_usage=$(ps -p $(cat $PIDFILE) -o %mem --no-headers 2>/dev/null)
    if (( $(echo "$mem_usage > 80" | bc -l) )); then
        echo "$(date): High memory usage: $mem_usage%" >> $LOGFILE
    fi
}

# 定期检查
while true; do
    check_process
    check_memory
    sleep 60
done
```

通过遵循这个故障排除指南，您可以快速诊断和解决 WebRTC 信令服务器的常见问题，确保系统的稳定运行。