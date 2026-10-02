ALTER TABLE registration_policy
    ADD COLUMN future_auth_enabled boolean NOT NULL DEFAULT true,
    ADD COLUMN future_auth_days integer NOT NULL DEFAULT 30 CHECK (future_auth_days BETWEEN 1 AND 365);
