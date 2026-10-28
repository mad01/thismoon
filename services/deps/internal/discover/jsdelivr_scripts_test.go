package discover

import (
	"os"
	"path/filepath"
	"testing"
)

// writeRepoFile writes content to rel under root, creating its directories.
func writeRepoFile(t *testing.T, root, rel string, content []byte) string {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// testdata/cache-assets.sh is a copy of present's asset script: ten libraries
// fetched from jsDelivr by *_VERSION variables, xterm's JS and CSS under one
// version, *_SHA256 pins beside them, and a Google Fonts URL.
func TestJSDelivrScriptsFindsEveryLibraryInPresentsAssetScript(t *testing.T) {
	raw, err := os.ReadFile("testdata/cache-assets.sh")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	script := writeRepoFile(t, root, "services/present/scripts/cache-assets.sh", raw)
	writeRepoFile(t, root, "services/present/THIRD-PARTY.md",
		[]byte("Each file comes from `https://cdn.jsdelivr.net/npm/d3@7.9.0/`.\n"))

	pkgs, err := JSDelivrScripts{}.Discover(root, Options{})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	want := []string{
		"cytoscape@3.31.0",
		"dagre@0.8.5",
		"cytoscape-dagre@2.5.0",
		"elkjs@0.12.0",
		"cytoscape-elk@2.3.0",
		"chart.js@4.4.6",
		"chartjs-chart-sankey@0.15.3",
		"d3@7.9.0",
		"@xterm/xterm@6.0.0",
		"@xterm/addon-fit@0.11.0",
	}
	if len(pkgs) != len(want) {
		t.Fatalf("got %d packages, want %d: %+v", len(pkgs), len(want), pkgs)
	}
	for i, p := range pkgs {
		if got := p.Name + "@" + p.Version; got != want[i] {
			t.Errorf("pkgs[%d] = %s, want %s", i, got, want[i])
		}
		if p.Ecosystem != "npm" || !p.Direct || !p.Imported || p.ManifestPath != script ||
			len(p.Advisories) != 0 {
			t.Errorf("%s = %+v, want a direct imported npm dep from the script", p.Name, p)
		}
	}
}

func TestParseJSDelivrScript(t *testing.T) {
	tests := []struct {
		name   string
		script string
		want   []string
	}{
		{
			name:   "literal version",
			script: `curl -o x.js https://cdn.jsdelivr.net/npm/lodash@4.17.21/lodash.min.js`,
			want:   []string{"lodash@4.17.21"},
		},
		{
			name: "variable forms and quoting",
			script: "export A=1.0.0\nreadonly B='2.0.0-rc.1'\n  C=\"3.0.0\"\n" +
				`u1="https://cdn.jsdelivr.net/npm/a@$A/a.js"` + "\n" +
				`u2='https://cdn.jsdelivr.net/npm/@s/b@${B}'` + "\n" +
				`u3=https://cdn.jsdelivr.net/npm/c@${C}/c.js`,
			want: []string{"a@1.0.0", "@s/b@2.0.0-rc.1", "c@3.0.0"},
		},
		{
			name: "no exact version pinned",
			script: "V=${OTHER:-1.0.0}\n" +
				"https://cdn.jsdelivr.net/npm/range@3/x.js\n" +
				"https://cdn.jsdelivr.net/npm/tag@latest/x.js\n" +
				"https://cdn.jsdelivr.net/npm/unset@${NOPE}/x.js\n" +
				"https://cdn.jsdelivr.net/npm/expands@${V}/x.js\n" +
				"https://cdn.jsdelivr.net/npm/bare/x.js\n",
		},
		{
			name: "same version twice is one package",
			script: "V=6.0.0\n" +
				"https://cdn.jsdelivr.net/npm/@xterm/xterm@${V}/lib/xterm.js\n" +
				"https://cdn.jsdelivr.net/npm/@xterm/xterm@${V}/css/xterm.css\n",
			want: []string{"@xterm/xterm@6.0.0"},
		},
		{
			name: "same literal assigned twice resolves",
			script: "V=1.0.0\nV=\"1.0.0\"; X=4.0.0; curl https://cdn.jsdelivr.net/npm/x@$X/x.js\n" +
				"curl https://cdn.jsdelivr.net/npm/a@$V/a.js\n",
			want: []string{"x@4.0.0", "a@1.0.0"},
		},
		{
			name: "variable assigned two versions resolves nowhere",
			script: "VERSION=1.0.0\n" +
				"curl https://cdn.jsdelivr.net/npm/a@${VERSION}/a.js\n" +
				"VERSION=2.0.0\n" +
				"curl https://cdn.jsdelivr.net/npm/b@${VERSION}/b.js\n" +
				"W=1.0.0\n[ -n \"$NEXT\" ] && W=2.0.0\n" +
				"curl https://cdn.jsdelivr.net/npm/c@$W/c.js\n" +
				"Y=1.0.0\nY=$(cat VERSION)\n" +
				"curl https://cdn.jsdelivr.net/npm/d@$Y/d.js\n",
		},
		{
			name: "value runs on past the literal",
			script: "V=1.2.3$SUFFIX\nW=\"4.5.6\"-rc.1\n" +
				"curl https://cdn.jsdelivr.net/npm/a@$V/a.js\n" +
				"curl https://cdn.jsdelivr.net/npm/b@${W}/b.js\n",
		},
		{
			name: "comment lines are not downloads",
			script: "#!/bin/sh\n" +
				"# was https://cdn.jsdelivr.net/npm/old@0.1.0/old.js\n" +
				"  # V=0.9.0 before the upgrade\n" +
				"V=1.0.0\n" +
				"curl https://cdn.jsdelivr.net/npm/new@$V/new.js\n",
			want: []string{"new@1.0.0"},
		},
		{
			name:   "other CDN paths",
			script: "https://cdn.jsdelivr.net/gh/user/repo@1.0.0/x.js\nhttps://unpkg.com/a@1.0.0/a.js\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pkgs := parseJSDelivrScript(tt.script, "/r/fetch.sh")
			var got []string
			for _, p := range pkgs {
				got = append(got, p.Name+"@"+p.Version)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("got %v, want %v", got, tt.want)
					break
				}
			}
		})
	}
}
