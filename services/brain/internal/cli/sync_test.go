package cli

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/brain/internal/sharing"
	"github.com/mad01/thismoon/services/brain/internal/testgit"
)

// mergeOnRemote merges branch into main on the remote, the way a reviewer
// merging the pull request would.
func mergeOnRemote(t *testing.T, remote, branch string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "merger")
	testgit.Clone(t, remote, dir)
	testgit.Run(t, dir, "merge", "-q", "--no-ff", "-m", "Merge "+branch, "origin/"+branch)
	testgit.Run(t, dir, "push", "-q", "origin", "HEAD:main")
}

func syncReport(t *testing.T, wantCode int, args ...string) sharing.SyncReport {
	t.Helper()
	got := runBrain(t, "", append([]string{"sync", "-o", "json"}, args...)...)
	if got.code != wantCode {
		t.Fatalf(
			"sync: exit %d, want %d; stderr %q\n%s",
			got.code,
			wantCode,
			got.stderr,
			got.stdout,
		)
	}
	return decodeStrict[sharing.SyncReport](t, got.stdout)
}

func vaultSync(t *testing.T, r sharing.SyncReport, name string) sharing.VaultSync {
	t.Helper()
	for _, v := range r.Vaults {
		if v.Vault == name {
			return v
		}
	}
	t.Fatalf("no vault %s in %+v", name, r)
	return sharing.VaultSync{}
}

func TestSyncRemovesThePrivateCopyOnceMerged(t *testing.T) {
	env := setupSharing(t, false)
	writeNote(t, env.private, "notes/uses-rollout.md",
		"---\ntitle: Uses Rollout\n---\nfollows [[rollout]]\n")
	testgit.Commit(t, env.private, "link", nil)
	if got := runBrain(t, "", "promote", "notes/rollout.md", "--to", "team"); got.code != exitOK {
		t.Fatalf("promote --to: exit %d, stderr %q", got.code, got.stderr)
	}

	env.gh.PRState("7", "OPEN")
	open := syncReport(t, exitOK)
	if p := vaultSync(t, open, "personal").Promotions; len(p) != 1 ||
		p[0].Action != sharing.ActionWaiting {
		t.Fatalf("open pull request: promotions = %+v", p)
	}

	mergeOnRemote(t, env.sharedRemote, "brain/promote/rollout-0000rq5t")
	env.gh.PRState("7", "MERGED")
	r := syncReport(t, exitOK)
	personal := vaultSync(t, r, "personal")
	if personal.State != sharing.StateLocalOnly ||
		personal.Message != "local-only: no remote, so nothing is pulled or pushed" {
		t.Errorf("personal = %+v", personal)
	}
	if len(personal.Promotions) != 1 || personal.Promotions[0].Action != sharing.ActionRemoved {
		t.Fatalf("promotions = %+v", personal.Promotions)
	}
	if team := vaultSync(t, r, "team"); team.State != sharing.StateSynced || team.Pulled != 2 {
		t.Errorf("team = %+v", team)
	}
	if _, err := os.Stat(filepath.Join(env.private, "notes", "rollout.md")); !os.IsNotExist(err) {
		t.Errorf("private copy still there: %v", err)
	}
	if s := testgit.Subjects(t, env.private); s[0] != "brain: remove notes/rollout.md" {
		t.Errorf("private commits = %q", s)
	}

	read := runBrain(t, "", "read", "id:"+rolloutID, "-o", "json")
	if read.code != exitOK || !strings.Contains(read.stdout, `"vault": "team"`) {
		t.Errorf("id ref after the merge: exit %d\n%s", read.code, read.stdout)
	}
	links := runBrain(t, "", "backlinks", "personal:notes/uses-rollout.md", "-o", "json")
	if !strings.Contains(links.stdout, `"vault": "team"`) ||
		strings.Contains(links.stdout, `"dangling": true`) {
		t.Errorf("[[rollout]] does not resolve to the shared copy:\n%s", links.stdout)
	}

	text := runBrain(t, "", "sync")
	if text.code != exitOK ||
		!strings.Contains(
			text.stdout,
			"personal: local-only: no remote, so nothing is pulled or pushed\n",
		) {
		t.Errorf("sync text: exit %d\n%s", text.code, text.stdout)
	}
}

