-- SQL Server initialization script for Baton SQL testing
-- This script creates the necessary tables and sample data for testing user provisioning
-- and random password generation functionality

-- Create Users table
CREATE TABLE Users (
    UserID INT IDENTITY(1,1) PRIMARY KEY,
    Username NVARCHAR(100) NOT NULL UNIQUE,
    Email NVARCHAR(255) NOT NULL,
    EmployeeID NVARCHAR(50) NULL,
    IsActive BIT DEFAULT 1,
    AccountType NVARCHAR(20) DEFAULT 'human',
    CreatedAt DATETIME2 DEFAULT GETDATE(),
    LastLogin DATETIME2 NULL,
    ManagerID INT NULL,
    PasswordHash VARBINARY(32) NULL,
    FOREIGN KEY (ManagerID) REFERENCES Users(UserID)
);

-- Create Roles table
CREATE TABLE Roles (
    RoleID INT IDENTITY(1,1) PRIMARY KEY,
    RoleName NVARCHAR(100) NOT NULL UNIQUE,
    Description NVARCHAR(255) NOT NULL
);

-- Create UserRoles junction table for many-to-many relationship
CREATE TABLE UserRoles (
    UserID INT,
    RoleID INT,
    AssignedAt DATETIME2 DEFAULT GETDATE(),
    PRIMARY KEY (UserID, RoleID),
    FOREIGN KEY (UserID) REFERENCES Users(UserID) ON DELETE CASCADE,
    FOREIGN KEY (RoleID) REFERENCES Roles(RoleID) ON DELETE CASCADE
);

-- Create LoginHistory table for tracking login attempts
CREATE TABLE LoginHistory (
    LoginID INT IDENTITY(1,1) PRIMARY KEY,
    UserID INT,
    LoginTime DATETIME2 DEFAULT GETDATE(),
    LoginTimeText NVARCHAR(50),
    LoginTimeAlt NVARCHAR(50),
    LoginSuccess BIT DEFAULT 1,
    FOREIGN KEY (UserID) REFERENCES Users(UserID) ON DELETE CASCADE
);

-- Create EmployeeData table for additional employee information
CREATE TABLE EmployeeData (
    EmployeeDataID INT IDENTITY(1,1) PRIMARY KEY,
    UserID INT,
    EmployeeID NVARCHAR(50),
    EmployeeNumber INT,
    EmployeeCode NVARCHAR(20),
    Department NVARCHAR(100),
    JobTitle NVARCHAR(100),
    HireDate DATETIME2,
    FOREIGN KEY (UserID) REFERENCES Users(UserID) ON DELETE CASCADE
);

-- Insert sample roles
INSERT INTO Roles (RoleName, Description) VALUES
('admin', 'Administrator role with full access'),
('user', 'Regular user role with standard access'),
('reader', 'Read-only access role'),
('manager', 'Management role with team oversight'),
('developer', 'Developer role with development access');

-- Insert sample users (without manager relationships first)
INSERT INTO Users (Username, Email, EmployeeID, IsActive, AccountType, CreatedAt, LastLogin, PasswordHash) VALUES
('admin', 'admin@example.com', 'EMP001', 1, 'human', '2025-01-01 12:00:00', '2025-04-15 09:30:00', HASHBYTES('SHA2_256', 'password123')),
('jane.doe', 'jane.doe@example.com', 'EMP002', 1, 'human', '2025-01-05 14:30:00', '2025-04-17 08:45:00', HASHBYTES('SHA2_256', 'password123')),
('john.smith', 'john.smith@example.com', 'EMP003', 1, 'human', '2025-01-10 09:45:00', '2025-04-16 16:20:00', HASHBYTES('SHA2_256', 'password123')),
('service.acct', 'service@example.com', 'SVC001', 1, 'service', '2025-02-01 08:00:00', NULL, HASHBYTES('SHA2_256', 'password123')),
('disabled.user', 'disabled@example.com', 'EMP004', 0, 'human', '2025-02-15 10:15:00', '2025-03-01 11:10:00', HASHBYTES('SHA2_256', 'password123')),
('alice.manager', 'alice.manager@example.com', 'EMP005', 1, 'human', '2025-03-01 09:00:00', '2025-04-18 10:15:00', HASHBYTES('SHA2_256', 'password123')),
('bob.developer', 'bob.developer@example.com', 'EMP006', 1, 'human', '2025-03-05 11:30:00', '2025-04-18 14:30:00', HASHBYTES('SHA2_256', 'password123'));

-- Update users to establish manager relationships
-- jane.doe and john.smith report to admin
UPDATE Users SET ManagerID = 1 WHERE Username IN ('jane.doe', 'john.smith');

-- service.acct reports to jane.doe
UPDATE Users SET ManagerID = (SELECT UserID FROM Users WHERE Username = 'jane.doe') WHERE Username = 'service.acct';

-- disabled.user reports to john.smith
UPDATE Users SET ManagerID = (SELECT UserID FROM Users WHERE Username = 'john.smith') WHERE Username = 'disabled.user';

-- alice.manager reports to admin
UPDATE Users SET ManagerID = 1 WHERE Username = 'alice.manager';

-- bob.developer reports to alice.manager
UPDATE Users SET ManagerID = (SELECT UserID FROM Users WHERE Username = 'alice.manager') WHERE Username = 'bob.developer';

