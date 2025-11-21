package config

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"
)

// HardwareCapabilityDetector 硬件能力检测器
type HardwareCapabilityDetector struct {
	logger *logrus.Entry
}

// HardwareCapabilities 硬件能力信息
type HardwareCapabilities struct {
	// GPU相关
	HasGPU      bool
	HasNVENC    bool
	HasVAAPI    bool
	HasQSV      bool // Intel Quick Sync Video
	GPUMemoryMB int
	GPUCount    int

	// CPU相关
	CPUCores   int
	CPUThreads int
	CPUFreqMHz int

	// 内存相关
	TotalMemoryMB     int
	AvailableMemoryMB int

	// 显示相关
	MaxResolution Resolution
	RefreshRate   int

	// 编码器支持
	SupportedEncoders []EncoderType
	SupportedCodecs   []CodecType

	// 系统信息
	OSType        string
	KernelVersion string

	// 检测时间戳
	DetectedAt string
}

// Resolution 分辨率信息
type Resolution struct {
	Width  int
	Height int
}

// OptimizationRecommendations 优化建议
type OptimizationRecommendations struct {
	EncoderType        EncoderType
	OptimalBitrate     int
	OptimalResolution  Resolution
	OptimalFrameRate   int
	UseHardware        bool
	RecommendedPreset  EncoderPreset
	RecommendedProfile EncoderProfile
	Reasoning          []string
}

// NewHardwareCapabilityDetector 创建硬件能力检测器
func NewHardwareCapabilityDetector(logger *logrus.Entry) *HardwareCapabilityDetector {
	return &HardwareCapabilityDetector{
		logger: logger,
	}
}

// DetectCapabilities 检测硬件能力
func (hcd *HardwareCapabilityDetector) DetectCapabilities() (*HardwareCapabilities, error) {
	hcd.logger.Debug("Starting hardware capability detection")

	capabilities := &HardwareCapabilities{
		SupportedEncoders: make([]EncoderType, 0),
		SupportedCodecs:   make([]CodecType, 0),
	}

	// 检测GPU能力
	if err := hcd.detectGPUCapabilities(capabilities); err != nil {
		hcd.logger.Warnf("Failed to detect GPU capabilities: %v", err)
	}

	// 检测CPU能力
	if err := hcd.detectCPUCapabilities(capabilities); err != nil {
		hcd.logger.Warnf("Failed to detect CPU capabilities: %v", err)
	}

	// 检测内存信息
	if err := hcd.detectMemoryCapabilities(capabilities); err != nil {
		hcd.logger.Warnf("Failed to detect memory capabilities: %v", err)
	}

	// 检测显示能力
	if err := hcd.detectDisplayCapabilities(capabilities); err != nil {
		hcd.logger.Warnf("Failed to detect display capabilities: %v", err)
	}

	// 检测编码器支持
	if err := hcd.detectEncoderSupport(capabilities); err != nil {
		hcd.logger.Warnf("Failed to detect encoder support: %v", err)
	}

	// 检测系统信息
	if err := hcd.detectSystemInfo(capabilities); err != nil {
		hcd.logger.Warnf("Failed to detect system info: %v", err)
	}

	hcd.logger.Infof("Hardware detection completed: GPU=%v, NVENC=%v, VAAPI=%v, CPU cores=%d, Memory=%dMB",
		capabilities.HasGPU, capabilities.HasNVENC, capabilities.HasVAAPI,
		capabilities.CPUCores, capabilities.TotalMemoryMB)

	return capabilities, nil
}

// detectGPUCapabilities 检测GPU能力
func (hcd *HardwareCapabilityDetector) detectGPUCapabilities(capabilities *HardwareCapabilities) error {
	// 检测NVIDIA GPU
	if err := hcd.detectNVIDIAGPU(capabilities); err != nil {
		hcd.logger.Debugf("NVIDIA GPU detection failed: %v", err)
	}

	// 检测Intel GPU (VAAPI)
	if err := hcd.detectIntelGPU(capabilities); err != nil {
		hcd.logger.Debugf("Intel GPU detection failed: %v", err)
	}

	// 检测AMD GPU (VAAPI)
	if err := hcd.detectAMDGPU(capabilities); err != nil {
		hcd.logger.Debugf("AMD GPU detection failed: %v", err)
	}

	return nil
}

