package core

import (
	"encoding/json"
	"testing"
)

// ptr returns a pointer to the given value.
func ptr[T any](v T) *T { return &v }

// assertJSONEqual marshals old (map) and new (struct) and compares bytes.
func assertJSONEqual(t *testing.T, label string, old, new any) {
	t.Helper()
	oldJSON, err := json.Marshal(old)
	if err != nil {
		t.Fatalf("%s: marshal old: %v", label, err)
	}
	newJSON, err := json.Marshal(new)
	if err != nil {
		t.Fatalf("%s: marshal new: %v", label, err)
	}
	if string(oldJSON) != string(newJSON) {
		t.Errorf("%s: JSON mismatch\nold: %s\nnew: %s", label, oldJSON, newJSON)
	}
}

func TestDNSObservationsJSON_Success(t *testing.T) {
	resolver := "8.8.8.8:53"
	old := map[string]any{
		"query_name":       "example.com",
		"answers":          []string{"1.2.3.4"},
		"dns_error_hint":   nil,
		"ttl":              nil,
		"resolver_address": resolver,
	}
	new := DNSObservations{
		QueryName:       "example.com",
		Answers:         []string{"1.2.3.4"},
		DNSErrorHint:    nil,
		TTL:             nil,
		ResolverAddress: &resolver,
	}
	assertJSONEqual(t, "dns-success", old, new)
}

func TestDNSObservationsJSON_Error(t *testing.T) {
	old := map[string]any{
		"answers":          []any{},
		"query_name":       "bad.example.com",
		"dns_error_hint":   "servfail",
		"ttl":              nil,
		"resolver_address": nil,
	}
	new := DNSObservations{
		Answers:         []string{},
		QueryName:       "bad.example.com",
		DNSErrorHint:    ptr("servfail"),
		TTL:             nil,
		ResolverAddress: nil,
	}
	assertJSONEqual(t, "dns-error", old, new)
}

func TestDNSObservationsJSON_ErrorNilHint(t *testing.T) {
	old := map[string]any{
		"answers":          []any{},
		"query_name":       "fail.example.com",
		"dns_error_hint":   nil,
		"ttl":              nil,
		"resolver_address": nil,
	}
	new := DNSObservations{
		Answers:         []string{},
		QueryName:       "fail.example.com",
		DNSErrorHint:    nil,
		TTL:             nil,
		ResolverAddress: nil,
	}
	assertJSONEqual(t, "dns-error-nil-hint", old, new)
}

func TestReachabilityObservationsJSON_OK(t *testing.T) {
	rttMS := 1.234
	old := map[string]any{
		"probe_method": "icmp",
		"reachable":    true,
		"rtt_ms":       rttMS,
		"skip_reason":  nil,
	}
	new := ReachabilityObservations{
		ProbeMethod: "icmp",
		Reachable:   ptr(true),
		RTTMS:       &rttMS,
		SkipReason:  nil,
	}
	assertJSONEqual(t, "reach-ok", old, new)
}

func TestReachabilityObservationsJSON_Fail(t *testing.T) {
	old := map[string]any{
		"probe_method": "icmp",
		"reachable":    false,
		"rtt_ms":       nil,
		"skip_reason":  nil,
	}
	new := ReachabilityObservations{
		ProbeMethod: "icmp",
		Reachable:   ptr(false),
		RTTMS:       nil,
		SkipReason:  nil,
	}
	assertJSONEqual(t, "reach-fail", old, new)
}

func TestReachabilityObservationsJSON_Skip(t *testing.T) {
	old := map[string]any{
		"probe_method": "none",
		"reachable":    nil,
		"rtt_ms":       nil,
		"skip_reason":  "permission_denied",
	}
	new := ReachabilityObservations{
		ProbeMethod: "none",
		Reachable:   nil,
		RTTMS:       nil,
		SkipReason:  ptr("permission_denied"),
	}
	assertJSONEqual(t, "reach-skip", old, new)
}

func TestTCPObservationsJSON_OK(t *testing.T) {
	old := map[string]any{
		"remote_ip":   "203.0.113.10",
		"remote_port": 443,
	}
	new := TCPObservations{
		RemoteIP:   "203.0.113.10",
		RemotePort: 443,
	}
	assertJSONEqual(t, "tcp-ok", old, new)
}

func TestTCPObservationsJSON_Error(t *testing.T) {
	old := map[string]any{"remote_ip": "", "remote_port": 0}
	new := TCPObservations{}
	assertJSONEqual(t, "tcp-error", old, new)
}

