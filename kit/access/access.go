// Package access is the sandbox access manifest: what one fleet process may
// reach, declared beside its code as the spec.access block of a component's
// service-info.yaml, or as a standalone access.yaml in a recipe directory
// that has no component. The manifest is the source of truth; the srt
// (anthropics/sandbox-runtime) settings JSON the kernel enforces is derived
// from it by Render, so a reviewer reads the YAML and never hand-edits the
// JSON.
//
// The model, its parsing and its validation live here. Rendering lives in
// render.go. Both are pure: Load is the only function that touches the
// filesystem.
package access

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Sandbox names the confinement a process runs under.
type Sandbox string

const (
	// SandboxSRT runs the process through srt-run under a rendered settings file.
	SandboxSRT Sandbox = "srt"
	// SandboxSeatbelt runs the process under a hand-written seatbelt profile,
	// for a need srt settings cannot express (Metal for an MLX engine).
	SandboxSeatbelt Sandbox = "seatbelt"
	// SandboxNone runs the process unconfined.
	SandboxNone Sandbox = "none"
)

// EgressAtLaunch is the egress entry for a host that differs per machine. It
// documents that the host reaches network.allowedDomains at launch through
// srt-run's SRT_RUN_ALLOW_DOMAINS, so no hostname is ever written into a
// shipped manifest. It renders to nothing.
const EgressAtLaunch = "via SRT_RUN_ALLOW_DOMAINS"

// Path symbols accepted in Reads and Writes.
const (
	// SymbolWorkdir stands for the manifest's Workdir.
	SymbolWorkdir = "workdir"
	// SymbolTemp stands for the temp roots srt-run points the child's TMPDIR
	// into, which srt does not allow on its own.
	SymbolTemp = "temp"
)

// ServiceInfoFile is the catalog descriptor whose spec.access block is a
// manifest.
const ServiceInfoFile = "service-info.yaml"

// StandaloneFile is the manifest file a recipe directory without a component
// carries.
const StandaloneFile = "access.yaml"

// ErrNoManifest reports a service-info.yaml that declares no spec.access.
var ErrNoManifest = errors.New("access: no spec.access block")

// tempDirs are the roots SymbolTemp expands to. The fleet default adds both
// to every allowWrite.
var tempDirs = []string{"/private/tmp", "/private/var/folders"}

// policyDirs are the directories no sandbox may write: the settings files
// themselves, and the ralph sources cache their links can point into. A
// confined process able to write either could rewrite its own policy for
// the next launch.
var policyDirs = []string{"~/.config/srt", "~/.config/ralph/sources"}

// Manifest is one process's declared access. Field order follows the order
// a reader asks the questions in: how is it confined, what does it listen
// on and talk to, what does it read and write.
type Manifest struct {
	// Sandbox is the confinement: srt, seatbelt or none. Required.
	Sandbox Sandbox `yaml:"sandbox"           json:"sandbox"`
	// Reason says why a process is not under srt. Required unless Sandbox is srt.
	Reason string `yaml:"reason,omitempty"  json:"reason,omitempty"`
	// Label is the srt-run label and the settings file's name
	// (~/.config/srt/<label>.json). Defaults to the component name, or to
	// the directory name for a standalone access.yaml.
	Label string `yaml:"label,omitempty"   json:"label,omitempty"`
	// Workdir is the process's own state directory, the value of the
	// workdir symbol in Reads and Writes.
	Workdir string `yaml:"workdir,omitempty" json:"workdir,omitempty"`
	// Listen is the loopback port the process serves on.
	Listen int `yaml:"listen,omitempty"  json:"listen,omitempty"`
	// Local names the loopback peers the process calls, such as events.
	// With Listen it is why allowLocalBinding is on.
	Local []string `yaml:"local,omitempty"   json:"local,omitempty"`
	// Egress lists external hosts as host or host:port, or EgressAtLaunch.
	Egress []string `yaml:"egress,omitempty"  json:"egress,omitempty"`
	// Reads lists the paths carved back out of the ~ read deny.
	Reads []string `yaml:"reads,omitempty"   json:"reads,omitempty"`
	// Writes lists the paths the process may write.
	Writes []string `yaml:"writes,omitempty"  json:"writes,omitempty"`
	// TLS marks a process that verifies certificates in-process (a Go
	// binary), which needs enableWeakerNetworkIsolation for trustd.
	TLS bool `yaml:"tls,omitempty"     json:"tls,omitempty"`
	// Deny lists extra read-deny globs under the Reads roots, such as the
	// .env, *.pem and *.key files inside a code root.
	Deny []string `yaml:"deny,omitempty"    json:"deny,omitempty"`
	// Extra is raw srt settings for anything the fields above cannot say,
	// merged last: objects merge, lists union, scalars override.
	Extra map[string]any `yaml:"extra,omitempty"   json:"extra,omitempty"`
}

