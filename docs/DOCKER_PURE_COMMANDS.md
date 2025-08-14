# BDWind-GStreamer 纯 Docker 命令部署指南

本指南展示如何使用纯 Docker 命令（不使用 Docker Compose）来构建和运行 BDWind-GStreamer 容器。

## 架构设计

### 分离式构建架构

为了便于学习和理解，我们将 Docker 构建过程分为两个独立的阶段：

1. **编译镜像** (`Dockerfile.build`) - 专门用于编译 Go 应用程序
2. **运行时镜像** (`Dockerfile.runtime`) - 专门用于运行应用程序的轻量级镜像

这种分离式设计的优势：
- **学习友好**: 清晰地展示编译和运行的不同需求
- **安全性**: 运行时镜像不包含编译工具和源代码
- **体积优化**: 运行时镜像更小，部署更快
- **维护性**: 可以独立更新编译环境和运行环境

### 虚拟显示环境

使用 Xvfb (X Virtual Framebuffer) 创建虚拟显示环境：
- **显示号**: `:99`
- **分辨率**: `1920x1080x24`
- **隔离性**: 不依赖宿主机 X11 环境
- **安全性**: 避免暴露宿主机显示系统

### 外部服务依赖

- **TURN 服务器**: 使用外部 coturn 服务器进行 WebRTC NAT 穿透
- **监控禁用**: 不包含 Prometheus 和 Grafana，专注核心功能

## 快速开始

### 1. 完整构建和运行

```bash
# 执行完整的构建和运行流程
./scripts/build-docker.sh all
```

这个命令会依次执行：
1. 构建编译镜像
2. 从编译镜像中提取二进制文件
3. 构建运行时镜像
4. 启动容器
5. 显示状态信息

### 2. 分步骤构建（推荐用于学习）

```bash
# 步骤 1: 构建编译镜像
./scripts/build-docker.sh build-compile

# 步骤 2: 从编译镜像中提取二进制文件
./scripts/build-docker.sh extract

# 步骤 3: 构建运行时镜像
./scripts/build-docker.sh build-runtime

# 步骤 4: 运行容器
./scripts/build-docker.sh run
```

### 3. 测试容器功能

```bash
# 运行容器功能测试
./scripts/test-docker-container.sh
```

## 详细命令说明

### 构建命令

#### 构建编译镜像

```bash
# 使用 Dockerfile.build 构建编译镜像
docker build -f Dockerfile.build -t bdwind-gstreamer:build .
```

编译镜像包含：
- Go 1.21 编译环境
- GStreamer 开发库
- X11 开发库
- 项目源代码

#### 提取编译结果

```bash
# 创建临时容器
docker create --name bdwind-build-temp bdwind-gstreamer:build

# 提取二进制文件
docker cp bdwind-build-temp:/app/bdwind-gstreamer ./bdwind-gstreamer

# 清理临时容器
docker rm bdwind-build-temp
```

#### 构建运行时镜像

```bash
# 使用 Dockerfile.runtime 构建运行时镜像
docker build -f Dockerfile.runtime -t bdwind-gstreamer:runtime .
```

运行时镜像包含：
- Ubuntu 22.04 基础系统
- GStreamer 运行时库
- Xvfb 虚拟显示
- 应用程序二进制文件

### 运行命令

#### 基本运行

```bash
docker run -d \
  --name bdwind-gstreamer \
  -p 8080:8080 \
  -e DISPLAY=:99 \
  -e DISPLAY_NUM=99 \
  -e DISPLAY_RESOLUTION=1920x1080x24 \
  bdwind-gstreamer:runtime
```

#### 完整配置运行

