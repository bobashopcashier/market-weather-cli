package cli

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/bobashopcashier/market-weather-cli/internal/provider"
)

// Intentional response-shape changes require incrementing OutputContractRevision
// before recording a digest under the new version key.
var outputContractSchemaDigests = map[string]string{
	"mwx.output/betmoar.book/v1":        "2e881f5dbfe3b857e3047eeec1d7ba35ff6b82139e103cb0fa0b8b1755aeb341",
	"mwx.output/betmoar.search/v1":      "00e6e06d970cc286fa35e0866fae5269a3f5a6637b7f136276cfe63907568e09",
	"mwx.output/metar/v1":               "f98a1be5977b5ff6cb3df42983a237745ea9ba43b46ed865576d3c72ef9c15b2",
	"mwx.output/meteoblue/v1":           "1e8b13ca90ce50d618f52ace2181d3ac5a024756e0dc770eeb89601469b9c17c",
	"mwx.output/open-meteo/v1":          "348500d22e014b656ab6488696deb61abaedbdbf8ddf4233fe643bfa15e83863",
	"mwx.output/polyweather/v1":         "c37984419426a2ed2fd0d5f3b3c48c097fcfb3cd18a86d773cfbdf9cfe1b4136",
	"mwx.output/providers/v1":           "325a6f345d074d8ab23c6e6fd51ac38e4fb292efc278e2e56eba15b59492e8d1",
	"mwx.output/wethr.accuracy/v1":      "09aed0358c2b32e6d8cc5cbd0a6f866211c0345446aaf3226b4893b7ce69f944",
	"mwx.output/wethr.extreme/v1":       "09aed0358c2b32e6d8cc5cbd0a6f866211c0345446aaf3226b4893b7ce69f944",
	"mwx.output/wethr.forecast/v1":      "09aed0358c2b32e6d8cc5cbd0a6f866211c0345446aaf3226b4893b7ce69f944",
	"mwx.output/wethr.nearby/v1":        "09aed0358c2b32e6d8cc5cbd0a6f866211c0345446aaf3226b4893b7ce69f944",
	"mwx.output/wethr.nws/v1":           "09aed0358c2b32e6d8cc5cbd0a6f866211c0345446aaf3226b4893b7ce69f944",
	"mwx.output/wethr.obs/v1":           "09aed0358c2b32e6d8cc5cbd0a6f866211c0345446aaf3226b4893b7ce69f944",
	"mwx.output/wethr.pacing/v1":        "09aed0358c2b32e6d8cc5cbd0a6f866211c0345446aaf3226b4893b7ce69f944",
	"mwx.output/wethr.precipitation/v1": "09aed0358c2b32e6d8cc5cbd0a6f866211c0345446aaf3226b4893b7ce69f944",
	"mwx.output/wunderground/v1":        "b57413c5387bc3d679a750c9fdbba1bb5dcfa86da22ebf93884fefb33273e7ab",
}

func TestSchemaCatalogContainsExactlyProviderMethods(t *testing.T) {
	definitions := commandDefinitions()
	actual := make([]string, 0, len(definitions))
	for _, definition := range definitions {
		actual = append(actual, strings.Join(definition.Path, "."))
	}
	expected := []string{
		"betmoar.book", "betmoar.search", "betmoar.upstream", "metar", "meteoblue", "open-meteo", "polyweather", "providers",
		"wethr.accuracy", "wethr.extreme", "wethr.forecast", "wethr.nearby", "wethr.nws", "wethr.obs", "wethr.pacing", "wethr.precipitation",
		"wunderground",
	}
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("registry paths = %#v, want %#v", actual, expected)
	}
}

func TestOutputContractRevisionPinsPublishedResponseSchema(t *testing.T) {
	seen := map[string]bool{}
	for _, definition := range commandDefinitions() {
		if definition.Passthrough {
			continue
		}
		if definition.OutputContractRevision < 1 {
			t.Fatalf("%s has no explicit output contract revision", commandName(definition.Path))
		}
		version := outputContractVersionFor(definition)
		if seen[version] {
			t.Fatalf("duplicate output contract version %q", version)
		}
		seen[version] = true
		encoded, err := json.Marshal(schemaDocument(definition).Response)
		if err != nil {
			t.Fatal(err)
		}
		actual := fmt.Sprintf("%x", sha256.Sum256(encoded))
		expected, ok := outputContractSchemaDigests[version]
		if !ok {
			t.Fatalf("%s has no pinned response schema digest", version)
		}
		if actual != expected {
			t.Fatalf("%s response schema changed without a contract revision bump: got %s, want %s", version, actual, expected)
		}
	}
	for version := range outputContractSchemaDigests {
		if !seen[version] {
			t.Fatalf("pinned response schema %s has no registered command", version)
		}
	}
}

