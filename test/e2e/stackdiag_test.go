package e2e

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// binaryPath returns the path to the built stdiag binary.
func binaryPath(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	projectRoot := filepath.Join(filepath.Dir(file), "..", "..")
	bin := filepath.Join(projectRoot, "bin", "stdiag")
	if _, err := os.Stat(bin); os.IsNotExist(err) {
		t.Fatalf("binary not found at %s — run 'make build' first", bin)
	}
	return bin
}

// runStackdiag runs stdiag with separate stdout/stderr capture.
func runStackdiag(t *testing.T, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	bin := binaryPath(t)
	cmd := exec.Command(bin, args...)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	exitCode = 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("unexpected error type %T: %v", err, err)
		}
	}
	return outBuf.String(), errBuf.String(), exitCode
}

// skipIfUnreachable skips the test if the host cannot be reached.
func skipIfUnreachable(t *testing.T, host string) {
	t.Helper()
	// Try port 443 first, then port 80.
	for _, port := range []string{"443", "80"} {
		conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 5e9)
		if err == nil {
			conn.Close()
			return
		}
	}
	t.Skipf("skipping: cannot reach %s on port 443 or 80", host)
}

func TestVersionFlag(t *testing.T) {
	bin := binaryPath(t)
	out, err := exec.Command(bin, "--version").CombinedOutput()
	if err != nil {
		t.Fatalf("--version failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "stdiag") {
		t.Errorf("--version output missing 'stdiag': %s", out)
	}
}

func TestNoArgsExitsWithError(t *testing.T) {
	_, stderr, exitCode := runStackdiag(t)
	if exitCode != 1 {
		t.Errorf("exit code = %d, want 1", exitCode)
	}
	if !strings.Contains(stderr, "USAGE:") {
		t.Errorf("expected usage in stderr: %s", stderr)
	}
}

func TestTCPClosedPort(t *testing.T) {
	// Find a port that is closed by binding and immediately releasing.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	bin := binaryPath(t)
	cmd := exec.Command(bin, "--json", "--timeout", "3",
		fmt.Sprintf("tcp://127.0.0.1:%d", port))
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected non-zero exit for closed TCP port")
	}

	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("expected ExitError, got %T: %v", err, err)
	}

	// Exit code 20 = TCP failure.
	if exitErr.ExitCode() != 20 {
		t.Errorf("exit code = %d, want 20\noutput: %s", exitErr.ExitCode(), out)
	}
}

func TestJSONOutputStructure(t *testing.T) {
	// Use tcp:// against a local listener to test JSON output.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	bin := binaryPath(t)
	cmd := exec.Command(bin, "--json", "--timeout", "3",
		fmt.Sprintf("tcp://127.0.0.1:%d", port))

	// Capture stdout (data) separately.
	out, err := cmd.Output()
	if err != nil {
		t.Logf("command error (may be expected for DNS): %v", err)
	}

	if len(out) == 0 {
		t.Fatal("expected JSON output on stdout")
	}

	var result map[string]any
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, out)
	}

	for _, key := range []string{"schema_version", "target", "layers", "summary"} {
		if _, ok := result[key]; !ok {
			t.Errorf("missing required field: %q", key)
		}
	}

	if v, ok := result["schema_version"].(string); !ok || v != "v0.1" {
		t.Errorf("schema_version = %v, want v0.1", result["schema_version"])
	}
}

// --- New E2E tests ---

func TestHelpFlag(t *testing.T) {
	stdout, stderr, exitCode := runStackdiag(t, "--help")
	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0", exitCode)
	}
	if !strings.Contains(stdout, "USAGE:") {
		t.Errorf("expected usage info on stdout, got: %s", stdout)
	}
	if len(stderr) > 0 {
		t.Errorf("expected empty stderr for --help, got: %s", stderr)
	}
}

func TestInvalidTarget(t *testing.T) {
	_, _, exitCode := runStackdiag(t, "ftp://example.com")
	if exitCode != 1 {
		t.Errorf("exit code = %d, want 1 for invalid target", exitCode)
	}
}

