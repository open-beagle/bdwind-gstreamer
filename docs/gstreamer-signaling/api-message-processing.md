# WebRTC 信令服务器消息处理 API 文档

## 概述

本文档详细说明了 WebRTC 信令服务器的消息处理流程，包括 `request-offer` 和 `ping` 消息的处理机制、错误处理和最佳实践。

## 消息处理流程

### 1. 消息接收和路由

信令服务器通过以下流程处理客户端消息：

```
客户端消息 → WebSocket 接收 → 协议检测 → 消息解析 → 消息路由 → 处理器执行 → 响应发送
```

#### 1.1 协议检测

服务器支持多种协议格式：

- **标准 JSON 协议**: 基于 GStreamer 规范的结构化消息
- **Selkies 兼容协议**: 支持 selkies-gstreamer 客户端
- **自动检测**: 根据消息格式自动选择协议处理器

```go
func (c *SignalingClient) handleMessageWithRouter(messageBytes []byte) {
    // 记录原始消息
    log.Printf("📨 Raw message from client %s: %s", c.ID, string(messageBytes))
    
    // 协议自动检测
    if c.MessageCount == 1 && !c.ProtocolDetected {
        c.autoDetectProtocol(messageBytes)
    }
    
    // 消息路由处理
    routeResult, err := c.Server.messageRouter.RouteMessage(messageBytes, c.ID)
    if err != nil {
        // 回退到原始 JSON 解析
        c.handleRawJSONMessage(messageBytes)
        return
    }
    
    // 处理标准化消息
    c.handleStandardMessage(routeResult.Message, routeResult.OriginalProtocol)
}
```

#### 1.2 消息路由

消息路由器根据消息类型分发到相应的处理器：

```go
func (c *SignalingClient) handleStandardMessage(message *protocol.StandardMessage, originalProtocol protocol.ProtocolVersion) {
    switch message.Type {
    case protocol.MessageTypeRequestOffer:
        c.handleRequestOfferMessage(message)
    case protocol.MessageTypePing:
        c.handlePingMessage(message)
    case protocol.MessageTypeHello:
        c.handleHelloMessage(message)
    // ... 其他消息类型
    default:
        c.sendStandardErrorMessage("UNSUPPORTED_MESSAGE_TYPE", 
            fmt.Sprintf("Message type '%s' is not supported", message.Type), "")
    }
}
```

### 2. Request-Offer 消息处理

#### 2.1 消息格式

**客户端请求格式**:
```json
{
  "type": "request-offer",
  "peer_id": "client_001",
  "message_id": "msg_1640995240000_003",
  "timestamp": 1640995240000,
  "data": {
    "constraints": {
      "video": true,
      "audio": true,
      "data_channel": true
    },
    "codec_preferences": ["H264", "VP8", "VP9"]
  }
}
```

**服务器响应格式**:
```json
{
  "type": "offer",
  "peer_id": "client_001",
  "message_id": "msg_1640995240000_003",
  "timestamp": 1640995240001,
  "data": {
    "type": "offer",
    "sdp": "v=0\r\no=- 4611731400430051336 2 IN IP4 127.0.0.1\r\n..."
  }
}
```

#### 2.2 处理流程

```go
func (c *SignalingClient) handleRequestOfferMessage(message *protocol.StandardMessage) {
    log.Printf("📞 Processing request-offer from client %s", c.ID)
    
    // 1. 解析请求数据
    var requestData map[string]any
    if message.Data != nil {
        if data, ok := message.Data.(map[string]any); ok {
            requestData = data
        }
    }
    
    // 2. 记录请求详情
    if requestData != nil {
        if constraints, exists := requestData["constraints"]; exists {
            log.Printf("📋 Request constraints: %+v", constraints)
        }
    }
    
    // 3. 检查 PeerConnection 管理器
    if c.Server.peerConnectionManager == nil {
        c.sendStandardErrorMessage("PEER_CONNECTION_UNAVAILABLE",
            "PeerConnection manager is not available", message.MessageID)
        return
    }
    
    // 4. 创建或获取 PeerConnection
    pc, err := c.Server.peerConnectionManager.CreatePeerConnection(c.ID)
    if err != nil {
        c.sendStandardErrorMessage("PEER_CONNECTION_CREATION_FAILED",
            "Failed to create PeerConnection", err.Error())
        return
    }
    
    // 5. 创建 SDP Offer
    offer, err := pc.CreateOffer(nil)
    if err != nil {
        c.sendStandardErrorMessage("OFFER_CREATION_FAILED",
            "Failed to create SDP offer", err.Error())
        return
    }
    
    // 6. 设置本地描述
    if err := pc.SetLocalDescription(offer); err != nil {
        c.sendStandardErrorMessage("LOCAL_DESCRIPTION_FAILED",
            "Failed to set local description", err.Error())
        return
    }
    
    // 7. 发送 offer 响应
    response := SignalingMessage{
        Type:      "offer",
        PeerID:    c.ID,
        MessageID: message.MessageID,
        Timestamp: time.Now().Unix(),
        Data: map[string]any{
            "type": offer.Type.String(),
            "sdp":  offer.SDP,
        },
    }
    
    if err := c.sendMessage(response); err != nil {
        log.Printf("❌ Failed to send offer: %v", err)
    } else {
        log.Printf("✅ Offer sent successfully")
    }
}
```

