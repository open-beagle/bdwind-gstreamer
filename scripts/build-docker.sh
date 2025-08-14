#!/bin/bash

# BDWind-GStreamer Docker 构建脚本
# 使用纯 Docker 命令构建和运行容器

set -e

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# 配置
BUILD_IMAGE_NAME="bdwind-gstreamer:build"
RUNTIME_IMAGE_NAME="bdwind-gstreamer:runtime"
CONTAINER_NAME="bdwind-gstreamer"
BUILD_CONTAINER_NAME="bdwind-gstreamer-build"

# 函数：构建编译镜像
build_compile_image() {
    log_info "构建编译镜像..."
    
    docker build -f Dockerfile.build -t "$BUILD_IMAGE_NAME" .
    
    if [ $? -eq 0 ]; then
        log_success "编译镜像构建成功: $BUILD_IMAGE_NAME"
    else
        log_error "编译镜像构建失败"
        exit 1
    fi
}

# 函数：从编译镜像中提取二进制文件
extract_binary() {
    log_info "从编译镜像中提取二进制文件..."
    
    # 创建临时容器来提取二进制文件
    docker create --name "$BUILD_CONTAINER_NAME" "$BUILD_IMAGE_NAME"
    
    # 提取二进制文件到 .tmp 目录
    mkdir -p .tmp
    docker cp "$BUILD_CONTAINER_NAME:/app/bdwind-gstreamer" .tmp/bdwind-gstreamer
    
    # 清理临时容器
    docker rm "$BUILD_CONTAINER_NAME"
    
    if [ -f ".tmp/bdwind-gstreamer" ]; then
        log_success "二进制文件提取成功"
        ls -la .tmp/bdwind-gstreamer
    else
        log_error "二进制文件提取失败"
        exit 1
    fi
}

