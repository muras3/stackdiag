package cli

import "testing"

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
