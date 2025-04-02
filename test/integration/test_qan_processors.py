#!/usr/bin/env python3
"""
Direct test of QAN processors for Project Obsidian Core
This script directly tests the MySQL and PostgreSQL QAN processors
without requiring the full Docker stack
"""

import argparse
import os
import subprocess
import sys
import time
from datetime import datetime

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


class QANProcessorTester:
    """Direct tester for QAN processors"""

    def __init__(self, args):
        """Initialize the test with configuration"""
        self.mysql_host = args.mysql_host
        self.mysql_port = args.mysql_port
        self.mysql_user = args.mysql_user
        self.mysql_password = args.mysql_password
        self.mysql_database = "test_qan"

        self.pg_host = args.pg_host
        self.pg_port = args.pg_port
        self.pg_user = args.pg_user
        self.pg_password = args.pg_password
        self.pg_database = args.pg_database
        self.psql_bin = args.psql_bin
        
        # Test results
        self.test_results = {
            "mysql_connection": False,
            "postgresql_connection": False,
            "mysql_perf_schema": False,
            "postgresql_stats": False,
            "mysql_test_data": False,
            "postgresql_test_data": False
        }

    def run_test(self):
        """Execute the full test"""
        log("INFO", "Starting QAN Processor Test")

        # Test database connections and setup test data
        self._test_mysql_connection()
        self._test_postgresql_connection()
        
        # Generate test data if connections are good
        if self.test_results["mysql_connection"] and self.test_results["mysql_perf_schema"]:
            self._generate_mysql_test_data()
        
        if self.test_results["postgresql_connection"] and self.test_results["postgresql_stats"]:
            self._generate_postgresql_test_data()
            
        # Run the shell script tests
        if self.test_results["mysql_test_data"]:
            self._run_mysql_script()
            
        if self.test_results["postgresql_test_data"]:
            self._run_postgresql_script()
        
        # Print summary
        self._print_summary()
        
        return all(self.test_results.values())

    def _test_mysql_connection(self):
        """Test MySQL connection and Performance Schema"""
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
            self.test_results["mysql_connection"] = True
            
            # Check if performance_schema is enabled
            cursor.execute("SHOW VARIABLES LIKE 'performance_schema'")
            perf_schema = cursor.fetchone()
            if perf_schema and perf_schema[1] == "ON":
                log("SUCCESS", "Performance Schema is enabled")
                self.test_results["mysql_perf_schema"] = True
            else:
                log("ERROR", "Performance Schema is not enabled!")
                self.test_results["mysql_perf_schema"] = False
                
            # Check if statements_digest is enabled
            cursor.execute(
                "SELECT enabled FROM performance_schema.setup_consumers WHERE name = 'statements_digest'"
            )
            digest_enabled = cursor.fetchone()
            if digest_enabled and digest_enabled[0] == "YES":
                log("SUCCESS", "Statements digest consumer is enabled")
            else:
                log("ERROR", "Statements digest consumer is not enabled!")
                self.test_results["mysql_perf_schema"] = False
            
            cursor.close()
            conn.close()
        except Exception as e:
            log("ERROR", f"Failed to connect to MySQL: {e}")
            self.test_results["mysql_connection"] = False
            self.test_results["mysql_perf_schema"] = False

    def _test_postgresql_connection(self):
        """Test PostgreSQL connection and pg_stat_statements"""
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
            self.test_results["postgresql_connection"] = True
            
            # Check if pg_stat_statements is installed
            cursor.execute(
                "SELECT EXISTS(SELECT 1 FROM pg_extension WHERE extname = 'pg_stat_statements')"
            )
            has_extension = cursor.fetchone()[0]
            if has_extension:
                log("SUCCESS", "pg_stat_statements extension is installed")
                self.test_results["postgresql_stats"] = True
            else:
                log("ERROR", "pg_stat_statements extension is not installed!")
                log("INFO", "You can install it with: CREATE EXTENSION pg_stat_statements;")
                self.test_results["postgresql_stats"] = False
                
            # Check if we can query pg_stat_statements
            if has_extension:
                cursor.execute("SELECT EXISTS(SELECT 1 FROM pg_stat_statements LIMIT 1)")
                can_query = cursor.fetchone()[0]
                if can_query:
                    log("SUCCESS", "Successfully queried pg_stat_statements")
                else:
                    log("ERROR", "Cannot query pg_stat_statements!")
                    self.test_results["postgresql_stats"] = False
            
            cursor.close()
            conn.close()
        except Exception as e:
            log("ERROR", f"Failed to connect to PostgreSQL: {e}")
            self.test_results["postgresql_connection"] = False
            self.test_results["postgresql_stats"] = False

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
                ON DUPLICATE KEY UPDATE order_date=NOW()
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

    def _run_mysql_script(self):
        """Run the MySQL test script"""
        log("INFO", "Running MySQL QAN processor test script")
        
        # Set environment variables for the script
        env = os.environ.copy()
        env["MYSQL_HOST"] = self.mysql_host
        env["MYSQL_PORT"] = str(self.mysql_port)
        env["MYSQL_USER"] = self.mysql_user
        env["MYSQL_PASSWORD"] = self.mysql_password
        
        script_path = os.path.join(
            os.path.dirname(os.path.abspath(__file__)),
            "../../otel-collector/extension/qanprocessor/test/scripts/run_mysql_test.sh"
        )
        
        try:
            result = subprocess.run(
                [script_path],
                env=env,
                check=True,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                text=True
            )
            log("SUCCESS", "MySQL QAN processor test script completed successfully")
            log("INFO", "Output excerpt:")
            # Print the last few lines of output
            output_lines = result.stdout.strip().split("\n")
            for line in output_lines[-10:]:
                print(f"  {line}")
        except subprocess.CalledProcessError as e:
            log("ERROR", f"MySQL QAN processor test script failed: {e}")
            log("ERROR", f"Error output: {e.stderr}")

    def _run_postgresql_script(self):
        """Run the PostgreSQL test script"""
        log("INFO", "Running PostgreSQL QAN processor test script")
        
        # Set environment variables for the script
        env = os.environ.copy()
        env["PG_HOST"] = self.pg_host
        env["PG_PORT"] = str(self.pg_port)
        env["PG_USER"] = self.pg_user
        env["PG_PASS"] = self.pg_password
        env["PG_DB"] = self.pg_database
        env["PSQL_BIN"] = self.psql_bin
        
        script_path = os.path.join(
            os.path.dirname(os.path.abspath(__file__)),
            "../../otel-collector/extension/qanprocessor/test/scripts/run_postgres_test.sh"
        )
        
        try:
            result = subprocess.run(
                [script_path],
                env=env,
                check=True,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                text=True
            )
            log("SUCCESS", "PostgreSQL QAN processor test script completed successfully")
            log("INFO", "Output excerpt:")
            # Print the last few lines of output
            output_lines = result.stdout.strip().split("\n")
            for line in output_lines[-10:]:
                print(f"  {line}")
        except subprocess.CalledProcessError as e:
            log("ERROR", f"PostgreSQL QAN processor test script failed: {e}")
            log("ERROR", f"Error output: {e.stderr}")

    def _print_summary(self):
        """Print a summary of the test results"""
        log("INFO", "=== QAN PROCESSOR TEST SUMMARY ===")
        
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
            print(f"\n{Colors.GREEN}Congratulations! The QAN processor test was successful.{Colors.ENDC}")
            print("Both MySQL and PostgreSQL QAN processors are working correctly and can collect query data.")
        else:
            print(f"\n{Colors.RED}The QAN processor test failed. Please check the individual test results above.{Colors.ENDC}")
            
            # Provide specific advice based on failed tests
            if not self.test_results["mysql_connection"]:
                print("- MySQL Connection: Ensure MySQL is running and credentials are correct")
            if not self.test_results["postgresql_connection"]:
                print("- PostgreSQL Connection: Ensure PostgreSQL is running and credentials are correct")
            if not self.test_results["mysql_perf_schema"]:
                print("- MySQL Performance Schema: Enable performance_schema and statements_digest in MySQL")
            if not self.test_results["postgresql_stats"]:
                print("- PostgreSQL Stats: Enable pg_stat_statements extension in PostgreSQL")


