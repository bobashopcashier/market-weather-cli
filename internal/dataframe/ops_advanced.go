package dataframe

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

type ColumnInfo struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	NonNull  int    `json:"nonNull"`
	Null     int    `json:"null"`
	Distinct int    `json:"distinct"`
}

type InfoResult struct {
	Rows    int          `json:"rows"`
	Columns int          `json:"columns"`
	Schema  []ColumnInfo `json:"schema"`
}

func Info(frame Frame) InfoResult {
	types := frame.ColumnTypes()
	result := InfoResult{Rows: len(frame.Rows), Columns: len(frame.Columns), Schema: make([]ColumnInfo, len(frame.Columns))}
	for columnIndex, column := range frame.Columns {
		seen := map[string]struct{}{}
		nonNull := 0
		for _, row := range frame.Rows {
			value := row[columnIndex]
			if !IsNull(value) {
				nonNull++
				seen[ValueKey(value)] = struct{}{}
			}
		}
		result.Schema[columnIndex] = ColumnInfo{Name: column, Type: types[columnIndex], NonNull: nonNull, Null: len(frame.Rows) - nonNull, Distinct: len(seen)}
	}
	return result
}

func Describe(frame Frame, columns []string) (Frame, error) {
	if len(columns) == 0 {
		types := frame.ColumnTypes()
		for index, columnType := range types {
			if columnType == "number" {
				columns = append(columns, frame.Columns[index])
			}
		}
	}
	if len(columns) == 0 {
		return Frame{}, fmt.Errorf("describe requires at least one numeric column")
	}
	indexes, err := frame.ColumnIndexes(columns)
	if err != nil {
		return Frame{}, err
	}
	stats := []string{"count", "mean", "std", "min", "25%", "50%", "75%", "max"}
	rows := make([][]any, len(stats))
	for index, stat := range stats {
		rows[index] = []any{stat}
	}
	for _, columnIndex := range indexes {
		values := numericValues(frame, columnIndex)
		if len(values) == 0 {
			return Frame{}, fmt.Errorf("column %s has no numeric values", frame.Columns[columnIndex])
		}
		sort.Float64s(values)
		meanValue := mean(values)
		var std any
		if len(values) > 1 {
			variance := 0.0
			for _, value := range values {
				difference := value - meanValue
				variance += difference * difference
			}
			std = math.Sqrt(variance / float64(len(values)-1))
		}
		columnStats := []any{int64(len(values)), meanValue, std, values[0], quantile(values, .25), quantile(values, .5), quantile(values, .75), values[len(values)-1]}
		for rowIndex := range rows {
			rows[rowIndex] = append(rows[rowIndex], jsonSafeScalar(columnStats[rowIndex]))
		}
	}
	return New(append([]string{"stat"}, columns...), rows)
}

func Query(frame Frame, expression string) (Frame, error) {
	if strings.TrimSpace(expression) == "" {
		return Frame{}, fmt.Errorf("query expression cannot be empty")
	}
	rows := make([][]any, 0, len(frame.Rows))
	for index, row := range frame.Rows {
		matches, err := EvalPredicate(expression, frame.RowMap(row))
		if err != nil {
			return Frame{}, fmt.Errorf("query row %d: %w", index, err)
		}
		if matches {
			rows = append(rows, append([]any(nil), row...))
		}
	}
	return New(frame.Columns, rows)
}

func Loc(frame Frame, expression string, columns []string) (Frame, error) {
	selected := frame
	var err error
	if strings.TrimSpace(expression) != "" {
		selected, err = Query(frame, expression)
		if err != nil {
			return Frame{}, err
		}
	}
	if len(columns) > 0 {
		return SelectColumns(selected, columns)
	}
	return selected, nil
}

