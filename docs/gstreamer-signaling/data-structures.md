# GStreamer WebRTC 信令数据结构规范

## 概述

本文档提供了 GStreamer WebRTC 信令协议中使用的详细数据结构定义、类型规范和示例数据。所有数据结构都基于 JSON 格式，并遵循严格的类型定义。

## 基础数据类型

### 原始类型

| 类型 | JSON 类型 | 描述 | 示例 |
|------|-----------|------|------|
| `string` | string | UTF-8 字符串 | `"hello"` |
| `number` | number | 数值（整数或浮点数） | `42`, `3.14` |
| `boolean` | boolean | 布尔值 | `true`, `false` |
| `timestamp` | number | Unix 时间戳（毫秒） | `1640995200000` |
| `uuid` | string | UUID 格式字符串 | `"550e8400-e29b-41d4-a716-446655440000"` |

### 复合类型

```typescript
// 错误信息结构
interface SignalingError {
  code: string;           // 错误代码
  message: string;        // 错误消息
  details?: string;       // 详细信息
  type?: string;          // 错误类型
  recoverable?: boolean;  // 是否可恢复
  suggestions?: string[]; // 建议操作
}

// 消息元数据结构
interface MessageMetadata {
  protocol?: string;      // 协议名称
  client_info?: ClientInfo;
  server_info?: ServerInfo;
  trace_id?: string;      // 追踪标识
}
```

## 核心消息结构

### SignalingMessage

标准信令消息的基础结构。

```typescript
interface SignalingMessage {
  version: string;        // 协议版本
  type: string;          // 消息类型
  id: string;            // 消息唯一标识
  timestamp: number;     // 时间戳
  peer_id?: string;      // 对等端标识
  data?: any;            // 消息数据
  metadata?: MessageMetadata;
  error?: SignalingError;
}
```

**示例**:
```json
{
  "version": "1.0",
  "type": "hello",
  "id": "msg_1640995200000_001",
  "timestamp": 1640995200000,
  "peer_id": "client_001",
  "data": {
    "client_info": {
      "user_agent": "Mozilla/5.0...",
      "capabilities": ["webrtc", "video"]
    }
  }
}
```

### SignalingResponse

标准化的响应消息结构。

```typescript
interface SignalingResponse {
  success: boolean;       // 操作是否成功
  data?: any;            // 响应数据
  error?: SignalingError; // 错误信息
  message_id?: string;   // 关联的消息ID
  timestamp: number;     // 响应时间戳
}
```

**示例**:
```json
{
  "success": true,
  "data": {
    "session_id": "sess_001",
    "capabilities": ["webrtc", "input"]
  },
  "message_id": "msg_1640995200000_001",
  "timestamp": 1640995200001
}
```

## 客户端信息结构

### ClientInfo

客户端基本信息和能力描述。

```typescript
interface ClientInfo {
  user_agent: string;           // 用户代理字符串
  screen_resolution: string;    // 屏幕分辨率 "1920x1080"
  device_pixel_ratio: number;  // 设备像素比
  language: string;             // 语言设置
  platform: string;            // 平台信息
  webrtc_version?: string;      // WebRTC 版本
  supported_codecs?: string[];  // 支持的编解码器
  network_info?: NetworkInfo;   // 网络信息
}
```

**示例**:
```json
{
  "user_agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36",
  "screen_resolution": "1920x1080",
  "device_pixel_ratio": 1.0,
  "language": "en-US",
  "platform": "Win32",
  "webrtc_version": "1.0",
  "supported_codecs": ["H264", "VP8", "VP9", "AV1"],
  "network_info": {
    "connection_type": "ethernet",
    "effective_type": "4g",
    "downlink": 10.0,
    "rtt": 50
  }
}
```

### ClientCapabilities

客户端能力声明。

