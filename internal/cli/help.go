package cli

import (
	"fmt"
	"sort"
	"strings"
)

const version = "0.2.0"

const rootHelp = `mwx: prediction-market and weather command-line tools

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
  providers     Show provider readiness and credential requirements

Every tool is also installed as its own executable. Use --json or
MWX_OUTPUT=json for stable machine-readable output. With JSON, use --fields to
project response fields and --compact to reduce formatting tokens. API keys are
read only from environment variables.

Agent discovery:
  mwx schema
  mwx schema wethr.forecast
  metar schema

Raw JSON request path:
  metar --params '{"station":["KSFO","KJFK"],"hours":2}' --json
  wethr forecast --params '{"station":"KSFO","model":"HRRR"}' --json

--params accepts one schema-checked JSON object and cannot be mixed with
positional arguments or convenience input flags.

Native commands are read-only. betmoar upstream delegates to another executable
and may trade; inspect its schema and require explicit user review.
`

var toolHelp = map[string]string{
	"betmoar": `betmoar: read-only Betmoar-like market tools using public Polymarket APIs

Usage:
  betmoar search <query> [--limit 5] [--closed] [--json]
  betmoar book <token-id> [--json]
  betmoar upstream --dry-run -- <official-polymarket-cli-arguments>
  betmoar upstream -- <official-polymarket-cli-arguments>

Examples:
  betmoar search "highest temperature in San Francisco"
  betmoar book 28123805041312479786525514640306506518624678789891919816014477273546501028892
  betmoar upstream --dry-run -- markets search bitcoin --limit 5
  betmoar upstream -- markets search bitcoin --limit 5

Betmoar does not publish a developer API. This command does not scrape or
impersonate Betmoar. Trading is delegated to Polymarket's official CLI. The
explicit -- boundary is required. Use --dry-run first and obtain user approval.
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
	"providers": `providers: provider readiness without exposing credential values

Usage:
  mwx providers [--json] [--fields PATHS] [--compact]
`,
}

func commandHelpDefinition(tool string, argv []string) (commandDefinition, bool) {
	path := []string{tool}
	if (tool == "betmoar" || tool == "wethr") && len(argv) > 0 && !strings.HasPrefix(argv[0], "-") {
		path = append(path, argv[0])
	}
	for _, definition := range commandDefinitions() {
		if equalStrings(definition.Path, path) {
			return definition, true
		}
	}
	return commandDefinition{}, false
}

func renderCommandHelp(definition commandDefinition) string {
	var output strings.Builder
	fmt.Fprintf(&output, "%s: %s\n\nUsage:\n  mwx %s", strings.Join(definition.Path, " "), definition.Summary, strings.Join(definition.Path, " "))
	for _, positional := range definition.Positionals {
		label := "<" + positional.Name + ">"
		if !positional.Required {
			label = "[" + positional.Name + "]"
		}
		if positional.Variadic {
			label += "..."
		}
		fmt.Fprintf(&output, " %s", label)
	}
	output.WriteString(" [options]\n")
	if definition.Passthrough {
		output.WriteString("\nThis command requires an explicit -- boundary. It is not agent-invocable and may mutate or trade.\n")
	}
	document := schemaDocument(definition)
	if document.CredentialEnv != "" {
		fmt.Fprintf(&output, "\nCredential:\n  Requires %s in the environment; credential values are not accepted as arguments.\n", document.CredentialEnv)
	}
	if len(document.Options) > 0 {
		output.WriteString("\nOptions:\n")
		for _, option := range document.Options {
			label := option.Name
			if option.Alias != "" {
				label += ", " + option.Alias
			}
			if option.Type != "boolean" {
				label += " <" + option.Type + ">"
			}
			details := []string{}
			if option.Default != nil {
				details = append(details, fmt.Sprintf("default %v", option.Default))
			}
			if len(option.Enum) > 0 {
				values := make([]string, len(option.Enum))
				for index, value := range option.Enum {
					values[index] = fmt.Sprint(value)
				}
				details = append(details, "one of "+strings.Join(values, ","))
			}
			if option.Minimum != nil || option.Maximum != nil {
				details = append(details, fmt.Sprintf("range %v..%v", valueOrNone(option.Minimum), valueOrNone(option.Maximum)))
			}
			description := option.Description
			if len(details) > 0 {
				description += " (" + strings.Join(details, "; ") + ")"
			}
			fmt.Fprintf(&output, "  %-24s %s\n", label, description)
		}
	}
	if document.Params != nil {
		fields := make([]string, 0, len(document.Params.Fields))
		for _, field := range document.Params.Fields {
			fields = append(fields, field.Name)
		}
		sort.Strings(fields)
		if len(fields) > 0 {
			fmt.Fprintf(&output, "\nRaw JSON fields: %s\n", strings.Join(fields, ", "))
		} else {
			output.WriteString("\nRaw JSON request: {}\n")
		}
		output.WriteString("Inspect the complete request and response shapes with mwx schema " + strings.Join(definition.Path, ".") + ".\n")
	}
	if len(definition.Examples) > 0 {
		output.WriteString("\nExamples:\n")
		for _, example := range definition.Examples {
			output.WriteString("  " + example + "\n")
		}
	}
	return output.String()
}

func valueOrNone(value *int) any {
	if value == nil {
		return "none"
	}
	return *value
}
