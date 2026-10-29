package replicate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mad01/thismoon/services/moondb"
	"github.com/mad01/thismoon/services/moondb/internal/store"
	"github.com/mad01/thismoon/services/moondb/internal/store/local"
)

const (
	// maxAnswerBytes caps how much of an answer from the instance is read.
	maxAnswerBytes = 4 << 20
	// missingPerRequest caps the chunk keys one missing request asks
	// about: a batch of snapshots can point at many thousands.
	missingPerRequest = 10000
)

// Config says what a Replicator sends and where.
type Config struct {
	// Dir is the local store's directory, whose log is sent.
	Dir string
	// Chunks is the local store's chunks.
	Chunks store.Chunks
	// Target is the shared instance's base URL.
	Target string
	// Key is the bearer key the instance accepts.
	Key string
	// State is the file that keeps the cursor between runs.
	State string
	// HTTP sends the requests; one with moondb.ReplicateTimeout when nil.
	HTTP *http.Client
	// Logf reports failed rounds; log.Printf when nil.
	Logf func(format string, args ...any)
	// Every is the pause once the log is sent; moondb.ReplicateEvery when 0.
	Every time.Duration
}

// Replicator sends a local store's log and chunks to a shared instance.
// It is driven by one goroutine: Run, or a test calling Round.
type Replicator struct {
	cfg    Config
	target string
	cursor local.Cursor
}

// New checks cfg and loads the cursor saved for its target. A cursor saved
// for another target, or one past the end of the log, starts over from the
// beginning of the log.
func New(cfg Config) (*Replicator, error) {
	target, err := parseTarget(cfg.Target)
	if err != nil {
		return nil, err
	}
	switch {
	case cfg.Key == "":
		return nil, errors.New("replicate: no key for the instance")
	case cfg.Dir == "" || cfg.State == "" || cfg.Chunks == nil:
		return nil, errors.New("replicate: the local store is not named")
	}
	if cfg.HTTP == nil {
		cfg.HTTP = &http.Client{Timeout: moondb.ReplicateTimeout}
	}
	if cfg.Logf == nil {
		cfg.Logf = log.Printf
	}
	if cfg.Every <= 0 {
		cfg.Every = moondb.ReplicateEvery
	}
	r := &Replicator{cfg: cfg, target: target}
	r.cursor, err = loadCursor(cfg.State, target, cfg.Dir, cfg.Logf)
	if err != nil {
		return nil, err
	}
	return r, nil
}

// parseTarget accepts an absolute http or https URL and drops a trailing
// slash.
func parseTarget(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("replicate: target %q is not an http or https URL", raw)
	}
	return strings.TrimRight(raw, "/"), nil
}

