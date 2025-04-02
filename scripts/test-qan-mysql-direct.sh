#!/bin/bash
# Script to test the QAN MySQL processor directly against a local MySQL instance

set -e

echo "Testing QAN MySQL processor directly against local MySQL instance..."

# MySQL connection parameters
MYSQL_HOST="localhost"
MYSQL_PORT="3306"
MYSQL_USER="root"
MYSQL_PASS="culo1234"

# Create a test directory
TEST_DIR=$(mktemp -d)
mkdir -p "${TEST_DIR}/mysql"

echo "Creating test project structure..."

# Create go.mod and go.sum
cd "${TEST_DIR}"
cat > go.mod << EOF
module qantest

go 1.21

require (
	github.com/go-sql-driver/mysql v1.7.1
	go.uber.org/zap v1.26.0
)
EOF

# Create a simplified version that doesn't depend on OpenTelemetry
cat > main.go << EOF
package main

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"go.uber.org/zap"
)

// DigestData represents a single row from MySQL performance_schema.events_statements_summary_by_digest
type DigestData struct {
	Digest                 string
	SchemaName             string
	DigestText             string
	CountStar              int64
	SumTimerWait           int64
	SumLockTime            int64
	SumErrors              int64
	SumWarnings            int64
	SumRowsAffected        int64
	SumRowsSent            int64
	SumRowsExamined        int64
	SumCreatedTmpTables    int64
	SumCreatedTmpDiskTables int64
	SumSortRows            int64
	SumNoIndexUsed         int64
	SumNoGoodIndexUsed     int64
	Timestamp              time.Time
}

// Snapshot represents a point-in-time collection of all statement digests
type Snapshot struct {
	Digests    map[string]DigestData
	Timestamp  time.Time
	InstanceID string
}

// SnapshotStore stores and manages snapshots for calculating deltas
type SnapshotStore struct {
	mu              sync.RWMutex
	latestSnapshots map[string]*Snapshot // Keyed by instanceID
}

// NewSnapshotStore creates a new snapshot store
func NewSnapshotStore() *SnapshotStore {
	return &SnapshotStore{
		latestSnapshots: make(map[string]*Snapshot),
	}
}

// StoreSnapshot stores a new snapshot for an instance
func (s *SnapshotStore) StoreSnapshot(instanceID string, snapshot *Snapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.latestSnapshots[instanceID] = snapshot
}

// GetSnapshot retrieves the latest snapshot for an instance
func (s *SnapshotStore) GetSnapshot(instanceID string) *Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.latestSnapshots[instanceID]
}

// DeltaResult holds the calculated delta between two snapshots for a single digest
type DeltaResult struct {
	Digest                     string
	SchemaName                 string
	DigestText                 string
	TimePeriodSecs             float64
	DeltaCountStar             int64
	DeltaSumTimerWait          int64
	DeltaSumLockTime           int64
	DeltaSumErrors             int64
	DeltaSumWarnings           int64
	DeltaSumRowsAffected       int64
	DeltaSumRowsSent           int64
	DeltaSumRowsExamined       int64
	DeltaSumCreatedTmpTables   int64
	DeltaSumCreatedTmpDiskTables int64
	DeltaSumSortRows           int64
	DeltaSumNoIndexUsed        int64
	DeltaSumNoGoodIndexUsed    int64
}

