# personal-agent-club

Personal Agent Club ("club") keeps your personal agents in one place: accounts you pair at agent services, their
sessions and messages, an outbox for what you send them, read state and search. It is one Go program that runs in
either of two ways:

- **As a Persona extension.** A Persona hub starts the program with no arguments and talks to it over the extension
  protocol on stdin and stdout. It contributes the agent source `pac`, so its sessions show up on Persona's Agents
  page (session ids `pac:<sessionKey>`).
- **Standalone, with `serve`.** The program serves the same protocol over HTTP on a unix socket (and, with tokens, on
  TCP), for the Lark gateway's agent bridge or any other client.

Both shells call the same core and answer the same protocol.

## Status

**Skeleton.** The core, both shells and the protocol are in place and tested; storage and the credential vault are in
memory only, and no real provider is included yet. The only provider is `echo`, an in-memory one for tests and demos,
off unless asked for. See the roadmap below.

License: not decided yet. Until a license is added, no rights beyond viewing the source are granted.

## Running

```sh
make build                     # bin/personal-agent-club
bin/personal-agent-club version
```

### Standalone

```sh
bin/personal-agent-club serve -demo-echo -socket /tmp/club/club.sock
```

By default `serve` listens only on a unix socket (`<data>/club.sock`, data directory `~/.personal-agent-club`), created
with mode 0600; whoever can open the socket is trusted. To listen on TCP as well, give `-listen` and a tokens file;
without tokens, `serve` refuses to start:

```sh
bin/personal-agent-club serve -listen 127.0.0.1:7655 -tokens tokens.json
```

```json
{"read": ["<read token>"], "write": ["<write token>"], "admin": ["<admin token>"]}
```

Tokens are at least 32 characters and sent as `Authorization: Bearer <token>`. Levels:

| Level | May call |
| --- | --- |
| `read` | `agentsrc.status`, `sessions`, `messages`, `search`, the event stream |
| `write` | also `agentsrc.send`, `delete_message`, `mark_read` |
| `admin` | also every `club.*` method |

`mark_read` needs `write` because it changes unread state, as with the Lark gateway's read-only agent token.

Try it with curl:

```sh
S=/tmp/club/club.sock
curl -s --unix-socket $S http://club/v1/spaces/default/rpc -d '{"method":"club.info"}'
curl -s --unix-socket $S http://club/v1/spaces/default/rpc \
  -d '{"method":"club.accounts.add","params":{"provider":"echo","credential":"echo-demo-credential"}}'
curl -s --unix-socket $S http://club/v1/spaces/default/rpc -d '{"method":"agentsrc.sessions"}'
curl -sN --unix-socket $S http://club/v1/spaces/default/events
```

### As a Persona extension

`persona-extension.json` is the manifest: extension id `personal-agent-club`, one `agent_sources` contribution `pac`,
`activation_events: on_startup`. `make dist` builds the four binaries it names under `bin/`. The configuration switch
`demo_echo` adds the echo provider.

## Protocol

Calls are `{method, params}`; results and errors follow Persona's agent source protocol field for field (results in
camelCase, parameters in snake_case), so the extension shell passes them through unchanged.

| Method | Params | Result |
| --- | --- | --- |
| `agentsrc.status` | — | `{enabled, setup?, providers, accounts}` |
| `agentsrc.sessions` | — | `{sessions: [...]}` |
| `agentsrc.messages` | `session_key`, `cursor?`, `limit?` | `{session?, messages, hasMore, nextCursor?}` |
| `agentsrc.send` | `session_key`, `client_id` (UUID), `text` | the message as queued, with `local {clientId, status, error?}` |
| `agentsrc.delete_message` | `session_key`, `client_id` | `{}`: cancels a send that has not gone out |
| `agentsrc.mark_read` | `session_key` | `{}` |
| `agentsrc.search` | `q` | `{hits: [...]}` |
| `club.info` | — | `{version, protocolVersion, providers, browser}` |
| `club.accounts.add` | `provider`, `credential`, `display_name?` | the account |
| `club.accounts.remove` | `account_id`, `delete_data?` | `{}` |
| `club.accounts.sync` | `account_id` | the account after one sync round |

Error codes: `not_found`, `invalid`, `conflict`, `unsupported`; other errors carry only a message. Over HTTP they come
as `{"error": {"code", "message"}}` with 404, 400, 409, 501 (502 for the rest). The other `club.*` methods of the design
(`update`, `replace_credential`, logins, settings, `live`, `sessions.start` / `interrupt`, `import` / `export`) answer
`unsupported` for now.

Event `agentsrc.updated {session_keys?}`: those sessions changed; no `session_keys` means the list itself changed. Over
HTTP: `GET /v1/spaces/{space}/events`, server-sent events.

Additions Persona hubs ignore: sessions carry `previewText`, `previewIsMine` and `onTopRankTimeMs`; a message's
`local` keeps `error`; accounts and sessions may carry `quota` (never filled in this version).

Rules the core keeps:

- **Spaces** isolate accounts, data and credentials (`/v1/spaces/{space}`); the Persona shell uses `default`.
- **Ids**: account ids are `a` and 12 hex digits; session keys `agent:<provider>:<accountId>:<externalId>`.
- **Outbox**: sends are deduplicated by `client_id`. A send whose outcome is unknown is marked `unconfirmed` and is
  never sent again automatically; after a restart, sends that were in flight become `unconfirmed` too.
- **Credentials** go in and never come out: no method returns one.
- **Assistant control** per account (`off`, `read`, `confirm`, `auto`; `off` by default): what the Persona assistant may
  do with that account's agents. With nobody present (scheduled tasks, background follow-ups) the assistant may only
  read, whatever the level.

## Layout

| Path | What |
| --- | --- |
| `cmd/personal-agent-club` | the program: extension mode, `serve`, `version` |
| `internal/protocol` | wire types and method names, pinned by golden JSON |
| `internal/provider` | the provider adapter interface, optional capabilities, the registry; `echo/` is the demo provider |
| `internal/store`, `internal/vault` | storage and credential interfaces with in-memory implementations |
| `internal/core` | spaces, accounts, sync, outbox, read state, search, events, assistant control |
| `internal/serve` | the HTTP shell and its token levels |
| `internal/extension` | the Persona extension shell |
| `internal/conformance` | one protocol test suite, run against both shells |

## Development

```sh
make check    # gofmt, go vet, go test -race
make dist     # the four platform binaries
```

## Roadmap

1. The Lark gateway draws an internal boundary around its built-in agent bridge (there; no behavior change).
2. **This repository**: core and `serve` shell, protocol conformance tests; then SQLite storage, the encrypted vault,
   the API providers ported with their recorded fixtures, quota collection, assistant control in accounts,
   interrupt and new-session capabilities. *(Skeleton done; the rest is next.)*
3. The Lark gateway gains a club backend behind an optional setting, off by default.
4. Migration of paired accounts into club, with verification, rollback and export.
5. The Persona extension: agent source and account page, then the assistant's `club_agent_*` tools, then gateway and
   launcher quota sources.
6. Browser-based providers, after checking that a browser can run in the extension sandbox.
7. Whether club becomes the Lark gateway's default backend is decided later.
