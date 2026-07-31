package main

import (
	"os"

	"github.com/bobashopcashier/market-weather-cli/internal/cli"
)

func main() { os.Exit(cli.Run("wunderground", os.Args[1:])) }
