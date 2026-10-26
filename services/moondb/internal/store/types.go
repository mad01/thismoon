// Package store defines what moondb remembers and the Store a backend has
// to implement. The records are append-only facts about what a session did
// between commits; nothing here merges or rewrites history.
//
// Vocabulary (CONTEXT.md): a Session is one agent session; a Turn is one
// prompt until the session stops; an Edit is one recorded change to one
// file by one tool call; an UnattributedDelta is a change between two known
// contents that no tool call explains; a Snapshot is a worktree's dirty set
// at a turn boundary; a CommitLink ties recorded content to a commit by its
// git blob id; a Chunk is content-addressed bytes.
package store

import (
	"cmp"
	"errors"
	"slices"
	"time"
)

// ErrNotFound is returned when no record has the given id.
var ErrNotFound = errors.New("store: not found")

// Session is one agent session, created lazily from its first turn.
type Session struct {
	// ID is the harness's session id (Claude Code session_id).
	ID string `json:"id"`
	// Harness names the agent runtime, "claude-code" today.
	Harness string `json:"harness"`
	// Cwd is the working directory the session started in.
	Cwd string `json:"cwd"`
	// Title is the harness's session title when it has one: the hook's
	// session title, or the name or generated title a transcript line
	// gives. OfferTitle decides which one it holds.
	Title string `json:"title,omitempty"`
	// TitleSource says where Title came from, which ranks it against the
	// next title offered.
	TitleSource TitleSource `json:"title_source,omitempty"`
	// PromptLine is the first line of the first prompt serve saw for the
	// session, kept short, so a listing can name the session without
	// reading a chunk. It is filled once and never replaced.
	PromptLine string `json:"prompt_line,omitempty"`
	// Model is the model the session ran on when the harness said.
	Model string `json:"model,omitempty"`
	// TranscriptPath is the harness transcript file, kept so a turn's raw
	// lines can be shown while the file still exists.
	TranscriptPath string     `json:"transcript_path,omitempty"`
	StartedAt      time.Time  `json:"started_at"`
	EndedAt        *time.Time `json:"ended_at,omitempty"`
}

// TitleSource is where a session title came from.
type TitleSource string

const (
	// TitleNamed is a name the person gave the session: a transcript
	// agent-name line.
	TitleNamed TitleSource = "named"
	// TitleGenerated is a title the harness generated: a transcript
	// ai-title line, or the hook's session title, whose origin the hook
	// does not say.
	TitleGenerated TitleSource = "generated"
)

// known is s, or TitleGenerated for any value but TitleNamed: an empty
// source on a title stored before sources existed, or on an envelope
// that did not say, ranks as generated.
func (s TitleSource) known() TitleSource {
	if s == TitleNamed {
		return TitleNamed
	}
	return TitleGenerated
}

// OfferTitle sets *title and *source to candidate from src when it
// outranks the title they hold, and reports whether it did. A name the
// person gave outranks a generated title, and within one rank the first
// title seen stays; an empty candidate changes nothing. Every path that
// names a session goes through it: the hook's title, the live turn close,
// the totals backfill and an import.
func OfferTitle(title *string, source *TitleSource, candidate string, src TitleSource) bool {
	if candidate == "" {
		return false
	}
	src = src.known()
	if *title != "" && (src == TitleGenerated || source.known() == TitleNamed) {
		return false
	}
	*title, *source = candidate, src
	return true
}

// TurnStatus is where a turn is in its life.
type TurnStatus string

const (
	TurnOpen   TurnStatus = "open"
	TurnClosed TurnStatus = "closed"
	TurnFailed TurnStatus = "failed"
)

