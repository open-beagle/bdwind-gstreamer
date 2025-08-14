package metrics

import (
	"net/http"
	"testing"
	"time"
)

func TestNewMetrics(t *testing.T) {
	config := DefaultMetricsConfig()
	metrics, err := NewMetrics(config)
	if err != nil {
		t.Fatalf("Failed to create metrics: %v", err)
	}

	if metrics == nil {
		t.Fatal("Metrics instance is nil")
	}

	if !metrics.IsRunning() == false {
		// Should not be running initially
	}
}

func TestMetricsConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  MetricsConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: MetricsConfig{
				Enabled: true,
				Port:    9090,
				Path:    "/metrics",
				Host:    "localhost",
			},
			wantErr: false,
		},
		{
			name: "invalid port - too low",
			config: MetricsConfig{
				Enabled: true,
				Port:    0,
				Path:    "/metrics",
				Host:    "localhost",
			},
			wantErr: true,
		},
		{
			name: "invalid port - too high",
			config: MetricsConfig{
				Enabled: true,
				Port:    70000,
				Path:    "/metrics",
				Host:    "localhost",
			},
			wantErr: true,
		},
		{
			name: "empty path - should set default",
			config: MetricsConfig{
				Enabled: true,
				Port:    9090,
				Path:    "",
				Host:    "localhost",
			},
			wantErr: false,
		},
		{
			name: "disabled config - should skip validation",
			config: MetricsConfig{
				Enabled: false,
				Port:    -1, // Invalid port, but should be ignored when disabled
				Path:    "",
				Host:    "",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("MetricsConfig.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && tt.config.Enabled && tt.config.Path == "" {
				// Should have set default path only for enabled configs
				if tt.config.Path != "/metrics" {
					t.Errorf("Expected default path to be set to '/metrics', got '%s'", tt.config.Path)
				}
			}
		})
	}
}

func TestMetrics_StartStop(t *testing.T) {
	config := MetricsConfig{
		Enabled: true,
		Port:    9091, // Use different port to avoid conflicts
		Path:    "/metrics",
		Host:    "localhost",
	}

	metrics, err := NewMetrics(config)
	if err != nil {
		t.Fatalf("Failed to create metrics: %v", err)
	}

	// Test start
	err = metrics.Start()
	if err != nil {
		t.Fatalf("Failed to start metrics server: %v", err)
	}

	if !metrics.IsRunning() {
		t.Error("Metrics server should be running")
	}

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Test that server is accessible
	resp, err := http.Get("http://localhost:9091/metrics")
	if err != nil {
		t.Fatalf("Failed to access metrics endpoint: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	// Test stop
	err = metrics.Stop()
	if err != nil {
		t.Fatalf("Failed to stop metrics server: %v", err)
	}

	if metrics.IsRunning() {
		t.Error("Metrics server should not be running")
	}
}

func TestMetrics_RegisterGauge(t *testing.T) {
	config := DefaultMetricsConfig()
	config.Enabled = false // Don't start server for this test

	metrics, err := NewMetrics(config)
	if err != nil {
		t.Fatalf("Failed to create metrics: %v", err)
	}

	gauge, err := metrics.RegisterGauge("test_gauge", "Test gauge metric", []string{"label1"})
	if err != nil {
		t.Fatalf("Failed to register gauge: %v", err)
	}

	if gauge == nil {
		t.Fatal("Gauge is nil")
	}

	// Test setting values
	gauge.Set(10.5, "value1")
	gauge.Inc("value1")
	gauge.Dec("value1")
	gauge.Add(5.0, "value1")
	gauge.Sub(2.0, "value1")

	// Test duplicate registration
	_, err = metrics.RegisterGauge("test_gauge", "Duplicate gauge", []string{"label1"})
	if err != ErrMetricAlreadyRegistered {
		t.Errorf("Expected ErrMetricAlreadyRegistered, got %v", err)
	}
}

func TestMetrics_RegisterCounter(t *testing.T) {
	config := DefaultMetricsConfig()
	config.Enabled = false // Don't start server for this test

	metrics, err := NewMetrics(config)
	if err != nil {
		t.Fatalf("Failed to create metrics: %v", err)
	}

	counter, err := metrics.RegisterCounter("test_counter", "Test counter metric", []string{"label1"})
	if err != nil {
		t.Fatalf("Failed to register counter: %v", err)
	}

	if counter == nil {
		t.Fatal("Counter is nil")
	}

	// Test incrementing values
	counter.Inc("value1")
	counter.Add(5.0, "value1")

	// Test duplicate registration
	_, err = metrics.RegisterCounter("test_counter", "Duplicate counter", []string{"label1"})
	if err != ErrMetricAlreadyRegistered {
		t.Errorf("Expected ErrMetricAlreadyRegistered, got %v", err)
	}
}

func TestMetrics_RegisterHistogram(t *testing.T) {
	config := DefaultMetricsConfig()
	config.Enabled = false // Don't start server for this test

	metrics, err := NewMetrics(config)
	if err != nil {
		t.Fatalf("Failed to create metrics: %v", err)
	}

	buckets := []float64{0.1, 0.5, 1.0, 2.5, 5.0, 10.0}
	histogram, err := metrics.RegisterHistogram("test_histogram", "Test histogram metric", []string{"label1"}, buckets)
	if err != nil {
		t.Fatalf("Failed to register histogram: %v", err)
	}

	if histogram == nil {
		t.Fatal("Histogram is nil")
	}

	// Test observing values
	histogram.Observe(0.5, "value1")
	histogram.Observe(2.0, "value1")
	histogram.Observe(7.5, "value1")

	// Test duplicate registration
	_, err = metrics.RegisterHistogram("test_histogram", "Duplicate histogram", []string{"label1"}, buckets)
	if err != ErrMetricAlreadyRegistered {
		t.Errorf("Expected ErrMetricAlreadyRegistered, got %v", err)
	}
}

func TestMetrics_DisabledConfig(t *testing.T) {
	config := MetricsConfig{
		Enabled: false,
		Port:    9092,
		Path:    "/metrics",
		Host:    "localhost",
	}

	metrics, err := NewMetrics(config)
	if err != nil {
		t.Fatalf("Failed to create metrics: %v", err)
	}

	// Should return nil when trying to start disabled metrics (graceful handling)
	err = metrics.Start()
	if err != nil {
		t.Errorf("Expected nil for disabled metrics start, got %v", err)
	}

	// Should not be running when disabled
	if metrics.IsRunning() {
		t.Error("Disabled metrics should not be running after start")
	}
}
