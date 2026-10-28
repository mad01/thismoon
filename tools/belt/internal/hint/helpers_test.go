package hint

import (
	"os/exec"

	"github.com/mad01/thismoon/tools/belt/internal/config"
)

func testConfig() config.Config { return config.Config{} }

func gitCommand(dir string, args ...string) *exec.Cmd {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	return cmd
}
