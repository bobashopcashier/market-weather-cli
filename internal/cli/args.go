package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/bobashopcashier/market-weather-cli/internal/provider"
)

type optionKind int

const (
	boolOption optionKind = iota
	stringOption
	intOption
)

type optionSpec struct {
	kind        optionKind
	alias       string
	defaultVal  string
	choices     []string
	min         int
	max         int
	maxLength   int
	pattern     string
	format      string
	normalize   string
	description string
}

type parsedArgs struct {
	positionals []string
	values      map[string]string
	bools       map[string]bool
}

var globalOptions = map[string]optionSpec{
	"help":    {kind: boolOption, alias: "h", description: "Show command help"},
	"json":    {kind: boolOption, alias: "j", description: "Emit stable machine-readable JSON"},
	"fields":  {kind: stringOption, maxLength: maximumFieldMaskBytes, description: "Project JSON output using comma-separated field paths"},
	"compact": {kind: boolOption, description: "Emit single-line JSON"},
}

func parseArgs(argv []string, commandSpec map[string]optionSpec, jsonDefault ...bool) (parsedArgs, error) {
	spec := map[string]optionSpec{}
	aliases := map[string]string{}
	for name, value := range globalOptions {
		spec[name] = value
	}
	for name, value := range commandSpec {
		spec[name] = value
	}
	for name, value := range spec {
		if value.alias != "" {
			aliases[value.alias] = name
		}
	}
	parsed := parsedArgs{values: map[string]string{}, bools: map[string]bool{}}
	seenOptions := map[string]bool{}
	if len(jsonDefault) > 0 && jsonDefault[0] || strings.EqualFold(strings.TrimSpace(os.Getenv("MWX_OUTPUT")), "json") {
		parsed.bools["json"] = true
	}
	passthrough := false
	for index := 0; index < len(argv); index++ {
		token := argv[index]
		if passthrough || token == "-" || !strings.HasPrefix(token, "-") {
			parsed.positionals = append(parsed.positionals, token)
			continue
		}
		if token == "--" {
			passthrough = true
			continue
		}
		nameValue := strings.TrimPrefix(token, "-")
		if strings.HasPrefix(token, "--") {
			nameValue = strings.TrimPrefix(token, "--")
		}
		name, inlineValue, hasInline := strings.Cut(nameValue, "=")
		if canonical, ok := aliases[name]; ok {
			name = canonical
		}
		option, ok := spec[name]
		if !ok {
			return parsed, provider.NewError("invalid_arguments", fmt.Sprintf("unknown option: %s", token), 2)
		}
		if option.kind == boolOption {
			if hasInline {
				return parsed, provider.NewError("invalid_arguments", fmt.Sprintf("--%s does not take a value", name), 2)
			}
			if seenOptions[name] {
				return parsed, provider.NewError("invalid_arguments", fmt.Sprintf("duplicate option: --%s", name), 2)
			}
			seenOptions[name] = true
			parsed.bools[name] = true
			continue
		}
		value := inlineValue
		if !hasInline {
			index++
			if index >= len(argv) {
				return parsed, provider.NewError("invalid_arguments", fmt.Sprintf("missing value for --%s", name), 2)
			}
			value = argv[index]
		}
		if option.kind == intOption {
			number, err := strconv.Atoi(value)
			if err != nil || (option.min != 0 && number < option.min) || (option.max != 0 && number > option.max) {
				return parsed, provider.NewError("invalid_arguments", fmt.Sprintf("invalid value for --%s: %s", name, value), 2)
			}
		}
		if len(option.choices) > 0 && !contains(option.choices, value) {
			return parsed, provider.NewError("invalid_arguments", fmt.Sprintf("--%s must be one of: %s", name, strings.Join(option.choices, ", ")), 2)
		}
		if seenOptions[name] {
			return parsed, provider.NewError("invalid_arguments", fmt.Sprintf("duplicate option: --%s", name), 2)
		}
		seenOptions[name] = true
		parsed.values[name] = value
	}
	for name, option := range commandSpec {
		if _, ok := parsed.values[name]; !ok && option.defaultVal != "" {
			parsed.values[name] = option.defaultVal
		}
	}
	if (parsed.value("fields") != "" || parsed.flag("compact")) && !parsed.flag("json") {
		return parsed, provider.NewError("invalid_arguments", "--fields and --compact require JSON output; add --json or set MWX_OUTPUT=json", 2)
	}
	return parsed, nil
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func (p parsedArgs) flag(name string) bool { return p.bools[name] }

func (p parsedArgs) value(name string) string { return p.values[name] }

func (p parsedArgs) integer(name string) int {
	value, _ := strconv.Atoi(p.values[name])
	return value
}

func required(values []string, index int, label string) (string, error) {
	if index >= len(values) || strings.TrimSpace(values[index]) == "" {
		err := provider.NewError("invalid_arguments", fmt.Sprintf("missing %s", label), 2)
		err.Hint = "Run the command with --help for examples."
		return "", err
	}
	return values[index], nil
}

func rejectExtraPositionals(values []string, maximum int, usage string) error {
	if len(values) <= maximum {
		return nil
	}
	err := provider.NewError("invalid_arguments", fmt.Sprintf("too many positional arguments for %s", usage), 2)
	err.Hint = "Run the command with --help for the accepted arguments."
	return err
}
