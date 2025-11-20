# WebServer 信令服务器迁移实施计划跟踪

## 项目概述

**项目名称**: 信令服务器迁移至WebServer组件 (事件驱动架构)  
**项目目标**: 将信令服务器从`internal/webrtc`迁移至`internal/webserver`，采用事件驱动架构实现组件解耦  
**预计工期**: 4-6个工作日  
**项目状态**: 🟡 规划中

## 执行进度总览

| 阶段 | 任务 | 状态 | 预计工期 | 实际工期 | 完成度 |
|------|------|------|----------|----------|--------|
| Step 1 | 协议定义公共化和代码清理 | 🟢 已完成 | 1天 | 1天 | 100% |
| Step 2 | SignalingServer迁移 | 🟢 已完成 | 1-2天 | 1天 | 100% |
| Step 3 | SignalingClient重构 | 🔴 未开始 | 1-2天 | - | 0% |
| Step 4 | 系统集成 | 🟢 已完成 | 1天 | 1天 | 100% |

**总体进度**: 75% (3/4 步骤完成)

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
- 🤝 协议迁移是否影响现有功能？
- 🤝 旧的protocol目录是否已完全删除？
- 🤝 WebRTC代码清理是否完整？
- 🤝 minimal_manager.go重命名后功能是否正常？
- 🤝 WebRTC事件系统设计是否合理？

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
- 🤝 WebSocket连接是否正常建立？
- 🤝 消息路由逻辑是否正确？
- 🤝 客户端注册流程是否完整？
- 🤝 旧文件是否已完全清理？

---

## Step 3: SignalingClient重构 (1-2天)

### 核心任务

- [ ] **3.1 移除直接依赖**
  - [ ] 移除 SignalingClient 中的 webrtcManager 直接引用
  - [ ] 改为使用事件系统进行通信

- [ ] **3.2 创建事件处理器**
  - [ ] 创建 `internal/webrtc/signaling_event_handlers.go` - 信令事件处理
  - [ ] 重构消息处理逻辑使用事件委托

### 人工确认点
- 🤝 SignalingClient是否成功解耦？
- 🤝 事件处理逻辑是否正确？
- 🤝 WebRTC协商流程是否完整？

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
- 🤝 系统是否能正常启动？
- 🤝 所有组件是否正确初始化？
- 🤝 WebSocket路由是否工作正常？
- 🤝 事件系统是否正确配置？
- 🤝 信令服务器是否正确集成到WebServer？

---

## 项目里程碑

| 里程碑 | 日期 | 状态 | 备注 |
|--------|------|------|------|
| 项目启动 | 2024-11-04 | 🟢 已完成 | 开始迁移工作 |
| Step 1 完成 | 2024-11-04 | 🟢 已完成 | 协议定义公共化和代码清理完成 |
| Step 2 完成 | - | 🔴 待定 | 服务器迁移完成 |
| Step 3 完成 | - | 🔴 待定 | 客户端重构完成 |
| Step 4 完成 | 2024-11-06 | 🟢 已完成 | 系统集成完成 |
| 项目交付 | - | 🟡 待Step 3 | 迁移全部完成 |

---

## 更新日志

### 2024-11-06
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

### 2024-11-04
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

**准备开始 Step 3: SignalingClient事件驱动重构**

当前状态：
- ✅ Step 1、Step 2 和 Step 4 已完成
- 🔄 SignalingClient 已临时禁用 (使用 `+build ignore`)
- 📋 需要重构 SignalingClient 使用事件系统
- ✅ 事件系统已完全集成到应用架构中
- ✅ SignalingServer 已支持事件驱动的WebRTC操作

下一步将：
1. 移除 SignalingClient 中对 webrtcManager 的直接引用
2. 重构 SignalingClient 使用事件系统进行 WebRTC 通信
3. 重新启用 SignalingClient 并确保功能完整
4. 验证端到端的事件驱动信令流程

**注意**: Step 4 已提前完成，为 Step 3 的 SignalingClient 重构提供了完整的事件系统基础设施。

---

*最后更新: 2024-XX-XX*  
*文档版本: v1.0*  
*维护者: 开发团队*