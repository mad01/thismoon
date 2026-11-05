package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/brain/internal/markdown"
	"github.com/mad01/thismoon/services/brain/internal/vault"
)

func newEditCmd(a *app) *cobra.Command {
	var (
		body, appendBody     string
		bodyFile, appendFile string
		ifMatch              string
		patch                markdown.Frontmatter
		fields               fieldFlags
	)
	c := &cobra.Command{
		Use:   "edit <note>",
		Short: "Edit body or frontmatter of an existing note",
		Long: `Edit an existing note, named by vault:path or by a vault-relative path
found in exactly one vault. A note in a shared vault is refused with exit
code 6.

Body handling (mutually exclusive):
  --body "..."          Replace the body entirely.
  --append "..."        Append to the body.
  --body -              Replace body from stdin.
  --append -            Append body from stdin.
  --body-file <path>    Replace the body with a file's contents.
  --append-file <path>  Append a file's contents to the body.

The edited note is scanned for secrets before it is written; one found
refuses the edit with exit code 8.

Frontmatter patch flags merge into the existing frontmatter (non-empty wins):
  --status, --type, --title, --source, --evidence, --tag (repeatable),
  --alias (repeatable)
--set key=value (YAML value) and --unset key then change single keys; keys
brain does not model are kept as written.

--if-match <sha> refuses the edit with exit code 7 when the note's bytes no
longer have that sha (read -o json prints it), so an edit never overwrites
a change made since the read.

At least one body or frontmatter flag must be provided. With -o json, prints
{"vault": ..., "path": ..., "sha": ...}, sha being the new content's.`,
		Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rel := args[0]
			if countSet(body, appendBody, bodyFile, appendFile) > 1 {
				return usageErrorf(
					"--body, --append, --body-file and --append-file are mutually exclusive",
				)
			}
			patch.Evidence = fields.evidence
			hasPatch := patch.Title != "" || patch.Type != "" || patch.Status != "" ||
				patch.Source != "" || patch.Evidence != "" ||
				len(patch.Tags) > 0 || len(patch.Aliases) > 0
			if countSet(body, appendBody, bodyFile, appendFile) == 0 && !hasPatch &&
				!fields.given() {
				return usageErrorf("nothing to do: set --body, --append, or a frontmatter flag")
			}
			set, unset, err := fields.parse()
			if err != nil {
				return err
			}
			opts := vault.EditOptions{Set: set, Unset: unset, IfMatch: ifMatch}
			if hasPatch {
				opts.FrontmatterPatch = &patch
			}
			if opts.Body, err = readBody(cmd.InOrStdin(), body, bodyFile); err != nil {
				return err
			}
			if opts.Append, err = readBody(cmd.InOrStdin(), appendBody, appendFile); err != nil {
				return err
			}
			_, ref, err := a.locate(rel)
			if err != nil {
				return err
			}
			sha, err := ref.Vault.Edit(ref.Abs(), opts)
			if err != nil {
				return withNoteHint(err, ref.Path)
			}
			res := vault.PathResult{Vault: ref.Vault.Name, Path: ref.Path, SHA: sha}
			return a.emit(cmd.OutOrStdout(), res, func(w io.Writer) error {
				fmt.Fprintln(w, a.show(res.Vault, res.Path))
				return nil
			})
		},
	}
	f := c.Flags()
	f.StringVar(&body, "body", "", `replace body ("-" for stdin)`)
	f.StringVar(&appendBody, "append", "", `append to body ("-" for stdin)`)
	f.StringVar(&bodyFile, "body-file", "", "replace the body with this file's contents")
	f.StringVar(&appendFile, "append-file", "", "append this file's contents to the body")
	f.StringVar(&ifMatch, "if-match", "", "refuse the edit unless the note still has this sha")
	f.StringVar(&patch.Status, "status", "", "set frontmatter status")
	f.StringVar(&patch.Type, "type", "", "set frontmatter type")
	f.StringVar(&patch.Title, "title", "", "set frontmatter title")
	f.StringVar(&patch.Source, "source", "", "set frontmatter source")
	f.StringSliceVar(&patch.Tags, "tag", nil, "set frontmatter tags (repeatable, replaces all)")
	f.StringSliceVar(
		&patch.Aliases,
		"alias",
		nil,
		"set frontmatter aliases (repeatable, replaces all)",
	)
	fields.register(c)
	return c
}

// readBody returns the body a flag pair names: the file's contents when
// file is set, else the flag value, with "-" reading stdin. A file that
// cannot be read is a usage error.
func readBody(stdin io.Reader, val, file string) (string, error) {
	if file == "" {
		s, err := readMaybeStdin(stdin, val)
		if err != nil {
			return "", fmt.Errorf("read stdin: %w", err)
		}
		return s, nil
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return "", usageErrorf("read body file: %w", err)
	}
	return string(data), nil
}

// countSet returns how many of vals are non-empty.
func countSet(vals ...string) int {
	n := 0
	for _, v := range vals {
		if v != "" {
			n++
		}
	}
	return n
}

// readMaybeStdin returns the flag value, or stdin's contents if the value is "-".
func readMaybeStdin(stdin io.Reader, val string) (string, error) {
	if val != "-" {
		return val, nil
	}
	data, err := io.ReadAll(stdin)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
