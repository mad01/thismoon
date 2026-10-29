package discover

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"
)

// tmanAdd matches a command that launches a process by registering a t-man
// service: t-man, any root flags such as --daemon, then add. srt-run is not
// a launch. This repo renders srt settings only from its own manifests, so
// an srt-run exec whose settings it renders already has one, and an exec
// whose manifest the consuming repo owns is outside what this repo can
// verify.
var tmanAdd = regexp.MustCompile(`\bt-man\b(\s+-\S+)*\s+add\b`)

// commandKeys are the recipe.toml keys whose values ralph runs as shell
// commands: the apply, link and uninstall hook lists, a build's commands and
// verify, a package's build, install and service restart steps, a tool's
// check, and shell alias and function bodies. Any other value, such as a
// description, is text and never counts as a launch.
var commandKeys = map[string]bool{
	"pre_apply":      true,
	"post_apply":     true,
	"pre_link":       true,
	"post_link":      true,
	"pre_uninstall":  true,
	"post_uninstall": true,
	"commands":       true,
	"verify":         true,
	"build":          true,
	"install":        true,
	"restart":        true,
	"check_command":  true,
	"command":        true,
	"body":           true,
}

// shellExts are the suffixes of the scripts in a recipe directory that
// count as shell.
var shellExts = []string{".sh", ".bash"}

// testSuffixes end the name of a script, before its suffix, that checks a
// recipe rather than running as part of it.
var testSuffixes = []string{"-test", "_test"}

// componentKind is the catalog kind that may carry spec.access.
const componentKind = "Component"

// Launch is a component whose recipe launches a process. Paths are relative
// to the root Undeclared was given.
type Launch struct {
	Path   string // the component's service-info.yaml
	File   string // the recipe file holding the launch
	Marker string // the launch found in it, such as "t-man add"
}

// Undeclared returns the components under root whose recipe launches a
// process but whose service-info.yaml declares no spec.access, sorted by
// path. targets is Find's result; a service-info.yaml among them declares
// one. The recipe is the one generate renders into: recipes/<name of the
// directory holding the service-info.yaml>. It launches a process when
// something in that directory registers a t-man service (t-man, root flags,
// add): a value under a command key of its recipe.toml, a file a build runs
// as script, or a line of a *.sh or *.bash file at any depth, except a test
// script (*-test.sh, *_test.sh). Shell comments never count. An MCP server,
// which the consuming repo registers, is not checked here even when its
// wrapper sits in the recipe directory.
func Undeclared(root string, targets []Target) ([]Launch, error) {
	declared := map[string]bool{}
	for _, t := range targets {
		declared[t.Path] = true
	}
	paths, err := glob(root, componentGlobs)
	if err != nil {
		return nil, err
	}
	var out []Launch
	for _, p := range paths {
		rel := relTo(root, p)
		if declared[rel] {
			continue
		}
		l, ok, err := launchOf(root, p)
		if err != nil {
			return nil, err
		}
		if ok {
			l.Path = rel
			out = append(out, l)
		}
	}
	return out, nil
}

// launchOf reports how the recipe of the service-info.yaml at path launches
// a process. ok is false when there is no recipe directory, nothing in it
// launches one, or no document in the file is a Component.
func launchOf(root, path string) (Launch, bool, error) {
	dir := filepath.Join(root, recipeDir(path))
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return Launch{}, false, nil
	}
	file, marker, err := recipeLaunch(dir)
	if err != nil || file == "" {
		return Launch{}, false, err
	}
	isComp, err := declaresComponent(path)
	if err != nil || !isComp {
		return Launch{}, false, err
	}
	return Launch{File: relTo(root, file), Marker: marker}, true, nil
}

// recipeLaunch returns the first file in the recipe directory dir that
// launches a process, and the launch found in it: recipe.toml first, then
// the shell scripts and build scripts in dir, sorted by path. file is ""
// when nothing in the recipe launches one.
func recipeLaunch(dir string) (file, marker string, err error) {
	recipe := filepath.Join(dir, "recipe.toml")
	doc, err := readRecipe(recipe)
	if err != nil {
		return "", "", err
	}
	if m := findMarker(strings.Join(recipeCommands(doc), "\n")); m != "" {
		return recipe, m, nil
	}
	scripts, err := shellScripts(dir)
	if err != nil {
		return "", "", err
	}
	scripts = append(scripts, buildScripts(doc, dir)...)
	slices.Sort(scripts)
	for _, s := range slices.Compact(scripts) {
		data, err := os.ReadFile(s)
		if err != nil {
			return "", "", fmt.Errorf("discover: %w", err)
		}
		if m := findMarker(string(data)); m != "" {
			return s, m, nil
		}
	}
	return "", "", nil
}

// readRecipe parses the recipe.toml at path. A missing file parses to an
// empty recipe.
func readRecipe(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("discover: %w", err)
	}
	var doc map[string]any
	if err := toml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("discover: %s: %w", path, err)
	}
	return doc, nil
}

