#!/usr/bin/env bash
set -euo pipefail

if ! command -v python3 >/dev/null 2>&1; then
  echo "python3 is required" >&2
  exit 1
fi

json_get() {
  local key="$1"
  python3 -c 'import json,sys; print(json.load(sys.stdin).get(sys.argv[1], ""))' "$key"
}

json_len() {
  local key="$1"
  python3 -c 'import json,sys; obj=json.load(sys.stdin); key=sys.argv[1]; print(len(obj) if isinstance(obj, list) else len(obj.get(key, [])))' "$key"
}

bench_get() {
  local label="$1"
  local query="$2"
  local runs="${3:-21}"
  LABEL="$label" QUERY="$query" RUNS="$runs" BIN="$bin" python3 - <<'PY'
import os
import subprocess
import time

query = os.environ["QUERY"]
runs = int(os.environ.get("RUNS", "21"))
bin_path = os.environ["BIN"]

times = []
for _ in range(runs):
    start = time.perf_counter()
    subprocess.run([bin_path, "get", query], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, check=True)
    times.append((time.perf_counter() - start) * 1000.0)

cold = times[0] if times else 0.0
warm = times[1:] if len(times) > 1 else []
warm_sorted = sorted(warm)

def pct(vals, p):
    if not vals:
        return 0.0
    idx = int(round((p / 100.0) * (len(vals) - 1)))
    return vals[idx]

p50 = pct(warm_sorted, 50)
p95 = pct(warm_sorted, 95)

print(f"Bench {os.environ['LABEL']}: cold_ms={cold:.2f} warm_p50_ms={p50:.2f} warm_p95_ms={p95:.2f} runs={runs}")
PY
}

bin_dir=$(mktemp -d)
bin="$bin_dir/mem"
go build -o "$bin" ./cmd/mem

# Test 1: large ingest + retrieval
{
  echo "Running Test 1: large ingest + retrieval"
  tmpdir=$(mktemp -d)
  export XDG_CONFIG_HOME="$tmpdir/xdg/config"
  export XDG_DATA_HOME="$tmpdir/xdg/data"
  export XDG_CACHE_HOME="$tmpdir/xdg/cache"

  repo="$tmpdir/repo"
  mkdir -p "$repo"
  git -C "$repo" init -q
  git -C "$repo" config user.name "Test"
  git -C "$repo" config user.email "test@example.com"
  echo "init" > "$repo/README.md"
  git -C "$repo" add README.md
  git -C "$repo" commit -m "init" -q

  mkdir -p "$repo/docs"
  REPO_DOCS="$repo/docs" python3 - <<'PY'
import os
root = os.environ["REPO_DOCS"]
for i in range(200):
    path = os.path.join(root, f"doc_{i}.md")
    with open(path, "w", encoding="utf-8") as f:
        f.write("token " * 50 + f"\nfile {i}\n")
PY

  cd "$repo"

  ingest_out=$("$bin" ingest-artifact --thread T1 "$repo/docs")
  files_ingested=$(printf '%s' "$ingest_out" | json_get files_ingested)
  chunks_added=$(printf '%s' "$ingest_out" | json_get chunks_added)

  "$bin" get "token" > "$tmpdir/get1.json"
  get_memories=$(python3 -c 'import json,sys; j=json.load(open(sys.argv[1])); print(len(j.get("top_memories", [])))' "$tmpdir/get1.json")
  get_chunks=$(python3 -c 'import json,sys; j=json.load(open(sys.argv[1])); print(len(j.get("top_chunks", [])))' "$tmpdir/get1.json")
  echo "Test 1 results: files_ingested=$files_ingested chunks_added=$chunks_added get_memories=$get_memories get_chunks=$get_chunks"
  bench_get "test1 token" "token" 21
}

