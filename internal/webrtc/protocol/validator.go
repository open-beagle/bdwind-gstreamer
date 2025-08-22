package protocol

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// MessageValidator 消息验证器
type MessageValidator struct {
	config           *ValidatorConfig
	formatValidators map[MessageType]FormatValidator
	contentRules     map[MessageType][]ContentRule
	sequenceRules    []SequenceRule
	messageHistory   []MessageHistoryEntry
	maxHistorySize   int
}

// ValidatorConfig 验证器配置
type ValidatorConfig struct {
	StrictMode          bool              `json:"strict_mode"`
	MaxMessageSize      int               `json:"max_message_size"`
	MaxHistorySize      int               `json:"max_history_size"`
	EnableSequenceCheck bool              `json:"enable_sequence_check"`
	AllowedProtocols    []ProtocolVersion `json:"allowed_protocols"`
	CustomRules         map[string]any    `json:"custom_rules"`
}

// FormatValidator 格式验证器
type FormatValidator struct {
	RequiredFields []string                         `json:"required_fields"`
	OptionalFields []string                         `json:"optional_fields"`
	DataSchema     *DataSchema                      `json:"data_schema"`
	CustomCheck    func(msg *StandardMessage) error `json:"-"`
}

// DataSchema 数据模式
type DataSchema struct {
	Type       string                     `json:"type"`
	Properties map[string]*PropertySchema `json:"properties"`
	Required   []string                   `json:"required"`
}

// PropertySchema 属性模式
type PropertySchema struct {
	Type        string   `json:"type"`
	Format      string   `json:"format,omitempty"`
	Pattern     string   `json:"pattern,omitempty"`
	MinLength   *int     `json:"min_length,omitempty"`
	MaxLength   *int     `json:"max_length,omitempty"`
	Minimum     *float64 `json:"minimum,omitempty"`
	Maximum     *float64 `json:"maximum,omitempty"`
	Enum        []any    `json:"enum,omitempty"`
	Description string   `json:"description,omitempty"`
}

// ContentRule 内容验证规则
type ContentRule struct {
	Name        string                           `json:"name"`
	Description string                           `json:"description"`
	Validator   func(msg *StandardMessage) error `json:"-"`
	Severity    RuleSeverity                     `json:"severity"`
	Enabled     bool                             `json:"enabled"`
}

// SequenceRule 序列验证规则
type SequenceRule struct {
	Name        string                                    `json:"name"`
	Description string                                    `json:"description"`
	Validator   func(history []MessageHistoryEntry) error `json:"-"`
	Severity    RuleSeverity                              `json:"severity"`
	Enabled     bool                                      `json:"enabled"`
}

// MessageHistoryEntry 消息历史条目
type MessageHistoryEntry struct {
	Message   *StandardMessage  `json:"message"`
	Timestamp time.Time         `json:"timestamp"`
	PeerID    string            `json:"peer_id"`
	Valid     bool              `json:"valid"`
	Errors    []ValidationError `json:"errors"`
}

// ValidationError 验证错误
type ValidationError struct {
	Code       string       `json:"code"`
	Message    string       `json:"message"`
	Field      string       `json:"field,omitempty"`
	Value      any          `json:"value,omitempty"`
	Severity   RuleSeverity `json:"severity"`
	Suggestion string       `json:"suggestion,omitempty"`
}

// RuleSeverity 规则严重性
type RuleSeverity string

const (
	SeverityError   RuleSeverity = "error"
	SeverityWarning RuleSeverity = "warning"
	SeverityInfo    RuleSeverity = "info"
)

// ValidationResult 验证结果
type ValidationResult struct {
	Valid    bool              `json:"valid"`
	Errors   []ValidationError `json:"errors"`
	Warnings []ValidationError `json:"warnings"`
	Info     []ValidationError `json:"info"`
	Summary  ValidationSummary `json:"summary"`
}

// ValidationSummary 验证摘要
type ValidationSummary struct {
	TotalChecks   int `json:"total_checks"`
	PassedChecks  int `json:"passed_checks"`
	FailedChecks  int `json:"failed_checks"`
	WarningChecks int `json:"warning_checks"`
	InfoChecks    int `json:"info_checks"`
}

