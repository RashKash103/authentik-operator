#!/usr/bin/env bash
# Shared configuration resolution for up.sh / down.sh.
# Not executable on its own; source it.

set -euo pipefail

AUTHENTIK_TEST_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly AUTHENTIK_TEST_DIR

COMPOSE_FILE="${AUTHENTIK_TEST_DIR}/docker-compose.yaml"
readonly COMPOSE_FILE

# --- output helpers ---------------------------------------------------------

if [[ -t 1 ]] && [[ -z "${NO_COLOR:-}" ]]; then
  _c_reset=$'\033[0m'; _c_bold=$'\033[1m'; _c_dim=$'\033[2m'
  _c_red=$'\033[31m'; _c_green=$'\033[32m'; _c_yellow=$'\033[33m'
else
  _c_reset=''; _c_bold=''; _c_dim=''; _c_red=''; _c_green=''; _c_yellow=''
fi

log()   { printf '%s==>%s %s\n' "${_c_bold}" "${_c_reset}" "$*"; }
info()  { printf '    %s\n' "$*"; }
dim()   { printf '%s    %s%s\n' "${_c_dim}" "$*" "${_c_reset}"; }
ok()    { printf '%s==>%s %s\n' "${_c_green}${_c_bold}" "${_c_reset}" "$*"; }
warn()  { printf '%s==>%s %s\n' "${_c_yellow}${_c_bold}" "${_c_reset}" "$*" >&2; }
fail()  { printf '%s==> ERROR:%s %s\n' "${_c_red}${_c_bold}" "${_c_reset}" "$*" >&2; }

die() { fail "$*"; exit 1; }

# --- .env loading -----------------------------------------------------------

# Load KEY=VALUE pairs from a file without clobbering anything already exported.
# Precedence ends up: shell environment > .env > defaults in this file.
load_env_file() {
  local file="$1" line key value
  [[ -f "${file}" ]] || return 0

  while IFS= read -r line || [[ -n "${line}" ]]; do
    line="${line%$'\r'}"
    # Skip blanks, comments and `export `-prefixed noise we can't safely eval.
    [[ "${line}" =~ ^[[:space:]]*(#.*)?$ ]] && continue
    [[ "${line}" == *"="* ]] || continue

    key="${line%%=*}"
    value="${line#*=}"
    key="${key#"${key%%[![:space:]]*}"}"
    key="${key%"${key##*[![:space:]]}"}"
    key="${key#export }"
    [[ "${key}" =~ ^[A-Za-z_][A-Za-z0-9_]*$ ]] || continue

    # Trim surrounding whitespace, then strip one layer of matching quotes
    # (so `KEY = value` and `KEY="a b"` both do the obvious thing).
    value="${value#"${value%%[![:space:]]*}"}"
    value="${value%"${value##*[![:space:]]}"}"
    if [[ "${value}" == \"*\" && ${#value} -ge 2 ]]; then
      value="${value:1:${#value}-2}"
    elif [[ "${value}" == \'*\' && ${#value} -ge 2 ]]; then
      value="${value:1:${#value}-2}"
    fi

    # Already set in the environment? The shell wins.
    [[ -n "${!key:-}" ]] && continue
    export "${key}=${value}"
  done < "${file}"
}

# --- configuration ----------------------------------------------------------

resolve_config() {
  load_env_file "${AUTHENTIK_TEST_DIR}/.env"

  : "${COMPOSE_PROJECT_NAME:=authentik-operator-test}"

  : "${AUTHENTIK_IMAGE:=ghcr.io/goauthentik/server}"
  : "${AUTHENTIK_TAG:=2026.8.2}"

  # AUTHENTIK_IMAGE is the repository; the compose file appends AUTHENTIK_TAG.
  # Callers naturally hand over a fully pinned reference instead, which would
  # compose to "server:2026.8.2:2026.8.2" and fail to pull. Accept both: if a
  # tag is already present, split it off and let it win.
  case "${AUTHENTIK_IMAGE##*/}" in
    *:*)
      AUTHENTIK_TAG="${AUTHENTIK_IMAGE##*:}"
      AUTHENTIK_IMAGE="${AUTHENTIK_IMAGE%:*}"
      ;;
  esac

  : "${AUTHENTIK_BOOTSTRAP_TOKEN:=authentik-operator-e2e-bootstrap-token}"
  : "${AUTHENTIK_BOOTSTRAP_PASSWORD:=authentik-operator-e2e-admin-password}"
  : "${AUTHENTIK_BOOTSTRAP_EMAIL:=akadmin@e2e.localhost}"

  : "${AUTHENTIK_SECRET_KEY:=insecure-local-test-secret-key-do-not-use-anywhere-else}"
  : "${AUTHENTIK_LOG_LEVEL:=info}"

  : "${AUTHENTIK_HOST_PORT:=9000}"
  : "${AUTHENTIK_HOST_PORT_HTTPS:=9443}"

  : "${POSTGRES_IMAGE:=docker.io/library/postgres:16-alpine}"
  : "${PG_DB:=authentik}"
  : "${PG_USER:=authentik}"
  : "${PG_PASS:=insecure-local-test-password}"

  : "${AUTHENTIK_READY_TIMEOUT:=600}"
  : "${AUTHENTIK_READY_INTERVAL:=3}"

  : "${AUTHENTIK_URL:=http://localhost:${AUTHENTIK_HOST_PORT}}"

  export COMPOSE_PROJECT_NAME \
    AUTHENTIK_IMAGE AUTHENTIK_TAG \
    AUTHENTIK_BOOTSTRAP_TOKEN AUTHENTIK_BOOTSTRAP_PASSWORD AUTHENTIK_BOOTSTRAP_EMAIL \
    AUTHENTIK_SECRET_KEY AUTHENTIK_LOG_LEVEL \
    AUTHENTIK_HOST_PORT AUTHENTIK_HOST_PORT_HTTPS \
    POSTGRES_IMAGE PG_DB PG_USER PG_PASS \
    AUTHENTIK_READY_TIMEOUT AUTHENTIK_READY_INTERVAL AUTHENTIK_URL
}

# --- docker -----------------------------------------------------------------

require_docker() {
  command -v docker >/dev/null 2>&1 \
    || die "docker is not installed or not on PATH."
  docker compose version >/dev/null 2>&1 \
    || die "the docker compose v2 plugin is required (\`docker compose version\` failed)."
  docker info >/dev/null 2>&1 \
    || die "cannot talk to the Docker daemon. Is it running, and is your user in the docker group?"
}

compose() {
  docker compose --project-directory "${AUTHENTIK_TEST_DIR}" -f "${COMPOSE_FILE}" "$@"
}
