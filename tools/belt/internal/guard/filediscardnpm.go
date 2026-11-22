package guard

import (
	"encoding/json"
	"path/filepath"
	"strings"
)

// This file holds file-discard's reading of a package.json script that
// npm, yarn or pnpm runs (scriptRun): `npm run X` and its aliases, `yarn
// X`, `pnpm X` and their `run` forms, with the preX and postX hooks around
// X, and the lifecycle scripts an install or a pack runs (lifecycleRun). A
// command that runs scripts across workspaces is not read: it is a run belt
// cannot read, which the trust rule judges (workspaceRun).

// npmRunWords are npm's names for run-script, and npmScriptCommands the
// commands that run the script of their own name.
var (
	npmRunWords       = wordSet("run run-script rum urn")
	npmScriptCommands = map[string]string{"test": "test", "t": "test", "tst": "test", "start": "start", "stop": "stop"}
)

// The lifecycle scripts of the package itself that an install, a pack and
// a publish run, in order, by the command names that run them. The scripts
// of the packages an install fetches are not read.
var (
	installScripts = []string{"preinstall", "install", "postinstall", "prepublish", "preprepare", "prepare",
		"postprepare"}
	packScripts    = []string{"prepack", "prepare", "postpack"}
	publishScripts = []string{"prepublishOnly", "prepack", "prepare", "postpack", "publish", "postpublish"}
	npmLifecycles  = map[string][]string{
		"install": installScripts, "i": installScripts, "in": installScripts, "ins": installScripts,
		"inst": installScripts, "insta": installScripts, "instal": installScripts, "isnt": installScripts,
		"isnta": installScripts, "isntal": installScripts, "isntall": installScripts, "add": installScripts,
		"ci": installScripts, "clean-install": installScripts, "ic": installScripts,
		"install-clean": installScripts, "isntall-clean": installScripts,
		"pack": packScripts, "publish": publishScripts,
	}
	yarnLifecycles = map[string][]string{"install": installScripts, "add": installScripts, "pack": packScripts,
		"publish": publishScripts}
	pnpmLifecycles = map[string][]string{"install": installScripts, "i": installScripts, "add": installScripts,
		"pack": packScripts, "publish": publishScripts}
)

// npmValueOptions are the options of npm, yarn and pnpm that take the next
// word as their value; npmFolderOptions the ones that name the folder the
// tool works in; npmWorkspaceOptions the ones that run in other packages,
// which belt does not read.
var (
	npmValueOptions     = wordSet("--prefix -C --dir --cwd --workspace -w --filter -F --loglevel --reporter")
	npmFolderOptions    = wordSet("--prefix -C --dir --cwd")
	npmWorkspaceOptions = wordSet("--workspace -w --workspaces -ws --filter -F -r --recursive " +
		"--include-workspace-root --workspace-root")
)

// yarnCommands and pnpmCommands are the tools' own commands, which run no
// script of that name.
var (
	yarnCommands = wordSet("add audit autoclean bin cache check config create dlx exec explain " +
		"generate-lock-entry global help import info init install licenses link list login logout node " +
		"npm outdated owner pack plugin policies publish remove set tag team unlink unplug up upgrade " +
		"upgrade-interactive version versions why workspace workspaces constraints dedupe rebuild")
	pnpmCommands = wordSet("add install i update up upgrade remove rm uninstall un link ln unlink import " +
		"rebuild rb prune fetch audit list ls ll la outdated why exec dlx create init publish pack store " +
		"env setup config c root bin patch patch-commit patch-remove deploy licenses server doctor help " +
		"self-update cat-file cat-index find-hash")
)

// npmWords is a tool's words past its name: the folder it works in, the
// words that are no option, whether it runs in other packages or skips
// the hook scripts, and the view it reads package.json through.
type npmWords struct {
	dir        string
	words      []shWord
	workspaces bool
	noHooks    bool
	view       *buildView
}