```typescript
interface ClientCapabilities {
  webrtc: boolean;              // WebRTC 支持
  datachannel: boolean;         // 数据通道支持
  video: VideoCapabilities;     // 视频能力
  audio: AudioCapabilities;     // 音频能力
  input_events: boolean;        // 输入事件支持
  statistics: boolean;          // 统计信息支持
  recording: boolean;           // 录制支持
  screen_sharing: boolean;      // 屏幕共享支持
}

interface VideoCapabilities {
  codecs: string[];             // 支持的视频编解码器
  max_resolution: string;       // 最大分辨率
  max_framerate: number;        // 最大帧率
  hardware_acceleration: boolean; // 硬件加速支持
}

interface AudioCapabilities {
  codecs: string[];             // 支持的音频编解码器
  sample_rates: number[];       // 支持的采样率
  channels: number;             // 声道数
  echo_cancellation: boolean;   // 回声消除
  noise_suppression: boolean;   // 噪声抑制
}
```

**示例**:
```json
{
  "webrtc": true,
  "datachannel": true,
  "video": {
    "codecs": ["H264", "VP8", "VP9"],
    "max_resolution": "1920x1080",
    "max_framerate": 60,
    "hardware_acceleration": true
  },
  "audio": {
    "codecs": ["OPUS", "G722", "PCMU"],
    "sample_rates": [8000, 16000, 48000],
    "channels": 2,
    "echo_cancellation": true,
    "noise_suppression": true
  },
  "input_events": true,
  "statistics": true,
  "recording": false,
  "screen_sharing": true
}
```

## 服务器信息结构

### ServerInfo

服务器基本信息和配置。

```typescript
interface ServerInfo {
  version: string;              // 服务器版本
  name: string;                 // 服务器名称
  capabilities: string[];       // 服务器能力
  max_connections: number;      // 最大连接数
  supported_protocols: string[]; // 支持的协议
  ice_servers: ICEServer[];     // ICE 服务器配置
  session_config: SessionConfig; // 会话配置
}

interface SessionConfig {
  heartbeat_interval: number;   // 心跳间隔（毫秒）
  max_message_size: number;     // 最大消息大小（字节）
  connection_timeout: number;   // 连接超时（毫秒）
  max_retry_attempts: number;   // 最大重试次数
}
```

**示例**:
```json
{
  "version": "1.2.3",
  "name": "GStreamer WebRTC Server",
  "capabilities": ["webrtc", "input", "stats", "recording"],
  "max_connections": 100,
  "supported_protocols": ["gstreamer-1.0", "selkies"],
  "ice_servers": [
    {
      "urls": ["stun:stun.l.google.com:19302"]
    },
    {
      "urls": ["turn:turn.example.com:3478"],
      "username": "user",
      "credential": "pass"
    }
  ],
  "session_config": {
    "heartbeat_interval": 30000,
    "max_message_size": 65536,
    "connection_timeout": 300000,
    "max_retry_attempts": 3
  }
}
```

## WebRTC 数据结构

### SDP 结构

Session Description Protocol 数据结构。

```typescript
interface SDPMessage {
  type: "offer" | "answer";     // SDP 类型
  sdp: string;                  // SDP 内容
}

interface SDPInfo {
  version: number;              // SDP 版本
  origin: SDPOrigin;           // 会话源信息
  session_name: string;        // 会话名称
  media_descriptions: MediaDescription[]; // 媒体描述
  attributes: SDPAttribute[];   // 会话属性
}

interface SDPOrigin {
  username: string;             // 用户名
  session_id: string;          // 会话ID
  session_version: number;     // 会话版本
  network_type: string;        // 网络类型
  address_type: string;        // 地址类型
  address: string;             // 地址
}

interface MediaDescription {
  media_type: "video" | "audio" | "application"; // 媒体类型
  port: number;                // 端口
  protocol: string;            // 协议
  formats: string[];           // 格式列表
  connection?: ConnectionInfo; // 连接信息
  attributes: SDPAttribute[];  // 媒体属性
}

interface SDPAttribute {
  name: string;                // 属性名
  value?: string;              // 属性值
}
```

