package cli

import (
	"flag"
	"fmt"
	"io"
	"strings"
)

// Config holds parsed CLI options.
type Config struct {
	Target   string
	JSON     bool
	Method   string
	Headers  map[string]string
	Timeout  int
	Insecure bool
	Version  bool
}

// headerList collects multiple --header flags.
type headerList []string

func (h *headerList) String() string { return strings.Join(*h, ", ") }

func (h *headerList) Set(value string) error {
	*h = append(*h, value)
	return nil
}

// ParseArgs parses command-line arguments into a Config.
func ParseArgs(args []string) (*Config, error) {
	// Separate positional args from flags so flags can appear anywhere.
	// Go's flag package stops parsing at the first non-flag argument.
	var flagArgs, positional []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		stripped := strings.TrimLeft(a, "-")
		if stripped != a && stripped != "" {
			flagArgs = append(flagArgs, a)
			// If this flag takes a value, consume the next arg too.
			if needsValue(stripped) && i+1 < len(args) {
				i++
				flagArgs = append(flagArgs, args[i])
			}
		} else {
			positional = append(positional, a)
		}
	}

	fs := flag.NewFlagSet("probe", flag.ContinueOnError)
	fs.SetOutput(io.Discard) // Suppress Go's default usage output; probe shows its own.

	cfg := &Config{
		Method:  "GET",
		Headers: make(map[string]string),
	}

	var headers headerList

	fs.BoolVar(&cfg.JSON, "json", false, "output as JSON")
	fs.StringVar(&cfg.Method, "method", "GET", "HTTP method")
	fs.Var(&headers, "header", "HTTP header (repeatable, format: Key: Value)")
	fs.IntVar(&cfg.Timeout, "timeout", 10, "timeout in seconds")
	fs.BoolVar(&cfg.Insecure, "insecure", false, "skip TLS certificate verification")
	fs.BoolVar(&cfg.Version, "version", false, "show version")

	if err := fs.Parse(flagArgs); err != nil {
		return nil, err
	}

	if cfg.Version {
		return cfg, nil
	}

	if cfg.Method == "" {
		return nil, fmt.Errorf("method must not be empty")
	}

	// Parse header values into map.
	for _, h := range headers {
		k, v, ok := strings.Cut(h, ":")
		if !ok {
			return nil, fmt.Errorf("invalid header format: %q (expected Key: Value)", h)
		}
		cfg.Headers[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}

	if len(positional) == 0 {
		return nil, fmt.Errorf("target URL required")
	}
	cfg.Target = positional[0]
	if strings.TrimSpace(cfg.Target) == "" {
		return nil, fmt.Errorf("target URL required")
	}

	return cfg, nil
}

// needsValue returns true for flags that require an argument.
func needsValue(name string) bool {
	switch name {
	case "method", "header", "timeout":
		return true
	}
	return false
}
