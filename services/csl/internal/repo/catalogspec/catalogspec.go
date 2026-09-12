// Package catalogspec reads the catalog descriptor a repo may carry at its
// root: a Backstage catalog-info.yaml, or the service-info.yaml the catalog
// service in this repo scans. Both use the Backstage entity shape
// (apiVersion, kind, metadata, spec), and csl needs only the identity fields
// of a Component from it (metadata.name, spec.owner, spec.system) so a repo
// can be found by who owns it and which system it belongs to.
//
// A repo whose root has neither, a monorepo whose descriptors sit one level
// down, say, can carry a .csl-catalog.yaml that points at the descriptor to
// read ("descriptor: services/x/service-info.yaml", relative to the repo
// root). It is a discovery nudge and nothing more: the identity still comes
// from the descriptor it names.
//
// The package is a file-format reader, not a catalog client: it never talks
// to the catalog service or to Backstage, and it does not validate the entity
// beyond what it needs, because a descriptor that the catalog would reject
// (say, a Component with no owner) still names a system worth matching on.
package catalogspec

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// PointerFile is the csl-only file at a repo root that names the descriptor
// to read, for a repo whose descriptor is not at the root. It is read ahead
// of FileNames because it is an explicit instruction to csl, and a repo adds
// it precisely when the root files do not say what csl should show.
const PointerFile = ".csl-catalog.yaml"

// FileNames are the descriptor file names looked for at a repo root after
// PointerFile, in order of precedence: the Backstage default first, then the
// catalog service's.
var FileNames = []string{"catalog-info.yaml", "service-info.yaml"}

// maxDescriptorBytes caps how much of a descriptor is read. Discovery reads
// every repo's descriptor on every walk, and a pointer file can name any
// tracked file, so a descriptor is refused rather than read whole once it is
// far larger than any real one.
const maxDescriptorBytes = 1 << 20

// Groups are the apiVersion groups (the text before the slash) a descriptor
// may declare. Any version under a listed group is accepted, so a repo on
// backstage.io/v1beta1 reads the same as one on backstage.io/v1alpha1. An
// empty apiVersion also passes, because the catalog service defaults it to
// its own group rather than rejecting the file.
var Groups = []string{"backstage.io", "catalog.mad01"}

// kindComponent is the only entity kind csl reads; System, Group, and the
// other Backstage kinds describe things a checkout is not.
const kindComponent = "Component"

// Component is the identity of a Component entity: the fields csl matches
// on and shows, and nothing else from the descriptor.
type Component struct {
	// Name is metadata.name, the entity's catalog name. It is often the
	// repo's directory name, but not always (a service can be renamed
	// without moving its checkout), which is why csl matches on it.
	Name string
	// Owner is spec.owner as written: a bare team name, or a Backstage
	// entity reference such as group:default/platform.
	Owner string
	// System is spec.system as written, empty for a Component that declares
	// none.
	System string
	// File is the absolute path of the descriptor the component was read
	// from, so a surprising owner or system can be traced to its source.
	File string
}

// entity is the lenient on-disk shape: every field optional, so a document
// that the catalog service would reject still yields whatever identity it
// carries.
type entity struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Metadata   struct {
		Name string `yaml:"name"`
	} `yaml:"metadata"`
	Spec struct {
		Owner  string `yaml:"owner"`
		System string `yaml:"system"`
	} `yaml:"spec"`
}

// Read looks for a descriptor at the root of dir and returns the first
// Component it declares: the descriptor PointerFile names when the repo has
// one, else the first of FileNames present. ok is false when dir has no
// descriptor or the descriptor declares no Component; err reports a
// descriptor that exists but cannot be read, parsed, or followed. Callers
// that only enrich a listing treat the two alike and carry on, since a broken
// descriptor is that repo's problem, not the walk's.
func Read(dir string) (c Component, ok bool, err error) {
	path, data, found, err := readFirst(dir, append([]string{PointerFile}, FileNames...))
	if err != nil || !found {
		return Component{}, false, err
	}
	if filepath.Base(path) == PointerFile {
		target, err := followPointer(dir, data)
		if err != nil {
			return Component{}, false, fmt.Errorf("catalogspec: %s: %w", path, err)
		}
		if data, err = readDescriptor(target); err != nil {
			return Component{}, false, fmt.Errorf(
				"catalogspec: %s: descriptor %s: %w", path, target, err,
			)
		}
		path = target
	}
	c, ok, err = Parse(data)
	if err != nil {
		return Component{}, false, fmt.Errorf("catalogspec: %s: %w", path, err)
	}
	if ok {
		c.File = path
	}
	return c, ok, nil
}

