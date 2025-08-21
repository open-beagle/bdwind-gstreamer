package webrtc

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/open-beagle/bdwind-gstreamer/internal/webrtc/protocol"
)

// ProtocolNegotiator 协议协商器
type ProtocolNegotiator struct {
	messageRouter  *MessageRouter
	config         *NegotiatorConfig
	detectionRules []DetectionRule
	fallbackChain  []protocol.ProtocolVersion
}

// NegotiatorConfig 协商器配置
type NegotiatorConfig struct {
	EnableAutoDetection  bool                       `json:"enable_auto_detection"`
	EnableProtocolSwitch bool                       `json:"enable_protocol_switch"`
	DefaultProtocol      protocol.ProtocolVersion   `json:"default_protocol"`
	FallbackProtocols    []protocol.ProtocolVersion `json:"fallback_protocols"`
	DetectionTimeout     time.Duration              `json:"detection_timeout"`
	MaxDetectionAttempts int                        `json:"max_detection_attempts"`
	ConfidenceThreshold  float64                    `json:"confidence_threshold"`
}

// DetectionRule 检测规则
type DetectionRule struct {
	Name       string                   `json:"name"`
	Protocol   protocol.ProtocolVersion `json:"protocol"`
	Patterns   []string                 `json:"patterns"`
	Confidence float64                  `json:"confidence"`
	Validator  func([]byte) bool        `json:"-"`
}

// NegotiationResult 协商结果
type NegotiationResult struct {
	SelectedProtocol protocol.ProtocolVersion `json:"selected_protocol"`
	Confidence       float64                  `json:"confidence"`
	DetectionMethod  string                   `json:"detection_method"`
	FallbackUsed     bool                     `json:"fallback_used"`
	Attempts         int                      `json:"attempts"`
	Errors           []string                 `json:"errors"`
}

// NewProtocolNegotiator 创建协议协商器
func NewProtocolNegotiator(messageRouter *MessageRouter, config *NegotiatorConfig) *ProtocolNegotiator {
	if config == nil {
		config = DefaultNegotiatorConfig()
	}

	negotiator := &ProtocolNegotiator{
		messageRouter: messageRouter,
		config:        config,
		fallbackChain: config.FallbackProtocols,
	}

	negotiator.initDetectionRules()
	return negotiator
}

// DefaultNegotiatorConfig 默认协商器配置
func DefaultNegotiatorConfig() *NegotiatorConfig {
	return &NegotiatorConfig{
		EnableAutoDetection:  true,
		EnableProtocolSwitch: true,
		DefaultProtocol:      protocol.ProtocolVersionGStreamer10,
		FallbackProtocols: []protocol.ProtocolVersion{
			protocol.ProtocolVersionGStreamer10,
			protocol.ProtocolVersionSelkies,
		},
		DetectionTimeout:     5 * time.Second,
		MaxDetectionAttempts: 3,
		ConfidenceThreshold:  0.7,
	}
}

// DetectProtocol 检测协议
func (pn *ProtocolNegotiator) DetectProtocol(data []byte) *NegotiationResult {
	result := &NegotiationResult{
		SelectedProtocol: pn.config.DefaultProtocol,
		Confidence:       0.0,
		DetectionMethod:  "default",
		FallbackUsed:     false,
		Attempts:         1,
		Errors:           make([]string, 0),
	}

	if !pn.config.EnableAutoDetection {
		return result
	}

	// 使用消息路由器进行基础检测
	detection := pn.messageRouter.DetectProtocol(data)
	if detection.Confidence >= pn.config.ConfidenceThreshold {
		result.SelectedProtocol = detection.Protocol
		result.Confidence = detection.Confidence
		result.DetectionMethod = "message_router"
		return result
	}

	// 使用自定义检测规则
	bestMatch := pn.applyDetectionRules(data)
	if bestMatch != nil && bestMatch.Confidence >= pn.config.ConfidenceThreshold {
		result.SelectedProtocol = bestMatch.Protocol
		result.Confidence = bestMatch.Confidence
		result.DetectionMethod = "custom_rules"
		return result
	}

	// 使用回退链
	fallbackProtocol := pn.selectFallbackProtocol(data)
	if fallbackProtocol != "" {
		result.SelectedProtocol = fallbackProtocol
		result.Confidence = 0.5 // 中等置信度
		result.DetectionMethod = "fallback"
		result.FallbackUsed = true
	}

	return result
}

