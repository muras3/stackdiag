package e2e

import (
	"encoding/json"
	"strings"
	"testing"
)

// --- TLS Scan tests ---

func TestTLSScanJSON(t *testing.T) {
	skipIfUnreachable(t, "example.com")

	stdout, _, exitCode := runStackdiag(t, "--json", "--tls-scan", "https://example.com", "--timeout", "15")
	// Exit code 0 (ok) or 2 (warn, e.g. deprecated TLS versions detected) are both acceptable.
	if exitCode != 0 && exitCode != 2 {
		t.Fatalf("exit code = %d, want 0 or 2\nstdout: %s", exitCode, stdout)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout)
	}

	layers, ok := result["layers"].(map[string]any)
	if !ok {
		t.Fatal("missing or invalid layers field")
	}

	tlsLayer, ok := layers["tls"].(map[string]any)
	if !ok {
		t.Fatal("missing tls layer")
	}

	obs, ok := tlsLayer["observations"].(map[string]any)
	if !ok {
		t.Fatal("missing tls observations")
	}

	tlsScan, ok := obs["tls_scan"].(map[string]any)
	if !ok {
		t.Fatal("missing tls_scan object in tls observations")
	}

	// Verify tls_scan has required fields.
	performed, ok := tlsScan["performed"].(bool)
	if !ok {
		t.Fatal("tls_scan.performed is not a bool")
	}
	if !performed {
		t.Error("tls_scan.performed = false, want true")
	}

	attempts, ok := tlsScan["attempts"].([]any)
	if !ok {
		t.Fatal("tls_scan.attempts is not an array")
	}
	if len(attempts) == 0 {
		t.Error("tls_scan.attempts is empty, expected at least one version attempt")
	}

	supportedVersions, ok := tlsScan["supported_versions"].([]any)
	if !ok {
		t.Fatal("tls_scan.supported_versions is not an array")
	}
	if len(supportedVersions) == 0 {
		t.Error("tls_scan.supported_versions is empty, expected at least one supported version")
	}
}

func TestTLSScanTable(t *testing.T) {
	skipIfUnreachable(t, "example.com")

	stdout, _, exitCode := runStackdiag(t, "--tls-scan", "https://example.com", "--timeout", "15")
	// Exit code 0 (ok) or 2 (warn, e.g. deprecated TLS versions detected) are both acceptable.
	if exitCode != 0 && exitCode != 2 {
		t.Errorf("exit code = %d, want 0 or 2", exitCode)
	}

	// Should contain TLS version info.
	hasTLSVersion := strings.Contains(stdout, "TLSv1.2") || strings.Contains(stdout, "TLSv1.3")
	if !hasTLSVersion {
		t.Errorf("table output missing TLS version info (TLSv1.2 or TLSv1.3), got:\n%s", stdout)
	}

	// Should NOT be valid JSON.
	var tmp map[string]any
	if json.Unmarshal([]byte(stdout), &tmp) == nil {
		t.Error("table output should not be valid JSON")
	}
}

// --- DNS Server tests ---

func TestDNSServerJSON(t *testing.T) {
	skipIfUnreachable(t, "example.com")

	stdout, _, exitCode := runStackdiag(t, "--json", "--dns-server", "8.8.8.8:53", "https://example.com", "--timeout", "10")
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0\nstdout: %s", exitCode, stdout)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout)
	}

	layers, ok := result["layers"].(map[string]any)
	if !ok {
		t.Fatal("missing or invalid layers field")
	}

	dnsLayer, ok := layers["dns"].(map[string]any)
	if !ok {
		t.Fatal("missing dns layer")
	}

	obs, ok := dnsLayer["observations"].(map[string]any)
	if !ok {
		t.Fatal("missing dns observations")
	}

	resolverAddr, ok := obs["resolver_address"].(string)
	if !ok {
		t.Fatalf("resolver_address is not a string, got: %v", obs["resolver_address"])
	}
	if resolverAddr != "8.8.8.8:53" {
		t.Errorf("resolver_address = %q, want \"8.8.8.8:53\"", resolverAddr)
	}
}

func TestDNSServerTable(t *testing.T) {
	skipIfUnreachable(t, "example.com")

	_, _, exitCode := runStackdiag(t, "--dns-server", "8.8.8.8:53", "https://example.com", "--timeout", "10")
	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0", exitCode)
	}
}

// --- Exit Code 2 (Warn) tests ---

