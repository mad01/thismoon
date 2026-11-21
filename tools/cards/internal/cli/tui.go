package cli

import (
	"fmt"
	"io"
	"os"

	tea "charm.land/bubbletea/v2"
	"golang.org/x/term"

	"github.com/mad01/thismoon/tools/cards/internal/input"
)

// ttyPath is the controlling terminal. Every mode reads keys from it and
// draws on it, because stdin is the card stream.
const ttyPath = "/dev/tty"

// checkPipe returns a usage error when stdin is a terminal: the cards come
// on stdin and the keys from the terminal, so stdin must be a pipe or a
// file. mode names the command in the message.
func checkPipe(stdin io.Reader, mode string) error {
	if isTerminal(stdin) {
		return fmt.Errorf("%w: stdin is a terminal; pipe card messages in: producer | cards %s",
			ErrUsage, mode)
	}
	return nil
}

// runTUI shows model on the terminal, feeding it the lines of stdin, until
// the user quits.
func runTUI(stdin io.Reader, model tea.Model) error {
	tty, err := os.OpenFile(ttyPath, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("open terminal: %w", err)
	}
	defer tty.Close()

	p := tea.NewProgram(model, tea.WithInput(tty), tea.WithOutput(tty))
	// Read returns at the end of stdin; when the user quits first, the
	// process exits with it still blocked on the pipe.
	go input.Read(stdin, p.Send)
	_, err = p.Run()
	return err
}

// isTerminal reports whether r is a terminal rather than a pipe or a file.
func isTerminal(r io.Reader) bool {
	f, ok := r.(*os.File)
	return ok && term.IsTerminal(int(f.Fd()))
}
