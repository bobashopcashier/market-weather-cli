package cli

import (
	"fmt"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/bobashopcashier/market-weather-cli/internal/provider"
	"github.com/bobashopcashier/market-weather-cli/internal/render"
)

const (
	stationSchemaPattern          = `^[A-Za-z0-9]{3,4}$`
	stationListSchemaPattern      = `^[A-Za-z0-9]{3,4}( *, *[A-Za-z0-9]{3,4})*$`
	decimalTokenSchemaPattern     = `^[0-9]{1,128}$`
	pwsStationSchemaPattern       = `^[A-Za-z0-9_-]{1,64}$`
	resourceValueSchemaPattern    = `^[A-Za-z0-9][A-Za-z0-9._,-]{0,255}$`
	windowSchemaPattern           = `^[1-9][0-9]{0,3}[dhm]$`
	dateSchemaPattern             = `^[0-9]{4}-[0-9]{2}-[0-9]{2}$`
	meteobluePackageSchemaPattern = `^[A-Za-z0-9_-]+$`
)

type positionalSchema struct {
	Name          string `json:"name"`
	Type          string `json:"type"`
	Required      bool   `json:"required"`
	Variadic      bool   `json:"variadic,omitempty"`
	MaxLength     int    `json:"maxLength,omitempty"`
	LengthUnit    string `json:"lengthUnit,omitempty"`
	Pattern       string `json:"pattern,omitempty"`
	Format        string `json:"format,omitempty"`
	Normalization string `json:"normalization,omitempty"`
	Description   string `json:"description,omitempty"`
}

type rawParamsFieldSchema struct {
	Name          string   `json:"name"`
	JSONTypes     []string `json:"jsonTypes"`
	Required      bool     `json:"required"`
	MaxLength     int      `json:"maxLength,omitempty"`
	LengthUnit    string   `json:"lengthUnit,omitempty"`
	Pattern       string   `json:"pattern,omitempty"`
	Format        string   `json:"format,omitempty"`
	Normalization string   `json:"normalization,omitempty"`
	Description   string   `json:"description,omitempty"`
	Default       any      `json:"default,omitempty"`
	Enum          []any    `json:"enum,omitempty"`
	Minimum       *int     `json:"minimum,omitempty"`
	Maximum       *int     `json:"maximum,omitempty"`
}

type rawParamsObjectSchema struct {
	Type                 string                 `json:"type"`
	MaximumBytes         int                    `json:"maximumBytes"`
	AdditionalProperties bool                   `json:"additionalProperties"`
	ConflictRule         string                 `json:"conflictRule"`
	OutputControls       []string               `json:"outputControls"`
	Fields               []rawParamsFieldSchema `json:"fields"`
}

type JSONTypeField struct {
	Name     string         `json:"name"`
	Required bool           `json:"required"`
	Schema   JSONTypeSchema `json:"schema"`
}

type JSONTypeSchema struct {
	Type                 string          `json:"type"`
	Nullable             bool            `json:"nullable,omitempty"`
	Properties           []JSONTypeField `json:"properties,omitempty"`
	Items                *JSONTypeSchema `json:"items,omitempty"`
	Values               *JSONTypeSchema `json:"values,omitempty"`
	AdditionalProperties bool            `json:"additionalProperties,omitempty"`
}

type optionSchema struct {
	Name             string `json:"name"`
	Alias            string `json:"alias,omitempty"`
	Type             string `json:"type"`
	MaxLength        int    `json:"maxLength,omitempty"`
	LengthUnit       string `json:"lengthUnit,omitempty"`
	MaximumPaths     int    `json:"maximumPaths,omitempty"`
	MaximumPathDepth int    `json:"maximumPathDepth,omitempty"`
	Pattern          string `json:"pattern,omitempty"`
	Format           string `json:"format,omitempty"`
	Normalization    string `json:"normalization,omitempty"`
	Description      string `json:"description,omitempty"`
	Default          any    `json:"default,omitempty"`
	Enum             []any  `json:"enum,omitempty"`
	Minimum          *int   `json:"minimum,omitempty"`
	Maximum          *int   `json:"maximum,omitempty"`
}

type outputSchema struct {
	Formats                     []string       `json:"formats"`
	DefaultFormat               string         `json:"defaultFormat"`
	MaximumProviderPayloadBytes int            `json:"maximumProviderPayloadBytes,omitempty"`
	MaximumJSONOutputBytes      int            `json:"maximumJSONOutputBytes,omitempty"`
	ItemLimits                  map[string]int `json:"itemLimits,omitempty"`
	ContextRules                []string       `json:"contextRules,omitempty"`
}

