package cli

import (
	"flag"
	"fmt"
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
	fs := flag.NewFlagSet("probe", flag.ContinueOnError)

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

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	if cfg.Version {
		return cfg, nil
	}

	// Parse header values into map.
	for _, h := range headers {
		k, v, ok := strings.Cut(h, ":")
		if !ok {
			return nil, fmt.Errorf("invalid header format: %q (expected Key: Value)", h)
		}
		cfg.Headers[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}

	remaining := fs.Args()
	if len(remaining) == 0 {
		return nil, fmt.Errorf("target URL required")
	}
	cfg.Target = remaining[0]

	return cfg, nil
}
