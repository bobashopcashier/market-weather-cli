package cli

const version = "0.3.0"

const rootHelp = `mwx: prediction-market, weather, and dataframe command-line tools

Usage:
  mwx <tool> [arguments] [options]

Tools:
  schema        Machine-readable runtime command schemas
  betmoar       Public Polymarket discovery and order books
  metar         NOAA Aviation Weather Center observations
  wethr         Wethr.net v2 observations and model analytics
  polyweather   Combined METAR, forecast, and Polymarket dashboard
  open-meteo    Geocoded current weather and forecasts
  meteoblue     meteoblue package API
  wunderground  Weather Underground PWS data via The Weather Company
  data          Pandas-style transforms for CSV, JSON, and JSONL tables
  providers     Show provider readiness and credential requirements

Provider tools are also installed as their own executables, and data is
installed as dataframe. Use --json for stable, machine-readable provider
output. API keys are read only from environment variables.

Agent discovery:
  mwx schema
  mwx schema data query
  dataframe schema query
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
	"data": `dataframe: composable, pandas-style table operations in pure Go

Usage:
  mwx data <operation> [input|-] [options]
  dataframe <operation> [input|-] [options]

Input:
  --input-format auto|csv|json|jsonl   Detect by extension/content by default
  --input-root DIR                     Restrict file reads to DIR (or MWX_INPUT_ROOT)
  --path /json/pointer                 Select a nested JSON array or object
  --layout auto|records|columns        Interpret JSON records or column arrays
  --strings                            Keep CSV fields as strings

Output:
  --output json|csv|table              JSON is the composable default
  --json                               Alias for --output json
  --fields NAME[,NAME...]              Project final table columns
  --limit N                            Bound final JSON table rows
  --compact                            Emit single-line JSON

Notebook operations:
  read-csv       columns        head            tail
  shape          info           describe        select-dtypes
  astype         value-counts   unique          nunique
  isnull         notnull        duplicated      drop-duplicates
  rename         map            query           isin
  drop           fillna         dropna          groupby
  agg            sort-values    loc             iloc
  cut            apply          profile         idxmax
  get-dummies    concat         to-numpy

Common examples:
  dataframe read-csv passengers.csv --output table
  dataframe describe passengers.csv --columns Age,Fare --output table
  dataframe query passengers.csv --expr 'Age >= 18 and Survived == 1'
  dataframe groupby passengers.csv --by Sex --agg Age:mean,Fare:max
  dataframe fillna passengers.csv --columns Age --strategy mean
  dataframe cut passengers.csv --column Age --bins 0,12,19,35,60,100 --labels child,teen,adult,middle,senior
  dataframe apply passengers.csv --expr 'SibSp + Parch' --output-column family_size

Compose with every market and weather CLI:
  metar KSFO KJFK --json | mwx data head --path /observations -n 1 --output table
  open-meteo "San Francisco" --json | mwx data describe --path /forecast/daily --layout columns --output table

Use dataframe <operation> --help for this overview. Operation-specific values
are supplied with --column/--columns, --expr, --by, --agg, and related flags.
`,
	"dataframe": `dataframe: composable, pandas-style table operations in pure Go

Usage:
  dataframe <operation> [input|-] [options]

Run mwx data --help for the full operation list and examples.
`,
	"providers": `providers: provider readiness without exposing credential values

Usage:
  mwx providers [--json]
`,
}