// detectNVIDIAGPU 检测NVIDIA GPU
func (hcd *HardwareCapabilityDetector) detectNVIDIAGPU(capabilities *HardwareCapabilities) error {
	// 检查nvidia-smi命令
	cmd := exec.Command("nvidia-smi", "--query-gpu=name,memory.total", "--format=csv,noheader,nounits")
	output, err := cmd.Output()
	if err != nil {
		// 检查设备文件
		if _, err := os.Stat("/dev/nvidia0"); err == nil {
			capabilities.HasGPU = true
			capabilities.HasNVENC = true
			capabilities.GPUCount = 1
			hcd.logger.Debug("NVIDIA GPU detected via device file")
			return nil
		}
		return fmt.Errorf("nvidia-smi not available and no device file found")
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	capabilities.GPUCount = len(lines)
	capabilities.HasGPU = true
	capabilities.HasNVENC = true

	// 解析第一个GPU的内存信息
	if len(lines) > 0 {
		parts := strings.Split(lines[0], ",")
		if len(parts) >= 2 {
			if memStr := strings.TrimSpace(parts[1]); memStr != "" {
				if mem, err := strconv.Atoi(memStr); err == nil {
					capabilities.GPUMemoryMB = mem
				}
			}
		}
	}

	hcd.logger.Debugf("NVIDIA GPU detected: count=%d, memory=%dMB",
		capabilities.GPUCount, capabilities.GPUMemoryMB)

	return nil
}

// detectIntelGPU 检测Intel GPU
func (hcd *HardwareCapabilityDetector) detectIntelGPU(capabilities *HardwareCapabilities) error {
	// 检查Intel GPU设备文件
	if _, err := os.Stat("/dev/dri/renderD128"); err != nil {
		return fmt.Errorf("Intel GPU device not found")
	}

	// 检查VAAPI支持
	cmd := exec.Command("vainfo")
	output, err := cmd.Output()
	if err != nil {
		hcd.logger.Debug("vainfo command not available")
	} else {
		outputStr := string(output)
		if strings.Contains(outputStr, "VAProfileH264") {
			capabilities.HasVAAPI = true
			capabilities.HasGPU = true
			hcd.logger.Debug("Intel GPU with VAAPI support detected")
		}
	}

	return nil
}

// detectAMDGPU 检测AMD GPU
func (hcd *HardwareCapabilityDetector) detectAMDGPU(capabilities *HardwareCapabilities) error {
	// 检查AMD GPU设备文件
	matches, err := filepath.Glob("/dev/dri/renderD*")
	if err != nil || len(matches) == 0 {
		return fmt.Errorf("AMD GPU device not found")
	}

	// 检查是否为AMD GPU
	cmd := exec.Command("lspci")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("lspci command failed")
	}

	outputStr := strings.ToLower(string(output))
	if strings.Contains(outputStr, "amd") || strings.Contains(outputStr, "radeon") {
		capabilities.HasGPU = true
		// AMD GPU通常也支持VAAPI
		capabilities.HasVAAPI = true
		hcd.logger.Debug("AMD GPU detected")
	}

	return nil
}

// detectCPUCapabilities 检测CPU能力
func (hcd *HardwareCapabilityDetector) detectCPUCapabilities(capabilities *HardwareCapabilities) error {
	// 读取/proc/cpuinfo
	data, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return fmt.Errorf("failed to read /proc/cpuinfo: %w", err)
	}

	content := string(data)

	// 计算CPU核心数
	processorCount := strings.Count(content, "processor")
	capabilities.CPUThreads = processorCount

	// 获取物理核心数（简化处理）
	if processorCount > 0 {
		capabilities.CPUCores = processorCount / 2 // 假设支持超线程
		if capabilities.CPUCores == 0 {
			capabilities.CPUCores = processorCount
		}
	}

	// 获取CPU频率
	freqRegex := regexp.MustCompile(`cpu MHz\s*:\s*(\d+\.?\d*)`)
	if matches := freqRegex.FindStringSubmatch(content); len(matches) > 1 {
		if freq, err := strconv.ParseFloat(matches[1], 64); err == nil {
			capabilities.CPUFreqMHz = int(freq)
		}
	}

	hcd.logger.Debugf("CPU detected: cores=%d, threads=%d, freq=%dMHz",
		capabilities.CPUCores, capabilities.CPUThreads, capabilities.CPUFreqMHz)

	return nil
}

