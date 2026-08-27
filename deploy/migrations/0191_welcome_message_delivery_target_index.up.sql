CREATE INDEX welcome_message_deliveries_target_idx
    ON welcome_message_deliveries (channel_id, target_user_id, id);
-- Renumbered after the SafeLink 0186 migration series.
