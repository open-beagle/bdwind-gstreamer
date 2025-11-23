# WebServer 信令服务器迁移实施计划跟踪

## 项目概述

**项目名称**: 信令服务器迁移至WebServer组件 (事件驱动架构)  
**项目目标**: 将信令服务器从`internal/webrtc`迁移至`internal/webserver`，采用事件驱动架构实现组件解耦  
**预计工期**: 4-6个工作日  
**项目状态**: 🟢 已完成

## 执行进度总览

| 阶段 | 任务 | 状态 | 预计工期 | 实际工期 | 完成度 |
|------|------|------|----------|----------|--------|
| Step 1 | 协议定义公共化和代码清理 | 🟢 已完成 | 1天 | 1天 | 100% |
| Step 2 | SignalingServer迁移 | 🟢 已完成 | 1-2天 | 1天 | 100% |
| Step 3 | SignalingClient重构 | 🟢 已完成 | 1-2天 | 1天 | 100% |
| Step 4 | 系统集成 | 🟢 已完成 | 1天 | 1天 | 100% |

**总体进度**: 100% (4/4 步骤完成)

## Step 1: 协议公共化 (1天)

### 核心任务

- [x] **1.1 协议定义迁移**
  - [x] 将 `internal/webrtc/protocol` 移到 `internal/common/protocol` (已完成)
  - [x] 更新所有相关文件的导入路径 (无需更新，未发现现有导入)
  - [x] 删除旧的 `internal/webrtc/protocol` 目录 (已删除)
  - [x] 确保信令服务器和客户端都能使用公共协议 (已迁移完整协议定义)

- [x] **1.2 WebRTC代码清理**
  - [x] 删除旧的 `internal/webrtc/manager.go` (已删除)
  - [x] 将 `internal/webrtc/minimal_manager.go` 重命名为 `manager.go` (已完成)
  - [x] 清理不再使用的旧代码和依赖 (已完成)

- [x] **1.3 WebRTC事件系统**
  - [x] 在 `internal/webrtc/events/` 创建WebRTC专用事件系统 (已创建)
  - [x] 事件系统只服务于WebRTC组件内部解耦 (已实现)

### 人工确认点
- ✅ 协议迁移是否影响现有功能？ (已确认)
- ✅ 旧的protocol目录是否已完全删除？ (已确认)
- ✅ WebRTC代码清理是否完整？ (已确认)
- ✅ minimal_manager.go重命名后功能是否正常？ (已确认)
- ✅ WebRTC事件系统设计是否合理？ (已确认)

---

## Step 2: SignalingServer迁移 (1-2天)

### 核心任务

- [x] **2.1 创建WebServer信令服务器**
  - [x] 创建 `internal/webserver/signaling_server.go` - 主服务器
  - [x] 创建 `internal/webserver/signaling_router.go` - 消息路由
  - [x] 实现WebSocket连接管理和消息转发

- [x] **2.2 实现双客户端管理**
  - [x] 实现推流客户端注册和管理
  - [x] 实现UI客户端连接和管理
  - [x] 实现基于ID的消息路由

- [x] **2.3 清理旧文件**
  - [x] 删除 `internal/webrtc/signalingserver.go`
  - [x] 删除 `internal/webserver/signaling/server.go` (如果存在)
  - [x] 更新所有引用旧SignalingServer的代码

### 人工确认点
- ✅ WebSocket连接是否正常建立？ (已确认 - 2025-11-20)
- ✅ 消息路由逻辑是否正确？ (已确认 - 2025-11-20)
- ✅ 客户端注册流程是否完整？ (已确认 - 2025-11-20)
- ✅ 旧文件是否已完全清理？ (已确认 - 2025-11-20)

---

## Step 3: SignalingClient重构 (1-2天)

### 核心任务

- [x] **3.1 移除直接依赖**
  - [x] 移除 SignalingClient 中的 webrtcManager 直接引用
  - [x] 改为使用事件系统进行通信

- [x] **3.2 创建事件处理器**
  - [x] 创建 `internal/webrtc/signaling_event_handlers.go` - 信令事件处理
  - [x] 重构消息处理逻辑使用事件委托

### 人工确认点
- ✅ SignalingClient是否成功解耦？ (已确认 - 2025-11-20)
- ✅ 事件处理逻辑是否正确？ (已确认 - ICE候选格式已修复)
- ✅ WebRTC协商流程是否完整？ (已确认 - 2025-11-20 19:32 连接成功)

---

