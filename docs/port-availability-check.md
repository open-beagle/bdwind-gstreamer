# 端口可用性检查功能

## 概述

**日期：** 2025年8月23日  
**功能：** 启动前端口占用检查  
**文件：** `cmd/bdwind-gstreamer/main.go`  
**状态：** 已实现

## 问题背景

在启动 BDWind-GStreamer 服务时，如果默认端口 8080 被其他应用占用，程序会在启动过程中失败，但错误信息不够明确，用户体验不佳。

## 解决方案

### 实现的功能

1. **启动前端口检查**
   - 在应用启动前检查所有需要的端口
   - 如果端口被占用，立即退出并显示清晰的错误信息

2. **检查的端口类型**
   - WebServer 端口（默认 8080）
   - WebRTC Signaling 端口（默认 8081）
   - WebRTC Media 端口（默认 8082）
   - Metrics 端口（如果启用）

3. **双协议检查**
   - TCP 端口检查
   - UDP 端口检查

### 代码实现

#### 主要函数

```go
// checkPortAvailability 检查配置中的端口是否可用
func checkPortAvailability(cfg *config.Config) error {
    portsToCheck := make(map[int]string)

    // 添加 WebServer 端口
    if cfg.WebServer != nil {
        portsToCheck[cfg.WebServer.Port] = "WebServer"
    }

    // 添加 WebRTC 相关端口
    if cfg.WebRTC != nil {
        if cfg.WebRTC.SignalingPort != 0 {
            portsToCheck[cfg.WebRTC.SignalingPort] = "WebRTC Signaling"
        }
        if cfg.WebRTC.MediaPort != 0 {
            portsToCheck[cfg.WebRTC.MediaPort] = "WebRTC Media"
        }
    }

    // 添加 Metrics 端口
    if cfg.Metrics != nil && cfg.Metrics.External.Enabled {
        portsToCheck[cfg.Metrics.External.Port] = "Metrics"
    }

    // 检查每个端口
    for port, service := range portsToCheck {
        if err := checkPortInUse(port); err != nil {
            return fmt.Errorf("%s port %d is already in use: %v", service, port, err)
        }
        log.Printf("  ✅ Port %d (%s) is available", port, service)
    }

    return nil
}

// checkPortInUse 检查指定端口是否被占用
func checkPortInUse(port int) error {
    // 检查 TCP 端口
    tcpAddr := fmt.Sprintf(":%d", port)
    tcpListener, err := net.Listen("tcp", tcpAddr)
    if err != nil {
        return fmt.Errorf("TCP port occupied: %v", err)
    }
    tcpListener.Close()

    // 检查 UDP 端口（对于某些服务可能需要）
    udpAddr := fmt.Sprintf(":%d", port)
    udpConn, err := net.ListenPacket("udp", udpAddr)
    if err != nil {
        return fmt.Errorf("UDP port occupied: %v", err)
    }
    udpConn.Close()

    return nil
}
```

#### 集成到主程序

```go
// 验证配置
if err := cfg.Validate(); err != nil {
    log.Fatalf("Invalid configuration: %v", err)
}

// 检查端口占用
log.Printf("Checking port availability...")
if err := checkPortAvailability(cfg); err != nil {
    log.Printf("❌ Port availability check failed: %v", err)
    log.Printf("💡 Please ensure the required ports are not in use by other applications")
    log.Printf("💡 You can check port usage with: netstat -tlnp | grep :<port>")
    os.Exit(1)
}
log.Printf("✅ All required ports are available")
```

## 使用示例

### 端口可用时

```bash
$ ./bdwind-gstreamer --port 8080
2025/08/23 23:40:17 Checking port availability...
2025/08/23 23:40:17   ✅ Port 8080 (WebServer) is available
2025/08/23 23:40:17   ✅ Port 8081 (WebRTC Signaling) is available
2025/08/23 23:40:17   ✅ Port 8082 (WebRTC Media) is available
2025/08/23 23:40:17 ✅ All required ports are available
2025/08/23 23:40:17 SDP generator created with codec: h264
...
🚀 BDWind-GStreamer v1.0.0 started successfully!
```

### 端口被占用时

```bash
$ ./bdwind-gstreamer --port 8080
2025/08/23 23:38:52 Checking port availability...
2025/08/23 23:38:52 ❌ Port availability check failed: WebServer port 8080 is already in use: TCP port occupied: listen tcp :8080: bind: address already in use
2025/08/23 23:38:52 💡 Please ensure the required ports are not in use by other applications
2025/08/23 23:38:52 💡 You can check port usage with: netstat -tlnp | grep :<port>
```

## 测试验证

### 测试场景 1：端口可用

```bash
# 确保端口未被占用
$ ss -tlnp | grep :8080
# (无输出)

# 启动应用
$ ./bdwind-gstreamer --port 8080
# 应用正常启动
```

### 测试场景 2：端口被占用

```bash
# 占用端口
$ python3 -m http.server 8080 &

# 启动应用
$ ./bdwind-gstreamer --port 8080
# 应用检测到端口占用并退出
```

## 技术细节

### 检查方法

1. **TCP 端口检查**
   - 使用 `net.Listen("tcp", ":port")` 尝试绑定端口
   - 如果成功，立即关闭监听器
   - 如果失败，说明端口被占用

2. **UDP 端口检查**
   - 使用 `net.ListenPacket("udp", ":port")` 尝试绑定端口
   - 如果成功，立即关闭连接
   - 如果失败，说明端口被占用

### 错误处理

1. **立即退出**
   - 使用 `os.Exit(1)` 而不是 `log.Fatalf()`
   - 提供更好的错误信息和建议

2. **用户友好的提示**
   - 显示具体哪个端口被占用
   - 提供检查端口使用情况的命令
   - 给出解决建议

## 优势

1. **早期发现问题**
   - 在应用启动前就发现端口冲突
   - 避免部分初始化后的失败

2. **清晰的错误信息**
   - 明确指出哪个服务的端口被占用
   - 提供具体的端口号和错误原因

3. **用户友好**
   - 提供解决问题的建议
   - 显示检查端口的命令

4. **全面检查**
   - 检查所有配置的端口
   - 同时检查 TCP 和 UDP 协议

## 注意事项

1. **权限要求**
   - 某些端口可能需要特殊权限
   - 1024 以下的端口通常需要 root 权限

2. **临时占用**
   - 检查时会临时绑定端口
   - 立即释放，不会长期占用

3. **配置依赖**
   - 检查基于当前配置
   - 如果配置更改，检查结果可能不同

## 未来改进

1. **更详细的端口信息**
   - 显示占用端口的进程信息
   - 提供更具体的解决建议

2. **可选的端口检查**
   - 添加命令行参数跳过端口检查
   - 用于特殊部署场景

3. **端口范围检查**
   - 支持检查端口范围
   - 自动寻找可用端口

---

**实现者**：Kiro AI Assistant  
**文档版本**：1.0  
**最后更新**：2025年8月23日