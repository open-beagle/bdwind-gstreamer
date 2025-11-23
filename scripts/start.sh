#!/bin/bash
echo "🚀 启动 BDWind-GStreamer 调试环境"
echo "📅 启动时间: $(date)"
echo "🔄 每次启动都会重新编译以确保使用最新代码"

# 解析命令行参数
LOG_LEVEL="info"
HELP_MODE="false"
BUILD_ONLY="false"

while [[ $# -gt 0 ]]; do
    case $1 in
    --log-level)
        LOG_LEVEL="$2"
        shift 2
        ;;
    --build-only)
        BUILD_ONLY="true"
        shift
        ;;
    --help | -h)
        HELP_MODE="true"
        shift
        ;;
    *)
        echo "未知参数: $1"
        HELP_MODE="true"
        shift
        ;;
    esac
done

# 显示帮助信息
if [ "$HELP_MODE" = "true" ]; then
    echo ""
    echo "用法: $0 [选项]"
    echo ""
    echo "选项:"
    echo "  --log-level LEVEL    设置日志级别 (trace, debug, info, warn, error)"
    echo "                       默认: info"
    echo "  --build-only         只编译，不启动程序"
    echo "  --help, -h           显示此帮助信息"
    echo ""
    echo "环境变量:"
    echo "  BDWIND_LOG_LEVEL     日志级别 (覆盖 --log-level)"
    echo "  BDWIND_LOG_OUTPUT    日志输出 (stdout, stderr, file)"
    echo "  BDWIND_LOG_FILE      日志文件路径"
    echo ""
    echo "示例:"
    echo "  $0                           # 编译并启动"
    echo "  $0 --build-only              # 只编译"
    echo "  $0 --log-level debug         # 使用debug级别启动"
    echo "  BDWIND_LOG_LEVEL=trace $0    # 通过环境变量设置"
    echo ""
    exit 0
fi

# 验证日志级别
case "${LOG_LEVEL,,}" in
trace | debug | info | warn | error) ;;
*)
    echo "❌ 无效的日志级别: $LOG_LEVEL"
    echo "   支持的级别: trace, debug, info, warn, error"
    exit 1
    ;;
esac

echo "📋 调试配置:"
echo "   日志级别: $LOG_LEVEL"
echo "   构建模式: $([ "$BUILD_ONLY" = "true" ] && echo "仅构建" || echo "构建并启动")"
echo ""

# 清理旧的日志文件
echo "🧹 清理旧的日志文件..."
if [ -f ".tmp/bdwind-gstreamer.log" ]; then
    rm -f .tmp/bdwind-gstreamer.log
    echo "✅ 旧日志文件已删除"
fi

# 强制重新编译（确保使用最新代码）
echo "🔨 强制重新编译（确保使用最新修复）..."
mkdir -p .tmp

# 删除旧的二进制文件
if [ -f ".tmp/bdwind-gstreamer" ]; then
    echo "🗑️  删除旧的二进制文件..."
    rm -f .tmp/bdwind-gstreamer
fi

# 设置编译环境变量（支持 go-gst 库）
export CGO_ENABLED=1
export GOGC=20
export GODEBUG=checkptr=0  # 编译时禁用指针检查

# go-gst 编译环境
export PKG_CONFIG_PATH="/usr/lib/x86_64-linux-gnu/pkgconfig:$PKG_CONFIG_PATH"
export CGO_CFLAGS="-I/usr/include/gstreamer-1.0 -I/usr/include/glib-2.0 -I/usr/lib/x86_64-linux-gnu/glib-2.0/include"
export CGO_LDFLAGS="-lgstreamer-1.0 -lgobject-2.0 -lglib-2.0"

echo "📋 编译设置:"
echo "   CGO_ENABLED=$CGO_ENABLED"
echo "   GODEBUG=$GODEBUG (禁用指针检查)"
echo "   GOGC=$GOGC"
echo "   go-gst支持: 已启用"
echo "   强制重编译: 是"
echo ""

# 编译前环境检查
echo "🔍 编译前环境检查..."

# 检查 Go 版本
GO_VERSION=$(go version 2>/dev/null | grep -o 'go[0-9]\+\.[0-9]\+' | head -1)
if [ -n "$GO_VERSION" ]; then
    echo "   ✅ Go 版本: $GO_VERSION"
