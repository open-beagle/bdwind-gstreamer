# BDWind-GStreamer 部署指南

本文档详细说明如何在 Linux 系统中部署 BDWind-GStreamer 桌面捕获服务，并通过 Web 客户端进行访问。

## 🎯 部署目标

- **服务端**：在 Linux 系统中运行 BDWind-GStreamer 服务器
- **客户端**：通过 Web 浏览器访问和控制桌面捕获
- **架构**：基于 WebRTC 的实时桌面流媒体传输

## 📋 系统要求

### 硬件要求

- **CPU**: x86_64 架构，建议 4 核心以上
- **内存**: 最低 4GB，建议 8GB 以上
- **GPU**: 可选，支持 NVIDIA (NVENC) 或 Intel/AMD (VAAPI) 硬件加速
- **网络**: 千兆网络，低延迟连接

### 软件要求

- **操作系统**: Linux (Ubuntu 20.04+, CentOS 8+, Debian 11+)
- **显示服务器**: X11 或 Wayland
- **GStreamer**: 1.16+ (包含必要插件)
- **Go**: 1.21+ (编译时需要)

## 🛠️ 安装依赖

### Ubuntu/Debian

```bash
# 更新包管理器
sudo apt update

# 安装基础依赖
sudo apt install -y build-essential pkg-config

# 安装 GStreamer 及插件
sudo apt install -y \
    gstreamer1.0-tools \
    gstreamer1.0-plugins-base \
    gstreamer1.0-plugins-good \
    gstreamer1.0-plugins-bad \
    gstreamer1.0-plugins-ugly \
    gstreamer1.0-libav \
    libgstreamer1.0-dev \
    libgstreamer-plugins-base1.0-dev

# 安装 X11 开发库
sudo apt install -y \
    libx11-dev \
    libxext-dev \
    libxfixes-dev \
    libxdamage-dev

# 安装 Wayland 开发库 (可选)
sudo apt install -y \
    libwayland-dev \
    libwayland-client0

# 安装硬件加速支持 (可选)
# NVIDIA
sudo apt install -y gstreamer1.0-plugins-bad

# Intel VAAPI
sudo apt install -y \
    gstreamer1.0-vaapi \
    libva-dev \
    vainfo

# 安装 Go (如果需要编译)
wget https://go.dev/dl/go1.21.5.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.21.5.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc
```

### CentOS/RHEL

```bash
# 启用 EPEL 仓库
sudo dnf install -y epel-release

# 安装基础依赖
sudo dnf groupinstall -y "Development Tools"
sudo dnf install -y pkg-config

# 安装 GStreamer
sudo dnf install -y \
    gstreamer1 \
    gstreamer1-devel \
    gstreamer1-plugins-base \
    gstreamer1-plugins-good \
    gstreamer1-plugins-bad-free \
    gstreamer1-plugins-ugly-free

# 安装 X11 开发库
sudo dnf install -y \
    libX11-devel \
    libXext-devel \
    libXfixes-devel \
    libXdamage-devel

# 安装 Go
sudo dnf install -y golang
```

## 📦 编译和安装

### 1. 获取源代码

```bash
# 克隆项目
git clone https://github.com/open-beagle/bdwind-gstreamer.git
cd bdwind-gstreamer

# 或者下载发布版本
wget https://github.com/open-beagle/bdwind-gstreamer/releases/latest/download/bdwind-gstreamer-linux-amd64.tar.gz
tar -xzf bdwind-gstreamer-linux-amd64.tar.gz
cd bdwind-gstreamer
```

### 2. 编译项目

```bash
# 下载依赖
go mod download

# 编译主服务器
go build -o bdwind-gstreamer ./cmd/bdwind-gstreamer

# 编译演示程序 (可选)
go build -o auth-demo ./cmd/auth-demo
go build -o web-server ./cmd/web-server
go build -o x11-capture-demo ./cmd/x11-capture-demo
go build -o wayland-capture-demo ./cmd/wayland-capture-demo

# 设置执行权限
chmod +x bdwind-gstreamer
```

### 3. 验证安装

```bash
# 检查 GStreamer 插件
gst-inspect-1.0 ximagesrc
gst-inspect-1.0 waylandsrc  # Wayland 环境

# 检查硬件加速 (可选)
# NVIDIA
gst-inspect-1.0 nvenc
# Intel VAAPI
vainfo

# 测试编译结果
./bdwind-gstreamer --version
```

## 🚀 部署配置

### 1. 创建配置文件

