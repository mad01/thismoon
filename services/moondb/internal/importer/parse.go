package importer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mad01/thismoon/services/moondb/internal/jsonl"
	"github.com/mad01/thismoon/services/moondb/internal/spool"
	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// session is one transcript parsed: its turns, its file-tool calls, and the
// file-history versions it lists.
type session struct {
	id, cwd, model string
	// branch is the git branch of the line that gave cwd, and entrypoint
	// the first one a line names: how the harness was started.
	branch, entrypoint string
	// title is the session's title by store.OfferTitle over its agent-name
	// and ai-title lines in transcript order, and titleSource the kind of
	// line it came from.
	title       string
	titleSource store.TitleSource
	// path is the main transcript; subagents counts the subagent
	// transcripts read beside it, and agentFiles names them.
	path       string
	subagents  int
	agentFiles []string
	turns      []*turn
	turnByID   map[string]*turn
	calls      map[string]*call
	order      []*call
	history    history
	stats      parseStats
}

// turn is one prompt and what followed it in the main transcript, up to the
// next prompt. begin and end are the byte range of its lines.
type turn struct {
	id         string
	at, endAt  time.Time
	prompt     string
	hasPrompt  bool
	final      string
	begin, end int64
}

// call is one file-tool call and its answer.
type call struct {
	id, tool  string
	input     json.RawMessage
	at        time.Time
	cwd       string
	gitBranch string
	agent     spool.AgentRef
	// The answer, once a tool_result names the call.
	answered bool
	failed   bool
	result   json.RawMessage // the toolUseResult object; nil when absent
	resultAt time.Time
	promptID string
	// current is the main turn open when the answer was read, for an answer
	// in the main transcript that names no prompt id.
	current *turn
}

// when is the time the change hit the disk, as close as the transcript
// tells: the answer's time, else the call's.
func (c *call) when() time.Time {
	if !c.resultAt.IsZero() {
		return c.resultAt
	}
	return c.at
}

// parseStats counts what parsing met but did not turn into records.
type parseStats struct {
	undecodable int
	unknown     map[string]int
	bash        int
}

func (p *parseStats) unknownType(t string) {
	if t == "" {
		t = "(none)"
	}
	if p.unknown == nil {
		p.unknown = map[string]int{}
	}
	p.unknown[t]++
}

// parseSession reads a main transcript and every subagent transcript in
// <session>/subagents beside it. A subagent transcript that cannot be read
// is skipped; only an unreadable main transcript is an error.
func parseSession(path string) (*session, error) {
	s := &session{
		path:     path,
		id:       strings.TrimSuffix(filepath.Base(path), ".jsonl"),
		turnByID: map[string]*turn{},
		calls:    map[string]*call{},
	}
	if err := s.read(path, &reader{s: s, main: true}); err != nil {
		return nil, err
	}
	subs, _ := filepath.Glob(filepath.Join(strings.TrimSuffix(path, ".jsonl"),
		"subagents", "agent-*.jsonl"))
	for _, sub := range subs {
		if err := s.read(sub, &reader{s: s, agent: subagentRef(sub)}); err == nil {
			s.subagents++
			s.agentFiles = append(s.agentFiles, sub)
		}
	}
	return s, nil
}

// read streams one transcript file through r.
func (s *session) read(path string, r *reader) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("importer: open transcript: %w", err)
	}
	defer f.Close()
	oversized, err := jsonl.Scan(f, maxLineBytes, r.line)
	s.stats.undecodable += oversized
	if err != nil {
		return fmt.Errorf("importer: read %s: %w", filepath.Base(path), err)
	}
	return nil
}

// subagentRef names the agent behind a subagent transcript: the id from its
// file name, the type from the meta file beside it. Entries that carry an
// agentId override the id.
func subagentRef(path string) spool.AgentRef {
	ref := spool.AgentRef{
		ID: strings.TrimSuffix(strings.TrimPrefix(filepath.Base(path), "agent-"), ".jsonl"),
	}
	raw, err := os.ReadFile(strings.TrimSuffix(path, ".jsonl") + ".meta.json")
	if err != nil {
		return ref
	}
	var meta struct {
		AgentType string `json:"agentType"`
	}
	if json.Unmarshal(raw, &meta) == nil {
		ref.Type = meta.AgentType
	}
	return ref
}

// reader folds the lines of one transcript file into the session. The main
// transcript opens turns; a subagent transcript only adds calls.
type reader struct {
	s     *session
	main  bool
	agent spool.AgentRef
	// cur is the turn the main transcript is in. pids is set once an entry
	// with a prompt id was read; from then on only a new prompt id opens a
	// turn, and a transcript without prompt ids opens one per prompt.
	cur  *turn
	pids bool
}

