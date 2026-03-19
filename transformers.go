package dsr

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	framework "github.com/dpopsuev/origami"
	"github.com/dpopsuev/origami/dispatch"
	"github.com/dpopsuev/origami/schematics/toolkit"
)

const maxTokenBudget = 32000

// CodeContext is the aggregate output of the GND circuit.
// It contains the file trees, search results, and file contents
// gathered from all configured sources.
type CodeContext struct {
	Trees         []RepoTree     `json:"trees,omitempty"`
	SearchResults []SearchHit    `json:"search_results,omitempty"`
	Files         []FileContent  `json:"files,omitempty"`
	Truncated     bool           `json:"truncated,omitempty"`
}

// RepoTree holds a repository's file tree listing.
type RepoTree struct {
	Repo    string             `json:"repo"`
	Branch  string             `json:"branch"`
	Entries []toolkit.ContentEntry `json:"entries"`
}

// SearchHit holds a single code search match.
type SearchHit struct {
	Repo    string  `json:"repo"`
	File    string  `json:"file"`
	Line    int     `json:"line"`
	Snippet string  `json:"snippet"`
}

// FileContent holds the content of a single source file.
type FileContent struct {
	Repo      string `json:"repo"`
	Path      string `json:"path"`
	Content   string `json:"content"`
	Truncated bool   `json:"truncated,omitempty"`
}

// TransformerComponent returns a Component containing the GND circuit's
// deterministic transformers: tree, search, read. These wrap SourceReader
// operations and require no LLM dispatch.
func TransformerComponent(reader toolkit.SourceReader, catalog toolkit.SourceCatalog) *framework.Component {
	return &framework.Component{
		Namespace:   "dsr",
		Name:        "dsr-transformers",
		Transformers: framework.TransformerRegistry{
			"tree":   newTreeTransformer(reader, catalog),
			"search": newSearchTransformer(reader, catalog),
			"read":   newReadTransformer(reader, catalog),
		},
	}
}

// SynthesizeComponent returns a Component containing the synthesize transformer.
// When disp is nil, the transformer passes through CodeContext from the read
// node (deterministic mode). When set, it builds a prompt and dispatches via
// the MuxDispatcher for LLM synthesis.
func SynthesizeComponent(disp dispatch.Dispatcher) *framework.Component {
	return &framework.Component{
		Namespace: "dsr",
		Name:      "dsr-synthesize",
		Transformers: framework.TransformerRegistry{
			"synthesize": &synthesizeTransformer{dispatcher: disp},
		},
	}
}

// synthesizeTransformer reads CodeContext from the read node output, builds
// a prompt, and dispatches via MuxDispatcher. If no dispatcher is set
// (stub/deterministic mode), it passes through the raw CodeContext.
type synthesizeTransformer struct {
	dispatcher dispatch.Dispatcher // nil = deterministic passthrough
}

func (t *synthesizeTransformer) Name() string { return "dsr.synthesize" }

func (t *synthesizeTransformer) Deterministic() bool { return t.dispatcher == nil }

func (t *synthesizeTransformer) Transform(ctx context.Context, tc *framework.TransformerContext) (any, error) {
	cc, ok := outputArtifact[*CodeContext](tc.WalkerState, "read")
	if !ok {
		return &CodeContext{}, nil
	}

	if t.dispatcher == nil {
		return cc, nil
	}

	prompt := buildSynthesizePrompt(cc)

	caseLabel := tc.WalkerState.ID
	data, err := t.dispatcher.Dispatch(ctx, dispatch.DispatchContext{
		CaseID:        caseLabel,
		Step:          "synthesize",
		PromptContent: prompt,
	})
	if err != nil {
		return nil, fmt.Errorf("dsr.synthesize dispatch: %w", err)
	}

	var result map[string]any
	if err := json.Unmarshal(cleanJSON(data), &result); err != nil {
		return &SynthesizeResult{
			Summary:     string(data),
			CodeContext: cc,
		}, nil
	}

	sr := &SynthesizeResult{CodeContext: cc}
	if s, ok := result["summary"].(string); ok {
		sr.Summary = s
	}
	if kf, ok := result["key_findings"].([]any); ok {
		for _, f := range kf {
			if s, ok := f.(string); ok {
				sr.KeyFindings = append(sr.KeyFindings, s)
			}
		}
	}
	return sr, nil
}

