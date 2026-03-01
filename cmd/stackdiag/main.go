package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/muras3/stackdiag/internal/cli"
	"github.com/muras3/stackdiag/internal/core"
	"github.com/muras3/stackdiag/internal/layers/dns"
	layerhttp "github.com/muras3/stackdiag/internal/layers/http"
	"github.com/muras3/stackdiag/internal/layers/reachability"
	"github.com/muras3/stackdiag/internal/layers/tcp"
	layertls "github.com/muras3/stackdiag/internal/layers/tls"
	renderjson "github.com/muras3/stackdiag/internal/render/json"
	"github.com/muras3/stackdiag/internal/render/table"
	"github.com/muras3/stackdiag/internal/runner"
)

// Set via -ldflags at build time.
var (
	version = "dev"
	commit  = "none"
)

func main() {
	cfg, err := cli.ParseArgs(os.Args[1:])
	if err != nil {
		if cli.HasJSONPrettyFlag(os.Args[1:]) {
			renderjson.RenderToolErrorPretty(os.Stdout, "INVALID_ARGS", err.Error(), 1)
		} else if cli.HasJSONFlag(os.Args[1:]) {
			renderjson.RenderToolError(os.Stdout, "INVALID_ARGS", err.Error(), 1)
		} else {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			fmt.Fprintln(os.Stderr, "\n"+cli.HelpText())
		}
		os.Exit(1)
	}

	if cfg.Help {
		fmt.Fprintln(os.Stdout, cli.HelpText())
		os.Exit(0)
	}

	if cfg.Version {
		fmt.Printf("stdiag %s (%s)\n", version, commit)
		os.Exit(0)
	}

	// Resolve auth from environment variables.
	if err := cli.ResolveAuth(cfg, os.LookupEnv); err != nil {
		exitToolError(cfg.JSON, "INVALID_ARGS", err.Error(), cfg.JSONPretty)
	}

	tgt, err := core.ParseTarget(cfg.Target)
	if err != nil {
		exitToolError(cfg.JSON, "INVALID_TARGET", err.Error(), cfg.JSONPretty)
	}

	// Build layers based on target scheme.
	layers := buildLayers(tgt, cfg.Insecure, cfg.DNSServer)

	// Set up context with signal handling and timeout.
	// signal.NotifyContext cancels on SIGINT/SIGTERM, ensuring graceful shutdown.
	sigCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(sigCtx, time.Duration(cfg.Timeout)*time.Second)
	defer cancel()

	pctx := &core.ProbeContext{
		Context:   ctx,
		Target:    tgt,
		Insecure:  cfg.Insecure,
		Redact:    cfg.Redact,
		Method:    cfg.Method,
		Headers:   cfg.Headers,
		TLSScan:   cfg.TLSScan,
		DNSServer: cfg.DNSServer,
	}

	// Run diagnostics.
	r := runner.New(layers)

	if cfg.Count > 0 {
		// Repeated measurement mode.
		countResult := r.RunCount(cfg.Count, func(attempt int) (*core.ProbeContext, context.CancelFunc) {
			attemptCtx, attemptCancel := context.WithTimeout(sigCtx, time.Duration(cfg.Timeout)*time.Second)
			return &core.ProbeContext{
				Context:   attemptCtx,
				Target:    tgt,
				Insecure:  cfg.Insecure,
				Redact:    cfg.Redact,
				Method:    cfg.Method,
				Headers:   cfg.Headers,
				TLSScan:   cfg.TLSScan,
				DNSServer: cfg.DNSServer,
			}, attemptCancel
		})

		renderAndExit(cfg, countResult.ExitCode, func(buf *bytes.Buffer) error {
			fn := renderjson.RenderCount
			if cfg.JSONPretty {
				fn = renderjson.RenderCountPretty
			}
			return fn(buf, countResult)
		}, func(buf *bytes.Buffer, color bool) error {
			return table.RenderCount(buf, countResult, color)
		})
	}

	// Single run mode.
	result := r.Run(pctx)

	renderAndExit(cfg, result.Summary.ExitCode, func(buf *bytes.Buffer) error {
		fn := renderjson.Render
		if cfg.JSONPretty {
			fn = renderjson.RenderPretty
		}
		return fn(buf, result)
	}, func(buf *bytes.Buffer, color bool) error {
		return table.Render(buf, result, color)
	})
}

// renderAndExit buffers output to prevent partial writes on signal interruption,
// then writes atomically to stdout.
func renderAndExit(cfg *cli.Config, exitCode int, jsonRender func(*bytes.Buffer) error, tableRender func(*bytes.Buffer, bool) error) {
	var buf bytes.Buffer
	if cfg.JSON {
		if err := jsonRender(&buf); err != nil {
			fmt.Fprintf(os.Stderr, "Error rendering JSON: %v\n", err)
			os.Exit(1)
		}
	} else {
		if err := tableRender(&buf, cli.IsColorEnabled(os.LookupEnv)); err != nil {
			fmt.Fprintf(os.Stderr, "Error rendering table: %v\n", err)
			os.Exit(1)
		}
	}
	buf.WriteTo(os.Stdout)
	os.Exit(exitCode)
}

// buildLayers constructs the layer stack based on the target scheme.
func buildLayers(tgt core.Target, insecure bool, dnsServer string) []core.Layer {
	var layers []core.Layer

	if dnsServer != "" {
		layers = append(layers, dns.NewWithServer(dnsServer))
	} else {
		layers = append(layers, dns.NewDefault())
	}
	layers = append(layers, reachability.NewDefault())
	layers = append(layers, tcp.NewDefault())

	if tgt.NeedsTLS() {
		layers = append(layers, layertls.NewDefault())
	}

	if tgt.NeedsHTTP() {
		layers = append(layers, layerhttp.NewDefault(insecure, tgt.Host))
	}

	return layers
}

// exitToolError outputs a structured error and exits with code 1.
func exitToolError(jsonMode bool, code, message string, pretty bool) {
	if jsonMode {
		if pretty {
			renderjson.RenderToolErrorPretty(os.Stdout, code, message, 1)
		} else {
			renderjson.RenderToolError(os.Stdout, code, message, 1)
		}
	} else {
		fmt.Fprintf(os.Stderr, "Error: %v\n", message)
	}
	os.Exit(1)
}

