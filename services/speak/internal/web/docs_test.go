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

// register posts body to /read under contentType.
func register(mux http.Handler, body, contentType string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/read", strings.NewReader(body))
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

// registered decodes a registration's answer, failing on any other.
func registered(t *testing.T, rec *httptest.ResponseRecorder) registerResponse {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /read = %d %s", rec.Code, rec.Body.String())
	}
	var got registerResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode /read: %v (body %s)", err, rec.Body.String())
	}
	return got
}

// registerDoc registers name with one list of block texts per section, the
// way a present page does on load. Its cleanup lets any synthesis the test
// started finish before the cache directory is removed.
func registerDoc(
	t *testing.T, mux *http.ServeMux, f *fakeSpeaker, name string, sections ...[]string,
) registerResponse {
	t.Helper()
	req := readRequest{Name: name, Sections: make([]requestSection, len(sections))}
	for i, blocks := range sections {
		req.Sections[i] = requestSection{Blocks: blocks}
	}
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	got := registered(t, register(mux, string(body), "application/json"))
	t.Cleanup(func() {
		f.open()
		settle(t, mux, got.Doc.ID)
	})
	return got
}

// prepareAll queues every part of document id, as the page's Prepare all does.
func prepareAll(t *testing.T, mux *http.ServeMux, id string) DocStatus {
	t.Helper()
	rec := serve(mux, http.MethodPost, "/doc/"+id+"/prepare")
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /doc/%s/prepare = %d %s", id, rec.Code, rec.Body.String())
	}
	return docStatus(t, rec)
}

// firstKey is the first part key a registration answered.
func firstKey(t *testing.T, got registerResponse) string {
	t.Helper()
	keys := allKeys(got)
	if len(keys) == 0 {
		t.Fatalf("registration %+v has no parts", got)
	}
	return keys[0]
}