// SynthesizeResult is the output of the synthesize node.
type SynthesizeResult struct {
	Summary     string       `json:"summary"`
	KeyFindings []string     `json:"key_findings,omitempty"`
	CodeContext *CodeContext  `json:"code_context,omitempty"`
}

func buildSynthesizePrompt(cc *CodeContext) string {
	var b strings.Builder
	b.WriteString("Synthesize the following gathered code context into a cohesive summary.\n\n")

	if len(cc.Trees) > 0 {
		b.WriteString("## Repository Trees\n\n")
		for _, t := range cc.Trees {
			b.WriteString(fmt.Sprintf("### %s (branch: %s)\n", t.Repo, t.Branch))
			for _, e := range t.Entries {
				b.WriteString(fmt.Sprintf("  %s\n", e.Path))
			}
			b.WriteString("\n")
		}
	}

	if len(cc.SearchResults) > 0 {
		b.WriteString("## Search Results\n\n")
		for _, sr := range cc.SearchResults {
			b.WriteString(fmt.Sprintf("- %s:%s:%d — %s\n", sr.Repo, sr.File, sr.Line, sr.Snippet))
		}
		b.WriteString("\n")
	}

	if len(cc.Files) > 0 {
		b.WriteString("## File Contents\n\n")
		for _, f := range cc.Files {
			b.WriteString(fmt.Sprintf("### %s:%s\n```\n%s\n```\n\n", f.Repo, f.Path, f.Content))
		}
	}

	b.WriteString("Respond with JSON: {\"summary\": \"...\", \"key_findings\": [\"...\"]}\n")
	return b.String()
}

// cleanJSON strips markdown code fences from LLM responses.
func cleanJSON(data []byte) []byte {
	s := strings.TrimSpace(string(data))
	if idx := strings.Index(s, "```json"); idx >= 0 {
		s = s[idx+len("```json"):]
		if end := strings.LastIndex(s, "```"); end >= 0 {
			s = s[:end]
		}
		return []byte(strings.TrimSpace(s))
	}
	if idx := strings.Index(s, "```"); idx >= 0 {
		s = s[idx+len("```"):]
		if end := strings.LastIndex(s, "```"); end >= 0 {
			s = s[:end]
		}
		return []byte(strings.TrimSpace(s))
	}
	return data
}

// treeTransformer ensures all repo sources are available and lists
// their file trees. Produces a []RepoTree artifact.
type treeTransformer struct {
	reader  toolkit.SourceReader
	catalog toolkit.SourceCatalog
}

func newTreeTransformer(r toolkit.SourceReader, c toolkit.SourceCatalog) *treeTransformer {
	return &treeTransformer{reader: r, catalog: c}
}

func (t *treeTransformer) Name() string { return "dsr.tree" }
func (t *treeTransformer) IsDeterministic() bool { return true }

func (t *treeTransformer) Transform(ctx context.Context, _ *framework.TransformerContext) (any, error) {
	if t.catalog == nil {
		return &CodeContext{}, nil
	}

	var trees []RepoTree
	for _, src := range t.catalog.Sources() {
		if src.Kind != toolkit.SourceKindRepo {
			continue
		}
		if err := t.reader.Ensure(ctx, src); err != nil {
			continue
		}
		entries, err := t.reader.List(ctx, src, "", 3)
		if err != nil {
			continue
		}
		trees = append(trees, RepoTree{
			Repo:    fmt.Sprintf("%s/%s", src.Org, src.Name),
			Branch:  src.Branch,
			Entries: entries,
		})
	}
	return trees, nil
}

// searchTransformer searches all repo sources for relevant code using
// keywords from the inherited walker context. Produces a []SearchHit artifact.
type searchTransformer struct {
	reader  toolkit.SourceReader
	catalog toolkit.SourceCatalog
}

func newSearchTransformer(r toolkit.SourceReader, c toolkit.SourceCatalog) *searchTransformer {
	return &searchTransformer{reader: r, catalog: c}
}

func (t *searchTransformer) Name() string { return "dsr.search" }
func (t *searchTransformer) IsDeterministic() bool { return true }

