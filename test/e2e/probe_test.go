package e2e

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// binaryPath returns the path to the built probe binary.
func binaryPath(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	projectRoot := filepath.Join(filepath.Dir(file), "..", "..")
	bin := filepath.Join(projectRoot, "bin", "probe")
	if _, err := os.Stat(bin); os.IsNotExist(err) {
		t.Fatalf("binary not found at %s — run 'make build' first", bin)
	}
	return bin
}

func TestVersionFlag(t *testing.T) {
	bin := binaryPath(t)
	out, err := exec.Command(bin, "--version").CombinedOutput()
	if err != nil {
		t.Fatalf("--version failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "probe") {
		t.Errorf("--version output missing 'probe': %s", out)
	}
}

func TestNoArgsExitsWithError(t *testing.T) {
	bin := binaryPath(t)
	cmd := exec.Command(bin)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected non-zero exit for no args")
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("expected ExitError, got %T", err)
	}
	if exitErr.ExitCode() != 1 {
		t.Errorf("exit code = %d, want 1", exitErr.ExitCode())
	}
	if !strings.Contains(string(out), "Usage") {
		t.Errorf("expected usage in stderr: %s", out)
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
