package core

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// Target represents a parsed probe target.
type Target struct {
	Original string
	Scheme   string
	Host     string
	Port     int
	Path     string
}

// ParseTarget parses a raw target string into a Target.
// Supported schemes: https, http, tcp. Bare hostnames default to https.
func ParseTarget(raw string) (Target, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Target{}, fmt.Errorf("empty target")
	}

	// If no scheme, default to https.
	hadScheme := strings.Contains(raw, "://")
	normalized := raw
	if !hadScheme {
		normalized = "https://" + normalized
	}

	u, err := url.Parse(normalized)
	if err != nil {
		return Target{}, fmt.Errorf("invalid target: %w", err)
	}

	scheme := strings.ToLower(u.Scheme)
	switch scheme {
	case "https", "http", "tcp":
		// valid
	default:
		return Target{}, fmt.Errorf("unsupported scheme: %q", scheme)
	}

	host := u.Hostname()
	if host == "" {
		return Target{}, fmt.Errorf("missing host in target")
	}

	port, err := resolvePort(u.Port(), scheme)
	if err != nil {
		return Target{}, err
	}

	// tcp:// targets must not have path, query, or fragment.
	if scheme == "tcp" && (u.Path != "" || u.RawQuery != "" || u.Fragment != "") {
		return Target{}, fmt.Errorf("tcp target must be tcp://host:port")
	}

	path := u.Path
	if u.RawQuery != "" {
		path = u.Path + "?" + u.RawQuery
	}
	if path == "" && (scheme == "https" || scheme == "http") {
		path = "/"
	}

	original := raw
	if u.User != nil {
		sanitized := *u
		sanitized.User = nil
		original = sanitized.String()
		if !hadScheme {
			original = strings.TrimPrefix(original, "https://")
		}
	}

	return Target{
		Original: original,
		Scheme:   scheme,
		Host:     host,
		Port:     port,
		Path:     path,
	}, nil
}

func resolvePort(portStr, scheme string) (int, error) {
	if portStr != "" {
		port, err := strconv.Atoi(portStr)
		if err != nil {
			return 0, err
		}
		if port < 1 || port > 65535 {
			return 0, fmt.Errorf("port out of range: %d", port)
		}
		return port, nil
	}
	switch scheme {
	case "https":
		return 443, nil
	case "http":
		return 80, nil
	case "tcp":
		return 0, fmt.Errorf("tcp scheme requires an explicit port")
	}
	return 0, fmt.Errorf("cannot determine default port for scheme %q", scheme)
}

// NeedsTLS returns true if the target requires a TLS handshake.
func (t Target) NeedsTLS() bool { return t.Scheme == "https" }

// NeedsHTTP returns true if the target requires an HTTP request.
func (t Target) NeedsHTTP() bool { return t.Scheme == "https" || t.Scheme == "http" }

// HostPort returns "host:port".
func (t Target) HostPort() string { return net.JoinHostPort(t.Host, strconv.Itoa(t.Port)) }