func Apply(frame Frame, expression, outputColumn string) (Frame, error) {
	if strings.TrimSpace(expression) == "" {
		return Frame{}, fmt.Errorf("apply expression cannot be empty")
	}
	if strings.TrimSpace(outputColumn) == "" {
		return Frame{}, fmt.Errorf("output column cannot be empty")
	}
	result := frame.Clone()
	outputIndex, err := result.ColumnIndex(outputColumn)
	if err != nil {
		outputIndex = len(result.Columns)
		result.Columns = append(result.Columns, outputColumn)
		for index := range result.Rows {
			result.Rows[index] = append(result.Rows[index], nil)
		}
	}
	for rowIndex := range result.Rows {
		value, evalErr := EvalExpression(expression, frame.RowMap(frame.Rows[rowIndex]))
		if evalErr != nil {
			return Frame{}, fmt.Errorf("apply row %d: %w", rowIndex, evalErr)
		}
		result.Rows[rowIndex][outputIndex] = value
	}
	return result, nil
}

type Aggregation struct {
	Column string `json:"column"`
	Func   string `json:"func"`
	As     string `json:"as,omitempty"`
}

func Aggregate(frame Frame, by []string, aggregations []Aggregation) (Frame, error) {
	return aggregate(frame, by, aggregations, false)
}

func AggregateIncludingNullGroups(frame Frame, by []string, aggregations []Aggregation) (Frame, error) {
	return aggregate(frame, by, aggregations, true)
}

func aggregate(frame Frame, by []string, aggregations []Aggregation, includeNullGroups bool) (Frame, error) {
	if len(aggregations) == 0 {
		return Frame{}, fmt.Errorf("at least one aggregation is required")
	}
	byIndexes, err := frame.ColumnIndexes(by)
	if err != nil {
		return Frame{}, err
	}
	aggregationIndexes := make([]int, len(aggregations))
	outputColumns := append([]string(nil), by...)
	seenOutput := map[string]struct{}{}
	for _, column := range outputColumns {
		seenOutput[column] = struct{}{}
	}
	for index, aggregation := range aggregations {
		columnIndex, columnErr := frame.ColumnIndex(aggregation.Column)
		if columnErr != nil {
			return Frame{}, columnErr
		}
		aggregationIndexes[index] = columnIndex
		name := aggregation.As
		if name == "" {
			name = aggregation.Column + "_" + strings.ToLower(aggregation.Func)
		}
		if _, exists := seenOutput[name]; exists {
			return Frame{}, fmt.Errorf("duplicate output column: %s", name)
		}
		seenOutput[name] = struct{}{}
		outputColumns = append(outputColumns, name)
	}
	type group struct {
		keys []any
		rows [][]any
	}
	groups := make([]group, 0)
	groupIndexes := map[string]int{}
	if len(by) == 0 {
		groups = append(groups, group{rows: frame.Rows})
	} else {
		for _, row := range frame.Rows {
			keys := make([]any, len(byIndexes))
			hasNullKey := false
			for index, columnIndex := range byIndexes {
				keys[index] = row[columnIndex]
				hasNullKey = hasNullKey || IsNull(row[columnIndex])
			}
			if hasNullKey && !includeNullGroups {
				continue
			}
			key := rowValueKey(row, byIndexes)
			groupIndex, exists := groupIndexes[key]
			if !exists {
				groupIndex = len(groups)
				groupIndexes[key] = groupIndex
				groups = append(groups, group{keys: keys})
			}
			groups[groupIndex].rows = append(groups[groupIndex].rows, row)
		}
		sort.SliceStable(groups, func(left, right int) bool {
			for index := range groups[left].keys {
				comparison := Compare(groups[left].keys[index], groups[right].keys[index])
				if comparison != 0 {
					return comparison < 0
				}
			}
			return false
		})
	}
	outputRows := make([][]any, 0, len(groups))
	for _, currentGroup := range groups {
		output := append([]any(nil), currentGroup.keys...)
		for index, aggregation := range aggregations {
			values := make([]any, len(currentGroup.rows))
			for rowIndex, row := range currentGroup.rows {
				values[rowIndex] = row[aggregationIndexes[index]]
			}
			value, aggregateErr := aggregateValues(values, aggregation.Func)
			if aggregateErr != nil {
				return Frame{}, fmt.Errorf("%s(%s): %w", aggregation.Func, aggregation.Column, aggregateErr)
			}
			output = append(output, value)
		}
		outputRows = append(outputRows, output)
	}
	return New(outputColumns, outputRows)
}

