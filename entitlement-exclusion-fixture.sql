-- Entitlement Exclusion Groups E2E fixture (postgres dialect).
--
-- Shape mirrors the gist but is sized up:
--   * 5 databases (the resource being granted)
--   * Up to two exclusion groups per database, exercising multi-group-per-resource
--   * 4-7 access levels per database (24 levels total across 7 connector groups)
--   * 20 users with varied status values
--   * 50 grants distributed across users and databases
--
-- Loaded by docker-compose at container init via /docker-entrypoint-initdb.d.

BEGIN;

DROP TABLE IF EXISTS database_user_access;
DROP TABLE IF EXISTS database_access_levels;
DROP TABLE IF EXISTS databases;
DROP TABLE IF EXISTS users;

CREATE TABLE users (
  username   VARCHAR(255) PRIMARY KEY,
  email      VARCHAR(255) NOT NULL,
  status     VARCHAR(64)  NOT NULL
);

CREATE TABLE databases (
  id           VARCHAR(255) PRIMARY KEY,
  display_name VARCHAR(255) NOT NULL
);

CREATE TABLE database_access_levels (
  database_id        VARCHAR(255) NOT NULL REFERENCES databases(id),
  level_id           VARCHAR(255) NOT NULL,
  display_name       VARCHAR(255) NOT NULL,
  sort_order         INTEGER      NOT NULL,
  is_default         BOOLEAN      NOT NULL,
  exclusion_group_id VARCHAR(255) NOT NULL,
  PRIMARY KEY (database_id, level_id)
);

CREATE TABLE database_user_access (
  database_id VARCHAR(255) NOT NULL,
  username    VARCHAR(255) NOT NULL REFERENCES users(username),
  level_id    VARCHAR(255) NOT NULL,
  PRIMARY KEY (database_id, username, level_id),
  FOREIGN KEY (database_id, level_id)
    REFERENCES database_access_levels(database_id, level_id)
);

-- 20 users with varied statuses.
INSERT INTO users (username, email, status) VALUES
  ('admin',           'admin@example.com',           'active'),
  ('jane.doe',        'jane.doe@example.com',        'active'),
  ('john.smith',      'john.smith@example.com',      'active'),
  ('alice.wong',      'alice.wong@example.com',      'active'),
  ('bob.chen',        'bob.chen@example.com',        'active'),
  ('carla.diaz',      'carla.diaz@example.com',      'active'),
  ('david.kim',       'david.kim@example.com',       'active'),
  ('emma.brown',      'emma.brown@example.com',      'suspended'),
  ('frank.lee',       'frank.lee@example.com',       'active'),
  ('grace.park',      'grace.park@example.com',      'active'),
  ('henry.singh',     'henry.singh@example.com',     'pending'),
  ('isabel.morales',  'isabel.morales@example.com',  'active'),
  ('james.taylor',    'james.taylor@example.com',    'active'),
  ('karen.adams',     'karen.adams@example.com',     'suspended'),
  ('liam.parker',     'liam.parker@example.com',     'active'),
  ('maria.gonzalez',  'maria.gonzalez@example.com',  'active'),
  ('nick.olsen',      'nick.olsen@example.com',      'pending'),
  ('olivia.harris',   'olivia.harris@example.com',   'active'),
  ('peter.davis',     'peter.davis@example.com',     'active'),
  ('quinn.murphy',    'quinn.murphy@example.com',    'active');

-- 5 databases.
INSERT INTO databases (id, display_name) VALUES
  ('main',      'Main DB'),
  ('analytics', 'Analytics Warehouse'),
  ('billing',   'Billing System'),
  ('reporting', 'Reporting Service'),
  ('audit',     'Audit Logs');

