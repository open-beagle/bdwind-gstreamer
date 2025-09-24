package gstreamer

import (
	"math"
	"sort"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// TrendAnalyzer analyzes error trends and provides predictions
type TrendAnalyzer struct {
	logger *logrus.Entry
	mu     sync.RWMutex

	// Configuration
	config *TrendAnalyzerConfig

	// Data storage
	errorHistory   []ErrorDataPoint
	maxHistorySize int
	analysisWindow time.Duration

	// Analysis results
	currentTrends    *ErrorTrendAnalysis
	lastAnalysis     time.Time
	analysisInterval time.Duration

	// Prediction models
	linearModel      *LinearTrendModel
	exponentialModel *ExponentialTrendModel
}

// TrendAnalyzerConfig configures the trend analyzer
type TrendAnalyzerConfig struct {
	MaxHistorySize       int
	AnalysisWindow       time.Duration
	AnalysisInterval     time.Duration
	MinDataPoints        int
	ConfidenceThreshold  float64
	BurstDetectionWindow time.Duration
	BurstThreshold       int64
}

// ErrorDataPoint represents a single error data point for analysis
type ErrorDataPoint struct {
	Timestamp    time.Time
	ErrorType    GStreamerErrorType
	Severity     GStreamerErrorSeverity
	Component    string
	Recovered    bool
	RecoveryTime time.Duration
}

// LinearTrendModel provides linear trend analysis
type LinearTrendModel struct {
	Slope       float64
	Intercept   float64
	Correlation float64
	Confidence  float64
}

// ExponentialTrendModel provides exponential trend analysis
type ExponentialTrendModel struct {
	GrowthRate float64
	BaseValue  float64
	Confidence float64
}

// NewTrendAnalyzer creates a new trend analyzer
func NewTrendAnalyzer(logger *logrus.Entry, config *TrendAnalyzerConfig) *TrendAnalyzer {
	if config == nil {
		config = DefaultTrendAnalyzerConfig()
	}

	if logger == nil {
		logger = logrus.WithField("component", "trend-analyzer")
	}

	return &TrendAnalyzer{
		logger:           logger,
		config:           config,
		errorHistory:     make([]ErrorDataPoint, 0, config.MaxHistorySize),
		maxHistorySize:   config.MaxHistorySize,
		analysisWindow:   config.AnalysisWindow,
		analysisInterval: config.AnalysisInterval,
		currentTrends:    NewErrorTrendAnalysis(),
		linearModel:      &LinearTrendModel{},
		exponentialModel: &ExponentialTrendModel{},
	}
}

// DefaultTrendAnalyzerConfig returns default configuration
func DefaultTrendAnalyzerConfig() *TrendAnalyzerConfig {
	return &TrendAnalyzerConfig{
		MaxHistorySize:       10000,
		AnalysisWindow:       24 * time.Hour,
		AnalysisInterval:     5 * time.Minute,
		MinDataPoints:        10,
		ConfidenceThreshold:  0.7,
		BurstDetectionWindow: 5 * time.Minute,
		BurstThreshold:       10,
	}
}

// NewErrorTrendAnalysis creates a new error trend analysis
func NewErrorTrendAnalysis() *ErrorTrendAnalysis {
	return &ErrorTrendAnalysis{
		HourlyErrorCounts: make(map[int]int64),
		DailyErrorCounts:  make(map[string]int64),
		WeeklyErrorCounts: make(map[int]int64),
		ComponentTrends:   make(map[string]*ComponentErrorTrend),
		SeverityTrends:    make(map[GStreamerErrorSeverity]*SeverityTrend),
		TrendDirection:    TrendStable,
		ConfidenceLevel:   0.0,
	}
}

// AddErrorDataPoint adds a new error data point for analysis
func (ta *TrendAnalyzer) AddErrorDataPoint(errorType GStreamerErrorType, severity GStreamerErrorSeverity,
	component string, recovered bool, recoveryTime time.Duration) {
	ta.mu.Lock()
	defer ta.mu.Unlock()

	dataPoint := ErrorDataPoint{
		Timestamp:    time.Now(),
		ErrorType:    errorType,
		Severity:     severity,
		Component:    component,
		Recovered:    recovered,
		RecoveryTime: recoveryTime,
	}

	// Add to history
	ta.errorHistory = append(ta.errorHistory, dataPoint)

	// Trim history if needed
	if len(ta.errorHistory) > ta.maxHistorySize {
		ta.errorHistory = ta.errorHistory[1:]
	}

	ta.logger.Debugf("Added error data point: type=%s, severity=%s, component=%s, recovered=%v",
		errorType.String(), severity.String(), component, recovered)

	// Trigger analysis if enough time has passed
	if time.Since(ta.lastAnalysis) >= ta.analysisInterval {
		go ta.performAnalysis()
	}
}

// performAnalysis performs comprehensive trend analysis
func (ta *TrendAnalyzer) performAnalysis() {
	ta.mu.Lock()
	defer ta.mu.Unlock()

	ta.logger.Debug("Starting error trend analysis")
	startTime := time.Now()

	// Check if we have enough data
	if len(ta.errorHistory) < ta.config.MinDataPoints {
		ta.logger.Debugf("Insufficient data points for analysis: %d < %d",
			len(ta.errorHistory), ta.config.MinDataPoints)
		return
	}

	// Create new analysis
	analysis := NewErrorTrendAnalysis()

	// Analyze time-based patterns
	ta.analyzeTimePatterns(analysis)

	// Analyze component trends
	ta.analyzeComponentTrends(analysis)

	// Analyze severity trends
	ta.analyzeSeverityTrends(analysis)

	// Analyze recovery patterns
	ta.analyzeRecoveryPatterns(analysis)

	// Detect error bursts
	ta.detectErrorBursts(analysis)

	// Perform predictive analysis
	ta.performPredictiveAnalysis(analysis)

	// Update current trends
	ta.currentTrends = analysis
	ta.lastAnalysis = time.Now()

	duration := time.Since(startTime)
	ta.logger.Debugf("Error trend analysis completed in %v", duration)
}

// analyzeTimePatterns analyzes time-based error patterns
func (ta *TrendAnalyzer) analyzeTimePatterns(analysis *ErrorTrendAnalysis) {
	cutoff := time.Now().Add(-ta.analysisWindow)

	for _, dataPoint := range ta.errorHistory {
		if dataPoint.Timestamp.Before(cutoff) {
			continue
		}

		// Hourly patterns
		hour := dataPoint.Timestamp.Hour()
		analysis.HourlyErrorCounts[hour]++

		// Daily patterns
		date := dataPoint.Timestamp.Format("2006-01-02")
		analysis.DailyErrorCounts[date]++

		// Weekly patterns
		_, week := dataPoint.Timestamp.ISOWeek()
		analysis.WeeklyErrorCounts[week]++
	}

	ta.logger.Debugf("Analyzed time patterns: %d hourly, %d daily, %d weekly data points",
		len(analysis.HourlyErrorCounts), len(analysis.DailyErrorCounts), len(analysis.WeeklyErrorCounts))
}

// analyzeComponentTrends analyzes error trends by component
func (ta *TrendAnalyzer) analyzeComponentTrends(analysis *ErrorTrendAnalysis) {
	componentData := make(map[string][]ErrorDataPoint)
	cutoff := time.Now().Add(-ta.analysisWindow)

	// Group errors by component
	for _, dataPoint := range ta.errorHistory {
		if dataPoint.Timestamp.Before(cutoff) {
			continue
		}
		componentData[dataPoint.Component] = append(componentData[dataPoint.Component], dataPoint)
	}

	// Analyze each component
	for component, errors := range componentData {
		trend := ta.analyzeComponentErrorTrend(component, errors)
		analysis.ComponentTrends[component] = trend
	}

	ta.logger.Debugf("Analyzed component trends for %d components", len(analysis.ComponentTrends))
}

// analyzeComponentErrorTrend analyzes error trend for a specific component
func (ta *TrendAnalyzer) analyzeComponentErrorTrend(component string, errors []ErrorDataPoint) *ComponentErrorTrend {
	if len(errors) == 0 {
		return &ComponentErrorTrend{
			Component:      component,
			TrendDirection: TrendStable,
			HealthScore:    100.0,
		}
	}

	// Sort by timestamp
	sort.Slice(errors, func(i, j int) bool {
		return errors[i].Timestamp.Before(errors[j].Timestamp)
	})

	trend := &ComponentErrorTrend{
		Component:     component,
		ErrorCount:    int64(len(errors)),
		LastErrorTime: errors[len(errors)-1].Timestamp,
	}

	// Calculate error rate (errors per hour)
	if len(errors) > 1 {
		duration := errors[len(errors)-1].Timestamp.Sub(errors[0].Timestamp)
		if duration > 0 {
			trend.ErrorRate = float64(len(errors)) / duration.Hours()
		}
	}

	// Find most common error type
	errorTypeCounts := make(map[GStreamerErrorType]int)
	for _, err := range errors {
		errorTypeCounts[err.ErrorType]++
	}

	maxCount := 0
	for errorType, count := range errorTypeCounts {
		if count > maxCount {
			maxCount = count
			trend.MostCommonErrorType = errorType
		}
	}

	// Calculate trend direction
	trend.TrendDirection = ta.calculateTrendDirection(errors)

	// Calculate health score (0-100, higher is better)
	trend.HealthScore = ta.calculateHealthScore(errors)

	return trend
}

// analyzeSeverityTrends analyzes error trends by severity
func (ta *TrendAnalyzer) analyzeSeverityTrends(analysis *ErrorTrendAnalysis) {
	severityData := make(map[GStreamerErrorSeverity][]ErrorDataPoint)
	cutoff := time.Now().Add(-ta.analysisWindow)

	// Group errors by severity
	for _, dataPoint := range ta.errorHistory {
		if dataPoint.Timestamp.Before(cutoff) {
			continue
		}
		severityData[dataPoint.Severity] = append(severityData[dataPoint.Severity], dataPoint)
	}

	// Analyze each severity level
	for severity, errors := range severityData {
		trend := &SeverityTrend{
			Severity:       severity,
			Count:          int64(len(errors)),
			TrendDirection: ta.calculateTrendDirection(errors),
		}

		// Calculate rate (errors per hour)
		if len(errors) > 1 {
			duration := errors[len(errors)-1].Timestamp.Sub(errors[0].Timestamp)
			if duration > 0 {
				trend.Rate = float64(len(errors)) / duration.Hours()
			}
		}

		// Check for recent increase
		trend.RecentIncrease = ta.hasRecentIncrease(errors, time.Hour)

		analysis.SeverityTrends[severity] = trend
	}

	ta.logger.Debugf("Analyzed severity trends for %d severity levels", len(analysis.SeverityTrends))
}

// analyzeRecoveryPatterns analyzes error recovery patterns
func (ta *TrendAnalyzer) analyzeRecoveryPatterns(analysis *ErrorTrendAnalysis) {
	var totalRecoveryTime time.Duration
	var recoveredErrors int64
	var totalErrors int64

	cutoff := time.Now().Add(-ta.analysisWindow)

	for _, dataPoint := range ta.errorHistory {
		if dataPoint.Timestamp.Before(cutoff) {
			continue
		}

		totalErrors++
		if dataPoint.Recovered {
			recoveredErrors++
			totalRecoveryTime += dataPoint.RecoveryTime
		}
	}

	if totalErrors > 0 {
		analysis.RecoverySuccessRate = float64(recoveredErrors) / float64(totalErrors)
	}

	if recoveredErrors > 0 {
		analysis.AvgRecoveryTime = totalRecoveryTime / time.Duration(recoveredErrors)
	}

	ta.logger.Debugf("Recovery analysis: success rate=%.2f%%, avg recovery time=%v",
		analysis.RecoverySuccessRate*100, analysis.AvgRecoveryTime)
}

// detectErrorBursts detects bursts of errors in short time periods
func (ta *TrendAnalyzer) detectErrorBursts(analysis *ErrorTrendAnalysis) {
	if len(ta.errorHistory) < 2 {
		return
	}

	var bursts []ErrorBurst
	currentBurst := ErrorBurst{}
	inBurst := false

	// Sort errors by timestamp
	sortedErrors := make([]ErrorDataPoint, len(ta.errorHistory))
	copy(sortedErrors, ta.errorHistory)
	sort.Slice(sortedErrors, func(i, j int) bool {
		return sortedErrors[i].Timestamp.Before(sortedErrors[j].Timestamp)
	})

	for i, dataPoint := range sortedErrors {
		if i == 0 {
			continue
		}

		timeDiff := dataPoint.Timestamp.Sub(sortedErrors[i-1].Timestamp)

		if timeDiff <= ta.config.BurstDetectionWindow {
			if !inBurst {
				// Start new burst
				currentBurst = ErrorBurst{
					StartTime:  sortedErrors[i-1].Timestamp,
					ErrorCount: 2,
					Components: []string{sortedErrors[i-1].Component, dataPoint.Component},
					Severity:   maxSeverity(sortedErrors[i-1].Severity, dataPoint.Severity),
				}
				inBurst = true
			} else {
				// Continue current burst
				currentBurst.ErrorCount++
				currentBurst.Components = append(currentBurst.Components, dataPoint.Component)
				if dataPoint.Severity > currentBurst.Severity {
					currentBurst.Severity = dataPoint.Severity
				}
			}
			currentBurst.EndTime = dataPoint.Timestamp
		} else {
			if inBurst && currentBurst.ErrorCount >= ta.config.BurstThreshold {
				// End current burst
				currentBurst.Duration = currentBurst.EndTime.Sub(currentBurst.StartTime)
				bursts = append(bursts, currentBurst)
			}
			inBurst = false
		}
	}

	// Check if we ended in a burst
	if inBurst && currentBurst.ErrorCount >= ta.config.BurstThreshold {
		currentBurst.Duration = currentBurst.EndTime.Sub(currentBurst.StartTime)
		bursts = append(bursts, currentBurst)
	}

	// Remove duplicates from component lists
	for i := range bursts {
		bursts[i].Components = removeDuplicateStrings(bursts[i].Components)
	}

	ta.logger.Debugf("Detected %d error bursts", len(bursts))
}

// performPredictiveAnalysis performs predictive analysis on error trends
func (ta *TrendAnalyzer) performPredictiveAnalysis(analysis *ErrorTrendAnalysis) {
	if len(ta.errorHistory) < ta.config.MinDataPoints {
		return
	}

	// Prepare time series data
	timeSeries := ta.prepareTimeSeriesData()

	// Perform linear regression
	ta.performLinearRegression(timeSeries)

	// Perform exponential analysis
	ta.performExponentialAnalysis(timeSeries)

	// Determine overall trend direction
	analysis.TrendDirection = ta.determineOverallTrend()

	// Calculate confidence level
	analysis.ConfidenceLevel = ta.calculateConfidenceLevel()

	// Predict error rate
	analysis.PredictedErrorRate = ta.predictErrorRate()

	ta.logger.Debugf("Predictive analysis: trend=%s, confidence=%.2f, predicted rate=%.2f errors/hour",
		analysis.TrendDirection.String(), analysis.ConfidenceLevel, analysis.PredictedErrorRate)
}

// Helper methods

func (ta *TrendAnalyzer) calculateTrendDirection(errors []ErrorDataPoint) TrendDirection {
	if len(errors) < 3 {
		return TrendStable
	}

	// Simple trend calculation based on error frequency over time
	midpoint := len(errors) / 2
	firstHalf := errors[:midpoint]
	secondHalf := errors[midpoint:]

	firstHalfRate := float64(len(firstHalf))
	secondHalfRate := float64(len(secondHalf))

	if len(firstHalf) > 0 && len(secondHalf) > 0 {
		firstDuration := firstHalf[len(firstHalf)-1].Timestamp.Sub(firstHalf[0].Timestamp)
		secondDuration := secondHalf[len(secondHalf)-1].Timestamp.Sub(secondHalf[0].Timestamp)

		if firstDuration > 0 {
			firstHalfRate = firstHalfRate / firstDuration.Hours()
		}
		if secondDuration > 0 {
			secondHalfRate = secondHalfRate / secondDuration.Hours()
		}
	}

	ratio := secondHalfRate / (firstHalfRate + 0.001) // Avoid division by zero

	if ratio > 1.2 {
		return TrendIncreasing
	} else if ratio < 0.8 {
		return TrendDecreasing
	} else {
		return TrendStable
	}
}

func (ta *TrendAnalyzer) calculateHealthScore(errors []ErrorDataPoint) float64 {
	if len(errors) == 0 {
		return 100.0
	}

	// Base score starts at 100
	score := 100.0

	// Deduct points for error frequency
	errorRate := float64(len(errors)) / ta.analysisWindow.Hours()
	score -= errorRate * 10 // 10 points per error per hour

	// Deduct points for severity
	for _, err := range errors {
		switch err.Severity {
		case SeverityFatal:
			score -= 20
		case SeverityCritical:
			score -= 15
		case SeverityError:
			score -= 10
		case SeverityWarning:
			score -= 5
		case SeverityInfo:
			score -= 1
		}
	}

	// Add points for recovery
	recoveredCount := 0
	for _, err := range errors {
		if err.Recovered {
			recoveredCount++
		}
	}
	recoveryRate := float64(recoveredCount) / float64(len(errors))
	score += recoveryRate * 20 // Up to 20 points for good recovery

	// Ensure score is between 0 and 100
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}

	return score
}

