# GStreamer Go-GST 重构部署指南

## 概述

本文档提供 GStreamer Go-GST 重构版本的完整部署指南，包括环境准备、构建配置、部署步骤和运维监控。

## 环境要求

### 系统要求

**操作系统支持**:
- Ubuntu 20.04 LTS 或更高版本
- Debian 11 (Bullseye) 或更高版本
- CentOS 8 或更高版本（需要额外配置）

**硬件要求**:
- CPU: 2 核心或更多
- 内存: 4GB 或更多
- 存储: 10GB 可用空间
- 网络: 稳定的网络连接

**可选硬件加速**:
- NVIDIA GPU (支持 NVENC)
- Intel GPU (支持 VAAPI)
- AMD GPU (支持 VAAPI)

### 软件依赖

#### 编译时依赖

```bash
# Ubuntu/Debian
sudo apt-get update
sudo apt-get install -y \
    build-essential \
    pkg-config \
    libgstreamer1.0-dev \
    libgstreamer-plugins-base1.0-dev \
    libgstreamer-plugins-good1.0-dev \
    libgstreamer-plugins-bad1.0-dev \
    libglib2.0-dev \
    libgobject-introspection-dev \
    libx11-dev \
    libxext-dev \
    libxfixes-dev \
    libxdamage-dev \
    libxcomposite-dev \
    libxrandr-dev \
    libxtst-dev
```

#### 运行时依赖

```bash
# Ubuntu/Debian
sudo apt-get install -y \
    gstreamer1.0-tools \
    gstreamer1.0-plugins-base \
    gstreamer1.0-plugins-good \
    gstreamer1.0-plugins-bad \
    gstreamer1.0-plugins-ugly \
    gstreamer1.0-libav \
    libgstreamer1.0-0 \
    libgstreamer-plugins-base1.0-0 \
    libglib2.0-0 \
    libgobject-2.0-0 \
    xvfb \
    x11-utils \
    x11-xserver-utils
```

#### 硬件加速支持

```bash
# NVIDIA 硬件编码支持
sudo apt-get install -y \
    nvidia-driver-470 \
    libnvidia-encode-470

# Intel VAAPI 支持
sudo apt-get install -y \
    intel-media-va-driver-non-free \
    mesa-va-drivers \
    vainfo

# AMD VAAPI 支持
sudo apt-get install -y \
    mesa-va-drivers \
    vainfo
```

## 构建配置

### Go 环境配置

```bash
# 安装 Go 1.24 或更高版本
wget https://go.dev/dl/go1.24.4.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.24.4.linux-amd64.tar.gz

# 设置环境变量
export PATH=$PATH:/usr/local/go/bin
export CGO_ENABLED=1
export PKG_CONFIG_PATH=/usr/lib/pkgconfig
```

### 项目构建

```bash
# 克隆项目
git clone https://github.com/open-beagle/bdwind-gstreamer.git
cd bdwind-gstreamer

# 下载依赖
go mod download

# 验证 go-gst 依赖
go list -m github.com/go-gst/go-gst

# 构建应用程序
CGO_ENABLED=1 go build -o bdwind-gstreamer ./cmd/bdwind-gstreamer

# 验证构建结果
./bdwind-gstreamer --version
```

### Docker 构建

#### 多阶段构建

```bash
# 构建 Docker 镜像
docker build -f Dockerfile.build -t bdwind-gstreamer:build .
docker build -f Dockerfile.runtime -t bdwind-gstreamer:latest .

# 验证镜像
docker run --rm bdwind-gstreamer:latest --version
```

#### 构建脚本

```bash
#!/bin/bash
# scripts/build-docker-go-gst.sh

set -e

echo "Building BDWind-GStreamer with go-gst support..."

# 构建编译镜像
echo "Building compile image..."
docker build -f Dockerfile.build -t bdwind-gstreamer:build .

# 从编译镜像中提取二进制文件
echo "Extracting binary..."
docker create --name temp-container bdwind-gstreamer:build
docker cp temp-container:/app/bdwind-gstreamer ./bdwind-gstreamer
docker rm temp-container

# 构建运行时镜像
echo "Building runtime image..."
docker build -f Dockerfile.runtime -t bdwind-gstreamer:latest .

# 清理临时文件
rm -f ./bdwind-gstreamer

echo "Build completed successfully!"
```

