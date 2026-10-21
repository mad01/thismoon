package settings

import (
	"slices"
	"testing"

	"github.com/mad01/thismoon/tools/agent-sync/internal/pyjson"
)

// The tables below port the unit tests in the reference
// test_sync_settings.py (MergeOverlayTest, LiveTemplateMergeTest and
// PruneTest). Expected values are the reference functions' output, printed
// with json.dumps, so they also pin key order, which the Python tests'
// dict comparisons did not check. The end-to-end tests live beside the CLI:
// internal/cli/settings_test.go and testdata/cases.

func obj(t *testing.T, text string) *pyjson.Object {
	t.Helper()
	v, err := pyjson.Loads([]byte(text))
	if err != nil {
		t.Fatalf("Loads(%s): %v", text, err)
	}
	o, ok := v.(*pyjson.Object)
	if !ok {
		t.Fatalf("Loads(%s) is not an object", text)
	}
	return o
}

type mergeCase struct {
	name, base, overlay, want string
}

// MergeOverlayTest: the local overlay merge unions permission lists and
// replaces other arrays.
var mergeOverlayCases = []mergeCase{
	{
		"overlay deny unions with template deny",
		`{"permissions": {"deny": ["Bash(a)"]}}`,
		`{"permissions": {"deny": ["mcp__x__*"]}}`,
		`{"permissions": {"deny": ["Bash(a)", "mcp__x__*"]}}`,
	},
	{
		"template-only entries survive",
		`{"permissions": {"deny": ["Bash(a)", "Bash(b)"]}}`,
		`{"permissions": {"deny": ["mcp__x__*"]}}`,
		`{"permissions": {"deny": ["Bash(a)", "Bash(b)", "mcp__x__*"]}}`,
	},
	{
		"overlay-only entries survive",
		`{"permissions": {"allow": ["Bash(a)"]}}`,
		`{"permissions": {"allow": ["mcp__x__*", "mcp__y__*"]}}`,
		`{"permissions": {"allow": ["Bash(a)", "mcp__x__*", "mcp__y__*"]}}`,
	},
	{
		"template first then overlay additions, no duplicates",
		`{"permissions": {"ask": ["t1", "shared", "t2", "t1"]}}`,
		`{"permissions": {"ask": ["o1", "shared", "o2", "o1"]}}`,
		`{"permissions": {"ask": ["t1", "shared", "t2", "o1", "o2"]}}`,
	},
	{
		"all three permission lists union",
		`{"permissions": {"allow": ["a"], "deny": ["d"], "ask": ["k"]}}`,
		`{"permissions": {"allow": ["a2"], "deny": ["d2"], "ask": ["k2"]}}`,
		`{"permissions": {"allow": ["a", "a2"], "deny": ["d", "d2"], "ask": ["k", "k2"]}}`,
	},
	{
		"additionalDirectories union",
		`{"permissions": {"additionalDirectories": ["~/code", "/tmp"]}}`,
		`{"permissions": {"additionalDirectories": ["~/Workspaces", "/tmp"]}}`,
		`{"permissions": {"additionalDirectories": ["~/code", "/tmp", "~/Workspaces"]}}`,
	},
	{
		"non-permission arrays in overlay replace",
		`{"hooks": {"Stop": ["template hook"]}, "autoMode": {"environment": ["template line"]}}`,
		`{"hooks": {"Stop": ["overlay hook"]}, "autoMode": {"environment": ["overlay line"]}}`,
		`{"hooks": {"Stop": ["overlay hook"]}, "autoMode": {"environment": ["overlay line"]}}`,
	},
	{
		"objects still merge additively",
		`{"enabledPlugins": {"one": true}}`,
		`{"enabledPlugins": {"two": true}}`,
		`{"enabledPlugins": {"one": true, "two": true}}`,
	},
}

func TestMergeOverlay(t *testing.T) {
	for _, tc := range mergeOverlayCases {
		t.Run(tc.name, func(t *testing.T) {
			got := pyjson.Compact(mergeOverlay(obj(t, tc.base), obj(t, tc.overlay)))
			if got != tc.want {
				t.Errorf("mergeOverlay:\n got %s\nwant %s", got, tc.want)
			}
		})
	}
}

func TestMergeOverlayDoesNotMutateInputs(t *testing.T) {
	base := obj(t, `{"permissions": {"deny": ["Bash(a)"]}}`)
	overlay := obj(t, `{"permissions": {"deny": ["mcp__x__*"]}}`)
	mergeOverlay(base, overlay)
	if got := pyjson.Compact(base); got != `{"permissions": {"deny": ["Bash(a)"]}}` {
		t.Errorf("base mutated: %s", got)
	}
	if got := pyjson.Compact(overlay); got != `{"permissions": {"deny": ["mcp__x__*"]}}` {
		t.Errorf("overlay mutated: %s", got)
	}
}