func TestTLSObservationsJSON_Full(t *testing.T) {
	old := map[string]any{
		"cert_chain": []map[string]string{
			{"issuer": "CN=LE", "not_after": "2026-06-01T00:00:00Z", "subject": "CN=example.com"},
		},
		"cert_days_until_expiry": 90,
		"cert_hostname_match":    true,
		"cert_issuer":            "CN=LE",
		"cert_not_after":         "2026-06-01T00:00:00Z",
		"cert_not_before":        "2025-12-01T00:00:00Z",
		"cert_san":               []string{"example.com", "*.example.com"},
		"cert_subject":           "CN=example.com",
		"cert_verified":          true,
		"cipher_suite":           "TLS_AES_256_GCM_SHA384",
		"version":                "TLSv1.3",
	}
	new := TLSObservations{
		CertChain: []CertChainEntry{
			{Issuer: "CN=LE", NotAfter: "2026-06-01T00:00:00Z", Subject: "CN=example.com"},
		},
		CertDaysUntilExpiry: ptr(90),
		CertHostnameMatch:   ptr(true),
		CertIssuer:          ptr("CN=LE"),
		CertNotAfter:        ptr("2026-06-01T00:00:00Z"),
		CertNotBefore:       ptr("2025-12-01T00:00:00Z"),
		CertSAN:             &[]string{"example.com", "*.example.com"},
		CertSubject:         ptr("CN=example.com"),
		CertVerified:        ptr(true),
		CipherSuite:         "TLS_AES_256_GCM_SHA384",
		Version:             "TLSv1.3",
	}
	assertJSONEqual(t, "tls-full", old, new)
}

func TestTLSObservationsJSON_NoCerts(t *testing.T) {
	old := map[string]any{
		"cipher_suite": "TLS_AES_256_GCM_SHA384",
		"version":      "TLSv1.3",
	}
	new := TLSObservations{
		CipherSuite: "TLS_AES_256_GCM_SHA384",
		Version:     "TLSv1.3",
	}
	assertJSONEqual(t, "tls-no-certs", old, new)
}

func TestTLSObservationsJSON_Empty(t *testing.T) {
	old := map[string]any{}
	new := TLSObservations{}
	assertJSONEqual(t, "tls-empty", old, new)
}

func TestTLSObservationsJSON_EmptySAN(t *testing.T) {
	// Verify that empty (non-nil) cert_san produces [] not omitted.
	old := map[string]any{
		"cert_chain": []map[string]string{
			{"issuer": "CN=LE", "not_after": "2026-06-01T00:00:00Z", "subject": "CN=x"},
		},
		"cert_days_until_expiry": 90,
		"cert_hostname_match":    true,
		"cert_issuer":            "CN=LE",
		"cert_not_after":         "2026-06-01T00:00:00Z",
		"cert_not_before":        "2025-12-01T00:00:00Z",
		"cert_san":               []string{},
		"cert_subject":           "CN=x",
		"cert_verified":          false,
		"cipher_suite":           "TLS_AES_256_GCM_SHA384",
		"version":                "TLSv1.3",
	}
	new := TLSObservations{
		CertChain: []CertChainEntry{
			{Issuer: "CN=LE", NotAfter: "2026-06-01T00:00:00Z", Subject: "CN=x"},
		},
		CertDaysUntilExpiry: ptr(90),
		CertHostnameMatch:   ptr(true),
		CertIssuer:          ptr("CN=LE"),
		CertNotAfter:        ptr("2026-06-01T00:00:00Z"),
		CertNotBefore:       ptr("2025-12-01T00:00:00Z"),
		CertSAN:             &[]string{},
		CertSubject:         ptr("CN=x"),
		CertVerified:        ptr(false),
		CipherSuite:         "TLS_AES_256_GCM_SHA384",
		Version:             "TLSv1.3",
	}
	assertJSONEqual(t, "tls-empty-san", old, new)
}

func TestHTTPObservationsJSON_Full(t *testing.T) {
	old := map[string]any{
		"method":           "GET",
		"protocol":         "HTTP/2",
		"request_headers":  map[string]string{"Authorization": "[REDACTED]"},
		"response_headers": map[string]string{"Content-Type": "application/json"},
		"status_code":      200,
		"status_text":      "OK",
	}
	new := HTTPObservations{
		Method:          "GET",
		Protocol:        "HTTP/2",
		RequestHeaders:  map[string]string{"Authorization": "[REDACTED]"},
		ResponseHeaders: map[string]string{"Content-Type": "application/json"},
		StatusCode:      200,
		StatusText:      "OK",
	}
	assertJSONEqual(t, "http-full", old, new)
}

func TestHTTPObservationsJSON_MethodOnly(t *testing.T) {
	old := map[string]any{
		"method": "GET",
	}
	new := HTTPObservations{
		Method: "GET",
	}
	assertJSONEqual(t, "http-method-only", old, new)
}

func TestCertChainEntryJSON(t *testing.T) {
	old := map[string]string{
		"subject":   "CN=example.com",
		"issuer":    "CN=LE",
		"not_after": "2026-06-01T00:00:00Z",
	}
	new := CertChainEntry{
		Subject:  "CN=example.com",
		Issuer:   "CN=LE",
		NotAfter: "2026-06-01T00:00:00Z",
	}
	assertJSONEqual(t, "cert-chain-entry", old, new)
}
