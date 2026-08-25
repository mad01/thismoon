package main

import (
	"fmt"
	"os"

	"github.com/mad01/thismoon/tools/opener/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "opener:", err)
		os.Exit(1)
	}
}