-- Assign roles to users
INSERT INTO UserRoles (UserID, RoleID) VALUES
((SELECT UserID FROM Users WHERE Username = 'admin'), (SELECT RoleID FROM Roles WHERE RoleName = 'admin')),
((SELECT UserID FROM Users WHERE Username = 'jane.doe'), (SELECT RoleID FROM Roles WHERE RoleName = 'user')),
((SELECT UserID FROM Users WHERE Username = 'john.smith'), (SELECT RoleID FROM Roles WHERE RoleName = 'user')),
((SELECT UserID FROM Users WHERE Username = 'john.smith'), (SELECT RoleID FROM Roles WHERE RoleName = 'reader')),
((SELECT UserID FROM Users WHERE Username = 'service.acct'), (SELECT RoleID FROM Roles WHERE RoleName = 'user')),
((SELECT UserID FROM Users WHERE Username = 'alice.manager'), (SELECT RoleID FROM Roles WHERE RoleName = 'manager')),
((SELECT UserID FROM Users WHERE Username = 'alice.manager'), (SELECT RoleID FROM Roles WHERE RoleName = 'admin')),
((SELECT UserID FROM Users WHERE Username = 'bob.developer'), (SELECT RoleID FROM Roles WHERE RoleName = 'developer')),
((SELECT UserID FROM Users WHERE Username = 'bob.developer'), (SELECT RoleID FROM Roles WHERE RoleName = 'user'));

-- Insert sample login history with various date formats
INSERT INTO LoginHistory (UserID, LoginTime, LoginTimeText, LoginTimeAlt, LoginSuccess) VALUES
((SELECT UserID FROM Users WHERE Username = 'admin'), '2025-04-15 09:30:00', '15-APR-2025 09:30:00', '15/04/2025 09:30:00', 1),
((SELECT UserID FROM Users WHERE Username = 'jane.doe'), '2025-04-17 08:45:00', '17-APR-2025 08:45:00', '17/04/2025 08:45:00', 1),
((SELECT UserID FROM Users WHERE Username = 'john.smith'), '2025-04-16 16:20:00', '16-APR-2025 16:20:00', '16/04/2025 16:20:00', 1),
((SELECT UserID FROM Users WHERE Username = 'disabled.user'), '2025-03-01 11:10:00', '01-MAR-2025 11:10:00', '01/03/2025 11:10:00', 1),
((SELECT UserID FROM Users WHERE Username = 'alice.manager'), '2025-04-18 10:15:00', '18-APR-2025 10:15:00', '18/04/2025 10:15:00', 1),
((SELECT UserID FROM Users WHERE Username = 'bob.developer'), '2025-04-18 14:30:00', '18-APR-2025 14:30:00', '18/04/2025 14:30:00', 1);

-- Insert sample employee data
INSERT INTO EmployeeData (UserID, EmployeeID, EmployeeNumber, EmployeeCode, Department, JobTitle, HireDate) VALUES
((SELECT UserID FROM Users WHERE Username = 'admin'), 'EMP001', 10001, 'E-10001', 'IT', 'System Administrator', '2025-01-01'),
((SELECT UserID FROM Users WHERE Username = 'jane.doe'), 'EMP002', 10002, 'E-10002', 'HR', 'HR Specialist', '2025-01-05'),
((SELECT UserID FROM Users WHERE Username = 'john.smith'), 'EMP003', 10003, 'E-10003', 'Finance', 'Financial Analyst', '2025-01-10'),
((SELECT UserID FROM Users WHERE Username = 'service.acct'), 'SVC001', 20001, 'S-20001', 'IT', 'Service Account', '2025-02-01'),
((SELECT UserID FROM Users WHERE Username = 'disabled.user'), 'EMP004', 10004, 'E-10004', 'Marketing', 'Marketing Coordinator', '2025-02-15'),
((SELECT UserID FROM Users WHERE Username = 'alice.manager'), 'EMP005', 10005, 'E-10005', 'IT', 'IT Manager', '2025-03-01'),
((SELECT UserID FROM Users WHERE Username = 'bob.developer'), 'EMP006', 10006, 'E-10006', 'IT', 'Software Developer', '2025-03-05');

-- Create indexes for better performance
CREATE INDEX IX_Users_Username ON Users(Username);
CREATE INDEX IX_Users_Email ON Users(Email);
CREATE INDEX IX_Users_EmployeeID ON Users(EmployeeID);
CREATE INDEX IX_UserRoles_UserID ON UserRoles(UserID);
CREATE INDEX IX_UserRoles_RoleID ON UserRoles(RoleID);

-- Print confirmation message
PRINT 'Baton SQL Server test database initialized successfully';
PRINT 'Database: BatonTestDB';
PRINT 'Tables created: Users, Roles, UserRoles, LoginHistory, EmployeeData';
PRINT 'Sample data inserted for testing account provisioning and random password generation';

-- Display sample data
SELECT 'Sample Users:' AS Info;
SELECT Username, Email, EmployeeID, IsActive, AccountType FROM Users;

SELECT 'Sample Roles:' AS Info;
SELECT RoleName, Description FROM Roles;

SELECT 'Sample User-Role Assignments:' AS Info;
SELECT u.Username, r.RoleName 
FROM UserRoles ur
JOIN Users u ON ur.UserID = u.UserID
JOIN Roles r ON ur.RoleID = r.RoleID
ORDER BY u.Username, r.RoleName;