// recipeCommands returns the shell commands a parsed recipe.toml holds,
// sorted: every string under a command key, at any depth.
func recipeCommands(doc map[string]any) []string {
	var cmds []string
	collectCommands(doc, false, &cmds)
	slices.Sort(cmds)
	return cmds
}

// collectCommands appends to cmds every string under v that sits below a
// command key; inCommand reports whether v itself does.
func collectCommands(v any, inCommand bool, cmds *[]string) {
	switch v := v.(type) {
	case string:
		if inCommand {
			*cmds = append(*cmds, v)
		}
	case []any:
		for _, e := range v {
			collectCommands(e, inCommand, cmds)
		}
	case []map[string]any:
		for _, e := range v {
			collectCommands(e, inCommand, cmds)
		}
	case map[string]any:
		for k, e := range v {
			collectCommands(e, inCommand || commandKeys[k], cmds)
		}
	}
}

// shellScripts returns every *.sh and *.bash file under the recipe
// directory dir, at any depth, in lexical order, except the test scripts.
func shellScripts(dir string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && isShellScript(d.Name()) {
			out = append(out, p)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("discover: %w", err)
	}
	return out, nil
}

// isShellScript reports whether a file named name is a shell script that
// runs as part of a recipe rather than checking it.
func isShellScript(name string) bool {
	ext := filepath.Ext(name)
	if !slices.Contains(shellExts, ext) {
		return false
	}
	stem := strings.TrimSuffix(name, ext)
	return !slices.ContainsFunc(testSuffixes, func(s string) bool {
		return strings.HasSuffix(stem, s)
	})
}

// buildScripts returns the regular files under the recipe directory dir
// that a build in the parsed recipe.toml doc runs as script, whatever their
// name. ralph resolves a relative script against the build's working_dir.
func buildScripts(doc map[string]any, dir string) []string {
	hooks, _ := doc["hooks"].(map[string]any)
	builds, _ := hooks["builds"].(map[string]any)
	var out []string
	for _, b := range builds {
		build, _ := b.(map[string]any)
		script, _ := build["script"].(string)
		if script == "" {
			continue
		}
		workingDir, _ := build["working_dir"].(string)
		if !rooted(script) && workingDir != "" {
			script = filepath.Join(workingDir, script)
		}
		if p, ok := inRecipe(dir, script); ok && isFile(p) {
			out = append(out, p)
		}
	}
	return out
}

// inRecipe maps p, a path as a recipe writes it, to the path it names in
// the recipe directory dir. A relative p is relative to dir. A rooted one
// names a path in dir when it runs through recipes/<name of dir>, as a
// working_dir in ralph's sources cache does. ok is false when p leads
// outside dir.
func inRecipe(dir, p string) (string, bool) {
	if rooted(p) {
		seg := "/recipes/" + filepath.Base(dir) + "/"
		i := strings.LastIndex(p, seg)
		if i < 0 {
			return "", false
		}
		p = p[i+len(seg):]
	}
	p = filepath.Clean(p)
	if p == ".." || strings.HasPrefix(p, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.Join(dir, p), true
}

// rooted reports whether a recipe path is absolute, ~-prefixed or starts
// with a variable such as $HOME, so ralph does not resolve it against a
// working_dir.
func rooted(p string) bool {
	return strings.HasPrefix(p, "/") || strings.HasPrefix(p, "~") ||
		strings.HasPrefix(p, "$")
}

// isFile reports whether p names a regular file, following symlinks.
func isFile(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.Mode().IsRegular()
}

// findMarker returns the first t-man registration in script, with its
// whitespace collapsed to single spaces, or "" when there is none. Shell
// comments never count.
func findMarker(script string) string {
	for _, line := range strings.Split(script, "\n") {
		if m := tmanAdd.FindString(stripComment(line)); m != "" {
			return strings.Join(strings.Fields(m), " ")
		}
	}
	return ""
}

// stripComment drops the shell comment from line: a # that starts the line
// or follows whitespace, outside single and double quotes, and everything
// after it. A backslash escapes the next byte outside single quotes.
func stripComment(line string) string {
	var quote byte
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case quote == '\'':
			if c == '\'' {
				quote = 0
			}
		case c == '\\':
			i++
		case quote == '"':
			if c == '"' {
				quote = 0
			}
		case c == '\'' || c == '"':
			quote = c
		case c == '#' && (i == 0 || line[i-1] == ' ' || line[i-1] == '\t'):
			return line[:i]
		}
	}
	return line
}

// declaresComponent reports whether any document in the service-info.yaml
// at path is a Component; only a Component may carry spec.access.
func declaresComponent(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("discover: %w", err)
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	for {
		var doc struct {
			Kind string `yaml:"kind"`
		}
		err := dec.Decode(&doc)
		if errors.Is(err, io.EOF) {
			return false, nil
		}
		if err != nil {
			return false, fmt.Errorf("discover: %s: %w", path, err)
		}
		if doc.Kind == componentKind {
			return true, nil
		}
	}
}
