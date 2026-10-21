// Package mcp registers and removes Claude Code MCP servers through the
// claude CLI, from a servers.json the mcp section of the agent-sync config
// names. It is the Go port of the dotfiles repo's sync_mcp.py and keeps
// that script's stdout, exit codes, claude calls and stamp file.
//
// The desired set is the base servers.json with every *.json in the
// drop-in directory merged over it by name, minus entries whose name starts
// with "_" and entries gated by hosts or profiles to other machines. The
// registered set is what `claude mcp list` prints, run from an empty
// directory so that no local or project scope server is in it. Missing
// servers are added; a server in both whose `claude mcp get` spec differs is
// removed and added again, unless get shows it at another scope; with prune,
// registered servers outside the desired set are removed, except the ones a
// user-scope remove cannot reach and the ones add would reject. The servers
// left alone are named in a note.
//
// The per-server get calls are the slow part, so sync records a hash of the
// desired set in a stamp file. While the stamp matches, sync and check treat
// every server in both sets as unchanged; plan, prune and --force always
// compare specs.
//
// config.go reads the section, servers.go and entry.go load the desired
// set, claude.go wraps the CLI and parses its output, and this file runs a
// mode.
package mcp

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mad01/thismoon/tools/agent-sync/internal/apply"
	"github.com/mad01/thismoon/tools/agent-sync/internal/gate"
	"github.com/mad01/thismoon/tools/agent-sync/internal/pyjson"
	"github.com/mad01/thismoon/tools/agent-sync/internal/pypath"
)

// The report reproduces sync_mcp.py's output byte for byte, and the plan
// footer carries an em dash. It is spelled as a code point so the source
// stays free of the character itself.
const emDash = string(rune(0x2014))

// Report text.
const (
	inSyncFormat   = "mcpServers already in sync (%d tracked)%s"
	cachedSuffix   = " [cached]"
	planFooter     = "(dry-run " + emDash + " no changes applied)"
	uncomparedNote = "note: spec not compared, get shows another scope for: "
	skippedNote    = "note: prune skips servers outside user scope: "
	invalidNote    = "note: not managed, name not valid for claude mcp add: "
)

// Run syncs the MCP servers as mode says, reporting to out.Out and warning
// on out.Err. Check prints a one-line summary first and returns
// apply.Exit(apply.CodeDrift) on drift. A failed claude call is
// apply.CodeExternal.
func Run(cfg Config, opts Options, mode apply.Mode, out apply.Streams) error {
	claude, err := openClaude()
	if err != nil {
		return err
	}
	defer claude.close(out.Err)
	m, err := gate.Current(cfg.ProfilesFile)
	if err != nil {
		return err
	}
	desired, err := loadDesired(cfg, m, out.Err)
	if err != nil {
		return err
	}
	current, err := claude.list()
	if err != nil {
		return err
	}
	r := runner{cfg: cfg, mode: mode, out: out.Out, warn: out.Err, claude: claude}
	stampHash := desired.hash(m.Host)
	full := mode == apply.Plan || cfg.Prune || opts.Force || readStamp(cfg.Stamp) != stampHash
	d, err := r.diff(desired, current, full)
	if err != nil {
		return err
	}
	return r.reconcile(desired, d, stampHash)
}

// runner is one Run after the inputs are loaded.
type runner struct {
	cfg    Config
	mode   apply.Mode
	out    io.Writer
	warn   io.Writer
	claude claudeCLI
}

// delta is what reconciling changes, each list in name order.
type delta struct {
	add, update, prune []string
	// uncompared are the names in both sets that get shows at another
	// scope, so their user-scope spec cannot be read; they count as
	// unchanged.
	uncompared []string
	// skipped are the names prune leaves alone because no user-scope
	// remove can reach them, and invalid the names it leaves alone because
	// add would reject them; neither is drift.
	skipped, invalid []string
	unchanged        int
	// full says every server in both sets had its spec compared; false
	// means the stamp vouched for them.
	full bool
}

func (d delta) empty() bool { return len(d.add)+len(d.update)+len(d.prune) == 0 }

// diff compares the desired set with the registered names. When full, it
// asks claude for each shared server's spec; otherwise it takes them as
// unchanged. A server get shows at another scope is unchanged too: get
// cannot show its user-scope copy, if it has one, so a re-add would repeat
// on every run.
func (r runner) diff(desired desiredSet, current listing, full bool) (delta, error) {
	d := delta{full: full}
	for _, name := range desired.names() {
		if !current.names[name] {
			d.add = append(d.add, name)
			continue
		}
		if !full {
			d.unchanged++
			continue
		}
		reg, ok, err := r.claude.get(name)
		if err != nil {
			return delta{}, err
		}
		switch {
		case reg.otherScope():
			d.unchanged++
			d.uncompared = append(d.uncompared, name)
		case ok && reg.matches(desired.servers[name]):
			d.unchanged++
		default:
			d.update = append(d.update, name)
		}
	}
	if r.cfg.Prune {
		var err error
		if d.prune, d.skipped, err = r.pruneCandidates(desired, current.names); err != nil {
			return delta{}, err
		}
		// selectDesired rejects a declared name add would reject, so
		// none of these is declared.
		d.invalid = current.invalid
	}
	return d, nil
}

