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
	"slices"
	"strconv"
	"strings"
)

const segmentExt = ".jsonl"

// segmentName is the file name of segment n: zero-padded so the names sort
// in segment order.
func segmentName(n int) string { return fmt.Sprintf("%06d%s", n, segmentExt) }

// listSegments returns the segment numbers in dir in order. Files that are
// not segments are ignored.
func listSegments(dir string) ([]int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("list segments: %w", err)
	}
	var nums []int
	for _, e := range entries {
		base, ok := strings.CutSuffix(e.Name(), segmentExt)
		if !ok || e.IsDir() {
			continue
		}
		n, err := strconv.Atoi(base)
		if err != nil || n < 1 || segmentName(n) != e.Name() {
			continue
		}
		nums = append(nums, n)
	}
	slices.Sort(nums)
	return nums, nil
}

// replay folds segment n into the index, line by line. Only the last
// segment can end in a line without its newline, since a segment is never
// written again once the next one exists.
func (s *Store) replay(n int, last bool) error {
	path := s.segmentPath(n)
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open segment: %w", err)
	}
	defer f.Close()

	r := bufio.NewReader(f)
	var offset int64
	for lineNo := 1; ; lineNo++ {
		line, err := r.ReadBytes('\n')
		if errors.Is(err, io.EOF) {
			return s.replayTail(path, lineNo, line, offset, last)
		}
		if err != nil {
			return fmt.Errorf("read %s: %w", filepath.Base(path), err)
		}
		if err := s.replayLine(path, lineNo, line); err != nil {
			return err
		}
		offset += int64(len(line))
	}
}

// replayTail handles what follows a segment's final newline. In the last
// segment, a tail that does not parse is the remains of an append cut short
// by a crash: it is logged and truncated away. A tail that parses but lost
// its newline gets one, so the next append starts on a line of its own.
// Anywhere else, a tail that does not parse is an error.
func (s *Store) replayTail(path string, lineNo int, tail []byte, offset int64, last bool) error {
	if len(bytes.TrimSpace(tail)) == 0 {
		return nil
	}
	if !last || json.Valid(tail) {
		if err := s.replayLine(path, lineNo, tail); err != nil {
			return err
		}
		if last {
			return appendNewline(path)
		}
		return nil
	}
	logf("moondb: dropping torn final line %s:%d (interrupted append)",
		filepath.Base(path), lineNo)
	if err := os.Truncate(path, offset); err != nil {
		return fmt.Errorf("repair %s: %w", filepath.Base(path), err)
	}
	return nil
}

// replayLine folds one complete line into the index. A line of a kind this
// build does not know is skipped with a warning.
func (s *Store) replayLine(path string, lineNo int, line []byte) error {
	text := bytes.TrimSpace(line)
	if len(text) == 0 {
		return nil
	}
	var l logLine
	if err := json.Unmarshal(text, &l); err != nil {
		return fmt.Errorf("parse %s:%d: %w", filepath.Base(path), lineNo, err)
	}
	err := s.ix.apply(l.T, l.R)
	if errors.Is(err, errUnknownKind) {
		logf("moondb: skipping %s:%d: %v", filepath.Base(path), lineNo, err)
		return nil
	}
	if err != nil {
		return fmt.Errorf("apply %s:%d: %w", filepath.Base(path), lineNo, err)
	}
	return nil
}

// appendNewline terminates a final line that parsed but lost its newline.
func appendNewline(path string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, filePerm)
	if err != nil {
		return fmt.Errorf("repair %s: %w", filepath.Base(path), err)
	}
	defer f.Close()
	if _, err := f.Write([]byte("\n")); err != nil {
		return fmt.Errorf("repair %s: %w", filepath.Base(path), err)
	}
	return nil
}