// line folds one line in; it always asks for the next.
func (r *reader) line(raw []byte, end int64) bool {
	begin := end - int64(len(raw))
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return true
	}
	var e entry
	if err := json.Unmarshal(raw, &e); err != nil {
		r.s.stats.undecodable++
		return true
	}
	if r.main && e.SessionID != "" {
		r.s.id = e.SessionID
	}
	switch e.Type {
	case typeUser:
		r.user(e, begin)
	case typeAssistant:
		r.assistant(e)
	case typeAgentName:
		name := strings.TrimSpace(e.AgentName)
		store.OfferTitle(&r.s.title, &r.s.titleSource, name, store.TitleNamed)
	case typeAITitle:
		generated := strings.TrimSpace(e.AITitle)
		store.OfferTitle(&r.s.title, &r.s.titleSource, generated, store.TitleGenerated)
	case typeSnapshot, typeDelta:
		r.s.history.add(e, r.s.cwd)
	default:
		if !ignoredTypes[e.Type] {
			r.s.stats.unknownType(e.Type)
		}
	}
	r.track(e, end)
	return true
}

// track notes the session's cwd with its branch and its entrypoint, and
// moves the open turn's end past this line.
func (r *reader) track(e entry, end int64) {
	if !r.main {
		return
	}
	if r.s.cwd == "" && e.Cwd != "" {
		r.s.cwd, r.s.branch = e.Cwd, e.GitBranch
	}
	if r.s.entrypoint == "" {
		r.s.entrypoint = e.Entrypoint
	}
	if r.cur == nil {
		return
	}
	r.cur.end = end
	if at := e.at(); !at.IsZero() {
		r.cur.endAt = at
	}
}

// user reads a user entry: in the main transcript it may open a turn and
// carry its prompt; anywhere it may answer a file-tool call.
func (r *reader) user(e entry, begin int64) {
	_, c, ok := e.content()
	if !ok {
		r.s.stats.undecodable++
		return
	}
	if r.main && !e.IsSidechain {
		r.turnAt(e, c, begin)
	}
	for _, b := range c.blocks {
		if b.Type == "tool_result" {
			r.answer(e, b)
		}
	}
}

// turnAt opens a turn when e starts one, and takes e's text as the turn's
// prompt when the turn has none yet.
func (r *reader) turnAt(e entry, c content, begin int64) {
	text, isPrompt := c.prompt()
	isPrompt = isPrompt && !e.IsMeta
	id := e.PromptID
	switch {
	case id != "":
		r.pids = true
	case !r.pids && isPrompt:
		id = e.UUID
	}
	if id != "" && (r.cur == nil || r.cur.id != id) {
		r.open(id, e.at(), begin)
	}
	if r.cur != nil && isPrompt && !r.cur.hasPrompt {
		r.cur.prompt, r.cur.hasPrompt = text, true
	}
}

// open makes id the current turn. An id seen before is reopened rather than
// added twice; real transcripts keep a prompt id's lines together.
func (r *reader) open(id string, at time.Time, begin int64) {
	if t, ok := r.s.turnByID[id]; ok {
		r.cur = t
		return
	}
	t := &turn{id: id, at: at, endAt: at, begin: begin, end: begin}
	r.s.turns = append(r.s.turns, t)
	r.s.turnByID[id] = t
	r.cur = t
}

// answer records a tool_result for a file-tool call read earlier.
func (r *reader) answer(e entry, b block) {
	c, ok := r.s.calls[b.ToolUseID]
	if !ok || c.answered {
		return
	}
	c.answered, c.failed = true, b.IsError
	c.resultAt, c.promptID = e.at(), e.PromptID
	if raw := bytes.TrimSpace(e.ToolUseResult); len(raw) > 0 && raw[0] == '{' {
		c.result = e.ToolUseResult
	}
	if r.main {
		c.current = r.cur
	}
}

// assistant reads an assistant entry: its file-tool calls, and in the main
// transcript the text that may end the turn and the model.
func (r *reader) assistant(e entry) {
	m, c, ok := e.content()
	if !ok {
		r.s.stats.undecodable++
		return
	}
	main := r.main && !e.IsSidechain
	if main && r.s.model == "" && m.Model != "" && !strings.HasPrefix(m.Model, "<") {
		r.s.model = m.Model
	}
	for _, b := range c.blocks {
		switch {
		case b.Type == "text" && main && r.cur != nil && b.Text != "":
			r.cur.final = b.Text
		case b.Type == "tool_use" && b.Name == toolBash:
			r.s.stats.bash++
		case b.Type == "tool_use" && fileTool(b.Name) && r.s.calls[b.ID] == nil:
			r.addCall(e, b)
		}
	}
}

func (r *reader) addCall(e entry, b block) {
	agent := r.agent
	if e.AgentID != "" {
		agent.ID = e.AgentID
	}
	c := &call{
		id: b.ID, tool: b.Name, input: b.Input, at: e.at(),
		cwd: e.Cwd, gitBranch: e.GitBranch, agent: agent,
	}
	r.s.calls[b.ID] = c
	r.s.order = append(r.s.order, c)
}
