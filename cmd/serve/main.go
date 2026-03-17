// Command serve runs the harvester schematic as an MCP server over
// Streamable HTTP. It exposes harvester Reader operations as MCP tools
// (ensure, search, read, list) for consumption by other schematics.
//
// Usage: serve [--port=9100] [--driver=git,docs]
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/dpopsuev/origami/connectors/docs"
	"github.com/dpopsuev/origami/connectors/github"
	dsr "github.com/dpopsuev/rh-dsr"
)

func main() {
	port := flag.Int("port", 9100, "HTTP port for the MCP server")
	healthz := flag.Bool("healthz", false, "probe /healthz and exit")
	drivers := flag.String("drivers", "", "comma-separated driver names to register (e.g. git,docs)")
	flag.Parse()

	if *healthz {
		resp, err := http.Get(fmt.Sprintf("http://localhost:%d/healthz", *port))
		if err != nil || resp.StatusCode != http.StatusOK {
			os.Exit(1)
		}
		os.Exit(0)
	}

	var opts []dsr.RouterOption
	if *drivers != "" {
		for _, name := range strings.Split(*drivers, ",") {
			name = strings.TrimSpace(name)
			switch name {
			case "git":
				d, err := github.DefaultGitDriver()
				if err != nil {
					log.Fatalf("create git driver: %v", err)
				}
				opts = append(opts, dsr.WithGitDriver(d))
				log.Printf("registered driver: git")
			case "docs":
				d, err := docs.DefaultDocsDriver()
				if err != nil {
					log.Fatalf("create docs driver: %v", err)
				}
				opts = append(opts, dsr.WithDocsDriver(d))
				log.Printf("registered driver: docs")
			default:
				log.Fatalf("unknown driver %q (known: git, docs)", name)
			}
		}
	}
	router := dsr.NewRouter(opts...)

	server := sdkmcp.NewServer(
		&sdkmcp.Implementation{Name: "origami-harvester", Version: "v0.1.0"},
		nil,
	)

	dsr.RegisterTools(server, router)
	dsr.RegisterSynthesizeTool(server, dsr.SynthesizeToolOpts{
		Synthesizer: &dsr.StructuralSynthesizer{},
		Router:      router,
	})

	mcpHandler := sdkmcp.NewStreamableHTTPHandler(
		func(_ *http.Request) *sdkmcp.Server { return server },
		&sdkmcp.StreamableHTTPOptions{Stateless: true},
	)

	mux := http.NewServeMux()
	mux.Handle("/mcp", mcpHandler)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		if router.Ready() {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	})

	addr := fmt.Sprintf(":%d", *port)
	httpServer := &http.Server{Addr: addr, Handler: mux}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	go func() {
		<-ctx.Done()
		httpServer.Shutdown(context.Background())
	}()

	log.Printf("harvester schematic listening on %s", addr)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}

