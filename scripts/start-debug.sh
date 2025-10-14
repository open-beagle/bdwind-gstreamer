#!/bin/bash
echo "🚀 启动 BDWind-GStreamer 调试环境"

# 解析命令行参数
LOG_LEVEL="info"
HELP_MODE="false"

while [[ $# -gt 0 ]]; do
    case $1 in
    --log-level)
        LOG_LEVEL="$2"
        shift 2
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
    echo "                       默认: debug"
    echo "  --no-auto-check      禁用关键日志节点自动检查"
    echo "  --help, -h           显示此帮助信息"
    echo ""
    echo "环境变量:"
    echo "  BDWIND_LOG_LEVEL     日志级别 (覆盖 --log-level)"
    echo "  BDWIND_LOG_OUTPUT    日志输出 (stdout, stderr, file)"
    echo "  BDWIND_LOG_FILE      日志文件路径"
    echo ""
    echo "示例:"
    echo "  $0                           # 使用默认debug级别"
    echo "  $0 --log-level trace         # 使用trace级别"
    echo "  $0 --no-auto-check           # 禁用自动检查"
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
echo "   自动检查: $AUTO_CHECK"
echo ""

# 清理旧的日志文件
echo "🧹 清理旧的日志文件..."
if [ -f ".tmp/bdwind-gstreamer.log" ]; then
    rm -f .tmp/bdwind-gstreamer.log
    echo "✅ 旧日志文件已删除"
fi

# 检查二进制文件
echo "🔨 检查二进制文件..."
mkdir -p .tmp

if [ -f ".tmp/bdwind-gstreamer" ]; then
    echo "✅ 找到修复版本的二进制文件"
    BINARY_FILE=".tmp/bdwind-gstreamer"
elif [ -f ".tmp/bdwind-gstreamer" ]; then
    echo "⚠️  使用标准二进制文件（可能有问题）"
    echo "   建议先运行修复编译"
    BINARY_FILE=".tmp/bdwind-gstreamer"
else
    echo "❌ 找不到二进制文件，使用修复编译..."
    
    # 设置编译环境变量（来自 compile-with-fixes.sh 的修复）
    export CGO_ENABLED=1
    export GOGC=20
    export GODEBUG=checkptr=0  # 编译时禁用指针检查
    
    echo "📋 编译设置:"
    echo "   CGO_ENABLED=$CGO_ENABLED"
    echo "   GODEBUG=$GODEBUG (禁用指针检查)"
    echo "   GOGC=$GOGC"
    echo ""
    
    # 使用修复的编译选项
    echo "🔨 开始修复编译..."
    go build -ldflags="-s -w" -o .tmp/bdwind-gstreamer ./cmd/bdwind-gstreamer
    
    if [ $? -eq 0 ]; then
        echo "✅ 修复编译成功"
        BINARY_FILE=".tmp/bdwind-gstreamer"
        echo "   二进制文件: $BINARY_FILE"
        echo "   文件大小: $(du -h $BINARY_FILE | cut -f1)"
    else
        echo "❌ 修复编译失败"
        exit 1
    fi
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
    echo "   glxinfo 未安装，跳过OpenGL验证"
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
    echo "🛑 停止应用程序..."

    # 停止应用程序
    if [ ! -z "$APP_PID" ]; then
        echo "发送SIGTERM信号到应用程序 (PID: $APP_PID)..."
        kill -TERM $APP_PID 2>/dev/null || true

        # 等待应用程序优雅退出
        for i in {1..10}; do
            if ! kill -0 $APP_PID 2>/dev/null; then
                echo "✅ 应用程序已优雅退出"
                break
            fi
            sleep 1
            if [ $i -eq 10 ]; then
                echo "⚠️  应用程序未在10秒内退出，发送SIGKILL信号..."
                kill -KILL $APP_PID 2>/dev/null || true
                sleep 1
            fi
        done
    fi

    # 停止日志监控
    if [ ! -z "$LOG_MONITOR_PID" ]; then
        echo "停止日志监控..."
        kill $LOG_MONITOR_PID 2>/dev/null || true
    fi

    # 停止 xeyes
    if [ ! -z "$XEYES_PID" ]; then
        kill $XEYES_PID 2>/dev/null || true
    fi

    # 停止虚拟显示
    echo "停止虚拟显示..."
    kill $XVFB_PID 2>/dev/null || true
    pkill -f "Xvfb.*:99" 2>/dev/null || true

    # 显示日志文件信息
    if [ -f "$BDWIND_LOG_FILE" ]; then
        echo ""
        echo "📄 日志文件已保存: $BDWIND_LOG_FILE"
        echo "   查看完整日志: cat $BDWIND_LOG_FILE"
        echo "   日志文件大小: $(du -h "$BDWIND_LOG_FILE" | cut -f1)"
    fi

    echo "✅ 清理完成"
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
echo "   使用二进制文件: $BINARY_FILE"
$BINARY_FILE --config examples/debug_config.yaml --log-level "$BDWIND_LOG_LEVEL" --log-output "$BDWIND_LOG_OUTPUT" --log-file "$BDWIND_LOG_FILE" &
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
    echo "   - WebSocket: ws://localhost:48080/ws"
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

    # 等待应用程序结束
    wait $APP_PID
else
    echo "❌ 应用程序启动失败"
    cleanup
fi