def parse_args():
    """Parse command line arguments"""
    parser = argparse.ArgumentParser(description="Project Obsidian Core QAN processor test")
    
    # MySQL options
    parser.add_argument("--mysql-host", default="localhost", help="MySQL host (default: localhost)")
    parser.add_argument("--mysql-port", type=int, default=3306, help="MySQL port (default: 3306)")
    parser.add_argument("--mysql-user", default="root", help="MySQL username (default: root)")
    parser.add_argument("--mysql-password", default="culo1234", help="MySQL password (default: culo1234)")
    
    # PostgreSQL options
    parser.add_argument("--pg-host", default="localhost", help="PostgreSQL host (default: localhost)")
    parser.add_argument("--pg-port", type=int, default=5432, help="PostgreSQL port (default: 5432)")
    parser.add_argument("--pg-user", default="postgres", help="PostgreSQL username (default: postgres)")
    parser.add_argument("--pg-password", default="postgres", help="PostgreSQL password (default: postgres)")
    parser.add_argument("--pg-database", default="postgres", help="PostgreSQL database (default: postgres)")
    parser.add_argument("--psql-bin", default="/Applications/Postgres.app/Contents/Versions/17/bin", 
                       help="PostgreSQL binary directory (default: /Applications/Postgres.app/Contents/Versions/17/bin)")
    
    return parser.parse_args()


def main():
    """Main entry point"""
    args = parse_args()
    tester = QANProcessorTester(args)
    success = tester.run_test()
    sys.exit(0 if success else 1)


if __name__ == "__main__":
    main()