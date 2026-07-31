package cli

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestCommandRegistryCoversAllDataOperations(t *testing.T) {
	dataCount := 0
	seen := map[string]bool{}
	for _, definition := range commandDefinitions() {
		key := definition.Path[0]
		for _, component := range definition.Path[1:] {
			key += "." + component
		}
		if seen[key] {
			t.Fatalf("duplicate command definition %q", key)
		}
		seen[key] = true
		if definition.Path[0] == "data" {
			dataCount++
		}
	}
	if dataCount != 35 {
		t.Fatalf("registry has %d dataframe operations, want 35", dataCount)
	}
	for _, required := range []string{"metar", "betmoar.search", "betmoar.book", "betmoar.upstream", "wethr.forecast", "providers"} {
		if !seen[required] {
			t.Errorf("registry is missing %q", required)
		}
	}
}

func TestSchemaDocumentUsesExactOperationOptions(t *testing.T) {
	var target commandDefinition
	for _, definition := range commandDefinitions() {
		if reflect.DeepEqual(definition.Path, []string{"data", "query"}) {
			target = definition
			break
		}
	}
	document := schemaDocument(target)
	if document.SchemaVersion != "mwx.command-schema/v1" || document.CLIVersion != version {
		t.Fatalf("unexpected schema identity: %#v", document)
	}
	options := map[string]bool{}
	for _, option := range document.Options {
		options[option.Name] = true
	}
	for _, expected := range []string{"--expr", "--input", "--input-root", "--fields", "--limit", "--compact", "--output", "--json", "--help"} {
		if !options[expected] {
			t.Errorf("query schema is missing %s", expected)
		}
	}
	for _, irrelevant := range []string{"--agg", "--bins", "--dtype", "--mapping"} {
		if options[irrelevant] {
			t.Errorf("query schema unexpectedly contains %s", irrelevant)
		}
	}
	if !reflect.DeepEqual(document.Constraints.RequiredOptions, []string{"expr"}) {
		t.Fatalf("query requirements = %#v", document.Constraints.RequiredOptions)
	}
}

func TestUnsafeUpstreamIsMarkedNonInvocablePassthrough(t *testing.T) {
	for _, definition := range commandDefinitions() {
		if !reflect.DeepEqual(definition.Path, []string{"betmoar", "upstream"}) {
			continue
		}
		document := schemaDocument(definition)
		if document.ReadOnly || document.AgentInvocable || !document.Effects.ExternalProcess || document.Effects.Mutation != "unknown" {
			t.Fatalf("unsafe effects are not explicit: %#v", document)
		}
		if len(document.Options) != 0 || !reflect.DeepEqual(document.Output.Formats, []string{"passthrough"}) {
			t.Fatalf("passthrough schema should not claim wrapper flags: %#v", document)
		}
		return
	}
	t.Fatal("betmoar upstream definition not found")
}

func TestReadCSVSchemaReflectsItsCSVDefault(t *testing.T) {
	for _, definition := range commandDefinitions() {
		if !reflect.DeepEqual(definition.Path, []string{"data", "read-csv"}) {
			continue
		}
		for _, option := range schemaDocument(definition).Options {
			if option.Name == "--input-format" {
				if option.Default != "csv" {
					t.Fatalf("read-csv input default = %#v", option.Default)
				}
				return
			}
		}
		t.Fatal("read-csv schema is missing --input-format")
	}
	t.Fatal("read-csv definition not found")
}

func TestConditionalConstraintsAndStructuredFormatsAreDescribed(t *testing.T) {
	for _, definition := range commandDefinitions() {
		document := schemaDocument(definition)
		switch strings.Join(definition.Path, ".") {
		case "data.fillna":
			if !strings.Contains(strings.Join(document.Constraints.Rules, " "), "--value") {
				t.Fatalf("fillna conditional requirement is missing: %#v", document.Constraints)
			}
		case "data.profile", "data.idxmax", "data.to-numpy":
			for _, option := range document.Options {
				if option.Name == "--output" && !reflect.DeepEqual(option.Enum, []any{"json"}) {
					t.Fatalf("structured output enum = %#v", option.Enum)
				}
			}
		}
	}
}

func TestSchemaSupportsCompactIndexAndDottedLookup(t *testing.T) {
	index := captureStdout(t, func() error { return runSchema(nil) })
	var catalog struct {
		SchemaVersion string              `json:"schemaVersion"`
		CLIVersion    string              `json:"cliVersion"`
		Commands      []commandIndexEntry `json:"commands"`
	}
	if err := json.Unmarshal(index, &catalog); err != nil {
		t.Fatal(err)
	}
	if catalog.SchemaVersion != "mwx.command-index/v1" || catalog.CLIVersion != version || len(catalog.Commands) < 50 {
		t.Fatalf("unexpected catalog: version=%q CLI=%q commands=%d", catalog.SchemaVersion, catalog.CLIVersion, len(catalog.Commands))
	}

	documentData := captureStdout(t, func() error { return runSchema([]string{"data.query"}) })
	var document commandSchemaDocument
	if err := json.Unmarshal(documentData, &document); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(document.Path, []string{"data", "query"}) {
		t.Fatalf("dotted schema path resolved to %#v", document.Path)
	}
}

func captureStdout(t *testing.T, run func() error) []byte {
	t.Helper()
	file, err := os.CreateTemp(t.TempDir(), "stdout-*.json")
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stdout
	os.Stdout = file
	defer func() { os.Stdout = original }()
	if err := run(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(file.Name())
	if err != nil {
		t.Fatal(err)
	}
	return data
}