**示例**:
```json
{
  "type": "offer",
  "sdp": "v=0\r\no=- 4611731400430051336 2 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\na=group:BUNDLE 0\r\nm=video 9 UDP/TLS/RTP/SAVPF 96 97 98\r\nc=IN IP4 0.0.0.0\r\na=rtcp:9 IN IP4 0.0.0.0\r\na=ice-ufrag:4ZcD\r\na=ice-pwd:2/1muCWoOi3uLifh0NuRHlM6\r\na=fingerprint:sha-256 75:74:5A:A6:A4:E5:52:F4:A7:67:4C:01:C7:EE:91:3F:21:3D:A2:E3:53:7B:6F:30:86:F2:30:FF:A6:22:D2:04\r\na=setup:actpass\r\na=mid:0\r\na=sendrecv\r\na=rtcp-mux\r\na=rtpmap:96 VP8/90000\r\na=rtcp-fb:96 nack\r\na=rtcp-fb:96 nack pli\r\n"
}
```

### ICE 候选结构

Interactive Connectivity Establishment 候选信息。

```typescript
interface ICECandidate {
  candidate: string;            // 候选字符串
  sdpMid?: string;             // SDP 媒体标识
  sdpMLineIndex?: number;      // SDP 媒体行索引
  usernameFragment?: string;   // 用户名片段
}

interface ICECandidateInfo {
  foundation: string;          // 基础
  component_id: number;        // 组件ID
  transport: "udp" | "tcp";    // 传输协议
  priority: number;            // 优先级
  address: string;             // IP地址
  port: number;                // 端口
  type: "host" | "srflx" | "prflx" | "relay"; // 候选类型
  related_address?: string;    // 相关地址
  related_port?: number;       // 相关端口
  extension_attributes?: { [key: string]: string }; // 扩展属性
}
```

**示例**:
```json
{
  "candidate": "candidate:842163049 1 udp 1677729535 192.168.1.100 54400 typ srflx raddr 192.168.1.100 rport 54400 generation 0 ufrag 4ZcD network-cost 999",
  "sdpMid": "0",
  "sdpMLineIndex": 0,
  "usernameFragment": "4ZcD"
}
```

### ICE 服务器配置

```typescript
interface ICEServer {
  urls: string[];              // STUN/TURN 服务器URL列表
  username?: string;           // 用户名（TURN）
  credential?: string;         // 凭据（TURN）
  credentialType?: "password" | "oauth"; // 凭据类型
}
```

**示例**:
```json
[
  {
    "urls": ["stun:stun.l.google.com:19302"]
  },
  {
    "urls": [
      "turn:turn.example.com:3478",
      "turns:turn.example.com:5349"
    ],
    "username": "turnuser",
    "credential": "turnpass",
    "credentialType": "password"
  }
]
```

## 输入事件数据结构

### 鼠标事件

```typescript
interface MouseEvent {
  x: number;                   // X 坐标
  y: number;                   // Y 坐标
  button?: "left" | "right" | "middle" | "x1" | "x2"; // 鼠标按键
  action: "down" | "up" | "move" | "wheel"; // 动作类型
  modifiers?: KeyModifier[];   // 修饰键
  wheel_delta?: number;        // 滚轮增量
  relative_x?: number;         // 相对X移动
  relative_y?: number;         // 相对Y移动
}

type KeyModifier = "ctrl" | "alt" | "shift" | "meta";
```

**示例**:
```json
{
  "x": 100,
  "y": 200,
  "button": "left",
  "action": "down",
  "modifiers": ["ctrl", "shift"]
}
```

### 键盘事件

```typescript
interface KeyboardEvent {
  key: string;                 // 按键字符
  code: string;                // 按键代码
  key_code?: number;           // 键码（已弃用但保持兼容）
  action: "down" | "up";       // 动作类型
  modifiers?: KeyModifier[];   // 修饰键
  repeat?: boolean;            // 是否重复
  location?: number;           // 按键位置
}
```

**示例**:
```json
{
  "key": "a",
  "code": "KeyA",
  "key_code": 65,
  "action": "down",
  "modifiers": ["ctrl"],
  "repeat": false,
  "location": 0
}
```

### 触摸事件