// readFirst returns the contents of the first of names that exists under
// dir. found is false when none does; err reports a file that exists but
// cannot be read.
func readFirst(dir string, names []string) (path string, data []byte, found bool, err error) {
	for _, name := range names {
		path = filepath.Join(dir, name)
		data, err = readDescriptor(path)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", nil, false, fmt.Errorf("catalogspec: read %s: %w", path, err)
		}
		return path, data, true, nil
	}
	return "", nil, false, nil
}

// readDescriptor reads one descriptor file, refusing one over
// maxDescriptorBytes so a mispointed pointer file cannot make every walk
// read a large blob. A missing file comes back as fs.ErrNotExist unwrapped,
// which readFirst uses to move on to the next name.
func readDescriptor(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Size() > maxDescriptorBytes {
		return nil, fmt.Errorf("%d bytes is larger than a descriptor can be (max %d)",
			info.Size(), maxDescriptorBytes)
	}
	return os.ReadFile(path)
}

// pointer is the shape of PointerFile: one key naming the descriptor.
type pointer struct {
	Descriptor string `yaml:"descriptor"`
}

// followPointer resolves a PointerFile's descriptor key to an absolute path
// under dir. The path must be relative and, once every symlink on it is
// resolved, still inside the repo: the file is committed and shared, so it
// must not be able to read an arbitrary path on whoever checks the repo out,
// and git checks symlinks out as symlinks, so a lexical check alone would
// let a committed link reach past the root.
func followPointer(dir string, data []byte) (string, error) {
	var p pointer
	if err := yaml.Unmarshal(data, &p); err != nil {
		return "", err
	}
	rel := strings.TrimSpace(p.Descriptor)
	if rel == "" {
		return "", errors.New("missing the descriptor key naming the file to read")
	}
	clean := filepath.Clean(rel)
	escapes := clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator))
	if filepath.IsAbs(clean) || escapes {
		return "", fmt.Errorf("descriptor %q must be a relative path inside the repo", rel)
	}
	target := filepath.Join(dir, clean)
	if err := insideRepo(dir, target); err != nil {
		return "", fmt.Errorf("descriptor %s: %w", rel, err)
	}
	// The path handed back is the lexical one under the repo, not the
	// resolved one, so Component.File reads like Repo.Path does.
	return target, nil
}

// insideRepo resolves target's symlinks and errors when the real path lands
// outside the real root. The root is resolved too, since a checkout can
// itself sit behind a symlink (macOS keeps /var behind /private/var, say).
func insideRepo(root, target string) error {
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	realTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		return err
	}
	sep := string(filepath.Separator)
	if realTarget != realRoot && !strings.HasPrefix(realTarget, realRoot+sep) {
		return fmt.Errorf("resolves to %s, outside the repo", realTarget)
	}
	return nil
}

// Parse returns the first Component entity in data, which may hold several
// YAML documents separated by "---" (Backstage allows a System and its
// Components in one file). Documents of another kind, another apiVersion
// group, or without a name are skipped rather than rejected; ok is false when
// no document qualifies. A document that is not valid YAML is an error, since
// a half-read descriptor would silently misname a repo.
func Parse(data []byte) (c Component, ok bool, err error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	for i := 0; ; i++ {
		var e entity
		err := dec.Decode(&e)
		if errors.Is(err, io.EOF) {
			return Component{}, false, nil
		}
		if err != nil {
			return Component{}, false, fmt.Errorf("document %d: %w", i, err)
		}
		if !isComponent(e) {
			continue
		}
		return Component{
			Name:   strings.TrimSpace(e.Metadata.Name),
			Owner:  strings.TrimSpace(e.Spec.Owner),
			System: strings.TrimSpace(e.Spec.System),
		}, true, nil
	}
}

// isComponent reports whether a document is a named Component under one of
// the accepted apiVersion groups. Kind compares case-insensitively because
// Backstage normalizes it, so "component" in a hand-written file is the same
// entity as "Component".
func isComponent(e entity) bool {
	if !strings.EqualFold(strings.TrimSpace(e.Kind), kindComponent) {
		return false
	}
	if strings.TrimSpace(e.Metadata.Name) == "" {
		return false
	}
	return groupAccepted(e.APIVersion)
}

// groupAccepted reports whether an apiVersion belongs to one of Groups. The
// version after the slash is not checked: the identity fields have been the
// same across every Backstage version, so pinning one would only drop repos.
func groupAccepted(apiVersion string) bool {
	apiVersion = strings.TrimSpace(apiVersion)
	if apiVersion == "" {
		return true
	}
	group, _, _ := strings.Cut(apiVersion, "/")
	for _, g := range Groups {
		if group == g {
			return true
		}
	}
	return false
}
