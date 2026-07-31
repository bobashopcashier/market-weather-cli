package provider

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var stationPattern = regexp.MustCompile(`^[A-Z0-9]{3,4}$`)

type METARObservation struct {
	ICAOID      string           `json:"icaoId"`
	ReceiptTime string           `json:"receiptTime,omitempty"`
	ObsTime     int64            `json:"obsTime,omitempty"`
	ReportTime  string           `json:"reportTime,omitempty"`
	Temperature *float64         `json:"temp,omitempty"`
	Dewpoint    *float64         `json:"dewp,omitempty"`
	WindDir     any              `json:"wdir,omitempty"`
	WindSpeed   *float64         `json:"wspd,omitempty"`
	Visibility  any              `json:"visib,omitempty"`
	Altimeter   *float64         `json:"altim,omitempty"`
	Raw         string           `json:"rawOb,omitempty"`
	Latitude    float64          `json:"lat,omitempty"`
	Longitude   float64          `json:"lon,omitempty"`
	Elevation   float64          `json:"elev,omitempty"`
	Name        string           `json:"name,omitempty"`
	Cover       string           `json:"cover,omitempty"`
	Clouds      []map[string]any `json:"clouds,omitempty"`
	FlightCat   string           `json:"fltCat,omitempty"`
}

type METARResult struct {
	Source       string             `json:"source"`
	FetchedAt    string             `json:"fetchedAt"`
	Stations     []string           `json:"stations"`
	Observations []METARObservation `json:"observations"`
}

func NormalizeStations(values []string) ([]string, error) {
	seen := map[string]bool{}
	stations := []string{}
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			station := strings.ToUpper(strings.TrimSpace(part))
			if station == "" {
				continue
			}
			if !stationPattern.MatchString(station) {
				return nil, NewError("invalid_arguments", fmt.Sprintf("invalid station identifier: %s", station), 2)
			}
			if !seen[station] {
				seen[station] = true
				stations = append(stations, station)
			}
		}
	}
	if len(stations) == 0 {
		return nil, NewError("invalid_arguments", "at least one ICAO station is required", 2)
	}
	return stations, nil
}

func (c *Client) GetMETAR(ctx context.Context, stationInput []string, hours int) (METARResult, error) {
	stations, err := NormalizeStations(stationInput)
	if err != nil {
		return METARResult{}, err
	}
	query := url.Values{"ids": {strings.Join(stations, ",")}, "format": {"json"}, "hours": {strconv.Itoa(hours)}}
	endpoint := "https://aviationweather.gov/api/data/metar?" + query.Encode()
	observations := []METARObservation{}
	available, err := c.GetJSON(ctx, endpoint, nil, &observations, true)
	if err != nil {
		return METARResult{}, err
	}
	if !available {
		observations = []METARObservation{}
	}
	sort.SliceStable(observations, func(i, j int) bool { return observations[i].ReportTime > observations[j].ReportTime })
	return METARResult{
		Source: "noaa-aviation-weather-center", FetchedAt: time.Now().UTC().Format(time.RFC3339),
		Stations: stations, Observations: observations,
	}, nil
}
