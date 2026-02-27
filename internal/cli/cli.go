package cli

import (
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"
)

const (
	minTimeoutSeconds = 1
	maxTimeoutSeconds = 300
)

// Config holds parsed CLI options.
type Config struct {
	Target    string
	JSON      bool
	Method    string
	Headers   map[string]string
	Timeout   int
	Insecure  bool
	Redact    bool
	Version   bool
	Help      bool
	BearerEnv string // --bearer-env ENV_VAR
	BasicEnv  string // --basic-env ENV_VAR
	TLSScan   bool   // --tls-scan
	Count     int    // --count N
}

// HelpText returns the full help message for stackdiag.
func HelpText() string {
	return `stackdiag - structured diagnostics for AI agents and humans

USAGE:
  stackdiag <url> [options]

TARGETS:
  https://host/path     Full HTTPS check (DNS → TCP → TLS → HTTP)
  http://host/path      HTTP check (DNS → TCP → HTTP, no TLS)
  tcp://host:port       TCP connectivity only (DNS → TCP)
  host                  Bare hostname defaults to https://

OPTIONS:
  --json                Output as JSON (default: table)
  --method METHOD       HTTP method (default: GET)
  --header KEY:VALUE    HTTP header (repeatable)
  --timeout N           Timeout in seconds (1-300, default: 10)
  --insecure            Skip TLS certificate verification
  --no-redact           Show sensitive header values (default: redacted)
  --version             Show version

AUTHENTICATION:
  --bearer-env VAR      Read Bearer token from environment variable
  --basic-env VAR       Read Basic auth (user:pass) from environment variable

DIAGNOSTICS:
  --tls-scan            Probe TLS 1.0/1.1/1.2/1.3 version support
  --count N             Run N attempts and show statistics

EXIT CODES:
  0   All layers passed
  1   Tool error
  2   Warning (e.g. certificate expiring soon)
  10  DNS failure
  20  TCP failure
  30  TLS failure
  40  HTTP failure

EXAMPLES:
  stackdiag https://example.com
  stackdiag --json https://api.example.com/health
  stackdiag tcp://db.internal:5432
  stackdiag --tls-scan https://example.com
  stackdiag --count 5 https://example.com
  stackdiag --bearer-env API_TOKEN https://api.example.com/health`
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

	fs := flag.NewFlagSet("stackdiag", flag.ContinueOnError)
	fs.SetOutput(io.Discard) // Suppress Go's default usage output; stackdiag shows its own.

	cfg := &Config{
		Method:  "GET",
		Headers: make(map[string]string),
	}

	var headers headerList
	var noRedact bool
	var redactFlag bool // explicit --redact (sugar, no-op since default is true)
	var countStr string

	fs.BoolVar(&cfg.JSON, "json", false, "output as JSON")
	fs.StringVar(&cfg.Method, "method", "GET", "HTTP method")
	fs.Var(&headers, "header", "HTTP header (repeatable, format: Key: Value)")
	fs.IntVar(&cfg.Timeout, "timeout", 10, "timeout in seconds")
	fs.BoolVar(&cfg.Insecure, "insecure", false, "skip TLS certificate verification")
	fs.BoolVar(&noRedact, "no-redact", false, "disable redaction of sensitive values")
	fs.BoolVar(&redactFlag, "redact", false, "redact sensitive values (default)")
	fs.BoolVar(&cfg.Version, "version", false, "show version")
	fs.StringVar(&cfg.BearerEnv, "bearer-env", "", "environment variable for Bearer token")
	fs.StringVar(&cfg.BasicEnv, "basic-env", "", "environment variable for Basic auth")
	fs.BoolVar(&cfg.TLSScan, "tls-scan", false, "probe TLS version support")
	fs.StringVar(&countStr, "count", "", "number of attempts")

	if err := fs.Parse(flagArgs); err != nil {
		if err.Error() == "flag: help requested" {
			return &Config{Help: true}, nil
		}
		return nil, err
	}

	// --no-redact is the canonical toggle; --redact is just sugar.
	// Default is redact=true. --no-redact overrides regardless of order.
	cfg.Redact = !noRedact

	if cfg.Version {
		return cfg, nil
	}

	cfg.Method = strings.ToUpper(strings.TrimSpace(cfg.Method))
	if cfg.Method == "" {
		return nil, fmt.Errorf("method must not be empty")
	}
	if !isValidMethod(cfg.Method) {
		return nil, fmt.Errorf("invalid HTTP method %q: use ASCII letters only", cfg.Method)
	}
	if cfg.Timeout < minTimeoutSeconds || cfg.Timeout > maxTimeoutSeconds {
		return nil, fmt.Errorf("timeout must be between %d and %d seconds", minTimeoutSeconds, maxTimeoutSeconds)
	}

	// Parse --count value.
	if countStr != "" {
		n, err := strconv.Atoi(countStr)
		if err != nil || n < 1 {
			return nil, fmt.Errorf("--count must be a positive integer, got %q", countStr)
		}
		cfg.Count = n
	}

	// Parse header values into map.
	for _, h := range headers {
		k, v, ok := strings.Cut(h, ":")
		if !ok {
			return nil, fmt.Errorf("invalid header format: %q (expected Key: Value)", h)
		}
		if containsForbiddenControl(k) {
			return nil, fmt.Errorf("invalid header name %q: contains forbidden control characters", strings.TrimSpace(k))
		}
		if containsForbiddenControl(v) {
			return nil, fmt.Errorf("invalid value for header %q: contains forbidden control characters", strings.TrimSpace(k))
		}
		key := strings.TrimSpace(k)
		value := strings.TrimSpace(v)
		if err := validateHeaderKV(key, value); err != nil {
			return nil, err
		}
		cfg.Headers[key] = value
	}

	// Auth conflict detection.
	if cfg.BearerEnv != "" && cfg.BasicEnv != "" {
		return nil, fmt.Errorf("--bearer-env and --basic-env cannot be used together")
	}
	if cfg.BearerEnv != "" || cfg.BasicEnv != "" {
		for k := range cfg.Headers {
			if strings.EqualFold(k, "authorization") {
				if cfg.BearerEnv != "" {
					return nil, fmt.Errorf("--bearer-env conflicts with --header Authorization")
				}
				return nil, fmt.Errorf("--basic-env conflicts with --header Authorization")
			}
		}
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
	case "method", "header", "timeout", "bearer-env", "basic-env", "count":
		return true
	}
	return false
}

func isValidMethod(method string) bool {
	for i := 0; i < len(method); i++ {
		c := method[i]
		if c < 'A' || c > 'Z' {
			return false
		}
	}
	return true
}

func validateHeaderKV(key, value string) error {
	if key == "" {
		return fmt.Errorf("header name must not be empty")
	}
	if !isValidHeaderName(key) {
		return fmt.Errorf("invalid header name %q", key)
	}
	if containsForbiddenControl(key) {
		return fmt.Errorf("invalid header name %q: contains forbidden control characters", key)
	}
	if containsForbiddenControl(value) {
		return fmt.Errorf("invalid value for header %q: contains forbidden control characters", key)
	}
	return nil
}

func containsForbiddenControl(s string) bool {
	return strings.ContainsAny(s, "\r\n\x00")
}

func isValidHeaderName(name string) bool {
	for i := 0; i < len(name); i++ {
		c := name[i]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' {
			continue
		}
		switch c {
		case '!', '#', '$', '%', '&', '\'', '*', '+', '-', '.', '^', '_', '`', '|', '~':
			continue
		default:
			return false
		}
	}
	return true
}
