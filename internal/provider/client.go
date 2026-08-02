package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const (
	userAgent                    = "market-weather-cli/0.3.0"
	MaximumProviderResponseBytes = 8 << 20
)

type Client struct {
	HTTP *http.Client
}

func NewClient() *Client {
	return &Client{HTTP: &http.Client{Timeout: 15 * time.Second}}
}

func RedactURL(input string) string {
	parsed, err := url.Parse(input)
	if err != nil {
		return "invalid-url"
	}
	query := parsed.Query()
	for key := range query {
		switch strings.ToLower(key) {
		case "apikey", "api_key", "key", "token":
			query.Set(key, "REDACTED")
		}
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func redactResponseText(text, endpoint string, headers map[string]string) string {
	parsed, err := url.Parse(endpoint)
	if err == nil {
		for key, values := range parsed.Query() {
			switch strings.ToLower(key) {
			case "apikey", "api_key", "key", "token":
				for _, value := range values {
					if value != "" {
						text = strings.ReplaceAll(text, value, "REDACTED")
					}
				}
			}
		}
	}
	for key, value := range headers {
		switch strings.ToLower(key) {
		case "authorization", "x-api-key":
			if value != "" {
				text = strings.ReplaceAll(text, value, "REDACTED")
				text = strings.ReplaceAll(text, strings.TrimPrefix(value, "Bearer "), "REDACTED")
			}
		}
	}
	return text
}

func (c *Client) GetJSON(ctx context.Context, endpoint string, headers map[string]string, target any, allowNoContent bool) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return false, NewError("invalid_request", "could not build provider request", 1)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		code := "network_error"
		message := "could not reach the provider"
		if errors.Is(err, context.DeadlineExceeded) {
			code, message = "timeout", "the provider request timed out"
		}
		return false, &Error{Code: code, Message: message, ExitCode: 1, Details: map[string]any{"url": RedactURL(endpoint)}}
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent && allowNoContent {
		return false, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 500))
		safeBody := redactResponseText(strings.TrimSpace(string(body)), endpoint, headers)
		return false, HTTPError(resp.StatusCode, RedactURL(endpoint), safeBody)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, MaximumProviderResponseBytes+1))
	if err != nil {
		return false, &Error{Code: "network_error", Message: "could not read the provider response", ExitCode: 1, Details: map[string]any{"url": RedactURL(endpoint)}}
	}
	if len(body) > MaximumProviderResponseBytes {
		return false, &Error{
			Code: "provider_response_too_large", Message: "provider response exceeded the 8388608-byte safety limit", ExitCode: 1,
			Details: map[string]any{"url": RedactURL(endpoint), "maximumBytes": MaximumProviderResponseBytes},
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var document any
	if err := decoder.Decode(&document); err != nil {
		return false, invalidProviderJSONError(endpoint, err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			err = errors.New("provider returned multiple JSON values")
		}
		return false, invalidProviderJSONError(endpoint, err)
	}
	validation := validateJSONShape(document, target)
	if !validation.valid() {
		return false, upstreamSchemaMismatchError(endpoint, validation)
	}
	if err := json.Unmarshal(body, target); err != nil {
		return false, invalidProviderJSONError(endpoint, err)
	}
	return true, nil
}

func upstreamSchemaMismatchError(endpoint string, validation schemaValidationResult) *Error {
	paths := append([]string(nil), validation.missingFields...)
	for _, mismatch := range validation.typeMismatches {
		paths = append(paths, mismatch.Path)
	}
	sort.Strings(paths)
	paths = compactSchemaPaths(paths)
	return &Error{
		Code:     "UPSTREAM_SCHEMA_MISMATCH",
		Message:  "provider response did not match the expected JSON schema",
		Hint:     "Affected JSON paths: " + strings.Join(paths, ", "),
		ExitCode: 6,
		Details: map[string]any{
			"url":            RedactURL(endpoint),
			"missingFields":  validation.missingFields,
			"typeMismatches": validation.typeMismatches,
		},
	}
}

func compactSchemaPaths(paths []string) []string {
	if len(paths) < 2 {
		return paths
	}
	result := paths[:1]
	for _, path := range paths[1:] {
		if path != result[len(result)-1] {
			result = append(result, path)
		}
	}
	return result
}

func invalidProviderJSONError(endpoint string, err error) *Error {
	return &Error{
		Code:     "invalid_provider_response",
		Message:  "provider returned invalid JSON",
		ExitCode: 1,
		Details:  map[string]any{"url": RedactURL(endpoint), "error": fmt.Sprint(err)},
	}
}

func requiredEnv(name, purpose string) (string, error) {
	value := strings.TrimSpace(getenv(name))
	if value == "" {
		return "", &Error{
			Code:     "not_configured",
			Message:  fmt.Sprintf("%s is required for %s", name, purpose),
			Hint:     fmt.Sprintf("Set %s in the environment. API keys are never accepted as command arguments.", name),
			ExitCode: 2,
		}
	}
	return value, nil
}

var getenv = func(name string) string {
	return lookupEnv(name)
}