func TestSchemaMatchesStrictProviderOptions(t *testing.T) {
	definition := findDefinition(t, "wethr", "forecast")
	document := schemaDocument(definition)
	if document.SchemaVersion != "mwx.command-schema/v1" || document.CLIVersion != version {
		t.Fatalf("unexpected schema identity: %#v", document)
	}
	options := map[string]optionSchema{}
	for _, option := range document.Options {
		options[option.Name] = option
	}
	for _, expected := range []string{"--model", "--run", "--daily", "--params", "--json", "--fields", "--require-fields", "--compact", "--help"} {
		if _, ok := options[expected]; !ok {
			t.Errorf("forecast schema is missing %s", expected)
		}
	}
	for _, irrelevant := range []string{"--radius", "--mode", "--logic", "--window"} {
		if _, ok := options[irrelevant]; ok {
			t.Errorf("forecast schema unexpectedly contains %s", irrelevant)
		}
	}
	if options["--run"].Default != "latest" {
		t.Fatalf("run default = %#v", options["--run"].Default)
	}
	if document.CredentialEnv != "WETHR_API_KEY" || document.Output.MaximumProviderPayloadBytes == 0 || document.Output.MaximumJSONOutputBytes == 0 || len(document.Examples) == 0 {
		t.Fatalf("schema is missing agent contract details: %#v", document)
	}
	if document.EnvelopeSchemaVersion != agentSchemaVersion || document.OutputContractVersion != "mwx.output/wethr.forecast/v1" {
		t.Fatalf("schema is missing output contract identity: %#v", document)
	}
	if document.Params == nil || document.Params.AdditionalProperties || document.Params.MaximumBytes != maximumRawParamsBytes {
		t.Fatalf("raw params object schema is missing: %#v", document.Params)
	}
	paramsFields := map[string]rawParamsFieldSchema{}
	for _, field := range document.Params.Fields {
		paramsFields[field.Name] = field
	}
	if !paramsFields["station"].Required || !reflect.DeepEqual(paramsFields["station"].JSONTypes, []string{"string"}) || paramsFields["model"].JSONTypes[0] != "string" {
		t.Fatalf("raw params fields are incomplete: %#v", paramsFields)
	}
	if len(document.Positionals) != 1 || document.Positionals[0].Type != "string" || document.Response.Type != "object" || !hasResponseProperty(document.Response, "data") {
		t.Fatalf("request or response type is incomplete: %#v", document)
	}
}

func hasResponseProperty(schema JSONTypeSchema, name string) bool {
	for _, property := range schema.Properties {
		if property.Name == name {
			return true
		}
	}
	return false
}

func TestUnsafeUpstreamSchemaIsExplicit(t *testing.T) {
	document := schemaDocument(findDefinition(t, "betmoar", "upstream"))
	if document.ReadOnly || document.AgentInvocable || !document.Effects.Network || !document.Effects.ExternalProcess || document.Effects.Mutation != "unknown" {
		t.Fatalf("unsafe effects are not explicit: %#v", document)
	}
	if len(document.Options) != 1 || document.Options[0].Name != "--dry-run" || !reflect.DeepEqual(document.Output.Formats, []string{"passthrough", "json"}) {
		t.Fatalf("unexpected passthrough contract: %#v", document)
	}
	if len(document.ConditionalResponses) != 1 || document.ConditionalResponses[0].When != "--dry-run" || document.ConditionalResponses[0].SchemaVersion != "mwx.dry-run/v1" {
		t.Fatalf("dry-run response contract is missing: %#v", document.ConditionalResponses)
	}
	response := document.ConditionalResponses[0].Response
	for _, name := range []string{"schemaVersion", "executable", "arguments", "executes", "effects"} {
		if !hasResponseProperty(response, name) {
			t.Errorf("dry-run response is missing %s", name)
		}
	}
}

