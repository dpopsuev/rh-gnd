package dsr_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	dsr "github.com/dpopsuev/rh-gnd"
	"github.com/dpopsuev/origami/toolkit"
)

// sessionToolCaller adapts *sdkmcp.ClientSession to subprocess.ToolCaller.
type sessionToolCaller struct {
	session *sdkmcp.ClientSession
}

func (s *sessionToolCaller) CallTool(ctx context.Context, name string, args map[string]any) (*sdkmcp.CallToolResult, error) {
	return s.session.CallTool(ctx, &sdkmcp.CallToolParams{Name: name, Arguments: args})
}

// TestMCPIntegration_StubDriver_RoundTrip wires:
// stub driver -> AccessRouter -> RegisterTools -> StreamableHTTPHandler (httptest)
// -> StreamableClientTransport -> MCPReader -> assert results
func TestMCPIntegration_StubDriver_RoundTrip(t *testing.T) {
	driver := &stubDriver{
		kind:         toolkit.SourceKindRepo,
		searchResult: []toolkit.SearchResult{{Source: "integration", Path: "cmd/main.go", Line: 10, Snippet: "func main()"}},
		readResult:   []byte("package main\n"),
		listResult:   []toolkit.ContentEntry{{Path: "cmd/", IsDir: true}, {Path: "cmd/main.go", Size: 14}},
	}

	router := dsr.NewRouter(dsr.WithGitDriver(driver))

	server := sdkmcp.NewServer(
		&sdkmcp.Implementation{Name: "test-gnd", Version: "v0.1.0"},
		nil,
	)
	dsr.RegisterTools(server, router)

	handler := sdkmcp.NewStreamableHTTPHandler(
		func(_ *http.Request) *sdkmcp.Server { return server },
		&sdkmcp.StreamableHTTPOptions{Stateless: true},
	)
	httpSrv := httptest.NewServer(handler)
	defer httpSrv.Close()

	ctx := t.Context()
	transport := &sdkmcp.StreamableClientTransport{Endpoint: httpSrv.URL}
	client := sdkmcp.NewClient(
		&sdkmcp.Implementation{Name: "test-client", Version: "v0.1.0"},
		nil,
	)
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer session.Close()

	reader := dsr.NewMCPReader(&sessionToolCaller{session: session})

	src := toolkit.Source{Name: "integration-repo", Kind: toolkit.SourceKindRepo, URI: "https://github.com/test/repo"}

	// Ensure
	if err := reader.Ensure(ctx, src); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	// Search
	results, err := reader.Search(ctx, src, "main", 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 search result, got %d", len(results))
	}
	if results[0].Path != "cmd/main.go" {
		t.Errorf("search result path = %q, want %q", results[0].Path, "cmd/main.go")
	}
	if results[0].Line != 10 {
		t.Errorf("search result line = %d, want 10", results[0].Line)
	}

	// Read
	content, err := reader.Read(ctx, src, "cmd/main.go")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if string(content) != "package main\n" {
		t.Errorf("read content = %q, want %q", string(content), "package main\n")
	}

	// List
	entries, err := reader.List(ctx, src, ".", 2)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 list entries, got %d", len(entries))
	}
	if !entries[0].IsDir || entries[0].Path != "cmd/" {
		t.Errorf("first entry = %+v, want dir cmd/", entries[0])
	}
}
