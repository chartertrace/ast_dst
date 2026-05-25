#!/usr/bin/env bash
# Run ast_dst against an arbitrary public Go repository and build a static
# docs+viewer site for it.
#
#   scripts/run-on-repo.sh <git-url> [ref] [--out DIR] [--exported-only]
#
#   <git-url>          e.g. https://github.com/cockroachdb/cockroach
#   [ref]              optional branch/tag/sha (default: the remote's default HEAD)
#   --out DIR          output site directory (default: ./ast_dst-site)
#   --exported-only    drop unexported declarations in structure mode
#
# ast_dst is a go/ast tool: it only understands Go source. A non-Go repo yields
# an empty model (by design). The script always runs the config-free `structure`
# mode, and additionally tries the default sim `extract` — using its richer DST
# model only if that repo actually has operations or a fault catalogue.
set -euo pipefail

if [ $# -lt 1 ]; then
  sed -n '2,16p' "$0" | sed 's/^# \{0,1\}//'
  exit 2
fi

URL="$1"; shift
REF=""
if [ $# -gt 0 ] && [ "${1#-}" = "$1" ]; then REF="$1"; shift; fi

OUT="./ast_dst-site"
EXPORTED_ONLY=""
while [ $# -gt 0 ]; do
  case "$1" in
    --out) OUT="$2"; shift 2 ;;
    --exported-only) EXPORTED_ONLY="--exported-only"; shift ;;
    *) echo "unknown flag: $1" >&2; exit 2 ;;
  esac
done

# Resolve repo paths relative to this script, so it runs from anywhere.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
OUT="$(mkdir -p "$OUT" && cd "$OUT" && pwd)"

WORK="$(mktemp -d)"
CLONE="$WORK/repo"
trap 'rm -rf "$WORK"' EXIT

echo "==> cloning $URL ${REF:+(ref: $REF)}"
if [ -n "$REF" ]; then
  git clone --depth 1 --branch "$REF" "$URL" "$CLONE" 2>/dev/null \
    || git clone "$URL" "$CLONE" # fall back: ref may be a sha, not a branch/tag
  git -C "$CLONE" checkout --quiet "$REF" 2>/dev/null || true
else
  git clone --depth 1 "$URL" "$CLONE"
fi
REF="$(git -C "$CLONE" rev-parse HEAD)"

# Clickable file:line links, only for github.com remotes.
NORM="${URL%.git}"; NORM="${NORM/git@github.com:/https://github.com/}"
REPO_BASE=""
case "$NORM" in
  https://github.com/*) REPO_BASE="$NORM/blob/$REF" ;;
esac

echo "==> structure extraction (config-free, works on any Go module)"
( cd "$ROOT_DIR/golang" \
  && go run ./cmd/astdst structure --root "$CLONE" $EXPORTED_ONLY --out "$WORK/structure.json" )

echo "==> trying sim extraction (default preset; used only if it finds ops/faults)"
SIM_SUMMARY="$( cd "$ROOT_DIR/golang" \
  && go run ./cmd/astdst --root "$CLONE" --out "$WORK/sim.json" 2>&1 >/dev/null )" || true
OPS="$(printf '%s' "$SIM_SUMMARY" | sed -n 's/.*, \([0-9]*\) operations.*/\1/p')"
FAULTS="$(printf '%s' "$SIM_SUMMARY" | sed -n 's/.*, \([0-9]*\) faults.*/\1/p')"
OPS="${OPS:-0}"; FAULTS="${FAULTS:-0}"

if [ "$OPS" -gt 0 ] || [ "$FAULTS" -gt 0 ]; then
  MODEL="$WORK/sim.json"
  echo "    sim-shaped repo: $OPS operations, $FAULTS faults — using the DST model"
else
  MODEL="$WORK/structure.json"
  echo "    not sim-shaped — using the generic structure model"
fi

echo "==> building docs+viewer site -> $OUT"
cd "$ROOT_DIR/typescript"
[ -d node_modules ] || npm install
npm run gen:docs -- --in "$MODEL" --out "$OUT" ${REPO_BASE:+--repo-base "$REPO_BASE"}

echo
echo "Done. Open: $OUT/index.html"
