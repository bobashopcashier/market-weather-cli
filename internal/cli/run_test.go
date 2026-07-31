package cli

import (
	"encoding/json"
	"os"
	"testing"
)

func TestSchemaErrorsUseVersionedJSONEnvelope(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "stderr-*.json")
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stderr
	os.Stderr = file
	defer func() { os.Stderr = original }()
	if code := Run("", []string{"schema", "does-not-exist"}); code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(file.Name())
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
}

func TestArgumentControlsAreRejectedWithoutReflection(t *testing.T) {
	err := execute(t.Context(), "", []string{"metar", "BAD\x1b[31m"})
	if err == nil || err.Error() != "argument 2 contains a control character" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHelpFlagAfterArgumentsDoesNotExecuteCommand(t *testing.T) {
	output := captureStdout(t, func() error {
		return execute(t.Context(), "", []string{"metar", "KSFO", "--help"})
	})
	if len(output) == 0 {
		t.Fatal("expected command help")
	}
}

func TestProvidersHelpDoesNotRunReadiness(t *testing.T) {
	output := captureStdout(t, func() error {
		return execute(t.Context(), "", []string{"providers", "--help"})
	})
	if string(output) != toolHelp["providers"] {
		t.Fatalf("unexpected providers help: %q", output)
	}
}

func TestWrapperOptionScanStopsAtPassthroughBoundary(t *testing.T) {
	if hasArg([]string{"upstream", "--", "--help"}, "--help") {
		t.Fatal("wrapper interpreted passthrough help")
	}
}

func TestToolFirstSchemaDefaultsToJSON(t *testing.T) {
	if !schemaDefaultsToJSON("", []string{"metar", "schema", "missing"}) {
		t.Fatal("tool-first schema errors would not use JSON")
	}
}
