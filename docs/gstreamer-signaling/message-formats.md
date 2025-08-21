# GStreamer WebRTC 信令消息格式规范

## 概述

本文档定义了 GStreamer WebRTC 信令协议中使用的标准消息格式、数据结构和编码规范。协议支持多种消息格式以确保与不同客户端的兼容性。

## 消息格式版本

- **标准格式版本**: 1.0 (基于 GStreamer 官方规范)
- **兼容格式**: selkies-gstreamer 协议
- **编码格式**: UTF-8 JSON

## 标准消息结构

### 基础消息格式

所有标准消息都遵循以下基本结构：

```json
{
  "version": "1.0",
  "type": "message_type",
  "id": "unique_message_id",
  "timestamp": 1640995200000,
  "peer_id": "client_peer_id",
  "data": {
    // 消息特定数据
  },
  "metadata": {
    // 可选的元数据
  },
  "error": {
    // 错误信息（仅错误消息）
  }
}
```

### 字段说明

| 字段 | 类型 | 必需 | 描述 |
|------|------|------|------|
| `version` | string | 是 | 协议版本号 |
| `type` | string | 是 | 消息类型标识符 |
| `id` | string | 是 | 唯一消息标识符 |
| `timestamp` | number | 是 | Unix 时间戳（毫秒） |
| `peer_id` | string | 否 | 对等端标识符 |
| `data` | object | 否 | 消息载荷数据 |
| `metadata` | object | 否 | 消息元数据 |
| `error` | object | 否 | 错误信息（仅错误消息） |

## 连接管理消息

### HELLO 消息

客户端注册和能力协商消息。

**客户端 → 服务器**

```json
{
  "version": "1.0",
  "type": "hello",
  "id": "msg_1640995200000_001",
  "timestamp": 1640995200000,
  "peer_id": "client_001",
  "data": {
    "client_info": {
      "user_agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
      "screen_resolution": "1920x1080",
      "device_pixel_ratio": 1.0,
      "language": "en-US",
      "platform": "Win32"
    },
    "capabilities": [
      "webrtc",
      "datachannel",
      "video",
      "audio",
      "input-events",
      "statistics"
    ],
    "supported_protocols": [
      "gstreamer-1.0",
      "selkies"
    ],
    "preferred_protocol": "gstreamer-1.0"
  }
}
```

### WELCOME 消息

服务器欢迎和确认消息。

**服务器 → 客户端**

```json
{
  "version": "1.0",
  "type": "welcome",
  "id": "msg_1640995200001_001",
  "timestamp": 1640995200001,
  "peer_id": "client_001",
  "data": {
    "client_id": "client_001",
    "app_name": "default",
    "server_time": 1640995200001,
    "server_capabilities": [
      "webrtc",
      "input",
      "stats",
      "recording"
    ],
    "protocol": "gstreamer-1.0",
    "session_config": {
      "heartbeat_interval": 30000,
      "max_message_size": 65536,
      "ice_servers": [
        {
          "urls": ["stun:stun.l.google.com:19302"]
        }
      ]
    }
  }
}
```

### PING/PONG 消息

连接保活心跳消息。

**PING (客户端 → 服务器)**

```json
{
  "version": "1.0",
  "type": "ping",
  "id": "msg_1640995230000_002",
  "timestamp": 1640995230000,
  "peer_id": "client_001",
  "data": {
    "client_state": "connected",
    "sequence": 1
  }
}
```

**PONG (服务器 → 客户端)**

```json
{
  "version": "1.0",
  "type": "pong",
  "id": "msg_1640995230001_002",
  "timestamp": 1640995230001,
  "peer_id": "client_001",
  "data": {
    "server_state": "running",
    "sequence": 1,
    "latency_ms": 1
  }
}
```

## WebRTC 协商消息

### REQUEST-OFFER 消息

客户端请求 WebRTC Offer。

**客户端 → 服务器**

```json
{
  "version": "1.0",
  "type": "request-offer",
  "id": "msg_1640995240000_003",
  "timestamp": 1640995240000,
  "peer_id": "client_001",
  "data": {
    "constraints": {
      "video": true,
      "audio": true,
      "data_channel": true
    },
    "codec_preferences": [
      "H264",
      "VP8",
      "VP9"
    ]
  }
}
```

### OFFER 消息

服务器发送 WebRTC SDP Offer。

**服务器 → 客户端**

