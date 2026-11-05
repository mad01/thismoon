package check

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/brain/internal/vault"
)

var checkDay = time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC)

const (
	idA = "01JC0000000000000000000001"
	idB = "01JC0000000000000000000002"
	idC = "01JC0000000000000000000003"
)

// note renders a note from its frontmatter lines and body.
func note(fields, body string) string { return "---\n" + fields + "---\n" + body }

// validated is the frontmatter of a validated note that checks clean. A
// body written under it starts on line 7.
func validated(id string) string {
	return "status: validated\nid: " + id + "\nclassification: internal\nreview_by: 2027-06-01\n"
}

const retracted = "status: retracted\nid: " + idB + "\nclassification: internal\n" +
	"retracted_reason: error\nretracted_at: 2026-10-01\n"

// newVault writes files under a temp vault root and returns it as a set.
func newVault(t *testing.T, files map[string]string) (*vault.Vault, *vault.Set) {
	t.Helper()
	root := t.TempDir()
	for rel, content := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	v := &vault.Vault{Root: root, Name: "main"}
	return v, vault.NewSet(v)
}

// runCheck checks targets (every note when empty) and returns the findings
// as "path:line rule level".
func runCheck(t *testing.T, files map[string]string, targets ...string) (Report, []string) {
	t.Helper()
	v, set := newVault(t, files)
	opts := Options{Set: set, Now: checkDay}
	for _, rel := range targets {
		opts.Targets = append(opts.Targets, vault.Ref{Vault: v, Path: rel})
	}
	r, err := Run(opts)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := make([]string, 0, len(r.Findings))
	for _, f := range r.Findings {
		got = append(got, fmt.Sprintf("%s:%d %s %s", f.Path, f.Line, f.Rule, f.Level))
	}
	return r, got
}