// detectMemoryCapabilities 检测内存能力
func (hcd *HardwareCapabilityDetector) detectMemoryCapabilities(capabilities *HardwareCapabilities) error {
	// 读取/proc/meminfo
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return fmt.Errorf("failed to read /proc/meminfo: %w", err)
	}

	content := string(data)

	// 解析总内存
	totalRegex := regexp.MustCompile(`MemTotal:\s*(\d+)\s*kB`)
	if matches := totalRegex.FindStringSubmatch(content); len(matches) > 1 {
		if totalKB, err := strconv.Atoi(matches[1]); err == nil {
			capabilities.TotalMemoryMB = totalKB / 1024
		}
	}

	// 解析可用内存
	availRegex := regexp.MustCompile(`MemAvailable:\s*(\d+)\s*kB`)
	if matches := availRegex.FindStringSubmatch(content); len(matches) > 1 {
		if availKB, err := strconv.Atoi(matches[1]); err == nil {
			capabilities.AvailableMemoryMB = availKB / 1024
		}
	}

	hcd.logger.Debugf("Memory detected: total=%dMB, available=%dMB",
		capabilities.TotalMemoryMB, capabilities.AvailableMemoryMB)

	return nil
}

// detectDisplayCapabilities 检测显示能力
func (hcd *HardwareCapabilityDetector) detectDisplayCapabilities(capabilities *HardwareCapabilities) error {
	// 尝试获取显示信息
	if display := os.Getenv("DISPLAY"); display != "" {
		// X11环境
		cmd := exec.Command("xrandr")
		output, err := cmd.Output()
		if err == nil {
			hcd.parseXrandrOutput(string(output), capabilities)
		}
	} else if os.Getenv("WAYLAND_DISPLAY") != "" {
		// Wayland环境 - 更难获取准确信息
		capabilities.MaxResolution = Resolution{Width: 1920, Height: 1080}
		capabilities.RefreshRate = 60
		hcd.logger.Debug("Wayland detected, using default resolution")
	}

	// 如果没有检测到，使用默认值
	if capabilities.MaxResolution.Width == 0 {
		capabilities.MaxResolution = Resolution{Width: 1920, Height: 1080}
		capabilities.RefreshRate = 60
	}

	return nil
}

// parseXrandrOutput 解析xrandr输出
func (hcd *HardwareCapabilityDetector) parseXrandrOutput(output string, capabilities *HardwareCapabilities) {
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		// 查找当前活动的分辨率
		if strings.Contains(line, "*") && strings.Contains(line, "+") {
			// 解析分辨率和刷新率
			resRegex := regexp.MustCompile(`(\d+)x(\d+).*?(\d+\.\d+)\*`)
			if matches := resRegex.FindStringSubmatch(line); len(matches) > 3 {
				if width, err := strconv.Atoi(matches[1]); err == nil {
					if height, err := strconv.Atoi(matches[2]); err == nil {
						if refresh, err := strconv.ParseFloat(matches[3], 64); err == nil {
							if width > capabilities.MaxResolution.Width {
								capabilities.MaxResolution.Width = width
								capabilities.MaxResolution.Height = height
								capabilities.RefreshRate = int(refresh)
							}
						}
					}
				}
			}
		}
	}

	hcd.logger.Debugf("Display detected: %dx%d@%dHz",
		capabilities.MaxResolution.Width, capabilities.MaxResolution.Height, capabilities.RefreshRate)
}

// detectEncoderSupport 检测编码器支持
func (hcd *HardwareCapabilityDetector) detectEncoderSupport(capabilities *HardwareCapabilities) error {
	// 基于硬件能力确定支持的编码器
	if capabilities.HasNVENC {
		capabilities.SupportedEncoders = append(capabilities.SupportedEncoders, EncoderTypeNVENC)
		capabilities.SupportedCodecs = append(capabilities.SupportedCodecs, CodecH264)
	}

	if capabilities.HasVAAPI {
		capabilities.SupportedEncoders = append(capabilities.SupportedEncoders, EncoderTypeVAAPI)
		capabilities.SupportedCodecs = append(capabilities.SupportedCodecs, CodecH264)
	}

	// 软件编码器总是可用
	capabilities.SupportedEncoders = append(capabilities.SupportedEncoders, EncoderTypeX264, EncoderTypeVP8, EncoderTypeVP9)
	capabilities.SupportedCodecs = append(capabilities.SupportedCodecs, CodecH264, CodecVP8, CodecVP9)

	return nil
}

