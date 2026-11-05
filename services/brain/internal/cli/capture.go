package cli

import (
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/mad01/thismoon/services/brain/internal/lifecycle"
	"github.com/mad01/thismoon/services/brain/internal/markdown"
	"github.com/mad01/thismoon/services/brain/internal/vault"
)

// capturedByUser is the captured_by value of a capture a person makes.
const capturedByUser = "user"

// maxDerivedTitle caps, in characters, a title capture takes from the body.
const maxDerivedTitle = 80

func newCaptureCmd(a *app) *cobra.Command {
	var (
		opts     vault.WriteOptions
		body     string
		bodyFile string
		by       string
		distinct []string
	)
	c := &cobra.Command{
		Use:   "capture",
		Short: "Capture a raw note into the inbox",
		Long: `Capture material into inbox/ for later verification: the quick write for an
agent or a person. The note gets status raw, captured_by (--by: user, the
default, or agent:<session> for an agent) and expires_at 30 days out;
brain inbox keep extends that. --source <url> records where it came from and
makes the note type source. The body comes from --body, --body-file or
stdin. Without --title, the first line of the body (or the source URL)
becomes the title.

capture runs write's checks: a secret refuses with exit code 8, a likely
duplicate with exit code 9 (--distinct-from <note> lets it through), a
taken filename with 4, a shared vault with 6. With -o json, prints {"vault",
"path", "warnings"}.`,
		Args: exactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !validCapturer(by) {
				return usageErrorf("--by must be user or agent:<session>, got %q", by)
			}
			if bodyFile != "" && cmd.Flags().Changed("body") {
				return usageErrorf("--body and --body-file are mutually exclusive")
			}
			var err error
			if opts.Body, err = readBody(cmd.InOrStdin(), body, bodyFile); err != nil {
				return err
			}
			if opts.Title = captureTitle(opts.Title, opts.Body, opts.Source); opts.Title == "" {
				return usageErrorf("give --title, a body or --source to title the capture")
			}
			opts.Tier, opts.Status, opts.Verb = vault.TierInbox, lifecycle.Raw, vault.VerbCapture
			if opts.Source != "" {
				opts.Type = "source"
			}
			opts.Set = []markdown.Field{capturedBy(by)}
			return a.createNote(cmd, opts, distinct)
		},
	}
	f := c.Flags()
	f.StringVar(&opts.Title, "title", "", "note title; defaults to the body's first line")
	f.StringVar(&body, "body", "-", `markdown body; "-" reads stdin`)
	f.StringVar(&bodyFile, "body-file", "", "read the markdown body from this file")
	f.StringVar(
		&opts.Source,
		"source",
		"",
		"where the material came from; makes the note type source",
	)
	f.StringVar(&by, "by", capturedByUser, "who captured it: user, or agent:<session>")
	f.StringSliceVar(&opts.Tags, "tag", nil, "frontmatter tag (repeatable)")
	f.StringArrayVar(&distinct, "distinct-from", nil,
		"a note the capture is distinct from, though alike (repeatable)")
	return c
}

// validCapturer reports whether by is a captured_by value: user, or
// agent:<session> with a session.
func validCapturer(by string) bool {
	session, agent := strings.CutPrefix(by, "agent:")
	return by == capturedByUser || (agent && strings.TrimSpace(session) != "")
}

func capturedBy(by string) markdown.Field {
	node := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: by}
	return markdown.Field{Key: lifecycle.KeyCapturedBy, Value: node}
}

// captureTitle returns the title a capture gets: the one given, else the
// body's first non-empty line without heading marks, cut to
// maxDerivedTitle characters, else the source.
func captureTitle(title, body, source string) string {
	if t := strings.TrimSpace(title); t != "" {
		return t
	}
	for line := range strings.SplitSeq(body, "\n") {
		line = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(line), "#"))
		if line == "" {
			continue
		}
		if r := []rune(line); len(r) > maxDerivedTitle {
			line = strings.TrimSpace(string(r[:maxDerivedTitle]))
		}
		return line
	}
	return strings.TrimSpace(source)
}