func (ta *TrendAnalyzer) hasRecentIncrease(errors []ErrorDataPoint, window time.Duration) bool {
	if len(errors) < 2 {
		return false
	}

	cutoff := time.Now().Add(-window)
	recentCount := 0
	totalCount := len(errors)

	for _, err := range errors {
		if err.Timestamp.After(cutoff) {
			recentCount++
		}
	}

	// Consider it a recent increase if more than 60% of errors occurred in the recent window
	return float64(recentCount)/float64(totalCount) > 0.6
}

func (ta *TrendAnalyzer) prepareTimeSeriesData() []float64 {
	// Create hourly buckets for the analysis window
	hours := int(ta.analysisWindow.Hours())
	timeSeries := make([]float64, hours)

	cutoff := time.Now().Add(-ta.analysisWindow)

	for _, dataPoint := range ta.errorHistory {
		if dataPoint.Timestamp.Before(cutoff) {
			continue
		}

		// Calculate which hour bucket this error belongs to
		hoursSince := int(time.Since(dataPoint.Timestamp).Hours())
		if hoursSince >= 0 && hoursSince < hours {
			timeSeries[hours-1-hoursSince]++ // Reverse order for chronological
		}
	}

	return timeSeries
}

func (ta *TrendAnalyzer) performLinearRegression(timeSeries []float64) {
	n := float64(len(timeSeries))
	if n < 2 {
		return
	}

	// Calculate means
	var sumX, sumY, sumXY, sumX2 float64
	for i, y := range timeSeries {
		x := float64(i)
		sumX += x
		sumY += y
		sumXY += x * y
		sumX2 += x * x
	}

	meanX := sumX / n
	meanY := sumY / n

	// Calculate slope and intercept
	denominator := sumX2 - n*meanX*meanX
	if denominator != 0 {
		ta.linearModel.Slope = (sumXY - n*meanX*meanY) / denominator
		ta.linearModel.Intercept = meanY - ta.linearModel.Slope*meanX

		// Calculate correlation coefficient
		var sumYDiff2, sumXDiff2, sumXYDiff float64
		for i, y := range timeSeries {
			x := float64(i)
			xDiff := x - meanX
			yDiff := y - meanY
			sumXDiff2 += xDiff * xDiff
			sumYDiff2 += yDiff * yDiff
			sumXYDiff += xDiff * yDiff
		}

		if sumXDiff2 > 0 && sumYDiff2 > 0 {
			ta.linearModel.Correlation = sumXYDiff / math.Sqrt(sumXDiff2*sumYDiff2)
			ta.linearModel.Confidence = math.Abs(ta.linearModel.Correlation)
		}
	}
}

