package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mad01/thismoon/tools/humanizer/internal/rules"
)

func TestDetectFileError(t *testing.T) {
	t.Run("permission denial gets sandbox guidance", func(t *testing.T) {
		stat := fmt.Errorf("stat /Users/x/secret.env: %w", fs.ErrPermission)
		got := detectFileError(stat, "/Users/x/secret.env")
		if !strings.Contains(got.Error(), "humanizer_detect instead") {
			t.Errorf("want guidance pointing at humanizer_detect, got: %v", got)
		}
		if !strings.Contains(got.Error(), `"use_instead":"humanizer_detect"`) {
			t.Errorf("want the machine-readable use_instead field, got: %v", got)
		}
		if !errors.Is(got, fs.ErrPermission) {
			t.Errorf("original error should stay wrapped, got: %v", got)
		}
	})

	t.Run("other errors pass through unchanged", func(t *testing.T) {
		notExist := fmt.Errorf("stat /tmp/gone.md: %w", fs.ErrNotExist)
		if got := detectFileError(notExist, "/tmp/gone.md"); got != notExist {
			t.Errorf("want error unchanged, got: %v", got)
		}
	})
}

func TestRepoGlossaryError(t *testing.T) {
	t.Run("permission denial names the CLI form", func(t *testing.T) {
		walk := fmt.Errorf("glossary: walk /Users/x/repo: %w", fs.ErrPermission)
		form := steCLIForm(rules.ProfileProcedure, detectInput{Repo: "/Users/x/repo"})
		got := repoGlossaryError(walk, "/Users/x/repo", form)
		want := `"use_instead":"humanizer ste --profile procedure --repo /Users/x/repo"`
		if !strings.Contains(got.Error(), want) {
			t.Errorf("want %s, got: %v", want, got)
		}
		if !errors.Is(got, fs.ErrPermission) {
			t.Errorf("original error should stay wrapped, got: %v", got)
		}
	})

	t.Run("other errors pass through unchanged", func(t *testing.T) {
		notExist := fmt.Errorf("glossary: repo /gone: %w", fs.ErrNotExist)
		if got := repoGlossaryError(notExist, "/gone", "humanizer ste"); got != notExist {
			t.Errorf("want error unchanged, got: %v", got)
		}
	})
}

func TestSTECLIForm(t *testing.T) {
	in := detectInput{Repo: "/r", Path: "services/present"}
	want := "humanizer ste --profile description --repo /r --path services/present"
	if got := steCLIForm(rules.ProfileDescription, in); got != want {
		t.Errorf("steCLIForm = %q, want %q", got, want)
	}
}

func TestDetectOptions(t *testing.T) {
	repo := t.TempDir()
	ctx := "**Tool**:\nA CLI.\n_Avoid_: binary, --bin\n"
	if err := os.WriteFile(filepath.Join(repo, "CONTEXT.md"), []byte(ctx), 0o600); err != nil {
		t.Fatalf("write CONTEXT.md: %v", err)
	}
	sub := "**Step**:\nOne press.\n_Avoid_: stage\n"
	if err := os.MkdirAll(filepath.Join(repo, "deck"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "deck", "CONTEXT.md"), []byte(sub), 0o600); err != nil {
		t.Fatalf("write deck/CONTEXT.md: %v", err)
	}
	tests := []struct {
		name         string
		in           detectInput
		wantErr      string
		wantProfile  rules.Profile
		wantGlossary bool
		wantBans     []string
		wantNotes    int
	}{
		{name: "humanizer pack", in: detectInput{Rules: []string{"Humanizer.AIVocabulary"}}},
		{
			name:        "profile only",
			in:          detectInput{Profile: "procedure"},
			wantProfile: rules.ProfileProcedure,
		},
		{
			name:         "profile and repo",
			in:           detectInput{Profile: "description", Repo: repo},
			wantProfile:  rules.ProfileDescription,
			wantGlossary: true,
			wantBans:     []string{"'binary'", "'stage'"},
			wantNotes:    1,
		},
		{
			name:         "path outside the deck context",
			in:           detectInput{Profile: "description", Repo: repo, Path: "README.md"},
			wantProfile:  rules.ProfileDescription,
			wantGlossary: true,
			wantBans:     []string{"'binary'"},
			wantNotes:    1,
		},
		{
			name:         "path inside the deck context",
			in:           detectInput{Profile: "procedure", Repo: repo, Path: "deck/slides.md"},
			wantProfile:  rules.ProfileProcedure,
			wantGlossary: true,
			wantBans:     []string{"'binary'", "'stage'"},
			wantNotes:    1,
		},
		{name: "repo without profile", in: detectInput{Repo: repo}, wantErr: "repo needs profile"},
		{
			name:    "path without repo",
			in:      detectInput{Profile: "procedure", Path: "README.md"},
			wantErr: "path needs repo",
		},
		{
			name:    "relative repo",
			in:      detectInput{Profile: "procedure", Repo: "tools/humanizer"},
			wantErr: "repo must be an absolute path",
		},
		{name: "unknown profile", in: detectInput{Profile: "steps"}, wantErr: "unknown profile"},
		{
			name:    "unknown rule",
			in:      detectInput{Rules: []string{"STE.Nope"}},
			wantErr: "unknown rule",
		},
		{
			name:    "missing repo",
			in:      detectInput{Profile: "procedure", Repo: filepath.Join(repo, "gone")},
			wantErr: "no such file",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, notes, err := detectOptions(tt.in)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("detectOptions error = %v, want it to contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("detectOptions: %v", err)
			}
			if got.Profile != tt.wantProfile || (len(got.Glossary) > 0) != tt.wantGlossary {
				t.Errorf("profile = %q, glossary set = %v; want %q, %v",
					got.Profile, len(got.Glossary) > 0, tt.wantProfile, tt.wantGlossary)
			}
			var bans []string
			for _, key := range []string{"'binary'", "'stage'"} {
				if strings.Contains(string(got.Glossary), key+":") {
					bans = append(bans, key)
				}
			}
			if !slices.Equal(bans, tt.wantBans) {
				t.Errorf("glossary bans %v, want %v", bans, tt.wantBans)
			}
			if len(notes) != tt.wantNotes {
				t.Errorf("notes = %q, want %d", notes, tt.wantNotes)
			}
		})
	}
}

// TestHandleDetectProfile runs humanizer_detect end to end with a profile:
// the findings are STE findings, enriched with the ste category.
func TestHandleDetectProfile(t *testing.T) {
	if _, err := exec.LookPath("vale"); err != nil {
		t.Skip("vale not installed")
	}
	t.Setenv("HUMANIZER_CACHE_DIR", t.TempDir())
	_, out, err := handleDetect(context.Background(), nil, detectInput{
		Text:    "Run the tests and then push the branch.\n",
		Profile: "procedure",
	})
	if err != nil {
		t.Fatalf("handleDetect: %v", err)
	}
	if len(out.Findings) != 1 || out.Findings[0].RuleID != "STE.OneInstruction" {
		t.Fatalf("findings = %+v, want one STE.OneInstruction", out.Findings)
	}
	if out.Findings[0].Category != "ste" {
		t.Errorf("category = %q, want ste", out.Findings[0].Category)
	}
}
