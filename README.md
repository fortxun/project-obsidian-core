# Project Obsidian Core

An adaptive monitoring system for database query analytics.

## Overview

Project Obsidian Core is a monitoring system that collects, processes, and analyzes database query performance data. It provides query analytics (QAN) for MySQL and PostgreSQL databases, with an adaptive monitoring governor that automatically adjusts collection intervals based on database load.

## Features

- Query Analytics (QAN) for MySQL and PostgreSQL databases
- OpenTelemetry integration for metrics collection and export
- Adaptive monitoring governor for intelligent polling
- Self-tuning collection intervals based on database load
- Support for both fixed and adaptive collection intervals
- State persistence for maintaining learned patterns

## Components

### Query Analytics Processor

The QAN processor collects query performance data from MySQL and PostgreSQL databases, processes it, and exports it as OpenTelemetry logs.

### Adaptive Monitoring Governor

The adaptive monitoring governor automatically adjusts collection intervals based on current database load. It uses EWMA (Exponentially Weighted Moving Average) calculations to track both short-term and long-term load trends, backing off during high load periods and returning to normal intervals when load decreases.

## Configuration

See the example configuration files in the `otel-collector/config` directory for detailed configuration options.

### MySQL Configuration with Adaptive Polling

```yaml
qanprocessor:
  mysql:
    enabled: true
    endpoint: "localhost:3306"
    username: "pmm"
    password: "password"
    collection_interval: "adaptive"
    adaptive:
      base_interval: 1  # Base interval in seconds
      state_directory: "/var/otel/governor_state"
```

### PostgreSQL Configuration

```yaml
qanprocessor:
  postgresql:
    enabled: true
    endpoint: "localhost:5432"
    username: "postgres"
    password: "password"
    database: "postgres"
    collection_interval: 60  # Fixed interval in seconds
```

## Building and Running

```bash
# Build the OTel collector with QAN processor
go build -o otelcol ./cmd/otelcol

# Run with a configuration file
./otelcol --config=./otel-collector/config/otel-config.yaml
```

## Documentation

See the `docs` directory for detailed documentation on each component.

## License

This project is licensed under the Apache License 2.0.