func TestExitCodeWarnInsecureExpired(t *testing.T) {
	skipIfUnreachable(t, "expired.badssl.com")

	stdout, _, exitCode := runStackdiag(t, "--json", "--insecure", "https://expired.badssl.com", "--timeout", "10")

	if len(stdout) == 0 {
		t.Fatal("expected JSON output on stdout")
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout)
	}

	layers, ok := result["layers"].(map[string]any)
	if !ok {
		t.Fatal("missing or invalid layers field")
	}

	tlsLayer, ok := layers["tls"].(map[string]any)
	if !ok {
		t.Fatal("missing tls layer")
	}

	tlsStatus, _ := tlsLayer["status"].(string)

	// Key assertion: TLS layer should NOT be "fail" with --insecure.
	if tlsStatus == "fail" {
		t.Error("tls status = \"fail\", but with --insecure it should be \"warn\" or \"ok\"")
	}

	// Accept exit code 2 (warn), 0 (all ok), or 40 (HTTP issue).
	// The important thing is TLS didn't fail.
	switch exitCode {
	case 0:
		t.Log("exit code 0: all layers passed")
	case 2:
		t.Log("exit code 2: warn path confirmed")
	case 40:
		t.Log("exit code 40: HTTP issue (TLS warn was downgraded)")
	default:
		if exitCode == 30 {
			t.Error("exit code 30 (TLS failure) should not occur with --insecure")
		} else {
			t.Logf("exit code = %d (unexpected but TLS status %q is acceptable)", exitCode, tlsStatus)
		}
	}
}

func TestInsecureExpiredCertObservations(t *testing.T) {
	skipIfUnreachable(t, "expired.badssl.com")

	stdout, _, _ := runStackdiag(t, "--json", "--insecure", "https://expired.badssl.com", "--timeout", "10")

	if len(stdout) == 0 {
		t.Fatal("expected JSON output on stdout")
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout)
	}

	layers, ok := result["layers"].(map[string]any)
	if !ok {
		t.Fatal("missing or invalid layers field")
	}

	tlsLayer, ok := layers["tls"].(map[string]any)
	if !ok {
		t.Fatal("missing tls layer")
	}

	obs, ok := tlsLayer["observations"].(map[string]any)
	if !ok {
		t.Fatal("missing tls observations")
	}

	// cert_verified should be false (retrieved via InsecureSkipVerify).
	certVerified, ok := obs["cert_verified"].(bool)
	if !ok {
		t.Fatal("cert_verified is not a bool")
	}
	if certVerified {
		t.Error("cert_verified = true, want false for expired cert with --insecure")
	}

	// cert_days_until_expiry should be negative (expired).
	daysRaw, ok := obs["cert_days_until_expiry"].(float64)
	if !ok {
		t.Fatalf("cert_days_until_expiry is not a number, got: %v (%T)", obs["cert_days_until_expiry"], obs["cert_days_until_expiry"])
	}
	if daysRaw >= 0 {
		t.Errorf("cert_days_until_expiry = %v, want negative (expired)", daysRaw)
	}
}

// --- Exit Code 15 (Reachability) test ---

func TestExitCodeReachabilityTimeout(t *testing.T) {
	// Use non-routable IP (RFC 5737 TEST-NET-1).
	stdout, _, exitCode := runStackdiag(t, "--json", "--timeout", "3", "tcp://192.0.2.1:1")

	if len(stdout) == 0 {
		t.Fatal("expected JSON output on stdout")
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout)
	}

	// Accept exit code 15 (reachability timeout) or 20 (TCP timeout when ICMP is not permitted).
	switch exitCode {
	case 15:
		t.Log("exit code 15: reachability timeout (ICMP available)")
	case 20:
		t.Log("exit code 20: TCP timeout (ICMP permission denied, reachability skipped)")
	default:
		t.Errorf("exit code = %d, want 15 (reachability) or 20 (TCP)", exitCode)
	}

	// Verify first_non_ok_layer matches the exit code.
	summary, ok := result["summary"].(map[string]any)
	if !ok {
		t.Fatal("missing or invalid summary field")
	}

	firstNonOK, _ := summary["first_non_ok_layer"].(string)
	switch exitCode {
	case 15:
		if firstNonOK != "reachability" {
			t.Errorf("first_non_ok_layer = %q, want \"reachability\" for exit code 15", firstNonOK)
		}
	case 20:
		if firstNonOK != "tcp" {
			t.Errorf("first_non_ok_layer = %q, want \"tcp\" for exit code 20", firstNonOK)
		}
	}
}

// --- Insecure flag variations ---

func TestInsecureSelfSignedOK(t *testing.T) {
	skipIfUnreachable(t, "self-signed.badssl.com")

	stdout, _, _ := runStackdiag(t, "--json", "--insecure", "https://self-signed.badssl.com", "--timeout", "10")

	if len(stdout) == 0 {
		t.Fatal("expected JSON output on stdout")
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout)
	}

	layers, ok := result["layers"].(map[string]any)
	if !ok {
		t.Fatal("missing or invalid layers field")
	}

	tlsLayer, ok := layers["tls"].(map[string]any)
	if !ok {
		t.Fatal("missing tls layer")
	}

	tlsStatus, _ := tlsLayer["status"].(string)
	if tlsStatus == "fail" {
		t.Error("tls status = \"fail\", should not fail with --insecure for self-signed cert")
	}

	obs, ok := tlsLayer["observations"].(map[string]any)
	if !ok {
		t.Fatal("missing tls observations")
	}

	certVerified, ok := obs["cert_verified"].(bool)
	if !ok {
		t.Fatal("cert_verified is not a bool")
	}
	if certVerified {
		t.Error("cert_verified = true, want false for self-signed cert with --insecure")
	}
}

