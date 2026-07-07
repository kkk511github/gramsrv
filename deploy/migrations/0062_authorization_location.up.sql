ALTER TABLE authorizations
    ADD COLUMN IF NOT EXISTS country varchar(64) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS region varchar(128) NOT NULL DEFAULT '';
