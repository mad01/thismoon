package cloud

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/mad01/thismoon/services/moondb/internal/store"
	"github.com/mad01/thismoon/services/moondb/internal/store/local"
)

// segmentPrefix is where the copy of the log lives in the bucket:
// log/<segment file name>.
const segmentPrefix = "log/"

// segmentCopier keeps a copy of the log in the bucket: each segment once
// the log has moved past it, and the open one at every copy tick. It
// remembers the size it copied of each segment, so a segment is sent again
// only once it has grown. Its state is owned by one goroutine at a time:
// run's, and Close's after run has returned.
type segmentCopier struct {
	dir    string
	bucket *Bucket
	logf   func(format string, args ...any)
	copied map[int]int64
}

func newSegmentCopier(dir string, b *Bucket, logf func(string, ...any)) *segmentCopier {
	return &segmentCopier{dir: dir, bucket: b, logf: logf, copied: map[int]int64{}}
}

// run copies the closed segments at once and at every pollEvery, and the
// open one at every copyEvery, until ctx ends. A failed copy is logged and
// tried again at the next tick.
func (c *segmentCopier) run(ctx context.Context, pollEvery, copyEvery time.Duration) {
	poll := time.NewTicker(pollEvery)
	defer poll.Stop()
	open := time.NewTicker(copyEvery)
	defer open.Stop()
	c.logErr(ctx, c.copyClosed(ctx))
	for {
		select {
		case <-ctx.Done():
			return
		case <-poll.C:
			c.logErr(ctx, c.copyClosed(ctx))
		case <-open.C:
			c.logErr(ctx, c.copyOpen(ctx))
		}
	}
}

func (c *segmentCopier) logErr(ctx context.Context, err error) {
	if err != nil && ctx.Err() == nil {
		c.logf("moondb: copy log to bucket %s: %v", c.bucket.Name(), err)
	}
}

// copyClosed copies every segment before the last one that the bucket
// does not hold whole.
func (c *segmentCopier) copyClosed(ctx context.Context) error {
	segs, err := local.Segments(c.dir)
	if err != nil || len(segs) == 0 {
		return err
	}
	for _, n := range segs[:len(segs)-1] {
		if err := c.copySegment(ctx, n); err != nil {
			return err
		}
	}
	return nil
}

// copyOpen copies the last segment, the one appends go to.
func (c *segmentCopier) copyOpen(ctx context.Context) error {
	segs, err := local.Segments(c.dir)
	if err != nil || len(segs) == 0 {
		return err
	}
	return c.copySegment(ctx, segs[len(segs)-1])
}

// copyAll copies the closed segments, then the open one.
func (c *segmentCopier) copyAll(ctx context.Context) error {
	return errors.Join(c.copyClosed(ctx), c.copyOpen(ctx))
}

// copySegment sends segment n as it is on disk now, unless it is empty or
// the bucket already holds that many bytes of it. A segment this process has not
// copied yet is looked up in the bucket first, so a restart does not send
// the whole log again. The open segment can end in a line being appended;
// a store opened from the copy drops such a torn line, as it does after a
// crash.
func (c *segmentCopier) copySegment(ctx context.Context, n int) error {
	path := local.SegmentPath(c.dir, n)
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat segment: %w", err)
	}
	size := info.Size()
	object := segmentPrefix + filepath.Base(path)
	done, seen := c.copied[n]
	if size == 0 || seen && done == size {
		return nil
	}
	if !seen {
		got, err := c.bucket.size(ctx, object)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return err
		}
		if err == nil && got == size {
			c.copied[n] = size
			return nil
		}
	}
	err = c.bucket.create(ctx, upload{
		object:      object,
		size:        size,
		open:        fileBody(path, size),
		contentType: "application/x-ndjson",
	})
	if err != nil {
		return err
	}
	c.copied[n] = size
	return nil
}

// fileBody opens the first size bytes of the file at path as an upload
// body, so the upload sends the size it announced whatever is appended
// meanwhile.
func fileBody(path string, size int64) func() (io.ReadCloser, error) {
	return func() (io.ReadCloser, error) {
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		return struct {
			io.Reader
			io.Closer
		}{io.LimitReader(f, size), f}, nil
	}
}
