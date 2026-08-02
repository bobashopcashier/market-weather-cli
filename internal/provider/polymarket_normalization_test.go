package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func TestSearchMarketsPreservesSupportedPolymarketEncodings(t *testing.T) {
	client := polymarketFixtureClient(validPolymarketSearchBody())
	result, err := client.SearchMarkets(context.Background(), "weather", 5, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Events) != 1 || len(result.Events[0].Markets) != 2 {
		t.Fatalf("unexpected normalized result: %#v", result)
	}
	event := result.Events[0]
	if event.ID != "900719925474099312345" || !equalOptionalNumber(event.Volume, 12.5) || !equalOptionalNumber(event.Liquidity, 4) {
		t.Fatalf("event unions were not preserved: %#v", event)
	}
	first := event.Markets[0]
	if first.ID != "market-1" || !equalOptionalNumber(first.Volume, 1.25) || !equalOptionalNumber(first.Liquidity, 3) || len(first.Outcomes) != 2 {
		t.Fatalf("first market was not normalized: %#v", first)
	}
	if first.Outcomes[0].Price == nil || *first.Outcomes[0].Price != 0.25 || first.Outcomes[0].TokenID != "101" {
		t.Fatalf("encoded/direct list values were not aligned: %#v", first.Outcomes)
	}
	second := event.Markets[1]
	if second.ID != "900719925474099312346" || !equalOptionalNumber(second.Volume, 0) || !equalOptionalNumber(second.Liquidity, 2) || len(second.Outcomes) != 2 {
		t.Fatalf("numeric ID or numeric aliases were not preserved: %#v", second)
	}
	if second.Outcomes[0].Price == nil || *second.Outcomes[0].Price != 0 || second.Outcomes[0].TokenID != "" {
		t.Fatalf("zero price or optional token list was not preserved: %#v", second.Outcomes)
	}
}

func TestSearchMarketsAllowsDocumentedOptionalFields(t *testing.T) {
	body := strings.Replace(validPolymarketSearchBody(), `"endDate":"2026-08-02T00:00:00Z",`, "", 1)
	body = strings.Replace(body, `"volume":"12.5",`, "", 1)
	body = strings.Replace(body, `"liquidity":4,`, "", 1)
	body = strings.Replace(body, `"volume":"1.25",`, "", 1)
	body = strings.Replace(body, `"liquidity":"2.5","liquidityNum":3,`, "", 1)
	body = strings.Replace(body, `"outcomePrices":["0.25","0.75"],`, "", 1)
	result, err := polymarketFixtureClient(body).SearchMarkets(context.Background(), "weather", 5, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Events) != 1 || result.Events[0].EndDate != "" || result.Events[0].Volume != nil || result.Events[0].Liquidity != nil || len(result.Events[0].Markets) != 2 || result.Events[0].Markets[0].Volume != nil || result.Events[0].Markets[0].Liquidity != nil || result.Events[0].Markets[0].Outcomes[0].Price != nil {
		t.Fatalf("optional fields were not preserved as absent: %#v", result.Events)
	}
}

func TestSearchMarketsAllowsArchivedOptionalFieldsWhenClosed(t *testing.T) {
	body := strings.Replace(validPolymarketSearchBody(), `"closed":false,"endDate"`, `"closed":true,"endDate"`, 1)
	body = strings.Replace(body, `"volume":"12.5",`, "", 1)
	body = strings.Replace(body, `"volume":"1.25",`, "", 1)
	body = strings.Replace(body, `"outcomePrices":["0.25","0.75"],`, "", 1)
	result, err := polymarketFixtureClient(body).SearchMarkets(context.Background(), "weather", 5, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Events) != 1 || result.Events[0].Volume != nil || len(result.Events[0].Markets) != 2 || result.Events[0].Markets[0].Volume != nil || result.Events[0].Markets[0].Outcomes[0].Price != nil {
		t.Fatalf("archived optional fields were not preserved as absent: %#v", result.Events)
	}
}

