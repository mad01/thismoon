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
	"slices"
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
// bytes of silence per character of text, or the error set with failWith.
type fakeSpeaker struct {
	gate     chan struct{}
	openOnce sync.Once
	calls    atomic.Int32

	mu  sync.Mutex
	err error
}

func newFakeSpeaker() *fakeSpeaker {
	return &fakeSpeaker{gate: make(chan struct{})}
}

func (f *fakeSpeaker) open() { f.openOnce.Do(func() { close(f.gate) }) }

// failWith makes every synthesis from now on fail with err; nil ends that.
func (f *fakeSpeaker) failWith(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = err
}

func (f *fakeSpeaker) Synthesize(ctx context.Context, req tts.Request) (tts.Audio, error) {
	f.calls.Add(1)
	select {
	case <-f.gate:
	case <-ctx.Done():
		return tts.Audio{}, ctx.Err()
	}
	f.mu.Lock()
	err := f.err
	f.mu.Unlock()
	if err != nil {
		return tts.Audio{}, err
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

// register posts body to /read under contentType: the JSON form when that
// is application/json.
func register(mux http.Handler, body, contentType string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/read", strings.NewReader(body))
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

// registered decodes a JSON registration's answer, failing on any other.
func registered(t *testing.T, rec *httptest.ResponseRecorder) registerResponse {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /read (JSON) = %d %s", rec.Code, rec.Body.String())
	}
	var got registerResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode /read: %v (body %s)", err, rec.Body.String())
	}
	return got
}

// TestReadJSONRegistersWithoutSynthesis pins the JSON form of POST /read: the
// keys come back aligned with the request, the document is kept, nothing is
// queued, and a part plays on demand all the same.
func TestReadJSONRegistersWithoutSynthesis(t *testing.T) {
	f := newFakeSpeaker()
	mux := newDocMux(t, f)
	body := `{"name":"Briefing","sections":[` +
		`{"blocks":["Overview","First paragraph.","A list item"]},{"blocks":["Second section."]}]}`
	got := registered(t, register(mux, body, "application/json; charset=utf-8"))
	t.Cleanup(func() {
		f.open()
		settle(t, mux, got.Doc.ID)
	})

	if got.Name != "Briefing" || got.Doc.Name != "Briefing" || len(got.Sections) != 2 {
		t.Fatalf("response = %+v, want the name and both sections", got)
	}
	first := got.Sections[0]
	if len(first.Parts) != 1 || !audiocache.ValidKey(first.Parts[0]) || len(first.Blocks) != 3 {
		t.Fatalf("section 1 = %+v, want three short blocks in one part with a cache key", first)
	}
	for j, keys := range first.Blocks {
		if len(keys) != 1 || keys[0] != first.Parts[0] {
			t.Errorf("block %d keys = %v, want the section's one part", j, keys)
		}
	}
	if second := got.Sections[1]; len(second.Parts) != 1 || len(second.Blocks) != 1 {
		t.Errorf("section 2 = %+v, want one block in one part", second)
	}
	if st := got.Doc.Total; st.Parts != 2 || st.Idle != 2 {
		t.Errorf("total = %+v, want both parts idle: registration queues nothing", st)
	}
	if n := f.calls.Load(); n != 0 {
		t.Errorf("registration ran %d syntheses, want none", n)
	}
	if again := docStatus(t, serve(mux, http.MethodGet, "/doc/"+got.Doc.ID)); again.ID != got.Doc.ID {
		t.Errorf("GET /doc/%s = %+v, want the registered document", got.Doc.ID, again)
	}

	f.open()
	rec := serve(mux, http.MethodGet, "/audio/"+first.Parts[0])
	if rec.Code != http.StatusOK || !bytes.HasPrefix(rec.Body.Bytes(), []byte("RIFF")) {
		t.Errorf("GET /audio = %d %s, want the part synthesized on demand", rec.Code,
			rec.Header().Get("Content-Type"))
	}
}

// TestReadJSONMatchesMarkdownKeys pins the contract the JSON form exists
// for: the same block texts get the same keys, and so the same document,
// whether they arrive as markdown or pre-split.
func TestReadJSONMatchesMarkdownKeys(t *testing.T) {
	f := newFakeSpeaker()
	mux := newDocMux(t, f)
	long := strings.Repeat("This sentence is here to make the paragraph long enough to split. ", 20)
	up := upload(t, mux, f, "## Overview\n\nFirst paragraph.\n\n- A list item\n- Another\n\n"+
		"## Details\n\n"+long+"\n")
	body, _ := json.Marshal(readRequest{Name: "notes.md", Sections: []requestSection{
		{Blocks: []string{"Overview", "First paragraph.", "A list item", "Another"}},
		{Blocks: []string{"Details", long}},
	}})
	got := registered(t, register(mux, string(body), "application/json"))

	if got.Doc.ID != up.Doc.ID {
		t.Errorf("registered doc %s, uploaded doc %s; want the same keys and so the same id",
			got.Doc.ID, up.Doc.ID)
	}
	partsAttrs := sectionParts.FindAllStringSubmatch(up.Content, -1)
	if len(partsAttrs) != len(got.Sections) {
		t.Fatalf(
			"%d sections listed in the HTML, %d registered",
			len(partsAttrs),
			len(got.Sections),
		)
	}
	for i, m := range partsAttrs {
		if want := strings.Fields(m[1]); !slices.Equal(got.Sections[i].Parts, want) {
			t.Errorf("section %d parts = %v, HTML lists %v", i+1, got.Sections[i].Parts, want)
		}
	}
	var blocks [][]string
	for _, sec := range got.Sections {
		blocks = append(blocks, sec.Blocks...)
	}
	tags := chunkKeys.FindAllStringSubmatch(up.Content, -1)
	if len(tags) != len(blocks) {
		t.Fatalf("%d tagged blocks in the HTML, %d registered", len(tags), len(blocks))
	}
	for j, m := range tags {
		if want := strings.Fields(m[1]); !slices.Equal(blocks[j], want) {
			t.Errorf("block %d keys = %v, HTML tags %v", j, blocks[j], want)
		}
	}
}

// TestReadJSONKeepsEmptyPlaces pins the alignment a page indexes by: an
// empty section and a block with nothing speakable answer [] (never null) in
// their place, and a missing name gets a label.
func TestReadJSONKeepsEmptyPlaces(t *testing.T) {
	mux := newDocMux(t, newFakeSpeaker())
	rec := register(mux, `{"sections":[{"blocks":[]},{"blocks":["\"\"","Spoken."]}]}`,
		"application/json")
	got := registered(t, rec)
	if got.Name != untitledName || got.Doc.Name != untitledName {
		t.Errorf("name = %q / %q, want %q", got.Name, got.Doc.Name, untitledName)
	}
	if len(got.Sections) != 2 || len(got.Sections[0].Parts) != 0 ||
		len(got.Sections[0].Blocks) != 0 {
		t.Errorf("sections = %+v, want an empty first section", got.Sections)
	}
	second := got.Sections[1]
	if len(second.Parts) != 1 || len(second.Blocks) != 2 || len(second.Blocks[0]) != 0 ||
		!slices.Equal(second.Blocks[1], second.Parts) {
		t.Errorf("section 2 = %+v, want the quotes-only block empty and the spoken one keyed",
			second)
	}
	raw := rec.Body.String()
	for _, want := range []string{`"parts":[],"blocks":[]`, `"blocks":[[],["`} {
		if !strings.Contains(raw, want) {
			t.Errorf("response %s lacks %s: empty places must encode as [], not null", raw, want)
		}
	}
}

// TestReadJSONRejectsBadBodies pins the 400s of the JSON form, each with the
// document routes' error shape, and that anything not declared JSON still
// takes the multipart path.
func TestReadJSONRejectsBadBodies(t *testing.T) {
	f := newFakeSpeaker()
	mux := newDocMux(t, f)
	cases := []struct {
		name, body, wantReason string
	}{
		{"malformed", `not json`, "decode JSON body"},
		{"no sections", `{"name":"x"}`, "at least one section"},
		{"empty sections", `{"sections":[]}`, "at least one section"},
		{
			"oversize", `{"sections":[{"blocks":["` + strings.Repeat("a", maxUploadBytes) + `"]}]}`,
			"body over 5 MB",
		},
	}
	for _, tc := range cases {
		rec := register(mux, tc.body, "application/json")
		var body errorBody
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		if rec.Code != http.StatusBadRequest ||
			!strings.Contains(body.Error.Message, tc.wantReason) {
			t.Errorf("%s: POST /read = %d %.120s, want 400 naming %q", tc.name, rec.Code,
				rec.Body.String(), tc.wantReason)
		}
	}
	rec := register(mux, `{"sections":[{"blocks":["Hi."]}]}`, "text/plain")
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "'doc' field") {
		t.Errorf("text/plain POST /read = %d %s, want the multipart path's plain-text 400",
			rec.Code, rec.Body.String())
	}
	if n := f.calls.Load(); n != 0 {
		t.Errorf("rejected requests ran %d syntheses, want none", n)
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
	f.failWith(&tts.Error{Kind: tts.KindQuota, Provider: "fake", Message: "rate limited"})
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
	// A rate limit is not retried: one attempt, and the reason says so.
	want := "failed after 1 attempt: rate limited"
	if st.Total.Failed != 1 || st.Reason != want || st.Sections[0].Reason != want {
		t.Errorf("status = %+v, want the part failed with reason %q", st, want)
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

// TestAddEvictsIdleDocumentsFirst pins the eviction order: a full registry
// makes room by dropping the least recently used document with no work in
// flight, so a registration that queues nothing (a page view) never costs an
// upload its queued parts; only when every other document has work in flight
// does the least recently used of them go.
func TestAddEvictsIdleDocumentsFirst(t *testing.T) {
	inFlight := make(map[string]bool)
	var forgotten []string
	r := docRegistry{
		forget: func(keys []string) { forgotten = append(forgotten, keys...) },
		pending: func(keys []string) bool {
			return slices.ContainsFunc(keys, func(k string) bool { return inFlight[k] })
		},
	}
	add := func(key string, busy bool) {
		inFlight[key] = busy
		r.add(newDocument(key, [][]Part{{{Key: key, Text: key}}}))
	}
	kept := func() []string {
		r.mu.Lock()
		defer r.mu.Unlock()
		names := make([]string, len(r.docs))
		for i, d := range r.docs {
			names[i] = d.name
		}
		return names
	}

	add("queued", true) // the oldest, with work in flight
	add("idle", false)
	for i := range maxDocs - 2 {
		add(fmt.Sprintf("busy%d", i), true)
	}
	add("view", false) // one past the cap, holding no work
	if names := kept(); slices.Contains(names, "idle") || !slices.Contains(names, "queued") ||
		!slices.Contains(names, "view") {
		t.Errorf("kept %v, want the idle document gone, the queued one and the new one kept",
			names)
	}
	if !slices.Equal(forgotten, []string{"idle"}) {
		t.Errorf("forgotten = %v, want the idle document's part", forgotten)
	}

	inFlight["view"] = true // now every kept document has work in flight
	add("view2", false)
	if names := kept(); slices.Contains(names, "queued") || !slices.Contains(names, "view2") {
		t.Errorf("kept %v, want plain LRU once every document is busy: the oldest gone", names)
	}
	if !slices.Equal(forgotten, []string{"idle", "queued"}) {
		t.Errorf("forgotten = %v, want the idle document's part, then the oldest's", forgotten)
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

// waitStatus polls document id until done accepts its status.
func waitStatus(t *testing.T, mux *http.ServeMux, id string, done func(DocStatus) bool) DocStatus {
	t.Helper()
	deadline := time.Now().Add(settleTimeout)
	for {
		st := docStatus(t, serve(mux, http.MethodGet, "/doc/"+id))
		if done(st) {
			return st
		}
		if time.Now().After(deadline) {
			t.Fatalf("document %s never reached the awaited status: %+v", id, st)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// TestStatusReportsRetries pins how a part the provider keeps failing reads
// in the status: retrying while another attempt is scheduled, then failed
// with the attempt count in the reason.
func TestStatusReportsRetries(t *testing.T) {
	stall := &tts.Error{
		Kind: tts.KindUpstream, Provider: "fake", Message: "fake did not answer within 1m30s",
	}
	t.Run("retrying", func(t *testing.T) {
		f := newFakeSpeaker()
		f.failWith(stall)
		cfg := Config{
			Speaker: f, Health: tts.NewHealth("fake", "model"), CacheDir: t.TempDir(),
			retryDelay: func(int) time.Duration { return time.Hour },
		}
		s := newDocServer(cfg, audiocache.NewStore(cfg.CacheDir))
		mux := http.NewServeMux()
		s.routes(mux)
		f.open()
		got := upload(t, mux, f, "Hello there.\n")
		t.Cleanup(func() { s.prep.Forget([]string{firstKey(t, got.Content)}) })

		st := waitStatus(t, mux, got.Doc.ID, func(st DocStatus) bool {
			return st.Total.Retrying == 1
		})
		want := "retrying after 1 attempt: fake did not answer within 1m30s"
		if st.Sections[0].Retrying != 1 || st.Total.Failed != 0 ||
			st.Sections[0].Reason != want || st.Reason != want {
			t.Errorf("status = %+v, want one part retrying with reason %q", st, want)
		}
		raw := serve(mux, http.MethodGet, "/doc/"+got.Doc.ID).Body.String()
		if !strings.Contains(raw, `"retrying":1`) {
			t.Errorf("status JSON %s has no retrying count", raw)
		}
	})
	t.Run("out of attempts", func(t *testing.T) {
		f := newFakeSpeaker()
		f.failWith(stall)
		cfg := Config{
			Speaker: f, Health: tts.NewHealth("fake", "model"), CacheDir: t.TempDir(),
			retryDelay: func(int) time.Duration { return 0 },
		}
		mux := NewMux(cfg)
		f.open()
		got := upload(t, mux, f, "Hello there.\n")
		st := waitStatus(t, mux, got.Doc.ID, func(st DocStatus) bool {
			return st.Total.Failed == 1
		})
		want := "failed after 3 attempts: fake did not answer within 1m30s"
		if st.Reason != want || st.Sections[0].Reason != want || f.calls.Load() != 3 {
			t.Errorf("status = %+v after %d syntheses, want reason %q after 3", st,
				f.calls.Load(), want)
		}
	})
}

// TestPrepareOneSection pins ?section=N: only that section's parts are
// queued again.
func TestPrepareOneSection(t *testing.T) {
	f := newFakeSpeaker()
	f.failWith(&tts.Error{Kind: tts.KindQuota, Provider: "fake", Message: "rate limited"})
	mux := newDocMux(t, f)
	f.open()
	got := upload(t, mux, f, "## One\n\nAlpha.\n\n## Two\n\nBeta.\n")
	if st := settle(t, mux, got.Doc.ID); st.Total.Ready != 0 {
		t.Fatalf("after the quota failure: %+v, want nothing ready", st.Total)
	}

	f.failWith(nil)
	rec := serve(mux, http.MethodPost, "/doc/"+got.Doc.ID+"/prepare?section=2")
	if rec.Code != http.StatusOK {
		t.Fatalf("prepare?section=2 = %d %s", rec.Code, rec.Body.String())
	}
	st := settle(t, mux, got.Doc.ID)
	if st.Sections[0].Ready != 0 || st.Sections[1].Ready != 1 {
		t.Errorf("sections = %+v, want only section 2 prepared", st.Sections)
	}
	for _, bad := range []string{"0", "3", "two"} {
		target := "/doc/" + got.Doc.ID + "/prepare?section=" + bad
		if rec := serve(mux, http.MethodPost, target); rec.Code != http.StatusBadRequest {
			t.Errorf("prepare?section=%s = %d, want 400", bad, rec.Code)
		}
	}
}

// TestPrepareFailedOnly pins ?failed=1: retrying what failed leaves the idle
// parts idle, so it spends nothing past what already failed.
func TestPrepareFailedOnly(t *testing.T) {
	f := newFakeSpeaker()
	f.failWith(&tts.Error{Kind: tts.KindQuota, Provider: "fake", Message: "rate limited"})
	mux := newDocMux(t, f)
	f.open()
	var md strings.Builder
	for i := range prepareWorkers + 2 { // more parts than workers: a halt leaves some idle
		fmt.Fprintf(&md, "Part %d. %s\n\n", i, strings.Repeat("Some words to read. ", 20))
	}
	got := upload(t, mux, f, md.String())
	before := settle(t, mux, got.Doc.ID).Total
	if before.Failed == 0 || before.Idle == 0 {
		t.Fatalf("after the quota failure: %+v, want some parts failed and some idle", before)
	}

	f.failWith(nil)
	rec := serve(mux, http.MethodPost, "/doc/"+got.Doc.ID+"/prepare?section=1&failed=1")
	if rec.Code != http.StatusOK {
		t.Fatalf("prepare?failed=1 = %d %s", rec.Code, rec.Body.String())
	}
	after := settle(t, mux, got.Doc.ID).Total
	if after.Ready != before.Failed || after.Idle != before.Idle || after.Failed != 0 {
		t.Errorf("after prepare?failed=1: %+v, want the %d failed parts ready and %d still idle",
			after, before.Failed, before.Idle)
	}
	target := "/doc/" + got.Doc.ID + "/prepare?failed=maybe"
	if rec := serve(mux, http.MethodPost, target); rec.Code != http.StatusBadRequest {
		t.Errorf("prepare?failed=maybe = %d, want 400", rec.Code)
	}
}
