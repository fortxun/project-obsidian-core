package adaptive

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"sync"
	"time"

	"go.uber.org/zap"
)

// MySQLMetrics contains the metrics collected from MySQL
type MySQLMetrics struct {
	ThreadsRunning    int
	ThreadsConnected  int
	Questions         int64
	SlowQueries       int64
	InnodbRowLockTime int64
	Uptime            int64
	Timestamp         time.Time
}

// CalculateLoad computes a load factor (0-1) based on the metrics
func (m *MySQLMetrics) CalculateLoad() float64 {
	// Simple calculation based on threads_running ratio
	// More sophisticated calculations could be implemented
	if m.ThreadsConnected <= 0 {
		return 0.0
	}

	// Calculate the ratio of running threads to connected threads
	ratio := float64(m.ThreadsRunning) / float64(m.ThreadsConnected)
	
	// Cap at 1.0 for normalization
	if ratio > 1.0 {
		ratio = 1.0
	}

	return ratio
}

// CalculateDiff returns the difference between two metrics snapshots
func (m *MySQLMetrics) CalculateDiff(previous *MySQLMetrics) *MySQLMetricsDiff {
	if previous == nil {
		return &MySQLMetricsDiff{
			ThreadsRunning:    m.ThreadsRunning,
			ThreadsConnected:  m.ThreadsConnected,
			Timestamp:         m.Timestamp,
			ElapsedSeconds:    0,
		}
	}

	// Calculate elapsed time in seconds
	elapsed := m.Timestamp.Sub(previous.Timestamp).Seconds()
	if elapsed <= 0 {
		elapsed = 1 // Avoid division by zero
	}

	return &MySQLMetricsDiff{
		ThreadsRunning:    m.ThreadsRunning,
		ThreadsConnected:  m.ThreadsConnected,
		QuestionsDiff:     m.Questions - previous.Questions,
		SlowQueriesDiff:   m.SlowQueries - previous.SlowQueries,
		LockTimeDiff:      m.InnodbRowLockTime - previous.InnodbRowLockTime,
		Timestamp:         m.Timestamp,
		ElapsedSeconds:    elapsed,
		QPS:               float64(m.Questions-previous.Questions) / elapsed,
		SlowQPS:           float64(m.SlowQueries-previous.SlowQueries) / elapsed,
	}
}

// MySQLMetricsDiff represents the difference between two metric snapshots
type MySQLMetricsDiff struct {
	ThreadsRunning    int
	ThreadsConnected  int
	QuestionsDiff     int64
	SlowQueriesDiff   int64
	LockTimeDiff      int64
	Timestamp         time.Time
	ElapsedSeconds    float64
	QPS               float64
	SlowQPS           float64
}

// CalculateLoad computes a composite load factor (0-1) based on the metrics diff
func (d *MySQLMetricsDiff) CalculateLoad() float64 {
	if d.ThreadsConnected <= 0 {
		return 0.0
	}

	// Calculate components that indicate load
	threadRatio := float64(d.ThreadsRunning) / float64(d.ThreadsConnected)
	if threadRatio > 1.0 {
		threadRatio = 1.0
	}

	// Slow queries as a percentage of total queries
	slowQueryRatio := 0.0
	if d.QuestionsDiff > 0 {
		slowQueryRatio = float64(d.SlowQueriesDiff) / float64(d.QuestionsDiff)
		if slowQueryRatio > 1.0 {
			slowQueryRatio = 1.0
		}
	}

	// Combine the factors with appropriate weights
	// Thread ratio is weighted highest as it's most indicative of current load
	total := threadRatio*0.7 + slowQueryRatio*0.3

	return total
}

// MySQLCollector collects metrics from MySQL for load calculation
type MySQLCollector struct {
	logger        *zap.Logger
	db            *sql.DB
	lastMetrics   *MySQLMetrics
	metricsMu     sync.RWMutex
}

// NewMySQLCollector creates a new MySQL metrics collector
func NewMySQLCollector(logger *zap.Logger, db *sql.DB) *MySQLCollector {
	return &MySQLCollector{
		logger: logger.With(zap.String("component", "mysql_collector")),
		db:     db,
	}
}

// CollectMetrics gathers current MySQL metrics and calculates load
func (c *MySQLCollector) CollectMetrics(ctx context.Context) (float64, error) {
	// Collect raw metrics
	metrics, err := c.collectMySQLMetrics(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to collect MySQL metrics: %w", err)
	}

	// Get last metrics for diff calculation
	c.metricsMu.RLock()
	last := c.lastMetrics
	c.metricsMu.RUnlock()

	// Calculate the diff
	diff := metrics.CalculateDiff(last)

	// Store current metrics for next time
	c.metricsMu.Lock()
	c.lastMetrics = metrics
	c.metricsMu.Unlock()

	// Calculate load based on the diff
	load := diff.CalculateLoad()

	// Log collection results at debug level
	c.logger.Debug("Collected MySQL metrics",
		zap.Int("threads_running", diff.ThreadsRunning),
		zap.Int("threads_connected", diff.ThreadsConnected),
		zap.Float64("qps", diff.QPS),
		zap.Float64("slow_qps", diff.SlowQPS),
		zap.Float64("calculated_load", load),
	)

	return load, nil
}

// collectMySQLMetrics collects the core metrics from MySQL
func (c *MySQLCollector) collectMySQLMetrics(ctx context.Context) (*MySQLMetrics, error) {
	// Query global status variables
	rows, err := c.db.QueryContext(ctx, `
		SELECT VARIABLE_NAME, VARIABLE_VALUE
		FROM performance_schema.global_status
		WHERE VARIABLE_NAME IN (
			'Threads_running',
			'Threads_connected',
			'Questions',
			'Slow_queries',
			'Innodb_row_lock_time',
			'Uptime'
		)
	`)
	if err != nil {
		return nil, fmt.Errorf("error querying global status: %w", err)
	}
	defer rows.Close()

	// Initialize metrics struct
	metrics := &MySQLMetrics{
		Timestamp: time.Now(),
	}

	// Process the results
	for rows.Next() {
		var name, valueStr string
		if err := rows.Scan(&name, &valueStr); err != nil {
			return nil, fmt.Errorf("error scanning row: %w", err)
		}

		// Convert string value to appropriate type
		switch name {
		case "Threads_running":
			val, _ := strconv.Atoi(valueStr)
			metrics.ThreadsRunning = val
		case "Threads_connected":
			val, _ := strconv.Atoi(valueStr)
			metrics.ThreadsConnected = val
		case "Questions":
			val, _ := strconv.ParseInt(valueStr, 10, 64)
			metrics.Questions = val
		case "Slow_queries":
			val, _ := strconv.ParseInt(valueStr, 10, 64)
			metrics.SlowQueries = val
		case "Innodb_row_lock_time":
			val, _ := strconv.ParseInt(valueStr, 10, 64)
			metrics.InnodbRowLockTime = val
		case "Uptime":
			val, _ := strconv.ParseInt(valueStr, 10, 64)
			metrics.Uptime = val
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return metrics, nil
}
