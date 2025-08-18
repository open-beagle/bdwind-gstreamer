#!/bin/bash

# BDWind-GStreamer 开发环境安装脚本
# 适用于 Debian 12 和 Ubuntu 24.04 环境

set -e

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

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

# 检查是否为 root 用户
check_root() {
    if [ "$EUID" -eq 0 ]; then
        log_warning "检测到 root 用户，将直接安装系统包"
        USE_SUDO=""
    else
        log_info "检测到非 root 用户，将使用 sudo"
        USE_SUDO="sudo"
        
        # 检查 sudo 是否可用
        if ! command -v sudo >/dev/null 2>&1; then
            log_error "sudo 命令不可用，请以 root 用户运行此脚本"
            exit 1
        fi
    fi
}

# 检查系统环境
check_environment() {
    log_info "检查系统环境..."
    
    # 检查操作系统
    if [ -f /etc/os-release ]; then
        . /etc/os-release
        log_info "操作系统: $PRETTY_NAME"
        
        if [[ "$ID" != "debian" && "$ID" != "ubuntu" ]]; then
            log_warning "此脚本专为 Debian/Ubuntu 设计，当前系统: $ID"
            read -p "是否继续? (y/N): " -n 1 -r
            echo
            if [[ ! $REPLY =~ ^[Yy]$ ]]; then
                exit 1
            fi
        fi
    else
        log_error "无法检测操作系统信息"
        exit 1
    fi
    
    # 检查架构
    ARCH=$(uname -m)
    log_info "系统架构: $ARCH"
    
    # 检查是否为WSL2环境
    if grep -qi microsoft /proc/version 2>/dev/null; then
        log_info "检测到 WSL2 环境"
        IS_WSL2=true
        
        # 检查WSL版本
        if command -v wsl.exe >/dev/null 2>&1; then
            WSL_VERSION=$(wsl.exe --version 2>/dev/null | head -1 || echo "WSL 2")
            log_info "WSL 版本: $WSL_VERSION"
        fi
    else
        IS_WSL2=false
    fi
}

