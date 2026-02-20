#!/usr/bin/env bash
set -euo pipefail

BIN_NAME="mem"
REPO="${MEMPACK_REPO:-}"
VERSION="${MEMPACK_VERSION:-latest}"
INSTALL_DIR="${MEMPACK_INSTALL_DIR:-}"
VERIFY_CHECKSUMS="${MEMPACK_VERIFY_CHECKSUMS:-1}"

usage() {
  cat <<'EOF'
Install mempack from GitHub Releases.

Usage:
  install.sh --repo <owner/repo> [--version <tag>] [--install-dir <dir>] [--bin-name <name>]

Environment overrides:
  MEMPACK_REPO, MEMPACK_VERSION, MEMPACK_INSTALL_DIR

Examples:
  ./install.sh --repo owner/mempack
  MEMPACK_REPO=owner/mempack ./install.sh
  ./install.sh --repo owner/mempack --version v0.2.0

Notes:
  - This script supports macOS, Linux, and Git-Bash/MSYS/Cygwin on Windows.
  - Native PowerShell installer: scripts/install.ps1
EOF
}

download_file() {
  local src="$1"
  local dst="$2"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL -o "$dst" "$src"
  elif command -v wget >/dev/null 2>&1; then
    wget -qO "$dst" "$src"
  else
    echo "error: curl or wget required" >&2
    return 1
  fi
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo)
      REPO="$2"
      shift 2
      ;;
    --version)
      VERSION="$2"
      shift 2
      ;;
    --install-dir)
      INSTALL_DIR="$2"
      shift 2
      ;;
    --bin-name)
      BIN_NAME="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "unknown arg: $1" >&2
      usage
      exit 1
      ;;
  esac
done

