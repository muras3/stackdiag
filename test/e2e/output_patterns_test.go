package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"testing"
)

// localTCPListener creates a TCP listener on 127.0.0.1 and returns the port.
func localTCPListener(t *testing.T) (net.Listener, int) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	return ln, ln.Addr().(*net.TCPAddr).Port
}

// --- Local TCP tests ---

func TestCountJSONCompact(t *testing.T) {
	ln, port := localTCPListener(t)
	defer ln.Close()

	stdout, _, exitCode := runStackdiag(t,
		"--json", "--count", "2", "--timeout", "3",
		fmt.Sprintf("tcp://127.0.0.1:%d", port),
	)

	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0", exitCode)
	}
	if len(stdout) == 0 {
		t.Fatal("expected JSON output on stdout")
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout)
	}

	for _, key := range []string{"schema_version", "target", "count", "attempts", "statistics", "exit_code"} {
		if _, ok := result[key]; !ok {
			t.Errorf("missing required field: %q", key)
		}
	}

	attempts, ok := result["attempts"].([]any)
	if !ok {
		t.Fatal("attempts is not an array")
	}
	if len(attempts) != 2 {
		t.Errorf("attempts length = %d, want 2", len(attempts))
	}

	// Compact = single line (no newlines except trailing).
	trimmed := strings.TrimRight(stdout, "\n")
	if strings.Contains(trimmed, "\n") {
		t.Error("--json output should be compact (single line), found embedded newlines")
	}
}

