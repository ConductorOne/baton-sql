CREATE TABLE psoprdefn (
    oprid VARCHAR(30) PRIMARY KEY,
    useridalias VARCHAR(30),
    emplid VARCHAR(11),
    emailid VARCHAR(70),
    symbolicid VARCHAR(8),
    defaultnavhp VARCHAR(30),
    rowsecclass VARCHAR(30),
    prcsprflcls VARCHAR(30),
    oprclass VARCHAR(30)
);

CREATE TABLE ps_opr_def_tbl_ap (
    oprid VARCHAR(30) PRIMARY KEY,
    origin VARCHAR(10),
    FOREIGN KEY (oprid) REFERENCES psoprdefn(oprid)
);

CREATE TABLE ps_opr_def_tbl_fs (
    oprid VARCHAR(30) PRIMARY KEY,
    FOREIGN KEY (oprid) REFERENCES psoprdefn(oprid)
);

CREATE TABLE ps_opr_def_tbl_gl (
    oprid VARCHAR(30) PRIMARY KEY,
    FOREIGN KEY (oprid) REFERENCES psoprdefn(oprid)
);

CREATE TABLE ps_opr_def_tbl_pm (
    oprid VARCHAR(30) PRIMARY KEY,
    origin VARCHAR(10),
    FOREIGN KEY (oprid) REFERENCES psoprdefn(oprid)
);

CREATE TABLE ps_opr_def_tbl_vnd (
    oprid VARCHAR(30) PRIMARY KEY,
    FOREIGN KEY (oprid) REFERENCES psoprdefn(oprid)
);

CREATE TABLE psroleuser (
    rolename VARCHAR(30) NOT NULL,
    roleuser VARCHAR(30) NOT NULL,
    PRIMARY KEY (rolename, roleuser),
    FOREIGN KEY (roleuser) REFERENCES psoprdefn(oprid)
);

-- Create indexes
CREATE INDEX idx_psoprdefn_useridalias ON psoprdefn(useridalias);
CREATE INDEX idx_psoprdefn_emplid ON psoprdefn(emplid);
CREATE INDEX idx_psoprdefn_emailid ON psoprdefn(emailid);
CREATE INDEX idx_psroleuser_roleuser ON psroleuser(roleuser);
CREATE INDEX idx_psroleuser_rolename ON psroleuser(rolename);
