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

Release assets download through artifactapi's GitHub proxy, which serves
`https://artifactapi.k8s.syd1.au.unkin.net/api/v1/remote/github/IUCLabs/vault-plugin-secrets-litellm/releases/download/<tag>/<file>`.
Each method below ends with `SHA256` set for [registration](#register-and-enable).
With OpenBao, use `bao` in place of `vault`.

```sh
TAG=vX.Y.Z  # the release to install
V=${TAG#v}
BASE=https://artifactapi.k8s.syd1.au.unkin.net/api/v1/remote/github/IUCLabs/vault-plugin-secrets-litellm/releases/download/$TAG
```

### RPM

`vault-plugin-secrets-litellm` installs `/usr/libexec/vault-plugins/vault-plugin-secrets-litellm`;
`openbao-plugin-secrets-litellm` installs it in `/usr/libexec/openbao-plugins/`.
Set the server's `plugin_directory` to that directory.

```sh
dnf install vault-plugin-secrets-litellm  # from a yum repo carrying it, or from the release:
dnf install "$BASE/vault-plugin-secrets-litellm-$V-1.x86_64.rpm"  # openbao-plugin-secrets-litellm-$V-1.x86_64.rpm for OpenBao

SHA256=$(sha256sum /usr/libexec/vault-plugins/vault-plugin-secrets-litellm | cut -d' ' -f1)
```

### Released binary

`vault-plugin-secrets-litellm_<version>_SHA256SUMS` lists the binary and the RPMs, and
`SHA256SUMS.sig` is its detached signature by the IUCLabs release GPG key.

```sh
curl -fLO "$BASE/vault-plugin-secrets-litellm_${V}_linux_amd64"
curl -fLO "$BASE/vault-plugin-secrets-litellm_${V}_SHA256SUMS"
curl -fLO "$BASE/vault-plugin-secrets-litellm_${V}_SHA256SUMS.sig"
gpg --verify "vault-plugin-secrets-litellm_${V}_SHA256SUMS.sig" "vault-plugin-secrets-litellm_${V}_SHA256SUMS"
sha256sum --check --ignore-missing "vault-plugin-secrets-litellm_${V}_SHA256SUMS"

install -m 0755 "vault-plugin-secrets-litellm_${V}_linux_amd64" <plugin_directory>/vault-plugin-secrets-litellm
SHA256=$(sha256sum <plugin_directory>/vault-plugin-secrets-litellm | cut -d' ' -f1)
```

### Kubernetes image volume

`ghcr.io/iuclabs/vault-plugin-secrets-litellm:<tag>` is a `FROM scratch` image holding only
`/vault-plugin-secrets-litellm`. Mount it read-only into the Vault/OpenBao container with an
[image volume](https://kubernetes.io/docs/concepts/storage/volumes/#image):
Kubernetes 1.31+ with the `ImageVolume` feature gate (beta, on by default from
1.33). Vault and OpenBao only run a binary that sits directly in
`plugin_directory`, not in a subdirectory, so mount the file itself with
`subPath`, which on image volumes needs 1.33+.

```yaml
spec:
  containers:
    - name: vault
      volumeMounts:
        - name: plugin-litellm
          mountPath: /vault/plugins/vault-plugin-secrets-litellm  # <plugin_directory>/vault-plugin-secrets-litellm
          subPath: vault-plugin-secrets-litellm
          readOnly: true
  volumes:
    - name: plugin-litellm
      image:
        reference: ghcr.io/iuclabs/vault-plugin-secrets-litellm:vX.Y.Z
        pullPolicy: IfNotPresent
```

The image holds the released binary, so its sha256 is the `linux_amd64` line
of `SHA256SUMS`:

```sh
SHA256=$(curl -fsSL "$BASE/vault-plugin-secrets-litellm_${V}_SHA256SUMS" | awk '/_linux_amd64$/ {print $1}')
```

### From source

```sh
make build  # writes ./dist/vault-plugin-secrets-litellm
```

Copy it into `plugin_directory` and take `SHA256` from `sha256sum` as above.

### Register and enable

```sh
vault plugin register -sha256="$SHA256" secret vault-plugin-secrets-litellm
vault secrets enable -path=litellm vault-plugin-secrets-litellm
```

After replacing the binary, register the new sha256 and reload the plugin:

```sh
vault write sys/plugins/reload/backend plugin=vault-plugin-secrets-litellm
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
