package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/bobashopcashier/market-weather-cli/internal/dataframe"
	"github.com/bobashopcashier/market-weather-cli/internal/provider"
	"github.com/bobashopcashier/market-weather-cli/internal/render"
)

const (
	maxConcatRetainedCells = 5_000_000
	maxConcatRetainedBytes = 128 << 20
)

func runData(ctx context.Context, argv []string) error {
	operation := normalizeDataOperation(argv[0])
	if !isDataOperation(operation) {
		err := provider.NewError("invalid_arguments", fmt.Sprintf("unknown dataframe operation: %s", operation), 2)
		err.Hint = "Run mwx data --help to list the 35 supported operations."
		return err
	}
	parsed, err := parseArgs(argv[1:], dataOperationOptions(operation))
	if err != nil {
		return err
	}
	if parsed.flag("help") {
		fmt.Fprint(os.Stdout, toolHelp["data"])
		return nil
	}
	if len(parsed.positionals) > 1 {
		return dataUsageError("data operations accept at most one positional input path")
	}
	if err := validateDataOperationArguments(operation, parsed, argv[1:]); err != nil {
		return err
	}
	output := parsed.value("output")
	if parsed.flag("json") {
		output = "json"
	}
	if parsed.flag("compact") && output != "json" {
		return dataUsageError("--compact requires JSON output")
	}
	if parsed.value("limit") != "" && output != "json" {
		return dataUsageError("--limit requires JSON output so truncation metadata is preserved")
	}
	frame, err := loadDataFrame(ctx, parsed, argv[1:], operation)
	if err != nil {
		return dataInputError(err)
	}
	writeFrame := func(result dataframe.Frame) error {
		sourceRows := len(result.Rows)
		fields := dataframe.SplitList(parsed.value("fields"))
		if len(fields) > 0 {
			result, err = dataframe.SelectColumns(result, fields)
			if err != nil {
				return dataOperationError(err)
			}
		}
		if limit := parsed.integer("limit"); limit > 0 && len(result.Rows) > limit {
			result = dataframe.Head(result, limit)
		}
		if err := dataframe.Write(os.Stdout, result, dataframe.OutputOptions{
			Format: output, Compact: parsed.flag("compact"), SourceRowCount: sourceRows,
		}); err != nil {
			return dataInputError(err)
		}
		return nil
	}
	var structuredMeta map[string]any
	writeResult := func(value any) error {
		if output != "json" {
			return dataUsageError(fmt.Sprintf("%s produces structured JSON and does not support --output %s", operation, output))
		}
		valueEnvelope := map[string]any{
			"schemaVersion": "mwx.result/v1",
			"operation":     operation,
			"data":          value,
		}
		if structuredMeta != nil {
			valueEnvelope["meta"] = structuredMeta
		}
		if parsed.flag("compact") {
			return render.CompactJSON(os.Stdout, valueEnvelope)
		}
		return render.JSON(os.Stdout, valueEnvelope)
	}

	switch operation {
	case "read-csv":
		return writeFrame(frame)
	case "columns":
		return writeFrame(columnListFrame(frame.Columns))
	case "head":
		return writeFrame(dataframe.Head(frame, parsed.integer("n")))
	case "tail":
		return writeFrame(dataframe.Tail(frame, parsed.integer("n")))
	case "shape":
		shape, _ := dataframe.New([]string{"rows", "columns"}, [][]any{{int64(len(frame.Rows)), int64(len(frame.Columns))}})
		return writeFrame(shape)
	case "info":
		return writeFrame(infoFrame(dataframe.Info(frame)))
	case "describe":
		result, operationErr := dataframe.Describe(frame, dataframe.SplitList(parsed.value("columns")))
		return writeFrameResult(result, operationErr, writeFrame)
	case "select-dtypes":
		result, operationErr := dataframe.SelectDTypes(frame, dataframe.SplitList(parsed.value("include")), dataframe.SplitList(parsed.value("exclude")))
		return writeFrameResult(result, operationErr, writeFrame)
	case "astype":
		columns, operationErr := requiredColumns(parsed, "astype")
		if operationErr != nil {
			return operationErr
		}
		dtype, operationErr := requiredOption(parsed, "dtype", "astype")
		if operationErr != nil {
			return operationErr
		}
		result := frame
		for _, column := range columns {
			result, operationErr = dataframe.Cast(result, column, dtype)
			if operationErr != nil {
				return dataOperationError(operationErr)
			}
		}
		return writeFrame(result)
	case "value-counts":
		column, operationErr := requiredOption(parsed, "column", operation)
		if operationErr != nil {
			return operationErr
		}
		result, operationErr := dataframe.ValueCounts(frame, column, !parsed.flag("include-null"))
		return writeFrameResult(result, operationErr, writeFrame)
	case "unique":
		column, operationErr := requiredOption(parsed, "column", operation)
		if operationErr != nil {
			return operationErr
		}
		values, operationErr := dataframe.Unique(frame, column)
		if operationErr != nil {
			return dataOperationError(operationErr)
		}
		return writeFrame(columnValuesFrame(column, values))
	case "nunique":
		result, operationErr := nuniqueFrame(frame, parsed.value("column"), !parsed.flag("include-null"))
		return writeFrameResult(result, operationErr, writeFrame)
	case "isnull", "notnull":
		mask := dataframe.NullMask(frame, operation == "notnull")
		if parsed.flag("sum") {
			return writeFrame(nullCountFrame(mask))
		}
		return writeFrame(mask)
	case "duplicated":
		mask, operationErr := dataframe.DuplicatedMask(frame, dataframe.SplitList(parsed.value("subset")), parsed.value("keep"))
		if operationErr != nil {
			return dataOperationError(operationErr)
		}
		return writeFrame(boolMaskFrame("duplicated", mask))
	case "drop-duplicates":
		result, operationErr := dataframe.DropDuplicates(frame, dataframe.SplitList(parsed.value("subset")), parsed.value("keep"))
		return writeFrameResult(result, operationErr, writeFrame)
	case "rename":
		mapping, operationErr := parseRenameMapping(parsed.value("mapping"))
		if operationErr != nil {
			return dataUsageError(operationErr.Error())
		}
		result, operationErr := dataframe.Rename(frame, mapping)
		return writeFrameResult(result, operationErr, writeFrame)
	case "map":
		column, operationErr := requiredOption(parsed, "column", operation)
		if operationErr != nil {
			return operationErr
		}
		mapping, operationErr := parseValueMapping(parsed.value("mapping"))
		if operationErr != nil {
			return dataUsageError(operationErr.Error())
		}
		result, operationErr := dataframe.MapColumn(frame, column, mapping, parsed.flag("keep-unmapped"))
		return writeFrameResult(result, operationErr, writeFrame)
	case "query":
		expression, operationErr := requiredOption(parsed, "expr", operation)
		if operationErr != nil {
			return operationErr
		}
		result, operationErr := dataframe.Query(frame, expression)
		return writeFrameResult(result, operationErr, writeFrame)
	case "isin":
		column, operationErr := requiredOption(parsed, "column", operation)
		if operationErr != nil {
			return operationErr
		}
		values, operationErr := requiredOption(parsed, "values", operation)
		if operationErr != nil {
			return operationErr
		}
		mask, operationErr := dataframe.IsIn(frame, column, parseScalarList(values, parsed.flag("strings")))
		if operationErr != nil {
			return dataOperationError(operationErr)
		}
		return writeFrame(boolMaskFrame("isin", mask))
	case "drop":
		columns, operationErr := requiredColumns(parsed, operation)
		if operationErr != nil {
			return operationErr
		}
		result, operationErr := dataframe.DropColumns(frame, columns)
		return writeFrameResult(result, operationErr, writeFrame)
	case "fillna":
		columns, operationErr := requiredColumns(parsed, operation)
		if operationErr != nil {
			return operationErr
		}
		method := dataframe.FillMethod(parsed.value("strategy"))
		result := frame
		literal := dataframe.ParseScalar(parsed.value("value"))
		if parsed.flag("strings") {
			literal = parsed.value("value")
		}
		for _, column := range columns {
			result, operationErr = dataframe.FillNA(result, column, method, literal)
			if operationErr != nil {
				return dataOperationError(operationErr)
			}
		}
		return writeFrame(result)
	case "dropna":
		result, operationErr := dataframe.DropNA(frame, dataframe.SplitList(parsed.value("subset")), parsed.value("how"))
		return writeFrameResult(result, operationErr, writeFrame)
	case "groupby":
		by := dataframe.SplitList(parsed.value("by"))
		if len(by) == 0 {
			return dataUsageError("groupby requires --by")
		}
		aggregations, operationErr := aggregationsFor(frame, by, parsed.value("agg"))
		if operationErr != nil {
			return dataOperationError(operationErr)
		}
		aggregate := dataframe.Aggregate
		if parsed.flag("include-null") {
			aggregate = dataframe.AggregateIncludingNullGroups
		}
		result, operationErr := aggregate(frame, by, aggregations)
		return writeFrameResult(result, operationErr, writeFrame)
	case "agg":
		aggregations, operationErr := parseAggregations(parsed.value("agg"))
		if operationErr != nil {
			return dataUsageError(operationErr.Error())
		}
		aggregate := dataframe.Aggregate
		if parsed.flag("include-null") {
			aggregate = dataframe.AggregateIncludingNullGroups
		}
		result, operationErr := aggregate(frame, dataframe.SplitList(parsed.value("by")), aggregations)
		return writeFrameResult(result, operationErr, writeFrame)
	case "sort-values":
		by := dataframe.SplitList(parsed.value("by"))
		if len(by) == 0 {
			by = dataframe.SplitList(parsed.value("columns"))
		}
		if len(by) == 0 {
			return dataUsageError("sort-values requires --by")
		}
		result, operationErr := dataframe.SortValues(frame, by, []bool{!parsed.flag("descending")}, !parsed.flag("nulls-first"))
		return writeFrameResult(result, operationErr, writeFrame)
	case "loc":
		result, operationErr := dataframe.Loc(frame, parsed.value("where"), dataframe.SplitList(parsed.value("columns")))
		return writeFrameResult(result, operationErr, writeFrame)
	case "iloc":
		rowStart, rowEnd, operationErr := parseSlice(parsed.value("rows"), len(frame.Rows))
		if operationErr != nil {
			return dataUsageError("invalid --rows: " + operationErr.Error())
		}
		columnStart, columnEnd, operationErr := parseSlice(parsed.value("cols"), len(frame.Columns))
		if operationErr != nil {
			return dataUsageError("invalid --cols: " + operationErr.Error())
		}
		return writeFrame(dataframe.ILoc(frame, rowStart, rowEnd, columnStart, columnEnd))
	case "cut":
		column, operationErr := requiredOption(parsed, "column", operation)
		if operationErr != nil {
			return operationErr
		}
		bins, operationErr := parseFloatList(parsed.value("bins"))
		if operationErr != nil {
			return dataUsageError(operationErr.Error())
		}
		result, operationErr := dataframe.Cut(frame, column, bins, dataframe.SplitList(parsed.value("labels")), parsed.value("output-column"))
		return writeFrameResult(result, operationErr, writeFrame)
	case "apply":
		expression, operationErr := requiredOption(parsed, "expr", operation)
		if operationErr != nil {
			return operationErr
		}
		outputColumn, operationErr := requiredOption(parsed, "output-column", operation)
		if operationErr != nil {
			return operationErr
		}
		result, operationErr := dataframe.Apply(frame, expression, outputColumn)
		return writeFrameResult(result, operationErr, writeFrame)
	case "profile":
		return writeResult(dataframe.Profile(frame))
	case "idxmax":
		column, operationErr := requiredOption(parsed, "column", operation)
		if operationErr != nil {
			return operationErr
		}
		result, operationErr := dataframe.IdxMax(frame, column)
		if operationErr != nil {
			return dataOperationError(operationErr)
		}
		return writeResult(result)
	case "get-dummies":
		columns, operationErr := requiredColumns(parsed, operation)
		if operationErr != nil {
			return operationErr
		}
		result, operationErr := dataframe.GetDummies(frame, columns, parsed.value("prefix"))
		return writeFrameResult(result, operationErr, writeFrame)
	case "concat":
		otherPaths := dataframe.SplitList(parsed.value("with"))
		if len(otherPaths) == 0 {
			return dataUsageError("concat requires --with path[,path...]")
		}
		if len(otherPaths) > 64 {
			return dataUsageError("concat accepts at most 64 additional input files")
		}
		frames := []dataframe.Frame{frame}
		budget := concatInputBudget{}
		if budgetErr := budget.add(frame); budgetErr != nil {
			return dataUsageError(budgetErr.Error())
		}
		for _, path := range otherPaths {
			other, loadErr := loadDataFile(ctx, path, parsed, "auto")
			if loadErr != nil {
				return dataInputError(loadErr)
			}
			if budgetErr := budget.add(other); budgetErr != nil {
				return dataUsageError(budgetErr.Error())
			}
			frames = append(frames, other)
		}
		result, operationErr := dataframe.Concat(frames, parsed.integer("axis"))
		return writeFrameResult(result, operationErr, writeFrame)
	case "to-numpy":
		sourceRows := len(frame.Rows)
		if fields := dataframe.SplitList(parsed.value("fields")); len(fields) > 0 {
			frame, err = dataframe.SelectColumns(frame, fields)
			if err != nil {
				return dataOperationError(err)
			}
		}
		if limit := parsed.integer("limit"); limit > 0 && len(frame.Rows) > limit {
			frame = dataframe.Head(frame, limit)
			structuredMeta = map[string]any{"rowCount": len(frame.Rows), "sourceRowCount": sourceRows, "truncated": true}
		}
		return writeResult(dataframe.ToNumpy(frame))
	}
	return dataUsageError("unhandled dataframe operation: " + operation)
}