func TestInsecureWrongHostOK(t *testing.T) {
	skipIfUnreachable(t, "wrong.host.badssl.com")

	stdout, _, _ := runStackdiag(t, "--json", "--insecure", "https://wrong.host.badssl.com", "--timeout", "10")

	if len(stdout) == 0 {
		t.Fatal("expected JSON output on stdout")
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout)
	}

	layers, ok := result["layers"].(map[string]any)
	if !ok {
		t.Fatal("missing or invalid layers field")
	}

	tlsLayer, ok := layers["tls"].(map[string]any)
	if !ok {
		t.Fatal("missing tls layer")
	}

	tlsStatus, _ := tlsLayer["status"].(string)
	if tlsStatus == "fail" {
		t.Error("tls status = \"fail\", should not fail with --insecure for wrong host cert")
	}
}

// --- Table output for failure modes ---

func TestTableTLSFailure(t *testing.T) {
	skipIfUnreachable(t, "expired.badssl.com")

	stdout, _, exitCode := runStackdiag(t, "https://expired.badssl.com", "--timeout", "10")

	if exitCode != 30 {
		t.Errorf("exit code = %d, want 30 (TLS failure)", exitCode)
	}

	// Table output should contain TLS failure indicator.
	hasTLSFailure := strings.Contains(stdout, "FAIL") ||
		strings.Contains(stdout, "fail") ||
		strings.Contains(stdout, "✗") ||
		strings.Contains(stdout, "[fail]")
	if !hasTLSFailure {
		t.Errorf("table output missing TLS failure indicator, got:\n%s", stdout)
	}
}

func TestTableDNSServerCustom(t *testing.T) {
	skipIfUnreachable(t, "example.com")

	// No port specified — should default to :53.
	_, _, exitCode := runStackdiag(t, "--dns-server", "8.8.8.8", "https://example.com", "--timeout", "10")
	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0 (--dns-server without port should default to :53)", exitCode)
	}
}

// --- Combined feature tests ---

func TestTLSScanWithInsecure(t *testing.T) {
	skipIfUnreachable(t, "self-signed.badssl.com")

	stdout, _, _ := runStackdiag(t, "--json", "--tls-scan", "--insecure", "https://self-signed.badssl.com", "--timeout", "20")

	if len(stdout) == 0 {
		t.Fatal("expected JSON output on stdout")
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout)
	}

	layers, ok := result["layers"].(map[string]any)
	if !ok {
		t.Fatal("missing or invalid layers field")
	}

	tlsLayer, ok := layers["tls"].(map[string]any)
	if !ok {
		t.Fatal("missing tls layer")
	}

	// TLS should not fail with --insecure.
	tlsStatus, _ := tlsLayer["status"].(string)
	if tlsStatus == "fail" {
		t.Error("tls status = \"fail\", should not fail with --insecure")
	}

	obs, ok := tlsLayer["observations"].(map[string]any)
	if !ok {
		t.Fatal("missing tls observations")
	}

	// tls_scan should be present despite self-signed cert.
	tlsScan, ok := obs["tls_scan"].(map[string]any)
	if !ok {
		t.Fatal("missing tls_scan object — --tls-scan with --insecure should still scan")
	}

	performed, _ := tlsScan["performed"].(bool)
	if !performed {
		t.Error("tls_scan.performed = false, want true")
	}
}

func TestCountWithDNSServer(t *testing.T) {
	skipIfUnreachable(t, "example.com")

	stdout, _, exitCode := runStackdiag(t, "--json", "--count", "2", "--dns-server", "8.8.8.8:53", "http://example.com", "--timeout", "10")
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0\nstdout: %s", exitCode, stdout)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout)
	}

	// Count mode should have "attempts" array.
	attempts, ok := result["attempts"].([]any)
	if !ok {
		t.Fatal("missing or invalid attempts array in count mode")
	}

	if len(attempts) != 2 {
		t.Fatalf("attempts count = %d, want 2", len(attempts))
	}

	// Verify resolver_address in each attempt's DNS layer.
	for i, attempt := range attempts {
		att, ok := attempt.(map[string]any)
		if !ok {
			t.Fatalf("attempt[%d] is not an object", i)
		}

		layers, ok := att["layers"].(map[string]any)
		if !ok {
			t.Fatalf("attempt[%d] missing layers", i)
		}

		dnsLayer, ok := layers["dns"].(map[string]any)
		if !ok {
			t.Fatalf("attempt[%d] missing dns layer", i)
		}

		obs, ok := dnsLayer["observations"].(map[string]any)
		if !ok {
			t.Fatalf("attempt[%d] missing dns observations", i)
		}

		resolverAddr, ok := obs["resolver_address"].(string)
		if !ok {
			t.Errorf("attempt[%d] resolver_address is not a string: %v", i, obs["resolver_address"])
			continue
		}
		if resolverAddr != "8.8.8.8:53" {
			t.Errorf("attempt[%d] resolver_address = %q, want \"8.8.8.8:53\"", i, resolverAddr)
		}
	}
}
