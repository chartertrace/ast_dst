package cli

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/apache/arrow-go/v18/parquet"
	"github.com/apache/arrow-go/v18/parquet/compress"
	"github.com/apache/arrow-go/v18/parquet/pqarrow"

	"github.com/chartertrace/ast_dst/golang/trace"
)

// RunTraceCompact reads canonical NDJSON trace events (one Event per line —
// see golang/trace) and writes a single Parquet file the flow viewer queries
// via DuckDB-WASM. The schema is fixed (see TraceParquetSchema) and pinned by
// the schema-sync test on both sides.
//
// Op names, fault enum names, and invariant IDs are resolved to integer
// indices against --model so the on-disk file carries no string identifiers
// per row — at billions of events that matters. The result is one column
// per concept: op as uint16 index, faults / invariants as uint64 bitsets.
func RunTraceCompact(args []string) {
	fs := flag.NewFlagSet("astdst trace-compact", flag.ExitOnError)
	in := fs.String("in", "", "NDJSON trace file to read (one Event per line)")
	out := fs.String("out", "", "Parquet file to write")
	modelPath := fs.String("model", "", "model.json that names the ops/faults/invariants the trace references")
	rowGroupSize := fs.Int("row-group-size", 32768, "rows per Parquet row group (~1 MB at this schema)")
	maxBits := fs.Int("max-bits", 64, "abort if the fault or invariant catalogue exceeds this (Stage 1 bitsets are uint64)")
	_ = fs.Parse(args)

	if *in == "" || *out == "" || *modelPath == "" {
		fail("trace-compact: --in, --out and --model are required")
	}

	idx, err := loadTraceIndex(*modelPath, *maxBits)
	if err != nil {
		fail("trace-compact: %v", err)
	}

	stats, err := compactTrace(*in, *out, idx, *rowGroupSize)
	if err != nil {
		fail("trace-compact: %v", err)
	}

	fmt.Fprintf(os.Stderr,
		"astdst: compacted %d events → %s (%d row groups, schema hash %s)\n",
		stats.Events, *out, stats.RowGroups, traceSchemaHashPrefix())
}

// TraceParquetSchema is the on-disk schema; both the writer and the TS-side
// schema constant pin to its hash so any drift trips a test on both sides.
var TraceParquetSchema = arrow.NewSchema([]arrow.Field{
	{Name: "i", Type: arrow.PrimitiveTypes.Uint64},
	{Name: "t", Type: arrow.PrimitiveTypes.Uint64},
	{Name: "op", Type: arrow.PrimitiveTypes.Uint16},
	{Name: "fault_bits", Type: arrow.PrimitiveTypes.Uint64},
	{Name: "inv_fired_bits", Type: arrow.PrimitiveTypes.Uint64},
	{Name: "inv_violated_bits", Type: arrow.PrimitiveTypes.Uint64},
	{Name: "writes_json", Type: arrow.BinaryTypes.Binary, Nullable: true},
}, nil)

// TraceSchemaHash is the SHA-256 of the canonical schema description. The TS
// side computes the same hash from the same canonical string; the schema-sync
// test on both sides asserts the constants agree.
func TraceSchemaHash() string {
	h := sha256.Sum256([]byte(traceSchemaCanonical()))
	return hex.EncodeToString(h[:])
}

// traceSchemaCanonical returns the schema as one string the TS side mirrors
// verbatim. Keep this exact format in lockstep with typescript/src/flow/schema.ts.
func traceSchemaCanonical() string {
	return "i:u64;t:u64;op:u16;fault_bits:u64;inv_fired_bits:u64;inv_violated_bits:u64;writes_json:binary?"
}

func traceSchemaHashPrefix() string {
	h := TraceSchemaHash()
	if len(h) > 12 {
		return h[:12]
	}
	return h
}

// traceIndex resolves the string identifiers carried by NDJSON events to the
// integer indices the Parquet file stores.
type traceIndex struct {
	op        map[string]uint16
	fault     map[string]int // bit position
	invariant map[string]int // bit position
}

func loadTraceIndex(modelPath string, maxBits int) (*traceIndex, error) {
	data, err := os.ReadFile(modelPath)
	if err != nil {
		return nil, fmt.Errorf("read model: %w", err)
	}
	// Decode just the parts we need; the full extract.Model schema is bigger
	// than necessary here and pulls in dependencies we don't want.
	var m struct {
		Operations []struct {
			Index int    `json:"index"`
			Name  string `json:"name"`
		} `json:"operations"`
		Faults []struct {
			Enum string `json:"enum"`
		} `json:"faults"`
		Invariants []struct {
			ID string `json:"id"`
		} `json:"invariants"`
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse model: %w", err)
	}

	if len(m.Faults) > maxBits {
		return nil, fmt.Errorf("model has %d faults; Stage 1 bitsets cap at %d (widen the schema before lifting)", len(m.Faults), maxBits)
	}
	if len(m.Invariants) > maxBits {
		return nil, fmt.Errorf("model has %d invariants; Stage 1 bitsets cap at %d (widen the schema before lifting)", len(m.Invariants), maxBits)
	}

	idx := &traceIndex{
		op:        make(map[string]uint16, len(m.Operations)),
		fault:     make(map[string]int, len(m.Faults)),
		invariant: make(map[string]int, len(m.Invariants)),
	}
	for _, op := range m.Operations {
		if op.Index < 0 || op.Index > 0xFFFF {
			return nil, fmt.Errorf("operation index %d out of uint16 range", op.Index)
		}
		idx.op[op.Name] = uint16(op.Index)
	}
	for i, f := range m.Faults {
		idx.fault[f.Enum] = i
	}
	for i, inv := range m.Invariants {
		idx.invariant[inv.ID] = i
	}
	return idx, nil
}

