package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func TestValidateJSONShapeReportsDeterministicFieldPaths(t *testing.T) {
	type item struct {
		Required string `json:"required"`
		Optional string `json:"optional,omitempty"`
	}
	type response struct {
		Name     string            `json:"name"`
		Items    []item            `json:"items"`
		Metadata map[string]string `json:"metadata"`
		Raw      json.RawMessage   `json:"raw"`
		Anything any               `json:"anything"`
	}

	document := decodeShapeTestDocument(t, `{
		"name":"example",
		"items":[{}, {"required":123}],
		"metadata":{"count":1},
		"raw":{"providerSpecific":true},
		"anything":["unconstrained"],
		"unknownFutureField":{"nested":true}
	}`)
	var target response
	result := validateJSONShape(document, &target)

	if !reflect.DeepEqual(result.missingFields, []string{"items[].required"}) {
		t.Fatalf("missing fields = %#v", result.missingFields)
	}
	wantMismatches := []schemaTypeMismatch{
		{Path: "items[].required", Expected: "string", Actual: "number"},
		{Path: "metadata.*", Expected: "string", Actual: "number"},
	}
	if !reflect.DeepEqual(result.typeMismatches, wantMismatches) {
		t.Fatalf("type mismatches = %#v, want %#v", result.typeMismatches, wantMismatches)
	}
}

func TestValidateJSONShapeChecksRootMapsAndArrayItems(t *testing.T) {
	var object map[string]any
	result := validateJSONShape(decodeShapeTestDocument(t, `[]`), &object)
	if !reflect.DeepEqual(result.typeMismatches, []schemaTypeMismatch{{Path: "$", Expected: "object", Actual: "array"}}) {
		t.Fatalf("root mismatch = %#v", result.typeMismatches)
	}

	type row struct {
		ID string `json:"id"`
	}
	var rows []row
	result = validateJSONShape(decodeShapeTestDocument(t, `[{"id":1}]`), &rows)
	if !reflect.DeepEqual(result.typeMismatches, []schemaTypeMismatch{{Path: "[].id", Expected: "string", Actual: "number"}}) {
		t.Fatalf("array mismatch = %#v", result.typeMismatches)
	}
}

func TestValidateJSONShapeRejectsNullForRootContainers(t *testing.T) {
	type response struct {
		Name string `json:"name"`
	}
	var object map[string]any
	var rows []response
	var structured response
	for _, test := range []struct {
		name     string
		target   any
		expected string
	}{
		{name: "object", target: &object, expected: "object"},
		{name: "array", target: &rows, expected: "array"},
		{name: "struct", target: &structured, expected: "object"},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := validateJSONShape(nil, test.target)
			want := []schemaTypeMismatch{{Path: "$", Expected: test.expected, Actual: "null"}}
			if !reflect.DeepEqual(result.typeMismatches, want) {
				t.Fatalf("null root mismatch = %#v, want %#v", result.typeMismatches, want)
			}
		})
	}
}

func TestValidateJSONShapeChecksPresentOmitEmptyFields(t *testing.T) {
	type response struct {
		Optional string `json:"optional,omitempty"`
	}
	var target response
	if result := validateJSONShape(decodeShapeTestDocument(t, `{}`), &target); !result.valid() {
		t.Fatalf("omitted optional field failed validation: %#v", result)
	}
	for _, test := range []struct {
		raw    string
		actual string
	}{
		{raw: `{"optional":null}`, actual: "null"},
		{raw: `{"optional":1}`, actual: "number"},
	} {
		result := validateJSONShape(decodeShapeTestDocument(t, test.raw), &target)
		want := []schemaTypeMismatch{{Path: "optional", Expected: "string", Actual: test.actual}}
		if !reflect.DeepEqual(result.typeMismatches, want) {
			t.Fatalf("present optional mismatch = %#v, want %#v", result.typeMismatches, want)
		}
	}
}

func TestValidateJSONShapeAcceptsAnyRawMessageShape(t *testing.T) {
	type response struct {
		Raw      json.RawMessage `json:"raw"`
		Anything any             `json:"anything"`
	}
	var target response
	values := []any{nil, "value", json.Number("1"), true, []any{"item"}, map[string]any{"field": "value"}}
	for _, raw := range values {
		for _, anything := range values {
			document := map[string]any{"raw": raw, "anything": anything}
			if result := validateJSONShape(document, &target); !result.valid() {
				t.Fatalf("RawMessage=%T any=%T failed validation: %#v", raw, anything, result)
			}
		}
	}
}

