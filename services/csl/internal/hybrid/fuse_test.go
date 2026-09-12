package hybrid

import (
	"testing"

	"github.com/mad01/thismoon/services/csl/internal/search"
	"github.com/mad01/thismoon/services/csl/internal/semantic"
)

func lex(repo, file string, line int) search.Match {
	return search.Match{Repo: repo, File: file, Line: line, Text: "x"}
}

func sem(repo, path string, start int) semantic.Result {
	return semantic.Result{
		Hit: semantic.Hit{Repo: repo, Path: path, StartLine: start, EndLine: start + 5, Score: 0.9},
	}
}

func paths(hits []FusedHit) []string {
	out := make([]string, len(hits))
	for i, h := range hits {
		out[i] = h.Repo + "/" + h.Path
	}
	return out
}

func TestFuse_ConsensusOutranksSingleListWinners(t *testing.T) {
	// "b" is rank 2 in both lists; "a" tops lexical only, "c" tops semantic only.
	// RRF should rank the consensus file "b" first.
	lexical := []search.Match{
		lex("o/r", "a.go", 1),
		lex("o/r", "b.go", 10),
	}
	semantic := []semantic.Result{
		sem("o/r", "c.go", 1),
		sem("o/r", "b.go", 20),
	}

	got := paths(Fuse(lexical, semantic, DefaultK, 0))
	if len(got) != 3 || got[0] != "o/r/b.go" {
		t.Fatalf("consensus file should rank first; got order %v", got)
	}
}

func TestFuse_LexicalOnly(t *testing.T) {
	got := Fuse([]search.Match{lex("o/r", "a.go", 3)}, nil, DefaultK, 0)
	if len(got) != 1 {
		t.Fatalf("want 1 hit, got %d", len(got))
	}
	h := got[0]
	if h.LexRank != 1 || h.LexLine != 3 || h.SemRank != 0 {
		t.Fatalf("lexical-only evidence wrong: %+v", h)
	}
}

func TestFuse_SemanticOnly(t *testing.T) {
	got := Fuse(nil, []semantic.Result{sem("o/r", "a.go", 7)}, DefaultK, 0)
	if len(got) != 1 {
		t.Fatalf("want 1 hit, got %d", len(got))
	}
	h := got[0]
	if h.SemRank != 1 || h.SemStart != 7 || h.LexRank != 0 {
		t.Fatalf("semantic-only evidence wrong: %+v", h)
	}
}

func TestFuse_DedupKeepsFirstRankPerBackend(t *testing.T) {
	// Two lexical matches in the same file: the file gets rank 1 (not 1 and 2),
	// and keeps the first line as evidence.
	lexical := []search.Match{
		lex("o/r", "a.go", 5),
		lex("o/r", "a.go", 50),
		lex("o/r", "b.go", 1),
	}
	got := Fuse(lexical, nil, DefaultK, 0)
	if len(got) != 2 {
		t.Fatalf("want 2 unique files, got %d: %v", len(got), paths(got))
	}
	if got[0].Path != "a.go" || got[0].LexLine != 5 {
		t.Fatalf("first file should be a.go @ line 5, got %+v", got[0])
	}
	// a.go is rank 1, b.go is rank 2 → a.go scores higher.
	if got[0].Score <= got[1].Score {
		t.Fatalf("rank-1 file should outscore rank-2; got %v / %v", got[0].Score, got[1].Score)
	}
}

func TestFuse_TieOrdering(t *testing.T) {
	// Both files appear only in lexical at the same rank position across separate
	// calls is impossible; instead give two files identical single-list rank by
	// putting each first in one backend. Equal scores → deterministic Repo/Path.
	lexical := []search.Match{lex("o/r", "z.go", 1)}
	semantic := []semantic.Result{sem("o/r", "a.go", 1)}
	got := paths(Fuse(lexical, semantic, DefaultK, 0))
	// Both have score 1/(k+1); tiebreak by path → a.go before z.go.
	if got[0] != "o/r/a.go" || got[1] != "o/r/z.go" {
		t.Fatalf("tie should order by path; got %v", got)
	}
}

func TestFuse_Limit(t *testing.T) {
	lexical := []search.Match{
		lex("o/r", "a.go", 1),
		lex("o/r", "b.go", 1),
		lex("o/r", "c.go", 1),
	}
	got := Fuse(lexical, nil, DefaultK, 2)
	if len(got) != 2 {
		t.Fatalf("limit should cap to 2, got %d", len(got))
	}
}

func TestFuse_KFallback(t *testing.T) {
	// k <= 0 must fall back to DefaultK and not divide by a small/zero k.
	got := Fuse([]search.Match{lex("o/r", "a.go", 1)}, nil, 0, 0)
	want := 1.0 / float64(DefaultK+1)
	if got[0].Score != want {
		t.Fatalf("k=0 should use DefaultK; score = %v, want %v", got[0].Score, want)
	}
}
