#!/bin/bash
# Script to test MySQL QAN collection

set -e

# MySQL connection parameters (can be overridden with environment variables)
export MYSQL_HOST="${MYSQL_HOST:-localhost}"
export MYSQL_PORT="${MYSQL_PORT:-3306}"
export MYSQL_USER="${MYSQL_USER:-root}"
export MYSQL_PASSWORD="${MYSQL_PASSWORD:-password}"

echo "=== MySQL QAN Test ==="
echo "Host: $MYSQL_HOST"
echo "Port: $MYSQL_PORT"
echo "User: $MYSQL_USER"
echo ""

echo "=== Testing MySQL configuration ==="
echo "Checking Performance Schema status..."

mysql -h "$MYSQL_HOST" -P "$MYSQL_PORT" -u "$MYSQL_USER" -p"$MYSQL_PASSWORD" -e "
SHOW VARIABLES LIKE 'performance_schema';
SELECT enabled FROM performance_schema.setup_consumers WHERE name = 'statements_digest';
"

echo ""
echo "Running test queries to generate performance_schema data..."

mysql -h "$MYSQL_HOST" -P "$MYSQL_PORT" -u "$MYSQL_USER" -p"$MYSQL_PASSWORD" -e "
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

-- Run different query patterns
SELECT * FROM qan_test WHERE id < 10;
SELECT * FROM qan_test WHERE name LIKE 'test_%';
SELECT AVG(value), SUM(value) FROM qan_test;
SELECT name, COUNT(*) FROM qan_test GROUP BY name HAVING COUNT(*) > 0;
"

echo ""
echo "Checking performance_schema for our test queries..."

mysql -h "$MYSQL_HOST" -P "$MYSQL_PORT" -u "$MYSQL_USER" -p"$MYSQL_PASSWORD" -e "
SELECT 
  DIGEST_TEXT,
  COUNT_STAR,
  ROUND(SUM_TIMER_WAIT/1000000000, 2) AS time_ms,
  SUM_ROWS_EXAMINED,
  SUM_ROWS_SENT
FROM performance_schema.events_statements_summary_by_digest
WHERE DIGEST_TEXT LIKE '%qan_test%'
ORDER BY SUM_TIMER_WAIT DESC
LIMIT 10;
"

echo ""
echo "=== MySQL QAN test complete ==="