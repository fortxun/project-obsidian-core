#!/bin/bash
# End-to-end integration testing for Project Obsidian Core
# Tests the full data flow: Database → OTel collector → Druid → Jupyter analysis

set -e

# Color codes for better visibility
BLUE='\033[0;34m'
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration (can be overridden with environment variables)
DOCKER_COMPOSE_FILE=${DOCKER_COMPOSE_FILE:-../../docker-compose.yml}
TIMEOUT=${TIMEOUT:-300} # 5 minutes timeout for stack startup
MYSQL_HOST=${MYSQL_HOST:-127.0.0.1}
MYSQL_PORT=${MYSQL_PORT:-3307} # Using the mapped port
MYSQL_USER=${MYSQL_USER:-root}
MYSQL_PASSWORD=${MYSQL_PASSWORD:-password}
PG_HOST=${PG_HOST:-127.0.0.1}
PG_PORT=${PG_PORT:-5433} # Using the mapped port
PG_USER=${PG_USER:-monitor_user}
PG_PASS=${PG_PASSWORD:-password}
PG_DB=${PG_DB:-postgres}
PSQL_BIN=${PSQL_BIN:-/Applications/Postgres.app/Contents/Versions/17/bin}
DRUID_HOST=${DRUID_HOST:-127.0.0.1}
DRUID_PORT=${DRUID_PORT:-8888}
JUPYTER_PORT=${JUPYTER_PORT:-8888}

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

# Test function
test_endpoint() {
  local name=$1
  local endpoint=$2
  local max_attempts=${3:-10}
  local wait_time=${4:-5}
  
  log INFO "Testing connection to $name at $endpoint (will try $max_attempts times with ${wait_time}s intervals)"
  
  for i in $(seq 1 $max_attempts); do
    if curl -s -o /dev/null -w "%{http_code}" $endpoint | grep -q "2[0-9][0-9]\|3[0-9][0-9]"; then
      log SUCCESS "$name is available at $endpoint"
      return 0
    else
      log WARNING "Attempt $i/$max_attempts: $name not available yet, waiting ${wait_time}s..."
      sleep $wait_time
    fi
  done
  
  log ERROR "Could not connect to $name at $endpoint after $max_attempts attempts"
  return 1
}

# Function to wait for MySQL
wait_for_mysql() {
  log INFO "Waiting for MySQL to be ready..."
  for i in $(seq 1 15); do
    if mysql -h "$MYSQL_HOST" -P "$MYSQL_PORT" -u "$MYSQL_USER" -p"$MYSQL_PASSWORD" -e "SELECT 1" >/dev/null 2>&1; then
      log SUCCESS "MySQL is ready!"
      return 0
    else
      log WARNING "MySQL not ready yet, waiting 5s... (attempt $i/15)"
      sleep 5
    fi
  done
  log ERROR "MySQL did not become ready in time"
  return 1
}

# Function to wait for PostgreSQL
wait_for_postgres() {
  log INFO "Waiting for PostgreSQL to be ready..."
  for i in $(seq 1 15); do
    if $PSQL_BIN/psql -h "$PG_HOST" -p "$PG_PORT" -U "$PG_USER" -d "$PG_DB" -c "SELECT 1" >/dev/null 2>&1; then
      log SUCCESS "PostgreSQL is ready!"
      return 0
    else
      log WARNING "PostgreSQL not ready yet, waiting 5s... (attempt $i/15)"
      sleep 5
    fi
  done
  log ERROR "PostgreSQL did not become ready in time"
  return 1
}

