package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/brain/internal/drift"
	"github.com/mad01/thismoon/services/brain/internal/lifecycle"
	"github.com/mad01/thismoon/services/brain/internal/stale"
	"github.com/mad01/thismoon/services/brain/internal/testgit"
	"github.com/mad01/thismoon/services/brain/internal/vault"
)

func TestFlag(t *testing.T) {
	roots := setupVaults(t)
	got := runBrain(
		t,
		"",
		"flag",
		"personal:notes/mine.md",
		"--reason",
		"numbers look off",
		"-o",
		"json",
	)
	if got.code != exitOK {
		t.Fatalf("flag: exit %d, stderr %q", got.code, got.stderr)
	}
	res := decodeStrict[vault.PathResult](t, got.stdout)
	if res.Vault != "personal" || res.Path != "notes/mine.md" || res.SHA == "" {
		t.Errorf("flag result = %+v", res)
	}
	read := runBrain(t, "", "read", "personal:notes/mine.md")
	if !strings.Contains(read.stdout, "DISPUTED on ") ||
		!strings.Contains(read.stdout, "numbers look off") {
		t.Errorf("read after flag shows no disputed banner:\n%s", read.stdout)
	}

	if got := runBrain(t, "", "flag", "personal:notes/mine.md"); got.code != exitUsage {
		t.Errorf("flag without --reason: exit %d, want %d", got.code, exitUsage)
	}

	testgit.Init(t, roots["team"])
	testgit.Run(t, roots["team"], "remote", "add", "origin", "https://example.com/team/notes.git")
	shared := runBrain(t, "", "flag", "team:notes/team-guide.md", "--reason", "outdated")
	if shared.code != exitReadOnly {
		t.Fatalf("flag in a shared vault: exit %d, want %d", shared.code, exitReadOnly)
	}
	for _, want := range []string{"opening an issue", "team:notes/team-guide.md", "https://example.com/team/notes.git"} {
		if !strings.Contains(shared.stderr, want) {
			t.Errorf("shared flag stderr %q lacks %q", shared.stderr, want)
		}
	}
}

func TestInboxListKeepArchive(t *testing.T) {
	root := setupCLI(t, "")
	writeNote(t, root, "inbox/stale-capture.md",
		"---\ntitle: Stale Capture\nstatus: raw\nexpires_at: 2020-01-01\n---\nzebrafish capture\n")

	list := decodeStrict[vault.InboxListing](
		t,
		runBrain(t, "", "inbox", "list", "-o", "json").stdout,
	)
	if list.Total != 3 || list.Notes[0].Path != "inbox/stale-capture.md" || !list.Notes[0].Expired {
		t.Fatalf("inbox list = %+v", list)
	}
	expired := decodeStrict[vault.InboxListing](
		t, runBrain(t, "", "inbox", "list", "--expired", "-o", "json").stdout)
	if expired.Total != 1 {
		t.Errorf("inbox list --expired = %+v, want the stale capture alone", expired)
	}

	keep := runBrain(t, "", "inbox", "keep", "inbox/raw.md", "--days", "10", "-o", "json")
	res := decodeStrict[vault.KeepResult](t, keep.stdout)
	want := lifecycle.Date(time.Now().AddDate(0, 0, 10))
	if keep.code != exitOK || res.ExpiresAt != want {
		t.Errorf("inbox keep: exit %d, %+v; want expires_at %s", keep.code, res, want)
	}
	if got := runBrain(t, "", "inbox", "keep", "notes/alpha.md"); got.code != exitUsage {
		t.Errorf("keep of a validated note: exit %d, want %d", got.code, exitUsage)
	}

	if got := runBrain(t, "", "search", "zebrafish"); !strings.Contains(
		got.stdout,
		"stale-capture",
	) {
		t.Fatalf("search before archive = %q", got.stdout)
	}
	if got := runBrain(t, "", "inbox", "archive"); got.code != exitUsage {
		t.Errorf("archive without --expired: exit %d, want %d", got.code, exitUsage)
	}
	archive := runBrain(t, "", "inbox", "archive", "--expired", "-o", "json")
	report := decodeStrict[ArchiveReport](t, archive.stdout)
	if archive.code != exitOK || len(report.Archived) != 1 ||
		report.Archived[0].ArchivedTo != ".brain/archive/inbox/stale-capture.md" {
		t.Fatalf("archive: exit %d, %+v, stderr %q", archive.code, report, archive.stderr)
	}
	if got := runBrain(t, "", "search", "zebrafish"); strings.Contains(
		got.stdout,
		"stale-capture",
	) {
		t.Errorf("search after archive = %q, want no hit", got.stdout)
	}
	if _, err := os.Stat(filepath.Join(root, ".brain", "archive", "inbox", "stale-capture.md")); err != nil {
		t.Errorf("archived file: %v", err)
	}
	if got := runBrain(t, "", "check", "-o", "json"); strings.Contains(
		got.stdout,
		"stale-capture",
	) {
		t.Errorf("check still sees the archived note:\n%s", got.stdout)
	}
}

