package dataframe

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// Frame is a small, ordered, in-memory table. Rows always have the same width
// as Columns, and nil is the missing-value representation.
type Frame struct {
	Columns []string `json:"columns"`
	Rows    [][]any  `json:"rows"`
}

func New(columns []string, rows [][]any) (Frame, error) {
	seen := make(map[string]struct{}, len(columns))
	for _, column := range columns {
		if strings.TrimSpace(column) == "" {
			return Frame{}, fmt.Errorf("column names cannot be empty")
		}
		if _, exists := seen[column]; exists {
			return Frame{}, fmt.Errorf("duplicate column: %s", column)
		}
		seen[column] = struct{}{}
	}
	result := Frame{Columns: append([]string(nil), columns...), Rows: make([][]any, len(rows))}
	for index, row := range rows {
		if len(row) != len(columns) {
			return Frame{}, fmt.Errorf("row %d has %d values, want %d", index, len(row), len(columns))
		}
		result.Rows[index] = append([]any(nil), row...)
	}
	return result, nil
}

func (f Frame) Clone() Frame {
	clone, _ := New(f.Columns, f.Rows)
	return clone
}

func (f Frame) ColumnIndex(name string) (int, error) {
	for index, column := range f.Columns {
		if column == name {
			return index, nil
		}
	}
	return -1, fmt.Errorf("unknown column: %s", name)
}

func (f Frame) ColumnIndexes(names []string) ([]int, error) {
	indexes := make([]int, len(names))
	for index, name := range names {
		columnIndex, err := f.ColumnIndex(name)
		if err != nil {
			return nil, err
		}
		indexes[index] = columnIndex
	}
	return indexes, nil
}

func (f Frame) RowMap(row []any) map[string]any {
	mapped := make(map[string]any, len(f.Columns))
	for index, column := range f.Columns {
		if index < len(row) {
			mapped[column] = row[index]
		}
	}
	return mapped
}

func (f Frame) Records() []map[string]any {
	records := make([]map[string]any, len(f.Rows))
	for index, row := range f.Rows {
		records[index] = f.RowMap(row)
	}
	return records
}

func IsNull(value any) bool {
	if value == nil {
		return true
	}
	switch typed := value.(type) {
	case float64:
		return math.IsNaN(typed)
	case float32:
		return math.IsNaN(float64(typed))
	default:
		return false
	}
}

func jsonSafeScalar(value any) any {
	if IsNull(value) {
		return nil
	}
	switch typed := value.(type) {
	case float64:
		if math.IsInf(typed, 0) {
			return nil
		}
	case float32:
		if math.IsInf(float64(typed), 0) {
			return nil
		}
	}
	return value
}

func Float(value any) (float64, bool) {
	if IsNull(value) {
		return 0, false
	}
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int8:
		return float64(typed), true
	case int16:
		return float64(typed), true
	case int32:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case uint:
		return float64(typed), true
	case uint8:
		return float64(typed), true
	case uint16:
		return float64(typed), true
	case uint32:
		return float64(typed), true
	case uint64:
		return float64(typed), true
	case json.Number:
		parsed, err := typed.Float64()
		return parsed, err == nil && !math.IsNaN(parsed) && !math.IsInf(parsed, 0)
	default:
		return 0, false
	}
}

func Bool(value any) (bool, bool) {
	if IsNull(value) {
		return false, false
	}
	switch typed := value.(type) {
	case bool:
		return typed, true
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(typed))
		return parsed, err == nil
	default:
		return false, false
	}
}

func ParseScalar(value string) any {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	if len(trimmed) >= 2 && ((trimmed[0] == '"' && trimmed[len(trimmed)-1] == '"') || (trimmed[0] == '`' && trimmed[len(trimmed)-1] == '`')) {
		if unquoted, err := strconv.Unquote(trimmed); err == nil {
			return unquoted
		}
	}
	if len(trimmed) >= 2 && trimmed[0] == '\'' && trimmed[len(trimmed)-1] == '\'' {
		return strings.ReplaceAll(strings.ReplaceAll(trimmed[1:len(trimmed)-1], `\'`, `'`), `\\`, `\`)
	}
	if strings.EqualFold(trimmed, "null") || strings.EqualFold(trimmed, "nil") || strings.EqualFold(trimmed, "na") {
		return nil
	}
	if parsed, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
		return parsed
	}
	if parsed, err := strconv.ParseFloat(trimmed, 64); err == nil {
		return parsed
	}
	if parsed, err := strconv.ParseBool(trimmed); err == nil {
		return parsed
	}
	return value
}

func ValueKey(value any) string {
	if IsNull(value) {
		return "null:"
	}
	switch typed := value.(type) {
	case bool:
		return "bool:" + strconv.FormatBool(typed)
	case string:
		return "string:" + typed
	default:
		if number, ok := Float(value); ok {
			return "number:" + strconv.FormatFloat(number, 'g', -1, 64)
		}
		encoded, _ := json.Marshal(value)
		return fmt.Sprintf("%T:%s", value, encoded)
	}
}

func Compare(left, right any) int {
	if IsNull(left) && IsNull(right) {
		return 0
	}
	if IsNull(left) {
		return 1
	}
	if IsNull(right) {
		return -1
	}
	if leftNumber, ok := Float(left); ok {
		if rightNumber, rightOK := Float(right); rightOK {
			switch {
			case leftNumber < rightNumber:
				return -1
			case leftNumber > rightNumber:
				return 1
			default:
				return 0
			}
		}
	}
	leftText, rightText := fmt.Sprint(left), fmt.Sprint(right)
	return strings.Compare(leftText, rightText)
}

func InferType(values []any) string {
	hasNumber, hasString, hasBool, hasOther := false, false, false, false
	for _, value := range values {
		if IsNull(value) {
			continue
		}
		switch value.(type) {
		case bool:
			hasBool = true
		case string:
			hasString = true
		default:
			if _, ok := Float(value); ok {
				hasNumber = true
			} else {
				hasOther = true
			}
		}
	}
	count := 0
	for _, present := range []bool{hasNumber, hasString, hasBool, hasOther} {
		if present {
			count++
		}
	}
	if count == 0 {
		return "null"
	}
	if count > 1 {
		return "mixed"
	}
	switch {
	case hasNumber:
		return "number"
	case hasString:
		return "string"
	case hasBool:
		return "bool"
	default:
		return "object"
	}
}

func (f Frame) ColumnTypes() []string {
	types := make([]string, len(f.Columns))
	for columnIndex := range f.Columns {
		values := make([]any, len(f.Rows))
		for rowIndex, row := range f.Rows {
			values[rowIndex] = row[columnIndex]
		}
		types[columnIndex] = InferType(values)
	}
	return types
}

func SplitList(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func SortedKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
