package local

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// errCorruptChunk is returned by Get when an object's content no longer
// hashes to its key.
var errCorruptChunk = errors.New("local: chunk content does not match its key")

// chunkStore keeps each chunk as a gzip file under dir, fanned out by the
// first two hex digits of its key. It needs no lock: an object is renamed
// into place whole, and two writers of one key write the same bytes.
type chunkStore struct {
	dir string
}

func (c *chunkStore) path(sha string) string {
	return filepath.Join(c.dir, sha[:2], sha+".gz")
}

// Put stores data and returns its key. Bytes already stored are not
// written again.
func (c *chunkStore) Put(ctx context.Context, data []byte) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	sha := hex.EncodeToString(sum[:])
	path := c.path(sha)
	if _, err := os.Stat(path); err == nil {
		return sha, nil
	}
	if err := writeObject(path, data); err != nil {
		return "", fmt.Errorf("put chunk %s: %w", sha, err)
	}
	return sha, nil
}

// writeObject gzips data into a temp file beside path, syncs it and renames
// it into place, so a reader sees the whole object or none.
func writeObject(path string, data []byte) (err error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			tmp.Close()
			_ = os.Remove(tmp.Name())
		}
	}()
	zw := gzip.NewWriter(tmp)
	if _, err := zw.Write(data); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// Get returns the bytes stored under sha, after checking they still hash
// to it. A key with no object wraps store.ErrNotFound.
func (c *chunkStore) Get(ctx context.Context, sha string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := checkKey(sha); err != nil {
		return nil, err
	}
	f, err := os.Open(c.path(sha))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("chunk %s: %w", sha, store.ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("open chunk %s: %w", sha, err)
	}
	defer f.Close()
	data, err := gunzip(f)
	if err != nil {
		return nil, fmt.Errorf("read chunk %s: %w", sha, err)
	}
	if sum := sha256.Sum256(data); hex.EncodeToString(sum[:]) != sha {
		return nil, fmt.Errorf("chunk %s: %w", sha, errCorruptChunk)
	}
	return data, nil
}

func gunzip(r io.Reader) ([]byte, error) {
	zr, err := gzip.NewReader(r)
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(zr)
	if err != nil {
		return nil, err
	}
	return data, zr.Close()
}

// gzipMinBytes is the smallest gzip member: a 10-byte header and an 8-byte
// trailer, the CRC-32 and then ISIZE, the length of the uncompressed bytes
// mod 2^32, both little-endian.
const gzipMinBytes = 18

// size returns the length of the bytes stored under sha, read from ISIZE
// in the object's gzip trailer without inflating it. writeObject writes
// one gzip member, and no chunk nears 4 GiB, where ISIZE wraps. A key with
// no object wraps store.ErrNotFound, and an object too short to be gzip
// wraps errCorruptChunk.
func (c *chunkStore) size(sha string) (int64, error) {
	if err := checkKey(sha); err != nil {
		return 0, err
	}
	f, err := os.Open(c.path(sha))
	if errors.Is(err, fs.ErrNotExist) {
		return 0, fmt.Errorf("chunk %s: %w", sha, store.ErrNotFound)
	}
	if err != nil {
		return 0, fmt.Errorf("open chunk %s: %w", sha, err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return 0, fmt.Errorf("stat chunk %s: %w", sha, err)
	}
	if info.Size() < gzipMinBytes {
		return 0, fmt.Errorf("chunk %s: %w", sha, errCorruptChunk)
	}
	var isize [4]byte
	if _, err := f.ReadAt(isize[:], info.Size()-int64(len(isize))); err != nil {
		return 0, fmt.Errorf("read chunk %s: %w", sha, err)
	}
	return int64(binary.LittleEndian.Uint32(isize[:])), nil
}

// Has reports whether an object is stored under sha, without reading it.
func (c *chunkStore) Has(ctx context.Context, sha string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if err := checkKey(sha); err != nil {
		return false, err
	}
	_, err := os.Stat(c.path(sha))
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("stat chunk %s: %w", sha, err)
	}
	return true, nil
}

// checkKey rejects anything but 64 lowercase hex digits, which also keeps
// a caller's key from naming a path outside the object directory.
func checkKey(sha string) error {
	if len(sha) != sha256.Size*2 {
		return fmt.Errorf("local: invalid chunk key %q", sha)
	}
	for _, c := range []byte(sha) {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return fmt.Errorf("local: invalid chunk key %q", sha)
		}
	}
	return nil
}