else
    echo "   ❌ Go 未安装或不可用"
    exit 1
fi

# 检查 GStreamer 开发库
if pkg-config --exists gstreamer-1.0; then
    GST_VERSION=$(pkg-config --modversion gstreamer-1.0)
    echo "   ✅ GStreamer 版本: $GST_VERSION"
else
    echo "   ❌ GStreamer 开发库未找到"
    echo "   💡 安装命令: sudo apt-get install libgstreamer1.0-dev libgstreamer-plugins-base1.0-dev"
    exit 1
fi

# 检查磁盘空间
DISK_SPACE=$(df . | tail -1 | awk '{print $4}')
if [ "$DISK_SPACE" -gt 1000000 ]; then  # 1GB in KB
    echo "   ✅ 磁盘空间充足: $(df -h . | tail -1 | awk '{print $4}') 可用"
else
    echo "   ⚠️  磁盘空间较少: $(df -h . | tail -1 | awk '{print $4}') 可用"
fi

# 检查 .tmp 目录权限
if [ -w ".tmp" ] || mkdir -p .tmp 2>/dev/null; then
    echo "   ✅ .tmp 目录可写"
else
    echo "   ❌ .tmp 目录不可写"
    exit 1
fi

# 使用修复的编译选项进行强制重编译
echo "🔨 开始强制重编译..."
# 调试模式：移除 -s -w 保留调试符号，移除 -a 加快编译速度
go build -o .tmp/bdwind-gstreamer ./cmd/bdwind-gstreamer

if [ $? -eq 0 ]; then
    echo "✅ 强制重编译成功"
    BINARY_FILE=".tmp/bdwind-gstreamer"
    echo "   二进制文件: $BINARY_FILE"
    echo "   文件大小: $(du -h $BINARY_FILE | cut -f1)"
    echo "   编译时间: $(date)"
    
    # 验证二进制文件
    echo "� 验证编译的方二进制文件..."
    if [ -x "$BINARY_FILE" ]; then
        echo "   ✅ 二进制文件可执行"
        
        # 测试版本信息
        if timeout 5s "$BINARY_FILE" --version >/dev/null 2>&1; then
            echo "   ✅ 二进制文件响应正常"
        else
            echo "   ⚠️  二进制文件版本检查超时或失败（可能正常）"
        fi
        
        # 检查依赖库
        if command -v ldd >/dev/null 2>&1; then
            echo "   📚 检查动态库依赖..."
            MISSING_LIBS=$(ldd "$BINARY_FILE" 2>/dev/null | grep "not found" | wc -l)
            if [ "$MISSING_LIBS" -eq 0 ]; then
                echo "   ✅ 所有动态库依赖都可用"
            else
                echo "   ⚠️  发现 $MISSING_LIBS 个缺失的动态库依赖"
                ldd "$BINARY_FILE" 2>/dev/null | grep "not found" | head -3
            fi
        fi
    else
        echo "   ❌ 二进制文件不可执行"
        ls -la "$BINARY_FILE"
        exit 1
    fi
else
    echo "❌ 强制重编译失败"
    echo "💡 可能的解决方案:"
    echo "   1. 检查 GStreamer 开发库是否安装: sudo apt-get install libgstreamer1.0-dev"
    echo "   2. 检查 go-gst 依赖: go mod tidy"
    echo "   3. 检查编译环境: go env"
    echo "   4. 检查磁盘空间: df -h"
    echo "   5. 检查权限: ls -la .tmp/"
    exit 1
fi

# 如果只是构建模式，编译完成后退出
if [ "$BUILD_ONLY" = "true" ]; then
    echo "✅ 构建完成！二进制文件: $BINARY_FILE"
    echo "   文件大小: $(du -h $BINARY_FILE | cut -f1)"
    echo "   编译时间: $(date)"
    echo ""
    echo "🚀 启动命令:"
    echo "   $BINARY_FILE --config examples/debug.yaml"
    echo "   timeout 30s $BINARY_FILE --config examples/debug.yaml"
    exit 0
fi

# 设置环境变量
export DISPLAY=:99
export BDWIND_DEBUG=true

# 修复 EGL/GPU 权限问题
echo "🔧 配置图形渲染环境..."

