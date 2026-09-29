package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/present/internal/store"
)

// deckWorkdir points the CLI at a temp workdir holding one page with a deck
// and one without, and returns their ids.
func deckWorkdir(t *testing.T) (withDeck, withoutDeck string) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.NewFS(dir)
	if err != nil {
		t.Fatal(err)
	}
	d, err := st.Create(t.Context(), store.Draft{
		Title: "D", Deck: "<wk-section></wk-section>", DeckSource: []byte(`{"sections":[]}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	b, err := st.Create(t.Context(), store.Draft{Title: "B", Content: "<p>x</p>"})
	if err != nil {
		t.Fatal(err)
	}
	prev := flagWorkdir
	flagWorkdir = dir
	t.Cleanup(func() { flagWorkdir = prev })
	return d.ID, b.ID
}

func runDeckCmd(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	deckCmd.SetOut(&out)
	t.Cleanup(func() { deckCmd.SetOut(nil) })
	err := runDeck(deckCmd, args)
	return out.String(), err
}

func TestDeckCommandWritesAndPrints(t *testing.T) {
	id, _ := deckWorkdir(t)

	out, err := runDeckCmd(t, id, "start")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if want := id + ": start seq 1\n"; out != want {
		t.Errorf("start printed %q, want %q", out, want)
	}
	out, err = runDeckCmd(t, id, "goto", "4")
	if err != nil {
		t.Fatalf("goto: %v", err)
	}
	if want := id + ": goto seq 2 slide 4\n"; out != want {
		t.Errorf("goto printed %q, want %q", out, want)
	}

	st, err := store.NewFS(flagWorkdir)
	if err != nil {
		t.Fatal(err)
	}
	cmd, err := st.DeckCommand(t.Context(), id)
	if err != nil {
		t.Fatalf("DeckCommand: %v", err)
	}
	if cmd.Seq != 2 || cmd.Action != store.DeckGoto || cmd.Slide != 4 {
		t.Errorf("stored command = %+v", cmd)
	}
}

func TestDeckCommandArguments(t *testing.T) {
	withDeck, withoutDeck := deckWorkdir(t)
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"goto without slide", []string{withDeck, "goto"}, "goto needs a slide number"},
		{"goto with a word", []string{withDeck, "goto", "two"}, "want an integer"},
		{"goto below one", []string{withDeck, "goto", "0"}, "1 or more"},
		{"next with a slide", []string{withDeck, "next", "2"}, "takes no slide number"},
		{"unknown action", []string{withDeck, "rewind"}, "unknown deck action"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := runDeckCmd(t, tc.args...)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("args %v: err = %v, want it to mention %q", tc.args, err, tc.want)
			}
		})
	}

	if _, err := runDeckCmd(t, withoutDeck, "next"); !errors.Is(err, store.ErrNoDeck) {
		t.Errorf("page without a deck: err = %v, want ErrNoDeck", err)
	}
	if _, err := runDeckCmd(t, "missing0000", "next"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("missing page: err = %v, want ErrNotFound", err)
	}
	if deckCmd.Args == nil {
		t.Fatal("deck command must bound its positional arguments")
	}
	if err := deckCmd.Args(deckCmd, []string{"only-id"}); err == nil {
		t.Error("one argument must be refused")
	}
	if err := deckCmd.Args(deckCmd, []string{"id", "goto", "2", "extra"}); err == nil {
		t.Error("four arguments must be refused")
	}
}
