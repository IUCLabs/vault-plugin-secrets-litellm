# vault-plugin-secrets-litellm

A dynamic secrets engine for [LiteLLM](https://github.com/BerriAI/litellm) that
runs on both **HashiCorp Vault** and **[OpenBao](https://openbao.org)**.
It generates **virtual API keys** on a LiteLLM proxy that are:

- **scoped to specific models** (`models`)
- capped by a **spending limit** (`max_budget`)
- bound to a **lease TTL** — revoking the lease revokes the key in LiteLLM

## Vault and OpenBao

OpenBao is a fork of Vault and keeps its plugin protocol compatible, so the
**same plugin binary** registers and runs on either engine unchanged — the CLI
commands below are identical apart from `vault` vs `bao`. The end-to-end test
suite exercises the full lifecycle against both to prove it.

## How it works

The backend authenticates to the LiteLLM proxy with the master key and manages
virtual keys through the proxy's key-management API (`/key/generate`,
`/key/update`, `/key/delete`, `/key/info`). Each generated key is wrapped in a
Vault lease, so Vault owns the key's lifecycle: renew extends it, revoke deletes
it.

```
┌─────────┐  read creds/<role>   ┌──────────────────────┐  /key/generate   ┌──────────┐
│  client │ ───────────────────► │ vault + litellm plugin│ ───────────────► │ litellm  │
└─────────┘  ◄─── virtual key ── └──────────────────────┘  ◄── sk-... ───── └──────────┘
```

## Usage

```sh
# 1. Enable the engine
vault secrets enable -path=litellm vault-plugin-secrets-litellm

# 2. Configure the connection to the LiteLLM proxy
vault write litellm/config \
  base_url=http://litellm:4000 \
  master_key=sk-master-...

# 3. Define a role: which models, how much budget, what TTL
vault write litellm/roles/team-a \
  models="gpt-3.5-turbo,gpt-4" \
  max_budget=50 \
  ttl=1h \
  max_ttl=24h

# 4. Generate a scoped, budgeted, time-limited virtual key
vault read litellm/creds/team-a
# Key                key
# ---                ---
# lease_id           litellm/creds/team-a/AbC...
# lease_duration     1h
# key                sk-...
# max_budget         50
# models             [gpt-3.5-turbo gpt-4]

# 5. Revoking the lease revokes the key in LiteLLM
vault lease revoke litellm/creds/team-a/AbC...
```

## Paths

| Path                     | Ops                | Description                                        |
| ------------------------ | ------------------ | -------------------------------------------------- |
| `config`                 | read/write/delete  | LiteLLM connection (`base_url`, `master_key`)      |
| `roles/<name>`           | read/write/delete  | Constraints for generated keys                     |
| `roles/`                 | list               | List configured roles                              |
| `creds/<name>`           | read               | Generate a virtual key for the role                |

### Role fields

| Field              | Type     | Description                                            |
| ------------------ | -------- | ------------------------------------------------------ |
| `models`           | list     | Allowed models; empty means unrestricted               |
| `max_budget`       | float    | Spending limit per key; 0 means unlimited              |
| `key_alias_prefix` | string   | Prefix for the generated key alias (default `vault`)   |
| `metadata`         | kv pairs | Metadata attached to each generated key                |
| `ttl`              | duration | Default lease TTL                                      |
| `max_ttl`          | duration | Maximum lease TTL                                      |

## Development

```sh
make build        # build the plugin into ./dist
make test         # unit tests (race-enabled)
make lint         # go vet
make fmt          # gofmt
make e2e          # full end-to-end test in Docker against Vault AND OpenBao
make e2e-vault    # e2e against Vault only
make e2e-openbao  # e2e against OpenBao only
```

### End-to-end tests

`make e2e` builds the plugin, spins up a LiteLLM proxy with its Postgres key
store, plus both a Vault and an OpenBao dev server with the plugin mounted, then
exercises the whole lifecycle against **each engine**: configure → create role →
generate key → call an allowed model → verify a disallowed model is rejected →
revoke → verify the key stops working.

Requires Docker. Bind mounts use the `:z` flag so the stack works under SELinux
(Fedora/RHEL). Select engines with `ENGINES="vault openbao"`.

### Releasing

Versioning is tag-driven:

```sh
make patch   # v0.1.0 -> v0.1.1
make minor   # v0.1.1 -> v0.2.0
make major   # v0.2.0 -> v1.0.0
```