// LiveTemplateMergeTest: the live <- template merge replaces arrays and
// keeps local-only keys.
func TestDeepMergeLiveTemplate(t *testing.T) {
	cases := []mergeCase{
		{
			"template replaces live arrays",
			`{"permissions": {"deny": ["session-added", "Bash(a)"]}}`,
			`{"permissions": {"deny": ["Bash(a)", "Bash(b)"]}}`,
			`{"permissions": {"deny": ["Bash(a)", "Bash(b)"]}}`,
		},
		{
			"local-only keys are preserved",
			`{"permissions": {"deny": ["x"]}, "localOnly": 1}`,
			`{"permissions": {"deny": ["y"]}}`,
			`{"permissions": {"deny": ["y"]}, "localOnly": 1}`,
		},
		{
			// Not in the Python tests: permission lists only union in the
			// overlay merge, never in the template merge.
			"permission lists replace in the template merge",
			`{"permissions": {"allow": ["live"]}}`,
			`{"permissions": {"allow": ["template"]}}`,
			`{"permissions": {"allow": ["template"]}}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := pyjson.Compact(deepMerge(obj(t, tc.base), obj(t, tc.overlay), nil, nil))
			if got != tc.want {
				t.Errorf("deepMerge:\n got %s\nwant %s", got, tc.want)
			}
		})
	}
}

// PruneTest: retired key paths leave the live data before the merge.
func TestPrune(t *testing.T) {
	cases := []struct {
		name, live string
		paths      []KeyPath
		want       string
	}{
		{
			"prune removes the key",
			`{"hooks": {"Stop": ["keep"], "Notification": ["old"]}}`,
			[]KeyPath{{"hooks", "Notification"}},
			`{"hooks": {"Stop": ["keep"]}}`,
		},
		{
			"absent path is a no-op: missing leaf",
			`{"hooks": {"Stop": ["keep"]}, "model": "x"}`,
			[]KeyPath{{"hooks", "Notification"}},
			`{"hooks": {"Stop": ["keep"]}, "model": "x"}`,
		},
		{
			"absent path is a no-op: missing parent",
			`{"hooks": {"Stop": ["keep"]}, "model": "x"}`,
			[]KeyPath{{"enabledPlugins", "old@x"}},
			`{"hooks": {"Stop": ["keep"]}, "model": "x"}`,
		},
		{
			"absent path is a no-op: parent is a scalar",
			`{"hooks": {"Stop": ["keep"]}, "model": "x"}`,
			[]KeyPath{{"model", "nested"}},
			`{"hooks": {"Stop": ["keep"]}, "model": "x"}`,
		},
		{
			"three-level path removes only the leaf",
			`{"a": {"b": {"c": 1, "d": 2}, "e": 3}}`,
			[]KeyPath{{"a", "b", "c"}},
			`{"a": {"b": {"d": 2}, "e": 3}}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			live := obj(t, tc.live)
			if got := pyjson.Compact(prune(live, tc.paths)); got != tc.want {
				t.Errorf("prune:\n got %s\nwant %s", got, tc.want)
			}
			if got := pyjson.Compact(live); got != tc.live {
				t.Errorf("prune mutated its input: %s", got)
			}
		})
	}
}

func TestPruneConflicts(t *testing.T) {
	cases := []struct {
		name, template string
		paths, want    []KeyPath
	}{
		{
			"conflicts are paths the template still sets",
			`{"hooks": {"Stop": []}}`,
			[]KeyPath{{"hooks", "Stop"}, {"hooks", "Notification"}, {"enabledPlugins", "x"}},
			[]KeyPath{{"hooks", "Stop"}},
		},
		{
			"child of a template object: absent child does not conflict",
			`{"statusLine": {"type": "command"}}`,
			[]KeyPath{{"statusLine", "old"}},
			nil,
		},
		{
			"child of a template object: present child conflicts",
			`{"statusLine": {"type": "command"}}`,
			[]KeyPath{{"statusLine", "type"}},
			[]KeyPath{{"statusLine", "type"}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := pruneConflicts(obj(t, tc.template), tc.paths)
			if !slices.EqualFunc(got, tc.want, slices.Equal) {
				t.Errorf("pruneConflicts = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestIsUnderComparesKeysNotDottedNames(t *testing.T) {
	paths := []KeyPath{{"modelSettings", "us.anthropic.old"}}
	cases := []struct {
		keys  KeyPath
		paths []KeyPath
		want  bool
	}{
		{KeyPath{"modelSettings", "us.anthropic.old"}, paths, true},
		{KeyPath{"modelSettings", "us.anthropic.old", "x"}, paths, true},
		{KeyPath{"modelSettings", "us", "anthropic", "old"}, paths, false},
		{KeyPath{"a.b", "c"}, []KeyPath{{"a", "b.c"}}, false},
		{KeyPath{"modelSettings"}, paths, false},
	}
	for _, tc := range cases {
		if got := isUnder(tc.keys, tc.paths); got != tc.want {
			t.Errorf("isUnder(%q, %q) = %v, want %v", tc.keys, tc.paths, got, tc.want)
		}
	}
}
