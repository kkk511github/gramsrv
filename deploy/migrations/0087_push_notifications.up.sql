CREATE TABLE IF NOT EXISTS push_devices (
  id bigserial PRIMARY KEY,
  user_id bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  auth_key_id bytea NOT NULL DEFAULT '\x',
  token_type integer NOT NULL,
  token text NOT NULL,
  app_sandbox boolean NOT NULL DEFAULT false,
  secret bytea NOT NULL DEFAULT '\x',
  no_muted boolean NOT NULL DEFAULT false,
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT push_devices_token_type_check CHECK (token_type > 0),
  CONSTRAINT push_devices_token_check CHECK (length(token) BETWEEN 1 AND 4096),
  CONSTRAINT push_devices_user_token_unique UNIQUE (user_id, token_type, token)
);

CREATE INDEX IF NOT EXISTS push_devices_user_idx
  ON push_devices (user_id, updated_at DESC);

CREATE TABLE IF NOT EXISTS push_notification_outbox (
  id bigserial PRIMARY KEY,
  target_user_id bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  pts integer NOT NULL,
  title text NOT NULL,
  body text NOT NULL,
  attempts integer NOT NULL DEFAULT 0,
  next_attempt_at timestamptz NOT NULL DEFAULT now(),
  locked_until timestamptz,
  last_error text NOT NULL DEFAULT '',
  delivered_device_ids bigint[] NOT NULL DEFAULT '{}',
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT push_notification_outbox_target_pts_unique UNIQUE (target_user_id, pts)
);

CREATE INDEX IF NOT EXISTS push_notification_outbox_ready_idx
  ON push_notification_outbox (next_attempt_at, id)
  WHERE locked_until IS NULL;
