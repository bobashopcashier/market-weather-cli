package provider

import (
	"context"
	"net/url"
	"strings"
	"time"
)

type WundergroundResult struct {
	Source    string         `json:"source"`
	FetchedAt string         `json:"fetchedAt"`
	StationID string         `json:"stationId"`
	Data      map[string]any `json:"data"`
}

func (c *Client) GetPWSCurrent(ctx context.Context, stationID, units string) (WundergroundResult, error) {
	key, err := requiredEnv("WEATHER_COMPANY_API_KEY", "Weather Underground PWS data")
	if err != nil {
		return WundergroundResult{}, err
	}
	stationID = strings.ToUpper(strings.TrimSpace(stationID))
	query := url.Values{
		"stationId": {stationID}, "format": {"json"}, "units": {units},
		"numericPrecision": {"decimal"}, "apiKey": {key},
	}
	endpoint := "https://api.weather.com/v2/pws/observations/current?" + query.Encode()
	data := map[string]any{}
	if _, err := c.GetJSON(ctx, endpoint, nil, &data, false); err != nil {
		return WundergroundResult{}, err
	}
	return WundergroundResult{
		Source: "the-weather-company-pws", FetchedAt: time.Now().UTC().Format(time.RFC3339), StationID: stationID, Data: data,
	}, nil
}
