package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func TestGetOrderBookValidatesAndPreservesSupportedNumbers(t *testing.T) {
	body := `{
		"market":"weather-market",
		"bids":[
			{"price":"0.25","size":10,"future":"preserved"},
			{"price":0,"size":"0"}
		],
		"asks":[{"price":1,"size":"2.5"}],
		"future":{"nested":true}
	}`
	result, err := polymarketOrderBookFixtureClient(body).GetOrderBook(context.Background(), "123")
	if err != nil {
		t.Fatal(err)
	}
	bids, ok := result.Book["bids"].([]any)
	if !ok || len(bids) != 2 {
		t.Fatalf("bids were not preserved: %#v", result.Book["bids"])
	}
	first, ok := bids[0].(map[string]any)
	if !ok || first["price"] != "0.25" || first["future"] != "preserved" {
		t.Fatalf("level encodings or unknown fields were not preserved: %#v", first)
	}
	if number, ok := first["size"].(json.Number); !ok || number != "10" {
		t.Fatalf("JSON number was not preserved exactly: %#v", first["size"])
	}
	if _, ok := result.Book["future"].(map[string]any); !ok {
		t.Fatalf("unknown root field was not preserved: %#v", result.Book)
	}
}

func TestGetOrderBookRejectsMalformedLevels(t *testing.T) {
	tests := []struct {
		name string
		body string
		path string
	}{
		{name: "missing bids", body: `{"asks":[]}`, path: "book.bids"},
		{name: "missing asks", body: `{"bids":[]}`, path: "book.asks"},
		{name: "null bids", body: `{"bids":null,"asks":[]}`, path: "book.bids"},
		{name: "object asks", body: `{"bids":[],"asks":{}}`, path: "book.asks"},
		{name: "scalar level", body: `{"bids":["raw-level-secret"],"asks":[]}`, path: "book.bids[]"},
		{name: "missing price", body: `{"bids":[{"size":"1"}],"asks":[]}`, path: "book.bids[].price"},
		{name: "missing size", body: `{"bids":[],"asks":[{"price":"0.5"}]}`, path: "book.asks[].size"},
		{name: "null price", body: `{"bids":[{"price":null,"size":"1"}],"asks":[]}`, path: "book.bids[].price"},
		{name: "boolean size", body: `{"bids":[],"asks":[{"price":"0.5","size":true}]}`, path: "book.asks[].size"},
		{name: "malformed price string", body: `{"bids":[{"price":"raw-price-secret","size":"1"}],"asks":[]}`, path: "book.bids[].price"},
		{name: "non-finite price", body: `{"bids":[{"price":"NaN","size":"1"}],"asks":[]}`, path: "book.bids[].price"},
		{name: "negative price", body: `{"bids":[{"price":-0.1,"size":"1"}],"asks":[]}`, path: "book.bids[].price"},
		{name: "price above one", body: `{"bids":[{"price":"1.5","size":"1"}],"asks":[]}`, path: "book.bids[].price"},
		{name: "overflow price", body: `{"bids":[{"price":1e9999,"size":"1"}],"asks":[]}`, path: "book.bids[].price"},
		{name: "malformed size string", body: `{"bids":[],"asks":[{"price":"0.5","size":"raw-size-secret"}]}`, path: "book.asks[].size"},
		{name: "negative size", body: `{"bids":[],"asks":[{"price":"0.5","size":-1}]}`, path: "book.asks[].size"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := polymarketOrderBookFixtureClient(test.body).GetOrderBook(context.Background(), "123")
			appErr, ok := err.(*Error)
			if !ok || appErr.Code != "UPSTREAM_SCHEMA_MISMATCH" || appErr.ExitCode != 6 {
				t.Fatalf("schema mismatch error = %#v", err)
			}
			if !schemaErrorHasPath(appErr, test.path) {
				t.Fatalf("schema mismatch was not localized at %q: %#v", test.path, appErr.Details)
			}
			encoded, marshalErr := json.Marshal(appErr)
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			for _, rawValue := range []string{"raw-level-secret", "raw-price-secret", "raw-size-secret"} {
				if strings.Contains(string(encoded), rawValue) {
					t.Fatalf("raw provider value %q leaked: %s", rawValue, encoded)
				}
			}
		})
	}
}

func TestGetOrderBookReportsSortedDeduplicatedPaths(t *testing.T) {
	body := `{
		"bids":[
			{"price":"raw-price-secret"},
			{"price":"raw-price-secret"}
		],
		"asks":[{"size":null}]
	}`
	_, err := polymarketOrderBookFixtureClient(body).GetOrderBook(context.Background(), "123")
	appErr, ok := err.(*Error)
	if !ok {
		t.Fatalf("error type = %T", err)
	}
	if got, want := appErr.Details["missingFields"], []string{"book.asks[].price", "book.bids[].size"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("missingFields = %#v, want %#v", got, want)
	}
	wantMismatches := []schemaTypeMismatch{
		{Path: "book.asks[].size", Expected: "non-negative finite number or numeric string", Actual: "null"},
		{Path: "book.bids[].price", Expected: "non-negative finite number or numeric string", Actual: "string"},
	}
	if got := appErr.Details["typeMismatches"]; !reflect.DeepEqual(got, wantMismatches) {
		t.Fatalf("typeMismatches = %#v, want %#v", got, wantMismatches)
	}
}

func TestGetOrderBookValidatesLevelsBeforeTruncation(t *testing.T) {
	levels := make([]string, MaximumOrderBookLevels+1)
	for index := range levels {
		levels[index] = `{"price":"0.5","size":"1"}`
	}
	levels[len(levels)-1] = `{"price":"raw-price-secret","size":"1"}`
	body := fmt.Sprintf(`{"bids":[%s],"asks":[]}`, strings.Join(levels, ","))

	_, err := polymarketOrderBookFixtureClient(body).GetOrderBook(context.Background(), "123")
	appErr, ok := err.(*Error)
	if !ok || appErr.Code != "UPSTREAM_SCHEMA_MISMATCH" || !schemaErrorHasPath(appErr, "book.bids[].price") {
		t.Fatalf("invalid truncated level was not rejected: %#v", err)
	}
}

func polymarketOrderBookFixtureClient(body string) *Client {
	return &Client{HTTP: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}}
}
