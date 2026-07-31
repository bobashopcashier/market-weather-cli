package dataframe

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadCSV(t *testing.T) {
	input := "name,age,active,note\nAda,37,true,\nBob,40,false,NA\n"
	frame, err := Load(strings.NewReader(input), LoadOptions{Format: "csv", InferTypes: true})
	if err != nil {
		t.Fatal(err)
	}
	want := Frame{
		Columns: []string{"name", "age", "active", "note"},
		Rows: [][]any{
			{"Ada", int64(37), true, nil},
			{"Bob", int64(40), false, nil},
		},
	}
	if !reflect.DeepEqual(frame, want) {
		t.Fatalf("Load CSV = %#v, want %#v", frame, want)
	}
}

func TestLoadCSVWithoutInferencePreservesStrings(t *testing.T) {
	frame, err := Load(strings.NewReader("id,value\n001,true\n"), LoadOptions{Format: "csv"})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := frame.Rows[0], []any{"001", "true"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("row = %#v, want %#v", got, want)
	}
}

func TestAutoDetectFallsBackToCSVForBracketHeader(t *testing.T) {
	frame, err := Load(strings.NewReader("[metric],value\nfoo,1\n"), LoadOptions{Format: "auto", InferTypes: true})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := frame.Columns, []string{"[metric]", "value"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("columns = %#v, want %#v", got, want)
	}
}

func TestAutoDetectFallsBackToSingleColumnBracketCSV(t *testing.T) {
	frame, err := Load(strings.NewReader("[metric]\nfoo\n"), LoadOptions{Format: "auto"})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := frame, (Frame{Columns: []string{"[metric]"}, Rows: [][]any{{"foo"}}}); !reflect.DeepEqual(got, want) {
		t.Fatalf("frame = %#v, want %#v", got, want)
	}
}

