package dataframe

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// FillMethod controls how FillNA chooses the replacement value.
type FillMethod string

const (
	// FillLiteral replaces nulls with the supplied literal value.
	FillLiteral FillMethod = "literal"
	// FillMean replaces nulls with the arithmetic mean of non-null values.
	FillMean FillMethod = "mean"
	// FillMode replaces nulls with the most frequent non-null value.
	FillMode FillMethod = "mode"
)

// Head returns the first n rows. A negative n returns all but the final -n
// rows, matching pandas head semantics.
func Head(frame Frame, n int) Frame {
	end := n
	if n < 0 {
		end = len(frame.Rows) + n
	}
	end = clamp(end, 0, len(frame.Rows))
	return sliceFrame(frame, 0, end, 0, len(frame.Columns))
}

// Tail returns the final n rows. A negative n returns all but the first -n
// rows, matching pandas tail semantics.
func Tail(frame Frame, n int) Frame {
	start := len(frame.Rows) - n
	if n < 0 {
		start = -n
	}
	start = clamp(start, 0, len(frame.Rows))
	return sliceFrame(frame, start, len(frame.Rows), 0, len(frame.Columns))
}

// SelectColumns returns the named columns in the requested order.
func SelectColumns(frame Frame, columns []string) (Frame, error) {
	indexes, err := frame.ColumnIndexes(columns)
	if err != nil {
		return Frame{}, err
	}
	rows := make([][]any, len(frame.Rows))
	for rowIndex, row := range frame.Rows {
		rows[rowIndex] = make([]any, len(indexes))
		for outputIndex, inputIndex := range indexes {
			rows[rowIndex][outputIndex] = row[inputIndex]
		}
	}
	return New(columns, rows)
}

// SelectDTypes returns columns whose inferred type matches include and does
// not match exclude. Supported selectors include number, int, float, string,
// object, bool, null, and mixed, plus common pandas-style aliases.
func SelectDTypes(frame Frame, include, exclude []string) (Frame, error) {
	if len(include) == 0 && len(exclude) == 0 {
		return Frame{}, fmt.Errorf("select dtypes requires include or exclude")
	}
	includeSet, err := normalizeDTypes(include)
	if err != nil {
		return Frame{}, err
	}
	excludeSet, err := normalizeDTypes(exclude)
	if err != nil {
		return Frame{}, err
	}
	selected := make([]string, 0, len(frame.Columns))
	for columnIndex, column := range frame.Columns {
		dtype := detailedColumnType(frame, columnIndex)
		if len(includeSet) > 0 && !dtypeMatchesAny(dtype, includeSet) {
			continue
		}
		if dtypeMatchesAny(dtype, excludeSet) {
			continue
		}
		selected = append(selected, column)
	}
	return SelectColumns(frame, selected)
}

// Cast converts every non-null value in column to dtype. Supported target
// types are int, float, string, and bool, including common aliases.
func Cast(frame Frame, column, dtype string) (Frame, error) {
	columnIndex, err := frame.ColumnIndex(column)
	if err != nil {
		return Frame{}, err
	}
	target, err := normalizeCastType(dtype)
	if err != nil {
		return Frame{}, err
	}
	result := frame.Clone()
	for rowIndex, row := range result.Rows {
		if IsNull(row[columnIndex]) {
			row[columnIndex] = nil
			continue
		}
		converted, convertErr := castValue(row[columnIndex], target)
		if convertErr != nil {
			return Frame{}, fmt.Errorf("cast %s row %d to %s: %w", column, rowIndex, target, convertErr)
		}
		row[columnIndex] = converted
	}
	return result, nil
}

// ValueCounts returns distinct values and their counts, ordered by descending
// count and then by first appearance. Nulls are omitted when dropNA is true.
func ValueCounts(frame Frame, column string, dropNA bool) (Frame, error) {
	columnIndex, err := frame.ColumnIndex(column)
	if err != nil {
		return Frame{}, err
	}
	type countEntry struct {
		value any
		count int64
		first int
	}
	entries := make([]countEntry, 0)
	entryByKey := make(map[string]int)
	for rowIndex, row := range frame.Rows {
		value := row[columnIndex]
		if dropNA && IsNull(value) {
			continue
		}
		key := ValueKey(value)
		if entryIndex, exists := entryByKey[key]; exists {
			entries[entryIndex].count++
			continue
		}
		entryByKey[key] = len(entries)
		entries = append(entries, countEntry{value: normalizedNull(value), count: 1, first: rowIndex})
	}
	sort.SliceStable(entries, func(left, right int) bool {
		if entries[left].count != entries[right].count {
			return entries[left].count > entries[right].count
		}
		return entries[left].first < entries[right].first
	})
	rows := make([][]any, len(entries))
	for index, entry := range entries {
		rows[index] = []any{entry.value, entry.count}
	}
	return New([]string{"value", "count"}, rows)
}

