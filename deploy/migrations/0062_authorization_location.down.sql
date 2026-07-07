ALTER TABLE authorizations
    DROP COLUMN IF EXISTS region,
    DROP COLUMN IF EXISTS country;