```typescript
interface TouchEvent {
  touches: TouchPoint[];       // 触摸点列表
  action: "start" | "move" | "end" | "cancel"; // 动作类型
  timestamp: number;           // 事件时间戳
}

interface TouchPoint {
  id: number;                  // 触摸点ID
  x: number;                   // X 坐标
  y: number;                   // Y 坐标
  pressure?: number;           // 压力
  radius_x?: number;           // X 半径
  radius_y?: number;           // Y 半径
  rotation_angle?: number;     // 旋转角度
}
```

**示例**:
```json
{
  "touches": [
    {
      "id": 1,
      "x": 100,
      "y": 200,
      "pressure": 0.5,
      "radius_x": 10,
      "radius_y": 10,
      "rotation_angle": 0
    }
  ],
  "action": "start",
  "timestamp": 1640995200000
}
```

## 统计信息数据结构

### WebRTC 统计

```typescript
interface WebRTCStats {
  connection: ConnectionStats;  // 连接统计
  video: VideoStats;           // 视频统计
  audio: AudioStats;           // 音频统计
  datachannel: DataChannelStats; // 数据通道统计
}

interface ConnectionStats {
  state: "new" | "connecting" | "connected" | "disconnected" | "failed" | "closed";
  ice_connection_state: "new" | "checking" | "connected" | "completed" | "failed" | "disconnected" | "closed";
  signaling_state: "stable" | "have-local-offer" | "have-remote-offer" | "have-local-pranswer" | "have-remote-pranswer" | "closed";
  bytes_sent: number;          // 发送字节数
  bytes_received: number;      // 接收字节数
  packets_sent: number;        // 发送包数
  packets_received: number;    // 接收包数
  packets_lost: number;        // 丢包数
  jitter: number;              // 抖动
  rtt: number;                 // 往返时间
  bandwidth: number;           // 带宽
}

interface VideoStats {
  codec: string;               // 编解码器
  resolution: string;          // 分辨率
  framerate: number;           // 帧率
  bitrate: number;             // 比特率
  frames_sent?: number;        // 发送帧数
  frames_received?: number;    // 接收帧数
  frames_dropped?: number;     // 丢帧数
  key_frames_decoded?: number; // 解码关键帧数
  total_decode_time?: number;  // 总解码时间
}

interface AudioStats {
  codec: string;               // 编解码器
  sample_rate: number;         // 采样率
  channels: number;            // 声道数
  bitrate: number;             // 比特率
  samples_sent?: number;       // 发送样本数
  samples_received?: number;   // 接收样本数
  audio_level?: number;        // 音频电平
  echo_return_loss?: number;   // 回声返回损耗
}

interface DataChannelStats {
  state: "connecting" | "open" | "closing" | "closed";
  messages_sent: number;       // 发送消息数
  messages_received: number;   // 接收消息数
  bytes_sent: number;          // 发送字节数
  bytes_received: number;      // 接收字节数
}
```

**示例**:
```json
{
  "connection": {
    "state": "connected",
    "ice_connection_state": "connected",
    "signaling_state": "stable",
    "bytes_sent": 1048576,
    "bytes_received": 2097152,
    "packets_sent": 1024,
    "packets_received": 2048,
    "packets_lost": 0,
    "jitter": 0.001,
    "rtt": 0.05,
    "bandwidth": 1000000
  },
  "video": {
    "codec": "H264",
    "resolution": "1920x1080",
    "framerate": 60,
    "bitrate": 2000000,
    "frames_sent": 3600,
    "frames_received": 3598,
    "frames_dropped": 2,
    "key_frames_decoded": 60,
    "total_decode_time": 0.5
  },
  "audio": {
    "codec": "OPUS",
    "sample_rate": 48000,
    "channels": 2,
    "bitrate": 128000,
    "samples_sent": 2880000,
    "samples_received": 2879500,
    "audio_level": 0.8,
    "echo_return_loss": 30
  },
  "datachannel": {
    "state": "open",
    "messages_sent": 150,
    "messages_received": 120,
    "bytes_sent": 15360,
    "bytes_received": 12288
  }
}
```

