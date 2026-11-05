package hybrid

import "testing"

func TestFuse_BothListsAgreeOnTop(t *testing.T) {
	hits := Fuse([]Input{
		{Source: "zoekt", IDs: []string{"a", "b", "c"}},
		{Source: "vector", IDs: []string{"a", "c", "b"}},
	}, DefaultK)

	if hits[0].ID != "a" {
		t.Errorf("top = %q, want a", hits[0].ID)
	}
	if len(hits[0].Sources) != 2 {
		t.Errorf("top sources = %v, want both", hits[0].Sources)
	}
	if hits[0].ZoektRank != 1 || hits[0].VectorRank != 1 {
		t.Errorf("top ranks: zoekt=%d vector=%d", hits[0].ZoektRank, hits[0].VectorRank)
	}
}

func TestFuse_OnlyOneSource(t *testing.T) {
	hits := Fuse([]Input{
		{Source: "vector", IDs: []string{"only"}},
	}, DefaultK)
	if len(hits) != 1 {
		t.Fatalf("got %d", len(hits))
	}
	if hits[0].ID != "only" {
		t.Errorf("got %q", hits[0].ID)
	}
	if len(hits[0].Sources) != 1 || hits[0].Sources[0] != "vector" {
		t.Errorf("Sources = %v", hits[0].Sources)
	}
	if hits[0].ZoektRank != 0 {
		t.Errorf("ZoektRank = %d, want 0 (absent)", hits[0].ZoektRank)
	}
}

func TestFuse_AgreementBeatsDeepSingleSource(t *testing.T) {
	// Doc that appears in BOTH lists at rank 2 wins against any single-source
	// doc at rank 4+. The QMD bonus schedule (0.05 / 0.02 / 0.02 for top-3)
	// favors top-1 hits even from a single source, so use rank 2 to make
	// the agreement signal dominate.
	hits := Fuse([]Input{
		{Source: "zoekt", IDs: []string{"top-z", "shared", "noise1", "noise2"}},
		{Source: "vector", IDs: []string{"top-v", "shared", "noise3", "noise4"}},
	}, DefaultK)
	var sharedIdx, deep1Idx int
	sharedIdx, deep1Idx = -1, -1
	for i, h := range hits {
		switch h.ID {
		case "shared":
			sharedIdx = i
		case "noise1":
			deep1Idx = i
		}
	}
	if sharedIdx < 0 || deep1Idx < 0 {
		t.Fatalf("missing ids: %+v", hits)
	}
	if sharedIdx >= deep1Idx {
		t.Errorf(
			"shared at %d should rank above noise1 at %d (agreement bonus)",
			sharedIdx,
			deep1Idx,
		)
	}
}

func TestFuse_DefaultK(t *testing.T) {
	a := Fuse([]Input{{Source: "zoekt", IDs: []string{"x"}}}, 0)
	b := Fuse([]Input{{Source: "zoekt", IDs: []string{"x"}}}, DefaultK)
	if a[0].Score != b[0].Score {
		t.Errorf("k<=0 should default to %d", DefaultK)
	}
}

func TestFuse_StableTieBreak(t *testing.T) {
	// Two docs each appearing once at rank 1 of different sources. They
	// share the same RRF score. Tie-break by ID alphabetical.
	hits := Fuse([]Input{
		{Source: "zoekt", IDs: []string{"b"}},
		{Source: "vector", IDs: []string{"a"}},
	}, DefaultK)
	if hits[0].ID != "a" {
		t.Errorf("first = %q, want a (alpha tie-break)", hits[0].ID)
	}
}

func TestFuse_PositionBonus(t *testing.T) {
	// QMD-style top-3 bonus: rank-1 (+0.05) is large enough that a single
	// rank-1 hit clearly beats a single rank-4+ hit from any other list.
	hits := Fuse([]Input{
		{Source: "zoekt", IDs: []string{"strong"}},
		{Source: "vector", IDs: []string{"a", "b", "c", "deep"}},
	}, DefaultK)
	// strong: 1/61 + 0.05 = 0.0664
	// a:      1/61 + 0.05 = 0.0664 — ties with strong, "a" wins on alpha
	// We want to verify position bonus magnitude. Inspect "deep" rank vs
	// "strong": strong (0.0664) > deep (1/64 = 0.0156).
	var sIdx, dIdx int
	sIdx, dIdx = -1, -1
	for i, h := range hits {
		switch h.ID {
		case "strong":
			sIdx = i
		case "deep":
			dIdx = i
		}
	}
	if sIdx < 0 || dIdx < 0 {
		t.Fatalf("missing ids: %+v", hits)
	}
	if sIdx >= dIdx {
		t.Errorf(
			"strong at %d should rank above deep at %d (top-1 bonus dominates rank-4)",
			sIdx,
			dIdx,
		)
	}
}
