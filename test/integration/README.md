# Project Obsidian Core - Integration Testing

This directory contains tools for testing Project Obsidian Core's QAN processors. There are both direct tests that can run against your local databases and full end-to-end tests for the complete stack.

## Quick Start: Direct QAN Processor Testing

If you want to quickly test just the QAN processors against your local databases, use these tools:

### 1. Quick Shell Test (`run_local_test.sh`)

This script directly tests the MySQL and PostgreSQL QAN processors against your local database instances:

```bash
# Run with default settings
./run_local_test.sh

# Or with custom settings
MYSQL_HOST=localhost MYSQL_PORT=3306 MYSQL_USER=root MYSQL_PASSWORD=your_password \
PG_HOST=localhost PG_PORT=5432 PG_USER=postgres PG_PASSWORD=your_password \
PSQL_BIN=/path/to/postgres/bin ./run_local_test.sh
```

### 2. Comprehensive Python QAN Test (`test_qan_processors.py`)

This script provides a more detailed QAN processor test with colored output and detailed reporting:

```bash
# Install dependencies first
pip install -r requirements.txt

# Run with default settings for your local instances
./test_qan_processors.py

# Run with custom parameters
./test_qan_processors.py --mysql-host localhost --mysql-port 3306 --mysql-user root --mysql-password your_password \
                         --pg-host localhost --pg-port 5432 --pg-user postgres --pg-password your_password
```

Options for `test_qan_processors.py`:
```
--mysql-host MYSQL_HOST     MySQL host (default: localhost)
--mysql-port MYSQL_PORT     MySQL port (default: 3306)
--mysql-user MYSQL_USER     MySQL username (default: root)
--mysql-password MYSQL_PASSWORD
                            MySQL password (default: culo1234)
--pg-host PG_HOST           PostgreSQL host (default: localhost)
--pg-port PG_PORT           PostgreSQL port (default: 5432)
--pg-user PG_USER           PostgreSQL username (default: postgres)
--pg-password PG_PASSWORD   PostgreSQL password (default: postgres)
--pg-database PG_DATABASE   PostgreSQL database (default: postgres)
--psql-bin PSQL_BIN         PostgreSQL binary directory (default: /Applications/Postgres.app/Contents/Versions/17/bin)
```

## Full End-to-End Testing (Docker-based)

For complete stack testing (requires Docker and Docker Compose):

### 1. Shell Script (`integration_test.sh`)

This script provides a full end-to-end test that:
- Starts the complete stack using docker-compose
- Verifies connections to all components
- Generates test data in MySQL and PostgreSQL
- Verifies data collection, ingestion into Druid, and JupyterLab analysis

```bash
# Run the full end-to-end test
./integration_test.sh
```

### 2. Python End-to-End Test (`e2e_test.py`)

A comprehensive Python-based end-to-end test:

```bash
# Install dependencies
pip install -r requirements.txt

# Run with default settings
./e2e_test.py

# With custom parameters
./e2e_test.py --mysql-host localhost --mysql-port 3307 --mysql-user root --mysql-password password
```

### 3. JupyterLab Notebook (`integration_test.ipynb`)

A Jupyter notebook in the `/notebooks` directory that visualizes the data flow:

- Located at `/notebooks/integration_test.ipynb`
- Verifies Druid connectivity and data ingestion
- Creates visualizations of query performance data
- Run it in JupyterLab at http://localhost:8888 after starting the stack

## Common Issues

### 1. Database Configuration

Make sure your databases are properly configured:

- **MySQL**: Performance Schema must be enabled with statements_digest consumer
- **PostgreSQL**: pg_stat_statements extension must be installed and enabled

### 2. Timing Issues

The full data flow takes time:

- Collection by OpenTelemetry happens at regular intervals
- Druid ingestion may take up to a minute to process new data
- Use the `--skip-wait` flag for Python tests only if you're sure data has been processed

### 3. Network Connectivity

In Docker environments, ensure services can communicate:

- The Docker Compose file maps ports to localhost by default
- Services reference each other by container name inside the network
- The Python script assumes database port mappings (MySQL:3307, PostgreSQL:5433)

### 4. Authentication

Test scripts default to:

- MySQL: `root:password`
- PostgreSQL: `monitor_user:password`

Update the credentials in the scripts or use command-line arguments if your setup differs.