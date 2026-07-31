package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/bobashopcashier/market-weather-cli/internal/provider"
	"github.com/bobashopcashier/market-weather-cli/internal/render"
)

func runMETAR(ctx context.Context, argv []string) error {
	parsed, err := parseArgs(argv, map[string]optionSpec{
		"hours": {kind: intOption, defaultVal: "2", min: 1, max: 360},
		"raw":   {kind: boolOption},
	})
	if err != nil {
		return err
	}
	if len(parsed.positionals) == 0 {
		return provider.NewError("invalid_arguments", "missing ICAO station", 2)
	}
	result, err := provider.NewClient().GetMETAR(ctx, parsed.positionals, parsed.integer("hours"))
	if err != nil {
		return err
	}
	if parsed.flag("json") {
		return render.JSON(os.Stdout, result)
	}
	if len(result.Observations) == 0 {
		return provider.NewError("not_found", "NOAA returned no observations for the requested station and time window", 3)
	}
	for index, observation := range result.Observations {
		if index > 0 && !parsed.flag("raw") {
			fmt.Fprintln(os.Stdout)
		}
		if parsed.flag("raw") {
			fmt.Fprintln(os.Stdout, observation.Raw)
			continue
		}
		fmt.Fprintf(os.Stdout, "%s  %s  %s\n", observation.ICAOID, observation.ReportTime, observation.FlightCat)
		fmt.Fprintf(os.Stdout, "  temperature %s°C  dewpoint %s°C  wind %s° at %s kt  visibility %s sm\n",
			pointerNumber(observation.Temperature), pointerNumber(observation.Dewpoint), render.Text(observation.WindDir),
			pointerNumber(observation.WindSpeed), render.Text(observation.Visibility))
		fmt.Fprintf(os.Stdout, "  %s\n", observation.Raw)
	}
	return nil
}

func pointerNumber(value *float64) string {
	if value == nil {
		return "n/a"
	}
	return render.Number(*value, 1)
}

func runOpenMeteo(ctx context.Context, argv []string) error {
	parsed, err := parseArgs(argv, map[string]optionSpec{
		"days":   {kind: intOption, defaultVal: "7", min: 1, max: 16},
		"unit":   {kind: stringOption, defaultVal: "f", choices: []string{"f", "c"}},
		"hourly": {kind: boolOption},
	})
	if err != nil {
		return err
	}
	if len(parsed.positionals) == 0 {
		return provider.NewError("invalid_arguments", "missing location", 2)
	}
	location := strings.Join(parsed.positionals, " ")
	result, err := provider.NewClient().GetForecast(ctx, location, parsed.integer("days"), parsed.value("unit"), parsed.flag("hourly"))
	if err != nil {
		return err
	}
	if parsed.flag("json") {
		return render.JSON(os.Stdout, result)
	}
	printForecast(result)
	return nil
}

func printForecast(result provider.ForecastResult) {
	current := render.Map(result.Forecast["current"])
	unit := result.Units["temperature"]
	locationLabel := result.Location.Name
	if result.Location.Admin1 != "" {
		locationLabel += ", " + result.Location.Admin1
	}
	fmt.Fprintf(os.Stdout, "%s  (%g, %g)\n", locationLabel, result.Location.Latitude, result.Location.Longitude)
	fmt.Fprintf(os.Stdout, "Now: %s%s, feels like %s%s, %s, wind %s %s\n\n",
		render.Number(current["temperature_2m"], 1), unit, render.Number(current["apparent_temperature"], 1), unit,
		render.WeatherCode(current["weather_code"]), render.Number(current["wind_speed_10m"], 1), result.Units["wind"])
	daily := render.Map(result.Forecast["daily"])
	rows := [][]string{}
	for index, value := range render.Slice(daily["time"]) {
		rows = append(rows, []string{
			render.Text(value), render.WeatherCode(at(daily["weather_code"], index)),
			render.Number(at(daily["temperature_2m_min"], index), 1) + unit,
			render.Number(at(daily["temperature_2m_max"], index), 1) + unit,
			render.Number(at(daily["precipitation_probability_max"], index), 0) + "%",
		})
	}
	fmt.Fprintln(os.Stdout, render.Table([]string{"DATE", "CONDITIONS", "LOW", "HIGH", "PRECIP"}, rows))
	fmt.Fprintln(os.Stdout, "\nWeather data by Open-Meteo.com (CC BY 4.0)")
}

func at(value any, index int) any {
	values := render.Slice(value)
	if index < 0 || index >= len(values) {
		return nil
	}
	return values[index]
}

