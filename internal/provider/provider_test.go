package provider

import (
	"context"
	"io"
	"net/http"
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
