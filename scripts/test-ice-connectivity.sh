#!/bin/bash

# ICE 服务器连通性测试脚本
# 使用 coturn 工具测试 examples/debug_config.yaml 中的 ICE 服务器

# set -e  # 注释掉，允许某些命令失败

# 脚本配置
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
CONFIG_FILE="$PROJECT_ROOT/examples/debug_config.yaml"

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

echo -e "${CYAN}==========================================${NC}"
echo -e "${CYAN}    ICE 服务器连通性测试 (使用 coturn)${NC}"
echo -e "${CYAN}==========================================${NC}"
echo

# 检查 coturn 工具是否安装
check_coturn_tools() {
    echo -e "${BLUE}检查 coturn 工具...${NC}"

    if ! command -v turnutils_uclient >/dev/null 2>&1; then
        echo -e "${RED}错误: turnutils_uclient 未找到${NC}"
        echo "请安装 coturn 工具包:"
        echo "  Ubuntu/Debian: sudo apt-get install coturn"
        echo "  CentOS/RHEL: sudo yum install coturn"
        echo "  macOS: brew install coturn"
        exit 1
    fi

    if ! command -v turnutils_stunclient >/dev/null 2>&1; then
        echo -e "${YELLOW}警告: turnutils_stunclient 未找到，将跳过 STUN 测试${NC}"
    fi

    echo -e "${GREEN}✓ coturn 工具检查完成${NC}"
    echo
}

# 全局变量存储解析的服务器信息
declare -a STUN_SERVERS
declare -a TURN_SERVERS
declare -a TURN_USERNAMES
declare -a TURN_PASSWORDS

# 配置验证和错误处理函数

# 检查全局数组的存在性和初始化状态
validate_global_arrays() {
    local validation_passed=true
    local error_messages=()

    echo -e "${BLUE}验证全局配置数组...${NC}"

    # 检查数组是否已声明
    if ! declare -p STUN_SERVERS >/dev/null 2>&1; then
        error_messages+=("STUN_SERVERS 数组未声明")
        validation_passed=false
    fi

    if ! declare -p TURN_SERVERS >/dev/null 2>&1; then
        error_messages+=("TURN_SERVERS 数组未声明")
        validation_passed=false
    fi

    if ! declare -p TURN_USERNAMES >/dev/null 2>&1; then
        error_messages+=("TURN_USERNAMES 数组未声明")
        validation_passed=false
    fi

    if ! declare -p TURN_PASSWORDS >/dev/null 2>&1; then
        error_messages+=("TURN_PASSWORDS 数组未声明")
        validation_passed=false
    fi

    # 如果有错误，显示并退出
    if [ "$validation_passed" = false ]; then
        echo -e "${RED}✗ 全局数组验证失败:${NC}"
        for msg in "${error_messages[@]}"; do
            echo -e "${RED}  - $msg${NC}"
        done
        return 1
    fi

    echo -e "${GREEN}✓ 全局数组验证通过${NC}"
    return 0
}

