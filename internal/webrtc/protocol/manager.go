package protocol

import (
	"fmt"
	"sync"

	"github.com/sirupsen/logrus"

	"github.com/open-beagle/bdwind-gstreamer/internal/config"
)

// ProtocolManager 协议管理器
type ProtocolManager struct {
	adapters        map[ProtocolVersion]ProtocolAdapter
	validator       *MessageValidator
	defaultProtocol ProtocolVersion
	autoDetection   bool
	mutex           sync.RWMutex
	config          *ManagerConfig
	logger          *logrus.Entry
}

// ManagerConfig 管理器配置
type ManagerConfig struct {
	DefaultProtocol    ProtocolVersion   `json:"default_protocol"`
	AutoDetection      bool              `json:"auto_detection"`
	StrictValidation   bool              `json:"strict_validation"`
	EnableLogging      bool              `json:"enable_logging"`
	SupportedProtocols []ProtocolVersion `json:"supported_protocols"`
	ValidatorConfig    *ValidatorConfig  `json:"validator_config"`
}

// DetectionResult 协议检测结果
type DetectionResult struct {
	Protocol   ProtocolVersion `json:"protocol"`
	Confidence float64         `json:"confidence"`
	Reason     string          `json:"reason"`
}

// ProcessingResult 处理结果
type ProcessingResult struct {
	Message          *StandardMessage  `json:"message"`
	OriginalProtocol ProtocolVersion   `json:"original_protocol"`
	ValidationResult *ValidationResult `json:"validation_result"`
	Errors           []string          `json:"errors"`
	Warnings         []string          `json:"warnings"`
}

// NewProtocolManager 创建协议管理器
func NewProtocolManager(cfg *ManagerConfig) *ProtocolManager {
	if cfg == nil {
		cfg = DefaultManagerConfig()
	}

	manager := &ProtocolManager{
		adapters:        make(map[ProtocolVersion]ProtocolAdapter),
		defaultProtocol: cfg.DefaultProtocol,
		autoDetection:   cfg.AutoDetection,
		config:          cfg,
		logger:          config.GetLoggerWithPrefix("webrtc-protocol-manager"),
	}

	// 初始化验证器
	if cfg.ValidatorConfig == nil {
		cfg.ValidatorConfig = DefaultValidatorConfig()
	}
	manager.validator = NewMessageValidator(cfg.ValidatorConfig)

	// 注册默认适配器
	manager.registerDefaultAdapters()

	return manager
}

// DefaultManagerConfig 默认管理器配置
func DefaultManagerConfig() *ManagerConfig {
	return &ManagerConfig{
		DefaultProtocol:  ProtocolVersionGStreamer10,
		AutoDetection:    true,
		StrictValidation: true,
		EnableLogging:    true,
		SupportedProtocols: []ProtocolVersion{
			ProtocolVersionGStreamer10,
			ProtocolVersionSelkies,
		},
		ValidatorConfig: DefaultValidatorConfig(),
	}
}

// RegisterAdapter 注册协议适配器
func (pm *ProtocolManager) RegisterAdapter(protocol ProtocolVersion, adapter ProtocolAdapter) {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	pm.adapters[protocol] = adapter

	pm.logger.Debugf("Protocol adapter registered: %s", protocol)
}

// UnregisterAdapter 注销协议适配器
func (pm *ProtocolManager) UnregisterAdapter(protocol ProtocolVersion) {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	delete(pm.adapters, protocol)

	pm.logger.Debugf("Protocol adapter unregistered: %s", protocol)
}

// DetectProtocol 检测协议类型
func (pm *ProtocolManager) DetectProtocol(data []byte) *DetectionResult {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()

	bestMatch := &DetectionResult{
		Protocol:   pm.defaultProtocol,
		Confidence: 0.0,
		Reason:     "default protocol",
	}

	// // 尝试每个适配器
	// for protocol, adapter := range pm.adapters {
	// 	if adapter.IsCompatible(data) {
	// 		confidence := pm.calculateConfidence(data, adapter)

	// 		if confidence > bestMatch.Confidence {
	// 			bestMatch = &DetectionResult{
	// 				Protocol:   protocol,
	// 				Confidence: confidence,
	// 				Reason:     fmt.Sprintf("compatible with %s adapter", protocol),
	// 			}
	// 		}
	// 	}
	// }

	pm.logger.Debugf("Protocol detection result: %s (confidence: %.2f) - %s",
		bestMatch.Protocol, bestMatch.Confidence, bestMatch.Reason)

	return bestMatch
}

