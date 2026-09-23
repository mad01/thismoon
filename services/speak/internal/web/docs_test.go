package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/speak/internal/audiocache"
	"github.com/mad01/thismoon/services/speak/internal/tts"
)

// settleTimeout bounds how long a test waits for background synthesis.
const settleTimeout = 5 * time.Second

// fakeSpeaker holds every synthesis until opened, then answers a WAV of two
// bytes of silence per character of text, or err.
type fakeSpeaker struct {
	gate     chan struct{}
	openOnce sync.Once
	err      error
	calls    atomic.Int32
}

func newFakeSpeaker() *fakeSpeaker {
	return &fakeSpeaker{gate: make(chan struct{})}
}

func (f *fakeSpeaker) open() { f.openOnce.Do(func() { close(f.gate) }) }

func (f *fakeSpeaker) Synthesize(ctx context.Context, req tts.Request) (tts.Audio, error) {
	f.calls.Add(1)
	select {
	case <-f.gate:
	case <-ctx.Done():
		return tts.Audio{}, ctx.Err()
	}
	if f.err != nil {
		return tts.Audio{}, f.err
	}
	pcm := make([]byte, 2*len(req.Text))
	return tts.Audio{Data: tts.WAV(pcm, 24000, 1), ContentType: tts.ContentTypeWAV}, nil
}

func (f *fakeSpeaker) ClipID(voice string) string { return "fake " + voice }

func newDocMux(t *testing.T, f *fakeSpeaker) *http.ServeMux {
	t.Helper()
	return NewMux(Config{
		Speaker:  f,
		Health:   tts.NewHealth("fake", "model"),
		Info:     testInfo,
		CacheDir: t.TempDir(),
	})
}

// upload posts markdown to /read. Its cleanup lets the background synthesis
// finish before the cache directory is removed.
func upload(t *testing.T, mux *http.ServeMux, f *fakeSpeaker, markdown string) readResponse {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("doc", "notes.md")
	_, _ = fw.Write([]byte(markdown))
	_ = mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/read", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /read = %d %s", rec.Code, rec.Body.String())
	}
	var got readResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode /read: %v (body %s)", err, rec.Body.String())
	}
	t.Cleanup(func() {
		f.open()
		settle(t, mux, got.Doc.ID)
	})
	return got
}

func serve(mux *http.ServeMux, method, target string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(method, target, nil))
	return rec
}

func docStatus(t *testing.T, rec *httptest.ResponseRecorder) DocStatus {
	t.Helper()
	var st DocStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil {
		t.Fatalf("decode doc status %q: %v", rec.Body.String(), err)
	}
	return st
}