// NegotiateProtocol 协商协议
func (pn *ProtocolNegotiator) NegotiateProtocol(clientProtocols []protocol.ProtocolVersion, serverProtocols []protocol.ProtocolVersion) *NegotiationResult {
	result := &NegotiationResult{
		SelectedProtocol: pn.config.DefaultProtocol,
		Confidence:       0.0,
		DetectionMethod:  "negotiation",
		FallbackUsed:     false,
		Attempts:         1,
		Errors:           make([]string, 0),
	}

	// 优先级顺序
	priorityOrder := []protocol.ProtocolVersion{
		protocol.ProtocolVersionGStreamer10,
		protocol.ProtocolVersionSelkies,
	}

	// 按优先级查找匹配的协议
	for _, preferred := range priorityOrder {
		clientSupports := pn.containsProtocol(clientProtocols, preferred)
		serverSupports := pn.containsProtocol(serverProtocols, preferred)

		if clientSupports && serverSupports {
			result.SelectedProtocol = preferred
			result.Confidence = 0.9
			result.DetectionMethod = "mutual_support"
			return result
		}
	}

	// 如果没有找到匹配的协议，尝试回退
	for _, fallback := range pn.config.FallbackProtocols {
		serverSupports := pn.containsProtocol(serverProtocols, fallback)
		if serverSupports {
			result.SelectedProtocol = fallback
			result.Confidence = 0.6
			result.DetectionMethod = "server_fallback"
			result.FallbackUsed = true
			return result
		}
	}

	return result
}

// SwitchProtocol 切换协议
func (pn *ProtocolNegotiator) SwitchProtocol(currentProtocol, targetProtocol protocol.ProtocolVersion, data []byte) ([]byte, error) {
	if !pn.config.EnableProtocolSwitch {
		return nil, fmt.Errorf("protocol switching is disabled")
	}

	if currentProtocol == targetProtocol {
		return data, nil // 无需切换
	}

	// 使用消息路由器进行协议转换
	convertedData, err := pn.messageRouter.ConvertMessage(data, currentProtocol, targetProtocol)
	if err != nil {
		return nil, fmt.Errorf("failed to convert protocol from %s to %s: %w", currentProtocol, targetProtocol, err)
	}

	log.Printf("🔄 Protocol switched: %s -> %s, size: %d -> %d bytes",
		currentProtocol, targetProtocol, len(data), len(convertedData))

	return convertedData, nil
}

// ValidateProtocolCompatibility 验证协议兼容性
func (pn *ProtocolNegotiator) ValidateProtocolCompatibility(data []byte, expectedProtocol protocol.ProtocolVersion) (bool, error) {
	detection := pn.DetectProtocol(data)

	if detection.SelectedProtocol == expectedProtocol {
		return true, nil
	}

	// 检查是否可以转换
	if pn.config.EnableProtocolSwitch {
		_, err := pn.SwitchProtocol(detection.SelectedProtocol, expectedProtocol, data)
		if err == nil {
			return true, nil
		}
		return false, fmt.Errorf("protocol mismatch and conversion failed: detected=%s, expected=%s, error=%v",
			detection.SelectedProtocol, expectedProtocol, err)
	}

	return false, fmt.Errorf("protocol mismatch: detected=%s, expected=%s",
		detection.SelectedProtocol, expectedProtocol)
}

// initDetectionRules 初始化检测规则
func (pn *ProtocolNegotiator) initDetectionRules() {
	pn.detectionRules = []DetectionRule{
		{
			Name:       "Selkies HELLO Text",
			Protocol:   protocol.ProtocolVersionSelkies,
			Patterns:   []string{"HELLO ", "ERROR "},
			Confidence: 0.95,
			Validator: func(data []byte) bool {
				text := string(data)
				return strings.HasPrefix(text, "HELLO ") || strings.HasPrefix(text, "ERROR ")
			},
		},
		{
			Name:       "Selkies JSON SDP",
			Protocol:   protocol.ProtocolVersionSelkies,
			Patterns:   []string{`"sdp":`},
			Confidence: 0.85,
			Validator: func(data []byte) bool {
				var msg map[string]any
				if err := json.Unmarshal(data, &msg); err != nil {
					return false
				}
				_, hasSDP := msg["sdp"]
				return hasSDP
			},
		},
		{
			Name:       "Selkies JSON ICE",
			Protocol:   protocol.ProtocolVersionSelkies,
			Patterns:   []string{`"ice":`},
			Confidence: 0.85,
			Validator: func(data []byte) bool {
				var msg map[string]any
				if err := json.Unmarshal(data, &msg); err != nil {
					return false
				}
				_, hasICE := msg["ice"]
				return hasICE
			},
		},
		{
			Name:       "GStreamer Standard Version",
			Protocol:   protocol.ProtocolVersionGStreamer10,
			Patterns:   []string{`"version":`, `"gstreamer-1.0"`},
			Confidence: 0.90,
			Validator: func(data []byte) bool {
				var msg map[string]any
				if err := json.Unmarshal(data, &msg); err != nil {
					return false
				}
				version, hasVersion := msg["version"]
				if !hasVersion {
					return false
				}
				versionStr, ok := version.(string)
				return ok && strings.Contains(versionStr, "gstreamer")
			},
		},
		{
			Name:       "GStreamer Standard Metadata",
			Protocol:   protocol.ProtocolVersionGStreamer10,
			Patterns:   []string{`"metadata":`, `"protocol":`},
			Confidence: 0.80,
			Validator: func(data []byte) bool {
				var msg map[string]any
				if err := json.Unmarshal(data, &msg); err != nil {
					return false
				}
				metadata, hasMetadata := msg["metadata"]
				if !hasMetadata {
					return false
				}
				metadataMap, ok := metadata.(map[string]any)
				if !ok {
					return false
				}
				_, hasProtocol := metadataMap["protocol"]
				return hasProtocol
			},
		},
		{
			Name:       "Standard Message Types",
			Protocol:   protocol.ProtocolVersionGStreamer10,
			Patterns:   []string{`"type":"ping"`, `"type":"pong"`, `"type":"request-offer"`},
			Confidence: 0.75,
			Validator: func(data []byte) bool {
				var msg map[string]any
				if err := json.Unmarshal(data, &msg); err != nil {
					return false
				}
				msgType, hasType := msg["type"]
				if !hasType {
					return false
				}
				typeStr, ok := msgType.(string)
				if !ok {
					return false
				}
				standardTypes := []string{"ping", "pong", "request-offer", "protocol-negotiation"}
				for _, standardType := range standardTypes {
					if typeStr == standardType {
						return true
					}
				}
				return false
			},
		},
	}
}

