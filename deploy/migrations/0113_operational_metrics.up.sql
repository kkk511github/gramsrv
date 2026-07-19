CREATE TABLE IF NOT EXISTS operational_metric_minutes (
    bucket_at timestamptz PRIMARY KEY,
    rpc_requests bigint NOT NULL DEFAULT 0,
    push_delivered bigint NOT NULL DEFAULT 0,
    push_failed bigint NOT NULL DEFAULT 0,
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT operational_metric_minutes_nonnegative_check CHECK (
        rpc_requests >= 0 AND push_delivered >= 0 AND push_failed >= 0
    )
);

CREATE INDEX IF NOT EXISTS private_messages_created_at_runtime_metrics_idx
    ON private_messages (created_at);

CREATE INDEX IF NOT EXISTS channel_messages_created_at_runtime_metrics_idx
    ON channel_messages (created_at);

