package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type driftScenario struct {
	name         string
	compatible   bool
	body         string
	target       func() any
	expectedPath string
}

type driftArmSummary struct {
	CompatibleSuccess int `json:"compatibleSuccess"`
	DetectedBreaks    int `json:"detectedBreaks"`
	SilentWrong       int `json:"silentWrong"`
	OtherFailure      int `json:"otherFailure"`
}

func TestSchemaDriftBenchmark(t *testing.T) {
	validSearch := `{"events":[{"id":"1","title":"Weather event","slug":"weather-event","active":true,"closed":false,"endDate":"2026-08-02T00:00:00Z","volume":"10","liquidity":"5","markets":[{"id":"2","question":"Will it rain?","slug":"rain","active":true,"closed":false,"volume":"1","volumeNum":1,"liquidity":"2","liquidityNum":2,"outcomes":"[\"Yes\",\"No\"]","outcomePrices":"[\"0.5\",\"0.5\"]","clobTokenIds":"[\"11\",\"12\"]"}]}]}`
	searchOptionalOmitted := strings.Replace(validSearch, `"endDate":"2026-08-02T00:00:00Z",`, "", 1)
	searchOptionalOmitted = strings.Replace(searchOptionalOmitted, `"volume":"10",`, "", 1)
	searchOptionalOmitted = strings.Replace(searchOptionalOmitted, `"liquidity":"5",`, "", 1)
	searchOptionalOmitted = strings.Replace(searchOptionalOmitted, `"volume":"1","volumeNum":1,`, "", 1)
	searchOptionalOmitted = strings.Replace(searchOptionalOmitted, `"liquidity":"2","liquidityNum":2,`, "", 1)
	searchOptionalOmitted = strings.Replace(searchOptionalOmitted, `"outcomePrices":"[\"0.5\",\"0.5\"]",`, "", 1)
	searchMissingQuestion := strings.Replace(validSearch, `"question":"Will it rain?",`, "", 1)
	searchNullQuestion := strings.Replace(validSearch, `"question":"Will it rain?"`, `"question":null`, 1)
	scenarios := []driftScenario{
		{name: "metar-valid", compatible: true, body: `[{"icaoId":"KSFO","reportTime":"2026-08-01T00:00:00Z"}]`, target: metarBenchmarkTarget},
		{name: "metar-additive", compatible: true, body: `[{"icaoId":"KSFO","future":{"nested":true}}]`, target: metarBenchmarkTarget},
		{name: "metar-empty", compatible: true, body: `[]`, target: metarBenchmarkTarget},
		{name: "metar-optional-omitted", compatible: true, body: `[{"icaoId":"KSFO"}]`, target: metarBenchmarkTarget},
		{name: "geocode-valid", compatible: true, body: `{"results":[{"name":"San Francisco","latitude":37.77,"longitude":-122.42}]}`, target: geocodeBenchmarkTarget},
		{name: "geocode-empty", compatible: true, body: `{}`, target: geocodeBenchmarkTarget},
		{name: "geocode-additive", compatible: true, body: `{"results":[{"name":"San Francisco","latitude":37.77,"longitude":-122.42,"future":true}],"generationtime_ms":1}`, target: geocodeBenchmarkTarget},
		{name: "search-valid", compatible: true, body: validSearch, target: searchBenchmarkTarget},
		{name: "search-additive", compatible: true, body: strings.Replace(validSearch, `"events":`, `"future":true,"events":`, 1), target: searchBenchmarkTarget},
		{name: "search-optional-omitted", compatible: true, body: searchOptionalOmitted, target: searchBenchmarkTarget},

		{name: "metar-missing-id", body: `[{"reportTime":"2026-08-01T00:00:00Z"}]`, target: metarBenchmarkTarget, expectedPath: "[].icaoId"},
		{name: "metar-null-id", body: `[{"icaoId":null}]`, target: metarBenchmarkTarget, expectedPath: "[].icaoId"},
		{name: "metar-wrong-id-type", body: `[{"icaoId":7}]`, target: metarBenchmarkTarget, expectedPath: "[].icaoId"},
		{name: "metar-later-item-missing-id", body: `[{"icaoId":"KSFO"},{}]`, target: metarBenchmarkTarget, expectedPath: "[].icaoId"},
		{name: "geocode-missing-name", body: `{"results":[{"latitude":37.77,"longitude":-122.42}]}`, target: geocodeBenchmarkTarget, expectedPath: "results[].name"},
		{name: "geocode-missing-latitude", body: `{"results":[{"name":"San Francisco","longitude":-122.42}]}`, target: geocodeBenchmarkTarget, expectedPath: "results[].latitude"},
		{name: "geocode-wrong-latitude-type", body: `{"results":[{"name":"San Francisco","latitude":"37.77","longitude":-122.42}]}`, target: geocodeBenchmarkTarget, expectedPath: "results[].latitude"},
		{name: "geocode-results-object", body: `{"results":{}}`, target: geocodeBenchmarkTarget, expectedPath: "results"},
		{name: "search-missing-events", body: `{}`, target: searchBenchmarkTarget, expectedPath: "events"},
		{name: "search-null-events", body: `{"events":null}`, target: searchBenchmarkTarget, expectedPath: "events"},
		{name: "search-event-missing-title", body: strings.Replace(validSearch, `"title":"Weather event",`, "", 1), target: searchBenchmarkTarget, expectedPath: "events[].title"},
		{name: "search-event-wrong-title-type", body: strings.Replace(validSearch, `"title":"Weather event"`, `"title":7`, 1), target: searchBenchmarkTarget, expectedPath: "events[].title"},
		{name: "search-market-missing-question", body: searchMissingQuestion, target: searchBenchmarkTarget, expectedPath: "events[].markets[].question"},
		{name: "search-market-null-question", body: searchNullQuestion, target: searchBenchmarkTarget, expectedPath: "events[].markets[].question"},
	}

	var direct, validated driftArmSummary
	compatibleCases, breakingCases := 0, 0
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			if scenario.compatible {
				compatibleCases++
			} else {
				breakingCases++
			}

			directErr := json.Unmarshal([]byte(scenario.body), scenario.target())
			if scenario.compatible {
				if directErr != nil {
					t.Fatalf("compatible direct decode failed: %v", directErr)
				}
				direct.CompatibleSuccess++
			} else if directErr == nil {
				direct.SilentWrong++
			} else {
				direct.DetectedBreaks++
			}

			client := benchmarkJSONClient(scenario.body)
			_, contractErr := client.GetJSON(context.Background(), "https://fixture.invalid/response", nil, scenario.target(), false)
			if scenario.compatible {
				if contractErr != nil {
					t.Fatalf("compatible validated decode failed: %v", contractErr)
				}
				validated.CompatibleSuccess++
				return
			}
			appErr, ok := contractErr.(*Error)
			if !ok || appErr.Code != "UPSTREAM_SCHEMA_MISMATCH" || !schemaErrorHasPath(appErr, scenario.expectedPath) {
				validated.OtherFailure++
				t.Fatalf("break was not localized at %s: %#v", scenario.expectedPath, contractErr)
			}
			validated.DetectedBreaks++
		})
	}
	if validated.CompatibleSuccess != compatibleCases || validated.DetectedBreaks != breakingCases || validated.SilentWrong != 0 || validated.OtherFailure != 0 {
		t.Fatalf("validated summary = %#v, compatible=%d breaking=%d", validated, compatibleCases, breakingCases)
	}
	summary := map[string]any{
		"compatibleCases": compatibleCases,
		"breakingCases":   breakingCases,
		"directAPIJSON":   direct,
		"weatherCLI":      validated,
	}
	raw, err := json.Marshal(summary)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("SCHEMA_DRIFT_BENCHMARK_JSON=%s", raw)
}

func metarBenchmarkTarget() any {
	value := []METARObservation{}
	return &value
}

func geocodeBenchmarkTarget() any {
	value := geocodeResponse{}
	return &value
}

func searchBenchmarkTarget() any {
	value := rawSearch{}
	return &value
}

func benchmarkJSONClient(body string) *Client {
	return &Client{HTTP: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}}
}

func schemaErrorHasPath(err *Error, path string) bool {
	if fields, ok := err.Details["missingFields"].([]string); ok {
		for _, field := range fields {
			if field == path {
				return true
			}
		}
	}
	if mismatches, ok := err.Details["typeMismatches"].([]schemaTypeMismatch); ok {
		for _, mismatch := range mismatches {
			if mismatch.Path == path {
				return true
			}
		}
	}
	return false
}
