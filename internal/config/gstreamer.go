package config

import (
	"fmt"
)

// GStreamerConfig GStreamer配置模块 - 只支持 go-gst 实现
type GStreamerConfig struct {
	Capture  DesktopCaptureConfig `yaml:"capture" json:"capture"`
	Encoding EncoderConfig        `yaml:"encoding" json:"encoding"`
	Audio    AudioConfig          `yaml:"audio" json:"audio"`
}

// DefaultGStreamerConfig 返回默认的GStreamer配置
func DefaultGStreamerConfig() *GStreamerConfig {
	return &GStreamerConfig{
		Capture:  DefaultDesktopCaptureConfig(),
		Encoding: DefaultEncoderConfig(),
		Audio:    DefaultAudioConfig(),
	}
}

// Validate 验证配置
func (c *GStreamerConfig) Validate() error {
	// 验证桌面捕获配置
	if err := ValidateDesktopCaptureConfig(&c.Capture); err != nil {
		return fmt.Errorf("invalid capture config: %w", err)
	}

	// 验证编码配置
	if err := ValidateEncoderConfig(&c.Encoding); err != nil {
		return fmt.Errorf("invalid encoding config: %w", err)
	}

	// 验证音频配置
	if err := ValidateAudioConfig(&c.Audio); err != nil {
		return fmt.Errorf("invalid audio config: %w", err)
	}

	return nil
}

// SetDefaults sets default values for the GStreamer configuration.
func (c *GStreamerConfig) SetDefaults() {
	c.Capture = DefaultDesktopCaptureConfig()
	c.Encoding = DefaultEncoderConfig()
	c.Audio = DefaultAudioConfig()
}

// Merge merges another ConfigModule into this one.
func (c *GStreamerConfig) Merge(other ConfigModule) error {
	otherConfig, ok := other.(*GStreamerConfig)
	if !ok {
		return fmt.Errorf("cannot merge different config types")
	}

	// 合并桌面捕获配置
	if err := c.mergeDesktopCaptureConfig(&otherConfig.Capture); err != nil {
		return fmt.Errorf("failed to merge capture config: %w", err)
	}

	// A more sophisticated merge would be needed here, for now we just overwrite
	c.Encoding = otherConfig.Encoding

	return nil
}

// mergeDesktopCaptureConfig 合并桌面捕获配置
func (c *GStreamerConfig) mergeDesktopCaptureConfig(other *DesktopCaptureConfig) error {
	if other.DisplayID != "" && other.DisplayID != ":0" {
		c.Capture.DisplayID = other.DisplayID
	}
	if other.FrameRate != 0 && other.FrameRate != 30 {
		c.Capture.FrameRate = other.FrameRate
	}
	if other.Width != 0 && other.Width != 1920 {
		c.Capture.Width = other.Width
	}
	if other.Height != 0 && other.Height != 1080 {
		c.Capture.Height = other.Height
	}
	if other.UseWayland != c.Capture.UseWayland {
		c.Capture.UseWayland = other.UseWayland
	}
	if other.CaptureRegion != nil {
		c.Capture.CaptureRegion = other.CaptureRegion
	}
	if other.AutoDetect != c.Capture.AutoDetect {
		c.Capture.AutoDetect = other.AutoDetect
	}
	if other.ShowPointer != c.Capture.ShowPointer {
		c.Capture.ShowPointer = other.ShowPointer
	}
	if other.UseDamage != c.Capture.UseDamage {
		c.Capture.UseDamage = other.UseDamage
	}
	if other.BufferSize != 0 && other.BufferSize != 10 {
		c.Capture.BufferSize = other.BufferSize
	}
	if other.Quality != "" && other.Quality != "medium" {
		c.Capture.Quality = other.Quality
	}

	return nil
}

// GetOptimalSettings 根据系统能力获取最优设置
func (c *GStreamerConfig) GetOptimalSettings() error {
	// 根据分辨率调整比特率
	pixels := c.Capture.Width * c.Capture.Height
	switch {
	case pixels <= 1280*720: // 720p
		if c.Encoding.Bitrate > 3000 {
			c.Encoding.Bitrate = 1500
		}
	case pixels <= 1920*1080: // 1080p
		if c.Encoding.Bitrate > 5000 {
			c.Encoding.Bitrate = 2500
		}
	case pixels <= 2560*1440: // 1440p
		if c.Encoding.Bitrate > 8000 {
			c.Encoding.Bitrate = 4000
		}
	default: // 4K+
		if c.Encoding.Bitrate > 15000 {
			c.Encoding.Bitrate = 8000
		}
	}

	// 根据帧率调整比特率
	if c.Capture.FrameRate > 30 {
		c.Encoding.Bitrate = int(float64(c.Encoding.Bitrate) * 1.5)
	}

	return nil
}

// GetCodecCapabilities 获取编解码器能力
func (c *GStreamerConfig) GetCodecCapabilities() map[string]interface{} {
	capabilities := make(map[string]interface{})

	switch c.Encoding.Codec {
	case "vp8":
		capabilities["hardware_acceleration"] = false
		capabilities["max_bitrate"] = 10000
		capabilities["supports_alpha"] = false
	case "vp9":
		capabilities["hardware_acceleration"] = true
		capabilities["max_bitrate"] = 20000
		capabilities["supports_alpha"] = true
	case "h264":
		capabilities["hardware_acceleration"] = true
		capabilities["max_bitrate"] = 25000
		capabilities["supports_alpha"] = false
	}

	return capabilities
}
