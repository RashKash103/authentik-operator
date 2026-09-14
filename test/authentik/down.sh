#!/usr/bin/env bash
#
# Stop the local authentik stack and remove its volumes.
#
# Volumes are removed on purpose. authentik only applies AUTHENTIK_BOOTSTRAP_*
# to a fresh database, so leaving the PostgreSQL volume behind would make a
# later run silently reuse the old token, old password and any objects a
# previous E2E run created. Every ./up.sh should start from nothing.
#
# Idempotent: exits 0 when there is nothing to tear down.
#
# Usage:
#   ./down.sh
#   ./down.sh --keep-volumes    # stop containers, keep the database

set -euo pipefail

# shellcheck source=lib.sh
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib.sh"

main() {
  local keep_volumes=0

  while (( $# )); do
    case "$1" in
      --keep-volumes) keep_volumes=1; shift ;;
      -h|--help)
        sed -n '2,/^set -euo/p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//; $d'
        exit 0
        ;;
      *) die "unknown argument: $1 (try --help)" ;;
    esac
  done

  # Nothing to do without a daemon, and that is not an error for a teardown.
  if ! command -v docker >/dev/null 2>&1; then
    warn "docker is not installed; nothing to tear down."
    exit 0
  fi
  if ! docker info >/dev/null 2>&1; then
    warn "cannot reach the Docker daemon; nothing to tear down."
    exit 0
  fi
  if ! docker compose version >/dev/null 2>&1; then
    warn "docker compose v2 plugin not available; nothing to tear down."
    exit 0
  fi

  resolve_config

  log "Tearing down the local authentik test stack"
  info "project ${COMPOSE_PROJECT_NAME}"

  local args=(down --remove-orphans --timeout 20)
  if (( keep_volumes )); then
    info "keeping volumes (--keep-volumes)"
  else
    info "removing volumes (database, media, certs, templates, redis)"
    args+=(--volumes)
  fi

  # `down` is already a no-op when nothing is running, but keep the whole
  # teardown non-fatal so a failed CI job can always run this in cleanup.
  # --profile "*" so the optional legacy-redis service is torn down too.
  if ! compose --profile "*" "${args[@]}" 2>&1 | sed 's/^/    /'; then
    warn "docker compose down reported an error; checking for leftovers."
  fi

  if (( ! keep_volumes )); then
    remove_leftover_volumes
  fi

  echo
  ok "Local authentik test stack is down."
}

# Belt and braces: if the compose file changed between up and down (say, a
# volume was renamed), `down -v` will not know about the old volume. Sweep
# anything still carrying this project's compose label.
remove_leftover_volumes() {
  local leftovers
  leftovers="$(docker volume ls --quiet \
    --filter "label=com.docker.compose.project=${COMPOSE_PROJECT_NAME}" 2>/dev/null || true)"
  [[ -n "${leftovers}" ]] || return 0

  info "removing leftover volumes:"
  while IFS= read -r vol; do
    [[ -n "${vol}" ]] || continue
    dim "  ${vol}"
    docker volume rm "${vol}" >/dev/null 2>&1 || warn "could not remove volume ${vol}"
  done <<< "${leftovers}"
}

main "$@"