// readNpmWords reads a tool's options from dir: a folder option moves it,
// a workspace option marks it. Words after `--` are kept as they are.
func readNpmWords(args []shWord, dir string, v *buildView) npmWords {
	n := npmWords{dir: dir, view: v}
	for i := 0; i < len(args); i++ {
		v, lit := args[i].literal()
		name, value, hasValue := strings.Cut(v, "=")
		switch {
		case lit && v == "--":
			n.words = append(n.words, args[i:]...)
			return n
		case !lit || !strings.HasPrefix(v, "-") || v == "-":
			n.words = append(n.words, args[i])
			continue
		case name == "--ignore-scripts":
			n.noHooks = true
		}
		n.workspaces = n.workspaces || npmWorkspaceOptions[name]
		if !hasValue && npmValueOptions[name] && i+1 < len(args) {
			i++
			value, hasValue = args[i].literal()
		}
		if npmFolderOptions[name] && hasValue {
			n.dir = joinDir(n.dir, value)
		}
	}
	return n
}

// readNpm reads an npm command: run and its aliases name a script, test,
// start and stop run their own, and an install, a pack or a publish runs
// the lifecycle scripts. `npm run` with no name lists the scripts and runs
// none.
func readNpm(args []shWord, _ map[string]string, dir string, v *buildView) (buildRun, bool) {
	n := readNpmWords(args, dir, v)
	if len(n.words) == 0 {
		return buildRun{}, false
	}
	sub, _ := n.words[0].literal()
	name, script := npmScriptCommands[sub]
	life, cycles := npmLifecycles[sub]
	runs := npmRunWords[sub] && len(n.words) > 1
	switch {
	case n.workspaces && (script || runs || cycles):
		return n.workspaceRun("npm")
	case script:
		return scriptRun("npm", n, name, n.words[1:])
	case cycles:
		return n.lifecycleRun("npm", life)
	case runs:
		return n.namedRun("npm", n.words[1:])
	}
	return buildRun{}, false
}

// readYarn reads a yarn command: `yarn run X`, or `yarn X` for an X that
// is no yarn command.
func readYarn(args []shWord, _ map[string]string, dir string, v *buildView) (buildRun, bool) {
	return readRunner("yarn", yarnCommands, yarnLifecycles, args, dir, v)
}

// readPnpm reads a pnpm command: `pnpm run X`, or `pnpm X` for an X that is
// no pnpm command.
func readPnpm(args []shWord, _ map[string]string, dir string, v *buildView) (buildRun, bool) {
	return readRunner("pnpm", pnpmCommands, pnpmLifecycles, args, dir, v)
}

// readRunner reads a yarn or pnpm command, whose own commands are builtin
// but for the ones that run lifecycle scripts (lifecycles); a bare yarn
// installs. `yarn workspace NAME X` runs X in a workspace, and `yarn
// workspaces` runs commands across them, but for info and list.
func readRunner(
	tool string,
	builtin map[string]bool,
	lifecycles map[string][]string,
	args []shWord,
	dir string,
	v *buildView,
) (buildRun, bool) {
	n := readNpmWords(args, dir, v)
	if len(n.words) == 0 {
		if tool == "yarn" {
			return n.lifecycleRun(tool, installScripts)
		}
		return buildRun{}, false
	}
	sub, _ := n.words[0].literal()
	next := ""
	if len(n.words) > 1 {
		next, _ = n.words[1].literal()
	}
	life, cycles := lifecycles[sub]
	switch {
	case tool == "yarn" && sub == "workspace":
		if len(n.words) < 3 {
			return buildRun{}, false
		}
		cmd, _ := n.words[2].literal()
		if builtin[cmd] && lifecycles[cmd] == nil {
			return buildRun{}, false
		}
		return n.workspaceRun(tool)
	case tool == "yarn" && sub == "workspaces":
		if next == "info" || next == "list" {
			return buildRun{}, false
		}
		return n.workspaceRun(tool)
	case n.workspaces && (cycles || sub == "run" && len(n.words) > 1 || !builtin[sub] && sub != "run"):
		return n.workspaceRun(tool)
	case cycles:
		return n.lifecycleRun(tool, life)
	case sub == "run" && len(n.words) > 1:
		return n.namedRun(tool, n.words[1:])
	case sub == "run", builtin[sub]:
		return buildRun{}, false
	}
	return n.namedRun(tool, n.words)
}

