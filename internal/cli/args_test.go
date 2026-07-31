package cli

import "testing"

func TestParseArgsMixedOrder(t *testing.T) {
	parsed, err := parseArgs([]string{"KSFO", "--hours", "6", "--json"}, map[string]optionSpec{
		"hours": {kind: intOption, defaultVal: "2", min: 1, max: 360},
	})
	if err != nil {
		t.Fatal(err)
	}
	if parsed.integer("hours") != 6 || !parsed.flag("json") || len(parsed.positionals) != 1 {
		t.Fatalf("unexpected parsed args: %#v", parsed)
	}
}

func TestParseArgsRejectsUnknown(t *testing.T) {
	if _, err := parseArgs([]string{"--nope"}, map[string]optionSpec{}); err == nil {
		t.Fatal("expected unknown option error")
	}
}
