package dsr

import (
	"context"
	"testing"

	framework "github.com/dpopsuev/origami"
	"github.com/dpopsuev/origami/schematics/toolkit"
)

type txReader struct {
	ensured  []toolkit.Source
	listings map[string][]toolkit.ContentEntry
	searches map[string][]toolkit.SearchResult
	files    map[string][]byte
}

func (s *txReader) Ensure(_ context.Context, src toolkit.Source) error {
	s.ensured = append(s.ensured, src)
	return nil
}

func (s *txReader) List(_ context.Context, src toolkit.Source, _ string, _ int) ([]toolkit.ContentEntry, error) {
	key := src.Org + "/" + src.Name
	return s.listings[key], nil
}

func (s *txReader) Search(_ context.Context, src toolkit.Source, _ string, _ int) ([]toolkit.SearchResult, error) {
	key := src.Org + "/" + src.Name
	return s.searches[key], nil
}

func (s *txReader) Read(_ context.Context, src toolkit.Source, path string) ([]byte, error) {
	key := src.Org + "/" + src.Name + ":" + path
	if data, ok := s.files[key]; ok {
		return data, nil
	}
	return nil, nil
}

func txCatalog() toolkit.SourceCatalog {
	return &toolkit.SliceCatalog{
		Items: []toolkit.Source{
			{Org: "acme", Name: "repo1", Kind: toolkit.SourceKindRepo, Branch: "main"},
		},
	}
}

func TestTreeTransformer(t *testing.T) {
	reader := &txReader{
		listings: map[string][]toolkit.ContentEntry{
			"acme/repo1": {
				{Path: "cmd/main.go", IsDir: false},
				{Path: "pkg/", IsDir: true},
			},
		},
	}
	tr := newTreeTransformer(reader, txCatalog())

	result, err := tr.Transform(context.Background(), &framework.TransformerContext{
		WalkerState: &framework.WalkerState{Context: map[string]any{}},
	})
	if err != nil {
		t.Fatalf("Transform() error: %v", err)
	}

	trees, ok := result.([]RepoTree)
	if !ok {
		t.Fatalf("result type = %T, want []RepoTree", result)
	}
	if len(trees) != 1 {
		t.Fatalf("len(trees) = %d, want 1", len(trees))
	}
	if trees[0].Repo != "acme/repo1" {
		t.Errorf("repo = %q, want %q", trees[0].Repo, "acme/repo1")
	}
	if len(trees[0].Entries) != 2 {
		t.Errorf("entries = %d, want 2", len(trees[0].Entries))
	}
	if len(reader.ensured) != 1 {
		t.Errorf("ensured = %d, want 1 (Ensure called for each repo source)", len(reader.ensured))
	}
}

func TestSearchTransformer(t *testing.T) {
	reader := &txReader{
		searches: map[string][]toolkit.SearchResult{
			"acme/repo1": {
				{Path: "pkg/handler.go", Line: 42, Snippet: "func TestPTP"},
			},
		},
	}
	tr := newSearchTransformer(reader, txCatalog())

	ws := &framework.WalkerState{
		Context: map[string]any{
			"dsr.search_keywords": []string{"TestPTP"},
		},
	}

	result, err := tr.Transform(context.Background(), &framework.TransformerContext{WalkerState: ws})
	if err != nil {
		t.Fatalf("Transform() error: %v", err)
	}

	hits, ok := result.([]SearchHit)
	if !ok {
		t.Fatalf("result type = %T, want []SearchHit", result)
	}
	if len(hits) != 1 {
		t.Fatalf("len(hits) = %d, want 1", len(hits))
	}
	if hits[0].Repo != "acme/repo1" {
		t.Errorf("repo = %q, want %q", hits[0].Repo, "acme/repo1")
	}
}

func TestSearchTransformer_NoKeywords(t *testing.T) {
	reader := &txReader{}
	tr := newSearchTransformer(reader, txCatalog())

	ws := &framework.WalkerState{Context: map[string]any{}}
	result, err := tr.Transform(context.Background(), &framework.TransformerContext{WalkerState: ws})
	if err != nil {
		t.Fatalf("Transform() error: %v", err)
	}

	hits, ok := result.([]SearchHit)
	if !ok {
		t.Fatalf("result type = %T, want []SearchHit", result)
	}
	if hits != nil {
		t.Errorf("expected nil hits when no keywords, got %d", len(hits))
	}
}