func TestJSONOnError(t *testing.T) {
	// Even on failure, --json should produce valid JSON (schema contract rule 4).
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close() // close so connection fails

	stdout, _, exitCode := runStackdiag(t, "--json", "--timeout", "3",
		fmt.Sprintf("tcp://127.0.0.1:%d", port))
	if exitCode == 0 {
		t.Fatal("expected non-zero exit for closed port")
	}

	if len(stdout) == 0 {
		t.Fatal("expected JSON output on stdout even on failure")
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("invalid JSON on failure: %v\n%s", err, stdout)
	}

	// Verify required fields are present even on error.
	for _, key := range []string{"schema_version", "target", "layers", "summary"} {
		if _, ok := result[key]; !ok {
			t.Errorf("missing required field %q in error JSON", key)
		}
	}
}

func TestTableOutputDefault(t *testing.T) {
	// Without --json, output should be table format with status symbols.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	stdout, _, _ := runStackdiag(t, "--timeout", "3",
		fmt.Sprintf("tcp://127.0.0.1:%d", port))

	// Table output should contain status symbols or text indicators.
	hasSymbol := strings.ContainsAny(stdout, "✓⚠✗") ||
		strings.Contains(stdout, "OK") ||
		strings.Contains(stdout, "WARN") ||
		strings.Contains(stdout, "FAIL")
	if !hasSymbol {
		t.Errorf("table output missing status symbols, got:\n%s", stdout)
	}

	// Should NOT be valid JSON.
	var tmp map[string]any
	if json.Unmarshal([]byte(stdout), &tmp) == nil {
		t.Error("table output should not be valid JSON")
	}
}

func TestStdoutStderrSeparation(t *testing.T) {
	// Verify data goes to stdout and log/errors go to stderr.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	stdout, stderr, _ := runStackdiag(t, "--json", "--timeout", "3",
		fmt.Sprintf("tcp://127.0.0.1:%d", port))

	// stdout must contain valid JSON data.
	if len(stdout) == 0 {
		t.Fatal("expected data on stdout")
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Errorf("stdout should be valid JSON: %v", err)
	}

	// stderr should not contain JSON data (only logs/diagnostics if any).
	if len(stderr) > 0 {
		var tmp map[string]any
		if json.Unmarshal([]byte(stderr), &tmp) == nil {
			t.Error("stderr should not contain JSON data — stdout=data, stderr=logs")
		}
	}
}

func TestTimeoutFlag(t *testing.T) {
	skipIfUnreachable(t, "example.com")

	stdout, _, exitCode := runStackdiag(t, "https://example.com", "--timeout", "10", "--json")
	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0", exitCode)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
}

func TestInsecureFlag(t *testing.T) {
	skipIfUnreachable(t, "self-signed.badssl.com")

	stdout, _, exitCode := runStackdiag(t, "https://self-signed.badssl.com", "--insecure", "--json", "--timeout", "10")

	// With --insecure, TLS should not fail even for self-signed certs.
	if exitCode == 30 {
		t.Error("exit code 30 (TLS failure) should not occur with --insecure")
	}

	if len(stdout) > 0 {
		var result map[string]any
		if err := json.Unmarshal([]byte(stdout), &result); err != nil {
			t.Logf("stdout: %s", stdout)
			t.Fatalf("invalid JSON: %v", err)
		}

		if layers, ok := result["layers"].(map[string]any); ok {
			if tls, ok := layers["tls"].(map[string]any); ok {
				if status, ok := tls["status"].(string); ok && status == "fail" {
					t.Error("tls layer should not fail with --insecure")
				}
			}
		}
	}
}

func TestMethodFlag(t *testing.T) {
	skipIfUnreachable(t, "example.com")

	stdout, _, exitCode := runStackdiag(t, "https://example.com", "--method", "HEAD", "--json", "--timeout", "10")
	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0", exitCode)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	// Check that the target or HTTP layer reflects the method.
	if layers, ok := result["layers"].(map[string]any); ok {
		if httpLayer, ok := layers["http"].(map[string]any); ok {
			if method, ok := httpLayer["method"].(string); ok && method != "HEAD" {
				t.Errorf("method = %q, want HEAD", method)
			}
		}
	}
}

