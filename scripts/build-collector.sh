#!/bin/bash
# Script to build the OpenTelemetry collector for Project Obsidian Core

set -e

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo -e "${GREEN}Building OpenTelemetry collector for Project Obsidian Core...${NC}"

# Check if Docker is available
if ! command -v docker &> /dev/null; then
    echo -e "${RED}Docker is not available. Please install Docker to build the collector.${NC}"
    exit 1
fi

# Build the collector using Docker
echo "Building Docker image for OpenTelemetry collector..."
docker build -t obsidian-core/otel-collector:latest ./otel-collector

# Verify the build
if [ $? -eq 0 ]; then
    echo -e "${GREEN}Successfully built OpenTelemetry collector!${NC}"
    echo "You can run the collector using:"
    echo "docker run -p 4317:4317 -p 4318:4318 -v $(pwd)/otel-collector/config:/etc/otel-collector obsidian-core/otel-collector:latest"
else
    echo -e "${RED}Failed to build OpenTelemetry collector.${NC}"
    exit 1
fi

# Tag the image with the current date
DATE_TAG=$(date +%Y%m%d)
docker tag obsidian-core/otel-collector:latest obsidian-core/otel-collector:${DATE_TAG}
echo -e "Tagged image as: obsidian-core/otel-collector:${DATE_TAG}"

echo -e "${GREEN}Build process completed!${NC}"