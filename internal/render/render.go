package render

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"
)

func JSON(writer io.Writer, value any) error {
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	return encoder.Encode(value)
}

func CompactJSON(writer io.Writer, value any) error {
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(value)
}

// SafeText escapes terminal control characters while leaving ordinary Unicode
// untouched. JSON output uses encoding/json's native escaping instead.
func SafeText(value any) string {
	input := fmt.Sprint(value)
	var output strings.Builder
	for _, current := range input {
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

func Table(headers []string, rows [][]string) string {
	safeHeaders := make([]string, len(headers))
	for index, header := range headers {
		safeHeaders[index] = SafeText(header)
	}
	safeRows := make([][]string, len(rows))
	for rowIndex, row := range rows {
		safeRows[rowIndex] = make([]string, len(row))
		for columnIndex, cell := range row {
			safeRows[rowIndex][columnIndex] = SafeText(cell)
		}
	}
	headers = safeHeaders
	rows = safeRows
	widths := make([]int, len(headers))
	for index, header := range headers {
		widths[index] = utf8.RuneCountInString(header)
	}
	for _, row := range rows {
		for index, cell := range row {
			cellWidth := utf8.RuneCountInString(cell)
			if index < len(widths) && cellWidth > widths[index] {
				widths[index] = cellWidth
			}
		}
	}
	formatRow := func(row []string) string {
		cells := make([]string, len(headers))
		for index := range headers {
			value := ""
			if index < len(row) {
				value = row[index]
			}
			cells[index] = value + strings.Repeat(" ", widths[index]-utf8.RuneCountInString(value))
		}
		return strings.TrimRight(strings.Join(cells, "  "), " ")
	}
	lines := []string{formatRow(headers)}
	separator := make([]string, len(widths))
	for index, width := range widths {
		separator[index] = strings.Repeat("-", width)
	}
	lines = append(lines, strings.Join(separator, "  "))
	for _, row := range rows {
		lines = append(lines, formatRow(row))
	}
	return strings.Join(lines, "\n")
}

func Number(value any, digits int) string {
	number, ok := Float(value)
	if !ok {
		return "n/a"
	}
	formatted := strconv.FormatFloat(number, 'f', digits, 64)
	if digits > 0 {
		formatted = strings.TrimRight(strings.TrimRight(formatted, "0"), ".")
	}
	return formatted
}

func Float(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, !math.IsNaN(typed)
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case json.Number:
		parsed, err := typed.Float64()
		return parsed, err == nil
	case string:
		parsed, err := strconv.ParseFloat(typed, 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func Text(value any) string {
	if value == nil {
		return "n/a"
	}
	return SafeText(value)
}

func Slice(value any) []any {
	if values, ok := value.([]any); ok {
		return values
	}
	return nil
}

func Map(value any) map[string]any {
	if mapped, ok := value.(map[string]any); ok {
		return mapped
	}
	return map[string]any{}
}

func WeatherCode(value any) string {
	codeFloat, ok := Float(value)
	if !ok {
		return "Unknown"
	}
	code := int(codeFloat)
	codes := map[int]string{
		0: "Clear", 1: "Mainly clear", 2: "Partly cloudy", 3: "Overcast", 45: "Fog", 48: "Rime fog",
		51: "Light drizzle", 53: "Drizzle", 55: "Heavy drizzle", 61: "Light rain", 63: "Rain", 65: "Heavy rain",
		71: "Light snow", 73: "Snow", 75: "Heavy snow", 80: "Light showers", 81: "Showers", 82: "Heavy showers", 95: "Thunderstorm",
	}
	if label, found := codes[code]; found {
		return label
	}
	return fmt.Sprintf("Weather code %d", code)
}