// writeNote writes content to the vault-relative path rel under root.
func writeNote(t *testing.T, root, rel, content string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestWriteRefusesAPossibleDuplicate(t *testing.T) {
	root := setupCLI(t, "")
	got := runBrain(t, "", "write", "--title", "ALPHA", "--body", "again")
	if got.code != exitDuplicate {
		t.Fatalf("write: exit %d, want %d; stderr %q", got.code, exitDuplicate, got.stderr)
	}
	for _, want := range []string{
		"default:notes/alpha.md (same title), default:inbox/dup.md (same title)",
		"brain edit default:notes/alpha.md",
	} {
		if !strings.Contains(got.stderr, want) {
			t.Errorf("stderr %q lacks %q", got.stderr, want)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "inbox", "alpha.md")); err == nil {
		t.Error("a refused write left a file")
	}
	ok := runBrain(t, "", "write", "--title", "ALPHA", "--body", "again",
		"--distinct-from", "notes/alpha.md", "--distinct-from", "inbox/dup.md", "-o", "json")
	if ok.code != exitOK {
		t.Fatalf("write --distinct-from: exit %d, stderr %q", ok.code, ok.stderr)
	}
	read := runBrain(t, "", "read", "inbox/alpha.md")
	if !strings.Contains(read.stdout,
		"distinct_from:\n  - default:notes/alpha.md\n  - default:inbox/dup.md\n") {
		t.Errorf("read after write --distinct-from:\n%s", read.stdout)
	}
	if got := runBrain(t, "", "edit", "notes/alpha.md", "--title", "Beta"); got.code != exitOK {
		t.Errorf("edit to another note's title: exit %d, want 0 (edit never checks)", got.code)
	}
}

func TestCapture(t *testing.T) {
	root := setupCLI(t, "")
	got := runBrain(t, "# Zonal rollouts\nDrain one zone at a time.\n", "capture",
		"--by", "agent:s-42", "--source", "https://example.com/zonal", "-o", "json")
	if got.code != exitOK {
		t.Fatalf("capture: exit %d, stderr %q", got.code, got.stderr)
	}
	res := decodeStrict[vault.PathResult](t, got.stdout)
	if res.Path != "inbox/zonal-rollouts.md" {
		t.Errorf("capture path = %q", res.Path)
	}
	v := &vault.Vault{Root: root}
	fm, _, err := v.ReadFrontmatter(filepath.Join(root, "inbox", "zonal-rollouts.md"))
	if err != nil {
		t.Fatal(err)
	}
	f := lifecycle.Read(fm)
	if fm.Status != lifecycle.Raw || fm.Type != "source" ||
		fm.Source != "https://example.com/zonal" ||
		f.CapturedBy != "agent:s-42" ||
		f.ExpiresAt != lifecycle.Date(time.Now().AddDate(0, 0, 30)) {
		t.Errorf("capture frontmatter = %+v, lifecycle %+v", fm, f)
	}

	tests := []struct {
		name  string
		stdin string
		args  []string
		code  int
	}{
		{"bad capturer", "x", []string{"capture", "--title", "X", "--by", "bot"}, exitUsage},
		{"no title", "", []string{"capture", "--body", ""}, exitUsage},
		{
			"secret",
			"key " + fixtureKey + "\n",
			[]string{"capture", "--title", "Leak"},
			exitSecretFound,
		},
		{"duplicate", "again", []string{"capture", "--title", "Alpha"}, exitDuplicate},
	}
	for _, tt := range tests {
		if got := runBrain(t, tt.stdin, tt.args...); got.code != tt.code {
			t.Errorf("%s: exit %d, want %d; stderr %q", tt.name, got.code, tt.code, got.stderr)
		}
	}
	ok := runBrain(t, "again", "capture", "--title", "Alpha",
		"--distinct-from", "notes/alpha.md", "--distinct-from", "inbox/dup.md")
	if ok.code != exitOK || strings.TrimSpace(ok.stdout) != "inbox/alpha.md" {
		t.Errorf(
			"capture --distinct-from: exit %d, stdout %q, stderr %q",
			ok.code,
			ok.stdout,
			ok.stderr,
		)
	}
}

func TestDrift(t *testing.T) {
	code := t.TempDir()
	repo := filepath.Join(code, "github.com", "acme", "app")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	testgit.Init(t, repo)
	first := testgit.Commit(t, repo, "first", map[string]string{"main.go": "func Serve() {}\n"})
	root := setupCLI(t, "code_roots: ["+code+"]\n")
	writeNote(
		t,
		root,
		"notes/serving.md",
		"---\ntitle: Serving\nsources:\n  - ref: acme/app:main.go@"+
			first+"#Serve\n  - url: https://example.invalid/never-fetched\n---\nServe handles it.\n",
	)

	clean := runBrain(t, "", "drift", "-o", "json")
	if clean.code != exitOK {
		t.Fatalf("drift before a change: exit %d, stderr %q", clean.code, clean.stderr)
	}
	// Offline counts this note's url and the one inbox/raw.md cites.
	if r := decodeStrict[drift.Report](t, clean.stdout); !r.OK || r.Offline != 2 ||
		len(r.Notes) != 0 {
		t.Errorf("clean report = %+v", r)
	}

	testgit.Commit(t, repo, "second", map[string]string{"main.go": "func Listen() {}\n"})
	got := runBrain(t, "", "drift", "notes/serving.md")
	if got.code != exitError {
		t.Fatalf(
			"drift after a change: exit %d, want %d; stderr %q",
			got.code,
			exitError,
			got.stderr,
		)
	}
	for _, want := range []string{"notes/serving.md", "changed", "symbol-missing", "1 of 1 note(s) drifted"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("drift text lacks %q:\n%s", want, got.stdout)
		}
	}
	if !strings.HasPrefix(got.stderr, "brain drift: 1 of 1 note(s) drifted") {
		t.Errorf("stderr = %q", got.stderr)
	}
}

