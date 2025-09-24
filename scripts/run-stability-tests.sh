#!/bin/bash

# GStreamer Go-GST 长时间稳定性测试运行脚本
# 用于执行24小时以上的稳定性测试

set -e

# 默认配置
DEFAULT_DURATION="24h"
DEFAULT_LOG_LEVEL="info"
DEFAULT_OUTPUT_DIR="./stability-test-results"
DEFAULT_TEST_PATTERN="TestLongRunningStability.*|TestContinuousMemoryStability.*|TestHighLoadStability.*"

# 解析命令行参数
DURATION="${STABILITY_TEST_DURATION:-$DEFAULT_DURATION}"
LOG_LEVEL="${LOG_LEVEL:-$DEFAULT_LOG_LEVEL}"
OUTPUT_DIR="${OUTPUT_DIR:-$DEFAULT_OUTPUT_DIR}"
TEST_PATTERN="${TEST_PATTERN:-$DEFAULT_TEST_PATTERN}"
ENABLE_PROFILING="${ENABLE_PROFILING:-false}"
ENABLE_RACE_DETECTION="${ENABLE_RACE_DETECTION:-false}"
PARALLEL_TESTS="${PARALLEL_TESTS:-1}"

# 显示帮助信息
show_help() {
    cat << EOF
GStreamer Go-GST 稳定性测试运行器

用法: $0 [选项]

选项:
    -d, --duration DURATION     测试持续时间 (默认: $DEFAULT_DURATION)
    -l, --log-level LEVEL       日志级别 (默认: $DEFAULT_LOG_LEVEL)
    -o, --output-dir DIR        输出目录 (默认: $DEFAULT_OUTPUT_DIR)
    -p, --pattern PATTERN       测试模式 (默认: $DEFAULT_TEST_PATTERN)
    --enable-profiling          启用性能分析
    --enable-race-detection     启用竞态检测
    --parallel N                并行测试数量 (默认: $PARALLEL_TESTS)
    -h, --help                  显示此帮助信息

环境变量:
    STABILITY_TEST_DURATION     测试持续时间
    LOG_LEVEL                   日志级别
    OUTPUT_DIR                  输出目录
    ENABLE_LONG_RUNNING_TESTS   启用长时间测试 (true/false)
    ENABLE_PROFILING            启用性能分析 (true/false)
    ENABLE_RACE_DETECTION       启用竞态检测 (true/false)

示例:
    # 运行24小时稳定性测试
    $0 --duration 24h

    # 运行1小时快速测试
    $0 --duration 1h --log-level debug

    # 启用性能分析的测试
    $0 --duration 2h --enable-profiling

    # 并行运行多个测试
    $0 --duration 30m --parallel 3
EOF
}

# 解析命令行参数
while [[ $# -gt 0 ]]; do
    case $1 in
        -d|--duration)
            DURATION="$2"
            shift 2
            ;;
        -l|--log-level)
            LOG_LEVEL="$2"
            shift 2
            ;;
        -o|--output-dir)
            OUTPUT_DIR="$2"
            shift 2
            ;;
        -p|--pattern)
            TEST_PATTERN="$2"
            shift 2
            ;;
        --enable-profiling)
            ENABLE_PROFILING="true"
            shift
            ;;
        --enable-race-detection)
            ENABLE_RACE_DETECTION="true"
            shift
            ;;
        --parallel)
            PARALLEL_TESTS="$2"
            shift 2
            ;;
        -h|--help)
            show_help
            exit 0
            ;;
        *)
            echo "未知选项: $1"
            show_help
            exit 1
            ;;
    esac
done

# 验证参数
if ! [[ "$PARALLEL_TESTS" =~ ^[0-9]+$ ]] || [ "$PARALLEL_TESTS" -lt 1 ]; then
    echo "错误: 并行测试数量必须是正整数"
    exit 1
fi

# 创建输出目录
mkdir -p "$OUTPUT_DIR"

# 设置日志文件
TIMESTAMP=$(date +"%Y%m%d_%H%M%S")
LOG_FILE="$OUTPUT_DIR/stability_test_$TIMESTAMP.log"
RESULTS_FILE="$OUTPUT_DIR/stability_results_$TIMESTAMP.json"
PROFILE_DIR="$OUTPUT_DIR/profiles_$TIMESTAMP"

echo "=== GStreamer Go-GST 稳定性测试 ==="
echo "测试持续时间: $DURATION"
echo "日志级别: $LOG_LEVEL"
echo "输出目录: $OUTPUT_DIR"
echo "测试模式: $TEST_PATTERN"
echo "并行测试: $PARALLEL_TESTS"
echo "性能分析: $ENABLE_PROFILING"
echo "竞态检测: $ENABLE_RACE_DETECTION"
echo "日志文件: $LOG_FILE"
echo "结果文件: $RESULTS_FILE"
echo "================================="

# 设置环境变量
export STABILITY_TEST_DURATION="$DURATION"
export LOG_LEVEL="$LOG_LEVEL"
export ENABLE_LONG_RUNNING_TESTS="true"

# 构建测试命令
TEST_CMD="go test"

# 添加竞态检测
if [ "$ENABLE_RACE_DETECTION" = "true" ]; then
    TEST_CMD="$TEST_CMD -race"
    echo "启用竞态检测"
