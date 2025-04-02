// PostgreSQL QAN Tester
// This tool tests the PostgreSQL QAN processor functionality against a real PostgreSQL instance

package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	_ "github.com/lib/pq"
	"go.uber.org/zap"

	"github.com/project-obsidian-core/otel-collector/extension/qanprocessor/postgresql"
)

func main() {
	// Set up logger
	logger, err := zap.NewDevelopment()
	if err != nil {
		fmt.Printf("Failed to create logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	// Get PostgreSQL connection parameters
	host := getEnvWithDefault("PG_HOST", "localhost")
	port := getEnvWithDefault("PG_PORT", "5432")
	user := getEnvWithDefault("PG_USER", "postgres")
	pass := getEnvWithDefault("PG_PASS", "postgres")
	dbName := getEnvWithDefault("PG_DB", "postgres")

	endpoint := fmt.Sprintf("%s:%s", host, port)
	logger.Info("Testing PostgreSQL QAN processor",
		zap.String("host", host),
		zap.String("port", port),
		zap.String("user", user),
		zap.String("database", dbName))

	// Test connection and extension status
	connStr := fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable", user, pass, endpoint, dbName)
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		logger.Fatal("Failed to create PostgreSQL connection", zap.Error(err))
	}
	defer db.Close()

	// Test the connection
	err = db.Ping()
	if err != nil {
		logger.Fatal("Failed to connect to PostgreSQL", zap.Error(err))
	}
	logger.Info("Successfully connected to PostgreSQL")

	// Check if pg_stat_statements extension is installed
	var hasExtension bool
	err = db.QueryRow(
		"SELECT EXISTS(SELECT 1 FROM pg_extension WHERE extname = 'pg_stat_statements')").
		Scan(&hasExtension)
	if err != nil {
		logger.Fatal("Failed to check for pg_stat_statements extension", zap.Error(err))
	}

	if !hasExtension {
		logger.Fatal("pg_stat_statements extension is not installed. Please install it with:\n" +
			"1. Add pg_stat_statements to shared_preload_libraries in postgresql.conf\n" +
			"2. Restart PostgreSQL\n" +
			"3. Run 'CREATE EXTENSION pg_stat_statements;' in the database")
	}
	logger.Info("pg_stat_statements extension is installed")

	// Check shared_preload_libraries configuration
	var sharedLibs string
	err = db.QueryRow("SELECT current_setting('shared_preload_libraries')").Scan(&sharedLibs)
	if err != nil {
		logger.Error("Failed to check shared_preload_libraries", zap.Error(err))
	} else {
		logger.Info("shared_preload_libraries", zap.String("value", sharedLibs))
	}

	// Check if we can query pg_stat_statements
	var canQuery bool
	err = db.QueryRow("SELECT EXISTS(SELECT 1 FROM pg_stat_statements LIMIT 1)").Scan(&canQuery)
	if err != nil {
		logger.Fatal("Failed to query pg_stat_statements", zap.Error(err))
	}
	if !canQuery {
		logger.Fatal("Cannot query pg_stat_statements")
	}
	logger.Info("Successfully queried pg_stat_statements")

	// Generate some test queries
	logger.Info("Generating test queries...")
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS qan_test (
			id SERIAL PRIMARY KEY,
			name TEXT,
			value INTEGER
		);
		
		-- Insert some test data
		INSERT INTO qan_test (name, value) 
		SELECT 'test_' || i, i 
		FROM generate_series(1, 100) i
		ON CONFLICT DO NOTHING;
	`)
	if err != nil {
		logger.Error("Failed to create test table", zap.Error(err))
	}

	// Run different query patterns
	_, err = db.Exec("SELECT * FROM qan_test WHERE id < 10")
	if err != nil {
		logger.Error("Failed to run test query 1", zap.Error(err))
	}

	_, err = db.Exec("SELECT * FROM qan_test WHERE name LIKE 'test_%'")
	if err != nil {
		logger.Error("Failed to run test query 2", zap.Error(err))
	}

	_, err = db.Exec("SELECT AVG(value), SUM(value) FROM qan_test")
	if err != nil {
		logger.Error("Failed to run test query 3", zap.Error(err))
	}

	_, err = db.Exec("SELECT name, COUNT(*) FROM qan_test GROUP BY name HAVING COUNT(*) > 0")
	if err != nil {
		logger.Error("Failed to run test query 4", zap.Error(err))
	}

	// Check pg_stat_statements for our test queries
	rows, err := db.Query(`
		SELECT 
			substring(query, 1, 50) AS query_sample,
			calls,
			total_exec_time,
			rows,
			shared_blks_read,
			shared_blks_hit
		FROM pg_stat_statements
		WHERE query LIKE '%qan_test%'
		ORDER BY total_exec_time DESC
		LIMIT 10
	`)
	if err != nil {
		logger.Error("Failed to query pg_stat_statements for test queries", zap.Error(err))
	} else {
		defer rows.Close()
		logger.Info("Test queries found in pg_stat_statements:")
		
		fmt.Printf("\n%-50s %-8s %-15s %-8s %-15s %-15s\n", 
			"Query Sample", "Calls", "Total Exec Time", "Rows", "Blks Read", "Blks Hit")
		fmt.Println(strings.Repeat("-", 120))
		
		for rows.Next() {
			var query string
			var calls int64
			var execTime float64
			var rowCount int64
			var blksRead, blksHit int64
			
			err := rows.Scan(&query, &calls, &execTime, &rowCount, &blksRead, &blksHit)
			if err != nil {
				logger.Error("Error scanning row", zap.Error(err))
				continue
			}
			
			fmt.Printf("%-50s %-8d %-15.2f %-8d %-15d %-15d\n", 
				query, calls, execTime, rowCount, blksRead, blksHit)
		}
		fmt.Println()
		
		if err = rows.Err(); err != nil {
			logger.Error("Error iterating rows", zap.Error(err))
		}
	}

	// Now test the QAN processor collector
	logger.Info("Testing the PostgreSQL QAN processor collector...")
	
	// Create snapshot store
	snapshotStore := postgresql.NewSnapshotStore()
	
	// Create collector
	collector, err := postgresql.NewCollector(
		logger,
		endpoint,
		user,
		pass,
		dbName,
		snapshotStore,
	)
	if err != nil {
		logger.Fatal("Failed to create PostgreSQL collector", zap.Error(err))
	}
	defer collector.Close()
	
	// Test first snapshot collection
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	_, err = collector.Collect(ctx)
	if err != nil {
		logger.Fatal("Failed to collect first snapshot", zap.Error(err))
	}
	logger.Info("Successfully collected first snapshot, waiting for second collection...")
	
	// Wait between snapshots - run a few more queries during this time
	for i := 0; i < 5; i++ {
		_, err = db.Exec(fmt.Sprintf("SELECT * FROM qan_test WHERE id < %d", 10+i))
		if err != nil {
			logger.Error("Failed to run additional test query", zap.Error(err))
		}
		time.Sleep(1 * time.Second)
	}
	
	// Test second snapshot with deltas
	logs, err := collector.Collect(ctx)
	if err != nil {
		logger.Fatal("Failed to collect second snapshot", zap.Error(err))
	}
	
	// Check logs
	logCount := logs.LogRecordCount()
	logger.Info("Collected logs", zap.Int("count", logCount))
	
	if logCount > 0 {
		if logs.ResourceLogs().Len() > 0 {
			rl := logs.ResourceLogs().At(0)
			logger.Info("Resource attributes", 
				zap.String("db.system", rl.Resource().Attributes().Get("db.system").Str()),
				zap.String("instance", rl.Resource().Attributes().Get("resource.instance.id").Str()))
			
			if rl.ScopeLogs().Len() > 0 {
				sl := rl.ScopeLogs().At(0)
				logger.Info("Scope", zap.String("name", sl.Scope().Name()))
				
				logger.Info("Sample log records:")
				limit := 5
				if sl.LogRecords().Len() < limit {
					limit = sl.LogRecords().Len()
				}
				
				for i := 0; i < limit; i++ {
					lr := sl.LogRecords().At(i)
					queryID := lr.Attributes().Get("db.query.id").Str()
					calls := lr.Attributes().Get("db.query.calls.delta").Int()
					execTime := lr.Attributes().Get("db.query.total_exec_time.delta").Double()
					rows := lr.Attributes().Get("db.query.rows.delta").Int()
					
					logger.Info("Log record",
						zap.String("query", lr.Body().Str()),
						zap.String("queryID", queryID),
						zap.Int64("calls", calls),
						zap.Float64("execTime", execTime),
						zap.Int64("rows", rows))
				}
			}
		}
	} else {
		logger.Warn("No log records collected - this might be normal if no queries were executed between snapshots")
	}
	
	logger.Info("PostgreSQL QAN processor test complete")
}

// getEnvWithDefault gets an environment variable value or returns the default value
func getEnvWithDefault(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}