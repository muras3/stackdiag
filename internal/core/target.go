package core

import (
	"fmt"
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
	if raw == "" {
		return Target{}, fmt.Errorf("empty target")
	}

	// If no scheme, default to https.
	normalized := raw
	if !strings.Contains(normalized, "://") {
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

	path := u.Path
	if path == "" && (scheme == "https" || scheme == "http") {
		path = "/"
	}

	return Target{
		Original: raw,
		Scheme:   scheme,
		Host:     host,
		Port:     port,
		Path:     path,
	}, nil
}

func resolvePort(portStr, scheme string) (int, error) {
	if portStr != "" {
		return strconv.Atoi(portStr)
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
func (t Target) HostPort() string { return fmt.Sprintf("%s:%d", t.Host, t.Port) }
