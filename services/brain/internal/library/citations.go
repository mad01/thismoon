package library

import (
	"bytes"
	"fmt"
	"os"
	"time"

	"github.com/mad01/thismoon/services/brain/internal/lifecycle"
	"github.com/mad01/thismoon/services/brain/internal/vault"
)

// CitationWarnings returns one warning per wikilink in the note at ref that
// resolves to a note that is not validated, or one past its review_by: what
// write and edit tell the writer about the citations they just made. Links
// resolve across every vault. A note without a wikilink costs no vault
// walk. A failure to check is itself the one warning, since the write it
// follows already succeeded.
func (l *Library) CitationWarnings(ref vault.Ref, now time.Time) []string {
	warnings, err := l.citationWarnings(ref, now)
	if err != nil {
		return []string{fmt.Sprintf("could not check the links in %s: %v", ref, err)}
	}
	return warnings
}

func (l *Library) citationWarnings(ref vault.Ref, now time.Time) ([]string, error) {
	data, err := os.ReadFile(ref.Abs())
	if err != nil {
		return nil, err
	}
	if !bytes.Contains(data, []byte("[[")) {
		return nil, nil
	}
	sn, err := l.set.Snapshot()
	if err != nil {
		return nil, err
	}
	note, err := sn.Note(ref)
	if err != nil {
		return nil, err
	}
	var warnings []string
	for _, c := range sn.Citations(note) {
		if c.To == nil || c.To == note {
			continue
		}
		standing := lifecycle.Standing(c.To.Status(), lifecycle.Read(c.To.Frontmatter), now)
		if standing == "" {
			continue
		}
		warnings = append(warnings, fmt.Sprintf(
			"line %d links [[%s]] (%s), which is %s", c.Line, c.Target, c.To.Ref, standing,
		))
	}
	return warnings, nil
}