#### 2.3 错误处理

常见错误情况和处理方式：

| 错误类型 | 错误代码 | 处理方式 |
|---------|---------|---------|
| PeerConnection 管理器不可用 | `PEER_CONNECTION_UNAVAILABLE` | 检查服务器配置 |
| PeerConnection 创建失败 | `PEER_CONNECTION_CREATION_FAILED` | 重试或重启服务 |
| SDP Offer 创建失败 | `OFFER_CREATION_FAILED` | 检查媒体配置 |
| 本地描述设置失败 | `LOCAL_DESCRIPTION_FAILED` | 检查 SDP 格式 |

### 3. Ping 消息处理

#### 3.1 消息格式

**客户端 Ping 格式**:
```json
{
  "type": "ping",
  "peer_id": "client_001",
  "message_id": "msg_1640995230000_002",
  "timestamp": 1640995230000,
  "data": {
    "client_state": "connected",
    "sequence": 1
  }
}
```

**服务器 Pong 响应格式**:
```json
{
  "type": "pong",
  "peer_id": "client_001",
  "message_id": "msg_1640995230000_002",
  "timestamp": 1640995230001,
  "data": {
    "timestamp": 1640995230001,
    "server_time": 1640995230001,
    "client_id": "client_001",
    "ping_timestamp": 1640995230000,
    "client_state": "connected"
  }
}
```

#### 3.2 处理流程

```go
func (c *SignalingClient) handlePingMessage(message *protocol.StandardMessage) {
    log.Printf("🏓 Processing ping from client %s", c.ID)
    
    // 1. 解析 ping 数据
    var pingData map[string]any
    if message.Data != nil {
        if data, ok := message.Data.(map[string]any); ok {
            pingData = data
        }
    }
    
    // 2. 构造 pong 响应数据
    pongData := map[string]any{
        "timestamp":   time.Now().Unix(),
        "server_time": time.Now().Unix(),
        "client_id":   c.ID,
    }
    
    // 3. 包含原始时间戳用于延迟计算
    if pingData != nil {
        if clientTimestamp, exists := pingData["timestamp"]; exists {
            pongData["ping_timestamp"] = clientTimestamp
        }
        if clientState, exists := pingData["client_state"]; exists {
            pongData["client_state"] = clientState
        }
    }
    
    // 4. 发送 pong 响应
    response := SignalingMessage{
        Type:      "pong",
        PeerID:    c.ID,
        MessageID: message.MessageID,
        Timestamp: time.Now().Unix(),
        Data:      pongData,
    }
    
    if err := c.sendMessage(response); err != nil {
        log.Printf("❌ Failed to send pong: %v", err)
    } else {
        log.Printf("✅ Pong sent successfully")
    }
}
```

#### 3.3 心跳监控

服务器使用 ping/pong 消息进行连接健康监控：

```go
func (s *SignalingServer) startHeartbeatMonitoring() {
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()
    
    for {
        select {
        case <-ticker.C:
            s.checkClientHeartbeats()
        case <-s.ctx.Done():
            return
        }
    }
}

func (s *SignalingServer) checkClientHeartbeats() {
    s.mutex.RLock()
    defer s.mutex.RUnlock()
    
    now := time.Now()
    timeout := 60 * time.Second
    
    for _, client := range s.clients {
        if now.Sub(client.LastSeen) > timeout {
            log.Printf("⚠️ Client %s heartbeat timeout", client.ID)
            s.unregister <- client
        }
    }
}
```

### 4. 错误处理机制

#### 4.1 错误消息格式

```json
{
  "type": "error",
  "peer_id": "client_001",
  "message_id": "msg_1640995310000_010",
  "timestamp": 1640995310000,
  "error": {
    "code": "INVALID_SDP",
    "message": "SDP format is invalid",
    "details": "Missing required 'v=' line in SDP",
    "type": "validation_error",
    "recoverable": true
  }
}
```

#### 4.2 错误处理函数