func TestStale(t *testing.T) {
	code := t.TempDir()
	repo := filepath.Join(code, "github.com", "acme", "app")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	testgit.Init(t, repo)
	first := testgit.Commit(t, repo, "first", map[string]string{"main.go": "func Serve() {}\n"})
	root := setupCLI(t, "code_roots: ["+code+"]\n")
	writeNote(t, root, "CODEOWNERS", "notes/infra/ @team/infra\n")
	writeNote(t, root, "notes/infra/dns.md",
		"---\ntitle: DNS\nreview_by: 2020-01-01\n---\nResolvers.\n")
	writeNote(t, root, "notes/infra/ntp.md",
		"---\ntitle: NTP\nreview_by: 2020-02-01\n---\nClocks.\n")
	writeNote(t, root, "notes/claim.md",
		"---\ntitle: Claim\nsteward: '@kim'\nstatus: disputed\nstatus_reason: one machine only\n"+
			"status_at: 2026-10-20\n---\nA claim.\n")
	writeNote(t, root, "notes/serving.md",
		"---\ntitle: Serving\nsources:\n  - ref: acme/app:main.go@"+first+"#Serve\n---\nServe.\n")

	got := runBrain(t, "", "stale", "-o", "json")
	if got.code != exitOK {
		t.Fatalf("stale: exit %d, stderr %q", got.code, got.stderr)
	}
	q := decodeStrict[stale.Queue](t, got.stdout)
	if len(q.Groups) < 2 || q.Groups[0].Domain != "@team/infra" || q.Groups[0].Total != 2 ||
		q.Groups[0].From != stale.FromCodeowners || q.Groups[1].Domain != "@kim" || q.Drift {
		t.Fatalf("stale queue = %+v", q)
	}
	if first := q.Groups[0].Notes[0]; first.Path != "notes/infra/dns.md" ||
		first.Reasons[0].Kind != stale.ReasonExpired {
		t.Errorf("oldest infra note = %+v, want dns.md expired", first)
	}

	limited := decodeStrict[stale.Queue](
		t,
		runBrain(t, "", "stale", "--limit", "1", "-o", "json").stdout,
	)
	if g := limited.Groups[0]; g.Total != 2 || len(g.Notes) != 1 {
		t.Errorf("--limit 1 infra group = %+v", g)
	}
	text := runBrain(t, "", "stale", "--limit", "1").stdout
	for _, want := range []string{"@team/infra (codeowners, 2 note(s), showing 1)", "notes/infra/dns.md  expired: review_by 2020-01-01"} {
		if !strings.Contains(text, want) {
			t.Errorf("stale text lacks %q:\n%s", want, text)
		}
	}

	testgit.Commit(t, repo, "second", map[string]string{"main.go": "func Listen() {}\n"})
	if strings.Contains(runBrain(t, "", "stale").stdout, "serving") {
		t.Error("stale without --drift queued the drifted note")
	}
	if withDrift := runBrain(t, "", "stale", "--drift").stdout; !strings.Contains(
		withDrift,
		"notes/serving.md  drift: changed acme/app:main.go@",
	) {
		t.Errorf("stale --drift misses the drifted note:\n%s", withDrift)
	}

	for _, args := range [][]string{{"stale", "--online"}, {"stale", "--limit", "0"}, {"stale", "x"}} {
		if got := runBrain(t, "", args...); got.code != exitUsage {
			t.Errorf("%v: exit %d, want %d", args, got.code, exitUsage)
		}
	}
}
