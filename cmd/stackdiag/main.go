package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
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
		if hasJSONPrettyFlag(os.Args[1:]) {
			renderjson.RenderToolErrorPretty(os.Stdout, "INVALID_ARGS", err.Error(), 1)
		} else if hasJSONFlag(os.Args[1:]) {
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

	if cfg.Timeout <= 0 {
		fmt.Fprintln(os.Stderr, "Error: timeout must be greater than 0")
		os.Exit(1)
	}
	if cfg.Timeout > 300 {
		fmt.Fprintln(os.Stderr, "Error: timeout must be 300 seconds or less")
		os.Exit(1)
	}

	// Auth header injection from environment variables.
	if cfg.BearerEnv != "" {
		envVal := os.Getenv(cfg.BearerEnv)
		if envVal == "" {
			if _, ok := os.LookupEnv(cfg.BearerEnv); !ok {
				exitToolError(cfg.JSON, "INVALID_ARGS", fmt.Sprintf("environment variable %q is not set", cfg.BearerEnv), cfg.JSONPretty)
			} else {
				exitToolError(cfg.JSON, "INVALID_ARGS", fmt.Sprintf("environment variable %q is empty", cfg.BearerEnv), cfg.JSONPretty)
			}
		}
		if strings.TrimSpace(envVal) == "" {
			exitToolError(cfg.JSON, "INVALID_ARGS", fmt.Sprintf("environment variable %q contains only whitespace", cfg.BearerEnv), cfg.JSONPretty)
		}
		cfg.Headers["Authorization"] = "Bearer " + envVal
	}
	if cfg.BasicEnv != "" {
		envVal := os.Getenv(cfg.BasicEnv)
		if envVal == "" {
			if _, ok := os.LookupEnv(cfg.BasicEnv); !ok {
				exitToolError(cfg.JSON, "INVALID_ARGS", fmt.Sprintf("environment variable %q is not set", cfg.BasicEnv), cfg.JSONPretty)
			} else {
				exitToolError(cfg.JSON, "INVALID_ARGS", fmt.Sprintf("environment variable %q is empty", cfg.BasicEnv), cfg.JSONPretty)
			}
		}
		if strings.TrimSpace(envVal) == "" {
			exitToolError(cfg.JSON, "INVALID_ARGS", fmt.Sprintf("environment variable %q contains only whitespace", cfg.BasicEnv), cfg.JSONPretty)
		}
		if !strings.Contains(envVal, ":") {
			exitToolError(cfg.JSON, "INVALID_ARGS", fmt.Sprintf("environment variable %q must be in user:password format", cfg.BasicEnv), cfg.JSONPretty)
		}
		encoded := base64Encode(envVal)
		cfg.Headers["Authorization"] = "Basic " + encoded
	}

	tgt, err := core.ParseTarget(cfg.Target)
	if err != nil {
		exitToolError(cfg.JSON, "INVALID_TARGET", err.Error(), cfg.JSONPretty)
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
		Redact:   cfg.Redact,
		Method:   cfg.Method,
		Headers:  cfg.Headers,
		TLSScan:  cfg.TLSScan,
	}

	// Run diagnostics.
	r := runner.New(layers)

	if cfg.Count > 0 {
		// Repeated measurement mode.
		countResult := r.RunCount(cfg.Count, func(attempt int) (*core.ProbeContext, context.CancelFunc) {
			attemptCtx, attemptCancel := context.WithTimeout(context.Background(), time.Duration(cfg.Timeout)*time.Second)
			return &core.ProbeContext{
				Context:  attemptCtx,
				Target:   tgt,
				Insecure: cfg.Insecure,
				Redact:   cfg.Redact,
				Method:   cfg.Method,
				Headers:  cfg.Headers,
				TLSScan:  cfg.TLSScan,
			}, attemptCancel
		})

		if cfg.JSON {
			renderCountFn := renderjson.RenderCount
			if cfg.JSONPretty {
				renderCountFn = renderjson.RenderCountPretty
			}
			if err := renderCountFn(os.Stdout, countResult); err != nil {
				fmt.Fprintf(os.Stderr, "Error rendering JSON: %v\n", err)
				os.Exit(1)
			}
		} else {
			useColor := isColorEnabled()
			if err := table.RenderCount(os.Stdout, countResult, useColor); err != nil {
				fmt.Fprintf(os.Stderr, "Error rendering table: %v\n", err)
				os.Exit(1)
			}
		}

		os.Exit(countResult.ExitCode)
	}

	// Single run mode.
	result := r.Run(pctx)

	// Render output: stdout = data, stderr = logs.
	if cfg.JSON {
		renderFn := renderjson.Render
		if cfg.JSONPretty {
			renderFn = renderjson.RenderPretty
		}
		if err := renderFn(os.Stdout, result); err != nil {
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

// base64Encode returns the standard base64 encoding of s.
func base64Encode(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}

// hasJSONPrettyFlag scans raw args for --json-pretty before full parsing.
// Uses last-flag-wins semantics to handle --json-pretty --json-pretty=false.
func hasJSONPrettyFlag(args []string) bool {
	result := false
	for _, a := range args {
		if a == "--json-pretty" {
			result = true
		} else if strings.HasPrefix(a, "--json-pretty=") {
			result = parseBoolValue(a[len("--json-pretty="):])
		}
	}
	return result
}

// hasJSONFlag scans raw args for --json or --json-pretty before full parsing.
// Uses last-flag-wins semantics to handle conflicting repeated flags.
func hasJSONFlag(args []string) bool {
	jsonFlag := false
	prettyFlag := false
	for _, a := range args {
		if a == "--json" {
			jsonFlag = true
		} else if strings.HasPrefix(a, "--json=") {
			jsonFlag = parseBoolValue(a[len("--json="):])
		} else if a == "--json-pretty" {
			prettyFlag = true
		} else if strings.HasPrefix(a, "--json-pretty=") {
			prettyFlag = parseBoolValue(a[len("--json-pretty="):])
		}
	}
	return jsonFlag || prettyFlag
}

// parseBoolValue returns true for "true", "1", "t" (case-insensitive).
func parseBoolValue(s string) bool {
	v := strings.ToLower(s)
	return v == "true" || v == "1" || v == "t"
}

// exitToolError outputs a structured error and exits with code 1.
// When jsonMode is true, writes JSON to stdout; otherwise writes plain text to stderr.
func exitToolError(jsonMode bool, code, message string, prettyMode ...bool) {
	pretty := len(prettyMode) > 0 && prettyMode[0]
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
