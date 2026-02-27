package core

import "testing"

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
