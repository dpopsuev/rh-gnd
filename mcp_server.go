package dsr

import (
	"context"
	"encoding/json"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/dpopsuev/origami/schematics/toolkit"
)

// RegisterTools adds the GND schematic's MCP tools to the given server.
// The four tools (ensure, search, read, list) delegate to the AccessRouter.
func RegisterTools(server *sdkmcp.Server, router *AccessRouter) {
	server.AddTool(
		&sdkmcp.Tool{
			Name:        "gnd_ensure",
			Description: "Ensure a GND source is available (e.g. clone a repo)",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"source":{"type":"object","description":"GND source descriptor"}}}`),
		},
		func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
			var args struct {
				Source toolkit.Source `json:"source"`
			}
			if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
				return errToolResult("invalid arguments: " + err.Error()), nil
			}
			if err := router.Ensure(ctx, args.Source); err != nil {
				return errToolResult(err.Error()), nil
			}
			return &sdkmcp.CallToolResult{
				Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: "ok"}},
			}, nil
		},
	)

	server.AddTool(
		&sdkmcp.Tool{
			Name:        "gnd_search",
			Description: "Search a GND source for matching content",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"source":{"type":"object","description":"GND source descriptor"},"query":{"type":"string","description":"Search query"},"max_results":{"type":"integer","description":"Maximum results to return"}}}`),
		},
		func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
			var args struct {
				Source     toolkit.Source `json:"source"`
				Query      string   `json:"query"`
				MaxResults int      `json:"max_results"`
			}
			if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
				return errToolResult("invalid arguments: " + err.Error()), nil
			}
			if args.MaxResults <= 0 {
				args.MaxResults = 10
			}
			results, err := router.Search(ctx, args.Source, args.Query, args.MaxResults)
			if err != nil {
				return errToolResult(err.Error()), nil
			}
			data, err := json.Marshal(results)
			if err != nil {
				return errToolResult("marshal search results: " + err.Error()), nil
			}
			return &sdkmcp.CallToolResult{
				Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: string(data)}},
			}, nil
		},
	)

	server.AddTool(
		&sdkmcp.Tool{
			Name:        "gnd_read",
			Description: "Read content from a GND source at a given path",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"source":{"type":"object","description":"GND source descriptor"},"path":{"type":"string","description":"Path to read"}}}`),
		},
		func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
			var args struct {
				Source toolkit.Source `json:"source"`
				Path   string   `json:"path"`
			}
			if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
				return errToolResult("invalid arguments: " + err.Error()), nil
			}
			content, err := router.Read(ctx, args.Source, args.Path)
			if err != nil {
				return errToolResult(err.Error()), nil
			}
			return &sdkmcp.CallToolResult{
				Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: string(content)}},
			}, nil
		},
	)

	server.AddTool(
		&sdkmcp.Tool{
			Name:        "gnd_list",
			Description: "List contents of a GND source directory",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"source":{"type":"object","description":"GND source descriptor"},"root":{"type":"string","description":"Root path to list from"},"max_depth":{"type":"integer","description":"Maximum directory depth"}}}`),
		},
		func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
			var args struct {
				Source   toolkit.Source `json:"source"`
				Root     string   `json:"root"`
				MaxDepth int      `json:"max_depth"`
			}
			if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
				return errToolResult("invalid arguments: " + err.Error()), nil
			}
			entries, err := router.List(ctx, args.Source, args.Root, args.MaxDepth)
			if err != nil {
				return errToolResult(err.Error()), nil
			}
			data, err := json.Marshal(entries)
			if err != nil {
				return errToolResult("marshal list entries: " + err.Error()), nil
			}
			return &sdkmcp.CallToolResult{
				Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: string(data)}},
			}, nil
		},
	)
}

// SynthesizeToolOpts configures the gnd_synthesize MCP tool.
type SynthesizeToolOpts struct {
	Synthesizer  Synthesizer
	PackResolver PackResolver
	Router       *AccessRouter
}

// RegisterSynthesizeTool adds the gnd_synthesize MCP tool.
// It loads a source pack by name, synthesizes a domain context artifact,
// and returns the result as JSON.
func RegisterSynthesizeTool(server *sdkmcp.Server, opts SynthesizeToolOpts) {
	if opts.Synthesizer == nil {
		opts.Synthesizer = &StructuralSynthesizer{}
	}
	server.AddTool(
		&sdkmcp.Tool{
			Name:        "gnd_synthesize",
			Description: "Synthesize a domain context artifact from a source pack",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"pack_path":{"type":"string","description":"Path to source pack YAML file"},"token_budget":{"type":"integer","description":"Maximum token budget for the artifact"},"sections":{"type":"array","items":{"type":"string"},"description":"Sections to include (component-map, source-index, version-info)"},"attrs":{"type":"object","description":"Attributes for branch resolution (e.g. ocp_version)"}},"required":["pack_path"]}`),
		},
		func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
			var args struct {
				PackPath    string            `json:"pack_path"`
				TokenBudget int               `json:"token_budget"`
				Sections    []string          `json:"sections"`
				Attrs       map[string]string `json:"attrs"`
			}
			if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
				return errToolResult("invalid arguments: " + err.Error()), nil
			}
			if args.PackPath == "" {
				return errToolResult("pack_path is required"), nil
			}

			pack, err := LoadPack(args.PackPath, opts.PackResolver)
			if err != nil {
				return errToolResult("load pack: " + err.Error()), nil
			}

			synthOpts := SynthesisOpts{
				TokenBudget: args.TokenBudget,
				Sections:    args.Sections,
				Attrs:       args.Attrs,
			}

			var reader toolkit.SourceReader
			if opts.Router != nil {
				reader = opts.Router
			}

			artifact, err := opts.Synthesizer.Synthesize(ctx, pack, reader, synthOpts)
			if err != nil {
				return errToolResult("synthesize: " + err.Error()), nil
			}

			data, err := json.Marshal(artifact)
			if err != nil {
				return errToolResult("marshal artifact: " + err.Error()), nil
			}
			return &sdkmcp.CallToolResult{
				Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: string(data)}},
			}, nil
		},
	)
}

func errToolResult(msg string) *sdkmcp.CallToolResult {
	return &sdkmcp.CallToolResult{
		IsError: true,
		Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: msg}},
	}
}