func (ta *TrendAnalyzer) performExponentialAnalysis(timeSeries []float64) {
	if len(timeSeries) < 3 {
		return
	}

	// Simple exponential analysis
	// Calculate growth rate based on first and last non-zero values
	var firstValue, lastValue float64
	var firstIndex, lastIndex int

	for i, val := range timeSeries {
		if val > 0 {
			if firstValue == 0 {
				firstValue = val
				firstIndex = i
			}
			lastValue = val
			lastIndex = i
		}
	}

	if firstValue > 0 && lastValue > 0 && lastIndex > firstIndex {
		periods := float64(lastIndex - firstIndex)
		ta.exponentialModel.GrowthRate = math.Pow(lastValue/firstValue, 1.0/periods) - 1.0
		ta.exponentialModel.BaseValue = firstValue
		ta.exponentialModel.Confidence = 0.5 // Simple confidence measure
	}
}

func (ta *TrendAnalyzer) determineOverallTrend() TrendDirection {
	// Use linear model slope to determine trend
	if math.Abs(ta.linearModel.Slope) < 0.1 {
		return TrendStable
	} else if ta.linearModel.Slope > 0 {
		return TrendIncreasing
	} else {
		return TrendDecreasing
	}
}

func (ta *TrendAnalyzer) calculateConfidenceLevel() float64 {
	// Use correlation coefficient as confidence measure
	return ta.linearModel.Confidence
}

