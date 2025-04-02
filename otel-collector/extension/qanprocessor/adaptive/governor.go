package adaptive

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

const (
	// Load thresholds for adjusting intervals
	HighLoadThreshold    = 0.7  // 70% load
	CriticalLoadThreshold = 0.9  // 90% load

	// Alpha values for EWMA calculations
	FastEMAAlpha = 0.3 // Faster response to immediate changes
	SlowEMAAlpha = 0.05 // Slower response for long-term trends

	// Interval constraints
	MinimumInterval = 500 * time.Millisecond
	MaximumInterval = 60 * time.Second

	// Default jitter percentage
	DefaultJitterPercent = 0.1 // 10% jitter

	// State file name
	StateFileName = "governor_state.json"
)

// EMA implements Exponentially Weighted Moving Average calculations
type EMA struct {
	alpha     float64
	value     float64
	initValue bool
	mu        sync.RWMutex
}

// NewEMA creates a new EMA instance with the given alpha value
func NewEMA(alpha float64) *EMA {
	return &EMA{
		alpha:     alpha,
		value:     0,
		initValue: false,
	}
}

// Update adds a new data point to the EMA calculation
func (e *EMA) Update(value float64) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.initValue {
		// First value, just set it directly
		e.value = value
		e.initValue = true
		return
	}

	// Calculate new EMA value
	// EMA = previous_EMA + alpha * (current_value - previous_EMA)
	e.value = e.value + e.alpha*(value-e.value)
}

// Value returns the current EMA value
func (e *EMA) Value() float64 {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.value
}

// SetValue directly sets the EMA value (used for state restoration)
func (e *EMA) SetValue(value float64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.value = value
	e.initValue = true
}

// Reset clears the EMA state
func (e *EMA) Reset() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.value = 0
	e.initValue = false
}

// GovernorState represents the persistent state of the AdaptiveGovernor
type GovernorState struct {
	FastEMAValue float64       `json:"fast_ema_value"`
	SlowEMAValue float64       `json:"slow_ema_value"`
	Interval     time.Duration `json:"interval_ns"`
	Timestamp    time.Time     `json:"timestamp"`
}

// AdaptiveGovernor manages adaptive polling for MySQL metrics
type AdaptiveGovernor struct {
	logger          *zap.Logger
	fastEMA         *EMA
	slowEMA         *EMA
	baseInterval    time.Duration
	currentInterval atomic.Int64 // Store as nanoseconds
	jitterPercent   float64
	stateDir        string
	stateLock       sync.RWMutex
	lastSnapshotTime time.Time
	intervalChangeCb func(time.Duration)
}

// NewAdaptiveGovernor creates a new adaptive governor instance
func NewAdaptiveGovernor(logger *zap.Logger, baseInterval time.Duration, stateDir string) *AdaptiveGovernor {
	if baseInterval < MinimumInterval {
		baseInterval = MinimumInterval
	}

	g := &AdaptiveGovernor{
		logger:        logger.With(zap.String("component", "adaptive_governor")),
		fastEMA:       NewEMA(FastEMAAlpha),
		slowEMA:       NewEMA(SlowEMAAlpha),
		baseInterval:  baseInterval,
		jitterPercent: DefaultJitterPercent,
		stateDir:      stateDir,
	}

	// Initialize current interval to base interval
	g.currentInterval.Store(int64(baseInterval))

	// Try to restore state from disk
	g.restoreState()

	return g
}

// WithJitterPercent sets the jitter percentage (0-1)
func (g *AdaptiveGovernor) WithJitterPercent(percent float64) *AdaptiveGovernor {
	if percent < 0 {
		percent = 0
	} else if percent > 0.5 {
		percent = 0.5 // Cap at 50% jitter
	}

	g.jitterPercent = percent
	return g
}

// WithIntervalChangeCallback sets a callback function that will be called
// whenever the interval changes
func (g *AdaptiveGovernor) WithIntervalChangeCallback(cb func(time.Duration)) *AdaptiveGovernor {
	g.intervalChangeCb = cb
	return g
}

// ProcessLoadMetrics updates the governor with new load metrics
// The load value should be between 0 and 1, where 1 is 100% load
func (g *AdaptiveGovernor) ProcessLoadMetrics(load float64) {
	// Validate load is between 0 and 1
	if load < 0 {
		load = 0
	} else if load > 1 {
		load = 1
	}

	// Update EMAs
	g.fastEMA.Update(load)
	g.slowEMA.Update(load)

	// Log current load status at debug level
	g.logger.Debug("Updated load metrics",
		zap.Float64("load", load),
		zap.Float64("fast_ema", g.fastEMA.Value()),
		zap.Float64("slow_ema", g.slowEMA.Value()),
	)

	// Adjust interval based on current load
	g.adjustInterval()

	// Save state periodically (every minute)
	now := time.Now()
	if now.Sub(g.lastSnapshotTime) > time.Minute {
		g.saveState()
		g.lastSnapshotTime = now
	}
}

