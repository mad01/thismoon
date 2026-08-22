package cli

import (
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestHintRunE(t *testing.T) {
	t.Run("wraps the error with the doc pointer exactly once", func(t *testing.T) {
		sentinel := errors.New("boom")
		run := hintRunE(func(*cobra.Command, []string) error { return sentinel })
		err := run(nil, nil)
		if err == nil {
			t.Fatal("hintRunE(failing run) = nil, want error")
		}
		if !errors.Is(err, sentinel) {
			t.Errorf("errors.Is(err, sentinel) = false, want true; err = %v", err)
		}
		if got, want := strings.Count(err.Error(), "t-man docs"), 1; got != want {
			t.Errorf("hint appears %d times in %q, want %d", got, err.Error(), want)
		}
	})

	t.Run("passes nil through", func(t *testing.T) {
		run := hintRunE(func(*cobra.Command, []string) error { return nil })
		if err := run(nil, nil); err != nil {
			t.Errorf("hintRunE(succeeding run) = %v, want nil", err)
		}
	})
}