fi

# 添加性能分析
if [ "$ENABLE_PROFILING" = "true" ]; then
    mkdir -p "$PROFILE_DIR"
    TEST_CMD="$TEST_CMD -cpuprofile=$PROFILE_DIR/cpu.prof -memprofile=$PROFILE_DIR/mem.prof"
    echo "启用性能分析，输出到: $PROFILE_DIR"
fi

# 添加其他测试参数
TEST_CMD="$TEST_CMD -v -timeout=${DURATION} -parallel=$PARALLEL_TESTS"
TEST_CMD="$TEST_CMD -run=\"$TEST_PATTERN\""
TEST_CMD="$TEST_CMD ./internal/gstreamer"

echo "执行命令: $TEST_CMD"
echo "开始时间: $(date)"

# 创建结果记录函数
record_result() {
    local test_name="$1"
    local status="$2"
    local duration="$3"
    local error_msg="$4"
    
    cat >> "$RESULTS_FILE" << EOF
{
    "test_name": "$test_name",
    "status": "$status",
    "duration": "$duration",
    "timestamp": "$(date -Iseconds)",
    "error_message": "$error_msg"
}
EOF
}

# 监控系统资源
monitor_resources() {
    local monitor_log="$OUTPUT_DIR/resource_monitor_$TIMESTAMP.log"
    echo "启动资源监控，输出到: $monitor_log"
    
    while true; do
        {
            echo "=== $(date) ==="
            echo "内存使用:"
            free -h
            echo "CPU使用:"
            top -bn1 | head -20
            echo "磁盘使用:"
            df -h
            echo "网络连接:"
            ss -tuln | head -10
            echo ""
        } >> "$monitor_log"
        
        sleep 60  # 每分钟记录一次
    done &
    
    MONITOR_PID=$!
    echo "资源监控进程 PID: $MONITOR_PID"
}

# 清理函数
cleanup() {
    echo "清理资源..."
    
    # 停止资源监控
    if [ ! -z "$MONITOR_PID" ]; then
        kill $MONITOR_PID 2>/dev/null || true
    fi
    
    # 生成最终报告
    generate_final_report
    
    echo "清理完成"
}

# 生成最终报告
generate_final_report() {
    local report_file="$OUTPUT_DIR/final_report_$TIMESTAMP.md"
    
    cat > "$report_file" << EOF
# GStreamer Go-GST 稳定性测试报告

## 测试配置
- 测试持续时间: $DURATION
- 日志级别: $LOG_LEVEL
- 测试模式: $TEST_PATTERN
- 并行测试数: $PARALLEL_TESTS
- 性能分析: $ENABLE_PROFILING
- 竞态检测: $ENABLE_RACE_DETECTION

## 测试时间
- 开始时间: $START_TIME
- 结束时间: $(date)
- 实际持续时间: $(($(date +%s) - START_TIMESTAMP)) 秒

## 文件位置
- 日志文件: $LOG_FILE
- 结果文件: $RESULTS_FILE
- 资源监控: $OUTPUT_DIR/resource_monitor_$TIMESTAMP.log
EOF

    if [ "$ENABLE_PROFILING" = "true" ]; then
        cat >> "$report_file" << EOF
- 性能分析: $PROFILE_DIR/
EOF
    fi

    cat >> "$report_file" << EOF

## 测试结果摘要
EOF

    # 分析测试结果
    if [ -f "$RESULTS_FILE" ]; then
        echo "- 详细结果请查看: $RESULTS_FILE" >> "$report_file"
    fi
    
    # 分析日志中的关键信息
    if [ -f "$LOG_FILE" ]; then
        echo "" >> "$report_file"
        echo "## 关键日志摘要" >> "$report_file"
        
        # 提取错误和警告
        local errors=$(grep -c "ERROR\|FATAL" "$LOG_FILE" 2>/dev/null || echo "0")
        local warnings=$(grep -c "WARN" "$LOG_FILE" 2>/dev/null || echo "0")
        
        echo "- 错误数量: $errors" >> "$report_file"
        echo "- 警告数量: $warnings" >> "$report_file"
        
        # 提取内存相关信息
        if grep -q "Memory" "$LOG_FILE" 2>/dev/null; then
            echo "" >> "$report_file"
            echo "### 内存相关日志" >> "$report_file"
            grep "Memory\|memory\|leak\|GC" "$LOG_FILE" | tail -20 >> "$report_file" 2>/dev/null || true
        fi
    fi
    
    echo "最终报告生成: $report_file"
}

# 设置信号处理
trap cleanup EXIT INT TERM

# 记录开始时间
START_TIME=$(date)
START_TIMESTAMP=$(date +%s)

# 启动资源监控
monitor_resources

# 执行测试
echo "开始执行稳定性测试..."
if eval "$TEST_CMD" 2>&1 | tee "$LOG_FILE"; then
    echo "稳定性测试成功完成"
    record_result "stability_test_suite" "PASS" "$(($(date +%s) - START_TIMESTAMP))" ""
    exit 0
else
    echo "稳定性测试失败"
    record_result "stability_test_suite" "FAIL" "$(($(date +%s) - START_TIMESTAMP))" "Test execution failed"
    exit 1
fi