// NewMessageValidator 创建消息验证器
func NewMessageValidator(config *ValidatorConfig) *MessageValidator {
	if config == nil {
		config = DefaultValidatorConfig()
	}

	validator := &MessageValidator{
		config:           config,
		formatValidators: make(map[MessageType]FormatValidator),
		contentRules:     make(map[MessageType][]ContentRule),
		sequenceRules:    make([]SequenceRule, 0),
		messageHistory:   make([]MessageHistoryEntry, 0),
		maxHistorySize:   config.MaxHistorySize,
	}

	validator.initDefaultValidators()
	validator.initDefaultContentRules()
	validator.initDefaultSequenceRules()

	return validator
}

// DefaultValidatorConfig 默认验证器配置
func DefaultValidatorConfig() *ValidatorConfig {
	return &ValidatorConfig{
		StrictMode:          true,
		MaxMessageSize:      64 * 1024, // 64KB
		MaxHistorySize:      100,
		EnableSequenceCheck: true,
		AllowedProtocols: []ProtocolVersion{
			ProtocolVersionGStreamer10,
			ProtocolVersionSelkies,
		},
		CustomRules: make(map[string]any),
	}
}

// ValidateFormat 验证消息格式
func (v *MessageValidator) ValidateFormat(msg *StandardMessage) *ValidationResult {
	result := &ValidationResult{
		Valid:    true,
		Errors:   make([]ValidationError, 0),
		Warnings: make([]ValidationError, 0),
		Info:     make([]ValidationError, 0),
	}

	// 基本格式验证
	v.validateBasicFormat(msg, result)

	// 特定消息类型格式验证（只有在消息不为空时才进行）
	if msg != nil {
		if validator, exists := v.formatValidators[msg.Type]; exists {
			v.applyFormatValidator(msg, validator, result)
		}
	}

	v.updateValidationSummary(result)
	return result
}

// ValidateContent 验证消息内容
func (v *MessageValidator) ValidateContent(msg *StandardMessage) *ValidationResult {
	result := &ValidationResult{
		Valid:    true,
		Errors:   make([]ValidationError, 0),
		Warnings: make([]ValidationError, 0),
		Info:     make([]ValidationError, 0),
	}

	// 应用内容验证规则
	if rules, exists := v.contentRules[msg.Type]; exists {
		for _, rule := range rules {
			if rule.Enabled {
				if err := rule.Validator(msg); err != nil {
					validationError := ValidationError{
						Code:     fmt.Sprintf("CONTENT_%s", strings.ToUpper(rule.Name)),
						Message:  err.Error(),
						Severity: rule.Severity,
					}

					switch rule.Severity {
					case SeverityError:
						result.Errors = append(result.Errors, validationError)
						result.Valid = false
					case SeverityWarning:
						result.Warnings = append(result.Warnings, validationError)
					case SeverityInfo:
						result.Info = append(result.Info, validationError)
					}
				}
			}
		}
	}

	v.updateValidationSummary(result)
	return result
}

// ValidateSequence 验证消息序列
func (v *MessageValidator) ValidateSequence(msg *StandardMessage) *ValidationResult {
	result := &ValidationResult{
		Valid:    true,
		Errors:   make([]ValidationError, 0),
		Warnings: make([]ValidationError, 0),
		Info:     make([]ValidationError, 0),
	}

	if !v.config.EnableSequenceCheck {
		return result
	}

	// 添加消息到历史记录
	v.addToHistory(msg, result)

	// 应用序列验证规则
	for _, rule := range v.sequenceRules {
		if rule.Enabled {
			if err := rule.Validator(v.messageHistory); err != nil {
				validationError := ValidationError{
					Code:     fmt.Sprintf("SEQUENCE_%s", strings.ToUpper(rule.Name)),
					Message:  err.Error(),
					Severity: rule.Severity,
				}

				switch rule.Severity {
				case SeverityError:
					result.Errors = append(result.Errors, validationError)
					result.Valid = false
				case SeverityWarning:
					result.Warnings = append(result.Warnings, validationError)
				case SeverityInfo:
					result.Info = append(result.Info, validationError)
				}
			}
		}
	}

	v.updateValidationSummary(result)
	return result
}

