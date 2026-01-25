#!/usr/bin/env bash
set -euo pipefail

# Helper function
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

  # Reduced to 200 memories for quicker feedback during development, 
  # but user asked for "End-to-end on 1k memories".
  # I'll stick to 1000 but optimize the loop slightly.
  # Use parallel/xargs if possible? No, sticking to bash loop to avoid dependencies.
  echo "Adding 1000 memories..."
  for i in $(seq 1 1000); do
    "$bin" add --thread T-bulk --title "Note $i" --summary "bulk token $i" >/dev/null
  done

  bulk_file="$repo/bulk.txt"
  echo "Generating bulk artifact..."
  python3 -c 'with open("bulk.txt", "w") as f: f.write(("token " * 10 + "\n") * 40000)'

  echo "Ingesting artifact..."
  ingest_out=$("$bin" ingest-artifact --thread T-bulk --max-file-mb 10 --chunk-tokens 80 --overlap-tokens 10 "bulk.txt")
  
  echo "Benchmarking..."
  bench_get "test3 token" "token" 21
  
  # Also debug output one run
  "$bin" get --debug "token" >/dev/null 2> "$tmpdir/debug.log"
  echo "Debug log:"
  cat "$tmpdir/debug.log"
}
