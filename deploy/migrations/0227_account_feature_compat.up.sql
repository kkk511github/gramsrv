CREATE TABLE account_profile_tabs (
    user_id bigint PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    tab text NOT NULL CHECK (tab IN ('posts', 'gifts', 'media', 'files', 'music', 'voice', 'links', 'gifs')),
    updated_at timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE business_connected_bots ADD COLUMN confirmed_at timestamptz;
