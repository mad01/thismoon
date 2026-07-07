package main

import (
	"fmt"
	"os"

	"github.com/mad01/thismoon/tools/humanizer/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "humanizer: "+err.Error())
		os.Exit(1)
	}
}
