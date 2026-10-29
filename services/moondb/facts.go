// Package moondb holds the mechanical facts about the moondb service that
// its operating doc, MCP instructions, error hints and CLI defaults share.
// Every user-facing default is a constant here so the help text, the docs
// and the running binary cannot disagree (docs/adr/0011).
package moondb

import (
	"fmt"
	"time"

	"github.com/mad01/thismoon/kit/agentdoc"
)

// DefaultPort is the port `moondb serve` listens on when MOONDB_PORT and
// --port are both unset. The server binds to loopback only.
const DefaultPort = 7433

// DefaultWorkdir is the state directory when MOONDB_WORKDIR is unset: the
// JSONL log, the chunk objects and the spool live under it. The leading ~ is
// expanded at runtime by the CLI, never at build time.
const DefaultWorkdir = "~/.local/state/moondb"

// Capture limits. They bound what one hook invocation reads, hashes and
// writes, so a tool call on a huge file costs a skipped chunk and never a
// stalled session.
const (
	// MaxChunkBytes is the largest file content a hook ships as a chunk.
	// Larger files are recorded by path and patch only.
	MaxChunkBytes = 1 << 20
	// MaxTranscriptBytes caps the transcript tail stored per turn.
	MaxTranscriptBytes = 4 << 20
	// RecordBudget is how long `moondb record` works before it writes what
	// it has and returns. The Edit/Write hook is synchronous, so this is
	// latency the session feels on every edit.
	RecordBudget = 200 * time.Millisecond
	// BashBudget is how long `moondb record` works on a Bash payload. The
	// Bash hook is async, so no session waits on it, and one command can
	// change hundreds of files: a status check, a placement and a disk
	// read each need more time than RecordBudget gives the Edit hook. It
	// stays under the ceiling the CLI puts on every record run. The files
	// the budget does not reach are skipped: counted on the turn as
	// timeout, with no edit.
	BashBudget = 2 * time.Second
	// GitBudget bounds every git call a hook makes.
	GitBudget = 1 * time.Second
	// SnapshotBudget is how long the turn-edge hooks (UserPromptSubmit and
	// Stop, both async) spend on snapshots of every worktree in scope. A
	// snapshot cut short is written with what it read and marked
	// truncated.
	SnapshotBudget = 2 * time.Second
	// MaxSnapshotFiles caps the paths one snapshot lists; the rest are
	// counted as skipped. The session's touched paths come first.
	MaxSnapshotFiles = 1000
	// MaxStatusBytes caps the `git status` output a snapshot reads. A
	// worktree whose status is longer gets no snapshot.
	MaxStatusBytes = 4 << 20
)

// Agent transcripts. The SubagentStop hook and `moondb import` read one
// subagent's transcript file and keep its timeline lines (user, assistant,
// API request and system lines) with the fields a trace reads. Each text,
// thinking, tool input and tool result is cut to a short head, so a
// transcript of many megabytes keeps its usage, ids and times in a chunk a
// fraction of its size.
const (
	// AgentHeadBytes is how much of each text, thinking block, tool input
	// and tool result an agent transcript keeps. The rest is replaced by a
	// count of the bytes cut.
	AgentHeadBytes = 240
	// MaxAgentTranscriptBytes caps an agent transcript after the cut to
	// heads. A longer one loses whole lines from its head, counted on the
	// Agent record, and the model calls in them add no tokens or cost, as
	// for a turn's transcript (MaxTranscriptBytes).
	MaxAgentTranscriptBytes = 8 << 20
	// AgentBudget is how long `moondb record` works on a SubagentStop
	// payload. The hook is async, so no session waits on it, and reading
	// and cutting a large transcript takes longer than RecordBudget. It
	// stays under the ceiling the CLI puts on every record run. A file the
	// budget does not reach the end of is recorded up to where it got and
	// marked partial.
	AgentBudget = 2 * time.Second
)

