package vault

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Set is the configured vaults in config order. Config order is the order
// a search lists vaults in and the order shared vaults resolve wikilinks in.
type Set struct {
	vaults []*Vault
}

// NewSet returns a Set over vaults, kept in the order given.
func NewSet(vaults ...*Vault) *Set {
	return &Set{vaults: vaults}
}

// Vaults returns the vaults in config order.
func (s *Set) Vaults() []*Vault { return s.vaults }

// Names returns the vault names in config order.
func (s *Set) Names() []string {
	names := make([]string, 0, len(s.vaults))
	for _, v := range s.vaults {
		names = append(names, v.Name)
	}
	return names
}

// Get returns the vault named name, or ErrUnknownVault.
func (s *Set) Get(name string) (*Vault, error) {
	for _, v := range s.vaults {
		if v.Name == name {
			return v, nil
		}
	}
	return nil, fmt.Errorf(
		"%w %q (configured: %s)",
		ErrUnknownVault, name, strings.Join(s.Names(), ", "),
	)
}

// Filter returns the named vaults in config order, or the whole set when no
// name is given. An unknown name is ErrUnknownVault.
func (s *Set) Filter(names []string) (*Set, error) {
	if len(names) == 0 {
		return s, nil
	}
	keep := make(map[string]bool, len(names))
	for _, name := range names {
		if _, err := s.Get(name); err != nil {
			return nil, err
		}
		keep[name] = true
	}
	var out []*Vault
	for _, v := range s.vaults {
		if keep[v.Name] {
			out = append(out, v)
		}
	}
	return NewSet(out...), nil
}

// Default returns the vault a write targets when no vault is named. A set
// of shared vaults alone has none, which is ErrReadOnly.
func (s *Set) Default() (*Vault, error) {
	for _, v := range s.vaults {
		if v.Default {
			return v, nil
		}
	}
	return nil, fmt.Errorf(
		"%w: every configured vault is shared; add a private vault to write",
		ErrReadOnly,
	)
}

// Ref names one note: its vault and its vault-relative, slash-separated path.
type Ref struct {
	Vault *Vault
	Path  string
}

// String returns the qualified form, vault:path, that Locate accepts.
func (r Ref) String() string { return r.Vault.Name + ":" + r.Path }

// Abs returns the note's absolute path.
func (r Ref) Abs() string { return filepath.Join(r.Vault.Root, filepath.FromSlash(r.Path)) }

// Locate resolves a note reference to an existing note: vault:path names
// the vault, and a bare path is looked up in every vault of the set. A bare
// path found in more than one vault is ErrAmbiguous, listing the qualified
// candidates; one found nowhere is ErrNotFound.
func (s *Set) Locate(ref string) (Ref, error) {
	name, rel, qualified := splitRef(ref)
	if qualified {
		v, err := s.Get(name)
		if err != nil {
			return Ref{}, err
		}
		return v.locate(rel)
	}
	var found []Ref
	for _, v := range s.vaults {
		r, err := v.locate(rel)
		switch {
		case err == nil:
			found = append(found, r)
		case !errors.Is(err, ErrNotFound):
			return Ref{}, err
		}
	}
	switch len(found) {
	case 0:
		return Ref{}, fmt.Errorf("%w: %s", ErrNotFound, rel)
	case 1:
		return found[0], nil
	}
	return Ref{}, ambiguous(rel, found)
}

// locate returns the Ref of the note at the vault-relative path rel, or
// ErrNotFound when no file is there.
func (v *Vault) locate(rel string) (Ref, error) {
	full, err := v.Resolve(rel)
	if err != nil {
		return Ref{}, err
	}
	info, err := os.Stat(full)
	if errors.Is(err, fs.ErrNotExist) || (err == nil && info.IsDir()) {
		return Ref{}, fmt.Errorf("%w: %s", ErrNotFound, v.Name+":"+v.Rel(full))
	}
	if err != nil {
		return Ref{}, err
	}
	return Ref{Vault: v, Path: v.Rel(full)}, nil
}

// splitRef splits vault:path. A prefix holding a path separator or a dot is
// part of a bare path, not a vault name.
func splitRef(ref string) (name, rel string, qualified bool) {
	before, after, ok := strings.Cut(ref, ":")
	if !ok || before == "" || strings.ContainsAny(before, `/\.`) {
		return "", ref, false
	}
	return before, after, true
}

func ambiguous(what string, found []Ref) error {
	cands := make([]string, 0, len(found))
	for _, r := range found {
		cands = append(cands, r.String())
	}
	return fmt.Errorf(
		"%w: %s is in more than one vault: %s (qualify it as <vault>:<path> or pass --vault)",
		ErrAmbiguous, what, strings.Join(cands, ", "),
	)
}

// FindByTitle returns the note whose frontmatter title or alias equals
// title (case-insensitive). Each vault contributes its first match; matches
// in more than one vault are ErrAmbiguous, and none is ErrNotFound.
func (s *Set) FindByTitle(title string) (Ref, error) {
	var found []Ref
	for _, v := range s.vaults {
		rel, err := v.FindByTitle(title)
		switch {
		case err == nil:
			found = append(found, Ref{Vault: v, Path: rel})
		case !errors.Is(err, ErrNotFound):
			return Ref{}, err
		}
	}
	switch len(found) {
	case 0:
		return Ref{}, fmt.Errorf("%w: no note with title or alias %q", ErrNotFound, title)
	case 1:
		return found[0], nil
	}
	return Ref{}, ambiguous(fmt.Sprintf("title %q", title), found)
}

// List returns the notes of one tier across the set that match the filters,
// newest update first, then by vault in config order and path. Total counts
// every match before the limit.
func (s *Set) List(opts ListOptions) (Listing, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = defaultListLimit
	}
	order := make(map[string]int, len(s.vaults))
	var notes []ListedNote
	for i, v := range s.vaults {
		order[v.Name] = i
		got, err := v.collectTier(opts)
		if err != nil {
			return Listing{}, fmt.Errorf("%s: %w", v.Name, err)
		}
		notes = append(notes, got...)
	}
	sort.SliceStable(notes, func(i, j int) bool {
		a, b := notes[i], notes[j]
		if a.Updated != b.Updated {
			return a.Updated > b.Updated
		}
		if a.Vault != b.Vault {
			return order[a.Vault] < order[b.Vault]
		}
		return a.Path < b.Path
	})
	total := len(notes)
	if len(notes) > limit {
		notes = notes[:limit]
	}
	return Listing{Tier: opts.Tier.Dir(), Total: total, Notes: notes}, nil
}