// ValidateMessage 完整验证消息
func (v *MessageValidator) ValidateMessage(msg *StandardMessage) *ValidationResult {
	// 合并所有验证结果
	formatResult := v.ValidateFormat(msg)
	contentResult := v.ValidateContent(msg)
	sequenceResult := v.ValidateSequence(msg)

	combinedResult := &ValidationResult{
		Valid:    formatResult.Valid && contentResult.Valid && sequenceResult.Valid,
		Errors:   append(append(formatResult.Errors, contentResult.Errors...), sequenceResult.Errors...),
		Warnings: append(append(formatResult.Warnings, contentResult.Warnings...), sequenceResult.Warnings...),
		Info:     append(append(formatResult.Info, contentResult.Info...), sequenceResult.Info...),
	}

	v.updateValidationSummary(combinedResult)
	return combinedResult
}

// validateBasicFormat 验证基本格式
func (v *MessageValidator) validateBasicFormat(msg *StandardMessage, result *ValidationResult) {
	// 验证消息不为空
	if msg == nil {
		result.Errors = append(result.Errors, ValidationError{
			Code:     "NULL_MESSAGE",
			Message:  "Message is null",
			Severity: SeverityError,
		})
		result.Valid = false
		return
	}

	// 验证消息类型
	if msg.Type == "" {
		result.Errors = append(result.Errors, ValidationError{
			Code:     "MISSING_TYPE",
			Message:  "Message type is required",
			Field:    "type",
			Severity: SeverityError,
		})
		result.Valid = false
	}

	// 验证消息ID
	if msg.ID == "" {
		result.Errors = append(result.Errors, ValidationError{
			Code:     "MISSING_ID",
			Message:  "Message ID is required",
			Field:    "id",
			Severity: SeverityError,
		})
		result.Valid = false
	}

	// 验证时间戳
	if msg.Timestamp <= 0 {
		result.Errors = append(result.Errors, ValidationError{
			Code:     "INVALID_TIMESTAMP",
			Message:  "Message timestamp must be positive",
			Field:    "timestamp",
			Value:    msg.Timestamp,
			Severity: SeverityError,
		})
		result.Valid = false
	}

	// 验证协议版本
	if v.config.StrictMode {
		validProtocol := false
		for _, allowedProtocol := range v.config.AllowedProtocols {
			if msg.Version == allowedProtocol {
				validProtocol = true
				break
			}
		}

		if !validProtocol {
			result.Errors = append(result.Errors, ValidationError{
				Code:     "UNSUPPORTED_PROTOCOL",
				Message:  fmt.Sprintf("Unsupported protocol version: %s", msg.Version),
				Field:    "version",
				Value:    msg.Version,
				Severity: SeverityError,
			})
			result.Valid = false
		}
	}
}

// applyFormatValidator 应用格式验证器
func (v *MessageValidator) applyFormatValidator(msg *StandardMessage, validator FormatValidator, result *ValidationResult) {
	// 验证数据模式
	if validator.DataSchema != nil {
		v.validateDataSchema(msg.Data, validator.DataSchema, result)
	}

	// 应用自定义检查
	if validator.CustomCheck != nil {
		if err := validator.CustomCheck(msg); err != nil {
			result.Errors = append(result.Errors, ValidationError{
				Code:     "CUSTOM_FORMAT_CHECK",
				Message:  err.Error(),
				Severity: SeverityError,
			})
			result.Valid = false
		}
	}
}

// validateDataSchema 验证数据模式
func (v *MessageValidator) validateDataSchema(data any, schema *DataSchema, result *ValidationResult) {
	if data == nil && len(schema.Required) > 0 {
		result.Errors = append(result.Errors, ValidationError{
			Code:     "MISSING_DATA",
			Message:  "Data is required but missing",
			Field:    "data",
			Severity: SeverityError,
		})
		result.Valid = false
		return
	}

	if data == nil {
		return
	}

	// 转换为 map 进行验证
	var dataMap map[string]any
	if err := convertToMap(data, &dataMap); err != nil {
		result.Errors = append(result.Errors, ValidationError{
			Code:     "INVALID_DATA_FORMAT",
			Message:  "Data must be an object",
			Field:    "data",
			Severity: SeverityError,
		})
		result.Valid = false
		return
	}

	// 验证必需字段
	for _, requiredField := range schema.Required {
		if _, exists := dataMap[requiredField]; !exists {
			result.Errors = append(result.Errors, ValidationError{
				Code:     "MISSING_REQUIRED_FIELD",
				Message:  fmt.Sprintf("Required field '%s' is missing", requiredField),
				Field:    fmt.Sprintf("data.%s", requiredField),
				Severity: SeverityError,
			})
			result.Valid = false
		}
	}

	// 验证属性
	for fieldName, fieldValue := range dataMap {
		if propertySchema, exists := schema.Properties[fieldName]; exists {
			v.validateProperty(fieldName, fieldValue, propertySchema, result)
		}
	}
}

