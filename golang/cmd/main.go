// Command astdst (the `go run ./cmd` entrypoint) emits a JSON model of a
// codebase's faults, invariants, and weighted operations for the viewer in
// ../typescript, and can generate a TLC-verified TLA+ spec. The implementation
// lives in package cli so this and the installable cmd/astdst binary stay in sync.
//
//	go run ./cmd                                   # extract: default preset + sim path
//	go run ./cmd --root /path/to/repo --out model.json
//	go run ./cmd generate --out-spec ./generated --out model.json
package main

import (
	"os"

	"github.com/chartertrace/ast_dst/golang/cli"
)

func main() { cli.Main(os.Args[1:]) }
