package cli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/chartertrace/ast_dst/golang/gen"
)

// RunDoctor reports whether the TLA+ verification toolchain (a JVM and
// tla2tools.jar) is wired up, so a user can see the dependency state up front
// instead of discovering it mid-run. It is purely diagnostic: it never
// downloads, never fails the build, and exits non-zero only so scripts/CI can
// gate on a ready toolchain. `astdst generate` works regardless — without the
// toolchain it still synthesises the spec, just marked unverified.
func RunDoctor(args []string) {
	fs := flag.NewFlagSet("astdst doctor", flag.ExitOnError)
	configPath := fs.String("config", "", "JSON config file (default: built-in sim preset)")
	root := fs.String("root", "", "codebase root (used only to locate the spec dir for the jar search)")
	jar := fs.String("jar", "", "path to tla2tools.jar (else $TLA2TOOLS_JAR or beside the spec)")
	_ = fs.Parse(args)

	fmt.Fprintln(os.Stderr, "astdst doctor: checking the TLA+ verification toolchain")

	ok := true

	// Java.
	java, err := gen.FindJava()
	if err != nil {
		ok = false
		fmt.Fprintf(os.Stderr, "  ✗ java        %v\n", err)
		fmt.Fprintln(os.Stderr, "                install a JRE (Java 11+) or set $JAVA_HOME")
	} else {
		fmt.Fprintf(os.Stderr, "  ✓ java        %s%s\n", java, javaVersion(java))
	}

	// tla2tools.jar — search the same dirs `generate` does (the configured spec dir).
	searchDir := ""
	if cfg := loadConfig(*configPath, *root); cfg.TLA.SpecDir != "" {
		searchDir = cfg.TLA.SpecDir
		if !filepath.IsAbs(searchDir) {
			if absRoot, aerr := filepath.Abs(cfg.Root); aerr == nil {
				searchDir = filepath.Join(absRoot, searchDir)
			}
		}
	}
	jarPath, err := gen.FindJar(*jar, searchDir)
	if err != nil {
		ok = false
		fmt.Fprintf(os.Stderr, "  ✗ tla2tools   %v\n", err)
		fmt.Fprintln(os.Stderr, "                download tla2tools.jar and set $TLA2TOOLS_JAR, pass --jar,")
		fmt.Fprintln(os.Stderr, "                or place it beside the configured spec dir")
	} else {
		fmt.Fprintf(os.Stderr, "  ✓ tla2tools   %s\n", jarPath)
	}

	if ok {
		fmt.Fprintln(os.Stderr, "astdst doctor: ✓ toolchain ready — `astdst generate` will verify with SANY + TLC")
		return
	}
	fmt.Fprintln(os.Stderr, "astdst doctor: ✗ toolchain incomplete — `astdst generate` still works, but the spec will be UNVERIFIED")
	os.Exit(1)
}

// javaVersion returns a " (…)" suffix with the reported version, or "" if it
// can't be determined. `java -version` prints to stderr; we keep only the first
// line. Best-effort and never fatal.
func javaVersion(java string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, java, "-version").CombinedOutput()
	if err != nil {
		return ""
	}
	line := strings.SplitN(strings.TrimSpace(string(out)), "\n", 2)[0]
	if line == "" {
		return ""
	}
	return " (" + line + ")"
}
