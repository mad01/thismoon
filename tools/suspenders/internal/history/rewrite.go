package history

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"path"
	"slices"
	"sort"
	"strconv"

	"github.com/gobwas/glob"
)

// RedactedPlaceholder is what every replaced string becomes in the rewritten
// history.
const RedactedPlaceholder = "***REDACTED***"

// DefaultProtectedPaths lists file basenames that are never subject to
// string replacement. These files contain integrity checksums, version
// pins, or structured dependency data that blind substring replacement
// corrupts (e.g. a replace-table key appearing inside a base64 hash).
var DefaultProtectedPaths = []string{
	"go.sum",
	"go.mod",
	"package.json",
	"package-lock.json",
	"npm-shrinkwrap.json",
	"yarn.lock",
	"pnpm-lock.yaml",
	"Cargo.lock",
	"Cargo.toml",
	"Gemfile.lock",
	"poetry.lock",
	"Pipfile.lock",
	"composer.lock",
	"composer.json",
	"pubspec.lock",
	"pubspec.yaml",
	"Podfile.lock",
	"Package.resolved",
	"flake.lock",
}

// dataKind says how to treat the payload of the next `data <n>` block.
type dataKind int

const (
	kindOther     dataKind = iota // unknown payload: pass through untouched
	kindBlob                      // file content: replace unless binary
	kindMessage                   // commit or tag message: always replace
	kindDrop                      // stale signature: emit nothing
	kindProtected                 // file in a protected path: pass through, count skip
	kindRedact                    // file in a redacted path: whole payload becomes RedactedPlaceholder
)

// Stats reports what a Transform did or (in a dry run) would do.
type Stats struct {
	// Replacements counts occurrences per replaced string, across blobs
	// and messages. Keys are the raw strings; redact before display.
	Replacements map[string]int
	// BinaryHits counts binary blobs that contain a replacement string but
	// were left untouched.
	BinaryHits int
	// SignaturesDropped counts stale commit signatures stripped.
	SignaturesDropped int
	// ProtectedSkips counts blobs in protected files (go.sum, lockfiles,
	// etc.) that contained a replacement string but were left untouched.
	ProtectedSkips int
	// BlobsRedacted counts file blobs whose entire content was replaced
	// because their path matched RedactPaths.
	BlobsRedacted int
}

// Total returns the total number of string replacements.
func (s Stats) Total() int {
	n := 0
	for _, c := range s.Replacements {
		n += c
	}
	return n
}

// Rewriter transforms a git fast-export stream: every occurrence of a
// replacement string in blob data and commit/tag messages becomes
// RedactedPlaceholder (or its ReplaceTable target). Everything else
// round-trips byte-identical.
//
// Blobs whose file path matches ProtectedPaths are passed through untouched
// to avoid corrupting integrity checksums in lockfiles and module metadata.
// Blobs whose file path matches RedactPaths have their entire content
// replaced with RedactedPlaceholder — for files that are secrets wholesale
// (key files, token files), including binary ones. Redaction wins over
// protection when both match.
type Rewriter struct {
	Replacements   []string
	ReplaceTable   map[string]string // old -> new (specific per-string targets)
	ProtectedPaths []string          // file basenames to skip replacement on
	RedactPaths    []string          // path globs whose blob content is fully redacted

	sortedTableKeys []string        // computed once, longest-first
	protectedMarks  map[string]bool // marks referencing protected files
	redactMarks     map[string]bool // marks referencing redacted files
	redactGlobs     []glob.Glob     // compiled RedactPaths
}

