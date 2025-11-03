# BDWind-GStreamer

[![Go Version](https://img.shields.io/badge/Go-1.21+-blue.svg)](https://golang.org)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![Build Status](https://img.shields.io/badge/Build-Passing-brightgreen.svg)](https://github.com/open-beagle/bdwind-gstreamer)

**BDWind-GStreamer** 是一个基于 Golang 开发的高性能 WebRTC 桌面流媒体服务器，专为 Linux 桌面环境设计，支持高帧率、低延迟的实时桌面共享和游戏流媒体传输。

## ✨ 特性

### 🖥️ 桌面捕获

- **多显示服务器支持**：X11 和 Wayland
- **多显示器支持**：支持多显示器环境
- **区域捕获**：支持特定区域捕获
- **高性能**：硬件加速编码 (NVENC/VAAPI)
- **低延迟**：优化的捕获管道

### 🌐 Web 管理界面

- **现代 Web UI**：基于 Vue.js 的响应式界面
- **实时控制**：WebSocket 实时通信
- **移动端支持**：响应式设计，支持触摸操作
- **多语言支持**：中英文界面

### 🔐 安全认证

- **基于令牌的认证**：JWT 令牌认证
- **会话管理**：完整的会话生命周期管理
- **角色权限**：基于角色的访问控制 (RBAC)
- **安全审计**：完整的操作日志记录

### 📊 监控统计

- **实时监控**：Prometheus 指标收集
- **性能统计**：帧率、延迟、丢帧统计
- **系统监控**：CPU、内存、GPU 使用率
- **网络监控**：带宽使用和连接状态

### 🚀 高性能

- **硬件加速**：NVIDIA NVENC、Intel VAAPI
- **自适应码率**：根据网络条件自动调整
- **多线程处理**：充分利用多核 CPU
- **内存优化**：零拷贝传输和内存池

## 🏗️ 架构

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│  Web Browser    │    │   Web Server    │    │  GStreamer      │
│                 │    │                 │    │  Pipeline       │
│  - Vue.js App   │◄──►│  - HTTP Server  │◄──►│                 │
│  - WebRTC       │    │  - WebSocket    │    │  - Desktop      │
│  - Media Player │    │  - Signaling    │    │    Capture      │
│  - Input Handler│    │  - REST API     │    │  - Encoding     │
└─────────────────┘    └─────────────────┘    └─────────────────┘
```

## 🚀 快速开始

### 方式一：一键启动脚本

```bash
# 克隆项目
git clone https://github.com/open-beagle/bdwind-gstreamer.git
cd bdwind-gstreamer

# 运行快速启动脚本
./scripts/quick-start.sh
```

### 方式二：手动编译

```bash
# 安装依赖 (Ubuntu/Debian)
sudo apt update
sudo apt install -y build-essential pkg-config \
    gstreamer1.0-tools gstreamer1.0-plugins-base \
    gstreamer1.0-plugins-good gstreamer1.0-plugins-bad \
    libgstreamer1.0-dev libgstreamer-plugins-base1.0-dev \
    libx11-dev

# 编译项目
go mod download
CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -o .tmp/bdwind-gstreamer ./cmd/bdwind-gstreamer

# 启动服务器
.tmp/bdwind-gstreamer --host 0.0.0.0 -port 8080
```

### 方式三：Docker 部署

#### 快速部署（推荐）

```bash
# 完整构建和运行流程
./scripts/build-docker.sh all

# 访问 Web 界面
http://localhost:8080
```

#### 分步骤部署（学习用）

```bash
# 步骤 1: 构建编译镜像
./scripts/build-docker.sh build-compile

# 步骤 2: 提取编译结果
./scripts/build-docker.sh extract

# 步骤 3: 构建运行时镜像
./scripts/build-docker.sh build-runtime

# 步骤 4: 运行容器
./scripts/build-docker.sh run
```

#### 手动 Docker 命令

```bash
# 构建编译镜像
docker build -f Dockerfile.build -t bdwind-gstreamer:build .

# 提取二进制文件
docker create --name temp-build bdwind-gstreamer:build
docker cp temp-build:/app/bdwind-gstreamer ./bdwind-gstreamer
docker rm temp-build

# 构建运行时镜像
docker build -f Dockerfile.runtime -t bdwind-gstreamer:runtime .

# 运行容器（使用 xvfb 虚拟显示）
docker run -d \
  --name bdwind-gstreamer \
  -p 8080:8080 \
  -e DISPLAY=:99 \
  -e DISPLAY_NUM=99 \
  -e DISPLAY_RESOLUTION=1920x1080x24 \
  -e BDWIND_AUTH_ENABLED=true \
  -e BDWIND_MONITORING_ENABLED=false \
  --restart unless-stopped \
  bdwind-gstreamer:runtime
```

#### 容器功能测试

```bash
# 运行容器测试
./scripts/test-docker-container.sh

# 查看容器日志
./scripts/build-docker.sh logs

# 进入容器调试
./scripts/build-docker.sh shell
```

**特性说明：**

- ✅ 使用 Xvfb 虚拟显示，无需宿主机 X11 环境
- ✅ 分离式构建：编译镜像和运行时镜像独立
- ✅ 支持外部 TURN 服务器配置
- ✅ 禁用内置监控，专注核心功能
- ✅ 完整的测试和调试工具

## 🌐 访问界面

启动服务器后，在浏览器中访问：

```
http://localhost:8080
```

## 📖 文档

- [📋 部署指南](DEPLOYMENT.md) - 完整的生产环境部署指南
- [🐳 Docker 纯命令部署](docs/DOCKER_PURE_COMMANDS.md) - 使用纯 Docker 命令的容器化部署
- [� 容问器化部署指南](docs/CONTAINER_DEPLOYMENT.md) - Docker 容器化部署详细说明
- [🏗️ 架构设计](docs/web-management-interface.md) - Web 管理界面设计文档
- [🔐 访问控制](internal/webserver/auth/README.md) - 认证和授权系统说明
- [📊 统计监控](docs/desktop-capture-statistics.md) - 性能监控和统计
- [🖥️ X11 捕获](docs/x11-desktop-capture.md) - X11 桌面捕获实现
- [🌊 Wayland 捕获](docs/wayland-desktop-capture.md) - Wayland 桌面捕获实现

## 🛠️ 开发

### 项目结构

```
bdwind-gstreamer/
├── cmd/                    # 可执行程序
│   ├── bdwind-gstreamer/   # 主服务器程序
│   ├── web-server/         # Web 服务器演示
│   ├── auth-demo/          # 认证系统演示
│   └── *-demo/            # 其他演示程序
├── internal/               # 内部包
│   ├── auth/              # 认证和授权
│   ├── config/            # 配置管理
│   ├── gstreamer/         # GStreamer 集成
│   ├── monitoring/        # 监控和指标
│   ├── signaling/         # WebSocket 信令
│   └── webrtc/           # WebRTC 集成
├── docs/                  # 文档
├── scripts/               # 脚本工具
├── web/                   # 前端资源
└── tests/                 # 测试文件
```

### 开发环境设置

```bash
# 安装开发依赖
go install golang.org/x/tools/cmd/goimports@latest
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# 运行测试
go test ./...

# 代码格式化
goimports -w .
golangci-lint run
```

### 添加新功能

1. 在 `internal/` 目录下创建新模块
2. 实现相应的接口和测试
3. 在 `cmd/bdwind-gstreamer/` 中集成新功能
4. 更新文档和配置

## 🧪 演示程序

项目包含多个演示程序，展示不同功能模块：

```bash
# 认证系统演示
go run cmd/auth-demo/main.go

# 系统监控演示
go run cmd/system-metrics-demo/main.go

# X11 桌面捕获演示
go run cmd/x11-capture-demo/main.go

# Wayland 桌面捕获演示
go run cmd/wayland-capture-demo/main.go

# Web 服务器演示
go run cmd/web-server/main.go
```

## 📊 API 接口

### REST API

```bash
# 系统状态
GET /api/status

# 桌面捕获控制
POST /api/capture/start
POST /api/capture/stop
GET  /api/capture/config

# 用户认证
POST /api/auth/login
POST /api/auth/logout
GET  /api/auth/status

# 监控指标
GET /metrics
```

### WebSocket 信令

```javascript
// 连接 WebSocket
const ws = new WebSocket("ws://localhost:8080/api/signaling");

// 发送消息
ws.send(
  JSON.stringify({
    type: "offer",
    data: { sdp: "..." },
  })
);
```

## 🔧 配置

### 配置文件示例

```yaml
# config.yaml
web:
  port: 8080
  host: "0.0.0.0"
  enable_tls: false

capture:
  display_id: ":0"
  frame_rate: 30
  width: 1920
  height: 1080
  quality: "medium"

auth:
  enabled: true
  default_role: "viewer"

monitoring:
  enabled: true
  metrics_port: 9090
```

### 环境变量

#### 本地开发环境

```bash
export BDWIND_WEB_PORT=8080
export BDWIND_CAPTURE_DISPLAY=":0"
export BDWIND_AUTH_ENABLED=true
export DISPLAY=:0
```

#### 容器化环境

```bash
export BDWIND_WEB_PORT=8080
export BDWIND_CAPTURE_DISPLAY=":99"  # 虚拟显示
export BDWIND_AUTH_ENABLED=true
export BDWIND_MONITORING_ENABLED=false  # 容器化时禁用
export DISPLAY=:99  # Xvfb 虚拟显示
export DISPLAY_NUM=99
export DISPLAY_RESOLUTION=1920x1080x24
export TURN_SERVER_HOST=your-coturn-server.com  # 外部 TURN 服务器
export TURN_SERVER_PORT=3478
```

## 🐳 Docker 支持

### 分离式构建架构

项目采用分离式 Docker 构建架构，将编译和运行环境完全分离：

#### 编译镜像 (Dockerfile.build)

```dockerfile
FROM docker.io/golang:1.21-bullseye

# 安装编译依赖
RUN apt-get update && apt-get install -y \
    pkg-config \
    libgstreamer1.0-dev \
    libgstreamer-plugins-base1.0-dev \
    libx11-dev libxext-dev libxfixes-dev \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo \
    -ldflags '-w -s' -o bdwind-gstreamer ./cmd/bdwind-gstreamer
```

#### 运行时镜像 (Dockerfile.runtime)

```dockerfile
FROM docker.io/debian:12

# 启用 non-free 仓库以获得完整的硬件支持
RUN echo "deb http://deb.debian.org/debian bookworm main non-free-firmware" > /etc/apt/sources.list

# 安装运行时依赖
RUN apt-get update && apt-get install -y \
    gstreamer1.0-tools gstreamer1.0-plugins-base \
    gstreamer1.0-plugins-good gstreamer1.0-plugins-bad \
    xvfb x11-utils x11-xserver-utils \
    vainfo intel-media-va-driver-non-free mesa-va-drivers \
    pulseaudio alsa-utils ca-certificates curl tini \
    && rm -rf /var/lib/apt/lists/*

# 创建非 root 用户
RUN groupadd -r bdwind && useradd -r -g bdwind -s /bin/bash bdwind

WORKDIR /app
COPY bdwind-gstreamer .
COPY scripts/docker-entrypoint.sh /usr/local/bin/
RUN chmod +x /usr/local/bin/docker-entrypoint.sh ./bdwind-gstreamer

# 虚拟显示环境
ENV DISPLAY=:99 DISPLAY_NUM=99 DISPLAY_RESOLUTION=1920x1080x24

EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8080/health || exit 1

USER bdwind
ENTRYPOINT ["/usr/bin/tini", "--", "/usr/local/bin/docker-entrypoint.sh"]
CMD ["./bdwind-gstreamer", "-host", "0.0.0.0", "-port", "8080"]
```

### 容器特性

- **🖥️ 虚拟显示**: 使用 Xvfb 创建虚拟显示环境，无需宿主机 X11
- **🔒 安全隔离**: 运行时镜像不包含编译工具和源代码
- **📦 轻量化**: 基于 Debian 12，镜像更小更安全
- **🌐 外部服务**: 支持外部 TURN 服务器配置
- **🚫 简化架构**: 禁用内置监控，专注核心功能
- **⚡ 多选择**: 提供 Debian 12 和 Ubuntu 22.04 两种基础镜像

### 快速使用

```bash
# 完整构建和运行 (默认使用 Debian 12)
./scripts/build-docker.sh all

# 选择特定基础镜像
./scripts/build-docker.sh build-runtime-debian   # Debian 12 (推荐)
./scripts/build-docker.sh build-runtime-ubuntu   # Ubuntu 22.04 (备选)

# 测试容器功能
./scripts/test-docker-container.sh

# 查看详细文档
cat docs/DOCKER_PURE_COMMANDS.md
cat docs/BASE_IMAGE_ANALYSIS.md  # 基础镜像选择分析
```

## 📈 性能

### 基准测试结果

- **延迟**: < 50ms (局域网)
- **帧率**: 60 FPS @ 1080p
- **CPU 使用率**: < 20% (硬件加速)
- **内存使用**: < 500MB
- **并发连接**: 支持 100+ 客户端

### 优化建议

1. **启用硬件加速**: 使用 NVENC 或 VAAPI
2. **调整编码参数**: 根据网络条件优化比特率
3. **使用 SSD**: 提高 I/O 性能
4. **网络优化**: 使用千兆网络和低延迟交换机

## 🔍 故障排除

### 常见问题

1. **服务启动失败**

   ```bash
   # 检查依赖
   ldd bdwind-gstreamer

   # 检查端口
   netstat -tlnp | grep :8080
   ```

2. **桌面捕获失败**

   ```bash
   # 检查显示环境
   echo $DISPLAY
   xdpyinfo

   # 检查权限
   xhost +local:
   ```

3. **WebRTC 连接失败**

   ```bash
   # 检查防火墙
   sudo ufw status

   # 检查 STUN/TURN 服务器
   ```

### 调试模式

```bash
# 启用调试日志
export GST_DEBUG=3
export BDWIND_DEBUG=true

# 运行服务器
./bdwind-gstreamer -config config.yaml
```

## 🤝 贡献

我们欢迎所有形式的贡献！

### 如何贡献

1. Fork 项目
2. 创建功能分支 (`git checkout -b feature/amazing-feature`)
3. 提交更改 (`git commit -m 'Add amazing feature'`)
4. 推送到分支 (`git push origin feature/amazing-feature`)
5. 创建 Pull Request

### 开发规范

- 遵循 Go 代码规范
- 添加适当的测试
- 更新相关文档
- 确保 CI 通过

## 📄 许可证

本项目采用 MIT 许可证 - 查看 [LICENSE](LICENSE) 文件了解详情。

## 🙏 致谢

- [GStreamer](https://gstreamer.freedesktop.org/) - 多媒体框架
- [Pion WebRTC](https://github.com/pion/webrtc) - Go WebRTC 实现
- [selkies-gstreamer](https://github.com/selkies-project/selkies-gstreamer) - 项目灵感来源
- [Vue.js](https://vuejs.org/) - 前端框架

## 📞 支持

- **问题报告**: [GitHub Issues](https://github.com/open-beagle/bdwind-gstreamer/issues)
- **功能请求**: [GitHub Discussions](https://github.com/open-beagle/bdwind-gstreamer/discussions)
- **文档**: [项目 Wiki](https://github.com/open-beagle/bdwind-gstreamer/wiki)

---

<div align="center">

**⭐ 如果这个项目对您有帮助，请给我们一个 Star！**

Made with ❤️ by the BDWind-GStreamer Team

</div>
