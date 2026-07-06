package web

import (
	"encoding/json"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/mad01/thismoon/services/csl/internal/repo/finder"
	"github.com/mad01/thismoon/services/csl/internal/search"
)

const (
	defaultLimit   = 50
	maxLimit       = 500
	maxContext     = 20
	maxQueryLength = 1000
)

// matchJSON is one search hit, with a link to the line on the git host.
type matchJSON struct {
	Line      int    `json:"line"`
	Column    int    `json:"column"`
	Text      string `json:"text"`
	Before    string `json:"before,omitempty"`
	After     string `json:"after,omitempty"`
	RemoteURL string `json:"remoteURL,omitempty"`
}

// fileGroup groups hits within a single file.
type fileGroup struct {
	File string `json:"file"`
	// LocalPath is the absolute on-disk path to the file, with the user's home
	// directory collapsed to "~" (see collapseHome). It powers the "copy local
	// path" action in the UI. Empty when the repo has no known local root.
	LocalPath string      `json:"localPath,omitempty"`
	FileURL   string      `json:"fileURL,omitempty"`
	Matches   []matchJSON `json:"matches"`
}

// repoGroup groups files within a single repo.
type repoGroup struct {
	Repo  string      `json:"repo"`
	Host  string      `json:"host"`
	Files []fileGroup `json:"files"`
}

// facetJSON is one quick-filter entry computed from the matched files, so the
// UI can offer one-click narrowing by file type (GitHub-style).
type facetJSON struct {
	// Label is the display name: a language name for known suffixes ("Go"),
	// otherwise the suffix itself (".zig"), or "no ext" for extensionless files.
	Label string `json:"label"`
	// Ext is the lowercased file suffix including the dot, "" for none.
	Ext     string `json:"ext"`
	Files   int    `json:"files"`
	Matches int    `json:"matches"`
}

// searchResponse is the /api/search payload.
type searchResponse struct {
	Query string `json:"query"`
	Mode  string `json:"mode"`
	Total int    `json:"total"`
	Files int    `json:"files"`
	Limit int    `json:"limit"`
	// Truncated is true when the file limit was reached, so more files may
	// exist beyond those returned. The UI surfaces this so a capped result set
	// is never mistaken for the complete set.
	Truncated bool        `json:"truncated"`
	Facets    []facetJSON `json:"facets"`
	Repos     []repoGroup `json:"repos"`
}

// extLabels maps common file suffixes to language display names. Unknown
// suffixes fall back to the suffix itself.
var extLabels = map[string]string{
	".go":    "Go",
	".py":    "Python",
	".js":    "JavaScript",
	".mjs":   "JavaScript",
	".ts":    "TypeScript",
	".tsx":   "TSX",
	".jsx":   "JSX",
	".rs":    "Rust",
	".rb":    "Ruby",
	".java":  "Java",
	".kt":    "Kotlin",
	".swift": "Swift",
	".c":     "C",
	".h":     "C header",
	".cpp":   "C++",
	".cs":    "C#",
	".sh":    "Shell",
	".md":    "Markdown",
	".html":  "HTML",
	".css":   "CSS",
	".json":  "JSON",
	".yaml":  "YAML",
	".yml":   "YAML",
	".toml":  "TOML",
	".sql":   "SQL",
	".proto": "Protobuf",
	".tf":    "Terraform",
	".xml":   "XML",
}

// extensionFacets aggregates matches into per-file-suffix facets: distinct
// files and total matches per suffix, sorted by files desc, matches desc, then
// suffix asc for a stable order. Pure, so the quick-filter data is unit
// testable.
func extensionFacets(matches []search.Match) []facetJSON {
	idx := make(map[string]int)
	seenFile := make(map[string]bool)
	facets := []facetJSON{}

	for _, m := range matches {
		ext := strings.ToLower(path.Ext(m.File))
		i, ok := idx[ext]
		if !ok {
			i = len(facets)
			idx[ext] = i
			label := extLabels[ext]
			if label == "" {
				label = ext
			}
			if ext == "" {
				label = "no ext"
			}
			facets = append(facets, facetJSON{Label: label, Ext: ext})
		}
		facets[i].Matches++

		fkey := m.Repo + "\x00" + m.File
		if !seenFile[fkey] {
			seenFile[fkey] = true
			facets[i].Files++
		}
	}

	sort.Slice(facets, func(a, b int) bool {
		if facets[a].Files != facets[b].Files {
			return facets[a].Files > facets[b].Files
		}
		if facets[a].Matches != facets[b].Matches {
			return facets[a].Matches > facets[b].Matches
		}
		return facets[a].Ext < facets[b].Ext
	})
	return facets
}

