package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/bobashopcashier/market-weather-cli/internal/provider"
	"github.com/bobashopcashier/market-weather-cli/internal/render"
)

var commands = map[string]func(context.Context, []string) error{
	"betmoar":      runBetmoar,
	"data":         runData,
	"dataframe":    runData,
	"metar":        runMETAR,
	"wethr":        runWethr,
	"polyweather":  runPolyweather,
	"open-meteo":   runOpenMeteo,
	"meteoblue":    runMeteoblue,
	"wunderground": runWunderground,
	"providers": func(_ context.Context, argv []string) error {
		return runProviders(argv)
	},
}

func Run(forcedTool string, argv []string) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	err := execute(ctx, forcedTool, argv)
	if err == nil {
		return 0
	}
	jsonOutput := hasArg(argv, "--json") || hasArg(argv, "-j") || dataDefaultsToJSON(forcedTool, argv) || schemaDefaultsToJSON(forcedTool, argv)
	var appErr *provider.Error
	if !errors.As(err, &appErr) {
		appErr = &provider.Error{Code: "internal_error", Message: err.Error(), ExitCode: 1}
	}
	if jsonOutput {
		_ = render.JSON(os.Stderr, struct {
			SchemaVersion string          `json:"schemaVersion"`
			Error         *provider.Error `json:"error"`
		}{SchemaVersion: "mwx.error/v1", Error: appErr})
	} else {
		fmt.Fprintf(os.Stderr, "error: %s\n", render.SafeText(appErr.Message))
		if appErr.Hint != "" {
			fmt.Fprintf(os.Stderr, "hint: %s\n", render.SafeText(appErr.Hint))
		}
	}
	if appErr.ExitCode == 0 {
		return 1
	}
	return appErr.ExitCode
}

func schemaDefaultsToJSON(forcedTool string, argv []string) bool {
	if forcedTool != "" {
		return len(argv) > 0 && argv[0] == "schema"
	}
	return len(argv) > 0 && argv[0] == "schema" || len(argv) > 1 && argv[1] == "schema"
}

func dataDefaultsToJSON(forcedTool string, argv []string) bool {
	isData := forcedTool == "data" || forcedTool == "dataframe" || forcedTool == "" && len(argv) > 0 && (argv[0] == "data" || argv[0] == "dataframe")
	if !isData {
		return false
	}
	for index, argument := range argv {
		if argument == "--" {
			break
		}
		if strings.HasPrefix(argument, "--output=") || strings.HasPrefix(argument, "-o=") {
			_, value, _ := strings.Cut(argument, "=")
			return value == "json"
		}
		if (argument == "--output" || argument == "-o") && index+1 < len(argv) {
			return argv[index+1] == "json"
		}
	}
	return true
}

func execute(ctx context.Context, forcedTool string, argv []string) error {
	if err := rejectArgumentControls(argv); err != nil {
		return err
	}
	if hasArg(argv, "--version") {
		fmt.Fprintln(os.Stdout, version)
		return nil
	}
	tool := forcedTool
	if tool != "" && len(argv) > 0 && argv[0] == "schema" {
		return runSchema(append([]string{tool}, argv[1:]...))
	}
	if tool == "" {
		if len(argv) == 0 || argv[0] == "help" || argv[0] == "--help" || argv[0] == "-h" {
			fmt.Fprint(os.Stdout, rootHelp)
			return nil
		}
		tool = argv[0]
		argv = argv[1:]
		if tool == "schema" {
			return runSchema(argv)
		}
	}
	if len(argv) > 0 && argv[0] == "schema" {
		return runSchema(append([]string{tool}, argv[1:]...))
	}
	command, ok := commands[tool]
	if !ok {
		err := provider.NewError("invalid_arguments", fmt.Sprintf("unknown tool: %s", tool), 2)
		err.Hint = "Run mwx --help to list available tools."
		return err
	}
	if len(argv) == 0 || argv[0] == "help" || argv[0] == "--help" || argv[0] == "-h" {
		fmt.Fprint(os.Stdout, toolHelp[tool])
		return nil
	}
	isUpstreamPassthrough := tool == "betmoar" && len(argv) > 0 && argv[0] == "upstream"
	if !isUpstreamPassthrough && (hasArg(argv, "--help") || hasArg(argv, "-h")) {
		fmt.Fprint(os.Stdout, toolHelp[tool])
		return nil
	}
	return command(ctx, argv)
}

func rejectArgumentControls(argv []string) error {
	for index, argument := range argv {
		for _, current := range argument {
			if current < 0x20 || current == 0x7f || current >= 0x80 && current <= 0x9f {
				return provider.NewError("invalid_arguments", fmt.Sprintf("argument %d contains a control character", index+1), 2)
			}
		}
	}
	return nil
}

func hasArg(argv []string, target string) bool {
	for _, value := range argv {
		if value == "--" {
			return false
		}
		if value == target || strings.HasPrefix(value, target+"=") {
			return true
		}
	}
	return false
}
