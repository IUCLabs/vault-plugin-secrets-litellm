#!/usr/bin/env bash
#
# End-to-end test for vault-plugin-secrets-litellm.
#
# Builds the plugin, brings up LiteLLM (+Postgres) plus both Vault and OpenBao,
# then drives the identical lifecycle against each engine to prove the same
# binary works on both:
#   configure -> create role -> generate key -> use key (scoped) -> revoke key.
#
# Select engines with ENGINES (default "vault openbao"), e.g. ENGINES=openbao.
#
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
COMPOSE_FILE="${ROOT_DIR}/test/docker-compose.yml"
COMPOSE="docker compose -f ${COMPOSE_FILE}"
BINARY="vault-plugin-secrets-litellm"

MASTER_KEY="sk-master-e2e-1234"
LITELLM_ADDR="http://127.0.0.1:4000"
MOUNT="litellm"
ENGINES="${ENGINES:-vault openbao}"

red()   { printf '\033[31m%s\033[0m\n' "$*"; }
green() { printf '\033[32m%s\033[0m\n' "$*"; }
blue()  { printf '\033[34m==> %s\033[0m\n' "$*"; }

cleanup() {
  blue "Tearing down containers"
  ${COMPOSE} down -v >/dev/null 2>&1 || true
}
trap cleanup EXIT

fail() { red "FAIL: $*"; exit 1; }

wait_for() {
  local desc="$1"; shift
  local retries="${WAIT_RETRIES:-90}"
  local i=0
  until "$@" >/dev/null 2>&1; do
    i=$((i + 1))
    if [ "$i" -ge "$retries" ]; then
      fail "timed out waiting for ${desc}"
    fi
    sleep 2
  done
  green "ready: ${desc}"
}

# chat_completion KEY MODEL -> prints the HTTP status code of a mock completion.
chat_completion() {
  local key="$1" model="$2"
  curl -s -o /dev/null -w '%{http_code}' \
    -H "Authorization: Bearer ${key}" -H 'Content-Type: application/json' \
    -d "{\"model\":\"${model}\",\"messages\":[{\"role\":\"user\",\"content\":\"hi\"}]}" \
    "${LITELLM_ADDR}/chat/completions"
}

# run_engine ENGINE_NAME CONTAINER CLI
run_engine() {
  local engine="$1" container="$2" cli="$3"
  blue "[${engine}] exercising the plugin"

  # ex runs the engine CLI inside its container (env already holds ADDR+TOKEN).
  ex() { ${COMPOSE} exec -T "${container}" "${cli}" "$@"; }

  local sha
  sha="$(sha256sum "${ROOT_DIR}/dist/${BINARY}" | awk '{print $1}')"

  ex plugin register -sha256="${sha}" secret "${BINARY}" >/dev/null || true
  ex secrets disable "${MOUNT}" >/dev/null 2>&1 || true
  ex secrets enable -path="${MOUNT}" "${BINARY}" >/dev/null
  green "[${engine}] plugin registered and mounted"

  # The plugin runs inside the engine container, so it reaches litellm by name.
  ex write "${MOUNT}/config" base_url="http://litellm:4000" master_key="${MASTER_KEY}" >/dev/null
  ex write "${MOUNT}/roles/team-a" \
    models="gpt-3.5-turbo" max_budget=10 ttl=1h max_ttl=24h >/dev/null
  green "[${engine}] configured + role created (models=gpt-3.5-turbo, budget=\$10)"

  # Generate a key, capturing both the key and its lease id.
  local json key lease
  json="$(ex read -format=json "${MOUNT}/creds/team-a")"
  key="$(printf '%s' "${json}" | python3 -c 'import sys,json;print(json.load(sys.stdin)["data"]["key"])')"
  lease="$(printf '%s' "${json}" | python3 -c 'import sys,json;print(json.load(sys.stdin)["lease_id"])')"
  [ -n "${key}" ] || fail "[${engine}] no key returned"
  green "[${engine}] issued key ${key:0:12}... (lease ${lease})"

  # Present in litellm?
  curl -fsS -H "Authorization: Bearer ${MASTER_KEY}" \
    "${LITELLM_ADDR}/key/info?key=${key}" >/dev/null \
    || fail "[${engine}] generated key not found in litellm"

  # Allowed model -> success.
  local code
  code="$(chat_completion "${key}" gpt-3.5-turbo)"
  [ "${code}" = "200" ] || fail "[${engine}] allowed model returned HTTP ${code}, expected 200"
  green "[${engine}] allowed model (gpt-3.5-turbo) accepted"

  # Disallowed model -> rejected.
  code="$(chat_completion "${key}" gpt-4)"
  [ "${code}" != "200" ] || fail "[${engine}] disallowed model unexpectedly succeeded"
  green "[${engine}] disallowed model (gpt-4) rejected (HTTP ${code})"

  # Revoke the lease -> key deleted from litellm.
  ex lease revoke "${lease}" >/dev/null
  sleep 2
  code="$(chat_completion "${key}" gpt-3.5-turbo)"
  [ "${code}" != "200" ] || fail "[${engine}] revoked key still works (HTTP ${code})"
  green "[${engine}] revoked key rejected (HTTP ${code})"

  green "[${engine}] PASSED"
}

# ---------------------------------------------------------------------------
blue "Building plugin for linux/amd64"
OS=linux ARCH=amd64 PLUGIN_DIR="${ROOT_DIR}/dist" make -C "${ROOT_DIR}" build

blue "Starting Docker stack (postgres + litellm + vault + openbao)"
${COMPOSE} up -d --build

wait_for "litellm" curl -fsS "${LITELLM_ADDR}/health/liveliness"

for engine in ${ENGINES}; do
  case "${engine}" in
    vault)
      wait_for "vault"   ${COMPOSE} exec -T vault   vault status -address=http://127.0.0.1:8200
      run_engine vault vault vault
      ;;
    openbao)
      wait_for "openbao" ${COMPOSE} exec -T openbao bao   status -address=http://127.0.0.1:8200
      run_engine openbao openbao bao
      ;;
    *)
      fail "unknown engine: ${engine}"
      ;;
  esac
done

green "ALL END-TO-END CHECKS PASSED (${ENGINES})"
