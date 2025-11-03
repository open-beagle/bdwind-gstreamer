package config

import (
	"fmt"
	"strings"
)

// CompatibilityGuide provides guidance for configuration compatibility and migration
type CompatibilityGuide struct {
	checker *ConfigCompatibilityChecker
}

// NewCompatibilityGuide creates a new compatibility guide
func NewCompatibilityGuide() *CompatibilityGuide {
	return &CompatibilityGuide{
		checker: NewConfigCompatibilityChecker(),
	}
}

// GetMigrationRecommendation provides migration recommendations based on current config
func (g *CompatibilityGuide) GetMigrationRecommendation(currentFormat string, useCase string) string {
	var guide strings.Builder
	
	guide.WriteString(fmt.Sprintf("# Migration Recommendation for %s Configuration\n\n", currentFormat))
	
	switch useCase {
	case "performance":
		guide.WriteString("## Performance-Focused Migration\n\n")
		guide.WriteString("For maximum performance, we recommend migrating to **Simple Configuration**:\n\n")
		guide.WriteString("### Benefits:\n")
		guide.WriteString("- Direct configuration access without validation overhead\n")
		guide.WriteString("- Minimal memory footprint\n")
		guide.WriteString("- Faster startup times\n")
		guide.WriteString("- Reduced CPU usage during configuration access\n\n")
		guide.WriteString("### Migration Command:\n")
		guide.WriteString("```bash\n")
		guide.WriteString("config-migration -source current.yaml -format simple\n")
		guide.WriteString("```\n\n")
		
	case "reliability":
		guide.WriteString("## Reliability-Focused Migration\n\n")
		guide.WriteString("For maximum reliability, we recommend migrating to **Full Configuration**:\n\n")
		guide.WriteString("### Benefits:\n")
		guide.WriteString("- Comprehensive validation and error checking\n")
		guide.WriteString("- Cross-module compatibility validation\n")
		guide.WriteString("- Advanced error handling and recovery\n")
		guide.WriteString("- Detailed configuration validation\n\n")
		guide.WriteString("### Migration Command:\n")
		guide.WriteString("```bash\n")
		guide.WriteString("config-migration -source current.yaml -format full\n")
		guide.WriteString("```\n\n")
		
	case "development":
		guide.WriteString("## Development-Focused Migration\n\n")
		guide.WriteString("For development environments, we recommend **Simple Configuration**:\n\n")
		guide.WriteString("### Benefits:\n")
		guide.WriteString("- Quick iteration and testing\n")
		guide.WriteString("- Easy configuration changes\n")
		guide.WriteString("- Minimal validation for faster development\n")
		guide.WriteString("- Direct access to configuration values\n\n")
		
	case "production":
		guide.WriteString("## Production-Focused Migration\n\n")
		guide.WriteString("For production environments, consider your priorities:\n\n")
		guide.WriteString("### High Performance Production -> Simple Configuration\n")
		guide.WriteString("- Use when performance is critical\n")
		guide.WriteString("- Suitable for well-tested configurations\n")
		guide.WriteString("- Minimal overhead and fast access\n\n")
		guide.WriteString("### High Reliability Production -> Full Configuration\n")
		guide.WriteString("- Use when reliability is critical\n")
		guide.WriteString("- Comprehensive validation and error checking\n")
		guide.WriteString("- Better error reporting and debugging\n\n")
	}
	
	// Add format-specific recommendations
	switch currentFormat {
	case "legacy":
		guide.WriteString("## Legacy Configuration Migration\n\n")
		guide.WriteString("Your current legacy configuration should be migrated:\n\n")
		guide.WriteString("### Immediate Actions:\n")
		guide.WriteString("1. **Backup** your current configuration\n")
		guide.WriteString("2. **Validate** the current configuration\n")
		guide.WriteString("3. **Choose** target format based on your needs\n")
		guide.WriteString("4. **Migrate** using the migration tool\n")
		guide.WriteString("5. **Test** thoroughly before deployment\n\n")
		
	case "full":
		guide.WriteString("## Full Configuration Optimization\n\n")
		guide.WriteString("Your current full configuration can be optimized:\n\n")
		guide.WriteString("### Consider Simple Configuration if:\n")
		guide.WriteString("- Performance is critical\n")
		guide.WriteString("- Configuration is stable and well-tested\n")
		guide.WriteString("- You don't need complex validation\n")
		guide.WriteString("- You want minimal overhead\n\n")
		
	case "simple":
		guide.WriteString("## Simple Configuration Enhancement\n\n")
		guide.WriteString("Your current simple configuration is optimized for performance:\n\n")
		guide.WriteString("### Consider Full Configuration if:\n")
		guide.WriteString("- You need comprehensive validation\n")
		guide.WriteString("- Configuration changes frequently\n")
		guide.WriteString("- You need advanced error handling\n")
		guide.WriteString("- You want cross-module compatibility checks\n\n")
	}
	
	return guide.String()
}