# Test 2: git reachability filter
{
  echo "Running Test 2: git reachability filter"
  tmpdir=$(mktemp -d)
  export XDG_CONFIG_HOME="$tmpdir/xdg/config"
  export XDG_DATA_HOME="$tmpdir/xdg/data"
  export XDG_CACHE_HOME="$tmpdir/xdg/cache"

  repo="$tmpdir/repo"
  mkdir -p "$repo"
  git -C "$repo" init -q
  git -C "$repo" config user.name "Test"
  git -C "$repo" config user.email "test@example.com"

  echo "A" > "$repo/file.txt"
  git -C "$repo" add file.txt
  git -C "$repo" commit -m "A" -q
  shaA=$(git -C "$repo" rev-parse HEAD)

  cd "$repo"
  memA=$("$bin" add --thread T-reach --title "Reach A" --summary "Reachability test")
  idA=$(printf '%s' "$memA" | json_get id)

  echo "B" > "$repo/file.txt"
  git -C "$repo" add file.txt
  git -C "$repo" commit -m "B" -q
  shaB=$(git -C "$repo" rev-parse HEAD)

  memB=$("$bin" add --thread T-reach --title "Reach B" --summary "Reachability test")
  idB=$(printf '%s' "$memB" | json_get id)

  git -C "$repo" checkout -q "$shaA"

  out=$("$bin" get "Reachability test")
  if [ -z "$out" ]; then
    echo "Test 2 error: empty output from mem get" >&2
    exit 1
  fi
  printf '%s' "$out" | python3 -c 'import json,sys; j=json.load(sys.stdin); idA=sys.argv[1]; idB=sys.argv[2]; ids=[m.get("id") for m in j.get("top_memories", [])]; print(f"Test 2 results: mem_ids={ids} (expect only {idA})");
if idB in ids: raise SystemExit(1)' "$idA" "$idB"
}

# Test 3: stress search
{
  echo "Running Test 3: stress search (1k memories + ~5k chunks)"
  tmpdir=$(mktemp -d)
  export XDG_CONFIG_HOME="$tmpdir/xdg/config"
  export XDG_DATA_HOME="$tmpdir/xdg/data"
  export XDG_CACHE_HOME="$tmpdir/xdg/cache"

  repo="$tmpdir/repo"
  mkdir -p "$repo"
  git -C "$repo" init -q
  git -C "$repo" config user.name "Test"
  git -C "$repo" config user.email "test@example.com"
  echo "seed" > "$repo/file.txt"
  git -C "$repo" add file.txt
  git -C "$repo" commit -m "init" -q

  cd "$repo"

  for i in $(seq 1 1000); do
    "$bin" add --thread T-bulk --title "Note $i" --summary "bulk token $i" >/dev/null
  done

  bulk_file="$repo/bulk.txt"
  BULK_FILE="$bulk_file" python3 - <<'PY'
import os
path=os.environ["BULK_FILE"]
with open(path, "w", encoding="utf-8") as f:
    line = "token " * 10
    for _ in range(40000):
        f.write(line + "\n")
PY

  ingest_out=$("$bin" ingest-artifact --thread T-bulk --max-file-mb 10 --chunk-tokens 80 --overlap-tokens 10 "$bulk_file")
  chunks_added=$(printf '%s' "$ingest_out" | json_get chunks_added)

  "$bin" get "token" > "$tmpdir/get3.json"
  get_memories=$(python3 -c 'import json,sys; j=json.load(open(sys.argv[1])); print(len(j.get("top_memories", [])))' "$tmpdir/get3.json")
  get_chunks=$(python3 -c 'import json,sys; j=json.load(open(sys.argv[1])); print(len(j.get("top_chunks", [])))' "$tmpdir/get3.json")

  "$bin" get --debug "token" >/dev/null 2> "$tmpdir/get3.debug"
  echo "Test 3 debug timings:"
  cat "$tmpdir/get3.debug"

  db_path=$(find "$XDG_DATA_HOME/mem/repos" -name memory.db | head -n 1)
  db_size_kb=$(du -k "$db_path" | awk '{print $1}')

  echo "Test 3 results: memories=1000 chunks_added=$chunks_added get_memories=$get_memories get_chunks=$get_chunks db_size_kb=$db_size_kb"
  bench_get "test3 token" "token" 21
  bench_get "test3 multi" "auth middleware 401 invalid token" 21
  bench_get "test3 miss" "qwertydoesnotexist" 21
}
