package discover

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// bare returns a service-info.yaml for an entity of kind with no access
// block.
func bare(kind, name string) string {
	return "kind: " + kind + "\nmetadata: {name: " + name + "}\nspec: {owner: mad01}\n"
}

const tmanRecipe = `[hooks.builds.web_service]
commands = ["t-man add --name web -- $HOME/code/bin/web serve"]
`

const srtRunWrapper = `#!/bin/sh
cd /
exec "$HOME/code/bin/srt-run" "$HOME/.config/srt/mcpish.json" "$HOME/code/bin/mcpish" mcp
`

func TestUndeclared(t *testing.T) {
	root := tree(t, map[string]string{
		// A build hook's commands array registers the service.
		"services/web/service-info.yaml": bare("Component", "web"),
		"recipes/web/recipe.toml":        tmanRecipe,
		// A package's service restart string.
		"services/rs/service-info.yaml": bare("Component", "rs"),
		"recipes/rs/recipe.toml":        "[packages.rs.service]\nrestart = \"t-man add --name rs -- rs serve\"\n",
		// An apply hook list.
		"services/hook/service-info.yaml": bare("Component", "hook"),
		"recipes/hook/recipe.toml":        "[hooks]\npost_apply = [\"t-man add --name hook -- hook serve\"]\n",
		// A shell function body, past a comment line inside it.
		"tools/fn/service-info.yaml": bare("Component", "fn"),
		"recipes/fn/recipe.toml": `[shell.functions.up]
body = '''
# start it
t-man add --name fn -- fn serve
'''
`,
		// A tool's check, inside an array of tables.
		"tools/tool/service-info.yaml": bare("Component", "tool"),
		"recipes/tool/recipe.toml":     "[[tools]]\nname = \"tool\"\ncheck_command = \"t-man add --name tool -- tool serve\"\n",
		// A shell script beside the recipe.
		"tools/shsvc/service-info.yaml": bare("Component", "shsvc"),
		"recipes/shsvc/recipe.toml":     "[packages.shsvc]\n",
		"recipes/shsvc/register.sh":     "#!/bin/sh\nt-man add --name shsvc -- shsvc serve\n",
		// A Component behind a System in the same file.
		"multi/service-info.yaml":   bare("System", "fleet") + "---\n" + bare("Component", "multi"),
		"recipes/multi/recipe.toml": tmanRecipe,
		// t-man root flags before add, as a daemon registration has.
		"services/daemon/service-info.yaml": bare("Component", "daemon"),
		"recipes/daemon/recipe.toml": "[hooks]\npost_apply = " +
			"[\"sudo t-man --daemon add --name daemon -- daemon serve\"]\n",
		// A post_link hook, a map from linked file to commands.
		"services/linked/service-info.yaml": bare("Component", "linked"),
		"recipes/linked/recipe.toml": "[hooks.post_link]\n" +
			"\"~/.config/linked/config.toml\" = [\"t-man add --name linked -- linked serve\"]\n",
		// A script in a subdirectory, and one with the .bash suffix.
		"tools/subdir/service-info.yaml":     bare("Component", "subdir"),
		"recipes/subdir/recipe.toml":         "[packages.subdir]\n",
		"recipes/subdir/scripts/register.sh": "#!/bin/sh\nt-man add --name subdir -- subdir serve\n",
		"tools/bashsvc/service-info.yaml":    bare("Component", "bashsvc"),
		"recipes/bashsvc/register.bash":      "#!/bin/bash\nt-man add --name bashsvc -- bashsvc serve\n",
		// A build's script, relative to the recipe directory and without a
		// shell suffix.
		"tools/scripted/service-info.yaml": bare("Component", "scripted"),
		"recipes/scripted/recipe.toml":     "[hooks.builds.reg]\nscript = \"register\"\n",
		"recipes/scripted/register":        "t-man add --name scripted -- scripted serve\n",
		// A build's script under a working_dir in ralph's sources cache. A
		// script a build names runs even when its name marks a test.
		"tools/cached/service-info.yaml": bare("Component", "cached"),
		"recipes/cached/recipe.toml": `[hooks.builds.reg]
script = "bin/smoke-test.sh"
working_dir = "~/.config/ralph/sources/thismoon/recipes/cached"
`,
		"recipes/cached/bin/smoke-test.sh": "t-man add --name cached -- cached serve\n",

		// Text that names t-man add but runs nothing: a description, a TOML
		// comment, a shell comment inside a command, a non-command key.
		"tools/desc/service-info.yaml": bare("Component", "desc"),
		"recipes/desc/recipe.toml": `[recipe]
name = "desc"
description = "the launcher behind srt-run; registers nothing with t-man add"
[packages.desc]
source = "local"
`,
		"tools/quiet/service-info.yaml": bare("Component", "quiet"),
		"recipes/quiet/recipe.toml": `# A comment that mentions t-man add and srt-run launches nothing.
[packages.quiet]
caveats = "run t-man add yourself"
[shell.functions.note]
body = "# t-man add is done by the service recipe"
`,
		// A trailing comment, another t-man subcommand, and test scripts
		// that grep the recipe for the marker.
		"tools/trail/service-info.yaml": bare("Component", "trail"),
		"recipes/trail/recipe.toml": `[hooks]
post_apply = ["echo hi # t-man add elsewhere"]
post_uninstall = ["sudo t-man --daemon remove trail"]
`,
		"recipes/trail/srt-settings-test.sh": "grep -F \"t-man add --name\" recipe.toml\n",
		"recipes/trail/check_test.bash":      "grep -F \"t-man add --name\" recipe.toml\n",
		// A build's script whose working_dir is outside the recipe
		// directory runs a file this check does not read.
		"tools/elsewhere/service-info.yaml": bare("Component", "elsewhere"),
		"recipes/elsewhere/recipe.toml": `[hooks.builds.reg]
script = "register"
working_dir = "~/.config/ralph/sources/thismoon/tools/elsewhere"
`,
		"tools/elsewhere/register": "t-man add --name elsewhere -- elsewhere serve\n",
		// An MCP server's srt-run wrapper is the consuming repo's to declare.
		"tools/mcpish/service-info.yaml":   bare("Component", "mcpish"),
		"recipes/mcpish/recipe.toml":       "[packages.mcpish]\n",
		"recipes/mcpish/mcpish-sandbox.sh": srtRunWrapper,
		// No recipe, a System, and a Component that declares access.
		"tools/norecipe/service-info.yaml": bare("Component", "norecipe"),
		"services/sys/service-info.yaml":   bare("System", "sys"),
		"recipes/sys/recipe.toml":          tmanRecipe,
		"services/csl/service-info.yaml":   comp("csl", "{sandbox: none, reason: ssh}"),
		"recipes/csl/recipe.toml":          tmanRecipe,
	})
	targets, err := Find(root)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Undeclared(root, targets)
	if err != nil {
		t.Fatal(err)
	}
	launch := func(path, file string) Launch {
		return Launch{Path: path, File: file, Marker: "t-man add"}
	}
	want := []Launch{
		launch("multi/service-info.yaml", "recipes/multi/recipe.toml"),
		{
			Path:   "services/daemon/service-info.yaml",
			File:   "recipes/daemon/recipe.toml",
			Marker: "t-man --daemon add",
		},
		launch("services/hook/service-info.yaml", "recipes/hook/recipe.toml"),
		launch("services/linked/service-info.yaml", "recipes/linked/recipe.toml"),
		launch("services/rs/service-info.yaml", "recipes/rs/recipe.toml"),
		launch("services/web/service-info.yaml", "recipes/web/recipe.toml"),
		launch("tools/bashsvc/service-info.yaml", "recipes/bashsvc/register.bash"),
		launch("tools/cached/service-info.yaml", "recipes/cached/bin/smoke-test.sh"),
		launch("tools/fn/service-info.yaml", "recipes/fn/recipe.toml"),
		launch("tools/scripted/service-info.yaml", "recipes/scripted/register"),
		launch("tools/shsvc/service-info.yaml", "recipes/shsvc/register.sh"),
		launch("tools/subdir/service-info.yaml", "recipes/subdir/scripts/register.sh"),
		launch("tools/tool/service-info.yaml", "recipes/tool/recipe.toml"),
	}
	if !slices.Equal(got, want) {
		t.Fatalf("Undeclared() =\n%+v\nwant\n%+v", got, want)
	}
}