type concatInputBudget struct {
	cells int64
	bytes int64
}

func (budget *concatInputBudget) add(frame dataframe.Frame) error {
	rows, columns := int64(len(frame.Rows)), int64(len(frame.Columns))
	if rows > 0 && columns > maxConcatRetainedCells/rows {
		return fmt.Errorf("concat inputs exceed the %d-cell retained-input limit", maxConcatRetainedCells)
	}
	cells := rows * columns
	if budget.cells > maxConcatRetainedCells-cells {
		return fmt.Errorf("concat inputs exceed the %d-cell retained-input limit", maxConcatRetainedCells)
	}
	estimatedBytes := cells * 16
	for _, column := range frame.Columns {
		estimatedBytes += int64(len(column))
	}
	for _, row := range frame.Rows {
		for _, value := range row {
			estimatedBytes += estimateDataValueBytes(value, 0)
			if estimatedBytes > maxConcatRetainedBytes {
				return fmt.Errorf("concat inputs exceed the %d MiB retained-input limit", maxConcatRetainedBytes>>20)
			}
		}
	}
	if budget.bytes > maxConcatRetainedBytes-estimatedBytes {
		return fmt.Errorf("concat inputs exceed the %d MiB retained-input limit", maxConcatRetainedBytes>>20)
	}
	budget.cells += cells
	budget.bytes += estimatedBytes
	return nil
}

