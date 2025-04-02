# Adaptive Monitoring Governor

This document describes the self-adaptive monitoring governor component for MySQL QAN collection.

## Overview

The adaptive monitoring governor automatically adjusts the collection interval based on the current MySQL load. It uses an Exponentially Weighted Moving Average (EWMA) approach to track both short-term and long-term load trends, backing off during high load periods and returning to base intervals when the load decreases.

## How it Works

### Load Measurement

The governor uses the following metrics from MySQL to determine the current load:

- **Threads_running / Threads_connected ratio**: The primary indicator of load
- **Slow_queries / Questions ratio**: Secondary indicator that shows the percentage of slow queries

These metrics are combined into a normalized load value between 0 and 1, where higher values indicate higher load.

### EWMA Calculations

The governor uses two EWMA calculations:

1. **Fast EWMA (α=0.3)**: Responds quickly to sudden changes in load
2. **Slow EWMA (α=0.05)**: Provides a more stable, long-term view of the load

These are used together to make intelligent decisions about interval adjustments.

### Interval Adjustment

Intervals are adjusted based on load thresholds:

- **Normal Load (<70%)**: Base interval is used
- **High Load (70-90%)**: Exponential backoff is applied based on the load ratio
- **Critical Load (>90%)**: Maximum interval is used

The interval is only changed when there's a significant difference (>10%) to avoid frequent small adjustments.

### State Persistence

The governor periodically saves its state to disk, allowing it to restore its learned patterns after restarts. This ensures it doesn't have to rediscover the patterns from scratch every time.

## Configuration

To enable adaptive polling, set the collection interval to "adaptive" in your configuration:

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

Configuration parameters:

- **base_interval**: The starting point for interval calculations (in seconds)
- **state_directory**: Directory where governor state is persisted

## Benefits

- **Zero MySQL-side storage**: All state is maintained client-side
- **Self-tuning**: Automatically adapts to current workloads
- **Low overhead**: Backs off during high load periods
- **Graceful recovery**: Returns to normal intervals when load decreases
- **Pattern recognition**: Learns and remembers workload patterns

## Limitations

- Initial startup requires at least one collection interval to establish the baseline
- Very short-lived, extreme spikes might be missed between collection intervals
- State persistence requires a writable directory on the collector host

## Tuning Guidelines

The default settings should work for most environments, but consider these adjustments:

- For busier servers, increase the base_interval to 5-10 seconds
- For servers with frequent load spikes, you may want to decrease the fast EWMA alpha value to be more conservative
