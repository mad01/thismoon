package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ErrNoDeck is returned when a deck operation names a page that has no deck.
var ErrNoDeck = errors.New("present: page has no deck")

// Remote control actions a deck view applies. Start and stop toggle
// presenting (chrome hidden, one slide filling the window); next, prev,
// and goto move between slides.
const (
	DeckStart = "start"
	DeckStop  = "stop"
	DeckNext  = "next"
	DeckPrev  = "prev"
	DeckGoto  = "goto"
)

// deckCommandFile holds a page's kept remote commands, oldest first.
const deckCommandFile = "deck-command.json"

// deckCommandKeep is how many past commands a page keeps. A tab polls once
// a second and asks for everything after the last sequence number it
// applied, so a sender that writes several commands inside one second
// loses none of them.
const deckCommandKeep = 32

// DeckCommand is one remote command sent to a page's open deck tabs. Tabs
// poll for the commands after the Seq they last applied and run them in
// order; a tab opened afterwards takes only a command sent moments before.
type DeckCommand struct {
	Seq    int64     `json:"seq"`
	Action string    `json:"action"`
	Slide  int       `json:"slide,omitempty"` // goto only, 1-based
	At     time.Time `json:"at"`
}

// ValidateDeckAction reports whether action is one a deck view applies and
// whether slide fits it: goto needs a slide of 1 or more, every other
// action ignores it.
func ValidateDeckAction(action string, slide int) error {
	switch action {
	case DeckStart, DeckStop, DeckNext, DeckPrev:
		return nil
	case DeckGoto:
		if slide < 1 {
			return fmt.Errorf("present: goto needs a slide number of 1 or more, got %d", slide)
		}
		return nil
	default:
		return fmt.Errorf(
			"present: unknown deck action %q (want %s, %s, %s, %s, or %s)",
			action, DeckStart, DeckStop, DeckNext, DeckPrev, DeckGoto,
		)
	}
}

// DeckController relays remote commands to a page's open deck tabs. The
// filesystem store implements it, which is the store a local present runs
// on; the cluster store does not, so a shared instance has no remote
// control. Callers type-assert for it.
type DeckController interface {
	// SendDeckCommand records action (with slide for goto) as the next
	// command, one Seq past the previous one, and returns it. ErrNotFound
	// for a missing page, ErrNoDeck for a page without a deck.
	SendDeckCommand(ctx context.Context, id, action string, slide int) (DeckCommand, error)
	// DeckCommand returns the last command sent to the page, or the zero
	// value (Seq 0) when none was. ErrNotFound for a missing page, ErrNoDeck
	// for a page without a deck.
	DeckCommand(ctx context.Context, id string) (DeckCommand, error)
	// DeckCommandsAfter returns the kept commands with a Seq past after,
	// oldest first, or nil when there are none; the store keeps a bounded
	// number, so a tab far behind sees only the newest. Same errors.
	DeckCommandsAfter(ctx context.Context, id string, after int64) ([]DeckCommand, error)
}

var _ DeckController = (*FS)(nil)

// SendDeckCommand validates the command, appends it to the page's kept
// commands with the next sequence number, and drops the oldest past
// deckCommandKeep.
func (s *FS) SendDeckCommand(
	ctx context.Context, id, action string, slide int,
) (DeckCommand, error) {
	if err := ValidateDeckAction(action, slide); err != nil {
		return DeckCommand{}, err
	}
	cmds, err := s.deckCommands(ctx, id)
	if err != nil {
		return DeckCommand{}, err
	}
	var last int64
	if len(cmds) > 0 {
		last = cmds[len(cmds)-1].Seq
	}
	cmd := DeckCommand{Seq: last + 1, Action: action, At: s.now().UTC()}
	if action == DeckGoto {
		cmd.Slide = slide
	}
	cmds = append(cmds, cmd)
	if len(cmds) > deckCommandKeep {
		cmds = cmds[len(cmds)-deckCommandKeep:]
	}
	raw, err := json.Marshal(cmds)
	if err != nil {
		return DeckCommand{}, fmt.Errorf("encode deck commands: %w", err)
	}
	path := filepath.Join(s.pageDir(id), deckCommandFile)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return DeckCommand{}, fmt.Errorf("write deck commands: %w", err)
	}
	return cmd, nil
}

// DeckCommand reads the page's last remote command; a page that never got
// one answers the zero value.
func (s *FS) DeckCommand(ctx context.Context, id string) (DeckCommand, error) {
	cmds, err := s.deckCommands(ctx, id)
	if err != nil || len(cmds) == 0 {
		return DeckCommand{}, err
	}
	return cmds[len(cmds)-1], nil
}

// DeckCommandsAfter reads the kept commands past after, oldest first.
func (s *FS) DeckCommandsAfter(
	ctx context.Context, id string, after int64,
) ([]DeckCommand, error) {
	cmds, err := s.deckCommands(ctx, id)
	if err != nil {
		return nil, err
	}
	i := 0
	for i < len(cmds) && cmds[i].Seq <= after {
		i++
	}
	if i == len(cmds) {
		return nil, nil
	}
	return cmds[i:], nil
}

// deckCommands reads a page's kept commands, oldest first: nil when none
// was sent, ErrNotFound for a missing page, ErrNoDeck for one without a
// deck.
func (s *FS) deckCommands(ctx context.Context, id string) ([]DeckCommand, error) {
	p, err := s.GetMeta(ctx, id)
	if err != nil {
		return nil, err
	}
	if !p.HasDeck {
		return nil, ErrNoDeck
	}
	raw, err := os.ReadFile(filepath.Join(s.pageDir(id), deckCommandFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read deck commands: %w", err)
	}
	var cmds []DeckCommand
	if err := json.Unmarshal(raw, &cmds); err != nil {
		return nil, fmt.Errorf("decode deck commands: %w", err)
	}
	return cmds, nil
}