// Import defaults: where `moondb import` finds the harness's transcripts and
// its file history, and how far back it reads. The leading ~ is expanded at
// runtime.
const (
	DefaultImportSince    = "30d"
	DefaultProjectsDir    = "~/.claude/projects"
	DefaultFileHistoryDir = "~/.claude/file-history"
)

// SessionTagsEnv is the variable a launcher sets to tag the sessions it
// starts, such as an eval runner tagging its runs "eval". The hooks inherit
// it, and `moondb record` keeps its comma-separated tags on the session
// (store.ParseTags says which ones count).
const SessionTagsEnv = "MOONDB_SESSION_TAGS"

// DefaultClaudeSettings is the Claude Code settings file `moondb doctor`
// reads the hook wiring from, unless the config's claude_settings names
// another. The leading ~ is expanded at runtime.
const DefaultClaudeSettings = "~/.claude/settings.json"

// Store limits.
const (
	// SegmentBytes is the size at which the JSONL log rolls to a new
	// segment file.
	SegmentBytes = 64 << 20
)

// Shared instance. `moondb serve --shared` serves the sessions of several
// machines; with `--backend cloud` its chunks live in a Cloud Storage
// bucket and a copy of its log sits beside them.
const (
	// DefaultBackend is the store backend when --backend is unset: the log
	// and the chunks on local disk. A shared instance may pick cloud.
	DefaultBackend = "local"
	// SegmentCopyEvery is how often the cloud backend copies the open log
	// segment to the bucket. A closed segment is copied once the log has
	// moved past it, and the open one once more when the store closes.
	SegmentCopyEvery = time.Hour
	// SegmentPollEvery is how often the cloud backend looks for closed
	// segments the bucket does not hold yet.
	SegmentPollEvery = time.Minute
	// FinalCopyTimeout bounds the copy of the log the cloud backend makes
	// when the store closes.
	FinalCopyTimeout = 20 * time.Second
	// MaxSyncBodyBytes caps the body of one request to /api/sync. A batch
	// of records or chunks larger than this is refused with 413.
	MaxSyncBodyBytes = 64 << 20
	// KeyCookie is the cookie the web page of a shared instance keeps its
	// key in, so links to chunks and scorecards open with it. API clients
	// send the key as a bearer token instead.
	KeyCookie = "moondb_key"
	// SyncParallelism is how many chunk lookups or writes one sync request
	// runs at once: on the cloud backend each is a bucket request.
	SyncParallelism = 16
)

// Replication. A local serve given --replicate-to copies its log and the
// chunks the records point at to a shared instance.
const (
	// ReplicateEvery is how often the replicate loop looks for new records
	// once it has sent everything.
	ReplicateEvery = 5 * time.Second
	// ReplicateMaxBackoff caps the wait after a failed round; the wait
	// doubles from ReplicateEvery up to it.
	ReplicateMaxBackoff = 5 * time.Minute
	// ReplicateRecordBytes caps the record lines one sync request carries.
	// The chunk keys they point at are asked about in one request, so it
	// also bounds that request.
	ReplicateRecordBytes = 1 << 20
	// ReplicateChunkBytes caps the chunk bytes one sync request carries;
	// a larger chunk travels alone.
	ReplicateChunkBytes = 8 << 20
	// ReplicateTimeout bounds one sync request.
	ReplicateTimeout = 2 * time.Minute
	// ReplicateCursorFile is the file under the workdir that holds how far
	// the log has been sent, and to which instance.
	ReplicateCursorFile = "replicate.json"
)

// Trace limits. A trace is computed on read from a turn's transcript chunk
// and kept in memory, so these bound what serve holds and what a summary
// lists.
const (
	// TraceCacheTurns is how many turn traces serve keeps in memory, the
	// least recently read dropped first.
	TraceCacheTurns = 512
	// TraceTopSpans is how many spans `moondb trace` and the moondb_trace
	// tool list per ranking: the longest tool calls, the most expensive
	// model calls.
	TraceTopSpans = 10
)

