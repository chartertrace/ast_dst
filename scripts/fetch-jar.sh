#!/usr/bin/env bash
# Resolve a pinned, checksum-verified tla2tools.jar and print its absolute path
# on stdout (all diagnostics go to stderr, so callers can capture the path):
#
#   export TLA2TOOLS_JAR="$(scripts/fetch-jar.sh)"
#
# The jar is cached under ${XDG_CACHE_HOME:-~/.cache}/ast_dst and downloaded at
# most once. Set $TLA2TOOLS_JAR to point at an existing copy and skip the cache.
# This is the single source of the pinned version — generate-spec.sh and CI both
# call it, so the verifier stays reproducible. Bump TLA_VERSION + TLA_SHA256
# together to upgrade (the output-string parsing in golang/gen/toolchain.go
# tracks a known tools release).
set -euo pipefail

TLA_VERSION="v1.8.0"
TLA_SHA256="71546dff3897a01b0ee4fa64135d9f5e9384d2b7e47b3cc20a16b655b0eb4f86"
CACHE_DIR="${XDG_CACHE_HOME:-$HOME/.cache}/ast_dst"
JAR="${TLA2TOOLS_JAR:-$CACHE_DIR/tla2tools.jar}"
URL="https://github.com/tlaplus/tlaplus/releases/download/${TLA_VERSION}/tla2tools.jar"

# sha256 of $1, portable across shasum (macOS) and sha256sum (Linux).
jar_sha256() {
  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
  else
    sha256sum "$1" | awk '{print $1}'
  fi
}

if [ ! -f "$JAR" ]; then
  echo "==> tla2tools.jar not found — downloading the TLA+ verifier ($TLA_VERSION) to $JAR" >&2
  mkdir -p "$(dirname "$JAR")"
  curl -fsSL -o "$JAR" "$URL"
fi

GOT="$(jar_sha256 "$JAR")"
if [ "$GOT" != "$TLA_SHA256" ]; then
  {
    echo "ERROR: tla2tools.jar checksum mismatch at $JAR"
    echo "  expected $TLA_SHA256"
    echo "  got      $GOT"
    echo "  The cached jar is corrupt or a different version. Delete it and re-run,"
    echo "  or update TLA_VERSION/TLA_SHA256 in this script to match."
  } >&2
  # Remove a jar we downloaded into the cache; never delete a user-supplied $TLA2TOOLS_JAR.
  if [ -z "${TLA2TOOLS_JAR:-}" ]; then rm -f "$JAR"; fi
  exit 1
fi

echo "$JAR"
