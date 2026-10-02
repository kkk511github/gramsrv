DROP TRIGGER IF EXISTS registration_password_completed ON account_passwords;
DROP FUNCTION IF EXISTS complete_registration_password();
DROP TABLE IF EXISTS registration_password_pending;
ALTER TABLE registration_policy DROP COLUMN registration_password_required;
