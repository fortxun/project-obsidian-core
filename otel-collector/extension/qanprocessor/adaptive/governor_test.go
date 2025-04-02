// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package adaptive

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

func TestNewAdaptiveGovernor(t *testing.T) {
	logger := zaptest.NewLogger(t)
	
	// Test with default options
	gov := NewAdaptiveGovernor(logger)
	assert.Equal(t, DefaultBaseInterval, gov.baseInterval)
	assert.Equal(t, DefaultJitterPercentage, gov.jitterPercent)
	assert.Equal(t, DefaultStateDir, gov.stateDir)
	
	// Test with custom options
	customInterval := 2 * time.Second
	customJitter := 0.2
	customStateDir := "/tmp/test_governor"
	
	gov = NewAdaptiveGovernor(logger,
		WithBaseInterval(customInterval),
		WithJitterPercentage(customJitter),
		WithStateDirectory(customStateDir),
	)
	
	assert.Equal(t, customInterval, gov.baseInterval)
	assert.Equal(t, customJitter, gov.jitterPercent)
	assert.Equal(t, customStateDir, gov.stateDir)
}

func TestAdaptiveGovernor_UpdateMetrics(t *testing.T) {
	logger := zaptest.NewLogger(t)
	
	// Create governor with test callback
	intervalChanged := false
	var newInterval time.Duration
	
	gov := NewAdaptiveGovernor(logger,
		WithBaseInterval(1*time.Second),
		WithIntervalChangeCallback(func(interval time.Duration) {
			intervalChanged = true
			newInterval = interval
		}),
	)
	
	// Test normal load
	gov.UpdateMetrics(MetricPoint{
		ThreadsRunning: 5, // 10% load (scaled by 2)
		QPS: 100,
	})
	
	// Should still be at base interval for normal load
	assert.Equal(t, gov.baseInterval, gov.GetCurrentInterval())
	
	// Test high load
	gov.UpdateMetrics(MetricPoint{
		ThreadsRunning: 45, // 90% load (scaled by 2)
		QPS: 500,
	})
	
	// Interval should increase
	assert.True(t, gov.GetCurrentInterval() > gov.baseInterval)
	assert.True(t, intervalChanged)
	assert.Equal(t, newInterval, gov.GetCurrentInterval())
	
	// Test critical load
	gov.UpdateMetrics(MetricPoint{
		ThreadsRunning: 50, // 100% load (scaled by 2)
		QPS: 1000,
	})
	
	// Should be at maximum interval for critical load
	assert.Equal(t, MaximumInterval, gov.GetCurrentInterval())
}

func TestAdaptiveGovernor_StateManagement(t *testing.T) {
	logger := zaptest.NewLogger(t)
	
	// Create temporary directory for state
	tempDir := t.TempDir()
	statePath := filepath.Join(tempDir, StateFileName)
	
	// Create governor with state in temp dir
	gov := NewAdaptiveGovernor(logger,
		WithStateDirectory(tempDir),
	)
	
	// Update metrics to set some state
	gov.UpdateMetrics(MetricPoint{
		ThreadsRunning: 40, // 80% load (scaled by 2)
		QPS: 500,
	})
	
	// Save state
	err := gov.saveState()
	require.NoError(t, err)
	
	// Verify state file was created
	_, err = os.Stat(statePath)
	require.NoError(t, err)
	
	// Create new governor and load state
	gov2 := NewAdaptiveGovernor(logger,
		WithStateDirectory(tempDir),
	)
	
	// Check if state was loaded correctly
	require.NoError(t, gov2.loadState())
	assert.True(t, gov2.fastEMA.Value() > 0)
	assert.True(t, gov2.slowEMA.Value() > 0)
	
	// Values should be similar between governors
	assert.InDelta(t, gov.fastEMA.Value(), gov2.fastEMA.Value(), 0.001)
	assert.InDelta(t, gov.slowEMA.Value(), gov2.slowEMA.Value(), 0.001)
}

func TestEMA(t *testing.T) {
	ema := NewEMA(0.3) // Fast alpha
	
	// Test first update
	ema.Update(100)
	assert.Equal(t, 100.0, ema.Value())
	
	// Test second update
	ema.Update(200)
	// Expected: 0.3*200 + 0.7*100 = 60 + 70 = 130
	assert.InDelta(t, 130.0, ema.Value(), 0.001)
	
	// Test third update
	ema.Update(300)
	// Expected: 0.3*300 + 0.7*130 = 90 + 91 = 181
	assert.InDelta(t, 181.0, ema.Value(), 0.001)
}

func BenchmarkAdaptiveGovernor_UpdateMetrics(b *testing.B) {
	logger, err := zap.NewDevelopment()
	require.NoError(b, err)
	
	gov := NewAdaptiveGovernor(logger)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Simulate varying load
		load := float64((i % 100) + 1)
		gov.UpdateMetrics(MetricPoint{
			ThreadsRunning: load / 2,
			QPS: load * 10,
			AvgLatency: load / 10,
			ConnectionCount: load,
		})
	}
}