func (ta *TrendAnalyzer) predictErrorRate() float64 {
	// Predict error rate for next hour using linear model
	nextHour := float64(len(ta.prepareTimeSeriesData()))
	predicted := ta.linearModel.Slope*nextHour + ta.linearModel.Intercept
	if predicted < 0 {
		predicted = 0
	}
	return predicted
}

// GetCurrentTrends returns the current trend analysis
func (ta *TrendAnalyzer) GetCurrentTrends() *ErrorTrendAnalysis {
	ta.mu.RLock()
	defer ta.mu.RUnlock()

	// Return a copy to prevent external modification
	if ta.currentTrends == nil {
		return NewErrorTrendAnalysis()
	}

	// Deep copy the trends
	trends := &ErrorTrendAnalysis{
		HourlyErrorCounts:   make(map[int]int64),
		DailyErrorCounts:    make(map[string]int64),
		WeeklyErrorCounts:   make(map[int]int64),
		ComponentTrends:     make(map[string]*ComponentErrorTrend),
		SeverityTrends:      make(map[GStreamerErrorSeverity]*SeverityTrend),
		RecoverySuccessRate: ta.currentTrends.RecoverySuccessRate,
		AvgRecoveryTime:     ta.currentTrends.AvgRecoveryTime,
		PredictedErrorRate:  ta.currentTrends.PredictedErrorRate,
		TrendDirection:      ta.currentTrends.TrendDirection,
		ConfidenceLevel:     ta.currentTrends.ConfidenceLevel,
	}

	// Copy maps
	for k, v := range ta.currentTrends.HourlyErrorCounts {
		trends.HourlyErrorCounts[k] = v
	}
	for k, v := range ta.currentTrends.DailyErrorCounts {
		trends.DailyErrorCounts[k] = v
	}
	for k, v := range ta.currentTrends.WeeklyErrorCounts {
		trends.WeeklyErrorCounts[k] = v
	}
	for k, v := range ta.currentTrends.ComponentTrends {
		trends.ComponentTrends[k] = &ComponentErrorTrend{
			Component:           v.Component,
			ErrorCount:          v.ErrorCount,
			LastErrorTime:       v.LastErrorTime,
			ErrorRate:           v.ErrorRate,
			MostCommonErrorType: v.MostCommonErrorType,
			TrendDirection:      v.TrendDirection,
			HealthScore:         v.HealthScore,
		}
	}
	for k, v := range ta.currentTrends.SeverityTrends {
		trends.SeverityTrends[k] = &SeverityTrend{
			Severity:       v.Severity,
			Count:          v.Count,
			Rate:           v.Rate,
			TrendDirection: v.TrendDirection,
			RecentIncrease: v.RecentIncrease,
		}
	}

	return trends
}

// Utility functions

func maxSeverity(a, b GStreamerErrorSeverity) GStreamerErrorSeverity {
	if a > b {
		return a
	}
	return b
}

func removeDuplicateStrings(slice []string) []string {
	keys := make(map[string]bool)
	result := []string{}

	for _, item := range slice {
		if !keys[item] {
			keys[item] = true
			result = append(result, item)
		}
	}

	return result
}
