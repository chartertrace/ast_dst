package trace

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestWriterRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriterTo(&buf)

	events := []Event{
		{I: 0, T: 1, Op: "Payout", Faults: []string{"FaultCreditRace"},
			Invariants: &Invariants{Fired: []string{"PAYMENT-1"}}},
		{I: 1, T: 2, Op: "Settle",
			Invariants: &Invariants{Fired: []string{"PAYMENT-2"}, Violated: []string{"PAYMENT-3"}}},
		{I: 2, T: 3, Op: "Tick", Writes: map[string]any{"clock": 1}},
	}
	for _, e := range events {
		if err := w.Event(e); err != nil {
			t.Fatalf("Event: %v", err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Each event must occupy exactly one line and round-trip cleanly.
	sc := bufio.NewScanner(&buf)
	i := 0
	for sc.Scan() {
		var got Event
		if err := json.Unmarshal(sc.Bytes(), &got); err != nil {
			t.Fatalf("line %d unmarshal: %v\nline: %s", i, err, sc.Text())
		}
		want := events[i]
		if got.I != want.I || got.T != want.T || got.Op != want.Op {
			t.Errorf("line %d: got %+v, want %+v", i, got, want)
		}
		if !stringSliceEqual(got.Faults, want.Faults) {
			t.Errorf("line %d faults: got %v want %v", i, got.Faults, want.Faults)
		}
		gotFired, gotViolated := invFields(got.Invariants)
		wantFired, wantViolated := invFields(want.Invariants)
		if !stringSliceEqual(gotFired, wantFired) {
			t.Errorf("line %d fired: got %v want %v", i, gotFired, wantFired)
		}
		if !stringSliceEqual(gotViolated, wantViolated) {
			t.Errorf("line %d violated: got %v want %v", i, gotViolated, wantViolated)
		}
		i++
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if i != len(events) {
		t.Errorf("got %d lines, want %d", i, len(events))
	}
}

func TestEmptyOmittedFields(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriterTo(&buf)
	// An event with no faults/invariants/writes should omit those keys.
	if err := w.Event(Event{I: 0, T: 0, Op: "Noop"}); err != nil {
		t.Fatalf("Event: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	line := strings.TrimSpace(buf.String())
	for _, key := range []string{"faults", "invariants", "writes"} {
		if strings.Contains(line, `"`+key+`"`) {
			t.Errorf("expected %q key omitted from minimal event, got %s", key, line)
		}
	}
}

func invFields(inv *Invariants) ([]string, []string) {
	if inv == nil {
		return nil, nil
	}
	return inv.Fired, inv.Violated
}

func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
