#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: render_homebrew_formula.sh --version <vX.Y.Z> --commit <sha> --sha256 <digest> --output <path> [--readme <path>]

Renders the mem-cli Homebrew formula for a tagged release and optionally updates
the tap README's "Current packaged release" line.
EOF
}

VERSION=""
COMMIT=""
SHA256=""
OUTPUT=""
README=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --version)
      VERSION="${2:-}"
      shift 2
      ;;
    --commit)
      COMMIT="${2:-}"
      shift 2
      ;;
    --sha256)
      SHA256="${2:-}"
      shift 2
      ;;
    --output)
      OUTPUT="${2:-}"
      shift 2
      ;;
    --readme)
      README="${2:-}"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "unknown argument: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

if [[ -z "$VERSION" || -z "$COMMIT" || -z "$SHA256" || -z "$OUTPUT" ]]; then
  echo "missing required arguments" >&2
  usage >&2
  exit 1
fi

if [[ "$VERSION" != v* ]]; then
  VERSION="v$VERSION"
fi

if [[ ! "$SHA256" =~ ^[0-9a-f]{64}$ ]]; then
  echo "sha256 must be a 64-character lowercase hex digest" >&2
  exit 1
fi

FORMULA_VERSION="${VERSION#v}"
SHORT_COMMIT="$(printf '%s' "$COMMIT" | cut -c1-7)"

mkdir -p "$(dirname "$OUTPUT")"

cat >"$OUTPUT" <<EOF
class MemCli < Formula
  desc "Repo-scoped memory for coding agents"
  homepage "https://github.com/suju297/mem"
  url "https://github.com/suju297/mem/archive/refs/tags/${VERSION}.tar.gz"
  sha256 "${SHA256}"
  license "MIT"

  depends_on "go" => :build

  def install
    commit = "${SHORT_COMMIT}"
    ldflags = %W[
      -s -w
      -X mem/internal/app.Version=#{version}
      -X mem/internal/app.Commit=#{commit}
    ]

    system "go", "build", *std_go_args(output: bin/"mem", ldflags: ldflags), "./cmd/mem"
  end

  test do
    assert_match "mem v#{version}", shell_output("#{bin}/mem --version")
  end
end
EOF

if [[ -n "$README" && -f "$README" ]]; then
  tmp_file="$(mktemp)"
  awk -v version="$VERSION" '
    /^- Current packaged release:/ {
      print "- Current packaged release: `" version "`"
      next
    }
    { print }
  ' "$README" >"$tmp_file"
  mv "$tmp_file" "$README"
fi