func TestRunFindings(t *testing.T) {
	tests := []struct {
		name    string
		files   map[string]string
		targets []string
		want    []string
	}{
		{
			name: "clean notes and a tombstone",
			files: map[string]string{
				"notes/a.md": note(validated(idA), "See [[b]].\n"),
				"notes/b.md": note(validated(idB), "B.\n"),
				"notes/w.md": note("status: withdrawn\nid: "+idC+"\nstatus_at: 2026-10-01\n", ""),
				"README.md":  "Not a note.\n",
			},
		},
		{
			name: "links to revoked, unvalidated and missing notes",
			files: map[string]string{
				"notes/a.md": note(
					validated(idA),
					"Cites [[gone]], [[draft]] and [[nowhere]].\n",
				),
				"notes/gone.md": note(retracted, "Old.\n"),
				"inbox/draft.md": note(
					"status: raw\nid: "+idC+"\nclassification: internal\nexpires_at: 2026-12-01\n",
					"D.\n",
				),
			},
			targets: []string{"notes/a.md"},
			want: []string{
				"notes/a.md:7 link-dangling warning",
				"notes/a.md:7 link-revoked error",
				"notes/a.md:7 link-unvalidated warning",
			},
		},
		{
			name: "sources refs to notes",
			files: map[string]string{
				"notes/a.md": note(validated(idA)+"sources:\n  - ref: id:"+idB+
					"\n  - ref: notes/nowhere.md\n", "A.\n"),
				"notes/b.md": note(validated(idB), "B.\n"),
			},
			targets: []string{"notes/a.md"},
			want:    []string{"notes/a.md:0 link-dangling warning"},
		},
		{
			name: "a link to a note past its review_by",
			files: map[string]string{
				"notes/a.md": note(validated(idA), "Cites [[stale]].\n"),
				"notes/stale.md": note(
					"status: validated\nid: "+idB+"\nclassification: internal\nreview_by: 2026-01-01\n",
					"S.\n",
				),
			},
			targets: []string{"notes/a.md"},
			want:    []string{"notes/a.md:7 link-unvalidated warning"},
		},
		{
			name: "links elsewhere to a checked note that is revoked",
			files: map[string]string{
				"notes/gone.md": note(retracted, "Old.\n"),
				"notes/old.md":  note(validated(idA), "Cited [[gone]] before.\n"),
			},
			targets: []string{"notes/gone.md"},
			want:    []string{"notes/old.md:7 link-unvalidated warning"},
		},
		{
			name: "status rules",
			files: map[string]string{
				"notes/sup.md": note(
					"status: superseded\nid: "+idA+"\nclassification: internal\n",
					"S.\n",
				),
				"notes/dang.md": note(
					"status: superseded\nid: "+idB+"\nclassification: internal\nsuperseded_by: id:01JZ0000000000000000000000\n",
					"D.\n",
				),
				"notes/exp.md": note(
					"status: expired\nid: "+idC+"\nclassification: internal\n",
					"E.\n",
				),
				"notes/odd.md": note("status: pondering\nclassification: internal\n", "O.\n"),
			},
			want: []string{
				"notes/dang.md:0 status-fields error",
				"notes/exp.md:0 schema error",
				"notes/odd.md:0 missing-field warning",
				"notes/odd.md:0 status-unknown warning",
				"notes/sup.md:0 status-fields error",
			},
		},
		{
			name: "classification and policy",
			files: map[string]string{
				".brain/policy.yaml": "classifications: [public, internal]\n",
				"notes/a.md": note(
					"status: validated\nid: "+idA+"\nclassification: secret\nreview_by: 2027-06-01\n",
					"A.\n",
				),
				"notes/b.md": note(
					"status: validated\nid: "+idB+"\nclassification: confidential\nreview_by: 2027-06-01\n",
					"B.\n",
				),
			},
			want: []string{
				"notes/a.md:0 schema error",
				"notes/b.md:0 classification error",
			},
		},
		{
			name: "duplicate ids and stems",
			files: map[string]string{
				"notes/a.md": note(validated(idA), "A.\n"),
				"notes/b.md": note(validated(idA), "B.\n"),
				"notes/c.md": note(validated(idB), "C.\n"),
				"inbox/C.md": note(
					"status: raw\nid: "+idC+"\nclassification: internal\nexpires_at: 2026-12-01\n",
					"C.\n",
				),
			},
			want: []string{
				"inbox/C.md:0 stem-duplicate warning",
				"notes/a.md:0 id-duplicate error",
				"notes/b.md:0 id-duplicate error",
				"notes/c.md:0 stem-duplicate warning",
			},
		},
		{
			name: "provenance",
			files: map[string]string{
				"notes/bare.md": note(validated(idA)+"type: source\n", "B.\n"),
				"notes/named.md": note(
					validated(idB)+"type: source\nsources:\n  - url: https://example.com/a\n",
					"N.\n",
				),
			},
			want: []string{"notes/bare.md:0 provenance warning"},
		},
		{
			name: "missing optional fields",
			files: map[string]string{
				"notes/bare.md": note("title: Bare\n", "B.\n"),
				"inbox/raw.md":  note("title: Raw\n", "R.\n"),
			},
			want: []string{
				"inbox/raw.md:0 missing-field warning",
				"inbox/raw.md:0 missing-field warning",
				"inbox/raw.md:0 missing-field warning",
				"notes/bare.md:0 missing-field warning",
				"notes/bare.md:0 missing-field warning",
				"notes/bare.md:0 missing-field warning",
			},
		},
		{
			name: "a policy that changes levels",
			files: map[string]string{
				".brain/policy.yaml": "rules:\n  link-dangling: error\n  missing-field: off\n",
				"notes/bare.md":      note("title: Bare\n", "Cites [[nowhere]].\n"),
			},
			want: []string{"notes/bare.md:4 link-dangling error"},
		},
		{
			name: "a policy that cannot be used",
			files: map[string]string{
				".brain/policy.yaml": "rules:\n  nope: error\n",
				"notes/a.md":         note(validated(idA), "A.\n"),
			},
			want: []string{".brain/policy.yaml:0 policy error"},
		},
		{
			name: "frontmatter that does not parse",
			files: map[string]string{
				"notes/a.md": "---\ntitle: [unclosed\n---\nA.\n",
			},
			want: []string{"notes/a.md:1 schema error"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, got := runCheck(t, tt.files, tt.targets...)
			if !slices.Equal(got, tt.want) {
				t.Errorf("findings:\n got %q\nwant %q", got, tt.want)
			}
		})
	}
}

