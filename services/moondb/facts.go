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

// Import defaults: where `moondb import` finds the harness's transcripts and
// its file history, and how far back it reads. The leading ~ is expanded at
// runtime.
const (
	DefaultImportSince    = "30d"
	DefaultProjectsDir    = "~/.claude/projects"
	DefaultFileHistoryDir = "~/.claude/file-history"
)

// Store limits.
const (
	// SegmentBytes is the size at which the JSONL log rolls to a new
	// segment file.
	SegmentBytes = 64 << 20
)

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
