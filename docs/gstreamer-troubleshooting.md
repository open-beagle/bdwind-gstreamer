# GStreamer 推流问题诊断

## 调试规则

⚠️ **重要：使用 `start.sh` 脚本调试只能由人类进行，AI禁止调试**

---

## 当前问题

*暂无待解决问题*

---

## 已解决问题

### 问题1：按需推流和连接状态管理 【已解决 ✅】

**解决时间：** 2025-11-21

**问题描述：** 应用启动即推流，浪费资源；ICE断开无重连机制；生命周期管理混乱

**解决方案：** 
- 实现基于事件的按需推流机制
- 添加GStreamer状态机（Idle/Starting/Streaming/Stopping）
- 实现ICE重连和超时管理
- 通过公共事件系统解耦组件

**核心实现：**
- 创建公共事件包 `internal/common/events/`
- 定义8个会话生命周期事件（`webrtc.session.*`）
- GStreamer订阅事件，按需启停
- WebRTC发布事件，管理会话状态

**配置参数：**
```yaml
gstreamer:
  on_demand:
    enabled: true               # 启用按需推流
    idle_timeout: 5s            # 空闲超时
    quick_reconnect_window: 10s # 快速重连窗口

webrtc:
  session:
    keep_alive_timeout: 30s     # 保活超时
```

**测试验证：** ✅ 已通过完整测试（基础功能、ICE重连、超时停止、多客户端、快速重连、配置测试）

**详细文档：** `docs/gstreamer-on-demand-streaming.md`

---

### 问题2：推的流看不到（黑屏问题）【已解决 ✅】

**问题：** WebRTC连接成功，但客户端显示黑屏

**根本原因：** H.264编码器缺少关键参数，导致浏览器无法解码

**解决方案：**
```go
// x264编码器参数
encoder.SetProperty("insert-vui", true)      // VUI信息
encoder.SetProperty("aud", true)             // 访问单元分隔符
encoder.SetProperty("byte-stream", true)     // 字节流格式
encoder.SetProperty("key-int-max", 30)       // 关键帧间隔

// h264parse配置
parser.SetProperty("config-interval", 1)
parser.SetProperty("disable-passthrough", true)

// appsink caps
caps := gst.NewCapsFromString("video/x-h264,stream-format=byte-stream,alignment=au")
appsink.SetCaps(caps)
```

**解决时间：** 2025-11-21

---

### 问题3：GStreamer组件重构 - 清理过时文件 【已解决 ✅】

**问题：** 目录中存在 42 个过时文件，只有 1 个在使用

**重构成果：**
- 删除 41 个过时文件
- 只保留 `manager.go`
- 删除旧架构代码（`app.go` 从 1351 行减少到 380 行）
- 代码行数减少 72%，文件数量减少 98%

**解决时间：** 2025-11-21

---

### 问题4：命名规范和架构优化 【已解决 ✅】

**重构成果：**
- ✅ 文件重命名：`minimal_gogst_manager.go` → `manager.go`
- ✅ 类型重命名：`MinimalGoGstManager` → `Manager`
- ✅ 对象统一：`app.mediaBridge` → `app.gstreamerMgr`
- ✅ 日志统一：`minimal-gogst` → `gstreamer`
- ✅ 删除 `internal/factory/` 目录（无意义的抽象层）
- ✅ 删除 `internal/bridge/` 目录（合并到 gstreamer）

**最终结构：**
```
internal/gstreamer/
└── manager.go  # GStreamer 核心管理器
```

**解决时间：** 2025-11-21

---

### 问题5：组件交叉引用解耦 【已解决 ✅】

**问题：** `internal/gstreamer` 直接依赖 `internal/webrtc`，违反组件独立性原则

**解决方案：** 使用回调接口解耦

**架构改进：**
```
Before:
internal/gstreamer → internal/webrtc ❌ 交叉引用

After:
internal/gstreamer → common/interfaces ← internal/webrtc ✅ 解耦
```

**实现方式：**
1. 定义 `VideoDataSink` 接口（在 `internal/common/interfaces/`）
2. WebRTC 实现该接口
3. GStreamer 通过接口发送数据
4. 在 `app.go` 中注入：`gstreamerMgr.SetVideoSink(webrtcMgr)`

**优势：**
- ✅ 完全解耦，无交叉引用
- ✅ 性能无损（同步调用）
- ✅ 易于测试和扩展

**解决时间：** 2025-11-21

---

## 经验总结

### H.264编码器配置要点

对于WebRTC视频流，x264编码器必须配置：

1. **VUI信息**：`insert-vui: true` - 浏览器解码必需
2. **访问单元分隔符**：`aud: true` - 帧边界识别
3. **字节流格式**：`byte-stream: true` - WebRTC要求
4. **关键帧间隔**：`key-int-max: 30` - 定期产生I帧

### 组件设计原则

1. **唯一Manager原则**：每个组件只保留一个manager实现
2. **命名规范**：使用标准命名，避免冗余前缀
3. **组件独立性**：核心组件之间不应有交叉引用（除了 app.go）
4. **接口解耦**：使用接口而不是具体类型进行组件通信

### 启动顺序设计

1. **初始化阶段**：注册事件、建立订阅关系
2. **启动阶段**：启动顺序不应有强依赖
3. **数据处理**：未就绪时应静默忽略，而不是报错

