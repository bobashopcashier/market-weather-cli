# Schema-drift containment benchmark

This fixed regression benchmark exercises the response-shape validator used by
the versioned contracts in `weather-cli`. The implemented matrix is in
`internal/provider/schema_drift_benchmark_test.go`; the later sections also list
the predeclared cases for expanding coverage into dynamic provider payloads and
the composite command.

The benchmark asks one narrow question: when a provider returns syntactically
valid HTTP 200 JSON that no longer satisfies a command's expected response, does
the interface return a correct result, reject the drift explicitly, or silently
accept a wrong result?

It does **not** estimate provider reliability, production incident frequency,
or agent failure rates. Repeating the same fixed cases demonstrates regression
stability; it does not create independent samples or justify a confidence
interval.

## Current scope and result

The current 24 cases cover public response structures that can be frozen without
credentials:

| Command | Provider surface | Output contract |
|---|---|---|
| `metar` | NOAA METAR observations | `mwx.output/metar/v1` |
| `open-meteo` | Open-Meteo geocoding | `mwx.output/open-meteo/v1` |
| `betmoar search` | Polymarket Gamma search | `mwx.output/betmoar.search/v1` |

| Arm | Compatible cases | Breaks detected | Silently accepted breaks |
|---|---:|---:|---:|
| Unvalidated typed JSON decoder | 10/10 | 4/14 | 10/14 |
| `weather-cli` provider-shape validator | 10/10 | **14/14** | **0/14** |

The last two columns use the same 14 breaking responses and are complements in
this matrix. Higher detection and lower silent acceptance are better. Every
validator detection included the expected JSON path.

Run it with:

```sh
go test ./internal/provider -run TestSchemaDriftBenchmark -count=1 -v
```

The current result is deliberately narrow. Open-Meteo forecast maps, Wethr
data, meteoblue data, Weather Underground data, and provider-specific
order-book metadata contain dynamic descendants. Task-critical presence can be
protected at command runtime with `--require-fields`, but those paths are not
yet counted in this structural matrix and `any` descendants do not gain a type
assertion.

## Expansion scope

Add frozen public fixtures for:

| Command | Provider surface | Output contract |
|---|---|---|
| `open-meteo` | Open-Meteo forecast maps | `mwx.output/open-meteo/v1` |
| `betmoar book` | Polymarket CLOB order book | `mwx.output/betmoar.book/v1` |

Add one command-level `polyweather` case in which a valid first provider call is
followed by a schema-broken forecast or market response. It verifies atomic
multi-provider behavior, but it should not be counted as pagination coverage.
None of the initial surfaces follows an upstream cursor, so this benchmark must
not claim to measure atomic pagination. Add page-two drift and cursor-cycle
cases only when a native command actually implements cursor pagination.

Paid providers can join the matrix after sanitized fixtures and explicit
response contracts exist. The benchmark must never contact a live provider or
load credentials.

## Paired arms

Every scenario sends the same frozen status, headers, and body through two
in-process arms:

1. `direct-api-json` runs Go's typed `encoding/json` decoder without the
   response-shape validator. It accepts syntactically valid JSON according to
   ordinary Go decoding rules.
2. `weather-cli-validator` decodes the same fixture with `UseNumber`, validates required
   paths and declared JSON types, then performs the same typed decode.

The direct arm deliberately shares fixtures and typed targets with the
validator arm so the comparison isolates pre-decode structural validation. The
current matrix does not invoke command normalization or rendering. It is not
`curl`, a separate SDK, an agent, or a best-possible hand-written API client.
A third equivalent-schema-validator arm may be added later to demonstrate that
the benefit comes from shipping the contract, not from the CLI process boundary.

In the current structural matrix, the case manifest is the oracle: a breaking
case that the direct decoder accepts without error is classified as silent
acceptance. The expanded command-level benchmark should add a task scorer that
is separate from both arms, knows the fixture's intended semantics, and decides
whether the command data remains usable. Scoring rules must never be exposed to
either arm as an implicit validator.

## Scenario manifest

Each case has a stable identifier and declares its truth before execution:

```json
{
  "name": "open-meteo-daily-high-wrong-type",
  "profile": "open-meteo",
  "truth": "breaking",
  "mutation": "daily temperature_2m_max element changes from number to string",
  "expectedPath": "forecast.daily.temperature_2m_max[]",
  "fields": [
    "units.temperature",
    "forecast.daily.time",
    "forecast.daily.temperature_2m_max"
  ],
  "requireFields": [
    "forecast.daily.time",
    "forecast.daily.temperature_2m_max"
  ]
}
```

Store canonical fixtures and mutation manifests with content hashes. Inject a
fixed clock so CLI-owned `fetchedAt` values are reproducible. A mutation helper
must assert that it changed exactly the intended JSON path; malformed test setup
is a harness failure, not an interface outcome.

## Compatible expansion cases

Compatible cases must succeed in both arms and satisfy the fixed task oracle.
Include at least:

- Unmodified canonical responses.
- Additive unknown fields at the root and nested object levels.
- JSON object key reordering.
- Omission of fields declared optional by the command contract.
- Empty result collections where the provider legitimately permits no matches.
- Both Polymarket encodings already supported for string-list fields: direct
  arrays and JSON-encoded strings.
- Extra Open-Meteo `current` or `hourly` fields outside the requested
  projection.

