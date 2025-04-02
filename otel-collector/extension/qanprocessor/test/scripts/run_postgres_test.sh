#!/bin/bash
# Script to test PostgreSQL QAN collection

set -e

# PostgreSQL connection parameters (can be overridden with environment variables)
export PG_HOST="${PG_HOST:-localhost}"
export PG_PORT="${PG_PORT:-5432}"
export PG_USER="${PG_USER:-postgres}"
export PG_PASS="${PG_PASS:-postgres}"
export PG_DB="${PG_DB:-postgres}"

# PostgreSQL bin directory
PSQL_BIN="${PSQL_BIN:-/Applications/Postgres.app/Contents/Versions/17/bin}"

echo "=== PostgreSQL QAN Test ==="
echo "Host: $PG_HOST"
echo "Port: $PG_PORT"
echo "User: $PG_USER"
echo "Database: $PG_DB"
echo ""

# Check if psql is available
if [ ! -x "$PSQL_BIN/psql" ]; then
  echo "Error: psql not found at $PSQL_BIN/psql"
  echo "Please set PSQL_BIN environment variable to your PostgreSQL bin directory"
  exit 1
fi

echo "=== Testing PostgreSQL configuration ==="
echo "Running SQL test script..."

# Run the SQL test script
$PSQL_BIN/psql -h "$PG_HOST" -p "$PG_PORT" -U "$PG_USER" -d "$PG_DB" -f "$(dirname "$0")/postgres_qan_test.sql"

echo ""
echo "=== Running Go test program ==="
echo "Compiling and running postgres_qan_tester.go..."

# Get the directory of this script
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../../../.." && pwd)"

# Build and run the test program (ensure we're in the right directory)
cd "$SCRIPT_DIR"
go build -o postgres_qan_tester postgres_qan_tester.go
./postgres_qan_tester

# Clean up
rm -f postgres_qan_tester

echo ""
echo "=== PostgreSQL QAN test complete ==="