func estimateDataValueBytes(value any, depth int) int64 {
	if depth >= 8 {
		return 64
	}
	switch typed := value.(type) {
	case string:
		return int64(len(typed))
	case json.Number:
		return int64(len(typed))
	case []any:
		total := int64(24)
		for _, item := range typed {
			total += 16 + estimateDataValueBytes(item, depth+1)
		}
		return total
	case map[string]any:
		total := int64(48)
		for key, item := range typed {
			total += int64(len(key)) + 32 + estimateDataValueBytes(item, depth+1)
		}
		return total
	default:
		return 0
	}
}

func validateDataOperationArguments(operation string, parsed parsedArgs, argv []string) error {
	spec := dataOperations[operation]
	for _, name := range spec.required {
		if strings.TrimSpace(parsed.value(name)) == "" && !parsed.flag(name) {
			return dataUsageError(fmt.Sprintf("%s requires --%s", operation, name))
		}
	}
	for _, group := range spec.anyOf {
		matched := false
		for _, name := range group {
			if strings.TrimSpace(parsed.value(name)) != "" || parsed.flag(name) {
				matched = true
				break
			}
		}
		if !matched {
			formatted := make([]string, len(group))
			for index, name := range group {
				formatted[index] = "--" + name
			}
			return dataUsageError(fmt.Sprintf("%s requires one of %s", operation, strings.Join(formatted, ", ")))
		}
	}
	if operation == "fillna" && parsed.value("strategy") == "literal" && !optionProvided(argv, "value") {
		return dataUsageError("fillna --strategy literal requires --value")
	}
	return nil
}