```json
{
  "version": "1.0",
  "type": "offer",
  "id": "msg_1640995240001_003",
  "timestamp": 1640995240001,
  "peer_id": "client_001",
  "data": {
    "sdp": {
      "type": "offer",
      "sdp": "v=0\r\no=- 4611731400430051336 2 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\na=group:BUNDLE 0\r\na=extmap-allow-mixed\r\na=msid-semantic: WMS\r\nm=video 9 UDP/TLS/RTP/SAVPF 96 97 98 99 100 101 102 121 127 120 125 107 108 109 124 119 123 118 114 115 116\r\nc=IN IP4 0.0.0.0\r\na=rtcp:9 IN IP4 0.0.0.0\r\na=ice-ufrag:4ZcD\r\na=ice-pwd:2/1muCWoOi3uLifh0NuRHlM6\r\na=ice-options:trickle\r\na=fingerprint:sha-256 75:74:5A:A6:A4:E5:52:F4:A7:67:4C:01:C7:EE:91:3F:21:3D:A2:E3:53:7B:6F:30:86:F2:30:FF:A6:22:D2:04\r\na=setup:actpass\r\na=mid:0\r\na=extmap:1 urn:ietf:params:rtp-hdrext:ssrc-audio-level\r\na=extmap:2 http://www.webrtc.org/experiments/rtp-hdrext/abs-send-time\r\na=extmap:3 http://www.ietf.org/id/draft-holmer-rmcat-transport-wide-cc-extensions-01\r\na=extmap:4 urn:ietf:params:rtp-hdrext:sdes:mid\r\na=extmap:5 urn:ietf:params:rtp-hdrext:sdes:rtp-stream-id\r\na=extmap:6 urn:ietf:params:rtp-hdrext:sdes:repaired-rtp-stream-id\r\na=sendrecv\r\na=msid:- \r\na=rtcp-mux\r\na=rtpmap:96 VP8/90000\r\na=rtcp-fb:96 goog-remb\r\na=rtcp-fb:96 transport-cc\r\na=rtcp-fb:96 ccm fir\r\na=rtcp-fb:96 nack\r\na=rtcp-fb:96 nack pli\r\na=rtpmap:97 rtx/90000\r\na=fmtp:97 apt=96\r\na=rtpmap:98 VP9/90000\r\na=rtcp-fb:98 goog-remb\r\na=rtcp-fb:98 transport-cc\r\na=rtcp-fb:98 ccm fir\r\na=rtcp-fb:98 nack\r\na=rtcp-fb:98 nack pli\r\na=fmtp:98 profile-id=0\r\na=rtpmap:99 rtx/90000\r\na=fmtp:99 apt=98\r\na=rtpmap:100 VP9/90000\r\na=rtcp-fb:100 goog-remb\r\na=rtcp-fb:100 transport-cc\r\na=rtcp-fb:100 ccm fir\r\na=rtcp-fb:100 nack\r\na=rtcp-fb:100 nack pli\r\na=fmtp:100 profile-id=2\r\na=rtpmap:101 rtx/90000\r\na=fmtp:101 apt=100\r\na=rtpmap:102 H264/90000\r\na=rtcp-fb:102 goog-remb\r\na=rtcp-fb:102 transport-cc\r\na=rtcp-fb:102 ccm fir\r\na=rtcp-fb:102 nack\r\na=rtcp-fb:102 nack pli\r\na=fmtp:102 level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42001f\r\na=rtpmap:121 rtx/90000\r\na=fmtp:121 apt=102\r\na=rtpmap:127 H264/90000\r\na=rtcp-fb:127 goog-remb\r\na=rtcp-fb:127 transport-cc\r\na=rtcp-fb:127 ccm fir\r\na=rtcp-fb:127 nack\r\na=rtcp-fb:127 nack pli\r\na=fmtp:127 level-asymmetry-allowed=1;packetization-mode=0;profile-level-id=42001f\r\na=rtpmap:120 rtx/90000\r\na=fmtp:120 apt=127\r\na=rtpmap:125 H264/90000\r\na=rtcp-fb:125 goog-remb\r\na=rtcp-fb:125 transport-cc\r\na=rtcp-fb:125 ccm fir\r\na=rtcp-fb:125 nack\r\na=rtcp-fb:125 nack pli\r\na=fmtp:125 level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f\r\na=rtpmap:107 rtx/90000\r\na=fmtp:107 apt=125\r\na=rtpmap:108 H264/90000\r\na=rtcp-fb:108 goog-remb\r\na=rtcp-fb:108 transport-cc\r\na=rtcp-fb:108 ccm fir\r\na=rtcp-fb:108 nack\r\na=rtcp-fb:108 nack pli\r\na=fmtp:108 level-asymmetry-allowed=1;packetization-mode=0;profile-level-id=42e01f\r\na=rtpmap:109 rtx/90000\r\na=fmtp:109 apt=108\r\na=rtpmap:124 H264/90000\r\na=rtcp-fb:124 goog-remb\r\na=rtcp-fb:124 transport-cc\r\na=rtcp-fb:124 ccm fir\r\na=rtcp-fb:124 nack\r\na=rtcp-fb:124 nack pli\r\na=fmtp:124 level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=4d001f\r\na=rtpmap:119 rtx/90000\r\na=fmtp:119 apt=124\r\na=rtpmap:123 H264/90000\r\na=rtcp-fb:123 goog-remb\r\na=rtcp-fb:123 transport-cc\r\na=rtcp-fb:123 ccm fir\r\na=rtcp-fb:123 nack\r\na=rtcp-fb:123 nack pli\r\na=fmtp:123 level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=64001f\r\na=rtpmap:118 rtx/90000\r\na=fmtp:118 apt=123\r\na=rtpmap:114 red/90000\r\na=rtpmap:115 rtx/90000\r\na=fmtp:115 apt=114\r\na=rtpmap:116 ulpfec/90000\r\na=ssrc-group:FID 2231627014 632943048\r\na=ssrc:2231627014 cname:bIU0fpgYFCaQMdau\r\na=ssrc:2231627014 msid:- \r\na=ssrc:2231627014 mslabel:-\r\na=ssrc:2231627014 label:\r\na=ssrc:632943048 cname:bIU0fpgYFCaQMdau\r\na=ssrc:632943048 msid:- \r\na=ssrc:632943048 mslabel:-\r\na=ssrc:632943048 label:\r\n"
    },
    "ice_servers": [
      {
        "urls": ["stun:stun.l.google.com:19302"]
      }
    ]
  }
}
```

