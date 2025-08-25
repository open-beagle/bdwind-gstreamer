#!/bin/bash
echo "🚀 启动 BDWind-GStreamer 调试环境"

# 解析命令行参数
LOG_LEVEL="debug"
AUTO_CHECK="true"
HELP_MODE="false"

while [[ $# -gt 0 ]]; do
    case $1 in
        --log-level)
            LOG_LEVEL="$2"
            shift 2
            ;;
        --no-auto-check)
            AUTO_CHECK="false"
            shift
            ;;
        --help|-h)
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
    trace|debug|info|warn|error)
        ;;
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
export GST_DEBUG=4
export BDWIND_DEBUG=true

# 设置日志相关环境变量
export BDWIND_LOG_LEVEL="${BDWIND_LOG_LEVEL:-$LOG_LEVEL}"
export BDWIND_LOG_OUTPUT="${BDWIND_LOG_OUTPUT:-file}"
export BDWIND_LOG_FILE="${BDWIND_LOG_FILE:-.tmp/bdwind-gstreamer.log}"
export BDWIND_LOG_TIMESTAMP="${BDWIND_LOG_TIMESTAMP:-true}"
export BDWIND_LOG_CALLER="${BDWIND_LOG_CALLER:-false}"
export BDWIND_LOG_COLORS="${BDWIND_LOG_COLORS:-false}"

echo "🔧 日志环境变量:"
echo "   BDWIND_LOG_LEVEL=$BDWIND_LOG_LEVEL"
echo "   BDWIND_LOG_OUTPUT=$BDWIND_LOG_OUTPUT"
echo "   BDWIND_LOG_FILE=$BDWIND_LOG_FILE"

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
    else
        echo "⚠️  xeyes 未安装，屏幕上可能没有任何内容可捕获"
        echo "   请安装 xeyes: sudo apt-get install x11-apps"
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

# 检查关键日志节点的函数
check_critical_log_nodes() {
    local log_file="$BDWIND_LOG_FILE"
    local check_passed=0
    local check_total=0
    
    if [ ! -f "$log_file" ]; then
        echo "   ❌ 日志文件不存在: $log_file"
        return 1
    fi
    
    echo "   📊 检查关键Info级别日志节点:"
    
    # 检查1: GStreamer Manager 启动
    check_total=$((check_total + 1))
    if grep -q "component=gstreamer" "$log_file" 2>/dev/null; then
        if grep -q "INFO.*GStreamer manager started successfully.*component=gstreamer" "$log_file" 2>/dev/null; then
            echo "   ✅ GStreamer Manager 启动成功"
            check_passed=$((check_passed + 1))
        else
            echo "   ⚠️  GStreamer Manager 启动中..."
        fi
    else
        echo "   ❌ GStreamer Manager 未启动"
    fi
    
    # 检查2: WebRTC Manager 启动
    check_total=$((check_total + 1))
    if grep -q "component=webrtc-manager" "$log_file" 2>/dev/null; then
        if grep -q "INFO.*WebRTC manager started successfully.*component=webrtc-manager" "$log_file" 2>/dev/null; then
            echo "   ✅ WebRTC Manager 启动成功"
            check_passed=$((check_passed + 1))
        else
            echo "   ⚠️  WebRTC Manager 启动中..."
        fi
    else
        echo "   ❌ WebRTC Manager 未启动"
    fi
    
    # 检查3: MediaStream 创建
    check_total=$((check_total + 1))
    if grep -q "MediaStream creation result" "$log_file" 2>/dev/null; then
        echo "   ✅ MediaStream 创建成功"
        check_passed=$((check_passed + 1))
    else
        echo "   ❌ MediaStream 创建失败或未完成"
    fi
    
    # 检查4: GStreamer Bridge 创建
    check_total=$((check_total + 1))
    if grep -q "GStreamer bridge.*started successfully" "$log_file" 2>/dev/null; then
        echo "   ✅ GStreamer Bridge 创建成功"
        check_passed=$((check_passed + 1))
    else
        echo "   ❌ GStreamer Bridge 创建失败或未完成"
    fi
    
    # 检查5: 样本回调链路建立
    check_total=$((check_total + 1))
    if grep -q "Sample callback.*established" "$log_file" 2>/dev/null || grep -q "connectGStreamerToWebRTC.*successful" "$log_file" 2>/dev/null; then
        echo "   ✅ 样本回调链路建立成功"
        check_passed=$((check_passed + 1))
    else
        echo "   ❌ 样本回调链路未建立"
    fi
    
    # 检查6: WebServer 启动
    check_total=$((check_total + 1))
    if grep -q "Webserver started successfully.*component=webserver" "$log_file" 2>/dev/null; then
        echo "   ✅ WebServer 启动成功"
        check_passed=$((check_passed + 1))
    else
        echo "   ❌ WebServer 启动失败或未完成"
    fi
    
    # 检查7: 样本处理活动
    check_total=$((check_total + 1))
    if grep -q "BaseEncoder.*Received frame" "$log_file" 2>/dev/null; then
        echo "   ✅ 样本处理活动检测到"
        check_passed=$((check_passed + 1))
    else
        echo "   ⚠️  暂未检测到样本处理活动"
    fi
    
    # 检查8: 应用程序整体启动
    check_total=$((check_total + 1))
    if grep -q "BDWind-GStreamer application started successfully" "$log_file" 2>/dev/null; then
        echo "   ✅ 应用程序整体启动成功"
        check_passed=$((check_passed + 1))
    else
        echo "   ❌ 应用程序整体启动失败或未完成"
    fi
    
    # 显示检查结果摘要
    echo ""
    echo "   📋 检查结果摘要: $check_passed/$check_total 项通过"
    
    if [ $check_passed -eq $check_total ]; then
        echo "   🎉 所有关键节点检查通过！"
    elif [ $check_passed -gt $((check_total / 2)) ]; then
        echo "   ⚠️  部分关键节点存在问题，建议检查日志"
    else
        echo "   ❌ 多个关键节点存在问题，请检查配置和环境"
    fi
    
    # 提供故障排除建议
    if [ $check_passed -lt $check_total ]; then
        echo ""
        echo "   🔧 故障排除建议:"
        echo "      - 查看完整日志: cat $log_file"
        echo "      - 查看错误日志: grep -i error $log_file"
        echo "      - 查看警告日志: grep -i warn $log_file"
        echo "      - 检查组件状态: grep -E 'Starting|started|failed' $log_file"
    fi
}

