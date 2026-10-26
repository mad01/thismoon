package main

import (
	"fmt"
	"os"

	"github.com/mad01/thismoon/tools/srtspec/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "srtspec:", err)
		os.Exit(cli.ExitCode(err))
	}
}
