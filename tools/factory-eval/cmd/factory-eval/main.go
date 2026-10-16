package main

import (
	"fmt"
	"os"

	"github.com/mad01/thismoon/tools/factory-eval/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "factory-eval:", err)
		os.Exit(1)
	}
}
