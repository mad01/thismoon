package web

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/mad01/thismoon/services/speak/internal/audiocache"
	"github.com/mad01/thismoon/services/speak/internal/tts"
)

// maxDocs is how many uploaded documents serve keeps for their audio routes.
// A page whose document fell out re-posts its markdown.
const maxDocs = 32

// autoPrepareChars caps what an upload queues for synthesis by itself:
// about 30 minutes of speech at ~14 characters a second, so a long document
// does not spend hours of remote synthesis nobody asked for. Parts past it
// stay idle until prepared.
const autoPrepareChars = 25_000

// prepareWorkers is how many syntheses run at once. Remote providers handled
// three parallel requests without slowing down; more would risk their rate
// limits.
const prepareWorkers = 3

// prepareTimeout backstops one background synthesis. The provider clients
// give up first (a remote one makes two attempts of 90 seconds each); this
// only stops a synthesis that somehow never returns from holding a worker
// forever.
const prepareTimeout = 4 * time.Minute

// audioMaxAge is how long a browser may reuse a part's audio. A key names
// its content, so the clip behind a URL never changes.
const audioMaxAge = 24 * time.Hour

// docIDHexLen is the length of a document ID: 64 bits of a sha256.
const docIDHexLen = 16

// failureEventInterval spaces the events.this events background preparation
// emits. A failing provider fails every queued part, and upstream failures
// do not stop the queue, so one event a minute tells the story without
// flooding the log. Every failure is still recorded in health and shown on
// the page.
const failureEventInterval = time.Minute

// document is an uploaded markdown file as serve keeps it: the name it was
// uploaded under and each section's parts in reading order.
type document struct {
	id       string
	name     string
	sections [][]Part
}

func newDocument(name string, sections [][]Part) *document {
	h := sha256.New()
	for _, parts := range sections {
		for _, p := range parts {
			_, _ = io.WriteString(h, p.Key+" ")
		}
		_, _ = io.WriteString(h, "\n") // section boundaries count too
	}
	return &document{
		id:       hex.EncodeToString(h.Sum(nil))[:docIDHexLen],
		name:     name,
		sections: sections,
	}
}

// PartCounts tallies a set of parts by audio state.
type PartCounts struct {
	Parts      int `json:"parts"`
	Ready      int `json:"ready"`
	Generating int `json:"generating"`
	Queued     int `json:"queued"`
	Retrying   int `json:"retrying"` // failed, another attempt scheduled
	Idle       int `json:"idle"`
	Failed     int `json:"failed"` // out of attempts, or stopped by a shared failure
}

// SectionStatus is one section's audio state; Section is 1-based, matching
// the rendered data-section.
type SectionStatus struct {
	Section int `json:"section"`
	PartCounts
	// Reason is the section's last failure with its attempt count ("failed
	// after 3 attempts: ..."), or, when nothing failed for good, the last
	// failure being retried ("retrying after 1 attempt: ..."); else empty.
	Reason string `json:"reason"`
}

// DocStatus is a document's audio state: what GET /doc/{id}, POST
// /doc/{id}/prepare and POST /read answer.
type DocStatus struct {
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	Total    PartCounts      `json:"total"`
	Sections []SectionStatus `json:"sections"`
	Reason   string          `json:"reason"` // as SectionStatus.Reason, over the document
}

func (c *PartCounts) count(state audiocache.State) {
	c.Parts++
	switch state {
	case audiocache.StateReady:
		c.Ready++
	case audiocache.StateGenerating:
		c.Generating++
	case audiocache.StateQueued:
		c.Queued++
	case audiocache.StateRetrying:
		c.Retrying++
	case audiocache.StateFailed:
		c.Failed++
	default:
		c.Idle++
	}
}

func (c *PartCounts) add(o PartCounts) {
	c.Parts += o.Parts
	c.Ready += o.Ready
	c.Generating += o.Generating
	c.Queued += o.Queued
	c.Retrying += o.Retrying
	c.Idle += o.Idle
	c.Failed += o.Failed
}

// allParts is every part of d in reading order.
func (d *document) allParts() []Part {
	var all []Part
	for _, parts := range d.sections {
		all = append(all, parts...)
	}
	return all
}

// items is parts as preparer work, in order.
func items(parts []Part) []audiocache.Item {
	out := make([]audiocache.Item, len(parts))
	for i, p := range parts {
		out[i] = audiocache.Item{Key: p.Key, Text: p.Text}
	}
	return out
}

