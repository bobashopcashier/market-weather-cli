package cli

const version = "0.1.0"

const rootHelp = `mwx: prediction-market and weather command-line tools

Usage:
  mwx <tool> [arguments] [options]

Tools:
  betmoar       Public Polymarket discovery and order books
  metar         NOAA Aviation Weather Center observations
  wethr         Wethr.net v2 observations and model analytics
  polyweather   Combined METAR, forecast, and Polymarket dashboard
  open-meteo    Geocoded current weather and forecasts
  meteoblue     meteoblue package API
  wunderground  Weather Underground PWS data via The Weather Company
  providers     Show provider readiness and credential requirements

Every tool is also installed as its own executable. Use --json for stable,
machine-readable output. API keys are read only from environment variables.
`

var toolHelp = map[string]string{
	"betmoar": `betmoar: read-only Betmoar-like market tools using public Polymarket APIs

Usage:
  betmoar search <query> [--limit 5] [--closed] [--json]
  betmoar book <token-id> [--json]
  betmoar upstream -- <official-polymarket-cli-arguments>

Examples:
  betmoar search "highest temperature in San Francisco"
  betmoar book 28123805041312479786525514640306506518624678789891919816014477273546501028892
  betmoar upstream -- markets search bitcoin --limit 5

Betmoar does not publish a developer API. This command does not scrape or
impersonate Betmoar. Trading is delegated to Polymarket's official CLI.
`,
	"metar": `metar: current and recent NOAA aviation observations

Usage:
  metar <ICAO> [ICAO...] [--hours 2] [--raw] [--json]

Examples:
  metar KSFO
  metar KSFO KJFK --hours 6 --json
  metar EGLL --raw
`,
	"open-meteo": `open-meteo: current weather and forecast by place or coordinates

Usage:
  open-meteo <location> [--days 7] [--unit f|c] [--hourly] [--json]

Examples:
  open-meteo "San Francisco" --days 5
  open-meteo 37.7749,-122.4194 --unit c --json
`,
	"polyweather": `polyweather: clean-room weather-market dashboard

Usage:
  polyweather <ICAO> [city] [--days 3] [--unit f|c] [--market query] [--json]

Examples:
  polyweather KSFO "San Francisco"
  polyweather KJFK "New York" --market "highest temperature in New York" --json

The command combines NOAA METAR, Open-Meteo, and public Polymarket data. It
does not call undocumented PolyWeather endpoints and does not place trades.
`,
	"wethr": `wethr: documented Wethr.net v2 API

Requires WETHR_API_KEY and a Wethr plan with API access.

Usage:
  wethr obs <ICAO> [--mode latest|history] [--json]
  wethr extreme <ICAO> [--logic nws|wu] [--json]
  wethr forecast <ICAO> [--model HRRR] [--run latest] [--daily] [--json]
  wethr precipitation <ICAO> [--json]
  wethr nws <ICAO> [--date YYYY-MM-DD] [--json]
  wethr pacing <ICAO> [--date YYYY-MM-DD] [--models LIST] [--json]
  wethr accuracy <ICAO> [--window 30d] [--model LIST] [--json]
  wethr nearby <ICAO> [--radius 50] [--json]
`,
	"meteoblue": `meteoblue: documented meteoblue package API

Requires METEOBLUE_API_KEY.

Usage:
  meteoblue <location> [--package basic-1h_basic-day] [--json]

Example:
  meteoblue "San Francisco" --package basic-1h_basic-day --json
`,
	"wunderground": `wunderground: current Weather Underground PWS observations

Requires WEATHER_COMPANY_API_KEY with The Weather Company PWS entitlement.
A consumer Weather Underground subscription is not an API credential.

Usage:
  wunderground <PWS-station-id> [--units e|m|h] [--json]

Example:
  wunderground KMAHANOV10 --units e
`,
}
