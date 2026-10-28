package store

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

// TestParseTags: a list keeps its valid tags, trimmed, deduplicated and
// sorted, and only the first MaxSessionTags of them.
func TestParseTags(t *testing.T) {
	tests := []struct {
		name string
		list string
		want []string
	}{
		{"unset", "", nil},
		{"one", "eval", []string{"eval"}},
		{"trimmed and sorted", " eval , ci,nightly-2 ", []string{"ci", "eval", "nightly-2"}},
		{"repeats once", "eval,eval,ci,eval", []string{"ci", "eval"}},
		{
			"invalid ones dropped",
			"Eval,-x,a_b,ok,a b,é," + strings.Repeat("x", MaxSessionTagLen+1),
			[]string{"ok"},
		},
		{
			"longest kept",
			strings.Repeat("x", MaxSessionTagLen),
			[]string{strings.Repeat("x", MaxSessionTagLen)},
		},
		{"none valid", ",, ,UPPER", nil},
		{
			"capped after sorting",
			"j,i,h,g,f,e,d,c,b,a",
			[]string{"a", "b", "c", "d", "e", "f", "g", "h"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ParseTags(tc.list); !slices.Equal(got, tc.want) ||
				(got == nil) != (tc.want == nil) {
				t.Errorf("ParseTags(%q) = %#v, want %#v", tc.list, got, tc.want)
			}
		})
	}
}

// TestMergeTags: a merge adds the valid tags a session lacks while there is
// room, and never drops one it holds.
func TestMergeTags(t *testing.T) {
	full := []string{"a1", "a2", "a3", "a4", "a5", "a6", "a7", "z"}
	tests := []struct {
		name     string
		cur, add []string
		want     []string
	}{
		{"nothing", nil, nil, nil},
		{"fills", nil, []string{"eval"}, []string{"eval"}},
		{"unions", []string{"eval"}, []string{"ci", "eval"}, []string{"ci", "eval"}},
		{"drops invalid", []string{"eval"}, []string{"Bad"}, []string{"eval"}},
		{"keeps what it holds when full", full, []string{"a0"}, full},
		{
			"room for one",
			full[:7],
			[]string{"b", "a0"},
			[]string{"a0", "a1", "a2", "a3", "a4", "a5", "a6", "a7"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cur := slices.Clone(tc.cur)
			got := MergeTags(cur, tc.add)
			if !slices.Equal(got, tc.want) {
				t.Errorf("MergeTags(%v, %v) = %v, want %v", tc.cur, tc.add, got, tc.want)
			}
			if !slices.Equal(cur, tc.cur) {
				t.Errorf("MergeTags changed its input to %v", cur)
			}
		})
	}
}

// TestSessionJSON: a session's location, entrypoint and tags round-trip,
// and a session without them leaves them out.
func TestSessionJSON(t *testing.T) {
	at := time.Date(2026, 10, 28, 9, 0, 0, 0, time.UTC)
	bare := Session{ID: "s1", Harness: "claude-code", Cwd: "/w", StartedAt: at}
	raw, err := json.Marshal(bare)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"location"`, `"entrypoint"`, `"tags"`} {
		if strings.Contains(string(raw), key) {
			t.Errorf("bare session JSON %s has %s", raw, key)
		}
	}
	full := bare
	full.Location = Location{
		Repo:      "example.com/o/r",
		Worktree:  "/w",
		Canonical: "/w",
		Branch:    "main",
	}
	full.Entrypoint = "sdk-cli"
	full.Tags = []string{"eval"}
	raw, err = json.Marshal(full)
	if err != nil {
		t.Fatal(err)
	}
	var back Session
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(back, full) {
		t.Errorf("round trip = %+v, want %+v", back, full)
	}
	for _, key := range []string{`"location":{"repo":"example.com/o/r"`, `"entrypoint":"sdk-cli"`, `"tags":["eval"]`} {
		if !strings.Contains(string(raw), key) {
			t.Errorf("session JSON %s lacks %s", raw, key)
		}
	}
}
