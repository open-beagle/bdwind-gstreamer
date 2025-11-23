# WebRTC NAT 穿透问题分析

## 测试时间

2025-11-22 10:18

## 问题描述

WebRTC 连接在 ICE checking 阶段失败，无法建立 P2P 连接。

## 测试环境

### 服务器端

- **公网 IP**: 43.224.73.222（通过 STUN 获取）
- **内网 IP**:
  - 192.168.1.246/24（局域网）
  - 172.17.0.1/16（Docker）
  - 10.2.5.36/32（Cilium）
- **IPv6**: 仅有 link-local 地址（fe80::），无全局 IPv6 地址
- **STUN 服务器**: stun.ali.wodcloud.com:3478
- **TURN 服务器**: 无

### 客户端

- **公网 IP**: 221.223.24.7
- **NAT 类型**: 未知（疑似对称 NAT）

## ICE 候选分析

### 服务器生成的候选（4 个 IPv4 候选）

| 候选地址      | 端口  | 类型  | 优先级     | 说明                  |
| ------------- | ----- | ----- | ---------- | --------------------- |
| 192.168.1.246 | 65002 | host  | 2130706431 | 局域网地址            |
| 172.17.0.1    | 65002 | host  | 2130706431 | Docker 网络           |
| 10.2.5.36     | 65002 | host  | 2130706431 | Cilium 网络           |
| 43.224.73.222 | 65004 | srflx | 1694498815 | STUN 反射地址（公网） |

### 客户端生成的候选（3 个）

| 候选地址        | 类型  | 说明                  |
| --------------- | ----- | --------------------- |
| e9777c87-b35... | host  | 本地地址              |
| 2cea573d-4e8... | host  | 本地地址              |
| 221.223.24.7... | srflx | STUN 反射地址（公网） |

## 连接失败原因分析

### 1. 缺少 IPv6 候选

- **现象**: 服务器和客户端都没有生成 IPv6 候选
- **原因**:
  - 服务器只有 link-local IPv6 地址（fe80::），不能用于远程连接
  - 缺少全局 IPv6 地址（2001::或 2400::开头）
  - STUN 服务器可能不支持 IPv6
- **影响**: 无法利用 IPv6 网络进行连接

### 2. 对称 NAT 问题

- **现象**:
  - 公网候选 43.224.73.222:65004 和 221.223.24.7 之间无法建立连接
  - ICE 状态从 checking → disconnected → failed
  - 候选对显示 0 bytes sent/received
- **原因**:
  - 双方或其中一方可能使用对称 NAT
  - 对称 NAT 会为每个目标分配不同的端口映射
  - STUN 无法解决对称 NAT 问题
- **影响**: P2P 连接失败

### 3. 缺少 TURN 服务器

- **现象**: 配置中只有 STUN 服务器，没有 TURN 服务器
- **原因**: TURN 服务器需要额外部署和配置
- **影响**:
  - 无法在对称 NAT 环境下建立连接
  - 无法使用中继方式传输数据
  - 连接成功率低

## 连接流程时序

```
10:18:21 - WebSocket连接成功
10:18:21 - 收到welcome消息，获取ICE服务器配置
10:18:21 - 创建PeerConnection
10:18:22 - 请求并收到Offer
10:18:22 - 收到4个服务器ICE候选（全部IPv4）
10:18:22 - 设置远程描述，创建Answer
10:18:22 - ICE状态: gathering → checking
10:18:22 - 连接状态: connecting
10:18:22 - 生成3个客户端ICE候选
10:18:22 - ICE收集完成
10:18:37 - ICE状态: disconnected（15秒后超时）
10:18:37 - 连接状态: failed
```

## 解决方案

### 方案 1：部署 TURN 服务器（推荐）

#### 优点

- 支持所有 NAT 类型
- 连接成功率接近 100%
- 可以使用开源方案（coturn）

#### 实施步骤

1. 部署 coturn 服务器
2. 配置 TURN 服务器地址和凭证
3. 更新 config.yaml 配置：

```yaml
webrtc:
  ice_servers:
    - urls: ["stun:stun.ali.wodcloud.com:3478"]
    - urls: ["turn:your-turn-server.com:3478"]
      username: "your-username"
      credential: "your-password"
```

#### 参考资源

- coturn 项目: https://github.com/coturn/coturn
- 部署文档: docs/gstreamer-tech-design.md

### 方案 2：启用 IPv6 支持

#### 前提条件

- 服务器需要全局 IPv6 地址
- 客户端网络支持 IPv6
- STUN/TURN 服务器支持 IPv6

#### 实施步骤

1. 配置服务器获取全局 IPv6 地址
2. 使用支持 IPv6 的 STUN/TURN 服务器
3. 验证 IPv6 候选生成

#### 优点

- 更好的连接性
- 避免 IPv4 NAT 问题
- 未来趋势

#### 缺点

- 需要网络环境支持
- 部署复杂度较高

### 方案 3：网络拓扑优化

#### 适用场景

- 内网环境
- 可控网络环境

#### 实施步骤

1. 配置端口转发
2. 使用 DMZ 主机
3. 配置防火墙规则

## 代码改进（2025-11-22）

### 前端 ICE 候选筛选和日志

已在 `internal/webserver/static/webrtc.html` 和 `internal/webserver/static/webrtc.js` 中添加：

1. **ICE 候选解析功能**

   - `parseCandidateString()`: 解析候选字符串，提取类型、地址、端口等信息
   - `detectIPVersion()`: 检测 IP 版本（IPv4/IPv6/IPv4-mapped）

2. **ICE 候选筛选功能**

   - `filterLocalCandidate()`: 筛选本地生成的候选
   - `filterRemoteCandidate()`: 筛选服务器发送的候选
   - 过滤规则：
     - 过滤无效候选（缺少地址或类型）
     - 过滤 IPv6 link-local 地址（fe80::）
     - 记录 IPv6 候选用于调试
     - 保留所有有效候选供 WebRTC 选择

3. **详细日志输出**
   - 记录每个候选的类型、协议、地址、端口、优先级
   - 记录筛选决策和原因
   - 区分本地生成和远程接收的候选
   - 使用 emoji 图标增强可读性

### WebRTC.js 改进

在 `internal/webserver/static/webrtc.js` 中添加：

- `_parseICECandidate()`: 候选解析方法
- `_detectIPVersion()`: IP 版本检测
- `_logICECandidate()`: 候选日志记录
- `_filterLocalCandidate()`: 本地候选筛选
- `_filterRemoteCandidate()`: 远程候选筛选

## 测试建议

### 1. NAT 类型检测

使用工具检测双方 NAT 类型：

- Full Cone NAT: STUN 可以工作
- Restricted Cone NAT: STUN 可以工作
- Port Restricted Cone NAT: STUN 可以工作
- Symmetric NAT: 需要 TURN 服务器

### 2. 网络连通性测试

```bash
# 测试STUN服务器
nc -u stun.ali.wodcloud.com 3478

# 测试端口可达性
nc -zv 43.224.73.222 65004
```

### 3. WebRTC 诊断

- 使用 chrome://webrtc-internals/ 查看详细连接信息
- 检查 ICE 候选对的连接尝试
- 分析失败原因
- 查看浏览器控制台的详细 ICE 候选日志

## 相关文档

- [WebRTC 技术设计](gstreamer-tech-design.md)
- [信令服务器实施计划](webserver-signalingserver-summary.md)
- [故障排查指南](gstreamer-troubleshooting.md)

## 更新日志

- 2025-11-22: 初始版本，分析 NAT 穿透失败问题
