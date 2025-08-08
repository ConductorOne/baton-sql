# Baton SQL Testing

This directory contains comprehensive testing infrastructure for the baton-sql connector, including database initialization scripts and Docker configurations for random password generation across multiple database engines.

## Supported Databases

The testing environment supports all major database engines:

- **MySQL** - With SHA2-256 password hashing and account provisioning
- **PostgreSQL** - With bcrypt password hashing via pgcrypto extension
- **Oracle Database** - With DBA_USERS integration and custom table provisioning
- **SQL Server** - With SHA2-256 password hashing and HASHBYTES functions
- **WordPress** - MySQL-based WordPress user management with MD5 hashing

## Test Files

### Database Initialization Scripts

- `mysql-init.sql` - Complete MySQL schema with password_hash column, roles, and test data
- `postgres-init.sql` - PostgreSQL schema with bcrypt support and pgcrypto extension
- `oracle-init.sql` - Complete Oracle setup with DBA user creation and custom tables
- `sqlserver-init.sql` - SQL Server schema with HASHBYTES password hashing
- `wordpress-init.sql` - WordPress wp_users table with MD5 password support
- `create_baton_pdb.sql` - Oracle Pluggable Database (PDB) specific setup

### Docker Compose Configurations

- `../docker-compose-mysql-test.yml` - MySQL test environment with account provisioning
- `../docker-compose-postgres-test.yml` - PostgreSQL test environment with pgcrypto
- `../docker-compose-oracle-test.yml` - Oracle XE test environment with DBA setup
- `../docker-compose-sqlserver-test.yml` - SQL Server test environment with HASHBYTES
- `../docker-compose-wordpress-test.yml` - WordPress + MySQL test environment

## Quick Start Testing

### Individual Database Testing

#### MySQL Testing

```bash
# Start MySQL test environment
docker-compose -f docker-compose-mysql-test.yml up -d

# Wait for MySQL to be ready (5-10 seconds)
sleep 10

# Test account creation with random password
./baton-sql --config-path examples/mysql-local-test.yml --provisioning \
  --create-account-login "test_mysql_user" \
  --create-account-profile '{"username": "test_mysql_user", "email": "test@mysql.com", "employee_id": "EMP001"}' \
  --log-level debug

# Verify user creation and password hash
docker exec -it baton-mysql-test mysql -u baton -ppassword -D batondb \
  -e "SELECT username, email, password_hash FROM users WHERE username = 'test_mysql_user';"
```

#### PostgreSQL Testing

```bash
# Start PostgreSQL test environment
docker-compose -f docker-compose-postgres-test.yml up -d

# Wait for PostgreSQL to be ready (10-15 seconds)
sleep 15

# Test account creation with random password
./baton-sql --config-path examples/postgres-test.yml --provisioning \
  --create-account-login "test_postgres_user" \
  --create-account-profile '{"username": "test_postgres_user", "email": "test@postgres.com", "employee_id": "EMP002"}' \
  --log-level debug

# Verify user creation and password hash
docker exec -it baton-postgres-test psql -U baton -d batondb \
  -c "SELECT username, email, password_hash FROM users WHERE username = 'test_postgres_user';"
```

#### Oracle Testing

```bash
# Start Oracle test environment (takes 5-10 minutes to initialize)
docker-compose -f docker-compose-oracle-test.yml up -d

# Wait for Oracle to be ready (check logs: docker logs baton-oracle-test)
# Once ready, test account creation
./baton-sql --config-path examples/oracle-test.yml --provisioning \
  --create-account-login "TEST_ORACLE_USER" \
  --create-account-profile '{"username": "TEST_ORACLE_USER"}' \
  --log-level debug

# Verify user creation
echo "SELECT username, email, password_hash FROM users WHERE username = 'TEST_ORACLE_USER';" | \
  docker exec -i baton-oracle-test sqlplus -s baton/password@localhost:1521/XEPDB1
```

#### SQL Server Testing

```bash
# Start SQL Server test environment
docker-compose -f docker-compose-sqlserver-test.yml up -d

# Wait for SQL Server to be ready (30-45 seconds)
sleep 45

# Test account creation with random password
./baton-sql --config-path examples/sqlserver-test.yml --provisioning \
  --create-account-login "test_sqlserver_user" \
  --create-account-profile '{"username": "test_sqlserver_user", "email": "test@sqlserver.com", "employee_id": "EMP003"}' \
  --log-level debug

# Verify user creation in SQL Server
docker exec -it baton-sqlserver-test /opt/mssql-tools18/bin/sqlcmd -S localhost -U sa -P 'YourStrong@Passw0rd' -C \
  -Q "USE BatonTestDB; SELECT Username, Email, PasswordHash FROM Users WHERE Username = 'test_sqlserver_user';"
```

#### WordPress Testing

```bash
# Start WordPress test environment
docker-compose -f docker-compose-wordpress-test.yml up -d

# Wait for WordPress to be ready (10-15 seconds)
sleep 15

# Test account creation with random password
./baton-sql --config-path examples/wordpress-test.yml --provisioning \
  --create-account-login "test_wp_user" \
  --create-account-profile '{"username": "test_wp_user", "email": "test@wordpress.com"}' \
  --log-level debug

# Verify user creation in WordPress
docker exec -it baton-wordpress-mysql-test mysql -u wp_user -pwp_password -D wordpress \
  -e "SELECT user_login, user_email, user_pass FROM wp_users WHERE user_login = 'test_wp_user';"
```

