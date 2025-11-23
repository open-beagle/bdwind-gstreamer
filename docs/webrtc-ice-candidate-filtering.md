# ICE 候选筛选配置指南

## 概述

WebRTC 连接需要通过 ICE (Interactive Connectivity Establishment) 协议来建立点对点连接。本系统现在支持灵活的 ICE 候选筛选策略，可以优先选择特定类型的候选来提高连接成功率。

## ICE 候选类型

### 1. HOST（主机候选）

- **描述**: 本地网络接口的直接地址
- **适用场景**: 局域网内连接
- **优点**: 延迟最低，无需外部服务器
- **缺点**: 无法穿透 NAT

### 2. SRFLX（服务器反射候选）

- **描述**: 通过 STUN 服务器获取的公网地址
- **适用场景**: 需要 NAT 穿透的场景
- **优点**: 可以穿透大多数 NAT，延迟较低
- **缺点**: 需要 STUN 服务器支持

### 3. RELAY（中继候选）

- **描述**: 通过 TURN 服务器中继的连接
- **适用场景**: 严格防火墙环境
- **优点**: 连接成功率最高
- **缺点**: 延迟较高，需要 TURN 服务器资源

## 配置策略

### 默认策略：优先 SRFLX

```javascript
{
  preferredTypes: ['srflx'],
  allowedTypes: ['srflx', 'relay', 'host'],
  strictMode: false,
  blockIPv6LinkLocal: true
}
```

这是推荐的默认配置，适合大多数 NAT 穿透场景。

### 可选策略

#### 1. 仅 SRFLX（严格模式）

```javascript
{
  preferredTypes: ['srflx'],
  allowedTypes: ['srflx'],
  strictMode: true
}
```

只使用 SRFLX 候选，确保连接通过 STUN 服务器。

#### 2. 优先 RELAY

```javascript
{
  preferredTypes: ['relay'],
  allowedTypes: ['relay', 'srflx', 'host'],
  strictMode: false
}
```

优先使用 TURN 中继，适合防火墙严格的环境。

#### 3. 仅 RELAY（强制 TURN）

```javascript
{
  preferredTypes: ['relay'],
  allowedTypes: ['relay'],
  strictMode: true
}
```

强制使用 TURN 中继，确保所有流量通过 TURN 服务器。

#### 4. 全部类型

```javascript
{
  preferredTypes: ['srflx', 'relay', 'host'],
  allowedTypes: ['srflx', 'relay', 'host'],
  strictMode: false
}
```

允许所有类型的候选，让 WebRTC 自动选择最佳路径。

## 使用方法

### 在 HTML 界面中配置

1. 点击 "ICE 配置" 按钮打开配置面板
2. 从下拉菜单中选择连接策略
3. 可选：勾选/取消勾选 "阻止 IPv6 Link-Local 地址"
4. 配置会立即应用到 WebRTC 连接

### 通过代码配置

```javascript
// 创建 WebRTC 管理器
const webrtc = new WebRTCManager(signaling, videoElement, peerId);

// 设置 ICE 筛选配置
webrtc.setICEFilterConfig({
  preferredTypes: ["srflx"],
  allowedTypes: ["srflx", "relay", "host"],
  strictMode: false,
  blockIPv6LinkLocal: true,
});

// 获取当前配置
const config = webrtc.getICEFilterConfig();
console.log("当前 ICE 配置:", config);
```

## 故障排查

### 连接失败：Peer connection failed

**可能原因**:

1. 当前策略过于严格，过滤掉了所有可用候选
2. STUN/TURN 服务器不可用
3. 网络防火墙阻止了 UDP 流量

**解决方案**:

1. 尝试切换到 "全部类型" 策略
2. 检查 STUN/TURN 服务器配置
3. 如果仍然失败，尝试 "仅 RELAY" 策略

### 连接延迟高

**可能原因**:
使用了 RELAY 候选，流量通过 TURN 服务器中继

**解决方案**:

1. 切换到 "优先 SRFLX" 策略
2. 检查网络环境是否支持 UDP 打洞
3. 优化 TURN 服务器部署位置

### 日志分析

查看浏览器控制台日志，关注以下信息：

```
📤 生成本地 ICE 候选:
   类型: srflx (ipv4)
   协议: udp
   地址: 221.223.24.75:64012
   ✅ 候选通过筛选（优先类型: srflx），将发送到服务器

📥 收到远程 ICE 候选:
   类型: srflx (ipv4)
   协议: udp
   地址: 43.224.73.222:65003
   ✅ 候选通过筛选（优先类型: srflx），将添加到PeerConnection
```

如果看到 "❌ 筛选原因: ..." 说明候选被过滤掉了。

## 最佳实践

1. **开发环境**: 使用 "全部类型" 策略，便于调试
2. **生产环境**: 使用 "优先 SRFLX" 策略，平衡连接成功率和延迟
3. **严格防火墙**: 使用 "优先 RELAY" 或 "仅 RELAY" 策略
4. **局域网**: 可以使用 HOST 候选，延迟最低

## 技术细节

### 筛选流程

1. **本地候选生成**: 当 WebRTC 生成本地 ICE 候选时，`_filterLocalCandidate()` 方法会检查候选类型
2. **远程候选接收**: 当收到远程 ICE 候选时，`_filterRemoteCandidate()` 方法会检查候选类型
3. **筛选决策**:
   - 严格模式：只允许 `preferredTypes` 中的类型
   - 宽松模式：允许 `allowedTypes` 中的类型，优先使用 `preferredTypes`

### 候选解析

系统会自动解析 ICE 候选字符串，提取以下信息：

- 候选类型 (host/srflx/relay)
- IP 版本 (IPv4/IPv6)
- 传输协议 (UDP/TCP)
- 地址和端口
- 优先级
- 相关地址（对于 srflx 和 relay）

所有信息都会在日志中详细显示，便于调试。
