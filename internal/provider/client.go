package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const userAgent = "market-weather-cli/0.1.0"

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
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return false, &Error{
			Code:     "invalid_provider_response",
			Message:  "provider returned invalid JSON",
			ExitCode: 1,
			Details:  map[string]any{"url": RedactURL(endpoint), "error": fmt.Sprint(err)},
		}
	}
	return true, nil
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