// allKeys is every part key a registration answered, in play order.
func allKeys(got registerResponse) []string {
	var keys []string
	for _, sec := range got.Sections {
		keys = append(keys, sec.Parts...)
	}
	return keys
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

// TestReadRegistersWithoutSynthesis pins POST /read: the keys come back
// aligned with the request, the document is kept, nothing is queued, and a
// part plays on demand all the same.
func TestReadRegistersWithoutSynthesis(t *testing.T) {
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
	if len(got.Doc.ID) != docIDHexLen || len(got.Doc.Sections) != 2 ||
		got.Doc.Sections[1].Section != 2 {
		t.Errorf("doc = %+v, want a 16-hex id and both sections, 1-based", got.Doc)
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
	if rec := serve(mux, http.MethodGet, "/doc/0123456789abcdef"); rec.Code != http.StatusNotFound {
		t.Errorf("unknown doc = %d, want 404", rec.Code)
	}

	f.open()
	rec := serve(mux, http.MethodGet, "/audio/"+first.Parts[0])
	if rec.Code != http.StatusOK || !bytes.HasPrefix(rec.Body.Bytes(), []byte("RIFF")) {
		t.Errorf("GET /audio = %d %s, want the part synthesized on demand", rec.Code,
			rec.Header().Get("Content-Type"))
	}
}

// TestReadIsStableAcrossViews pins what prepared mode relies on: the same
// texts register to the same keys and the same document id every time.
func TestReadIsStableAcrossViews(t *testing.T) {
	f := newFakeSpeaker()
	mux := newDocMux(t, f)
	long := strings.Repeat("This sentence is here to make the paragraph long enough to split. ", 20)
	first := registerDoc(t, mux, f, "notes",
		[]string{"Overview", "First paragraph.", "A list item", "Another"},
		[]string{"Details", long})
	again := registerDoc(t, mux, f, "notes",
		[]string{"Overview", "First paragraph.", "A list item", "Another"},
		[]string{"Details", long})
	if first.Doc.ID != again.Doc.ID || !slices.Equal(allKeys(first), allKeys(again)) {
		t.Errorf("registered twice: %s %v, then %s %v; want the same document and keys",
			first.Doc.ID, allKeys(first), again.Doc.ID, allKeys(again))
	}
	if len(first.Sections[1].Parts) < 3 {
		t.Errorf("section 2 parts = %v, want the long block spread over several",
			first.Sections[1].Parts)
	}
}

// TestReadKeepsEmptyPlaces pins the alignment a page indexes by: an empty
// section and a block with nothing speakable answer [] (never null) in their
// place, and a missing name gets a label.
func TestReadKeepsEmptyPlaces(t *testing.T) {
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

// TestReadRejectsBadBodies pins the 400s of POST /read, each with the
// document routes' error shape.
func TestReadRejectsBadBodies(t *testing.T) {
	f := newFakeSpeaker()
	mux := newDocMux(t, f)
	cases := []struct {
		name, body, wantReason string
	}{
		{"malformed", `not json`, "decode JSON body"},
		{"no sections", `{"name":"x"}`, "at least one section"},
		{"empty sections", `{"sections":[]}`, "at least one section"},
		{
			"oversize", `{"sections":[{"blocks":["` + strings.Repeat("a", maxReadBytes) + `"]}]}`,
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
	if n := f.calls.Load(); n != 0 {
		t.Errorf("rejected requests ran %d syntheses, want none", n)
	}
}

// TestReadRefusesOtherContentTypes pins that POST /read is JSON only: the
// retired markdown upload (multipart field doc) and any other body get 415
// in the routes' JSON error shape, naming the form that works, and nothing
// is synthesized.
func TestReadRefusesOtherContentTypes(t *testing.T) {
	f := newFakeSpeaker()
	mux := newDocMux(t, f)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("doc", "notes.md")
	_, _ = fw.Write([]byte("## Hello\n\nworld paragraph\n"))
	_ = mw.Close()
	cases := []struct {
		name, body, contentType string
	}{
		{"multipart upload", buf.String(), mw.FormDataContentType()},
		{"text", `{"sections":[{"blocks":["Hi."]}]}`, "text/plain"},
		{"no content type", `{"sections":[{"blocks":["Hi."]}]}`, ""},
	}
	for _, tc := range cases {
		rec := register(mux, tc.body, tc.contentType)
		var body errorBody
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Errorf("%s: body %q is not the JSON error shape: %v", tc.name, rec.Body.String(), err)
			continue
		}
		if rec.Code != http.StatusUnsupportedMediaType ||
			!strings.Contains(body.Error.Message, "application/json") {
			t.Errorf("%s: POST /read = %d %s, want 415 naming application/json", tc.name,
				rec.Code, rec.Body.String())
		}
		if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
			t.Errorf("%s: Content-Type = %q, want application/json", tc.name, ct)
		}
	}
	if n := f.calls.Load(); n != 0 {
		t.Errorf("refused requests ran %d syntheses, want none", n)
	}
}

// TestAudioWaitsForPart pins the play path: a part asked for before it is
// ready is waited for, and the wait shares the background synthesis.
func TestAudioWaitsForPart(t *testing.T) {
	f := newFakeSpeaker()
	mux := newDocMux(t, f)
	key := firstKey(t, registerDoc(t, mux, f, "notes", []string{"Hello there, reader."}))

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
	got := registerDoc(t, mux, f, "notes", []string{"Hello there."})
	f.open()

	rec := serve(mux, http.MethodGet, "/audio/"+firstKey(t, got))
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
	got := registerDoc(t, mux, f, "notes.md", []string{"One", "Alpha."}, []string{"Two", "Beta."})
	id := got.Doc.ID

	rec := serve(mux, http.MethodGet, "/doc/"+id+"/audio")
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "0 of 2 parts") {
		t.Fatalf("download before ready = %d %s, want 409 naming the ready count", rec.Code,
			rec.Body.String())
	}
	prepareAll(t, mux, id)
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

// longBlocks is n block texts long enough to be a part each: over the first
// part's 250 characters, and two together over 600.
func longBlocks(n int) []string {
	filler := strings.Repeat("Words to read aloud at length. ", 17) // 527 characters
	blocks := make([]string, n)
	for i := range blocks {
		blocks[i] = fmt.Sprintf("Paragraph %03d. %s", i, filler)
	}
	return blocks
}

// TestPrepareQueuesIdleParts pins Prepare all: a registration leaves every
// part idle, and one POST /doc/{id}/prepare puts all of them on their way.
func TestPrepareQueuesIdleParts(t *testing.T) {
	const parts = 60
	f := newFakeSpeaker()
	mux := newDocMux(t, f)
	doc := registerDoc(t, mux, f, "long", longBlocks(parts)).Doc
	if doc.Total.Parts != parts || doc.Total.Idle != parts {
		t.Fatalf("after registration: %+v, want all %d parts idle", doc.Total, parts)
	}

	if st := prepareAll(t, mux, doc.ID); st.Total.Idle != 0 ||
		st.Total.Queued+st.Total.Generating != parts {
		t.Errorf("after prepare: %+v, want every part on its way", st.Total)
	}
	f.open()
	if st := settle(t, mux, doc.ID); st.Total.Ready != parts {
		t.Errorf("after synthesis: %+v, want all %d ready", st.Total, parts)
	}
}

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

	// One part more than there are workers, all queued.
	first := registerDoc(t, mux, f, "first", longBlocks(prepareWorkers+1))
	prepareAll(t, mux, first.Doc.ID)
	keys := allKeys(first)
	for _, key := range keys[:prepareWorkers] {
		waitPart(t, s, key, audiocache.StateGenerating)
	}
	// Every later document has work in flight too, so eviction falls back to
	// the least recently used: the first.
	for i := range maxDocs {
		doc := registerDoc(
			t,
			mux,
			f,
			fmt.Sprintf("doc%d", i),
			[]string{fmt.Sprintf("Document %d.", i)},
		)
		prepareAll(t, mux, doc.Doc.ID)
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
// flight, so a registration that queues nothing (a page view) never costs a
// document being prepared its queued parts; only when every other document
// has work in flight does the least recently used of them go.
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
		got := registerDoc(t, mux, f, "notes", []string{"Hello there."})
		t.Cleanup(func() { s.prep.Forget([]string{firstKey(t, got)}) })
		prepareAll(t, mux, got.Doc.ID)

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
		got := registerDoc(t, mux, f, "notes", []string{"Hello there."})
		prepareAll(t, mux, got.Doc.ID)
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
	got := registerDoc(t, mux, f, "notes", []string{"One", "Alpha."}, []string{"Two", "Beta."})
	prepareAll(t, mux, got.Doc.ID)
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
	// More parts than workers: a halt leaves some idle.
	got := registerDoc(t, mux, f, "notes", longBlocks(prepareWorkers+2))
	prepareAll(t, mux, got.Doc.ID)
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
