package cli

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/bobashopcashier/market-weather-cli/internal/provider"
	"github.com/bobashopcashier/market-weather-cli/internal/render"
)

func runWethr(ctx context.Context, argv []string) error {
	command := argv[0]
	parsed, err := parseArgs(argv[1:], map[string]optionSpec{
		"mode":   {kind: stringOption, defaultVal: "latest", choices: []string{"latest", "history"}},
		"logic":  {kind: stringOption, defaultVal: "nws", choices: []string{"nws", "wu"}},
		"model":  {kind: stringOption},
		"models": {kind: stringOption},
		"run":    {kind: stringOption, defaultVal: "latest"},
		"daily":  {kind: boolOption},
		"date":   {kind: stringOption},
		"window": {kind: stringOption, defaultVal: "30d"},
		"radius": {kind: intOption, defaultVal: "50", min: 1, max: 500},
	})
	if err != nil {
		return err
	}
	stationInput, err := required(parsed.positionals, 0, "ICAO station")
	if err != nil {
		return err
	}
	station, err := provider.WethrStation(stationInput)
	if err != nil {
		return err
	}
	endpoint := ""
	params := url.Values{}
	switch command {
	case "obs":
		endpoint = "observations"
		params.Set("station_code", station)
		params.Set("mode", parsed.value("mode"))
	case "extreme":
		endpoint = "observations"
		params.Set("station_code", station)
		params.Set("mode", "wethr_high")
		params.Set("logic", parsed.value("logic"))
	case "forecast":
		endpoint = "forecasts"
		params.Set("location_name", station)
		if parsed.flag("daily") {
			params.Set("mode", "daily")
		} else {
			params.Set("run", parsed.value("run"))
		}
		if parsed.value("model") != "" {
			params.Set("model", parsed.value("model"))
		}
	case "precipitation":
		endpoint = "precipitation"
		params.Set("station_code", station)
	case "nws":
		endpoint = "nws_forecasts"
		params.Set("station_code", station)
		if parsed.value("date") != "" {
			params.Set("date", parsed.value("date"))
		} else {
			params.Set("mode", "latest")
		}
	case "pacing":
		endpoint = "pacing"
		params.Set("station_code", station)
		setIf(params, "date", parsed.value("date"))
		setIf(params, "models", parsed.value("models"))
	case "accuracy":
		endpoint = "model_accuracy"
		params.Set("station_code", station)
		params.Set("window", parsed.value("window"))
		setIf(params, "model", parsed.value("model"))
	case "nearby":
		endpoint = "nearby"
		params.Set("station_code", station)
		params.Set("radius_km", parsed.value("radius"))
	default:
		return provider.NewError("invalid_arguments", fmt.Sprintf("unknown wethr command: %s", command), 2)
	}
	result, err := provider.NewClient().CallWethr(ctx, endpoint, params)
	if err != nil {
		return err
	}
	if parsed.flag("json") {
		return render.JSON(os.Stdout, result)
	}
	fmt.Fprintf(os.Stdout, "Wethr.net %s for %s\n", endpoint, station)
	return render.JSON(os.Stdout, result.Data)
}

func setIf(values url.Values, key, value string) {
	if strings.TrimSpace(value) != "" {
		values.Set(key, value)
	}
}

func runMeteoblue(ctx context.Context, argv []string) error {
	parsed, err := parseArgs(argv, map[string]optionSpec{
		"package": {kind: stringOption, defaultVal: "basic-1h_basic-day"},
	})
	if err != nil {
		return err
	}
	if len(parsed.positionals) == 0 {
		return provider.NewError("invalid_arguments", "missing location", 2)
	}
	result, err := provider.NewClient().GetMeteoblue(ctx, strings.Join(parsed.positionals, " "), parsed.value("package"))
	if err != nil {
		return err
	}
	if parsed.flag("json") {
		return render.JSON(os.Stdout, result)
	}
	fmt.Fprintf(os.Stdout, "meteoblue %s for %s (%g, %g)\n", result.PackageName, result.Location.Name, result.Location.Latitude, result.Location.Longitude)
	return render.JSON(os.Stdout, result.Data)
}

func runWunderground(ctx context.Context, argv []string) error {
	parsed, err := parseArgs(argv, map[string]optionSpec{
		"units": {kind: stringOption, defaultVal: "e", choices: []string{"e", "m", "h"}},
	})
	if err != nil {
		return err
	}
	station, err := required(parsed.positionals, 0, "PWS station ID")
	if err != nil {
		return err
	}
	result, err := provider.NewClient().GetPWSCurrent(ctx, station, parsed.value("units"))
	if err != nil {
		return err
	}
	if parsed.flag("json") {
		return render.JSON(os.Stdout, result)
	}
	observations := render.Slice(result.Data["observations"])
	if len(observations) == 0 {
		return provider.NewError("not_found", "The Weather Company returned no PWS observation", 3)
	}
	observation := render.Map(observations[0])
	unitBlock := render.Map(observation[unitBlockName(parsed.value("units"))])
	fmt.Fprintf(os.Stdout, "%s  %s\n", result.StationID, render.Text(observation["obsTimeLocal"]))
	fmt.Fprintf(os.Stdout, "  temperature %s  humidity %s%%  wind %s at %s\n",
		render.Number(unitBlock["temp"], 1), render.Number(observation["humidity"], 0),
		render.Text(observation["winddir"]), render.Number(unitBlock["windSpeed"], 1))
	fmt.Fprintf(os.Stdout, "  solar radiation %s W/m²  precipitation rate %s\n",
		render.Number(observation["solarRadiation"], 1), render.Number(unitBlock["precipRate"], 2))
	return nil
}

func unitBlockName(units string) string {
	switch units {
	case "m":
		return "metric"
	case "h":
		return "uk_hybrid"
	default:
		return "imperial"
	}
}
