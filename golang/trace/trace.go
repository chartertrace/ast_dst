// Package trace is the canonical NDJSON event writer for DST harnesses
// instrumented for ast_dst's flow viewer.
//
// One Event per line, newline-delimited JSON. The line schema is the single
// source of truth shared with two consumers:
//
//   - astdst trace-compact reads NDJSON and writes a columnar Parquet file
//     the browser queries via DuckDB-WASM.
//   - The TLA+ trace-validation pipeline (typescript gen:trace) reads the same
//     NDJSON for TLC replay against an existing spec.
//
// Owning the writer in one place keeps the format honest across both paths;
// every consuming codebase imports this rather than re-implementing it.
package trace

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// Event is one DST event in the canonical wire format. Field names match the
// JSON keys the compactor and TLC scaffold both expect.
type Event struct {
	// I is the monotonic event index across a run (0-based).
	I uint64 `json:"i"`
	// T is the simulated time the event was recorded at; units are the
	// harness's (ticks, nanoseconds, ...). Must be non-decreasing across a run.
	T uint64 `json:"t"`
	// Op is the operation name matching model.operations[].name.
	Op string `json:"op"`
	// Writes is an optional map of state-field name to the new rendered value.
	// Free-form JSON; Stage 1 of the compactor stores it as opaque bytes.
	Writes map[string]any `json:"writes,omitempty"`
	// Faults are the fault enum names injected by this event (matching
	// model.faults[].enum). Empty when none were injected.
	Faults []string `json:"faults,omitempty"`
	// Invariants records which invariants fired/violated on this event,
	// keyed by invariant ID (matching model.invariants[].id). Pointer so
	// omitempty elides the key entirely when no invariant ran on this event.
	Invariants *Invariants `json:"invariants,omitempty"`
}

// Invariants is the per-event invariant outcome. Both lists are optional;
// the wrapping pointer in Event lets the whole object be omitted when neither.
type Invariants struct {
	Fired    []string `json:"fired,omitempty"`
	Violated []string `json:"violated,omitempty"`
}

// Writer emits canonical NDJSON, one Event per line, to an io.Writer.
// It is not safe for concurrent use; a DST harness should serialise events
// through a single Writer (or use one per goroutine and merge by I).
type Writer struct {
	bw  *bufio.Writer
	enc *json.Encoder
	// own is the underlying file when NewWriter created it, so Close knows
	// to close it; nil when wrapping a caller-owned io.Writer.
	own io.Closer
}

// NewWriter creates the file at path (truncating any existing) and returns a
// Writer that buffers NDJSON to it. Close it to flush.
func NewWriter(path string) (*Writer, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("trace: create %s: %w", path, err)
	}
	w := newWriter(f)
	w.own = f
	return w, nil
}

// NewWriterTo wraps an existing io.Writer (e.g. for tests or in-memory
// capture). The caller retains ownership; Close flushes but does not close.
func NewWriterTo(w io.Writer) *Writer {
	return newWriter(w)
}

func newWriter(w io.Writer) *Writer {
	bw := bufio.NewWriterSize(w, 1<<16)
	enc := json.NewEncoder(bw)
	// SetEscapeHTML(false) keeps strings like "<", "&" readable in traces;
	// no HTML context here, no reason to escape.
	enc.SetEscapeHTML(false)
	return &Writer{bw: bw, enc: enc}
}

// Event appends one NDJSON line. json.Encoder writes a trailing newline so
// the result is canonical NDJSON without extra work.
func (w *Writer) Event(e Event) error {
	if err := w.enc.Encode(e); err != nil {
		return fmt.Errorf("trace: encode event %d: %w", e.I, err)
	}
	return nil
}

// Close flushes the buffer and (when NewWriter created the file) closes it.
func (w *Writer) Close() error {
	if err := w.bw.Flush(); err != nil {
		return fmt.Errorf("trace: flush: %w", err)
	}
	if w.own != nil {
		return w.own.Close()
	}
	return nil
}
