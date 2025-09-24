package gstreamer

import (
	"fmt"
	"runtime"
	"time"

	"github.com/sirupsen/logrus"
)

// MemoryVerificationReport contains detailed memory verification results
type MemoryVerificationReport struct {
	Timestamp          time.Time
	TotalObjects       int64
	ActiveObjects      int64
	ReleasedObjects    int64
	LeakedObjects      int64
	SuspectedLeaks     []MemoryLeak
	MemoryUsage        runtime.MemStats
	BufferPoolStats    BufferPoolStats
	Recommendations    []string
	CriticalIssues     []string
	PerformanceMetrics PerformanceMetrics
}

// BufferPoolStats contains buffer pool performance statistics
type BufferPoolStats struct {
	Hits       int64
	Misses     int64
	PoolSize   int
	HitRatio   float64
	Efficiency string
}

// PerformanceMetrics contains performance-related metrics
type PerformanceMetrics struct {
	GCCount          uint32
	GCPauseTotal     time.Duration
	AvgGCPause       time.Duration
	AllocRate        float64 // bytes per second
	MemoryEfficiency float64 // percentage
}

// MemoryVerifier provides comprehensive memory verification and analysis
type MemoryVerifier struct {
	logger *logrus.Entry
}

// NewMemoryVerifier creates a new memory verifier
func NewMemoryVerifier() *MemoryVerifier {
	return &MemoryVerifier{
		logger: logrus.WithField("component", "memory-verifier"),
	}
}

// VerifyMemoryManager performs comprehensive verification of memory manager state
func (mv *MemoryVerifier) VerifyMemoryManager(mm *MemoryManager) *MemoryVerificationReport {
	if mm == nil {
		return &MemoryVerificationReport{
			Timestamp:      time.Now(),
			CriticalIssues: []string{"Memory manager is nil"},
		}
	}

	report := &MemoryVerificationReport{
		Timestamp: time.Now(),
	}

	// Get memory manager stats
	stats := mm.GetMemoryStats()
	report.TotalObjects = stats.ObjectRefsCount
	report.ActiveObjects = stats.ObjectRefsCount // Active objects are tracked refs
	report.ReleasedObjects = 0                   // Not tracked in MemoryStatistics
	report.LeakedObjects = stats.LeaksDetected
	report.SuspectedLeaks = mm.CheckMemoryLeaks()

	// Get runtime memory stats
	runtime.ReadMemStats(&report.MemoryUsage)

	// Calculate buffer pool stats from available data
	report.BufferPoolStats = BufferPoolStats{
		Hits:     0, // Not tracked in MemoryStatistics
		Misses:   0, // Not tracked in MemoryStatistics
		PoolSize: int(stats.BufferPoolsCount),
		HitRatio: 0.0, // Cannot calculate without hit/miss data
	}

	// Determine buffer pool efficiency
	if report.BufferPoolStats.HitRatio >= 0.9 {
		report.BufferPoolStats.Efficiency = "Excellent"
	} else if report.BufferPoolStats.HitRatio >= 0.7 {
		report.BufferPoolStats.Efficiency = "Good"
	} else if report.BufferPoolStats.HitRatio >= 0.5 {
		report.BufferPoolStats.Efficiency = "Fair"
	} else {
		report.BufferPoolStats.Efficiency = "Poor"
	}

	// Calculate performance metrics
	report.PerformanceMetrics = PerformanceMetrics{
		GCCount:      report.MemoryUsage.NumGC,
		GCPauseTotal: time.Duration(report.MemoryUsage.PauseTotalNs),
	}

	if report.MemoryUsage.NumGC > 0 {
		report.PerformanceMetrics.AvgGCPause = report.PerformanceMetrics.GCPauseTotal / time.Duration(report.MemoryUsage.NumGC)
	}

	// Calculate memory efficiency
	if report.MemoryUsage.Sys > 0 {
		report.PerformanceMetrics.MemoryEfficiency = float64(report.MemoryUsage.Alloc) / float64(report.MemoryUsage.Sys) * 100
	}

	// Generate recommendations and identify critical issues
	mv.analyzeAndRecommend(report)

	return report
}

// analyzeAndRecommend analyzes the memory state and provides recommendations
func (mv *MemoryVerifier) analyzeAndRecommend(report *MemoryVerificationReport) {
	// Check for critical issues
	if report.LeakedObjects > 0 {
		report.CriticalIssues = append(report.CriticalIssues,
			fmt.Sprintf("Memory leaks detected: %d objects", report.LeakedObjects))
	}

	if report.ActiveObjects > 1000 {
		report.CriticalIssues = append(report.CriticalIssues,
			fmt.Sprintf("High number of active objects: %d", report.ActiveObjects))
	}

	if report.MemoryUsage.Alloc > 1024*1024*1024 { // 1GB
		report.CriticalIssues = append(report.CriticalIssues,
			fmt.Sprintf("High memory usage: %d bytes", report.MemoryUsage.Alloc))
	}

	if report.PerformanceMetrics.AvgGCPause > 100*time.Millisecond {
		report.CriticalIssues = append(report.CriticalIssues,
			fmt.Sprintf("High GC pause time: %v", report.PerformanceMetrics.AvgGCPause))
	}

	// Generate recommendations
	if report.BufferPoolStats.HitRatio < 0.7 {
		report.Recommendations = append(report.Recommendations,
			"Consider increasing buffer pool size to improve hit ratio")
	}

	if report.PerformanceMetrics.MemoryEfficiency < 50 {
		report.Recommendations = append(report.Recommendations,
			"Memory efficiency is low, consider optimizing allocations")
	}

	if report.ActiveObjects > report.ReleasedObjects*2 {
		report.Recommendations = append(report.Recommendations,
			"High ratio of active to released objects, check for proper cleanup")
	}

	if len(report.SuspectedLeaks) > 0 {
		report.Recommendations = append(report.Recommendations,
			"Investigate suspected memory leaks and ensure proper object lifecycle management")
	}

	if report.MemoryUsage.NumGC > 100 && report.PerformanceMetrics.AvgGCPause > 10*time.Millisecond {
		report.Recommendations = append(report.Recommendations,
			"Frequent GC with high pause times, consider reducing allocation rate")
	}
}