// CalculateDeltas computes the deltas between the previous and current snapshots
func CalculateDeltas(prev, curr *Snapshot) []DeltaResult {
	if prev == nil || curr == nil {
		return nil
	}

	results := make([]DeltaResult, 0)
	timeDiffSecs := curr.Timestamp.Sub(prev.Timestamp).Seconds()

	// Process all digests in the current snapshot
	for digest, currData := range curr.Digests {
		// Get previous data for this digest, if it exists
		prevData, exists := prev.Digests[digest]

		// If this is a new digest that didn't exist in the previous snapshot,
		// we consider all its values as the delta
		if !exists {
			results = append(results, DeltaResult{
				Digest:                     digest,
				SchemaName:                 currData.SchemaName,
				DigestText:                 currData.DigestText,
				TimePeriodSecs:             timeDiffSecs,
				DeltaCountStar:             currData.CountStar,
				DeltaSumTimerWait:          currData.SumTimerWait,
				DeltaSumLockTime:           currData.SumLockTime,
				DeltaSumErrors:             currData.SumErrors,
				DeltaSumWarnings:           currData.SumWarnings,
				DeltaSumRowsAffected:       currData.SumRowsAffected,
				DeltaSumRowsSent:           currData.SumRowsSent,
				DeltaSumRowsExamined:       currData.SumRowsExamined,
				DeltaSumCreatedTmpTables:   currData.SumCreatedTmpTables,
				DeltaSumCreatedTmpDiskTables: currData.SumCreatedTmpDiskTables,
				DeltaSumSortRows:           currData.SumSortRows,
				DeltaSumNoIndexUsed:        currData.SumNoIndexUsed,
				DeltaSumNoGoodIndexUsed:    currData.SumNoGoodIndexUsed,
			})
			continue
		}

		// Calculate deltas for existing digests
		// Handle potential counter resets (when current value is less than previous)
		var deltaCountStar int64
		if currData.CountStar >= prevData.CountStar {
			deltaCountStar = currData.CountStar - prevData.CountStar
		} else {
			deltaCountStar = currData.CountStar // Counter reset case
		}

		// Only include digests that have been executed during this interval
		if deltaCountStar > 0 {
			// Helper function to handle counter resets
			calcDelta := func(curr, prev int64) int64 {
				if curr >= prev {
					return curr - prev
				}
				return curr // Assume a reset occurred
			}

			results = append(results, DeltaResult{
				Digest:                     digest,
				SchemaName:                 currData.SchemaName,
				DigestText:                 currData.DigestText,
				TimePeriodSecs:             timeDiffSecs,
				DeltaCountStar:             deltaCountStar,
				DeltaSumTimerWait:          calcDelta(currData.SumTimerWait, prevData.SumTimerWait),
				DeltaSumLockTime:           calcDelta(currData.SumLockTime, prevData.SumLockTime),
				DeltaSumErrors:             calcDelta(currData.SumErrors, prevData.SumErrors),
				DeltaSumWarnings:           calcDelta(currData.SumWarnings, prevData.SumWarnings),
				DeltaSumRowsAffected:       calcDelta(currData.SumRowsAffected, prevData.SumRowsAffected),
				DeltaSumRowsSent:           calcDelta(currData.SumRowsSent, prevData.SumRowsSent),
				DeltaSumRowsExamined:       calcDelta(currData.SumRowsExamined, prevData.SumRowsExamined),
				DeltaSumCreatedTmpTables:   calcDelta(currData.SumCreatedTmpTables, prevData.SumCreatedTmpTables),
				DeltaSumCreatedTmpDiskTables: calcDelta(currData.SumCreatedTmpDiskTables, prevData.SumCreatedTmpDiskTables),
				DeltaSumSortRows:           calcDelta(currData.SumSortRows, prevData.SumSortRows),
				DeltaSumNoIndexUsed:        calcDelta(currData.SumNoIndexUsed, prevData.SumNoIndexUsed),
				DeltaSumNoGoodIndexUsed:    calcDelta(currData.SumNoGoodIndexUsed, prevData.SumNoGoodIndexUsed),
			})
		}
	}

	return results
}

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

	logger.Info("Successfully connected to MySQL",
		zap.String("endpoint", endpoint),
		zap.String("database", database),
		zap.String("instanceID", instanceID))

	return &Collector{
		logger:        logger,
		dsn:           dsn,
		db:            db,
		snapshotStore: snapshotStore,
		instanceID:    instanceID,
	}, nil
}

// Collect gathers data from MySQL performance_schema and generates OTel logs.
func (c *Collector) Collect(ctx context.Context) ([]DeltaResult, error) {
	// Collect data from performance_schema
	snapshot, err := c.collectSnapshot(ctx)
	if err != nil {
		return nil, err
	}

	// Get previous snapshot to calculate deltas
	prevSnapshot := c.snapshotStore.GetSnapshot(c.instanceID)

	// Store the current snapshot for next time
	c.snapshotStore.StoreSnapshot(c.instanceID, snapshot)

	// If this is the first snapshot, we don't have deltas yet
	if prevSnapshot == nil {
		c.logger.Info("First MySQL snapshot collected, deltas will be available on next collection")
		return nil, nil
	}

	// Calculate deltas between snapshots
	deltas := CalculateDeltas(prevSnapshot, snapshot)

	c.logger.Info("Generated deltas from MySQL QAN data",
		zap.Int("delta_count", len(deltas)))

	return deltas, nil
}