// IdleGapAfter is the longest stretch inside a turn that a trace gives to
// model, tool or harness time. A trace classes a stretch by the line that
// ends it, but no line is written while a laptop sleeps, a session waits
// days to be resumed, or a request hangs. So a longer stretch keeps its
// first IdleGapAfter in its own class and the rest is idle time, which a
// session's active time leaves out. Waiting for input is user time and is
// never idle. Two texts state 30 minutes as a literal and change with it:
// IDLE_RULE in internal/server/app.js and the idle paragraph of
// operating.md.
const IdleGapAfter = 30 * time.Minute

// Scorecard peers. A session's scorecard joins one loopback service: the
// events log, for the belt hints a session was shown and its belt ledger
// entries. EVENTS_BASE_URL overrides the default, as it does for belt. It
// is not required: a peer that does not answer leaves its part of the
// scorecard null and says so in its sources.
const (
	// DefaultEventsURL is where the events service listens.
	DefaultEventsURL = "http://127.0.0.1:7430"
	// PeerTimeout bounds one request to a peer.
	PeerTimeout = 2 * time.Second
	// EventsPerSource is how many entries the events service keeps per
	// source, and how many a scorecard asks for. An older entry is gone.
	EventsPerSource = 500
)

// Scorecard thresholds. A scorecard flags a pattern when its count reaches
// the threshold; the flags name the costliest patterns a session showed.
const (
	// LargeReadLines is the line count from which a Read of a whole file
	// counts as large: at least this many lines.
	LargeReadLines = 300
	// FlagLargeReads flags the large reads of a whole file.
	FlagLargeReads = 1
	// FlagZeroSearches flags the csl searches that found nothing.
	FlagZeroSearches = 3
	// FlagTruncatedSearches flags the csl searches that hit their limit.
	FlagTruncatedSearches = 3
	// FlagFailedSearches flags the csl searches that failed, a bad query
	// among them.
	FlagFailedSearches = 2
	// FlagToolErrors flags the tool calls that failed, every agent's
	// included.
	FlagToolErrors = 5
	// FlagCompactions flags the context compactions (belt ledger
	// pre-compact entries).
	FlagCompactions = 1
	// FlagStopFailures flags the turns that ended on an API error (belt
	// ledger stop-failure entries).
	FlagStopFailures = 1
	// FlagPermissionDenials flags the tool calls a permission check denied
	// (belt ledger permission-denied entries).
	FlagPermissionDenials = 1
	// ScorecardTopTools is how many tools and agents the text rendering of
	// `moondb scorecard` lists; the JSON lists every one.
	ScorecardTopTools = 10
)

// Turn totals. serve stores each turn's trace totals when the turn closes;
// a turn stored before that, or by an older trace build, gets them from the
// totals backfill, which runs while the spool is empty, a batch per tick,
// newest session first and a session's turns in order. A batch reads one
// transcript chunk per turn, so it stays small to keep the next envelope
// waiting at most a moment.
const (
	// TotalsBackfillTurns is how many turns one totals backfill batch
	// tries, computed or not, and how many sessions it fills a missing
	// prompt line for.
	TotalsBackfillTurns = 20
	// TotalsBackfillEvery is how often the totals backfill runs a batch.
	TotalsBackfillEvery = time.Second
)

// Session listing. The listing reads records only, never a chunk, so the
// fields a row shows are kept on the records or counted from them.
const (
	// SessionIdleAfter is how long a session goes without a recorded turn
	// start, turn end, edit or agent stop before the listing calls it
	// ended rather than running.
	SessionIdleAfter = 10 * time.Minute
	// OpenTurnStaleAfter is how long an open last turn keeps a session
	// running without other activity. The harness sends no Stop for a turn
	// the person interrupted, so such a turn stays open for good.
	OpenTurnStaleAfter = 12 * time.Hour
	// SessionPromptLineRunes caps a prompt line: the one a session keeps
	// for its title and the one a trace shows per turn.
	SessionPromptLineRunes = 100
)

