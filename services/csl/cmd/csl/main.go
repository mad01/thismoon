package main

import (
	"fmt"
	"os"

	"github.com/mad01/thismoon/services/csl/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "csl: %s\n", err)
		os.Exit(1)
	}
}
