package provider

import (
	"encoding/json"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

var (
	jsonNumberType     = reflect.TypeOf(json.Number(""))
	jsonRawMessageType = reflect.TypeOf(json.RawMessage(nil))
)

type schemaTypeMismatch struct {
	Path     string `json:"path"`
	Expected string `json:"expected"`
	Actual   string `json:"actual"`
}

type schemaValidationResult struct {
	missingFields  []string
	typeMismatches []schemaTypeMismatch
}

type schemaValidationCollector struct {
	missingFields  map[string]struct{}
	typeMismatches map[string]schemaTypeMismatch
}

func validateJSONShape(document any, target any) schemaValidationResult {
	collector := newSchemaValidationCollector()
	targetType := reflect.TypeOf(target)
	if targetType != nil && targetType.Kind() == reflect.Pointer {
		targetType = targetType.Elem()
	}
	collector.validate(document, targetType, "")
	return collector.result()
}

func newSchemaValidationCollector() *schemaValidationCollector {
	return &schemaValidationCollector{
		missingFields:  map[string]struct{}{},
		typeMismatches: map[string]schemaTypeMismatch{},
	}
}

func (collector *schemaValidationCollector) result() schemaValidationResult {
	result := schemaValidationResult{
		missingFields:  make([]string, 0, len(collector.missingFields)),
		typeMismatches: make([]schemaTypeMismatch, 0, len(collector.typeMismatches)),
	}
	for path := range collector.missingFields {
		result.missingFields = append(result.missingFields, path)
	}
	for _, mismatch := range collector.typeMismatches {
		result.typeMismatches = append(result.typeMismatches, mismatch)
	}
	sort.Strings(result.missingFields)
	sort.Slice(result.typeMismatches, func(i, j int) bool {
		left, right := result.typeMismatches[i], result.typeMismatches[j]
		if left.Path != right.Path {
			return left.Path < right.Path
		}
		if left.Expected != right.Expected {
			return left.Expected < right.Expected
		}
		return left.Actual < right.Actual
	})
	return result
}

func (result schemaValidationResult) valid() bool {
	return len(result.missingFields) == 0 && len(result.typeMismatches) == 0
}

func (collector *schemaValidationCollector) validate(value any, targetType reflect.Type, path string) {
	if targetType == nil {
		return
	}
	for targetType.Kind() == reflect.Pointer {
		if value == nil {
			return
		}
		targetType = targetType.Elem()
	}
	if targetType == jsonRawMessageType || targetType.Kind() == reflect.Interface {
		return
	}
	if targetType == jsonNumberType {
		if _, ok := value.(json.Number); !ok {
			collector.addTypeMismatch(path, "number", jsonValueType(value))
		}
		return
	}

	switch targetType.Kind() {
	case reflect.Struct:
		object, ok := value.(map[string]any)
		if !ok {
			collector.addTypeMismatch(path, "object", jsonValueType(value))
			return
		}
		collector.validateStruct(object, targetType, path)
	case reflect.Map:
		object, ok := value.(map[string]any)
		if !ok {
			collector.addTypeMismatch(path, "object", jsonValueType(value))
			return
		}
		keys := make([]string, 0, len(object))
		for key := range object {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			collector.validate(object[key], targetType.Elem(), wildcardPath(path))
		}
	case reflect.Slice, reflect.Array:
		if targetType.Kind() == reflect.Slice && targetType.Elem().Kind() == reflect.Uint8 {
			if _, ok := value.(string); !ok {
				collector.addTypeMismatch(path, "string", jsonValueType(value))
			}
			return
		}
		array, ok := value.([]any)
		if !ok {
			collector.addTypeMismatch(path, "array", jsonValueType(value))
			return
		}
		for _, item := range array {
			collector.validate(item, targetType.Elem(), arrayPath(path))
		}
	case reflect.String:
		if _, ok := value.(string); !ok {
			collector.addTypeMismatch(path, "string", jsonValueType(value))
		}
	case reflect.Bool:
		if _, ok := value.(bool); !ok {
			collector.addTypeMismatch(path, "boolean", jsonValueType(value))
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		number, ok := value.(json.Number)
		if !ok || !validSignedInteger(number, targetType.Bits()) {
			collector.addTypeMismatch(path, "integer", jsonValueType(value))
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		number, ok := value.(json.Number)
		if !ok || !validUnsignedInteger(number, targetType.Bits()) {
			collector.addTypeMismatch(path, "unsigned integer", jsonValueType(value))
		}
	case reflect.Float32, reflect.Float64:
		number, ok := value.(json.Number)
		if !ok || !validFloat(number, targetType.Bits()) {
			collector.addTypeMismatch(path, "number", jsonValueType(value))
		}
	}
}

func (collector *schemaValidationCollector) validateStruct(object map[string]any, targetType reflect.Type, path string) {
	for index := 0; index < targetType.NumField(); index++ {
		field := targetType.Field(index)
		if !field.IsExported() {
			continue
		}
		name, omitEmpty, ignored := jsonField(field)
		if ignored {
			continue
		}
		fieldPath := objectPath(path, name)
		value, present := object[name]
		if !present {
			if !omitEmpty {
				collector.missingFields[fieldPath] = struct{}{}
			}
			continue
		}
		collector.validate(value, field.Type, fieldPath)
	}
}

func (collector *schemaValidationCollector) addTypeMismatch(path, expected, actual string) {
	if path == "" {
		path = "$"
	}
	mismatch := schemaTypeMismatch{Path: path, Expected: expected, Actual: actual}
	key := strings.Join([]string{mismatch.Path, mismatch.Expected, mismatch.Actual}, "\x00")
	collector.typeMismatches[key] = mismatch
}

func (collector *schemaValidationCollector) addMissingField(path string) {
	collector.missingFields[path] = struct{}{}
}

func jsonField(field reflect.StructField) (name string, omitEmpty bool, ignored bool) {
	name = field.Name
	parts := strings.Split(field.Tag.Get("json"), ",")
	if parts[0] == "-" {
		return "", false, true
	}
	if parts[0] != "" {
		name = parts[0]
	}
	for _, option := range parts[1:] {
		if option == "omitempty" {
			omitEmpty = true
		}
	}
	return name, omitEmpty, false
}

func objectPath(parent, field string) string {
	if parent == "" {
		return field
	}
	return parent + "." + field
}

func arrayPath(parent string) string {
	if parent == "" {
		return "[]"
	}
	return parent + "[]"
}

func wildcardPath(parent string) string {
	if parent == "" {
		return "*"
	}
	return parent + ".*"
}

func jsonValueType(value any) string {
	switch value.(type) {
	case nil:
		return "null"
	case map[string]any:
		return "object"
	case []any:
		return "array"
	case string:
		return "string"
	case bool:
		return "boolean"
	case json.Number:
		return "number"
	default:
		return "unknown"
	}
}

func validSignedInteger(number json.Number, bits int) bool {
	if number == "" {
		return false
	}
	_, err := strconv.ParseInt(string(number), 10, bits)
	return err == nil
}

func validUnsignedInteger(number json.Number, bits int) bool {
	if number == "" {
		return false
	}
	_, err := strconv.ParseUint(string(number), 10, bits)
	return err == nil
}

func validFloat(number json.Number, bits int) bool {
	if number == "" {
		return false
	}
	_, err := strconv.ParseFloat(string(number), bits)
	return err == nil
}
