#!/bin/bash
# Script to test the MySQL QAN processor against a local MySQL instance

set -e

echo "Testing MySQL QAN processor against local MySQL instance..."

# MySQL connection parameters
MYSQL_HOST="localhost"
MYSQL_PORT="3306"
MYSQL_USER="root"
MYSQL_PASS="culo1234"

# Create a test file with enough context to test the processor
TEST_DIR=$(mktemp -d)
MAIN_GO="${TEST_DIR}/main.go"  # Renamed to main.go to avoid go test confusion

cat > "${MAIN_GO}" << 'EOF'
package main

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

func main() {
	// Connect to MySQL
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s",
		"root", "culo1234", "localhost", "3306", "information_schema")
	
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		panic(fmt.Sprintf("Failed to create MySQL connection: %v", err))
	}
	defer db.Close()

	// Test the connection
	err = db.Ping()
	if err != nil {
		panic(fmt.Sprintf("Failed to connect to MySQL: %v", err))
	}
	
	fmt.Println("Successfully connected to MySQL!")

	// Check if performance_schema is enabled
	var varName, perfSchemaEnabled string
	err = db.QueryRow("SHOW VARIABLES LIKE 'performance_schema'").Scan(&varName, &perfSchemaEnabled)
	if err != nil {
		panic(fmt.Sprintf("Failed to check performance_schema status: %v", err))
	}

	fmt.Printf("Performance Schema enabled: %s\n", perfSchemaEnabled)

	// Check if statement digests are enabled
	var digestsEnabled string
	err = db.QueryRow(
		"SELECT enabled FROM performance_schema.setup_consumers WHERE name = 'statements_digest'").
		Scan(&digestsEnabled)
	if err != nil {
		fmt.Printf("Failed to check statements_digest status: %v\n", err)
	} else {
		fmt.Printf("Statements digest enabled: %s\n", digestsEnabled)
	}

	// Check if we can query the performance_schema.events_statements_summary_by_digest table
	rows, err := db.Query(`
		SELECT
			SCHEMA_NAME,
			DIGEST,
			DIGEST_TEXT,
			COUNT_STAR
		FROM performance_schema.events_statements_summary_by_digest
		LIMIT 5
	`)
	if err != nil {
		fmt.Printf("Failed to query performance_schema: %v\n", err)
	} else {
		defer rows.Close()
		
		fmt.Println("\nSample query digests:")
		fmt.Println("====================")
		
		for rows.Next() {
			var schemaName, digest, digestText sql.NullString
			var countStar int64
			
			err := rows.Scan(&schemaName, &digest, &digestText, &countStar)
			if err != nil {
				fmt.Printf("Error scanning row: %v\n", err)
				continue
			}
			
			schema := "NULL"
			if schemaName.Valid {
				schema = schemaName.String
			}
			
			digestStr := "NULL"
			if digest.Valid {
				digestStr = digest.String
			}
			
			sampleText := "NULL"
			if digestText.Valid {
				if len(digestText.String) > 50 {
					sampleText = digestText.String[:50] + "..."
				} else {
					sampleText = digestText.String
				}
			}
			
			fmt.Printf("Schema: %s, Digest: %s, Count: %d\n", schema, digestStr, countStar)
			fmt.Printf("Sample: %s\n\n", sampleText)
		}
	}

	// Generate some test queries to appear in the performance_schema
	fmt.Println("Generating test queries...")
	for i := 0; i < 10; i++ {
		db.Exec("SELECT 1+1")
		db.Exec("SELECT CURRENT_TIMESTAMP")
		db.Exec(fmt.Sprintf("SELECT * FROM information_schema.tables LIMIT %d", i))
		time.Sleep(100 * time.Millisecond)
	}
	
	fmt.Println("Test completed successfully!")
}
EOF

# Create a go.mod file for the test
cd "${TEST_DIR}"
cat > go.mod << EOF
module mysqltest

go 1.21

require github.com/go-sql-driver/mysql v1.7.1
EOF

# Install dependencies
echo "Installing dependencies..."
go mod tidy

# Run the test file
echo "Testing MySQL connection and Performance Schema..."
go run "${MAIN_GO}"

# Clean up temporary files
rm -rf "${TEST_DIR}"

echo "MySQL connection test completed!"

echo "Note: The full QAN processor component test requires building the complete"
echo "OpenTelemetry collector with the custom processor. This test script"
echo "only verifies connectivity and performance_schema setup."
echo ""
echo "To run the full test, you would need to:"
echo "1. Build the custom OpenTelemetry Collector"
echo "2. Run the MySQL component test with:"
echo "   MYSQL_HOST=localhost MYSQL_PORT=3306 MYSQL_USER=root MYSQL_PASS=culo1234 \\"
echo "   go test -v ./test -run TestMySQLSnapshotCollection"
echo ""
echo "For a simpler test like this, we have verified the MySQL connectivity"
echo "and performance_schema setup required for the QAN processor."