# BDWind-GStreamer 快速开始指南

## 🎯 重要更新

**主程序入口已更名为 `bdwind-gstreamer`，与项目名称保持一致！**

## 🚀 三种启动方式

### 方式一：一键启动（推荐）

```bash
# 克隆项目
git clone https://github.com/open-beagle/bdwind-gstreamer.git
cd bdwind-gstreamer

# 一键启动
./scripts/quick-start.sh
```

### 方式二：手动编译启动

```bash
# 编译主程序
go build -o bdwind-gstreamer ./cmd/bdwind-gstreamer

# 启动服务器
./bdwind-gstreamer
```

### 方式三：指定参数启动

```bash
# 指定端口和配置
./bdwind-gstreamer -port 9090 -host 127.0.0.1 -display :0

# 查看帮助
./bdwind-gstreamer --help

# 查看版本
./bdwind-gstreamer --version
```

## 🌐 访问界面

启动后在浏览器中访问：

```
http://localhost:8080
```

## 📊 功能特性

### ✅ 已实现功能
- **Web 管理界面** - 现代化的浏览器界面
- **用户认证系统** - 完整的令牌认证和会话管理
- **系统监控** - 实时的系统资源监控
- **WebSocket 通信** - 实时双向通信
- **配置管理** - 灵活的配置系统
- **健康检查** - 服务状态监控

### 🚧 开发中功能
- **桌面捕获** - GStreamer 桌面捕获模块
- **WebRTC 流媒体** - 实时视频流传输
- **硬件加速** - NVENC/VAAPI 编码支持

## 🛠️ 可用的演示程序

```bash
# 认证系统演示
go run cmd/auth-demo/main.go

# 系统监控演示
go run cmd/system-metrics-demo/main.go

# Web 服务器演示
go run cmd/web-server/main.go

# X11 桌面捕获演示
go run cmd/x11-capture-demo/main.go

# Wayland 桌面捕获演示
go run cmd/wayland-capture-demo/main.go
```

## 📋 API 接口

### 系统状态
```bash
curl http://localhost:8080/api/status
curl http://localhost:8080/health
```

### 监控指标
```bash
curl http://localhost:8080/api/stats
curl http://localhost:8080/metrics
curl http://localhost:9090/metrics
```

### 桌面捕获（开发中）
```bash
curl -X POST http://localhost:8080/api/capture/start
curl -X POST http://localhost:8080/api/capture/stop
```

### 用户认证
```bash
curl -X POST http://localhost:8080/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"demo","password":"password"}'
```

## 🔧 配置选项

### 命令行参数
- `-port` - Web 服务器端口 (默认: 8080)
- `-host` - 服务器主机 (默认: 0.0.0.0)
- `-display` - 显示 ID (默认: :0)
- `-config` - 配置文件路径
- `-version` - 显示版本信息

### 配置文件示例
```yaml
web:
  port: 8080
  host: "0.0.0.0"
  enable_tls: false

capture:
  display_id: ":0"
  frame_rate: 30
  width: 1920
  height: 1080

auth:
  enabled: true
  default_role: "viewer"

monitoring:
  enabled: true
  metrics_port: 9090
```

## 🐛 故障排除

### 编译问题
```bash
# 检查 Go 版本
go version

# 清理模块缓存
go clean -modcache
go mod download
```

### 运行问题
```bash
# 检查端口占用
netstat -tlnp | grep :8080

# 检查显示环境
echo $DISPLAY

# 查看日志
./bdwind-gstreamer 2>&1 | tee server.log
```

## 📚 更多文档

- [📋 完整部署指南](DEPLOYMENT.md)
- [🏗️ 架构设计文档](docs/web-management-interface.md)
- [🔐 认证系统说明](internal/webserver/auth/README.md)
- [📊 监控统计文档](docs/desktop-capture-statistics.md)

## 🎉 成功启动示例

```
🚀 BDWind-GStreamer v1.0.0 started successfully!
📱 Web Interface: http://0.0.0.0:8080
📊 Metrics: http://0.0.0.0:9090/metrics
🖥️  Display: :0
🔐 Authentication: true

Press Ctrl+C to stop
```

现在您就可以通过浏览器访问 `http://localhost:8080` 来使用 BDWind-GStreamer 了！🎊