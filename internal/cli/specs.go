package cli

var metarOptions = map[string]optionSpec{
	"hours": {kind: intOption, defaultVal: "2", min: 1, max: 360, description: "Observation lookback window in hours"},
	"raw":   {kind: boolOption, description: "Print raw METAR reports"},
}

var openMeteoOptions = map[string]optionSpec{
	"days":   {kind: intOption, defaultVal: "7", min: 1, max: 16, description: "Forecast days"},
	"unit":   {kind: stringOption, defaultVal: "f", choices: []string{"f", "c"}, description: "Temperature unit"},
	"hourly": {kind: boolOption, description: "Include hourly forecast data"},
}

var polyweatherOptions = map[string]optionSpec{
	"days":   {kind: intOption, defaultVal: "3", min: 1, max: 16, description: "Forecast days"},
	"unit":   {kind: stringOption, defaultVal: "f", choices: []string{"f", "c"}, description: "Temperature unit"},
	"market": {kind: stringOption, maxLength: 512, normalize: "trim", description: "Polymarket search query"},
	"limit":  {kind: intOption, defaultVal: "3", min: 1, max: 10, description: "Maximum market events"},
}

var betmoarOptions = map[string]map[string]optionSpec{
	"search": {
		"limit":  {kind: intOption, defaultVal: "5", min: 1, max: 50, description: "Maximum market events"},
		"closed": {kind: boolOption, description: "Include closed markets"},
	},
	"book": {},
}

var upstreamOptions = map[string]optionSpec{
	"dry-run": {kind: boolOption, description: "Resolve and print the delegated argv without starting the process"},
}

var wethrOptions = map[string]map[string]optionSpec{
	"obs": {
		"mode": {kind: stringOption, defaultVal: "latest", choices: []string{"latest", "history"}, description: "Observation mode"},
	},
	"extreme": {
		"logic": {kind: stringOption, defaultVal: "nws", choices: []string{"nws", "wu"}, description: "Temperature extreme logic"},
	},
	"forecast": {
		"model": {kind: stringOption, maxLength: 256, pattern: resourceValueSchemaPattern, normalize: "trim", description: "Forecast model identifier"},
		"run":   {kind: stringOption, defaultVal: "latest", maxLength: 256, pattern: resourceValueSchemaPattern, normalize: "trim", description: "Forecast model run"},
		"daily": {kind: boolOption, description: "Return daily forecast data"},
	},
	"precipitation": {},
	"nws": {
		"date": {kind: stringOption, maxLength: 10, pattern: dateSchemaPattern, format: "date", normalize: "trim", description: "Forecast date in YYYY-MM-DD format"},
	},
	"pacing": {
		"date":   {kind: stringOption, maxLength: 10, pattern: dateSchemaPattern, format: "date", normalize: "trim", description: "Forecast date in YYYY-MM-DD format"},
		"models": {kind: stringOption, maxLength: 256, pattern: resourceValueSchemaPattern, normalize: "trim", description: "Comma-separated forecast model identifiers"},
	},
	"accuracy": {
		"window": {kind: stringOption, defaultVal: "30d", maxLength: 5, pattern: windowSchemaPattern, normalize: "trim", description: "Accuracy window such as 30d"},
		"model":  {kind: stringOption, maxLength: 256, pattern: resourceValueSchemaPattern, normalize: "trim", description: "Forecast model identifier"},
	},
	"nearby": {
		"radius": {kind: intOption, defaultVal: "50", min: 1, max: 500, description: "Search radius"},
	},
}

var meteoblueOptions = map[string]optionSpec{
	"package": {kind: stringOption, defaultVal: "basic-1h_basic-day", maxLength: 128, pattern: meteobluePackageSchemaPattern, description: "meteoblue package identifier"},
}

var wundergroundOptions = map[string]optionSpec{
	"units": {kind: stringOption, defaultVal: "e", choices: []string{"e", "m", "h"}, description: "Weather Company unit system"},
}
