package cli

import (
	"errors"
	"strings"
	"testing"

	"github.com/creack/pty"

	"github.com/mad01/thismoon/tools/cards/internal/feedview"
	"github.com/mad01/thismoon/tools/cards/internal/listview"
)

func TestListRefusesTerminalStdin(t *testing.T) {
	ptmx, tty, err := pty.Open()
	if err != nil {
		t.Skipf("no pseudo-terminal: %v", err)
	}
	defer ptmx.Close()
	defer tty.Close()

	if err := runList(tty, "", listview.Config{Style: listStyle}); !errors.Is(err, ErrUsage) {
		t.Errorf("runList(terminal) error = %v, want ErrUsage", err)
	}
}

func TestFeedRefusesTerminalStdin(t *testing.T) {
	ptmx, tty, err := pty.Open()
	if err != nil {
		t.Skipf("no pseudo-terminal: %v", err)
	}
	defer ptmx.Close()
	defer tty.Close()

	if err := runFeed(tty, feedview.Config{Max: 10, Style: feedStyle}); !errors.Is(err, ErrUsage) {
		t.Errorf("runFeed(terminal) error = %v, want ErrUsage", err)
	}
}

func TestFeedMaxBelowOneIsUsageError(t *testing.T) {
	err := runFeed(strings.NewReader(""), feedview.Config{Style: feedStyle})
	if !errors.Is(err, ErrUsage) {
		t.Errorf("runFeed(--max 0) error = %v, want ErrUsage", err)
	}
}

func TestUnknownFlagIsUsageError(t *testing.T) {
	for _, mode := range []string{"list", "feed"} {
		c := root()
		c.SetArgs([]string{mode, "--bogus"})
		if err := c.Execute(); !errors.Is(err, ErrUsage) {
			t.Errorf("%s --bogus error = %v, want ErrUsage", mode, err)
		}
	}
}