// workspaceRun is a run in other packages of a workspace, which belt does
// not read: one the trust rule passes only when git holds the root
// package.json and every package.json under its folder committed
// (buildRun.packages).
func (n npmWords) workspaceRun(tool string) (buildRun, bool) {
	file := nearestFile(n.dir, "package.json", n.view.exists)
	if file == "" {
		return buildRun{}, false
	}
	return buildRun{
		file: file, files: []string{file}, packages: filepath.Dir(file),
		unread: "belt does not read the scripts `" + tool + "` runs in the packages of a workspace",
	}, true
}

// namedRun reads a run whose first word names the script.
func (n npmWords) namedRun(tool string, words []shWord) (buildRun, bool) {
	name, ok := words[0].literal()
	if !ok {
		return buildRun{unread: "belt cannot tell which script `" + words[0].raw + "` names"}, true
	}
	return scriptRun(tool, n, name, words[1:])
}

// readPackage reads the scripts of the nearest package.json at or above
// the tool's folder, through its view. ok is false for none, or one belt
// cannot read.
func (n npmWords) readPackage() (file, text string, scripts map[string]string, ok bool) {
	file = nearestFile(n.dir, "package.json", n.view.exists)
	if file == "" {
		return "", "", nil, false
	}
	text, ok = n.view.read(file)
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if !ok || json.Unmarshal([]byte(text), &pkg) != nil {
		return "", "", nil, false
	}
	return file, text, pkg.Scripts, true
}

// lifecycleRun reads the lifecycle scripts names lists from the nearest
// package.json, each one it holds in order. --ignore-scripts runs none.
func (n npmWords) lifecycleRun(tool string, names []string) (buildRun, bool) {
	file, text, scripts, ok := n.readPackage()
	if !ok || n.noHooks {
		return buildRun{}, false
	}
	run := buildRun{file: file, files: []string{file}, text: text}
	shown := shownPath(file, n.dir)
	for _, s := range names {
		if body, ok := scripts[s]; ok {
			run.bodies = append(run.bodies, buildBody{
				what: bodyWhat(tool+" script", s, shown, body), text: body, dir: filepath.Dir(file), file: file,
			})
		}
	}
	return run, true
}

// scriptRun reads a script from the nearest package.json at or above the
// tool's folder, with its pre and post hooks; the words after its name are
// appended to it, past a `--`. A script package.json does not hold runs
// nothing.
func scriptRun(tool string, n npmWords, name string, extra []shWord) (buildRun, bool) {
	file, text, scripts, ok := n.readPackage()
	if !ok {
		return buildRun{}, false
	}
	main, ok := scripts[name]
	if !ok {
		return buildRun{}, false
	}
	run := buildRun{file: file, files: []string{file}, text: text}
	root, shown := filepath.Dir(file), shownPath(file, n.dir)
	for i, w := range extra {
		if v, lit := w.literal(); i > 0 || !lit || v != "--" {
			main += " " + argText(w)
		}
	}
	steps := []string{name}
	if !n.noHooks {
		steps = []string{"pre" + name, name, "post" + name}
	}
	for _, s := range steps {
		body, ok := scripts[s]
		if s == name {
			body = main
		}
		if ok || s == name {
			run.bodies = append(run.bodies, buildBody{
				what: bodyWhat(tool+" script", s, shown, body), text: body, dir: root, file: file,
			})
		}
	}
	return run, true
}

// nearestFile returns the file named name in dir or the nearest folder
// above it that holds one (exists), "" for none.
func nearestFile(dir, name string, exists func(string) bool) string {
	for {
		p := filepath.Join(dir, name)
		if exists(p) {
			return p
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}
