package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
	"github.com/sirupsen/logrus"
)

// ConfigMigrationTool provides tools for migrating between configuration formats
type ConfigMigrationTool struct {
	logger *logrus.Entry
}

// NewConfigMigrationTool creates a new configuration migration tool
func NewConfigMigrationTool() *ConfigMigrationTool {
	return &ConfigMigrationTool{
		logger: logrus.WithField("component", "config-migration"),
	}
}

// MigrationResult contains the result of a configuration migration
type MigrationResult struct {
	Success        bool                   `json:"success"`
	SourceFormat   string                 `json:"source_format"`
	TargetFormat   string                 `json:"target_format"`
	SourceFile     string                 `json:"source_file"`
	TargetFile     string                 `json:"target_file"`
	BackupFile     string                 `json:"backup_file,omitempty"`
	Issues         []string               `json:"issues,omitempty"`
	Warnings       []string               `json:"warnings,omitempty"`
	MigrationTime  time.Time              `json:"migration_time"`
	ConfigSummary  map[string]interface{} `json:"config_summary"`
}

// DetectConfigFormat detects the format of a configuration file
func (m *ConfigMigrationTool) DetectConfigFormat(filename string) (string, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return "", fmt.Errorf("failed to read config file: %w", err)
	}

	// Try to parse as different formats
	
	// Try SimpleConfig first
	var simpleConfig SimpleConfig
	if err := yaml.Unmarshal(data, &simpleConfig); err == nil {
		// Check if it has SimpleConfig-specific fields
		if simpleConfig.Implementation.Type != "" || simpleConfig.Lifecycle.ShutdownTimeout != 0 {
			return "simple", nil
		}
	}

	// Try full Config
	var fullConfig Config
	if err := yaml.Unmarshal(data, &fullConfig); err == nil {
		// Check if it has full Config-specific fields
		if fullConfig.Lifecycle.ShutdownTimeout != 0 || len(fullConfig.Implementation.Type) > 0 {
			return "full", nil
		}
	}

	// Try legacy Config
	var legacyConfig LegacyConfig
	if err := yaml.Unmarshal(data, &legacyConfig); err == nil {
		// Check if it has legacy-specific structure
		if legacyConfig.Web.Host != "" || legacyConfig.Capture.Width != 0 {
			return "legacy", nil
		}
	}

	return "unknown", fmt.Errorf("unable to detect configuration format")
}

// ValidateConfigFile validates a configuration file and returns issues
func (m *ConfigMigrationTool) ValidateConfigFile(filename string) ([]string, []string, error) {
	format, err := m.DetectConfigFormat(filename)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to detect config format: %w", err)
	}

	var issues []string
	var warnings []string

	switch format {
	case "simple":
		cfg, err := LoadSimpleConfigFromFile(filename)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to load simple config: %w", err)
		}
		
		checker := NewConfigCompatibilityChecker()
		issues = checker.CheckSimpleConfigCompatibility(cfg)
		
		// Add warnings for simple config
		if cfg.GStreamer != nil && cfg.GStreamer.Encoding.UseHardware {
			warnings = append(warnings, "hardware encoding may not be available on all systems")
		}
		
	case "full":
		cfg, err := LoadConfigFromFile(filename)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to load full config: %w", err)
		}
		
		if err := cfg.Validate(); err != nil {
			issues = append(issues, err.Error())
		}
		
		checker := NewConfigCompatibilityChecker()
		compatIssues := checker.CheckFullConfigCompatibility(cfg)
		issues = append(issues, compatIssues...)
		
	case "legacy":
		// Legacy configs have minimal validation
		warnings = append(warnings, "legacy configuration format detected - consider migrating to newer format")
		
	default:
		issues = append(issues, "unknown configuration format")
	}

	return issues, warnings, nil
}

