package dsr_test

import (
	"testing"

	"github.com/dpopsuev/rh-gnd"
	"github.com/dpopsuev/origami/toolkit"
)

func TestDepGraph_TopologicalSort_NoDeps(t *testing.T) {
	g := dsr.NewDepGraph()
	names := []string{"a", "b", "c"}
	sorted, err := g.TopologicalSort(names)
	if err != nil {
		t.Fatal(err)
	}
	if len(sorted) != 3 {
		t.Errorf("got %d items, want 3", len(sorted))
	}
}

func TestDepGraph_TopologicalSort_Order(t *testing.T) {
	g := dsr.NewDepGraph(
		dsr.DepEdge{From: "rp-items", To: "jira-tickets"},
		dsr.DepEdge{From: "rp-items", To: "ci-logs"},
	)
	names := []string{"jira-tickets", "rp-items", "ci-logs"}
	sorted, err := g.TopologicalSort(names)
	if err != nil {
		t.Fatal(err)
	}
	if sorted[0] != "rp-items" {
		t.Errorf("expected rp-items first, got %q", sorted[0])
	}
}

func TestDepGraph_TopologicalSort_Cycle(t *testing.T) {
	g := dsr.NewDepGraph(
		dsr.DepEdge{From: "a", To: "b"},
		dsr.DepEdge{From: "b", To: "a"},
	)
	_, err := g.TopologicalSort([]string{"a", "b"})
	if err == nil {
		t.Fatal("expected cycle error")
	}
}

func TestDepGraph_OrderSources(t *testing.T) {
	g := dsr.NewDepGraph(
		dsr.DepEdge{From: "base", To: "derived"},
	)
	sources := []toolkit.Source{
		{Name: "derived", Kind: toolkit.SourceKindDoc},
		{Name: "base", Kind: toolkit.SourceKindRepo},
	}
	ordered, err := g.OrderSources(sources)
	if err != nil {
		t.Fatal(err)
	}
	if ordered[0].Name != "base" {
		t.Errorf("expected base first, got %q", ordered[0].Name)
	}
	if ordered[1].Name != "derived" {
		t.Errorf("expected derived second, got %q", ordered[1].Name)
	}
}

func TestDepGraph_Nil(t *testing.T) {
	var g *dsr.DepGraph
	sorted, err := g.TopologicalSort([]string{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	if len(sorted) != 2 {
		t.Errorf("nil graph should pass through, got %d", len(sorted))
	}
}
