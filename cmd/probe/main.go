package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/muras3/probe/internal/cli"
	"github.com/muras3/probe/internal/core"
	"github.com/muras3/probe/internal/layers/dns"
	layerhttp "github.com/muras3/probe/internal/layers/http"
	"github.com/muras3/probe/internal/layers/tcp"
	layertls "github.com/muras3/probe/internal/layers/tls"
	renderjson "github.com/muras3/probe/internal/render/json"
	"github.com/muras3/probe/internal/render/table"
	"github.com/muras3/probe/internal/runner"
)

// Set via -ldflags at build time.
var (
	version = "dev"
	commit  = "none"
)

func main() {
	cfg, err := cli.ParseArgs(os.Args[1:])
	if err != nil {
		if err.Error() == "flag: help requested" {
			fmt.Fprintln(os.Stdout, "Usage: probe <url> [--json] [--method METHOD] [--header KEY:VALUE] [--timeout N] [--insecure]")
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		fmt.Fprintln(os.Stderr, "Usage: probe <url> [--json] [--method METHOD] [--header KEY:VALUE] [--timeout N] [--insecure]")
		os.Exit(1)
	}

	if cfg.Version {
		fmt.Printf("probe %s (%s)\n", version, commit)
		os.Exit(0)
	}

	if cfg.Timeout <= 0 {
		fmt.Fprintln(os.Stderr, "Error: timeout must be greater than 0")
		os.Exit(1)
	}

	tgt, err := core.ParseTarget(cfg.Target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Build layers based on target scheme.
	layers := buildLayers(tgt, cfg.Insecure)

	// Set up context with timeout.
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.Timeout)*time.Second)
	defer cancel()

	pctx := &core.ProbeContext{
		Context:  ctx,
		Target:   tgt,
		Insecure: cfg.Insecure,
		Method:   cfg.Method,
		Headers:  cfg.Headers,
	}

	// Run probe.
	r := runner.New(layers)
	result := r.Run(pctx)

	// Render output: stdout = data, stderr = logs.
	if cfg.JSON {
		if err := renderjson.Render(os.Stdout, result); err != nil {
			fmt.Fprintf(os.Stderr, "Error rendering JSON: %v\n", err)
			os.Exit(1)
		}
	} else {
		useColor := isColorEnabled()
		if err := table.Render(os.Stdout, result, useColor); err != nil {
			fmt.Fprintf(os.Stderr, "Error rendering table: %v\n", err)
			os.Exit(1)
		}
	}

	os.Exit(result.Summary.ExitCode)
}

// buildLayers constructs the layer stack based on the target scheme.
func buildLayers(tgt core.Target, insecure bool) []core.Layer {
	var layers []core.Layer

	layers = append(layers, dns.NewDefault())
	layers = append(layers, tcp.NewDefault())

	if tgt.NeedsTLS() {
		layers = append(layers, layertls.NewDefault())
	}

	if tgt.NeedsHTTP() {
		layers = append(layers, layerhttp.NewDefault(insecure, tgt.Host))
	}

	return layers
}

// isColorEnabled checks if color output should be used.
func isColorEnabled() bool {
	// Respect NO_COLOR convention (https://no-color.org/).
	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		return false
	}
	// Check if stdout is a terminal.
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
