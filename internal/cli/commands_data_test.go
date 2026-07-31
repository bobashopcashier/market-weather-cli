package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/bobashopcashier/market-weather-cli/internal/dataframe"
)

func TestAllNotebookOperationsAreExposed(t *testing.T) {
	operations := []string{
		"read-csv", "columns", "head", "tail", "shape", "info", "describe", "select-dtypes",
		"astype", "value-counts", "unique", "nunique", "isnull", "notnull", "duplicated", "drop-duplicates",
		"rename", "map", "query", "isin", "drop", "fillna", "dropna", "groupby", "agg", "sort-values",
		"loc", "iloc", "cut", "apply", "profile", "idxmax", "get-dummies", "concat", "to-numpy",
	}
	if len(operations) != 35 {
		t.Fatalf("operation list has %d entries, want 35", len(operations))
	}
	for _, operation := range operations {
		if !isDataOperation(operation) {
			t.Errorf("operation %q is not exposed", operation)
		}
		if !strings.Contains(toolHelp["data"], operation) {
			t.Errorf("operation %q is missing from help", operation)
		}
	}
	for input, want := range map[string]string{
		"read_csv":      "read-csv",
		"value_counts":  "value-counts",
		"ProfileReport": "profile",
		"get_dummies":   "get-dummies",
		"to_numpy":      "to-numpy",
	} {
		if got := normalizeDataOperation(input); got != want {
			t.Errorf("normalizeDataOperation(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestEveryDataOperationRunsThroughCLI(t *testing.T) {
	directory := t.TempDir()
	input := filepath.Join(directory, "passengers.csv")
	contents := "name,group,age,fare,active\nAda,A,20,10,true\nBen,A,30,,false\nCleo,B,40,30,true\nCleo,B,40,30,true\n"
	if err := os.WriteFile(input, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		operation string
		options   []string
	}{
		{"read-csv", nil},
		{"columns", nil},
		{"head", []string{"--n", "2"}},
		{"tail", []string{"--n", "2"}},
		{"shape", nil},
		{"info", nil},
		{"describe", []string{"--columns", "age,fare"}},
		{"select-dtypes", []string{"--include", "number"}},
		{"astype", []string{"--columns", "age", "--dtype", "float"}},
		{"value-counts", []string{"--column", "group"}},
		{"unique", []string{"--column", "group"}},
		{"nunique", nil},
		{"isnull", []string{"--sum"}},
		{"notnull", nil},
		{"duplicated", nil},
		{"drop-duplicates", nil},
		{"rename", []string{"--mapping", "age:years"}},
		{"map", []string{"--column", "group", "--mapping", "A:alpha,B:beta"}},
		{"query", []string{"--expr", "age >= 30"}},
		{"isin", []string{"--column", "group", "--values", "A,C"}},
		{"drop", []string{"--columns", "active"}},
		{"fillna", []string{"--columns", "fare", "--strategy", "mean"}},
		{"dropna", []string{"--subset", "fare"}},
		{"groupby", []string{"--by", "group"}},
		{"agg", []string{"--agg", "age:mean"}},
		{"sort-values", []string{"--by", "age", "--descending"}},
		{"loc", []string{"--where", "age >= 30", "--columns", "name,age"}},
		{"iloc", []string{"--rows", "0:2", "--cols", "0:2"}},
		{"cut", []string{"--column", "age", "--bins", "0,25,50", "--labels", "young,adult"}},
		{"apply", []string{"--expr", "age + fare", "--output-column", "total"}},
		{"profile", nil},
		{"idxmax", []string{"--column", "age"}},
		{"get-dummies", []string{"--columns", "group"}},
		{"concat", []string{"--with", input, "--axis", "0"}},
		{"to-numpy", nil},
	}
	if len(tests) != 35 {
		t.Fatalf("integration table has %d operations, want 35", len(tests))
	}
	for _, test := range tests {
		t.Run(test.operation, func(t *testing.T) {
			arguments := []string{test.operation, input}
			arguments = append(arguments, test.options...)
			output, err := captureDataOutput(t, arguments)
			if err != nil {
				t.Fatalf("runData(%q): %v", test.operation, err)
			}
			if !json.Valid(output) {
				t.Fatalf("%s output is not JSON: %s", test.operation, output)
			}
		})
	}
}

func TestDataArgumentHelpers(t *testing.T) {
	rename, err := parseRenameMapping("a:alpha,b:beta")
	if err != nil || !reflect.DeepEqual(rename, map[string]string{"a": "alpha", "b": "beta"}) {
		t.Fatalf("rename mapping = %#v, err = %v", rename, err)
	}
	values, err := parseValueMapping("yes:true,no:0,missing:null")
	if err != nil || values["yes"] != true || values["no"] != int64(0) || values["missing"] != nil {
		t.Fatalf("value mapping = %#v, err = %v", values, err)
	}
	aggregations, err := parseAggregations("age:mean:average_age,fare:max")
	if err != nil || len(aggregations) != 2 || aggregations[0].As != "average_age" {
		t.Fatalf("aggregations = %#v, err = %v", aggregations, err)
	}
	start, end, err := parseSlice("-3:-1", 10)
	if err != nil || start != -3 || end != -1 {
		t.Fatalf("slice = %d:%d, err = %v", start, end, err)
	}
}

func TestUnknownDataOperationFailsBeforeReadingInput(t *testing.T) {
	err := runData(context.Background(), []string{"not-a-real-operation"})
	if err == nil || !strings.Contains(err.Error(), "unknown dataframe operation") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDataRejectsIrrelevantOperationOptions(t *testing.T) {
	err := runData(context.Background(), []string{"head", "--agg", "age:mean"})
	if err == nil || !strings.Contains(err.Error(), "unknown option") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDataValidatesRequiredOptionsBeforeReadingInput(t *testing.T) {
	err := runData(context.Background(), []string{"query", "/definitely/not/a/real/input.csv"})
	if err == nil || !strings.Contains(err.Error(), "query requires --expr") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDataOutputProjectionLimitAndCompactJSON(t *testing.T) {
	directory := t.TempDir()
	input := filepath.Join(directory, "values.csv")
	if err := os.WriteFile(input, []byte("name,value,ignored\na,1,x\nb,2,y\nc,3,z\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := captureDataOutput(t, []string{"head", input, "--n", "3", "--fields", "name,value", "--limit", "2", "--compact"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(output), "\n") != 1 {
		t.Fatalf("compact JSON should be one line: %q", output)
	}
	var envelope struct {
		Columns []struct {
			Name string `json:"name"`
		} `json:"columns"`
		Rows [][]any `json:"rows"`
		Meta struct {
			RowCount       int  `json:"rowCount"`
			SourceRowCount int  `json:"sourceRowCount"`
			Truncated      bool `json:"truncated"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(output, &envelope); err != nil {
		t.Fatal(err)
	}
	if len(envelope.Columns) != 2 || envelope.Columns[0].Name != "name" || len(envelope.Rows) != 2 {
		t.Fatalf("unexpected projection: %#v", envelope)
	}
	if envelope.Meta.RowCount != 2 || envelope.Meta.SourceRowCount != 3 || !envelope.Meta.Truncated {
		t.Fatalf("missing truncation metadata: %#v", envelope.Meta)
	}
}

func TestToNumpySupportsProjectionAndLimit(t *testing.T) {
	directory := t.TempDir()
	input := filepath.Join(directory, "values.csv")
	if err := os.WriteFile(input, []byte("a,b,c\n1,2,3\n4,5,6\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := captureDataOutput(t, []string{"to-numpy", input, "--fields", "a,c", "--limit", "1", "--compact"})
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Data [][]any `json:"data"`
		Meta struct {
			RowCount       int  `json:"rowCount"`
			SourceRowCount int  `json:"sourceRowCount"`
			Truncated      bool `json:"truncated"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(output, &envelope); err != nil {
		t.Fatal(err)
	}
	if len(envelope.Data) != 1 || len(envelope.Data[0]) != 2 || envelope.Meta.SourceRowCount != 2 || !envelope.Meta.Truncated {
		t.Fatalf("unexpected bounded matrix: %#v", envelope)
	}
}

func TestLimitAndStructuredFormatsRejectBeforeReading(t *testing.T) {
	for _, arguments := range [][]string{
		{"head", "/not/a/real/file.csv", "--limit", "1", "--output", "csv"},
		{"profile", "/not/a/real/file.csv", "--output", "table"},
	} {
		if err := runData(context.Background(), arguments); err == nil || !strings.Contains(err.Error(), "output") {
			t.Fatalf("unexpected error for %#v: %v", arguments, err)
		}
	}
}

func TestDataInputRootBlocksTraversalAndSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "inside.csv")
	if err := os.WriteFile(inside, []byte("x\n1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := openDataInput("inside.csv", root)
	if err != nil {
		t.Fatalf("open inside path: %v", err)
	}
	file.Close()
	outsideDirectory := t.TempDir()
	outside := filepath.Join(outsideDirectory, "outside.csv")
	if err := os.WriteFile(outside, []byte("x\n2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := openDataInput(outside, root); err == nil || !strings.Contains(err.Error(), "outside") {
		t.Fatalf("absolute escape was not rejected: %v", err)
	}
	link := filepath.Join(root, "escape.csv")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	if _, err := openDataInput("escape.csv", root); err == nil {
		t.Fatalf("symlink escape was not rejected: %v", err)
	}
	if _, err := openDataInput(".", root); err == nil || !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("directory input was not rejected: %v", err)
	}
}

func TestConcatBudgetRejectsAmplifiedRetainedFrames(t *testing.T) {
	budget := concatInputBudget{}
	frame := dataframe.Frame{Columns: make([]string, 2000), Rows: make([][]any, 3000)}
	if err := budget.add(frame); err == nil || !strings.Contains(err.Error(), "cell") {
		t.Fatalf("unexpected budget error: %v", err)
	}
}

func TestStructuredResultRejectsNonJSONOutput(t *testing.T) {
	directory := t.TempDir()
	input := filepath.Join(directory, "values.csv")
	if err := os.WriteFile(input, []byte("x\n1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := captureDataOutput(t, []string{"to-numpy", input, "--output", "table"})
	if err == nil || !strings.Contains(err.Error(), "--output must be one of: json") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStringsPreserveNumericLookingScalarOptions(t *testing.T) {
	directory := t.TempDir()
	input := filepath.Join(directory, "identifiers.csv")
	if err := os.WriteFile(input, []byte("id,value\n001,\n1,present\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	isinOutput, err := captureDataOutput(t, []string{"isin", input, "--strings", "--column", "id", "--values", "001"})
	if err != nil {
		t.Fatal(err)
	}
	isinRows := decodeTableRows(t, isinOutput)
	if !reflect.DeepEqual(isinRows, [][]any{{true}, {false}}) {
		t.Fatalf("string isin rows = %#v", isinRows)
	}
	fillOutput, err := captureDataOutput(t, []string{"fillna", input, "--strings", "--columns", "value", "--value", "001"})
	if err != nil {
		t.Fatal(err)
	}
	fillRows := decodeTableRows(t, fillOutput)
	if fillRows[0][1] != "001" {
		t.Fatalf("string fill value = %#v", fillRows[0][1])
	}
}

func TestContextReaderStopsBeforeReadWhenCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	reader := contextReader{ctx: ctx, reader: strings.NewReader("ignored")}
	buffer := make([]byte, 8)
	if _, err := reader.Read(buffer); err != context.Canceled {
		t.Fatalf("context reader error = %v", err)
	}
}

func TestInterruptibleLoadClosesBlockingReaderOnCancellation(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, loadErr := loadInterruptibly(ctx, reader, dataframe.LoadOptions{Format: "csv"})
		result <- loadErr
	}()
	cancel()
	select {
	case loadErr := <-result:
		if loadErr != context.Canceled {
			t.Fatalf("blocking load error = %v", loadErr)
		}
	case <-time.After(time.Second):
		t.Fatal("blocking load did not stop after cancellation")
	}
}

func decodeTableRows(t *testing.T, data []byte) [][]any {
	t.Helper()
	var envelope struct {
		Rows [][]any `json:"rows"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatal(err)
	}
	return envelope.Rows
}

func captureDataOutput(t *testing.T, arguments []string) ([]byte, error) {
	t.Helper()
	output, err := os.CreateTemp(t.TempDir(), "data-output-*.json")
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stdout
	os.Stdout = output
	defer func() { os.Stdout = original }()
	runErr := runData(context.Background(), arguments)
	if closeErr := output.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	data, readErr := os.ReadFile(output.Name())
	if readErr != nil {
		t.Fatal(readErr)
	}
	return data, runErr
}