func TestHeaderFlag(t *testing.T) {
	skipIfUnreachable(t, "example.com")

	stdout, _, exitCode := runStackdiag(t, "https://example.com", "--header", "X-Test: value", "--json", "--timeout", "10")
	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0", exitCode)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	// Basic validation: JSON should be valid with the required fields.
	for _, key := range []string{"schema_version", "target", "layers", "summary"} {
		if _, ok := result[key]; !ok {
			t.Errorf("missing required field: %q", key)
		}
	}
}

func TestExitCodeWarn(t *testing.T) {
	// Exit code 2 = warn. This is hard to trigger reliably without an expired cert.
	// Use expired.badssl.com but with --insecure to avoid TLS fail and check for HTTP warn.
	t.Skip("exit code 2 (warn) is difficult to trigger reliably in E2E without a controlled environment")
}

func TestExitCodeDNSFailure(t *testing.T) {
	_, _, exitCode := runStackdiag(t, "https://this-domain-does-not-exist-xyz123.example", "--json", "--timeout", "5")
	if exitCode != 10 {
		t.Errorf("exit code = %d, want 10 (DNS failure)", exitCode)
	}
}

func TestExitCodeTLSFailure(t *testing.T) {
	skipIfUnreachable(t, "expired.badssl.com")

	stdout, _, exitCode := runStackdiag(t, "https://expired.badssl.com", "--json", "--timeout", "10")
	if exitCode != 30 {
		t.Errorf("exit code = %d, want 30 (TLS failure)", exitCode)
	}

	if len(stdout) > 0 {
		var result map[string]any
		if err := json.Unmarshal([]byte(stdout), &result); err != nil {
			t.Fatalf("invalid JSON on TLS failure: %v", err)
		}
		if layers, ok := result["layers"].(map[string]any); ok {
			if tls, ok := layers["tls"].(map[string]any); ok {
				if status, ok := tls["status"].(string); ok && status != "fail" {
					t.Errorf("tls status = %q, want fail", status)
				}
			}
		}
	}
}

func TestExitCodeHTTPFailure(t *testing.T) {
	// Use http:// (not https://) to avoid TLS layer interfering.
	skipIfUnreachable(t, "httpstat.us")

	stdout, _, exitCode := runStackdiag(t, "http://httpstat.us/500", "--json", "--timeout", "10")
	if exitCode != 40 {
		t.Errorf("exit code = %d, want 40 (HTTP failure)", exitCode)
	}

	if len(stdout) > 0 {
		var result map[string]any
		if err := json.Unmarshal([]byte(stdout), &result); err != nil {
			t.Fatalf("invalid JSON on HTTP failure: %v", err)
		}
	}
}

func TestFlagsAfterTarget(t *testing.T) {
	// Regression test: flags after the target should work.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	// Place --json AFTER the target.
	stdout, _, _ := runStackdiag(t, fmt.Sprintf("tcp://127.0.0.1:%d", port), "--json", "--timeout", "3")

	if len(stdout) == 0 {
		t.Fatal("expected JSON output when --json comes after target")
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("invalid JSON with flags after target: %v\n%s", err, stdout)
	}
}

func TestHTTPScheme(t *testing.T) {
	skipIfUnreachable(t, "example.com")

	stdout, _, exitCode := runStackdiag(t, "http://example.com", "--json", "--timeout", "10")
	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0", exitCode)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	// For http:// scheme, TLS layer should be skipped.
	if layers, ok := result["layers"].(map[string]any); ok {
		if tls, ok := layers["tls"].(map[string]any); ok {
			if status, ok := tls["status"].(string); ok && status != "skip" {
				t.Errorf("tls status = %q, want skip for http:// scheme", status)
			}
		}
		// If tls layer is absent entirely, that's also acceptable.
	}
}

func TestRedactFlagAccepted(t *testing.T) {
	// --redact should be accepted without error and produce valid JSON.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	stdout, stderr, _ := runStackdiag(t, "--json", "--timeout", "3", "--redact",
		fmt.Sprintf("tcp://127.0.0.1:%d", port))

	if strings.Contains(stderr, "unknown flag") || strings.Contains(stderr, "flag provided but not defined") {
		t.Errorf("--redact flag not recognized: %s", stderr)
	}

	if len(stdout) == 0 {
		t.Fatal("expected JSON output")
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("invalid JSON with --redact: %v\n%s", err, stdout)
	}
}