// detectSystemInfo 检测系统信息
func (hcd *HardwareCapabilityDetector) detectSystemInfo(capabilities *HardwareCapabilities) error {
	// 检测操作系统
	if data, err := os.ReadFile("/etc/os-release"); err == nil {
		content := string(data)
		if strings.Contains(content, "Ubuntu") {
			capabilities.OSType = "Ubuntu"
		} else if strings.Contains(content, "Debian") {
			capabilities.OSType = "Debian"
		} else if strings.Contains(content, "CentOS") {
			capabilities.OSType = "CentOS"
		} else {
			capabilities.OSType = "Linux"
		}
	}

	// 检测内核版本
	if data, err := os.ReadFile("/proc/version"); err == nil {
		content := string(data)
		versionRegex := regexp.MustCompile(`Linux version (\S+)`)
		if matches := versionRegex.FindStringSubmatch(content); len(matches) > 1 {
			capabilities.KernelVersion = matches[1]
		}
	}

	return nil
}

// GetOptimizationRecommendations 获取优化建议
func (hcd *HardwareCapabilityDetector) GetOptimizationRecommendations(capabilities *HardwareCapabilities, config *Config) *OptimizationRecommendations {
	recommendations := &OptimizationRecommendations{
		Reasoning: make([]string, 0),
	}

	gstreamerConfig := config.GetGStreamerConfig()

	// 选择最佳编码器
	hcd.recommendEncoder(capabilities, recommendations)

	// 推荐比特率
	hcd.recommendBitrate(capabilities, gstreamerConfig, recommendations)

	// 推荐分辨率
	hcd.recommendResolution(capabilities, gstreamerConfig, recommendations)

	// 推荐帧率
	hcd.recommendFrameRate(capabilities, gstreamerConfig, recommendations)

	// 推荐编码器设置
	hcd.recommendEncoderSettings(capabilities, recommendations)

	return recommendations
}

// recommendEncoder 推荐编码器
func (hcd *HardwareCapabilityDetector) recommendEncoder(capabilities *HardwareCapabilities, recommendations *OptimizationRecommendations) {
	if capabilities.HasNVENC && capabilities.GPUMemoryMB > 2048 {
		recommendations.EncoderType = EncoderTypeNVENC
		recommendations.UseHardware = true
		recommendations.Reasoning = append(recommendations.Reasoning, "NVENC recommended: NVIDIA GPU with sufficient memory detected")
	} else if capabilities.HasVAAPI {
		recommendations.EncoderType = EncoderTypeVAAPI
		recommendations.UseHardware = true
		recommendations.Reasoning = append(recommendations.Reasoning, "VAAPI recommended: Intel/AMD GPU with hardware acceleration detected")
	} else if capabilities.CPUCores >= 4 {
		recommendations.EncoderType = EncoderTypeX264
		recommendations.UseHardware = false
		recommendations.Reasoning = append(recommendations.Reasoning, "x264 recommended: Multi-core CPU detected, no hardware acceleration available")
	} else {
		recommendations.EncoderType = EncoderTypeVP8
		recommendations.UseHardware = false
		recommendations.Reasoning = append(recommendations.Reasoning, "VP8 recommended: Limited CPU resources, VP8 is more efficient than x264")
	}
}