```bash
# 创建配置目录
sudo mkdir -p /etc/bdwind-gstreamer
sudo mkdir -p /var/log/bdwind-gstreamer
sudo mkdir -p /var/lib/bdwind-gstreamer

# 创建配置文件
sudo tee /etc/bdwind-gstreamer/config.yaml << EOF
# BDWind-GStreamer 配置文件

# Web 服务器配置
web:
  port: 8080
  host: "0.0.0.0"
  static_dir: "/opt/bdwind-gstreamer/web"
  enable_tls: false
  tls_cert_file: "/etc/ssl/certs/bdwind.crt"
  tls_key_file: "/etc/ssl/private/bdwind.key"

# 桌面捕获配置
capture:
  display_id: ":0"
  frame_rate: 30
  width: 1920
  height: 1080
  use_wayland: false
  auto_detect: true
  show_pointer: true
  use_damage: true
  buffer_size: 10
  quality: "medium"

# 认证配置
auth:
  enabled: true
  default_role: "viewer"
  admin_users:
    - "admin"

# 监控配置
monitoring:
  enabled: true
  metrics_port: 9090
EOF
```

### 2. 安装服务文件

```bash
# 复制程序文件
sudo mkdir -p /opt/bdwind-gstreamer
sudo cp bdwind-gstreamer /opt/bdwind-gstreamer/
sudo cp -r web /opt/bdwind-gstreamer/ 2>/dev/null || true

# 创建用户
sudo useradd -r -s /bin/false -d /var/lib/bdwind-gstreamer bdwind

# 设置权限
sudo chown -R bdwind:bdwind /var/lib/bdwind-gstreamer
sudo chown -R bdwind:bdwind /var/log/bdwind-gstreamer
sudo chmod +x /opt/bdwind-gstreamer/bdwind-gstreamer

# 创建 systemd 服务文件
sudo tee /etc/systemd/system/bdwind-gstreamer.service << EOF
[Unit]
Description=BDWind-GStreamer Desktop Capture Server
Documentation=https://github.com/open-beagle/bdwind-gstreamer
After=network.target graphical-session.target
Wants=network.target

[Service]
Type=simple
User=bdwind
Group=bdwind
WorkingDirectory=/opt/bdwind-gstreamer
ExecStart=/opt/bdwind-gstreamer/bdwind-gstreamer -config /etc/bdwind-gstreamer/config.yaml
ExecReload=/bin/kill -HUP \$MAINPID
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=bdwind-gstreamer

# 环境变量
Environment=DISPLAY=:0
Environment=XDG_RUNTIME_DIR=/run/user/1000
Environment=WAYLAND_DISPLAY=wayland-0

# 安全设置
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/bdwind-gstreamer /var/log/bdwind-gstreamer
PrivateTmp=true

# 资源限制
LimitNOFILE=65536
LimitNPROC=4096

[Install]
WantedBy=multi-user.target
EOF

# 重新加载 systemd
sudo systemctl daemon-reload
```

### 3. 配置防火墙

```bash
# Ubuntu/Debian (ufw)
sudo ufw allow 8080/tcp
sudo ufw allow 9090/tcp  # 监控端口

# CentOS/RHEL (firewalld)
sudo firewall-cmd --permanent --add-port=8080/tcp
sudo firewall-cmd --permanent --add-port=9090/tcp
sudo firewall-cmd --reload

# 或者使用 iptables
sudo iptables -A INPUT -p tcp --dport 8080 -j ACCEPT
sudo iptables -A INPUT -p tcp --dport 9090 -j ACCEPT
```

## 🎮 启动服务

### 1. 启动服务

```bash
# 启用并启动服务
sudo systemctl enable bdwind-gstreamer
sudo systemctl start bdwind-gstreamer

# 检查服务状态
sudo systemctl status bdwind-gstreamer

# 查看日志
sudo journalctl -u bdwind-gstreamer -f
```

### 2. 验证部署

```bash
# 检查端口监听
sudo netstat -tlnp | grep :8080
sudo ss -tlnp | grep :8080

# 测试 HTTP 接口
curl http://localhost:8080/health
curl http://localhost:8080/api/status

# 检查监控指标
curl http://localhost:8080/metrics
```

## 🌐 Web 客户端访问

### 1. 浏览器访问

打开浏览器访问：

```
http://your-server-ip:8080
```

### 2. 功能测试

1. **连接测试**：
   - 检查 WebSocket 连接状态
   - 验证服务器响应

2. **桌面捕获**：
   - 点击"开始捕获"按钮
   - 检查视频流是否正常

3. **控制功能**：
   - 测试鼠标和键盘输入
   - 测试全屏模式

4. **性能监控**：
   - 查看实时统计信息
   - 监控系统资源使用

## 🔧 高级配置

