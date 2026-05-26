#!/usr/bin/env bash
# Generate a TLA+ spec from a Go codebase and verify it with SANY + TLC.
#
# Thin wrapper over `astdst generate` that auto-provides tla2tools.jar, so
# verification "just works" without a manual download or $TLA2TOOLS_JAR export.
# Every flag is passed straight through to `astdst generate`.
#
#   scripts/generate-spec.sh [generate args...]
#
# Examples:
#   scripts/generate-spec.sh --root /path/to/sim --out-spec ./generated
#   scripts/generate-spec.sh --config presets/sim.json --out-spec ./generated
#
# The jar is cached under ${XDG_CACHE_HOME:-~/.cache}/ast_dst and downloaded at
# most once. Set $TLA2TOOLS_JAR to point at an existing copy and skip the cache.
#
# The version is pinned (not "latest") and the download is checksum-verified, so
# the build is reproducible and the output-string parsing in golang/gen/toolchain.go
# tracks a known tools release. Bump TLA_VERSION + TLA_SHA256 together to upgrade.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

TLA_VERSION="v1.8.0"
TLA_SHA256="71546dff3897a01b0ee4fa64135d9f5e9384d2b7e47b3cc20a16b655b0eb4f86"
CACHE_DIR="${XDG_CACHE_HOME:-$HOME/.cache}/ast_dst"
JAR="${TLA2TOOLS_JAR:-$CACHE_DIR/tla2tools.jar}"
URL="https://github.com/tlaplus/tlaplus/releases/download/${TLA_VERSION}/tla2tools.jar"

# sha256 of $JAR, portable across shasum (macOS) and sha256sum (Linux).
jar_sha256() {
  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
  else
    sha256sum "$1" | awk '{print $1}'
  fi
}

if [ ! -f "$JAR" ]; then
  echo "==> tla2tools.jar not found — downloading the TLA+ verifier ($TLA_VERSION) to $JAR"
  mkdir -p "$(dirname "$JAR")"
  curl -fsSL -o "$JAR" "$URL"
fi

GOT="$(jar_sha256 "$JAR")"
if [ "$GOT" != "$TLA_SHA256" ]; then
  echo "==> ERROR: tla2tools.jar checksum mismatch at $JAR" >&2
  echo "    expected $TLA_SHA256" >&2
  echo "    got      $GOT" >&2
  echo "    The cached jar is corrupt or a different version. Delete it and re-run," >&2
  echo "    or update TLA_VERSION/TLA_SHA256 in this script to match." >&2
  # Remove a jar we downloaded into the cache; never delete a user-supplied $TLA2TOOLS_JAR.
  if [ -z "${TLA2TOOLS_JAR:-}" ]; then rm -f "$JAR"; fi
  exit 1
fi
export TLA2TOOLS_JAR="$JAR"

echo "==> verifier: $TLA2TOOLS_JAR ($TLA_VERSION, sha256 ok)"
cd "$ROOT_DIR/golang"
exec go run ./cmd/astdst generate "$@"
