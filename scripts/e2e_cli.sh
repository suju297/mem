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

echo "hello" > "$repo/file.txt"
git -C "$repo" add file.txt
git -C "$repo" commit -m "init" -q

export XDG_CONFIG_HOME="$scratch/xdg/config"
export XDG_DATA_HOME="$scratch/xdg/data"
export XDG_CACHE_HOME="$scratch/xdg/cache"

cd "$repo"

"$bin" init --no-agents >/dev/null

add1=$("$bin" add --thread T1 --title "First" --summary "Initial decision")
mem1=$(printf '%s' "$add1" | json_get id)

# Ensure distinct created_at ordering for recent tests.
sleep 1

add2=$("$bin" add --thread T1 --title "Second" --summary "Follow-up")
mem2=$(printf '%s' "$add2" | json_get id)

get_out=$("$bin" get "decision")
mem_count=$(printf '%s' "$get_out" | json_len top_memories)

threads_out=$("$bin" threads --format json)
threads_count=$(printf '%s' "$threads_out" | json_len '')

thread_out=$("$bin" thread T1 --limit 10 --format json)
thread_mem_count=$(printf '%s' "$thread_out" | json_len memories)

recent_out=$("$bin" recent --limit 1 --format json)
recent_id=$(printf '%s' "$recent_out" | "$PYTHON_BIN" -c 'import json,sys; data=json.load(sys.stdin); print(data[0].get("id", "")) if data else print("")')
recent_title=$(printf '%s' "$recent_out" | "$PYTHON_BIN" -c 'import json,sys; data=json.load(sys.stdin); print(data[0].get("title", "")) if data else print("")')

sup_out=$("$bin" supersede --title "Third" --summary "Superseded" "$mem1")
new_id=$(printf '%s' "$sup_out" | json_get new_id)

"$bin" forget "$new_id" >/dev/null

checkpoint_out=$("$bin" checkpoint --reason "Snapshot" --state-json '{"goal":"ship"}' --thread T1)
checkpoint_mem=$(printf '%s' "$checkpoint_out" | json_get memory_id)

ingest_out=$("$bin" ingest-artifact --thread T1 "$repo/file.txt")
chunks_added=$(printf '%s' "$ingest_out" | json_get chunks_added)

printf '%s\n' "e2e init: ok"
printf '%s\n' "e2e add/get: mem_count=$mem_count"
printf '%s\n' "e2e threads: count=$threads_count"
printf '%s\n' "e2e thread: memories=$thread_mem_count"
printf '%s\n' "e2e recent: id=$recent_id title=$recent_title"
printf '%s\n' "e2e supersede+forget: new_id=$new_id"
printf '%s\n' "e2e checkpoint: memory_id=$checkpoint_mem"
printf '%s\n' "e2e ingest: chunks_added=$chunks_added"

if [ "$mem_count" -lt 1 ]; then
  echo "e2e error: expected mem_count >= 1" >&2
  exit 1
fi
if [ "$threads_count" -lt 1 ]; then
  echo "e2e error: expected threads_count >= 1" >&2
  exit 1
fi
if [ "$thread_mem_count" -lt 1 ]; then
  echo "e2e error: expected thread_mem_count >= 1" >&2
  exit 1
fi
if [ "$recent_id" != "$mem2" ]; then
  echo "e2e error: expected recent id $mem2, got $recent_id" >&2
  exit 1
fi
if [ "$chunks_added" -lt 1 ]; then
  echo "e2e error: expected chunks_added >= 1" >&2
  exit 1
fi
