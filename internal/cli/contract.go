package cli

import (
	"fmt"
	"strings"

	"github.com/bobashopcashier/market-weather-cli/internal/provider"
)

const agentSchemaVersion = "mwx.agent/v1"

const genericErrorOutputContractVersion = "mwx.output/error/v1"

type agentEnvelope struct {
	SchemaVersion         string          `json:"schemaVersion"`
	OutputContractVersion string          `json:"outputContractVersion"`
	OK                    bool            `json:"ok"`
	Command               string          `json:"command"`
	Data                  any             `json:"data,omitempty"`
	Error                 *provider.Error `json:"error,omitempty"`
}

func commandName(path []string) string {
	return strings.Join(path, ".")
}

func outputContractVersion(command string) string {
	definition, ok := definitionByName(command)
	if !ok || definition.Passthrough || definition.OutputContractRevision < 1 {
		return ""
	}
	return outputContractVersionFor(definition)
}

func outputContractVersionFor(definition commandDefinition) string {
	return fmt.Sprintf("mwx.output/%s/v%d", commandName(definition.Path), definition.OutputContractRevision)
}

func errorOutputContractVersion(command string) string {
	definition, ok := definitionByName(command)
	if !ok || definition.Passthrough || definition.OutputContractRevision < 1 {
		return genericErrorOutputContractVersion
	}
	return outputContractVersionFor(definition)
}

func requestedCommand(forcedTool string, argv []string) string {
	if forcedTool != "" {
		if len(argv) > 0 && argv[0] == "schema" {
			return "schema"
		}
		if (forcedTool == "betmoar" || forcedTool == "wethr") && len(argv) > 0 && !strings.HasPrefix(argv[0], "-") {
			return forcedTool + "." + argv[0]
		}
		return forcedTool
	}
	if len(argv) == 0 {
		return ""
	}
	if argv[0] == "schema" {
		return "schema"
	}
	if (argv[0] == "betmoar" || argv[0] == "wethr") && len(argv) > 1 && !strings.HasPrefix(argv[1], "-") {
		return argv[0] + "." + argv[1]
	}
	return argv[0]
}