// MigrateConfig migrates a configuration file from one format to another
func (m *ConfigMigrationTool) MigrateConfig(sourceFile, targetFile, targetFormat string, createBackup bool) (*MigrationResult, error) {
	result := &MigrationResult{
		SourceFile:    sourceFile,
		TargetFile:    targetFile,
		TargetFormat:  targetFormat,
		MigrationTime: time.Now(),
		Issues:        make([]string, 0),
		Warnings:      make([]string, 0),
	}

	m.logger.Infof("Starting config migration: %s -> %s (format: %s)", sourceFile, targetFile, targetFormat)

	// Detect source format
	sourceFormat, err := m.DetectConfigFormat(sourceFile)
	if err != nil {
		result.Issues = append(result.Issues, fmt.Sprintf("failed to detect source format: %v", err))
		return result, err
	}
	result.SourceFormat = sourceFormat

	// Validate source config
	issues, warnings, err := m.ValidateConfigFile(sourceFile)
	if err != nil {
		result.Issues = append(result.Issues, fmt.Sprintf("source validation failed: %v", err))
		return result, err
	}
	result.Issues = append(result.Issues, issues...)
	result.Warnings = append(result.Warnings, warnings...)

	// Create backup if requested
	if createBackup {
		backupFile := sourceFile + ".backup." + time.Now().Format("20060102-150405")
		if err := m.createBackup(sourceFile, backupFile); err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("failed to create backup: %v", err))
		} else {
			result.BackupFile = backupFile
			m.logger.Infof("Created backup: %s", backupFile)
		}
	}

	// Load source configuration
	sourceConfig, err := m.loadAnyConfig(sourceFile, sourceFormat)
	if err != nil {
		result.Issues = append(result.Issues, fmt.Sprintf("failed to load source config: %v", err))
		return result, err
	}

	// Convert to target format
	targetConfig, err := m.convertConfig(sourceConfig, sourceFormat, targetFormat)
	if err != nil {
		result.Issues = append(result.Issues, fmt.Sprintf("failed to convert config: %v", err))
		return result, err
	}

	// Save target configuration
	if err := m.saveConfig(targetConfig, targetFile, targetFormat); err != nil {
		result.Issues = append(result.Issues, fmt.Sprintf("failed to save target config: %v", err))
		return result, err
	}

	// Generate config summary
	result.ConfigSummary = m.generateConfigSummary(targetConfig, targetFormat)

	result.Success = len(result.Issues) == 0
	m.logger.Infof("Config migration completed: success=%v, issues=%d, warnings=%d", 
		result.Success, len(result.Issues), len(result.Warnings))

	return result, nil
}

// createBackup creates a backup of the source file
func (m *ConfigMigrationTool) createBackup(sourceFile, backupFile string) error {
	data, err := os.ReadFile(sourceFile)
	if err != nil {
		return fmt.Errorf("failed to read source file: %w", err)
	}

	if err := os.WriteFile(backupFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write backup file: %w", err)
	}

	return nil
}

// loadAnyConfig loads a configuration file in any supported format
func (m *ConfigMigrationTool) loadAnyConfig(filename, format string) (interface{}, error) {
	switch format {
	case "simple":
		return LoadSimpleConfigFromFile(filename)
	case "full":
		return LoadConfigFromFile(filename)
	case "legacy":
		data, err := os.ReadFile(filename)
		if err != nil {
			return nil, err
		}
		return loadLegacyConfig(data)
	default:
		return nil, fmt.Errorf("unsupported source format: %s", format)
	}
}

// convertConfig converts configuration from one format to another
func (m *ConfigMigrationTool) convertConfig(sourceConfig interface{}, sourceFormat, targetFormat string) (interface{}, error) {
	if sourceFormat == targetFormat {
		return sourceConfig, nil
	}

	switch targetFormat {
	case "simple":
		return m.convertToSimple(sourceConfig, sourceFormat)
	case "full":
		return m.convertToFull(sourceConfig, sourceFormat)
	default:
		return nil, fmt.Errorf("unsupported target format: %s", targetFormat)
	}
}

// convertToSimple converts any config format to SimpleConfig
func (m *ConfigMigrationTool) convertToSimple(sourceConfig interface{}, sourceFormat string) (*SimpleConfig, error) {
	switch sourceFormat {
	case "full":
		if cfg, ok := sourceConfig.(*Config); ok {
			return ConvertToSimpleConfig(cfg), nil
		}
	case "legacy":
		if cfg, ok := sourceConfig.(*LegacyConfig); ok {
			return convertLegacyToSimpleConfig(cfg), nil
		}
	case "simple":
		if cfg, ok := sourceConfig.(*SimpleConfig); ok {
			return cfg, nil
		}
	}
	return nil, fmt.Errorf("failed to convert %s config to simple format", sourceFormat)
}

