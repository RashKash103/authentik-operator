#!/usr/bin/env bash
#
# Start the local authentik stack and block until it is genuinely ready.
#
# "Ready" here means one specific thing: an *authenticated* API call using the
# bootstrap token succeeds. Not a TCP connect, not an unauthenticated health
# endpoint - both of those go green long before authentik has finished running
# its migrations and importing the bootstrap blueprint that creates the token.
# A suite that starts against a half-migrated authentik fails intermittently,
# which is far worse to debug than failing outright.
#
# Usage:
#   ./up.sh                        # defaults, or whatever ./.env says
#   AUTHENTIK_TAG=2026.2.7 ./up.sh # pick a version from supported-versions.yaml
#
# See .env.example for every variable.

set -euo pipefail

# shellcheck source=lib.sh
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib.sh"

readonly READY_PATH="/api/v3/core/users/me/"

main() {
  require_docker
  resolve_config

  local image="${AUTHENTIK_IMAGE}:${AUTHENTIK_TAG}"

  log "Starting local authentik test stack"
  info "project    ${COMPOSE_PROJECT_NAME}"
  info "image      ${image}"
  info "url        ${AUTHENTIK_URL}"
  info "timeout    ${AUTHENTIK_READY_TIMEOUT}s"
  echo

  command -v curl >/dev/null 2>&1 || die "curl is required for the readiness check."

  log "Pulling images (first run downloads ~1GB, subsequent runs are cached)"
  if ! compose pull --quiet 2>&1 | sed 's/^/    /'; then
    die "failed to pull images. Check that ${image} exists and that you can reach the registry."
  fi

  log "Bringing up postgresql, server and worker"
  if ! compose up -d --remove-orphans 2>&1 | sed 's/^/    /'; then
    fail "docker compose up failed."
    dump_diagnostics
    exit 1
  fi
  echo

  wait_for_ready

  echo
  ok "authentik is up and answering authenticated API calls."
  echo
  printf '    %sAPI / UI URL%s   %s\n'      "${_c_bold}" "${_c_reset}" "${AUTHENTIK_URL}"
  printf '    %sAPI base%s       %s/api/v3\n' "${_c_bold}" "${_c_reset}" "${AUTHENTIK_URL}"
  printf '    %sAdmin UI%s       %s/if/admin/\n' "${_c_bold}" "${_c_reset}" "${AUTHENTIK_URL}"
  printf '    %sVersion%s        %s\n'      "${_c_bold}" "${_c_reset}" "${AUTHENTIK_TAG}"
  echo
  printf '    %sAPI token%s      $AUTHENTIK_BOOTSTRAP_TOKEN\n' "${_c_bold}" "${_c_reset}"
  printf '                   defined in %s\n' "${AUTHENTIK_TEST_DIR}/.env.example"
  printf '                   overridden by %s if present\n' "${AUTHENTIK_TEST_DIR}/.env"
  printf '                   current value: %s\n' "${AUTHENTIK_BOOTSTRAP_TOKEN}"
  echo
  printf '    %sAdmin login%s    akadmin / %s\n' "${_c_bold}" "${_c_reset}" "${AUTHENTIK_BOOTSTRAP_PASSWORD}"
  echo
  dim "Point the operator at it with:"
  dim "  export AUTHENTIK_URL=${AUTHENTIK_URL}"
  dim "  export AUTHENTIK_TOKEN=${AUTHENTIK_BOOTSTRAP_TOKEN}"
  echo
  dim "Tear it down (and wipe its volumes) with: ./test/authentik/down.sh"
}

# Single authenticated probe. Prints the HTTP status on stdout and leaves the
# response body in the file named by $1.
probe() {
  local body_file="$1" status
  # curl already writes 000 for a connection-level failure, and also exits
  # non-zero; capture rather than `|| echo 000`, which would concatenate.
  status="$(curl --silent --output "${body_file}" \
    --write-out '%{http_code}' \
    --max-time 10 \
    --header "Authorization: Bearer ${AUTHENTIK_BOOTSTRAP_TOKEN}" \
    --header "Accept: application/json" \
    "${AUTHENTIK_URL}${READY_PATH}" 2>/dev/null)" || true
  [[ "${status}" =~ ^[0-9]{3}$ ]] || status="000"
  printf '%s' "${status}"
}

