package cli

import (
	"strings"
	"testing"
)

func TestParseArgsBasic(t *testing.T) {
	cfg, err := ParseArgs([]string{"https://example.com"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Target != "https://example.com" {
		t.Errorf("Target = %q, want https://example.com", cfg.Target)
	}
	if cfg.JSON {
		t.Error("JSON should default to false")
	}
	if cfg.Method != "GET" {
		t.Errorf("Method = %q, want GET", cfg.Method)
	}
	if cfg.Timeout != 10 {
		t.Errorf("Timeout = %d, want 10", cfg.Timeout)
	}
	if cfg.Insecure {
		t.Error("Insecure should default to false")
	}
}

func TestParseArgsJSON(t *testing.T) {
	cfg, err := ParseArgs([]string{"--json", "https://example.com"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.JSON {
		t.Error("JSON should be true")
	}
}

func TestParseArgsNoArgs(t *testing.T) {
	_, err := ParseArgs([]string{})
	if err == nil {
		t.Fatal("expected error for no args")
	}
}

func TestParseArgsExtraPositionalArgs(t *testing.T) {
	_, err := ParseArgs([]string{"https://example.com", "extra-arg"})
	if err == nil {
		t.Fatal("expected error for extra positional arguments")
	}
	if !strings.Contains(err.Error(), "unexpected") {
		t.Errorf("error should mention 'unexpected', got: %v", err)
	}
}

func TestParseArgsHeaders(t *testing.T) {
	cfg, err := ParseArgs([]string{
		"--header", "Authorization: Bearer token",
		"--header", "X-Custom: value",
		"https://example.com",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Headers) != 2 {
		t.Fatalf("Headers len = %d, want 2", len(cfg.Headers))
	}
	if cfg.Headers["Authorization"] != "Bearer token" {
		t.Errorf("Authorization header = %q", cfg.Headers["Authorization"])
	}
	if cfg.Headers["X-Custom"] != "value" {
		t.Errorf("X-Custom header = %q", cfg.Headers["X-Custom"])
	}
}

func TestParseArgsMethod(t *testing.T) {
	cfg, err := ParseArgs([]string{"--method", "POST", "https://example.com"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Method != "POST" {
		t.Errorf("Method = %q, want POST", cfg.Method)
	}
}

func TestParseArgsInsecure(t *testing.T) {
	cfg, err := ParseArgs([]string{"--insecure", "https://example.com"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.Insecure {
		t.Error("Insecure should be true")
	}
}

func TestParseArgsTimeout(t *testing.T) {
	cfg, err := ParseArgs([]string{"--timeout", "30", "https://example.com"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Timeout != 30 {
		t.Errorf("Timeout = %d, want 30", cfg.Timeout)
	}
}

func TestParseArgsVersion(t *testing.T) {
	cfg, err := ParseArgs([]string{"--version"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.Version {
		t.Error("Version should be true")
	}
}

func TestParseArgsFlagsAfterTarget(t *testing.T) {
	cfg, err := ParseArgs([]string{"https://example.com", "--json"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.JSON {
		t.Error("JSON should be true when flag comes after target")
	}
	if cfg.Target != "https://example.com" {
		t.Errorf("Target = %q, want https://example.com", cfg.Target)
	}
}

func TestParseArgsEmptyMethod(t *testing.T) {
	_, err := ParseArgs([]string{"--method", "", "https://example.com"})
	if err == nil {
		t.Fatal("expected error for empty method")
	}
}

func TestParseArgsUnknownFlagNoDoubleUsage(t *testing.T) {
	_, err := ParseArgs([]string{"--bogus", "https://example.com"})
	if err == nil {
		t.Fatal("expected error for unknown flag")
	}
	// The error should come from our code, not contain duplicate usage lines.
	// The key thing is that fs.Parse doesn't print its own usage to stderr.
}

func TestParseArgsFlagsMixedPosition(t *testing.T) {
	cfg, err := ParseArgs([]string{"--method", "POST", "https://example.com", "--json", "--timeout", "5"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Method != "POST" {
		t.Errorf("Method = %q, want POST", cfg.Method)
	}
	if !cfg.JSON {
		t.Error("JSON should be true")
	}
	if cfg.Timeout != 5 {
		t.Errorf("Timeout = %d, want 5", cfg.Timeout)
	}
	if cfg.Target != "https://example.com" {
		t.Errorf("Target = %q, want https://example.com", cfg.Target)
	}
}

func TestParseArgsInvalidHeader(t *testing.T) {
	_, err := ParseArgs([]string{"--header", "BadHeader", "https://example.com"})
	if err == nil {
		t.Fatal("expected error for header without colon")
	}
	if !strings.Contains(err.Error(), "invalid header format") {
		t.Errorf("error = %q, want it to contain 'invalid header format'", err.Error())
	}
}

func TestParseArgsDoubleDashSeparator(t *testing.T) {
	cfg, err := ParseArgs([]string{"--", "--json"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Target != "--json" {
		t.Errorf("Target = %q, want --json", cfg.Target)
	}
	if cfg.JSON {
		t.Error("JSON should be false when --json is after --")
	}
}

func TestParseArgsInvalidTimeout(t *testing.T) {
	_, err := ParseArgs([]string{"--timeout", "abc", "example.com"})
	if err == nil {
		t.Fatal("expected error for non-numeric timeout")
	}
}

func TestParseArgsEmptyTarget(t *testing.T) {
	_, err := ParseArgs([]string{""})
	if err == nil {
		t.Fatal("expected error for empty target")
	}
	if !strings.Contains(err.Error(), "target URL required") {
		t.Errorf("error = %q, want 'target URL required'", err.Error())
	}
}

func TestParseArgsRedactDefault(t *testing.T) {
	cfg, err := ParseArgs([]string{"https://example.com"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.Redact {
		t.Error("Redact should default to true")
	}
}

func TestParseArgsNoRedact(t *testing.T) {
	cfg, err := ParseArgs([]string{"--no-redact", "https://example.com"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Redact {
		t.Error("Redact should be false with --no-redact")
	}
}

func TestParseArgsExplicitRedact(t *testing.T) {
	cfg, err := ParseArgs([]string{"--redact", "https://example.com"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.Redact {
		t.Error("Redact should be true with explicit --redact")
	}
}

func TestParseArgsNoRedactAfterTarget(t *testing.T) {
	cfg, err := ParseArgs([]string{"https://example.com", "--no-redact"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Redact {
		t.Error("Redact should be false with --no-redact after target")
	}
}

func TestParseArgsRedactAndNoRedact(t *testing.T) {
	// When both --redact and --no-redact are present, --no-redact wins.
	cfg, err := ParseArgs([]string{"--redact", "--no-redact", "https://example.com"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Redact {
		t.Error("Redact should be false when --no-redact is present (overrides --redact)")
	}
}

func TestParseArgsNoRedactThenRedact(t *testing.T) {
	// Reverse order: --no-redact then --redact. --no-redact still wins.
	cfg, err := ParseArgs([]string{"--no-redact", "--redact", "https://example.com"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Redact {
		t.Error("Redact should be false when --no-redact is present regardless of order")
	}
}

func TestHelpTextContainsSections(t *testing.T) {
	text := HelpText()
	sections := []string{"USAGE:", "TARGETS:", "OPTIONS:", "EXIT CODES:", "EXAMPLES:"}
	for _, sec := range sections {
		if !strings.Contains(text, sec) {
			t.Errorf("HelpText() missing section %q", sec)
		}
	}
}

func TestHelpTextContainsOptions(t *testing.T) {
	text := HelpText()
	options := []string{"--json", "--method", "--header", "--timeout", "--insecure", "--no-redact", "--version"}
	for _, opt := range options {
		if !strings.Contains(text, opt) {
			t.Errorf("HelpText() missing option %q", opt)
		}
	}
}

func TestHelpTextContainsExitCodes(t *testing.T) {
	text := HelpText()
	codes := []string{"0 ", "1 ", "2 ", "10 ", "15 ", "20 ", "30 ", "40 "}
	for _, code := range codes {
		if !strings.Contains(text, code) {
			t.Errorf("HelpText() missing exit code %q", code)
		}
	}
}

func TestHelpTextContainsExitCode15Reachability(t *testing.T) {
	text := HelpText()
	if !strings.Contains(text, "15") {
		t.Error("HelpText() does not contain exit code 15")
	}
	if !strings.Contains(text, "Reachability failure") {
		t.Error("HelpText() does not mention 'Reachability failure' for exit code 15")
	}
}

func TestParseArgsHelpFlag(t *testing.T) {
	cfg, err := ParseArgs([]string{"--help"})
	if err != nil {
		t.Fatalf("--help should not return error, got: %v", err)
	}
	if !cfg.Help {
		t.Error("Help should be true with --help")
	}
}

func TestParseArgsErrorMessages(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantMsg string
	}{
		{
			name:    "no args",
			args:    []string{},
			wantMsg: "target URL required",
		},
		{
			name:    "invalid header",
			args:    []string{"--header", "NoColon", "example.com"},
			wantMsg: "invalid header format",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseArgs(tt.args)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("error = %q, want it to contain %q", err.Error(), tt.wantMsg)
			}
		})
	}
}

func TestParseArgsRejectsMethodWithNonLetters(t *testing.T) {
	_, err := ParseArgs([]string{"--method", "POST1", "https://example.com"})
	if err == nil {
		t.Fatal("expected error for method containing non-letter")
	}
	if !strings.Contains(err.Error(), "invalid HTTP method") {
		t.Fatalf("error = %q, want invalid HTTP method", err.Error())
	}
}

func TestParseArgsRejectsHeaderControlCharacters(t *testing.T) {
	tests := []struct {
		name string
		arg  string
	}{
		{name: "value contains newline", arg: "X-Test: hello\nworld"},
		{name: "name contains carriage return", arg: "X-Test\r: value"},
		{name: "value contains NUL", arg: "X-Test: a\x00b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseArgs([]string{"--header", tt.arg, "https://example.com"})
			if err == nil {
				t.Fatal("expected header validation error")
			}
		})
	}
}

// --- v0.1 expansion: auth flags ---

func TestParseArgsBearerEnv(t *testing.T) {
	cfg, err := ParseArgs([]string{"--bearer-env", "MY_TOKEN", "https://example.com"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.BearerEnv != "MY_TOKEN" {
		t.Errorf("BearerEnv = %q, want MY_TOKEN", cfg.BearerEnv)
	}
}

func TestParseArgsBasicEnv(t *testing.T) {
	cfg, err := ParseArgs([]string{"--basic-env", "MY_CRED", "https://example.com"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.BasicEnv != "MY_CRED" {
		t.Errorf("BasicEnv = %q, want MY_CRED", cfg.BasicEnv)
	}
}

func TestParseArgsBearerAndBasicConflict(t *testing.T) {
	_, err := ParseArgs([]string{
		"--bearer-env", "TOK",
		"--basic-env", "CRED",
		"https://example.com",
	})
	if err == nil {
		t.Fatal("expected error for --bearer-env + --basic-env conflict")
	}
	if !strings.Contains(err.Error(), "cannot be used together") {
		t.Errorf("error = %q, want conflict message", err.Error())
	}
}

func TestParseArgsBearerEnvConflictsWithAuthHeader(t *testing.T) {
	_, err := ParseArgs([]string{
		"--bearer-env", "TOK",
		"--header", "Authorization: Bearer xxx",
		"https://example.com",
	})
	if err == nil {
		t.Fatal("expected error for --bearer-env + --header Authorization conflict")
	}
	if !strings.Contains(err.Error(), "conflicts") {
		t.Errorf("error = %q, want conflict message", err.Error())
	}
}

func TestParseArgsBasicEnvConflictsWithAuthHeader(t *testing.T) {
	_, err := ParseArgs([]string{
		"--basic-env", "CRED",
		"--header", "authorization: Basic xxx",
		"https://example.com",
	})
	if err == nil {
		t.Fatal("expected error: --basic-env conflicts with Authorization header")
	}
	if !strings.Contains(err.Error(), "conflicts") {
		t.Errorf("error = %q, want conflict message", err.Error())
	}
}

// --- v0.1 expansion: tls-scan flag ---

func TestParseArgsTLSScan(t *testing.T) {
	cfg, err := ParseArgs([]string{"--tls-scan", "https://example.com"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.TLSScan {
		t.Error("TLSScan should be true")
	}
}

func TestParseArgsTLSScanDefault(t *testing.T) {
	cfg, err := ParseArgs([]string{"https://example.com"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.TLSScan {
		t.Error("TLSScan should default to false")
	}
}

// --- v0.1 expansion: count flag ---

func TestParseArgsCount(t *testing.T) {
	cfg, err := ParseArgs([]string{"--count", "5", "https://example.com"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Count != 5 {
		t.Errorf("Count = %d, want 5", cfg.Count)
	}
}

func TestParseArgsCountDefault(t *testing.T) {
	cfg, err := ParseArgs([]string{"https://example.com"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Count != 0 {
		t.Errorf("Count = %d, want 0 (unset)", cfg.Count)
	}
}

func TestParseArgsCountZero(t *testing.T) {
	_, err := ParseArgs([]string{"--count", "0", "https://example.com"})
	if err == nil {
		t.Fatal("expected error for --count 0")
	}
	if !strings.Contains(err.Error(), "positive integer") {
		t.Errorf("error = %q, want positive integer message", err.Error())
	}
}

func TestParseArgsCountNegative(t *testing.T) {
	_, err := ParseArgs([]string{"--count", "-1", "https://example.com"})
	if err == nil {
		t.Fatal("expected error for --count -1")
	}
}

func TestParseArgsCountNonNumeric(t *testing.T) {
	_, err := ParseArgs([]string{"--count", "abc", "https://example.com"})
	if err == nil {
		t.Fatal("expected error for --count abc")
	}
}

// --- v0.1 expansion: help text sections ---

func TestHelpTextContainsAuthSection(t *testing.T) {
	text := HelpText()
	if !strings.Contains(text, "AUTHENTICATION:") {
		t.Error("HelpText() missing AUTHENTICATION section")
	}
	if !strings.Contains(text, "--bearer-env") {
		t.Error("HelpText() missing --bearer-env option")
	}
	if !strings.Contains(text, "--basic-env") {
		t.Error("HelpText() missing --basic-env option")
	}
}

func TestHelpTextContainsDiagnosticsSection(t *testing.T) {
	text := HelpText()
	if !strings.Contains(text, "DIAGNOSTICS:") {
		t.Error("HelpText() missing DIAGNOSTICS section")
	}
	if !strings.Contains(text, "--tls-scan") {
		t.Error("HelpText() missing --tls-scan option")
	}
	if !strings.Contains(text, "--count") {
		t.Error("HelpText() missing --count option")
	}
}

func TestParseArgsJSONPretty(t *testing.T) {
	cfg, err := ParseArgs([]string{"--json-pretty", "https://example.com"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.JSON {
		t.Error("JSON should be true when --json-pretty is used")
	}
	if !cfg.JSONPretty {
		t.Error("JSONPretty should be true")
	}
}

func TestParseArgsJSONPrettyDefault(t *testing.T) {
	cfg, err := ParseArgs([]string{"--json", "https://example.com"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.JSON {
		t.Error("JSON should be true")
	}
	if cfg.JSONPretty {
		t.Error("JSONPretty should default to false")
	}
}

// --- v0.1 expansion: dns-server flag ---

func TestParseArgsDNSServer(t *testing.T) {
	cfg, err := ParseArgs([]string{"--dns-server", "8.8.8.8:53", "https://example.com"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DNSServer != "8.8.8.8:53" {
		t.Errorf("DNSServer = %q, want 8.8.8.8:53", cfg.DNSServer)
	}
}

func TestParseArgsDNSServerDefault(t *testing.T) {
	cfg, err := ParseArgs([]string{"https://example.com"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DNSServer != "" {
		t.Errorf("DNSServer = %q, want empty", cfg.DNSServer)
	}
}

func TestParseArgsDNSServerAfterTarget(t *testing.T) {
	cfg, err := ParseArgs([]string{"https://example.com", "--dns-server", "1.1.1.1:53"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DNSServer != "1.1.1.1:53" {
		t.Errorf("DNSServer = %q, want 1.1.1.1:53", cfg.DNSServer)
	}
}

func TestHelpTextContainsDNSServer(t *testing.T) {
	text := HelpText()
	if !strings.Contains(text, "--dns-server") {
		t.Error("HelpText() missing --dns-server option")
	}
}

func TestParseArgsTimeoutRange(t *testing.T) {
	tests := []struct {
		name    string
		timeout string
		wantErr bool
	}{
		{name: "minimum", timeout: "1", wantErr: false},
		{name: "maximum", timeout: "300", wantErr: false},
		{name: "zero", timeout: "0", wantErr: true},
		{name: "negative", timeout: "-1", wantErr: true},
		{name: "too large", timeout: "301", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseArgs([]string{"--timeout", tt.timeout, "https://example.com"})
			if tt.wantErr && err == nil {
				t.Fatalf("expected timeout error for %s", tt.timeout)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected timeout error for %s: %v", tt.timeout, err)
			}
		})
	}
}
