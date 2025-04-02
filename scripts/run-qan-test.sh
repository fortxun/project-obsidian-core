#!/bin/bash
# Master script to test the QAN processor with real MySQL and PostgreSQL instances

set -e

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
PROJECT_ROOT="$( cd "$SCRIPT_DIR/.." && pwd )"

# Default settings
TEST_MODE="docker"  # docker or external
DOCKER_TIMEOUT=600  # 10 minutes
SKIP_BUILD=false
SKIP_MYSQL=false
SKIP_POSTGRES=false

# Parse command line arguments
while [[ "$#" -gt 0 ]]; do
    case $1 in
        --mode) TEST_MODE="$2"; shift ;;
        --timeout) DOCKER_TIMEOUT="$2"; shift ;;
        --skip-build) SKIP_BUILD=true ;;
        --skip-mysql) SKIP_MYSQL=true ;;
        --skip-postgres) SKIP_POSTGRES=true ;;
        --help) 
            echo "Usage: run-qan-test.sh [options]"
            echo "Options:"
            echo "  --mode <docker|external>  Test mode (default: docker)"
            echo "  --timeout <seconds>       Docker container timeout (default: 600)"
            echo "  --skip-build              Skip building the QAN processor"
            echo "  --skip-mysql              Skip MySQL testing"
            echo "  --skip-postgres           Skip PostgreSQL testing"
            echo "  --help                    Display this help message"
            exit 0
            ;;
        *) echo "Unknown parameter: $1"; exit 1 ;;
    esac
    shift
done

# Build the QAN processor if needed
if [ "$SKIP_BUILD" = false ]; then
    echo "Building the QAN processor..."
    cd "$PROJECT_ROOT"
    if [ ! -d "$PROJECT_ROOT/build" ]; then
        mkdir -p "$PROJECT_ROOT/build"
    fi
    
    echo "Running build script..."
    bash "$PROJECT_ROOT/scripts/build-custom-collector.sh"
fi

# Testing with Docker containers
if [ "$TEST_MODE" = "docker" ]; then
    echo "Starting test environment with Docker..."
    cd "$PROJECT_ROOT"
    
    # Start Docker containers
    docker-compose -f docker/test-qan-processor.yml up -d
    
    echo "Waiting for containers to be ready..."
    sleep 15
    
    # Create environment file for the test
    TEST_ENV_FILE=$(mktemp)
    
    if [ "$SKIP_MYSQL" = false ]; then
        echo "MYSQL_HOST=localhost" >> "$TEST_ENV_FILE"
        echo "MYSQL_PORT=13306" >> "$TEST_ENV_FILE"
        echo "MYSQL_USER=monitor_user" >> "$TEST_ENV_FILE"
        echo "MYSQL_PASS=password" >> "$TEST_ENV_FILE"
    else
        echo "SKIP_MYSQL_TEST=true" >> "$TEST_ENV_FILE"
    fi
    
    if [ "$SKIP_POSTGRES" = false ]; then
        echo "PG_HOST=localhost" >> "$TEST_ENV_FILE"
        echo "PG_PORT=15432" >> "$TEST_ENV_FILE"
        echo "PG_USER=monitor_user" >> "$TEST_ENV_FILE"
        echo "PG_PASS=password" >> "$TEST_ENV_FILE"
        echo "PG_DB=postgres" >> "$TEST_ENV_FILE"
    else
        echo "SKIP_POSTGRES_TEST=true" >> "$TEST_ENV_FILE"
    fi
    
    # Run the component tests
    echo "Running component tests..."
    cd "$PROJECT_ROOT/otel-collector/extension/qanprocessor"
    cat "$TEST_ENV_FILE" | xargs -I{} bash -c "export {}"
    env $(cat "$TEST_ENV_FILE") go test -v ./test
    
    # Create a basic config file for testing the collector
    CONFIG_DIR=$(mktemp -d)
    cat > "${CONFIG_DIR}/otel-config.yaml" << EOF
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
      http:
        endpoint: 0.0.0.0:4318

processors:
  batch:
    timeout: 1s

  qanprocessor:
    mysql:
      enabled: $([ "$SKIP_MYSQL" = false ] && echo "true" || echo "false")
      endpoint: localhost:13306
      username: monitor_user
      password: password
      collection_interval: 10

    postgresql:
      enabled: $([ "$SKIP_POSTGRES" = false ] && echo "true" || echo "false")
      endpoint: localhost:15432
      username: monitor_user
      password: password
      database: postgres
      collection_interval: 10

exporters:
  logging:
    verbosity: detailed
    sampling_initial: 1
    sampling_thereafter: 1

service:
  pipelines:
    logs/qan:
      processors: [qanprocessor, batch]
      exporters: [logging]
EOF
    
    # Run the collector with the test config
    echo "Running the collector with the test configuration..."
    COLLECTOR_BIN="$PROJECT_ROOT/build/bin/obsidian-otel-collector"
    
    # Start the collector in the background
    "$COLLECTOR_BIN" --config "${CONFIG_DIR}/otel-config.yaml" > collector_output.log 2>&1 &
    COLLECTOR_PID=$!
    
    echo "Collector is running with PID $COLLECTOR_PID. Logs are in collector_output.log"
    echo "Waiting for $DOCKER_TIMEOUT seconds to collect data..."
    
    # Wait for the specified timeout
    sleep "$DOCKER_TIMEOUT"
    
    # Stop the collector
    echo "Stopping the collector..."
    kill $COLLECTOR_PID
    
    # Display a summary of collected data
    echo "=== Collector Log Summary ==="
    grep -n "QAN data" collector_output.log | tail -n 10
    echo "==========================="
    
    # Clean up Docker containers
    echo "Cleaning up Docker containers..."
    docker-compose -f docker/test-qan-processor.yml down
    
    # Clean up temporary files
    rm -f "$TEST_ENV_FILE"
    rm -rf "${CONFIG_DIR}"
    
    echo "Test completed successfully!"
else
    # Testing with external databases
    echo "Testing with external databases..."
    echo "Please ensure your external databases are properly configured."
    
    # Run the component tests
    cd "$PROJECT_ROOT/otel-collector/extension/qanprocessor"
    go test -v ./test
    
    echo "Test completed successfully!"
fi