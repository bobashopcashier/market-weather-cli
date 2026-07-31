package provider

import "fmt"

type Error struct {
	Code     string         `json:"code"`
	Message  string         `json:"message"`
	Hint     string         `json:"hint,omitempty"`
	ExitCode int            `json:"exitCode"`
	Details  map[string]any `json:"details,omitempty"`
}

func (e *Error) Error() string { return e.Message }

func NewError(code, message string, exitCode int) *Error {
	return &Error{Code: code, Message: message, ExitCode: exitCode}
}

func HTTPError(status int, safeURL, body string) *Error {
	code := "http_error"
	hint := ""
	switch status {
	case 401:
		code, hint = "authentication_failed", "Check that the configured API key is valid."
	case 403:
		code, hint = "plan_required", "The key may not include this API or location."
	case 404:
		code = "not_found"
	case 429:
		code, hint = "rate_limited", "Wait before retrying or check the provider plan limits."
	default:
		if status >= 500 {
			code, hint = "provider_unavailable", "The upstream provider returned a temporary server error."
		}
	}
	return &Error{
		Code:     code,
		Message:  fmt.Sprintf("provider returned HTTP %d", status),
		Hint:     hint,
		ExitCode: 1,
		Details:  map[string]any{"status": status, "url": safeURL, "body": body},
	}
}
