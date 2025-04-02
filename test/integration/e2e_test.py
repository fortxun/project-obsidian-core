#!/usr/bin/env python3
"""
End-to-end integration test for Project Obsidian Core
This script verifies the complete data flow from databases to analysis
"""

import argparse
import json
import os
import subprocess
import sys
import time
from datetime import datetime, timedelta

import pandas as pd
import requests
import mysql.connector
import psycopg2
from psycopg2.extensions import ISOLATION_LEVEL_AUTOCOMMIT
from tabulate import tabulate


class Colors:
    """ANSI color codes for colorful terminal output"""
    BLUE = '\033[94m'
    GREEN = '\033[92m'
    YELLOW = '\033[93m'
    RED = '\033[91m'
    ENDC = '\033[0m'
    BOLD = '\033[1m'


def log(level, message):
    """Log messages with color coding by level"""
    color = Colors.ENDC
    if level == "INFO":
        color = Colors.BLUE
    elif level == "SUCCESS":
        color = Colors.GREEN
    elif level == "WARNING":
        color = Colors.YELLOW
    elif level == "ERROR":
        color = Colors.RED

    print(f"{color}[{level}] {message}{Colors.ENDC}")


class ObsidianE2ETest:
    """End-to-end tester for Project Obsidian Core"""

    def __init__(self, args):
        """Initialize the test with configuration"""
        self.mysql_host = args.mysql_host
        self.mysql_port = args.mysql_port
        self.mysql_user = args.mysql_user
        self.mysql_password = args.mysql_password
        self.mysql_database = "test_e2e"

        self.pg_host = args.pg_host
        self.pg_port = args.pg_port
        self.pg_user = args.pg_user
        self.pg_password = args.pg_password
        self.pg_database = args.pg_database

        self.druid_host = args.druid_host
        self.druid_port = args.druid_port
        self.druid_url = f"http://{self.druid_host}:{self.druid_port}"

        self.use_existing_stack = args.use_existing
        self.docker_compose_file = args.docker_compose_file
        self.skip_wait = args.skip_wait
        
        # Test results
        self.test_results = {
            "mysql_connection": False,
            "postgresql_connection": False,
            "druid_connection": False,
            "mysql_test_data": False,
            "postgresql_test_data": False,
            "otel_collection": False,
            "druid_ingestion": False,
            "jupyter_connection": False
        }

    def run_test(self):
        """Execute the full end-to-end test"""
        log("INFO", "Starting Project Obsidian Core end-to-end test")

        if not self.use_existing_stack:
            self._start_stack()
        else:
            log("INFO", "Using existing stack (skipping startup)")

        # Test database connections and setup test data
        self._test_mysql_connection()
        self._test_postgresql_connection()
        
        # Generate test data if connections are good
        if self.test_results["mysql_connection"]:
            self._generate_mysql_test_data()
        
        if self.test_results["postgresql_connection"]:
            self._generate_postgresql_test_data()

        # Test Druid connection
        self._test_druid_connection()
        
        # Wait for data to be processed and ingested
        if not self.skip_wait:
            log("INFO", "Waiting for data to be processed by OTel and ingested into Druid (60s)")
            time.sleep(60)
        else:
            log("INFO", "Skipping wait period (--skip-wait flag used)")
        
        # Check data in Druid
        self._check_druid_ingestion()
        
        # Check Jupyter connection
        self._test_jupyter_connection()
        
        # Print summary
        self._print_summary()
        
        return all(self.test_results.values())

    def _start_stack(self):
        """Start the Obsidian Core stack using docker-compose"""
        log("INFO", f"Starting Obsidian Core stack using {self.docker_compose_file}")
        
        try:
            subprocess.run(
                ["docker-compose", "-f", self.docker_compose_file, "up", "-d"],
                check=True,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                universal_newlines=True
            )
            log("SUCCESS", "Stack started successfully")
            
            # Wait for services to be ready
            log("INFO", "Waiting for services to start (30s initial wait)")
            time.sleep(30)
        except subprocess.CalledProcessError as e:
            log("ERROR", f"Failed to start stack: {e.stderr}")
            sys.exit(1)

    def _test_mysql_connection(self):
        """Test MySQL connection"""
        log("INFO", f"Testing MySQL connection to {self.mysql_host}:{self.mysql_port}")
        
        try:
            conn = mysql.connector.connect(
                host=self.mysql_host,
                port=self.mysql_port,
                user=self.mysql_user,
                password=self.mysql_password
            )
            
            cursor = conn.cursor()
            cursor.execute("SELECT VERSION()")
            version = cursor.fetchone()[0]
            log("SUCCESS", f"Connected to MySQL version: {version}")
            
            # Check if performance_schema is enabled
            cursor.execute("SHOW VARIABLES LIKE 'performance_schema'")
            perf_schema = cursor.fetchone()
            if perf_schema and perf_schema[1] == "ON":
                log("SUCCESS", "Performance Schema is enabled")
            else:
                log("ERROR", "Performance Schema is not enabled!")
                
            # Check if statements_digest is enabled
            cursor.execute(
                "SELECT enabled FROM performance_schema.setup_consumers WHERE name = 'statements_digest'"
            )
            digest_enabled = cursor.fetchone()
            if digest_enabled and digest_enabled[0] == "YES":
                log("SUCCESS", "Statements digest consumer is enabled")
            else:
                log("ERROR", "Statements digest consumer is not enabled!")
            
            cursor.close()
            conn.close()
            self.test_results["mysql_connection"] = True
        except Exception as e:
            log("ERROR", f"Failed to connect to MySQL: {e}")
            self.test_results["mysql_connection"] = False

    def _test_postgresql_connection(self):
        """Test PostgreSQL connection"""
        log("INFO", f"Testing PostgreSQL connection to {self.pg_host}:{self.pg_port}")
        
        try:
            conn = psycopg2.connect(
                host=self.pg_host,
                port=self.pg_port,
                user=self.pg_user,
                password=self.pg_password,
                database=self.pg_database
            )
            conn.set_isolation_level(ISOLATION_LEVEL_AUTOCOMMIT)
            
            cursor = conn.cursor()
            cursor.execute("SELECT version()")
            version = cursor.fetchone()[0]
            log("SUCCESS", f"Connected to PostgreSQL version: {version}")
            
            # Check if pg_stat_statements is installed
            cursor.execute(
                "SELECT EXISTS(SELECT 1 FROM pg_extension WHERE extname = 'pg_stat_statements')"
            )
            has_extension = cursor.fetchone()[0]
            if has_extension:
                log("SUCCESS", "pg_stat_statements extension is installed")
            else:
                log("ERROR", "pg_stat_statements extension is not installed!")
                
            # Check if we can query pg_stat_statements
            cursor.execute("SELECT EXISTS(SELECT 1 FROM pg_stat_statements LIMIT 1)")
            can_query = cursor.fetchone()[0]
            if can_query:
                log("SUCCESS", "Successfully queried pg_stat_statements")
            else:
                log("ERROR", "Cannot query pg_stat_statements!")
            
            cursor.close()
            conn.close()
            self.test_results["postgresql_connection"] = True
        except Exception as e:
            log("ERROR", f"Failed to connect to PostgreSQL: {e}")
            self.test_results["postgresql_connection"] = False

    def _generate_mysql_test_data(self):
        """Generate test data in MySQL"""
        log("INFO", "Generating test data in MySQL")
        
        try:
            conn = mysql.connector.connect(
                host=self.mysql_host,
                port=self.mysql_port,
                user=self.mysql_user,
                password=self.mysql_password
            )
            cursor = conn.cursor()
            
            # Create test database
            cursor.execute(f"CREATE DATABASE IF NOT EXISTS {self.mysql_database}")
            cursor.execute(f"USE {self.mysql_database}")
            
            # Create test table
            cursor.execute("""
                CREATE TABLE IF NOT EXISTS orders (
                  id INT AUTO_INCREMENT PRIMARY KEY,
                  customer_id INT NOT NULL,
                  order_date DATETIME NOT NULL,
                  amount DECIMAL(10,2) NOT NULL,
                  status VARCHAR(20) NOT NULL
                )
            """)
            
            # Insert test data
            cursor.execute("""
                INSERT INTO orders (customer_id, order_date, amount, status)
                VALUES 
                  (101, NOW(), 199.99, 'completed'),
                  (102, NOW(), 99.50, 'pending'),
                  (103, NOW(), 50.25, 'completed'),
                  (104, NOW(), 25.99, 'cancelled'),
                  (105, NOW(), 39.99, 'pending')
            """)
            
            # Run test queries
            test_queries = [
                "SELECT * FROM orders WHERE amount > 50",
                "SELECT status, COUNT(*) AS count, SUM(amount) AS total FROM orders GROUP BY status",
                "SELECT customer_id, COUNT(*) FROM orders GROUP BY customer_id HAVING COUNT(*) > 0",
                "SELECT AVG(amount) AS average_order FROM orders",
                "SELECT AVG(amount) AS average_order FROM orders",
                "SELECT AVG(amount) AS average_order FROM orders"
            ]
            
            for query in test_queries:
                cursor.execute(query)
                cursor.fetchall()  # Consume the result set
            
            conn.commit()
            cursor.close()
            conn.close()
            
            log("SUCCESS", "Generated test data in MySQL")
            self.test_results["mysql_test_data"] = True
        except Exception as e:
            log("ERROR", f"Failed to generate MySQL test data: {e}")
            self.test_results["mysql_test_data"] = False

    def _generate_postgresql_test_data(self):
        """Generate test data in PostgreSQL"""
        log("INFO", "Generating test data in PostgreSQL")
        
        try:
            conn = psycopg2.connect(
                host=self.pg_host,
                port=self.pg_port,
                user=self.pg_user,
                password=self.pg_password,
                database=self.pg_database
            )
            conn.set_isolation_level(ISOLATION_LEVEL_AUTOCOMMIT)
            cursor = conn.cursor()
            
            # Create test table
            cursor.execute("""
                CREATE TABLE IF NOT EXISTS products (
                  id SERIAL PRIMARY KEY,
                  name VARCHAR(100) NOT NULL,
                  category VARCHAR(50) NOT NULL,
                  price DECIMAL(10,2) NOT NULL,
                  inventory INT NOT NULL
                )
            """)
            
            # Insert test data
            cursor.execute("""
                INSERT INTO products (name, category, price, inventory)
                VALUES 
                  ('Laptop', 'Electronics', 999.99, 25),
                  ('Smartphone', 'Electronics', 699.50, 50),
                  ('Headphones', 'Accessories', 89.99, 100),
                  ('Monitor', 'Electronics', 249.99, 15),
                  ('Keyboard', 'Accessories', 59.99, 30)
                ON CONFLICT (id) DO NOTHING
            """)
            
            # Run test queries
            test_queries = [
                "SELECT * FROM products WHERE price > 100",
                "SELECT category, COUNT(*) AS count, SUM(price) AS total_price FROM products GROUP BY category",
                "SELECT * FROM products ORDER BY price DESC",
                "SELECT AVG(price) AS average_price FROM products",
                "SELECT AVG(price) AS average_price FROM products",
                "SELECT AVG(price) AS average_price FROM products"
            ]
            
            for query in test_queries:
                cursor.execute(query)
                cursor.fetchall()  # Consume the result set
            
            # Check for test queries in pg_stat_statements
            cursor.execute("""
                SELECT query, calls, total_exec_time 
                FROM pg_stat_statements 
                WHERE query LIKE '%products%' 
                ORDER BY total_exec_time DESC 
                LIMIT 5
            """)
            
            found_queries = cursor.fetchall()
            if found_queries:
                log("INFO", f"Found {len(found_queries)} product queries in pg_stat_statements")
            else:
                log("WARNING", "No product queries found in pg_stat_statements yet")
            
            cursor.close()
            conn.close()
            
            log("SUCCESS", "Generated test data in PostgreSQL")
            self.test_results["postgresql_test_data"] = True
        except Exception as e:
            log("ERROR", f"Failed to generate PostgreSQL test data: {e}")
            self.test_results["postgresql_test_data"] = False

    def _test_druid_connection(self):
        """Test connection to Druid"""
        log("INFO", f"Testing Druid connection to {self.druid_url}")
        
        try:
            response = requests.get(f"{self.druid_url}/status")
            if response.status_code == 200:
                log("SUCCESS", "Druid is available")
                self.test_results["druid_connection"] = True
            else:
                log("ERROR", f"Druid returned status code {response.status_code}")
                self.test_results["druid_connection"] = False
        except Exception as e:
            log("ERROR", f"Failed to connect to Druid: {e}")
            self.test_results["druid_connection"] = False

    def _check_druid_ingestion(self):
        """Check if data has been ingested into Druid"""
        log("INFO", "Checking data ingestion in Druid")
        
        if not self.test_results["druid_connection"]:
            log("ERROR", "Skipping Druid ingestion check because Druid connection failed")
            self.test_results["druid_ingestion"] = False
            return
        
        try:
            # Check for qan_db table
            tables_response = requests.post(
                f"{self.druid_url}/druid/v2/sql",
                headers={"Content-Type": "application/json"},
                json={"query": "SHOW TABLES", "context": {"sqlQueryId": "test-tables"}}
            )
            
            if tables_response.status_code != 200:
                log("ERROR", f"Failed to query Druid tables: {tables_response.text}")
                self.test_results["druid_ingestion"] = False
                return
            
            tables = tables_response.json()
            table_found = False
            for table in tables:
                if 'TABLE_NAME' in table and table['TABLE_NAME'] == 'qan_db':
                    table_found = True
                    break
            
            if not table_found:
                log("ERROR", "qan_db table not found in Druid")
                self.test_results["druid_ingestion"] = False
                return
            
            log("SUCCESS", "qan_db table found in Druid")
            
            # Define time range for queries
            end_time = datetime.now()
            start_time = end_time - timedelta(hours=1)
            start_time_str = start_time.strftime("%Y-%m-%d %H:%M:%S")
            end_time_str = end_time.strftime("%Y-%m-%d %H:%M:%S")
            
            # Check for MySQL data
            mysql_query = f"""
                SELECT COUNT(*) AS count
                FROM qan_db
                WHERE "__time" BETWEEN TIMESTAMP '{start_time_str}' AND TIMESTAMP '{end_time_str}'
                AND db.system = 'mysql'
            """
            
            mysql_response = requests.post(
                f"{self.druid_url}/druid/v2/sql",
                headers={"Content-Type": "application/json"},
                json={"query": mysql_query, "context": {"sqlQueryId": "test-mysql-count"}}
            )
            
            if mysql_response.status_code != 200:
                log("ERROR", f"Failed to query MySQL data count: {mysql_response.text}")
            else:
                mysql_count = mysql_response.json()[0]['count']
                log("INFO", f"Found {mysql_count} MySQL QAN records in Druid")
                
                if mysql_count > 0:
                    self.test_results["otel_collection"] = True
                
                # Check for test data specifically
                mysql_test_query = f"""
                    SELECT COUNT(*) AS count
                    FROM qan_db
                    WHERE "__time" BETWEEN TIMESTAMP '{start_time_str}' AND TIMESTAMP '{end_time_str}'
                    AND db.system = 'mysql'
                    AND db.statement.sample LIKE '%orders%'
                """
                
                mysql_test_response = requests.post(
                    f"{self.druid_url}/druid/v2/sql",
                    headers={"Content-Type": "application/json"},
                    json={"query": mysql_test_query, "context": {"sqlQueryId": "test-mysql-test-count"}}
                )
                
                if mysql_test_response.status_code == 200:
                    mysql_test_count = mysql_test_response.json()[0]['count']
                    log("INFO", f"Found {mysql_test_count} MySQL test query records in Druid")
            
            # Check for PostgreSQL data
            pg_query = f"""
                SELECT COUNT(*) AS count
                FROM qan_db
                WHERE "__time" BETWEEN TIMESTAMP '{start_time_str}' AND TIMESTAMP '{end_time_str}'
                AND db.system = 'postgresql'
            """
            
            pg_response = requests.post(
                f"{self.druid_url}/druid/v2/sql",
                headers={"Content-Type": "application/json"},
                json={"query": pg_query, "context": {"sqlQueryId": "test-pg-count"}}
            )
            
            if pg_response.status_code != 200:
                log("ERROR", f"Failed to query PostgreSQL data count: {pg_response.text}")
            else:
                pg_count = pg_response.json()[0]['count']
                log("INFO", f"Found {pg_count} PostgreSQL QAN records in Druid")
                
                if pg_count > 0:
                    self.test_results["otel_collection"] = True
                
                # Check for test data specifically
                pg_test_query = f"""
                    SELECT COUNT(*) AS count
                    FROM qan_db
                    WHERE "__time" BETWEEN TIMESTAMP '{start_time_str}' AND TIMESTAMP '{end_time_str}'
                    AND db.system = 'postgresql'
                    AND db.statement.sample LIKE '%products%'
                """
                
                pg_test_response = requests.post(
                    f"{self.druid_url}/druid/v2/sql",
                    headers={"Content-Type": "application/json"},
                    json={"query": pg_test_query, "context": {"sqlQueryId": "test-pg-test-count"}}
                )
                
                if pg_test_response.status_code == 200:
                    pg_test_count = pg_test_response.json()[0]['count']
                    log("INFO", f"Found {pg_test_count} PostgreSQL test query records in Druid")
            
            # Mark ingestion as successful if we found any data
            mysql_data_found = mysql_response.status_code == 200 and mysql_response.json()[0]['count'] > 0
            pg_data_found = pg_response.status_code == 200 and pg_response.json()[0]['count'] > 0
            
            if mysql_data_found or pg_data_found:
                log("SUCCESS", "Found QAN data in Druid")
                self.test_results["druid_ingestion"] = True
            else:
                log("ERROR", "No QAN data found in Druid")
                self.test_results["druid_ingestion"] = False
                
        except Exception as e:
            log("ERROR", f"Error checking Druid ingestion: {e}")
            self.test_results["druid_ingestion"] = False

    def _test_jupyter_connection(self):
        """Test connection to JupyterLab"""
        log("INFO", "Testing JupyterLab connection")
        
        try:
            response = requests.get("http://localhost:8888")
            if response.status_code == 200:
                log("SUCCESS", "JupyterLab is available")
                self.test_results["jupyter_connection"] = True
            else:
                log("ERROR", f"JupyterLab returned status code {response.status_code}")
                self.test_results["jupyter_connection"] = False
        except Exception as e:
            log("ERROR", f"Failed to connect to JupyterLab: {e}")
            self.test_results["jupyter_connection"] = False

    def _print_summary(self):
        """Print a summary of the test results"""
        log("INFO", "=== END-TO-END TEST SUMMARY ===")
        
        table_data = []
        for test, result in self.test_results.items():
            status = f"{Colors.GREEN}✓ PASSED{Colors.ENDC}" if result else f"{Colors.RED}✗ FAILED{Colors.ENDC}"
            table_data.append([test.replace("_", " ").title(), status])
        
        print(tabulate(table_data, headers=["Test", "Status"], tablefmt="grid"))
        
        # Overall status
        all_passed = all(self.test_results.values())
        overall_status = f"{Colors.GREEN}PASSED{Colors.ENDC}" if all_passed else f"{Colors.RED}FAILED{Colors.ENDC}"
        print(f"\nOverall Status: {overall_status}")
        
        if all_passed:
            print(f"\n{Colors.GREEN}Congratulations! The end-to-end test has verified the full data flow:{Colors.ENDC}")
            print("1. Test data was generated in MySQL and PostgreSQL")
            print("2. Data was collected by OpenTelemetry QAN processors")
            print("3. Data was successfully ingested into Druid")
            print("4. All components of the stack are operational")
        else:
            print(f"\n{Colors.RED}The end-to-end test failed. Please check the individual test results above.{Colors.ENDC}")
            print("Common issues and solutions:")
            print("- Database configuration: Ensure Performance Schema and pg_stat_statements are enabled")
            print("- Network connectivity: Check that services can communicate with each other")
            print("- Startup timing: The stack might need more time to fully initialize")


