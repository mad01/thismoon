package markdown

import (
	"reflect"
	"strings"
	"testing"
)

const unknownKeysNote = `---
owner: alex
title: Source Note
classification: internal
sources:
  - url: https://example.com/a
    note: primary
  - url: https://example.com/b
review:
  by: sam
  on: 2026-03-01
tags:
  - ai
status: raw
---
Body.
`

// keyOrder returns the top-level frontmatter keys of a rendered note in order.
func keyOrder(t *testing.T, rendered []byte) []string {
	t.Helper()
	raw, _ := Split(rendered)
	var keys []string
	for _, line := range strings.Split(string(raw), "\n") {
		if line == "" || strings.HasPrefix(line, " ") || strings.HasPrefix(line, "-") {
			continue
		}
		key, _, _ := strings.Cut(line, ":")
		keys = append(keys, key)
	}
	return keys
}

func TestRoundTripKeepsUnknownKeys(t *testing.T) {
	wantExtra := map[string]any{
		"owner":          "alex",
		"classification": "internal",
		"sources": []any{
			map[string]any{"url": "https://example.com/a", "note": "primary"},
			map[string]any{"url": "https://example.com/b"},
		},
		"review": map[string]any{"by": "sam", "on": "2026-03-01"},
	}
	tests := []struct {
		name      string
		mutate    func(*Frontmatter)
		wantOrder []string
	}{
		{
			name:   "unchanged",
			mutate: func(*Frontmatter) {},
			wantOrder: []string{
				"owner",
				"title",
				"classification",
				"sources",
				"review",
				"tags",
				"status",
			},
		},
		{
			name:   "typed field changed in place",
			mutate: func(f *Frontmatter) { f.Status = "validated" },
			wantOrder: []string{
				"owner",
				"title",
				"classification",
				"sources",
				"review",
				"tags",
				"status",
			},
		},
		{
			name:   "new typed field appended after source keys",
			mutate: func(f *Frontmatter) { f.Updated = "2026-04-01" },
			wantOrder: []string{
				"owner", "title", "classification", "sources", "review", "tags", "status", "updated",
			},
		},
		{
			name:      "cleared typed field dropped",
			mutate:    func(f *Frontmatter) { f.Tags = nil },
			wantOrder: []string{"owner", "title", "classification", "sources", "review", "status"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fm, body, err := ParseFrontmatter([]byte(unknownKeysNote))
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			tc.mutate(&fm)
			out, err := Render(fm, body)
			if err != nil {
				t.Fatalf("render: %v", err)
			}
			if got := keyOrder(t, out); !reflect.DeepEqual(got, tc.wantOrder) {
				t.Errorf("key order = %v, want %v\n%s", got, tc.wantOrder, out)
			}
			again, gotBody, err := ParseFrontmatter(out)
			if err != nil {
				t.Fatalf("re-parse: %v", err)
			}
			extra, err := again.ExtraValues()
			if err != nil {
				t.Fatalf("ExtraValues: %v", err)
			}
			if !reflect.DeepEqual(extra, wantExtra) {
				t.Errorf("extra = %#v, want %#v", extra, wantExtra)
			}
			if again.Title != "Source Note" {
				t.Errorf("title = %q", again.Title)
			}
			if string(gotBody) != "Body.\n" {
				t.Errorf("body = %q", gotBody)
			}
		})
	}
}

// TestEditOneKeyKeepsTheRestByteIdentical pins the reviewed-diff contract:
// changing one key of a 2-space-indented frontmatter rewrites that key's
// lines and nothing else.
func TestEditOneKeyKeepsTheRestByteIdentical(t *testing.T) {
	set := func(key, value string) func(*Frontmatter) error {
		return func(f *Frontmatter) error {
			node, err := ParseValue(value)
			if err != nil {
				return err
			}
			return f.Set(key, node)
		}
	}
	tests := []struct {
		name   string
		mutate func(*Frontmatter) error
		want   string
	}{
		{
			name:   "unchanged",
			mutate: func(*Frontmatter) error { return nil },
			want:   unknownKeysNote,
		},
		{
			name:   "typed key",
			mutate: set("status", "validated"),
			want:   strings.Replace(unknownKeysNote, "status: raw", "status: validated", 1),
		},
		{
			name:   "unknown key",
			mutate: set("classification", "public"),
			want: strings.Replace(
				unknownKeysNote, "classification: internal", "classification: public", 1,
			),
		},
		{
			name:   "nested unknown key replaced",
			mutate: set("review", "{by: kim}"),
			want: strings.Replace(
				unknownKeysNote, "review:\n  by: sam\n  on: 2026-03-01\n", "review: {by: kim}\n", 1,
			),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fm, body, err := ParseFrontmatter([]byte(unknownKeysNote))
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if err := tc.mutate(&fm); err != nil {
				t.Fatalf("mutate: %v", err)
			}
			out, err := Render(fm, body)
			if err != nil {
				t.Fatalf("render: %v", err)
			}
			if string(out) != tc.want {
				t.Errorf("render =\n%s\nwant\n%s", out, tc.want)
			}
		})
	}
}

