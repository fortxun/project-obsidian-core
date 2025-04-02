-- PostgreSQL QAN Test Script
-- This script verifies if PostgreSQL is properly configured for QAN collection
-- and runs sample queries to generate data in pg_stat_statements

-- Check if pg_stat_statements extension is installed
SELECT EXISTS(
    SELECT 1 
    FROM pg_extension 
    WHERE extname = 'pg_stat_statements'
) AS pg_stat_statements_installed;

-- Check if pg_stat_statements is in shared_preload_libraries
SELECT current_setting('shared_preload_libraries') AS shared_preload_libraries;

-- Check pg_stat_statements configuration
SELECT name, setting 
FROM pg_settings 
WHERE name LIKE 'pg_stat_statements%';

-- Check if we can query pg_stat_statements
SELECT EXISTS(
    SELECT 1 
    FROM pg_stat_statements 
    LIMIT 1
) AS can_query_pg_stat_statements;

-- Execute some sample queries to generate metrics
CREATE TABLE IF NOT EXISTS qan_test (
    id SERIAL PRIMARY KEY,
    name TEXT,
    value INTEGER
);

-- Insert some test data
INSERT INTO qan_test (name, value) 
SELECT 'test_' || i, i 
FROM generate_series(1, 100) i
ON CONFLICT DO NOTHING;

-- Run different query patterns
SELECT * FROM qan_test WHERE id < 10;
SELECT * FROM qan_test WHERE name LIKE 'test_%';
SELECT AVG(value), SUM(value) FROM qan_test;
SELECT name, COUNT(*) FROM qan_test GROUP BY name HAVING COUNT(*) > 0;

-- Check pg_stat_statements for our test queries
SELECT 
    substring(query, 1, 50) AS query_sample,
    calls,
    total_exec_time,
    rows,
    shared_blks_read,
    shared_blks_hit
FROM pg_stat_statements
WHERE query LIKE '%qan_test%'
ORDER BY total_exec_time DESC
LIMIT 10;