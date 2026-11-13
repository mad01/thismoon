package markdown

import (
	"slices"
	"strings"
	"testing"
)

// styledFrontmatter has every shape a re-encode used to rewrite: comments,
// a flow list, quotes, a plain date, 4-space and 0-space lists and a folded
// scalar. Each line is tagged with the key it belongs to.
var styledFrontmatter = []string{
	"# kept comment",
	`title: "Quoted Title"`,
	"tags: [ai, research]",
	"created: 2024-01-01",
	"owner: alex # who owns it",
	"four:",
	"    - a",
	"    - b",
	"zero:",
	"- x",
	"- y",
	"summary: >",
	"  folded line one",
	"  folded line two",
	"status: raw",
}

func styledNote(eol string) []byte {
	return []byte(
		"---" + eol + strings.Join(styledFrontmatter, eol) + eol + "---" + eol + "Body." + eol,
	)
}

// patched parses note, applies change and renders it again.
func patched(t *testing.T, note []byte, change func(*Frontmatter)) []byte {
	t.Helper()
	fm, body, err := ParseFrontmatter(note)
	if err != nil {
		t.Fatal(err)
	}
	change(&fm)
	out, err := Render(fm, body)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestRenderPatchesOnlyChangedKeys(t *testing.T) {
	tests := []struct {
		name   string
		change func(*Frontmatter)
		// drop are the source lines the change removes; add the lines it
		// writes in place of the first dropped one, or before the fence.
		drop []string
		add  []string
	}{
		{
			name:   "unchanged",
			change: func(*Frontmatter) {},
		},
		{
			name:   "typed key changed",
			change: func(f *Frontmatter) { f.Status = "validated" },
			drop:   []string{"status: raw"},
			add:    []string{"status: validated"},
		},
		{
			name:   "quoted title keeps its quotes",
			change: func(f *Frontmatter) { f.Title = "New Title" },
			drop:   []string{`title: "Quoted Title"`},
			add:    []string{`title: "New Title"`},
		},
		{
			name:   "flow list stays flow",
			change: func(f *Frontmatter) { f.Tags = append(f.Tags, "new") },
			drop:   []string{"tags: [ai, research]"},
			add:    []string{"tags: [ai, research, new]"},
		},
		{
			name:   "plain date stays plain",
			change: func(f *Frontmatter) { f.Created = "2026-11-05" },
			drop:   []string{"created: 2024-01-01"},
			add:    []string{"created: 2026-11-05"},
		},
		{
			name: "extra key set",
			change: func(f *Frontmatter) {
				v, _ := ParseValue("sam")
				_ = f.Set("owner", v)
			},
			drop: []string{"owner: alex # who owns it"},
			add:  []string{"owner: sam"},
		},
		{
			name: "same value set again keeps the text",
			change: func(f *Frontmatter) {
				v, _ := ParseValue("[a, b]")
				_ = f.Set("four", v)
				f.Tags = []string{"ai", "research"}
			},
		},
		{
			name:   "unset key drops its lines",
			change: func(f *Frontmatter) { f.Unset("four") },
			drop:   []string{"four:", "    - a", "    - b"},
		},
		{
			name:   "new key goes before the fence",
			change: func(f *Frontmatter) { f.ID = "01ABC" },
			add:    []string{"id: 01ABC"},
		},
	}
	for _, eol := range []string{"\n", "\r\n"} {
		for _, tt := range tests {
			t.Run(tt.name+" "+strings.ReplaceAll(eol, "\r", "CR"), func(t *testing.T) {
				out := patched(t, styledNote(eol), tt.change)
				want := expectedLines(tt.drop, tt.add)
				wantNote := "---" + eol + strings.Join(
					want,
					eol,
				) + eol + "---" + eol + "Body." + eol
				if string(out) != wantNote {
					t.Errorf("rendered:\n%q\nwant:\n%q", out, wantNote)
				}
			})
		}
	}
}

// expectedLines is styledFrontmatter with drop removed and add written in
// place of the first dropped line, or at the end.
func expectedLines(drop, add []string) []string {
	var out []string
	placed := false
	for _, l := range styledFrontmatter {
		if !slices.Contains(drop, l) {
			out = append(out, l)
			continue
		}
		if !placed {
			out = append(out, add...)
			placed = true
		}
	}
	if !placed {
		out = append(out, add...)
	}
	return out
}

func TestRenderKeepsFences(t *testing.T) {
	tests := []struct {
		name, note string
	}{
		{"byte order mark", "\xEF\xBB\xBF---\ntitle: A\n---\nBody.\n"},
		{"trailing spaces", "--- \ntitle: A\n---  \nBody.\n"},
		{"dot fence", "---\ntitle: A\n...\nBody.\n"},
		{"fence at end of file", "---\ntitle: A\n---"},
		{"empty block", "---\n---\nBody.\n"},
		{"comment only", "---\n# nothing yet\n---\nBody.\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if out := patched(t, []byte(tt.note), func(*Frontmatter) {}); string(out) != tt.note {
				t.Errorf("unchanged render = %q, want %q", out, tt.note)
			}
			fm, _, err := ParseFrontmatter([]byte(tt.note))
			if err != nil {
				t.Fatal(err)
			}
			if fm.Title == "" && !strings.Contains(tt.note, "title") {
				return
			}
			if fm.Title != "A" {
				t.Errorf("title = %q, want A: the fence was not recognized", fm.Title)
			}
		})
	}
}