---

## 调试技巧

### 1. 检查GStreamer数据产生

```bash
grep "GStreamer first frame" .tmp/bdwind-gstreamer.log
```

正常输出：`size=10374 bytes (10 KB)`

### 2. 检查WebRTC数据发送

```bash
grep "WebRTC video: first frame sent" .tmp/bdwind-gstreamer.log
```

帧大小应与GStreamer产生的一致

### 3. 检查浏览器解码

在测试页面中：
1. 点击"显示/隐藏统计"
2. 查看：
   - **接收字节数**：应持续增长
   - **已解码帧数**：应持续增长（如果为0说明无法解码）
   - **关键帧数**：应大于0（如果为0说明缺少I帧）

或使用 `chrome://webrtc-internals/` 查看详细统计

---

## 已知信息

### GStreamer管道配置
- 捕获源：ximagesrc (X11屏幕捕获)
- 编码器：x264enc (H.264软件编码)
- 输出：appsink (应用程序接收)
- 显示：:99 (Xvfb虚拟显示)

### 数据流向
```
Xvfb :99 → ximagesrc → videoscale → videoconvert → queue → x264enc → h264parse → appsink
    ↓
onNewSample (manager.go)
    ↓
VideoDataSink.SendVideoData (接口)
    ↓
WebRTCManager.SendVideoData (实现)
    ↓
videoTrack.WriteSample
    ↓
WebRTC → 客户端
```

### 最终架构

```
internal/
├── common/
│   └── interfaces/
│       └── video.go          # VideoDataSink 接口
├── gstreamer/
│   └── manager.go            # GStreamer 管理器
├── webrtc/
│   └── manager.go            # WebRTC 管理器（实现 VideoDataSink）
└── cmd/bdwind-gstreamer/
    └── app.go                # 应用入口（组件组装和注入）
```

---

*最后更新：2025-11-21*  
*状态：所有问题已解决并通过测试验证 ✅*

---

## 附录：问题1设计总结

### 按需推流和连接状态管理 - 架构设计

#### 核心思想
基于客户端连接状态和事件驱动机制管理GStreamer生命周期

#### 状态机设计

**GStreamer状态：**
- Idle（空闲）→ Starting（启动中）→ Streaming（推流中）→ Stopping（停止中）→ Idle

**WebRTC连接状态：**
- NoClient → Connecting → Connected → Disconnected/Failed → Closed

#### 事件系统

**8个会话生命周期事件（`webrtc.session.*`）：**

| 事件 | 触发时机 | 动作 |
|------|----------|------|
| `started` | 客户端连接 | 启动GStreamer |
| `ready` | ICE建立完成 | 记录日志 |
| `paused` | ICE断开 | 启动重连定时器 |
| `resumed` | ICE重连成功 | 取消定时器 |
| `failed` | ICE失败 | 记录错误 |
| `ended` | 客户端断开 | 检查会话数 |
| `timeout` | 重连超时 | 停止GStreamer |
| `no_active_sessions` | 无活跃会话 | 停止GStreamer |

#### 重连策略

| 场景 | 等待时间 | 动作 |
|------|----------|------|
| ICE Disconnected | 30秒 | 继续推流，等待重连 |
| ICE Failed | 10秒 | 继续推流，尝试重建 |
| 超时未重连 | - | 停止推流，清理资源 |
| 重连成功 | - | 继续推流 |

#### 工作流程

```
客户端连接 → session.started → GStreamer启动
    ↓
ICE建立 → session.ready → 开始传输
    ↓
ICE断开 → session.paused → 启动30秒定时器
    ↓
重连成功 → session.resumed → 取消定时器
或
重连超时 → session.timeout → GStreamer停止
    ↓
客户端断开 → session.ended → 检查会话数
    ↓
无活跃会话 → 延迟5秒 → no_active_sessions → GStreamer停止
```

#### 配置参数

```yaml
gstreamer:
  on_demand:
    enabled: true                # 启用按需推流
    idle_timeout: 5s             # 空闲超时
    quick_reconnect_window: 10s  # 快速重连窗口

webrtc:
  session:
    keep_alive_timeout: 30s      # 保活超时
```

#### 架构优势

- ✅ 节省资源：无客户端时不推流
- ✅ 减少延迟：推流与ICE建立同步
- ✅ 自动恢复：ICE断开后自动重连
- ✅ 优雅降级：超时后自动清理
- ✅ 完全解耦：通过事件系统，组件无直接依赖
- ✅ 易于配置：可通过配置文件控制行为

#### 实现要点

1. **公共事件包** - `internal/common/events/`
   - 事件接口和基础类型
   - 事件总线实现
   - 会话生命周期事件定义

2. **GStreamer增强** - `internal/gstreamer/manager.go`
   - 状态机实现
   - 事件订阅方法
   - 状态转换逻辑

3. **WebRTC增强** - `internal/webrtc/manager.go`
   - 会话管理（activeSessions）
   - ICE状态监听
   - 重连和空闲定时器
   - 事件发布

4. **应用层集成** - `cmd/bdwind-gstreamer/app.go`
   - 事件总线创建
   - 组件事件订阅
   - 客户端连接/断开回调

---

*设计完成时间：2025-11-21*
