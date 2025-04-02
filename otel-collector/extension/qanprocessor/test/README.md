# QAN Processor Tests

This directory contains tests for the QAN processor components.

## Prerequisites

To run these tests, you need:

1. MySQL 8.0+ and/or PostgreSQL 13+ instances
2. Users with appropriate permissions (see below)
3. Go 1.21+

## MySQL Test Prerequisites

- MySQL 8.0 or later
- User with `SELECT`, `PROCESS`, `SHOW VIEW`, `REPLICATION CLIENT` privileges
- Performance Schema enabled:
  ```ini
  performance_schema=ON
  performance_schema_consumer_events_statements_current=ON
  performance_schema_consumer_events_statements_history=ON
  performance_schema_consumer_events_statements_history_long=ON
  ```

## PostgreSQL Test Prerequisites

- PostgreSQL 13 or later
- User with appropriate access to `pg_stat_statements`
- `pg_stat_statements` extension installed and configured:
  ```ini
  shared_preload_libraries = 'pg_stat_statements'
  pg_stat_statements.max = 10000
  pg_stat_statements.track = all
  ```
- Extension created in the database:
  ```sql
  CREATE EXTENSION pg_stat_statements;
  ```

## Running Tests

### Using Docker Test Containers

The easiest way to run the tests is using the provided test script:

```bash
cd ../../../
./scripts/test-qan-processor.sh
```

This script will:
1. Start Docker containers for MySQL and PostgreSQL
2. Configure them with the required settings
3. Generate test load 
4. Build and run the collector with the QAN processor

### Using Existing Databases

To test with existing database instances:

```bash
# MySQL tests
cd otel-collector/extension/qanprocessor
MYSQL_HOST=your-mysql-host MYSQL_PORT=3306 MYSQL_USER=monitor_user MYSQL_PASS=password go test -v ./test -run TestMySQLSnapshotCollection

# PostgreSQL tests
cd otel-collector/extension/qanprocessor
PG_HOST=your-pg-host PG_PORT=5432 PG_USER=monitor_user PG_PASS=password PG_DB=postgres go test -v ./test -run TestPostgreSQLSnapshotCollection
```

To skip a specific database test:

```bash
# Skip MySQL test
SKIP_MYSQL_TEST=true go test -v ./test

# Skip PostgreSQL test
SKIP_POSTGRES_TEST=true go test -v ./test
```

## Test Cases

These tests verify that:

1. The collectors can connect to the databases
2. Performance schema/pg_stat_statements is properly configured
3. Snapshots are collected correctly
4. Deltas are calculated between snapshots
5. OpenTelemetry logs are generated with the expected attributes