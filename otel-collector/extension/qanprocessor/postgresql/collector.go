// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package postgresql

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"time"

	_ "github.com/lib/pq"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.uber.org/zap"
)

// Collector handles the collection of PostgreSQL QAN data.
type Collector struct {
	logger        *zap.Logger
	connStr       string
	db            *sql.DB
	snapshotStore *SnapshotStore
	instanceID    string
}

// NewCollector creates a new PostgreSQL QAN data collector.
func NewCollector(
	logger *zap.Logger,
	endpoint string,
	username string,
	password string,
	database string,
	snapshotStore *SnapshotStore,
) (*Collector, error) {
	// Build connection string
	connStr := fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable", 
		username, password, endpoint, database)

	// Verify connection
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to create PostgreSQL connection: %w", err)
	}

	// Test the connection
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to connect to PostgreSQL: %w", err)
	}

	// Generate instance ID
	instanceID := fmt.Sprintf("postgresql://%s/%s", endpoint, database)

	return &Collector{
		logger:        logger,
		connStr:       connStr,
		db:            db,
		snapshotStore: snapshotStore,
		instanceID:    instanceID,
	}, nil
}

// Collect gathers data from PostgreSQL pg_stat_statements and generates OTel logs.
func (c *Collector) Collect(ctx context.Context) (plog.Logs, error) {
	// Collect data from pg_stat_statements
	snapshot, err := c.collectSnapshot(ctx)
	if err != nil {
		return plog.Logs{}, err
	}

	// Get previous snapshot to calculate deltas
	prevSnapshot := c.snapshotStore.GetSnapshot(c.instanceID)

	// Store the current snapshot for next time
	c.snapshotStore.StoreSnapshot(c.instanceID, snapshot)

	// If this is the first snapshot, we don't have deltas yet
	if prevSnapshot == nil {
		c.logger.Info("First PostgreSQL snapshot collected, deltas will be available on next collection")
		return plog.Logs{}, nil
	}

	// Calculate deltas between snapshots
	deltas := CalculateDeltas(prevSnapshot, snapshot)

	// Convert deltas to OpenTelemetry logs
	logs := c.deltaToLogs(deltas)

	return logs, nil
}

