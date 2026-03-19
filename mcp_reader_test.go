package dsr_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	dsr "github.com/dpopsuev/rh-gnd"
	"github.com/dpopsuev/origami/schematics/toolkit"
)

type mockToolCaller struct {
	calls []mockCall
	err   error
}

type mockCall struct {
	Name string
	Args map[string]any
}

func (m *mockToolCaller) CallTool(_ context.Context, name string, args map[string]any) (*sdkmcp.CallToolResult, error) {
	m.calls = append(m.calls, mockCall{Name: name, Args: args})
	if m.err != nil {
		return nil, m.err
	}

	var text string
	switch name {
	case "gnd_ensure":
		text = "ok"
	case "gnd_search":
		results := []toolkit.SearchResult{{Source: "test", Path: "main.go", Line: 1, Snippet: "func main()"}}
		data, _ := json.Marshal(results)
		text = string(data)
	case "gnd_read":
		text = "file content here"
	case "gnd_list":
		entries := []toolkit.ContentEntry{{Path: "src/", IsDir: true}, {Path: "main.go", Size: 42}}
		data, _ := json.Marshal(entries)
		text = string(data)
	}

	return &sdkmcp.CallToolResult{
		Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: text}},
	}, nil
}

var testSource = toolkit.Source{
	Name: "test-repo",
	Kind: toolkit.SourceKindRepo,
	URI:  "https://github.com/example/test",
}

func TestMCPReader_Ensure(t *testing.T) {
	mock := &mockToolCaller{}
	reader := dsr.NewMCPReader(mock)

	err := reader.Ensure(context.Background(), testSource)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	if len(mock.calls) != 1 || mock.calls[0].Name != "gnd_ensure" {
		t.Fatalf("expected 1 call to gnd_ensure, got %v", mock.calls)
	}
}

func TestMCPReader_Search(t *testing.T) {
	mock := &mockToolCaller{}
	reader := dsr.NewMCPReader(mock)

	results, err := reader.Search(context.Background(), testSource, "main", 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Path != "main.go" {
		t.Errorf("result path = %q, want main.go", results[0].Path)
	}
	if mock.calls[0].Name != "gnd_search" {
		t.Errorf("tool name = %q, want gnd_search", mock.calls[0].Name)
	}
}

func TestMCPReader_Read(t *testing.T) {
	mock := &mockToolCaller{}
	reader := dsr.NewMCPReader(mock)

	content, err := reader.Read(context.Background(), testSource, "main.go")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	if string(content) != "file content here" {
		t.Errorf("content = %q, want %q", string(content), "file content here")
	}
}

func TestMCPReader_List(t *testing.T) {
	mock := &mockToolCaller{}
	reader := dsr.NewMCPReader(mock)

	entries, err := reader.List(context.Background(), testSource, ".", 2)
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if !entries[0].IsDir {
		t.Errorf("first entry should be a directory")
	}
	if entries[1].Size != 42 {
		t.Errorf("second entry size = %d, want 42", entries[1].Size)
	}
}

func TestMCPReader_TransportError(t *testing.T) {
	mock := &mockToolCaller{err: fmt.Errorf("connection refused")}
	reader := dsr.NewMCPReader(mock)

	_, err := reader.Search(context.Background(), testSource, "test", 5)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestMCPReader_EnsureErrorResult(t *testing.T) {
	errMock := &errorToolCaller{errMsg: "source unavailable"}
	reader := dsr.NewMCPReader(errMock)

	err := reader.Ensure(context.Background(), testSource)
	if err == nil {
		t.Fatal("expected error from error result")
	}
}

type errorToolCaller struct {
	errMsg string
}

func (e *errorToolCaller) CallTool(_ context.Context, _ string, _ map[string]any) (*sdkmcp.CallToolResult, error) {
	return &sdkmcp.CallToolResult{
		IsError: true,
		Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: e.errMsg}},
	}, nil
}