// ParseMessage 解析消息
func (pm *ProtocolManager) ParseMessage(data []byte, protocol ...ProtocolVersion) (*ProcessingResult, error) {
	result := &ProcessingResult{
		Errors:   make([]string, 0),
		Warnings: make([]string, 0),
	}

	// 确定使用的协议
	var targetProtocol ProtocolVersion
	if len(protocol) > 0 && protocol[0] != "" {
		targetProtocol = protocol[0]
	} else if pm.autoDetection {
		detection := pm.DetectProtocol(data)
		targetProtocol = detection.Protocol
		result.OriginalProtocol = detection.Protocol
	} else {
		targetProtocol = pm.defaultProtocol
		result.OriginalProtocol = pm.defaultProtocol
	}

	// 获取适配器
	adapter, exists := pm.getAdapter(targetProtocol)
	if !exists {
		return nil, fmt.Errorf("no adapter found for protocol: %s", targetProtocol)
	}

	// 解析消息
	message, err := adapter.ParseMessage(data)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("parse error: %v", err))
		return result, fmt.Errorf("failed to parse message: %w", err)
	}

	result.Message = message

	// 验证消息
	if pm.config.StrictValidation {
		validationResult := pm.validator.ValidateMessage(message)
		result.ValidationResult = validationResult

		// 收集验证错误和警告
		for _, validationError := range validationResult.Errors {
			result.Errors = append(result.Errors, validationError.Message)
		}

		for _, validationWarning := range validationResult.Warnings {
			result.Warnings = append(result.Warnings, validationWarning.Message)
		}

		// if !validationResult.Valid {
		// 	return result, fmt.Errorf("message validation failed")
		// }
	}

	pm.logger.Debugf("Message parsed successfully: type=%s, protocol=%s", message.Type, targetProtocol)

	return result, nil
}

// FormatMessage 格式化消息
func (pm *ProtocolManager) FormatMessage(message *StandardMessage, protocol ...ProtocolVersion) ([]byte, error) {
	if message == nil {
		return nil, fmt.Errorf("message is nil")
	}

	// 确定使用的协议
	var targetProtocol ProtocolVersion
	if len(protocol) > 0 && protocol[0] != "" {
		targetProtocol = protocol[0]
	} else if message.Version != "" {
		targetProtocol = message.Version
	} else {
		targetProtocol = pm.defaultProtocol
	}

	// 获取适配器
	adapter, exists := pm.getAdapter(targetProtocol)
	if !exists {
		return nil, fmt.Errorf("no adapter found for protocol: %s", targetProtocol)
	}

	// // 验证消息
	// if pm.config.StrictValidation {
	// 	validationResult := pm.validator.ValidateMessage(message)
	// 	if !validationResult.Valid {
	// 		return nil, fmt.Errorf("message validation failed: %v", validationResult.Errors)
	// 	}
	// }

	// 格式化消息
	data, err := adapter.FormatMessage(message)
	if err != nil {
		return nil, fmt.Errorf("failed to format message: %w", err)
	}

	pm.logger.Debugf("Message formatted successfully: type=%s, protocol=%s, size=%d bytes",
		message.Type, targetProtocol, len(data))

	return data, nil
}

// ConvertMessage 转换消息协议
func (pm *ProtocolManager) ConvertMessage(data []byte, fromProtocol, toProtocol ProtocolVersion) ([]byte, error) {
	// 使用源协议解析消息
	parseResult, err := pm.ParseMessage(data, fromProtocol)
	if err != nil {
		return nil, fmt.Errorf("failed to parse source message: %w", err)
	}

	// 更新消息协议版本
	parseResult.Message.Version = toProtocol

	// 使用目标协议格式化消息
	convertedData, err := pm.FormatMessage(parseResult.Message, toProtocol)
	if err != nil {
		return nil, fmt.Errorf("failed to format target message: %w", err)
	}

	pm.logger.Debugf("Message converted: %s -> %s, size: %d -> %d bytes",
		fromProtocol, toProtocol, len(data), len(convertedData))

	return convertedData, nil
}

// ValidateMessage 验证消息
func (pm *ProtocolManager) ValidateMessage(message *StandardMessage) *ValidationResult {
	return pm.validator.ValidateMessage(message)
}

// GetSupportedProtocols 获取支持的协议列表
func (pm *ProtocolManager) GetSupportedProtocols() []ProtocolVersion {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()

	protocols := make([]ProtocolVersion, 0, len(pm.adapters))
	for protocol := range pm.adapters {
		protocols = append(protocols, protocol)
	}

	return protocols
}

