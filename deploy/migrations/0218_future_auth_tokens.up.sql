CREATE TABLE future_auth_tokens (
    token_hash bytea PRIMARY KEY CHECK (octet_length(token_hash) = 32),
    user_id bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    source_auth_key_id bigint NOT NULL,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (user_id, source_auth_key_id)
);
CREATE INDEX future_auth_tokens_user_created ON future_auth_tokens(user_id, created_at DESC);

-- Voluntary logout replaces its token after deleting the authorization, in the
-- same transaction. Remote revocation only deletes, so cannot mint a new token.
CREATE FUNCTION invalidate_future_auth_on_authorization_delete() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    DELETE FROM future_auth_tokens WHERE source_auth_key_id = OLD.auth_key_id;
    RETURN OLD;
END $$;
CREATE TRIGGER future_auth_authorization_delete AFTER DELETE ON authorizations
FOR EACH ROW EXECUTE FUNCTION invalidate_future_auth_on_authorization_delete();

CREATE FUNCTION invalidate_future_auth_on_password_change() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        DELETE FROM future_auth_tokens WHERE user_id = OLD.user_id;
        RETURN OLD;
    END IF;
    IF TG_OP = 'INSERT' OR
       ROW(OLD.has_password, OLD.srp_verifier, OLD.current_algo_salt1, OLD.current_algo_salt2, OLD.current_algo_g, OLD.current_algo_p)
       IS DISTINCT FROM
       ROW(NEW.has_password, NEW.srp_verifier, NEW.current_algo_salt1, NEW.current_algo_salt2, NEW.current_algo_g, NEW.current_algo_p) THEN
        DELETE FROM future_auth_tokens WHERE user_id = NEW.user_id;
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER future_auth_password_change AFTER INSERT OR UPDATE OR DELETE ON account_passwords
FOR EACH ROW EXECUTE FUNCTION invalidate_future_auth_on_password_change();

CREATE FUNCTION invalidate_future_auth_on_user_change() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.phone IS DISTINCT FROM NEW.phone OR OLD.deleted_at IS DISTINCT FROM NEW.deleted_at THEN
        DELETE FROM future_auth_tokens WHERE user_id = NEW.id;
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER future_auth_user_change AFTER UPDATE OF phone, deleted_at ON users
FOR EACH ROW EXECUTE FUNCTION invalidate_future_auth_on_user_change();
