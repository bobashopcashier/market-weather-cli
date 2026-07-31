package cli

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/bobashopcashier/market-weather-cli/internal/provider"
	"github.com/bobashopcashier/market-weather-cli/internal/render"
)

func runMETAR(ctx context.Context, argv []string) error {
	parsed, err := parseArgs(argv, metarOptions)
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
		return writeJSON(parsed, result)
	}
	if len(result.Observations) == 0 {
		return provider.NewError("not_found", "NOAA returned no observations for the requested station and time window", 3)
	}
	for index, observation := range result.Observations {
		if index > 0 && !parsed.flag("raw") {
			fmt.Fprintln(os.Stdout)
		}
		if parsed.flag("raw") {
			fmt.Fprintln(os.Stdout, render.SafeText(observation.Raw))
			continue
		}
		fmt.Fprintf(os.Stdout, "%s  %s  %s\n", render.SafeText(observation.ICAOID), render.SafeText(observation.ReportTime), render.SafeText(observation.FlightCat))
		fmt.Fprintf(os.Stdout, "  temperature %s°C  dewpoint %s°C  wind %s° at %s kt  visibility %s sm\n",
			pointerNumber(observation.Temperature), pointerNumber(observation.Dewpoint), render.Text(observation.WindDir),
			pointerNumber(observation.WindSpeed), render.Text(observation.Visibility))
		fmt.Fprintf(os.Stdout, "  %s\n", render.SafeText(observation.Raw))
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
	parsed, err := parseArgs(argv, openMeteoOptions)
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
		return writeJSON(parsed, result)
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
	fmt.Fprintf(os.Stdout, "%s  (%g, %g)\n", render.SafeText(locationLabel), result.Location.Latitude, result.Location.Longitude)
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
		return runPolymarketUpstream(ctx, argv[1:])
	}
	commandOptions, ok := betmoarOptions[argv[0]]
	if !ok {
		return provider.NewError("invalid_arguments", fmt.Sprintf("unknown betmoar command: %s", argv[0]), 2)
	}
	parsed, err := parseArgs(argv[1:], commandOptions, argv[0] == "book")
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
			return writeJSON(parsed, result)
		}
		printMarketSearch(result)
		return nil
	case "book":
		if err := rejectExtraPositionals(parsed.positionals, 1, "betmoar book"); err != nil {
			return err
		}
		tokenID, err := required(parsed.positionals, 0, "token ID")
		if err != nil {
			return err
		}
		result, err := provider.NewClient().GetOrderBook(ctx, tokenID)
		if err != nil {
			return err
		}
		return writeJSON(parsed, result)
	default:
		return provider.NewError("invalid_arguments", fmt.Sprintf("unknown betmoar command: %s", argv[0]), 2)
	}
}

func runPolymarketUpstream(ctx context.Context, argv []string) error {
	dryRun := false
	if len(argv) > 0 && argv[0] == "--dry-run" {
		dryRun = true
		argv = argv[1:]
	}
	if len(argv) == 0 || argv[0] != "--" {
		err := provider.NewError("invalid_arguments", "betmoar upstream requires an explicit -- boundary", 2)
		err.Hint = "Use betmoar upstream --dry-run -- <arguments> before requesting approval."
		return err
	}
	args := argv[1:]
	for index := 0; index+1 < len(args); index++ {
		if strings.EqualFold(args[index], "wallet") && strings.EqualFold(args[index+1], "import") {
			return provider.NewError("invalid_arguments", "wallet import is blocked because it accepts a positional private key", 2)
		}
	}
	for _, argument := range args {
		name, _, _ := strings.Cut(strings.ToLower(argument), "=")
		switch name {
		case "--private-key", "--api-key", "--secret", "--password", "--credential", "--credentials":
			return provider.NewError("invalid_arguments", "credentials must be supplied through the official CLI's environment-based interfaces, not arguments", 2)
		}
	}
	path, err := exec.LookPath("polymarket")
	if err != nil {
		appErr := provider.NewError("not_configured", "Polymarket's official CLI is not installed", 2)
		appErr.Hint = "Install it with: brew tap Polymarket/polymarket-cli https://github.com/Polymarket/polymarket-cli && brew install polymarket"
		return appErr
	}
	if dryRun {
		return render.JSON(os.Stdout, map[string]any{
			"schemaVersion": "mwx.dry-run/v1",
			"executable":    path,
			"arguments":     args,
			"executes":      false,
			"effects":       map[string]any{"network": true, "mutation": "unknown", "externalProcess": true},
		})
	}
	command := exec.CommandContext(ctx, path, args...)
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := command.Run(); err != nil {
		return provider.NewError("upstream_failed", fmt.Sprintf("official Polymarket CLI failed: %v", err), 1)
	}
	return nil
}

