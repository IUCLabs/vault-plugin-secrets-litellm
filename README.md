# vault-plugin-secrets-litellm

A dynamic secrets engine for [HashiCorp Vault](https://www.vaultproject.io) and
[OpenBao](https://openbao.org) that issues [LiteLLM](https://github.com/BerriAI/litellm)
virtual API keys. Each key is scoped to a set of models, capped by a spending
limit and bound to a Vault lease: renewing the lease extends the key, revoking
the lease deletes the key from the LiteLLM proxy.

The same plugin binary runs unchanged on Vault and OpenBao. Every `vault`
command below works with `bao` in its place.

To manage the engine's configuration and roles from Terraform, use
[terraform-provider-vault-secrets-litellm](https://github.com/IUCLabs/terraform-provider-vault-secrets-litellm).

## Requirements

- Vault or OpenBao (tested against Vault 1.18 and the latest OpenBao release)
- A LiteLLM proxy with a database-backed key store and a master key
- Go 1.25+ (to build from source)

## Installation

### Install

Install the x86_64 RPM (vault or openbao flavour) from the
[releases page](https://github.com/IUCLabs/vault-plugin-secrets-litellm/releases).
It installs the binary as `/usr/libexec/vault-plugins/vault-plugin-secrets-litellm`
(vault) or `/usr/libexec/openbao-plugins/vault-plugin-secrets-litellm` (openbao).
Or build one:

```sh
git clone https://github.com/IUCLabs/vault-plugin-secrets-litellm
cd vault-plugin-secrets-litellm
make build   # writes ./dist/vault-plugin-secrets-litellm
```

### Register

Copy the binary into the directory set as `plugin_directory` in the server
config (Vault or OpenBao), then register it in the plugin catalog with its
SHA-256. Replace `<plugin_directory>` below with that path:

```sh
cp dist/vault-plugin-secrets-litellm <plugin_directory>/

SHA256=$(sha256sum <plugin_directory>/vault-plugin-secrets-litellm | cut -d' ' -f1)

vault plugin register \
  -sha256="$SHA256" \
  secret vault-plugin-secrets-litellm
```

### Enable

```sh
vault secrets enable -path=litellm vault-plugin-secrets-litellm
```

## Configuration

Point the engine at the LiteLLM proxy. The master key is write-only and is
never returned on read.

```sh
vault write litellm/config \
  base_url=https://litellm.example.com \
  master_key=sk-master-...
```

## Usage

Create a role that sets the models, budget and TTL of the keys it issues:

```sh
vault write litellm/roles/team-a \
  models="gpt-4o,claude-3-5-sonnet" \
  max_budget=50 \
  ttl=1h \
  max_ttl=24h
```

Generate a key:

```sh
$ vault read litellm/creds/team-a
Key                Value
---                -----
lease_id           litellm/creds/team-a/AbC123...
lease_duration     1h
lease_renewable    true
key                sk-...
key_alias          vault-team-a-1a2b3c4d
max_budget         50
models             [gpt-4o claude-3-5-sonnet]
role               team-a
```

Revoke it, which deletes the key in LiteLLM:

```sh
vault lease revoke litellm/creds/team-a/AbC123...
```

## Documentation

Full path reference:

- [config](docs/config.md) - connection to the LiteLLM proxy
- [roles](docs/roles.md) - constraints applied to generated keys
- [creds](docs/creds.md) - generating, renewing and revoking keys

## Development

```sh
make build   # build ./dist/vault-plugin-secrets-litellm
make test    # unit tests
make lint    # go vet
make e2e     # end-to-end tests in Docker against Vault and OpenBao
```

## License

[MIT](LICENSE)
