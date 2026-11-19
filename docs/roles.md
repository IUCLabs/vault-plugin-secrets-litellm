# roles

A role sets the constraints applied to every key generated from
`creds/<name>`: which models it may call, its spending limit, its alias and
metadata, and its lease TTLs.

## Create or update a role

| Method | Path                    |
| ------ | ----------------------- |
| `POST` | `/litellm/roles/:name`  |

### Parameters

| Name               | Type                   | Default   | Description                                                                 |
| ------------------ | ---------------------- | --------- | --------------------------------------------------------------------------- |
| `name`             | string (in path)       |           | Name of the role. Lower-cased.                                              |
| `models`           | comma-separated list   | `[]`      | Models a generated key may call. Empty means unrestricted.                  |
| `max_budget`       | float                  | `0`       | Spending limit per key, in the proxy's currency units. `0` means unlimited. |
| `key_alias_prefix` | string                 | `vault`   | Prefix of the generated key alias (`<prefix>-<role>-<8 hex chars>`).        |
| `metadata`         | key=value pairs        | `{}`      | Metadata attached to each generated key in LiteLLM.                         |
| `ttl`              | duration               | mount default | Default lease TTL.                                                      |
| `max_ttl`          | duration               | mount max | Maximum lease TTL. `ttl` must not exceed it.                                |

An update only changes the fields you pass. `max_budget` must not be negative.

### Example

```sh
vault write litellm/roles/team-a \
  models="gpt-4o,claude-3-5-sonnet" \
  max_budget=50 \
  key_alias_prefix=team \
  metadata="team=a" metadata="cost_center=1234" \
  ttl=1h \
  max_ttl=24h
```

## Read a role

| Method | Path                   |
| ------ | ---------------------- |
| `GET`  | `/litellm/roles/:name` |

`ttl` and `max_ttl` are returned in seconds.

```sh
$ vault read litellm/roles/team-a
Key                 Value
---                 -----
key_alias_prefix    team
max_budget          50
max_ttl             86400
metadata            map[cost_center:1234 team:a]
models              [gpt-4o claude-3-5-sonnet]
ttl                 3600
```

## List roles

| Method | Path              |
| ------ | ----------------- |
| `LIST` | `/litellm/roles`  |

```sh
vault list litellm/roles
```

## Delete a role

| Method   | Path                   |
| -------- | ---------------------- |
| `DELETE` | `/litellm/roles/:name` |

```sh
vault delete litellm/roles/team-a
```

Keys already issued from the role stay valid until their lease expires or is
revoked, but they can no longer be renewed.