wait_for_ready() {
  local body_file status elapsed=0 last_status="" announced=0
  local deadline="${AUTHENTIK_READY_TIMEOUT}"
  local interval="${AUTHENTIK_READY_INTERVAL}"

  body_file="$(mktemp)"
  # shellcheck disable=SC2064
  trap "rm -f '${body_file}'" RETURN

  log "Waiting for authentik to finish migrating and bootstrapping"
  info "probe: GET ${AUTHENTIK_URL}${READY_PATH} with Authorization: Bearer <bootstrap token>"
  info "cold database: ~45s on a workstation, longer on a loaded CI runner"
  info "(migrations, then the worker imports blueprints and creates the token)"
  echo

  while (( elapsed < deadline )); do
    # Fail fast rather than burning the full timeout if a container died.
    if container_died; then
      echo
      fail "a container exited while waiting for authentik to become ready."
      dump_diagnostics
      exit 1
    fi

    status="$(probe "${body_file}")"

    if [[ "${status}" == "200" ]] && grep -q '"username"' "${body_file}" 2>/dev/null; then
      printf '\n'
      ok "readiness check passed after ${elapsed}s (HTTP 200 from ${READY_PATH})"
      local who
      who="$(sed -n 's/.*"username"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "${body_file}" | head -n1)"
      info "authenticated as: ${who:-<unknown>}"
      return 0
    fi

    if [[ "${status}" != "${last_status}" ]]; then
      printf '\n'
      info "$(printf '[%4ds] %s' "${elapsed}" "$(explain_status "${status}")")"
      last_status="${status}"
      announced=1
    elif (( elapsed % 15 == 0 )); then
      printf '.'
      announced=1
    fi

    sleep "${interval}"
    elapsed=$(( elapsed + interval ))
  done

  if (( announced )); then printf '\n'; fi
  echo
  fail "authentik did not become ready within ${deadline}s."
  fail "last probe: GET ${AUTHENTIK_URL}${READY_PATH} -> HTTP ${last_status:-000}"
  echo >&2
  cat >&2 <<HINT
  Common causes:
    * Still migrating. A very slow or loaded machine can exceed the timeout -
      raise it with AUTHENTIK_READY_TIMEOUT=1200 ./up.sh
    * HTTP 403 that never clears: the database predates the current
      AUTHENTIK_BOOTSTRAP_TOKEN. Bootstrap values are only applied to a *fresh*
      database. Run ./down.sh (which removes the volumes) and try again.
    * Port ${AUTHENTIK_HOST_PORT} is taken by something else on this host - set
      AUTHENTIK_HOST_PORT to a free port.
    * The image tag ${AUTHENTIK_TAG} does not exist or failed to pull.
HINT
  dump_diagnostics
  exit 1
}

explain_status() {
  case "$1" in
    000) echo "no HTTP response yet - server container is still starting" ;;
    200) echo "HTTP 200 but response body was not the expected user payload" ;;
    401|403) echo "HTTP $1 - API is up, bootstrap token not provisioned yet" ;;
    404) echo "HTTP 404 - API routes not registered yet" ;;
    500|502|503|504) echo "HTTP $1 - server is up but not serving yet (migrations?)" ;;
    *) echo "HTTP $1" ;;
  esac
}

# True if any stack container has exited. `restart: unless-stopped` means a
# crash usually shows as a restart loop rather than a clean exit, so this only
# catches hard failures - the timeout catches the rest.
container_died() {
  local ids id state
  ids="$(compose ps --all --quiet 2>/dev/null || true)"
  [[ -n "${ids}" ]] || return 1
  while IFS= read -r id; do
    [[ -n "${id}" ]] || continue
    state="$(docker inspect --format '{{.State.Status}}' "${id}" 2>/dev/null || echo unknown)"
    if [[ "${state}" == "exited" || "${state}" == "dead" ]]; then
      return 0
    fi
  done <<< "${ids}"
  return 1
}

dump_diagnostics() {
  echo >&2
  fail "--- container status ---"
  compose ps --all >&2 2>&1 || true
  local svc
  for svc in server worker postgresql; do
    echo >&2
    fail "--- last 60 log lines: ${svc} ---"
    compose logs --tail 60 --no-color "${svc}" >&2 2>&1 || true
  done
  echo >&2
  fail "--- full logs: docker compose -f ${COMPOSE_FILE} logs ---"
  fail "--- the stack is left running for inspection; ./down.sh removes it ---"
}

main "$@"
