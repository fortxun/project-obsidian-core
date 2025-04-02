// Test file for MySQL QAN collection

package test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/project-obsidian-core/otel-collector/extension/qanprocessor/mysql"
	"go.uber.org/zap"
)

// These tests require a real MySQL instance
// Environment variables:
// - MYSQL_HOST: MySQL host (default: localhost)
// - MYSQL_PORT: MySQL port (default: 3306)
// - MYSQL_USER: MySQL username (default: monitor_user)
// - MYSQL_PASS: MySQL password (default: password)

func TestMySQLSnapshotCollection(t *testing.T) {
	// Check if we should skip this test
	if os.Getenv("SKIP_MYSQL_TEST") == "true" {
		t.Skip("Skipping MySQL test")
	}

	// Get MySQL connection info from environment or use defaults
	host := getEnvWithDefault("MYSQL_HOST", "localhost")
	port := getEnvWithDefault("MYSQL_PORT", "3306")
	user := getEnvWithDefault("MYSQL_USER", "monitor_user")
	pass := getEnvWithDefault("MYSQL_PASS", "password")
	endpoint := fmt.Sprintf("%s:%s", host, port)

	// Set up logger
	logger, err := zap.NewDevelopment()
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Create snapshot store
	store := mysql.NewSnapshotStore()

	// Create collector
	collector, err := mysql.NewCollector(logger, endpoint, user, pass, "", store)
	if err != nil {
		t.Fatalf("Failed to create MySQL collector: %v", err)
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
	logger.Info("Waiting for MySQL workload...")
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
				digest := attrs.Get("db.statement.digest").Str()
				calls := attrs.Get("db.query.calls.delta").Int()
				timer := attrs.Get("db.query.total_timer_wait.delta").Int()
				logger.Info("Log attributes",
					zap.String("digest", digest),
					zap.Int64("calls", calls),
					zap.Int64("timer", timer))
			}
		}
	}
}

func getEnvWithDefault(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}