// Transform streams the fast-export data from r to w. It is a pure stream
// transform; running the surrounding git processes is the caller's job.
//
// When ProtectedPaths or RedactPaths is set, the input is buffered in memory
// for a pre-scan that identifies which blob marks reference protected or
// redacted files. This is necessary because in the fast-export format,
// referenced blobs are emitted before the filemodify command that reveals
// their path.
func (rw *Rewriter) Transform(r io.Reader, w io.Writer) (Stats, error) {
	if err := rw.compileRedactGlobs(); err != nil {
		return Stats{}, err
	}
	if len(rw.ProtectedPaths) > 0 || len(rw.redactGlobs) > 0 {
		data, err := io.ReadAll(r)
		if err != nil {
			return Stats{}, fmt.Errorf("buffer fast-export stream: %w", err)
		}
		rw.scanMarks(data)
		r = bytes.NewReader(data)
	}

	stats := Stats{Replacements: make(map[string]int)}
	br := bufio.NewReaderSize(r, 64*1024)
	bw := bufio.NewWriterSize(w, 64*1024)

	kind := kindOther
	for {
		line, readErr := br.ReadBytes('\n')
		if len(line) > 0 {
			done, err := rw.transformLine(bw, br, line, &kind, &stats)
			if err != nil {
				return stats, err
			}
			if done {
				break
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return stats, fmt.Errorf("read fast-export stream: %w", readErr)
		}
	}

	if err := bw.Flush(); err != nil {
		return stats, fmt.Errorf("write fast-import stream: %w", err)
	}
	return stats, nil
}

// transformLine handles one command line of the stream. Returns done=true
// on the stream-terminating `done` command.
func (rw *Rewriter) transformLine(
	bw *bufio.Writer,
	br *bufio.Reader,
	line []byte,
	kind *dataKind,
	stats *Stats,
) (bool, error) {
	switch {
	case bytes.HasPrefix(line, []byte("data ")):
		if err := rw.transformData(bw, br, line, *kind, stats); err != nil {
			return false, err
		}
		if *kind == kindDrop {
			// A dropped signature sits between a commit's headers and its
			// message; the next data block is still that message.
			*kind = kindMessage
		} else {
			*kind = kindOther
		}
		return false, nil

	case bytes.HasPrefix(line, []byte("blob")):
		*kind = kindBlob

	case bytes.HasPrefix(line, []byte("mark ")):
		if *kind == kindBlob {
			m := string(bytes.TrimRight(line[len("mark "):], "\n"))
			switch {
			case rw.redactMarks[m]:
				*kind = kindRedact
			case rw.protectedMarks[m]:
				*kind = kindProtected
			}
		}

	case bytes.HasPrefix(line, []byte("commit ")), bytes.HasPrefix(line, []byte("tag ")):
		*kind = kindMessage

	case bytes.HasPrefix(line, []byte("gpgsig")):
		// A signature over the old content is invalid after the rewrite;
		// drop the header and its data block.
		*kind = kindDrop
		stats.SignaturesDropped++
		return false, nil

	case isInlineFileModify(line):
		*kind = kindBlob
		fields := bytes.SplitN(line, []byte(" "), 4)
		if len(fields) == 4 {
			switch {
			case rw.isRedactPath(fields[3]):
				*kind = kindRedact
			case len(rw.ProtectedPaths) > 0 && rw.isProtectedPath(fields[3]):
				*kind = kindProtected
			}
		}

	case bytes.Equal(bytes.TrimRight(line, "\n"), []byte("done")):
		_, err := bw.Write(line)
		return true, err
	}

	_, err := bw.Write(line)
	return false, err
}

// transformData consumes one `data <n>` block and emits its (possibly
// rewritten) replacement. header is the original "data <n>\n" line.
func (rw *Rewriter) transformData(
	bw *bufio.Writer,
	br *bufio.Reader,
	header []byte,
	kind dataKind,
	stats *Stats,
) error {
	arg := bytes.TrimRight(header[len("data "):], "\n")
	if bytes.HasPrefix(arg, []byte("<<")) {
		return fmt.Errorf("unsupported delimited data block in fast-export stream")
	}
	size, err := strconv.Atoi(string(arg))
	if err != nil {
		return fmt.Errorf("malformed data header %q: %w", header, err)
	}

	payload := make([]byte, size)
	if _, err := io.ReadFull(br, payload); err != nil {
		return fmt.Errorf("read %d-byte data block: %w", size, err)
	}

	if kind == kindDrop {
		return nil
	}

	out := payload
	switch kind {
	case kindBlob:
		if isBinary(payload) {
			if rw.containsAny(payload) {
				stats.BinaryHits++
			}
		} else {
			out = rw.replace(payload, stats)
		}
	case kindMessage:
		out = rw.replace(payload, stats)
	case kindProtected:
		if rw.containsAny(payload) {
			stats.ProtectedSkips++
		}
	case kindRedact:
		// The whole file is the secret; binary content is redacted too.
		out = []byte(RedactedPlaceholder + "\n")
		stats.BlobsRedacted++
	case kindOther, kindDrop:
	}

	if _, err := fmt.Fprintf(bw, "data %d\n", len(out)); err != nil {
		return err
	}
	_, err = bw.Write(out)
	return err
}

func (rw *Rewriter) tableKeys() []string {
	if rw.sortedTableKeys != nil {
		return rw.sortedTableKeys
	}
	keys := make([]string, 0, len(rw.ReplaceTable))
	for k := range rw.ReplaceTable {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return len(keys[i]) > len(keys[j])
	})
	rw.sortedTableKeys = keys
	return keys
}