func TestLoadJSONRecordsDeterministicColumns(t *testing.T) {
	frame, err := Load(strings.NewReader(`[{"b":2,"a":1},{"a":3,"c":4}]`), LoadOptions{Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := frame.Columns, []string{"a", "b", "c"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("columns = %#v, want %#v", got, want)
	}
	wantRows := [][]any{{json.Number("1"), json.Number("2"), nil}, {json.Number("3"), nil, json.Number("4")}}
	if !reflect.DeepEqual(frame.Rows, wantRows) {
		t.Fatalf("rows = %#v, want %#v", frame.Rows, wantRows)
	}
}

func TestLoadJSONPreservesNAString(t *testing.T) {
	frame, err := Load(strings.NewReader(`[{"region":"NA"}]`), LoadOptions{Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	if IsNull(frame.Rows[0][0]) || frame.Rows[0][0] != "NA" {
		t.Fatalf("JSON string was treated as null: %#v", frame.Rows[0][0])
	}
}

func TestLoadJSONSingleRecordAndColumnsLayout(t *testing.T) {
	record, err := Load(strings.NewReader(`{"name":"Ada","age":37}`), LoadOptions{Layout: "records"})
	if err != nil {
		t.Fatal(err)
	}
	if len(record.Rows) != 1 || len(record.Columns) != 2 {
		t.Fatalf("unexpected record frame: %#v", record)
	}

	columns, err := Load(strings.NewReader(`{"city":["SFO","NYC"],"temp":[60,70]}`), LoadOptions{Layout: "columns"})
	if err != nil {
		t.Fatal(err)
	}
	want := Frame{
		Columns: []string{"city", "temp"},
		Rows:    [][]any{{"SFO", json.Number("60")}, {"NYC", json.Number("70")}},
	}
	if !reflect.DeepEqual(columns, want) {
		t.Fatalf("columns frame = %#v, want %#v", columns, want)
	}
}

func TestAutoColumnsRejectsUnequalArrays(t *testing.T) {
	_, err := Load(strings.NewReader(`{"time":["a","b"],"temperature":[72]}`), LoadOptions{Format: "json"})
	if err == nil || !strings.Contains(err.Error(), "has 2 values, want 1") && !strings.Contains(err.Error(), "has 1 values, want 2") {
		t.Fatalf("unequal column arrays error = %v", err)
	}
}

func TestVendorSchemaVersionIsAnOrdinaryRecord(t *testing.T) {
	frame, err := Load(strings.NewReader(`{"schemaVersion":"vendor.v2","value":1}`), LoadOptions{Format: "json", Layout: "records"})
	if err != nil {
		t.Fatal(err)
	}
	if len(frame.Rows) != 1 || frame.Rows[0][0] != "vendor.v2" {
		t.Fatalf("vendor record = %#v", frame)
	}
}

func TestLoadJSONPointerRFC6901Escapes(t *testing.T) {
	input := `{"forecast":{"a/b":{"~daily":[{"time":"2026-07-31","high":72}]}}}`
	frame, err := Load(strings.NewReader(input), LoadOptions{Format: "json", Path: "/forecast/a~1b/~0daily"})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := frame.Columns, []string{"high", "time"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("columns = %#v, want %#v", got, want)
	}
}

func TestLoadCanonicalEnvelopePreservesColumnOrder(t *testing.T) {
	input := `{
  "schemaVersion":"mwx.table/v1",
  "columns":[{"name":"z","type":"string"},{"name":"a","type":"number"}],
  "rows":[["last",1]],
  "meta":{"rowCount":1,"columnCount":2}
}`
	frame, err := Load(strings.NewReader(input), LoadOptions{Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	want := Frame{Columns: []string{"z", "a"}, Rows: [][]any{{"last", json.Number("1")}}}
	if !reflect.DeepEqual(frame, want) {
		t.Fatalf("frame = %#v, want %#v", frame, want)
	}
}

func TestLoadJSONLAndAutoFallback(t *testing.T) {
	input := "{\"station\":\"KSFO\",\"temp\":18}\n{\"station\":\"KJFK\",\"temp\":21}\n"
	frame, err := Load(strings.NewReader(input), LoadOptions{Format: "auto"})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(frame.Rows), 2; got != want {
		t.Fatalf("row count = %d, want %d", got, want)
	}
	if got, want := frame.Columns, []string{"station", "temp"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("columns = %#v, want %#v", got, want)
	}
}

func TestLoadJSONLAppliesPathPerRecord(t *testing.T) {
	input := "{\"data\":{\"x\":1}}\n{\"data\":{\"x\":2}}\n"
	frame, err := Load(strings.NewReader(input), LoadOptions{Format: "jsonl", Path: "/data"})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := frame.Rows, [][]any{{json.Number("1")}, {json.Number("2")}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("rows = %#v, want %#v", got, want)
	}
}

func TestLoadFileInfersFormatFromExtension(t *testing.T) {
	path := filepath.Join(t.TempDir(), "values.csv")
	if err := os.WriteFile(path, []byte("x\n1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	frame, err := LoadFile(path, LoadOptions{InferTypes: true})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := frame.Rows[0][0], int64(1); got != want {
		t.Fatalf("value = %#v, want %#v", got, want)
	}
}

func TestWriteJSONCanonicalAndRoundTrip(t *testing.T) {
	frame, err := New([]string{"name", "score", "missing"}, [][]any{{"Ada", 9.5, nil}, {"Bob", math.NaN(), true}})
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := Write(&output, frame, OutputOptions{Format: "json"}); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	for _, expected := range []string{
		`"schemaVersion": "mwx.table/v1"`,
		`"name": "score"`,
		`"type": "number"`,
		`"rowCount": 2`,
		`"columnCount": 3`,
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("JSON output missing %q:\n%s", expected, text)
		}
	}
	if strings.Contains(text, "NaN") {
		t.Fatalf("JSON output contains invalid NaN: %s", text)
	}
	roundTrip, err := Load(strings.NewReader(text), LoadOptions{Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := roundTrip.Columns, frame.Columns; !reflect.DeepEqual(got, want) {
		t.Fatalf("round-trip columns = %#v, want %#v", got, want)
	}
	if roundTrip.Rows[1][1] != nil {
		t.Fatalf("NaN round-trip value = %#v, want nil", roundTrip.Rows[1][1])
	}
}

func TestWriteJSONPreservesIntegralFloatKind(t *testing.T) {
	frame, _ := New([]string{"value"}, [][]any{{7.0}})
	var output bytes.Buffer
	if err := Write(&output, frame, OutputOptions{Format: "json"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "7.0") {
		t.Fatalf("integral float encoding = %s", output.String())
	}
	roundTrip, err := Load(strings.NewReader(output.String()), LoadOptions{Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	selected, err := SelectDTypes(roundTrip, []string{"float"}, nil)
	if err != nil || len(selected.Columns) != 1 {
		t.Fatalf("float selection after round trip = %#v, err = %v", selected.Columns, err)
	}
}

func TestWriteCSVAndReload(t *testing.T) {
	frame := Frame{Columns: []string{"name", "note", "value"}, Rows: [][]any{{"Ada", "hello, world", int64(7)}, {"Bob", nil, false}}}
	var output bytes.Buffer
	if err := Write(&output, frame, OutputOptions{Format: "csv"}); err != nil {
		t.Fatal(err)
	}
	wantCSV := "name,note,value\nAda,\"hello, world\",7\nBob,,false\n"
	if got := output.String(); got != wantCSV {
		t.Fatalf("CSV = %q, want %q", got, wantCSV)
	}
	reloaded, err := Load(strings.NewReader(output.String()), LoadOptions{Format: "csv", InferTypes: true})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(reloaded, frame) {
		t.Fatalf("reloaded = %#v, want %#v", reloaded, frame)
	}
}

func TestWriteTable(t *testing.T) {
	frame := Frame{Columns: []string{"CITY", "TEMP"}, Rows: [][]any{{"München", 12}, {"SF", nil}}}
	var output bytes.Buffer
	if err := Write(&output, frame, OutputOptions{Format: "table"}); err != nil {
		t.Fatal(err)
	}
	want := "CITY     TEMP\n-------  ----\nMünchen  12\nSF\n"
	if got := output.String(); got != want {
		t.Fatalf("table = %q, want %q", got, want)
	}
}

func TestLoadRejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		options LoadOptions
		want    string
	}{
		{name: "missing CSV header", input: "", options: LoadOptions{Format: "csv"}, want: "header"},
		{name: "unequal CSV row", input: "a,b\n1\n", options: LoadOptions{Format: "csv"}, want: "record on line 2"},
		{name: "unequal columns", input: `{"a":[1],"b":[2,3]}`, options: LoadOptions{Format: "json", Layout: "columns"}, want: "want 1"},
		{name: "bad pointer", input: `{"a":[]}`, options: LoadOptions{Format: "json", Path: "a"}, want: "must be empty or start with /"},
		{name: "missing pointer", input: `{"a":[]}`, options: LoadOptions{Format: "json", Path: "/b"}, want: "does not exist"},
		{name: "bad record", input: `[1]`, options: LoadOptions{Format: "json"}, want: "want an object"},
		{name: "bad layout", input: `{}`, options: LoadOptions{Format: "json", Layout: "diagonal"}, want: "unsupported JSON layout"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Load(strings.NewReader(test.input), test.options)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestWriteRejectsInvalidFrameAndFormat(t *testing.T) {
	if err := Write(&bytes.Buffer{}, Frame{Columns: []string{"x"}, Rows: [][]any{{1, 2}}}, OutputOptions{}); err == nil {
		t.Fatal("expected invalid frame error")
	}
	if err := Write(&bytes.Buffer{}, Frame{}, OutputOptions{Format: "parquet"}); err == nil {
		t.Fatal("expected unsupported format error")
	}
}
