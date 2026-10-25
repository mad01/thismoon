package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mad01/thismoon/kit/gitloc"
	"github.com/mad01/thismoon/services/moondb/internal/client"
)

// Display limits for the text renderings.
const (
	shortIDLen    = 8
	promptPreview = 120
	gitLookup     = 2 * time.Second
)

// located is a file resolved against git: the checkout it sits in and its
// path inside it. Outside any repository Worktree is "" and Path absolute.
type located struct {
	Worktree string
	Repo     string
	Path     string
}

// locate resolves a path argument the way record does for an edit, so the
// query hits the same keys: symlinks resolved, worktree from the file's own
// directory, path relative to it with forward slashes.
func locate(ctx context.Context, path string) (located, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return located{}, err
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	ctx, cancel := context.WithTimeout(ctx, gitLookup)
	defer cancel()
	loc := gitloc.LocationAt(ctx, filepath.Dir(abs))
	if loc.Toplevel == "" {
		return located{Path: abs}, nil
	}
	rel, err := filepath.Rel(loc.Toplevel, abs)
	if err != nil {
		return located{}, err
	}
	return located{Worktree: loc.Toplevel, Repo: loc.Repo, Path: filepath.ToSlash(rel)}, nil
}

// printJSON writes v indented, the shape the API returns.
func printJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// short trims an id for a table column.
func short(id string) string {
	if len(id) <= shortIDLen {
		return id
	}
	return id[:shortIDLen]
}

// firstLine is the first line of s, cut to n runes with an ellipsis.
func firstLine(s string, n int) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	runes := []rune(s)
	return string(runes[:n]) + "..."
}

// promptOf fetches a turn's prompt text for display, "" when the chunk is
// missing or unreadable: a preview is never worth failing the command.
func promptOf(ctx context.Context, c *client.Client, chunk string) string {
	if chunk == "" {
		return ""
	}
	raw, err := c.Chunk(ctx, chunk)
	if err != nil {
		return ""
	}
	return firstLine(string(raw), promptPreview)
}

// webURL is the page a human opens: --base-url when set, else the API.
func webURL() string {
	if flagBaseURL != "" {
		return strings.TrimRight(flagBaseURL, "/")
	}
	return apiURL()
}

// sessionURL is the web page for one session.
func sessionURL(id string) string {
	return webURL() + "/?session=" + url.QueryEscape(id)
}

// orDash shows "" as a dash in a table.
func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// when formats a time for a table, local and to the minute.
func when(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Local().Format("2006-01-02 15:04")
}

// agentLabel names who made an edit: the subagent type, else main.
func agentLabel(agentType string) string {
	if agentType == "" {
		return "main"
	}
	return agentType
}

// fmtCount is a right-aligned count for a table cell.
func fmtCount(n int) string {
	return fmt.Sprintf("%d", n)
}