// collectSnapshot collects QAN data from PostgreSQL and returns a snapshot.
func (c *Collector) collectSnapshot(ctx context.Context) (*Snapshot, error) {
	// Check if pg_stat_statements extension is installed
	var hasExtension bool
	err := c.db.QueryRowContext(ctx, 
		"SELECT EXISTS(SELECT 1 FROM pg_extension WHERE extname = 'pg_stat_statements')").
		Scan(&hasExtension)
	if err != nil {
		return nil, fmt.Errorf("failed to check for pg_stat_statements extension: %w", err)
	}

	if !hasExtension {
		return nil, fmt.Errorf("pg_stat_statements extension is not installed")
	}

	// Query pg_stat_statements for QAN data
	rows, err := c.db.QueryContext(ctx, `
		SELECT
			queryid::text,
			userid::text,
			dbid::text,
			query,
			calls,
			total_plan_time,
			total_exec_time,
			rows,
			shared_blks_hit,
			shared_blks_read,
			shared_blks_dirtied,
			shared_blks_written,
			local_blks_hit,
			local_blks_read,
			local_blks_dirtied,
			local_blks_written,
			temp_blks_read,
			temp_blks_written,
			blk_read_time,
			blk_write_time
		FROM pg_stat_statements
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query pg_stat_statements: %w", err)
	}
	defer rows.Close()

	// Process the result set
	snapshot := &Snapshot{
		Queries:    make(map[string]QueryData),
		Timestamp:  time.Now(),
		InstanceID: c.instanceID,
	}

	for rows.Next() {
		var data QueryData
		
		err := rows.Scan(
			&data.QueryID,
			&data.UserID,
			&data.DBID,
			&data.Query,
			&data.Calls,
			&data.TotalPlanTime,
			&data.TotalExecTime,
			&data.Rows,
			&data.SharedBlksHit,
			&data.SharedBlksRead,
			&data.SharedBlksDirtied,
			&data.SharedBlksWritten,
			&data.LocalBlksHit,
			&data.LocalBlksRead,
			&data.LocalBlksDirtied,
			&data.LocalBlksWritten,
			&data.TempBlksRead,
			&data.TempBlksWritten,
			&data.BlkReadTime,
			&data.BlkWriteTime,
		)
		if err != nil {
			c.logger.Error("Error scanning row from pg_stat_statements", zap.Error(err))
			continue
		}

		data.Timestamp = snapshot.Timestamp
		snapshot.Queries[data.QueryID] = data
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating pg_stat_statements rows: %w", err)
	}

	c.logger.Info("Collected PostgreSQL QAN snapshot", 
		zap.Int("query_count", len(snapshot.Queries)),
		zap.String("instance", c.instanceID))

	return snapshot, nil
}

// deltaToLogs converts QAN deltas to OpenTelemetry logs.
func (c *Collector) deltaToLogs(deltas []DeltaResult) plog.Logs {
	logs := plog.NewLogs()
	
	// Skip if no deltas
	if len(deltas) == 0 {
		return logs
	}

	resourceLogs := logs.ResourceLogs().AppendEmpty()
	
	// Set resource attributes
	attrs := resourceLogs.Resource().Attributes()
	attrs.PutStr("service.name", "obsidian-core")
	attrs.PutStr("db.system", "postgresql")
	attrs.PutStr("resource.instance.id", c.instanceID)
	
	scopeLogs := resourceLogs.ScopeLogs().AppendEmpty()
	scopeLogs.Scope().SetName("qanprocessor")
	
	// Convert each delta to a log record
	for _, delta := range deltas {
		// Skip queries with no execution count delta
		if delta.DeltaCalls <= 0 {
			continue
		}
		
		logRecord := scopeLogs.LogRecords().AppendEmpty()
		
		// Set timestamp to current time (end of the aggregation interval)
		logRecord.SetTimestamp(pcommon.NewTimestampFromTime(time.Now()))
		logRecord.SetSeverityNumber(plog.SeverityNumberInfo)
		logRecord.SetSeverityText("INFO")
		
		// Set log attributes
		attrs := logRecord.Attributes()
		attrs.PutStr("db.query.id", delta.QueryID)
		attrs.PutStr("db.statement.sample", delta.Query)
		attrs.PutStr("db.user.id", delta.UserID)
		attrs.PutStr("db.name.id", delta.DBID)
		
		// Add all numeric delta values as attributes
		attrs.PutInt("db.query.calls.delta", delta.DeltaCalls)
		attrs.PutDouble("db.query.total_plan_time.delta", delta.DeltaTotalPlanTime)
		attrs.PutDouble("db.query.total_exec_time.delta", delta.DeltaTotalExecTime)
		attrs.PutInt("db.query.rows.delta", delta.DeltaRows)
		attrs.PutInt("db.query.shared_blks_hit.delta", delta.DeltaSharedBlksHit)
		attrs.PutInt("db.query.shared_blks_read.delta", delta.DeltaSharedBlksRead)
		attrs.PutInt("db.query.shared_blks_dirtied.delta", delta.DeltaSharedBlksDirtied)
		attrs.PutInt("db.query.shared_blks_written.delta", delta.DeltaSharedBlksWritten)
		attrs.PutInt("db.query.local_blks_hit.delta", delta.DeltaLocalBlksHit)
		attrs.PutInt("db.query.local_blks_read.delta", delta.DeltaLocalBlksRead)
		attrs.PutInt("db.query.local_blks_dirtied.delta", delta.DeltaLocalBlksDirtied)
		attrs.PutInt("db.query.local_blks_written.delta", delta.DeltaLocalBlksWritten)
		attrs.PutInt("db.query.temp_blks_read.delta", delta.DeltaTempBlksRead)
		attrs.PutInt("db.query.temp_blks_written.delta", delta.DeltaTempBlksWritten)
		attrs.PutDouble("db.query.blk_read_time.delta", delta.DeltaBlkReadTime)
		attrs.PutDouble("db.query.blk_write_time.delta", delta.DeltaBlkWriteTime)
		
		// Add rows examined as a synonym for compatibility
		attrs.PutInt("db.query.rows_examined.delta", delta.DeltaRows)
		
		// Add time period for rate calculations if needed
		attrs.PutDouble("db.query.time_period_seconds", delta.TimePeriodSecs)
		
		// Set the body as the query text
		logRecord.Body().SetStr(delta.Query)
	}
	
	return logs
}

// Close closes the database connection.
func (c *Collector) Close() error {
	if c.db != nil {
		return c.db.Close()
	}
	return nil
}