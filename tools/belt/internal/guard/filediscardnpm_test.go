package guard

import (
	"fmt"
	"testing"
)

// The rows below run package.json scripts and just recipes through
// file-discard, as Check reads them: belt reads the build file, and
// nothing runs.

// TestFileDiscardPackageScripts covers npm, yarn and pnpm scripts with their
// hooks, found from a folder below the package.
func TestFileDiscardPackageScripts(t *testing.T) {
	home := t.TempDir()
	tmp := tempFixture(t)
	g := newFileDiscardGuard(t, home)
	writeTree(t, home, map[string]string{
		"work/notes.txt": "n\n",
		"pkg/sub/":       "",
		"pkg/package.json": fmt.Sprintf(`{"scripts": {
			"zap": "rm -rf ../work",
			"tmpzap": "rm -rf %s/x",
			"prehooked": "rm -rf ../work",
			"hooked": "echo hi",
			"test": "rm -rf ../work",
			"args": "rm -rf",
			"chain": "echo a && npm run zap",
			"var": "rm -rf \"$X/y\"",
			"rim": "rimraf --glob ../work",
			"rimtmp": "rimraf %s/x"
		}}`, tmp, tmp),
		"life/package.json": `{"scripts": {"prepare": "rimraf ../work", "test": "echo"}}`,
	})
	workspaces := "belt does not read the scripts"
	noTree := "which is in no git work tree"
	rows := []fileDiscardRow{
		{"npm run", "~/pkg/sub", "npm run zap", []string{"the npm script `zap` in /", noTree}, nil},
		{"npm run-script", "~/pkg", "npm run-script zap", []string{noTree}, nil},
		{"temp", "~/pkg", "npm run tmpzap", nil, nil},
		{"pre hook", "~/pkg", "npm run hooked", []string{"`prehooked`", noTree}, nil},
		{"hooks skipped", "~/pkg", "npm run hooked --ignore-scripts", nil, nil},
		{"npm test", "~/pkg", "npm test", []string{noTree}, nil},
		{"list", "~/pkg", "npm run", nil, nil},
		{"install", "~/pkg", "npm install", nil, nil},
		{"missing script", "~/pkg", "npm run nope", nil, nil},
		{"arguments", "~/pkg", "npm run args -- ../work", []string{noTree}, nil},
		{"nested run", "~/pkg", "npm run chain", []string{"`zap`", noTree}, nil},
		{"variable", "~/pkg", "npm run var", []string{"belt cannot tell which path"}, nil},
		{"prefix", "~", "npm --prefix pkg run zap", []string{noTree}, nil},
		{"yarn", "~/pkg", "yarn zap", []string{"the yarn script `zap`", noTree}, nil},
		{"yarn run", "~/pkg", "yarn run zap", []string{noTree}, nil},
		{"yarn command", "~/pkg", "yarn add zap", nil, nil},
		{"yarn cwd", "~", "yarn --cwd pkg zap", []string{noTree}, nil},
		{"pnpm", "~/pkg", "pnpm zap", []string{noTree}, nil},
		{"pnpm run", "~/pkg", "pnpm run zap", []string{noTree}, nil},
		{"pnpm dir", "~", "pnpm -C pkg zap", []string{noTree}, nil},
		{"pnpm recursive", "~/pkg", "pnpm -r zap", []string{workspaces}, nil},
		{"pnpm filter", "~/pkg", "pnpm --filter x zap", []string{workspaces}, nil},
		{"npm workspaces", "~/pkg", "npm run zap --workspaces", []string{workspaces}, nil},
		{"npm workspace", "~/pkg", "npm run zap -w x", []string{workspaces}, nil},
		{"yarn workspace", "~/pkg", "yarn workspace x zap", []string{workspaces}, nil},
		{"yarn workspace command", "~/pkg", "yarn workspace x why lodash", nil, nil},
		{"yarn workspaces", "~/pkg", "yarn workspaces foreach run zap", []string{workspaces}, nil},
		{"yarn workspaces list", "~/pkg", "yarn workspaces list", nil, nil},
		{"rimraf script", "~/pkg", "npm run rim", []string{"the npm script `rim`", noTree}, nil},
		{"rimraf temp", "~/pkg", "npm run rimtmp", nil, nil},
		{"npx rimraf", "~/pkg", "npx rimraf ../work", []string{noTree}, nil},
		{"npm exec rimraf", "~/pkg", "npm exec -- rimraf ../work", []string{noTree}, nil},
		{"pnpm dlx rimraf", "~/pkg", "pnpm dlx rimraf ../work", []string{noTree}, nil},
		{"npx other", "~/pkg", "npx prettier --check .", nil, nil},
		{"install lifecycle", "~/life", "npm install", []string{"the npm script `prepare`", noTree}, nil},
		{"ci lifecycle", "~/life", "npm ci", []string{"`prepare`", noTree}, nil},
		{"pack lifecycle", "~/life", "npm pack", []string{"`prepare`", noTree}, nil},
		{"yarn install lifecycle", "~/life", "yarn", []string{"`prepare`", noTree}, nil},
		{"pnpm install lifecycle", "~/life", "pnpm install", []string{"`prepare`", noTree}, nil},
		{"lifecycle skipped", "~/life", "npm ci --ignore-scripts", nil, nil},
		{"test runs no lifecycle", "~/life", "npm test", nil, nil},
		{"no package", "~/work", "npm run zap", nil, nil},
	}
	runFileDiscardRows(t, g, home, rows)
}