func TestSchemaPublishesRuntimeStringConstraints(t *testing.T) {
	book := schemaDocument(findDefinition(t, "betmoar", "book"))
	token := book.Positionals[0]
	if token.MaxLength != 128 || token.LengthUnit != "utf8Bytes" || token.Pattern != decimalTokenSchemaPattern || token.Normalization != "trim" {
		t.Fatalf("token constraint does not match runtime validation: %#v", token)
	}
	if book.Params.Fields[0].MaxLength != token.MaxLength || book.Params.Fields[0].LengthUnit != token.LengthUnit || book.Params.Fields[0].Pattern != token.Pattern {
		t.Fatalf("raw token constraint diverges from positional constraint: %#v", book.Params.Fields[0])
	}

	forecast := schemaDocument(findDefinition(t, "wethr", "forecast"))
	options := map[string]optionSchema{}
	for _, option := range forecast.Options {
		options[option.Name] = option
	}
	if options["--model"].MaxLength != 256 || options["--model"].Pattern != resourceValueSchemaPattern || options["--model"].Normalization != "trim" {
		t.Fatalf("Wethr model constraint does not match runtime validation: %#v", options["--model"])
	}
	if options["--fields"].MaxLength != maximumFieldMaskBytes || options["--fields"].LengthUnit != "utf8Bytes" || options["--fields"].MaximumPaths != maximumFieldPaths || options["--fields"].MaximumPathDepth != maximumFieldPathDepth {
		t.Fatalf("field-mask limits are missing: %#v", options["--fields"])
	}
	if options["--require-fields"].MaxLength != maximumFieldMaskBytes || options["--require-fields"].MaximumPaths != maximumFieldPaths || options["--require-fields"].MaximumPathDepth != maximumFieldPathDepth {
		t.Fatalf("required-field limits are missing: %#v", options["--require-fields"])
	}

	nws := schemaDocument(findDefinition(t, "wethr", "nws"))
	for _, option := range nws.Options {
		if option.Name == "--date" && (option.MaxLength != 10 || option.Pattern != dateSchemaPattern || option.Format != "date") {
			t.Fatalf("Wethr date constraint does not match runtime validation: %#v", option)
		}
	}
}

func TestResponseSchemaPreservesOmitEmpty(t *testing.T) {
	document := schemaDocument(findDefinition(t, "betmoar", "search"))
	source, found := responseProperty(document.Response, "source")
	if !found || !source.Required {
		t.Fatalf("required response property was not marked required: %#v", source)
	}
	truncation, found := responseProperty(document.Response, "truncation")
	if !found || truncation.Required {
		t.Fatalf("omitempty response property was not marked optional: %#v", truncation)
	}
}

func TestPolyweatherPublishesNestedItemLimits(t *testing.T) {
	document := schemaDocument(findDefinition(t, "polyweather"))
	if document.Output.ItemLimits["markets.events"] != 10 || document.Output.ItemLimits["markets.events.markets"] != provider.MaximumMarketsPerEvent {
		t.Fatalf("polyweather nested limits are incomplete: %#v", document.Output.ItemLimits)
	}
}

func responseProperty(schema JSONTypeSchema, name string) (JSONTypeField, bool) {
	for _, property := range schema.Properties {
		if property.Name == name {
			return property, true
		}
	}
	return JSONTypeField{}, false
}

func TestSchemaSupportsIndexDottedAndSpacedLookup(t *testing.T) {
	first := captureStdout(t, func() error { return runSchema(nil) })
	second := captureStdout(t, func() error { return runSchema(nil) })
	if string(first) != string(second) {
		t.Fatal("schema index is not deterministic")
	}
	var catalog commandIndexDocument
	if err := json.Unmarshal(first, &catalog); err != nil {
		t.Fatal(err)
	}
	if catalog.SchemaVersion != "mwx.command-index/v1" || catalog.CLIVersion != version || len(catalog.Commands) != 17 {
		t.Fatalf("unexpected catalog: %#v", catalog)
	}
	for _, command := range catalog.Commands {
		if !command.Effects.ExternalProcess && command.OutputContractVersion == "" {
			t.Fatalf("native command lacks an output contract: %#v", command)
		}
	}
	for _, path := range [][]string{{"wethr.forecast"}, {"wethr", "forecast"}} {
		data := captureStdout(t, func() error { return runSchema(path) })
		var document commandSchemaDocument
		if err := json.Unmarshal(data, &document); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(document.Path, []string{"wethr", "forecast"}) {
			t.Fatalf("schema path resolved to %#v", document.Path)
		}
	}
}

func findDefinition(t *testing.T, path ...string) commandDefinition {
	t.Helper()
	for _, definition := range commandDefinitions() {
		if reflect.DeepEqual(definition.Path, path) {
			return definition
		}
	}
	t.Fatalf("missing schema definition %q", strings.Join(path, "."))
	return commandDefinition{}
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
