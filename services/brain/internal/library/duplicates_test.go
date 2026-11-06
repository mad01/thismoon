package library

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/brain/internal/config"
	"github.com/mad01/thismoon/services/brain/internal/lifecycle"
	"github.com/mad01/thismoon/services/brain/internal/vault"
)

const duplicateBody = "Rolling node pools one zone at a time keeps the control plane " +
	"reachable while the kubelets drain and rejoin."

// duplicateFixture is a private personal vault and a shared team vault.
func duplicateFixture(t *testing.T, semanticOn bool) (*Library, *semanticFixture) {
	t.Helper()
	f := newSemanticFixture(t, map[string]string{
		"notes/node-pools.md": "---\ntitle: Node Pool Upgrades\nid: 01JC0000000000000000000001\n" +
			"aliases: [Pool rollouts]\n---\n" + duplicateBody + "\n",
		"notes/gone.md": "---\ntitle: Retired Topic\nstatus: retracted\nretracted_reason: error\n" +
			"retracted_at: 2026-01-01\n---\nold\n",
	})
	team := newSemanticFixture(t, map[string]string{
		"notes/guide.md": "---\ntitle: Team Guide\n---\nThe team guide.\n",
	})
	cfg := &config.Config{
		Vaults: []config.Vault{
			{Name: "personal", Path: f.root, Default: true},
			{Name: "team", Path: team.root, Shared: true},
		},
		Semantic: config.SemanticConfig{Enabled: semanticOn, EmbedModel: "m"},
	}
	lib, err := Open(Options{Config: cfg, Embed: f.embed.embed})
	if err != nil {
		t.Fatal(err)
	}
	return lib, f
}

func personal(t *testing.T, lib *Library) *vault.Vault {
	t.Helper()
	v, err := lib.WriteTarget("")
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestWriteRefusesATitleDuplicate(t *testing.T) {
	lib, _ := duplicateFixture(t, false)
	v := personal(t, lib)
	ctx := context.Background()
	tests := []struct {
		title, want string
	}{
		{"node-pool upgrades!", "personal:notes/node-pools.md (same title)"},
		{"Pool Rollouts", `personal:notes/node-pools.md (alias of "Node Pool Upgrades")`},
		{"TEAM guide", "team:notes/guide.md (same title)"},
	}
	for _, tt := range tests {
		_, _, err := lib.Write(
			ctx,
			v,
			vault.WriteOptions{Tier: vault.TierInbox, Title: tt.title},
			nil,
		)
		if !errors.Is(err, vault.ErrPossibleDuplicate) || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("write %q = %v, want a possible duplicate naming %s", tt.title, err, tt.want)
		}
	}
	// A revoked note is no candidate: its replacement may take its title.
	if _, _, err := lib.Write(ctx, v, vault.WriteOptions{Tier: vault.TierNotes, Title: "Retired topic!"}, nil); err != nil {
		t.Errorf("write over a retracted title = %v, want it written", err)
	}
}

func TestWriteDistinctFromLetsTheWriteThrough(t *testing.T) {
	lib, _ := duplicateFixture(t, false)
	v := personal(t, lib)
	ctx := context.Background()
	opts := vault.WriteOptions{Tier: vault.TierInbox, Title: "Node pool upgrades", Filename: "np-2"}
	if _, _, err := lib.Write(ctx, v, opts, []string{"team:notes/guide.md"}); !errors.Is(
		err, vault.ErrPossibleDuplicate) {
		t.Fatalf("distinct from another note = %v, want still refused", err)
	}
	if _, _, err := lib.Write(ctx, v, opts, []string{"notes/missing.md"}); !errors.Is(
		err,
		vault.ErrNotFound,
	) {
		t.Errorf("distinct from a missing note = %v, want ErrNotFound", err)
	}
	path, _, err := lib.Write(ctx, v, opts, []string{"personal:notes/node-pools.md"})
	if err != nil {
		t.Fatal(err)
	}
	fm, _, err := v.ReadFrontmatter(path)
	if err != nil {
		t.Fatal(err)
	}
	got := lifecycle.Read(fm).DistinctFrom
	if len(got) != 1 || got[0] != "id:01JC0000000000000000000001" {
		t.Errorf("distinct_from = %v, want the candidate's id ref", got)
	}
}

func TestWriteRefusesASemanticDuplicate(t *testing.T) {
	lib, f := duplicateFixture(t, true)
	v := personal(t, lib)
	ctx := context.Background()
	opts := vault.WriteOptions{Tier: vault.TierInbox, Title: "Zonal rollout", Body: duplicateBody}
	// No semantic index yet: only the title check runs.
	if _, warnings, err := lib.Write(ctx, v, withFilename(opts, "first"), nil); err != nil ||
		len(warnings) > 0 {
		t.Fatalf("write before indexing = %v (warnings %v), want it written", err, warnings)
	}
	if _, err := lib.RefreshSemantic(ctx, v, true, nil); err != nil {
		t.Fatal(err)
	}
	calls := f.embed.calls.Load()
	_, _, err := lib.Write(ctx, v, withFilename(opts, "second"), nil)
	if !errors.Is(err, vault.ErrPossibleDuplicate) ||
		!strings.Contains(err.Error(), "similarity 1.00") {
		t.Fatalf("write of the same body = %v, want a semantic duplicate", err)
	}
	if f.embed.calls.Load() != calls+1 {
		t.Errorf("the check embedded %d texts, want the draft's first chunk alone",
			f.embed.calls.Load()-calls)
	}
	other := vault.WriteOptions{
		Tier:  vault.TierInbox,
		Title: "Espresso",
		Body:  "Grind finer for a slower shot.",
	}
	if _, _, err := lib.Write(ctx, v, other, nil); err != nil {
		t.Errorf("write of an unrelated note = %v", err)
	}
}

func TestWriteSkipsTheSemanticCheckWhenTheEmbedderIsDown(t *testing.T) {
	lib, _ := duplicateFixture(t, true)
	v := personal(t, lib)
	ctx := context.Background()
	if _, err := lib.RefreshSemantic(ctx, v, true, nil); err != nil {
		t.Fatal(err)
	}
	lib.health = func(context.Context) error { return errors.New("connection refused") }
	opts := vault.WriteOptions{Tier: vault.TierInbox, Title: "Zonal rollout", Body: duplicateBody}
	_, warnings, err := lib.Write(ctx, v, opts, nil)
	if err != nil || len(warnings) != 1 || !strings.Contains(warnings[0], "connection refused") {
		t.Errorf("write with the embedder down = %v, warnings %v; want written with one warning",
			err, warnings)
	}
}

func withFilename(opts vault.WriteOptions, name string) vault.WriteOptions {
	opts.Filename = name
	return opts
}