if [[ -z "$REPO" ]] && command -v git >/dev/null 2>&1; then
  origin="$(git config --get remote.origin.url || true)"
  if [[ -n "$origin" ]]; then
    if [[ "$origin" == git@github.com:* ]]; then
      origin="${origin#git@github.com:}"
    elif [[ "$origin" == https://github.com/* ]]; then
      origin="${origin#https://github.com/}"
    fi
    origin="${origin%.git}"
    if [[ "$origin" == */* ]]; then
      REPO="$origin"
    fi
  fi
fi

if [[ -z "$REPO" ]]; then
  echo "error: repo not set. Use --repo <owner/repo> or set MEMPACK_REPO." >&2
  exit 1
fi

os_raw="$(uname -s | tr '[:upper:]' '[:lower:]')"
os="$os_raw"
archive_ext="tar.gz"
bin_file="$BIN_NAME"
case "$os_raw" in
  darwin|linux) ;;
  msys*|mingw*|cygwin*)
    os="windows"
    archive_ext="zip"
    bin_file="${BIN_NAME}.exe"
    ;;
  *)
    echo "unsupported OS: $os_raw" >&2
    exit 1
    ;;
esac

arch="$(uname -m)"
case "$arch" in
  x86_64|amd64) arch="amd64" ;;
  arm64|aarch64) arch="arm64" ;;
  *)
    echo "unsupported arch: $arch" >&2
    exit 1
    ;;
esac

if [[ -z "$INSTALL_DIR" ]]; then
  if [[ "$os" == "windows" ]]; then
    INSTALL_DIR="${USERPROFILE:-$HOME}/bin"
  else
    INSTALL_DIR="${HOME}/.local/bin"
  fi
fi

asset="mempack_${os}_${arch}.${archive_ext}"
if [[ "$VERSION" == "latest" ]]; then
  url="https://github.com/${REPO}/releases/latest/download/${asset}"
  checksums_url="https://github.com/${REPO}/releases/latest/download/checksums.txt"
else
  url="https://github.com/${REPO}/releases/download/${VERSION}/${asset}"
  checksums_url="https://github.com/${REPO}/releases/download/${VERSION}/checksums.txt"
fi

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT
archive="$tmpdir/$asset"
downloaded_release=0
if download_file "$url" "$archive"; then
  downloaded_release=1
else
  echo "warning: release asset not found (${asset}); falling back to source build" >&2
fi

if [[ "$downloaded_release" == "1" ]] && [[ "$VERIFY_CHECKSUMS" != "0" ]]; then
  checksums_path="$tmpdir/checksums.txt"
  if ! download_file "$checksums_url" "$checksums_path"; then
    checksums_path=""
  fi

  if [[ -n "${checksums_path:-}" ]] && [[ -f "$checksums_path" ]]; then
    expected="$(awk -v file="$asset" '$2==file {print $1}' "$checksums_path")"
    if [[ -n "$expected" ]]; then
      if command -v sha256sum >/dev/null 2>&1; then
        actual="$(sha256sum "$archive" | awk '{print $1}')"
      elif command -v shasum >/dev/null 2>&1; then
        actual="$(shasum -a 256 "$archive" | awk '{print $1}')"
      else
        actual=""
      fi
      if [[ -z "$actual" ]]; then
        echo "warning: sha256 tool not found; skipping checksum verification" >&2
      elif [[ "$actual" != "$expected" ]]; then
        echo "error: checksum mismatch for $asset" >&2
        exit 1
      fi
    else
      echo "warning: checksum entry not found for $asset; skipping verification" >&2
    fi
  else
    echo "warning: checksums.txt not found; skipping verification" >&2
  fi
fi

if [[ "$downloaded_release" == "1" ]]; then
  if [[ "$archive_ext" == "zip" ]]; then
    if command -v unzip >/dev/null 2>&1; then
      unzip -q "$archive" -d "$tmpdir"
    elif command -v bsdtar >/dev/null 2>&1; then
      bsdtar -xf "$archive" -C "$tmpdir"
    elif command -v powershell.exe >/dev/null 2>&1; then
      powershell.exe -NoProfile -Command \
        "Expand-Archive -Path '$archive' -DestinationPath '$tmpdir' -Force" >/dev/null
    else
      echo "error: unzip, bsdtar, or powershell.exe required to extract $asset" >&2
      exit 1
    fi
  else
    tar -xzf "$archive" -C "$tmpdir"
  fi

  bin_path="$tmpdir/$bin_file"
  if [[ ! -f "$bin_path" ]]; then
    bin_path="$(find "$tmpdir" -maxdepth 2 -type f -name "$bin_file" | head -n 1)"
  fi
  if [[ -z "$bin_path" ]] || [[ ! -f "$bin_path" ]]; then
    echo "error: binary $bin_file not found in archive" >&2
    exit 1
  fi
else
  if ! command -v go >/dev/null 2>&1; then
    echo "error: release asset unavailable and Go toolchain not found for source build fallback" >&2
    exit 1
  fi

  if [[ "$VERSION" == "latest" ]]; then
    source_ref="heads/main"
  else
    source_ref="tags/$VERSION"
  fi
  source_url="https://github.com/${REPO}/archive/refs/${source_ref}.tar.gz"
  source_archive="$tmpdir/source.tar.gz"
  if ! download_file "$source_url" "$source_archive"; then
    echo "error: unable to download source archive from $source_url" >&2
    exit 1
  fi

  tar -xzf "$source_archive" -C "$tmpdir"
  main_go="$(find "$tmpdir" -maxdepth 5 -type f -path "*/cmd/mem/main.go" | head -n 1)"
  if [[ -z "$main_go" ]]; then
    echo "error: source archive missing cmd/mem/main.go" >&2
    exit 1
  fi
  src_root="${main_go%/cmd/mem/main.go}"
  echo "Building $bin_file from source (${source_ref})..." >&2
  (
    cd "$src_root"
    CGO_ENABLED=0 go build -trimpath -o "$tmpdir/$bin_file" ./cmd/mem
  )
  bin_path="$tmpdir/$bin_file"
fi

mkdir -p "$INSTALL_DIR"
if [[ "$os" == "windows" ]]; then
  cp "$bin_path" "$INSTALL_DIR/$bin_file"
else
  install -m 0755 "$bin_path" "$INSTALL_DIR/$bin_file"
fi

echo "Installed $bin_file to $INSTALL_DIR/$bin_file"
