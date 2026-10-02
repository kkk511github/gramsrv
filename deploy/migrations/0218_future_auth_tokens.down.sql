DROP TRIGGER IF EXISTS future_auth_user_change ON users;
DROP FUNCTION IF EXISTS invalidate_future_auth_on_user_change();
DROP TRIGGER IF EXISTS future_auth_password_change ON account_passwords;
DROP FUNCTION IF EXISTS invalidate_future_auth_on_password_change();
DROP TRIGGER IF EXISTS future_auth_authorization_delete ON authorizations;
DROP FUNCTION IF EXISTS invalidate_future_auth_on_authorization_delete();
DROP TABLE IF EXISTS future_auth_tokens;