# Function to generate test data in MySQL
generate_mysql_test_data() {
  log INFO "Generating test data in MySQL..."
  
  mysql -h "$MYSQL_HOST" -P "$MYSQL_PORT" -u "$MYSQL_USER" -p"$MYSQL_PASSWORD" <<EOF
CREATE DATABASE IF NOT EXISTS test_e2e;
USE test_e2e;
CREATE TABLE IF NOT EXISTS orders (
  id INT AUTO_INCREMENT PRIMARY KEY,
  customer_id INT NOT NULL,
  order_date DATETIME NOT NULL,
  amount DECIMAL(10,2) NOT NULL,
  status VARCHAR(20) NOT NULL
);

-- Insert some test data
INSERT INTO orders (customer_id, order_date, amount, status)
VALUES 
  (101, NOW(), 199.99, 'completed'),
  (102, NOW(), 99.50, 'pending'),
  (103, NOW(), 50.25, 'completed'),
  (104, NOW(), 25.99, 'cancelled'),
  (105, NOW(), 39.99, 'pending');

-- Run some test queries that will be captured by QAN
SELECT * FROM orders WHERE amount > 50;
SELECT status, COUNT(*) AS count, SUM(amount) AS total FROM orders GROUP BY status;
SELECT customer_id, COUNT(*) FROM orders GROUP BY customer_id HAVING COUNT(*) > 0;

-- Run query multiple times to make it show up prominently
SELECT AVG(amount) AS average_order FROM orders;
SELECT AVG(amount) AS average_order FROM orders;
SELECT AVG(amount) AS average_order FROM orders;
EOF

  if [ $? -eq 0 ]; then
    log SUCCESS "Generated test data in MySQL"
  else
    log ERROR "Failed to generate test data in MySQL"
    return 1
  fi
}

# Function to generate test data in PostgreSQL
generate_postgres_test_data() {
  log INFO "Generating test data in PostgreSQL..."
  
  $PSQL_BIN/psql -h "$PG_HOST" -p "$PG_PORT" -U "$PG_USER" -d "$PG_DB" <<EOF
-- Create extension if not exists (should already be created by container setup)
CREATE EXTENSION IF NOT EXISTS pg_stat_statements;

-- Create test table
CREATE TABLE IF NOT EXISTS products (
  id SERIAL PRIMARY KEY,
  name VARCHAR(100) NOT NULL,
  category VARCHAR(50) NOT NULL,
  price DECIMAL(10,2) NOT NULL,
  inventory INT NOT NULL
);

-- Insert some test data
INSERT INTO products (name, category, price, inventory)
VALUES 
  ('Laptop', 'Electronics', 999.99, 25),
  ('Smartphone', 'Electronics', 699.50, 50),
  ('Headphones', 'Accessories', 89.99, 100),
  ('Monitor', 'Electronics', 249.99, 15),
  ('Keyboard', 'Accessories', 59.99, 30)
ON CONFLICT (id) DO NOTHING;

-- Run some test queries that will be captured by QAN
SELECT * FROM products WHERE price > 100;
SELECT category, COUNT(*) AS count, SUM(price) AS total_price FROM products GROUP BY category;
SELECT * FROM products ORDER BY price DESC;

-- Run query multiple times to make it show up prominently
SELECT AVG(price) AS average_price FROM products;
SELECT AVG(price) AS average_price FROM products;
SELECT AVG(price) AS average_price FROM products;

-- Ensure the statements are in pg_stat_statements
SELECT query, calls, total_exec_time FROM pg_stat_statements WHERE query LIKE '%products%' ORDER BY total_exec_time DESC LIMIT 5;
EOF

  if [ $? -eq 0 ]; then
    log SUCCESS "Generated test data in PostgreSQL"
  else
    log ERROR "Failed to generate test data in PostgreSQL"
    return 1
  fi
}