### ANSWER 消息

客户端发送 WebRTC SDP Answer。

**客户端 → 服务器**

```json
{
  "version": "1.0",
  "type": "answer",
  "id": "msg_1640995250000_004",
  "timestamp": 1640995250000,
  "peer_id": "client_001",
  "data": {
    "sdp": {
      "type": "answer",
      "sdp": "v=0\r\no=- 1640995250000 2 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\na=group:BUNDLE 0\r\na=extmap-allow-mixed\r\na=msid-semantic: WMS\r\nm=video 9 UDP/TLS/RTP/SAVPF 102\r\nc=IN IP4 0.0.0.0\r\na=rtcp:9 IN IP4 0.0.0.0\r\na=ice-ufrag:abcd\r\na=ice-pwd:efgh1234567890abcdef\r\na=ice-options:trickle\r\na=fingerprint:sha-256 AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99:AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99\r\na=setup:active\r\na=mid:0\r\na=extmap:1 urn:ietf:params:rtp-hdrext:ssrc-audio-level\r\na=extmap:2 http://www.webrtc.org/experiments/rtp-hdrext/abs-send-time\r\na=extmap:3 http://www.ietf.org/id/draft-holmer-rmcat-transport-wide-cc-extensions-01\r\na=extmap:4 urn:ietf:params:rtp-hdrext:sdes:mid\r\na=recvonly\r\na=rtcp-mux\r\na=rtpmap:102 H264/90000\r\na=rtcp-fb:102 goog-remb\r\na=rtcp-fb:102 transport-cc\r\na=rtcp-fb:102 ccm fir\r\na=rtcp-fb:102 nack\r\na=rtcp-fb:102 nack pli\r\na=fmtp:102 level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42001f\r\n"
    }
  }
}
```

### ICE-CANDIDATE 消息

ICE 候选交换消息。

**双向**

```json
{
  "version": "1.0",
  "type": "ice-candidate",
  "id": "msg_1640995260000_005",
  "timestamp": 1640995260000,
  "peer_id": "client_001",
  "data": {
    "candidate": {
      "candidate": "candidate:842163049 1 udp 1677729535 192.168.1.100 54400 typ srflx raddr 192.168.1.100 rport 54400 generation 0 ufrag 4ZcD network-cost 999",
      "sdpMid": "0",
      "sdpMLineIndex": 0,
      "usernameFragment": "4ZcD"
    }
  }
}
```

## 媒体控制消息

### 鼠标事件消息

#### MOUSE-CLICK 消息

**客户端 → 服务器**