## Database Connection Details

### MySQL

- **Host**: localhost:3306
- **Database**: batondb
- **User**: baton
- **Password**: password
- **Container**: baton-mysql-test

### PostgreSQL

- **Host**: localhost:5432
- **Database**: batondb
- **User**: baton
- **Password**: password
- **Container**: baton-postgres-test

### Oracle

- **Host**: localhost:1521
- **Service**: XEPDB1 (Pluggable Database)
- **User**: baton
- **Password**: password
- **Container**: baton-oracle-test

### SQL Server

- **Host**: localhost:1433
- **Database**: BatonTestDB
- **User**: sa
- **Password**: YourStrong@Passw0rd
- **Container**: baton-sqlserver-test

### WordPress

- **Host**: localhost:3307 (MySQL backend)
- **Database**: wordpress
- **User**: wp_user
- **Password**: wp_password
- **Container**: baton-wordpress-mysql-test

## Testing Features

### Random Password Generation

Each database configuration tests:

- **Secure password generation** (12-32 characters)
- **Database-specific hashing** (SHA2, bcrypt, MD5)
- **Account provisioning** with profile data
- **Password verification** in database

### Account Provisioning Schema

Test profiles include:

- **username** (required)
- **email** (required for most DBs)
- **employee_id** (optional)
- **custom fields** per database

### Manual Database Inspection

#### Connect to MySQL

```bash
docker exec -it baton-mysql-test mysql -u baton -ppassword batondb

# Example queries
mysql> SELECT username, email, employee_id, password_hash FROM users;
mysql> SELECT COUNT(*) as total_users FROM users;
mysql> SHOW TABLES;
```

#### Connect to PostgreSQL

```bash
docker exec -it baton-postgres-test psql -U baton -d batondb

# Example queries
batondb=> SELECT username, email, employee_id, password_hash FROM users;
batondb=> SELECT COUNT(*) as total_users FROM users;
batondb=> \dt
```

#### Connect to Oracle

```bash
# Connect via sqlplus
echo "SELECT username, email, password_hash FROM users;" | \
  docker exec -i baton-oracle-test sqlplus -s baton/password@localhost:1521/XEPDB1

# Interactive connection
docker exec -it baton-oracle-test sqlplus baton/password@localhost:1521/XEPDB1
```

#### Connect to SQL Server

```bash
docker exec -it baton-sqlserver-test /opt/mssql-tools18/bin/sqlcmd -S localhost -U sa -P 'YourStrong@Passw0rd' -C

# Example queries
1> USE BatonTestDB;
2> GO
1> SELECT Username, Email, PasswordHash FROM Users;
2> GO
1> SELECT COUNT(*) as total_users FROM Users;
2> GO
```

#### Connect to WordPress

```bash
docker exec -it baton-wordpress-mysql-test mysql -u wp_user -pwp_password wordpress

# Example queries
mysql> SELECT user_login, user_email, user_pass FROM wp_users;
mysql> SELECT COUNT(*) as total_users FROM wp_users;
mysql> SHOW TABLES LIKE 'wp_%';
```

## Troubleshooting

### Common Issues

#### Database Connection Refused

```bash
# Check if containers are running
docker ps | grep baton

# Check container logs
docker logs baton-mysql-test
docker logs baton-postgres-test
docker logs baton-oracle-test
docker logs baton-sqlserver-test
docker logs baton-wordpress-mysql-test
```

#### Oracle Takes Too Long

Oracle XE initialization can take 5-10 minutes:

```bash
# Monitor Oracle startup
docker logs -f baton-oracle-test

# Look for: "DATABASE IS READY TO USE!"
```

#### Permission Denied Errors

```bash
# Ensure user has correct permissions
# For MySQL:
docker exec -it baton-mysql-test mysql -u root -ppassword -e "SHOW GRANTS FOR 'baton'@'%';"

# For PostgreSQL:
docker exec -it baton-postgres-test psql -U postgres -c "\du baton"
```

### Debug Mode

Enable debug logging to see generated passwords:

```bash
./baton-sql --config-path examples/mysql-local-test.yml --provisioning \
  --create-account-login "debug_user" \
  --create-account-profile '{"username": "debug_user", "email": "debug@example.com"}' \
  --log-level debug
```

## Cleanup

Stop and remove all test containers:

```bash
# Stop individual environments
docker-compose -f docker-compose-mysql-test.yml down
docker-compose -f docker-compose-postgres-test.yml down
docker-compose -f docker-compose-oracle-test.yml down
docker-compose -f docker-compose-sqlserver-test.yml down
docker-compose -f docker-compose-wordpress-test.yml down

# Remove all containers and volumes
docker-compose -f docker-compose-mysql-test.yml down -v
docker-compose -f docker-compose-postgres-test.yml down -v
docker-compose -f docker-compose-oracle-test.yml down -v
docker-compose -f docker-compose-sqlserver-test.yml down -v
docker-compose -f docker-compose-wordpress-test.yml down -v
```
