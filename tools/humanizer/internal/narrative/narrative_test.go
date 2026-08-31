package narrative

import (
	"strings"
	"testing"
)

func TestFeaturesIntegrity(t *testing.T) {
	fs := Features()
	if len(fs) != 30 {
		t.Fatalf("expected 30 core features, got %d", len(fs))
	}
	themes := make(map[string]bool, len(Themes()))
	for _, th := range Themes() {
		themes[th] = true
	}
	seen := make(map[string]bool, len(fs))
	var ai, human int
	for _, f := range fs {
		if f.ID == "" || f.Name == "" || f.Question == "" || f.Dimension == "" || f.Direction == "" {
			t.Errorf("feature %q has an empty required field: %+v", f.ID, f)
		}
		if seen[f.ID] {
			t.Errorf("duplicate feature ID %q", f.ID)
		}
		seen[f.ID] = true
		if !themes[f.Theme] {
			t.Errorf("feature %q has unknown theme %q", f.ID, f.Theme)
		}
		switch f.Signal {
		case SignalAI:
			ai++
		case SignalHuman:
			human++
		default:
			t.Errorf("feature %q has invalid signal %q", f.ID, f.Signal)
		}
	}
	if ai == 0 || human == 0 {
		t.Errorf("expected both signals present, got ai=%d human=%d", ai, human)
	}
}

func TestFeaturesReturnsCopy(t *testing.T) {
	a := Features()
	a[0].ID = "mutated"
	if b := Features(); b[0].ID == "mutated" {
		t.Fatal("Features() exposes internal slice; callers can corrupt the rubric")
	}
}

func TestBuildRubricConsistency(t *testing.T) {
	r := BuildRubric()
	if r.Total != len(r.Features) {
		t.Errorf("rubric Total %d != len(Features) %d", r.Total, len(r.Features))
	}
	if r.Prompt != Prompt() || r.Source != Source {
		t.Error("rubric payload diverges from the package's Prompt/Source")
	}
	if len(r.Themes) != len(Themes()) {
		t.Errorf("rubric has %d themes, want %d", len(r.Themes), len(Themes()))
	}
}

func TestPromptCoversEveryFeatureAndTheme(t *testing.T) {
	p := Prompt()
	if !strings.Contains(p, Source) {
		t.Errorf("prompt does not cite the source %s", Source)
	}
	for _, contract := range []string{"VERDICT", "counter-direction", "stdin"} {
		if !strings.Contains(p, contract) {
			t.Errorf("prompt is missing the %q contract clause", contract)
		}
	}
	for _, th := range Themes() {
		if !strings.Contains(p, "## "+th) {
			t.Errorf("prompt is missing theme section %q", th)
		}
	}
	for _, f := range Features() {
		if !strings.Contains(p, f.Name) {
			t.Errorf("prompt is missing feature %q", f.Name)
		}
		if !strings.Contains(p, f.Question) {
			t.Errorf("prompt is missing question for %q", f.ID)
		}
	}
}
