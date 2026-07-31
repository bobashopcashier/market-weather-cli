package dataframe

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"
)

const maxInputBytes = 256 << 20

// LoadOptions controls how tabular input is decoded.
type LoadOptions struct {
	Format     string
	Path       string
	Layout     string
	InferTypes bool
}

// OutputOptions controls how a Frame is encoded.
type OutputOptions struct {
	Format         string
	Compact        bool
	SourceRowCount int
}

// Load reads a CSV, JSON, or JSONL table. JSON Path is an RFC 6901 JSON
// Pointer applied before the selected value is converted to a Frame.
func Load(reader io.Reader, options LoadOptions) (Frame, error) {
	data, err := io.ReadAll(io.LimitReader(reader, maxInputBytes+1))
	if err != nil {
		return Frame{}, fmt.Errorf("read dataframe input: %w", err)
	}
	if len(data) > maxInputBytes {
		return Frame{}, fmt.Errorf("dataframe input exceeds the 256 MiB safety limit")
	}
	format := normalizeFormat(options.Format)
	auto := format == "" || format == "auto"
	if auto {
		format = detectFormat(data)
		if format == "json" && options.Path == "" {
			frame, jsonErr := loadJSON(data, options)
			if jsonErr == nil {
				return frame, nil
			}
			csvFrame, csvErr := loadCSV(bytes.NewReader(data), options.InferTypes)
			if csvErr == nil && (len(csvFrame.Columns) > 1 || len(csvFrame.Rows) > 0) {
				return csvFrame, nil
			}
			return Frame{}, jsonErr
		}
	}
	switch format {
	case "csv":
		if options.Path != "" {
			return Frame{}, fmt.Errorf("JSON Pointer path is not supported for CSV input")
		}
		return loadCSV(bytes.NewReader(data), options.InferTypes)
	case "json":
		return loadJSON(data, options)
	case "jsonl":
		return loadJSONL(data, options)
	default:
		return Frame{}, fmt.Errorf("unsupported input format %q; want auto, csv, json, or jsonl", options.Format)
	}
}

// LoadFile opens path and loads a Frame. With auto format, a recognized file
// extension takes precedence over content sniffing.
func LoadFile(path string, options LoadOptions) (Frame, error) {
	file, err := os.Open(path)
	if err != nil {
		return Frame{}, fmt.Errorf("open dataframe input %q: %w", path, err)
	}
	defer file.Close()
	if format := normalizeFormat(options.Format); format == "" || format == "auto" {
		switch strings.ToLower(filepath.Ext(path)) {
		case ".csv":
			options.Format = "csv"
		case ".json":
			options.Format = "json"
		case ".jsonl", ".ndjson":
			options.Format = "jsonl"
		}
	}
	frame, err := Load(file, options)
	if err != nil {
		return Frame{}, fmt.Errorf("load dataframe input %q: %w", path, err)
	}
	return frame, nil
}

// Write encodes a Frame as canonical JSON, CSV, or a human-readable table.
func Write(writer io.Writer, frame Frame, options OutputOptions) error {
	if _, err := New(frame.Columns, frame.Rows); err != nil {
		return fmt.Errorf("invalid dataframe: %w", err)
	}
	format := normalizeFormat(options.Format)
	if format == "" || format == "auto" {
		format = "json"
	}
	switch format {
	case "json":
		return writeJSON(writer, frame, options)
	case "csv":
		return writeCSV(writer, frame)
	case "table":
		return writeTable(writer, frame)
	default:
		return fmt.Errorf("unsupported output format %q; want json, csv, or table", options.Format)
	}
}

func normalizeFormat(format string) string {
	return strings.ToLower(strings.TrimSpace(format))
}

func detectFormat(data []byte) string {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[') {
		return "json"
	}
	return "csv"
}