## Step 4: 系统集成 (1天)

### 核心任务

- [x] **4.1 更新应用初始化**
  - [x] 修改 `cmd/bdwind-gstreamer/app.go` - 集成新的组件架构
  - [x] 添加事件总线创建和启动逻辑
  - [x] 创建信令事件处理器并注册到事件总线
  - [x] 配置事件系统和组件间的连接关系

- [x] **4.2 更新WebServer管理器**
  - [x] 修改 `internal/webserver/manager.go` - 添加信令服务器组件
  - [x] 集成信令服务器到WebServer管理器生命周期
  - [x] 添加事件总线配置方法
  - [x] 确保WebSocket路由正确配置到新的信令服务器

- [x] **4.3 事件系统集成**
  - [x] 为SignalingServer添加事件总线支持
  - [x] 更新SignalingRouter支持事件驱动的WebRTC操作
  - [x] 实现REQUEST_OFFER、ANSWER、ICE_CANDIDATE的事件处理
  - [x] 保持消息路由作为回退机制

- [x] **4.4 系统启动流程验证**
  - [x] 验证事件总线在应用启动时正确初始化
  - [x] 确保信令事件处理器正确注册
  - [x] 验证WebServer管理器正确集成信令服务器
  - [x] 确保组件间事件连接关系正确建立

### 人工确认点
- ✅ 系统是否能正常启动？ (已确认 - 2025-11-20)
- ✅ 所有组件是否正确初始化？ (已确认 - 2025-11-20)
- ✅ WebSocket路由是否工作正常？ (已确认 - 2025-11-20)
- ✅ 事件系统是否正确配置？ (已确认 - 2025-11-20)
- ✅ 信令服务器是否正确集成到WebServer？ (已确认 - 2025-11-20)

---

## 项目里程碑

| 里程碑 | 日期 | 状态 | 备注 |
|--------|------|------|------|
| 项目启动 | 2025-11-04 | 🟢 已完成 | 开始迁移工作 |
| Step 1 完成 | 2025-11-04 | 🟢 已完成 | 协议定义公共化和代码清理完成 |
| Step 2 完成 | 2025-11-04 | 🟢 已完成 | 服务器迁移完成 |
| Step 3 完成 | 2025-11-20 | 🟢 已完成 | 客户端重构完成，WebRTC连接成功 |
| Step 4 完成 | 2025-11-06 | 🟢 已完成 | 系统集成完成 |
| 项目交付 | 2025-11-20 | 🟢 已完成 | 迁移全部完成，WebRTC连接验证通过 |

---

## 更新日志

### 2025-11-22
- ✅ **IPv6支持启用**
  - 移除了 `internal/webrtc/manager.go` 中的IPv6候选过滤逻辑
  - 服务器现在支持IPv4和IPv6双栈ICE候选
  - 优化了日志输出，避免字符串越界错误

- ⚠️ **IPv6实际使用情况**
  - 测试发现服务器只有link-local IPv6地址（`fe80::`），无法用于远程连接
  - 需要全局IPv6地址（如 `2001::` 或 `2400::` 开头）才能生成有效的IPv6候选
  - 当前STUN服务器只返回IPv4公网地址

- ⚠️ **NAT穿透问题**
  - 测试发现在对称NAT环境下，仅使用STUN无法成功打洞
  - **建议添加TURN服务器**以支持所有NAT类型的穿透
  - 公网候选 `43.224.73.222:65004` 无法与客户端 `221.223.24.7` 建立连接

### 2025-11-20
- ✅ 完成Step 2验证: WebSocket信令服务器
  - WebSocket连接正常建立
  - 消息路由逻辑正确
  - 客户端注册流程完整
  - ICE候选数据格式错误已修复
    - 修复 `internal/webrtc/manager.go` 中ICE候选事件发布格式
    - 修复 `internal/webrtc/signaling_client.go` 中ICE候选消息发送格式
    - 将 `ICECandidateInit` 正确转换为 `map[string]interface{}`
    - 消除了 "Invalid candidate data format" 错误