func TestNoRedactFlagAccepted(t *testing.T) {
	// --no-redact should be accepted without error and produce valid JSON.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	stdout, stderr, _ := runStackdiag(t, "--json", "--timeout", "3", "--no-redact",
		fmt.Sprintf("tcp://127.0.0.1:%d", port))

	if strings.Contains(stderr, "unknown flag") || strings.Contains(stderr, "flag provided but not defined") {
		t.Errorf("--no-redact flag not recognized: %s", stderr)
	}

	if len(stdout) == 0 {
		t.Fatal("expected JSON output")
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("invalid JSON with --no-redact: %v\n%s", err, stdout)
	}
}

func TestRedactMasksAuthorizationHeader(t *testing.T) {
	skipIfUnreachable(t, "example.com")
	secret := "Bearer secret-token-e2e-test"

	stdout, _, exitCode := runStackdiag(t, "https://example.com", "--json", "--timeout", "10",
		"--header", "Authorization: "+secret)
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}

	// The raw secret must NOT appear anywhere in stdout.
	if strings.Contains(stdout, "secret-token-e2e-test") {
		t.Fatal("secret appeared in stdout with default redaction enabled")
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	layers := result["layers"].(map[string]any)
	httpLayer := layers["http"].(map[string]any)
	obs := httpLayer["observations"].(map[string]any)
	headers, ok := obs["request_headers"].(map[string]any)
	if !ok {
		t.Fatal("missing request_headers in observations")
	}
	if headers["Authorization"] != "[REDACTED]" {
		t.Fatalf("Authorization = %v, want [REDACTED]", headers["Authorization"])
	}
}

func TestNoRedactShowsRawAuthorizationHeader(t *testing.T) {
	skipIfUnreachable(t, "example.com")
	secret := "Bearer secret-token-e2e-noredact"

	stdout, _, exitCode := runStackdiag(t, "https://example.com", "--json", "--timeout", "10",
		"--no-redact", "--header", "Authorization: "+secret)
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	layers := result["layers"].(map[string]any)
	httpLayer := layers["http"].(map[string]any)
	obs := httpLayer["observations"].(map[string]any)
	headers, ok := obs["request_headers"].(map[string]any)
	if !ok {
		t.Fatal("missing request_headers in observations")
	}
	if headers["Authorization"] != secret {
		t.Fatalf("Authorization = %v, want %q", headers["Authorization"], secret)
	}
}

func TestNonSensitiveHeaderShownWithRedact(t *testing.T) {
	skipIfUnreachable(t, "example.com")

	stdout, _, exitCode := runStackdiag(t, "https://example.com", "--json", "--timeout", "10",
		"--header", "X-Request-Id: abc123")
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	layers := result["layers"].(map[string]any)
	httpLayer := layers["http"].(map[string]any)
	obs := httpLayer["observations"].(map[string]any)
	headers, ok := obs["request_headers"].(map[string]any)
	if !ok {
		t.Fatal("missing request_headers in observations")
	}
	if headers["X-Request-Id"] != "abc123" {
		t.Fatalf("X-Request-Id = %v, want abc123", headers["X-Request-Id"])
	}
}

func TestBareHostname(t *testing.T) {
	skipIfUnreachable(t, "example.com")

	stdout, _, exitCode := runStackdiag(t, "example.com", "--json", "--timeout", "10")
	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0", exitCode)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	// Bare hostname should default to HTTPS, so TLS layer should be present and not skipped.
	if layers, ok := result["layers"].(map[string]any); ok {
		if tls, ok := layers["tls"].(map[string]any); ok {
			if status, ok := tls["status"].(string); ok && status == "skip" {
				t.Error("bare hostname should default to HTTPS, but tls was skipped")
			}
		} else {
			t.Error("bare hostname should include tls layer (defaults to HTTPS)")
		}
	}
}

// --- Auth header injection E2E tests ---

