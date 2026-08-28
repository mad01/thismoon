package poller

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/mad01/thismoon/kit/repofind"
	"github.com/mad01/thismoon/services/prs/internal/config"
	"github.com/mad01/thismoon/services/prs/internal/store"
)

var base = time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)

// fakeFetcher returns canned PRs or errors per host/owner/name key.
type fakeFetcher struct {
	mu   sync.Mutex
	prs  map[string][]store.PR
	errs map[string]error
}

func (f *fakeFetcher) OpenPRs(_ context.Context, host, owner, name string) ([]store.PR, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := host + "/" + owner + "/" + name
	if err, ok := f.errs[key]; ok {
		return nil, err
	}
	return f.prs[key], nil
}

func TestPollTargets(t *testing.T) {
	cfg := config.Config{Hosts: []string{"github.com"}}
	repos := []repofind.Repo{
		{Name: "org/alpha", Host: "github.com"},
		{Name: "org/alpha", Host: "github.com"}, // second checkout of the same remote
		{Name: "org/beta", Host: "blocked.example.com"},
		{Name: "local/no-remote", Host: ""},
		{Name: "group/sub/deep", Host: "github.com"}, // nested path keeps last two
	}
	targets, err := pollTargets(repos, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 2 {
		t.Fatalf("got %d targets, want 2: %+v", len(targets), targets)
	}
	if targets[0].Owner != "org" || targets[0].Name != "alpha" {
		t.Errorf("target[0] = %+v", targets[0])
	}
	if targets[1].Owner != "sub" || targets[1].Name != "deep" {
		t.Errorf("target[1] = %+v", targets[1])
	}
}

func TestPollTargetsExcludeAndInclude(t *testing.T) {
	repos := []repofind.Repo{
		{Name: "mad01/prs-testbed", Host: "github.com"},
		{Name: "org/alpha", Host: "github.com"},
		{Name: "org/noisy", Host: "github.com"},
	}

	// The fixture repo is excluded by default.
	targets, err := pollTargets(repos, config.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 2 {
		t.Fatalf("default excludes: got %d targets, want 2: %+v", len(targets), targets)
	}

	// A config exclude glob drops more.
	targets, err = pollTargets(repos, config.Config{Exclude: []string{"org/noisy"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 || targets[0].Name != "alpha" {
		t.Fatalf("config exclude: got %+v", targets)
	}

	// Include overrides both the default and config excludes.
	targets, err = pollTargets(repos, config.Config{
		Exclude: []string{"org/noisy"},
		Include: []string{"mad01/prs-testbed", "org/noisy"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 3 {
		t.Fatalf("include override: got %d targets, want 3: %+v", len(targets), targets)
	}

	// A bad glob is an error, not a silent skip.
	if _, err := pollTargets(repos, config.Config{Exclude: []string{"[unclosed"}}); err == nil {
		t.Fatal("expected error for invalid exclude glob")
	}
}

// makeRepo builds a fake git checkout with the given origin remote.
func makeRepo(t *testing.T, root, name, remoteURL string) {
	t.Helper()
	dir := filepath.Join(root, name, ".git")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "[remote \"origin\"]\n\turl = " + remoteURL + "\n"
	if err := os.WriteFile(filepath.Join(dir, "config"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRefreshFetchesAndStores(t *testing.T) {
	root := t.TempDir()
	makeRepo(t, root, "alpha", "git@github.com:org/alpha.git")
	makeRepo(t, root, "beta", "git@github.com:org/beta.git")

	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	fetcher := &fakeFetcher{
		prs: map[string][]store.PR{
			"github.com/org/alpha": {{Host: "github.com", Repo: "org/alpha", Number: 1}},
		},
		errs: map[string]error{
			"github.com/org/beta": errors.New("boom"),
		},
	}
	p := New(config.Config{Dirs: []string{root}, PollInterval: time.Minute}, st, fetcher)
	p.now = func() time.Time { return base }

	sum, err := p.Refresh(context.Background())
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if sum.Repos != 2 || sum.OpenPRs != 1 || sum.Errors != 1 {
		t.Errorf("Summary = %+v", sum)
	}
	if prs := st.List(store.Filter{}); len(prs) != 1 || prs[0].Number != 1 {
		t.Errorf("stored PRs wrong: %+v", prs)
	}
	status := st.Status()
	if len(status.Errors) != 1 || status.Errors[0].Repo != "org/beta" {
		t.Errorf("status errors = %+v", status.Errors)
	}
}

func TestRefreshKeepsPRsOnFetchError(t *testing.T) {
	root := t.TempDir()
	makeRepo(t, root, "alpha", "git@github.com:org/alpha.git")

	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	fetcher := &fakeFetcher{
		prs: map[string][]store.PR{
			"github.com/org/alpha": {{Host: "github.com", Repo: "org/alpha", Number: 1}},
		},
	}
	p := New(config.Config{Dirs: []string{root}, PollInterval: time.Minute}, st, fetcher)
	p.now = func() time.Time { return base }
	if _, err := p.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}

	// Second cycle fails for alpha: the PR list must survive, with the error
	// recorded beside it.
	fetcher.mu.Lock()
	fetcher.errs = map[string]error{"github.com/org/alpha": errors.New("rate limited")}
	fetcher.mu.Unlock()
	if _, err := p.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}

	if prs := st.List(store.Filter{}); len(prs) != 1 {
		t.Errorf("error cycle blanked the PRs: %+v", prs)
	}
	if status := st.Status(); len(status.Errors) != 1 {
		t.Errorf("error not recorded: %+v", status.Errors)
	}
}

func TestDueUsesIntervalAndLastAttempt(t *testing.T) {
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	p := New(config.Config{PollInterval: 5 * time.Minute}, st, &fakeFetcher{})
	clock := base
	p.now = func() time.Time { return clock }

	if !p.due() {
		t.Error("empty store should be immediately due")
	}
	if _, err := p.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if p.due() {
		t.Error("freshly polled should not be due")
	}
	clock = clock.Add(6 * time.Minute)
	if !p.due() {
		t.Error("past the interval should be due")
	}
}