func MeanAggregations(frame Frame, by []string) ([]Aggregation, error) {
	bySet := map[string]struct{}{}
	for _, column := range by {
		bySet[column] = struct{}{}
		if _, err := frame.ColumnIndex(column); err != nil {
			return nil, err
		}
	}
	types := frame.ColumnTypes()
	result := make([]Aggregation, 0)
	for index, columnType := range types {
		column := frame.Columns[index]
		if _, grouped := bySet[column]; !grouped && columnType == "number" {
			result = append(result, Aggregation{Column: column, Func: "mean", As: column + "_mean"})
		}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("groupby found no numeric columns to average")
	}
	return result, nil
}

func Cut(frame Frame, column string, bins []float64, labels []string, outputColumn string) (Frame, error) {
	if len(bins) < 2 {
		return Frame{}, fmt.Errorf("cut requires at least two bin edges")
	}
	for index := 1; index < len(bins); index++ {
		if bins[index] <= bins[index-1] {
			return Frame{}, fmt.Errorf("bin edges must be strictly increasing")
		}
	}
	if len(labels) > 0 && len(labels) != len(bins)-1 {
		return Frame{}, fmt.Errorf("cut needs %d labels, got %d", len(bins)-1, len(labels))
	}
	columnIndex, err := frame.ColumnIndex(column)
	if err != nil {
		return Frame{}, err
	}
	if outputColumn == "" {
		outputColumn = column + "_bin"
	}
	result := frame.Clone()
	outputIndex, outputErr := result.ColumnIndex(outputColumn)
	if outputErr != nil {
		outputIndex = len(result.Columns)
		result.Columns = append(result.Columns, outputColumn)
		for index := range result.Rows {
			result.Rows[index] = append(result.Rows[index], nil)
		}
	}
	for rowIndex, row := range frame.Rows {
		number, ok := Float(row[columnIndex])
		if !ok {
			continue
		}
		for binIndex := 0; binIndex < len(bins)-1; binIndex++ {
			inside := number > bins[binIndex] && number <= bins[binIndex+1]
			if inside {
				value := fmt.Sprintf("(%g,%g]", bins[binIndex], bins[binIndex+1])
				if len(labels) > 0 {
					value = labels[binIndex]
				}
				result.Rows[rowIndex][outputIndex] = value
				break
			}
		}
	}
	return result, nil
}

type ProfileResult struct {
	Shape      [2]int       `json:"shape"`
	Columns    []ColumnInfo `json:"columns"`
	Duplicates int          `json:"duplicateRows"`
	Describe   *Frame       `json:"describe,omitempty"`
}

func Profile(frame Frame) ProfileResult {
	info := Info(frame)
	result := ProfileResult{Shape: [2]int{len(frame.Rows), len(frame.Columns)}, Columns: info.Schema}
	seen := map[string]struct{}{}
	allColumns := make([]int, len(frame.Columns))
	for index := range allColumns {
		allColumns[index] = index
	}
	for _, row := range frame.Rows {
		key := rowValueKey(row, allColumns)
		if _, exists := seen[key]; exists {
			result.Duplicates++
		} else {
			seen[key] = struct{}{}
		}
	}
	if described, err := Describe(frame, nil); err == nil {
		result.Describe = &described
	}
	return result
}

type IndexedRow struct {
	Index int            `json:"index"`
	Row   map[string]any `json:"row"`
}

