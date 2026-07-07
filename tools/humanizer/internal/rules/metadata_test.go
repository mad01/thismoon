package rules

import "testing"

func TestAllRulesHaveMetadata(t *testing.T) {
	rs := All()
	if len(rs) < 20 {
		t.Fatalf("expected at least 20 rules, got %d", len(rs))
	}
	validCats := map[string]bool{
		"content": true, "language": true, "style": true, "communication": true,
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
