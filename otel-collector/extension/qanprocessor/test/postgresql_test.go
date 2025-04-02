// Test file for PostgreSQL QAN collection

package test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/project-obsidian-core/otel-collector/extension/qanprocessor/postgresql"
	"go.uber.org/zap"
)

// These tests require a real PostgreSQL instance
// Environment variables:
// - PG_HOST: PostgreSQL host (default: localhost)
// - PG_PORT: PostgreSQL port (default: 5432)
// - PG_USER: PostgreSQL username (default: monitor_user)
// - PG_PASS: PostgreSQL password (default: password)
// - PG_DB: PostgreSQL database (default: postgres)

func TestPostgreSQLSnapshotCollection(t *testing.T) {
	// Check if we should skip this test
	if os.Getenv("SKIP_POSTGRES_TEST") == "true" {
		t.Skip("Skipping PostgreSQL test")
	}

	// Get PostgreSQL connection info from environment or use defaults
	host := getEnvWithDefault("PG_HOST", "localhost")
	port := getEnvWithDefault("PG_PORT", "5432")
	user := getEnvWithDefault("PG_USER", "monitor_user")
	pass := getEnvWithDefault("PG_PASS", "password")
	db := getEnvWithDefault("PG_DB", "postgres")
	endpoint := fmt.Sprintf("%s:%s", host, port)

	// Set up logger
	logger, err := zap.NewDevelopment()
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Create snapshot store
	store := postgresql.NewSnapshotStore()

	// Create collector
	collector, err := postgresql.NewCollector(logger, endpoint, user, pass, db, store)
	if err != nil {
		t.Fatalf("Failed to create PostgreSQL collector: %v", err)
	}
	defer collector.Close()

	// Test first snapshot collection
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err = collector.Collect(ctx)
	if err != nil {
		t.Fatalf("Failed to collect first snapshot: %v", err)
	}

	// Generate some test load
	// This would be better with a real database, but we'll just wait
	logger.Info("Waiting for PostgreSQL workload...")
	time.Sleep(10 * time.Second)

	// Test second snapshot with deltas
	logs, err := collector.Collect(ctx)
	if err != nil {
		t.Fatalf("Failed to collect second snapshot: %v", err)
	}

	// Check logs
	logCount := logs.LogRecordCount()
	logger.Info("Collected logs", zap.Int("count", logCount))

	if logs.ResourceLogs().Len() > 0 {
		rl := logs.ResourceLogs().At(0)
		if rl.ScopeLogs().Len() > 0 {
			sl := rl.ScopeLogs().At(0)
			if sl.LogRecords().Len() > 0 {
				lr := sl.LogRecords().At(0)
				body := lr.Body().Str()
				logger.Info("Sample log record", zap.String("body", body))
				attrs := lr.Attributes()
				queryID := attrs.Get("db.query.id").Str()
				calls := attrs.Get("db.query.calls.delta").Int()
				execTime := attrs.Get("db.query.total_exec_time.delta").Double()
				logger.Info("Log attributes",
					zap.String("queryID", queryID),
					zap.Int64("calls", calls),
					zap.Float64("execTime", execTime))
			}
		}
	}
}