// convertToFull converts any config format to full Config
func (m *ConfigMigrationTool) convertToFull(sourceConfig interface{}, sourceFormat string) (*Config, error) {
	switch sourceFormat {
	case "simple":
		if cfg, ok := sourceConfig.(*SimpleConfig); ok {
			adapter := NewSimpleConfigAdapter(cfg)
			return adapter.GetFullConfig(), nil
		}
	case "legacy":
		if cfg, ok := sourceConfig.(*LegacyConfig); ok {
			return convertLegacyConfig(cfg), nil
		}
	case "full":
		if cfg, ok := sourceConfig.(*Config); ok {
			return cfg, nil
		}
	}
	return nil, fmt.Errorf("failed to convert %s config to full format", sourceFormat)
}

// saveConfig saves a configuration in the specified format
func (m *ConfigMigrationTool) saveConfig(config interface{}, filename, format string) error {
	// Ensure target directory exists
	if dir := filepath.Dir(filename); dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create target directory: %w", err)
		}
	}

	switch format {
	case "simple":
		if cfg, ok := config.(*SimpleConfig); ok {
			return cfg.SaveToFile(filename)
		}
	case "full":
		if cfg, ok := config.(*Config); ok {
			return cfg.SaveToFile(filename)
		}
	}
	return fmt.Errorf("unsupported config type for format %s", format)
}

// generateConfigSummary generates a summary of the configuration
func (m *ConfigMigrationTool) generateConfigSummary(config interface{}, format string) map[string]interface{} {
	summary := map[string]interface{}{
		"format": format,
	}

	switch format {
	case "simple":
		if cfg, ok := config.(*SimpleConfig); ok {
			summary["implementation"] = cfg.Implementation.Type
			summary["gstreamer_enabled"] = cfg.GStreamer != nil
			summary["webrtc_enabled"] = cfg.WebRTC != nil
			summary["webserver_enabled"] = cfg.WebServer != nil
			
			if cfg.GStreamer != nil {
				summary["video_resolution"] = fmt.Sprintf("%dx%d", 
					cfg.GStreamer.Capture.Width, cfg.GStreamer.Capture.Height)
				summary["video_codec"] = cfg.GStreamer.Encoding.Codec
			}
		}
	case "full":
		if cfg, ok := config.(*Config); ok {
			summary["implementation"] = cfg.Implementation.Type
			summary["gstreamer_enabled"] = cfg.GStreamer != nil
			summary["webrtc_enabled"] = cfg.WebRTC != nil
			summary["webserver_enabled"] = cfg.WebServer != nil
			summary["metrics_enabled"] = cfg.Metrics != nil
			
			if cfg.GStreamer != nil {
				summary["video_resolution"] = fmt.Sprintf("%dx%d", 
					cfg.GStreamer.Capture.Width, cfg.GStreamer.Capture.Height)
				summary["video_codec"] = cfg.GStreamer.Encoding.Codec
			}
		}
	}

	return summary
}

