package gen

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// --- Pure parser tests (no JVM required) ----------------------------------

func TestParseTLCClean(t *testing.T) {
	t.Parallel()
	out := `TLC2 Version 2.18
Computing initial states...
Model checking completed. No error has been found.
  Estimates of the probability that TLC did not check ...
106 states generated, 53 distinct states found, 0 states left on queue.
The depth of the complete state graph search is 7.`
	r := parseTLC(out, 0, false)
	if !r.OK {
		t.Errorf("expected OK, got %+v", r)
	}
	if r.States != 53 {
		t.Errorf("States = %d, want 53", r.States)
	}
	if r.Depth != 7 {
		t.Errorf("Depth = %d, want 7", r.Depth)
	}
}

func TestParseTLCViolation(t *testing.T) {
	t.Parallel()
	out := `Error: Invariant NoRestrictedWithdrawals is violated.
Error: The behavior up to this point is:
200 states generated, 110 distinct states found, 58 states left on queue.`
	r := parseTLC(out, 12, false)
	if r.OK {
		t.Error("violation must not be OK")
	}
	if r.Violated != "NoRestrictedWithdrawals" {
		t.Errorf("Violated = %q, want NoRestrictedWithdrawals", r.Violated)
	}
	if len(r.Errors) == 0 {
		t.Error("expected error lines for repair feedback")
	}
}

func TestParseTLCTimeout(t *testing.T) {
	t.Parallel()
	r := parseTLC("Computing initial states...", -1, true)
	if r.OK || !r.TimedOut {
		t.Errorf("timeout must be !OK and TimedOut, got %+v", r)
	}
	if len(r.Errors) == 0 {
		t.Error("timeout should advise tightening bounds")
	}
}

func TestParseSANY(t *testing.T) {
	t.Parallel()
	clean := parseSANY("Parsing file X\nSemantic processing of module Y\n", 0, false)
	if !clean.OK {
		t.Error("exit 0 SANY should be OK")
	}
	bad := parseSANY("***Parse Error***\nEncountered \"=\" at line 7\n", 1, false)
	if bad.OK {
		t.Error("non-zero SANY should not be OK")
	}
	if len(bad.Errors) == 0 {
		t.Error("expected captured SANY error lines")
	}
}

// --- Integration tests (require java + tla2tools.jar) ----------------------

// testToolchain locates the toolchain or skips. The sim ships a tla2tools.jar
// we reuse; tests also honour $TLA2TOOLS_JAR.
func testToolchain(t *testing.T) *Toolchain {
	t.Helper()
	simJar := "../../../app/primary-server/docs/tla" // relative to gen/
	tc, err := NewToolchain("", simJar)
	if err != nil {
		t.Skipf("TLA+ toolchain unavailable, skipping: %v", err)
	}
	return tc
}

// copyToTemp copies a fixture .tla/.cfg pair into a fresh temp dir so TLC can
// write its scratch state without touching the repo.
func copyToTemp(t *testing.T, names ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, n := range names {
		data, err := os.ReadFile(filepath.Join("testdata", n))
		if err != nil {
			t.Fatalf("read fixture %s: %v", n, err)
		}
		if err := os.WriteFile(filepath.Join(dir, n), data, 0o644); err != nil {
			t.Fatalf("write fixture %s: %v", n, err)
		}
	}
	return dir
}

func TestSANYAcceptsGoodRejectsBroken(t *testing.T) {
	tc := testToolchain(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	good := copyToTemp(t, "Good.tla")
	if r, err := tc.SANY(ctx, filepath.Join(good, "Good.tla")); err != nil {
		t.Fatalf("SANY good: %v", err)
	} else if !r.OK {
		t.Errorf("Good.tla should parse, got %+v", r.Errors)
	}

	broken := copyToTemp(t, "Broken.tla")
	if r, err := tc.SANY(ctx, filepath.Join(broken, "Broken.tla")); err != nil {
		t.Fatalf("SANY broken: %v", err)
	} else if r.OK {
		t.Error("Broken.tla should fail to parse")
	}
}

func TestTLCVerifiesGoodCatchesBad(t *testing.T) {
	tc := testToolchain(t)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	good := copyToTemp(t, "Good.tla", "Good.cfg")
	r, err := tc.TLC(ctx, filepath.Join(good, "Good.tla"), filepath.Join(good, "Good.cfg"), TLCOptions{Workers: 2})
	if err != nil {
		t.Fatalf("TLC good: %v", err)
	}
	if !r.OK {
		t.Errorf("Good spec should verify clean, got %+v", r)
	}

	bad := copyToTemp(t, "Bad.tla", "Bad.cfg")
	r, err = tc.TLC(ctx, filepath.Join(bad, "Bad.tla"), filepath.Join(bad, "Bad.cfg"), TLCOptions{Workers: 2})
	if err != nil {
		t.Fatalf("TLC bad: %v", err)
	}
	if r.OK {
		t.Error("Bad spec should be caught")
	}
	if r.Violated != "Inv" {
		t.Errorf("expected Inv violated, got %q (%+v)", r.Violated, r.Errors)
	}
}
