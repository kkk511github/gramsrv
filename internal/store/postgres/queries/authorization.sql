-- name: UpsertAuthorization :exec
INSERT INTO authorizations (auth_key_id, user_id, layer, device_model, platform, system_version, api_id, app_version, ip, country, region)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
ON CONFLICT (auth_key_id) DO UPDATE SET
  user_id = EXCLUDED.user_id,
  layer = EXCLUDED.layer,
  device_model = EXCLUDED.device_model,
  platform = EXCLUDED.platform,
  system_version = EXCLUDED.system_version,
  api_id = EXCLUDED.api_id,
  app_version = EXCLUDED.app_version,
  ip = CASE WHEN EXCLUDED.ip <> '' THEN EXCLUDED.ip ELSE authorizations.ip END,
  country = CASE WHEN EXCLUDED.country <> '' THEN EXCLUDED.country ELSE authorizations.country END,
  region = CASE WHEN EXCLUDED.region <> '' THEN EXCLUDED.region ELSE authorizations.region END,
  active_at = now();

-- name: GetAuthorizationByAuthKey :one
SELECT auth_key_id, user_id, hash, layer, device_model, platform, system_version, api_id, app_version, ip, country, region, created_at, active_at, password_pending
FROM authorizations
WHERE auth_key_id = $1;

-- name: ListAuthorizationsByUser :many
SELECT auth_key_id, user_id, hash, layer, device_model, platform, system_version, api_id, app_version, ip, country, region, created_at, active_at, password_pending
FROM authorizations
WHERE user_id = $1
ORDER BY active_at DESC, auth_key_id DESC;

-- name: DeleteAuthorization :exec
DELETE FROM authorizations WHERE auth_key_id = $1;
