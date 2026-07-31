package cli

import "sort"

var metarOptions = map[string]optionSpec{
	"hours": {kind: intOption, defaultVal: "2", min: 1, max: 360},
	"raw":   {kind: boolOption},
}

var openMeteoOptions = map[string]optionSpec{
	"days":   {kind: intOption, defaultVal: "7", min: 1, max: 16},
	"unit":   {kind: stringOption, defaultVal: "f", choices: []string{"f", "c"}},
	"hourly": {kind: boolOption},
}

var polyweatherOptions = map[string]optionSpec{
	"days":   {kind: intOption, defaultVal: "3", min: 1, max: 16},
	"unit":   {kind: stringOption, defaultVal: "f", choices: []string{"f", "c"}},
	"market": {kind: stringOption},
	"limit":  {kind: intOption, defaultVal: "3", min: 1, max: 10},
}

var betmoarOptions = map[string]map[string]optionSpec{
	"search": {
		"limit":  {kind: intOption, defaultVal: "5", min: 1, max: 50},
		"closed": {kind: boolOption},
	},
	"book": {},
}

var wethrOptions = map[string]map[string]optionSpec{
	"obs": {
		"mode": {kind: stringOption, defaultVal: "latest", choices: []string{"latest", "history"}},
	},
	"extreme": {
		"logic": {kind: stringOption, defaultVal: "nws", choices: []string{"nws", "wu"}},
	},
	"forecast": {
		"model": {kind: stringOption},
		"run":   {kind: stringOption, defaultVal: "latest"},
		"daily": {kind: boolOption},
	},
	"precipitation": {},
	"nws": {
		"date": {kind: stringOption},
	},
	"pacing": {
		"date":   {kind: stringOption},
		"models": {kind: stringOption},
	},
	"accuracy": {
		"window": {kind: stringOption, defaultVal: "30d"},
		"model":  {kind: stringOption},
	},
	"nearby": {
		"radius": {kind: intOption, defaultVal: "50", min: 1, max: 500},
	},
}

var meteoblueOptions = map[string]optionSpec{
	"package": {kind: stringOption, defaultVal: "basic-1h_basic-day"},
}

var wundergroundOptions = map[string]optionSpec{
	"units": {kind: stringOption, defaultVal: "e", choices: []string{"e", "m", "h"}},
}

var dataCommonOptions = map[string]optionSpec{
	"input":        {kind: stringOption, alias: "i"},
	"input-format": {kind: stringOption, defaultVal: "auto", choices: []string{"auto", "csv", "json", "jsonl"}},
	"input-root":   {kind: stringOption},
	"path":         {kind: stringOption},
	"layout":       {kind: stringOption, defaultVal: "auto", choices: []string{"auto", "records", "columns"}},
	"strings":      {kind: boolOption},
	"output":       {kind: stringOption, alias: "o", defaultVal: "json", choices: []string{"json", "csv", "table"}},
	"compact":      {kind: boolOption},
}

var dataProjectionOptions = map[string]optionSpec{
	"fields": {kind: stringOption},
	"limit":  {kind: intOption, min: 1},
}

var dataSpecificOptions = map[string]optionSpec{
	"n":             {kind: intOption, alias: "n", defaultVal: "5"},
	"column":        {kind: stringOption, alias: "c"},
	"columns":       {kind: stringOption},
	"include":       {kind: stringOption},
	"exclude":       {kind: stringOption},
	"dtype":         {kind: stringOption},
	"include-null":  {kind: boolOption},
	"sum":           {kind: boolOption},
	"subset":        {kind: stringOption},
	"keep":          {kind: stringOption, defaultVal: "first", choices: []string{"first", "last", "none"}},
	"mapping":       {kind: stringOption},
	"keep-unmapped": {kind: boolOption},
	"expr":          {kind: stringOption},
	"values":        {kind: stringOption},
	"strategy":      {kind: stringOption, defaultVal: "literal", choices: []string{"literal", "mean", "mode"}},
	"value":         {kind: stringOption},
	"how":           {kind: stringOption, defaultVal: "any", choices: []string{"any", "all"}},
	"by":            {kind: stringOption},
	"agg":           {kind: stringOption},
	"descending":    {kind: boolOption},
	"nulls-first":   {kind: boolOption},
	"where":         {kind: stringOption},
	"rows":          {kind: stringOption},
	"cols":          {kind: stringOption},
	"bins":          {kind: stringOption},
	"labels":        {kind: stringOption},
	"output-column": {kind: stringOption},
	"prefix":        {kind: stringOption},
	"with":          {kind: stringOption},
	"axis":          {kind: intOption, defaultVal: "0", choices: []string{"0", "1"}},
}

type dataOperationSpec struct {
	summary          string
	options          []string
	required         []string
	anyOf            [][]string
	structuredOutput bool
}

