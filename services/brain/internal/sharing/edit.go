package sharing

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/mad01/thismoon/services/brain/internal/ghcli"
	"github.com/mad01/thismoon/services/brain/internal/library"
	"github.com/mad01/thismoon/services/brain/internal/markdown"
	"github.com/mad01/thismoon/services/brain/internal/vault"
)

// EditResult is the JSON result of brain edit --pr.
type EditResult struct {
	Vault  string `json:"vault"`
	Path   string `json:"path"`
	Branch string `json:"branch"`
	PR     string `json:"pr"`
	// SHA is the edited note's sha on the branch.
	SHA string `json:"sha"`
}

// EditPR applies opts to the shared note at ref on a branch of the shared
// vault's repo and opens a pull request from it. The edit runs on the
// remote's tip in a temporary worktree, with every check a direct edit
// makes: IfMatch compares against the note as the remote has it, and a
// secret refuses the edit. An edit that changes nothing is refused.
func EditPR(
	ctx context.Context,
	lib *library.Library,
	ref vault.Ref,
	opts vault.EditOptions,
) (res EditResult, err error) {
	if !ref.Vault.Shared {
		return EditResult{}, fmt.Errorf(
			"%w: %s is in private vault %s; edit it directly, without --pr",
			vault.ErrInvalidPath, ref, ref.Vault.Name,
		)
	}
	wt, err := openWorktree(lib, ref.Vault)
	if err != nil {
		return EditResult{}, err
	}
	defer func() { err = errors.Join(err, wt.close()) }()
	abs, err := wt.vault.Resolve(ref.Path)
	if err != nil {
		return EditResult{}, err
	}
	res = EditResult{Vault: ref.Vault.Name, Path: ref.Path}
	if res.SHA, err = wt.vault.Edit(abs, opts); err != nil {
		return res, err
	}
	if dirty, err := wt.repo.Dirty(); err != nil || len(dirty) == 0 {
		return res, errors.Join(
			err,
			fmt.Errorf("%w: the edit changes nothing in %s", vault.ErrInvalidPath, ref),
		)
	}
	fm, _, err := wt.vault.ReadFrontmatter(abs)
	if err != nil {
		return res, err
	}
	if res.Branch, err = wt.freeBranch(editBranch(ref.Path, fm.ID)); err != nil {
		return res, err
	}
	msg := fmt.Sprintf("brain: %s %s", vault.VerbEdit, ref.Path)
	res.PR, err = wt.publish(ctx, res.Branch, msg, []string{ref.Path}, ghcli.PR{
		Title: "Edit note: " + titleOf(fm, ref.Path),
		Body:  editBody(ref, fm, opts),
	})
	return res, err
}

// editBranch is brain/edit/<stem>-<short id>, or brain/edit/<stem> for a
// note without an id.
func editBranch(rel, id string) string {
	stem := strings.TrimSuffix(path.Base(rel), path.Ext(rel))
	if id == "" {
		return "brain/edit/" + stem
	}
	return "brain/edit/" + stem + "-" + strings.ToLower(id[max(0, len(id)-shortIDLen):])
}

// editBody is the pull request description: the note and what the edit
// changes.
func editBody(ref vault.Ref, fm markdown.Frontmatter, opts vault.EditOptions) string {
	var b strings.Builder
	fmt.Fprintf(
		&b,
		"Edits **%s** in the shared vault %s.\n\n",
		titleOf(fm, ref.Path),
		ref.Vault.Name,
	)
	fmt.Fprintf(&b, "- Path: `%s`\n", ref.Path)
	if fm.ID != "" {
		fmt.Fprintf(&b, "- Id: `%s`\n", fm.ID)
	}
	for _, c := range editChanges(opts) {
		fmt.Fprintf(&b, "- %s\n", c)
	}
	b.WriteString("\nOpened by `brain edit --pr`.\n")
	return b.String()
}

// editChanges says in words what opts changes.
func editChanges(opts vault.EditOptions) []string {
	var out []string
	switch {
	case opts.Body != "":
		out = append(out, "Replaces the body")
	case opts.Append != "":
		out = append(out, "Appends to the body")
	}
	var keys []string
	if p := opts.FrontmatterPatch; p != nil {
		for key, set := range map[string]bool{
			"title": p.Title != "", "aliases": len(p.Aliases) > 0, "tags": len(p.Tags) > 0,
			"type": p.Type != "", "status": p.Status != "", "source": p.Source != "",
			"evidence": p.Evidence != "",
		} {
			if set {
				keys = append(keys, key)
			}
		}
	}
	for _, f := range opts.Set {
		keys = append(keys, f.Key)
	}
	if len(keys) > 0 {
		out = append(out, "Sets "+strings.Join(sortedUnique(keys), ", "))
	}
	if len(opts.Unset) > 0 {
		out = append(out, "Removes "+strings.Join(sortedUnique(opts.Unset), ", "))
	}
	return out
}