```json
{
  "version": "1.0",
  "type": "mouse-click",
  "id": "msg_1640995270000_006",
  "timestamp": 1640995270000,
  "peer_id": "client_001",
  "data": {
    "x": 100,
    "y": 200,
    "button": "left",
    "action": "down",
    "modifiers": ["ctrl", "shift"]
  }
}
```

#### MOUSE-MOVE 消息

**客户端 → 服务器**

```json
{
  "version": "1.0",
  "type": "mouse-move",
  "id": "msg_1640995280000_007",
  "timestamp": 1640995280000,
  "peer_id": "client_001",
  "data": {
    "x": 150,
    "y": 250,
    "relative_x": 50,
    "relative_y": 50
  }
}
```

### 键盘事件消息

#### KEY-PRESS 消息

**客户端 → 服务器**

```json
{
  "version": "1.0",
  "type": "key-press",
  "id": "msg_1640995290000_008",
  "timestamp": 1640995290000,
  "peer_id": "client_001",
  "data": {
    "key": "a",
    "code": "KeyA",
    "key_code": 65,
    "action": "down",
    "modifiers": ["ctrl"],
    "repeat": false
  }
}
```

### 统计信息消息

#### GET-STATS 消息

**客户端 → 服务器**

```json
{
  "version": "1.0",
  "type": "get-stats",
  "id": "msg_1640995300000_009",
  "timestamp": 1640995300000,
  "peer_id": "client_001",
  "data": {
    "stats_type": ["webrtc", "system", "network"],
    "interval": 1000
  }
}
```

#### STATS 消息

**服务器 → 客户端**

```json
{
  "version": "1.0",
  "type": "stats",
  "id": "msg_1640995301000_009",
  "timestamp": 1640995301000,
  "peer_id": "client_001",
  "data": {
    "webrtc": {
      "bytes_sent": 1048576,
      "bytes_received": 2097152,
      "packets_sent": 1024,
      "packets_received": 2048,
      "packets_lost": 0,
      "jitter": 0.001,
      "rtt": 0.05,
      "bandwidth": 1000000
    },
    "system": {
      "cpu_usage": 25.5,
      "memory_usage": 512000000,
      "gpu_usage": 45.2,
      "fps": 60
    },
    "network": {
      "connection_type": "ethernet",
      "effective_type": "4g",
      "downlink": 10.0,
      "rtt": 50
    }
  }
}
```

## 错误消息

### ERROR 消息

**服务器 → 客户端**

```json
{
  "version": "1.0",
  "type": "error",
  "id": "msg_1640995310000_010",
  "timestamp": 1640995310000,
  "peer_id": "client_001",
  "error": {
    "code": "INVALID_SDP",
    "message": "SDP format is invalid",
    "details": "Missing required 'v=' line in SDP",
    "type": "validation_error",
    "recoverable": true,
    "suggestions": [
      "Check SDP format according to RFC 4566",
      "Retry with valid SDP"
    ]
  }
}
```

### 错误代码定义

| 错误代码 | 类型 | 描述 |
|---------|------|------|
| `CONNECTION_FAILED` | connection_error | 连接建立失败 |
| `CONNECTION_TIMEOUT` | connection_error | 连接超时 |
| `CONNECTION_LOST` | connection_error | 连接丢失 |
| `INVALID_MESSAGE` | validation_error | 消息格式无效 |
| `INVALID_MESSAGE_TYPE` | validation_error | 消息类型无效 |
| `INVALID_MESSAGE_DATA` | validation_error | 消息数据无效 |
| `MESSAGE_TOO_LARGE` | validation_error | 消息过大 |
| `SDP_PROCESSING_FAILED` | webrtc_error | SDP 处理失败 |
| `ICE_CANDIDATE_FAILED` | webrtc_error | ICE 候选处理失败 |
| `PEER_CONNECTION_FAILED` | webrtc_error | 对等连接失败 |
| `SERVER_UNAVAILABLE` | server_error | 服务器不可用 |
| `INTERNAL_ERROR` | server_error | 内部错误 |
| `RATE_LIMITED` | server_error | 请求频率限制 |

## Selkies 兼容格式

### Selkies 文本协议

#### HELLO 消息

```
HELLO 1 eyJyZXMiOiIxOTIweDEwODAiLCJzY2FsZSI6MX0=
```

其中 Base64 编码的部分解码后为：
```json
{
  "res": "1920x1080",
  "scale": 1
}
```

#### 服务器响应

```
HELLO
```

#### 错误消息

```
ERROR Invalid peer ID
```