```bash
docker run -d \
  --name bdwind-gstreamer \
  -p 8080:8080 \
  -e DISPLAY=:99 \
  -e DISPLAY_NUM=99 \
  -e DISPLAY_RESOLUTION=1920x1080x24 \
  -e BDWIND_WEB_PORT=8080 \
  -e BDWIND_WEB_HOST=0.0.0.0 \
  -e BDWIND_CAPTURE_DISPLAY=:99 \
  -e BDWIND_AUTH_ENABLED=true \
  -e BDWIND_MONITORING_ENABLED=false \
  -e GST_DEBUG=2 \
  -e TURN_SERVER_HOST=your-coturn-server.com \
  -e TURN_SERVER_PORT=3478 \
  -e TURN_USERNAME=your-username \
  -e TURN_PASSWORD=your-password \
  --restart unless-stopped \
  bdwind-gstreamer:runtime
```

### 管理命令

#### 查看容器状态

```bash
# 查看运行中的容器
docker ps

# 查看容器详细信息
docker inspect bdwind-gstreamer

# 查看容器日志
docker logs -f bdwind-gstreamer
```

#### 进入容器

```bash
# 进入容器 shell
docker exec -it bdwind-gstreamer /bin/bash

# 测试虚拟显示
docker exec -it bdwind-gstreamer xdpyinfo -display :99

# 查看进程
docker exec -it bdwind-gstreamer ps aux
```

#### 停止和清理

```bash
# 停止容器
docker stop bdwind-gstreamer

# 删除容器
docker rm bdwind-gstreamer

# 删除镜像
docker rmi bdwind-gstreamer:build bdwind-gstreamer:runtime
```

## 环境变量配置

### 核心配置

| 变量名 | 默认值 | 说明 |
|--------|--------|------|
| `DISPLAY` | `:99` | 虚拟显示号 |
| `DISPLAY_NUM` | `99` | Xvfb 显示编号 |
| `DISPLAY_RESOLUTION` | `1920x1080x24` | 虚拟显示分辨率 |
| `BDWIND_WEB_PORT` | `8080` | Web 服务端口 |
| `BDWIND_WEB_HOST` | `0.0.0.0` | Web 服务监听地址 |

### 功能开关

| 变量名 | 默认值 | 说明 |
|--------|--------|------|
| `BDWIND_AUTH_ENABLED` | `true` | 启用认证 |
| `BDWIND_MONITORING_ENABLED` | `false` | 禁用监控 |
| `BDWIND_DEBUG` | `false` | 调试模式 |

### 外部服务配置

| 变量名 | 默认值 | 说明 |
|--------|--------|------|
| `TURN_SERVER_HOST` | - | TURN 服务器地址 |
| `TURN_SERVER_PORT` | `3478` | TURN 服务器端口 |
| `TURN_USERNAME` | - | TURN 用户名 |
| `TURN_PASSWORD` | - | TURN 密码 |

### GStreamer 配置

| 变量名 | 默认值 | 说明 |
|--------|--------|------|
| `GST_DEBUG` | `2` | GStreamer 调试级别 |
| `GST_PLUGIN_PATH` | - | GStreamer 插件路径 |

## 外部 TURN 服务器配置

### coturn 服务器部署

```bash
# 使用 Docker 运行 coturn 服务器
docker run -d \
  --name coturn-server \
  -p 3478:3478/udp \
  -p 3478:3478/tcp \
  -p 49152-65535:49152-65535/udp \
  -e TURN_USERNAME=bdwind \
  -e TURN_PASSWORD=bdwind123 \
  coturn/coturn:latest \
  turnserver \
  -n \
  --log-file=stdout \
  --min-port=49152 \
  --max-port=65535 \
  --lt-cred-mech \
  --fingerprint \
  --no-multicast-peers \
  --no-cli \
  --no-tlsv1 \
  --no-tlsv1_1 \
  --realm=bdwind.local \
  --user=bdwind:bdwind123
```

### 配置 BDWind-GStreamer 使用外部 TURN

```bash
docker run -d \
  --name bdwind-gstreamer \
  -p 8080:8080 \
  -e TURN_SERVER_HOST=your-coturn-server.com \
  -e TURN_SERVER_PORT=3478 \
  -e TURN_USERNAME=bdwind \
  -e TURN_PASSWORD=bdwind123 \
  bdwind-gstreamer:runtime
```

## 测试和验证

### 功能测试

