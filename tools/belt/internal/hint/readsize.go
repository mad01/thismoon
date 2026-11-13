package hint

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/mad01/thismoon/tools/belt/internal/config"
	"github.com/mad01/thismoon/tools/belt/internal/cslindex"
)

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

// ReadSize advises csl_ask, csl_outline or a ranged read after a Read or a
// csl_read pulled a large file in without a range. Like prefer-csl it speaks
// after the fact: this file's content is already in context, so the advice
// is for the next read, and a false positive costs one ignored line
// (docs/adr/0008).
type ReadSize struct {
	// minLines is the file length at which a read without a range, a Read
	// or a csl_read, earns the hint: large_file_lines, shared with the
	// large-read guard. Below it, reading the whole file is usually the
	// right call. Above it, most of what lands in context is code the task
	// did not need, and an outline or a ranged read gets to the same place
	// for far fewer tokens.
	minLines int
}

// NewReadSize returns the read-size hint. Besides enabled, which ForEvent
// checks before the hint runs, it reads only the top-level
// large_file_lines.
func NewReadSize(cfg config.Config) *ReadSize {
	return &ReadSize{minLines: cfg.LargeFileThreshold()}
}

func (h *ReadSize) ID() string    { return "read-size" }
func (h *ReadSize) Event() string { return EventRead }

// Check fires once per file per agent per session, for a read with no range
// of a file of minLines or more that csl can outline, inside a
// csl-indexed repo or a linked worktree of one. A Read names the file by
// absolute path and a csl_read by repo and path as csl names them; both
// resolve to the tree and path csl knows the file by, and that pair keys
// the marker, so a Read and a csl_read of one file share one nudge. A Read
// is judged the large-read guard's way: cslindex.Resolve finds the tree,
// and a file cslindex.Skipped reports, which a csl_ask call cannot answer
// from, gets no advice. That check reads the file, so it runs only once
// the marker says the nudge is still due, and it spends no nudge. Every
// field it cannot trust (a relative path, an unknown line count, no session
// id) means silence: csl_outline needs a repo-relative path, and without a
// session id the once-per-file promise cannot hold.
func (h *ReadSize) Check(in Input) *Advice {
	if in.Ranged || in.TotalLines < h.minLines {
		return nil
	}
	file, ok := locateRead(in)
	if !ok {
		return nil
	}
	seenPath := seen.path(in.SessionID)
	marker := readSizeMarker(in.AgentID, file.markerScope(), file.loc.Rel)
	if seenPath == "" || strings.ContainsAny(marker, "\r\n") || seen.load(seenPath)[marker] {
		return nil
	}
	if file.loc.Root != "" && cslindex.Skipped(file.loc.Root, file.loc.Rel) {
		return nil
	}
	seen.record(seenPath, []string{marker})
	return &Advice{Hint: h.ID(), Text: readSizeAdvice(file, in.TotalLines)}
}

// readFile is a file a read pulled in whole: the name the advice gives it,
// how it was read, and where csl knows it. A csl_read names no checkout on
// disk, so its loc has a repo and a path and no Root.
type readFile struct {
	name, how string
	loc       cslindex.Located
}

// markerScope names the tree the file sits in for the marker: the repo for
// a main checkout, and the worktree's root for a linked worktree, whose
// file is not the main checkout's.
func (f readFile) markerScope() string {
	if f.loc.Worktree {
		return f.loc.Root
	}
	return f.loc.Repo
}

// locateRead resolves the file a read named, ok only for a type csl_outline
// has symbols for, inside a csl-indexed repo or a linked worktree of one.
// The type is checked first because it needs no filesystem access.
func locateRead(in Input) (readFile, bool) {
	if in.CSLFile.Known() {
		return locateCSLRead(in.CSLFile)
	}
	if !filepath.IsAbs(in.FilePath) {
		return readFile{}, false
	}
	file := filepath.Clean(in.FilePath)
	if !outlineable(file) {
		return readFile{}, false
	}
	loc, ok := cslindex.Resolve(file)
	return readFile{name: file, how: "without a range", loc: loc}, ok
}

// locateCSLRead resolves the file a csl_read returned. The path must stay
// inside the repo, as csl_read itself requires, and is cleaned so an
// unclean spelling of a file keys the same marker. The type check reads
// whichever of Name and Path the response set.
func locateCSLRead(f CSLFile) (readFile, bool) {
	if !outlineable(f.Name) && !outlineable(f.Path) {
		return readFile{}, false
	}
	repo, rel, ok := cslRepoPath(f, cslindex.Repos())
	if !ok || !filepath.IsLocal(rel) {
		return readFile{}, false
	}
	rel = path.Clean(rel)
	return readFile{
		name: repo + "/" + rel,
		how:  "without a range through csl_read",
		loc:  cslindex.Located{Repo: repo, Rel: rel},
	}, true
}

