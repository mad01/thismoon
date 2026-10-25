package record

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/mad01/thismoon/services/moondb"
	"github.com/mad01/thismoon/services/moondb/internal/spool"
)

// pipeGrace bounds how long a git call waits for its stdout pipe after git
// has exited or been killed, as in kit/gitloc.
const pipeGrace = 100 * time.Millisecond

// chunkSet collects the chunks one envelope points at, keyed the way the
// spool stores them.
type chunkSet map[string][]byte

// add stores data and returns its key. Empty data is a real chunk: an
// empty file is content, not the absence of it.
func (c chunkSet) add(data []byte) string {
	key := spool.ChunkKey(data)
	c[key] = data
	return key
}

// observed is a file as read from disk after the tool ran. read is set
// when data holds the whole file; past the chunk cap only size and blob
// are known.
type observed struct {
	data []byte
	read bool
	blob string
	size int64
}

// observe reads path, or only hashes it when it is over the chunk cap. A
// file that is missing, not regular, or changes size under the read yields
// the zero observed; a hash the budget cuts short yields no blob.
func observe(ctx context.Context, path string) observed {
	// Stat before opening: opening a FIFO would block until a writer came.
	if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
		return observed{}
	}
	f, err := os.Open(path)
	if err != nil {
		return observed{}
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return observed{}
	}
	if info.Size() > moondb.MaxChunkBytes {
		return observed{size: info.Size(), blob: hashBlob(ctx, f, info.Size())}
	}
	data, err := io.ReadAll(io.LimitReader(f, moondb.MaxChunkBytes+1))
	if err != nil || len(data) > moondb.MaxChunkBytes {
		return observed{}
	}
	return observed{data: data, read: true, blob: blobID(data), size: int64(len(data))}
}

// blobID is the id git gives data as a blob: the SHA-1 of a "blob <len>"
// header, a NUL, and the bytes. It is the join key between recorded
// content and a commit's tree.
func blobID(data []byte) string {
	h := sha1.New()
	fmt.Fprintf(h, "blob %d\x00", len(data))
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

// hashBlob is blobID for a file too large to hold, streamed from r. It
// returns "" when ctx ends first or r does not hold exactly size bytes.
func hashBlob(ctx context.Context, r io.Reader, size int64) string {
	h := sha1.New()
	fmt.Fprintf(h, "blob %d\x00", size)
	n, err := io.Copy(h, ctxReader{ctx: ctx, r: r})
	if err != nil || n != size {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))
}

// ctxReader stops reading once ctx is done.
type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (c ctxReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}

// detectEOL names the line ending convention of data: "lf", "crlf",
// "mixed", or "" for content without a line break.
func detectEOL(data []byte) string {
	lf := bytes.Count(data, []byte("\n"))
	crlf := bytes.Count(data, []byte("\r\n"))
	switch {
	case lf == 0:
		return ""
	case crlf == 0:
		return "lf"
	case crlf == lf:
		return "crlf"
	default:
		return "mixed"
	}
}

// gitBytes runs one git command in dir and returns its stdout untouched,
// for file content that gitloc.Output would trim. It gives up, killing
// git, once the output passes limit bytes or ctx ends.
func gitBytes(ctx context.Context, dir string, limit int64, args ...string) ([]byte, bool) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.WaitDelay = pipeGrace
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, false
	}
	if err := cmd.Start(); err != nil {
		return nil, false
	}
	data, readErr := io.ReadAll(io.LimitReader(out, limit+1))
	if readErr != nil || int64(len(data)) > limit {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, false
	}
	if err := cmd.Wait(); err != nil && !errors.Is(err, exec.ErrWaitDelay) {
		return nil, false
	}
	return data, true
}