### 系统统计

```typescript
interface SystemStats {
  cpu: CPUStats;               // CPU 统计
  memory: MemoryStats;         // 内存统计
  gpu?: GPUStats;              // GPU 统计（可选）
  network: NetworkStats;       // 网络统计
  display?: DisplayStats;      // 显示统计（可选）
}

interface CPUStats {
  usage_percent: number;       // CPU 使用率
  cores: number;               // 核心数
  frequency_mhz: number;       // 频率
  temperature?: number;        // 温度（可选）
}

interface MemoryStats {
  total_bytes: number;         // 总内存
  used_bytes: number;          // 已用内存
  available_bytes: number;     // 可用内存
  usage_percent: number;       // 使用率
}

interface GPUStats {
  name: string;                // GPU 名称
  usage_percent: number;       // GPU 使用率
  memory_total_bytes: number;  // GPU 总内存
  memory_used_bytes: number;   // GPU 已用内存
  temperature?: number;        // 温度（可选）
}

interface NetworkStats {
  connection_type: "ethernet" | "wifi" | "cellular" | "unknown";
  effective_type: "slow-2g" | "2g" | "3g" | "4g" | "5g";
  downlink_mbps: number;       // 下行带宽
  uplink_mbps?: number;        // 上行带宽（可选）
  rtt_ms: number;              // 往返时间
  packet_loss_percent?: number; // 丢包率（可选）
}

interface DisplayStats {
  resolution: string;          // 分辨率
  refresh_rate: number;        // 刷新率
  color_depth: number;         // 色深
  pixel_density: number;       // 像素密度
}
```

**示例**:
```json
{
  "cpu": {
    "usage_percent": 25.5,
    "cores": 8,
    "frequency_mhz": 3200,
    "temperature": 65
  },
  "memory": {
    "total_bytes": 16777216000,
    "used_bytes": 8388608000,
    "available_bytes": 8388608000,
    "usage_percent": 50.0
  },
  "gpu": {
    "name": "NVIDIA GeForce RTX 3080",
    "usage_percent": 45.2,
    "memory_total_bytes": 10737418240,
    "memory_used_bytes": 4294967296,
    "temperature": 72
  },
  "network": {
    "connection_type": "ethernet",
    "effective_type": "4g",
    "downlink_mbps": 100.0,
    "uplink_mbps": 50.0,
    "rtt_ms": 20,
    "packet_loss_percent": 0.1
  },
  "display": {
    "resolution": "1920x1080",
    "refresh_rate": 60,
    "color_depth": 24,
    "pixel_density": 96
  }
}
```

## 会话管理数据结构

### 会话信息

```typescript
interface SessionInfo {
  session_id: string;          // 会话ID
  peer_id: string;             // 对等端ID
  app_name: string;            // 应用名称
  state: SessionState;         // 会话状态
  created_at: number;          // 创建时间
  last_activity: number;       // 最后活动时间
  connection_info: ConnectionInfo; // 连接信息
  statistics: SessionStatistics; // 会话统计
}

type SessionState = "connecting" | "connected" | "disconnected" | "error";

interface ConnectionInfo {
  remote_address: string;      // 远程地址
  user_agent: string;          // 用户代理
  protocol: string;            // 使用的协议
  encryption: boolean;         // 是否加密
}

interface SessionStatistics {
  messages_sent: number;       // 发送消息数
  messages_received: number;   // 接收消息数
  bytes_sent: number;          // 发送字节数
  bytes_received: number;      // 接收字节数
  errors: number;              // 错误数
  connection_quality: "excellent" | "good" | "fair" | "poor"; // 连接质量
}
```

**示例**:
```json
{
  "session_id": "sess_1640995200000_001",
  "peer_id": "client_001",
  "app_name": "desktop-streaming",
  "state": "connected",
  "created_at": 1640995200000,
  "last_activity": 1640995800000,
  "connection_info": {
    "remote_address": "192.168.1.100:54321",
    "user_agent": "Mozilla/5.0...",
    "protocol": "gstreamer-1.0",
    "encryption": true
  },
  "statistics": {
    "messages_sent": 150,
    "messages_received": 120,
    "bytes_sent": 1048576,
    "bytes_received": 2097152,
    "errors": 2,
    "connection_quality": "good"
  }
}
```

