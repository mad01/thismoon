package sharing

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/mad01/thismoon/services/brain/internal/ghcli"
	"github.com/mad01/thismoon/services/brain/internal/library"
	"github.com/mad01/thismoon/services/brain/internal/vault"
)

// IssueResult is the JSON result of brain flag on a shared note: the issue
// opened, or with DryRun the issue that would be.
type IssueResult struct {
	Vault string `json:"vault"`
	Path  string `json:"path"`
	ID    string `json:"id,omitempty"`
	// Repo is the HOST/OWNER/REPO the issue goes to; empty lets gh read it
	// from the clone's remote.
	Repo   string `json:"repo,omitempty"`
	Title  string `json:"title"`
	Body   string `json:"body"`
	URL    string `json:"url,omitempty"`
	DryRun bool   `json:"dry_run"`
}

// FlagIssue opens an issue on the repo of the shared vault holding ref,
// naming the note, its id and reason: a shared note takes no direct flag.
// A dry run returns the issue without opening it.
func FlagIssue(
	ctx context.Context,
	lib *library.Library,
	ref vault.Ref,
	reason string,
	dryRun bool,
) (IssueResult, error) {
	if !ref.Vault.Shared {
		return IssueResult{}, fmt.Errorf(
			"%w: %s is in private vault %s, where flag marks it disputed directly",
			vault.ErrInvalidPath, ref, ref.Vault.Name,
		)
	}
	fm, _, err := ref.Vault.ReadFrontmatter(ref.Abs())
	if err != nil {
		return IssueResult{}, err
	}
	res := IssueResult{
		Vault: ref.Vault.Name, Path: ref.Path, ID: fm.ID, DryRun: dryRun,
		Title: "Disputed: " + titleOf(fm, ref.Path),
	}
	res.Body = issueBody(ref, fm.ID, reason)
	remote, err := issueRemote(lib, ref.Vault)
	if err != nil && !dryRun {
		return res, err
	}
	if remote != "" {
		if url, err := lib.Git(ref.Vault).RemoteURL(remote); err == nil {
			res.Repo = ghcli.RepoOf(url)
		}
	}
	if dryRun {
		return res, nil
	}
	res.URL, err = ghcli.CreateIssue(ctx, ref.Vault.Root, res.Repo, res.Title, res.Body)
	return res, err
}

// issueRemote returns the remote of the shared vault's clone an issue
// goes to: the one its branch tracks, else origin.
func issueRemote(lib *library.Library, v *vault.Vault) (string, error) {
	clone := lib.Git(v)
	if !clone.IsRepo() {
		return "", fmt.Errorf(
			"%w: shared vault %s is not a git clone, so brain cannot open an issue on its repo",
			vault.ErrReadOnly, v.Name,
		)
	}
	if remote, _, err := clone.Upstream(); err == nil {
		return remote, nil
	}
	remotes, err := clone.Remotes()
	if err != nil {
		return "", err
	}
	if !slices.Contains(remotes, "origin") {
		return "", fmt.Errorf("%w: shared vault %s has no remote to open an issue on",
			vault.ErrReadOnly, v.Name)
	}
	return "origin", nil
}

func issueBody(ref vault.Ref, id, reason string) string {
	var b strings.Builder
	b.WriteString("A reader disputes this shared note.\n\n")
	fmt.Fprintf(&b, "- Note: `%s`\n", ref)
	if id != "" {
		fmt.Fprintf(&b, "- Id: `%s`\n", id)
	}
	fmt.Fprintf(&b, "- Reason: %s\n\n", strings.TrimSpace(reason))
	fmt.Fprintf(&b, "Settle it with a pull request, for instance `brain edit %s --pr "+
		"--status disputed --set status_reason=...` or a fix to the note.\n", ref)
	b.WriteString("\nOpened by `brain flag`.\n")
	return b.String()
}

// sortedUnique returns the distinct values of in, sorted.
func sortedUnique(in []string) []string {
	out := slices.Clone(in)
	slices.Sort(out)
	return slices.Compact(out)
}
