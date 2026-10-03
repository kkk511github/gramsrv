ALTER TABLE registration_invites
    ADD COLUMN code text CHECK (code IS NULL OR code ~ '^[0-9]{5}$');
