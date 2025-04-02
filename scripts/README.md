# Project Obsidian Core Scripts

This directory contains various utility scripts for building, testing, and running Project Obsidian Core components.

## Available Scripts

### build-collector.sh

Builds the OpenTelemetry collector Docker image for Project Obsidian Core.

```bash
./build-collector.sh
```

Requires Docker to be installed and running.

### build-custom-collector.sh

Custom build script for the OpenTelemetry collector when advanced customization is needed.

```bash
./build-custom-collector.sh
```

### run-qan-test.sh

Runs Query Analytics (QAN) tests for MySQL and PostgreSQL.

```bash
./run-qan-test.sh
```

### test-mysql-processor.sh

Tests the MySQL QAN processor specifically.

```bash
./test-mysql-processor.sh
```

### test-qan-mysql-direct.sh

Directly tests the MySQL QAN processor without Docker.

```bash
./test-qan-mysql-direct.sh
```

### test-qan-processor.sh

Tests both MySQL and PostgreSQL QAN processors.

```bash
./test-qan-processor.sh
```

### mysql-workload.sh

Generates test workload for MySQL to test the QAN processor.

```bash
./mysql-workload.sh
```

### postgres-workload.sh

Generates test workload for PostgreSQL to test the QAN processor.

```bash
./postgres-workload.sh
```