// settle waits until nothing in document id is queued or generating.
func settle(t *testing.T, mux *http.ServeMux, id string) DocStatus {
	t.Helper()
	deadline := time.Now().Add(settleTimeout)
	for {
		st := docStatus(t, serve(mux, http.MethodGet, "/doc/"+id))
		if st.Total.Queued == 0 && st.Total.Generating == 0 {
			return st
		}
		if time.Now().After(deadline) {
			t.Fatalf("document %s did not settle: %+v", id, st.Total)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

var chunkKeys = regexp.MustCompile(`data-ra-chunk="([^"]+)"`)

// firstKey is the first part key tagged in rendered content.
func firstKey(t *testing.T, content string) string {
	t.Helper()
	m := chunkKeys.FindStringSubmatch(content)
	if m == nil {
		t.Fatalf("no data-ra-chunk in content:\n%s", content)
	}
	return strings.Fields(m[1])[0]
}

func TestReadReturnsDocStatus(t *testing.T) {
	f := newFakeSpeaker()
	mux := newDocMux(t, f)
	// An empty heading and a code block: a section with nothing to read.
	got := upload(t, mux, f, "## One\n\nFirst paragraph.\n\n##\n\n```\ncode only\n```\n")

	if !audiocache.ValidKey(firstKey(t, got.Content)) {
		t.Errorf("tagged key %q is not a cache key", firstKey(t, got.Content))
	}
	doc := got.Doc
	if len(doc.ID) != docIDHexLen || doc.Name != "notes.md" || len(doc.Sections) != 2 {
		t.Fatalf("doc = %+v, want a 16-hex id, the name and both sections", doc)
	}
	if doc.Sections[0].Parts != 1 || doc.Sections[1].Parts != 0 || doc.Sections[1].Section != 2 {
		t.Errorf("sections = %+v, want one part, then a code-only section with none",
			doc.Sections)
	}
	if doc.Total.Queued+doc.Total.Generating != 1 {
		t.Errorf("total = %+v, want the one part on its way", doc.Total)
	}
	if again := docStatus(t, serve(mux, http.MethodGet, "/doc/"+doc.ID)); again.ID != doc.ID {
		t.Errorf("GET /doc/%s = %+v", doc.ID, again)
	}
	if rec := serve(mux, http.MethodGet, "/doc/0123456789abcdef"); rec.Code != http.StatusNotFound {
		t.Errorf("unknown doc = %d, want 404", rec.Code)
	}

	f.open()
	if st := settle(t, mux, doc.ID); st.Total.Ready != 1 {
		t.Errorf("after synthesis: %+v, want the part ready", st.Total)
	}
}

// TestAudioWaitsForPart pins the play path: a part asked for before it is
// ready is waited for, and the wait shares the background synthesis.
func TestAudioWaitsForPart(t *testing.T) {
	f := newFakeSpeaker()
	mux := newDocMux(t, f)
	key := firstKey(t, upload(t, mux, f, "Hello there, reader.\n").Content)

	done := make(chan *httptest.ResponseRecorder, 1)
	go func() { done <- serve(mux, http.MethodGet, "/audio/"+key) }()
	select {
	case rec := <-done:
		t.Fatalf("GET /audio answered %d before its synthesis finished", rec.Code)
	case <-time.After(50 * time.Millisecond):
	}
	f.open()
	rec := <-done
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != tts.ContentTypeWAV ||
		!bytes.HasPrefix(rec.Body.Bytes(), []byte("RIFF")) {
		t.Fatalf("GET /audio = %d %s, want the WAV", rec.Code, rec.Header().Get("Content-Type"))
	}
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "max-age=") {
		t.Errorf("Cache-Control = %q, want a max-age", cc)
	}
	if n := f.calls.Load(); n != 1 {
		t.Errorf("provider saw %d syntheses, want 1", n)
	}
}

func TestAudioRejectsKeys(t *testing.T) {
	mux := newDocMux(t, newFakeSpeaker())
	cases := map[string]int{
		"/audio/not-a-key":                        http.StatusBadRequest,
		"/audio/0123456789ABCDEF0123456789ABCDEF": http.StatusBadRequest,
		"/audio/0123456789abcdef0123456789abcdef": http.StatusNotFound,
	}
	for target, want := range cases {
		rec := serve(mux, http.MethodGet, target)
		var body errorBody
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		if rec.Code != want || body.Error.Message == "" {
			t.Errorf("GET %s = %d %s, want %d with an error message", target, rec.Code,
				rec.Body.String(), want)
		}
	}
}

// TestAudioFailureAnswersLikeSpeech pins that a part that cannot be made
// answers with the speech endpoint's status and body, and that the document
// status carries the reason.
func TestAudioFailureAnswersLikeSpeech(t *testing.T) {
	f := newFakeSpeaker()
	f.err = &tts.Error{Kind: tts.KindQuota, Provider: "fake", Message: "rate limited"}
	mux := newDocMux(t, f)
	got := upload(t, mux, f, "Hello there.\n")
	f.open()

	rec := serve(mux, http.MethodGet, "/audio/"+firstKey(t, got.Content))
	var body speechError
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if rec.Code != http.StatusTooManyRequests || body.Error.Type != tts.KindQuota ||
		body.Error.Message != "rate limited" {
		t.Errorf("GET /audio = %d %s, want 429 with the quota reason", rec.Code,
			rec.Body.String())
	}
	st := settle(t, mux, got.Doc.ID)
	if st.Total.Failed != 1 || st.Reason != "rate limited" || st.Sections[0].Reason == "" {
		t.Errorf("status = %+v, want the part failed with its reason", st)
	}
}

func TestDocAudioDownload(t *testing.T) {
	f := newFakeSpeaker()
	mux := newDocMux(t, f)
	got := upload(t, mux, f, "## One\n\nAlpha.\n\n## Two\n\nBeta.\n")
	id := got.Doc.ID

	rec := serve(mux, http.MethodGet, "/doc/"+id+"/audio")
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "0 of 2 parts") {
		t.Fatalf("download before ready = %d %s, want 409 naming the ready count", rec.Code,
			rec.Body.String())
	}
	f.open()
	settle(t, mux, id)

	cases := []struct {
		query, filename string
		texts           []string
	}{
		{"", "notes.wav", []string{"One. Alpha.", "Two. Beta."}},
		{"?section=2", "notes-section-2.wav", []string{"Two. Beta."}},
	}
	for _, tc := range cases {
		rec := serve(mux, http.MethodGet, "/doc/"+id+"/audio"+tc.query)
		if rec.Code != http.StatusOK {
			t.Fatalf("download%s = %d %s", tc.query, rec.Code, rec.Body.String())
		}
		disposition, params, err := mime.ParseMediaType(rec.Header().Get("Content-Disposition"))
		if err != nil || disposition != "attachment" || params["filename"] != tc.filename {
			t.Errorf("download%s Content-Disposition = %q, want an attachment named %s",
				tc.query, rec.Header().Get("Content-Disposition"), tc.filename)
		}
		samples := 0
		for _, text := range tc.texts {
			samples += 2 * len(text)
		}
		if rec.Body.Len() != 44+samples { // one 44-byte header over every part's samples
			t.Errorf("download%s is %d bytes, want %d", tc.query, rec.Body.Len(), 44+samples)
		}
	}
	for _, bad := range []string{"?section=0", "?section=3", "?section=two"} {
		if rec := serve(mux, http.MethodGet, "/doc/"+id+"/audio"+bad); rec.Code != 400 {
			t.Errorf("download%s = %d, want 400", bad, rec.Code)
		}
	}
}

// TestPrepareQueuesPastTheCap pins the upload cap: parts past about 30
// minutes of speech stay idle until the page asks to prepare them.
func TestPrepareQueuesPastTheCap(t *testing.T) {
	const paragraphs = 60
	filler := strings.Repeat("Words to read aloud at length. ", 17) // 527 characters
	var md strings.Builder
	for i := range paragraphs {
		fmt.Fprintf(&md, "Paragraph %03d. %s\n\n", i, filler) // one part each
	}
	partChars := len(fmt.Sprintf("Paragraph %03d. %s", 0, strings.TrimSpace(filler)))
	wantQueued := autoPrepareChars / partChars

	f := newFakeSpeaker()
	mux := newDocMux(t, f)
	doc := upload(t, mux, f, md.String()).Doc
	if doc.Total.Parts != paragraphs || doc.Total.Queued+doc.Total.Generating != wantQueued ||
		doc.Total.Idle != paragraphs-wantQueued {
		t.Fatalf("after upload: %+v, want %d of %d parts on their way", doc.Total, wantQueued,
			paragraphs)
	}

	rec := serve(mux, http.MethodPost, "/doc/"+doc.ID+"/prepare")
	if st := docStatus(t, rec); st.Total.Idle != 0 ||
		st.Total.Queued+st.Total.Generating != paragraphs {
		t.Errorf("after prepare: %+v, want every part on its way", st.Total)
	}
	f.open()
	if st := settle(t, mux, doc.ID); st.Total.Ready != paragraphs {
		t.Errorf("after synthesis: %+v, want all %d ready", st.Total, paragraphs)
	}
}

var sectionParts = regexp.MustCompile(`data-ra-parts="([^"]+)"`)

// waitPart polls until the preparer reports want for key.
func waitPart(t *testing.T, s *docServer, key string, want audiocache.State) {
	t.Helper()
	deadline := time.Now().Add(settleTimeout)
	for {
		got, _ := s.prep.State(key)
		if got == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("part %s = %s, want %s", key, got, want)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// TestEvictionDropsQueuedParts pins the registry cap: when a document falls
// out, its parts still waiting go back to idle and are never synthesized,
// while the ones already generating finish into the cache.
func TestEvictionDropsQueuedParts(t *testing.T) {
	f := newFakeSpeaker()
	cfg := Config{Speaker: f, Health: tts.NewHealth("fake", "model"), CacheDir: t.TempDir()}
	s := newDocServer(cfg, audiocache.NewStore(cfg.CacheDir))
	mux := http.NewServeMux()
	s.routes(mux)

	var md strings.Builder
	for i := range prepareWorkers + 1 { // one part more than there are workers
		fmt.Fprintf(&md, "Part %d. %s\n\n", i, strings.Repeat("Some words to read. ", 20))
	}
	first := upload(t, mux, f, md.String())
	keys := strings.Fields(sectionParts.FindStringSubmatch(first.Content)[1])
	for _, key := range keys[:prepareWorkers] {
		waitPart(t, s, key, audiocache.StateGenerating)
	}
	for i := range maxDocs {
		upload(t, mux, f, fmt.Sprintf("Document %d.\n", i))
	}

	if rec := serve(mux, http.MethodGet, "/doc/"+first.Doc.ID); rec.Code != http.StatusNotFound {
		t.Fatalf("GET the evicted document = %d, want 404", rec.Code)
	}
	waiting := keys[prepareWorkers]
	if state, _ := s.prep.State(waiting); state != audiocache.StateIdle {
		t.Errorf("evicted document's waiting part = %s, want idle", state)
	}
	f.open()
	for _, key := range keys[:prepareWorkers] {
		waitPart(t, s, key, audiocache.StateReady)
	}
	if s.store.Has(waiting) {
		t.Error("the evicted document's waiting part was synthesized anyway")
	}
}

// TestEventLimiterSpacesEvents pins that a failing provider emits at most
// one preparation event per failureEventInterval.
func TestEventLimiterSpacesEvents(t *testing.T) {
	start := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	now := start
	l := &eventLimiter{interval: failureEventInterval, now: func() time.Time { return now }}
	steps := []struct {
		at   time.Duration
		want bool
	}{
		{0, true},
		{time.Second, false},
		{failureEventInterval - time.Second, false},
		{failureEventInterval, true},
		{failureEventInterval + time.Second, false},
		{3 * failureEventInterval, true},
	}
	for _, step := range steps {
		now = start.Add(step.at)
		if got := l.allow(); got != step.want {
			t.Errorf("allow() at +%s = %v, want %v", step.at, got, step.want)
		}
	}
}

// TestWhileKeptSkipsEvictedDocuments pins the queueing guard: once a
// document is evicted (and its parts forgotten), queueing it does nothing.
func TestWhileKeptSkipsEvictedDocuments(t *testing.T) {
	var forgotten []string
	r := docRegistry{forget: func(keys []string) { forgotten = append(forgotten, keys...) }}
	first := newDocument("first.md", [][]Part{{{Key: "k0", Text: "zero"}}})
	r.add(first)
	for i := range maxDocs {
		r.add(newDocument("other.md", [][]Part{{{Key: fmt.Sprintf("k%d", i+1), Text: "x"}}}))
	}
	if ran := r.whileKept(first, func() { t.Error("ran for an evicted document") }); ran {
		t.Error("whileKept reported running for an evicted document")
	}
	if len(forgotten) != 1 || forgotten[0] != "k0" {
		t.Errorf("forgotten = %v, want the evicted document's part k0", forgotten)
	}
}
