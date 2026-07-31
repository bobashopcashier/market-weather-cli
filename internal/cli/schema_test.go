package cli

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/bobashopcashier/market-weather-cli/internal/provider"
)

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
	for _, expected := range []string{"--model", "--run", "--daily", "--params", "--json", "--fields", "--compact", "--help"} {
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
	if options["--fields"].MaxLength != maximumFieldMaskBytes || options["--fields"].LengthUnit != "utf8Bytes" {
		t.Fatalf("field-mask length is missing: %#v", options["--fields"])
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
