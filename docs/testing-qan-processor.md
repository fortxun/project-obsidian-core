# Testing the QAN Processor

This document provides comprehensive instructions for testing the Query Analytics (QAN) processor with real MySQL and PostgreSQL databases.

## Overview

The QAN processor requires actual database instances with specific configurations to properly collect query analytics data. The testing framework provides multiple options for testing:

1. **Docker-based testing**: Automatically sets up test databases and workloads
2. **Component tests**: Tests individual collector components
3. **Full integration testing**: Tests the entire OpenTelemetry pipeline

## Prerequisites

- Docker and Docker Compose (for Docker-based testing)
- Go 1.21 or later
- MySQL 8.0+ and/or PostgreSQL 13+ (for external database testing)

## Testing Options

### Option 1: One-Step Testing with Docker (Recommended)

The simplest approach is to use the provided `run-qan-test.sh` script:

```bash
cd project-obsidian-core
./scripts/run-qan-test.sh
```

This script:
1. Builds the custom OpenTelemetry Collector
2. Starts Docker containers for MySQL and PostgreSQL
3. Configures the databases with the required settings
4. Generates test workloads
5. Runs component tests
6. Runs a full integration test with the collector
7. Cleans up when finished

Options:
```
./scripts/run-qan-test.sh [options]
Options:
  --mode <docker|external>  Test mode (default: docker)
  --timeout <seconds>       Docker container timeout (default: 600)
  --skip-build              Skip building the QAN processor
  --skip-mysql              Skip MySQL testing
  --skip-postgres           Skip PostgreSQL testing
  --help                    Display this help message
```

### Option 2: Manual Testing with Docker Compose

If you want more control over the testing process:

```bash
# Start the test environment
docker-compose -f docker/test-qan-processor.yml up -d

# Run the component tests
cd otel-collector/extension/qanprocessor
MYSQL_HOST=localhost MYSQL_PORT=13306 MYSQL_USER=monitor_user MYSQL_PASS=password \
PG_HOST=localhost PG_PORT=15432 PG_USER=monitor_user PG_PASS=password PG_DB=postgres \
go test -v ./test

# Clean up when done
docker-compose -f docker/test-qan-processor.yml down
```

### Option 3: Testing with External Databases

If you have existing MySQL and PostgreSQL instances:

```bash
# MySQL test
cd otel-collector/extension/qanprocessor
MYSQL_HOST=your-mysql-host MYSQL_PORT=3306 MYSQL_USER=monitor_user MYSQL_PASS=password \
go test -v ./test -run TestMySQLSnapshotCollection

# PostgreSQL test
cd otel-collector/extension/qanprocessor
PG_HOST=your-pg-host PG_PORT=5432 PG_USER=monitor_user PG_PASS=password PG_DB=postgres \
go test -v ./test -run TestPostgreSQLSnapshotCollection
```

## Database Requirements

### MySQL Configuration

- MySQL 8.0 or later
- Performance Schema enabled with statement digesting
- User with `SELECT`, `PROCESS`, `SHOW VIEW`, `REPLICATION CLIENT` privileges

### PostgreSQL Configuration

- PostgreSQL 13 or later
- `pg_stat_statements` extension installed and configured
- User with access to the pg_stat_statements view

## Verification

The test logs will show QAN data collection results. Successful tests should show:

1. Successful connection to the databases
2. Collection of snapshots
3. Calculation of deltas between snapshots
4. Generation of OpenTelemetry logs with query attributes

Example log output:
```
Collected MySQL QAN snapshot digest_count=127 instance="mysql://localhost:3306/information_schema"
Collected logs count=42
Sample log record body="SELECT * FROM users WHERE id = ?"
Log attributes digest="01bb3d3eb127b6937784053493c55ab2195d085b" calls=15 timer=12859342
```

## Troubleshooting

### MySQL Issues

- **Performance Schema not enabled**: Check MySQL configuration with `SHOW VARIABLES LIKE 'performance_schema'`
- **Statement digests not enabled**: Verify with `SELECT * FROM performance_schema.setup_consumers WHERE NAME LIKE 'events_statements%'`
- **Permission errors**: Verify user grants with `SHOW GRANTS FOR 'monitor_user'@'%'`

### PostgreSQL Issues

- **pg_stat_statements not installed**: Check with `SELECT * FROM pg_extension WHERE extname = 'pg_stat_statements'`
- **pg_stat_statements not in shared_preload_libraries**: Requires server restart after configuration
- **Permission errors**: Ensure the user has appropriate access to the extension