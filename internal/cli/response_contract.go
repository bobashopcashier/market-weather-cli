package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/bobashopcashier/market-weather-cli/internal/provider"
)

type outputTypeMismatch struct {
	Field    string `json:"field"`
	Expected string `json:"expected"`
	Actual   string `json:"actual"`
}

func preflightResponseFields(tool string, argv []string) error {
	path := []string{tool}
	rest := argv
	jsonDefault := false
	if tool == "betmoar" || tool == "wethr" {
		if len(argv) == 0 {
			return nil
		}
		path = append(path, argv[0])
		rest = argv[1:]
		jsonDefault = tool == "betmoar" && argv[0] == "book"
	}
	definition, ok := definitionByName(commandName(path))
	if !ok || definition.Passthrough {
		return nil
	}
	parsed, err := parseArgs(rest, definition.Options, jsonDefault)
	if err != nil {
		return err
	}
	unknown := make([]string, 0)
	for _, option := range []string{"fields", "require-fields"} {
		if parsed.value(option) == "" {
			continue
		}
		_, paths, err := parseFieldMask(parsed.value(option))
		if err != nil {
			return err
		}
		for _, path := range paths {
			declared, definitive := declaredFieldPath(definition.ResponseType, strings.Split(path, "."))
			if definitive && !declared {
				unknown = append(unknown, path)
			}
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		unknown = compactStrings(unknown)
		return provider.NewError("invalid_arguments", fmt.Sprintf("field(s) not declared by %s: %s", commandName(definition.Path), strings.Join(unknown, ", ")), 2)
	}
	return nil
}

func validateRequiredFieldOptions(parsed parsedArgs) error {
	required := parsed.value("require-fields")
	if required == "" {
		return nil
	}
	_, requiredPaths, err := parseFieldMask(required)
	if err != nil {
		return err
	}
	if projected := parsed.value("fields"); projected != "" {
		_, projectedPaths, err := parseFieldMask(projected)
		if err != nil {
			return err
		}
		uncovered := make([]string, 0)
		for _, requiredPath := range requiredPaths {
			covered := false
			for _, projectedPath := range projectedPaths {
				if requiredPath == projectedPath || strings.HasPrefix(requiredPath, projectedPath+".") {
					covered = true
					break
				}
			}
			if !covered {
				uncovered = append(uncovered, requiredPath)
			}
		}
		if len(uncovered) > 0 {
			return provider.NewError("invalid_arguments", fmt.Sprintf("--require-fields paths are not covered by --fields: %s", strings.Join(uncovered, ", ")), 2)
		}
	}
	return nil
}

func validateCommandOutput(command string, value any) (any, error) {
	document, err := jsonDocument(value)
	if err != nil {
		return nil, provider.NewError("internal_error", "could not prepare the command output contract", 10)
	}
	definition, ok := definitionByName(command)
	if !ok || definition.Passthrough {
		return nil, provider.NewError("internal_error", "command output contract is not registered", 10)
	}
	missing := make([]string, 0)
	types := make([]outputTypeMismatch, 0)
	validateDocumentSchema(document, describeJSONType(definition.ResponseType, 0), "", &missing, &types)
	if len(missing) > 0 || len(types) > 0 {
		sort.Strings(missing)
		sort.Slice(types, func(i, j int) bool { return types[i].Field < types[j].Field })
		paths := append([]string(nil), missing...)
		for _, mismatch := range types {
			paths = append(paths, mismatch.Field)
		}
		sort.Strings(paths)
		paths = compactStrings(paths)
		return nil, &provider.Error{
			Code: "INTERNAL_OUTPUT_CONTRACT_MISMATCH", Message: "CLI output does not satisfy its declared contract", Hint: "Affected JSON paths: " + strings.Join(paths, ", "), ExitCode: 10,
			Details: map[string]any{"outputContractVersion": outputContractVersion(command), "missingFields": missing, "typeMismatches": types},
		}
	}
	return document, nil
}

func validateTaskRequiredFields(command string, document any, raw string) error {
	if raw == "" {
		return nil
	}
	_, paths, err := parseFieldMask(raw)
	if err != nil {
		return err
	}
	missing := make([]string, 0)
	for _, path := range paths {
		validateRequiredPath(document, strings.Split(path, "."), "", &missing)
	}
	sort.Strings(missing)
	missing = compactStrings(missing)
	if len(missing) == 0 {
		return nil
	}
	return &provider.Error{
		Code: "UPSTREAM_SCHEMA_MISMATCH", Message: "provider response is missing task-required data", Hint: "Affected JSON paths: " + strings.Join(missing, ", "), ExitCode: 6,
		Details: map[string]any{"outputContractVersion": outputContractVersion(command), "missingFields": missing},
	}
}

func jsonDocument(value any) (any, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var document any
	if err := decoder.Decode(&document); err != nil {
		return nil, err
	}
	return document, nil
}

func definitionByName(name string) (commandDefinition, bool) {
	for _, definition := range commandDefinitions() {
		if commandName(definition.Path) == name {
			return definition, true
		}
	}
	return commandDefinition{}, false
}

func validateRequiredPath(value any, segments []string, prefix string, missing *[]string) {
	if len(segments) == 0 {
		if value == nil {
			*missing = append(*missing, prefix)
		}
		return
	}
	switch typed := value.(type) {
	case []any:
		arrayPrefix := prefix + "[]"
		for _, item := range typed {
			validateRequiredPath(item, segments, arrayPrefix, missing)
		}
	case map[string]any:
		path := segments[0]
		if prefix != "" {
			path = prefix + "." + path
		}
		next, ok := typed[segments[0]]
		if !ok || next == nil {
			*missing = append(*missing, path)
			return
		}
		validateRequiredPath(next, segments[1:], path, missing)
	default:
		path := strings.Join(segments, ".")
		if prefix != "" {
			path = prefix + "." + path
		}
		*missing = append(*missing, path)
	}
}

func validateDocumentSchema(value any, schema JSONTypeSchema, path string, missing *[]string, mismatches *[]outputTypeMismatch) {
	if value == nil {
		if !schema.Nullable && schema.Type != "any" && schema.Type != "passthrough" {
			*mismatches = append(*mismatches, outputTypeMismatch{Field: displayPath(path), Expected: schema.Type, Actual: "null"})
		}
		return
	}
	if !jsonTypeMatches(value, schema.Type) {
		*mismatches = append(*mismatches, outputTypeMismatch{Field: displayPath(path), Expected: schema.Type, Actual: jsonTypeName(value)})
		return
	}
	switch schema.Type {
	case "object":
		object, ok := value.(map[string]any)
		if !ok {
			return
		}
		for _, property := range schema.Properties {
			childPath := joinContractPath(path, property.Name)
			child, exists := object[property.Name]
			if !exists {
				if property.Required {
					*missing = append(*missing, childPath)
				}
				continue
			}
			validateDocumentSchema(child, property.Schema, childPath, missing, mismatches)
		}
		if schema.Values != nil {
			keys := make([]string, 0, len(object))
			for key := range object {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				validateDocumentSchema(object[key], *schema.Values, joinContractPath(path, key), missing, mismatches)
			}
		}
	case "array":
		items, ok := value.([]any)
		if !ok || schema.Items == nil {
			return
		}
		for _, item := range items {
			validateDocumentSchema(item, *schema.Items, path+"[]", missing, mismatches)
		}
	case "passthrough", "any":
		return
	}
}

func jsonTypeMatches(value any, expected string) bool {
	switch expected {
	case "any", "passthrough":
		return true
	case "object":
		_, ok := value.(map[string]any)
		return ok
	case "array":
		_, ok := value.([]any)
		return ok
	case "string":
		_, ok := value.(string)
		return ok
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "number":
		_, ok := value.(json.Number)
		return ok
	case "integer":
		number, ok := value.(json.Number)
		if !ok {
			return false
		}
		_, err := number.Int64()
		return err == nil
	default:
		return true
	}
}

func jsonTypeName(value any) string {
	if value == nil {
		return "null"
	}
	switch typed := value.(type) {
	case map[string]any:
		return "object"
	case []any:
		return "array"
	case string:
		return "string"
	case bool:
		return "boolean"
	case json.Number:
		if _, err := typed.Int64(); err == nil {
			return "integer"
		}
		return "number"
	default:
		return reflect.TypeOf(value).String()
	}
}

func joinContractPath(prefix, name string) string {
	if prefix == "" {
		return name
	}
	return prefix + "." + name
}

func displayPath(path string) string {
	if path == "" {
		return "$"
	}
	return path
}

func compactStrings(values []string) []string {
	if len(values) < 2 {
		return values
	}
	out := values[:1]
	for _, value := range values[1:] {
		if value != out[len(out)-1] {
			out = append(out, value)
		}
	}
	return out
}
