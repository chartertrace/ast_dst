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
# Jar resolution (pinned version + checksum) lives in fetch-jar.sh, shared with
# CI so the verifier is reproducible everywhere.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

TLA2TOOLS_JAR="$("$SCRIPT_DIR/fetch-jar.sh")"
export TLA2TOOLS_JAR

echo "==> verifier: $TLA2TOOLS_JAR (sha256 ok)"
cd "$ROOT_DIR/golang"
exec go run ./cmd/astdst generate "$@"