### 1. HTTPS/SSL 配置

```bash
# 生成自签名证书 (测试用)
sudo openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
  -keyout /etc/ssl/private/bdwind.key \
  -out /etc/ssl/certs/bdwind.crt \
  -subj "/C=CN/ST=State/L=City/O=Organization/CN=your-domain.com"

# 更新配置文件
sudo sed -i 's/enable_tls: false/enable_tls: true/' /etc/bdwind-gstreamer/config.yaml

# 重启服务
sudo systemctl restart bdwind-gstreamer
```

### 2. 反向代理配置 (Nginx)

```bash
# 安装 Nginx
sudo apt install -y nginx  # Ubuntu/Debian
sudo dnf install -y nginx  # CentOS/RHEL

# 创建配置文件
sudo tee /etc/nginx/sites-available/bdwind-gstreamer << EOF
server {
    listen 80;
    server_name your-domain.com;
    
    # 重定向到 HTTPS
    return 301 https://\$server_name\$request_uri;
}

server {
    listen 443 ssl http2;
    server_name your-domain.com;
    
    # SSL 配置
    ssl_certificate /etc/ssl/certs/bdwind.crt;
    ssl_certificate_key /etc/ssl/private/bdwind.key;
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers ECDHE-RSA-AES256-GCM-SHA512:DHE-RSA-AES256-GCM-SHA512;
    ssl_prefer_server_ciphers off;
    
    # 代理配置
    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade \$http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_cache_bypass \$http_upgrade;
        proxy_read_timeout 86400;
    }
    
    # WebSocket 代理
    location /ws {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade \$http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host \$host;
        proxy_cache_bypass \$http_upgrade;
        proxy_read_timeout 86400;
    }
    
    # 监控代理
    location /metrics {
        proxy_pass http://127.0.0.1:9090;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        
        # 限制访问
        allow 127.0.0.1;
        allow 10.0.0.0/8;
        allow 172.16.0.0/12;
        allow 192.168.0.0/16;
        deny all;
    }
}
EOF

# 启用站点
sudo ln -s /etc/nginx/sites-available/bdwind-gstreamer /etc/nginx/sites-enabled/
sudo nginx -t
sudo systemctl restart nginx
```

### 3. Docker 部署

```dockerfile
# 创建 Dockerfile
FROM ubuntu:22.04

# 安装依赖
RUN apt-get update && apt-get install -y \
    gstreamer1.0-tools \
    gstreamer1.0-plugins-base \
    gstreamer1.0-plugins-good \
    gstreamer1.0-plugins-bad \
    gstreamer1.0-plugins-ugly \
    gstreamer1.0-libav \
    libgstreamer1.0-dev \
    libgstreamer-plugins-base1.0-dev \
    libx11-dev \
    libxext-dev \
    libxfixes-dev \
    libxdamage-dev \
    && rm -rf /var/lib/apt/lists/*

# 创建用户
RUN useradd -r -s /bin/false -d /app bdwind

# 复制程序
COPY bdwind-gstreamer /app/
COPY config.yaml /app/
COPY web/ /app/web/

# 设置权限
RUN chown -R bdwind:bdwind /app
USER bdwind

WORKDIR /app
EXPOSE 8080 9090

CMD ["./bdwind-gstreamer", "-config", "config.yaml"]
```

```bash
# 构建镜像
docker build -t bdwind-gstreamer:latest .

# 运行容器
docker run -d \
  --name bdwind-gstreamer \
  --restart unless-stopped \
  -p 8080:8080 \
  -p 9090:9090 \
  -v /tmp/.X11-unix:/tmp/.X11-unix:rw \
  -e DISPLAY=:0 \
  bdwind-gstreamer:latest
```

## 📊 监控和维护

### 1. 日志管理

```bash
# 查看实时日志
sudo journalctl -u bdwind-gstreamer -f

# 查看错误日志
sudo journalctl -u bdwind-gstreamer -p err

# 日志轮转配置
sudo tee /etc/logrotate.d/bdwind-gstreamer << EOF
/var/log/bdwind-gstreamer/*.log {
    daily
    missingok
    rotate 30
    compress
    delaycompress
    notifempty
    create 0644 bdwind bdwind
    postrotate
        systemctl reload bdwind-gstreamer
    endscript
}
EOF
```

### 2. 性能监控

