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
	command := ""
	if rejectArgumentControls(argv) == nil {
		command = requestedCommand(forcedTool, argv)
	}
	err := execute(ctx, forcedTool, argv)
	if err == nil {
		return 0
	}
	jsonOutput := hasArg(argv, "--json") || hasArg(argv, "-j") || strings.EqualFold(strings.TrimSpace(os.Getenv("MWX_OUTPUT")), "json") || schemaDefaultsToJSON(forcedTool, argv) || commandDefaultsToJSON(forcedTool, argv)
	var appErr *provider.Error
	if !errors.As(err, &appErr) {
		appErr = &provider.Error{Code: "internal_error", Message: err.Error(), ExitCode: 1}
	}
	if jsonOutput {
		envelope := agentEnvelope{
			SchemaVersion: agentSchemaVersion, OutputContractVersion: errorOutputContractVersion(command),
			OK: false, Command: command, Error: appErr,
		}
		if hasArg(argv, "--compact") {
			_ = render.CompactJSON(os.Stderr, envelope)
		} else {
			_ = render.JSON(os.Stderr, envelope)
		}
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

func commandDefaultsToJSON(forcedTool string, argv []string) bool {
	if forcedTool == "betmoar" {
		return len(argv) > 0 && argv[0] == "book"
	}
	return forcedTool == "" && len(argv) > 1 && argv[0] == "betmoar" && argv[1] == "book"
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
	if len(argv) == 0 || argv[0] == "help" {
		fmt.Fprint(os.Stdout, toolHelp[tool])
		return nil
	}
	if hasArg(argv, "--help") || hasArg(argv, "-h") {
		if definition, ok := commandHelpDefinition(tool, argv); ok {
			fmt.Fprint(os.Stdout, renderCommandHelp(definition))
		} else {
			fmt.Fprint(os.Stdout, toolHelp[tool])
		}
		return nil
	}
	expanded, err := expandRawParams(tool, argv)
	if err != nil {
		return err
	}
	if err := preflightResponseFields(tool, expanded); err != nil {
		return err
	}
	return command(ctx, expanded)
}

func rejectArgumentControls(argv []string) error {
	for index, argument := range argv {
		for _, current := range argument {
			if current < 0x20 || current == 0x7f || current >= 0x80 && current <= 0x9f ||
				current >= 0x200b && current <= 0x200f || current >= 0x202a && current <= 0x202e ||
				current >= 0x2066 && current <= 0x2069 || current == 0xfeff {
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
