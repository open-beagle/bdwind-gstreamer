#!/bin/bash

# 设置虚拟桌面内容脚本
# 在 Xvfb 虚拟显示上创建测试内容

set -e

DISPLAY_ID=":99"

echo "🖥️  设置虚拟桌面内容..."

# 检查虚拟显示是否运行
if ! pgrep -f "Xvfb $DISPLAY_ID" > /dev/null; then
    echo "❌ 虚拟显示 $DISPLAY_ID 未运行"
    exit 1
fi

# 设置显示环境变量
export DISPLAY=$DISPLAY_ID

# 安装必要的工具（如果没有安装）
if ! command -v xterm >/dev/null 2>&1; then
    echo "📦 安装桌面测试工具..."
    sudo apt-get update -qq
    sudo apt-get install -y xterm x11-apps imagemagick
fi

# 启动窗口管理器（简单的）
if ! pgrep -f "twm" > /dev/null; then
    echo "🪟 启动窗口管理器..."
    twm &
    sleep 2
fi

# 创建一些测试窗口和内容
echo "🎨 创建测试内容..."

# 启动一个时钟显示
xclock -geometry 200x200+100+100 -update 1 &

# 启动一个终端显示系统信息
xterm -geometry 80x24+350+100 -title "System Info" -e "
while true; do
    clear
    echo '=== BDWind-GStreamer 测试环境 ==='
    echo
    echo '时间:' \$(date)
    echo 'CPU:' \$(nproc) 'cores'
    echo '内存:' \$(free -h | grep Mem | awk '{print \$2}')
    echo '负载:' \$(uptime | cut -d',' -f3-5)
    echo
    echo '=== GStreamer 测试中 ==='
    echo '显示: $DISPLAY_ID'
    echo '分辨率: 1920x1080'
    echo
    sleep 5
done
" &

# 创建一个彩色测试图案
xterm -geometry 60x20+600+300 -title "Color Test" -e "
while true; do
    for color in red green blue yellow magenta cyan white; do
        echo -e \"\033[1;31m████ \033[1;32m████ \033[1;34m████ \033[1;33m████\033[0m\"
        echo -e \"\033[1;35m████ \033[1;36m████ \033[1;37m████ \033[1;30m████\033[0m\"
        sleep 2
    done
done
" &

# 显示一些图形测试
if command -v xeyes >/dev/null 2>&1; then
    xeyes -geometry 100x100+800+100 &
fi

if command -v xcalc >/dev/null 2>&1; then
    xcalc -geometry 200x300+900+200 &
fi

echo "✅ 虚拟桌面内容设置完成"
echo "🔍 可以通过以下命令查看窗口："
echo "   DISPLAY=$DISPLAY_ID xwininfo -root -tree"
echo "📸 截图测试："
echo "   DISPLAY=$DISPLAY_ID import -window root test_screenshot.png"

# 等待一下让窗口完全加载
sleep 3

# 截图测试
echo "📸 创建测试截图..."
DISPLAY=$DISPLAY_ID import -window root /tmp/virtual_desktop_test.png
echo "✅ 测试截图已保存: /tmp/virtual_desktop_test.png"

echo "🎯 虚拟桌面已准备就绪，可以开始桌面捕获测试"