// adjustInterval updates the polling interval based on current load
func (g *AdaptiveGovernor) adjustInterval() {
	// Get current load estimates
	fastValue := g.fastEMA.Value()
	slowValue := g.slowEMA.Value()

	// Get current interval
	currentIntervalNanos := g.currentInterval.Load()
	currentInterval := time.Duration(currentIntervalNanos)

	// Determine new interval based on load thresholds
	var newInterval time.Duration

	// Adjust interval based on load thresholds
	if fastValue > CriticalLoadThreshold {
		// Critical load - max backoff
		newInterval = MaximumInterval
	} else if fastValue > HighLoadThreshold {
		// High load - exponential backoff
		loadRatio := fastValue / HighLoadThreshold
		multiplier := math.Pow(2, loadRatio-1) // Exponential scaling
		newInterval = time.Duration(float64(g.baseInterval) * multiplier)

		// Cap at maximum
		if newInterval > MaximumInterval {
			newInterval = MaximumInterval
		}
	} else {
		// Normal load - use base interval
		newInterval = g.baseInterval
	}

	// If the difference is significant (>10%), update the interval
	if math.Abs(float64(newInterval-currentInterval))/float64(currentInterval) > 0.1 {
		g.logger.Info("Adjusting collection interval",
			zap.Duration("old_interval", currentInterval),
			zap.Duration("new_interval", newInterval),
			zap.Float64("fast_load", fastValue),
			zap.Float64("slow_load", slowValue),
		)

		// Update the current interval
		g.currentInterval.Store(int64(newInterval))

		// Call the callback if registered
		if g.intervalChangeCb != nil {
			g.intervalChangeCb(newInterval)
		}
	}
}

// GetCurrentInterval returns the current collection interval with jitter applied
func (g *AdaptiveGovernor) GetCurrentInterval() time.Duration {
	intervalNanos := g.currentInterval.Load()
	interval := time.Duration(intervalNanos)

	// Apply jitter if configured
	if g.jitterPercent > 0 {
		jitterRange := float64(interval) * g.jitterPercent
		jitterAmount := time.Duration(jitterRange * (0.5 - rand.Float64())) // +/- jitterRange/2
		interval += jitterAmount

		// Ensure we don't go below minimum interval
		if interval < MinimumInterval {
			interval = MinimumInterval
		}
	}

	return interval
}

// GetBaseInterval returns the configured base interval
func (g *AdaptiveGovernor) GetBaseInterval() time.Duration {
	return g.baseInterval
}

// GetRawInterval returns the current collection interval without jitter
func (g *AdaptiveGovernor) GetRawInterval() time.Duration {
	return time.Duration(g.currentInterval.Load())
}

// Reset resets the governor to its initial state
func (g *AdaptiveGovernor) Reset() {
	g.fastEMA.Reset()
	g.slowEMA.Reset()
	g.currentInterval.Store(int64(g.baseInterval))
	g.lastSnapshotTime = time.Time{}

	// Attempt to delete state file
	if g.stateDir != "" {
		statePath := filepath.Join(g.stateDir, StateFileName)
		_ = os.Remove(statePath) // Ignore errors, it's best-effort
	}

	// Call the callback if registered
	if g.intervalChangeCb != nil {
		g.intervalChangeCb(g.baseInterval)
	}
}

// saveState persists the current governor state to disk
func (g *AdaptiveGovernor) saveState() {
	if g.stateDir == "" {
		return // No state directory configured
	}

	g.stateLock.Lock()
	defer g.stateLock.Unlock()

	// Create state directory if it doesn't exist
	if err := os.MkdirAll(g.stateDir, 0755); err != nil {
		g.logger.Error("Failed to create state directory", zap.Error(err), zap.String("dir", g.stateDir))
		return
	}

	// Create state object
	state := GovernorState{
		FastEMAValue: g.fastEMA.Value(),
		SlowEMAValue: g.slowEMA.Value(),
		Interval:     time.Duration(g.currentInterval.Load()),
		Timestamp:    time.Now(),
	}

	// Serialize to JSON
	data, err := json.Marshal(state)
	if err != nil {
		g.logger.Error("Failed to marshal governor state", zap.Error(err))
		return
	}

	// Write to file (atomically by writing to temp file first)
	statePath := filepath.Join(g.stateDir, StateFileName)
	tempPath := statePath + ".tmp"

	if err := os.WriteFile(tempPath, data, 0644); err != nil {
		g.logger.Error("Failed to write governor state file", zap.Error(err), zap.String("path", tempPath))
		return
	}

	if err := os.Rename(tempPath, statePath); err != nil {
		g.logger.Error("Failed to rename governor state file", zap.Error(err))
		return
	}

	g.logger.Debug("Saved governor state to disk", zap.String("path", statePath))
}

// restoreState attempts to restore governor state from disk
func (g *AdaptiveGovernor) restoreState() {
	if g.stateDir == "" {
		return // No state directory configured
	}

	g.stateLock.Lock()
	defer g.stateLock.Unlock()

	statePath := filepath.Join(g.stateDir, StateFileName)

	// Check if the state file exists
	info, err := os.Stat(statePath)
	if err != nil {
		if !os.IsNotExist(err) {
			g.logger.Error("Failed to stat governor state file", zap.Error(err), zap.String("path", statePath))
		}
		return // File doesn't exist or can't be accessed
	}

	// Check if the file is too old (> 1 hour)
	if time.Since(info.ModTime()) > time.Hour {
		g.logger.Info("Governor state file is too old, not restoring", 
			zap.Time("mod_time", info.ModTime()), 
			zap.Duration("age", time.Since(info.ModTime())))
		return
	}

	// Read the state file
	data, err := os.ReadFile(statePath)
	if err != nil {
		g.logger.Error("Failed to read governor state file", zap.Error(err), zap.String("path", statePath))
		return
	}

	// Parse the state
	var state GovernorState
	if err := json.Unmarshal(data, &state); err != nil {
		g.logger.Error("Failed to unmarshal governor state", zap.Error(err))
		return
	}

	// Restore state
	g.fastEMA.SetValue(state.FastEMAValue)
	g.slowEMA.SetValue(state.SlowEMAValue)
	g.currentInterval.Store(int64(state.Interval))
	g.lastSnapshotTime = state.Timestamp

	g.logger.Info("Restored governor state from disk",
		zap.Float64("fast_ema", state.FastEMAValue),
		zap.Float64("slow_ema", state.SlowEMAValue),
		zap.Duration("interval", state.Interval),
		zap.Time("timestamp", state.Timestamp))
}
