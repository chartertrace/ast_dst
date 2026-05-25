// Command astdst walks a Go codebase and emits a JSON model of its faults,
// invariants, and weighted operations — along with the edges the source itself
// encodes — for the viewer in ../typescript, and can generate a TLC-verified
// TLA+ spec. The implementation lives in package cli, shared with the
// `go run ./cmd` entrypoint.
//
// The extractor is config-driven: with no --config it uses a built-in preset
// (the names CharterTrace's primary-server/sim happens to use), but it knows no
// codebase-specific names by default. Point it at any sim-shaped codebase with a
// --root and, if its catalogue/enum/trace names differ, a --config — see
// ../presets/sim.json for the schema.
//
//	go install github.com/chartertrace/ast_dst/golang/cmd/astdst@latest
//	astdst --root /path/to/repo                       # built-in preset names
//	astdst --root /path/to/repo --config my.json --out model.json
//	astdst generate --root /path/to/repo --out-spec ./generated
package main

import (
	"os"

	"github.com/chartertrace/ast_dst/golang/cli"
)

func main() { cli.Main(os.Args[1:]) }
