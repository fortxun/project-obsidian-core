// MySQL QAN Tester
// This tool tests the MySQL QAN processor functionality against a real MySQL instance

package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"go.uber.org/zap"

	"github.com/project-obsidian-core/otel-collector/extension/qanprocessor/mysql"
)

func main() {
	// Set up logger
	logger, err := zap.NewDevelopment()
	if err != nil {
		fmt.Printf("Failed to create logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	// Get MySQL connection parameters
	host := getEnvWithDefault("MYSQL_HOST", "localhost")
	port := getEnvWithDefault("MYSQL_PORT", "3306")
	user := getEnvWithDefault("MYSQL_USER", "root")
	pass := getEnvWithDefault("MYSQL_PASSWORD", "password")
	database := getEnvWithDefault("MYSQL_DB", "information_schema")

	endpoint := fmt.Sprintf("%s:%s", host, port)
	logger.Info("Testing MySQL QAN processor",
		zap.String("host", host),
		zap.String("port", port),
		zap.String("user", user),
		zap.String("database", database))

	// Test connection and performance_schema status
	dsn := fmt.Sprintf("%s:%s@tcp(%s)/%s", user, pass, endpoint, database)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		logger.Fatal("Failed to create MySQL connection", zap.Error(err))
	}
	defer db.Close()

	// Test the connection
	err = db.Ping()
	if err != nil {
		logger.Fatal("Failed to connect to MySQL", zap.Error(err))
	}
	logger.Info("Successfully connected to MySQL")

	// Check if performance_schema is enabled
	var perfSchemaEnabled string
	err = db.QueryRow("SHOW VARIABLES LIKE 'performance_schema'").Scan(nil, &perfSchemaEnabled)
	if err != nil {
		logger.Fatal("Failed to check performance_schema status", zap.Error(err))
	}

	if perfSchemaEnabled != "ON" {
		logger.Fatal("Performance Schema is not enabled. Please enable it in MySQL configuration.")
	}
	logger.Info("Performance Schema is enabled")

	// Check if statement digests are enabled
	var digestsEnabled string
	err = db.QueryRow(
		"SELECT enabled FROM performance_schema.setup_consumers WHERE name = 'statements_digest'").
		Scan(&digestsEnabled)
	if err != nil {
		logger.Fatal("Failed to check statements_digest status", zap.Error(err))
	}

	if digestsEnabled != "YES" {
		logger.Fatal("Performance Schema statements_digest consumer is not enabled")
	}
	logger.Info("Statements digest consumer is enabled")

	// Generate some test queries
	logger.Info("Generating test queries...")
	_, err = db.Exec(`
		CREATE DATABASE IF NOT EXISTS test_qan;
		USE test_qan;
		CREATE TABLE IF NOT EXISTS qan_test (
			id INT AUTO_INCREMENT PRIMARY KEY,
			name VARCHAR(100),
			value INT
		);
		
		-- Insert some test data
		INSERT INTO qan_test (name, value) 
		SELECT CONCAT('test_', seq), seq 
		FROM (
			SELECT @seq:=@seq+1 AS seq 
			FROM information_schema.columns, (SELECT @seq:=0) AS init
			LIMIT 100
		) sub
		ON DUPLICATE KEY UPDATE name=name;
	`)
	if err != nil {
		logger.Error("Failed to create test table", zap.Error(err))
	}

	// Run different query patterns
	_, err = db.Exec("SELECT * FROM test_qan.qan_test WHERE id < 10")
	if err != nil {
		logger.Error("Failed to run test query 1", zap.Error(err))
	}

	_, err = db.Exec("SELECT * FROM test_qan.qan_test WHERE name LIKE 'test_%'")
	if err != nil {
		logger.Error("Failed to run test query 2", zap.Error(err))
	}

	_, err = db.Exec("SELECT AVG(value), SUM(value) FROM test_qan.qan_test")
	if err != nil {
		logger.Error("Failed to run test query 3", zap.Error(err))
	}

	_, err = db.Exec("SELECT name, COUNT(*) FROM test_qan.qan_test GROUP BY name HAVING COUNT(*) > 0")
	if err != nil {
		logger.Error("Failed to run test query 4", zap.Error(err))
	}

	// Check performance_schema for our test queries
	rows, err := db.Query(`
		SELECT 
			DIGEST_TEXT,
			COUNT_STAR,
			ROUND(SUM_TIMER_WAIT/1000000000, 2) AS time_ms,
			SUM_ROWS_EXAMINED,
			SUM_ROWS_SENT
		FROM performance_schema.events_statements_summary_by_digest
		WHERE DIGEST_TEXT LIKE '%qan_test%'
		ORDER BY SUM_TIMER_WAIT DESC
		LIMIT 10
	`)
	if err != nil {
		logger.Error("Failed to query performance_schema for test queries", zap.Error(err))
	} else {
		defer rows.Close()
		logger.Info("Test queries found in performance_schema:")
		
		fmt.Printf("\n%-60s %-8s %-10s %-15s %-10s\n", 
			"Query Sample", "Count", "Time (ms)", "Rows Examined", "Rows Sent")
		fmt.Println(strings.Repeat("-", 110))
		
		for rows.Next() {
			var digestText string
			var countStar int64
			var timeMs float64
			var rowsExamined, rowsSent int64
			
			err := rows.Scan(&digestText, &countStar, &timeMs, &rowsExamined, &rowsSent)
			if err != nil {
				logger.Error("Error scanning row", zap.Error(err))
				continue
			}
			
			// Truncate long digest text
			if len(digestText) > 60 {
				digestText = digestText[:57] + "..."
			}
			
			fmt.Printf("%-60s %-8d %-10.2f %-15d %-10d\n", 
				digestText, countStar, timeMs, rowsExamined, rowsSent)
		}
		fmt.Println()
		
		if err = rows.Err(); err != nil {
			logger.Error("Error iterating rows", zap.Error(err))
		}
	}

	// Now test the QAN processor collector
	logger.Info("Testing the MySQL QAN processor collector...")
	
	// Create snapshot store
	snapshotStore := mysql.NewSnapshotStore()
	
	// Create collector
	collector, err := mysql.NewCollector(
		logger,
		endpoint,
		user,
		pass,
		database,
		snapshotStore,
	)
	if err != nil {
		logger.Fatal("Failed to create MySQL collector", zap.Error(err))
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
		_, err = db.Exec(fmt.Sprintf("SELECT * FROM test_qan.qan_test WHERE id < %d", 10+i))
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
					digest := lr.Attributes().Get("db.statement.digest").Str()
					calls := lr.Attributes().Get("db.query.calls.delta").Int()
					waitTime := lr.Attributes().Get("db.query.total_timer_wait.delta").Int()
					rowsExamined := lr.Attributes().Get("db.query.rows_examined.delta").Int()
					
					logger.Info("Log record",
						zap.String("digest", digest),
						zap.String("query", lr.Body().Str()),
						zap.Int64("calls", calls),
						zap.Int64("waitTime", waitTime),
						zap.Int64("rowsExamined", rowsExamined))
				}
			}
		}
	} else {
		logger.Warn("No log records collected - this might be normal if no queries were executed between snapshots")
	}
	
	logger.Info("MySQL QAN processor test complete")
}

// getEnvWithDefault gets an environment variable value or returns the default value
func getEnvWithDefault(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}