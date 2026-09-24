package web

import (
	"slices"
	"unicode/utf8"

	"github.com/mad01/thismoon/services/speak/internal/chunk"
)

// Part is one request's worth of a section: the text synthesized in one go
// and the key its audio is cached under.
type Part struct {
	Key  string
	Text string
}

// sectionPlan is one section's parts in reading order and, per block, the
// keys of the parts that read it.
type sectionPlan struct {
	parts []Part
	// blockKeys[j] covers block j, in play order; empty, never nil, for a
	// block with nothing speakable, so it encodes as [].
	blockKeys [][]string
}

// planBlocks groups a section's block texts into parts sized by
// chunk.Prepared and names each part's audio with key. The plan depends on
// the texts alone, so a page registering the same text on every view gets
// the same keys, and so the same document, each time. A block is one piece
// unless it is longer than a part may be; then it splits at sentences, so a
// part boundary falls inside a block only when it must.
func planBlocks(texts []string, key func(string) string) sectionPlan {
	var pieces []string
	var owners []int // owners[i] is the block pieces[i] came from
	for j, text := range texts {
		for _, p := range blockPieces(text) {
			pieces = append(pieces, p)
			owners = append(owners, j)
		}
	}
	plan := sectionPlan{blockKeys: make([][]string, len(texts))}
	for j := range plan.blockKeys {
		plan.blockKeys[j] = []string{}
	}
	for _, members := range chunk.Group(pieces, chunk.Prepared) {
		own := make([]string, len(members))
		for i, idx := range members {
			own[i] = pieces[idx]
		}
		part := Part{Text: chunk.Join(own)}
		part.Key = key(part.Text)
		plan.parts = append(plan.parts, part)
		for _, idx := range members {
			j := owners[idx]
			if !slices.Contains(plan.blockKeys[j], part.Key) {
				plan.blockKeys[j] = append(plan.blockKeys[j], part.Key)
			}
		}
	}
	return plan
}

// blockPieces is a block's text made speakable, as one piece when it fits
// in a part and split into sentences when it does not.
func blockPieces(text string) []string {
	s := chunk.Speakable(text)
	switch {
	case s == "":
		return nil
	case utf8.RuneCountInString(s) <= chunk.MaxChars:
		return []string{s}
	default:
		return chunk.Pieces(s)
	}
}

// keysOf is the keys of parts in order, repeats included; empty, never nil,
// so it encodes as [].
func keysOf(parts []Part) []string {
	keys := make([]string, len(parts))
	for i, p := range parts {
		keys[i] = p.Key
	}
	return keys
}
