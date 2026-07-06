package main

import (
	"os"

	"github.com/mad01/thismoon/services/speak/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