// applyDetectionRules 应用检测规则
func (pn *ProtocolNegotiator) applyDetectionRules(data []byte) *DetectionRule {
	var bestMatch *DetectionRule
	highestConfidence := 0.0

	text := string(data)

	for i := range pn.detectionRules {
		rule := &pn.detectionRules[i]

		// 检查模式匹配
		patternMatches := 0
		for _, pattern := range rule.Patterns {
			if strings.Contains(text, pattern) {
				patternMatches++
			}
		}

		// 如果有模式匹配，进行验证
		if patternMatches > 0 && rule.Validator != nil {
			if rule.Validator(data) {
				confidence := rule.Confidence * (float64(patternMatches) / float64(len(rule.Patterns)))
				if confidence > highestConfidence {
					highestConfidence = confidence
					bestMatch = rule
				}
			}
		}
	}

	return bestMatch
}

// selectFallbackProtocol 选择回退协议
func (pn *ProtocolNegotiator) selectFallbackProtocol(data []byte) protocol.ProtocolVersion {
	// 尝试每个回退协议
	for _, fallback := range pn.config.FallbackProtocols {
		// 检查消息路由器是否支持该协议
		if pn.messageRouter.IsProtocolSupported(fallback) {
			return fallback
		}
	}

	return pn.config.DefaultProtocol
}

// containsProtocol 检查协议列表是否包含指定协议
func (pn *ProtocolNegotiator) containsProtocol(protocols []protocol.ProtocolVersion, target protocol.ProtocolVersion) bool {
	for _, p := range protocols {
		if p == target {
			return true
		}
	}
	return false
}

// GetSupportedProtocols 获取支持的协议列表
func (pn *ProtocolNegotiator) GetSupportedProtocols() []protocol.ProtocolVersion {
	return pn.messageRouter.GetSupportedProtocols()
}

// UpdateConfig 更新配置
func (pn *ProtocolNegotiator) UpdateConfig(config *NegotiatorConfig) {
	if config != nil {
		pn.config = config
		pn.fallbackChain = config.FallbackProtocols
	}
}

// GetConfig 获取当前配置
func (pn *ProtocolNegotiator) GetConfig() *NegotiatorConfig {
	configCopy := *pn.config
	return &configCopy
}

// GetStats 获取统计信息
func (pn *ProtocolNegotiator) GetStats() map[string]any {
	return map[string]any{
		"config":              pn.config,
		"detection_rules":     len(pn.detectionRules),
		"fallback_chain":      pn.fallbackChain,
		"supported_protocols": pn.GetSupportedProtocols(),
	}
}

// AddDetectionRule 添加检测规则
func (pn *ProtocolNegotiator) AddDetectionRule(rule DetectionRule) {
	pn.detectionRules = append(pn.detectionRules, rule)
}

// RemoveDetectionRule 移除检测规则
func (pn *ProtocolNegotiator) RemoveDetectionRule(name string) {
	for i, rule := range pn.detectionRules {
		if rule.Name == name {
			pn.detectionRules = append(pn.detectionRules[:i], pn.detectionRules[i+1:]...)
			break
		}
	}
}

// GetDetectionRules 获取所有检测规则
func (pn *ProtocolNegotiator) GetDetectionRules() []DetectionRule {
	rules := make([]DetectionRule, len(pn.detectionRules))
	copy(rules, pn.detectionRules)
	return rules
}
