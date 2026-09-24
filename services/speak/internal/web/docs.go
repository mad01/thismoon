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

	"github.com/mad01/thismoon/services/speak/internal/audiocache"
	"github.com/mad01/thismoon/services/speak/internal/tts"
)

// maxDocs is how many registered documents serve keeps for their audio
// routes. A page whose document fell out registers it again.
const maxDocs = 32

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

// untitledName labels a document registered under no name.
const untitledName = "untitled"

// failureEventInterval spaces the events.this events background preparation
// emits. A failing provider fails every queued part, and upstream failures
// do not stop the queue, so one event a minute tells the story without
// flooding the log. Every failure is still recorded in health and shown on
// the page that registered the document.
const failureEventInterval = time.Minute

// document is a registered page as serve keeps it: the name it was registered
// under and each section's parts in reading order.
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

// SectionStatus is one section's audio state; Section is 1-based, the index
// the routes take as ?section=N.
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
	// document holds. It runs under mu, so a re-registration of an evicted
	// document cannot queue its parts between eviction and forgetting.
	forget func(keys []string)
	// pending reports whether any of the keys has work in flight; nil
	// means none ever does. It decides which document an eviction takes.
	pending func(keys []string) bool

	mu   sync.Mutex
	docs []*document // least recently used first
}

// add keeps d as the most recent document, replacing one with its ID and
// evicting one past maxDocs (see evictOne).
func (r *docRegistry) add(d *document) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.docs = slices.DeleteFunc(r.docs, func(o *document) bool { return o.id == d.id })
	r.docs = append(r.docs, d)
	if len(r.docs) <= maxDocs {
		return
	}
	evicted := r.evictOne()
	if keys := orphanKeys([]*document{evicted}, r.docs); len(keys) > 0 && r.forget != nil {
		r.forget(keys)
	}
}

// evictOne removes and returns the document that makes room for the one
// just added (the last): the least recently used with no work in flight, so
// a registration that queues nothing (a page view) never costs a document
// being prepared its queued parts; when every other document has work in
// flight, the least recently used of them. The caller holds r.mu.
func (r *docRegistry) evictOne() *document {
	others := r.docs[:len(r.docs)-1]
	i := slices.IndexFunc(others, func(d *document) bool { return !r.busy(d) })
	if i < 0 {
		i = 0
	}
	d := r.docs[i]
	r.docs = slices.Delete(r.docs, i, i+1)
	return d
}

// busy reports whether any part of d is queued, generating or retrying.
func (r *docRegistry) busy(d *document) bool {
	return r.pending != nil && r.pending(keysOf(d.allParts()))
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

// docServer serves registered documents and their pre-synthesized audio.
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
	s.docs.pending = s.prep.Pending
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

// readRequest is the body of POST /read: a page's text already split into
// the sections it plays by and the blocks that show them, in reading order,
// from a page that renders itself (present). Name labels the document; empty
// falls back to untitledName.
type readRequest struct {
	Name     string           `json:"name"`
	Sections []requestSection `json:"sections"`
}

type requestSection struct {
	Blocks []string `json:"blocks"`
}

// registerResponse is what POST /read returns: the keys the page plays and
// highlights by, mirroring the request's sections and blocks one to one,
// plus the document's audio state.
type registerResponse struct {
	Name     string        `json:"name"`
	Doc      DocStatus     `json:"doc"`
	Sections []sectionKeys `json:"sections"`
}

// sectionKeys is one section's part keys in play order (what the page stamps
// on the section as data-ra-parts) and, per block, the keys of the parts that
// read it (data-ra-chunk); [] for a block with nothing speakable.
type sectionKeys struct {
	Parts  []string   `json:"parts"`
	Blocks [][]string `json:"blocks"`
}

// read plans and keeps a document posted as pre-split block text (JSON, at
// most maxReadBytes) and answers the keys, but queues no synthesis: a page
// registers on every view and a remote provider bills every part, so
// playback (GET /audio/{key}) and POST /doc/{id}/prepare start it instead.
// Errors answer the document routes' JSON shape; a body not declared JSON
// (the retired markdown upload, say) gets 415.
func (s *docServer) read(w http.ResponseWriter, r *http.Request) {
	if !isJSON(r.Header.Get("Content-Type")) {
		writeError(w, http.StatusUnsupportedMediaType,
			`POST /read takes application/json: {"name", "sections": [{"blocks": [text, ...]}]}`)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxReadBytes)
	var req readRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, decodeReason(err))
		return
	}
	if len(req.Sections) == 0 {
		writeError(w, http.StatusBadRequest, "sections must hold at least one section")
		return
	}
	name := req.Name
	if strings.TrimSpace(name) == "" {
		name = untitledName
	}
	key := s.keyFunc()
	sections := make([][]Part, len(req.Sections))
	keys := make([]sectionKeys, len(req.Sections))
	for i, sec := range req.Sections {
		plan := planBlocks(sec.Blocks, key)
		sections[i] = plan.parts
		keys[i] = sectionKeys{Parts: keysOf(plan.parts), Blocks: plan.blockKeys}
	}
	d := newDocument(name, sections)
	s.docs.add(d)
	writeJSON(w, http.StatusOK, registerResponse{Name: name, Doc: s.docStatus(d), Sections: keys})
}

// isJSON reports whether contentType declares a JSON body, with or without a
// charset parameter.
func isJSON(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	return err == nil && mediaType == "application/json"
}

// decodeReason words a failed JSON body decode for the caller: the body cap
// by name, else the decoder's own reason.
func decodeReason(err error) string {
	if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
		return fmt.Sprintf("body over %d MB", maxReadBytes>>20)
	}
	return "decode JSON body: " + err.Error()
}

// keyFunc names a part's audio from its text: the cache key for the active
// provider's default voice at its default speed, the way every part is
// synthesized (see newDocServer).
func (s *docServer) keyFunc() func(text string) string {
	clipID := s.speaker.ClipID("")
	return func(text string) string { return audiocache.Key(clipID, 0, text) }
}

func (s *docServer) status(w http.ResponseWriter, r *http.Request) {
	if d, ok := s.lookup(w, r); ok {
		writeJSON(w, http.StatusOK, s.docStatus(d))
	}
}

// prepare queues every part of a document, or of the section named by
// ?section=N, that is not stored yet, ahead of other documents' parts. With
// ?failed=1 it queues only the failed ones: retrying what failed without
// also starting the idle parts. Failed parts start over with a fresh attempt
// count.
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
			"unknown document; register it again (serve keeps the "+
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

// downloadName is a file name for a document's audio: the document's name
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
		writeError(w, http.StatusNotFound, "no loaded document reads this part; register it again")
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