// Unique returns distinct column values in order of first appearance. Null is
// retained as one distinct value.
func Unique(frame Frame, column string) ([]any, error) {
	columnIndex, err := frame.ColumnIndex(column)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{})
	values := make([]any, 0)
	for _, row := range frame.Rows {
		key := ValueKey(row[columnIndex])
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		values = append(values, normalizedNull(row[columnIndex]))
	}
	return values, nil
}

// NUnique returns the number of distinct column values. Null is omitted when
// dropNA is true.
func NUnique(frame Frame, column string, dropNA bool) (int, error) {
	values, err := Unique(frame, column)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, value := range values {
		if dropNA && IsNull(value) {
			continue
		}
		count++
	}
	return count, nil
}

// NullMask returns a boolean frame indicating null values. When notNull is
// true, the mask is inverted and indicates non-null values.
func NullMask(frame Frame, notNull bool) Frame {
	rows := make([][]any, len(frame.Rows))
	for rowIndex, row := range frame.Rows {
		rows[rowIndex] = make([]any, len(row))
		for columnIndex, value := range row {
			isNull := IsNull(value)
			if notNull {
				isNull = !isNull
			}
			rows[rowIndex][columnIndex] = isNull
		}
	}
	result, _ := New(frame.Columns, rows)
	return result
}

// DuplicatedMask marks duplicate rows using columns as the key. An empty
// column list uses all columns. keep accepts "first", "last", or "none".
func DuplicatedMask(frame Frame, columns []string, keep string) ([]bool, error) {
	indexes, err := duplicateIndexes(frame, columns)
	if err != nil {
		return nil, err
	}
	keep = strings.ToLower(strings.TrimSpace(keep))
	if keep == "" {
		keep = "first"
	}
	if keep != "first" && keep != "last" && keep != "none" {
		return nil, fmt.Errorf("keep must be first, last, or none")
	}
	mask := make([]bool, len(frame.Rows))
	groups := make(map[string][]int)
	for rowIndex, row := range frame.Rows {
		key := rowValueKey(row, indexes)
		groups[key] = append(groups[key], rowIndex)
	}
	for _, rowIndexes := range groups {
		if len(rowIndexes) < 2 {
			continue
		}
		for _, rowIndex := range rowIndexes {
			mask[rowIndex] = true
		}
		switch keep {
		case "first":
			mask[rowIndexes[0]] = false
		case "last":
			mask[rowIndexes[len(rowIndexes)-1]] = false
		}
	}
	return mask, nil
}

// DropDuplicates removes rows marked by DuplicatedMask.
func DropDuplicates(frame Frame, columns []string, keep string) (Frame, error) {
	mask, err := DuplicatedMask(frame, columns, keep)
	if err != nil {
		return Frame{}, err
	}
	rows := make([][]any, 0, len(frame.Rows))
	for rowIndex, row := range frame.Rows {
		if !mask[rowIndex] {
			rows = append(rows, row)
		}
	}
	return New(frame.Columns, rows)
}

// Rename returns a frame with columns renamed according to mapping. Mapping
// keys must exist and the resulting names must remain unique and non-empty.
func Rename(frame Frame, mapping map[string]string) (Frame, error) {
	columns := append([]string(nil), frame.Columns...)
	for oldName, newName := range mapping {
		columnIndex, err := frame.ColumnIndex(oldName)
		if err != nil {
			return Frame{}, err
		}
		columns[columnIndex] = newName
	}
	return New(columns, frame.Rows)
}

// MapColumn replaces values in column using their string representation as
// mapping keys. Unmapped values become null unless keepUnmapped is true.
func MapColumn(frame Frame, column string, mapping map[string]any, keepUnmapped bool) (Frame, error) {
	columnIndex, err := frame.ColumnIndex(column)
	if err != nil {
		return Frame{}, err
	}
	result := frame.Clone()
	for _, row := range result.Rows {
		key := fmt.Sprint(row[columnIndex])
		if IsNull(row[columnIndex]) {
			key = "null"
		}
		if replacement, exists := mapping[key]; exists {
			row[columnIndex] = replacement
		} else if !keepUnmapped {
			row[columnIndex] = nil
		}
	}
	return result, nil
}

