package library

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/mad01/thismoon/services/brain/internal/vault"
)

// rejectingEmbed is countingEmbed behind an embed model's input limit: a
// text over max bytes fails the way Ollama answers a prompt longer than the
// model's context, and every text fails while down is set.
type rejectingEmbed struct {
	countingEmbed
	max  int
	down atomic.Bool
}

func (e *rejectingEmbed) embed(ctx context.Context, text string) ([]float32, error) {
	if e.down.Load() {
		return nil, errors.New("embed: ollama unreachable: connection refused")
	}
	if len(text) > e.max {
		return nil, errors.New(`embed: ollama 500 Internal Server Error: ` +
			`{"error":"the input length exceeds the context length"}`)
	}
	return e.countingEmbed.embed(ctx, text)
}

// longMOC is a map of content whose list has no blank line, so the chunker
// reads it as one paragraph of about 17 KB.
func longMOC() string {
	var b strings.Builder
	b.WriteString("---\ntitle: Solo Founder Business\n---\n")
	for i := range 300 {
		fmt.Fprintf(&b, "- [[solo-founder-topic-%03d]] pricing and positioning notes\n", i)
	}
	return b.String()
}

func withLongMOC() map[string]string {
	notes := map[string]string{"MOCs/solo.md": longMOC()}
	for rel, content := range semanticNotes {
		notes[rel] = content
	}
	return notes
}

// TestSearchesEmbedALongNoteInParts pins that a note longer than the embed
// model's context is embedded in parts the model takes, so it is found and
// the searches answer without a warning. The embedder takes 2048 bytes, a
// 2048-token context when every byte is a token.
func TestSearchesEmbedALongNoteInParts(t *testing.T) {
	f := newSemanticFixture(t, withLongMOC())
	lib, _ := f.openWith("m", (&rejectingEmbed{max: 2048}).embed)
	ctx := context.Background()

	res, err := lib.HybridSearch(ctx, lib.Set(), HybridQuery{Text: "pricing positioning"})
	if err != nil {
		t.Fatalf("hybrid search over a long note = %v", err)
	}
	if len(res.Warnings) > 0 {
		t.Errorf("hybrid warnings = %q, want none", res.Warnings)
	}
	if !f.paths(lib, "solo founder pricing positioning")["MOCs/solo.md"] {
		t.Error("semantic search does not find the long note")
	}
}

// TestASearchSkipsANoteTheEmbedderRejects pins that a note the embedder
// still refuses never fails a search: the other notes are found and every
// query names the missing note in a warning until a sync embeds it.
func TestASearchSkipsANoteTheEmbedderRejects(t *testing.T) {
	f := newSemanticFixture(t, withLongMOC())
	lib, _ := f.openWith("m", (&rejectingEmbed{max: 300}).embed)
	ctx := context.Background()

	for range 2 { // the sync, then a query over the fresh index
		res, err := lib.HybridSearch(ctx, lib.Set(), HybridQuery{Text: "alpha apple"})
		if err != nil {
			t.Fatalf("hybrid search = %v, want an answer", err)
		}
		if len(res.Hits) == 0 || res.Hits[0].Path != "notes/a.md" {
			t.Errorf("hybrid hits = %+v, want notes/a.md first", res.Hits)
		}
		if len(res.Warnings) != 1 ||
			!strings.Contains(
				res.Warnings[0],
				"personal: MOCs/solo.md is not in the semantic index",
			) ||
			!strings.Contains(res.Warnings[0], "exceeds the context length") {
			t.Errorf("hybrid warnings = %q, want one naming MOCs/solo.md and why", res.Warnings)
		}
	}
	sem, err := lib.SemanticSearch(ctx, lib.Set(), SemanticQuery{Text: "bravo zebra"})
	if err != nil || len(sem.Hits) == 0 || sem.Hits[0].Path != "notes/b.md" {
		t.Fatalf("semantic search = %+v, %v; want notes/b.md first", sem.Hits, err)
	}
	if len(sem.Warnings) != 1 || !strings.Contains(sem.Warnings[0], "MOCs/solo.md") {
		t.Errorf("semantic warnings = %q, want one naming MOCs/solo.md", sem.Warnings)
	}

	f.write("MOCs/solo.md", "---\ntitle: Solo Founder Business\n---\nsolo founder pricing\n")
	sem, err = lib.SemanticSearch(ctx, lib.Set(), SemanticQuery{Text: "solo founder pricing"})
	if err != nil || len(sem.Warnings) > 0 || len(sem.Hits) == 0 ||
		sem.Hits[0].Path != "MOCs/solo.md" {
		t.Errorf("after the note shrank: hits %+v, warnings %q, %v; want it found, no warning",
			sem.Hits, sem.Warnings, err)
	}
}

// TestAnEmbedderLostMidSyncLeavesTheIndexStale pins that an embedder that
// stops answering during a sync reads as Ollama down, not as notes it
// rejected: the query warns that the index was not refreshed, and the next
// query, with the embedder back, embeds what the first one missed.
func TestAnEmbedderLostMidSyncLeavesTheIndexStale(t *testing.T) {
	f := newSemanticFixture(t, semanticNotes)
	e := &rejectingEmbed{max: 1 << 20}
	lib, v := f.openWith("m", e.embed)
	f.refresh(lib, v, false)
	f.write("notes/d.md", "---\ntitle: D\n---\ndelta dolphin\n")
	probes := 0
	lib.health = func(context.Context) error {
		if probes++; probes == 1 {
			return nil // up for the probe before the sync, gone after it
		}
		return errors.New("connection refused")
	}
	e.down.Store(true)

	res, err := lib.HybridSearch(context.Background(), lib.Set(), HybridQuery{Text: "dolphin"})
	if err != nil || len(res.Hits) == 0 || res.Hits[0].Path != "notes/d.md" {
		t.Fatalf("hybrid search = %+v, %v; want the literal hit", res.Hits, err)
	}
	warnings := strings.Join(res.Warnings, "\n")
	if !strings.Contains(warnings, "personal: semantic index is stale and was not refreshed") ||
		strings.Contains(warnings, "not in the semantic index") {
		t.Errorf("warnings = %q, want the index stale, no note named as failed", res.Warnings)
	}

	e.down.Store(false)
	lib.health = func(context.Context) error { return nil }
	if !f.paths(lib, "delta dolphin")["notes/d.md"] {
		t.Error("the next query did not embed the note the lost sync missed")
	}
}

// TestTheDuplicateCheckEmbedsALongDraft pins that the semantic duplicate
// check a write runs embeds a draft whose first paragraph is longer than the
// embed model's context, so the check runs instead of warning.
func TestTheDuplicateCheckEmbedsALongDraft(t *testing.T) {
	f := newSemanticFixture(t, semanticNotes)
	lib, v := f.openWith("m", (&rejectingEmbed{max: 2048}).embed)
	ctx := context.Background()
	f.refresh(lib, v, true)
	body := strings.Repeat("Rolling node pools one zone at a time keeps the plane reachable. ", 40)
	_, warnings, err := lib.Write(ctx, v, vault.WriteOptions{
		Tier:  vault.TierInbox,
		Title: "Zonal rollout",
		Body:  body,
	}, nil)
	if err != nil || len(warnings) > 0 {
		t.Errorf(
			"write of a long draft = %v, warnings %q; want written, the check run",
			err,
			warnings,
		)
	}
}
