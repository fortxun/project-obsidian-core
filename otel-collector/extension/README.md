# Project Obsidian Core - OpenTelemetry Extensions

This directory contains custom extensions for the OpenTelemetry Collector used in Project Obsidian Core.

## QAN Processor

The QAN (Query Analytics) processor is a custom OpenTelemetry processor that collects and processes query performance data from MySQL and PostgreSQL databases.

### Features

- Collects query performance data from MySQL `performance_schema.events_statements_summary_by_digest`
- Collects query performance data from PostgreSQL `pg_stat_statements`
- Calculates delta values between collection intervals
- Converts query data to OpenTelemetry logs format
- Provides configurable collection intervals

### Configuration

Example configuration in `otel-config.yaml`:

```yaml
processors:
  qanprocessor:
    mysql:
      enabled: true
      endpoint: "localhost:3306"
      username: "monitor_user"
      password: "password"
      collection_interval: 60

    postgresql:
      enabled: true
      endpoint: "localhost:5432"
      username: "monitor_user"
      password: "password"
      database: "postgres"
      collection_interval: 60

service:
  pipelines:
    logs/qan:
      receivers: []  # No direct receivers, data comes from processors
      processors: [qanprocessor, batch]
      exporters: [otlphttp]
```

### Implementation Details

The processor implements a snapshot-based differential analysis approach:

1. At each collection interval, a snapshot of all query metrics is taken
2. Deltas are calculated between the current and previous snapshots
3. Deltas are converted to OpenTelemetry logs with standardized attributes
4. Logs are sent to the configured exporters

### Building

To build the custom processor:

```bash
cd otel-collector/extension/qanprocessor
go build ./...
```

### Integration with OpenTelemetry Collector

This processor needs to be integrated with a custom build of the OpenTelemetry Collector. See the build scripts in the `/scripts` directory for more information.