## 配置数据结构

### 编码器配置

```typescript
interface EncoderConfig {
  video: VideoEncoderConfig;   // 视频编码配置
  audio: AudioEncoderConfig;   // 音频编码配置
}

interface VideoEncoderConfig {
  codec: "H264" | "VP8" | "VP9" | "AV1"; // 编解码器
  profile?: string;            // 编码档次
  level?: string;              // 编码级别
  bitrate: number;             // 比特率
  max_bitrate?: number;        // 最大比特率
  min_bitrate?: number;        // 最小比特率
  framerate: number;           // 帧率
  resolution: string;          // 分辨率
  keyframe_interval?: number;  // 关键帧间隔
  quality: "low" | "medium" | "high" | "ultra"; // 质量等级
  hardware_acceleration?: boolean; // 硬件加速
}

interface AudioEncoderConfig {
  codec: "OPUS" | "AAC" | "G722" | "PCMU" | "PCMA"; // 编解码器
  bitrate: number;             // 比特率
  sample_rate: number;         // 采样率
  channels: number;            // 声道数
  frame_size?: number;         // 帧大小
  complexity?: number;         // 复杂度（OPUS）
  vbr?: boolean;               // 可变比特率
  dtx?: boolean;               // 不连续传输
}
```

**示例**:
```json
{
  "video": {
    "codec": "H264",
    "profile": "baseline",
    "level": "3.1",
    "bitrate": 2000000,
    "max_bitrate": 4000000,
    "min_bitrate": 500000,
    "framerate": 60,
    "resolution": "1920x1080",
    "keyframe_interval": 60,
    "quality": "high",
    "hardware_acceleration": true
  },
  "audio": {
    "codec": "OPUS",
    "bitrate": 128000,
    "sample_rate": 48000,
    "channels": 2,
    "frame_size": 960,
    "complexity": 10,
    "vbr": true,
    "dtx": false
  }
}
```

## 网络信息数据结构

### 网络配置

```typescript
interface NetworkConfig {
  ice_servers: ICEServer[];    // ICE 服务器列表
  ice_transport_policy: "all" | "relay"; // ICE 传输策略
  bundle_policy: "balanced" | "max-compat" | "max-bundle"; // 捆绑策略
  rtcp_mux_policy: "negotiate" | "require"; // RTCP 复用策略
  certificates?: RTCCertificate[]; // 证书列表
}

interface NetworkInfo {
  local_candidates: ICECandidateInfo[]; // 本地候选
  remote_candidates: ICECandidateInfo[]; // 远程候选
  selected_pair?: CandidatePair; // 选中的候选对
  network_type: string;        // 网络类型
  transport_type: "udp" | "tcp"; // 传输类型
}

interface CandidatePair {
  local: ICECandidateInfo;     // 本地候选
  remote: ICECandidateInfo;    // 远程候选
  state: "waiting" | "in-progress" | "succeeded" | "failed"; // 状态
  priority: number;            // 优先级
  nominated: boolean;          // 是否被提名
}
```

**示例**:
```json
{
  "local_candidates": [
    {
      "foundation": "1",
      "component_id": 1,
      "transport": "udp",
      "priority": 2130706431,
      "address": "192.168.1.100",
      "port": 54400,
      "type": "host"
    }
  ],
  "remote_candidates": [
    {
      "foundation": "2",
      "component_id": 1,
      "transport": "udp",
      "priority": 1677729535,
      "address": "203.0.113.1",
      "port": 12345,
      "type": "srflx",
      "related_address": "192.168.1.200",
      "related_port": 54401
    }
  ],
  "selected_pair": {
    "local": {
      "foundation": "1",
      "component_id": 1,
      "transport": "udp",
      "priority": 2130706431,
      "address": "192.168.1.100",
      "port": 54400,
      "type": "host"
    },
    "remote": {
      "foundation": "2",
      "component_id": 1,
      "transport": "udp",
      "priority": 1677729535,
      "address": "203.0.113.1",
      "port": 12345,
      "type": "srflx"
    },
    "state": "succeeded",
    "priority": 9151314440652587007,
    "nominated": true
  },
  "network_type": "ethernet",
  "transport_type": "udp"
}
```

