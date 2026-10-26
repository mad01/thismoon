// Package jsonl reads JSON lines files one line at a time, the shape of
// a Claude Code transcript, without ever holding an oversized line whole.
package jsonl

import (
	"bufio"
	"errors"
	"io"
)

// Scan calls fn with every line of r and the offset just past it,
// reusing the line's buffer between calls, until fn returns false. A line
// longer than maxLine is skipped and counted in the returned total; it is
// never held whole.
func Scan(r io.Reader, maxLine int, fn func(line []byte, end int64) bool) (int, error) {
	br := bufio.NewReaderSize(r, 64<<10)
	var (
		end     int64
		line    []byte
		tooLong bool
		skipped int
	)
	for {
		frag, err := br.ReadSlice('\n')
		end += int64(len(frag))
		if !tooLong && len(line)+len(frag) > maxLine {
			tooLong, line = true, line[:0]
		}
		if !tooLong {
			line = append(line, frag...)
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		if len(line) > 0 || tooLong {
			if tooLong {
				skipped++
			} else if !fn(line, end) {
				return skipped, nil
			}
			line, tooLong = line[:0], false
		}
		if errors.Is(err, io.EOF) {
			return skipped, nil
		}
		if err != nil {
			return skipped, err
		}
	}
}