// headerCaptureServer starts an HTTP server that captures the Authorization header
// from incoming requests and returns it in the response body.
func headerCaptureServer(t *testing.T) (*httptest.Server, *string) {
	t.Helper()
	var captured string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "OK")
	}))
	t.Cleanup(srv.Close)
	return srv, &captured
}

func TestBearerEnvSendsToken(t *testing.T) {
	srv, captured := headerCaptureServer(t)

	token := "my-secret-bearer-token-12345"
	t.Setenv("TEST_BEARER_TOKEN", token)

	_, _, exitCode := runStackdiag(t, srv.URL, "--bearer-env", "TEST_BEARER_TOKEN", "--json", "--timeout", "5")
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}

	want := "Bearer " + token
	if *captured != want {
		t.Errorf("Authorization header = %q, want %q", *captured, want)
	}
}

func TestBasicEnvSendsHeader(t *testing.T) {
	srv, captured := headerCaptureServer(t)

	cred := "admin:hunter2"
	t.Setenv("TEST_BASIC_CRED", cred)

	_, _, exitCode := runStackdiag(t, srv.URL, "--basic-env", "TEST_BASIC_CRED", "--json", "--timeout", "5")
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}

	want := "Basic " + base64.StdEncoding.EncodeToString([]byte(cred))
	if *captured != want {
		t.Errorf("Authorization header = %q, want %q", *captured, want)
	}
}

func TestBearerEnvUnsetExits1(t *testing.T) {
	// Use an env var name that does not exist.
	stdout, _, exitCode := runStackdiag(t, "https://example.com", "--bearer-env", "NONEXISTENT_VAR_E2E_TEST", "--json", "--timeout", "5")
	if exitCode != 1 {
		t.Errorf("exit code = %d, want 1", exitCode)
	}

	// With --json, error goes to stdout as structured JSON.
	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("expected valid JSON: %v\n%s", err, stdout)
	}
	errObj, ok := result["error"].(map[string]any)
	if !ok {
		t.Fatal("missing error object")
	}
	if errObj["code"] != "INVALID_ARGS" {
		t.Errorf("error code = %v, want INVALID_ARGS", errObj["code"])
	}
	msg, _ := errObj["message"].(string)
	if !strings.Contains(msg, "not set") {
		t.Errorf("error message should mention 'not set', got: %s", msg)
	}
}

func TestBearerEnvEmptyExits1(t *testing.T) {
	t.Setenv("TEST_BEARER_EMPTY", "")

	stdout, _, exitCode := runStackdiag(t, "https://example.com", "--bearer-env", "TEST_BEARER_EMPTY", "--json", "--timeout", "5")
	if exitCode != 1 {
		t.Errorf("exit code = %d, want 1", exitCode)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("expected valid JSON: %v\n%s", err, stdout)
	}
	errObj, _ := result["error"].(map[string]any)
	msg, _ := errObj["message"].(string)
	if !strings.Contains(msg, "empty") {
		t.Errorf("error message should mention 'empty', got: %s", msg)
	}
}

func TestBasicEnvNoColonExits1(t *testing.T) {
	t.Setenv("TEST_BASIC_NOCOLON", "nocolon")

	stdout, _, exitCode := runStackdiag(t, "https://example.com", "--basic-env", "TEST_BASIC_NOCOLON", "--json", "--timeout", "5")
	if exitCode != 1 {
		t.Errorf("exit code = %d, want 1", exitCode)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("expected valid JSON: %v\n%s", err, stdout)
	}
	errObj, _ := result["error"].(map[string]any)
	msg, _ := errObj["message"].(string)
	if !strings.Contains(msg, "user:password") {
		t.Errorf("error message should mention format requirement, got: %s", msg)
	}

	// Security: stdout must NOT contain the credential value.
	if strings.Contains(stdout, "nocolon") {
		t.Error("stdout should not leak credential values")
	}
}

func TestBearerEnvBasicEnvConflict(t *testing.T) {
	t.Setenv("TEST_TOK", "token")
	t.Setenv("TEST_CRED", "user:pass")

	stdout, _, exitCode := runStackdiag(t, "https://example.com",
		"--bearer-env", "TEST_TOK", "--basic-env", "TEST_CRED", "--json", "--timeout", "5")
	if exitCode != 1 {
		t.Errorf("exit code = %d, want 1", exitCode)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("expected valid JSON: %v\n%s", err, stdout)
	}
	errObj, _ := result["error"].(map[string]any)
	msg, _ := errObj["message"].(string)
	if !strings.Contains(msg, "cannot be used together") {
		t.Errorf("error message should mention conflict, got: %s", msg)
	}
}

