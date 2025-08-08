# Baton SQL Connector Documentation

While developing the connector, please fill out this form. This information is needed to write docs and to help other users set up the connector.

## Connector capabilities

1. What resources does the connector sync?

   > The Baton SQL connector syncs users, roles, and other resource types from SQL databases. Supported database systems include:
   >
   > - MySQL
   > - PostgreSQL
   > - Oracle Database
   > - SQL Server
   > - WordPress (MySQL-based)
   >
   > The connector can sync custom user tables, role hierarchies, entitlements, and permissions based on configurable SQL queries.

2. Can the connector provision any resources? If so, which ones?

   > Yes, the connector can provision user accounts with the following capabilities:
   >
   > - **User Account Creation**: Create new user accounts with configurable fields (username, email, employee_id, etc.)
   > - **Random Password Generation**: Automatically generate secure random passwords (12-32 characters) with appropriate hashing for each database type:
   >   - MySQL: SHA2 hashing
   >   - PostgreSQL: bcrypt hashing
   >   - WordPress: MD5 hashing
   >   - Oracle: SHA2 hashing
   >   - SQL Server: SHA2 hashing
   > - **Role Assignment Management**: Grant and revoke role memberships and entitlements for users
   > - **Custom Provisioning Logic**: Execute custom SQL queries for complex provisioning workflows

## Connector credentials

1. What credentials or information are needed to set up the connector?

   > The connector requires database connection credentials and configuration:
   >
   > - **Database Connection String (DSN)**: A connection string containing host, port, database name, username, and password
   > - **Database User Credentials**: Username and password for a database user with appropriate permissions
   > - **Configuration File**: A YAML configuration file defining resource types, queries, and mappings
   >
   > **Example DSN formats:**
   >
   > - MySQL: `mysql://username:password@host:port/database`
   > - PostgreSQL: `postgres://username:password@host:port/database`
   > - Oracle: `oracle://username:password@host:port/service`
   > - SQL Server: `sqlserver://username:password@host:port?database=dbname`

2. For each item in the list above:

   - How does a user create or look up that credential or info? Please include links to (non-gated) documentation, screenshots (of the UI or of gated docs), or a video of the process.

     > **Database Connection String**:
     >
     > - Obtain from your database administrator or cloud provider console
     > - For AWS RDS: Available in the RDS console under "Connectivity & security"
     > - For Google Cloud SQL: Available in the Cloud SQL console under "Overview"
     > - For Azure Database: Available in the Azure portal under "Connection strings"
     >
     > **Database User Credentials**:
     >
     > - Create a dedicated database user for the connector (recommended for security)
     > - For MySQL: Use `CREATE USER` and `GRANT` statements
     > - For PostgreSQL: Use `CREATE ROLE` and `GRANT` statements
     > - For Oracle: Use `CREATE USER` and `GRANT` statements
     > - Refer to your database documentation for user management procedures
     >
     > **Configuration File**:
     >
     > - Use the provided example configurations in the `examples/` directory
     > - Customize SQL queries to match your database schema
     > - Define resource mappings based on your table structure

   - Does the credential need any specific scopes or permissions? If so, list them here.

     > The database user account needs different permissions depending on the operations:
     >
     > **For Read-Only Sync Operations:**
     >
     > - `SELECT` permissions on user, role, and entitlement tables
     > - Access to system tables for listing database users and roles (if applicable)
     >
     > **For Provisioning Operations (Read-Write):**
     >
     > - All read permissions listed above
     > - `INSERT` permissions on user tables for account creation
     > - `UPDATE` permissions for modifying user attributes
     > - `DELETE` permissions for account deprovisioning (if implemented)
     > - For Oracle: `CREATE USER`, `GRANT`, and `REVOKE` system privileges for database user management
     >
     > **Database-Specific Requirements:**
     >
     > - **MySQL**: `CREATE USER`, `GRANT OPTION` for user management
     > - **PostgreSQL**: `CREATEROLE` privilege for user management, access to `pgcrypto` extension for password hashing
     > - **Oracle**: `DBA` role or specific system privileges (`CREATE USER`, `ALTER USER`, `DROP USER`)
     > - **WordPress**: Standard MySQL permissions on `wp_users` and `wp_usermeta` tables

   - If applicable: Is the list of scopes or permissions different to sync (read) versus provision (read-write)? If so, list the difference here.

     > Yes, the permissions differ significantly:
     >
     > **Sync (Read-only) Operations:**
     >
     > - `SELECT` on user and role tables
     > - `SELECT` on system catalog tables (for database-native users/roles)
     > - No modification permissions required
     >
     > **Provision (Read-Write) Operations:**
     >
     > - All read permissions from sync operations
     > - `INSERT`, `UPDATE`, `DELETE` on user tables
     > - System-level user management privileges (for database-native provisioning)
     > - Permission to execute stored procedures or functions (if used for provisioning)
     > - Access to password hashing functions (`SHA2`, `crypt`, `gen_salt`, etc.)

   - What level of access or permissions does the user need in order to create the credentials?

     > To create the necessary database credentials, you need:
     >
     > **Database Administrator Access:**
     >
     > - MySQL: `root` user or user with `GRANT OPTION` and `CREATE USER` privileges
     > - PostgreSQL: Superuser or user with `CREATEROLE` and `GRANT` privileges
     > - Oracle: `DBA` role or `SYSDBA` privileges
     > - SQL Server: `sysadmin` server role or `securityadmin` + `dbowner` roles
     >
     > **Cloud Database Services:**
     >
     > - AWS RDS: IAM permissions to manage database users or use the master user
     > - Google Cloud SQL: Cloud SQL Admin role or equivalent IAM permissions
     > - Azure SQL Database: SQL authentication with admin credentials or Azure AD admin rights
     >
     > **Security Best Practices:**
     >
     > - Create a dedicated service account with minimal required permissions
     > - Use connection pooling and SSL/TLS encryption
     > - Rotate credentials regularly
     > - Monitor database access logs for the connector account

## Configuration Examples

The connector includes example configurations for common scenarios:

- `examples/mysql-test.yml` - MySQL with employee data and random password support
- `examples/postgres-test.yml` - PostgreSQL with bcrypt password hashing
- `examples/oracle-test.yml` - Oracle with SHA2-256
- `examples/wordpress-test.yml` - WordPress user and role management
- `examples/sqlserver-test.yml` - SQL Server with SHA2-256 password hashing

Each example demonstrates:

- Database connection configuration
- Resource type definitions (users, roles, etc.)
- Account provisioning with random password generation
- Entitlement management and role assignments
- Custom SQL queries for different use cases
