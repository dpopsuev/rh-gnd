package dsr

import (
	"testing"

	"github.com/dpopsuev/origami/schematics/toolkit"
)

var testCatalog = &toolkit.SliceCatalog{
	Items: []toolkit.Source{
		{Name: "operator", Kind: toolkit.SourceKindRepo, URI: "/op", Tags: map[string]string{"component": "ptp", "role": "operator"}},
		{Name: "tests", Kind: toolkit.SourceKindRepo, URI: "/tests", Tags: map[string]string{"component": "ptp", "role": "tests"}},
		{Name: "cloud-events", Kind: toolkit.SourceKindRepo, URI: "/ce", Tags: map[string]string{"component": "cep", "role": "proxy"}},
		{Name: "runbook", Kind: toolkit.SourceKindDoc, URI: "/docs/runbook.md"},
	},
}

func TestTagMatchRule_Match(t *testing.T) {
	rule := TagMatchRule{Required: map[string]string{"component": "ptp"}}
	router := NewSourceRouter(testCatalog, rule)
	got := router.Route(RouteRequest{})

	if len(got) != 2 {
		t.Fatalf("want 2 ptp sources, got %d", len(got))
	}
	for _, s := range got {
		if s.Tags["component"] != "ptp" {
			t.Errorf("unexpected source: %s", s.Name)
		}
	}
}

func TestTagMatchRule_MultipleRequired(t *testing.T) {
	rule := TagMatchRule{Required: map[string]string{"component": "ptp", "role": "operator"}}
	router := NewSourceRouter(testCatalog, rule)
	got := router.Route(RouteRequest{})

	if len(got) != 1 || got[0].Name != "operator" {
		t.Errorf("want only 'operator', got %v", got)
	}
}

func TestRouter_NoRules_ReturnsAll(t *testing.T) {
	router := NewSourceRouter(testCatalog)
	got := router.Route(RouteRequest{})

	if len(got) != 4 {
		t.Errorf("want all 4, got %d", len(got))
	}
}

func TestRouter_NoMatch_ReturnsAll(t *testing.T) {
	rule := TagMatchRule{Required: map[string]string{"component": "nonexistent"}}
	router := NewSourceRouter(testCatalog, rule)
	got := router.Route(RouteRequest{})

	if len(got) != 4 {
		t.Errorf("no match should return all 4, got %d", len(got))
	}
}

func TestRouter_NilCatalog(t *testing.T) {
	router := NewSourceRouter(nil, TagMatchRule{Required: map[string]string{"x": "y"}})
	got := router.Route(RouteRequest{})

	if got != nil {
		t.Errorf("nil catalog should return nil, got %v", got)
	}
}

func TestRouter_EmptyCatalog(t *testing.T) {
	empty := &toolkit.SliceCatalog{}
	router := NewSourceRouter(empty, TagMatchRule{Required: map[string]string{"x": "y"}})
	got := router.Route(RouteRequest{})

	if len(got) != 0 {
		t.Errorf("empty catalog should return empty, got %d", len(got))
	}
}

func TestRouter_MultipleRules_AnyMatch(t *testing.T) {
	rule1 := TagMatchRule{Required: map[string]string{"component": "ptp", "role": "operator"}}
	rule2 := TagMatchRule{Required: map[string]string{"component": "cep"}}
	router := NewSourceRouter(testCatalog, rule1, rule2)
	got := router.Route(RouteRequest{})

	if len(got) != 2 {
		t.Fatalf("want 2 (operator + cloud-events), got %d", len(got))
	}
	names := map[string]bool{}
	for _, s := range got {
		names[s.Name] = true
	}
	if !names["operator"] || !names["cloud-events"] {
		t.Errorf("expected operator and cloud-events, got %v", names)
	}
}

func TestRequestTagMatchRule_OverlappingTags(t *testing.T) {
	rule := RequestTagMatchRule{}
	router := NewSourceRouter(testCatalog, rule)

	got := router.Route(RouteRequest{Tags: map[string]string{"component": "ptp"}})
	names := map[string]bool{}
	for _, s := range got {
		names[s.Name] = true
	}
	if !names["operator"] || !names["tests"] || !names["runbook"] {
		t.Errorf("expected operator, tests, runbook; got %v", names)
	}
	if names["cloud-events"] {
		t.Errorf("cloud-events has component=cep, should not match component=ptp")
	}
}

func TestRequestTagMatchRule_NoReqTags_MatchesAll(t *testing.T) {
	rule := RequestTagMatchRule{}
	router := NewSourceRouter(testCatalog, rule)

	got := router.Route(RouteRequest{})
	if len(got) != 4 {
		t.Errorf("empty request tags should match all, got %d", len(got))
	}
}

func TestRouter_ReturnsDefensiveCopy(t *testing.T) {
	router := NewSourceRouter(testCatalog)
	got := router.Route(RouteRequest{})
	got[0].Name = "mutated"

	if testCatalog.Items[0].Name == "mutated" {
		t.Error("Route() should return a copy, not a reference to catalog sources")
	}
}

var alwaysReadCatalog = &toolkit.SliceCatalog{
	Items: []toolkit.Source{
		{Name: "arch-doc", Kind: toolkit.SourceKindDoc, URI: "/docs/arch.md", ReadPolicy: toolkit.ReadAlways},
		{Name: "operator", Kind: toolkit.SourceKindRepo, URI: "/op", Tags: map[string]string{"component": "ptp"}},
		{Name: "tests", Kind: toolkit.SourceKindRepo, URI: "/tests", Tags: map[string]string{"component": "ptp"}},
		{Name: "cloud-events", Kind: toolkit.SourceKindRepo, URI: "/ce", Tags: map[string]string{"component": "cep"}},
	},
}

