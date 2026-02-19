#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION="${1:-}"

if [[ -z "$VERSION" ]]; then
  echo "Usage: $(basename "$0") <version>"
  echo "Examples: $(basename "$0") 0.2.3  |  $(basename "$0") v0.2.3"
  exit 1
fi

if [[ "$VERSION" != v* ]]; then
  VERSION="v$VERSION"
fi
EXT_VERSION="${VERSION#v}"

export ROOT_DIR VERSION EXT_VERSION
python3 - <<'PY'
import json
import os
import re
from pathlib import Path

root = Path(os.environ["ROOT_DIR"])
version = os.environ["VERSION"]
ext_version = os.environ["EXT_VERSION"]

version_path = root / "internal" / "app" / "version.go"
data = version_path.read_text(encoding="utf-8")
data = re.sub(r'Version\s*=\s*"v[^"]+"', f'Version = "{version}"', data)
version_path.write_text(data, encoding="utf-8")

def update_json(path: Path) -> None:
    obj = json.loads(path.read_text(encoding="utf-8"))
    obj["version"] = ext_version
    if path.name == "package-lock.json":
        packages = obj.get("packages")
        if isinstance(packages, dict) and "" in packages and isinstance(packages[""], dict):
            packages[""]["version"] = ext_version
    path.write_text(json.dumps(obj, indent=2) + "\n", encoding="utf-8")

update_json(root / "extensions" / "vscode-mempack" / "package.json")
update_json(root / "extensions" / "vscode-mempack" / "package-lock.json")
PY

gofmt -w "$ROOT_DIR/internal/app/version.go" >/dev/null 2>&1 || true

echo "Synced versions:"
echo "  CLI: $VERSION"
echo "  VSIX: $EXT_VERSION"