func TestSetUnset(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		value   string // YAML; empty means Unset
		wantErr bool
		check   func(t *testing.T, f Frontmatter)
	}{
		{
			name: "typed string", key: "title", value: "New",
			check: func(t *testing.T, f Frontmatter) {
				if f.Title != "New" {
					t.Errorf("title = %q", f.Title)
				}
			},
		},
		{
			name: "typed list", key: "tags", value: "[a, b]",
			check: func(t *testing.T, f Frontmatter) {
				if !reflect.DeepEqual(f.Tags, []string{"a", "b"}) {
					t.Errorf("tags = %v", f.Tags)
				}
			},
		},
		{name: "typed string rejects a list", key: "title", value: "[a]", wantErr: true},
		{name: "typed list rejects a mapping", key: "tags", value: "{a: b}", wantErr: true},
		{
			name: "unknown nested value", key: "sources", value: "[{url: https://x}]",
			check: func(t *testing.T, f Frontmatter) {
				got, _ := f.ExtraValues()
				want := []any{map[string]any{"url": "https://x"}}
				if !reflect.DeepEqual(got["sources"], want) {
					t.Errorf("sources = %#v", got["sources"])
				}
			},
		},
		{
			name: "unknown key replaced in place", key: "owner", value: "sam",
			check: func(t *testing.T, f Frontmatter) {
				if len(f.Extra) != 1 || f.Extra[0].Key != "owner" ||
					f.Extra[0].Value.Value != "sam" {
					t.Errorf("extra = %+v", f.Extra)
				}
			},
		},
		{
			name: "null unsets typed", key: "status", value: "null",
			check: func(t *testing.T, f Frontmatter) {
				if f.Status != "" {
					t.Errorf("status = %q", f.Status)
				}
			},
		},
		{
			name: "unset typed", key: "status",
			check: func(t *testing.T, f Frontmatter) {
				if f.Status != "" {
					t.Errorf("status = %q", f.Status)
				}
			},
		},
		{
			name: "unset extra", key: "owner",
			check: func(t *testing.T, f Frontmatter) {
				if len(f.Extra) != 0 {
					t.Errorf("extra = %+v", f.Extra)
				}
			},
		},
		{
			name: "unset absent key is a no-op", key: "nope",
			check: func(t *testing.T, f Frontmatter) {
				if len(f.Extra) != 1 || f.Status != "raw" {
					t.Errorf("frontmatter changed: %+v", f)
				}
			},
		},
		{name: "empty key", key: "", value: "x", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fm, _, err := ParseFrontmatter([]byte("---\nowner: alex\nstatus: raw\n---\n"))
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if tc.value == "" && tc.key != "" {
				fm.Unset(tc.key)
			} else {
				node, err := ParseValue(tc.value)
				if err != nil {
					t.Fatalf("ParseValue: %v", err)
				}
				err = fm.Set(tc.key, node)
				if (err != nil) != tc.wantErr {
					t.Fatalf("Set err = %v, wantErr %v", err, tc.wantErr)
				}
				if err != nil {
					return
				}
			}
			tc.check(t, fm)
		})
	}
}

func TestSetDoesNotLeakIntoCopies(t *testing.T) {
	orig, _, err := ParseFrontmatter([]byte("---\nowner: alex\n---\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	cp := orig
	node, err := ParseValue("sam")
	if err != nil {
		t.Fatalf("ParseValue: %v", err)
	}
	if err := cp.Set("owner", node); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if got := orig.Extra[0].Value.Value; got != "alex" {
		t.Errorf("original owner changed to %q", got)
	}
}

func TestParseValue(t *testing.T) {
	tests := []struct {
		in      string
		want    string // ShortTag of the parsed node
		wantErr bool
	}{
		{in: "alex", want: "!!str"},
		{in: "3", want: "!!int"},
		{in: "[a, b]", want: "!!seq"},
		{in: "{url: https://x}", want: "!!map"},
		{in: "", wantErr: true},
		{in: "[unclosed", wantErr: true},
		{in: "a\n---\nb", wantErr: true},
	}
	for _, tc := range tests {
		node, err := ParseValue(tc.in)
		if (err != nil) != tc.wantErr {
			t.Errorf("ParseValue(%q) err = %v, wantErr %v", tc.in, err, tc.wantErr)
			continue
		}
		if err == nil && node.ShortTag() != tc.want {
			t.Errorf("ParseValue(%q) tag = %s, want %s", tc.in, node.ShortTag(), tc.want)
		}
	}
}

func TestParseFrontmatterRejectsNonMapping(t *testing.T) {
	if _, _, err := ParseFrontmatter([]byte("---\njust a string\n---\nbody\n")); err == nil {
		t.Error("want an error for scalar frontmatter")
	}
}