// outlineable reports whether csl_outline has symbols for file's type.
func outlineable(file string) bool {
	return outlineExts[strings.ToLower(filepath.Ext(file))]
}

// readSizeMarker is the seen-file id for one file read by one agent, named
// the way csl knows the file: the tree's scope (the repo, or a linked
// worktree's root) and the path below it, so a Read and a csl_read of the
// file share it. The scope is lowercased because csl matches repo names
// regardless of case, and a checkout's directory names can spell a repo
// differently from the remote csl names it after. A subagent's tool calls
// carry its parent's session id, so without the agent id one Explore
// subagent reading a file whole would spend the nudge for the main thread
// and every later subagent. The id is trimmed because the seen store trims
// each line on load: an untrimmed id would never match its own record and
// the hint would repeat on every read.
func readSizeMarker(agentID, scope, rel string) string {
	return strings.TrimSpace(
		readSizeMarkerPrefix + agentID + ":" + strings.ToLower(scope) + "/" + rel,
	)
}

// cslRepoPath resolves a csl_read's file to the repo and repo-relative path
// csl knows it by, ok only when csl has a shard for that repo. The json
// forms state both, so only the shard is checked. The text header joins
// them, so its name is split at the longest indexed repo name.
func cslRepoPath(f CSLFile, repos map[string]bool) (repo, rel string, ok bool) {
	if f.Name == "" {
		return f.Repo, f.Path, repos[strings.ToLower(f.Repo)]
	}
	return splitIndexed(f.Name, repos)
}

// splitIndexed splits a csl file name, org/repo/path, at the longest prefix
// csl has a shard for. csl names a repo after its origin remote, where a
// name can hold more than one slash (a group and a subgroup), so the name
// alone cannot say where the repo ends. ok is false when no prefix is
// indexed.
func splitIndexed(name string, repos map[string]bool) (repo, rel string, ok bool) {
	for i := strings.LastIndex(name, "/"); i > 0; i = strings.LastIndex(name[:i], "/") {
		if repos[strings.ToLower(name[:i])] {
			return name[:i], name[i+1:], true
		}
	}
	return "", "", false
}

// readSizeAdvice is the three-line nudge: what was read, then the cheaper
// ways in, each call filled in. csl_ask comes first: when only an answer is
// needed, it returns that answer and the file never enters context. The
// call is the large-read guard's (cslindex.Located.AskCall), sized for the
// file; a file past csl_ask's line ceiling gets the reason no call can
// read it and the ranged reads instead. csl_outline orients, and csl_read
// or a ranged Read fetches a region the agent will edit, which needs the
// real lines.
func readSizeAdvice(file readFile, lines int) string {
	head := fmt.Sprintf("%s is %d lines and was read %s.\n", file.name, lines, file.how)
	const ranged = "csl_read or Read with offset and limit"
	outline := outlineCall(file.loc)
	ask, ok := file.loc.AskCall(lines)
	if !ok {
		return head + cslindex.AskTooLong + "; read the region you need with " + ranged + ".\n" +
			"For orientation use " + outline + "."
	}
	return head +
		"When you only need an answer from it, use " + ask + ", which returns the answer, not the file.\n" +
		"For orientation use " + outline + "; for a region you will edit use " + ranged + "."
}

// outlineCall is the csl_outline call for the file: by repo, or by
// worktree root for a linked worktree, so the outline describes the
// worktree's files. include_tests is always set: csl leaves test files out
// of an outline by default, and its test-file rules (per-language name
// conventions plus test directories) are csl's to change. Setting it costs
// nothing on a file that is not a test, where a copy of the rules here
// could drift and hand over a call returning nothing.
func outlineCall(loc cslindex.Located) string {
	scope := fmt.Sprintf("repo: %q", loc.Repo)
	if loc.Worktree {
		scope = fmt.Sprintf("worktree: %q", loc.Root)
	}
	call := fmt.Sprintf("csl_outline(%s, path: %q, include_tests: true", scope, loc.Rel)
	if headingExts[strings.ToLower(filepath.Ext(loc.Rel))] {
		call += `, kinds: ["section"]`
	}
	return call + ")"
}
