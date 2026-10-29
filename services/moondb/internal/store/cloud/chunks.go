package cloud

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// chunkPrefix is where the chunks live in the bucket: objects/<sha>.
const chunkPrefix = "objects/"

// errCorruptChunk is returned by Get when an object's content no longer
// hashes to its key.
var errCorruptChunk = errors.New("cloud: chunk content does not match its key")

// chunkStore keeps each chunk gzipped in the bucket at objects/<sha>. An
// object is only ever created when absent (ifGenerationMatch=0), so two
// writers of one key never replace each other's object. A key once seen
// in the bucket is remembered for the life of the process, since nothing
// deletes a chunk, and Put and Has then answer without a request.
type chunkStore struct {
	bucket *Bucket
	mu     sync.Mutex
	known  map[string]struct{}
}

func newChunkStore(b *Bucket) *chunkStore {
	return &chunkStore{bucket: b, known: map[string]struct{}{}}
}

// Put stores data and returns its key. Bytes already stored are not sent
// again.
func (c *chunkStore) Put(ctx context.Context, data []byte) (string, error) {
	sha := store.ChunkKey(data)
	if c.isKnown(sha) {
		return sha, nil
	}
	gz, err := gzipBytes(data)
	if err != nil {
		return "", fmt.Errorf("put chunk %s: %w", sha, err)
	}
	err = c.bucket.create(ctx, upload{
		object:      chunkPrefix + sha,
		size:        int64(len(gz)),
		open:        bytesBody(gz),
		contentType: "application/gzip",
		ifAbsent:    true,
	})
	if err != nil && !errors.Is(err, errExists) {
		return "", fmt.Errorf("put chunk %s: %w", sha, err)
	}
	c.remember(sha)
	return sha, nil
}

// Get returns the bytes stored under sha, after checking they still hash
// to it. A key with no object wraps store.ErrNotFound.
func (c *chunkStore) Get(ctx context.Context, sha string) ([]byte, error) {
	if !store.ValidChunkKey(sha) {
		return nil, fmt.Errorf("cloud: invalid chunk key %q", sha)
	}
	gz, err := c.bucket.read(ctx, chunkPrefix+sha)
	if err != nil {
		return nil, fmt.Errorf("chunk %s: %w", sha, err)
	}
	data, err := gunzip(gz)
	if err != nil {
		return nil, fmt.Errorf("read chunk %s: %w", sha, err)
	}
	if store.ChunkKey(data) != sha {
		return nil, fmt.Errorf("chunk %s: %w", sha, errCorruptChunk)
	}
	c.remember(sha)
	return data, nil
}

// Has reports whether an object is stored under sha, from its metadata
// when the key is not known yet.
func (c *chunkStore) Has(ctx context.Context, sha string) (bool, error) {
	if !store.ValidChunkKey(sha) {
		return false, fmt.Errorf("cloud: invalid chunk key %q", sha)
	}
	if c.isKnown(sha) {
		return true, nil
	}
	_, err := c.bucket.size(ctx, chunkPrefix+sha)
	if errors.Is(err, store.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("chunk %s: %w", sha, err)
	}
	c.remember(sha)
	return true, nil
}

func (c *chunkStore) isKnown(sha string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.known[sha]
	return ok
}

func (c *chunkStore) remember(sha string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.known[sha] = struct{}{}
}

func gzipBytes(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(data); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func gunzip(gz []byte) ([]byte, error) {
	zr, err := gzip.NewReader(bytes.NewReader(gz))
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(zr)
	if err != nil {
		return nil, err
	}
	return data, zr.Close()
}