# 函数：构建运行时镜像
build_runtime_image() {
    local dockerfile=${1:-Dockerfile.runtime}
    local tag_suffix=${2:-}
    
    log_info "构建运行时镜像 (使用 $dockerfile)..."
    
    # 确保二进制文件存在
    if [ ! -f ".tmp/bdwind-gstreamer" ]; then
        log_error "二进制文件不存在，请先运行编译步骤"
        exit 1
    fi
    
    # 根据选择的 Dockerfile 创建临时文件
    local base_image
    if [[ "$dockerfile" == *"debian"* ]]; then
        base_image="docker.io/debian:12"
    else
        base_image="docker.io/debian:12"  # 默认使用 Debian 12
    fi
    
    # 创建临时 Dockerfile 来复制二进制文件
    cat > Dockerfile.runtime.tmp << EOF
# BDWind-GStreamer Runtime Dockerfile (临时)
FROM $base_image

LABEL maintainer="BDWind-GStreamer Team"
LABEL description="Runtime image for BDWind-GStreamer application"

# 启用 non-free 仓库 (仅对 Debian)
RUN if grep -q debian /etc/os-release; then \\
        echo "deb http://deb.debian.org/debian bookworm main non-free-firmware" > /etc/apt/sources.list && \\
        echo "deb http://deb.debian.org/debian bookworm-updates main non-free-firmware" >> /etc/apt/sources.list && \\
        echo "deb http://security.debian.org/debian-security bookworm-security main non-free-firmware" >> /etc/apt/sources.list; \\
    fi

# 安装运行时依赖
RUN apt-get update && apt-get install -y \\
    # GStreamer 运行时
    gstreamer1.0-tools \\
    gstreamer1.0-plugins-base \\
    gstreamer1.0-plugins-good \\
    gstreamer1.0-plugins-bad \\
    gstreamer1.0-plugins-ugly \\
    gstreamer1.0-libav \\
    # X11 和 Wayland 支持
    libx11-6 \\
    libxext6 \\
    libxfixes3 \\
    libxdamage1 \\
    libxcomposite1 \\
    libxrandr2 \\
    libxtst6 \\
    # 虚拟显示支持
    xvfb \\
    x11-utils \\
    x11-xserver-utils \\
    # 硬件加速 (根据发行版选择包名)
    vainfo \\
    mesa-va-drivers \\
    # 音频支持
    pulseaudio \\
    alsa-utils \\
    # 网络工具
    ca-certificates \\
    curl \\
    # 进程管理
    tini \\
    && (apt-get install -y intel-media-va-driver-non-free 2>/dev/null || apt-get install -y intel-media-va-driver 2>/dev/null || true) \\
    && rm -rf /var/lib/apt/lists/*

# 创建非 root 用户
RUN groupadd -r bdwind && useradd -r -g bdwind -s /bin/bash bdwind

# 创建应用目录
WORKDIR /app

# 复制编译好的二进制文件
COPY .tmp/bdwind-gstreamer ./bdwind-gstreamer

# 复制配置文件和脚本
COPY examples/ ./examples/
COPY docs/ ./docs/
COPY scripts/docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh

# 设置脚本权限
RUN chmod +x /usr/local/bin/docker-entrypoint.sh && \\
    chmod +x ./bdwind-gstreamer

# 创建运行时目录
RUN mkdir -p /app/logs /app/data && \\
    chown -R bdwind:bdwind /app

# 设置环境变量
ENV DISPLAY=:99
ENV DISPLAY_NUM=99
ENV DISPLAY_RESOLUTION=1920x1080x24
ENV XDG_RUNTIME_DIR=/tmp/runtime-bdwind

# 暴露端口
EXPOSE 8080

# 健康检查
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \\
    CMD curl -f http://localhost:8080/health || exit 1

# 切换到非 root 用户
USER bdwind

# 使用 tini 作为初始化系统和自定义入口点
ENTRYPOINT ["/usr/bin/tini", "--", "/usr/local/bin/docker-entrypoint.sh"]

# 默认命令
CMD ["./bdwind-gstreamer", "-host", "0.0.0.0", "-port", "8080"]
EOF
    
    docker build -f Dockerfile.runtime.tmp -t "$RUNTIME_IMAGE_NAME" .
    
    # 清理临时文件
    rm -f Dockerfile.runtime.tmp
    
    if [ $? -eq 0 ]; then
        log_success "运行时镜像构建成功: $RUNTIME_IMAGE_NAME"
    else
        log_error "运行时镜像构建失败"
        exit 1
    fi
}

# 函数：运行容器
run_container() {
    log_info "启动容器..."
    
    # 停止并删除现有容器
    docker stop "$CONTAINER_NAME" 2>/dev/null || true
    docker rm "$CONTAINER_NAME" 2>/dev/null || true
    
    # 运行新容器
    docker run -d \
        --name "$CONTAINER_NAME" \
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
        --restart unless-stopped \
        "$RUNTIME_IMAGE_NAME"
    
    if [ $? -eq 0 ]; then
        log_success "容器启动成功: $CONTAINER_NAME"
        log_info "Web 界面: http://localhost:8080"
    else
        log_error "容器启动失败"
        exit 1
    fi
}

# 函数：显示容器状态
show_status() {
    log_info "容器状态:"
    docker ps -a --filter "name=$CONTAINER_NAME"
    
    log_info "容器日志 (最后 20 行):"
    docker logs --tail 20 "$CONTAINER_NAME" 2>/dev/null || log_warning "无法获取容器日志"
}

# 函数：清理资源
cleanup() {
    log_info "清理资源..."
    
    # 停止并删除容器
    docker stop "$CONTAINER_NAME" 2>/dev/null || true
    docker rm "$CONTAINER_NAME" 2>/dev/null || true
    
    # 删除镜像（可选）
    if [ "${1:-}" = "all" ]; then
        docker rmi "$BUILD_IMAGE_NAME" 2>/dev/null || true
        docker rmi "$RUNTIME_IMAGE_NAME" 2>/dev/null || true
    fi
    
    # 清理二进制文件
    rm -f .tmp/bdwind-gstreamer
    rm -f Dockerfile.runtime.tmp
    
    log_success "清理完成"
}

# 函数：显示帮助信息
show_help() {
    echo "BDWind-GStreamer Docker 构建脚本"
    echo ""
    echo "用法: $0 [命令]"
    echo ""
    echo "命令:"
    echo "  build-compile     构建编译镜像"
    echo "  extract           从编译镜像中提取二进制文件"
    echo "  build-runtime     构建运行时镜像 (Debian 12)"
    echo "  build-runtime-debian  构建 Debian 12 运行时镜像"
    echo "  build-runtime-ubuntu  构建 Ubuntu 22.04 运行时镜像"
    echo "  run             运行容器"
    echo "  status          显示容器状态"
    echo "  logs            显示容器日志"
    echo "  shell           进入容器 shell"
    echo "  stop            停止容器"
    echo "  restart         重启容器"
    echo "  cleanup         清理容器"
    echo "  cleanup-all     清理容器和镜像"
    echo "  all             执行完整构建流程"
    echo "  help            显示此帮助信息"
    echo ""
    echo "示例:"
    echo "  $0 all                    # 完整构建和运行流程"
    echo "  $0 build-compile          # 只构建编译镜像"
    echo "  $0 run                    # 运行容器"
    echo "  $0 logs                   # 查看日志"
}

# 主函数
main() {
    case "${1:-}" in
        "build-compile")
            build_compile_image
            ;;
        "extract")
            extract_binary
            ;;
        "build-runtime")
            build_runtime_image
            ;;
        "build-runtime-debian")
            build_runtime_image "Dockerfile.runtime" "-debian"
            ;;
        "build-runtime-ubuntu")
            build_runtime_image "Dockerfile.runtime.ubuntu" "-ubuntu"
            ;;
        "run")
            run_container
            ;;
        "status")
            show_status
            ;;
        "logs")
            docker logs -f "$CONTAINER_NAME"
            ;;
        "shell")
            docker exec -it "$CONTAINER_NAME" /bin/bash
            ;;
        "stop")
            docker stop "$CONTAINER_NAME"
            ;;
        "restart")
            docker restart "$CONTAINER_NAME"
            ;;
        "cleanup")
            cleanup
            ;;
        "cleanup-all")
            cleanup all
            ;;
        "all")
            log_info "开始完整构建流程..."
            build_compile_image
            extract_binary
            build_runtime_image
            run_container
            show_status
            ;;
        "help"|"-h"|"--help"|"")
            show_help
            ;;
        *)
            log_error "未知命令: $1"
            show_help
            exit 1
            ;;
    esac
}

# 设置清理陷阱
trap 'log_info "脚本被中断，正在清理..."; cleanup' INT TERM

# 执行主函数
main "$@"