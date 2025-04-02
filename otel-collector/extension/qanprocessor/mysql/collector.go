// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.uber.org/zap"
)

// Collector handles the collection of MySQL QAN data.
type Collector struct {
	logger        *zap.Logger
	dsn           string
	db            *sql.DB
	snapshotStore *SnapshotStore
	instanceID    string
}

// NewCollector creates a new MySQL QAN data collector.
func NewCollector(
	logger *zap.Logger,
	endpoint string,
	username string,
	password string,
	database string,
	snapshotStore *SnapshotStore,
) (*Collector, error) {
	if database == "" {
		database = "information_schema" // Default database for connection
	}

	// Build DSN (Data Source Name)
	dsn := fmt.Sprintf("%s:%s@tcp(%s)/%s", username, password, endpoint, database)

	// Verify connection
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to create MySQL connection: %w", err)
	}

	// Test the connection
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to connect to MySQL: %w", err)
	}

	// Generate instance ID
	instanceID := fmt.Sprintf("mysql://%s/%s", endpoint, database)

	return &Collector{
		logger:        logger,
		dsn:           dsn,
		db:            db,
		snapshotStore: snapshotStore,
		instanceID:    instanceID,
	}, nil
}

// Collect gathers data from MySQL performance_schema and generates OTel logs.
func (c *Collector) Collect(ctx context.Context) (plog.Logs, error) {
	// Collect data from performance_schema
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
		c.logger.Info("First MySQL snapshot collected, deltas will be available on next collection")
		return plog.Logs{}, nil
	}

	// Calculate deltas between snapshots
	deltas := CalculateDeltas(prevSnapshot, snapshot)

	// Convert deltas to OpenTelemetry logs
	logs := c.deltaToLogs(deltas)

	return logs, nil
}

