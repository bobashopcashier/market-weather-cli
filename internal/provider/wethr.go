package provider

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"
)

var wethrEndpoints = map[string]bool{
	"observations": true, "forecasts": true, "precipitation": true, "nws_forecasts": true,
	"pacing": true, "model_accuracy": true, "nearby": true,
}

type WethrResult struct {
	Source    string `json:"source"`
	FetchedAt string `json:"fetchedAt"`
	Endpoint  string `json:"endpoint"`
	Data      any    `json:"data"`
}

func WethrStation(input string) (string, error) {
	return NormalizeStation(input)
}

func (c *Client) CallWethr(ctx context.Context, endpoint string, params url.Values) (WethrResult, error) {
	if !wethrEndpoints[endpoint] {
		return WethrResult{}, NewError("invalid_arguments", fmt.Sprintf("unsupported Wethr endpoint: %s", endpoint), 2)
	}
	for name, values := range params {
		for _, value := range values {
			value, err := validateFreeText("Wethr parameter "+name, value, 512)
			if err != nil {
				return WethrResult{}, err
			}
			if strings.ContainsAny(value, "?#%") {
				return WethrResult{}, NewError("invalid_arguments", "Wethr parameters cannot contain query fragments or pre-encoded values", 2)
			}
		}
	}
	key, err := requiredEnv("WETHR_API_KEY", "Wethr.net")
	if err != nil {
		return WethrResult{}, err
	}
	requestURL := fmt.Sprintf("https://wethr.net/api/v2/%s.php?%s", endpoint, params.Encode())
	var data any
	if _, err := c.GetJSON(ctx, requestURL, map[string]string{"Authorization": "Bearer " + strings.TrimSpace(key)}, &data, false); err != nil {
		return WethrResult{}, err
	}
	return WethrResult{
		Source: "wethr.net-v2", FetchedAt: time.Now().UTC().Format(time.RFC3339), Endpoint: endpoint, Data: data,
	}, nil
}