func runBetmoar(ctx context.Context, argv []string) error {
	if argv[0] == "upstream" {
		args := argv[1:]
		if len(args) > 0 && args[0] == "--" {
			args = args[1:]
		}
		path, err := exec.LookPath("polymarket")
		if err != nil {
			appErr := provider.NewError("not_configured", "Polymarket's official CLI is not installed", 2)
			appErr.Hint = "Install it with: brew tap Polymarket/polymarket-cli https://github.com/Polymarket/polymarket-cli && brew install polymarket"
			return appErr
		}
		command := exec.CommandContext(ctx, path, args...)
		command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
		if err := command.Run(); err != nil {
			return provider.NewError("upstream_failed", fmt.Sprintf("official Polymarket CLI failed: %v", err), 1)
		}
		return nil
	}
	parsed, err := parseArgs(argv[1:], map[string]optionSpec{
		"limit":  {kind: intOption, defaultVal: "5", min: 1, max: 50},
		"closed": {kind: boolOption},
	})
	if err != nil {
		return err
	}
	switch argv[0] {
	case "search":
		if len(parsed.positionals) == 0 {
			return provider.NewError("invalid_arguments", "missing market search query", 2)
		}
		result, err := provider.NewClient().SearchMarkets(ctx, strings.Join(parsed.positionals, " "), parsed.integer("limit"), parsed.flag("closed"))
		if err != nil {
			return err
		}
		if parsed.flag("json") {
			return render.JSON(os.Stdout, result)
		}
		printMarketSearch(result)
		return nil
	case "book":
		tokenID, err := required(parsed.positionals, 0, "token ID")
		if err != nil {
			return err
		}
		result, err := provider.NewClient().GetOrderBook(ctx, tokenID)
		if err != nil {
			return err
		}
		return render.JSON(os.Stdout, result)
	default:
		return provider.NewError("invalid_arguments", fmt.Sprintf("unknown betmoar command: %s", argv[0]), 2)
	}
}

func printMarketSearch(result provider.MarketSearchResult) {
	if len(result.Events) == 0 {
		fmt.Fprintln(os.Stdout, "No active markets found.")
		return
	}
	for eventIndex, event := range result.Events {
		if eventIndex > 0 {
			fmt.Fprintln(os.Stdout)
		}
		fmt.Fprintf(os.Stdout, "%s\nhttps://polymarket.com/event/%s\n", event.Title, event.Slug)
		rows := [][]string{}
		for _, market := range event.Markets {
			price := "n/a"
			token := ""
			if len(market.Outcomes) > 0 {
				if market.Outcomes[0].Price != nil {
					price = strconv.FormatFloat(*market.Outcomes[0].Price*100, 'f', 1, 64) + "%"
				}
				token = market.Outcomes[0].TokenID
			}
			rows = append(rows, []string{market.Question, price, shortToken(token)})
		}
		fmt.Fprintln(os.Stdout, render.Table([]string{"MARKET", "YES", "YES TOKEN"}, rows))
	}
}

func shortToken(token string) string {
	if len(token) <= 20 {
		return token
	}
	return token[:10] + "…" + token[len(token)-8:]
}

type polyweatherResult struct {
	Source      string                      `json:"source"`
	Station     string                      `json:"station"`
	Location    string                      `json:"location"`
	Observation provider.METARObservation   `json:"observation"`
	Forecast    provider.ForecastResult     `json:"forecast"`
	Markets     provider.MarketSearchResult `json:"markets"`
	Note        string                      `json:"note"`
}