type conditionalResponseSchema struct {
	When          string         `json:"when"`
	SchemaVersion string         `json:"schemaVersion,omitempty"`
	Response      JSONTypeSchema `json:"response"`
}

type effectsSchema struct {
	Network         bool   `json:"network"`
	Mutation        string `json:"mutation"`
	ExternalProcess bool   `json:"externalProcess"`
}

type dryRunResponseSchema struct {
	SchemaVersion string        `json:"schemaVersion"`
	Executable    string        `json:"executable"`
	Arguments     []string      `json:"arguments"`
	Executes      bool          `json:"executes"`
	Effects       effectsSchema `json:"effects"`
}

type commandDefinition struct {
	Path                   []string
	Summary                string
	ReadOnly               bool
	AgentInvocable         bool
	OutputContractRevision int
	Positionals            []positionalSchema
	Options                map[string]optionSpec
	Output                 outputSchema
	CredentialEnv          string
	Passthrough            bool
	Examples               []string
	ResponseType           reflect.Type
}

type commandSchemaDocument struct {
	SchemaVersion         string                      `json:"schemaVersion"`
	EnvelopeSchemaVersion string                      `json:"envelopeSchemaVersion,omitempty"`
	OutputContractVersion string                      `json:"outputContractVersion,omitempty"`
	CLIVersion            string                      `json:"cliVersion"`
	Path                  []string                    `json:"path"`
	Summary               string                      `json:"summary"`
	ReadOnly              bool                        `json:"readOnly"`
	AgentInvocable        bool                        `json:"agentInvocable"`
	Effects               effectsSchema               `json:"effects"`
	Positionals           []positionalSchema          `json:"positionals,omitempty"`
	Options               []optionSchema              `json:"options"`
	Output                outputSchema                `json:"output"`
	CredentialEnv         string                      `json:"credentialEnv,omitempty"`
	ErrorCodes            []string                    `json:"errorCodes"`
	Examples              []string                    `json:"examples,omitempty"`
	Params                *rawParamsObjectSchema      `json:"params,omitempty"`
	Response              JSONTypeSchema              `json:"response"`
	ConditionalResponses  []conditionalResponseSchema `json:"conditionalResponses,omitempty"`
}

type commandIndexEntry struct {
	Path                  []string      `json:"path"`
	OutputContractVersion string        `json:"outputContractVersion,omitempty"`
	Summary               string        `json:"summary"`
	ReadOnly              bool          `json:"readOnly"`
	AgentInvocable        bool          `json:"agentInvocable"`
	Effects               effectsSchema `json:"effects"`
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
			return render.CompactJSON(os.Stdout, schemaDocument(definition))
		}
	}
	matches := make([]commandIndexEntry, 0)
	for _, definition := range definitions {
		if hasStringPrefix(definition.Path, path) {
			entry := commandIndexEntry{
				Path: definition.Path, Summary: definition.Summary, ReadOnly: definition.ReadOnly,
				AgentInvocable: definition.AgentInvocable, Effects: effectsFor(definition),
			}
			if !definition.Passthrough {
				entry.OutputContractVersion = outputContractVersionFor(definition)
			}
			matches = append(matches, entry)
		}
	}
	if len(matches) == 0 {
		display := strings.Join(path, " ")
		if display == "" {
			display = "root"
		}
		return provider.NewError("invalid_arguments", fmt.Sprintf("unknown schema path: %s", display), 2)
	}
	return render.CompactJSON(os.Stdout, commandIndexDocument{
		SchemaVersion: "mwx.command-index/v1", CLIVersion: version, Prefix: path, Commands: matches,
	})
}

func normalizeSchemaPath(path []string) []string {
	result := append([]string{}, path...)
	if len(result) == 1 && strings.Contains(result[0], ".") {
		result = strings.Split(result[0], ".")
	}
	return result
}