func printMarketSearch(result provider.MarketSearchResult) {
	if len(result.Events) == 0 {
		fmt.Fprintln(os.Stdout, "No active markets found.")
		printMarketTruncation(result.Truncation)
		return
	}
	for eventIndex, event := range result.Events {
		if eventIndex > 0 {
			fmt.Fprintln(os.Stdout)
		}
		fmt.Fprintf(os.Stdout, "%s\nhttps://polymarket.com/event/%s\n", render.SafeText(event.Title), url.PathEscape(event.Slug))
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
	printMarketTruncation(result.Truncation)
}

func printMarketTruncation(truncation []provider.Truncation) {
	if len(truncation) == 0 {
		return
	}
	rows := make([][]string, 0, len(truncation))
	for _, item := range truncation {
		rows = append(rows, []string{item.Path, strconv.Itoa(item.SourceCount), strconv.Itoa(item.EmittedCount)})
	}
	fmt.Fprintln(os.Stdout, "\nResults truncated by safety limits:")
	fmt.Fprintln(os.Stdout, render.Table([]string{"PATH", "AVAILABLE", "SHOWN"}, rows))
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
	parsed, err := parseArgs(argv, polyweatherOptions)
	if err != nil {
		return err
	}
	station, err := required(parsed.positionals, 0, "ICAO station")
	if err != nil {
		return err
	}
	station, err = provider.NormalizeStation(station)
	if err != nil {
		return err
	}
	city := strings.Join(parsed.positionals[1:], " ")
	if city != "" {
		city, err = provider.ValidateFreeText("city", city, 256)
		if err != nil {
			return err
		}
	}
	marketQuery := parsed.value("market")
	if marketQuery != "" {
		marketQuery, err = provider.ValidateFreeText("market search query", marketQuery, 512)
		if err != nil {
			return err
		}
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
	if city == "" {
		city = inferCity(observation.Name)
	}
	location := provider.Location{Name: city, Latitude: observation.Latitude, Longitude: observation.Longitude}
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
		return writeJSON(parsed, result)
	}
	fmt.Fprintf(os.Stdout, "%s weather-market desk  (%s)\n\n", render.SafeText(city), strings.ToUpper(station))
	fmt.Fprintf(os.Stdout, "METAR %s: %s°C, wind %s° at %s kt\n%s\n\n", render.SafeText(observation.ReportTime),
		pointerNumber(observation.Temperature), render.Text(observation.WindDir), pointerNumber(observation.WindSpeed), render.SafeText(observation.Raw))
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

type providerStatus struct {
	Name    string `json:"name"`
	Command string `json:"command"`
	Status  string `json:"status"`
	Auth    string `json:"auth"`
}

type providersResult struct {
	Providers []providerStatus `json:"providers"`
}

func runProviders(argv []string) error {
	parsed, err := parseArgs(argv, map[string]optionSpec{})
	if err != nil {
		return err
	}
	if err := rejectExtraPositionals(parsed.positionals, 0, "providers"); err != nil {
		return err
	}
	_, upstreamErr := exec.LookPath("polymarket")
	providers := []providerStatus{
		{Name: "NOAA METAR", Command: "metar", Status: "ready", Auth: "none"},
		{Name: "Open-Meteo", Command: "open-meteo", Status: "ready", Auth: "none"},
		{Name: "Polymarket public APIs", Command: "betmoar", Status: "ready", Auth: "none for read-only"},
		{Name: "Polymarket official CLI", Command: "betmoar upstream", Status: readyStatus(upstreamErr == nil), Auth: "wallet only for trading"},
		{Name: "Wethr.net", Command: "wethr", Status: configuredStatus("WETHR_API_KEY"), Auth: "WETHR_API_KEY, paid API plan"},
		{Name: "meteoblue", Command: "meteoblue", Status: configuredStatus("METEOBLUE_API_KEY"), Auth: "METEOBLUE_API_KEY"},
		{Name: "Weather Underground PWS", Command: "wunderground", Status: configuredStatus("WEATHER_COMPANY_API_KEY"), Auth: "WEATHER_COMPANY_API_KEY with PWS entitlement"},
	}
	if parsed.flag("json") {
		return writeJSON(parsed, providersResult{Providers: providers})
	}
	rows := [][]string{}
	for _, item := range providers {
		rows = append(rows, []string{item.Name, item.Command, item.Status, item.Auth})
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
