-- Insert test users into the main PSOPRDEFN table
INSERT INTO psoprdefn (oprid, useridalias, emplid, emailid, symbolicid, defaultnavhp, rowsecclass, prcsprflcls, oprclass) VALUES
('JSMITH', 'john.smith', 'EMP001', 'john.smith@company.com', 'SYM001', 'HOMEPAGE1', 'ROWSEC1', 'PROC1', 'ADMIN'),
('MDOE', 'mary.doe', 'EMP002', 'mary.doe@company.com', 'SYM002', 'HOMEPAGE2', 'ROWSEC2', 'PROC2', 'USER'),
('BJONES', 'bob.jones', 'EMP003', 'bob.jones@company.com', 'SYM003', 'HOMEPAGE1', 'ROWSEC1', 'PROC1', 'MANAGER'),
('AWILSON', 'alice.wilson', 'EMP004', 'alice.wilson@company.com', 'SYM004', 'HOMEPAGE3', 'ROWSEC3', 'PROC3', 'USER'),
('CTAYLOR', 'charlie.taylor', 'EMP005', 'charlie.taylor@company.com', 'SYM005', 'HOMEPAGE2', 'ROWSEC2', 'PROC2', 'ADMIN'),
('DLEE', 'diana.lee', 'EMP006', 'diana.lee@company.com', 'SYM006', 'HOMEPAGE1', 'ROWSEC1', 'PROC1', 'USER');

-- Insert test data for Accounts Payable operators
INSERT INTO ps_opr_def_tbl_ap (oprid, origin) VALUES
('JSMITH', 'INTERNAL'),
('MDOE', 'EXTERNAL'),
('BJONES', 'INTERNAL'),
('CTAYLOR', 'INTERNAL');

-- Insert test data for Financial Services operators
INSERT INTO ps_opr_def_tbl_fs (oprid) VALUES
('JSMITH'),
('AWILSON'),
('CTAYLOR'),
('DLEE');

-- Insert test data for General Ledger operators
INSERT INTO ps_opr_def_tbl_gl (oprid) VALUES
('JSMITH'),
('BJONES'),
('CTAYLOR');

-- Insert test data for Project Management operators
INSERT INTO ps_opr_def_tbl_pm (oprid, origin) VALUES
('BJONES', 'INTERNAL'),
('AWILSON', 'EXTERNAL'),
('DLEE', 'INTERNAL');

-- Insert test data for Vendor operators
INSERT INTO ps_opr_def_tbl_vnd (oprid) VALUES
('MDOE'),
('BJONES'),
('AWILSON'),
('CTAYLOR');

-- Insert test roles and role assignments
INSERT INTO psroleuser (rolename, roleuser) VALUES
-- Admin roles
('PS_ADMIN', 'JSMITH'),
('PS_ADMIN', 'CTAYLOR'),
-- Financial roles
('PS_FINANCIAL_ANALYST', 'JSMITH'),
('PS_FINANCIAL_ANALYST', 'AWILSON'),
('PS_FINANCIAL_ANALYST', 'DLEE'),
-- AP roles
('PS_AP_CLERK', 'MDOE'),
('PS_AP_CLERK', 'BJONES'),
('PS_AP_MANAGER', 'JSMITH'),
-- PM roles
('PS_PROJECT_MANAGER', 'BJONES'),
('PS_PROJECT_COORDINATOR', 'AWILSON'),
('PS_PROJECT_COORDINATOR', 'DLEE'),
-- Vendor roles
('PS_VENDOR_MANAGER', 'BJONES'),
('PS_VENDOR_CLERK', 'MDOE'),
('PS_VENDOR_CLERK', 'AWILSON'),
-- General roles
('PS_USER', 'MDOE'),
('PS_USER', 'AWILSON'),
('PS_USER', 'DLEE'),
('PS_MANAGER', 'BJONES'),
('PS_MANAGER', 'CTAYLOR');