// validateProperty 验证属性
func (v *MessageValidator) validateProperty(fieldName string, value any, schema *PropertySchema, result *ValidationResult) {
	// 验证类型
	if !v.isValidType(value, schema.Type) {
		result.Errors = append(result.Errors, ValidationError{
			Code:     "INVALID_TYPE",
			Message:  fmt.Sprintf("Field '%s' must be of type %s", fieldName, schema.Type),
			Field:    fmt.Sprintf("data.%s", fieldName),
			Value:    value,
			Severity: SeverityError,
		})
		result.Valid = false
		return
	}

	// 验证字符串属性
	if schema.Type == "string" {
		v.validateStringProperty(fieldName, value.(string), schema, result)
	}

	// 验证数字属性
	if schema.Type == "number" || schema.Type == "integer" {
		v.validateNumberProperty(fieldName, value, schema, result)
	}

	// 验证枚举
	if len(schema.Enum) > 0 {
		v.validateEnumProperty(fieldName, value, schema, result)
	}
}

// validateStringProperty 验证字符串属性
func (v *MessageValidator) validateStringProperty(fieldName, value string, schema *PropertySchema, result *ValidationResult) {
	// 验证长度
	if schema.MinLength != nil && len(value) < *schema.MinLength {
		result.Errors = append(result.Errors, ValidationError{
			Code:     "STRING_TOO_SHORT",
			Message:  fmt.Sprintf("Field '%s' must be at least %d characters", fieldName, *schema.MinLength),
			Field:    fmt.Sprintf("data.%s", fieldName),
			Value:    value,
			Severity: SeverityError,
		})
		result.Valid = false
	}

	if schema.MaxLength != nil && len(value) > *schema.MaxLength {
		result.Errors = append(result.Errors, ValidationError{
			Code:     "STRING_TOO_LONG",
			Message:  fmt.Sprintf("Field '%s' must be at most %d characters", fieldName, *schema.MaxLength),
			Field:    fmt.Sprintf("data.%s", fieldName),
			Value:    value,
			Severity: SeverityError,
		})
		result.Valid = false
	}

	// 验证模式
	if schema.Pattern != "" {
		if matched, err := regexp.MatchString(schema.Pattern, value); err != nil || !matched {
			result.Errors = append(result.Errors, ValidationError{
				Code:     "PATTERN_MISMATCH",
				Message:  fmt.Sprintf("Field '%s' does not match required pattern", fieldName),
				Field:    fmt.Sprintf("data.%s", fieldName),
				Value:    value,
				Severity: SeverityError,
			})
			result.Valid = false
		}
	}
}

// validateNumberProperty 验证数字属性
func (v *MessageValidator) validateNumberProperty(fieldName string, value any, schema *PropertySchema, result *ValidationResult) {
	var numValue float64

	switch v := value.(type) {
	case float64:
		numValue = v
	case int:
		numValue = float64(v)
	case int64:
		numValue = float64(v)
	default:
		result.Errors = append(result.Errors, ValidationError{
			Code:     "INVALID_NUMBER",
			Message:  fmt.Sprintf("Field '%s' is not a valid number", fieldName),
			Field:    fmt.Sprintf("data.%s", fieldName),
			Value:    value,
			Severity: SeverityError,
		})
		result.Valid = false
		return
	}

	// 验证范围
	if schema.Minimum != nil && numValue < *schema.Minimum {
		result.Errors = append(result.Errors, ValidationError{
			Code:     "NUMBER_TOO_SMALL",
			Message:  fmt.Sprintf("Field '%s' must be at least %f", fieldName, *schema.Minimum),
			Field:    fmt.Sprintf("data.%s", fieldName),
			Value:    value,
			Severity: SeverityError,
		})
		result.Valid = false
	}

	if schema.Maximum != nil && numValue > *schema.Maximum {
		result.Errors = append(result.Errors, ValidationError{
			Code:     "NUMBER_TOO_LARGE",
			Message:  fmt.Sprintf("Field '%s' must be at most %f", fieldName, *schema.Maximum),
			Field:    fmt.Sprintf("data.%s", fieldName),
			Value:    value,
			Severity: SeverityError,
		})
		result.Valid = false
	}
}

