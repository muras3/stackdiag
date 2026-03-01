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

// --- Invalid target formats ---

func TestInvalidTargetTCPNoPort(t *testing.T) {
	_, _, exitCode := runStackdiag(t, "tcp://host")
	if exitCode != 1 {
		t.Errorf("exit code = %d, want 1 for tcp without port", exitCode)
	}
}

func TestInvalidTargetMissingHost(t *testing.T) {
	_, _, exitCode := runStackdiag(t, "://")
	if exitCode != 1 {
		t.Errorf("exit code = %d, want 1 for missing host", exitCode)
	}
}

func TestInvalidTargetUnsupportedScheme(t *testing.T) {
	_, _, exitCode := runStackdiag(t, "ftp://host")
	if exitCode != 1 {
		t.Errorf("exit code = %d, want 1 for unsupported scheme", exitCode)
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

// --- Timeout range validation ---

func TestTimeoutTooLow(t *testing.T) {
	_, stderr, exitCode := runStackdiag(t, "--timeout", "0", "https://example.com")
	if exitCode != 1 {
		t.Errorf("exit code = %d, want 1", exitCode)
	}
	if !strings.Contains(stderr, "between 1 and 300") {
		t.Errorf("stderr should mention 'between 1 and 300', got: %s", stderr)
	}
}

func TestTimeoutTooHigh(t *testing.T) {
	_, stderr, exitCode := runStackdiag(t, "--timeout", "301", "https://example.com")
	if exitCode != 1 {
		t.Errorf("exit code = %d, want 1", exitCode)
	}
	if !strings.Contains(stderr, "between 1 and 300") {
		t.Errorf("stderr should mention 'between 1 and 300', got: %s", stderr)
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
	// Pass actual CR+LF control characters in the header value.
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
	// The conflict is detected in ParseArgs, which uses HasJSONFlag → JSON on stdout.
	errObj := parseJSONError(t, stdout)
	msg, _ := errObj["message"].(string)
	if !strings.Contains(msg, "conflicts") {
		t.Errorf("error message should mention 'conflicts', got: %s", msg)
	}
}

// --- Auth env whitespace validation ---

func TestBearerEnvWhitespaceOnly(t *testing.T) {
	t.Setenv("TEST_BEARER_WS", " ")

	// --json is required because ResolveAuth errors use cfg.JSON (exitToolError path).
	stdout, _, exitCode := runStackdiag(t,
		"--json",
		"--bearer-env", "TEST_BEARER_WS",
		"https://example.com")
	if exitCode != 1 {
		t.Errorf("exit code = %d, want 1", exitCode)
	}
	errObj := parseJSONError(t, stdout)
	msg, _ := errObj["message"].(string)
	if !strings.Contains(msg, "whitespace") {
		t.Errorf("error message should mention 'whitespace', got: %s", msg)
	}
}

func TestBasicEnvWhitespaceOnly(t *testing.T) {
	t.Setenv("TEST_BASIC_WS", " ")

	// --json is required because ResolveAuth errors use cfg.JSON (exitToolError path).
	stdout, _, exitCode := runStackdiag(t,
		"--json",
		"--basic-env", "TEST_BASIC_WS",
		"https://example.com")
	if exitCode != 1 {
		t.Errorf("exit code = %d, want 1", exitCode)
	}
	errObj := parseJSONError(t, stdout)
	msg, _ := errObj["message"].(string)
	if !strings.Contains(msg, "whitespace") {
		t.Errorf("error message should mention 'whitespace', got: %s", msg)
	}
}

// --- --count flag validation ---

func TestCountZeroError(t *testing.T) {
	_, _, exitCode := runStackdiag(t, "--count", "0", "https://example.com")
	if exitCode != 1 {
		t.Errorf("exit code = %d, want 1 for --count 0", exitCode)
	}
}

func TestCountNegativeError(t *testing.T) {
	_, _, exitCode := runStackdiag(t, "--count", "-1", "https://example.com")
	if exitCode != 1 {
		t.Errorf("exit code = %d, want 1 for --count -1", exitCode)
	}
}

func TestCountNonNumericError(t *testing.T) {
	_, _, exitCode := runStackdiag(t, "--count", "abc", "https://example.com")
	if exitCode != 1 {
		t.Errorf("exit code = %d, want 1 for --count abc", exitCode)
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
	// Without --json, errors go to stderr with "Error:" prefix and USAGE block.
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
	// --json-pretty with parse error should produce multi-line indented JSON on stdout.
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
	// Must be multi-line (indented).
	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	if len(lines) <= 1 {
		t.Error("--json-pretty error output should be multi-line indented")
	}
	if !strings.Contains(stdout, "  ") {
		t.Error("--json-pretty error output should be indented with spaces")
	}
}
