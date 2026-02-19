#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
PROJECT_ROOT=$(cd -- "$SCRIPT_DIR/.." && pwd)

MEM_BIN="${MEM_BIN:-mem}"
SANDBOX_ROOT="${MEMORY_SANDBOX_ROOT:-$PROJECT_ROOT/sandbox/memory-eval}"
SANDBOX_REPO="${MEMORY_SANDBOX_REPO:-$SANDBOX_ROOT/repo}"
SANDBOX_DATA_DIR="${MEMORY_SANDBOX_DATA_DIR:-$SANDBOX_ROOT/data}"
SANDBOX_XDG_CONFIG="${MEMORY_SANDBOX_XDG_CONFIG:-$SANDBOX_ROOT/xdg/config}"
SANDBOX_XDG_CACHE="${MEMORY_SANDBOX_XDG_CACHE:-$SANDBOX_ROOT/xdg/cache}"
SANDBOX_EVIDENCE_DIR="${MEMORY_SANDBOX_EVIDENCE_DIR:-$SANDBOX_ROOT/evidence}"
THREAD_ID="${MEMORY_SANDBOX_THREAD:-T-SANDBOX-EVAL}"
RUN_ID=$(date -u +%Y%m%dT%H%M%SZ)
RUN_DIR="$SANDBOX_EVIDENCE_DIR/$RUN_ID"

log() {
  printf '%s\n' "$*"
}

fail() {
  printf 'ERROR: %s\n' "$*" >&2
  exit 1
}

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    fail "required command not found: $1"
  fi
}

extract_id() {
  sed -n 's/.*"id"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1
}

run_mem() {
  (
    cd "$SANDBOX_REPO"
    MEMPACK_DATA_DIR="$SANDBOX_DATA_DIR" \
      XDG_CONFIG_HOME="$SANDBOX_XDG_CONFIG" \
      XDG_CACHE_HOME="$SANDBOX_XDG_CACHE" \
      "$MEM_BIN" "$@"
  )
}

setup_sandbox() {
  need_cmd git
  need_cmd "$MEM_BIN"

  mkdir -p "$SANDBOX_REPO" "$SANDBOX_DATA_DIR" "$SANDBOX_XDG_CONFIG" "$SANDBOX_XDG_CACHE" "$SANDBOX_EVIDENCE_DIR"

  if [ ! -d "$SANDBOX_REPO/.git" ]; then
    git -C "$SANDBOX_REPO" init -q
  fi

  if ! git -C "$SANDBOX_REPO" rev-parse --verify HEAD >/dev/null 2>&1; then
    cat > "$SANDBOX_REPO/README.md" <<'TXT'
# Mempack Sandbox Memory Eval Repo

This repo is intentionally local-only and ignored by git in the parent project.
TXT
    git -C "$SANDBOX_REPO" add README.md
    git -C "$SANDBOX_REPO" -c user.name='Sandbox' -c user.email='sandbox@example.com' commit -q -m 'Initialize sandbox repo'
  fi

  # Ensure repo cache in isolated config points to the sandbox repo itself.
  run_mem use "$SANDBOX_REPO" >/dev/null
  run_mem init --no-agents >/dev/null

  log "Sandbox repo ready: $SANDBOX_REPO"
  log "Sandbox data dir:   $SANDBOX_DATA_DIR"
  log "Sandbox config dir: $SANDBOX_XDG_CONFIG"
  log "Evidence root:      $SANDBOX_EVIDENCE_DIR"
}