// IsIn returns a row mask indicating whether column values occur in values.
func IsIn(frame Frame, column string, values []any) ([]bool, error) {
	columnIndex, err := frame.ColumnIndex(column)
	if err != nil {
		return nil, err
	}
	accepted := make(map[string]struct{}, len(values))
	for _, value := range values {
		accepted[ValueKey(value)] = struct{}{}
	}
	mask := make([]bool, len(frame.Rows))
	for rowIndex, row := range frame.Rows {
		_, mask[rowIndex] = accepted[ValueKey(row[columnIndex])]
	}
	return mask, nil
}

// DropColumns removes the named columns.
func DropColumns(frame Frame, columns []string) (Frame, error) {
	dropped := make(map[int]struct{}, len(columns))
	for _, column := range columns {
		columnIndex, err := frame.ColumnIndex(column)
		if err != nil {
			return Frame{}, err
		}
		dropped[columnIndex] = struct{}{}
	}
	remaining := make([]string, 0, len(frame.Columns)-len(dropped))
	for columnIndex, column := range frame.Columns {
		if _, exists := dropped[columnIndex]; !exists {
			remaining = append(remaining, column)
		}
	}
	return SelectColumns(frame, remaining)
}

// FillNA replaces null values in column using a literal, the numeric mean, or
// the first most-frequent value. literal is used only with FillLiteral.
func FillNA(frame Frame, column string, method FillMethod, literal any) (Frame, error) {
	columnIndex, err := frame.ColumnIndex(column)
	if err != nil {
		return Frame{}, err
	}
	replacement := literal
	switch method {
	case FillLiteral:
	case FillMean:
		total, count := 0.0, 0
		for rowIndex, row := range frame.Rows {
			if IsNull(row[columnIndex]) {
				continue
			}
			number, ok := Float(row[columnIndex])
			if !ok {
				return Frame{}, fmt.Errorf("mean fill requires numeric %s value at row %d", column, rowIndex)
			}
			total += number
			count++
		}
		if count == 0 {
			return Frame{}, fmt.Errorf("mean fill requires at least one non-null value in %s", column)
		}
		replacement = total / float64(count)
	case FillMode:
		counts, countErr := ValueCounts(frame, column, true)
		if countErr != nil {
			return Frame{}, countErr
		}
		if len(counts.Rows) == 0 {
			return Frame{}, fmt.Errorf("mode fill requires at least one non-null value in %s", column)
		}
		replacement = counts.Rows[0][0]
	default:
		return Frame{}, fmt.Errorf("unknown fill method: %s", method)
	}
	result := frame.Clone()
	for _, row := range result.Rows {
		if IsNull(row[columnIndex]) {
			row[columnIndex] = replacement
		}
	}
	return result, nil
}

// DropNA removes rows with nulls in columns. An empty column list examines all
// columns. how accepts "any" or "all".
func DropNA(frame Frame, columns []string, how string) (Frame, error) {
	indexes, err := frame.ColumnIndexes(columns)
	if err != nil {
		return Frame{}, err
	}
	if len(columns) == 0 {
		indexes = make([]int, len(frame.Columns))
		for index := range indexes {
			indexes[index] = index
		}
	}
	how = strings.ToLower(strings.TrimSpace(how))
	if how == "" {
		how = "any"
	}
	if how != "any" && how != "all" {
		return Frame{}, fmt.Errorf("how must be any or all")
	}
	rows := make([][]any, 0, len(frame.Rows))
	for _, row := range frame.Rows {
		nullCount := 0
		for _, columnIndex := range indexes {
			if IsNull(row[columnIndex]) {
				nullCount++
			}
		}
		drop := nullCount > 0
		if how == "all" {
			drop = len(indexes) > 0 && nullCount == len(indexes)
		}
		if !drop {
			rows = append(rows, row)
		}
	}
	return New(frame.Columns, rows)
}