// GetSupportedMessageTypes 获取指定协议支持的消息类型
func (pm *ProtocolManager) GetSupportedMessageTypes(protocol ProtocolVersion) ([]MessageType, error) {
	adapter, exists := pm.getAdapter(protocol)
	if !exists {
		return nil, fmt.Errorf("no adapter found for protocol: %s", protocol)
	}

	return adapter.GetSupportedMessageTypes(), nil
}

// SetDefaultProtocol 设置默认协议
func (pm *ProtocolManager) SetDefaultProtocol(protocol ProtocolVersion) error {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	if _, exists := pm.adapters[protocol]; !exists {
		return fmt.Errorf("protocol not supported: %s", protocol)
	}

	pm.defaultProtocol = protocol
	pm.config.DefaultProtocol = protocol

	pm.logger.Infof("Default protocol changed to: %s", protocol)

	return nil
}

// SetAutoDetection 设置自动检测
func (pm *ProtocolManager) SetAutoDetection(enabled bool) {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	pm.autoDetection = enabled
	pm.config.AutoDetection = enabled

	pm.logger.Infof("Auto detection %s", map[bool]string{true: "enabled", false: "disabled"}[enabled])
}

// GetStats 获取统计信息
func (pm *ProtocolManager) GetStats() map[string]any {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()

	stats := map[string]any{
		"supported_protocols": len(pm.adapters),
		"default_protocol":    pm.defaultProtocol,
		"auto_detection":      pm.autoDetection,
		"strict_validation":   pm.config.StrictValidation,
		"protocols":           pm.GetSupportedProtocols(),
	}

	// 添加验证器统计
	if pm.validator != nil {
		history := pm.validator.GetMessageHistory()
		stats["validation"] = map[string]any{
			"message_history_size": len(history),
			"max_history_size":     pm.validator.maxHistorySize,
		}
	}

	return stats
}

// ClearHistory 清空消息历史
func (pm *ProtocolManager) ClearHistory() {
	if pm.validator != nil {
		pm.validator.ClearHistory()
	}

	pm.logger.Debug("Message history cleared")
}

// getAdapter 获取适配器（线程安全）
func (pm *ProtocolManager) getAdapter(protocol ProtocolVersion) (ProtocolAdapter, bool) {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()

	adapter, exists := pm.adapters[protocol]
	return adapter, exists
}

// calculateConfidence 计算协议匹配置信度
func (pm *ProtocolManager) calculateConfidence(data []byte, adapter ProtocolAdapter) float64 {
	// 基础置信度
	confidence := 0.5

	// 尝试解析消息以提高置信度
	if message, err := adapter.ParseMessage(data); err == nil {
		confidence += 0.3

		// 如果消息有效，进一步提高置信度
		if message.IsValid() {
			confidence += 0.2
		}

		// 如果协议版本匹配，提高置信度
		if message.Version == adapter.GetVersion() {
			confidence += 0.1
		}
	}

	// 确保置信度在 0-1 范围内
	if confidence > 1.0 {
		confidence = 1.0
	}

	return confidence
}

// registerDefaultAdapters 注册默认适配器
func (pm *ProtocolManager) registerDefaultAdapters() {
	// 注册 GStreamer 适配器
	gstreamerConfig := DefaultAdapterConfig(ProtocolVersionGStreamer10)
	gstreamerAdapter := NewGStreamerAdapter(gstreamerConfig)
	pm.RegisterAdapter(ProtocolVersionGStreamer10, gstreamerAdapter)

	// 注册 Selkies 适配器
	selkiesConfig := DefaultAdapterConfig(ProtocolVersionSelkies)
	selkiesAdapter := NewSelkiesAdapter(selkiesConfig)
	pm.RegisterAdapter(ProtocolVersionSelkies, selkiesAdapter)

	pm.logger.Trace("Default protocol adapters registered: GStreamer, Selkies")
}

// UpdateConfig 更新配置
func (pm *ProtocolManager) UpdateConfig(config *ManagerConfig) error {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	if config == nil {
		return fmt.Errorf("config is nil")
	}

	// 验证配置
	if config.DefaultProtocol != "" {
		if _, exists := pm.adapters[config.DefaultProtocol]; !exists {
			return fmt.Errorf("default protocol not supported: %s", config.DefaultProtocol)
		}
	}

	// 更新配置
	pm.config = config
	pm.defaultProtocol = config.DefaultProtocol
	pm.autoDetection = config.AutoDetection

	// 更新验证器配置
	if config.ValidatorConfig != nil {
		pm.validator.SetConfig(config.ValidatorConfig)
	}

	pm.logger.Info("Protocol manager configuration updated")

	return nil
}

// GetConfig 获取当前配置
func (pm *ProtocolManager) GetConfig() *ManagerConfig {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()

	// 返回配置的副本
	configCopy := *pm.config
	return &configCopy
}
