package voice

import (
	"os"
	"testing"
)

func TestComputeEmpty(t *testing.T) {
	p := Compute("")
	if p.WordCount != 0 {
		t.Errorf("expected zero words, got %d", p.WordCount)
	}
}

func TestComputeBasics(t *testing.T) {
	text := "The quick brown fox jumps over the lazy dog. A second sentence."
	p := Compute(text)
	if p.WordCount != 12 {
		t.Errorf("word count: got %d, want 12", p.WordCount)
	}
	if p.SentenceCount != 2 {
		t.Errorf("sentence count: got %d, want 2", p.SentenceCount)
	}
	if p.TypeTokenRatio <= 0 || p.TypeTokenRatio > 1 {
		t.Errorf("TTR out of range: %v", p.TypeTokenRatio)
	}
}

func TestComputeHumanSample(t *testing.T) {
	data, err := os.ReadFile("../../testdata/human_sample.md")
	if err != nil {
		t.Skipf("skipping: %v", err)
	}
	p := Compute(string(data))
	if p.WordCount < 50 {
		t.Errorf("expected at least 50 words in human sample, got %d", p.WordCount)
	}
	if p.SentenceCount < 5 {
		t.Errorf("expected at least 5 sentences, got %d", p.SentenceCount)
	}
	// Contractions should be present in this sample.
	if p.ContractionRate <= 0 {
		t.Errorf("expected some contractions, got rate %v", p.ContractionRate)
	}
}

func TestDiffSortsByMagnitude(t *testing.T) {
	a := Profile{SentenceLengthMean: 20, EmDashDensity: 1}
	b := Profile{SentenceLengthMean: 10, EmDashDensity: 0.9}
	d := DiffProfiles(a, b)
	if len(d.Metrics) == 0 {
		t.Fatal("no metrics")
	}
	// The biggest delta (sentence length, 10) should come first.
	if d.Metrics[0].Metric != "sentence_length_mean" {
		t.Errorf("expected sentence_length_mean first, got %q", d.Metrics[0].Metric)
	}
}
