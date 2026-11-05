package vault

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// linkFixture is a private personal vault, a second private vault, and two
// shared vaults, in that config order.
func linkFixture(t *testing.T) *Set {
	t.Helper()
	personal := newTestVault(t, map[string]string{
		"notes/mine.md":   "---\ntitle: Mine\n---\n",
		"notes/both.md":   "---\ntitle: Personal Both\n---\n",
		"notes/dup.md":    "---\ntitle: Dup Notes\n---\n",
		"inbox/dup.md":    "---\ntitle: Dup Inbox\n---\n",
		"notes/local.md":  "---\ntitle: Local\naliases: [x-shared]\n---\n",
		"notes/source.md": "---\ntitle: Source\n---\nlinks below\n",
	})
	personal.Name, personal.Default = "personal", true
	work := newTestVault(t, map[string]string{"notes/secret.md": "---\ntitle: Secret\n---\n"})
	work.Name = "work"
	team := newTestVault(t, map[string]string{
		"notes/team-guide.md": "---\ntitle: Team Guide\naliases: [The Guide]\n---\n",
		"notes/both.md":       "---\ntitle: Team Both\n---\n",
		"notes/x-shared.md":   "---\ntitle: X\n---\n",
		"notes/tsource.md":    "---\ntitle: TSource\n---\n",
	})
	team.Name, team.Shared = "team", true
	team2 := newTestVault(t, map[string]string{"notes/guide2.md": "---\ntitle: Guide Two\n---\n"})
	team2.Name, team2.Shared = "team2", true
	return NewSet(personal, work, team, team2)
}

func TestLinkResolution(t *testing.T) {
	set := linkFixture(t)
	tests := []struct {
		name   string
		from   string // vault the linking note is in
		target string
		want   string // vault:path, or "" for dangling
	}{
		{"own vault by stem", "personal", "mine", "personal:notes/mine.md"},
		{"private to shared", "personal", "team-guide", "team:notes/team-guide.md"},
		{"private own vault first", "personal", "both", "personal:notes/both.md"},
		{"own alias before a shared stem", "personal", "x-shared", "personal:notes/local.md"},
		{"private not into another private", "personal", "secret", ""},
		{"shared to private is dangling", "team", "mine", ""},
		{"shared to shared", "team", "guide2", "team2:notes/guide2.md"},
		{"shared own vault first", "team", "both", "team:notes/both.md"},
		{"stem ignores case", "personal", "Team-GUIDE", "team:notes/team-guide.md"},
		{"title", "personal", "team guide", "team:notes/team-guide.md"},
		{"alias", "personal", "THE GUIDE", "team:notes/team-guide.md"},
		{"never a prefix", "personal", "team", ""},
		{"never a prefix of a path", "personal", "notes/team", ""},
		{"path", "personal", "notes/team-guide", "team:notes/team-guide.md"},
		{"path with extension", "personal", "notes/mine.md", "personal:notes/mine.md"},
		{"stem with extension", "personal", "mine.md", "personal:notes/mine.md"},
		{"tier precedence", "personal", "dup", "personal:notes/dup.md"},
		{"path picks the tier", "personal", "inbox/dup", "personal:inbox/dup.md"},
	}
	idx, err := buildLinkIndex(set)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			from, err := set.Get(tc.from)
			if err != nil {
				t.Fatal(err)
			}
			got := ""
			if n, _ := idx.resolve(from, tc.target); n != nil {
				got = n.ref.String()
			}
			if got != tc.want {
				t.Errorf("[[%s]] from %s resolved to %q, want %q", tc.target, tc.from, got, tc.want)
			}
		})
	}
}

func TestBacklinksAcrossVaults(t *testing.T) {
	set := linkFixture(t)
	personal, _ := set.Get("personal")
	team, _ := set.Get("team")
	write := func(v *Vault, rel, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(v.Root, rel), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(personal, "notes/source.md",
		"---\ntitle: Source\n---\nSee [[team-guide]] and ![[The Guide]].\n"+
			"Code `[[team-guide]]` is not a link.\n[[nowhere]]\n")
	write(
		team,
		"notes/tsource.md",
		"---\ntitle: TSource\n---\nAlso [[Team-Guide#Setup|setup]], [[mine]].\n",
	)

	guide := Ref{Vault: team, Path: "notes/team-guide.md"}
	links, err := set.Backlinks(guide)
	if err != nil {
		t.Fatal(err)
	}
	want := []Inbound{
		{
			Vault:   "personal",
			Path:    "notes/source.md",
			Line:    4,
			Excerpt: "See [[team-guide]] and ![[The Guide]].",
		},
		{
			Vault:   "team",
			Path:    "notes/tsource.md",
			Line:    4,
			Excerpt: "Also [[Team-Guide#Setup|setup]], [[mine]].",
		},
	}
	if !reflect.DeepEqual(links.Inbound, want) {
		t.Errorf("inbound = %+v\nwant %+v", links.Inbound, want)
	}
	peek, err := set.Peek(guide, 1)
	if err != nil || peek.BacklinkCount != 2 {
		t.Errorf("peek count = %d, %v; want 2 linking notes", peek.BacklinkCount, err)
	}

	source, err := set.Backlinks(Ref{Vault: team, Path: "notes/tsource.md"})
	if err != nil {
		t.Fatal(err)
	}
	if len(source.Outbound) != 2 || source.Outbound[1].Target != "mine" ||
		!source.Outbound[1].Dangling {
		t.Errorf("shared outbound = %+v, want [[mine]] dangling", source.Outbound)
	}
}

func TestPromoteRewritesResolvedLinks(t *testing.T) {
	v := newTestVault(t, map[string]string{
		"inbox/2026-capture.md": "---\ntitle: Capture\naliases: [Cap]\nstatus: raw\n---\n" +
			"Self [[2026-capture#top]].\n",
		"notes/linker.md": "---\ntitle: Linker\n---\n" +
			"[[2026-capture]] [[2026-CAPTURE#h|text]] [[inbox/2026-capture]]\n" +
			"[[Capture]] [[Cap]] `[[2026-capture]]` [[2026-capture-old]]\n",
		"notes/other.md": "---\ntitle: Other\n---\nNo links to it.\n",
	})
	res, err := v.Promote(
		filepath.Join(v.Root, "inbox", "2026-capture.md"),
		PromoteOptions{NewTitle: "Captured Idea"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{filepath.Join(v.Root, "notes", "linker.md")}; !reflect.DeepEqual(
		res.BacklinkRewrites,
		want,
	) {
		t.Errorf("rewrites = %v, want %v", res.BacklinkRewrites, want)
	}
	data, err := os.ReadFile(filepath.Join(v.Root, "notes", "linker.md"))
	if err != nil {
		t.Fatal(err)
	}
	want := "---\ntitle: Linker\n---\n" +
		"[[captured-idea]] [[captured-idea#h|text]] [[notes/captured-idea]]\n" +
		"[[captured-idea|Capture]] [[Cap]] `[[2026-capture]]` [[2026-capture-old]]\n"
	if string(data) != want {
		t.Errorf("linker =\n%s\nwant\n%s", data, want)
	}
	moved, err := os.ReadFile(res.NewPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(moved), "Self [[captured-idea#top]].") {
		t.Errorf("the promoted note's link to itself was not retargeted:\n%s", moved)
	}
}
