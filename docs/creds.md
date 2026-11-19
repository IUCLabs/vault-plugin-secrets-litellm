# creds

Reading `creds/<name>` generates a new LiteLLM virtual key constrained by the
named role, wrapped in a lease.

## Generate a key

| Method         | Path                   |
| -------------- | ---------------------- |
| `GET` / `POST` | `/litellm/creds/:name` |

### Parameters

| Name   | Type             | Description                          |
| ------ | ---------------- | ------------------------------------ |
| `name` | string (in path) | Role to generate the key from.       |

### Response

| Field        | Description                                     |
| ------------ | ----------------------------------------------- |
| `key`        | The LiteLLM virtual key (`sk-...`).             |
| `key_alias`  | Alias of the key in LiteLLM.                    |
| `models`     | Models the key may call.                        |
| `max_budget` | Spending limit of the key (`0` = unlimited).    |
| `role`       | Role the key was generated from.                |

The lease TTL is the role's `ttl` (or the mount default), clamped to the role's
`max_ttl` and the mount maximum. The same duration is set as the key's expiry
in LiteLLM, so the key also stops working if the server cannot revoke it.

### Example

```sh
$ vault read litellm/creds/team-a
Key                Value
---                -----
lease_id           litellm/creds/team-a/AbC123...
lease_duration     1h
lease_renewable    true
key                sk-...
key_alias          team-team-a-1a2b3c4d
max_budget         50
models             [gpt-4o claude-3-5-sonnet]
role               team-a
```

Use the key against the proxy like any LiteLLM key:

```sh
curl https://litellm.example.com/v1/chat/completions \
  -H "Authorization: Bearer sk-..." \
  -H "Content-Type: application/json" \
  -d '{"model": "gpt-4o", "messages": [{"role": "user", "content": "hello"}]}'
```

## Renew

Renewing the lease resets it to the role's current `ttl`, if set, and pushes
the new expiry to LiteLLM (best effort; the lease stays authoritative). Renewal
fails if the role has been deleted.

```sh
vault lease renew litellm/creds/team-a/AbC123...
```

## Revoke

Revoking the lease deletes the key from LiteLLM.

```sh
vault lease revoke litellm/creds/team-a/AbC123...

# revoke every key issued from a role
vault lease revoke -prefix litellm/creds/team-a
```
