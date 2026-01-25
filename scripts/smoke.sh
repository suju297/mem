#!/usr/bin/env bash
set -euo pipefail

if ! command -v git >/dev/null 2>&1; then
  echo "git is required" >&2
  exit 1
fi
if ! command -v go >/dev/null 2>&1; then
  echo "go is required" >&2
  exit 1
fi

PYTHON_BIN=""
if command -v python3 >/dev/null 2>&1; then
  PYTHON_BIN="python3"
elif command -v python >/dev/null 2>&1; then
  PYTHON_BIN="python"
else
  echo "python3 or python is required for JSON parsing" >&2
  exit 1
fi

json_get() {
  local key="$1"
  "$PYTHON_BIN" -c 'import json,sys; obj=json.load(sys.stdin); print(obj.get(sys.argv[1], ""))' "$key"
}

json_len() {
  local key="$1"
  "$PYTHON_BIN" -c 'import json,sys; obj=json.load(sys.stdin); key=sys.argv[1]; print(len(obj) if isinstance(obj, list) else len(obj.get(key, [])))' "$key"
}

scratch=$(mktemp -d)
trap 'rm -rf "$scratch"' EXIT

bin="$scratch/mem"
go build -o "$bin" ./cmd/mem

repo="$scratch/repo"
mkdir -p "$repo"

git -C "$repo" init -q
git -C "$repo" config user.name "Test"
git -C "$repo" config user.email "test@example.com"

printf "hello\n" > "$repo/file.txt"
git -C "$repo" add file.txt
git -C "$repo" commit -m "init" -q

export XDG_CONFIG_HOME="$scratch/xdg/config"
export XDG_DATA_HOME="$scratch/xdg/data"
export XDG_CACHE_HOME="$scratch/xdg/cache"

cd "$repo"

add_out=$("$bin" add --thread T1 --title "First" --summary "Initial decision")
mem_id=$(printf '%s' "$add_out" | json_get id)

get_out=$("$bin" get "decision")
explain_out=$("$bin" explain "decision")
show_out=$("$bin" show "$mem_id")

sup_out=$("$bin" supersede --title "Second" --summary "Updated decision" "$mem_id")
new_id=$(printf '%s' "$sup_out" | json_get new_id)

forget_out=$("$bin" forget "$new_id")

checkpoint_out=$("$bin" checkpoint --reason "Snapshot" --state-json '{"goal":"ship"}' --thread T1)
checkpoint_mem=$(printf '%s' "$checkpoint_out" | json_get memory_id)

threads_out=$("$bin" threads)
thread_out=$("$bin" thread --limit 5 T1)
repos_out=$("$bin" repos)
use_out=$("$bin" use "$repo")
ingest_out=$("$bin" ingest-artifact --thread T1 "$repo/file.txt")

printf '%s\n' "add: ok" "  id=$mem_id"
printf '%s\n' "get: ok" "  memories=$(printf '%s' "$get_out" | json_len top_memories)" "  chunks=$(printf '%s' "$get_out" | json_len top_chunks)"
printf '%s\n' "explain: ok" "  memories=$(printf '%s' "$explain_out" | json_len memories)" "  chunks=$(printf '%s' "$explain_out" | json_len chunks)"
printf '%s\n' "show: ok" "  kind=$(printf '%s' "$show_out" | json_get kind)"
printf '%s\n' "supersede: ok" "  new_id=$new_id"
printf '%s\n' "forget: ok" "  status=$(printf '%s' "$forget_out" | json_get status)"
printf '%s\n' "checkpoint: ok" "  state_id=$(printf '%s' "$checkpoint_out" | json_get state_id)" "  memory_id=$checkpoint_mem"
printf '%s\n' "threads: ok" "  count=$(printf '%s' "$threads_out" | json_len '')"
printf '%s\n' "thread: ok" "  memories=$(printf '%s' "$thread_out" | json_len memories)"
printf '%s\n' "repos: ok" "  count=$(printf '%s' "$repos_out" | json_len '')"
printf '%s\n' "use: ok" "  active_repo=$(printf '%s' "$use_out" | json_get active_repo)"
printf '%s\n' "ingest: ok" "  files=$(printf '%s' "$ingest_out" | json_get files_ingested)" "  chunks=$(printf '%s' "$ingest_out" | json_get chunks_added)"
