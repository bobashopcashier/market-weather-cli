package main

import (
	"os"

	"github.com/bobashopcashier/market-weather-cli/internal/cli"
)

func main() { os.Exit(cli.Run("metar", os.Args[1:])) }
