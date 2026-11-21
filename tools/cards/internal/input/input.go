// Package input reads the card stream for every cards mode: JSON Lines from
// a reader, decoded one line at a time into Bubble Tea messages. A line that
// does not decode, or is too long to read, is passed on with its error, so
// one bad line never stops the stream.
package input

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"

	tea "charm.land/bubbletea/v2"

	"github.com/mad01/thismoon/tools/cards/internal/protocol"
)

// LineMsg is one non-blank input line, decoded. When the line did not
// decode, Err holds why and Message is nil.
type LineMsg struct {
	// Line is the 1-based line number in the input, blank lines counted.
	Line    int
	Message protocol.Message
	Err     error
}

// EndMsg reports that the input is done. Err is set when reading failed
// rather than reaching the end.
type EndMsg struct {
	Err error
}

// maxLineBytes caps one input line, its line ending ("\n" or "\r\n") not
// counted.
const maxLineBytes = 16 << 20

// ErrLineTooLong is the LineMsg error for a line longer than maxLineBytes.
// Read drops the line and goes on with the next one.
var ErrLineTooLong = fmt.Errorf("longer than %d MiB, skipped", maxLineBytes>>20)

// Read decodes the JSON lines in r and passes each to send as a LineMsg,
// skipping blank lines, then sends one EndMsg. It returns once r reaches its
// end or fails.
func Read(r io.Reader, send func(tea.Msg)) {
	br := bufio.NewReader(r)
	for n := 1; ; n++ {
		line, tooLong, err := readLine(br)
		switch {
		case tooLong:
			send(LineMsg{Line: n, Err: ErrLineTooLong})
		case len(line) > 0:
			msg, decodeErr := protocol.Decode(line)
			send(LineMsg{Line: n, Message: msg, Err: decodeErr})
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				err = nil
			}
			send(EndMsg{Err: err})
			return
		}
	}
}

// readLine reads the next line from br and trims white space from both
// ends, which drops its line ending. A line longer than maxLineBytes is read
// to its end and dropped, with tooLong set. err is io.EOF at the end of the
// input, which may come with a last line that had no newline. On a read
// error the partial line is dropped.
func readLine(br *bufio.Reader) (line []byte, tooLong bool, err error) {
	for {
		chunk, err := br.ReadSlice('\n')
		// Past the cap and room for a line ending, the line is too long
		// either way: read the rest of it but keep none of it.
		if len(line) <= maxLineBytes+len("\r\n") {
			line = append(line, chunk...)
		}
		switch {
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		case err != nil && !errors.Is(err, io.EOF):
			return nil, false, err
		}
		body := bytes.TrimSuffix(bytes.TrimSuffix(line, []byte("\n")), []byte("\r"))
		if len(body) > maxLineBytes {
			return nil, true, err
		}
		return bytes.TrimSpace(body), false, err
	}
}
