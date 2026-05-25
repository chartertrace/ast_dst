package extract

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// TLA+ is not Go, so go/ast does not apply. The spec files are plain text with a
// small, regular grammar, so a line scanner suffices and keeps the extractor
// dependency-free (matching the rest of the package). We recognise exactly two
// shapes:
//
//	Name == <predicate body>      operator definitions (one per top-level name)
//	\A x \in S: ...               body lines, indented, until a blank line
//
// and, in any "*Invariant*" aggregator, the conjuncts `/\ OtherName` that name
// the invariants the spec actually declares.

var (
	// defStart matches a top-level operator definition: a name at column 0
	// followed by "==". The remainder of the line is the first body line.
	defStart = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9_]*)\s*==\s*(.*)$`)
	// conjunct pulls the identifier out of a `/\ Name` aggregator line.
	conjunct = regexp.MustCompile(`^/\\\s*([A-Za-z][A-Za-z0-9_]*)\s*$`)
)

// specDef is one parsed TLA+ operator definition before we decide whether it is
// an invariant worth surfacing.
type specDef struct {
	name      string
	predicate string
	doc       string
	file      string // base name, e.g. "PayoutFlowV3.tla"
	loc       *Loc
}

// parseSpecs reads every *.tla file under absSpecDir plus the TLC config
// (cfgFile, within that dir) and returns the declared invariants. simRoot is
// used only to write Loc paths relative to the sim, matching the Go nodes.
//
// "Declared invariant" means: a name conjoined in some `*Invariant*` aggregator
// (e.g. AllInvariants in PayoutFlowV3.tla), unioned with any name the .cfg
// model-checks. Helper operators (Init, Next, TypeOK-style state predicates not
// referenced as invariants) are excluded. Checked reports .cfg membership.
func parseSpecs(absSpecDir, cfgFile, simRoot string) ([]SpecInvariant, error) {
	entries, err := os.ReadDir(absSpecDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // configured but absent: a gap, not a failure
		}
		return nil, err
	}

	defs := map[string]specDef{} // name -> chosen definition
	declared := map[string]bool{}
	// primaryFile is the .tla paired with the .cfg ("PayoutFlow.cfg" ->
	// "PayoutFlow.tla"); its definitions win when a name is defined in several
	// files, since that is the spec the .cfg actually model-checks.
	primaryFile := strings.TrimSuffix(cfgFile, ".cfg") + ".tla"

	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".tla") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files) // deterministic dedupe order

	for _, name := range files {
		path := filepath.Join(absSpecDir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		rel, relErr := filepath.Rel(simRoot, path)
		if relErr != nil {
			rel = path
		}
		fileDefs, fileDeclared := parseSpecFile(string(data), name, rel)
		for k := range fileDeclared {
			declared[k] = true
		}
		for _, d := range fileDefs {
			if cur, ok := defs[d.name]; ok && cur.file == primaryFile && d.file != primaryFile {
				continue // keep the canonical (model-checked) definition
			}
			defs[d.name] = d
		}
	}

	checked, err := parseCfgInvariants(filepath.Join(absSpecDir, cfgFile))
	if err != nil {
		return nil, err
	}

	// Surface a name if it's declared as an invariant, or model-checked and
	// backed by a definition we found.
	wanted := map[string]bool{}
	for n := range declared {
		wanted[n] = true
	}
	for n := range checked {
		if _, ok := defs[n]; ok {
			wanted[n] = true
		}
	}

	var out []SpecInvariant
	for name := range wanted {
		d, ok := defs[name]
		if !ok {
			continue // declared in an aggregator but no definition body found
		}
		out = append(out, SpecInvariant{
			Name:      d.name,
			Predicate: d.predicate,
			SpecFile:  d.file,
			Checked:   checked[name],
			Doc:       d.doc,
			Loc:       d.loc,
		})
	}
	// Checked invariants first, then by file and name — stable for golden tests.
	sort.Slice(out, func(i, j int) bool {
		if out[i].Checked != out[j].Checked {
			return out[i].Checked
		}
		if out[i].SpecFile != out[j].SpecFile {
			return out[i].SpecFile < out[j].SpecFile
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// parseSpecFile scans one .tla file, returning its operator definitions and the
// set of names referenced as conjuncts inside any "*Invariant*" aggregator.
func parseSpecFile(src, base, rel string) (map[string]specDef, map[string]bool) {
	lines := strings.Split(src, "\n")
	defs := map[string]specDef{}

	// Pass 1: collect every top-level definition with its body and doc.
	type rawDef struct {
		name      string
		startLine int // 1-based
		body      []string
	}
	var raws []rawDef
	var pendingDoc []string
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if rest, ok := strings.CutPrefix(trimmed, `\*`); ok {
			pendingDoc = append(pendingDoc, strings.TrimSpace(rest))
			continue
		}
		m := defStart.FindStringSubmatch(line)
		if m == nil {
			if trimmed == "" {
				pendingDoc = nil // blank line breaks a doc block
			}
			continue
		}
		name := m[1]
		body := []string{}
		if first := strings.TrimRight(m[2], " \t"); first != "" {
			body = append(body, first)
		}
		// Body continues on indented lines until a blank line or the next
		// top-level construct (column-0 token).
		j := i + 1
		for ; j < len(lines); j++ {
			next := lines[j]
			if strings.TrimSpace(next) == "" {
				break
			}
			if next[0] != ' ' && next[0] != '\t' {
				break // a new column-0 definition or comment ends this body
			}
			body = append(body, strings.TrimRight(next, " \t"))
		}
		defs[name] = specDef{
			name:      name,
			predicate: dedent(body),
			doc:       strings.TrimSpace(strings.Join(pendingDoc, " ")),
			file:      base,
			loc:       &Loc{File: rel, Line: i + 1},
		}
		raws = append(raws, rawDef{name: name, startLine: i + 1, body: body})
		pendingDoc = nil
		i = j - 1
	}

	// Pass 2: a definition whose name contains "invariant" (case-insensitive) is
	// an aggregator; the bare identifiers it conjoins are the declared invariants.
	declared := map[string]bool{}
	for _, r := range raws {
		if !strings.Contains(strings.ToLower(r.name), "invariant") {
			continue
		}
		for _, bl := range r.body {
			cm := conjunct.FindStringSubmatch(strings.TrimSpace(bl))
			if cm == nil {
				continue
			}
			ref := cm[1]
			// Skip references to other aggregators (e.g. AllInvariantsExtended
			// conjoins AllInvariants); we want the leaf invariants only.
			if strings.Contains(strings.ToLower(ref), "invariant") {
				continue
			}
			declared[ref] = true
		}
	}
	return defs, declared
}

// dedent joins body lines into a predicate, stripping the common leading
// indentation so the stored text reads cleanly (the TLA+ source indents bodies
// under the "Name ==" line). Relative indentation between lines is preserved.
func dedent(body []string) string {
	min := -1
	for _, l := range body {
		if strings.TrimSpace(l) == "" {
			continue
		}
		n := len(l) - len(strings.TrimLeft(l, " \t"))
		if min < 0 || n < min {
			min = n
		}
	}
	if min <= 0 {
		return strings.Join(body, "\n")
	}
	out := make([]string, len(body))
	for i, l := range body {
		if len(l) >= min {
			out[i] = l[min:]
		} else {
			out[i] = strings.TrimLeft(l, " \t")
		}
	}
	return strings.Join(out, "\n")
}

// parseCfgInvariants reads a TLC .cfg and returns the names on its INVARIANT /
// INVARIANTS lines — the subset the model checker actually verifies. A missing
// file yields an empty set (the spec exists but nothing is model-checked).
func parseCfgInvariants(path string) (map[string]bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]bool{}, nil
		}
		return nil, err
	}
	out := map[string]bool{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, `\*`) {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 || (fields[0] != "INVARIANT" && fields[0] != "INVARIANTS") {
			continue
		}
		for _, name := range fields[1:] {
			out[name] = true
		}
	}
	return out, nil
}
