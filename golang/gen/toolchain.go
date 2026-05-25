// Package gen turns an extracted codebase model into a TLA+ specification: it
// synthesises the spec deterministically from the model (no LLM, no network —
// the same model in yields the same spec out) and, when a JVM + tla2tools.jar
// are available, verifies the draft with the real TLA+ tools (SANY then TLC).
// Nothing is labelled "verified" unless TLC model-checked it clean, which keeps
// the project's honesty guarantee: a spec that does not hold — or that was never
// checked because the toolchain was absent — is reported as such, never massaged
// to look complete.
package gen

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Toolchain runs the bundled TLA+ tools (tla2tools.jar) through a JVM.
type Toolchain struct {
	Java string // path to the java binary
	Jar  string // path to tla2tools.jar
}

// Phase names which tool produced a Result.
type Phase string

const (
	PhaseSANY Phase = "sany" // syntax + semantic check (fast, no state space)
	PhaseTLC  Phase = "tlc"  // explicit-state model checking
)

// Result is the parsed outcome of one tool invocation.
type Result struct {
	Phase    Phase    `json:"phase"`
	OK       bool     `json:"ok"`                 // tool exited cleanly with no errors
	Errors   []string `json:"errors,omitempty"`   // parsed error lines, for repair feedback
	Violated string   `json:"violated,omitempty"` // invariant TLC reported violated
	Deadlock bool     `json:"deadlock,omitempty"`
	States   int      `json:"states,omitempty"` // distinct states explored (TLC)
	Depth    int      `json:"depth,omitempty"`  // search depth reached (TLC)
	ExitCode int      `json:"exitCode"`
	TimedOut bool     `json:"timedOut,omitempty"`
	Raw      string   `json:"-"` // full combined output, for the repair prompt
}

// TLCOptions bounds a model-checking run so a runaway state space cannot hang
// the pipeline. Timeouts are the caller's responsibility via the context.
type TLCOptions struct {
	Workers int // parallel workers; 0 => 1
}

// NewToolchain locates java and tla2tools.jar. The jar is resolved from (in
// order) the explicit jarHint, $TLA2TOOLS_JAR, or any of searchDirs; java from
// $JAVA_HOME or PATH. A missing tool is an error the caller surfaces — there is
// no silent fallback, because an unverified spec must never pass as verified.
func NewToolchain(jarHint string, searchDirs ...string) (*Toolchain, error) {
	java, err := findJava()
	if err != nil {
		return nil, err
	}
	jar, err := FindJar(jarHint, searchDirs...)
	if err != nil {
		return nil, err
	}
	return &Toolchain{Java: java, Jar: jar}, nil
}

// FindJava resolves the java binary from $JAVA_HOME or PATH, the same way the
// toolchain does. Exported so `astdst doctor` can report its location without
// constructing a full Toolchain (which also requires the jar).
func FindJava() (string, error) { return findJava() }

func findJava() (string, error) {
	if home := os.Getenv("JAVA_HOME"); home != "" {
		cand := filepath.Join(home, "bin", "java")
		if isExecutable(cand) {
			return cand, nil
		}
	}
	if p, err := exec.LookPath("java"); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("java not found: install a JRE or set JAVA_HOME (TLC needs a JVM)")
}

// FindJar resolves tla2tools.jar without downloading. searchDirs are scanned for
// a file named "tla2tools.jar".
func FindJar(jarHint string, searchDirs ...string) (string, error) {
	candidates := []string{jarHint, os.Getenv("TLA2TOOLS_JAR")}
	for _, d := range searchDirs {
		if d != "" {
			candidates = append(candidates, filepath.Join(d, "tla2tools.jar"))
		}
	}
	for _, c := range candidates {
		if c != "" && fileExists(c) {
			abs, err := filepath.Abs(c)
			if err != nil {
				return c, nil
			}
			return abs, nil
		}
	}
	return "", fmt.Errorf("tla2tools.jar not found: set $TLA2TOOLS_JAR or place it beside the spec (looked in %d locations)", len(candidates))
}

// SANY runs the syntax/semantic checker on tlaPath. It is fast and catches the
// bulk of malformed drafts before the expensive TLC run.
func (t *Toolchain) SANY(ctx context.Context, tlaPath string) (*Result, error) {
	dir, file := filepath.Split(tlaPath)
	out, code, timedOut, err := t.run(ctx, dir, "tla2sany.SANY", file)
	if err != nil && !timedOut {
		return nil, err
	}
	return parseSANY(out, code, timedOut), nil
}

