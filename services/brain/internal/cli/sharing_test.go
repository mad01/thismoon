package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/brain/internal/markdown"
	"github.com/mad01/thismoon/services/brain/internal/sharing"
	"github.com/mad01/thismoon/services/brain/internal/testgh"
	"github.com/mad01/thismoon/services/brain/internal/testgit"
	"github.com/mad01/thismoon/services/brain/internal/testhome"
)

const (
	rolloutID = "01JC000000000000000000RQ5T"
	prURL     = "https://github.com/example-org/brain-shared/pull/7"
	issueURL  = "https://github.com/example-org/brain-shared/issues/3"
)

// sharedNotes seed the shared vault's remote.
var sharedNotes = map[string]string{
	".brain/policy.yaml": "classifications: [public, internal]\n",
	"notes/team-guide.md": "---\ntitle: Team Guide\nid: 01JC0000000000000000000GD1\n" +
		"classification: internal\nreview_by: 2027-11-01\n---\nThe team guide.\n",
}

// privateNotes seed the private vault.
var privateNotes = map[string]string{
	"notes/rollout.md": "---\ntitle: Rollout Plan\nid: " + rolloutID + "\nstatus: validated\n" +
		"classification: internal\nreview_by: 2027-11-01\nsteward: alex\n" +
		"status_reason: checked twice\nsources:\n  - url: https://example.com/rollout\n" +
		"mocs: [MOCs/operations.md]\n---\nRoll out region by region, see [[team-guide]].\n",
	"notes/secret-plan.md": "---\ntitle: Secret Plan\nid: 01JC00000000000000000005EC\n" +
		"status: validated\nclassification: confidential\nreview_by: 2027-11-01\n---\n" +
		"Only for us.\n",
	"inbox/idea.md": "---\ntitle: Idea\nid: 01JC0000000000000000001DEA\nstatus: raw\n---\nmaybe\n",
}

// sharingEnv is a private vault and a shared clone of a bare remote, with
// a fake gh on PATH.
type sharingEnv struct {
	private, shared, sharedRemote string
	gh                            *testgh.Fake
}

// setupSharing isolates brain and git, seeds the shared remote and clones
// it, makes the private vault a git repo (with a bare remote when
// privateRemote is set), and writes the config naming both.
func setupSharing(t *testing.T, privateRemote bool) sharingEnv {
	t.Helper()
	home := testhome.Isolate(t)
	testgit.Isolate(t)
	env := sharingEnv{sharedRemote: testgit.Bare(t)}
	env.gh = testgh.Install(t, prURL, issueURL)

	seed := filepath.Join(t.TempDir(), "seed")
	testgit.Clone(t, env.sharedRemote, seed)
	testgit.Commit(t, seed, "seed", sharedNotes)
	testgit.Run(t, seed, "push", "-q", "origin", "HEAD:main")
	env.shared = filepath.Join(t.TempDir(), "shared")
	testgit.Clone(t, env.sharedRemote, env.shared)

	if privateRemote {
		bare := testgit.Bare(t)
		env.private = filepath.Join(t.TempDir(), "private")
		testgit.Clone(t, bare, env.private)
		testgit.Commit(t, env.private, "seed", privateNotes)
		testgit.Run(t, env.private, "push", "-q", "-u", "origin", "HEAD:main")
	} else {
		env.private = t.TempDir()
		testgit.Init(t, env.private)
		testgit.Commit(t, env.private, "seed", privateNotes)
	}

	cfg := "vaults:\n" +
		"  - {name: personal, path: " + env.private + "}\n" +
		"  - {name: team, path: " + env.shared + ", shared: true}\n"
	cfgDir := filepath.Join(home, ".config", "brain")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	return env
}

// noteFM reads the frontmatter of the note at rel under root.
func noteFM(t *testing.T, root, rel string) markdown.Frontmatter {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	fm, _, err := markdown.ParseFrontmatter(data)
	if err != nil {
		t.Fatal(err)
	}
	return fm
}

func extraOf(t *testing.T, fm markdown.Frontmatter) map[string]any {
	t.Helper()
	extra, err := fm.ExtraValues()
	if err != nil {
		t.Fatal(err)
	}
	return extra
}

// assertWorktreesGone fails when a temporary worktree outlived its command.
func assertWorktreesGone(t *testing.T, env sharingEnv) {
	t.Helper()
	if n := len(strings.Split(testgit.Run(t, env.shared, "worktree", "list"), "\n")); n != 1 {
		t.Errorf("shared clone has %d worktrees, want its own alone", n)
	}
	if status := testgit.Status(t, env.shared); status != "" {
		t.Errorf("the shared clone's checkout changed: %q", status)
	}
	root := filepath.Join(os.Getenv("HOME"), ".local", "state", "brain", "worktrees")
	if entries, _ := os.ReadDir(root); len(entries) > 0 {
		t.Errorf("worktree dirs left under the state dir: %v", entries)
	}
}