```bash
# 安装 Prometheus (可选)
wget https://github.com/prometheus/prometheus/releases/latest/download/prometheus-linux-amd64.tar.gz
tar -xzf prometheus-linux-amd64.tar.gz
sudo mv prometheus-*/prometheus /usr/local/bin/
sudo mv prometheus-*/promtool /usr/local/bin/

# 配置 Prometheus
sudo tee /etc/prometheus/prometheus.yml << EOF
global:
  scrape_interval: 15s

scrape_configs:
  - job_name: 'bdwind-gstreamer'
    static_configs:
      - targets: ['localhost:9090']
EOF

# 安装 Grafana (可选)
sudo apt-get install -y software-properties-common
sudo add-apt-repository "deb https://packages.grafana.com/oss/deb stable main"
wget -q -O - https://packages.grafana.com/gpg.key | sudo apt-key add -
sudo apt-get update
sudo apt-get install -y grafana
```

### 3. 备份和恢复

```bash
# 备份配置
sudo tar -czf bdwind-backup-$(date +%Y%m%d).tar.gz \
  /etc/bdwind-gstreamer/ \
  /var/lib/bdwind-gstreamer/ \
  /opt/bdwind-gstreamer/

# 恢复配置
sudo tar -xzf bdwind-backup-20240101.tar.gz -C /
sudo systemctl restart bdwind-gstreamer
```

## 🔍 故障排除

### 常见问题

1. **服务启动失败**
   ```bash
   # 检查配置文件
   sudo /opt/bdwind-gstreamer/bdwind-gstreamer -config /etc/bdwind-gstreamer/config.yaml
   
   # 检查依赖
   ldd /opt/bdwind-gstreamer/bdwind-gstreamer
   ```

2. **桌面捕获失败**
   ```bash
   # 检查显示环境
   echo $DISPLAY
   xdpyinfo  # X11
   echo $WAYLAND_DISPLAY  # Wayland
   
   # 检查权限
   xhost +local:bdwind
   ```

3. **WebSocket 连接失败**
   ```bash
   # 检查防火墙
   sudo ufw status
   sudo iptables -L
   
   # 检查端口
   sudo netstat -tlnp | grep :8080
   ```

4. **性能问题**
   ```bash
   # 检查系统资源
   top -p $(pgrep bdwind-gstreamer)
   nvidia-smi  # NVIDIA GPU
   vainfo      # Intel GPU
   ```

### 调试模式

```bash
# 启用调试日志
export GST_DEBUG=3
export BDWIND_DEBUG=true

# 手动运行服务
sudo -u bdwind /opt/bdwind-gstreamer/bdwind-gstreamer \
  -config /etc/bdwind-gstreamer/config.yaml
```

## 📈 性能优化

### 1. 系统优化

```bash
# 调整内核参数
sudo tee -a /etc/sysctl.conf << EOF
# 网络优化
net.core.rmem_max = 134217728
net.core.wmem_max = 134217728
net.ipv4.tcp_rmem = 4096 87380 134217728
net.ipv4.tcp_wmem = 4096 65536 134217728

# 文件描述符限制
fs.file-max = 1000000
EOF

sudo sysctl -p

# 调整用户限制
sudo tee -a /etc/security/limits.conf << EOF
bdwind soft nofile 65536
bdwind hard nofile 65536
bdwind soft nproc 4096
bdwind hard nproc 4096
EOF
```

### 2. GStreamer 优化

```bash
# 设置环境变量
sudo tee -a /etc/systemd/system/bdwind-gstreamer.service.d/override.conf << EOF
[Service]
Environment=GST_PLUGIN_PATH=/usr/lib/x86_64-linux-gnu/gstreamer-1.0
Environment=GST_DEBUG_NO_COLOR=1
Environment=GST_DEBUG_DUMP_DOT_DIR=/tmp/gst-debug
EOF
```

## 🔐 安全配置

### 1. 访问控制

```bash
# 配置 iptables 规则
sudo iptables -A INPUT -p tcp --dport 8080 -s 192.168.1.0/24 -j ACCEPT
sudo iptables -A INPUT -p tcp --dport 8080 -j DROP

# 保存规则
sudo iptables-save > /etc/iptables/rules.v4
```

### 2. 用户认证

```yaml
# 更新配置文件
auth:
  enabled: true
  default_role: "viewer"
  admin_users:
    - "admin"
  session_timeout: 3600  # 1小时
  max_sessions: 10
```

## 📞 支持和帮助

- **项目主页**: https://github.com/open-beagle/bdwind-gstreamer
- **问题报告**: https://github.com/open-beagle/bdwind-gstreamer/issues
- **文档**: https://github.com/open-beagle/bdwind-gstreamer/wiki
- **社区**: https://github.com/open-beagle/bdwind-gstreamer/discussions

---

**部署完成后，您就可以通过 Web 浏览器访问 `http://your-server-ip:8080` 来使用 BDWind-GStreamer 桌面捕获服务了！**