// A recipes directory above the root belongs to another tree: the mapping
// is relative to the root verify was given.
func TestUndeclaredStaysUnderRoot(t *testing.T) {
	outer := tree(t, map[string]string{
		"recipes/svc/recipe.toml":    tmanRecipe,
		"repo/svc/service-info.yaml": bare("Component", "svc"),
	})
	got, err := Undeclared(filepath.Join(outer, "repo"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("Undeclared() = %+v, want none past the root", got)
	}
}

func TestUndeclaredErrors(t *testing.T) {
	tests := []struct {
		name   string
		recipe string // recipe.toml content; "/" makes it a directory
		want   string
	}{
		{name: "unreadable recipe", recipe: "/", want: "recipe.toml"},
		{name: "recipe that is not TOML", recipe: "[packages\n", want: "recipe.toml"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := map[string]string{"services/web/service-info.yaml": bare("Component", "web")}
			if tt.recipe == "/" {
				files["recipes/web/recipe.toml/"] = ""
			} else {
				files["recipes/web/recipe.toml"] = tt.recipe
			}
			_, err := Undeclared(tree(t, files), nil)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Undeclared() = %v, want an error naming %s", err, tt.want)
			}
		})
	}
}

func TestFindMarker(t *testing.T) {
	tests := []struct {
		name   string
		script string
		want   string
	}{
		{name: "plain", script: "t-man add --name x -- x serve", want: "t-man add"},
		{name: "by path", script: "$HOME/code/bin/t-man add --name x", want: "t-man add"},
		{
			name:   "root flags before add",
			script: "sudo t-man --daemon add --name x -- x serve",
			want:   "t-man --daemon add",
		},
		{name: "whitespace collapsed", script: "t-man\tadd --name x", want: "t-man add"},
		{name: "after a comment line", script: "# start\nt-man add --name x", want: "t-man add"},
		{name: "trailing comment", script: "t-man add --name x # register", want: "t-man add"},
		{name: "quoted #", script: "echo 'a # b' && t-man add --name x", want: "t-man add"},
		{name: "double-quoted #", script: `echo "a # b" && t-man add --name x`, want: "t-man add"},
		{
			name:   "# inside a word",
			script: "curl http://h/#frag; t-man add --name x",
			want:   "t-man add",
		},
		{name: "escaped quote", script: `echo "a \" # b" && t-man add --name x`, want: "t-man add"},
		{name: "comment line", script: "  # t-man add --name x", want: ""},
		{name: "comment after a command", script: "echo hi # t-man add elsewhere", want: ""},
		{name: "another subcommand", script: "sudo t-man --daemon remove x", want: ""},
		{name: "list", script: "t-man list", want: ""},
		{name: "a longer name", script: "t-man-helper add x", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := findMarker(tt.script); got != tt.want {
				t.Fatalf("findMarker(%q) = %q, want %q", tt.script, got, tt.want)
			}
		})
	}
}