func IdxMax(frame Frame, column string) (IndexedRow, error) {
	columnIndex, err := frame.ColumnIndex(column)
	if err != nil {
		return IndexedRow{}, err
	}
	found, bestIndex := false, 0
	var best any
	for rowIndex, row := range frame.Rows {
		if IsNull(row[columnIndex]) {
			continue
		}
		if !found || Compare(row[columnIndex], best) > 0 {
			found, bestIndex, best = true, rowIndex, row[columnIndex]
		}
	}
	if !found {
		return IndexedRow{}, fmt.Errorf("column %s has no non-null values", column)
	}
	row := frame.RowMap(frame.Rows[bestIndex])
	for column, value := range row {
		row[column] = jsonSafeScalar(value)
	}
	return IndexedRow{Index: bestIndex, Row: row}, nil
}

func GetDummies(frame Frame, columns []string, prefix string) (Frame, error) {
	if len(columns) == 0 {
		return Frame{}, fmt.Errorf("get-dummies requires one or more columns")
	}
	if prefix != "" && len(columns) > 1 {
		return Frame{}, fmt.Errorf("get-dummies --prefix supports exactly one column")
	}
	indexes, err := frame.ColumnIndexes(columns)
	if err != nil {
		return Frame{}, err
	}
	remove := map[int]struct{}{}
	for _, index := range indexes {
		remove[index] = struct{}{}
	}
	resultColumns := make([]string, 0)
	usedNames := map[string]struct{}{}
	for index, column := range frame.Columns {
		if _, selected := remove[index]; !selected {
			resultColumns = append(resultColumns, column)
			usedNames[column] = struct{}{}
		}
	}
	type dummy struct {
		columnIndex int
		value       any
		name        string
	}
	dummies := make([]dummy, 0)
	for selectedIndex, columnIndex := range indexes {
		values := make([]any, 0)
		seen := map[string]struct{}{}
		for _, row := range frame.Rows {
			value := row[columnIndex]
			if IsNull(value) {
				continue
			}
			key := ValueKey(value)
			if _, exists := seen[key]; !exists {
				seen[key] = struct{}{}
				values = append(values, value)
			}
		}
		sort.SliceStable(values, func(i, j int) bool { return Compare(values[i], values[j]) < 0 })
		for _, value := range values {
			namePrefix := columns[selectedIndex]
			if prefix != "" && len(columns) == 1 {
				namePrefix = prefix
			}
			name := nextAvailableName(namePrefix+"_"+fmt.Sprint(value), usedNames)
			usedNames[name] = struct{}{}
			resultColumns = append(resultColumns, name)
			dummies = append(dummies, dummy{columnIndex: columnIndex, value: value, name: name})
		}
	}
	rows := make([][]any, len(frame.Rows))
	for rowIndex, row := range frame.Rows {
		output := make([]any, 0, len(resultColumns))
		for columnIndex, value := range row {
			if _, selected := remove[columnIndex]; !selected {
				output = append(output, value)
			}
		}
		for _, current := range dummies {
			if ValueKey(row[current.columnIndex]) == ValueKey(current.value) {
				output = append(output, int64(1))
			} else {
				output = append(output, int64(0))
			}
		}
		rows[rowIndex] = output
	}
	return New(resultColumns, rows)
}

func Concat(frames []Frame, axis int) (Frame, error) {
	if len(frames) == 0 {
		return Frame{}, fmt.Errorf("concat requires at least one frame")
	}
	switch axis {
	case 0:
		columns := make([]string, 0)
		seen := map[string]struct{}{}
		for _, frame := range frames {
			for _, column := range frame.Columns {
				if _, exists := seen[column]; !exists {
					seen[column] = struct{}{}
					columns = append(columns, column)
				}
			}
		}
		rows := make([][]any, 0)
		for _, frame := range frames {
			indexes := make(map[string]int, len(frame.Columns))
			for index, column := range frame.Columns {
				indexes[column] = index
			}
			for _, row := range frame.Rows {
				output := make([]any, len(columns))
				for index, column := range columns {
					if sourceIndex, exists := indexes[column]; exists {
						output[index] = row[sourceIndex]
					}
				}
				rows = append(rows, output)
			}
		}
		return New(columns, rows)
	case 1:
		rowCount := 0
		for _, frame := range frames {
			if len(frame.Rows) > rowCount {
				rowCount = len(frame.Rows)
			}
		}
		columns := make([]string, 0)
		seen := map[string]struct{}{}
		for _, frame := range frames {
			for _, original := range frame.Columns {
				name := nextAvailableName(original, seen)
				seen[name] = struct{}{}
				columns = append(columns, name)
			}
		}
		rows := make([][]any, rowCount)
		for rowIndex := 0; rowIndex < rowCount; rowIndex++ {
			row := make([]any, 0, len(columns))
			for _, frame := range frames {
				if rowIndex < len(frame.Rows) {
					row = append(row, frame.Rows[rowIndex]...)
				} else {
					row = append(row, make([]any, len(frame.Columns))...)
				}
			}
			rows[rowIndex] = row
		}
		return New(columns, rows)
	default:
		return Frame{}, fmt.Errorf("axis must be 0 or 1")
	}
}

