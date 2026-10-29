package discover

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"
)

// launchMarkers are the strings that mark a recipe command as launching a
// process: registering a t-man service. srt-run is not one. This repo
// renders srt settings only from its own manifests, so an srt-run exec whose
// settings it renders already has one, and an exec whose manifest the
// consuming repo owns is outside what this repo can verify.
var launchMarkers = []string{"t-man add"}

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

// componentKind is the catalog kind that may carry spec.access.
const componentKind = "Component"

// Launch is a component whose recipe launches a process. Paths are relative
// to the root Undeclared was given.
type Launch struct {
	Path   string // the component's service-info.yaml
	File   string // the recipe file holding the launch
	Marker string // the launch marker found in it
}

// Undeclared returns the components under root whose recipe launches a
// process but whose service-info.yaml declares no spec.access, sorted by
// path. targets is Find's result; a service-info.yaml among them declares
// one. The recipe is the one generate renders into: recipes/<name of the
// directory holding the service-info.yaml>. It launches a process when a
// command it runs registers a t-man service ("t-man add"): a value under a
// command key of its recipe.toml, or a line of a *.sh file beside it. Shell
// comment lines never count. An MCP server, which the consuming repo
// registers, is not checked here even when its wrapper sits beside the
// recipe.
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
// launches a process, and the marker found in it: recipe.toml first, then
// the *.sh files beside it. file is "" when nothing in the recipe launches
// one.
func recipeLaunch(dir string) (file, marker string, err error) {
	recipe := filepath.Join(dir, "recipe.toml")
	cmds, err := recipeCommands(recipe)
	if err != nil {
		return "", "", err
	}
	if m := findMarker(strings.Join(cmds, "\n")); m != "" {
		return recipe, m, nil
	}
	scripts, err := filepath.Glob(filepath.Join(dir, "*.sh"))
	if err != nil {
		return "", "", fmt.Errorf("discover: %w", err)
	}
	for _, s := range scripts {
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

// recipeCommands returns the shell commands the recipe.toml at path holds,
// sorted: every string under a command key, at any depth. A missing file
// holds none.
func recipeCommands(path string) ([]string, error) {
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
	var cmds []string
	collectCommands(doc, false, &cmds)
	slices.Sort(cmds)
	return cmds, nil
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

// findMarker returns the first launch marker on a line of script that is not
// a shell comment, or "" when none is.
func findMarker(script string) string {
	for _, line := range strings.Split(script, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			continue
		}
		for _, m := range launchMarkers {
			if strings.Contains(line, m) {
				return m
			}
		}
	}
	return ""
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
