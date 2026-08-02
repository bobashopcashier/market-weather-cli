package cli

import (
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"testing"

	"github.com/bobashopcashier/market-weather-cli/internal/provider"
)

func TestRequiredFieldsCheckEveryArrayItem(t *testing.T) {
	document := map[string]any{
		"events": []any{
			map[string]any{"title": "first"},
			map[string]any{"title": nil},
		},
	}
	err := validateTaskRequiredFields("betmoar.search", document, "events.title")
	var appErr *provider.Error
	if !errors.As(err, &appErr) || appErr.Code != "UPSTREAM_SCHEMA_MISMATCH" || appErr.ExitCode != 6 {
		t.Fatalf("required-field error = %#v", err)
	}
	if got := appErr.Details["missingFields"]; !reflect.DeepEqual(got, []string{"events[].title"}) {
		t.Fatalf("missing fields = %#v", got)
	}
	if appErr.Hint != "Affected JSON paths: events[].title" {
		t.Fatalf("required-field hint = %q", appErr.Hint)
	}
	if appErr.Details["outputContractVersion"] != "mwx.output/betmoar.search/v1" {
		t.Fatalf("contract identity = %#v", appErr.Details)
	}
}

func TestRequiredFieldsAllowEmptyArrays(t *testing.T) {
	document := map[string]any{"events": []any{}}
	if err := validateTaskRequiredFields("betmoar.search", document, "events.title"); err != nil {
		t.Fatalf("empty collection failed: %v", err)
	}
}

func TestRequiredFieldsMustBeCoveredByProjection(t *testing.T) {
	_, err := parseArgs([]string{"KSFO", "--json", "--fields", "source", "--require-fields", "observations.icaoId"}, metarOptions)
	if err == nil {
		t.Fatal("accepted an unprojected required field")
	}
	parsed, err := parseArgs([]string{"KSFO", "--json", "--fields", "observations", "--require-fields", "observations.icaoId"}, metarOptions)
	if err != nil || parsed.value("require-fields") == "" {
		t.Fatalf("parent projection did not cover required field: %#v, %v", parsed, err)
	}
}

func TestRequiredFieldsRejectTypedTyposBeforeExecution(t *testing.T) {
	err := preflightResponseFields("metar", []string{"KSFO", "--json", "--require-fields", "observations.titel"})
	var appErr *provider.Error
	if !errors.As(err, &appErr) || appErr.Code != "invalid_arguments" {
		t.Fatalf("typed typo error = %#v", err)
	}
	if err := preflightResponseFields("open-meteo", []string{"KSFO", "--json", "--require-fields", "forecast.daily.temperature_2m_max"}); err != nil {
		t.Fatalf("dynamic provider path was rejected before runtime validation: %v", err)
	}
	if err := preflightResponseFields("metar", []string{"KSFO", "--json", "--fields", "observations.titel"}); err == nil {
		t.Fatal("typed projection typo reached command execution")
	}
}

func TestJSONSuccessUsesVersionedCommandEnvelope(t *testing.T) {
	parsed := parsedArgs{
		values: map[string]string{"fields": "source,observations.icaoId", "require-fields": "source,observations.icaoId"},
		bools:  map[string]bool{"json": true, "compact": true},
	}
	data := captureStdout(t, func() error {
		return writeJSON(parsed, "metar", provider.METARResult{
			Source: "noaa", FetchedAt: "2026-08-01T00:00:00Z", Stations: []string{"KSFO"},
			Observations: []provider.METARObservation{{ICAOID: "KSFO"}},
		})
	})
	var envelope agentEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatal(err)
	}
	if !envelope.OK || envelope.Command != "metar" || envelope.SchemaVersion != agentSchemaVersion || envelope.OutputContractVersion != "mwx.output/metar/v1" || envelope.Error != nil {
		t.Fatalf("unexpected envelope: %#v", envelope)
	}
	mapped, ok := envelope.Data.(map[string]any)
	if !ok || len(mapped) != 2 {
		t.Fatalf("projection was not applied to envelope data: %#v", envelope.Data)
	}
}

func TestErrorContractsDoNotInventUnknownCommandVersions(t *testing.T) {
	if got := errorOutputContractVersion("metar"); got != "mwx.output/metar/v1" {
		t.Fatalf("known command error contract = %q", got)
	}
	for _, command := range []string{"", "schema", "does-not-exist", "betmoar.upstream"} {
		if got := errorOutputContractVersion(command); got != genericErrorOutputContractVersion {
			t.Fatalf("unresolved command %q error contract = %q", command, got)
		}
	}
}

func TestOutputContractFailurePrecedesProjection(t *testing.T) {
	stdout, err := os.CreateTemp(t.TempDir(), "stdout-*.json")
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stdout
	os.Stdout = stdout
	defer func() { os.Stdout = original }()
	parsed := parsedArgs{values: map[string]string{"fields": "source"}, bools: map[string]bool{"json": true}}
	err = writeJSON(parsed, "metar", map[string]any{"source": "noaa"})
	var appErr *provider.Error
	if !errors.As(err, &appErr) || appErr.Code != "INTERNAL_OUTPUT_CONTRACT_MISMATCH" || appErr.ExitCode != 10 {
		t.Fatalf("output contract error = %#v", err)
	}
	if got := appErr.Details["missingFields"]; !reflect.DeepEqual(got, []string{"fetchedAt", "observations", "stations"}) {
		t.Fatalf("missing fields = %#v", got)
	}
	if err := stdout.Close(); err != nil {
		t.Fatal(err)
	}
	if output, err := os.ReadFile(stdout.Name()); err != nil || len(output) != 0 {
		t.Fatalf("contract failure leaked partial stdout: %q, err=%v", output, err)
	}
}