var dataOperations = map[string]dataOperationSpec{
	"read-csv":        {summary: "Read CSV input into a table"},
	"columns":         {summary: "List ordered column names"},
	"head":            {summary: "Return the first rows", options: []string{"n"}},
	"tail":            {summary: "Return the final rows", options: []string{"n"}},
	"shape":           {summary: "Return row and column counts"},
	"info":            {summary: "Summarize column types and null counts"},
	"describe":        {summary: "Compute descriptive statistics", options: []string{"columns"}},
	"select-dtypes":   {summary: "Select columns by inferred type", options: []string{"include", "exclude"}, anyOf: [][]string{{"include", "exclude"}}},
	"astype":          {summary: "Cast one or more columns", options: []string{"column", "columns", "dtype"}, required: []string{"dtype"}, anyOf: [][]string{{"column", "columns"}}},
	"value-counts":    {summary: "Count distinct values", options: []string{"column", "include-null"}, required: []string{"column"}},
	"unique":          {summary: "List unique values", options: []string{"column"}, required: []string{"column"}},
	"nunique":         {summary: "Count unique values", options: []string{"column", "include-null"}},
	"isnull":          {summary: "Return a null mask or counts", options: []string{"sum"}},
	"notnull":         {summary: "Return a non-null mask or counts", options: []string{"sum"}},
	"duplicated":      {summary: "Return a duplicate-row mask", options: []string{"subset", "keep"}},
	"drop-duplicates": {summary: "Remove duplicate rows", options: []string{"subset", "keep"}},
	"rename":          {summary: "Rename columns", options: []string{"mapping"}, required: []string{"mapping"}},
	"map":             {summary: "Map values in one column", options: []string{"column", "mapping", "keep-unmapped"}, required: []string{"column", "mapping"}},
	"query":           {summary: "Filter rows with a safe expression", options: []string{"expr"}, required: []string{"expr"}},
	"isin":            {summary: "Test values for membership", options: []string{"column", "values"}, required: []string{"column", "values"}},
	"drop":            {summary: "Drop columns", options: []string{"column", "columns"}, anyOf: [][]string{{"column", "columns"}}},
	"fillna":          {summary: "Fill missing values", options: []string{"column", "columns", "strategy", "value"}, anyOf: [][]string{{"column", "columns"}}},
	"dropna":          {summary: "Drop rows containing nulls", options: []string{"subset", "how"}},
	"groupby":         {summary: "Group and aggregate rows", options: []string{"by", "agg", "include-null"}, required: []string{"by"}},
	"agg":             {summary: "Aggregate columns", options: []string{"by", "agg", "include-null"}, required: []string{"agg"}},
	"sort-values":     {summary: "Sort rows by columns", options: []string{"by", "columns", "descending", "nulls-first"}, anyOf: [][]string{{"by", "columns"}}},
	"loc":             {summary: "Select rows and named columns", options: []string{"where", "columns"}},
	"iloc":            {summary: "Select rows and columns by slice", options: []string{"rows", "cols"}},
	"cut":             {summary: "Bin a numeric column", options: []string{"column", "bins", "labels", "output-column"}, required: []string{"column", "bins"}},
	"apply":           {summary: "Compute a column with a safe expression", options: []string{"expr", "output-column"}, required: []string{"expr", "output-column"}},
	"profile":         {summary: "Build a deterministic profile report", structuredOutput: true},
	"idxmax":          {summary: "Return the row containing a column maximum", options: []string{"column"}, required: []string{"column"}, structuredOutput: true},
	"get-dummies":     {summary: "Create categorical indicator columns", options: []string{"column", "columns", "prefix"}, anyOf: [][]string{{"column", "columns"}}},
	"concat":          {summary: "Concatenate compatible tables", options: []string{"with", "axis"}, required: []string{"with"}},
	"to-numpy":        {summary: "Return a row-major matrix", structuredOutput: true},
}

func dataOperationNames() []string {
	names := make([]string, 0, len(dataOperations))
	for name := range dataOperations {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func dataOperationOptions(operation string) map[string]optionSpec {
	result := make(map[string]optionSpec, len(dataCommonOptions)+8)
	for name, spec := range dataCommonOptions {
		result[name] = spec
	}
	if operation == "read-csv" {
		inputFormat := result["input-format"]
		inputFormat.defaultVal = "csv"
		result["input-format"] = inputFormat
	}
	operationSpec, ok := dataOperations[operation]
	if !ok {
		return result
	}
	for _, name := range operationSpec.options {
		result[name] = dataSpecificOptions[name]
	}
	if operationSpec.structuredOutput {
		output := result["output"]
		output.choices = []string{"json"}
		result["output"] = output
	}
	if !operationSpec.structuredOutput || operation == "to-numpy" {
		for name, spec := range dataProjectionOptions {
			result[name] = spec
		}
	}
	return result
}