func TestSearchMarketsDoesNotSemanticallyValidateFilteredClosedMarkets(t *testing.T) {
	body := strings.Replace(validPolymarketSearchBody(), `"id":"market-1","question":"Will it rain?","slug":"rain","active":true,"closed":false`, `"id":"market-1","question":"Will it rain?","slug":"rain","active":true,"closed":true`, 1)
	body = strings.Replace(body, `"liquidity":"2.5","liquidityNum":3,`, "", 1)
	body = strings.Replace(body, `"clobTokenIds":"[\"101\",\"102\"]"`, `"clobTokenIds":["not-decimal","still-not-decimal"]`, 1)
	result, err := polymarketFixtureClient(body).SearchMarkets(context.Background(), "weather", 5, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Events) != 1 || len(result.Events[0].Markets) != 1 || result.Events[0].Markets[0].ID != "900719925474099312346" {
		t.Fatalf("closed market was not filtered before semantic normalization: %#v", result.Events)
	}
}

func TestSearchMarketsRejectsSilentPolymarketNormalization(t *testing.T) {
	valid := validPolymarketSearchBody()
	tests := []struct {
		name string
		body string
		path string
	}{
		{name: "missing event id", body: strings.Replace(valid, `"id":900719925474099312345,`, "", 1), path: "events[].id"},
		{name: "null event id", body: strings.Replace(valid, `"id":900719925474099312345`, `"id":null`, 1), path: "events[].id"},
		{name: "empty event id", body: strings.Replace(valid, `"id":900719925474099312345`, `"id":"  "`, 1), path: "events[].id"},
		{name: "fractional event id", body: strings.Replace(valid, `"id":900719925474099312345`, `"id":1.5`, 1), path: "events[].id"},
		{name: "missing market id", body: strings.Replace(valid, `"id":"market-1",`, "", 1), path: "events[].markets[].id"},
		{name: "object market id", body: strings.Replace(valid, `"id":"market-1"`, `"id":{"secret":"raw-id"}`, 1), path: "events[].markets[].id"},
		{name: "malformed event volume", body: strings.Replace(valid, `"volume":"12.5"`, `"volume":"raw-volume-secret"`, 1), path: "events[].volume"},
		{name: "negative event liquidity", body: strings.Replace(valid, `"liquidity":4`, `"liquidity":-4`, 1), path: "events[].liquidity"},
		{name: "underflow event volume", body: strings.Replace(valid, `"volume":"12.5"`, `"volume":"1e-1000"`, 1), path: "events[].volume"},
		{name: "malformed base volume despite alias", body: strings.Replace(valid, `"volume":"1.25",`, `"volume":"raw-volume-secret","volumeNum":1.25,`, 1), path: "events[].markets[].volume"},
		{name: "null numeric alias", body: strings.Replace(valid, `"liquidityNum":3`, `"liquidityNum":null`, 1), path: "events[].markets[].liquidityNum"},
		{name: "malformed encoded outcomes", body: strings.Replace(valid, `"outcomes":"[\"Yes\",\"No\"]"`, `"outcomes":"raw-list-secret"`, 1), path: "events[].markets[].outcomes"},
		{name: "mixed direct outcomes", body: strings.Replace(valid, `"outcomes":["Up","Down"]`, `"outcomes":["Up",7]`, 1), path: "events[].markets[].outcomes[]"},
		{name: "empty outcomes", body: strings.Replace(valid, `"outcomes":["Up","Down"],"outcomePrices":"[\"0\",\"1\"]"`, `"outcomes":[],"outcomePrices":"[]"`, 1), path: "events[].markets[].outcomes"},
		{name: "non-finite price", body: strings.Replace(valid, `"outcomePrices":["0.25","0.75"]`, `"outcomePrices":["NaN","0.75"]`, 1), path: "events[].markets[].outcomePrices[]"},
		{name: "out-of-range price", body: strings.Replace(valid, `"outcomePrices":["0.25","0.75"]`, `"outcomePrices":["1.1","-0.1"]`, 1), path: "events[].markets[].outcomePrices[]"},
		{name: "underflow price", body: strings.Replace(valid, `"outcomePrices":["0.25","0.75"]`, `"outcomePrices":["1e-1000","0.75"]`, 1), path: "events[].markets[].outcomePrices[]"},
		{name: "short prices", body: strings.Replace(valid, `"outcomePrices":["0.25","0.75"]`, `"outcomePrices":["0.25"]`, 1), path: "events[].markets[].outcomePrices"},
		{name: "long prices", body: strings.Replace(valid, `"outcomePrices":["0.25","0.75"]`, `"outcomePrices":["0.25","0.5","0.25"]`, 1), path: "events[].markets[].outcomePrices"},
		{name: "malformed encoded tokens", body: strings.Replace(valid, `"clobTokenIds":"[\"101\",\"102\"]"`, `"clobTokenIds":"raw-token-secret"`, 1), path: "events[].markets[].clobTokenIds"},
		{name: "blank token", body: strings.Replace(valid, `"clobTokenIds":"[\"101\",\"102\"]"`, `"clobTokenIds":["101",""]`, 1), path: "events[].markets[].clobTokenIds[]"},
		{name: "non-decimal token", body: strings.Replace(valid, `"clobTokenIds":"[\"101\",\"102\"]"`, `"clobTokenIds":["abc","102"]`, 1), path: "events[].markets[].clobTokenIds[]"},
		{name: "oversized token", body: strings.Replace(valid, `"clobTokenIds":"[\"101\",\"102\"]"`, `"clobTokenIds":["`+strings.Repeat("1", 129)+`","102"]`, 1), path: "events[].markets[].clobTokenIds[]"},
		{name: "short tokens", body: strings.Replace(valid, `"clobTokenIds":"[\"101\",\"102\"]"`, `"clobTokenIds":["101"]`, 1), path: "events[].markets[].clobTokenIds"},
		{name: "long tokens", body: strings.Replace(valid, `"clobTokenIds":"[\"101\",\"102\"]"`, `"clobTokenIds":["101","102","103"]`, 1), path: "events[].markets[].clobTokenIds"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := polymarketFixtureClient(test.body).SearchMarkets(context.Background(), "weather", 5, false)
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
			for _, rawValue := range []string{"raw-id", "raw-volume-secret", "raw-list-secret", "raw-token-secret"} {
				if strings.Contains(string(encoded), rawValue) {
					t.Fatalf("raw provider value %q leaked: %s", rawValue, encoded)
				}
			}
		})
	}
}