// collectSnapshot collects QAN data from MySQL and returns a snapshot.
func (c *Collector) collectSnapshot(ctx context.Context) (*Snapshot, error) {
	// Check if performance_schema is enabled
	var varName, perfSchemaEnabled string
	err := c.db.QueryRowContext(ctx, "SHOW VARIABLES LIKE 'performance_schema'").Scan(&varName, &perfSchemaEnabled)
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

// GenerateTestQueries runs some test queries to appear in performance_schema
func (c *Collector) GenerateTestQueries(i int) {
	c.db.Exec("SELECT 1+1")
	c.db.Exec("SELECT CURRENT_TIMESTAMP")
	c.db.Exec("SELECT * FROM information_schema.tables LIMIT 10")
	c.db.Exec("SELECT table_name, table_schema FROM information_schema.tables WHERE table_schema = 'information_schema' LIMIT 20")
	c.db.Exec(fmt.Sprintf("SELECT COUNT(*) FROM information_schema.tables WHERE table_name LIKE 'T%%' LIMIT %d", i))
}

// Close closes the database connection.
func (c *Collector) Close() error {
	if c.db != nil {
		return c.db.Close()
	}
	return nil
}

func main() {
	// Get MySQL connection info
	host := "${MYSQL_HOST}"
	port := "${MYSQL_PORT}"
	user := "${MYSQL_USER}"
	pass := "${MYSQL_PASS}"
	endpoint := fmt.Sprintf("%s:%s", host, port)

	// Setup logger
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	// Create snapshot store
	store := NewSnapshotStore()

	// Create collector
	collector, err := NewCollector(logger, endpoint, user, pass, "", store)
	if err != nil {
		logger.Error("Failed to create MySQL collector", zap.Error(err))
		return
	}
	defer collector.Close()

	// Test first snapshot collection
	logger.Info("Collecting first snapshot...")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err = collector.Collect(ctx)
	if err != nil {
		logger.Error("Failed to collect first snapshot", zap.Error(err))
		return
	}

	// Generate some test queries
	logger.Info("Generating test queries...")
	for i := 0; i < 10; i++ {
		collector.GenerateTestQueries(i)
		time.Sleep(100 * time.Millisecond)
	}

	// Wait a moment for the performance_schema to update
	logger.Info("Waiting for performance_schema to update...")
	time.Sleep(2 * time.Second)

	// Test second snapshot with deltas
	logger.Info("Collecting second snapshot...")
	deltas, err := collector.Collect(ctx)
	if err != nil {
		logger.Error("Failed to collect second snapshot", zap.Error(err))
		return
	}

	// Print results
	logger.Info("Collection results", zap.Int("delta_count", len(deltas)))

	if len(deltas) > 0 {
		// Print details for first few deltas
		maxDeltas := 5
		if len(deltas) < 5 {
			maxDeltas = len(deltas)
		}

		for i := 0; i < maxDeltas; i++ {
			delta := deltas[i]
			
			fmt.Printf("\nDelta Result %d:\n", i+1)
			fmt.Printf("  SQL: %s\n", delta.DigestText)
			fmt.Printf("  Digest: %s\n", delta.Digest)
			fmt.Printf("  Schema: %s\n", delta.SchemaName)
			fmt.Printf("  Calls: %d\n", delta.DeltaCountStar)
			fmt.Printf("  Execution Time: %d\n", delta.DeltaSumTimerWait)
			fmt.Printf("  Rows Examined: %d\n", delta.DeltaSumRowsExamined)
		}
	}

	logger.Info("Test completed successfully!")
}
EOF

# Install dependencies
cd "${TEST_DIR}"
echo "Installing dependencies..."
go mod tidy

# Run the test
echo "Running QAN processor test..."
echo "This will test the full snapshot/delta calculation process"
go run main.go

# Clean up
rm -rf "${TEST_DIR}"

echo "Test completed!"