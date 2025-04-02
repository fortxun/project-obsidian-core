#!/bin/bash
# Script to build a custom OpenTelemetry Collector with the QAN processor

set -e

# Ensure ocb (OpenTelemetry Collector Builder) is installed
if ! command -v ocb &> /dev/null; then
    echo "Installing OpenTelemetry Collector Builder..."
    go install go.opentelemetry.io/collector/cmd/builder@latest
fi

# Create a temporary builder config
cat > /tmp/builder-config.yaml << EOF
dist:
  name: obsidian-otel-collector
  description: "OpenTelemetry Collector distribution for Project Obsidian Core"
  output_path: ./build/bin
  otelcol_version: "0.96.0"

exporters:
  - gomod: github.com/open-telemetry/opentelemetry-collector-contrib/exporter/loggingexporter v0.96.0
  - gomod: github.com/open-telemetry/opentelemetry-collector-contrib/exporter/otlphttpexporter v0.96.0

receivers:
  - gomod: github.com/open-telemetry/opentelemetry-collector-contrib/receiver/mysqlreceiver v0.96.0
  - gomod: github.com/open-telemetry/opentelemetry-collector-contrib/receiver/postgresqlreceiver v0.96.0

processors:
  - gomod: github.com/open-telemetry/opentelemetry-collector-contrib/processor/batchprocessor v0.96.0
  - gomod: github.com/open-telemetry/opentelemetry-collector-contrib/processor/resourceprocessor v0.96.0
  - gomod: github.com/open-telemetry/opentelemetry-collector-contrib/processor/memorylimiterprocessor v0.96.0
  - gomod: github.com/project-obsidian-core/otel-collector/extension/qanprocessor v0.1.0
    path: ./otel-collector/extension/qanprocessor

extensions:
  - gomod: github.com/open-telemetry/opentelemetry-collector-contrib/extension/healthcheckextension v0.96.0

connectors:
  - gomod: github.com/open-telemetry/opentelemetry-collector-contrib/connector/spanmetricsconnector v0.96.0
EOF

# Create build directory
mkdir -p build/bin

# Build the custom collector
echo "Building custom OpenTelemetry Collector..."
cd "$(dirname "$0")/.."
ocb --config=/tmp/builder-config.yaml

echo "Build completed. Binary is located at: ./build/bin/obsidian-otel-collector"