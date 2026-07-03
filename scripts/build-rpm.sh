#!/usr/bin/env bash
#
# Package the (already built) plugin binary into an RPM with nfpm.
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

export PACKAGE_NAME="${BINARY}"
export PACKAGE_VERSION="${VERSION}"
export PACKAGE_RELEASE="1"
export PACKAGE_ARCH="amd64"
export PACKAGE_PLATFORM="linux"
export PACKAGE_DESCRIPTION="Vault/OpenBao dynamic secrets engine for LiteLLM virtual keys"
export PACKAGE_MAINTAINER="Ben Vincent <ben@unkin.net>"
export PACKAGE_HOMEPAGE="https://git.unkin.net/unkin/vault-plugin-secrets-litellm"
export PACKAGE_LICENSE="MIT"

envsubst < packaging/nfpm.yaml > "${DIST}/nfpm.yaml"
nfpm pkg --config "${DIST}/nfpm.yaml" --target "${DIST}" --packager rpm

echo "Built:"
ls -1 "${DIST}"/*.rpm