func normalizeDataOperation(operation string) string {
	operation = strings.ToLower(strings.TrimSpace(operation))
	operation = strings.ReplaceAll(operation, "_", "-")
	switch operation {
	case "profile-report", "profilereport":
		return "profile"
	case "select-dtypes":
		return operation
	case "sort-values":
		return operation
	default:
		return operation
	}
}

func isDataOperation(operation string) bool {
	_, ok := dataOperations[operation]
	return ok
}

func loadDataFrame(ctx context.Context, parsed parsedArgs, argv []string, operation string) (dataframe.Frame, error) {
	input := parsed.value("input")
	if len(parsed.positionals) == 1 {
		if input != "" {
			return dataframe.Frame{}, fmt.Errorf("use either a positional input path or --input, not both")
		}
		input = parsed.positionals[0]
	}
	if input == "" {
		input = "-"
	}
	format := parsed.value("input-format")
	if operation == "read-csv" && !optionProvided(argv, "input-format") {
		format = "csv"
	}
	options := loadOptions(parsed, format)
	if input == "-" {
		return loadInterruptibly(ctx, os.Stdin, options)
	}
	return loadDataFile(ctx, input, parsed, format)
}

func loadDataFile(ctx context.Context, input string, parsed parsedArgs, format string) (dataframe.Frame, error) {
	file, err := openDataInput(input, parsed.value("input-root"))
	if err != nil {
		return dataframe.Frame{}, err
	}
	defer file.Close()
	options := loadOptions(parsed, format)
	if options.Format == "" || options.Format == "auto" {
		switch strings.ToLower(filepath.Ext(input)) {
		case ".csv":
			options.Format = "csv"
		case ".json":
			options.Format = "json"
		case ".jsonl", ".ndjson":
			options.Format = "jsonl"
		}
	}
	return loadInterruptibly(ctx, file, options)
}

