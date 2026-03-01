package schema_test

import (
	"encoding/json"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/muras3/stackdiag/internal/core"
)

const schemaFile = "stackdiag-v0.1.schema.json"

// loadSchema reads and parses the JSON schema file, failing the test on error.
func loadSchema(t *testing.T) map[string]any {
	t.Helper()
	data, err := os.ReadFile(schemaFile)
	if err != nil {
		t.Fatalf("failed to read schema file: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("schema is not valid JSON: %v", err)
	}
	return parsed
}

// TestSchemaIsValidJSON verifies that the schema file is well-formed JSON.
func TestSchemaIsValidJSON(t *testing.T) {
	parsed := loadSchema(t)

	// Verify it's JSON Schema draft 2020-12
	schemaURI, ok := parsed["$schema"].(string)
	if !ok || schemaURI != "https://json-schema.org/draft/2020-12/schema" {
		t.Errorf("expected JSON Schema draft 2020-12, got %q", schemaURI)
	}
}

// TestSchemaDefinitionsExist checks that all expected type definitions are present.
func TestSchemaDefinitionsExist(t *testing.T) {
	parsed := loadSchema(t)

	defs, ok := parsed["$defs"].(map[string]any)
	if !ok {
		t.Fatal("schema has no $defs")
	}

	expectedDefs := []string{
		"Status",
		"ProbeError",
		"DNSObservations",
		"ReachabilityObservations",
		"TCPObservations",
		"TLSObservations",
		"HTTPObservations",
		"CertChainEntry",
		"TLSScan",
		"TLSScanAttempt",
		"LayerResult",
		"Layers",
		"Summary",
		"Result",
		"CountResult",
		"AttemptResult",
		"LayerStatistics",
		"Statistics",
		"ToolError",
	}

	for _, name := range expectedDefs {
		if _, ok := defs[name]; !ok {
			t.Errorf("missing definition: %s", name)
		}
	}
}

// TestSchemaFieldsMatchGoStructs verifies that schema property names match Go struct JSON tags.
func TestSchemaFieldsMatchGoStructs(t *testing.T) {
	parsed := loadSchema(t)
	defs := parsed["$defs"].(map[string]any)

	cases := []struct {
		defName    string
		goType     reflect.Type
		skipFields []string // fields not in Go struct (e.g., schema-only additions)
	}{
		{"Result", reflect.TypeOf(core.Result{}), nil},
		{"Summary", reflect.TypeOf(core.Summary{}), nil},
		{"LayerResult", reflect.TypeOf(core.LayerResult{}), nil},
		{"DNSObservations", reflect.TypeOf(core.DNSObservations{}), nil},
		{"ReachabilityObservations", reflect.TypeOf(core.ReachabilityObservations{}), nil},
		{"TCPObservations", reflect.TypeOf(core.TCPObservations{}), nil},
		{"TLSObservations", reflect.TypeOf(core.TLSObservations{}), nil},
		{"HTTPObservations", reflect.TypeOf(core.HTTPObservations{}), nil},
		{"CertChainEntry", reflect.TypeOf(core.CertChainEntry{}), nil},
		{"AttemptResult", reflect.TypeOf(core.AttemptResult{}), nil},
		{"CountResult", reflect.TypeOf(core.CountResult{}), nil},
		{"LayerStatistics", reflect.TypeOf(core.LayerStatistics{}), nil},
		{"ProbeError", reflect.TypeOf(core.ProbeError{}), nil},
	}

	for _, tc := range cases {
		t.Run(tc.defName, func(t *testing.T) {
			def, ok := defs[tc.defName].(map[string]any)
			if !ok {
				t.Fatalf("definition %s not found", tc.defName)
			}
			props, ok := def["properties"].(map[string]any)
			if !ok {
				t.Fatalf("definition %s has no properties", tc.defName)
			}

			// Build set of Go JSON field names
			goFields := jsonFieldNames(tc.goType)

			// Build skip set
			skipSet := make(map[string]bool)
			for _, f := range tc.skipFields {
				skipSet[f] = true
			}

			// Every Go struct field should be in schema
			for _, gf := range goFields {
				if skipSet[gf] {
					continue
				}
				if _, ok := props[gf]; !ok {
					t.Errorf("Go field %q (from %s) missing in schema definition %s", gf, tc.goType.Name(), tc.defName)
				}
			}

			// Every schema property should be in Go struct (unless skipped)
			for sp := range props {
				if skipSet[sp] {
					continue
				}
				found := false
				for _, gf := range goFields {
					if gf == sp {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("schema property %q in %s not found in Go struct %s", sp, tc.defName, tc.goType.Name())
				}
			}
		})
	}
}

// TestSchemaStatusEnum verifies the status enum matches Go constants.
func TestSchemaStatusEnum(t *testing.T) {
	parsed := loadSchema(t)
	defs := parsed["$defs"].(map[string]any)
	statusDef := defs["Status"].(map[string]any)
	enumRaw := statusDef["enum"].([]any)

	var schemaStatuses []string
	for _, v := range enumRaw {
		schemaStatuses = append(schemaStatuses, v.(string))
	}
	sort.Strings(schemaStatuses)

	goStatuses := []string{
		string(core.StatusOK),
		string(core.StatusWarn),
		string(core.StatusFail),
		string(core.StatusSkip),
	}
	sort.Strings(goStatuses)

	if !reflect.DeepEqual(schemaStatuses, goStatuses) {
		t.Errorf("status enum mismatch: schema=%v, go=%v", schemaStatuses, goStatuses)
	}
}

// TestSchemaLayerNames verifies the layers object requires exactly the 5 layer names.
func TestSchemaLayerNames(t *testing.T) {
	parsed := loadSchema(t)
	defs := parsed["$defs"].(map[string]any)
	layersDef := defs["Layers"].(map[string]any)
	props := layersDef["properties"].(map[string]any)
	required := layersDef["required"].([]any)

	var schemaLayers []string
	for k := range props {
		schemaLayers = append(schemaLayers, k)
	}
	sort.Strings(schemaLayers)

	goLayers := make([]string, len(core.LayerOrder))
	copy(goLayers, core.LayerOrder)
	sort.Strings(goLayers)

	if !reflect.DeepEqual(schemaLayers, goLayers) {
		t.Errorf("layer property mismatch: schema=%v, go=%v", schemaLayers, goLayers)
	}

	var requiredLayers []string
	for _, v := range required {
		requiredLayers = append(requiredLayers, v.(string))
	}
	sort.Strings(requiredLayers)

	if !reflect.DeepEqual(requiredLayers, goLayers) {
		t.Errorf("required layer mismatch: schema=%v, go=%v", requiredLayers, goLayers)
	}
}

// TestSchemaExitCodes verifies the exit_code enum in Summary matches documented values.
func TestSchemaExitCodes(t *testing.T) {
	parsed := loadSchema(t)
	defs := parsed["$defs"].(map[string]any)
	summaryDef := defs["Summary"].(map[string]any)
	exitCodeProp := summaryDef["properties"].(map[string]any)["exit_code"].(map[string]any)
	enumRaw := exitCodeProp["enum"].([]any)

	var codes []int
	for _, v := range enumRaw {
		codes = append(codes, int(v.(float64)))
	}
	sort.Ints(codes)

	expected := []int{0, 1, 2, 10, 15, 20, 30, 40}
	if !reflect.DeepEqual(codes, expected) {
		t.Errorf("exit_code enum mismatch: schema=%v, expected=%v", codes, expected)
	}
}

// TestResultMarshalHasRequiredFields constructs a valid Result, marshals it,
// and verifies the required top-level fields are present.
func TestResultMarshalHasRequiredFields(t *testing.T) {
	result := &core.Result{
		SchemaVersion: core.SchemaVersion,
		StartedAt:     time.Now().UTC(),
		Target:        "https://example.com",
		Layers: map[string]*core.LayerResult{
			"dns":          {Status: core.StatusOK, DurationMS: 5.0, Observations: core.DNSObservations{QueryName: "example.com", Answers: []string{"93.184.216.34"}}, Error: nil},
			"reachability": {Status: core.StatusSkip, DurationMS: 0, Observations: struct{}{}, Error: nil},
			"tcp":          {Status: core.StatusOK, DurationMS: 10.0, Observations: core.TCPObservations{RemoteIP: "93.184.216.34", RemotePort: 443}, Error: nil},
			"tls":          {Status: core.StatusOK, DurationMS: 20.0, Observations: core.TLSObservations{Version: "TLSv1.3", CipherSuite: "TLS_AES_256_GCM_SHA384"}, Error: nil},
			"http":         {Status: core.StatusOK, DurationMS: 30.0, Observations: core.HTTPObservations{Method: "GET", Protocol: "HTTP/2", StatusCode: 200, StatusText: "OK"}, Error: nil},
		},
		Summary: &core.Summary{
			WallClockMS:     65.0,
			FirstNonOKLayer: "",
			ExitCode:        0,
		},
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("failed to marshal Result: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal Result: %v", err)
	}

	requiredFields := []string{"schema_version", "started_at", "target", "layers", "summary"}
	for _, f := range requiredFields {
		if _, ok := parsed[f]; !ok {
			t.Errorf("missing required field: %s", f)
		}
	}

	// Verify all 5 layers present
	layers := parsed["layers"].(map[string]any)
	for _, name := range core.LayerOrder {
		if _, ok := layers[name]; !ok {
			t.Errorf("missing layer: %s", name)
		}
	}

	// Verify schema_version value
	if v := parsed["schema_version"].(string); v != "v0.1" {
		t.Errorf("schema_version = %q, want %q", v, "v0.1")
	}
}

// TestCountResultMarshalHasRequiredFields constructs a CountResult and verifies fields.
func TestCountResultMarshalHasRequiredFields(t *testing.T) {
	cr := &core.CountResult{
		SchemaVersion: core.SchemaVersion,
		Target:        "https://example.com",
		Count:         1,
		ExitCode:      0,
		Attempts: []*core.AttemptResult{
			{
				Attempt:   1,
				StartedAt: time.Now().UTC(),
				Layers: map[string]*core.LayerResult{
					"dns":          {Status: core.StatusOK, DurationMS: 5.0, Observations: core.DNSObservations{QueryName: "example.com", Answers: []string{"93.184.216.34"}}, Error: nil},
					"reachability": {Status: core.StatusSkip, DurationMS: 0, Observations: struct{}{}, Error: nil},
					"tcp":          {Status: core.StatusOK, DurationMS: 10.0, Observations: core.TCPObservations{RemoteIP: "93.184.216.34", RemotePort: 443}, Error: nil},
					"tls":          {Status: core.StatusOK, DurationMS: 20.0, Observations: core.TLSObservations{Version: "TLSv1.3", CipherSuite: "TLS_AES_256_GCM_SHA384"}, Error: nil},
					"http":         {Status: core.StatusOK, DurationMS: 30.0, Observations: core.HTTPObservations{Method: "GET", Protocol: "HTTP/2", StatusCode: 200, StatusText: "OK"}, Error: nil},
				},
				Summary: &core.Summary{WallClockMS: 65.0, FirstNonOKLayer: "", ExitCode: 0},
			},
		},
		Statistics: map[string]*core.LayerStatistics{
			"dns":          {SuccessCount: 1, SampleCount: 1},
			"reachability": {SkipCount: 1, SampleCount: 1},
			"tcp":          {SuccessCount: 1, SampleCount: 1},
			"tls":          {SuccessCount: 1, SampleCount: 1},
			"http":         {SuccessCount: 1, SampleCount: 1},
		},
	}

	data, err := json.Marshal(cr)
	if err != nil {
		t.Fatalf("failed to marshal CountResult: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal CountResult: %v", err)
	}

	requiredFields := []string{"schema_version", "target", "count", "exit_code", "attempts", "statistics"}
	for _, f := range requiredFields {
		if _, ok := parsed[f]; !ok {
			t.Errorf("missing required field: %s", f)
		}
	}
}

// TestSchemaErrorCodesComplete verifies all documented error codes are in the schema enum.
func TestSchemaErrorCodesComplete(t *testing.T) {
	parsed := loadSchema(t)
	defs := parsed["$defs"].(map[string]any)
	errorDef := defs["ProbeError"].(map[string]any)
	codeProp := errorDef["properties"].(map[string]any)["code"].(map[string]any)
	enumRaw := codeProp["enum"].([]any)

	codeSet := make(map[string]bool)
	for _, v := range enumRaw {
		codeSet[v.(string)] = true
	}

	// All documented error codes from schema.md
	documentedCodes := []string{
		// DNS
		"DNS_NXDOMAIN", "DNS_TIMEOUT", "DNS_ERROR",
		// Reachability
		"REACHABILITY_TIMEOUT", "REACHABILITY_ERROR",
		// TCP
		"TCP_TIMEOUT", "TCP_REFUSED", "TCP_HOST_UNREACHABLE", "TCP_NETWORK_UNREACHABLE", "TCP_ERROR",
		// TLS
		"TLS_CERT_EXPIRED", "TLS_CERT_NOT_YET_VALID", "TLS_CERT_EXPIRING_SOON",
		"TLS_HOSTNAME_MISMATCH", "TLS_UNTRUSTED_CHAIN", "TLS_NO_CERTIFICATES",
		"TLS_HANDSHAKE_TIMEOUT", "TLS_PROTOCOL_ERROR", "TLS_DEPRECATED_VERSION_ENABLED", "TLS_ERROR",
		// HTTP
		"HTTP_401", "HTTP_403", "HTTP_404", "HTTP_429",
		"HTTP_500", "HTTP_502", "HTTP_503", "HTTP_504", "HTTP_5XX",
		"HTTP_TIMEOUT", "HTTP_ERROR",
		// Tool
		"INVALID_TARGET", "INVALID_ARGS",
	}

	for _, code := range documentedCodes {
		if !codeSet[code] {
			t.Errorf("documented error code %q missing from schema enum", code)
		}
	}
}

// jsonFieldNames extracts JSON field names from a Go struct type.
func jsonFieldNames(t reflect.Type) []string {
	var names []string
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		tag := field.Tag.Get("json")
		if tag == "" || tag == "-" {
			continue
		}
		name := strings.Split(tag, ",")[0]
		names = append(names, name)
	}
	return names
}
