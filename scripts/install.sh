#!/usr/bin/env bash
set -euo pipefail

BIN_NAME="mem"
REPO="${MEM_REPO:-${MEMPACK_REPO:-}}"
VERSION="${MEM_VERSION:-${MEMPACK_VERSION:-latest}}"
INSTALL_DIR="${MEM_INSTALL_DIR:-${MEMPACK_INSTALL_DIR:-}}"
VERIFY_CHECKSUMS="${MEM_VERIFY_CHECKSUMS:-${MEMPACK_VERIFY_CHECKSUMS:-1}}"
ADD_TO_PATH="${MEM_ADD_TO_PATH:-${MEMPACK_ADD_TO_PATH:-0}}"
PATH_RC_FILE="${MEM_PATH_RC_FILE:-${MEMPACK_PATH_RC_FILE:-}}"

usage() {
  cat <<'EOF'
Install mem from GitHub Releases.

Usage:
  install.sh --repo <owner/repo> [--version <tag>] [--install-dir <dir>] [--bin-name <name>] [--add-to-path]

Environment overrides:
  MEM_REPO, MEM_VERSION, MEM_INSTALL_DIR, MEM_ADD_TO_PATH, MEM_PATH_RC_FILE
  Legacy aliases are still supported:
  MEMPACK_REPO, MEMPACK_VERSION, MEMPACK_INSTALL_DIR, MEMPACK_ADD_TO_PATH, MEMPACK_PATH_RC_FILE

Examples:
  ./install.sh --repo owner/mem
  MEM_REPO=owner/mem ./install.sh
  ./install.sh --repo owner/mem --version v0.2.0
  ./install.sh --repo owner/mem --add-to-path

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

select_path_rc() {
  if [[ -n "${PATH_RC_FILE}" ]]; then
    printf '%s\n' "${PATH_RC_FILE}"
    return
  fi
  local shell_name
  shell_name="$(basename "${SHELL:-}")"
  case "$shell_name" in
    zsh)
      printf '%s\n' "${HOME}/.zshrc"
      ;;
    bash)
      if [[ "${os:-}" == "darwin" ]]; then
        printf '%s\n' "${HOME}/.bash_profile"
      else
        printf '%s\n' "${HOME}/.bashrc"
      fi
      ;;
    fish)
      printf '%s\n' "${HOME}/.config/fish/config.fish"
      ;;
    *)
      printf '%s\n' "${HOME}/.profile"
      ;;
  esac
}

persist_path_entry() {
  local install_dir="$1"
  local rc_file
  rc_file="$(select_path_rc)"
  mkdir -p "$(dirname "$rc_file")"
  if [[ -f "$rc_file" ]] && grep -F "$install_dir" "$rc_file" >/dev/null 2>&1; then
    echo "PATH already configured in $rc_file"
    return 0
  fi

  local shell_name line
  shell_name="$(basename "${SHELL:-}")"
  if [[ "$shell_name" == "fish" ]] || [[ "$rc_file" == *"/config.fish" ]]; then
    line="set -gx PATH \"$install_dir\" \$PATH"
  else
    line="export PATH=\"$install_dir:\$PATH\""
  fi

  {
    echo ""
    echo "# mem installer PATH update"
    echo "$line"
  } >> "$rc_file"

  echo "Added PATH entry to $rc_file"
  echo "Open a new terminal or source the file to apply changes."
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
    --add-to-path)
      ADD_TO_PATH="1"
      shift
      ;;
    --path-rc-file)
      PATH_RC_FILE="$2"
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
  echo "error: repo not set. Use --repo <owner/repo> or set MEM_REPO." >&2
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

asset="mem_${os}_${arch}.${archive_ext}"
legacy_asset="mempack_${os}_${arch}.${archive_ext}"
if [[ "$VERSION" == "latest" ]]; then
  url="https://github.com/${REPO}/releases/latest/download/${asset}"
  legacy_url="https://github.com/${REPO}/releases/latest/download/${legacy_asset}"
  checksums_url="https://github.com/${REPO}/releases/latest/download/checksums.txt"
else
  url="https://github.com/${REPO}/releases/download/${VERSION}/${asset}"
  legacy_url="https://github.com/${REPO}/releases/download/${VERSION}/${legacy_asset}"
  checksums_url="https://github.com/${REPO}/releases/download/${VERSION}/checksums.txt"
fi

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT
archive="$tmpdir/$asset"
asset_used="$asset"
downloaded_release=0
if download_file "$url" "$archive"; then
  downloaded_release=1
else
  archive="$tmpdir/$legacy_asset"
  asset_used="$legacy_asset"
  if download_file "$legacy_url" "$archive"; then
    downloaded_release=1
    echo "warning: using legacy release asset name (${legacy_asset})" >&2
  else
    echo "warning: release asset not found (${asset}, ${legacy_asset}); falling back to source build" >&2
  fi
fi

if [[ "$downloaded_release" == "1" ]] && [[ "$VERIFY_CHECKSUMS" != "0" ]]; then
  checksums_path="$tmpdir/checksums.txt"
  if ! download_file "$checksums_url" "$checksums_path"; then
    checksums_path=""
  fi

  if [[ -n "${checksums_path:-}" ]] && [[ -f "$checksums_path" ]]; then
    expected="$(awk -v file="$asset_used" '$2==file {print $1}' "$checksums_path")"
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
        echo "error: checksum mismatch for $asset_used" >&2
        exit 1
      fi
    else
      echo "warning: checksum entry not found for $asset_used; skipping verification" >&2
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
      echo "error: unzip, bsdtar, or powershell.exe required to extract $asset_used" >&2
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
case ":$PATH:" in
  *":$INSTALL_DIR:"*)
    if [[ "$ADD_TO_PATH" == "1" ]]; then
      persist_path_entry "$INSTALL_DIR"
    fi
    ;;
  *)
    if [[ "$ADD_TO_PATH" == "1" ]]; then
      if [[ "$os" == "windows" ]]; then
        echo "warning: persistent PATH edits are not supported from install.sh on Windows."
        echo "Use scripts/install.ps1 for Windows PATH updates."
      else
        persist_path_entry "$INSTALL_DIR"
      fi
    else
      echo "Add $INSTALL_DIR to PATH to run '$BIN_NAME' without full path."
      echo "Quick option: rerun with --add-to-path"
      echo "Manual option: export PATH=\"$INSTALL_DIR:\$PATH\""
    fi
    ;;
esac
