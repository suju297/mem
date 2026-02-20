#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION="${VERSION:-}"
COMMIT="${COMMIT:-}"

if [[ -z "$VERSION" ]] && command -v git >/dev/null 2>&1; then
  VERSION="$(git describe --tags --dirty --always 2>/dev/null || true)"
fi
if [[ -z "$VERSION" ]] && command -v python3 >/dev/null 2>&1 && [[ -f "$ROOT_DIR/extensions/vscode-mempack/package.json" ]]; then
  VERSION="v$(python3 - <<'PY'
import json, os
path = os.path.join(os.environ["ROOT_DIR"], "extensions", "vscode-mempack", "package.json")
with open(path, "r", encoding="utf-8") as f:
    print(json.load(f).get("version", "").strip())
PY
)"
fi
if [[ -z "$VERSION" || "$VERSION" == "v" ]]; then
  VERSION="v0.0.0-dev"
fi

if [[ -z "$COMMIT" ]] && command -v git >/dev/null 2>&1; then
  COMMIT="$(git rev-parse --short HEAD 2>/dev/null || true)"
fi
if [[ -z "$COMMIT" ]]; then
  COMMIT="dev"
fi
DIST_DIR="$ROOT_DIR/dist/$VERSION"
BIN_NAME="mem"

mkdir -p "$DIST_DIR"

targets=(
  "darwin/amd64"
  "darwin/arm64"
  "linux/amd64"
  "linux/arm64"
  "windows/amd64"
  "windows/arm64"
)

for target in "${targets[@]}"; do
  IFS="/" read -r os arch <<<"$target"
  pkgdir="$DIST_DIR/mempack_${os}_${arch}"
  bin_file="$BIN_NAME"
  asset="$DIST_DIR/mempack_${os}_${arch}.tar.gz"
  if [[ "$os" == "windows" ]]; then
    bin_file="${BIN_NAME}.exe"
    asset="$DIST_DIR/mempack_${os}_${arch}.zip"
  fi
  rm -rf "$pkgdir"
  mkdir -p "$pkgdir"

  echo "Building $os/$arch..."
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" \
    go build -trimpath -ldflags "-s -w -X mempack/internal/app.Version=$VERSION -X mempack/internal/app.Commit=$COMMIT" \
    -o "$pkgdir/$bin_file" ./cmd/mem

  if [[ -f "$ROOT_DIR/ReadMe.md" ]]; then
    cp "$ROOT_DIR/ReadMe.md" "$pkgdir/README.md"
  fi
  if [[ -f "$ROOT_DIR/LICENSE" ]]; then
    cp "$ROOT_DIR/LICENSE" "$pkgdir/LICENSE"
  fi

  if [[ "$os" == "windows" ]]; then
    PKG_DIR="$pkgdir" ASSET_PATH="$asset" python3 - <<'PY'
import os
from pathlib import Path
from zipfile import ZIP_DEFLATED, ZipFile

pkg_dir = Path(os.environ["PKG_DIR"])
asset_path = Path(os.environ["ASSET_PATH"])
with ZipFile(asset_path, "w", compression=ZIP_DEFLATED) as zf:
    for item in sorted(pkg_dir.rglob("*")):
        if item.is_file():
            zf.write(item, item.relative_to(pkg_dir))
PY
  else
    tar -czf "$asset" -C "$pkgdir" .
  fi
done

DIST_DIR="$DIST_DIR" python3 - <<'PY'
import hashlib
import os
from pathlib import Path

dist = Path(os.environ["DIST_DIR"])
checksums = []
assets = sorted(list(dist.glob("mempack_*.tar.gz")) + list(dist.glob("mempack_*.zip")))
for path in assets:
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
