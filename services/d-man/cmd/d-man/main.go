package main

import (
	"fmt"
	"os"

	"github.com/mad01/thismoon/services/d-man/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "d-man: "+err.Error())
		os.Exit(1)
	}
}
