package main

import (
	"fmt"
	"os"

	"github.com/mad01/thismoon/tools/bionic/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "bionic: "+err.Error())
		os.Exit(1)
	}
}
