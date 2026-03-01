package e2e

import (
	"encoding/json"
	"strings"
	"testing"
)

// parseJSONError unmarshals stdout JSON and returns the error object.
// Fails the test if stdout is not valid JSON or missing the error object.
func parseJSONError(t *testing.T, stdout string) map[string]any {
	t.Helper()
	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("expected valid JSON on stdout: %v\n%s", err, stdout)
	}
	errObj, ok := result["error"].(map[string]any)
	if !ok {
		t.Fatalf("missing error object in JSON output: %s", stdout)
	}
	return errObj
}

// verifyJSONErrorStructure checks required top-level fields for tool errors.
func verifyJSONErrorStructure(t *testing.T, stdout string) {
	t.Helper()
	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("expected valid JSON on stdout: %v\n%s", err, stdout)
	}
	for _, key := range []string{"schema_version", "error", "exit_code"} {
		if _, ok := result[key]; !ok {
			t.Errorf("missing required field %q in tool error JSON", key)
		}
	}
	if v, ok := result["schema_version"].(string); !ok || v != "v0.1" {
		t.Errorf("schema_version = %v, want v0.1", result["schema_version"])
	}
	if code, ok := result["exit_code"].(float64); !ok || int(code) != 1 {
		t.Errorf("exit_code = %v, want 1", result["exit_code"])
	}
}

// --- Extra positional arguments ---

func TestExtraPositionalArgsTableError(t *testing.T) {
	_, stderr, exitCode := runStackdiag(t, "https://example.com", "extra-arg")
	if exitCode != 1 {
		t.Errorf("exit code = %d, want 1", exitCode)
	}
	if !strings.Contains(stderr, "unexpected") {
		t.Errorf("stderr should mention 'unexpected', got: %s", stderr)
	}
}

func TestExtraPositionalArgsJSONError(t *testing.T) {
	stdout, _, exitCode := runStackdiag(t, "--json", "https://example.com", "extra-arg")
	if exitCode != 1 {
		t.Errorf("exit code = %d, want 1", exitCode)
	}
	verifyJSONErrorStructure(t, stdout)
	errObj := parseJSONError(t, stdout)
	if errObj["code"] != "INVALID_ARGS" {
		t.Errorf("error code = %v, want INVALID_ARGS", errObj["code"])
	}
	msg, _ := errObj["message"].(string)
	if !strings.Contains(msg, "unexpected") {
		t.Errorf("error message should mention 'unexpected', got: %s", msg)
	}
}

// --- Invalid target formats (table-driven) ---

func TestInvalidTargetVariants(t *testing.T) {
	cases := []struct {
		name   string
		target string
	}{
		{"tcp_no_port", "tcp://host"},
		{"missing_host", "://"},
		{"unsupported_scheme", "ftp://host"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, exitCode := runStackdiag(t, tc.target)
			if exitCode != 1 {
				t.Errorf("exit code = %d, want 1 for target %q", exitCode, tc.target)
			}
		})
	}
}

func TestInvalidTargetJSONError(t *testing.T) {
	stdout, _, exitCode := runStackdiag(t, "--json", "ftp://host")
	if exitCode != 1 {
		t.Errorf("exit code = %d, want 1", exitCode)
	}
	verifyJSONErrorStructure(t, stdout)
	errObj := parseJSONError(t, stdout)
	if errObj["code"] != "INVALID_TARGET" {
		t.Errorf("error code = %v, want INVALID_TARGET", errObj["code"])
	}
}

// --- Timeout range validation (table-driven) ---

func TestTimeoutRange(t *testing.T) {
	cases := []struct {
		name    string
		timeout string
	}{
		{"too_low", "0"},
		{"too_high", "301"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, stderr, exitCode := runStackdiag(t, "--timeout", tc.timeout, "https://example.com")
			if exitCode != 1 {
				t.Errorf("exit code = %d, want 1", exitCode)
			}
			if !strings.Contains(stderr, "between 1 and 300") {
				t.Errorf("stderr should mention 'between 1 and 300', got: %s", stderr)
			}
		})
	}
}

func TestTimeoutRangeJSONError(t *testing.T) {
	stdout, _, exitCode := runStackdiag(t, "--json", "--timeout", "0", "https://example.com")
	if exitCode != 1 {
		t.Errorf("exit code = %d, want 1", exitCode)
	}
	verifyJSONErrorStructure(t, stdout)
	errObj := parseJSONError(t, stdout)
	if errObj["code"] != "INVALID_ARGS" {
		t.Errorf("error code = %v, want INVALID_ARGS", errObj["code"])
	}
}

// --- HTTP method validation ---

