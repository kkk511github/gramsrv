CREATE OR REPLACE FUNCTION public.telesrv_bump_read_model_version(p_model text, p_owner_user_id bigint, p_peer_type text DEFAULT ''::text, p_peer_id bigint DEFAULT 0) RETURNS void
    LANGUAGE plpgsql
    AS $$
DECLARE
    next_version BIGINT;
    next_hash BIGINT;
BEGIN
    INSERT INTO read_model_versions (model, owner_user_id, peer_type, peer_id, version, hash, updated_at)
    VALUES (
        p_model,
        COALESCE(p_owner_user_id, 0),
        COALESCE(p_peer_type, ''),
        COALESCE(p_peer_id, 0),
        1,
        telesrv_random_read_model_hash(),
        now()
    )
    ON CONFLICT (model, owner_user_id, peer_type, peer_id) DO UPDATE SET
        version = read_model_versions.version + 1,
        hash = telesrv_random_read_model_hash(),
        updated_at = EXCLUDED.updated_at
    RETURNING version, hash INTO next_version, next_hash;

    PERFORM pg_notify(
        'safelink_read_model_changed',
        json_build_object(
            'model', p_model,
            'owner_user_id', COALESCE(p_owner_user_id, 0),
            'peer_type', COALESCE(p_peer_type, ''),
            'peer_id', COALESCE(p_peer_id, 0),
            'version', next_version,
            'hash', next_hash
        )::text
    );
END;
$$;

CREATE OR REPLACE FUNCTION public.telesrv_notify_channel_changed() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    changed_id BIGINT;
BEGIN
    IF TG_OP = 'DELETE' THEN
        changed_id := OLD.id;
    ELSE
        changed_id := NEW.id;
    END IF;
    PERFORM pg_notify('safelink_channel_changed', changed_id::text);
    RETURN NULL;
END;
$$;
