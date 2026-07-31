package render

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestSafeTextEscapesTerminalAndDirectionalityControls(t *testing.T) {
	input := "safe\x1b[31m\nnext\x7f\u202eTXT\u200b"
	output := SafeText(input)
	for _, unsafe := range []rune{'\x1b', '\n', '\x7f', '\u202e', '\u200b'} {
		if strings.ContainsRune(output, unsafe) {
			t.Fatalf("unsafe control survived: %q", output)
		}
	}
	if output != `safe\u001b[31m\nnext\u007f\u202eTXT\u200b` {
		t.Fatalf("unexpected escaped output: %q", output)
	}
	table := Table([]string{"name"}, [][]string{{input}})
	if strings.ContainsRune(table, '\x1b') || strings.ContainsRune(table, '\u202e') {
		t.Fatalf("table emitted unsafe controls: %q", table)
	}
}

func TestSafeJSONSanitizesProviderKeysAndValues(t *testing.T) {
	var output bytes.Buffer
	if err := SafeJSON(&output, map[string]any{
		"key\u202e":  []any{"value\u200b", map[string]any{"ansi": "\x1b[31m"}},
		"key\\u202e": "literal",
	}); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	for _, unsafe := range []rune{'\u202e', '\u200b', '\x1b'} {
		if strings.ContainsRune(text, unsafe) {
			t.Fatalf("unsafe provider character survived: %q", text)
		}
	}
	for _, escaped := range []string{`\u202e`, `\u200b`, `\u001b`, `\\u202e`} {
		if !strings.Contains(text, escaped) {
			t.Fatalf("missing safe escape %q in %q", escaped, text)
		}
	}
	var decoded map[string]any
	if err := json.Unmarshal(output.Bytes(), &decoded); err != nil {
		t.Fatalf("safe output is not valid JSON: %v", err)
	}
	if len(decoded) != 2 {
		t.Fatalf("provider keys collided: %#v", decoded)
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