// In-flight files: what `GET /api/inflight` answers about one checkout,
// the files other sessions edited that no commit has taken yet.
const (
	// InflightWindow is how far back the query reads edits when the
	// request names no since. An older edit is never reported, committed
	// or not.
	InflightWindow = 72 * time.Hour
	// InflightLimit caps the files the query returns when the request
	// names no limit.
	InflightLimit = 20
)

// Prompt caching bills a cache write and a cache read as a multiple of the
// model's input rate (the claude-api skill, shared/prompt-caching.md,
// Economics). A model whose cache read rate differs names it in its row.
const (
	CacheWrite5mMultiplier = 1.25
	CacheWrite1hMultiplier = 2
	CacheReadMultiplier    = 0.1
)

// USOnlyMultiplier is what US-only inference (usage inference_geo "us")
// costs on Claude 4.6 and later models: every token category, cache writes
// and reads and the long-prompt rates included, at 1.1 times the price.
// Earlier models do not take the parameter and bill the standard price.
const USOnlyMultiplier = 1.1

// Rates are USD per million tokens of each kind a model call bills.
type Rates struct {
	Input        float64
	Output       float64
	CacheWrite5m float64
	CacheWrite1h float64
	CacheRead    float64
}

// Price is what one model charges. Rates apply to every call unless one of
// the modifiers below says otherwise; the two multipliers stack.
type Price struct {
	Rates
	// LongPromptTokens, when set, is the prompt size (input plus cache
	// writes and reads) above which a call bills at Long instead.
	LongPromptTokens int64
	Long             Rates
	// FastMultiplier scales every rate of a call made at speed "fast"; 0
	// when the model has no published fast-mode price, so such a call has
	// no cost.
	FastMultiplier float64
	// USMultiplier scales every rate of a call whose usage reports
	// inference_geo "us"; 0 for a model without a US-only premium, which
	// bills such a call at its rates.
	USMultiplier float64
}

// RatesFrom builds a model's rates from its input and output price and its
// cache read price, the cache writes at the standard multipliers.
func RatesFrom(input, output, cacheRead float64) Rates {
	return Rates{
		Input:        input,
		Output:       output,
		CacheWrite5m: input * CacheWrite5mMultiplier,
		CacheWrite1h: input * CacheWrite1hMultiplier,
		CacheRead:    cacheRead,
	}
}

// standardRates is RatesFrom with the cache read at the standard multiple
// of input.
func standardRates(input, output float64) Rates {
	return RatesFrom(input, output, input*CacheReadMultiplier)
}

