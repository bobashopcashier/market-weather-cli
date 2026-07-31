package dataframe

import (
	"math"
	"reflect"
	"testing"
)

func basicFixture(t *testing.T) Frame {
	t.Helper()
	frame, err := New(
		[]string{"name", "age", "score", "active", "group"},
		[][]any{
			{"Ada", int64(30), 9.5, true, "a"},
			{"Bob", nil, 7.0, false, "b"},
			{"Ada", int64(30), math.NaN(), true, "a"},
			{"Cy", int64(20), 7.0, false, nil},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return frame
}

func TestHeadAndTail(t *testing.T) {
	frame := basicFixture(t)
	if got := Head(frame, 2); len(got.Rows) != 2 || got.Rows[1][0] != "Bob" {
		t.Fatalf("unexpected head: %#v", got.Rows)
	}
	if got := Head(frame, -1); len(got.Rows) != 3 || got.Rows[2][0] != "Ada" {
		t.Fatalf("unexpected negative head: %#v", got.Rows)
	}
	if got := Tail(frame, 2); len(got.Rows) != 2 || got.Rows[0][0] != "Ada" {
		t.Fatalf("unexpected tail: %#v", got.Rows)
	}
	if got := Tail(frame, -1); len(got.Rows) != 3 || got.Rows[0][0] != "Bob" {
		t.Fatalf("unexpected negative tail: %#v", got.Rows)
	}
	if got := Head(frame, 99); len(got.Rows) != 4 {
		t.Fatalf("head should clamp: %#v", got.Rows)
	}
}

func TestSelectColumnsAndDTypes(t *testing.T) {
	frame := basicFixture(t)
	selected, err := SelectColumns(frame, []string{"active", "name"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(selected.Columns, []string{"active", "name"}) || !reflect.DeepEqual(selected.Rows[0], []any{true, "Ada"}) {
		t.Fatalf("unexpected selected frame: %#v", selected)
	}
	if _, err := SelectColumns(frame, []string{"missing"}); err == nil {
		t.Fatal("expected unknown column error")
	}

	numeric, err := SelectDTypes(frame, []string{"number"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(numeric.Columns, []string{"age", "score"}) {
		t.Fatalf("unexpected numeric columns: %#v", numeric.Columns)
	}
	nonObject, err := SelectDTypes(frame, nil, []string{"object"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(nonObject.Columns, []string{"age", "score", "active"}) {
		t.Fatalf("unexpected excluded columns: %#v", nonObject.Columns)
	}
	if _, err := SelectDTypes(frame, nil, nil); err == nil {
		t.Fatal("expected missing selector error")
	}
	if _, err := SelectDTypes(frame, []string{"datetime"}, nil); err == nil {
		t.Fatal("expected unsupported selector error")
	}
}

func TestCast(t *testing.T) {
	frame, err := New([]string{"value"}, [][]any{{"12.8"}, {true}, {nil}})
	if err != nil {
		t.Fatal(err)
	}
	integers, err := Cast(frame, "value", "int64")
	if err != nil || !reflect.DeepEqual(integers.Rows, [][]any{{int64(12)}, {int64(1)}, {nil}}) {
		t.Fatalf("unexpected mixed integer cast: %#v err=%v", integers.Rows, err)
	}

	numbers, _ := New([]string{"value"}, [][]any{{"12.8"}, {2.2}, {nil}})
	integers, err = Cast(numbers, "value", "int")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(integers.Rows, [][]any{{int64(12)}, {int64(2)}, {nil}}) {
		t.Fatalf("unexpected integer cast: %#v", integers.Rows)
	}
	limits, _ := New([]string{"value"}, [][]any{{"9223372036854775807"}, {int64(-9223372036854775807 - 1)}})
	limits, err = Cast(limits, "value", "int")
	if err != nil || !reflect.DeepEqual(limits.Rows, [][]any{{int64(9223372036854775807)}, {int64(-9223372036854775807 - 1)}}) {
		t.Fatalf("unexpected int64 limit cast: %#v err=%v", limits.Rows, err)
	}
	overflow, _ := New([]string{"value"}, [][]any{{"9223372036854775808"}})
	if _, err := Cast(overflow, "value", "int"); err == nil {
		t.Fatal("expected int64 overflow error")
	}
	booleans, _ := New([]string{"value"}, [][]any{{"false"}, {"true"}, {"0"}, {"2"}})
	booleans, err = Cast(booleans, "value", "bool")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(booleans.Rows, [][]any{{false}, {true}, {false}, {true}}) {
		t.Fatalf("unexpected bool cast: %#v", booleans.Rows)
	}
	if _, err := Cast(frame, "value", "datetime"); err == nil {
		t.Fatal("expected unsupported cast error")
	}
}

func TestCountsUniqueAndNUnique(t *testing.T) {
	frame := basicFixture(t)
	counts, err := ValueCounts(frame, "group", false)
	if err != nil {
		t.Fatal(err)
	}
	wantCounts := [][]any{{"a", int64(2)}, {"b", int64(1)}, {nil, int64(1)}}
	if !reflect.DeepEqual(counts.Rows, wantCounts) {
		t.Fatalf("unexpected counts: %#v", counts.Rows)
	}
	unique, err := Unique(frame, "group")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(unique, []any{"a", "b", nil}) {
		t.Fatalf("unexpected unique values: %#v", unique)
	}
	if count, err := NUnique(frame, "group", true); err != nil || count != 2 {
		t.Fatalf("unexpected nunique: count=%d err=%v", count, err)
	}
	if count, err := NUnique(frame, "group", false); err != nil || count != 3 {
		t.Fatalf("unexpected nunique with null: count=%d err=%v", count, err)
	}
}

func TestNullMask(t *testing.T) {
	frame := basicFixture(t)
	nulls := NullMask(frame, false)
	if nulls.Rows[1][1] != true || nulls.Rows[0][1] != false || nulls.Rows[2][2] != true {
		t.Fatalf("unexpected null mask: %#v", nulls.Rows)
	}
	notNulls := NullMask(frame, true)
	if notNulls.Rows[1][1] != false || notNulls.Rows[0][1] != true {
		t.Fatalf("unexpected not-null mask: %#v", notNulls.Rows)
	}
}

func TestDuplicatedMaskAndDropDuplicates(t *testing.T) {
	frame, _ := New([]string{"a", "b"}, [][]any{{1, "x"}, {1, "y"}, {1, "x"}, {2, "x"}, {1, "x"}})
	first, err := DuplicatedMask(frame, nil, "first")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, []bool{false, false, true, false, true}) {
		t.Fatalf("unexpected first mask: %#v", first)
	}
	last, _ := DuplicatedMask(frame, nil, "last")
	if !reflect.DeepEqual(last, []bool{true, false, true, false, false}) {
		t.Fatalf("unexpected last mask: %#v", last)
	}
	none, _ := DuplicatedMask(frame, []string{"a"}, "none")
	if !reflect.DeepEqual(none, []bool{true, true, true, false, true}) {
		t.Fatalf("unexpected none mask: %#v", none)
	}
	deduplicated, err := DropDuplicates(frame, nil, "first")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(deduplicated.Rows, [][]any{{1, "x"}, {1, "y"}, {2, "x"}}) {
		t.Fatalf("unexpected deduplicated rows: %#v", deduplicated.Rows)
	}
	if _, err := DuplicatedMask(frame, nil, "middle"); err == nil {
		t.Fatal("expected invalid keep error")
	}
}

func TestRenameMapIsInAndDropColumns(t *testing.T) {
	frame := basicFixture(t)
	renamed, err := Rename(frame, map[string]string{"group": "cohort", "active": "enabled"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(renamed.Columns, []string{"name", "age", "score", "enabled", "cohort"}) {
		t.Fatalf("unexpected renamed columns: %#v", renamed.Columns)
	}
	if _, err := Rename(frame, map[string]string{"group": "name"}); err == nil {
		t.Fatal("expected duplicate rename error")
	}

	mapped, err := MapColumn(frame, "group", map[string]any{"a": "A", "null": "unknown"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual([]any{mapped.Rows[0][4], mapped.Rows[1][4], mapped.Rows[3][4]}, []any{"A", nil, "unknown"}) {
		t.Fatalf("unexpected mapped values: %#v", mapped.Rows)
	}
	kept, err := MapColumn(frame, "group", map[string]any{"a": "A"}, true)
	if err != nil || kept.Rows[1][4] != "b" {
		t.Fatalf("unexpected keep-unmapped result: %#v err=%v", kept.Rows, err)
	}

	mask, err := IsIn(frame, "group", []any{"a", nil})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(mask, []bool{true, false, true, true}) {
		t.Fatalf("unexpected isin mask: %#v", mask)
	}
	dropped, err := DropColumns(frame, []string{"age", "active"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(dropped.Columns, []string{"name", "score", "group"}) {
		t.Fatalf("unexpected remaining columns: %#v", dropped.Columns)
	}
}

func TestFillNA(t *testing.T) {
	frame := basicFixture(t)
	literal, err := FillNA(frame, "age", FillLiteral, int64(99))
	if err != nil || literal.Rows[1][1] != int64(99) {
		t.Fatalf("unexpected literal fill: %#v err=%v", literal.Rows, err)
	}
	mean, err := FillNA(frame, "age", FillMean, nil)
	if err != nil {
		t.Fatal(err)
	}
	if mean.Rows[1][1] != float64(80)/3 {
		t.Fatalf("unexpected mean fill: %#v", mean.Rows[1][1])
	}
	mode, err := FillNA(frame, "group", FillMode, nil)
	if err != nil || mode.Rows[3][4] != "a" {
		t.Fatalf("unexpected mode fill: %#v err=%v", mode.Rows, err)
	}
	if _, err := FillNA(frame, "name", FillMean, nil); err == nil {
		t.Fatal("expected non-numeric mean error")
	}
	allNull, _ := New([]string{"x"}, [][]any{{nil}})
	if _, err := FillNA(allNull, "x", FillMode, nil); err == nil {
		t.Fatal("expected empty mode error")
	}
}

func TestDropNA(t *testing.T) {
	frame := basicFixture(t)
	any, err := DropNA(frame, []string{"age", "group"}, "any")
	if err != nil {
		t.Fatal(err)
	}
	if len(any.Rows) != 2 {
		t.Fatalf("unexpected any rows: %#v", any.Rows)
	}
	all, err := DropNA(frame, []string{"age", "group"}, "all")
	if err != nil {
		t.Fatal(err)
	}
	if len(all.Rows) != 4 {
		t.Fatalf("unexpected all rows: %#v", all.Rows)
	}
	if _, err := DropNA(frame, nil, "some"); err == nil {
		t.Fatal("expected invalid how error")
	}
}

func TestSortValues(t *testing.T) {
	frame := basicFixture(t)
	sorted, err := SortValues(frame, []string{"age", "name"}, []bool{true, false}, true)
	if err != nil {
		t.Fatal(err)
	}
	if got := []any{sorted.Rows[0][0], sorted.Rows[1][0], sorted.Rows[2][0], sorted.Rows[3][0]}; !reflect.DeepEqual(got, []any{"Cy", "Ada", "Ada", "Bob"}) {
		t.Fatalf("unexpected sort order: %#v", got)
	}
	descending, err := SortValues(frame, []string{"age"}, []bool{false}, false)
	if err != nil {
		t.Fatal(err)
	}
	if descending.Rows[0][0] != "Bob" || descending.Rows[3][0] != "Cy" {
		t.Fatalf("null placement changed with descending order: %#v", descending.Rows)
	}
	if _, err := SortValues(frame, []string{"age", "name"}, []bool{true, false, true}, true); err == nil {
		t.Fatal("expected ascending length error")
	}
}

func TestILocAndToNumpyCopy(t *testing.T) {
	frame := basicFixture(t)
	sliced := ILoc(frame, 1, 4, 1, 3)
	if !reflect.DeepEqual(sliced.Columns, []string{"age", "score"}) || len(sliced.Rows) != 3 {
		t.Fatalf("unexpected iloc: %#v", sliced)
	}
	negative := ILoc(frame, -3, -1, -2, 5)
	if !reflect.DeepEqual(negative.Columns, []string{"active", "group"}) || len(negative.Rows) != 2 {
		t.Fatalf("unexpected negative iloc: %#v", negative)
	}
	empty := ILoc(frame, 3, 1, 4, 2)
	if len(empty.Rows) != 0 || len(empty.Columns) != 0 {
		t.Fatalf("expected empty slice: %#v", empty)
	}
	matrix := ToNumpy(frame)
	matrix[0][0] = "changed"
	if frame.Rows[0][0] != "Ada" {
		t.Fatal("to-numpy result aliases source rows")
	}
}
