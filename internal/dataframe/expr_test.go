package dataframe

import (
	"strings"
	"testing"
)

func TestEvalExpressionLiteralsAndIdentifiers(t *testing.T) {
	values := map[string]any{
		"Age":              int64(27),
		"passenger.name":   "Ada",
		"siblings-spouses": int64(2),
		"Cabin Number":     "C42",
	}
	tests := []struct {
		name       string
		expression string
		want       any
	}{
		{name: "integer", expression: "42", want: int64(42)},
		{name: "float", expression: "1.25e2", want: float64(125)},
		{name: "single quoted string", expression: `'hello\nworld'`, want: "hello\nworld"},
		{name: "double quoted string", expression: `"hello"`, want: "hello"},
		{name: "bool", expression: "TRUE", want: true},
		{name: "null", expression: "null", want: nil},
		{name: "identifier", expression: "Age", want: int64(27)},
		{name: "dotted identifier", expression: "passenger.name", want: "Ada"},
		{name: "hyphenated identifier", expression: "siblings-spouses", want: int64(2)},
		{name: "quoted identifier", expression: "`Cabin Number`", want: "C42"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := EvalExpression(test.expression, values)
			if err != nil {
				t.Fatalf("EvalExpression(%q): %v", test.expression, err)
			}
			if Compare(got, test.want) != 0 || IsNull(got) != IsNull(test.want) {
				t.Fatalf("EvalExpression(%q) = %#v, want %#v", test.expression, got, test.want)
			}
		})
	}
}

func TestEvalExpressionArithmeticPrecedence(t *testing.T) {
	tests := []struct {
		expression string
		want       float64
	}{
		{expression: "1 + 2 * 3", want: 7},
		{expression: "(1 + 2) * 3", want: 9},
		{expression: "-4 + +10", want: 6},
		{expression: "7 / 2", want: 3.5},
		{expression: "7 % 4", want: 3},
		{expression: "10-3", want: 7},
	}
	for _, test := range tests {
		got, err := EvalExpression(test.expression, nil)
		if err != nil {
			t.Fatalf("EvalExpression(%q): %v", test.expression, err)
		}
		number, ok := Float(got)
		if !ok || number != test.want {
			t.Fatalf("EvalExpression(%q) = %#v, want %v", test.expression, got, test.want)
		}
	}

	got, err := EvalExpression(`"hello" + " world"`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != "hello world" {
		t.Fatalf("string concatenation = %#v", got)
	}

	got, err = EvalExpression("a*b-c", map[string]any{"a": 4, "b": 5, "c": 3})
	if err != nil {
		t.Fatal(err)
	}
	if number, _ := Float(got); number != 17 {
		t.Fatalf("identifier subtraction = %#v, want 17", got)
	}
}

func TestEvalPredicateComparisonsAndBooleanOperators(t *testing.T) {
	values := map[string]any{"Age": int64(27), "Embarked": "Cherbourg", "Active": true}
	tests := []struct {
		expression string
		want       bool
	}{
		{expression: "Age >= 20 and Age <= 30", want: true},
		{expression: `Embarked == "Queenstown" or Active`, want: true},
		{expression: `!(Age < 20) && not false`, want: true},
		{expression: `not Age > 30`, want: true},
		{expression: `Age != 27`, want: false},
		{expression: `Age > 27 || Age == 27`, want: true},
		{expression: `null == null`, want: true},
		{expression: `null != 1`, want: true},
	}
	for _, test := range tests {
		got, err := EvalPredicate(test.expression, values)
		if err != nil {
			t.Fatalf("EvalPredicate(%q): %v", test.expression, err)
		}
		if got != test.want {
			t.Fatalf("EvalPredicate(%q) = %v, want %v", test.expression, got, test.want)
		}
	}
}

func TestEvalExpressionDoesNotCoerceStringsInComparisons(t *testing.T) {
	values := map[string]any{"numeric": int64(1), "text": "001", "boolean": true}
	for _, expression := range []string{`numeric == "1"`, `text == 1`, `boolean == "true"`} {
		got, err := EvalPredicate(expression, values)
		if err != nil {
			t.Fatalf("EvalPredicate(%q): %v", expression, err)
		}
		if got {
			t.Fatalf("EvalPredicate(%q) coerced incompatible types", expression)
		}
	}
	if _, err := EvalPredicate(`numeric < "2"`, values); err == nil || !strings.Contains(err.Error(), "cannot order") {
		t.Fatalf("mixed ordering error = %v", err)
	}
}

func TestEvalPredicateOrderingWithNullIsFalse(t *testing.T) {
	for _, expression := range []string{"null < 1", "null <= 1", "null > 1", "null >= 1", "1 < null", "1 > null"} {
		got, err := EvalPredicate(expression, nil)
		if err != nil {
			t.Fatalf("EvalPredicate(%q): %v", expression, err)
		}
		if got {
			t.Fatalf("EvalPredicate(%q) = true, want false", expression)
		}
	}
}

func TestEvalPredicateMembership(t *testing.T) {
	values := map[string]any{"Embarked": "Cherbourg", "Code": int64(2), "Missing": nil}
	tests := []struct {
		expression string
		want       bool
	}{
		{expression: `Embarked in ("Cherbourg", "Queenstown")`, want: true},
		{expression: `Code in (1, 2, 3)`, want: true},
		{expression: `Code in ()`, want: false},
		{expression: `Missing in (1, null)`, want: true},
		{expression: `Embarked in ("Southampton",)`, want: false},
	}
	for _, test := range tests {
		got, err := EvalPredicate(test.expression, values)
		if err != nil {
			t.Fatalf("EvalPredicate(%q): %v", test.expression, err)
		}
		if got != test.want {
			t.Fatalf("EvalPredicate(%q) = %v, want %v", test.expression, got, test.want)
		}
	}
}

func TestEvalExpressionShortCircuits(t *testing.T) {
	for _, expression := range []string{
		"false and (1 / 0 > 0)",
		"true or MissingColumn == 1",
	} {
		if _, err := EvalPredicate(expression, nil); err != nil {
			t.Fatalf("EvalPredicate(%q): %v", expression, err)
		}
	}
}

func TestEvalExpressionErrors(t *testing.T) {
	tests := []struct {
		name       string
		expression string
		want       string
	}{
		{name: "missing identifier", expression: "Missing + 1", want: "unknown identifier"},
		{name: "division by zero", expression: "10 / 0", want: "division by zero"},
		{name: "modulo by zero", expression: "10 % 0", want: "modulo by zero"},
		{name: "unterminated string", expression: `'nope`, want: "unterminated string"},
		{name: "empty", expression: "  ", want: "expression is empty"},
		{name: "single equals", expression: "Age = 1", want: "use =="},
		{name: "missing paren", expression: "(1 + 2", want: "expected )"},
		{name: "in without list", expression: "Age in 1", want: "parenthesized list"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := EvalExpression(test.expression, map[string]any{"Age": int64(1)})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("EvalExpression(%q) error = %v, want containing %q", test.expression, err, test.want)
			}
		})
	}
}

func TestEvalPredicateRequiresBooleanResult(t *testing.T) {
	_, err := EvalPredicate("1 + 2", nil)
	if err == nil || !strings.Contains(err.Error(), "want bool") {
		t.Fatalf("EvalPredicate error = %v", err)
	}
}
