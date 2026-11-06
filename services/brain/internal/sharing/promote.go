package sharing

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/mad01/thismoon/services/brain/internal/ghcli"
	"github.com/mad01/thismoon/services/brain/internal/library"
	"github.com/mad01/thismoon/services/brain/internal/lifecycle"
	"github.com/mad01/thismoon/services/brain/internal/markdown"
	"github.com/mad01/thismoon/services/brain/internal/vault"
)

// maxReportedFindings caps the check findings a refused promote lists.
const maxReportedFindings = 5

// shortIDLen is how many characters of the note id a branch name keeps:
// the end of the ULID, its random part.
const shortIDLen = 8

// PromoteOptions controls a promotion into a shared vault.
type PromoteOptions struct {
	// Force opens another pull request for a note that is promoting
	// through one already.
	Force bool
	// Now dates the private copy's status change; zero means time.Now.
	Now time.Time
}

// PromoteResult is the JSON result of brain promote --to.
type PromoteResult struct {
	Vault       string `json:"vault"`
	Path        string `json:"path"`
	ID          string `json:"id"`
	SharedVault string `json:"shared_vault"`
	Branch      string `json:"branch"`
	PR          string `json:"pr"`
	// Checks name what the note passed before the pull request opened.
	Checks []string `json:"checks"`
	// MOCs are the note's mocs field: the shared MOCs it should be listed
	// in, which the pull request body names and brain never edits.
	MOCs []string `json:"mocs,omitempty"`
	// Warnings are the check findings below error level.
	Warnings []string `json:"warnings,omitempty"`
}

// Promote opens a pull request that adds the private note at ref to the
// shared vault target, under the same path and with the same id, and marks
// the private copy promoting with shared_pr naming the pull request. It
// stops at the first gate that refuses (see the Gate names): a target that
// is not shared is vault.ErrReadOnly, a note that is not validated (or is
// promoting already, without Force) vault.ErrInvalidStatusChange, a path
// the shared vault has vault.ErrExists, a secret vault.ErrSecretFound. A
// classification the policy does not accept, or an error from brain check,
// refuses with a plain error. DryRun runs the same gates.
func Promote(
	ctx context.Context,
	lib *library.Library,
	ref vault.Ref,
	target *vault.Vault,
	opts PromoteOptions,
) (PromoteResult, error) {
	if opts.Now.IsZero() {
		opts.Now = time.Now()
	}
	fm, body, gates, err := noteGates(ref, target, opts.Force)
	if err != nil {
		return PromoteResult{}, err
	}
	if err := refusal(gates); err != nil {
		return PromoteResult{}, err
	}
	wt, err := openWorktree(lib, target)
	if err != nil {
		return PromoteResult{}, err
	}
	gates = append(gates, repoPassed(target, wt.remote, wt.base))
	res, err := promoteIn(ctx, lib, wt, ref, fm, body, gates)
	err = errors.Join(err, wt.close())
	if res.PR == "" {
		return res, err
	}
	if _, markErr := ref.Vault.StartPromotion(ref.Abs(), res.PR, opts.Now); markErr != nil {
		err = errors.Join(err, fmt.Errorf(
			"opened %s, but marking %s promoting failed: %w (set status: promoting and shared_pr: %s by hand)",
			res.PR,
			ref,
			markErr,
			res.PR,
		))
	}
	return res, err
}

// promotable refuses a target that is not a shared vault, or a note that
// is in one already.
func promotable(ref vault.Ref, target *vault.Vault) error {
	if !target.Shared {
		return fmt.Errorf(
			"%w: %s is not a shared vault; promote --to opens a pull request on a shared vault",
			vault.ErrReadOnly, target.Name,
		)
	}
	if ref.Vault.Shared {
		return fmt.Errorf(
			"%w: %s is in shared vault %s already",
			vault.ErrReadOnly,
			ref,
			ref.Vault.Name,
		)
	}
	return nil
}

