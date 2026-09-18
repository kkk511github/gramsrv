\set ON_ERROR_STOP on

-- main and v2 intentionally have independent migration histories. Keep them
-- in separate PostgreSQL databases even when they share the local container.
SELECT 'CREATE DATABASE safelink_main OWNER safelink'
WHERE NOT EXISTS (SELECT 1 FROM pg_database WHERE datname = 'safelink_main')
\gexec

SELECT 'CREATE DATABASE safelink_v2 OWNER safelink'
WHERE NOT EXISTS (SELECT 1 FROM pg_database WHERE datname = 'safelink_v2')
\gexec
