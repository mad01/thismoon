package run

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/mad01/thismoon/tools/harvest/internal/digest"
	"github.com/mad01/thismoon/tools/harvest/internal/state"
)

// dateLayout is the date in the user text, as `date '+%Y-%m-%d'` prints it.
const dateLayout = "2006-01-02"

// closingDigest matches a closing digest tag in any case, with the
// whitespace an XML end tag allows before its >.
var closingDigest = regexp.MustCompile(`(?i)</digest\s*>`)

// PrepareConfig is what Prepare needs besides the input.
type PrepareConfig struct {
	State    *state.Dir
	MinBytes int // a digest shorter than this is skipped
	MaxBytes int // a digest keeps its last MaxBytes bytes
	Now      func() time.Time
}

// Prepare is the prepare step. It builds the digest of the lines of
// in.Transcript after the session's mark. A session with nothing new, or a
// digest under the floor, is logged as a skip and leaves the job dir with no
// user text. Otherwise the job dir gets the user text and the counts apply
// reads. Prepare logs no failure: it returns it, and the run logs it.
func Prepare(cfg PrepareConfig, in Input) error {
	st := cfg.State
	job, err := st.Job(in.Session)
	if err != nil {
		return err
	}
	seen, err := st.Mark(in.Session)
	if err != nil {
		return err
	}
	d, err := build(in.Transcript, seen, cfg.MaxBytes)
	if err != nil {
		return err
	}
	// A resumed session ends more than once. The mark holds how many lines
	// the last harvest read, so this one reads only what came after.
	if d.Lines <= seen {
		return st.Log(fmt.Sprintf("skip %s: nothing new since line %d", in.Session, seen))
	}
	if len(d.Text) < cfg.MinBytes {
		return st.Log(fmt.Sprintf("skip %s: digest is %d bytes, under %d",
			in.Session, len(d.Text), cfg.MinBytes))
	}

	if err := job.SetCounts(state.Counts{Lines: d.Lines, DigestBytes: len(d.Text)}); err != nil {
		return err
	}
	user := userText(in, cfg.Now().Format(dateLayout), d.Text)
	if err := os.WriteFile(job.User(), []byte(user), 0o600); err != nil {
		return fmt.Errorf("run: write the user text: %w", err)
	}
	return nil
}

// userText is what the model reads after the skill file: three header lines
// that name the session, then the digest in a <digest> block. A closing tag
// inside the digest, in any case or spacing, is broken up as "</ digest>",
// which no end tag can be, so session text cannot end the block early and
// pass for something outside it.
func userText(in Input, date, digestText string) string {
	text := closingDigest.ReplaceAllString(strings.TrimRight(digestText, "\n"), "</ digest>")
	return fmt.Sprintf(
		"Session id: %s\nWorking directory: %s\nDate: %s\n\n<digest>\n%s\n</digest>\n",
		in.Session,
		in.Cwd,
		date,
		text,
	)
}

// build opens the transcript and digests it past the mark.
func build(path string, seen, maxBytes int) (digest.Digest, error) {
	f, err := os.Open(path)
	if err != nil {
		return digest.Digest{}, fmt.Errorf("run: %w", err)
	}
	defer f.Close()
	return digest.Build(f, seen, maxBytes)
}