// GetCompatibilityMatrix returns a compatibility matrix for different configurations
func (g *CompatibilityGuide) GetCompatibilityMatrix() map[string]map[string]string {
	return map[string]map[string]string{
		"legacy": {
			"simple": "✅ Recommended - Direct migration with performance benefits",
			"full":   "✅ Supported - Adds comprehensive validation",
		},
		"full": {
			"simple": "⚠️ Consider - Removes validation but improves performance",
			"legacy": "❌ Not recommended - Loss of functionality",
		},
		"simple": {
			"full":   "✅ Supported - Adds comprehensive validation",
			"legacy": "❌ Not recommended - Loss of functionality",
		},
	}
}

// GetFeatureComparison returns a feature comparison between configuration formats
func (g *CompatibilityGuide) GetFeatureComparison() map[string]map[string]string {
	return map[string]map[string]string{
		"Performance": {
			"legacy": "⚠️ Moderate - Flat structure but limited optimization",
			"full":   "⚠️ Moderate - Comprehensive but with validation overhead",
			"simple": "✅ Excellent - Direct access with minimal overhead",
		},
		"Validation": {
			"legacy": "❌ Minimal - Basic structure validation only",
			"full":   "✅ Comprehensive - Full validation and compatibility checks",
			"simple": "⚠️ Essential - Only critical validation for functionality",
		},
		"Maintainability": {
			"legacy": "❌ Poor - Flat structure, hard to extend",
			"full":   "✅ Excellent - Modular structure with clear separation",
			"simple": "✅ Good - Clean structure with direct access",
		},
		"Error Handling": {
			"legacy": "❌ Basic - Limited error reporting",
			"full":   "✅ Advanced - Comprehensive error handling and reporting",
			"simple": "⚠️ Functional - Essential error handling only",
		},
		"Development Speed": {
			"legacy": "⚠️ Moderate - Simple but limited",
			"full":   "⚠️ Moderate - Feature-rich but complex",
			"simple": "✅ Fast - Quick iteration and testing",
		},
		"Production Readiness": {
			"legacy": "❌ Limited - Missing modern features",
			"full":   "✅ Excellent - Production-ready with all features",
			"simple": "✅ Good - Production-ready with focus on performance",
		},
	}
}

