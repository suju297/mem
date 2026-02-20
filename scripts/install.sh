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

if command -v curl >/dev/null 2>&1; then
  curl -fsSL -o "$archive" "$url"
elif command -v wget >/dev/null 2>&1; then
  wget -qO "$archive" "$url"
else
  echo "error: curl or wget required" >&2
  exit 1
fi

if [[ "$VERIFY_CHECKSUMS" != "0" ]]; then
  checksums_path="$tmpdir/checksums.txt"
  if command -v curl >/dev/null 2>&1; then
    if ! curl -fsSL -o "$checksums_path" "$checksums_url"; then
      checksums_path=""
    fi
  elif command -v wget >/dev/null 2>&1; then
    if ! wget -qO "$checksums_path" "$checksums_url"; then
      checksums_path=""
    fi
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

mkdir -p "$INSTALL_DIR"
if [[ "$os" == "windows" ]]; then
  cp "$bin_path" "$INSTALL_DIR/$bin_file"
else
  install -m 0755 "$bin_path" "$INSTALL_DIR/$bin_file"
fi

echo "Installed $bin_file to $INSTALL_DIR/$bin_file"