// Turn is one user prompt and everything until the session stopped.
type Turn struct {
	// ID is the harness's prompt id (Claude Code prompt_id).
	ID        string `json:"id"`
	SessionID string `json:"session_id"`
	// Seq orders turns within a session in arrival order, from 1.
	Seq int `json:"seq"`
	// PromptChunk holds the prompt text.
	PromptChunk string `json:"prompt_chunk,omitempty"`
	// FinalChunk holds the last assistant message of the turn.
	FinalChunk string `json:"final_chunk,omitempty"`
	// TranscriptChunk holds the raw transcript lines written during the
	// turn, from TranscriptOffset to the offset seen at close.
	TranscriptChunk  string     `json:"transcript_chunk,omitempty"`
	TranscriptOffset int64      `json:"transcript_offset"`
	Status           TurnStatus `json:"status"`
	StartedAt        time.Time  `json:"started_at"`
	EndedAt          *time.Time `json:"ended_at,omitempty"`
	// Totals is what the turn's trace adds up to, computed by serve when
	// the turn closes. It is nil on an open turn, and on a turn stored
	// before totals existed until serve's totals backfill reaches it. Replace it
	// whole, never change it in place: a backend may share it.
	Totals *TurnTotals `json:"totals,omitempty"`
	// Skipped lists, per tool call, the files the call changed that
	// capture counted instead of recording as Edits, so a turn whose Bash
	// command pulled or checked out files says so. Replace it whole, never
	// change it in place, like Totals.
	Skipped []SkippedFiles `json:"skipped,omitempty"`
	// Origin is what opened the turn, read from the prompt's head when the
	// turn opened (PromptOrigin). It is "" on a turn stored before origins
	// existed; a reader classifies its prompt then.
	Origin TurnOrigin `json:"origin,omitempty"`
}

// SkippedFiles counts the skipped files of one tool call, the ones capture
// counted instead of recording as Edits, by reason. Only the Bash hook
// skips files today.
type SkippedFiles struct {
	Tool      string    `json:"tool"`
	ToolUseID string    `json:"tool_use_id"`
	At        time.Time `json:"at,omitzero"`
	// Clean counts the files git status reported clean after the call: a
	// pull, checkout, reset or stash brought them to their committed
	// state, and the call authored no Edit of them.
	Clean int `json:"clean,omitempty"`
	// Timeout counts the files the hook's budget, or a git call's, ran out
	// on before the hook could place, check or read them.
	Timeout int `json:"timeout,omitempty"`
	// Failed counts the files whose status the hook could not read: git
	// failed or gave more than moondb.MaxStatusBytes of status, or the file
	// is gone and no worktree holds it.
	Failed int `json:"failed,omitempty"`
}

// Total is every file of the call the hook skipped.
func (s SkippedFiles) Total() int { return s.Clean + s.Timeout + s.Failed }

// TurnTotals is what a turn's trace adds up to, kept on the Turn so a
// listing reads it without the transcript chunk. The *MS fields split
// WallMS by what was running. It holds no cost: a reader prices Usage with
// the table it has, so a price change reprices every stored turn.
type TurnTotals struct {
	// Version is the trace build that computed the totals. serve computes
	// them again when its own version is newer.
	Version   int   `json:"version"`
	WallMS    int64 `json:"wall_ms"`
	ModelMS   int64 `json:"model_ms"`
	ToolMS    int64 `json:"tool_ms"`
	HarnessMS int64 `json:"harness_ms"`
	UserMS    int64 `json:"user_ms"`
	// Usage is the tokens the turn's model calls billed, one entry per
	// model and rate, sorted (MergeUsage).
	Usage      []ModelUsage `json:"usage,omitempty"`
	ModelCalls int          `json:"model_calls"`
	ToolCalls  int          `json:"tool_calls"`
	ToolErrors int          `json:"tool_errors"`
}

// Tokens counts what model calls billed, by kind. Thinking is part of
// Output and is not billed again.
type Tokens struct {
	Input        int64 `json:"input"`
	CacheWrite5m int64 `json:"cache_write_5m"`
	CacheWrite1h int64 `json:"cache_write_1h"`
	CacheRead    int64 `json:"cache_read"`
	Output       int64 `json:"output"`
	Thinking     int64 `json:"thinking,omitempty"`
}

// Add returns the sum of t and o.
func (t Tokens) Add(o Tokens) Tokens {
	return Tokens{
		Input:        t.Input + o.Input,
		CacheWrite5m: t.CacheWrite5m + o.CacheWrite5m,
		CacheWrite1h: t.CacheWrite1h + o.CacheWrite1h,
		CacheRead:    t.CacheRead + o.CacheRead,
		Output:       t.Output + o.Output,
		Thinking:     t.Thinking + o.Thinking,
	}
}

