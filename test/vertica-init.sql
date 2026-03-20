-- Create users table
CREATE TABLE IF NOT EXISTS users (
  id AUTO_INCREMENT PRIMARY KEY,
  username VARCHAR(100) NOT NULL,
  email VARCHAR(255) NOT NULL,
  employee_id VARCHAR(50),
  status VARCHAR(20) DEFAULT 'active',
  account_type VARCHAR(20) DEFAULT 'human',
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  last_login TIMESTAMP NULL,
  manager_id INTEGER
);

-- Insert sample users
INSERT INTO users (username, email, employee_id, status, account_type, created_at, last_login) VALUES
('admin', 'admin@example.com', 'EMP001', 'active', 'human', '2025-01-01 12:00:00', '2025-04-15 09:30:00'),
('jane.doe', 'jane.doe@example.com', 'EMP002', 'active', 'human', '2025-01-05 14:30:00', '2025-04-17 08:45:00'),
('john.smith', 'john.smith@example.com', 'EMP003', 'active', 'human', '2025-01-10 09:45:00', '2025-04-16 16:20:00'),
('service.acct', 'service@example.com', 'SVC001', 'active', 'service', '2025-02-01 08:00:00', NULL),
('disabled.user', 'disabled@example.com', 'EMP004', 'disabled', 'human', '2025-02-15 10:15:00', '2025-03-01 11:10:00');

-- Update manager relationships
UPDATE users SET manager_id = 1 WHERE username IN ('jane.doe', 'john.smith');
UPDATE users SET manager_id = 2 WHERE username = 'service.acct';
UPDATE users SET manager_id = 3 WHERE username = 'disabled.user';

-- Create roles table
CREATE TABLE IF NOT EXISTS roles (
  id AUTO_INCREMENT PRIMARY KEY,
  role_name VARCHAR(100) NOT NULL
);

-- Insert sample roles
INSERT INTO roles (role_name) VALUES ('admin');
INSERT INTO roles (role_name) VALUES ('user');
INSERT INTO roles (role_name) VALUES ('reader');

-- Create user_roles table
CREATE TABLE IF NOT EXISTS user_roles (
  user_id INTEGER,
  role_id INTEGER,
  PRIMARY KEY (user_id, role_id)
);

-- Assign roles to users
INSERT INTO user_roles (user_id, role_id) VALUES (1, 1);
INSERT INTO user_roles (user_id, role_id) VALUES (2, 2);
INSERT INTO user_roles (user_id, role_id) VALUES (3, 2);
INSERT INTO user_roles (user_id, role_id) VALUES (3, 3);
INSERT INTO user_roles (user_id, role_id) VALUES (4, 2);

SELECT 'Vertica test database initialized successfully' AS message;
