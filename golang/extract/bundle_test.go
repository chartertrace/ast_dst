package extract

import (
	"strings"
	"testing"
)

func TestBuildBundleCarriesHandlerSource(t *testing.T) {
	t.Parallel()
	b, err := BuildBundle(stateConfig())
	if err != nil {
		t.Fatalf("BuildBundle: %v", err)
	}
	if b.State == nil || len(b.State.Variables) == 0 {
		t.Fatal("bundle missing state model")
	}
	if len(b.Operations) == 0 {
		t.Fatal("bundle has no operations")
	}

	// The transfer handler records two faults in its body; its source must come
	// through so the generator can see the actual mutations, not just the name.
	var transfer *OpSpec
	for i := range b.Operations {
		if b.Operations[i].Name == "Transfer" {
			transfer = &b.Operations[i]
		}
	}
	if transfer == nil {
		t.Fatal("Transfer op not in bundle")
	}
	if !strings.Contains(transfer.Source, "recordFault") {
		t.Errorf("Transfer handler source not carried: %q", transfer.Source)
	}
}