An optional projected field that is absent without `--require-fields` follows
the command's documented omission or null-materialization rule and is not a
breaking case. The same task-critical path becomes a breaking case when named
in `--require-fields`.

## Breaking expansion cases

Breaking cases are valid JSON responses that violate a declared contract or the
fixed task requirements.

### METAR

- Root observations change from an array to an object.
- `icaoId` is missing, renamed, null, or the wrong type in any returned item.
- A task-required `reportTime` is missing or is not valid RFC 3339.
- A selected temperature changes from a JSON number to a string.
- A later item in a multi-observation response drifts while earlier items remain
  valid, proving validation applies to every item.

### Open-Meteo

- Geocoding `results` changes type. Omission remains the provider's valid
  no-results response and must preserve `not_found` behavior.
- The selected location loses `name`, `latitude`, or `longitude`, or a
  coordinate changes type.
- `daily` or `daily.time` is missing, renamed, null, or changes shape.
- A required high/low temperature series is missing or contains a wrong-type
  element.
- A task-required date has the wrong format.
- Parallel daily arrays have incompatible lengths when the response contract
  declares alignment as an invariant.

### Polymarket search

- `events` changes from an array to an object.
- Event identity/title/slug or market identity/question is missing, renamed,
  null, or the wrong type.
- A markets collection changes shape or contains a non-object item.
- Outcome, price, or token arrays use a malformed encoding.
- A malformed or missing volume/liquidity value would otherwise be normalized
  silently to zero.
- A later event or market drifts after earlier entries remain valid.

### Polymarket order book

- `bids` or `asks` is missing, renamed, null, or not an array.
- A level is not an object or loses a task-required price/size field.
- A task-required price or size changes type or format.
- An additive book metadata field remains compatible.

### Composite atomicity

- `polyweather` receives a valid METAR result followed by a schema-broken
  Open-Meteo response.
- `polyweather` receives valid METAR and forecast results followed by a
  schema-broken Polymarket response.

Both composite cases must produce no partial standard output.

## Expected command-level detection for the expansion

The future command-level harness should count a breaking response as
`detected_failure` only when all of these hold:

- The command exits nonzero with stable code `UPSTREAM_SCHEMA_MISMATCH`.
- Standard output is empty.
- The error is a valid versioned envelope for the invoked command.
- The envelope's `outputContractVersion` equals the exact command contract.
- `details.missingFields` or `details.typeMismatches` contains the expected
  normalized JSON path. Add `details.formatMismatches` to this criterion only
  after a format-aware validator is implemented.

Errors and mismatch arrays must be deterministically sorted. A generic decode
error, panic, timeout, or unrelated failure is safe from silent corruption but
is classified as `other_failure`, not a successful drift detection.

## Outcome taxonomy and denominators

Each scenario-arm pair receives exactly one outcome:

- `correct_success`: exit zero and the task oracle accepts the result.
- `detected_failure`: the CLI rejects a breaking response with the exact
  structured diagnosis above.
- `silent_wrong_success`: exit zero, but the task oracle rejects the result.
- `unexpected_correct`: a breaking mutation remains task-correct without being
  rejected.
- `false_positive`: a compatible response is rejected.
- `other_failure`: any remaining nonzero, malformed, or harness-visible result.

The primary unsafe metric is lower-is-better:

```text
silent_wrong_rate = silent_wrong_success / breaking_cases
```

Also report:

```text
detection_rate = detected_failure / breaking_cases
path_localization_rate = detections_with_expected_path / detected_failure
compatible_success_rate = correct_success / compatible_cases
false_positive_rate = false_positive / compatible_cases
```

For every arm, the breaking-case partition must satisfy:

```text
breaking_cases = detected_failure
               + silent_wrong_success
               + unexpected_correct
               + other_failure
```

Publish raw counts alongside rates. Do not pool compatible and breaking cases
into one headline percentage, and do not call a fixed-case pass fraction an
empirical failure probability.

## Determinism and execution

The eventual test should:

- Use only checked-in fixtures and an injected HTTP client.
- Disable network access and credential loading.
- Use a fixed clock and stable command version.
- Run both arms for every scenario in the same process.
- Buffer all CLI JSON before writing it.
- Fail if scenario names, fixture hashes, or expected paths are duplicated.
- Log every scenario-arm outcome.
- Emit one machine-readable summary line, for example
  `SCHEMA_DRIFT_BENCHMARK_JSON=...`.

The expanded command-level regression command will be:

```sh
go test ./internal/cli -run TestCommandSchemaDriftBenchmark -count=1 -v
```

`-count=1` prevents the Go test cache from hiding execution. Repeated runs are
useful for confirming byte-for-byte determinism, but they remain repetitions of
the same designed cases.

## Reporting rules

The report must name the commit, contract versions, fixture hashes, case counts,
and exact task-required paths. It should show results by command and drift class
before any aggregate.

Acceptable wording:

> On this fixed schema-drift matrix, the CLI detected X/Y injected breaking
> responses and silently accepted Z/Y. The unvalidated decoder silently accepted
> A/Y. All B compatible responses succeeded in both arms.

Do not describe the result as Kalshi-, Polymarket-, NOAA-, or Open-Meteo
production reliability. Do not describe it as an agent failure rate, token-usage
measurement, latency benchmark, or evidence that all future schema changes are
covered. A separate randomized, paired agent study is required for claims about
agent outcomes.
