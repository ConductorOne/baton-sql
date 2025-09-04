-- Script to generate 1 million test records for PeopleSoft tables
-- This script uses PostgreSQL's generate_series() function for efficient bulk data generation
-- Fixed to respect VARCHAR length constraints and handle existing data

-- First, let's create some helper arrays for realistic data generation
DO $$
DECLARE
    batch_size INTEGER := 10000;
    total_users INTEGER := 1000000;
    current_batch INTEGER := 0;
    
    -- Arrays for generating realistic data
    first_names TEXT[] := ARRAY['John', 'Mary', 'Bob', 'Alice', 'Charlie', 'Diana', 'David', 'Sarah', 'Michael', 'Jennifer', 'Robert', 'Lisa', 'William', 'Karen', 'James', 'Nancy', 'Chris', 'Betty', 'Daniel', 'Helen', 'Paul', 'Sandra', 'Mark', 'Donna', 'Donald', 'Carol', 'George', 'Ruth', 'Ken', 'Sharon', 'Steven', 'Michelle', 'Edward', 'Laura', 'Brian', 'Sarah', 'Ronald', 'Kim', 'Anthony', 'Deborah', 'Kevin', 'Dorothy', 'Jason', 'Amy', 'Matt', 'Angela', 'Gary', 'Brenda', 'Tim', 'Emma'];
    
    last_names TEXT[] := ARRAY['Smith', 'Johnson', 'Brown', 'Jones', 'Garcia', 'Miller', 'Davis', 'Rodriguez', 'Martinez', 'Lopez', 'Wilson', 'Anderson', 'Thomas', 'Taylor', 'Moore', 'Jackson', 'Martin', 'Lee', 'Perez', 'Thompson', 'White', 'Harris', 'Clark', 'Lewis', 'Robinson', 'Walker', 'Young', 'Allen', 'King', 'Wright', 'Scott', 'Torres', 'Nguyen', 'Hill', 'Flores', 'Green', 'Adams', 'Nelson', 'Baker', 'Hall', 'Rivera', 'Campbell', 'Mitchell', 'Carter', 'Roberts'];
    
    homepages TEXT[] := ARRAY['HOME1', 'HOME2', 'HOME3', 'HOME4', 'HOME5'];
    
    rowsec_classes TEXT[] := ARRAY['ROWSEC1', 'ROWSEC2', 'ROWSEC3', 'ROWSEC4', 'ROWSEC5'];
    
    proc_classes TEXT[] := ARRAY['PROC1', 'PROC2', 'PROC3', 'PROC4', 'PROC5'];
    
    opr_classes TEXT[] := ARRAY['ADMIN', 'USER', 'MANAGER', 'SUPER', 'ANALYST'];
    
    -- Origin values that fit in VARCHAR(8)
    origins_short TEXT[] := ARRAY['INTERNAL', 'EXTERNAL', 'CONTRACT', 'VENDOR'];
    
    user_id INTEGER;
    