// DefaultPrices is the Claude API list price per model id, USD per million
// tokens, from the pricing page the claude-api skill names as its pricing
// source (platform.claude.com/docs/en/about-claude/pricing, read
// 2026-10-25). Cache reads cost 0.1x input except on Fable 5.1 and Mythos
// 5.1 (0.025x, $0.25) and on Opus 5.5 ($0.20) and Sonnet 5.5 ($0.10), both
// 0.05x. Haiku 5.5 bills a second tier above 100,000 prompt tokens. Fast
// mode costs 2x on Opus 5.5 ($8 in, $40 out), Opus 5 and Opus 4.8 ($10 in,
// $50 out), cache multipliers on top. US-only inference costs
// USOnlyMultiplier more on the rows of Claude 4.6 and later.
//
// A trace matches a model id exactly first, then by family: the longest id
// here that the model id starts with, followed by a dated snapshot
// ("-20251101"), a provider version ("@...") or a context marker ("[1m]").
// The config file's [prices."<model id>"] tables override or extend it.
//
// A change to a LongPromptTokens threshold here needs a trace.TotalsVersion
// bump: a stored turn's usage keeps the side of the threshold it was
// computed on, and only the totals backfill computes it again.
var DefaultPrices = map[string]Price{
	"claude-fable-5-1": {
		Rates: RatesFrom(10, 50, 0.25), USMultiplier: USOnlyMultiplier,
	},
	"claude-mythos-5-1": {
		Rates: RatesFrom(10, 50, 0.25), USMultiplier: USOnlyMultiplier,
	},
	"claude-fable-5":  {Rates: standardRates(10, 50), USMultiplier: USOnlyMultiplier},
	"claude-mythos-5": {Rates: standardRates(10, 50), USMultiplier: USOnlyMultiplier},
	"claude-opus-5-5": {
		Rates: RatesFrom(4, 20, 0.20), FastMultiplier: 2, USMultiplier: USOnlyMultiplier,
	},
	"claude-opus-5": {
		Rates: standardRates(5, 25), FastMultiplier: 2, USMultiplier: USOnlyMultiplier,
	},
	"claude-opus-4-8": {
		Rates: standardRates(5, 25), FastMultiplier: 2, USMultiplier: USOnlyMultiplier,
	},
	"claude-opus-4-7": {Rates: standardRates(5, 25), USMultiplier: USOnlyMultiplier},
	"claude-opus-4-6": {Rates: standardRates(5, 25), USMultiplier: USOnlyMultiplier},
	"claude-opus-4-5": {Rates: standardRates(5, 25)},
	"claude-opus-4-1": {Rates: standardRates(15, 75)},
	"claude-opus-4-0": {Rates: standardRates(15, 75)},
	"claude-sonnet-5-5": {
		Rates: RatesFrom(2, 10, 0.10), USMultiplier: USOnlyMultiplier,
	},
	"claude-sonnet-5":   {Rates: standardRates(2, 10), USMultiplier: USOnlyMultiplier},
	"claude-sonnet-4-6": {Rates: standardRates(3, 15), USMultiplier: USOnlyMultiplier},
	"claude-sonnet-4-5": {Rates: standardRates(3, 15)},
	"claude-sonnet-4-0": {Rates: standardRates(3, 15)},
	"claude-haiku-5-5": {
		Rates:            standardRates(0.10, 0.50),
		LongPromptTokens: 100_000,
		Long:             standardRates(0.50, 2.50),
		USMultiplier:     USOnlyMultiplier,
	},
	"claude-haiku-4-5": {Rates: standardRates(1, 5)},
	// The dated ids of the models whose alias is not a prefix of them.
	"claude-opus-4-20250514":    {Rates: standardRates(15, 75)},
	"claude-sonnet-4-20250514":  {Rates: standardRates(3, 15)},
	"claude-3-5-haiku-20241022": {Rates: standardRates(0.80, 4)},
}

// Workdir layout: the subdirectories of the state directory. record writes
// spool and sessions; serve owns log and objects; record reads objects only
// to skip chunks the store already has.
const (
	LogDirName      = "log"
	ObjectsDirName  = "objects"
	SpoolDirName    = "spool"
	SessionsDirName = "sessions"
)

// DefaultExcludes are the path globs whose contents are never captured;
// the edit is recorded by path only. Matched against the base name and the
// worktree-relative path.
var DefaultExcludes = []string{
	".env", ".env.*", "*.pem", "*.key", "id_*", "*.p12",
	"*.tfvars", "*.tfstate", "credentials*.json", "kubeconfig", ".npmrc",
}

// Facts returns the mechanical facts rendered into the operating doc, the
// MCP instructions block, and error hints.
func Facts() agentdoc.Facts {
	return agentdoc.Facts{
		Name:          "moondb",
		Bin:           "moondb",
		Purpose:       "edit history between commits, linked to the agent turn that made it",
		BaseURL:       fmt.Sprintf("http://localhost:%d", DefaultPort),
		StorePath:     DefaultWorkdir,
		HasDoctor:     true,
		MCPDoctorTool: "moondb_doctor",
	}
}
