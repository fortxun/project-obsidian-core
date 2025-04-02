#!/bin/bash
# Script to build the OpenTelemetry collector without Docker

set -e

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo -e "${GREEN}Building OpenTelemetry collector using Go...${NC}"

# Check if Go is available
if ! command -v go &> /dev/null; then
    echo -e "${RED}Go is not available. Please install Go to build the collector.${NC}"
    exit 1
fi

# Install OpenTelemetry Collector builder
go install go.opentelemetry.io/collector/cmd/builder@latest

# Create a config file for the builder
cat > ./otel-collector/builder-config.yaml << EOL
dist:
  name: obsidian-core-collector
  description: OpenTelemetry Collector for Project Obsidian Core
  output_path: ./bin
  otelcol_version: 0.96.0

receivers:
  - gomod: go.opentelemetry.io/collector/receiver/otlpreceiver v0.96.0

processors:
  - gomod: go.opentelemetry.io/collector/processor/batchprocessor v0.96.0

exporters:
  - gomod: go.opentelemetry.io/collector/exporter/loggingexporter v0.96.0

extensions:
  - gomod: go.opentelemetry.io/collector/extension/healthcheckextension v0.96.0
EOL

# Create bin directory
mkdir -p ./otel-collector/bin

# Run the builder
echo "Building collector using OpenTelemetry builder..."
cd ./otel-collector
${HOME}/go/bin/builder --config=builder-config.yaml

# Verify the build
if [ -f "./bin/obsidian-core-collector" ]; then
    echo -e "${GREEN}Successfully built OpenTelemetry collector!${NC}"
    echo "Binary available at: ./otel-collector/bin/obsidian-core-collector"
    echo "You can run the collector using:"
    echo "./otel-collector/bin/obsidian-core-collector --config=./otel-collector/config/otel-config.yaml"
else
    echo -e "${RED}Failed to build OpenTelemetry collector.${NC}"
    exit 1
fi

echo -e "${GREEN}Build process completed!${NC}"