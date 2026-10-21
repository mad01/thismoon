package mods

import (
	"fmt"
	"strings"

	"github.com/mad01/thismoon/tools/agent-sync/internal/pyjson"
	"github.com/mad01/thismoon/tools/agent-sync/internal/pypath"
)

// The claude plugin verbs a sync runs, one per kind of action.
const (
	verbMarketplace = "marketplace"
	verbInstall     = "install"
	verbEnable      = "enable"
	verbUpdate      = "update"
	verbDisable     = "disable"
	verbUninstall   = "uninstall"
)

// signs marks each verb in the report: + adds, ~ changes, - removes.
var signs = map[string]string{
	verbMarketplace: "+",
	verbInstall:     "+",
	verbEnable:      "~",
	verbUpdate:      "~",
	verbDisable:     "-",
	verbUninstall:   "-",
}

// action is one claude plugin call that closes part of the gap.
type action struct {
	verb   string
	target string // the marketplace name or the plugin id
	// detail is the marketplace path for verbMarketplace, and
	// "<installed> -> <folder>" versions for verbUpdate.
	detail string
}

// String is the action's report line, e.g. "~ update a@b (0.1.0 -> 0.2.0)".
func (a action) String() string {
	line := signs[a.verb] + " " + a.verb + " " + a.target
	switch {
	case a.detail == "":
		return line
	case a.verb == verbMarketplace:
		return line + " -> " + a.detail
	default:
		return line + " (" + a.detail + ")"
	}
}

// args is the action's argv after `claude plugin`, always at user scope.
func (a action) args() []string {
	if a.verb == verbMarketplace {
		return []string{"marketplace", "add", a.detail, "--scope", "user"}
	}
	return []string{a.verb, a.target, "--scope", "user"}
}

// planAll plans every mods file whose marketplace is on disk and returns
// those files, the actions and the notes. A file whose marketplace is not on
// disk is skipped with a note.
func planAll(sources []modsFile, st state) ([]modsFile, []action, []string, error) {
	var present []modsFile
	var actions []action
	var notes []string
	for _, f := range sources {
		listed, onDisk, err := listing(f)
		if err != nil {
			return nil, nil, nil, err
		}
		if !onDisk {
			notes = append(notes, fmt.Sprintf(
				"claude-mods: marketplace %s is not on disk at %s (%s; ralph up pulls the sources); skipped",
				pyjson.Repr(f.marketplace),
				f.path,
				baseName(f.source),
			))
			continue
		}
		present = append(present, f)
		versions, versionNotes := folderVersions(f, listed)
		planned, planNotes := plan(f, st, listed, versions)
		actions = append(actions, planned...)
		notes = append(append(notes, planNotes...), versionNotes...)
	}
	return present, actions, notes, nil
}

// plan returns the claude plugin calls that close the gap for one mods
// file. versions maps a listed mod to the version in its marketplace
// folder; a mod missing from it is not checked for updates.
func plan(
	f modsFile,
	st state,
	listed map[string]any,
	versions map[string]string,
) ([]action, []string) {
	var actions []action
	var notes []string
	recorded, registered := st.marketplaces[f.marketplace]
	if !registered || !samePath(recorded, f.path) {
		actions = append(
			actions,
			action{verb: verbMarketplace, target: f.marketplace, detail: f.path},
		)
	}
	for _, m := range f.mods {
		id := m.name + "@" + f.marketplace
		_, installed := st.installed[id]
		switch m.want {
		case wantRetired:
			// No listing check: a retired mod is usually gone from the
			// manifest already.
			if installed {
				actions = append(actions, action{verb: verbUninstall, target: id})
			}
		case wantEnabled:
			if _, ok := listed[m.name]; !ok {
				notes = append(notes,
					fmt.Sprintf("warn: %s is not in the marketplace listing yet; skipped", id))
			} else if !installed {
				actions = append(actions, action{verb: verbInstall, target: id})
			} else {
				actions = append(actions, planInstalled(id, versions[m.name], st)...)
			}
		case wantDisabled:
			if st.enabled[id] {
				actions = append(actions, action{verb: verbDisable, target: id})
			}
		}
	}
	return actions, notes
}

// planInstalled enables and updates an installed mod the file enables. The
// update rule: a record version that differs from the folder version runs
// `claude plugin update`, which moves the record to the folder version, so
// one sync settles it. A version that is unknown on either side ("") is
// never compared.
func planInstalled(id, folder string, st state) []action {
	var actions []action
	if !st.enabled[id] {
		actions = append(actions, action{verb: verbEnable, target: id})
	}
	if recorded := st.installed[id]; recorded != "" && folder != "" && recorded != folder {
		actions = append(
			actions,
			action{verb: verbUpdate, target: id, detail: recorded + " -> " + folder},
		)
	}
	return actions
}

// samePath reports whether a recorded marketplace path names the wanted
// directory once ~ and symlinks are resolved.
func samePath(recorded, wanted string) bool {
	expanded, err := pypath.ExpandUser(recorded)
	if err != nil {
		return false
	}
	return pypath.RealPath(expanded) == pypath.RealPath(wanted)
}

// baseName is Path.name: the last path segment.
func baseName(p string) string {
	p = pypath.Clean(p)
	return p[strings.LastIndex(p, "/")+1:]
}
