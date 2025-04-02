# Database Configuration Requirements

This document outlines the necessary configuration and permissions for MySQL and PostgreSQL databases to be monitored by Project Obsidian Core.

## MySQL Configuration

### Required Permissions

The monitoring user requires the following privileges:

```sql
GRANT SELECT, PROCESS, SHOW VIEW, REPLICATION CLIENT ON *.* TO 'monitor_user'@'%';
```

These permissions enable:
- `SELECT`: Access to query database objects
- `PROCESS`: Access to view server process information
- `SHOW VIEW`: Ability to view metadata about database views
- `REPLICATION CLIENT`: Access to replication status information

### Performance Schema Configuration

MySQL's Performance Schema must be enabled. Add these settings to your `my.cnf` file:

```ini
[mysqld]
performance_schema=ON
performance_schema_consumer_events_statements_current=ON
performance_schema_consumer_events_statements_history=ON
performance_schema_consumer_events_statements_history_long=ON
```

For Amazon RDS MySQL, you'll need to modify the parameter group:

```ini
performance_schema = 1
performance_schema_consumer_events_statements_history_long = 1
```

### Verification

To verify your Performance Schema is correctly configured:

```sql
SHOW VARIABLES LIKE 'performance_schema';
-- Should return 'ON'

SELECT * FROM performance_schema.setup_consumers 
WHERE name LIKE 'events_statements%';
-- Should show all relevant consumers are enabled
```

## PostgreSQL Configuration

### Required Permissions

#### Option 1: Superuser Access (Simplest)

```sql
CREATE ROLE monitor_user SUPERUSER LOGIN PASSWORD 'password';
```

#### Option 2: Limited Privileges via Security Definer (for managed services)

```sql
-- Create function with security definer
CREATE FUNCTION get_querystats() RETURNS SETOF pg_stat_statements 
LANGUAGE sql SECURITY DEFINER AS $$ SELECT * FROM pg_stat_statements $$;

-- Create view accessible to monitor user
CREATE VIEW pg_stat_statements_allusers AS SELECT * FROM get_querystats();
GRANT SELECT ON pg_stat_statements_allusers TO monitor_user;
```

### pg_stat_statements Configuration

1. Modify `postgresql.conf`:

```ini
shared_preload_libraries = 'pg_stat_statements'
pg_stat_statements.max = 10000
pg_stat_statements.track = all
```

2. Create extension in each database you want to monitor:

```sql
CREATE EXTENSION pg_stat_statements;
```

3. For Docker deployments:

```bash
docker run -e POSTGRES_PASSWORD=secret \
  -e shared_preload_libraries=pg_stat_statements \
  postgres:15
```

### Verification

To verify pg_stat_statements is correctly configured:

```sql
SELECT * FROM pg_stat_statements LIMIT 1;
-- Should return at least one row if statements have been executed
```

## Managed Service Considerations

| Service          | MySQL Requirements              | PostgreSQL Requirements         |
|------------------|---------------------------------|----------------------------------|
| **Amazon RDS**   | Enable Performance Schema in PG | Grant `rds_superuser` role      |
| **Google Cloud** | Default PFS enabled             | Use cloudsqlsuperuser role      |
| **Azure DB**     | Enable query_store=ON           | Limited to azure_pg_admin role  |

## Troubleshooting

If you encounter issues with data collection:

1. Verify permissions:
   ```sql
   -- MySQL
   SHOW GRANTS FOR 'monitor_user'@'%';
   
   -- PostgreSQL
   SELECT rolname, rolsuper FROM pg_roles WHERE rolname = 'monitor_user';
   ```

2. Check extension status:
   ```sql
   -- PostgreSQL
   SELECT * FROM pg_extension WHERE extname = 'pg_stat_statements';
   ```

3. Review collector logs for any permission errors.