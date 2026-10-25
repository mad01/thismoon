package record

import (
	"bytes"
	"os"

	"github.com/mad01/thismoon/services/moondb"
	"github.com/mad01/thismoon/services/moondb/internal/spool"
	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// openTurn records a submitted prompt and remembers where the transcript
// stood, so the close can tail the turn's lines from there.
func (rec *recorder) openTurn() error {
	offset := fileSize(rec.p.transcriptPath)
	env := rec.envelope(spool.KindTurnOpen, rec.p.promptID+":open")
	env.Turn.TranscriptOffset = offset
	chunks := chunkSet{}
	env.TurnOpen = &spool.TurnOpen{}
	if rec.p.prompt != "" {
		env.TurnOpen.PromptChunk = chunks.add([]byte(rec.p.prompt))
	}
	if err := rec.spool.Write(env, chunks); err != nil {
		return err
	}
	rec.state.openTurn(rec.p.promptID, offset)
	return nil
}

// closeTurn records the end of a turn: the last assistant message, the
// transcript lines written since the turn opened when the open was seen,
// and a head-moved report for every touched worktree whose HEAD changed.
func (rec *recorder) closeTurn() error {
	env := rec.envelope(spool.KindTurnClose, rec.p.promptID+":close")
	chunks := chunkSet{}
	tc := &spool.TurnClose{Status: store.TurnClosed}
	if rec.p.lastMessage != "" {
		tc.FinalChunk = chunks.add([]byte(rec.p.lastMessage))
	}
	if offset, ok := rec.state.turnOffset(rec.p.promptID); ok {
		env.Turn.TranscriptOffset = offset
		var tail []byte
		tail, tc.TranscriptEnd = readTail(rec.p.transcriptPath, offset, moondb.MaxTranscriptBytes)
		if len(tail) > 0 {
			tc.TranscriptChunk = chunks.add(tail)
		}
	} else {
		tc.TranscriptEnd = fileSize(rec.p.transcriptPath)
	}
	env.TurnClose = tc
	if err := rec.spool.Write(env, chunks); err != nil {
		return err
	}
	return rec.moveHeads()
}

// fileSize is the size of path, 0 when it cannot be read.
func fileSize(path string) int64 {
	if path == "" {
		return 0
	}
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

// readTail returns the bytes of path from offset to its current end, and
// that end. Over limit it keeps the last bytes, cut to start at a line.
// A file now shorter than offset was replaced, so it yields nothing.
func readTail(path string, offset, limit int64) ([]byte, int64) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, 0
	}
	end := info.Size()
	if offset > end {
		return nil, end
	}
	start := max(offset, end-limit)
	data := make([]byte, end-start)
	n, _ := f.ReadAt(data, start) // a short read keeps what it got
	data = data[:n]
	if start > offset {
		if i := bytes.IndexByte(data, '\n'); i >= 0 {
			data = data[i+1:]
		}
	}
	return data, end
}
