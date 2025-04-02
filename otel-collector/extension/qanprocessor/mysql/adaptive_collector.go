package mysql

import (
	"context"
	"database/sql"
	"sync"
	"time"

	"go.opentelemetry.io/collector/pdata/plog"
	"go.uber.org/zap"

	"github.com/user/project-obsidian-core/otel-collector/extension/qanprocessor/adaptive"
)

// AdaptiveCollector extends the standard MySQL collector with
// adaptive polling capabilities.
type AdaptiveCollector struct {
	logger     *zap.Logger
	db         *sql.DB
	snapshotMgr *snapshotManager
	governor   *adaptive.AdaptiveGovernor
	collector  *adaptive.MySQLCollector
	callback   func(plog.Logs, error)
	pollMu     sync.Mutex
	ticker     *time.Ticker
	done       chan struct{}
}

// NewAdaptiveCollector creates a new adaptive MySQL QAN collector
func NewAdaptiveCollector(
	logger *zap.Logger,
	dsn string,
	username string,
	password string,
	database string,
	snapshotMgr *snapshotManager,
	baseInterval time.Duration,
	stateDir string,
) (*AdaptiveCollector, error) {
	// Initialize logger
	collectorLogger := logger.With(zap.String("component", "mysql_adaptive_collector"))

	// Create MySQL connection
	db, err := newMySQLConnection(dsn, username, password, database)
	if err != nil {
		return nil, err
	}

	// Create governor
	governor := adaptive.NewAdaptiveGovernor(logger, baseInterval, stateDir)

	// Create metrics collector
	metricsCollector := adaptive.NewMySQLCollector(logger, db)

	// Create collector
	return &AdaptiveCollector{
		logger:     collectorLogger,
		db:         db,
		snapshotMgr: snapshotMgr,
		governor:   governor,
		collector:  metricsCollector,
		done:       make(chan struct{}),
	}, nil
}

// StartCollection begins the adaptive collection process.
func (c *AdaptiveCollector) StartCollection(ctx context.Context, callback func(plog.Logs, error)) error {
	c.pollMu.Lock()
	defer c.pollMu.Unlock()

	// Stop any existing collection
	if c.ticker != nil {
		c.stopCollection()
	}

	c.callback = callback

	// Register interval change callback
	c.governor.WithIntervalChangeCallback(c.onIntervalChange)

	// Initialize with the current interval
	interval := c.governor.GetCurrentInterval()
	c.ticker = time.NewTicker(interval)

	// Start collection goroutine
	go c.collectionLoop(ctx)

	c.logger.Info("Started adaptive collection", zap.Duration("initial_interval", interval))

	return nil
}

// StopCollection stops the ongoing collection.
func (c *AdaptiveCollector) StopCollection() {
	c.pollMu.Lock()
	defer c.pollMu.Unlock()

	c.stopCollection()
}

// stopCollection is the internal (non-mutex-locked) version.
func (c *AdaptiveCollector) stopCollection() {
	if c.ticker != nil {
		c.ticker.Stop()
		c.ticker = nil
	}

	// Signal the collection loop to stop
	select {
	case c.done <- struct{}{}:
		// Successfully sent signal
	default:
		// Channel already closed or full, that's OK
	}

	c.logger.Info("Stopped adaptive collection")
}

// close properly closes all resources
func (c *AdaptiveCollector) close() error {
	// Stop collection first
	c.StopCollection()

	// Close database connection
	if c.db != nil {
		if err := c.db.Close(); err != nil {
			c.logger.Error("Error closing MySQL connection", zap.Error(err))
			return err
		}
	}

	return nil
}

// onIntervalChange is called by the governor when the interval changes
func (c *AdaptiveCollector) onIntervalChange(newInterval time.Duration) {
	c.pollMu.Lock()
	defer c.pollMu.Unlock()

	// Only update if we're actively collecting
	if c.ticker != nil {
		c.ticker.Reset(newInterval)
		c.logger.Info("Updated collection interval", zap.Duration("new_interval", newInterval))
	}
}

// collectionLoop is the main collection loop
func (c *AdaptiveCollector) collectionLoop(ctx context.Context) {
	// Set up a timeout context for each collection
	collectionTimeout := 30 * time.Second

	// Perform first collection immediately
	c.performCollection(ctx, collectionTimeout)

	for {
		select {
		case <-c.ticker.C:
			// It's time to collect metrics
			c.performCollection(ctx, collectionTimeout)

		case <-c.done:
			// We've been asked to stop
			return

		case <-ctx.Done():
			// Context canceled
			return
		}
	}
}

// performCollection does a single collection cycle
func (c *AdaptiveCollector) performCollection(ctx context.Context, timeout time.Duration) {
	// Create a timeout context for this collection
	ctxTimeout, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// First, collect MySQL metrics and update the governor
	load, err := c.collector.CollectMetrics(ctxTimeout)
	if err != nil {
		c.logger.Error("Error collecting MySQL metrics", zap.Error(err))
	} else {
		// Update the governor with the load metrics
		c.governor.ProcessLoadMetrics(load)
	}

	// Then perform QAN collection
	c.collectQANMetrics(ctxTimeout)
}

// collectQANMetrics collects the QAN metrics using the snapshot approach
func (c *AdaptiveCollector) collectQANMetrics(ctx context.Context) {
	// Get a new snapshot
	newSnapshot, err := takeSnapshot(ctx, c.db, c.logger)
	if err != nil {
		c.logger.Error("Failed to take performance_schema snapshot", zap.Error(err))
		if c.callback != nil {
			c.callback(plog.NewLogs(), err)
		}
		return
	}

	// Get the previous snapshot and calculate the diff
	prevSnapshot := c.snapshotMgr.getLastSnapshot()
	c.snapshotMgr.setLastSnapshot(newSnapshot)

	if prevSnapshot == nil {
		c.logger.Info("Initial snapshot captured, waiting for next interval")
		return
	}

	// Calculate the time between snapshots
	intervalSecs := newSnapshot.Timestamp.Sub(prevSnapshot.Timestamp).Seconds()
	if intervalSecs <= 0 {
		intervalSecs = 1 // Avoid division by zero
	}

	c.logger.Debug("Processing performance_schema snapshots", 
		zap.Float64("interval_seconds", intervalSecs),
		zap.Time("prev_time", prevSnapshot.Timestamp),
		zap.Time("curr_time", newSnapshot.Timestamp))

	// Convert to logs
	logs := processSnapshots(prevSnapshot, newSnapshot, c.logger)

	// Call the callback with the results
	if c.callback != nil {
		c.callback(logs, nil)
	}
}
