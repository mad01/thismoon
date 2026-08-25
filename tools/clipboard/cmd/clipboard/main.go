package main

import (
	"fmt"
	"os"

	"github.com/mad01/thismoon/tools/clipboard/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "clipboard:", err)
		os.Exit(1)
	}
}
