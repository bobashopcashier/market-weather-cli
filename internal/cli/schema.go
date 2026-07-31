package cli

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/bobashopcashier/market-weather-cli/internal/provider"
	"github.com/bobashopcashier/market-weather-cli/internal/render"
)

type positionalSchema struct {
	Name        string `json:"name"`
	Required    bool   `json:"required"`
	Variadic    bool   `json:"variadic,omitempty"`
	Description string `json:"description,omitempty"`
}

type optionSchema struct {
	Name    string `json:"name"`
	Alias   string `json:"alias,omitempty"`
	Type    string `json:"type"`
	Default any    `json:"default,omitempty"`
	Enum    []any  `json:"enum,omitempty"`
	Minimum *int   `json:"minimum,omitempty"`
	Maximum *int   `json:"maximum,omitempty"`
}

type commandConstraints struct {
	RequiredOptions []string   `json:"requiredOptions,omitempty"`
	AnyOfOptions    [][]string `json:"anyOfOptions,omitempty"`
	Rules           []string   `json:"rules,omitempty"`
}

type outputSchema struct {
	Formats        []string `json:"formats"`
	DefaultFormat  string   `json:"defaultFormat"`
	SchemaVersions []string `json:"schemaVersions,omitempty"`
}

type effectsSchema struct {
	Network         bool   `json:"network"`
	Mutation        string `json:"mutation"`
	ExternalProcess bool   `json:"externalProcess"`
}

type commandDefinition struct {
	Path           []string
	Summary        string
	ReadOnly       bool
	AgentInvocable bool
	Positionals    []positionalSchema
	Options        map[string]optionSpec
	Required       []string
	AnyOf          [][]string
	Rules          []string
	Output         outputSchema
	CredentialEnv  string
	Passthrough    bool
}

type commandSchemaDocument struct {
	SchemaVersion  string             `json:"schemaVersion"`
	CLIVersion     string             `json:"cliVersion"`
	Path           []string           `json:"path"`
	Summary        string             `json:"summary"`
	ReadOnly       bool               `json:"readOnly"`
	AgentInvocable bool               `json:"agentInvocable"`
	Effects        effectsSchema      `json:"effects"`
	Positionals    []positionalSchema `json:"positionals,omitempty"`
	Options        []optionSchema     `json:"options"`
	Constraints    commandConstraints `json:"constraints,omitempty"`
	Output         outputSchema       `json:"output"`
	CredentialEnv  string             `json:"credentialEnv,omitempty"`
}

type commandIndexEntry struct {
	Path           []string      `json:"path"`
	Summary        string        `json:"summary"`
	ReadOnly       bool          `json:"readOnly"`
	AgentInvocable bool          `json:"agentInvocable"`
	Effects        effectsSchema `json:"effects"`
}

type commandIndexDocument struct {
	SchemaVersion string              `json:"schemaVersion"`
	CLIVersion    string              `json:"cliVersion"`
	Prefix        []string            `json:"prefix"`
	Commands      []commandIndexEntry `json:"commands"`
}

func runSchema(path []string) error {
	path = normalizeSchemaPath(path)
	definitions := commandDefinitions()
	for _, definition := range definitions {
		if equalStrings(definition.Path, path) {
			return render.JSON(os.Stdout, schemaDocument(definition))
		}
	}
	matches := make([]commandIndexEntry, 0)
	for _, definition := range definitions {
		if hasStringPrefix(definition.Path, path) {
			matches = append(matches, commandIndexEntry{
				Path: definition.Path, Summary: definition.Summary, ReadOnly: definition.ReadOnly, AgentInvocable: definition.AgentInvocable, Effects: effectsFor(definition),
			})
		}
	}
	if len(matches) == 0 {
		display := strings.Join(path, " ")
		if display == "" {
			display = "root"
		}
		return provider.NewError("invalid_arguments", fmt.Sprintf("unknown schema path: %s", display), 2)
	}
	return render.JSON(os.Stdout, commandIndexDocument{
		SchemaVersion: "mwx.command-index/v1", CLIVersion: version, Prefix: path, Commands: matches,
	})
}

func normalizeSchemaPath(path []string) []string {
	result := append([]string{}, path...)
	if len(result) == 1 && strings.Contains(result[0], ".") {
		result = strings.Split(result[0], ".")
	}
	if len(result) > 0 && result[0] == "dataframe" {
		result[0] = "data"
	}
	if len(result) > 1 && result[0] == "data" {
		result[1] = normalizeDataOperation(result[1])
	}
	return result
}