// Prompt is a call's whole prompt: input plus cache writes and reads.
func (t Tokens) Prompt() int64 {
	return t.Input + t.CacheWrite5m + t.CacheWrite1h + t.CacheRead
}

// Total is every billed token: the prompt plus the output.
func (t Tokens) Total() int64 { return t.Prompt() + t.Output }

// ModelUsage is the tokens of the model calls that bill at one rate: one
// model, at one speed, in one inference region, on one side of the
// model's long-prompt threshold. Those are what set a call's price
// besides its tokens, so a reader can price the entry with any table.
type ModelUsage struct {
	Model string `json:"model"`
	// Speed is "fast" for fast mode, "" for standard speed.
	Speed string `json:"speed,omitempty"`
	// Geo is the usage's inference_geo: "us" for US-only inference.
	Geo string `json:"geo,omitempty"`
	// Long is set for calls whose prompt was over the model's long-prompt
	// threshold in the price table that computed the entry.
	Long   bool   `json:"long,omitempty"`
	Tokens Tokens `json:"tokens"`
}

// sameRate reports whether u and o bill at one rate.
func (u ModelUsage) sameRate(o ModelUsage) bool {
	return u.Model == o.Model && u.Speed == o.Speed && u.Geo == o.Geo && u.Long == o.Long
}

// cmpRate orders usage entries by model, then speed, region and tier.
func cmpRate(a, b ModelUsage) int {
	return cmp.Or(
		cmp.Compare(a.Model, b.Model),
		cmp.Compare(a.Speed, b.Speed),
		cmp.Compare(a.Geo, b.Geo),
		compareBool(a.Long, b.Long),
	)
}

func compareBool(a, b bool) int {
	switch {
	case a == b:
		return 0
	case a:
		return 1
	}
	return -1
}

// MergeUsage sums usage lists into a new one with one entry per rate,
// sorted by model, then speed, region and tier. It never changes its
// arguments, so it is safe on a stored turn's Usage.
func MergeUsage(lists ...[]ModelUsage) []ModelUsage {
	var out []ModelUsage
	for _, list := range lists {
		for _, u := range list {
			i := slices.IndexFunc(out, u.sameRate)
			if i < 0 {
				out = append(out, u)
				continue
			}
			out[i].Tokens = out[i].Tokens.Add(u.Tokens)
		}
	}
	slices.SortFunc(out, cmpRate)
	return out
}

// Location is where an edited file sits in git, resolved from the file's
// own directory at capture time (kit/gitloc). Repo is "" for a file outside
// any repository; such edits are grouped under Worktree "" and keep an
// absolute Path.
type Location struct {
	// Repo is the canonical host/owner/repo identity from the origin remote.
	Repo string `json:"repo,omitempty"`
	// Worktree is the root of the working tree the file belongs to.
	Worktree string `json:"worktree,omitempty"`
	// Canonical is the root of the main checkout; equals Worktree outside a
	// linked worktree.
	Canonical string `json:"canonical,omitempty"`
	// Branch is the branch checked out in the worktree at the time.
	Branch string `json:"branch,omitempty"`
	// Head is the commit checked out in the worktree at the time.
	Head string `json:"head,omitempty"`
}

// EditKind says what the tool call did to the file.
type EditKind string

const (
	EditCreate EditKind = "create"
	EditUpdate EditKind = "update"
	EditDelete EditKind = "delete"
	// EditUnknown covers tools that report a path and nothing else.
	EditUnknown EditKind = "unknown"
)

// BeforeSource says where an Edit's before-content came from, in the order
// the deriver tries them (internal/derive).
type BeforeSource string

const (
	// BeforeAbsent: the file did not exist before the call; a create.
	BeforeAbsent BeforeSource = "absent"
	// BeforeOriginal: the harness shipped the whole pre-edit file.
	BeforeOriginal BeforeSource = "original"
	// BeforeChain: the previous Edit's after-content for the same path.
	BeforeChain BeforeSource = "chain"
	// BeforeSnapshot: the pre-turn snapshot of the file.
	BeforeSnapshot BeforeSource = "snapshot"
	// BeforeHead: the file at HEAD, shipped by the hook when it was clean.
	BeforeHead BeforeSource = "head"
	// BeforeObserved: the post-edit disk read with the patch reversed.
	BeforeObserved BeforeSource = "observed"
	// BeforeUnknown: hunks only; no full before-content could be derived.
	BeforeUnknown BeforeSource = "unknown"
)