// UnmarshalYAML decodes a manifest strictly: an unknown field is an error,
// so a typo cannot silently drop a deny.
func (m *Manifest) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("access: line %d: want a mapping", n.Line)
	}
	known := fieldNames()
	for i := 0; i+1 < len(n.Content); i += 2 {
		key := n.Content[i]
		if !known[key.Value] {
			return fmt.Errorf("access: line %d: unknown field %q", key.Line, key.Value)
		}
	}
	type plain Manifest
	return n.Decode((*plain)(m))
}

// fieldNames returns the YAML keys Manifest declares, read off its tags so
// the strict check cannot drift from the struct.
func fieldNames() map[string]bool {
	t := reflect.TypeFor[Manifest]()
	names := make(map[string]bool, t.NumField())
	for i := range t.NumField() {
		name, _, _ := strings.Cut(t.Field(i).Tag.Get("yaml"), ",")
		names[name] = true
	}
	return names
}

var (
	labelRe = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
	peerRe  = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
	hostRe  = regexp.MustCompile(
		`^(\*\.)?[A-Za-z0-9]([A-Za-z0-9-]*[A-Za-z0-9])?(\.[A-Za-z0-9]([A-Za-z0-9-]*[A-Za-z0-9])?)*$`,
	)
)

// Validate reports every problem with m in one error, or nil.
func (m Manifest) Validate() error {
	var p problems
	switch m.Sandbox {
	case SandboxSRT:
	case SandboxSeatbelt, SandboxNone:
		if strings.TrimSpace(m.Reason) == "" {
			p.addf("sandbox %s needs a reason", m.Sandbox)
		}
	case "":
		p.addf("sandbox is required (srt, seatbelt or none)")
	default:
		p.addf("unknown sandbox %q (srt, seatbelt or none)", m.Sandbox)
	}
	if m.Label != "" && !labelRe.MatchString(m.Label) {
		p.addf("label %q must match [A-Za-z0-9._-]+", m.Label)
	}
	if m.Workdir != "" {
		if err := checkPath(m.Workdir); err != nil {
			p.addf("workdir %q: %v", m.Workdir, err)
		} else if hasGlob(m.Workdir) {
			p.addf("workdir %q: no globs", m.Workdir)
		}
	}
	if m.Listen < 0 || m.Listen > 65535 {
		p.addf("listen %d: not a port", m.Listen)
	}
	for _, peer := range m.Local {
		if !peerRe.MatchString(peer) {
			p.addf("local %q: name a peer service, like events", peer)
		}
	}
	for _, e := range m.Egress {
		if err := checkEgress(e); err != nil {
			p.addf("egress %q: %v", e, err)
		}
	}
	m.checkEntries(&p, "reads", m.Reads)
	m.checkEntries(&p, "writes", m.Writes)
	for _, w := range m.resolve(m.Writes) {
		if dir, ok := ReachesPolicy(w); ok {
			p.addf("writes %q reaches %s, where the policy lives", w, dir)
		}
	}
	m.checkDeny(&p)
	return p.err()
}

// checkEntries validates a reads or writes list: each entry is a symbol or
// an absolute or ~-anchored path that does not lift the whole ~ deny.
func (m Manifest) checkEntries(p *problems, field string, entries []string) {
	for _, e := range entries {
		switch e {
		case SymbolTemp:
			continue
		case SymbolWorkdir:
			if m.Workdir == "" {
				p.addf("%s uses workdir, but workdir is not set", field)
			}
			continue
		case "~", "~/", "/":
			p.addf("%s %q would lift the ~ read deny; name the directories instead", field, e)
			continue
		}
		if err := checkPath(e); err != nil {
			p.addf("%s %q: %v", field, e, err)
		}
	}
}

// checkDeny validates the extra read denies. A ~-anchored deny outside every
// read root is already covered by the ~ deny, so it marks a typo.
func (m Manifest) checkDeny(p *problems) {
	roots := m.resolve(m.Reads)
	for _, d := range m.Deny {
		if err := checkPath(d); err != nil {
			p.addf("deny %q: %v", d, err)
			continue
		}
		if !strings.HasPrefix(d, "~/") {
			continue
		}
		if !underAny(LiteralPrefix(d), roots) {
			p.addf("deny %q is outside every reads root, where ~ is denied already", d)
		}
	}
}

// resolve expands the workdir and temp symbols in entries, keeping order and
// dropping duplicates.
func (m Manifest) resolve(entries []string) []string {
	var out []string
	for _, e := range entries {
		switch e {
		case SymbolWorkdir:
			if m.Workdir != "" {
				out = append(out, m.Workdir)
			}
		case SymbolTemp:
			out = append(out, tempDirs...)
		default:
			out = append(out, e)
		}
	}
	return dedupe(out)
}

// checkPath accepts an absolute or ~-anchored path. A relative path resolves
// against the process's working directory, which a sandbox wrapper sets to /.
func checkPath(path string) error {
	if strings.HasPrefix(path, "/") || strings.HasPrefix(path, "~/") {
		return nil
	}
	return errors.New("must start with / or ~/")
}