// docRegistry keeps the most recently used documents, up to maxDocs.
type docRegistry struct {
	// forget is handed the part keys of evicted documents that no kept
	// document holds. It runs under mu, so a re-upload of an evicted
	// document cannot queue its parts between eviction and forgetting.
	forget func(keys []string)

	mu   sync.Mutex
	docs []*document // least recently used first
}

// add keeps d as the most recent document, replacing one with its ID and
// evicting the least recently used past maxDocs.
func (r *docRegistry) add(d *document) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.docs = slices.DeleteFunc(r.docs, func(o *document) bool { return o.id == d.id })
	r.docs = append(r.docs, d)
	if len(r.docs) <= maxDocs {
		return
	}
	evicted := slices.Clone(r.docs[:len(r.docs)-maxDocs])
	r.docs = slices.Delete(r.docs, 0, len(evicted))
	if keys := orphanKeys(evicted, r.docs); len(keys) > 0 && r.forget != nil {
		r.forget(keys)
	}
}

// orphanKeys is the part keys of gone documents that no kept document holds.
func orphanKeys(gone, kept []*document) []string {
	held := make(map[string]bool)
	for _, d := range kept {
		for _, p := range d.allParts() {
			held[p.Key] = true
		}
	}
	var keys []string
	for _, d := range gone {
		for _, p := range d.allParts() {
			if !held[p.Key] {
				held[p.Key] = true // once is enough
				keys = append(keys, p.Key)
			}
		}
	}
	return keys
}

// whileKept runs fn under the registry lock if a document with d's ID (and
// so d's parts) is still kept, and reports whether it ran. Queueing a document's parts goes through it: an eviction
// forgets the parts under the same lock, so it cannot slip in between a
// lookup and the queueing and leave orphaned parts queued.
func (r *docRegistry) whileKept(d *document, fn func()) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !slices.ContainsFunc(r.docs, func(o *document) bool { return o.id == d.id }) {
		return false
	}
	fn()
	return true
}

// get returns the document with id and marks it used.
func (r *docRegistry) get(id string) (*document, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	i := slices.IndexFunc(r.docs, func(d *document) bool { return d.id == id })
	if i < 0 {
		return nil, false
	}
	d := r.docs[i]
	r.docs = append(slices.Delete(r.docs, i, i+1), d)
	return d, true
}

// text returns the text of the part with key in any kept document.
func (r *docRegistry) text(key string) (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, d := range slices.Backward(r.docs) {
		for _, parts := range d.sections {
			if i := slices.IndexFunc(parts, func(p Part) bool { return p.Key == key }); i >= 0 {
				return parts[i].Text, true
			}
		}
	}
	return "", false
}

// docServer serves uploaded documents and their pre-synthesized audio.
type docServer struct {
	speaker Speaker
	health  *tts.Health
	store   *audiocache.Store
	prep    *audiocache.Preparer
	docs    docRegistry
}

func newDocServer(cfg Config, store *audiocache.Store) *docServer {
	events := &eventLimiter{interval: failureEventInterval, now: time.Now}
	synth := func(ctx context.Context, text string) (tts.Audio, error) {
		// Default voice, provider speed: the browser applies its speed
		// control to the clip, so one clip serves every speed.
		audio, err := cfg.Speaker.Synthesize(ctx, tts.Request{Text: text})
		if err != nil && events.allow() {
			emitFailure(cfg.Health, err)
		}
		return audio, err
	}
	s := &docServer{
		speaker: cfg.Speaker,
		health:  cfg.Health,
		store:   store,
		prep: audiocache.NewPreparer(audiocache.PreparerConfig{
			Store:      store,
			Synth:      synth,
			Health:     cfg.Health,
			Workers:    prepareWorkers,
			Timeout:    prepareTimeout,
			RetryDelay: cfg.retryDelay, // nil: the preparer's own backoff
		}),
	}
	s.docs.forget = s.prep.Forget
	return s
}

// eventLimiter lets one event through per interval.
type eventLimiter struct {
	interval time.Duration
	now      func() time.Time

	mu   sync.Mutex
	last time.Time // when the last event went out
}

// allow reports whether an event may go out now, counting it if so.
func (l *eventLimiter) allow() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if !l.last.IsZero() && now.Sub(l.last) < l.interval {
		return false
	}
	l.last = now
	return true
}

// routes registers the document and audio endpoints on mux.
func (s *docServer) routes(mux *http.ServeMux) {
	mux.HandleFunc("POST /read", s.read)
	mux.HandleFunc("GET /doc/{id}", s.status)
	mux.HandleFunc("POST /doc/{id}/prepare", s.prepare)
	mux.HandleFunc("GET /doc/{id}/audio", s.download)
	mux.HandleFunc("GET /audio/{key}", s.audio)
}

