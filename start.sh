#!/bin/bash
set -euo pipefail

log() {
  echo "[start.sh] $*"
}

wait_for_port() {
  local host="$1"
  local port="$2"
  local retries="${3:-10}"
  local delay="${4:-1}"
  local i
  for ((i=1; i<=retries; i++)); do
    if timeout 1 bash -c "</dev/tcp/${host}/${port}" >/dev/null 2>&1; then
      return 0
    fi
    sleep "${delay}"
  done
  return 1
}

RELOAD_FLAG="/app/data/reload"
DOZZLE_PID=""
SSH_PID=""

load_env() {
  if [[ -f /app/data/env.sh ]]; then
    # Allow user-provided overrides (e.g. DOZZLE_REMOTE_HOST).
    set +e +u
    source /app/data/env.sh
    set -euo pipefail
  fi
}

export DOZZLE_ADDR=":8081"
mkdir -p /app/data
chown -R cloudron:cloudron /app/data
export HOME="/app/data"

run_as_cloudron() {
  if command -v /usr/local/bin/gosu >/dev/null 2>&1; then
    /usr/local/bin/gosu cloudron "$@"
  else
    log "gosu missing, running as root"
    "$@"
  fi
}

start_tunnel_if_needed() {
  local raw_remote label dozzle_target
  raw_remote="${DOZZLE_REMOTE_HOST:-}"
  label="${DOZZLE_REMOTE_LABEL:-}"
  if [[ -n "${raw_remote}" && "${raw_remote}" == *"|"* && -z "${label}" ]]; then
    label="${raw_remote#*|}"
    raw_remote="${raw_remote%%|*}"
  fi

  if [[ -z "${DOZZLE_REMOTE_HOST:-}" && -n "${DOCKER_HOST:-}" ]]; then
    case "${DOCKER_HOST}" in
      tcp://*|ssh://*)
        export DOZZLE_REMOTE_HOST="${DOCKER_HOST}"
        raw_remote="${DOCKER_HOST}"
        ;;
    esac
  fi

  if [[ "${raw_remote}" == ssh://* ]]; then
    local ssh_target ssh_host ssh_port
    ssh_target="${raw_remote#ssh://}"
    ssh_host="${ssh_target}"
    ssh_port=""
    if [[ "${ssh_target}" == *:* ]]; then
      ssh_port="${ssh_target##*:}"
      ssh_host="${ssh_target%:*}"
    fi
    if [[ ! -f /app/data/.ssh/id_rsa ]]; then
      log "ssh key missing at /app/data/.ssh/id_rsa"
    else
      chmod 600 /app/data/.ssh/id_rsa || true
    fi
    if [[ -f /app/data/.ssh/known_hosts ]]; then
      chmod 644 /app/data/.ssh/known_hosts || true
    fi
    log "starting ssh tunnel to ${ssh_host}${ssh_port:+:${ssh_port}}"
    local ssh_opts=(
      -o ExitOnForwardFailure=yes
      -o StrictHostKeyChecking=accept-new
      -o IdentitiesOnly=yes
      -i /app/data/.ssh/id_rsa
      -o ServerAliveInterval=30
      -o ServerAliveCountMax=3
      -o ConnectTimeout=5
      -N
      -L 127.0.0.1:2375:/var/run/docker.sock
    )
    if [[ -n "${ssh_port}" && "${ssh_port}" != "${ssh_target}" ]]; then
      ssh_opts+=(-p "${ssh_port}")
    fi
    run_as_cloudron ssh "${ssh_opts[@]}" "${ssh_host}" &
    SSH_PID=$!
    dozzle_target="tcp://127.0.0.1:2375"
    if wait_for_port 127.0.0.1 2375 12 1; then
      log "ssh tunnel ready, using ${dozzle_target}"
    else
      if ! kill -0 "${SSH_PID}" >/dev/null 2>&1; then
        log "ssh tunnel failed to start"
      else
        log "ssh tunnel still not ready, proceeding anyway"
      fi
    fi
  fi

  if [[ -n "${raw_remote}" && "${raw_remote}" != ssh://* ]]; then
    dozzle_target="${raw_remote}"
  fi

  if [[ -n "${dozzle_target:-}" ]]; then
    if [[ -n "${label}" ]]; then
      export DOZZLE_REMOTE_HOST="${dozzle_target}|${label}"
    else
      export DOZZLE_REMOTE_HOST="${dozzle_target}"
    fi
  fi
}

stop_stack() {
  if [[ -n "${DOZZLE_PID}" ]]; then
    kill "${DOZZLE_PID}" >/dev/null 2>&1 || true
    wait "${DOZZLE_PID}" >/dev/null 2>&1 || true
    DOZZLE_PID=""
  fi
  if [[ -n "${SSH_PID}" ]]; then
    kill "${SSH_PID}" >/dev/null 2>&1 || true
    wait "${SSH_PID}" >/dev/null 2>&1 || true
    SSH_PID=""
  fi
}

start_stack() {
  load_env
  start_tunnel_if_needed
  log "starting dozzle on ${DOZZLE_ADDR}"
  run_as_cloudron /usr/local/bin/dozzle &
  DOZZLE_PID=$!
}

trap "stop_stack; exit 0" SIGTERM SIGINT

log "starting config-proxy on :8080"
if command -v /usr/local/bin/gosu >/dev/null 2>&1; then
  /usr/local/bin/gosu cloudron /usr/local/bin/config-proxy &
else
  /usr/local/bin/config-proxy &
fi

while true; do
  start_stack
  while kill -0 "${DOZZLE_PID}" >/dev/null 2>&1; do
    if [[ -f "${RELOAD_FLAG}" ]]; then
      log "reload requested"
      rm -f "${RELOAD_FLAG}"
      stop_stack
      break
    fi
    sleep 1
  done
  if ! kill -0 "${DOZZLE_PID}" >/dev/null 2>&1; then
    stop_stack
    sleep 1
  fi
done
