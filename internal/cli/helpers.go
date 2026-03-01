package cli

import (
	"os"
	"strings"
)

// HasJSONPrettyFlag scans raw args for --json-pretty before full parsing.
// Uses last-flag-wins semantics to handle --json-pretty --json-pretty=false.
func HasJSONPrettyFlag(args []string) bool {
	result := false
	for _, a := range args {
		if a == "--json-pretty" {
			result = true
		} else if strings.HasPrefix(a, "--json-pretty=") {
			result = ParseBoolValue(a[len("--json-pretty="):])
		}
	}
	return result
}

// HasJSONFlag scans raw args for --json or --json-pretty before full parsing.
// Uses last-flag-wins semantics to handle conflicting repeated flags.
func HasJSONFlag(args []string) bool {
	jsonFlag := false
	prettyFlag := false
	for _, a := range args {
		if a == "--json" {
			jsonFlag = true
		} else if strings.HasPrefix(a, "--json=") {
			jsonFlag = ParseBoolValue(a[len("--json="):])
		} else if a == "--json-pretty" {
			prettyFlag = true
		} else if strings.HasPrefix(a, "--json-pretty=") {
			prettyFlag = ParseBoolValue(a[len("--json-pretty="):])
		}
	}
	return jsonFlag || prettyFlag
}

// ParseBoolValue returns true for "true", "1", "t" (case-insensitive).
func ParseBoolValue(s string) bool {
	v := strings.ToLower(s)
	return v == "true" || v == "1" || v == "t"
}

// IsColorEnabled checks if color output should be used.
// The lookupEnv function should behave like os.LookupEnv.
func IsColorEnabled(lookupEnv func(string) (string, bool)) bool {
	// Respect NO_COLOR convention (https://no-color.org/).
	if _, ok := lookupEnv("NO_COLOR"); ok {
		return false
	}
	// Check if stdout is a terminal.
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
