CREATE TABLE registration_policy (
    singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton),
    invite_required boolean NOT NULL DEFAULT false
);
INSERT INTO registration_policy(singleton) VALUES(true);
CREATE TABLE registration_invites (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    code_hash bytea NOT NULL UNIQUE CHECK(octet_length(code_hash)=32),
    prefix text NOT NULL,
    max_uses integer NOT NULL CHECK(max_uses BETWEEN 1 AND 10000),
    used_count integer NOT NULL DEFAULT 0 CHECK(used_count>=0 AND used_count<=max_uses),
    disabled boolean NOT NULL DEFAULT false,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE registration_invite_uses (
    user_id bigint PRIMARY KEY REFERENCES users(id),
    invite_id bigint NOT NULL REFERENCES registration_invites(id),
    created_at timestamptz NOT NULL DEFAULT now()
);
