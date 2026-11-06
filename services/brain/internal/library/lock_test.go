package library

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/brain/internal/config"
	"github.com/mad01/thismoon/services/brain/internal/lockfile"
	"github.com/mad01/thismoon/services/brain/internal/semantic"
	"github.com/mad01/thismoon/services/brain/internal/vault"
)

// holdIndexLock takes one of the vault's index locks for the rest of the
// test, standing in for another brain process mid-build.
func holdIndexLock(t *testing.T, v *vault.Vault, kind string) string {
	t.Helper()
	path, err := lockPath(v, kind)
	if err != nil {
		t.Fatal(err)
	}
	release, err := lockfile.Hold(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = release() })
	return path
}

// within runs fn and fails the test when it has not returned in time, so a
// lock that blocks forever fails instead of hanging the run.
func within(t *testing.T, d time.Duration, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() { defer close(done); fn() }()
	select {
	case <-done:
	case <-time.After(d):
		t.Fatalf("still waiting on the lock after %s", d)
	}
}

func TestIndexLocksTimeOut(t *testing.T) {
	f := newSemanticFixture(t, semanticNotes)
	cfg := &config.Config{
		Vaults:   []config.Vault{{Name: "personal", Path: f.root, Default: true}},
		Semantic: config.SemanticConfig{Enabled: true, EmbedModel: "model-a"},
	}
	lib, err := Open(Options{Config: cfg, Embed: f.embed.embed, LockTimeout: 100 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	v := lib.Set().Vaults()[0]

	literal := holdIndexLock(t, v, lockLiteral)
	within(t, 5*time.Second, func() {
		_, err = lib.RefreshLiteral(v, true, nil)
	})
	if !errors.Is(err, lockfile.ErrTimeout) || !strings.Contains(err.Error(), literal) {
		t.Errorf("RefreshLiteral = %v, want ErrTimeout naming %s", err, literal)
	}

	holdIndexLock(t, v, lockSemantic)
	var res semantic.SearchResult
	within(t, 5*time.Second, func() {
		res, err = lib.SemanticSearch(context.Background(), lib.Set(), SemanticQuery{
			Text: "apple", Mode: semantic.ModePaths, Filter: semantic.QueryOptions{K: 5},
		})
	})
	if err != nil {
		t.Fatalf("a semantic query behind a held lock failed: %v", err)
	}
	if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "not refreshed") {
		t.Errorf("warnings = %q, want one saying the stale index was not refreshed", res.Warnings)
	}
}
