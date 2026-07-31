package provider

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return fn(request) }

func TestRedactURL(t *testing.T) {
	redacted := RedactURL("https://example.test/data?apiKey=secret&station=KSFO&token=hidden")
	if strings.Contains(redacted, "secret") || strings.Contains(redacted, "hidden") {
		t.Fatalf("secret leaked in redacted URL: %s", redacted)
	}
	if !strings.Contains(redacted, "station=KSFO") {
		t.Fatalf("non-secret parameter was removed: %s", redacted)
	}
}

func TestNormalizeStations(t *testing.T) {
	stations, err := NormalizeStations([]string{"ksfo,kjfk", "KSFO"})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(stations, ","); got != "KSFO,KJFK" {
		t.Fatalf("unexpected stations: %s", got)
	}
	if _, err := NormalizeStations([]string{"not-a-station"}); err == nil {
		t.Fatal("expected invalid station error")
	}
	if _, err := NormalizeStation("KSFO,KJFK"); err == nil {
		t.Fatal("single-station validation accepted a list")
	}
}

func TestWethrRejectsQueryFragmentsBeforeCredentials(t *testing.T) {
	client := NewClient()
	for _, values := range []url.Values{{"model": {"HRRR?x=1"}}, {"run": {"latest#fragment"}}, {"window": {"%33%30d"}}} {
		if _, err := client.CallWethr(context.Background(), "forecasts", values); err == nil {
			t.Fatalf("accepted unsafe values: %#v", values)
		}
	}
}

func TestParseCoordinates(t *testing.T) {
	location, matched, err := parseCoordinates("37.7749,-122.4194")
	if err != nil || !matched {
		t.Fatalf("expected coordinate match: matched=%v err=%v", matched, err)
	}
	if location.Latitude != 37.7749 || location.Longitude != -122.4194 {
		t.Fatalf("unexpected location: %#v", location)
	}
	if _, _, err := parseCoordinates("91,0"); err == nil {
		t.Fatal("expected bounds error")
	}
}

func TestGetMETAR(t *testing.T) {
	client := &Client{HTTP: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Query().Get("ids") != "KSFO" || request.URL.Query().Get("format") != "json" {
			t.Fatalf("unexpected query: %s", request.URL.RawQuery)
		}
		if request.Header.Get("User-Agent") == "" {
			t.Fatal("missing user agent")
		}
		body := `[{"icaoId":"KSFO","reportTime":"2026-07-31T17:00:00Z","temp":18.3,"rawOb":"METAR KSFO"}]`
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}}
	result, err := client.GetMETAR(context.Background(), []string{"ksfo"}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Observations) != 1 || result.Observations[0].ICAOID != "KSFO" {
		t.Fatalf("unexpected observations: %#v", result.Observations)
	}
}

func TestHTTPErrorRedactsReflectedSecret(t *testing.T) {
	client := &Client{HTTP: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		body := `bad api key secret-value and Bearer header-secret`
		return &http.Response{StatusCode: 401, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}}
	var target map[string]any
	_, err := client.GetJSON(context.Background(), "https://example.test/data?apiKey=secret-value", map[string]string{"Authorization": "Bearer header-secret"}, &target, false)
	appErr, ok := err.(*Error)
	if !ok {
		t.Fatalf("unexpected error type: %T", err)
	}
	details := appErr.Details["body"].(string) + appErr.Details["url"].(string)
	if strings.Contains(details, "secret-value") || strings.Contains(details, "header-secret") {
		t.Fatalf("secret leaked in error details: %s", details)
	}
}

func TestDecodeStringList(t *testing.T) {
	encoded := decodeStringList([]byte(`"[\"Yes\",\"No\"]"`))
	if strings.Join(encoded, ",") != "Yes,No" {
		t.Fatalf("unexpected decoded list: %#v", encoded)
	}
	direct := decodeStringList([]byte(`["A","B"]`))
	if strings.Join(direct, ",") != "A,B" {
		t.Fatalf("unexpected direct list: %#v", direct)
	}
}

func TestAgentOrientedIdentifierValidation(t *testing.T) {
	for _, token := range []string{"123?fields=name", "123#fragment", "%31%32%33", "123\x1b"} {
		if _, err := validateDecimalToken(token); err == nil {
			t.Errorf("accepted unsafe token %q", token)
		}
	}
	if token, err := validateDecimalToken("1234567890"); err != nil || token != "1234567890" {
		t.Fatalf("valid token = %q, err = %v", token, err)
	}
	for _, station := range []string{"ABC?units=e", "ABC#x", "ABC%2FDEF", "ABC DEF", "ABC\x00"} {
		if _, err := validatePWSStation(station); err == nil {
			t.Errorf("accepted unsafe PWS station %q", station)
		}
	}
	if station, err := validatePWSStation("kmahanov10"); err != nil || station != "KMAHANOV10" {
		t.Fatalf("valid station = %q, err = %v", station, err)
	}
}

func TestUnsafeProviderInputsFailBeforeTransportOrCredentials(t *testing.T) {
	client := &Client{HTTP: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		t.Fatalf("transport called for unsafe input: %s", request.URL)
		return nil, nil
	})}}
	if _, err := client.GetOrderBook(context.Background(), "123?fields=name"); err == nil {
		t.Fatal("unsafe token was accepted")
	}
	if _, err := client.SearchMarkets(context.Background(), "weather\x1b[31m", 5, false); err == nil {
		t.Fatal("control character in query was accepted")
	}
	if _, err := client.GetPWSCurrent(context.Background(), "ABC?units=e", "e"); err == nil {
		t.Fatal("unsafe PWS station was accepted")
	}
}

func FuzzResourceIdentifiers(f *testing.F) {
	for _, seed := range []string{"123", "KMAHANOV10", "?fields=name", "%2e%2e", "abc\x00def", "abc#fragment"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, value string) {
		token, tokenErr := validateDecimalToken(value)
		if tokenErr == nil && !decimalTokenPattern.MatchString(token) {
			t.Fatalf("accepted token does not match grammar: %q", token)
		}
		station, stationErr := validatePWSStation(value)
		if stationErr == nil && !pwsStationPattern.MatchString(station) {
			t.Fatalf("accepted station does not match grammar: %q", station)
		}
	})
}
