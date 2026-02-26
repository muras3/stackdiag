package main

import (
	"fmt"
	"os"

	"github.com/muras3/probe/internal/cli"
	"github.com/muras3/probe/internal/core"
)

// Set via -ldflags at build time.
var (
	version = "dev"
	commit  = "none"
)

func main() {
	cfg, err := cli.ParseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		fmt.Fprintln(os.Stderr, "Usage: probe <url> [--json] [--method METHOD] [--header KEY:VALUE] [--timeout N] [--insecure]")
		os.Exit(1)
	}

	if cfg.Version {
		fmt.Printf("probe %s (%s)\n", version, commit)
		os.Exit(0)
	}

	tgt, err := core.ParseTarget(cfg.Target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Stub: print parsed target info until layers are wired.
	fmt.Fprintf(os.Stderr, "probe: %s (scheme=%s host=%s port=%d)\n", tgt.Original, tgt.Scheme, tgt.Host, tgt.Port)
	fmt.Fprintln(os.Stderr, "layers not yet implemented")
	os.Exit(0)
}