## 部署配置

### 配置文件准备

#### 基础配置 (config.yaml)

```yaml
# GStreamer go-gst 配置
gstreamer:
  capture:
    display_id: ":0"
    width: 1920
    height: 1080
    frame_rate: 30
    use_wayland: false
    show_pointer: true
    use_damage: true
    quality: "high"
    buffer_size: 10

  encoding:
    type: "video"
    codec: "h264"
    bitrate: 2000000
    min_bitrate: 500000
    max_bitrate: 5000000
    use_hardware: true
    preset: "ultrafast"
    profile: "baseline"
    rate_control: "cbr"
    zero_latency: true

# WebRTC 配置
webrtc:
  ice_servers:
    - urls: ["stun:stun.l.google.com:19302"]
  
# 日志配置
logging:
  level: "info"
  output: "stdout"
  colored: true
  format: "json"
  categories:
    gstreamer: "debug"
    webrtc: "info"
    http: "info"

# Web 服务器配置
webserver:
  host: "0.0.0.0"
  port: 8080
  static_dir: "./internal/webserver/static"
  
# 监控配置
metrics:
  enabled: true
  port: 9090
  path: "/metrics"
```

#### 硬件优化配置

```yaml
# 针对 NVIDIA GPU 优化
gstreamer:
  encoding:
    use_hardware: true
    codec: "h264"
    preset: "low-latency-hq"
    rate_control: "cbr"
    # NVIDIA 特定参数
    nvenc_preset: "low-latency-hq"
    nvenc_rc_mode: "cbr"
    nvenc_multipass: "quarter-resolution"

# 针对 Intel VAAPI 优化
gstreamer:
  encoding:
    use_hardware: true
    codec: "h264"
    # VAAPI 特定参数
    vaapi_rate_control: "cbr"
    vaapi_quality_level: 4
    vaapi_b_frames: 0
```

### 环境变量配置

```bash
# 创建环境变量文件
cat > .env << EOF
# GStreamer 配置
GST_DEBUG=2
GST_DEBUG_NO_COLOR=1
GST_REGISTRY_FORK=no
GST_REGISTRY_UPDATE=no

# go-gst 配置
CGO_ENABLED=1
PKG_CONFIG_PATH=/usr/lib/pkgconfig

# 显示配置
DISPLAY=:99
DISPLAY_NUM=99
DISPLAY_RESOLUTION=1920x1080x24
XDG_RUNTIME_DIR=/tmp/runtime-bdwind

# 应用配置
BDWIND_CONFIG_FILE=/app/config.yaml
BDWIND_LOG_LEVEL=info
BDWIND_HTTP_PORT=8080
BDWIND_METRICS_PORT=9090

# 性能优化
GOMAXPROCS=4
GOMEMLIMIT=2GiB
EOF
```

### Docker Compose 部署

```yaml
# docker-compose.go-gst.yml
version: '3.8'

services:
  bdwind-gstreamer:
    image: bdwind-gstreamer:latest
    container_name: bdwind-gstreamer
    restart: unless-stopped
    
    # 网络配置
    ports:
      - "8080:8080"
      - "9090:9090"
    
    # 环境变量
    environment:
      - DISPLAY=:99
      - GST_DEBUG=2
      - CGO_ENABLED=1
    
    # 卷挂载
    volumes:
      - ./config.yaml:/app/config.yaml:ro
      - ./logs:/app/logs
      - /tmp/.X11-unix:/tmp/.X11-unix:rw
    
    # 设备访问
    devices:
      - /dev/dri:/dev/dri  # GPU 访问
    
    # 特权和能力
    privileged: false
    cap_add:
      - SYS_ADMIN
    
    # 健康检查
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8080/health"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 40s
    
    # 资源限制
    deploy:
      resources:
        limits:
          memory: 2G
          cpus: '2.0'
        reservations:
          memory: 1G
          cpus: '1.0'

  # 虚拟显示服务
  xvfb:
    image: jlesage/xvfb:latest
    container_name: bdwind-xvfb
    restart: unless-stopped
    environment:
      - DISPLAY_WIDTH=1920
      - DISPLAY_HEIGHT=1080
    volumes:
      - /tmp/.X11-unix:/tmp/.X11-unix:rw
```

### Kubernetes 部署

