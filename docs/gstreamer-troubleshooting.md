# GStreamer 推流问题诊断

## 调试规则

⚠️ **重要：使用 `start.sh` 脚本调试只能由人类进行，AI禁止调试**

## 当前问题

### 问题1：逻辑问题 - 应用启动时就开始产生流 【不着急】

**现状：**
- 应用程序启动时，GStreamer管道立即开始捕获和编码视频
- 此时还没有客户端连接，产生的数据被丢弃
- 浪费系统资源

**期望行为：**
- 应用启动时，GStreamer管道处于待机状态
- 当第一个客户端连接时，才开始捕获和编码
- 当所有客户端断开时，停止捕获

**优先级：** 低（不着急）

---

### 问题2：推的流看不到 【着急】

**现状：**
- ✅ WebRTC连接已成功建立（ICE connected）
- ✅ 客户端收到远程视频轨道（ontrack事件触发）
- ✅ GStreamer管道正在运行并产生数据
- ✅ 视频数据正在发送到WebRTC track
- ❌ **但客户端video元素显示黑屏，看不到视频**

**关键日志：**
```
📊 GStreamer stats: frame=300, total_bytes=0 MB, current_frame_size=0 KB
📹 WebRTC video: first frame sent, size=0 KB
📹 WebRTC video: sent 300 frames, current size=0 KB
```

**问题分析：**
- `current_frame_size=0 KB` - 发送的数据大小为0字节
- 可能原因：
  1. GStreamer产生的数据本身就是空的（编码器问题）
  2. 数据在传输过程中丢失
  3. 数据格式不正确

**优先级：** 高（着急）

---

## 诊断步骤

### 第一步：确认GStreamer是否产生有效数据

**需要检查的日志：**
```bash
grep "GStreamer first frame" .tmp/bdwind-gstreamer.log
```

**期望看到：**
```
📊 GStreamer first frame: size=XXXX bytes (XX KB)
```

**如果size=0：**
- GStreamer管道配置有问题
- 编码器没有产生数据
- 需要检查 `internal/gstreamer/minimal_gogst_manager.go` 中的管道配置

**如果size>0：**
- GStreamer正常工作
- 问题在数据传输环节

---

### 第二步：确认数据是否到达WebRTC

**需要检查的日志：**
```bash
grep "WebRTC video: first frame sent" .tmp/bdwind-gstreamer.log
```

**期望看到：**
```
📹 WebRTC video: first frame sent, size=XXXX bytes (XX KB)
```

**如果size=0：**
- 数据在bridge层丢失
- 需要检查 `internal/bridge/gogst_bridge.go` 中的 `onVideoFrame` 方法

**如果size>0：**
- 数据已发送到WebRTC track
- 问题可能在客户端接收或渲染

---

### 第三步：检查客户端接收

**在浏览器控制台检查：**
1. 打开 chrome://webrtc-internals/
2. 查看 "Stats graphs for peer-connection"
3. 检查 "bytesReceived" 是否在增长

**如果bytesReceived=0：**
- 数据没有通过网络传输
- 可能是编码格式问题
- 检查SDP中的codec配置

**如果bytesReceived>0：**
- 数据已接收
- 问题在视频解码或渲染
- 检查浏览器是否支持H.264解码

---

## 已知信息

### WebRTC连接状态
- ✅ ICE连接：connected
- ✅ PeerConnection：connected
- ✅ 视频轨道：已接收（ontrack触发）
- ✅ ICE候选：4个IPv4候选成功交换

### GStreamer管道配置
- 捕获源：ximagesrc (X11屏幕捕获)
- 编码器：x264enc (H.264软件编码)
- 输出：appsink (应用程序接收)
- 显示：:99 (Xvfb虚拟显示)

### 数据流向
```
Xvfb :99 → ximagesrc → videoscale → videoconvert → queue → x264enc → h264parse → appsink
    ↓
onNewSample (minimal_gogst_manager.go)
    ↓
videoCallback (gogst_bridge.go)
    ↓
SendVideoDataWithTimestamp (webrtc/manager.go)
    ↓
videoTrack.WriteSample
    ↓
WebRTC → 客户端
```

---

## 下一步行动

1. **人类执行：** 重新启动服务器，查看最新日志
2. **人类执行：** 检查 "GStreamer first frame" 的实际字节数
3. **人类执行：** 检查 "WebRTC video: first frame sent" 的实际字节数
4. **人类提供：** 将关键日志信息反馈给AI
5. **AI分析：** 根据日志数据定位问题根源

---

## 临时解决方案

如果需要快速验证推流功能，可以：
1. 使用测试视频文件代替屏幕捕获
2. 降低分辨率和码率
3. 使用更简单的编码器配置

---

*最后更新：2025-11-20*
*状态：问题2待解决（着急）*
