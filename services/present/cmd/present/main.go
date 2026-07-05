package main

import (
	"fmt"
	"os"

	"github.com/mad01/thismoon/services/present/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "present: "+err.Error())
		os.Exit(1)
	}
}
