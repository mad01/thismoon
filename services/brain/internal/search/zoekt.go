package search

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/sourcegraph/zoekt"
	"github.com/sourcegraph/zoekt/index"
	"github.com/sourcegraph/zoekt/query"
	zoektsearch "github.com/sourcegraph/zoekt/search"

	"github.com/mad01/thismoon/services/brain/internal/lifecycle"
	"github.com/mad01/thismoon/services/brain/internal/markdown"
	"github.com/mad01/thismoon/services/brain/internal/vault"
)

// Target names one vault's literal index: the zoekt repo name (the vault
// name), the vault root it reads, and the directory its shards live in.
type Target struct {
	Name     string
	Root     string
	IndexDir string
}

// IndexVault walks the target vault, indexes each markdown/MDX note into
// zoekt under the target's name, and returns after the shard is flushed to
// disk. The walk finishes before the build starts, so a vault it cannot
// walk fails the call with nothing written and the previous index still in
// place. progress (if non-nil) is called for each indexed file with its
// vault-relative path.
func IndexVault(t Target, progress func(path string)) error {
	vaultRoot, indexDir := t.Root, t.IndexDir
	if err := os.MkdirAll(indexDir, 0o755); err != nil {
		return fmt.Errorf("create index dir %s: %w", indexDir, err)
	}

	opts := index.Options{
		IndexDir:     indexDir,
		DisableCTags: true,
		RepositoryDescription: zoekt.Repository{
			Name:   t.Name,
			Source: vaultRoot,
		},
	}
	opts.SetDefaults()

	rels, err := indexablePaths(vaultRoot, int64(opts.SizeMax))
	if err != nil {
		return fmt.Errorf("walk %s: %w (the previous index is kept)", vaultRoot, err)
	}

	builder, err := index.NewBuilder(opts)
	if err != nil {
		return fmt.Errorf("create index builder for %s: %w", vaultRoot, err)
	}
	for _, rel := range rels {
		content, err := readNote(vaultRoot, rel)
		if err != nil {
			continue // best effort: skip a file that vanished or cannot be read
		}
		if err := builder.AddFile(rel, content); err != nil {
			// The failed shard build is the builder's error, so Finish
			// only removes the temporary shards and commits none.
			_ = builder.Finish()
			return fmt.Errorf("add file %s: %w", rel, err)
		}
		if progress != nil {
			progress(rel)
		}
	}

	if err := builder.Finish(); err != nil {
		return fmt.Errorf("finish index for %s: %w", vaultRoot, err)
	}
	return nil
}

