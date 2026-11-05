package store

import (
	"cmp"
	"slices"
)

// CutSession is one session's turns whose stored transcript lines a size
// cap cut, as the store alone can tell (CapCheck.Cuts).
type CutSession struct {
	SessionID string `json:"session_id"`
	// TranscriptPath is the session's transcript file, which an import
	// repairs the turns from while it exists.
	TranscriptPath string `json:"transcript_path,omitempty"`
	// Turns are the ids of the turns the cap cut: their stored lines fit
	// within it while their range, from their offset to the next turn's,
	// ran past it.
	Turns []string `json:"turns,omitempty"`
	// Tail is the session's last turn when its stored lines fit within
	// the cap. No later turn bounds its range, which runs to the
	// transcript's end, so only the transcript's size tells whether the
	// cap cut it.
	Tail *CutTail `json:"tail,omitempty"`
}

// CutTail is the last turn of a session, whose range runs from Offset to
// the end of the transcript.
type CutTail struct {
	TurnID string `json:"turn_id"`
	Offset int64  `json:"offset"`
}

// CapCheck finds the turns a transcript cap cut from what the store holds:
// each turn's transcript offset and the size of its stored lines. A cap
// keeps the last lines of a longer turn, so a turn was cut when its lines
// fit within the cap while its range, up to where the next turn's lines
// start, ran past it. Nothing reads a transcript.
type CapCheck struct {
	// Cap is the most bytes of a turn's lines the store kept.
	Cap int64
	// Kept returns the length of the bytes stored under a transcript
	// chunk key, and false when the length is not known.
	Kept func(chunk string) (int64, bool)
}

// Cuts judges the turns of session s and reports whether it found a cut
// turn or a tail. A turn is judged when it is closed or failed, has stored
// lines of a known size and an offset the store recorded: one above zero,
// or zero on the session's first turn, since a turn stored before its open
// arrived carries zero too. Every offset above zero bounds the range of
// the turns below it. A session with an open turn is still running, so it
// has no tail.
func (c CapCheck) Cuts(s Session, turns []Turn) (CutSession, bool) {
	out := CutSession{SessionID: s.ID, TranscriptPath: s.TranscriptPath}
	starts, running := turnStarts(turns)
	first := firstTurn(turns)
	for _, t := range turns {
		if t.Status == TurnOpen || t.TranscriptChunk == "" ||
			(t.TranscriptOffset == 0 && t.ID != first) {
			continue
		}
		if kept, ok := c.Kept(t.TranscriptChunk); !ok || kept > c.Cap {
			continue
		}
		i, _ := slices.BinarySearch(starts, t.TranscriptOffset+1)
		switch {
		case i < len(starts) && starts[i]-t.TranscriptOffset > c.Cap:
			out.Turns = append(out.Turns, t.ID)
		case i == len(starts) && !running:
			out.Tail = &CutTail{TurnID: t.ID, Offset: t.TranscriptOffset}
		}
	}
	return out, len(out.Turns) > 0 || out.Tail != nil
}

// turnStarts returns the offsets above zero where the turns' lines start,
// sorted, and whether any turn is still open.
func turnStarts(turns []Turn) (starts []int64, running bool) {
	for _, t := range turns {
		running = running || t.Status == TurnOpen
		if t.TranscriptOffset > 0 {
			starts = append(starts, t.TranscriptOffset)
		}
	}
	slices.Sort(starts)
	return starts, running
}

// firstTurn returns the id of the turn that started first, by start and
// then Seq; "" for no turns.
func firstTurn(turns []Turn) string {
	if len(turns) == 0 {
		return ""
	}
	return slices.MinFunc(turns, func(a, b Turn) int {
		return cmp.Or(a.StartedAt.Compare(b.StartedAt), cmp.Compare(a.Seq, b.Seq))
	}).ID
}
