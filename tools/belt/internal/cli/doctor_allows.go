package cli

import (
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"

	"github.com/mad01/thismoon/tools/belt/internal/config"
	"github.com/mad01/thismoon/tools/belt/internal/guard"
)

// printAllows renders the allows section (docs/adr/0032) and returns its
// warnings: every roots entry an allow ignores, a root that sits inside a
// git work tree, and an allows key no registered allow answers to. A
// typo'd key leaves its allow off, the safe direction, but the operator
// meant to turn something on.
func printAllows(w io.Writer, cfg config.Config) []string {
	fmt.Fprintln(w, "\nallows, which may answer allow once every guard passed (allows.<id>.*):")
	var warns []string
	known := map[string]bool{}
	for _, a := range guard.Allows(cfg) {
		id := a.ID()
		known[id] = true
		warns = append(warns, cfg.Allows[id].Adjustments(id)...)
		if !cfg.AllowEnabled(id) {
			fmt.Fprintf(w, "  %-22s %-7s DISABLED (the default; set allows.%s.enabled to run it)\n",
				id, a.Event(), id)
			continue
		}
		roots := cfg.Allows[id].RootDirs()
		fmt.Fprintf(
			w,
			"  %-22s %-7s enabled  roots: the session's own work tree (unless it is home)",
			id,
			a.Event(),
		)
		for _, root := range roots {
			fmt.Fprintf(w, ", %s", root)
		}
		fmt.Fprintln(w)
		fmt.Fprintf(
			w,
			"  %-30s a linked worktree needs its folder and its main checkout under roots\n",
			"",
		)
		for _, root := range roots {
			if top, ok := enclosingWorkTree(root); ok {
				warns = append(warns, fmt.Sprintf(
					"allows.%s.roots entry %s sits inside the git work tree at %s: "+
						"a directory under it with no repo of its own reads that repo, so %s gives no answer there",
					id, root, top, id,
				))
			}
		}
	}
	for _, key := range slices.Sorted(maps.Keys(cfg.Allows)) {
		if !known[key] {
			warns = append(warns, fmt.Sprintf(
				"unknown allow %q in config file: no registered allow answers to it", key,
			))
		}
	}
	return warns
}

// enclosingWorkTree returns the nearest directory above root that holds a
// .git entry: the work tree root sits in, if any.
func enclosingWorkTree(root string) (string, bool) {
	for d := filepath.Dir(root); ; d = filepath.Dir(d) {
		if _, err := os.Lstat(filepath.Join(d, ".git")); err == nil {
			return d, true
		}
		if filepath.Dir(d) == d {
			return "", false
		}
	}
}