// GenerateMigrationGuide generates a migration guide for moving between config formats
func (m *ConfigMigrationTool) GenerateMigrationGuide(sourceFormat, targetFormat string) (string, error) {
	var guide strings.Builder
	
	guide.WriteString(fmt.Sprintf("# Configuration Migration Guide: %s -> %s\n\n", sourceFormat, targetFormat))
	
	switch fmt.Sprintf("%s->%s", sourceFormat, targetFormat) {
	case "legacy->simple":
		guide.WriteString("## Migrating from Legacy to Simple Configuration\n\n")
		guide.WriteString("### Key Changes:\n")
		guide.WriteString("- Simplified structure with direct configuration access\n")
		guide.WriteString("- Removed complex validation layers\n")
		guide.WriteString("- Added implementation selection options\n")
		guide.WriteString("- Streamlined lifecycle management\n\n")
		guide.WriteString("### Migration Steps:\n")
		guide.WriteString("1. Use the migration tool: `migrate-config legacy.yaml simple.yaml simple`\n")
		guide.WriteString("2. Review the generated simple configuration\n")
		guide.WriteString("3. Update your application to use SimpleConfig\n")
		guide.WriteString("4. Test the new configuration\n\n")
		
	case "full->simple":
		guide.WriteString("## Migrating from Full to Simple Configuration\n\n")
		guide.WriteString("### Key Changes:\n")
		guide.WriteString("- Removed complex validation and cross-module compatibility checks\n")
		guide.WriteString("- Direct configuration access for better performance\n")
		guide.WriteString("- Simplified lifecycle management\n")
		guide.WriteString("- Focus on core functionality\n\n")
		guide.WriteString("### Migration Steps:\n")
		guide.WriteString("1. Use the migration tool: `migrate-config full.yaml simple.yaml simple`\n")
		guide.WriteString("2. Review any compatibility warnings\n")
		guide.WriteString("3. Update your application to use SimpleConfig\n")
		guide.WriteString("4. Test the simplified implementation\n\n")
		
	case "simple->full":
		guide.WriteString("## Migrating from Simple to Full Configuration\n\n")
		guide.WriteString("### Key Changes:\n")
		guide.WriteString("- Added comprehensive validation and error checking\n")
		guide.WriteString("- Cross-module compatibility validation\n")
		guide.WriteString("- Advanced lifecycle management options\n")
		guide.WriteString("- Support for complex features\n\n")
		guide.WriteString("### Migration Steps:\n")
		guide.WriteString("1. Use the migration tool: `migrate-config simple.yaml full.yaml full`\n")
		guide.WriteString("2. Review the generated full configuration\n")
		guide.WriteString("3. Update your application to use full Config\n")
		guide.WriteString("4. Test the complex implementation\n\n")
		
	default:
		return "", fmt.Errorf("unsupported migration path: %s -> %s", sourceFormat, targetFormat)
	}
	
	guide.WriteString("### Common Issues and Solutions:\n\n")
	guide.WriteString("#### Port Conflicts\n")
	guide.WriteString("- Ensure web server and metrics ports don't conflict\n")
	guide.WriteString("- Check that no other services are using the configured ports\n\n")
	
	guide.WriteString("#### Video Configuration\n")
	guide.WriteString("- Verify video resolution is supported by your system\n")
	guide.WriteString("- Check that the selected codec is available\n")
	guide.WriteString("- Test hardware encoding availability if enabled\n\n")
	
	guide.WriteString("#### WebRTC Configuration\n")
	guide.WriteString("- Ensure ICE servers are accessible\n")
	guide.WriteString("- Verify TURN server credentials if using TURN\n")
	guide.WriteString("- Test connectivity from client networks\n\n")
	
	guide.WriteString("### Validation Commands:\n")
	guide.WriteString("```bash\n")
	guide.WriteString("# Validate configuration\n")
	guide.WriteString("validate-config config.yaml\n\n")
	guide.WriteString("# Test configuration compatibility\n")
	guide.WriteString("check-config-compatibility config.yaml\n\n")
	guide.WriteString("# Generate example configuration\n")
	guide.WriteString("generate-example-config example.yaml\n")
	guide.WriteString("```\n\n")
	
	return guide.String(), nil
}

// CreateExampleConfigs creates example configuration files for all supported formats
func (m *ConfigMigrationTool) CreateExampleConfigs(outputDir string) error {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Create simple config example
	simpleConfigFile := filepath.Join(outputDir, "simple-config-example.yaml")
	if err := CreateExampleSimpleConfig(simpleConfigFile); err != nil {
		return fmt.Errorf("failed to create simple config example: %w", err)
	}
	m.logger.Infof("Created simple config example: %s", simpleConfigFile)

	// Create full config example
	fullConfig := DefaultConfig()
	fullConfigFile := filepath.Join(outputDir, "full-config-example.yaml")
	if err := fullConfig.SaveToFile(fullConfigFile); err != nil {
		return fmt.Errorf("failed to create full config example: %w", err)
	}
	m.logger.Infof("Created full config example: %s", fullConfigFile)

	return nil
}