func runPolyweather(ctx context.Context, argv []string) error {
	parsed, err := parseArgs(argv, map[string]optionSpec{
		"days":   {kind: intOption, defaultVal: "3", min: 1, max: 16},
		"unit":   {kind: stringOption, defaultVal: "f", choices: []string{"f", "c"}},
		"market": {kind: stringOption},
		"limit":  {kind: intOption, defaultVal: "3", min: 1, max: 10},
	})
	if err != nil {
		return err
	}
	station, err := required(parsed.positionals, 0, "ICAO station")
	if err != nil {
		return err
	}
	client := provider.NewClient()
	metar, err := client.GetMETAR(ctx, []string{station}, 2)
	if err != nil {
		return err
	}
	if len(metar.Observations) == 0 {
		return provider.NewError("not_found", "NOAA returned no current observation for the station", 3)
	}
	observation := metar.Observations[0]
	city := strings.Join(parsed.positionals[1:], " ")
	if city == "" {
		city = inferCity(observation.Name)
	}
	location := provider.Location{Name: city, Latitude: observation.Latitude, Longitude: observation.Longitude}
	marketQuery := parsed.value("market")
	if marketQuery == "" {
		marketQuery = "highest temperature in " + city
	}

	type forecastResponse struct {
		value provider.ForecastResult
		err   error
	}
	type marketsResponse struct {
		value provider.MarketSearchResult
		err   error
	}
	forecastChannel := make(chan forecastResponse, 1)
	marketsChannel := make(chan marketsResponse, 1)
	go func() {
		value, fetchErr := client.Forecast(ctx, location, parsed.integer("days"), parsed.value("unit"), false)
		forecastChannel <- forecastResponse{value: value, err: fetchErr}
	}()
	go func() {
		value, fetchErr := client.SearchMarkets(ctx, marketQuery, parsed.integer("limit"), false)
		marketsChannel <- marketsResponse{value: value, err: fetchErr}
	}()
	forecast := <-forecastChannel
	markets := <-marketsChannel
	if forecast.err != nil {
		return forecast.err
	}
	if markets.err != nil {
		return markets.err
	}
	result := polyweatherResult{
		Source: "noaa+open-meteo+polymarket", Station: strings.ToUpper(station), Location: city,
		Observation: observation, Forecast: forecast.value, Markets: markets.value,
		Note: "Informational only. Check each market's resolution source and rules before acting.",
	}
	if parsed.flag("json") {
		return render.JSON(os.Stdout, result)
	}
	fmt.Fprintf(os.Stdout, "%s weather-market desk  (%s)\n\n", city, strings.ToUpper(station))
	fmt.Fprintf(os.Stdout, "METAR %s: %s°C, wind %s° at %s kt\n%s\n\n", observation.ReportTime,
		pointerNumber(observation.Temperature), render.Text(observation.WindDir), pointerNumber(observation.WindSpeed), observation.Raw)
	printForecast(forecast.value)
	fmt.Fprintln(os.Stdout, "\nPolymarket")
	printMarketSearch(markets.value)
	fmt.Fprintln(os.Stdout, "\nInformational only. Check the market resolution source and rules.")
	return nil
}

func inferCity(name string) string {
	city := strings.TrimSpace(strings.Split(name, ",")[0])
	for _, suffix := range []string{" International Airport", " Intl Airport", " Intl", " Airport"} {
		city = strings.TrimSuffix(city, suffix)
	}
	if city == "" {
		return "weather"
	}
	return city
}

func runProviders(argv []string) error {
	parsed, err := parseArgs(argv, map[string]optionSpec{})
	if err != nil {
		return err
	}
	_, upstreamErr := exec.LookPath("polymarket")
	providers := []map[string]any{
		{"name": "NOAA METAR", "command": "metar", "status": "ready", "auth": "none"},
		{"name": "Open-Meteo", "command": "open-meteo", "status": "ready", "auth": "none"},
		{"name": "Polymarket public APIs", "command": "betmoar", "status": "ready", "auth": "none for read-only"},
		{"name": "Polymarket official CLI", "command": "betmoar upstream", "status": readyStatus(upstreamErr == nil), "auth": "wallet only for trading"},
		{"name": "Wethr.net", "command": "wethr", "status": configuredStatus("WETHR_API_KEY"), "auth": "WETHR_API_KEY, paid API plan"},
		{"name": "meteoblue", "command": "meteoblue", "status": configuredStatus("METEOBLUE_API_KEY"), "auth": "METEOBLUE_API_KEY"},
		{"name": "Weather Underground PWS", "command": "wunderground", "status": configuredStatus("WEATHER_COMPANY_API_KEY"), "auth": "WEATHER_COMPANY_API_KEY with PWS entitlement"},
	}
	if parsed.flag("json") {
		return render.JSON(os.Stdout, map[string]any{"providers": providers})
	}
	rows := [][]string{}
	for _, item := range providers {
		rows = append(rows, []string{fmt.Sprint(item["name"]), fmt.Sprint(item["command"]), fmt.Sprint(item["status"]), fmt.Sprint(item["auth"])})
	}
	fmt.Fprintln(os.Stdout, render.Table([]string{"PROVIDER", "COMMAND", "STATUS", "AUTH"}, rows))
	return nil
}

func configuredStatus(name string) string {
	if strings.TrimSpace(os.Getenv(name)) == "" {
		return "not_configured"
	}
	return "configured"
}

func readyStatus(ready bool) string {
	if ready {
		return "ready"
	}
	return "not_installed"
}
