#!/bin/bash
# BDWind-GStreamer 停止脚本
# 用于清理由 start.sh 启动的所有进程，包括意外退出后残留的进程

echo "🛑 停止 BDWind-GStreamer 及相关进程"
echo "📅 停止时间: $(date)"
echo ""

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 计数器
KILLED_COUNT=0
FAILED_COUNT=0

# 停止进程的函数
kill_process() {
    local process_name=$1
    local pattern=$2
    local signal=${3:-TERM}
    
    echo -n "🔍 查找 $process_name 进程..."
    
    # 查找进程
    PIDS=$(pgrep -f "$pattern" 2>/dev/null)
    
    if [ -z "$PIDS" ]; then
        echo -e " ${YELLOW}未找到${NC}"
        return 0
    fi
    
    echo -e " ${GREEN}找到 $(echo "$PIDS" | wc -w) 个${NC}"
    
    # 显示进程信息
    echo "$PIDS" | while read pid; do
        if [ -n "$pid" ]; then
            CMD=$(ps -p $pid -o cmd= 2>/dev/null || echo "未知命令")
            echo "   PID $pid: ${CMD:0:80}"
        fi
    done
    
    # 发送信号
    echo -n "   发送 SIG$signal 信号..."
    echo "$PIDS" | while read pid; do
        if [ -n "$pid" ]; then
            kill -$signal $pid 2>/dev/null && KILLED_COUNT=$((KILLED_COUNT + 1)) || FAILED_COUNT=$((FAILED_COUNT + 1))
        fi
    done
    
    # 等待进程退出
    sleep 2
    
    # 检查是否还有残留
    REMAINING=$(pgrep -f "$pattern" 2>/dev/null | wc -l)
    if [ "$REMAINING" -gt 0 ]; then
        echo -e " ${YELLOW}还有 $REMAINING 个进程残留${NC}"
        
        # 如果是TERM信号失败，尝试KILL
        if [ "$signal" = "TERM" ]; then
            echo "   尝试使用 SIGKILL 强制终止..."
            pkill -9 -f "$pattern" 2>/dev/null
            sleep 1
            
            REMAINING=$(pgrep -f "$pattern" 2>/dev/null | wc -l)
            if [ "$REMAINING" -eq 0 ]; then
                echo -e "   ${GREEN}✅ 强制终止成功${NC}"
            else
                echo -e "   ${RED}❌ 仍有 $REMAINING 个进程无法终止${NC}"
            fi
        fi
    else
        echo -e " ${GREEN}✅ 已停止${NC}"
    fi
}

# 停止主程序
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "📦 停止主程序"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
kill_process "bdwind-gstreamer" "bdwind-gstreamer" "TERM"
kill_process "bdwind-gstreamer (二进制)" ".tmp/bdwind-gstreamer" "TERM"

# 停止GStreamer相关进程
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "🎬 停止GStreamer进程"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
kill_process "gst-launch" "gst-launch" "TERM"
kill_process "gstreamer" "gstreamer" "TERM"

# 停止测试窗口
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "🖼️  停止测试窗口"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
kill_process "xterm测试窗口" "xterm.*Test.*Content" "TERM"
kill_process "xterm动态窗口" "xterm.*Dynamic.*Content" "TERM"
kill_process "xterm动画窗口" "xterm.*Animated.*Content" "TERM"
kill_process "xterm静态窗口" "xterm.*Static.*Content" "TERM"
kill_process "xeyes" "xeyes" "TERM"

# 停止虚拟显示
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "🖥️  停止虚拟显示"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
kill_process "Xvfb :99" "Xvfb.*:99" "TERM"

# 清理其他可能的残留进程
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "🧹 清理其他残留进程"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

# 清理可能的日志监控进程
kill_process "tail日志监控" "tail.*bdwind-gstreamer.log" "TERM"

# 清理可能的WebSocket连接
kill_process "WebSocket连接" "ws://.*signaling" "TERM"

# 最终检查
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "🔍 最终检查"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

# 检查端口占用
echo "📊 检查端口占用..."
PORT_48080=$(ss -tlnp 2>/dev/null | grep :48080 | wc -l)
if [ "$PORT_48080" -gt 0 ]; then
    echo -e "   ${YELLOW}⚠️  端口 48080 仍被占用${NC}"
    ss -tlnp 2>/dev/null | grep :48080 | head -3
    
    # 尝试找到并终止占用端口的进程
    PORT_PID=$(ss -tlnp 2>/dev/null | grep :48080 | grep -oP 'pid=\K[0-9]+' | head -1)
    if [ -n "$PORT_PID" ]; then
        echo "   尝试终止占用端口的进程 (PID: $PORT_PID)..."
        kill -9 $PORT_PID 2>/dev/null && echo -e "   ${GREEN}✅ 已终止${NC}" || echo -e "   ${RED}❌ 终止失败${NC}"
    fi