// Run sends rounds until ctx ends: at once while there is more to send,
// every cfg.Every once the log is sent, and after a failed round a backoff
// that doubles up to moondb.ReplicateMaxBackoff.
func (r *Replicator) Run(ctx context.Context) {
	backoff := r.cfg.Every
	for {
		n, err := r.Round(ctx)
		if ctx.Err() != nil {
			return
		}
		delay := r.cfg.Every
		switch {
		case err != nil:
			r.cfg.Logf("moondb: replicate to %s: %v (next try in %s)", r.target, err, backoff)
			delay, backoff = backoff, min(backoff*2, moondb.ReplicateMaxBackoff)
		case n > 0:
			backoff = r.cfg.Every
			continue
		default:
			backoff = r.cfg.Every
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
	}
}

// Round sends the next stretch of the log: the chunks its records point at
// that the instance lacks, then the records, and saves the cursor once the
// instance has taken them. It returns how many records it sent; 0 means
// the whole log is sent.
func (r *Replicator) Round(ctx context.Context) (int, error) {
	lines, next, err := local.ReadLog(r.cfg.Dir, r.cursor, moondb.ReplicateRecordBytes)
	if err != nil || len(lines) == 0 {
		return 0, err
	}
	refs, err := chunkRefs(ctx, r.cfg.Chunks, lines)
	if err != nil {
		return 0, err
	}
	if err := r.sendChunks(ctx, refs); err != nil {
		return 0, err
	}
	var res BatchResult
	if err := r.post(ctx, "/api/sync", Batch{Records: lines}, &res); err != nil {
		return 0, err
	}
	if res.Records+res.Skipped != len(lines) {
		return 0, fmt.Errorf("replicate: sent %d records, the instance took %d",
			len(lines), res.Records+res.Skipped)
	}
	if err := saveCursor(r.cfg.State, r.target, next); err != nil {
		return 0, err
	}
	r.cursor = next
	return len(lines), nil
}

// sendChunks asks which of refs the instance lacks and sends those, in
// batches of up to moondb.ReplicateChunkBytes; a larger chunk goes alone.
// A chunk the local store lacks too is left out with a log line.
func (r *Replicator) sendChunks(ctx context.Context, refs []string) error {
	missing, err := r.missing(ctx, refs)
	if err != nil {
		return err
	}
	var batch [][]byte
	size := 0
	for _, sha := range missing {
		data, err := r.cfg.Chunks.Get(ctx, sha)
		if errors.Is(err, store.ErrNotFound) {
			r.cfg.Logf("moondb: replicate: chunk %s is not in the local store; not sent", sha)
			continue
		}
		if err != nil {
			return err
		}
		if size > 0 && size+len(data) > moondb.ReplicateChunkBytes {
			if err := r.sendBatch(ctx, batch); err != nil {
				return err
			}
			batch, size = nil, 0
		}
		batch, size = append(batch, data), size+len(data)
	}
	return r.sendBatch(ctx, batch)
}

// missing asks the instance which of refs it lacks, missingPerRequest keys
// at a time.
func (r *Replicator) missing(ctx context.Context, refs []string) ([]string, error) {
	var out []string
	for len(refs) > 0 {
		n := min(len(refs), missingPerRequest)
		var res MissingResult
		if err := r.post(ctx, "/api/sync/missing", MissingRequest{Chunks: refs[:n]}, &res); err != nil {
			return nil, err
		}
		out, refs = append(out, res.Missing...), refs[n:]
	}
	return out, nil
}

// sendBatch sends chunks with no records. A lone chunk the instance
// refuses as too large is left out with a log line, so one oversized chunk
// does not stop the log.
func (r *Replicator) sendBatch(ctx context.Context, chunks [][]byte) error {
	if len(chunks) == 0 {
		return nil
	}
	var res BatchResult
	err := r.post(ctx, "/api/sync", Batch{Chunks: chunks}, &res)
	var se *statusError
	if errors.As(err, &se) && se.code == http.StatusRequestEntityTooLarge && len(chunks) == 1 {
		r.cfg.Logf("moondb: replicate: chunk %s (%d bytes) is too large for the instance; not sent",
			store.ChunkKey(chunks[0]), len(chunks[0]))
		return nil
	}
	if err != nil {
		return err
	}
	if res.Chunks != len(chunks) {
		return fmt.Errorf(
			"replicate: sent %d chunks, the instance stored %d",
			len(chunks),
			res.Chunks,
		)
	}
	return nil
}

// statusError is an answer from the instance that is not 2xx.
type statusError struct {
	code int
	msg  string
}

func (e *statusError) Error() string { return e.msg }

// post sends body as JSON to path on the instance with the key and decodes
// a 2xx answer into out.
func (r *Replicator) post(ctx context.Context, path string, body, out any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("replicate: encode %s: %w", path, err)
	}
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		r.target+path,
		bytes.NewReader(raw),
	)
	if err != nil {
		return fmt.Errorf("replicate: %s: %w", path, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.cfg.Key)
	res, err := r.cfg.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("replicate: %s: %w", path, err)
	}
	defer func() { _ = res.Body.Close() }()
	answer, err := io.ReadAll(io.LimitReader(res.Body, maxAnswerBytes))
	if err != nil {
		return fmt.Errorf("replicate: %s: %w", path, err)
	}
	if res.StatusCode/100 != 2 {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(answer, &e)
		return &statusError{code: res.StatusCode, msg: fmt.Sprintf(
			"replicate: %s answered %s: %s", path, res.Status, e.Error)}
	}
	if err := json.Unmarshal(answer, out); err != nil {
		return fmt.Errorf("replicate: decode %s: %w", path, err)
	}
	return nil
}