// indexablePaths returns the vault-relative paths of the notes IndexVault
// indexes, in walk order: markdown and MDX files of at most sizeMax bytes,
// outside skipped and hidden directories. An entry that vanished during the
// walk is skipped; any other error reading the vault fails the walk.
func indexablePaths(vaultRoot string, sizeMax int64) ([]string, error) {
	var rels []string
	err := filepath.Walk(vaultRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			if path != vaultRoot && errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if info.IsDir() {
			base := filepath.Base(path)
			if path != vaultRoot && (skipDirs[base] || strings.HasPrefix(base, ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if !info.Mode().IsRegular() || (ext != ".md" && ext != ".mdx") || info.Size() > sizeMax {
			return nil
		}
		rel, err := filepath.Rel(vaultRoot, path)
		if err != nil {
			return err
		}
		rels = append(rels, filepath.ToSlash(rel))
		return nil
	})
	return rels, err
}

// Search queries the zoekt index and returns matches.
// Opens a new searcher per call. For reusing an existing searcher, use SearchWith.
func Search(ctx context.Context, indexDir string, opts SearchOptions) ([]Match, error) {
	searcher, err := zoektsearch.NewDirectorySearcher(indexDir)
	if err != nil {
		return nil, fmt.Errorf("open index at %s: %w (build it with: brain index)", indexDir, err)
	}
	defer searcher.Close()
	return SearchWith(ctx, searcher, opts)
}

// SearchWith queries an existing searcher and returns matches.
// Use this with a long-lived searcher to avoid per-call mmap overhead.
func SearchWith(ctx context.Context, searcher zoekt.Searcher, opts SearchOptions) ([]Match, error) {
	qStr := buildQueryString(opts)
	q, err := query.Parse(qStr)
	if err != nil {
		return nil, invalidQuery(err, opts.Pattern)
	}
	q = query.Map(q, query.ExpandFileContent)
	q = query.Simplify(q)

	limit := opts.Limit
	if limit <= 0 {
		limit = 50
	}

	sOpts := zoekt.SearchOptions{
		NumContextLines:    opts.ContextLines,
		TotalMaxMatchCount: limit * 10,
		ShardMaxMatchCount: limit * 5,
	}

	result, err := searcher.Search(ctx, q, &sOpts)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	name, root := searcherRepo(searcher)
	matches := convertResults(result.Files, limit, noteFilter(root, opts))
	if name != "" {
		for i := range matches {
			matches[i].Vault = name
		}
	}
	return matches, nil
}

// Count queries the zoekt index and returns match counts.
// Opens a new searcher per call. For reusing an existing searcher, use CountWith.
func Count(ctx context.Context, indexDir string, opts CountOptions) ([]CountResult, int, error) {
	searcher, err := zoektsearch.NewDirectorySearcher(indexDir)
	if err != nil {
		return nil, 0, fmt.Errorf(
			"open index at %s: %w (build it with: brain index)",
			indexDir,
			err,
		)
	}
	defer searcher.Close()
	return CountWith(ctx, searcher, opts)
}

// CountWith queries an existing searcher and returns match counts.
func CountWith(
	ctx context.Context,
	searcher zoekt.Searcher,
	opts CountOptions,
) ([]CountResult, int, error) {
	qStr := opts.Pattern
	if opts.Tier != "" {
		qStr += " file:^" + regexpQuote(opts.Tier.Dir()) + "/"
	}

	q, err := query.Parse(qStr)
	if err != nil {
		return nil, 0, invalidQuery(err, opts.Pattern)
	}
	q = query.Map(q, query.ExpandFileContent)
	q = query.Simplify(q)

	result, err := searcher.Search(ctx, q, &zoekt.SearchOptions{})
	if err != nil {
		return nil, 0, fmt.Errorf("search failed: %w", err)
	}

	total := 0
	groups := make(map[string]int)
	for _, f := range result.Files {
		count := len(f.LineMatches) + len(f.ChunkMatches)
		if count == 0 {
			count = 1
		}
		total += count

		switch opts.GroupBy {
		case GroupTier:
			groups[tierOfPath(f.FileName)] += count
		case GroupTag:
			_, root := searcherRepo(searcher)
			for _, tag := range tagsForPath(root, f.FileName) {
				groups[tag] += count
			}
		case "":
			// no grouping — skip
		default:
			groups["all"] += count
		}
	}

	results := make([]CountResult, 0, len(groups))
	for group, count := range groups {
		results = append(results, CountResult{Group: group, Count: count})
	}
	return results, total, nil
}

// invalidQuery wraps a zoekt parse error in ErrInvalidQuery.
func invalidQuery(err error, pattern string) error {
	return fmt.Errorf("%w: %w (check it with: brain query %q)", ErrInvalidQuery, err, pattern)
}

// ValidateQuery parses a query and returns information about it.
func ValidateQuery(pattern string) QueryInfo {
	q, err := query.Parse(pattern)
	if err != nil {
		hint := ""
		errStr := err.Error()
		if strings.Contains(errStr, "missing closing") || strings.Contains(errStr, "missing )") {
			hint = "Escape special regex characters with backslash, e.g. \"func \\(Walk\""
		} else if strings.Contains(errStr, "invalid") {
			hint = "Check your query syntax. Use 'brain search --help' for examples."
		}
		return QueryInfo{
			Valid: false,
			Error: errStr,
			Hint:  hint,
		}
	}

	q = query.Map(q, query.ExpandFileContent)
	q = query.Simplify(q)

	return QueryInfo{
		Valid:  true,
		Parsed: q.String(),
	}
}

// buildQueryString combines SearchOptions into a zoekt query string.
func buildQueryString(opts SearchOptions) string {
	parts := []string{opts.Pattern}
	if opts.Tier != "" {
		parts = append(parts, "file:^"+regexpQuote(opts.Tier.Dir())+"/")
	}
	if opts.File != "" {
		parts = append(parts, "file:"+opts.File)
	}
	if opts.Lang != "" {
		parts = append(parts, "lang:"+opts.Lang)
	}
	if opts.CaseSensitive {
		parts = append(parts, "case:yes")
	}
	return strings.Join(parts, " ")
}

// convertResults transforms zoekt FileMatches into our Match type, keeping
// at most limit files that keep accepts (every file when keep is nil). The
// filter runs before the limit, so a hidden note never takes a slot.
func convertResults(files []zoekt.FileMatch, limit int, keep func(path string) bool) []Match {
	var matches []Match
	fileCount := 0
	for _, f := range files {
		if fileCount >= limit {
			break
		}
		if keep != nil && !keep(f.FileName) {
			continue
		}
		fileCount++

		if len(f.LineMatches) > 0 {
			for _, lm := range f.LineMatches {
				col := 1
				if len(lm.LineFragments) > 0 {
					col = lm.LineFragments[0].LineOffset + 1
				}
				m := Match{
					Path:   f.FileName,
					Score:  f.Score,
					Line:   lm.LineNumber,
					Column: col,
					Text:   strings.TrimSuffix(string(lm.Line), "\n"),
				}
				if len(lm.Before) > 0 {
					m.Before = string(lm.Before)
				}
				if len(lm.After) > 0 {
					m.After = string(lm.After)
				}
				matches = append(matches, m)
			}
		} else if len(f.ChunkMatches) > 0 {
			for _, cm := range f.ChunkMatches {
				for _, r := range cm.Ranges {
					matches = append(matches, Match{
						Path:   f.FileName,
						Score:  f.Score,
						Line:   int(r.Start.LineNumber),
						Column: int(r.Start.Column),
						Text:   strings.TrimSuffix(string(cm.Content), "\n"),
					})
				}
			}
		} else {
			matches = append(matches, Match{
				Path:   f.FileName,
				Score:  f.Score,
				Line:   1,
				Column: 1,
			})
		}
	}
	return matches
}

// tierOfPath returns the tier name for a vault-relative path, or "unknown" if
// the first segment isn't a recognized tier.
func tierOfPath(relPath string) string {
	slash := strings.IndexByte(relPath, '/')
	if slash < 0 {
		return "unknown"
	}
	head := relPath[:slash]
	for _, t := range vault.AllTiers {
		if head == t.Dir() {
			return head
		}
	}
	return "unknown"
}

// tagsForPath reads the note at vault-relative path and returns its tags, or
// nil on any error.
func tagsForPath(vaultRoot, relPath string) []string {
	if vaultRoot == "" {
		return nil
	}
	data, err := readNote(vaultRoot, relPath)
	if err != nil {
		return nil
	}
	fm, _, err := markdown.ParseFrontmatter(data)
	if err != nil {
		return nil
	}
	return fm.Tags
}

// noteFilter returns the frontmatter check a hit's note must pass, or nil
// when there is none. The tag, type and status filters, and the default
// that hides revoked notes, can't be expressed in the zoekt query without
// false positives on body text that mentions the word, so each note's
// frontmatter is read to confirm. A note that cannot be read is dropped.
func noteFilter(vaultRoot string, opts SearchOptions) func(path string) bool {
	if vaultRoot == "" ||
		(opts.IncludeRevoked && opts.Tag == "" && opts.Type == "" && opts.Status == "") {
		return nil
	}
	return func(relPath string) bool {
		return noteMatchesFrontmatter(vaultRoot, relPath, opts)
	}
}

// noteMatchesFrontmatter returns true when the note's frontmatter satisfies
// every non-empty filter in opts and the note is not hidden by its status.
// The status compared is the effective one (vault.StatusOf): the stored
// value, else the default of the note's tier.
func noteMatchesFrontmatter(vaultRoot, relPath string, opts SearchOptions) bool {
	data, err := readNote(vaultRoot, relPath)
	if err != nil {
		return false
	}
	fm, _, err := markdown.ParseFrontmatter(data)
	if err != nil {
		return false
	}
	if opts.Tag != "" && !fm.HasTag(opts.Tag) {
		return false
	}
	if opts.Type != "" && !strings.EqualFold(fm.Type, opts.Type) {
		return false
	}
	status := vault.StatusOf(relPath, fm)
	if opts.Status != "" && status != lifecycle.Normalize(opts.Status) {
		return false
	}
	return !Hidden(status, opts.IncludeRevoked, opts.Status)
}

// searcherRepo returns the name and filesystem source (the vault root) of
// the one repo a vault's index holds, or empty strings when it can't be
// determined.
func searcherRepo(searcher zoekt.Searcher) (name, root string) {
	list, err := searcher.List(context.Background(), &query.Const{Value: true}, nil)
	if err != nil || list == nil || len(list.Repos) == 0 {
		return "", ""
	}
	r := list.Repos[0].Repository
	return r.Name, r.Source
}

// regexpQuote escapes regex metacharacters in s so it matches literally. Tier
// names don't contain any, but keep the helper so future additions are safe.
func regexpQuote(s string) string {
	const meta = `\.+*?()|[]{}^$`
	var b strings.Builder
	for _, r := range s {
		if strings.ContainsRune(meta, r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}
