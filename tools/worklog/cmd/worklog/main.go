package main

import (
	"fmt"
	"os"

	"github.com/mad01/thismoon/tools/worklog/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "worklog:", err)
		os.Exit(1)
	}
}