// TestFileDiscardJustRecipes covers just recipes: the default one, their
// dependencies, parameters, variables, aliases and script recipes.
func TestFileDiscardJustRecipes(t *testing.T) {
	home := t.TempDir()
	tmp := tempFixture(t)
	g := newFileDiscardGuard(t, home)
	writeTree(t, home, map[string]string{
		"work/notes.txt": "n\n",
		"j/sub/":         "",
		"j/justfile": fmt.Sprintf(`dir := "../work"
tmp := '%s/x'

default: zap

zap:
    rm -rf {{dir}}

tmpzap:
    @rm -rf {{tmp}}

param target="../work":
    rm -rf {{target}}

alias z := zap

py:
    #!/usr/bin/env python3
    import shutil
    shutil.rmtree("../work")

[no-cd]
here:
    rm -rf work

after: tmpzap && zap
    echo hi

[windows]
win:
    rm -rf ../work

joined:
    rm -rf \
      {{dir}}

mod tools
mod lib 'libs/one'
`, tmp),
		"j/tools.just":        "wipe:\n    rm -rf ../work\n\ntmp:\n    echo hi\n",
		"j/libs/one/mod.just": "default:\n    rm -rf ../../../work\n",
		"py/justfile":         "set shell := [\"python3\", \"-c\"]\n\nclean:\n    import shutil; shutil.rmtree('../work')\n",
		"fish/justfile":       "set shell := [\"fish\", \"-c\"]\n\nclean:\n    echo hi\n",
	})
	noTree := "which is in no git work tree"
	rows := []fileDiscardRow{
		{"default", "~/j", "just", []string{"the just recipe `zap` in justfile:7", noTree}, nil},
		{"temp", "~/j", "just tmpzap", nil, nil},
		{"parameter default", "~/j", "just param", []string{noTree}, nil},
		{"parameter argument", "~/j", "just param " + tmp + "/y", nil, nil},
		{"alias", "~/j", "just z", []string{noTree}, nil},
		{"script recipe", "~/j", "just py", []string{"the just recipe `py` in justfile:17 runs `#!/usr/bin/env python3`", noTree}, nil},
		{"no-cd from home", "~", "just -f j/justfile here", []string{noTree}, nil},
		{"no-cd from its folder", "~/j", "just here", nil, nil},
		{"after dependency", "~/j", "just after", []string{"`zap`", noTree}, nil},
		{"other system", "~/j", "just win", nil, nil},
		{"joined line", "~/j", "just joined", []string{noTree}, nil},
		{"found from below", "~/j/sub", "just zap", []string{noTree}, nil},
		{"list", "~/j", "just --list", nil, nil},
		{"missing recipe", "~/j", "just nope", nil, nil},
		{"variable override", "~/j", "just dir=" + tmp + "/z zap", nil, nil},
		{"variable set", "~/j", "just --set tmp ../work tmpzap", []string{noTree}, nil},
		{"variable set to a temp", "~/j", "just --set dir " + tmp + "/z zap", nil, nil},
		{"module path", "~/j", "just tools::wipe", []string{"the just recipe `wipe` in tools.just:2", noTree}, nil},
		{"module words", "~/j", "just tools wipe", []string{"`wipe`", noTree}, nil},
		{"module, then a root recipe", "~/j", "just tools tmp zap", []string{"`zap`", noTree}, nil},
		{"module recipe passes", "~/j", "just tools tmp", nil, nil},
		{"module folder default", "~/j", "just lib", []string{"libs/one/mod.just", noTree}, nil},
		{"python shell", "~/py", "just clean", []string{"`clean` in justfile:4", noTree}, nil},
		{"other shell", "~/fish", "just clean", []string{"the shell `fish`"}, nil},
	}
	runFileDiscardRows(t, g, home, rows)
}

// TestFillPositional covers how a just recipe line takes its arguments as
// $1 and on under positional-arguments: outside single quotes and escapes,
// and only for a plain word.
func TestFillPositional(t *testing.T) {
	tests := []struct {
		line string
		pos  []string
		want string
	}{
		{`rm -rf "$1"`, []string{"src"}, `rm -rf "src"`},
		{`rm -rf ${2} $10`, []string{"a", "b"}, `rm -rf b a0`},
		{`sh -c 'rm -rf "$1"' x "$1"`, []string{"src"}, `sh -c 'rm -rf "$1"' x "src"`},
		{`echo \$1 "it's $1"`, []string{"src"}, `echo \$1 "it's src"`},
		{`rm -rf "$2"`, []string{"src"}, `rm -rf "$2"`},
		{`rm -rf $1`, []string{"'a b'"}, `rm -rf $1`},
		{`rm -rf "$1"`, []string{""}, `rm -rf "$1"`},
		{`rm -rf "$1"`, nil, `rm -rf "$1"`},
	}
	for _, tt := range tests {
		if got := fillPositional(tt.line, tt.pos); got != tt.want {
			t.Errorf("fillPositional(%q, %q) = %q, want %q", tt.line, tt.pos, got, tt.want)
		}
	}
}