func TestBearerEnvHeaderConflict(t *testing.T) {
	t.Setenv("TEST_TOK2", "token")

	stdout, _, exitCode := runStackdiag(t, "https://example.com",
		"--bearer-env", "TEST_TOK2", "--header", "Authorization: Bearer manual", "--json", "--timeout", "5")
	if exitCode != 1 {
		t.Errorf("exit code = %d, want 1", exitCode)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("expected valid JSON: %v\n%s", err, stdout)
	}
	errObj, _ := result["error"].(map[string]any)
	msg, _ := errObj["message"].(string)
	if !strings.Contains(msg, "conflicts") {
		t.Errorf("error message should mention conflict, got: %s", msg)
	}
}

func TestBearerEnvRedactedInJSON(t *testing.T) {
	srv, _ := headerCaptureServer(t)

	token := "super-secret-redacted-test"
	t.Setenv("TEST_BEARER_REDACT", token)

	stdout, _, exitCode := runStackdiag(t, srv.URL, "--bearer-env", "TEST_BEARER_REDACT", "--json", "--timeout", "5")
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}

	// Raw token must NOT appear in stdout.
	if strings.Contains(stdout, token) {
		t.Fatal("raw token appeared in JSON output with default redaction")
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout)
	}

	layers := result["layers"].(map[string]any)
	httpLayer := layers["http"].(map[string]any)
	obs := httpLayer["observations"].(map[string]any)
	headers, ok := obs["request_headers"].(map[string]any)
	if !ok {
		t.Fatal("missing request_headers in observations")
	}
	if headers["Authorization"] != "[REDACTED]" {
		t.Errorf("Authorization = %v, want [REDACTED]", headers["Authorization"])
	}
}

func TestJSONInvalidTargetStructuredError(t *testing.T) {
	stdout, _, exitCode := runStackdiag(t, "--json", "ftp://example.com")
	if exitCode != 1 {
		t.Errorf("exit code = %d, want 1", exitCode)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("expected valid JSON on stdout for invalid target: %v\n%s", err, stdout)
	}

	errObj, ok := result["error"].(map[string]any)
	if !ok {
		t.Fatal("missing error object in JSON output")
	}
	if errObj["code"] != "INVALID_TARGET" {
		t.Errorf("error code = %v, want INVALID_TARGET", errObj["code"])
	}
}

func TestJSONNoArgsStructuredError(t *testing.T) {
	stdout, _, exitCode := runStackdiag(t, "--json")
	if exitCode != 1 {
		t.Errorf("exit code = %d, want 1", exitCode)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("expected valid JSON on stdout for missing args: %v\n%s", err, stdout)
	}

	errObj, ok := result["error"].(map[string]any)
	if !ok {
		t.Fatal("missing error object in JSON output")
	}
	if errObj["code"] != "INVALID_ARGS" {
		t.Errorf("error code = %v, want INVALID_ARGS", errObj["code"])
	}
}

func TestBearerEnvNoRedactShowsRaw(t *testing.T) {
	srv, _ := headerCaptureServer(t)

	token := "raw-token-visible-test"
	t.Setenv("TEST_BEARER_NOREDACT", token)

	stdout, _, exitCode := runStackdiag(t, srv.URL, "--bearer-env", "TEST_BEARER_NOREDACT",
		"--no-redact", "--json", "--timeout", "5")
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout)
	}

	layers := result["layers"].(map[string]any)
	httpLayer := layers["http"].(map[string]any)
	obs := httpLayer["observations"].(map[string]any)
	headers, ok := obs["request_headers"].(map[string]any)
	if !ok {
		t.Fatal("missing request_headers in observations")
	}

	want := "Bearer " + token
	if headers["Authorization"] != want {
		t.Errorf("Authorization = %v, want %q", headers["Authorization"], want)
	}
}
