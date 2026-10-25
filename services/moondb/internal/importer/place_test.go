package importer

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/mad01/thismoon/kit/gitloc"
	"github.com/mad01/thismoon/services/moondb/internal/store"
)

func TestPlace(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// A live checkout named tool whose origin names the repository repo,
	// and a live directory outside any repository.
	tool := filepath.Join(root, "code", "tool")
	plain := filepath.Join(root, "plain")
	for _, dir := range []string{filepath.Join(tool, "pkg"), plain} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	git(t, tool, "init", "-q", "-b", "main")
	git(t, tool, "remote", "add", "origin", fixtureOrigin)
	if err := os.WriteFile(filepath.Join(tool, "README"), []byte("fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	git(t, tool, "add", "README")
	git(t, tool, "commit", "-q", "-m", "init")
	// A linked worktree that still exists.
	live := filepath.Join(root, worktreesDir, "repo", "live")
	git(t, tool, "worktree", "add", "-q", "-b", "live", live)
	repo := gitloc.CanonicalRepo(fixtureOrigin)
	known := []store.Location{{Repo: repo, Worktree: tool, Canonical: tool}}

	// Paths that are gone: a linked worktree of repo, a checkout named like
	// the live one, and one the store knows nothing about.
	linked := filepath.Join(root, worktreesDir, "repo", "feature")
	agent := filepath.Join(tool, agentDir, agentWorktrees, "agent-1")
	renamed := filepath.Join(root, "old", "tool")
	stranger := filepath.Join(root, "old", "other")

	tests := []struct {
		name, abs, cwd, branch string
		// storeless drops the store's locations, leaving the cwd's own.
		storeless  bool
		want       store.Location
		wantPath   string
		wantCounts Placement
	}{
		{
			name: "live directory in a work tree",
			abs:  filepath.Join(tool, "pkg", "a.go"), cwd: tool, branch: "feature-x",
			want: store.Location{
				Repo:      repo,
				Worktree:  tool,
				Canonical: tool,
				Branch:    "feature-x",
			},
			wantPath: "pkg/a.go", wantCounts: Placement{Live: 1},
		},
		{
			name: "gone directory under a live work tree",
			abs:  filepath.Join(tool, "gone", "deep", "a.go"), cwd: tool, branch: "feature-x",
			want: store.Location{
				Repo:      repo,
				Worktree:  tool,
				Canonical: tool,
				Branch:    "feature-x",
			},
			wantPath: "gone/deep/a.go", wantCounts: Placement{Ancestor: 1},
		},
		{
			name: "gone linked worktree, repo by name",
			abs:  filepath.Join(linked, "pkg", "a.go"), cwd: linked, branch: "feature-y",
			want: store.Location{
				Repo:      repo,
				Worktree:  linked,
				Canonical: tool,
				Branch:    "feature-y",
			},
			wantPath: "pkg/a.go", wantCounts: Placement{Layout: 1},
		},
		{
			name: "gone directory in a live linked worktree",
			abs:  filepath.Join(live, "gone", "a.go"), cwd: live, branch: "live",
			want:     store.Location{Repo: repo, Worktree: live, Canonical: tool, Branch: "live"},
			wantPath: "gone/a.go", wantCounts: Placement{Ancestor: 1},
		},
		{
			name: "gone agent worktree inside the live checkout",
			abs:  filepath.Join(agent, "pkg", "a.go"), cwd: agent, branch: "worktree-agent-1",
			storeless: true,
			want: store.Location{
				Repo: repo, Worktree: agent, Canonical: tool, Branch: "worktree-agent-1",
			},
			wantPath: "pkg/a.go", wantCounts: Placement{Layout: 1},
		},
		{
			name: "gone flat worktree named for a known repo and a slug",
			abs:  filepath.Join(root, worktreesDir, "repo-fix", "pkg", "a.go"), cwd: tool,
			branch: "main",
			want: store.Location{
				Repo:      repo,
				Worktree:  filepath.Join(root, worktreesDir, "repo-fix"),
				Canonical: tool,
			},
			wantPath: "pkg/a.go", wantCounts: Placement{Layout: 1},
		},
		{
			name: "gone worktree of a repo nobody knows",
			abs:  filepath.Join(root, worktreesDir, "mystery", "slug", "a.go"), cwd: tool,
			branch: "main",
			want: store.Location{
				Worktree: filepath.Join(root, worktreesDir, "mystery", "slug"),
			},
			wantPath:   "a.go",
			wantCounts: Placement{Layout: 1, WithoutRepo: 1},
		},
		{
			name: "gone linked worktree, cwd in a subdirectory of it",
			abs:  filepath.Join(linked, "a.go"), cwd: filepath.Join(linked, "sub"),
			branch: "feature-y",
			want: store.Location{
				Repo:      repo,
				Worktree:  linked,
				Canonical: tool,
				Branch:    "feature-y",
			},
			wantPath: "a.go", wantCounts: Placement{Layout: 1},
		},
		{
			name: "gone linked worktree edited from the live checkout",
			abs:  filepath.Join(linked, "pkg", "a.go"), cwd: tool, branch: "main",
			storeless: true,
			want:      store.Location{Repo: repo, Worktree: linked, Canonical: tool},
			wantPath:  "pkg/a.go", wantCounts: Placement{Layout: 1},
		},
		{
			name: "gone checkout, repo by worktree name, detached",
			abs:  filepath.Join(renamed, "a.go"), cwd: renamed, branch: detachedHead,
			want:     store.Location{Repo: repo, Worktree: renamed, Canonical: tool},
			wantPath: "a.go", wantCounts: Placement{Cwd: 1},
		},
		{
			name: "gone checkout the store does not know",
			abs:  filepath.Join(stranger, "pkg", "a.go"), cwd: stranger, branch: "main",
			want:     store.Location{Worktree: stranger, Branch: "main"},
			wantPath: "pkg/a.go", wantCounts: Placement{Cwd: 1, WithoutRepo: 1},
		},
		{
			name: "gone file outside the cwd",
			abs:  filepath.Join(root, "elsewhere", "a.go"), cwd: stranger, branch: "main",
			wantPath: filepath.Join(root, "elsewhere", "a.go"), wantCounts: Placement{NoRepo: 1},
		},
		{
			name: "live directory outside any work tree",
			abs:  filepath.Join(plain, "a.go"), cwd: plain, branch: "main",
			wantPath: filepath.Join(plain, "a.go"), wantCounts: Placement{NoRepo: 1},
		},
		{
			name: "gone directory under a live cwd outside any work tree",
			abs:  filepath.Join(plain, "gone", "a.go"), cwd: plain, branch: "main",
			wantPath: filepath.Join(plain, "gone", "a.go"), wantCounts: Placement{NoRepo: 1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &placer{ctx: context.Background(), known: known}
			if tt.storeless {
				p.known = nil
			}
			var counts Placement
			loc, rel := p.place(tt.abs, tt.cwd, tt.branch, &counts)
			if loc != tt.want || rel != tt.wantPath {
				t.Errorf("place = %+v, %q; want %+v, %q", loc, rel, tt.want, tt.wantPath)
			}
			if counts != tt.wantCounts {
				t.Errorf("counts = %+v, want %+v", counts, tt.wantCounts)
			}
		})
	}
}

func TestRepoNamed(t *testing.T) {
	tests := []struct {
		name                    string
		known                   []store.Location
		wantRepo, wantCanonical string
	}{
		{
			name: "one repo, canonical agreed",
			known: []store.Location{
				{Repo: "example.com/o/tool", Worktree: "/a/tool", Canonical: "/a/tool"},
				{Repo: "example.com/o/tool", Worktree: "/w/tool/x", Canonical: "/a/tool"},
			},
			wantRepo: "example.com/o/tool", wantCanonical: "/a/tool",
		},
		{
			name: "one repo, canonicals differ",
			known: []store.Location{
				{Repo: "example.com/o/tool", Worktree: "/a/tool", Canonical: "/a/tool"},
				{Repo: "example.com/o/tool", Worktree: "/b/tool", Canonical: "/b/tool"},
			},
			wantRepo: "example.com/o/tool",
		},
		{
			name: "two repos by that name",
			known: []store.Location{
				{Repo: "example.com/o/tool", Worktree: "/a/tool"},
				{Repo: "example.com/p/tool", Worktree: "/b/tool"},
			},
		},
		{
			name:  "no repo by that name",
			known: []store.Location{{Repo: "example.com/o/other", Worktree: "/a/other"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &placer{known: tt.known}
			repo, canonical := p.repoNamed("tool")
			if repo != tt.wantRepo || canonical != tt.wantCanonical {
				t.Errorf("repoNamed = %q, %q; want %q, %q",
					repo, canonical, tt.wantRepo, tt.wantCanonical)
			}
		})
	}
}