func loadCSV(reader io.Reader, inferTypes bool) (Frame, error) {
	decoder := csv.NewReader(reader)
	records, err := decoder.ReadAll()
	if err != nil {
		return Frame{}, fmt.Errorf("decode CSV: %w", err)
	}
	if len(records) == 0 {
		return Frame{}, fmt.Errorf("CSV input is missing a header row")
	}
	columns := records[0]
	rows := make([][]any, 0, len(records)-1)
	for rowIndex, record := range records[1:] {
		if len(record) != len(columns) {
			return Frame{}, fmt.Errorf("CSV row %d has %d fields, want %d", rowIndex+2, len(record), len(columns))
		}
		row := make([]any, len(record))
		for columnIndex, cell := range record {
			if cell == "" {
				row[columnIndex] = nil
			} else if inferTypes {
				row[columnIndex] = parseCSVScalar(cell)
			} else {
				row[columnIndex] = cell
			}
		}
		rows = append(rows, row)
	}
	frame, err := New(columns, rows)
	if err != nil {
		return Frame{}, fmt.Errorf("decode CSV table: %w", err)
	}
	return frame, nil
}

func parseCSVScalar(value string) any {
	trimmed := strings.TrimSpace(value)
	if strings.EqualFold(trimmed, "null") || strings.EqualFold(trimmed, "nil") || strings.EqualFold(trimmed, "na") || strings.EqualFold(trimmed, "n/a") {
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

func loadJSON(data []byte, options LoadOptions) (Frame, error) {
	value, trailing, err := decodeJSONDocument(data)
	if err != nil {
		return Frame{}, fmt.Errorf("decode JSON: %w", err)
	}
	if trailing {
		return loadJSONL(data, options)
	}
	selected, err := selectJSONPointer(value, options.Path)
	if err != nil {
		return Frame{}, err
	}
	return valueToFrame(selected, options.Layout)
}

func decodeJSONDocument(data []byte) (any, bool, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, false, err
	}
	var extra any
	err := decoder.Decode(&extra)
	if err == io.EOF {
		return value, false, nil
	}
	if err == nil {
		return value, true, nil
	}
	return nil, false, fmt.Errorf("after first JSON value: %w", err)
}

func loadJSONL(data []byte, options LoadOptions) (Frame, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	records := make([]map[string]any, 0)
	for recordIndex := 0; ; recordIndex++ {
		var value any
		if err := decoder.Decode(&value); err != nil {
			if err == io.EOF {
				break
			}
			return Frame{}, fmt.Errorf("decode JSONL record %d: %w", recordIndex+1, err)
		}
		selected, err := selectJSONPointer(value, options.Path)
		if err != nil {
			return Frame{}, fmt.Errorf("JSONL record %d: %w", recordIndex+1, err)
		}
		record, ok := selected.(map[string]any)
		if !ok {
			return Frame{}, fmt.Errorf("JSONL record %d is %T, want an object after applying path", recordIndex+1, selected)
		}
		records = append(records, record)
	}
	return recordsToFrame(records)
}

func valueToFrame(value any, layout string) (Frame, error) {
	layout = strings.ToLower(strings.TrimSpace(layout))
	if layout == "" {
		layout = "auto"
	}
	if layout != "auto" && layout != "records" && layout != "columns" {
		return Frame{}, fmt.Errorf("unsupported JSON layout %q; want auto, records, or columns", layout)
	}
	if object, ok := value.(map[string]any); ok {
		if frame, matched, err := canonicalToFrame(object); matched || err != nil {
			return frame, err
		}
		switch layout {
		case "records":
			return recordsToFrame([]map[string]any{object})
		case "columns":
			return columnsToFrame(object)
		default:
			if allValuesAreArrays(object) {
				return columnsToFrame(object)
			}
			return recordsToFrame([]map[string]any{object})
		}
	}
	array, ok := value.([]any)
	if !ok {
		return Frame{}, fmt.Errorf("JSON table is %T, want an object or array of objects", value)
	}
	if layout == "columns" {
		return Frame{}, fmt.Errorf("columns layout requires a JSON object of arrays")
	}
	records := make([]map[string]any, len(array))
	for index, item := range array {
		record, ok := item.(map[string]any)
		if !ok {
			return Frame{}, fmt.Errorf("JSON record %d is %T, want an object", index, item)
		}
		records[index] = record
	}
	return recordsToFrame(records)
}

func recordsToFrame(records []map[string]any) (Frame, error) {
	columnSet := make(map[string]struct{})
	for _, record := range records {
		for key := range record {
			columnSet[key] = struct{}{}
		}
	}
	columns := make([]string, 0, len(columnSet))
	for key := range columnSet {
		columns = append(columns, key)
	}
	sortStrings(columns)
	rows := make([][]any, len(records))
	for rowIndex, record := range records {
		row := make([]any, len(columns))
		for columnIndex, column := range columns {
			row[columnIndex] = record[column]
		}
		rows[rowIndex] = row
	}
	return New(columns, rows)
}

func allValuesAreArrays(object map[string]any) bool {
	if len(object) == 0 {
		return false
	}
	for _, value := range object {
		if _, ok := value.([]any); !ok {
			return false
		}
	}
	return true
}

func columnsToFrame(object map[string]any) (Frame, error) {
	columns := SortedKeys(object)
	rowCount := -1
	columnValues := make([][]any, len(columns))
	for columnIndex, column := range columns {
		values, ok := object[column].([]any)
		if !ok {
			return Frame{}, fmt.Errorf("column %q is %T, want an array", column, object[column])
		}
		if rowCount < 0 {
			rowCount = len(values)
		} else if len(values) != rowCount {
			return Frame{}, fmt.Errorf("column %q has %d values, want %d", column, len(values), rowCount)
		}
		columnValues[columnIndex] = values
	}
	if rowCount < 0 {
		rowCount = 0
	}
	rows := make([][]any, rowCount)
	for rowIndex := range rows {
		rows[rowIndex] = make([]any, len(columns))
		for columnIndex := range columns {
			rows[rowIndex][columnIndex] = columnValues[columnIndex][rowIndex]
		}
	}
	return New(columns, rows)
}

func canonicalToFrame(object map[string]any) (Frame, bool, error) {
	version, hasVersion := object["schemaVersion"]
	if !hasVersion {
		return Frame{}, false, nil
	}
	if version != "mwx.table/v1" {
		_, hasColumns := object["columns"]
		_, hasRows := object["rows"]
		if hasColumns && hasRows {
			return Frame{}, true, fmt.Errorf("unsupported table schemaVersion %q", version)
		}
		return Frame{}, false, nil
	}
	columnValues, ok := object["columns"].([]any)
	if !ok {
		return Frame{}, true, fmt.Errorf("canonical table columns must be an array")
	}
	columns := make([]string, len(columnValues))
	for index, value := range columnValues {
		switch typed := value.(type) {
		case string:
			columns[index] = typed
		case map[string]any:
			name, ok := typed["name"].(string)
			if !ok {
				return Frame{}, true, fmt.Errorf("canonical column %d is missing a string name", index)
			}
			columns[index] = name
		default:
			return Frame{}, true, fmt.Errorf("canonical column %d is %T, want a name or schema object", index, value)
		}
	}
	rowValues, ok := object["rows"].([]any)
	if !ok {
		return Frame{}, true, fmt.Errorf("canonical table rows must be an array")
	}
	rows := make([][]any, len(rowValues))
	for index, value := range rowValues {
		row, ok := value.([]any)
		if !ok {
			return Frame{}, true, fmt.Errorf("canonical row %d is %T, want an array", index, value)
		}
		rows[index] = row
	}
	frame, err := New(columns, rows)
	if err != nil {
		return Frame{}, true, fmt.Errorf("decode canonical table: %w", err)
	}
	return frame, true, nil
}

func selectJSONPointer(value any, pointer string) (any, error) {
	if pointer == "" {
		return value, nil
	}
	if !strings.HasPrefix(pointer, "/") {
		return nil, fmt.Errorf("invalid JSON Pointer %q: must be empty or start with /", pointer)
	}
	current := value
	for _, encodedToken := range strings.Split(pointer[1:], "/") {
		token, err := decodePointerToken(encodedToken)
		if err != nil {
			return nil, fmt.Errorf("invalid JSON Pointer %q: %w", pointer, err)
		}
		switch typed := current.(type) {
		case map[string]any:
			next, exists := typed[token]
			if !exists {
				return nil, fmt.Errorf("JSON Pointer %q: object key %q does not exist", pointer, token)
			}
			current = next
		case []any:
			if token == "-" {
				return nil, fmt.Errorf("JSON Pointer %q: - is not valid for reading an array", pointer)
			}
			index, err := strconv.Atoi(token)
			if err != nil || index < 0 || index >= len(typed) {
				return nil, fmt.Errorf("JSON Pointer %q: array index %q is out of range", pointer, token)
			}
			current = typed[index]
		default:
			return nil, fmt.Errorf("JSON Pointer %q: cannot select %q from %T", pointer, token, current)
		}
	}
	return current, nil
}

func decodePointerToken(token string) (string, error) {
	var result strings.Builder
	for index := 0; index < len(token); index++ {
		if token[index] != '~' {
			result.WriteByte(token[index])
			continue
		}
		if index+1 >= len(token) {
			return "", fmt.Errorf("trailing ~ escape")
		}
		index++
		switch token[index] {
		case '0':
			result.WriteByte('~')
		case '1':
			result.WriteByte('/')
		default:
			return "", fmt.Errorf("invalid ~%c escape", token[index])
		}
	}
	return result.String(), nil
}

type columnSchema struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type tableMeta struct {
	RowCount       int  `json:"rowCount"`
	ColumnCount    int  `json:"columnCount"`
	SourceRowCount int  `json:"sourceRowCount,omitempty"`
	Truncated      bool `json:"truncated,omitempty"`
}

type tableEnvelope struct {
	SchemaVersion string         `json:"schemaVersion"`
	Columns       []columnSchema `json:"columns"`
	Rows          [][]any        `json:"rows"`
	Meta          tableMeta      `json:"meta"`
}

func writeJSON(writer io.Writer, frame Frame, options OutputOptions) error {
	types := frame.ColumnTypes()
	columns := make([]columnSchema, len(frame.Columns))
	for index, name := range frame.Columns {
		columns[index] = columnSchema{Name: name, Type: types[index]}
	}
	rows := make([][]any, len(frame.Rows))
	for rowIndex, source := range frame.Rows {
		row := make([]any, len(source))
		for columnIndex, value := range source {
			row[columnIndex] = jsonValue(value)
		}
		rows[rowIndex] = row
	}
	envelope := tableEnvelope{
		SchemaVersion: "mwx.table/v1",
		Columns:       columns,
		Rows:          rows,
		Meta:          tableMeta{RowCount: len(frame.Rows), ColumnCount: len(frame.Columns)},
	}
	if options.SourceRowCount > len(frame.Rows) {
		envelope.Meta.SourceRowCount = options.SourceRowCount
		envelope.Meta.Truncated = true
	}
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	if !options.Compact {
		encoder.SetIndent("", "  ")
	}
	if err := encoder.Encode(envelope); err != nil {
		return fmt.Errorf("encode JSON dataframe: %w", err)
	}
	return nil
}

func jsonValue(value any) any {
	if value == nil {
		return nil
	}
	switch typed := value.(type) {
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) {
			return nil
		}
		if math.Trunc(typed) == typed {
			return json.Number(strconv.FormatFloat(typed, 'f', 1, 64))
		}
	case float32:
		if math.IsNaN(float64(typed)) || math.IsInf(float64(typed), 0) {
			return nil
		}
		if math.Trunc(float64(typed)) == float64(typed) {
			return json.Number(strconv.FormatFloat(float64(typed), 'f', 1, 32))
		}
	}
	return value
}

