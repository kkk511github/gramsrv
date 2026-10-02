#!/bin/sh
set -eu
cd /home/safelink-chat
release=${1:?release directory required}
case "$release" in /home/safelink-chat/releases/login-policy-*) ;; *) exit 2 ;; esac
cd "$release"
sha256sum -c SHA256SUMS
cd /home/safelink-chat
dc() { docker compose -f compose.yaml -f safelink-admin87-compose.yaml "$@"; }
dc config --quiet
stamp=$(date -u +%Y%m%dT%H%M%SZ)
backup="/home/safelink-chat/backups/login-policy-$stamp"
umask 077
mkdir -p "$backup"
cp -p safelink-server "$backup/safelink-server"
cp -p admin/safelink-admin "$backup/safelink-admin"
cp -p server.env admin.env compose.yaml safelink-admin87-compose.yaml "$backup/"
docker exec safelink-chat-postgres-1 pg_dump -U safelink -d safelink -Fc > "$backup/database.dump"
test -s "$backup/database.dump"
docker image tag safelink-chat:local "safelink-chat:rollback-login-$stamp"
docker image tag safelink-admin:local "safelink-admin:rollback-login-$stamp"
rollback() {
  rc=$?
  trap - EXIT
  if [ "$rc" -ne 0 ]; then
    install -m 0555 "$backup/safelink-server" safelink-server
    install -m 0555 "$backup/safelink-admin" admin/safelink-admin
    docker image tag "safelink-chat:rollback-login-$stamp" safelink-chat:local
    docker image tag "safelink-admin:rollback-login-$stamp" safelink-admin:local
    dc up -d --no-deps --force-recreate server admin
    printf 'ROLLBACK_STARTED backup=%s (additive migrations retained)\n' "$backup" >&2
  fi
  exit "$rc"
}
trap rollback EXIT
install -m 0555 "$release/safelink-server" safelink-server
install -m 0555 "$release/safelink-admin" admin/safelink-admin
dc build server
dc build admin
dc up -d --no-deps --force-recreate server
healthy() {
  service=$1
  i=0
  until [ "$(docker inspect -f '{{.State.Health.Status}}' "safelink-chat-$service-1")" = healthy ]; do
    i=$((i+1))
    [ "$i" -lt 90 ] || return 1
    sleep 2
  done
}
healthy server
dc up -d --no-deps --force-recreate admin
healthy admin
curl --fail --silent --show-error http://127.0.0.1:2401/healthz > /dev/null
python3 "$release/verify-login-policy.py"
printf 'DEPLOY_OK backup=%s release=%s\n' "$backup" "$release"
