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

// Skip is a matching file the corpus left out, and why.
type Skip struct {
	Hit
	Reason string `json:"reason"`
}

// Corpus is the files a question is answered over, in rank order, with the
// matching files the caps left out.
type Corpus struct {
	Files []CorpusFile `json:"files"`
	// FilesTotal counts the distinct files the selection offered: the ones
	// read plus the ones skipped.
	FilesTotal int    `json:"files_total"`
	Lines      int    `json:"lines"`
	Chars      int    `json:"chars"`
	Skipped    []Skip `json:"skipped,omitempty"`
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

// Build reads hits in order into a corpus, each file whole, until the caps
// stop it. A file that does not fit the lines left is skipped and the next
// is tried, since a smaller one may still fit; one past max_files is
// skipped unread. roots maps a repo name to its checkout; a path that
// leaves its repo is refused the way csl_read refuses it. lim must be
// resolved.
func Build(hits []Hit, roots map[string]string, lim Limits) Corpus {
	var c Corpus
	seen := map[Hit]bool{}
	for _, h := range hits {
		if seen[h] {
			continue
		}
		seen[h] = true
		c.FilesTotal++
		if len(c.Files) == lim.MaxFiles {
			c.skip(h, fmt.Sprintf("over max_files (%d)", lim.MaxFiles))
			continue
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
	return c
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
