#!/bin/bash
# Local integration test for Project Obsidian Core QAN processors
# This script tests the QAN processors directly against local database instances

set -e

# Color codes for better visibility
BLUE='\033[0;34m'
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration (can be overridden with environment variables)
MYSQL_HOST=${MYSQL_HOST:-localhost}
MYSQL_PORT=${MYSQL_PORT:-3306}
MYSQL_USER=${MYSQL_USER:-root}
MYSQL_PASSWORD=${MYSQL_PASSWORD:-culo1234}

PG_HOST=${PG_HOST:-localhost}
PG_PORT=${PG_PORT:-5432}
PG_USER=${PG_USER:-postgres}
PG_PASS=${PG_PASSWORD:-postgres}
PG_DB=${PG_DB:-postgres}
PSQL_BIN=${PSQL_BIN:-/Applications/Postgres.app/Contents/Versions/17/bin}

# Log function
log() {
  local level=$1
  local message=$2
  local color=$NC
  
  case $level in
    INFO) color=$BLUE ;;
    SUCCESS) color=$GREEN ;;
    WARNING) color=$YELLOW ;;
    ERROR) color=$RED ;;
  esac
  
  echo -e "${color}[$level] $message${NC}"
}

# Function to test MySQL
test_mysql() {
  log INFO "Testing MySQL QAN processor against $MYSQL_HOST:$MYSQL_PORT"
  
  cd ../../otel-collector/extension/qanprocessor/test/scripts
  
  # Run the MySQL test
  export MYSQL_HOST=$MYSQL_HOST
  export MYSQL_PORT=$MYSQL_PORT
  export MYSQL_USER=$MYSQL_USER
  export MYSQL_PASSWORD=$MYSQL_PASSWORD
  
  log INFO "Running MySQL QAN test script..."
  
  if ./run_mysql_test.sh; then
    log SUCCESS "MySQL QAN test completed successfully"
    
    # Try to compile the Go test program
    log INFO "Compiling MySQL QAN tester..."
    
    if go build -o mysql_qan_tester mysql_qan_tester.go; then
      log SUCCESS "MySQL QAN tester compiled successfully"
      
      log INFO "Running MySQL QAN tester..."
      ./mysql_qan_tester
      
      # Clean up
      rm -f mysql_qan_tester
    else
      log ERROR "Failed to compile MySQL QAN tester"
    fi
  else
    log ERROR "MySQL QAN test failed"
  fi
}

# Function to test PostgreSQL
test_postgresql() {
  log INFO "Testing PostgreSQL QAN processor against $PG_HOST:$PG_PORT"
  
  cd ../../otel-collector/extension/qanprocessor/test/scripts
  
  # Run the PostgreSQL test
  export PG_HOST=$PG_HOST
  export PG_PORT=$PG_PORT
  export PG_USER=$PG_USER
  export PG_PASS=$PG_PASS
  export PG_DB=$PG_DB
  export PSQL_BIN=$PSQL_BIN
  
  log INFO "Running PostgreSQL QAN test script..."
  
  if ./run_postgres_test.sh; then
    log SUCCESS "PostgreSQL QAN test completed successfully"
    
    # Try to compile the Go test program
    log INFO "Compiling PostgreSQL QAN tester..."
    
    if go build -o postgres_qan_tester postgres_qan_tester.go; then
      log SUCCESS "PostgreSQL QAN tester compiled successfully"
      
      log INFO "Running PostgreSQL QAN tester..."
      ./postgres_qan_tester
      
      # Clean up
      rm -f postgres_qan_tester
    else
      log ERROR "Failed to compile PostgreSQL QAN tester"
    fi
  else
    log ERROR "PostgreSQL QAN test failed"
  fi
}

# Main function
main() {
  log INFO "Starting local integration test for Project Obsidian Core QAN processors"
  
  # Test MySQL
  test_mysql
  
  # Test PostgreSQL
  test_postgresql
  
  log INFO "Local integration test completed"
}

# Run main function
main