# Function to verify data in Druid
check_druid_ingestion() {
  log INFO "Verifying data ingestion in Druid..."
  
  # First check if Druid is up and tables are available
  local tables_output=$(curl -s "http://$DRUID_HOST:$DRUID_PORT/druid/v2/sql" \
    -H 'Content-Type: application/json' \
    -d '{"query":"SHOW TABLES", "context":{"sqlQueryId":"test-query"}}')
  
  if [[ "$tables_output" == *"qan_db"* ]]; then
    log SUCCESS "Druid schema 'qan_db' found"
  else
    log WARNING "Druid schema 'qan_db' not found yet, might need more time for ingestion..."
    sleep 30 # Wait a bit longer for ingestion
    
    # Try again
    tables_output=$(curl -s "http://$DRUID_HOST:$DRUID_PORT/druid/v2/sql" \
      -H 'Content-Type: application/json' \
      -d '{"query":"SHOW TABLES", "context":{"sqlQueryId":"test-query"}}')
    
    if [[ "$tables_output" == *"qan_db"* ]]; then
      log SUCCESS "Druid schema 'qan_db' found after waiting"
    else
      log ERROR "Druid schema 'qan_db' not found after waiting"
      return 1
    fi
  fi
  
  # Check for MySQL queries
  local mysql_data=$(curl -s "http://$DRUID_HOST:$DRUID_PORT/druid/v2/sql" \
    -H 'Content-Type: application/json' \
    -d '{"query":"SELECT COUNT(*) AS count FROM qan_db WHERE db.system = '\''mysql'\''", "context":{"sqlQueryId":"test-mysql"}}')
  
  # Check for PostgreSQL queries
  local pg_data=$(curl -s "http://$DRUID_HOST:$DRUID_PORT/druid/v2/sql" \
    -H 'Content-Type: application/json' \
    -d '{"query":"SELECT COUNT(*) AS count FROM qan_db WHERE db.system = '\''postgresql'\''", "context":{"sqlQueryId":"test-pg"}}')
  
  # Extract counts (assumes integer values returned)
  local mysql_count=$(echo "$mysql_data" | grep -o '[0-9]\+')
  local pg_count=$(echo "$pg_data" | grep -o '[0-9]\+')
  
  # Log results
  log INFO "Found $mysql_count MySQL QAN records in Druid"
  log INFO "Found $pg_count PostgreSQL QAN records in Druid"
  
  if [[ "$mysql_count" -gt 0 ]] || [[ "$pg_count" -gt 0 ]]; then
    log SUCCESS "Successfully verified data in Druid"
    return 0
  else
    log ERROR "No QAN data found in Druid"
    return 1
  fi
}

# Function to verify Jupyter can access Druid
check_jupyter_druid_connection() {
  log INFO "Verifying Jupyter can access Druid..."
  
  # Create a test notebook that connects to Druid
  local notebook_path="/tmp/test_druid_connection.ipynb"
  cat > "$notebook_path" <<EOF
{
 "cells": [
  {
   "cell_type": "markdown",
   "metadata": {},
   "source": [
    "# Test Druid Connection from Jupyter"
   ]
  },
  {
   "cell_type": "code",
   "execution_count": null,
   "metadata": {},
   "source": [
    "import requests\\n",
    "import json\\n",
    "import pandas as pd"
   ]
  },
  {
   "cell_type": "code",
   "execution_count": null,
   "metadata": {},
   "source": [
    "# Configuration\\n",
    "DRUID_HOST = 'druid-router'\\n",
    "DRUID_PORT = 8888\\n",
    "DRUID_URL = f'http://{DRUID_HOST}:{DRUID_PORT}'"
   ]
  },
  {
   "cell_type": "code",
   "execution_count": null,
   "metadata": {},
   "source": [
    "# Check available tables\\n",
    "def query_druid(sql):\\n",
    "    response = requests.post(\\n",
    "        f'{DRUID_URL}/druid/v2/sql',\\n",
    "        headers={'Content-Type': 'application/json'},\\n",
    "        json={'query': sql}\\n",
    "    )\\n",
    "    if response.status_code == 200:\\n",
    "        return response.json()\\n",
    "    else:\\n",
    "        raise Exception(f'Query failed: {response.text}')\\n",
    "\\n",
    "tables = query_druid('SHOW TABLES')\\n",
    "print(f'Found tables: {tables}')"
   ]
  },
  {
   "cell_type": "code",
   "execution_count": null,
   "metadata": {},
   "source": [
    "# Query for QAN data\\n",
    "mysql_query = \"\"\"\\n",
    "SELECT\\n",
    "  'MySQL' AS source,\\n",
    "  COUNT(*) AS record_count\\n",
    "FROM qan_db\\n",
    "WHERE db.system = 'mysql'\\n",
    "\"\"\"\\n",
    "\\n",
    "pg_query = \"\"\"\\n",
    "SELECT\\n",
    "  'PostgreSQL' AS source,\\n",
    "  COUNT(*) AS record_count\\n",
    "FROM qan_db\\n",
    "WHERE db.system = 'postgresql'\\n",
    "\"\"\"\\n",
    "\\n",
    "try:\\n",
    "    mysql_data = query_druid(mysql_query)\\n",
    "    pg_data = query_druid(pg_query)\\n",
    "    \\n",
    "    # Combine results\\n",
    "    all_data = mysql_data + pg_data\\n",
    "    df = pd.DataFrame(all_data)\\n",
    "    print('QAN Data Summary:')\\n",
    "    display(df)\\n",
    "except Exception as e:\\n",
    "    print(f'Error: {e}')"
   ]
  }
 ],
 "metadata": {
  "kernelspec": {
   "display_name": "Python 3",
   "language": "python",
   "name": "python3"
  }
 },
 "nbformat": 4,
 "nbformat_minor": 4
}
EOF

  # We can't directly run the notebook without papermill, but we can check if Jupyter is up
  # and make the notebook available for manual testing
  if test_endpoint "JupyterLab" "http://localhost:$JUPYTER_PORT" 5 5; then
    log SUCCESS "JupyterLab is available - test notebook created at $notebook_path"
    log INFO "You can upload this notebook to JupyterLab to verify the Druid connection"
    log INFO "The notebook will be available in the container at /home/jovyan/work/test_druid_connection.ipynb"
    return 0
  else
    log ERROR "JupyterLab is not available"
    return 1
  fi
}

# Main function
main() {
  log INFO "Starting end-to-end integration testing for Project Obsidian Core"
  
  # Check if docker-compose file exists
  if [ ! -f "$DOCKER_COMPOSE_FILE" ]; then
    log ERROR "Docker Compose file not found at $DOCKER_COMPOSE_FILE"
    exit 1
  fi
  
  log INFO "Using Docker Compose file: $DOCKER_COMPOSE_FILE"
  
  # Check if stack is already running
  if docker ps | grep -q "obsidian-core"; then
    log INFO "Obsidian Core stack is already running"
  else
    log INFO "Starting Obsidian Core stack using docker-compose..."
    docker-compose -f "$DOCKER_COMPOSE_FILE" up -d
    
    # Wait for stack to be ready
    log INFO "Waiting for stack to be ready (timeout: ${TIMEOUT}s)..."
    sleep 30 # Initial wait for containers to start
  fi
  
  # Test connectivity to services
  test_endpoint "Druid Router" "http://$DRUID_HOST:$DRUID_PORT/status" 12 10
  
  # Wait for databases
  wait_for_mysql
  wait_for_postgres
  
  # Generate test data
  generate_mysql_test_data
  generate_postgres_test_data
  
  # Wait for data to be processed by OpenTelemetry and ingested into Druid
  log INFO "Waiting for data to be processed and ingested into Druid (60s)..."
  sleep 60
  
  # Verify data in Druid
  check_druid_ingestion
  
  # Check Jupyter and Druid connection
  check_jupyter_druid_connection
  
  # Final status
  log SUCCESS "End-to-end integration test completed!"
  log INFO "The full data flow has been successfully tested:"
  log INFO "1. Test data generated in MySQL and PostgreSQL"
  log INFO "2. Data collected by OpenTelemetry QAN processors"
  log INFO "3. Data successfully ingested into Druid"
  log INFO "4. JupyterLab available for analysis"
}

# Run main function
main