func writeCSV(writer io.Writer, frame Frame) error {
	encoder := csv.NewWriter(writer)
	if err := encoder.Write(frame.Columns); err != nil {
		return fmt.Errorf("encode CSV header: %w", err)
	}
	for rowIndex, row := range frame.Rows {
		record := make([]string, len(row))
		for columnIndex, value := range row {
			record[columnIndex] = formatCell(value)
		}
		if err := encoder.Write(record); err != nil {
			return fmt.Errorf("encode CSV row %d: %w", rowIndex, err)
		}
	}
	encoder.Flush()
	if err := encoder.Error(); err != nil {
		return fmt.Errorf("encode CSV: %w", err)
	}
	return nil
}

func writeTable(writer io.Writer, frame Frame) error {
	columns := make([]string, len(frame.Columns))
	widths := make([]int, len(frame.Columns))
	for index, column := range frame.Columns {
		columns[index] = safeTableText(column)
		widths[index] = utf8.RuneCountInString(columns[index])
	}
	rows := make([][]string, len(frame.Rows))
	for rowIndex, row := range frame.Rows {
		rows[rowIndex] = make([]string, len(row))
		for columnIndex, value := range row {
			text := safeTableText(formatCell(value))
			rows[rowIndex][columnIndex] = text
			if width := utf8.RuneCountInString(text); width > widths[columnIndex] {
				widths[columnIndex] = width
			}
		}
	}
	writeRow := func(values []string) error {
		cells := make([]string, len(frame.Columns))
		for index := range frame.Columns {
			value := ""
			if index < len(values) {
				value = values[index]
			}
			cells[index] = value + strings.Repeat(" ", widths[index]-utf8.RuneCountInString(value))
		}
		_, err := fmt.Fprintln(writer, strings.TrimRight(strings.Join(cells, "  "), " "))
		return err
	}
	if err := writeRow(columns); err != nil {
		return fmt.Errorf("encode table header: %w", err)
	}
	separators := make([]string, len(widths))
	for index, width := range widths {
		separators[index] = strings.Repeat("-", width)
	}
	if _, err := fmt.Fprintln(writer, strings.Join(separators, "  ")); err != nil {
		return fmt.Errorf("encode table separator: %w", err)
	}
	for rowIndex, row := range rows {
		if err := writeRow(row); err != nil {
			return fmt.Errorf("encode table row %d: %w", rowIndex, err)
		}
	}
	return nil
}

