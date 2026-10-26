package hint

import (
	"fmt"
	"path/filepath"
	"strings"
)

// readSizeMinLines is the file length at which a Read without a range earns
// the hint. Below it, reading the whole file is usually the right call.
// Above it, most of what lands in context is code the task did not need, and
// an outline or a ranged read gets to the same place for far fewer tokens.
const readSizeMinLines = 300

// readSizeMarkerPrefix starts the seen-file id that keeps the hint to one
// word per file per agent per session.
const readSizeMarkerPrefix = "read-size:"

// outlineExts are the file extensions csl_outline extracts symbols from:
// the languages csl maps a path to (its grammar.LangForPath) that also have
// symbol rules. Every other type, JSON, YAML, TOML, plain text, lock files,
// JavaScript and Dockerfiles among them, gets "no symbols" back, so the hint
// stays silent on it rather than hand over a call that returns nothing. A
// language csl adds later stays silent here until it is listed, which is
// the safe direction for that drift.
var outlineExts = map[string]bool{
	".go": true, ".ts": true, ".tsx": true, ".py": true, ".java": true,
	".proto": true, ".sh": true, ".bash": true, ".md": true, ".markdown": true,
}

// headingExts are the outline-able types whose symbols are headings, which
// csl_outline leaves out unless kinds names "section".
var headingExts = map[string]bool{".md": true, ".markdown": true}

// ReadSize advises csl_outline or a ranged read after a Read pulled a large
// file in without a range. Like prefer-csl it speaks after the fact: this
// file's content is already in context, so the advice is for the next read,
// and a false positive costs one ignored line (docs/adr/0008).
type ReadSize struct{}

// NewReadSize returns the read-size hint. It reads no config key besides
// enabled, which ForEvent checks before the hint runs.
func NewReadSize() *ReadSize { return &ReadSize{} }

func (h *ReadSize) ID() string    { return "read-size" }
func (h *ReadSize) Event() string { return EventRead }

// Check fires once per file per agent per session, for a Read with no offset
// and no limit of a file of readSizeMinLines or more that csl can outline,
// inside a csl-indexed repo. Every field it cannot trust (a relative path,
// an unknown line count, no session id) means silence: csl_outline needs a
// repo-relative path, and without a session id the once-per-file promise
// cannot hold.
func (h *ReadSize) Check(in Input) *Advice {
	if in.Ranged || in.TotalLines < readSizeMinLines || !filepath.IsAbs(in.FilePath) {
		return nil
	}
	file := filepath.Clean(in.FilePath)
	if !outlineExts[strings.ToLower(filepath.Ext(file))] {
		return nil
	}
	seenPath := seen.path(in.SessionID)
	marker := readSizeMarker(in.AgentID, file)
	if seenPath == "" || strings.ContainsAny(marker, "\r\n") || seen.load(seenPath)[marker] {
		return nil
	}
	repo, rel, ok := indexedLocation(file)
	if !ok {
		return nil
	}
	seen.record(seenPath, []string{marker})
	return &Advice{Hint: h.ID(), Text: readSizeAdvice(file, in.TotalLines, repo, rel)}
}

// readSizeMarker is the seen-file id for one file read by one agent. A
// subagent's tool calls carry its parent's session id, so without the agent
// id one Explore subagent reading a file whole would spend the nudge for the
// main thread and every later subagent. The id is trimmed because the seen
// store trims each line on load: an untrimmed id would never match its own
// record and the hint would repeat on every read.
func readSizeMarker(agentID, file string) string {
	return strings.TrimSpace(readSizeMarkerPrefix + agentID + ":" + file)
}

// indexedLocation resolves file to the repo name and repo-relative path
// csl_outline takes, ok only when csl has a shard for that repo. A loom
// worktree is named after its own directory, which has no shard, so it
// resolves to not indexed.
func indexedLocation(file string) (repo, rel string, ok bool) {
	root := repoRoot(file)
	if root == "" {
		return "", "", false
	}
	repo = repoName(root)
	if !indexedRepos()[strings.ToLower(repo)] {
		return "", "", false
	}
	r, err := filepath.Rel(root, file)
	if err != nil {
		return "", "", false
	}
	return repo, filepath.ToSlash(r), true
}

// readSizeAdvice is the two-line nudge: what was read, then the two cheaper
// ways in, with the outline call filled in. include_tests is always set:
// csl leaves test files out of an outline by default, and its test-file
// rules (per-language name conventions plus test directories) are csl's to
// change. Setting it costs nothing on a file that is not a test, where a
// copy of the rules here could drift and hand over a call returning nothing.
func readSizeAdvice(file string, lines int, repo, rel string) string {
	call := fmt.Sprintf("csl_outline(repo: %q, path: %q, include_tests: true", repo, rel)
	if headingExts[strings.ToLower(filepath.Ext(rel))] {
		call += `, kinds: ["section"]`
	}
	return fmt.Sprintf(
		"%s is %d lines and was read without a range.\n"+
			"For orientation use %s); for a known region use csl_read or Read with offset and limit.",
		file, lines, call,
	)
}
