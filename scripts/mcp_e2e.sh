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

root="$(pwd)"
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

# Seed a memory so get_context has content
"$bin" add --thread T1 --title "Decision" --summary "Decision summary" >/dev/null

MEM_BIN="$bin" REPO_DIR="$repo" QUERY="decision" go -C "$root" run "scripts/mcp_e2e_client.go"
