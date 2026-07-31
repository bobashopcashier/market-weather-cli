package cli

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/bobashopcashier/market-weather-cli/internal/provider"
)

func TestFieldProjectionAcrossArrays(t *testing.T) {
	input := map[string]any{
		"source":  "noaa",
		"ignored": true,
		"observations": []map[string]any{
			{"icaoId": "KSFO", "temp": 18.0, "rawOb": "METAR KSFO"},
			{"icaoId": "KJFK", "temp": 22.0, "rawOb": "METAR KJFK"},
		},
	}
	projected, err := projectJSONFields(input, "source,observations.icaoId,observations.temp")
	if err != nil {
		t.Fatal(err)
	}
	mapped := projected.(map[string]any)
	if len(mapped) != 2 || mapped["source"] != "noaa" {
		t.Fatalf("unexpected projection: %#v", projected)
	}
	rows := mapped["observations"].([]any)
	if len(rows) != 2 || !reflect.DeepEqual(rows[0], map[string]any{"icaoId": "KSFO", "temp": json.Number("18")}) {
		t.Fatalf("unexpected projected rows: %#v", rows)
	}
}

func TestFieldProjectionRejectsHallucinatedPaths(t *testing.T) {
	input := map[string]any{"source": "noaa"}
	for _, mask := range []string{
		"missing",
		"source?fields=x",
		"",
		"   ",
		"source,source",
		"source,source.value",
		"source.value,source",
		"a.b.c.d.e.f.g.h.i",
		strings.Repeat("x", maximumFieldMaskBytes+1),
	} {
		if _, err := projectJSONFields(input, mask); err == nil {
			t.Errorf("accepted invalid field mask %q", mask)
		}
	}
}

func TestFieldProjectionAllowsEmptyResultArrays(t *testing.T) {
	type event struct {
		Title string `json:"title"`
	}
	type result struct {
		Events []event `json:"events"`
	}
	projected, err := projectJSONFields(result{Events: []event{}}, "events.title")
	if err != nil {
		t.Fatal(err)
	}
	events := projected.(map[string]any)["events"].([]any)
	if len(events) != 0 {
		t.Fatalf("unexpected empty projection: %#v", projected)
	}
	if _, err := projectJSONFields(result{Events: []event{}}, "events.titel"); err == nil {
		t.Fatal("empty typed array accepted a hallucinated field")
	}
}

func TestFieldProjectionAllowsDeclaredDynamicFieldsWithoutRuntimeExamples(t *testing.T) {
	type result struct {
		Units map[string]string `json:"units"`
	}
	for _, test := range []struct {
		value any
		mask  string
	}{
		{value: provider.MeteoblueResult{}, mask: "data.hourly.temperature_2m"},
		{value: provider.WethrResult{}, mask: "data.forecast.temperature"},
		{value: provider.OrderBookResult{}, mask: "book.bids.price"},
		{value: result{Units: map[string]string{}}, mask: "units.temperature"},
	} {
		if _, err := projectJSONFields(test.value, test.mask); err != nil {
			t.Errorf("declared dynamic field mask %q depends on response content: %v", test.mask, err)
		}
	}
	if _, err := projectJSONFields(result{}, "units.temperature.symbol"); err == nil {
		t.Fatal("typed dynamic map accepted a path below its scalar value")
	}
	if _, err := projectJSONFields(map[string]any{}, "missing"); err == nil {
		t.Fatal("ad hoc root map accepted a field absent from runtime content")
	}
}

func TestContextControlsRequireJSON(t *testing.T) {
	if _, err := parseArgs([]string{"KSFO", "--fields", "source"}, metarOptions); err == nil {
		t.Fatal("--fields was accepted without JSON output")
	}
	t.Setenv("MWX_OUTPUT", "json")
	parsed, err := parseArgs([]string{"KSFO", "--fields", "source", "--compact"}, metarOptions)
	if err != nil || !parsed.flag("json") || !parsed.flag("compact") {
		t.Fatalf("MWX_OUTPUT did not enable JSON controls: %#v, err=%v", parsed, err)
	}
}

func TestFieldMaskSyntaxIsRejectedDuringArgumentParsing(t *testing.T) {
	for _, argv := range [][]string{
		{"KSFO", "--json", "--fields="},
		{"KSFO", "--json", "--fields", "   "},
		{"KSFO", "--json", "--fields", "source,source"},
		{"KSFO", "--json", "--fields", "source,source.value"},
		{"KSFO", "--json", "--fields", "a.b.c.d.e.f.g.h.i"},
		{"KSFO", "--json", "--fields", strings.Repeat("x", maximumFieldMaskBytes+1)},
	} {
		if _, err := parseArgs(argv, metarOptions); err == nil {
			t.Errorf("accepted invalid argv %#v", argv)
		}
	}
}

func TestJSONDefaultAllowsBookContextControls(t *testing.T) {
	parsed, err := parseArgs([]string{"123", "--fields", "source", "--compact"}, betmoarOptions["book"], true)
	if err != nil || !parsed.flag("json") || !parsed.flag("compact") {
		t.Fatalf("JSON default was not applied: %#v, err=%v", parsed, err)
	}
	if !commandDefaultsToJSON("", []string{"betmoar", "book", "nope"}) || !commandDefaultsToJSON("betmoar", []string{"book", "nope"}) {
		t.Fatal("book errors would not default to JSON")
	}
}

func TestJSONOutputHasHardContextLimit(t *testing.T) {
	parsed := parsedArgs{values: map[string]string{}, bools: map[string]bool{"json": true, "compact": true}}
	err := writeJSON(parsed, map[string]any{"data": strings.Repeat("x", maximumJSONOutputBytes)})
	var appErr *provider.Error
	if !errors.As(err, &appErr) || appErr.Code != "output_too_large" {
		t.Fatalf("oversized output error = %#v", err)
	}
	err = writeSafeHumanJSON("provider\n", map[string]any{"data": strings.Repeat("x", maximumJSONOutputBytes)})
	if !errors.As(err, &appErr) || appErr.Code != "output_too_large" {
		t.Fatalf("oversized human output error = %#v", err)
	}
}
