package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/brain/internal/sharing"
	"github.com/mad01/thismoon/services/brain/internal/testgit"
)

// diskState fingerprints everything a promote could change: every file
// under each checkout but .git, each checkout's git refs, worktrees, index
// and working tree status, and the refs of the bare remote.
func diskState(t *testing.T, remote string, roots ...string) string {
	t.Helper()
	var b strings.Builder
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() && d.Name() == ".git" {
				return filepath.SkipDir
			}
			if d.IsDir() {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			sum := sha256.Sum256(data)
			b.WriteString(path + " " + hex.EncodeToString(sum[:]) + "\n")
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		for _, args := range [][]string{
			{"for-each-ref"},
			{"worktree", "list", "--porcelain"},
			{"status", "--porcelain=v1", "--untracked-files=all"},
			{"ls-files", "--stage"},
		} {
			b.WriteString(testgit.Run(t, root, args...) + "\n")
		}
	}
	return b.String() + testgit.Run(t, remote, "for-each-ref")
}

// stateDirEntries lists brain's state directory, which a dry run neither
// creates nor changes.
func stateDirEntries(t *testing.T) []string {
	t.Helper()
	var out []string
	root := filepath.Join(os.Getenv("HOME"), ".local", "state", "brain")
	_ = filepath.WalkDir(root, func(path string, _ fs.DirEntry, err error) error {
		if err == nil {
			out = append(out, path)
		}
		return nil
	})
	return out
}

func TestPromoteDryRunChangesNothingAndMatchesThePromote(t *testing.T) {
	env := setupSharing(t, false)
	writeNote(t, env.private, "notes/leak.md", "---\ntitle: Leak\nid: 01JC0000000000000000007EAK\n"+
		"status: validated\nclassification: internal\nreview_by: 2027-11-01\n---\n"+
		"token ghp_"+strings.Repeat("a1B2", 9)+"\n")
	writeNote(t, env.private, "notes/team-guide.md", "---\ntitle: Our Guide\n"+
		"id: 01JC0000000000000000000GD9\nstatus: validated\nclassification: internal\n"+
		"review_by: 2027-11-01\n---\nOur take.\n")
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)

	// The passing note goes last: its real promote changes both vaults.
	cases := []struct {
		name    string
		args    []string
		code    int
		verdict string
		gate    string
		want    string
	}{
		{
			"target not shared",
			[]string{"notes/rollout.md", "--to", "personal"},
			exitReadOnly, sharing.VerdictRefused, sharing.GateTarget, "not a shared vault",
		},
		{
			"not validated",
			[]string{"inbox/idea.md", "--to", "team"},
			exitUsage, sharing.VerdictRefused, sharing.GateStatus, "only a validated note",
		},
		{
			"path taken",
			[]string{"personal:notes/team-guide.md", "--to", "team"},
			exitExists, sharing.VerdictRefused, sharing.GateDuplicate, "team has notes/team-guide.md already",
		},
		{
			"secret",
			[]string{"notes/leak.md", "--to", "team"},
			exitSecretFound, sharing.VerdictRefused, sharing.GateSecrets, "github-pat",
		},
		{
			"classification",
			[]string{"notes/secret-plan.md", "--to", "team"},
			exitError, sharing.VerdictRefused, sharing.GateClassification,
			"classification confidential is not accepted by team (accepts: public, internal)",
		},
		{
			"passes",
			[]string{"notes/rollout.md", "--to", "team"},
			exitOK, sharing.VerdictProceed, sharing.GateCheck, "no errors",
		},
	}
	for _, tc := range cases {
		before := diskState(t, env.sharedRemote, env.private, env.shared)
		stateBefore := strings.Join(stateDirEntries(t), "\n")
		args := append([]string{"promote"}, tc.args...)
		dry := runBrain(t, "", append(args, "--dry-run", "-o", "json")...)
		if dry.code != tc.code {
			t.Fatalf(
				"%s: dry run exit %d, want %d; stderr %q",
				tc.name,
				dry.code,
				tc.code,
				dry.stderr,
			)
		}
		plan := decodeStrict[sharing.PromotePlan](t, dry.stdout)
		if !plan.DryRun || plan.Verdict != tc.verdict || len(plan.Gates) != 7 {
			t.Errorf("%s: plan = %+v", tc.name, plan)
		}
		if g := gateNamed(plan, tc.gate); !strings.Contains(g.Detail, tc.want) ||
			(tc.code == exitOK) != (g.Result == sharing.GatePass) {
			t.Errorf("%s: gate %s = %+v, want %q", tc.name, tc.gate, g, tc.want)
		}
		if after := diskState(t, env.sharedRemote, env.private, env.shared); after != before {
			t.Errorf(
				"%s: the dry run changed the vaults or their git state:\nbefore\n%s\nafter\n%s",
				tc.name,
				before,
				after,
			)
		}
		if calls := env.gh.Calls(); len(calls) != 0 {
			t.Fatalf("%s: the dry run called gh: %q", tc.name, calls)
		}
		if entries, _ := os.ReadDir(tmp); len(entries) != 0 {
			t.Errorf("%s: the dry run left %v in the temp dir", tc.name, entries)
		}
		if state := strings.Join(stateDirEntries(t), "\n"); state != stateBefore {
			t.Errorf(
				"%s: the dry run changed the state dir:\n%s\nwas\n%s",
				tc.name,
				state,
				stateBefore,
			)
		}

		real := runBrain(t, "", args...)
		if real.code != dry.code {
			t.Errorf("%s: promote exit %d, dry run exit %d; stderr %q",
				tc.name, real.code, dry.code, real.stderr)
		}
		if tc.code != exitOK && !strings.Contains(real.stderr, tc.want) {
			t.Errorf("%s: promote stderr %q lacks %q", tc.name, real.stderr, tc.want)
		}
	}
	if calls := env.gh.Calls(); len(calls) != 1 {
		t.Errorf("gh calls = %q, want the passing promote's pr create alone", calls)
	}
}

