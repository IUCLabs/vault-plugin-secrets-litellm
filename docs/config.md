# config

Configures the connection to the LiteLLM proxy. The engine uses the master key
to create, update and delete virtual keys through the proxy's key-management
API (`/key/generate`, `/key/update`, `/key/delete`).

The config is seal-wrapped where the server supports it. Writing it drops the
cached client, so new settings apply to the next request.

## Write config

| Method | Path             |
| ------ | ---------------- |
| `POST` | `/litellm/config` |

### Parameters

| Name                      | Type   | Required | Default | Description                                         |
| ------------------------- | ------ | -------- | ------- | --------------------------------------------------- |
| `base_url`                | string | yes      |         | Base URL of the LiteLLM proxy.                      |
| `master_key`              | string | yes      |         | LiteLLM master key used to manage virtual keys.     |
| `request_timeout_seconds` | int    | no       | `30`    | HTTP timeout in seconds for calls to the proxy.     |

An update only changes the fields you pass.

### Example

```sh
vault write litellm/config \
  base_url=https://litellm.example.com \
  master_key=sk-master-... \
  request_timeout_seconds=15
```

## Read config

| Method | Path             |
| ------ | ---------------- |
| `GET`  | `/litellm/config` |

Returns `base_url` and `request_timeout_seconds`. The master key is never
returned.

```sh
$ vault read litellm/config
Key                        Value
---                        -----
base_url                   https://litellm.example.com
request_timeout_seconds    15
```

## Delete config

| Method   | Path             |
| -------- | ---------------- |
| `DELETE` | `/litellm/config` |

```sh
vault delete litellm/config
```

Existing leases cannot be revoked in LiteLLM until the engine is configured
again.
