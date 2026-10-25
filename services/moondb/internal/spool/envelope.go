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
}

// SessionRef identifies the session a payload belongs to and carries what
// the hook knows about it; serve upserts the Session from it.
type SessionRef struct {
	ID             string `json:"id"`
	Cwd            string `json:"cwd,omitempty"`
	Title          string `json:"title,omitempty"`
	Model          string `json:"model,omitempty"`
	TranscriptPath string `json:"transcript_path,omitempty"`
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

// TurnOpen carries the prompt.
type TurnOpen struct {
	PromptChunk string `json:"prompt_chunk,omitempty"`
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
