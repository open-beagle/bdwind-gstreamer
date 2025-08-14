# BDWind-GStreamer Container Deployment Guide

This guide covers containerized deployment of BDWind-GStreamer using Docker and Docker Compose.

## Quick Start

### Prerequisites

- Docker Engine 20.10+
- Docker Compose 2.0+
- X11 display server running
- Hardware acceleration drivers (optional but recommended)

### Basic Deployment

1. **Clone the repository:**
   ```bash
   git clone https://github.com/open-beagle/bdwind-gstreamer.git
   cd bdwind-gstreamer
   ```

2. **Build and start the container:**
   ```bash
   docker-compose up -d
   ```

3. **Access the web interface:**
   ```
   http://localhost:8080
   ```

## Container Configurations

### Development Environment

For development with live code reloading and debug features:

```bash
# Uses docker-compose.override.yml automatically
docker-compose up -d
```

### Production Environment

For production deployment with optimized settings:

```bash
docker-compose -f docker-compose.yml -f docker-compose.prod.yml up -d
```

### With Monitoring Stack

To enable Prometheus and Grafana monitoring:

```bash
docker-compose --profile monitoring up -d
```

### With WebRTC Support

To enable TURN server for NAT traversal:

```bash
docker-compose --profile webrtc up -d
```

## Virtual Display Environment

BDWind-GStreamer uses Xvfb (X Virtual Framebuffer) to create an isolated virtual display environment within the container. This approach provides several benefits:

- **Security**: No need to expose the host's X11 socket
- **Portability**: Works on any system without X11 dependencies
- **Isolation**: Complete separation from host display system
- **Consistency**: Predictable display environment across deployments

The virtual display is automatically configured with:
- Display number: `:99` (configurable via `DISPLAY_NUM`)
- Resolution: `1920x1080x24` (configurable via `DISPLAY_RESOLUTION`)
- Color depth: 24-bit
- DPI: 96

## Configuration

### Environment Variables

Key environment variables for container configuration:

| Variable | Default | Description |
|----------|---------|-------------|
| `DISPLAY` | `:99` | Virtual X11 display number |
| `DISPLAY_NUM` | `99` | Xvfb display number |
| `DISPLAY_RESOLUTION` | `1920x1080x24` | Virtual display resolution |
| `BDWIND_WEB_PORT` | `8080` | Web interface port |
| `BDWIND_WEB_HOST` | `0.0.0.0` | Web interface host |
| `BDWIND_AUTH_ENABLED` | `true` | Enable authentication |
| `BDWIND_MONITORING_ENABLED` | `true` | Enable metrics collection |
| `GST_DEBUG` | `2` | GStreamer debug level |
| `LIBVA_DRIVER_NAME` | `iHD` | VAAPI driver name |

### Volume Mounts

Essential volume mounts for proper operation:

```yaml
volumes:
  # Hardware acceleration
  - /dev/dri:/dev/dri:rw
  
  # Configuration
  - ./config:/app/config:ro
  
  # Persistent data
  - ./data:/app/data:rw
  - ./logs:/app/logs:rw
```

**Note**: This container uses Xvfb (X Virtual Framebuffer) to create a virtual display environment, eliminating the need to mount the host's `/tmp/.X11-unix` directory. This provides better security isolation and portability.

### Hardware Acceleration

#### NVIDIA GPU Support

For NVIDIA GPU acceleration, use the NVIDIA Container Toolkit:

```yaml
services:
  bdwind-gstreamer:
    runtime: nvidia
    environment:
      - NVIDIA_VISIBLE_DEVICES=all
      - NVIDIA_DRIVER_CAPABILITIES=compute,video,utility
```

#### Intel GPU Support

For Intel integrated graphics:

```yaml
services:
  bdwind-gstreamer:
    devices:
      - /dev/dri:/dev/dri
    environment:
      - LIBVA_DRIVER_NAME=iHD
      - VAAPI_DEVICE=/dev/dri/renderD128
```

## Networking

### Port Configuration

Default ports used by the application:

- `8080`: Web interface and WebRTC signaling
- `9090`: Prometheus metrics endpoint
- `3000`: Grafana dashboard (when enabled)
- `9091`: Prometheus web interface (when enabled)

### WebRTC NAT Traversal

For clients behind NAT, configure TURN server:

```yaml
environment:
  - TURN_USERNAME=your-username
  - TURN_PASSWORD=your-password
```

## Security

### Authentication

Enable authentication in production:

```yaml
environment:
  - BDWIND_AUTH_ENABLED=true
  - BDWIND_AUTH_DEFAULT_ROLE=viewer
```

### TLS/SSL

For HTTPS support, mount certificates:

```yaml
volumes:
  - ./ssl/cert.pem:/app/ssl/cert.pem:ro
  - ./ssl/key.pem:/app/ssl/key.pem:ro
environment:
  - BDWIND_WEB_ENABLE_TLS=true
  - BDWIND_WEB_TLS_CERT=/app/ssl/cert.pem
  - BDWIND_WEB_TLS_KEY=/app/ssl/key.pem
```