// recommendBitrate 推荐比特率
func (hcd *HardwareCapabilityDetector) recommendBitrate(capabilities *HardwareCapabilities, config *GStreamerConfig, recommendations *OptimizationRecommendations) {
	pixels := config.Capture.Width * config.Capture.Height
	baseRate := 1500 // 基础比特率 (kbps)

	// 根据分辨率调整
	switch {
	case pixels <= 1280*720: // 720p
		baseRate = 1500
	case pixels <= 1920*1080: // 1080p
		baseRate = 2500
	case pixels <= 2560*1440: // 1440p
		baseRate = 4000
	default: // 4K+
		baseRate = 8000
	}

	// 根据硬件能力调整
	if capabilities.HasNVENC || capabilities.HasVAAPI {
		// 硬件编码可以使用更高比特率
		baseRate = int(float64(baseRate) * 1.2)
		recommendations.Reasoning = append(recommendations.Reasoning, "Bitrate increased 20% for hardware encoding")
	} else if capabilities.CPUCores < 4 {
		// CPU资源有限，降低比特率
		baseRate = int(float64(baseRate) * 0.8)
		recommendations.Reasoning = append(recommendations.Reasoning, "Bitrate reduced 20% due to limited CPU resources")
	}

	// 根据可用内存调整
	if capabilities.AvailableMemoryMB < 2048 {
		baseRate = int(float64(baseRate) * 0.9)
		recommendations.Reasoning = append(recommendations.Reasoning, "Bitrate reduced 10% due to limited memory")
	}

	recommendations.OptimalBitrate = baseRate
}

// recommendResolution 推荐分辨率
func (hcd *HardwareCapabilityDetector) recommendResolution(capabilities *HardwareCapabilities, config *GStreamerConfig, recommendations *OptimizationRecommendations) {
	maxRes := capabilities.MaxResolution

	// 如果检测到的分辨率合理，使用它
	if maxRes.Width > 0 && maxRes.Height > 0 {
		recommendations.OptimalResolution = maxRes
		recommendations.Reasoning = append(recommendations.Reasoning, fmt.Sprintf("Resolution set to detected display resolution: %dx%d", maxRes.Width, maxRes.Height))
	} else {
		// 使用默认分辨率
		recommendations.OptimalResolution = Resolution{Width: 1920, Height: 1080}
		recommendations.Reasoning = append(recommendations.Reasoning, "Resolution set to default 1080p")
	}

	// 根据硬件能力限制分辨率
	if !capabilities.HasGPU && capabilities.CPUCores < 4 {
		if recommendations.OptimalResolution.Width > 1280 {
			recommendations.OptimalResolution = Resolution{Width: 1280, Height: 720}
			recommendations.Reasoning = append(recommendations.Reasoning, "Resolution limited to 720p due to limited CPU resources")
		}
	}
}

// recommendFrameRate 推荐帧率
func (hcd *HardwareCapabilityDetector) recommendFrameRate(capabilities *HardwareCapabilities, config *GStreamerConfig, recommendations *OptimizationRecommendations) {
	baseFrameRate := 30

	// 根据硬件能力调整
	if capabilities.HasNVENC || capabilities.HasVAAPI {
		if capabilities.RefreshRate > 60 {
			baseFrameRate = 60
			recommendations.Reasoning = append(recommendations.Reasoning, "Frame rate set to 60fps for hardware encoding with high refresh display")
		}
	} else if capabilities.CPUCores < 4 {
		baseFrameRate = 24
		recommendations.Reasoning = append(recommendations.Reasoning, "Frame rate limited to 24fps due to limited CPU resources")
	}

	recommendations.OptimalFrameRate = baseFrameRate
}

// recommendEncoderSettings 推荐编码器设置
func (hcd *HardwareCapabilityDetector) recommendEncoderSettings(capabilities *HardwareCapabilities, recommendations *OptimizationRecommendations) {
	if recommendations.UseHardware {
		recommendations.RecommendedPreset = PresetFast
		recommendations.RecommendedProfile = ProfileMain
		recommendations.Reasoning = append(recommendations.Reasoning, "Fast preset and Main profile recommended for hardware encoding")
	} else if capabilities.CPUCores >= 8 {
		recommendations.RecommendedPreset = PresetMedium
		recommendations.RecommendedProfile = ProfileHigh
		recommendations.Reasoning = append(recommendations.Reasoning, "Medium preset and High profile recommended for high-end CPU")
	} else if capabilities.CPUCores >= 4 {
		recommendations.RecommendedPreset = PresetFast
		recommendations.RecommendedProfile = ProfileMain
		recommendations.Reasoning = append(recommendations.Reasoning, "Fast preset and Main profile recommended for mid-range CPU")
	} else {
		recommendations.RecommendedPreset = PresetVeryFast
		recommendations.RecommendedProfile = ProfileBaseline
		recommendations.Reasoning = append(recommendations.Reasoning, "Very fast preset and Baseline profile recommended for limited CPU")
	}
}