```bash
# 运行完整测试套件
./scripts/test-docker-container.sh

# 测试特定功能
curl http://localhost:8080/health
curl http://localhost:8080/api/status
curl http://localhost:8080/api/config
```

### 虚拟显示测试

```bash
# 检查 Xvfb 进程
docker exec bdwind-gstreamer pgrep Xvfb

# 测试显示可访问性
docker exec bdwind-gstreamer xdpyinfo -display :99

# 测试 GStreamer 捕获
docker exec bdwind-gstreamer timeout 5s gst-launch-1.0 ximagesrc display-name=:99 ! fakesink
```

### 网络连接测试

```bash
# 测试 Web 界面
curl -I http://localhost:8080/

# 测试 WebSocket 连接
wscat -c ws://localhost:8080/ws

# 测试 API 端点
curl http://localhost:8080/api/status | jq
```

## 故障排除

### 常见问题

1. **容器启动失败**
   ```bash
   # 查看详细日志
   docker logs bdwind-gstreamer
   
   # 检查容器状态
   docker inspect bdwind-gstreamer
   ```

2. **虚拟显示问题**
   ```bash
   # 检查 Xvfb 进程
   docker exec bdwind-gstreamer ps aux | grep Xvfb
   
   # 手动启动 Xvfb
   docker exec bdwind-gstreamer Xvfb :99 -screen 0 1920x1080x24 -ac &
   ```

3. **网络连接问题**
   ```bash
   # 检查端口绑定
   docker port bdwind-gstreamer
   
   # 检查防火墙设置
   sudo ufw status
   ```

### 调试模式

```bash
# 启用调试模式
docker run -d \
  --name bdwind-gstreamer-debug \
  -p 8080:8080 \
  -e BDWIND_DEBUG=true \
  -e GST_DEBUG=3 \
  bdwind-gstreamer:runtime

# 查看调试日志
docker logs -f bdwind-gstreamer-debug
```

### 交互式调试

```bash
# 启动交互式容器
docker run -it --rm \
  -p 8080:8080 \
  -e DISPLAY=:99 \
  bdwind-gstreamer:runtime /bin/bash

# 在容器内手动启动服务
/usr/local/bin/docker-entrypoint.sh ./bdwind-gstreamer
```

## 生产部署建议

### 安全配置

```bash
docker run -d \
  --name bdwind-gstreamer \
  -p 8080:8080 \
  --read-only \
  --tmpfs /tmp \
  --tmpfs /var/run \
  --tmpfs /var/log \
  -e BDWIND_AUTH_ENABLED=true \
  -e BDWIND_DEBUG=false \
  --restart unless-stopped \
  --memory=1g \
  --cpus=2 \
  bdwind-gstreamer:runtime
```

### 资源限制

```bash
docker run -d \
  --name bdwind-gstreamer \
  -p 8080:8080 \
  --memory=2g \
  --memory-swap=2g \
  --cpus=2 \
  --pids-limit=100 \
  bdwind-gstreamer:runtime
```

### 日志管理

```bash
docker run -d \
  --name bdwind-gstreamer \
  -p 8080:8080 \
  --log-driver=json-file \
  --log-opt max-size=10m \
  --log-opt max-file=3 \
  bdwind-gstreamer:runtime
```

## 脚本工具

项目提供了以下脚本工具：

- `scripts/build-docker.sh` - Docker 构建和管理脚本
- `scripts/test-docker-container.sh` - 容器功能测试脚本
- `scripts/docker-entrypoint.sh` - 容器入口点脚本
- `scripts/test-xvfb.sh` - Xvfb 功能测试脚本

使用 `--help` 参数查看各脚本的详细用法。

## 总结

通过使用纯 Docker 命令和分离式构建架构，您可以：

1. **清晰理解**构建和运行的不同阶段
2. **灵活控制**每个构建步骤
3. **安全部署**不包含编译工具的运行时镜像
4. **高效运行**使用虚拟显示环境的容器化应用

这种方法特别适合学习 Docker 容器化技术和理解应用程序的部署过程。