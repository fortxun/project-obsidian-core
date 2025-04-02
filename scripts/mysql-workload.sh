#!/bin/bash
# Script to generate test workload on MySQL

# Wait a moment for the database to be fully ready
sleep 5

echo "Starting MySQL workload generation..."

# Run in a loop to continuously generate queries
while true; do
  # Simple queries
  mysql -h mysql -u monitor_user -ppassword test -e "SELECT * FROM users WHERE id = 1;"
  mysql -h mysql -u monitor_user -ppassword test -e "SELECT * FROM users WHERE name LIKE 'John%';"
  mysql -h mysql -u monitor_user -ppassword test -e "SELECT COUNT(*) FROM users;"
  
  # Queries with joins
  mysql -h mysql -u monitor_user -ppassword test -e "
    SELECT u.name, COUNT(o.id) AS order_count
    FROM users u
    LEFT JOIN orders o ON u.id = o.user_id
    GROUP BY u.name;
  "
  
  # Inserts and updates
  random_id=$((1 + RANDOM % 5))
  random_timestamp=$(date +"%Y-%m-%d %H:%M:%S")
  
  mysql -h mysql -u monitor_user -ppassword test -e "
    INSERT INTO orders (user_id, total, status, created_at)
    VALUES ($random_id, RAND() * 100, 'pending', '$random_timestamp');
  "
  
  last_order_id=$(mysql -h mysql -u monitor_user -ppassword test -se "SELECT MAX(id) FROM orders;")
  if [ ! -z "$last_order_id" ]; then
    mysql -h mysql -u monitor_user -ppassword test -e "
      INSERT INTO order_items (order_id, product_name, quantity, price)
      VALUES ($last_order_id, 'Test Product', 1, RAND() * 50);
    "
    
    mysql -h mysql -u monitor_user -ppassword test -e "
      UPDATE orders SET status = 'completed' WHERE id = $last_order_id;
    "
  fi
  
  # Add some queries that will cause different execution plans
  mysql -h mysql -u monitor_user -ppassword test -e "
    SELECT * FROM users ORDER BY created_at DESC LIMIT 3;
  "
  
  mysql -h mysql -u monitor_user -ppassword test -e "
    SELECT * FROM orders WHERE total > 50;
  "
  
  # Add some queries with temp tables
  mysql -h mysql -u monitor_user -ppassword test -e "
    SELECT u.id, u.name, 
    (SELECT COUNT(*) FROM orders WHERE user_id = u.id) as order_count
    FROM users u
    HAVING order_count > 0
    ORDER BY order_count DESC;
  "
  
  # Wait a bit between each batch
  sleep 2
done