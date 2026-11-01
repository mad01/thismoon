package selfcheck

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/thismoon/kit/llm"
	"github.com/mad01/thismoon/services/csl/internal/ask"
	"github.com/mad01/thismoon/services/csl/internal/repo/config"
)

// namedBackend is a worker that only reports its name and model.
type namedBackend struct{}

func (namedBackend) Name() string  { return "fake" }
func (namedBackend) Model() string { return "claude-haiku-5-5" }
func (namedBackend) Complete(context.Context, string, string) (string, error) {
	return "", errors.New("the doctor must not call the worker")
}

// TestAskBackend covers the three outcomes: a resolved worker and a missing
// one both pass with a note, since search works without csl_ask, and only a
// config value kit/llm cannot run fails.
func TestAskBackend(t *testing.T) {
	resolved := func(ask.Settings, func(llm.CallInfo)) (llm.Backend, error) {
		return namedBackend{}, nil
	}
	missing := func(ask.Settings, func(llm.CallInfo)) (llm.Backend, error) {
		return nil, llm.ErrNoBackend
	}
	tests := []struct {
		name      string
		yaml      string
		newWorker func(ask.Settings, func(llm.CallInfo)) (llm.Backend, error)
		wantSkip  bool
		want      string
	}{
		{
			name: "resolved", yaml: "dirs: [/tmp]\n", newWorker: resolved,
			wantSkip: true, want: "fake/claude-haiku-5-5, effort low",
		},
		{
			name: "none configured", yaml: "dirs: [/tmp]\n", newWorker: missing,
			wantSkip: true, want: "search works without one",
		},
		{
			name: "unknown backend", yaml: "ask:\n  backend: vertex\n", newWorker: resolved,
			want: "ask.backend",
		},
		{
			name: "unknown effort", yaml: "ask:\n  effort: turbo\n", newWorker: resolved,
			want: "ask.effort",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			writeFile(t, path, tc.yaml)
			config.SetPath(path)
			t.Cleanup(func() { config.SetPath("") })

			err := askBackend(newShared(), tc.newWorker).Run(context.Background())
			switch {
			case err == nil:
				t.Fatalf("Run() = nil, want a note or a failure mentioning %q", tc.want)
			case isSkip(err) != tc.wantSkip:
				t.Errorf("Run() = %v, skip=%v, want skip=%v", err, isSkip(err), tc.wantSkip)
			case !strings.Contains(err.Error(), tc.want):
				t.Errorf("Run() = %q, want it to mention %q", err, tc.want)
			}
		})
	}
}
