package main

import (
	"os"

	"github.com/dipendra-sharma/jev-cli/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