// Edit is one recorded change to one file by one tool call. It is an
// immutable fact: identity is the tool call plus the path, and a replayed
// spool envelope must not produce a second one.
type Edit struct {
	// ID is ToolUseID plus the path index within the call ("<tool_use_id>:0").
	ID        string `json:"id"`
	ToolUseID string `json:"tool_use_id"`
	SessionID string `json:"session_id"`
	TurnID    string `json:"turn_id"`
	// AgentID and AgentType name the subagent that made the edit; both are
	// "" for the main conversation.
	AgentID   string `json:"agent_id,omitempty"`
	AgentType string `json:"agent_type,omitempty"`
	// Tool is the harness tool name: Edit, Write, MultiEdit, NotebookEdit, Bash.
	Tool string    `json:"tool"`
	At   time.Time `json:"at"`
	Location
	// Path is relative to Worktree with forward slashes, or absolute when
	// the file is outside any repository.
	Path string   `json:"path"`
	Kind EditKind `json:"kind"`
	// PatchChunk holds the harness's hunks for this file as JSON
	// (structuredPatch), "" when the tool reported none.
	PatchChunk string `json:"patch_chunk,omitempty"`
	// BeforeChunk and AfterChunk hold the whole file before and after; ""
	// when unknown or withheld.
	BeforeChunk  string       `json:"before_chunk,omitempty"`
	AfterChunk   string       `json:"after_chunk,omitempty"`
	BeforeSource BeforeSource `json:"before_source"`
	// AfterBlob is the git blob id of the after-content, the join key to a
	// later commit.
	AfterBlob string `json:"after_blob,omitempty"`
	// ObservedChunk is what the hook read from disk right after the tool
	// ran. Evidence, not truth: ObservedMatches reports whether it equals
	// the derived after-content; nil when either side is unknown.
	ObservedChunk   string `json:"observed_chunk,omitempty"`
	ObservedMatches *bool  `json:"observed_matches,omitempty"`
	// EOL records the line ending convention of the content: lf, crlf,
	// mixed, or "" when unknown.
	EOL string `json:"eol,omitempty"`
	// Excluded is set when a capture.exclude glob matched: the edit is
	// recorded by path and kind, with no content and no patch.
	Excluded bool `json:"excluded,omitempty"`
}

// DeltaCause says why an UnattributedDelta exists.
type DeltaCause string

const (
	// DeltaObservedMismatch: the disk read after a tool call differed from
	// the content the patch derives, so something else wrote the file too.
	DeltaObservedMismatch DeltaCause = "observed-mismatch"
	// DeltaSnapshot: a turn boundary snapshot saw a change no Edit made.
	DeltaSnapshot DeltaCause = "snapshot"
)

// UnattributedDelta is a change between two known contents of a path that no
// tool call explains: a formatter hook, a Bash command without a diff, a
// human editing between turns.
type UnattributedDelta struct {
	ID        string    `json:"id"`
	SessionID string    `json:"session_id"`
	TurnID    string    `json:"turn_id"`
	At        time.Time `json:"at"`
	Location
	Path       string     `json:"path"`
	FromChunk  string     `json:"from_chunk,omitempty"`
	ToChunk    string     `json:"to_chunk,omitempty"`
	PatchChunk string     `json:"patch_chunk,omitempty"`
	Cause      DeltaCause `json:"cause"`
	// Ambiguous is set when another session had an open turn in the same
	// worktree, so the change may be theirs.
	Ambiguous bool `json:"ambiguous,omitempty"`
}

// SnapshotPhase is which turn boundary a snapshot was taken at.
type SnapshotPhase string

const (
	// SnapshotPre is taken when a prompt opens the turn.
	SnapshotPre SnapshotPhase = "pre"
	// SnapshotPost is taken when the session stops at the turn's end.
	SnapshotPost SnapshotPhase = "post"
)