func openDataInput(input, configuredRoot string) (*os.File, error) {
	root := strings.TrimSpace(configuredRoot)
	if root == "" {
		root = strings.TrimSpace(os.Getenv("MWX_INPUT_ROOT"))
	}
	if root == "" {
		return os.Open(input)
	}
	rootPath, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve dataframe input root: %w", err)
	}
	rootPath, err = filepath.EvalSymlinks(rootPath)
	if err != nil {
		return nil, fmt.Errorf("resolve dataframe input root: %w", err)
	}
	rootHandle, err := os.OpenRoot(rootPath)
	if err != nil {
		return nil, fmt.Errorf("open dataframe input root: %w", err)
	}
	defer rootHandle.Close()
	relative := input
	if filepath.IsAbs(input) {
		inputPath, resolveErr := filepath.EvalSymlinks(input)
		if resolveErr != nil {
			return nil, fmt.Errorf("resolve dataframe input path: %w", resolveErr)
		}
		relative, err = filepath.Rel(rootPath, inputPath)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
			return nil, fmt.Errorf("dataframe input is outside the configured input root")
		}
	}
	file, err := rootHandle.Open(relative)
	if err != nil {
		return nil, fmt.Errorf("open dataframe input within configured root: %w", err)
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("inspect dataframe input: %w", err)
	}
	if !info.Mode().IsRegular() {
		file.Close()
		return nil, fmt.Errorf("dataframe input must be a regular file when an input root is configured")
	}
	return file, nil
}

