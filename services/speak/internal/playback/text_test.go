package playback

import "testing"

func TestExtractSections(t *testing.T) {
	src := []byte(
		"Intro paragraph.\n\n# Heading One\n\nFirst body.\n\n## Heading Two\n\nSecond body.\n",
	)
	got := ExtractSections(src)
	if len(got) != 3 {
		t.Fatalf("got %d sections, want 3: %#v", len(got), got)
	}
	if got[0] != "Intro paragraph." {
		t.Errorf("section 0 = %q", got[0])
	}
	if !contains(got[1], "Heading One") || !contains(got[1], "First body.") {
		t.Errorf("section 1 = %q", got[1])
	}
	if !contains(got[2], "Second body.") {
		t.Errorf("section 2 = %q", got[2])
	}
}

func TestExtractSectionsEmpty(t *testing.T) {
	if got := ExtractSections([]byte("   \n\n")); len(got) != 0 {
		t.Errorf("want no sections, got %#v", got)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
