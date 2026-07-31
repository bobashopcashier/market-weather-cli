package dataframe

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"
)

func advancedFixture(t *testing.T) Frame {
	t.Helper()
	frame, err := New(
		[]string{"name", "group", "age", "fare"},
		[][]any{
			{"Ada", "A", int64(20), 10.0},
			{"Ben", "A", int64(30), 20.0},
			{"Cleo", "B", int64(40), nil},
			{"Dee", "B", int64(50), 40.0},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return frame
}

func TestDescribeAndInfo(t *testing.T) {
	frame := advancedFixture(t)
	described, err := Describe(frame, []string{"age"})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := described.Columns, []string{"stat", "age"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("columns = %#v, want %#v", got, want)
	}
	if got := described.Rows[1][1]; got != 35.0 {
		t.Fatalf("mean = %#v, want 35", got)
	}
	info := Info(frame)
	if info.Rows != 4 || info.Columns != 4 || info.Schema[3].Null != 1 {
		t.Fatalf("unexpected info: %#v", info)
	}
	single, _ := New([]string{"value"}, [][]any{{int64(1)}})
	singleDescription, err := Describe(single, nil)
	if err != nil {
		t.Fatal(err)
	}
	if singleDescription.Rows[2][1] != nil {
		t.Fatalf("single-value sample std = %#v, want null", singleDescription.Rows[2][1])
	}
}

func TestQueryLocAndApply(t *testing.T) {
	frame := advancedFixture(t)
	queried, err := Query(frame, "age >= 30 and group == 'A'")
	if err != nil {
		t.Fatal(err)
	}
	if len(queried.Rows) != 1 || queried.Rows[0][0] != "Ben" {
		t.Fatalf("unexpected query result: %#v", queried.Rows)
	}
	located, err := Loc(frame, "age >= 40", []string{"name", "age"})
	if err != nil {
		t.Fatal(err)
	}
	if len(located.Rows) != 2 || len(located.Columns) != 2 {
		t.Fatalf("unexpected loc result: %#v", located)
	}
	applied, err := Apply(frame, "age + fare", "total")
	if err != nil {
		t.Fatal(err)
	}
	if applied.Rows[1][4] != 50.0 {
		t.Fatalf("applied value = %#v, want 50", applied.Rows[1][4])
	}
}

func TestAggregateAndMeanAggregations(t *testing.T) {
	frame := advancedFixture(t)
	aggregated, err := Aggregate(frame, []string{"group"}, []Aggregation{
		{Column: "age", Func: "mean", As: "average_age"},
		{Column: "fare", Func: "max"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := aggregated.Columns, []string{"group", "average_age", "fare_max"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("columns = %#v, want %#v", got, want)
	}
	if aggregated.Rows[0][1] != 25.0 || aggregated.Rows[1][2] != 40.0 {
		t.Fatalf("unexpected aggregate rows: %#v", aggregated.Rows)
	}
	defaults, err := MeanAggregations(frame, []string{"group"})
	if err != nil {
		t.Fatal(err)
	}
	if len(defaults) != 2 {
		t.Fatalf("default aggregations = %#v", defaults)
	}
	withNull, _ := New([]string{"group", "value"}, [][]any{{"B", int64(2)}, {nil, int64(9)}, {"A", int64(1)}})
	pandasDefault, err := Aggregate(withNull, []string{"group"}, []Aggregation{{Column: "value", Func: "sum"}})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := pandasDefault.Rows, [][]any{{"A", 1.0}, {"B", 2.0}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("default groupby rows = %#v, want %#v", got, want)
	}
	includingNull, err := AggregateIncludingNullGroups(withNull, []string{"group"}, []Aggregation{{Column: "value", Func: "sum"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(includingNull.Rows) != 3 || includingNull.Rows[2][0] != nil {
		t.Fatalf("include-null groupby rows = %#v", includingNull.Rows)
	}
}

func TestCutIdxMaxAndDummies(t *testing.T) {
	frame := advancedFixture(t)
	cut, err := Cut(frame, "age", []float64{0, 29, 39, 100}, []string{"young", "middle", "older"}, "band")
	if err != nil {
		t.Fatal(err)
	}
	if got := []any{cut.Rows[0][4], cut.Rows[1][4], cut.Rows[2][4]}; !reflect.DeepEqual(got, []any{"young", "middle", "older"}) {
		t.Fatalf("cut labels = %#v", got)
	}
	maximum, err := IdxMax(frame, "age")
	if err != nil {
		t.Fatal(err)
	}
	if maximum.Index != 3 || maximum.Row["name"] != "Dee" {
		t.Fatalf("unexpected idxmax: %#v", maximum)
	}
	dummies, err := GetDummies(frame, []string{"group"}, "team")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := dummies.Columns, []string{"name", "age", "fare", "team_A", "team_B"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("dummy columns = %#v, want %#v", got, want)
	}
	boundaries, _ := New([]string{"value"}, [][]any{{int64(0)}, {int64(1)}, {int64(2)}})
	boundaryCut, err := Cut(boundaries, "value", []float64{0, 1, 2}, nil, "band")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := []any{boundaryCut.Rows[0][1], boundaryCut.Rows[1][1], boundaryCut.Rows[2][1]}, []any{nil, "(0,1]", "(1,2]"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("cut boundaries = %#v, want %#v", got, want)
	}
	collision, _ := New([]string{"group", "group_A"}, [][]any{{"A", "existing"}})
	collisionDummies, err := GetDummies(collision, []string{"group"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := collisionDummies.Columns, []string{"group_A", "group_A.1"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("dummy collision columns = %#v, want %#v", got, want)
	}
	if _, err := GetDummies(frame, []string{"group", "name"}, "category"); err == nil {
		t.Fatal("expected multi-column prefix error")
	}
}

func TestGetDummiesRejectsAmplifiedResultBeforeAllocation(t *testing.T) {
	rows := make([][]any, 2237)
	for index := range rows {
		rows[index] = []any{int64(index)}
	}
	frame, err := New([]string{"id"}, rows)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := GetDummies(frame, []string{"id"}, ""); err == nil || !strings.Contains(err.Error(), "safety limit") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNonFiniteValuesKeepOperationSemanticsAndProduceSafeResults(t *testing.T) {
	frame, _ := New([]string{"value", "other"}, [][]any{{math.Inf(1), math.NaN()}, {2.0, 1.0}})
	if NullMask(frame, false).Rows[0][0] != false {
		t.Fatal("infinity must not be treated as null")
	}
	maximum, err := IdxMax(frame, "value")
	if err != nil || maximum.Index != 0 {
		t.Fatalf("idxmax with infinity = %#v, err = %v", maximum, err)
	}
	if maximum.Row["value"] != nil || maximum.Row["other"] != nil {
		t.Fatalf("idxmax JSON row is not sanitized: %#v", maximum.Row)
	}
	if matrix := ToNumpy(frame); matrix[0][0] != nil || matrix[0][1] != nil {
		t.Fatalf("to-numpy JSON matrix is not sanitized: %#v", matrix)
	}
	if _, err := json.Marshal(Profile(frame)); err != nil {
		t.Fatalf("profile is not JSON-safe: %v", err)
	}
}

func TestConcatAndProfile(t *testing.T) {
	left, _ := New([]string{"a"}, [][]any{{int64(1)}, {int64(2)}})
	right, _ := New([]string{"b"}, [][]any{{"x"}})
	columns, err := Concat([]Frame{left, right}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(columns.Rows) != 2 || columns.Rows[1][1] != nil {
		t.Fatalf("axis=1 concat = %#v", columns)
	}
	rows, err := Concat([]Frame{left, right}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := rows.Columns, []string{"a", "b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("axis=0 columns = %#v, want %#v", got, want)
	}
	duplicateNames, err := Concat([]Frame{left, left, {Columns: []string{"a.1"}, Rows: [][]any{{int64(3)}}}}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := duplicateNames.Columns, []string{"a", "a.1", "a.1.1"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("axis=1 collision columns = %#v, want %#v", got, want)
	}
	duplicate, _ := New([]string{"a"}, [][]any{{int64(1)}, {int64(1)}})
	if profile := Profile(duplicate); profile.Duplicates != 1 || profile.Shape != [2]int{2, 1} {
		t.Fatalf("unexpected profile: %#v", profile)
	}
}
