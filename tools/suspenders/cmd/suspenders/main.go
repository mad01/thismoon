package main

import (
	"fmt"
	"os"

	"github.com/mad01/thismoon/tools/suspenders/cmd/suspenders/commands"
)

func main() {
	if err := commands.Execute(); err != nil {
		if commands.IsFindingsError(err) {
			os.Exit(1)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}