func TestInvalidMethodNonASCII(t *testing.T) {
	_, _, exitCode := runStackdiag(t, "--method", "GÉT", "https://example.com")
	if exitCode != 1 {
		t.Errorf("exit code = %d, want 1 for non-ASCII method", exitCode)
	}
}

func TestEmptyMethod(t *testing.T) {
	_, _, exitCode := runStackdiag(t, "--method", "", "https://example.com")
	if exitCode != 1 {
		t.Errorf("exit code = %d, want 1 for empty method", exitCode)
	}
}

// --- Header validation ---

func TestInvalidHeaderMissingColon(t *testing.T) {
	_, _, exitCode := runStackdiag(t, "--header", "NoColon", "https://example.com")
	if exitCode != 1 {
		t.Errorf("exit code = %d, want 1 for header without colon", exitCode)
	}
}

func TestHeaderControlChars(t *testing.T) {
	_, _, exitCode := runStackdiag(t, "--header", "X-Bad: val\r\n", "https://example.com")
	if exitCode != 1 {
		t.Errorf("exit code = %d, want 1 for header with control chars", exitCode)
	}
}

// --- Auth conflict validation ---

func TestBasicEnvHeaderAuthConflict(t *testing.T) {
	t.Setenv("TEST_BASIC_CONFLICT_VAR", "user:pass")

	stdout, _, exitCode := runStackdiag(t,
		"--json",
		"--basic-env", "TEST_BASIC_CONFLICT_VAR",
		"--header", "Authorization: Bearer x",
		"https://example.com")
	if exitCode != 1 {
		t.Errorf("exit code = %d, want 1", exitCode)
	}
	errObj := parseJSONError(t, stdout)
	msg, _ := errObj["message"].(string)
	if !strings.Contains(msg, "conflicts") {
		t.Errorf("error message should mention 'conflicts', got: %s", msg)
	}
}

// --- Auth env whitespace validation (table-driven) ---

func TestAuthEnvWhitespaceOnly(t *testing.T) {
	cases := []struct {
		name   string
		flag   string
		envVar string
	}{
		{"bearer", "--bearer-env", "TEST_BEARER_WS"},
		{"basic", "--basic-env", "TEST_BASIC_WS"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(tc.envVar, " ")
			stdout, _, exitCode := runStackdiag(t,
				"--json", tc.flag, tc.envVar, "https://example.com")
			if exitCode != 1 {
				t.Errorf("exit code = %d, want 1", exitCode)
			}
			errObj := parseJSONError(t, stdout)
			msg, _ := errObj["message"].(string)
			if !strings.Contains(msg, "whitespace") {
				t.Errorf("error message should mention 'whitespace', got: %s", msg)
			}
		})
	}
}

// --- --count flag validation (table-driven) ---

func TestCountInvalidValues(t *testing.T) {
	cases := []struct {
		name  string
		value string
	}{
		{"zero", "0"},
		{"negative", "-1"},
		{"non_numeric", "abc"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, exitCode := runStackdiag(t, "--count", tc.value, "https://example.com")
			if exitCode != 1 {
				t.Errorf("exit code = %d, want 1 for --count %s", exitCode, tc.value)
			}
		})
	}
}

// --- Unknown flag ---

func TestUnknownFlagError(t *testing.T) {
	_, _, exitCode := runStackdiag(t, "--unknown-flag", "https://example.com")
	if exitCode != 1 {
		t.Errorf("exit code = %d, want 1 for unknown flag", exitCode)
	}
}

// --- Output format for errors ---

func TestTableErrorOutputFormat(t *testing.T) {
	_, stderr, exitCode := runStackdiag(t, "--timeout", "0", "https://example.com")
	if exitCode != 1 {
		t.Errorf("exit code = %d, want 1", exitCode)
	}
	if !strings.Contains(stderr, "Error:") {
		t.Errorf("stderr should contain 'Error:' prefix, got: %s", stderr)
	}
	if !strings.Contains(stderr, "USAGE:") {
		t.Errorf("stderr should contain USAGE block, got: %s", stderr)
	}
}

func TestJSONPrettyErrorIsIndented(t *testing.T) {
	stdout, _, exitCode := runStackdiag(t, "--json-pretty", "--timeout", "0", "https://example.com")
	if exitCode != 1 {
		t.Errorf("exit code = %d, want 1", exitCode)
	}
	if len(stdout) == 0 {
		t.Fatal("expected JSON output on stdout")
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("expected valid JSON: %v\n%s", err, stdout)
	}
	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	if len(lines) <= 1 {
		t.Error("--json-pretty error output should be multi-line indented")
	}
	if !strings.Contains(stdout, "  ") {
		t.Error("--json-pretty error output should be indented with spaces")
	}
}
