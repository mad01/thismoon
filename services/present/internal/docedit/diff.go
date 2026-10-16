package docedit

import (
	"encoding/json"
	"fmt"

	"github.com/mad01/thismoon/services/present/internal/render"
)

// SectionDiff names the sections that differ between two versions of a
// Doc. A section is named by its anchor, or by IndexPrefix and its
// position in the version it comes from when its anchor is empty or
// shared. A section that only moved is not a change.
type SectionDiff struct {
	Changed []string `json:"changed"`
	Added   []string `json:"added"`
	Removed []string `json:"removed"`
}

// keyedSection is a section's name in a diff and its canonical JSON.
type keyedSection struct {
	key string
	raw string
}

// DiffSections compares two Doc sources section by section. Either may be
// nil: the version had no such rendition, so all its sections count as
// added or removed.
func DiffSections(before, after []byte) (SectionDiff, error) {
	old, err := keyedSections(before)
	if err != nil {
		return SectionDiff{}, fmt.Errorf("older Doc: %w", err)
	}
	cur, err := keyedSections(after)
	if err != nil {
		return SectionDiff{}, fmt.Errorf("newer Doc: %w", err)
	}
	oldRaw := rawByKey(old)
	curRaw := rawByKey(cur)
	d := SectionDiff{Changed: []string{}, Added: []string{}, Removed: []string{}}
	for _, s := range cur {
		raw, ok := oldRaw[s.key]
		switch {
		case !ok:
			d.Added = append(d.Added, s.key)
		case raw != s.raw:
			d.Changed = append(d.Changed, s.key)
		}
	}
	for _, s := range old {
		if _, ok := curRaw[s.key]; !ok {
			d.Removed = append(d.Removed, s.key)
		}
	}
	return d, nil
}

// keyedSections names each section of a Doc source and keeps its JSON.
func keyedSections(src []byte) ([]keyedSection, error) {
	if src == nil {
		return nil, nil
	}
	var doc render.Doc
	if err := json.Unmarshal(src, &doc); err != nil {
		return nil, err
	}
	out := make([]keyedSection, len(doc.Sections))
	for i, s := range doc.Sections {
		raw, err := json.Marshal(s)
		if err != nil {
			return nil, fmt.Errorf("section %d: %w", i, err)
		}
		key := render.SectionAnchor(s.Heading)
		if key == "" || len(holding(doc.Sections, key, -1)) > 1 {
			key = fmt.Sprintf("%s%d", IndexPrefix, i)
		}
		out[i] = keyedSection{key: key, raw: string(raw)}
	}
	return out, nil
}

func rawByKey(sections []keyedSection) map[string]string {
	m := make(map[string]string, len(sections))
	for _, s := range sections {
		m[s.key] = s.raw
	}
	return m
}
