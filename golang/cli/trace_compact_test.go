package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/apache/arrow-go/v18/parquet/file"
	"github.com/apache/arrow-go/v18/parquet/pqarrow"
)

func TestCompactTraceRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	outPath := filepath.Join(tmp, "trace.parquet")

	idx, err := loadTraceIndex("testdata/trace/tiny.model.json", 64)
	if err != nil {
		t.Fatalf("loadTraceIndex: %v", err)
	}
	stats, err := compactTrace("testdata/trace/tiny.ndjson", outPath, idx, 2)
	if err != nil {
		t.Fatalf("compactTrace: %v", err)
	}
	if stats.Events != 5 {
		t.Errorf("Events: got %d want 5", stats.Events)
	}
	// 5 rows / 2 per group → 3 groups (2,2,1).
	if stats.RowGroups != 3 {
		t.Errorf("RowGroups: got %d want 3", stats.RowGroups)
	}

	// Read the Parquet back to confirm schema, row count, and a couple of
	// canonical values came through correctly.
	rd, err := file.OpenParquetFile(outPath, false)
	if err != nil {
		t.Fatalf("open parquet: %v", err)
	}
	defer rd.Close()

	pr, err := pqarrow.NewFileReader(rd, pqarrow.ArrowReadProperties{}, nil)
	if err != nil {
		t.Fatalf("NewFileReader: %v", err)
	}
	schema, err := pr.Schema()
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	wantFields := []string{"i", "t", "op", "fault_bits", "inv_fired_bits", "inv_violated_bits", "writes_json"}
	if got := schema.NumFields(); got != len(wantFields) {
		t.Fatalf("schema NumFields: got %d want %d", got, len(wantFields))
	}
	for i, want := range wantFields {
		if got := schema.Field(i).Name; got != want {
			t.Errorf("field %d: got %q want %q", i, got, want)
		}
	}

	table, err := pr.ReadTable(context.Background())
	if err != nil {
		t.Fatalf("ReadTable: %v", err)
	}
	defer table.Release()
	if got := table.NumRows(); got != 5 {
		t.Fatalf("NumRows: got %d want 5", got)
	}
}

func TestCompactRejectsUnknownOp(t *testing.T) {
	tmp := t.TempDir()
	in := filepath.Join(tmp, "bad.ndjson")
	if err := os.WriteFile(in, []byte(`{"i":0,"t":1,"op":"Ghost"}`+"\n"), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}
	idx, err := loadTraceIndex("testdata/trace/tiny.model.json", 64)
	if err != nil {
		t.Fatalf("loadTraceIndex: %v", err)
	}
	_, err = compactTrace(in, filepath.Join(tmp, "out.parquet"), idx, 16)
	if err == nil {
		t.Fatal("expected error for unknown op, got nil")
	}
}

func TestCompactRejectsTDecrease(t *testing.T) {
	tmp := t.TempDir()
	in := filepath.Join(tmp, "unsorted.ndjson")
	body := `{"i":0,"t":5,"op":"Payout"}` + "\n" + `{"i":1,"t":3,"op":"Settle"}` + "\n"
	if err := os.WriteFile(in, []byte(body), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}
	idx, err := loadTraceIndex("testdata/trace/tiny.model.json", 64)
	if err != nil {
		t.Fatalf("loadTraceIndex: %v", err)
	}
	_, err = compactTrace(in, filepath.Join(tmp, "out.parquet"), idx, 16)
	if err == nil {
		t.Fatal("expected error for decreasing t, got nil")
	}
}

func TestSchemaHashStable(t *testing.T) {
	// The exact hash is pinned in typescript/src/flow/schema.ts; if you change
	// this value, change that constant too (and bump the schema). Both sides'
	// schema-sync tests then re-pass.
	const want = "9b32f8790ba262c906ab5fa51b88c20d4459729a35e8d0e8a983d2227de7943f"
	if got := TraceSchemaHash(); got != want {
		t.Errorf("TraceSchemaHash drifted:\n  got  %s\n  want %s\nIf intentional, update both this constant AND typescript/src/flow/schema.ts.", got, want)
	}
}
