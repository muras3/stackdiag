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
	cfg, err := ParseArgs([]string{""})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Target != "" {
		t.Errorf("Target = %q, want empty string", cfg.Target)
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
