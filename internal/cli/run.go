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
}

func Run(forcedTool string, argv []string) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	err := execute(ctx, forcedTool, argv)
	if err == nil {
		return 0
	}
	jsonOutput := hasArg(argv, "--json") || hasArg(argv, "-j")
	var appErr *provider.Error
	if !errors.As(err, &appErr) {
		appErr = &provider.Error{Code: "internal_error", Message: err.Error(), ExitCode: 1}
	}
	if jsonOutput {
		_ = render.JSON(os.Stderr, map[string]any{"error": appErr})
	} else {
		fmt.Fprintf(os.Stderr, "error: %s\n", appErr.Message)
		if appErr.Hint != "" {
			fmt.Fprintf(os.Stderr, "hint: %s\n", appErr.Hint)
		}
	}
	if appErr.ExitCode == 0 {
		return 1
	}
	return appErr.ExitCode
}

func execute(ctx context.Context, forcedTool string, argv []string) error {
	if hasArg(argv, "--version") {
		fmt.Fprintln(os.Stdout, version)
		return nil
	}
	tool := forcedTool
	if tool == "" {
		if len(argv) == 0 || argv[0] == "help" || argv[0] == "--help" || argv[0] == "-h" {
			fmt.Fprint(os.Stdout, rootHelp)
			return nil
		}
		tool = argv[0]
		argv = argv[1:]
		if tool == "providers" {
			return runProviders(argv)
		}
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
	return command(ctx, argv)
}

func hasArg(argv []string, target string) bool {
	for _, value := range argv {
		if value == target || strings.HasPrefix(value, target+"=") {
			return true
		}
	}
	return false
}
