package hint

import (
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

// readSizeMinLines is the file length at which a read without a range, a
// Read or a csl_read, earns the hint. Below it, reading the whole file is
// usually the right call. Above it, most of what lands in context is code
// the task did not need, and an outline or a ranged read gets to the same
// place for far fewer tokens.
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

// ReadSize advises csl_ask, csl_outline or a ranged read after a Read or a
// csl_read pulled a large file in without a range. Like prefer-csl it speaks
// after the fact: this file's content is already in context, so the advice
// is for the next read, and a false positive costs one ignored line
// (docs/adr/0008).
type ReadSize struct{}

// NewReadSize returns the read-size hint. It reads no config key besides
// enabled, which ForEvent checks before the hint runs.
func NewReadSize() *ReadSize { return &ReadSize{} }

func (h *ReadSize) ID() string    { return "read-size" }
func (h *ReadSize) Event() string { return EventRead }

// Check fires once per file per agent per session, for a read with no range
// of a file of readSizeMinLines or more that csl can outline, inside a
// csl-indexed repo. A Read names the file by absolute path and a csl_read by
// repo and path as csl names them; both resolve to the repo and path csl knows
// the file by, and that pair keys the marker, so a Read and a csl_read of
// one file share one nudge. Every field it cannot trust (a relative path, an
// unknown line count, no session id) means silence: csl_outline needs a
// repo-relative path, and without a session id the once-per-file promise
// cannot hold.
func (h *ReadSize) Check(in Input) *Advice {
	if in.Ranged || in.TotalLines < readSizeMinLines {
		return nil
	}
	file, ok := locateRead(in)
	if !ok {
		return nil
	}
	seenPath := seen.path(in.SessionID)
	marker := readSizeMarker(in.AgentID, file.repo, file.rel)
	if seenPath == "" || strings.ContainsAny(marker, "\r\n") || seen.load(seenPath)[marker] {
		return nil
	}
	seen.record(seenPath, []string{marker})
	return &Advice{Hint: h.ID(), Text: readSizeAdvice(file, in.TotalLines)}
}

// readFile is a file a read pulled in whole: the name the advice gives it,
// how it was read, and the repo and repo-relative path csl knows it by.
type readFile struct {
	name, how string
	repo, rel string
}

// locateRead resolves the file a read named, ok only for a type csl_outline
// has symbols for, inside a csl-indexed repo. The type is checked first
// because it needs no filesystem access.
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
	repo, rel, ok := indexedLocation(file)
	return readFile{name: file, how: "without a range", repo: repo, rel: rel}, ok
}

// locateCSLRead resolves the file a csl_read returned. The path must stay
// inside the repo, as csl_read itself requires, and is cleaned so an
// unclean spelling of a file keys the same marker. The type check reads
// whichever of Name and Path the response set.
func locateCSLRead(f CSLFile) (readFile, bool) {
	if !outlineable(f.Name) && !outlineable(f.Path) {
		return readFile{}, false
	}
	repo, rel, ok := cslRepoPath(f, indexedRepos())
	if !ok || !filepath.IsLocal(rel) {
		return readFile{}, false
	}
	rel = path.Clean(rel)
	return readFile{
		name: repo + "/" + rel,
		how:  "without a range through csl_read",
		repo: repo,
		rel:  rel,
	}, true
}

// outlineable reports whether csl_outline has symbols for file's type.
func outlineable(file string) bool {
	return outlineExts[strings.ToLower(filepath.Ext(file))]
}

// readSizeMarker is the seen-file id for one file read by one agent, named
// the way csl knows the file: repo and repo-relative path, so a Read and a
// csl_read of the file share it. The repo is lowercased because csl matches
// repo names regardless of case, and a checkout's directory names can spell
// a repo differently from the remote csl names it after. A subagent's tool
// calls carry its parent's session id, so without the agent id one Explore
// subagent reading a file whole would spend the nudge for the main thread
// and every later subagent. The id is trimmed because the seen store trims
// each line on load: an untrimmed id would never match its own record and
// the hint would repeat on every read.
func readSizeMarker(agentID, repo, rel string) string {
	return strings.TrimSpace(
		readSizeMarkerPrefix + agentID + ":" + strings.ToLower(repo) + "/" + rel,
	)
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
// needed, it returns that answer and the file never enters context.
// csl_outline orients, and csl_read or a ranged Read fetches a region the
// agent will edit, which needs the real lines. include_tests is always set:
// csl leaves test files out of an outline by default, and its test-file
// rules (per-language name conventions plus test directories) are csl's to
// change. Setting it costs nothing on a file that is not a test, where a
// copy of the rules here could drift and hand over a call returning nothing.
func readSizeAdvice(file readFile, lines int) string {
	outline := fmt.Sprintf(
		"csl_outline(repo: %q, path: %q, include_tests: true", file.repo, file.rel,
	)
	if headingExts[strings.ToLower(filepath.Ext(file.rel))] {
		outline += `, kinds: ["section"]`
	}
	return fmt.Sprintf(
		"%s is %d lines and was read %s.\n"+
			"When you only need an answer from it, use %s, which returns the answer, not the file.\n"+
			"For orientation use %s); for a region you will edit use csl_read or Read with offset and limit.",
		file.name, lines, file.how, askCall(file.repo, file.rel), outline,
	)
}

// askCall is the csl_ask call scoped to the one file just read. zoekt reads
// f: as a regex over the repo-relative path, and csl reads repo as a regex
// over repo names, so both are escaped and anchored at both ends: unanchored,
// the path also matches a same-named file deeper in the tree and the repo
// also matches every repo its name prefixes. zoektTerm quotes a path holding
// a space as one term.
func askCall(repo, rel string) string {
	query := "f:" + zoektTerm("^"+regexp.QuoteMeta(rel)+"$")
	repoRE := "^" + regexp.QuoteMeta(repo) + "$"
	return fmt.Sprintf(
		`csl_ask(question: "<what you need from it>", queries: [%q], repo: %q)`,
		query, repoRE,
	)
}