func nextAvailableName(original string, used map[string]struct{}) string {
	if _, exists := used[original]; !exists {
		return original
	}
	for suffix := 1; ; suffix++ {
		candidate := original + "." + strconv.Itoa(suffix)
		if _, exists := used[candidate]; !exists {
			return candidate
		}
	}
}

func numericValues(frame Frame, columnIndex int) []float64 {
	values := make([]float64, 0, len(frame.Rows))
	for _, row := range frame.Rows {
		if value, ok := Float(row[columnIndex]); ok {
			values = append(values, value)
		}
	}
	return values
}

func mean(values []float64) float64 {
	total := 0.0
	for _, value := range values {
		total += value
	}
	return total / float64(len(values))
}

func quantile(sortedValues []float64, fraction float64) float64 {
	if len(sortedValues) == 1 {
		return sortedValues[0]
	}
	position := fraction * float64(len(sortedValues)-1)
	lower := int(math.Floor(position))
	upper := int(math.Ceil(position))
	if lower == upper {
		return sortedValues[lower]
	}
	weight := position - float64(lower)
	return sortedValues[lower]*(1-weight) + sortedValues[upper]*weight
}

func aggregateValues(values []any, function string) (any, error) {
	function = strings.ToLower(strings.TrimSpace(function))
	nonNull := make([]any, 0, len(values))
	for _, value := range values {
		if !IsNull(value) {
			nonNull = append(nonNull, value)
		}
	}
	switch function {
	case "count":
		return int64(len(nonNull)), nil
	case "nunique":
		seen := map[string]struct{}{}
		for _, value := range nonNull {
			seen[ValueKey(value)] = struct{}{}
		}
		return int64(len(seen)), nil
	case "min", "max":
		if len(nonNull) == 0 {
			return nil, nil
		}
		best := nonNull[0]
		for _, value := range nonNull[1:] {
			comparison := Compare(value, best)
			if (function == "min" && comparison < 0) || (function == "max" && comparison > 0) {
				best = value
			}
		}
		return best, nil
	case "mode":
		if len(nonNull) == 0 {
			return nil, nil
		}
		counts := map[string]int{}
		first := map[string]any{}
		bestKey, bestCount := "", -1
		for _, value := range nonNull {
			key := ValueKey(value)
			counts[key]++
			if _, exists := first[key]; !exists {
				first[key] = value
			}
			if counts[key] > bestCount {
				bestKey, bestCount = key, counts[key]
			}
		}
		return first[bestKey], nil
	case "sum", "mean", "median":
		numbers := make([]float64, 0, len(nonNull))
		for _, value := range nonNull {
			number, ok := Float(value)
			if !ok {
				return nil, fmt.Errorf("%s requires numeric values", function)
			}
			numbers = append(numbers, number)
		}
		if len(numbers) == 0 {
			return nil, nil
		}
		if function == "sum" {
			total := 0.0
			for _, number := range numbers {
				total += number
			}
			return total, nil
		}
		if function == "mean" {
			return mean(numbers), nil
		}
		sort.Float64s(numbers)
		return quantile(numbers, .5), nil
	default:
		return nil, fmt.Errorf("unsupported aggregation: %s", function)
	}
}
