#!/bin/bash

# 测试真实桌面捕获的脚本
# 使用 GStreamer 直接从虚拟显示捕获内容

set -e

DISPLAY_ID=":99"
OUTPUT_FILE="/tmp/real_capture_test.mp4"

echo "🎥 测试真实桌面捕获..."

# 检查虚拟显示是否运行
if ! pgrep -f "Xvfb $DISPLAY_ID" > /dev/null; then
    echo "❌ 虚拟显示 $DISPLAY_ID 未运行"
    exit 1
fi

# 检查 GStreamer 是否安装
if ! command -v gst-launch-1.0 >/dev/null 2>&1; then
    echo "❌ GStreamer 未安装"
    exit 1
fi

echo "🖥️  从显示 $DISPLAY_ID 捕获视频..."

# 使用 GStreamer 从 X11 显示捕获视频
DISPLAY=$DISPLAY_ID gst-launch-1.0 \
    ximagesrc display-name=$DISPLAY_ID use-damage=false \
    ! video/x-raw,framerate=30/1 \
    ! videoconvert \
    ! x264enc tune=zerolatency bitrate=2000 speed-preset=ultrafast \
    ! mp4mux \
    ! filesink location="$OUTPUT_FILE" &

CAPTURE_PID=$!

echo "✅ 开始捕获 (PID: $CAPTURE_PID)"
echo "⏱️  捕获10秒钟..."

sleep 10

echo "🛑 停止捕获..."
kill $CAPTURE_PID 2>/dev/null || true
wait $CAPTURE_PID 2>/dev/null || true

if [ -f "$OUTPUT_FILE" ]; then
    FILE_SIZE=$(stat -c%s "$OUTPUT_FILE")
    echo "✅ 捕获完成: $OUTPUT_FILE"
    echo "📁 文件大小: $FILE_SIZE bytes"
    
    if [ $FILE_SIZE -gt 1000 ]; then
        echo "🎉 捕获成功！文件大小正常"
        
        # 尝试获取视频信息
        if command -v ffprobe >/dev/null 2>&1; then
            echo "📊 视频信息:"
            ffprobe -v quiet -print_format json -show_format -show_streams "$OUTPUT_FILE" | jq -r '.streams[0] | "分辨率: \(.width)x\(.height), 帧率: \(.r_frame_rate), 编码: \(.codec_name)"' 2>/dev/null || echo "无法获取详细信息"
        fi
    else
        echo "⚠️  文件太小，可能捕获失败"
    fi
else
    echo "❌ 捕获失败，未生成文件"
fi

echo "🔍 测试完成"