# 提供日志分析工具的函数
show_log_analysis_tools() {
    local log_file="$BDWIND_LOG_FILE"
    
    echo "🔧 日志分析工具:"
    echo "   基础查看:"
    echo "     cat $log_file                    # 查看完整日志"
    echo "     tail -f $log_file                # 实时查看日志"
    echo "     head -n 50 $log_file             # 查看前50行"
    echo "     tail -n 50 $log_file             # 查看后50行"
    echo ""
    echo "   按级别过滤:"
    echo "     grep 'level=info' $log_file      # 查看Info级别日志"
    echo "     grep 'level=debug' $log_file     # 查看Debug级别日志"
    echo "     grep 'level=trace' $log_file     # 查看Trace级别日志"
    echo "     grep 'level=error' $log_file     # 查看Error级别日志"
    echo "     grep 'level=warn' $log_file      # 查看Warn级别日志"
    echo ""
    echo "   按组件过滤:"
    echo "     grep 'component=gstreamer' $log_file    # GStreamer相关日志"
    echo "     grep 'component=webrtc' $log_file       # WebRTC相关日志"
    echo "     grep 'component=webserver' $log_file    # WebServer相关日志"
    echo ""
    echo "   问题排查:"
    echo "     grep -i error $log_file          # 查找错误信息"
    echo "     grep -i warn $log_file           # 查找警告信息"
    echo "     grep -i fail $log_file           # 查找失败信息"
    echo "     grep -E 'Starting|started|failed' $log_file  # 查看启动状态"
    echo ""
    echo "   统计信息:"
    echo "     grep -c 'level=info' $log_file   # 统计Info日志数量"
    echo "     grep -c 'level=error' $log_file  # 统计Error日志数量"
    echo "     wc -l $log_file                  # 统计总行数"
}

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
        
        # 执行最终的日志分析
        if [ "$AUTO_CHECK" = "true" ]; then
            echo ""
            echo "🔍 最终日志分析:"
            check_critical_log_nodes
        fi
        
        echo ""
        show_log_analysis_tools
    fi
    
    echo "✅ 清理完成"
    exit 0
}

trap cleanup INT TERM

# 启动应用程序并获取PID
echo "🚀 启动应用程序..."
echo "📝 日志文件: $BDWIND_LOG_FILE"
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
    echo "   - 日志文件: .tmp/bdwind-gstreamer.log"
    echo ""
    echo "🔧 故障排除:"
    echo "   - 查看实时日志: tail -f $BDWIND_LOG_FILE"
    echo "   - 查看完整日志: cat $BDWIND_LOG_FILE"
    echo "   - 查看分析工具: 在脚本结束后会显示详细的日志分析命令"
    echo ""
    
    # 显示最近的日志
    if [ -f "$BDWIND_LOG_FILE" ]; then
        echo "📄 最近的日志输出:"
        tail -n 5 "$BDWIND_LOG_FILE" | sed 's/^/   /'
        echo ""
    fi
    
    # 执行关键日志节点自动检查
    if [ "$AUTO_CHECK" = "true" ]; then
        echo "🔍 执行关键日志节点自动检查..."
        check_critical_log_nodes
        echo ""
    fi
    
    # 等待应用程序结束
    echo "应用程序正在运行，按Ctrl+C停止..."
    wait $APP_PID
else
    echo "❌ 应用程序启动失败"
    cleanup
fi