func schemaDocument(definition commandDefinition) commandSchemaDocument {
	options := make(map[string]optionSpec, len(definition.Options)+len(globalOptions))
	if !definition.Passthrough {
		for name, spec := range globalOptions {
			options[name] = spec
		}
		options["params"] = rawParamsSchemaOption
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
	positionals := append([]positionalSchema(nil), definition.Positionals...)
	for index := range positionals {
		if positionals[index].Type == "" {
			positionals[index].Type = "string"
		}
		if positionals[index].MaxLength > 0 {
			positionals[index].LengthUnit = "utf8Bytes"
		}
	}
	document := commandSchemaDocument{
		SchemaVersion: "mwx.command-schema/v1", CLIVersion: version, Path: definition.Path,
		Summary: definition.Summary, ReadOnly: definition.ReadOnly, AgentInvocable: definition.AgentInvocable,
		Effects: effectsFor(definition), Positionals: positionals, Options: optionDocuments,
		Output: definition.Output, CredentialEnv: definition.CredentialEnv,
		ErrorCodes: []string{"invalid_arguments", "invalid_request", "internal_error", "not_configured", "authentication_failed", "plan_required", "rate_limited", "not_found", "http_error", "provider_unavailable", "provider_response_too_large", "output_too_large", "invalid_provider_response", "UPSTREAM_SCHEMA_MISMATCH", "INTERNAL_OUTPUT_CONTRACT_MISMATCH", "network_error", "timeout", "upstream_failed"},
		Examples:   append([]string(nil), definition.Examples...),
		Response:   JSONTypeSchema{Type: "passthrough"},
	}
	if !definition.Passthrough {
		document.EnvelopeSchemaVersion = agentSchemaVersion
		document.OutputContractVersion = outputContractVersionFor(definition)
		document.Params = describeRawParams(definition)
		document.Response = describeJSONType(definition.ResponseType, 0)
	} else {
		document.ConditionalResponses = []conditionalResponseSchema{{
			When:          "--dry-run",
			SchemaVersion: "mwx.dry-run/v1",
			Response:      describeJSONType(reflect.TypeOf(dryRunResponseSchema{}), 0),
		}}
	}
	return document
}

func effectsFor(definition commandDefinition) effectsSchema {
	if definition.Passthrough {
		return effectsSchema{Network: true, Mutation: "unknown", ExternalProcess: true}
	}
	network := definition.Path[0] != "providers"
	return effectsSchema{Network: network, Mutation: "none"}
}

func describeOption(name string, spec optionSpec) optionSchema {
	document := optionSchema{
		Name: "--" + name, MaxLength: spec.maxLength, MaximumPaths: spec.maxPaths, MaximumPathDepth: spec.maxPathDepth, Pattern: spec.pattern, Format: spec.format,
		Normalization: spec.normalize, Description: spec.description,
	}
	if document.MaxLength > 0 {
		document.LengthUnit = "utf8Bytes"
	}
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

func describeRawParams(definition commandDefinition) *rawParamsObjectSchema {
	document := &rawParamsObjectSchema{
		Type: "object", MaximumBytes: maximumRawParamsBytes, AdditionalProperties: false,
		ConflictRule:   "cannot be combined with positional arguments or convenience input flags",
		OutputControls: []string{"--json", "--fields", "--require-fields", "--compact"},
	}
	for _, positional := range definition.Positionals {
		types := []string{"string"}
		if positional.Variadic {
			types = append(types, "array<string>")
		}
		lengthUnit := positional.LengthUnit
		if positional.MaxLength > 0 && lengthUnit == "" {
			lengthUnit = "utf8Bytes"
		}
		document.Fields = append(document.Fields, rawParamsFieldSchema{
			Name: positional.Name, JSONTypes: types, Required: positional.Required, MaxLength: positional.MaxLength,
			LengthUnit: lengthUnit, Pattern: positional.Pattern, Format: positional.Format, Normalization: positional.Normalization,
			Description: positional.Description,
		})
	}
	names := make([]string, 0, len(definition.Options))
	for name := range definition.Options {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		option := describeOption(name, definition.Options[name])
		document.Fields = append(document.Fields, rawParamsFieldSchema{
			Name: name, JSONTypes: []string{option.Type}, Description: option.Description,
			MaxLength: option.MaxLength, LengthUnit: option.LengthUnit, Pattern: option.Pattern, Format: option.Format, Normalization: option.Normalization,
			Default: option.Default, Enum: option.Enum, Minimum: option.Minimum, Maximum: option.Maximum,
		})
	}
	return document
}

func describeJSONType(valueType reflect.Type, depth int) JSONTypeSchema {
	if valueType == nil || depth > 10 {
		return JSONTypeSchema{Type: "any", AdditionalProperties: true}
	}
	nullable := false
	for valueType.Kind() == reflect.Pointer {
		nullable = true
		valueType = valueType.Elem()
	}
	schema := JSONTypeSchema{Nullable: nullable}
	switch valueType.Kind() {
	case reflect.Struct:
		schema.Type = "object"
		for index := 0; index < valueType.NumField(); index++ {
			field := valueType.Field(index)
			if !field.IsExported() {
				continue
			}
			name := field.Name
			required := true
			if rawTag := field.Tag.Get("json"); rawTag != "" {
				parts := strings.Split(rawTag, ",")
				if parts[0] == "-" {
					continue
				}
				if parts[0] != "" {
					name = parts[0]
				}
				for _, option := range parts[1:] {
					if option == "omitempty" {
						required = false
					}
				}
			}
			schema.Properties = append(schema.Properties, JSONTypeField{Name: name, Required: required, Schema: describeJSONType(field.Type, depth+1)})
		}
	case reflect.Slice, reflect.Array:
		schema.Type = "array"
		item := describeJSONType(valueType.Elem(), depth+1)
		schema.Items = &item
	case reflect.Map:
		schema.Type = "object"
		schema.AdditionalProperties = true
		value := describeJSONType(valueType.Elem(), depth+1)
		schema.Values = &value
	case reflect.Interface:
		schema.Type = "any"
		schema.AdditionalProperties = true
	case reflect.Bool:
		schema.Type = "boolean"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		schema.Type = "integer"
	case reflect.Float32, reflect.Float64:
		schema.Type = "number"
	case reflect.String:
		schema.Type = "string"
	default:
		schema.Type = "any"
	}
	return schema
}

func commandDefinitions() []commandDefinition {
	providerJSON := outputSchema{
		Formats: []string{"text", "json"}, DefaultFormat: "text", MaximumProviderPayloadBytes: provider.MaximumProviderResponseBytes, MaximumJSONOutputBytes: maximumJSONOutputBytes,
		ContextRules: []string{"--fields, --require-fields, and --compact require JSON output", "field paths are relative to envelope.data", "use --require-fields for task-critical optional data", "use --compact for single-line JSON", "bound provider-specific list and time-window options"},
	}
	marketSearchJSON := providerJSON
	marketSearchJSON.ItemLimits = map[string]int{"events": 50, "events.markets": provider.MaximumMarketsPerEvent}
	orderBookJSON := outputSchema{Formats: []string{"json"}, DefaultFormat: "json", MaximumProviderPayloadBytes: provider.MaximumProviderResponseBytes, MaximumJSONOutputBytes: maximumJSONOutputBytes, ContextRules: providerJSON.ContextRules, ItemLimits: map[string]int{"book.bids": provider.MaximumOrderBookLevels, "book.asks": provider.MaximumOrderBookLevels}}
	metarJSON := providerJSON
	metarJSON.ItemLimits = map[string]int{"stations": 50}
	polyweatherJSON := providerJSON
	polyweatherJSON.ItemLimits = map[string]int{"markets.events": 10, "markets.events.markets": provider.MaximumMarketsPerEvent}
	definitions := []commandDefinition{
		{Path: []string{"betmoar", "search"}, Summary: "Search public Polymarket markets", ReadOnly: true, AgentInvocable: true, OutputContractRevision: 1, Positionals: []positionalSchema{{Name: "query", Required: true, Variadic: true, MaxLength: 512, Normalization: "trim", Description: "Market search query"}}, Options: betmoarOptions["search"], Output: marketSearchJSON, Examples: []string{`betmoar search "highest temperature" --limit 5 --json`}, ResponseType: reflect.TypeOf(provider.MarketSearchResult{})},
		{Path: []string{"betmoar", "book"}, Summary: "Fetch a public Polymarket order book", ReadOnly: true, AgentInvocable: true, OutputContractRevision: 1, Positionals: []positionalSchema{{Name: "token-id", Required: true, MaxLength: 128, Pattern: decimalTokenSchemaPattern, Normalization: "trim", Description: "Decimal Polymarket token identifier"}}, Options: betmoarOptions["book"], Output: orderBookJSON, Examples: []string{"betmoar book 123456789 --json"}, ResponseType: reflect.TypeOf(provider.OrderBookResult{})},
		{Path: []string{"betmoar", "upstream"}, Summary: "Delegate arguments to the official Polymarket CLI", ReadOnly: false, AgentInvocable: false, Positionals: []positionalSchema{{Name: "arguments", Variadic: true, Description: "May mutate or trade; requires an explicit -- boundary and user review"}}, Options: upstreamOptions, Output: outputSchema{Formats: []string{"passthrough", "json"}, DefaultFormat: "passthrough"}, Passthrough: true, Examples: []string{"betmoar upstream --dry-run -- markets search bitcoin --limit 5"}},
		{Path: []string{"metar"}, Summary: "Fetch NOAA aviation observations", ReadOnly: true, AgentInvocable: true, OutputContractRevision: 1, Positionals: []positionalSchema{{Name: "station", Required: true, Variadic: true, Pattern: stationListSchemaPattern, Normalization: "split on commas, trim, uppercase", Description: "Three or four character ICAO station identifier"}}, Options: metarOptions, Output: metarJSON, Examples: []string{"metar KSFO KJFK --hours 2 --json"}, ResponseType: reflect.TypeOf(provider.METARResult{})},
		{Path: []string{"open-meteo"}, Summary: "Resolve a location and fetch weather forecasts", ReadOnly: true, AgentInvocable: true, OutputContractRevision: 1, Positionals: []positionalSchema{{Name: "location", Required: true, Variadic: true, MaxLength: 256, Normalization: "join with spaces, trim"}}, Options: openMeteoOptions, Output: providerJSON, Examples: []string{`open-meteo "San Francisco" --days 3 --json`}, ResponseType: reflect.TypeOf(provider.ForecastResult{})},
		{Path: []string{"polyweather"}, Summary: "Combine METAR, forecast, and market data", ReadOnly: true, AgentInvocable: true, OutputContractRevision: 1, Positionals: []positionalSchema{{Name: "station", Required: true, MaxLength: 4, Pattern: stationSchemaPattern, Normalization: "trim, uppercase"}, {Name: "city", Variadic: true, MaxLength: 256, Normalization: "join with spaces, trim"}}, Options: polyweatherOptions, Output: polyweatherJSON, Examples: []string{`polyweather KSFO "San Francisco" --limit 3 --json`}, ResponseType: reflect.TypeOf(polyweatherResult{})},
		{Path: []string{"meteoblue"}, Summary: "Fetch a meteoblue forecast package", ReadOnly: true, AgentInvocable: true, OutputContractRevision: 1, Positionals: []positionalSchema{{Name: "location", Required: true, Variadic: true, MaxLength: 256, Normalization: "join with spaces, trim"}}, Options: meteoblueOptions, Output: providerJSON, CredentialEnv: "METEOBLUE_API_KEY", Examples: []string{`meteoblue "San Francisco" --json`}, ResponseType: reflect.TypeOf(provider.MeteoblueResult{})},
		{Path: []string{"wunderground"}, Summary: "Fetch Weather Underground PWS observations", ReadOnly: true, AgentInvocable: true, OutputContractRevision: 1, Positionals: []positionalSchema{{Name: "station-id", Required: true, MaxLength: 64, Pattern: pwsStationSchemaPattern, Normalization: "trim, uppercase"}}, Options: wundergroundOptions, Output: providerJSON, CredentialEnv: "WEATHER_COMPANY_API_KEY", Examples: []string{"wunderground KMAHANOV10 --units e --json"}, ResponseType: reflect.TypeOf(provider.WundergroundResult{})},
		{Path: []string{"providers"}, Summary: "Inspect provider and credential readiness", ReadOnly: true, AgentInvocable: true, OutputContractRevision: 1, Options: map[string]optionSpec{}, Output: providerJSON, Examples: []string{"mwx providers --json"}, ResponseType: reflect.TypeOf(providersResult{})},
	}
	wethrSummaries := map[string]string{
		"obs": "Fetch Wethr observations", "extreme": "Fetch Wethr temperature extremes", "forecast": "Fetch Wethr model forecasts",
		"precipitation": "Fetch Wethr precipitation data", "nws": "Fetch Wethr NWS forecasts", "pacing": "Fetch Wethr model pacing",
		"accuracy": "Fetch Wethr model accuracy", "nearby": "Find nearby Wethr stations",
	}
	wethrExamples := map[string]string{
		"obs": "wethr obs KSFO --json", "extreme": "wethr extreme KSFO --logic nws --json", "forecast": "wethr forecast KSFO --model HRRR --json",
		"precipitation": "wethr precipitation KSFO --json", "nws": "wethr nws KSFO --json", "pacing": "wethr pacing KSFO --models GFS,HRRR --json",
		"accuracy": "wethr accuracy KSFO --window 30d --json", "nearby": "wethr nearby KSFO --radius 50 --json",
	}
	wethrNames := make([]string, 0, len(wethrOptions))
	for name := range wethrOptions {
		wethrNames = append(wethrNames, name)
	}
	sort.Strings(wethrNames)
	for _, name := range wethrNames {
		definitions = append(definitions, commandDefinition{
			Path: []string{"wethr", name}, Summary: wethrSummaries[name], ReadOnly: true, AgentInvocable: true, OutputContractRevision: 1,
			Positionals: []positionalSchema{{Name: "station", Required: true, MaxLength: 4, Pattern: stationSchemaPattern, Normalization: "trim, uppercase"}}, Options: wethrOptions[name], Output: providerJSON, CredentialEnv: "WETHR_API_KEY", Examples: []string{wethrExamples[name]}, ResponseType: reflect.TypeOf(provider.WethrResult{}),
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