// groupMatches turns flat matches into a repo → file → match tree, preserving
// first-appearance order, and attaches remote links using repo metadata.
func groupMatches(matches []search.Match, repos map[string]finder.Repo) []repoGroup {
	var groups []repoGroup
	repoIdx := make(map[string]int)
	fileIdx := make(map[string]int) // key: repo + "\x00" + file

	for _, m := range matches {
		ri, ok := repoIdx[m.Repo]
		if !ok {
			ri = len(groups)
			repoIdx[m.Repo] = ri
			groups = append(groups, repoGroup{Repo: m.Repo, Host: repos[m.Repo].Host})
		}

		fkey := m.Repo + "\x00" + m.File
		fi, ok := fileIdx[fkey]
		if !ok {
			fi = len(groups[ri].Files)
			fileIdx[fkey] = fi
			groups[ri].Files = append(groups[ri].Files, fileGroup{
				File:      m.File,
				LocalPath: localPath(repos[m.Repo].Path, m.File),
				FileURL:   finder.FileURL(repos[m.Repo], m.File, 0),
			})
		}

		groups[ri].Files[fi].Matches = append(groups[ri].Files[fi].Matches, matchJSON{
			Line:      m.Line,
			Column:    m.Column,
			Text:      m.Text,
			Before:    m.Before,
			After:     m.After,
			RemoteURL: finder.FileURL(repos[m.Repo], m.File, m.Line),
		})
	}
	return groups
}

// localPath joins a repo's filesystem root with a repo-relative file to the
// absolute on-disk path. It returns "" when the root is unknown, so the UI
// omits the copy-path action rather than offering a bogus path.
func localPath(repoRoot, file string) string {
	if repoRoot == "" {
		return ""
	}
	return filepath.Join(repoRoot, file)
}

// collapseHome rewrites a leading home-directory prefix to "~" so copied local
// paths are short and portable. It returns p unchanged when home is empty or p
// lies outside home.
func collapseHome(p, home string) string {
	if home == "" || p == "" {
		return p
	}
	if p == home {
		return "~"
	}
	if strings.HasPrefix(p, home+string(os.PathSeparator)) {
		return "~" + p[len(home):]
	}
	return p
}

// buildSearchResponse groups flat matches and computes pagination metadata. It
// is pure (no I/O) so the truncation signal and file/match counts are unit
// testable. Truncated is true when the returned file count reaches the limit:
// convertResults stops at the file limit, so hitting it means more files may
// match and the result set was capped.
func buildSearchResponse(
	query, mode string,
	limit int,
	matches []search.Match,
	repoMap map[string]finder.Repo,
) searchResponse {
	repos := groupMatches(matches, repoMap)
	if repos == nil {
		// Marshal as [] not null, so the UI can always call data.repos.length.
		repos = []repoGroup{}
	}
	files := 0
	for _, r := range repos {
		files += len(r.Files)
	}
	return searchResponse{
		Query:     query,
		Mode:      mode,
		Total:     len(matches),
		Files:     files,
		Limit:     limit,
		Truncated: files >= limit,
		Facets:    extensionFacets(matches),
		Repos:     repos,
	}
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	pattern := q.Get("q")
	if pattern == "" {
		writeError(w, http.StatusBadRequest, "missing required query parameter 'q'")
		return
	}
	if len(pattern) > maxQueryLength {
		writeError(w, http.StatusBadRequest, "query too long")
		return
	}

	mode := q.Get("mode")
	if mode != "content" {
		mode = "files_with_matches"
	}

	limit := clampInt(q.Get("limit"), defaultLimit, 1, maxLimit)
	opts := search.SearchOptions{
		Pattern:       pattern,
		RepoFilter:    q.Get("repo"),
		FileFilter:    q.Get("file"),
		Lang:          q.Get("lang"),
		CaseSensitive: q.Get("case") == "yes",
		Limit:         limit,
		OutputMode:    mode,
	}
	if mode == "content" {
		opts.ContextLines = clampInt(q.Get("context"), 0, 0, maxContext)
	}

	matches, err := s.svc.Search(r.Context(), opts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	repoMeta, err := s.svc.Repos()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	repoMap := make(map[string]finder.Repo, len(repoMeta))
	for _, rp := range repoMeta {
		repoMap[rp.Name] = rp
	}

	resp := buildSearchResponse(pattern, mode, limit, matches, repoMap)
	// Collapse $HOME to "~" in copy-paths here, in the imperative shell, so
	// buildSearchResponse stays a pure function of its inputs.
	if home, err := os.UserHomeDir(); err == nil {
		for i := range resp.Repos {
			for j := range resp.Repos[i].Files {
				resp.Repos[i].Files[j].LocalPath = collapseHome(resp.Repos[i].Files[j].LocalPath, home)
			}
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleRead(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	repo := q.Get("repo")
	file := q.Get("file")
	if repo == "" || file == "" {
		writeError(w, http.StatusBadRequest, "missing required parameters 'repo' and 'file'")
		return
	}
	start := clampInt(q.Get("start"), 0, 0, 1<<30)
	end := clampInt(q.Get("end"), 0, 0, 1<<30)

	res, err := s.svc.ReadFile(repo, file, start, end)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// repoJSON is one repo in the /api/repos list.
type repoJSON struct {
	Name   string `json:"name"`
	Host   string `json:"host"`
	Remote string `json:"remote"`
}

func (s *Server) handleRepos(w http.ResponseWriter, _ *http.Request) {
	repos, err := s.svc.Repos()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]repoJSON, len(repos))
	for i, rp := range repos {
		out[i] = repoJSON{Name: rp.Name, Host: rp.Host, Remote: rp.Remote}
	}
	writeJSON(w, http.StatusOK, out)
}

// clampInt parses s as an int, falling back to def, and clamps to [min, max].
func clampInt(s string, def, min, max int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	if n < min {
		return min
	}
	if n > max {
		return max
	}
	return n
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