run_cli_suite() {
  mkdir -p "$RUN_DIR"

  local token_a="sbx-alpha-$RUN_ID"
  local token_b="sbx-beta-$RUN_ID"
  local token_c="sbx-gamma-$RUN_ID"

  local add_a add_b id_a id_b
  add_a=$(run_mem add --thread "$THREAD_ID" --title "Sandbox A" --summary "Primary token $token_a" --entities "ent_$token_a")
  printf '%s\n' "$add_a" > "$RUN_DIR/step1_add_a.json"
  id_a=$(printf '%s\n' "$add_a" | extract_id)
  [ -n "$id_a" ] || fail "failed to parse id for memory A"

  add_b=$(run_mem add --thread "$THREAD_ID" --title "Sandbox B" --summary "Secondary token $token_b" --entities "ent_$token_b")
  printf '%s\n' "$add_b" > "$RUN_DIR/step2_add_b.json"
  id_b=$(printf '%s\n' "$add_b" | extract_id)
  [ -n "$id_b" ] || fail "failed to parse id for memory B"

  run_mem link --from "$id_a" --rel relates_to --to "$id_b" > "$RUN_DIR/step3_link.json"

  local get_a
  get_a=$(run_mem get "$token_a")
  printf '%s\n' "$get_a" > "$RUN_DIR/step4_get_a.json"
  printf '%s\n' "$get_a" | grep -F "\"id\": \"$id_a\"" >/dev/null || fail "token_a did not retrieve memory A"
  printf '%s\n' "$get_a" | grep -F '"rel": "relates_to"' >/dev/null || fail "link trail missing relation"
  printf '%s\n' "$get_a" | grep -F "$id_b" >/dev/null || fail "link trail missing memory B id"

  run_mem update "$id_a" --summary "Primary token $token_c" --entities "ent_$token_c" > "$RUN_DIR/step5_update_a.json"

  local get_old get_new
  get_old=$(run_mem get "$token_a")
  printf '%s\n' "$get_old" > "$RUN_DIR/step6_get_old_token.json"
  if printf '%s\n' "$get_old" | grep -F "\"id\": \"$id_a\"" >/dev/null; then
    fail "old token still retrieves updated memory A"
  fi

  get_new=$(run_mem get "$token_c")
  printf '%s\n' "$get_new" > "$RUN_DIR/step7_get_new_token.json"
  printf '%s\n' "$get_new" | grep -F "\"id\": \"$id_a\"" >/dev/null || fail "new token did not retrieve updated memory A"

  cat > "$RUN_DIR/cli_report.txt" <<TXT
status: PASS
run_id: $RUN_ID
thread: $THREAD_ID
sandbox_repo: $SANDBOX_REPO
sandbox_data_dir: $SANDBOX_DATA_DIR
memory_a: $id_a
memory_b: $id_b
assertions:
- add memory A/B succeeded
- link relation persisted and appears in retrieval link_trail
- update replaced token and retrieval switched from old token to new token
TXT

  log "CLI suite PASS"
  log "CLI evidence: $RUN_DIR"
}

run_mcp_suite() {
  need_cmd go
  mkdir -p "$RUN_DIR"

  local mcp_log="$RUN_DIR/mcp_e2e.log"

  # Single source of truth: use the same MEMPACK_DATA_DIR as CLI suite.
  run_mem init --no-agents >/dev/null

  (
    cd "$PROJECT_ROOT"
    MEMPACK_DATA_DIR="$SANDBOX_DATA_DIR" \
    XDG_CONFIG_HOME="$SANDBOX_XDG_CONFIG" \
    XDG_CACHE_HOME="$SANDBOX_XDG_CACHE" \
    MEM_BIN="$MEM_BIN" \
    REPO_DIR="$SANDBOX_REPO" \
    QUERY="sandbox-memory-eval-$RUN_ID" \
    go run ./scripts/mcp_e2e_client.go
  ) >"$mcp_log" 2>&1

  log "MCP suite PASS"
  log "MCP evidence: $mcp_log"
}

usage() {
  cat <<'TXT'
Usage:
  scripts/memory_sandbox_eval.sh setup
  scripts/memory_sandbox_eval.sh cli
  scripts/memory_sandbox_eval.sh mcp
  scripts/memory_sandbox_eval.sh all

Environment overrides:
  MEM_BIN                    mem binary path (default: mem)
  MEMORY_SANDBOX_ROOT        root dir (default: <repo>/sandbox/memory-eval)
  MEMORY_SANDBOX_REPO        sandbox git repo path
  MEMORY_SANDBOX_DATA_DIR    single mempack data root for both CLI and MCP
  MEMORY_SANDBOX_XDG_CONFIG  isolated config dir for sandbox runs
  MEMORY_SANDBOX_XDG_CACHE   isolated cache dir for sandbox runs
  MEMORY_SANDBOX_EVIDENCE_DIR evidence output directory
  MEMORY_SANDBOX_THREAD      thread used by CLI suite
TXT
}

cmd="${1:-all}"
case "$cmd" in
  setup)
    setup_sandbox
    ;;
  cli)
    setup_sandbox
    run_cli_suite
    ;;
  mcp)
    setup_sandbox
    run_mcp_suite
    ;;
  all)
    setup_sandbox
    run_cli_suite
    run_mcp_suite
    ;;
  -h|--help|help)
    usage
    ;;
  *)
    usage
    fail "unknown command: $cmd"
    ;;
esac