// SnapshotScope names the rule that chose a snapshot's files: the
// worktree's dirty paths, tracked and untracked, plus the paths the session
// recorded edits of. An excluded path is listed without its content.
const SnapshotScope = "dirty+touched"

// Snapshot is the state of one worktree at a turn boundary: its dirty
// paths plus the session's touched paths, each with its content as a
// chunk. The file map lives in a chunk (a JSON array of SnapshotFile,
// sorted by path) so the index stays small. A path the snapshot could not
// read is listed without content and with its reason, so a path missing
// from a snapshot that is not Truncated was clean then: its content was
// HEAD's. Git status does not show ignored files or files marked
// skip-worktree or assume-unchanged, so a snapshot lists those only when
// the session that took it edited them, and their absence says nothing.
type Snapshot struct {
	ID        string        `json:"id"`
	SessionID string        `json:"session_id"`
	TurnID    string        `json:"turn_id"`
	Phase     SnapshotPhase `json:"phase"`
	// Started is when the hook began the capture and At when its reads were
	// done. An edit stamped between the two may or may not be in what the
	// snapshot read. A record without Started began at At.
	Started time.Time `json:"started_at,omitzero"`
	At      time.Time `json:"at"`
	Location
	// Scope is the rule that chose the files, SnapshotScope today.
	Scope     string `json:"scope,omitempty"`
	MapChunk  string `json:"map_chunk,omitempty"`
	FileCount int    `json:"file_count"`
	// Truncated is set when the map lacks paths the scope held, because
	// the budget ran out or the file cap was reached.
	Truncated bool            `json:"truncated,omitempty"`
	Skipped   SnapshotSkipped `json:"skipped"`
}

// SnapshotSkipped counts the paths in a snapshot's scope whose content it
// does not hold, by reason.
type SnapshotSkipped struct {
	// Timeout counts paths left out when the budget ran out.
	Timeout int `json:"timeout,omitempty"`
	// TooMany counts paths left out past the file cap.
	TooMany int `json:"too_many,omitempty"`
	// TooLarge counts files listed without content: over the chunk cap.
	TooLarge int `json:"too_large,omitempty"`
	// Excluded counts paths a capture.exclude glob matched, listed without
	// content.
	Excluded int `json:"excluded,omitempty"`
}

// SnapshotReason says why a snapshot lists a path that exists without its
// content.
type SnapshotReason string

const (
	// ReasonExcluded: a capture.exclude glob matched the path.
	ReasonExcluded SnapshotReason = "excluded"
	// ReasonSymlink: the path is a symbolic link.
	ReasonSymlink SnapshotReason = "symlink"
	// ReasonNotRegular: the path is neither a regular file nor a symlink.
	ReasonNotRegular SnapshotReason = "not-regular"
	// ReasonUnreadable: the path could not be stated or read whole.
	ReasonUnreadable SnapshotReason = "unreadable"
	// ReasonTooLarge: the file is over the chunk cap.
	ReasonTooLarge SnapshotReason = "too-large"
)

// SnapshotFile is one entry of a snapshot's file map.
type SnapshotFile struct {
	// Path is relative to the worktree with forward slashes.
	Path string `json:"path"`
	// Chunk holds the file's content; "" when the file is deleted or
	// Reason says why it was not read.
	Chunk string `json:"chunk,omitempty"`
	// Blob is the content's git blob id.
	Blob string `json:"blob,omitempty"`
	Size int64  `json:"size"`
	// Status is git's two-letter porcelain status for the path; "" for a
	// touched path git reports clean.
	Status string `json:"status,omitempty"`
	// Deleted is set when the file did not exist at the snapshot.
	Deleted bool `json:"deleted,omitempty"`
	// Reason is set on an entry for a file that exists but whose content
	// the snapshot does not hold.
	Reason SnapshotReason `json:"reason,omitempty"`
	// HeadBlob is the path's git blob id at the snapshot's HEAD, set for a
	// path HEAD tracks, and HeadChunk that blob's content when the hook
	// could ship it.
	HeadBlob  string `json:"head_blob,omitempty"`
	HeadChunk string `json:"head_chunk,omitempty"`
}

