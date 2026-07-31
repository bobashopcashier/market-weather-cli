package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bobashopcashier/market-weather-cli/internal/provider"
)

func TestSchemaErrorsUseVersionedJSONEnvelope(t *testing.T) {
	stderr, err := os.CreateTemp(t.TempDir(), "stderr-*.json")
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := os.CreateTemp(t.TempDir(), "stdout-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	originalErr, originalOut := os.Stderr, os.Stdout
	os.Stderr, os.Stdout = stderr, stdout
	defer func() { os.Stderr, os.Stdout = originalErr, originalOut }()
	if code := Run("", []string{"schema", "does-not-exist"}); code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if err := stderr.Close(); err != nil {
		t.Fatal(err)
	}
	if err := stdout.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(stderr.Name())
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		SchemaVersion string `json:"schemaVersion"`
		Error         struct {
			Code     string `json:"code"`
			ExitCode int    `json:"exitCode"`
		} `json:"error"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatalf("invalid JSON error: %v: %s", err, data)
	}
	if envelope.SchemaVersion != "mwx.error/v1" || envelope.Error.Code != "invalid_arguments" || envelope.Error.ExitCode != 2 {
		t.Fatalf("unexpected error envelope: %#v", envelope)
	}
	if output, err := os.ReadFile(stdout.Name()); err != nil || len(output) != 0 {
		t.Fatalf("JSON error wrote stdout: %q, err=%v", output, err)
	}
}

func TestArgumentControlsAreRejectedWithoutReflection(t *testing.T) {
	for _, value := range []string{"BAD\x1b[31m", "BAD\u202eTXT", "BAD\u200bTXT"} {
		err := execute(t.Context(), "", []string{"metar", value})
		if err == nil || err.Error() != "argument 2 contains a control character" {
			t.Fatalf("unexpected error for %q: %v", value, err)
		}
	}
}

func TestHelpAnywhereDoesNotExecute(t *testing.T) {
	output := captureStdout(t, func() error {
		return execute(t.Context(), "", []string{"metar", "KSFO", "--help"})
	})
	if !strings.Contains(string(output), "--hours") || !strings.Contains(string(output), "mwx schema metar") {
		t.Fatalf("unexpected command help: %q", output)
	}
	output = captureStdout(t, func() error {
		return execute(t.Context(), "", []string{"providers", "--help"})
	})
	if !strings.Contains(string(output), "mwx providers") || !strings.Contains(string(output), "--params") {
		t.Fatalf("providers executed instead of showing help: %q", output)
	}
	output = captureStdout(t, func() error {
		return execute(t.Context(), "", []string{"wethr", "forecast", "--help"})
	})
	if !strings.Contains(string(output), "--model") || strings.Contains(string(output), "--radius") {
		t.Fatalf("nested help is not operation-specific: %q", output)
	}
}

func TestOperationSpecificOptionsRejectIrrelevantFlags(t *testing.T) {
	err := execute(t.Context(), "", []string{"wethr", "obs", "KSFO", "--radius", "10"})
	if err == nil || err.Error() != "unknown option: --radius" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPolyweatherValidatesAllUserTextBeforeNetwork(t *testing.T) {
	for _, argv := range [][]string{
		{"polyweather", "KSFO", strings.Repeat("c", 257)},
		{"polyweather", "KSFO", "San Francisco", "--market", strings.Repeat("m", 513)},
	} {
		if err := execute(t.Context(), "", argv); err == nil {
			t.Fatalf("accepted oversized user text: %#v", argv)
		}
	}
}

func TestWrapperOptionScanStopsAtPassthroughBoundary(t *testing.T) {
	if hasArg([]string{"upstream", "--", "--help"}, "--help") {
		t.Fatal("wrapper interpreted passthrough help")
	}
	if err := runPolymarketUpstream(t.Context(), []string{"markets", "search"}); err == nil {
		t.Fatal("upstream accepted arguments without an explicit boundary")
	}
}

func TestUpstreamDryRunDoesNotSpawn(t *testing.T) {
	directory := t.TempDir()
	executable := filepath.Join(directory, "polymarket")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\nexit 99\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", directory)
	data := captureStdout(t, func() error {
		return runPolymarketUpstream(t.Context(), []string{"--dry-run", "--", "markets", "search", "bitcoin"})
	})
	var result struct {
		SchemaVersion string   `json:"schemaVersion"`
		Executable    string   `json:"executable"`
		Arguments     []string `json:"arguments"`
		Executes      bool     `json:"executes"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	if result.SchemaVersion != "mwx.dry-run/v1" || result.Executable != executable || result.Executes || len(result.Arguments) != 3 {
		t.Fatalf("unexpected dry run: %#v", result)
	}
}

func TestUpstreamBlocksPositionalPrivateKeysBeforeRendering(t *testing.T) {
	key := "0x" + "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	for _, args := range [][]string{
		{"--dry-run", "--", "wallet", "import", key},
		{"--dry-run", "--", "-o", "json", "wallet", "IMPORT", key},
		{"--dry-run", "--", "markets", "search", "bitcoin", "--private-key=" + key},
	} {
		if err := runPolymarketUpstream(t.Context(), args); err == nil {
			t.Fatalf("accepted credential-bearing args: %#v", args)
		}
	}
}

func TestGeneratedLeafHelpIncludesCredentialEnvironment(t *testing.T) {
	for _, test := range []struct {
		argv       []string
		credential string
	}{
		{argv: []string{"meteoblue", "--help"}, credential: "METEOBLUE_API_KEY"},
		{argv: []string{"wunderground", "--help"}, credential: "WEATHER_COMPANY_API_KEY"},
		{argv: []string{"wethr", "forecast", "--help"}, credential: "WETHR_API_KEY"},
	} {
		output := captureStdout(t, func() error { return execute(t.Context(), "", test.argv) })
		if !strings.Contains(string(output), "Requires "+test.credential+" in the environment") {
			t.Fatalf("help for %v omitted credential requirement: %q", test.argv, output)
		}
	}
}

func TestHumanMarketOutputReportsTruncationCounts(t *testing.T) {
	output := captureStdout(t, func() error {
		printMarketSearch(provider.MarketSearchResult{
			Events: []provider.MarketEvent{{Title: "Weather", Slug: "weather"}},
			Truncation: []provider.Truncation{
				{Path: "events", SourceCount: 9, EmittedCount: 5},
				{Path: "events.42.markets", SourceCount: 150, EmittedCount: 100},
			},
		})
		return nil
	})
	text := string(output)
	for _, expected := range []string{"Results truncated by safety limits:", "AVAILABLE", "SHOWN", "events.42.markets", "150", "100"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("human output omitted %q: %q", expected, text)
		}
	}
}

func TestToolFirstSchemaDefaultsToJSON(t *testing.T) {
	if !schemaDefaultsToJSON("", []string{"metar", "schema", "missing"}) {
		t.Fatal("tool-first schema errors would not use JSON")
	}
	data := captureStdout(t, func() error {
		return execute(t.Context(), "metar", []string{"schema"})
	})
	var document commandSchemaDocument
	if err := json.Unmarshal(data, &document); err != nil || len(document.Path) != 1 || document.Path[0] != "metar" {
		t.Fatalf("standalone schema failed: %#v, err=%v", document, err)
	}
}
