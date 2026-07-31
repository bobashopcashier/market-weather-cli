package provider

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"time"
)

var meteobluePackagePattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

type MeteoblueResult struct {
	Source      string         `json:"source"`
	FetchedAt   string         `json:"fetchedAt"`
	PackageName string         `json:"package"`
	Location    Location       `json:"location"`
	Data        map[string]any `json:"data"`
}

func (c *Client) GetMeteoblue(ctx context.Context, input, packageName string) (MeteoblueResult, error) {
	if len(packageName) > 128 || !meteobluePackagePattern.MatchString(packageName) {
		return MeteoblueResult{}, NewError("invalid_arguments", "invalid meteoblue package name", 2)
	}
	if _, err := validateFreeText("location", input, 256); err != nil {
		return MeteoblueResult{}, err
	}
	key, err := requiredEnv("METEOBLUE_API_KEY", "meteoblue")
	if err != nil {
		return MeteoblueResult{}, err
	}
	location, err := c.ResolveLocation(ctx, input)
	if err != nil {
		return MeteoblueResult{}, err
	}
	query := url.Values{
		"lat":    {strconv.FormatFloat(location.Latitude, 'f', -1, 64)},
		"lon":    {strconv.FormatFloat(location.Longitude, 'f', -1, 64)},
		"apikey": {key},
	}
	endpoint := fmt.Sprintf("https://my.meteoblue.com/packages/%s?%s", packageName, query.Encode())
	data := map[string]any{}
	if _, err := c.GetJSON(ctx, endpoint, nil, &data, false); err != nil {
		return MeteoblueResult{}, err
	}
	return MeteoblueResult{
		Source: "meteoblue", FetchedAt: time.Now().UTC().Format(time.RFC3339), PackageName: packageName, Location: location, Data: data,
	}, nil
}
