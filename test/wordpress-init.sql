-- Drop existing tables if they exist
DROP TABLE IF EXISTS wp_usermeta;
DROP TABLE IF EXISTS wp_users;

-- Create wp_users table (WordPress standard table)
CREATE TABLE wp_users (
  ID bigint(20) unsigned NOT NULL AUTO_INCREMENT,
  user_login varchar(60) NOT NULL DEFAULT '',
  user_pass varchar(255) NOT NULL DEFAULT '',
  user_nicename varchar(50) NOT NULL DEFAULT '',
  user_email varchar(100) NOT NULL DEFAULT '',
  user_url varchar(100) NOT NULL DEFAULT '',
  user_registered datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  user_activation_key varchar(255) NOT NULL DEFAULT '',
  user_status int(11) NOT NULL DEFAULT 0,
  display_name varchar(250) NOT NULL DEFAULT '',
  PRIMARY KEY (ID),
  KEY user_login_key (user_login),
  KEY user_nicename (user_nicename),
  KEY user_email (user_email)
);

-- Create wp_usermeta table (WordPress standard table for user metadata)
CREATE TABLE wp_usermeta (
  umeta_id bigint(20) unsigned NOT NULL AUTO_INCREMENT,
  user_id bigint(20) unsigned NOT NULL DEFAULT 0,
  meta_key varchar(255) DEFAULT NULL,
  meta_value longtext DEFAULT NULL,
  PRIMARY KEY (umeta_id),
  KEY user_id (user_id),
  KEY meta_key (meta_key(191))
);

-- Insert sample WordPress users
INSERT INTO wp_users (user_login, user_pass, user_nicename, user_email, user_registered, display_name) VALUES
('admin', MD5('admin123'), 'admin', 'admin@example.com', '2025-01-01 12:00:00', 'Administrator'),
('editor', MD5('editor123'), 'editor', 'editor@example.com', '2025-01-05 14:30:00', 'Editor User'),
('author', MD5('author123'), 'author', 'author@example.com', '2025-01-10 09:45:00', 'Author User'),
('subscriber', MD5('subscriber123'), 'subscriber', 'subscriber@example.com', '2025-02-01 08:00:00', 'Subscriber User');

-- Insert user capabilities (WordPress roles)
INSERT INTO wp_usermeta (user_id, meta_key, meta_value) VALUES
(1, 'wp_capabilities', 'a:1:{s:13:"administrator";b:1;}'),
(2, 'wp_capabilities', 'a:1:{s:6:"editor";b:1;}'),
(3, 'wp_capabilities', 'a:1:{s:6:"author";b:1;}'),
(4, 'wp_capabilities', 'a:1:{s:10:"subscriber";b:1;}');

-- Insert user levels (WordPress legacy)
INSERT INTO wp_usermeta (user_id, meta_key, meta_value) VALUES
(1, 'wp_user_level', '10'),
(2, 'wp_user_level', '7'),
(3, 'wp_user_level', '2'),
(4, 'wp_user_level', '0');

-- Print success message
SELECT 'WordPress test database initialized successfully' as message;