// checkEgress accepts host, *.domain, host:port or EgressAtLaunch, and
// refuses a bare * or *:port, which srt rejects anyway.
func checkEgress(e string) error {
	if e == EgressAtLaunch {
		return nil
	}
	host := e
	if h, port, ok := strings.Cut(e, ":"); ok {
		host = h
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return errors.New("port must be 1-65535")
		}
	}
	if host == "*" {
		return errors.New("a bare * allows every host; name the hosts")
	}
	if !hostRe.MatchString(host) {
		return fmt.Errorf("want host, *.domain or host:port, or %q", EgressAtLaunch)
	}
	return nil
}

// ReachesPolicy reports whether an allowWrite entry would let a process write
// a settings file or the sources cache, and names the directory it reaches.
// An entry reaches a policy directory when its literal prefix (the path up to
// the first glob) is that directory, an ancestor of it, or inside it.
func ReachesPolicy(entry string) (string, bool) {
	prefix := LiteralPrefix(entry)
	for _, dir := range policyDirs {
		if under(dir, prefix) || under(prefix, dir) {
			return dir, true
		}
	}
	return "", false
}

// LiteralPrefix returns path up to its first segment holding a glob
// character, without a trailing slash: ~/code/**/.env gives ~/code.
func LiteralPrefix(path string) string {
	segs := strings.Split(strings.TrimRight(path, "/"), "/")
	for i, s := range segs {
		if hasGlob(s) {
			segs = segs[:i]
			break
		}
	}
	return strings.Join(segs, "/")
}

func hasGlob(s string) bool { return strings.ContainsAny(s, "*?[{") }

// under reports whether path is dir or lies inside it.
func under(path, dir string) bool {
	return path == dir || strings.HasPrefix(path, dir+"/")
}

func underAny(path string, dirs []string) bool {
	for _, d := range dirs {
		if under(path, LiteralPrefix(d)) {
			return true
		}
	}
	return false
}

func dedupe(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// problems collects validation failures so a manifest reports all of them
// at once.
type problems []string

func (p *problems) addf(format string, args ...any) {
	*p = append(*p, fmt.Sprintf(format, args...))
}

func (p problems) err() error {
	if len(p) == 0 {
		return nil
	}
	return fmt.Errorf("access: %s", strings.Join(p, "; "))
}

// Parse decodes a standalone access.yaml and validates it. The label is
// left as written; Load fills the default.
func Parse(data []byte) (Manifest, error) {
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return Manifest{}, fmt.Errorf("access: %w", err)
	}
	if err := m.Validate(); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

// serviceInfoDoc is the slice of a catalog entity a manifest lives in.
type serviceInfoDoc struct {
	Kind     string `yaml:"kind"`
	Metadata struct {
		Name string `yaml:"name"`
	} `yaml:"metadata"`
	Spec struct {
		Access *Manifest `yaml:"access"`
	} `yaml:"spec"`
}

// ParseServiceInfo finds the spec.access block in a service-info.yaml, which
// may hold several entities separated by ---. Exactly one Component may
// declare it; the label defaults to that Component's name. It returns
// ErrNoManifest when no document declares one.
func ParseServiceInfo(data []byte) (Manifest, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	var found []serviceInfoDoc
	for i := 0; ; i++ {
		var doc serviceInfoDoc
		err := dec.Decode(&doc)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return Manifest{}, fmt.Errorf("access: document %d: %w", i, err)
		}
		if doc.Spec.Access == nil {
			continue
		}
		if doc.Kind != "Component" {
			return Manifest{}, fmt.Errorf(
				"access: document %d: spec.access belongs on a Component, not a %s",
				i,
				doc.Kind,
			)
		}
		found = append(found, doc)
	}
	switch len(found) {
	case 0:
		return Manifest{}, ErrNoManifest
	case 1:
	default:
		return Manifest{}, errors.New("access: more than one document declares spec.access")
	}
	m := *found[0].Spec.Access
	if err := m.Validate(); err != nil {
		return Manifest{}, err
	}
	if m.Label == "" {
		m.Label = found[0].Metadata.Name
	}
	return m, nil
}

// Load reads the manifest at path: the spec.access block of a
// service-info.yaml, or any other file as a standalone manifest whose label
// defaults to its directory's name. The returned manifest always has a label.
func Load(path string) (Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("access: %w", err)
	}
	if filepath.Base(path) == ServiceInfoFile {
		m, err := ParseServiceInfo(data)
		if err != nil {
			return Manifest{}, fmt.Errorf("%s: %w", path, err)
		}
		return m, nil
	}
	m, err := Parse(data)
	if err != nil {
		return Manifest{}, fmt.Errorf("%s: %w", path, err)
	}
	if m.Label == "" {
		abs, err := filepath.Abs(path)
		if err != nil {
			return Manifest{}, fmt.Errorf("access: %w", err)
		}
		m.Label = filepath.Base(filepath.Dir(abs))
	}
	if !labelRe.MatchString(m.Label) {
		return Manifest{}, fmt.Errorf(
			"%s: access: label %q must match [A-Za-z0-9._-]+; set label",
			path,
			m.Label,
		)
	}
	return m, nil
}