// GenerateCompatibilityReport generates a comprehensive compatibility report
func (g *CompatibilityGuide) GenerateCompatibilityReport(currentConfig interface{}, currentFormat string) string {
	var report strings.Builder
	
	report.WriteString("# Configuration Compatibility Report\n\n")
	report.WriteString(fmt.Sprintf("**Current Format:** %s\n", currentFormat))
	report.WriteString(fmt.Sprintf("**Generated:** %s\n\n", "now"))
	
	// Add current configuration analysis
	report.WriteString("## Current Configuration Analysis\n\n")
	
	var issues []string
	switch currentFormat {
	case "simple":
		if cfg, ok := currentConfig.(*SimpleConfig); ok {
			issues = g.checker.CheckSimpleConfigCompatibility(cfg)
		}
	case "full":
		if cfg, ok := currentConfig.(*Config); ok {
			issues = g.checker.CheckFullConfigCompatibility(cfg)
		}
	}
	
	if len(issues) == 0 {
		report.WriteString("✅ **Status:** Configuration is compatible\n\n")
	} else {
		report.WriteString(fmt.Sprintf("⚠️ **Status:** Configuration has %d compatibility issues\n\n", len(issues)))
		report.WriteString("### Issues Found:\n")
		for i, issue := range issues {
			report.WriteString(fmt.Sprintf("%d. %s\n", i+1, issue))
		}
		report.WriteString("\n")
	}
	
	// Add feature comparison
	report.WriteString("## Feature Comparison\n\n")
	features := g.GetFeatureComparison()
	
	report.WriteString("| Feature | Legacy | Full | Simple |\n")
	report.WriteString("|---------|--------|------|--------|\n")
	
	for feature, formats := range features {
		report.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n",
			feature, formats["legacy"], formats["full"], formats["simple"]))
	}
	report.WriteString("\n")
	
	// Add migration recommendations
	report.WriteString("## Migration Recommendations\n\n")
	matrix := g.GetCompatibilityMatrix()
	
	if migrations, exists := matrix[currentFormat]; exists {
		for targetFormat, recommendation := range migrations {
			report.WriteString(fmt.Sprintf("### To %s Format\n", strings.Title(targetFormat)))
			report.WriteString(fmt.Sprintf("%s\n\n", recommendation))
		}
	}
	
	// Add next steps
	report.WriteString("## Next Steps\n\n")
	report.WriteString("1. **Review** the compatibility issues identified above\n")
	report.WriteString("2. **Choose** target configuration format based on your needs\n")
	report.WriteString("3. **Backup** your current configuration\n")
	report.WriteString("4. **Use migration tool** to convert configuration\n")
	report.WriteString("5. **Test** the new configuration thoroughly\n")
	report.WriteString("6. **Deploy** with monitoring and rollback plan\n\n")
	
	report.WriteString("## Migration Commands\n\n")
	report.WriteString("```bash\n")
	report.WriteString("# Validate current configuration\n")
	report.WriteString("config-migration -validate -source current.yaml\n\n")
	report.WriteString("# Migrate to simple format\n")
	report.WriteString("config-migration -source current.yaml -format simple\n\n")
	report.WriteString("# Migrate to full format\n")
	report.WriteString("config-migration -source current.yaml -format full\n")
	report.WriteString("```\n")
	
	return report.String()
}

// GetQuickMigrationGuide provides a quick migration guide for common scenarios
func (g *CompatibilityGuide) GetQuickMigrationGuide() string {
	return `# Quick Migration Guide

## Common Migration Scenarios

### 🚀 Performance Optimization
**Goal:** Maximize performance and minimize overhead
**Recommendation:** Migrate to Simple Configuration
**Command:** ` + "`config-migration -source config.yaml -format simple`" + `

### 🛡️ Reliability Enhancement  
**Goal:** Comprehensive validation and error handling
**Recommendation:** Migrate to Full Configuration
**Command:** ` + "`config-migration -source config.yaml -format full`" + `

### 🔄 Legacy Modernization
**Goal:** Update from legacy format
**Recommendation:** Migrate to Simple Configuration first, then Full if needed
**Commands:**
` + "```bash" + `
# Step 1: Legacy to Simple
config-migration -source legacy.yaml -format simple

# Step 2: Simple to Full (if needed)
config-migration -source simple.yaml -format full
` + "```" + `

## Quick Validation
` + "```bash" + `
# Check current configuration
config-migration -validate -source config.yaml

# Detect format
config-migration -detect -source config.yaml
` + "```" + `

## Emergency Rollback
` + "```bash" + `
# Restore from backup (created automatically)
cp config.yaml.backup.TIMESTAMP config.yaml
` + "```" + `
`
}