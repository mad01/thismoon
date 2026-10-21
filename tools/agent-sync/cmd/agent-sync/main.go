package main

import (
	"os"

	"github.com/mad01/thismoon/tools/agent-sync/internal/cli"
)

func main() {
	os.Exit(cli.Main(os.Args[1:], os.Stdout, os.Stderr))
}
