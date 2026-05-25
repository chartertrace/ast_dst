package gen

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/chartertrace/ast_dst/golang/extract"
)

// Options tunes deterministic generation + verification.
type Options struct {
	ModuleName string // TLA+ module / file base name; default "DstSpec"
	Workers    int    // TLC parallel workers
	ScratchDir string // where the draft is written for checking; temp dir if empty
}

// GenResult is the outcome: the synthesised draft, the synthesis report, and the
// verification results. Verified is true only when TLC checked the spec clean —
// here that means the initial state satisfies TypeOK and the active invariants.
type GenResult struct {
	Draft    *Draft       `json:"-"`
	Report   *SynthReport `json:"report"`
	Verified bool         `json:"verified"`
	SANY     *Result      `json:"sany,omitempty"`
	TLC      *Result      `json:"tlc,omitempty"`
}

// Generate deterministically synthesises a TLA+ spec from the model and verifies
// it with the real tools: SANY (does it parse and type-check?) then TLC (does the
// initial state satisfy the invariants?). No LLM, no network — the same model in
// yields the same spec out. Transitions are stubs, so "verified" is an honest,
// bounded claim: well-formed and consistent at Init, not a behavioural proof.
func Generate(ctx context.Context, tc *Toolchain, m *extract.Model, opts Options) (*GenResult, error) {
	draft, report, err := Synthesize(m, opts.ModuleName)
	if err != nil {
		return nil, err
	}

	scratch := opts.ScratchDir
	if scratch == "" {
		tmp, err := os.MkdirTemp("", "astdst-gen-")
		if err != nil {
			return nil, err
		}
		defer os.RemoveAll(tmp)
		scratch = tmp
	}
	tlaPath, cfgPath, err := writeDraft(scratch, draft)
	if err != nil {
		return nil, err
	}

	res := &GenResult{Draft: draft, Report: report}

	sany, err := tc.SANY(ctx, tlaPath)
	if err != nil {
		return nil, fmt.Errorf("SANY: %w", err)
	}
	res.SANY = sany
	if !sany.OK {
		return res, nil // parse/type error: not verified, returned for reporting
	}

	tlc, err := tc.TLC(ctx, tlaPath, cfgPath, TLCOptions{Workers: opts.Workers})
	if err != nil {
		return nil, fmt.Errorf("TLC: %w", err)
	}
	res.TLC = tlc
	res.Verified = tlc.OK
	return res, nil
}

// writeDraft writes the draft's module and config into dir and returns the paths.
func writeDraft(dir string, d *Draft) (tlaPath, cfgPath string, err error) {
	if err = os.MkdirAll(dir, 0o755); err != nil {
		return "", "", err
	}
	tlaPath = filepath.Join(dir, d.Module+".tla")
	cfgPath = filepath.Join(dir, d.Module+".cfg")
	if err = os.WriteFile(tlaPath, []byte(d.TLA), 0o644); err != nil {
		return "", "", err
	}
	if err = os.WriteFile(cfgPath, []byte(d.CFG), 0o644); err != nil {
		return "", "", err
	}
	return tlaPath, cfgPath, nil
}
