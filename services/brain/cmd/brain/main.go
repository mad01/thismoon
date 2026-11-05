package main

import (
	"os"

	"github.com/mad01/thismoon/services/brain/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
