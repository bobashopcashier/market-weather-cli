package cli

import (
	"strings"
	"testing"
)

func TestRawParamsExpandsToTypedArguments(t *testing.T) {
	argv, err := expandRawParams("metar", []string{
		"--params", `{"station":["KSFO","KJFK"],"hours":6,"raw":true}`, "--json", "--fields", "observations.icaoId", "--require-fields", "observations.icaoId", "--compact",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"--hours", "6", "--raw", "--json", "--fields", "observations.icaoId", "--require-fields", "observations.icaoId", "--compact", "--", "KSFO", "KJFK"}
	if strings.Join(argv, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("expanded args = %#v, want %#v", argv, want)
	}

	argv, err = expandRawParams("wethr", []string{"forecast", "--params", `{"station":"KSFO","model":"HRRR","daily":true}`, "--json"})
	if err != nil {
		t.Fatal(err)
	}
	want = []string{"forecast", "--daily", "--model", "HRRR", "--json", "--", "KSFO"}
	if strings.Join(argv, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("expanded nested args = %#v, want %#v", argv, want)
	}
}

func TestRawParamsRejectsAmbiguityAndMalformedObjects(t *testing.T) {
	tests := [][]string{
		{"KSFO", "--params", `{"station":"KJFK"}`},
		{"--hours", "2", "--params", `{"station":"KSFO"}`},
		{"--params", `{"station":"KSFO","station":"KJFK"}`},
		{"--params", `{"station":"KSFO"} []`},
		{"--params", `{"station":"KSFO","unknown":true}`},
		{"--params", `{"station":42}`},
		{"--params", `[]`},
	}
	for _, argv := range tests {
		if _, err := expandRawParams("metar", argv); err == nil {
			t.Errorf("accepted invalid raw params: %#v", argv)
		}
	}
	tooLarge := `{"station":"KSFO","padding":"` + strings.Repeat("x", maximumRawParamsBytes) + `"}`
	if _, err := expandRawParams("metar", []string{"--params", tooLarge}); err == nil {
		t.Fatal("accepted oversized raw params")
	}
}

func TestRawParamsCannotSetOutputControls(t *testing.T) {
	if _, err := expandRawParams("metar", []string{"--params", `{"station":"KSFO","json":true}`}); err == nil {
		t.Fatal("raw params accepted an output-control field")
	}
}

func TestRawParamsPositionalsCannotBecomeFlags(t *testing.T) {
	argv, err := expandRawParams("metar", []string{"--params", `{"station":["KSFO","--hours","360"]}`, "--json"})
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := parseArgs(argv, metarOptions)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.integer("hours") != 2 || len(parsed.positionals) != 3 || parsed.positionals[1] != "--hours" {
		t.Fatalf("positional value was reinterpreted: argv=%#v parsed=%#v", argv, parsed)
	}

	argv, err = expandRawParams("open-meteo", []string{"--params", `{"location":"-33.8688,151.2093"}`, "--json"})
	if err != nil {
		t.Fatal(err)
	}
	parsed, err = parseArgs(argv, openMeteoOptions)
	if err != nil || len(parsed.positionals) != 1 || parsed.positionals[0] != "-33.8688,151.2093" {
		t.Fatalf("negative coordinate did not survive raw params: %#v, err=%v", parsed, err)
	}
}

func FuzzRawParams(f *testing.F) {
	for _, seed := range []string{`{}`, `{"station":"KSFO"}`, `{"station":"KSFO","station":"KJFK"}`, `[]`, `%2e%2e`, "{\x00}"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, value string) {
		if len(value) > maximumRawParamsBytes+1 {
			t.Skip()
		}
		_, _ = expandRawParams("metar", []string{"--params", value})
	})
}