func safeTableText(value string) string {
	var output strings.Builder
	for _, current := range value {
		switch current {
		case '\n':
			output.WriteString(`\n`)
		case '\r':
			output.WriteString(`\r`)
		case '\t':
			output.WriteString(`\t`)
		default:
			if current < 0x20 || current == 0x7f || current >= 0x80 && current <= 0x9f {
				fmt.Fprintf(&output, `\u%04x`, current)
			} else {
				output.WriteRune(current)
			}
		}
	}
	return output.String()
}

func formatCell(value any) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	case bool:
		return strconv.FormatBool(typed)
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case float64:
		if math.IsNaN(typed) {
			return ""
		}
		return strconv.FormatFloat(typed, 'g', -1, 64)
	case float32:
		if math.IsNaN(float64(typed)) {
			return ""
		}
		return strconv.FormatFloat(float64(typed), 'g', -1, 32)
	case json.Number:
		return typed.String()
	default:
		encoded, err := json.Marshal(typed)
		if err == nil {
			return string(encoded)
		}
		return fmt.Sprint(typed)
	}
}

func sortStrings(values []string) {
	for index := 1; index < len(values); index++ {
		for cursor := index; cursor > 0 && values[cursor] < values[cursor-1]; cursor-- {
			values[cursor], values[cursor-1] = values[cursor-1], values[cursor]
		}
	}
}