func equalOptionalNumber(value *float64, expected float64) bool {
	return value != nil && *value == expected
}

func TestPolymarketNormalizationIssuesAreSortedAndDeduplicated(t *testing.T) {
	body := strings.Replace(validPolymarketSearchBody(), `"id":900719925474099312345`, `"id":null`, 1)
	body = strings.Replace(body, `"volume":"12.5"`, `"volume":"raw-volume-secret"`, 1)
	body = strings.Replace(body, `"liquidityNum":3`, `"liquidityNum":null`, 1)
	body = strings.Replace(body, `"id":900719925474099312346`, `"id":false`, 1)

	_, err := polymarketFixtureClient(body).SearchMarkets(context.Background(), "weather", 5, false)
	appErr, ok := err.(*Error)
	if !ok {
		t.Fatalf("error type = %T", err)
	}
	want := []schemaTypeMismatch{
		{Path: "events[].id", Expected: "non-empty string or non-negative integer", Actual: "null"},
		{Path: "events[].markets[].id", Expected: "non-empty string or non-negative integer", Actual: "boolean"},
		{Path: "events[].markets[].liquidityNum", Expected: "non-negative finite number or numeric string", Actual: "null"},
		{Path: "events[].volume", Expected: "non-negative finite number or numeric string", Actual: "string"},
	}
	if got := appErr.Details["typeMismatches"]; !reflect.DeepEqual(got, want) {
		t.Fatalf("typeMismatches = %#v, want %#v", got, want)
	}
	if got := appErr.Details["missingFields"]; !reflect.DeepEqual(got, []string{}) {
		t.Fatalf("missingFields = %#v", got)
	}
}

func validPolymarketSearchBody() string {
	return `{"events":[{"id":900719925474099312345,"title":"Weather event","slug":"weather-event","active":true,"closed":false,"endDate":"2026-08-02T00:00:00Z","volume":"12.5","liquidity":4,"markets":[{"id":"market-1","question":"Will it rain?","slug":"rain","active":true,"closed":false,"volume":"1.25","liquidity":"2.5","liquidityNum":3,"outcomes":"[\"Yes\",\"No\"]","outcomePrices":["0.25","0.75"],"clobTokenIds":"[\"101\",\"102\"]"},{"id":900719925474099312346,"question":"Will it be warm?","slug":"warm","active":true,"closed":false,"volumeNum":0,"liquidityNum":"2","outcomes":["Up","Down"],"outcomePrices":"[\"0\",\"1\"]"}]}]}`
}

func polymarketFixtureClient(body string) *Client {
	return &Client{HTTP: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}}
}
