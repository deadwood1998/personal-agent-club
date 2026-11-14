# Import bundles

An import bundle moves one space of the Lark gateway's built-in agent bridge into club: its paired accounts, their
sessions, messages, read state and outbox, the accounts' credentials and the browser identity, with every id kept. It
is what `personal-agent-club import` and the `club.import` method read (`internal/core/importer.go`: `ReadBundle`,
`planImport`, `Space.Import`).

## The file

One JSON object, in a file only its owner can read or write: a bundle whose mode has any group or other bit set (for
example 0644) is refused before it is read, because it holds the credentials in plain text. Make it 0600 and delete it
once the import is done; club leaves it where it is.

```json
{
  "format": "personal-agent-club/import",
  "version": 1,
  "accounts": [ ... ],
  "sessions": [ ... ],
  "messages": [ ... ],
  "outbox": [ ... ],
  "credentials": { "<account_id>": "<credential>", ... },
  "browserIdentity": { "userAgent": "", "locale": "", "timezone": "", "extraArgs": [] }
}
```

- `version` must be `1`. `format` may be left out; when present it must be `personal-agent-club/import`.
- `accounts`, `sessions`, `messages` and `outbox` are the rows of the gateway's `agent_accounts`, `agent_sessions`,
  `agent_messages` and `agent_outbox` tables, with the column names as keys. Unknown keys are ignored.
- Integer columns (`*_at`, `*_ms`, `position`, `unread_count`, `attempt_count`) may be JSON numbers, strings of digits
  (as node-postgres writes `bigint`) or `null` (0). Boolean columns may be `true`/`false`, `0`/`1`, `"t"`/`"f"` or
  `null` (false).
- `browserIdentity` is optional. When present it is checked as `club.settings.set` checks it (the presentation
  switches the Lark gateway allows) and replaces the space's.

### accounts

| Key | |
| --- | --- |
| `account_id` | required: `a` and 12 lower-case hex digits; at most once in the bundle |
| `provider` | required, e.g. `cue`, `grok_bot`, `groq`, `muse`, `dots`, `claude_duet` |
| `status` | `active`, `paused`, `error`, `removed` or `pending_pairing` (imported as `error`) |
| `display_name` | empty becomes the provider's name |
| `external_account_id`, `last_synced_at`, `last_error`, `created_at`, `updated_at` | kept as they are |

Every imported account gets assistant control `off` and quota display off, whatever it had.

### sessions

`session_key` must be `agent:<provider>:<account_id>:<external_id>` and agree with the row's `provider`,
`account_id` and `external_id`; its account must be in the bundle. `kind` is `main`, `side` or `task`. `ref_json` is an
object (or `null`): its string values are kept as the session's provider ids, others are dropped, and `avatarUrl`
becomes the session's avatar. `parent_session_key`, `can_send`, `rank_time_ms`, `on_top_rank_time_ms`, `unread_count`,
`read_time_ms`, `is_hidden`, `preview_text`, `preview_is_mine` and `updated_at` are kept, so read state comes over as
the gateway had it.

### messages

`session_key` must name a session of an account in the bundle; a message whose session row is missing (of an account
that is in the bundle) is dropped. `external_id` and `role` are required. `blocks_json` is the array of blocks; blocks of
a type club does not know are kept byte for byte. `position` and `create_time_ms` are kept.

### outbox

`client_id` is required and `session_key` must be a session of an account in the bundle; `account_id` must be that
session's. `status` is one of the gateway's:

| In the bundle | Imported as | Why |
| --- | --- | --- |
| `sent`, `unconfirmed`, `failed` | the same | already settled |
| `claimed` | `unconfirmed`, error `SEND_UNCONFIRMED` | it may have reached the provider; club never sends it again |
| `pending` | `failed`, error `IMPORTED_NOT_SENT` | the gateway never sent it, and club does not either |

So the outbox an import leaves holds nothing `pending` or `claimed`: when a serve later opens the space, its outbox
recovery finds nothing of the import's to send. A row whose `client_id` is already in the space is left alone.

### credentials

An object from account id to credential, as the gateway's vault held it (for Grok Bot, the access token or the JSON
with both tokens). Every key must be an account of the bundle. The credentials of imported accounts must be 8 to 8192
characters (after trimming); they go into the space's encrypted vault and are never printed or answered. The
credentials of skipped accounts are neither checked nor stored.

## What is skipped

- **Providers this build cannot reach.** An account whose provider has no adapter in this build (the browser providers
  `muse`, `dots` and `claude_duet`, or a name club does not know) is skipped with reason `PROVIDER_UNSUPPORTED`, with
  its sessions, messages, sends and credential.
- **Providers not selected.** With `-only-provider` (`only_providers`), accounts of other providers are skipped with
  reason `NOT_SELECTED`, the same way.

Skipped accounts are reported, and nothing of theirs is written.

## What is refused

Anything above that does not hold (an unreadable or world-readable file, another version, a bad account id or status,
a session key that does not match its row, a row of an account not in the bundle, an unknown session kind or send
status, a credential for an account not in the bundle or of the wrong length, a browser identity the gateway would
refuse) refuses the whole bundle with `invalid` (`INVALID_AGENT_BRIDGE_INPUT`): nothing is written. An account whose
remote account (`provider` and `external_account_id`) is already paired in the space under another account id is a
`conflict` (`ACCOUNT_ALREADY_PAIRED`), also before anything is written.

## Writing, idempotency, verification

The credentials go into the vault first, then every row in one transaction: accounts and sessions replace the rows
with the same account id or session key, messages are upserted by session key and external id, and sends whose client
id is taken are left alone. Importing the same bundle again leaves the space as the first import did; it also resets
what was changed in club since on those accounts and sessions (status, name, assistant control, read state) to the
bundle's.

Nothing is sent and nothing is synced. After writing, each imported account is verified read-only with its provider,
one request each (Groq's model list, Grok Bot's `GetMe`, Cue's `agent.list`). The answer says per account whether it
verified and, if not, why: the provider's code (`AUTH_FAILED`, `RATE_LIMITED`, …), `NO_CREDENTIAL`,
`ACCOUNT_MISMATCH` (the credential belongs to another remote account) or `REMOVED`. Verification changes nothing in
the space; the next sync of a serve sets the account's status as usual.

A dry run (`-dry-run`, `dry_run`) reads and checks the bundle against the space's existing accounts and answers what
would be imported (counts from the bundle, nothing verified). It writes nothing.

## Running it

```sh
chmod 600 bundle.json
personal-agent-club import -data ~/.personal-agent-club -dry-run bundle.json   # works while serve runs
personal-agent-club import -data ~/.personal-agent-club bundle.json            # stop serve first
personal-agent-club import -only-provider groq,cue -json bundle.json
```

The import owns the data directory while it runs: it refuses to start while a serve or the Persona extension holds the
directory's lock. It opens the space without outbox recovery, follow-ups, transcript watches or polling. `-space` picks
the space (default `default`), `-key-file` the key (default `<data>/key`, created when missing). It prints a line per
account (id, provider, name, imported or skipped and why, sessions, messages, latest message time, verified or why
not) and the totals; `-json` prints `club.import`'s answer instead. Exit codes: 0 done, 1 error (nothing written when
the bundle is refused), 2 usage, 3 imported but at least one imported account did not verify.

Over the protocol, `club.import {path, only_providers?, dry_run?}` does the same in a running club (the path is on
club's machine); it needs an admin token over TCP.
