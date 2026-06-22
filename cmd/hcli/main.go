package main

import (
	"os"

	"github.com/justinmaks/hedge-local/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
