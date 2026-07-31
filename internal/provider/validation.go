package provider

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	decimalTokenPattern = regexp.MustCompile(`^[0-9]{1,128}$`)
	pwsStationPattern   = regexp.MustCompile(`^[A-Z0-9_-]{1,64}$`)
)

func validateFreeText(label, value string, maximum int) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", NewError("invalid_arguments", fmt.Sprintf("%s cannot be empty", label), 2)
	}
	if maximum > 0 && len(value) > maximum {
		return "", NewError("invalid_arguments", fmt.Sprintf("%s exceeds the %d-byte limit", label, maximum), 2)
	}
	if containsControlCharacter(value) {
		return "", NewError("invalid_arguments", fmt.Sprintf("%s contains a control character", label), 2)
	}
	return value, nil
}

func validateDecimalToken(value string) (string, error) {
	value = strings.TrimSpace(value)
	if !decimalTokenPattern.MatchString(value) {
		return "", NewError("invalid_arguments", "token ID must be a decimal identifier of at most 128 digits", 2)
	}
	return value, nil
}

func validatePWSStation(value string) (string, error) {
	value = strings.ToUpper(strings.TrimSpace(value))
	if !pwsStationPattern.MatchString(value) {
		return "", NewError("invalid_arguments", "PWS station ID must contain only letters, numbers, underscores, or hyphens", 2)
	}
	return value, nil
}

func containsControlCharacter(value string) bool {
	for _, current := range value {
		if current < 0x20 || current == 0x7f || current >= 0x80 && current <= 0x9f {
			return true
		}
	}
	return false
}