func TestRouter_AlwaysRead_IncludedRegardlessOfRules(t *testing.T) {
	rule := TagMatchRule{Required: map[string]string{"component": "cep"}}
	router := NewSourceRouter(alwaysReadCatalog, rule)
	got := router.Route(RouteRequest{})

	names := map[string]bool{}
	for _, s := range got {
		names[s.Name] = true
	}
	if !names["arch-doc"] {
		t.Error("ReadAlways source 'arch-doc' should always be included")
	}
	if !names["cloud-events"] {
		t.Error("rule-matched 'cloud-events' should be included")
	}
	if names["operator"] || names["tests"] {
		t.Error("non-matching conditional sources should be excluded")
	}
}

func TestRouter_AlwaysRead_Dedup(t *testing.T) {
	cat := &toolkit.SliceCatalog{
		Items: []toolkit.Source{
			{Name: "arch-doc", Kind: toolkit.SourceKindDoc, URI: "/docs/arch.md", ReadPolicy: toolkit.ReadAlways, Tags: map[string]string{"component": "ptp"}},
			{Name: "operator", Kind: toolkit.SourceKindRepo, URI: "/op", Tags: map[string]string{"component": "ptp"}},
		},
	}
	rule := TagMatchRule{Required: map[string]string{"component": "ptp"}}
	router := NewSourceRouter(cat, rule)
	got := router.Route(RouteRequest{})

	count := 0
	for _, s := range got {
		if s.Name == "arch-doc" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("arch-doc should appear exactly once, got %d", count)
	}
}

func TestRouter_OnlyAlwaysRead_FallbackToAll(t *testing.T) {
	rule := TagMatchRule{Required: map[string]string{"component": "nonexistent"}}
	router := NewSourceRouter(alwaysReadCatalog, rule)
	got := router.Route(RouteRequest{})

	if len(got) != 4 {
		t.Errorf("when only always-read sources match, should return all; got %d", len(got))
	}
}

func TestSource_IsAlwaysRead(t *testing.T) {
	s := toolkit.Source{ReadPolicy: toolkit.ReadAlways}
	if !s.IsAlwaysRead() {
		t.Error("ReadAlways should return true")
	}
	s2 := toolkit.Source{}
	if s2.IsAlwaysRead() {
		t.Error("empty ReadPolicy should return false")
	}
	s3 := toolkit.Source{ReadPolicy: toolkit.ReadConditional}
	if s3.IsAlwaysRead() {
		t.Error("ReadConditional should return false")
	}
}

func TestCatalog_AlwaysReadSources(t *testing.T) {
	got := alwaysReadCatalog.AlwaysReadSources()
	if len(got) != 1 || got[0].Name != "arch-doc" {
		t.Errorf("want [arch-doc], got %v", got)
	}
}

func TestCatalog_AlwaysReadSources_Nil(t *testing.T) {
	var cat *toolkit.SliceCatalog
	got := cat.AlwaysReadSources()
	if got != nil {
		t.Errorf("nil catalog should return nil, got %v", got)
	}
}

func TestRouter_LayeredRoute(t *testing.T) {
	catalog := &toolkit.SliceCatalog{
		Items: []toolkit.Source{
			{Name: "base-repo", Kind: toolkit.SourceKindRepo, Tags: map[string]string{"layer": "base", "component": "ptp"}},
			{Name: "version-docs", Kind: toolkit.SourceKindDoc, Tags: map[string]string{"layer": "version", "version": "4.21"}},
			{Name: "investigation-log", Kind: toolkit.SourceKindDoc, Tags: map[string]string{"layer": "investigation", "launch": "33195"}},
			{Name: "untagged", Kind: toolkit.SourceKindRepo},
			{Name: "always-doc", Kind: toolkit.SourceKindDoc, ReadPolicy: toolkit.ReadAlways},
		},
	}
	router := NewSourceRouter(catalog, RequestTagMatchRule{})

	got := router.LayeredRoute(
		map[string]string{"layer": "base"},
		map[string]string{"layer": "version", "version": "4.21"},
		map[string]string{"layer": "investigation", "launch": "33195"},
	)

	names := make(map[string]bool)
	for _, s := range got {
		names[s.Name] = true
	}

	if !names["always-doc"] {
		t.Error("always-read source should be included")
	}
	if !names["base-repo"] {
		t.Error("base layer source should be included")
	}
	if !names["version-docs"] {
		t.Error("version layer source should be included")
	}
	if !names["investigation-log"] {
		t.Error("investigation layer source should be included")
	}
	if names["untagged"] {
		t.Error("untagged source should NOT be included")
	}
}

func TestRouter_LayeredRoute_EmptyLayers(t *testing.T) {
	router := NewSourceRouter(testCatalog, RequestTagMatchRule{})
	got := router.LayeredRoute(nil, nil, nil)
	if len(got) != len(testCatalog.Items) {
		t.Errorf("empty layers should return all sources, got %d", len(got))
	}
}

func TestRouter_LayeredRoute_NilCatalog(t *testing.T) {
	router := NewSourceRouter(nil)
	got := router.LayeredRoute(
		map[string]string{"layer": "base"},
		nil, nil,
	)
	if got != nil {
		t.Error("nil catalog should return nil")
	}
}
