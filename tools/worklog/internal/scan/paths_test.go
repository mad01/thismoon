package scan

import (
	"encoding/json"
	"slices"
	"testing"
)

func TestPathTokens(t *testing.T) {
	const widgets = "/Users/example/code/src/github.com/acme/widgets"
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"cd in a shell chain", "cd " + widgets + " && make build", []string{widgets}},
		{"git -C with quotes", `git -C "` + widgets + `" status`, []string{widgets}},
		{"flag assignment and semicolon", "ls --dir=/tmp/x;echo done", []string{"/tmp/x"}},
		{
			"whole value is a path",
			widgets + "/internal/scan/scan.go",
			[]string{widgets + "/internal/scan/scan.go"},
		},
		{"trailing prose dot dropped", "work in " + widgets + ".", []string{widgets}},
		{"go package pattern", "go test " + widgets + "/...", []string{widgets + "/"}},
		{
			"tilde root kept",
			"cd ~/code/src/github.com/acme/widgets",
			[]string{"~/code/src/github.com/acme/widgets"},
		},
		{
			"double-quoted path with a space",
			`cd "` + widgets + ` two" && ls`,
			[]string{widgets + " two"},
		},
		{
			"single-quoted path with a space",
			`ls '` + widgets + ` two'`,
			[]string{widgets + " two"},
		},
		{
			"flag assignment with a quoted path",
			`--dir="` + widgets + ` two"`,
			[]string{widgets + " two"},
		},
		{
			"apostrophe in prose keeps the path",
			"the repo's README at " + widgets + "/README.md",
			[]string{widgets + "/README.md"},
		},
		{"url is not a path", "see https://github.com/acme/widgets/pull/5", nil},
		{"repo slug is not a path", "acme/widgets", nil},
		{"two paths", "cp " + widgets + "/a /tmp/b", []string{widgets + "/a", "/tmp/b"}},
		{"empty", "", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := pathTokens(tc.in); !slices.Equal(got, tc.want) {
				t.Errorf("pathTokens(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestCheckoutDir(t *testing.T) {
	def := Config{}.WithDefaults()
	const widgets = "/Users/example/code/src/github.com/acme/widgets"
	cases := []struct {
		name string
		cfg  Config
		path string
		want string
	}{
		{"file under a gopath checkout", def, widgets + "/internal/scan/scan.go", widgets},
		{"the checkout itself", def, widgets, widgets},
		{"checkout with trailing slash", def, widgets + "/", widgets},
		{"org dir only", def, "/Users/example/code/src/github.com/acme", ""},
		{"host segment without a dot", def, "/Users/example/code/src/local/repo/a.go", ""},
		{
			"internal host checkout", def, "/Users/example/code/src/git.example.test/team/svc/x.go",
			"/Users/example/code/src/git.example.test/team/svc",
		},
		{
			"workspace file", def, "/Users/example/workspace/billing-api/cmd/main.go",
			"/Users/example/workspace/billing-api",
		},
		{
			"repo directly under a marker", def, "/Users/example/code/worklog/MAD-1/CONTEXT.md",
			"/Users/example/code/worklog",
		},
		{"tmp dir", def, "/tmp/x/a.go", ""},
		{
			"tilde root",
			def,
			"~/code/src/github.com/acme/widgets/a.go",
			"~/code/src/github.com/acme/widgets",
		},
		{
			"marker gate applies before the root rule",
			Config{RepoPathMarkers: []string{"/repos/"}}.WithDefaults(),
			widgets + "/a.go",
			"",
		},
		{
			"configured marker",
			Config{RepoPathMarkers: []string{"/repos/"}}.WithDefaults(),
			"/Users/example/repos/thing/a.go",
			"/Users/example/repos/thing",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := checkoutDir(tc.cfg, tc.path); got != tc.want {
				t.Errorf("checkoutDir(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}

func TestToolInputPaths(t *testing.T) {
	const widgets = "/Users/example/code/src/github.com/acme/widgets"
	cases := []struct {
		name    string
		content string
		want    []string
	}{
		{
			name: "nested objects and arrays, non-strings skipped",
			content: `[{"type":"text","text":"ignored /tmp/not-a-tool-input"},
				{"type":"tool_use","id":"t1","name":"X","input":{"a":{"b":["` + widgets + `/a.go",3,true]},"c":"cd /tmp/y","d":null}}]`,
			want: []string{widgets + "/a.go", "/tmp/y"},
		},
		{
			name:    "no tool_use blocks",
			content: `[{"type":"text","text":"just prose about ` + widgets + `"}]`,
			want:    nil,
		},
		{
			name:    "string content",
			content: `"plain string content"`,
			want:    nil,
		},
		{
			name:    "malformed input object",
			content: `[{"type":"tool_use","id":"t1","name":"X","input":"not an object"}]`,
			want:    nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := toolInputPaths(json.RawMessage(tc.content))
			if !slices.Equal(got, tc.want) {
				t.Errorf("toolInputPaths = %v, want %v", got, tc.want)
			}
		})
	}
}
