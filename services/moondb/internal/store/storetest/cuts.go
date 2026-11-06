package storetest

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// transcriptCuts: TranscriptCuts sizes the stored transcript chunks and
// lists, oldest session first, the sessions in which store.CapCheck finds
// a cut turn or a tail. A turn whose chunk has no stored bytes is not
// judged, and a session with neither a cut turn nor a tail is left out.
func transcriptCuts(t *testing.T, st store.Store) {
	ctx := context.Background()
	short, err := st.Chunks().Put(ctx, []byte("ten bytes!"))
	must(t, err)
	long, err := st.Chunks().Put(ctx, []byte(strings.Repeat("x", 30)))
	must(t, err)
	unstored := strings.Repeat("0", 64)
	cut := session("s1", at(0))
	cut.TranscriptPath = "/t/s1.jsonl"
	seed(t, st, session("s2", at(10)), cut, session("s3", at(20)),
		// s1: a1 holds 10 bytes of a 50-byte range; a2 holds more than the cap.
		lines(turn("a1", "s1", store.TurnClosed, at(1)), 0, short),
		lines(turn("a2", "s1", store.TurnClosed, at(2)), 50, long),
		// s2: b1's range fits the cap, and b2 holds more than it.
		lines(turn("b1", "s2", store.TurnClosed, at(11)), 0, short),
		lines(turn("b2", "s2", store.TurnClosed, at(12)), 15, long),
		// s3: c1's chunk is not stored, so only the tail c2 is judged.
		lines(turn("c1", "s3", store.TurnClosed, at(21)), 0, unstored),
		lines(turn("c2", "s3", store.TurnClosed, at(22)), 40, short),
	)
	got, err := st.TranscriptCuts(ctx, 20)
	must(t, err)
	rows := make([]string, 0, len(got))
	for _, c := range got {
		rows = append(rows, cutLine(c))
	}
	wantIDs(t, "TranscriptCuts(20)", rows, "s1 /t/s1.jsonl [a1] -", "s3  [] c2@40")
}

// lines gives a fixture turn its transcript offset and chunk.
func lines(tr store.Turn, offset int64, chunk string) store.Turn {
	tr.TranscriptOffset, tr.TranscriptChunk = offset, chunk
	return tr
}

// cutLine is one CutSession on a line: its id, path, cut turns and tail.
func cutLine(c store.CutSession) string {
	tail := "-"
	if c.Tail != nil {
		tail = fmt.Sprintf("%s@%d", c.Tail.TurnID, c.Tail.Offset)
	}
	return fmt.Sprintf("%s %s %v %s", c.SessionID, c.TranscriptPath, c.Turns, tail)
}
