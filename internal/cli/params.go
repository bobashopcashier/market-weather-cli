package cli

import (
	"encoding/json"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/bobashopcashier/market-weather-cli/internal/provider"
)

const maximumRawParamsBytes = 64 << 10

var rawParamsSchemaOption = optionSpec{
	kind:        stringOption,
	maxLength:   maximumRawParamsBytes,
	description: "Invoke the command from one JSON object; cannot be combined with convenience inputs",
}

func expandRawParams(tool string, argv []string) ([]string, error) {
	path := []string{tool}
	prefix := []string{}
	rest := argv
	if tool == "betmoar" || tool == "wethr" {
		if len(argv) == 0 {
			return argv, nil
		}
		path = append(path, argv[0])
		prefix = append(prefix, argv[0])
		rest = argv[1:]
	}
	var definition *commandDefinition
	for _, candidate := range commandDefinitions() {
		if equalStrings(candidate.Path, path) {
			copy := candidate
			definition = &copy
			break
		}
	}
	if definition == nil || definition.Passthrough {
		return argv, nil
	}
	raw, remaining, found, err := extractRawParams(rest)
	if err != nil || !found {
		return argv, err
	}
	outputArgs, err := rawParamsOutputArgs(remaining)
	if err != nil {
		return nil, err
	}
	values, err := decodeRawParams(raw)
	if err != nil {
		return nil, err
	}
	expanded, err := rawParamsArguments(*definition, values)
	if err != nil {
		return nil, err
	}
	boundary := len(expanded)
	for index, argument := range expanded {
		if argument == "--" {
			boundary = index
			break
		}
	}
	result := append(prefix, expanded[:boundary]...)
	result = append(result, outputArgs...)
	result = append(result, expanded[boundary:]...)
	if err := rejectArgumentControls(result); err != nil {
		return nil, err
	}
	return result, nil
}

func extractRawParams(argv []string) (string, []string, bool, error) {
	remaining := make([]string, 0, len(argv))
	found := false
	raw := ""
	for index := 0; index < len(argv); index++ {
		argument := argv[index]
		if argument == "--params" {
			if found || index+1 >= len(argv) {
				return "", nil, false, provider.NewError("invalid_arguments", "--params must be supplied exactly once with a JSON object", 2)
			}
			found = true
			index++
			raw = argv[index]
			continue
		}
		if strings.HasPrefix(argument, "--params=") {
			if found {
				return "", nil, false, provider.NewError("invalid_arguments", "--params must be supplied exactly once", 2)
			}
			found = true
			raw = strings.TrimPrefix(argument, "--params=")
			continue
		}
		remaining = append(remaining, argument)
	}
	return raw, remaining, found, nil
}

func rawParamsOutputArgs(argv []string) ([]string, error) {
	result := make([]string, 0, len(argv))
	for index := 0; index < len(argv); index++ {
		switch argument := argv[index]; {
		case argument == "--json" || argument == "-j" || argument == "--compact":
			result = append(result, argument)
		case argument == "--fields" || argument == "--require-fields":
			if index+1 >= len(argv) {
				return nil, provider.NewError("invalid_arguments", "missing value for "+argument, 2)
			}
			result = append(result, argument, argv[index+1])
			index++
		case strings.HasPrefix(argument, "--fields=") || strings.HasPrefix(argument, "--require-fields="):
			result = append(result, argument)
		default:
			return nil, provider.NewError("invalid_arguments", "--params cannot be combined with positional arguments or convenience input flags", 2)
		}
	}
	return result, nil
}

func decodeRawParams(raw string) (map[string]json.RawMessage, error) {
	if len(raw) == 0 || len(raw) > maximumRawParamsBytes {
		return nil, provider.NewError("invalid_arguments", "--params must contain a JSON object no larger than 65536 bytes", 2)
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, provider.NewError("invalid_arguments", "--params must contain one JSON object", 2)
	}
	values := map[string]json.RawMessage{}
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return nil, provider.NewError("invalid_arguments", "--params contains invalid JSON", 2)
		}
		key, ok := keyToken.(string)
		if !ok {
			return nil, provider.NewError("invalid_arguments", "--params contains a non-string field name", 2)
		}
		if _, exists := values[key]; exists {
			return nil, provider.NewError("invalid_arguments", "--params contains a duplicate field", 2)
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, provider.NewError("invalid_arguments", "--params contains invalid JSON", 2)
		}
		values[key] = value
	}
	if _, err := decoder.Token(); err != nil {
		return nil, provider.NewError("invalid_arguments", "--params contains invalid JSON", 2)
	}
	if token, err := decoder.Token(); err != io.EOF || token != nil {
		return nil, provider.NewError("invalid_arguments", "--params must contain exactly one JSON object", 2)
	}
	return values, nil
}

func rawParamsArguments(definition commandDefinition, values map[string]json.RawMessage) ([]string, error) {
	positionals := []string{}
	for _, positional := range definition.Positionals {
		raw, exists := values[positional.Name]
		if !exists {
			continue
		}
		delete(values, positional.Name)
		if positional.Variadic {
			var list []string
			if err := json.Unmarshal(raw, &list); err == nil {
				positionals = append(positionals, list...)
				continue
			}
		}
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return nil, provider.NewError("invalid_arguments", "--params positional fields must be strings or string arrays when variadic", 2)
		}
		positionals = append(positionals, value)
	}
	result := []string{}
	optionNames := make([]string, 0, len(definition.Options))
	for name := range definition.Options {
		optionNames = append(optionNames, name)
	}
	sort.Strings(optionNames)
	for _, name := range optionNames {
		raw, exists := values[name]
		if !exists {
			continue
		}
		delete(values, name)
		spec := definition.Options[name]
		switch spec.kind {
		case boolOption:
			var value bool
			if err := json.Unmarshal(raw, &value); err != nil {
				return nil, provider.NewError("invalid_arguments", "--params contains a boolean option with the wrong type", 2)
			}
			if value {
				result = append(result, "--"+name)
			}
		case intOption:
			var number json.Number
			if err := json.Unmarshal(raw, &number); err != nil {
				return nil, provider.NewError("invalid_arguments", "--params contains an integer option with the wrong type", 2)
			}
			value, err := strconv.Atoi(number.String())
			if err != nil {
				return nil, provider.NewError("invalid_arguments", "--params integer options must be whole numbers", 2)
			}
			result = append(result, "--"+name, strconv.Itoa(value))
		default:
			var value string
			if err := json.Unmarshal(raw, &value); err != nil {
				return nil, provider.NewError("invalid_arguments", "--params contains a string option with the wrong type", 2)
			}
			result = append(result, "--"+name, value)
		}
	}
	if len(values) > 0 {
		return nil, provider.NewError("invalid_arguments", "--params contains an unknown field", 2)
	}
	if len(positionals) > 0 {
		result = append(result, "--")
		result = append(result, positionals...)
	}
	return result, nil
}
