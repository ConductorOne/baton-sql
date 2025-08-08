-- Enable pgcrypto extension for password hashing
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- Drop existing tables if they exist
DROP TABLE IF EXISTS employee_data;
DROP TABLE IF EXISTS login_history;
DROP TABLE IF EXISTS user_roles;
DROP TABLE IF EXISTS roles;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS features;

-- Create users table
CREATE TABLE users (
  id SERIAL PRIMARY KEY,
  username VARCHAR(100) NOT NULL,
  email VARCHAR(255) NOT NULL,
  employee_id VARCHAR(50),
  status VARCHAR(20) DEFAULT 'active',
  account_type VARCHAR(20) DEFAULT 'human',
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  last_login TIMESTAMP NULL,
  manager_id INTEGER,
  password_hash VARCHAR(255)
);

-- Insert sample users with different date formats for last_login (without manager relationships)
INSERT INTO users (username, email, employee_id, status, account_type, created_at, last_login) VALUES
('admin', 'admin@example.com', 'EMP001', 'active', 'human', '2025-01-01 12:00:00', '2025-04-15 09:30:00'),
('jane.doe', 'jane.doe@example.com', 'EMP002', 'active', 'human', '2025-01-05 14:30:00', '2025-04-17 08:45:00'),
('john.smith', 'john.smith@example.com', 'EMP003', 'active', 'human', '2025-01-10 09:45:00', '2025-04-16 16:20:00'),
('service.acct', 'service@example.com', 'SVC001', 'active', 'service', '2025-02-01 08:00:00', NULL),
('disabled.user', 'disabled@example.com', 'EMP004', 'disabled', 'human', '2025-02-15 10:15:00', '2025-03-01 11:10:00'),
('bjorn.tipling.c1', 'bjorn.tipling@conductorone.com', 'EMP005', 'active', 'human', '2025-03-01 09:00:00', '2025-04-18 10:15:00'),
('bjorn.tipling.ins', 'bjorn.tipling@insulator.one', 'EMP006', 'active', 'human', '2025-03-05 11:30:00', '2025-04-18 14:30:00');

-- Update users to establish manager relationships
-- jane.doe and john.smith report to admin
UPDATE users SET manager_id = 1 WHERE username IN ('jane.doe', 'john.smith');

-- service.acct reports to jane.doe
UPDATE users SET manager_id = 2 WHERE username = 'service.acct';

-- disabled.user reports to john.smith
UPDATE users SET manager_id = 3 WHERE username = 'disabled.user';

-- bjorn.tipling.c1 reports to admin
UPDATE users SET manager_id = 1 WHERE username = 'bjorn.tipling.c1';

-- bjorn.tipling.ins reports to bjorn.tipling.c1
UPDATE users SET manager_id = (SELECT id FROM users WHERE username = 'bjorn.tipling.c1') WHERE username = 'bjorn.tipling.ins';

-- Create roles table
CREATE TABLE roles (
  id SERIAL PRIMARY KEY,
  role_name VARCHAR(100) NOT NULL
);

-- Insert sample roles
INSERT INTO roles (role_name) VALUES
('admin'),
('user'),
('reader');

-- Create user_roles table for many-to-many relationship
CREATE TABLE user_roles (
  user_id INTEGER,
  role_id INTEGER,
  PRIMARY KEY (user_id, role_id),
  FOREIGN KEY (user_id) REFERENCES users(id),
  FOREIGN KEY (role_id) REFERENCES roles(id)
);

-- Assign roles to users
INSERT INTO user_roles (user_id, role_id) VALUES
(1, 1), -- admin has admin role
(2, 2), -- jane.doe has user role
(3, 2), -- john.smith has user role
(3, 3), -- john.smith also has reader role
(4, 2), -- service.acct has user role
(6, 1), -- bjorn.tipling.c1 has admin role
(7, 2); -- bjorn.tipling.ins has user role

-- Create table to track last login attempts (for testing date formats)
CREATE TABLE login_history (
  id SERIAL PRIMARY KEY,
  user_id INTEGER,
  login_time TIMESTAMP,
  login_time_text VARCHAR(50),
  login_time_alt VARCHAR(50),
  FOREIGN KEY (user_id) REFERENCES users(id)
);

-- Insert various date formats for testing
INSERT INTO login_history (user_id, login_time, login_time_text, login_time_alt) VALUES
(1, '2025-04-15 09:30:00', '15-APR-2025 09:30:00', '15/04/2025 09:30:00'),
(2, '2025-04-17 08:45:00', '17-APR-2025 08:45:00', '17/04/2025 08:45:00'),
(3, '2025-04-16 16:20:00', '16-APR-2025 16:20:00', '16/04/2025 16:20:00'),
(5, '2025-03-01 11:10:00', '01-MAR-2025 11:10:00', '01/03/2025 11:10:00'),
(6, '2025-04-18 10:15:00', '18-APR-2025 10:15:00', '18/04/2025 10:15:00'),
(7, '2025-04-18 14:30:00', '18-APR-2025 14:30:00', '18/04/2025 14:30:00');

-- Create test table for employee IDs in different formats
CREATE TABLE employee_data (
  id SERIAL PRIMARY KEY,
  user_id INTEGER,
  employee_id VARCHAR(50),
  employee_number INTEGER,
  employee_code VARCHAR(20),
  FOREIGN KEY (user_id) REFERENCES users(id)
);

-- Insert sample employee ID data
INSERT INTO employee_data (user_id, employee_id, employee_number, employee_code) VALUES
(1, 'EMP001', 10001, 'E-10001'),
(2, 'EMP002', 10002, 'E-10002'),
(3, 'EMP003', 10003, 'E-10003'),
(4, 'SVC001', 20001, 'S-20001'),
(5, 'EMP004', 10004, 'E-10004'),
(6, 'EMP005', 10005, 'E-10005'),
(7, 'EMP006', 10006, 'E-10006');

-- Create features table
CREATE TABLE features (
  id SERIAL PRIMARY KEY,
  name VARCHAR(100) NOT NULL,
  description VARCHAR(255) NOT NULL,
  is_deleted BOOLEAN DEFAULT FALSE
);

-- Insert sample features
INSERT INTO features (name, description) VALUES
('feature1', 'Feature 1 description'),
('feature2', 'Feature 2 description'),
('feature3', 'Feature 3 description');

-- Create feature_roles table
CREATE TABLE feature_roles (
  feature_id INTEGER,
  role_id INTEGER,
  PRIMARY KEY (feature_id, role_id),
  FOREIGN KEY (feature_id) REFERENCES features(id),
  FOREIGN KEY (role_id) REFERENCES roles(id)
);

-- Assign roles to features
INSERT INTO feature_roles (feature_id, role_id) VALUES
(1, 1), -- feature1 has admin role
(2, 2); -- feature2 has user role

-- Print a message indicating successful setup
SELECT 'Baton SQL test database initialized successfully' as message;