-- Access levels.
--
-- Each (database_id, exclusion_group_id) pair groups entitlements on the same
-- app resource and forms one C1 AppEntitlementExclusionGroup. The fixture
-- produces:
--
--   main      -> 1 group  (database-main-role-tier)        : 4 entries
--   analytics -> 1 group  (database-analytics-role-tier)   : 4 entries
--   billing   -> 2 groups (role-tier + clearance-tier)     : 4 + 3 entries
--   reporting -> 1 group  (database-reporting-role-tier)   : 4 entries
--   audit     -> 1 group  (database-audit-role-tier)       : 3 entries
--
-- => 6 distinct exclusion groups total, 22 entries total, each group has
--    exactly one is_default = true.
INSERT INTO database_access_levels
  (database_id, level_id, display_name, sort_order, is_default, exclusion_group_id) VALUES
  ('main',      'reader',                'Reader',                10, TRUE,  'database-main-role-tier'),
  ('main',      'writer',                'Writer',                20, FALSE, 'database-main-role-tier'),
  ('main',      'admin',                 'Admin',                 30, FALSE, 'database-main-role-tier'),
  ('main',      'owner',                 'Owner',                 40, FALSE, 'database-main-role-tier'),

  ('analytics', 'viewer',                'Viewer',                10, TRUE,  'database-analytics-role-tier'),
  ('analytics', 'analyst',               'Analyst',               20, FALSE, 'database-analytics-role-tier'),
  ('analytics', 'engineer',              'Engineer',              30, FALSE, 'database-analytics-role-tier'),
  ('analytics', 'admin',                 'Admin',                 40, FALSE, 'database-analytics-role-tier'),

  ('billing',   'viewer',                'Viewer',                10, TRUE,  'database-billing-role-tier'),
  ('billing',   'processor',             'Processor',             20, FALSE, 'database-billing-role-tier'),
  ('billing',   'reviewer',              'Reviewer',              30, FALSE, 'database-billing-role-tier'),
  ('billing',   'admin',                 'Admin',                 40, FALSE, 'database-billing-role-tier'),
  ('billing',   'clearance-restricted',  'Clearance: Restricted', 10, TRUE,  'database-billing-clearance-tier'),
  ('billing',   'clearance-standard',    'Clearance: Standard',   20, FALSE, 'database-billing-clearance-tier'),
  ('billing',   'clearance-elevated',    'Clearance: Elevated',   30, FALSE, 'database-billing-clearance-tier'),

  ('reporting', 'consumer',              'Consumer',              10, TRUE,  'database-reporting-role-tier'),
  ('reporting', 'author',                'Author',                20, FALSE, 'database-reporting-role-tier'),
  ('reporting', 'maintainer',            'Maintainer',            30, FALSE, 'database-reporting-role-tier'),
  ('reporting', 'admin',                 'Admin',                 40, FALSE, 'database-reporting-role-tier'),

  ('audit',     'viewer',                'Viewer',                10, TRUE,  'database-audit-role-tier'),
  ('audit',     'auditor',               'Auditor',               20, FALSE, 'database-audit-role-tier'),
  ('audit',     'admin',                 'Admin',                 30, FALSE, 'database-audit-role-tier');

-- 50 grants. Distribution is intentionally uneven so the grant-count is
-- non-trivial but every level still has at least one grantee.
INSERT INTO database_user_access (database_id, username, level_id) VALUES
  -- main
  ('main', 'admin',          'owner'),
  ('main', 'jane.doe',       'admin'),
  ('main', 'john.smith',     'writer'),
  ('main', 'alice.wong',     'writer'),
  ('main', 'bob.chen',       'reader'),
  ('main', 'carla.diaz',     'reader'),
  ('main', 'david.kim',      'reader'),
  ('main', 'frank.lee',      'reader'),
  ('main', 'grace.park',     'reader'),
  ('main', 'liam.parker',    'writer'),

  -- analytics
  ('analytics', 'admin',          'admin'),
  ('analytics', 'jane.doe',       'engineer'),
  ('analytics', 'alice.wong',     'engineer'),
  ('analytics', 'bob.chen',       'analyst'),
  ('analytics', 'maria.gonzalez', 'analyst'),
  ('analytics', 'olivia.harris',  'analyst'),
  ('analytics', 'peter.davis',    'viewer'),
  ('analytics', 'quinn.murphy',   'viewer'),
  ('analytics', 'isabel.morales', 'viewer'),

  -- billing: role-tier
  ('billing', 'admin',         'admin'),
  ('billing', 'jane.doe',      'reviewer'),
  ('billing', 'john.smith',    'processor'),
  ('billing', 'david.kim',     'processor'),
  ('billing', 'grace.park',    'viewer'),
  ('billing', 'maria.gonzalez','viewer'),

  -- billing: clearance-tier (same users can be in both groups)
  ('billing', 'admin',         'clearance-elevated'),
  ('billing', 'jane.doe',      'clearance-elevated'),
  ('billing', 'john.smith',    'clearance-standard'),
  ('billing', 'david.kim',     'clearance-standard'),
  ('billing', 'grace.park',    'clearance-restricted'),
  ('billing', 'maria.gonzalez','clearance-restricted'),

  -- reporting
  ('reporting', 'admin',          'admin'),
  ('reporting', 'james.taylor',   'maintainer'),
  ('reporting', 'isabel.morales', 'author'),
  ('reporting', 'olivia.harris',  'author'),
  ('reporting', 'liam.parker',    'author'),
  ('reporting', 'peter.davis',    'consumer'),
  ('reporting', 'quinn.murphy',   'consumer'),
  ('reporting', 'frank.lee',      'consumer'),
  ('reporting', 'carla.diaz',     'consumer'),

  -- audit
  ('audit', 'admin',         'admin'),
  ('audit', 'james.taylor',  'auditor'),
  ('audit', 'jane.doe',      'auditor'),
  ('audit', 'alice.wong',    'viewer'),
  ('audit', 'bob.chen',      'viewer'),
  ('audit', 'carla.diaz',    'viewer'),
  ('audit', 'david.kim',     'viewer'),
  ('audit', 'frank.lee',     'viewer'),
  ('audit', 'grace.park',    'viewer'),
  ('audit', 'liam.parker',   'viewer');

COMMIT;
