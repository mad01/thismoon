package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/brain/internal/check"
	"github.com/mad01/thismoon/services/brain/internal/library"
	"github.com/mad01/thismoon/services/brain/internal/vault"
)

// outputGitHub is the -o value only brain check takes: one GitHub Actions
// workflow command per finding, which the run page shows as an annotation.
const outputGitHub = "github"

func newCheckCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "check [paths...]",
		Short: "Check notes against the lifecycle model and the vault policy",
		Long: `Check notes for problems a reviewer would otherwise catch by hand: frontmatter
that breaks the status rules, a source note without a source, links that
resolve to nothing, ids and filename stems shared within a vault, a
classification the vault's .brain/policy.yaml does not allow, secrets, NUL
bytes and notes that are symbolic links.
A link from a checked note to a retracted or withdrawn note is an error; a
link to any other note that is not validated, or one past its review_by, is
a warning, and so is a link from an unchecked note to a checked one that is
not validated.

Each argument is a file path, a vault:path or id:<ulid> ref, or a bare path
found in one vault; with none, every note of the --vault vaults is checked.
A path that names no note (a file deleted in the change under review) is
skipped with a stderr line. Links resolve across every configured vault.

The vault's .brain/policy.yaml sets the allowed classifications and the
level of each rule (error, warning or off); without the file every missing
optional field is a warning:

  classifications: [public, internal]
  rules:
    provenance: error
    missing-field: off

Rules: schema, status-unknown, status-fields, missing-field, provenance,
link-dangling, link-revoked, link-unvalidated, id-duplicate, stem-duplicate,
classification, secret, nul-byte, symlink. -o text prints one line per
finding, -o json the report {"ok", "checked", "errors", "warnings",
"findings", "skipped"}, -o github one workflow annotation per finding.
Exits 0 when no finding is an error, 1 when one is, 2 on a usage error.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			lib, r, err := a.runCheck(cmd, args)
			if err != nil {
				return err
			}
			if err := a.writeCheck(cmd.OutOrStdout(), lib.Set(), r); err != nil {
				return err
			}
			if !r.OK {
				return &reportedError{
					err: fmt.Errorf("%d error(s) in %d note(s) checked", r.Errors, r.Checked),
				}
			}
			return nil
		},
	}
}

// runCheck resolves the arguments and runs the check.
func (a *app) runCheck(cmd *cobra.Command, args []string) (*library.Library, check.Report, error) {
	lib, scope, err := a.openVaults()
	if err != nil {
		return nil, check.Report{}, err
	}
	opts := check.Options{Set: lib.Set(), Scope: scope}
	var skipped []check.Skipped
	for _, arg := range args {
		ref, reason, err := a.checkTarget(lib, scope, arg)
		if err != nil {
			return nil, check.Report{}, err
		}
		if reason != "" {
			skipped = append(skipped, check.Skipped{Path: arg, Reason: reason})
			fmt.Fprintf(cmd.ErrOrStderr(), "%s: skipped %s: %s\n", cmd.CommandPath(), arg, reason)
			continue
		}
		opts.Targets = append(opts.Targets, ref)
	}
	if len(args) > 0 && len(opts.Targets) == 0 {
		return lib, check.Report{OK: true, Findings: []check.Finding{}, Skipped: skipped}, nil
	}
	r, err := check.Run(opts)
	r.Skipped = skipped
	return lib, r, err
}

// checkTarget resolves one argument to a note. A file on disk wins over a
// ref of the same spelling, so a CI job in the vault's checkout can pass
// the paths a diff names. It returns a skip reason, not an error, for an
// argument that names no note.
func (a *app) checkTarget(
	lib *library.Library,
	scope *vault.Set,
	arg string,
) (vault.Ref, string, error) {
	if info, err := os.Stat(arg); err == nil {
		switch {
		case info.IsDir():
			return vault.Ref{}, "a directory; run brain check without arguments to check every note", nil
		case !strings.EqualFold(filepath.Ext(arg), ".md"):
			return vault.Ref{}, "not a markdown note", nil
		}
		ref, ok := fileRef(scope, arg)
		if !ok {
			return vault.Ref{}, "outside the checked vaults", nil
		}
		return ref, "", nil
	}
	ref, err := lib.Locate(a.vaults, arg)
	switch {
	case errors.Is(err, vault.ErrNotFound):
		return vault.Ref{}, "no such note", nil
	case err != nil:
		return vault.Ref{}, "", withNoteHint(err, refPath(arg))
	}
	return ref, "", nil
}

// fileRef finds the vault of set that holds the file at path.
func fileRef(set *vault.Set, path string) (vault.Ref, bool) {
	abs, err := realPath(path)
	if err != nil {
		return vault.Ref{}, false
	}
	for _, v := range set.Vaults() {
		root, err := realPath(v.Root)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(root, abs)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		return vault.Ref{Vault: v, Path: filepath.ToSlash(rel)}, true
	}
	return vault.Ref{}, false
}

// realPath is the absolute path with symlinks resolved, so a vault under a
// symlinked directory still contains its files.
func realPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(abs)
}

func (a *app) writeCheck(w io.Writer, set *vault.Set, r check.Report) error {
	switch a.output {
	case outputJSON:
		return writeJSON(w, r)
	case outputGitHub:
		writeCheckGitHub(w, set, r)
	default:
		a.writeCheckText(w, r)
	}
	return nil
}

func (a *app) writeCheckText(w io.Writer, r check.Report) {
	for _, f := range r.Findings {
		where := a.show(f.Vault, f.Path)
		if f.Line > 0 {
			where = fmt.Sprintf("%s:%d", where, f.Line)
		}
		fmt.Fprintf(w, "%s: %s [%s] %s\n", where, f.Level, f.Rule, f.Message)
	}
	fmt.Fprintf(
		w,
		"checked %d note(s): %d error(s), %d warning(s)\n",
		r.Checked,
		r.Errors,
		r.Warnings,
	)
}

// writeCheckGitHub prints each finding as a workflow command. The file is
// relative to the working directory when the note is under it, which is the
// repository root in a CI job, and vault-relative otherwise.
func writeCheckGitHub(w io.Writer, set *vault.Set, r check.Report) {
	roots := map[string]string{}
	for _, v := range set.Vaults() {
		roots[v.Name] = v.Root
	}
	cwd, _ := os.Getwd()
	for _, f := range r.Findings {
		props := "file=" + escapeProperty(annotationFile(cwd, roots[f.Vault], f.Path))
		if f.Line > 0 {
			props += fmt.Sprintf(",line=%d", f.Line)
		}
		props += ",title=" + escapeProperty("brain check "+f.Rule)
		fmt.Fprintf(w, "::%s %s::%s\n", f.Level, props, escapeData(f.Message))
	}
	fmt.Fprintf(
		w,
		"checked %d note(s): %d error(s), %d warning(s)\n",
		r.Checked,
		r.Errors,
		r.Warnings,
	)
}

func annotationFile(cwd, root, rel string) string {
	if cwd == "" || root == "" {
		return rel
	}
	abs := filepath.Join(root, filepath.FromSlash(rel))
	realCwd, err1 := filepath.EvalSymlinks(cwd)
	realRoot, err2 := filepath.EvalSymlinks(root)
	if err1 == nil && err2 == nil {
		abs = filepath.Join(realRoot, filepath.FromSlash(rel))
		cwd = realCwd
	}
	out, err := filepath.Rel(cwd, abs)
	if err != nil || out == ".." || strings.HasPrefix(out, ".."+string(filepath.Separator)) {
		return rel
	}
	return filepath.ToSlash(out)
}

// escapeData escapes a workflow command's message the way the Actions
// toolkit does.
func escapeData(s string) string {
	return strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A").Replace(s)
}

// escapeProperty escapes a workflow command property value.
func escapeProperty(s string) string {
	return strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A", ":", "%3A", ",", "%2C").
		Replace(s)
}
