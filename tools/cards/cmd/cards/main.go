package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/mad01/thismoon/tools/cards/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "cards:", err)
		if errors.Is(err, cli.ErrUsage) {
			os.Exit(2)
		}
		os.Exit(1)
	}
}