// CommitLink records that content moondb saw became a committed blob: the
// Edit whose after-content has Blob, found in Commit at Path. Found by
// matching blob ids when a worktree's HEAD moves; never by a commit trailer.
type CommitLink struct {
	ID        string    `json:"id"`
	Repo      string    `json:"repo"`
	Worktree  string    `json:"worktree,omitempty"`
	Branch    string    `json:"branch,omitempty"`
	Commit    string    `json:"commit"`
	Parent    string    `json:"parent,omitempty"`
	Path      string    `json:"path"`
	Blob      string    `json:"blob"`
	EditID    string    `json:"edit_id"`
	TurnID    string    `json:"turn_id"`
	SessionID string    `json:"session_id"`
	At        time.Time `json:"at"`
}

// SessionDetail is a session with what it did: its summary, its turns and
// its recorded agents.
type SessionDetail struct {
	SessionSummary
	Turns []Turn `json:"turns"`
	// Agents are the session's Agent records in SortAgents order. An
	// agent known only from its edits has none.
	Agents []Agent `json:"agents"`
}

// TurnDetail is a turn with everything recorded under it.
type TurnDetail struct {
	Turn
	Edits       []Edit              `json:"edits"`
	Deltas      []UnattributedDelta `json:"deltas"`
	Snapshots   []Snapshot          `json:"snapshots"`
	CommitLinks []CommitLink        `json:"commit_links"`
}

// NeedsTotals reports whether a turn lacks totals from trace version
// version: it is closed or failed, and has none or older ones. An open
// turn gets its totals when it closes.
func NeedsTotals(t Turn, version int) bool {
	return t.Status != TurnOpen && (t.Totals == nil || t.Totals.Version < version)
}

// SortForTotals puts turns in the order TurnsWithoutTotals lists them: the
// newest session first by its start, then by session id, and a session's
// turns by Seq, so the totals backfill reads a session's lines in the
// order it wrote them. started gives a session's StartedAt.
func SortForTotals(turns []Turn, started func(sessionID string) time.Time) {
	slices.SortFunc(turns, func(a, b Turn) int {
		return cmp.Or(
			started(b.SessionID).Compare(started(a.SessionID)),
			cmp.Compare(b.SessionID, a.SessionID),
			cmp.Compare(a.Seq, b.Seq),
			cmp.Compare(a.ID, b.ID),
		)
	})
}

// SessionFilter narrows a Sessions listing. Zero values mean no filter.
type SessionFilter struct {
	// Repo keeps sessions with at least one edit in the repo.
	Repo string
	// Since keeps sessions last active at or after it
	// (SessionSummary.LastActivityAt), whenever they started.
	Since time.Time
	Limit int
}

// Keeps reports whether f's Repo and Since keep a summarized session.
// Limit is the listing's to apply.
func (f SessionFilter) Keeps(sum SessionSummary) bool {
	return !sum.LastActivityAt.Before(f.Since) &&
		(f.Repo == "" || slices.Contains(sum.Repos, f.Repo))
}

// EditQuery narrows an Edits listing. Zero values mean no filter; results
// come newest first.
type EditQuery struct {
	Repo      string
	Path      string
	Worktree  string
	SessionID string
	TurnID    string
	// AfterBlob matches edits whose after-content has this git blob id.
	AfterBlob string
	Since     time.Time
	Limit     int
}

// ChainQuery asks for the head of one edit's before-derivation chain: the
// newest other edit of Worktree and Path, made at or before At, that has
// after-content. A zero At sets no bound.
type ChainQuery struct {
	Worktree string
	Path     string
	At       time.Time
	// ExcludeID is the edit being derived, which is never its own chain:
	// a replayed envelope finds its own Edit already recorded.
	ExcludeID string
}

// SnapshotQuery asks for the newest snapshot of Worktree taken at or
// before At, other than ExcludeID. A zero At sets no bound. It names no
// path: the newest snapshot speaks for every path in the worktree, since a
// path it lacks was clean then, and an older snapshot's copy of it is
// stale.
type SnapshotQuery struct {
	Worktree string
	At       time.Time
	// ExcludeID is the snapshot being ingested, which a replayed envelope
	// finds already recorded.
	ExcludeID string
}