func TestReadTransformer(t *testing.T) {
	reader := &txReader{
		files: map[string][]byte{
			"acme/repo1:pkg/handler.go": []byte("package pkg\n\nfunc TestPTP() {}"),
		},
	}
	tr := newReadTransformer(reader, txCatalog())

	// Simulate walker state with prior node outputs.
	trees := []RepoTree{{Repo: "acme/repo1", Branch: "main"}}
	hits := []SearchHit{{Repo: "acme/repo1", File: "pkg/handler.go", Line: 42}}

	ws := &framework.WalkerState{
		Context: map[string]any{},
		Outputs: map[string]framework.Artifact{
			"tree":   &testArtifact{raw: trees},
			"search": &testArtifact{raw: hits},
		},
	}

	result, err := tr.Transform(context.Background(), &framework.TransformerContext{WalkerState: ws})
	if err != nil {
		t.Fatalf("Transform() error: %v", err)
	}

	cc, ok := result.(*CodeContext)
	if !ok {
		t.Fatalf("result type = %T, want *CodeContext", result)
	}
	if len(cc.Trees) != 1 {
		t.Errorf("trees = %d, want 1", len(cc.Trees))
	}
	if len(cc.Files) != 1 {
		t.Fatalf("files = %d, want 1", len(cc.Files))
	}
	if cc.Files[0].Content != "package pkg\n\nfunc TestPTP() {}" {
		t.Errorf("content = %q", cc.Files[0].Content)
	}
}

func TestReadTransformer_TokenBudget(t *testing.T) {
	bigContent := make([]byte, maxTokenBudget+100)
	for i := range bigContent {
		bigContent[i] = 'x'
	}
	reader := &txReader{
		files: map[string][]byte{
			"acme/repo1:big.go": bigContent,
		},
	}
	tr := newReadTransformer(reader, txCatalog())

	ws := &framework.WalkerState{
		Context: map[string]any{},
		Outputs: map[string]framework.Artifact{
			"tree":   &testArtifact{raw: []RepoTree(nil)},
			"search": &testArtifact{raw: []SearchHit{{Repo: "acme/repo1", File: "big.go"}}},
		},
	}

	result, err := tr.Transform(context.Background(), &framework.TransformerContext{WalkerState: ws})
	if err != nil {
		t.Fatalf("Transform() error: %v", err)
	}

	cc := result.(*CodeContext)
	if !cc.Files[0].Truncated {
		t.Error("expected file to be truncated")
	}
	if len(cc.Files[0].Content) != maxTokenBudget {
		t.Errorf("content length = %d, want %d", len(cc.Files[0].Content), maxTokenBudget)
	}
}

func TestTransformerComponent(t *testing.T) {
	reader := &txReader{}
	catalog := txCatalog()

	comp := TransformerComponent(reader, catalog)
	if comp.Namespace != "dsr" {
		t.Errorf("namespace = %q, want %q", comp.Namespace, "dsr")
	}
	for _, name := range []string{"tree", "search", "read"} {
		if _, ok := comp.Transformers[name]; !ok {
			t.Errorf("transformer %q not registered", name)
		}
	}
}

func TestSplitRepoKey(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"acme/repo1", []string{"acme", "repo1"}},
		{"no-slash", nil},
	}
	for _, tt := range tests {
		got := splitRepoKey(tt.input)
		if tt.want == nil && got != nil {
			t.Errorf("splitRepoKey(%q) = %v, want nil", tt.input, got)
		}
		if tt.want != nil {
			if got == nil || got[0] != tt.want[0] || got[1] != tt.want[1] {
				t.Errorf("splitRepoKey(%q) = %v, want %v", tt.input, got, tt.want)
			}
		}
	}
}

// testArtifact implements framework.Artifact for testing.
type testArtifact struct {
	raw any
}

func (a *testArtifact) Type() string        { return "test" }
func (a *testArtifact) Confidence() float64  { return 1.0 }
func (a *testArtifact) Raw() any             { return a.raw }