// PrintReport prints a formatted memory verification report
func (mv *MemoryVerifier) PrintReport(report *MemoryVerificationReport) {
	mv.logger.Info("=== Memory Verification Report ===")
	mv.logger.Infof("Timestamp: %s", report.Timestamp.Format(time.RFC3339))

	mv.logger.Info("--- Object Tracking ---")
	mv.logger.Infof("Total Objects: %d", report.TotalObjects)
	mv.logger.Infof("Active Objects: %d", report.ActiveObjects)
	mv.logger.Infof("Released Objects: %d", report.ReleasedObjects)
	mv.logger.Infof("Leaked Objects: %d", report.LeakedObjects)

	mv.logger.Info("--- Memory Usage ---")
	mv.logger.Infof("Allocated: %d bytes (%.2f MB)", report.MemoryUsage.Alloc, float64(report.MemoryUsage.Alloc)/1024/1024)
	mv.logger.Infof("System: %d bytes (%.2f MB)", report.MemoryUsage.Sys, float64(report.MemoryUsage.Sys)/1024/1024)
	mv.logger.Infof("Memory Efficiency: %.1f%%", report.PerformanceMetrics.MemoryEfficiency)

	mv.logger.Info("--- Buffer Pool ---")
	mv.logger.Infof("Hits: %d, Misses: %d", report.BufferPoolStats.Hits, report.BufferPoolStats.Misses)
	mv.logger.Infof("Hit Ratio: %.2f%% (%s)", report.BufferPoolStats.HitRatio*100, report.BufferPoolStats.Efficiency)
	mv.logger.Infof("Pool Size: %d", report.BufferPoolStats.PoolSize)

	mv.logger.Info("--- Performance ---")
	mv.logger.Infof("GC Count: %d", report.PerformanceMetrics.GCCount)
	mv.logger.Infof("Total GC Pause: %v", report.PerformanceMetrics.GCPauseTotal)
	mv.logger.Infof("Average GC Pause: %v", report.PerformanceMetrics.AvgGCPause)

	if len(report.CriticalIssues) > 0 {
		mv.logger.Warn("--- Critical Issues ---")
		for _, issue := range report.CriticalIssues {
			mv.logger.Warnf("⚠️  %s", issue)
		}
	}

	if len(report.Recommendations) > 0 {
		mv.logger.Info("--- Recommendations ---")
		for _, rec := range report.Recommendations {
			mv.logger.Infof("💡 %s", rec)
		}
	}

	if len(report.SuspectedLeaks) > 0 {
		mv.logger.Warn("--- Suspected Leaks ---")
		for _, leak := range report.SuspectedLeaks {
			mv.logger.Warnf("🔍 %s (ID: %x, Age: %v, RefCount: %d)",
				leak.ObjectType, leak.ObjectID, leak.Age, leak.RefCount)
		}
	}

	mv.logger.Info("=== End Report ===")
}

// VerifyDesktopCapture performs memory verification specifically for desktop capture
func (mv *MemoryVerifier) VerifyDesktopCapture(dc *DesktopCaptureGst) *MemoryVerificationReport {
	if dc == nil {
		return &MemoryVerificationReport{
			Timestamp:      time.Now(),
			CriticalIssues: []string{"Desktop capture is nil"},
		}
	}

	report := mv.VerifyMemoryManager(dc.memoryManager)

	// Add desktop capture specific checks
	if dc.isRunning {
		stats := dc.GetStats()
		if stats.FramesDropped > int64(float64(stats.FramesCapture)*0.1) { // More than 10% dropped
			report.CriticalIssues = append(report.CriticalIssues,
				fmt.Sprintf("High frame drop rate: %d/%d (%.1f%%)",
					stats.FramesDropped, stats.FramesCapture,
					float64(stats.FramesDropped)/float64(stats.FramesCapture)*100))
		}

		if stats.CurrentFPS < stats.AverageFPS*0.8 { // Current FPS significantly lower
			report.Recommendations = append(report.Recommendations,
				"Current FPS is significantly lower than average, check for performance issues")
		}
	}

	return report
}

// RunMemoryHealthCheck runs a comprehensive memory health check
func (mv *MemoryVerifier) RunMemoryHealthCheck(dc *DesktopCaptureGst) bool {
	report := mv.VerifyDesktopCapture(dc)
	mv.PrintReport(report)

	// Return true if no critical issues found
	return len(report.CriticalIssues) == 0
}