# 验证 TURN 配置的完整性
validate_turn_configuration() {
    local validation_passed=true
    local warning_messages=()
    local error_messages=()

    echo -e "${BLUE}验证 TURN 服务器配置完整性...${NC}"

    # 检查数组长度是否匹配
    local turn_servers_count=${#TURN_SERVERS[@]}
    local turn_usernames_count=${#TURN_USERNAMES[@]}
    local turn_passwords_count=${#TURN_PASSWORDS[@]}

    if [ $turn_servers_count -eq 0 ]; then
        echo -e "${YELLOW}⚠ 未找到 TURN 服务器配置${NC}"
        return 0
    fi

    # 检查数组长度不匹配的情况
    if [ $turn_servers_count -ne $turn_usernames_count ]; then
        error_messages+=("TURN 服务器数量 ($turn_servers_count) 与用户名数量 ($turn_usernames_count) 不匹配")
        validation_passed=false
    fi

    if [ $turn_servers_count -ne $turn_passwords_count ]; then
        error_messages+=("TURN 服务器数量 ($turn_servers_count) 与密码数量 ($turn_passwords_count) 不匹配")
        validation_passed=false
    fi

    # 如果数组长度不匹配，显示错误并返回
    if [ "$validation_passed" = false ]; then
        echo -e "${RED}✗ TURN 配置验证失败:${NC}"
        for msg in "${error_messages[@]}"; do
            echo -e "${RED}  - $msg${NC}"
        done
        return 1
    fi

    # 检查每个 TURN 服务器的配置完整性
    for i in "${!TURN_SERVERS[@]}"; do
        local server="${TURN_SERVERS[$i]:-}"
        local username="${TURN_USERNAMES[$i]:-}"
        local password="${TURN_PASSWORDS[$i]:-}"

        # 检查服务器地址
        if [ -z "$server" ]; then
            warning_messages+=("TURN 服务器 #$((i + 1)): 服务器地址为空")
        fi

        # 检查用户名
        if [ -z "$username" ]; then
            warning_messages+=("TURN 服务器 #$((i + 1)) ($server): 用户名为空")
        fi

        # 检查密码
        if [ -z "$password" ]; then
            warning_messages+=("TURN 服务器 #$((i + 1)) ($server): 密码为空")
        fi

        # 检查服务器地址格式
        if [ -n "$server" ] && [[ ! $server =~ ^[^:]+:[0-9]+$ ]]; then
            warning_messages+=("TURN 服务器 #$((i + 1)) ($server): 地址格式可能不正确 (期望格式: host:port)")
        fi
    done

    # 显示警告信息
    if [ ${#warning_messages[@]} -gt 0 ]; then
        echo -e "${YELLOW}⚠ TURN 配置警告:${NC}"
        for msg in "${warning_messages[@]}"; do
            echo -e "${YELLOW}  - $msg${NC}"
        done
    fi

    echo -e "${GREEN}✓ TURN 配置验证完成 (${#TURN_SERVERS[@]} 个服务器)${NC}"
    return 0
}

# 处理数组长度不匹配的情况
handle_array_length_mismatch() {
    local turn_servers_count=${#TURN_SERVERS[@]}
    local turn_usernames_count=${#TURN_USERNAMES[@]}
    local turn_passwords_count=${#TURN_PASSWORDS[@]}

    echo -e "${BLUE}处理数组长度不匹配...${NC}"

    # 找到最大长度
    local max_length=$turn_servers_count
    if [ $turn_usernames_count -gt $max_length ]; then
        max_length=$turn_usernames_count
    fi
    if [ $turn_passwords_count -gt $max_length ]; then
        max_length=$turn_passwords_count
    fi

    # 扩展数组到相同长度，用默认值填充
    for ((i = turn_servers_count; i < max_length; i++)); do
        TURN_SERVERS+=("未设置")
    done

    for ((i = turn_usernames_count; i < max_length; i++)); do
        TURN_USERNAMES+=("未设置")
    done

    for ((i = turn_passwords_count; i < max_length; i++)); do
        TURN_PASSWORDS+=("未设置")
    done

    echo -e "${GREEN}✓ 数组长度已统一为 $max_length${NC}"
}

# 提供配置状态摘要
provide_configuration_summary() {
    echo -e "${CYAN}配置状态摘要:${NC}"

    # STUN 服务器状态
    if [ ${#STUN_SERVERS[@]} -eq 0 ]; then
        echo -e "${YELLOW}  STUN 服务器: 未配置${NC}"
    else
        echo -e "${GREEN}  STUN 服务器: ${#STUN_SERVERS[@]} 个已配置${NC}"
    fi

    # TURN 服务器状态
    if [ ${#TURN_SERVERS[@]} -eq 0 ]; then
        echo -e "${YELLOW}  TURN 服务器: 未配置${NC}"
    else
        local complete_turn_configs=0
        for i in "${!TURN_SERVERS[@]}"; do
            local server="${TURN_SERVERS[$i]:-}"
            local username="${TURN_USERNAMES[$i]:-}"
            local password="${TURN_PASSWORDS[$i]:-}"

            if [ -n "$server" ] && [ -n "$username" ] && [ -n "$password" ] &&
                [ "$server" != "未设置" ] && [ "$username" != "未设置" ] && [ "$password" != "未设置" ]; then
                ((complete_turn_configs++))
            fi
        done

        if [ $complete_turn_configs -eq ${#TURN_SERVERS[@]} ]; then
            echo -e "${GREEN}  TURN 服务器: ${#TURN_SERVERS[@]} 个完整配置${NC}"
        else
            echo -e "${YELLOW}  TURN 服务器: ${#TURN_SERVERS[@]} 个配置 ($complete_turn_configs 个完整)${NC}"
        fi
    fi

    # 总体配置状态
    if [ ${#STUN_SERVERS[@]} -eq 0 ] && [ ${#TURN_SERVERS[@]} -eq 0 ]; then
        echo -e "${RED}  总体状态: 未找到任何 ICE 服务器配置${NC}"
        return 1
    else
        echo -e "${GREEN}  总体状态: 配置已加载${NC}"
        return 0
    fi
}

# 凭据掩码处理函数
# 用于安全显示密码信息，避免在输出中暴露完整凭据
mask_credential() {
    local credential="$1"

    # 检查输入是否为空
    if [ -z "$credential" ]; then
        echo "未设置"
        return
    fi

    # 获取凭据长度
    local length=${#credential}

    # 如果长度小于等于4，全部用星号掩码
    if [ "$length" -le 4 ]; then
        echo "****"
        return
    fi

    # 显示前2个字符，其余用星号掩码
    local visible_chars=2
    local masked_length=$((length - visible_chars))
    local visible_part="${credential:0:$visible_chars}"
    local masked_part=$(printf '%*s' "$masked_length" | tr ' ' '*')

    echo "${visible_part}${masked_part}"
}

# 解析 YAML 配置文件中的 ICE 服务器
parse_ice_servers() {
    echo -e "${BLUE}解析配置文件: $CONFIG_FILE${NC}"

    if [ ! -f "$CONFIG_FILE" ]; then
        echo -e "${RED}错误: 配置文件不存在: $CONFIG_FILE${NC}"
        exit 1
    fi

    # 清空数组
    STUN_SERVERS=()
    TURN_SERVERS=()
    TURN_USERNAMES=()
    TURN_PASSWORDS=()

    # 使用 python3 解析 YAML 配置
    if command -v python3 >/dev/null 2>&1; then
        echo -e "${GREEN}✓ 使用 python3 解析配置文件${NC}"

        # 检查是否有 PyYAML
        if python3 -c "import yaml" 2>/dev/null; then
            echo -e "${GREEN}✓ PyYAML 可用，使用 Python 解析${NC}"
            # 创建临时 Python 脚本来解析 YAML
            cat >/tmp/parse_ice_config.py <<'EOF'
import yaml
import sys
import re

def parse_url(url):
    """解析 ICE 服务器 URL，提取主机和端口"""
    # 移除协议前缀
    url_clean = re.sub(r'^(stun|turn|turns):', '', url)
    # 移除查询参数
    url_clean = re.sub(r'\?.*$', '', url_clean)
    
    if ':' in url_clean:
        host, port = url_clean.rsplit(':', 1)
        return host, port
    else:
        # 默认端口
        if url.startswith('stun'):
            return url_clean, '3478'
        elif url.startswith('turn'):
            return url_clean, '3478'
        else:
            return url_clean, '3478'

try:
    with open(sys.argv[1], 'r', encoding='utf-8') as f:
        config = yaml.safe_load(f)
    
    ice_servers = config.get('webrtc', {}).get('ice_servers', [])
    
    for server in ice_servers:
        urls = server.get('urls', [])
        username = server.get('username', '')
        credential = server.get('credential', '')
        
        for url in urls:
            host, port = parse_url(url)
            server_addr = f"{host}:{port}"
            
            if url.startswith('stun'):
                print(f"STUN:{server_addr}")
            elif url.startswith('turn'):
                print(f"TURN:{server_addr}:{username}:{credential}")

except Exception as e:
    print(f"ERROR: {e}", file=sys.stderr)
    sys.exit(1)
EOF
            # 运行 Python 脚本解析配置
            local parse_output
            if parse_output=$(python3 /tmp/parse_ice_config.py "$CONFIG_FILE" 2>/dev/null); then
                while IFS= read -r line; do
                    if [[ $line == STUN:* ]]; then
                        server_addr="${line#STUN:}"
                        STUN_SERVERS+=("$server_addr")
                        echo -e "${CYAN}  发现 STUN 服务器: $server_addr${NC}"
                    elif [[ $line == TURN:* ]]; then
                        # 格式: TURN:host:port:username:password
                        IFS=':' read -r _ host port username password <<<"$line"
                        server_addr="$host:$port"
                        TURN_SERVERS+=("$server_addr")
                        TURN_USERNAMES+=("$username")
                        TURN_PASSWORDS+=("$password")
                        echo -e "${CYAN}  发现 TURN 服务器: $server_addr (用户: $username)${NC}"
                    fi
                done <<<"$parse_output"
                echo -e "${GREEN}✓ Python 解析成功${NC}"
            else
                echo -e "${RED}✗ Python 解析失败，使用备用方法${NC}"
                fallback_parse
            fi

            # 清理临时文件
            rm -f /tmp/parse_ice_config.py
        else
            echo -e "${YELLOW}PyYAML 未安装，使用备用方法${NC}"
            fallback_parse
        fi

    elif command -v yq >/dev/null 2>&1; then
        echo -e "${GREEN}✓ 使用 yq 解析配置文件${NC}"

        # 使用 yq 解析（如果可用）
        local ice_servers_json
        if ice_servers_json=$(yq eval '.webrtc.ice_servers' "$CONFIG_FILE" -o json 2>/dev/null); then
            # 这里可以添加 yq 的解析逻辑
            echo -e "${YELLOW}yq 解析功能待实现，使用备用方法${NC}"
            fallback_parse
        else
            echo -e "${RED}✗ yq 解析失败，使用备用方法${NC}"
            fallback_parse
        fi
    else
        echo -e "${YELLOW}警告: 未找到 python3 或 yq，使用简单文本解析${NC}"
        fallback_parse
    fi

    echo -e "${GREEN}✓ 解析完成: ${#STUN_SERVERS[@]} 个 STUN 服务器, ${#TURN_SERVERS[@]} 个 TURN 服务器${NC}"
    echo

    # 执行配置验证
    echo -e "${BLUE}========================================${NC}"
    echo -e "${BLUE}配置验证${NC}"
    echo -e "${BLUE}========================================${NC}"

    # 验证全局数组
    if ! validate_global_arrays; then
        echo -e "${RED}配置验证失败，退出测试${NC}"
        exit 1
    fi

    # 验证 TURN 配置完整性
    if ! validate_turn_configuration; then
        echo -e "${RED}TURN 配置验证失败${NC}"
        echo -e "${YELLOW}尝试处理数组长度不匹配问题...${NC}"
        handle_array_length_mismatch

        # 重新验证
        if ! validate_turn_configuration; then
            echo -e "${RED}TURN 配置仍然无效，但将继续测试${NC}"
        fi
    fi

    # 提供配置状态摘要
    echo
    if ! provide_configuration_summary; then
        echo -e "${YELLOW}警告: 未找到有效的 ICE 服务器配置，测试可能无法进行${NC}"
    fi
    echo
}

# 备用解析方法（简化且准确的文本匹配）
fallback_parse() {
    echo -e "${YELLOW}使用备用文本解析方法...${NC}"

    # 直接从配置文件中提取所有ICE服务器URL
    local in_ice_servers=false
    local current_username=""
    local current_credential=""
    local server_block=""

    while IFS= read -r line; do
        # 检查是否进入 ice_servers 部分
        if [[ $line =~ ice_servers: ]]; then
            in_ice_servers=true
            continue
        fi

        # 如果不在 ice_servers 部分，跳过
        if [[ $in_ice_servers == false ]]; then
            continue
        fi

        # 检查是否退出 ice_servers 部分
        if [[ $line =~ ^[a-zA-Z] ]] && [[ ! $line =~ ^[[:space:]] ]]; then
            in_ice_servers=false
            break
        fi

        # 收集服务器块信息
        if [[ $line =~ ^[[:space:]]*-[[:space:]]*urls: ]]; then
            # 处理之前的服务器块
            if [[ -n "$server_block" ]]; then
                process_server_block "$server_block" "$current_username" "$current_credential"
            fi

            # 开始新的服务器块
            server_block="$line"
            current_username=""
            current_credential=""
        elif [[ $line =~ ^[[:space:]]+username: ]] || [[ $line =~ ^[[:space:]]+credential: ]] || [[ $line =~ ^[[:space:]]*# ]]; then
            # 添加到当前服务器块
            server_block="$server_block"$'\n'"$line"
        fi

        # 提取用户名和密码
        if [[ $line =~ username:[[:space:]]*\"([^\"]+)\" ]]; then
            current_username="${BASH_REMATCH[1]}"
        elif [[ $line =~ credential:[[:space:]]*\"([^\"]+)\" ]]; then
            current_credential="${BASH_REMATCH[1]}"
        fi

    done <"$CONFIG_FILE"

    # 处理最后一个服务器块
    if [[ -n "$server_block" ]]; then
        process_server_block "$server_block" "$current_username" "$current_credential"
    fi
}

# 处理单个服务器块
process_server_block() {
    local block="$1"
    local username="$2"
    local credential="$3"

    # 从服务器块中提取URL
    if [[ $block =~ urls:[[:space:]]*\[\"([^\"]+)\"\] ]]; then
        local url="${BASH_REMATCH[1]}"
        parse_single_url "$url" "$username" "$credential"
    fi
}

# 解析单个URL
parse_single_url() {
    local url="$1"
    local username="$2"
    local credential="$3"

    # 移除查询参数
    local clean_url="${url%\?*}"

    # 提取主机和端口
    local host_port="${clean_url#*:}"

    # 添加默认端口如果没有指定
    if [[ ! $host_port =~ : ]]; then
        host_port="$host_port:3478"
    fi

    if [[ $url =~ ^stun ]]; then
        STUN_SERVERS+=("$host_port")
        echo -e "${CYAN}  发现 STUN 服务器: $host_port${NC}"
    elif [[ $url =~ ^turn ]]; then
        TURN_SERVERS+=("$host_port")
        TURN_USERNAMES+=("$username")
        TURN_PASSWORDS+=("$credential")
        echo -e "${CYAN}  发现 TURN 服务器: $host_port (用户: $username)${NC}"
    fi
}

# 测试 STUN 服务器
test_stun_servers() {
    echo -e "${BLUE}========================================${NC}"
    echo -e "${BLUE}测试 STUN 服务器${NC}"
    echo -e "${BLUE}========================================${NC}"

    if [ ${#STUN_SERVERS[@]} -eq 0 ]; then
        echo -e "${YELLOW}未找到 STUN 服务器配置${NC}"
        echo
        return
    fi

    local success_count=0
    local total_count=${#STUN_SERVERS[@]}

    for server in "${STUN_SERVERS[@]}"; do
        echo -e "${CYAN}测试 STUN 服务器: $server${NC}"

        # 解析主机和端口
        local host="${server%:*}"
        local port="${server##*:}"

        if command -v turnutils_stunclient >/dev/null 2>&1; then
            # turnutils_stunclient 需要分开传递主机和端口
            if timeout 10s turnutils_stunclient -p "$port" "$host"; then
                echo -e "${GREEN}✓ STUN 服务器 $server 测试成功${NC}"
                ((success_count++))
            else
                echo -e "${RED}✗ STUN 服务器 $server 测试失败${NC}"
            fi
        else
            # 使用 turnutils_uclient 作为备选
            if timeout 10s turnutils_uclient -S -p "$port" "$host"; then
                echo -e "${GREEN}✓ STUN 服务器 $server 测试成功${NC}"
                ((success_count++))
            else
                echo -e "${RED}✗ STUN 服务器 $server 测试失败${NC}"
            fi
        fi
        echo
    done

    echo -e "${BLUE}STUN 测试摘要: $success_count/$total_count 成功${NC}"
    echo
}

# 测试 TURN 服务器
test_turn_servers() {
    echo -e "${BLUE}========================================${NC}"
    echo -e "${BLUE}测试 TURN 服务器${NC}"
    echo -e "${BLUE}========================================${NC}"

    if [ ${#TURN_SERVERS[@]} -eq 0 ]; then
        echo -e "${YELLOW}未找到 TURN 服务器配置${NC}"
        echo
        return
    fi

    # 测试每个 TURN 服务器
    for i in "${!TURN_SERVERS[@]}"; do
        local server="${TURN_SERVERS[$i]:-未设置}"
        local username="${TURN_USERNAMES[$i]:-未设置}"
        local password="${TURN_PASSWORDS[$i]:-未设置}"

        # 跳过无效的服务器配置
        if [ "$server" = "未设置" ] || [ -z "$server" ]; then
            echo -e "${YELLOW}⚠ 跳过 TURN 服务器 #$((i + 1)): 服务器地址未设置${NC}"
            echo
            continue
        fi

        # 解析主机和端口
        local host="${server%:*}"
        local port="${server##*:}"

        echo -e "${CYAN}测试 TURN 服务器 $((i + 1))/${#TURN_SERVERS[@]}: $server${NC}"
        echo -e "${CYAN}用户名: $username${NC}"
        echo -e "${CYAN}密码: $password${NC}"
        echo

        if [ -z "$username" ] || [ -z "$password" ] || [ "$username" = "未设置" ] || [ "$password" = "未设置" ]; then
            echo -e "${RED}✗ 缺少认证信息，跳过此服务器${NC}"
            echo -e "${YELLOW}  用户名: ${username:-未设置}${NC}"
            echo -e "${YELLOW}  密码: ${password:-未设置}${NC}"
            echo
            continue
        fi

        # 使用 turnutils_uclient 测试 TURN 服务器
        echo -e "${BLUE}运行 TURN 连通性测试...${NC}"

        # 基本 TURN 测试
        echo "1. 基本 TURN 测试:"
        if timeout 30s turnutils_uclient -T -u "$username" -w "$password" -p "$port" "$host"; then
            echo -e "${GREEN}✓ TURN 基本测试成功${NC}"
        else
            echo -e "${RED}✗ TURN 基本测试失败${NC}"
        fi
        echo

        # TURN 分配测试（使用正确的参数）
        echo "3. TURN 分配测试 (UDP模式):"
        if timeout 20s turnutils_uclient -v -n 3 -u "$username" -w "$password" -p "$port" -y "$host"; then
            echo -e "${GREEN}✓ TURN UDP 分配测试成功${NC}"
        else
            echo -e "${RED}✗ TURN UDP 分配测试失败${NC}"
        fi
        echo

        echo -e "${BLUE}----------------------------------------${NC}"
        echo
    done
}

# 生成详细的测试摘要
generate_test_summary() {
    echo -e "${BLUE}========================================${NC}"
    echo -e "${BLUE}测试摘要${NC}"
    echo -e "${BLUE}========================================${NC}"

    echo -e "${CYAN}STUN 服务器测试结果:${NC}"
    if [ ${#STUN_SERVERS[@]} -gt 0 ]; then
        echo -e "${BLUE}- 总计: ${#STUN_SERVERS[@]} 个服务器${NC}"
        echo -e "${BLUE}- 测试工具: turnutils_stunclient${NC}"
        echo -e "${BLUE}- 服务器列表:${NC}"
        for server in "${STUN_SERVERS[@]}"; do
            echo "  * $server"
        done
    else
        echo "- 未找到 STUN 服务器配置"
    fi
    echo

    echo -e "${CYAN}TURN 服务器测试结果:${NC}"
    if [ ${#TURN_SERVERS[@]} -gt 0 ]; then
        echo -e "${BLUE}- 总计: ${#TURN_SERVERS[@]} 个服务器${NC}"
        echo -e "${BLUE}- 测试工具: turnutils_uclient${NC}"
        echo -e "${BLUE}- 服务器列表:${NC}"
        for i in "${!TURN_SERVERS[@]}"; do
            local server="${TURN_SERVERS[$i]:-未设置}"
            local username="${TURN_USERNAMES[$i]:-未设置}"
            local password="${TURN_PASSWORDS[$i]:-未设置}"
            local masked_password
            local config_status=""

            # 使用凭据掩码功能显示密码信息
            if [ "$password" != "未设置" ] && [ -n "$password" ]; then
                masked_password=$(mask_credential "$password")
            else
                masked_password="未设置"
            fi

            # 检查配置完整性并添加状态标记
            if [ "$server" = "未设置" ] || [ -z "$server" ]; then
                config_status=" ${RED}[服务器地址缺失]${NC}"
            elif [ "$username" = "未设置" ] || [ -z "$username" ]; then
                config_status=" ${YELLOW}[用户名缺失]${NC}"
            elif [ "$password" = "未设置" ] || [ -z "$password" ]; then
                config_status=" ${YELLOW}[密码缺失]${NC}"
            else
                config_status=" ${GREEN}[配置完整]${NC}"
            fi

            echo -e "  * 服务器: $server$config_status"
            echo "    用户名: $username"
            echo "    密码: $masked_password"
        done
    else
        echo "- 未找到 TURN 服务器配置"
    fi
    echo
}

# 生成测试报告
generate_report() {
    echo -e "${CYAN}========================================${NC}"
    echo -e "${CYAN}测试报告和建议${NC}"
    echo -e "${CYAN}========================================${NC}"

    echo -e "${BLUE}测试环境信息:${NC}"
    echo "- 操作系统: $(uname -s)"
    echo "- 内核版本: $(uname -r)"
    echo "- 网络接口: $(ip addr show | grep -E "inet.*scope global" | head -2 | awk '{print $2}' | tr '\n' ' ')"
    echo "- DNS 服务器: $(cat /etc/resolv.conf | grep nameserver | head -1 | awk '{print $2}')"
    echo

    echo -e "${BLUE}coturn 工具版本:${NC}"
    turnutils_uclient --help | head -1 || echo "无法获取版本信息"
    echo

    echo -e "${BLUE}建议:${NC}"
    echo "1. 如果 coturn 工具测试成功但我们的工具失败，可能是实现差异"
    echo "2. 如果两者都失败，可能是服务器配置或网络问题"
    echo "3. 检查防火墙设置，确保 UDP 3478 端口开放"
    echo "4. 验证 TURN 服务器的用户名和密码是否正确"
    echo "5. 考虑网络环境的 NAT 类型和限制"
    echo
}

# 主函数
main() {
    check_coturn_tools
    parse_ice_servers
    test_stun_servers
    test_turn_servers
    generate_test_summary
    generate_report

    echo -e "${CYAN}========================================${NC}"
    echo -e "${CYAN}测试完成${NC}"
    echo -e "${CYAN}========================================${NC}"
}

# 运行主函数
main "$@"