func loadInterruptibly(ctx context.Context, reader io.ReadCloser, options dataframe.LoadOptions) (dataframe.Frame, error) {
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = reader.Close()
		case <-done:
		}
	}()
	frame, err := dataframe.Load(contextReader{ctx: ctx, reader: reader}, options)
	close(done)
	if ctx.Err() != nil {
		return dataframe.Frame{}, ctx.Err()
	}
	return frame, err
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (reader contextReader) Read(buffer []byte) (int, error) {
	select {
	case <-reader.ctx.Done():
		return 0, reader.ctx.Err()
	default:
		return reader.reader.Read(buffer)
	}
}

func loadOptions(parsed parsedArgs, format string) dataframe.LoadOptions {
	return dataframe.LoadOptions{
		Format:     format,
		Path:       parsed.value("path"),
		Layout:     parsed.value("layout"),
		InferTypes: !parsed.flag("strings"),
	}
}

func requiredOption(parsed parsedArgs, name, operation string) (string, error) {
	value := strings.TrimSpace(parsed.value(name))
	if value == "" {
		return "", dataUsageError(fmt.Sprintf("%s requires --%s", operation, name))
	}
	return value, nil
}

func requiredColumns(parsed parsedArgs, operation string) ([]string, error) {
	columns := dataframe.SplitList(parsed.value("columns"))
	if len(columns) == 0 && parsed.value("column") != "" {
		columns = []string{parsed.value("column")}
	}
	if len(columns) == 0 {
		return nil, dataUsageError(fmt.Sprintf("%s requires --column or --columns", operation))
	}
	return columns, nil
}

func dataUsageError(message string) error {
	err := provider.NewError("invalid_arguments", message, 2)
	err.Hint = "Run mwx data --help for operation examples."
	return err
}

func dataInputError(err error) error {
	appErr := provider.NewError("invalid_data", err.Error(), 2)
	appErr.Hint = "Check --input-format, --path, and --layout, then retry."
	return appErr
}

func dataOperationError(err error) error {
	appErr := provider.NewError("data_operation_failed", err.Error(), 2)
	appErr.Hint = "Check the column names and operation options."
	return appErr
}

func writeFrameResult(result dataframe.Frame, err error, write func(dataframe.Frame) error) error {
	if err != nil {
		return dataOperationError(err)
	}
	return write(result)
}

func columnListFrame(columns []string) dataframe.Frame {
	return columnValuesFrame("column", stringsToAny(columns))
}

func columnValuesFrame(column string, values []any) dataframe.Frame {
	rows := make([][]any, len(values))
	for index, value := range values {
		rows[index] = []any{value}
	}
	frame, _ := dataframe.New([]string{column}, rows)
	return frame
}

func stringsToAny(values []string) []any {
	result := make([]any, len(values))
	for index, value := range values {
		result[index] = value
	}
	return result
}

func infoFrame(info dataframe.InfoResult) dataframe.Frame {
	rows := make([][]any, len(info.Schema))
	for index, column := range info.Schema {
		rows[index] = []any{column.Name, column.Type, int64(column.NonNull), int64(column.Null), int64(column.Distinct)}
	}
	frame, _ := dataframe.New([]string{"column", "type", "non_null", "null", "distinct"}, rows)
	return frame
}

func nuniqueFrame(frame dataframe.Frame, column string, dropNA bool) (dataframe.Frame, error) {
	columns := frame.Columns
	if column != "" {
		columns = []string{column}
	}
	rows := make([][]any, len(columns))
	for index, name := range columns {
		count, err := dataframe.NUnique(frame, name, dropNA)
		if err != nil {
			return dataframe.Frame{}, err
		}
		rows[index] = []any{name, int64(count)}
	}
	return dataframe.New([]string{"column", "nunique"}, rows)
}

func nullCountFrame(mask dataframe.Frame) dataframe.Frame {
	rows := make([][]any, len(mask.Columns))
	for columnIndex, column := range mask.Columns {
		count := int64(0)
		for _, row := range mask.Rows {
			if value, ok := row[columnIndex].(bool); ok && value {
				count++
			}
		}
		rows[columnIndex] = []any{column, count}
	}
	frame, _ := dataframe.New([]string{"column", "count"}, rows)
	return frame
}

func boolMaskFrame(column string, values []bool) dataframe.Frame {
	rows := make([][]any, len(values))
	for index, value := range values {
		rows[index] = []any{value}
	}
	frame, _ := dataframe.New([]string{column}, rows)
	return frame
}