// TLC model-checks tlaPath against cfgPath. Both files must live in the same
// directory; TLC writes its scratch state there. The caller bounds runtime by
// cancelling ctx.
func (t *Toolchain) TLC(ctx context.Context, tlaPath, cfgPath string, opts TLCOptions) (*Result, error) {
	dir, file := filepath.Split(tlaPath)
	workers := opts.Workers
	if workers <= 0 {
		workers = 1
	}
	args := []string{
		"tlc2.TLC",
		"-workers", strconv.Itoa(workers),
		"-config", filepath.Base(cfgPath),
		"-cleanup", // remove the states/ scratch dir on exit
		file,
	}
	out, code, timedOut, err := t.run(ctx, dir, args...)
	if err != nil && !timedOut {
		return nil, err
	}
	return parseTLC(out, code, timedOut), nil
}

// run invokes the JVM with the jar on the classpath, in workdir, capturing
// combined output. A context cancellation (timeout) is reported via timedOut.
func (t *Toolchain) run(ctx context.Context, workdir string, args ...string) (out string, code int, timedOut bool, err error) {
	full := append([]string{"-cp", t.Jar}, args...)
	cmd := exec.CommandContext(ctx, t.Java, full...)
	if workdir != "" {
		cmd.Dir = workdir
	}
	b, runErr := cmd.CombinedOutput()
	out = string(b)
	if ctx.Err() == context.DeadlineExceeded {
		return out, -1, true, ctx.Err()
	}
	if runErr != nil {
		if ee, ok := runErr.(*exec.ExitError); ok {
			return out, ee.ExitCode(), false, nil // non-zero exit is data, not a Go error
		}
		return out, -1, false, runErr // failed to launch the JVM at all
	}
	return out, 0, false, nil
}

var (
	reTLCStates    = regexp.MustCompile(`(\d+) distinct states found`)
	reTLCDepth     = regexp.MustCompile(`depth of the complete state graph search is (\d+)`)
	reTLCViolated  = regexp.MustCompile(`Invariant (\w+) is violated`)
	reTLCErrorLine = regexp.MustCompile(`(?m)^Error: .*$`)
)

// parseTLC turns TLC's output into a Result. OK means the model checker explored
// the state space and found no error; a violation, deadlock, or non-zero exit
// all make OK false and populate the fields the repair prompt feeds back.
func parseTLC(out string, code int, timedOut bool) *Result {
	r := &Result{Phase: PhaseTLC, ExitCode: code, TimedOut: timedOut, Raw: out}
	if m := reTLCStates.FindStringSubmatch(out); m != nil {
		r.States, _ = strconv.Atoi(m[1])
	}
	if m := reTLCDepth.FindStringSubmatch(out); m != nil {
		r.Depth, _ = strconv.Atoi(m[1])
	}
	if m := reTLCViolated.FindStringSubmatch(out); m != nil {
		r.Violated = m[1]
	}
	if strings.Contains(out, "Deadlock reached") {
		r.Deadlock = true
	}
	r.Errors = reTLCErrorLine.FindAllString(out, -1)
	if timedOut {
		r.Errors = append(r.Errors, "TLC timed out: the state space is too large; tighten the .cfg bounds (smaller CONSTANT sets, add a CONSTRAINT)")
		return r
	}
	r.OK = code == 0 && r.Violated == "" && !r.Deadlock &&
		strings.Contains(out, "Model checking completed")
	return r
}

// parseSANY turns SANY's output into a Result. SANY exits 0 only when the module
// parses and type-checks; otherwise the error block is captured for repair.
func parseSANY(out string, code int, timedOut bool) *Result {
	r := &Result{Phase: PhaseSANY, ExitCode: code, TimedOut: timedOut, Raw: out, OK: code == 0 && !timedOut}
	if r.OK {
		return r
	}
	if timedOut {
		r.Errors = []string{"SANY timed out (unexpected for parsing; check the spec size)"}
		return r
	}
	r.Errors = sanyErrorLines(out)
	return r
}

// sanyErrorLines pulls the meaningful diagnostic lines out of SANY output,
// skipping the routine "Parsing file" / "Semantic processing" progress noise.
func sanyErrorLines(out string) []string {
	var errs []string
	for _, line := range strings.Split(out, "\n") {
		t := strings.TrimSpace(line)
		if t == "" ||
			strings.HasPrefix(t, "Parsing file") ||
			strings.HasPrefix(t, "Semantic processing") ||
			strings.HasPrefix(t, "******") {
			continue
		}
		errs = append(errs, t)
	}
	if len(errs) == 0 {
		errs = []string{"SANY reported a non-zero exit with no parseable diagnostic"}
	}
	return errs
}

func fileExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}

func isExecutable(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir() && fi.Mode()&0o111 != 0
}
