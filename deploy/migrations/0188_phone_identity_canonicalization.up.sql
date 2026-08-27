-- The country-aware data rewrite is executed transactionally by
-- postgres.canonicalizeStoredPhoneIdentities before this durable version marker.
-- Keeping it in Go avoids incorrect SQL-only stripping of significant leading
-- zeroes (for example Italian fixed-line numbers).
SELECT 1;
-- Renumbered after the SafeLink 0186 migration series.