## 验证规则

### 数据验证约束

```typescript
// 字符串长度限制
const STRING_LIMITS = {
  message_id: { min: 1, max: 64 },
  peer_id: { min: 1, max: 32 },
  session_id: { min: 1, max: 64 },
  user_agent: { min: 1, max: 512 },
  error_message: { min: 1, max: 256 },
  sdp_content: { min: 1, max: 32768 }
};

// 数值范围限制
const NUMBER_LIMITS = {
  timestamp: { min: 0, max: Number.MAX_SAFE_INTEGER },
  coordinates: { min: -32768, max: 32767 },
  bitrate: { min: 1000, max: 100000000 },
  framerate: { min: 1, max: 120 },
  sample_rate: { min: 8000, max: 192000 },
  port: { min: 1, max: 65535 }
};

// 枚举值验证
const ENUM_VALUES = {
  message_types: ["hello", "welcome", "ping", "pong", "offer", "answer", "ice-candidate", "error"],
  session_states: ["connecting", "connected", "disconnected", "error"],
  connection_states: ["new", "connecting", "connected", "disconnected", "failed", "closed"],
  video_codecs: ["H264", "VP8", "VP9", "AV1"],
  audio_codecs: ["OPUS", "AAC", "G722", "PCMU", "PCMA"]
};
```

### 格式验证正则表达式

```typescript
const FORMAT_PATTERNS = {
  // 消息ID格式: msg_timestamp_sequence
  message_id: /^msg_\d{13}_\d{3,6}$/,
  
  // 会话ID格式: sess_timestamp_sequence
  session_id: /^sess_\d{13}_\d{3,6}$/,
  
  // 客户端ID格式: client_timestamp_random
  client_id: /^client_\d{13}_\d{4,6}$/,
  
  // 分辨率格式: widthxheight
  resolution: /^\d{1,5}x\d{1,5}$/,
  
  // IP地址格式（IPv4）
  ipv4: /^(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)$/,
  
  // MAC地址格式
  mac_address: /^([0-9A-Fa-f]{2}[:-]){5}([0-9A-Fa-f]{2})$/,
  
  // UUID格式
  uuid: /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i
};
```

## 序列化和反序列化

### JSON 序列化规则

1. **数值精度**: 浮点数保留最多6位小数
2. **时间戳**: 使用毫秒级Unix时间戳
3. **字符编码**: 统一使用UTF-8编码
4. **空值处理**: 使用`null`表示空值，省略可选的空字段
5. **数组排序**: 保持数组元素的原始顺序

### 示例序列化代码

```typescript
// TypeScript 序列化示例
function serializeSignalingMessage(message: SignalingMessage): string {
  // 移除undefined字段
  const cleanMessage = JSON.parse(JSON.stringify(message));
  
  // 格式化时间戳
  if (cleanMessage.timestamp) {
    cleanMessage.timestamp = Math.floor(cleanMessage.timestamp);
  }
  
  // 格式化浮点数
  if (cleanMessage.data) {
    formatFloatingNumbers(cleanMessage.data);
  }
  
  return JSON.stringify(cleanMessage);
}

function formatFloatingNumbers(obj: any): void {
  for (const key in obj) {
    if (typeof obj[key] === 'number' && !Number.isInteger(obj[key])) {
      obj[key] = Math.round(obj[key] * 1000000) / 1000000; // 6位小数
    } else if (typeof obj[key] === 'object' && obj[key] !== null) {
      formatFloatingNumbers(obj[key]);
    }
  }
}
```

这些数据结构定义为 GStreamer WebRTC 信令协议提供了完整的类型系统和验证规则，确保了消息的一致性和可靠性。