func gateNamed(p sharing.PromotePlan, name string) sharing.Gate {
	for _, g := range p.Gates {
		if g.Name == name {
			return g
		}
	}
	return sharing.Gate{}
}

func TestPromoteDryRunText(t *testing.T) {
	setupSharing(t, false)
	got := runBrain(t, "", "promote", "notes/secret-plan.md", "--to", "team", "--dry-run")
	if got.code != exitError {
		t.Fatalf("exit %d, stderr %q", got.code, got.stderr)
	}
	for _, want := range []string{
		"dry run: personal:notes/secret-plan.md -> team:notes/secret-plan.md: refused\n",
		"  classification  refused  classification confidential is not accepted by team " +
			"(accepts: public, internal)\n",
		"  status          pass     notes/secret-plan.md is validated",
		"  check           pass     brain check against team's policy: no errors",
		"branch: brain/promote/secret-plan-000005ec",
		"Nothing was written",
	} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout lacks %q:\n%s", want, got.stdout)
		}
	}
	if lines := strings.Count(got.stderr, "\n"); lines != 1 ||
		!strings.Contains(got.stderr, "would be refused") {
		t.Errorf("stderr = %q, want one line saying the promote would be refused", got.stderr)
	}

	ok := runBrain(t, "", "promote", "notes/rollout.md", "--to", "team", "--dry-run")
	if ok.code != exitOK || !strings.Contains(ok.stdout, "team:notes/rollout.md: proceed\n") ||
		!strings.Contains(ok.stdout, "list it in MOC MOCs/operations.md") {
		t.Errorf("passing dry run: exit %d, stdout %q, stderr %q", ok.code, ok.stdout, ok.stderr)
	}
}

func TestPromoteDryRunUsage(t *testing.T) {
	setupSharing(t, false)
	for _, args := range [][]string{
		{"promote", "inbox/idea.md", "--dry-run"},
		{"promote", "notes/rollout.md", "--to", "team", "--dry-run", "--moc", "MOCs/x.md"},
	} {
		if got := runBrain(t, "", args...); got.code != exitUsage {
			t.Errorf("%q: exit %d, stderr %q", args, got.code, got.stderr)
		}
	}
}
