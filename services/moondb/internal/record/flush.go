package record

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"time"
	"unicode/utf8"

	"github.com/mad01/thismoon/services/moondb"
)

// flushPoll is how often the Stop hook looks at the transcript again while
// it waits for the turn's final message.
const flushPoll = 20 * time.Millisecond

// awaitFinal waits up to the flush wait for the transcript's lines from
// offset on to hold the final message the Stop payload carries. Claude
// Code writes the transcript asynchronously, and at Stop the file usually
// lacks the turn's last response, which lands a few hundred milliseconds
// later. A payload without a final message has nothing to wait for. It
// reports whether the message was found; when the wait runs out the
// caller records the lines there are, as it would without the wait.
func (rec *recorder) awaitFinal(offset int64) bool {
	if rec.p.lastMessage == "" || rec.p.transcriptPath == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(rec.base, rec.opts.FlushWait)
	defer cancel()
	return waitFor(ctx, rec.p.transcriptPath, offset, textValue(rec.p.lastMessage))
}

// waitFor polls path every flushPoll until its bytes from offset on hold
// needle, and reports whether they did before ctx ended. The first look
// reads no more than the tail the turn keeps; each later one reads what was
// appended since, with enough of the bytes before it to catch a needle
// split across two reads.
func waitFor(ctx context.Context, path string, offset int64, needle []byte) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	size := sizeOf(f)
	if size < offset {
		return false // replaced: the turn's lines are gone
	}
	from := max(offset, size-moondb.MaxTranscriptBytes)
	tick := time.NewTicker(flushPoll)
	defer tick.Stop()
	for {
		found, next := scan(f, from, needle)
		if found {
			return true
		}
		from = next
		select {
		case <-ctx.Done():
			return false
		case <-tick.C:
		}
	}
}

// scan reports whether f's bytes from from to its current end hold needle,
// and where the next scan starts so a needle the end cut in two is still
// seen whole.
func scan(f *os.File, from int64, needle []byte) (bool, int64) {
	end := sizeOf(f)
	if end <= from {
		return false, from
	}
	data := make([]byte, end-from)
	n, err := f.ReadAt(data, from)
	if err != nil && err != io.EOF {
		return false, from
	}
	data = data[:n]
	if bytes.Contains(data, needle) {
		return true, end
	}
	return false, max(from, from+int64(n)-int64(len(needle))+1)
}

// sizeOf is f's current size, 0 when it cannot be read.
func sizeOf(f *os.File) int64 {
	info, err := f.Stat()
	if err != nil {
		return 0
	}
	return info.Size()
}

// textValue is how a transcript line holds text as a content block's
// value: the "text" key and the text as a JSON string. Claude Code writes
// its lines with JSON.stringify, which escapes no HTML characters and
// leaves U+2028 and U+2029 as they are, where encoding/json escapes both
// kinds, so each separator is written raw between encoded pieces.
func textValue(text string) []byte {
	out := []byte(`"text":"`)
	start := 0
	for i, r := range text {
		if r == ' ' || r == ' ' {
			out = append(append(out, jsonBody(text[start:i])...), text[i:i+utf8.RuneLen(r)]...)
			start = i + utf8.RuneLen(r)
		}
	}
	return append(append(out, jsonBody(text[start:])...), '"')
}

// jsonBody is s encoded as a JSON string without HTML escapes or quotes.
func jsonBody(s string) []byte {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(s) // a string always encodes
	body := bytes.TrimSuffix(b.Bytes(), []byte("\n"))
	return body[1 : len(body)-1]
}
