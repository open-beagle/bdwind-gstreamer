package config

import (
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHardwareCapabilityDetector_Basic(t *testing.T) {
	logger := logrus.WithField("test", "hardware-detector")
	detector := NewHardwareCapabilityDetector(logger)

	// 检测硬件能力
	capabilities, err := detector.DetectCapabilities()
	require.NoError(t, err)
	assert.NotNil(t, capabilities)

	// 验证基本字段
	assert.GreaterOrEqual(t, capabilities.CPUCores, 1)
	assert.GreaterOrEqual(t, capabilities.TotalMemoryMB, 1)
	assert.NotEmpty(t, capabilities.SupportedEncoders)
	assert.NotEmpty(t, capabilities.SupportedCodecs)

	// 获取优化建议
	config := DefaultConfig()
	recommendations := detector.GetOptimizationRecommendations(capabilities, config)
	assert.NotNil(t, recommendations)
	assert.NotEmpty(t, recommendations.Reasoning)
}

func TestConfigValidator_BasicValidation(t *testing.T) {
	logger := logrus.WithField("test", "config-validator")
	validator := NewConfigValidator(logger)

	// 创建一个基本有效的配置
	validConfig := DefaultConfig()

	// 测试基本结构验证
	result := &ValidationResult{
		Valid:    true,
		Errors:   make([]ValidationError, 0),
		Warnings: make([]ValidationWarning, 0),
	}

	validator.validateBasicStructure(validConfig, result)
	assert.True(t, result.Valid)
	assert.Empty(t, result.Errors)

	// 测试GStreamer配置验证
	validator.validateGStreamerConfig(validConfig, result)
	assert.True(t, result.Valid)

	// 测试无效配置
	invalidConfig := DefaultConfig()
	invalidConfig.GStreamer.Capture.Width = -1
	invalidConfig.GStreamer.Capture.Height = -1

	result = &ValidationResult{
		Valid:    true,
		Errors:   make([]ValidationError, 0),
		Warnings: make([]ValidationWarning, 0),
	}

	validator.validateGStreamerConfig(invalidConfig, result)
	assert.False(t, result.Valid)
	assert.NotEmpty(t, result.Errors)
}
