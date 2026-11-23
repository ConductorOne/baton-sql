#!/usr/bin/env bash
# Smoke test for the event feed of the shipped examples against a freshly loaded test database.
#
# Usage: test/event_feed_smoke.sh <postgres|mysql|sqlserver|vertica|oracle>
#
# Environment:
#   BATON_SQL          path to the connector binary (default: ./baton-sql)
#   BATON_CONFIG_PATH  example config for the engine, plus the DB_* variables it reads
#   SQL_CMD            command that reads one SQL statement on stdin and prints the result rows.
#                      Required for engines whose example provisions role grants.
#   CHANGED_USER       optional user whose row was updated before running; the feed must report it.
set -euo pipefail

ENGINE="${1:?usage: $0 <postgres|mysql|sqlserver|vertica|oracle>}"
BATON_SQL="${BATON_SQL:-./baton-sql}"
WORKDIR="$(mktemp -d)"
trap 'rm -rf "$WORKDIR"' EXIT

# Seeded soft-deleted assignment, and a live assignment to revoke and grant again.
SEEDED_USER="disabled.user"
SEEDED_ROLE="reader"
LIVE_USER="service.acct"
LIVE_ROLE="user"

case "$ENGINE" in
  postgres | mysql | vertica)
    HAS_GRANTS_FEED=1
    deleted_at_sql="SELECT CASE WHEN ur.deleted_at IS NULL THEN 'live' ELSE 'deleted' END FROM user_roles ur JOIN users u ON u.id = ur.user_id JOIN roles r ON r.id = ur.role_id WHERE u.username = '$LIVE_USER' AND r.role_name = '$LIVE_ROLE'"
    ;;
  sqlserver)
    HAS_GRANTS_FEED=1
    deleted_at_sql="SELECT CASE WHEN ur.IsDeleted = 0 AND ur.DeletedAt IS NULL THEN 'live' ELSE 'deleted' END FROM UserRoles ur JOIN Users u ON u.UserID = ur.UserID JOIN Roles r ON r.RoleID = ur.RoleID WHERE u.Username = '$LIVE_USER' AND r.RoleName = '$LIVE_ROLE'"
    ;;
  oracle)
    HAS_GRANTS_FEED=0
    ;;
  *)
    echo "unknown engine: $ENGINE" >&2
    exit 2
    ;;
esac

case "$ENGINE" in
  postgres | sqlserver) HAS_ROLE_PROVISIONING=1 ;;
  *) HAS_ROLE_PROVISIONING=0 ;;
esac

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

run_feed() {
  local out="$WORKDIR/feed.json"
  "$BATON_SQL" --event-feed=true --event-feed-id=sql_event_feed --log-level=error >"$out" ||
    fail "event feed exited with $?"
  echo "$out"
}

has_event() {
  local file="$1" kind="$2" role="$3" user="$4"
  grep "\"$kind\"" "$file" | grep "\"resource\":\"$role\"" | grep -q "\"resourceType\":\"user\", \"resource\":\"$user\""
}

run_sync() {
  local c1z="$WORKDIR/sync.c1z"
  rm -f "$c1z"
  "$BATON_SQL" -f "$c1z" --log-level=error >/dev/null 2>&1 || fail "full sync exited with $?"
  echo "$c1z"
}

synced_grant() {
  baton grants -f "$1" -o json 2>/dev/null | grep -q "\"role:$LIVE_ROLE:member:user:$LIVE_USER\""
}

db_state() {
  echo "$deleted_at_sql" | $SQL_CMD | awk 'NF { print $1; exit }'
}

echo "== feed runs and reports seeded changes"
feed="$(run_feed)"
grep -q '"resourceChangeEvent"' "$feed" || fail "no resource change events"
if [ -n "${CHANGED_USER:-}" ]; then
  grep '"resourceChangeEvent"' "$feed" | grep -q "\"resourceType\":\"user\", \"resource\":\"$CHANGED_USER\"" ||
    fail "no resource change event for $CHANGED_USER"
fi
if [ "$HAS_GRANTS_FEED" = 1 ]; then
  has_event "$feed" createRevokeEvent "$SEEDED_ROLE" "$SEEDED_USER" || fail "no revoke event for seeded $SEEDED_USER/$SEEDED_ROLE"
  has_event "$feed" createGrantEvent "$SEEDED_ROLE" "$SEEDED_USER" && fail "grant event for soft-deleted $SEEDED_USER/$SEEDED_ROLE"
  c1z="$(run_sync)"
  baton grants -f "$c1z" -o json 2>/dev/null | grep -q "\"role:$SEEDED_ROLE:member:user:$SEEDED_USER\"" &&
    fail "full sync returned soft-deleted grant $SEEDED_USER/$SEEDED_ROLE"
fi

if [ "$HAS_ROLE_PROVISIONING" = 0 ]; then
  echo "PASS ($ENGINE: feed only)"
  exit 0
fi
: "${SQL_CMD:?SQL_CMD is required for $ENGINE}"

echo "== revoke through the connector soft-deletes"
c1z="$(run_sync)"
synced_grant "$c1z" || fail "expected live grant $LIVE_USER/$LIVE_ROLE before revoke"
"$BATON_SQL" -f "$c1z" --revoke-grant="role:$LIVE_ROLE:member:user:$LIVE_USER" --log-level=error >/dev/null 2>&1 ||
  fail "revoke failed"
[ "$(db_state)" = "deleted" ] || fail "row for $LIVE_USER/$LIVE_ROLE is not soft-deleted"
"$BATON_SQL" -f "$c1z" --revoke-grant="role:$LIVE_ROLE:member:user:$LIVE_USER" --log-level=error >/dev/null 2>&1 ||
  fail "revoking an already revoked grant failed"
feed="$(run_feed)"
has_event "$feed" createRevokeEvent "$LIVE_ROLE" "$LIVE_USER" || fail "no revoke event after revoke"
has_event "$feed" createGrantEvent "$LIVE_ROLE" "$LIVE_USER" && fail "grant event after revoke"

echo "== full sync keeps it revoked"
c1z="$(run_sync)"
synced_grant "$c1z" && fail "full sync restored revoked grant"

echo "== grant through the connector revives the row"
"$BATON_SQL" -f "$c1z" --grant-entitlement="role:$LIVE_ROLE:member" --grant-principal="$LIVE_USER" \
  --grant-principal-type=user --log-level=error >/dev/null 2>&1 || fail "grant failed"
[ "$(db_state)" = "live" ] || fail "row for $LIVE_USER/$LIVE_ROLE was not revived"
"$BATON_SQL" -f "$c1z" --grant-entitlement="role:$LIVE_ROLE:member" --grant-principal="$LIVE_USER" \
  --grant-principal-type=user --log-level=error >/dev/null 2>&1 || fail "granting an already live grant failed"
feed="$(run_feed)"
has_event "$feed" createGrantEvent "$LIVE_ROLE" "$LIVE_USER" || fail "no grant event after grant"
has_event "$feed" createRevokeEvent "$LIVE_ROLE" "$LIVE_USER" && fail "revoke event after grant"
c1z="$(run_sync)"
synced_grant "$c1z" || fail "full sync is missing the granted $LIVE_USER/$LIVE_ROLE"

echo "PASS ($ENGINE)"
