# 按需推流功能使用指南

## 概述

按需推流（On-Demand Streaming）功能允许 bdwind-gstreamer 只在有客户端连接时才启动视频捕获和编码，从而节省系统资源。

## 功能特性

### 1. 按需启动
- 应用启动时，GStreamer 处于空闲状态，不占用资源
- 当第一个客户端连接时，自动启动视频捕获和编码
- 推流与 ICE 连接建立同步，减少延迟

### 2. 自动停止
- 当最后一个客户端断开连接后，延迟一段时间（默认5秒）停止推流
- 避免客户端快速重连导致频繁启停

### 3. 连接恢复
- ICE 连接断开后，继续推流并等待重连（默认30秒）
- 给客户端网络恢复的机会
- 超时后自动停止推流，清理资源

### 4. 快速重连
- 客户端断开后短时间内（默认10秒）重连，不重启 GStreamer
- 减少重连延迟

## 配置参数

### GStreamer 配置

```yaml
gstreamer:
  on_demand:
    enabled: true                      # 是否启用按需启动（默认：true）
    idle_timeout: 5s                   # 无客户端后多久停止推流（默认：5秒）
    quick_reconnect_window: 10s        # 快速重连窗口（默认：10秒）
```

**参数说明：**

- `enabled`: 
  - `true`: 按需启动，有客户端才推流（推荐）
  - `false`: 启动即推流，保持旧行为

- `idle_timeout`: 
  - 最后一个客户端离开后的等待时间
  - 避免快速重连导致频繁启停
  - 建议：5-10秒

- `quick_reconnect_window`:
  - 客户端断开后的快速重连窗口
  - 此时间内重连不会重启 GStreamer
  - 建议：10-30秒

### WebRTC 配置

```yaml
webrtc:
  session:
    keep_alive_timeout: 30s            # ICE断开后继续推流的时间（默认：30秒）
```

**参数说明：**

- `keep_alive_timeout`:
  - WebRTC 连接彻底断开后继续推流的时间
  - 给客户端网络恢复的机会
  - 建议：30-60秒

## 使用示例

### 1. 启用按需推流（推荐）

```yaml
# config.yaml
gstreamer:
  on_demand:
    enabled: true
    idle_timeout: 5s
    quick_reconnect_window: 10s

webrtc:
  session:
    keep_alive_timeout: 30s
```

**行为：**
- 应用启动后，GStreamer 不启动
- 客户端连接 → GStreamer 启动 → 开始推流
- 客户端断开 → 等待5秒 → 无新连接 → GStreamer 停止

### 2. 禁用按需推流（传统模式）

```yaml
# config.yaml
gstreamer:
  on_demand:
    enabled: false
```

**行为：**
- 应用启动后，GStreamer 立即启动
- 持续推流，无论是否有客户端连接

### 3. 调整超时参数

```yaml
# config.yaml
gstreamer:
  on_demand:
    enabled: true
    idle_timeout: 10s                  # 延长空闲超时到10秒
    quick_reconnect_window: 30s        # 延长快速重连窗口到30秒

webrtc:
  session:
    keep_alive_timeout: 60s            # 延长保活超时到60秒
```

**适用场景：**
- 网络不稳定的环境
- 客户端可能频繁重连
- 需要更长的恢复时间

## 工作流程

### 正常连接流程

```
1. 应用启动
   └─ GStreamer: Idle（空闲）
   └─ WebRTC: 等待连接

2. 客户端连接
   └─ 发布 webrtc.session.started 事件
   └─ GStreamer: Starting → Streaming
   └─ 开始视频捕获和编码

3. ICE 连接建立
   └─ 发布 webrtc.session.ready 事件
   └─ 开始传输视频数据

4. 客户端断开
   └─ 发布 webrtc.session.ended 事件
   └─ 启动空闲定时器（5秒）

5. 空闲超时
   └─ 发布 webrtc.no_active_sessions 事件
   └─ GStreamer: Stopping → Idle
```

### ICE 重连流程

```
1. ICE 连接断开
   └─ 发布 webrtc.session.paused 事件
   └─ 启动重连定时器（30秒）
   └─ GStreamer 继续推流

2a. 重连成功（30秒内）
    └─ 发布 webrtc.session.resumed 事件
    └─ 取消重连定时器
    └─ 继续推流

2b. 重连超时（30秒后）
    └─ 发布 webrtc.session.timeout 事件
    └─ GStreamer: Stopping → Idle
```

### 快速重连流程

```
1. 客户端断开
   └─ 发布 webrtc.session.ended 事件
   └─ 启动空闲定时器（5秒）
   └─ GStreamer 继续推流

2. 客户端重连（5秒内）
   └─ 取消空闲定时器
   └─ 发布 webrtc.session.started 事件
   └─ GStreamer 继续推流（不重启）
```

## 日志示例

### 启动日志

```
INFO  GStreamer on-demand mode enabled, will start when client connects
INFO  WebRTC manager started successfully
INFO  Webserver manager started successfully
INFO  🚀 Go-gst BDWind-GStreamer started successfully!
```

### 客户端连接日志