// validateEnumProperty 验证枚举属性
func (v *MessageValidator) validateEnumProperty(fieldName string, value any, schema *PropertySchema, result *ValidationResult) {
	for _, enumValue := range schema.Enum {
		if value == enumValue {
			return
		}
	}

	result.Errors = append(result.Errors, ValidationError{
		Code:     "INVALID_ENUM_VALUE",
		Message:  fmt.Sprintf("Field '%s' must be one of the allowed values", fieldName),
		Field:    fmt.Sprintf("data.%s", fieldName),
		Value:    value,
		Severity: SeverityError,
	})
	result.Valid = false
}

// isValidType 检查类型是否有效
func (v *MessageValidator) isValidType(value any, expectedType string) bool {
	switch expectedType {
	case "string":
		_, ok := value.(string)
		return ok
	case "number":
		switch value.(type) {
		case float64, int, int64:
			return true
		}
		return false
	case "integer":
		switch value.(type) {
		case int, int64:
			return true
		case float64:
			// 检查是否为整数
			f := value.(float64)
			return f == float64(int64(f))
		}
		return false
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "object":
		_, ok := value.(map[string]any)
		return ok
	case "array":
		_, ok := value.([]any)
		return ok
	}
	return false
}

// addToHistory 添加到历史记录
func (v *MessageValidator) addToHistory(msg *StandardMessage, result *ValidationResult) {
	entry := MessageHistoryEntry{
		Message:   msg,
		Timestamp: time.Now(),
		PeerID:    msg.PeerID,
		Valid:     result.Valid,
		Errors:    result.Errors,
	}

	v.messageHistory = append(v.messageHistory, entry)

	// 限制历史记录大小
	if len(v.messageHistory) > v.maxHistorySize {
		v.messageHistory = v.messageHistory[1:]
	}
}

// updateValidationSummary 更新验证摘要
func (v *MessageValidator) updateValidationSummary(result *ValidationResult) {
	result.Summary.TotalChecks = 1
	result.Summary.FailedChecks = len(result.Errors)
	result.Summary.WarningChecks = len(result.Warnings)
	result.Summary.InfoChecks = len(result.Info)

	if result.Valid {
		result.Summary.PassedChecks = 1
		result.Summary.FailedChecks = 0
	} else {
		result.Summary.PassedChecks = 0
		result.Summary.FailedChecks = 1
	}
}

// convertToMap 转换为 map
func convertToMap(data any, target *map[string]any) error {
	dataBytes, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal data: %w", err)
	}

	if err := json.Unmarshal(dataBytes, target); err != nil {
		return fmt.Errorf("failed to unmarshal data: %w", err)
	}

	return nil
}

// GetMessageHistory 获取消息历史
func (v *MessageValidator) GetMessageHistory() []MessageHistoryEntry {
	return append([]MessageHistoryEntry(nil), v.messageHistory...)
}

// ClearHistory 清空历史记录
func (v *MessageValidator) ClearHistory() {
	v.messageHistory = make([]MessageHistoryEntry, 0)
}

// AddContentRule 添加内容规则
func (v *MessageValidator) AddContentRule(msgType MessageType, rule ContentRule) {
	if v.contentRules[msgType] == nil {
		v.contentRules[msgType] = make([]ContentRule, 0)
	}
	v.contentRules[msgType] = append(v.contentRules[msgType], rule)
}

// AddSequenceRule 添加序列规则
func (v *MessageValidator) AddSequenceRule(rule SequenceRule) {
	v.sequenceRules = append(v.sequenceRules, rule)
}

// SetConfig 设置配置
func (v *MessageValidator) SetConfig(config *ValidatorConfig) {
	v.config = config
	v.maxHistorySize = config.MaxHistorySize
}
