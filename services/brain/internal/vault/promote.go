package vault

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// noteWriter replaces the bytes of the file at path. Promote writes every
// rewritten note through one, so a test can fail a write partway.
type noteWriter func(path string, data []byte) error

func writeNoteFile(path string, data []byte) error { return atomicWrite(path, data, 0o644) }

// fileChange is one existing file a promote rewrites: the bytes it had,
// whether it existed at all, and the bytes it gets.
type fileChange struct {
	path    string
	was     []byte
	existed bool
	data    []byte
}

// promotion is everything one promote writes, computed before the first
// write: the new note, the linking notes and the MOC.
type promotion struct {
	oldPath, newPath string
	content          []byte
	rewrites         []fileChange
	moc              *fileChange
}

// apply writes p: the new note, every rewrite, the MOC, then removes the
// inbox note. It is all or nothing: on any failure every file it touched is
// put back as it was and the new note removed, and the error says whether
// that rollback was complete.
func (v *Vault) apply(p promotion, write noteWriter) (PromoteResult, error) {
	tx := &changeSet{write: write}
	if err := tx.run(v, p); err != nil {
		if rollbackErr := tx.rollback(); rollbackErr != nil {
			return PromoteResult{}, fmt.Errorf(
				"promote %s: %w; the rollback failed too, so check these files by hand: %w",
				v.Rel(p.oldPath), err, rollbackErr,
			)
		}
		return PromoteResult{}, fmt.Errorf("promote %s: %w; nothing was changed", v.Rel(p.oldPath), err)
	}
	res := PromoteResult{OldPath: p.oldPath, NewPath: p.newPath}
	for _, c := range p.rewrites {
		res.BacklinkRewrites = append(res.BacklinkRewrites, c.path)
	}
	if p.moc != nil {
		res.MOCAppended = p.moc.path
	}
	return res, nil
}

// changeSet records what a promote wrote so far, to undo it.
type changeSet struct {
	write   noteWriter
	created []string
	written []fileChange
}

func (tx *changeSet) run(v *Vault, p promotion) error {
	if err := os.MkdirAll(filepath.Dir(p.newPath), 0o755); err != nil {
		return err
	}
	if err := v.create(p.newPath, p.content); err != nil {
		return err
	}
	tx.created = append(tx.created, p.newPath)
	for _, c := range p.rewrites {
		if err := tx.rewrite(v, c); err != nil {
			return fmt.Errorf("rewrite backlinks in %s: %w", v.Rel(c.path), err)
		}
	}
	if p.moc != nil {
		if err := tx.rewrite(v, *p.moc); err != nil {
			return fmt.Errorf("append to MOC %s: %w", v.Rel(p.moc.path), err)
		}
	}
	if err := os.Remove(p.oldPath); err != nil {
		return fmt.Errorf("remove %s after writing %s: %w", v.Rel(p.oldPath), v.Rel(p.newPath), err)
	}
	return nil
}

// rewrite writes c, recording it first: a write that fails partway is put
// back too.
func (tx *changeSet) rewrite(v *Vault, c fileChange) error {
	if err := v.contained(c.path); err != nil {
		return err
	}
	tx.written = append(tx.written, c)
	return tx.write(c.path, c.data)
}

// rollback restores every rewritten file, newest first, and removes the
// files the promote created.
func (tx *changeSet) rollback() error {
	var errs []error
	for i := len(tx.written) - 1; i >= 0; i-- {
		c := tx.written[i]
		if c.existed {
			errs = append(errs, atomicWrite(c.path, c.was, 0o644))
		} else if err := os.Remove(c.path); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	for _, p := range tx.created {
		if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// linkChanges applies each note's link edits in memory, sorted by path.
// Nothing is written: an edit that no longer fits its note fails here.
func linkChanges(edits map[string]noteEdits) ([]fileChange, error) {
	rels := make([]string, 0, len(edits))
	for rel := range edits {
		rels = append(rels, rel)
	}
	sort.Strings(rels)
	out := make([]fileChange, 0, len(rels))
	for _, rel := range rels {
		e := edits[rel]
		data, err := applyLinkEdits(e.note.data, e.edits)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", rel, err)
		}
		out = append(out, fileChange{path: e.note.ref.Abs(), was: e.note.data, existed: true, data: data})
	}
	return out, nil
}

// mocChange is the MOC at mocPath with a "- [[stem|title]]  <!-- added
// YYYY-MM-DD -->" line added at its end. A MOC that does not exist yet is
// created with the line alone.
func mocChange(mocPath, title, stem string, now time.Time) (*fileChange, error) {
	line := fmt.Sprintf("- [[%s|%s]]  <!-- added %s -->\n", stem, title, now.UTC().Format("2006-01-02"))
	was, err := os.ReadFile(mocPath)
	if errors.Is(err, os.ErrNotExist) {
		return &fileChange{path: mocPath, data: []byte(line)}, nil
	}
	if err != nil {
		return nil, err
	}
	data := bytes.Clone(was)
	if len(data) > 0 && data[len(data)-1] != '\n' {
		data = append(data, '\n')
	}
	return &fileChange{path: mocPath, was: was, existed: true, data: append(data, line...)}, nil
}