func TestPromoteToOpensAPullRequest(t *testing.T) {
	env := setupSharing(t, false)
	got := runBrain(t, "", "promote", "notes/rollout.md", "--to", "team", "-o", "json")
	if got.code != exitOK {
		t.Fatalf("promote --to: exit %d, stderr %q", got.code, got.stderr)
	}
	res := decodeStrict[sharing.PromoteResult](t, got.stdout)
	branch := "brain/promote/rollout-0000rq5t"
	if res.PR != prURL || res.Branch != branch || res.ID != rolloutID || res.SharedVault != "team" {
		t.Errorf("result = %+v", res)
	}

	// The branch on the remote adds the note, private-only fields gone.
	if subject := testgit.Run(t, env.sharedRemote, "log", "-1", "--format=%s", branch); subject != "brain: promote notes/rollout.md" {
		t.Errorf("branch head = %q", subject)
	}
	shared := testgit.Run(t, env.sharedRemote, "show", branch+":notes/rollout.md")
	for _, want := range []string{"id: " + rolloutID, "steward: alex", "status: validated", "mocs:"} {
		if !strings.Contains(shared, want) {
			t.Errorf("shared copy lacks %q:\n%s", want, shared)
		}
	}
	for _, gone := range []string{"status_reason", "shared_pr"} {
		if strings.Contains(shared, gone) {
			t.Errorf("shared copy keeps the private-only %s:\n%s", gone, shared)
		}
	}

	// The private copy is promoting, committed as a share.
	fm := noteFM(t, env.private, "notes/rollout.md")
	if fm.Status != "promoting" || extraOf(t, fm)["shared_pr"] != prURL {
		t.Errorf("private copy = status %q, extra %v", fm.Status, extraOf(t, fm))
	}
	if s := testgit.Subjects(t, env.private); s[0] != "brain: share notes/rollout.md" {
		t.Errorf("private commits = %q", s)
	}

	calls := env.gh.Calls()
	if len(calls) != 1 || calls[0][0] != "pr" || calls[0][1] != "create" {
		t.Fatalf("gh calls = %q", calls)
	}
	pr := calls[0]
	if testgh.Flag(pr, "--head") != branch || testgh.Flag(pr, "--base") != "main" ||
		testgh.Flag(pr, "--title") != "Promote note: Rollout Plan" {
		t.Errorf("pr create args = %q", pr)
	}
	body := testgh.Flag(pr, "--body")
	for _, want := range []string{
		rolloutID, "https://example.com/rollout", "MOCs/operations.md",
		"brain check against team's policy: no errors", "secret scan",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("pr body lacks %q:\n%s", want, body)
		}
	}
	assertWorktreesGone(t, env)

	again := runBrain(t, "", "promote", "notes/rollout.md", "--to", "team")
	if again.code != exitUsage || !strings.Contains(again.stderr, "--force") {
		t.Errorf("second promote: exit %d, stderr %q", again.code, again.stderr)
	}
	forced := runBrain(
		t,
		"",
		"promote",
		"notes/rollout.md",
		"--to",
		"team",
		"--force",
		"-o",
		"json",
	)
	if forced.code != exitOK {
		t.Fatalf("promote --force: exit %d, stderr %q", forced.code, forced.stderr)
	}
	if res := decodeStrict[sharing.PromoteResult](t, forced.stdout); res.Branch != branch+"-2" {
		t.Errorf("forced branch = %q, want %s-2", res.Branch, branch)
	}
}

func TestPromoteToRefusals(t *testing.T) {
	env := setupSharing(t, false)
	cases := []struct {
		name string
		args []string
		code int
		want string
	}{
		{
			"not validated",
			[]string{"inbox/idea.md", "--to", "team"},
			exitUsage,
			"only a validated note",
		},
		{
			"target not shared",
			[]string{"notes/rollout.md", "--to", "personal"},
			exitReadOnly,
			"not a shared vault",
		},
		{
			"policy",
			[]string{"notes/secret-plan.md", "--to", "team"},
			exitError,
			`classification "confidential" is not allowed`,
		},
		{
			"with --moc",
			[]string{"notes/rollout.md", "--to", "team", "--moc", "MOCs/x.md"},
			exitUsage,
			"never edits a shared MOC",
		},
		{"unknown vault", []string{"notes/rollout.md", "--to", "nope"}, exitUsage, "unknown vault"},
	}
	for _, tc := range cases {
		got := runBrain(t, "", append([]string{"promote"}, tc.args...)...)
		if got.code != tc.code || !strings.Contains(got.stderr, tc.want) {
			t.Errorf("%s: exit %d, stderr %q; want exit %d and %q",
				tc.name, got.code, got.stderr, tc.code, tc.want)
		}
	}

	writeNote(t, env.private, "notes/leak.md", "---\ntitle: Leak\nid: 01JC0000000000000000007EAK\n"+
		"status: validated\nclassification: internal\nreview_by: 2027-11-01\n---\n"+
		"token ghp_"+strings.Repeat("a1B2", 9)+"\n")
	if got := runBrain(t, "", "promote", "notes/leak.md", "--to", "team"); got.code != exitSecretFound {
		t.Errorf("secret: exit %d, stderr %q", got.code, got.stderr)
	}
	if calls := env.gh.Calls(); len(calls) != 0 {
		t.Errorf("a refused promote called gh: %q", calls)
	}
	if branches := testgit.Run(t, env.sharedRemote, "branch", "--list", "brain/*"); branches != "" {
		t.Errorf("a refused promote pushed %q", branches)
	}
	assertWorktreesGone(t, env)
}
