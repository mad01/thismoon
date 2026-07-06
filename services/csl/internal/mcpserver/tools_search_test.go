package mcpserver

import (
	"fmt"
	"testing"

	"github.com/mad01/thismoon/services/csl/internal/search"
)

func makeMatches(n int, repo, file string) []search.Match {
	matches := make([]search.Match, n)
	for i := range n {
		matches[i] = search.Match{
			Repo:   repo,
			File:   file,
			Line:   i + 1,
			Column: 1,
			Text:   "match line",
		}
	}
	return matches
}

func makeFileMatches(n int, repo string) []search.Match {
	matches := make([]search.Match, n)
	for i := range n {
		matches[i] = search.Match{
			Repo:   repo,
			File:   fmt.Sprintf("pkg%d/file%d.go", i/10, i),
			Line:   1,
			Column: 1,
			Text:   "match",
		}
	}
	return matches
}

func TestBuildSearchOutput_FilesNotTruncated(t *testing.T) {
	matches := makeFileMatches(10, "org/repo")
	out := buildSearchOutput(filesOutputMode, 50, matches)

	if out.Truncated {
		t.Error("expected truncated=false for 10 files with limit 50")
	}
	if out.TotalAvailable != 0 {
		t.Errorf("expected total_available=0 when not truncated, got %d", out.TotalAvailable)
	}
	if out.Total != 10 {
		t.Errorf("expected total=10, got %d", out.Total)
	}
}

func TestBuildSearchOutput_FilesTruncated(t *testing.T) {
	matches := makeFileMatches(60, "org/repo")
	out := buildSearchOutput(filesOutputMode, 50, matches)

	if !out.Truncated {
		t.Error("expected truncated=true when files >= limit")
	}
	if out.TotalAvailable != 60 {
		t.Errorf("expected total_available=60, got %d", out.TotalAvailable)
	}
	if out.Total != 50 {
		t.Errorf("expected total=50 (capped at limit), got %d", out.Total)
	}
	if len(out.Files) != 50 {
		t.Errorf("expected 50 file entries, got %d", len(out.Files))
	}
}

func TestBuildSearchOutput_FilesExactlyAtLimit(t *testing.T) {
	matches := makeFileMatches(50, "org/repo")
	out := buildSearchOutput(filesOutputMode, 50, matches)

	if !out.Truncated {
		t.Error("expected truncated=true when files == limit (more may exist upstream)")
	}
	if out.Total != 50 {
		t.Errorf("expected total=50, got %d", out.Total)
	}
}

func TestBuildSearchOutput_ContentNotTruncated(t *testing.T) {
	matches := makeMatches(100, "org/repo", "main.go")
	out := buildSearchOutput(contentOutputMode, 50, matches)

	if out.Truncated {
		t.Error("expected truncated=false for 100 lines (under maxContentLines)")
	}
	if out.Total != 100 {
		t.Errorf("expected total=100, got %d", out.Total)
	}
}

func TestBuildSearchOutput_ContentTruncatedAtMaxLines(t *testing.T) {
	matches := makeMatches(500, "org/repo", "main.go")
	out := buildSearchOutput(contentOutputMode, 50, matches)

	if !out.Truncated {
		t.Error("expected truncated=true for 500 lines")
	}
	if out.TotalAvailable != 500 {
		t.Errorf("expected total_available=500, got %d", out.TotalAvailable)
	}
	if out.Total != maxContentLines {
		t.Errorf("expected total=%d (maxContentLines), got %d", maxContentLines, out.Total)
	}
	if len(out.Lines) != maxContentLines {
		t.Errorf("expected %d line entries, got %d", maxContentLines, len(out.Lines))
	}
}

func TestBuildSearchOutput_ContentExactlyAtMax(t *testing.T) {
	matches := makeMatches(maxContentLines, "org/repo", "main.go")
	out := buildSearchOutput(contentOutputMode, 50, matches)

	if out.Truncated {
		t.Error("expected truncated=false when lines == maxContentLines exactly")
	}
	if out.Total != maxContentLines {
		t.Errorf("expected total=%d, got %d", maxContentLines, out.Total)
	}
}

func TestBuildSearchOutput_EmptyResults(t *testing.T) {
	out := buildSearchOutput(filesOutputMode, 50, nil)
	if out.Truncated {
		t.Error("expected truncated=false for empty results")
	}
	if out.Total != 0 {
		t.Errorf("expected total=0, got %d", out.Total)
	}

	out = buildSearchOutput(contentOutputMode, 50, nil)
	if out.Truncated {
		t.Error("expected truncated=false for empty content results")
	}
}

func TestBuildSearchOutput_FilesDeduplicated(t *testing.T) {
	// Multiple matches in the same file should collapse to one entry.
	matches := makeMatches(20, "org/repo", "main.go")
	out := buildSearchOutput(filesOutputMode, 50, matches)

	if out.Total != 1 {
		t.Errorf("expected 1 unique file, got %d", out.Total)
	}
	if out.Truncated {
		t.Error("expected truncated=false for 1 unique file")
	}
}
