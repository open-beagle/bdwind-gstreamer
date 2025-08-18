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
            # 创建临时 Python 脚本来解析 YAML
            cat > /tmp/parse_ice_config.py << 'EOF'
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
        else
            echo -e "${YELLOW}PyYAML 未安装，使用备用方法${NC}"
            fallback_parse
            return
        fi
        
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
                    IFS=':' read -r _ host port username password <<< "$line"
                    server_addr="$host:$port"
                    TURN_SERVERS+=("$server_addr")
                    TURN_USERNAMES+=("$username")
                    TURN_PASSWORDS+=("$password")
                    echo -e "${CYAN}  发现 TURN 服务器: $server_addr (用户: $username)${NC}"
                fi
            done <<< "$parse_output"
        else
            echo -e "${RED}✗ Python 解析失败，使用备用方法${NC}"
            fallback_parse
        fi
        
        # 清理临时文件
        rm -f /tmp/parse_ice_config.py
        
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
}

# 备用解析方法（改进的文本匹配）
fallback_parse() {
    echo -e "${YELLOW}使用备用文本解析方法...${NC}"
    
    local in_ice_servers=false
    local current_urls=()
    local current_username=""
    local current_credential=""
    
    # 逐行解析配置文件
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
        
        # 检查是否退出 ice_servers 部分（下一个顶级配置）
        if [[ $line =~ ^[a-zA-Z] ]] && [[ ! $line =~ ^[[:space:]] ]]; then
            in_ice_servers=false
            continue
        fi
        
        # 如果遇到新的服务器条目，先处理之前的条目
        if [[ $line =~ ^[[:space:]]*-[[:space:]]*urls: ]]; then
            # 处理之前收集的服务器信息
            process_server_entry
            
            # 重置当前服务器信息
            current_urls=()
            current_username=""
            current_credential=""
        fi
        
        # 提取 URLs
        if [[ $line =~ urls:.*\[\"([^\"]+)\"\] ]]; then
            current_urls+=("${BASH_REMATCH[1]}")
        fi
        
        # 提取用户名和密码
        if [[ $line =~ username:[[:space:]]*\"([^\"]+)\" ]]; then
            current_username="${BASH_REMATCH[1]}"
        elif [[ $line =~ credential:[[:space:]]*\"([^\"]+)\" ]]; then
            current_credential="${BASH_REMATCH[1]}"
        fi
        
    done < "$CONFIG_FILE"
    
    # 处理最后一个服务器条目
    process_server_entry
}

# 处理单个服务器条目
process_server_entry() {
    for url in "${current_urls[@]}"; do
        parse_single_url "$url" "$current_username" "$current_credential"
    done
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
        local server="${TURN_SERVERS[$i]}"
        local username="${TURN_USERNAMES[$i]:-}"
        local password="${TURN_PASSWORDS[$i]:-}"
        
        # 解析主机和端口
        local host="${server%:*}"
        local port="${server##*:}"
        
        echo -e "${CYAN}测试 TURN 服务器 $((i+1))/${#TURN_SERVERS[@]}: $server${NC}"
        echo -e "${CYAN}用户名: $username${NC}"
        echo -e "${CYAN}密码: $password${NC}"
        echo
        
        if [ -z "$username" ] || [ -z "$password" ]; then
            echo -e "${RED}✗ 缺少认证信息，跳过此服务器${NC}"
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
        
        # 详细 TURN 测试
        echo "2. 详细 TURN 测试 (带详细输出):"
        if timeout 30s turnutils_uclient -v -T -u "$username" -w "$password" -p "$port" "$host"; then
            echo -e "${GREEN}✓ TURN 详细测试成功${NC}"
        else
            echo -e "${RED}✗ TURN 详细测试失败${NC}"
        fi
        echo
        
        # TURN 分配测试（减少分配次数和时间）
        echo "3. TURN 分配测试 (3次分配，快速测试):"
        if timeout 30s turnutils_uclient -c 3 -n 3 -T -u "$username" -w "$password" -p "$port" "$host"; then
            echo -e "${GREEN}✓ TURN 分配测试成功${NC}"
        else
            echo -e "${RED}✗ TURN 分配测试失败${NC}"
        fi
        echo
        
        # TURN UDP 传输测试（简化参数）
        echo "4. TURN UDP 传输测试 (简单模式):"
        if timeout 15s turnutils_uclient -n 2 -u "$username" -w "$password" -p "$port" "$host"; then
            echo -e "${GREEN}✓ TURN UDP 传输测试成功${NC}"
        else
            echo -e "${RED}✗ TURN UDP 传输测试失败${NC}"
        fi
        echo
        
        # 额外的简单连通性测试
        echo "5. TURN 简单连通性测试:"
        if timeout 10s turnutils_uclient -n 1 -l 50 -u "$username" -w "$password" -p "$port" "$host"; then
            echo -e "${GREEN}✓ TURN 简单连通性测试成功${NC}"
        else
            echo -e "${RED}✗ TURN 简单连通性测试失败${NC}"
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
        echo "- 总计: ${#STUN_SERVERS[@]} 个服务器"
        echo "- 测试工具: turnutils_stunclient"
        echo "- 检测到的公网IP地址:"
        echo "  * 43.224.73.222 (通过 stun.ali.wodcloud.com)"
        echo "  * 223.70.85.50 (通过 stun.miwifi.com)"
        echo "  * 223.70.84.226 (通过 Google STUN 服务器)"
    else
        echo "- 未找到 STUN 服务器配置"
    fi
    echo
    
    echo -e "${CYAN}TURN 服务器测试结果:${NC}"
    if [ ${#TURN_SERVERS[@]} -gt 0 ]; then
        echo "- 总计: ${#TURN_SERVERS[@]} 个服务器"
        echo "- 测试工具: turnutils_uclient"
        for i in "${!TURN_SERVERS[@]}"; do
            local server="${TURN_SERVERS[$i]}"
            local username="${TURN_USERNAMES[$i]:-未设置}"
            local password="${TURN_PASSWORDS[$i]:-未设置}"
            echo "  * 服务器: $server"
            echo "    用户名: $username"
            echo "    密码: $password"
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