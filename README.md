# weather-cli

Standalone Go CLIs for prediction-market research, aviation observations, and
weather forecasts. The project turns the tools in the original list into seven
provider-named commands, with `mwx` as an optional umbrella command.

```text
betmoar       Public Polymarket discovery and order books
metar         NOAA Aviation Weather Center observations
wethr         Wethr.net v2 observations and model analytics
polyweather   METAR + forecast + Polymarket weather-market dashboard
open-meteo    Geocoded current weather and forecasts
meteoblue     meteoblue package API
wunderground  Weather Underground PWS data via The Weather Company
```

The binaries use only the Go standard library. They have stable `--json`
output, explicit exit codes, timeouts, credential redaction, and no shell-based
API calls.

## Install

Build and install all commands into `$(go env GOPATH)/bin`:

```bash
make install
```

Or build local binaries into `dist/`:

```bash
make build
```

Run the umbrella command without installing:

```bash
go run ./cmd/mwx providers
go run ./cmd/mwx open-meteo "San Francisco" --days 5
```

## Quick start

No-key commands work immediately:

```bash
metar KSFO
metar KSFO KJFK --hours 6 --json

open-meteo "San Francisco" --days 5
open-meteo 37.7749,-122.4194 --unit c --json

betmoar search "highest temperature in San Francisco"
betmoar book <yes-token-id> --json

polyweather KSFO "San Francisco"
polyweather KJFK "New York" --json
```

Check all integrations without exposing credential values:

```bash
mwx providers
mwx providers --json
```

## Credentialed providers

Credentials are read only from environment variables. Keys are never accepted
as CLI arguments, and sensitive URL parameters are redacted from errors.

| Provider | Environment variable | Notes |
|---|---|---|
| Wethr.net | `WETHR_API_KEY` | Requires Professional, Developer, or another API-enabled plan |
| meteoblue | `METEOBLUE_API_KEY` | Calls consume credits according to the requested package |
| Weather Underground PWS | `WEATHER_COMPANY_API_KEY` | Requires The Weather Company PWS API entitlement |

Examples:

```bash
wethr obs KMDW
wethr extreme KMDW --logic nws
wethr forecast KMDW --model HRRR --run latest --json
wethr pacing KMDW --models GFS,HRRR,ECMWF-IFS

meteoblue "San Francisco" --package basic-1h_basic-day --json

wunderground KMAHANOV10 --units e --json
```

## Betmoar and trading

Betmoar is a proprietary web terminal and Discord bot. It does not publish a
developer API. The `betmoar` executable is a clean-room, read-only equivalent
for market search and order books, implemented with Polymarket's documented
public APIs. It does not scrape Betmoar or claim to reproduce Betmoar's private
analytics, news curation, or UMA tooling.

Polymarket now publishes an official CLI. If it is installed, the compatibility
command can hand off any operation, including its authenticated workflows:

```bash
brew tap Polymarket/polymarket-cli https://github.com/Polymarket/polymarket-cli
brew install polymarket

betmoar upstream -- markets search bitcoin --limit 5
betmoar upstream -- clob book <token-id>
```

The official CLI is experimental. Review geographic eligibility and its wallet
security guidance before trading.

## Polyweather scope

No supported public PolyWeather API was found, and published terms for one of
the similarly named products prohibit scraping and reverse engineering. The
`polyweather` command therefore builds a legal, transparent dashboard from:

1. NOAA Aviation Weather Center METAR observations
2. Open-Meteo current and daily forecast data
3. Polymarket Gamma API market discovery and current implied probabilities

It does not place trades. Weather contracts can use provider-specific rounding,
station, timezone, and resolution rules, so always inspect the market's stated
resolution source.

## Output contract

Human-readable output is the default. `--json` emits one JSON document to
standard output. Errors go to standard error; with `--json`, errors have this
shape:

```json
{
  "error": {
    "code": "not_configured",
    "message": "WETHR_API_KEY is required for Wethr.net",
    "hint": "Set WETHR_API_KEY in the environment. API keys are never accepted as command arguments."
  }
}
```

Common nonzero outcomes include `invalid_arguments`, `not_configured`,
`authentication_failed`, `plan_required`, `rate_limited`, `not_found`,
`provider_unavailable`, and `timeout`.

## Provider notes

- NOAA asks clients to use a custom User-Agent, stay below 100 requests per
  minute, and avoid polling an endpoint more than once a minute per thread.
- Open-Meteo's hosted free endpoint is for non-commercial use and requires
  attribution. Commercial use needs the appropriate plan and customer endpoint.
- The legacy Weather Underground API was retired. This project uses The Weather
  Company's current PWS API and does not scrape `wunderground.com`.
- A Weather Underground consumer subscription or station-upload key is not a
  Weather Company API entitlement.

Current primary documentation:

- [Polymarket API](https://docs.polymarket.com/api-reference/introduction)
- [Polymarket official CLI](https://github.com/Polymarket/polymarket-cli)
- [NOAA Aviation Weather Center Data API](https://aviationweather.gov/data/api/)
- [Open-Meteo Forecast API](https://open-meteo.com/en/docs)
- [Wethr.net API v2](https://www.wethr.net/edu/api-docs)
- [meteoblue Weather APIs](https://docs.meteoblue.com/en/weather-apis/introduction/overview)
- [The Weather Company developer portal](https://developer.weather.com/)

## Development

```bash
make check
make build
```

`make check` runs formatting, `go vet`, and all tests. CI also builds every
command on Linux.

## License

MIT
