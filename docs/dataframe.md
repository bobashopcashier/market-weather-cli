# Dataframe CLI

`dataframe` is a pure Go table processor based on the operations demonstrated
in the Kaggle notebook [“35 important pandas functions”](https://www.kaggle.com/code/saeedeheydarian/35-important-pandas-functions).
It does not embed Python, execute user code, or depend on pandas.

Every command is also available as `mwx data <operation>`.

## Command shape

```text
dataframe <operation> [input|-] [options]
mwx data <operation> [input|-] [options]
```

The input defaults to `-`, which means standard input.

### Input options

| Option | Meaning |
|---|---|
| `--input`, `-i` | Input path, as an alternative to the positional path |
| `--input-format auto|csv|json|jsonl` | Input decoder, detected by extension or content by default |
| `--input-root DIR` | Restrict reads to a user-approved directory; also available as `MWX_INPUT_ROOT` |
| `--path /pointer` | RFC 6901 JSON Pointer applied before table decoding |
| `--layout auto|records|columns` | Interpret JSON as objects per row or arrays per column |
| `--strings` | Disable CSV scalar inference and keep non-empty fields as strings |

Empty CSV fields are null. With normal inference, integers, floats, and
booleans are converted to native values. JSON numbers retain their numeric
meaning. A 256 MiB per-input safety limit prevents an accidental unbounded
allocation. Infinity remains numeric for filtering and aggregation. Because
JSON has no infinity value, JSON output serializes it as null; CSV and table
output preserve it as `+Inf` or `-Inf`.

### Output options

| Option | Meaning |
|---|---|
| `--output json|csv|table`, `-o` | Output format, defaults to JSON |
| `--json`, `-j` | Explicit JSON output |
| `--fields A,B` | Project final table columns after the operation |
| `--limit N` | Emit at most N final rows; requires JSON so truncation metadata is preserved |
| `--compact` | Emit single-line JSON |

`profile`, `idxmax`, and `to-numpy` return structured result documents and
support JSON output only. `to-numpy` also accepts `--fields` and `--limit`
before creating its matrix. Other operations return tables in any output
format.

Table-valued JSON is an ordered, reusable envelope:

```json
{
  "schemaVersion": "mwx.table/v1",
  "columns": [
    {"name": "Age", "type": "number"}
  ],
  "rows": [[22], [38]],
  "meta": {"rowCount": 2, "columnCount": 1}
}
```

The CLI accepts that envelope as input, which makes pipelines directly
round-trippable and deterministic. Types are inferred again from current values
on each output, so an empty or all-null column is reported as type `null`.
When `--limit` truncates a table, `meta.truncated` is true and
`meta.sourceRowCount` records the pre-limit row count.

`--fields` and `--limit` are final output controls, so they work consistently
across every table-valued operation. Structured results from `profile`,
`idxmax`, and `to-numpy` support `--compact`, but not table projection.

For agent execution, prefer `MWX_INPUT_ROOT` or `--input-root`. The root is
opened as a capability, symlink and traversal escapes are rejected during the
open, and only regular files inside it can be read. Standard input (`-`)
remains allowed. Without an input root, the CLI retains normal local-tool
behavior and accepts arbitrary readable paths.

High-amplification `get-dummies` and `concat` results are capped at five
million cells. `concat` also enforces cumulative retained-input limits of five
million cells and approximately 128 MiB, with no more than 64 additional files.

## All 35 operations

Both the pandas spelling with underscores and the CLI spelling with hyphens are
accepted. For example, `drop_duplicates` and `drop-duplicates` are equivalent.

| # | Notebook function | CLI operation | Important options or result |
|---:|---|---|---|
| 1 | `read_csv` | `read-csv` | Reads CSV and emits a table |
| 2 | `columns` | `columns` | One row per ordered column name |
| 3 | `head` | `head` | `--n 5` |
| 4 | `tail` | `tail` | `--n 5` |
| 5 | `shape` | `shape` | One row containing row and column counts |
| 6 | `info` | `info` | Type, non-null, null, and distinct counts |
| 7 | `describe` | `describe` | `--columns Age,Fare`, defaults to numeric columns |
| 8 | `select_dtypes` | `select-dtypes` | `--include number,string`, `--exclude object` |
| 9 | `astype` | `astype` | `--columns Age --dtype int|float|string|bool` |
| 10 | `value_counts` | `value-counts` | `--column Sex`, optional `--include-null` |
| 11 | `unique` | `unique` | `--column Embarked` |
| 12 | `nunique` | `nunique` | All columns or `--column`, optional `--include-null` |
| 13 | `isnull` | `isnull` | Boolean table, optional `--sum` for per-column counts |
| 14 | `notnull` | `notnull` | Boolean table, optional `--sum` |
| 15 | `duplicated` | `duplicated` | `--subset A,B --keep first|last|none` |
| 16 | `drop_duplicates` | `drop-duplicates` | `--subset A,B --keep first|last|none` |
| 17 | `rename` | `rename` | `--mapping old:new,old2:new2` |
| 18 | `map` | `map` | `--column Sex --mapping male:0,female:1`, optional `--keep-unmapped` |
| 19 | `query` | `query` | `--expr 'Age >= 18 and Fare < 100'` |
| 20 | `isin` | `isin` | `--column Embarked --values S,C,Q`, emits a Boolean mask; `--strings` preserves numeric-looking IDs |
| 21 | `drop` | `drop` | `--columns Cabin,Ticket` |
| 22 | `fillna` | `fillna` | `--columns Age --strategy literal|mean|mode`, plus `--value` for literal; `--strings` preserves a literal string |
| 23 | `dropna` | `dropna` | `--subset Age,Fare --how any|all` |
| 24 | `groupby` | `groupby` | `--by Sex`, optional `--agg Age:mean,Fare:max --include-null` |
| 25 | `agg` | `agg` | `--agg column:function[:alias]`, optional `--by --include-null` |
| 26 | `sort_values` | `sort-values` | `--by Age,Fare`, optional `--descending --nulls-first` |
| 27 | `loc` | `loc` | `--where 'Age >= 18' --columns Name,Age` |
| 28 | `iloc` | `iloc` | Half-open `--rows 0:10 --cols 1:4`, negative bounds work |
| 29 | `cut` | `cut` | `--column Age --bins 0,12,19,35,60,100 --labels ...` |
| 30 | `apply` | `apply` | `--expr 'SibSp + Parch' --output-column family_size` |
| 31 | `ProfileReport` | `profile` | Built-in shape, schema, missingness, duplicates, and numeric summary |
| 32 | `idxmax` | `idxmax` | `--column Fare`, returns the original row index and row |
| 33 | `get_dummies` | `get-dummies` | `--columns Sex`, optional single-column `--prefix gender` |
| 34 | `concat` | `concat` | `--with other.csv[,more.json] --axis 0|1` |
| 35 | `to_numpy` | `to-numpy` | Returns a row-major JSON matrix |

Supported aggregation functions are `count`, `nunique`, `min`, `max`, `mode`,
`sum`, `mean`, and `median`. Group keys sort ascending and null keys are dropped
by default, matching pandas. Use `--include-null` to retain a null-key group.

`profile` intentionally provides a focused, deterministic report. The notebook
literally imports `pandas_profiling.ProfileReport`, whose maintained successor
is `ydata-profiling`; neither package is a pandas function or embedded here.

## Expressions

`query`, `loc`, and `apply` use a small side-effect-free expression language.
It supports:

- Numbers, quoted strings, booleans, and `null`
- Column identifiers such as `Age`, `weather.code`, and `family-size`
- Backtick-quoted columns such as `` `Passenger Name` ``
- Arithmetic: `+`, `-`, `*`, `/`, `%`
- Comparisons: `==`, `!=`, `<`, `<=`, `>`, `>=`
- Logic: `and`, `or`, `not`, plus `&&`, `||`, `!`
- Membership: `Embarked in ("S", "C")`
- Parentheses and normal arithmetic precedence

For scalar options, values are inferred as numbers, booleans, or null when
possible. Use `--strings` with `isin` or literal `fillna` to preserve values such
as `001`, or quote an individual value inside the option value.

Expressions cannot call functions, access the filesystem, start processes, or
run arbitrary Go, Python, or shell code. Missing identifiers, invalid types,
and division by zero produce explicit errors.

## Pipelines

Clean a passenger table and compute group summaries:

```bash
dataframe fillna passengers.csv --columns Age --strategy mean \
  | dataframe dropna - --subset Fare \
  | dataframe query - --expr 'Age >= 18' \
  | dataframe groupby - --by Sex --agg Age:mean:average_age,Fare:max:max_fare \
  | dataframe sort-values - --by average_age --descending --output table
```

Select nested provider output with a JSON Pointer:

```bash
metar KSFO KJFK KLAX --json \
  | mwx data loc - --path /observations \
      --where 'temp >= 20' \
      --columns icaoId,reportTime,temp \
      --output table
```

Turn Open-Meteo’s parallel daily arrays into rows:

```bash
open-meteo "San Francisco" --days 7 --json \
  | mwx data describe - \
      --path /forecast/daily \
      --layout columns \
      --columns temperature_2m_min,temperature_2m_max \
      --output table
```

Create categorical indicators:

```bash
dataframe get-dummies passengers.csv --columns Sex,Embarked --output csv > encoded.csv
```

Concatenate compatible files by rows:

```bash
dataframe concat january.csv --with february.csv,march.csv --axis 0 --output csv
```

For `concat --axis 1`, this CLI suffixes duplicate column labels with `.1`,
`.2`, and so on because its table model requires unique column names.

## References

- [pandas DataFrame API](https://pandas.pydata.org/docs/reference/frame.html)
- [Indexing and selecting data](https://pandas.pydata.org/docs/user_guide/indexing.html)
- [Group by: split-apply-combine](https://pandas.pydata.org/docs/user_guide/groupby.html)
- [Working with missing data](https://pandas.pydata.org/docs/user_guide/missing_data.html)
