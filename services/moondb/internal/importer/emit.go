package importer

import (
	"bytes"
	"cmp"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/mad01/thismoon/services/moondb"
	"github.com/mad01/thismoon/services/moondb/internal/patch"
	"github.com/mad01/thismoon/services/moondb/internal/record"
	"github.com/mad01/thismoon/services/moondb/internal/spool"
	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// detachedHead is the branch name a transcript records for a detached HEAD.
const detachedHead = "HEAD"

// chunkSet collects the chunks one envelope points at, keyed as the spool
// stores them.
type chunkSet map[string][]byte

// add stores data and returns its key. Empty data is a real chunk.
func (c chunkSet) add(data []byte) string {
	key := spool.ChunkKey(data)
	c[key] = data
	return key
}

// step is one envelope to write, in the order the session lived it: a turn
// opening, an edit, a turn closing.
type step struct {
	at   time.Time
	rank int // breaks ties: open before edit before close
	seq  int
	turn *turn
	call *call
}

const (
	rankOpen = iota
	rankEdit
	rankClose
)

// emitter writes one parsed session to the spool.
type emitter struct {
	s       *session
	opts    Options
	places  *placer
	counts  *Counts
	sink    Sink
	main    *os.File
	touched map[string]time.Time // last edit time per file key
}

// emit writes the session's envelopes in time order. A nil sink builds
// every envelope and writes none, which is the dry run.
func (em *emitter) emit(edits []*call) error {
	f, err := os.Open(em.s.path)
	if err != nil {
		return fmt.Errorf("importer: open transcript: %w", err)
	}
	defer f.Close()
	em.main, em.touched = f, map[string]time.Time{}
	for _, st := range em.steps(edits) {
		env, chunks := em.build(st)
		em.counts.Envelopes++
		if em.sink == nil {
			continue
		}
		if err := em.sink.Write(env, chunks); err != nil {
			return fmt.Errorf("importer: session %s: %w", em.s.id, err)
		}
	}
	em.counts.Turns += len(em.s.turns)
	return nil
}

// steps orders the session's turns and edits by time.
func (em *emitter) steps(edits []*call) []step {
	var steps []step
	for _, t := range em.s.turns {
		steps = append(steps,
			step{at: t.at, rank: rankOpen, seq: len(steps), turn: t},
			step{at: t.endAt, rank: rankClose, seq: len(steps) + 1, turn: t})
	}
	for _, c := range edits {
		steps = append(steps, step{at: c.when(), rank: rankEdit, seq: len(steps), call: c})
	}
	slices.SortStableFunc(steps, func(a, b step) int {
		return cmp.Or(a.at.Compare(b.at), cmp.Compare(a.rank, b.rank), cmp.Compare(a.seq, b.seq))
	})
	return steps
}

func (em *emitter) build(st step) (spool.Envelope, chunkSet) {
	switch {
	case st.call != nil:
		return em.edit(st.call)
	case st.rank == rankOpen:
		return em.open(st.turn)
	default:
		return em.close(st.turn)
	}
}

// envelope starts an envelope with what every one of the session carries.
// Every envelope is a backfill, so it never overwrites what the store
// already holds.
func (em *emitter) envelope(
	kind spool.Kind,
	id string,
	at time.Time,
	turnID string,
) spool.Envelope {
	s := em.s
	return spool.Envelope{
		Version: spool.Version,
		Kind:    kind,
		ID:      id,
		At:      at,
		Harness: record.Harness,
		Session: spool.SessionRef{
			ID: s.id, Cwd: s.cwd, Title: s.title(), Model: s.model, TranscriptPath: s.path,
		},
		Turn:     spool.TurnRef{ID: turnID},
		Backfill: true,
	}
}

func (em *emitter) open(t *turn) (spool.Envelope, chunkSet) {
	env := em.envelope(spool.KindTurnOpen, t.id+":open", t.at, t.id)
	env.Turn.TranscriptOffset = t.begin
	chunks := chunkSet{}
	env.TurnOpen = &spool.TurnOpen{}
	if t.hasPrompt {
		env.TurnOpen.PromptChunk = chunks.add([]byte(t.prompt))
	}
	return env, chunks
}

// close carries the turn's last assistant text and its transcript lines,
// the tail when they run past the cap, as the Stop hook does.
func (em *emitter) close(t *turn) (spool.Envelope, chunkSet) {
	env := em.envelope(spool.KindTurnClose, t.id+":close", t.endAt, t.id)
	env.Turn.TranscriptOffset = t.begin
	chunks := chunkSet{}
	tc := &spool.TurnClose{Status: store.TurnClosed, TranscriptEnd: t.end}
	if t.final != "" {
		tc.FinalChunk = chunks.add([]byte(t.final))
	}
	if lines := readRange(em.main, t.begin, t.end, moondb.MaxTranscriptBytes); len(lines) > 0 {
		tc.TranscriptChunk = chunks.add(lines)
	}
	env.TurnClose = tc
	return env, chunks
}

// readRange returns the bytes of f in [begin, end), keeping the last limit
// bytes cut to start at a line when the range is longer.
func readRange(f io.ReaderAt, begin, end, limit int64) []byte {
	if end <= begin {
		return nil
	}
	start := max(begin, end-limit)
	data := make([]byte, end-start)
	n, _ := f.ReadAt(data, start) // a short read keeps what it got
	data = data[:n]
	if start > begin {
		if i := bytes.IndexByte(data, '\n'); i >= 0 {
			data = data[i+1:]
		}
	}
	return data
}

// edit builds the edit envelope for one answered file-tool call. Every
// file tool names one file, so the index is always 0, as in the hook.
func (em *emitter) edit(c *call) (spool.Envelope, chunkSet) {
	in, res := objectOf(c.input), objectOf(c.result)
	cwd := cmp.Or(c.cwd, em.s.cwd)
	name := c.target()
	abs := record.Resolve(name, cwd)
	loc, rel := em.places.place(abs, cwd, c.gitBranch, &em.counts.Placement)
	ep := &spool.EditPayload{
		Tool:         c.tool,
		ToolUseID:    c.id,
		AbsPath:      abs,
		Path:         rel,
		Location:     loc,
		Kind:         record.EditKind(c.tool, res.text("type")),
		UserModified: res.flag("userModified"),
	}
	chunks := chunkSet{}
	keys := pathKeys(name, abs, cwd)
	if record.Excluded(em.opts.excludes(), abs, rel) {
		ep.Excluded = true
	} else {
		em.fill(ep, c, in, res, keys, chunks)
	}
	em.note(keys, c.when())
	em.counts.Edits[c.tool]++
	if ep.PatchChunk == "" && !ep.Excluded {
		em.counts.EditsWithoutPatch++
	}
	env := em.envelope(spool.KindEdit, fmt.Sprintf("%s:%d", c.id, ep.Index), c.when(),
		em.turnOf(c))
	env.Agent = c.agent
	env.Edit = ep
	return env, chunks
}

// fill sets the content fields from the call and its answer. There is no
// observed read: the import runs long after the edit, so the disk says
// nothing about the moment it happened.
func (em *emitter) fill(
	ep *spool.EditPayload, c *call, in, res object, keys []string, chunks chunkSet,
) {
	ep.Input = c.input
	if p := res.array("structuredPatch"); p != nil {
		ep.PatchChunk = chunks.add(p)
	}
	original, ok := res.textOK("originalFile")
	if !ok {
		original, ok = res.textOK("originalFileContents")
	}
	var originalData []byte
	switch {
	case ok:
		originalData = []byte(original)
		ep.OriginalChunk = chunks.add(originalData)
	case ep.Kind != store.EditCreate:
		if data, found := em.fromHistory(keys, c.when()); found {
			originalData = data
			ep.OriginalChunk = chunks.add(data)
			em.counts.OriginalsFromHistory++
		}
	}
	if content, ok := in.textOK("content"); ok && c.tool == toolWrite {
		ep.ContentChunk = chunks.add([]byte(content))
		ep.EOL = patch.DetectEOL([]byte(content))
	} else if originalData != nil {
		ep.EOL = patch.DetectEOL(originalData)
	}
}

// fromHistory returns the file as the edit found it from the session's
// file history: the copy taken last before the edit, when no other edit
// of the file came between that copy and this edit.
func (em *emitter) fromHistory(keys []string, at time.Time) ([]byte, bool) {
	v, ok := em.s.history.latest(keys, at)
	if !ok || v.name == "" {
		return nil, false
	}
	for _, k := range keys {
		if last, seen := em.touched[k]; seen && !last.Before(v.at) {
			return nil, false
		}
	}
	return readVersion(em.opts.FileHistory, em.s.id, v.name)
}

// note remembers when the file was last edited, under every key.
func (em *emitter) note(keys []string, at time.Time) {
	for _, k := range keys {
		if at.After(em.touched[k]) {
			em.touched[k] = at
		}
	}
}

// target is the file the call names: the tool input's path, else the one
// its answer reports; "" when neither names one.
func (c *call) target() string {
	in, res := objectOf(c.input), objectOf(c.result)
	return cmp.Or(in.text("file_path"), in.text("notebook_path"), res.text("filePath"))
}

// pathKeys are the absolute paths an edited file may be listed under in the
// file history: as the tool named it, and physical.
func pathKeys(name, abs, cwd string) []string {
	keys := []string{abs}
	if !filepath.IsAbs(name) {
		name = filepath.Join(cwd, name)
	}
	if k := filepath.Clean(name); k != abs {
		keys = append(keys, k)
	}
	return keys
}

// turnOf names the turn an edit belongs to: the main turn its answer's
// prompt id names, else the main turn open when the main transcript
// answered it, else the last main turn that opened before it; "" when the
// edit came before any turn.
func (em *emitter) turnOf(c *call) string {
	if t, ok := em.s.turnByID[c.promptID]; ok {
		return t.id
	}
	if c.current != nil {
		return c.current.id
	}
	id := ""
	for _, t := range em.s.turns {
		if !t.at.After(c.when()) {
			id = t.id
		}
	}
	return id
}

// object is a JSON object decoded one level deep, so a field of the wrong
// type reads as absent instead of failing the whole call.
type object map[string]json.RawMessage

func objectOf(raw json.RawMessage) object {
	var o object
	if json.Unmarshal(raw, &o) != nil {
		return nil
	}
	return o
}

// text returns the string at key, "" when absent or not a string.
func (o object) text(key string) string {
	s, _ := o.textOK(key)
	return s
}

// textOK tells an empty string from a null or a missing field.
func (o object) textOK(key string) (string, bool) {
	raw := bytes.TrimSpace(o[key])
	if len(raw) == 0 || raw[0] != '"' {
		return "", false
	}
	var s string
	if json.Unmarshal(raw, &s) != nil {
		return "", false
	}
	return s, true
}

func (o object) flag(key string) bool {
	var b bool
	_ = json.Unmarshal(o[key], &b)
	return b
}

// array returns the raw value at key when it is a JSON array.
func (o object) array(key string) json.RawMessage {
	raw := bytes.TrimSpace(o[key])
	if len(raw) == 0 || raw[0] != '[' {
		return nil
	}
	return raw
}