// SortValues stably sorts rows by columns. ascending may contain one value for
// all columns or one per column. Null placement is controlled independently by
// naLast.
func SortValues(frame Frame, columns []string, ascending []bool, naLast bool) (Frame, error) {
	if len(columns) == 0 {
		return Frame{}, fmt.Errorf("sort requires at least one column")
	}
	indexes, err := frame.ColumnIndexes(columns)
	if err != nil {
		return Frame{}, err
	}
	if len(ascending) == 0 {
		ascending = []bool{true}
	}
	if len(ascending) != 1 && len(ascending) != len(columns) {
		return Frame{}, fmt.Errorf("ascending must contain one value or one per sort column")
	}
	rows := make([][]any, len(frame.Rows))
	for index, row := range frame.Rows {
		rows[index] = append([]any(nil), row...)
	}
	sort.SliceStable(rows, func(left, right int) bool {
		for keyIndex, columnIndex := range indexes {
			leftValue, rightValue := rows[left][columnIndex], rows[right][columnIndex]
			leftNull, rightNull := IsNull(leftValue), IsNull(rightValue)
			if leftNull || rightNull {
				if leftNull && rightNull {
					continue
				}
				if naLast {
					return !leftNull
				}
				return leftNull
			}
			comparison := Compare(leftValue, rightValue)
			if comparison == 0 {
				continue
			}
			isAscending := ascending[0]
			if len(ascending) > 1 {
				isAscending = ascending[keyIndex]
			}
			if isAscending {
				return comparison < 0
			}
			return comparison > 0
		}
		return false
	})
	return New(frame.Columns, rows)
}

// ILoc returns half-open positional row and column slices. Negative bounds are
// interpreted relative to the corresponding length, and bounds are clamped.
func ILoc(frame Frame, rowStart, rowEnd, columnStart, columnEnd int) Frame {
	rowStart, rowEnd = normalizeSlice(rowStart, rowEnd, len(frame.Rows))
	columnStart, columnEnd = normalizeSlice(columnStart, columnEnd, len(frame.Columns))
	return sliceFrame(frame, rowStart, rowEnd, columnStart, columnEnd)
}

// ToNumpy returns a row-major matrix with row slices copied from the frame.
func ToNumpy(frame Frame) [][]any {
	rows := make([][]any, len(frame.Rows))
	for index, row := range frame.Rows {
		rows[index] = make([]any, len(row))
		for columnIndex, value := range row {
			rows[index][columnIndex] = jsonSafeScalar(value)
		}
	}
	return rows
}

func normalizeCastType(dtype string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(dtype)) {
	case "int", "int64", "integer":
		return "int", nil
	case "float", "float64", "number", "numeric":
		return "float", nil
	case "str", "string", "object":
		return "string", nil
	case "bool", "boolean":
		return "bool", nil
	default:
		return "", fmt.Errorf("unsupported cast type: %s", dtype)
	}
}

func castValue(value any, dtype string) (any, error) {
	switch dtype {
	case "string":
		return fmt.Sprint(value), nil
	case "int":
		switch typed := value.(type) {
		case bool:
			if typed {
				return int64(1), nil
			}
			return int64(0), nil
		case int:
			return int64(typed), nil
		case int64:
			return typed, nil
		case json.Number:
			if integer, err := typed.Int64(); err == nil {
				return integer, nil
			}
		case string:
			trimmed := strings.TrimSpace(typed)
			if !strings.ContainsAny(trimmed, ".eE") {
				if integer, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
					return integer, nil
				}
			}
			if number, parseErr := strconv.ParseFloat(trimmed, 64); parseErr == nil {
				return finiteInt64(number, value)
			}
		}
		number, ok := Float(value)
		if !ok {
			return nil, fmt.Errorf("%v is not a finite int64", value)
		}
		return finiteInt64(number, value)
	case "float":
		if boolean, ok := value.(bool); ok {
			if boolean {
				return float64(1), nil
			}
			return float64(0), nil
		}
		number, ok := Float(value)
		if text, isString := value.(string); isString {
			var parseErr error
			number, parseErr = strconv.ParseFloat(strings.TrimSpace(text), 64)
			ok = parseErr == nil && !math.IsNaN(number) && !math.IsInf(number, 0)
		}
		if !ok {
			return nil, fmt.Errorf("%v is not numeric", value)
		}
		return number, nil
	case "bool":
		if boolean, ok := Bool(value); ok {
			return boolean, nil
		}
		if text, isString := value.(string); isString {
			if number, parseErr := strconv.ParseFloat(strings.TrimSpace(text), 64); parseErr == nil && !math.IsNaN(number) && !math.IsInf(number, 0) {
				return number != 0, nil
			}
		}
		if number, ok := Float(value); ok {
			return number != 0, nil
		}
		return nil, fmt.Errorf("%v is not boolean", value)
	default:
		return nil, fmt.Errorf("unsupported cast type: %s", dtype)
	}
}