// readResponse is the JSON shape POST /read returns: the uploaded file name,
// the rendered HTML body (goldmark sections) and the document's audio state.
// app.js mounts Content as-is and shows Name as the doc label.
type readResponse struct {
	Name    string    `json:"name"`
	Content string    `json:"content"`
	Doc     DocStatus `json:"doc"`
}

// read renders an uploaded markdown file, keeps it for the audio routes and
// starts synthesizing its opening parts.
func (s *docServer) read(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
	file, header, err := r.FormFile("doc")
	if err != nil {
		http.Error(w, "upload a markdown file in the 'doc' field: "+err.Error(),
			http.StatusBadRequest)
		return
	}
	defer func() { _ = file.Close() }()
	source, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "read upload: "+err.Error(), http.StatusBadRequest)
		return
	}
	clipID := s.speaker.ClipID("")
	plan, err := PlanSections(source, func(text string) string {
		return audiocache.Key(clipID, 0, text)
	})
	if err != nil {
		http.Error(w, "render markdown: "+err.Error(), http.StatusUnprocessableEntity)
		return
	}
	d := newDocument(header.Filename, plan.Sections)
	s.docs.add(d)
	s.docs.whileKept(d, func() { s.queueOpening(d) })
	writeJSON(w, http.StatusOK, readResponse{
		Name:    header.Filename,
		Content: plan.HTML,
		Doc:     s.docStatus(d),
	})
}

// queueOpening queues d's parts in reading order up to autoPrepareChars,
// ahead of older documents' parts. Parts already stored count toward the
// cap: it measures how far into the document the listener can go without
// waiting.
func (s *docServer) queueOpening(d *document) {
	parts := d.allParts()
	chars := 0
	for i, p := range parts {
		chars += utf8.RuneCountInString(p.Text)
		if chars > autoPrepareChars {
			parts = parts[:i]
			break
		}
	}
	s.prep.Queue(items(parts))
}

func (s *docServer) status(w http.ResponseWriter, r *http.Request) {
	if d, ok := s.lookup(w, r); ok {
		writeJSON(w, http.StatusOK, s.docStatus(d))
	}
}