BEGIN
    RAISE NOTICE 'Starting bulk data generation for % users', total_users;
    
    -- Clear existing data to avoid conflicts (optional - uncomment if you want to start fresh)
    TRUNCATE ps_opr_def_tbl_ap, ps_opr_def_tbl_fs, ps_opr_def_tbl_gl, ps_opr_def_tbl_pm, ps_opr_def_tbl_vnd, psroleuser CASCADE;
    DELETE FROM psoprdefn WHERE oprid LIKE 'USR%';
    
    -- Generate users in batches to avoid memory issues
    WHILE current_batch * batch_size < total_users LOOP
        RAISE NOTICE 'Processing batch %: records % to %', 
            current_batch + 1, 
            current_batch * batch_size + 1, 
            LEAST((current_batch + 1) * batch_size, total_users);
        
        -- Insert batch of users into PSOPRDEFN
        INSERT INTO psoprdefn (oprid, useridalias, emplid, emailid, symbolicid, defaultnavhp, rowsecclass, prcsprflcls, oprclass)
        SELECT 
            -- OPRID: Use 7 digits to accommodate 1 million users (USR0000001 to USR1000000)
            'USR' || LPAD((current_batch * batch_size + gs)::TEXT, 7, '0') as oprid,
            -- USERIDALIAS: Use first initial + last name to keep it shorter
            LOWER(LEFT(first_names[1 + (gs % array_length(first_names, 1))], 1)) || 
            LOWER(last_names[1 + ((gs * 7) % array_length(last_names, 1))]) as useridalias,
            -- EMPLID: Use 7 digits to accommodate 1 million employees
            'EMP' || LPAD((current_batch * batch_size + gs)::TEXT, 7, '0') as emplid,
            -- EMAIL: Use the same pattern as useridalias
            LOWER(LEFT(first_names[1 + (gs % array_length(first_names, 1))], 1)) || 
            LOWER(last_names[1 + ((gs * 7) % array_length(last_names, 1))]) || '@co.com' as emailid,
            -- SYMBOLICID: SYM + 4 digits to fit in VARCHAR(8), cycle through 1-9999
            'SYM' || LPAD(((current_batch * batch_size + gs - 1) % 9999 + 1)::TEXT, 4, '0') as symbolicid,
            homepages[1 + (gs % array_length(homepages, 1))] as defaultnavhp,
            rowsec_classes[1 + (gs % array_length(rowsec_classes, 1))] as rowsecclass,
            proc_classes[1 + (gs % array_length(proc_classes, 1))] as prcsprflcls,
            opr_classes[1 + (gs % array_length(opr_classes, 1))] as oprclass
        FROM generate_series(1, LEAST(batch_size, total_users - current_batch * batch_size)) as gs
        ON CONFLICT (oprid) DO NOTHING;
        
        current_batch := current_batch + 1;
    END LOOP;
    
    RAISE NOTICE 'Completed PSOPRDEFN table with % records', total_users;
    
    -- Generate data for PS_OPR_DEF_TBL_AP (about 30% of users)
    -- Fixed: Use shorter origin values that fit in VARCHAR(8) and handle conflicts
    INSERT INTO ps_opr_def_tbl_ap (oprid, origin)
    SELECT 
        oprid,
        origins_short[1 + (random() * (array_length(origins_short, 1) - 1))::int] as origin
    FROM psoprdefn 
    WHERE random() < 0.3
    ON CONFLICT (oprid) DO NOTHING;

    RAISE NOTICE 'Completed PS_OPR_DEF_TBL_AP table';

    -- Generate data for PS_OPR_DEF_TBL_FS (about 25% of users)
    INSERT INTO ps_opr_def_tbl_fs (oprid)
    SELECT oprid
    FROM psoprdefn 
    WHERE random() < 0.25
    ON CONFLICT (oprid) DO NOTHING;

    RAISE NOTICE 'Completed PS_OPR_DEF_TBL_FS table';

    -- Generate data for PS_OPR_DEF_TBL_GL (about 20% of users)
    INSERT INTO ps_opr_def_tbl_gl (oprid)
    SELECT oprid
    FROM psoprdefn 
    WHERE random() < 0.20
    ON CONFLICT (oprid) DO NOTHING;

    RAISE NOTICE 'Completed PS_OPR_DEF_TBL_GL table';

    -- Generate data for PS_OPR_DEF_TBL_PM (about 35% of users)
    -- Fixed: Use shorter origin values that fit in VARCHAR(8) and handle conflicts
    INSERT INTO ps_opr_def_tbl_pm (oprid, origin)
    SELECT 
        oprid,
        origins_short[1 + (random() * (array_length(origins_short, 1) - 1))::int] as origin
    FROM psoprdefn 
    WHERE random() < 0.35
    ON CONFLICT (oprid) DO NOTHING;

    RAISE NOTICE 'Completed PS_OPR_DEF_TBL_PM table';

    -- Generate data for PS_OPR_DEF_TBL_VND (about 15% of users)
    INSERT INTO ps_opr_def_tbl_vnd (oprid)
    SELECT oprid
    FROM psoprdefn 
    WHERE random() < 0.15
    ON CONFLICT (oprid) DO NOTHING;

    RAISE NOTICE 'Completed PS_OPR_DEF_TBL_VND table';
END $$;