```yaml
# k8s-deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: bdwind-gstreamer
  labels:
    app: bdwind-gstreamer
spec:
  replicas: 1
  selector:
    matchLabels:
      app: bdwind-gstreamer
  template:
    metadata:
      labels:
        app: bdwind-gstreamer
    spec:
      containers:
      - name: bdwind-gstreamer
        image: bdwind-gstreamer:latest
        ports:
        - containerPort: 8080
        - containerPort: 9090
        
        env:
        - name: DISPLAY
          value: ":99"
        - name: GST_DEBUG
          value: "2"
        - name: CGO_ENABLED
          value: "1"
        
        volumeMounts:
        - name: config
          mountPath: /app/config.yaml
          subPath: config.yaml
        - name: logs
          mountPath: /app/logs
        
        resources:
          limits:
            memory: "2Gi"
            cpu: "2000m"
          requests:
            memory: "1Gi"
            cpu: "1000m"
        
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 30
          periodSeconds: 30
        
        readinessProbe:
          httpGet:
            path: /ready
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 10
      
      volumes:
      - name: config
        configMap:
          name: bdwind-config
      - name: logs
        emptyDir: {}

---
apiVersion: v1
kind: Service
metadata:
  name: bdwind-gstreamer-service
spec:
  selector:
    app: bdwind-gstreamer
  ports:
  - name: http
    port: 8080
    targetPort: 8080
  - name: metrics
    port: 9090
    targetPort: 9090
  type: LoadBalancer

---
apiVersion: v1
kind: ConfigMap
metadata:
  name: bdwind-config
data:
  config.yaml: |
    gstreamer:
      capture:
        display_id: ":0"
        width: 1920
        height: 1080
        frame_rate: 30
        use_hardware: true
    # ... 其他配置
```

## 运维监控

### 健康检查

```bash
#!/bin/bash
# scripts/health-check.sh

# 检查应用程序状态
check_app_health() {
    local response=$(curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/health)
    if [ "$response" = "200" ]; then
        echo "✅ Application health check passed"
        return 0
    else
        echo "❌ Application health check failed (HTTP $response)"
        return 1
    fi
}

# 检查 GStreamer 功能
check_gstreamer() {
    if gst-inspect-1.0 ximagesrc > /dev/null 2>&1; then
        echo "✅ GStreamer ximagesrc available"
    else
        echo "❌ GStreamer ximagesrc not available"
        return 1
    fi
    
    if gst-inspect-1.0 x264enc > /dev/null 2>&1; then
        echo "✅ GStreamer x264enc available"
    else
        echo "❌ GStreamer x264enc not available"
        return 1
    fi
}

# 检查硬件编码器
check_hardware_encoders() {
    if gst-inspect-1.0 nvh264enc > /dev/null 2>&1; then
        echo "✅ NVIDIA hardware encoder available"
    else
        echo "ℹ️  NVIDIA hardware encoder not available"
    fi
    
    if vainfo > /dev/null 2>&1; then
        echo "✅ VAAPI hardware acceleration available"
    else
        echo "ℹ️  VAAPI hardware acceleration not available"
    fi
}

# 执行所有检查
main() {
    echo "🔍 Running health checks..."
    
    check_app_health || exit 1
    check_gstreamer || exit 1
    check_hardware_encoders
    
    echo "✅ All health checks completed successfully"
}

main "$@"
```

### 监控指标

#### Prometheus 配置

```yaml
# prometheus.yml
global:
  scrape_interval: 15s

scrape_configs:
  - job_name: 'bdwind-gstreamer'
    static_configs:
      - targets: ['localhost:9090']
    metrics_path: /metrics
    scrape_interval: 10s
```

#### Grafana 仪表板

```json
{
  "dashboard": {
    "title": "BDWind GStreamer Go-GST Monitoring",
    "panels": [
      {
        "title": "GStreamer Pipeline Status",
        "type": "stat",
        "targets": [
          {
            "expr": "gstreamer_pipeline_state",
            "legendFormat": "Pipeline State"
          }
        ]
      },
      {
        "title": "Video Capture FPS",
        "type": "graph",
        "targets": [
          {
            "expr": "rate(gstreamer_capture_frames_total[1m])",
            "legendFormat": "Capture FPS"
          }
        ]
      },
      {
        "title": "Encoding Performance",
        "type": "graph",
        "targets": [
          {
            "expr": "rate(gstreamer_encoder_frames_total[1m])",
            "legendFormat": "Encoder FPS"
          },
          {
            "expr": "gstreamer_encoder_bitrate_bps",
            "legendFormat": "Bitrate (bps)"
          }
        ]
      },
      {
        "title": "Memory Usage",
        "type": "graph",
        "targets": [
          {
            "expr": "process_resident_memory_bytes",
            "legendFormat": "RSS Memory"
          }
        ]
      }
    ]
  }
}
```