func finiteInt64(number float64, original any) (any, error) {
	const maxInt64Exclusive = 9223372036854775808.0
	const minInt64Inclusive = -9223372036854775808.0
	if math.IsNaN(number) || math.IsInf(number, 0) || number >= maxInt64Exclusive || number < minInt64Inclusive {
		return nil, fmt.Errorf("%v is not a finite int64", original)
	}
	return int64(number), nil
}

func normalizeDTypes(values []string) (map[string]struct{}, error) {
	result := make(map[string]struct{})
	for _, value := range values {
		var normalized string
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "number", "numeric":
			normalized = "number"
		case "int", "int64", "integer":
			normalized = "int"
		case "float", "float64", "double":
			normalized = "float"
		case "str", "string":
			normalized = "string"
		case "object":
			normalized = "object"
		case "bool", "boolean":
			normalized = "bool"
		case "null", "nil", "na":
			normalized = "null"
		case "mixed":
			normalized = "mixed"
		default:
			return nil, fmt.Errorf("unsupported dtype selector: %s", value)
		}
		result[normalized] = struct{}{}
	}
	return result, nil
}

func dtypeMatchesAny(dtype string, selectors map[string]struct{}) bool {
	for selector := range selectors {
		if selector == dtype || selector == "number" && (dtype == "int" || dtype == "float") || selector == "object" && (dtype == "string" || dtype == "mixed" || dtype == "object") {
			return true
		}
	}
	return false
}

func detailedColumnType(frame Frame, columnIndex int) string {
	hasInt, hasFloat, hasString, hasBool, hasOther := false, false, false, false, false
	for _, row := range frame.Rows {
		value := row[columnIndex]
		if IsNull(value) {
			continue
		}
		switch typed := value.(type) {
		case bool:
			hasBool = true
		case string:
			hasString = true
		case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
			hasInt = true
		case float32, float64:
			hasFloat = true
		case json.Number:
			if strings.ContainsAny(string(typed), ".eE") {
				hasFloat = true
			} else {
				hasInt = true
			}
		default:
			hasOther = true
		}
	}
	count := 0
	for _, present := range []bool{hasInt || hasFloat, hasString, hasBool, hasOther} {
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
	case hasFloat:
		return "float"
	case hasInt:
		return "int"
	case hasString:
		return "string"
	case hasBool:
		return "bool"
	default:
		return "object"
	}
}

func duplicateIndexes(frame Frame, columns []string) ([]int, error) {
	if len(columns) > 0 {
		return frame.ColumnIndexes(columns)
	}
	indexes := make([]int, len(frame.Columns))
	for index := range indexes {
		indexes[index] = index
	}
	return indexes, nil
}

func rowValueKey(row []any, indexes []int) string {
	var builder strings.Builder
	for _, index := range indexes {
		key := ValueKey(row[index])
		builder.WriteString(strconv.Itoa(len(key)))
		builder.WriteByte(':')
		builder.WriteString(key)
	}
	return builder.String()
}

func normalizedNull(value any) any {
	if IsNull(value) {
		return nil
	}
	return value
}

func normalizeSlice(start, end, length int) (int, int) {
	if start < 0 {
		start += length
	}
	if end < 0 {
		end += length
	}
	start = clamp(start, 0, length)
	end = clamp(end, 0, length)
	if end < start {
		end = start
	}
	return start, end
}

func sliceFrame(frame Frame, rowStart, rowEnd, columnStart, columnEnd int) Frame {
	columns := append([]string(nil), frame.Columns[columnStart:columnEnd]...)
	rows := make([][]any, rowEnd-rowStart)
	for index, row := range frame.Rows[rowStart:rowEnd] {
		rows[index] = append([]any(nil), row[columnStart:columnEnd]...)
	}
	result, _ := New(columns, rows)
	return result
}

func clamp(value, minimum, maximum int) int {
	if value < minimum {
		return minimum
	}
	if value > maximum {
		return maximum
	}
	return value
}