func (rw *Rewriter) replace(payload []byte, stats *Stats) []byte {
	for _, k := range rw.tableKeys() {
		n := bytes.Count(payload, []byte(k))
		if n == 0 {
			continue
		}
		stats.Replacements[k] += n
		payload = bytes.ReplaceAll(payload, []byte(k), []byte(rw.ReplaceTable[k]))
	}
	for _, s := range rw.Replacements {
		n := bytes.Count(payload, []byte(s))
		if n == 0 {
			continue
		}
		stats.Replacements[s] += n
		payload = bytes.ReplaceAll(payload, []byte(s), []byte(RedactedPlaceholder))
	}
	return payload
}

func (rw *Rewriter) containsAny(payload []byte) bool {
	for _, k := range rw.tableKeys() {
		if bytes.Contains(payload, []byte(k)) {
			return true
		}
	}
	for _, s := range rw.Replacements {
		if bytes.Contains(payload, []byte(s)) {
			return true
		}
	}
	return false
}

// compileRedactGlobs compiles RedactPaths into matchers. Safe to call on
// every Transform; it rebuilds from scratch.
func (rw *Rewriter) compileRedactGlobs() error {
	rw.redactGlobs = nil
	for _, p := range rw.RedactPaths {
		g, err := glob.Compile(p, '/')
		if err != nil {
			return fmt.Errorf("invalid redact path glob %q: %w", p, err)
		}
		rw.redactGlobs = append(rw.redactGlobs, g)
	}
	return nil
}

// scanMarks pre-scans a fast-export stream to find blob marks that reference
// protected or redacted file paths, filling protectedMarks and redactMarks.
// It parses command lines only, skipping past data block payloads so blob
// content cannot false-positive.
func (rw *Rewriter) scanMarks(data []byte) {
	rw.protectedMarks = make(map[string]bool)
	rw.redactMarks = make(map[string]bool)
	br := bufio.NewReaderSize(bytes.NewReader(data), 64*1024)
	for {
		line, err := br.ReadBytes('\n')
		if len(line) > 0 {
			if bytes.HasPrefix(line, []byte("data ")) {
				arg := bytes.TrimRight(line[len("data "):], "\n")
				if !bytes.HasPrefix(arg, []byte("<<")) {
					if size, parseErr := strconv.Atoi(string(arg)); parseErr == nil && size > 0 {
						if _, discardErr := br.Discard(size); discardErr != nil {
							break
						}
					}
				}
				continue
			}
			if !bytes.HasPrefix(line, []byte("M ")) {
				continue
			}
			fields := bytes.SplitN(line, []byte(" "), 4)
			if len(fields) != 4 {
				continue
			}
			ref := fields[2]
			if len(ref) == 0 || ref[0] != ':' {
				continue
			}
			if rw.isRedactPath(fields[3]) {
				rw.redactMarks[string(ref)] = true
			}
			if rw.isProtectedPath(fields[3]) {
				rw.protectedMarks[string(ref)] = true
			}
		}
		if err != nil {
			break
		}
	}
}

// unquotePath strips the trailing newline from a fast-export file path and
// undoes git's C-style quoting of unusual path names.
func unquotePath(rawPath []byte) string {
	p := string(bytes.TrimRight(rawPath, "\n"))
	if len(p) >= 2 && p[0] == '"' {
		if unquoted, err := strconv.Unquote(p); err == nil {
			p = unquoted
		}
	}
	return p
}

// isProtectedPath checks whether the basename of a fast-export file path
// matches any entry in ProtectedPaths.
func (rw *Rewriter) isProtectedPath(rawPath []byte) bool {
	return slices.Contains(rw.ProtectedPaths, path.Base(unquotePath(rawPath)))
}

// isRedactPath checks whether a fast-export file path matches any RedactPaths
// glob. Globs match against the full repo-relative path and, so a bare file
// name like "id_rsa" also catches the file in subdirectories, against the
// basename.
func (rw *Rewriter) isRedactPath(rawPath []byte) bool {
	if len(rw.redactGlobs) == 0 {
		return false
	}
	p := unquotePath(rawPath)
	base := path.Base(p)
	for _, g := range rw.redactGlobs {
		if g.Match(p) || g.Match(base) {
			return true
		}
	}
	return false
}

// isInlineFileModify matches "M <mode> inline <path>" filemodify commands,
// whose blob data follows inline. The dataref field is checked positionally
// so a path containing "inline" cannot false-positive.
func isInlineFileModify(line []byte) bool {
	if !bytes.HasPrefix(line, []byte("M ")) {
		return false
	}
	fields := bytes.SplitN(line, []byte(" "), 4)
	return len(fields) == 4 && bytes.Equal(fields[2], []byte("inline"))
}

// isBinary reports whether content looks binary: a null byte in the first
// 512 bytes, mirroring the scanner's detection.
func isBinary(content []byte) bool {
	header := content
	if len(header) > 512 {
		header = header[:512]
	}
	return bytes.IndexByte(header, 0) >= 0
}
