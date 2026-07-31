package provider

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Location struct {
	ID          int64   `json:"id,omitempty"`
	Name        string  `json:"name"`
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
	Elevation   float64 `json:"elevation,omitempty"`
	Timezone    string  `json:"timezone,omitempty"`
	Country     string  `json:"country,omitempty"`
	CountryCode string  `json:"countryCode,omitempty"`
	Admin1      string  `json:"admin1,omitempty"`
}

type ForecastResult struct {
	Source      string            `json:"source"`
	Attribution string            `json:"attribution"`
	FetchedAt   string            `json:"fetchedAt"`
	Location    Location          `json:"location"`
	Units       map[string]string `json:"units"`
	Forecast    map[string]any    `json:"forecast"`
}

type geocodeResponse struct {
	Results []struct {
		ID          int64   `json:"id"`
		Name        string  `json:"name"`
		Latitude    float64 `json:"latitude"`
		Longitude   float64 `json:"longitude"`
		Elevation   float64 `json:"elevation"`
		Timezone    string  `json:"timezone"`
		Country     string  `json:"country"`
		CountryCode string  `json:"country_code"`
		Admin1      string  `json:"admin1"`
	} `json:"results"`
}

func parseCoordinates(input string) (Location, bool, error) {
	parts := strings.Split(input, ",")
	if len(parts) != 2 {
		return Location{}, false, nil
	}
	lat, latErr := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	lon, lonErr := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if latErr != nil || lonErr != nil {
		return Location{}, false, nil
	}
	if lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		return Location{}, true, NewError("invalid_arguments", "coordinates are outside the valid latitude/longitude range", 2)
	}
	return Location{Name: fmt.Sprintf("%g,%g", lat, lon), Latitude: lat, Longitude: lon, Timezone: "auto"}, true, nil
}

func (c *Client) ResolveLocation(ctx context.Context, input string) (Location, error) {
	input, err := validateFreeText("location", input, 256)
	if err != nil {
		return Location{}, err
	}
	if location, matched, err := parseCoordinates(input); matched || err != nil {
		return location, err
	}
	query := url.Values{"name": {input}, "count": {"1"}, "language": {"en"}, "format": {"json"}}
	endpoint := "https://geocoding-api.open-meteo.com/v1/search?" + query.Encode()
	var response geocodeResponse
	if _, err := c.GetJSON(ctx, endpoint, nil, &response, false); err != nil {
		return Location{}, err
	}
	if len(response.Results) == 0 {
		return Location{}, &Error{Code: "not_found", Message: fmt.Sprintf("no location matched %q", input), ExitCode: 3}
	}
	result := response.Results[0]
	return Location{
		ID: result.ID, Name: result.Name, Latitude: result.Latitude, Longitude: result.Longitude,
		Elevation: result.Elevation, Timezone: result.Timezone, Country: result.Country,
		CountryCode: result.CountryCode, Admin1: result.Admin1,
	}, nil
}

func (c *Client) Forecast(ctx context.Context, location Location, days int, unit string, hourly bool) (ForecastResult, error) {
	query := url.Values{
		"latitude":           {strconv.FormatFloat(location.Latitude, 'f', -1, 64)},
		"longitude":          {strconv.FormatFloat(location.Longitude, 'f', -1, 64)},
		"timezone":           {"auto"},
		"forecast_days":      {strconv.Itoa(days)},
		"current":            {"temperature_2m,relative_humidity_2m,apparent_temperature,precipitation,weather_code,cloud_cover,wind_speed_10m,wind_direction_10m"},
		"daily":              {"weather_code,temperature_2m_max,temperature_2m_min,precipitation_sum,precipitation_probability_max,wind_speed_10m_max,sunrise,sunset"},
		"temperature_unit":   {"fahrenheit"},
		"wind_speed_unit":    {"mph"},
		"precipitation_unit": {"inch"},
	}
	units := map[string]string{"temperature": "°F", "wind": "mph", "precipitation": "in"}
	if unit == "c" {
		query.Set("temperature_unit", "celsius")
		query.Set("wind_speed_unit", "kmh")
		query.Set("precipitation_unit", "mm")
		units = map[string]string{"temperature": "°C", "wind": "km/h", "precipitation": "mm"}
	}
	if hourly {
		query.Set("hourly", "temperature_2m,precipitation_probability,weather_code,wind_speed_10m")
	}
	endpoint := "https://api.open-meteo.com/v1/forecast?" + query.Encode()
	forecast := map[string]any{}
	if _, err := c.GetJSON(ctx, endpoint, nil, &forecast, false); err != nil {
		return ForecastResult{}, err
	}
	return ForecastResult{
		Source: "open-meteo", Attribution: "Weather data by Open-Meteo.com (CC BY 4.0)",
		FetchedAt: time.Now().UTC().Format(time.RFC3339), Location: location, Units: units, Forecast: forecast,
	}, nil
}

func (c *Client) GetForecast(ctx context.Context, input string, days int, unit string, hourly bool) (ForecastResult, error) {
	location, err := c.ResolveLocation(ctx, input)
	if err != nil {
		return ForecastResult{}, err
	}
	return c.Forecast(ctx, location, days, unit, hourly)
}
