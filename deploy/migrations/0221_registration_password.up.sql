ALTER TABLE registration_policy ADD COLUMN registration_password_required boolean NOT NULL DEFAULT false;

CREATE TABLE registration_password_pending (
    user_id bigint PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now()
);

-- Complete only after a usable password has been saved. Removing it later
-- does not turn an existing account back into an incomplete registration.
CREATE FUNCTION complete_registration_password() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.has_password AND NEW.email_unconfirmed_pattern = '' THEN
        DELETE FROM registration_password_pending WHERE user_id = NEW.user_id;
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER registration_password_completed AFTER INSERT OR UPDATE ON account_passwords
    FOR EACH ROW EXECUTE FUNCTION complete_registration_password();