- ✅ 完成Step 3: SignalingClient重构和WebRTC连接
  - ✅ 重构 `internal/webrtc/signaling_client.go`，移除 WebSocket 直接依赖
  - ✅ 引入 `SendFunc` 回调，实现消息发送解耦
  - ✅ 确保所有 WebRTC 操作（Offer, Answer, ICE）通过 `EventBus` 通信
  - ✅ 更新 `internal/webserver/signaling_server.go` 添加 `SendMessageByClientID`
  - ✅ 在 `cmd/bdwind-gstreamer/app.go` 中实现 `SignalingMessageHandler` 进行集成
  - ✅ 验证了事件处理流程与 `SignalingEventHandlers` 的对齐
  - ✅ **ICE服务器配置传递** - 通过welcome消息传递给客户端
  - ✅ **PeerConnection按需创建** - 解决了时序问题
  - ✅ **WebRTC连接成功** - ICE连接建立，状态变为connected (19:32)

### 2025-11-06
- ✅ 完成Step 4: 系统集成和配置更新
  - 修改了 `cmd/bdwind-gstreamer/app.go` 集成新的组件架构
  - 添加了事件总线创建、启动和生命周期管理
  - 创建了信令事件处理器并注册到事件总线
  - 更新了 `internal/webserver/manager.go` 添加信令服务器组件
  - 集成了信令服务器到WebServer管理器生命周期
  - 配置了事件系统和组件间的连接关系
  - 为SignalingServer和SignalingRouter添加了事件总线支持
  - 实现了事件驱动的WebRTC操作处理（CREATE_OFFER、PROCESS_ANSWER、ADD_ICE_CANDIDATE）
  - 保持了消息路由作为回退机制以确保兼容性
  - 验证了系统启动和组件初始化流程

### 2025-11-04
- 📝 创建简化的迁移实施计划
- 🎯 专注于核心迁移任务，去除测试验证环节
- 🤝 采用人工确认方式进行质量控制
- ✅ 完成Step 2: SignalingServer迁移
  - 创建了新的WebServer信令服务器 (`internal/webserver/signaling_server.go`)
  - 创建了消息路由器 (`internal/webserver/signaling_router.go`)
  - 实现了双客户端管理（推流客户端和UI客户端）
  - 实现了基于ID的消息路由机制
  - 删除了旧的信令服务器文件
  - 添加了兼容性层以保持编译通过

---

## 下一步行动

**项目状态：⚠️ 需要改进 - NAT穿透问题**

### 待解决问题

1. **TURN服务器集成**（高优先级）
   - 当前仅使用STUN，在对称NAT环境下无法打洞
   - 需要添加TURN服务器配置以支持所有NAT类型
   - 建议使用开源TURN服务器如 coturn

2. **IPv6支持完善**（中优先级）
   - 代码已支持IPv6，但服务器缺少全局IPv6地址
   - 需要配置全局IPv6地址或使用支持IPv6的云服务器
   - 需要支持IPv6的STUN/TURN服务器

3. **ICE候选优化**（低优先级）
   - 考虑实现ICE候选优先级调整
   - 优先使用低延迟的候选路径

### 已完成的所有任务
- ✅ WebSocket信令服务器正常工作
- ✅ ICE候选格式错误已修复
- ✅ ICE服务器配置正确传递（服务器→客户端）
- ✅ PeerConnection按需创建（解决时序问题）
- ✅ ICE连接成功建立
- ✅ WebRTC协商流程完整
- ✅ IPv6支持已启用（2025-11-22）

### 关键修复总结
1. **ICE候选数据格式** - 将 `ICECandidateInit` 正确转换为 `map[string]interface{}`
2. **ICE服务器配置传递** - 通过welcome消息的 `SessionConfig` 传递
3. **PeerConnection创建时序** - 改为按需创建，而不是启动时创建
4. **客户端流程优化** - 先收到welcome消息，再创建PeerConnection
5. **IPv6支持** - 移除IPv6候选过滤，支持IPv4和IPv6双栈连接（2025-11-22）

### 测试结果（2025-11-20 19:32）
```
✅ WebSocket连接成功
✅ 收到4个服务器ICE候选并成功添加
✅ ICE连接状态: checking → connected
✅ PeerConnection状态: connecting → connected
✅ WebRTC连接建立成功
```

### 生成的ICE候选
服务器端生成ICE候选（支持IPv4和IPv6双栈）：
- **IPv4候选示例**：
  1. `192.168.1.246:65003` - 局域网host候选
  2. `172.17.0.1:65002` - Docker网络host候选
  3. `10.2.5.36:65002` - 内网host候选
  4. `43.224.73.222:65004` - 公网srflx候选（通过STUN获取）
- **IPv6候选**：根据网络环境自动生成（2025-11-22起支持）

客户端成功添加所有候选并建立连接。

---

*最后更新: 2024-11-20*  
*文档版本: v1.1*  
*维护者: 开发团队*