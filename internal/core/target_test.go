package core

import (
	"strings"
	"testing"
)

func TestParseTarget(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    Target
		wantErr bool
	}{
		{
			name:  "https URL with path",
			input: "https://api.example.com/health",
			want: Target{
				Original: "https://api.example.com/health",
				Scheme:   "https",
				Host:     "api.example.com",
				Port:     443,
				Path:     "/health",
			},
		},
		{
			name:  "http URL",
			input: "http://example.com",
			want: Target{
				Original: "http://example.com",
				Scheme:   "http",
				Host:     "example.com",
				Port:     80,
				Path:     "/",
			},
		},
		{
			name:  "tcp scheme with port",
			input: "tcp://example.com:8080",
			want: Target{
				Original: "tcp://example.com:8080",
				Scheme:   "tcp",
				Host:     "example.com",
				Port:     8080,
				Path:     "",
			},
		},
		{
			name:  "https with custom port",
			input: "https://example.com:8443/api",
			want: Target{
				Original: "https://example.com:8443/api",
				Scheme:   "https",
				Host:     "example.com",
				Port:     8443,
				Path:     "/api",
			},
		},
		{
			name:  "https IPv6 URL with port and path",
			input: "https://[::1]:443/path",
			want: Target{
				Original: "https://[::1]:443/path",
				Scheme:   "https",
				Host:     "::1",
				Port:     443,
				Path:     "/path",
			},
		},
		{
			name:  "tcp IPv6 URL with port",
			input: "tcp://[::1]:8080",
			want: Target{
				Original: "tcp://[::1]:8080",
				Scheme:   "tcp",
				Host:     "::1",
				Port:     8080,
				Path:     "",
			},
		},
		{
			name:  "https URL preserves query string",
			input: "https://api.example.com/health?ready=1",
			want: Target{
				Original: "https://api.example.com/health?ready=1",
				Scheme:   "https",
				Host:     "api.example.com",
				Port:     443,
				Path:     "/health?ready=1",
			},
		},
		{
			name:  "bare hostname defaults to https",
			input: "example.com",
			want: Target{
				Original: "example.com",
				Scheme:   "https",
				Host:     "example.com",
				Port:     443,
				Path:     "/",
			},
		},
		{
			name:    "empty input",
			input:   "",
			wantErr: true,
		},
		{
			name:    "invalid scheme",
			input:   "ftp://example.com",
			wantErr: true,
		},
		{
			name:    "tcp without port",
			input:   "tcp://example.com",
			wantErr: true,
		},
		{
			name:    "port out of range",
			input:   "https://example.com:70000",
			wantErr: true,
		},
		{
			name:    "tcp with path rejected",
			input:   "tcp://example.com:5432/db",
			wantErr: true,
		},
		{
			name:    "tcp with query rejected",
			input:   "tcp://example.com:5432?x=1",
			wantErr: true,
		},
		{
			name:  "whitespace trimmed",
			input: "  https://example.com  ",
			want: Target{
				Original: "https://example.com",
				Scheme:   "https",
				Host:     "example.com",
				Port:     443,
				Path:     "/",
			},
		},
		{
			name:    "whitespace only",
			input:   "   ",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseTarget(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Scheme != tt.want.Scheme {
				t.Errorf("Scheme = %q, want %q", got.Scheme, tt.want.Scheme)
			}
			if got.Host != tt.want.Host {
				t.Errorf("Host = %q, want %q", got.Host, tt.want.Host)
			}
			if got.Port != tt.want.Port {
				t.Errorf("Port = %d, want %d", got.Port, tt.want.Port)
			}
			if got.Path != tt.want.Path {
				t.Errorf("Path = %q, want %q", got.Path, tt.want.Path)
			}
			if got.Original != tt.want.Original {
				t.Errorf("Original = %q, want %q", got.Original, tt.want.Original)
			}
		})
	}
}

func TestTargetNeedsTLS(t *testing.T) {
	tgt := Target{Scheme: "https"}
	if !tgt.NeedsTLS() {
		t.Error("https should need TLS")
	}
	tgt.Scheme = "http"
	if tgt.NeedsTLS() {
		t.Error("http should not need TLS")
	}
	tgt.Scheme = "tcp"
	if tgt.NeedsTLS() {
		t.Error("tcp should not need TLS")
	}
}

func TestTargetNeedsHTTP(t *testing.T) {
	tgt := Target{Scheme: "https"}
	if !tgt.NeedsHTTP() {
		t.Error("https should need HTTP")
	}
	tgt.Scheme = "http"
	if !tgt.NeedsHTTP() {
		t.Error("http should need HTTP")
	}
	tgt.Scheme = "tcp"
	if tgt.NeedsHTTP() {
		t.Error("tcp should not need HTTP")
	}
}

func TestTargetHostPort(t *testing.T) {
	tgt := Target{Host: "example.com", Port: 443}
	if got := tgt.HostPort(); got != "example.com:443" {
		t.Errorf("HostPort() = %q, want example.com:443", got)
	}

	tgt = Target{Host: "::1", Port: 443}
	if got := tgt.HostPort(); got != "[::1]:443" {
		t.Errorf("HostPort() = %q, want [::1]:443", got)
	}
}

func TestParseTargetPortBoundaries(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		port    int
	}{
		{name: "port 0", input: "https://example.com:0", wantErr: true},
		{name: "port 1", input: "https://example.com:1", wantErr: false, port: 1},
		{name: "port 65535", input: "https://example.com:65535", wantErr: false, port: 65535},
		{name: "port 65536", input: "https://example.com:65536", wantErr: true},
		{name: "port -1", input: "https://example.com:-1", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseTarget(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Port != tt.port {
				t.Errorf("Port = %d, want %d", got.Port, tt.port)
			}
		})
	}
}

func TestParseTargetURLWithFragment(t *testing.T) {
	// Fragments are stripped by url.Parse; should not cause error for http/https.
	got, err := ParseTarget("https://example.com/path#frag")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Host != "example.com" {
		t.Errorf("Host = %q, want example.com", got.Host)
	}
	// Fragment is not included in Path (url.Parse strips it).
	if got.Path != "/path" {
		t.Errorf("Path = %q, want /path", got.Path)
	}
}

func TestParseTargetBareHostnameWithPort(t *testing.T) {
	got, err := ParseTarget("example.com:8443")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Host != "example.com" {
		t.Errorf("Host = %q, want example.com", got.Host)
	}
	if got.Port != 8443 {
		t.Errorf("Port = %d, want 8443", got.Port)
	}
	if got.Scheme != "https" {
		t.Errorf("Scheme = %q, want https", got.Scheme)
	}
}

func TestParseTargetErrorMessages(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantMsg string
	}{
		{name: "empty", input: "", wantMsg: "empty target"},
		{name: "bad scheme", input: "ftp://example.com", wantMsg: "unsupported scheme"},
		{name: "tcp no port", input: "tcp://example.com", wantMsg: "tcp scheme requires an explicit port"},
		{name: "port out of range", input: "https://example.com:99999", wantMsg: "port out of range"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseTarget(tt.input)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("error = %q, want it to contain %q", err.Error(), tt.wantMsg)
			}
		})
	}
}

func TestParseTargetTCPWithFragment(t *testing.T) {
	_, err := ParseTarget("tcp://example.com:5432#frag")
	if err == nil {
		t.Fatal("expected error for tcp with fragment")
	}
}
