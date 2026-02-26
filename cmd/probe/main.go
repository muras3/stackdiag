package main

import (
	"fmt"
	"os"
)

// Set via -ldflags at build time.
var (
	version = "dev"
	commit  = "none"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Printf("probe %s (%s)\n", version, commit)
		os.Exit(0)
	}

	fmt.Fprintln(os.Stderr, "Usage: probe <url>")
	os.Exit(1)
}