func schemaDocument(definition commandDefinition) commandSchemaDocument {
	options := make(map[string]optionSpec, len(definition.Options)+len(globalOptions))
	if !definition.Passthrough {
		for name, spec := range globalOptions {
			options[name] = spec
		}
	}
	for name, spec := range definition.Options {
		options[name] = spec
	}
	names := make([]string, 0, len(options))
	for name := range options {
		names = append(names, name)
	}
	sort.Strings(names)
	optionDocuments := make([]optionSchema, 0, len(names))
	for _, name := range names {
		optionDocuments = append(optionDocuments, describeOption(name, options[name]))
	}
	return commandSchemaDocument{
		SchemaVersion:  "mwx.command-schema/v1",
		CLIVersion:     version,
		Path:           definition.Path,
		Summary:        definition.Summary,
		ReadOnly:       definition.ReadOnly,
		AgentInvocable: definition.AgentInvocable,
		Effects:        effectsFor(definition),
		Positionals:    definition.Positionals,
		Options:        optionDocuments,
		Constraints: commandConstraints{
			RequiredOptions: append([]string(nil), definition.Required...),
			AnyOfOptions:    cloneStringLists(definition.AnyOf),
			Rules:           append([]string(nil), definition.Rules...),
		},
		Output:        definition.Output,
		CredentialEnv: definition.CredentialEnv,
	}
}

func effectsFor(definition commandDefinition) effectsSchema {
	if definition.Passthrough {
		return effectsSchema{Network: true, Mutation: "unknown", ExternalProcess: true}
	}
	network := definition.Path[0] != "data" && definition.Path[0] != "providers"
	return effectsSchema{Network: network, Mutation: "none"}
}

func describeOption(name string, spec optionSpec) optionSchema {
	document := optionSchema{Name: "--" + name}
	if spec.alias != "" {
		document.Alias = "-" + spec.alias
	}
	switch spec.kind {
	case boolOption:
		document.Type = "boolean"
		document.Default = false
	case intOption:
		document.Type = "integer"
		for _, choice := range spec.choices {
			value, _ := strconv.Atoi(choice)
			document.Enum = append(document.Enum, value)
		}
		if spec.defaultVal != "" {
			value, _ := strconv.Atoi(spec.defaultVal)
			document.Default = value
		}
	default:
		document.Type = "string"
		for _, choice := range spec.choices {
			document.Enum = append(document.Enum, choice)
		}
		if spec.defaultVal != "" {
			document.Default = spec.defaultVal
		}
	}
	if spec.min != 0 {
		value := spec.min
		document.Minimum = &value
	}
	if spec.max != 0 {
		value := spec.max
		document.Maximum = &value
	}
	return document
}