func TestRunReport(t *testing.T) {
	r, _ := runCheck(t, map[string]string{
		"notes/a.md":    note(validated(idA), "Cites [[gone]] and [[nowhere]].\n"),
		"notes/gone.md": note(retracted, "Old.\n"),
	}, "notes/a.md")
	if r.OK || r.Checked != 1 || r.Errors != 1 || r.Warnings != 1 {
		t.Fatalf("report = ok %v checked %d errors %d warnings %d, want false 1 1 1",
			r.OK, r.Checked, r.Errors, r.Warnings)
	}
	msg := r.Findings[0].Message
	if r.Findings[0].Rule != RuleLinkDangling || !strings.Contains(msg, "[[nowhere]]") {
		t.Errorf(
			"first finding = %s %q, want link-dangling naming [[nowhere]]",
			r.Findings[0].Rule,
			msg,
		)
	}
	msg = r.Findings[1].Message
	if !strings.Contains(msg, "main:notes/gone.md") || !strings.Contains(msg, "retracted") {
		t.Errorf("revoked link message = %q, want the target and its status", msg)
	}

	clean, _ := runCheck(t, map[string]string{"notes/a.md": note(validated(idA), "A.\n")})
	if !clean.OK || clean.Findings == nil {
		t.Errorf(
			"clean report = ok %v findings %v, want true and an empty list",
			clean.OK,
			clean.Findings,
		)
	}
}

func TestLoadPolicy(t *testing.T) {
	tests := []struct {
		name    string
		policy  *string
		wantErr string
		check   func(Policy) bool
	}{
		{
			name:  "missing",
			check: func(p Policy) bool { return p.Level(RuleSecret) == Error && p.Allows("restricted") },
		},
		{
			name:   "empty",
			policy: ptr(""),
			check:  func(p Policy) bool { return p.Level(RuleProvenance) == Warning },
		},
		{
			name:   "overrides",
			policy: ptr("classifications: [public]\nrules:\n  provenance: error\n"),
			check: func(p Policy) bool {
				return p.Level(RuleProvenance) == Error && p.Allows("public") &&
					!p.Allows("internal")
			},
		},
		{
			name:   "the policy rule stays an error",
			policy: ptr("rules: {}\n"),
			check:  func(p Policy) bool { return p.Level(RulePolicy) == Error },
		},
		{
			name:    "unknown key",
			policy:  ptr("rule:\n  secret: off\n"),
			wantErr: "field rule not found",
		},
		{
			name:    "unknown rule",
			policy:  ptr("rules:\n  nope: off\n"),
			wantErr: `unknown rule "nope"`,
		},
		{name: "bad level", policy: ptr("rules:\n  secret: loud\n"), wantErr: `level "loud"`},
		{
			name:    "bad classification",
			policy:  ptr("classifications: [top-secret]\n"),
			wantErr: `classification "top-secret"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			if tt.policy != nil {
				path := filepath.Join(root, filepath.FromSlash(PolicyFile))
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(*tt.policy), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			p, err := LoadPolicy(root)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("LoadPolicy error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadPolicy: %v", err)
			}
			if !tt.check(p) {
				t.Errorf("policy %+v fails the check", p)
			}
		})
	}
}

func ptr(s string) *string { return &s }
