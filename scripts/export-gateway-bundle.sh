#!/bin/bash
# Write an import bundle (docs/import-bundle.md) from a Lark gateway's built-in agent bridge: the four agent_* tables
# of one gateway account, and that account's credentials file. It only reads. The bundle holds the credentials in
# plain text: it is written 0600, and is yours to delete once `personal-agent-club import` has taken it.
#
#   PSQL="docker exec -i <postgres container> psql -U <user> -d <database>" \
#   CREDENTIALS=/path/to/agent-bridge/credentials.json \
#     scripts/export-gateway-bundle.sh bundle.json
#
#   PSQL         the command that runs psql against the gateway's database, reading SQL on stdin
#                (default: psql, with the usual PG* variables)
#   SCHEMA       the gateway account's schema: public for the first account (the default), ol_account_<id> for others
#   CREDENTIALS  a readable copy of that account's <data dir>/agent-bridge/credentials.json; without it the bundle
#                has no credentials and every imported account answers NO_CREDENTIAL when it is verified
#
# Stop the gateway's agent bridge (or the gateway) first when the bundle is meant to replace it, so nothing is sent
# or synced there after the export.
set -euo pipefail
if [ $# -ne 1 ]; then
  sed -n '2,17p' "$0" | sed 's/^# \{0,1\}//' >&2
  exit 2
fi
out=$1
schema=${SCHEMA:-public}
case "$schema" in
  public | ol_account_[1-9]*) ;;
  *) echo "SCHEMA must be public or ol_account_<id>" >&2; exit 2 ;;
esac
case "$schema" in *[!a-z0-9_]*) echo "SCHEMA must be public or ol_account_<id>" >&2; exit 2 ;; esac
command -v python3 >/dev/null || { echo "python3 is needed to put the bundle together" >&2; exit 1; }

umask 077
rows=$(mktemp)
trap 'rm -f "$rows"' EXIT

# shellcheck disable=SC2086
${PSQL:-psql} -X -q -A -t -v ON_ERROR_STOP=1 >"$rows" <<SQL
SET search_path TO $schema;
SELECT json_build_object(
  'accounts', (SELECT coalesce(json_agg(a), '[]'::json) FROM (
    SELECT account_id, provider, display_name, status, external_account_id, last_synced_at, last_error,
           created_at, updated_at
    FROM agent_accounts ORDER BY created_at, account_id) a),
  'sessions', (SELECT coalesce(json_agg(s), '[]'::json) FROM (
    SELECT session_key, account_id, provider, external_id, title, kind, parent_session_key, can_send, ref_json,
           rank_time_ms, on_top_rank_time_ms, unread_count, read_time_ms, is_hidden, preview_text, preview_is_mine,
           updated_at
    FROM agent_sessions ORDER BY session_key) s),
  'messages', (SELECT coalesce(json_agg(m), '[]'::json) FROM (
    SELECT session_key, external_id, position, role, blocks_json, create_time_ms
    FROM agent_messages ORDER BY session_key, position, external_id) m),
  'outbox', (SELECT coalesce(json_agg(o), '[]'::json) FROM (
    SELECT client_id, session_key, account_id, text, status, attempt_count, last_error, created_at, updated_at
    FROM agent_outbox ORDER BY created_at, client_id) o)
);
SQL

python3 - "$rows" "${CREDENTIALS:-}" "$out" <<'PY'
import json, os, sys

rows_path, credentials_path, out = sys.argv[1:4]
with open(rows_path, encoding="utf-8") as f:
    bundle = json.load(f)
accounts = {a["account_id"] for a in bundle["accounts"]}
credentials = {}
if credentials_path:
    with open(credentials_path, encoding="utf-8") as f:
        stored = json.load(f)
    credentials = {k: v for k, v in stored.items() if k in accounts and isinstance(v, str)}
bundle = {"format": "personal-agent-club/import", "version": 1, **bundle, "credentials": credentials}
fd = os.open(out, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
os.fchmod(fd, 0o600)
with os.fdopen(fd, "w", encoding="utf-8") as f:
    json.dump(bundle, f, ensure_ascii=False)
by_provider = {}
for a in bundle["accounts"]:
    by_provider[a["provider"]] = by_provider.get(a["provider"], 0) + 1
print(
    f"{out}: {len(bundle['accounts'])} account(s) {by_provider}, {len(bundle['sessions'])} session(s), "
    f"{len(bundle['messages'])} message(s), {len(bundle['outbox'])} send(s), "
    f"{len(credentials)} credential(s); mode 0600, holds credentials in plain text",
    file=sys.stderr,
)
PY