type compactStats struct {
	Events    int
	RowGroups int
}

func compactTrace(inPath, outPath string, idx *traceIndex, rowGroupSize int) (compactStats, error) {
	inF, err := os.Open(inPath)
	if err != nil {
		return compactStats{}, fmt.Errorf("open input: %w", err)
	}
	defer inF.Close()

	outF, err := os.Create(outPath)
	if err != nil {
		return compactStats{}, fmt.Errorf("create output: %w", err)
	}
	defer outF.Close()

	props := parquet.NewWriterProperties(
		parquet.WithCompression(compress.Codecs.Zstd),
		parquet.WithMaxRowGroupLength(int64(rowGroupSize)),
	)
	arrProps := pqarrow.DefaultWriterProps()

	w, err := pqarrow.NewFileWriter(TraceParquetSchema, outF, props, arrProps)
	if err != nil {
		return compactStats{}, fmt.Errorf("parquet writer: %w", err)
	}
	defer w.Close()

	mem := memory.NewGoAllocator()
	builder := array.NewRecordBuilder(mem, TraceParquetSchema)
	defer builder.Release()

	// bufio.Scanner default token size (64 KB) is too small for events whose
	// writes map carries arbitrary JSON; raise to 8 MB per line.
	sc := bufio.NewScanner(inF)
	sc.Buffer(make([]byte, 1<<16), 8<<20)

	stats := compactStats{}
	lastT := uint64(0)
	rowsInBatch := 0

	flush := func() error {
		if rowsInBatch == 0 {
			return nil
		}
		rec := builder.NewRecord()
		err := w.Write(rec)
		rec.Release()
		if err != nil {
			return err
		}
		stats.RowGroups++
		rowsInBatch = 0
		return nil
	}

	lineNo := 0
	for sc.Scan() {
		lineNo++
		raw := sc.Bytes()
		if len(raw) == 0 {
			continue
		}
		var ev trace.Event
		if err := json.Unmarshal(raw, &ev); err != nil {
			return stats, fmt.Errorf("line %d: %w", lineNo, err)
		}

		// Sortedness check — the writer must hand us a non-decreasing t so we
		// can stream straight to disk without buffering the whole run.
		if stats.Events > 0 && ev.T < lastT {
			return stats, fmt.Errorf("line %d: t=%d decreased (previous %d); the harness must emit in t order", lineNo, ev.T, lastT)
		}
		lastT = ev.T

		opIdx, ok := idx.op[ev.Op]
		if !ok {
			return stats, fmt.Errorf("line %d: unknown op %q (not in model.operations)", lineNo, ev.Op)
		}
		faultBits, err := bitsetFor(ev.Faults, idx.fault, "fault")
		if err != nil {
			return stats, fmt.Errorf("line %d: %w", lineNo, err)
		}
		var firedBits, violatedBits uint64
		if ev.Invariants != nil {
			firedBits, err = bitsetFor(ev.Invariants.Fired, idx.invariant, "invariant")
			if err != nil {
				return stats, fmt.Errorf("line %d (fired): %w", lineNo, err)
			}
			violatedBits, err = bitsetFor(ev.Invariants.Violated, idx.invariant, "invariant")
			if err != nil {
				return stats, fmt.Errorf("line %d (violated): %w", lineNo, err)
			}
		}

		builder.Field(0).(*array.Uint64Builder).Append(ev.I)
		builder.Field(1).(*array.Uint64Builder).Append(ev.T)
		builder.Field(2).(*array.Uint16Builder).Append(opIdx)
		builder.Field(3).(*array.Uint64Builder).Append(faultBits)
		builder.Field(4).(*array.Uint64Builder).Append(firedBits)
		builder.Field(5).(*array.Uint64Builder).Append(violatedBits)
		wb := builder.Field(6).(*array.BinaryBuilder)
		if len(ev.Writes) == 0 {
			wb.AppendNull()
		} else {
			wj, err := json.Marshal(ev.Writes)
			if err != nil {
				return stats, fmt.Errorf("line %d writes: %w", lineNo, err)
			}
			wb.Append(wj)
		}

		stats.Events++
		rowsInBatch++
		if rowsInBatch >= rowGroupSize {
			if err := flush(); err != nil {
				return stats, fmt.Errorf("flush row group: %w", err)
			}
		}
	}
	if err := sc.Err(); err != nil && err != io.EOF {
		return stats, fmt.Errorf("scan: %w", err)
	}
	if err := flush(); err != nil {
		return stats, fmt.Errorf("final flush: %w", err)
	}

	if err := w.Close(); err != nil {
		return stats, fmt.Errorf("close parquet: %w", err)
	}
	return stats, nil
}

// bitsetFor turns a list of catalogue identifiers into a uint64 bitset against
// the supplied index. An unknown identifier is an error; a silent drop would
// undercut the project's "gaps are real gaps" honesty guarantee.
func bitsetFor(names []string, idx map[string]int, what string) (uint64, error) {
	var bits uint64
	for _, n := range names {
		pos, ok := idx[n]
		if !ok {
			return 0, fmt.Errorf("unknown %s %q (not in model)", what, n)
		}
		bits |= 1 << uint(pos)
	}
	return bits, nil
}
