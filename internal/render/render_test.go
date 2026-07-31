package render

import (
	"strings"
	"testing"
)

func TestSafeTextEscapesTerminalControls(t *testing.T) {
	input := "safe\x1b[31m\nnext\x7f"
	output := SafeText(input)
	if strings.ContainsRune(output, '\x1b') || strings.ContainsRune(output, '\n') || strings.ContainsRune(output, '\x7f') {
		t.Fatalf("unsafe control survived: %q", output)
	}
	if output != `safe\u001b[31m\nnext\u007f` {
		t.Fatalf("unexpected escaped output: %q", output)
	}
	table := Table([]string{"name"}, [][]string{{input}})
	if strings.ContainsRune(table, '\x1b') {
		t.Fatalf("table emitted terminal escape: %q", table)
	}
}

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
