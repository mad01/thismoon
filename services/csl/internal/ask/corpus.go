package ask

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/mad01/thismoon/services/csl/internal/repo/repopath"
)

// The corpus caps: what a call gets when it names none, and the most it may
// ask for. The defaults keep a call near the size the eval measured; the
// ceilings keep one call inside a worker's context window.
const (
	DefaultMaxFiles = 20
	MaxFilesCeiling = 100
	DefaultMaxLines = 6000
	MaxLinesCeiling = 20000
)

// maxFileBytes refuses a file too large to be source a worker should read
// whole, before it is loaded: a 4 MiB file holds far more lines than any
// line budget allows, so reading it would only fill memory.
const maxFileBytes = 4 << 20

// maxSkipped stops a corpus that keeps refusing files. Each refusal reads a
// file, and a line budget with a few lines left refuses nearly every file
// after it, so without a stop a long selection would be read whole only to
// list it as skipped. Past this many skips the rest are not considered and
// the corpus is marked truncated.
const maxSkipped = 50

// Hit is one file a search selected: the repo name as discovery reports it
// (org/repo) and a path relative to that repo's root.
type Hit struct {
	Repo string `json:"repo"`
	Path string `json:"path"`
}

// Name is the hit as csl prints a file and as the worker cites it:
// org/repo/path.
func (h Hit) Name() string { return h.Repo + "/" + h.Path }

// Limits caps a corpus. A zero field takes its default.
type Limits struct {
	MaxFiles int
	MaxLines int
}

// Resolve applies the defaults and refuses a value below zero or over its
// ceiling, so a caller asking for more than a call allows hears so instead
// of getting less than it asked for.
func (l Limits) Resolve() (Limits, error) {
	files, err := resolveLimit("max_files", l.MaxFiles, DefaultMaxFiles, MaxFilesCeiling)
	if err != nil {
		return Limits{}, err
	}
	lines, err := resolveLimit("max_lines", l.MaxLines, DefaultMaxLines, MaxLinesCeiling)
	if err != nil {
		return Limits{}, err
	}
	return Limits{MaxFiles: files, MaxLines: lines}, nil
}

func resolveLimit(name string, v, def, ceiling int) (int, error) {
	switch {
	case v == 0:
		return def, nil
	case v < 0 || v > ceiling:
		return 0, fmt.Errorf(
			"%s must be between 1 and %d (default %d), got %d",
			name, ceiling, def, v,
		)
	}
	return v, nil
}

// CorpusFile is one file the worker reads. The text stays out of the JSON:
// keeping it out of the caller's context is the point of the tool.
type CorpusFile struct {
	Hit
	Lines int `json:"lines"`

	text  []string
	bytes int
}

// Skip is a matching file the corpus considered and left out, and why.
type Skip struct {
	Hit
	Reason string `json:"reason"`
}

// Corpus is the files a question is answered over, in rank order, and how
// much of the selection they cover.
type Corpus struct {
	Files []CorpusFile `json:"files"`
	// Matched counts the distinct files the selection offered.
	Matched int `json:"matched"`
	// Read counts the files in the worker's message.
	Read  int `json:"read"`
	Lines int `json:"lines"`
	Chars int `json:"chars"`
	// Skipped lists the files considered and left out, each with a reason.
	Skipped []Skip `json:"skipped,omitempty"`
	// Truncated is set when matching files were never considered: the caps
	// were reached before them, or a query's search stopped at its limit.
	Truncated bool `json:"truncated"`
}

// Union merges ranked hit lists into one: every list's first hit, then
// every list's second, and so on, keeping the first sighting of each file.
// Taking turns keeps one query's tail from filling the cap before another
// query's best match is in.
func Union(lists [][]Hit) []Hit {
	var out []Hit
	seen := map[Hit]bool{}
	for rank := 0; ; rank++ {
		more := false
		for _, list := range lists {
			if rank >= len(list) {
				continue
			}
			more = true
			if h := list[rank]; !seen[h] {
				seen[h] = true
				out = append(out, h)
			}
		}
		if !more {
			return out
		}
	}
}

// Build reads hits in rank order into a corpus, each file whole, until
// max_files files are in or the line budget is spent. A file that cannot go
// in (binary, missing, outside its repo, or longer than the lines left) is
// skipped with its reason and the next is tried, so a later file fills its
// place. Once a cap is reached, or maxSkipped files were refused, nothing
// more is read: the files left are counted in Matched and mark the corpus
// truncated. A repeated hit counts once. roots maps a repo name to its
// checkout; a path that leaves its repo is refused the way csl_read refuses
// it. lim must be resolved.
func Build(hits []Hit, roots map[string]string, lim Limits) Corpus {
	hits = distinct(hits)
	c := Corpus{Matched: len(hits)}
	for _, h := range hits {
		if len(c.Files) == lim.MaxFiles || c.Lines >= lim.MaxLines ||
			len(c.Skipped) == maxSkipped {
			break
		}
		f, err := readCorpusFile(roots, h)
		if err != nil {
			c.skip(h, err.Error())
			continue
		}
		if left := lim.MaxLines - c.Lines; f.Lines > left {
			c.skip(h, fmt.Sprintf(
				"%d lines, %d left under max_lines (%d)", f.Lines, left, lim.MaxLines,
			))
			continue
		}
		c.Files = append(c.Files, f)
		c.Lines += f.Lines
		c.Chars += f.bytes
	}
	c.Read = len(c.Files)
	c.Truncated = c.Matched > c.Read+len(c.Skipped)
	return c
}

// distinct drops repeated hits, keeping each one's first place.
func distinct(hits []Hit) []Hit {
	out := make([]Hit, 0, len(hits))
	seen := make(map[Hit]bool, len(hits))
	for _, h := range hits {
		if !seen[h] {
			seen[h] = true
			out = append(out, h)
		}
	}
	return out
}

func (c *Corpus) skip(h Hit, reason string) {
	c.Skipped = append(c.Skipped, Skip{Hit: h, Reason: reason})
}

// workerFiles is the corpus as the worker reads it, each file named
// org/repo/path so an answer's citation is a path csl_read resolves.
func workerFiles(files []CorpusFile) []File {
	out := make([]File, len(files))
	for i, f := range files {
		out[i] = File{Path: f.Name(), Lines: f.text}
	}
	return out
}

var errBinary = errors.New("binary file")

// readCorpusFile reads one hit whole from its repo's checkout.
func readCorpusFile(roots map[string]string, h Hit) (CorpusFile, error) {
	root, ok := roots[h.Repo]
	if !ok {
		return CorpusFile{}, fmt.Errorf("repo %s is not among the discovered repos", h.Repo)
	}
	path, err := repopath.Resolve(root, "path", h.Path)
	if err != nil {
		return CorpusFile{}, err
	}
	info, err := os.Stat(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return CorpusFile{}, errors.New("not in the checkout")
	case err != nil:
		return CorpusFile{}, err
	case info.IsDir():
		return CorpusFile{}, fmt.Errorf("%s is a directory", h.Path)
	case info.Size() > maxFileBytes:
		return CorpusFile{}, fmt.Errorf(
			"%d bytes, over the %d-byte file cap",
			info.Size(),
			maxFileBytes,
		)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return CorpusFile{}, err
	}
	if bytes.IndexByte(data, 0) >= 0 {
		return CorpusFile{}, errBinary
	}
	lines := SplitLines(data)
	return CorpusFile{Hit: h, Lines: len(lines), text: lines, bytes: len(data)}, nil
}
