# Baton SQL Testing

This directory contains tests for the baton-sql connector, including tests for specific features like employee ID and last login.

## Running the Tests

### Docker Compose Test Environment

We provide a Docker Compose setup to easily test the baton-sql connector with a MySQL database:

```bash
# Start the test environment
docker-compose -f docker-compose-test.yml up

# To run in the background
docker-compose -f docker-compose-test.yml up -d

# To stop the test environment
docker-compose -f docker-compose-test.yml down
```

### Validating Features

To validate that the employee ID and last login features are working correctly:

1. First, run the connector using the test configuration:
   ```bash
   # Either through Docker Compose:
   docker-compose -f docker-compose-test.yml up

   # Or directly:
   ./dist/darwin_arm64/baton-sql --config-path ./examples/mysql-test.yml --log-level debug
   ```

2. Then run the validation script:
   ```bash
   ./test/validate_features.sh
   ```

## Test Data

The MySQL database is initialized with test data that includes:

- Users with employee IDs and last login timestamps
- Different date formats for last login
- Roles and user-role relationships
- Additional tables for testing various aspects of the connector

## Manual Testing

You can also connect to the database manually to inspect or modify the test data:

```bash
# Connect to the MySQL database
docker exec -it baton-mysql-test mysql -u baton -ppassword batondb

# Example query to see users with their employee IDs and last login times
mysql> SELECT username, employee_id, last_login FROM users;
```