-- Generate role assignments (each user gets 1-5 roles randomly)
DO $$
DECLARE
    role_names TEXT[] := ARRAY[
        'PS_ADMIN', 'PS_USER', 'PS_MANAGER', 'PS_SUPERVISOR',
        'PS_FINANCIAL_ANALYST', 'PS_FINANCIAL_MANAGER', 'PS_FINANCIAL_CLERK',
        'PS_AP_CLERK', 'PS_AP_MANAGER', 'PS_AP_SUPERVISOR',
        'PS_AR_CLERK', 'PS_AR_MANAGER', 'PS_AR_SUPERVISOR',
        'PS_PROJECT_MANAGER', 'PS_PROJECT_COORDINATOR', 'PS_PROJECT_ANALYST',
        'PS_VENDOR_MANAGER', 'PS_VENDOR_CLERK', 'PS_VENDOR_ANALYST',
        'PS_HR_MANAGER', 'PS_HR_SPECIALIST', 'PS_HR_CLERK',
        'PS_IT_ADMIN', 'PS_IT_SUPPORT', 'PS_IT_ANALYST',
        'PS_SALES_MANAGER', 'PS_SALES_REP', 'PS_SALES_SUPPORT',
        'PS_MARKETING_MANAGER', 'PS_MARKETING_SPECIALIST'
    ];
    user_rec RECORD;
    num_roles INTEGER;
    role_idx INTEGER;
    role_name TEXT;
    processed_count INTEGER := 0;
BEGIN
    RAISE NOTICE 'Starting role assignment generation...';
    
    -- For each user, assign 1-5 random roles
    FOR user_rec IN (SELECT oprid FROM psoprdefn ORDER BY oprid) LOOP
        -- Each user gets between 1 and 5 roles
        num_roles := 1 + (random() * 4)::int;
        
        FOR i IN 1..num_roles LOOP
            role_idx := 1 + (random() * (array_length(role_names, 1) - 1))::int;
            role_name := role_names[role_idx];
            
            -- Insert role assignment (ignore duplicates)
            INSERT INTO psroleuser (rolename, roleuser) 
            VALUES (role_name, user_rec.oprid)
            ON CONFLICT DO NOTHING;
        END LOOP;
        
        processed_count := processed_count + 1;
        
        -- Progress indicator every 50,000 users
        IF processed_count % 50000 = 0 THEN
            RAISE NOTICE 'Processed % users for role assignments', processed_count;
        END IF;
    END LOOP;
    
    RAISE NOTICE 'Completed role assignments';
END $$;

-- Create indexes for better performance
CREATE INDEX IF NOT EXISTS idx_psoprdefn_oprid ON psoprdefn(oprid);
CREATE INDEX IF NOT EXISTS idx_psoprdefn_useridalias ON psoprdefn(useridalias);
CREATE INDEX IF NOT EXISTS idx_psoprdefn_emplid ON psoprdefn(emplid);
CREATE INDEX IF NOT EXISTS idx_psroleuser_rolename ON psroleuser(rolename);
CREATE INDEX IF NOT EXISTS idx_psroleuser_roleuser ON psroleuser(roleuser);
CREATE INDEX IF NOT EXISTS idx_psroleuser_composite ON psroleuser(rolename, roleuser);

-- Final statistics
DO $$
BEGIN
    RAISE NOTICE '=== DATA GENERATION COMPLETE ===';
    RAISE NOTICE 'PSOPRDEFN records: %', (SELECT COUNT(*) FROM psoprdefn);
    RAISE NOTICE 'PS_OPR_DEF_TBL_AP records: %', (SELECT COUNT(*) FROM ps_opr_def_tbl_ap);
    RAISE NOTICE 'PS_OPR_DEF_TBL_FS records: %', (SELECT COUNT(*) FROM ps_opr_def_tbl_fs);
    RAISE NOTICE 'PS_OPR_DEF_TBL_GL records: %', (SELECT COUNT(*) FROM ps_opr_def_tbl_gl);
    RAISE NOTICE 'PS_OPR_DEF_TBL_PM records: %', (SELECT COUNT(*) FROM ps_opr_def_tbl_pm);
    RAISE NOTICE 'PS_OPR_DEF_TBL_VND records: %', (SELECT COUNT(*) FROM ps_opr_def_tbl_vnd);
    RAISE NOTICE 'PSROLEUSER records: %', (SELECT COUNT(*) FROM psroleuser);
    RAISE NOTICE 'Average roles per user: %', 
        ROUND((SELECT COUNT(*)::numeric FROM psroleuser) / (SELECT COUNT(*)::numeric FROM psoprdefn), 2);
END $$;