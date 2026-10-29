package local

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// Line is one line of the log: a record's kind and its JSON. A copy of the
// log, or a replica fed from it, carries the same lines.
type Line struct {
	Kind   string          `json:"t"`
	Record json.RawMessage `json:"r"`
}

// Decode returns the record l holds as its store type: store.Session,
// store.Turn, store.Agent, store.Edit, store.UnattributedDelta,
// store.Snapshot or store.CommitLink. A kind this build does not know
// wraps ErrUnknownKind.
func (l Line) Decode() (any, error) {
	switch l.Kind {
	case kindSession:
		return decode[store.Session](l.Record)
	case kindTurn:
		return decode[store.Turn](l.Record)
	case kindEdit:
		return decode[store.Edit](l.Record)
	case kindDelta:
		return decode[store.UnattributedDelta](l.Record)
	case kindSnapshot:
		return decode[store.Snapshot](l.Record)
	case kindCommitLink:
		return decode[store.CommitLink](l.Record)
	case kindAgent:
		return decode[store.Agent](l.Record)
	}
	return nil, fmt.Errorf("%w %q", ErrUnknownKind, l.Kind)
}

func decode[T any](raw json.RawMessage) (any, error) {
	var v T
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, fmt.Errorf("decode %T: %w", v, err)
	}
	return v, nil
}

// Cursor is a position in a store's log: a byte offset into one segment.
// The zero Cursor is the start of the log.
type Cursor struct {
	Segment int   `json:"segment"`
	Offset  int64 `json:"offset"`
}

// Segments lists the numbers of the log segments of the store under dir,
// oldest first. The last one is the segment appends go to.
func Segments(dir string) ([]int, error) {
	return listSegments(filepath.Join(dir, logDirName))
}

// SegmentPath is the file of segment n of the store under dir.
func SegmentPath(dir string, n int) string {
	return filepath.Join(dir, logDirName, segmentName(n))
}

// ReadLog returns the lines of the log under dir from c on, in log order,
// and the cursor just past the last one. It stops once the lines it read
// hold maxBytes, and returns at least one line when one is there. A line
// still being appended to the last segment, without its newline yet, is
// left for a later read, so no lines means c is at the end of the log.
// ReadLog only reads the files: it runs beside the serve that writes them.
func ReadLog(dir string, c Cursor, maxBytes int) ([]Line, Cursor, error) {
	segs, err := Segments(dir)
	if err != nil || len(segs) == 0 {
		return nil, c, err
	}
	if c.Segment == 0 {
		c = Cursor{Segment: segs[0]}
	}
	var out []Line
	read := int64(0)
	for {
		last := c.Segment >= segs[len(segs)-1]
		lines, end, eof, err := readSegment(SegmentPath(dir, c.Segment), c.Offset,
			int64(maxBytes)-read, last)
		if err != nil {
			return nil, c, err
		}
		out = append(out, lines...)
		read += end - c.Offset
		c.Offset = end
		if !eof || last || read >= int64(maxBytes) {
			return out, c, nil
		}
		c = Cursor{Segment: nextSegment(segs, c.Segment)}
	}
}

// nextSegment is the first segment after n; segs holds a later one.
func nextSegment(segs []int, n int) int {
	for _, s := range segs {
		if s > n {
			return s
		}
	}
	return n
}

// readSegment reads the lines of one segment from offset on until they
// hold budget bytes, and returns them with the offset after the last one.
// eof says the segment had no more complete lines. In the last segment a
// tail without its newline is an append in progress and stays unread; in
// an earlier one it is a line, as replay reads it.
func readSegment(path string, offset, budget int64, last bool) (
	lines []Line, end int64, eof bool, err error,
) {
	f, err := os.Open(path)
	if err != nil {
		return nil, offset, false, fmt.Errorf("open segment: %w", err)
	}
	defer f.Close()
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return nil, offset, false, fmt.Errorf("seek %s: %w", filepath.Base(path), err)
	}
	r := bufio.NewReader(f)
	end = offset
	for end-offset < budget {
		raw, err := r.ReadBytes('\n')
		if errors.Is(err, io.EOF) && (last || len(bytes.TrimSpace(raw)) == 0) {
			return lines, end, true, nil
		}
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, offset, false, fmt.Errorf("read %s: %w", filepath.Base(path), err)
		}
		l, ok, perr := parseLine(raw)
		if perr != nil {
			return nil, offset, false, fmt.Errorf("parse %s at byte %d: %w",
				filepath.Base(path), end, perr)
		}
		end += int64(len(raw))
		if ok {
			lines = append(lines, l)
		}
		if err != nil {
			return lines, end, true, nil
		}
	}
	return lines, end, false, nil
}

// parseLine decodes one log line; a blank line has nothing to decode.
func parseLine(raw []byte) (Line, bool, error) {
	text := bytes.TrimSpace(raw)
	if len(text) == 0 {
		return Line{}, false, nil
	}
	var l Line
	if err := json.Unmarshal(text, &l); err != nil {
		return Line{}, false, err
	}
	return l, true, nil
}
