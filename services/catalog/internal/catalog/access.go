package catalog

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// launchMarkers are the strings that mark a recipe line as launching a
// process.
var launchMarkers = []string{"t-man add", "srt-run"}

// RecipeDir returns the recipe directory for the component declared in the
// service-info.yaml at path, and whether one exists.
func RecipeDir(path string) (string, bool) {
	dir := filepath.Dir(path)
	name := filepath.Base(dir)
	for cur := dir; ; {
		candidate := filepath.Join(cur, "recipes", name)
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate, true
		}
		if _, err := os.Stat(filepath.Join(cur, ".git")); err == nil {
			return "", false
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", false
		}
		cur = parent
	}
}

// Launch reports how the recipe in dir launches a process: the file and the
// launch marker found in it. ok is false when nothing in the recipe launches
// one.
func Launch(dir string) (file, marker string, ok bool, err error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.sh"))
	if err != nil {
		return "", "", false, fmt.Errorf("catalog: %w", err)
	}
	files = append([]string{filepath.Join(dir, "recipe.toml")}, files...)
	for _, f := range files {
		m, ok, err := launchMarker(f)
		if err != nil {
			return "", "", false, err
		}
		if ok {
			return f, m, true, nil
		}
	}
	return "", "", false, nil
}

// launchMarker returns the launch marker on the first non-comment line of
// path that holds one. A missing file holds none.
func launchMarker(path string) (string, bool, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("catalog: %w", err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		text := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(text, "#") {
			continue
		}
		for _, m := range launchMarkers {
			if strings.Contains(text, m) {
				return m, true, nil
			}
		}
	}
	if err := sc.Err(); err != nil {
		return "", false, fmt.Errorf("catalog: read %s: %w", path, err)
	}
	return "", false, nil
}

// NeedsAccess reports why e must declare spec.access, or "" when it need
// not. A Component needs one when the recipe that installs it launches a
// long-running process. The recipe is found by the fleet's layout
// convention: recipes/<name of the directory holding the service-info.yaml>,
// in the nearest ancestor directory that has one, searching up to the repo
// root. It launches a process when a non-comment line of its recipe.toml, or
// of a *.sh file beside it, registers a t-man service ("t-man add") or execs
// srt-run (the spawn wrapper a sandboxed MCP server or service runs
// through). An MCP server registered only in the consuming repo's server
// list, with no wrapper beside its recipe, is not detected.
func NeedsAccess(e Entity) (string, error) {
	if e.Kind != KindComponent || e.SourcePath == "" {
		return "", nil
	}
	dir, ok := RecipeDir(e.SourcePath)
	if !ok {
		return "", nil
	}
	file, marker, ok, err := Launch(dir)
	if err != nil || !ok {
		return "", err
	}
	return fmt.Sprintf("%s runs %q", file, marker), nil
}

// CheckAccess fails when a Component that needs spec.access lacks it,
// naming every such Component and why it needs one. needs is NeedsAccess in
// production; tests pass a stub.
func CheckAccess(entities []Entity, needs func(Entity) (string, error)) error {
	var missing []string
	for _, e := range entities {
		if e.Spec.Access != nil {
			continue
		}
		why, err := needs(e)
		if err != nil {
			return err
		}
		if why != "" {
			missing = append(
				missing,
				fmt.Sprintf("  %s (%s): %s", e.Metadata.Name, e.SourcePath, why),
			)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf(
		"%d component(s) launch a process but declare no spec.access "+
			"(add one; sandbox none needs a reason):\n%s",
		len(missing), strings.Join(missing, "\n"))
}
