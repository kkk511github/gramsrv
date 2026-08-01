-- SafeLink renumbered the retired upstream 0163 migration after the existing
-- 0168 migration. Production databases that already applied version 169 are
-- unaffected; new databases retain the version slot without rewriting data.
-- The original migration attempted to repair development-only message-box
-- projections and could abort server startup when an aggregate no longer had a
-- live owner box. Project policy forbids migrations for unpublished internal
-- shapes. Keep the version slot so a database at version 168 can advance, but
-- never inspect or mutate gift/message/PTS/outbox state here.
SELECT 1;
