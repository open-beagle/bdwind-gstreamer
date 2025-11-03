# Configuration Examples

This directory contains configuration examples for bdwind-gstreamer.

## Available Configurations

### config.yaml
Standard configuration for production use with external STUN servers.

### debug.yaml
Debug configuration identical to config.yaml, suitable for development and testing.

## Key Changes

**Note**: As of the latest version, bdwind-gstreamer uses go-gst implementation exclusively. The following configuration options have been removed:

- `gstreamer.implementation` - No longer needed (always uses go-gst)
- `implementation.use_simplified` - No longer needed (simplified architecture)
- `implementation.type` - No longer needed

## Usage

```bash
# Start with standard configuration
./bdwind-gstreamer --config examples/config.yaml

# Start with debug configuration
./bdwind-gstreamer --config examples/debug.yaml

# Build only (for testing)
./scripts/start.sh --build-only

# Full startup with virtual display
./scripts/start.sh
```

## Configuration Structure

```yaml
# Web server settings
webserver:
  host: "0.0.0.0"
  port: 48080
  enable_tls: false
  static_dir: "internal/webserver/static"
  auth:
    enabled: false

# GStreamer capture and encoding
gstreamer:
  capture:
    display_id: ":99"
    enabled: true
    use_software_rendering: true
    width: 1920
    height: 1080
    framerate: 30
  encoding:
    codec: "h264"
    use_software_encoder: true
    bitrate: 2500

# WebRTC signaling
webrtc:
  ice_servers:
    - urls: ["stun:stun.ali.wodcloud.com:3478"]

# Logging configuration
logging:
  level: "info"
  file: ".tmp/bdwind-gstreamer.log"
```

## Architecture

The current implementation uses:
- **go-gst**: Direct GStreamer library integration
- **appsink**: Direct memory transfer from GStreamer to WebRTC
- **Minimal managers**: Simplified component architecture
- **Media bridge**: Direct connection between GStreamer and WebRTC

This provides better performance, lower latency, and more reliable operation compared to the previous command-line based approach.