func TestCountTableOutput(t *testing.T) {
	ln, port := localTCPListener(t)
	defer ln.Close()

	stdout, _, _ := runStackdiag(t,
		"--count", "2", "--timeout", "3",
		fmt.Sprintf("tcp://127.0.0.1:%d", port),
	)

	if len(stdout) == 0 {
		t.Fatal("expected table output on stdout")
	}

	if !strings.Contains(stdout, "Attempt 1/2") {
		t.Errorf("table missing 'Attempt 1/2', got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "Attempt 2/2") {
		t.Errorf("table missing 'Attempt 2/2', got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "Statistics") {
		t.Errorf("table missing 'Statistics' section, got:\n%s", stdout)
	}

	// Should NOT be valid JSON.
	var tmp map[string]any
	if json.Unmarshal([]byte(stdout), &tmp) == nil {
		t.Error("table output should not be valid JSON")
	}
}

func TestTableOutputNoColor(t *testing.T) {
	ln, port := localTCPListener(t)
	defer ln.Close()

	bin := binaryPath(t)
	cmd := exec.Command(bin, "--timeout", "3",
		fmt.Sprintf("tcp://127.0.0.1:%d", port))
	// Propagate environment and add NO_COLOR=1.
	cmd.Env = append(cmd.Environ(), "NO_COLOR=1")

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	_ = cmd.Run() // exit code may be non-zero; we don't care here

	stdout := outBuf.String()

	// Should not contain ANSI escape sequences.
	if strings.Contains(stdout, "\x1b[") {
		t.Errorf("NO_COLOR=1 output should not contain ANSI escape sequences, got:\n%s", stdout)
	}

	// Should contain bracket indicators instead of Unicode symbols.
	hasIndicator := strings.Contains(stdout, "[ok]") ||
		strings.Contains(stdout, "[skip]") ||
		strings.Contains(stdout, "[FAIL]") ||
		strings.Contains(stdout, "[fail]") ||
		strings.Contains(stdout, "[warn]")
	if !hasIndicator {
		t.Errorf("NO_COLOR=1 output should contain bracket indicators, got:\n%s", stdout)
	}
}

func TestTableOutputPipedNoANSI(t *testing.T) {
	ln, port := localTCPListener(t)
	defer ln.Close()

	// runStackdiag captures stdout via a bytes.Buffer pipe,
	// which triggers the IsColorEnabled piped path.
	stdout, _, _ := runStackdiag(t,
		"--timeout", "3",
		fmt.Sprintf("tcp://127.0.0.1:%d", port),
	)

	if len(stdout) == 0 {
		t.Fatal("expected output on stdout")
	}

	// When stdout is a pipe (not a TTY), color should be disabled.
	if strings.Contains(stdout, "\x1b[") {
		t.Errorf("piped output should not contain ANSI escape sequences, got:\n%s", stdout)
	}
}

func TestTableContainsAllLayerNames(t *testing.T) {
	ln, port := localTCPListener(t)
	defer ln.Close()

	stdout, _, _ := runStackdiag(t,
		"--timeout", "3",
		fmt.Sprintf("tcp://127.0.0.1:%d", port),
	)

	lower := strings.ToLower(stdout)
	for _, layer := range []string{"dns", "reachability", "tcp", "tls", "http"} {
		if !strings.Contains(lower, layer) {
			t.Errorf("table missing layer name %q, got:\n%s", layer, stdout)
		}
	}
}

func TestTableSummaryLine(t *testing.T) {
	ln, port := localTCPListener(t)
	defer ln.Close()

	stdout, _, _ := runStackdiag(t,
		"--timeout", "3",
		fmt.Sprintf("tcp://127.0.0.1:%d", port),
	)

	lower := strings.ToLower(stdout)
	if !strings.Contains(lower, "total") {
		t.Errorf("table missing 'total' in summary line, got:\n%s", stdout)
	}
	if !strings.Contains(lower, "ms") {
		t.Errorf("table missing 'ms' in summary line, got:\n%s", stdout)
	}
}

func TestCountTableStatistics(t *testing.T) {
	ln, port := localTCPListener(t)
	defer ln.Close()

	stdout, _, _ := runStackdiag(t,
		"--count", "3", "--timeout", "3",
		fmt.Sprintf("tcp://127.0.0.1:%d", port),
	)

	if !strings.Contains(stdout, "Statistics") {
		t.Errorf("table missing 'Statistics' section, got:\n%s", stdout)
	}

	lower := strings.ToLower(stdout)
	if !strings.Contains(lower, "p50") {
		t.Errorf("Statistics section missing 'p50', got:\n%s", stdout)
	}
	if !strings.Contains(lower, "p95") {
		t.Errorf("Statistics section missing 'p95', got:\n%s", stdout)
	}
}

func TestJSONCompactIsSingleLine(t *testing.T) {
	ln, port := localTCPListener(t)
	defer ln.Close()

	stdout, _, _ := runStackdiag(t,
		"--json", "--timeout", "3",
		fmt.Sprintf("tcp://127.0.0.1:%d", port),
	)

	if len(stdout) == 0 {
		t.Fatal("expected JSON output on stdout")
	}

	// Strip trailing newline, then verify no embedded newlines remain.
	trimmed := strings.TrimRight(stdout, "\n")
	if strings.Contains(trimmed, "\n") {
		t.Errorf("--json output should be a single line, found embedded newlines:\n%s", stdout)
	}
}

// --- Network tests ---

func TestTableOutputHTTPS(t *testing.T) {
	skipIfUnreachable(t, "example.com")

	stdout, _, _ := runStackdiag(t, "https://example.com", "--timeout", "10")

	if len(stdout) == 0 {
		t.Fatal("expected table output on stdout")
	}

	// Should contain status indicators.
	hasIndicator := strings.ContainsAny(stdout, "✓⚠✗") ||
		strings.Contains(stdout, "[ok]") ||
		strings.Contains(stdout, "[skip]") ||
		strings.Contains(stdout, "[FAIL]") ||
		strings.Contains(stdout, "[fail]") ||
		strings.Contains(stdout, "OK") ||
		strings.Contains(stdout, "FAIL")
	if !hasIndicator {
		t.Errorf("table output missing status indicators, got:\n%s", stdout)
	}

	// Should contain layer names.
	lower := strings.ToLower(stdout)
	for _, layer := range []string{"dns", "tcp", "tls", "http"} {
		if !strings.Contains(lower, layer) {
			t.Errorf("table missing layer name %q, got:\n%s", layer, stdout)
		}
	}

	// Should contain a summary with total and ms.
	if !strings.Contains(lower, "total") || !strings.Contains(lower, "ms") {
		t.Errorf("table output missing summary line (total/ms), got:\n%s", stdout)
	}

	// Should NOT be valid JSON.
	var tmp map[string]any
	if json.Unmarshal([]byte(stdout), &tmp) == nil {
		t.Error("table output should not be valid JSON")
	}
}

func TestTableOutputDNSFailure(t *testing.T) {
	stdout, _, exitCode := runStackdiag(t,
		"https://this-domain-does-not-exist-xyz123.example", "--timeout", "5",
	)

	if exitCode != 10 {
		t.Errorf("exit code = %d, want 10 (DNS failure)", exitCode)
	}

	lower := strings.ToLower(stdout)

	// DNS layer should be present.
	if !strings.Contains(lower, "dns") {
		t.Errorf("table output missing 'dns' layer, got:\n%s", stdout)
	}

	// Should indicate failure.
	hasFail := strings.Contains(lower, "fail") ||
		strings.Contains(stdout, "[FAIL]") ||
		strings.ContainsRune(stdout, '✗')
	if !hasFail {
		t.Errorf("table output should indicate DNS failure, got:\n%s", stdout)
	}

	// Should NOT be valid JSON (no --json flag).
	var tmp map[string]any
	if json.Unmarshal([]byte(stdout), &tmp) == nil {
		t.Error("table output should not be valid JSON")
	}
}

func TestTableOutputHTTPFailure(t *testing.T) {
	skipIfUnreachable(t, "httpstat.us")

	stdout, _, _ := runStackdiag(t, "http://httpstat.us/500", "--timeout", "10")

	if len(stdout) == 0 {
		t.Fatal("expected table output on stdout")
	}

	lower := strings.ToLower(stdout)

	// HTTP layer should be present.
	if !strings.Contains(lower, "http") {
		t.Errorf("table output missing 'http' layer, got:\n%s", stdout)
	}

	// Should indicate failure.
	hasFail := strings.Contains(lower, "fail") ||
		strings.Contains(stdout, "[FAIL]") ||
		strings.ContainsRune(stdout, '✗')
	if !hasFail {
		t.Errorf("table output should indicate HTTP failure (500), got:\n%s", stdout)
	}
}

func TestCountJSONWithNetwork(t *testing.T) {
	skipIfUnreachable(t, "example.com")

	stdout, _, exitCode := runStackdiag(t,
		"--json", "--count", "2", "http://example.com", "--timeout", "10",
	)

	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0", exitCode)
	}
	if len(stdout) == 0 {
		t.Fatal("expected JSON output on stdout")
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout)
	}

	attempts, ok := result["attempts"].([]any)
	if !ok {
		t.Fatal("attempts is not an array")
	}
	if len(attempts) != 2 {
		t.Errorf("attempts length = %d, want 2", len(attempts))
	}

	stats, ok := result["statistics"].(map[string]any)
	if !ok {
		t.Fatal("missing or invalid statistics field")
	}

	// Check at least one layer has p50_ms and p95_ms.
	foundStats := false
	for _, v := range stats {
		layerStats, ok := v.(map[string]any)
		if !ok {
			continue
		}
		if _, hasP50 := layerStats["p50_ms"]; hasP50 {
			if _, hasP95 := layerStats["p95_ms"]; hasP95 {
				foundStats = true
				break
			}
		}
	}
	if !foundStats {
		t.Errorf("statistics should contain p50_ms and p95_ms in at least one layer, got: %v", stats)
	}
}
