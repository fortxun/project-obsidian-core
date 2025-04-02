#!/bin/bash
# Script to generate test workload on PostgreSQL

# Wait a moment for the database to be fully ready
sleep 5

echo "Starting PostgreSQL workload generation..."

# Run in a loop to continuously generate queries
while true; do
  # Simple queries
  PGPASSWORD=password psql -h postgres -U monitor_user -c "SELECT * FROM users WHERE id = 1;"
  PGPASSWORD=password psql -h postgres -U monitor_user -c "SELECT * FROM users WHERE name LIKE 'John%';"
  PGPASSWORD=password psql -h postgres -U monitor_user -c "SELECT COUNT(*) FROM users;"
  
  # Queries with joins
  PGPASSWORD=password psql -h postgres -U monitor_user -c "
    SELECT u.name, COUNT(o.id) AS order_count
    FROM users u
    LEFT JOIN orders o ON u.id = o.user_id
    GROUP BY u.name;
  "
  
  # Inserts and updates
  random_id=$((1 + RANDOM % 5))
  random_timestamp=$(date +"%Y-%m-%d %H:%M:%S")
  
  PGPASSWORD=password psql -h postgres -U monitor_user -c "
    INSERT INTO orders (user_id, total, status, created_at)
    VALUES ($random_id, RANDOM() * 100, 'pending', '$random_timestamp');
  "
  
  last_order_id=$(PGPASSWORD=password psql -h postgres -U monitor_user -t -c "SELECT MAX(id) FROM orders;")
  last_order_id=$(echo $last_order_id | tr -d '[:space:]')
  
  if [ ! -z "$last_order_id" ]; then
    PGPASSWORD=password psql -h postgres -U monitor_user -c "
      INSERT INTO order_items (order_id, product_name, quantity, price)
      VALUES ($last_order_id, 'Test Product', 1, RANDOM() * 50);
    "
    
    PGPASSWORD=password psql -h postgres -U monitor_user -c "
      UPDATE orders SET status = 'completed' WHERE id = $last_order_id;
    "
  fi
  
  # Add some queries that will cause different execution plans
  PGPASSWORD=password psql -h postgres -U monitor_user -c "
    SELECT * FROM users ORDER BY created_at DESC LIMIT 3;
  "
  
  PGPASSWORD=password psql -h postgres -U monitor_user -c "
    SELECT * FROM orders WHERE total > 50;
  "
  
  # Add some queries with temp tables and complex operations
  PGPASSWORD=password psql -h postgres -U monitor_user -c "
    WITH order_summary AS (
      SELECT user_id, COUNT(*) as order_count, SUM(total) as total_spent
      FROM orders
      GROUP BY user_id
    )
    SELECT u.name, COALESCE(os.order_count, 0) as orders, COALESCE(os.total_spent, 0) as total
    FROM users u
    LEFT JOIN order_summary os ON u.id = os.user_id
    ORDER BY total DESC;
  "
  
  # Wait a bit between each batch
  sleep 2
done