# 更新包管理器
update_package_manager() {
    log_info "更新包管理器..."
    
    # 根据系统类型配置软件源
    if [[ "$ID" == "debian" ]]; then
        # Debian 系统的 non-free 仓库配置
        if [ -f "/etc/apt/sources.list.d/debian.sources" ]; then
            if grep -q "non-free" /etc/apt/sources.list.d/debian.sources; then
                log_info "non-free 仓库已启用"
            else
                log_info "启用 non-free 仓库..."
                $USE_SUDO sed -i 's/Components: main/Components: main contrib non-free non-free-firmware/' /etc/apt/sources.list.d/debian.sources
            fi
        elif [ -f "/etc/apt/sources.list" ]; then
            if ! grep -q "non-free" /etc/apt/sources.list; then
                log_info "启用 non-free 仓库..."
                $USE_SUDO bash -c 'echo "deb http://deb.debian.org/debian bookworm main non-free-firmware" > /etc/apt/sources.list'
                $USE_SUDO bash -c 'echo "deb http://deb.debian.org/debian bookworm-updates main non-free-firmware" >> /etc/apt/sources.list'
                $USE_SUDO bash -c 'echo "deb http://security.debian.org/debian-security bookworm-security main non-free-firmware" >> /etc/apt/sources.list'
            fi
        else
            log_warning "未找到标准的软件源配置文件，跳过软件源配置"
        fi
    elif [[ "$ID" == "ubuntu" ]]; then
        # Ubuntu 系统检查 universe 和 multiverse 仓库
        if ! grep -q "universe" /etc/apt/sources.list /etc/apt/sources.list.d/* 2>/dev/null; then
            log_info "启用 universe 仓库..."
            $USE_SUDO add-apt-repository universe -y
        fi
        if ! grep -q "multiverse" /etc/apt/sources.list /etc/apt/sources.list.d/* 2>/dev/null; then
            log_info "启用 multiverse 仓库..."
            $USE_SUDO add-apt-repository multiverse -y
        fi
    fi
    
    $USE_SUDO apt-get update
}

# 安装基础开发工具
install_basic_tools() {
    log_info "安装基础开发工具..."
    
    # 基础工具包列表
    BASIC_PACKAGES="curl wget git vim nano htop tree jq unzip ca-certificates gnupg lsb-release build-essential pkg-config lsof"
    
    # Ubuntu 24.04 需要额外的软件属性工具
    if [[ "$ID" == "ubuntu" ]]; then
        BASIC_PACKAGES="$BASIC_PACKAGES software-properties-common"
    fi
    
    $USE_SUDO apt-get install -y $BASIC_PACKAGES
    
    log_success "基础开发工具安装完成"
}

# 安装 Go 语言环境
install_golang() {
    log_info "安装 Go 语言环境..."
    
    # 检查是否已安装 Go
    if command -v go >/dev/null 2>&1; then
        GO_VERSION=$(go version | awk '{print $3}')
        log_info "Go 已安装: $GO_VERSION"
        
        # 检查版本是否满足要求 (Go 1.24+ required)
        GO_MAJOR=$(echo "$GO_VERSION" | sed 's/go\([0-9]*\)\.\([0-9]*\).*/\1/')
        GO_MINOR=$(echo "$GO_VERSION" | sed 's/go\([0-9]*\)\.\([0-9]*\).*/\2/')
        
        if [ "$GO_MAJOR" -lt 1 ] || ([ "$GO_MAJOR" -eq 1 ] && [ "$GO_MINOR" -lt 24 ]); then
            log_warning "Go 版本过低，需要 1.24+，当前版本: $GO_VERSION，将重新安装"
            INSTALL_GO=true
        else
            log_success "Go 版本满足要求: $GO_VERSION"
            INSTALL_GO=false
        fi
    else
        INSTALL_GO=true
    fi
    
    if [ "$INSTALL_GO" = true ]; then
        # 下载并安装 Go 1.24 (项目要求版本)
        GO_VERSION="1.24.4"
        
        # 根据系统架构选择合适的包
        case "$ARCH" in
            "x86_64")
                GO_TARBALL="go${GO_VERSION}.linux-amd64.tar.gz"
                ;;
            "aarch64"|"arm64")
                GO_TARBALL="go${GO_VERSION}.linux-arm64.tar.gz"
                ;;
            *)
                log_error "不支持的系统架构: $ARCH"
                exit 1
                ;;
        esac
        
        log_info "下载 Go $GO_VERSION..."
        cd /tmp
        wget -q "https://golang.org/dl/$GO_TARBALL"
        
        log_info "安装 Go..."
        $USE_SUDO rm -rf /usr/local/go
        $USE_SUDO tar -C /usr/local -xzf "$GO_TARBALL"
        
        # 设置环境变量
        if ! grep -q "/usr/local/go/bin" ~/.bashrc 2>/dev/null; then
            echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
            echo 'export GOPATH=$HOME/go' >> ~/.bashrc
            echo 'export PATH=$PATH:$GOPATH/bin' >> ~/.bashrc
        fi
        
        # 临时设置环境变量
        export PATH=$PATH:/usr/local/go/bin
        export GOPATH=$HOME/go
        export PATH=$PATH:$GOPATH/bin
        
        log_success "Go $GO_VERSION 安装完成 (满足 1.24+ 要求)"
    fi
}

# 安装 GStreamer 开发环境
install_gstreamer() {
    log_info "安装 GStreamer 开发环境..."
    
    # GStreamer 核心库和开发头文件
    $USE_SUDO apt-get install -y \
        libgstreamer1.0-dev \
        libgstreamer-plugins-base1.0-dev \
        libgstreamer-plugins-bad1.0-dev \
        gstreamer1.0-tools \
        gstreamer1.0-plugins-base \
        gstreamer1.0-plugins-good \
        gstreamer1.0-plugins-bad \
        gstreamer1.0-plugins-ugly \
        gstreamer1.0-libav
    
    log_success "GStreamer 开发环境安装完成"
}

# 安装 X11 开发支持
install_x11_support() {
    log_info "安装 X11 开发支持..."
    
    # X11 开发库和虚拟显示工具
    $USE_SUDO apt-get install -y \
        libx11-dev \
        libxext-dev \
        libxfixes-dev \
        libxdamage-dev \
        libxcomposite-dev \
        libxrandr-dev \
        libxtst-dev \
        xvfb \
        x11-utils \
        x11-xserver-utils \
        xauth \
        mesa-utils
    
    log_success "X11 开发支持和虚拟显示工具安装完成"
}

# 安装音频开发支持
install_audio_support() {
    log_info "安装音频开发支持..."
    
    $USE_SUDO apt-get install -y \
        libasound2-dev \
        libpulse-dev
    
    log_success "音频开发支持安装完成"
}

# 安装调试工具
install_debug_tools() {
    log_info "安装调试工具..."
    
    $USE_SUDO apt-get install -y \
        netcat-openbsd \
        telnet \
        dnsutils \
        iputils-ping \
        procps \
        psmisc \
        strace \
        tcpdump \
        net-tools
    
    log_success "调试工具安装完成"
}

