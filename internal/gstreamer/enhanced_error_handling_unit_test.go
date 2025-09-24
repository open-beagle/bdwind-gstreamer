package gstreamer

import (
	"testing"
	"time"

	"github.com/go-gst/go-gst/gst"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestErrorSeverityMethods(t *testing.T) {
	t.Run("Severity String Representation", func(t *testing.T) {
		assert.Equal(t, "Info", SeverityInfo.String())
		assert.Equal(t, "Warning", SeverityWarning.String())
		assert.Equal(t, "Error", SeverityError.String())
		assert.Equal(t, "Critical", SeverityCritical.String())
		assert.Equal(t, "Fatal", SeverityFatal.String())
	})

	t.Run("Severity Log Levels", func(t *testing.T) {
		assert.Equal(t, logrus.InfoLevel, SeverityInfo.GetLogLevel())
		assert.Equal(t, logrus.WarnLevel, SeverityWarning.GetLogLevel())
		assert.Equal(t, logrus.ErrorLevel, SeverityError.GetLogLevel())
		assert.Equal(t, logrus.ErrorLevel, SeverityCritical.GetLogLevel())
		assert.Equal(t, logrus.FatalLevel, SeverityFatal.GetLogLevel())
	})

	t.Run("Severity Comparison", func(t *testing.T) {
		assert.True(t, SeverityCritical.IsHigherThan(SeverityWarning))
		assert.True(t, SeverityFatal.IsHigherThan(SeverityError))
		assert.False(t, SeverityInfo.IsHigherThan(SeverityWarning))
		assert.False(t, SeverityWarning.IsHigherThan(SeverityWarning))
	})

	t.Run("Immediate Action Required", func(t *testing.T) {
		assert.False(t, SeverityInfo.RequiresImmediateAction())
		assert.False(t, SeverityWarning.RequiresImmediateAction())
		assert.False(t, SeverityError.RequiresImmediateAction())
		assert.True(t, SeverityCritical.RequiresImmediateAction())
		assert.True(t, SeverityFatal.RequiresImmediateAction())
	})
}

func TestTrendDirection(t *testing.T) {
	t.Run("Trend Direction String", func(t *testing.T) {
		assert.Equal(t, "Stable", TrendStable.String())
		assert.Equal(t, "Increasing", TrendIncreasing.String())
		assert.Equal(t, "Decreasing", TrendDecreasing.String())
		assert.Equal(t, "Volatile", TrendVolatile.String())
	})
}

func TestErrorTrendAnalysisCreation(t *testing.T) {
	t.Run("New Error Trend Analysis", func(t *testing.T) {
		trends := NewErrorTrendAnalysis()
		require.NotNil(t, trends)

		assert.NotNil(t, trends.HourlyErrorCounts)
		assert.NotNil(t, trends.DailyErrorCounts)
		assert.NotNil(t, trends.WeeklyErrorCounts)
		assert.NotNil(t, trends.ComponentTrends)
		assert.NotNil(t, trends.SeverityTrends)
		assert.Equal(t, TrendStable, trends.TrendDirection)
		assert.Equal(t, 0.0, trends.ConfidenceLevel)
	})
}

func TestTrendAnalyzerConfig(t *testing.T) {
	t.Run("Default Config", func(t *testing.T) {
		config := DefaultTrendAnalyzerConfig()
		require.NotNil(t, config)

		assert.Equal(t, 10000, config.MaxHistorySize)
		assert.Equal(t, 24*time.Hour, config.AnalysisWindow)
		assert.Equal(t, 5*time.Minute, config.AnalysisInterval)
		assert.Equal(t, 10, config.MinDataPoints)
		assert.Equal(t, 0.7, config.ConfidenceThreshold)
		assert.Equal(t, 5*time.Minute, config.BurstDetectionWindow)
		assert.Equal(t, int64(10), config.BurstThreshold)
	})
}

func TestTrendAnalyzerCreation(t *testing.T) {
	t.Run("Create Trend Analyzer", func(t *testing.T) {
		logger := logrus.WithField("test", "trend-analyzer")
		analyzer := NewTrendAnalyzer(logger, nil)

		require.NotNil(t, analyzer)
		assert.NotNil(t, analyzer.logger)
		assert.NotNil(t, analyzer.config)
		assert.NotNil(t, analyzer.errorHistory)
		assert.NotNil(t, analyzer.currentTrends)
		assert.NotNil(t, analyzer.linearModel)
		assert.NotNil(t, analyzer.exponentialModel)
	})

	t.Run("Create with Custom Config", func(t *testing.T) {
		logger := logrus.WithField("test", "trend-analyzer")
		config := &TrendAnalyzerConfig{
			MaxHistorySize:       5000,
			AnalysisWindow:       12 * time.Hour,
			AnalysisInterval:     2 * time.Minute,
			MinDataPoints:        5,
			ConfidenceThreshold:  0.8,
			BurstDetectionWindow: 3 * time.Minute,
			BurstThreshold:       5,
		}

		analyzer := NewTrendAnalyzer(logger, config)
		require.NotNil(t, analyzer)
		assert.Equal(t, config, analyzer.config)
	})
}

func TestVisualizerConfig(t *testing.T) {
	t.Run("Default Visualizer Config", func(t *testing.T) {
		config := DefaultVisualizerConfig()
		require.NotNil(t, config)

		assert.Equal(t, "./pipeline_debug", config.OutputDirectory)
		assert.True(t, config.EnableAutoExport)
		assert.True(t, config.ExportOnError)
		assert.False(t, config.ExportOnStateChange)
		assert.Equal(t, 100, config.MaxExportFiles)
		assert.True(t, len(config.ExportFormats) > 0)
		assert.True(t, config.IncludeTimestamp)
		assert.True(t, config.IncludeMetadata)
	})
}

func TestExportFormat(t *testing.T) {
	t.Run("Export Format String", func(t *testing.T) {
		assert.Equal(t, "DOT", ExportDOT.String())
		assert.Equal(t, "SVG", ExportSVG.String())
		assert.Equal(t, "PNG", ExportPNG.String())
		assert.Equal(t, "JSON", ExportJSON.String())
		assert.Equal(t, "Text", ExportText.String())
	})

	t.Run("File Extensions", func(t *testing.T) {
		assert.Equal(t, ".dot", ExportDOT.FileExtension())
		assert.Equal(t, ".svg", ExportSVG.FileExtension())
		assert.Equal(t, ".png", ExportPNG.FileExtension())
		assert.Equal(t, ".json", ExportJSON.FileExtension())
		assert.Equal(t, ".txt", ExportText.FileExtension())
	})
}

func TestGStreamerErrorCreation(t *testing.T) {
	t.Run("Error Creation and Methods", func(t *testing.T) {
		err := &GStreamerError{
			Type:        ErrorTypePipelineState,
			Severity:    SeverityCritical,
			Component:   "test-component",
			Message:     "Test error message",
			Timestamp:   time.Now(),
			Recoverable: true,
			Suggested:   []RecoveryAction{RecoveryRetry, RecoveryRestart},
		}

		assert.Equal(t, ErrorTypePipelineState, err.Type)
		assert.Equal(t, SeverityCritical, err.Severity)
		assert.True(t, err.IsCritical())
		assert.True(t, err.IsRecoverable())
		assert.Contains(t, err.Error(), "test-component")
		assert.Contains(t, err.Error(), "Test error message")
	})
}

func TestErrorStatistics(t *testing.T) {
	t.Run("Error Statistics Structure", func(t *testing.T) {
		stats := &ErrorStatistics{
			TotalErrors:       100,
			ErrorsByType:      make(map[string]int64),
			ErrorsBySeverity:  make(map[string]int64),
			ErrorsByComponent: make(map[string]int64),
			RecoveryStats: RecoveryStatistics{
				TotalAttempts: 50,
				Successes:     30,
				Failures:      20,
				SuccessRate:   0.6,
			},
			TimeStats: TimeStatistics{
				LastErrorTime: time.Now(),
				Uptime:        time.Hour,
			},
		}

		assert.Equal(t, int64(100), stats.TotalErrors)
		assert.Equal(t, int64(50), stats.RecoveryStats.TotalAttempts)
		assert.Equal(t, 0.6, stats.RecoveryStats.SuccessRate)
		assert.NotNil(t, stats.ErrorsByType)
		assert.NotNil(t, stats.ErrorsBySeverity)
		assert.NotNil(t, stats.ErrorsByComponent)
	})
}

func TestBusMessageStats(t *testing.T) {
	t.Run("Bus Message Stats Creation", func(t *testing.T) {
		stats := &BusMessageStats{
			TotalMessages:         1000,
			MessagesByType:        make(map[gst.MessageType]int64),
			MessagesBySeverity:    make(map[GStreamerErrorSeverity]int64),
			MessagesPerSecond:     10.5,
			LastMessageTime:       time.Now(),
			ErrorMessages:         50,
			WarningMessages:       100,
			InfoMessages:          850,
			ProcessingTime:        time.Millisecond * 500,
			AverageProcessingTime: time.Microsecond * 500,
			MaxProcessingTime:     time.Millisecond * 10,
		}

		assert.Equal(t, int64(1000), stats.TotalMessages)
		assert.Equal(t, 10.5, stats.MessagesPerSecond)
		assert.Equal(t, int64(50), stats.ErrorMessages)
		assert.Equal(t, int64(100), stats.WarningMessages)
		assert.Equal(t, int64(850), stats.InfoMessages)
		assert.NotNil(t, stats.MessagesByType)
		assert.NotNil(t, stats.MessagesBySeverity)
	})
}

func TestUtilityFunctions(t *testing.T) {
	t.Run("Max Severity", func(t *testing.T) {
		assert.Equal(t, SeverityError, maxSeverity(SeverityWarning, SeverityError))
		assert.Equal(t, SeverityCritical, maxSeverity(SeverityInfo, SeverityCritical))
		assert.Equal(t, SeverityFatal, maxSeverity(SeverityFatal, SeverityError))
	})

	t.Run("Remove Duplicate Strings", func(t *testing.T) {
		input := []string{"a", "b", "a", "c", "b", "d"}
		result := removeDuplicateStrings(input)

		assert.Equal(t, 4, len(result))
		assert.Contains(t, result, "a")
		assert.Contains(t, result, "b")
		assert.Contains(t, result, "c")
		assert.Contains(t, result, "d")

		// Check that each element appears only once
		counts := make(map[string]int)
		for _, item := range result {
			counts[item]++
		}
		for _, count := range counts {
			assert.Equal(t, 1, count)
		}
	})
}

func TestErrorBurst(t *testing.T) {
	t.Run("Error Burst Structure", func(t *testing.T) {
		burst := ErrorBurst{
			StartTime:  time.Now().Add(-time.Minute),
			EndTime:    time.Now(),
			ErrorCount: 15,
			Duration:   time.Minute,
			Components: []string{"component1", "component2"},
			Severity:   SeverityCritical,
			Resolved:   false,
		}

		assert.Equal(t, int64(15), burst.ErrorCount)
		assert.Equal(t, time.Minute, burst.Duration)
		assert.Equal(t, SeverityCritical, burst.Severity)
		assert.False(t, burst.Resolved)
		assert.Equal(t, 2, len(burst.Components))
	})
}

func TestComponentErrorTrend(t *testing.T) {
	t.Run("Component Error Trend Structure", func(t *testing.T) {
		trend := &ComponentErrorTrend{
			Component:           "test-component",
			ErrorCount:          25,
			LastErrorTime:       time.Now(),
			ErrorRate:           2.5,
			MostCommonErrorType: ErrorTypePipelineState,
			TrendDirection:      TrendIncreasing,
			HealthScore:         75.0,
		}

		assert.Equal(t, "test-component", trend.Component)
		assert.Equal(t, int64(25), trend.ErrorCount)
		assert.Equal(t, 2.5, trend.ErrorRate)
		assert.Equal(t, ErrorTypePipelineState, trend.MostCommonErrorType)
		assert.Equal(t, TrendIncreasing, trend.TrendDirection)
		assert.Equal(t, 75.0, trend.HealthScore)
	})
}

func TestSeverityTrend(t *testing.T) {
	t.Run("Severity Trend Structure", func(t *testing.T) {
		trend := &SeverityTrend{
			Severity:       SeverityError,
			Count:          50,
			Rate:           5.0,
			TrendDirection: TrendDecreasing,
			RecentIncrease: false,
		}

		assert.Equal(t, SeverityError, trend.Severity)
		assert.Equal(t, int64(50), trend.Count)
		assert.Equal(t, 5.0, trend.Rate)
		assert.Equal(t, TrendDecreasing, trend.TrendDirection)
		assert.False(t, trend.RecentIncrease)
	})
}

// Test that doesn't require GStreamer initialization
func TestBasicErrorHandlerCreation(t *testing.T) {
	t.Run("Create Error Handler Without GStreamer", func(t *testing.T) {
		logger := logrus.WithField("test", "error-handler")

		// Test with nil config (should use defaults)
		handler := NewErrorHandler(logger, nil)
		require.NotNil(t, handler)
		assert.NotNil(t, handler.logger)
		assert.NotNil(t, handler.config)
		assert.NotNil(t, handler.metrics)
		assert.NotNil(t, handler.trendAnalyzer)
		assert.NotNil(t, handler.pipelineVisualizer)

		// Test debug mode methods
		assert.False(t, handler.IsDebugMode())
		handler.EnableDebugMode()
		assert.True(t, handler.IsDebugMode())
		handler.DisableDebugMode()
		assert.False(t, handler.IsDebugMode())
	})
}
