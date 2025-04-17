-- Create users table
CREATE TABLE users (
  id INT AUTO_INCREMENT PRIMARY KEY,
  username VARCHAR(100) NOT NULL,
  email VARCHAR(255) NOT NULL,
  employee_id VARCHAR(50),
  status VARCHAR(20) DEFAULT 'active',
  account_type VARCHAR(20) DEFAULT 'human',
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  last_login TIMESTAMP NULL,
  manager_id INT
);

-- Insert sample users with different date formats for last_login
INSERT INTO users (username, email, employee_id, status, account_type, created_at, last_login) VALUES
('admin', 'admin@example.com', 'EMP001', 'active', 'human', '2023-01-01 12:00:00', '2023-04-15 09:30:00'),
('jane.doe', 'jane.doe@example.com', 'EMP002', 'active', 'human', '2023-01-05 14:30:00', '2023-04-17 08:45:00'),
('john.smith', 'john.smith@example.com', 'EMP003', 'active', 'human', '2023-01-10 09:45:00', '2023-04-16 16:20:00'),
('service.acct', 'service@example.com', 'SVC001', 'active', 'service', '2023-02-01 08:00:00', NULL),
('disabled.user', 'disabled@example.com', 'EMP004', 'disabled', 'human', '2023-02-15 10:15:00', '2023-03-01 11:10:00');

-- Create roles table
CREATE TABLE roles (
  id INT AUTO_INCREMENT PRIMARY KEY,
  role_name VARCHAR(100) NOT NULL
);

-- Insert sample roles
INSERT INTO roles (role_name) VALUES
('admin'),
('user'),
('reader');

-- Create user_roles table for many-to-many relationship
CREATE TABLE user_roles (
  user_id INT,
  role_id INT,
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
(4, 2); -- service.acct has user role

-- Create table to track last login attempts (for testing date formats)
CREATE TABLE login_history (
  id INT AUTO_INCREMENT PRIMARY KEY,
  user_id INT,
  login_time TIMESTAMP,
  login_time_text VARCHAR(50),
  login_time_alt VARCHAR(50),
  FOREIGN KEY (user_id) REFERENCES users(id)
);

-- Insert various date formats for testing
INSERT INTO login_history (user_id, login_time, login_time_text, login_time_alt) VALUES
(1, '2023-04-15 09:30:00', '15-APR-2023 09:30:00', '15/04/2023 09:30:00'),
(2, '2023-04-17 08:45:00', '17-APR-2023 08:45:00', '17/04/2023 08:45:00'),
(3, '2023-04-16 16:20:00', '16-APR-2023 16:20:00', '16/04/2023 16:20:00'),
(5, '2023-03-01 11:10:00', '01-MAR-2023 11:10:00', '01/03/2023 11:10:00');

-- Create test table for employee IDs in different formats
CREATE TABLE employee_data (
  id INT AUTO_INCREMENT PRIMARY KEY,
  user_id INT,
  employee_id VARCHAR(50),
  employee_number INT,
  employee_code VARCHAR(20),
  FOREIGN KEY (user_id) REFERENCES users(id)
);

-- Insert sample employee ID data
INSERT INTO employee_data (user_id, employee_id, employee_number, employee_code) VALUES
(1, 'EMP001', 10001, 'E-10001'),
(2, 'EMP002', 10002, 'E-10002'),
(3, 'EMP003', 10003, 'E-10003'),
(4, 'SVC001', 20001, 'S-20001'),
(5, 'EMP004', 10004, 'E-10004');

-- Print a message indicating successful setup
SELECT 'Baton SQL test database initialized successfully' as message;