// pruneCandidates splits the registered names outside the desired set, in
// name order, into the ones prune removes and the ones no user-scope remove
// can reach.
func (r runner) pruneCandidates(
	desired desiredSet,
	current map[string]bool,
) (prune, skipped []string, err error) {
	var names []string
	for name := range current {
		if _, ok := desired.servers[name]; !ok {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		user, err := r.claude.userScoped(name)
		switch {
		case err != nil:
			return nil, nil, err
		case user:
			prune = append(prune, name)
		default:
			skipped = append(skipped, name)
		}
	}
	return prune, skipped, nil
}

// reconcile reports d and, in Sync, applies it and records the stamp. The
// notes follow the in-sync line or check's drift list, which keeps the
// summary first, and come before sync's changes.
func (r runner) reconcile(desired desiredSet, d delta, stampHash string) error {
	if d.empty() {
		suffix := ""
		if !d.full {
			suffix = cachedSuffix
		}
		fmt.Fprintf(r.out, inSyncFormat+"\n", d.unchanged, suffix)
		r.notes(d)
		if r.mode.Writes() {
			writeStamp(r.cfg.Stamp, stampHash, r.warn)
		}
		return nil
	}
	if r.mode == apply.Check {
		fmt.Fprintln(r.out, driftSummary(d))
		writeMarks(r.out, d)
		r.notes(d)
		return apply.Exit(apply.CodeDrift)
	}
	r.notes(d)
	for _, name := range d.add {
		fmt.Fprintf(r.out, "+ %s\n", name)
		if err := r.add(name, desired.servers[name]); err != nil {
			return err
		}
	}
	for _, name := range d.update {
		fmt.Fprintf(r.out, "~ %s\n", name)
		if err := r.remove(name); err != nil {
			return err
		}
		if err := r.add(name, desired.servers[name]); err != nil {
			return err
		}
	}
	for _, name := range d.prune {
		fmt.Fprintf(r.out, "- %s\n", name)
		if err := r.remove(name); err != nil {
			return err
		}
	}
	if !r.mode.Writes() {
		fmt.Fprintln(r.out, planFooter)
		return nil
	}
	writeStamp(r.cfg.Stamp, stampHash, r.warn)
	return nil
}

// notes prints one line naming the servers whose spec was not compared and
// one for each reason prune left servers alone, each only when it has
// names.
func (r runner) notes(d delta) {
	for _, n := range []struct {
		prefix string
		names  []string
	}{{uncomparedNote, d.uncompared}, {skippedNote, d.skipped}, {invalidNote, d.invalid}} {
		if len(n.names) == 0 {
			continue
		}
		quoted := make([]string, len(n.names))
		for i, name := range n.names {
			quoted[i] = pyjson.Repr(name)
		}
		fmt.Fprintln(r.out, n.prefix+strings.Join(quoted, ", "))
	}
}

// add prints the add call for name and, in Sync, runs it.
func (r runner) add(name string, s server) error {
	inv, err := r.claude.addInvocation(name, s)
	if err != nil {
		return err
	}
	return r.invoke("add", name, inv)
}

// remove prints the remove call for name and, in Sync, runs it.
func (r runner) remove(name string) error {
	return r.invoke("remove", name, r.claude.removeInvocation(name))
}

func (r runner) invoke(verb, name string, inv invocation) error {
	fmt.Fprintf(r.out, "  $ %s\n", inv.shown)
	if !r.mode.Writes() {
		return nil
	}
	return r.claude.change(verb, name, inv)
}

// driftSummary is check's first line, the one `ralph doctor` shows, e.g.
// "drift: 1 MCP server to add, 2 to update (run 'ralph up' to sync)".
func driftSummary(d delta) string {
	var parts []string
	for _, c := range []struct {
		n    int
		verb string
	}{{len(d.add), "add"}, {len(d.update), "update"}, {len(d.prune), "remove"}} {
		if c.n == 0 {
			continue
		}
		noun := ""
		if len(parts) == 0 {
			noun = " MCP servers"
			if c.n == 1 {
				noun = " MCP server"
			}
		}
		parts = append(parts, fmt.Sprintf("%d%s to %s", c.n, noun, c.verb))
	}
	return "drift: " + strings.Join(parts, ", ") + " (run 'ralph up' to sync)"
}

// writeMarks lists the servers check found out of sync, marked as plan
// marks them.
func writeMarks(w io.Writer, d delta) {
	for _, m := range []struct {
		mark  string
		names []string
	}{{"+", d.add}, {"~", d.update}, {"-", d.prune}} {
		for _, name := range m.names {
			fmt.Fprintf(w, "%s %s\n", m.mark, name)
		}
	}
}

// readStamp returns the recorded hash, "" when there is none.
func readStamp(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return pyjson.Strip(pyjson.Text(data))
}

// writeStamp records value. A failure is a warning: the next run compares
// every spec again, which is slow but right.
func writeStamp(path, value string, warn io.Writer) {
	err := pypath.MkdirParents(filepath.Dir(path))
	if err == nil {
		err = os.WriteFile(path, []byte(value+"\n"), 0o666)
	}
	if err != nil {
		apply.Warn(warn, "could not write sync stamp %s: %s", path, pypath.ErrorText(err))
	}
}
