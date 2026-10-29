// Package spool is the on-disk inbox between the capture hook and the
// service. `moondb record` and `moondb import` write envelopes here and
// return; `moondb serve`, the only writer of the store, drains them. The
// spool is not the store: an envelope lives until serve has ingested it,
// and nothing is lost while serve is down or restarting.
//
// Layout under <workdir>/spool:
//
//	tmp/                 envelopes and chunks being written
//	<nanos>-<kind>-<id>.json   one Envelope, renamed into place last
//	<sha>.chunk          content a pending envelope points at, raw bytes
//
// A writer puts every chunk file in place before it renames the envelope,
// so a reader that sees an envelope sees its chunks. A chunk already in the
// store's object directory is not written again. Serve ingests envelopes in
// name order, stores their chunks, appends the records, and deletes the
// envelope; a chunk file goes once no pending envelope names it. Ingest is
// idempotent on Envelope.ID, so a crash between append and delete replays
// harmlessly.
package spool

import (
	"encoding/json"
	"time"

	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// Version is the envelope schema version this build writes and reads.
const Version = 1

// Kind says which payload an Envelope carries.
type Kind string

const (
	// KindTurnOpen opens a turn: a prompt was submitted.
	KindTurnOpen Kind = "turn-open"
	// KindTurnClose closes a turn: the session stopped.
	KindTurnClose Kind = "turn-close"
	// KindEdit records one file a tool call changed.
	KindEdit Kind = "edit"
	// KindHeadMoved reports that a worktree's HEAD changed since the hook
	// last saw it, with the blobs of the new commits, so serve can link
	// recorded content to commits.
	KindHeadMoved Kind = "head-moved"
	// KindSnapshot records one worktree's state at a turn boundary.
	KindSnapshot Kind = "snapshot"
	// KindSkipped counts the files one tool call changed that the hook
	// skipped, so serve can show the count on the call's turn.
	KindSkipped Kind = "skipped"
	// KindAgentClose records one subagent when it stops: what kind it was,
	// the turns it ran under and its agent transcript. Envelope.Agent
	// names it.
	KindAgentClose Kind = "agent-close"
)

// Envelope is one record handed to serve. Exactly one payload field is set,
// the one Kind names. ID is the idempotency key: the same ID ingested twice
// changes nothing.
type Envelope struct {
	Version int       `json:"v"`
	Kind    Kind      `json:"kind"`
	ID      string    `json:"id"`
	At      time.Time `json:"at"`
	// Harness names the agent runtime that produced the hook payload.
	Harness string     `json:"harness"`
	Session SessionRef `json:"session"`
	Turn    TurnRef    `json:"turn"`
	Agent   AgentRef   `json:"agent,omitempty"`
	// Backfill marks an envelope `moondb import` wrote from a transcript.
	// Its session and turn fields only fill what the stored record has
	// empty; every value a record already holds stays.
	Backfill bool `json:"backfill,omitempty"`

	TurnOpen  *TurnOpen    `json:"turn_open,omitempty"`
	TurnClose *TurnClose   `json:"turn_close,omitempty"`
	Edit      *EditPayload `json:"edit,omitempty"`
	HeadMoved *HeadMoved   `json:"head_moved,omitempty"`
	Snapshot  *Snapshot    `json:"snapshot,omitempty"`
	// Skipped is the payload of a skipped envelope. Its At stays zero:
	// the envelope's own At says when the hook began.
	Skipped    *store.SkippedFiles `json:"skipped,omitempty"`
	AgentClose *AgentClose         `json:"agent_close,omitempty"`
}

// SessionRef identifies the session a payload belongs to and carries what
// the hook knows about it; serve upserts the Session from it.
type SessionRef struct {
	ID    string `json:"id"`
	Cwd   string `json:"cwd,omitempty"`
	Title string `json:"title,omitempty"`
	// TitleSource says where Title came from. The hook leaves it empty,
	// which ranks its session title as generated (store.OfferTitle); an
	// import names the transcript line its title came from.
	TitleSource    store.TitleSource `json:"title_source,omitempty"`
	Model          string            `json:"model,omitempty"`
	TranscriptPath string            `json:"transcript_path,omitempty"`
	// Location is where Cwd sits in git. The hook sets it on a turn-open
	// only, an import on every envelope.
	Location store.Location `json:"location,omitzero"`
	// Entrypoint is the harness entrypoint: the hook's
	// CLAUDE_CODE_ENTRYPOINT, or an import's first transcript line that
	// names a valid one, empty when invalid (store.ParseEntrypoint).
	Entrypoint string `json:"entrypoint,omitempty"`
	// Tags are the session tags the hook read from its environment
	// (store.ParseTags); an import sets none.
	Tags []string `json:"tags,omitempty"`
}

// TurnRef identifies the turn a payload belongs to. TranscriptOffset is the
// transcript file size when the turn opened, so the close can tail from it.
type TurnRef struct {
	ID               string `json:"id"`
	TranscriptOffset int64  `json:"transcript_offset,omitempty"`
}

// AgentRef names the subagent that made the call; zero for the main
// conversation.
type AgentRef struct {
	ID   string `json:"id,omitempty"`
	Type string `json:"type,omitempty"`
}

// TurnOpen carries the prompt and what it says opened the turn.
type TurnOpen struct {
	PromptChunk string `json:"prompt_chunk,omitempty"`
	// Origin is store.PromptOrigin of the prompt; empty from a build that
	// did not classify it.
	Origin store.TurnOrigin `json:"origin,omitempty"`
}

// TurnClose carries what the session said last and the transcript lines of
// the turn.
type TurnClose struct {
	FinalChunk      string `json:"final_chunk,omitempty"`
	TranscriptChunk string `json:"transcript_chunk,omitempty"`
	// TranscriptEnd is the transcript file size at close.
	TranscriptEnd int64            `json:"transcript_end,omitempty"`
	Status        store.TurnStatus `json:"status"`
}

// AgentClose is one subagent as it stopped, read from its transcript file
// and the meta file beside it. Serve puts the Agent record from it and
// computes the agent's totals from its transcript lines.
type AgentClose struct {
	Type        string `json:"type"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Model       string `json:"model,omitempty"`
	// TurnIDs are the main turns the agent's lines name, in order.
	TurnIDs  []string       `json:"turn_ids,omitempty"`
	Cwd      string         `json:"cwd,omitempty"`
	Location store.Location `json:"location"`
	// StartedAt is the agent's first line's time; EndedAt when it stopped,
	// or its last line's time when the stop is not known (an import).
	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at"`
	// TranscriptChunk holds the agent transcript: the agent's lines cut
	// to heads and capped (moondb.MaxAgentTranscriptBytes).
	TranscriptChunk string           `json:"transcript_chunk,omitempty"`
	Lines           store.AgentLines `json:"lines"`
	// BriefChunk holds the agent's spawn brief, the text of its
	// transcript's first user line (moondb.MaxBriefBytes); empty when it
	// had none.
	BriefChunk string `json:"brief_chunk,omitempty"`
	// LaterMessages counts the messages its transcript shows reaching it
	// after the spawn brief.
	LaterMessages int `json:"later_messages,omitempty"`
}

// EditPayload is everything the hook could gather about one file a tool
// call changed. Serve derives the Edit record from it (internal/derive):
// the before-content from the chain of sources, the after-content from the
// patch, and the observed read as evidence.
type EditPayload struct {
	// Tool is the harness tool name and ToolUseID its call id; Index is the
	// file's position within the call for tools that touch several.
	Tool      string `json:"tool"`
	ToolUseID string `json:"tool_use_id"`
	Index     int    `json:"index"`
	// AbsPath is the file as the tool named it, symlinks resolved; Path is
	// worktree-relative with forward slashes when Location.Worktree is set.
	AbsPath  string         `json:"abs_path"`
	Path     string         `json:"path"`
	Location store.Location `json:"location"`
	Kind     store.EditKind `json:"kind"`
	// Input is the tool_input as the harness sent it, kept verbatim for the
	// record (old_string, new_string, content, replace_all, ...).
	Input json.RawMessage `json:"input,omitempty"`
	// PatchChunk holds the harness's structuredPatch JSON for this file.
	PatchChunk string `json:"patch_chunk,omitempty"`
	// OriginalChunk holds the harness's originalFile when it shipped one.
	OriginalChunk string `json:"original_chunk,omitempty"`
	// ContentChunk holds the whole after-content when the tool input
	// carried it (Write).
	ContentChunk string `json:"content_chunk,omitempty"`
	// ObservedChunk, ObservedBlob and ObservedSize describe the file as read
	// from disk right after the tool ran; the chunk is empty past the cap.
	ObservedChunk string `json:"observed_chunk,omitempty"`
	ObservedBlob  string `json:"observed_blob,omitempty"`
	ObservedSize  int64  `json:"observed_size,omitempty"`
	// HeadBlob is the git blob id of Path at Location.Head, and HeadChunk
	// its content when the hook shipped it (tracked file under the cap).
	HeadBlob  string `json:"head_blob,omitempty"`
	HeadChunk string `json:"head_chunk,omitempty"`
	// EOL is the line ending convention seen in the observed content.
	EOL string `json:"eol,omitempty"`
	// Excluded is set when a capture.exclude glob matched; no chunk fields
	// are set then.
	Excluded bool `json:"excluded,omitempty"`
	// UserModified is the harness's flag for an edit the person changed in
	// the permission dialog.
	UserModified bool `json:"user_modified,omitempty"`
}

// HeadMoved reports new commits in a worktree. Files lists every path the
// range OldHead..NewHead changed with its blob in NewHead's tree, so serve
// can match blobs against recorded after-content.
type HeadMoved struct {
	Location store.Location `json:"location"`
	OldHead  string         `json:"old_head"`
	NewHead  string         `json:"new_head"`
	Files    []ChangedFile  `json:"files"`
	// Truncated is set when the range changed more files than the hook
	// was willing to list.
	Truncated bool `json:"truncated,omitempty"`
}

// ChangedFile is one path a commit range changed.
type ChangedFile struct {
	Path string `json:"path"`
	// Blob is the git blob id in the new tree; "" for a deletion.
	Blob string `json:"blob,omitempty"`
	// Status is git's diff-tree status letter (A, M, D, R, ...).
	Status string `json:"status,omitempty"`
}

// Snapshot is one worktree's state at a turn boundary: every path in Scope
// with its content as a chunk, deduplicated like any chunk. Serve stores
// the file list as one chunk and compares each file with the latest
// content it knew of the path before, so a change no tool call explains
// shows up as an unattributed delta on the turn boundary.
type Snapshot struct {
	Location store.Location      `json:"location"`
	Phase    store.SnapshotPhase `json:"phase"`
	// Started is when the hook began this snapshot; the envelope's At is
	// when its reads were done.
	Started time.Time `json:"started,omitzero"`
	// Scope is the rule that chose Files (store.SnapshotScope).
	Scope string `json:"scope"`
	// Files lists the paths in scope, sorted by path, each with the chunk
	// of its content when the hook read it.
	Files []store.SnapshotFile `json:"files"`
	// Truncated is set when Files lacks paths the scope held: the budget
	// ran out or the file cap was reached. Skipped counts them by reason.
	Truncated bool                  `json:"truncated,omitempty"`
	Skipped   store.SnapshotSkipped `json:"skipped"`
	// StateHash is what the hook compared with the last snapshot it took of
	// the worktree in the session: HEAD, git's status output, and the size
	// and modification time of every path in scope.
	StateHash string `json:"state_hash"`
}