```
INFO  Client connected: abc123
INFO  WebRTC session started (session=abc123), starting GStreamer...
INFO  GStreamer state changed: Idle -> Starting
INFO  GStreamer state changed: Starting -> Streaming
INFO  GoGst manager started successfully
INFO  🎬 First sample received from appsink, video pipeline is working
INFO  WebRTC session ready (session=abc123), streaming active
```

### 客户端断开日志

```
INFO  Client disconnected: abc123
INFO  Session removed (session=abc123), active sessions: 0
INFO  No active sessions, will stop streaming after 5s
INFO  Idle timeout reached, publishing no active sessions event
INFO  No active WebRTC sessions (idle=5s), stopping GStreamer...
INFO  GStreamer state changed: Streaming -> Stopping
INFO  GStreamer state changed: Stopping -> Idle
INFO  GoGst manager stopped successfully
```

### ICE 重连日志

```
INFO  ICE connection state changed: disconnected
INFO  ICE disconnected, will wait 30s for reconnection
INFO  ICE connection state changed: connected
INFO  WebRTC session ready (session=abc123), streaming active
```

## 监控和调试

### 查看 GStreamer 状态

```bash
curl http://localhost:8080/api/status
```

响应示例：

```json
{
  "gstreamer": {
    "running": true,
    "state": "Streaming",
    "frame_count": 1234,
    "uptime": 45.6
  }
}
```

### 查看 WebRTC 会话

```bash
curl http://localhost:8080/api/signaling/sessions
```

响应示例：

```json
{
  "active_sessions": 1,
  "sessions": [
    {
      "id": "abc123",
      "connected_at": "2025-11-21T10:00:00Z",
      "state": "connected"
    }
  ]
}
```

### 查看事件总线统计

```bash
curl http://localhost:8080/api/events/stats
```

响应示例：

```json
{
  "running": true,
  "event_types": 8,
  "total_handlers": 12,
  "handlers_by_type": {
    "webrtc.session.started": 1,
    "webrtc.session.ready": 1,
    "webrtc.session.timeout": 1,
    "webrtc.no_active_sessions": 1
  }
}
```

## 性能优化建议

### 1. 调整空闲超时

**场景：** 客户端频繁连接/断开

```yaml
gstreamer:
  on_demand:
    idle_timeout: 30s  # 延长到30秒
```

**效果：** 减少 GStreamer 启停次数，提高稳定性

### 2. 调整保活超时

**场景：** 网络不稳定，经常出现短暂断开

```yaml
webrtc:
  session:
    keep_alive_timeout: 60s  # 延长到60秒
```

**效果：** 给网络更多恢复时间，减少不必要的停止

### 3. 禁用按需启动

**场景：** 需要持续推流，客户端随时连接

```yaml
gstreamer:
  on_demand:
    enabled: false
```

**效果：** 客户端连接时无延迟，但会持续占用资源

## 故障排查

### 问题1：客户端连接后无视频

**可能原因：** GStreamer 启动失败

**排查步骤：**
1. 查看日志：`grep "GStreamer" .tmp/bdwind-gstreamer.log`
2. 检查显示环境：`echo $DISPLAY`
3. 确认 Xvfb 运行：`ps aux | grep Xvfb`

**解决方案：**
```bash
# 启动 Xvfb
Xvfb :99 -screen 0 1920x1080x24 -ac &
export DISPLAY=:99
```

### 问题2：客户端断开后 GStreamer 未停止

**可能原因：** 空闲超时未触发

**排查步骤：**
1. 查看活跃会话：`curl http://localhost:8080/api/signaling/sessions`
2. 检查事件总线：`curl http://localhost:8080/api/events/stats`

**解决方案：**
- 确认所有客户端已断开
- 等待 `idle_timeout` 时间
- 检查日志中的 `no_active_sessions` 事件

### 问题3：ICE 重连失败

**可能原因：** 网络问题或超时设置过短

**排查步骤：**
1. 查看 ICE 状态：`grep "ICE connection state" .tmp/bdwind-gstreamer.log`
2. 检查网络连接
3. 查看 `keep_alive_timeout` 配置

**解决方案：**
```yaml
webrtc:
  session:
    keep_alive_timeout: 60s  # 延长超时时间
```

## 最佳实践

1. **生产环境推荐配置：**
   ```yaml
   gstreamer:
     on_demand:
       enabled: true
       idle_timeout: 10s
       quick_reconnect_window: 30s
   
   webrtc:
     session:
       keep_alive_timeout: 60s
   ```

2. **开发环境推荐配置：**
   ```yaml
   gstreamer:
     on_demand:
       enabled: false  # 禁用按需启动，方便调试
   ```

3. **监控建议：**
   - 监控 GStreamer 状态变化
   - 监控活跃会话数量
   - 监控 ICE 连接状态
   - 设置告警：GStreamer 启动失败、ICE 连接频繁断开

4. **日志级别：**
   - 生产环境：`info`
   - 调试环境：`debug`

## 相关文档

- [GStreamer 故障排查](./gstreamer-troubleshooting.md)
- [WebRTC 配置指南](./webrtc.md)
- [事件系统设计](./gstreamer-tech-design.md)

---

*最后更新：2025-11-21*