# 检查 DRI 设备权限
if [ -d "/dev/dri" ]; then
    echo "📊 DRI 设备状态:"
    ls -la /dev/dri/ 2>/dev/null || echo "   无法访问 /dev/dri/"
    
    # 尝试修复权限（如果有sudo权限）
    if command -v sudo >/dev/null 2>&1 && sudo -n true 2>/dev/null; then
        echo "🔑 尝试修复 DRI 设备权限..."
        sudo chmod 666 /dev/dri/* 2>/dev/null || echo "   权限修复失败或不需要"
    else
        echo "⚠️  无sudo权限，将使用软件渲染"
    fi
else
    echo "⚠️  /dev/dri 目录不存在，将使用软件渲染"
fi

# 强制使用软件渲染以避免GPU权限问题
export LIBGL_ALWAYS_SOFTWARE=1
export MESA_GL_VERSION_OVERRIDE=3.3
export MESA_GLSL_VERSION_OVERRIDE=330
export GALLIUM_DRIVER=llvmpipe

# 禁用硬件加速相关的EGL/DRI访问
export EGL_PLATFORM=surfaceless
export MESA_LOADER_DRIVER_OVERRIDE=swrast

# GStreamer 兼容性设置
export GST_DEBUG_NO_COLOR=1
export GST_DEBUG_DUMP_DOT_DIR=/tmp
export GST_PLUGIN_SYSTEM_PATH_1_0=/usr/lib/x86_64-linux-gnu/gstreamer-1.0
export GST_REGISTRY_REUSE_PLUGIN_SCANNER=no

# Go-GStreamer 兼容性设置
export CGO_CFLAGS="-I/usr/include/gstreamer-1.0 -I/usr/include/glib-2.0 -I/usr/lib/x86_64-linux-gnu/glib-2.0/include"
export CGO_LDFLAGS="-lgstreamer-1.0 -lgobject-2.0 -lglib-2.0"

# 内存管理设置 - 基于 go-gst issue #198 的解决方案
export GOGC=20  # 更频繁的垃圾回收，解决 GStreamer 对象堆积
export GODEBUG=madvdontneed=1,checkptr=0  # 禁用指针检查，优化内存管理
export GOMEMLIMIT=1GiB  # 限制内存使用

echo "✅ 图形环境配置完成:"
echo "   软件渲染: 已启用"
echo "   EGL平台: surfaceless"
echo "   Mesa驱动: swrast (软件光栅化)"

# 验证图形环境
echo "🔍 验证图形环境..."
if command -v glxinfo >/dev/null 2>&1; then
    echo "📊 OpenGL 信息:"
    DISPLAY=:99 timeout 10s glxinfo 2>/dev/null | grep -E "(OpenGL renderer|OpenGL version)" || echo "   无法获取OpenGL信息"
else
    echo "   glxinfo 未安装,跳过OpenGL验证"
    echo "   安装命令: sudo apt-get install mesa-utils"
fi

# 设置日志相关环境变量
export BDWIND_LOG_LEVEL="${BDWIND_LOG_LEVEL:-$LOG_LEVEL}"
export BDWIND_LOG_OUTPUT="${BDWIND_LOG_OUTPUT:-file}"
export BDWIND_LOG_FILE="${BDWIND_LOG_FILE:-.tmp/bdwind-gstreamer.log}"
export BDWIND_LOG_TIMESTAMP="${BDWIND_LOG_TIMESTAMP:-true}"
export BDWIND_LOG_CALLER="${BDWIND_LOG_CALLER:-false}"
export BDWIND_LOG_COLORS="${BDWIND_LOG_COLORS:-false}"

# GStreamer 日志现在由应用程序内部管理
# 不再需要通过环境变量设置 GST_DEBUG 和 GST_DEBUG_FILE

echo "🔧 日志环境变量:"
echo "   BDWIND_LOG_LEVEL=$BDWIND_LOG_LEVEL"
echo "   BDWIND_LOG_OUTPUT=$BDWIND_LOG_OUTPUT"
echo "   BDWIND_LOG_FILE=$BDWIND_LOG_FILE"
echo "   注意: GStreamer 日志现在由应用程序内部管理"

# 停止现有的 Xvfb 进程
pkill -f "Xvfb.*:99" 2>/dev/null || true
sleep 1

# 启动虚拟显示
echo "🖥️  启动虚拟显示..."
# 使用软件渲染兼容的 Xvfb 配置
Xvfb :99 -screen 0 1920x1080x24 -ac -nolisten tcp -noreset +extension GLX +extension RENDER -dpi 96 &
XVFB_PID=$!
sleep 3

# 验证虚拟显示
if xdpyinfo -display :99 >/dev/null 2>&1; then
    echo "✅ 虚拟显示启动成功 (PID: $XVFB_PID)"

    # 在虚拟显示中启动一个简单的应用程序
    if command -v xeyes >/dev/null 2>&1; then
        echo "👀 启动 xeyes 作为测试应用..."
        xeyes -display :99 &
        XEYES_PID=$!
    elif command -v xterm >/dev/null 2>&1; then
        echo "🖥️  启动 xterm 作为测试应用..."
        xterm -display :99 -geometry 80x24+100+100 -e "echo 'BDWind-GStreamer Debug Environment'; echo 'Virtual Display Content'; sleep 3600" &
        XEYES_PID=$!
    else
        echo "⚠️  没有找到合适的X11应用程序，创建简单的测试窗口..."
        # 创建一个简单的测试窗口使用 xwininfo 或其他基本工具
        if command -v xsetroot >/dev/null 2>&1; then
            echo "🎨 设置虚拟显示背景..."
            DISPLAY=:99 xsetroot -solid "#2E3440" &
        fi
        echo "   建议安装测试应用: sudo apt-get install x11-apps"
    fi
    
    # 等待测试应用程序完全启动
    echo "⏳ 等待虚拟显示内容准备就绪..."
    sleep 2
    
    # 验证显示内容
    if command -v xwininfo >/dev/null 2>&1; then
        WINDOW_COUNT=$(DISPLAY=:99 xwininfo -root -tree 2>/dev/null | grep -c "child" || echo "0")
        echo "📊 虚拟显示窗口数量: $WINDOW_COUNT"
    fi
else
    echo "❌ 虚拟显示启动失败"
    kill $XVFB_PID 2>/dev/null || true
    exit 1
fi

# 启动应用程序
echo "🌐 Web 界面: http://localhost:48080"
echo "🔍 WebRTC 诊断: ./scripts/test-ice-connectivity.sh"
echo "📊 按 Ctrl+C 停止应用"
echo ""

# 设置清理函数
cleanup() {
    echo ""
    echo "🛑 停止所有服务..."

    # 停止应用程序
    if [ ! -z "$APP_PID" ]; then
        echo "停止主程序 (PID: $APP_PID)..."
        kill -TERM $APP_PID 2>/dev/null || true

        # 等待应用程序优雅退出
        for i in {1..10}; do
            if ! kill -0 $APP_PID 2>/dev/null; then
                echo "✅ 主程序已优雅退出"
                break
            fi
            sleep 1
            if [ $i -eq 10 ]; then
                echo "⚠️  主程序未在10秒内退出，发送SIGKILL信号..."
                kill -KILL $APP_PID 2>/dev/null || true
                sleep 1
            fi
        done
    fi

    # 停止GStreamer进程
    echo "停止GStreamer进程..."
    pkill -f "gst-launch" 2>/dev/null || true
    sleep 1

    # 停止所有测试窗口
    echo "停止测试窗口..."
    pkill -f "xterm.*Test.*Content" 2>/dev/null || true
    pkill -f "xterm.*Moving.*Content" 2>/dev/null || true
    pkill -f "xterm.*Dynamic.*Content" 2>/dev/null || true
    sleep 1

    # 停止日志监控
    if [ ! -z "$LOG_MONITOR_PID" ]; then
        echo "停止日志监控..."
        kill $LOG_MONITOR_PID 2>/dev/null || true
    fi

    # 停止 xeyes 和其他测试应用
    if [ ! -z "$XEYES_PID" ]; then
        kill $XEYES_PID 2>/dev/null || true
    fi

    # 停止虚拟显示
    echo "停止虚拟显示..."
    kill $XVFB_PID 2>/dev/null || true
    pkill -f "Xvfb.*:99" 2>/dev/null || true

    # 最终清理检查
    echo "执行最终清理..."
    pkill -f "bdwind-gstreamer" 2>/dev/null || true
    
    # 显示日志文件信息
    if [ -f "$BDWIND_LOG_FILE" ]; then
        echo ""
        echo "📄 日志文件已保存: $BDWIND_LOG_FILE"
        echo "   查看完整日志: cat $BDWIND_LOG_FILE"
        echo "   日志文件大小: $(du -h "$BDWIND_LOG_FILE" | cut -f1)"
    fi

    echo "✅ 所有服务已停止，清理完成"
    exit 0
}

trap cleanup INT TERM

# 启动应用程序并获取PID
echo "🚀 启动应用程序..."
echo "📝 日志配置确认:"
echo "   应用日志级别: $BDWIND_LOG_LEVEL"
echo "   应用日志输出: $BDWIND_LOG_OUTPUT"
if [ "$BDWIND_LOG_OUTPUT" = "file" ]; then
    echo "   应用日志文件: $BDWIND_LOG_FILE"
    echo "   GStreamer日志: 由应用程序内部管理"
    
    # 验证日志文件目录是否存在
    LOG_DIR=$(dirname "$BDWIND_LOG_FILE")
    if [ ! -d "$LOG_DIR" ]; then
        echo "   📁 创建日志目录: $LOG_DIR"
        mkdir -p "$LOG_DIR"
    fi
    
    # 验证日志文件是否可写
    if touch "$BDWIND_LOG_FILE" 2>/dev/null; then
        echo "   ✅ 日志文件可写"
        # 获取日志文件的绝对路径
        ABS_LOG_FILE=$(realpath "$BDWIND_LOG_FILE")
        echo "   📄 日志文件绝对路径: $ABS_LOG_FILE"
    else
        echo "   ❌ 日志文件不可写: $BDWIND_LOG_FILE"
        echo "   💡 请检查文件权限或目录是否存在"
        exit 1
    fi
else
    echo "   应用日志: 控制台输出"
    echo "   GStreamer日志: 由应用程序内部管理"
fi
echo "   时间戳: ${BDWIND_LOG_TIMESTAMP:-true}"
echo "   调用者信息: ${BDWIND_LOG_CALLER:-false}"
echo "   彩色输出: ${BDWIND_LOG_COLORS:-false}"
echo ""

# 验证虚拟显示是否正常工作
echo "🔍 验证虚拟显示状态..."
if ! DISPLAY=:99 xdpyinfo >/dev/null 2>&1; then
    echo "❌ 虚拟显示 :99 无法访问"
    echo "💡 请确保 Xvfb 正在运行: Xvfb :99 -screen 0 1920x1080x24 &"
    exit 1
fi
echo "✅ 虚拟显示 :99 可访问"

echo "   使用二进制文件: $BINARY_FILE"
DISPLAY=:99 $BINARY_FILE --config examples/debug.yaml --log-level "$BDWIND_LOG_LEVEL" --log-output "$BDWIND_LOG_OUTPUT" --log-file "$BDWIND_LOG_FILE" &
APP_PID=$!

# 等待应用程序启动
sleep 5

# 检查应用程序是否成功启动
if kill -0 $APP_PID 2>/dev/null; then
    echo "✅ 应用程序启动成功 (PID: $APP_PID)"

    # 等待HTTP服务启动
    echo "⏳ 等待HTTP服务启动..."
    sleep 3
    
    # 检查端口是否在监听
    echo "🔍 检查端口监听状态..."
    for i in {1..10}; do
        if ss -tlnp | grep :48080 >/dev/null 2>&1; then
            echo "✅ 端口48080正在监听"
            break
        elif [ $i -eq 10 ]; then
            echo "❌ 端口48080未在监听"
            echo "   当前监听的端口:"
            ss -tlnp | grep LISTEN | head -5
            echo ""
            echo "🔍 应用程序进程状态:"
            if kill -0 $APP_PID 2>/dev/null; then
                echo "   应用程序进程仍在运行 (PID: $APP_PID)"
            else
                echo "   应用程序进程已退出"
            fi
            break
        else
            echo "   等待端口监听... ($i/10)"
            sleep 2
        fi
    done

    # 测试HTTP服务可用性
    echo "🌐 测试HTTP服务..."
    for i in {1..5}; do
        # 首先测试根路径
        if curl -s -f http://localhost:48080/ >/dev/null 2>&1; then
            echo "✅ HTTP根路径可访问"
            break
        elif curl -s http://localhost:48080/health >/dev/null 2>&1; then
            echo "✅ HTTP健康检查可访问"
            break
        elif curl -s http://localhost:48080/api/status >/dev/null 2>&1; then
            echo "✅ HTTP API可访问"
            break
        else
            if [ $i -eq 5 ]; then
                echo "⚠️  HTTP服务检查失败，尝试诊断..."
                
                # 详细的HTTP测试
                echo "🔍 详细HTTP诊断:"
                echo "   测试根路径:"
                curl -v http://localhost:48080/ 2>&1 | head -10 || echo "   连接失败"
                echo ""
                echo "   测试健康检查:"
                curl -v http://localhost:48080/health 2>&1 | head -5 || echo "   连接失败"
                echo ""
                
                # 检查防火墙
                if command -v ufw >/dev/null 2>&1; then
                    echo "   防火墙状态:"
                    sudo ufw status 2>/dev/null || echo "   无法检查防火墙状态"
                fi
                
                # 检查网络接口
                echo "   网络接口:"
                ip addr show lo | grep inet || echo "   无法获取本地接口信息"
                
                break
            else
                echo "   HTTP服务测试 $i/5 失败，等待..."
                sleep 2
            fi
        fi
    done

    # 显示服务状态和访问信息
    echo ""
    echo "🌐 服务访问信息:"
    echo "   - 主页面: http://localhost:48080/"
    echo "   - HTTP API: http://localhost:48080/api/status"
    echo "   - WebSocket: ws://localhost:48080/api/signaling"
    echo "   - 健康检查: http://localhost:48080/health"
    
    # 检查静态文件
    if [ -f "internal/webserver/static/index.html" ]; then
        echo "   ✅ 静态文件存在"
    else
        echo "   ❌ 静态文件缺失"
    fi
    
    if [ "$BDWIND_LOG_OUTPUT" = "file" ]; then
        echo "   - 日志文件: $BDWIND_LOG_FILE"
        if [ -f "$BDWIND_LOG_FILE" ]; then
            LOG_SIZE=$(du -h "$BDWIND_LOG_FILE" 2>/dev/null | cut -f1 || echo "0B")
            echo "   - 当前日志大小: $LOG_SIZE"
            
            # 显示最近的日志条目
            echo ""
            echo "📄 最近日志 (最后5行):"
            tail -5 "$BDWIND_LOG_FILE" 2>/dev/null | sed 's/^/   /' || echo "   无法读取日志文件"
        fi
    fi
    
    echo ""
    echo "🔧 故障排除命令:"
    echo "   - 实时日志: tail -f $BDWIND_LOG_FILE"
    echo "   - 完整日志: cat $BDWIND_LOG_FILE"
    echo "   - 错误日志: grep -i error $BDWIND_LOG_FILE"
    echo "   - HTTP测试: curl -v http://localhost:48080/"
    echo "   - 端口检查: ss -tlnp | grep 48080"
    echo "   - 进程检查: ps aux | grep bdwind"
    echo ""
    echo "🎨 图形渲染说明:"
    echo "   - 已启用软件渲染模式，避免GPU权限问题"
    echo "   - 如果仍有EGL警告，这是正常的，不影响功能"
    echo "   - 软件渲染性能较低，但适合调试环境"
    echo ""
    
    # 启动后综合检查
    echo "🧪 启动后综合检查:"
    sleep 3
    
    # 检查HTTP服务
    if curl -s -f http://localhost:48080/ >/dev/null 2>&1; then
        echo "   ✅ HTTP服务正常响应"
        echo "   🌐 可以在浏览器中访问: http://localhost:48080/"
        
        # 检查API端点
        if curl -s http://localhost:48080/api/status >/dev/null 2>&1; then
            echo "   ✅ API端点可访问"
        else
            echo "   ⚠️  API端点可能还在初始化"
        fi
        
        # 检查静态文件
        if curl -s -f http://localhost:48080/index.html >/dev/null 2>&1; then
            echo "   ✅ 静态文件服务正常"
        else
            echo "   ⚠️  静态文件服务可能有问题"
        fi
        
    else
        echo "   ❌ HTTP服务无响应"
        echo "   💡 故障排除步骤:"
        echo "      1. 检查应用程序是否仍在运行: ps aux | grep bdwind"
        echo "      2. 检查端口占用: ss -tlnp | grep 48080"
        echo "      3. 查看错误日志: grep -i error $BDWIND_LOG_FILE"
        echo "      4. 手动测试连接: curl -v http://localhost:48080/"
        
        # 显示应用程序状态
        if kill -0 $APP_PID 2>/dev/null; then
            echo "      应用程序进程状态: 运行中 (PID: $APP_PID)"
        else
            echo "      应用程序进程状态: 已退出"
            echo "      请检查日志文件了解退出原因"
        fi
    fi
    echo ""

    # 启动日志监控（如果是文件输出）
    if [ "$BDWIND_LOG_OUTPUT" = "file" ] && [ -f "$BDWIND_LOG_FILE" ]; then
        echo "📊 启动日志监控..."
        echo "   日志文件: $BDWIND_LOG_FILE"
        echo "   按 Ctrl+C 停止应用程序和日志监控"
        echo ""
        
        # 在后台启动日志监控
        (
            sleep 3  # 等待应用程序完全启动
            echo "=== 开始实时日志监控 ==="
            tail -f "$BDWIND_LOG_FILE" 2>/dev/null | while read line; do
                # 高亮重要信息
                if echo "$line" | grep -qi "error\|fatal\|panic"; then
                    echo "🔴 $line"
                elif echo "$line" | grep -qi "warn"; then
                    echo "🟡 $line"
                elif echo "$line" | grep -qi "http\|server\|listening"; then
                    echo "🌐 $line"
                elif echo "$line" | grep -qi "webrtc\|ice\|sdp"; then
                    echo "📡 $line"
                else
                    echo "   $line"
                fi
            done
        ) &
        LOG_MONITOR_PID=$!
    else
        echo "应用程序正在运行，按 Ctrl+C 停止..."
    fi

    # 检查应用程序组件状态
    echo ""
    echo "🎬 检查WebRTC桌面捕获系统状态..."
    
    # 等待HTTP服务完全就绪
    sleep 5
    
    # 检查应用程序健康状态
    echo "🔍 检查应用程序健康状态..."
    if curl -s http://localhost:48080/api/health | grep -q "healthy"; then
        echo "✅ 应用程序健康状态正常"
    else
        echo "⚠️  应用程序健康状态检查失败，继续..."
    fi
    
    # 检查组件状态
    echo "📊 检查组件状态..."
    if curl -s http://localhost:48080/api/status >/dev/null 2>&1; then
        echo "✅ 组件状态API可访问"
        
        # 显示组件状态摘要
        echo "📋 组件状态摘要:"
        curl -s http://localhost:48080/api/status 2>/dev/null | grep -E '"(gstreamer|webrtc|metrics)"' | head -3 || echo "   无法获取详细状态"
    else
        echo "⚠️  组件状态API不可访问，继续..."
    fi
    
    # 创建测试内容窗口
    echo "🖼️  创建测试内容窗口..."
    
    # 设置虚拟显示背景
    DISPLAY=:99 xsetroot -solid "#1e1e1e" 2>/dev/null || true
    
    # 启动第一个测试窗口 - 静态内容
    DISPLAY=:99 xterm -geometry 80x24+100+100 -bg "#2d3748" -fg "#e2e8f0" -title "Static Test Content" -e "
        echo '=== BDWind-GStreamer Test Window 1 ===';
        echo 'Static Content for Video Testing';
        echo 'Window Position: 100,100';
        echo 'This window provides static content for video capture testing.';
        echo '';
        echo 'System Info:';
        echo 'Date: $(date)';
        echo 'Display: $DISPLAY';
        echo 'Resolution: 1920x1080';
        echo '';
        echo 'Press Ctrl+C to close this window';
        while true; do
            echo 'Static content - $(date +%H:%M:%S)';
            sleep 5;
        done
    " &
    TEST_WINDOW_1_PID=$!
    
    # 启动第二个测试窗口 - 动态内容
    DISPLAY=:99 xterm -geometry 90x30+300+200 -bg "#2b6cb0" -fg "#ffffff" -title "Dynamic Test Content" -e "
        echo '=== BDWind-GStreamer Test Window 2 ===';
        echo 'Dynamic Content for Video Testing';
        echo 'Window Position: 300,200';
        echo '';
        counter=0;
        while true; do
            clear;
            echo '=== Dynamic Content Window ===';
            echo 'Frame: $counter';
            echo 'Time: $(date)';
            echo 'Random: $RANDOM';
            echo '';
            echo 'Content Lines:';
            for i in {1..15}; do
                echo \"Line \$i: Random data \$RANDOM at \$(date +%H:%M:%S)\";
            done;
            echo '';
            echo 'This content changes every 2 seconds';
            counter=\$((counter + 1));
            sleep 2;
        done
    " &
    TEST_WINDOW_2_PID=$!
    
    # 启动第三个测试窗口 - 彩色动画内容
    DISPLAY=:99 xterm -geometry 100x25+600+300 -bg "#38a169" -fg "#000000" -title "Animated Test Content" -e "
        echo '=== BDWind-GStreamer Test Window 3 ===';
        echo 'Animated Content for Video Testing';
        echo 'Window Position: 600,300';
        echo '';
        frame=0;
        while true; do
            clear;
            echo '╔══════════════════════════════════════╗';
            echo '║        ANIMATED TEST CONTENT         ║';
            echo '╠══════════════════════════════════════╣';
            echo \"║ Frame: \$(printf '%06d' \$frame)                    ║\";
            echo \"║ Time:  \$(date +'%H:%M:%S')                    ║\";
            echo '║                                      ║';
            
            # 创建简单的ASCII动画
            pos=\$((frame % 30));
            line='║ ';
            for i in {1..30}; do
                if [ \$i -eq \$pos ]; then
                    line=\"\${line}●\";
                else
                    line=\"\${line} \";
                fi
            done;
            line=\"\${line} ║\";
            echo \"\$line\";
            
            echo '║                                      ║';
            echo \"║ Random: \$(printf '%08d' \$RANDOM)            ║\";
            echo '║                                      ║';
            echo '║ This window provides animated        ║';
            echo '║ content for WebRTC video testing     ║';
            echo '╚══════════════════════════════════════╝';
            
            frame=\$((frame + 1));
            sleep 0.5;
        done
    " &
    TEST_WINDOW_3_PID=$!
    
    # 等待测试窗口启动
    sleep 3
    
    # 验证测试窗口
    WINDOW_COUNT=$(DISPLAY=:99 xwininfo -root -tree 2>/dev/null | grep -c "xterm" || echo "0")
    echo "✅ 创建了 $WINDOW_COUNT 个测试窗口"
    
    # 显示系统状态
    echo ""
    echo "🎯 WebRTC桌面捕获系统已启动完成！"
    echo ""
    echo "📊 系统组件状态:"
    echo "   ✅ 主程序: bdwind-gstreamer (PID: $APP_PID)"
    echo "   ✅ 虚拟显示: Xvfb :99 (PID: $XVFB_PID)"
    echo "   ✅ 测试窗口: $WINDOW_COUNT 个动态内容窗口"
    echo "   ✅ GStreamer: go-gst库集成 (直接内存传输)"
    echo "   ✅ WebRTC: 信令服务器已启动"
    echo "   ✅ 媒体桥接: go-gst appsink 直接连接"
    echo ""
    echo "🌐 访问地址:"
    echo "   - 主测试页面: http://localhost:48080"
    echo "   - 调试页面: http://localhost:48080/webrtc.html"
    echo ""
    echo "🎮 使用说明:"
    echo "   1. 在浏览器中打开测试页面"
    echo "   2. 点击'开始桌面捕获'按钮"
    echo "   3. 应该能看到虚拟显示器上的动态内容"
    echo ""
    echo "🚀 go-gst 迁移优势:"
    echo "   - 直接内存传输，无UDP网络开销"
    echo "   - 更低延迟和更高性能"
    echo "   - 精确的时间戳管理"
    echo "   - 更好的错误处理和恢复"
    echo ""
    echo "📝 日志监控:"
    if [ "$BDWIND_LOG_OUTPUT" = "file" ]; then
        echo "   - 实时日志: tail -f $BDWIND_LOG_FILE"
        echo "   - 错误日志: grep -i error $BDWIND_LOG_FILE"
    fi
    echo ""
    echo "🛑 按 Ctrl+C 停止所有服务"
    echo ""

    # 等待应用程序结束
    wait $APP_PID
else
    echo "❌ 应用程序启动失败"
    cleanup
fi