// prepare queues every part of a document, or of the section named by
// ?section=N, that is not stored yet, ahead of other documents' parts. With
// ?failed=1 it queues only the failed ones: retrying what failed without
// also starting the idle parts past the upload cap. Failed parts start over
// with a fresh attempt count.
func (s *docServer) prepare(w http.ResponseWriter, r *http.Request) {
	d, ok := s.lookup(w, r)
	if !ok {
		return
	}
	query := r.URL.Query()
	parts, _, err := d.partsFor(query.Get("section"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	failedOnly, err := optionalBool(query.Get("failed"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed must be 1 or 0: "+err.Error())
		return
	}
	s.docs.whileKept(d, func() {
		if failedOnly {
			parts = s.failedParts(parts)
		}
		s.prep.Queue(items(parts))
	})
	writeJSON(w, http.StatusOK, s.docStatus(d))
}

// failedParts is the parts whose last synthesis failed for good.
func (s *docServer) failedParts(parts []Part) []Part {
	return slices.DeleteFunc(slices.Clone(parts), func(p Part) bool {
		state, _ := s.prep.State(p.Key)
		return state != audiocache.StateFailed
	})
}

// optionalBool parses a query flag; absent is false.
func optionalBool(v string) (bool, error) {
	if v == "" {
		return false, nil
	}
	return strconv.ParseBool(v)
}

// lookup finds the document named in the path, answering 404 when serve
// does not keep it.
func (s *docServer) lookup(w http.ResponseWriter, r *http.Request) (*document, bool) {
	d, ok := s.docs.get(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound,
			"unknown document; upload it again (serve keeps the "+
				strconv.Itoa(maxDocs)+" most recent)")
	}
	return d, ok
}

func (s *docServer) docStatus(d *document) DocStatus {
	status := DocStatus{ID: d.id, Name: d.name, Sections: make([]SectionStatus, len(d.sections))}
	for i, parts := range d.sections {
		sec := SectionStatus{Section: i + 1}
		for _, p := range parts {
			state, err := s.prep.State(p.Key)
			sec.count(state)
			// A failure for good outranks one being retried.
			if err != nil && (state == audiocache.StateFailed || sec.Failed == 0) {
				sec.Reason = err.Error()
			}
		}
		status.Total.add(sec.PartCounts)
		if sec.Reason != "" && (sec.Failed > 0 || status.Total.Failed == 0) {
			status.Reason = sec.Reason
		}
		status.Sections[i] = sec
	}
	return status
}

// download answers a document, or one section of it, as a single clip once
// every part is stored.
func (s *docServer) download(w http.ResponseWriter, r *http.Request) {
	d, ok := s.lookup(w, r)
	if !ok {
		return
	}
	parts, section, err := d.partsFor(r.URL.Query().Get("section"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(parts) == 0 {
		writeError(w, http.StatusNotFound, "nothing there is read aloud")
		return
	}
	if ready := s.readyCount(parts); ready < len(parts) {
		writeError(w, http.StatusConflict, fmt.Sprintf(
			"%d of %d parts are ready; prepare the rest first", ready, len(parts),
		))
		return
	}
	joined, err := s.joinStored(parts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment",
		map[string]string{"filename": downloadName(d.name, section) + joined.Ext()}))
	w.Header().Set("Cache-Control", "no-store")
	writeAudio(w, joined)
}

// partsFor returns the parts of the section named by query ("" for the
// whole document) and the section number, 0 for the whole document.
func (d *document) partsFor(query string) ([]Part, int, error) {
	if query == "" {
		return d.allParts(), 0, nil
	}
	n, err := strconv.Atoi(query)
	if err != nil || n < 1 || n > len(d.sections) {
		return nil, 0, fmt.Errorf("section must be a number from 1 to %d", len(d.sections))
	}
	return d.sections[n-1], n, nil
}

// readyCount is how many of parts have a stored clip.
func (s *docServer) readyCount(parts []Part) int {
	ready := 0
	for _, p := range parts {
		if s.store.Has(p.Key) {
			ready++
		}
	}
	return ready
}

// joinStored joins the stored clips of parts into one.
func (s *docServer) joinStored(parts []Part) (tts.Audio, error) {
	clips := make([]tts.Audio, 0, len(parts))
	for _, p := range parts {
		audio, ok, err := s.store.Get(p.Key)
		if err != nil {
			return tts.Audio{}, err
		}
		if !ok {
			return tts.Audio{}, fmt.Errorf("speak: part %s left the cache while joined", p.Key)
		}
		clips = append(clips, audio)
	}
	return audiocache.Join(clips)
}

// downloadName is a file name for a document's audio: the uploaded name
// without its extension, cut to characters safe in any file system, plus
// the section when there is one.
func downloadName(docName string, section int) string {
	base := strings.TrimSuffix(docName, filepath.Ext(docName))
	base = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '.' {
			return r
		}
		return '-'
	}, base)
	base = strings.Trim(base, "-.")
	if base == "" {
		base = "speak"
	}
	if section > 0 {
		base += "-section-" + strconv.Itoa(section)
	}
	return base
}

// audio answers one part's clip, synthesizing it ahead of the background
// queue when it is not stored yet. A stored clip is served even when no kept
// document holds its key, so an open page keeps playing across a restart.
func (s *docServer) audio(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if !audiocache.ValidKey(key) {
		writeError(w, http.StatusBadRequest, "an audio key is 32 lowercase hex characters")
		return
	}
	var audio tts.Audio
	var err error
	if text, ok := s.docs.text(key); ok {
		audio, err = s.prep.Fetch(r.Context(), key, text)
	} else if audio, ok, err = s.store.Get(key); err == nil && !ok {
		writeError(w, http.StatusNotFound, "no loaded document reads this part; upload it again")
		return
	}
	switch {
	case err == nil:
		w.Header().Set("Cache-Control",
			"private, max-age="+strconv.Itoa(int(audioMaxAge.Seconds())))
		writeAudio(w, audio)
	case r.Context().Err() != nil:
		// The listener went away; the synthesis carries on into the cache.
	case isFileError(err):
		writeError(w, http.StatusInternalServerError, err.Error())
	default:
		// The preparer recorded the failure; answer like the speech endpoint.
		writeFailure(w, s.health, err)
	}
}

// isFileError reports an error from the cache's own files rather than from
// synthesis.
func isFileError(err error) bool {
	_, ok := errors.AsType[*fs.PathError](err)
	return ok
}

// writeJSON answers v as JSON with status.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("speak: encode response: %v", err)
	}
}

// errorBody is the OpenAI-style error shape the document routes answer with.
type errorBody struct {
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

func writeError(w http.ResponseWriter, status int, message string) {
	var body errorBody
	body.Error.Message = message
	writeJSON(w, status, body)
}
