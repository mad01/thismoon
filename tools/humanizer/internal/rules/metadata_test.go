package rules

import "testing"

func TestAllRulesHaveMetadata(t *testing.T) {
	rs := All()
	if len(rs) < 20 {
		t.Fatalf("expected at least 20 rules, got %d", len(rs))
	}
	validCats := map[string]bool{
		"content": true, "language": true, "style": true, "communication": true, "clarity": true,
		"ste": true,
	}
	for _, r := range rs {
		if r.ID == "" {
			t.Errorf("rule with empty ID: %+v", r)
		}
		if r.Name == "" {
			t.Errorf("%s: missing humanizer-name", r.ID)
		}
		if !validCats[r.Category] {
			t.Errorf("%s: invalid category %q", r.ID, r.Category)
		}
		if r.Summary == "" {
			t.Errorf("%s: missing humanizer-summary", r.ID)
		}
		if r.DefaultSeverity == "" {
			t.Errorf("%s: missing level: header", r.ID)
		}
	}
}

func TestGetKnownRule(t *testing.T) {
	r, ok := Get("Humanizer.AIVocabulary")
	if !ok {
		t.Fatal("expected Humanizer.AIVocabulary to be registered")
	}
	if r.Category != "language" {
		t.Errorf("expected category=language, got %q", r.Category)
	}
}

func TestGetUnknownRule(t *testing.T) {
	if _, ok := Get("Humanizer.DoesNotExist"); ok {
		t.Fatal("expected unknown rule lookup to fail")
	}
}

func TestCategories(t *testing.T) {
	cats := Categories()
	if len(cats) == 0 {
		t.Fatal("expected at least one category")
	}
}

// TestProfilesBelongToSTE pins the profile contract: every STE rule names at
// least one valid profile, and no Humanizer rule names any, since a profile
// run loads only the STE style.
func TestProfilesBelongToSTE(t *testing.T) {
	for _, r := range All() {
		if !isSTERule(r.ID) {
			if len(r.Profiles) > 0 {
				t.Errorf("%s: non-STE rule has profiles %v", r.ID, r.Profiles)
			}
			continue
		}
		if r.Category != "ste" {
			t.Errorf("%s: category %q, want ste", r.ID, r.Category)
		}
		if len(r.Profiles) == 0 {
			t.Errorf("%s: STE rule has no humanizer-profile header", r.ID)
		}
		for _, p := range r.Profiles {
			if _, err := ParseProfile(p); err != nil {
				t.Errorf("%s: %v", r.ID, err)
			}
		}
	}
}

// TestGlossaryRuleRegistered checks the generated rule's metadata, parsed from
// the header glossary.Render writes, is served like an embedded rule's.
func TestGlossaryRuleRegistered(t *testing.T) {
	r, ok := Get("STE.Glossary")
	if !ok {
		t.Fatal("expected STE.Glossary to be registered")
	}
	if r.DefaultSeverity != "suggestion" {
		t.Errorf("severity = %q, want suggestion", r.DefaultSeverity)
	}
	if len(r.Profiles) != 2 {
		t.Errorf("profiles = %v, want procedure and description", r.Profiles)
	}
	if r.File != "" {
		t.Errorf("file = %q, want empty for a generated rule", r.File)
	}
}