# 配置WSL2 GPU支持
configure_wsl2_gpu() {
    if [ "$IS_WSL2" != true ]; then
        return 0
    fi
    
    log_info "配置 WSL2 GPU 支持..."
    
    # 检查GPU驱动支持
    if [ ! -d "/dev/dri" ]; then
        log_warning "未检测到 GPU 设备 (/dev/dri)，可能需要在 Windows 端安装最新的 GPU 驱动"
        log_info "请确保："
        log_info "1. Windows 端已安装最新的 AMD 驱动程序"
        log_info "2. WSL2 内核版本 >= 5.10.43.3"
        log_info "3. 在 Windows 端运行: wsl --update"
    else
        log_success "检测到 GPU 设备支持"
        ls -la /dev/dri/
    fi
    
    # 安装Mesa驱动和OpenGL支持
    log_info "安装 Mesa 驱动和 OpenGL 支持..."
    
    # 根据系统版本选择合适的包名
    if [[ "$ID" == "ubuntu" && "$VERSION_ID" == "24.04" ]]; then
        # Ubuntu 24.04 的包名
        MESA_PACKAGES="mesa-va-drivers mesa-vdpau-drivers mesa-vulkan-drivers libgl1-mesa-dri libglx-mesa0 libegl1-mesa-dev libgles2-mesa-dev vainfo vdpauinfo"
    else
        # Debian 或其他版本的包名
        MESA_PACKAGES="mesa-va-drivers mesa-vdpau-drivers mesa-vulkan-drivers libgl1-mesa-dri libglx-mesa0 libgl1-mesa-glx libegl1-mesa libgles2-mesa vainfo vdpauinfo"
    fi
    
    # 逐个安装包，跳过不存在的包
    for package in $MESA_PACKAGES; do
        if $USE_SUDO apt-get install -y "$package" 2>/dev/null; then
            log_info "已安装: $package"
        else
            log_warning "跳过不存在的包: $package"
        fi
    done
    
    # 检查OpenGL支持
    if command -v glxinfo >/dev/null 2>&1; then
        log_info "OpenGL 信息:"
        DISPLAY=:99 glxinfo | grep -E "(OpenGL vendor|OpenGL renderer|OpenGL version)" || log_warning "无法获取 OpenGL 信息，可能需要启动虚拟显示"
    fi
    
    # 检查VA-API支持（硬件加速）
    if command -v vainfo >/dev/null 2>&1; then
        log_info "检查 VA-API 硬件加速支持..."
        DISPLAY=:99 vainfo 2>/dev/null | head -5 || log_warning "VA-API 不可用，将使用软件渲染"
    fi
    
    log_success "WSL2 GPU 配置完成"
}

main() {
    echo "=== BDWind-GStreamer 开发环境安装脚本 ==="
    echo "适用于 Debian 12 和 Ubuntu 24.04 环境"
    echo ""
    
    check_root
    check_environment
    
    log_info "开始安装开发环境..."
    
    update_package_manager
    install_basic_tools
    install_golang
    install_gstreamer
    install_x11_support
    install_audio_support
    install_debug_tools
    configure_wsl2_gpu
    
    log_success "开发环境安装完成！"
    echo ""
    echo "=== 使用说明 ==="
    echo ""
    echo "1. 重新加载环境变量:"
    echo "   source ~/.bashrc"
    echo ""
    echo "2. 编译项目:"
    echo "   go build -o bdwind-gstreamer ./cmd/bdwind-gstreamer"
    echo ""
    echo "3. 运行项目:"
    echo "   ./bdwind-gstreamer --help"
    echo ""
    if [ "$IS_WSL2" = true ]; then
        echo "=== WSL2 特别说明 ==="
        echo ""
        echo "如果遇到 GPU 相关问题，请确保："
        echo "1. Windows 端安装最新 AMD 驱动: https://www.amd.com/support"
        echo "2. 更新 WSL2: wsl --update (在 Windows 命令行运行)"
        echo "3. 重启 WSL2: wsl --shutdown && wsl"
        echo "4. 检查 GPU 设备: ls -la /dev/dri/"
        echo ""
    fi
}

# 处理参数
case "${1:-}" in
    "help"|"-h"|"--help")
        echo "BDWind-GStreamer 开发环境安装脚本"
        echo ""
        echo "用法: $0 [选项]"
        echo ""
        echo "选项:"
        echo "  help    显示此帮助信息"
        echo ""
        echo "此脚本将在 Debian 12 或 Ubuntu 24.04 环境中安装所有必要的开发依赖"
        echo "包括 Go、GStreamer、X11 支持等"
        exit 0
        ;;
esac

# 运行主函数
main "$@"
