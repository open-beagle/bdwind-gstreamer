#!/bin/bash

# BDWind-GStreamer Docker Entrypoint Script
# This script sets up the virtual display environment and starts the application

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Configuration
DISPLAY_NUM=${DISPLAY_NUM:-99}
DISPLAY_RESOLUTION=${DISPLAY_RESOLUTION:-1920x1080x24}
XVFB_ARGS=${XVFB_ARGS:-"-screen 0 ${DISPLAY_RESOLUTION} -ac -nolisten tcp -dpi 96 +extension GLX"}

# Set DISPLAY environment variable
export DISPLAY=:${DISPLAY_NUM}

# Function to start Xvfb
start_xvfb() {
    log_info "Starting Xvfb virtual display server..."
    log_info "Display: ${DISPLAY}"
    log_info "Resolution: ${DISPLAY_RESOLUTION}"
    
    # Start Xvfb in background
    Xvfb :${DISPLAY_NUM} ${XVFB_ARGS} &
    XVFB_PID=$!
    
    # Wait for Xvfb to start
    local timeout=10
    local counter=0
    
    while [ $counter -lt $timeout ]; do
        if xdpyinfo -display ${DISPLAY} >/dev/null 2>&1; then
            log_success "Xvfb started successfully on display ${DISPLAY}"
            return 0
        fi
        
        sleep 1
        counter=$((counter + 1))
    done
    
    log_error "Failed to start Xvfb within ${timeout} seconds"
    return 1
}

# Function to setup virtual desktop environment
setup_virtual_desktop() {
    log_info "Setting up virtual desktop environment..."
    
    # Create runtime directory
    mkdir -p ${XDG_RUNTIME_DIR:-/tmp/runtime-bdwind}
    
    # Set up window manager (optional, for testing)
    if command -v openbox >/dev/null 2>&1; then
        log_info "Starting Openbox window manager..."
        openbox &
    elif command -v fluxbox >/dev/null 2>&1; then
        log_info "Starting Fluxbox window manager..."
        fluxbox &
    else
        log_warning "No window manager found, running without one"
    fi
    
    # Create a simple test pattern for desktop capture
    if command -v xsetroot >/dev/null 2>&1; then
        xsetroot -solid "#2E3440" 2>/dev/null || true
    fi
    
    # 显示外部 TURN 服务器配置信息
    if [ ! -z "${TURN_SERVER_HOST:-}" ]; then
        log_info "External TURN server configured: ${TURN_SERVER_HOST}:${TURN_SERVER_PORT:-3478}"
    else
        log_warning "No external TURN server configured, WebRTC may not work behind NAT"
    fi
    
    log_success "Virtual desktop environment setup complete"
}

# Function to cleanup on exit
cleanup() {
    log_info "Cleaning up..."
    
    # Kill Xvfb if it's running
    if [ ! -z "$XVFB_PID" ]; then
        log_info "Stopping Xvfb (PID: $XVFB_PID)..."
        kill $XVFB_PID 2>/dev/null || true
        wait $XVFB_PID 2>/dev/null || true
    fi
    
    # Kill any remaining X processes
    pkill -f "Xvfb :${DISPLAY_NUM}" 2>/dev/null || true
    
    log_info "Cleanup complete"
}

# Set up signal handlers
trap cleanup EXIT INT TERM

# Function to validate environment
validate_environment() {
    log_info "Validating container environment..."
    
    # Check if running in container
    if [ ! -f /.dockerenv ]; then
        log_warning "Not running in Docker container"
    fi
    
    # Check required commands
    local required_commands="Xvfb xdpyinfo"
    for cmd in $required_commands; do
        if ! command -v $cmd >/dev/null 2>&1; then
            log_error "Required command not found: $cmd"
            exit 1
        fi
    done
    
    # Check if application binary exists
    if [ ! -f "/app/bdwind-gstreamer" ]; then
        log_error "Application binary not found: /app/bdwind-gstreamer"
        exit 1
    fi
    
    log_success "Environment validation passed"
}

# Function to show system information
show_system_info() {
    log_info "System Information:"
    echo "  OS: $(cat /etc/os-release | grep PRETTY_NAME | cut -d'"' -f2)"
    echo "  Kernel: $(uname -r)"
    echo "  Architecture: $(uname -m)"
    echo "  User: $(whoami)"
    echo "  UID: $(id -u)"
    echo "  GID: $(id -g)"
    echo "  Display: ${DISPLAY}"
    echo "  Resolution: ${DISPLAY_RESOLUTION}"
    echo "  XDG_RUNTIME_DIR: ${XDG_RUNTIME_DIR:-/tmp/runtime-bdwind}"
    
    # Show GStreamer information
    if command -v gst-inspect-1.0 >/dev/null 2>&1; then
        echo "  GStreamer version: $(gst-inspect-1.0 --version | head -1)"
    fi
}

# Main execution
main() {
    log_info "BDWind-GStreamer Docker Container Starting..."
    
    # Show system information
    show_system_info
    
    # Validate environment
    validate_environment
    
    # Start Xvfb virtual display
    if ! start_xvfb; then
        log_error "Failed to start virtual display"
        exit 1
    fi
    
    # Setup virtual desktop environment
    setup_virtual_desktop
    
    # Wait a moment for everything to settle
    sleep 2
    
    # Verify display is working
    if ! xdpyinfo -display ${DISPLAY} >/dev/null 2>&1; then
        log_error "Virtual display is not accessible"
        exit 1
    fi
    
    log_success "Virtual display environment ready"
    log_info "Starting BDWind-GStreamer application..."
    
    # Execute the main application with all passed arguments
    exec "$@"
}

# Handle special cases
case "${1:-}" in
    "bash"|"sh"|"/bin/bash"|"/bin/sh")
        # If user wants a shell, start Xvfb first then exec shell
        log_info "Starting interactive shell with virtual display..."
        validate_environment
        start_xvfb
        setup_virtual_desktop
        exec "$@"
        ;;
    "test")
        # Test mode - just validate and show info
        log_info "Running in test mode..."
        validate_environment
        start_xvfb
        setup_virtual_desktop
        log_success "Test completed successfully"
        exit 0
        ;;
    "--help"|"-h"|"help")
        echo "BDWind-GStreamer Docker Entrypoint"
        echo ""
        echo "Usage: $0 [command] [args...]"
        echo ""
        echo "Environment Variables:"
        echo "  DISPLAY_NUM         Virtual display number (default: 99)"
        echo "  DISPLAY_RESOLUTION  Virtual display resolution (default: 1920x1080x24)"
        echo "  XVFB_ARGS          Additional Xvfb arguments"
        echo ""
        echo "Special Commands:"
        echo "  test               Run environment test"
        echo "  bash/sh            Start interactive shell"
        echo "  help               Show this help"
        echo ""
        echo "Default: Start BDWind-GStreamer application"
        exit 0
        ;;
esac

# Run main function with all arguments
main "$@"