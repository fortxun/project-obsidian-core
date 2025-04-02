#!/bin/bash
# Script to test the QAN processor with real MySQL and PostgreSQL instances

set -e

# Default values
MYSQL_ENABLED=true
MYSQL_HOST="localhost"
MYSQL_PORT="3306"
MYSQL_USER="monitor_user"
MYSQL_PASS="password"

PG_ENABLED=true
PG_HOST="localhost"
PG_PORT="5432"
PG_USER="monitor_user"
PG_PASS="password"
PG_DB="postgres"

# Parse command line arguments
while [[ "$#" -gt 0 ]]; do
    case $1 in
        --no-mysql) MYSQL_ENABLED=false ;;
        --no-postgres) PG_ENABLED=false ;;
        --mysql-host) MYSQL_HOST="$2"; shift ;;
        --mysql-port) MYSQL_PORT="$2"; shift ;;
        --mysql-user) MYSQL_USER="$2"; shift ;;
        --mysql-pass) MYSQL_PASS="$2"; shift ;;
        --pg-host) PG_HOST="$2"; shift ;;
        --pg-port) PG_PORT="$2"; shift ;;
        --pg-user) PG_USER="$2"; shift ;;
        --pg-pass) PG_PASS="$2"; shift ;;
        --pg-db) PG_DB="$2"; shift ;;
        *) echo "Unknown parameter: $1"; exit 1 ;;
    esac
    shift
done

# Create test database containers if needed
if [ "$MYSQL_ENABLED" = true ] && ! docker ps | grep -q "obsidian-test-mysql"; then
    echo "Starting test MySQL container..."
    docker run -d --name obsidian-test-mysql \
        -e MYSQL_ROOT_PASSWORD=root \
        -e MYSQL_DATABASE=test \
        -e MYSQL_USER=$MYSQL_USER \
        -e MYSQL_PASSWORD=$MYSQL_PASS \
        -p ${MYSQL_PORT}:3306 \
        mysql:8.0 \
        --performance-schema=ON \
        --performance-schema-consumer-events-statements-current=ON \
        --performance-schema-consumer-events-statements-history=ON \
        --performance-schema-consumer-events-statements-history-long=ON
    
    echo "Waiting for MySQL to start..."
    sleep 15
    
    echo "Creating test data and permissions..."
    docker exec obsidian-test-mysql mysql -uroot -proot -e "
        GRANT SELECT, PROCESS, SHOW VIEW, REPLICATION CLIENT ON *.* TO '$MYSQL_USER'@'%';
        CREATE DATABASE IF NOT EXISTS test;
        USE test;
        CREATE TABLE IF NOT EXISTS users (id INT AUTO_INCREMENT PRIMARY KEY, name VARCHAR(100), email VARCHAR(100));
        INSERT INTO users (name, email) VALUES ('User 1', 'user1@example.com'), ('User 2', 'user2@example.com');
    "
    
    echo "Running test queries on MySQL..."
    for i in {1..50}; do
        docker exec obsidian-test-mysql mysql -u$MYSQL_USER -p$MYSQL_PASS -e "
            USE test;
            SELECT * FROM users WHERE id = 1;
            SELECT * FROM users WHERE name LIKE 'User%';
            SELECT COUNT(*) FROM users;
            INSERT INTO users (name, email) VALUES ('User $i', 'user$i@example.com');
            UPDATE users SET email = CONCAT('updated-', email) WHERE id = 1;
        " > /dev/null 2>&1
    done
fi

if [ "$PG_ENABLED" = true ] && ! docker ps | grep -q "obsidian-test-postgres"; then
    echo "Starting test PostgreSQL container..."
    docker run -d --name obsidian-test-postgres \
        -e POSTGRES_PASSWORD=$PG_PASS \
        -e POSTGRES_USER=$PG_USER \
        -e shared_preload_libraries=pg_stat_statements \
        -p ${PG_PORT}:5432 \
        postgres:13 \
        -c pg_stat_statements.max=10000 \
        -c pg_stat_statements.track=all
    
    echo "Waiting for PostgreSQL to start..."
    sleep 15
    
    echo "Creating test data and enabling pg_stat_statements..."
    docker exec obsidian-test-postgres psql -U $PG_USER -c "
        CREATE EXTENSION IF NOT EXISTS pg_stat_statements;
        CREATE TABLE IF NOT EXISTS users (id SERIAL PRIMARY KEY, name VARCHAR(100), email VARCHAR(100));
        INSERT INTO users (name, email) VALUES ('User 1', 'user1@example.com'), ('User 2', 'user2@example.com');
    "
    
    echo "Running test queries on PostgreSQL..."
    for i in {1..50}; do
        docker exec obsidian-test-postgres psql -U $PG_USER -c "
            SELECT * FROM users WHERE id = 1;
            SELECT * FROM users WHERE name LIKE 'User%';
            SELECT COUNT(*) FROM users;
            INSERT INTO users (name, email) VALUES ('User $i', 'user$i@example.com');
            UPDATE users SET email = CONCAT('updated-', email) WHERE id = 1;
        " > /dev/null 2>&1
    done
fi

# Create a temporary config file for the collector
CONFIG_DIR=$(mktemp -d)
cat > ${CONFIG_DIR}/otel-config.yaml << EOF
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
      http:
        endpoint: 0.0.0.0:4318

processors:
  batch:
    timeout: 1s

  qanprocessor:
    mysql:
      enabled: ${MYSQL_ENABLED}
      endpoint: ${MYSQL_HOST}:${MYSQL_PORT}
      username: ${MYSQL_USER}
      password: ${MYSQL_PASS}
      collection_interval: 10

    postgresql:
      enabled: ${PG_ENABLED}
      endpoint: ${PG_HOST}:${PG_PORT}
      username: ${PG_USER}
      password: ${PG_PASS}
      database: ${PG_DB}
      collection_interval: 10

exporters:
  logging:
    verbosity: detailed
    sampling_initial: 1
    sampling_thereafter: 1

service:
  pipelines:
    logs/qan:
      processors: [qanprocessor, batch]
      exporters: [logging]
EOF

# Run the collector with the test config if the binary exists
COLLECTOR_BIN="./build/bin/obsidian-otel-collector"

if [ ! -f "$COLLECTOR_BIN" ]; then
    echo "Collector binary not found. Building it first..."
    ./scripts/build-custom-collector.sh
fi

echo "Running collector with test configuration..."
echo "Press Ctrl+C to stop the test"

$COLLECTOR_BIN --config ${CONFIG_DIR}/otel-config.yaml

# Cleanup
rm -rf ${CONFIG_DIR}