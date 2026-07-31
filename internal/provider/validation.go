package provider

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

var (
	decimalTokenPattern = regexp.MustCompile(`^[0-9]{1,128}$`)
	pwsStationPattern   = regexp.MustCompile(`^[A-Z0-9_-]{1,64}$`)
	resourcePattern     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._,-]{0,255}$`)
	windowPattern       = regexp.MustCompile(`^[1-9][0-9]{0,3}[dhm]$`)
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

func ValidateFreeText(label, value string, maximum int) (string, error) {
	return validateFreeText(label, value, maximum)
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

func validateResourceValue(label, value string) (string, error) {
	value = strings.TrimSpace(value)
	if !resourcePattern.MatchString(value) || strings.ContainsAny(value, "?#%") {
		return "", NewError("invalid_arguments", fmt.Sprintf("%s contains an unsafe or pre-encoded resource value", label), 2)
	}
	return value, nil
}

func validateWethrParameter(name, value string) (string, error) {
	value, err := validateFreeText("Wethr parameter "+name, value, 512)
	if err != nil {
		return "", err
	}
	if strings.ContainsAny(value, "?#%") {
		return "", NewError("invalid_arguments", "Wethr parameters cannot contain query fragments or pre-encoded values", 2)
	}
	switch name {
	case "date":
		if _, err := time.Parse("2006-01-02", value); err != nil {
			return "", NewError("invalid_arguments", "Wethr date must use YYYY-MM-DD format", 2)
		}
	case "window":
		if !windowPattern.MatchString(value) {
			return "", NewError("invalid_arguments", "Wethr window must be a positive duration such as 30d", 2)
		}
	case "model", "models", "run":
		if !resourcePattern.MatchString(value) {
			return "", NewError("invalid_arguments", "Wethr model and run identifiers contain unsupported characters", 2)
		}
	}
	return value, nil
}

func containsControlCharacter(value string) bool {
	for _, current := range value {
		if current < 0x20 || current == 0x7f || current >= 0x80 && current <= 0x9f ||
			current >= 0x200b && current <= 0x200f || current >= 0x202a && current <= 0x202e ||
			current >= 0x2066 && current <= 0x2069 || current == 0xfeff {
			return true
		}
	}
	return false
}