```go
func (c *SignalingClient) sendStandardErrorMessage(code, message, details string) {
    errorResponse := SignalingMessage{
        Type:      "error",
        PeerID:    c.ID,
        MessageID: generateMessageID(),
        Timestamp: time.Now().Unix(),
        Error: &SignalingError{
            Code:        code,
            Message:     message,
            Details:     details,
            Type:        "server_error",
            Recoverable: true,
        },
    }
    
    if err := c.sendMessage(errorResponse); err != nil {
        log.Printf("❌ Failed to send error message: %v", err)
    }
}
```

#### 4.3 常见错误代码

| 错误代码 | 描述 | 恢复建议 |
|---------|------|---------|
| `MESSAGE_PARSING_FAILED` | 消息解析失败 | 检查消息格式 |
| `UNSUPPORTED_MESSAGE_TYPE` | 不支持的消息类型 | 使用支持的消息类型 |
| `PEER_CONNECTION_UNAVAILABLE` | PeerConnection 不可用 | 重新连接 |
| `OFFER_CREATION_FAILED` | Offer 创建失败 | 检查媒体配置 |
| `CONNECTION_TIMEOUT` | 连接超时 | 重新建立连接 |

### 5. 性能监控

#### 5.1 消息处理指标

```go
type MessageProcessingMetrics struct {
    TotalMessages     int64         `json:"total_messages"`
    ProcessingTime    time.Duration `json:"processing_time"`
    ErrorCount        int64         `json:"error_count"`
    SuccessRate       float64       `json:"success_rate"`
    AverageLatency    time.Duration `json:"average_latency"`
}

func (c *SignalingClient) recordMessageMetrics(messageType string, processingTime time.Duration, success bool) {
    c.MessageCount++
    
    if !success {
        c.ErrorCount++
    }
    
    // 记录到监控系统
    if c.Server.metricsCollector != nil {
        c.Server.metricsCollector.RecordMessageProcessing(messageType, processingTime, success)
    }
}
```

#### 5.2 连接状态监控

```go
func (s *SignalingServer) getServerStatus() ServerStatus {
    s.mutex.RLock()
    defer s.mutex.RUnlock()
    
    return ServerStatus{
        Running:           s.running,
        ClientCount:       len(s.clients),
        AppCount:          len(s.apps),
        TotalMessages:     s.getTotalMessageCount(),
        ErrorRate:         s.getErrorRate(),
        AverageLatency:    s.getAverageLatency(),
        UptimeSeconds:     int64(time.Since(s.startTime).Seconds()),
    }
}
```

### 6. 最佳实践

#### 6.1 消息处理优化

1. **异步处理**: 使用 goroutine 处理耗时操作
2. **批量处理**: 对于高频消息使用批量处理
3. **连接池**: 复用 WebRTC 连接资源
4. **缓存机制**: 缓存常用的 SDP 模板

#### 6.2 错误恢复策略

1. **重试机制**: 对临时错误实施指数退避重试
2. **降级处理**: 在部分功能失败时提供基础服务
3. **状态恢复**: 保存关键状态以支持快速恢复
4. **监控告警**: 及时发现和处理异常情况

#### 6.3 安全考虑

1. **输入验证**: 严格验证所有客户端输入
2. **速率限制**: 防止消息洪水攻击
3. **连接限制**: 限制单个客户端的连接数
4. **日志审计**: 记录关键操作用于安全审计

## API 端点

### WebSocket 端点

- `GET /ws` - 通用信令连接
- `GET /ws/{app}` - 应用特定信令连接

### REST API 端点

- `GET /api/signaling/status` - 获取服务器状态
- `GET /api/signaling/clients` - 获取客户端列表
- `POST /api/signaling/broadcast` - 广播消息

## 配置参数

```yaml
signaling:
  host: "0.0.0.0"
  port: 8080
  max_connections: 1000
  connection_timeout: "60s"
  heartbeat_interval: "30s"
  cleanup_interval: "5m"
  max_message_size: 65536
  supported_protocols: ["gstreamer-1.0", "selkies"]
  default_protocol: "gstreamer-1.0"
```

## 故障排除

### 常见问题

1. **消息处理失败**: 检查消息格式和协议版本
2. **连接频繁断开**: 检查网络稳定性和心跳配置
3. **Offer 创建失败**: 检查 WebRTC 配置和媒体设备
4. **性能问题**: 监控消息处理延迟和资源使用

### 调试工具

1. **日志分析**: 使用结构化日志进行问题定位
2. **性能监控**: 监控消息处理时间和成功率
3. **连接状态**: 实时查看客户端连接状态
4. **协议分析**: 分析消息协议兼容性问题