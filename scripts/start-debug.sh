#!/bin/bash
echo "🚀 启动 BDWind-GStreamer 调试环境"

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
export GST_DEBUG=2
export BDWIND_DEBUG=true

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
else
    echo "❌ 虚拟显示启动失败"
    kill $XVFB_PID 2>/dev/null || true
    exit 1
fi

# 启动应用程序
echo "🌐 Web 界面: http://localhost:8080"
echo "🔍 WebSocket 诊断: ./scripts/diagnose_websocket.sh"
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
    
    # 停止虚拟显示
    echo "停止虚拟显示..."
    kill $XVFB_PID 2>/dev/null || true
    pkill -f "Xvfb.*:99" 2>/dev/null || true
    
    # 显示日志文件信息
    if [ -f ".tmp/bdwind-gstreamer.log" ]; then
        echo ""
        echo "📄 日志文件已保存: .tmp/bdwind-gstreamer.log"
        echo "   查看完整日志: cat .tmp/bdwind-gstreamer.log"
        echo "   日志文件大小: $(du -h .tmp/bdwind-gstreamer.log | cut -f1)"
    fi
    
    echo "✅ 清理完成"
    exit 0
}

trap cleanup INT TERM

# 启动应用程序并获取PID
echo "🚀 启动应用程序..."
echo "📝 日志文件: .tmp/bdwind-gstreamer.log"
.tmp/bdwind-gstreamer --config examples/debug_config.yaml > .tmp/bdwind-gstreamer.log 2>&1 &
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
    echo "   - 日志文件: .tmp/bdwind-gstreamer.log"
    echo ""
    echo "🔧 故障排除:"
    echo "   - 查看实时日志: tail -f .tmp/bdwind-gstreamer.log"
    echo "   - 查看完整日志: cat .tmp/bdwind-gstreamer.log"
    echo ""
    
    # 显示最近的日志
    if [ -f ".tmp/bdwind-gstreamer.log" ]; then
        echo "📄 最近的日志输出:"
        tail -n 5 .tmp/bdwind-gstreamer.log | sed 's/^/   /'
        echo ""
    fi
    
    # 等待应用程序结束
    echo "应用程序正在运行，按Ctrl+C停止..."
    wait $APP_PID
else
    echo "❌ 应用程序启动失败"
    cleanup
fi