### Selkies JSON 协议

#### SDP 消息

```json
{
  "sdp": {
    "type": "offer",
    "sdp": "v=0\r\no=- ..."
  }
}
```

#### ICE 候选消息

```json
{
  "ice": {
    "candidate": "candidate:842163049 1 udp 1677729535 192.168.1.100 54400 typ srflx raddr 192.168.1.100 rport 54400 generation 0 ufrag 4ZcD network-cost 999",
    "sdpMid": "0",
    "sdpMLineIndex": 0,
    "usernameFragment": "4ZcD"
  }
}
```

## 协议协商消息

### PROTOCOL-NEGOTIATION 消息

**客户端 → 服务器**

```json
{
  "version": "1.0",
  "type": "protocol-negotiation",
  "id": "msg_1640995320000_011",
  "timestamp": 1640995320000,
  "peer_id": "client_001",
  "data": {
    "supported_protocols": [
      "gstreamer-1.0",
      "selkies"
    ],
    "preferred_protocol": "gstreamer-1.0",
    "client_capabilities": [
      "webrtc",
      "datachannel",
      "video",
      "audio"
    ]
  }
}
```

### PROTOCOL-SELECTED 消息

**服务器 → 客户端**

```json
{
  "version": "1.0",
  "type": "protocol-selected",
  "id": "msg_1640995320001_011",
  "timestamp": 1640995320001,
  "peer_id": "client_001",
  "data": {
    "selected_protocol": "gstreamer-1.0",
    "protocol_version": "1.0",
    "server_capabilities": [
      "webrtc",
      "input",
      "stats",
      "recording"
    ],
    "fallback_protocols": [
      "selkies"
    ]
  }
}
```

## 消息大小限制

### 大小限制

| 消息类型 | 最大大小 | 说明 |
|---------|---------|------|
| 标准消息 | 64KB | 包含完整消息头和数据 |
| SDP 消息 | 32KB | SDP 内容部分 |
| ICE 候选 | 1KB | 单个 ICE 候选 |
| 输入事件 | 512B | 鼠标/键盘事件 |
| 统计数据 | 16KB | 统计信息响应 |

### 大消息处理

对于超过大小限制的消息，可以使用分片机制：

```json
{
  "version": "1.0",
  "type": "message-fragment",
  "id": "msg_1640995330000_012",
  "timestamp": 1640995330000,
  "peer_id": "client_001",
  "data": {
    "fragment_id": "frag_001",
    "fragment_index": 0,
    "total_fragments": 3,
    "original_type": "large-stats",
    "fragment_data": "base64_encoded_fragment_data"
  }
}
```

## 消息验证规则

### 必需字段验证

1. 所有消息必须包含 `version`、`type`、`id`、`timestamp` 字段
2. `version` 必须是支持的版本号
3. `type` 必须是已定义的消息类型
4. `id` 必须是唯一标识符
5. `timestamp` 必须是有效的 Unix 时间戳

### 数据格式验证

1. JSON 格式必须有效
2. 字符编码必须是 UTF-8
3. 数值字段必须在有效范围内
4. 字符串字段不能为空（除非明确允许）

### 业务逻辑验证

1. SDP 消息必须符合 RFC 4566 规范
2. ICE 候选必须包含有效的候选信息
3. 输入事件坐标必须在有效范围内
4. 时间戳不能过于偏离当前时间

## 编码和序列化

### JSON 序列化规则

1. 使用紧凑格式（无额外空格）
2. 字段顺序不敏感
3. 数值使用标准 JSON 数值格式
4. 字符串使用双引号
5. 布尔值使用 `true`/`false`

### 特殊字符处理

1. 换行符在 SDP 中使用 `\r\n`
2. Base64 编码用于二进制数据
3. URL 编码用于特殊字符
4. Unicode 字符使用 UTF-8 编码

## 向后兼容性

### 版本兼容性

1. 新版本必须能处理旧版本消息
2. 未知字段应被忽略而不是拒绝
3. 新字段应有合理的默认值
4. 协议降级应平滑进行

### 格式兼容性

1. 同时支持标准格式和 selkies 格式
2. 自动检测消息格式
3. 透明的格式转换
4. 保持语义一致性

## 性能优化

### 消息优化

1. 使用消息批处理减少网络开销
2. 压缩大型消息（如 SDP）
3. 缓存重复的消息内容
4. 使用增量更新减少数据传输

### 解析优化

1. 流式 JSON 解析
2. 消息类型快速识别
3. 字段验证优化
4. 内存使用优化