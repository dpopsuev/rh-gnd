// Command serve runs the GND (Gather & Diffuse) schematic as a Papercup
// circuit server over Streamable HTTP. It exposes the GND circuit
// (tree → search → read → synthesize) via the standard Papercup protocol
// (start_circuit, get_next_step, submit_step, get_report).
//
// Usage: serve [--port=9100] [--drivers=git,docs]
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
	dsr "github.com/dpopsuev/rh-gnd"
	"github.com/dpopsuev/rh-gnd/mcpconfig"
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

	var routerOpts []dsr.RouterOption
	if *drivers != "" {
		for _, name := range strings.Split(*drivers, ",") {
			name = strings.TrimSpace(name)
			switch name {
			case "git":
				d, err := github.DefaultGitDriver()
				if err != nil {
					log.Fatalf("create git driver: %v", err)
				}
				routerOpts = append(routerOpts, dsr.WithGitDriver(d))
				log.Printf("registered driver: git")
			case "docs":
				d, err := docs.DefaultDocsDriver()
				if err != nil {
					log.Fatalf("create docs driver: %v", err)
				}
				routerOpts = append(routerOpts, dsr.WithDocsDriver(d))
				log.Printf("registered driver: docs")
			default:
				log.Fatalf("unknown driver %q (known: git, docs)", name)
			}
		}
	}
	router := dsr.NewRouter(routerOpts...)

	server := mcpconfig.NewServer(
		mcpconfig.WithReader(router),
	)

	mcpHandler := sdkmcp.NewStreamableHTTPHandler(
		func(_ *http.Request) *sdkmcp.Server { return server.MCPServer },
		&sdkmcp.StreamableHTTPOptions{Stateless: false},
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

	log.Printf("gnd circuit server listening on %s", addr)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}