func TestGetJSONReturnsRedactedSchemaMismatchWithoutRawValues(t *testing.T) {
	type item struct {
		Count int `json:"count"`
	}
	type response struct {
		Name  string `json:"name"`
		Items []item `json:"items"`
	}
	client := &Client{HTTP: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		body := `{"items":[{"count":"raw-secret"}],"unknown":"do-not-reflect"}`
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}}

	target := response{Name: "unchanged", Items: []item{{Count: 7}}}
	_, err := client.GetJSON(
		context.Background(),
		"https://example.test/data?apiKey=query-secret&station=KSFO",
		map[string]string{"Authorization": "Bearer header-secret"},
		&target,
		false,
	)
	appErr, ok := err.(*Error)
	if !ok {
		t.Fatalf("error type = %T, want *Error", err)
	}
	if appErr.Code != "UPSTREAM_SCHEMA_MISMATCH" || appErr.ExitCode != 6 {
		t.Fatalf("schema mismatch error = %#v", appErr)
	}
	if appErr.Hint != "Affected JSON paths: items[].count, name" {
		t.Fatalf("schema mismatch hint = %q", appErr.Hint)
	}
	if !reflect.DeepEqual(target, response{Name: "unchanged", Items: []item{{Count: 7}}}) {
		t.Fatalf("schema mismatch mutated target: %#v", target)
	}
	if got := appErr.Details["missingFields"]; !reflect.DeepEqual(got, []string{"name"}) {
		t.Fatalf("missingFields = %#v", got)
	}
	wantMismatches := []schemaTypeMismatch{{Path: "items[].count", Expected: "integer", Actual: "string"}}
	if got := appErr.Details["typeMismatches"]; !reflect.DeepEqual(got, wantMismatches) {
		t.Fatalf("typeMismatches = %#v", got)
	}
	encoded, marshalErr := json.Marshal(appErr)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	text := string(encoded)
	for _, secret := range []string{"query-secret", "header-secret", "raw-secret", "do-not-reflect"} {
		if strings.Contains(text, secret) {
			t.Fatalf("schema mismatch reflected %q: %s", secret, text)
		}
	}
	if !strings.Contains(text, "apiKey=REDACTED") || !strings.Contains(text, "station=KSFO") {
		t.Fatalf("schema mismatch URL was not safely redacted: %s", text)
	}
}

func TestGetJSONAllowsUnknownFieldsRawMessageAnyAndOptionalOmission(t *testing.T) {
	type response struct {
		Name     string          `json:"name"`
		Optional string          `json:"optional,omitempty"`
		Raw      json.RawMessage `json:"raw"`
		Anything any             `json:"anything"`
	}
	client := &Client{HTTP: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		body := `{"name":"ok","raw":[1,{"nested":true}],"anything":{"value":42},"future":"preserved-upstream"}`
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}}

	var target response
	if _, err := client.GetJSON(context.Background(), "https://example.test/data", nil, &target, false); err != nil {
		t.Fatalf("flexible fields or unknown field failed validation: %v", err)
	}
	if target.Name != "ok" || !bytes.Equal(target.Raw, []byte(`[1,{"nested":true}]`)) {
		t.Fatalf("unexpected decoded target: %#v", target)
	}
}

func TestResolveLocationPreservesNoResultsSemantics(t *testing.T) {
	client := &Client{HTTP: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{}`)), Header: make(http.Header)}, nil
	})}}

	_, err := client.ResolveLocation(context.Background(), "no matching place")
	appErr, ok := err.(*Error)
	if !ok || appErr.Code != "not_found" || appErr.ExitCode != 3 {
		t.Fatalf("no-results error = %#v", err)
	}
}

func TestGetJSONKeepsMalformedJSONClassification(t *testing.T) {
	client := &Client{HTTP: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"name":`)), Header: make(http.Header)}, nil
	})}}
	var target struct {
		Name string `json:"name"`
	}
	_, err := client.GetJSON(context.Background(), "https://example.test/data", nil, &target, false)
	appErr, ok := err.(*Error)
	if !ok || appErr.Code != "invalid_provider_response" || appErr.ExitCode != 1 {
		t.Fatalf("malformed JSON error = %#v", err)
	}
}

func decodeShapeTestDocument(t *testing.T, raw string) any {
	t.Helper()
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	var document any
	if err := decoder.Decode(&document); err != nil {
		t.Fatal(err)
	}
	return document
}
