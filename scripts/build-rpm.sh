#!/usr/bin/env bash
#
# Package the (already built) plugin binary into RPMs with nfpm.
# Builds one RPM per target server: Vault (/opt/vault-plugins) and
# OpenBao (/opt/openbao-plugins). Both wrap the same binary.
# Usage: scripts/build-rpm.sh [version]   (version defaults to $CI_COMMIT_TAG)
#
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"

VERSION="${1:-${CI_COMMIT_TAG:-0.0.0-dev}}"
VERSION="${VERSION#v}" # strip a leading v
BINARY="vault-plugin-secrets-litellm"
DIST="dist"

if [ ! -f "${DIST}/${BINARY}" ]; then
  echo "ERROR: ${DIST}/${BINARY} not found; run 'make build' first" >&2
  exit 1
fi

# Fields shared across every flavour.
export PACKAGE_VERSION="${VERSION}"
export PACKAGE_RELEASE="1"
export PACKAGE_ARCH="amd64"
export PACKAGE_PLATFORM="linux"
export PACKAGE_DESCRIPTION="Vault/OpenBao dynamic secrets engine for LiteLLM virtual keys"
export PACKAGE_MAINTAINER="IUCLabs"
export PACKAGE_HOMEPAGE="https://github.com/IUCLabs/vault-plugin-secrets-litellm"
export PACKAGE_LICENSE="MIT"

# build_flavor <package-name> <plugin-dir>
build_flavor() {
  export PACKAGE_NAME="$1"
  export PACKAGE_PLUGIN_DIR="$2"
  export PACKAGE_PREINSTALL="${DIST}/preinstall-${PACKAGE_NAME}.sh"

  envsubst '${PACKAGE_PLUGIN_DIR}' \
    < packaging/scripts/preinstall.sh.tmpl > "${PACKAGE_PREINSTALL}"
  envsubst < packaging/nfpm.yaml > "${DIST}/nfpm-${PACKAGE_NAME}.yaml"
  nfpm pkg --config "${DIST}/nfpm-${PACKAGE_NAME}.yaml" --target "${DIST}" --packager rpm
}

build_flavor "vault-plugin-secrets-litellm"   "/opt/vault-plugins"
build_flavor "openbao-plugin-secrets-litellm" "/opt/openbao-plugins"

echo "Built:"
ls -1 "${DIST}"/*.rpm