func (t *searchTransformer) Transform(ctx context.Context, tc *framework.TransformerContext) (any, error) {
	keywords := extractSearchKeywords(tc.WalkerState)
	if len(keywords) == 0 || t.catalog == nil {
		return []SearchHit(nil), nil
	}

	query := keywords[0]
	for _, kw := range keywords[1:] {
		query += " " + kw
	}

	var hits []SearchHit
	for _, src := range t.catalog.Sources() {
		if src.Kind != toolkit.SourceKindRepo {
			continue
		}
		results, err := t.reader.Search(ctx, src, query, 20)
		if err != nil {
			continue
		}
		repoName := fmt.Sprintf("%s/%s", src.Org, src.Name)
		for _, r := range results {
			hits = append(hits, SearchHit{
				Repo:    repoName,
				File:    r.Path,
				Line:    r.Line,
				Snippet: r.Snippet,
			})
		}
	}
	return hits, nil
}

// readTransformer reads files identified by search results. Produces
// a *CodeContext artifact containing the complete code context.
type readTransformer struct {
	reader  toolkit.SourceReader
	catalog toolkit.SourceCatalog
}

func newReadTransformer(r toolkit.SourceReader, c toolkit.SourceCatalog) *readTransformer {
	return &readTransformer{reader: r, catalog: c}
}

func (t *readTransformer) Name() string { return "dsr.read" }
func (t *readTransformer) IsDeterministic() bool { return true }

func (t *readTransformer) Transform(ctx context.Context, tc *framework.TransformerContext) (any, error) {
	cc := &CodeContext{}

	// Collect trees from tree node output.
	if trees, ok := outputArtifact[[]RepoTree](tc.WalkerState, "tree"); ok {
		cc.Trees = trees
	}

	// Collect search results from search node output.
	hits, _ := outputArtifact[[]SearchHit](tc.WalkerState, "search")

	seen := make(map[string]bool)
	budgetRemaining := maxTokenBudget
	for _, sr := range hits {
		fileKey := sr.Repo + ":" + sr.File
		if seen[fileKey] {
			continue
		}
		seen[fileKey] = true

		cc.SearchResults = append(cc.SearchResults, sr)

		parts := splitRepoKey(sr.Repo)
		if parts == nil {
			continue
		}
		src := toolkit.Source{
			Org:  parts[0],
			Name: parts[1],
			Kind: toolkit.SourceKindRepo,
		}
		data, err := t.reader.Read(ctx, src, sr.File)
		if err != nil {
			continue
		}

		content := string(data)
		truncated := false
		if len(content) > budgetRemaining {
			content = content[:budgetRemaining]
			truncated = true
		}
		budgetRemaining -= len(content)

		cc.Files = append(cc.Files, FileContent{
			Repo:      sr.Repo,
			Path:      sr.File,
			Content:   content,
			Truncated: truncated,
		})

		if budgetRemaining <= 0 {
			cc.Truncated = true
			break
		}
	}
	return cc, nil
}

// extractSearchKeywords reads search keywords from the walker context.
// It looks for a dedicated "dsr.search_keywords" key first (set by
// the parent circuit), then falls back to reading failure test name and
// prior candidate repos from RCA context keys.
func extractSearchKeywords(ws *framework.WalkerState) []string {
	if ws == nil {
		return nil
	}

	if kw, ok := ws.Context["dsr.search_keywords"].([]string); ok && len(kw) > 0 {
		return kw
	}

	var keywords []string

	// Read test name from failure params (works with any struct that has TestName).
	if fp := ws.Context["params.failure"]; fp != nil {
		if named, ok := fp.(interface{ GetTestName() string }); ok {
			if tn := named.GetTestName(); tn != "" {
				keywords = append(keywords, tn)
			}
		}
		// Fall back to map-based access for generic maps.
		if m, ok := fp.(map[string]any); ok {
			if tn, ok := m["test_name"].(string); ok && tn != "" {
				keywords = append(keywords, tn)
			}
		}
	}

	return keywords
}

// outputArtifact extracts a typed value from a walker's Outputs by node name.
// It unwraps transformerArtifact via Raw() if needed.
func outputArtifact[T any](ws *framework.WalkerState, nodeName string) (T, bool) {
	var zero T
	if ws == nil || ws.Outputs == nil {
		return zero, false
	}
	art, ok := ws.Outputs[nodeName]
	if !ok {
		return zero, false
	}
	if v, ok := art.Raw().(T); ok {
		return v, true
	}
	if v, ok := art.(interface{ Raw() any }); ok {
		if typed, ok := v.Raw().(T); ok {
			return typed, true
		}
	}
	return zero, false
}

func splitRepoKey(key string) []string {
	for i, c := range key {
		if c == '/' {
			return []string{key[:i], key[i+1:]}
		}
	}
	return nil
}
