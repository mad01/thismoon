package web

import (
	"crypto/sha256"
	"fmt"
	"slices"
	"strings"
	"testing"
)

// sequentialKeys names parts p1, p2, ... in the order they are planned.
func sequentialKeys() func(string) string {
	n := 0
	return func(string) string {
		n++
		return fmt.Sprintf("p%d", n)
	}
}

// byText keys a part by its text, the way the audio cache does.
func byText(text string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(text)))[:8] }

// TestPlanBlocksReadsEveryBlockOnce pins the plan of short blocks: a heading
// and a few paragraphs share one part, read in order, and each block's keys
// name the part that reads it.
func TestPlanBlocksReadsEveryBlockOnce(t *testing.T) {
	got := planBlocks([]string{"Top", "Sub", "text"}, sequentialKeys())
	if len(got.parts) != 1 || got.parts[0].Text != "Top. Sub. text." {
		t.Errorf("parts = %+v, want one part reading every block", got.parts)
	}
	for j, keys := range got.blockKeys {
		if !slices.Equal(keys, []string{"p1"}) {
			t.Errorf("block %d keys = %v, want the one part", j, keys)
		}
	}
}

// TestPlanBlocksRampAndSplit pins the ramp: a first part of at most 250
// characters, then up to 600, and a block longer than a part splits at
// sentences and carries every key that reads it, in play order.
func TestPlanBlocksRampAndSplit(t *testing.T) {
	sentence := "This sentence is here to make the paragraph long enough to split. "
	got := planBlocks([]string{"Long", strings.Repeat(sentence, 20)}, sequentialKeys())
	if len(got.parts) < 3 {
		t.Fatalf("planned %d parts, want the paragraph spread over several", len(got.parts))
	}
	for i, p := range got.parts {
		limit := 600
		if i == 0 {
			limit = 250
		}
		if len(p.Text) > limit {
			t.Errorf("part %d is %d chars, over its bound %d", i+1, len(p.Text), limit)
		}
	}
	// p1 holds the heading and the paragraph's first sentences.
	if keys := got.blockKeys[0]; !slices.Equal(keys, []string{"p1"}) {
		t.Errorf("heading keys = %v, want p1", keys)
	}
	if keys := got.blockKeys[1]; !slices.Equal(keys, keysOf(got.parts)) {
		t.Errorf("paragraph keys = %v, want all %d parts in order", keys, len(got.parts))
	}
}

// TestPlanBlocksKeepsRepeatedParts pins the play order the page stamps as
// data-ra-parts: two blocks with the same text are the same part, listed
// twice, since element order cannot stand in for reading order.
func TestPlanBlocksKeepsRepeatedParts(t *testing.T) {
	long := strings.Repeat("again ", 60) // over 250: a part of its own each time
	got := planBlocks([]string{long, long}, byText)
	if len(got.parts) != 2 || got.parts[0].Key != got.parts[1].Key {
		t.Fatalf("parts = %+v, want the same part twice", got.parts)
	}
	key := got.parts[0].Key
	if !slices.Equal(keysOf(got.parts), []string{key, key}) {
		t.Errorf("keys = %v, want the repeated part listed twice", keysOf(got.parts))
	}
	for j, keys := range got.blockKeys {
		if !slices.Equal(keys, []string{key}) {
			t.Errorf("block %d keys = %v, want %s once", j, keys, key)
		}
	}
}

// TestPlanBlocksEmptyBlocksKeepTheirPlace pins the alignment the page relies
// on: a block with nothing speakable still has an entry, an empty one, and a
// section with no blocks plans no parts.
func TestPlanBlocksEmptyBlocksKeepTheirPlace(t *testing.T) {
	got := planBlocks([]string{`""`, "Spoken.", "   "}, sequentialKeys())
	if len(got.parts) != 1 || got.parts[0].Text != "Spoken." {
		t.Errorf("parts = %+v, want the one spoken block", got.parts)
	}
	if len(got.blockKeys) != 3 || got.blockKeys[0] == nil || len(got.blockKeys[0]) != 0 ||
		!slices.Equal(got.blockKeys[1], []string{"p1"}) || len(got.blockKeys[2]) != 0 {
		t.Errorf("block keys = %v, want [] p1 [] with the empty ones non-nil", got.blockKeys)
	}
	if none := planBlocks(nil, sequentialKeys()); len(none.parts) != 0 || len(none.blockKeys) != 0 {
		t.Errorf("no blocks planned %+v, want nothing", none)
	}
	if keys := keysOf(nil); keys == nil || len(keys) != 0 {
		t.Errorf("keysOf(nil) = %#v, want an empty, non-nil slice", keys)
	}
}