### Firewall Configuration

Open required ports in your firewall:

```bash
# Web interface
sudo ufw allow 8080/tcp

# Metrics (if exposed)
sudo ufw allow 9090/tcp

# TURN server (if used)
sudo ufw allow 3478/udp
sudo ufw allow 49152:65535/udp
```

## Monitoring

### Prometheus Metrics

Access metrics at: `http://localhost:9090/metrics`

Key metrics collected:
- CPU and memory usage
- GStreamer pipeline statistics
- WebRTC connection metrics
- Frame rate and encoding statistics

### Grafana Dashboard

Access Grafana at: `http://localhost:3000`
- Username: `admin`
- Password: `admin` (change in production)

### Health Checks

The container includes built-in health checks:

```bash
# Check container health
docker ps

# View health check logs
docker inspect bdwind-gstreamer | grep -A 10 Health
```

## Troubleshooting

### Common Issues

1. **Virtual display not working:**
   ```bash
   # Check if Xvfb is running in container
   docker exec -it bdwind-gstreamer ps aux | grep Xvfb
   
   # Test virtual display
   docker exec -it bdwind-gstreamer xdpyinfo -display :99
   
   # Check display environment
   docker exec -it bdwind-gstreamer env | grep DISPLAY
   ```

2. **Hardware acceleration not working:**
   ```bash
   # Check GPU devices
   ls -la /dev/dri/
   
   # Test VAAPI
   docker exec -it bdwind-gstreamer vainfo
   ```

3. **Permission denied errors:**
   ```bash
   # Fix ownership
   sudo chown -R $USER:$USER ./data ./logs
   ```

### Debug Mode

Enable debug logging:

```yaml
environment:
  - BDWIND_DEBUG=true
  - GST_DEBUG=3
```

### Container Logs

View application logs:

```bash
# Follow logs
docker-compose logs -f bdwind-gstreamer

# View specific service logs
docker logs bdwind-gstreamer
```

## Performance Tuning

### Resource Limits

Adjust resource limits based on your needs:

```yaml
deploy:
  resources:
    limits:
      memory: 4G
      cpus: '4.0'
    reservations:
      memory: 1G
      cpus: '1.0'
```

### GStreamer Optimization

Optimize GStreamer pipeline:

```yaml
environment:
  - GST_PLUGIN_PATH=/usr/lib/x86_64-linux-gnu/gstreamer-1.0
  - GST_REGISTRY=/tmp/gst-registry.bin
```

## Backup and Recovery

### Data Backup

Backup persistent data:

```bash
# Create backup
docker run --rm -v bdwind_data:/data -v $(pwd):/backup alpine tar czf /backup/bdwind-data-backup.tar.gz -C /data .

# Restore backup
docker run --rm -v bdwind_data:/data -v $(pwd):/backup alpine tar xzf /backup/bdwind-data-backup.tar.gz -C /data
```

### Configuration Backup

Backup configuration:

```bash
cp -r config/ config-backup-$(date +%Y%m%d)
```

## Scaling

### Multiple Instances

Run multiple instances with different displays:

```yaml
services:
  bdwind-display-0:
    extends: bdwind-gstreamer
    environment:
      - DISPLAY=:0
    ports:
      - "8080:8080"
  
  bdwind-display-1:
    extends: bdwind-gstreamer
    environment:
      - DISPLAY=:1
    ports:
      - "8081:8080"
```

### Load Balancing

Use nginx for load balancing:

```nginx
upstream bdwind_backend {
    server bdwind-display-0:8080;
    server bdwind-display-1:8080;
}

server {
    listen 80;
    location / {
        proxy_pass http://bdwind_backend;
    }
}
```

## Testing

### Automated Testing

Run container tests:

```bash
# Run all tests
./scripts/test-container.sh

# View test logs
./scripts/test-container.sh logs

# Access test container shell
./scripts/test-container.sh shell

# Cleanup test containers
./scripts/test-container.sh cleanup
```

### Manual Testing

1. **Web Interface Test:**
   ```bash
   curl -f http://localhost:8080/health
   ```

2. **API Test:**
   ```bash
   curl -f http://localhost:8080/api/status
   ```

3. **Metrics Test:**
   ```bash
   curl -f http://localhost:9090/metrics
   ```

## Production Checklist

- [ ] Change default passwords
- [ ] Configure TLS/SSL certificates
- [ ] Set up proper authentication
- [ ] Configure firewall rules
- [ ] Set up monitoring and alerting
- [ ] Configure log rotation
- [ ] Set up backup procedures
- [ ] Test disaster recovery
- [ ] Configure resource limits
- [ ] Set up health checks

## Support

For issues and questions:

- GitHub Issues: [https://github.com/open-beagle/bdwind-gstreamer/issues](https://github.com/open-beagle/bdwind-gstreamer/issues)
- Documentation: [https://github.com/open-beagle/bdwind-gstreamer/wiki](https://github.com/open-beagle/bdwind-gstreamer/wiki)