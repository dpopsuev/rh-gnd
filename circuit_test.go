package dsr

import (
	"context"
	"testing"

	"github.com/dpopsuev/origami/circuit"
	"github.com/dpopsuev/origami/engine"
	"github.com/dpopsuev/origami/schematics/toolkit"
	_ "github.com/dpopsuev/origami/topology"
)

// gatherOnlyCircuitYAML is a legacy 3-node circuit (tree→search→read)
// used by tests that don't need the synthesize step.
const gatherOnlyCircuitYAML = `
circuit: gnd
topology: cascade
handler_type: transformer
nodes:
  - name: tree
    handler: dsr.tree
  - name: search
    handler: dsr.search
  - name: read
    handler: dsr.read
edges:
  - id: tree-search
    from: tree
    to: search
  - id: search-read
    from: search
    to: read
  - id: read-done
    from: read
    to: _done
start: tree
done: _done
`

func TestCircuit_Walk(t *testing.T) {
	reader := &txReader{
		listings: map[string][]toolkit.ContentEntry{
			"acme/repo1": {
				{Path: "main.go", IsDir: false},
				{Path: "pkg/", IsDir: true},
			},
		},
		searches: map[string][]toolkit.SearchResult{
			"acme/repo1": {
				{Path: "main.go", Line: 10, Snippet: "func TestPTP()"},
			},
		},
		files: map[string][]byte{
			"acme/repo1:main.go": []byte("package main\n"),
		},
	}
	catalog := txCatalog()

	def, err := circuit.LoadCircuit([]byte(gatherOnlyCircuitYAML))
	if err != nil {
		t.Fatalf("LoadCircuit: %v", err)
	}

	comp := TransformerComponent(reader, catalog)
	reg, err := engine.MergeComponents(engine.GraphRegistries{}, comp)
	if err != nil {
		t.Fatalf("MergeComponents: %v", err)
	}

	g, err := engine.BuildGraph(def, reg)
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}

	walker := circuit.NewProcessWalker("test-gnd")
	walker.State().Context["dsr.search_keywords"] = []string{"TestPTP"}

	if err := g.Walk(context.Background(), walker, "tree"); err != nil {
		t.Fatalf("Walk: %v", err)
	}

	// Verify all 3 nodes produced outputs.
	for _, name := range []string{"tree", "search", "read"} {
		if _, ok := walker.State().Outputs[name]; !ok {
			t.Errorf("missing output for node %q", name)
		}
	}

	// Verify the final node produced a CodeContext.
	readArt := walker.State().Outputs["read"]
	raw := readArt.Raw()
	cc, ok := raw.(*CodeContext)
	if !ok {
		t.Fatalf("read artifact Raw() type = %T, want *CodeContext", raw)
	}

	if len(cc.Trees) != 1 {
		t.Errorf("trees = %d, want 1", len(cc.Trees))
	}
	if len(cc.SearchResults) != 1 {
		t.Errorf("search results = %d, want 1", len(cc.SearchResults))
	}
	if len(cc.Files) != 1 {
		t.Errorf("files = %d, want 1", len(cc.Files))
	}
	if cc.Files[0].Content != "package main\n" {
		t.Errorf("file content = %q", cc.Files[0].Content)
	}
}

func TestCircuit_FullWithSynthesize(t *testing.T) {
	reader := &txReader{
		listings: map[string][]toolkit.ContentEntry{
			"acme/repo1": {
				{Path: "main.go", IsDir: false},
			},
		},
		searches: map[string][]toolkit.SearchResult{
			"acme/repo1": {
				{Path: "main.go", Line: 10, Snippet: "func TestPTP()"},
			},
		},
		files: map[string][]byte{
			"acme/repo1:main.go": []byte("package main\n"),
		},
	}
	catalog := txCatalog()

	// Use embedded circuit YAML (includes synthesize node).
	def, err := circuit.LoadCircuit(DefaultCircuitYAML())
	if err != nil {
		t.Fatalf("LoadCircuit: %v", err)
	}

	gatherComp := TransformerComponent(reader, catalog)
	synthComp := SynthesizeComponent(nil) // deterministic passthrough
	reg, err := engine.MergeComponents(engine.GraphRegistries{}, gatherComp, synthComp)
	if err != nil {
		t.Fatalf("MergeComponents: %v", err)
	}

	g, err := engine.BuildGraph(def, reg)
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}

	walker := circuit.NewProcessWalker("test-gnd-full")
	walker.State().Context["dsr.search_keywords"] = []string{"TestPTP"}

	if err := g.Walk(context.Background(), walker, "tree"); err != nil {
		t.Fatalf("Walk: %v", err)
	}

	// Verify all 4 nodes produced outputs.
	for _, name := range []string{"tree", "search", "read", "synthesize"} {
		if _, ok := walker.State().Outputs[name]; !ok {
			t.Errorf("missing output for node %q", name)
		}
	}

	// In deterministic mode, synthesize passes through the CodeContext.
	synthArt := walker.State().Outputs["synthesize"]
	cc, ok := synthArt.Raw().(*CodeContext)
	if !ok {
		t.Fatalf("synthesize artifact Raw() type = %T, want *CodeContext", synthArt.Raw())
	}
	if len(cc.Files) != 1 {
		t.Errorf("files = %d, want 1", len(cc.Files))
	}
}

func TestCircuit_EmptyCatalog(t *testing.T) {
	reader := &txReader{}
	emptyCatalog := &toolkit.SliceCatalog{}

	def, err := circuit.LoadCircuit([]byte(gatherOnlyCircuitYAML))
	if err != nil {
		t.Fatalf("LoadCircuit: %v", err)
	}

	comp := TransformerComponent(reader, emptyCatalog)
	reg, err := engine.MergeComponents(engine.GraphRegistries{}, comp)
	if err != nil {
		t.Fatalf("MergeComponents: %v", err)
	}

	g, err := engine.BuildGraph(def, reg)
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}

	walker := circuit.NewProcessWalker("test-empty")
	if err := g.Walk(context.Background(), walker, "tree"); err != nil {
		t.Fatalf("Walk: %v", err)
	}

	readArt := walker.State().Outputs["read"]
	cc := readArt.Raw().(*CodeContext)
	if len(cc.Files) != 0 {
		t.Errorf("expected no files for empty catalog, got %d", len(cc.Files))
	}
}
