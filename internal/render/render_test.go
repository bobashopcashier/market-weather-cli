package render

import (
	"strings"
	"testing"
)

func TestTableHandlesUnicodeWidth(t *testing.T) {
	output := Table([]string{"LOW", "HIGH"}, [][]string{{"54°F", "73°F"}, {"5°C", "12°C"}})
	lines := strings.Split(output, "\n")
	if len(lines) != 4 {
		t.Fatalf("unexpected table: %q", output)
	}
	if !strings.Contains(output, "54°F  73°F") {
		t.Fatalf("columns are not aligned: %q", output)
	}
}