func parseRenameMapping(value string) (map[string]string, error) {
	if strings.TrimSpace(value) == "" {
		return nil, fmt.Errorf("rename requires --mapping old:new[,old:new...]")
	}
	result := map[string]string{}
	for _, pair := range dataframe.SplitList(value) {
		from, to, ok := strings.Cut(pair, ":")
		if !ok || strings.TrimSpace(from) == "" || strings.TrimSpace(to) == "" {
			return nil, fmt.Errorf("invalid mapping %q, want old:new", pair)
		}
		result[strings.TrimSpace(from)] = strings.TrimSpace(to)
	}
	return result, nil
}

func parseValueMapping(value string) (map[string]any, error) {
	if strings.TrimSpace(value) == "" {
		return nil, fmt.Errorf("map requires --mapping old:new[,old:new...]")
	}
	result := map[string]any{}
	for _, pair := range dataframe.SplitList(value) {
		from, to, ok := strings.Cut(pair, ":")
		if !ok || strings.TrimSpace(from) == "" {
			return nil, fmt.Errorf("invalid mapping %q, want old:new", pair)
		}
		result[strings.TrimSpace(from)] = dataframe.ParseScalar(strings.TrimSpace(to))
	}
	return result, nil
}

func parseScalarList(value string, preserveStrings bool) []any {
	values := dataframe.SplitList(value)
	result := make([]any, len(values))
	for index, current := range values {
		if preserveStrings {
			result[index] = current
		} else {
			result[index] = dataframe.ParseScalar(current)
		}
	}
	return result
}

func parseAggregations(value string) ([]dataframe.Aggregation, error) {
	if strings.TrimSpace(value) == "" {
		return nil, fmt.Errorf("agg requires --agg column:function[:alias][,...]")
	}
	result := make([]dataframe.Aggregation, 0)
	for _, specification := range dataframe.SplitList(value) {
		parts := strings.Split(specification, ":")
		if len(parts) < 2 || len(parts) > 3 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			return nil, fmt.Errorf("invalid aggregation %q, want column:function[:alias]", specification)
		}
		aggregation := dataframe.Aggregation{Column: strings.TrimSpace(parts[0]), Func: strings.TrimSpace(parts[1])}
		if len(parts) == 3 {
			aggregation.As = strings.TrimSpace(parts[2])
		}
		result = append(result, aggregation)
	}
	return result, nil
}

func aggregationsFor(frame dataframe.Frame, by []string, value string) ([]dataframe.Aggregation, error) {
	if strings.TrimSpace(value) != "" {
		return parseAggregations(value)
	}
	return dataframe.MeanAggregations(frame, by)
}

func parseFloatList(value string) ([]float64, error) {
	parts := dataframe.SplitList(value)
	if len(parts) < 2 {
		return nil, fmt.Errorf("cut requires --bins edge,edge,... with at least two edges")
	}
	result := make([]float64, len(parts))
	for index, part := range parts {
		parsed, err := strconv.ParseFloat(part, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid bin edge %q", part)
		}
		result[index] = parsed
	}
	return result, nil
}

func parseSlice(value string, length int) (int, int, error) {
	if strings.TrimSpace(value) == "" || value == ":" {
		return 0, length, nil
	}
	if !strings.Contains(value, ":") {
		index, err := strconv.Atoi(value)
		if err != nil {
			return 0, 0, fmt.Errorf("%q is not an index or start:end slice", value)
		}
		if index < 0 {
			index += length
		}
		return index, index + 1, nil
	}
	startText, endText, _ := strings.Cut(value, ":")
	start, end := 0, length
	var err error
	if startText != "" {
		start, err = strconv.Atoi(startText)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid slice start %q", startText)
		}
	}
	if endText != "" {
		end, err = strconv.Atoi(endText)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid slice end %q", endText)
		}
	}
	return start, end, nil
}

func optionProvided(argv []string, name string) bool {
	long := "--" + name
	for _, argument := range argv {
		if argument == "--" {
			return false
		}
		if argument == long || strings.HasPrefix(argument, long+"=") {
			return true
		}
	}
	return false
}