func commandDefinitions() []commandDefinition {
	providerJSON := outputSchema{Formats: []string{"text", "json"}, DefaultFormat: "text"}
	dataTable := outputSchema{Formats: []string{"json", "csv", "table"}, DefaultFormat: "json", SchemaVersions: []string{"mwx.table/v1"}}
	dataResult := outputSchema{Formats: []string{"json"}, DefaultFormat: "json", SchemaVersions: []string{"mwx.result/v1"}}
	definitions := []commandDefinition{
		{Path: []string{"betmoar", "search"}, Summary: "Search public Polymarket markets", ReadOnly: true, AgentInvocable: true, Positionals: []positionalSchema{{Name: "query", Required: true, Variadic: true}}, Options: betmoarOptions["search"], Output: providerJSON},
		{Path: []string{"betmoar", "book"}, Summary: "Fetch a public Polymarket order book", ReadOnly: true, AgentInvocable: true, Positionals: []positionalSchema{{Name: "token-id", Required: true}}, Options: betmoarOptions["book"], Output: outputSchema{Formats: []string{"json"}, DefaultFormat: "json"}},
		{Path: []string{"betmoar", "upstream"}, Summary: "Delegate arguments to the official Polymarket CLI", ReadOnly: false, AgentInvocable: false, Positionals: []positionalSchema{{Name: "arguments", Variadic: true, Description: "May mutate or trade; requires explicit user review"}}, Options: map[string]optionSpec{}, Output: outputSchema{Formats: []string{"passthrough"}, DefaultFormat: "passthrough"}, Passthrough: true},
		{Path: []string{"metar"}, Summary: "Fetch NOAA aviation observations", ReadOnly: true, AgentInvocable: true, Positionals: []positionalSchema{{Name: "station", Required: true, Variadic: true}}, Options: metarOptions, Output: providerJSON},
		{Path: []string{"open-meteo"}, Summary: "Resolve a location and fetch weather forecasts", ReadOnly: true, AgentInvocable: true, Positionals: []positionalSchema{{Name: "location", Required: true, Variadic: true}}, Options: openMeteoOptions, Output: providerJSON},
		{Path: []string{"polyweather"}, Summary: "Combine METAR, forecast, and market data", ReadOnly: true, AgentInvocable: true, Positionals: []positionalSchema{{Name: "station", Required: true}, {Name: "city", Variadic: true}}, Options: polyweatherOptions, Output: providerJSON},
		{Path: []string{"meteoblue"}, Summary: "Fetch a meteoblue forecast package", ReadOnly: true, AgentInvocable: true, Positionals: []positionalSchema{{Name: "location", Required: true, Variadic: true}}, Options: meteoblueOptions, Output: providerJSON, CredentialEnv: "METEOBLUE_API_KEY"},
		{Path: []string{"wunderground"}, Summary: "Fetch Weather Underground PWS observations", ReadOnly: true, AgentInvocable: true, Positionals: []positionalSchema{{Name: "station-id", Required: true}}, Options: wundergroundOptions, Output: providerJSON, CredentialEnv: "WEATHER_COMPANY_API_KEY"},
		{Path: []string{"providers"}, Summary: "Inspect provider and credential readiness", ReadOnly: true, AgentInvocable: true, Options: map[string]optionSpec{}, Output: providerJSON},
	}
	wethrSummaries := map[string]string{
		"obs": "Fetch Wethr observations", "extreme": "Fetch Wethr temperature extremes", "forecast": "Fetch Wethr model forecasts",
		"precipitation": "Fetch Wethr precipitation data", "nws": "Fetch Wethr NWS forecasts", "pacing": "Fetch Wethr model pacing",
		"accuracy": "Fetch Wethr model accuracy", "nearby": "Find nearby Wethr stations",
	}
	wethrNames := make([]string, 0, len(wethrOptions))
	for name := range wethrOptions {
		wethrNames = append(wethrNames, name)
	}
	sort.Strings(wethrNames)
	for _, name := range wethrNames {
		definitions = append(definitions, commandDefinition{
			Path: []string{"wethr", name}, Summary: wethrSummaries[name], ReadOnly: true, AgentInvocable: true,
			Positionals: []positionalSchema{{Name: "station", Required: true}}, Options: wethrOptions[name], Output: providerJSON, CredentialEnv: "WETHR_API_KEY",
		})
	}
	for _, operation := range dataOperationNames() {
		spec := dataOperations[operation]
		output := dataTable
		if spec.structuredOutput {
			output = dataResult
		}
		rules := []string{"the positional input and --input are mutually exclusive"}
		if !spec.structuredOutput {
			rules = append(rules, "--compact and --limit require JSON output")
		}
		switch operation {
		case "fillna":
			rules = append(rules, "--value is required when --strategy is literal or omitted")
		case "get-dummies":
			rules = append(rules, "--prefix requires exactly one selected column")
		}
		definitions = append(definitions, commandDefinition{
			Path: []string{"data", operation}, Summary: spec.summary, ReadOnly: true, AgentInvocable: true,
			Positionals: []positionalSchema{{Name: "input", Description: "File path or - for standard input"}},
			Options:     dataOperationOptions(operation), Required: spec.required, AnyOf: spec.anyOf, Rules: rules, Output: output,
		})
	}
	sort.Slice(definitions, func(i, j int) bool {
		return strings.Join(definitions[i].Path, "\x00") < strings.Join(definitions[j].Path, "\x00")
	})
	return definitions
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func hasStringPrefix(values, prefix []string) bool {
	return len(values) >= len(prefix) && equalStrings(values[:len(prefix)], prefix)
}

func cloneStringLists(values [][]string) [][]string {
	result := make([][]string, len(values))
	for index, value := range values {
		result[index] = append([]string(nil), value...)
	}
	return result
}