func TestRenderAppendsAfterFenceAtEndOfFile(t *testing.T) {
	fm, _, err := ParseFrontmatter([]byte("---\r\ntitle: A\r\n---"))
	if err != nil {
		t.Fatal(err)
	}
	out, err := Render(fm, []byte("Body.\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	if want := "---\r\ntitle: A\r\n---\r\nBody.\r\n"; string(out) != want {
		t.Errorf("render = %q, want %q", out, want)
	}
}

func TestRenderFallsBackWhenPatchCannotHold(t *testing.T) {
	tests := []struct {
		name, note string
		change     func(*Frontmatter)
		want       string
	}{
		{
			name:   "flow mapping",
			note:   "---\n{title: A, status: raw}\n---\n",
			change: func(f *Frontmatter) { f.Status = "validated" },
			want:   "---\ntitle: A\nstatus: validated\n---\n",
		},
		{
			name:   "key repeated",
			note:   "---\nowner: a\nowner: b\n---\n",
			change: func(f *Frontmatter) { f.Status = "raw" },
			want:   "---\nowner: b\nstatus: raw\n---\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if out := patched(t, []byte(tt.note), tt.change); string(out) != tt.want {
				t.Errorf("render = %q, want %q", out, tt.want)
			}
		})
	}
}

func TestNewFrontmatterIsTwoSpaceYAML(t *testing.T) {
	fm := Frontmatter{Title: "T", Tags: []string{"a", "b"}, Created: "2026-11-05"}
	out, err := Render(fm, []byte("Body.\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := "---\ntitle: T\ntags:\n  - a\n  - b\ncreated: \"2026-11-05\"\n---\nBody.\n"
	if string(out) != want {
		t.Errorf("render = %q, want %q", out, want)
	}
}

func TestUnclosed(t *testing.T) {
	tests := []struct {
		note string
		want bool
	}{
		{"---\ntitle: A\nBody.\n", true},
		{"\xEF\xBB\xBF---\ntitle: A\n", true},
		{"---", true},
		{"---\ntitle: A\n---\n", false},
		{"No frontmatter.\n", false},
		{"----\nnot a fence\n", false},
	}
	for _, tt := range tests {
		if got := Unclosed([]byte(tt.note)); got != tt.want {
			t.Errorf("Unclosed(%q) = %v, want %v", tt.note, got, tt.want)
		}
	}
}
