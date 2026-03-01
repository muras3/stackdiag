package core

// DNSObservations holds typed observations for the DNS layer.
// Fields are ordered alphabetically by JSON tag for byte-identical
// marshaling with the previous map[string]any approach.
type DNSObservations struct {
	Answers         []string `json:"answers,omitempty"`
	DNSErrorHint    *string  `json:"dns_error_hint"`
	QueryName       string   `json:"query_name"`
	ResolverAddress *string  `json:"resolver_address"`
	TTL             *int     `json:"ttl"`
}

// ReachabilityObservations holds typed observations for the reachability layer.
type ReachabilityObservations struct {
	ProbeMethod string   `json:"probe_method"`
	Reachable   *bool    `json:"reachable"`
	RTTMS       *float64 `json:"rtt_ms"`
	SkipReason  *string  `json:"skip_reason"`
}

// TCPObservations holds typed observations for the TCP layer.
type TCPObservations struct {
	RemoteIP   string `json:"remote_ip,omitempty"`
	RemotePort int    `json:"remote_port,omitempty"`
}

// CertChainEntry holds a single certificate in the TLS chain.
type CertChainEntry struct {
	Issuer   string `json:"issuer"`
	NotAfter string `json:"not_after"`
	Subject  string `json:"subject"`
}

// TLSObservations holds typed observations for the TLS layer.
type TLSObservations struct {
	CertChain           []CertChainEntry `json:"cert_chain,omitempty"`
	CertDaysUntilExpiry *int             `json:"cert_days_until_expiry,omitempty"`
	CertHostnameMatch   *bool            `json:"cert_hostname_match,omitempty"`
	CertIssuer          *string          `json:"cert_issuer,omitempty"`
	CertNotAfter        *string          `json:"cert_not_after,omitempty"`
	CertNotBefore       *string          `json:"cert_not_before,omitempty"`
	CertSAN             *[]string        `json:"cert_san,omitempty"`
	CertSubject         *string          `json:"cert_subject,omitempty"`
	CertVerified        *bool            `json:"cert_verified,omitempty"`
	CipherSuite         string           `json:"cipher_suite,omitempty"`
	TLSScan             map[string]any   `json:"tls_scan,omitempty"`
	Version             string           `json:"version,omitempty"`
}

// HTTPObservations holds typed observations for the HTTP layer.
type HTTPObservations struct {
	Method          string            `json:"method"`
	Protocol        string            `json:"protocol,omitempty"`
	RequestHeaders  map[string]string `json:"request_headers,omitempty"`
	ResponseHeaders map[string]string `json:"response_headers,omitempty"`
	StatusCode      int               `json:"status_code,omitempty"`
	StatusText      string            `json:"status_text,omitempty"`
}
