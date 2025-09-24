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

# 编译程序
echo "🔨 编译程序..."
mkdir -p .tmp
go build -o .tmp/bdwind-gstreamer ./cmd/bdwind-gstreamer
if [ $? -ne 0 ]; then
    echo "❌ 编译失败"
    exit 1
fi
echo "✅ 编译成功"

# 设置环境变量
export DISPLAY=:99
export BDWIND_DEBUG=true

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
Xvfb :99 -screen 0 1920x1080x24 -ac -nolisten tcp &
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
echo "🌐 Web 界面: http://localhost:8080"
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
.tmp/bdwind-gstreamer --config examples/debug_config.yaml --log-level "$BDWIND_LOG_LEVEL" --log-output "$BDWIND_LOG_OUTPUT" --log-file "$BDWIND_LOG_FILE" &
APP_PID=$!

# 等待应用程序启动
sleep 5

# 检查应用程序是否成功启动
if kill -0 $APP_PID 2>/dev/null; then
    echo "✅ 应用程序启动成功 (PID: $APP_PID)"

    # 检查端口是否在监听
    echo "🔍 检查端口监听状态..."
    if ss -tlnp | grep :8080 >/dev/null 2>&1; then
        echo "✅ 端口8080正在监听"
    else
        echo "⚠️  端口8080未在监听"
        echo "   当前监听的端口:"
        ss -tlnp | grep LISTEN | head -5
    fi

    # 等待HTTP服务可用
    echo "⏳ 等待HTTP服务启动..."
    for i in {1..5}; do
        if curl -s http://localhost:8080/health >/dev/null 2>&1; then
            echo "✅ HTTP服务已就绪"
            break
        fi
        sleep 1
        if [ $i -eq 5 ]; then
            echo "⚠️  HTTP服务检查超时，但应用程序可能仍在运行"
            echo "   请手动检查: curl http://localhost:8080/health"
            break
        fi
    done

    # 显示服务状态
    echo ""
    echo "📋 服务状态:"
    echo "   - HTTP API: http://localhost:8080/api/status"
    echo "   - WebSocket: ws://localhost:8080/ws"
    if [ "$BDWIND_LOG_OUTPUT" = "file" ]; then
        echo "   - 日志文件: $BDWIND_LOG_FILE"
        if [ -f "$BDWIND_LOG_FILE" ]; then
            LOG_SIZE=$(du -h "$BDWIND_LOG_FILE" 2>/dev/null | cut -f1 || echo "0B")
            echo "   - 当前日志大小: $LOG_SIZE"
        fi
    fi
    echo ""
    echo "🔧 故障排除:"
    echo "   - 查看实时日志: tail -f $BDWIND_LOG_FILE"
    echo "   - 查看完整日志: cat $BDWIND_LOG_FILE"
    echo "   - 查看分析工具: 在脚本结束后会显示详细的日志分析命令"
    echo ""

    # 等待应用程序结束
    echo "应用程序正在运行，按Ctrl+C停止..."
    wait $APP_PID
else
    echo "❌ 应用程序启动失败"
    cleanup
fi
