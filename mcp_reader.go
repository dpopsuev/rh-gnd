package dsr

import (
	"context"
	"fmt"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/dpopsuev/origami/schematics/toolkit"
	"github.com/dpopsuev/origami/subprocess"
)

// MCPReader implements Reader by delegating to an MCP server via
// subprocess.ToolCaller. It translates Reader method calls into
// MCP tool calls matching the harvester schematic's tool names.
type MCPReader struct {
	caller subprocess.ToolCaller
}

// NewMCPReader creates an MCPReader backed by the given ToolCaller.
func NewMCPReader(caller subprocess.ToolCaller) *MCPReader {
	return &MCPReader{caller: caller}
}

func (r *MCPReader) Ensure(ctx context.Context, src toolkit.Source) error {
	args := map[string]any{"source": src}
	result, err := r.caller.CallTool(ctx, "harvester_ensure", args)
	if err != nil {
		return fmt.Errorf("MCPReader.Ensure: %w", err)
	}
	if result.IsError {
		return fmt.Errorf("MCPReader.Ensure: %s", firstText(result))
	}
	return nil
}

func (r *MCPReader) Search(ctx context.Context, src toolkit.Source, query string, maxResults int) ([]toolkit.SearchResult, error) {
	args := map[string]any{
		"source":      src,
		"query":       query,
		"max_results": maxResults,
	}
	results, err := subprocess.CallToolTyped[[]toolkit.SearchResult](ctx, r.caller, "harvester_search", args)
	if err != nil {
		return nil, fmt.Errorf("MCPReader.Search: %w", err)
	}
	return results, nil
}

func (r *MCPReader) Read(ctx context.Context, src toolkit.Source, path string) ([]byte, error) {
	args := map[string]any{
		"source": src,
		"path":   path,
	}
	result, err := r.caller.CallTool(ctx, "harvester_read", args)
	if err != nil {
		return nil, fmt.Errorf("MCPReader.Read: %w", err)
	}
	if result.IsError {
		return nil, fmt.Errorf("MCPReader.Read: %s", firstText(result))
	}
	return []byte(firstText(result)), nil
}

func (r *MCPReader) List(ctx context.Context, src toolkit.Source, root string, maxDepth int) ([]toolkit.ContentEntry, error) {
	args := map[string]any{
		"source":    src,
		"root":      root,
		"max_depth": maxDepth,
	}
	entries, err := subprocess.CallToolTyped[[]toolkit.ContentEntry](ctx, r.caller, "harvester_list", args)
	if err != nil {
		return nil, fmt.Errorf("MCPReader.List: %w", err)
	}
	return entries, nil
}

func firstText(result *sdkmcp.CallToolResult) string {
	for _, c := range result.Content {
		if tc, ok := c.(*sdkmcp.TextContent); ok {
			return tc.Text
		}
	}
	return ""
}

// Compile-time check.
var _ toolkit.SourceReader = (*MCPReader)(nil)
