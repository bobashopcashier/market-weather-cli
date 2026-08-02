package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strings"

	"github.com/bobashopcashier/market-weather-cli/internal/provider"
	"github.com/bobashopcashier/market-weather-cli/internal/render"
)

const (
	maximumFieldMaskBytes  = 1024
	maximumFieldPaths      = 64
	maximumFieldPathDepth  = 8
	maximumJSONOutputBytes = 8 << 20
)

var fieldSegmentPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

type fieldMaskNode map[string]fieldMaskNode

func writeJSON(parsed parsedArgs, command string, value any) error {
	document, err := validateCommandOutput(command, value)
	if err != nil {
		return err
	}
	if err := validateTaskRequiredFields(command, document, parsed.value("require-fields")); err != nil {
		return err
	}
	if mask := parsed.value("fields"); mask != "" {
		projected, err := projectJSONFields(value, mask)
		if err != nil {
			return err
		}
		value = projected
	}
	value = agentEnvelope{
		SchemaVersion: agentSchemaVersion, OutputContractVersion: outputContractVersion(command),
		OK: true, Command: command, Data: value,
	}
	var output bytes.Buffer
	if parsed.flag("compact") {
		if err := render.CompactJSON(&output, value); err != nil {
			return err
		}
	} else if err := render.JSON(&output, value); err != nil {
		return err
	}
	if output.Len() > maximumJSONOutputBytes {
		err := provider.NewError("output_too_large", "JSON output exceeded the 8388608-byte context safety limit", 1)
		err.Hint = "Retry with --fields, --compact, and smaller provider-specific bounds."
		return err
	}
	_, err = output.WriteTo(os.Stdout)
	return err
}

func writeSafeHumanJSON(prefix string, value any) error {
	var output bytes.Buffer
	// Prefix formatting is CLI-owned; callers sanitize any interpolated values.
	output.WriteString(prefix)
	if err := render.SafeJSON(&output, value); err != nil {
		return err
	}
	if output.Len() > maximumJSONOutputBytes {
		return provider.NewError("output_too_large", "human output exceeded the 8388608-byte safety limit", 1)
	}
	_, err := output.WriteTo(os.Stdout)
	return err
}

func projectJSONFields(value any, rawMask string) (any, error) {
	root, paths, err := parseFieldMask(rawMask)
	if err != nil {
		return nil, err
	}

	var document any
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, provider.NewError("internal_error", "could not prepare JSON field projection", 1)
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	if err := decoder.Decode(&document); err != nil {
		return nil, provider.NewError("internal_error", "could not decode JSON field projection", 1)
	}
	for _, rawPath := range paths {
		segments := strings.Split(rawPath, ".")
		declared, definitive := declaredFieldPath(reflect.TypeOf(value), segments)
		if definitive && !declared || !definitive && !fieldPathExists(document, segments) {
			return nil, provider.NewError("invalid_arguments", fmt.Sprintf("JSON field path not found: %s", rawPath), 2)
		}
	}
	projected, ok := projectJSONNode(document, root)
	if !ok {
		return nil, provider.NewError("invalid_arguments", "--fields requires an object or array-of-objects JSON response", 2)
	}
	return projected, nil
}

func parseFieldMask(rawMask string) (fieldMaskNode, []string, error) {
	if len(rawMask) > maximumFieldMaskBytes {
		return nil, nil, provider.NewError("invalid_arguments", "--fields exceeds the 1024-byte limit", 2)
	}
	root := fieldMaskNode{}
	rawPaths := strings.Split(rawMask, ",")
	if len(rawPaths) > maximumFieldPaths {
		return nil, nil, provider.NewError("invalid_arguments", fmt.Sprintf("--fields accepts at most %d paths", maximumFieldPaths), 2)
	}
	paths := make([]string, 0, len(rawPaths))
	seen := map[string]bool{}
	for _, rawPath := range rawPaths {
		rawPath = strings.TrimSpace(rawPath)
		if rawPath == "" {
			return nil, nil, provider.NewError("invalid_arguments", "--fields must not be empty or contain an empty path", 2)
		}
		segments := strings.Split(rawPath, ".")
		if len(segments) > maximumFieldPathDepth {
			return nil, nil, provider.NewError("invalid_arguments", fmt.Sprintf("--fields path exceeds the %d-segment limit: %s", maximumFieldPathDepth, rawPath), 2)
		}
		current := root
		for _, segment := range segments {
			if !fieldSegmentPattern.MatchString(segment) {
				return nil, nil, provider.NewError("invalid_arguments", "--fields paths may contain only letters, numbers, underscores, hyphens, and dots", 2)
			}
		}
		if seen[rawPath] {
			return nil, nil, provider.NewError("invalid_arguments", fmt.Sprintf("duplicate --fields path: %s", rawPath), 2)
		}
		for _, existing := range paths {
			if strings.HasPrefix(rawPath, existing+".") || strings.HasPrefix(existing, rawPath+".") {
				return nil, nil, provider.NewError("invalid_arguments", fmt.Sprintf("overlapping --fields paths: %s and %s", existing, rawPath), 2)
			}
		}
		seen[rawPath] = true
		paths = append(paths, rawPath)
		for _, segment := range segments {
			if current[segment] == nil {
				current[segment] = fieldMaskNode{}
			}
			current = current[segment]
		}
	}
	return root, paths, nil
}

func fieldPathExists(value any, path []string) bool {
	if len(path) == 0 {
		return true
	}
	switch typed := value.(type) {
	case map[string]any:
		next, ok := typed[path[0]]
		return ok && fieldPathExists(next, path[1:])
	case []any:
		for _, item := range typed {
			if fieldPathExists(item, path) {
				return true
			}
		}
	}
	return false
}

func declaredFieldPath(valueType reflect.Type, path []string) (bool, bool) {
	return declaredFieldPathFrom(valueType, path, false)
}

func declaredFieldPathFrom(valueType reflect.Type, path []string, declared bool) (bool, bool) {
	for valueType != nil && valueType.Kind() == reflect.Pointer {
		valueType = valueType.Elem()
	}
	if len(path) == 0 {
		return true, true
	}
	if valueType == nil {
		return false, false
	}
	switch valueType.Kind() {
	case reflect.Struct:
		for index := 0; index < valueType.NumField(); index++ {
			field := valueType.Field(index)
			if !field.IsExported() {
				continue
			}
			name := field.Name
			if tag := strings.Split(field.Tag.Get("json"), ",")[0]; tag != "" {
				if tag == "-" {
					continue
				}
				name = tag
			}
			if name == path[0] {
				return declaredFieldPathFrom(field.Type, path[1:], true)
			}
		}
		return false, true
	case reflect.Slice, reflect.Array:
		return declaredFieldPathFrom(valueType.Elem(), path, declared)
	case reflect.Map:
		if !declared {
			return false, false
		}
		return declaredFieldPathFrom(valueType.Elem(), path[1:], true)
	case reflect.Interface:
		if !declared {
			return false, false
		}
		return true, true
	default:
		return false, true
	}
}

func projectJSONNode(value any, mask fieldMaskNode) (any, bool) {
	if len(mask) == 0 {
		return value, true
	}
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(mask))
		for key, childMask := range mask {
			child, exists := typed[key]
			if !exists {
				continue
			}
			if projected, ok := projectJSONNode(child, childMask); ok {
				result[key] = projected
			}
		}
		return result, true
	case []any:
		result := make([]any, 0, len(typed))
		for _, item := range typed {
			projected, ok := projectJSONNode(item, mask)
			if ok {
				result = append(result, projected)
			}
		}
		return result, true
	default:
		return nil, false
	}
}
