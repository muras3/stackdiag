package cli

import (
	"strings"
	"testing"
)

func TestResolveAuthBearerEnvUnset(t *testing.T) {
	cfg := &Config{
		BearerEnv: "MY_TOKEN",
		Headers:   make(map[string]string),
	}
	lookup := func(key string) (string, bool) {
		return "", false
	}
	err := ResolveAuth(cfg, lookup)
	if err == nil {
		t.Fatal("expected error for unset env var")
	}
	if !strings.Contains(err.Error(), "is not set") {
		t.Errorf("error = %q, want 'is not set'", err.Error())
	}
}

func TestResolveAuthBearerEnvEmpty(t *testing.T) {
	cfg := &Config{
		BearerEnv: "MY_TOKEN",
		Headers:   make(map[string]string),
	}
	lookup := func(key string) (string, bool) {
		return "", true
	}
	err := ResolveAuth(cfg, lookup)
	if err == nil {
		t.Fatal("expected error for empty env var")
	}
	if !strings.Contains(err.Error(), "is empty") {
		t.Errorf("error = %q, want 'is empty'", err.Error())
	}
}

func TestResolveAuthBearerEnvWhitespace(t *testing.T) {
	cfg := &Config{
		BearerEnv: "MY_TOKEN",
		Headers:   make(map[string]string),
	}
	lookup := func(key string) (string, bool) {
		return "   ", true
	}
	err := ResolveAuth(cfg, lookup)
	if err == nil {
		t.Fatal("expected error for whitespace-only env var")
	}
	if !strings.Contains(err.Error(), "only whitespace") {
		t.Errorf("error = %q, want 'only whitespace'", err.Error())
	}
}

func TestResolveAuthBearerEnvValid(t *testing.T) {
	cfg := &Config{
		BearerEnv: "MY_TOKEN",
		Headers:   make(map[string]string),
	}
	lookup := func(key string) (string, bool) {
		return "my-secret-token", true
	}
	err := ResolveAuth(cfg, lookup)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "Bearer my-secret-token"
	if cfg.Headers["Authorization"] != want {
		t.Errorf("Authorization = %q, want %q", cfg.Headers["Authorization"], want)
	}
}

func TestResolveAuthBasicEnvValid(t *testing.T) {
	cfg := &Config{
		BasicEnv: "MY_CRED",
		Headers:  make(map[string]string),
	}
	lookup := func(key string) (string, bool) {
		return "user:pass", true
	}
	err := ResolveAuth(cfg, lookup)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := cfg.Headers["Authorization"]
	if !strings.HasPrefix(got, "Basic ") {
		t.Errorf("Authorization = %q, want Basic prefix", got)
	}
}

func TestResolveAuthBasicEnvNoColon(t *testing.T) {
	cfg := &Config{
		BasicEnv: "MY_CRED",
		Headers:  make(map[string]string),
	}
	lookup := func(key string) (string, bool) {
		return "nocolon", true
	}
	err := ResolveAuth(cfg, lookup)
	if err == nil {
		t.Fatal("expected error for basic env without colon")
	}
	if !strings.Contains(err.Error(), "user:password format") {
		t.Errorf("error = %q, want 'user:password format'", err.Error())
	}
}

func TestResolveAuthBasicEnvUnset(t *testing.T) {
	cfg := &Config{
		BasicEnv: "MY_CRED",
		Headers:  make(map[string]string),
	}
	lookup := func(key string) (string, bool) {
		return "", false
	}
	err := ResolveAuth(cfg, lookup)
	if err == nil {
		t.Fatal("expected error for unset env var")
	}
	if !strings.Contains(err.Error(), "is not set") {
		t.Errorf("error = %q, want 'is not set'", err.Error())
	}
}

func TestResolveAuthNoEnvVars(t *testing.T) {
	cfg := &Config{
		Headers: make(map[string]string),
	}
	lookup := func(key string) (string, bool) {
		return "", false
	}
	err := ResolveAuth(cfg, lookup)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Headers) != 0 {
		t.Errorf("Headers should be empty, got %v", cfg.Headers)
	}
}