// collectSnapshot collects QAN data from MySQL and returns a snapshot.
func (c *Collector) collectSnapshot(ctx context.Context) (*Snapshot, error) {
	// Check if performance_schema is enabled
	var perfSchemaEnabled string
	err := c.db.QueryRowContext(ctx, "SHOW VARIABLES LIKE 'performance_schema'").Scan(nil, &perfSchemaEnabled)
	if err != nil {
		return nil, fmt.Errorf("failed to check performance_schema status: %w", err)
	}

	if perfSchemaEnabled != "ON" {
		return nil, fmt.Errorf("performance_schema is not enabled (status: %s)", perfSchemaEnabled)
	}

	// Check if statement digests are enabled
	var digestsEnabled string
	err = c.db.QueryRowContext(ctx, 
		"SELECT enabled FROM performance_schema.setup_consumers WHERE name = 'statements_digest'").
		Scan(&digestsEnabled)
	if err != nil {
		return nil, fmt.Errorf("failed to check statements_digest status: %w", err)
	}

	if digestsEnabled != "YES" {
		return nil, fmt.Errorf("performance_schema statements_digest consumer is not enabled")
	}

	// Query performance_schema for QAN data
	rows, err := c.db.QueryContext(ctx, `
		SELECT
			SCHEMA_NAME,
			DIGEST,
			DIGEST_TEXT,
			COUNT_STAR,
			SUM_TIMER_WAIT,
			SUM_LOCK_TIME,
			SUM_ERRORS,
			SUM_WARNINGS,
			SUM_ROWS_AFFECTED,
			SUM_ROWS_SENT,
			SUM_ROWS_EXAMINED,
			SUM_CREATED_TMP_TABLES,
			SUM_CREATED_TMP_DISK_TABLES,
			SUM_SORT_ROWS,
			SUM_NO_INDEX_USED,
			SUM_NO_GOOD_INDEX_USED
		FROM performance_schema.events_statements_summary_by_digest
		WHERE SCHEMA_NAME IS NOT NULL
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query performance_schema: %w", err)
	}
	defer rows.Close()

	// Process the result set
	snapshot := &Snapshot{
		Digests:    make(map[string]DigestData),
		Timestamp:  time.Now(),
		InstanceID: c.instanceID,
	}

	for rows.Next() {
		var data DigestData
		var schemaName, digest, digestText sql.NullString
		
		err := rows.Scan(
			&schemaName,
			&digest,
			&digestText,
			&data.CountStar,
			&data.SumTimerWait,
			&data.SumLockTime,
			&data.SumErrors,
			&data.SumWarnings,
			&data.SumRowsAffected,
			&data.SumRowsSent,
			&data.SumRowsExamined,
			&data.SumCreatedTmpTables,
			&data.SumCreatedTmpDiskTables,
			&data.SumSortRows,
			&data.SumNoIndexUsed,
			&data.SumNoGoodIndexUsed,
		)
		if err != nil {
			c.logger.Error("Error scanning row from performance_schema", zap.Error(err))
			continue
		}

		// Handle NULL values
		if !digest.Valid {
			continue // Skip rows without a digest
		}

		data.Digest = digest.String
		data.SchemaName = schemaName.String
		data.DigestText = digestText.String
		data.Timestamp = snapshot.Timestamp

		snapshot.Digests[data.Digest] = data
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating performance_schema rows: %w", err)
	}

	c.logger.Info("Collected MySQL QAN snapshot", 
		zap.Int("digest_count", len(snapshot.Digests)),
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
	attrs.PutStr("db.system", "mysql")
	attrs.PutStr("resource.instance.id", c.instanceID)
	
	scopeLogs := resourceLogs.ScopeLogs().AppendEmpty()
	scopeLogs.Scope().SetName("qanprocessor")
	
	// Convert each delta to a log record
	for _, delta := range deltas {
		// Skip digests with no execution count delta
		if delta.DeltaCountStar <= 0 {
			continue
		}
		
		logRecord := scopeLogs.LogRecords().AppendEmpty()
		
		// Set timestamp to current time (end of the aggregation interval)
		logRecord.SetTimestamp(pcommon.NewTimestampFromTime(time.Now()))
		logRecord.SetSeverityNumber(plog.SeverityNumberInfo)
		logRecord.SetSeverityText("INFO")
		
		// Set log attributes
		attrs := logRecord.Attributes()
		attrs.PutStr("db.statement.digest", delta.Digest)
		attrs.PutStr("db.statement.sample", delta.DigestText)
		attrs.PutStr("db.schema", delta.SchemaName)
		
		// Add all numeric delta values as attributes
		attrs.PutInt("db.query.calls.delta", delta.DeltaCountStar)
		attrs.PutInt("db.query.total_timer_wait.delta", delta.DeltaSumTimerWait)
		attrs.PutInt("db.query.lock_time.delta", delta.DeltaSumLockTime)
		attrs.PutInt("db.query.errors.delta", delta.DeltaSumErrors)
		attrs.PutInt("db.query.warnings.delta", delta.DeltaSumWarnings)
		attrs.PutInt("db.query.rows_affected.delta", delta.DeltaSumRowsAffected)
		attrs.PutInt("db.query.rows_sent.delta", delta.DeltaSumRowsSent)
		attrs.PutInt("db.query.rows_examined.delta", delta.DeltaSumRowsExamined)
		attrs.PutInt("db.query.created_tmp_tables.delta", delta.DeltaSumCreatedTmpTables)
		attrs.PutInt("db.query.created_tmp_disk_tables.delta", delta.DeltaSumCreatedTmpDiskTables)
		attrs.PutInt("db.query.sort_rows.delta", delta.DeltaSumSortRows)
		attrs.PutInt("db.query.no_index_used.delta", delta.DeltaSumNoIndexUsed)
		attrs.PutInt("db.query.no_good_index_used.delta", delta.DeltaSumNoGoodIndexUsed)
		
		// Add time period for rate calculations if needed
		attrs.PutDouble("db.query.time_period_seconds", delta.TimePeriodSecs)
		
		// Set the body as the digest text
		logRecord.Body().SetStr(delta.DigestText)
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