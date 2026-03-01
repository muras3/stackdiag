package cli

import (
	"testing"
)

func TestHasJSONFlag(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{"no flags", []string{"example.com"}, false},
		{"--json", []string{"--json", "example.com"}, true},
		{"--json=true", []string{"--json=true", "example.com"}, true},
		{"--json=false", []string{"--json=false", "example.com"}, false},
		{"--json-pretty", []string{"--json-pretty", "example.com"}, true},
		{"--json-pretty=false", []string{"--json-pretty=false", "example.com"}, false},
		{"last wins: --json then --json=false", []string{"--json", "--json=false", "example.com"}, false},
		{"last wins: --json=false then --json", []string{"--json=false", "--json", "example.com"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HasJSONFlag(tt.args)
			if got != tt.want {
				t.Errorf("HasJSONFlag(%v) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}

func TestHasJSONPrettyFlag(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{"no flags", []string{"example.com"}, false},
		{"--json-pretty", []string{"--json-pretty", "example.com"}, true},
		{"--json-pretty=true", []string{"--json-pretty=true", "example.com"}, true},
		{"--json-pretty=false", []string{"--json-pretty=false", "example.com"}, false},
		{"--json only", []string{"--json", "example.com"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HasJSONPrettyFlag(tt.args)
			if got != tt.want {
				t.Errorf("HasJSONPrettyFlag(%v) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}

func TestParseBoolValue(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"true", true},
		{"TRUE", true},
		{"True", true},
		{"1", true},
		{"t", true},
		{"T", true},
		{"false", false},
		{"0", false},
		{"no", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ParseBoolValue(tt.input)
			if got != tt.want {
				t.Errorf("ParseBoolValue(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsColorEnabled(t *testing.T) {
	// Test NO_COLOR set
	lookup := func(key string) (string, bool) {
		if key == "NO_COLOR" {
			return "", true
		}
		return "", false
	}
	got := IsColorEnabled(lookup)
	if got {
		t.Error("IsColorEnabled should return false when NO_COLOR is set")
	}

	// Test NO_COLOR not set (falls through to non-terminal check)
	lookup = func(key string) (string, bool) {
		return "", false
	}
	// In test, stdout is not a terminal, so should be false
	got = IsColorEnabled(lookup)
	if got {
		t.Error("IsColorEnabled should return false when stdout is not a terminal")
	}
}
