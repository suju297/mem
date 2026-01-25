#!/usr/bin/env bash
set -euo pipefail

VERSION="${VERSION:-v0.2.0}"
COMMIT="${COMMIT:-}"

if [[ -z "$COMMIT" ]] && command -v git >/dev/null 2>&1; then
  COMMIT="$(git rev-parse --short HEAD || true)"
fi
if [[ -z "$COMMIT" ]]; then
  COMMIT="dev"
fi

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DIST_DIR="$ROOT_DIR/dist/$VERSION"
BIN_NAME="mem"

mkdir -p "$DIST_DIR"

targets=(
  "darwin/amd64"
  "darwin/arm64"
  "linux/amd64"
  "linux/arm64"
)

for target in "${targets[@]}"; do
  IFS="/" read -r os arch <<<"$target"
  pkgdir="$DIST_DIR/mempack_${os}_${arch}"
  rm -rf "$pkgdir"
  mkdir -p "$pkgdir"

  echo "Building $os/$arch..."
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" \
    go build -trimpath -ldflags "-s -w -X mempack/internal/app.Version=$VERSION -X mempack/internal/app.Commit=$COMMIT" \
    -o "$pkgdir/$BIN_NAME" ./cmd/mem

  if [[ -f "$ROOT_DIR/ReadMe.md" ]]; then
    cp "$ROOT_DIR/ReadMe.md" "$pkgdir/README.md"
  fi
  if [[ -f "$ROOT_DIR/LICENSE" ]]; then
    cp "$ROOT_DIR/LICENSE" "$pkgdir/LICENSE"
  fi

  tarball="$DIST_DIR/mempack_${os}_${arch}.tar.gz"
  tar -czf "$tarball" -C "$pkgdir" .
done

DIST_DIR="$DIST_DIR" python3 - <<'PY'
import hashlib
import os
from pathlib import Path

dist = Path(os.environ["DIST_DIR"])
checksums = []
for path in sorted(dist.glob("mempack_*.tar.gz")):
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(1024 * 1024), b""):
            h.update(chunk)
    checksums.append((h.hexdigest(), path.name))

with open(dist / "checksums.txt", "w", encoding="utf-8") as f:
    for digest, name in checksums:
        f.write(f"{digest}  {name}\n")
PY

echo "Artifacts in: $DIST_DIR"