else
    echo -e "   ${GREEN}✅ 端口 48080 已释放${NC}"
fi

# 检查虚拟显示
echo "🖥️  检查虚拟显示..."
if DISPLAY=:99 xdpyinfo >/dev/null 2>&1; then
    echo -e "   ${YELLOW}⚠️  虚拟显示 :99 仍在运行${NC}"
    # 强制终止所有X进程
    pkill -9 -f "Xvfb.*:99" 2>/dev/null
    sleep 1
    if DISPLAY=:99 xdpyinfo >/dev/null 2>&1; then
        echo -e "   ${RED}❌ 无法停止虚拟显示${NC}"
    else
        echo -e "   ${GREEN}✅ 虚拟显示已停止${NC}"
    fi
else
    echo -e "   ${GREEN}✅ 虚拟显示已停止${NC}"
fi

# 检查残留的bdwind进程
echo "🔍 检查残留的bdwind进程..."
BDWIND_PROCS=$(ps aux | grep -i bdwind | grep -v grep | grep -v "stop.sh" | wc -l)
if [ "$BDWIND_PROCS" -gt 0 ]; then
    echo -e "   ${YELLOW}⚠️  发现 $BDWIND_PROCS 个残留进程${NC}"
    ps aux | grep -i bdwind | grep -v grep | grep -v "stop.sh" | head -5
    
    echo "   尝试强制清理..."
    pkill -9 -f bdwind 2>/dev/null
    sleep 1
    
    BDWIND_PROCS=$(ps aux | grep -i bdwind | grep -v grep | grep -v "stop.sh" | wc -l)
    if [ "$BDWIND_PROCS" -eq 0 ]; then
        echo -e "   ${GREEN}✅ 残留进程已清理${NC}"
    else
        echo -e "   ${RED}❌ 仍有 $BDWIND_PROCS 个进程无法清理${NC}"
    fi
else
    echo -e "   ${GREEN}✅ 无残留进程${NC}"
fi

# 显示日志文件信息
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "📄 日志文件信息"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
if [ -f ".tmp/bdwind-gstreamer.log" ]; then
    LOG_SIZE=$(du -h .tmp/bdwind-gstreamer.log | cut -f1)
    LOG_LINES=$(wc -l < .tmp/bdwind-gstreamer.log)
    echo "   文件路径: .tmp/bdwind-gstreamer.log"
    echo "   文件大小: $LOG_SIZE"
    echo "   日志行数: $LOG_LINES"
    echo ""
    echo "   查看命令:"
    echo "   - 完整日志: cat .tmp/bdwind-gstreamer.log"
    echo "   - 最后50行: tail -50 .tmp/bdwind-gstreamer.log"
    echo "   - 错误日志: grep -i error .tmp/bdwind-gstreamer.log"
    echo ""
    echo -n "   是否删除日志文件? [y/N] "
    read -t 10 -n 1 DELETE_LOG
    echo ""
    if [ "$DELETE_LOG" = "y" ] || [ "$DELETE_LOG" = "Y" ]; then
        rm -f .tmp/bdwind-gstreamer.log
        echo -e "   ${GREEN}✅ 日志文件已删除${NC}"
    else
        echo "   日志文件已保留"
    fi
else
    echo "   无日志文件"
fi

# 总结
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "📊 清理总结"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "   已终止进程: $KILLED_COUNT"
echo "   终止失败: $FAILED_COUNT"
echo ""

# 最终状态检查
ALL_CLEAR=true

# 检查关键进程
if pgrep -f "bdwind-gstreamer" >/dev/null 2>&1; then
    echo -e "${RED}❌ bdwind-gstreamer 进程仍在运行${NC}"
    ALL_CLEAR=false
fi

if pgrep -f "Xvfb.*:99" >/dev/null 2>&1; then
    echo -e "${YELLOW}⚠️  Xvfb :99 进程仍在运行${NC}"
    ALL_CLEAR=false
fi

if ss -tlnp 2>/dev/null | grep :48080 >/dev/null; then
    echo -e "${YELLOW}⚠️  端口 48080 仍被占用${NC}"
    ALL_CLEAR=false
fi

if [ "$ALL_CLEAR" = true ]; then
    echo -e "${GREEN}✅ 所有服务已成功停止，系统已清理干净${NC}"
    echo ""
    echo "🚀 重新启动命令: ./scripts/start.sh"
    exit 0
else
    echo -e "${YELLOW}⚠️  部分服务可能未完全停止${NC}"
    echo ""
    echo "💡 如果问题持续，可以尝试:"
    echo "   1. 重启系统: sudo reboot"
    echo "   2. 手动检查进程: ps aux | grep bdwind"
    echo "   3. 手动终止进程: kill -9 <PID>"
    echo "   4. 检查端口占用: ss -tlnp | grep 48080"
    exit 1
fi