// ready refuses a note that is not validated, one promoting through a pull
// request already (unless force), and one without an id.
func ready(ref vault.Ref, fm markdown.Frontmatter, force bool) error {
	status := vault.StatusOf(ref.Path, fm)
	f := lifecycle.Read(fm)
	switch {
	case status == lifecycle.Promoting && f.SharedPR != "" && !force:
		return fmt.Errorf(
			"%w: %s is promoting through %s already; brain sync settles it once that merges "+
				"or closes, and --force opens another pull request",
			vault.ErrInvalidStatusChange, ref, f.SharedPR,
		)
	case status != lifecycle.Validated && status != lifecycle.Promoting:
		return fmt.Errorf(
			"%w: %s is %s; only a validated note goes to a shared vault "+
				"(verify it, then brain edit %s --status validated)",
			vault.ErrInvalidStatusChange, ref, status, ref,
		)
	case fm.ID == "":
		return fmt.Errorf(
			"%w: %s has no id, which its shared copy keeps (give it one with: brain migrate --ids)",
			vault.ErrInvalidStatusChange, ref,
		)
	}
	return nil
}

// promoteIn runs the gates on the shared copy in the worktree, after the
// gates already passed, and publishes the copy as a pull request.
func promoteIn(
	ctx context.Context,
	lib *library.Library,
	wt *worktree,
	ref vault.Ref,
	fm markdown.Frontmatter,
	body []byte,
	gates []Gate,
) (PromoteResult, error) {
	res := PromoteResult{
		Vault: ref.Vault.Name, Path: ref.Path, ID: fm.ID, SharedVault: wt.shared.Name,
		MOCs: lifecycle.Read(fm).MOCs,
	}
	copied, warnings, err := copyGates(lib, wt.vault, wt.shared, ref, fm, body)
	if err != nil {
		return res, err
	}
	if err := refusal(copied); err != nil {
		return res, fmt.Errorf("%s: %w; nothing was pushed", ref, err)
	}
	res.Warnings = warnings
	res.Checks = details(append(gates, copied...))
	branch, err := wt.freeBranch(promoteBranch(ref.Path, fm.ID))
	if err != nil {
		return res, err
	}
	msg := fmt.Sprintf("brain: %s %s\n\nid: %s", vault.VerbPromote, ref.Path, fm.ID)
	url, err := wt.publish(ctx, branch, msg, []string{ref.Path}, ghcli.PR{
		Title: "Promote note: " + titleOf(fm, ref.Path),
		Body:  promoteBody(ref, fm, wt.shared.Name, res),
	})
	if err != nil {
		return res, err
	}
	res.Branch, res.PR = branch, url
	return res, nil
}

// promoteBranch is brain/promote/<stem>-<short id>.
func promoteBranch(rel, id string) string {
	stem := strings.TrimSuffix(path.Base(rel), path.Ext(rel))
	short := strings.ToLower(id[max(0, len(id)-shortIDLen):])
	return "brain/promote/" + stem + "-" + short
}

func titleOf(fm markdown.Frontmatter, rel string) string {
	if fm.Title != "" {
		return fm.Title
	}
	return rel
}

// promoteBody is the pull request description: the note, its id and
// sources, the MOCs it belongs in, and the checks it passed.
func promoteBody(ref vault.Ref, fm markdown.Frontmatter, shared string, res PromoteResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Adds **%s** to the shared vault %s, promoted from a private vault.\n\n",
		titleOf(fm, ref.Path), shared)
	fmt.Fprintf(&b, "- Path: `%s`\n- Id: `%s`\n", ref.Path, fm.ID)
	sources := sourceList(fm)
	if len(sources) == 0 {
		b.WriteString("- Sources: none listed\n")
	} else {
		b.WriteString("- Sources:\n")
		for _, s := range sources {
			fmt.Fprintf(&b, "  - %s\n", s)
		}
	}
	if len(res.MOCs) > 0 {
		fmt.Fprintf(&b, "- MOCs it should appear in: %s. brain never edits a shared MOC; "+
			"add the links in this pull request or a follow-up.\n", strings.Join(res.MOCs, ", "))
	}
	b.WriteString("- Checks passed before this pull request opened:\n")
	for _, c := range res.Checks {
		fmt.Fprintf(&b, "  - %s\n", c)
	}
	fmt.Fprintf(&b, "\nOpened by `brain promote --to %s`. The private copy stays as promoting "+
		"until this merges or closes; `brain sync` settles it.\n", shared)
	return b.String()
}

// sourceList returns the note's source and each sources entry, as a URL
// or a ref.
func sourceList(fm markdown.Frontmatter) []string {
	var out []string
	if fm.Source != "" {
		out = append(out, fm.Source)
	}
	for _, s := range lifecycle.Read(fm).Sources {
		switch {
		case s.URL != "":
			out = append(out, s.URL)
		case s.Ref != "":
			out = append(out, "`"+s.Ref+"`")
		}
	}
	return out
}