def parse_args():
    """Parse command line arguments"""
    parser = argparse.ArgumentParser(description="Project Obsidian Core end-to-end integration test")
    
    # MySQL options
    parser.add_argument("--mysql-host", default="localhost", help="MySQL host (default: localhost)")
    parser.add_argument("--mysql-port", type=int, default=3307, help="MySQL port (default: 3307)")
    parser.add_argument("--mysql-user", default="root", help="MySQL username (default: root)")
    parser.add_argument("--mysql-password", default="password", help="MySQL password (default: password)")
    
    # PostgreSQL options
    parser.add_argument("--pg-host", default="localhost", help="PostgreSQL host (default: localhost)")
    parser.add_argument("--pg-port", type=int, default=5433, help="PostgreSQL port (default: 5433)")
    parser.add_argument("--pg-user", default="monitor_user", help="PostgreSQL username (default: monitor_user)")
    parser.add_argument("--pg-password", default="password", help="PostgreSQL password (default: password)")
    parser.add_argument("--pg-database", default="postgres", help="PostgreSQL database (default: postgres)")
    
    # Druid options
    parser.add_argument("--druid-host", default="localhost", help="Druid host (default: localhost)")
    parser.add_argument("--druid-port", type=int, default=8888, help="Druid port (default: 8888)")
    
    # Test options
    parser.add_argument("--use-existing", action="store_true", help="Use existing stack (don't start with docker-compose)")
    parser.add_argument("--docker-compose-file", default="../../docker-compose.yml", help="Path to docker-compose.yml")
    parser.add_argument("--skip-wait", action="store_true", help="Skip waiting for data processing")
    
    return parser.parse_args()


def main():
    """Main entry point"""
    args = parse_args()
    tester = ObsidianE2ETest(args)
    success = tester.run_test()
    sys.exit(0 if success else 1)


if __name__ == "__main__":
    main()