### 日志管理

#### 结构化日志配置

```yaml
# 日志配置
logging:
  level: "info"
  output: "file"
  file: "/app/logs/bdwind-gstreamer.log"
  format: "json"
  rotation:
    max_size: "100MB"
    max_files: 10
    max_age: "7d"
  categories:
    gstreamer: "debug"
    webrtc: "info"
    http: "warn"
```

#### 日志收集 (ELK Stack)

```yaml
# filebeat.yml
filebeat.inputs:
- type: log
  enabled: true
  paths:
    - /app/logs/bdwind-gstreamer.log
  json.keys_under_root: true
  json.add_error_key: true

output.elasticsearch:
  hosts: ["elasticsearch:9200"]
  index: "bdwind-gstreamer-%{+yyyy.MM.dd}"

setup.template.name: "bdwind-gstreamer"
setup.template.pattern: "bdwind-gstreamer-*"
```

### 性能调优

#### 系统级优化

```bash
# 系统参数优化
echo 'net.core.rmem_max = 134217728' >> /etc/sysctl.conf
echo 'net.core.wmem_max = 134217728' >> /etc/sysctl.conf
echo 'vm.swappiness = 10' >> /etc/sysctl.conf
sysctl -p

# CPU 调度优化
echo 'performance' > /sys/devices/system/cpu/cpu*/cpufreq/scaling_governor

# 内存优化
echo 'never' > /sys/kernel/mm/transparent_hugepage/enabled
```

#### GStreamer 优化

```bash
# GStreamer 环境变量优化
export GST_REGISTRY_FORK=no
export GST_REGISTRY_UPDATE=no
export GST_DEBUG_NO_COLOR=1
export GST_GL_XINITTHREADS=1

# 性能跟踪
export GST_TRACERS=stats,rusage
export GST_DEBUG=GST_TRACER:7
```

#### Go 应用优化

```bash
# Go 运行时优化
export GOMAXPROCS=4
export GOMEMLIMIT=2GiB
export GOGC=100
export GODEBUG=gctrace=1
```

## 故障排除

### 常见问题解决

#### 问题 1: go-gst 初始化失败

```bash
# 检查 GStreamer 安装
gst-inspect-1.0 --version

# 检查 go-gst 编译
go list -m github.com/go-gst/go-gst

# 重新安装依赖
sudo apt-get install --reinstall libgstreamer1.0-dev
```

#### 问题 2: 硬件编码器不工作

```bash
# 检查 NVIDIA 驱动
nvidia-smi

# 检查 VAAPI 支持
vainfo

# 测试硬件编码
gst-launch-1.0 videotestsrc ! nvh264enc ! fakesink
```

#### 问题 3: 内存泄漏

```bash
# 启用内存跟踪
export GST_TRACERS=leaks
export GST_DEBUG=GST_TRACER:7

# 使用 valgrind
valgrind --tool=memcheck --leak-check=full ./bdwind-gstreamer
```

### 紧急恢复程序

```bash
#!/bin/bash
# scripts/emergency-recovery.sh

echo "🚨 Starting emergency recovery procedure..."

# 停止服务
systemctl stop bdwind-gstreamer

# 检查系统资源
echo "📊 System resources:"
free -h
df -h
ps aux | grep bdwind

# 清理临时文件
rm -rf /tmp/gst-*
rm -rf /tmp/.X11-unix/*

# 重启相关服务
systemctl restart xvfb
systemctl start bdwind-gstreamer

# 验证恢复
sleep 10
curl -f http://localhost:8080/health && echo "✅ Recovery successful" || echo "❌ Recovery failed"
```

这个部署指南提供了完整的 go-gst 重构版本部署流程，确保系统能够稳定运行并提供良好的监控和维护支持。