func TestSyncSettlesClosedAndKeepsOpen(t *testing.T) {
	env := setupSharing(t, false)
	for rel, pr := range map[string]string{"notes/closed.md": "8", "notes/open.md": "9"} {
		writeNote(t, env.private, rel, "---\ntitle: "+rel+"\nstatus: promoting\n"+
			"shared_pr: https://github.com/example-org/brain-shared/pull/"+pr+"\n---\nbody\n")
	}
	testgit.Commit(t, env.private, "promoting", nil)
	env.gh.PRState("8", "CLOSED")
	env.gh.PRState("9", "OPEN")

	r := syncReport(t, exitOK, "--vault", "personal")
	actions := map[string]string{}
	for _, p := range vaultSync(t, r, "personal").Promotions {
		actions[p.Path] = p.Action
	}
	want := map[string]string{
		"notes/closed.md": sharing.ActionValidated,
		"notes/open.md":   sharing.ActionWaiting,
	}
	if !mapsEqual(actions, want) {
		t.Errorf("actions = %v, want %v", actions, want)
	}
	if len(r.Vaults) != 1 {
		t.Errorf("--vault personal synced %d vaults", len(r.Vaults))
	}
	fm := noteFM(t, env.private, "notes/closed.md")
	if fm.Status != "validated" || extraOf(t, fm)["shared_pr"] != nil {
		t.Errorf("closed note = %q %v", fm.Status, extraOf(t, fm))
	}
	if s := testgit.Subjects(t, env.private); s[0] != "brain: unshare notes/closed.md" {
		t.Errorf("private commits = %q", s)
	}
	if noteFM(t, env.private, "notes/open.md").Status != "promoting" {
		t.Error("an open pull request's note changed")
	}

	// gh cannot answer for a pull request: the note stays, sync exits 1.
	writeNote(t, env.private, "notes/unknown.md", "---\ntitle: Unknown\nstatus: promoting\n"+
		"shared_pr: https://github.com/example-org/brain-shared/pull/10\n---\nbody\n")
	r = syncReport(t, exitError, "--vault", "personal")
	failed := false
	for _, p := range vaultSync(t, r, "personal").Promotions {
		failed = failed || (p.Path == "notes/unknown.md" && p.Action == sharing.ActionFailed)
	}
	if !failed || noteFM(t, env.private, "notes/unknown.md").Status != "promoting" {
		t.Errorf("unreadable pull request: %+v", r)
	}
}

func TestSyncPullsPushesAndRefuses(t *testing.T) {
	env := setupSharing(t, true)
	if got := runBrain(t, "", "write", "--tier", "notes", "--title", "Fresh", "--body", "x"); got.code != exitOK {
		t.Fatalf("write: exit %d, stderr %q", got.code, got.stderr)
	}
	r := syncReport(t, exitOK)
	personal := vaultSync(t, r, "personal")
	if personal.State != sharing.StateSynced || personal.Pushed != 1 ||
		!strings.HasSuffix(personal.Message, "pushed 1 commit(s)") {
		t.Fatalf("personal = %+v", personal)
	}
	remote := testgit.Run(t, env.private, "remote", "get-url", "origin")
	if got := testgit.Run(t, remote, "log", "-1", "--format=%s", "main"); got != "brain: write notes/fresh.md" {
		t.Errorf("remote head = %q", got)
	}

	// A hand edit to a tracked note: refused, and the edit stays.
	writeNote(t, env.private, "notes/fresh.md", "edited by hand\n")
	r = syncReport(t, exitError)
	if p := vaultSync(t, r, "personal"); p.State != sharing.StateRefused ||
		!strings.Contains(
			p.Message,
			"notes/fresh.md",
		) || !strings.Contains(p.Message, "never stashes") {
		t.Errorf("dirty personal = %+v", p)
	}
	if data, _ := os.ReadFile(filepath.Join(env.private, "notes", "fresh.md")); string(
		data,
	) != "edited by hand\n" {
		t.Errorf("the hand edit was touched: %q", data)
	}
	testgit.Run(t, env.private, "checkout", "--", "notes/fresh.md")

	// Diverged: the remote and the vault each have a commit the other lacks.
	other := filepath.Join(t.TempDir(), "other")
	testgit.Clone(t, remote, other)
	testgit.Commit(t, other, "elsewhere", map[string]string{"notes/elsewhere.md": "x\n"})
	testgit.Run(t, other, "push", "-q", "origin", "HEAD:main")
	if got := runBrain(t, "", "write", "--tier", "notes", "--title", "Local", "--body", "x"); got.code != exitOK {
		t.Fatalf("write: exit %d, stderr %q", got.code, got.stderr)
	}
	before := testgit.Run(t, env.private, "rev-parse", "HEAD")
	r = syncReport(t, exitError)
	if p := vaultSync(t, r, "personal"); p.State != sharing.StateRefused ||
		!strings.Contains(p.Message, "diverged") {
		t.Errorf("diverged personal = %+v", p)
	}
	if after := testgit.Run(t, env.private, "rev-parse", "HEAD"); after != before {
		t.Error("a diverged vault's HEAD moved")
	}
	if team := vaultSync(t, r, "team"); team.State != sharing.StateSynced {
		t.Errorf("team = %+v", team)
	}
}

func TestSyncPlainVault(t *testing.T) {
	setupCLI(t, "")
	r := syncReport(t, exitOK)
	if len(r.Vaults) != 1 || r.Vaults[0].State != sharing.StateNotGit ||
		!slices.Contains([]string{"not a git repo: nothing to pull or push"}, r.Vaults[0].Message) {
		t.Errorf("report = %+v", r)
	}
}
