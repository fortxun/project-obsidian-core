# Project Obsidian Core: TODO List

This document tracks features and improvements needed to meet all requirements for a production-ready MySQL query analytics system.

## Missing Requirements to Implement

### High Priority

1. **Query Samples Control**
   - [ ] Add configuration option to disable query sample collection by default
   - [ ] Implement opt-in mechanism for query samples
   - [ ] Add sampling rate configuration (collect only N% of queries)
   - [ ] Add maximum sample length configuration to prevent large query storage

2. **Time Zone Support**
   - [ ] Add explicit UTC and local time zone configuration options
   - [ ] Update timestamp handling in collectors to respect time zone settings
   - [ ] Add time zone conversion utilities in analysis notebooks/dashboards
   - [ ] Document time zone configuration and best practices

3. **Data Retention Policies**
   - [ ] Implement explicit retention configuration (min: 2 weeks of full resolution data)
   - [ ] Add automatic data roll-up for older data to save storage
   - [ ] Document retention policy configuration in Druid
   - [ ] Add monitoring for data storage usage

### Medium Priority

4. **Database/Cluster Grouping Enhancements**
   - [ ] Enhance instance tracking with explicit cluster/replica grouping
   - [ ] Add server metadata collection (version, config details)
   - [ ] Create visualizations for comparing queries across database instances
   - [ ] Implement filtering by account, schema, or other metadata

5. **EXPLAIN Plan Generation**
   - [ ] Add automatic EXPLAIN plan collection for slow queries
   - [ ] Implement storage for EXPLAIN plans in Druid
   - [ ] Create visualization for query execution plans
   - [ ] Add EXPLAIN plan analysis and recommendations

6. **Side-by-Side Query Comparisons**
   - [ ] Implement time-based comparison views (today vs. yesterday, etc.)
   - [ ] Add query variant comparison (similar queries across different apps)
   - [ ] Create visualization for side-by-side metric comparison
   - [ ] Add statistical significance indicators for performance changes

### Low Priority

7. **Metadata Parsing**
   - [ ] Implement parsing for query comments (e.g., /* application:name */ style)
   - [ ] Add support for custom annotations in queries
   - [ ] Create filters and views based on query metadata
   - [ ] Document metadata conventions and best practices

8. **Advanced Analysis Features**
   - [ ] Implement workload difference analysis between time periods
   - [ ] Add profiling by custom metadata
   - [ ] Create intelligent query sampling algorithms
   - [ ] Add machine learning for anomaly detection in query patterns

9. **Cluster-Level Query Metrics**
   - [ ] Implement aggregation of metrics across all nodes in a cluster
   - [ ] Add replication lag tracking between primary and replicas
   - [ ] Create dashboards for cluster-wide query performance
   - [ ] Implement query routing recommendations

## Technical Improvements

1. **Performance Optimizations**
   - [ ] Benchmark and optimize collector overhead
   - [ ] Implement efficient storage compression in Druid
   - [ ] Optimize analysis queries for better dashboard performance

2. **Security Enhancements**
   - [ ] Implement sensitive data redaction in query samples
   - [ ] Add user authentication for dashboards
   - [ ] Implement role-based access control for query data
   - [ ] Add audit logging for system access

3. **User Interface**
   - [ ] Develop web dashboard alternative to Jupyter notebooks
   - [ ] Create saved views and reports functionality
   - [ ] Implement alert configuration for slow queries
   - [ ] Add email/notification delivery for reports

4. **Documentation**
   - [ ] Create comprehensive user guide
   - [ ] Add installation and setup tutorials for various environments
   - [ ] Document all configuration parameters
   - [ ] Create troubleshooting guide

## Integration and Testing

1. **Integration Tests**
   - [ ] Add comprehensive end-to-end tests for all data flows
   - [ ] Implement performance benchmarks
   - [ ] Create regression test suite for core functionality

2. **Managed Service Integration**
   - [ ] Add specific support for Amazon RDS
   - [ ] Add specific support for Google Cloud SQL
   - [ ] Add specific support for Azure Database
   - [ ] Document service-specific configurations